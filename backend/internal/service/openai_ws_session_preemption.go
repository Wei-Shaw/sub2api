package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	coderws "github.com/coder/websocket"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

var errOpenAIWSSessionPreempted = errors.New("openai ws session preempted by newer request")

const (
	openAIWSSessionPreemptOwnerTTL      = 2 * time.Hour
	openAIWSSessionPreemptWatchInterval = 2 * time.Second
	openAIWSSessionPreemptCachePrefix   = "wspreempt:"
	// openAIWSSessionPreemptCloseGrace 是被抢占连接从收到关闭帧到被取消的最长等待：
	// 关闭帧在 Close 一开始就写出，其余时间只是等对端回应，到点直接取消。
	openAIWSSessionPreemptCloseGrace    = time.Second
	openAIWSSessionPreemptedCloseReason = "session preempted by a newer connection"
)

// openAIWSPreemptClientCloser 是被抢占时用来给旧客户端发关闭帧的连接，*coderws.Conn 满足该接口。
type openAIWSPreemptClientCloser interface {
	Close(code coderws.StatusCode, reason string) error
}

// openAIWSSessionPreemptState 随抢占上下文传递：preempted 在关闭帧发出前就置位，
// 让旧连接在被取消之前遇到的读写错误也能归类为“被抢占”。
type openAIWSSessionPreemptState struct {
	preempted atomic.Bool
}

// OpenAIWSSessionPreemptionCache is an optional GatewayCache capability. The
// production Redis cache implements all operations atomically; cache stubs do
// not need to implement it for ordinary gateway tests.
type OpenAIWSSessionPreemptionCache interface {
	ClaimOpenAIResponsesSessionWindow(ctx context.Context, groupID int64, sessionHash string, owner []byte, ttl time.Duration) ([]byte, error)
	CompareAndRefreshOpenAIResponsesSessionWindow(ctx context.Context, groupID int64, sessionHash string, expected []byte, ttl time.Duration) (bool, error)
	CompareAndDeleteOpenAIResponsesSessionWindow(ctx context.Context, groupID int64, sessionHash string, expected []byte) (bool, error)
}

func NewOpenAIWSSessionPreemptedError() error {
	return errOpenAIWSSessionPreempted
}

type openAIWSSessionPreemptKey struct {
	groupID     int64
	apiKeyID    int64
	sessionHash string
}

type openAIWSSessionPreemptContextKey struct{}

// BeginOpenAIWSIngressSessionPreemption keeps a persistent inbound WS session
// registered across upstream retry attempts. Nested forwarding calls reuse the
// registration so returning from one attempt cannot create a preemption gap.
func (s *OpenAIGatewayService) BeginOpenAIWSIngressSessionPreemption(
	ctx context.Context,
	c *gin.Context,
	account *Account,
	firstClientMessage []byte,
) (context.Context, func(), bool) {
	return s.BeginOpenAIWSIngressSessionPreemptionWithClient(ctx, c, account, firstClientMessage, nil)
}

