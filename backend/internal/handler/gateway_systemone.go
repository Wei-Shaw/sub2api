package handler

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/decisionbridge"
	"github.com/Wei-Shaw/sub2api/internal/pkg/ip"
	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	openai_decisions "github.com/Wei-Shaw/sub2api/internal/pkg/openai_decisions"
	"github.com/Wei-Shaw/sub2api/internal/pkg/typesafe"
	middleware2 "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

const systemOneOnlyPlatformMessage = "TypeSafe models are only available through POST /v1/systemone"

// rejectSystemOneOnlyPlatform stops TypeSafe traffic from entering a non System
// One protocol chain. TypeSafe accounts only speak the native System One
// protocol; letting them reach the Anthropic/OpenAI converters would send
// foreign payloads (and the account key) to the wrong upstream path and feed
// the resulting auth failures back into account state.
func rejectSystemOneOnlyPlatform(c *gin.Context, apiKey *service.APIKey, writeError func(*gin.Context, int, string, string)) bool {
	if _, forced := middleware2.GetForcePlatformFromContext(c); forced {
		return false
	}
	if effectiveAPIKeyPlatform(c, apiKey) != service.PlatformTypeSafe {
		return false
	}
	service.MarkOpsClientBusinessLimited(c, service.OpsClientBusinessLimitedReasonLocalFeatureGate)
	writeError(c, http.StatusNotFound, "not_found_error", systemOneOnlyPlatformMessage)
	return true
}

// SystemOne proxies TypeSafe's native, non-streaming System One protocol.
func (h *GatewayHandler) SystemOne(c *gin.Context) {
	h.systemOne(c, false)
}

// DecisionsViaSystemOne retains the TypeSafe account selection and billing
// path while translating the public Decisions envelope at the edge.
func (h *GatewayHandler) DecisionsViaSystemOne(c *gin.Context) {
	h.systemOne(c, true)
}

