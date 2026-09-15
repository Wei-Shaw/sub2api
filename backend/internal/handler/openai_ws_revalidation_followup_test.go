package handler

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	middleware2 "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/Wei-Shaw/sub2api/internal/testutil"
	coderws "github.com/coder/websocket"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

// wsRevalidationKeyRepoStub is a mutable API key repository: tests replace the
// current key between turns to model admin changes while a WS connection is open.
type wsRevalidationKeyRepoStub struct {
	service.APIKeyRepository

	mu      sync.Mutex
	current *service.APIKey
	err     error
}

func (r *wsRevalidationKeyRepoStub) set(key *service.APIKey) {
	r.mu.Lock()
	r.current = key
	r.err = nil
	r.mu.Unlock()
}

func (r *wsRevalidationKeyRepoStub) setError(err error) {
	r.mu.Lock()
	r.err = err
	r.mu.Unlock()
}

func (r *wsRevalidationKeyRepoStub) GetByKey(context.Context, string) (*service.APIKey, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.err != nil {
		return nil, r.err
	}
	if r.current == nil {
		return nil, service.ErrAPIKeyNotFound
	}
	clone := *r.current
	return &clone, nil
}

func (r *wsRevalidationKeyRepoStub) GetByKeyForAuth(ctx context.Context, key string) (*service.APIKey, error) {
	return r.GetByKey(ctx, key)
}

func (r *wsRevalidationKeyRepoStub) UpdateLastUsed(context.Context, int64, time.Time) error {
	return nil
}

// wsRevalidationKey builds a fresh key snapshot safe to publish on the stub.
func wsRevalidationKey(limit int, allowlistEnabled bool, models []string, allowImage bool) *service.APIKey {
	group := &service.Group{
		ID:                   4201,
		Platform:             service.PlatformOpenAI,
		Status:               service.StatusActive,
		Hydrated:             true,
		AllowImageGeneration: allowImage,
		ModelAllowlist: service.GroupModelAllowlist{
			Enabled: allowlistEnabled,
			Models:  models,
		},
	}
	user := &service.User{ID: 1701, Status: service.StatusActive, Concurrency: 3}
	key := &service.APIKey{
		ID:               1801,
		UserID:           user.ID,
		Key:              "sk-ws-revalidation-followup",
		Status:           service.StatusActive,
		ConcurrencyLimit: limit,
		User:             user,
		Group:            group,
	}
	key.GroupID = &group.ID
	return key
}

// wsRevalidationUpstream accepts one passthrough upstream connection, records
// every accepted turn frame and answers each with response.completed.
type wsRevalidationUpstream struct {
	mu     sync.Mutex
	frames [][]byte

	// afterFrame runs after a frame is recorded; tests use it to hold a turn
	// open while asserting Redis membership.
	afterFrame func(frame int, payload []byte)
}

func (u *wsRevalidationUpstream) record(payload []byte) {
	u.mu.Lock()
	u.frames = append(u.frames, append([]byte(nil), payload...))
	u.mu.Unlock()
}

func (u *wsRevalidationUpstream) frameCount() int {
	u.mu.Lock()
	defer u.mu.Unlock()
	return len(u.frames)
}

type wsRevalidationHarness struct {
	t           *testing.T
	conn        *coderws.Conn
	repo        *wsRevalidationKeyRepoStub
	concurrency *service.ConcurrencyService
	queueCache  service.APIKeySlotQueueCache
	upstream    *wsRevalidationUpstream
	apiKeyID    int64
	handlerDone chan struct{}
}