// BeginOpenAIWSIngressSessionPreemptionWithClient 与 BeginOpenAIWSIngressSessionPreemption 相同，
// 另外登记客户端连接：本连接被更新的连接取代时，先给它发带原因的关闭帧，再取消上下文，
// 客户端因此能立即重试，而不是在裸断开后静默等到空闲超时。
func (s *OpenAIGatewayService) BeginOpenAIWSIngressSessionPreemptionWithClient(
	ctx context.Context,
	c *gin.Context,
	account *Account,
	firstClientMessage []byte,
	clientConn openAIWSPreemptClientCloser,
) (context.Context, func(), bool) {
	if ctx == nil {
		ctx = context.Background()
	}
	if state, _ := ctx.Value(openAIWSSessionPreemptContextKey{}).(*openAIWSSessionPreemptState); state != nil {
		return ctx, func() {}, true
	}
	var notifyPreempted func()
	if clientConn != nil {
		notifyPreempted = func() {
			_ = clientConn.Close(coderws.StatusTryAgainLater, openAIWSSessionPreemptedCloseReason)
		}
	}
	if s != nil && s.cfg != nil && s.cfg.Gateway.OpenAIWS.ModeRouterV2Enabled && account != nil {
		switch account.ResolveOpenAIResponsesWebSocketV2Mode(s.cfg.Gateway.OpenAIWS.IngressModeDefault) {
		case OpenAIWSIngressModePassthrough, OpenAIWSIngressModeHTTPBridge:
			// These modes own their upstream transport and replay state per
			// inbound connection. Sharing a sticky/cache identity does not mean
			// a new connection should cancel an independent in-flight request.
			return ctx, func() {}, false
		}
	}

	preemptScope := ""
	preemptThreadID := ""
	preemptGroupID := getOpenAIGroupIDFromContext(c)
	preemptAPIKeyID := getAPIKeyIDFromContext(c)
	if account != nil && account.Platform == PlatformOpenAI && account.Type == AccountTypeOAuth {
		// Codex multi-agent sessions share one session-id across the parent thread
		// and every sub-agent. Key preemption by the declared execution identity
		// (thread first, explicit session otherwise) so siblings never cancel each
		// other while a reconnect of the same thread still replaces it. Requests
		// without any declared identity are not registered at all.
		preemptScope, preemptThreadID = resolveOpenAIWSExecutionScope(c, firstClientMessage, preemptAPIKeyID)
	}
	preemptCtx, cleanup, armed, preemptedPrevious := s.beginOpenAIWSSessionPreemptContext(
		ctx,
		account,
		preemptGroupID,
		preemptAPIKeyID,
		preemptScope,
		false,
		notifyPreempted,
	)
	if !armed {
		return ctx, func() {}, false
	}
	if preemptedPrevious {
		if stateStore := s.getOpenAIWSStateStore(); stateStore != nil {
			stateStore.DeleteSessionTurnState(preemptGroupID, preemptScope)
			stateStore.DeleteSessionConn(preemptGroupID, preemptScope)
		}
		lane := resolveOpenAIWSExecutionLane(c, firstClientMessage)
		if lane == "" {
			lane = "main"
		}
		logOpenAIWSModeInfo(
			"ingress_ws_session_preempted account_id=%d group_id=%d api_key_id=%d scope=%s thread_id=%s lane=%s",
			account.ID,
			preemptGroupID,
			preemptAPIKeyID,
			truncateOpenAIWSLogValue(preemptScope, 12),
			truncateOpenAIWSLogValue(preemptThreadID, openAIWSIDValueMaxLen),
			truncateOpenAIWSLogValue(lane, openAIWSIDValueMaxLen),
		)
	}
	return preemptCtx, cleanup, true
}

func newOpenAIWSSessionPreemptKey(groupID, apiKeyID int64, sessionHash string) (openAIWSSessionPreemptKey, bool) {
	sessionHash = strings.TrimSpace(sessionHash)
	if groupID <= 0 || apiKeyID <= 0 || sessionHash == "" {
		return openAIWSSessionPreemptKey{}, false
	}
	return openAIWSSessionPreemptKey{groupID: groupID, apiKeyID: apiKeyID, sessionHash: sessionHash}, true
}

func openAIWSSessionPreemptCacheHash(apiKeyID int64, sessionHash string) string {
	return fmt.Sprintf("%s%d:%s", openAIWSSessionPreemptCachePrefix, apiKeyID, strings.TrimSpace(sessionHash))
}

type openAIWSSessionPreemptEntry struct {
	generation uint64
	cancel     func()
}

type openAIWSSessionPreemptRegistry struct {
	mu     sync.Mutex
	next   uint64
	active map[openAIWSSessionPreemptKey]openAIWSSessionPreemptEntry
}

