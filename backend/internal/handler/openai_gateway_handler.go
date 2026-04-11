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
	gatewayService          *service.OpenAIGatewayService
	billingCacheService     *service.BillingCacheService
	apiKeyService           *service.APIKeyService
	usageRecordWorkerPool   *service.UsageRecordWorkerPool
	errorPassthroughService *service.ErrorPassthroughService
	concurrencyHelper       *ConcurrencyHelper
	maxAccountSwitches      int
	cfg                     *config.Config
}

func resolveOpenAIForwardDefaultMappedModel(apiKey *service.APIKey, fallbackModel string) string {
	if fallbackModel = strings.TrimSpace(fallbackModel); fallbackModel != "" {
		return fallbackModel
	}
	if apiKey == nil || apiKey.Group == nil {
		return ""
	}
	return strings.TrimSpace(apiKey.Group.DefaultMappedModel)
}

func resolveOpenAIMessagesDispatchMappedModel(apiKey *service.APIKey, requestedModel string) string {
	if apiKey == nil || apiKey.Group == nil {
		return ""
	}
	return strings.TrimSpace(apiKey.Group.ResolveMessagesDispatchModel(requestedModel))
}

// NewOpenAIGatewayHandler creates a new OpenAIGatewayHandler
func NewOpenAIGatewayHandler(
	gatewayService *service.OpenAIGatewayService,
	concurrencyService *service.ConcurrencyService,
	billingCacheService *service.BillingCacheService,
	apiKeyService *service.APIKeyService,
	usageRecordWorkerPool *service.UsageRecordWorkerPool,
	errorPassthroughService *service.ErrorPassthroughService,
	cfg *config.Config,
) *OpenAIGatewayHandler {
	pingInterval := time.Duration(0)
	maxAccountSwitches := 3
	if cfg != nil {
		pingInterval = time.Duration(cfg.Concurrency.PingInterval) * time.Second
		if cfg.Gateway.MaxAccountSwitches > 0 {
			maxAccountSwitches = cfg.Gateway.MaxAccountSwitches
		}
	}
	return &OpenAIGatewayHandler{
		gatewayService:          gatewayService,
		billingCacheService:     billingCacheService,
		apiKeyService:           apiKeyService,
		usageRecordWorkerPool:   usageRecordWorkerPool,
		errorPassthroughService: errorPassthroughService,
		concurrencyHelper:       NewConcurrencyHelper(concurrencyService, SSEPingFormatComment, pingInterval),
		maxAccountSwitches:      maxAccountSwitches,
		cfg:                     cfg,
	}
}

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
	}

	subject, ok := middleware2.GetAuthSubjectFromContext(c)
	if !ok {
		h.errorResponse(c, http.StatusInternalServerError, "api_error", "User context not found")
		return
	}
	reqLog := requestLogger(
		c,
		"handler.openai_gateway.responses",
		zap.Int64("user_id", subject.UserID),
		zap.Int64("api_key_id", apiKey.ID),
		zap.Any("group_id", apiKey.GroupID),
	)
	if !h.ensureResponsesDependencies(c, reqLog) {
		return
	}

	// Read request body
	body, err := pkghttputil.ReadRequestBodyWithPrealloc(c.Request)
	if err != nil {
		if maxErr, ok := extractMaxBytesError(err); ok {
			h.errorResponse(c, http.StatusRequestEntityTooLarge, "invalid_request_error", buildBodyTooLargeMessage(maxErr.Limit))
			return
		}
		h.errorResponse(c, http.StatusBadRequest, "invalid_request_error", "Failed to read request body")
		return
	}

	if len(body) == 0 {
		h.errorResponse(c, http.StatusBadRequest, "invalid_request_error", "Request body is empty")
		return
	}

	setOpsRequestContext(c, "", false, body)
	sessionHashBody := body
	if service.IsOpenAIResponsesCompactPathForTest(c) {
		if compactSeed := strings.TrimSpace(gjson.GetBytes(body, "prompt_cache_key").String()); compactSeed != "" {
			c.Set(service.OpenAICompactSessionSeedKeyForTest(), compactSeed)
		}
		normalizedCompactBody, normalizedCompact, compactErr := service.NormalizeOpenAICompactRequestBodyForTest(body)
		if compactErr != nil {
			h.errorResponse(c, http.StatusBadRequest, "invalid_request_error", "Failed to normalize compact request body")
			return
		}
		if normalizedCompact {
			body = normalizedCompactBody
		}
	}

	// 校验请求体 JSON 合法性
	if !gjson.ValidBytes(body) {
		h.errorResponse(c, http.StatusBadRequest, "invalid_request_error", "Failed to parse request body")
		return
	}

	// 使用 gjson 只读提取字段做校验，避免完整 Unmarshal
	modelResult := gjson.GetBytes(body, "model")
	if !modelResult.Exists() || modelResult.Type != gjson.String || modelResult.String() == "" {
		h.errorResponse(c, http.StatusBadRequest, "invalid_request_error", "model is required")
		return
	}
	reqModel := modelResult.String()

	streamResult := gjson.GetBytes(body, "stream")
	if streamResult.Exists() && streamResult.Type != gjson.True && streamResult.Type != gjson.False {
		h.errorResponse(c, http.StatusBadRequest, "invalid_request_error", "invalid stream field type")
		return
	}
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
		}
	}

	setOpsRequestContext(c, reqModel, reqStream, body)
	setOpsEndpointContext(c, "", int16(service.RequestTypeFromLegacy(reqStream, false)))

	// 解析渠道级模型映射
	channelMapping, _ := h.gatewayService.ResolveChannelMappingAndRestrict(c.Request.Context(), apiKey.GroupID, reqModel)

	// 提前校验 function_call_output 是否具备可关联上下文，避免上游 400。
	if !h.validateFunctionCallOutputRequest(c, body, reqLog) {
		return
	}

	// 绑定错误透传服务，允许 service 层在非 failover 错误场景复用规则。
	if h.errorPassthroughService != nil {
		service.BindErrorPassthroughService(c, h.errorPassthroughService)
	}

	// Get subscription info (may be nil)
	subscription, _ := middleware2.GetSubscriptionFromContext(c)

	service.SetOpsLatencyMs(c, service.OpsAuthLatencyMsKey, time.Since(requestStart).Milliseconds())
	routingStart := time.Now()

	userReleaseFunc, acquired := h.acquireResponsesUserSlot(c, subject.UserID, subject.Concurrency, reqStream, &streamStarted, reqLog)
	if !acquired {
		return
	}
	// 确保请求取消时也会释放槽位，避免长连接被动中断造成泄漏
	if userReleaseFunc != nil {
		defer userReleaseFunc()
	}

	// 2. Re-check billing eligibility after wait
	if err := h.billingCacheService.CheckBillingEligibility(c.Request.Context(), apiKey.User, apiKey, apiKey.Group, subscription); err != nil {
		reqLog.Info("openai.billing_eligibility_check_failed", zap.Error(err))
		status, code, message := billingErrorDetails(err)
		h.handleStreamingAwareError(c, status, code, message, streamStarted)
		return
	}

	// Generate session hash (header first; fallback to prompt_cache_key)
	sessionHash := h.gatewayService.GenerateSessionHash(c, sessionHashBody)

	maxAccountSwitches := h.maxAccountSwitches
	switchCount := 0
	failedAccountIDs := make(map[int64]struct{})
	sameAccountRetryCount := make(map[int64]int)
	var lastFailoverErr *service.UpstreamFailoverError

	for {
		// Select account supporting the requested model
		reqLog.Debug("openai.account_selecting", zap.Int("excluded_account_count", len(failedAccountIDs)))
		selection, scheduleDecision, err := h.gatewayService.SelectAccountWithScheduler(
			c.Request.Context(),
			apiKey.GroupID,
			previousResponseID,
			sessionHash,
			reqModel,
			failedAccountIDs,
			service.OpenAIUpstreamTransportAny,
		)
		if err != nil {
			reqLog.Warn("openai.account_select_failed",
				zap.Error(err),
				zap.Int("excluded_account_count", len(failedAccountIDs)),
			)
			if len(failedAccountIDs) == 0 {
				h.handleStreamingAwareError(c, http.StatusServiceUnavailable, "api_error", "Service temporarily unavailable", streamStarted)
				return
			}
			if lastFailoverErr != nil {
				h.handleFailoverExhausted(c, lastFailoverErr, streamStarted)
			} else {
				h.handleFailoverExhaustedSimple(c, 502, streamStarted)
			}
			return
		}
		if selection == nil || selection.Account == nil {
			h.handleStreamingAwareError(c, http.StatusServiceUnavailable, "api_error", "No available accounts", streamStarted)
			return
		}
		if previousResponseID != "" && selection != nil && selection.Account != nil {
			reqLog.Debug("openai.account_selected_with_previous_response_id", zap.Int64("account_id", selection.Account.ID))
		}
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
		}

		// Forward request
		service.SetOpsLatencyMs(c, service.OpsRoutingLatencyMsKey, time.Since(routingStart).Milliseconds())
		forwardStart := time.Now()
		// 应用渠道模型映射到请求体
		forwardBody := body
		if channelMapping.Mapped {
			forwardBody = h.gatewayService.ReplaceModelInBody(body, channelMapping.MappedModel)
		}
		result, err := h.gatewayService.Forward(c.Request.Context(), c, account, forwardBody)
		forwardDurationMs := time.Since(forwardStart).Milliseconds()
		if accountReleaseFunc != nil {
			accountReleaseFunc()
		}
		upstreamLatencyMs, _ := getContextInt64(c, service.OpsUpstreamLatencyMsKey)
		responseLatencyMs := forwardDurationMs
		if upstreamLatencyMs > 0 && forwardDurationMs > upstreamLatencyMs {
			responseLatencyMs = forwardDurationMs - upstreamLatencyMs
		}
		service.SetOpsLatencyMs(c, service.OpsResponseLatencyMsKey, responseLatencyMs)
		if err == nil && result != nil && result.FirstTokenMs != nil {
			service.SetOpsLatencyMs(c, service.OpsTimeToFirstTokenMsKey, int64(*result.FirstTokenMs))
		}
		if err != nil {
			var failoverErr *service.UpstreamFailoverError
			if errors.As(err, &failoverErr) {
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
						}
						continue
					}
				}
				h.gatewayService.RecordOpenAIAccountSwitch()
				failedAccountIDs[account.ID] = struct{}{}
				lastFailoverErr = failoverErr
				if switchCount >= maxAccountSwitches {
					h.handleFailoverExhausted(c, failoverErr, streamStarted)
					return
				}
				switchCount++
				reqLog.Warn("openai.upstream_failover_switching",
					zap.Int64("account_id", account.ID),
					zap.Int("upstream_status", failoverErr.StatusCode),
					zap.Int("switch_count", switchCount),
					zap.Int("max_switches", maxAccountSwitches),
				)
				continue
			}
			h.gatewayService.ReportOpenAIAccountScheduleResult(account.ID, false, nil)
			wroteFallback := h.ensureForwardErrorResponse(c, streamStarted)
			fields := []zap.Field{
				zap.Int64("account_id", account.ID),
				zap.Bool("fallback_error_response_written", wroteFallback),
				zap.Error(err),
			}
			if shouldLogOpenAIForwardFailureAsWarn(c, wroteFallback) {
				reqLog.Warn("openai.forward_failed", fields...)
				return
			}
			reqLog.Error("openai.forward_failed", fields...)
			return
		}
		if result != nil {
			if account.Type == service.AccountTypeOAuth {
				h.gatewayService.UpdateCodexUsageSnapshotFromHeaders(c.Request.Context(), account.ID, result.ResponseHeaders)
			}
			h.gatewayService.ReportOpenAIAccountScheduleResult(account.ID, true, result.FirstTokenMs)
		} else {
			h.gatewayService.ReportOpenAIAccountScheduleResult(account.ID, true, nil)
		}

		// 捕获请求信息（用于异步记录，避免在 goroutine 中访问 gin.Context）
		userAgent := c.GetHeader("User-Agent")
		clientIP := ip.GetClientIP(c)
		requestPayloadHash := service.HashUsageRequestPayload(body)

		// 使用量记录通过有界 worker 池提交，避免请求热路径创建无界 goroutine。
		h.submitUsageRecordTask(func(ctx context.Context) {
			if err := h.gatewayService.RecordUsage(ctx, &service.OpenAIRecordUsageInput{
				Result:             result,
				APIKey:             apiKey,
				User:               apiKey.User,
				Account:            account,
				Subscription:       subscription,
				InboundEndpoint:    GetInboundEndpoint(c),
				UpstreamEndpoint:   GetUpstreamEndpoint(c, account.Platform),
				UserAgent:          userAgent,
				IPAddress:          clientIP,
				RequestPayloadHash: requestPayloadHash,
				APIKeyService:      h.apiKeyService,
				ChannelUsageFields: channelMapping.ToUsageFields(reqModel, result.UpstreamModel),
			}); err != nil {
				logger.L().With(
					zap.String("component", "handler.openai_gateway.responses"),
					zap.Int64("user_id", subject.UserID),
					zap.Int64("api_key_id", apiKey.ID),
					zap.Any("group_id", apiKey.GroupID),
					zap.String("model", reqModel),
					zap.Int64("account_id", account.ID),
				).Error("openai.record_usage_failed", zap.Error(err))
			}
		})
		reqLog.Debug("openai.request_completed",
			zap.Int64("account_id", account.ID),
			zap.Int("switch_count", switchCount),
		)
		return
	}
}

