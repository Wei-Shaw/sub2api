package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// CyberSessionBlockStore 是 cyber 会话屏蔽表的存取接口。
// repository 层 gatewayCache 附带实现（类型断言探测接入，不改 GatewayCache
// 共享接口）；测试 stub 不实现时屏蔽能力自动降级关闭。
type CyberSessionBlockStore interface {
	SetCyberSessionBlocked(ctx context.Context, scopeKey string, keys []string, ttl time.Duration) error
	IsCyberSessionScopeActive(ctx context.Context, scopeKey string) (bool, error)
	FindCyberSessionBlocked(ctx context.Context, keys []string) (string, error)
}

// CyberSessionLineageStore maps downstream Responses IDs to opaque,
// API-key-isolated cyber session roots. It is optional so existing GatewayCache
// implementations and test doubles keep working without the lineage extension.
type CyberSessionLineageStore interface {
	BindCyberSessionRoot(ctx context.Context, groupID, apiKeyID int64, responseID, root string, ttl time.Duration) error
	GetCyberSessionRoot(ctx context.Context, groupID, apiKeyID int64, responseID string) (root string, found bool, err error)
}

// The Redis implementation renews all response aliases of an active root at
// a cyber hit, including earlier turns, so aliases cannot expire before blocks.
type CyberSessionLineageRefresher interface {
	RefreshCyberSessionLineage(ctx context.Context, groupID, apiKeyID int64, root string, ttl time.Duration) error
}

const cyberSessionTranscriptLookupOverflowBlockKey = "transcript_lookup_limit_exceeded"
const cyberSessionIdentityContextKey = "openai_cyber_session_identity"
const cyberSessionStoreTimeout = 500 * time.Millisecond
const maxCyberSessionObservedResponseIDs = 8

// CyberSessionIdentity is the request-local exact identity used for both
// pre-routing block lookups and response-ID lineage binding. Mutable turn state
// is guarded independently; Redis operations must never run while mu is held.
type CyberSessionIdentity struct {
	mu sync.Mutex

	gateway     *OpenAIGatewayService
	groupID     int64
	apiKeyID    int64
	explicitKey string
	lineageRoot string
	scopeKey    string
	ttl         time.Duration

	transcriptBlockKeys      []string
	transcriptLookupKeys     []string
	transcriptLookupOverflow bool
	observedResponseIDs      map[string]struct{}
	activating               bool
	activated                bool
	activationDone           chan struct{}
	blockedUntil             time.Time
}

// CyberSessionExplicitBlockKey returns an inexpensive exact key when the
// client supplies a stable session signal.
func CyberSessionExplicitBlockKey(apiKeyID int64, c *gin.Context, body []byte) string {
	return hashCyberSessionBlockKey(apiKeyID, explicitOpenAISessionID(c, body))
}

// CyberSessionTranscriptBlockKeys returns the exact full-request key followed
// by an optional rewrite-tolerant context key. The latter is emitted only after
// model-generated history has been observed.
func CyberSessionTranscriptBlockKeys(apiKeyID int64, body []byte) []string {
	derived := deriveOpenAICyberTranscriptBlockKeys(apiKeyID, body)
	if len(derived.lookupKeys) == 0 {
		return nil
	}
	keys := []string{derived.lookupKeys[len(derived.lookupKeys)-1]}
	if derived.preLatestUserKey != "" && derived.preLatestUserKey != keys[0] {
		keys = append(keys, derived.preLatestUserKey)
	}
	return keys
}

func CyberSessionTranscriptLookupKeys(apiKeyID int64, body []byte) []string {
	return deriveOpenAICyberTranscriptBlockKeys(apiKeyID, body).lookupKeys
}

// CyberSessionScopeKey is a coarse, non-blocking fingerprint used only to
// avoid transcript parsing and MGET for sources that never produced a hit.
func CyberSessionScopeKey(apiKeyID int64, clientIP, userAgent string) string {
	if apiKeyID <= 0 {
		return ""
	}
	raw := "cyber-scope:v1|api_key=" + strconv.FormatInt(apiKeyID, 10) +
		"|ip=" + strings.TrimSpace(clientIP) +
		"|ua=" + NormalizeSessionUserAgent(userAgent)
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}

