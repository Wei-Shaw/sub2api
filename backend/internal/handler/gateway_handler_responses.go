package handler

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/ip"
	middleware2 "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
	"go.uber.org/zap"
)

// Responses handles OpenAI Responses API endpoint for Anthropic platform groups.
// POST /v1/responses
// This converts Responses API requests to Anthropic format, forwards to Anthropic
// upstream, and converts responses back to Responses format.
func (h *GatewayHandler) Responses(c *gin.Context) {
	streamStarted := false

	requestStart := time.Now()

	apiKey, ok := middleware2.GetAPIKeyFromContext(c)
	if !ok {
		h.responsesErrorResponse(c, http.StatusUnauthorized, "authentication_error", "Invalid API key")
		return
	}

	subject, ok := middleware2.GetAuthSubjectFromContext(c)
	if !ok {
		h.responsesErrorResponse(c, http.StatusInternalServerError, "api_error", "User context not found")
		return
	}
	reqLog := requestLogger(
		c,
		"handler.gateway.responses",
		zap.Int64("user_id", subject.UserID),
		zap.Int64("api_key_id", apiKey.ID),
		zap.Any("group_id", apiKey.GroupID),
	)

	// Read request body
	body, err := readLenientJSONRequestBodyWithPrealloc(c.Request, h.cfg)
	if err != nil {
		if maxErr, ok := extractMaxBytesError(err); ok {
			h.responsesErrorResponse(c, http.StatusRequestEntityTooLarge, "invalid_request_error", buildBodyTooLargeMessage(maxErr.Limit))
			return
		}
		h.responsesErrorResponse(c, http.StatusBadRequest, "invalid_request_error", "Failed to read request body")
		return
	}

	if len(body) == 0 {
		h.responsesErrorResponse(c, http.StatusBadRequest, "invalid_request_error", "Request body is empty")
		return
	}

	setOpsRequestContext(c, "", false)

	// Validate JSON
	if !gjson.ValidBytes(body) {
		logRequestBodyParseFailure(reqLog, body, nil)
		h.responsesErrorResponse(c, http.StatusBadRequest, "invalid_request_error", "Failed to parse request body")
		return
	}

	// Extract model and stream using gjson (like OpenAI handler)
	modelResult := gjson.GetBytes(body, "model")
	if !modelResult.Exists() || modelResult.Type != gjson.String || modelResult.String() == "" {
		h.responsesErrorResponse(c, http.StatusBadRequest, "invalid_request_error", "model is required")
		return
	}
	reqModel := modelResult.String()
	bindRequestedReasoningEffort(c, body, reqModel)
	ensureCompositeTargetPlatform(c, apiKey, reqModel)
	if !compositeTargetPlatformResolved(c, apiKey, reqModel) {
		h.responsesErrorResponse(c, http.StatusBadRequest, "invalid_request_error", "Model is not supported by composite groups")
		return
	}
	reqStream, ok := parseOpenAICompatibleStream(body)
	if !ok {
		h.responsesErrorResponse(c, http.StatusBadRequest, "invalid_request_error", invalidStreamFieldTypeMessage)
		return
	}
	reqLog = reqLog.With(zap.String("model", reqModel), zap.Bool("stream", reqStream))

	setOpsRequestContext(c, reqModel, reqStream)
	setOpsEndpointContext(c, "", int16(service.RequestTypeFromLegacy(reqStream, false)))
	requestCtx := c.Request.Context()
	// 定价上下文无条件装配：/v1/responses 是 token 计费端点，声明生图工具的
	// 混合请求同样按 token 计费（外加图片部分），其 token 利润保护不因请求体
	// 里的任何工具声明（含 Codex 被动 image_gen namespace）而关闭。生图意图
	// 仅用于能力路由与图片计费；独立图片/视频端点才在利润门范围之外。
	requestCtx, pricingAt := service.WithGatewayTokenRequestPricing(requestCtx)
	if service.IsImageGenerationIntentForPlatform("/v1/responses", reqModel, body, openAICompatibleRequestPlatform(c.Request.Context(), apiKey)) {
		requestCtx = service.WithOpenAIImageGenerationIntent(requestCtx)
	}
	c.Request = c.Request.WithContext(requestCtx)

	// 解析渠道级模型映射
	channelMapping, _ := h.gatewayService.ResolveChannelMappingAndRestrict(requestCtx, apiKey.GroupID, reqModel)

	// Claude Code only restriction:
	// /v1/responses is never a Claude Code endpoint.
	// When claude_code_only is enabled, this endpoint is rejected.
	// The existing service-layer checkClaudeCodeRestriction handles degradation
	// to fallback groups when the Forward path calls SelectAccountForModelWithExclusions.
	// Here we just reject at handler level since /v1/responses clients can't be Claude Code.
	if apiKey.Group != nil && apiKey.Group.ClaudeCodeOnly {
		h.responsesErrorResponse(c, http.StatusForbidden, "permission_error",
			"This group is restricted to Claude Code clients (/v1/messages only)")
		return
	}

	if decision := h.checkSecurityAudit(c, reqLog, apiKey, subject, service.ContentModerationProtocolOpenAIResponses, reqModel, body); decision != nil && !decision.AllowNextStage {
		h.responsesSecurityAuditError(c, decision)
		return
	}

	// Error passthrough binding
	if h.errorPassthroughService != nil {
		service.BindErrorPassthroughService(c, h.errorPassthroughService)
	}

	subscription, _ := middleware2.GetSubscriptionFromContext(c)

	service.SetOpsLatencyMs(c, service.OpsAuthLatencyMsKey, time.Since(requestStart).Milliseconds())

	userReleaseFunc, err := h.concurrencyHelper.AcquireUserSlotWithWait(c, subject.UserID, subject.Concurrency, reqStream, &streamStarted)
	if err != nil {
		reqLog.Warn("gateway.responses.user_slot_acquire_failed", zap.Error(err))
		h.handleConcurrencyError(c, err, "user", streamStarted)
		return
	}
	userReleaseFunc = wrapReleaseOnDone(c.Request.Context(), userReleaseFunc)
	if userReleaseFunc != nil {
		defer userReleaseFunc()
	}

	// 2. Re-check billing
	if err := h.billingCacheService.CheckBillingEligibility(requestCtx, apiKey.User, apiKey, apiKey.Group, subscription, service.QuotaPlatform(requestCtx, apiKey)); err != nil {
		reqLog.Info("gateway.responses.billing_check_failed", zap.Error(err))
		status, code, message, retryAfter := billingErrorDetails(err)
		if retryAfter > 0 {
			c.Header("Retry-After", strconv.Itoa(retryAfter))
		}
		h.responsesErrorResponse(c, status, code, message)
		return
	}

	// Parse request for session hash
	bodyRef := service.NewRequestBodyRef(body)
	parsedReq, _ := service.ParseGatewayRequest(bodyRef, "responses")
	if parsedReq == nil {
		parsedReq = &service.ParsedRequest{Model: reqModel, Stream: reqStream, Body: bodyRef}
	}
	parsedReq.SessionContext = &service.SessionContext{
		ClientIP:  ip.GetClientIP(c),
		UserAgent: c.GetHeader("User-Agent"),
		APIKeyID:  apiKey.ID,
	}
	sessionHash := h.gatewayService.GenerateSessionHash(parsedReq)

	// composite 账号池请求状态（B2：generic Responses 循环池化）。池激活判定与
	// generic 调度器共用同一谓词（ctx 携带候选池且无 resolved 平台）；非池请求
	// poolDenials 恒为空、attempt 策略与委派分支不生效，整条路径零变化。
	isPoolRequest := service.GenericCompositePoolActive(requestCtx)
	poolDenials := newCompositePoolPlatformDenials()

	// 3. Account selection + failover loop
	fs := NewFailoverState(h.maxAccountSwitches, false)

	for {
		if requestCtx.Err() != nil {
			return
		}
		// 池请求把业务限制 deny 过的平台按 deniedPlatforms 整平台 mask 后重选
		//（候选池 immutable，mask 只叠加在当前请求 ctx 之上）；无 deny 时
		// 返回原 ctx，非池请求不经此分支，选号 ctx 与原行为一致。
		selectCtx := requestCtx
		if isPoolRequest {
			if maskedCtx, ok := compositePoolSelectionRetryContext(selectCtx, poolDenials); ok {
				selectCtx = maskedCtx
			}
		}
		selection, err := h.gatewayService.SelectAccountWithLoadAwareness(selectCtx, apiKey.GroupID, sessionHash, reqModel, fs.FailedAccountIDs, "", int64(0))
		if err != nil {
			if len(fs.FailedAccountIDs) == 0 {
				cls := classifyNoAccountErrorFromGin(c, h.gatewayService, apiKey, reqModel, reqModel, effectiveAPIKeyPlatform(c, apiKey))
				cls = classifySelectionFailureError(err, cls)
				if !cls.ModelNotFound {
					markOpsRoutingCapacityLimitedIfNoAvailable(c, err)
				}
				message := cls.Message
				if !cls.ModelNotFound {
					message = "No available accounts: " + err.Error()
				}
				h.responsesErrorResponse(c, cls.Status, cls.ErrType, message)
				return
			}
			action := fs.HandleSelectionExhausted(requestCtx)
			switch action {
			case FailoverContinue:
				continue
			case FailoverCanceled:
				failoverClientGone(c)
				return
			default:
				if fs.LastFailoverErr != nil {
					h.handleResponsesFailoverExhausted(c, fs.LastFailoverErr, streamStarted)
				} else {
					h.responsesErrorResponse(c, http.StatusBadGateway, "server_error", "All available accounts exhausted")
				}
				return
			}
		}
		account := selection.Account
		setOpsSelectedAccount(c, account.ID, account.Platform)

		// composite 账号池 per-attempt 平台策略：对实际选中平台补做 user×platform
		// 配额预检与渠道映射/限制（准入 CheckBillingEligibility 时池没有 resolved
		// 平台、这两个维度被跳过；不得只后扣不预检）。检查为纯读，不写 RPM/计数，
		// 放在槽位获取前：deny 不占并发槽。generic Responses 循环无
		// previous_response_id，无 pinned 分支；deny 记入 poolDenials 后按 masked
		// 候选池整平台重选，候选全部被业务限制时按原语义终止，不误报 model
		// unsupported。
		attemptPolicy := compositePoolAttemptPolicy{AttemptCtx: requestCtx, Mapping: channelMapping}
		isPoolAttempt := false
		if isPoolRequest && compositePoolAttemptPolicyApplies(c, apiKey) {
			attemptPolicy = evaluateCompositePoolAttemptPolicy(
				requestCtx, h.billingCacheService, h.openAIGatewayService, apiKey, subscription, account, reqModel, false)
			isPoolAttempt = true
			if attemptPolicy.Failure.denied() {
				releaseCompositePoolSelection(selection)
				// 选号链可能已为该账号注册会话槽：排除前释放（镜像 Messages 池分支）。
				h.gatewayService.ReleaseAccountSession(context.Background(), account, sessionHash)
				poolDenials.deny(account.Platform)
				fs.FailedAccountIDs[account.ID] = struct{}{}
				// 候选全部被业务限制：按 deny 原因明确终止（后续重选不再尝试）。
				if _, ok := compositePoolSelectionRetryContext(requestCtx, poolDenials); !ok {
					streamAwareWrite := func(cc *gin.Context, status int, code, message string) {
						if streamStarted {
							h.handleStreamingAwareError(cc, status, code, message, true)
							return
						}
						h.responsesErrorResponse(cc, status, code, message)
					}
					respondCompositePoolAttemptPolicyFailure(c, attemptPolicy.Failure, streamAwareWrite)
					return
				}
				continue
			}
		}

		// 4. Acquire account concurrency slot
		accountReleaseFunc := selection.ReleaseFunc
		if !selection.Acquired {
			if selection.WaitPlan == nil {
				markOpsRoutingCapacityLimited(c)
				h.responsesErrorResponse(c, http.StatusServiceUnavailable, "api_error", "No available accounts")
				return
			}
			accountReleaseFunc, err = h.concurrencyHelper.AcquireAccountSlotWithWaitTimeout(
				c,
				account.ID,
				selection.WaitPlan.MaxConcurrency,
				selection.WaitPlan.Timeout,
				reqStream,
				&streamStarted,
			)
			if err != nil {
				reqLog.Warn("gateway.responses.account_slot_acquire_failed", zap.Int64("account_id", account.ID), zap.Error(err))
				h.handleConcurrencyError(c, err, "account", streamStarted)
				return
			}
		}
		// 终检与准入后绑定必须使用选号结果携带的门：门安装在调度栈的局部
		// ctx 上（composite/fallback 还可能解析出与入口分组不同的门），直接用
		// requestCtx 会退化为空操作。
		admissionCtx := service.ContextWithSelectionProfitGate(requestCtx, selection)
		latest, vetoed, reason := h.gatewayService.GatewayProfitControlVetoLatest(admissionCtx, account)
		if vetoed {
			if accountReleaseFunc != nil {
				accountReleaseFunc()
			}
			reqLog.Debug("gateway.responses.account_slot_profit_vetoed", zap.Int64("account_id", account.ID), zap.String("reason", reason))
			if fs.RecordProfitVeto(account.ID) == FailoverExhausted {
				reqLog.Warn("gateway.responses.profit_veto_attempts_exhausted", zap.Int("profit_veto_count", fs.ProfitVetoCount()))
				h.responsesErrorResponse(c, http.StatusServiceUnavailable, "api_error", profitVetoExhaustedMessage)
				return
			}
			continue
		}
		account = latest
		selection.Account = latest
		if selection.ProfitGateActive() {
			if err := h.gatewayService.BindStickySessionAfterProfitAdmission(admissionCtx, apiKey.GroupID, sessionHash, account.ID); err != nil {
				reqLog.Warn("gateway.responses.bind_sticky_session_after_profit_admission_failed", zap.Int64("account_id", account.ID), zap.Error(err))
			}
		}
		accountReleaseFunc = wrapReleaseOnDone(c.Request.Context(), accountReleaseFunc)

		// 5. Forward request
		writerSizeBeforeForward := c.Writer.Size()
		// 渠道模型映射只作用于本次账号尝试：池请求按选中平台的渠道映射（attempt
		// 策略产出，platform-scoped）派生，非池保持请求级映射语义不变。
		attemptChannelMapping := channelMapping
		if isPoolAttempt {
			attemptChannelMapping = attemptPolicy.Mapping
		}
		forwardBody := body
		if attemptChannelMapping.Mapped {
			forwardBody = h.gatewayService.ReplaceModelInBody(body, attemptChannelMapping.MappedModel)
		}
		var result *service.ForwardResult
		setActualUpstreamEndpoint(c, "")
		// 池请求的 attempt 局部 ctx 携带选中平台（不写回 requestCtx，否则下一轮
		// 选号会被 resolved 平台截断池语义）：forward 内的渠道定价作用域与计费
		// QuotaPlatform 跟随实际平台，对齐 Messages 池 attempt 语义。
		attemptCtx := requestCtx
		if isPoolRequest {
			attemptCtx = compositePoolAttemptContext(attemptCtx, account)
		}
		if shouldUseAntigravityCompat(account) {
			if h.antigravityGatewayService == nil {
				h.responsesErrorResponse(c, http.StatusBadGateway, "upstream_error", "Antigravity compatibility service is not configured")
				if accountReleaseFunc != nil {
					accountReleaseFunc()
				}
				return
			}
			setActualUpstreamEndpoint(c, EndpointAntigravityGenerateContent)
			result, err = h.antigravityGatewayService.ForwardAsResponses(attemptCtx, c, account, forwardBody, parsedReq)
		} else if isPoolRequest && compositePoolAccountDelegatesResponsesToOpenAI(account) {
			// 纯 OpenAI 族 / 原生 Responses 账号在池激活时委派 OpenAI 网关 Responses
			// 链：入站同族优先（零或近零转换），响应写回与 failover 错误信封都由该
			// 链完成，错误信封已是 OpenAI Responses 风格，不再二次包装。
			// promptCacheKey 由链内从 body 派生（与 OpenAI handler Responses 路径一致）。
			setOpenAIClientTransportHTTP(c)
			openAIResult, delegateErr := h.openAIGatewayService.Forward(attemptCtx, c, account, forwardBody)
			result = adaptOpenAIForwardResultToForwardResult(openAIResult)
			err = delegateErr
		} else {
			result, err = h.gatewayService.ForwardAsResponses(attemptCtx, c, account, forwardBody, parsedReq)
		}

		if accountReleaseFunc != nil {
			accountReleaseFunc()
		}

		if err != nil {
			var failoverErr *service.UpstreamFailoverError
			if errors.As(err, &failoverErr) {
				// Can't failover if streaming content already sent
				if c.Writer.Size() != writerSizeBeforeForward {
					h.handleResponsesFailoverExhausted(c, failoverErr, true)
					return
				}
				action := fs.HandleFailoverError(requestCtx, h.gatewayService, account.ID, account.Platform, account.GetPoolModeRetryCount(), failoverErr)
				switch action {
				case FailoverContinue:
					continue
				case FailoverExhausted:
					h.handleResponsesFailoverExhausted(c, fs.LastFailoverErr, streamStarted)
					return
				case FailoverCanceled:
					failoverClientGone(c)
					return
				}
			}
			upstreamErrorAlreadyCommunicated := gatewayForwardErrorAlreadyCommunicated(c, writerSizeBeforeForward, err)
			wroteFallback := false
			if !upstreamErrorAlreadyCommunicated {
				wroteFallback = h.ensureForwardErrorResponse(c, streamStarted)
			}
			reqLog.Error("gateway.responses.forward_failed",
				zap.Int64("account_id", account.ID),
				zap.Bool("fallback_error_response_written", wroteFallback),
				zap.Bool("upstream_error_response_already_written", upstreamErrorAlreadyCommunicated),
				zap.Error(err),
			)
			return
		}

		// 6. Record usage
		userAgent := c.GetHeader("User-Agent")
		clientIP := ip.GetClientIP(c)
		requestPayloadHash := service.HashUsageRequestPayload(body)
		inboundEndpoint := GetInboundEndpoint(c)
		upstreamEndpoint := GetUpstreamEndpoint(c, account.Platform)

		quotaPlatform := service.QuotaPlatform(c.Request.Context(), apiKey)
		// 池请求按选中平台计量 user×platform 配额（attempt ctx 携带 resolved
		// 平台）；非池保持准入语义不变。
		if isPoolAttempt {
			quotaPlatform = service.QuotaPlatform(attemptPolicy.AttemptCtx, apiKey)
		}
		sessionID := service.ExtractClientSessionID(c)
		stampForwardRequestedReasoningEffort(result, service.RequestedReasoningEffortFromContext(c.Request.Context()))
		h.submitUsageRecordTask(c.Request.Context(), func(ctx context.Context) {
			if err := h.gatewayService.RecordUsage(ctx, &service.RecordUsageInput{
				Result:             result,
				QuotaPlatform:      quotaPlatform,
				APIKey:             apiKey,
				User:               apiKey.User,
				Account:            account,
				Subscription:       subscription,
				PricingAt:          pricingAt,
				InboundEndpoint:    inboundEndpoint,
				UpstreamEndpoint:   upstreamEndpoint,
				UserAgent:          userAgent,
				IPAddress:          clientIP,
				RequestPayloadHash: requestPayloadHash,
				APIKeyService:      h.apiKeyService,
				SessionID:          sessionID,
				ChannelUsageFields: clientRequestedUsageFields(c, attemptChannelMapping, reqModel, result.UpstreamModel),
			}); err != nil {
				reqLog.Error("gateway.responses.record_usage_failed",
					zap.Int64("account_id", account.ID),
					zap.Error(err),
				)
			}
		})
		return
	}
}

