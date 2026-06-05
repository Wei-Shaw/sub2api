package handler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"runtime/debug"
	"strconv"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/ctxkey"
	pkghttputil "github.com/Wei-Shaw/sub2api/internal/pkg/httputil"
	"github.com/Wei-Shaw/sub2api/internal/pkg/ip"
	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	middleware2 "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"

	coderws "github.com/coder/websocket"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/tidwall/gjson"
	"go.uber.org/zap"
)

// OpenAIGatewayHandler handles OpenAI API gateway requests
type OpenAIGatewayHandler struct {
	gatewayService           *service.OpenAIGatewayService
	billingCacheService      *service.BillingCacheService
	apiKeyService            *service.APIKeyService
	usageRecordWorkerPool    *service.UsageRecordWorkerPool
	errorPassthroughService  *service.ErrorPassthroughService
	contentModerationService *service.ContentModerationService
	concurrencyHelper        *ConcurrencyHelper
	imageLimiter             *imageConcurrencyLimiter
	maxAccountSwitches       int
	cfg                      *config.Config
REDACTED

func resolveOpenAIMessagesDispatchMappedModel(apiKey *service.APIKey, requestedModel string) string {
	if apiKey == nil || apiKey.Group == nil {
		return ""
REDACTED
	return strings.TrimSpace(apiKey.Group.ResolveMessagesDispatchModel(requestedModel))
REDACTED

func usageRecordContext(parent context.Context, base context.Context) context.Context {
	if base == nil {
		base = context.Background()
REDACTED
	if parent == nil {
		return base
REDACTED
	if clientRequestID, _ := parent.Value(ctxkey.ClientRequestID).(string); strings.TrimSpace(clientRequestID) != "" {
		base = context.WithValue(base, ctxkey.ClientRequestID, strings.TrimSpace(clientRequestID))
REDACTED
	if requestID, _ := parent.Value(ctxkey.RequestID).(string); strings.TrimSpace(requestID) != "" {
		base = context.WithValue(base, ctxkey.RequestID, strings.TrimSpace(requestID))
REDACTED
	return base
REDACTED

func wrapUsageRecordTaskContext(parent context.Context, task service.UsageRecordTask) service.UsageRecordTask {
	if task == nil {
		return nil
REDACTED
	return func(ctx context.Context) {
		task(usageRecordContext(parent, ctx))
REDACTED
REDACTED

// NewOpenAIGatewayHandler creates a new OpenAIGatewayHandler
func NewOpenAIGatewayHandler(
	gatewayService *service.OpenAIGatewayService,
	concurrencyService *service.ConcurrencyService,
	billingCacheService *service.BillingCacheService,
	apiKeyService *service.APIKeyService,
	usageRecordWorkerPool *service.UsageRecordWorkerPool,
	errorPassthroughService *service.ErrorPassthroughService,
	contentModerationService *service.ContentModerationService,
	cfg *config.Config,
) *OpenAIGatewayHandler {
	pingInterval := time.Duration(0)
	maxAccountSwitches := 3
	if cfg != nil {
		pingInterval = time.Duration(cfg.Concurrency.PingInterval) * time.Second
		if cfg.Gateway.MaxAccountSwitches > 0 {
			maxAccountSwitches = cfg.Gateway.MaxAccountSwitches
	REDACTED
REDACTED
	return &OpenAIGatewayHandler{
		gatewayService:           gatewayService,
		billingCacheService:      billingCacheService,
		apiKeyService:            apiKeyService,
		usageRecordWorkerPool:    usageRecordWorkerPool,
		errorPassthroughService:  errorPassthroughService,
		contentModerationService: contentModerationService,
		concurrencyHelper:        NewConcurrencyHelper(concurrencyService, SSEPingFormatComment, pingInterval),
		imageLimiter:             &imageConcurrencyLimiter{REDACTED,
		maxAccountSwitches:       maxAccountSwitches,
		cfg:                      cfg,
REDACTED
REDACTED

// Responses handles OpenAI Responses API endpoint
// POST /openai/v1/responses
func (h *OpenAIGatewayHandler) Responses(c *gin.Context) {
	// 局部兜底：确保该 handler 内部任何 panic 都不会击穿到进程级。
	streamStarted := false
	defer h.recoverResponsesPanic(c, &streamStarted)
	compactStartedAt := time.Now()
	defer h.logOpenAIRemoteCompactOutcome(c, compactStartedAt)
	setOpenAIClientTransportHTTP(c)

	requestStart := time.Now()

	// Get apiKey and user from context (set by ApiKeyAuth middleware)
	apiKey, ok := middleware2.GetAPIKeyFromContext(c)
	if !ok {
		h.errorResponse(c, http.StatusUnauthorized, "authentication_error", "Invalid API key")
		return
REDACTED

	subject, ok := middleware2.GetAuthSubjectFromContext(c)
	if !ok {
		h.errorResponse(c, http.StatusInternalServerError, "api_error", "User context not found")
		return
REDACTED
	reqLog := requestLogger(
		c,
		"handler.openai_gateway.responses",
		zap.Int64("user_id", subject.UserID),
		zap.Int64("api_key_id", apiKey.ID),
		zap.Any("group_id", apiKey.GroupID),
	)
	if !h.ensureResponsesDependencies(c, reqLog) {
		return
REDACTED

	// Read request body
	body, err := pkghttputil.ReadRequestBodyWithPrealloc(c.Request)
	if err != nil {
		if maxErr, ok := extractMaxBytesError(err); ok {
			h.errorResponse(c, http.StatusRequestEntityTooLarge, "invalid_request_error", buildBodyTooLargeMessage(maxErr.Limit))
			return
	REDACTED
		h.errorResponse(c, http.StatusBadRequest, "invalid_request_error", "Failed to read request body")
		return
REDACTED

	if len(body) == 0 {
		h.errorResponse(c, http.StatusBadRequest, "invalid_request_error", "Request body is empty")
		return
REDACTED

	setOpsRequestContext(c, "", false)
	sessionHashBody := body
	if service.IsOpenAIResponsesCompactPathForTest(c) {
		if compactSeed := strings.TrimSpace(gjson.GetBytes(body, "prompt_cache_key").String()); compactSeed != "" {
			c.Set(service.OpenAICompactSessionSeedKeyForTest(), compactSeed)
	REDACTED
		normalizedCompactBody, normalizedCompact, compactErr := service.NormalizeOpenAICompactRequestBodyForTest(body)
		if compactErr != nil {
			h.errorResponse(c, http.StatusBadRequest, "invalid_request_error", "Failed to normalize compact request body")
			return
	REDACTED
		if normalizedCompact {
			body = normalizedCompactBody
	REDACTED
REDACTED

	// 校验请求体 JSON 合法性
	if !gjson.ValidBytes(body) {
		h.errorResponse(c, http.StatusBadRequest, "invalid_request_error", "Failed to parse request body")
		return
REDACTED

	// 使用 gjson 只读提取字段做校验，避免完整 Unmarshal
	modelResult := gjson.GetBytes(body, "model")
	if !modelResult.Exists() || modelResult.Type != gjson.String || modelResult.String() == "" {
		h.errorResponse(c, http.StatusBadRequest, "invalid_request_error", "model is required")
		return
REDACTED
	reqModel := modelResult.String()

	streamResult := gjson.GetBytes(body, "stream")
	if streamResult.Exists() && streamResult.Type != gjson.True && streamResult.Type != gjson.False {
		h.errorResponse(c, http.StatusBadRequest, "invalid_request_error", "invalid stream field type")
		return
REDACTED
	reqStream := streamResult.Bool()
	reqLog = reqLog.With(zap.String("model", reqModel), zap.Bool("stream", reqStream))
	previousResponseID := strings.TrimSpace(gjson.GetBytes(body, "previous_response_id").String())
	if previousResponseID != "" {
		previousResponseIDKind := service.ClassifyOpenAIPreviousResponseIDKind(previousResponseID)
		reqLog = reqLog.With(
			zap.Bool("has_previous_response_id", true),
			zap.String("previous_response_id_kind", previousResponseIDKind),
			zap.Int("previous_response_id_len", len(previousResponseID)),
		)
		if previousResponseIDKind == service.OpenAIPreviousResponseIDKindMessageID {
			reqLog.Warn("openai.request_validation_failed",
				zap.String("reason", "previous_response_id_looks_like_message_id"),
			)
			h.errorResponse(c, http.StatusBadRequest, "invalid_request_error", "previous_response_id must be a response.id (resp_*), not a message id")
			return
	REDACTED
		reqLog.Warn("openai.request_validation_failed",
			zap.String("reason", "previous_response_id_requires_wsv2"),
		)
		h.errorResponse(c, http.StatusBadRequest, "invalid_request_error", "previous_response_id is only supported on Responses WebSocket v2")
		return
REDACTED

	setOpsRequestContext(c, reqModel, reqStream)
	setOpsEndpointContext(c, "", int16(service.RequestTypeFromLegacy(reqStream, false)))

	if decision := h.checkContentModeration(c, reqLog, apiKey, subject, service.ContentModerationProtocolOpenAIResponses, reqModel, body); decision != nil && decision.Blocked {
		h.errorResponse(c, contentModerationStatus(decision), contentModerationErrorCode(decision), decision.Message)
		return
REDACTED

	imageIntent := service.IsImageGenerationIntent("/v1/responses", reqModel, body)
	if imageIntent && !service.GroupAllowsImageGeneration(apiKey.Group) {
		h.errorResponse(c, http.StatusForbidden, "permission_error", service.ImageGenerationPermissionMessage())
		return
REDACTED
	var imageReleaseFunc func()
	if imageIntent {
		var imageAcquired bool
		imageReleaseFunc, imageAcquired = h.acquireImageGenerationSlot(c, streamStarted)
		if !imageAcquired {
			return
	REDACTED
		if imageReleaseFunc != nil {
			defer imageReleaseFunc()
	REDACTED
REDACTED

	// 解析渠道级模型映射
	channelMapping, _ := h.gatewayService.ResolveChannelMappingAndRestrict(c.Request.Context(), apiKey.GroupID, reqModel)

	// 提前校验 function_call_output 是否具备可关联上下文，避免上游 400。
	if !h.validateFunctionCallOutputRequest(c, body, reqLog) {
		return
REDACTED

	// 绑定错误透传服务，允许 service 层在非 failover 错误场景复用规则。
	if h.errorPassthroughService != nil {
		service.BindErrorPassthroughService(c, h.errorPassthroughService)
REDACTED

	// Get subscription info (may be nil)
	subscription, _ := middleware2.GetSubscriptionFromContext(c)

	service.SetOpsLatencyMs(c, service.OpsAuthLatencyMsKey, time.Since(requestStart).Milliseconds())
	routingStart := time.Now()

	userReleaseFunc, acquired := h.acquireResponsesUserSlot(c, subject.UserID, subject.Concurrency, reqStream, &streamStarted, reqLog)
	if !acquired {
		return
REDACTED
	// 确保请求取消时也会释放槽位，避免长连接被动中断造成泄漏
	if userReleaseFunc != nil {
		defer userReleaseFunc()
REDACTED

	// 2. Re-check billing eligibility after wait
	if err := h.billingCacheService.CheckBillingEligibility(c.Request.Context(), apiKey.User, apiKey, apiKey.Group, subscription, service.QuotaPlatform(c.Request.Context(), apiKey)); err != nil {
		reqLog.Info("openai.billing_eligibility_check_failed", zap.Error(err))
		status, code, message, retryAfter := billingErrorDetails(err)
		if retryAfter > 0 {
			c.Header("Retry-After", strconv.Itoa(retryAfter))
	REDACTED
		h.handleStreamingAwareError(c, status, code, message, streamStarted)
		return
REDACTED

	// Generate session hash (header first; fallback to prompt_cache_key)
	sessionHash := h.gatewayService.GenerateSessionHash(c, sessionHashBody)
	requireCompact := isOpenAIRemoteCompactPath(c)

	maxAccountSwitches := h.maxAccountSwitches
	switchCount := 0
	failedAccountIDs := make(map[int64]struct{REDACTED)
	sameAccountRetryCount := make(map[int64]int)
	var lastFailoverErr *service.UpstreamFailoverError

	for {
		// Select account supporting the requested model
		reqLog.Debug("openai.account_selecting", zap.Int("excluded_account_count", len(failedAccountIDs)))
		selection, scheduleDecision, err := h.gatewayService.SelectAccountWithSchedulerForCapability(
			c.Request.Context(),
			apiKey.GroupID,
			previousResponseID,
			sessionHash,
			reqModel,
			failedAccountIDs,
			service.OpenAIUpstreamTransportAny,
			service.OpenAIEndpointCapabilityChatCompletions,
			requireCompact,
		)
		if err != nil {
			reqLog.Warn("openai.account_select_failed",
				zap.Error(err),
				zap.Int("excluded_account_count", len(failedAccountIDs)),
			)
			if len(failedAccountIDs) == 0 {
				markOpsRoutingCapacityLimitedIfNoAvailable(c, err)
				if errors.Is(err, service.ErrNoAvailableCompactAccounts) {
					h.handleStreamingAwareError(c, http.StatusServiceUnavailable, "compact_not_supported", "No available OpenAI accounts support /responses/compact", streamStarted)
					return
			REDACTED
				h.handleStreamingAwareError(c, http.StatusServiceUnavailable, "api_error", "Service temporarily unavailable", streamStarted)
				return
		REDACTED
			if lastFailoverErr != nil {
				h.handleFailoverExhausted(c, lastFailoverErr, streamStarted)
		REDACTED else {
				h.handleFailoverExhaustedSimple(c, 502, streamStarted)
		REDACTED
			return
	REDACTED
		if selection == nil || selection.Account == nil {
			markOpsRoutingCapacityLimited(c)
			h.handleStreamingAwareError(c, http.StatusServiceUnavailable, "api_error", "No available accounts", streamStarted)
			return
	REDACTED
		if previousResponseID != "" && selection != nil && selection.Account != nil {
			reqLog.Debug("openai.account_selected_with_previous_response_id", zap.Int64("account_id", selection.Account.ID))
	REDACTED
		reqLog.Debug("openai.account_schedule_decision",
			zap.String("layer", scheduleDecision.Layer),
			zap.Bool("sticky_previous_hit", scheduleDecision.StickyPreviousHit),
			zap.Bool("sticky_session_hit", scheduleDecision.StickySessionHit),
			zap.Int("candidate_count", scheduleDecision.CandidateCount),
			zap.Int("top_k", scheduleDecision.TopK),
			zap.Int64("latency_ms", scheduleDecision.LatencyMs),
			zap.Float64("load_skew", scheduleDecision.LoadSkew),
		)
		account := selection.Account
		sessionHash = ensureOpenAIPoolModeSessionHash(sessionHash, account)
		reqLog.Debug("openai.account_selected", zap.Int64("account_id", account.ID), zap.String("account_name", account.Name))
		setOpsSelectedAccount(c, account.ID, account.Platform)

		accountReleaseFunc, acquired := h.acquireResponsesAccountSlot(c, apiKey.GroupID, sessionHash, selection, reqStream, &streamStarted, reqLog)
		if !acquired {
			return
	REDACTED

		// Forward request
		service.SetOpsLatencyMs(c, service.OpsRoutingLatencyMsKey, time.Since(routingStart).Milliseconds())
		forwardStart := time.Now()
		// 应用渠道模型映射到请求体
		forwardBody := body
		if channelMapping.Mapped {
			forwardBody = h.gatewayService.ReplaceModelInBody(body, channelMapping.MappedModel)
	REDACTED
		writerSizeBeforeForward := c.Writer.Size()
		result, err := func() (*service.OpenAIForwardResult, error) {
			defer func() {
				if accountReleaseFunc != nil {
					accountReleaseFunc()
			REDACTED
		REDACTED()
			return h.gatewayService.Forward(c.Request.Context(), c, account, forwardBody)
	REDACTED()
		forwardDurationMs := time.Since(forwardStart).Milliseconds()
		upstreamLatencyMs, _ := getContextInt64(c, service.OpsUpstreamLatencyMsKey)
		responseLatencyMs := forwardDurationMs
		if upstreamLatencyMs > 0 && forwardDurationMs > upstreamLatencyMs {
			responseLatencyMs = forwardDurationMs - upstreamLatencyMs
	REDACTED
		service.SetOpsLatencyMs(c, service.OpsResponseLatencyMsKey, responseLatencyMs)
		if err == nil && result != nil && result.FirstTokenMs != nil {
			service.SetOpsLatencyMs(c, service.OpsTimeToFirstTokenMsKey, int64(*result.FirstTokenMs))
	REDACTED
		if err != nil {
			if result != nil && result.ImageCount > 0 {
				reqLog.Warn("openai.forward_partial_error_with_image_result",
					zap.Int64("account_id", account.ID),
					zap.Int("image_count", result.ImageCount),
					zap.Error(err),
				)
		REDACTED else {
				var failoverErr *service.UpstreamFailoverError
				if errors.As(err, &failoverErr) {
					if c.Writer.Size() != writerSizeBeforeForward {
						h.handleFailoverExhausted(c, failoverErr, true)
						return
				REDACTED
					h.gatewayService.ReportOpenAIAccountScheduleResult(account.ID, false, nil)
					// 池模式：同账号重试
					if failoverErr.RetryableOnSameAccount {
						retryLimit := account.GetPoolModeRetryCount()
						if sameAccountRetryCount[account.ID] < retryLimit {
							sameAccountRetryCount[account.ID]++
							reqLog.Warn("openai.pool_mode_same_account_retry",
								zap.Int64("account_id", account.ID),
								zap.Int("upstream_status", failoverErr.StatusCode),
								zap.Int("retry_limit", retryLimit),
								zap.Int("retry_count", sameAccountRetryCount[account.ID]),
							)
							select {
							case <-c.Request.Context().Done():
								return
							case <-time.After(sameAccountRetryDelay):
						REDACTED
							continue
					REDACTED
				REDACTED
					h.gatewayService.RecordOpenAIAccountSwitch()
					failedAccountIDs[account.ID] = struct{REDACTED{REDACTED
					lastFailoverErr = failoverErr
					if switchCount >= maxAccountSwitches {
						h.handleFailoverExhausted(c, failoverErr, streamStarted)
						return
				REDACTED
					switchCount++
					if h.gatewayService.ShouldStopOpenAIOAuth429Failover(account, failoverErr.StatusCode, switchCount) {
						h.handleFailoverExhausted(c, failoverErr, streamStarted)
						return
				REDACTED
					reqLog.Warn("openai.upstream_failover_switching",
						zap.Int64("account_id", account.ID),
						zap.Int("upstream_status", failoverErr.StatusCode),
						zap.Int("switch_count", switchCount),
						zap.Int("max_switches", maxAccountSwitches),
					)
					continue
			REDACTED
				h.gatewayService.ReportOpenAIAccountScheduleResult(account.ID, false, nil)
				wroteFallback := h.ensureForwardErrorResponse(c, streamStarted)
				fields := []zap.Field{
					zap.Int64("account_id", account.ID),
					zap.Bool("fallback_error_response_written", wroteFallback),
					zap.Error(err),
			REDACTED
				if shouldLogOpenAIForwardFailureAsWarn(c, wroteFallback) {
					reqLog.Warn("openai.forward_failed", fields...)
					return
			REDACTED
				reqLog.Error("openai.forward_failed", fields...)
				return
		REDACTED
	REDACTED
		if result != nil {
			if account.Type == service.AccountTypeOAuth {
				h.gatewayService.UpdateCodexUsageSnapshotFromHeaders(c.Request.Context(), account.ID, result.ResponseHeaders)
		REDACTED
			h.gatewayService.ReportOpenAIAccountScheduleResult(account.ID, true, result.FirstTokenMs)
	REDACTED else {
			h.gatewayService.ReportOpenAIAccountScheduleResult(account.ID, true, nil)
	REDACTED

		// 捕获请求信息（用于异步记录，避免在 goroutine 中访问 gin.Context）
		userAgent := c.GetHeader("User-Agent")
		clientIP := ip.GetClientIP(c)
		requestPayloadHash := service.HashUsageRequestPayload(body)
		inboundEndpoint := GetInboundEndpoint(c)
		upstreamEndpoint := GetUpstreamEndpoint(c, account.Platform)

		// 使用量记录通过有界 worker 池提交，避免请求热路径创建无界 goroutine。
		h.submitOpenAIUsageRecordTask(c.Request.Context(), result, func(ctx context.Context) {
			if err := h.gatewayService.RecordUsage(ctx, &service.OpenAIRecordUsageInput{
				Result:             result,
				APIKey:             apiKey,
				User:               apiKey.User,
				Account:            account,
				Subscription:       subscription,
				InboundEndpoint:    inboundEndpoint,
				UpstreamEndpoint:   upstreamEndpoint,
				UserAgent:          userAgent,
				IPAddress:          clientIP,
				RequestPayloadHash: requestPayloadHash,
				APIKeyService:      h.apiKeyService,
				ChannelUsageFields: channelMapping.ToUsageFields(reqModel, result.UpstreamModel),
		REDACTED); err != nil {
				logger.L().With(
					zap.String("component", "handler.openai_gateway.responses"),
					zap.Int64("user_id", subject.UserID),
					zap.Int64("api_key_id", apiKey.ID),
					zap.Any("group_id", apiKey.GroupID),
					zap.String("model", reqModel),
					zap.Int64("account_id", account.ID),
				).Error("openai.record_usage_failed", zap.Error(err))
		REDACTED
	REDACTED)
		reqLog.Debug("openai.request_completed",
			zap.Int64("account_id", account.ID),
			zap.Int("switch_count", switchCount),
		)
		return
REDACTED
REDACTED

func isOpenAIRemoteCompactPath(c *gin.Context) bool {
	if c == nil || c.Request == nil || c.Request.URL == nil {
		return false
REDACTED
	normalizedPath := strings.TrimRight(strings.TrimSpace(c.Request.URL.Path), "/")
	return strings.HasSuffix(normalizedPath, "/responses/compact")
REDACTED

func (h *OpenAIGatewayHandler) logOpenAIRemoteCompactOutcome(c *gin.Context, startedAt time.Time) {
	if !isOpenAIRemoteCompactPath(c) {
		return
REDACTED

	var (
		ctx    = context.Background()
		path   string
		status int
	)
	if c != nil {
		if c.Request != nil {
			ctx = c.Request.Context()
			if c.Request.URL != nil {
				path = strings.TrimSpace(c.Request.URL.Path)
		REDACTED
	REDACTED
		if c.Writer != nil {
			status = c.Writer.Status()
	REDACTED
REDACTED

	outcome := "failed"
	if status >= 200 && status < 300 {
		outcome = "succeeded"
REDACTED
	latencyMs := time.Since(startedAt).Milliseconds()
	if latencyMs < 0 {
		latencyMs = 0
REDACTED

	fields := []zap.Field{
		zap.String("component", "handler.openai_gateway.responses"),
		zap.Bool("remote_compact", true),
		zap.String("compact_outcome", outcome),
		zap.Int("status_code", status),
		zap.Int64("latency_ms", latencyMs),
		zap.String("path", path),
		zap.Bool("force_codex_cli", h != nil && h.cfg != nil && h.cfg.Gateway.ForceCodexCLI),
REDACTED

	if c != nil {
		if userAgent := strings.TrimSpace(c.GetHeader("User-Agent")); userAgent != "" {
			fields = append(fields, zap.String("request_user_agent", userAgent))
	REDACTED
		if v, ok := c.Get(opsModelKey); ok {
			if model, ok := v.(string); ok && strings.TrimSpace(model) != "" {
				fields = append(fields, zap.String("request_model", strings.TrimSpace(model)))
		REDACTED
	REDACTED
		if v, ok := c.Get(opsAccountIDKey); ok {
			if accountID, ok := v.(int64); ok && accountID > 0 {
				fields = append(fields, zap.Int64("account_id", accountID))
		REDACTED
	REDACTED
		if c.Writer != nil {
			if upstreamRequestID := strings.TrimSpace(c.Writer.Header().Get("x-request-id")); upstreamRequestID != "" {
				fields = append(fields, zap.String("upstream_request_id", upstreamRequestID))
		REDACTED else if upstreamRequestID := strings.TrimSpace(c.Writer.Header().Get("X-Request-Id")); upstreamRequestID != "" {
				fields = append(fields, zap.String("upstream_request_id", upstreamRequestID))
		REDACTED
	REDACTED
REDACTED

	log := logger.FromContext(ctx).With(fields...)
	if outcome == "succeeded" {
		log.Info("codex.remote_compact.succeeded")
		return
REDACTED
	log.Warn("codex.remote_compact.failed")
REDACTED

// Messages handles Anthropic Messages API requests routed to OpenAI platform.
// POST /v1/messages (when group platform is OpenAI)
func (h *OpenAIGatewayHandler) Messages(c *gin.Context) {
	streamStarted := false
	defer h.recoverAnthropicMessagesPanic(c, &streamStarted)

	requestStart := time.Now()

	apiKey, ok := middleware2.GetAPIKeyFromContext(c)
	if !ok {
		h.anthropicErrorResponse(c, http.StatusUnauthorized, "authentication_error", "Invalid API key")
		return
REDACTED

	subject, ok := middleware2.GetAuthSubjectFromContext(c)
	if !ok {
		h.anthropicErrorResponse(c, http.StatusInternalServerError, "api_error", "User context not found")
		return
REDACTED
	reqLog := requestLogger(
		c,
		"handler.openai_gateway.messages",
		zap.Int64("user_id", subject.UserID),
		zap.Int64("api_key_id", apiKey.ID),
		zap.Any("group_id", apiKey.GroupID),
	)

	// 检查分组是否允许 /v1/messages 调度
	if apiKey.Group != nil && !apiKey.Group.AllowMessagesDispatch {
		h.anthropicErrorResponse(c, http.StatusForbidden, "permission_error",
			"This group does not allow /v1/messages dispatch")
		return
REDACTED

	if !h.ensureResponsesDependencies(c, reqLog) {
		return
REDACTED

	body, err := pkghttputil.ReadRequestBodyWithPrealloc(c.Request)
	if err != nil {
		if maxErr, ok := extractMaxBytesError(err); ok {
			h.anthropicErrorResponse(c, http.StatusRequestEntityTooLarge, "invalid_request_error", buildBodyTooLargeMessage(maxErr.Limit))
			return
	REDACTED
		h.anthropicErrorResponse(c, http.StatusBadRequest, "invalid_request_error", "Failed to read request body")
		return
REDACTED
	if len(body) == 0 {
		h.anthropicErrorResponse(c, http.StatusBadRequest, "invalid_request_error", "Request body is empty")
		return
REDACTED

	if !gjson.ValidBytes(body) {
		h.anthropicErrorResponse(c, http.StatusBadRequest, "invalid_request_error", "Failed to parse request body")
		return
REDACTED

	modelResult := gjson.GetBytes(body, "model")
	if !modelResult.Exists() || modelResult.Type != gjson.String || modelResult.String() == "" {
		h.anthropicErrorResponse(c, http.StatusBadRequest, "invalid_request_error", "model is required")
		return
REDACTED
	reqModel := modelResult.String()
	routingModel := service.NormalizeOpenAICompatRequestedModel(reqModel)
	preferredMappedModel := resolveOpenAIMessagesDispatchMappedModel(apiKey, reqModel)
	reqStream := gjson.GetBytes(body, "stream").Bool()

	reqLog = reqLog.With(zap.String("model", reqModel), zap.Bool("stream", reqStream))

	setOpsRequestContext(c, reqModel, reqStream)
	setOpsEndpointContext(c, "", int16(service.RequestTypeFromLegacy(reqStream, false)))

	if decision := h.checkContentModeration(c, reqLog, apiKey, subject, service.ContentModerationProtocolAnthropicMessages, reqModel, body); decision != nil && decision.Blocked {
		h.anthropicErrorResponse(c, contentModerationStatus(decision), contentModerationErrorCode(decision), decision.Message)
		return
REDACTED

	// 解析渠道级模型映射
	channelMappingMsg, _ := h.gatewayService.ResolveChannelMappingAndRestrict(c.Request.Context(), apiKey.GroupID, reqModel)

	// 绑定错误透传服务，允许 service 层在非 failover 错误场景复用规则。
	if h.errorPassthroughService != nil {
		service.BindErrorPassthroughService(c, h.errorPassthroughService)
REDACTED

	subscription, _ := middleware2.GetSubscriptionFromContext(c)

	service.SetOpsLatencyMs(c, service.OpsAuthLatencyMsKey, time.Since(requestStart).Milliseconds())
	routingStart := time.Now()

	userReleaseFunc, acquired := h.acquireResponsesUserSlot(c, subject.UserID, subject.Concurrency, reqStream, &streamStarted, reqLog)
	if !acquired {
		return
REDACTED
	if userReleaseFunc != nil {
		defer userReleaseFunc()
REDACTED

	if err := h.billingCacheService.CheckBillingEligibility(c.Request.Context(), apiKey.User, apiKey, apiKey.Group, subscription, service.QuotaPlatform(c.Request.Context(), apiKey)); err != nil {
		reqLog.Info("openai_messages.billing_eligibility_check_failed", zap.Error(err))
		status, code, message, retryAfter := billingErrorDetails(err)
		if retryAfter > 0 {
			c.Header("Retry-After", strconv.Itoa(retryAfter))
	REDACTED
		h.anthropicStreamingAwareError(c, status, code, message, streamStarted)
		return
REDACTED

	sessionHash := h.gatewayService.GenerateSessionHash(c, body)
	promptCacheKey := h.gatewayService.ExtractSessionID(c, body)
	sessionHash, promptCacheKey = resolveOpenAIMessagesMetadataSession(sessionHash, promptCacheKey, reqModel, body)

	maxAccountSwitches := h.maxAccountSwitches
	switchCount := 0
	failedAccountIDs := make(map[int64]struct{REDACTED)
	sameAccountRetryCount := make(map[int64]int)
	var lastFailoverErr *service.UpstreamFailoverError
	effectiveMappedModel := preferredMappedModel

	for {
		currentRoutingModel := routingModel
		if effectiveMappedModel != "" {
			currentRoutingModel = effectiveMappedModel
	REDACTED
		reqLog.Debug("openai_messages.account_selecting", zap.Int("excluded_account_count", len(failedAccountIDs)))
		selection, scheduleDecision, err := h.gatewayService.SelectAccountWithSchedulerForCapability(
			c.Request.Context(),
			apiKey.GroupID,
			"", // no previous_response_id
			sessionHash,
			currentRoutingModel,
			failedAccountIDs,
			service.OpenAIUpstreamTransportAny,
			service.OpenAIEndpointCapabilityChatCompletions,
			false,
		)
		if err != nil {
			reqLog.Warn("openai_messages.account_select_failed",
				zap.Error(err),
				zap.Int("excluded_account_count", len(failedAccountIDs)),
			)
			if len(failedAccountIDs) == 0 {
				if err != nil {
					markOpsRoutingCapacityLimitedIfNoAvailable(c, err)
					h.anthropicStreamingAwareError(c, http.StatusServiceUnavailable, "api_error", "Service temporarily unavailable", streamStarted)
					return
			REDACTED
		REDACTED else {
				if lastFailoverErr != nil {
					h.handleAnthropicFailoverExhausted(c, lastFailoverErr, streamStarted)
			REDACTED else {
					h.anthropicStreamingAwareError(c, http.StatusBadGateway, "api_error", "Upstream request failed", streamStarted)
			REDACTED
				return
		REDACTED
	REDACTED
		if selection == nil || selection.Account == nil {
			markOpsRoutingCapacityLimited(c)
			h.anthropicStreamingAwareError(c, http.StatusServiceUnavailable, "api_error", "No available accounts", streamStarted)
			return
	REDACTED
		account := selection.Account
		sessionHash = ensureOpenAIPoolModeSessionHash(sessionHash, account)
		reqLog.Debug("openai_messages.account_selected", zap.Int64("account_id", account.ID), zap.String("account_name", account.Name))
		_ = scheduleDecision
		setOpsSelectedAccount(c, account.ID, account.Platform)

		accountReleaseFunc, acquired := h.acquireResponsesAccountSlot(c, apiKey.GroupID, sessionHash, selection, reqStream, &streamStarted, reqLog)
		if !acquired {
			return
	REDACTED

		service.SetOpsLatencyMs(c, service.OpsRoutingLatencyMsKey, time.Since(routingStart).Milliseconds())
		forwardStart := time.Now()

		defaultMappedModel := strings.TrimSpace(effectiveMappedModel)
		// 应用渠道模型映射到请求体
		forwardBody := body
		if channelMappingMsg.Mapped {
			forwardBody = h.gatewayService.ReplaceModelInBody(body, channelMappingMsg.MappedModel)
	REDACTED
		writerSizeBeforeForward := c.Writer.Size()
		result, err := func() (*service.OpenAIForwardResult, error) {
			defer func() {
				if accountReleaseFunc != nil {
					accountReleaseFunc()
			REDACTED
		REDACTED()
			return h.gatewayService.ForwardAsAnthropic(c.Request.Context(), c, account, forwardBody, promptCacheKey, defaultMappedModel)
	REDACTED()

		forwardDurationMs := time.Since(forwardStart).Milliseconds()
		upstreamLatencyMs, _ := getContextInt64(c, service.OpsUpstreamLatencyMsKey)
		responseLatencyMs := forwardDurationMs
		if upstreamLatencyMs > 0 && forwardDurationMs > upstreamLatencyMs {
			responseLatencyMs = forwardDurationMs - upstreamLatencyMs
	REDACTED
		service.SetOpsLatencyMs(c, service.OpsResponseLatencyMsKey, responseLatencyMs)
		if err == nil && result != nil && result.FirstTokenMs != nil {
			service.SetOpsLatencyMs(c, service.OpsTimeToFirstTokenMsKey, int64(*result.FirstTokenMs))
	REDACTED
		if err != nil {
			if result != nil && result.ImageCount > 0 {
				reqLog.Warn("openai_messages.forward_partial_error_with_image_result",
					zap.Int64("account_id", account.ID),
					zap.Int("image_count", result.ImageCount),
					zap.Error(err),
				)
		REDACTED else {
				var failoverErr *service.UpstreamFailoverError
				if errors.As(err, &failoverErr) {
					if c.Writer.Size() != writerSizeBeforeForward {
						h.handleAnthropicFailoverExhausted(c, failoverErr, true)
						return
				REDACTED
					h.gatewayService.ReportOpenAIAccountScheduleResult(account.ID, false, nil)
					// 池模式：同账号重试
					if failoverErr.RetryableOnSameAccount {
						retryLimit := account.GetPoolModeRetryCount()
						if sameAccountRetryCount[account.ID] < retryLimit {
							sameAccountRetryCount[account.ID]++
							reqLog.Warn("openai_messages.pool_mode_same_account_retry",
								zap.Int64("account_id", account.ID),
								zap.Int("upstream_status", failoverErr.StatusCode),
								zap.Int("retry_limit", retryLimit),
								zap.Int("retry_count", sameAccountRetryCount[account.ID]),
							)
							select {
							case <-c.Request.Context().Done():
								return
							case <-time.After(sameAccountRetryDelay):
						REDACTED
							continue
					REDACTED
				REDACTED
					h.gatewayService.RecordOpenAIAccountSwitch()
					failedAccountIDs[account.ID] = struct{REDACTED{REDACTED
					lastFailoverErr = failoverErr
					if switchCount >= maxAccountSwitches {
						h.handleAnthropicFailoverExhausted(c, failoverErr, streamStarted)
						return
				REDACTED
					switchCount++
					if h.gatewayService.ShouldStopOpenAIOAuth429Failover(account, failoverErr.StatusCode, switchCount) {
						h.handleAnthropicFailoverExhausted(c, failoverErr, streamStarted)
						return
				REDACTED
					reqLog.Warn("openai_messages.upstream_failover_switching",
						zap.Int64("account_id", account.ID),
						zap.Int("upstream_status", failoverErr.StatusCode),
						zap.Int("switch_count", switchCount),
						zap.Int("max_switches", maxAccountSwitches),
					)
					continue
			REDACTED
				if result != nil && result.ClientDisconnect {
					reqLog.Info("openai_messages.client_disconnected",
						zap.Int64("account_id", account.ID),
						zap.Error(err),
					)
					return
			REDACTED
				h.gatewayService.ReportOpenAIAccountScheduleResult(account.ID, false, nil)
				wroteFallback := h.ensureAnthropicErrorResponse(c, streamStarted)
				reqLog.Warn("openai_messages.forward_failed",
					zap.Int64("account_id", account.ID),
					zap.Bool("fallback_error_response_written", wroteFallback),
					zap.Error(err),
				)
				return
		REDACTED
	REDACTED
		if result != nil {
			h.gatewayService.ReportOpenAIAccountScheduleResult(account.ID, true, result.FirstTokenMs)
	REDACTED else {
			h.gatewayService.ReportOpenAIAccountScheduleResult(account.ID, true, nil)
	REDACTED

		userAgent := c.GetHeader("User-Agent")
		clientIP := ip.GetClientIP(c)
		requestPayloadHash := service.HashUsageRequestPayload(body)
		inboundEndpoint := GetInboundEndpoint(c)
		upstreamEndpoint := GetUpstreamEndpoint(c, account.Platform)

		h.submitOpenAIUsageRecordTask(c.Request.Context(), result, func(ctx context.Context) {
			if err := h.gatewayService.RecordUsage(ctx, &service.OpenAIRecordUsageInput{
				Result:             result,
				APIKey:             apiKey,
				User:               apiKey.User,
				Account:            account,
				Subscription:       subscription,
				InboundEndpoint:    inboundEndpoint,
				UpstreamEndpoint:   upstreamEndpoint,
				UserAgent:          userAgent,
				IPAddress:          clientIP,
				RequestPayloadHash: requestPayloadHash,
				APIKeyService:      h.apiKeyService,
				ChannelUsageFields: channelMappingMsg.ToUsageFields(reqModel, result.UpstreamModel),
		REDACTED); err != nil {
				logger.L().With(
					zap.String("component", "handler.openai_gateway.messages"),
					zap.Int64("user_id", subject.UserID),
					zap.Int64("api_key_id", apiKey.ID),
					zap.Any("group_id", apiKey.GroupID),
					zap.String("model", reqModel),
					zap.Int64("account_id", account.ID),
				).Error("openai_messages.record_usage_failed", zap.Error(err))
		REDACTED
	REDACTED)
		reqLog.Debug("openai_messages.request_completed",
			zap.Int64("account_id", account.ID),
			zap.Int("switch_count", switchCount),
		)
		return
REDACTED
REDACTED

func resolveOpenAIMessagesMetadataSession(sessionHash, promptCacheKey, reqModel string, body []byte) (string, string) {
	// Anthropic metadata.user_id 只作为账号粘性信号。上游 GPT/Codex 缓存键
	// 交给 ForwardAsAnthropic 从 cache_control 或完整消息 digest 派生，避免
	// 固定 metadata key 压住后续 turn 的缓存滚动。
	if sessionHash != "" {
		return sessionHash, promptCacheKey
REDACTED
	if userID := strings.TrimSpace(gjson.GetBytes(body, "metadata.user_id").String()); userID != "" {
		seed := reqModel + "-" + userID
		sessionHash = service.DeriveSessionHashFromSeed(seed)
REDACTED
	return sessionHash, promptCacheKey
REDACTED

// anthropicErrorResponse writes an error in Anthropic Messages API format.
func (h *OpenAIGatewayHandler) anthropicErrorResponse(c *gin.Context, status int, errType, message string) {
	c.JSON(status, gin.H{
		"type": "error",
		"error": gin.H{
			"type":    errType,
			"message": message,
	REDACTED,
REDACTED)
REDACTED

// anthropicStreamingAwareError handles errors that may occur during streaming,
// using Anthropic SSE error format.
func (h *OpenAIGatewayHandler) anthropicStreamingAwareError(c *gin.Context, status int, errType, message string, streamStarted bool) {
	if streamStarted {
		flusher, ok := c.Writer.(http.Flusher)
		if ok {
			errPayload, _ := json.Marshal(gin.H{
				"type": "error",
				"error": gin.H{
					"type":    errType,
					"message": message,
			REDACTED,
		REDACTED)
			fmt.Fprintf(c.Writer, "event: error\ndata: %s\n\n", errPayload) //nolint:errcheck
			flusher.Flush()
	REDACTED
		return
REDACTED
	h.anthropicErrorResponse(c, status, errType, message)
REDACTED

// handleAnthropicFailoverExhausted maps upstream failover errors to Anthropic format.
func (h *OpenAIGatewayHandler) handleAnthropicFailoverExhausted(c *gin.Context, failoverErr *service.UpstreamFailoverError, streamStarted bool) {
	status, errType, errMsg := h.mapUpstreamError(failoverErr.StatusCode)
	h.anthropicStreamingAwareError(c, status, errType, errMsg, streamStarted)
REDACTED

// ensureAnthropicErrorResponse writes a fallback Anthropic error if no response was written.
func (h *OpenAIGatewayHandler) ensureAnthropicErrorResponse(c *gin.Context, streamStarted bool) bool {
	if c == nil || c.Writer == nil || c.Writer.Written() {
		return false
REDACTED
	h.anthropicStreamingAwareError(c, http.StatusBadGateway, "api_error", "Upstream request failed", streamStarted)
	return true
REDACTED

func (h *OpenAIGatewayHandler) validateFunctionCallOutputRequest(c *gin.Context, body []byte, reqLog *zap.Logger) bool {
	if !gjson.GetBytes(body, `input.#(type=="function_call_output")`).Exists() {
		return true
REDACTED

	validation := service.ValidateFunctionCallOutputContextBytes(body)
	if !validation.HasFunctionCallOutput {
		return true
REDACTED

	previousResponseID := gjson.GetBytes(body, "previous_response_id").String()
	if strings.TrimSpace(previousResponseID) != "" || validation.HasToolCallContext {
		return true
REDACTED

	if validation.HasFunctionCallOutputMissingCallID {
		reqLog.Warn("openai.request_validation_failed",
			zap.String("reason", "function_call_output_missing_call_id"),
		)
		h.errorResponse(c, http.StatusBadRequest, "invalid_request_error", "function_call_output requires call_id on HTTP requests; continuation via previous_response_id is only supported on Responses WebSocket v2")
		return false
REDACTED
	if validation.HasItemReferenceForAllCallIDs {
		return true
REDACTED

	reqLog.Warn("openai.request_validation_failed",
		zap.String("reason", "function_call_output_missing_item_reference"),
	)
	h.errorResponse(c, http.StatusBadRequest, "invalid_request_error", "function_call_output requires item_reference ids matching each call_id on HTTP requests; continuation via previous_response_id is only supported on Responses WebSocket v2")
	return false
REDACTED

func (h *OpenAIGatewayHandler) acquireResponsesUserSlot(
	c *gin.Context,
	userID int64,
	userConcurrency int,
	reqStream bool,
	streamStarted *bool,
	reqLog *zap.Logger,
) (func(), bool) {
	ctx := c.Request.Context()
	userReleaseFunc, userAcquired, err := h.concurrencyHelper.TryAcquireUserSlot(ctx, userID, userConcurrency)
	if err != nil {
		reqLog.Warn("openai.user_slot_acquire_failed", zap.Error(err))
		h.handleConcurrencyError(c, err, "user", *streamStarted)
		return nil, false
REDACTED
	if userAcquired {
		return wrapReleaseOnDone(ctx, userReleaseFunc), true
REDACTED

	maxWait := service.CalculateMaxWait(userConcurrency)
	canWait, waitErr := h.concurrencyHelper.IncrementWaitCount(ctx, userID, maxWait)
	if waitErr != nil {
		reqLog.Warn("openai.user_wait_counter_increment_failed", zap.Error(waitErr))
		// 按现有降级语义：等待计数异常时放行后续抢槽流程
REDACTED else if !canWait {
		reqLog.Info("openai.user_wait_queue_full", zap.Int("max_wait", maxWait))
		h.errorResponse(c, http.StatusTooManyRequests, "rate_limit_error", "Too many pending requests, please retry later")
		return nil, false
REDACTED

	waitCounted := waitErr == nil && canWait
	defer func() {
		if waitCounted {
			h.concurrencyHelper.DecrementWaitCount(ctx, userID)
	REDACTED
REDACTED()

	userReleaseFunc, err = h.concurrencyHelper.AcquireUserSlotWithWait(c, userID, userConcurrency, reqStream, streamStarted)
	if err != nil {
		reqLog.Warn("openai.user_slot_acquire_failed_after_wait", zap.Error(err))
		h.handleConcurrencyError(c, err, "user", *streamStarted)
		return nil, false
REDACTED

	// 槽位获取成功后，立刻退出等待计数。
	if waitCounted {
		h.concurrencyHelper.DecrementWaitCount(ctx, userID)
		waitCounted = false
REDACTED
	return wrapReleaseOnDone(ctx, userReleaseFunc), true
REDACTED

func (h *OpenAIGatewayHandler) acquireResponsesAccountSlot(
	c *gin.Context,
	groupID *int64,
	sessionHash string,
	selection *service.AccountSelectionResult,
	reqStream bool,
	streamStarted *bool,
	reqLog *zap.Logger,
) (func(), bool) {
	if selection == nil || selection.Account == nil {
		markOpsRoutingCapacityLimited(c)
		h.handleStreamingAwareError(c, http.StatusServiceUnavailable, "api_error", "No available accounts", *streamStarted)
		return nil, false
REDACTED

	ctx := c.Request.Context()
	account := selection.Account
	if selection.Acquired {
		return wrapReleaseOnDone(ctx, selection.ReleaseFunc), true
REDACTED
	if selection.WaitPlan == nil {
		markOpsRoutingCapacityLimited(c)
		h.handleStreamingAwareError(c, http.StatusServiceUnavailable, "api_error", "No available accounts", *streamStarted)
		return nil, false
REDACTED

	fastReleaseFunc, fastAcquired, err := h.concurrencyHelper.TryAcquireAccountSlot(
		ctx,
		account.ID,
		selection.WaitPlan.MaxConcurrency,
	)
	if err != nil {
		reqLog.Warn("openai.account_slot_quick_acquire_failed", zap.Int64("account_id", account.ID), zap.Error(err))
		h.handleConcurrencyError(c, err, "account", *streamStarted)
		return nil, false
REDACTED
	if fastAcquired {
		if err := h.gatewayService.BindStickySession(ctx, groupID, sessionHash, account.ID); err != nil {
			reqLog.Warn("openai.bind_sticky_session_failed", zap.Int64("account_id", account.ID), zap.Error(err))
	REDACTED
		return wrapReleaseOnDone(ctx, fastReleaseFunc), true
REDACTED

	canWait, waitErr := h.concurrencyHelper.IncrementAccountWaitCount(ctx, account.ID, selection.WaitPlan.MaxWaiting)
	if waitErr != nil {
		reqLog.Warn("openai.account_wait_counter_increment_failed", zap.Int64("account_id", account.ID), zap.Error(waitErr))
REDACTED else if !canWait {
		reqLog.Info("openai.account_wait_queue_full",
			zap.Int64("account_id", account.ID),
			zap.Int("max_waiting", selection.WaitPlan.MaxWaiting),
		)
		h.handleStreamingAwareError(c, http.StatusTooManyRequests, "rate_limit_error", "Too many pending requests, please retry later", *streamStarted)
		return nil, false
REDACTED

	accountWaitCounted := waitErr == nil && canWait
	releaseWait := func() {
		if accountWaitCounted {
			h.concurrencyHelper.DecrementAccountWaitCount(ctx, account.ID)
			accountWaitCounted = false
	REDACTED
REDACTED
	defer releaseWait()

	accountReleaseFunc, err := h.concurrencyHelper.AcquireAccountSlotWithWaitTimeout(
		c,
		account.ID,
		selection.WaitPlan.MaxConcurrency,
		selection.WaitPlan.Timeout,
		reqStream,
		streamStarted,
	)
	if err != nil {
		reqLog.Warn("openai.account_slot_acquire_failed", zap.Int64("account_id", account.ID), zap.Error(err))
		h.handleConcurrencyError(c, err, "account", *streamStarted)
		return nil, false
REDACTED

	// Slot acquired: no longer waiting in queue.
	releaseWait()
	if err := h.gatewayService.BindStickySession(ctx, groupID, sessionHash, account.ID); err != nil {
		reqLog.Warn("openai.bind_sticky_session_failed", zap.Int64("account_id", account.ID), zap.Error(err))
REDACTED
	return wrapReleaseOnDone(ctx, accountReleaseFunc), true
REDACTED

// ResponsesWebSocket handles OpenAI Responses API WebSocket ingress endpoint
// GET /openai/v1/responses (Upgrade: websocket)
func (h *OpenAIGatewayHandler) ResponsesWebSocket(c *gin.Context) {
	if !isOpenAIWSUpgradeRequest(c.Request) {
		h.errorResponse(c, http.StatusUpgradeRequired, "invalid_request_error", "WebSocket upgrade required (Upgrade: websocket)")
		return
REDACTED
	setOpenAIClientTransportWS(c)

	apiKey, ok := middleware2.GetAPIKeyFromContext(c)
	if !ok {
		h.errorResponse(c, http.StatusUnauthorized, "authentication_error", "Invalid API key")
		return
REDACTED
	subject, ok := middleware2.GetAuthSubjectFromContext(c)
	if !ok {
		h.errorResponse(c, http.StatusInternalServerError, "api_error", "User context not found")
		return
REDACTED

	reqLog := requestLogger(
		c,
		"handler.openai_gateway.responses_ws",
		zap.Int64("user_id", subject.UserID),
		zap.Int64("api_key_id", apiKey.ID),
		zap.Any("group_id", apiKey.GroupID),
		zap.Bool("openai_ws_mode", true),
	)
	if !h.ensureResponsesDependencies(c, reqLog) {
		return
REDACTED
	reqLog.Info("openai.websocket_ingress_started")
	clientIP := ip.GetClientIP(c)
	userAgent := strings.TrimSpace(c.GetHeader("User-Agent"))

	wsConn, err := coderws.Accept(c.Writer, c.Request, &coderws.AcceptOptions{
		CompressionMode: coderws.CompressionContextTakeover,
REDACTED)
	if err != nil {
		reqLog.Warn("openai.websocket_accept_failed",
			zap.Error(err),
			zap.String("client_ip", clientIP),
			zap.String("request_user_agent", userAgent),
			zap.String("upgrade_header", strings.TrimSpace(c.GetHeader("Upgrade"))),
			zap.String("connection_header", strings.TrimSpace(c.GetHeader("Connection"))),
			zap.String("sec_websocket_version", strings.TrimSpace(c.GetHeader("Sec-WebSocket-Version"))),
			zap.Bool("has_sec_websocket_key", strings.TrimSpace(c.GetHeader("Sec-WebSocket-Key")) != ""),
		)
		return
REDACTED
	defer func() {
		_ = wsConn.CloseNow()
REDACTED()
	wsConn.SetReadLimit(service.ResolveOpenAIWSClientReadLimitBytes(h.cfg))

	ctx := c.Request.Context()
	readCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	msgType, firstMessage, err := wsConn.Read(readCtx)
	cancel()
	if err != nil {
		closeStatus, closeReason := summarizeWSCloseErrorForLog(err)
		reqLog.Warn("openai.websocket_read_first_message_failed",
			zap.Error(err),
			zap.String("client_ip", clientIP),
			zap.String("close_status", closeStatus),
			zap.String("close_reason", closeReason),
			zap.Duration("read_timeout", 30*time.Second),
		)
		closeOpenAIClientWS(wsConn, coderws.StatusPolicyViolation, "missing first response.create message")
		return
REDACTED
	if msgType != coderws.MessageText && msgType != coderws.MessageBinary {
		closeOpenAIClientWS(wsConn, coderws.StatusPolicyViolation, "unsupported websocket message type")
		return
REDACTED
	if !gjson.ValidBytes(firstMessage) {
		closeOpenAIClientWS(wsConn, coderws.StatusPolicyViolation, "invalid JSON payload")
		return
REDACTED

	reqModel := strings.TrimSpace(gjson.GetBytes(firstMessage, "model").String())
	if reqModel == "" {
		closeOpenAIClientWS(wsConn, coderws.StatusPolicyViolation, "model is required in first response.create payload")
		return
REDACTED
	previousResponseID := strings.TrimSpace(gjson.GetBytes(firstMessage, "previous_response_id").String())
	previousResponseIDKind := service.ClassifyOpenAIPreviousResponseIDKind(previousResponseID)
	if previousResponseID != "" && previousResponseIDKind == service.OpenAIPreviousResponseIDKindMessageID {
		closeOpenAIClientWS(wsConn, coderws.StatusPolicyViolation, "previous_response_id must be a response.id (resp_*), not a message id")
		return
REDACTED
	reqLog = reqLog.With(
		zap.Bool("ws_ingress", true),
		zap.String("model", reqModel),
		zap.Bool("has_previous_response_id", previousResponseID != ""),
		zap.String("previous_response_id_kind", previousResponseIDKind),
	)
	setOpsRequestContext(c, reqModel, true)
	setOpsEndpointContext(c, "", int16(service.RequestTypeWSV2))

	if decision := h.checkContentModeration(c, reqLog, apiKey, subject, service.ContentModerationProtocolOpenAIResponses, reqModel, firstMessage); decision != nil && decision.Blocked {
		writeContentModerationWSError(ctx, wsConn, decision)
		closeOpenAIClientWS(wsConn, coderws.StatusPolicyViolation, decision.Message)
		return
REDACTED

	if service.IsImageGenerationIntent("/v1/responses", reqModel, firstMessage) && !service.GroupAllowsImageGeneration(apiKey.Group) {
		closeOpenAIClientWS(wsConn, coderws.StatusPolicyViolation, service.ImageGenerationPermissionMessage())
		return
REDACTED

	// 解析渠道级模型映射
	channelMappingWS, _ := h.gatewayService.ResolveChannelMappingAndRestrict(ctx, apiKey.GroupID, reqModel)

	var currentUserRelease func()
	var currentAccountRelease func()
	releaseAccountSlot := func() {
		if currentAccountRelease != nil {
			currentAccountRelease()
			currentAccountRelease = nil
	REDACTED
REDACTED
	releaseTurnSlots := func() {
		releaseAccountSlot()
		if currentUserRelease != nil {
			currentUserRelease()
			currentUserRelease = nil
	REDACTED
REDACTED
	// 必须尽早注册，确保任何 early return 都能释放已获取的并发槽位。
	defer releaseTurnSlots()

	userReleaseFunc, userAcquired, err := h.concurrencyHelper.TryAcquireUserSlot(ctx, subject.UserID, subject.Concurrency)
	if err != nil {
		reqLog.Warn("openai.websocket_user_slot_acquire_failed", zap.Error(err))
		closeOpenAIClientWS(wsConn, coderws.StatusInternalError, "failed to acquire user concurrency slot")
		return
REDACTED
	if !userAcquired {
		closeOpenAIClientWS(wsConn, coderws.StatusTryAgainLater, "too many concurrent requests, please retry later")
		return
REDACTED
	currentUserRelease = wrapReleaseOnDone(ctx, userReleaseFunc)
	ensureUserSlotHeld := func() bool {
		if currentUserRelease != nil {
			return true
	REDACTED
		userReleaseFunc, userAcquired, err := h.concurrencyHelper.TryAcquireUserSlot(ctx, subject.UserID, subject.Concurrency)
		if err != nil {
			reqLog.Warn("openai.websocket_user_slot_reacquire_failed", zap.Error(err))
			closeOpenAIClientWS(wsConn, coderws.StatusInternalError, "failed to acquire user concurrency slot")
			return false
	REDACTED
		if !userAcquired {
			closeOpenAIClientWS(wsConn, coderws.StatusTryAgainLater, "too many concurrent requests, please retry later")
			return false
	REDACTED
		currentUserRelease = wrapReleaseOnDone(ctx, userReleaseFunc)
		return true
REDACTED

	subscription, _ := middleware2.GetSubscriptionFromContext(c)
	if err := h.billingCacheService.CheckBillingEligibility(ctx, apiKey.User, apiKey, apiKey.Group, subscription, service.QuotaPlatform(c.Request.Context(), apiKey)); err != nil {
		reqLog.Info("openai.websocket_billing_eligibility_check_failed", zap.Error(err))
		closeOpenAIClientWS(wsConn, coderws.StatusPolicyViolation, "billing check failed")
		return
REDACTED

	sessionHash := h.gatewayService.GenerateSessionHashWithFallback(
		c,
		firstMessage,
		openAIWSIngressFallbackSessionSeed(subject.UserID, apiKey.ID, apiKey.GroupID),
	)
	maxAccountSwitches := h.maxAccountSwitches
	switchCount := 0
	failedAccountIDs := make(map[int64]struct{REDACTED)
	var lastFailoverErr *service.UpstreamFailoverError

	for {
		reqLog.Debug("openai.websocket_account_selecting", zap.Int("excluded_account_count", len(failedAccountIDs)))
		selection, scheduleDecision, err := h.gatewayService.SelectAccountWithSchedulerForCapability(
			ctx,
			apiKey.GroupID,
			previousResponseID,
			sessionHash,
			reqModel,
			failedAccountIDs,
			service.OpenAIUpstreamTransportResponsesWebsocketV2,
			service.OpenAIEndpointCapabilityChatCompletions,
			false,
		)
		if err != nil {
			reqLog.Warn("openai.websocket_account_select_failed",
				zap.Error(err),
				zap.Int("excluded_account_count", len(failedAccountIDs)),
			)
			if lastFailoverErr != nil {
				closeOpenAIWSFailoverExhausted(wsConn, lastFailoverErr)
		REDACTED else {
				closeOpenAIClientWS(wsConn, coderws.StatusTryAgainLater, "no available account")
		REDACTED
			return
	REDACTED
		if selection == nil || selection.Account == nil {
			if lastFailoverErr != nil {
				closeOpenAIWSFailoverExhausted(wsConn, lastFailoverErr)
		REDACTED else {
				closeOpenAIClientWS(wsConn, coderws.StatusTryAgainLater, "no available account")
		REDACTED
			return
	REDACTED

		account := selection.Account
		accountMaxConcurrency := account.Concurrency
		if selection.WaitPlan != nil && selection.WaitPlan.MaxConcurrency > 0 {
			accountMaxConcurrency = selection.WaitPlan.MaxConcurrency
	REDACTED
		accountReleaseFunc := selection.ReleaseFunc
		if !selection.Acquired {
			if selection.WaitPlan == nil {
				closeOpenAIClientWS(wsConn, coderws.StatusTryAgainLater, "account is busy, please retry later")
				return
		REDACTED
			fastReleaseFunc, fastAcquired, err := h.concurrencyHelper.TryAcquireAccountSlot(
				ctx,
				account.ID,
				selection.WaitPlan.MaxConcurrency,
			)
			if err != nil {
				reqLog.Warn("openai.websocket_account_slot_acquire_failed", zap.Int64("account_id", account.ID), zap.Error(err))
				closeOpenAIClientWS(wsConn, coderws.StatusInternalError, "failed to acquire account concurrency slot")
				return
		REDACTED
			if !fastAcquired {
				closeOpenAIClientWS(wsConn, coderws.StatusTryAgainLater, "account is busy, please retry later")
				return
		REDACTED
			accountReleaseFunc = fastReleaseFunc
	REDACTED
		currentAccountRelease = wrapReleaseOnDone(ctx, accountReleaseFunc)
		if err := h.gatewayService.BindStickySession(ctx, apiKey.GroupID, sessionHash, account.ID); err != nil {
			reqLog.Warn("openai.websocket_bind_sticky_session_failed", zap.Int64("account_id", account.ID), zap.Error(err))
	REDACTED

		token, _, err := h.gatewayService.GetAccessToken(ctx, account)
		if err != nil {
			reqLog.Warn("openai.websocket_get_access_token_failed", zap.Int64("account_id", account.ID), zap.Error(err))
			closeOpenAIClientWS(wsConn, coderws.StatusInternalError, "failed to get access token")
			return
	REDACTED

		reqLog.Debug("openai.websocket_account_selected",
			zap.Int64("account_id", account.ID),
			zap.String("account_name", account.Name),
			zap.String("schedule_layer", scheduleDecision.Layer),
			zap.Int("candidate_count", scheduleDecision.CandidateCount),
		)

		var requestPayloadHash string
		hooks := &service.OpenAIWSIngressHooks{
			InitialRequestModel: reqModel,
			BeforeRequest: func(turn int, payload []byte, originalModel string) error {
				if turn == 1 {
					return nil
			REDACTED
				if !gjson.ValidBytes(payload) {
					return service.NewOpenAIWSClientCloseError(coderws.StatusPolicyViolation, "invalid websocket request payload", errors.New("invalid json"))
			REDACTED
				model := strings.TrimSpace(originalModel)
				if model == "" {
					model = strings.TrimSpace(gjson.GetBytes(payload, "model").String())
			REDACTED
				if model == "" {
					model = reqModel
			REDACTED
				if decision := h.checkContentModeration(c, reqLog, apiKey, subject, service.ContentModerationProtocolOpenAIResponses, model, payload); decision != nil && decision.Blocked {
					writeContentModerationWSError(ctx, wsConn, decision)
					return service.NewOpenAIWSClientCloseError(coderws.StatusPolicyViolation, decision.Message, nil)
			REDACTED
				return nil
		REDACTED,
			BeforeTurn: func(turn int) error {
				if turn == 1 {
					return nil
			REDACTED
				// 防御式清理：避免异常路径下旧槽位覆盖导致泄漏。
				releaseTurnSlots()
				// 非首轮 turn 需要重新抢占并发槽位，避免长连接空闲占槽。
				userReleaseFunc, userAcquired, err := h.concurrencyHelper.TryAcquireUserSlot(ctx, subject.UserID, subject.Concurrency)
				if err != nil {
					return service.NewOpenAIWSClientCloseError(coderws.StatusInternalError, "failed to acquire user concurrency slot", err)
			REDACTED
				if !userAcquired {
					return service.NewOpenAIWSClientCloseError(coderws.StatusTryAgainLater, "too many concurrent requests, please retry later", nil)
			REDACTED
				accountReleaseFunc, accountAcquired, err := h.concurrencyHelper.TryAcquireAccountSlot(ctx, account.ID, accountMaxConcurrency)
				if err != nil {
					if userReleaseFunc != nil {
						userReleaseFunc()
				REDACTED
					return service.NewOpenAIWSClientCloseError(coderws.StatusInternalError, "failed to acquire account concurrency slot", err)
			REDACTED
				if !accountAcquired {
					if userReleaseFunc != nil {
						userReleaseFunc()
				REDACTED
					return service.NewOpenAIWSClientCloseError(coderws.StatusTryAgainLater, "account is busy, please retry later", nil)
			REDACTED
				currentUserRelease = wrapReleaseOnDone(ctx, userReleaseFunc)
				currentAccountRelease = wrapReleaseOnDone(ctx, accountReleaseFunc)
				return nil
		REDACTED,
			AfterTurn: func(turn int, result *service.OpenAIForwardResult, turnErr error) {
				releaseTurnSlots()
				if turnErr != nil {
					if result == nil || result.ImageCount <= 0 {
						return
				REDACTED
					reqLog.Warn("openai.websocket_partial_error_with_image_result",
						zap.Int64("account_id", account.ID),
						zap.Int("image_count", result.ImageCount),
						zap.Error(turnErr),
					)
			REDACTED
				if result == nil {
					return
			REDACTED
				if account.Type == service.AccountTypeOAuth {
					h.gatewayService.UpdateCodexUsageSnapshotFromHeaders(ctx, account.ID, result.ResponseHeaders)
			REDACTED
				h.gatewayService.ReportOpenAIAccountScheduleResult(account.ID, true, result.FirstTokenMs)
				inboundEndpoint := GetInboundEndpoint(c)
				upstreamEndpoint := GetUpstreamEndpoint(c, account.Platform)
				h.submitOpenAIUsageRecordTask(ctx, result, func(taskCtx context.Context) {
					if err := h.gatewayService.RecordUsage(taskCtx, &service.OpenAIRecordUsageInput{
						Result:             result,
						APIKey:             apiKey,
						User:               apiKey.User,
						Account:            account,
						Subscription:       subscription,
						InboundEndpoint:    inboundEndpoint,
						UpstreamEndpoint:   upstreamEndpoint,
						UserAgent:          userAgent,
						IPAddress:          clientIP,
						RequestPayloadHash: requestPayloadHash,
						APIKeyService:      h.apiKeyService,
						ChannelUsageFields: channelMappingWS.ToUsageFields(reqModel, result.UpstreamModel),
				REDACTED); err != nil {
						reqLog.Error("openai.websocket_record_usage_failed",
							zap.Int64("account_id", account.ID),
							zap.String("request_id", result.RequestID),
							zap.Error(err),
						)
				REDACTED
			REDACTED)
		REDACTED,
	REDACTED

		// 应用渠道模型映射到 WebSocket 首条消息
		wsFirstMessage := firstMessage
		if channelMappingWS.Mapped {
			wsFirstMessage = h.gatewayService.ReplaceModelInBody(firstMessage, channelMappingWS.MappedModel)
	REDACTED

		// WebSocket 首包可能很大，hash 必须在 hooks 外算成字符串，避免 AfterTurn 闭包保活请求体。
		requestPayloadHash = service.HashUsageRequestPayload(wsFirstMessage)

		if err := h.gatewayService.ProxyResponsesWebSocketFromClient(ctx, c, wsConn, account, token, wsFirstMessage, hooks); err != nil {
			var failoverErr *service.UpstreamFailoverError
			if errors.As(err, &failoverErr) {
				h.gatewayService.ReportOpenAIAccountScheduleResult(account.ID, false, nil)
				releaseAccountSlot()
				failedAccountIDs[account.ID] = struct{REDACTED{REDACTED
				lastFailoverErr = failoverErr
				if switchCount >= maxAccountSwitches {
					closeOpenAIWSFailoverExhausted(wsConn, failoverErr)
					return
			REDACTED
				switchCount++
				if h.gatewayService.ShouldStopOpenAIOAuth429Failover(account, failoverErr.StatusCode, switchCount) {
					closeOpenAIWSFailoverExhausted(wsConn, failoverErr)
					return
			REDACTED
				h.gatewayService.RecordOpenAIAccountSwitch()
				reqLog.Warn("openai.websocket_upstream_failover_switching",
					zap.Int64("account_id", account.ID),
					zap.Int("upstream_status", failoverErr.StatusCode),
					zap.Int("switch_count", switchCount),
					zap.Int("max_switches", maxAccountSwitches),
				)
				if !ensureUserSlotHeld() {
					return
			REDACTED
				continue
		REDACTED

			h.gatewayService.ReportOpenAIAccountScheduleResult(account.ID, false, nil)
			closeStatus, closeReason := summarizeWSCloseErrorForLog(err)
			reqLog.Warn("openai.websocket_proxy_failed",
				zap.Int64("account_id", account.ID),
				zap.Error(err),
				zap.String("close_status", closeStatus),
				zap.String("close_reason", closeReason),
			)
			var closeErr *service.OpenAIWSClientCloseError
			if errors.As(err, &closeErr) {
				closeOpenAIClientWS(wsConn, closeErr.StatusCode(), closeErr.Reason())
				return
		REDACTED
			closeOpenAIClientWS(wsConn, coderws.StatusInternalError, "upstream websocket proxy failed")
			return
	REDACTED
		reqLog.Info("openai.websocket_ingress_closed", zap.Int64("account_id", account.ID))
		return
REDACTED

REDACTED

func (h *OpenAIGatewayHandler) recoverResponsesPanic(c *gin.Context, streamStarted *bool) {
	recovered := recover()
	if recovered == nil {
		return
REDACTED

	started := false
	if streamStarted != nil {
		started = *streamStarted
REDACTED
	wroteFallback := h.ensureForwardErrorResponse(c, started)
	requestLogger(c, "handler.openai_gateway.responses").Error(
		"openai.responses_panic_recovered",
		zap.Bool("fallback_error_response_written", wroteFallback),
		zap.Any("panic", recovered),
		zap.ByteString("stack", debug.Stack()),
	)
REDACTED

// recoverAnthropicMessagesPanic recovers from panics in the Anthropic Messages
// handler and returns an Anthropic-formatted error response.
func (h *OpenAIGatewayHandler) recoverAnthropicMessagesPanic(c *gin.Context, streamStarted *bool) {
	recovered := recover()
	if recovered == nil {
		return
REDACTED

	started := streamStarted != nil && *streamStarted
	requestLogger(c, "handler.openai_gateway.messages").Error(
		"openai.messages_panic_recovered",
		zap.Bool("stream_started", started),
		zap.Any("panic", recovered),
		zap.ByteString("stack", debug.Stack()),
	)
	if !started {
		h.anthropicErrorResponse(c, http.StatusInternalServerError, "api_error", "Internal server error")
REDACTED
REDACTED

func (h *OpenAIGatewayHandler) ensureResponsesDependencies(c *gin.Context, reqLog *zap.Logger) bool {
	missing := h.missingResponsesDependencies()
	if len(missing) == 0 {
		return true
REDACTED

	if reqLog == nil {
		reqLog = requestLogger(c, "handler.openai_gateway.responses")
REDACTED
	reqLog.Error("openai.handler_dependencies_missing", zap.Strings("missing_dependencies", missing))

	if c != nil && c.Writer != nil && !c.Writer.Written() {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"error": gin.H{
				"type":    "api_error",
				"message": "Service temporarily unavailable",
		REDACTED,
	REDACTED)
REDACTED
	return false
REDACTED

func (h *OpenAIGatewayHandler) missingResponsesDependencies() []string {
	missing := make([]string, 0, 5)
	if h == nil {
		return append(missing, "handler")
REDACTED
	if h.gatewayService == nil {
		missing = append(missing, "gatewayService")
REDACTED
	if h.billingCacheService == nil {
		missing = append(missing, "billingCacheService")
REDACTED
	if h.apiKeyService == nil {
		missing = append(missing, "apiKeyService")
REDACTED
	if h.concurrencyHelper == nil || h.concurrencyHelper.concurrencyService == nil {
		missing = append(missing, "concurrencyHelper")
REDACTED
	return missing
REDACTED

func getContextInt64(c *gin.Context, key string) (int64, bool) {
	if c == nil || key == "" {
		return 0, false
REDACTED
	v, ok := c.Get(key)
	if !ok {
		return 0, false
REDACTED
	switch t := v.(type) {
	case int64:
		return t, true
	case int:
		return int64(t), true
	case int32:
		return int64(t), true
	case float64:
		return int64(t), true
	default:
		return 0, false
REDACTED
REDACTED

func (h *OpenAIGatewayHandler) submitUsageRecordTask(parent context.Context, task service.UsageRecordTask) {
	if task == nil {
		return
REDACTED
	task = wrapUsageRecordTaskContext(parent, task)
	if h.usageRecordWorkerPool != nil {
		h.usageRecordWorkerPool.Submit(task)
		return
REDACTED
	// 回退路径：worker 池未注入时同步执行，避免退回到无界 goroutine 模式。
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	defer func() {
		if recovered := recover(); recovered != nil {
			logger.L().With(
				zap.String("component", "handler.openai_gateway.responses"),
				zap.Any("panic", recovered),
			).Error("openai.usage_record_task_panic_recovered")
	REDACTED
REDACTED()
	task(ctx)
REDACTED

func (h *OpenAIGatewayHandler) submitOpenAIUsageRecordTask(parent context.Context, result *service.OpenAIForwardResult, task service.UsageRecordTask) {
	if result != nil && result.ImageCount > 0 {
		h.submitMandatoryUsageRecordTask(parent, task)
		return
REDACTED
	h.submitUsageRecordTask(parent, task)
REDACTED

func (h *OpenAIGatewayHandler) submitMandatoryUsageRecordTask(parent context.Context, task service.UsageRecordTask) {
	if task == nil {
		return
REDACTED
	task = wrapUsageRecordTaskContext(parent, task)
	if h.usageRecordWorkerPool != nil {
		if mode := h.usageRecordWorkerPool.Submit(task); mode != service.UsageRecordSubmitModeDropped {
			return
	REDACTED
		logger.L().With(
			zap.String("component", "handler.openai_gateway.usage"),
		).Warn("openai.usage_record_task_mandatory_sync_fallback")
REDACTED
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	defer func() {
		if recovered := recover(); recovered != nil {
			logger.L().With(
				zap.String("component", "handler.openai_gateway.usage"),
				zap.Any("panic", recovered),
			).Error("openai.usage_record_task_panic_recovered")
	REDACTED
REDACTED()
	task(ctx)
REDACTED

func (h *OpenAIGatewayHandler) acquireImageGenerationSlot(c *gin.Context, streamStarted bool) (func(), bool) {
	if h == nil || h.cfg == nil || h.imageLimiter == nil {
		return nil, true
REDACTED
	imageConcurrency := h.cfg.Gateway.ImageConcurrency
	wait := strings.TrimSpace(imageConcurrency.OverflowMode) == config.ImageConcurrencyOverflowModeWait
	release, acquired := h.imageLimiter.Acquire(
		c.Request.Context(),
		imageConcurrency.Enabled,
		imageConcurrency.MaxConcurrentRequests,
		wait,
		time.Duration(imageConcurrency.WaitTimeoutSeconds)*time.Second,
		imageConcurrency.MaxWaitingRequests,
	)
	if acquired {
		return release, true
REDACTED
	h.handleStreamingAwareError(c, http.StatusTooManyRequests, "rate_limit_error", "Image generation concurrency limit exceeded, please retry later", streamStarted)
	return nil, false
REDACTED

// handleConcurrencyError handles concurrency-related acquire errors.
func (h *OpenAIGatewayHandler) handleConcurrencyError(c *gin.Context, err error, slotType string, streamStarted bool) {
	status, errType, message := concurrencyErrorResponse(err, slotType)
	h.handleStreamingAwareError(c, status, errType, message, streamStarted)
REDACTED

func (h *OpenAIGatewayHandler) handleFailoverExhausted(c *gin.Context, failoverErr *service.UpstreamFailoverError, streamStarted bool) {
	statusCode := failoverErr.StatusCode
	responseBody := failoverErr.ResponseBody
	if service.IsOpenAISilentRefusalErrorBody(responseBody) {
		service.SetOpsUpstreamError(c, statusCode, service.OpenAISilentRefusalClientMessage(), "")
		h.handleStreamingAwareError(c, http.StatusBadGateway, "upstream_error", service.OpenAISilentRefusalClientMessage(), streamStarted)
		return
REDACTED

	// 先检查透传规则
	if h.errorPassthroughService != nil && len(responseBody) > 0 {
		if rule := h.errorPassthroughService.MatchRule("openai", statusCode, responseBody); rule != nil {
			// 确定响应状态码
			respCode := statusCode
			if !rule.PassthroughCode && rule.ResponseCode != nil {
				respCode = *rule.ResponseCode
		REDACTED

			// 确定响应消息
			msg := service.ExtractUpstreamErrorMessage(responseBody)
			if !rule.PassthroughBody && rule.CustomMessage != nil {
				msg = *rule.CustomMessage
		REDACTED

			if rule.SkipMonitoring {
				c.Set(service.OpsSkipPassthroughKey, true)
		REDACTED

			h.handleStreamingAwareError(c, respCode, "upstream_error", msg, streamStarted)
			return
	REDACTED
REDACTED

	// 记录原始上游状态码，以便 ops 错误日志捕获真实的上游错误
	upstreamMsg := service.ExtractUpstreamErrorMessage(responseBody)
	service.SetOpsUpstreamError(c, statusCode, upstreamMsg, "")

	// 使用默认的错误映射
	status, errType, errMsg := h.mapUpstreamError(statusCode)
	h.handleStreamingAwareError(c, status, errType, errMsg, streamStarted)
REDACTED

// handleFailoverExhaustedSimple 简化版本，用于没有响应体的情况
func (h *OpenAIGatewayHandler) handleFailoverExhaustedSimple(c *gin.Context, statusCode int, streamStarted bool) {
	status, errType, errMsg := h.mapUpstreamError(statusCode)
	service.SetOpsUpstreamError(c, statusCode, errMsg, "")
	h.handleStreamingAwareError(c, status, errType, errMsg, streamStarted)
REDACTED

func (h *OpenAIGatewayHandler) mapUpstreamError(statusCode int) (int, string, string) {
	switch statusCode {
	case 401:
		return http.StatusBadGateway, "upstream_error", "Upstream authentication failed, please contact administrator"
	case 403:
		return http.StatusBadGateway, "upstream_error", "Upstream access forbidden, please contact administrator"
	case 429:
		return http.StatusTooManyRequests, "rate_limit_error", "Upstream rate limit exceeded, please retry later"
	case 529:
		return http.StatusServiceUnavailable, "upstream_error", "Upstream service overloaded, please retry later"
	case 500, 502, 503, 504:
		return http.StatusBadGateway, "upstream_error", "Upstream service temporarily unavailable"
	default:
		return http.StatusBadGateway, "upstream_error", "Upstream request failed"
REDACTED
REDACTED

// handleStreamingAwareError handles errors that may occur after streaming has started
func (h *OpenAIGatewayHandler) handleStreamingAwareError(c *gin.Context, status int, errType, message string, streamStarted bool) {
	if streamStarted {
		// /v1/responses 的严格 SDK（Codex CLI）要求终止事件必须属于
		// response.completed/failed/incomplete/cancelled 集合。
		// 通用 `event: error` 帧不被识别为终止事件，会导致
		// "stream closed before response.completed"。
		if inboundIsResponses(c) {
			if writeResponsesFailedSSE(c, errType, message) {
				return
		REDACTED
	REDACTED
		// Stream already started, send error as SSE event then close
		flusher, ok := c.Writer.(http.Flusher)
		if ok {
			// SSE 错误事件固定 schema，使用 Quote 直拼可避免额外 Marshal 分配。
			errorEvent := "event: error\ndata: " + `{"error":{"type":` + strconv.Quote(errType) + `,"message":` + strconv.Quote(message) + `REDACTEDREDACTED` + "\n\n"
			if _, err := fmt.Fprint(c.Writer, errorEvent); err != nil {
				_ = c.Error(err)
		REDACTED
			flusher.Flush()
	REDACTED
		return
REDACTED

	// Normal case: return JSON response with proper status code
	h.errorResponse(c, status, errType, message)
REDACTED

// ensureForwardErrorResponse 在 Forward 返回错误但尚未写响应时补写统一错误响应。
func (h *OpenAIGatewayHandler) ensureForwardErrorResponse(c *gin.Context, streamStarted bool) bool {
	if c == nil || c.Writer == nil {
		return false
REDACTED
	// 旧实现在 Writer.Written 时直接 return false，导致 ping 已 flush 之后的
	// 上游错误（http2 timeout、连接中断等）完全无法把错误传给客户端——
	// HTTP 200 已锁死，TCP 直接 EOF，Codex CLI 报 "stream closed before response.completed"。
	// 这里改成：Writer 已写过时强制走 streamStarted 分支，让
	// handleStreamingAwareError 通过 SSE 发协议合规的 response.failed。
	if c.Writer.Written() {
		streamStarted = true
REDACTED
	h.handleStreamingAwareError(c, http.StatusBadGateway, "upstream_error", "Upstream request failed", streamStarted)
	return true
REDACTED

func shouldLogOpenAIForwardFailureAsWarn(c *gin.Context, wroteFallback bool) bool {
	if wroteFallback {
		return false
REDACTED
	if c == nil || c.Writer == nil {
		return false
REDACTED
	return c.Writer.Written()
REDACTED

// errorResponse returns OpenAI API format error response
func (h *OpenAIGatewayHandler) errorResponse(c *gin.Context, status int, errType, message string) {
	c.JSON(status, gin.H{
		"error": gin.H{
			"type":    errType,
			"message": message,
	REDACTED,
REDACTED)
REDACTED

func setOpenAIClientTransportHTTP(c *gin.Context) {
	service.SetOpenAIClientTransport(c, service.OpenAIClientTransportHTTP)
REDACTED

func setOpenAIClientTransportWS(c *gin.Context) {
	service.SetOpenAIClientTransport(c, service.OpenAIClientTransportWS)
REDACTED

func ensureOpenAIPoolModeSessionHash(sessionHash string, account *service.Account) string {
	if sessionHash != "" || account == nil || !account.IsPoolMode() {
		return sessionHash
REDACTED
	// 为当前请求生成一次性粘性会话键，确保同账号重试不会重新负载均衡到其他账号。
	return "openai-pool-retry-" + uuid.NewString()
REDACTED

func openAIWSIngressFallbackSessionSeed(userID, apiKeyID int64, groupID *int64) string {
	gid := int64(0)
	if groupID != nil {
		gid = *groupID
REDACTED
	return fmt.Sprintf("openai_ws_ingress:%d:%d:%d", gid, userID, apiKeyID)
REDACTED

func isOpenAIWSUpgradeRequest(r *http.Request) bool {
	if r == nil {
		return false
REDACTED
	if !strings.EqualFold(strings.TrimSpace(r.Header.Get("Upgrade")), "websocket") {
		return false
REDACTED
	return strings.Contains(strings.ToLower(strings.TrimSpace(r.Header.Get("Connection"))), "upgrade")
REDACTED

func closeOpenAIClientWS(conn *coderws.Conn, status coderws.StatusCode, reason string) {
	if conn == nil {
		return
REDACTED
	reason = strings.TrimSpace(reason)
	if len(reason) > 120 {
		reason = reason[:120]
REDACTED
	_ = conn.Close(status, reason)
	_ = conn.CloseNow()
REDACTED

func closeOpenAIWSFailoverExhausted(conn *coderws.Conn, failoverErr *service.UpstreamFailoverError) {
	if failoverErr == nil {
		closeOpenAIClientWS(conn, coderws.StatusInternalError, "upstream websocket proxy failed")
		return
REDACTED
	switch failoverErr.StatusCode {
	case http.StatusTooManyRequests:
		closeOpenAIClientWS(conn, coderws.StatusTryAgainLater, "upstream rate limit exceeded, please retry later")
	case 529, http.StatusInternalServerError, http.StatusBadGateway, http.StatusServiceUnavailable, http.StatusGatewayTimeout:
		closeOpenAIClientWS(conn, coderws.StatusTryAgainLater, "upstream service temporarily unavailable")
	case http.StatusUnauthorized, http.StatusForbidden:
		closeOpenAIClientWS(conn, coderws.StatusPolicyViolation, "upstream websocket authentication failed")
	default:
		closeOpenAIClientWS(conn, coderws.StatusInternalError, "upstream websocket proxy failed")
REDACTED
REDACTED

func writeContentModerationWSError(ctx context.Context, conn *coderws.Conn, decision *service.ContentModerationDecision) {
	if conn == nil || decision == nil {
		return
REDACTED
	if ctx == nil {
		ctx = context.Background()
REDACTED
	message := strings.TrimSpace(decision.Message)
	if message == "" {
		message = "content moderation blocked this request"
REDACTED
	payload, err := json.Marshal(gin.H{
		"event_id": "evt_content_moderation_blocked",
		"type":     "error",
		"error": gin.H{
			"type":    "invalid_request_error",
			"code":    contentModerationErrorCode(decision),
			"message": message,
	REDACTED,
REDACTED)
	if err != nil {
		payload = []byte(`{"event_id":"evt_content_moderation_blocked","type":"error","error":{"type":"invalid_request_error","code":"content_policy_violation","message":"content moderation blocked this request"REDACTEDREDACTED`)
REDACTED
	writeCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	_ = conn.Write(writeCtx, coderws.MessageText, payload)
REDACTED

func summarizeWSCloseErrorForLog(err error) (string, string) {
	if err == nil {
		return "-", "-"
REDACTED
	statusCode := coderws.CloseStatus(err)
	if statusCode == -1 {
		return "-", "-"
REDACTED
	closeStatus := fmt.Sprintf("%d(%s)", int(statusCode), statusCode.String())
	closeReason := "-"
	var closeErr coderws.CloseError
	if errors.As(err, &closeErr) {
		reason := strings.TrimSpace(closeErr.Reason)
		if reason != "" {
			closeReason = reason
	REDACTED
REDACTED
	return closeStatus, closeReason
REDACTED