func (h *GatewayHandler) systemOne(c *gin.Context, bridge bool) {
	writeError := h.errorResponse
	if bridge {
		writeError = func(c *gin.Context, status int, typ, message string) {
			c.JSON(status, gin.H{"error": gin.H{"type": typ, "message": message}})
		}
	}
	failoverExhausted := func(f *service.UpstreamFailoverError) {
		if !bridge {
			if f != nil {
				h.handleFailoverExhausted(c, f, service.PlatformTypeSafe, false)
			} else {
				h.handleFailoverExhaustedSimple(c, http.StatusBadGateway, false)
			}
			return
		}
		if f != nil {
			copyFailoverRetryAfter(c, f.ResponseHeaders)
		}
		status := http.StatusBadGateway
		if f != nil {
			status = f.StatusCode
		}
		mappedStatus, typ, message := h.mapUpstreamError(status)
		writeError(c, mappedStatus, typ, message)
	}
	requestStart := time.Now()
	apiKey, ok := middleware2.GetAPIKeyFromContext(c)
	if !ok || apiKey == nil || apiKey.Group == nil {
		writeError(c, http.StatusUnauthorized, "authentication_error", "Invalid API key")
		return
	}
	subject, ok := middleware2.GetAuthSubjectFromContext(c)
	if !ok {
		writeError(c, http.StatusInternalServerError, "api_error", "User context not found")
		return
	}
	reqLog := requestLogger(c, "handler.gateway.systemone",
		zap.Int64("user_id", subject.UserID),
		zap.Int64("api_key_id", apiKey.ID),
		zap.Any("group_id", apiKey.GroupID),
	)

	body, err := readLenientJSONRequestBodyWithPrealloc(c.Request, h.cfg)
	if err != nil {
		if maxErr, ok := extractMaxBytesError(err); ok {
			writeError(c, http.StatusRequestEntityTooLarge, "invalid_request_error", buildBodyTooLargeMessage(maxErr.Limit))
			return
		}
		writeError(c, http.StatusBadRequest, "invalid_request_error", "Failed to read request body")
		return
	}
	clientBody := body
	if bridge {
		body, err = decisionbridge.ToSystemOne(clientBody)
		if err != nil {
			writeError(c, http.StatusBadRequest, "invalid_request_error", err.Error())
			return
		}
	}
	model, err := typesafe.ValidateSystemOneRequest(body)
	if err != nil {
		writeError(c, http.StatusBadRequest, "invalid_request_error", err.Error())
		return
	}
	ensureCompositeTargetPlatform(c, apiKey, model)
	if apiKey.Group.Platform != service.PlatformTypeSafe &&
		(apiKey.Group.Platform != service.PlatformComposite || !compositeTargetPlatformAllowed(c, apiKey, model, service.PlatformTypeSafe)) {
		writeError(c, http.StatusNotFound, "not_found_error", "System One is only available for TypeSafe and compatible Composite groups")
		return
	}
	auditProtocol, auditModel := service.ContentModerationProtocolTypeSafeSystemOne, model
	if bridge {
		auditProtocol, auditModel = service.ContentModerationProtocolOpenAIDecisions, openai_decisions.Model
	}
	if decision := h.checkSecurityAudit(c, reqLog, apiKey, subject, auditProtocol, auditModel, clientBody); decision != nil && !decision.AllowNextStage {
		if bridge {
			h.openAISecurityAuditError(c, decision)
		} else {
			h.anthropicSecurityAuditError(c, decision)
		}
		return
	}

	clientModel := model
	if bridge {
		clientModel = openai_decisions.Model
	}
	setOpsRequestContext(c, clientModel, false)
	setOpsEndpointContext(c, "", int16(service.RequestTypeSync))
	service.SetOpsLatencyMs(c, service.OpsAuthLatencyMsKey, time.Since(requestStart).Milliseconds())
	pricingCtx, pricingAt := service.WithGatewayTokenRequestPricing(c.Request.Context())
	c.Request = c.Request.WithContext(pricingCtx)
	channelMapping, _ := h.gatewayService.ResolveChannelMappingAndRestrict(c.Request.Context(), apiKey.GroupID, model)
	if bridge {
		// Price by the account's actual model, never by the foreign ingress name.
		channelMapping.BillingModelSource = service.BillingModelSourceUpstream
	}
	subscription, _ := middleware2.GetSubscriptionFromContext(c)

	streamStarted := false
	userRelease, err := h.concurrencyHelper.AcquireUserSlotWithWait(c, subject.UserID, subject.Concurrency, false, &streamStarted)
	if err != nil {
		reqLog.Warn("systemone.user_slot_acquire_failed", zap.Error(err))
		if bridge {
			status, typ, _, message := concurrencyErrorResponse(err, "user")
			writeError(c, status, typ, message)
		} else {
			h.handleConcurrencyError(c, err, "user", false)
		}
		return
	}
	// 在请求结束或 Context 取消时确保释放槽位，避免客户端断开造成泄漏。
	userRelease = wrapReleaseOnDone(c.Request.Context(), userRelease)
	if userRelease != nil {
		defer userRelease()
	}
	if err := h.billingCacheService.CheckBillingEligibility(c.Request.Context(), apiKey.User, apiKey, apiKey.Group, subscription, service.QuotaPlatform(c.Request.Context(), apiKey)); err != nil {
		reqLog.Info("systemone.billing_eligibility_check_failed", zap.Error(err))
		status, code, message, retryAfter := billingErrorDetails(err)
		if retryAfter > 0 {
			c.Header("Retry-After", strconv.Itoa(retryAfter))
		}
		writeError(c, status, code, message)
		return
	}
	// 余额模式在途预留：防止并发请求在预检时看到同一份余额而集体透支。
	inflightRelease, err := reserveInflightBalance(c, h.billingCacheService, h.gatewayService, apiKey, subscription, tokenInflightEstimate(model, body))
	if err != nil {
		reqLog.Info("systemone.inflight_reservation_rejected", zap.Error(err))
		status, code, message, retryAfter := billingErrorDetails(err)
		if retryAfter > 0 {
			c.Header("Retry-After", strconv.Itoa(retryAfter))
		}
		writeError(c, status, code, message)
		return
	}
	defer inflightRelease()

	fs := NewFailoverState(h.maxAccountSwitches, false)
	for {
		if failoverClientGone(c) {
			return
		}
		selection, err := h.gatewayService.SelectAccountWithLoadAwareness(c.Request.Context(), apiKey.GroupID, "", model, fs.FailedAccountIDs, "", subject.UserID)
		if err == nil && (selection == nil || selection.Account == nil) {
			err = service.ErrNoAvailableAccounts
		}
		if err != nil {
			if failoverClientGone(c) {
				reqLog.Info("systemone.account_select_aborted_client_disconnected", zap.Error(err))
				return
			}
			if len(fs.FailedAccountIDs) == 0 {
				cls := classifyNoAccountErrorFromGin(c, h.gatewayService, apiKey, model, model, service.PlatformTypeSafe)
				cls = classifySelectionFailureError(err, cls)
				if !cls.ModelNotFound {
					markOpsRoutingCapacityLimitedIfNoAvailable(c, err)
				}
				reqLog.Warn("systemone.select_account_no_available", zap.Bool("model_not_found", cls.ModelNotFound), zap.Error(err))
				writeError(c, cls.Status, cls.ErrType, cls.Message)
				return
			}
			switch fs.HandleSelectionExhausted(c.Request.Context()) {
			case FailoverContinue:
				continue
			case FailoverCanceled:
				failoverClientGone(c)
				return
			default:
				failoverExhausted(fs.LastFailoverErr)
				return
			}
		}
		account := selection.Account

		accountRelease := selection.ReleaseFunc
		if !selection.Acquired {
			if selection.WaitPlan == nil {
				markOpsRoutingCapacityLimited(c)
				writeError(c, http.StatusServiceUnavailable, "api_error", "No available accounts")
				return
			}
			accountRelease, err = h.concurrencyHelper.AcquireAccountSlotWithWaitTimeout(c, account.ID, selection.WaitPlan.MaxConcurrency, selection.WaitPlan.Timeout, false, &streamStarted)
			if err != nil {
				reqLog.Warn("systemone.account_slot_acquire_failed", zap.Int64("account_id", account.ID), zap.Error(err))
				if bridge {
					status, typ, _, message := concurrencyErrorResponse(err, "account")
					writeError(c, status, typ, message)
				} else {
					h.handleConcurrencyError(c, err, "account", false)
				}
				return
			}
		}
		// 准入终检：与其他网关入口一致，利润控制否决的账号不得承接本次请求。
		admissionCtx := service.ContextWithSelectionProfitGate(c.Request.Context(), selection)
		latest, vetoed, reason := h.gatewayService.GatewayProfitControlVetoLatest(admissionCtx, account)
		if vetoed {
			if accountRelease != nil {
				accountRelease()
			}
			reqLog.Debug("systemone.account_slot_profit_vetoed", zap.Int64("account_id", account.ID), zap.String("reason", reason))
			if fs.RecordProfitVeto(account.ID) == FailoverExhausted {
				reqLog.Warn("systemone.profit_veto_attempts_exhausted", zap.Int("profit_veto_count", fs.ProfitVetoCount()))
				writeError(c, http.StatusServiceUnavailable, "api_error", profitVetoExhaustedMessage)
				return
			}
			continue
		}
		account = latest
		accountRelease = wrapReleaseOnDone(c.Request.Context(), accountRelease)
		setOpsSelectedAccount(c, account.ID, account.Platform)
		service.SetOpsUpstreamModel(c, model)

		forwardStart := time.Now()
		result, forwardErr := h.gatewayService.ForwardSystemOne(c.Request.Context(), c, account, body)
		if accountRelease != nil {
			accountRelease()
		}
		service.SetOpsLatencyMs(c, service.OpsResponseLatencyMsKey, time.Since(forwardStart).Milliseconds())

		if forwardErr != nil {
			var failoverErr *service.UpstreamFailoverError
			if errors.As(forwardErr, &failoverErr) {
				switch fs.HandleFailoverError(c.Request.Context(), h.gatewayService, account.ID, account.Platform, account.GetPoolModeRetryCount(), failoverErr) {
				case FailoverContinue:
					reqLog.Warn("systemone.upstream_failover_switching",
						zap.Int64("account_id", account.ID),
						zap.Int("upstream_status", failoverErr.StatusCode),
						zap.Int("switch_count", fs.SwitchCount),
					)
					continue
				case FailoverExhausted:
					failoverExhausted(fs.LastFailoverErr)
					return
				case FailoverCanceled:
					failoverClientGone(c)
					return
				}
			}
			if failoverClientGone(c) {
				return
			}
			var upstreamErr *service.SystemOneUpstreamError
			if errors.As(forwardErr, &upstreamErr) {
				status := upstreamErr.StatusCode
				if !service.IsSystemOneRequestErrorStatus(status) {
					status = http.StatusBadGateway
				}
				writeError(c, status, "upstream_error", "TypeSafe rejected the request")
				return
			}
			reqLog.Warn("systemone.forward_failed", zap.Int64("account_id", account.ID), zap.Error(forwardErr))
			if errors.Is(forwardErr, typesafe.ErrSystemOneResponseTooLarge) {
				writeError(c, http.StatusBadGateway, "upstream_error", "TypeSafe response exceeds the gateway size limit")
				return
			}
			writeError(c, http.StatusBadGateway, "upstream_error", "TypeSafe upstream request failed")
			return
		}

		if bridge {
			if result.StatusCode < 200 || result.StatusCode >= 300 {
				copyFailoverRetryAfter(c, result.UpstreamHeaders)
				status, typ, message := h.mapUpstreamError(result.StatusCode)
				writeError(c, status, typ, message)
				return
			}
			converted, conversionErr := decisionbridge.FromSystemOne(clientBody, result.Body)
			if conversionErr != nil {
				service.SetOpsUpstreamError(c, result.StatusCode, "System One response cannot be represented as Decisions", "")
				reqLog.Warn("decision_bridge.response_conversion_failed", zap.Int64("account_id", account.ID))
				h.recordSystemOneUsage(c, apiKey, account, subscription, channelMapping, model, clientBody, result, subject.UserID, pricingAt)
				writeError(c, http.StatusBadGateway, "upstream_error", "System One response cannot be represented as Decisions")
				return
			}
			result.Body = converted
		}
		c.Data(result.StatusCode, result.ContentType, result.Body)
		h.recordSystemOneUsage(c, apiKey, account, subscription, channelMapping, model, clientBody, result, subject.UserID, pricingAt)
		return
	}
}