func hashCyberSessionBlockKey(apiKeyID int64, raw string) string {
	if raw == "" {
		return ""
	}
	isolated := isolateOpenAISessionID(apiKeyID, raw)
	sum := sha256.Sum256([]byte(isolated))
	return hex.EncodeToString(sum[:])
}

// cyberSessionBlockStore 探测 cache 是否具备屏蔽存储能力。
// 注意：若未来以装饰器包装 GatewayCache（如日志/指标装饰器），该装饰器必须同时实现
// CyberSessionBlockStore，否则会话屏蔽能力将静默降级关闭
// （编译断言 var _ service.CyberSessionBlockStore = (*gatewayCache)(nil) 只覆盖
// *gatewayCache 本体，无法覆盖其外层包装）。
func (s *OpenAIGatewayService) cyberSessionBlockStore() CyberSessionBlockStore {
	if s == nil || s.cache == nil {
		return nil
	}
	store, ok := s.cache.(CyberSessionBlockStore)
	if !ok {
		return nil
	}
	return store
}

func (s *OpenAIGatewayService) cyberSessionLineageStore() CyberSessionLineageStore {
	if s == nil || s.cache == nil {
		return nil
	}
	store, ok := s.cache.(CyberSessionLineageStore)
	if !ok {
		return nil
	}
	return store
}

// CyberSessionBlockRuntime 返回 (开关, TTL)。开关默认关。
// 委托给 SettingService.GetCyberSessionBlockRuntime，进程内缓存避免热路径 DB 往返。
func (s *OpenAIGatewayService) CyberSessionBlockRuntime(ctx context.Context) (bool, time.Duration) {
	if s == nil || s.settingService == nil {
		return false, time.Hour
	}
	return s.settingService.GetCyberSessionBlockRuntime(ctx)
}

// MarkCyberSessionBlocked 把会话写入屏蔽表（写入点：cyber 命中后）。
// 开关关闭、key 为空或存储不可用时静默跳过。
func (s *OpenAIGatewayService) MarkCyberSessionBlocked(ctx context.Context, scopeKey string, keys []string) {
	if s == nil || len(keys) == 0 {
		return
	}
	enabled, ttl := s.CyberSessionBlockRuntime(ctx)
	if !enabled {
		return
	}
	store := s.cyberSessionBlockStore()
	if store == nil {
		return
	}
	if err := store.SetCyberSessionBlocked(ctx, scopeKey, keys, ttl); err != nil {
		logger.LegacyPrintf("service.openai_gateway", "cyber session block write failed: err=%v", err)
	}
}

// PrepareCyberSessionIdentity resolves the strongest available request
// identity. The disabled path returns before parsing the request or touching
// the optional lineage store.
func (s *OpenAIGatewayService) PrepareCyberSessionIdentity(
	ctx context.Context,
	groupID, apiKeyID int64,
	c *gin.Context,
	body []byte,
	clientIP, userAgent string,
) *CyberSessionIdentity {
	enabled, ttl := s.CyberSessionBlockRuntime(ctx)
	if !enabled || apiKeyID <= 0 {
		return nil
	}

	transcript := deriveOpenAICyberTranscriptBlockKeys(apiKeyID, body)
	transcriptBlockKeys := make([]string, 0, 2)
	if len(transcript.lookupKeys) > 0 {
		transcriptBlockKeys = append(transcriptBlockKeys, transcript.lookupKeys[len(transcript.lookupKeys)-1])
		if transcript.preLatestUserKey != "" && transcript.preLatestUserKey != transcriptBlockKeys[0] {
			transcriptBlockKeys = append(transcriptBlockKeys, transcript.preLatestUserKey)
		}
	}
	identity := &CyberSessionIdentity{
		gateway:                  s,
		groupID:                  groupID,
		apiKeyID:                 apiKeyID,
		explicitKey:              CyberSessionExplicitBlockKey(apiKeyID, c, body),
		scopeKey:                 CyberSessionScopeKey(apiKeyID, clientIP, userAgent),
		ttl:                      ttl,
		transcriptBlockKeys:      transcriptBlockKeys,
		transcriptLookupKeys:     transcript.lookupKeys,
		observedResponseIDs:      make(map[string]struct{}),
		transcriptLookupOverflow: transcript.lookupKeysTruncated,
	}
	if identity.explicitKey != "" {
		identity.lineageRoot = identity.explicitKey
		return identity
	}

	previousResponseID := strings.TrimSpace(openAIRequestPayloadView(body).Get("previous_response_id").String())
	if previousResponseID == "" {
		identity.lineageRoot = hashCyberSessionBlockKey(apiKeyID, "response-lineage-root:"+uuid.NewString())
		return identity
	}
	identity.observedResponseIDs[previousResponseID] = struct{}{}

	store := s.cyberSessionLineageStore()
	if store != nil && groupID > 0 {
		storeCtx, cancel := context.WithTimeout(ctx, cyberSessionStoreTimeout)
		defer cancel()
		root, found, err := store.GetCyberSessionRoot(storeCtx, groupID, apiKeyID, previousResponseID)
		if err != nil {
			logger.LegacyPrintf("service.openai_gateway", "cyber session lineage read failed: err=%v", err)
		} else if found && strings.TrimSpace(root) != "" {
			identity.lineageRoot = strings.TrimSpace(root)
			if err := store.BindCyberSessionRoot(storeCtx, groupID, apiKeyID, previousResponseID, identity.lineageRoot, ttl); err != nil {
				logger.LegacyPrintf("service.openai_gateway", "cyber session lineage refresh failed: err=%v", err)
			}
			return identity
		}
	}

	identity.lineageRoot = hashCyberSessionBlockKey(apiKeyID, "response-lineage:"+previousResponseID)
	return identity
}