// newWSRevalidationHarness mounts the real API key auth middleware, the real
// ResponsesWebSocket handler and a real Redis-backed key queue (miniredis).
func newWSRevalidationHarness(t *testing.T, initial *service.APIKey, policy service.APIKeyQueuePolicy, ingressMode string) *wsRevalidationHarness {
	t.Helper()
	gin.SetMode(gin.TestMode)

	upstream := &wsRevalidationUpstream{}
	upstreamServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, acceptErr := coderws.Accept(w, r, &coderws.AcceptOptions{CompressionMode: coderws.CompressionContextTakeover})
		if acceptErr != nil {
			return
		}
		defer func() { _ = conn.CloseNow() }()

		for frame := 1; frame <= 4; frame++ {
			readCtx, cancelRead := context.WithTimeout(r.Context(), 5*time.Second)
			_, payload, readErr := conn.Read(readCtx)
			cancelRead()
			if readErr != nil {
				return
			}
			upstream.record(payload)
			if upstream.afterFrame != nil {
				upstream.afterFrame(frame, payload)
			}
			response := `{"type":"response.completed","response":{"id":"resp_ws_revalidation_` + strconv.Itoa(frame) + `","model":"` + gjson.GetBytes(payload, "model").String() + `","usage":{"input_tokens":2,"output_tokens":1}}}`
			writeCtx, cancelWrite := context.WithTimeout(r.Context(), 5*time.Second)
			writeErr := conn.Write(writeCtx, coderws.MessageText, []byte(response))
			cancelWrite()
			if writeErr != nil {
				return
			}
		}
	}))
	t.Cleanup(upstreamServer.Close)

	account := service.Account{
		ID:          9901,
		Name:        "openai-ws-revalidation-followup",
		Platform:    service.PlatformOpenAI,
		Type:        service.AccountTypeAPIKey,
		Status:      service.StatusActive,
		Schedulable: true,
		Concurrency: 4,
		Credentials: map[string]any{"api_key": "sk-test", "base_url": upstreamServer.URL},
		Extra: map[string]any{
			"openai_apikey_responses_websockets_v2_enabled": true,
			"openai_apikey_responses_websockets_v2_mode":    ingressMode,
		},
	}
	cfg := &config.Config{}
	cfg.RunMode = config.RunModeSimple
	cfg.Default.RateMultiplier = 1
	cfg.Security.URLAllowlist.Enabled = false
	cfg.Security.URLAllowlist.AllowInsecureHTTP = true
	cfg.Gateway.OpenAIWS.Enabled = true
	cfg.Gateway.OpenAIWS.APIKeyEnabled = true
	cfg.Gateway.OpenAIWS.ResponsesWebsocketsV2 = true
	cfg.Gateway.OpenAIWS.ModeRouterV2Enabled = true
	cfg.Gateway.OpenAIWS.DialTimeoutSeconds = 3
	cfg.Gateway.OpenAIWS.ReadTimeoutSeconds = 5
	cfg.Gateway.OpenAIWS.WriteTimeoutSeconds = 5
	cfg.Gateway.OpenAIWS.IngressInterTurnIdleTimeoutSeconds = 5

	repo := &wsRevalidationKeyRepoStub{current: initial}
	apiKeyService := service.NewAPIKeyService(repo, nil, nil, nil, nil, nil, cfg)
	billingCacheSvc := service.NewBillingCacheService(nil, nil, nil, nil, nil, nil, cfg, nil)
	gatewaySvc := service.NewOpenAIGatewayService(
		&openAIWSUsageHandlerAccountRepoStub{account: account},
		&openAIWSUsageHandlerUsageLogRepoStub{created: make(chan *service.UsageLog, 4)},
		nil, nil, nil, nil, nil,
		cfg,
		nil, nil,
		service.NewBillingService(cfg, nil),
		nil,
		billingCacheSvc,
		nil,
		&service.DeferredService{},
		nil, nil, nil, nil, nil, nil, nil,
	)
	rawCache := testutil.NewRedisConcurrencyCache(t)
	queueCache, ok := rawCache.(service.APIKeySlotQueueCache)
	require.True(t, ok)
	concurrencyService := service.NewConcurrencyService(rawCache)
	concurrencyService.SetAPIKeyQueuePolicy(policy)

	h := &OpenAIGatewayHandler{
		gatewayService:      gatewaySvc,
		billingCacheService: billingCacheSvc,
		apiKeyService:       apiKeyService,
		concurrencyHelper:   NewConcurrencyHelper(concurrencyService, SSEPingFormatNone, time.Second),
	}

	handlerDone := make(chan struct{})
	router := gin.New()
	router.Use(gin.HandlerFunc(middleware2.NewAPIKeyAuthMiddleware(apiKeyService, nil, cfg)))
	router.GET("/v1/responses", func(c *gin.Context) {
		h.ResponsesWebSocket(c)
		close(handlerDone)
	})
	handlerServer := httptest.NewServer(router)
	t.Cleanup(handlerServer.Close)

	dialCtx, cancelDial := context.WithTimeout(context.Background(), 5*time.Second)
	conn, _, dialErr := coderws.Dial(dialCtx, "ws"+strings.TrimPrefix(handlerServer.URL, "http")+"/v1/responses", &coderws.DialOptions{
		HTTPHeader: http.Header{"X-Api-Key": []string{initial.Key}},
	})
	cancelDial()
	require.NoError(t, dialErr)
	t.Cleanup(func() { _ = conn.CloseNow() })

	return &wsRevalidationHarness{
		t:           t,
		conn:        conn,
		repo:        repo,
		concurrency: concurrencyService,
		queueCache:  queueCache,
		upstream:    upstream,
		apiKeyID:    initial.ID,
		handlerDone: handlerDone,
	}
}

