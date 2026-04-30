package service

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"math/rand"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/apicompat"
	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	"github.com/Wei-Shaw/sub2api/internal/pkg/openai"
	"github.com/Wei-Shaw/sub2api/internal/util/responseheaders"
	"github.com/Wei-Shaw/sub2api/internal/util/urlvalidator"
	"github.com/cespare/xxhash/v2"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
	"go.uber.org/zap"
)

const (
	// ChatGPT internal API for OAuth accounts
	chatgptCodexURL = "https://chatgpt.com/backend-api/codex/responses"
	// OpenAI Platform API for API Key accounts (fallback)
	openaiPlatformAPIURL   = "https://api.openai.com/v1/responses"
	openaiStickySessionTTL = time.Hour // 粘性会话TTL
	codexCLIUserAgent      = "codex_cli_rs/0.125.0"
	// codex_cli_only 拒绝时单个请求头日志长度上限（字符）
	codexCLIOnlyHeaderValueMaxBytes = 256

	// OpenAIParsedRequestBodyKey 缓存 handler 侧已解析的请求体，避免重复解析。
	OpenAIParsedRequestBodyKey = "openai_parsed_request_body"
	// OpenAISysToolContinuationKey 标记当前请求需要为 -Sys 路由补最小 tool continuation。
	OpenAISysToolContinuationKey = "openai_sys_tool_continuation"
	// OpenAI WS Mode 失败后的重连次数上限（不含首次尝试）。
	// 与 Codex 客户端保持一致：失败后最多重连 5 次。
	openAIWSReconnectRetryLimit = 5
	// OpenAI WS Mode 重连退避默认值（可由配置覆盖）。
	openAIWSRetryBackoffInitialDefault = 120 * time.Millisecond
	openAIWSRetryBackoffMaxDefault     = 2 * time.Second
	openAIWSRetryJitterRatioDefault    = 0.2
	openAICompactSessionSeedKey        = "openai_compact_session_seed"
	codexCLIVersion                    = "0.125.0"
	// Codex 限额快照仅用于后台展示/诊断，不需要每个成功请求都立即落库。
	openAICodexSnapshotPersistMinInterval = 30 * time.Second
	openAIUnknownModelRefreshRequestTTL   = 5 * time.Minute
)

const openAIUnknownModelRefreshRequestNamespace = "openai_unknown_model_refresh_request"

// OpenAI allowed headers whitelist (for non-passthrough).
var openaiAllowedHeaders = map[string]bool{
	"accept-language":       true,
	"content-type":          true,
	"conversation_id":       true,
	"user-agent":            true,
	"originator":            true,
	"session_id":            true,
	"x-codex-turn-state":    true,
	"x-codex-turn-metadata": true,
}

// OpenAI passthrough allowed headers whitelist.
// 透传模式下仅放行这些低风险请求头，避免将非标准/环境噪声头传给上游触发风控。
var openaiPassthroughAllowedHeaders = map[string]bool{
	"accept":                true,
	"accept-language":       true,
	"content-type":          true,
	"conversation_id":       true,
	"openai-beta":           true,
	"user-agent":            true,
	"originator":            true,
	"session_id":            true,
	"x-codex-turn-state":    true,
	"x-codex-turn-metadata": true,
}

// codex_cli_only 拒绝时记录的请求头白名单（仅用于诊断日志，不参与上游透传）
var codexCLIOnlyDebugHeaderWhitelist = []string{
	"User-Agent",
	"Content-Type",
	"Accept",
	"Accept-Language",
	"OpenAI-Beta",
	"Originator",
	"Session_ID",
	"Conversation_ID",
	"X-Request-ID",
	"X-Client-Request-ID",
	"X-Forwarded-For",
	"X-Real-IP",
}

// OpenAICodexUsageSnapshot represents Codex API usage limits from response headers
type OpenAICodexUsageSnapshot struct {
	PrimaryUsedPercent          *float64 `json:"primary_used_percent,omitempty"`
	PrimaryResetAfterSeconds    *int     `json:"primary_reset_after_seconds,omitempty"`
	PrimaryWindowMinutes        *int     `json:"primary_window_minutes,omitempty"`
	SecondaryUsedPercent        *float64 `json:"secondary_used_percent,omitempty"`
	SecondaryResetAfterSeconds  *int     `json:"secondary_reset_after_seconds,omitempty"`
	SecondaryWindowMinutes      *int     `json:"secondary_window_minutes,omitempty"`
	PrimaryOverSecondaryPercent *float64 `json:"primary_over_secondary_percent,omitempty"`
	UpdatedAt                   string   `json:"updated_at,omitempty"`
}

// NormalizedCodexLimits contains normalized 5h/7d rate limit data
type NormalizedCodexLimits struct {
	Used5hPercent   *float64
	Reset5hSeconds  *int
	Window5hMinutes *int
	Used7dPercent   *float64
	Reset7dSeconds  *int
	Window7dMinutes *int
}

// Normalize converts primary/secondary fields to canonical 5h/7d fields.
// Strategy: Compare window_minutes to determine which is 5h vs 7d.
// Returns nil if snapshot is nil or has no useful data.
func (s *OpenAICodexUsageSnapshot) Normalize() *NormalizedCodexLimits {
	if s == nil {
		return nil
	}

	result := &NormalizedCodexLimits{}

	primaryMins := 0
	secondaryMins := 0
	hasPrimaryWindow := false
	hasSecondaryWindow := false

	if s.PrimaryWindowMinutes != nil {
		primaryMins = *s.PrimaryWindowMinutes
		hasPrimaryWindow = true
	}
	if s.SecondaryWindowMinutes != nil {
		secondaryMins = *s.SecondaryWindowMinutes
		hasSecondaryWindow = true
	}

	// Determine mapping based on window_minutes
	use5hFromPrimary := false
	use7dFromPrimary := false

	if hasPrimaryWindow && hasSecondaryWindow {
		// Both known: smaller window is 5h, larger is 7d
		if primaryMins < secondaryMins {
			use5hFromPrimary = true
		} else {
			use7dFromPrimary = true
		}
	} else if hasPrimaryWindow {
		// Only primary known: classify by threshold (<=360 min = 6h -> 5h window)
		if primaryMins <= 360 {
			use5hFromPrimary = true
		} else {
			use7dFromPrimary = true
		}
	} else if hasSecondaryWindow {
		// Only secondary known: classify by threshold
		if secondaryMins <= 360 {
			// 5h from secondary, so primary (if any data) is 7d
			use7dFromPrimary = true
		} else {
			// 7d from secondary, so primary (if any data) is 5h
			use5hFromPrimary = true
		}
	} else {
		// No window_minutes: fall back to legacy assumption (primary=7d, secondary=5h)
		use7dFromPrimary = true
	}

	// Assign values
	if use5hFromPrimary {
		result.Used5hPercent = s.PrimaryUsedPercent
		result.Reset5hSeconds = s.PrimaryResetAfterSeconds
		result.Window5hMinutes = s.PrimaryWindowMinutes
		result.Used7dPercent = s.SecondaryUsedPercent
		result.Reset7dSeconds = s.SecondaryResetAfterSeconds
		result.Window7dMinutes = s.SecondaryWindowMinutes
	} else if use7dFromPrimary {
		result.Used7dPercent = s.PrimaryUsedPercent
		result.Reset7dSeconds = s.PrimaryResetAfterSeconds
		result.Window7dMinutes = s.PrimaryWindowMinutes
		result.Used5hPercent = s.SecondaryUsedPercent
		result.Reset5hSeconds = s.SecondaryResetAfterSeconds
		result.Window5hMinutes = s.SecondaryWindowMinutes
	}

	return result
}

// OpenAIUsage represents OpenAI API response usage
type OpenAIUsage struct {
	InputTokens              int `json:"input_tokens"`
	OutputTokens             int `json:"output_tokens"`
	CacheCreationInputTokens int `json:"cache_creation_input_tokens,omitempty"`
	CacheReadInputTokens     int `json:"cache_read_input_tokens,omitempty"`
	ImageOutputTokens        int `json:"image_output_tokens,omitempty"`
}

// OpenAIForwardResult represents the result of forwarding
type OpenAIForwardResult struct {
	RequestID string
	Usage     OpenAIUsage
	Model     string // 原始模型（用于响应和日志显示）
	// BillingModel is the model used for cost calculation.
	// When non-empty, CalculateCost uses this instead of Model.
	// This is set by the Anthropic Messages conversion path where
	// the mapped upstream model differs from the client-facing model.
	BillingModel string
	// UpstreamModel is the actual model sent to the upstream provider after mapping.
	// Empty when no mapping was applied (requested model was used as-is).
	UpstreamModel string
	// ServiceTier records the OpenAI Responses API service tier, e.g. "priority" / "flex".
	// Nil means the request did not specify a recognized tier.
	ServiceTier *string
	// ReasoningEffort is extracted from request body (reasoning.effort) or derived from model suffix.
	// Stored for usage records display; nil means not provided / not applicable.
	ReasoningEffort *string
	Stream          bool
	OpenAIWSMode    bool
	ResponseHeaders http.Header
	Duration        time.Duration
	FirstTokenMs    *int
	ImageCount      int
	ImageSize       string
}

type OpenAIWSRetryMetricsSnapshot struct {
	RetryAttemptsTotal            int64 `json:"retry_attempts_total"`
	RetryBackoffMsTotal           int64 `json:"retry_backoff_ms_total"`
	RetryExhaustedTotal           int64 `json:"retry_exhausted_total"`
	NonRetryableFastFallbackTotal int64 `json:"non_retryable_fast_fallback_total"`
}

type OpenAICompatibilityFallbackMetricsSnapshot struct {
	SessionHashLegacyReadFallbackTotal int64   `json:"session_hash_legacy_read_fallback_total"`
	SessionHashLegacyReadFallbackHit   int64   `json:"session_hash_legacy_read_fallback_hit"`
	SessionHashLegacyDualWriteTotal    int64   `json:"session_hash_legacy_dual_write_total"`
	SessionHashLegacyReadHitRate       float64 `json:"session_hash_legacy_read_hit_rate"`

	MetadataLegacyFallbackIsMaxTokensOneHaikuTotal int64 `json:"metadata_legacy_fallback_is_max_tokens_one_haiku_total"`
	MetadataLegacyFallbackThinkingEnabledTotal     int64 `json:"metadata_legacy_fallback_thinking_enabled_total"`
	MetadataLegacyFallbackPrefetchedStickyAccount  int64 `json:"metadata_legacy_fallback_prefetched_sticky_account_total"`
	MetadataLegacyFallbackPrefetchedStickyGroup    int64 `json:"metadata_legacy_fallback_prefetched_sticky_group_total"`
	MetadataLegacyFallbackSingleAccountRetryTotal  int64 `json:"metadata_legacy_fallback_single_account_retry_total"`
	MetadataLegacyFallbackAccountSwitchCountTotal  int64 `json:"metadata_legacy_fallback_account_switch_count_total"`
	MetadataLegacyFallbackTotal                    int64 `json:"metadata_legacy_fallback_total"`
}

type openAIWSRetryMetrics struct {
	retryAttempts            atomic.Int64
	retryBackoffMs           atomic.Int64
	retryExhausted           atomic.Int64
	nonRetryableFastFallback atomic.Int64
}

type accountWriteThrottle struct {
	minInterval time.Duration
	mu          sync.Mutex
	lastByID    map[int64]time.Time
}

func newAccountWriteThrottle(minInterval time.Duration) *accountWriteThrottle {
	return &accountWriteThrottle{
		minInterval: minInterval,
		lastByID:    make(map[int64]time.Time),
	}
}

func (t *accountWriteThrottle) Allow(id int64, now time.Time) bool {
	if t == nil || id <= 0 || t.minInterval <= 0 {
		return true
	}

	t.mu.Lock()
	defer t.mu.Unlock()

	if last, ok := t.lastByID[id]; ok && now.Sub(last) < t.minInterval {
		return false
	}
	t.lastByID[id] = now

	if len(t.lastByID) > 4096 {
		cutoff := now.Add(-4 * t.minInterval)
		for accountID, writtenAt := range t.lastByID {
			if writtenAt.Before(cutoff) {
				delete(t.lastByID, accountID)
			}
		}
	}

	return true
}

var defaultOpenAICodexSnapshotPersistThrottle = newAccountWriteThrottle(openAICodexSnapshotPersistMinInterval)

// ErrNoAvailableCompactAccounts indicates the request needs /responses/compact
// support but no compatible account is available.
var ErrNoAvailableCompactAccounts = errors.New("no available OpenAI accounts support /responses/compact")

// OpenAIGatewayService handles OpenAI API gateway operations
type OpenAIGatewayService struct {
	accountRepo            AccountRepository
	usageLogRepo           UsageLogRepository
	usageBillingRepo       UsageBillingRepository
	userRepo               UserRepository
	userSubRepo            UserSubscriptionRepository
	cache                  GatewayCache
	cfg                    *config.Config
	codexDetector          CodexClientRestrictionDetector
	schedulerSnapshot      *SchedulerSnapshotService
	concurrencyService     *ConcurrencyService
	billingService         *BillingService
	rateLimitService       *RateLimitService
	billingCacheService    *BillingCacheService
	resolver               *ModelPricingResolver
	channelService         *ChannelService
	userGroupRateResolver  *userGroupRateResolver
	httpUpstream           HTTPUpstream
	deferredService        *DeferredService
	openAITokenProvider    *OpenAITokenProvider
	toolCorrector          *CodexToolCorrector
	openaiWSResolver       OpenAIWSProtocolResolver
	balanceNotifyService   *BalanceNotifyService
	generatedImageStore    *OpenAIGeneratedImageStore
	publicSettingsProvider openCodePublicSettingsProvider

	openaiWSPoolOnce              sync.Once
	openaiWSStateStoreOnce        sync.Once
	openaiSchedulerOnce           sync.Once
	openaiWSPassthroughDialerOnce sync.Once
	openaiWSPool                  *openAIWSConnPool
	openaiWSStateStore            OpenAIWSStateStore
	openaiScheduler               OpenAIAccountScheduler
	openaiWSPassthroughDialer     openAIWSClientDialer
	openaiAccountStats            *openAIAccountRuntimeStats

	openaiWSFallbackUntil sync.Map // key: int64(accountID), value: time.Time
	openaiWSRetryMetrics  openAIWSRetryMetrics
	responseHeaderFilter  *responseheaders.CompiledHeaderFilter
	codexSnapshotThrottle *accountWriteThrottle
}

// NewOpenAIGatewayService creates a new OpenAIGatewayService
func NewOpenAIGatewayService(
	accountRepo AccountRepository,
	usageLogRepo UsageLogRepository,
	usageBillingRepo UsageBillingRepository,
	userRepo UserRepository,
	userSubRepo UserSubscriptionRepository,
	userGroupRateRepo UserGroupRateRepository,
	cache GatewayCache,
	cfg *config.Config,
	schedulerSnapshot *SchedulerSnapshotService,
	concurrencyService *ConcurrencyService,
	billingService *BillingService,
	rateLimitService *RateLimitService,
	billingCacheService *BillingCacheService,
	httpUpstream HTTPUpstream,
	deferredService *DeferredService,
	openAITokenProvider *OpenAITokenProvider,
	resolver *ModelPricingResolver,
	channelService *ChannelService,
	balanceNotifyService *BalanceNotifyService,
	generatedImageStore *OpenAIGeneratedImageStore,
	publicSettingsProvider openCodePublicSettingsProvider,
) *OpenAIGatewayService {
	svc := &OpenAIGatewayService{
		accountRepo:         accountRepo,
		usageLogRepo:        usageLogRepo,
		usageBillingRepo:    usageBillingRepo,
		userRepo:            userRepo,
		userSubRepo:         userSubRepo,
		cache:               cache,
		cfg:                 cfg,
		codexDetector:       NewOpenAICodexClientRestrictionDetector(cfg),
		schedulerSnapshot:   schedulerSnapshot,
		concurrencyService:  concurrencyService,
		billingService:      billingService,
		rateLimitService:    rateLimitService,
		billingCacheService: billingCacheService,
		userGroupRateResolver: newUserGroupRateResolver(
			userGroupRateRepo,
			nil,
			resolveUserGroupRateCacheTTL(cfg),
			nil,
			"service.openai_gateway",
		),
		httpUpstream:           httpUpstream,
		deferredService:        deferredService,
		openAITokenProvider:    openAITokenProvider,
		toolCorrector:          NewCodexToolCorrector(),
		openaiWSResolver:       NewOpenAIWSProtocolResolver(cfg),
		resolver:               resolver,
		channelService:         channelService,
		balanceNotifyService:   balanceNotifyService,
		generatedImageStore:    generatedImageStore,
		publicSettingsProvider: publicSettingsProvider,
		responseHeaderFilter:   compileResponseHeaderFilter(cfg),
		codexSnapshotThrottle:  newAccountWriteThrottle(openAICodexSnapshotPersistMinInterval),
	}
	svc.logOpenAIWSModeBootstrap()
	return svc
}

// ResolveChannelMappingAndRestrict 解析渠道映射。
// 模型限制检查已移至调度阶段，restricted 始终返回 false。
func (s *OpenAIGatewayService) ResolveChannelMappingAndRestrict(ctx context.Context, groupID *int64, model string) (ChannelMappingResult, bool) {
	if s.channelService == nil {
		return ChannelMappingResult{MappedModel: model}, false
	}
	return s.channelService.ResolveChannelMappingAndRestrict(ctx, groupID, model)
}

func (s *OpenAIGatewayService) checkChannelPricingRestriction(ctx context.Context, groupID *int64, requestedModel string) bool {
	if groupID == nil || s.channelService == nil || requestedModel == "" {
		return false
	}
	mapping := s.channelService.ResolveChannelMapping(ctx, *groupID, requestedModel)
	billingModel := billingModelForRestriction(mapping.BillingModelSource, requestedModel, mapping.MappedModel)
	if billingModel == "" {
		return false
	}
	return s.channelService.IsModelRestricted(ctx, *groupID, billingModel)
}

func (s *OpenAIGatewayService) isUpstreamModelRestrictedByChannel(ctx context.Context, groupID int64, account *Account, requestedModel string, requireCompact bool) bool {
	if s.channelService == nil {
		return false
	}
	upstreamModel := resolveOpenAIAccountUpstreamModelForRequest(account, requestedModel, requireCompact)
	if upstreamModel == "" {
		return false
	}
	return s.channelService.IsModelRestricted(ctx, groupID, upstreamModel)
}

func (s *OpenAIGatewayService) needsUpstreamChannelRestrictionCheck(ctx context.Context, groupID *int64) bool {
	if groupID == nil || s.channelService == nil {
		return false
	}
	ch, err := s.channelService.GetChannelForGroup(ctx, *groupID)
	if err != nil {
		slog.Warn("failed to check openai channel upstream restriction", "group_id", *groupID, "error", err)
		return false
	}
	if ch == nil || !ch.RestrictModels {
		return false
	}
	return ch.BillingModelSource == BillingModelSourceUpstream
}

// ReplaceModelInBody 替换请求体中的 JSON model 字段（通用 gjson/sjson 实现）。
func (s *OpenAIGatewayService) ReplaceModelInBody(body []byte, newModel string) []byte {
	return ReplaceModelInBody(body, newModel)
}

func (s *OpenAIGatewayService) getCodexSnapshotThrottle() *accountWriteThrottle {
	if s != nil && s.codexSnapshotThrottle != nil {
		return s.codexSnapshotThrottle
	}
	return defaultOpenAICodexSnapshotPersistThrottle
}

func (s *OpenAIGatewayService) billingDeps() *billingDeps {
	return &billingDeps{
		accountRepo:          s.accountRepo,
		userRepo:             s.userRepo,
		userSubRepo:          s.userSubRepo,
		billingCacheService:  s.billingCacheService,
		deferredService:      s.deferredService,
		balanceNotifyService: s.balanceNotifyService,
	}
}

// CloseOpenAIWSPool 关闭 OpenAI WebSocket 连接池的后台 worker 和空闲连接。
// 应在应用优雅关闭时调用。
func (s *OpenAIGatewayService) CloseOpenAIWSPool() {
	if s != nil && s.openaiWSPool != nil {
		s.openaiWSPool.Close()
	}
}

func (s *OpenAIGatewayService) logOpenAIWSModeBootstrap() {
	if s == nil || s.cfg == nil {
		return
	}
	wsCfg := s.cfg.Gateway.OpenAIWS
	logOpenAIWSModeInfo(
		"bootstrap enabled=%v oauth_enabled=%v apikey_enabled=%v force_http=%v responses_websockets_v2=%v responses_websockets=%v payload_log_sample_rate=%.3f event_flush_batch_size=%d event_flush_interval_ms=%d prewarm_cooldown_ms=%d retry_backoff_initial_ms=%d retry_backoff_max_ms=%d retry_jitter_ratio=%.3f retry_total_budget_ms=%d ws_read_limit_bytes=%d",
		wsCfg.Enabled,
		wsCfg.OAuthEnabled,
		wsCfg.APIKeyEnabled,
		wsCfg.ForceHTTP,
		wsCfg.ResponsesWebsocketsV2,
		wsCfg.ResponsesWebsockets,
		wsCfg.PayloadLogSampleRate,
		wsCfg.EventFlushBatchSize,
		wsCfg.EventFlushIntervalMS,
		wsCfg.PrewarmCooldownMS,
		wsCfg.RetryBackoffInitialMS,
		wsCfg.RetryBackoffMaxMS,
		wsCfg.RetryJitterRatio,
		wsCfg.RetryTotalBudgetMS,
		openAIWSMessageReadLimitBytes,
	)
}

func (s *OpenAIGatewayService) getCodexClientRestrictionDetector() CodexClientRestrictionDetector {
	if s != nil && s.codexDetector != nil {
		return s.codexDetector
	}
	var cfg *config.Config
	if s != nil {
		cfg = s.cfg
	}
	return NewOpenAICodexClientRestrictionDetector(cfg)
}

func (s *OpenAIGatewayService) getOpenAIWSProtocolResolver() OpenAIWSProtocolResolver {
	if s != nil && s.openaiWSResolver != nil {
		return s.openaiWSResolver
	}
	var cfg *config.Config
	if s != nil {
		cfg = s.cfg
	}
	return NewOpenAIWSProtocolResolver(cfg)
}

func classifyOpenAIWSReconnectReason(err error) (string, bool) {
	if err == nil {
		return "", false
	}
	var fallbackErr *openAIWSFallbackError
	if !errors.As(err, &fallbackErr) || fallbackErr == nil {
		return "", false
	}
	reason := strings.TrimSpace(fallbackErr.Reason)
	if reason == "" {
		return "", false
	}

	baseReason := strings.TrimPrefix(reason, "prewarm_")

	switch baseReason {
	case "policy_violation",
		"message_too_big",
		"upgrade_required",
		"ws_unsupported",
		"auth_failed",
		"invalid_encrypted_content",
		"previous_response_not_found":
		return reason, false
	}

	switch baseReason {
	case "read_event",
		"write_request",
		"write",
		"acquire_timeout",
		"acquire_conn",
		"conn_queue_full",
		"dial_failed",
		"upstream_5xx",
		"event_error",
		"error_event",
		"upstream_error_event",
		"ws_connection_limit_reached",
		"missing_final_response":
		return reason, true
	default:
		return reason, false
	}
}

func resolveOpenAIWSFallbackErrorResponse(err error) (statusCode int, errType string, clientMessage string, upstreamMessage string, ok bool) {
	if err == nil {
		return 0, "", "", "", false
	}
	var fallbackErr *openAIWSFallbackError
	if !errors.As(err, &fallbackErr) || fallbackErr == nil {
		return 0, "", "", "", false
	}

	reason := strings.TrimSpace(fallbackErr.Reason)
	reason = strings.TrimPrefix(reason, "prewarm_")
	if reason == "" {
		return 0, "", "", "", false
	}

	var dialErr *openAIWSDialError
	if fallbackErr.Err != nil && errors.As(fallbackErr.Err, &dialErr) && dialErr != nil {
		if dialErr.StatusCode > 0 {
			statusCode = dialErr.StatusCode
		}
		if dialErr.Err != nil {
			upstreamMessage = sanitizeUpstreamErrorMessage(strings.TrimSpace(dialErr.Err.Error()))
		}
	}

	switch reason {
	case "invalid_encrypted_content":
		if statusCode == 0 {
			statusCode = http.StatusBadRequest
		}
		errType = "invalid_request_error"
		if upstreamMessage == "" {
			upstreamMessage = "encrypted content could not be verified"
		}
	case "previous_response_not_found":
		if statusCode == 0 {
			statusCode = http.StatusBadRequest
		}
		errType = "invalid_request_error"
		if upstreamMessage == "" {
			upstreamMessage = "previous response not found"
		}
	case "upgrade_required":
		if statusCode == 0 {
			statusCode = http.StatusUpgradeRequired
		}
	case "ws_unsupported":
		if statusCode == 0 {
			statusCode = http.StatusBadRequest
		}
	case "auth_failed":
		if statusCode == 0 {
			statusCode = http.StatusUnauthorized
		}
	case "upstream_rate_limited":
		if statusCode == 0 {
			statusCode = http.StatusTooManyRequests
		}
	default:
		if statusCode == 0 {
			return 0, "", "", "", false
		}
	}

	if upstreamMessage == "" && fallbackErr.Err != nil {
		upstreamMessage = sanitizeUpstreamErrorMessage(strings.TrimSpace(fallbackErr.Err.Error()))
	}
	if upstreamMessage == "" {
		switch reason {
		case "upgrade_required":
			upstreamMessage = "upstream websocket upgrade required"
		case "ws_unsupported":
			upstreamMessage = "upstream websocket not supported"
		case "auth_failed":
			upstreamMessage = "upstream authentication failed"
		case "upstream_rate_limited":
			upstreamMessage = "upstream rate limit exceeded, please retry later"
		default:
			upstreamMessage = "Upstream request failed"
		}
	}

	if errType == "" {
		if statusCode == http.StatusTooManyRequests {
			errType = "rate_limit_error"
		} else {
			errType = "upstream_error"
		}
	}
	clientMessage = upstreamMessage
	return statusCode, errType, clientMessage, upstreamMessage, true
}

func (s *OpenAIGatewayService) writeOpenAIWSFallbackErrorResponse(c *gin.Context, account *Account, wsErr error) bool {
	if c == nil || c.Writer == nil || c.Writer.Written() {
		return false
	}
	statusCode, errType, clientMessage, upstreamMessage, ok := resolveOpenAIWSFallbackErrorResponse(wsErr)
	if !ok {
		return false
	}
	if strings.TrimSpace(clientMessage) == "" {
		clientMessage = "Upstream request failed"
	}
	if strings.TrimSpace(upstreamMessage) == "" {
		upstreamMessage = clientMessage
	}

	setOpsUpstreamError(c, statusCode, upstreamMessage, "")
	if account != nil {
		appendOpsUpstreamError(c, OpsUpstreamErrorEvent{
			Platform:           account.Platform,
			AccountID:          account.ID,
			AccountName:        account.Name,
			UpstreamStatusCode: statusCode,
			Kind:               "ws_error",
			Message:            upstreamMessage,
		})
	}
	c.JSON(statusCode, gin.H{
		"error": gin.H{
			"type":    errType,
			"message": clientMessage,
		},
	})
	return true
}

func (s *OpenAIGatewayService) openAIWSRetryBackoff(attempt int) time.Duration {
	if attempt <= 0 {
		return 0
	}

	initial := openAIWSRetryBackoffInitialDefault
	maxBackoff := openAIWSRetryBackoffMaxDefault
	jitterRatio := openAIWSRetryJitterRatioDefault
	if s != nil && s.cfg != nil {
		wsCfg := s.cfg.Gateway.OpenAIWS
		if wsCfg.RetryBackoffInitialMS > 0 {
			initial = time.Duration(wsCfg.RetryBackoffInitialMS) * time.Millisecond
		}
		if wsCfg.RetryBackoffMaxMS > 0 {
			maxBackoff = time.Duration(wsCfg.RetryBackoffMaxMS) * time.Millisecond
		}
		if wsCfg.RetryJitterRatio >= 0 {
			jitterRatio = wsCfg.RetryJitterRatio
		}
	}
	if initial <= 0 {
		return 0
	}
	if maxBackoff <= 0 {
		maxBackoff = initial
	}
	if maxBackoff < initial {
		maxBackoff = initial
	}
	if jitterRatio < 0 {
		jitterRatio = 0
	}
	if jitterRatio > 1 {
		jitterRatio = 1
	}

	shift := attempt - 1
	if shift < 0 {
		shift = 0
	}
	backoff := initial
	if shift > 0 {
		backoff = initial * time.Duration(1<<shift)
	}
	if backoff > maxBackoff {
		backoff = maxBackoff
	}
	if jitterRatio <= 0 {
		return backoff
	}
	jitter := time.Duration(float64(backoff) * jitterRatio)
	if jitter <= 0 {
		return backoff
	}
	delta := time.Duration(rand.Int63n(int64(jitter)*2+1)) - jitter
	withJitter := backoff + delta
	if withJitter < 0 {
		return 0
	}
	return withJitter
}

func (s *OpenAIGatewayService) openAIWSRetryTotalBudget() time.Duration {
	if s != nil && s.cfg != nil {
		ms := s.cfg.Gateway.OpenAIWS.RetryTotalBudgetMS
		if ms <= 0 {
			return 0
		}
		return time.Duration(ms) * time.Millisecond
	}
	return 0
}

func (s *OpenAIGatewayService) recordOpenAIWSRetryAttempt(backoff time.Duration) {
	if s == nil {
		return
	}
	s.openaiWSRetryMetrics.retryAttempts.Add(1)
	if backoff > 0 {
		s.openaiWSRetryMetrics.retryBackoffMs.Add(backoff.Milliseconds())
	}
}

func (s *OpenAIGatewayService) recordOpenAIWSRetryExhausted() {
	if s == nil {
		return
	}
	s.openaiWSRetryMetrics.retryExhausted.Add(1)
}

func (s *OpenAIGatewayService) recordOpenAIWSNonRetryableFastFallback() {
	if s == nil {
		return
	}
	s.openaiWSRetryMetrics.nonRetryableFastFallback.Add(1)
}

func (s *OpenAIGatewayService) SnapshotOpenAIWSRetryMetrics() OpenAIWSRetryMetricsSnapshot {
	if s == nil {
		return OpenAIWSRetryMetricsSnapshot{}
	}
	return OpenAIWSRetryMetricsSnapshot{
		RetryAttemptsTotal:            s.openaiWSRetryMetrics.retryAttempts.Load(),
		RetryBackoffMsTotal:           s.openaiWSRetryMetrics.retryBackoffMs.Load(),
		RetryExhaustedTotal:           s.openaiWSRetryMetrics.retryExhausted.Load(),
		NonRetryableFastFallbackTotal: s.openaiWSRetryMetrics.nonRetryableFastFallback.Load(),
	}
}

func SnapshotOpenAICompatibilityFallbackMetrics() OpenAICompatibilityFallbackMetricsSnapshot {
	legacyReadFallbackTotal, legacyReadFallbackHit, legacyDualWriteTotal := openAIStickyCompatStats()
	isMaxTokensOneHaiku, thinkingEnabled, prefetchedStickyAccount, prefetchedStickyGroup, singleAccountRetry, accountSwitchCount := RequestMetadataFallbackStats()

	readHitRate := float64(0)
	if legacyReadFallbackTotal > 0 {
		readHitRate = float64(legacyReadFallbackHit) / float64(legacyReadFallbackTotal)
	}
	metadataFallbackTotal := isMaxTokensOneHaiku + thinkingEnabled + prefetchedStickyAccount + prefetchedStickyGroup + singleAccountRetry + accountSwitchCount

	return OpenAICompatibilityFallbackMetricsSnapshot{
		SessionHashLegacyReadFallbackTotal: legacyReadFallbackTotal,
		SessionHashLegacyReadFallbackHit:   legacyReadFallbackHit,
		SessionHashLegacyDualWriteTotal:    legacyDualWriteTotal,
		SessionHashLegacyReadHitRate:       readHitRate,

		MetadataLegacyFallbackIsMaxTokensOneHaikuTotal: isMaxTokensOneHaiku,
		MetadataLegacyFallbackThinkingEnabledTotal:     thinkingEnabled,
		MetadataLegacyFallbackPrefetchedStickyAccount:  prefetchedStickyAccount,
		MetadataLegacyFallbackPrefetchedStickyGroup:    prefetchedStickyGroup,
		MetadataLegacyFallbackSingleAccountRetryTotal:  singleAccountRetry,
		MetadataLegacyFallbackAccountSwitchCountTotal:  accountSwitchCount,
		MetadataLegacyFallbackTotal:                    metadataFallbackTotal,
	}
}

func (s *OpenAIGatewayService) detectCodexClientRestriction(c *gin.Context, account *Account) CodexClientRestrictionDetectionResult {
	return s.getCodexClientRestrictionDetector().Detect(c, account)
}

func getAPIKeyIDFromContext(c *gin.Context) int64 {
	if c == nil {
		return 0
	}
	v, exists := c.Get("api_key")
	if !exists {
		return 0
	}
	apiKey, ok := v.(*APIKey)
	if !ok || apiKey == nil {
		return 0
	}
	return apiKey.ID
}

// isolateOpenAISessionID 将 apiKeyID 混入 session 标识符，
// 确保不同 API Key 的用户即使使用相同的原始 session_id/conversation_id，
// 到达上游的标识符也不同，防止跨用户会话碰撞。
func isolateOpenAISessionID(apiKeyID int64, raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	h := xxhash.New()
	_, _ = fmt.Fprintf(h, "k%d:", apiKeyID)
	_, _ = h.WriteString(raw)
	return fmt.Sprintf("%016x", h.Sum64())
}

func logCodexCLIOnlyDetection(ctx context.Context, c *gin.Context, account *Account, apiKeyID int64, result CodexClientRestrictionDetectionResult, body []byte) {
	if !result.Enabled {
		return
	}
	if ctx == nil {
		ctx = context.Background()
	}
	accountID := int64(0)
	if account != nil {
		accountID = account.ID
	}
	fields := []zap.Field{
		zap.String("component", "service.openai_gateway"),
		zap.Int64("account_id", accountID),
		zap.Bool("codex_cli_only_enabled", result.Enabled),
		zap.Bool("codex_official_client_match", result.Matched),
		zap.String("reject_reason", result.Reason),
	}
	if apiKeyID > 0 {
		fields = append(fields, zap.Int64("api_key_id", apiKeyID))
	}
	if !result.Matched {
		fields = appendCodexCLIOnlyRejectedRequestFields(fields, c, body)
	}
	log := logger.FromContext(ctx).With(fields...)
	if result.Matched {
		return
	}
	log.Warn("OpenAI codex_cli_only 拒绝非官方客户端请求")
}

func appendCodexCLIOnlyRejectedRequestFields(fields []zap.Field, c *gin.Context, body []byte) []zap.Field {
	if c == nil || c.Request == nil {
		return fields
	}

	req := c.Request
	requestModel, requestStream, promptCacheKey := extractOpenAIRequestMetaFromBody(body)
	fields = append(fields,
		zap.String("request_method", strings.TrimSpace(req.Method)),
		zap.String("request_path", strings.TrimSpace(req.URL.Path)),
		zap.String("request_query", strings.TrimSpace(req.URL.RawQuery)),
		zap.String("request_host", strings.TrimSpace(req.Host)),
		zap.String("request_client_ip", strings.TrimSpace(c.ClientIP())),
		zap.String("request_remote_addr", strings.TrimSpace(req.RemoteAddr)),
		zap.String("request_user_agent", strings.TrimSpace(req.Header.Get("User-Agent"))),
		zap.String("request_content_type", strings.TrimSpace(req.Header.Get("Content-Type"))),
		zap.Int64("request_content_length", req.ContentLength),
		zap.Bool("request_stream", requestStream),
	)
	if requestModel != "" {
		fields = append(fields, zap.String("request_model", requestModel))
	}
	if promptCacheKey != "" {
		fields = append(fields, zap.String("request_prompt_cache_key_sha256", hashSensitiveValueForLog(promptCacheKey)))
	}

	if headers := snapshotCodexCLIOnlyHeaders(req.Header); len(headers) > 0 {
		fields = append(fields, zap.Any("request_headers", headers))
	}
	fields = append(fields, zap.Int("request_body_size", len(body)))
	return fields
}

func snapshotCodexCLIOnlyHeaders(header http.Header) map[string]string {
	if len(header) == 0 {
		return nil
	}
	result := make(map[string]string, len(codexCLIOnlyDebugHeaderWhitelist))
	for _, key := range codexCLIOnlyDebugHeaderWhitelist {
		value := strings.TrimSpace(header.Get(key))
		if value == "" {
			continue
		}
		result[strings.ToLower(key)] = truncateString(value, codexCLIOnlyHeaderValueMaxBytes)
	}
	return result
}

func hashSensitiveValueForLog(raw string) string {
	value := strings.TrimSpace(raw)
	if value == "" {
		return ""
	}
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:8])
}

func logOpenAIInstructionsRequiredDebug(
	ctx context.Context,
	c *gin.Context,
	account *Account,
	upstreamStatusCode int,
	upstreamMsg string,
	requestBody []byte,
	upstreamBody []byte,
) {
	msg := strings.TrimSpace(upstreamMsg)
	if !isOpenAIInstructionsRequiredError(upstreamStatusCode, msg, upstreamBody) {
		return
	}
	if ctx == nil {
		ctx = context.Background()
	}

	accountID := int64(0)
	accountName := ""
	if account != nil {
		accountID = account.ID
		accountName = strings.TrimSpace(account.Name)
	}

	userAgent := ""
	originator := ""
	if c != nil {
		userAgent = strings.TrimSpace(c.GetHeader("User-Agent"))
		originator = strings.TrimSpace(c.GetHeader("originator"))
	}

	fields := []zap.Field{
		zap.String("component", "service.openai_gateway"),
		zap.Int64("account_id", accountID),
		zap.String("account_name", accountName),
		zap.Int("upstream_status_code", upstreamStatusCode),
		zap.String("upstream_error_message", msg),
		zap.String("request_user_agent", userAgent),
		zap.Bool("codex_official_client_match", openai.IsCodexOfficialClientByHeaders(userAgent, originator)),
	}
	fields = appendCodexCLIOnlyRejectedRequestFields(fields, c, requestBody)

	logger.FromContext(ctx).With(fields...).Warn("OpenAI 上游返回 Instructions are required，已记录请求详情用于排查")
}

func isOpenAIInstructionsRequiredError(upstreamStatusCode int, upstreamMsg string, upstreamBody []byte) bool {
	if upstreamStatusCode != http.StatusBadRequest {
		return false
	}

	hasInstructionRequired := func(text string) bool {
		lower := strings.ToLower(strings.TrimSpace(text))
		if lower == "" {
			return false
		}
		if strings.Contains(lower, "instructions are required") {
			return true
		}
		if strings.Contains(lower, "required parameter: 'instructions'") {
			return true
		}
		if strings.Contains(lower, "required parameter: instructions") {
			return true
		}
		if strings.Contains(lower, "missing required parameter") && strings.Contains(lower, "instructions") {
			return true
		}
		return strings.Contains(lower, "instruction") && strings.Contains(lower, "required")
	}

	if hasInstructionRequired(upstreamMsg) {
		return true
	}
	if len(upstreamBody) == 0 {
		return false
	}

	errMsg := gjson.GetBytes(upstreamBody, "error.message").String()
	errMsgLower := strings.ToLower(strings.TrimSpace(errMsg))
	errCode := strings.ToLower(strings.TrimSpace(gjson.GetBytes(upstreamBody, "error.code").String()))
	errParam := strings.ToLower(strings.TrimSpace(gjson.GetBytes(upstreamBody, "error.param").String()))
	errType := strings.ToLower(strings.TrimSpace(gjson.GetBytes(upstreamBody, "error.type").String()))

	if errParam == "instructions" {
		return true
	}
	if hasInstructionRequired(errMsg) {
		return true
	}
	if strings.Contains(errCode, "missing_required_parameter") && strings.Contains(errMsgLower, "instructions") {
		return true
	}
	if strings.Contains(errType, "invalid_request") && strings.Contains(errMsgLower, "instructions") && strings.Contains(errMsgLower, "required") {
		return true
	}

	return false
}