func isOpenAIRemoteCompactPath(c *gin.Context) bool {
	if c == nil || c.Request == nil || c.Request.URL == nil {
		return false
	}
	normalizedPath := strings.TrimRight(strings.TrimSpace(c.Request.URL.Path), "/")
	return strings.HasSuffix(normalizedPath, "/responses/compact")
}

func (h *OpenAIGatewayHandler) logOpenAIRemoteCompactOutcome(c *gin.Context, startedAt time.Time) {
	if !isOpenAIRemoteCompactPath(c) {
		return
	}

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
			}
		}
		if c.Writer != nil {
			status = c.Writer.Status()
		}
	}

	outcome := "failed"
	if status >= 200 && status < 300 {
		outcome = "succeeded"
	}
	latencyMs := time.Since(startedAt).Milliseconds()
	if latencyMs < 0 {
		latencyMs = 0
	}

	fields := []zap.Field{
		zap.String("component", "handler.openai_gateway.responses"),
		zap.Bool("remote_compact", true),
		zap.String("compact_outcome", outcome),
		zap.Int("status_code", status),
		zap.Int64("latency_ms", latencyMs),
		zap.String("path", path),
		zap.Bool("force_codex_cli", h != nil && h.cfg != nil && h.cfg.Gateway.ForceCodexCLI),
	}

	if c != nil {
		if userAgent := strings.TrimSpace(c.GetHeader("User-Agent")); userAgent != "" {
			fields = append(fields, zap.String("request_user_agent", userAgent))
		}
		if v, ok := c.Get(opsModelKey); ok {
			if model, ok := v.(string); ok && strings.TrimSpace(model) != "" {
				fields = append(fields, zap.String("request_model", strings.TrimSpace(model)))
			}
		}
		if v, ok := c.Get(opsAccountIDKey); ok {
			if accountID, ok := v.(int64); ok && accountID > 0 {
				fields = append(fields, zap.Int64("account_id", accountID))
			}
		}
		if c.Writer != nil {
			if upstreamRequestID := strings.TrimSpace(c.Writer.Header().Get("x-request-id")); upstreamRequestID != "" {
				fields = append(fields, zap.String("upstream_request_id", upstreamRequestID))
			} else if upstreamRequestID := strings.TrimSpace(c.Writer.Header().Get("X-Request-Id")); upstreamRequestID != "" {
				fields = append(fields, zap.String("upstream_request_id", upstreamRequestID))
			}
		}
	}

	log := logger.FromContext(ctx).With(fields...)
	if outcome == "succeeded" {
		log.Info("codex.remote_compact.succeeded")
		return
	}
	log.Warn("codex.remote_compact.failed")
}

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
	}

	subject, ok := middleware2.GetAuthSubjectFromContext(c)
	if !ok {
		h.anthropicErrorResponse(c, http.StatusInternalServerError, "api_error", "User context not found")
		return
	}
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
	}

	if !h.ensureResponsesDependencies(c, reqLog) {
		return
	}

	body, err := pkghttputil.ReadRequestBodyWithPrealloc(c.Request)
	if err != nil {
		if maxErr, ok := extractMaxBytesError(err); ok {
			h.anthropicErrorResponse(c, http.StatusRequestEntityTooLarge, "invalid_request_error", buildBodyTooLargeMessage(maxErr.Limit))
			return
		}
		h.anthropicErrorResponse(c, http.StatusBadRequest, "invalid_request_error", "Failed to read request body")
		return
	}
	if len(body) == 0 {
		h.anthropicErrorResponse(c, http.StatusBadRequest, "invalid_request_error", "Request body is empty")
		return
	}

	if !gjson.ValidBytes(body) {
		h.anthropicErrorResponse(c, http.StatusBadRequest, "invalid_request_error", "Failed to parse request body")
		return
	}

	modelResult := gjson.GetBytes(body, "model")
	if !modelResult.Exists() || modelResult.Type != gjson.String || modelResult.String() == "" {
		h.anthropicErrorResponse(c, http.StatusBadRequest, "invalid_request_error", "model is required")
		return
	}
	reqModel := modelResult.String()
	routingModel := service.NormalizeOpenAICompatRequestedModel(reqModel)
	preferredMappedModel := resolveOpenAIMessagesDispatchMappedModel(apiKey, reqModel)
	reqStream := gjson.GetBytes(body, "stream").Bool()

	reqLog = reqLog.With(zap.String("model", reqModel), zap.Bool("stream", reqStream))

	setOpsRequestContext(c, reqModel, reqStream, body)
	setOpsEndpointContext(c, "", int16(service.RequestTypeFromLegacy(reqStream, false)))

	// 解析渠道级模型映射
	channelMappingMsg, _ := h.gatewayService.ResolveChannelMappingAndRestrict(c.Request.Context(), apiKey.GroupID, reqModel)

	// 绑定错误透传服务，允许 service 层在非 failover 错误场景复用规则。
	if h.errorPassthroughService != nil {
		service.BindErrorPassthroughService(c, h.errorPassthroughService)
	}

	subscription, _ := middleware2.GetSubscriptionFromContext(c)

	service.SetOpsLatencyMs(c, service.OpsAuthLatencyMsKey, time.Since(requestStart).Milliseconds())
	routingStart := time.Now()

	userReleaseFunc, acquired := h.acquireResponsesUserSlot(c, subject.UserID, subject.Concurrency, reqStream, &streamStarted, reqLog)
	if !acquired {
		return
	}
	if userReleaseFunc != nil {
		defer userReleaseFunc()
	}

	if err := h.billingCacheService.CheckBillingEligibility(c.Request.Context(), apiKey.User, apiKey, apiKey.Group, subscription); err != nil {
		reqLog.Info("openai_messages.billing_eligibility_check_failed", zap.Error(err))
		status, code, message := billingErrorDetails(err)
		h.anthropicStreamingAwareError(c, status, code, message, streamStarted)
		return
	}

	sessionHash := h.gatewayService.GenerateSessionHash(c, body)
	promptCacheKey := h.gatewayService.ExtractSessionID(c, body)

	// Anthropic 格式的请求在 metadata.user_id 中携带 session 标识，
	// 而非 OpenAI 的 session_id/conversation_id headers。
	// 从中派生 sessionHash（sticky session）和 promptCacheKey（upstream cache）。
	if sessionHash == "" || promptCacheKey == "" {
		if userID := strings.TrimSpace(gjson.GetBytes(body, "metadata.user_id").String()); userID != "" {
			seed := reqModel + "-" + userID
			if promptCacheKey == "" {
				promptCacheKey = service.GenerateSessionUUID(seed)
			}
			if sessionHash == "" {
				sessionHash = service.DeriveSessionHashFromSeed(seed)
			}
		}
	}

	maxAccountSwitches := h.maxAccountSwitches
	switchCount := 0
	failedAccountIDs := make(map[int64]struct{})
	sameAccountRetryCount := make(map[int64]int)
	var lastFailoverErr *service.UpstreamFailoverError
	effectiveMappedModel := preferredMappedModel

	for {
		currentRoutingModel := routingModel
		if effectiveMappedModel != "" {
			currentRoutingModel = effectiveMappedModel
		}
		reqLog.Debug("openai_messages.account_selecting", zap.Int("excluded_account_count", len(failedAccountIDs)))
		selection, scheduleDecision, err := h.gatewayService.SelectAccountWithScheduler(
			c.Request.Context(),
			apiKey.GroupID,
			"", // no previous_response_id
			sessionHash,
			currentRoutingModel,
			failedAccountIDs,
			service.OpenAIUpstreamTransportAny,
		)
		if err != nil {
			reqLog.Warn("openai_messages.account_select_failed",
				zap.Error(err),
				zap.Int("excluded_account_count", len(failedAccountIDs)),
			)
			if len(failedAccountIDs) == 0 {
				if err != nil {
					h.anthropicStreamingAwareError(c, http.StatusServiceUnavailable, "api_error", "Service temporarily unavailable", streamStarted)
					return
				}
			} else {
				if lastFailoverErr != nil {
					h.handleAnthropicFailoverExhausted(c, lastFailoverErr, streamStarted)
				} else {
					h.anthropicStreamingAwareError(c, http.StatusBadGateway, "api_error", "Upstream request failed", streamStarted)
				}
				return
			}
		}
		if selection == nil || selection.Account == nil {
			h.anthropicStreamingAwareError(c, http.StatusServiceUnavailable, "api_error", "No available accounts", streamStarted)
			return
		}
		account := selection.Account
		sessionHash = ensureOpenAIPoolModeSessionHash(sessionHash, account)
		reqLog.Debug("openai_messages.account_selected", zap.Int64("account_id", account.ID), zap.String("account_name", account.Name))
		_ = scheduleDecision
		setOpsSelectedAccount(c, account.ID, account.Platform)

		accountReleaseFunc, acquired := h.acquireResponsesAccountSlot(c, apiKey.GroupID, sessionHash, selection, reqStream, &streamStarted, reqLog)
		if !acquired {
			return
		}

		service.SetOpsLatencyMs(c, service.OpsRoutingLatencyMsKey, time.Since(routingStart).Milliseconds())
		forwardStart := time.Now()

		defaultMappedModel := strings.TrimSpace(effectiveMappedModel)
		// 应用渠道模型映射到请求体
		forwardBody := body
		if channelMappingMsg.Mapped {
			forwardBody = h.gatewayService.ReplaceModelInBody(body, channelMappingMsg.MappedModel)
		}
		result, err := h.gatewayService.ForwardAsAnthropic(c.Request.Context(), c, account, forwardBody, promptCacheKey, defaultMappedModel)

		forwardDurationMs := time.Since(forwardStart).Milliseconds()
		if accountReleaseFunc != nil {
			accountReleaseFunc()
		}
		upstreamLatencyMs, _ := getContextInt64(c, service.OpsUpstreamLatencyMsKey)
		responseLatencyMs := forwardDurationMs
		if upstreamLatencyMs > 0 && forwardDurationMs > upstreamLatencyMs {
			responseLatencyMs = forwardDurationMs - upstreamLatencyMs
		}
		service.SetOpsLatencyMs(c, service.OpsResponseLatencyMsKey, responseLatencyMs)
		if err == nil && result != nil && result.FirstTokenMs != nil {
			service.SetOpsLatencyMs(c, service.OpsTimeToFirstTokenMsKey, int64(*result.FirstTokenMs))
		}
		if err != nil {
			var failoverErr *service.UpstreamFailoverError
			if errors.As(err, &failoverErr) {
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
						}
						continue
					}
				}
				h.gatewayService.RecordOpenAIAccountSwitch()
				failedAccountIDs[account.ID] = struct{}{}
				lastFailoverErr = failoverErr
				if switchCount >= maxAccountSwitches {
					h.handleAnthropicFailoverExhausted(c, failoverErr, streamStarted)
					return
				}
				switchCount++
				reqLog.Warn("openai_messages.upstream_failover_switching",
					zap.Int64("account_id", account.ID),
					zap.Int("upstream_status", failoverErr.StatusCode),
					zap.Int("switch_count", switchCount),
					zap.Int("max_switches", maxAccountSwitches),
				)
				continue
			}
			h.gatewayService.ReportOpenAIAccountScheduleResult(account.ID, false, nil)
			wroteFallback := h.ensureAnthropicErrorResponse(c, streamStarted)
			reqLog.Warn("openai_messages.forward_failed",
				zap.Int64("account_id", account.ID),
				zap.Bool("fallback_error_response_written", wroteFallback),
				zap.Error(err),
			)
			return
		}
		if result != nil {
			h.gatewayService.ReportOpenAIAccountScheduleResult(account.ID, true, result.FirstTokenMs)
		} else {
			h.gatewayService.ReportOpenAIAccountScheduleResult(account.ID, true, nil)
		}

		userAgent := c.GetHeader("User-Agent")
		clientIP := ip.GetClientIP(c)
		requestPayloadHash := service.HashUsageRequestPayload(body)

		h.submitUsageRecordTask(func(ctx context.Context) {
			if err := h.gatewayService.RecordUsage(ctx, &service.OpenAIRecordUsageInput{
				Result:             result,
				APIKey:             apiKey,
				User:               apiKey.User,
				Account:            account,
				Subscription:       subscription,
				InboundEndpoint:    GetInboundEndpoint(c),
				UpstreamEndpoint:   GetUpstreamEndpoint(c, account.Platform),
				UserAgent:          userAgent,
				IPAddress:          clientIP,
				RequestPayloadHash: requestPayloadHash,
				APIKeyService:      h.apiKeyService,
				ChannelUsageFields: channelMappingMsg.ToUsageFields(reqModel, result.UpstreamModel),
			}); err != nil {
				logger.L().With(
					zap.String("component", "handler.openai_gateway.messages"),
					zap.Int64("user_id", subject.UserID),
					zap.Int64("api_key_id", apiKey.ID),
					zap.Any("group_id", apiKey.GroupID),
					zap.String("model", reqModel),
					zap.Int64("account_id", account.ID),
				).Error("openai_messages.record_usage_failed", zap.Error(err))
			}
		})
		reqLog.Debug("openai_messages.request_completed",
			zap.Int64("account_id", account.ID),
			zap.Int("switch_count", switchCount),
		)
		return
	}
}