func (h *wsRevalidationHarness) sendTurn(payload string) {
	h.t.Helper()
	writeCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	require.NoError(h.t, h.conn.Write(writeCtx, coderws.MessageText, []byte(payload)))
}

// readTurn reads one server frame: a completion event or a close.
func (h *wsRevalidationHarness) readTurn(timeout time.Duration) (event []byte, closeCode coderws.StatusCode, reason string) {
	h.t.Helper()
	readCtx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	_, data, err := h.conn.Read(readCtx)
	if err != nil {
		var closeErr coderws.CloseError
		if errors.As(err, &closeErr) {
			return nil, closeErr.Code, closeErr.Reason
		}
		return nil, 0, err.Error()
	}
	return data, 0, ""
}

func (h *wsRevalidationHarness) requireCompleted(timeout time.Duration) {
	h.t.Helper()
	event, closeCode, reason := h.readTurn(timeout)
	require.Equal(h.t, "response.completed", gjson.GetBytes(event, "type").String(), "close code=%d reason=%q", closeCode, reason)
}

func (h *wsRevalidationHarness) requireClosed(want coderws.StatusCode, reasonContains string) {
	h.t.Helper()
	event, closeCode, reason := h.readTurn(5 * time.Second)
	require.Nil(h.t, event, "expected a close, got event %s", string(event))
	require.Equal(h.t, want, closeCode, "reason=%q", reason)
	require.Contains(h.t, reason, reasonContains)
}

func (h *wsRevalidationHarness) queueStats() (active int, waiting int) {
	h.t.Helper()
	active, waiting, err := h.queueCache.GetAPIKeyQueueStats(context.Background(), h.apiKeyID)
	require.NoError(h.t, err)
	return active, waiting
}

func (h *wsRevalidationHarness) requireQueueWaiting(want int) {
	h.t.Helper()
	require.Eventually(h.t, func() bool {
		_, waiting := h.queueStats()
		return waiting == want
	}, 3*time.Second, 10*time.Millisecond, "queue waiting count")
}