func isOpenAITransientProcessingError(upstreamStatusCode int, upstreamMsg string, upstreamBody []byte) bool {
	errCode := strings.ToLower(strings.TrimSpace(gjson.GetBytes(upstreamBody, "error.code").String()))
	errType := strings.ToLower(strings.TrimSpace(gjson.GetBytes(upstreamBody, "error.type").String()))
	if errCode == "server_is_overloaded" || errType == "service_unavailable_error" {
		return true
	}
	if strings.Contains(strings.ToLower(strings.TrimSpace(upstreamMsg)), "currently overloaded") {
		return true
	}
	if upstreamStatusCode != http.StatusBadRequest {
		return false
	}

	match := func(text string) bool {
		lower := strings.ToLower(strings.TrimSpace(text))
		if lower == "" {
			return false
		}
		if strings.Contains(lower, "an error occurred while processing your request") {
			return true
		}
		return strings.Contains(lower, "you can retry your request") &&
			strings.Contains(lower, "help.openai.com") &&
			strings.Contains(lower, "request id")
	}

	if match(upstreamMsg) {
		return true
	}
	if len(upstreamBody) == 0 {
		return false
	}
	if match(gjson.GetBytes(upstreamBody, "error.message").String()) {
		return true
	}
	return match(string(upstreamBody))
}

func logOpenAITransientProcessingFailover(ctx context.Context, c *gin.Context, account *Account, statusCode int, upstreamRequestID, upstreamMsg string) {
	fields := []zap.Field{
		zap.Int("status_code", statusCode),
		zap.String("upstream_request_id", strings.TrimSpace(upstreamRequestID)),
		zap.String("upstream_message", strings.TrimSpace(upstreamMsg)),
	}
	if account != nil {
		fields = append(fields,
			zap.Int64("account_id", account.ID),
			zap.String("account_name", account.Name),
			zap.String("account_platform", string(account.Platform)),
		)
	}
	if c != nil {
		fields = append(fields,
			zap.String("request_id", strings.TrimSpace(c.Writer.Header().Get("X-Request-Id"))),
			zap.String("path", c.FullPath()),
		)
	}
	logger.FromContext(ctx).Warn("openai transient processing error entered failover path", fields...)
}

type openAIUpstreamErrorEnvelope struct {
	Error openAIUpstreamErrorEnvelopeError `json:"error"`
}

type openAIUpstreamErrorEnvelopeError struct {
	Type     string         `json:"type"`
	Message  string         `json:"message"`
	Upstream map[string]any `json:"upstream,omitempty"`
}

func hasOpenAIStructuredError(body []byte) bool {
	if len(body) == 0 {
		return false
	}
	if gjson.GetBytes(body, "error").Exists() {
		return true
	}
	if strings.TrimSpace(gjson.GetBytes(body, "detail").String()) != "" {
		return true
	}
	return false
}

func buildOpenAIUpstreamErrorEnvelope(status int, body []byte, fallback string) (int, openAIUpstreamErrorEnvelope) {
	msg := strings.TrimSpace(extractUpstreamErrorMessage(body))
	if msg == "" {
		msg = strings.TrimSpace(fallback)
	}
	if msg == "" {
		msg = fmt.Sprintf("Upstream error: %d", status)
	}

	envelope := openAIUpstreamErrorEnvelope{}
	envelope.Error.Type = "upstream_error"
	envelope.Error.Message = msg

	upstream := map[string]any{
		"status":  status,
		"message": msg,
	}
	if code := strings.TrimSpace(gjson.GetBytes(body, "error.code").String()); code != "" {
		upstream["code"] = code
	}
	if typ := strings.TrimSpace(gjson.GetBytes(body, "error.type").String()); typ != "" {
		upstream["type"] = typ
	}
	if param := strings.TrimSpace(gjson.GetBytes(body, "error.param").String()); param != "" {
		upstream["param"] = param
	}

	var raw any
	if len(body) > 0 {
		if err := json.Unmarshal(body, &raw); err == nil {
			upstream["raw"] = raw
		} else {
			upstream["raw"] = string(body)
		}
	}

	envelope.Error.Upstream = upstream
	return status, envelope
}

func extractOpenAISessionSignal(c *gin.Context, body []byte, includeContentFallback bool) string {
	if c != nil {
		if sessionID := strings.TrimSpace(c.GetHeader("session_id")); sessionID != "" {
			return sessionID
		}
		if conversationID := strings.TrimSpace(c.GetHeader("conversation_id")); conversationID != "" {
			return conversationID
		}
		if sessionAffinity := strings.TrimSpace(c.GetHeader("x-session-affinity")); sessionAffinity != "" {
			return sessionAffinity
		}
	}
	if len(body) == 0 {
		return ""
	}
	if promptCacheKey := strings.TrimSpace(gjson.GetBytes(body, "prompt_cache_key").String()); promptCacheKey != "" {
		return promptCacheKey
	}
	if includeContentFallback {
		return deriveOpenAIContentSessionSeed(body)
	}
	return ""
}

// ExtractSessionID extracts the raw session ID from headers or body without hashing.
// Used by ForwardAsAnthropic to pass as prompt_cache_key for upstream cache.
func (s *OpenAIGatewayService) ExtractSessionID(c *gin.Context, body []byte) string {
	return extractOpenAISessionSignal(c, body, false)
}

// GenerateSessionHash generates a sticky-session hash for OpenAI requests.
//
// Priority:
//  1. Header: session_id
//  2. Header: conversation_id
//  3. Header: x-session-affinity
//  4. Body:   prompt_cache_key (opencode)
//  5. Body:   content-based fallback (model + system + tools + first user message)
func (s *OpenAIGatewayService) GenerateSessionHash(c *gin.Context, body []byte) string {
	if c == nil {
		return ""
	}

	sessionID := extractOpenAISessionSignal(c, body, true)
	if sessionID == "" {
		return ""
	}

	currentHash, legacyHash := deriveOpenAISessionHashes(sessionID)
	attachOpenAILegacySessionHashToGin(c, legacyHash)
	return currentHash
}

// GenerateSessionHashWithFallback 先按常规信号生成会话哈希；
// 当未携带 session_id/conversation_id/x-session-affinity/prompt_cache_key 时，使用 fallbackSeed 生成稳定哈希。
// 该方法用于 WS ingress，避免会话信号缺失时发生跨账号漂移。
func (s *OpenAIGatewayService) GenerateSessionHashWithFallback(c *gin.Context, body []byte, fallbackSeed string) string {
	sessionHash := s.GenerateSessionHash(c, body)
	if sessionHash != "" {
		return sessionHash
	}

	seed := strings.TrimSpace(fallbackSeed)
	if seed == "" {
		return ""
	}

	currentHash, legacyHash := deriveOpenAISessionHashes(seed)
	attachOpenAILegacySessionHashToGin(c, legacyHash)
	return currentHash
}

func resolveOpenAIUpstreamOriginator(c *gin.Context, isOfficialClient bool) string {
	if c != nil {
		if originator := strings.TrimSpace(c.GetHeader("originator")); originator != "" {
			return originator
		}
	}
	if isOfficialClient {
		return "codex_cli_rs"
	}
	return "opencode"
}

// BindStickySession sets session -> account binding with standard TTL.
func (s *OpenAIGatewayService) BindStickySession(ctx context.Context, groupID *int64, sessionHash string, accountID int64) error {
	if sessionHash == "" || accountID <= 0 {
		return nil
	}
	ttl := openaiStickySessionTTL
	if s != nil && s.cfg != nil && s.cfg.Gateway.OpenAIWS.StickySessionTTLSeconds > 0 {
		ttl = time.Duration(s.cfg.Gateway.OpenAIWS.StickySessionTTLSeconds) * time.Second
	}
	return s.setStickySessionAccountID(ctx, groupID, sessionHash, accountID, ttl)
}

// SelectAccount selects an OpenAI account with sticky session support
func (s *OpenAIGatewayService) SelectAccount(ctx context.Context, groupID *int64, sessionHash string) (*Account, error) {
	return s.SelectAccountForModel(ctx, groupID, sessionHash, "")
}

// SelectAccountForModel selects an account supporting the requested model
func (s *OpenAIGatewayService) SelectAccountForModel(ctx context.Context, groupID *int64, sessionHash string, requestedModel string) (*Account, error) {
	return s.SelectAccountForModelWithExclusions(ctx, groupID, sessionHash, requestedModel, nil)
}

// SelectAccountForModelWithExclusions selects an account supporting the requested model while excluding specified accounts.
// SelectAccountForModelWithExclusions 选择支持指定模型的账号，同时排除指定的账号。
func (s *OpenAIGatewayService) SelectAccountForModelWithExclusions(ctx context.Context, groupID *int64, sessionHash string, requestedModel string, excludedIDs map[int64]struct{}) (*Account, error) {
	return s.selectAccountForModelWithExclusions(ctx, groupID, sessionHash, requestedModel, excludedIDs, false, 0)
}

// noAvailableOpenAISelectionError builds the standard "no account available" error
// while preserving the compact-specific error when applicable.
func noAvailableOpenAISelectionError(requestedModel string, compactBlocked bool) error {
	if compactBlocked {
		return ErrNoAvailableCompactAccounts
	}
	if requestedModel != "" {
		return fmt.Errorf("no available OpenAI accounts supporting model: %s", requestedModel)
	}
	return errors.New("no available OpenAI accounts")
}

// openAICompactSupportTier classifies an OpenAI account by compact capability.
// 0 = explicitly unsupported, 1 = unknown / not yet probed, 2 = explicitly supported.
func openAICompactSupportTier(account *Account) int {
	if account == nil || !account.IsOpenAI() {
		return 0
	}
	supported, known := account.OpenAICompactSupportKnown()
	if !known {
		return 1
	}
	if supported {
		return 2
	}
	return 0
}

// isOpenAIAccountEligibleForRequest centralises the schedulable / OpenAI / model /
// compact-support checks used during account selection.
func isOpenAIAccountEligibleForRequest(account *Account, requestedModel string, requireCompact bool) bool {
	if account == nil || !account.IsSchedulable() || !account.IsOpenAI() {
		return false
	}
	if requestedModel != "" && !account.IsModelSupported(requestedModel) {
		return false
	}
	if requireCompact && openAICompactSupportTier(account) == 0 {
		return false
	}
	return true
}

// prioritizeOpenAICompactAccounts re-orders a slice so that accounts with known
// compact support are tried first, followed by unknown, then explicitly unsupported.
// The relative order within each tier is preserved.
func prioritizeOpenAICompactAccounts(accounts []*Account) []*Account {
	if len(accounts) == 0 {
		return nil
	}
	supported := make([]*Account, 0, len(accounts))
	unknown := make([]*Account, 0, len(accounts))
	unsupported := make([]*Account, 0, len(accounts))
	for _, account := range accounts {
		switch openAICompactSupportTier(account) {
		case 2:
			supported = append(supported, account)
		case 1:
			unknown = append(unknown, account)
		default:
			unsupported = append(unsupported, account)
		}
	}
	out := make([]*Account, 0, len(accounts))
	out = append(out, supported...)
	out = append(out, unknown...)
	out = append(out, unsupported...)
	return out
}

// resolveOpenAIAccountUpstreamModelForRequest resolves the upstream model that
// would be sent for a given request, honouring compact-only mappings when the
// caller is on the /responses/compact path.
func resolveOpenAIAccountUpstreamModelForRequest(account *Account, requestedModel string, requireCompact bool) string {
	upstreamModel := resolveOpenAIForwardModel(account, requestedModel, "")
	if upstreamModel == "" {
		return ""
	}
	if requireCompact {
		return resolveOpenAICompactForwardModel(account, upstreamModel)
	}
	return upstreamModel
}

func (s *OpenAIGatewayService) selectAccountForModelWithExclusions(ctx context.Context, groupID *int64, sessionHash string, requestedModel string, excludedIDs map[int64]struct{}, requireCompact bool, stickyAccountID int64) (*Account, error) {
	if s.checkChannelPricingRestriction(ctx, groupID, requestedModel) {
		slog.Warn("channel pricing restriction blocked request",
			"group_id", derefGroupID(groupID),
			"model", requestedModel)
		return nil, fmt.Errorf("%w supporting model: %s (channel pricing restriction)", ErrNoAvailableAccounts, requestedModel)
	}
	noAvailableErr := func() error {
		if requestedModel != "" {
			return fmt.Errorf("%w supporting model: %s", ErrNoAvailableAccounts, requestedModel)
		}
		return ErrNoAvailableAccounts
	}

	// 1. 尝试粘性会话命中
	// Try sticky session hit
	if account := s.tryStickySessionHit(ctx, groupID, sessionHash, requestedModel, excludedIDs, requireCompact, stickyAccountID); account != nil {
		return account, nil
	}

	// 2. 获取可调度的 OpenAI 账号
	// Get schedulable OpenAI accounts
	accounts, err := s.listSchedulableAccounts(ctx, groupID, TargetGroupAny)
	if err != nil {
		return nil, fmt.Errorf("query accounts failed: %w", err)
	}
	if len(accounts) == 0 {
		return nil, noAvailableErr()
	}

	// 3. 按优先级 + LRU 选择最佳账号
	// Select by priority + LRU
	selected, compactBlocked := s.selectBestAccount(ctx, groupID, accounts, requestedModel, excludedIDs, requireCompact)

	if selected == nil {
		return nil, noAvailableOpenAISelectionError(requestedModel, compactBlocked)
	}

	// 4. 设置粘性会话绑定
	// Set sticky session binding
	if sessionHash != "" {
		_ = s.setStickySessionAccountID(ctx, groupID, sessionHash, selected.ID, openaiStickySessionTTL)
	}

	return s.hydrateSelectedAccount(ctx, selected)
}

// tryStickySessionHit 尝试从粘性会话获取账号。
// 如果命中且账号可用则返回账号；如果账号不可用则清理会话并返回 nil。
//
// tryStickySessionHit attempts to get account from sticky session.
// Returns account if hit and usable; clears session and returns nil if account is unavailable.
func (s *OpenAIGatewayService) tryStickySessionHit(ctx context.Context, groupID *int64, sessionHash, requestedModel string, excludedIDs map[int64]struct{}, requireCompact bool, stickyAccountID int64) *Account {
	if sessionHash == "" {
		return nil
	}

	accountID := stickyAccountID
	if accountID <= 0 {
		var err error
		accountID, err = s.getStickySessionAccountID(ctx, groupID, sessionHash)
		if err != nil || accountID <= 0 {
			return nil
		}
	}

	if _, excluded := excludedIDs[accountID]; excluded {
		return nil
	}

	account, err := s.getSchedulableAccount(ctx, accountID)
	if err != nil {
		return nil
	}

	// 检查账号是否需要清理粘性会话
	// Check if sticky session should be cleared
	if shouldClearStickySession(account, requestedModel) {
		_ = s.deleteStickySessionAccountID(ctx, groupID, sessionHash)
		return nil
	}
	needsUpstreamCheck := s.needsUpstreamChannelRestrictionCheck(ctx, groupID)
	account, shouldClearSticky := s.prepareStickySelectedOpenAIAccount(ctx, groupID, account, requestedModel, TargetGroupAny, needsUpstreamCheck, requireCompact)
	if account == nil {
		if shouldClearSticky {
			_ = s.deleteStickySessionAccountID(ctx, groupID, sessionHash)
		}
		return nil
	}
	if groupID != nil && s.needsUpstreamChannelRestrictionCheck(ctx, groupID) &&
		s.isUpstreamModelRestrictedByChannel(ctx, *groupID, account, requestedModel, requireCompact) {
		_ = s.deleteStickySessionAccountID(ctx, groupID, sessionHash)
		return nil
	}

	// 刷新会话 TTL 并返回账号
	// Refresh session TTL and return account
	_ = s.refreshStickySessionTTL(ctx, groupID, sessionHash, openaiStickySessionTTL)
	return account
}

// selectBestAccount 从候选账号中选择最佳账号（优先级 + LRU）。
// 返回 nil 表示无可用账号。
//
// selectBestAccount selects the best account from candidates (priority + LRU).
// Returns nil if no available account. The second return reports whether at
// least one candidate was filtered out solely because it lacks compact support
// (only meaningful when requireCompact=true).
func (s *OpenAIGatewayService) selectBestAccount(ctx context.Context, groupID *int64, accounts []Account, requestedModel string, excludedIDs map[int64]struct{}, requireCompact bool) (*Account, bool) {
	var selected *Account
	selectedCompactTier := -1
	compactBlocked := false
	needsUpstreamCheck := s.needsUpstreamChannelRestrictionCheck(ctx, groupID)

	for i := range accounts {
		acc := &accounts[i]

		// 跳过被排除的账号
		// Skip excluded accounts
		if _, excluded := excludedIDs[acc.ID]; excluded {
			continue
		}

		fresh := s.resolveFreshSchedulableOpenAIAccount(ctx, acc, requestedModel, TargetGroupAny, requireCompact)
		if fresh == nil {
			continue
		}
		fresh = s.recheckSelectedOpenAIAccountFromDB(ctx, fresh, requestedModel, TargetGroupAny, requireCompact)
		if fresh == nil {
			continue
		}
		if needsUpstreamCheck && s.isUpstreamModelRestrictedByChannel(ctx, *groupID, fresh, requestedModel, requireCompact) {
			continue
		}
		compactTier := 0
		if requireCompact {
			compactTier = openAICompactSupportTier(fresh)
			if compactTier == 0 {
				compactBlocked = true
				continue
			}
		}

		// 选择优先级最高且最久未使用的账号
		// Select highest priority and least recently used
		if selected == nil {
			selected = fresh
			selectedCompactTier = compactTier
			continue
		}

		// compact 模式下高 tier 优先；同 tier 内才比较 priority/LRU。
		if requireCompact && compactTier != selectedCompactTier {
			if compactTier > selectedCompactTier {
				selected = fresh
				selectedCompactTier = compactTier
			}
			continue
		}

		if s.isBetterAccount(fresh, selected) {
			selected = fresh
			selectedCompactTier = compactTier
		}
	}

	return selected, compactBlocked
}

// isBetterAccount 判断 candidate 是否比 current 更优。
// 规则：优先级更高（数值更小）优先；同优先级时，未使用过的优先，其次是最久未使用的。
//
// isBetterAccount checks if candidate is better than current.
// Rules: higher priority (lower value) wins; same priority: never used > least recently used.
func (s *OpenAIGatewayService) isBetterAccount(candidate, current *Account) bool {
	// 优先级更高（数值更小）
	// Higher priority (lower value)
	if candidate.Priority < current.Priority {
		return true
	}
	if candidate.Priority > current.Priority {
		return false
	}

	// 同优先级，比较最后使用时间
	// Same priority, compare last used time
	switch {
	case candidate.LastUsedAt == nil && current.LastUsedAt != nil:
		// candidate 从未使用，优先
		return true
	case candidate.LastUsedAt != nil && current.LastUsedAt == nil:
		// current 从未使用，保持
		return false
	case candidate.LastUsedAt == nil && current.LastUsedAt == nil:
		// 都未使用，保持
		return false
	default:
		// 都使用过，选择最久未使用的
		return candidate.LastUsedAt.Before(*current.LastUsedAt)
	}
}

// SelectAccountWithLoadAwareness selects an account with load-awareness and wait plan.
func (s *OpenAIGatewayService) SelectAccountWithLoadAwareness(ctx context.Context, groupID *int64, sessionHash string, requestedModel string, excludedIDs map[int64]struct{}) (*AccountSelectionResult, error) {
	return s.selectAccountWithLoadAwareness(ctx, groupID, sessionHash, requestedModel, excludedIDs, false)
}

func (s *OpenAIGatewayService) selectAccountWithLoadAwareness(ctx context.Context, groupID *int64, sessionHash string, requestedModel string, excludedIDs map[int64]struct{}, requireCompact bool) (*AccountSelectionResult, error) {
	if s.checkChannelPricingRestriction(ctx, groupID, requestedModel) {
		slog.Warn("channel pricing restriction blocked request",
			"group_id", derefGroupID(groupID),
			"model", requestedModel)
		return nil, fmt.Errorf("%w supporting model: %s (channel pricing restriction)", ErrNoAvailableAccounts, requestedModel)
	}

	cfg := s.schedulingConfig()
	needsUpstreamCheck := s.needsUpstreamChannelRestrictionCheck(ctx, groupID)
	var stickyAccountID int64
	if sessionHash != "" && s.cache != nil {
		if accountID, err := s.getStickySessionAccountID(ctx, groupID, sessionHash); err == nil {
			stickyAccountID = accountID
		}
	}
	if s.concurrencyService == nil || !cfg.LoadBatchEnabled {
		account, err := s.selectAccountForModelWithExclusions(ctx, groupID, sessionHash, requestedModel, excludedIDs, requireCompact, stickyAccountID)
		if err != nil {
			return nil, err
		}
		result, err := s.tryAcquireAccountSlot(ctx, account.ID, account.Concurrency)
		if err == nil && result.Acquired {
			return s.newSelectionResult(ctx, account, true, result.ReleaseFunc, nil)
		}
		if stickyAccountID > 0 && stickyAccountID == account.ID && s.concurrencyService != nil {
			waitingCount, _ := s.concurrencyService.GetAccountWaitingCount(ctx, account.ID)
			if waitingCount < cfg.StickySessionMaxWaiting {
				return s.newSelectionResult(ctx, account, false, nil, &AccountWaitPlan{
					AccountID:      account.ID,
					MaxConcurrency: account.Concurrency,
					Timeout:        cfg.StickySessionWaitTimeout,
					MaxWaiting:     cfg.StickySessionMaxWaiting,
				})
			}
		}
		return s.newSelectionResult(ctx, account, false, nil, &AccountWaitPlan{
			AccountID:      account.ID,
			MaxConcurrency: account.Concurrency,
			Timeout:        cfg.FallbackWaitTimeout,
			MaxWaiting:     cfg.FallbackMaxWaiting,
		})
	}

	accounts, err := s.listSchedulableAccounts(ctx, groupID, TargetGroupAny)
	if err != nil {
		return nil, err
	}
	if len(accounts) == 0 {
		return nil, ErrNoAvailableAccounts
	}

	isExcluded := func(accountID int64) bool {
		if excludedIDs == nil {
			return false
		}
		_, excluded := excludedIDs[accountID]
		return excluded
	}

	// ============ Layer 1: Sticky session ============
	if sessionHash != "" {
		accountID := stickyAccountID
		if accountID > 0 && !isExcluded(accountID) {
			account, err := s.getSchedulableAccount(ctx, accountID)
			if err == nil {
				clearSticky := shouldClearStickySession(account, requestedModel)
				if clearSticky {
					_ = s.deleteStickySessionAccountID(ctx, groupID, sessionHash)
				}
				if !clearSticky {
					account, shouldClearSticky := s.prepareStickySelectedOpenAIAccount(ctx, groupID, account, requestedModel, TargetGroupAny, needsUpstreamCheck, requireCompact)
					if account == nil {
						if shouldClearSticky {
							_ = s.deleteStickySessionAccountID(ctx, groupID, sessionHash)
						}
					} else {
						result, err := s.tryAcquireAccountSlot(ctx, accountID, account.Concurrency)
						if err == nil && result.Acquired {
							_ = s.refreshStickySessionTTL(ctx, groupID, sessionHash, openaiStickySessionTTL)
							return s.newSelectionResult(ctx, account, true, result.ReleaseFunc, nil)
						}

						waitingCount, _ := s.concurrencyService.GetAccountWaitingCount(ctx, accountID)
						if waitingCount < cfg.StickySessionMaxWaiting {
							return s.newSelectionResult(ctx, account, false, nil, &AccountWaitPlan{
								AccountID:      accountID,
								MaxConcurrency: account.Concurrency,
								Timeout:        cfg.StickySessionWaitTimeout,
								MaxWaiting:     cfg.StickySessionMaxWaiting,
							})
						}
					}
				}
			}
		}
	}

	// ============ Layer 2: Load-aware selection ============
	baseCandidateCount := 0
	candidates := make([]*Account, 0, len(accounts))
	for i := range accounts {
		acc := &accounts[i]
		if isExcluded(acc.ID) {
			continue
		}
		// Scheduler snapshots can be temporarily stale (bucket rebuild is throttled);
		// re-check schedulability here so recently rate-limited/overloaded accounts
		// are not selected again before the bucket is rebuilt.
		if !acc.IsSchedulable() {
			continue
		}
		if requestedModel != "" && !acc.IsModelSupported(requestedModel) {
			continue
		}
		if needsUpstreamCheck && s.isUpstreamModelRestrictedByChannel(ctx, *groupID, acc, requestedModel, requireCompact) {
			continue
		}
		baseCandidateCount++
		candidates = append(candidates, acc)
	}

	if len(candidates) == 0 {
		return nil, ErrNoAvailableAccounts
	}

	accountLoads := make([]AccountWithConcurrency, 0, len(candidates))
	for _, acc := range candidates {
		accountLoads = append(accountLoads, AccountWithConcurrency{
			ID:             acc.ID,
			MaxConcurrency: acc.EffectiveLoadFactor(),
		})
	}

	loadMap, err := s.concurrencyService.GetAccountsLoadBatch(ctx, accountLoads)
	if err != nil {
		ordered := append([]*Account(nil), candidates...)
		sortAccountsByPriorityAndLastUsed(ordered, false)
		if requireCompact {
			ordered = prioritizeOpenAICompactAccounts(ordered)
		}
		for _, acc := range ordered {
			fresh := s.resolveFreshSchedulableOpenAIAccount(ctx, acc, requestedModel, TargetGroupAny, requireCompact)
			if fresh == nil {
				continue
			}
			fresh = s.recheckSelectedOpenAIAccountFromDB(ctx, fresh, requestedModel, TargetGroupAny, requireCompact)
			if fresh == nil {
				continue
			}
			if needsUpstreamCheck && s.isUpstreamModelRestrictedByChannel(ctx, *groupID, fresh, requestedModel, requireCompact) {
				continue
			}
			result, err := s.tryAcquireAccountSlot(ctx, fresh.ID, fresh.Concurrency)
			if err == nil && result.Acquired {
				if sessionHash != "" {
					_ = s.setStickySessionAccountID(ctx, groupID, sessionHash, fresh.ID, openaiStickySessionTTL)
				}
				return s.newSelectionResult(ctx, fresh, true, result.ReleaseFunc, nil)
			}
		}
	} else {
		var available []accountWithLoad
		for _, acc := range candidates {
			loadInfo := loadMap[acc.ID]
			if loadInfo == nil {
				loadInfo = &AccountLoadInfo{AccountID: acc.ID}
			}
			if loadInfo.LoadRate < 100 {
				available = append(available, accountWithLoad{
					account:  acc,
					loadInfo: loadInfo,
				})
			}
		}

		if len(available) > 0 {
			sort.SliceStable(available, func(i, j int) bool {
				a, b := available[i], available[j]
				if a.account.Priority != b.account.Priority {
					return a.account.Priority < b.account.Priority
				}
				if a.loadInfo.LoadRate != b.loadInfo.LoadRate {
					return a.loadInfo.LoadRate < b.loadInfo.LoadRate
				}
				switch {
				case a.account.LastUsedAt == nil && b.account.LastUsedAt != nil:
					return true
				case a.account.LastUsedAt != nil && b.account.LastUsedAt == nil:
					return false
				case a.account.LastUsedAt == nil && b.account.LastUsedAt == nil:
					return false
				default:
					return a.account.LastUsedAt.Before(*b.account.LastUsedAt)
				}
			})
			shuffleWithinSortGroups(available)

			selectionOrder := make([]accountWithLoad, 0, len(available))
			if requireCompact {
				appendTier := func(out []accountWithLoad, tier int) []accountWithLoad {
					for _, item := range available {
						if openAICompactSupportTier(item.account) == tier {
							out = append(out, item)
						}
					}
					return out
				}
				selectionOrder = appendTier(selectionOrder, 2)
				selectionOrder = appendTier(selectionOrder, 1)
				// tier 0 候选作为兜底追加：DB recheck 时若发现 cache tier 0 实际
				// 已升级为 1/2（探测刚跑完，cache 尚未刷新），仍可正常命中。
				selectionOrder = appendTier(selectionOrder, 0)
			} else {
				selectionOrder = append(selectionOrder, available...)
			}

			for _, item := range selectionOrder {
				fresh := s.resolveFreshSchedulableOpenAIAccount(ctx, item.account, requestedModel, TargetGroupAny, requireCompact)
				if fresh == nil {
					continue
				}
				fresh = s.recheckSelectedOpenAIAccountFromDB(ctx, fresh, requestedModel, TargetGroupAny, requireCompact)
				if fresh == nil {
					continue
				}
				if needsUpstreamCheck && s.isUpstreamModelRestrictedByChannel(ctx, *groupID, fresh, requestedModel, requireCompact) {
					continue
				}
				result, err := s.tryAcquireAccountSlot(ctx, fresh.ID, fresh.Concurrency)
				if err == nil && result.Acquired {
					if sessionHash != "" {
						_ = s.setStickySessionAccountID(ctx, groupID, sessionHash, fresh.ID, openaiStickySessionTTL)
					}
					return s.newSelectionResult(ctx, fresh, true, result.ReleaseFunc, nil)
				}
			}
		}
	}

	// ============ Layer 3: Fallback wait ============
	sortAccountsByPriorityAndLastUsed(candidates, false)
	if requireCompact {
		candidates = prioritizeOpenAICompactAccounts(candidates)
	}
	for _, acc := range candidates {
		fresh := s.resolveFreshSchedulableOpenAIAccount(ctx, acc, requestedModel, TargetGroupAny, requireCompact)
		if fresh == nil {
			continue
		}
		fresh = s.recheckSelectedOpenAIAccountFromDB(ctx, fresh, requestedModel, TargetGroupAny, requireCompact)
		if fresh == nil {
			continue
		}
		if needsUpstreamCheck && s.isUpstreamModelRestrictedByChannel(ctx, *groupID, fresh, requestedModel, requireCompact) {
			continue
		}
		return s.newSelectionResult(ctx, fresh, false, nil, &AccountWaitPlan{
			AccountID:      fresh.ID,
			MaxConcurrency: fresh.Concurrency,
			Timeout:        cfg.FallbackWaitTimeout,
			MaxWaiting:     cfg.FallbackMaxWaiting,
		})
	}

	if requireCompact && baseCandidateCount > 0 {
		return nil, ErrNoAvailableCompactAccounts
	}
	return nil, ErrNoAvailableAccounts
}

func (s *OpenAIGatewayService) listSchedulableAccounts(ctx context.Context, groupID *int64, targetGroup AccountTargetGroup) ([]Account, error) {
	targetGroup = normalizeTargetGroup(targetGroup)
	if s.schedulerSnapshot != nil {
		accounts, _, err := s.schedulerSnapshot.ListSchedulableAccounts(ctx, groupID, PlatformOpenAI, false)
		if err != nil {
			return nil, err
		}
		if targetGroup == TargetGroupExhausted {
			accounts, err = s.mergeExhaustedFromBroadSource(ctx, groupID, accounts)
			if err != nil {
				return nil, err
			}
		}
		if targetGroup == TargetGroupAny {
			return accounts, nil
		}
		filtered := make([]Account, 0, len(accounts))
		for _, account := range accounts {
			if account.MatchesTargetGroup(targetGroup) {
				filtered = append(filtered, account)
			}
		}
		return filtered, nil
	}
	var accounts []Account
	var err error
	if s.cfg != nil && s.cfg.RunMode == config.RunModeSimple {
		accounts, err = s.accountRepo.ListSchedulableByPlatform(ctx, PlatformOpenAI)
	} else if groupID != nil {
		accounts, err = s.accountRepo.ListSchedulableByGroupIDAndPlatform(ctx, *groupID, PlatformOpenAI)
	} else {
		accounts, err = s.accountRepo.ListSchedulableUngroupedByPlatform(ctx, PlatformOpenAI)
	}
	if err != nil {
		return nil, fmt.Errorf("query accounts failed: %w", err)
	}
	if targetGroup == TargetGroupExhausted {
		accounts, err = s.mergeExhaustedFromBroadSource(ctx, groupID, accounts)
		if err != nil {
			return nil, err
		}
	}
	if targetGroup == TargetGroupAny {
		return accounts, nil
	}
	filtered := make([]Account, 0, len(accounts))
	for _, account := range accounts {
		if account.MatchesTargetGroup(targetGroup) {
			filtered = append(filtered, account)
		}
	}
	return filtered, nil
}

type openAIProjectionViewResult struct {
	state          *OpenAISchedulerBucketState
	view           OpenAIModelRoleView
	canonicalModel string
}

func (r *openAIProjectionViewResult) containsReserve(accountID int64) bool {
	return r != nil && containsOpenAIProjectionID(r.view.ReserveOverflowIDs, accountID)
}

func (r *openAIProjectionViewResult) containsExhausted(accountID int64) bool {
	return r != nil && containsOpenAIProjectionID(r.view.ExhaustedBaseIDs, accountID)
}

func (r *openAIProjectionViewResult) selectedGroupForAccount(accountID int64) string {
	if r.containsReserve(accountID) {
		return openAISelectedGroupReserve
	}
	if r.containsExhausted(accountID) {
		return string(TargetGroupExhausted)
	}
	return string(TargetGroupActive)
}

func (r *openAIProjectionViewResult) selectedGroupForTarget(accountID int64, targetGroup AccountTargetGroup) string {
	return r.selectedGroupForAccount(accountID)
}

func (r *openAIProjectionViewResult) bindingMatches(binding *openAIAffinityBinding) bool {
	if r == nil || binding == nil {
		return false
	}
	if binding.ProjectionVersion > 0 && binding.ProjectionVersion != r.state.ProjectionVersion {
		return false
	}
	if strings.TrimSpace(binding.ProjectionModelKey) != "" {
		if NormalizeOpenAIProjectionModelKey(binding.ProjectionModelKey) != r.canonicalModel {
			return false
		}
	}
	if builtAt := derefOpenAIProjectionBuiltAt(binding.ProjectionBuiltAt); !builtAt.IsZero() {
		if normalizeOpenAIProjectionBuiltAt(r.state.BuiltAt) != builtAt {
			return false
		}
	}
	return true
}

func (r *openAIProjectionViewResult) newAffinityBinding(accountID int64, selectedGroup string) *openAIAffinityBinding {
	binding := newOpenAIAffinityBinding(accountID, selectedGroup)
	if binding == nil || r == nil || r.state == nil {
		return binding
	}
	binding.ProjectionVersion = r.state.ProjectionVersion
	binding.ProjectionModelKey = r.canonicalModel
	binding.ProjectionBuiltAt = cloneOpenAIProjectionBuiltAt(&r.state.BuiltAt)
	return binding
}

func (s *OpenAIGatewayService) openAIProjectionAccountRepo() AccountRepository {
	if s == nil {
		return nil
	}
	if s.accountRepo != nil {
		return s.accountRepo
	}
	if s.schedulerSnapshot != nil {
		return s.schedulerSnapshot.accountRepo
	}
	return nil
}

func openAICatalogModelsContain(account Account, canonicalModel string) bool {
	for _, model := range canonicalizeOpenAIProjectionCatalog(parseOpenAIProjectionStringSlice(account.Extra[openAICapabilityCatalogModelsExtraKey])) {
		if model == canonicalModel {
			return true
		}
	}
	return false
}

func shouldRefreshOpenAIProjectionCatalogForAccount(account Account, canonicalModel string) bool {
	snapshot := buildOpenAIModelCapabilitySnapshot(account)
	if snapshot.DefaultAllow {
		return true
	}
	return wildcardRulesSupportProjectionModel(snapshot.WildcardRules, canonicalModel)
}

type openAIUnknownModelRefreshRequest struct {
	Bucket      string    `json:"bucket"`
	Model       string    `json:"model"`
	AccountIDs  []int64   `json:"account_ids"`
	RequestedAt time.Time `json:"requested_at"`
}

func openAIUnknownModelRefreshRequestKey(bucket SchedulerBucket, canonicalModel string) string {
	return bucket.String() + ":" + canonicalModel
}

func (s *OpenAIGatewayService) loadOpenAIProjectionInputsForModelMiss(ctx context.Context, bucket SchedulerBucket, requestedModel string) (*OpenAIProjectionInputs, string, bool, error) {
	if s == nil || s.schedulerSnapshot == nil {
		return nil, "", false, ErrSchedulerCacheNotReady
	}
	canonicalModel := NormalizeOpenAIProjectionModelKey(requestedModel)
	if canonicalModel == "" {
		return nil, "", false, nil
	}
	inputs, err := s.schedulerSnapshot.loadOpenAIProjectionInputs(ctx, bucket)
	if err != nil {
		return nil, canonicalModel, false, err
	}
	if inputs == nil {
		return nil, canonicalModel, false, nil
	}
	for _, model := range canonicalizeOpenAIProjectionCatalog(inputs.CanonicalCatalog) {
		if model == canonicalModel {
			return inputs, canonicalModel, true, nil
		}
	}
	return inputs, canonicalModel, false, nil
}

func (s *OpenAIGatewayService) requestUnknownOpenAIProjectionRefresh(ctx context.Context, bucket SchedulerBucket, canonicalModel string, accounts []Account) (bool, error) {
	if s == nil || s.cache == nil || canonicalModel == "" || len(accounts) == 0 {
		return false, nil
	}
	cache := s.openAICompanionBindingCache()
	if cache == nil {
		return false, nil
	}
	candidateIDs := make([]int64, 0, len(accounts))
	for _, account := range accounts {
		if !account.IsOpenAI() || openAICatalogModelsContain(account, canonicalModel) {
			continue
		}
		if !shouldRefreshOpenAIProjectionCatalogForAccount(account, canonicalModel) {
			continue
		}
		candidateIDs = append(candidateIDs, account.ID)
	}
	if len(candidateIDs) == 0 {
		return false, nil
	}
	bindingKey := openAIUnknownModelRefreshRequestKey(bucket, canonicalModel)
	if _, err := cache.GetOpenAICompanionBinding(ctx, bucket.GroupID, openAIUnknownModelRefreshRequestNamespace, bindingKey); err == nil {
		return true, nil
	}
	payload, err := json.Marshal(openAIUnknownModelRefreshRequest{
		Bucket:      bucket.String(),
		Model:       canonicalModel,
		AccountIDs:  candidateIDs,
		RequestedAt: time.Now().UTC(),
	})
	if err != nil {
		return false, err
	}
	if err := cache.SetOpenAICompanionBinding(ctx, bucket.GroupID, openAIUnknownModelRefreshRequestNamespace, bindingKey, string(payload), openAIUnknownModelRefreshRequestTTL); err != nil {
		return false, err
	}
	return true, nil
}