// BindCyberSessionIdentity installs the prepared identity on the request.
func BindCyberSessionIdentity(c *gin.Context, identity *CyberSessionIdentity) {
	if c == nil {
		return
	}
	c.Set(cyberSessionIdentityContextKey, identity)
}

func getCyberSessionIdentity(c *gin.Context) *CyberSessionIdentity {
	if c == nil {
		return nil
	}
	value, ok := c.Get(cyberSessionIdentityContextKey)
	if !ok {
		return nil
	}
	identity, _ := value.(*CyberSessionIdentity)
	return identity
}

func cyberSessionRequestContext(c *gin.Context) context.Context {
	if c != nil && c.Request != nil {
		return c.Request.Context()
	}
	return context.Background()
}

// ObserveCyberSessionResponseID persists a response-to-root binding before the
// corresponding response event is made visible. Duplicate observations within
// a turn do not repeat Redis writes.
func ObserveCyberSessionResponseID(c *gin.Context, responseID string) {
	identity := getCyberSessionIdentity(c)
	responseID = strings.TrimSpace(responseID)
	if identity == nil || responseID == "" {
		return
	}

	identity.mu.Lock()
	if _, exists := identity.observedResponseIDs[responseID]; exists {
		identity.mu.Unlock()
		return
	}
	if len(identity.observedResponseIDs) >= maxCyberSessionObservedResponseIDs {
		identity.mu.Unlock()
		return
	}
	identity.observedResponseIDs[responseID] = struct{}{}
	gateway, groupID, apiKeyID := identity.gateway, identity.groupID, identity.apiKeyID
	root, ttl := identity.lineageRoot, identity.ttl
	identity.mu.Unlock()

	if gateway == nil || groupID <= 0 || apiKeyID <= 0 || root == "" {
		return
	}
	store := gateway.cyberSessionLineageStore()
	if store == nil {
		return
	}
	storeCtx, cancel := context.WithTimeout(cyberSessionRequestContext(c), cyberSessionStoreTimeout)
	defer cancel()
	if err := store.BindCyberSessionRoot(storeCtx, groupID, apiKeyID, responseID, root, ttl); err != nil {
		logger.LegacyPrintf("service.openai_gateway", "cyber session lineage bind failed: err=%v", err)
		return
	}
}