// TestOpenAIWSRevalidationFollowupAllowsExpandedAllowlist reproduces the stale
// handshake allowlist bug: while the connection is open the admin adds model B;
// the next response.create for B must be admitted in every capacity mode.
func TestOpenAIWSRevalidationFollowupAllowsExpandedAllowlist(t *testing.T) {
	for _, tc := range []struct {
		name   string
		limit  int
		policy service.APIKeyQueuePolicy
	}{
		{name: "unlimited", limit: 0, policy: service.APIKeyQueuePolicy{MaxWaiting: 4, Timeout: 5 * time.Second}},
		{name: "queue disabled", limit: 1, policy: service.APIKeyQueuePolicy{MaxWaiting: 0}},
		{name: "queue enabled", limit: 1, policy: service.APIKeyQueuePolicy{MaxWaiting: 4, Timeout: 5 * time.Second}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := newWSRevalidationHarness(t, wsRevalidationKey(tc.limit, true, []string{"gpt-5.4"}, true), tc.policy, service.OpenAIWSIngressModePassthrough)

			h.sendTurn(`{"type":"response.create","model":"gpt-5.4","stream":false}`)
			h.requireCompleted(5 * time.Second)

			// Handshake snapshot would still reject gpt-5.1; latest permission allows it.
			h.repo.set(wsRevalidationKey(tc.limit, true, []string{"gpt-5.4", "gpt-5.1"}, true))
			h.sendTurn(`{"type":"response.create","model":"gpt-5.1","stream":false}`)
			h.requireCompleted(5 * time.Second)
			require.Equal(t, 2, h.upstream.frameCount(), "the expanded-model turn must reach upstream")

			// Passthrough has no pre-parse hook. Its third and later turns must
			// replace the previous turn's model candidates, not keep accumulating.
			h.repo.set(wsRevalidationKey(tc.limit, true, []string{"gpt-5.4"}, true))
			h.sendTurn(`{"type":"response.create","model":"gpt-5.4","stream":false}`)
			h.requireCompleted(5 * time.Second)
			require.Equal(t, 3, h.upstream.frameCount(), "the third turn must use its own allowed model")
			h.sendTurn(`{"type":"response.create","model":"gpt-5.1","stream":false}`)
			h.requireClosed(coderws.StatusPolicyViolation, "not available for this group")
			require.Equal(t, 3, h.upstream.frameCount(), "the fourth turn's revoked model must not reach upstream")
		})
	}
}

// TestOpenAIWSRevalidationFollowupRevokesModelWhenUnlimited proves the fresh
// per-turn gate exists even when the key is unlimited (initial limit 0), so a
// revoked model is still rejected with 1008 and never reaches upstream.
func TestOpenAIWSRevalidationFollowupRevokesModelWhenUnlimited(t *testing.T) {
	h := newWSRevalidationHarness(t, wsRevalidationKey(0, true, []string{"gpt-5.4", "gpt-5.1"}, true), service.APIKeyQueuePolicy{MaxWaiting: 4, Timeout: 5 * time.Second}, service.OpenAIWSIngressModePassthrough)

	h.sendTurn(`{"type":"response.create","model":"gpt-5.4","stream":false}`)
	h.requireCompleted(5 * time.Second)

	h.repo.set(wsRevalidationKey(0, true, []string{"gpt-5.4"}, true))
	h.sendTurn(`{"type":"response.create","model":"gpt-5.1","stream":false}`)
	h.requireClosed(coderws.StatusPolicyViolation, "not available for this group")
	require.Equal(t, 1, h.upstream.frameCount(), "a revoked model must not reach upstream")
}

// TestOpenAIWSRevalidationFollowupRevokesKeyAndImagePermission covers the
// queue-disabled mode: a disabled key and a revoked image permission both stop
// the next turn with 1008 before any upstream work.
func TestOpenAIWSRevalidationFollowupRevokesKeyAndImagePermission(t *testing.T) {
	t.Run("key disabled", func(t *testing.T) {
		h := newWSRevalidationHarness(t, wsRevalidationKey(1, false, nil, true), service.APIKeyQueuePolicy{MaxWaiting: 0}, service.OpenAIWSIngressModePassthrough)

		h.sendTurn(`{"type":"response.create","model":"gpt-5.4","stream":false}`)
		h.requireCompleted(5 * time.Second)

		disabled := wsRevalidationKey(1, false, nil, true)
		disabled.Status = service.StatusDisabled
		h.repo.set(disabled)
		h.sendTurn(`{"type":"response.create","model":"gpt-5.4","stream":false}`)
		h.requireClosed(coderws.StatusPolicyViolation, "disabled")
		require.Equal(t, 1, h.upstream.frameCount())
	})

	t.Run("image permission revoked", func(t *testing.T) {
		h := newWSRevalidationHarness(t, wsRevalidationKey(1, false, nil, true), service.APIKeyQueuePolicy{MaxWaiting: 0}, service.OpenAIWSIngressModePassthrough)

		h.sendTurn(`{"type":"response.create","model":"gpt-5.4","stream":false}`)
		h.requireCompleted(5 * time.Second)

		h.repo.set(wsRevalidationKey(1, false, nil, false))
		h.sendTurn(`{"type":"response.create","model":"gpt-5.4","tools":[{"type":"image_generation"}],"stream":false}`)
		h.requireClosed(coderws.StatusPolicyViolation, "Image generation is not enabled")
		require.Equal(t, 1, h.upstream.frameCount())
	})
}