func (s *OpenAIGatewayService) getOpenAIProjectionView(ctx context.Context, groupID *int64, requestedModel string) (*openAIProjectionViewResult, error) {
	if s == nil || s.schedulerSnapshot == nil {
		return nil, ErrSchedulerCacheNotReady
	}
	bucket := s.schedulerSnapshot.bucketFor(groupID, PlatformOpenAI, SchedulerModeSingle)
	loadState := func(reason string) (*OpenAISchedulerBucketState, error) {
		state, hit, err := s.schedulerSnapshot.GetOpenAIBucketState(ctx, bucket)
		if err != nil {
			return nil, err
		}
		if hit && state != nil && state.Projection != nil {
			return state, nil
		}
		state, hit, err = s.schedulerSnapshot.RefreshOpenAIBucketState(ctx, bucket, reason)
		if err != nil {
			return nil, err
		}
		if !hit || state == nil || state.Projection == nil {
			return nil, ErrSchedulerCacheNotReady
		}
		return state, nil
	}

	state, err := loadState("openai_projection_cache_miss")
	if err != nil {
		return nil, err
	}
	view, ok := state.Projection.ViewForModel(requestedModel)
	if !ok {
		inputs, canonicalModel, sourceKnowsModel, err := s.loadOpenAIProjectionInputsForModelMiss(ctx, bucket, requestedModel)
		if err != nil {
			return nil, err
		}
		if !sourceKnowsModel {
			accountsAll := []Account(nil)
			if inputs != nil {
				accountsAll = inputs.AccountsAll
			}
			_, err = s.requestUnknownOpenAIProjectionRefresh(ctx, bucket, canonicalModel, accountsAll)
			if err != nil {
				return nil, err
			}
			return nil, ErrSchedulerCacheNotReady
		}
		var hit bool
		state, hit, err = s.schedulerSnapshot.RefreshOpenAIBucketState(ctx, bucket, "openai_projection_model_miss")
		if err != nil {
			return nil, err
		}
		if !hit || state == nil || state.Projection == nil {
			return nil, ErrSchedulerCacheNotReady
		}
		view, ok = state.Projection.ViewForModel(requestedModel)
		if !ok {
			return nil, ErrSchedulerCacheNotReady
		}
	}
	return &openAIProjectionViewResult{
		state:          state,
		view:           view,
		canonicalModel: view.CanonicalModel,
	}, nil
}

func (s *OpenAIGatewayService) openAIProjectionSourceKnowsModel(ctx context.Context, groupID *int64, requestedModel string) (bool, error) {
	if s == nil || s.schedulerSnapshot == nil {
		return false, ErrSchedulerCacheNotReady
	}
	canonicalModel := NormalizeOpenAIProjectionModelKey(requestedModel)
	if canonicalModel == "" {
		return false, ErrSchedulerCacheNotReady
	}
	if getNormalizedCodexModel(canonicalModel) != "" || normalizeCodexModel(canonicalModel) == canonicalModel {
		return true, nil
	}
	repo := s.openAIProjectionAccountRepo()
	if repo == nil {
		return false, ErrSchedulerCacheNotReady
	}
	snapshot := s.schedulerSnapshot
	if snapshot.accountRepo == nil {
		cloned := *snapshot
		cloned.accountRepo = repo
		snapshot = &cloned
	}
	bucket := snapshot.bucketFor(groupID, PlatformOpenAI, SchedulerModeSingle)
	inputs, err := snapshot.loadOpenAIProjectionInputs(ctx, bucket)
	if err != nil {
		return false, err
	}
	for _, model := range canonicalizeOpenAIProjectionCatalog(inputs.CanonicalCatalog) {
		if model == canonicalModel {
			return true, nil
		}
	}
	return false, nil
}

func (s *OpenAIGatewayService) getOpenAIProjectionViewWithLegacyFallback(ctx context.Context, groupID *int64, requestedModel string) (*openAIProjectionViewResult, bool, error) {
	if s == nil || s.schedulerSnapshot == nil || strings.TrimSpace(requestedModel) == "" {
		return nil, true, nil
	}
	projectionView, err := s.getOpenAIProjectionView(ctx, groupID, requestedModel)
	if err == nil {
		return projectionView, false, nil
	}
	if !errors.Is(err, ErrSchedulerCacheNotReady) {
		return nil, false, err
	}
	sourceKnowsModel, sourceErr := s.openAIProjectionSourceKnowsModel(ctx, groupID, requestedModel)
	if sourceErr != nil {
		return nil, false, err
	}
	if !sourceKnowsModel {
		return nil, false, ErrSchedulerCacheNotReady
	}
	return nil, true, nil
}

func (s *OpenAIGatewayService) loadOpenAIProjectionAccounts(ctx context.Context, state *OpenAISchedulerBucketState, ids []int64) ([]Account, error) {
	if state == nil {
		return nil, ErrSchedulerCacheNotReady
	}
	if state.ProjectionAccounts == nil {
		return nil, ErrSchedulerCacheNotReady
	}
	byID := make(map[int64]Account, len(state.ProjectionAccounts))
	for _, account := range state.ProjectionAccounts {
		if account == nil {
			continue
		}
		byID[account.ID] = *account
	}
	out := make([]Account, 0, len(ids))
	for _, accountID := range ids {
		if account, ok := byID[accountID]; ok {
			out = append(out, account)
			continue
		}
		return nil, ErrSchedulerCacheNotReady
	}
	return out, nil
}

func buildOpenAIReserveCandidatePool(accounts []Account) []Account {
	pool := make([]Account, 0, len(accounts))
	for _, account := range accounts {
		if account.IsOpenAIReserveCandidate() {
			pool = append(pool, account)
		}
	}
	sort.SliceStable(pool, func(i, j int) bool {
		iScore := pool[i].OpenAIRemainingQuotaScore()
		jScore := pool[j].OpenAIRemainingQuotaScore()
		if iScore != jScore {
			return iScore > jScore
		}
		return pool[i].ID < pool[j].ID
	})
	return pool
}

func buildOpenAIModelSubsetReserveCandidatePool(accounts []Account) []Account {
	pool := make([]Account, 0, len(accounts))
	for _, account := range accounts {
		if !account.IsOpenAI() || !account.IsSchedulableForTargetGroup(TargetGroupActive) {
			continue
		}
		pool = append(pool, account)
	}
	sort.SliceStable(pool, func(i, j int) bool {
		iScore := pool[i].OpenAIRemainingQuotaScore()
		jScore := pool[j].OpenAIRemainingQuotaScore()
		if iScore != jScore {
			return iScore > jScore
		}
		return pool[i].ID < pool[j].ID
	})
	return pool
}

func calculateOpenAIConcurrentCapacity(accounts []Account) int {
	total := 0
	for _, account := range accounts {
		if account.Concurrency > 0 {
			total += account.Concurrency
		}
	}
	return total
}

func buildOpenAIReservePool(activeAccounts []Account, exhaustedAccounts []Account) []Account {
	reserveCandidates := buildOpenAIReserveCandidatePool(activeAccounts)
	if len(reserveCandidates) == 0 {
		return nil
	}

	exhaustedCapacity := calculateOpenAIConcurrentCapacity(exhaustedAccounts)
	activeFreeCapacity := calculateOpenAIConcurrentCapacity(reserveCandidates)
	totalCapacity := exhaustedCapacity + activeFreeCapacity
	if totalCapacity <= 0 {
		return nil
	}

	targetCapacity := (totalCapacity*60 + 99) / 100
	reserveNeeded := targetCapacity - exhaustedCapacity
	if reserveNeeded <= 0 {
		return nil
	}

	reservePool := make([]Account, 0, len(reserveCandidates))
	reserveCapacity := 0
	for _, account := range reserveCandidates {
		reservePool = append(reservePool, account)
		if account.Concurrency > 0 {
			reserveCapacity += account.Concurrency
		}
		if reserveCapacity >= reserveNeeded {
			break
		}
	}
	return reservePool
}

func buildOpenAIReserveOverflowPoolFromCandidates(reserveCandidates []Account, exhaustedAccounts []Account) []Account {
	if len(reserveCandidates) == 0 {
		return nil
	}

	exhaustedCapacity := calculateOpenAIConcurrentCapacity(exhaustedAccounts)
	activeFreeCapacity := calculateOpenAIConcurrentCapacity(reserveCandidates)
	totalCapacity := exhaustedCapacity + activeFreeCapacity
	if totalCapacity <= 0 {
		return nil
	}

	targetCapacity := (totalCapacity*60 + 99) / 100
	reserveNeeded := targetCapacity - exhaustedCapacity
	if reserveNeeded <= 0 {
		return nil
	}

	reservePool := make([]Account, 0, len(reserveCandidates))
	reserveCapacity := 0
	for i := len(reserveCandidates) - 1; i >= 0; i-- {
		account := reserveCandidates[i]
		reservePool = append(reservePool, account)
		if account.Concurrency > 0 {
			reserveCapacity += account.Concurrency
		}
		if reserveCapacity >= reserveNeeded {
			break
		}
	}
	return reservePool
}

func buildOpenAILegacyReserveOverflowPool(activeAccounts []Account, exhaustedAccounts []Account) []Account {
	return buildOpenAIReserveOverflowPoolFromCandidates(buildOpenAIReserveCandidatePool(activeAccounts), exhaustedAccounts)
}

func buildOpenAIReserveOverflowPool(activeAccounts []Account, exhaustedAccounts []Account) []Account {
	reserveCandidates := buildOpenAIReserveCandidatePool(activeAccounts)
	if len(reserveCandidates) == 0 {
		reserveCandidates = buildOpenAIModelSubsetReserveCandidatePool(activeAccounts)
	}
	return buildOpenAIReserveOverflowPoolFromCandidates(reserveCandidates, exhaustedAccounts)
}

func shouldRouteExhaustedOverflowToReserve(exhaustedAccounts []Account, reserveAccounts []Account, loadMap map[int64]*AccountLoadInfo) bool {
	if calculateOpenAIConcurrentCapacity(reserveAccounts) <= 0 {
		return false
	}

	exhaustedCapacity := calculateOpenAIConcurrentCapacity(exhaustedAccounts)
	if exhaustedCapacity <= 0 {
		return len(exhaustedAccounts) == 0
	}

	usagePercent := calculateOpenAIConcurrentUsagePercent(exhaustedAccounts, loadMap)
	return usagePercent > 60
}

func (s *OpenAIGatewayService) listOpenAIExhaustedWithReserveOverlay(ctx context.Context, groupID *int64, requestedModel string) ([]Account, []Account, error) {
	projectionView, err := s.getOpenAIProjectionView(ctx, groupID, requestedModel)
	if err != nil {
		return nil, nil, err
	}
	exhaustedAccounts, err := s.loadOpenAIProjectionAccounts(ctx, projectionView.state, projectionView.view.ExhaustedBaseIDs)
	if err != nil {
		return nil, nil, err
	}
	reserveAccounts, err := s.loadOpenAIProjectionAccounts(ctx, projectionView.state, projectionView.view.ReserveOverflowIDs)
	if err != nil {
		return nil, nil, err
	}
	return exhaustedAccounts, reserveAccounts, nil
}

func calculateOpenAIConcurrentUsagePercent(accounts []Account, loadMap map[int64]*AccountLoadInfo) float64 {
	totalCapacity := calculateOpenAIConcurrentCapacity(accounts)
	if totalCapacity <= 0 || len(accounts) == 0 {
		return 0
	}

	currentAvailable := true
	totalCurrent := 0
	weightedLoad := 0
	capacityAccounts := 0
	for _, account := range accounts {
		if account.Concurrency <= 0 {
			continue
		}
		capacityAccounts++
		loadInfo := loadMap[account.ID]
		if loadInfo == nil {
			currentAvailable = false
			weightedLoad += 100 * account.Concurrency
			continue
		}
		totalCurrent += loadInfo.CurrentConcurrency
		weightedLoad += loadInfo.LoadRate * account.Concurrency
	}

	if capacityAccounts == 0 {
		return 0
	}
	if currentAvailable {
		return float64(totalCurrent) * 100 / float64(totalCapacity)
	}
	return float64(weightedLoad) / float64(totalCapacity)
}

func (s *OpenAIGatewayService) mergeExhaustedFromBroadSource(ctx context.Context, groupID *int64, base []Account) ([]Account, error) {
	if s == nil || s.accountRepo == nil {
		return base, nil
	}
	broad, err := s.listOpenAIAccountsFromBroadSource(ctx, groupID)
	if err != nil {
		return nil, fmt.Errorf("query accounts failed: %w", err)
	}
	merged, _ := mergeOpenAIExhaustedAccountsFromBroadSource(base, broad)
	return merged, nil
}

func (s *OpenAIGatewayService) listOpenAIAccountsFromBroadSource(ctx context.Context, groupID *int64) ([]Account, error) {
	repo := s.openAIProjectionAccountRepo()
	if s == nil || repo == nil {
		return nil, nil
	}
	if s.cfg != nil && s.cfg.RunMode == config.RunModeSimple {
		return repo.ListByPlatform(ctx, PlatformOpenAI)
	}
	if groupID != nil {
		accounts, err := repo.ListByGroup(ctx, *groupID)
		if err != nil {
			return nil, err
		}
		filtered := make([]Account, 0, len(accounts))
		for _, acc := range accounts {
			if acc.Platform == PlatformOpenAI {
				filtered = append(filtered, acc)
			}
		}
		return filtered, nil
	}
	accounts, err := repo.ListByPlatform(ctx, PlatformOpenAI)
	if err != nil {
		return nil, err
	}
	filtered := make([]Account, 0, len(accounts))
	for _, acc := range accounts {
		if len(acc.AccountGroups) == 0 && len(acc.GroupIDs) == 0 && len(acc.Groups) == 0 {
			filtered = append(filtered, acc)
		}
	}
	return filtered, nil
}

func (s *OpenAIGatewayService) tryAcquireAccountSlot(ctx context.Context, accountID int64, maxConcurrency int) (*AcquireResult, error) {
	if s.concurrencyService == nil {
		return &AcquireResult{Acquired: true, ReleaseFunc: func() {}}, nil
	}
	return s.concurrencyService.AcquireAccountSlot(ctx, accountID, maxConcurrency)
}

func (s *OpenAIGatewayService) resolveFreshSchedulableOpenAIAccount(ctx context.Context, account *Account, requestedModel string, targetGroup AccountTargetGroup, requireCompact bool) *Account {
	if account == nil {
		return nil
	}
	targetGroup = normalizeTargetGroup(targetGroup)

	fresh := account
	if s.schedulerSnapshot != nil {
		current, err := s.getSchedulableAccount(ctx, account.ID)
		if err != nil || current == nil {
			return nil
		}
		fresh = current
	}

	if !fresh.IsOpenAI() || !fresh.MatchesTargetGroup(targetGroup) || !fresh.IsSchedulableForTargetGroup(targetGroup) {
		return nil
	}
	if requestedModel != "" && !fresh.IsModelSupported(requestedModel) {
		return nil
	}
	if requireCompact && openAICompactSupportTier(fresh) == 0 {
		return nil
	}
	return fresh
}

func (s *OpenAIGatewayService) resolveFreshOpenAIExhaustedAccount(ctx context.Context, groupID *int64, account *Account, requestedModel string, requireCompact bool) *Account {
	if account == nil {
		return nil
	}
	projectionView := (*openAIProjectionViewResult)(nil)
	allowLegacyFallback := true
	if s != nil && s.schedulerSnapshot != nil && strings.TrimSpace(requestedModel) != "" {
		var err error
		projectionView, allowLegacyFallback, err = s.getOpenAIProjectionViewWithLegacyFallback(ctx, groupID, requestedModel)
		if err != nil {
			return nil
		}
	}
	if s == nil || s.schedulerSnapshot == nil || strings.TrimSpace(requestedModel) == "" || (projectionView == nil && allowLegacyFallback) {
		fresh := account
		if s != nil && s.schedulerSnapshot != nil {
			current, err := s.getSchedulableAccount(ctx, account.ID)
			if err != nil || current == nil {
				return nil
			}
			fresh = current
		}
		if !fresh.IsOpenAI() || !fresh.MatchesTargetGroup(TargetGroupExhausted) || shouldClearStickySessionForTargetGroup(fresh, requestedModel, TargetGroupExhausted) || (requireCompact && openAICompactSupportTier(fresh) == 0) {
			return nil
		}
		return fresh
	}
	if projectionView == nil {
		return nil
	}
	if !projectionView.containsExhausted(account.ID) {
		return nil
	}

	fresh := account
	if s.schedulerSnapshot != nil {
		current, err := s.getSchedulableAccount(ctx, account.ID)
		if err != nil || current == nil {
			return nil
		}
		fresh = current
	}

	if !fresh.IsOpenAI() || shouldClearStickySessionForTargetGroup(fresh, requestedModel, TargetGroupExhausted) || (requireCompact && openAICompactSupportTier(fresh) == 0) {
		return nil
	}
	return fresh
}

func (s *OpenAIGatewayService) resolveFreshProjectedOpenAIAccount(ctx context.Context, groupID *int64, account *Account, requestedModel string, selectedGroup string, targetGroup AccountTargetGroup, requireCompact bool) *Account {
	switch normalizeOpenAISelectedGroup(selectedGroup) {
	case openAISelectedGroupReserve:
		return s.resolveFreshOpenAIReserveAccount(ctx, groupID, account, requestedModel, targetGroup, requireCompact)
	case string(TargetGroupExhausted):
		return s.resolveFreshOpenAIExhaustedAccount(ctx, groupID, account, requestedModel, requireCompact)
	default:
		return s.resolveFreshSchedulableOpenAIAccount(ctx, account, requestedModel, targetGroup, requireCompact)
	}
}

func (s *OpenAIGatewayService) resolveFreshOpenAIReserveAccount(ctx context.Context, groupID *int64, account *Account, requestedModel string, targetGroup AccountTargetGroup, requireCompact bool) *Account {
	if account == nil {
		return nil
	}
	requestedModel = strings.TrimSpace(requestedModel)
	targetGroup = normalizeTargetGroup(targetGroup)
	projectionView := (*openAIProjectionViewResult)(nil)
	allowLegacyFallback := true
	if s != nil && s.schedulerSnapshot != nil && requestedModel != "" {
		var err error
		projectionView, allowLegacyFallback, err = s.getOpenAIProjectionViewWithLegacyFallback(ctx, groupID, requestedModel)
		if err != nil {
			return nil
		}
	}
	resolveLegacyReserve := func() *Account {
		fresh := account
		if s != nil && s.schedulerSnapshot != nil {
			current, err := s.getSchedulableAccount(ctx, account.ID)
			if err != nil || current == nil {
				return nil
			}
			fresh = current
		}
		if !fresh.IsOpenAI() || !fresh.IsOpenAIReserveCandidate() {
			return nil
		}
		if requestedModel != "" && !fresh.IsModelSupported(requestedModel) {
			return nil
		}
		if requireCompact && openAICompactSupportTier(fresh) == 0 {
			return nil
		}
		return fresh
	}
	if s == nil || s.schedulerSnapshot == nil || requestedModel == "" {
		return resolveLegacyReserve()
	}
	if projectionView == nil {
		if allowLegacyFallback && targetGroup == TargetGroupExhausted {
			return resolveLegacyReserve()
		}
		return nil
	}
	if !projectionView.containsReserve(account.ID) {
		return nil
	}

	fresh := account
	if s.schedulerSnapshot != nil {
		current, err := s.getSchedulableAccount(ctx, account.ID)
		if err != nil || current == nil {
			return nil
		}
		fresh = current
	}

	if !isCurrentOpenAIReserveSelectionValid(fresh, requestedModel, targetGroup) || (requireCompact && openAICompactSupportTier(fresh) == 0) {
		return nil
	}
	return fresh
}

func isCurrentOpenAIReserveSelectionValid(account *Account, requestedModel string, targetGroup AccountTargetGroup) bool {
	if account == nil || !account.IsOpenAI() {
		return false
	}
	if !account.IsSchedulableForTargetGroup(TargetGroupActive) {
		return false
	}
	return !shouldClearStickySessionForTargetGroup(account, requestedModel, targetGroup)
}

func (s *OpenAIGatewayService) isCurrentOpenAIReserveOverlayAccount(ctx context.Context, groupID *int64, requestedModel string, account *Account) bool {
	if s == nil || account == nil || !account.IsOpenAIReserveCandidate() {
		return false
	}
	if s.schedulerSnapshot == nil || strings.TrimSpace(requestedModel) == "" {
		return false
	}
	projectionView, err := s.getOpenAIProjectionView(ctx, groupID, requestedModel)
	if err != nil {
		return false
	}
	return projectionView.containsReserve(account.ID)
}

func (s *OpenAIGatewayService) isOpenAILegacyReserveOverlayAccount(ctx context.Context, groupID *int64, requestedModel string, account *Account) bool {
	if s == nil || account == nil || !account.IsOpenAIReserveCandidate() {
		return false
	}
	reserveAccounts, err := s.listCurrentOpenAILegacyReserveOverlay(ctx, groupID, requestedModel)
	if err != nil {
		return false
	}
	for _, reserveAccount := range reserveAccounts {
		if reserveAccount.ID == account.ID {
			return true
		}
	}
	return false
}

func (s *OpenAIGatewayService) listCurrentOpenAILegacyReserveOverlay(ctx context.Context, groupID *int64, requestedModel string) ([]Account, error) {
	repo := s.openAIProjectionAccountRepo()
	if s == nil || repo == nil {
		return nil, ErrSchedulerCacheNotReady
	}
	var (
		accounts []Account
		err      error
	)
	if s.cfg != nil && s.cfg.RunMode == config.RunModeSimple {
		accounts, err = repo.ListSchedulableByPlatform(ctx, PlatformOpenAI)
	} else if groupID != nil {
		accounts, err = repo.ListSchedulableByGroupIDAndPlatform(ctx, *groupID, PlatformOpenAI)
	} else {
		accounts, err = repo.ListSchedulableUngroupedByPlatform(ctx, PlatformOpenAI)
	}
	if err != nil {
		return nil, err
	}
	activeAccounts := make([]Account, 0, len(accounts))
	exhaustedAccounts := make([]Account, 0, len(accounts))
	for _, candidate := range accounts {
		if !candidate.IsOpenAI() {
			continue
		}
		if requestedModel != "" && !candidate.IsModelSupported(requestedModel) {
			continue
		}
		if candidate.IsSchedulableForTargetGroup(TargetGroupExhausted) {
			exhaustedAccounts = append(exhaustedAccounts, candidate)
			continue
		}
		if candidate.IsSchedulableForTargetGroup(TargetGroupActive) {
			activeAccounts = append(activeAccounts, candidate)
		}
	}
	return buildOpenAILegacyReserveOverflowPool(activeAccounts, exhaustedAccounts), nil
}

func (s *OpenAIGatewayService) isOpenAIReservePreviousResponseAnchor(ctx context.Context, groupID *int64, requestedModel string, previousResponseID string) bool {
	if s == nil {
		return false
	}
	responseID := strings.TrimSpace(previousResponseID)
	if responseID == "" {
		return false
	}
	store := s.getOpenAIWSStateStore()
	if store == nil {
		return false
	}
	binding, ok := getOpenAIWSResponseAffinityBinding(store, derefGroupID(groupID), responseID)
	accountID, err := store.GetResponseAccount(ctx, derefGroupID(groupID), responseID)
	if err != nil || accountID <= 0 {
		return false
	}
	if ok && isOpenAIReserveAffinityBinding(binding) && (strings.TrimSpace(requestedModel) == "" || s.schedulerSnapshot == nil) {
		return true
	}
	projectionView, err := s.getOpenAIProjectionView(ctx, groupID, requestedModel)
	if err != nil {
		return false
	}
	if binding != nil && !projectionView.bindingMatches(binding) {
		return false
	}
	if projectionView.containsReserve(accountID) {
		return true
	}
	return false
}

func (s *OpenAIGatewayService) deleteOpenAIWSResponseAccount(ctx context.Context, groupID *int64, previousResponseID string) {
	if s == nil {
		return
	}
	responseID := strings.TrimSpace(previousResponseID)
	if responseID == "" {
		return
	}
	store := s.getOpenAIWSStateStore()
	if store == nil {
		return
	}
	_ = store.DeleteResponseAccount(ctx, derefGroupID(groupID), responseID)
}

func (s *OpenAIGatewayService) prepareSelectedOpenAIAccount(ctx context.Context, groupID *int64, account *Account, requestedModel string, targetGroup AccountTargetGroup, needsUpstreamCheck bool, requireCompact bool) *Account {
	fresh := s.resolveFreshSchedulableOpenAIAccount(ctx, account, requestedModel, targetGroup, requireCompact)
	if fresh == nil {
		return nil
	}
	fresh = s.recheckSelectedOpenAIAccountFromDB(ctx, fresh, requestedModel, targetGroup, requireCompact)
	if fresh == nil {
		return nil
	}
	if needsUpstreamCheck && s.isUpstreamModelRestrictedByChannel(ctx, derefGroupID(groupID), fresh, requestedModel, requireCompact) {
		return nil
	}
	return fresh
}

func (s *OpenAIGatewayService) prepareStickySelectedOpenAIAccount(ctx context.Context, groupID *int64, account *Account, requestedModel string, targetGroup AccountTargetGroup, needsUpstreamCheck bool, requireCompact bool) (*Account, bool) {
	fresh := s.resolveFreshSchedulableOpenAIAccount(ctx, account, requestedModel, targetGroup, requireCompact)
	if fresh == nil {
		return nil, false
	}
	fresh = s.recheckSelectedOpenAIAccountFromDB(ctx, fresh, requestedModel, targetGroup, requireCompact)
	if fresh == nil {
		return nil, true
	}
	if needsUpstreamCheck && s.isUpstreamModelRestrictedByChannel(ctx, derefGroupID(groupID), fresh, requestedModel, requireCompact) {
		return nil, true
	}
	return fresh, false
}

func (s *OpenAIGatewayService) recheckSelectedOpenAIAccountFromDB(ctx context.Context, account *Account, requestedModel string, targetGroup AccountTargetGroup, requireCompact bool) *Account {
	if account == nil {
		return nil
	}
	targetGroup = normalizeTargetGroup(targetGroup)
	if s.schedulerSnapshot == nil || s.accountRepo == nil {
		return account
	}

	latest, err := s.accountRepo.GetByID(ctx, account.ID)
	if err != nil || latest == nil {
		return nil
	}
	if !latest.IsOpenAI() || !latest.MatchesTargetGroup(targetGroup) || !latest.IsSchedulableForTargetGroup(targetGroup) {
		return nil
	}
	if requestedModel != "" && !latest.IsModelSupported(requestedModel) {
		return nil
	}
	if requireCompact && openAICompactSupportTier(latest) == 0 {
		return nil
	}
	return latest
}

func (s *OpenAIGatewayService) recheckSelectedOpenAIExhaustedAccountFromDB(ctx context.Context, groupID *int64, account *Account, requestedModel string, requireCompact bool) *Account {
	if account == nil {
		return nil
	}
	projectionView := (*openAIProjectionViewResult)(nil)
	allowLegacyFallback := true
	if s != nil && s.schedulerSnapshot != nil && strings.TrimSpace(requestedModel) != "" {
		var err error
		projectionView, allowLegacyFallback, err = s.getOpenAIProjectionViewWithLegacyFallback(ctx, groupID, requestedModel)
		if err != nil {
			return nil
		}
	}
	if s == nil || s.schedulerSnapshot == nil || strings.TrimSpace(requestedModel) == "" || (projectionView == nil && allowLegacyFallback) {
		if s == nil || s.schedulerSnapshot == nil || s.accountRepo == nil {
			return account
		}
		latest, err := s.accountRepo.GetByID(ctx, account.ID)
		if err != nil || latest == nil {
			return nil
		}
		if !latest.IsOpenAI() || !latest.MatchesTargetGroup(TargetGroupExhausted) || shouldClearStickySessionForTargetGroup(latest, requestedModel, TargetGroupExhausted) {
			return nil
		}
		return latest
	}
	if projectionView == nil {
		return nil
	}
	if !projectionView.containsExhausted(account.ID) {
		return nil
	}
	if s.schedulerSnapshot == nil || s.accountRepo == nil {
		if !isOpenAIAccountEligibleForRequest(account, requestedModel, requireCompact) {
			return nil
		}
		return account
	}

	latest, err := s.accountRepo.GetByID(ctx, account.ID)
	if err != nil || latest == nil {
		return nil
	}
	if !latest.IsOpenAI() || shouldClearStickySessionForTargetGroup(latest, requestedModel, TargetGroupExhausted) || (requireCompact && openAICompactSupportTier(latest) == 0) {
		return nil
	}
	return latest
}

func (s *OpenAIGatewayService) recheckSelectedOpenAIReserveAccountFromDB(ctx context.Context, groupID *int64, account *Account, requestedModel string, targetGroup AccountTargetGroup) *Account {
	if account == nil {
		return nil
	}
	requestedModel = strings.TrimSpace(requestedModel)
	targetGroup = normalizeTargetGroup(targetGroup)
	projectionView := (*openAIProjectionViewResult)(nil)
	allowLegacyFallback := true
	if s != nil && s.schedulerSnapshot != nil && requestedModel != "" {
		var err error
		projectionView, allowLegacyFallback, err = s.getOpenAIProjectionViewWithLegacyFallback(ctx, groupID, requestedModel)
		if err != nil {
			return nil
		}
	}
	recheckLegacyReserve := func() *Account {
		if s == nil || s.schedulerSnapshot == nil || s.accountRepo == nil {
			return account
		}
		latest, err := s.accountRepo.GetByID(ctx, account.ID)
		if err != nil || latest == nil {
			return nil
		}
		if !latest.IsOpenAI() || !latest.IsOpenAIReserveCandidate() {
			return nil
		}
		if requestedModel != "" && !latest.IsModelSupported(requestedModel) {
			return nil
		}
		return latest
	}
	if s == nil || s.schedulerSnapshot == nil || requestedModel == "" {
		return recheckLegacyReserve()
	}
	if projectionView == nil {
		if allowLegacyFallback && targetGroup == TargetGroupExhausted {
			return recheckLegacyReserve()
		}
		return nil
	}
	if !projectionView.containsReserve(account.ID) {
		return nil
	}
	if s.schedulerSnapshot == nil || s.accountRepo == nil {
		return account
	}

	latest, err := s.accountRepo.GetByID(ctx, account.ID)
	if err != nil || latest == nil {
		return nil
	}
	if !isCurrentOpenAIReserveSelectionValid(latest, requestedModel, targetGroup) {
		return nil
	}
	return latest
}

func (s *OpenAIGatewayService) recheckSelectedProjectedOpenAIAccountFromDB(ctx context.Context, groupID *int64, account *Account, requestedModel string, selectedGroup string, targetGroup AccountTargetGroup) *Account {
	switch normalizeOpenAISelectedGroup(selectedGroup) {
	case openAISelectedGroupReserve:
		return s.recheckSelectedOpenAIReserveAccountFromDB(ctx, groupID, account, requestedModel, targetGroup)
	case string(TargetGroupExhausted):
		return s.recheckSelectedOpenAIExhaustedAccountFromDB(ctx, groupID, account, requestedModel, false)
	default:
		return s.recheckSelectedOpenAIAccountFromDB(ctx, account, requestedModel, targetGroup, false)
	}
}

func (s *OpenAIGatewayService) getSchedulableAccount(ctx context.Context, accountID int64) (*Account, error) {
	var (
		account *Account
		err     error
	)
	if s.schedulerSnapshot != nil {
		account, err = s.schedulerSnapshot.GetAccount(ctx, accountID)
	} else {
		account, err = s.accountRepo.GetByID(ctx, accountID)
	}
	if err != nil || account == nil {
		return account, err
	}
	return account, nil
}

func (s *OpenAIGatewayService) hydrateSelectedAccount(ctx context.Context, account *Account) (*Account, error) {
	if account == nil || s.schedulerSnapshot == nil {
		return account, nil
	}
	hydrated, err := s.schedulerSnapshot.GetAccount(ctx, account.ID)
	if err != nil {
		return nil, err
	}
	if hydrated == nil {
		return nil, fmt.Errorf("selected openai account %d not found during hydration", account.ID)
	}
	return hydrated, nil
}

func (s *OpenAIGatewayService) newSelectionResult(ctx context.Context, account *Account, acquired bool, release func(), waitPlan *AccountWaitPlan) (*AccountSelectionResult, error) {
	hydrated, err := s.hydrateSelectedAccount(ctx, account)
	if err != nil {
		return nil, err
	}
	return &AccountSelectionResult{
		Account:     hydrated,
		Acquired:    acquired,
		ReleaseFunc: release,
		WaitPlan:    waitPlan,
	}, nil
}

func (s *OpenAIGatewayService) schedulingConfig() config.GatewaySchedulingConfig {
	if s.cfg != nil {
		return s.cfg.Gateway.Scheduling
	}
	return config.GatewaySchedulingConfig{
		StickySessionMaxWaiting:  3,
		StickySessionWaitTimeout: 45 * time.Second,
		FallbackWaitTimeout:      30 * time.Second,
		FallbackMaxWaiting:       100,
		LoadBatchEnabled:         true,
		SlotCleanupInterval:      30 * time.Second,
	}
}

// GetAccessToken gets the access token for an OpenAI account
func (s *OpenAIGatewayService) GetAccessToken(ctx context.Context, account *Account) (string, string, error) {
	switch account.Type {
	case AccountTypeOAuth:
		// 使用 TokenProvider 获取缓存的 token
		if s.openAITokenProvider != nil {
			accessToken, err := s.openAITokenProvider.GetAccessToken(ctx, account)
			if err != nil {
				return "", "", err
			}
			return accessToken, "oauth", nil
		}
		// 降级：TokenProvider 未配置时直接从账号读取
		accessToken := account.GetOpenAIAccessToken()
		if accessToken == "" {
			return "", "", errors.New("access_token not found in credentials")
		}
		return accessToken, "oauth", nil
	case AccountTypeAPIKey:
		apiKey := account.GetOpenAIApiKey()
		if apiKey == "" {
			return "", "", errors.New("api_key not found in credentials")
		}
		return apiKey, "apikey", nil
	default:
		return "", "", fmt.Errorf("unsupported account type: %s", account.Type)
	}
}

func (s *OpenAIGatewayService) shouldFailoverUpstreamError(statusCode int) bool {
	switch statusCode {
	case 401, 402, 403, 429, 529:
		return true
	default:
		return statusCode >= 500
	}
}

func (s *OpenAIGatewayService) shouldFailoverOpenAIUpstreamResponse(statusCode int, upstreamMsg string, upstreamBody []byte) bool {
	if s.shouldFailoverUpstreamError(statusCode) {
		return true
	}
	return isOpenAITransientProcessingError(statusCode, upstreamMsg, upstreamBody)
}

func (s *OpenAIGatewayService) handleFailoverSideEffects(ctx context.Context, resp *http.Response, account *Account) {
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	s.rateLimitService.HandleUpstreamError(ctx, account, resp.StatusCode, resp.Header, body)
}