func (r *openAIWSSessionPreemptRegistry) Begin(key openAIWSSessionPreemptKey, cancel func()) (cleanup func(), preemptedPrevious bool) {
	if r == nil || strings.TrimSpace(key.sessionHash) == "" {
		return func() {}, false
	}
	r.mu.Lock()
	if r.active == nil {
		r.active = make(map[openAIWSSessionPreemptKey]openAIWSSessionPreemptEntry)
	}
	r.next++
	generation := r.next
	previous, hadPrevious := r.active[key]
	r.active[key] = openAIWSSessionPreemptEntry{generation: generation, cancel: cancel}
	r.mu.Unlock()
	if hadPrevious && previous.cancel != nil {
		previous.cancel()
	}
	return func() {
		r.mu.Lock()
		defer r.mu.Unlock()
		current, ok := r.active[key]
		if ok && current.generation == generation {
			delete(r.active, key)
		}
	}, hadPrevious
}

func (s *OpenAIGatewayService) beginOpenAIWSSessionPreemptContext(
	ctx context.Context,
	account *Account,
	groupID, apiKeyID int64,
	sessionHash string,
	httpIngressWSOneShot bool,
	notifyPreempted func(),
) (context.Context, func(), bool, bool) {
	if ctx == nil {
		ctx = context.Background()
	}
	if s == nil || account == nil || account.Platform != PlatformOpenAI || account.Type != AccountTypeOAuth || httpIngressWSOneShot {
		return ctx, func() {}, false, false
	}
	key, ok := newOpenAIWSSessionPreemptKey(groupID, apiKeyID, sessionHash)
	if !ok {
		return ctx, func() {}, false, false
	}

	state := &openAIWSSessionPreemptState{}
	preemptCtx, cancel := context.WithCancelCause(context.WithValue(ctx, openAIWSSessionPreemptContextKey{}, state))
	ownerToken := uuid.NewString()
	var preemptOnce sync.Once
	preempt := func() {
		preemptOnce.Do(func() {
			state.preempted.Store(true)
			if stateStore := s.getOpenAIWSStateStore(); stateStore != nil {
				stateStore.DeleteSessionTurnState(key.groupID, key.sessionHash)
				stateStore.DeleteSessionConn(key.groupID, key.sessionHash)
			}
			if notifyPreempted == nil {
				cancel(errOpenAIWSSessionPreempted)
				return
			}
			// 取消会让 coder 立即关掉正在读写的客户端连接，关闭帧必须先于取消发出；
			// 通知在独立协程里做，不阻塞取代它的新连接。
			notified := make(chan struct{})
			go func() {
				defer close(notified)
				notifyPreempted()
			}()
			go func() {
				select {
				case <-notified:
				case <-time.After(openAIWSSessionPreemptCloseGrace):
				}
				cancel(errOpenAIWSSessionPreempted)
			}()
		})
	}
	previousRemoteOwner, remoteClaimed := s.claimOpenAIWSSessionPreemptOwner(ctx, key, ownerToken)
	preemptedPrevious := remoteClaimed && previousRemoteOwner != "" && previousRemoteOwner != ownerToken
	cleanupLocal, hadLocalPrevious := s.openaiWSSessionPreemptions.Begin(key, preempt)
	preemptedPrevious = preemptedPrevious || hadLocalPrevious
	stopWatch := func() {}
	if remoteClaimed {
		stopWatch = s.watchOpenAIWSSessionPreemptOwner(preemptCtx, key, ownerToken, preempt)
	}

	return preemptCtx, func() {
		stopWatch()
		cleanupLocal()
		if remoteClaimed {
			s.releaseOpenAIWSSessionPreemptOwner(context.Background(), key, ownerToken)
		}
		cancel(nil)
	}, true, preemptedPrevious
}