// TestOpenAIWSRevalidationFollowupCoreChangeIsRetryable covers the queued mode:
// a genuine binding change closes retryable with 1013 and the configuration
// message, distinct from a temporary authentication read failure.
func TestOpenAIWSRevalidationFollowupCoreChangeIsRetryable(t *testing.T) {
	h := newWSRevalidationHarness(t, wsRevalidationKey(1, false, nil, true), service.APIKeyQueuePolicy{MaxWaiting: 4, Timeout: 5 * time.Second}, service.OpenAIWSIngressModePassthrough)

	h.sendTurn(`{"type":"response.create","model":"gpt-5.4","stream":false}`)
	h.requireCompleted(5 * time.Second)

	changed := wsRevalidationKey(1, false, nil, true)
	changed.Group.Platform = service.PlatformGrok
	h.repo.set(changed)
	h.sendTurn(`{"type":"response.create","model":"gpt-5.4","stream":false}`)
	h.requireClosed(coderws.StatusTryAgainLater, "configuration changed")
	require.Equal(t, 1, h.upstream.frameCount())
}

// TestOpenAIWSRevalidationFollowupAuthReadFailureStaysTemporary proves the
// other retryable 5xx flavor: a temporary authentication read failure closes
// with 1013 and the service-unavailable message, not the configuration text.
func TestOpenAIWSRevalidationFollowupAuthReadFailureStaysTemporary(t *testing.T) {
	h := newWSRevalidationHarness(t, wsRevalidationKey(1, false, nil, true), service.APIKeyQueuePolicy{MaxWaiting: 4, Timeout: 5 * time.Second}, service.OpenAIWSIngressModePassthrough)

	h.sendTurn(`{"type":"response.create","model":"gpt-5.4","stream":false}`)
	h.requireCompleted(5 * time.Second)

	h.repo.setError(errors.New("auth cache unavailable"))
	h.sendTurn(`{"type":"response.create","model":"gpt-5.4","stream":false}`)
	h.requireClosed(coderws.StatusTryAgainLater, "temporarily unavailable")
	require.Equal(t, 1, h.upstream.frameCount())
}

// TestOpenAIWSRevalidationNativeImagePermissionTiming reproduces the stale
// parser timing gap on the native/bridge shared parser: after a text first
// turn, relaxing the image permission must let the next explicit
// image_generation turn reach the mock upstream (not die in the parser on the
// previous turn's flag), revoking it must stop the frame before upstream, and a
// new model must be validated as itself rather than the previous turn's model.
func TestOpenAIWSRevalidationNativeImagePermissionTiming(t *testing.T) {
	policy := service.APIKeyQueuePolicy{MaxWaiting: 4, Timeout: 5 * time.Second}
	textTurn := `{"type":"response.create","model":"gpt-5.4","stream":false}`
	imageTurn := `{"type":"response.create","model":"gpt-5.4","tools":[{"type":"image_generation"}],"stream":false}`
	for _, mode := range []string{service.OpenAIWSIngressModeCtxPool, service.OpenAIWSIngressModeDedicated} {
		t.Run(mode, func(t *testing.T) {
			t.Run("relaxed image permission reaches upstream", func(t *testing.T) {
				h := newWSRevalidationHarness(t, wsRevalidationKey(1, false, nil, false), policy, mode)
				h.sendTurn(textTurn)
				h.requireCompleted(5 * time.Second)

				h.repo.set(wsRevalidationKey(1, false, nil, true))
				h.sendTurn(imageTurn)
				h.requireCompleted(5 * time.Second)
				require.Equal(t, 2, h.upstream.frameCount(), "relaxed permission must not be rejected from the stale parse flag")
			})

			t.Run("revoked image permission does not reach upstream", func(t *testing.T) {
				h := newWSRevalidationHarness(t, wsRevalidationKey(1, false, nil, true), policy, mode)
				h.sendTurn(textTurn)
				h.requireCompleted(5 * time.Second)

				h.repo.set(wsRevalidationKey(1, false, nil, false))
				h.sendTurn(imageTurn)
				h.requireClosed(coderws.StatusPolicyViolation, "Image generation is not enabled")
				require.Equal(t, 1, h.upstream.frameCount(), "revoked permission must stop the frame before upstream")
			})

			t.Run("current model validated, not the previous one", func(t *testing.T) {
				h := newWSRevalidationHarness(t, wsRevalidationKey(1, true, []string{"gpt-5.4"}, true), policy, mode)
				h.sendTurn(textTurn)
				h.requireCompleted(5 * time.Second)

				// The allowlist never changed; model B must be judged as itself.
				h.sendTurn(`{"type":"response.create","model":"gpt-5.1","stream":false}`)
				h.requireClosed(coderws.StatusPolicyViolation, "not available for this group")
				require.Equal(t, 1, h.upstream.frameCount())
			})
		})
	}
}