// Forward forwards request to OpenAI API
func (s *OpenAIGatewayService) Forward(ctx context.Context, c *gin.Context, account *Account, body []byte) (*OpenAIForwardResult, error) {
	startTime := time.Now()

	restrictionResult := s.detectCodexClientRestriction(c, account)
	apiKeyID := getAPIKeyIDFromContext(c)
	logCodexCLIOnlyDetection(ctx, c, account, apiKeyID, restrictionResult, body)
	if restrictionResult.Enabled && !restrictionResult.Matched {
		c.JSON(http.StatusForbidden, gin.H{
			"error": gin.H{
				"type":    "forbidden_error",
				"message": "This account only allows Codex official clients",
			},
		})
		return nil, errors.New("codex_cli_only restriction: only codex official clients are allowed")
	}

	originalBody := body
	reqModel, reqStream, promptCacheKey := extractOpenAIRequestMetaFromBody(body)
	originalModel := reqModel

	isCodexCLI := openai.IsCodexOfficialClientByHeaders(c.GetHeader("User-Agent"), c.GetHeader("originator")) || (s.cfg != nil && s.cfg.Gateway.ForceCodexCLI)
	wsDecision := s.getOpenAIWSProtocolResolver().Resolve(account)
	clientTransport := GetOpenAIClientTransport(c)
	// 仅允许 WS 入站请求走 WS 上游，避免出现 HTTP -> WS 协议混用。
	wsDecision = resolveOpenAIWSDecisionByClientTransport(wsDecision, clientTransport)
	if c != nil {
		c.Set("openai_ws_transport_decision", string(wsDecision.Transport))
		c.Set("openai_ws_transport_reason", wsDecision.Reason)
	}
	if wsDecision.Transport == OpenAIUpstreamTransportResponsesWebsocketV2 {
		logOpenAIWSModeDebug(
			"selected account_id=%d account_type=%s transport=%s reason=%s model=%s stream=%v",
			account.ID,
			account.Type,
			normalizeOpenAIWSLogValue(string(wsDecision.Transport)),
			normalizeOpenAIWSLogValue(wsDecision.Reason),
			reqModel,
			reqStream,
		)
	}
	// 当前仅支持 WSv2；WSv1 命中时直接返回错误，避免出现“配置可开但行为不确定”。
	if wsDecision.Transport == OpenAIUpstreamTransportResponsesWebsocket {
		if c != nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"error": gin.H{
					"type":    "invalid_request_error",
					"message": "OpenAI WSv1 is temporarily unsupported. Please enable responses_websockets_v2.",
				},
			})
		}
		return nil, errors.New("openai ws v1 is temporarily unsupported; use ws v2")
	}
	passthroughEnabled := account.IsOpenAIPassthroughEnabled()
	if passthroughEnabled {
		// 透传分支只需要轻量提取字段，避免热路径全量 Unmarshal。
		reasoningEffort := extractOpenAIReasoningEffortFromBody(body, reqModel)
		return s.forwardOpenAIPassthrough(ctx, c, account, originalBody, reqModel, reasoningEffort, reqStream, startTime)
	}

	reqBody, err := getOpenAIRequestBodyMap(c, body)
	if err != nil {
		return nil, err
	}

	if v, ok := reqBody["model"].(string); ok {
		reqModel = v
		originalModel = reqModel
	}
	if v, ok := reqBody["stream"].(bool); ok {
		reqStream = v
	}
	if promptCacheKey == "" {
		if v, ok := reqBody["prompt_cache_key"].(string); ok {
			promptCacheKey = strings.TrimSpace(v)
		}
	}

	// Track if body needs re-serialization
	bodyModified := false
	// 单字段补丁快速路径：只要整个变更集最终可归约为同一路径的 set/delete，就避免全量 Marshal。
	patchDisabled := false
	patchHasOp := false
	patchDelete := false
	patchPath := ""
	var patchValue any
	markPatchSet := func(path string, value any) {
		if strings.TrimSpace(path) == "" {
			patchDisabled = true
			return
		}
		if patchDisabled {
			return
		}
		if !patchHasOp {
			patchHasOp = true
			patchDelete = false
			patchPath = path
			patchValue = value
			return
		}
		if patchDelete || patchPath != path {
			patchDisabled = true
			return
		}
		patchValue = value
	}
	markPatchDelete := func(path string) {
		if strings.TrimSpace(path) == "" {
			patchDisabled = true
			return
		}
		if patchDisabled {
			return
		}
		if !patchHasOp {
			patchHasOp = true
			patchDelete = true
			patchPath = path
			return
		}
		if !patchDelete || patchPath != path {
			patchDisabled = true
		}
	}
	disablePatch := func() {
		patchDisabled = true
	}

	// 非透传模式下，instructions 为空时注入默认指令。
	if isInstructionsEmpty(reqBody) {
		reqBody["instructions"] = "You are a helpful coding assistant."
		bodyModified = true
		markPatchSet("instructions", "You are a helpful coding assistant.")
	}

	if applyOpenAIBuiltinToolsRequestPathTransform(c, reqBody) {
		bodyModified = true
		disablePatch()
	}

	if normalizeOpenAIResponsesImageGenerationTools(reqBody) {
		bodyModified = true
		disablePatch()
		logger.LegacyPrintf("service.openai_gateway", "[OpenAI] Normalized /responses image_generation tool payload")
	}
	if isCodexCLI && applyCodexImageGenerationBridgeInstructions(reqBody) {
		bodyModified = true
		disablePatch()
		logger.LegacyPrintf("service.openai_gateway", "[OpenAI] Added Codex image_generation bridge instructions")
	}

	// 对所有请求执行模型映射（包含 Codex CLI）。
	billingModel := account.GetMappedModel(reqModel)
	if billingModel != reqModel {
		logger.LegacyPrintf("service.openai_gateway", "[OpenAI] Model mapping applied: %s -> %s (account: %s, isCodexCLI: %v)", reqModel, billingModel, account.Name, isCodexCLI)
		reqBody["model"] = billingModel
		bodyModified = true
		markPatchSet("model", billingModel)
	}
	upstreamModel := billingModel
	if normalizeOpenAIResponsesImageOnlyModel(reqBody) {
		bodyModified = true
		disablePatch()
		if model, ok := reqBody["model"].(string); ok {
			upstreamModel = strings.TrimSpace(model)
		}
		logger.LegacyPrintf(
			"service.openai_gateway",
			"[OpenAI] Normalized /responses image-only model request inbound_model=%s image_model=%s upstream_model=%s",
			reqModel,
			billingModel,
			upstreamModel,
		)
	}
	if err := validateOpenAIResponsesImageModel(reqBody, upstreamModel); err != nil {
		setOpsUpstreamError(c, http.StatusBadRequest, err.Error(), "")
		c.JSON(http.StatusBadRequest, gin.H{
			"error": gin.H{
				"type":    "invalid_request_error",
				"message": err.Error(),
				"param":   "model",
			},
		})
		return nil, err
	}
	if hasOpenAIImageGenerationTool(reqBody) {
		logger.LegacyPrintf(
			"service.openai_gateway",
			"[OpenAI] /responses image_generation request inbound_model=%s mapped_model=%s account_type=%s",
			reqModel,
			upstreamModel,
			account.Type,
		)
	}
	if isOpenCodeResponsesClient(c) {
		changed, rehydrateErr := rehydrateOpenCodeGeneratedImageMarkers(ctx, reqBody, s.generatedImageStore, openCodeImageRehydrateOptions{MaxImages: 3})
		if rehydrateErr != nil {
			return nil, rehydrateErr
		}
		if changed {
			bodyModified = true
			disablePatch()
		}
	}
	if err := validateCodexSparkInput(reqBody, upstreamModel); err != nil {
		setOpsUpstreamError(c, http.StatusBadRequest, err.Error(), "")
		c.JSON(http.StatusBadRequest, gin.H{
			"error": gin.H{
				"type":    "invalid_request_error",
				"message": err.Error(),
				"param":   "input",
			},
		})
		return nil, err
	}

	// Compact-only model 映射：仅在 /responses/compact 路径生效，且优先级高于
	// OAuth 模型规范化（避免 OAuth 规范化覆盖 compact-only 自定义模型）。
	isCompactRequest := isOpenAIResponsesCompactPath(c)
	compactMapped := false
	if isCompactRequest {
		compactMappedModel := resolveOpenAICompactForwardModel(account, billingModel)
		if compactMappedModel != "" && compactMappedModel != billingModel {
			compactMapped = true
			upstreamModel = compactMappedModel
			reqBody["model"] = compactMappedModel
			bodyModified = true
			markPatchSet("model", compactMappedModel)
			logger.LegacyPrintf("service.openai_gateway", "[OpenAI] Compact model mapping applied: %s -> %s (account: %s, isCodexCLI: %v)", billingModel, compactMappedModel, account.Name, isCodexCLI)
		}
	}

	// OpenAI OAuth 账号走 ChatGPT internal Codex endpoint，需要将模型名规范化为
	// 上游可识别的 Codex/GPT 系列。API Key 账号则应保留原始/映射后的模型名，
	// 以兼容自定义 base_url 的 OpenAI-compatible 上游。
	if model, ok := reqBody["model"].(string); ok {
		if !compactMapped {
			upstreamModel = normalizeOpenAIModelForUpstream(account, model)
			if upstreamModel != "" && upstreamModel != model {
				logger.LegacyPrintf("service.openai_gateway", "[OpenAI] Upstream model resolved: %s -> %s (account: %s, type: %s, isCodexCLI: %v)",
					model, upstreamModel, account.Name, account.Type, isCodexCLI)
				reqBody["model"] = upstreamModel
				bodyModified = true
				markPatchSet("model", upstreamModel)
			}
		}

		// 移除 gpt-5.2-codex 以下的版本 verbosity 参数
		// 确保高版本模型向低版本模型映射不报错
		if !SupportsVerbosity(upstreamModel) {
			if text, ok := reqBody["text"].(map[string]any); ok {
				delete(text, "verbosity")
			}
		}
	}

	// 规范化 reasoning.effort 参数（minimal -> none），与上游允许值对齐。
	if reasoning, ok := reqBody["reasoning"].(map[string]any); ok {
		if effort, ok := reasoning["effort"].(string); ok && effort == "minimal" {
			reasoning["effort"] = "none"
			bodyModified = true
			markPatchSet("reasoning.effort", "none")
			logger.LegacyPrintf("service.openai_gateway", "[OpenAI] Normalized reasoning.effort: minimal -> none (account: %s)", account.Name)
		}
	}

	if account.Type == AccountTypeOAuth {
		codexResult := applyCodexOAuthTransform(reqBody, isCodexCLI, isCompactRequest)
		if codexResult.Modified {
			bodyModified = true
			disablePatch()
		}
		if codexResult.NormalizedModel != "" {
			upstreamModel = codexResult.NormalizedModel
		}
		if codexResult.PromptCacheKey != "" {
			promptCacheKey = codexResult.PromptCacheKey
		}
	}

	// Handle max_output_tokens based on platform and account type
	if !isCodexCLI {
		if maxOutputTokens, hasMaxOutputTokens := reqBody["max_output_tokens"]; hasMaxOutputTokens {
			switch account.Platform {
			case PlatformOpenAI:
				// For OpenAI API Key, remove max_output_tokens (not supported)
				// For OpenAI OAuth (Responses API), keep it (supported)
				if account.Type == AccountTypeAPIKey {
					delete(reqBody, "max_output_tokens")
					bodyModified = true
					markPatchDelete("max_output_tokens")
				}
			case PlatformAnthropic:
				// For Anthropic (Claude), convert to max_tokens
				delete(reqBody, "max_output_tokens")
				markPatchDelete("max_output_tokens")
				if _, hasMaxTokens := reqBody["max_tokens"]; !hasMaxTokens {
					reqBody["max_tokens"] = maxOutputTokens
					disablePatch()
				}
				bodyModified = true
			case PlatformGemini:
				// For Gemini, remove (will be handled by Gemini-specific transform)
				delete(reqBody, "max_output_tokens")
				bodyModified = true
				markPatchDelete("max_output_tokens")
			default:
				// For unknown platforms, remove to be safe
				delete(reqBody, "max_output_tokens")
				bodyModified = true
				markPatchDelete("max_output_tokens")
			}
		}

		// Also handle max_completion_tokens (similar logic)
		if _, hasMaxCompletionTokens := reqBody["max_completion_tokens"]; hasMaxCompletionTokens {
			if account.Type == AccountTypeAPIKey || account.Platform != PlatformOpenAI {
				delete(reqBody, "max_completion_tokens")
				bodyModified = true
				markPatchDelete("max_completion_tokens")
			}
		}

		// Remove unsupported fields (not supported by upstream OpenAI API)
		unsupportedFields := []string{"prompt_cache_retention", "safety_identifier"}
		for _, unsupportedField := range unsupportedFields {
			if _, has := reqBody[unsupportedField]; has {
				delete(reqBody, unsupportedField)
				bodyModified = true
				markPatchDelete(unsupportedField)
			}
		}
	}

	// 仅在 WSv2 模式保留 previous_response_id，其他模式（HTTP/WSv1）统一过滤。
	// 注意：该规则同样适用于 Codex CLI 请求，避免 WSv1 向上游透传不支持字段。
	if wsDecision.Transport != OpenAIUpstreamTransportResponsesWebsocketV2 {
		if _, has := reqBody["previous_response_id"]; has {
			delete(reqBody, "previous_response_id")
			bodyModified = true
			markPatchDelete("previous_response_id")
		}
	}

	if sanitizeEmptyBase64InputImagesInOpenAIRequestBodyMap(reqBody) {
		bodyModified = true
		disablePatch()
	}

	// Re-serialize body only if modified
	if bodyModified {
		serializedByPatch := false
		if !patchDisabled && patchHasOp {
			var patchErr error
			if patchDelete {
				body, patchErr = sjson.DeleteBytes(body, patchPath)
			} else {
				body, patchErr = sjson.SetBytes(body, patchPath, patchValue)
			}
			if patchErr == nil {
				serializedByPatch = true
			}
		}
		if !serializedByPatch {
			var marshalErr error
			body, marshalErr = json.Marshal(reqBody)
			if marshalErr != nil {
				return nil, fmt.Errorf("serialize request body: %w", marshalErr)
			}
		}
	}

	// Get access token
	token, _, err := s.GetAccessToken(ctx, account)
	if err != nil {
		return nil, err
	}

	// Capture upstream request body for ops retry of this attempt.
	setOpsUpstreamRequestBody(c, body)

	// 命中 WS 时仅走 WebSocket Mode；不再自动回退 HTTP。
	if wsDecision.Transport == OpenAIUpstreamTransportResponsesWebsocketV2 {
		wsReqBody := reqBody
		if len(reqBody) > 0 {
			wsReqBody = make(map[string]any, len(reqBody))
			for k, v := range reqBody {
				wsReqBody[k] = v
			}
		}
		_, hasPreviousResponseID := wsReqBody["previous_response_id"]
		logOpenAIWSModeDebug(
			"forward_start account_id=%d account_type=%s model=%s stream=%v has_previous_response_id=%v",
			account.ID,
			account.Type,
			upstreamModel,
			reqStream,
			hasPreviousResponseID,
		)
		maxAttempts := openAIWSReconnectRetryLimit + 1
		wsAttempts := 0
		var wsResult *OpenAIForwardResult
		var wsErr error
		wsLastFailureReason := ""
		wsPrevResponseRecoveryTried := false
		wsInvalidEncryptedContentRecoveryTried := false
		recoverPrevResponseNotFound := func(attempt int) bool {
			if wsPrevResponseRecoveryTried {
				return false
			}
			previousResponseID := openAIWSPayloadString(wsReqBody, "previous_response_id")
			if previousResponseID == "" {
				logOpenAIWSModeInfo(
					"reconnect_prev_response_recovery_skip account_id=%d attempt=%d reason=missing_previous_response_id previous_response_id_present=false",
					account.ID,
					attempt,
				)
				return false
			}
			if HasFunctionCallOutput(wsReqBody) {
				logOpenAIWSModeInfo(
					"reconnect_prev_response_recovery_skip account_id=%d attempt=%d reason=has_function_call_output previous_response_id_present=true",
					account.ID,
					attempt,
				)
				return false
			}
			delete(wsReqBody, "previous_response_id")
			wsPrevResponseRecoveryTried = true
			logOpenAIWSModeInfo(
				"reconnect_prev_response_recovery account_id=%d attempt=%d action=drop_previous_response_id retry=1 previous_response_id=%s previous_response_id_kind=%s",
				account.ID,
				attempt,
				truncateOpenAIWSLogValue(previousResponseID, openAIWSIDValueMaxLen),
				normalizeOpenAIWSLogValue(ClassifyOpenAIPreviousResponseIDKind(previousResponseID)),
			)
			return true
		}
		recoverInvalidEncryptedContent := func(attempt int) bool {
			if wsInvalidEncryptedContentRecoveryTried {
				return false
			}
			removedReasoningItems := trimOpenAIEncryptedReasoningItems(wsReqBody)
			if !removedReasoningItems {
				logOpenAIWSModeInfo(
					"reconnect_invalid_encrypted_content_recovery_skip account_id=%d attempt=%d reason=missing_encrypted_reasoning_items",
					account.ID,
					attempt,
				)
				return false
			}
			previousResponseID := openAIWSPayloadString(wsReqBody, "previous_response_id")
			hasFunctionCallOutput := HasFunctionCallOutput(wsReqBody)
			if previousResponseID != "" && !hasFunctionCallOutput {
				delete(wsReqBody, "previous_response_id")
			}
			wsInvalidEncryptedContentRecoveryTried = true
			logOpenAIWSModeInfo(
				"reconnect_invalid_encrypted_content_recovery account_id=%d attempt=%d action=drop_encrypted_reasoning_items retry=1 previous_response_id_present=%v previous_response_id=%s previous_response_id_kind=%s has_function_call_output=%v dropped_previous_response_id=%v",
				account.ID,
				attempt,
				previousResponseID != "",
				truncateOpenAIWSLogValue(previousResponseID, openAIWSIDValueMaxLen),
				normalizeOpenAIWSLogValue(ClassifyOpenAIPreviousResponseIDKind(previousResponseID)),
				hasFunctionCallOutput,
				previousResponseID != "" && !hasFunctionCallOutput,
			)
			return true
		}
		retryBudget := s.openAIWSRetryTotalBudget()
		retryStartedAt := time.Now()
	wsRetryLoop:
		for attempt := 1; attempt <= maxAttempts; attempt++ {
			wsAttempts = attempt
			wsResult, wsErr = s.forwardOpenAIWSV2(
				ctx,
				c,
				account,
				wsReqBody,
				token,
				wsDecision,
				isCodexCLI,
				reqStream,
				originalModel,
				upstreamModel,
				startTime,
				attempt,
				wsLastFailureReason,
			)
			if wsErr == nil {
				break
			}
			if c != nil && c.Writer != nil && c.Writer.Written() {
				break
			}

			reason, retryable := classifyOpenAIWSReconnectReason(wsErr)
			if reason != "" {
				wsLastFailureReason = reason
			}
			// previous_response_not_found 说明续链锚点不可用：
			// 对非 function_call_output 场景，允许一次“去掉 previous_response_id 后重放”。
			if reason == "previous_response_not_found" && recoverPrevResponseNotFound(attempt) {
				continue
			}
			if reason == "invalid_encrypted_content" && recoverInvalidEncryptedContent(attempt) {
				continue
			}
			if retryable && attempt < maxAttempts {
				backoff := s.openAIWSRetryBackoff(attempt)
				if retryBudget > 0 && time.Since(retryStartedAt)+backoff > retryBudget {
					s.recordOpenAIWSRetryExhausted()
					logOpenAIWSModeInfo(
						"reconnect_budget_exhausted account_id=%d attempts=%d max_retries=%d reason=%s elapsed_ms=%d budget_ms=%d",
						account.ID,
						attempt,
						openAIWSReconnectRetryLimit,
						normalizeOpenAIWSLogValue(reason),
						time.Since(retryStartedAt).Milliseconds(),
						retryBudget.Milliseconds(),
					)
					break
				}
				s.recordOpenAIWSRetryAttempt(backoff)
				logOpenAIWSModeInfo(
					"reconnect_retry account_id=%d retry=%d max_retries=%d reason=%s backoff_ms=%d",
					account.ID,
					attempt,
					openAIWSReconnectRetryLimit,
					normalizeOpenAIWSLogValue(reason),
					backoff.Milliseconds(),
				)
				if backoff > 0 {
					timer := time.NewTimer(backoff)
					select {
					case <-ctx.Done():
						if !timer.Stop() {
							<-timer.C
						}
						wsErr = wrapOpenAIWSFallback("retry_backoff_canceled", ctx.Err())
						break wsRetryLoop
					case <-timer.C:
					}
				}
				continue
			}
			if retryable {
				s.recordOpenAIWSRetryExhausted()
				logOpenAIWSModeInfo(
					"reconnect_exhausted account_id=%d attempts=%d max_retries=%d reason=%s",
					account.ID,
					attempt,
					openAIWSReconnectRetryLimit,
					normalizeOpenAIWSLogValue(reason),
				)
			} else if reason != "" {
				s.recordOpenAIWSNonRetryableFastFallback()
				logOpenAIWSModeInfo(
					"reconnect_stop account_id=%d attempt=%d reason=%s",
					account.ID,
					attempt,
					normalizeOpenAIWSLogValue(reason),
				)
			}
			break
		}
		if wsErr == nil {
			firstTokenMs := int64(0)
			hasFirstTokenMs := wsResult != nil && wsResult.FirstTokenMs != nil
			if hasFirstTokenMs {
				firstTokenMs = int64(*wsResult.FirstTokenMs)
			}
			requestID := ""
			if wsResult != nil {
				requestID = strings.TrimSpace(wsResult.RequestID)
			}
			logOpenAIWSModeDebug(
				"forward_succeeded account_id=%d request_id=%s stream=%v has_first_token_ms=%v first_token_ms=%d ws_attempts=%d",
				account.ID,
				requestID,
				reqStream,
				hasFirstTokenMs,
				firstTokenMs,
				wsAttempts,
			)
			wsResult.UpstreamModel = upstreamModel
			return wsResult, nil
		}
		s.writeOpenAIWSFallbackErrorResponse(c, account, wsErr)
		return nil, wsErr
	}

	httpInvalidEncryptedContentRetryTried := false
	for {
		// Build upstream request
		upstreamCtx, releaseUpstreamCtx := detachStreamUpstreamContext(ctx, reqStream)
		upstreamReq, err := s.buildUpstreamRequest(upstreamCtx, c, account, body, token, reqStream, promptCacheKey, isCodexCLI)
		releaseUpstreamCtx()
		if err != nil {
			return nil, err
		}

		// Get proxy URL
		proxyURL := ""
		if account.ProxyID != nil && account.Proxy != nil {
			proxyURL = account.Proxy.URL()
		}

		// Send request
		upstreamStart := time.Now()
		resp, err := s.httpUpstream.Do(upstreamReq, proxyURL, account.ID, account.Concurrency)
		SetOpsLatencyMs(c, OpsUpstreamLatencyMsKey, time.Since(upstreamStart).Milliseconds())
		if err != nil {
			// Ensure the client receives an error response (handlers assume Forward writes on non-failover errors).
			safeErr := sanitizeUpstreamErrorMessage(err.Error())
			setOpsUpstreamError(c, 0, safeErr, "")
			appendOpsUpstreamError(c, OpsUpstreamErrorEvent{
				Platform:           account.Platform,
				AccountID:          account.ID,
				AccountName:        account.Name,
				UpstreamStatusCode: 0,
				Kind:               "request_error",
				Message:            safeErr,
			})
			c.JSON(http.StatusBadGateway, gin.H{
				"error": gin.H{
					"type":    "upstream_error",
					"message": "Upstream request failed",
				},
			})
			return nil, fmt.Errorf("upstream request failed: %s", safeErr)
		}

		// Handle error response
		if resp.StatusCode >= 400 {
			respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
			_ = resp.Body.Close()
			resp.Body = io.NopCloser(bytes.NewReader(respBody))

			upstreamMsg := strings.TrimSpace(extractUpstreamErrorMessage(respBody))
			upstreamMsg = sanitizeUpstreamErrorMessage(upstreamMsg)
			upstreamCode := extractUpstreamErrorCode(respBody)
			if !httpInvalidEncryptedContentRetryTried && resp.StatusCode == http.StatusBadRequest && upstreamCode == "invalid_encrypted_content" {
				if trimOpenAIEncryptedReasoningItems(reqBody) {
					body, err = json.Marshal(reqBody)
					if err != nil {
						return nil, fmt.Errorf("serialize invalid_encrypted_content retry body: %w", err)
					}
					setOpsUpstreamRequestBody(c, body)
					httpInvalidEncryptedContentRetryTried = true
					logger.LegacyPrintf("service.openai_gateway", "[OpenAI] Retrying non-WSv2 request once after invalid_encrypted_content (account: %s)", account.Name)
					continue
				}
				logger.LegacyPrintf("service.openai_gateway", "[OpenAI] Skip non-WSv2 invalid_encrypted_content retry because encrypted reasoning items are missing (account: %s)", account.Name)
			}
			if s.shouldFailoverOpenAIUpstreamResponse(resp.StatusCode, upstreamMsg, respBody) {
				if isOpenAITransientProcessingError(resp.StatusCode, upstreamMsg, respBody) {
					logOpenAITransientProcessingFailover(ctx, c, account, resp.StatusCode, resp.Header.Get("x-request-id"), upstreamMsg)
				}
				upstreamDetail := ""
				if s.cfg != nil && s.cfg.Gateway.LogUpstreamErrorBody {
					maxBytes := s.cfg.Gateway.LogUpstreamErrorBodyMaxBytes
					if maxBytes <= 0 {
						maxBytes = 2048
					}
					upstreamDetail = truncateString(string(respBody), maxBytes)
				}
				appendOpsUpstreamError(c, OpsUpstreamErrorEvent{
					Platform:           account.Platform,
					AccountID:          account.ID,
					AccountName:        account.Name,
					UpstreamStatusCode: resp.StatusCode,
					UpstreamRequestID:  resp.Header.Get("x-request-id"),
					Kind:               "failover",
					Message:            upstreamMsg,
					Detail:             upstreamDetail,
				})

				s.handleFailoverSideEffects(ctx, resp, account)
				return nil, &UpstreamFailoverError{
					StatusCode:             resp.StatusCode,
					ResponseBody:           respBody,
					RetryableOnSameAccount: account.IsPoolMode() && (isPoolModeRetryableStatus(resp.StatusCode) || isOpenAITransientProcessingError(resp.StatusCode, upstreamMsg, respBody)),
				}
			}
			return s.handleErrorResponse(ctx, resp, c, account, body)
		}
		defer func() { _ = resp.Body.Close() }()

		// Handle normal response
		var usage *OpenAIUsage
		var firstTokenMs *int
		if reqStream {
			streamResult, err := s.handleStreamingResponseWithOpenCodeContinuation(ctx, resp, c, account, startTime, originalModel, upstreamModel, &openCodeImageServerContinuationContext{
				RequestBody:    body,
				Token:          token,
				PromptCacheKey: promptCacheKey,
				IsCodexCLI:     isCodexCLI,
			})
			if err != nil {
				return nil, err
			}
			usage = streamResult.usage
			firstTokenMs = streamResult.firstTokenMs
		} else {
			usage, err = s.handleNonStreamingResponseWithOpenCodeContinuation(ctx, resp, c, account, originalModel, upstreamModel, &openCodeImageServerContinuationContext{
				RequestBody:    body,
				Token:          token,
				PromptCacheKey: promptCacheKey,
				IsCodexCLI:     isCodexCLI,
			})
			if err != nil {
				return nil, err
			}
		}

		// Extract and save Codex usage snapshot from response headers (for OAuth accounts)
		if account.Type == AccountTypeOAuth {
			if snapshot := ParseCodexRateLimitHeaders(resp.Header); snapshot != nil {
				s.updateCodexUsageSnapshot(ctx, account.ID, snapshot)
			}
		}

		if usage == nil {
			usage = &OpenAIUsage{}
		}

		reasoningEffort := extractOpenAIReasoningEffort(reqBody, originalModel)
		serviceTier := extractOpenAIServiceTier(reqBody)

		return &OpenAIForwardResult{
			RequestID:       resp.Header.Get("x-request-id"),
			Usage:           *usage,
			Model:           originalModel,
			UpstreamModel:   upstreamModel,
			ServiceTier:     serviceTier,
			ReasoningEffort: reasoningEffort,
			Stream:          reqStream,
			OpenAIWSMode:    false,
			Duration:        time.Since(startTime),
			FirstTokenMs:    firstTokenMs,
		}, nil
	}
}

func (s *OpenAIGatewayService) forwardOpenAIPassthrough(
	ctx context.Context,
	c *gin.Context,
	account *Account,
	body []byte,
	reqModel string,
	reasoningEffort *string,
	reqStream bool,
	startTime time.Time,
) (*OpenAIForwardResult, error) {
	if strippedBody, changed := stripOpenAIBuiltinToolsFieldFromBody(body); changed {
		body = strippedBody
	}
	upstreamPassthroughModel := ""
	if isOpenAIResponsesCompactPath(c) {
		compactMappedModel := resolveOpenAICompactForwardModel(account, reqModel)
		if compactMappedModel != "" && compactMappedModel != reqModel {
			nextBody, setErr := sjson.SetBytes(body, "model", compactMappedModel)
			if setErr != nil {
				return nil, fmt.Errorf("set compact passthrough model: %w", setErr)
			}
			body = nextBody
			upstreamPassthroughModel = compactMappedModel
		}
	}

	if account != nil && account.Type == AccountTypeOAuth {
		if rejectReason := detectOpenAIPassthroughInstructionsRejectReason(reqModel, body); rejectReason != "" {
			rejectMsg := "OpenAI codex passthrough requires a non-empty instructions field"
			setOpsUpstreamError(c, http.StatusForbidden, rejectMsg, "")
			appendOpsUpstreamError(c, OpsUpstreamErrorEvent{
				Platform:           account.Platform,
				AccountID:          account.ID,
				AccountName:        account.Name,
				UpstreamStatusCode: http.StatusForbidden,
				Passthrough:        true,
				Kind:               "request_error",
				Message:            rejectMsg,
				Detail:             rejectReason,
			})
			logOpenAIPassthroughInstructionsRejected(ctx, c, account, reqModel, rejectReason, body)
			c.JSON(http.StatusForbidden, gin.H{
				"error": gin.H{
					"type":    "forbidden_error",
					"message": rejectMsg,
				},
			})
			return nil, fmt.Errorf("openai passthrough rejected before upstream: %s", rejectReason)
		}

		normalizedBody, normalized, err := normalizeOpenAIPassthroughOAuthBody(body, isOpenAIResponsesCompactPath(c))
		if err != nil {
			return nil, err
		}
		if normalized {
			body = normalizedBody
		}
		reqStream = gjson.GetBytes(body, "stream").Bool()
	}

	sanitizedBody, sanitized, err := sanitizeEmptyBase64InputImagesInOpenAIBody(body)
	if err != nil {
		return nil, err
	}
	if sanitized {
		body = sanitizedBody
	}

	logger.LegacyPrintf("service.openai_gateway",
		"[OpenAI 自动透传] 命中自动透传分支: account=%d name=%s type=%s model=%s stream=%v",
		account.ID,
		account.Name,
		account.Type,
		reqModel,
		reqStream,
	)
	if reqStream && c != nil && c.Request != nil {
		if timeoutHeaders := collectOpenAIPassthroughTimeoutHeaders(c.Request.Header); len(timeoutHeaders) > 0 {
			streamWarnLogger := logger.FromContext(ctx).With(
				zap.String("component", "service.openai_gateway"),
				zap.Int64("account_id", account.ID),
				zap.Strings("timeout_headers", timeoutHeaders),
			)
			if s.isOpenAIPassthroughTimeoutHeadersAllowed() {
				streamWarnLogger.Warn("OpenAI passthrough 透传请求包含超时相关请求头，且当前配置为放行，可能导致上游提前断流")
			} else {
				streamWarnLogger.Warn("OpenAI passthrough 检测到超时相关请求头，将按配置过滤以降低断流风险")
			}
		}
	}

	// Get access token
	token, _, err := s.GetAccessToken(ctx, account)
	if err != nil {
		return nil, err
	}

	upstreamCtx, releaseUpstreamCtx := detachStreamUpstreamContext(ctx, reqStream)
	upstreamReq, err := s.buildUpstreamRequestOpenAIPassthrough(upstreamCtx, c, account, body, token)
	releaseUpstreamCtx()
	if err != nil {
		return nil, err
	}

	proxyURL := ""
	if account.ProxyID != nil && account.Proxy != nil {
		proxyURL = account.Proxy.URL()
	}

	setOpsUpstreamRequestBody(c, body)
	if c != nil {
		c.Set("openai_passthrough", true)
	}

	upstreamStart := time.Now()
	resp, err := s.httpUpstream.Do(upstreamReq, proxyURL, account.ID, account.Concurrency)
	SetOpsLatencyMs(c, OpsUpstreamLatencyMsKey, time.Since(upstreamStart).Milliseconds())
	if err != nil {
		safeErr := sanitizeUpstreamErrorMessage(err.Error())
		setOpsUpstreamError(c, 0, safeErr, "")
		appendOpsUpstreamError(c, OpsUpstreamErrorEvent{
			Platform:           account.Platform,
			AccountID:          account.ID,
			AccountName:        account.Name,
			UpstreamStatusCode: 0,
			Passthrough:        true,
			Kind:               "request_error",
			Message:            safeErr,
		})
		c.JSON(http.StatusBadGateway, gin.H{
			"error": gin.H{
				"type":    "upstream_error",
				"message": "Upstream request failed",
			},
		})
		return nil, fmt.Errorf("upstream request failed: %s", safeErr)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode >= 400 {
		// 透传模式默认保持原样代理；但 429/529 属于网关必须兜底的
		// 上游容量类错误，应先触发多账号 failover 以维持基础 SLA。
		if shouldFailoverOpenAIPassthroughResponse(resp.StatusCode) {
			return nil, s.handleFailoverErrorResponsePassthrough(ctx, resp, c, account, body)
		}
		return nil, s.handleErrorResponsePassthrough(ctx, resp, c, account, body)
	}

	var usage *OpenAIUsage
	var firstTokenMs *int
	if reqStream {
		result, err := s.handleStreamingResponsePassthrough(ctx, resp, c, account, startTime, reqModel, upstreamPassthroughModel)
		if err != nil {
			return nil, err
		}
		usage = result.usage
		firstTokenMs = result.firstTokenMs
	} else {
		usage, err = s.handleNonStreamingResponsePassthrough(ctx, resp, c, reqModel, upstreamPassthroughModel)
		if err != nil {
			return nil, err
		}
	}

	if snapshot := ParseCodexRateLimitHeaders(resp.Header); snapshot != nil {
		s.updateCodexUsageSnapshot(ctx, account.ID, snapshot)
	}

	if usage == nil {
		usage = &OpenAIUsage{}
	}

	return &OpenAIForwardResult{
		RequestID:       resp.Header.Get("x-request-id"),
		Usage:           *usage,
		Model:           reqModel,
		UpstreamModel:   upstreamPassthroughModel,
		ServiceTier:     extractOpenAIServiceTierFromBody(body),
		ReasoningEffort: reasoningEffort,
		Stream:          reqStream,
		OpenAIWSMode:    false,
		Duration:        time.Since(startTime),
		FirstTokenMs:    firstTokenMs,
	}, nil
}

func logOpenAIPassthroughInstructionsRejected(
	ctx context.Context,
	c *gin.Context,
	account *Account,
	reqModel string,
	rejectReason string,
	body []byte,
) {
	if ctx == nil {
		ctx = context.Background()
	}
	accountID := int64(0)
	accountName := ""
	accountType := ""
	if account != nil {
		accountID = account.ID
		accountName = strings.TrimSpace(account.Name)
		accountType = strings.TrimSpace(string(account.Type))
	}
	fields := []zap.Field{
		zap.String("component", "service.openai_gateway"),
		zap.Int64("account_id", accountID),
		zap.String("account_name", accountName),
		zap.String("account_type", accountType),
		zap.String("request_model", strings.TrimSpace(reqModel)),
		zap.String("reject_reason", strings.TrimSpace(rejectReason)),
	}
	fields = appendCodexCLIOnlyRejectedRequestFields(fields, c, body)
	logger.FromContext(ctx).With(fields...).Warn("OpenAI passthrough 本地拦截：Codex 请求缺少有效 instructions")
}

func (s *OpenAIGatewayService) buildUpstreamRequestOpenAIPassthrough(
	ctx context.Context,
	c *gin.Context,
	account *Account,
	body []byte,
	token string,
) (*http.Request, error) {
	targetURL := openaiPlatformAPIURL
	switch account.Type {
	case AccountTypeOAuth:
		targetURL = chatgptCodexURL
	case AccountTypeAPIKey:
		baseURL := account.GetOpenAIBaseURL()
		if baseURL != "" {
			validatedURL, err := s.validateUpstreamBaseURL(baseURL)
			if err != nil {
				return nil, err
			}
			targetURL = buildOpenAIResponsesURL(validatedURL)
		}
	}
	targetURL = appendOpenAIResponsesRequestPathSuffix(targetURL, openAIResponsesRequestPathSuffix(c))

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, targetURL, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}

	// 透传客户端请求头（安全白名单）。
	allowTimeoutHeaders := s.isOpenAIPassthroughTimeoutHeadersAllowed()
	if c != nil && c.Request != nil {
		for key, values := range c.Request.Header {
			lower := strings.ToLower(strings.TrimSpace(key))
			if !isOpenAIPassthroughAllowedRequestHeader(lower, allowTimeoutHeaders) {
				continue
			}
			for _, v := range values {
				req.Header.Add(key, v)
			}
		}
	}

	// 覆盖入站鉴权残留，并注入上游认证
	req.Header.Del("authorization")
	req.Header.Del("x-api-key")
	req.Header.Del("x-goog-api-key")
	req.Header.Set("authorization", "Bearer "+token)

	// OAuth 透传到 ChatGPT internal API 时补齐必要头。
	if account.Type == AccountTypeOAuth {
		sessionSignal := extractOpenAISessionSignal(c, body, false)
		req.Host = "chatgpt.com"
		if chatgptAccountID := account.GetChatGPTAccountID(); chatgptAccountID != "" {
			req.Header.Set("chatgpt-account-id", chatgptAccountID)
		}
		apiKeyID := getAPIKeyIDFromContext(c)
		// 先保存客户端原始值，再做 compact 补充，避免后续统一隔离时读到已处理的值。
		clientSessionID := strings.TrimSpace(req.Header.Get("session_id"))
		clientConversationID := strings.TrimSpace(req.Header.Get("conversation_id"))
		if clientSessionID == "" {
			clientSessionID = sessionSignal
		}
		if clientConversationID == "" {
			clientConversationID = sessionSignal
		}
		if isOpenAIResponsesCompactPath(c) {
			req.Header.Set("accept", "application/json")
			if req.Header.Get("version") == "" {
				req.Header.Set("version", codexCLIVersion)
			}
			if clientSessionID == "" {
				clientSessionID = resolveOpenAICompactSessionID(c, body)
			}
		} else if req.Header.Get("accept") == "" {
			req.Header.Set("accept", "text/event-stream")
		}
		if req.Header.Get("OpenAI-Beta") == "" {
			req.Header.Set("OpenAI-Beta", "responses=experimental")
		}
		if req.Header.Get("originator") == "" {
			req.Header.Set("originator", "codex_cli_rs")
		}
		// 用隔离后的 session 标识符覆盖客户端透传值，防止跨用户会话碰撞。
		if clientSessionID != "" {
			req.Header.Set("session_id", isolateOpenAISessionID(apiKeyID, clientSessionID))
		}
		if clientConversationID != "" {
			req.Header.Set("conversation_id", isolateOpenAISessionID(apiKeyID, clientConversationID))
		}
	}

	// 透传模式也支持账户自定义 User-Agent 与 ForceCodexCLI 兜底。
	customUA := account.GetOpenAIUserAgent()
	if customUA != "" {
		req.Header.Set("user-agent", customUA)
	}
	if s.cfg != nil && s.cfg.Gateway.ForceCodexCLI {
		req.Header.Set("user-agent", codexCLIUserAgent)
	}
	// OAuth 安全透传：对非 Codex UA 统一兜底，降低被上游风控拦截概率。
	if account.Type == AccountTypeOAuth && !openai.IsCodexCLIRequest(req.Header.Get("user-agent")) {
		req.Header.Set("user-agent", codexCLIUserAgent)
	}

	if req.Header.Get("content-type") == "" {
		req.Header.Set("content-type", "application/json")
	}

	return req, nil
}

func shouldFailoverOpenAIPassthroughResponse(statusCode int) bool {
	switch statusCode {
	case http.StatusTooManyRequests, 529:
		return true
	default:
		return false
	}
}

func (s *OpenAIGatewayService) handleFailoverErrorResponsePassthrough(
	ctx context.Context,
	resp *http.Response,
	c *gin.Context,
	account *Account,
	requestBody []byte,
) error {
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 2<<20))

	upstreamMsg := strings.TrimSpace(extractUpstreamErrorMessage(body))
	upstreamMsg = sanitizeUpstreamErrorMessage(upstreamMsg)
	upstreamDetail := ""
	if s.cfg != nil && s.cfg.Gateway.LogUpstreamErrorBody {
		maxBytes := s.cfg.Gateway.LogUpstreamErrorBodyMaxBytes
		if maxBytes <= 0 {
			maxBytes = 2048
		}
		upstreamDetail = truncateString(string(body), maxBytes)
	}
	setOpsUpstreamError(c, resp.StatusCode, upstreamMsg, upstreamDetail)
	logOpenAIInstructionsRequiredDebug(ctx, c, account, resp.StatusCode, upstreamMsg, requestBody, body)
	if s.rateLimitService != nil {
		_ = s.rateLimitService.HandleUpstreamError(ctx, account, resp.StatusCode, resp.Header, body)
	}
	appendOpsUpstreamError(c, OpsUpstreamErrorEvent{
		Platform:             account.Platform,
		AccountID:            account.ID,
		AccountName:          account.Name,
		UpstreamStatusCode:   resp.StatusCode,
		UpstreamRequestID:    resp.Header.Get("x-request-id"),
		Passthrough:          true,
		Kind:                 "failover",
		Message:              upstreamMsg,
		Detail:               upstreamDetail,
		UpstreamResponseBody: upstreamDetail,
	})
	return &UpstreamFailoverError{
		StatusCode:      resp.StatusCode,
		ResponseBody:    body,
		ResponseHeaders: resp.Header.Clone(),
	}
}

func (s *OpenAIGatewayService) handleErrorResponsePassthrough(
	ctx context.Context,
	resp *http.Response,
	c *gin.Context,
	account *Account,
	requestBody []byte,
) error {
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 2<<20))

	upstreamMsg := strings.TrimSpace(extractUpstreamErrorMessage(body))
	upstreamMsg = sanitizeUpstreamErrorMessage(upstreamMsg)
	upstreamDetail := ""
	if s.cfg != nil && s.cfg.Gateway.LogUpstreamErrorBody {
		maxBytes := s.cfg.Gateway.LogUpstreamErrorBodyMaxBytes
		if maxBytes <= 0 {
			maxBytes = 2048
		}
		upstreamDetail = truncateString(string(body), maxBytes)
	}
	setOpsUpstreamError(c, resp.StatusCode, upstreamMsg, upstreamDetail)
	logOpenAIInstructionsRequiredDebug(ctx, c, account, resp.StatusCode, upstreamMsg, requestBody, body)
	if s.rateLimitService != nil {
		// Passthrough mode preserves the raw upstream error response, but runtime
		// account state still needs to be updated so sticky routing can stop
		// reusing a freshly rate-limited account.
		_ = s.rateLimitService.HandleUpstreamError(ctx, account, resp.StatusCode, resp.Header, body)
	}
	appendOpsUpstreamError(c, OpsUpstreamErrorEvent{
		Platform:             account.Platform,
		AccountID:            account.ID,
		AccountName:          account.Name,
		UpstreamStatusCode:   resp.StatusCode,
		UpstreamRequestID:    resp.Header.Get("x-request-id"),
		Passthrough:          true,
		Kind:                 "http_error",
		Message:              upstreamMsg,
		Detail:               upstreamDetail,
		UpstreamResponseBody: upstreamDetail,
	})

	writeOpenAIPassthroughResponseHeaders(c.Writer.Header(), resp.Header, s.responseHeaderFilter)
	contentType := resp.Header.Get("Content-Type")
	if contentType == "" {
		contentType = "application/json"
	}
	c.Data(resp.StatusCode, contentType, body)

	if upstreamMsg == "" {
		return fmt.Errorf("upstream error: %d", resp.StatusCode)
	}
	return fmt.Errorf("upstream error: %d message=%s", resp.StatusCode, upstreamMsg)
}

