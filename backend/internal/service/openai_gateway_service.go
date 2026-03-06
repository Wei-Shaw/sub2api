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
	"math/rand"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	"github.com/Wei-Shaw/sub2api/internal/pkg/openai"
	"github.com/Wei-Shaw/sub2api/internal/util/responseheaders"
	"github.com/Wei-Shaw/sub2api/internal/util/urlvalidator"
	"github.com/gin-gonic/gin"
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
	codexCLIUserAgent      = "codex_cli_rs/0.104.0"
	// codex_cli_only 拒绝时单个请求头日志长度上限（字符）
	codexCLIOnlyHeaderValueMaxBytes = 256

	// OpenAIParsedRequestBodyKey 缓存 handler 侧已解析的请求体，避免重复解析。
	OpenAIParsedRequestBodyKey = "openai_parsed_request_body"
	// OpenAI WS Mode 失败后的重连次数上限（不含首次尝试）。
	// 与 Codex 客户端保持一致：失败后最多重连 5 次。
	openAIWSReconnectRetryLimit = 5
	// OpenAI WS Mode 重连退避默认值（可由配置覆盖）。
	openAIWSRetryBackoffInitialDefault = 120 * time.Millisecond
	openAIWSRetryBackoffMaxDefault     = 2 * time.Second
	openAIWSRetryJitterRatioDefault    = 0.2
)

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
REDACTED

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
REDACTED

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
REDACTED

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
REDACTED