// anthropicErrorResponse writes an error in Anthropic Messages API format.
func (h *OpenAIGatewayHandler) anthropicErrorResponse(c *gin.Context, status int, errType, message string) {
	c.JSON(status, gin.H{
		"type": "error",
		"error": gin.H{
			"type":    errType,
			"message": message,
		},
	})
}

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
				},
			})
			fmt.Fprintf(c.Writer, "event: error\ndata: %s\n\n", errPayload) //nolint:errcheck
			flusher.Flush()
		}
		return
	}
	h.anthropicErrorResponse(c, status, errType, message)
}

// handleAnthropicFailoverExhausted maps upstream failover errors to Anthropic format.
func (h *OpenAIGatewayHandler) handleAnthropicFailoverExhausted(c *gin.Context, failoverErr *service.UpstreamFailoverError, streamStarted bool) {
	status, errType, errMsg := h.mapUpstreamError(failoverErr.StatusCode)
	h.anthropicStreamingAwareError(c, status, errType, errMsg, streamStarted)
}

// ensureAnthropicErrorResponse writes a fallback Anthropic error if no response was written.
func (h *OpenAIGatewayHandler) ensureAnthropicErrorResponse(c *gin.Context, streamStarted bool) bool {
	if c == nil || c.Writer == nil || c.Writer.Written() {
		return false
	}
	h.anthropicStreamingAwareError(c, http.StatusBadGateway, "api_error", "Upstream request failed", streamStarted)
	return true
}