func isOpenAIPassthroughAllowedRequestHeader(lowerKey string, allowTimeoutHeaders bool) bool {
	if lowerKey == "" {
		return false
	}
	if isOpenAIPassthroughTimeoutHeader(lowerKey) {
		return allowTimeoutHeaders
	}
	return openaiPassthroughAllowedHeaders[lowerKey]
}

func isOpenAIPassthroughTimeoutHeader(lowerKey string) bool {
	switch lowerKey {
	case "x-stainless-timeout", "x-stainless-read-timeout", "x-stainless-connect-timeout", "x-request-timeout", "request-timeout", "grpc-timeout":
		return true
	default:
		return false
	}
}

func (s *OpenAIGatewayService) isOpenAIPassthroughTimeoutHeadersAllowed() bool {
	return s != nil && s.cfg != nil && s.cfg.Gateway.OpenAIPassthroughAllowTimeoutHeaders
}

func collectOpenAIPassthroughTimeoutHeaders(h http.Header) []string {
	if h == nil {
		return nil
	}
	var matched []string
	for key, values := range h {
		lowerKey := strings.ToLower(strings.TrimSpace(key))
		if isOpenAIPassthroughTimeoutHeader(lowerKey) {
			entry := lowerKey
			if len(values) > 0 {
				entry = fmt.Sprintf("%s=%s", lowerKey, strings.Join(values, "|"))
			}
			matched = append(matched, entry)
		}
	}
	sort.Strings(matched)
	return matched
}

type openaiStreamingResultPassthrough struct {
	usage        *OpenAIUsage
	firstTokenMs *int
}

func openAIStreamClientOutputStarted(c *gin.Context, localStarted bool) bool {
	if localStarted {
		return true
	}
	return c != nil && c.Writer != nil && c.Writer.Written()
}

func openAIStreamEventIsPreamble(eventType string) bool {
	switch strings.TrimSpace(eventType) {
	case "response.created", "response.in_progress":
		return true
	default:
		return false
	}
}

func openAIStreamDataStartsClientOutput(data, eventType string) bool {
	trimmed := strings.TrimSpace(data)
	if trimmed == "" {
		return false
	}
	if strings.TrimSpace(eventType) == "response.failed" {
		return false
	}
	return !openAIStreamEventIsPreamble(eventType)
}

func openAIStreamFailedEventShouldFailover(payload []byte, message string) bool {
	code := strings.ToLower(strings.TrimSpace(gjson.GetBytes(payload, "response.error.code").String()))
	if code == "" {
		code = strings.ToLower(strings.TrimSpace(gjson.GetBytes(payload, "error.code").String()))
	}
	errType := strings.ToLower(strings.TrimSpace(gjson.GetBytes(payload, "response.error.type").String()))
	if errType == "" {
		errType = strings.ToLower(strings.TrimSpace(gjson.GetBytes(payload, "error.type").String()))
	}
	combined := strings.ToLower(strings.TrimSpace(message + " " + code + " " + errType))
	if combined == "" {
		return true
	}
	nonRetryableMarkers := []string{
		"invalid_request",
		"content_policy",
		"policy",
		"safety",
		"high-risk cyber",
		"not allowed",
		"violat",
	}
	for _, marker := range nonRetryableMarkers {
		if strings.Contains(combined, marker) {
			return false
		}
	}
	return true
}

func (s *OpenAIGatewayService) newOpenAIStreamFailoverError(
	c *gin.Context,
	account *Account,
	passthrough bool,
	upstreamRequestID string,
	payload []byte,
	message string,
) *UpstreamFailoverError {
	message = sanitizeUpstreamErrorMessage(strings.TrimSpace(message))
	if message == "" {
		message = "OpenAI stream disconnected before completion"
	}
	detail := ""
	if len(payload) > 0 && s != nil && s.cfg != nil && s.cfg.Gateway.LogUpstreamErrorBody {
		maxBytes := s.cfg.Gateway.LogUpstreamErrorBodyMaxBytes
		if maxBytes <= 0 {
			maxBytes = 2048
		}
		detail = truncateString(string(payload), maxBytes)
	}
	if c != nil {
		setOpsUpstreamError(c, http.StatusBadGateway, message, detail)
		event := OpsUpstreamErrorEvent{
			Platform:           PlatformOpenAI,
			UpstreamStatusCode: http.StatusBadGateway,
			UpstreamRequestID:  strings.TrimSpace(upstreamRequestID),
			Passthrough:        passthrough,
			Kind:               "failover",
			Message:            message,
			Detail:             detail,
		}
		if account != nil {
			event.Platform = account.Platform
			event.AccountID = account.ID
			event.AccountName = account.Name
		}
		appendOpsUpstreamError(c, event)
	}
	body, _ := json.Marshal(gin.H{
		"error": gin.H{
			"type":    "upstream_error",
			"message": message,
		},
	})
	return &UpstreamFailoverError{
		StatusCode:   http.StatusBadGateway,
		ResponseBody: body,
	}
}

func (s *OpenAIGatewayService) handleStreamingResponsePassthrough(
	ctx context.Context,
	resp *http.Response,
	c *gin.Context,
	account *Account,
	startTime time.Time,
	originalModel string,
	mappedModel string,
) (*openaiStreamingResultPassthrough, error) {
	w := c.Writer
	flusher, ok := w.(http.Flusher)
	if !ok {
		return nil, errors.New("streaming not supported")
	}

	usage := &OpenAIUsage{}
	var firstTokenMs *int
	clientDisconnected := false
	sawDone := false
	sawTerminalEvent := false
	streamStarted := false
	sawFailedEvent := false
	failedMessage := ""
	clientOutputStarted := false
	upstreamRequestID := strings.TrimSpace(resp.Header.Get("x-request-id"))
	startStream := func() {
		if streamStarted {
			return
		}
		writeOpenAIPassthroughResponseHeaders(c.Writer.Header(), resp.Header, s.responseHeaderFilter)
		c.Header("Content-Type", "text/event-stream")
		c.Header("Cache-Control", "no-cache")
		c.Header("Connection", "keep-alive")
		c.Header("X-Accel-Buffering", "no")
		if upstreamRequestID != "" {
			c.Header("x-request-id", upstreamRequestID)
		}
		c.Writer.WriteHeader(http.StatusOK)
		streamStarted = true
	}
	pendingLines := make([]string, 0, 8)
	writePendingLines := func() bool {
		startStream()
		for _, pending := range pendingLines {
			if _, err := fmt.Fprintln(w, pending); err != nil {
				clientDisconnected = true
				logger.LegacyPrintf("service.openai_gateway", "[OpenAI passthrough] Client disconnected during streaming, continue draining upstream for usage: account=%d", account.ID)
				return false
			}
		}
		pendingLines = pendingLines[:0]
		return true
	}

	scanner := bufio.NewScanner(resp.Body)
	maxLineSize := defaultMaxLineSize
	if s.cfg != nil && s.cfg.Gateway.MaxLineSize > 0 {
		maxLineSize = s.cfg.Gateway.MaxLineSize
	}
	scanBuf := getSSEScannerBuf64K()
	scanner.Buffer(scanBuf[:0], maxLineSize)
	defer putSSEScannerBuf64K(scanBuf)

	needModelReplace := strings.TrimSpace(originalModel) != "" && strings.TrimSpace(mappedModel) != "" && strings.TrimSpace(originalModel) != strings.TrimSpace(mappedModel)

	for scanner.Scan() {
		line := scanner.Text()
		lineStartsClientOutput := false
		forceFlushFailedEvent := false
		if data, ok := extractOpenAISSEDataLine(line); ok {
			dataBytes := []byte(data)
			trimmedData := strings.TrimSpace(data)
			if !openAIStreamClientOutputStarted(c, clientOutputStarted) && firstTokenMs == nil && gjson.Get(trimmedData, "type").String() == "error" {
				msg := extractOpenAISSEErrorMessage(dataBytes)
				if isOpenAITransientProcessingError(http.StatusBadRequest, msg, dataBytes) {
					return &openaiStreamingResultPassthrough{usage: usage, firstTokenMs: firstTokenMs}, &UpstreamFailoverError{
						StatusCode:   http.StatusServiceUnavailable,
						ResponseBody: dataBytes,
					}
				}
			}
			if needModelReplace && strings.Contains(data, mappedModel) {
				line = s.replaceModelInSSELine(line, mappedModel, originalModel)
				if replacedData, replaced := extractOpenAISSEDataLine(line); replaced {
					dataBytes = []byte(replacedData)
					trimmedData = strings.TrimSpace(replacedData)
				}
			}
			eventType := strings.TrimSpace(gjson.Get(trimmedData, "type").String())
			if eventType == "response.failed" {
				failedMessage = extractOpenAISSEErrorMessage(dataBytes)
				if !openAIStreamClientOutputStarted(c, clientOutputStarted) && openAIStreamFailedEventShouldFailover(dataBytes, failedMessage) {
					return &openaiStreamingResultPassthrough{usage: usage, firstTokenMs: firstTokenMs},
						s.newOpenAIStreamFailoverError(c, account, true, upstreamRequestID, dataBytes, failedMessage)
				}
				forceFlushFailedEvent = true
				sawFailedEvent = true
			}
			if trimmedData == "[DONE]" {
				sawDone = true
			}
			if openAIStreamEventIsTerminal(trimmedData) {
				sawTerminalEvent = true
			}
			lineStartsClientOutput = forceFlushFailedEvent || openAIStreamDataStartsClientOutput(trimmedData, eventType)
			if firstTokenMs == nil && lineStartsClientOutput && trimmedData != "[DONE]" {
				ms := int(time.Since(startTime).Milliseconds())
				firstTokenMs = &ms
			}
			s.parseSSEUsageBytes(dataBytes, usage)
		}

		if !clientDisconnected {
			if !clientOutputStarted && !lineStartsClientOutput {
				pendingLines = append(pendingLines, line)
				continue
			}
			if !clientOutputStarted && len(pendingLines) > 0 {
				if !writePendingLines() {
					continue
				}
			}
			startStream()
			if _, err := fmt.Fprintln(w, line); err != nil {
				clientDisconnected = true
				logger.LegacyPrintf("service.openai_gateway", "[OpenAI passthrough] Client disconnected during streaming, continue draining upstream for usage: account=%d", account.ID)
			} else {
				clientOutputStarted = true
				flusher.Flush()
			}
		}
	}
	if err := scanner.Err(); err != nil {
		if sawTerminalEvent && !sawFailedEvent {
			return &openaiStreamingResultPassthrough{usage: usage, firstTokenMs: firstTokenMs}, nil
		}
		if sawFailedEvent {
			return &openaiStreamingResultPassthrough{usage: usage, firstTokenMs: firstTokenMs}, fmt.Errorf("upstream response failed: %s", failedMessage)
		}
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return &openaiStreamingResultPassthrough{usage: usage, firstTokenMs: firstTokenMs}, fmt.Errorf("stream usage incomplete: %w", err)
		}
		if errors.Is(err, bufio.ErrTooLong) {
			logger.LegacyPrintf("service.openai_gateway", "[OpenAI passthrough] SSE line too long: account=%d max_size=%d error=%v", account.ID, maxLineSize, err)
			return &openaiStreamingResultPassthrough{usage: usage, firstTokenMs: firstTokenMs}, err
		}
		if !openAIStreamClientOutputStarted(c, clientOutputStarted) {
			msg := "OpenAI stream disconnected before completion"
			if errText := strings.TrimSpace(err.Error()); errText != "" {
				msg += ": " + errText
			}
			return &openaiStreamingResultPassthrough{usage: usage, firstTokenMs: firstTokenMs},
				s.newOpenAIStreamFailoverError(c, account, true, upstreamRequestID, nil, msg)
		}
		if clientDisconnected {
			return &openaiStreamingResultPassthrough{usage: usage, firstTokenMs: firstTokenMs}, fmt.Errorf("stream usage incomplete after disconnect: %w", err)
		}
		logger.LegacyPrintf("service.openai_gateway",
			"[OpenAI passthrough] 流读取异常中断: account=%d request_id=%s err=%v",
			account.ID,
			upstreamRequestID,
			err,
		)
		return &openaiStreamingResultPassthrough{usage: usage, firstTokenMs: firstTokenMs}, fmt.Errorf("stream read error: %w", err)
	}
	if sawFailedEvent {
		return &openaiStreamingResultPassthrough{usage: usage, firstTokenMs: firstTokenMs}, fmt.Errorf("upstream response failed: %s", failedMessage)
	}
	if !clientDisconnected && !sawDone && !sawTerminalEvent && ctx.Err() == nil {
		logger.FromContext(ctx).With(
			zap.String("component", "service.openai_gateway"),
			zap.Int64("account_id", account.ID),
			zap.String("upstream_request_id", upstreamRequestID),
		).Info("OpenAI passthrough 上游流在未收到 [DONE] 时结束，疑似断流")
		if !openAIStreamClientOutputStarted(c, clientOutputStarted) {
			return &openaiStreamingResultPassthrough{usage: usage, firstTokenMs: firstTokenMs},
				s.newOpenAIStreamFailoverError(c, account, true, upstreamRequestID, nil, "OpenAI stream ended before a terminal event")
		}
		return &openaiStreamingResultPassthrough{usage: usage, firstTokenMs: firstTokenMs}, errors.New("stream usage incomplete: missing terminal event")
	}

	return &openaiStreamingResultPassthrough{usage: usage, firstTokenMs: firstTokenMs}, nil
}

func (s *OpenAIGatewayService) handleNonStreamingResponsePassthrough(
	ctx context.Context,
	resp *http.Response,
	c *gin.Context,
	originalModel string,
	mappedModel string,
) (*OpenAIUsage, error) {
	body, err := ReadUpstreamResponseBody(resp.Body, s.cfg, c, openAITooLargeError)
	if err != nil {
		return nil, err
	}

	// Detect SSE responses from upstream and convert to JSON.
	// Some upstreams (e.g. other sub2api instances) may return SSE even when
	// stream=false was requested. Without this conversion the client would
	// receive raw SSE text or a terminal event with empty output.
	if isEventStreamResponse(resp.Header) {
		return s.handlePassthroughSSEToJSON(resp, c, body, originalModel, mappedModel)
	}

	usage := &OpenAIUsage{}
	usageParsed := false
	if len(body) > 0 {
		if parsedUsage, ok := extractOpenAIUsageFromJSONBytes(body); ok {
			*usage = parsedUsage
			usageParsed = true
		}
	}
	if !usageParsed {
		// 兜底：尝试从 SSE 文本中解析 usage
		usage = s.parseSSEUsageFromBody(string(body))
	}

	writeOpenAIPassthroughResponseHeaders(c.Writer.Header(), resp.Header, s.responseHeaderFilter)

	contentType := resp.Header.Get("Content-Type")
	if contentType == "" {
		contentType = "application/json"
	}
	if originalModel != "" && mappedModel != "" && originalModel != mappedModel {
		body = s.replaceModelInResponseBody(body, mappedModel, originalModel)
	}
	c.Data(resp.StatusCode, contentType, body)
	return usage, nil
}

// handlePassthroughSSEToJSON converts an SSE response body into a JSON
// response for the passthrough path. It mirrors handleSSEToJSON while
// preserving passthrough payloads, except compact-only model remapping may
// rewrite model fields back to the original requested model.
func (s *OpenAIGatewayService) handlePassthroughSSEToJSON(resp *http.Response, c *gin.Context, body []byte, originalModel string, mappedModel string) (*OpenAIUsage, error) {
	bodyText := string(body)
	finalResponse, ok := extractCodexFinalResponse(bodyText)

	usage := &OpenAIUsage{}
	if ok {
		if parsedUsage, parsed := extractOpenAIUsageFromJSONBytes(finalResponse); parsed {
			*usage = parsedUsage
		}
		// When the terminal event has an empty output array, reconstruct
		// output from accumulated delta events so the client gets full content.
		if len(gjson.GetBytes(finalResponse, "output").Array()) == 0 {
			if outputJSON, reconstructed := reconstructResponseOutputFromSSE(bodyText); reconstructed {
				if patched, err := sjson.SetRawBytes(finalResponse, "output", outputJSON); err == nil {
					finalResponse = patched
				}
			}
		}
		body = finalResponse
		if originalModel != "" && mappedModel != "" && originalModel != mappedModel {
			body = s.replaceModelInResponseBody(body, mappedModel, originalModel)
		}
		// Correct tool calls in final response
		body = s.correctToolCallsInResponseBody(body)
	} else {
		terminalType, terminalPayload, terminalOK := extractOpenAISSETerminalEvent(bodyText)
		if terminalOK && terminalType == "response.failed" {
			msg := extractOpenAISSEErrorMessage(terminalPayload)
			if msg == "" {
				msg = "Upstream compact response failed"
			}
			return nil, s.writeOpenAINonStreamingProtocolError(resp, c, msg)
		}
		usage = s.parseSSEUsageFromBody(bodyText)
		if originalModel != "" && mappedModel != "" && originalModel != mappedModel {
			bodyText = s.replaceModelInSSEBody(bodyText, mappedModel, originalModel)
		}
		body = []byte(bodyText)
	}

	writeOpenAIPassthroughResponseHeaders(c.Writer.Header(), resp.Header, s.responseHeaderFilter)

	contentType := "application/json; charset=utf-8"
	if !ok {
		contentType = resp.Header.Get("Content-Type")
		if contentType == "" {
			contentType = "text/event-stream"
		}
	}
	c.Data(resp.StatusCode, contentType, body)

	return usage, nil
}

func writeOpenAIPassthroughResponseHeaders(dst http.Header, src http.Header, filter *responseheaders.CompiledHeaderFilter) {
	if dst == nil || src == nil {
		return
	}
	if filter != nil {
		responseheaders.WriteFilteredHeaders(dst, src, filter)
	} else {
		// 兜底：尽量保留最基础的 content-type
		if v := strings.TrimSpace(src.Get("Content-Type")); v != "" {
			dst.Set("Content-Type", v)
		}
	}
	// 透传模式强制放行 x-codex-* 响应头（若上游返回）。
	// 注意：真实 http.Response.Header 的 key 一般会被 canonicalize；但为了兼容测试/自建响应，
	// 这里用 EqualFold 做一次大小写不敏感的查找。
	getCaseInsensitiveValues := func(h http.Header, want string) []string {
		if h == nil {
			return nil
		}
		for k, vals := range h {
			if strings.EqualFold(k, want) {
				return vals
			}
		}
		return nil
	}

	for _, rawKey := range []string{
		"x-codex-primary-used-percent",
		"x-codex-primary-reset-after-seconds",
		"x-codex-primary-window-minutes",
		"x-codex-secondary-used-percent",
		"x-codex-secondary-reset-after-seconds",
		"x-codex-secondary-window-minutes",
		"x-codex-primary-over-secondary-limit-percent",
	} {
		vals := getCaseInsensitiveValues(src, rawKey)
		if len(vals) == 0 {
			continue
		}
		key := http.CanonicalHeaderKey(rawKey)
		dst.Del(key)
		for _, v := range vals {
			dst.Add(key, v)
		}
	}
}

func (s *OpenAIGatewayService) buildUpstreamRequest(ctx context.Context, c *gin.Context, account *Account, body []byte, token string, isStream bool, promptCacheKey string, isCodexCLI bool) (*http.Request, error) {
	// Determine target URL based on account type
	var targetURL string
	switch account.Type {
	case AccountTypeOAuth:
		// OAuth accounts use ChatGPT internal API
		targetURL = chatgptCodexURL
	case AccountTypeAPIKey:
		// API Key accounts use Platform API or custom base URL
		baseURL := account.GetOpenAIBaseURL()
		if baseURL == "" {
			targetURL = openaiPlatformAPIURL
		} else {
			validatedURL, err := s.validateUpstreamBaseURL(baseURL)
			if err != nil {
				return nil, err
			}
			targetURL = buildOpenAIResponsesURL(validatedURL)
		}
	default:
		targetURL = openaiPlatformAPIURL
	}
	targetURL = appendOpenAIResponsesRequestPathSuffix(targetURL, openAIResponsesRequestPathSuffix(c))

	req, err := http.NewRequestWithContext(ctx, "POST", targetURL, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	sessionSignal := ""
	if account.Type == AccountTypeOAuth {
		sessionSignal = extractOpenAISessionSignal(c, body, false)
		if sessionSignal == "" {
			sessionSignal = strings.TrimSpace(promptCacheKey)
		}
	}

	// Set authentication header
	req.Header.Set("authorization", "Bearer "+token)

	// Set headers specific to OAuth accounts (ChatGPT internal API)
	if account.Type == AccountTypeOAuth {
		// Required: set Host for ChatGPT API (must use req.Host, not Header.Set)
		req.Host = "chatgpt.com"
		// Required: set chatgpt-account-id header
		chatgptAccountID := account.GetChatGPTAccountID()
		if chatgptAccountID != "" {
			req.Header.Set("chatgpt-account-id", chatgptAccountID)
		}
	}

	// Whitelist passthrough headers
	for key, values := range c.Request.Header {
		lowerKey := strings.ToLower(key)
		if openaiAllowedHeaders[lowerKey] {
			for _, v := range values {
				req.Header.Add(key, v)
			}
		}
	}
	if account.Type == AccountTypeOAuth {
		// 清除客户端透传的 session 头，后续用隔离后的值重新设置，防止跨用户会话碰撞。
		req.Header.Del("conversation_id")
		req.Header.Del("session_id")

		req.Header.Set("OpenAI-Beta", "responses=experimental")
		req.Header.Set("originator", resolveOpenAIUpstreamOriginator(c, isCodexCLI))
		apiKeyID := getAPIKeyIDFromContext(c)
		clientSessionID := sessionSignal
		clientConversationID := sessionSignal
		if isOpenAIResponsesCompactPath(c) {
			req.Header.Set("accept", "application/json")
			if req.Header.Get("version") == "" {
				req.Header.Set("version", codexCLIVersion)
			}
			if clientSessionID == "" {
				clientSessionID = resolveOpenAICompactSessionID(c, body)
			}
		} else if shouldRequestJSONForOAuthNonStreamIncludeSources(c, account, body, isStream) {
			req.Header.Set("accept", "application/json")
		} else {
			req.Header.Set("accept", "text/event-stream")
		}
		if clientSessionID != "" {
			req.Header.Set("session_id", isolateOpenAISessionID(apiKeyID, clientSessionID))
		}
		if clientConversationID != "" {
			req.Header.Set("conversation_id", isolateOpenAISessionID(apiKeyID, clientConversationID))
		}
	}

	// Apply custom User-Agent if configured
	customUA := account.GetOpenAIUserAgent()
	if customUA != "" {
		req.Header.Set("user-agent", customUA)
	}

	// 若开启 ForceCodexCLI，则强制将上游 User-Agent 伪装为 Codex CLI。
	// 用于网关未透传/改写 User-Agent 时，仍能命中 Codex 侧识别逻辑。
	if s.cfg != nil && s.cfg.Gateway.ForceCodexCLI {
		req.Header.Set("user-agent", codexCLIUserAgent)
	}

	// Ensure required headers exist
	if req.Header.Get("content-type") == "" {
		req.Header.Set("content-type", "application/json")
	}

	return req, nil
}

func (s *OpenAIGatewayService) handleErrorResponse(
	ctx context.Context,
	resp *http.Response,
	c *gin.Context,
	account *Account,
	requestBody []byte,
) (*OpenAIForwardResult, error) {
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 2<<20))

	upstreamMsg := strings.TrimSpace(extractUpstreamErrorMessage(body))
	upstreamMsg = sanitizeUpstreamErrorMessage(upstreamMsg)
	upstreamDetail := ""
	if s.cfg != nil && s.cfg.Gateway.LogUpstreamErrorBody {
		maxBytes := s.cfg.Gateway.LogUpstreamErrorBodyMaxBytes
		if maxBytes <= 0 {
			maxBytes = 2048
		}
		upstreamDetail = truncateString(string(body), maxBytes)
	}
	setOpsUpstreamError(c, resp.StatusCode, upstreamMsg, upstreamDetail)
	logOpenAIInstructionsRequiredDebug(ctx, c, account, resp.StatusCode, upstreamMsg, requestBody, body)

	if s.cfg != nil && s.cfg.Gateway.LogUpstreamErrorBody {
		logger.LegacyPrintf("service.openai_gateway",
			"OpenAI upstream error %d (account=%d platform=%s type=%s): %s",
			resp.StatusCode,
			account.ID,
			account.Platform,
			account.Type,
			truncateForLog(body, s.cfg.Gateway.LogUpstreamErrorBodyMaxBytes),
		)
	}

	if status, errType, errMsg, matched := applyErrorPassthroughRule(
		c,
		PlatformOpenAI,
		resp.StatusCode,
		body,
		http.StatusBadGateway,
		"upstream_error",
		"Upstream request failed",
	); matched {
		c.JSON(status, gin.H{
			"error": gin.H{
				"type":    errType,
				"message": errMsg,
			},
		})
		if upstreamMsg == "" {
			upstreamMsg = errMsg
		}
		if upstreamMsg == "" {
			return nil, fmt.Errorf("upstream error: %d (passthrough rule matched)", resp.StatusCode)
		}
		return nil, fmt.Errorf("upstream error: %d (passthrough rule matched) message=%s", resp.StatusCode, upstreamMsg)
	}

	// Check custom error codes
	if !account.ShouldHandleErrorCode(resp.StatusCode) {
		appendOpsUpstreamError(c, OpsUpstreamErrorEvent{
			Platform:           account.Platform,
			AccountID:          account.ID,
			AccountName:        account.Name,
			UpstreamStatusCode: resp.StatusCode,
			UpstreamRequestID:  resp.Header.Get("x-request-id"),
			Kind:               "http_error",
			Message:            upstreamMsg,
			Detail:             upstreamDetail,
		})
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": gin.H{
				"type":    "upstream_error",
				"message": "Upstream gateway error",
			},
		})
		if upstreamMsg == "" {
			return nil, fmt.Errorf("upstream error: %d (not in custom error codes)", resp.StatusCode)
		}
		return nil, fmt.Errorf("upstream error: %d (not in custom error codes) message=%s", resp.StatusCode, upstreamMsg)
	}

	// Handle upstream error (mark account status)
	shouldDisable := false
	if s.rateLimitService != nil {
		shouldDisable = s.rateLimitService.HandleUpstreamError(ctx, account, resp.StatusCode, resp.Header, body)
	}
	kind := "http_error"
	if shouldDisable {
		kind = "failover"
	}
	appendOpsUpstreamError(c, OpsUpstreamErrorEvent{
		Platform:           account.Platform,
		AccountID:          account.ID,
		AccountName:        account.Name,
		UpstreamStatusCode: resp.StatusCode,
		UpstreamRequestID:  resp.Header.Get("x-request-id"),
		Kind:               kind,
		Message:            upstreamMsg,
		Detail:             upstreamDetail,
	})
	if shouldDisable {
		return nil, &UpstreamFailoverError{
			StatusCode:             resp.StatusCode,
			ResponseBody:           body,
			RetryableOnSameAccount: account.IsPoolMode() && isPoolModeRetryableStatus(resp.StatusCode),
		}
	}

	if hasOpenAIStructuredError(body) {
		statusCode, envelope := buildOpenAIUpstreamErrorEnvelope(resp.StatusCode, body, upstreamMsg)
		c.JSON(statusCode, envelope)
		return nil, fmt.Errorf("upstream error: %d message=%s", resp.StatusCode, upstreamMsg)
	}

	// Return appropriate error response
	var errType, errMsg string
	var statusCode int

	switch resp.StatusCode {
	case 401:
		statusCode = http.StatusBadGateway
		errType = "upstream_error"
		errMsg = "Upstream authentication failed, please contact administrator"
	case 402:
		statusCode = http.StatusBadGateway
		errType = "upstream_error"
		errMsg = "Upstream payment required: insufficient balance or billing issue"
	case 403:
		statusCode = http.StatusBadGateway
		errType = "upstream_error"
		errMsg = "Upstream access forbidden, please contact administrator"
	case 429:
		statusCode = http.StatusTooManyRequests
		errType = "rate_limit_error"
		errMsg = "Upstream rate limit exceeded, please retry later"
	default:
		statusCode = http.StatusBadGateway
		errType = "upstream_error"
		errMsg = "Upstream request failed"
	}

	c.JSON(statusCode, gin.H{
		"error": gin.H{
			"type":    errType,
			"message": errMsg,
		},
	})

	if upstreamMsg == "" {
		return nil, fmt.Errorf("upstream error: %d", resp.StatusCode)
	}
	return nil, fmt.Errorf("upstream error: %d message=%s", resp.StatusCode, upstreamMsg)
}

// compatErrorWriter is the signature for format-specific error writers used by
// the compat paths (Chat Completions and Anthropic Messages).
type compatErrorWriter func(c *gin.Context, statusCode int, errType, message string)

// handleCompatErrorResponse is the shared non-failover error handler for the
// Chat Completions and Anthropic Messages compat paths. It mirrors the logic of
// handleErrorResponse (passthrough rules, ShouldHandleErrorCode, rate-limit
// tracking, secondary failover) but delegates the final error write to the
// format-specific writer function.
func (s *OpenAIGatewayService) handleCompatErrorResponse(
	resp *http.Response,
	c *gin.Context,
	account *Account,
	writeError compatErrorWriter,
) (*OpenAIForwardResult, error) {
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 2<<20))

	upstreamMsg := strings.TrimSpace(extractUpstreamErrorMessage(body))
	if upstreamMsg == "" {
		upstreamMsg = fmt.Sprintf("Upstream error: %d", resp.StatusCode)
	}
	upstreamMsg = sanitizeUpstreamErrorMessage(upstreamMsg)

	upstreamDetail := ""
	if s.cfg != nil && s.cfg.Gateway.LogUpstreamErrorBody {
		maxBytes := s.cfg.Gateway.LogUpstreamErrorBodyMaxBytes
		if maxBytes <= 0 {
			maxBytes = 2048
		}
		upstreamDetail = truncateString(string(body), maxBytes)
	}
	setOpsUpstreamError(c, resp.StatusCode, upstreamMsg, upstreamDetail)

	// Apply error passthrough rules
	if status, errType, errMsg, matched := applyErrorPassthroughRule(
		c, account.Platform, resp.StatusCode, body,
		http.StatusBadGateway, "api_error", "Upstream request failed",
	); matched {
		writeError(c, status, errType, errMsg)
		if upstreamMsg == "" {
			upstreamMsg = errMsg
		}
		if upstreamMsg == "" {
			return nil, fmt.Errorf("upstream error: %d (passthrough rule matched)", resp.StatusCode)
		}
		return nil, fmt.Errorf("upstream error: %d (passthrough rule matched) message=%s", resp.StatusCode, upstreamMsg)
	}

	// Check custom error codes — if the account does not handle this status,
	// return a generic error without exposing upstream details.
	if !account.ShouldHandleErrorCode(resp.StatusCode) {
		appendOpsUpstreamError(c, OpsUpstreamErrorEvent{
			Platform:           account.Platform,
			AccountID:          account.ID,
			AccountName:        account.Name,
			UpstreamStatusCode: resp.StatusCode,
			UpstreamRequestID:  resp.Header.Get("x-request-id"),
			Kind:               "http_error",
			Message:            upstreamMsg,
			Detail:             upstreamDetail,
		})
		writeError(c, http.StatusInternalServerError, "api_error", "Upstream gateway error")
		if upstreamMsg == "" {
			return nil, fmt.Errorf("upstream error: %d (not in custom error codes)", resp.StatusCode)
		}
		return nil, fmt.Errorf("upstream error: %d (not in custom error codes) message=%s", resp.StatusCode, upstreamMsg)
	}

	// Track rate limits and decide whether to trigger secondary failover.
	shouldDisable := false
	if s.rateLimitService != nil {
		shouldDisable = s.rateLimitService.HandleUpstreamError(
			c.Request.Context(), account, resp.StatusCode, resp.Header, body,
		)
	}
	kind := "http_error"
	if shouldDisable {
		kind = "failover"
	}
	appendOpsUpstreamError(c, OpsUpstreamErrorEvent{
		Platform:           account.Platform,
		AccountID:          account.ID,
		AccountName:        account.Name,
		UpstreamStatusCode: resp.StatusCode,
		UpstreamRequestID:  resp.Header.Get("x-request-id"),
		Kind:               kind,
		Message:            upstreamMsg,
		Detail:             upstreamDetail,
	})
	if shouldDisable {
		return nil, &UpstreamFailoverError{
			StatusCode:             resp.StatusCode,
			ResponseBody:           body,
			RetryableOnSameAccount: account.IsPoolMode() && isPoolModeRetryableStatus(resp.StatusCode),
		}
	}

	// Map status code to error type and write response
	errType := "api_error"
	switch {
	case resp.StatusCode == 400:
		errType = "invalid_request_error"
	case resp.StatusCode == 404:
		errType = "not_found_error"
	case resp.StatusCode == 429:
		errType = "rate_limit_error"
	case resp.StatusCode >= 500:
		errType = "api_error"
	}

	writeError(c, resp.StatusCode, errType, upstreamMsg)
	return nil, fmt.Errorf("upstream error: %d %s", resp.StatusCode, upstreamMsg)
}

// openaiStreamingResult streaming response result
type openaiStreamingResult struct {
	usage        *OpenAIUsage
	firstTokenMs *int
}

func (s *OpenAIGatewayService) handleStreamingResponse(ctx context.Context, resp *http.Response, c *gin.Context, account *Account, startTime time.Time, originalModel, mappedModel string) (*openaiStreamingResult, error) {
	return s.handleStreamingResponseWithOpenCodeContinuation(ctx, resp, c, account, startTime, originalModel, mappedModel, nil)
}

