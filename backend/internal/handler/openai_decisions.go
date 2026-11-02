package handler

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/decisionbridge"
	"github.com/Wei-Shaw/sub2api/internal/pkg/httputil"
	"github.com/Wei-Shaw/sub2api/internal/pkg/ip"
	openai_decisions "github.com/Wei-Shaw/sub2api/internal/pkg/openai_decisions"
	"github.com/Wei-Shaw/sub2api/internal/pkg/typesafe"
	middleware2 "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// Decisions handles OpenAI's typed, non-streaming Decisions API.
// POST /v1/decisions
func (h *OpenAIGatewayHandler) Decisions(c *gin.Context) {
	h.decisions(c, false)
}

// SystemOneViaDecisions uses the same eligibility, failover and billing path as
// native Decisions, but keeps the client's System One envelope at the edge.
func (h *OpenAIGatewayHandler) SystemOneViaDecisions(c *gin.Context) {
	h.decisions(c, true)
}

func (h *OpenAIGatewayHandler) decisions(c *gin.Context, bridge bool) {
	writeError := h.errorResponse
	if bridge {
		writeError = func(c *gin.Context, status int, typ, message string) {
			c.JSON(status, gin.H{"type": "error", "error": gin.H{"type": typ, "message": message}})
		}
	}
	failoverExhausted := func(f *service.UpstreamFailoverError) {
		if !bridge {
			h.handleFailoverExhausted(c, f, false)
			return
		}
		copyFailoverRetryAfter(c, f.ResponseHeaders)
		status, typ, message := h.mapUpstreamError(f.StatusCode)
		writeError(c, status, typ, message)
	}
	streamStarted := false
	defer h.recoverResponsesPanic(c, &streamStarted)
	started := time.Now()
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
	reqLog := requestLogger(c, "handler.openai_gateway.decisions",
		zap.Int64("user_id", subject.UserID), zap.Int64("api_key_id", apiKey.ID), zap.Any("group_id", apiKey.GroupID))
	if !h.ensureResponsesDependencies(c, reqLog) {
		return
	}
	body, err := httputil.ReadRequestBodyWithPrealloc(c.Request)
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
		body, err = decisionbridge.ToDecisions(clientBody)
		if err != nil {
			writeError(c, http.StatusBadRequest, "invalid_request_error", err.Error())
			return
		}
	}
	req, err := openai_decisions.ParseRequest(body)
	if err != nil {
		writeError(c, http.StatusBadRequest, "invalid_request_error", err.Error())
		return
	}
	if apiKey.Group.Platform != service.PlatformOpenAI && apiKey.Group.Platform != service.PlatformComposite {
		writeError(c, http.StatusNotFound, "not_found_error", "Decisions is only available for OpenAI and Composite groups")
		return
	}
	ensureCompositeTargetPlatform(c, apiKey, req.Model)
	if !compositeTargetPlatformAllowed(c, apiKey, req.Model, service.PlatformOpenAI) {
		writeError(c, http.StatusNotFound, "not_found_error", "Decisions only supports OpenAI targets for Composite groups")
		return
	}
	reqLog = reqLog.With(zap.String("model", req.Model), zap.Int("image_count", req.ImageCount))
	clientModel := req.Model
	if bridge {
		clientModel = typesafe.JevLatestModel
	}
	setOpsRequestContext(c, clientModel, false)
	setOpsEndpointContext(c, "", int16(service.RequestTypeSync))
	auditProtocol, auditModel := service.ContentModerationProtocolOpenAIDecisions, req.Model
	if bridge {
		auditProtocol, auditModel = service.ContentModerationProtocolTypeSafeSystemOne, typesafe.JevLatestModel
	}
	if decision := h.checkSecurityAudit(c, reqLog, apiKey, subject, auditProtocol, auditModel, clientBody); decision != nil && !decision.AllowNextStage {
		if bridge {
			h.anthropicSecurityAuditError(c, decision)
		} else {
			h.openAISecurityAuditError(c, decision)
		}
		return
	}
	channelMapping, restricted := h.gatewayService.ResolveChannelMappingAndRestrict(c.Request.Context(), apiKey.GroupID, req.Model)
	if bridge {
		channelMapping.BillingModelSource = service.BillingModelSourceUpstream
	}
	if restricted {
		writeError(c, http.StatusBadRequest, "invalid_request_error", "Decisions model is not allowed by the channel")
		return
	}
	if channelMapping.Mapped && channelMapping.MappedModel != req.Model {
		writeError(c, http.StatusBadRequest, "invalid_request_error", "Decisions channel mapping must preserve gpt-6-luna")
		return
	}
	subscription, _ := middleware2.GetSubscriptionFromContext(c)
	service.SetOpsLatencyMs(c, service.OpsAuthLatencyMsKey, time.Since(started).Milliseconds())
	var userRelease func()
	acquired := true
	if bridge {
		var slotErr error
		userRelease, slotErr = h.concurrencyHelper.AcquireUserSlotWithWait(c, subject.UserID, subject.Concurrency, false, &streamStarted)
		if slotErr != nil {
			status, typ, _, message := concurrencyErrorResponse(slotErr, "user")
			writeError(c, status, typ, message)
			acquired = false
		} else {
			userRelease = wrapReleaseOnDone(c.Request.Context(), userRelease)
		}
	} else {
		userRelease, acquired = h.acquireResponsesUserSlot(c, subject.UserID, subject.Concurrency, false, &streamStarted, reqLog)
	}
	if !acquired {
		return
	}
	if userRelease != nil {
		defer userRelease()
	}
	if err := h.billingCacheService.CheckBillingEligibility(c.Request.Context(), apiKey.User, apiKey, apiKey.Group, subscription, service.QuotaPlatform(c.Request.Context(), apiKey)); err != nil {
		status, code, message, retryAfter := billingErrorDetails(err)
		if retryAfter > 0 {
			c.Header("Retry-After", strconv.Itoa(retryAfter))
		}
		writeError(c, status, code, message)
		return
	}
	inflightRelease, err := reserveInflightBalance(c, h.billingCacheService, h.gatewayService, apiKey, subscription, tokenInflightEstimate(req.Model, body))
	if err != nil {
		status, code, message, retryAfter := billingErrorDetails(err)
		if retryAfter > 0 {
			c.Header("Retry-After", strconv.Itoa(retryAfter))
		}
		writeError(c, status, code, message)
		return
	}
	defer inflightRelease()
	pricingCtx, pricingAt := h.gatewayService.WithOpenAIRequestPricingContext(c.Request.Context(), apiKey.GroupID)
	c.Request = c.Request.WithContext(pricingCtx)
	sessionHash := h.gatewayService.GenerateSessionHash(c, body)
	failedIDs := make(map[int64]struct{})
	capability := service.OpenAIEndpointCapabilityDecisions
	if req.ImageCount > 1 {
		capability = service.OpenAIEndpointCapabilityDecisionsMultiImage
	}
	profitVetoCount := 0
	var lastFailover *service.UpstreamFailoverError
	maxSwitches := h.maxAccountSwitches
	if maxSwitches <= 0 {
		maxSwitches = 3
	}
	for switches := 0; ; {
		if failoverClientGone(c) {
			return
		}
		selection, _, selectErr := h.gatewayService.SelectAccountWithSchedulerForCapability(
			c.Request.Context(), apiKey.GroupID, "", sessionHash, req.Model, failedIDs,
			service.OpenAIUpstreamTransportHTTPSSE, capability,
			false, false, false, service.PlatformOpenAI,
		)
		if selectErr != nil || selection == nil || selection.Account == nil {
			if failoverClientGone(c) {
				return
			}
			if req.ImageCount > 1 && lastFailover == nil {
				writeError(c, http.StatusServiceUnavailable, "api_error", "No available Decisions accounts support this multi-image request")
				return
			}
			if lastFailover != nil {
				failoverExhausted(lastFailover)
			} else {
				cls := classifyOpenAICompatibleNoAccountErrorFromGin(c, h.gatewayService, apiKey, req.Model, req.Model)
				writeError(c, cls.Status, cls.ErrType, cls.Message)
			}
			return
		}
		account := selection.Account
		setOpsSelectedAccount(c, account.ID, account.Platform)
		var accountRelease func()
		var slotResult openAISlotAcquireResult
		if bridge {
			accountRelease, slotResult = h.acquireOpenAIAccountSlot(c, apiKey.GroupID, sessionHash, selection, false, &streamStarted, reqLog,
				func(status int, typ, _, message string) { writeError(c, status, typ, message) })
		} else {
			accountRelease, slotResult = h.acquireResponsesAccountSlot(c, apiKey.GroupID, sessionHash, selection, false, &streamStarted, reqLog)
		}
		if slotResult == openAISlotAcquireProfitVetoed {
			if !recordOpenAIProfitVeto(failedIDs, account.ID, &profitVetoCount) {
				if bridge {
					markOpsRoutingCapacityLimited(c)
					writeError(c, http.StatusServiceUnavailable, "api_error", profitVetoExhaustedMessage)
				} else {
					h.handleOpenAIProfitVetoExhausted(c, false, reqLog, profitVetoCount)
				}
				return
			}
			continue
		}
		if slotResult != openAISlotAcquireOK {
			return
		}
		account = selection.Account
		result, forwardErr := func() (*service.OpenAIDecisionsForwardResult, error) {
			if accountRelease != nil {
				defer accountRelease()
			}
			return h.gatewayService.ForwardDecisions(c.Request.Context(), c, account, body)
		}()
		if forwardErr != nil {
			if failoverClientGone(c) {
				return
			}
			var failoverErr *service.UpstreamFailoverError
			if errors.As(forwardErr, &failoverErr) {
				if failoverErr.ShouldReportAccountScheduleFailure() {
					h.gatewayService.ReportOpenAIAccountScheduleResult(account, req.Model, false, nil, forwardErr)
				}
				if !failoverErr.ShouldRetryNextAccount() {
					failoverExhausted(failoverErr)
					return
				}
				h.gatewayService.RecordOpenAIAccountSwitch()
				lastFailover = failoverErr
				failedIDs[account.ID] = struct{}{}
				if switches >= maxSwitches {
					failoverExhausted(failoverErr)
					return
				}
				switches++
				continue
			}
			writeError(c, http.StatusBadGateway, "upstream_error", "OpenAI Decisions upstream request failed")
			return
		}
		if bridge && (result.StatusCode < 200 || result.StatusCode >= 300) {
			copyFailoverRetryAfter(c, result.UpstreamHeaders)
			status, typ, message := h.mapUpstreamError(result.StatusCode)
			if result.StatusCode == http.StatusBadRequest || result.StatusCode == http.StatusRequestEntityTooLarge || result.StatusCode == http.StatusUnprocessableEntity {
				status, typ, message = result.StatusCode, "invalid_request_error", "Decisions upstream rejected the request"
			}
			writeError(c, status, typ, message)
			return
		}
		if bridge && result.StatusCode >= 200 && result.StatusCode < 300 {
			converted, conversionErr := decisionbridge.FromDecisions(clientBody, body, result.Body)
			if conversionErr != nil {
				service.SetOpsUpstreamError(c, result.StatusCode, "Decisions response cannot be represented as System One", "")
				reqLog.Warn("decision_bridge.response_conversion_failed", zap.Int64("account_id", account.ID))
				// The upstream succeeded and reported tokens even though the answer
				// cannot be exposed to this client. Keep the mandatory usage record.
				h.recordDecisionsUsage(c, apiKey, account, subscription, clientBody, &result.OpenAIForwardResult, pricingAt, subject.UserID, channelMapping)
				writeError(c, http.StatusBadGateway, "upstream_error", "Decisions response cannot be represented as System One")
				return
			}
			result.Body = converted
		}
		if result.StatusCode >= 200 && result.StatusCode < 300 {
			h.gatewayService.ReportOpenAIAccountScheduleResult(account, result.UpstreamModel, true, nil)
		}
		c.Data(result.StatusCode, result.ContentType, result.Body)
		if result.StatusCode >= 200 && result.StatusCode < 300 {
			h.recordDecisionsUsage(c, apiKey, account, subscription, clientBody, &result.OpenAIForwardResult, pricingAt, subject.UserID, channelMapping)
		}
		return
	}
}