func (h *OpenAIGatewayHandler) validateFunctionCallOutputRequest(c *gin.Context, body []byte, reqLog *zap.Logger) bool {
	if !gjson.GetBytes(body, `input.#(type=="function_call_output")`).Exists() {
		return true
	}

	var reqBody map[string]any
	if err := json.Unmarshal(body, &reqBody); err != nil {
		// 保持原有容错语义：解析失败时跳过预校验，沿用后续上游校验结果。
		return true
	}

	c.Set(service.OpenAIParsedRequestBodyKey, reqBody)
	validation := service.ValidateFunctionCallOutputContext(reqBody)
	if !validation.HasFunctionCallOutput {
		return true
	}

	previousResponseID, _ := reqBody["previous_response_id"].(string)
	if strings.TrimSpace(previousResponseID) != "" || validation.HasToolCallContext {
		return true
	}

	if validation.HasFunctionCallOutputMissingCallID {
		reqLog.Warn("openai.request_validation_failed",
			zap.String("reason", "function_call_output_missing_call_id"),
		)
		h.errorResponse(c, http.StatusBadRequest, "invalid_request_error", "function_call_output requires call_id or previous_response_id; if relying on history, ensure store=true and reuse previous_response_id")
		return false
	}
	if validation.HasItemReferenceForAllCallIDs {
		return true
	}

	reqLog.Warn("openai.request_validation_failed",
		zap.String("reason", "function_call_output_missing_item_reference"),
	)
	h.errorResponse(c, http.StatusBadRequest, "invalid_request_error", "function_call_output requires item_reference ids matching each call_id, or previous_response_id/tool_call context; if relying on history, ensure store=true and reuse previous_response_id")
	return false
}

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
	}
	if userAcquired {
		return wrapReleaseOnDone(ctx, userReleaseFunc), true
	}

	maxWait := service.CalculateMaxWait(userConcurrency)
	canWait, waitErr := h.concurrencyHelper.IncrementWaitCount(ctx, userID, maxWait)
	if waitErr != nil {
		reqLog.Warn("openai.user_wait_counter_increment_failed", zap.Error(waitErr))
		// 按现有降级语义：等待计数异常时放行后续抢槽流程
	} else if !canWait {
		reqLog.Info("openai.user_wait_queue_full", zap.Int("max_wait", maxWait))
		h.errorResponse(c, http.StatusTooManyRequests, "rate_limit_error", "Too many pending requests, please retry later")
		return nil, false
	}

	waitCounted := waitErr == nil && canWait
	defer func() {
		if waitCounted {
			h.concurrencyHelper.DecrementWaitCount(ctx, userID)
		}
	}()

	userReleaseFunc, err = h.concurrencyHelper.AcquireUserSlotWithWait(c, userID, userConcurrency, reqStream, streamStarted)
	if err != nil {
		reqLog.Warn("openai.user_slot_acquire_failed_after_wait", zap.Error(err))
		h.handleConcurrencyError(c, err, "user", *streamStarted)
		return nil, false
	}

	// 槽位获取成功后，立刻退出等待计数。
	if waitCounted {
		h.concurrencyHelper.DecrementWaitCount(ctx, userID)
		waitCounted = false
	}
	return wrapReleaseOnDone(ctx, userReleaseFunc), true
}

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
		h.handleStreamingAwareError(c, http.StatusServiceUnavailable, "api_error", "No available accounts", *streamStarted)
		return nil, false
	}

	ctx := c.Request.Context()
	account := selection.Account
	if selection.Acquired {
		return wrapReleaseOnDone(ctx, selection.ReleaseFunc), true
	}
	if selection.WaitPlan == nil {
		h.handleStreamingAwareError(c, http.StatusServiceUnavailable, "api_error", "No available accounts", *streamStarted)
		return nil, false
	}

	fastReleaseFunc, fastAcquired, err := h.concurrencyHelper.TryAcquireAccountSlot(
		ctx,
		account.ID,
		selection.WaitPlan.MaxConcurrency,
	)
	if err != nil {
		reqLog.Warn("openai.account_slot_quick_acquire_failed", zap.Int64("account_id", account.ID), zap.Error(err))
		h.handleConcurrencyError(c, err, "account", *streamStarted)
		return nil, false
	}
	if fastAcquired {
		if err := h.gatewayService.BindStickySession(ctx, groupID, sessionHash, account.ID); err != nil {
			reqLog.Warn("openai.bind_sticky_session_failed", zap.Int64("account_id", account.ID), zap.Error(err))
		}
		return wrapReleaseOnDone(ctx, fastReleaseFunc), true
	}

	canWait, waitErr := h.concurrencyHelper.IncrementAccountWaitCount(ctx, account.ID, selection.WaitPlan.MaxWaiting)
	if waitErr != nil {
		reqLog.Warn("openai.account_wait_counter_increment_failed", zap.Int64("account_id", account.ID), zap.Error(waitErr))
	} else if !canWait {
		reqLog.Info("openai.account_wait_queue_full",
			zap.Int64("account_id", account.ID),
			zap.Int("max_waiting", selection.WaitPlan.MaxWaiting),
		)
		h.handleStreamingAwareError(c, http.StatusTooManyRequests, "rate_limit_error", "Too many pending requests, please retry later", *streamStarted)
		return nil, false
	}

	accountWaitCounted := waitErr == nil && canWait
	releaseWait := func() {
		if accountWaitCounted {
			h.concurrencyHelper.DecrementAccountWaitCount(ctx, account.ID)
			accountWaitCounted = false
		}
	}
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
	}

	// Slot acquired: no longer waiting in queue.
	releaseWait()
	if err := h.gatewayService.BindStickySession(ctx, groupID, sessionHash, account.ID); err != nil {
		reqLog.Warn("openai.bind_sticky_session_failed", zap.Int64("account_id", account.ID), zap.Error(err))
	}
	return wrapReleaseOnDone(ctx, accountReleaseFunc), true
}