// TestOpenAIWSRevalidationFollowupLimitRisesAndFalls covers the capacity
// lifecycle: 0 -> positive acquires an enforced lease through the installed
// control owner (waiting behind a real holder, never 503), and positive -> 0
// releases enforcement so a held slot no longer blocks the next turn.
func TestOpenAIWSRevalidationFollowupLimitRisesAndFalls(t *testing.T) {
	h := newWSRevalidationHarness(t, wsRevalidationKey(0, false, nil, true), service.APIKeyQueuePolicy{MaxWaiting: 4, Timeout: 5 * time.Second}, service.OpenAIWSIngressModePassthrough)

	h.sendTurn(`{"type":"response.create","model":"gpt-5.4","stream":false}`)
	h.requireCompleted(5 * time.Second)

	// 0 -> positive: hold the only enforced slot; the next turn must wait and
	// then acquire it with a legitimate control owner instead of failing 503.
	h.repo.set(wsRevalidationKey(1, false, nil, true))
	holderCtx, cancelHolder := service.WithAPIKeyAdmissionOwner(context.Background())
	defer cancelHolder()
	holder, err := h.concurrency.ReserveAPIKeySlotWithWait(holderCtx, h.apiKeyID, 1)
	require.NoError(t, err)
	h.sendTurn(`{"type":"response.create","model":"gpt-5.4","stream":false}`)
	h.requireQueueWaiting(1)
	holder.Release()
	h.requireCompleted(5 * time.Second)
	require.Equal(t, 2, h.upstream.frameCount())

	// positive -> 0: a fresh enforced slot must no longer block the unlimited turn.
	h.repo.set(wsRevalidationKey(0, false, nil, true))
	secondHolderCtx, cancelSecondHolder := service.WithAPIKeyAdmissionOwner(context.Background())
	defer cancelSecondHolder()
	secondHolder, err := h.concurrency.ReserveAPIKeySlotWithWait(secondHolderCtx, h.apiKeyID, 1)
	require.NoError(t, err)
	h.sendTurn(`{"type":"response.create","model":"gpt-5.4","stream":false}`)
	h.requireCompleted(5 * time.Second)
	require.Equal(t, 3, h.upstream.frameCount())
	secondHolder.Release()

	// Closing the connection leaves no waiting ticket or enforced member behind.
	require.NoError(t, h.conn.CloseNow())
	select {
	case <-h.handlerDone:
	case <-time.After(5 * time.Second):
		t.Fatal("websocket handler did not exit after client close")
	}
	require.Eventually(t, func() bool {
		active, waiting := h.queueStats()
		return active == 0 && waiting == 0
	}, 3*time.Second, 10*time.Millisecond, "queue membership must be cleaned up")
}
