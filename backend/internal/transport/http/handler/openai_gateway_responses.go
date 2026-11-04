package handler

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/application/service"
	"github.com/Wei-Shaw/sub2api/internal/shared/apicompat"
	"github.com/Wei-Shaw/sub2api/internal/shared/ip"
	"github.com/Wei-Shaw/sub2api/internal/shared/logger"
	middleware2 "github.com/Wei-Shaw/sub2api/internal/transport/http/server/middleware"

	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
	"go.uber.org/zap"
)

func shouldApplyCodexContinuationAffinity(forceImageTool bool, requestPlatform string) bool {
	return !forceImageTool && requestPlatform == service.PlatformOpenAI
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
	service.BeginOpenAIRequestFirstEventTTFT(c, requestStart)

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
	body, err := readLenientJSONRequestBodyWithPrealloc(c.Request, h.cfg)
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

	setOpsRequestContext(c, "", false)
	sessionHashBody := body
	body, compactionRoute, ok := h.normalizeOpenAIResponsesCompactRequest(c, reqLog, body)
	if !ok {
		return
	}
	h.concurrencyHelper.SetPriorityAdmissionPendingBytes(c, int64(len(body)))
	// body-signal compact：上游 unary 等待期间向下游发 SSE 注释行心跳，防止
	// 反向代理空闲超时掐断长压缩连接（#3887）。首拍延迟一个心跳间隔，快速
	// 失败仍走 JSON+状态码链路；未标记客户端流式或间隔为 0 时是 no-op。
	stopCompactKeepalive := service.StartOpenAICompactSSEKeepalive(c, h.openAICompactKeepaliveInterval(c))
	defer stopCompactKeepalive()

	// 校验请求体 JSON 合法性
	if !gjson.ValidBytes(body) {
		logRequestBodyParseFailure(reqLog, body, nil)
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
	ensureCompositeTargetPlatform(c, apiKey, reqModel)
	if !compositeTargetPlatformAllowed(c, apiKey, reqModel,
		service.PlatformOpenAI, service.PlatformGrok,
		service.PlatformKimi, service.PlatformZhipu, service.PlatformDeepseek, service.PlatformMiniMax,
		service.PlatformOpenCodeGo,
	) {
		h.errorResponse(c, http.StatusBadRequest, "invalid_request_error", "Model is not supported by this OpenAI-compatible endpoint for composite groups")
		return
	}
	if cappedBody, changed := applyOpenAIReasoningEffortPolicyForRequest(c, apiKey, body); changed {
		body = cappedBody
	}
	if normalizedBody, bootstrapKind, changed := apicompat.NormalizeCodexCallOutputBootstrap(body); changed {
		body = normalizedBody
		reqLog.Info("openai.codex_bootstrap_normalized",
			zap.String("kind", bootstrapKind),
			zap.String("normalization", "call_output_to_user_message"),
		)
	}

	reqStream, ok := parseOpenAICompatibleStream(body)
	if !ok {
		h.errorResponse(c, http.StatusBadRequest, "invalid_request_error", invalidStreamFieldTypeMessage)
		return
	}
	forceImageTool := groupForcesOpenAIImageTool(c, apiKey) && isBareOpenAIResponsesPath(c)
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
		if !forceImageTool {
			reqLog.Warn("openai.request_validation_failed",
				zap.String("reason", "previous_response_id_requires_wsv2"),
			)
			h.errorResponse(c, http.StatusBadRequest, "invalid_request_error", "previous_response_id is only supported on Responses WebSocket v2")
			return
		}
	}

	setOpsRequestContext(c, reqModel, reqStream)
	setOpsEndpointContext(c, "", int16(service.RequestTypeFromLegacy(reqStream, false)))
	pricingCtx, pricingAt := h.gatewayService.WithOpenAIRequestPricingContext(c.Request.Context(), apiKey.GroupID)
	c.Request = c.Request.WithContext(pricingCtx)

	if decision := h.checkSecurityAudit(c, reqLog, apiKey, subject, service.ContentModerationProtocolOpenAIResponses, reqModel, body); decision != nil && !decision.AllowNextStage {
		h.openAISecurityAuditError(c, decision)
		return
	}

	// 使用 IsExplicitImageGenerationIntent 排除被动 image_gen namespace 声明。
	// Codex 在所有请求中被动声明 image_gen namespace，宽泛检测会导致禁了生图的
	// 分组中所有 Codex 请求被 403（#4447），并误占生图并发槽位。
	imageIntent := forceImageTool || service.IsExplicitImageGenerationIntent("/v1/responses", reqModel, body)
	if imageIntent && !service.GroupAllowsImageGeneration(apiKey.Group) {
		h.errorResponse(c, http.StatusForbidden, "permission_error", service.ImageGenerationPermissionMessage())
		return
	}
	if imageIntent {
		c.Request = c.Request.WithContext(service.WithOpenAIImageGenerationIntent(c.Request.Context()))
	}
	llmStream := reqStream && !imageIntent
	stopForcedImagePreKeepalive := func() {}
	if forceImageTool && reqStream {
		streamStarted = true
		stopForcedImagePreKeepalive = service.StartOpenAIResponsesImageSSEKeepalive(c, h.openAIImagesStreamKeepaliveInterval())
	}
	defer func() { stopForcedImagePreKeepalive() }()
	var imageReleaseFunc func()
	if imageIntent {
		var imageAcquired bool
		imageReleaseFunc, imageAcquired = h.acquireImageGenerationSlot(c, streamStarted)
		if !imageAcquired {
			return
		}
		if imageReleaseFunc != nil {
			defer imageReleaseFunc()
		}
	}

	// 解析渠道级模型映射
	channelMapping, _ := h.gatewayService.ResolveChannelMappingAndRestrict(c.Request.Context(), apiKey.GroupID, reqModel)
	forwardBody := openAIModelMappedBody(body, channelMapping.Mapped, channelMapping.MappedModel, h.gatewayService.ReplaceModelInBody)
	requestPlatform := openAICompatibleRequestPlatform(c.Request.Context(), apiKey)
	h.gatewayService.PrepareCodexSimulationRequest(c, apiKey.ID, apiKey.GroupID, forwardBody)
	// Forced image bridging performs a separate Images-account selection and does
	// not forward the Responses continuation to that account. Keep its API-key
	// image candidates outside the OAuth continuation-principal constraint.
	if shouldApplyCodexContinuationAffinity(forceImageTool, requestPlatform) {
		continuationSchedulingCtx, continuationSchedulingErr := h.gatewayService.WithCodexContinuationSchedulingAffinity(c.Request.Context(), c, forwardBody)
		if continuationSchedulingErr != nil {
			var terminalErr *service.CodexContinuationTerminalError
			if errors.As(continuationSchedulingErr, &terminalErr) {
				h.handleStreamingAwareError(c, terminalErr.HTTPStatus, terminalErr.ErrorType, terminalErr.Message, streamStarted)
				return
			}
			h.handleStreamingAwareError(c, http.StatusInternalServerError, "api_error", "Failed to prepare Codex continuation scheduling", streamStarted)
			return
		}
		c.Request = c.Request.WithContext(continuationSchedulingCtx)
	}
	seedOpenAIForwardImageIntentHint(c, channelMapping.Mapped, imageIntent)
	if channelMapping.Mapped {
		c.Request = c.Request.WithContext(service.WithOpenAIForwardModel(
			c.Request.Context(),
			channelMapping.MappedModel,
			compactionRoute.legacyCompact,
		))
	}

	// 提前校验 function_call_output 是否具备可关联上下文，避免上游 400。
	if !forceImageTool && !h.validateFunctionCallOutputRequest(c, body, reqLog) {
		return
	}
	// Slow admission and account selection precede Forward's HTTP/header loop.
	// Start the same serialized, replay-safe heartbeat before those waits.
	if llmStream && requestPlatform == service.PlatformOpenAI {
		stopPreparationKeepalive := service.StartOpenAIResponsesSSEKeepalive(c, h.openAICompactKeepaliveInterval(c))
		defer stopPreparationKeepalive()
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
	if err := h.billingCacheService.CheckBillingEligibility(c.Request.Context(), apiKey.User, apiKey, apiKey.Group, subscription, service.QuotaPlatform(c.Request.Context(), apiKey)); err != nil {
		reqLog.Info("openai.billing_eligibility_check_failed", zap.Error(err))
		status, code, message, retryAfter := billingErrorDetails(err)
		if retryAfter > 0 {
			c.Header("Retry-After", strconv.Itoa(retryAfter))
		}
		h.handleStreamingAwareError(c, status, code, message, streamStarted)
		return
	}

	// Generate session hash (header first; fallback to prompt_cache_key)
	sessionHash := h.gatewayService.GenerateSessionHashForRequest(c, apiKey.GroupID, sessionHashBody)
	defer h.gatewayService.ReleaseOpenAIContentSessionRequest(c.Request.Context(), apiKey.GroupID, sessionHash)
	if h.rejectIfCyberSessionBlocked(c, apiKey, sessionHashBody, reqModel, cyberBlockFormatResponses) {
		return
	}
	if forceImageTool {
		stopForcedImagePreKeepalive()
		stopForcedImagePreKeepalive = func() {}
		h.handleForcedOpenAIImageResponses(
			c,
			body,
			reqModel,
			reqStream,
			sessionHash,
			apiKey,
			subscription,
			reqLog,
			&streamStarted,
		)
		return
	}
	// The native v2 route stays on the ordinary /responses wire and must not use
	// the legacy compact probe gate. Existing accounts can carry a stale
	// openai_compact_supported=false value from the retired /responses/compact
	// probe; blocking those accounts would make a rolling upgrade fail closed.
	// Native v2 is constrained by requiredCapability=Responses below, while the
	// legacy endpoint alone keeps the compact-capability scheduler gate.
	requireCompact := compactionRoute.legacyCompact

	maxAccountSwitches := h.maxAccountSwitches
	// A connected LLM stream keeps recovering before semantic commitment. Its
	// finite boundary is the eligible pool, not an arbitrary early switch cap.
	recoverStreamPool := llmStream && requestPlatform == service.PlatformOpenAI
	service.ConfigureOpenAIPassthroughAttemptBudget(c, maxAccountSwitches)
	switchCount := 0
	firstOutputTimeoutSwitchCount := 0
	if imageIntent && requestPlatform == service.PlatformOpenAI {
		imageSize := service.OpenAIResponsesImageRequestSize(body)
		c.Request = c.Request.WithContext(service.WithOpenAIImageSizeRouting(c.Request.Context(), imageSize))
	}
	var failedAccountIDs map[int64]struct{}
	profitVetoCount := 0
	var sameAccountRetryCount map[int64]int
	var lastFailoverErr *service.UpstreamFailoverError
	var oauth429FailoverState service.OpenAIOAuth429FailoverState
	var passthroughFailoverState openAIPassthroughFailoverState

	// 生图意图的 /v1/responses 请求必须调度到确实支持 Responses API 的账号，否则
	// 会在 forward 阶段被静默降级为无法生图的 Chat Completions 直转（#4417）。
	// 仅对 OpenAI 平台生效：Grok 生图走独立的 forwardGrokResponses 路径，不应被过滤。
	// 使用 IsExplicitImageGenerationIntent 排除被动 image_gen namespace 声明，
	// 避免 Codex 的被动工具目录使 CC-only 账号被误过滤（#4476）。
	requiredCapability := openAIResponsesRequiredCapabilityForRequest(
		imageIntent,
		compactionRoute.requiresResponses(),
		requestPlatform,
		reqModel,
		channelMapping.Mapped,
		channelMapping.MappedModel,
	)

	for {
		// Streaming Forward intentionally detaches the upstream request so usage can
		// be drained after a disconnect. Re-check the client context before every
		// account attempt so a canceled request never starts a failover replay.
		if !openAIRequestAllowsFailoverReplay(c) {
			return
		}
		// Select account supporting the requested model
		reqLog.Debug("openai.account_selecting", zap.Int("excluded_account_count", len(failedAccountIDs)))
		selection, scheduleDecision, err := h.gatewayService.SelectAccountWithSchedulerForCapability(
			service.WithOpenAIStreamScheduling(c.Request.Context(), llmStream),
			apiKey.GroupID,
			previousResponseID,
			sessionHash,
			reqModel,
			failedAccountIDs,
			service.OpenAIUpstreamTransportAny,
			requiredCapability,
			requireCompact,
			false,
			!imageIntent,
			requestPlatform,
		)
		if err != nil {
			if failoverClientGone(c) {
				reqLog.Info("openai.account_select_aborted_client_disconnected", zap.Error(err))
				return
			}
			reqLog.Warn("openai.account_select_failed",
				zap.Error(openAICompatibleSelectionErrorForLog(err, requestPlatform)),
				zap.Int("excluded_account_count", len(failedAccountIDs)),
			)
			if errors.Is(err, service.ErrPriorityAdmissionUnavailable) {
				h.handleStreamingAwareError(c, http.StatusServiceUnavailable, "api_error", "Service temporarily unavailable, please retry later", streamStarted)
				return
			}
			if len(failedAccountIDs) == 0 {
				if compactionRoute.legacyCompact && errors.Is(err, service.ErrNoAvailableCompactAccounts) {
					markOpsRoutingCapacityLimitedIfNoAvailable(c, err)
					h.handleStreamingAwareError(c, http.StatusServiceUnavailable, "compact_not_supported", "No available accounts support /responses/compact", streamStarted)
					return
				}
				if imageIntent && requestPlatform == service.PlatformOpenAI {
					size := service.OpenAIResponsesImageRequestSize(body)
					if tier, unsupported := h.gatewayService.OpenAIImageSizeTierUnsupported(c.Request.Context(), apiKey.GroupID, reqModel, size, requiredCapability, ""); unsupported {
						service.MarkOpsClientBusinessLimited(c, service.OpsClientBusinessLimitedReasonLocalModelConfiguration)
						h.handleStreamingAwareError(c, http.StatusBadRequest, "invalid_request_error", service.OpenAIImageSizeTierUnsupportedMessage(size, tier), streamStarted)
						return
					}
				}
				cls := classifyNoAccountErrorFromGin(c, h.gatewayService, apiKey, reqModel, reqModel, requestPlatform)
				if !cls.ModelNotFound {
					markOpsRoutingCapacityLimitedIfNoAvailable(c, err)
				}
				h.handleStreamingAwareError(c, cls.Status, cls.ErrType, cls.Message, streamStarted)
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
			cls := classifyNoAccountErrorFromGin(c, h.gatewayService, apiKey, reqModel, reqModel, requestPlatform)
			if !cls.ModelNotFound {
				markOpsRoutingCapacityLimited(c)
			}
			h.handleStreamingAwareError(c, cls.Status, cls.ErrType, cls.Message, streamStarted)
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

		accountReleaseFunc, acquireOutcome := h.acquireResponsesAccountSlot(c, apiKey.GroupID, sessionHash, selection, reqStream, &streamStarted, reqLog)
		if acquireOutcome == accountSlotAcquireProfitVeto {
			if !recordOpenAIProfitVeto(&failedAccountIDs, account.ID, &profitVetoCount) {
				h.handleOpenAIProfitVetoExhausted(c, streamStarted, reqLog, profitVetoCount)
				return
			}
			continue
		}
		if acquireOutcome == accountSlotAcquireReschedule {
			addFailedAccountID(&failedAccountIDs, account.ID)
			continue
		}
		if acquireOutcome != accountSlotAcquireReady {
			return
		}
		account = selection.Account
		if recoverStreamPool {
			service.UseOpenAIStreamPoolAccount(c, account.ID)
		}

		// Forward request
		service.SetOpsLatencyMs(c, service.OpsRoutingLatencyMsKey, time.Since(routingStart).Milliseconds())
		forwardStart := time.Now()
		// 用扣除 compact 心跳字节的口径快照：心跳注释不构成语义响应，
		// 不能因心跳字节变化而放弃 failover 换号（#3887）。
		writerSizeBeforeForward := service.OpenAICompactKeepaliveAdjustedWrittenSize(c)
		attemptBody := h.deriveOpenAIForwardAttemptBody(reqLog, forwardBody, account, &passthroughFailoverState)
		if h.gatewayService.CodexSimulationRequestEnabled(c) {
			attemptBody, err = h.gatewayService.PrepareCodexSimulationAttempt(c.Request.Context(), c, account, attemptBody)
			if err != nil {
				if accountReleaseFunc != nil {
					accountReleaseFunc()
				}
				var terminalErr *service.CodexContinuationTerminalError
				if errors.As(err, &terminalErr) {
					h.handleStreamingAwareError(c, terminalErr.HTTPStatus, terminalErr.ErrorType, terminalErr.Message, streamStarted)
					return
				}
				h.handleStreamingAwareError(c, http.StatusInternalServerError, "api_error", "Failed to prepare Codex request", streamStarted)
				return
			}
		}
		result, err := func() (*service.OpenAIForwardResult, error) {
			defer func() {
				if accountReleaseFunc != nil {
					accountReleaseFunc()
				}
			}()
			return h.gatewayService.Forward(c.Request.Context(), c, account, attemptBody)
		}()
		cyberBlockKeyHTTP := ""
		if service.GetOpsCyberPolicy(c) != nil {
			cyberBlockKeyHTTP = service.CyberSessionBlockKey(apiKey.ID, c, sessionHashBody)
		}
		h.recordCyberPolicyIfMarked(c, apiKey, account, subscription, reqModel, err != nil, cyberBlockKeyHTTP, clientRequestedUsageFields(c, channelMapping, reqModel, ""), service.HashUsageRequestPayload(body))
		forwardDurationMs := time.Since(forwardStart).Milliseconds()
		upstreamLatencyMs, _ := getContextInt64(c, service.OpsUpstreamLatencyMsKey)
		responseLatencyMs := forwardDurationMs
		if upstreamLatencyMs > 0 && forwardDurationMs > upstreamLatencyMs {
			responseLatencyMs = forwardDurationMs - upstreamLatencyMs
		}
		service.SetOpsLatencyMs(c, service.OpsResponseLatencyMsKey, responseLatencyMs)
		if err == nil && reqStream && !imageIntent && result != nil {
			result.FirstTokenMs = service.PreferOpenAIRequestFirstEventTTFT(c, result.FirstTokenMs)
		}
		if err == nil && !imageIntent && result != nil && result.ImageCount <= 0 && result.FirstTokenMs != nil {
			service.SetOpsLatencyMs(c, service.OpsTimeToFirstTokenMsKey, int64(*result.FirstTokenMs))
		}
		if err != nil {
			var terminalErr *service.CodexContinuationTerminalError
			if errors.As(err, &terminalErr) {
				h.handleStreamingAwareError(c, terminalErr.HTTPStatus, terminalErr.ErrorType, terminalErr.Message, streamStarted)
				return
			}
			if result != nil && result.ImageCount > 0 {
				reqLog.Warn("openai.forward_partial_error_with_image_result",
					zap.Int64("account_id", account.ID),
					zap.Int("image_count", result.ImageCount),
					zap.Error(err),
				)
			} else {
				var failoverErr *service.UpstreamFailoverError
				if errors.As(err, &failoverErr) {
					if h.gatewayService.IsDistillationGroupRequest(c, account) {
						h.handleFailoverExhausted(c, failoverErr, streamStarted)
						return
					}
					if failoverClientGone(c) {
						reqLog.Info("openai.failover_aborted_client_disconnected",
							zap.Int64("account_id", account.ID),
							zap.Int("upstream_status", failoverErr.StatusCode),
						)
						return
					}
					if !openAIForwardMayFailover(c, writerSizeBeforeForward, failoverErr) {
						h.handleFailoverExhausted(c, failoverErr, true)
						return
					}
					// Failover safety was evaluated against semantic bytes above. If a
					// keepalive already committed HTTP 200, exhaustion must still end as SSE.
					if c.Writer.Written() {
						streamStarted = true
					}
					if failoverErr.ShouldReportAccountScheduleFailure() {
						h.gatewayService.ReportOpenAIAccountStreamScheduleResult(account.ID, account.GetMappedModel(reqModel), false, nil, llmStream)
					}
					if !failoverErr.ShouldRetryNextAccount() {
						h.handleFailoverExhausted(c, failoverErr, streamStarted)
						return
					}
					if !recoverStreamPool && openAIFirstOutputFailoverExhausted(failoverErr, &firstOutputTimeoutSwitchCount) {
						h.handleFailoverExhausted(c, failoverErr, streamStarted)
						return
					}
					// 池模式：同账号重试
					if failoverErr.RetryableOnSameAccount {
						retryLimit := account.GetPoolModeRetryCount()
						if retryCount, retry := tryIncrementSameAccountRetry(&sameAccountRetryCount, account.ID, retryLimit); retry {
							retryDelay := sameAccountRetryDelayFor(failoverErr, retryCount)
							reqLog.Warn("openai.pool_mode_same_account_retry",
								zap.Int64("account_id", account.ID),
								zap.Int("upstream_status", failoverErr.StatusCode),
								zap.Int("retry_limit", retryLimit),
								zap.Int("retry_count", retryCount),
								zap.Duration("retry_delay", retryDelay),
							)
							if !sleepWithContext(c.Request.Context(), retryDelay) {
								return
							}
							continue
						}
					}
					if failoverErr.Scope == service.GatewayFailureScopeRequest && sessionHash != "" {
						c.Request = c.Request.WithContext(h.gatewayService.PreserveOpenAIStickyBindingForFailover(c.Request.Context(), apiKey.GroupID, sessionHash, account.ID))
					}
					h.gatewayService.RecordOpenAIAccountSwitch()
					addFailedAccountID(&failedAccountIDs, account.ID)
					lastFailoverErr = failoverErr
					if !recoverStreamPool && switchCount >= maxAccountSwitches {
						h.handleFailoverExhausted(c, failoverErr, streamStarted)
						return
					}
					switchCount++
					if !recoverStreamPool && !failoverErr.IsOpenAIOAuthGatewayRateLimit() && h.gatewayService.ShouldStopOpenAIOAuth429Failover(account, failoverErr.StatusCode, switchCount, &oauth429FailoverState) {
						h.handleFailoverExhausted(c, failoverErr, streamStarted)
						return
					}
					failoverSwitchFields := []zap.Field{
						zap.Int64("account_id", account.ID),
						zap.Int("upstream_status", failoverErr.StatusCode),
						zap.Int("switch_count", switchCount),
						zap.Int("max_switches", maxAccountSwitches),
					}
					if account.Proxy != nil {
						failoverSwitchFields = append(failoverSwitchFields,
							zap.Int64("proxy_id", account.Proxy.ID),
							zap.String("proxy_name", account.Proxy.Name),
							zap.String("proxy_host", account.Proxy.Host),
							zap.Int("proxy_port", account.Proxy.Port),
						)
					} else if account.ProxyID != nil {
						failoverSwitchFields = append(failoverSwitchFields, zap.Int64p("proxy_id", account.ProxyID))
					}
					reqLog.Warn("openai.upstream_failover_switching", failoverSwitchFields...)
					continue
				}
				h.gatewayService.ReportOpenAIAccountStreamScheduleResult(account.ID, account.GetMappedModel(reqModel), false, nil, llmStream)
				upstreamErrorAlreadyCommunicated := openAIForwardErrorAlreadyCommunicated(c, writerSizeBeforeForward, err)
				wroteFallback := false
				if !upstreamErrorAlreadyCommunicated {
					wroteFallback = h.ensureForwardErrorResponse(c, streamStarted)
				}
				fields := []zap.Field{
					zap.Int64("account_id", account.ID),
					zap.Bool("fallback_error_response_written", wroteFallback),
					zap.Bool("upstream_error_response_already_written", upstreamErrorAlreadyCommunicated),
					zap.Error(err),
				}
				if shouldLogOpenAIForwardFailureAsWarn(c, wroteFallback) {
					reqLog.Warn("openai.forward_failed", fields...)
					return
				}
				reqLog.Error("openai.forward_failed", fields...)
				return
			}
		}
		if result != nil {
			// 排除 spark 影子:其 codex_* 仅由 QueryUsage(/wham/usage bengalfox)更新(外审第7轮 P1)。
			if account.Type == service.AccountTypeOAuth && !account.IsShadow() {
				h.gatewayService.UpdateCodexUsageSnapshotFromHeaders(c.Request.Context(), account.ID, result.ResponseHeaders)
			}
			h.gatewayService.ReportOpenAIAccountStreamScheduleResult(account.ID, account.GetMappedModel(reqModel), openAIForwardSucceededForScheduling(result), openAIFirstTokenForTTFT(result, imageIntent), llmStream)
		} else {
			h.gatewayService.ReportOpenAIAccountStreamScheduleResult(account.ID, account.GetMappedModel(reqModel), openAIForwardSucceededForScheduling(result), nil, llmStream)
		}

		// 捕获请求信息（用于异步记录，避免在 goroutine 中访问 gin.Context）
		userAgent := c.GetHeader("User-Agent")
		clientIP := ip.GetClientIP(c)
		requestPayloadHash := service.HashUsageRequestPayload(body)
		inboundEndpoint := GetInboundEndpoint(c)
		upstreamEndpoint := resolveOpenAIUpstreamEndpoint(c, account, result)
		quotaPlatform := service.QuotaPlatform(c.Request.Context(), apiKey)
		sessionID := service.ExtractClientSessionID(c)

		// 使用量记录通过有界 worker 池提交，避免请求热路径创建无界 goroutine。
		cyberBlocked := service.GetOpsCyberPolicy(c) != nil
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
				SessionID:          sessionID,
				RequestPayloadHash: requestPayloadHash,
				APIKeyService:      h.apiKeyService,
				QuotaPlatform:      quotaPlatform,
				PricingAt:          pricingAt,
				ChannelUsageFields: clientRequestedUsageFields(c, channelMapping, reqModel, result.UpstreamModel),
				CyberBlocked:       cyberBlocked,
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
	return service.IsOpenAIResponsesCompactPath(c)
}

// isBareOpenAIResponsesPath 仅匹配裸 /responses 端点（无 /compact 等子路径），
// body-signal 提升只允许发生在这里，避免误伤 /responses/{id}/... 形态的请求。
func isBareOpenAIResponsesPath(c *gin.Context) bool {
	if c == nil || c.Request == nil || c.Request.URL == nil {
		return false
	}
	normalizedPath := strings.TrimRight(strings.TrimSpace(c.Request.URL.Path), "/")
	switch normalizedPath {
	case EndpointResponses, "/openai/v1/responses", "/responses", "/backend-api/codex/responses":
		return true
	default:
		return false
	}
}

type openAIResponsesCompactionRoute struct {
	legacyCompact bool
	nativeV2      bool
}

func (r openAIResponsesCompactionRoute) requiresResponses() bool {
	return r.legacyCompact || r.nativeV2
}

// normalizeOpenAIResponsesCompactRequest keeps Codex remote compaction v2 on
// its native streaming /responses wire and preserves the legacy body-signal
// promotion for non-streaming requests. It classifies the request while the
// input is already being inspected so the hot path does not scan it twice.
// 返回归一化后的 body；ok=false 表示错误响应已写出，调用方应直接 return。
func (h *OpenAIGatewayHandler) normalizeOpenAIResponsesCompactRequest(c *gin.Context, reqLog *zap.Logger, body []byte) ([]byte, openAIResponsesCompactionRoute, bool) {
	route := openAIResponsesCompactionRoute{legacyCompact: service.IsOpenAIResponsesCompactPath(c)}
	if !route.legacyCompact && isBareOpenAIResponsesPath(c) && service.HasCompactionTriggerInInput(body) {
		stream, valid := parseOpenAICompatibleStream(body)
		if valid && stream {
			route.nativeV2 = true
			service.MarkOpenAINativeCompactionV2(c)
			return body, route, true
		}
		c.Request.URL.Path = strings.TrimRight(c.Request.URL.Path, "/") + "/compact"
		route.legacyCompact = true
		clientStream := gjson.GetBytes(body, "stream").Bool()
		if clientStream {
			service.MarkOpenAICompactClientStream(c)
		}
		reqLog.Info("codex.remote_compact.detected_body_signal", zap.Bool("client_stream", clientStream))
	}
	if !route.legacyCompact {
		return body, route, true
	}
	if compactSeed := strings.TrimSpace(gjson.GetBytes(body, "prompt_cache_key").String()); compactSeed != "" {
		c.Set(service.OpenAICompactSessionSeedKeyForTest(), compactSeed)
	}
	normalizedCompactBody, normalizedCompact, compactErr := service.NormalizeOpenAICompactRequestBodyForTest(body)
	if compactErr != nil {
		h.errorResponse(c, http.StatusBadRequest, "invalid_request_error", "Failed to normalize compact request body")
		return nil, route, false
	}
	if normalizedCompact {
		body = normalizedCompactBody
	}
	return body, route, true
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
	// compact 心跳提交后失败的 wire 状态码固化为 200，真实结局以流内错误
	// 标记为准（response.failed 降级路径会 MarkOpsStreamError）。
	if outcome == "succeeded" && c != nil {
		if _, hasStreamErr := service.GetOpsStreamError(c); hasStreamErr {
			outcome = "failed"
		}
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