// ResponsesWebSocket handles OpenAI Responses API WebSocket ingress endpoint
// GET /openai/v1/responses (Upgrade: websocket)
func (h *OpenAIGatewayHandler) ResponsesWebSocket(c *gin.Context) {
	if !isOpenAIWSUpgradeRequest(c.Request) {
		h.errorResponse(c, http.StatusUpgradeRequired, "invalid_request_error", "WebSocket upgrade required (Upgrade: websocket)")
		return
	}
	setOpenAIClientTransportWS(c)

	apiKey, ok := middleware2.GetAPIKeyFromContext(c)
	if !ok {
		h.errorResponse(c, http.StatusUnauthorized, "authentication_error", "Invalid API key")
		return
	}
	subject, ok := middleware2.GetAuthSubjectFromContext(c)
	if !ok {
		h.errorResponse(c, http.StatusInternalServerError, "api_error", "User context not found")
		return
	}

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
	}
	reqLog.Info("openai.websocket_ingress_started")
	clientIP := ip.GetClientIP(c)
	userAgent := strings.TrimSpace(c.GetHeader("User-Agent"))

	wsConn, err := coderws.Accept(c.Writer, c.Request, &coderws.AcceptOptions{
		CompressionMode: coderws.CompressionContextTakeover,
	})
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
	}
	defer func() {
		_ = wsConn.CloseNow()
	}()
	wsConn.SetReadLimit(16 * 1024 * 1024)

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
	}
	if msgType != coderws.MessageText && msgType != coderws.MessageBinary {
		closeOpenAIClientWS(wsConn, coderws.StatusPolicyViolation, "unsupported websocket message type")
		return
	}
	if !gjson.ValidBytes(firstMessage) {
		closeOpenAIClientWS(wsConn, coderws.StatusPolicyViolation, "invalid JSON payload")
		return
	}

	reqModel := strings.TrimSpace(gjson.GetBytes(firstMessage, "model").String())
	if reqModel == "" {
		closeOpenAIClientWS(wsConn, coderws.StatusPolicyViolation, "model is required in first response.create payload")
		return
	}
	previousResponseID := strings.TrimSpace(gjson.GetBytes(firstMessage, "previous_response_id").String())
	previousResponseIDKind := service.ClassifyOpenAIPreviousResponseIDKind(previousResponseID)
	if previousResponseID != "" && previousResponseIDKind == service.OpenAIPreviousResponseIDKindMessageID {
		closeOpenAIClientWS(wsConn, coderws.StatusPolicyViolation, "previous_response_id must be a response.id (resp_*), not a message id")
		return
	}
	reqLog = reqLog.With(
		zap.Bool("ws_ingress", true),
		zap.String("model", reqModel),
		zap.Bool("has_previous_response_id", previousResponseID != ""),
		zap.String("previous_response_id_kind", previousResponseIDKind),
	)
	setOpsRequestContext(c, reqModel, true, firstMessage)
	setOpsEndpointContext(c, "", int16(service.RequestTypeWSV2))

	// 解析渠道级模型映射
	channelMappingWS, _ := h.gatewayService.ResolveChannelMappingAndRestrict(ctx, apiKey.GroupID, reqModel)

	var currentUserRelease func()
	var currentAccountRelease func()
	releaseTurnSlots := func() {
		if currentAccountRelease != nil {
			currentAccountRelease()
			currentAccountRelease = nil
		}
		if currentUserRelease != nil {
			currentUserRelease()
			currentUserRelease = nil
		}
	}
	// 必须尽早注册，确保任何 early return 都能释放已获取的并发槽位。
	defer releaseTurnSlots()

	userReleaseFunc, userAcquired, err := h.concurrencyHelper.TryAcquireUserSlot(ctx, subject.UserID, subject.Concurrency)
	if err != nil {
		reqLog.Warn("openai.websocket_user_slot_acquire_failed", zap.Error(err))
		closeOpenAIClientWS(wsConn, coderws.StatusInternalError, "failed to acquire user concurrency slot")
		return
	}
	if !userAcquired {
		closeOpenAIClientWS(wsConn, coderws.StatusTryAgainLater, "too many concurrent requests, please retry later")
		return
	}
	currentUserRelease = wrapReleaseOnDone(ctx, userReleaseFunc)

	subscription, _ := middleware2.GetSubscriptionFromContext(c)
	if err := h.billingCacheService.CheckBillingEligibility(ctx, apiKey.User, apiKey, apiKey.Group, subscription); err != nil {
		reqLog.Info("openai.websocket_billing_eligibility_check_failed", zap.Error(err))
		closeOpenAIClientWS(wsConn, coderws.StatusPolicyViolation, "billing check failed")
		return
	}

	sessionHash := h.gatewayService.GenerateSessionHashWithFallback(
		c,
		firstMessage,
		openAIWSIngressFallbackSessionSeed(subject.UserID, apiKey.ID, apiKey.GroupID),
	)
	selection, scheduleDecision, err := h.gatewayService.SelectAccountWithScheduler(
		ctx,
		apiKey.GroupID,
		previousResponseID,
		sessionHash,
		reqModel,
		nil,
		service.OpenAIUpstreamTransportResponsesWebsocketV2,
	)
	if err != nil {
		reqLog.Warn("openai.websocket_account_select_failed", zap.Error(err))
		closeOpenAIClientWS(wsConn, coderws.StatusTryAgainLater, "no available account")
		return
	}
	if selection == nil || selection.Account == nil {
		closeOpenAIClientWS(wsConn, coderws.StatusTryAgainLater, "no available account")
		return
	}

	account := selection.Account
	accountMaxConcurrency := account.Concurrency
	if selection.WaitPlan != nil && selection.WaitPlan.MaxConcurrency > 0 {
		accountMaxConcurrency = selection.WaitPlan.MaxConcurrency
	}
	accountReleaseFunc := selection.ReleaseFunc
	if !selection.Acquired {
		if selection.WaitPlan == nil {
			closeOpenAIClientWS(wsConn, coderws.StatusTryAgainLater, "account is busy, please retry later")
			return
		}
		fastReleaseFunc, fastAcquired, err := h.concurrencyHelper.TryAcquireAccountSlot(
			ctx,
			account.ID,
			selection.WaitPlan.MaxConcurrency,
		)
		if err != nil {
			reqLog.Warn("openai.websocket_account_slot_acquire_failed", zap.Int64("account_id", account.ID), zap.Error(err))
			closeOpenAIClientWS(wsConn, coderws.StatusInternalError, "failed to acquire account concurrency slot")
			return
		}
		if !fastAcquired {
			closeOpenAIClientWS(wsConn, coderws.StatusTryAgainLater, "account is busy, please retry later")
			return
		}
		accountReleaseFunc = fastReleaseFunc
	}
	currentAccountRelease = wrapReleaseOnDone(ctx, accountReleaseFunc)
	if err := h.gatewayService.BindStickySession(ctx, apiKey.GroupID, sessionHash, account.ID); err != nil {
		reqLog.Warn("openai.websocket_bind_sticky_session_failed", zap.Int64("account_id", account.ID), zap.Error(err))
	}

	token, _, err := h.gatewayService.GetAccessToken(ctx, account)
	if err != nil {
		reqLog.Warn("openai.websocket_get_access_token_failed", zap.Int64("account_id", account.ID), zap.Error(err))
		closeOpenAIClientWS(wsConn, coderws.StatusInternalError, "failed to get access token")
		return
	}

	reqLog.Debug("openai.websocket_account_selected",
		zap.Int64("account_id", account.ID),
		zap.String("account_name", account.Name),
		zap.String("schedule_layer", scheduleDecision.Layer),
		zap.Int("candidate_count", scheduleDecision.CandidateCount),
	)

	hooks := &service.OpenAIWSIngressHooks{
		BeforeTurn: func(turn int) error {
			if turn == 1 {
				return nil
			}
			// 防御式清理：避免异常路径下旧槽位覆盖导致泄漏。
			releaseTurnSlots()
			// 非首轮 turn 需要重新抢占并发槽位，避免长连接空闲占槽。
			userReleaseFunc, userAcquired, err := h.concurrencyHelper.TryAcquireUserSlot(ctx, subject.UserID, subject.Concurrency)
			if err != nil {
				return service.NewOpenAIWSClientCloseError(coderws.StatusInternalError, "failed to acquire user concurrency slot", err)
			}
			if !userAcquired {
				return service.NewOpenAIWSClientCloseError(coderws.StatusTryAgainLater, "too many concurrent requests, please retry later", nil)
			}
			accountReleaseFunc, accountAcquired, err := h.concurrencyHelper.TryAcquireAccountSlot(ctx, account.ID, accountMaxConcurrency)
			if err != nil {
				if userReleaseFunc != nil {
					userReleaseFunc()
				}
				return service.NewOpenAIWSClientCloseError(coderws.StatusInternalError, "failed to acquire account concurrency slot", err)
			}
			if !accountAcquired {
				if userReleaseFunc != nil {
					userReleaseFunc()
				}
				return service.NewOpenAIWSClientCloseError(coderws.StatusTryAgainLater, "account is busy, please retry later", nil)
			}
			currentUserRelease = wrapReleaseOnDone(ctx, userReleaseFunc)
			currentAccountRelease = wrapReleaseOnDone(ctx, accountReleaseFunc)
			return nil
		},
		AfterTurn: func(turn int, result *service.OpenAIForwardResult, turnErr error) {
			releaseTurnSlots()
			if turnErr != nil || result == nil {
				return
			}
			if account.Type == service.AccountTypeOAuth {
				h.gatewayService.UpdateCodexUsageSnapshotFromHeaders(ctx, account.ID, result.ResponseHeaders)
			}
			h.gatewayService.ReportOpenAIAccountScheduleResult(account.ID, true, result.FirstTokenMs)
			h.submitUsageRecordTask(func(taskCtx context.Context) {
				if err := h.gatewayService.RecordUsage(taskCtx, &service.OpenAIRecordUsageInput{
					Result:             result,
					APIKey:             apiKey,
					User:               apiKey.User,
					Account:            account,
					Subscription:       subscription,
					InboundEndpoint:    GetInboundEndpoint(c),
					UpstreamEndpoint:   GetUpstreamEndpoint(c, account.Platform),
					UserAgent:          userAgent,
					IPAddress:          clientIP,
					RequestPayloadHash: service.HashUsageRequestPayload(firstMessage),
					APIKeyService:      h.apiKeyService,
					ChannelUsageFields: channelMappingWS.ToUsageFields(reqModel, result.UpstreamModel),
				}); err != nil {
					reqLog.Error("openai.websocket_record_usage_failed",
						zap.Int64("account_id", account.ID),
						zap.String("request_id", result.RequestID),
						zap.Error(err),
					)
				}
			})
		},
	}

	// 应用渠道模型映射到 WebSocket 首条消息
	wsFirstMessage := firstMessage
	if channelMappingWS.Mapped {
		wsFirstMessage = h.gatewayService.ReplaceModelInBody(firstMessage, channelMappingWS.MappedModel)
	}

	if err := h.gatewayService.ProxyResponsesWebSocketFromClient(ctx, c, wsConn, account, token, wsFirstMessage, hooks); err != nil {
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
		}
		closeOpenAIClientWS(wsConn, coderws.StatusInternalError, "upstream websocket proxy failed")
		return
	}
	reqLog.Info("openai.websocket_ingress_closed", zap.Int64("account_id", account.ID))
}