// CyberSessionIdentityBlockWritePlan exposes the exact request-local keys for
// the handler's post-forward idempotent fallback write.
func CyberSessionIdentityBlockWritePlan(c *gin.Context) (scopeKey string, keys []string) {
	identity := getCyberSessionIdentity(c)
	if identity == nil {
		return "", nil
	}
	identity.mu.Lock()
	defer identity.mu.Unlock()
	keys = appendCyberSessionUniqueKey(keys, identity.lineageRoot)
	keys = appendCyberSessionUniqueKey(keys, identity.explicitKey)
	for _, key := range identity.transcriptBlockKeys {
		keys = appendCyberSessionUniqueKey(keys, key)
	}
	if len(identity.transcriptBlockKeys) > 0 {
		scopeKey = identity.scopeKey
	}
	return scopeKey, keys
}

func appendCyberSessionUniqueKey(keys []string, key string) []string {
	key = strings.TrimSpace(key)
	if key == "" {
		return keys
	}
	for _, existing := range keys {
		if existing == key {
			return keys
		}
	}
	return append(keys, key)
}

func activateCyberSessionIdentity(c *gin.Context) {
	identity := getCyberSessionIdentity(c)
	if identity == nil {
		return
	}
	enabled, configuredTTL := identity.gateway.CyberSessionBlockRuntime(cyberSessionRequestContext(c))
	if !enabled {
		return
	}

	identity.mu.Lock()
	if identity.activated {
		identity.mu.Unlock()
		return
	}
	if identity.activating {
		done := identity.activationDone
		identity.mu.Unlock()
		<-done
		return
	}
	identity.activating = true
	identity.activationDone = make(chan struct{})
	gateway, groupID, apiKeyID := identity.gateway, identity.groupID, identity.apiKeyID
	if identity.blockedUntil.IsZero() {
		identity.ttl = configuredTTL
		hitAt := time.Now()
		if mark := GetOpsCyberPolicy(c); mark != nil && !mark.hitAt.IsZero() {
			hitAt = mark.hitAt
		}
		identity.blockedUntil = hitAt.Add(configuredTTL)
	}
	root, deadline := identity.lineageRoot, identity.blockedUntil
	ttl := time.Until(deadline)
	responseIDs := make([]string, 0, len(identity.observedResponseIDs))
	for responseID := range identity.observedResponseIDs {
		responseIDs = append(responseIDs, responseID)
	}
	identity.mu.Unlock()

	ctx, cancel := context.WithTimeout(context.WithoutCancel(cyberSessionRequestContext(c)), cyberSessionStoreTimeout)
	defer cancel()
	if ttl > 0 && gateway != nil && groupID > 0 && apiKeyID > 0 && root != "" {
		if store := gateway.cyberSessionLineageStore(); store != nil {
			for _, responseID := range responseIDs {
				if err := store.BindCyberSessionRoot(ctx, groupID, apiKeyID, responseID, root, ttl); err != nil {
					logger.LegacyPrintf("service.openai_gateway", "cyber session lineage activation bind failed: err=%v", err)
				}
			}
		}
		if refresher, ok := gateway.cache.(CyberSessionLineageRefresher); ok {
			if err := refresher.RefreshCyberSessionLineage(ctx, groupID, apiKeyID, root, ttl); err != nil {
				logger.LegacyPrintf("service.openai_gateway", "cyber session lineage refresh failed: err=%v", err)
			}
		}
	}
	scopeKey, keys := CyberSessionIdentityBlockWritePlan(c)
	ttl = time.Until(deadline)
	succeeded := ttl <= 0
	if ttl > 0 && gateway != nil {
		if store := gateway.cyberSessionBlockStore(); store != nil {
			if err := store.SetCyberSessionBlocked(ctx, scopeKey, keys, ttl); err != nil {
				logger.LegacyPrintf("service.openai_gateway", "cyber session block write failed: err=%v", err)
			} else {
				succeeded = true
			}
		}
	}

	identity.mu.Lock()
	identity.activated = succeeded
	identity.activating = false
	close(identity.activationDone)
	identity.mu.Unlock()
}

// RetryCyberSessionBlock reuses the first hit's deadline. Returning true means
// an identity owns this turn; the handler must not start a fresh TTL itself.
func RetryCyberSessionBlock(c *gin.Context) bool {
	if getCyberSessionIdentity(c) == nil {
		return false
	}
	activateCyberSessionIdentity(c)
	return true
}

