package handler

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"strconv"
	"strings"

	pkghttputil "github.com/Wei-Shaw/sub2api/internal/pkg/httputil"
	"github.com/Wei-Shaw/sub2api/internal/pkg/ip"
	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// GeminiV1BetaModels proxies Gemini native REST endpoints like:
// POST /v1beta/models/{model}:generateContent
// POST /v1beta/models/{model}:streamGenerateContent?alt=sse
func (h *GatewayHandler) GeminiV1BetaModels(c *gin.Context) {
	apiKey, ok := middleware.GetAPIKeyFromContext(c)
	if !ok || apiKey == nil {
		googleError(c, http.StatusUnauthorized, "Invalid API key")
		return
	}
	authSubject, ok := middleware.GetAuthSubjectFromContext(c)
	if !ok {
		googleError(c, http.StatusInternalServerError, "User context not found")
		return
	}
	reqLog := requestLogger(
		c,
		"handler.gemini_v1beta.models",
		zap.Int64("user_id", authSubject.UserID),
		zap.Int64("api_key_id", apiKey.ID),
		zap.Any("group_id", apiKey.GroupID),
	)

	// 检查平台：优先使用强制平台（/antigravity 路由，中间件已设置 request.Context），否则要求 gemini 分组（composite 分组允许路由到 antigravity）
	effectivePlatform := effectiveAPIKeyPlatform(c, apiKey)
	if !middleware.HasForcePlatform(c) {
		if effectivePlatform != service.PlatformGemini &&
			!servesCompositeAntigravityGemini(apiKey, effectivePlatform) {
			googleError(c, http.StatusBadRequest, "API key group platform is not gemini")
			return
		}
	}

	modelName, action, err := parseGeminiModelAction(strings.TrimPrefix(c.Param("modelAction"), "/"))
	if err != nil {
		googleError(c, http.StatusNotFound, err.Error())
		return
	}
	// URL 里的模型名最终会被拼进上游 /v1beta/models/{model}:{action}，
	// 先在入口校验片段合规性，见 service/upstream_path_guard.go。
	if !service.IsSafeGeminiModelPathSegment(modelName) {
		googleError(c, http.StatusBadRequest, "Invalid model in URL")
		return
	}
	urlModel := modelName
	if resolvedModel, ok := service.ResolvedUpstreamModelFromContext(c.Request.Context()); ok && strings.TrimSpace(resolvedModel) != "" {
		modelName = strings.TrimSpace(resolvedModel)
	}
	if !middleware.HasForcePlatform(c) && servesCompositeAntigravityGemini(apiKey, effectivePlatform) && !isAntigravityGeminiModel(modelName) {
		googleError(c, http.StatusBadRequest, "API key group platform is not gemini")
		return
	}

	stream := action == "streamGenerateContent"
	reqLog = reqLog.With(zap.String("model", modelName), zap.String("action", action), zap.Bool("stream", stream))

	body, err := pkghttputil.ReadRequestBodyWithPrealloc(c.Request)
	if err != nil {
		if handleRequestBodyReadError(c, err) {
			return
		}
		if maxErr, ok := extractMaxBytesError(err); ok {
			googleError(c, http.StatusRequestEntityTooLarge, buildBodyTooLargeMessage(maxErr.Limit))
			return
		}
		googleError(c, http.StatusBadRequest, "Failed to read request body")
		return
	}
	if len(body) == 0 {
		googleError(c, http.StatusBadRequest, "Request body is empty")
		return
	}

	setOpsRequestContext(c, modelName, stream)
	setOpsEndpointContext(c, "", int16(service.RequestTypeFromLegacy(stream, false)))
	pricingCtx, pricingAt := service.WithGatewayTokenRequestPricing(c.Request.Context())
	c.Request = c.Request.WithContext(pricingCtx)

	// 解析渠道级模型映射
	channelMapping, _ := h.gatewayService.ResolveChannelMappingAndRestrict(c.Request.Context(), apiKey.GroupID, modelName)
	reqModel := modelName // 保存映射前的原始模型名
	if action != "countTokens" && geminiImageGenerationRejected(c, apiKey, body, urlModel, reqModel, channelMapping.MappedModel) {
		googleError(c, http.StatusBadRequest, service.ImageGenerationPermissionMessage())
		return
	}

	if decision := h.checkSecurityAudit(c, reqLog, apiKey, authSubject, service.ContentModerationProtocolGemini, reqModel, body); decision != nil && !decision.AllowNextStage {
		googleSecurityAuditError(c, decision)
		return
	}

	if channelMapping.Mapped {
		modelName = channelMapping.MappedModel
	}

	// 引用显式缓存的请求只能发往持有该缓存的账号，且不参与会话粘性与换号。
	cachedContentBinding, body, ok := h.resolveGeminiCachedContentBinding(c, reqLog, apiKey, modelName, body)
	if !ok {
		return
	}

	// Get subscription (may be nil)
	subscription, _ := middleware.GetSubscriptionFromContext(c)

	// For Gemini native API, do not send Claude-style ping frames.
	geminiConcurrency := NewConcurrencyHelper(h.concurrencyHelper.concurrencyService, SSEPingFormatNone, 0)

	// 1) user concurrency slot
	streamStarted := false
	if h.errorPassthroughService != nil {
		service.BindErrorPassthroughService(c, h.errorPassthroughService)
	}
	userReleaseFunc, err := geminiConcurrency.AcquireUserSlotWithWait(c, authSubject.UserID, authSubject.Concurrency, stream, &streamStarted)
	if err != nil {
		reqLog.Warn("gemini.user_slot_acquire_failed", zap.Error(err))
		googleConcurrencyError(c, err, "user")
		return
	}
	// 确保请求取消时也会释放槽位，避免长连接被动中断造成泄漏
	userReleaseFunc = wrapReleaseOnDone(c.Request.Context(), userReleaseFunc)
	if userReleaseFunc != nil {
		defer userReleaseFunc()
	}

	// 2) billing eligibility check (after wait)
	if err := h.billingCacheService.CheckBillingEligibility(c.Request.Context(), apiKey.User, apiKey, apiKey.Group, subscription, service.QuotaPlatform(c.Request.Context(), apiKey)); err != nil {
		reqLog.Info("gemini.billing_eligibility_check_failed", zap.Error(err))
		status, code, message, retryAfter := billingErrorDetails(err)
		if retryAfter > 0 {
			c.Header("Retry-After", strconv.Itoa(retryAfter))
		}
		googleErrorWithType(c, status, code, "", message)
		return
	}

	// 余额模式在途预留：防止并发请求在预检时看到同一份余额而集体透支。
	inflightRelease, err := reserveInflightBalance(c, h.billingCacheService, h.gatewayService, apiKey, subscription, tokenInflightEstimate(modelName, body))
	if err != nil {
		reqLog.Info("gemini.inflight_reservation_rejected", zap.Error(err))
		status, code, message, retryAfter := billingErrorDetails(err)
		if retryAfter > 0 {
			c.Header("Retry-After", strconv.Itoa(retryAfter))
		}
		googleErrorWithType(c, status, code, "", message)
		return
	}
	defer inflightRelease()

	// 3) select account (sticky session based on explicit session / digest chain / conversation anchor)
	sessionState := h.resolveGeminiStickySession(c, reqLog, authSubject.UserID, apiKey, modelName, body, cachedContentBinding)
	sessionKey := sessionState.sessionKey
	sessionBoundAccountID := sessionState.sessionBoundAccountID

	// 判断是否真的绑定了粘性会话：有 sessionKey 且已经绑定到某个账号
	hasBoundSession := sessionKey != "" && sessionBoundAccountID > 0
	cleanedForUnknownBinding := false

	maxAccountSwitches := h.maxAccountSwitchesGemini
	var cachedContentExclusions map[int64]struct{}
	if cachedContentBinding != nil {
		// 缓存只存在于绑定账号：排除分组内其余账号，失败不换号。
		maxAccountSwitches = 0
		sessionBoundAccountID = cachedContentBinding.AccountID
		c.Request = c.Request.WithContext(service.WithGeminiCachedContentBound(c.Request.Context()))
		cachedContentExclusions, err = h.gatewayService.GeminiExplicitCacheExclusions(c.Request.Context(), apiKey.GroupID, cachedContentBinding.AccountID)
		if err != nil {
			reqLog.Warn("gemini.cached_content.list_accounts_failed", zap.Error(err))
			googleError(c, http.StatusServiceUnavailable, geminiCachedContentUnavailableMessage)
			return
		}
	}
	fs := NewFailoverState(maxAccountSwitches, hasBoundSession)

	// 单账号分组首次 503 就不设模型限流标记，避免设 29s 限流导致后续请求连续快速失败；
	// 是否单账号分组在 antigravity 账号遇到 503 或模型限流预检查时才判定。
	c.Request = c.Request.WithContext(h.gatewayService.WithSingleAntigravityAccountGroupProbe(c.Request.Context(), apiKey.GroupID))

	for {
		excludedAccountIDs := fs.FailedAccountIDs
		if cachedContentExclusions != nil {
			excludedAccountIDs = mergeAccountIDSets(cachedContentExclusions, fs.FailedAccountIDs)
		}
		selection, err := h.gatewayService.SelectAccountWithLoadAwareness(c.Request.Context(), apiKey.GroupID, sessionKey, modelName, excludedAccountIDs, "", int64(0)) // Gemini 不使用会话限制
		if err == nil && cachedContentBinding != nil && selection.Account.ID != cachedContentBinding.AccountID {
			if selection.Acquired && selection.ReleaseFunc != nil {
				selection.ReleaseFunc()
			}
			err = service.ErrNoAvailableAccounts
		}
		if err != nil {
			if failoverClientGone(c) {
				reqLog.Info("gemini.account_select_aborted_client_disconnected", zap.Error(err))
				return
			}
			if cachedContentBinding != nil {
				if fs.LastFailoverErr != nil {
					h.handleGeminiCachedContentFailoverExhausted(c, fs.LastFailoverErr)
					return
				}
				reqLog.Info("gemini.cached_content.bound_account_unavailable", zap.Int64("account_id", cachedContentBinding.AccountID), zap.Error(err))
				markOpsRoutingCapacityLimited(c)
				googleError(c, http.StatusServiceUnavailable, geminiCachedContentUnavailableMessage)
				return
			}
			if len(fs.FailedAccountIDs) == 0 {
				diagPlatform := service.PlatformGemini
				if p := effectiveAPIKeyPlatform(c, apiKey); p == service.PlatformAntigravity {
					diagPlatform = service.PlatformAntigravity
				}
				cls := classifyNoAccountErrorFromGin(c, h.gatewayService, apiKey, modelName, modelName, diagPlatform)
				if !cls.ModelNotFound {
					markOpsRoutingCapacityLimitedIfNoAvailable(c, err)
				}
				message := cls.Message
				if !cls.ModelNotFound {
					message = "No available Gemini accounts: " + err.Error()
				}
				googleError(c, cls.Status, message)
				return
			}
			action := fs.HandleSelectionExhausted(c.Request.Context())
			switch action {
			case FailoverContinue:
				ctx := service.WithSingleAccountRetry(c.Request.Context(), true, h.metadataBridgeEnabled())
				c.Request = c.Request.WithContext(ctx)
				continue
			case FailoverCanceled:
				failoverClientGone(c)
				return
			default: // FailoverExhausted
				h.handleGeminiFailoverExhausted(c, fs.LastFailoverErr)
				return
			}
		}
		account := selection.Account
		setOpsSelectedAccount(c, account.ID, account.Platform)

		// 检测账号切换：如果粘性会话绑定的账号与当前选择的账号不同，清除 thoughtSignature
		// 注意：Antigravity 转发层已具备 400 签名异常自动清理重试能力，因此在未命中绑定但极可能选回原账号时，
		// Antigravity 平台不提前覆写签名，以免破坏提示词前缀导致隐式缓存失效。
		if sessionBoundAccountID > 0 && sessionBoundAccountID != account.ID {
			reqLog.Info("gemini.sticky_session_account_switched",
				zap.Int64("from_account_id", sessionBoundAccountID),
				zap.Int64("to_account_id", account.ID),
				zap.Bool("clean_thought_signature", true),
			)
			body = service.CleanGeminiNativeThoughtSignatures(body)
			sessionBoundAccountID = account.ID
		} else if sessionKey != "" && sessionBoundAccountID == 0 && !cleanedForUnknownBinding &&
			account.Platform != service.PlatformAntigravity &&
			bytes.Contains(body, []byte(`"thoughtSignature"`)) {
			reqLog.Info("gemini.sticky_session_binding_missing",
				zap.Bool("clean_thought_signature", true),
			)
			body = service.CleanGeminiNativeThoughtSignatures(body)
			cleanedForUnknownBinding = true
			sessionBoundAccountID = account.ID
		} else if sessionBoundAccountID == 0 {
			// 记录本次请求中首次选择到的账号，便于同一请求内 failover 时检测切换。
			sessionBoundAccountID = account.ID
		}

		// 4) account concurrency slot
		accountReleaseFunc := selection.ReleaseFunc
		if !selection.Acquired {
			if selection.WaitPlan == nil {
				markOpsRoutingCapacityLimited(c)
				googleError(c, http.StatusServiceUnavailable, "No available Gemini accounts")
				return
			}
			accountWaitCounted := false
			canWait, err := geminiConcurrency.IncrementAccountWaitCount(c.Request.Context(), account.ID, selection.WaitPlan.MaxWaiting)
			if err != nil {
				reqLog.Warn("gemini.account_wait_counter_increment_failed", zap.Int64("account_id", account.ID), zap.Error(err))
			} else if !canWait {
				reqLog.Info("gemini.account_wait_queue_full",
					zap.Int64("account_id", account.ID),
					zap.Int("max_waiting", selection.WaitPlan.MaxWaiting),
				)
				googleError(c, http.StatusTooManyRequests, "Too many pending requests, please retry later")
				return
			}
			if err == nil && canWait {
				accountWaitCounted = true
			}
			defer func() {
				if accountWaitCounted {
					geminiConcurrency.DecrementAccountWaitCount(c.Request.Context(), account.ID)
				}
			}()

			accountReleaseFunc, err = geminiConcurrency.AcquireAccountSlotWithWaitTimeout(
				c,
				account.ID,
				selection.WaitPlan.MaxConcurrency,
				selection.WaitPlan.Timeout,
				stream,
				&streamStarted,
			)
			if err != nil {
				reqLog.Warn("gemini.account_slot_acquire_failed", zap.Int64("account_id", account.ID), zap.Error(err))
				googleConcurrencyError(c, err, "account")
				return
			}
			if accountWaitCounted {
				geminiConcurrency.DecrementAccountWaitCount(c.Request.Context(), account.ID)
				accountWaitCounted = false
			}
		}
		// 终检与准入后绑定使用选号结果携带的门（见 responses 同名注释）。
		admissionCtx := service.ContextWithSelectionProfitGate(c.Request.Context(), selection)
		latest, vetoed, reason := h.gatewayService.GatewayProfitControlVetoLatest(admissionCtx, account)
		if vetoed {
			if accountReleaseFunc != nil {
				accountReleaseFunc()
			}
			reqLog.Debug("gemini.account_slot_profit_vetoed", zap.Int64("account_id", account.ID), zap.String("reason", reason))
			if cachedContentBinding != nil {
				markOpsRoutingCapacityLimited(c)
				googleError(c, http.StatusServiceUnavailable, geminiCachedContentUnavailableMessage)
				return
			}
			if fs.RecordProfitVeto(account.ID) == FailoverExhausted {
				reqLog.Warn("gemini.profit_veto_attempts_exhausted", zap.Int("profit_veto_count", fs.ProfitVetoCount()))
				markOpsRoutingCapacityLimited(c)
				googleError(c, http.StatusServiceUnavailable, profitVetoExhaustedMessage)
				return
			}
			continue
		}
		account = latest
		selection.Account = latest
		if selection.ProfitGateActive() || !selection.Acquired {
			if err := h.gatewayService.BindStickySessionAfterProfitAdmission(admissionCtx, apiKey.GroupID, sessionKey, account.ID); err != nil {
				reqLog.Warn("gemini.bind_sticky_session_after_profit_admission_failed", zap.Int64("account_id", account.ID), zap.Error(err))
			}
		}
		accountReleaseFunc = wrapReleaseOnDone(c.Request.Context(), accountReleaseFunc)

		// 5) forward (根据平台分流)
		var result *service.ForwardResult
		requestCtx := c.Request.Context()
		if fs.SwitchCount > 0 {
			requestCtx = service.WithAccountSwitchCount(requestCtx, fs.SwitchCount, h.metadataBridgeEnabled())
		}
		sessionGroupID := derefGroupID(apiKey.GroupID)
		if account.Platform == service.PlatformAntigravity && account.Type != service.AccountTypeAPIKey {
			result, err = h.antigravityGatewayService.ForwardGemini(
				requestCtx,
				c,
				account,
				modelName,
				action,
				stream,
				body,
				hasBoundSession,
				service.WithForwardGeminiSession(sessionGroupID, sessionKey),
			)
		} else {
			result, err = h.geminiCompatService.ForwardNative(requestCtx, c, account, modelName, action, stream, body)
		}
		if accountReleaseFunc != nil {
			accountReleaseFunc()
		}
		if err != nil {
			var failoverErr *service.UpstreamFailoverError
			if errors.As(err, &failoverErr) {
				failoverAction := fs.HandleFailoverError(c.Request.Context(), h.gatewayService, account.ID, account.Platform, account.GetPoolModeRetryCount(), failoverErr)
				switch failoverAction {
				case FailoverContinue:
					continue
				case FailoverExhausted:
					if cachedContentBinding != nil {
						h.handleGeminiCachedContentFailoverExhausted(c, fs.LastFailoverErr)
						return
					}
					h.handleGeminiFailoverExhausted(c, fs.LastFailoverErr)
					return
				case FailoverCanceled:
					failoverClientGone(c)
					return
				}
			}
			failoverClientGone(c)
			reqLog.Error("gemini.forward_failed", zap.Int64("account_id", account.ID), zap.Error(err))
			return
		}

		userAgent := c.GetHeader("User-Agent")
		clientIP := ip.GetClientIP(c)

		// 保存 Gemini 内容摘要会话及 Redis 首轮锚点粘性会话
		h.persistGeminiStickySession(c, reqLog, apiKey.GroupID, account.ID, &sessionState)

		requestPayloadHash := service.HashUsageRequestPayload(body)
		inboundEndpoint := GetInboundEndpoint(c)
		upstreamEndpoint := GetUpstreamEndpoint(c, account.Platform)
		forceCacheBilling := fs.ForceCacheBilling
		quotaPlatform := service.QuotaPlatform(c.Request.Context(), apiKey)
		sessionID := service.ExtractClientSessionID(c)
		// 长上下文阶梯由目录数据驱动，统一在计费路径内生效，入口无需声明。
		channelUsageFields := clientRequestedUsageFields(c, channelMapping, reqModel, result.UpstreamModel)
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
				ForceCacheBilling:  forceCacheBilling,
				APIKeyService:      h.apiKeyService,
				SessionID:          sessionID,
				ChannelUsageFields: channelUsageFields,
			}); err != nil {
				logger.L().With(
					zap.String("component", "handler.gemini_v1beta.models"),
					zap.Int64("user_id", authSubject.UserID),
					zap.Int64("api_key_id", apiKey.ID),
					zap.Any("group_id", apiKey.GroupID),
					zap.String("model", modelName),
					zap.Int64("account_id", account.ID),
				).Error("gemini.record_usage_failed", zap.Error(err))
			}
		})
		reqLog.Debug("gemini.request_completed",
			zap.Int64("account_id", account.ID),
			zap.Int("switch_count", fs.SwitchCount),
		)
		return
	}
}