func (h *OpenAIGatewayHandler) recoverResponsesPanic(c *gin.Context, streamStarted *bool) {
	recovered := recover()
	if recovered == nil {
		return
	}

	started := false
	if streamStarted != nil {
		started = *streamStarted
	}
	wroteFallback := h.ensureForwardErrorResponse(c, started)
	requestLogger(c, "handler.openai_gateway.responses").Error(
		"openai.responses_panic_recovered",
		zap.Bool("fallback_error_response_written", wroteFallback),
		zap.Any("panic", recovered),
		zap.ByteString("stack", debug.Stack()),
	)
}

// recoverAnthropicMessagesPanic recovers from panics in the Anthropic Messages
// handler and returns an Anthropic-formatted error response.
func (h *OpenAIGatewayHandler) recoverAnthropicMessagesPanic(c *gin.Context, streamStarted *bool) {
	recovered := recover()
	if recovered == nil {
		return
	}

	started := streamStarted != nil && *streamStarted
	requestLogger(c, "handler.openai_gateway.messages").Error(
		"openai.messages_panic_recovered",
		zap.Bool("stream_started", started),
		zap.Any("panic", recovered),
		zap.ByteString("stack", debug.Stack()),
	)
	if !started {
		h.anthropicErrorResponse(c, http.StatusInternalServerError, "api_error", "Internal server error")
	}
}