// NormalizedCodexLimits contains normalized 5h/7d rate limit data
type NormalizedCodexLimits struct {
	Used5hPercent   *float64
	Reset5hSeconds  *int
	Window5hMinutes *int
	Used7dPercent   *float64
	Reset7dSeconds  *int
	Window7dMinutes *int
REDACTED

// Normalize converts primary/secondary fields to canonical 5h/7d fields.
// Strategy: Compare window_minutes to determine which is 5h vs 7d.
// Returns nil if snapshot is nil or has no useful data.
func (s *OpenAICodexUsageSnapshot) Normalize() *NormalizedCodexLimits {
	if s == nil {
		return nil
REDACTED

	result := &NormalizedCodexLimits{REDACTED

	primaryMins := 0
	secondaryMins := 0
	hasPrimaryWindow := false
	hasSecondaryWindow := false

	if s.PrimaryWindowMinutes != nil {
		primaryMins = *s.PrimaryWindowMinutes
		hasPrimaryWindow = true
REDACTED
	if s.SecondaryWindowMinutes != nil {
		secondaryMins = *s.SecondaryWindowMinutes
		hasSecondaryWindow = true
REDACTED

	// Determine mapping based on window_minutes
	use5hFromPrimary := false
	use7dFromPrimary := false

	if hasPrimaryWindow && hasSecondaryWindow {
		// Both known: smaller window is 5h, larger is 7d
		if primaryMins < secondaryMins {
			use5hFromPrimary = true
	REDACTED else {
			use7dFromPrimary = true
	REDACTED
REDACTED else if hasPrimaryWindow {
		// Only primary known: classify by threshold (<=360 min = 6h -> 5h window)
		if primaryMins <= 360 {
			use5hFromPrimary = true
	REDACTED else {
			use7dFromPrimary = true
	REDACTED
REDACTED else if hasSecondaryWindow {
		// Only secondary known: classify by threshold
		if secondaryMins <= 360 {
			// 5h from secondary, so primary (if any data) is 7d
			use7dFromPrimary = true
	REDACTED else {
			// 7d from secondary, so primary (if any data) is 5h
			use5hFromPrimary = true
	REDACTED
REDACTED else {
		// No window_minutes: fall back to legacy assumption (primary=7d, secondary=5h)
		use7dFromPrimary = true
REDACTED

	// Assign values
	if use5hFromPrimary {
		result.Used5hPercent = s.PrimaryUsedPercent
		result.Reset5hSeconds = s.PrimaryResetAfterSeconds
		result.Window5hMinutes = s.PrimaryWindowMinutes
		result.Used7dPercent = s.SecondaryUsedPercent
		result.Reset7dSeconds = s.SecondaryResetAfterSeconds
		result.Window7dMinutes = s.SecondaryWindowMinutes
REDACTED else if use7dFromPrimary {
		result.Used7dPercent = s.PrimaryUsedPercent
		result.Reset7dSeconds = s.PrimaryResetAfterSeconds
		result.Window7dMinutes = s.PrimaryWindowMinutes
		result.Used5hPercent = s.SecondaryUsedPercent
		result.Reset5hSeconds = s.SecondaryResetAfterSeconds
		result.Window5hMinutes = s.SecondaryWindowMinutes
REDACTED

	return result
REDACTED

// OpenAIUsage represents OpenAI API response usage
type OpenAIUsage struct {
	InputTokens              int `json:"input_tokens"`
	OutputTokens             int `json:"output_tokens"`
	CacheCreationInputTokens int `json:"cache_creation_input_tokens,omitempty"`
	CacheReadInputTokens     int `json:"cache_read_input_tokens,omitempty"`
REDACTED

// OpenAIForwardResult represents the result of forwarding
type OpenAIForwardResult struct {
	RequestID string
	Usage     OpenAIUsage
	Model     string
	// ReasoningEffort is extracted from request body (reasoning.effort) or derived from model suffix.
	// Stored for usage records display; nil means not provided / not applicable.
	ReasoningEffort *string
	Stream          bool
	OpenAIWSMode    bool
	Duration        time.Duration
	FirstTokenMs    *int
REDACTED

type OpenAIWSRetryMetricsSnapshot struct {
	RetryAttemptsTotal            int64 `json:"retry_attempts_total"`
	RetryBackoffMsTotal           int64 `json:"retry_backoff_ms_total"`
	RetryExhaustedTotal           int64 `json:"retry_exhausted_total"`
	NonRetryableFastFallbackTotal int64 `json:"non_retryable_fast_fallback_total"`
REDACTED

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
REDACTED

type openAIWSRetryMetrics struct {
	retryAttempts            atomic.Int64
	retryBackoffMs           atomic.Int64
	retryExhausted           atomic.Int64
	nonRetryableFastFallback atomic.Int64
REDACTED

// OpenAIGatewayService handles OpenAI API gateway operations
type OpenAIGatewayService struct {
	accountRepo         AccountRepository
	usageLogRepo        UsageLogRepository
	userRepo            UserRepository
	userSubRepo         UserSubscriptionRepository
	cache               GatewayCache
	cfg                 *config.Config
	codexDetector       CodexClientRestrictionDetector
	schedulerSnapshot   *SchedulerSnapshotService
	concurrencyService  *ConcurrencyService
	billingService      *BillingService
	rateLimitService    *RateLimitService
	billingCacheService *BillingCacheService
	httpUpstream        HTTPUpstream
	deferredService     *DeferredService
	openAITokenProvider *OpenAITokenProvider
	toolCorrector       *CodexToolCorrector
	openaiWSResolver    OpenAIWSProtocolResolver

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
REDACTED

// NewOpenAIGatewayService creates a new OpenAIGatewayService
func NewOpenAIGatewayService(
	accountRepo AccountRepository,
	usageLogRepo UsageLogRepository,
	userRepo UserRepository,
	userSubRepo UserSubscriptionRepository,
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
) *OpenAIGatewayService {
	svc := &OpenAIGatewayService{
		accountRepo:          accountRepo,
		usageLogRepo:         usageLogRepo,
		userRepo:             userRepo,
		userSubRepo:          userSubRepo,
		cache:                cache,
		cfg:                  cfg,
		codexDetector:        NewOpenAICodexClientRestrictionDetector(cfg),
		schedulerSnapshot:    schedulerSnapshot,
		concurrencyService:   concurrencyService,
		billingService:       billingService,
		rateLimitService:     rateLimitService,
		billingCacheService:  billingCacheService,
		httpUpstream:         httpUpstream,
		deferredService:      deferredService,
		openAITokenProvider:  openAITokenProvider,
		toolCorrector:        NewCodexToolCorrector(),
		openaiWSResolver:     NewOpenAIWSProtocolResolver(cfg),
		responseHeaderFilter: compileResponseHeaderFilter(cfg),
REDACTED
	svc.logOpenAIWSModeBootstrap()
	return svc
REDACTED

func (s *OpenAIGatewayService) billingDeps() *billingDeps {
	return &billingDeps{
		accountRepo:         s.accountRepo,
		userRepo:            s.userRepo,
		userSubRepo:         s.userSubRepo,
		billingCacheService: s.billingCacheService,
		deferredService:     s.deferredService,
REDACTED
REDACTED

// CloseOpenAIWSPool 关闭 OpenAI WebSocket 连接池的后台 worker 和空闲连接。
// 应在应用优雅关闭时调用。
func (s *OpenAIGatewayService) CloseOpenAIWSPool() {
	if s != nil && s.openaiWSPool != nil {
		s.openaiWSPool.Close()
REDACTED
REDACTED

func (s *OpenAIGatewayService) logOpenAIWSModeBootstrap() {
	if s == nil || s.cfg == nil {
		return
REDACTED
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
REDACTED

func (s *OpenAIGatewayService) getCodexClientRestrictionDetector() CodexClientRestrictionDetector {
	if s != nil && s.codexDetector != nil {
		return s.codexDetector
REDACTED
	var cfg *config.Config
	if s != nil {
		cfg = s.cfg
REDACTED
	return NewOpenAICodexClientRestrictionDetector(cfg)
REDACTED

func (s *OpenAIGatewayService) getOpenAIWSProtocolResolver() OpenAIWSProtocolResolver {
	if s != nil && s.openaiWSResolver != nil {
		return s.openaiWSResolver
REDACTED
	var cfg *config.Config
	if s != nil {
		cfg = s.cfg
REDACTED
	return NewOpenAIWSProtocolResolver(cfg)
REDACTED

func classifyOpenAIWSReconnectReason(err error) (string, bool) {
	if err == nil {
		return "", false
REDACTED
	var fallbackErr *openAIWSFallbackError
	if !errors.As(err, &fallbackErr) || fallbackErr == nil {
		return "", false
REDACTED
	reason := strings.TrimSpace(fallbackErr.Reason)
	if reason == "" {
		return "", false
REDACTED

	baseReason := strings.TrimPrefix(reason, "prewarm_")

	switch baseReason {
	case "policy_violation",
		"message_too_big",
		"upgrade_required",
		"ws_unsupported",
		"auth_failed",
		"previous_response_not_found":
		return reason, false
REDACTED

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
REDACTED
REDACTED

func resolveOpenAIWSFallbackErrorResponse(err error) (statusCode int, errType string, clientMessage string, upstreamMessage string, ok bool) {
	if err == nil {
		return 0, "", "", "", false
REDACTED
	var fallbackErr *openAIWSFallbackError
	if !errors.As(err, &fallbackErr) || fallbackErr == nil {
		return 0, "", "", "", false
REDACTED

	reason := strings.TrimSpace(fallbackErr.Reason)
	reason = strings.TrimPrefix(reason, "prewarm_")
	if reason == "" {
		return 0, "", "", "", false
REDACTED

	var dialErr *openAIWSDialError
	if fallbackErr.Err != nil && errors.As(fallbackErr.Err, &dialErr) && dialErr != nil {
		if dialErr.StatusCode > 0 {
			statusCode = dialErr.StatusCode
	REDACTED
		if dialErr.Err != nil {
			upstreamMessage = sanitizeUpstreamErrorMessage(strings.TrimSpace(dialErr.Err.Error()))
	REDACTED
REDACTED

	switch reason {
	case "previous_response_not_found":
		if statusCode == 0 {
			statusCode = http.StatusBadRequest
	REDACTED
		errType = "invalid_request_error"
		if upstreamMessage == "" {
			upstreamMessage = "previous response not found"
	REDACTED
	case "upgrade_required":
		if statusCode == 0 {
			statusCode = http.StatusUpgradeRequired
	REDACTED
	case "ws_unsupported":
		if statusCode == 0 {
			statusCode = http.StatusBadRequest
	REDACTED
	case "auth_failed":
		if statusCode == 0 {
			statusCode = http.StatusUnauthorized
	REDACTED
	case "upstream_rate_limited":
		if statusCode == 0 {
			statusCode = http.StatusTooManyRequests
	REDACTED
	default:
		if statusCode == 0 {
			return 0, "", "", "", false
	REDACTED
REDACTED

	if upstreamMessage == "" && fallbackErr.Err != nil {
		upstreamMessage = sanitizeUpstreamErrorMessage(strings.TrimSpace(fallbackErr.Err.Error()))
REDACTED
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
	REDACTED
REDACTED

	if errType == "" {
		if statusCode == http.StatusTooManyRequests {
			errType = "rate_limit_error"
	REDACTED else {
			errType = "upstream_error"
	REDACTED
REDACTED
	clientMessage = upstreamMessage
	return statusCode, errType, clientMessage, upstreamMessage, true
REDACTED

func (s *OpenAIGatewayService) writeOpenAIWSFallbackErrorResponse(c *gin.Context, account *Account, wsErr error) bool {
	if c == nil || c.Writer == nil || c.Writer.Written() {
		return false
REDACTED
	statusCode, errType, clientMessage, upstreamMessage, ok := resolveOpenAIWSFallbackErrorResponse(wsErr)
	if !ok {
		return false
REDACTED
	if strings.TrimSpace(clientMessage) == "" {
		clientMessage = "Upstream request failed"
REDACTED
	if strings.TrimSpace(upstreamMessage) == "" {
		upstreamMessage = clientMessage
REDACTED

	setOpsUpstreamError(c, statusCode, upstreamMessage, "")
	if account != nil {
		appendOpsUpstreamError(c, OpsUpstreamErrorEvent{
			Platform:           account.Platform,
			AccountID:          account.ID,
			AccountName:        account.Name,
			UpstreamStatusCode: statusCode,
			Kind:               "ws_error",
			Message:            upstreamMessage,
	REDACTED)
REDACTED
	c.JSON(statusCode, gin.H{
		"error": gin.H{
			"type":    errType,
			"message": clientMessage,
	REDACTED,
REDACTED)
	return true
REDACTED

func (s *OpenAIGatewayService) openAIWSRetryBackoff(attempt int) time.Duration {
	if attempt <= 0 {
		return 0
REDACTED

	initial := openAIWSRetryBackoffInitialDefault
	maxBackoff := openAIWSRetryBackoffMaxDefault
	jitterRatio := openAIWSRetryJitterRatioDefault
	if s != nil && s.cfg != nil {
		wsCfg := s.cfg.Gateway.OpenAIWS
		if wsCfg.RetryBackoffInitialMS > 0 {
			initial = time.Duration(wsCfg.RetryBackoffInitialMS) * time.Millisecond
	REDACTED
		if wsCfg.RetryBackoffMaxMS > 0 {
			maxBackoff = time.Duration(wsCfg.RetryBackoffMaxMS) * time.Millisecond
	REDACTED
		if wsCfg.RetryJitterRatio >= 0 {
			jitterRatio = wsCfg.RetryJitterRatio
	REDACTED
REDACTED
	if initial <= 0 {
		return 0
REDACTED
	if maxBackoff <= 0 {
		maxBackoff = initial
REDACTED
	if maxBackoff < initial {
		maxBackoff = initial
REDACTED
	if jitterRatio < 0 {
		jitterRatio = 0
REDACTED
	if jitterRatio > 1 {
		jitterRatio = 1
REDACTED

	shift := attempt - 1
	if shift < 0 {
		shift = 0
REDACTED
	backoff := initial
	if shift > 0 {
		backoff = initial * time.Duration(1<<shift)
REDACTED
	if backoff > maxBackoff {
		backoff = maxBackoff
REDACTED
	if jitterRatio <= 0 {
		return backoff
REDACTED
	jitter := time.Duration(float64(backoff) * jitterRatio)
	if jitter <= 0 {
		return backoff
REDACTED
	delta := time.Duration(rand.Int63n(int64(jitter)*2+1)) - jitter
	withJitter := backoff + delta
	if withJitter < 0 {
		return 0
REDACTED
	return withJitter
REDACTED

func (s *OpenAIGatewayService) openAIWSRetryTotalBudget() time.Duration {
	if s != nil && s.cfg != nil {
		ms := s.cfg.Gateway.OpenAIWS.RetryTotalBudgetMS
		if ms <= 0 {
			return 0
	REDACTED
		return time.Duration(ms) * time.Millisecond
REDACTED
	return 0
REDACTED

func (s *OpenAIGatewayService) recordOpenAIWSRetryAttempt(backoff time.Duration) {
	if s == nil {
		return
REDACTED
	s.openaiWSRetryMetrics.retryAttempts.Add(1)
	if backoff > 0 {
		s.openaiWSRetryMetrics.retryBackoffMs.Add(backoff.Milliseconds())
REDACTED
REDACTED

func (s *OpenAIGatewayService) recordOpenAIWSRetryExhausted() {
	if s == nil {
		return
REDACTED
	s.openaiWSRetryMetrics.retryExhausted.Add(1)
REDACTED

func (s *OpenAIGatewayService) recordOpenAIWSNonRetryableFastFallback() {
	if s == nil {
		return
REDACTED
	s.openaiWSRetryMetrics.nonRetryableFastFallback.Add(1)
REDACTED

func (s *OpenAIGatewayService) SnapshotOpenAIWSRetryMetrics() OpenAIWSRetryMetricsSnapshot {
	if s == nil {
		return OpenAIWSRetryMetricsSnapshot{REDACTED
REDACTED
	return OpenAIWSRetryMetricsSnapshot{
		RetryAttemptsTotal:            s.openaiWSRetryMetrics.retryAttempts.Load(),
		RetryBackoffMsTotal:           s.openaiWSRetryMetrics.retryBackoffMs.Load(),
		RetryExhaustedTotal:           s.openaiWSRetryMetrics.retryExhausted.Load(),
		NonRetryableFastFallbackTotal: s.openaiWSRetryMetrics.nonRetryableFastFallback.Load(),
REDACTED
REDACTED

func SnapshotOpenAICompatibilityFallbackMetrics() OpenAICompatibilityFallbackMetricsSnapshot {
	legacyReadFallbackTotal, legacyReadFallbackHit, legacyDualWriteTotal := openAIStickyCompatStats()
	isMaxTokensOneHaiku, thinkingEnabled, prefetchedStickyAccount, prefetchedStickyGroup, singleAccountRetry, accountSwitchCount := RequestMetadataFallbackStats()

	readHitRate := float64(0)
	if legacyReadFallbackTotal > 0 {
		readHitRate = float64(legacyReadFallbackHit) / float64(legacyReadFallbackTotal)
REDACTED
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
REDACTED
REDACTED

func (s *OpenAIGatewayService) detectCodexClientRestriction(c *gin.Context, account *Account) CodexClientRestrictionDetectionResult {
	return s.getCodexClientRestrictionDetector().Detect(c, account)
REDACTED

func getAPIKeyIDFromContext(c *gin.Context) int64 {
	if c == nil {
		return 0
REDACTED
	v, exists := c.Get("api_key")
	if !exists {
		return 0
REDACTED
	apiKey, ok := v.(*APIKey)
	if !ok || apiKey == nil {
		return 0
REDACTED
	return apiKey.ID
REDACTED

func logCodexCLIOnlyDetection(ctx context.Context, c *gin.Context, account *Account, apiKeyID int64, result CodexClientRestrictionDetectionResult, body []byte) {
	if !result.Enabled {
		return
REDACTED
	if ctx == nil {
		ctx = context.Background()
REDACTED
	accountID := int64(0)
	if account != nil {
		accountID = account.ID
REDACTED
	fields := []zap.Field{
		zap.String("component", "service.openai_gateway"),
		zap.Int64("account_id", accountID),
		zap.Bool("codex_cli_only_enabled", result.Enabled),
		zap.Bool("codex_official_client_match", result.Matched),
		zap.String("reject_reason", result.Reason),
REDACTED
	if apiKeyID > 0 {
		fields = append(fields, zap.Int64("api_key_id", apiKeyID))
REDACTED
	if !result.Matched {
		fields = appendCodexCLIOnlyRejectedRequestFields(fields, c, body)
REDACTED
	log := logger.FromContext(ctx).With(fields...)
	if result.Matched {
		return
REDACTED
	log.Warn("OpenAI codex_cli_only 拒绝非官方客户端请求")
REDACTED

func appendCodexCLIOnlyRejectedRequestFields(fields []zap.Field, c *gin.Context, body []byte) []zap.Field {
	if c == nil || c.Request == nil {
		return fields
REDACTED

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
REDACTED
	if promptCacheKey != "" {
		fields = append(fields, zap.String("request_prompt_cache_key_sha256", hashSensitiveValueForLog(promptCacheKey)))
REDACTED

	if headers := snapshotCodexCLIOnlyHeaders(req.Header); len(headers) > 0 {
		fields = append(fields, zap.Any("request_headers", headers))
REDACTED
	fields = append(fields, zap.Int("request_body_size", len(body)))
	return fields
REDACTED

func snapshotCodexCLIOnlyHeaders(header http.Header) map[string]string {
	if len(header) == 0 {
		return nil
REDACTED
	result := make(map[string]string, len(codexCLIOnlyDebugHeaderWhitelist))
	for _, key := range codexCLIOnlyDebugHeaderWhitelist {
		value := strings.TrimSpace(header.Get(key))
		if value == "" {
			continue
	REDACTED
		result[strings.ToLower(key)] = truncateString(value, codexCLIOnlyHeaderValueMaxBytes)
REDACTED
	return result
REDACTED

func hashSensitiveValueForLog(raw string) string {
	value := strings.TrimSpace(raw)
	if value == "" {
		return ""
REDACTED
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:8])
REDACTED

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
REDACTED
	if ctx == nil {
		ctx = context.Background()
REDACTED

	accountID := int64(0)
	accountName := ""
	if account != nil {
		accountID = account.ID
		accountName = strings.TrimSpace(account.Name)
REDACTED

	userAgent := ""
	if c != nil {
		userAgent = strings.TrimSpace(c.GetHeader("User-Agent"))
REDACTED

	fields := []zap.Field{
		zap.String("component", "service.openai_gateway"),
		zap.Int64("account_id", accountID),
		zap.String("account_name", accountName),
		zap.Int("upstream_status_code", upstreamStatusCode),
		zap.String("upstream_error_message", msg),
		zap.String("request_user_agent", userAgent),
		zap.Bool("codex_official_client_match", openai.IsCodexCLIRequest(userAgent)),
REDACTED
	fields = appendCodexCLIOnlyRejectedRequestFields(fields, c, requestBody)

	logger.FromContext(ctx).With(fields...).Warn("OpenAI 上游返回 Instructions are required，已记录请求详情用于排查")
REDACTED

func isOpenAIInstructionsRequiredError(upstreamStatusCode int, upstreamMsg string, upstreamBody []byte) bool {
	if upstreamStatusCode != http.StatusBadRequest {
		return false
REDACTED

	hasInstructionRequired := func(text string) bool {
		lower := strings.ToLower(strings.TrimSpace(text))
		if lower == "" {
			return false
	REDACTED
		if strings.Contains(lower, "instructions are required") {
			return true
	REDACTED
		if strings.Contains(lower, "required parameter: 'instructions'") {
			return true
	REDACTED
		if strings.Contains(lower, "required parameter: instructions") {
			return true
	REDACTED
		if strings.Contains(lower, "missing required parameter") && strings.Contains(lower, "instructions") {
			return true
	REDACTED
		return strings.Contains(lower, "instruction") && strings.Contains(lower, "required")
REDACTED

	if hasInstructionRequired(upstreamMsg) {
		return true
REDACTED
	if len(upstreamBody) == 0 {
		return false
REDACTED

	errMsg := gjson.GetBytes(upstreamBody, "error.message").String()
	errMsgLower := strings.ToLower(strings.TrimSpace(errMsg))
	errCode := strings.ToLower(strings.TrimSpace(gjson.GetBytes(upstreamBody, "error.code").String()))
	errParam := strings.ToLower(strings.TrimSpace(gjson.GetBytes(upstreamBody, "error.param").String()))
	errType := strings.ToLower(strings.TrimSpace(gjson.GetBytes(upstreamBody, "error.type").String()))

	if errParam == "instructions" {
		return true
REDACTED
	if hasInstructionRequired(errMsg) {
		return true
REDACTED
	if strings.Contains(errCode, "missing_required_parameter") && strings.Contains(errMsgLower, "instructions") {
		return true
REDACTED
	if strings.Contains(errType, "invalid_request") && strings.Contains(errMsgLower, "instructions") && strings.Contains(errMsgLower, "required") {
		return true
REDACTED

	return false
REDACTED

// ExtractSessionID extracts the raw session ID from headers or body without hashing.
// Used by ForwardAsAnthropic to pass as prompt_cache_key for upstream cache.
func (s *OpenAIGatewayService) ExtractSessionID(c *gin.Context, body []byte) string {
	if c == nil {
		return ""
REDACTED
	sessionID := strings.TrimSpace(c.GetHeader("session_id"))
	if sessionID == "" {
		sessionID = strings.TrimSpace(c.GetHeader("conversation_id"))
REDACTED
	if sessionID == "" && len(body) > 0 {
		sessionID = strings.TrimSpace(gjson.GetBytes(body, "prompt_cache_key").String())
REDACTED
	return sessionID
REDACTED

// GenerateSessionHash generates a sticky-session hash for OpenAI requests.
//
// Priority:
//  1. Header: session_id
//  2. Header: conversation_id
//  3. Body:   prompt_cache_key (opencode)
func (s *OpenAIGatewayService) GenerateSessionHash(c *gin.Context, body []byte) string {
	if c == nil {
		return ""
REDACTED

	sessionID := strings.TrimSpace(c.GetHeader("session_id"))
	if sessionID == "" {
		sessionID = strings.TrimSpace(c.GetHeader("conversation_id"))
REDACTED
	if sessionID == "" && len(body) > 0 {
		sessionID = strings.TrimSpace(gjson.GetBytes(body, "prompt_cache_key").String())
REDACTED
	if sessionID == "" {
		return ""
REDACTED

	currentHash, legacyHash := deriveOpenAISessionHashes(sessionID)
	attachOpenAILegacySessionHashToGin(c, legacyHash)
	return currentHash
REDACTED

// GenerateSessionHashWithFallback 先按常规信号生成会话哈希；
// 当未携带 session_id/conversation_id/prompt_cache_key 时，使用 fallbackSeed 生成稳定哈希。
// 该方法用于 WS ingress，避免会话信号缺失时发生跨账号漂移。
func (s *OpenAIGatewayService) GenerateSessionHashWithFallback(c *gin.Context, body []byte, fallbackSeed string) string {
	sessionHash := s.GenerateSessionHash(c, body)
	if sessionHash != "" {
		return sessionHash
REDACTED

	seed := strings.TrimSpace(fallbackSeed)
	if seed == "" {
		return ""
REDACTED

	currentHash, legacyHash := deriveOpenAISessionHashes(seed)
	attachOpenAILegacySessionHashToGin(c, legacyHash)
	return currentHash
REDACTED

// BindStickySession sets session -> account binding with standard TTL.
func (s *OpenAIGatewayService) BindStickySession(ctx context.Context, groupID *int64, sessionHash string, accountID int64) error {
	if sessionHash == "" || accountID <= 0 {
		return nil
REDACTED
	ttl := openaiStickySessionTTL
	if s != nil && s.cfg != nil && s.cfg.Gateway.OpenAIWS.StickySessionTTLSeconds > 0 {
		ttl = time.Duration(s.cfg.Gateway.OpenAIWS.StickySessionTTLSeconds) * time.Second
REDACTED
	return s.setStickySessionAccountID(ctx, groupID, sessionHash, accountID, ttl)
REDACTED

// SelectAccount selects an OpenAI account with sticky session support
func (s *OpenAIGatewayService) SelectAccount(ctx context.Context, groupID *int64, sessionHash string) (*Account, error) {
	return s.SelectAccountForModel(ctx, groupID, sessionHash, "")
REDACTED

// SelectAccountForModel selects an account supporting the requested model
func (s *OpenAIGatewayService) SelectAccountForModel(ctx context.Context, groupID *int64, sessionHash string, requestedModel string) (*Account, error) {
	return s.SelectAccountForModelWithExclusions(ctx, groupID, sessionHash, requestedModel, nil)
REDACTED

// SelectAccountForModelWithExclusions selects an account supporting the requested model while excluding specified accounts.
// SelectAccountForModelWithExclusions 选择支持指定模型的账号，同时排除指定的账号。
func (s *OpenAIGatewayService) SelectAccountForModelWithExclusions(ctx context.Context, groupID *int64, sessionHash string, requestedModel string, excludedIDs map[int64]struct{REDACTED) (*Account, error) {
	return s.selectAccountForModelWithExclusions(ctx, groupID, sessionHash, requestedModel, excludedIDs, 0)
REDACTED

func (s *OpenAIGatewayService) selectAccountForModelWithExclusions(ctx context.Context, groupID *int64, sessionHash string, requestedModel string, excludedIDs map[int64]struct{REDACTED, stickyAccountID int64) (*Account, error) {
	// 1. 尝试粘性会话命中
	// Try sticky session hit
	if account := s.tryStickySessionHit(ctx, groupID, sessionHash, requestedModel, excludedIDs, stickyAccountID); account != nil {
		return account, nil
REDACTED

	// 2. 获取可调度的 OpenAI 账号
	// Get schedulable OpenAI accounts
	accounts, err := s.listSchedulableAccounts(ctx, groupID)
	if err != nil {
		return nil, fmt.Errorf("query accounts failed: %w", err)
REDACTED

	// 3. 按优先级 + LRU 选择最佳账号
	// Select by priority + LRU
	selected := s.selectBestAccount(accounts, requestedModel, excludedIDs)

	if selected == nil {
		if requestedModel != "" {
			return nil, fmt.Errorf("no available OpenAI accounts supporting model: %s", requestedModel)
	REDACTED
		return nil, errors.New("no available OpenAI accounts")
REDACTED

	// 4. 设置粘性会话绑定
	// Set sticky session binding
	if sessionHash != "" {
		_ = s.setStickySessionAccountID(ctx, groupID, sessionHash, selected.ID, openaiStickySessionTTL)
REDACTED

	return selected, nil
REDACTED

// tryStickySessionHit 尝试从粘性会话获取账号。
// 如果命中且账号可用则返回账号；如果账号不可用则清理会话并返回 nil。
//
// tryStickySessionHit attempts to get account from sticky session.
// Returns account if hit and usable; clears session and returns nil if account is unavailable.
func (s *OpenAIGatewayService) tryStickySessionHit(ctx context.Context, groupID *int64, sessionHash, requestedModel string, excludedIDs map[int64]struct{REDACTED, stickyAccountID int64) *Account {
	if sessionHash == "" {
		return nil
REDACTED

	accountID := stickyAccountID
	if accountID <= 0 {
		var err error
		accountID, err = s.getStickySessionAccountID(ctx, groupID, sessionHash)
		if err != nil || accountID <= 0 {
			return nil
	REDACTED
REDACTED

	if _, excluded := excludedIDs[accountID]; excluded {
		return nil
REDACTED

	account, err := s.getSchedulableAccount(ctx, accountID)
	if err != nil {
		return nil
REDACTED

	// 检查账号是否需要清理粘性会话
	// Check if sticky session should be cleared
	if shouldClearStickySession(account, requestedModel) {
		_ = s.deleteStickySessionAccountID(ctx, groupID, sessionHash)
		return nil
REDACTED

	// 验证账号是否可用于当前请求
	// Verify account is usable for current request
	if !account.IsSchedulable() || !account.IsOpenAI() {
		return nil
REDACTED
	if requestedModel != "" && !account.IsModelSupported(requestedModel) {
		return nil
REDACTED

	// 刷新会话 TTL 并返回账号
	// Refresh session TTL and return account
	_ = s.refreshStickySessionTTL(ctx, groupID, sessionHash, openaiStickySessionTTL)
	return account
REDACTED

// selectBestAccount 从候选账号中选择最佳账号（优先级 + LRU）。
// 返回 nil 表示无可用账号。
//
// selectBestAccount selects the best account from candidates (priority + LRU).
// Returns nil if no available account.
func (s *OpenAIGatewayService) selectBestAccount(accounts []Account, requestedModel string, excludedIDs map[int64]struct{REDACTED) *Account {
	var selected *Account

	for i := range accounts {
		acc := &accounts[i]

		// 跳过被排除的账号
		// Skip excluded accounts
		if _, excluded := excludedIDs[acc.ID]; excluded {
			continue
	REDACTED

		// 调度器快照可能暂时过时，这里重新检查可调度性和平台
		// Scheduler snapshots can be temporarily stale; re-check schedulability and platform
		if !acc.IsSchedulable() || !acc.IsOpenAI() {
			continue
	REDACTED

		// 检查模型支持
		// Check model support
		if requestedModel != "" && !acc.IsModelSupported(requestedModel) {
			continue
	REDACTED

		// 选择优先级最高且最久未使用的账号
		// Select highest priority and least recently used
		if selected == nil {
			selected = acc
			continue
	REDACTED

		if s.isBetterAccount(acc, selected) {
			selected = acc
	REDACTED
REDACTED

	return selected
REDACTED

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
REDACTED
	if candidate.Priority > current.Priority {
		return false
REDACTED

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
REDACTED
REDACTED

// SelectAccountWithLoadAwareness selects an account with load-awareness and wait plan.
func (s *OpenAIGatewayService) SelectAccountWithLoadAwareness(ctx context.Context, groupID *int64, sessionHash string, requestedModel string, excludedIDs map[int64]struct{REDACTED) (*AccountSelectionResult, error) {
	cfg := s.schedulingConfig()
	var stickyAccountID int64
	if sessionHash != "" && s.cache != nil {
		if accountID, err := s.getStickySessionAccountID(ctx, groupID, sessionHash); err == nil {
			stickyAccountID = accountID
	REDACTED
REDACTED
	if s.concurrencyService == nil || !cfg.LoadBatchEnabled {
		account, err := s.selectAccountForModelWithExclusions(ctx, groupID, sessionHash, requestedModel, excludedIDs, stickyAccountID)
		if err != nil {
			return nil, err
	REDACTED
		result, err := s.tryAcquireAccountSlot(ctx, account.ID, account.Concurrency)
		if err == nil && result.Acquired {
			return &AccountSelectionResult{
				Account:     account,
				Acquired:    true,
				ReleaseFunc: result.ReleaseFunc,
		REDACTED, nil
	REDACTED
		if stickyAccountID > 0 && stickyAccountID == account.ID && s.concurrencyService != nil {
			waitingCount, _ := s.concurrencyService.GetAccountWaitingCount(ctx, account.ID)
			if waitingCount < cfg.StickySessionMaxWaiting {
				return &AccountSelectionResult{
					Account: account,
					WaitPlan: &AccountWaitPlan{
						AccountID:      account.ID,
						MaxConcurrency: account.Concurrency,
						Timeout:        cfg.StickySessionWaitTimeout,
						MaxWaiting:     cfg.StickySessionMaxWaiting,
				REDACTED,
			REDACTED, nil
		REDACTED
	REDACTED
		return &AccountSelectionResult{
			Account: account,
			WaitPlan: &AccountWaitPlan{
				AccountID:      account.ID,
				MaxConcurrency: account.Concurrency,
				Timeout:        cfg.FallbackWaitTimeout,
				MaxWaiting:     cfg.FallbackMaxWaiting,
		REDACTED,
	REDACTED, nil
REDACTED

	accounts, err := s.listSchedulableAccounts(ctx, groupID)
	if err != nil {
		return nil, err
REDACTED
	if len(accounts) == 0 {
		return nil, errors.New("no available accounts")
REDACTED

	isExcluded := func(accountID int64) bool {
		if excludedIDs == nil {
			return false
	REDACTED
		_, excluded := excludedIDs[accountID]
		return excluded
REDACTED

	// ============ Layer 1: Sticky session ============
	if sessionHash != "" {
		accountID := stickyAccountID
		if accountID > 0 && !isExcluded(accountID) {
			account, err := s.getSchedulableAccount(ctx, accountID)
			if err == nil {
				clearSticky := shouldClearStickySession(account, requestedModel)
				if clearSticky {
					_ = s.deleteStickySessionAccountID(ctx, groupID, sessionHash)
			REDACTED
				if !clearSticky && account.IsSchedulable() && account.IsOpenAI() &&
					(requestedModel == "" || account.IsModelSupported(requestedModel)) {
					result, err := s.tryAcquireAccountSlot(ctx, accountID, account.Concurrency)
					if err == nil && result.Acquired {
						_ = s.refreshStickySessionTTL(ctx, groupID, sessionHash, openaiStickySessionTTL)
						return &AccountSelectionResult{
							Account:     account,
							Acquired:    true,
							ReleaseFunc: result.ReleaseFunc,
					REDACTED, nil
				REDACTED

					waitingCount, _ := s.concurrencyService.GetAccountWaitingCount(ctx, accountID)
					if waitingCount < cfg.StickySessionMaxWaiting {
						return &AccountSelectionResult{
							Account: account,
							WaitPlan: &AccountWaitPlan{
								AccountID:      accountID,
								MaxConcurrency: account.Concurrency,
								Timeout:        cfg.StickySessionWaitTimeout,
								MaxWaiting:     cfg.StickySessionMaxWaiting,
						REDACTED,
					REDACTED, nil
				REDACTED
			REDACTED
		REDACTED
	REDACTED
REDACTED

	// ============ Layer 2: Load-aware selection ============
	candidates := make([]*Account, 0, len(accounts))
	for i := range accounts {
		acc := &accounts[i]
		if isExcluded(acc.ID) {
			continue
	REDACTED
		// Scheduler snapshots can be temporarily stale (bucket rebuild is throttled);
		// re-check schedulability here so recently rate-limited/overloaded accounts
		// are not selected again before the bucket is rebuilt.
		if !acc.IsSchedulable() {
			continue
	REDACTED
		if requestedModel != "" && !acc.IsModelSupported(requestedModel) {
			continue
	REDACTED
		candidates = append(candidates, acc)
REDACTED

	if len(candidates) == 0 {
		return nil, errors.New("no available accounts")
REDACTED

	accountLoads := make([]AccountWithConcurrency, 0, len(candidates))
	for _, acc := range candidates {
		accountLoads = append(accountLoads, AccountWithConcurrency{
			ID:             acc.ID,
			MaxConcurrency: acc.EffectiveLoadFactor(),
	REDACTED)
REDACTED

	loadMap, err := s.concurrencyService.GetAccountsLoadBatch(ctx, accountLoads)
	if err != nil {
		ordered := append([]*Account(nil), candidates...)
		sortAccountsByPriorityAndLastUsed(ordered, false)
		for _, acc := range ordered {
			result, err := s.tryAcquireAccountSlot(ctx, acc.ID, acc.Concurrency)
			if err == nil && result.Acquired {
				if sessionHash != "" {
					_ = s.setStickySessionAccountID(ctx, groupID, sessionHash, acc.ID, openaiStickySessionTTL)
			REDACTED
				return &AccountSelectionResult{
					Account:     acc,
					Acquired:    true,
					ReleaseFunc: result.ReleaseFunc,
			REDACTED, nil
		REDACTED
	REDACTED
REDACTED else {
		var available []accountWithLoad
		for _, acc := range candidates {
			loadInfo := loadMap[acc.ID]
			if loadInfo == nil {
				loadInfo = &AccountLoadInfo{AccountID: acc.IDREDACTED
		REDACTED
			if loadInfo.LoadRate < 100 {
				available = append(available, accountWithLoad{
					account:  acc,
					loadInfo: loadInfo,
			REDACTED)
		REDACTED
	REDACTED

		if len(available) > 0 {
			sort.SliceStable(available, func(i, j int) bool {
				a, b := available[i], available[j]
				if a.account.Priority != b.account.Priority {
					return a.account.Priority < b.account.Priority
			REDACTED
				if a.loadInfo.LoadRate != b.loadInfo.LoadRate {
					return a.loadInfo.LoadRate < b.loadInfo.LoadRate
			REDACTED
				switch {
				case a.account.LastUsedAt == nil && b.account.LastUsedAt != nil:
					return true
				case a.account.LastUsedAt != nil && b.account.LastUsedAt == nil:
					return false
				case a.account.LastUsedAt == nil && b.account.LastUsedAt == nil:
					return false
				default:
					return a.account.LastUsedAt.Before(*b.account.LastUsedAt)
			REDACTED
		REDACTED)
			shuffleWithinSortGroups(available)

			for _, item := range available {
				result, err := s.tryAcquireAccountSlot(ctx, item.account.ID, item.account.Concurrency)
				if err == nil && result.Acquired {
					if sessionHash != "" {
						_ = s.setStickySessionAccountID(ctx, groupID, sessionHash, item.account.ID, openaiStickySessionTTL)
				REDACTED
					return &AccountSelectionResult{
						Account:     item.account,
						Acquired:    true,
						ReleaseFunc: result.ReleaseFunc,
				REDACTED, nil
			REDACTED
		REDACTED
	REDACTED
REDACTED

	// ============ Layer 3: Fallback wait ============
	sortAccountsByPriorityAndLastUsed(candidates, false)
	for _, acc := range candidates {
		return &AccountSelectionResult{
			Account: acc,
			WaitPlan: &AccountWaitPlan{
				AccountID:      acc.ID,
				MaxConcurrency: acc.Concurrency,
				Timeout:        cfg.FallbackWaitTimeout,
				MaxWaiting:     cfg.FallbackMaxWaiting,
		REDACTED,
	REDACTED, nil
REDACTED

	return nil, errors.New("no available accounts")
REDACTED

func (s *OpenAIGatewayService) listSchedulableAccounts(ctx context.Context, groupID *int64) ([]Account, error) {
	if s.schedulerSnapshot != nil {
		accounts, _, err := s.schedulerSnapshot.ListSchedulableAccounts(ctx, groupID, PlatformOpenAI, false)
		return accounts, err
REDACTED
	var accounts []Account
	var err error
	if s.cfg != nil && s.cfg.RunMode == config.RunModeSimple {
		accounts, err = s.accountRepo.ListSchedulableByPlatform(ctx, PlatformOpenAI)
REDACTED else if groupID != nil {
		accounts, err = s.accountRepo.ListSchedulableByGroupIDAndPlatform(ctx, *groupID, PlatformOpenAI)
REDACTED else {
		accounts, err = s.accountRepo.ListSchedulableUngroupedByPlatform(ctx, PlatformOpenAI)
REDACTED
	if err != nil {
		return nil, fmt.Errorf("query accounts failed: %w", err)
REDACTED
	return accounts, nil
REDACTED

func (s *OpenAIGatewayService) tryAcquireAccountSlot(ctx context.Context, accountID int64, maxConcurrency int) (*AcquireResult, error) {
	if s.concurrencyService == nil {
		return &AcquireResult{Acquired: true, ReleaseFunc: func() {REDACTEDREDACTED, nil
REDACTED
	return s.concurrencyService.AcquireAccountSlot(ctx, accountID, maxConcurrency)
REDACTED

func (s *OpenAIGatewayService) getSchedulableAccount(ctx context.Context, accountID int64) (*Account, error) {
	if s.schedulerSnapshot != nil {
		return s.schedulerSnapshot.GetAccount(ctx, accountID)
REDACTED
	return s.accountRepo.GetByID(ctx, accountID)
REDACTED

func (s *OpenAIGatewayService) schedulingConfig() config.GatewaySchedulingConfig {
	if s.cfg != nil {
		return s.cfg.Gateway.Scheduling
REDACTED
	return config.GatewaySchedulingConfig{
		StickySessionMaxWaiting:  3,
		StickySessionWaitTimeout: 45 * time.Second,
		FallbackWaitTimeout:      30 * time.Second,
		FallbackMaxWaiting:       100,
		LoadBatchEnabled:         true,
		SlotCleanupInterval:      30 * time.Second,
REDACTED
REDACTED

// GetAccessToken gets the access token for an OpenAI account
func (s *OpenAIGatewayService) GetAccessToken(ctx context.Context, account *Account) (string, string, error) {
	switch account.Type {
	case AccountTypeOAuth:
		// 使用 TokenProvider 获取缓存的 token
		if s.openAITokenProvider != nil {
			accessToken, err := s.openAITokenProvider.GetAccessToken(ctx, account)
			if err != nil {
				return "", "", err
		REDACTED
			return accessToken, "oauth", nil
	REDACTED
		// 降级：TokenProvider 未配置时直接从账号读取
		accessToken := account.GetOpenAIAccessToken()
		if accessToken == "" {
			return "", "", errors.New("access_token not found in credentials")
	REDACTED
		return accessToken, "oauth", nil
	case AccountTypeAPIKey:
		apiKey := account.GetOpenAIApiKey()
		if apiKey == "" {
			return "", "", errors.New("api_key not found in credentials")
	REDACTED
		return apiKey, "apikey", nil
	default:
		return "", "", fmt.Errorf("unsupported account type: %s", account.Type)
REDACTED
REDACTED

func (s *OpenAIGatewayService) shouldFailoverUpstreamError(statusCode int) bool {
	switch statusCode {
	case 401, 402, 403, 429, 529:
		return true
	default:
		return statusCode >= 500
REDACTED
REDACTED

func (s *OpenAIGatewayService) handleFailoverSideEffects(ctx context.Context, resp *http.Response, account *Account) {
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	s.rateLimitService.HandleUpstreamError(ctx, account, resp.StatusCode, resp.Header, body)
REDACTED

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
		REDACTED,
	REDACTED)
		return nil, errors.New("codex_cli_only restriction: only codex official clients are allowed")
REDACTED

	originalBody := body
	reqModel, reqStream, promptCacheKey := extractOpenAIRequestMetaFromBody(body)
	originalModel := reqModel

	isCodexCLI := openai.IsCodexCLIRequest(c.GetHeader("User-Agent")) || (s.cfg != nil && s.cfg.Gateway.ForceCodexCLI)
	wsDecision := s.getOpenAIWSProtocolResolver().Resolve(account)
	clientTransport := GetOpenAIClientTransport(c)
	// 仅允许 WS 入站请求走 WS 上游，避免出现 HTTP -> WS 协议混用。
	wsDecision = resolveOpenAIWSDecisionByClientTransport(wsDecision, clientTransport)
	if c != nil {
		c.Set("openai_ws_transport_decision", string(wsDecision.Transport))
		c.Set("openai_ws_transport_reason", wsDecision.Reason)
REDACTED
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
REDACTED
	// 当前仅支持 WSv2；WSv1 命中时直接返回错误，避免出现“配置可开但行为不确定”。
	if wsDecision.Transport == OpenAIUpstreamTransportResponsesWebsocket {
		if c != nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"error": gin.H{
					"type":    "invalid_request_error",
					"message": "OpenAI WSv1 is temporarily unsupported. Please enable responses_websockets_v2.",
			REDACTED,
		REDACTED)
	REDACTED
		return nil, errors.New("openai ws v1 is temporarily unsupported; use ws v2")
REDACTED
	passthroughEnabled := account.IsOpenAIPassthroughEnabled()
	if passthroughEnabled {
		// 透传分支只需要轻量提取字段，避免热路径全量 Unmarshal。
		reasoningEffort := extractOpenAIReasoningEffortFromBody(body, reqModel)
		return s.forwardOpenAIPassthrough(ctx, c, account, originalBody, reqModel, reasoningEffort, reqStream, startTime)
REDACTED

	reqBody, err := getOpenAIRequestBodyMap(c, body)
	if err != nil {
		return nil, err
REDACTED

	if v, ok := reqBody["model"].(string); ok {
		reqModel = v
		originalModel = reqModel
REDACTED
	if v, ok := reqBody["stream"].(bool); ok {
		reqStream = v
REDACTED
	if promptCacheKey == "" {
		if v, ok := reqBody["prompt_cache_key"].(string); ok {
			promptCacheKey = strings.TrimSpace(v)
	REDACTED
REDACTED

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
	REDACTED
		if patchDisabled {
			return
	REDACTED
		if !patchHasOp {
			patchHasOp = true
			patchDelete = false
			patchPath = path
			patchValue = value
			return
	REDACTED
		if patchDelete || patchPath != path {
			patchDisabled = true
			return
	REDACTED
		patchValue = value
REDACTED
	markPatchDelete := func(path string) {
		if strings.TrimSpace(path) == "" {
			patchDisabled = true
			return
	REDACTED
		if patchDisabled {
			return
	REDACTED
		if !patchHasOp {
			patchHasOp = true
			patchDelete = true
			patchPath = path
			return
	REDACTED
		if !patchDelete || patchPath != path {
			patchDisabled = true
	REDACTED
REDACTED
	disablePatch := func() {
		patchDisabled = true
REDACTED

	// 非透传模式下，保持历史行为：非 Codex CLI 请求在 instructions 为空时注入默认指令。
	if !isCodexCLI && isInstructionsEmpty(reqBody) {
		if instructions := strings.TrimSpace(GetOpenCodeInstructions()); instructions != "" {
			reqBody["instructions"] = instructions
			bodyModified = true
			markPatchSet("instructions", instructions)
	REDACTED
REDACTED

	// 对所有请求执行模型映射（包含 Codex CLI）。
	mappedModel := account.GetMappedModel(reqModel)
	if mappedModel != reqModel {
		logger.LegacyPrintf("service.openai_gateway", "[OpenAI] Model mapping applied: %s -> %s (account: %s, isCodexCLI: %v)", reqModel, mappedModel, account.Name, isCodexCLI)
		reqBody["model"] = mappedModel
		bodyModified = true
		markPatchSet("model", mappedModel)
REDACTED

	// 针对所有 OpenAI 账号执行 Codex 模型名规范化，确保上游识别一致。
	if model, ok := reqBody["model"].(string); ok {
		normalizedModel := normalizeCodexModel(model)
		if normalizedModel != "" && normalizedModel != model {
			logger.LegacyPrintf("service.openai_gateway", "[OpenAI] Codex model normalization: %s -> %s (account: %s, type: %s, isCodexCLI: %v)",
				model, normalizedModel, account.Name, account.Type, isCodexCLI)
			reqBody["model"] = normalizedModel
			mappedModel = normalizedModel
			bodyModified = true
			markPatchSet("model", normalizedModel)
	REDACTED
REDACTED

	// 规范化 reasoning.effort 参数（minimal -> none），与上游允许值对齐。
	if reasoning, ok := reqBody["reasoning"].(map[string]any); ok {
		if effort, ok := reasoning["effort"].(string); ok && effort == "minimal" {
			reasoning["effort"] = "none"
			bodyModified = true
			markPatchSet("reasoning.effort", "none")
			logger.LegacyPrintf("service.openai_gateway", "[OpenAI] Normalized reasoning.effort: minimal -> none (account: %s)", account.Name)
	REDACTED
REDACTED

	if account.Type == AccountTypeOAuth {
		codexResult := applyCodexOAuthTransform(reqBody, isCodexCLI)
		if codexResult.Modified {
			bodyModified = true
			disablePatch()
	REDACTED
		if codexResult.NormalizedModel != "" {
			mappedModel = codexResult.NormalizedModel
	REDACTED
		if codexResult.PromptCacheKey != "" {
			promptCacheKey = codexResult.PromptCacheKey
	REDACTED
REDACTED

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
			REDACTED
			case PlatformAnthropic:
				// For Anthropic (Claude), convert to max_tokens
				delete(reqBody, "max_output_tokens")
				markPatchDelete("max_output_tokens")
				if _, hasMaxTokens := reqBody["max_tokens"]; !hasMaxTokens {
					reqBody["max_tokens"] = maxOutputTokens
					disablePatch()
			REDACTED
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
		REDACTED
	REDACTED

		// Also handle max_completion_tokens (similar logic)
		if _, hasMaxCompletionTokens := reqBody["max_completion_tokens"]; hasMaxCompletionTokens {
			if account.Type == AccountTypeAPIKey || account.Platform != PlatformOpenAI {
				delete(reqBody, "max_completion_tokens")
				bodyModified = true
				markPatchDelete("max_completion_tokens")
		REDACTED
	REDACTED

		// Remove unsupported fields (not supported by upstream OpenAI API)
		unsupportedFields := []string{"prompt_cache_retention", "safety_identifier"REDACTED
		for _, unsupportedField := range unsupportedFields {
			if _, has := reqBody[unsupportedField]; has {
				delete(reqBody, unsupportedField)
				bodyModified = true
				markPatchDelete(unsupportedField)
		REDACTED
	REDACTED
REDACTED

	// 仅在 WSv2 模式保留 previous_response_id，其他模式（HTTP/WSv1）统一过滤。
	// 注意：该规则同样适用于 Codex CLI 请求，避免 WSv1 向上游透传不支持字段。
	if wsDecision.Transport != OpenAIUpstreamTransportResponsesWebsocketV2 {
		if _, has := reqBody["previous_response_id"]; has {
			delete(reqBody, "previous_response_id")
			bodyModified = true
			markPatchDelete("previous_response_id")
	REDACTED
REDACTED

	// Re-serialize body only if modified
	if bodyModified {
		serializedByPatch := false
		if !patchDisabled && patchHasOp {
			var patchErr error
			if patchDelete {
				body, patchErr = sjson.DeleteBytes(body, patchPath)
		REDACTED else {
				body, patchErr = sjson.SetBytes(body, patchPath, patchValue)
		REDACTED
			if patchErr == nil {
				serializedByPatch = true
		REDACTED
	REDACTED
		if !serializedByPatch {
			var marshalErr error
			body, marshalErr = json.Marshal(reqBody)
			if marshalErr != nil {
				return nil, fmt.Errorf("serialize request body: %w", marshalErr)
		REDACTED
	REDACTED
REDACTED

	// Get access token
	token, _, err := s.GetAccessToken(ctx, account)
	if err != nil {
		return nil, err
REDACTED

	// Capture upstream request body for ops retry of this attempt.
	setOpsUpstreamRequestBody(c, body)

	// 命中 WS 时仅走 WebSocket Mode；不再自动回退 HTTP。
	if wsDecision.Transport == OpenAIUpstreamTransportResponsesWebsocketV2 {
		wsReqBody := reqBody
		if len(reqBody) > 0 {
			wsReqBody = make(map[string]any, len(reqBody))
			for k, v := range reqBody {
				wsReqBody[k] = v
		REDACTED
	REDACTED
		_, hasPreviousResponseID := wsReqBody["previous_response_id"]
		logOpenAIWSModeDebug(
			"forward_start account_id=%d account_type=%s model=%s stream=%v has_previous_response_id=%v",
			account.ID,
			account.Type,
			mappedModel,
			reqStream,
			hasPreviousResponseID,
		)
		maxAttempts := openAIWSReconnectRetryLimit + 1
		wsAttempts := 0
		var wsResult *OpenAIForwardResult
		var wsErr error
		wsLastFailureReason := ""
		wsPrevResponseRecoveryTried := false
		recoverPrevResponseNotFound := func(attempt int) bool {
			if wsPrevResponseRecoveryTried {
				return false
		REDACTED
			previousResponseID := openAIWSPayloadString(wsReqBody, "previous_response_id")
			if previousResponseID == "" {
				logOpenAIWSModeInfo(
					"reconnect_prev_response_recovery_skip account_id=%d attempt=%d reason=missing_previous_response_id previous_response_id_present=false",
					account.ID,
					attempt,
				)
				return false
		REDACTED
			if HasFunctionCallOutput(wsReqBody) {
				logOpenAIWSModeInfo(
					"reconnect_prev_response_recovery_skip account_id=%d attempt=%d reason=has_function_call_output previous_response_id_present=true",
					account.ID,
					attempt,
				)
				return false
		REDACTED
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
	REDACTED
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
				mappedModel,
				startTime,
				attempt,
				wsLastFailureReason,
			)
			if wsErr == nil {
				break
		REDACTED
			if c != nil && c.Writer != nil && c.Writer.Written() {
				break
		REDACTED

			reason, retryable := classifyOpenAIWSReconnectReason(wsErr)
			if reason != "" {
				wsLastFailureReason = reason
		REDACTED
			// previous_response_not_found 说明续链锚点不可用：
			// 对非 function_call_output 场景，允许一次“去掉 previous_response_id 后重放”。
			if reason == "previous_response_not_found" && recoverPrevResponseNotFound(attempt) {
				continue
		REDACTED
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
			REDACTED
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
					REDACTED
						wsErr = wrapOpenAIWSFallback("retry_backoff_canceled", ctx.Err())
						break wsRetryLoop
					case <-timer.C:
				REDACTED
			REDACTED
				continue
		REDACTED
			if retryable {
				s.recordOpenAIWSRetryExhausted()
				logOpenAIWSModeInfo(
					"reconnect_exhausted account_id=%d attempts=%d max_retries=%d reason=%s",
					account.ID,
					attempt,
					openAIWSReconnectRetryLimit,
					normalizeOpenAIWSLogValue(reason),
				)
		REDACTED else if reason != "" {
				s.recordOpenAIWSNonRetryableFastFallback()
				logOpenAIWSModeInfo(
					"reconnect_stop account_id=%d attempt=%d reason=%s",
					account.ID,
					attempt,
					normalizeOpenAIWSLogValue(reason),
				)
		REDACTED
			break
	REDACTED
		if wsErr == nil {
			firstTokenMs := int64(0)
			hasFirstTokenMs := wsResult != nil && wsResult.FirstTokenMs != nil
			if hasFirstTokenMs {
				firstTokenMs = int64(*wsResult.FirstTokenMs)
		REDACTED
			requestID := ""
			if wsResult != nil {
				requestID = strings.TrimSpace(wsResult.RequestID)
		REDACTED
			logOpenAIWSModeDebug(
				"forward_succeeded account_id=%d request_id=%s stream=%v has_first_token_ms=%v first_token_ms=%d ws_attempts=%d",
				account.ID,
				requestID,
				reqStream,
				hasFirstTokenMs,
				firstTokenMs,
				wsAttempts,
			)
			return wsResult, nil
	REDACTED
		s.writeOpenAIWSFallbackErrorResponse(c, account, wsErr)
		return nil, wsErr
REDACTED

	// Build upstream request
	upstreamReq, err := s.buildUpstreamRequest(ctx, c, account, body, token, reqStream, promptCacheKey, isCodexCLI)
	if err != nil {
		return nil, err
REDACTED

	// Get proxy URL
	proxyURL := ""
	if account.ProxyID != nil && account.Proxy != nil {
		proxyURL = account.Proxy.URL()
REDACTED

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
	REDACTED)
		c.JSON(http.StatusBadGateway, gin.H{
			"error": gin.H{
				"type":    "upstream_error",
				"message": "Upstream request failed",
		REDACTED,
	REDACTED)
		return nil, fmt.Errorf("upstream request failed: %s", safeErr)
REDACTED
	defer func() { _ = resp.Body.Close() REDACTED()

	// Handle error response
	if resp.StatusCode >= 400 {
		if s.shouldFailoverUpstreamError(resp.StatusCode) {
			respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
			_ = resp.Body.Close()
			resp.Body = io.NopCloser(bytes.NewReader(respBody))

			upstreamMsg := strings.TrimSpace(extractUpstreamErrorMessage(respBody))
			upstreamMsg = sanitizeUpstreamErrorMessage(upstreamMsg)
			upstreamDetail := ""
			if s.cfg != nil && s.cfg.Gateway.LogUpstreamErrorBody {
				maxBytes := s.cfg.Gateway.LogUpstreamErrorBodyMaxBytes
				if maxBytes <= 0 {
					maxBytes = 2048
			REDACTED
				upstreamDetail = truncateString(string(respBody), maxBytes)
		REDACTED
			appendOpsUpstreamError(c, OpsUpstreamErrorEvent{
				Platform:           account.Platform,
				AccountID:          account.ID,
				AccountName:        account.Name,
				UpstreamStatusCode: resp.StatusCode,
				UpstreamRequestID:  resp.Header.Get("x-request-id"),
				Kind:               "failover",
				Message:            upstreamMsg,
				Detail:             upstreamDetail,
		REDACTED)

			s.handleFailoverSideEffects(ctx, resp, account)
			return nil, &UpstreamFailoverError{StatusCode: resp.StatusCode, ResponseBody: respBodyREDACTED
	REDACTED
		return s.handleErrorResponse(ctx, resp, c, account, body)
REDACTED

	// Handle normal response
	var usage *OpenAIUsage
	var firstTokenMs *int
	if reqStream {
		streamResult, err := s.handleStreamingResponse(ctx, resp, c, account, startTime, originalModel, mappedModel)
		if err != nil {
			return nil, err
	REDACTED
		usage = streamResult.usage
		firstTokenMs = streamResult.firstTokenMs
REDACTED else {
		usage, err = s.handleNonStreamingResponse(ctx, resp, c, account, originalModel, mappedModel)
		if err != nil {
			return nil, err
	REDACTED
REDACTED

	// Extract and save Codex usage snapshot from response headers (for OAuth accounts)
	if account.Type == AccountTypeOAuth {
		if snapshot := ParseCodexRateLimitHeaders(resp.Header); snapshot != nil {
			s.updateCodexUsageSnapshot(ctx, account.ID, snapshot)
	REDACTED
REDACTED

	if usage == nil {
		usage = &OpenAIUsage{REDACTED
REDACTED

	reasoningEffort := extractOpenAIReasoningEffort(reqBody, originalModel)

	return &OpenAIForwardResult{
		RequestID:       resp.Header.Get("x-request-id"),
		Usage:           *usage,
		Model:           originalModel,
		ReasoningEffort: reasoningEffort,
		Stream:          reqStream,
		OpenAIWSMode:    false,
		Duration:        time.Since(startTime),
		FirstTokenMs:    firstTokenMs,
REDACTED, nil
REDACTED

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
		REDACTED)
			logOpenAIPassthroughInstructionsRejected(ctx, c, account, reqModel, rejectReason, body)
			c.JSON(http.StatusForbidden, gin.H{
				"error": gin.H{
					"type":    "forbidden_error",
					"message": rejectMsg,
			REDACTED,
		REDACTED)
			return nil, fmt.Errorf("openai passthrough rejected before upstream: %s", rejectReason)
	REDACTED

		normalizedBody, normalized, err := normalizeOpenAIPassthroughOAuthBody(body)
		if err != nil {
			return nil, err
	REDACTED
		if normalized {
			body = normalizedBody
			reqStream = true
	REDACTED
REDACTED

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
		REDACTED else {
				streamWarnLogger.Warn("OpenAI passthrough 检测到超时相关请求头，将按配置过滤以降低断流风险")
		REDACTED
	REDACTED
REDACTED

	// Get access token
	token, _, err := s.GetAccessToken(ctx, account)
	if err != nil {
		return nil, err
REDACTED

	upstreamReq, err := s.buildUpstreamRequestOpenAIPassthrough(ctx, c, account, body, token)
	if err != nil {
		return nil, err
REDACTED

	proxyURL := ""
	if account.ProxyID != nil && account.Proxy != nil {
		proxyURL = account.Proxy.URL()
REDACTED

	setOpsUpstreamRequestBody(c, body)
	if c != nil {
		c.Set("openai_passthrough", true)
REDACTED

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
	REDACTED)
		c.JSON(http.StatusBadGateway, gin.H{
			"error": gin.H{
				"type":    "upstream_error",
				"message": "Upstream request failed",
		REDACTED,
	REDACTED)
		return nil, fmt.Errorf("upstream request failed: %s", safeErr)
REDACTED
	defer func() { _ = resp.Body.Close() REDACTED()

	if resp.StatusCode >= 400 {
		// 透传模式不做 failover（避免改变原始上游语义），按上游原样返回错误响应。
		return nil, s.handleErrorResponsePassthrough(ctx, resp, c, account, body)
REDACTED

	var usage *OpenAIUsage
	var firstTokenMs *int
	if reqStream {
		result, err := s.handleStreamingResponsePassthrough(ctx, resp, c, account, startTime)
		if err != nil {
			return nil, err
	REDACTED
		usage = result.usage
		firstTokenMs = result.firstTokenMs
REDACTED else {
		usage, err = s.handleNonStreamingResponsePassthrough(ctx, resp, c)
		if err != nil {
			return nil, err
	REDACTED
REDACTED

	if snapshot := ParseCodexRateLimitHeaders(resp.Header); snapshot != nil {
		s.updateCodexUsageSnapshot(ctx, account.ID, snapshot)
REDACTED

	if usage == nil {
		usage = &OpenAIUsage{REDACTED
REDACTED

	return &OpenAIForwardResult{
		RequestID:       resp.Header.Get("x-request-id"),
		Usage:           *usage,
		Model:           reqModel,
		ReasoningEffort: reasoningEffort,
		Stream:          reqStream,
		OpenAIWSMode:    false,
		Duration:        time.Since(startTime),
		FirstTokenMs:    firstTokenMs,
REDACTED, nil
REDACTED

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
REDACTED
	accountID := int64(0)
	accountName := ""
	accountType := ""
	if account != nil {
		accountID = account.ID
		accountName = strings.TrimSpace(account.Name)
		accountType = strings.TrimSpace(string(account.Type))
REDACTED
	fields := []zap.Field{
		zap.String("component", "service.openai_gateway"),
		zap.Int64("account_id", accountID),
		zap.String("account_name", accountName),
		zap.String("account_type", accountType),
		zap.String("request_model", strings.TrimSpace(reqModel)),
		zap.String("reject_reason", strings.TrimSpace(rejectReason)),
REDACTED
	fields = appendCodexCLIOnlyRejectedRequestFields(fields, c, body)
	logger.FromContext(ctx).With(fields...).Warn("OpenAI passthrough 本地拦截：Codex 请求缺少有效 instructions")
REDACTED

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
		REDACTED
			targetURL = buildOpenAIResponsesURL(validatedURL)
	REDACTED
REDACTED

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, targetURL, bytes.NewReader(body))
	if err != nil {
		return nil, err
REDACTED

	// 透传客户端请求头（安全白名单）。
	allowTimeoutHeaders := s.isOpenAIPassthroughTimeoutHeadersAllowed()
	if c != nil && c.Request != nil {
		for key, values := range c.Request.Header {
			lower := strings.ToLower(strings.TrimSpace(key))
			if !isOpenAIPassthroughAllowedRequestHeader(lower, allowTimeoutHeaders) {
				continue
		REDACTED
			for _, v := range values {
				req.Header.Add(key, v)
		REDACTED
	REDACTED
REDACTED

	// 覆盖入站鉴权残留，并注入上游认证
	req.Header.Del("authorization")
	req.Header.Del("x-api-key")
	req.Header.Del("x-goog-api-key")
	req.Header.Set("authorization", "Bearer "+token)

	// OAuth 透传到 ChatGPT internal API 时补齐必要头。
	if account.Type == AccountTypeOAuth {
		promptCacheKey := strings.TrimSpace(gjson.GetBytes(body, "prompt_cache_key").String())
		req.Host = "chatgpt.com"
		if chatgptAccountID := account.GetChatGPTAccountID(); chatgptAccountID != "" {
			req.Header.Set("chatgpt-account-id", chatgptAccountID)
	REDACTED
		if req.Header.Get("accept") == "" {
			req.Header.Set("accept", "text/event-stream")
	REDACTED
		if req.Header.Get("OpenAI-Beta") == "" {
			req.Header.Set("OpenAI-Beta", "responses=experimental")
	REDACTED
		if req.Header.Get("originator") == "" {
			req.Header.Set("originator", "codex_cli_rs")
	REDACTED
		if promptCacheKey != "" {
			if req.Header.Get("conversation_id") == "" {
				req.Header.Set("conversation_id", promptCacheKey)
		REDACTED
			if req.Header.Get("session_id") == "" {
				req.Header.Set("session_id", promptCacheKey)
		REDACTED
	REDACTED
REDACTED

	// 透传模式也支持账户自定义 User-Agent 与 ForceCodexCLI 兜底。
	customUA := account.GetOpenAIUserAgent()
	if customUA != "" {
		req.Header.Set("user-agent", customUA)
REDACTED
	if s.cfg != nil && s.cfg.Gateway.ForceCodexCLI {
		req.Header.Set("user-agent", codexCLIUserAgent)
REDACTED
	// OAuth 安全透传：对非 Codex UA 统一兜底，降低被上游风控拦截概率。
	if account.Type == AccountTypeOAuth && !openai.IsCodexCLIRequest(req.Header.Get("user-agent")) {
		req.Header.Set("user-agent", codexCLIUserAgent)
REDACTED

	if req.Header.Get("content-type") == "" {
		req.Header.Set("content-type", "application/json")
REDACTED

	return req, nil
REDACTED

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
	REDACTED
		upstreamDetail = truncateString(string(body), maxBytes)
REDACTED
	setOpsUpstreamError(c, resp.StatusCode, upstreamMsg, upstreamDetail)
	logOpenAIInstructionsRequiredDebug(ctx, c, account, resp.StatusCode, upstreamMsg, requestBody, body)
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
REDACTED)

	writeOpenAIPassthroughResponseHeaders(c.Writer.Header(), resp.Header, s.responseHeaderFilter)
	contentType := resp.Header.Get("Content-Type")
	if contentType == "" {
		contentType = "application/json"
REDACTED
	c.Data(resp.StatusCode, contentType, body)

	if upstreamMsg == "" {
		return fmt.Errorf("upstream error: %d", resp.StatusCode)
REDACTED
	return fmt.Errorf("upstream error: %d message=%s", resp.StatusCode, upstreamMsg)
REDACTED

func isOpenAIPassthroughAllowedRequestHeader(lowerKey string, allowTimeoutHeaders bool) bool {
	if lowerKey == "" {
		return false
REDACTED
	if isOpenAIPassthroughTimeoutHeader(lowerKey) {
		return allowTimeoutHeaders
REDACTED
	return openaiPassthroughAllowedHeaders[lowerKey]
REDACTED

func isOpenAIPassthroughTimeoutHeader(lowerKey string) bool {
	switch lowerKey {
	case "x-stainless-timeout", "x-stainless-read-timeout", "x-stainless-connect-timeout", "x-request-timeout", "request-timeout", "grpc-timeout":
		return true
	default:
		return false
REDACTED
REDACTED

func (s *OpenAIGatewayService) isOpenAIPassthroughTimeoutHeadersAllowed() bool {
	return s != nil && s.cfg != nil && s.cfg.Gateway.OpenAIPassthroughAllowTimeoutHeaders
REDACTED

func collectOpenAIPassthroughTimeoutHeaders(h http.Header) []string {
	if h == nil {
		return nil
REDACTED
	var matched []string
	for key, values := range h {
		lowerKey := strings.ToLower(strings.TrimSpace(key))
		if isOpenAIPassthroughTimeoutHeader(lowerKey) {
			entry := lowerKey
			if len(values) > 0 {
				entry = fmt.Sprintf("%s=%s", lowerKey, strings.Join(values, "|"))
		REDACTED
			matched = append(matched, entry)
	REDACTED
REDACTED
	sort.Strings(matched)
	return matched
REDACTED

type openaiStreamingResultPassthrough struct {
	usage        *OpenAIUsage
	firstTokenMs *int
REDACTED

func (s *OpenAIGatewayService) handleStreamingResponsePassthrough(
	ctx context.Context,
	resp *http.Response,
	c *gin.Context,
	account *Account,
	startTime time.Time,
) (*openaiStreamingResultPassthrough, error) {
	writeOpenAIPassthroughResponseHeaders(c.Writer.Header(), resp.Header, s.responseHeaderFilter)

	// SSE headers
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("X-Accel-Buffering", "no")
	if v := resp.Header.Get("x-request-id"); v != "" {
		c.Header("x-request-id", v)
REDACTED

	w := c.Writer
	flusher, ok := w.(http.Flusher)
	if !ok {
		return nil, errors.New("streaming not supported")
REDACTED

	usage := &OpenAIUsage{REDACTED
	var firstTokenMs *int
	clientDisconnected := false
	sawDone := false
	upstreamRequestID := strings.TrimSpace(resp.Header.Get("x-request-id"))

	scanner := bufio.NewScanner(resp.Body)
	maxLineSize := defaultMaxLineSize
	if s.cfg != nil && s.cfg.Gateway.MaxLineSize > 0 {
		maxLineSize = s.cfg.Gateway.MaxLineSize
REDACTED
	scanBuf := getSSEScannerBuf64K()
	scanner.Buffer(scanBuf[:0], maxLineSize)
	defer putSSEScannerBuf64K(scanBuf)

	for scanner.Scan() {
		line := scanner.Text()
		if data, ok := extractOpenAISSEDataLine(line); ok {
			dataBytes := []byte(data)
			trimmedData := strings.TrimSpace(data)
			if trimmedData == "[DONE]" {
				sawDone = true
		REDACTED
			if firstTokenMs == nil && trimmedData != "" && trimmedData != "[DONE]" {
				ms := int(time.Since(startTime).Milliseconds())
				firstTokenMs = &ms
		REDACTED
			s.parseSSEUsageBytes(dataBytes, usage)
	REDACTED

		if !clientDisconnected {
			if _, err := fmt.Fprintln(w, line); err != nil {
				clientDisconnected = true
				logger.LegacyPrintf("service.openai_gateway", "[OpenAI passthrough] Client disconnected during streaming, continue draining upstream for usage: account=%d", account.ID)
		REDACTED else {
				flusher.Flush()
		REDACTED
	REDACTED
REDACTED
	if err := scanner.Err(); err != nil {
		if clientDisconnected {
			logger.LegacyPrintf("service.openai_gateway", "[OpenAI passthrough] Upstream read error after client disconnect: account=%d err=%v", account.ID, err)
			return &openaiStreamingResultPassthrough{usage: usage, firstTokenMs: firstTokenMsREDACTED, nil
	REDACTED
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			logger.LegacyPrintf("service.openai_gateway",
				"[OpenAI passthrough] 流读取被取消，可能发生断流: account=%d request_id=%s err=%v ctx_err=%v",
				account.ID,
				upstreamRequestID,
				err,
				ctx.Err(),
			)
			return &openaiStreamingResultPassthrough{usage: usage, firstTokenMs: firstTokenMsREDACTED, nil
	REDACTED
		if errors.Is(err, bufio.ErrTooLong) {
			logger.LegacyPrintf("service.openai_gateway", "[OpenAI passthrough] SSE line too long: account=%d max_size=%d error=%v", account.ID, maxLineSize, err)
			return &openaiStreamingResultPassthrough{usage: usage, firstTokenMs: firstTokenMsREDACTED, err
	REDACTED
		logger.LegacyPrintf("service.openai_gateway",
			"[OpenAI passthrough] 流读取异常中断: account=%d request_id=%s err=%v",
			account.ID,
			upstreamRequestID,
			err,
		)
		return &openaiStreamingResultPassthrough{usage: usage, firstTokenMs: firstTokenMsREDACTED, fmt.Errorf("stream read error: %w", err)
REDACTED
	if !clientDisconnected && !sawDone && ctx.Err() == nil {
		logger.FromContext(ctx).With(
			zap.String("component", "service.openai_gateway"),
			zap.Int64("account_id", account.ID),
			zap.String("upstream_request_id", upstreamRequestID),
		).Info("OpenAI passthrough 上游流在未收到 [DONE] 时结束，疑似断流")
REDACTED

	return &openaiStreamingResultPassthrough{usage: usage, firstTokenMs: firstTokenMsREDACTED, nil
REDACTED

func (s *OpenAIGatewayService) handleNonStreamingResponsePassthrough(
	ctx context.Context,
	resp *http.Response,
	c *gin.Context,
) (*OpenAIUsage, error) {
	maxBytes := resolveUpstreamResponseReadLimit(s.cfg)
	body, err := readUpstreamResponseBodyLimited(resp.Body, maxBytes)
	if err != nil {
		if errors.Is(err, ErrUpstreamResponseBodyTooLarge) {
			setOpsUpstreamError(c, http.StatusBadGateway, "upstream response too large", "")
			c.JSON(http.StatusBadGateway, gin.H{
				"error": gin.H{
					"type":    "upstream_error",
					"message": "Upstream response too large",
			REDACTED,
		REDACTED)
	REDACTED
		return nil, err
REDACTED

	usage := &OpenAIUsage{REDACTED
	usageParsed := false
	if len(body) > 0 {
		if parsedUsage, ok := extractOpenAIUsageFromJSONBytes(body); ok {
			*usage = parsedUsage
			usageParsed = true
	REDACTED
REDACTED
	if !usageParsed {
		// 兜底：尝试从 SSE 文本中解析 usage
		usage = s.parseSSEUsageFromBody(string(body))
REDACTED

	writeOpenAIPassthroughResponseHeaders(c.Writer.Header(), resp.Header, s.responseHeaderFilter)

	contentType := resp.Header.Get("Content-Type")
	if contentType == "" {
		contentType = "application/json"
REDACTED
	c.Data(resp.StatusCode, contentType, body)
	return usage, nil
REDACTED

func writeOpenAIPassthroughResponseHeaders(dst http.Header, src http.Header, filter *responseheaders.CompiledHeaderFilter) {
	if dst == nil || src == nil {
		return
REDACTED
	if filter != nil {
		responseheaders.WriteFilteredHeaders(dst, src, filter)
REDACTED else {
		// 兜底：尽量保留最基础的 content-type
		if v := strings.TrimSpace(src.Get("Content-Type")); v != "" {
			dst.Set("Content-Type", v)
	REDACTED
REDACTED
	// 透传模式强制放行 x-codex-* 响应头（若上游返回）。
	// 注意：真实 http.Response.Header 的 key 一般会被 canonicalize；但为了兼容测试/自建响应，
	// 这里用 EqualFold 做一次大小写不敏感的查找。
	getCaseInsensitiveValues := func(h http.Header, want string) []string {
		if h == nil {
			return nil
	REDACTED
		for k, vals := range h {
			if strings.EqualFold(k, want) {
				return vals
		REDACTED
	REDACTED
		return nil
REDACTED

	for _, rawKey := range []string{
		"x-codex-primary-used-percent",
		"x-codex-primary-reset-after-seconds",
		"x-codex-primary-window-minutes",
		"x-codex-secondary-used-percent",
		"x-codex-secondary-reset-after-seconds",
		"x-codex-secondary-window-minutes",
		"x-codex-primary-over-secondary-limit-percent",
REDACTED {
		vals := getCaseInsensitiveValues(src, rawKey)
		if len(vals) == 0 {
			continue
	REDACTED
		key := http.CanonicalHeaderKey(rawKey)
		dst.Del(key)
		for _, v := range vals {
			dst.Add(key, v)
	REDACTED
REDACTED
REDACTED

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
	REDACTED else {
			validatedURL, err := s.validateUpstreamBaseURL(baseURL)
			if err != nil {
				return nil, err
		REDACTED
			targetURL = buildOpenAIResponsesURL(validatedURL)
	REDACTED
	default:
		targetURL = openaiPlatformAPIURL
REDACTED

	req, err := http.NewRequestWithContext(ctx, "POST", targetURL, bytes.NewReader(body))
	if err != nil {
		return nil, err
REDACTED

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
	REDACTED
REDACTED

	// Whitelist passthrough headers
	for key, values := range c.Request.Header {
		lowerKey := strings.ToLower(key)
		if openaiAllowedHeaders[lowerKey] {
			for _, v := range values {
				req.Header.Add(key, v)
		REDACTED
	REDACTED
REDACTED
	if account.Type == AccountTypeOAuth {
		req.Header.Set("OpenAI-Beta", "responses=experimental")
		if isCodexCLI {
			req.Header.Set("originator", "codex_cli_rs")
	REDACTED else {
			req.Header.Set("originator", "opencode")
	REDACTED
		req.Header.Set("accept", "text/event-stream")
		if promptCacheKey != "" {
			req.Header.Set("conversation_id", promptCacheKey)
			req.Header.Set("session_id", promptCacheKey)
	REDACTED
REDACTED

	// Apply custom User-Agent if configured
	customUA := account.GetOpenAIUserAgent()
	if customUA != "" {
		req.Header.Set("user-agent", customUA)
REDACTED

	// 若开启 ForceCodexCLI，则强制将上游 User-Agent 伪装为 Codex CLI。
	// 用于网关未透传/改写 User-Agent 时，仍能命中 Codex 侧识别逻辑。
	if s.cfg != nil && s.cfg.Gateway.ForceCodexCLI {
		req.Header.Set("user-agent", codexCLIUserAgent)
REDACTED

	// Ensure required headers exist
	if req.Header.Get("content-type") == "" {
		req.Header.Set("content-type", "application/json")
REDACTED

	return req, nil
REDACTED

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
	REDACTED
		upstreamDetail = truncateString(string(body), maxBytes)
REDACTED
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
REDACTED

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
		REDACTED,
	REDACTED)
		if upstreamMsg == "" {
			upstreamMsg = errMsg
	REDACTED
		if upstreamMsg == "" {
			return nil, fmt.Errorf("upstream error: %d (passthrough rule matched)", resp.StatusCode)
	REDACTED
		return nil, fmt.Errorf("upstream error: %d (passthrough rule matched) message=%s", resp.StatusCode, upstreamMsg)
REDACTED

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
	REDACTED)
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": gin.H{
				"type":    "upstream_error",
				"message": "Upstream gateway error",
		REDACTED,
	REDACTED)
		if upstreamMsg == "" {
			return nil, fmt.Errorf("upstream error: %d (not in custom error codes)", resp.StatusCode)
	REDACTED
		return nil, fmt.Errorf("upstream error: %d (not in custom error codes) message=%s", resp.StatusCode, upstreamMsg)
REDACTED

	// Handle upstream error (mark account status)
	shouldDisable := false
	if s.rateLimitService != nil {
		shouldDisable = s.rateLimitService.HandleUpstreamError(ctx, account, resp.StatusCode, resp.Header, body)
REDACTED
	kind := "http_error"
	if shouldDisable {
		kind = "failover"
REDACTED
	appendOpsUpstreamError(c, OpsUpstreamErrorEvent{
		Platform:           account.Platform,
		AccountID:          account.ID,
		AccountName:        account.Name,
		UpstreamStatusCode: resp.StatusCode,
		UpstreamRequestID:  resp.Header.Get("x-request-id"),
		Kind:               kind,
		Message:            upstreamMsg,
		Detail:             upstreamDetail,
REDACTED)
	if shouldDisable {
		return nil, &UpstreamFailoverError{StatusCode: resp.StatusCode, ResponseBody: bodyREDACTED
REDACTED

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
REDACTED

	c.JSON(statusCode, gin.H{
		"error": gin.H{
			"type":    errType,
			"message": errMsg,
	REDACTED,
REDACTED)

	if upstreamMsg == "" {
		return nil, fmt.Errorf("upstream error: %d", resp.StatusCode)
REDACTED
	return nil, fmt.Errorf("upstream error: %d message=%s", resp.StatusCode, upstreamMsg)
REDACTED

// openaiStreamingResult streaming response result
type openaiStreamingResult struct {
	usage        *OpenAIUsage
	firstTokenMs *int
REDACTED

func (s *OpenAIGatewayService) handleStreamingResponse(ctx context.Context, resp *http.Response, c *gin.Context, account *Account, startTime time.Time, originalModel, mappedModel string) (*openaiStreamingResult, error) {
	if s.responseHeaderFilter != nil {
		responseheaders.WriteFilteredHeaders(c.Writer.Header(), resp.Header, s.responseHeaderFilter)
REDACTED

	// Set SSE response headers
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("X-Accel-Buffering", "no")

	// Pass through other headers
	if v := resp.Header.Get("x-request-id"); v != "" {
		c.Header("x-request-id", v)
REDACTED

	w := c.Writer
	flusher, ok := w.(http.Flusher)
	if !ok {
		return nil, errors.New("streaming not supported")
REDACTED
	bufferedWriter := bufio.NewWriterSize(w, 4*1024)
	flushBuffered := func() error {
		if err := bufferedWriter.Flush(); err != nil {
			return err
	REDACTED
		flusher.Flush()
		return nil
REDACTED

	usage := &OpenAIUsage{REDACTED
	var firstTokenMs *int
	scanner := bufio.NewScanner(resp.Body)
	maxLineSize := defaultMaxLineSize
	if s.cfg != nil && s.cfg.Gateway.MaxLineSize > 0 {
		maxLineSize = s.cfg.Gateway.MaxLineSize
REDACTED
	scanBuf := getSSEScannerBuf64K()
	scanner.Buffer(scanBuf[:0], maxLineSize)

	streamInterval := time.Duration(0)
	if s.cfg != nil && s.cfg.Gateway.StreamDataIntervalTimeout > 0 {
		streamInterval = time.Duration(s.cfg.Gateway.StreamDataIntervalTimeout) * time.Second
REDACTED
	// 仅监控上游数据间隔超时，不被下游写入阻塞影响
	var intervalTicker *time.Ticker
	if streamInterval > 0 {
		intervalTicker = time.NewTicker(streamInterval)
		defer intervalTicker.Stop()
REDACTED
	var intervalCh <-chan time.Time
	if intervalTicker != nil {
		intervalCh = intervalTicker.C
REDACTED

	keepaliveInterval := time.Duration(0)
	if s.cfg != nil && s.cfg.Gateway.StreamKeepaliveInterval > 0 {
		keepaliveInterval = time.Duration(s.cfg.Gateway.StreamKeepaliveInterval) * time.Second
REDACTED
	// 下游 keepalive 仅用于防止代理空闲断开
	var keepaliveTicker *time.Ticker
	if keepaliveInterval > 0 {
		keepaliveTicker = time.NewTicker(keepaliveInterval)
		defer keepaliveTicker.Stop()
REDACTED
	var keepaliveCh <-chan time.Time
	if keepaliveTicker != nil {
		keepaliveCh = keepaliveTicker.C
REDACTED
	// 记录上次收到上游数据的时间，用于控制 keepalive 发送频率
	lastDataAt := time.Now()

	// 仅发送一次错误事件，避免多次写入导致协议混乱。
	// 注意：OpenAI `/v1/responses` streaming 事件必须符合 OpenAI Responses schema；
	// 否则下游 SDK（例如 OpenCode）会因为类型校验失败而报错。
	errorEventSent := false
	clientDisconnected := false // 客户端断开后继续 drain 上游以收集 usage
	sendErrorEvent := func(reason string) {
		if errorEventSent || clientDisconnected {
			return
	REDACTED
		errorEventSent = true
		payload := `{"type":"error","sequence_number":0,"error":{"type":"upstream_error","message":` + strconv.Quote(reason) + `,"code":` + strconv.Quote(reason) + `REDACTEDREDACTED`
		if err := flushBuffered(); err != nil {
			clientDisconnected = true
			return
	REDACTED
		if _, err := bufferedWriter.WriteString("data: " + payload + "\n\n"); err != nil {
			clientDisconnected = true
			return
	REDACTED
		if err := flushBuffered(); err != nil {
			clientDisconnected = true
	REDACTED
REDACTED

	needModelReplace := originalModel != mappedModel
	resultWithUsage := func() *openaiStreamingResult {
		return &openaiStreamingResult{usage: usage, firstTokenMs: firstTokenMsREDACTED
REDACTED
	finalizeStream := func() (*openaiStreamingResult, error) {
		if !clientDisconnected {
			if err := flushBuffered(); err != nil {
				clientDisconnected = true
				logger.LegacyPrintf("service.openai_gateway", "Client disconnected during final flush, returning collected usage")
		REDACTED
	REDACTED
		return resultWithUsage(), nil
REDACTED
	handleScanErr := func(scanErr error) (*openaiStreamingResult, error, bool) {
		if scanErr == nil {
			return nil, nil, false
	REDACTED
		// 客户端断开/取消请求时，上游读取往往会返回 context canceled。
		// /v1/responses 的 SSE 事件必须符合 OpenAI 协议；这里不注入自定义 error event，避免下游 SDK 解析失败。
		if errors.Is(scanErr, context.Canceled) || errors.Is(scanErr, context.DeadlineExceeded) {
			logger.LegacyPrintf("service.openai_gateway", "Context canceled during streaming, returning collected usage")
			return resultWithUsage(), nil, true
	REDACTED
		// 客户端已断开时，上游出错仅影响体验，不影响计费；返回已收集 usage
		if clientDisconnected {
			logger.LegacyPrintf("service.openai_gateway", "Upstream read error after client disconnect: %v, returning collected usage", scanErr)
			return resultWithUsage(), nil, true
	REDACTED
		if errors.Is(scanErr, bufio.ErrTooLong) {
			logger.LegacyPrintf("service.openai_gateway", "SSE line too long: account=%d max_size=%d error=%v", account.ID, maxLineSize, scanErr)
			sendErrorEvent("response_too_large")
			return resultWithUsage(), scanErr, true
	REDACTED
		sendErrorEvent("stream_read_error")
		return resultWithUsage(), fmt.Errorf("stream read error: %w", scanErr), true
REDACTED
	processSSELine := func(line string, queueDrained bool) {
		lastDataAt = time.Now()

		// Extract data from SSE line (supports both "data: " and "data:" formats)
		if data, ok := extractOpenAISSEDataLine(line); ok {

			// Replace model in response if needed.
			// Fast path: most events do not contain model field values.
			if needModelReplace && mappedModel != "" && strings.Contains(data, mappedModel) {
				line = s.replaceModelInSSELine(line, mappedModel, originalModel)
		REDACTED

			dataBytes := []byte(data)

			// Correct Codex tool calls if needed (apply_patch -> edit, etc.)
			if correctedData, corrected := s.toolCorrector.CorrectToolCallsInSSEBytes(dataBytes); corrected {
				dataBytes = correctedData
				data = string(correctedData)
				line = "data: " + data
		REDACTED

			// 写入客户端（客户端断开后继续 drain 上游）
			if !clientDisconnected {
				shouldFlush := queueDrained
				if firstTokenMs == nil && data != "" && data != "[DONE]" {
					// 保证首个 token 事件尽快出站，避免影响 TTFT。
					shouldFlush = true
			REDACTED
				if _, err := bufferedWriter.WriteString(line); err != nil {
					clientDisconnected = true
					logger.LegacyPrintf("service.openai_gateway", "Client disconnected during streaming, continuing to drain upstream for billing")
			REDACTED else if _, err := bufferedWriter.WriteString("\n"); err != nil {
					clientDisconnected = true
					logger.LegacyPrintf("service.openai_gateway", "Client disconnected during streaming, continuing to drain upstream for billing")
			REDACTED else if shouldFlush {
					if err := flushBuffered(); err != nil {
						clientDisconnected = true
						logger.LegacyPrintf("service.openai_gateway", "Client disconnected during streaming flush, continuing to drain upstream for billing")
				REDACTED
			REDACTED
		REDACTED

			// Record first token time
			if firstTokenMs == nil && data != "" && data != "[DONE]" {
				ms := int(time.Since(startTime).Milliseconds())
				firstTokenMs = &ms
		REDACTED
			s.parseSSEUsageBytes(dataBytes, usage)
			return
	REDACTED

		// Forward non-data lines as-is
		if !clientDisconnected {
			if _, err := bufferedWriter.WriteString(line); err != nil {
				clientDisconnected = true
				logger.LegacyPrintf("service.openai_gateway", "Client disconnected during streaming, continuing to drain upstream for billing")
		REDACTED else if _, err := bufferedWriter.WriteString("\n"); err != nil {
				clientDisconnected = true
				logger.LegacyPrintf("service.openai_gateway", "Client disconnected during streaming, continuing to drain upstream for billing")
		REDACTED else if queueDrained {
				if err := flushBuffered(); err != nil {
					clientDisconnected = true
					logger.LegacyPrintf("service.openai_gateway", "Client disconnected during streaming flush, continuing to drain upstream for billing")
			REDACTED
		REDACTED
	REDACTED
REDACTED

	// 无超时/无 keepalive 的常见路径走同步扫描，减少 goroutine 与 channel 开销。
	if streamInterval <= 0 && keepaliveInterval <= 0 {
		defer putSSEScannerBuf64K(scanBuf)
		for scanner.Scan() {
			processSSELine(scanner.Text(), true)
	REDACTED
		if result, err, done := handleScanErr(scanner.Err()); done {
			return result, err
	REDACTED
		return finalizeStream()
REDACTED

	type scanEvent struct {
		line string
		err  error
REDACTED
	// 独立 goroutine 读取上游，避免读取阻塞影响 keepalive/超时处理
	events := make(chan scanEvent, 16)
	done := make(chan struct{REDACTED)
	sendEvent := func(ev scanEvent) bool {
		select {
		case events <- ev:
			return true
		case <-done:
			return false
	REDACTED
REDACTED
	var lastReadAt int64
	atomic.StoreInt64(&lastReadAt, time.Now().UnixNano())
	go func(scanBuf *sseScannerBuf64K) {
		defer putSSEScannerBuf64K(scanBuf)
		defer close(events)
		for scanner.Scan() {
			atomic.StoreInt64(&lastReadAt, time.Now().UnixNano())
			if !sendEvent(scanEvent{line: scanner.Text()REDACTED) {
				return
		REDACTED
	REDACTED
		if err := scanner.Err(); err != nil {
			_ = sendEvent(scanEvent{err: errREDACTED)
	REDACTED
REDACTED(scanBuf)
	defer close(done)

	for {
		select {
		case ev, ok := <-events:
			if !ok {
				return finalizeStream()
		REDACTED
			if result, err, done := handleScanErr(ev.err); done {
				return result, err
		REDACTED
			processSSELine(ev.line, len(events) == 0)

		case <-intervalCh:
			lastRead := time.Unix(0, atomic.LoadInt64(&lastReadAt))
			if time.Since(lastRead) < streamInterval {
				continue
		REDACTED
			if clientDisconnected {
				logger.LegacyPrintf("service.openai_gateway", "Upstream timeout after client disconnect, returning collected usage")
				return resultWithUsage(), nil
		REDACTED
			logger.LegacyPrintf("service.openai_gateway", "Stream data interval timeout: account=%d model=%s interval=%s", account.ID, originalModel, streamInterval)
			// 处理流超时，可能标记账户为临时不可调度或错误状态
			if s.rateLimitService != nil {
				s.rateLimitService.HandleStreamTimeout(ctx, account, originalModel)
		REDACTED
			sendErrorEvent("stream_timeout")
			return resultWithUsage(), fmt.Errorf("stream data interval timeout")

		case <-keepaliveCh:
			if clientDisconnected {
				continue
		REDACTED
			if time.Since(lastDataAt) < keepaliveInterval {
				continue
		REDACTED
			if _, err := bufferedWriter.WriteString(":\n\n"); err != nil {
				clientDisconnected = true
				logger.LegacyPrintf("service.openai_gateway", "Client disconnected during streaming, continuing to drain upstream for billing")
				continue
		REDACTED
			if err := flushBuffered(); err != nil {
				clientDisconnected = true
				logger.LegacyPrintf("service.openai_gateway", "Client disconnected during keepalive flush, continuing to drain upstream for billing")
		REDACTED
	REDACTED
REDACTED

REDACTED

// extractOpenAISSEDataLine 低开销提取 SSE `data:` 行内容。
// 兼容 `data: xxx` 与 `data:xxx` 两种格式。
func extractOpenAISSEDataLine(line string) (string, bool) {
	if !strings.HasPrefix(line, "data:") {
		return "", false
REDACTED
	start := len("data:")
	for start < len(line) {
		if line[start] != ' ' && line[start] != '	' {
			break
	REDACTED
		start++
REDACTED
	return line[start:], true
REDACTED

func (s *OpenAIGatewayService) replaceModelInSSELine(line, fromModel, toModel string) string {
	data, ok := extractOpenAISSEDataLine(line)
	if !ok {
		return line
REDACTED
	if data == "" || data == "[DONE]" {
		return line
REDACTED

	// 使用 gjson 精确检查 model 字段，避免全量 JSON 反序列化
	if m := gjson.Get(data, "model"); m.Exists() && m.Str == fromModel {
		newData, err := sjson.Set(data, "model", toModel)
		if err != nil {
			return line
	REDACTED
		return "data: " + newData
REDACTED

	// 检查嵌套的 response.model 字段
	if m := gjson.Get(data, "response.model"); m.Exists() && m.Str == fromModel {
		newData, err := sjson.Set(data, "response.model", toModel)
		if err != nil {
			return line
	REDACTED
		return "data: " + newData
REDACTED

	return line
REDACTED

// correctToolCallsInResponseBody 修正响应体中的工具调用
func (s *OpenAIGatewayService) correctToolCallsInResponseBody(body []byte) []byte {
	if len(body) == 0 {
		return body
REDACTED

	corrected, changed := s.toolCorrector.CorrectToolCallsInSSEBytes(body)
	if changed {
		return corrected
REDACTED
	return body
REDACTED

func (s *OpenAIGatewayService) parseSSEUsage(data string, usage *OpenAIUsage) {
	s.parseSSEUsageBytes([]byte(data), usage)
REDACTED

func (s *OpenAIGatewayService) parseSSEUsageBytes(data []byte, usage *OpenAIUsage) {
	if usage == nil || len(data) == 0 || bytes.Equal(data, []byte("[DONE]")) {
		return
REDACTED
	// 选择性解析：仅在数据中包含 completed 事件标识时才进入字段提取。
	if len(data) < 80 || !bytes.Contains(data, []byte(`"response.completed"`)) {
		return
REDACTED
	if gjson.GetBytes(data, "type").String() != "response.completed" {
		return
REDACTED

	usage.InputTokens = int(gjson.GetBytes(data, "response.usage.input_tokens").Int())
	usage.OutputTokens = int(gjson.GetBytes(data, "response.usage.output_tokens").Int())
	usage.CacheReadInputTokens = int(gjson.GetBytes(data, "response.usage.input_tokens_details.cached_tokens").Int())
REDACTED

func extractOpenAIUsageFromJSONBytes(body []byte) (OpenAIUsage, bool) {
	if len(body) == 0 || !gjson.ValidBytes(body) {
		return OpenAIUsage{REDACTED, false
REDACTED
	values := gjson.GetManyBytes(
		body,
		"usage.input_tokens",
		"usage.output_tokens",
		"usage.input_tokens_details.cached_tokens",
	)
	return OpenAIUsage{
		InputTokens:          int(values[0].Int()),
		OutputTokens:         int(values[1].Int()),
		CacheReadInputTokens: int(values[2].Int()),
REDACTED, true
REDACTED

func (s *OpenAIGatewayService) handleNonStreamingResponse(ctx context.Context, resp *http.Response, c *gin.Context, account *Account, originalModel, mappedModel string) (*OpenAIUsage, error) {
	maxBytes := resolveUpstreamResponseReadLimit(s.cfg)
	body, err := readUpstreamResponseBodyLimited(resp.Body, maxBytes)
	if err != nil {
		if errors.Is(err, ErrUpstreamResponseBodyTooLarge) {
			setOpsUpstreamError(c, http.StatusBadGateway, "upstream response too large", "")
			c.JSON(http.StatusBadGateway, gin.H{
				"error": gin.H{
					"type":    "upstream_error",
					"message": "Upstream response too large",
			REDACTED,
		REDACTED)
	REDACTED
		return nil, err
REDACTED

	if account.Type == AccountTypeOAuth {
		bodyLooksLikeSSE := bytes.Contains(body, []byte("data:")) || bytes.Contains(body, []byte("event:"))
		if isEventStreamResponse(resp.Header) || bodyLooksLikeSSE {
			return s.handleOAuthSSEToJSON(resp, c, body, originalModel, mappedModel)
	REDACTED
REDACTED

	usageValue, usageOK := extractOpenAIUsageFromJSONBytes(body)
	if !usageOK {
		return nil, fmt.Errorf("parse response: invalid json response")
REDACTED
	usage := &usageValue

	// Replace model in response if needed
	if originalModel != mappedModel {
		body = s.replaceModelInResponseBody(body, mappedModel, originalModel)
REDACTED

	responseheaders.WriteFilteredHeaders(c.Writer.Header(), resp.Header, s.responseHeaderFilter)

	contentType := "application/json"
	if s.cfg != nil && !s.cfg.Security.ResponseHeaders.Enabled {
		if upstreamType := resp.Header.Get("Content-Type"); upstreamType != "" {
			contentType = upstreamType
	REDACTED
REDACTED

	c.Data(resp.StatusCode, contentType, body)

	return usage, nil
REDACTED

func isEventStreamResponse(header http.Header) bool {
	contentType := strings.ToLower(header.Get("Content-Type"))
	return strings.Contains(contentType, "text/event-stream")
REDACTED

func (s *OpenAIGatewayService) handleOAuthSSEToJSON(resp *http.Response, c *gin.Context, body []byte, originalModel, mappedModel string) (*OpenAIUsage, error) {
	bodyText := string(body)
	finalResponse, ok := extractCodexFinalResponse(bodyText)

	usage := &OpenAIUsage{REDACTED
	if ok {
		if parsedUsage, parsed := extractOpenAIUsageFromJSONBytes(finalResponse); parsed {
			*usage = parsedUsage
	REDACTED
		body = finalResponse
		if originalModel != mappedModel {
			body = s.replaceModelInResponseBody(body, mappedModel, originalModel)
	REDACTED
		// Correct tool calls in final response
		body = s.correctToolCallsInResponseBody(body)
REDACTED else {
		usage = s.parseSSEUsageFromBody(bodyText)
		if originalModel != mappedModel {
			bodyText = s.replaceModelInSSEBody(bodyText, mappedModel, originalModel)
	REDACTED
		body = []byte(bodyText)
REDACTED

	responseheaders.WriteFilteredHeaders(c.Writer.Header(), resp.Header, s.responseHeaderFilter)

	contentType := "application/json; charset=utf-8"
	if !ok {
		contentType = resp.Header.Get("Content-Type")
		if contentType == "" {
			contentType = "text/event-stream"
	REDACTED
REDACTED
	c.Data(resp.StatusCode, contentType, body)

	return usage, nil
REDACTED

func extractCodexFinalResponse(body string) ([]byte, bool) {
	lines := strings.Split(body, "\n")
	for _, line := range lines {
		data, ok := extractOpenAISSEDataLine(line)
		if !ok {
			continue
	REDACTED
		if data == "" || data == "[DONE]" {
			continue
	REDACTED
		eventType := gjson.Get(data, "type").String()
		if eventType == "response.done" || eventType == "response.completed" {
			if response := gjson.Get(data, "response"); response.Exists() && response.Type == gjson.JSON && response.Raw != "" {
				return []byte(response.Raw), true
		REDACTED
	REDACTED
REDACTED
	return nil, false
REDACTED

func (s *OpenAIGatewayService) parseSSEUsageFromBody(body string) *OpenAIUsage {
	usage := &OpenAIUsage{REDACTED
	lines := strings.Split(body, "\n")
	for _, line := range lines {
		data, ok := extractOpenAISSEDataLine(line)
		if !ok {
			continue
	REDACTED
		if data == "" || data == "[DONE]" {
			continue
	REDACTED
		s.parseSSEUsageBytes([]byte(data), usage)
REDACTED
	return usage
REDACTED

func (s *OpenAIGatewayService) replaceModelInSSEBody(body, fromModel, toModel string) string {
	lines := strings.Split(body, "\n")
	for i, line := range lines {
		if _, ok := extractOpenAISSEDataLine(line); !ok {
			continue
	REDACTED
		lines[i] = s.replaceModelInSSELine(line, fromModel, toModel)
REDACTED
	return strings.Join(lines, "\n")
REDACTED

func (s *OpenAIGatewayService) validateUpstreamBaseURL(raw string) (string, error) {
	if s.cfg != nil && !s.cfg.Security.URLAllowlist.Enabled {
		normalized, err := urlvalidator.ValidateURLFormat(raw, s.cfg.Security.URLAllowlist.AllowInsecureHTTP)
		if err != nil {
			return "", fmt.Errorf("invalid base_url: %w", err)
	REDACTED
		return normalized, nil
REDACTED
	normalized, err := urlvalidator.ValidateHTTPSURL(raw, urlvalidator.ValidationOptions{
		AllowedHosts:     s.cfg.Security.URLAllowlist.UpstreamHosts,
		RequireAllowlist: true,
		AllowPrivate:     s.cfg.Security.URLAllowlist.AllowPrivateHosts,
REDACTED)
	if err != nil {
		return "", fmt.Errorf("invalid base_url: %w", err)
REDACTED
	return normalized, nil
REDACTED

// buildOpenAIResponsesURL 组装 OpenAI Responses 端点。
// - base 以 /v1 结尾：追加 /responses
// - base 已是 /responses：原样返回
// - 其他情况：追加 /v1/responses
func buildOpenAIResponsesURL(base string) string {
	normalized := strings.TrimRight(strings.TrimSpace(base), "/")
	if strings.HasSuffix(normalized, "/responses") {
		return normalized
REDACTED
	if strings.HasSuffix(normalized, "/v1") {
		return normalized + "/responses"
REDACTED
	return normalized + "/v1/responses"
REDACTED

func (s *OpenAIGatewayService) replaceModelInResponseBody(body []byte, fromModel, toModel string) []byte {
	// 使用 gjson/sjson 精确替换 model 字段，避免全量 JSON 反序列化
	if m := gjson.GetBytes(body, "model"); m.Exists() && m.Str == fromModel {
		newBody, err := sjson.SetBytes(body, "model", toModel)
		if err != nil {
			return body
	REDACTED
		return newBody
REDACTED
	return body
REDACTED

// OpenAIRecordUsageInput input for recording usage
type OpenAIRecordUsageInput struct {
	Result        *OpenAIForwardResult
	APIKey        *APIKey
	User          *User
	Account       *Account
	Subscription  *UserSubscription
	UserAgent     string // 请求的 User-Agent
	IPAddress     string // 请求的客户端 IP 地址
	APIKeyService APIKeyQuotaUpdater
REDACTED

// RecordUsage records usage and deducts balance
func (s *OpenAIGatewayService) RecordUsage(ctx context.Context, input *OpenAIRecordUsageInput) error {
	result := input.Result
	apiKey := input.APIKey
	user := input.User
	account := input.Account
	subscription := input.Subscription

	// 计算实际的新输入token（减去缓存读取的token）
	// 因为 input_tokens 包含了 cache_read_tokens，而缓存读取的token不应按输入价格计费
	actualInputTokens := result.Usage.InputTokens - result.Usage.CacheReadInputTokens
	if actualInputTokens < 0 {
		actualInputTokens = 0
REDACTED

	// Calculate cost
	tokens := UsageTokens{
		InputTokens:         actualInputTokens,
		OutputTokens:        result.Usage.OutputTokens,
		CacheCreationTokens: result.Usage.CacheCreationInputTokens,
		CacheReadTokens:     result.Usage.CacheReadInputTokens,
REDACTED

	// Get rate multiplier
	multiplier := s.cfg.Default.RateMultiplier
	if apiKey.GroupID != nil && apiKey.Group != nil {
		multiplier = apiKey.Group.RateMultiplier
REDACTED

	cost, err := s.billingService.CalculateCost(result.Model, tokens, multiplier)
	if err != nil {
		cost = &CostBreakdown{ActualCost: 0REDACTED
REDACTED

	// Determine billing type
	isSubscriptionBilling := subscription != nil && apiKey.Group != nil && apiKey.Group.IsSubscriptionType()
	billingType := BillingTypeBalance
	if isSubscriptionBilling {
		billingType = BillingTypeSubscription
REDACTED

	// Create usage log
	durationMs := int(result.Duration.Milliseconds())
	accountRateMultiplier := account.BillingRateMultiplier()
	usageLog := &UsageLog{
		UserID:                user.ID,
		APIKeyID:              apiKey.ID,
		AccountID:             account.ID,
		RequestID:             result.RequestID,
		Model:                 result.Model,
		ReasoningEffort:       result.ReasoningEffort,
		InputTokens:           actualInputTokens,
		OutputTokens:          result.Usage.OutputTokens,
		CacheCreationTokens:   result.Usage.CacheCreationInputTokens,
		CacheReadTokens:       result.Usage.CacheReadInputTokens,
		InputCost:             cost.InputCost,
		OutputCost:            cost.OutputCost,
		CacheCreationCost:     cost.CacheCreationCost,
		CacheReadCost:         cost.CacheReadCost,
		TotalCost:             cost.TotalCost,
		ActualCost:            cost.ActualCost,
		RateMultiplier:        multiplier,
		AccountRateMultiplier: &accountRateMultiplier,
		BillingType:           billingType,
		Stream:                result.Stream,
		OpenAIWSMode:          result.OpenAIWSMode,
		DurationMs:            &durationMs,
		FirstTokenMs:          result.FirstTokenMs,
		CreatedAt:             time.Now(),
REDACTED

	// 添加 UserAgent
	if input.UserAgent != "" {
		usageLog.UserAgent = &input.UserAgent
REDACTED

	// 添加 IPAddress
	if input.IPAddress != "" {
		usageLog.IPAddress = &input.IPAddress
REDACTED

	if apiKey.GroupID != nil {
		usageLog.GroupID = apiKey.GroupID
REDACTED
	if subscription != nil {
		usageLog.SubscriptionID = &subscription.ID
REDACTED

	inserted, err := s.usageLogRepo.Create(ctx, usageLog)
	if s.cfg != nil && s.cfg.RunMode == config.RunModeSimple {
		logger.LegacyPrintf("service.openai_gateway", "[SIMPLE MODE] Usage recorded (not billed): user=%d, tokens=%d", usageLog.UserID, usageLog.TotalTokens())
		s.deferredService.ScheduleLastUsedUpdate(account.ID)
		return nil
REDACTED

	shouldBill := inserted || err != nil

	if shouldBill {
		postUsageBilling(ctx, &postUsageBillingParams{
			Cost:                  cost,
			User:                  user,
			APIKey:                apiKey,
			Account:               account,
			Subscription:          subscription,
			IsSubscriptionBill:    isSubscriptionBilling,
			AccountRateMultiplier: accountRateMultiplier,
			APIKeyService:         input.APIKeyService,
	REDACTED, s.billingDeps())
REDACTED else {
		s.deferredService.ScheduleLastUsedUpdate(account.ID)
REDACTED

	return nil
REDACTED

// ParseCodexRateLimitHeaders extracts Codex usage limits from response headers.
// Exported for use in ratelimit_service when handling OpenAI 429 responses.
func ParseCodexRateLimitHeaders(headers http.Header) *OpenAICodexUsageSnapshot {
	snapshot := &OpenAICodexUsageSnapshot{REDACTED
	hasData := false

	// Helper to parse float64 from header
	parseFloat := func(key string) *float64 {
		if v := headers.Get(key); v != "" {
			if f, err := strconv.ParseFloat(v, 64); err == nil {
				return &f
		REDACTED
	REDACTED
		return nil
REDACTED

	// Helper to parse int from header
	parseInt := func(key string) *int {
		if v := headers.Get(key); v != "" {
			if i, err := strconv.Atoi(v); err == nil {
				return &i
		REDACTED
	REDACTED
		return nil
REDACTED

	// Primary (weekly) limits
	if v := parseFloat("x-codex-primary-used-percent"); v != nil {
		snapshot.PrimaryUsedPercent = v
		hasData = true
REDACTED
	if v := parseInt("x-codex-primary-reset-after-seconds"); v != nil {
		snapshot.PrimaryResetAfterSeconds = v
		hasData = true
REDACTED
	if v := parseInt("x-codex-primary-window-minutes"); v != nil {
		snapshot.PrimaryWindowMinutes = v
		hasData = true
REDACTED

	// Secondary (5h) limits
	if v := parseFloat("x-codex-secondary-used-percent"); v != nil {
		snapshot.SecondaryUsedPercent = v
		hasData = true
REDACTED
	if v := parseInt("x-codex-secondary-reset-after-seconds"); v != nil {
		snapshot.SecondaryResetAfterSeconds = v
		hasData = true
REDACTED
	if v := parseInt("x-codex-secondary-window-minutes"); v != nil {
		snapshot.SecondaryWindowMinutes = v
		hasData = true
REDACTED

	// Overflow ratio
	if v := parseFloat("x-codex-primary-over-secondary-limit-percent"); v != nil {
		snapshot.PrimaryOverSecondaryPercent = v
		hasData = true
REDACTED

	if !hasData {
		return nil
REDACTED

	snapshot.UpdatedAt = time.Now().Format(time.RFC3339)
	return snapshot
REDACTED

func codexSnapshotBaseTime(snapshot *OpenAICodexUsageSnapshot, fallback time.Time) time.Time {
	if snapshot == nil {
		return fallback
REDACTED
	if snapshot.UpdatedAt == "" {
		return fallback
REDACTED
	base, err := time.Parse(time.RFC3339, snapshot.UpdatedAt)
	if err != nil {
		return fallback
REDACTED
	return base
REDACTED

func codexResetAtRFC3339(base time.Time, resetAfterSeconds *int) *string {
	if resetAfterSeconds == nil {
		return nil
REDACTED
	sec := *resetAfterSeconds
	if sec < 0 {
		sec = 0
REDACTED
	resetAt := base.Add(time.Duration(sec) * time.Second).Format(time.RFC3339)
	return &resetAt
REDACTED

func buildCodexUsageExtraUpdates(snapshot *OpenAICodexUsageSnapshot, fallbackNow time.Time) map[string]any {
	if snapshot == nil {
		return nil
REDACTED

	baseTime := codexSnapshotBaseTime(snapshot, fallbackNow)
	updates := make(map[string]any)

	// 保存原始 primary/secondary 字段，便于排查问题
	if snapshot.PrimaryUsedPercent != nil {
		updates["codex_primary_used_percent"] = *snapshot.PrimaryUsedPercent
REDACTED
	if snapshot.PrimaryResetAfterSeconds != nil {
		updates["codex_primary_reset_after_seconds"] = *snapshot.PrimaryResetAfterSeconds
REDACTED
	if snapshot.PrimaryWindowMinutes != nil {
		updates["codex_primary_window_minutes"] = *snapshot.PrimaryWindowMinutes
REDACTED
	if snapshot.SecondaryUsedPercent != nil {
		updates["codex_secondary_used_percent"] = *snapshot.SecondaryUsedPercent
REDACTED
	if snapshot.SecondaryResetAfterSeconds != nil {
		updates["codex_secondary_reset_after_seconds"] = *snapshot.SecondaryResetAfterSeconds
REDACTED
	if snapshot.SecondaryWindowMinutes != nil {
		updates["codex_secondary_window_minutes"] = *snapshot.SecondaryWindowMinutes
REDACTED
	if snapshot.PrimaryOverSecondaryPercent != nil {
		updates["codex_primary_over_secondary_percent"] = *snapshot.PrimaryOverSecondaryPercent
REDACTED
	updates["codex_usage_updated_at"] = baseTime.Format(time.RFC3339)

	// 归一化到 5h/7d 规范字段
	if normalized := snapshot.Normalize(); normalized != nil {
		if normalized.Used5hPercent != nil {
			updates["codex_5h_used_percent"] = *normalized.Used5hPercent
	REDACTED
		if normalized.Reset5hSeconds != nil {
			updates["codex_5h_reset_after_seconds"] = *normalized.Reset5hSeconds
	REDACTED
		if normalized.Window5hMinutes != nil {
			updates["codex_5h_window_minutes"] = *normalized.Window5hMinutes
	REDACTED
		if normalized.Used7dPercent != nil {
			updates["codex_7d_used_percent"] = *normalized.Used7dPercent
	REDACTED
		if normalized.Reset7dSeconds != nil {
			updates["codex_7d_reset_after_seconds"] = *normalized.Reset7dSeconds
	REDACTED
		if normalized.Window7dMinutes != nil {
			updates["codex_7d_window_minutes"] = *normalized.Window7dMinutes
	REDACTED
		if reset5hAt := codexResetAtRFC3339(baseTime, normalized.Reset5hSeconds); reset5hAt != nil {
			updates["codex_5h_reset_at"] = *reset5hAt
	REDACTED
		if reset7dAt := codexResetAtRFC3339(baseTime, normalized.Reset7dSeconds); reset7dAt != nil {
			updates["codex_7d_reset_at"] = *reset7dAt
	REDACTED
REDACTED

	return updates
REDACTED

// updateCodexUsageSnapshot saves the Codex usage snapshot to account's Extra field
func (s *OpenAIGatewayService) updateCodexUsageSnapshot(ctx context.Context, accountID int64, snapshot *OpenAICodexUsageSnapshot) {
	if snapshot == nil {
		return
REDACTED
	if s == nil || s.accountRepo == nil {
		return
REDACTED

	updates := buildCodexUsageExtraUpdates(snapshot, time.Now())
	if len(updates) == 0 {
		return
REDACTED

	// Update account's Extra field asynchronously
	go func() {
		updateCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = s.accountRepo.UpdateExtra(updateCtx, accountID, updates)
REDACTED()
REDACTED

func getOpenAIReasoningEffortFromReqBody(reqBody map[string]any) (value string, present bool) {
	if reqBody == nil {
		return "", false
REDACTED

	// Primary: reasoning.effort
	if reasoning, ok := reqBody["reasoning"].(map[string]any); ok {
		if effort, ok := reasoning["effort"].(string); ok {
			return normalizeOpenAIReasoningEffort(effort), true
	REDACTED
REDACTED

	// Fallback: some clients may use a flat field.
	if effort, ok := reqBody["reasoning_effort"].(string); ok {
		return normalizeOpenAIReasoningEffort(effort), true
REDACTED

	return "", false
REDACTED

func deriveOpenAIReasoningEffortFromModel(model string) string {
	if strings.TrimSpace(model) == "" {
		return ""
REDACTED

	modelID := strings.TrimSpace(model)
	if strings.Contains(modelID, "/") {
		parts := strings.Split(modelID, "/")
		modelID = parts[len(parts)-1]
REDACTED

	parts := strings.FieldsFunc(strings.ToLower(modelID), func(r rune) bool {
		switch r {
		case '-', '_', ' ':
			return true
		default:
			return false
	REDACTED
REDACTED)
	if len(parts) == 0 {
		return ""
REDACTED

	return normalizeOpenAIReasoningEffort(parts[len(parts)-1])
REDACTED

func extractOpenAIRequestMetaFromBody(body []byte) (model string, stream bool, promptCacheKey string) {
	if len(body) == 0 {
		return "", false, ""
REDACTED

	model = strings.TrimSpace(gjson.GetBytes(body, "model").String())
	stream = gjson.GetBytes(body, "stream").Bool()
	promptCacheKey = strings.TrimSpace(gjson.GetBytes(body, "prompt_cache_key").String())
	return model, stream, promptCacheKey
REDACTED

// normalizeOpenAIPassthroughOAuthBody 将透传 OAuth 请求体收敛为旧链路关键行为：
// 1) store=false 2) stream=true
func normalizeOpenAIPassthroughOAuthBody(body []byte) ([]byte, bool, error) {
	if len(body) == 0 {
		return body, false, nil
REDACTED

	normalized := body
	changed := false

	if store := gjson.GetBytes(normalized, "store"); !store.Exists() || store.Type != gjson.False {
		next, err := sjson.SetBytes(normalized, "store", false)
		if err != nil {
			return body, false, fmt.Errorf("normalize passthrough body store=false: %w", err)
	REDACTED
		normalized = next
		changed = true
REDACTED

	if stream := gjson.GetBytes(normalized, "stream"); !stream.Exists() || stream.Type != gjson.True {
		next, err := sjson.SetBytes(normalized, "stream", true)
		if err != nil {
			return body, false, fmt.Errorf("normalize passthrough body stream=true: %w", err)
	REDACTED
		normalized = next
		changed = true
REDACTED

	return normalized, changed, nil
REDACTED

func detectOpenAIPassthroughInstructionsRejectReason(reqModel string, body []byte) string {
	model := strings.ToLower(strings.TrimSpace(reqModel))
	if !strings.Contains(model, "codex") {
		return ""
REDACTED

	instructions := gjson.GetBytes(body, "instructions")
	if !instructions.Exists() {
		return "instructions_missing"
REDACTED
	if instructions.Type != gjson.String {
		return "instructions_not_string"
REDACTED
	if strings.TrimSpace(instructions.String()) == "" {
		return "instructions_empty"
REDACTED
	return ""
REDACTED

func extractOpenAIReasoningEffortFromBody(body []byte, requestedModel string) *string {
	reasoningEffort := strings.TrimSpace(gjson.GetBytes(body, "reasoning.effort").String())
	if reasoningEffort == "" {
		reasoningEffort = strings.TrimSpace(gjson.GetBytes(body, "reasoning_effort").String())
REDACTED
	if reasoningEffort != "" {
		normalized := normalizeOpenAIReasoningEffort(reasoningEffort)
		if normalized == "" {
			return nil
	REDACTED
		return &normalized
REDACTED

	value := deriveOpenAIReasoningEffortFromModel(requestedModel)
	if value == "" {
		return nil
REDACTED
	return &value
REDACTED

func getOpenAIRequestBodyMap(c *gin.Context, body []byte) (map[string]any, error) {
	if c != nil {
		if cached, ok := c.Get(OpenAIParsedRequestBodyKey); ok {
			if reqBody, ok := cached.(map[string]any); ok && reqBody != nil {
				return reqBody, nil
		REDACTED
	REDACTED
REDACTED

	var reqBody map[string]any
	if err := json.Unmarshal(body, &reqBody); err != nil {
		return nil, fmt.Errorf("parse request: %w", err)
REDACTED
	if c != nil {
		c.Set(OpenAIParsedRequestBodyKey, reqBody)
REDACTED
	return reqBody, nil
REDACTED

func extractOpenAIReasoningEffort(reqBody map[string]any, requestedModel string) *string {
	if value, present := getOpenAIReasoningEffortFromReqBody(reqBody); present {
		if value == "" {
			return nil
	REDACTED
		return &value
REDACTED

	value := deriveOpenAIReasoningEffortFromModel(requestedModel)
	if value == "" {
		return nil
REDACTED
	return &value
REDACTED

func normalizeOpenAIReasoningEffort(raw string) string {
	value := strings.ToLower(strings.TrimSpace(raw))
	if value == "" {
		return ""
REDACTED

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
REDACTED
REDACTED