func (s *OpenAIGatewayService) openAIWSSessionPreemptionCache() OpenAIWSSessionPreemptionCache {
	if s == nil || s.cache == nil {
		return nil
	}
	cache, _ := s.cache.(OpenAIWSSessionPreemptionCache)
	return cache
}

func (s *OpenAIGatewayService) claimOpenAIWSSessionPreemptOwner(ctx context.Context, key openAIWSSessionPreemptKey, ownerToken string) (string, bool) {
	cache := s.openAIWSSessionPreemptionCache()
	if cache == nil || strings.TrimSpace(ownerToken) == "" {
		return "", false
	}
	cacheCtx, cancel := context.WithTimeout(ctx, openAIWSStateStoreRedisTimeout)
	defer cancel()
	previous, err := cache.ClaimOpenAIResponsesSessionWindow(
		cacheCtx,
		key.groupID,
		openAIWSSessionPreemptCacheHash(key.apiKeyID, key.sessionHash),
		[]byte(strings.TrimSpace(ownerToken)),
		openAIWSSessionPreemptOwnerTTL,
	)
	if err != nil {
		return "", false
	}
	return strings.TrimSpace(string(previous)), true
}

func (s *OpenAIGatewayService) releaseOpenAIWSSessionPreemptOwner(ctx context.Context, key openAIWSSessionPreemptKey, ownerToken string) {
	cache := s.openAIWSSessionPreemptionCache()
	if cache == nil || strings.TrimSpace(ownerToken) == "" {
		return
	}
	cacheCtx, cancel := context.WithTimeout(ctx, openAIWSStateStoreRedisTimeout)
	defer cancel()
	_, _ = cache.CompareAndDeleteOpenAIResponsesSessionWindow(
		cacheCtx,
		key.groupID,
		openAIWSSessionPreemptCacheHash(key.apiKeyID, key.sessionHash),
		[]byte(strings.TrimSpace(ownerToken)),
	)
}

func (s *OpenAIGatewayService) watchOpenAIWSSessionPreemptOwner(ctx context.Context, key openAIWSSessionPreemptKey, ownerToken string, onLost func()) func() {
	cache := s.openAIWSSessionPreemptionCache()
	if cache == nil || onLost == nil || strings.TrimSpace(ownerToken) == "" {
		return func() {}
	}
	stopCh := make(chan struct{})
	var once sync.Once
	go func() {
		ticker := time.NewTicker(openAIWSSessionPreemptWatchInterval)
		defer ticker.Stop()
		for {
			select {
			case <-stopCh:
				return
			case <-ctx.Done():
				return
			case <-ticker.C:
				cacheCtx, cancel := context.WithTimeout(context.Background(), openAIWSStateStoreRedisTimeout)
				owned, err := cache.CompareAndRefreshOpenAIResponsesSessionWindow(
					cacheCtx,
					key.groupID,
					openAIWSSessionPreemptCacheHash(key.apiKeyID, key.sessionHash),
					[]byte(strings.TrimSpace(ownerToken)),
					openAIWSSessionPreemptOwnerTTL,
				)
				cancel()
				if err == nil && !owned {
					onLost()
					return
				}
			}
		}
	}()
	return func() { once.Do(func() { close(stopCh) }) }
}

func isOpenAIWSSessionPreempted(ctx context.Context) bool {
	if ctx == nil {
		return false
	}
	if state, _ := ctx.Value(openAIWSSessionPreemptContextKey{}).(*openAIWSSessionPreemptState); state != nil && state.preempted.Load() {
		return true
	}
	return errors.Is(context.Cause(ctx), errOpenAIWSSessionPreempted)
}

func IsOpenAIWSSessionPreemptedError(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, errOpenAIWSSessionPreempted) {
		return true
	}
	var fallbackErr *openAIWSFallbackError
	return errors.As(err, &fallbackErr) && fallbackErr != nil && strings.TrimPrefix(strings.TrimSpace(fallbackErr.Reason), "prewarm_") == "session_preempted"
}