func (s *OpenAIGatewayService) handleStreamingResponseWithOpenCodeContinuation(ctx context.Context, resp *http.Response, c *gin.Context, account *Account, startTime time.Time, originalModel, mappedModel string, continuation *openCodeImageServerContinuationContext) (*openaiStreamingResult, error) {
	w := c.Writer
	flusher, ok := w.(http.Flusher)
	if !ok {
		return nil, errors.New("streaming not supported")
	}
	bufferedWriter := bufio.NewWriterSize(w, 4*1024)
	flushBuffered := func() error {
		if err := bufferedWriter.Flush(); err != nil {
			return err
		}
		flusher.Flush()
		return nil
	}

	usage := &OpenAIUsage{}
	var firstTokenMs *int
	scanner := bufio.NewScanner(resp.Body)
	maxLineSize := defaultMaxLineSize
	if s.cfg != nil && s.cfg.Gateway.MaxLineSize > 0 {
		maxLineSize = s.cfg.Gateway.MaxLineSize
	}
	scanBuf := getSSEScannerBuf64K()
	scanner.Buffer(scanBuf[:0], maxLineSize)

	streamInterval := time.Duration(0)
	if s.cfg != nil && s.cfg.Gateway.StreamDataIntervalTimeout > 0 {
		streamInterval = time.Duration(s.cfg.Gateway.StreamDataIntervalTimeout) * time.Second
	}
	// 仅监控上游数据间隔超时，不被下游写入阻塞影响
	var intervalTicker *time.Ticker
	if streamInterval > 0 {
		intervalTicker = time.NewTicker(streamInterval)
		defer intervalTicker.Stop()
	}
	var intervalCh <-chan time.Time
	if intervalTicker != nil {
		intervalCh = intervalTicker.C
	}

	keepaliveInterval := time.Duration(0)
	if s.cfg != nil && s.cfg.Gateway.StreamKeepaliveInterval > 0 {
		keepaliveInterval = time.Duration(s.cfg.Gateway.StreamKeepaliveInterval) * time.Second
	}
	// 下游 keepalive 仅用于防止代理空闲断开
	var keepaliveTicker *time.Ticker
	if keepaliveInterval > 0 {
		keepaliveTicker = time.NewTicker(keepaliveInterval)
		defer keepaliveTicker.Stop()
	}
	var keepaliveCh <-chan time.Time
	if keepaliveTicker != nil {
		keepaliveCh = keepaliveTicker.C
	}
	// Track downstream writes separately from upstream reads: pre-output failover
	// can buffer response.created / response.in_progress, so keepalive must be
	// based on downstream idle time.
	lastDownstreamWriteAt := time.Now()

	// 仅发送一次错误事件，避免多次写入导致协议混乱。
	// 注意：OpenAI `/v1/responses` streaming 事件必须符合 OpenAI Responses schema；
	// 否则下游 SDK（例如 OpenCode）会因为类型校验失败而报错。
	errorEventSent := false
	clientDisconnected := false // 客户端断开后继续 drain 上游以收集 usage
	sawTerminalEvent := false
	streamStarted := false
	sawResponseCreated := false
	sawResponseInProgress := false
	sawSubstantiveEvent := false
	openCodeClient := isOpenCodeResponsesClient(c)
	openCodeGeneratedMessages := make([]openCodeImageGeneratedMessage, 0, 1)
	openCodeContinuationReady := false
	openCodeSuppressedTerminalFrame := ""
	openCodeSuppressedDoneFrame := ""
	openCodeImageOpts := openCodeImageRewriteOptions{}
	if openCodeClient {
		openCodeImageOpts.BaseURL = s.resolveOpenCodeImageDownloadBaseURL(c.Request.Context(), c)
		openCodeImageOpts.RewrittenImageMessages = make(map[string]map[string]any)
		openCodeImageOpts.GeneratedMessages = &openCodeGeneratedMessages
	}
	pendingOpenCodeFrame := make([]string, 0, 4)
	localRequestID := strings.TrimSpace(c.Writer.Header().Get("X-Request-Id"))
	startStream := func() error {
		if streamStarted {
			return nil
		}
		if s.responseHeaderFilter != nil {
			responseheaders.WriteFilteredHeaders(c.Writer.Header(), resp.Header, s.responseHeaderFilter)
		}
		c.Header("Content-Type", "text/event-stream")
		c.Header("Cache-Control", "no-cache")
		c.Header("Connection", "keep-alive")
		c.Header("X-Accel-Buffering", "no")
		if v := resp.Header.Get("x-request-id"); v != "" {
			c.Header("x-request-id", v)
		}
		c.Writer.WriteHeader(http.StatusOK)
		streamStarted = true
		if localRequestID == "" {
			localRequestID = strings.TrimSpace(c.Writer.Header().Get("X-Request-Id"))
		}
		return nil
	}
	sawFailedEvent := false
	failedMessage := ""
	clientOutputStarted := false
	upstreamRequestID := strings.TrimSpace(resp.Header.Get("x-request-id"))
	sendErrorEvent := func(reason string) {
		if errorEventSent || clientDisconnected {
			return
		}
		errorEventSent = true
		errorJSON := gin.H{
			"type":                "upstream_error",
			"message":             reason,
			"code":                reason,
			"upstream_request_id": strings.TrimSpace(resp.Header.Get("x-request-id")),
		}
		if localRequestID != "" {
			errorJSON["request_id"] = localRequestID
		}
		payloadBytes, _ := json.Marshal(gin.H{"type": "error", "sequence_number": 0, "error": errorJSON})
		payload := string(payloadBytes)
		if err := startStream(); err != nil {
			clientDisconnected = true
			return
		}
		if err := flushBuffered(); err != nil {
			clientDisconnected = true
			return
		}
		if _, err := bufferedWriter.WriteString("data: " + payload + "\n\n"); err != nil {
			clientDisconnected = true
			return
		}
		if err := flushBuffered(); err != nil {
			clientDisconnected = true
			return
		}
		clientOutputStarted = true
		lastDownstreamWriteAt = time.Now()
	}

	needModelReplace := originalModel != mappedModel
	resultWithUsage := func() *openaiStreamingResult {
		return &openaiStreamingResult{usage: usage, firstTokenMs: firstTokenMs}
	}
	flushOpenCodeSuppressedTerminal := func() {
		if clientDisconnected || (openCodeSuppressedTerminalFrame == "" && openCodeSuppressedDoneFrame == "") {
			return
		}
		if err := startStream(); err != nil {
			clientDisconnected = true
			return
		}
		if openCodeSuppressedTerminalFrame != "" {
			if _, err := bufferedWriter.WriteString(openCodeSuppressedTerminalFrame); err != nil {
				clientDisconnected = true
				return
			}
		}
		if openCodeSuppressedDoneFrame != "" {
			if _, err := bufferedWriter.WriteString(openCodeSuppressedDoneFrame); err != nil {
				clientDisconnected = true
				return
			}
		}
		if err := flushBuffered(); err != nil {
			clientDisconnected = true
			return
		}
		clientOutputStarted = true
		lastDownstreamWriteAt = time.Now()
	}
	finalizeStream := func() (*openaiStreamingResult, error) {
		if !sawTerminalEvent {
			if !openAIStreamClientOutputStarted(c, clientOutputStarted) {
				return resultWithUsage(), s.newOpenAIStreamFailoverError(
					c,
					account,
					false,
					upstreamRequestID,
					nil,
					"OpenAI stream ended before a terminal event",
				)
			}
			return resultWithUsage(), fmt.Errorf("stream usage incomplete: missing terminal event")
		}
		if sawFailedEvent {
			return resultWithUsage(), fmt.Errorf("upstream response failed: %s", failedMessage)
		}
		if openCodeContinuationReady && continuation != nil && !clientDisconnected {
			continued, err := s.openCodeImageStreamingServerContinuation(ctx, c, account, startTime, originalModel, mappedModel, continuation, openCodeGeneratedMessages)
			if err != nil {
				flushOpenCodeSuppressedTerminal()
				return resultWithUsage(), err
			}
			if continued != nil {
				addOpenAIUsage(usage, continued.usage)
				if firstTokenMs == nil && continued.firstTokenMs != nil {
					firstTokenMs = continued.firstTokenMs
				}
			}
		}
		if !clientDisconnected {
			hadBufferedData := bufferedWriter.Buffered() > 0
			if err := flushBuffered(); err != nil {
				clientDisconnected = true
				logger.LegacyPrintf("service.openai_gateway", "Client disconnected during final flush, returning collected usage")
			} else if hadBufferedData {
				clientOutputStarted = true
				lastDownstreamWriteAt = time.Now()
			}
		}
		return resultWithUsage(), nil
	}
	handleScanErr := func(scanErr error) (*openaiStreamingResult, error, bool) {
		if scanErr == nil {
			return nil, nil, false
		}
		if sawTerminalEvent && !sawFailedEvent {
			logger.LegacyPrintf("service.openai_gateway", "Upstream scan ended after terminal event: %v", scanErr)
			return resultWithUsage(), nil, true
		}
		if sawFailedEvent {
			return resultWithUsage(), fmt.Errorf("upstream response failed: %s", failedMessage), true
		}
		// 客户端断开/取消请求时，上游读取往往会返回 context canceled。
		// /v1/responses 的 SSE 事件必须符合 OpenAI 协议；这里不注入自定义 error event，避免下游 SDK 解析失败。
		if errors.Is(scanErr, context.Canceled) || errors.Is(scanErr, context.DeadlineExceeded) {
			return resultWithUsage(), fmt.Errorf("stream usage incomplete: %w", scanErr), true
		}
		if errors.Is(scanErr, bufio.ErrTooLong) {
			logger.LegacyPrintf("service.openai_gateway", "SSE line too long: account=%d max_size=%d error=%v", account.ID, maxLineSize, scanErr)
			sendErrorEvent("response_too_large")
			return resultWithUsage(), scanErr, true
		}
		if !sawSubstantiveEvent {
			return resultWithUsage(), &UpstreamFailoverError{
				StatusCode:   http.StatusBadGateway,
				ResponseBody: []byte(scanErr.Error()),
			}, true
		}
		logger.FromContext(ctx).Warn("openai stream read error after stream start",
			zap.String("request_id", localRequestID),
			zap.String("upstream_request_id", strings.TrimSpace(resp.Header.Get("x-request-id"))),
			zap.Bool("stream_started", streamStarted),
			zap.Bool("saw_response_created", sawResponseCreated),
			zap.Bool("saw_response_in_progress", sawResponseInProgress),
			zap.Bool("saw_substantive_event", sawSubstantiveEvent),
			zap.Bool("saw_terminal_event", sawTerminalEvent),
			zap.Error(scanErr),
		)
		sendErrorEvent("stream_read_error")
		return resultWithUsage(), fmt.Errorf("stream read error: %w", scanErr), true
	}
	processOpenCodeSSEFrame := func(frameLines []string, queueDrained bool) (*openaiStreamingResult, error, bool) {
		frameBody, data, hasData, keep := filterOpenCodeResponsesSSEFrameWithImages(ctx, frameLines, s.generatedImageStore, openCodeImageOpts)
		generatedAfter := len(openCodeGeneratedMessages)
		if !keep {
			return nil, nil, false
		}
		if !hasData {
			if !clientDisconnected && streamStarted && frameBody != "" {
				if _, err := bufferedWriter.WriteString(frameBody); err != nil {
					clientDisconnected = true
					logger.LegacyPrintf("service.openai_gateway", "Client disconnected during streaming, continuing to drain upstream for billing")
				} else if queueDrained {
					if err := flushBuffered(); err != nil {
						clientDisconnected = true
						logger.LegacyPrintf("service.openai_gateway", "Client disconnected during streaming flush, continuing to drain upstream for billing")
					}
				}
			}
			return nil, nil, false
		}

		frameEventType := openCodeSSEFrameEventType(frameLines)
		eventType := strings.TrimSpace(gjson.Get(data, "type").String())
		if eventType == "" {
			eventType = frameEventType
		}
		switch eventType {
		case "response.created":
			sawResponseCreated = true
		case "response.in_progress":
			sawResponseInProgress = true
		}
		if !streamStarted && firstTokenMs == nil && gjson.Get(data, "type").String() == "error" {
			msg := extractOpenAISSEErrorMessage([]byte(data))
			if isOpenAITransientProcessingError(http.StatusBadRequest, msg, []byte(data)) {
				return resultWithUsage(), &UpstreamFailoverError{StatusCode: http.StatusServiceUnavailable, ResponseBody: []byte(data)}, true
			}
		}
		if continuation != nil && strings.TrimSpace(data) == "[DONE]" && openCodeContinuationReady {
			openCodeSuppressedDoneFrame = frameBody
			return nil, nil, false
		}

		if needModelReplace && mappedModel != "" && strings.Contains(data, mappedModel) {
			replacedLine := s.replaceModelInSSELine("data: "+data, mappedModel, originalModel)
			if replacedData, ok := extractOpenAISSEDataLine(replacedLine); ok {
				oldData := data
				data = replacedData
				frameBody = replaceSSEFrameDataPayload(frameBody, oldData, data, frameLines)
				eventType = strings.TrimSpace(gjson.Get(data, "type").String())
				if eventType == "" {
					eventType = frameEventType
				}
			}
		}
		if continuation != nil && generatedAfter > 0 && (eventType == "response.completed" || eventType == "response.done") {
			openCodeContinuationReady = true
			sawTerminalEvent = true
			s.parseSSEUsageBytesWithEventType([]byte(data), eventType, usage)
			clientFrames, terminalFrame := splitOpenCodeTerminalContinuationFrames(frameBody)
			if terminalFrame != "" {
				openCodeSuppressedTerminalFrame = terminalFrame
			}
			frameBody = clientFrames
			if strings.TrimSpace(frameBody) == "" {
				return nil, nil, false
			}
		}

		dataBytes := []byte(data)
		if openAIStreamEventIsTerminal(data) || isOpenAITerminalResponseEventType(eventType) {
			sawTerminalEvent = true
		}
		forceFlushFailedEvent := false
		if eventType == "response.failed" {
			failedMessage = extractOpenAISSEErrorMessage(dataBytes)
			if !openAIStreamClientOutputStarted(c, clientOutputStarted) && openAIStreamFailedEventShouldFailover(dataBytes, failedMessage) {
				sawFailedEvent = true
				return resultWithUsage(), s.newOpenAIStreamFailoverError(c, account, false, upstreamRequestID, dataBytes, failedMessage), true
			}
			forceFlushFailedEvent = true
			sawFailedEvent = true
		}
		if correctedData, corrected := s.toolCorrector.CorrectToolCallsInSSEBytes(dataBytes); corrected {
			oldData := data
			dataBytes = correctedData
			data = string(correctedData)
			frameBody = replaceSSEFrameDataPayload(frameBody, oldData, data, frameLines)
			eventType = strings.TrimSpace(gjson.GetBytes(dataBytes, "type").String())
			if eventType == "" {
				eventType = frameEventType
			}
		}
		startsClientOutput := forceFlushFailedEvent || openAIStreamDataStartsClientOutput(data, eventType)

		if !clientDisconnected {
			if !clientOutputStarted && !startsClientOutput {
				s.parseSSEUsageBytesWithEventType(dataBytes, eventType, usage)
				return nil, nil, false
			}
			sawSubstantiveEvent = true
			shouldFlush := queueDrained && (clientOutputStarted || startsClientOutput)
			if firstTokenMs == nil && startsClientOutput && data != "" && data != "[DONE]" {
				shouldFlush = true
				ms := int(time.Since(startTime).Milliseconds())
				firstTokenMs = &ms
			}
			if err := startStream(); err != nil {
				clientDisconnected = true
				return resultWithUsage(), err, true
			}
			if _, err := bufferedWriter.WriteString(frameBody); err != nil {
				clientDisconnected = true
				logger.LegacyPrintf("service.openai_gateway", "Client disconnected during streaming, continuing to drain upstream for billing")
			} else {
				if startsClientOutput {
					clientOutputStarted = true
					lastDownstreamWriteAt = time.Now()
				}
				if shouldFlush {
					if err := flushBuffered(); err != nil {
						clientDisconnected = true
						logger.LegacyPrintf("service.openai_gateway", "Client disconnected during streaming flush, continuing to drain upstream for billing")
					} else if startsClientOutput {
						lastDownstreamWriteAt = time.Now()
					}
				}
			}
		}
		s.parseSSEUsageBytesWithEventType(dataBytes, eventType, usage)
		return nil, nil, false
	}

	processSSELine := func(line string, queueDrained bool) (*openaiStreamingResult, error, bool) {
		if openCodeClient {
			pendingOpenCodeFrame = append(pendingOpenCodeFrame, line)
			if line != "" {
				return nil, nil, false
			}
			frameLines := append([]string(nil), pendingOpenCodeFrame...)
			pendingOpenCodeFrame = pendingOpenCodeFrame[:0]
			return processOpenCodeSSEFrame(frameLines, queueDrained)
		}
		// Extract data from SSE line (supports both "data: " and "data:" formats)
		if data, ok := extractOpenAISSEDataLine(line); ok {
			switch strings.TrimSpace(gjson.Get(data, "type").String()) {
			case "response.created":
				sawResponseCreated = true
			case "response.in_progress":
				sawResponseInProgress = true
			}
			if !streamStarted && firstTokenMs == nil && gjson.Get(data, "type").String() == "error" {
				msg := extractOpenAISSEErrorMessage([]byte(data))
				if isOpenAITransientProcessingError(http.StatusBadRequest, msg, []byte(data)) {
					return resultWithUsage(), &UpstreamFailoverError{StatusCode: http.StatusServiceUnavailable, ResponseBody: []byte(data)}, true
				}
			}

			// Replace model in response if needed.
			// Fast path: most events do not contain model field values.
			if needModelReplace && mappedModel != "" && strings.Contains(data, mappedModel) {
				line = s.replaceModelInSSELine(line, mappedModel, originalModel)
			}

			dataBytes := []byte(data)
			if openAIStreamEventIsTerminal(data) {
				sawTerminalEvent = true
			}
			eventType := strings.TrimSpace(gjson.GetBytes(dataBytes, "type").String())
			forceFlushFailedEvent := false
			if eventType == "response.failed" {
				failedMessage = extractOpenAISSEErrorMessage(dataBytes)
				if !openAIStreamClientOutputStarted(c, clientOutputStarted) && openAIStreamFailedEventShouldFailover(dataBytes, failedMessage) {
					sawFailedEvent = true
					return resultWithUsage(), s.newOpenAIStreamFailoverError(c, account, false, upstreamRequestID, dataBytes, failedMessage), true
				}
				forceFlushFailedEvent = true
				sawFailedEvent = true
			}

			// Correct Codex tool calls if needed (apply_patch -> edit, etc.)
			if correctedData, corrected := s.toolCorrector.CorrectToolCallsInSSEBytes(dataBytes); corrected {
				dataBytes = correctedData
				data = string(correctedData)
				line = "data: " + data
				eventType = strings.TrimSpace(gjson.GetBytes(dataBytes, "type").String())
			}
			startsClientOutput := forceFlushFailedEvent || openAIStreamDataStartsClientOutput(data, eventType)

			// 写入客户端（客户端断开后继续 drain 上游）
			if !clientDisconnected {
				if !clientOutputStarted && !startsClientOutput {
					s.parseSSEUsageBytes(dataBytes, usage)
					return nil, nil, false
				}
				sawSubstantiveEvent = true
				shouldFlush := queueDrained && (clientOutputStarted || startsClientOutput)
				if firstTokenMs == nil && startsClientOutput {
					// 保证首个 token 事件尽快出站，避免影响 TTFT。
					shouldFlush = true
					ms := int(time.Since(startTime).Milliseconds())
					firstTokenMs = &ms
				}
				if err := startStream(); err != nil {
					clientDisconnected = true
					return resultWithUsage(), err, true
				}
				if _, err := bufferedWriter.WriteString(line); err != nil {
					clientDisconnected = true
					logger.LegacyPrintf("service.openai_gateway", "Client disconnected during streaming, continuing to drain upstream for billing")
				} else if _, err := bufferedWriter.WriteString("\n"); err != nil {
					clientDisconnected = true
					logger.LegacyPrintf("service.openai_gateway", "Client disconnected during streaming, continuing to drain upstream for billing")
				} else {
					if startsClientOutput {
						clientOutputStarted = true
						lastDownstreamWriteAt = time.Now()
					}
					if shouldFlush {
						if err := flushBuffered(); err != nil {
							clientDisconnected = true
							logger.LegacyPrintf("service.openai_gateway", "Client disconnected during streaming flush, continuing to drain upstream for billing")
						} else if startsClientOutput {
							lastDownstreamWriteAt = time.Now()
						}
					}
				}
			}
			s.parseSSEUsageBytes(dataBytes, usage)
			return nil, nil, false
		}

		// Forward non-data lines as-is
		if !clientDisconnected && streamStarted {
			if _, err := bufferedWriter.WriteString(line); err != nil {
				clientDisconnected = true
				logger.LegacyPrintf("service.openai_gateway", "Client disconnected during streaming, continuing to drain upstream for billing")
			} else if _, err := bufferedWriter.WriteString("\n"); err != nil {
				clientDisconnected = true
				logger.LegacyPrintf("service.openai_gateway", "Client disconnected during streaming, continuing to drain upstream for billing")
			} else {
				if queueDrained && clientOutputStarted {
					if err := flushBuffered(); err != nil {
						clientDisconnected = true
						logger.LegacyPrintf("service.openai_gateway", "Client disconnected during streaming flush, continuing to drain upstream for billing")
					} else {
						lastDownstreamWriteAt = time.Now()
					}
				}
			}
		}
		return nil, nil, false
	}

	// 无超时/无 keepalive 的常见路径走同步扫描，减少 goroutine 与 channel 开销。
	if streamInterval <= 0 && keepaliveInterval <= 0 {
		defer putSSEScannerBuf64K(scanBuf)
		for scanner.Scan() {
			if result, err, done := processSSELine(scanner.Text(), true); done {
				return result, err
			}
		}
		if openCodeClient && len(pendingOpenCodeFrame) > 0 {
			if result, err, done := processOpenCodeSSEFrame(append([]string(nil), pendingOpenCodeFrame...), true); done {
				return result, err
			}
			pendingOpenCodeFrame = pendingOpenCodeFrame[:0]
		}
		if result, err, done := handleScanErr(scanner.Err()); done {
			return result, err
		}
		return finalizeStream()
	}

	type scanEvent struct {
		line string
		err  error
	}
	// 独立 goroutine 读取上游，避免读取阻塞影响 keepalive/超时处理
	events := make(chan scanEvent, 16)
	done := make(chan struct{})
	sendEvent := func(ev scanEvent) bool {
		select {
		case events <- ev:
			return true
		case <-done:
			return false
		}
	}
	var lastReadAt int64
	atomic.StoreInt64(&lastReadAt, time.Now().UnixNano())
	go func(scanBuf *sseScannerBuf64K) {
		defer putSSEScannerBuf64K(scanBuf)
		defer close(events)
		for scanner.Scan() {
			atomic.StoreInt64(&lastReadAt, time.Now().UnixNano())
			if !sendEvent(scanEvent{line: scanner.Text()}) {
				return
			}
		}
		if err := scanner.Err(); err != nil {
			_ = sendEvent(scanEvent{err: err})
		}
	}(scanBuf)
	defer close(done)

	for {
		select {
		case ev, ok := <-events:
			if !ok {
				if openCodeClient && len(pendingOpenCodeFrame) > 0 {
					if result, err, done := processOpenCodeSSEFrame(append([]string(nil), pendingOpenCodeFrame...), true); done {
						return result, err
					}
					pendingOpenCodeFrame = pendingOpenCodeFrame[:0]
				}
				return finalizeStream()
			}
			if result, err, done := handleScanErr(ev.err); done {
				return result, err
			}
			if result, err, done := processSSELine(ev.line, len(events) == 0); done {
				return result, err
			}

		case <-intervalCh:
			lastRead := time.Unix(0, atomic.LoadInt64(&lastReadAt))
			if time.Since(lastRead) < streamInterval {
				continue
			}
			if clientDisconnected {
				return resultWithUsage(), fmt.Errorf("stream usage incomplete after timeout")
			}
			logger.LegacyPrintf("service.openai_gateway", "Stream data interval timeout: account=%d model=%s interval=%s", account.ID, originalModel, streamInterval)
			// 处理流超时，可能标记账户为临时不可调度或错误状态
			if s.rateLimitService != nil {
				s.rateLimitService.HandleStreamTimeout(ctx, account, originalModel)
			}
			sendErrorEvent("stream_timeout")
			return resultWithUsage(), fmt.Errorf("stream data interval timeout")

		case <-keepaliveCh:
			if clientDisconnected {
				continue
			}
			if time.Since(lastDownstreamWriteAt) < keepaliveInterval {
				continue
			}
			if !streamStarted {
				if err := startStream(); err != nil {
					clientDisconnected = true
					continue
				}
			}
			if _, err := bufferedWriter.WriteString(":\n\n"); err != nil {
				clientDisconnected = true
				logger.LegacyPrintf("service.openai_gateway", "Client disconnected during streaming, continuing to drain upstream for billing")
				continue
			}
			streamStarted = true
			if err := flushBuffered(); err != nil {
				clientDisconnected = true
				logger.LegacyPrintf("service.openai_gateway", "Client disconnected during keepalive flush, continuing to drain upstream for billing")
			} else {
				lastDownstreamWriteAt = time.Now()
			}
		}
	}

}

// extractOpenAISSEDataLine 低开销提取 SSE `data:` 行内容。
// 兼容 `data: xxx` 与 `data:xxx` 两种格式。
func extractOpenAISSEDataLine(line string) (string, bool) {
	if !strings.HasPrefix(line, "data:") {
		return "", false
	}
	start := len("data:")
	for start < len(line) {
		if line[start] != ' ' && line[start] != '	' {
			break
		}
		start++
	}
	return line[start:], true
}

func openAIResponsesEventStartsClientStream(data string) bool {
	eventType := strings.TrimSpace(gjson.Get(data, "type").String())
	switch eventType {
	case "response.created", "response.in_progress":
		return false
	default:
		return true
	}
}

func (s *OpenAIGatewayService) replaceModelInSSELine(line, fromModel, toModel string) string {
	data, ok := extractOpenAISSEDataLine(line)
	if !ok {
		return line
	}
	if data == "" || data == "[DONE]" {
		return line
	}

	// 使用 gjson 精确检查 model 字段，避免全量 JSON 反序列化
	if m := gjson.Get(data, "model"); m.Exists() && m.Str == fromModel {
		newData, err := sjson.Set(data, "model", toModel)
		if err != nil {
			return line
		}
		return "data: " + newData
	}

	// 检查嵌套的 response.model 字段
	if m := gjson.Get(data, "response.model"); m.Exists() && m.Str == fromModel {
		newData, err := sjson.Set(data, "response.model", toModel)
		if err != nil {
			return line
		}
		return "data: " + newData
	}

	return line
}

// correctToolCallsInResponseBody 修正响应体中的工具调用
func (s *OpenAIGatewayService) correctToolCallsInResponseBody(body []byte) []byte {
	if len(body) == 0 {
		return body
	}

	corrected, changed := s.toolCorrector.CorrectToolCallsInSSEBytes(body)
	if changed {
		return corrected
	}
	return body
}

func (s *OpenAIGatewayService) parseSSEUsage(data string, usage *OpenAIUsage) {
	s.parseSSEUsageBytes([]byte(data), usage)
}

func (s *OpenAIGatewayService) parseSSEUsageBytes(data []byte, usage *OpenAIUsage) {
	s.parseSSEUsageBytesWithEventType(data, strings.TrimSpace(gjson.GetBytes(data, "type").String()), usage)
}

func (s *OpenAIGatewayService) parseSSEUsageBytesWithEventType(data []byte, eventType string, usage *OpenAIUsage) {
	if usage == nil || len(data) == 0 || bytes.Equal(data, []byte("[DONE]")) {
		return
	}
	// 选择性解析：仅在数据中包含终止事件标识时才进入字段提取。
	if len(data) < 72 {
		return
	}
	if eventType != "response.completed" && eventType != "response.done" &&
		eventType != "response.incomplete" && eventType != "response.cancelled" && eventType != "response.canceled" {
		return
	}

	usage.InputTokens = int(gjson.GetBytes(data, "response.usage.input_tokens").Int())
	usage.OutputTokens = int(gjson.GetBytes(data, "response.usage.output_tokens").Int())
	usage.CacheReadInputTokens = int(gjson.GetBytes(data, "response.usage.input_tokens_details.cached_tokens").Int())
	usage.ImageOutputTokens = int(gjson.GetBytes(data, "response.usage.output_tokens_details.image_tokens").Int())
}

func extractOpenAIUsageFromJSONBytes(body []byte) (OpenAIUsage, bool) {
	if len(body) == 0 || !gjson.ValidBytes(body) {
		return OpenAIUsage{}, false
	}
	values := gjson.GetManyBytes(
		body,
		"usage.input_tokens",
		"usage.output_tokens",
		"usage.input_tokens_details.cached_tokens",
		"usage.output_tokens_details.image_tokens",
	)
	return OpenAIUsage{
		InputTokens:          int(values[0].Int()),
		OutputTokens:         int(values[1].Int()),
		CacheReadInputTokens: int(values[2].Int()),
		ImageOutputTokens:    int(values[3].Int()),
	}, true
}

func isOpenCodeResponsesClient(c *gin.Context) bool {
	if c == nil || c.Request == nil {
		return false
	}
	userAgent := strings.ToLower(strings.TrimSpace(c.Request.Header.Get("User-Agent")))
	return strings.Contains(userAgent, "opencode")
}

func isOpenCodeFilteredProviderBuiltInOutputType(outputType string) bool {
	switch strings.TrimSpace(outputType) {
	case "web_search_call":
		return true
	default:
		return false
	}
}

func sanitizeOpenCodeResponsesOutput(body []byte) ([]byte, bool, error) {
	if len(body) == 0 || !gjson.ValidBytes(body) {
		return body, false, nil
	}
	output := gjson.GetBytes(body, "output")
	if !output.Exists() || !output.IsArray() {
		return body, false, nil
	}
	var items []json.RawMessage
	if err := json.Unmarshal([]byte(output.Raw), &items); err != nil {
		return body, false, err
	}
	filtered := make([]json.RawMessage, 0, len(items))
	changed := false
	for _, raw := range items {
		if isOpenCodeFilteredProviderBuiltInOutputType(gjson.GetBytes(raw, "type").String()) {
			changed = true
			continue
		}
		filtered = append(filtered, raw)
	}
	if !changed {
		return body, false, nil
	}
	outputJSON, err := json.Marshal(filtered)
	if err != nil {
		return body, false, err
	}
	patched, err := sjson.SetRawBytes(body, "output", outputJSON)
	if err != nil {
		return body, false, err
	}
	return patched, true, nil
}

func normalizeResponsesJSONForAISDK(body []byte) ([]byte, bool, error) {
	if len(body) == 0 || !gjson.ValidBytes(body) {
		return body, false, nil
	}
	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		return body, false, err
	}
	output, ok := payload["output"].([]any)
	if !ok || len(output) == 0 {
		return body, false, nil
	}
	changed := false
	for _, item := range output {
		itemMap, ok := item.(map[string]any)
		if !ok {
			continue
		}
		if strings.TrimSpace(asStringMaybe(itemMap["type"])) != "message" {
			continue
		}
		if strings.TrimSpace(asStringMaybe(itemMap["id"])) == "" {
			itemMap["id"] = generateOpenAIMessageItemID()
			changed = true
		}
		content, ok := itemMap["content"].([]any)
		if !ok {
			continue
		}
		for _, part := range content {
			partMap, ok := part.(map[string]any)
			if !ok {
				continue
			}
			if strings.TrimSpace(asStringMaybe(partMap["type"])) != "output_text" {
				continue
			}
			if _, exists := partMap["annotations"]; !exists {
				partMap["annotations"] = []any{}
				changed = true
			}
		}
	}
	if !changed {
		return body, false, nil
	}
	normalized, err := json.Marshal(payload)
	if err != nil {
		return body, false, err
	}
	return normalized, true, nil
}

func generateOpenAIMessageItemID() string {
	b := make([]byte, 12)
	_, _ = rand.Read(b)
	return "msg_" + hex.EncodeToString(b)
}

func asStringMaybe(v any) string {
	s, _ := v.(string)
	return s
}

func filterOpenCodeResponsesSSEData(data string) (string, bool) {
	eventType := strings.TrimSpace(gjson.Get(data, "type").String())
	switch eventType {
	case "response.web_search_call.in_progress", "response.web_search_call.searching", "response.web_search_call.completed":
		return "", false
	case "response.output_item.added", "response.output_item.done":
		if isOpenCodeFilteredProviderBuiltInOutputType(gjson.Get(data, "item.type").String()) {
			return "", false
		}
	case "response.completed", "response.done":
		responseRaw := gjson.Get(data, "response").Raw
		if responseRaw == "" {
			return data, true
		}
		patched, changed, err := sanitizeOpenCodeResponsesOutput([]byte(responseRaw))
		if err != nil || !changed {
			return data, true
		}
		updated, err := sjson.SetRaw(data, "response", string(patched))
		if err != nil {
			return data, true
		}
		return updated, true
	}
	return data, true
}

func rebuildSSEFrame(lines []string) string {
	trimmed := make([]string, 0, len(lines))
	for _, line := range lines {
		if line == "" {
			continue
		}
		trimmed = append(trimmed, line)
	}
	if len(trimmed) == 0 {
		return ""
	}
	return strings.Join(trimmed, "\n") + "\n\n"
}

func rebuildSSEFrameWithData(lines []string, newData string) string {
	out := make([]string, 0, len(lines)+1)
	wroteData := false
	for _, line := range lines {
		if line == "" {
			continue
		}
		if _, ok := extractOpenAISSEDataLine(line); ok {
			if !wroteData {
				for _, part := range strings.Split(newData, "\n") {
					out = append(out, "data: "+part)
				}
				wroteData = true
			}
			continue
		}
		out = append(out, line)
	}
	if !wroteData {
		for _, part := range strings.Split(newData, "\n") {
			out = append(out, "data: "+part)
		}
	}
	if len(out) == 0 {
		return ""
	}
	return strings.Join(out, "\n") + "\n\n"
}

func splitOpenCodeTerminalContinuationFrames(frameBody string) (string, string) {
	parts := strings.Split(frameBody, "\n\n")
	last := -1
	for i := len(parts) - 1; i >= 0; i-- {
		if strings.TrimSpace(parts[i]) != "" {
			last = i
			break
		}
	}
	if last < 0 {
		return "", ""
	}
	var prefix strings.Builder
	for _, part := range parts[:last] {
		if strings.TrimSpace(part) == "" {
			continue
		}
		prefix.WriteString(part)
		prefix.WriteString("\n\n")
	}
	return prefix.String(), strings.TrimRight(parts[last], "\n") + "\n\n"
}

func filterOpenCodeResponsesSSEFrame(lines []string) (frame string, data string, hasData bool, keep bool) {
	if len(lines) == 0 {
		return "", "", false, false
	}
	dataLines := make([]string, 0, 1)
	for _, line := range lines {
		if extracted, ok := extractOpenAISSEDataLine(line); ok {
			dataLines = append(dataLines, extracted)
		}
	}
	if len(dataLines) == 0 {
		return rebuildSSEFrame(lines), "", false, true
	}
	joined := strings.Join(dataLines, "\n")
	filteredData, keep := filterOpenCodeResponsesSSEData(joined)
	if !keep {
		return "", joined, true, false
	}
	return rebuildSSEFrameWithData(lines, filteredData), filteredData, true, true
}

func filterOpenCodeResponsesSSEBody(bodyText string) string {
	lines := strings.Split(bodyText, "\n")
	frames := make([]string, 0, len(lines)/2)
	current := make([]string, 0, 4)
	flush := func() {
		if len(current) == 0 {
			return
		}
		frame, _, _, keep := filterOpenCodeResponsesSSEFrame(current)
		if keep && frame != "" {
			frames = append(frames, frame)
		}
		current = current[:0]
	}
	for _, line := range lines {
		current = append(current, line)
		if line == "" {
			flush()
		}
	}
	flush()
	return strings.Join(frames, "")
}

type openCodeImageServerContinuationContext struct {
	RequestBody    []byte
	Token          string
	PromptCacheKey string
	IsCodexCLI     bool
}

func (s *OpenAIGatewayService) handleNonStreamingResponse(ctx context.Context, resp *http.Response, c *gin.Context, account *Account, originalModel, mappedModel string) (*OpenAIUsage, error) {
	return s.handleNonStreamingResponseWithOpenCodeContinuation(ctx, resp, c, account, originalModel, mappedModel, nil)
}

func (s *OpenAIGatewayService) handleNonStreamingResponseWithOpenCodeContinuation(ctx context.Context, resp *http.Response, c *gin.Context, account *Account, originalModel, mappedModel string, continuation *openCodeImageServerContinuationContext) (*OpenAIUsage, error) {
	body, err := ReadUpstreamResponseBody(resp.Body, s.cfg, c, openAITooLargeError)
	if err != nil {
		return nil, err
	}

	// Detect SSE responses for ALL account types via Content-Type header.
	// Some OpenAI-compatible upstreams (including other sub2api instances)
	// may return SSE even when stream=false was requested.
	if isEventStreamResponse(resp.Header) {
		return s.handleSSEToJSONForAccountWithOpenCodeContinuation(resp, c, body, account, originalModel, mappedModel, continuation)
	}
	// For OAuth accounts, also fall back to a body-content heuristic because
	// the upstream may omit the Content-Type header while still sending SSE.
	// This heuristic is NOT applied to API-key accounts to avoid false
	// positives on JSON responses that coincidentally contain "data:" or
	// "event:" in their text content.
	if account.Type == AccountTypeOAuth {
		bodyLooksLikeSSE := bytes.Contains(body, []byte("data:")) || bytes.Contains(body, []byte("event:"))
		if bodyLooksLikeSSE {
			return s.handleSSEToJSONForAccountWithOpenCodeContinuation(resp, c, body, account, originalModel, mappedModel, continuation)
		}
	}

	usageValue, usageOK := extractOpenAIUsageFromJSONBytes(body)
	if !usageOK {
		return nil, fmt.Errorf("parse response: invalid json response")
	}
	usage := &usageValue
	var generated []openCodeImageGeneratedMessage
	if isOpenCodeResponsesClient(c) {
		filteredBody, rewrittenGenerated, changed, err := rewriteOpenCodeImageGenerationOutputWithGenerated(ctx, body, s.generatedImageStore, openCodeImageRewriteOptions{
			BaseURL: s.resolveOpenCodeImageDownloadBaseURL(ctx, c),
		})
		if err != nil {
			return nil, fmt.Errorf("rewrite opencode image generation output: %w", err)
		}
		if changed {
			body = filteredBody
			generated = rewrittenGenerated
		}
		if len(generated) > 0 && continuation != nil {
			continuedBody, continuedUsage, continued, err := s.openCodeImageServerContinuationResponse(ctx, c, account, originalModel, mappedModel, continuation, generated)
			if err != nil {
				return nil, err
			}
			if continued {
				mergedBody, err := mergeOpenCodeImageContinuationResponseBodies(body, continuedBody)
				if err != nil {
					return nil, err
				}
				body = mergedBody
				addOpenAIUsage(usage, continuedUsage)
			}
		}
	}
	if normalizedBody, changed, err := normalizeResponsesJSONForAISDK(body); err != nil {
		return nil, fmt.Errorf("normalize responses json for ai sdk: %w", err)
	} else if changed {
		body = normalizedBody
	}

	// Replace model in response if needed
	if originalModel != mappedModel {
		body = s.replaceModelInResponseBody(body, mappedModel, originalModel)
	}

	responseheaders.WriteFilteredHeaders(c.Writer.Header(), resp.Header, s.responseHeaderFilter)

	contentType := "application/json"
	if s.cfg != nil && !s.cfg.Security.ResponseHeaders.Enabled {
		if upstreamType := resp.Header.Get("Content-Type"); upstreamType != "" {
			contentType = upstreamType
		}
	}

	c.Data(resp.StatusCode, contentType, body)

	return usage, nil
}

func (s *OpenAIGatewayService) openCodeImageServerContinuationResponse(ctx context.Context, c *gin.Context, account *Account, originalModel, mappedModel string, continuation *openCodeImageServerContinuationContext, generated []openCodeImageGeneratedMessage) ([]byte, *OpenAIUsage, bool, error) {
	if continuation == nil || len(generated) == 0 {
		return nil, nil, false, nil
	}
	continuedRequestBody, changed, err := buildOpenCodeImageServerContinuationBody(continuation.RequestBody, generated)
	if err != nil || !changed {
		return nil, nil, false, err
	}
	resp, err := s.doOpenAIResponsesUpstream(ctx, c, account, continuedRequestBody, continuation.Token, false, continuation.PromptCacheKey, continuation.IsCodexCLI)
	if err != nil {
		return nil, nil, false, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode >= 400 {
		return nil, nil, false, fmt.Errorf("opencode image continuation upstream returned status %d", resp.StatusCode)
	}
	body, err := ReadUpstreamResponseBody(resp.Body, s.cfg, c, openAITooLargeError)
	if err != nil {
		return nil, nil, false, err
	}
	if isEventStreamResponse(resp.Header) || (account != nil && account.Type == AccountTypeOAuth && (bytes.Contains(body, []byte("data:")) || bytes.Contains(body, []byte("event:")))) {
		finalResponse, ok := extractCodexFinalResponse(string(body))
		if !ok {
			return nil, nil, false, fmt.Errorf("opencode image continuation: missing terminal response")
		}
		body = finalResponse
	}
	usageValue, usageOK := extractOpenAIUsageFromJSONBytes(body)
	if !usageOK {
		return nil, nil, false, fmt.Errorf("opencode image continuation: invalid json response")
	}
	usage := &usageValue
	filteredBody, changed, err := rewriteOpenCodeImageGenerationOutput(ctx, body, s.generatedImageStore, openCodeImageRewriteOptions{
		BaseURL: s.resolveOpenCodeImageDownloadBaseURL(ctx, c),
	})
	if err != nil {
		return nil, nil, false, fmt.Errorf("rewrite opencode image continuation output: %w", err)
	}
	if changed {
		body = filteredBody
	}
	if normalizedBody, changed, err := normalizeResponsesJSONForAISDK(body); err != nil {
		return nil, nil, false, fmt.Errorf("normalize opencode image continuation response: %w", err)
	} else if changed {
		body = normalizedBody
	}
	if originalModel != mappedModel {
		body = s.replaceModelInResponseBody(body, mappedModel, originalModel)
	}
	return body, usage, true, nil
}

func (s *OpenAIGatewayService) openCodeImageStreamingServerContinuation(ctx context.Context, c *gin.Context, account *Account, startTime time.Time, originalModel, mappedModel string, continuation *openCodeImageServerContinuationContext, generated []openCodeImageGeneratedMessage) (*openaiStreamingResult, error) {
	if continuation == nil || len(generated) == 0 {
		return nil, nil
	}
	continuedRequestBody, changed, err := buildOpenCodeImageServerContinuationBody(continuation.RequestBody, generated)
	if err != nil || !changed {
		return nil, err
	}
	resp, err := s.doOpenAIResponsesUpstream(ctx, c, account, continuedRequestBody, continuation.Token, true, continuation.PromptCacheKey, continuation.IsCodexCLI)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("opencode image streaming continuation upstream returned status %d", resp.StatusCode)
	}
	return s.handleStreamingResponseWithOpenCodeContinuation(ctx, resp, c, account, startTime, originalModel, mappedModel, nil)
}

func (s *OpenAIGatewayService) doOpenAIResponsesUpstream(ctx context.Context, c *gin.Context, account *Account, body []byte, token string, isStream bool, promptCacheKey string, isCodexCLI bool) (*http.Response, error) {
	upstreamReq, err := s.buildUpstreamRequest(ctx, c, account, body, token, isStream, promptCacheKey, isCodexCLI)
	if err != nil {
		return nil, err
	}
	proxyURL := ""
	if account != nil && account.ProxyID != nil && account.Proxy != nil {
		proxyURL = account.Proxy.URL()
	}
	upstreamStart := time.Now()
	resp, err := s.httpUpstream.Do(upstreamReq, proxyURL, account.ID, account.Concurrency)
	SetOpsLatencyMs(c, OpsUpstreamLatencyMsKey, time.Since(upstreamStart).Milliseconds())
	return resp, err
}

func mergeOpenCodeImageContinuationResponseBodies(firstBody, continuationBody []byte) ([]byte, error) {
	firstOutput := gjson.GetBytes(firstBody, "output")
	continuationOutput := gjson.GetBytes(continuationBody, "output")
	if !firstOutput.IsArray() || !continuationOutput.IsArray() {
		return continuationBody, nil
	}
	combined := make([]json.RawMessage, 0, len(firstOutput.Array())+len(continuationOutput.Array()))
	if err := json.Unmarshal([]byte(firstOutput.Raw), &combined); err != nil {
		return nil, err
	}
	var tail []json.RawMessage
	if err := json.Unmarshal([]byte(continuationOutput.Raw), &tail); err != nil {
		return nil, err
	}
	combined = append(combined, tail...)
	combinedJSON, err := json.Marshal(combined)
	if err != nil {
		return nil, err
	}
	merged, err := sjson.SetRawBytes(continuationBody, "output", combinedJSON)
	if err != nil {
		return nil, err
	}
	return mergeOpenCodeImageContinuationResponseUsage(firstBody, merged)
}