func (h *OpenAIGatewayHandler) ensureResponsesDependencies(c *gin.Context, reqLog *zap.Logger) bool {
	missing := h.missingResponsesDependencies()
	if len(missing) == 0 {
		return true
	}

	if reqLog == nil {
		reqLog = requestLogger(c, "handler.openai_gateway.responses")
	}
	reqLog.Error("openai.handler_dependencies_missing", zap.Strings("missing_dependencies", missing))

	if c != nil && c.Writer != nil && !c.Writer.Written() {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"error": gin.H{
				"type":    "api_error",
				"message": "Service temporarily unavailable",
			},
		})
	}
	return false
}

func (h *OpenAIGatewayHandler) missingResponsesDependencies() []string {
	missing := make([]string, 0, 5)
	if h == nil {
		return append(missing, "handler")
	}
	if h.gatewayService == nil {
		missing = append(missing, "gatewayService")
	}
	if h.billingCacheService == nil {
		missing = append(missing, "billingCacheService")
	}
	if h.apiKeyService == nil {
		missing = append(missing, "apiKeyService")
	}
	if h.concurrencyHelper == nil || h.concurrencyHelper.concurrencyService == nil {
		missing = append(missing, "concurrencyHelper")
	}
	return missing
}

func getContextInt64(c *gin.Context, key string) (int64, bool) {
	if c == nil || key == "" {
		return 0, false
	}
	v, ok := c.Get(key)
	if !ok {
		return 0, false
	}
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
	}
}