func decisionsUsageSnapshot(c *gin.Context, apiKey *service.APIKey, account *service.Account, subscription *service.UserSubscription, body []byte, result *service.OpenAIForwardResult, pricingAt time.Time, mapping service.ChannelMappingResult) service.OpenAIRecordUsageInput {
	usageFields := clientRequestedUsageFields(c, mapping, openai_decisions.Model, result.UpstreamModel)
	if GetInboundEndpoint(c) == EndpointSystemOne {
		usageFields.OriginalModel = typesafe.JevLatestModel
	}
	return service.OpenAIRecordUsageInput{
		Result: result, APIKey: apiKey, User: apiKey.User, Account: account, Subscription: subscription,
		InboundEndpoint: GetInboundEndpoint(c), UpstreamEndpoint: result.UpstreamEndpoint,
		UserAgent: c.GetHeader("User-Agent"), IPAddress: ip.GetClientIP(c), SessionID: service.ExtractClientSessionID(c),
		RequestPayloadHash: service.HashUsageRequestPayload(body), QuotaPlatform: service.QuotaPlatform(c.Request.Context(), apiKey), PricingAt: pricingAt,
		ChannelUsageFields: usageFields,
	}
}

func (h *OpenAIGatewayHandler) recordDecisionsUsage(c *gin.Context, apiKey *service.APIKey, account *service.Account, subscription *service.UserSubscription, body []byte, result *service.OpenAIForwardResult, pricingAt time.Time, userID int64, mapping service.ChannelMappingResult) {
	input := decisionsUsageSnapshot(c, apiKey, account, subscription, body, result, pricingAt, mapping)
	input.APIKeyService = h.apiKeyService
	log := requestLogger(c, "handler.openai_gateway.decisions").With(zap.Int64("user_id", userID))
	h.submitMandatoryUsageRecordTask(c.Request.Context(), func(ctx context.Context) {
		if err := h.gatewayService.RecordUsage(ctx, &input); err != nil {
			log.Error("decisions.record_usage_failed", zap.Error(err))
		}
	})
}