// responsesErrorResponse writes an error in OpenAI Responses API format.
func (h *GatewayHandler) responsesErrorResponse(c *gin.Context, status int, code, message string) {
	c.JSON(status, gin.H{
		"error": gin.H{
			"code":    code,
			"message": message,
		},
	})
}

// handleResponsesFailoverExhausted writes a failover-exhausted error in Responses format.
func (h *GatewayHandler) handleResponsesFailoverExhausted(c *gin.Context, lastErr *service.UpstreamFailoverError, streamStarted bool) {
	if lastErr != nil {
		copyFailoverRetryAfter(c, lastErr.ResponseHeaders)
	}
	statusCode := http.StatusBadGateway
	if lastErr != nil && lastErr.StatusCode > 0 {
		statusCode = lastErr.StatusCode
	}
	status, code, message := statusCode, "server_error", "All available accounts exhausted"
	if lastErr != nil && lastErr.IsCredentialFailure() {
		status, message = credentialFailoverClientResponse(lastErr)
	} else if lastErr != nil && lastErr.IsOpenAICapacityShed() && strings.TrimSpace(lastErr.ClientMessage) != "" {
		status = lastErr.ClientStatusCode
		if status <= 0 {
			status = http.StatusServiceUnavailable
		}
		message = lastErr.ClientMessage
	} else if lastErr != nil && service.IsOpenAISilentRefusalErrorBody(lastErr.ResponseBody) {
		service.SetOpsUpstreamError(c, statusCode, service.OpenAISilentRefusalClientMessage(), "")
		status, code, message = http.StatusBadGateway, "upstream_error", service.OpenAISilentRefusalClientMessage()
	} else if lastErr != nil && statusCode == http.StatusTooManyRequests {
		status, code, message = http.StatusTooManyRequests, "rate_limit_error", "All available accounts are currently rate-limited. Please retry later."
	}
	if streamStarted {
		// A slot-wait heartbeat commits HTTP 200 before any upstream response.
		// In that case a terminal frame is still required; once any semantic or
		// official terminal bytes exist, preserve them without appending a second
		// generic response.failed.
		service.MarkOpsStreamError(c, code, message, status)
		if c != nil && c.Writer != nil && (c.Writer.Size() <= 0 || gatewayStreamHasOnlyHeartbeats(c)) {
			writeResponsesFailedSSE(c, code, "", message)
		}
		return
	}
	h.responsesErrorResponse(c, status, code, message)
}