func (h *GatewayHandler) recordSystemOneUsage(c *gin.Context, apiKey *service.APIKey, account *service.Account, subscription *service.UserSubscription, mapping service.ChannelMappingResult, model string, body []byte, result *service.SystemOneForwardResult, userID int64, pricingAt time.Time) {
	userAgent := c.GetHeader("User-Agent")
	clientIP := ip.GetClientIP(c)
	inboundEndpoint := GetInboundEndpoint(c)
	upstreamEndpoint := GetUpstreamEndpoint(c, account.Platform)
	quotaPlatform := service.QuotaPlatform(c.Request.Context(), apiKey)
	sessionID := service.ExtractClientSessionID(c)
	requestPayloadHash := service.HashUsageRequestPayload(body)
	usageFields := clientRequestedUsageFields(c, mapping, model, result.UpstreamModel)
	if inboundEndpoint == EndpointDecisions {
		usageFields.OriginalModel = openai_decisions.Model
	}

	h.submitMandatoryUsageRecordTask(c.Request.Context(), func(ctx context.Context) {
		if err := h.gatewayService.RecordUsage(ctx, &service.RecordUsageInput{
			Result:             &result.ForwardResult,
			APIKey:             apiKey,
			User:               apiKey.User,
			Account:            account,
			Subscription:       subscription,
			PricingAt:          pricingAt,
			InboundEndpoint:    inboundEndpoint,
			UpstreamEndpoint:   upstreamEndpoint,
			UserAgent:          userAgent,
			IPAddress:          clientIP,
			SessionID:          sessionID,
			RequestPayloadHash: requestPayloadHash,
			APIKeyService:      h.apiKeyService,
			QuotaPlatform:      quotaPlatform,
			ChannelUsageFields: usageFields,
		}); err != nil {
			logger.L().With(
				zap.String("component", "handler.gateway.systemone"),
				zap.Int64("user_id", userID),
				zap.Int64("api_key_id", apiKey.ID),
				zap.Int64("account_id", account.ID),
			).Error("systemone.record_usage_failed", zap.Error(err))
		}
	})
}