func mergeOpenCodeImageContinuationResponseUsage(firstBody, mergedBody []byte) ([]byte, error) {
	firstUsage := gjson.GetBytes(firstBody, "usage")
	mergedUsage := gjson.GetBytes(mergedBody, "usage")
	if !firstUsage.Exists() || !mergedUsage.Exists() {
		return mergedBody, nil
	}
	patched := mergedBody
	var err error
	for _, path := range []string{"input_tokens", "output_tokens", "total_tokens"} {
		if path == "total_tokens" && !firstUsage.Get(path).Exists() && !mergedUsage.Get(path).Exists() {
			continue
		}
		patched, err = sjson.SetBytes(patched, "usage."+path, firstUsage.Get(path).Int()+mergedUsage.Get(path).Int())
		if err != nil {
			return nil, err
		}
	}
	for _, path := range []string{"input_tokens_details.cached_tokens", "output_tokens_details.image_tokens"} {
		if !firstUsage.Get(path).Exists() && !mergedUsage.Get(path).Exists() {
			continue
		}
		patched, err = sjson.SetBytes(patched, "usage."+path, firstUsage.Get(path).Int()+mergedUsage.Get(path).Int())
		if err != nil {
			return nil, err
		}
	}
	return patched, nil
}

func addOpenAIUsage(base *OpenAIUsage, extra *OpenAIUsage) {
	if base == nil || extra == nil {
		return
	}
	base.InputTokens += extra.InputTokens
	base.OutputTokens += extra.OutputTokens
	base.CacheReadInputTokens += extra.CacheReadInputTokens
	base.CacheCreationInputTokens += extra.CacheCreationInputTokens
	base.ImageOutputTokens += extra.ImageOutputTokens
}

func isEventStreamResponse(header http.Header) bool {
	contentType := strings.ToLower(header.Get("Content-Type"))
	return strings.Contains(contentType, "text/event-stream")
}

func (s *OpenAIGatewayService) handleSSEToJSON(resp *http.Response, c *gin.Context, body []byte, originalModel, mappedModel string) (*OpenAIUsage, error) {
	return s.handleSSEToJSONForAccount(resp, c, body, nil, originalModel, mappedModel)
}

func (s *OpenAIGatewayService) handleSSEToJSONForAccount(resp *http.Response, c *gin.Context, body []byte, account *Account, originalModel, mappedModel string) (*OpenAIUsage, error) {
	return s.handleSSEToJSONForAccountWithOpenCodeContinuation(resp, c, body, account, originalModel, mappedModel, nil)
}

func (s *OpenAIGatewayService) handleSSEToJSONForAccountWithOpenCodeContinuation(resp *http.Response, c *gin.Context, body []byte, account *Account, originalModel, mappedModel string, continuation *openCodeImageServerContinuationContext) (*OpenAIUsage, error) {
	bodyText := string(body)
	finalResponse, ok := extractCodexFinalResponse(bodyText)
	isOpenCodeClient := isOpenCodeResponsesClient(c)
	applySupplement := shouldSupplementOAuthNonCompactResponses(c, account)
	applyToolUsageReconcile := applySupplement && !isOpenCodeClient

	usage := &OpenAIUsage{}
	if ok {
		if parsedUsage, parsed := extractOpenAIUsageFromJSONBytes(finalResponse); parsed {
			*usage = parsedUsage
		}
		if applySupplement {
			if mergedResponse, changed, err := mergeCompletedResponsesOutputFromSSE(finalResponse, bodyText, applyToolUsageReconcile); err != nil {
				return nil, fmt.Errorf("merge completed responses output from sse: %w", err)
			} else if changed {
				finalResponse = mergedResponse
			}
		} else if len(gjson.GetBytes(finalResponse, "output").Array()) == 0 {
			// When the terminal event has an empty output array, reconstruct
			// output from accumulated delta events so the client gets full content.
			// gjson Array() returns empty slice for null, missing, or empty arrays.
			var outputJSON []byte
			var reconstructed bool
			if isOpenCodeClient {
				outputJSON, reconstructed = reconstructOpenCodeResponseOutputFromSSE(bodyText)
			} else {
				outputJSON, reconstructed = reconstructResponseOutputFromSSE(bodyText)
			}
			if reconstructed {
				if patched, err := sjson.SetRawBytes(finalResponse, "output", outputJSON); err == nil {
					finalResponse = patched
				}
			}
		}
		body = finalResponse
		if isOpenCodeClient {
			filteredBody, generated, changed, err := rewriteOpenCodeImageGenerationOutputWithGenerated(c.Request.Context(), body, s.generatedImageStore, openCodeImageRewriteOptions{
				BaseURL: s.resolveOpenCodeImageDownloadBaseURL(c.Request.Context(), c),
			})
			if err != nil {
				return nil, fmt.Errorf("rewrite opencode image generation sse output: %w", err)
			}
			if changed {
				body = filteredBody
			}
			if len(generated) > 0 && continuation != nil {
				continuedBody, continuedUsage, continued, err := s.openCodeImageServerContinuationResponse(c.Request.Context(), c, account, originalModel, mappedModel, continuation, generated)
				if err != nil {
					return nil, err
				}
				if continued {
					mergedBody, err := mergeOpenCodeImageContinuationResponseBodies(body, continuedBody)
					if err != nil {
						return nil, err
					}
					body = mergedBody
					addOpenAIUsage(usage, continuedUsage)
				}
			}
		}
		if normalizedBody, changed, err := normalizeResponsesJSONForAISDK(body); err != nil {
			return nil, fmt.Errorf("normalize completed responses json for ai sdk: %w", err)
		} else if changed {
			body = normalizedBody
		}
		if originalModel != mappedModel {
			body = s.replaceModelInResponseBody(body, mappedModel, originalModel)
		}
		// Correct tool calls in final response
		body = s.correctToolCallsInResponseBody(body)
	} else {
		terminalType, terminalPayload, terminalOK := extractOpenAISSETerminalEvent(bodyText)
		if terminalOK && terminalType == "response.failed" {
			msg := extractOpenAISSEErrorMessage(terminalPayload)
			if msg == "" {
				msg = "Upstream compact response failed"
			}
			return nil, s.writeOpenAINonStreamingProtocolError(resp, c, msg)
		}
		if isOpenCodeClient && containsOpenCodeImageGenerationSSE(bodyText) {
			return nil, s.writeOpenAINonStreamingProtocolError(resp, c, "Upstream returned image generation events without a terminal response")
		}
		usage = s.parseSSEUsageFromBody(bodyText)
		if originalModel != mappedModel {
			bodyText = s.replaceModelInSSEBody(bodyText, mappedModel, originalModel)
		}
		if isOpenCodeClient {
			bodyText = filterOpenCodeResponsesSSEBody(bodyText)
		}
		body = []byte(bodyText)
	}

	responseheaders.WriteFilteredHeaders(c.Writer.Header(), resp.Header, s.responseHeaderFilter)

	contentType := "application/json; charset=utf-8"
	if !ok {
		contentType = resp.Header.Get("Content-Type")
		if contentType == "" {
			contentType = "text/event-stream"
		}
	}
	c.Data(resp.StatusCode, contentType, body)

	return usage, nil
}

func (s *OpenAIGatewayService) handleOAuthSSEToJSON(resp *http.Response, c *gin.Context, body []byte, originalModel, mappedModel string) (*OpenAIUsage, error) {
	return s.handleSSEToJSONForAccount(resp, c, body, &Account{Type: AccountTypeOAuth}, originalModel, mappedModel)
}

func extractOpenAISSETerminalEvent(body string) (string, []byte, bool) {
	lines := strings.Split(body, "\n")
	currentEventType := ""
	for _, line := range lines {
		if line == "" {
			currentEventType = ""
			continue
		}
		if eventType, ok := extractSSEEventLine(line); ok {
			currentEventType = eventType
			continue
		}
		data, ok := extractOpenAISSEDataLine(line)
		if !ok || data == "" || data == "[DONE]" {
			continue
		}
		eventType := strings.TrimSpace(gjson.Get(data, "type").String())
		if eventType == "" {
			eventType = currentEventType
		}
		switch eventType {
		case "response.completed", "response.done", "response.failed", "response.incomplete", "response.cancelled", "response.canceled":
			return eventType, []byte(data), true
		}
	}
	return "", nil, false
}

func extractOpenAISSEErrorMessage(payload []byte) string {
	if len(payload) == 0 {
		return ""
	}
	for _, path := range []string{"response.error.message", "error.message", "message"} {
		if msg := strings.TrimSpace(gjson.GetBytes(payload, path).String()); msg != "" {
			return sanitizeUpstreamErrorMessage(msg)
		}
	}
	return sanitizeUpstreamErrorMessage(strings.TrimSpace(extractUpstreamErrorMessage(payload)))
}

func (s *OpenAIGatewayService) writeOpenAINonStreamingProtocolError(resp *http.Response, c *gin.Context, message string) error {
	message = sanitizeUpstreamErrorMessage(strings.TrimSpace(message))
	if message == "" {
		message = "Upstream returned an invalid non-streaming response"
	}
	setOpsUpstreamError(c, http.StatusBadGateway, message, "")
	responseheaders.WriteFilteredHeaders(c.Writer.Header(), resp.Header, s.responseHeaderFilter)
	c.Writer.Header().Set("Content-Type", "application/json; charset=utf-8")
	c.JSON(http.StatusBadGateway, gin.H{
		"error": gin.H{
			"type":    "upstream_error",
			"message": message,
		},
	})
	return fmt.Errorf("non-streaming openai protocol error: %s", message)
}

func extractCodexFinalResponse(body string) ([]byte, bool) {
	lines := strings.Split(body, "\n")
	currentEventType := ""
	for _, line := range lines {
		if line == "" {
			currentEventType = ""
			continue
		}
		if eventType, ok := extractSSEEventLine(line); ok {
			currentEventType = eventType
			continue
		}
		data, ok := extractOpenAISSEDataLine(line)
		if !ok {
			continue
		}
		if data == "" || data == "[DONE]" {
			continue
		}
		eventType := gjson.Get(data, "type").String()
		if eventType == "" {
			eventType = currentEventType
		}
		if isOpenAITerminalResponseEventType(eventType) && eventType != "response.failed" {
			if response := gjson.Get(data, "response"); response.Exists() && response.Type == gjson.JSON && response.Raw != "" {
				return []byte(response.Raw), true
			}
		}
	}
	return nil, false
}

// reconstructResponseOutputFromSSE scans raw SSE body text for delta events and
// returns a JSON-encoded output array reconstructed from accumulated deltas.
// Returns (nil, false) if no content was found in deltas.
func reconstructResponseOutputFromSSE(bodyText string) ([]byte, bool) {
	return reconstructResponseOutputFromSSEWithOptions(bodyText, false)
}

func reconstructOpenCodeResponseOutputFromSSE(bodyText string) ([]byte, bool) {
	return reconstructResponseOutputFromSSEWithOptions(bodyText, true)
}

func reconstructResponseOutputFromSSEWithOptions(bodyText string, includeImageWithoutResult bool) ([]byte, bool) {
	if !includeImageWithoutResult {
		return reconstructResponseOutputFromSSELegacy(bodyText)
	}

	acc := apicompat.NewBufferedResponseAccumulator()
	currentEventType := ""
	for _, line := range strings.Split(bodyText, "\n") {
		if line == "" {
			currentEventType = ""
			continue
		}
		if eventType, ok := extractSSEEventLine(line); ok {
			currentEventType = eventType
			continue
		}
		data, ok := extractOpenAISSEDataLine(line)
		if !ok || data == "" || data == "[DONE]" {
			continue
		}
		var event apicompat.ResponsesStreamEvent
		if err := json.Unmarshal([]byte(data), &event); err != nil {
			continue
		}
		if event.Type == "" {
			event.Type = currentEventType
			if event.Type == "" {
				event.Type = inferImageGenerationStreamEventType(&event)
			}
		}
		acc.ProcessEvent(&event)
	}

	indexed := acc.BuildIndexedOutput()
	if len(indexed) == 0 {
		return nil, false
	}
	output := make([]json.RawMessage, 0, len(indexed))
	for _, indexedItem := range indexed {
		raw, err := json.Marshal(indexedItem.Item)
		if err != nil {
			return nil, false
		}
		output = append(output, raw)
	}
	if len(output) == 0 {
		return nil, false
	}

	outputJSON, err := json.Marshal(output)
	if err != nil {
		return nil, false
	}
	return outputJSON, true
}

func reconstructResponseOutputFromSSELegacy(bodyText string) ([]byte, bool) {
	acc := apicompat.NewBufferedResponseAccumulator()
	imageOutputs := make([]json.RawMessage, 0, 1)
	seenImages := make(map[string]struct{})
	lines := strings.Split(bodyText, "\n")
	for _, line := range lines {
		data, ok := extractOpenAISSEDataLine(line)
		if !ok || data == "" || data == "[DONE]" {
			continue
		}
		if imageOutput, ok := extractImageGenerationOutputFromSSEDataWithOptions([]byte(data), seenImages, false); ok {
			imageOutputs = append(imageOutputs, imageOutput)
		}
		var event apicompat.ResponsesStreamEvent
		if err := json.Unmarshal([]byte(data), &event); err != nil {
			continue
		}
		acc.ProcessEvent(&event)
	}
	if !acc.HasContent() && len(imageOutputs) == 0 {
		return nil, false
	}

	var output []json.RawMessage
	if acc.HasContent() {
		outputJSON, err := json.Marshal(acc.BuildOutput())
		if err == nil {
			_ = json.Unmarshal(outputJSON, &output)
		}
	}
	output = append(output, imageOutputs...)
	if len(output) == 0 {
		return nil, false
	}

	outputJSON, err := json.Marshal(output)
	if err != nil {
		return nil, false
	}
	return outputJSON, true
}

func containsOpenCodeImageGenerationSSE(bodyText string) bool {
	currentEventType := ""
	for _, line := range strings.Split(bodyText, "\n") {
		if line == "" {
			currentEventType = ""
			continue
		}
		if eventType, ok := extractSSEEventLine(line); ok {
			currentEventType = eventType
			if strings.HasPrefix(currentEventType, "response.image_generation_call.") {
				return true
			}
			continue
		}
		data, ok := extractOpenAISSEDataLine(line)
		if !ok || data == "" || data == "[DONE]" {
			continue
		}
		if !gjson.Valid(data) {
			if containsMalformedOpenCodeImageSSEMarker(data) {
				return true
			}
			continue
		}
		if gjson.Get(data, "partial_image_b64").Exists() {
			return true
		}
		if gjson.Get(data, "item.type").String() == "image_generation_call" {
			return true
		}
		for _, item := range gjson.Get(data, "response.output").Array() {
			if item.Get("type").String() == "image_generation_call" {
				return true
			}
		}
		eventType := strings.TrimSpace(gjson.Get(data, "type").String())
		if eventType == "" {
			eventType = currentEventType
		}
		if strings.HasPrefix(eventType, "response.image_generation_call.") {
			return true
		}
		if (eventType == "response.output_item.added" || eventType == "response.output_item.done") && gjson.Get(data, "item.type").String() == "image_generation_call" {
			return true
		}
	}
	return false
}

func shouldSupplementOAuthNonCompactResponses(c *gin.Context, account *Account) bool {
	return account != nil && account.Type == AccountTypeOAuth && !isOpenAIResponsesCompactPath(c)
}

func shouldRequestJSONForOAuthNonStreamIncludeSources(c *gin.Context, account *Account, body []byte, isStream bool) bool {
	if account == nil || account.Type != AccountTypeOAuth || isStream || isOpenAIResponsesCompactPath(c) {
		return false
	}
	reqBody, err := getOpenAIRequestBodyMap(c, body)
	if err != nil {
		return false
	}
	includeValue, ok := reqBody["include"]
	if !ok {
		return false
	}
	switch includes := includeValue.(type) {
	case []any:
		for _, raw := range includes {
			if include, ok := raw.(string); ok && include == "web_search_call.action.sources" {
				return true
			}
		}
	case []string:
		for _, include := range includes {
			if include == "web_search_call.action.sources" {
				return true
			}
		}
	}
	return false
}

type canonicalResponsesOutputSlot struct {
	OutputIndex int
	Item        map[string]any
}

func mergeCompletedResponsesOutputFromSSE(finalResponse []byte, bodyText string, reconcileToolUsage bool) ([]byte, bool, error) {
	if len(finalResponse) == 0 || !gjson.ValidBytes(finalResponse) {
		return finalResponse, false, nil
	}

	canonical, ok, err := buildCanonicalOutputMapsFromSSE(bodyText)
	if err != nil {
		return nil, false, err
	}
	if !ok {
		return finalResponse, false, nil
	}

	var responseMap map[string]any
	if err := json.Unmarshal(finalResponse, &responseMap); err != nil {
		return nil, false, err
	}

	changed, err := mergeTerminalOutputWithCanonical(responseMap, canonical)
	if err != nil {
		return nil, false, err
	}
	if reconcileToolUsage && reconcileWebSearchToolUsage(responseMap) {
		changed = true
	}
	if !changed {
		return finalResponse, false, nil
	}

	mergedResponse, err := json.Marshal(responseMap)
	if err != nil {
		return nil, false, err
	}
	return mergedResponse, true, nil
}

func buildCanonicalOutputMapsFromSSE(bodyText string) ([]canonicalResponsesOutputSlot, bool, error) {
	acc := apicompat.NewBufferedResponseAccumulator()
	currentEventType := ""
	for _, line := range strings.Split(bodyText, "\n") {
		if line == "" {
			currentEventType = ""
			continue
		}
		if eventType, ok := extractSSEEventLine(line); ok {
			currentEventType = eventType
			continue
		}
		data, ok := extractOpenAISSEDataLine(line)
		if !ok || data == "" || data == "[DONE]" {
			continue
		}

		var event apicompat.ResponsesStreamEvent
		if err := json.Unmarshal([]byte(data), &event); err != nil {
			continue
		}
		if event.Type == "" {
			event.Type = currentEventType
			if event.Type == "" {
				event.Type = inferImageGenerationStreamEventType(&event)
			}
		}
		acc.ProcessEvent(&event)
	}

	indexed := acc.BuildIndexedOutput()
	if len(indexed) == 0 {
		return nil, false, nil
	}

	canonical := make([]canonicalResponsesOutputSlot, 0, len(indexed))
	for _, indexedItem := range indexed {
		itemMap, err := responsesOutputToMap(indexedItem.Item)
		if err != nil {
			return nil, false, err
		}
		canonical = append(canonical, canonicalResponsesOutputSlot{
			OutputIndex: indexedItem.OutputIndex,
			Item:        itemMap,
		})
	}
	return canonical, true, nil
}

func inferImageGenerationStreamEventType(event *apicompat.ResponsesStreamEvent) string {
	if event == nil || event.Item == nil || event.Item.Type != "image_generation_call" {
		return ""
	}
	if strings.TrimSpace(event.Item.Result) != "" || strings.TrimSpace(event.Item.Status) == "completed" {
		return "response.output_item.done"
	}
	return ""
}

func mergeTerminalOutputWithCanonical(finalResponse map[string]any, canonical []canonicalResponsesOutputSlot) (bool, error) {
	if len(canonical) == 0 {
		return false, nil
	}

	oldOutputJSON, err := json.Marshal(finalResponse["output"])
	if err != nil {
		return false, err
	}

	terminalOutput, err := responseOutputMapsFromValue(finalResponse["output"])
	if err != nil {
		return false, err
	}

	remainingCanonicalNoKeyByType := make(map[string]int)
	for _, slot := range canonical {
		if responsesOutputStableKey(slot.Item) != "" {
			continue
		}
		itemType := responsesOutputMapType(slot.Item)
		if itemType == "" {
			continue
		}
		remainingCanonicalNoKeyByType[itemType]++
	}

	matchedTerminal := make([]bool, len(terminalOutput))
	seenKeys := make(map[string]struct{})
	unresolvedCanonicalNoKeyByType := make(map[string]int)
	mergedOutput := make([]map[string]any, 0, len(canonical)+len(terminalOutput))
	for _, canonicalSlot := range canonical {
		canonicalItem := canonicalSlot.Item
		matchIndex := findMatchingTerminalOutputItem(terminalOutput, matchedTerminal, canonicalSlot, remainingCanonicalNoKeyByType)
		mergedItem := cloneJSONObject(canonicalItem)
		if matchIndex >= 0 {
			matchedTerminal[matchIndex] = true
			mergedItem = mergeCanonicalAndTerminalOutputItem(canonicalItem, terminalOutput[matchIndex])
		}
		if responsesOutputStableKey(canonicalItem) == "" {
			itemType := responsesOutputMapType(canonicalItem)
			if itemType != "" {
				remainingCanonicalNoKeyByType[itemType]--
				if matchIndex < 0 {
					unresolvedCanonicalNoKeyByType[itemType]++
				}
			}
		}
		if stableKey := responsesOutputStableKey(mergedItem); stableKey != "" {
			if _, exists := seenKeys[stableKey]; exists {
				continue
			}
			seenKeys[stableKey] = struct{}{}
		}
		mergedOutput = append(mergedOutput, mergedItem)
	}

	for i, terminalItem := range terminalOutput {
		if matchedTerminal[i] {
			continue
		}
		itemType := responsesOutputMapType(terminalItem)
		stableKey := responsesOutputStableKey(terminalItem)
		if stableKey == "" && itemType != "" && unresolvedCanonicalNoKeyByType[itemType] > 0 {
			continue
		}
		if stableKey != "" {
			if _, exists := seenKeys[stableKey]; exists {
				continue
			}
			seenKeys[stableKey] = struct{}{}
		}
		mergedOutput = append(mergedOutput, cloneJSONObject(terminalItem))
	}

	newOutputJSON, err := json.Marshal(mergedOutput)
	if err != nil {
		return false, err
	}
	if bytes.Equal(oldOutputJSON, newOutputJSON) {
		return false, nil
	}

	finalResponse["output"] = mergedOutput
	return true, nil
}

func reconcileWebSearchToolUsage(finalResponse map[string]any) bool {
	toolUsage, ok := finalResponse["tool_usage"].(map[string]any)
	if !ok {
		return false
	}
	webSearchUsage, ok := toolUsage["web_search"].(map[string]any)
	if !ok {
		return false
	}

	outputItems, err := responseOutputMapsFromValue(finalResponse["output"])
	if err != nil {
		return false
	}
	webSearchCalls := 0
	for _, item := range outputItems {
		if responsesOutputMapType(item) == "web_search_call" {
			webSearchCalls++
		}
	}
	if webSearchCalls == 0 {
		return false
	}

	current, ok := jsonInt64Value(webSearchUsage["num_requests"])
	if !ok {
		current = 0
	}
	if current >= int64(webSearchCalls) {
		return false
	}
	webSearchUsage["num_requests"] = webSearchCalls
	return true
}

func responsesOutputToMap(item apicompat.ResponsesOutput) (map[string]any, error) {
	raw, err := json.Marshal(item)
	if err != nil {
		return nil, err
	}
	var itemMap map[string]any
	if err := json.Unmarshal(raw, &itemMap); err != nil {
		return nil, err
	}
	return itemMap, nil
}

func responseOutputMapsFromValue(value any) ([]map[string]any, error) {
	if value == nil {
		return nil, nil
	}

	raw, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	if bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		return nil, nil
	}

	var items []any
	if err := json.Unmarshal(raw, &items); err != nil {
		return nil, err
	}

	output := make([]map[string]any, 0, len(items))
	for _, item := range items {
		itemMap, ok := item.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("response output item has type %T", item)
		}
		output = append(output, itemMap)
	}
	return output, nil
}

func findMatchingTerminalOutputItem(terminalOutput []map[string]any, matchedTerminal []bool, canonicalSlot canonicalResponsesOutputSlot, remainingCanonicalNoKeyByType map[string]int) int {
	canonicalItem := canonicalSlot.Item
	canonicalKey := responsesOutputStableKey(canonicalItem)
	if canonicalKey != "" {
		for i, terminalItem := range terminalOutput {
			if matchedTerminal[i] || responsesOutputStableKey(terminalItem) != canonicalKey {
				continue
			}
			return i
		}
		return -1
	}

	itemType := responsesOutputMapType(canonicalItem)
	if itemType == "" || remainingCanonicalNoKeyByType[itemType] != 1 {
		return -1
	}

	var candidateIndexes []int
	for i, terminalItem := range terminalOutput {
		if matchedTerminal[i] || responsesOutputMapType(terminalItem) != itemType {
			continue
		}
		candidateIndexes = append(candidateIndexes, i)
	}
	if len(candidateIndexes) == 1 {
		return candidateIndexes[0]
	}
	return -1
}

func mergeCanonicalAndTerminalOutputItem(canonicalItem, terminalItem map[string]any) map[string]any {
	switch responsesOutputMapType(canonicalItem) {
	case "message":
		return mergeJSONObject(canonicalItem, terminalItem, true)
	case "reasoning", "function_call", "web_search_call":
		return mergeJSONObject(canonicalItem, terminalItem, false)
	default:
		return mergeJSONObject(canonicalItem, terminalItem, false)
	}
}

func mergeJSONObject(base, overlay map[string]any, preferOverlay bool) map[string]any {
	result := cloneJSONObject(base)
	for key, overlayValue := range overlay {
		existingValue, exists := result[key]
		if !exists {
			result[key] = cloneJSONValue(overlayValue)
			continue
		}

		overlayMap, overlayIsMap := overlayValue.(map[string]any)
		existingMap, existingIsMap := existingValue.(map[string]any)
		if overlayIsMap {
			if existingIsMap {
				result[key] = mergeJSONObject(existingMap, overlayMap, preferOverlay)
			} else if preferOverlay || isEmptyJSONValue(existingValue) {
				result[key] = cloneJSONObject(overlayMap)
			}
			continue
		}

		if _, ok := overlayValue.([]any); ok {
			if preferOverlay || isEmptyJSONValue(existingValue) {
				result[key] = cloneJSONValue(overlayValue)
			}
			continue
		}

		if preferOverlay {
			if !isEmptyJSONValue(overlayValue) || isEmptyJSONValue(existingValue) {
				result[key] = cloneJSONValue(overlayValue)
			}
			continue
		}
		if isEmptyJSONValue(existingValue) && !isEmptyJSONValue(overlayValue) {
			result[key] = cloneJSONValue(overlayValue)
		}
	}
	return result
}

func responsesOutputStableKey(item map[string]any) string {
	itemType := responsesOutputMapType(item)
	if itemType == "" {
		return ""
	}

	switch itemType {
	case "function_call":
		callID := strings.TrimSpace(asStringMaybe(item["call_id"]))
		if callID == "" {
			return ""
		}
		return itemType + ":" + callID
	case "message", "reasoning", "web_search_call", "image_generation_call":
		id := strings.TrimSpace(asStringMaybe(item["id"]))
		if id == "" {
			return ""
		}
		return itemType + ":" + id
	default:
		return ""
	}
}

func responsesOutputMapType(item map[string]any) string {
	return strings.TrimSpace(asStringMaybe(item["type"]))
}

func isEmptyJSONValue(value any) bool {
	switch typed := value.(type) {
	case nil:
		return true
	case string:
		return strings.TrimSpace(typed) == ""
	case []any:
		return len(typed) == 0
	case map[string]any:
		return len(typed) == 0
	default:
		return false
	}
}

func cloneJSONObject(input map[string]any) map[string]any {
	if input == nil {
		return nil
	}
	cloned, _ := cloneJSONValue(input).(map[string]any)
	if cloned == nil {
		return map[string]any{}
	}
	return cloned
}

func cloneJSONValue(value any) any {
	raw, err := json.Marshal(value)
	if err != nil {
		return value
	}
	var cloned any
	if err := json.Unmarshal(raw, &cloned); err != nil {
		return value
	}
	return cloned
}

func jsonInt64Value(value any) (int64, bool) {
	switch typed := value.(type) {
	case nil:
		return 0, false
	case int:
		return int64(typed), true
	case int8:
		return int64(typed), true
	case int16:
		return int64(typed), true
	case int32:
		return int64(typed), true
	case int64:
		return typed, true
	case float32:
		return int64(typed), true
	case float64:
		return int64(typed), true
	case json.Number:
		parsed, err := typed.Int64()
		return parsed, err == nil
	case string:
		parsed, err := strconv.ParseInt(strings.TrimSpace(typed), 10, 64)
		return parsed, err == nil
	default:
		return 0, false
	}
}

func extractImageGenerationOutputFromSSEData(data []byte, seen map[string]struct{}) (json.RawMessage, bool) {
	return extractImageGenerationOutputFromSSEDataWithOptions(data, seen, false)
}

func extractImageGenerationOutputFromSSEDataWithOptions(data []byte, seen map[string]struct{}, includeWithoutResult bool) (json.RawMessage, bool) {
	if len(data) == 0 || !gjson.ValidBytes(data) {
		return nil, false
	}
	if gjson.GetBytes(data, "type").String() != "response.output_item.done" {
		return nil, false
	}
	item := gjson.GetBytes(data, "item")
	if !item.Exists() || !item.IsObject() || item.Get("type").String() != "image_generation_call" {
		return nil, false
	}
	result := strings.TrimSpace(item.Get("result").String())
	if result == "" && !includeWithoutResult {
		return nil, false
	}
	key := strings.TrimSpace(item.Get("id").String())
	if key == "" {
		if result == "" {
			key = "no_result|" + strings.TrimSpace(gjson.GetBytes(data, "output_index").String())
		} else {
			key = strings.TrimSpace(item.Get("output_format").String()) + "|" + result
		}
	}
	if key != "" && seen != nil {
		if _, exists := seen[key]; exists {
			return nil, false
		}
		seen[key] = struct{}{}
	}
	return json.RawMessage(item.Raw), true
}

func (s *OpenAIGatewayService) parseSSEUsageFromBody(body string) *OpenAIUsage {
	usage := &OpenAIUsage{}
	lines := strings.Split(body, "\n")
	for _, line := range lines {
		data, ok := extractOpenAISSEDataLine(line)
		if !ok {
			continue
		}
		if data == "" || data == "[DONE]" {
			continue
		}
		s.parseSSEUsageBytes([]byte(data), usage)
	}
	return usage
}

func (s *OpenAIGatewayService) replaceModelInSSEBody(body, fromModel, toModel string) string {
	lines := strings.Split(body, "\n")
	for i, line := range lines {
		if _, ok := extractOpenAISSEDataLine(line); !ok {
			continue
		}
		lines[i] = s.replaceModelInSSELine(line, fromModel, toModel)
	}
	return strings.Join(lines, "\n")
}

func (s *OpenAIGatewayService) validateUpstreamBaseURL(raw string) (string, error) {
	if s.cfg != nil && !s.cfg.Security.URLAllowlist.Enabled {
		normalized, err := urlvalidator.ValidateURLFormat(raw, s.cfg.Security.URLAllowlist.AllowInsecureHTTP)
		if err != nil {
			return "", fmt.Errorf("invalid base_url: %w", err)
		}
		return normalized, nil
	}
	normalized, err := urlvalidator.ValidateHTTPSURL(raw, urlvalidator.ValidationOptions{
		AllowedHosts:     s.cfg.Security.URLAllowlist.UpstreamHosts,
		RequireAllowlist: true,
		AllowPrivate:     s.cfg.Security.URLAllowlist.AllowPrivateHosts,
	})
	if err != nil {
		return "", fmt.Errorf("invalid base_url: %w", err)
	}
	return normalized, nil
}

// buildOpenAIResponsesURL 组装 OpenAI Responses 端点。
// - base 以 /v1 结尾：追加 /responses
// - base 已是 /responses：原样返回
// - 其他情况：追加 /v1/responses
func buildOpenAIResponsesURL(base string) string {
	normalized := strings.TrimRight(strings.TrimSpace(base), "/")
	if strings.HasSuffix(normalized, "/responses") {
		return normalized
	}
	if strings.HasSuffix(normalized, "/v1") {
		return normalized + "/responses"
	}
	return normalized + "/v1/responses"
}

func trimOpenAIEncryptedReasoningItems(reqBody map[string]any) bool {
	if len(reqBody) == 0 {
		return false
	}

	inputValue, has := reqBody["input"]
	if !has {
		return false
	}

	switch input := inputValue.(type) {
	case []any:
		filtered := input[:0]
		changed := false
		for _, item := range input {
			nextItem, itemChanged, keep := sanitizeEncryptedReasoningInputItem(item)
			if itemChanged {
				changed = true
			}
			if !keep {
				continue
			}
			filtered = append(filtered, nextItem)
		}
		if !changed {
			return false
		}
		if len(filtered) == 0 {
			delete(reqBody, "input")
			return true
		}
		reqBody["input"] = filtered
		return true
	case []map[string]any:
		filtered := input[:0]
		changed := false
		for _, item := range input {
			nextItem, itemChanged, keep := sanitizeEncryptedReasoningInputItem(item)
			if itemChanged {
				changed = true
			}
			if !keep {
				continue
			}
			nextMap, ok := nextItem.(map[string]any)
			if !ok {
				filtered = append(filtered, item)
				continue
			}
			filtered = append(filtered, nextMap)
		}
		if !changed {
			return false
		}
		if len(filtered) == 0 {
			delete(reqBody, "input")
			return true
		}
		reqBody["input"] = filtered
		return true
	case map[string]any:
		nextItem, changed, keep := sanitizeEncryptedReasoningInputItem(input)
		if !changed {
			return false
		}
		if !keep {
			delete(reqBody, "input")
			return true
		}
		nextMap, ok := nextItem.(map[string]any)
		if !ok {
			return false
		}
		reqBody["input"] = nextMap
		return true
	default:
		return false
	}
}

func sanitizeEncryptedReasoningInputItem(item any) (next any, changed bool, keep bool) {
	inputItem, ok := item.(map[string]any)
	if !ok {
		return item, false, true
	}

	itemType, _ := inputItem["type"].(string)
	if strings.TrimSpace(itemType) != "reasoning" {
		return item, false, true
	}

	_, hasEncryptedContent := inputItem["encrypted_content"]
	if !hasEncryptedContent {
		return item, false, true
	}

	delete(inputItem, "encrypted_content")
	if len(inputItem) == 1 {
		return nil, true, false
	}
	return inputItem, true, true
}

func stripOpenAIBuiltinToolsField(reqBody map[string]any) bool {
	if reqBody == nil {
		return false
	}

	changed := false
	if _, ok := reqBody["builtin_tools"]; ok {
		delete(reqBody, "builtin_tools")
		changed = true
	}

	metadata, _ := reqBody["metadata"].(map[string]any)
	if metadata != nil {
		if _, ok := metadata["builtin_tools"]; ok {
			delete(reqBody, "metadata")
			changed = true
		}
	}

	return changed
}

func extractOpenAIBuiltinToolsCarrier(reqBody map[string]any) (any, bool) {
	if reqBody == nil {
		return nil, false
	}

	if raw, ok := reqBody["builtin_tools"]; ok {
		return raw, true
	}

	metadata, _ := reqBody["metadata"].(map[string]any)
	if metadata == nil {
		return nil, false
	}

	raw, ok := metadata["builtin_tools"]
	return raw, ok
}

func applyOpenAIBuiltinToolsAugmentation(reqBody map[string]any) bool {
	if reqBody == nil {
		return false
	}

	raw, ok := extractOpenAIBuiltinToolsCarrier(reqBody)
	if !ok {
		return false
	}

	stripOpenAIBuiltinToolsField(reqBody)
	augmented := normalizeOpenAIBuiltinTools(raw)
	if len(augmented) == 0 {
		return true
	}

	var existing []any
	if toolsRaw, hasTools := reqBody["tools"]; hasTools {
		var ok bool
		existing, ok = toolsRaw.([]any)
		if !ok {
			return true
		}
	}
	for _, tool := range augmented {
		toolType := strings.TrimSpace(fmt.Sprint(tool["type"]))
		if toolType == "" || hasOpenAIBuiltinTool(existing, toolType) {
			continue
		}
		existing = append(existing, tool)
	}

	reqBody["tools"] = existing
	return true
}

func applyOpenAIBuiltinToolsRequestPathTransform(c *gin.Context, reqBody map[string]any) bool {
	if isOpenAIResponsesCompactPath(c) {
		return stripOpenAIBuiltinToolsField(reqBody)
	}
	return applyOpenAIBuiltinToolsAugmentation(reqBody)
}

func stripOpenAIBuiltinToolsFieldFromBody(body []byte) ([]byte, bool) {
	if len(body) == 0 {
		return body, false
	}

	changed := false
	if gjson.GetBytes(body, "builtin_tools").Exists() {
		strippedBody, err := sjson.DeleteBytes(body, "builtin_tools")
		if err != nil {
			return body, false
		}
		body = strippedBody
		changed = true
	}

	if gjson.GetBytes(body, "metadata.builtin_tools").Exists() {
		strippedBody, err := sjson.DeleteBytes(body, "metadata")
		if err != nil {
			return body, changed
		}
		body = strippedBody
		changed = true
	}

	return body, changed
}

func hasOpenAIBuiltinTool(tools []any, toolType string) bool {
	for _, rawTool := range tools {
		tool, ok := rawTool.(map[string]any)
		if !ok {
			continue
		}
		if strings.TrimSpace(fmt.Sprint(tool["type"])) == toolType {
			return true
		}
	}
	return false
}

func IsOpenAIResponsesCompactPathForTest(c *gin.Context) bool {
	return isOpenAIResponsesCompactPath(c)
}

func OpenAICompactSessionSeedKeyForTest() string {
	return openAICompactSessionSeedKey
}

func NormalizeOpenAICompactRequestBodyForTest(body []byte) ([]byte, bool, error) {
	return normalizeOpenAICompactRequestBody(body)
}

func isOpenAIResponsesCompactPath(c *gin.Context) bool {
	suffix := strings.TrimSpace(openAIResponsesRequestPathSuffix(c))
	return suffix == "/compact" || strings.HasPrefix(suffix, "/compact/")
}

func normalizeOpenAICompactRequestBody(body []byte) ([]byte, bool, error) {
	if len(body) == 0 {
		return body, false, nil
	}

	normalized := []byte(`{}`)
	for _, field := range []string{"model", "input", "instructions", "previous_response_id"} {
		value := gjson.GetBytes(body, field)
		if !value.Exists() {
			continue
		}
		next, err := sjson.SetRawBytes(normalized, field, []byte(value.Raw))
		if err != nil {
			return body, false, fmt.Errorf("normalize compact body %s: %w", field, err)
		}
		normalized = next
	}

	if bytes.Equal(bytes.TrimSpace(body), bytes.TrimSpace(normalized)) {
		return body, false, nil
	}
	return normalized, true, nil
}

func resolveOpenAICompactSessionID(c *gin.Context, body []byte) string {
	if sessionID := extractOpenAISessionSignal(c, body, false); sessionID != "" {
		return sessionID
	}
	if c != nil {
		if seed, ok := c.Get(openAICompactSessionSeedKey); ok {
			if seedStr, ok := seed.(string); ok && strings.TrimSpace(seedStr) != "" {
				return strings.TrimSpace(seedStr)
			}
		}
	}
	return uuid.NewString()
}

func openAIResponsesRequestPathSuffix(c *gin.Context) string {
	if c == nil || c.Request == nil || c.Request.URL == nil {
		return ""
	}
	normalizedPath := strings.TrimRight(strings.TrimSpace(c.Request.URL.Path), "/")
	if normalizedPath == "" {
		return ""
	}
	idx := strings.LastIndex(normalizedPath, "/responses")
	if idx < 0 {
		return ""
	}
	suffix := normalizedPath[idx+len("/responses"):]
	if suffix == "" || suffix == "/" {
		return ""
	}
	if !strings.HasPrefix(suffix, "/") {
		return ""
	}
	return suffix
}