func (h *OpenAIGatewayHandler) submitUsageRecordTask(task service.UsageRecordTask) {
	if task == nil {
		return
	}
	if h.usageRecordWorkerPool != nil {
		h.usageRecordWorkerPool.Submit(task)
		return
	}
	// 回退路径：worker 池未注入时同步执行，避免退回到无界 goroutine 模式。
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	defer func() {
		if recovered := recover(); recovered != nil {
			logger.L().With(
				zap.String("component", "handler.openai_gateway.responses"),
				zap.Any("panic", recovered),
			).Error("openai.usage_record_task_panic_recovered")
		}
	}()
	task(ctx)
}

// handleConcurrencyError handles concurrency-related errors with proper 429 response
func (h *OpenAIGatewayHandler) handleConcurrencyError(c *gin.Context, err error, slotType string, streamStarted bool) {
	h.handleStreamingAwareError(c, http.StatusTooManyRequests, "rate_limit_error",
		fmt.Sprintf("Concurrency limit exceeded for %s, please retry later", slotType), streamStarted)
}

func (h *OpenAIGatewayHandler) handleFailoverExhausted(c *gin.Context, failoverErr *service.UpstreamFailoverError, streamStarted bool) {
	statusCode := failoverErr.StatusCode
	responseBody := failoverErr.ResponseBody

	// 先检查透传规则
	if h.errorPassthroughService != nil && len(responseBody) > 0 {
		if rule := h.errorPassthroughService.MatchRule("openai", statusCode, responseBody); rule != nil {
			// 确定响应状态码
			respCode := statusCode
			if !rule.PassthroughCode && rule.ResponseCode != nil {
				respCode = *rule.ResponseCode
			}

			// 确定响应消息
			msg := service.ExtractUpstreamErrorMessage(responseBody)
			if !rule.PassthroughBody && rule.CustomMessage != nil {
				msg = *rule.CustomMessage
			}

			if rule.SkipMonitoring {
				c.Set(service.OpsSkipPassthroughKey, true)
			}

			h.handleStreamingAwareError(c, respCode, "upstream_error", msg, streamStarted)
			return
		}
	}

	// 记录原始上游状态码，以便 ops 错误日志捕获真实的上游错误
	upstreamMsg := service.ExtractUpstreamErrorMessage(responseBody)
	service.SetOpsUpstreamError(c, statusCode, upstreamMsg, "")

	// 使用默认的错误映射
	status, errType, errMsg := h.mapUpstreamError(statusCode)
	h.handleStreamingAwareError(c, status, errType, errMsg, streamStarted)
}

// handleFailoverExhaustedSimple 简化版本，用于没有响应体的情况
func (h *OpenAIGatewayHandler) handleFailoverExhaustedSimple(c *gin.Context, statusCode int, streamStarted bool) {
	status, errType, errMsg := h.mapUpstreamError(statusCode)
	service.SetOpsUpstreamError(c, statusCode, errMsg, "")
	h.handleStreamingAwareError(c, status, errType, errMsg, streamStarted)
}

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
	}
}

// handleStreamingAwareError handles errors that may occur after streaming has started
func (h *OpenAIGatewayHandler) handleStreamingAwareError(c *gin.Context, status int, errType, message string, streamStarted bool) {
	if streamStarted {
		// Stream already started, send error as SSE event then close
		flusher, ok := c.Writer.(http.Flusher)
		if ok {
			// SSE 错误事件固定 schema，使用 Quote 直拼可避免额外 Marshal 分配。
			errorEvent := "event: error\ndata: " + `{"error":{"type":` + strconv.Quote(errType) + `,"message":` + strconv.Quote(message) + `}}` + "\n\n"
			if _, err := fmt.Fprint(c.Writer, errorEvent); err != nil {
				_ = c.Error(err)
			}
			flusher.Flush()
		}
		return
	}

	// Normal case: return JSON response with proper status code
	h.errorResponse(c, status, errType, message)
}

// ensureForwardErrorResponse 在 Forward 返回错误但尚未写响应时补写统一错误响应。
func (h *OpenAIGatewayHandler) ensureForwardErrorResponse(c *gin.Context, streamStarted bool) bool {
	if c == nil || c.Writer == nil || c.Writer.Written() {
		return false
	}
	h.handleStreamingAwareError(c, http.StatusBadGateway, "upstream_error", "Upstream request failed", streamStarted)
	return true
}

func shouldLogOpenAIForwardFailureAsWarn(c *gin.Context, wroteFallback bool) bool {
	if wroteFallback {
		return false
	}
	if c == nil || c.Writer == nil {
		return false
	}
	return c.Writer.Written()
}

// errorResponse returns OpenAI API format error response
func (h *OpenAIGatewayHandler) errorResponse(c *gin.Context, status int, errType, message string) {
	c.JSON(status, gin.H{
		"error": gin.H{
			"type":    errType,
			"message": message,
		},
	})
}

func setOpenAIClientTransportHTTP(c *gin.Context) {
	service.SetOpenAIClientTransport(c, service.OpenAIClientTransportHTTP)
}

func setOpenAIClientTransportWS(c *gin.Context) {
	service.SetOpenAIClientTransport(c, service.OpenAIClientTransportWS)
}

func ensureOpenAIPoolModeSessionHash(sessionHash string, account *service.Account) string {
	if sessionHash != "" || account == nil || !account.IsPoolMode() {
		return sessionHash
	}
	// 为当前请求生成一次性粘性会话键，确保同账号重试不会重新负载均衡到其他账号。
	return "openai-pool-retry-" + uuid.NewString()
}

func openAIWSIngressFallbackSessionSeed(userID, apiKeyID int64, groupID *int64) string {
	gid := int64(0)
	if groupID != nil {
		gid = *groupID
	}
	return fmt.Sprintf("openai_ws_ingress:%d:%d:%d", gid, userID, apiKeyID)
}

func isOpenAIWSUpgradeRequest(r *http.Request) bool {
	if r == nil {
		return false
	}
	if !strings.EqualFold(strings.TrimSpace(r.Header.Get("Upgrade")), "websocket") {
		return false
	}
	return strings.Contains(strings.ToLower(strings.TrimSpace(r.Header.Get("Connection"))), "upgrade")
}

func closeOpenAIClientWS(conn *coderws.Conn, status coderws.StatusCode, reason string) {
	if conn == nil {
		return
	}
	reason = strings.TrimSpace(reason)
	if len(reason) > 120 {
		reason = reason[:120]
	}
	_ = conn.Close(status, reason)
	_ = conn.CloseNow()
}

func summarizeWSCloseErrorForLog(err error) (string, string) {
	if err == nil {
		return "-", "-"
	}
	statusCode := coderws.CloseStatus(err)
	if statusCode == -1 {
		return "-", "-"
	}
	closeStatus := fmt.Sprintf("%d(%s)", int(statusCode), statusCode.String())
	closeReason := "-"
	var closeErr coderws.CloseError
	if errors.As(err, &closeErr) {
		reason := strings.TrimSpace(closeErr.Reason)
		if reason != "" {
			closeReason = reason
		}
	}
	return closeStatus, closeReason
}