// CyberSessionBlockStillActive bounds the WS connection's defensive flag by
// the same configured deadline as the distributed session block.
func CyberSessionBlockStillActive(c *gin.Context) bool {
	identity := getCyberSessionIdentity(c)
	if identity == nil {
		return false
	}
	if enabled, _ := identity.gateway.CyberSessionBlockRuntime(cyberSessionRequestContext(c)); !enabled {
		return false
	}
	identity.mu.Lock()
	defer identity.mu.Unlock()
	return time.Now().Before(identity.blockedUntil)
}

// FindCyberSessionBlockedForRequest applies explicit-first lookup followed by
// scope-gated transcript matching. All failures remain fail-open.
func (s *OpenAIGatewayService) FindCyberSessionBlockedForRequest(ctx context.Context, apiKeyID int64, c *gin.Context, body []byte, clientIP, userAgent string) string {
	enabled, _ := s.CyberSessionBlockRuntime(ctx)
	if !enabled {
		return ""
	}
	store := s.cyberSessionBlockStore()
	if store == nil {
		return ""
	}
	if identity := getCyberSessionIdentity(c); identity != nil && identity.gateway == s && identity.apiKeyID == apiKeyID {
		return s.findBoundCyberSessionBlocked(ctx, store, identity)
	}
	if explicitKey := CyberSessionExplicitBlockKey(apiKeyID, c, body); explicitKey != "" {
		key, err := store.FindCyberSessionBlocked(ctx, []string{explicitKey})
		if err != nil {
			logger.LegacyPrintf("service.openai_gateway", "cyber explicit session read failed: err=%v", err)
			return ""
		}
		if key != "" {
			return key
		}
	}
	scopeKey := CyberSessionScopeKey(apiKeyID, clientIP, userAgent)
	active, err := store.IsCyberSessionScopeActive(ctx, scopeKey)
	if err != nil {
		logger.LegacyPrintf("service.openai_gateway", "cyber session scope read failed: err=%v", err)
		return ""
	}
	if !active {
		return ""
	}
	transcript := deriveOpenAICyberTranscriptBlockKeys(apiKeyID, body)
	if transcript.lookupKeysTruncated {
		// Once the coarse scope is active, silently dropping old candidates would
		// let a blocked client evade prefix matching by appending dummy items.
		return cyberSessionTranscriptLookupOverflowBlockKey
	}
	keys := transcript.lookupKeys
	if len(keys) == 0 {
		return ""
	}
	key, err := store.FindCyberSessionBlocked(ctx, keys)
	if err != nil {
		logger.LegacyPrintf("service.openai_gateway", "cyber session block batch read failed: err=%v", err)
		return ""
	}
	return key
}

func (s *OpenAIGatewayService) findBoundCyberSessionBlocked(ctx context.Context, store CyberSessionBlockStore, identity *CyberSessionIdentity) string {
	if identity.explicitKey != "" {
		key, err := store.FindCyberSessionBlocked(ctx, []string{identity.explicitKey})
		if err != nil {
			logger.LegacyPrintf("service.openai_gateway", "cyber explicit session read failed: err=%v", err)
			return ""
		}
		if key != "" {
			return key
		}
	}
	if identity.lineageRoot != "" && identity.lineageRoot != identity.explicitKey {
		key, err := store.FindCyberSessionBlocked(ctx, []string{identity.lineageRoot})
		if err != nil {
			logger.LegacyPrintf("service.openai_gateway", "cyber session lineage block read failed: err=%v", err)
			return ""
		}
		if key != "" {
			return key
		}
	}
	active, err := store.IsCyberSessionScopeActive(ctx, identity.scopeKey)
	if err != nil {
		logger.LegacyPrintf("service.openai_gateway", "cyber session scope read failed: err=%v", err)
		return ""
	}
	if !active {
		return ""
	}
	if identity.transcriptLookupOverflow {
		return cyberSessionTranscriptLookupOverflowBlockKey
	}
	if len(identity.transcriptLookupKeys) == 0 {
		return ""
	}
	key, err := store.FindCyberSessionBlocked(ctx, identity.transcriptLookupKeys)
	if err != nil {
		logger.LegacyPrintf("service.openai_gateway", "cyber session block batch read failed: err=%v", err)
		return ""
	}
	return key
}