func appendOpenAIResponsesRequestPathSuffix(baseURL, suffix string) string {
	trimmedBase := strings.TrimRight(strings.TrimSpace(baseURL), "/")
	trimmedSuffix := strings.TrimSpace(suffix)
	if trimmedBase == "" || trimmedSuffix == "" {
		return trimmedBase
	}
	return trimmedBase + trimmedSuffix
}

func (s *OpenAIGatewayService) replaceModelInResponseBody(body []byte, fromModel, toModel string) []byte {
	// 使用 gjson/sjson 精确替换 model 字段，避免全量 JSON 反序列化
	if m := gjson.GetBytes(body, "model"); m.Exists() && m.Str == fromModel {
		newBody, err := sjson.SetBytes(body, "model", toModel)
		if err != nil {
			return body
		}
		return newBody
	}
	return body
}

// OpenAIRecordUsageInput input for recording usage
type OpenAIRecordUsageInput struct {
	Result       *OpenAIForwardResult
	APIKey       *APIKey
	User         *User
	Account      *Account
	Subscription *UserSubscription
	ChannelUsageFields
	RoutingSnapshot    *OpenAIRoutingSnapshot
	InboundEndpoint    string
	UpstreamEndpoint   string
	UserAgent          string // 请求的 User-Agent
	IPAddress          string // 请求的客户端 IP 地址
	RequestPayloadHash string
	APIKeyService      APIKeyQuotaUpdater
}

type openAIRecordUsageCoreInput struct {
	Result       *OpenAIForwardResult
	APIKey       *APIKey
	User         *User
	Account      *Account
	Subscription *UserSubscription
	ChannelUsageFields
	RoutingSnapshot    *OpenAIRoutingSnapshot
	InboundEndpoint    string
	UpstreamEndpoint   string
	UserAgent          string
	IPAddress          string
	RequestPayloadHash string
	APIKeyService      APIKeyQuotaUpdater
}

type openAIRecordUsageState struct {
	Result                      *OpenAIForwardResult
	APIKey                      *APIKey
	User                        *User
	Account                     *Account
	Subscription                *UserSubscription
	RequestID                   string
	UsageModel                  string
	RequestedModel              string
	EffectiveModel              string
	ActualInputTokens           int
	Multiplier                  float64
	ServiceTier                 string
	Cost                        *CostBreakdown
	ResolvedPricing             *ResolvedPricing
	AccountRateMultiplier       float64
	PriorityAccountMultiplier   float64
	EffectiveMultiplier         float64
	EffectiveInputUnitPrice     *float64
	EffectiveOutputUnitPrice    *float64
	EffectiveCacheReadUnitPrice *float64
	PricingSource               string
	IsSubscriptionBilling       bool
	BillingType                 int8
}

func CloneOpenAIRoutingSnapshot(snapshot *OpenAIRoutingSnapshot) *OpenAIRoutingSnapshot {
	if snapshot == nil {
		return nil
	}
	cloned := *snapshot
	cloned.Sticky = cloneOpenAIStickyEval(snapshot.Sticky)
	if snapshot.SelectedAccountID != nil {
		selectedAccountID := *snapshot.SelectedAccountID
		cloned.SelectedAccountID = &selectedAccountID
	}
	if snapshot.SelectedAccountName != nil {
		selectedAccountName := *snapshot.SelectedAccountName
		cloned.SelectedAccountName = &selectedAccountName
	}
	return &cloned
}

// RecordUsage records usage and deducts balance
func (s *OpenAIGatewayService) RecordUsage(ctx context.Context, input *OpenAIRecordUsageInput) error {
	if s.rateLimitService != nil && input != nil && input.Account != nil && input.Account.Platform == PlatformOpenAI {
		s.rateLimitService.ResetOpenAI403Counter(ctx, input.Account.ID)
	}
	return s.recordUsageCore(ctx, &openAIRecordUsageCoreInput{
		Result:             input.Result,
		APIKey:             input.APIKey,
		User:               input.User,
		Account:            input.Account,
		Subscription:       input.Subscription,
		ChannelUsageFields: input.ChannelUsageFields,
		RoutingSnapshot:    input.RoutingSnapshot,
		InboundEndpoint:    input.InboundEndpoint,
		UpstreamEndpoint:   input.UpstreamEndpoint,
		UserAgent:          input.UserAgent,
		IPAddress:          input.IPAddress,
		RequestPayloadHash: input.RequestPayloadHash,
		APIKeyService:      input.APIKeyService,
	})
}

func (s *OpenAIGatewayService) getUserGroupRateMultiplier(ctx context.Context, userID, groupID int64, groupDefaultMultiplier float64) float64 {
	if s == nil {
		return groupDefaultMultiplier
	}
	resolver := s.userGroupRateResolver
	if resolver == nil {
		resolver = newUserGroupRateResolver(nil, nil, resolveUserGroupRateCacheTTL(s.cfg), nil, "service.openai_gateway")
	}
	return resolver.Resolve(ctx, userID, groupID, groupDefaultMultiplier)
}

func (s *OpenAIGatewayService) recordUsageCore(ctx context.Context, input *openAIRecordUsageCoreInput) error {
	state := s.prepareOpenAIRecordUsageState(ctx, input)
	if state == nil {
		return nil
	}

	usageLog := s.buildOpenAIRecordUsageLog(ctx, input, state)
	if s.cfg != nil && s.cfg.RunMode == config.RunModeSimple {
		writeUsageLogBestEffort(ctx, s.usageLogRepo, usageLog, "service.openai_gateway")
		logger.LegacyPrintf("service.openai_gateway", "[SIMPLE MODE] Usage recorded (not billed): user=%d, tokens=%d", usageLog.UserID, usageLog.TotalTokens())
		s.deferredService.ScheduleLastUsedUpdate(state.Account.ID)
		return nil
	}

	_, billingErr := applyUsageBilling(ctx, state.RequestID, usageLog, &postUsageBillingParams{
		Cost:                  state.Cost,
		User:                  state.User,
		APIKey:                state.APIKey,
		Account:               state.Account,
		Subscription:          state.Subscription,
		RequestPayloadHash:    resolveUsageBillingPayloadFingerprint(ctx, input.RequestPayloadHash),
		IsSubscriptionBill:    state.IsSubscriptionBilling,
		AccountRateMultiplier: state.AccountRateMultiplier,
		APIKeyService:         input.APIKeyService,
	}, s.billingDeps(), s.usageBillingRepo)
	if billingErr != nil {
		return billingErr
	}
	writeUsageLogBestEffort(ctx, s.usageLogRepo, usageLog, "service.openai_gateway")

	return nil
}

func (s *OpenAIGatewayService) prepareOpenAIRecordUsageState(ctx context.Context, input *openAIRecordUsageCoreInput) *openAIRecordUsageState {
	result := input.Result
	if result.Usage.InputTokens == 0 && result.Usage.OutputTokens == 0 &&
		result.Usage.CacheCreationInputTokens == 0 && result.Usage.CacheReadInputTokens == 0 &&
		result.Usage.ImageOutputTokens == 0 && result.ImageCount == 0 {
		return nil
	}

	state := &openAIRecordUsageState{
		Result:       result,
		APIKey:       input.APIKey,
		User:         input.User,
		Account:      input.Account,
		Subscription: input.Subscription,
		RequestID:    resolveUsageBillingRequestID(ctx, result.RequestID),
	}

	state.ActualInputTokens = result.Usage.InputTokens - result.Usage.CacheReadInputTokens
	if state.ActualInputTokens < 0 {
		state.ActualInputTokens = 0
	}

	state.Multiplier = 1.0
	if s.cfg != nil {
		state.Multiplier = s.cfg.Default.RateMultiplier
	}
	if state.APIKey.GroupID != nil && state.APIKey.Group != nil {
		state.Multiplier = s.getUserGroupRateMultiplier(ctx, state.User.ID, *state.APIKey.GroupID, state.APIKey.Group.RateMultiplier)
	}
	if state.Multiplier <= 0 {
		state.Multiplier = 1.0
	}

	billingModel := forwardResultBillingModel(result.Model, result.UpstreamModel)
	if result.BillingModel != "" {
		billingModel = strings.TrimSpace(result.BillingModel)
	}
	if input.BillingModelSource == BillingModelSourceChannelMapped && input.ChannelMappedModel != "" && input.ChannelMappedModel != input.OriginalModel {
		billingModel = input.ChannelMappedModel
	}
	if input.BillingModelSource == BillingModelSourceRequested && input.OriginalModel != "" {
		billingModel = input.OriginalModel
	}

	if result.ServiceTier != nil {
		state.ServiceTier = strings.TrimSpace(*result.ServiceTier)
	}

	tokens := UsageTokens{
		InputTokens:         state.ActualInputTokens,
		OutputTokens:        result.Usage.OutputTokens,
		CacheCreationTokens: result.Usage.CacheCreationInputTokens,
		CacheReadTokens:     result.Usage.CacheReadInputTokens,
		ImageOutputTokens:   result.Usage.ImageOutputTokens,
	}

	if s.resolver != nil && state.APIKey.GroupID != nil {
		state.ResolvedPricing = s.resolver.Resolve(ctx, PricingInput{Model: billingModel, GroupID: state.APIKey.GroupID})
	}

	var err error
	state.Cost, err = s.calculateOpenAIRecordUsageCost(ctx, result, state.APIKey, billingModel, state.Multiplier, tokens, state.ServiceTier)
	if err != nil {
		state.Cost = &CostBreakdown{ActualCost: 0}
	}

	pricingSourceParts := make([]string, 0, 3)
	state.AccountRateMultiplier = state.Account.BillingRateMultiplier()
	var pricing *ModelPricing
	applySessionLongContext := true
	if state.ResolvedPricing != nil {
		if state.ResolvedPricing.Mode == BillingModeToken && s.resolver != nil {
			pricing = s.resolver.GetIntervalPricing(state.ResolvedPricing, tokens.InputTokens+tokens.CacheReadTokens)
			applySessionLongContext = len(state.ResolvedPricing.Intervals) == 0
			if source := strings.TrimSpace(state.ResolvedPricing.Source); source != "" {
				pricingSourceParts = append(pricingSourceParts, source)
			}
		}
	} else if resolvedPricing, priceErr := s.billingService.GetModelPricing(billingModel); priceErr == nil && resolvedPricing != nil {
		pricing = resolvedPricing
	}
	if pricing != nil {
		pricing = s.billingService.applyModelSpecificPricingPolicy(billingModel, pricing)
		inputUnitPrice := pricing.InputPricePerToken
		outputUnitPrice := pricing.OutputPricePerToken
		cacheReadUnitPrice := pricing.CacheReadPricePerToken
		if usePriorityServiceTierPricing(state.ServiceTier, pricing) {
			if pricing.InputPricePerTokenPriority > 0 {
				inputUnitPrice = pricing.InputPricePerTokenPriority
			}
			if pricing.OutputPricePerTokenPriority > 0 {
				outputUnitPrice = pricing.OutputPricePerTokenPriority
			}
			if pricing.CacheReadPricePerTokenPriority > 0 {
				cacheReadUnitPrice = pricing.CacheReadPricePerTokenPriority
			}
			pricingSourceParts = append(pricingSourceParts, "priority_pricing")
		} else {
			tierMultiplier := serviceTierCostMultiplier(state.ServiceTier)
			if tierMultiplier != 1.0 {
				inputUnitPrice *= tierMultiplier
				outputUnitPrice *= tierMultiplier
				cacheReadUnitPrice *= tierMultiplier
			}
		}
		if applySessionLongContext && s.billingService.shouldApplySessionLongContextPricing(tokens, pricing) {
			inputUnitPrice *= pricing.LongContextInputMultiplier
			outputUnitPrice *= pricing.LongContextOutputMultiplier
			pricingSourceParts = append(pricingSourceParts, "long_context_pricing")
		}
		state.EffectiveInputUnitPrice = &inputUnitPrice
		state.EffectiveOutputUnitPrice = &outputUnitPrice
		state.EffectiveCacheReadUnitPrice = &cacheReadUnitPrice
	}

	state.PriorityAccountMultiplier = 1.0
	if snapshot := input.RoutingSnapshot; snapshot != nil && normalizeTargetGroup(AccountTargetGroup(snapshot.TargetGroup)) == TargetGroupActive {
		state.PriorityAccountMultiplier = 100.0
		pricingSourceParts = append(pricingSourceParts, "priority_account_multiplier")
	}
	state.EffectiveMultiplier = state.Multiplier * state.AccountRateMultiplier * state.PriorityAccountMultiplier
	state.Cost.ActualCost = state.Cost.TotalCost * state.EffectiveMultiplier
	state.PricingSource = strings.Join(pricingSourceParts, ",")

	state.UsageModel = strings.TrimSpace(result.Model)
	state.RequestedModel = state.UsageModel
	if input.OriginalModel != "" {
		state.RequestedModel = input.OriginalModel
	}
	state.EffectiveModel = strings.TrimSpace(result.UpstreamModel)
	if state.EffectiveModel == "" {
		state.EffectiveModel = state.UsageModel
	}
	if snapshot := input.RoutingSnapshot; snapshot != nil {
		if value := strings.TrimSpace(snapshot.RequestedModel); value != "" {
			state.UsageModel = value
			state.RequestedModel = value
		}
		if value := strings.TrimSpace(snapshot.EffectiveModel); value != "" {
			state.EffectiveModel = value
		}
	}

	state.IsSubscriptionBilling = state.Subscription != nil && state.APIKey.Group != nil && state.APIKey.Group.IsSubscriptionType()
	state.BillingType = BillingTypeBalance
	if state.IsSubscriptionBilling {
		state.BillingType = BillingTypeSubscription
	}

	return state
}

func (s *OpenAIGatewayService) buildOpenAIRecordUsageLog(ctx context.Context, input *openAIRecordUsageCoreInput, state *openAIRecordUsageState) *UsageLog {
	durationMs := int(state.Result.Duration.Milliseconds())
	usageLog := &UsageLog{
		UserID:            state.User.ID,
		APIKeyID:          state.APIKey.ID,
		AccountID:         state.Account.ID,
		RequestID:         state.RequestID,
		Model:             state.UsageModel,
		RequestedModel:    state.RequestedModel,
		UpstreamModel:     optionalNonEqualStringPtr(state.EffectiveModel, state.UsageModel),
		ChannelID:         optionalInt64Ptr(input.ChannelID),
		ModelMappingChain: optionalTrimmedStringPtr(input.ModelMappingChain),
		BillingTier: func() *string {
			if state.ResolvedPricing == nil || strings.TrimSpace(state.ResolvedPricing.Source) == "" {
				return nil
			}
			v := strings.TrimSpace(state.ResolvedPricing.Source)
			return &v
		}(),
		ServiceTier:                 state.Result.ServiceTier,
		ReasoningEffort:             state.Result.ReasoningEffort,
		InboundEndpoint:             optionalTrimmedStringPtr(input.InboundEndpoint),
		UpstreamEndpoint:            optionalTrimmedStringPtr(input.UpstreamEndpoint),
		InputTokens:                 state.ActualInputTokens,
		OutputTokens:                state.Result.Usage.OutputTokens,
		CacheCreationTokens:         state.Result.Usage.CacheCreationInputTokens,
		CacheReadTokens:             state.Result.Usage.CacheReadInputTokens,
		ImageOutputTokens:           state.Result.Usage.ImageOutputTokens,
		ImageCount:                  state.Result.ImageCount,
		ImageSize:                   optionalTrimmedStringPtr(state.Result.ImageSize),
		InputCost:                   state.Cost.InputCost,
		OutputCost:                  state.Cost.OutputCost,
		ImageOutputCost:             state.Cost.ImageOutputCost,
		CacheCreationCost:           state.Cost.CacheCreationCost,
		CacheReadCost:               state.Cost.CacheReadCost,
		TotalCost:                   state.Cost.TotalCost,
		ActualCost:                  state.Cost.ActualCost,
		RateMultiplier:              state.Multiplier,
		AccountRateMultiplier:       &state.AccountRateMultiplier,
		PriorityAccountMultiplier:   &state.PriorityAccountMultiplier,
		EffectiveMultiplier:         &state.EffectiveMultiplier,
		EffectiveInputUnitPrice:     state.EffectiveInputUnitPrice,
		EffectiveOutputUnitPrice:    state.EffectiveOutputUnitPrice,
		EffectiveCacheReadUnitPrice: state.EffectiveCacheReadUnitPrice,
		PricingSource:               optionalTrimmedStringPtr(state.PricingSource),
		BillingType:                 state.BillingType,
		BillingMode:                 optionalTrimmedStringPtr(state.Cost.BillingMode),
		Stream:                      state.Result.Stream,
		OpenAIWSMode:                state.Result.OpenAIWSMode,
		DurationMs:                  &durationMs,
		FirstTokenMs:                state.Result.FirstTokenMs,
		CreatedAt:                   time.Now(),
	}
	if snapshot := input.RoutingSnapshot; snapshot != nil {
		usageLog.RoutingTargetGroup = optionalTrimmedStringPtr(snapshot.TargetGroup)
		usageLog.RoutingSelectedGroup = optionalTrimmedStringPtr(snapshot.SelectedGroup)
		usageLog.RoutingScheduleLayer = optionalTrimmedStringPtr(snapshot.ScheduleLayer)
		usageLog.RoutingSelectedAccountID = snapshot.SelectedAccountID
		usageLog.RoutingSelectedAccountName = snapshot.SelectedAccountName
		usageLog.RoutingEffectiveModel = optionalTrimmedStringPtr(snapshot.EffectiveModel)
		usageLog.RoutingFailoverCount = intPtrValue(snapshot.FailoverCount)
		usageLog.RoutingFailoverFinalReason = optionalTrimmedStringPtr(snapshot.FailoverFinalReason)
		if sticky := snapshot.Sticky; sticky != nil {
			usageLog.StickySessionSource = optionalTrimmedStringPtr(sticky.SessionSource)
			usageLog.StickySessionHashPresent = boolPtr(sticky.SessionHashPresent)
			usageLog.StickyEvalResult = optionalTrimmedStringPtr(sticky.EvalResult)
			usageLog.StickySelectedAccountChanged = boolPtr(sticky.SelectedAccountChanged)
			usageLog.StickyParentSessionPresent = boolPtr(sticky.ParentSessionPresent)
			usageLog.StickyParentSessionKey = optionalTrimmedStringPtr(sticky.ParentSessionKey)
		}
	}
	if usageLog.BillingMode == nil {
		billingMode := string(BillingModeToken)
		usageLog.BillingMode = &billingMode
	}
	usageLog.UserAgent = optionalTrimmedStringPtr(input.UserAgent)
	usageLog.IPAddress = optionalTrimmedStringPtr(input.IPAddress)
	if state.APIKey.GroupID != nil {
		usageLog.GroupID = state.APIKey.GroupID
	}
	if state.Subscription != nil {
		usageLog.SubscriptionID = &state.Subscription.ID
	}

	if state.APIKey.GroupID != nil {
		applyAccountStatsCost(ctx, usageLog, s.channelService, s.billingService,
			state.Account.ID, *state.APIKey.GroupID, state.Result.UpstreamModel, state.Result.Model,
			UsageTokens{
				InputTokens:         state.ActualInputTokens,
				OutputTokens:        state.Result.Usage.OutputTokens,
				CacheCreationTokens: state.Result.Usage.CacheCreationInputTokens,
				CacheReadTokens:     state.Result.Usage.CacheReadInputTokens,
				ImageOutputTokens:   state.Result.Usage.ImageOutputTokens,
			},
			state.Cost.TotalCost,
		)
	}

	return usageLog
}

func (s *OpenAIGatewayService) calculateOpenAIRecordUsageCost(
	ctx context.Context,
	result *OpenAIForwardResult,
	apiKey *APIKey,
	billingModel string,
	multiplier float64,
	tokens UsageTokens,
	serviceTier string,
) (*CostBreakdown, error) {
	if result != nil && result.ImageCount > 0 {
		return s.calculateOpenAIImageCost(ctx, billingModel, apiKey, result, multiplier), nil
	}
	if s.resolver != nil && apiKey.Group != nil {
		gid := apiKey.Group.ID
		return s.billingService.CalculateCostUnified(CostInput{
			Ctx:            ctx,
			Model:          billingModel,
			GroupID:        &gid,
			Tokens:         tokens,
			RequestCount:   1,
			RateMultiplier: multiplier,
			ServiceTier:    serviceTier,
			Resolver:       s.resolver,
		})
	}
	return s.billingService.CalculateCostWithServiceTier(billingModel, tokens, multiplier, serviceTier)
}

func (s *OpenAIGatewayService) calculateOpenAIImageCost(
	ctx context.Context,
	billingModel string,
	apiKey *APIKey,
	result *OpenAIForwardResult,
	multiplier float64,
) *CostBreakdown {
	if resolved := s.resolveOpenAIChannelPricing(ctx, billingModel, apiKey); resolved != nil &&
		(resolved.Mode == BillingModePerRequest || resolved.Mode == BillingModeImage) {
		gid := apiKey.Group.ID
		cost, err := s.billingService.CalculateCostUnified(CostInput{
			Ctx:            ctx,
			Model:          billingModel,
			GroupID:        &gid,
			RequestCount:   1,
			SizeTier:       result.ImageSize,
			RateMultiplier: multiplier,
			Resolver:       s.resolver,
			Resolved:       resolved,
		})
		if err == nil {
			return cost
		}
		logger.LegacyPrintf("service.openai_gateway", "Calculate image channel cost failed: %v", err)
	}

	var groupConfig *ImagePriceConfig
	if apiKey != nil && apiKey.Group != nil {
		groupConfig = &ImagePriceConfig{
			Price1K: apiKey.Group.ImagePrice1K,
			Price2K: apiKey.Group.ImagePrice2K,
			Price4K: apiKey.Group.ImagePrice4K,
		}
	}
	return s.billingService.CalculateImageCost(billingModel, result.ImageSize, result.ImageCount, groupConfig, multiplier)
}

func (s *OpenAIGatewayService) resolveOpenAIChannelPricing(ctx context.Context, billingModel string, apiKey *APIKey) *ResolvedPricing {
	if s.resolver == nil || apiKey == nil || apiKey.Group == nil {
		return nil
	}
	gid := apiKey.Group.ID
	resolved := s.resolver.Resolve(ctx, PricingInput{Model: billingModel, GroupID: &gid})
	if resolved.Source == PricingSourceChannel {
		return resolved
	}
	return nil
}

// ParseCodexRateLimitHeaders extracts Codex usage limits from response headers.
// Exported for use in ratelimit_service when handling OpenAI 429 responses.
func ParseCodexRateLimitHeaders(headers http.Header) *OpenAICodexUsageSnapshot {
	snapshot := &OpenAICodexUsageSnapshot{}
	hasData := false

	// Helper to parse float64 from header
	parseFloat := func(key string) *float64 {
		if v := headers.Get(key); v != "" {
			if f, err := strconv.ParseFloat(v, 64); err == nil {
				return &f
			}
		}
		return nil
	}

	// Helper to parse int from header
	parseInt := func(key string) *int {
		if v := headers.Get(key); v != "" {
			if i, err := strconv.Atoi(v); err == nil {
				return &i
			}
		}
		return nil
	}

	// Primary (weekly) limits
	if v := parseFloat("x-codex-primary-used-percent"); v != nil {
		snapshot.PrimaryUsedPercent = v
		hasData = true
	}
	if v := parseInt("x-codex-primary-reset-after-seconds"); v != nil {
		snapshot.PrimaryResetAfterSeconds = v
		hasData = true
	}
	if v := parseInt("x-codex-primary-window-minutes"); v != nil {
		snapshot.PrimaryWindowMinutes = v
		hasData = true
	}

	// Secondary (5h) limits
	if v := parseFloat("x-codex-secondary-used-percent"); v != nil {
		snapshot.SecondaryUsedPercent = v
		hasData = true
	}
	if v := parseInt("x-codex-secondary-reset-after-seconds"); v != nil {
		snapshot.SecondaryResetAfterSeconds = v
		hasData = true
	}
	if v := parseInt("x-codex-secondary-window-minutes"); v != nil {
		snapshot.SecondaryWindowMinutes = v
		hasData = true
	}

	// Overflow ratio
	if v := parseFloat("x-codex-primary-over-secondary-limit-percent"); v != nil {
		snapshot.PrimaryOverSecondaryPercent = v
		hasData = true
	}

	if !hasData {
		return nil
	}

	snapshot.UpdatedAt = time.Now().Format(time.RFC3339)
	return snapshot
}

func codexSnapshotBaseTime(snapshot *OpenAICodexUsageSnapshot, fallback time.Time) time.Time {
	if snapshot == nil {
		return fallback
	}
	if snapshot.UpdatedAt == "" {
		return fallback
	}
	base, err := time.Parse(time.RFC3339, snapshot.UpdatedAt)
	if err != nil {
		return fallback
	}
	return base
}

func codexResetAtRFC3339(base time.Time, resetAfterSeconds *int) *string {
	if resetAfterSeconds == nil {
		return nil
	}
	sec := *resetAfterSeconds
	if sec < 0 {
		sec = 0
	}
	resetAt := base.Add(time.Duration(sec) * time.Second).Format(time.RFC3339)
	return &resetAt
}

func buildCodexUsageExtraUpdates(snapshot *OpenAICodexUsageSnapshot, fallbackNow time.Time) map[string]any {
	if snapshot == nil {
		return nil
	}

	baseTime := codexSnapshotBaseTime(snapshot, fallbackNow)
	updates := make(map[string]any)

	// 保存原始 primary/secondary 字段，便于排查问题
	if snapshot.PrimaryUsedPercent != nil {
		updates["codex_primary_used_percent"] = *snapshot.PrimaryUsedPercent
	}
	if snapshot.PrimaryResetAfterSeconds != nil {
		updates["codex_primary_reset_after_seconds"] = *snapshot.PrimaryResetAfterSeconds
	}
	if snapshot.PrimaryWindowMinutes != nil {
		updates["codex_primary_window_minutes"] = *snapshot.PrimaryWindowMinutes
	}
	if snapshot.SecondaryUsedPercent != nil {
		updates["codex_secondary_used_percent"] = *snapshot.SecondaryUsedPercent
	}
	if snapshot.SecondaryResetAfterSeconds != nil {
		updates["codex_secondary_reset_after_seconds"] = *snapshot.SecondaryResetAfterSeconds
	}
	if snapshot.SecondaryWindowMinutes != nil {
		updates["codex_secondary_window_minutes"] = *snapshot.SecondaryWindowMinutes
	}
	if snapshot.PrimaryOverSecondaryPercent != nil {
		updates["codex_primary_over_secondary_percent"] = *snapshot.PrimaryOverSecondaryPercent
	}
	updates["codex_usage_updated_at"] = baseTime.Format(time.RFC3339)

	// 归一化到 5h/7d 规范字段
	if normalized := snapshot.Normalize(); normalized != nil {
		if normalized.Used5hPercent != nil {
			updates["codex_5h_used_percent"] = *normalized.Used5hPercent
		}
		if normalized.Reset5hSeconds != nil {
			updates["codex_5h_reset_after_seconds"] = *normalized.Reset5hSeconds
		}
		if normalized.Window5hMinutes != nil {
			updates["codex_5h_window_minutes"] = *normalized.Window5hMinutes
		}
		if normalized.Used7dPercent != nil {
			updates["codex_7d_used_percent"] = *normalized.Used7dPercent
		}
		if normalized.Reset7dSeconds != nil {
			updates["codex_7d_reset_after_seconds"] = *normalized.Reset7dSeconds
		}
		if normalized.Window7dMinutes != nil {
			updates["codex_7d_window_minutes"] = *normalized.Window7dMinutes
		}
		if reset5hAt := codexResetAtRFC3339(baseTime, normalized.Reset5hSeconds); reset5hAt != nil {
			updates["codex_5h_reset_at"] = *reset5hAt
		}
		if reset7dAt := codexResetAtRFC3339(baseTime, normalized.Reset7dSeconds); reset7dAt != nil {
			updates["codex_7d_reset_at"] = *reset7dAt
		}
	}

	return updates
}

// updateCodexUsageSnapshot saves the Codex usage snapshot to account's Extra field
func (s *OpenAIGatewayService) updateCodexUsageSnapshot(ctx context.Context, accountID int64, snapshot *OpenAICodexUsageSnapshot) {
	if snapshot == nil {
		return
	}
	if s == nil || s.accountRepo == nil {
		return
	}

	now := time.Now()
	updates := buildCodexUsageExtraUpdates(snapshot, now)
	if len(updates) == 0 {
		return
	}
	if !s.getCodexSnapshotThrottle().Allow(accountID, now) {
		return
	}

	go func() {
		updateCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = s.accountRepo.UpdateExtra(updateCtx, accountID, updates)
	}()
}

func (s *OpenAIGatewayService) UpdateCodexUsageSnapshotFromHeaders(ctx context.Context, accountID int64, headers http.Header) {
	if accountID <= 0 || headers == nil {
		return
	}
	if snapshot := ParseCodexRateLimitHeaders(headers); snapshot != nil {
		s.updateCodexUsageSnapshot(ctx, accountID, snapshot)
	}
}

func getOpenAIReasoningEffortFromReqBody(reqBody map[string]any) (value string, present bool) {
	if reqBody == nil {
		return "", false
	}

	// Primary: reasoning.effort
	if reasoning, ok := reqBody["reasoning"].(map[string]any); ok {
		if effort, ok := reasoning["effort"].(string); ok {
			return normalizeOpenAIReasoningEffort(effort), true
		}
	}

	// Fallback: some clients may use a flat field.
	if effort, ok := reqBody["reasoning_effort"].(string); ok {
		return normalizeOpenAIReasoningEffort(effort), true
	}

	return "", false
}

func deriveOpenAIReasoningEffortFromModel(model string) string {
	if strings.TrimSpace(model) == "" {
		return ""
	}

	modelID := strings.TrimSpace(model)
	if strings.Contains(modelID, "/") {
		parts := strings.Split(modelID, "/")
		modelID = parts[len(parts)-1]
	}

	parts := strings.FieldsFunc(strings.ToLower(modelID), func(r rune) bool {
		switch r {
		case '-', '_', ' ':
			return true
		default:
			return false
		}
	})
	if len(parts) == 0 {
		return ""
	}

	return normalizeOpenAIReasoningEffort(parts[len(parts)-1])
}

func extractOpenAIRequestMetaFromBody(body []byte) (model string, stream bool, promptCacheKey string) {
	if len(body) == 0 {
		return "", false, ""
	}

	model = strings.TrimSpace(gjson.GetBytes(body, "model").String())
	stream = gjson.GetBytes(body, "stream").Bool()
	promptCacheKey = strings.TrimSpace(gjson.GetBytes(body, "prompt_cache_key").String())
	return model, stream, promptCacheKey
}

// normalizeOpenAIPassthroughOAuthBody 将透传 OAuth 请求体收敛为旧链路关键行为：
// 1) store=false 2) 非 compact 保持 stream=true；compact 强制 stream=false
func normalizeOpenAIPassthroughOAuthBody(body []byte, compact bool) ([]byte, bool, error) {
	if len(body) == 0 {
		return body, false, nil
	}

	normalized := body
	changed := false

	if compact {
		if store := gjson.GetBytes(normalized, "store"); store.Exists() {
			next, err := sjson.DeleteBytes(normalized, "store")
			if err != nil {
				return body, false, fmt.Errorf("normalize passthrough body delete store: %w", err)
			}
			normalized = next
			changed = true
		}
		if stream := gjson.GetBytes(normalized, "stream"); stream.Exists() {
			next, err := sjson.DeleteBytes(normalized, "stream")
			if err != nil {
				return body, false, fmt.Errorf("normalize passthrough body delete stream: %w", err)
			}
			normalized = next
			changed = true
		}
	} else {
		if store := gjson.GetBytes(normalized, "store"); !store.Exists() || store.Type != gjson.False {
			next, err := sjson.SetBytes(normalized, "store", false)
			if err != nil {
				return body, false, fmt.Errorf("normalize passthrough body store=false: %w", err)
			}
			normalized = next
			changed = true
		}
		if stream := gjson.GetBytes(normalized, "stream"); !stream.Exists() || stream.Type != gjson.True {
			next, err := sjson.SetBytes(normalized, "stream", true)
			if err != nil {
				return body, false, fmt.Errorf("normalize passthrough body stream=true: %w", err)
			}
			normalized = next
			changed = true
		}
	}

	var reqBody map[string]any
	if err := json.Unmarshal(normalized, &reqBody); err != nil {
		return body, false, fmt.Errorf("normalize passthrough body parse: %w", err)
	}
	if applyInstructions(reqBody, false) {
		encoded, err := json.Marshal(reqBody)
		if err != nil {
			return body, false, fmt.Errorf("normalize passthrough body instructions: %w", err)
		}
		normalized = encoded
		changed = true
	}

	return normalized, changed, nil
}

func detectOpenAIPassthroughInstructionsRejectReason(reqModel string, body []byte) string {
	model := strings.ToLower(strings.TrimSpace(reqModel))
	if !strings.Contains(model, "codex") {
		return ""
	}

	instructions := gjson.GetBytes(body, "instructions")
	if !instructions.Exists() {
		return "instructions_missing"
	}
	if instructions.Type != gjson.String {
		return "instructions_not_string"
	}
	if strings.TrimSpace(instructions.String()) == "" {
		return "instructions_empty"
	}
	return ""
}

func extractOpenAIReasoningEffortFromBody(body []byte, requestedModel string) *string {
	reasoningEffort := strings.TrimSpace(gjson.GetBytes(body, "reasoning.effort").String())
	if reasoningEffort == "" {
		reasoningEffort = strings.TrimSpace(gjson.GetBytes(body, "reasoning_effort").String())
	}
	if reasoningEffort != "" {
		normalized := normalizeOpenAIReasoningEffort(reasoningEffort)
		if normalized == "" {
			return nil
		}
		return &normalized
	}

	value := deriveOpenAIReasoningEffortFromModel(requestedModel)
	if value == "" {
		return nil
	}
	return &value
}

func extractOpenAIServiceTier(reqBody map[string]any) *string {
	if reqBody == nil {
		return nil
	}
	raw, ok := reqBody["service_tier"].(string)
	if !ok {
		return nil
	}
	return normalizeOpenAIServiceTier(raw)
}

func extractOpenAIServiceTierFromBody(body []byte) *string {
	if len(body) == 0 {
		return nil
	}
	return normalizeOpenAIServiceTier(gjson.GetBytes(body, "service_tier").String())
}

func normalizeOpenAIServiceTier(raw string) *string {
	value := strings.ToLower(strings.TrimSpace(raw))
	if value == "" {
		return nil
	}
	if value == "fast" {
		value = "priority"
	}
	switch value {
	case "priority", "flex":
		return &value
	default:
		return nil
	}
}

func sanitizeEmptyBase64InputImagesInOpenAIBody(body []byte) ([]byte, bool, error) {
	if len(body) == 0 || !bytes.Contains(body, []byte(`"image_url"`)) || !bytes.Contains(body, []byte(`base64,`)) {
		return body, false, nil
	}

	var reqBody map[string]any
	if err := json.Unmarshal(body, &reqBody); err != nil {
		return body, false, fmt.Errorf("sanitize request body: %w", err)
	}
	if !sanitizeEmptyBase64InputImagesInOpenAIRequestBodyMap(reqBody) {
		return body, false, nil
	}
	normalized, err := json.Marshal(reqBody)
	if err != nil {
		return body, false, fmt.Errorf("serialize sanitized request body: %w", err)
	}
	return normalized, true, nil
}

func sanitizeEmptyBase64InputImagesInOpenAIRequestBodyMap(reqBody map[string]any) bool {
	if reqBody == nil {
		return false
	}
	input, ok := reqBody["input"]
	if !ok {
		return false
	}
	normalizedInput, changed := sanitizeEmptyBase64InputImagesInOpenAIInput(input)
	if !changed {
		return false
	}
	reqBody["input"] = normalizedInput
	return true
}

func sanitizeEmptyBase64InputImagesInOpenAIInput(input any) (any, bool) {
	items, ok := input.([]any)
	if !ok {
		return input, false
	}

	normalizedItems := make([]any, 0, len(items))
	changed := false
	for _, item := range items {
		itemMap, ok := item.(map[string]any)
		if !ok {
			normalizedItems = append(normalizedItems, item)
			continue
		}
		if shouldDropEmptyBase64InputImagePart(itemMap) {
			changed = true
			continue
		}
		content, ok := itemMap["content"]
		if !ok {
			normalizedItems = append(normalizedItems, itemMap)
			continue
		}
		parts, ok := content.([]any)
		if !ok {
			normalizedItems = append(normalizedItems, itemMap)
			continue
		}

		normalizedParts := make([]any, 0, len(parts))
		itemChanged := false
		for _, part := range parts {
			if shouldDropEmptyBase64InputImagePart(part) {
				changed = true
				itemChanged = true
				continue
			}
			normalizedParts = append(normalizedParts, part)
		}
		if itemChanged {
			if len(normalizedParts) == 0 {
				continue
			}
			itemMap["content"] = normalizedParts
		}
		normalizedItems = append(normalizedItems, itemMap)
	}
	if !changed {
		return input, false
	}
	return normalizedItems, true
}

func shouldDropEmptyBase64InputImagePart(part any) bool {
	partMap, ok := part.(map[string]any)
	if !ok {
		return false
	}
	typeValue, _ := partMap["type"].(string)
	if strings.TrimSpace(typeValue) != "input_image" {
		return false
	}
	imageURL, _ := partMap["image_url"].(string)
	return isEmptyBase64DataURI(imageURL)
}

func isEmptyBase64DataURI(raw string) bool {
	if !strings.HasPrefix(raw, "data:") {
		return false
	}
	rest := strings.TrimPrefix(raw, "data:")
	semicolonIdx := strings.Index(rest, ";")
	if semicolonIdx < 0 {
		return false
	}
	rest = rest[semicolonIdx+1:]
	if !strings.HasPrefix(rest, "base64,") {
		return false
	}
	return strings.TrimSpace(strings.TrimPrefix(rest, "base64,")) == ""
}

func getOpenAIRequestBodyMap(c *gin.Context, body []byte) (map[string]any, error) {
	if c != nil {
		if cached, ok := c.Get(OpenAIParsedRequestBodyKey); ok {
			if reqBody, ok := cached.(map[string]any); ok && reqBody != nil {
				return reqBody, nil
			}
		}
	}

	var reqBody map[string]any
	if err := json.Unmarshal(body, &reqBody); err != nil {
		return nil, fmt.Errorf("parse request: %w", err)
	}
	if c != nil {
		c.Set(OpenAIParsedRequestBodyKey, reqBody)
	}
	return reqBody, nil
}

func extractOpenAIReasoningEffort(reqBody map[string]any, requestedModel string) *string {
	if value, present := getOpenAIReasoningEffortFromReqBody(reqBody); present {
		if value == "" {
			return nil
		}
		return &value
	}

	value := deriveOpenAIReasoningEffortFromModel(requestedModel)
	if value == "" {
		return nil
	}
	return &value
}

func normalizeOpenAIReasoningEffort(raw string) string {
	value := strings.ToLower(strings.TrimSpace(raw))
	if value == "" {
		return ""
	}

	// Normalize separators for "x-high"/"x_high" variants.
	value = strings.NewReplacer("-", "", "_", "", " ", "").Replace(value)

	switch value {
	case "none", "minimal":
		return ""
	case "low", "medium", "high":
		return value
	case "xhigh", "extrahigh":
		return "xhigh"
	default:
		// Only store known effort levels for now to keep UI consistent.
		return ""
	}
}
