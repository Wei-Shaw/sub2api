package handler

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	pkghttputil "github.com/Wei-Shaw/sub2api/internal/pkg/httputil"
	"github.com/Wei-Shaw/sub2api/internal/pkg/ip"
	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	middleware2 "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
	"go.uber.org/zap"
)

// Rerank handles the OpenRouter-compatible rerank API.
// POST /v1/rerank and POST /rerank
func (h *OpenAIGatewayHandler) Rerank(c *gin.Context) {
	streamStarted := false
	requestStart := time.Now()

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
	if apiKey.Group != nil && apiKey.Group.Platform != service.PlatformComposite && apiKey.Group.Platform != service.PlatformOpenAI {
		service.MarkOpsClientBusinessLimited(c, service.OpsClientBusinessLimitedReasonLocalFeatureGate)
		h.errorResponse(c, http.StatusNotFound, "not_found_error", "Rerank API is not supported for this provider")
		return
	}
	reqLog := requestLogger(c, "handler.openai_gateway.rerank",
		zap.Int64("user_id", subject.UserID), zap.Int64("api_key_id", apiKey.ID), zap.Any("group_id", apiKey.GroupID))
	if !h.ensureResponsesDependencies(c, reqLog) {
		return
	}

	body, err := pkghttputil.ReadRequestBodyWithPrealloc(c.Request)
	if err != nil {
		if maxErr, ok := extractMaxBytesError(err); ok {
			h.errorResponse(c, http.StatusRequestEntityTooLarge, "invalid_request_error", buildBodyTooLargeMessage(maxErr.Limit))
			return
		}
		h.errorResponse(c, http.StatusBadRequest, "invalid_request_error", "Failed to read request body")
		return
	}
	if len(body) == 0 || !gjson.ValidBytes(body) {
		h.errorResponse(c, http.StatusBadRequest, "invalid_request_error", "Invalid request body")
		return
	}
	model := strings.TrimSpace(gjson.GetBytes(body, "model").String())
	if model == "" {
		h.errorResponse(c, http.StatusBadRequest, "invalid_request_error", "model is required")
		return
	}
	ensureCompositeTargetPlatform(c, apiKey, model)
	routingPlatform := effectiveAPIKeyPlatform(c, apiKey)
	if !compositeTargetPlatformAllowed(c, apiKey, model, service.PlatformOpenAI) || routingPlatform != service.PlatformOpenAI {
		service.MarkOpsClientBusinessLimited(c, service.OpsClientBusinessLimitedReasonLocalFeatureGate)
		h.errorResponse(c, http.StatusNotFound, "not_found_error", "Rerank API is not supported for this provider")
		return
	}
	query := gjson.GetBytes(body, "query")
	if !query.Exists() || query.Type != gjson.String || strings.TrimSpace(query.String()) == "" || !gjson.GetBytes(body, "documents").Exists() {
		h.errorResponse(c, http.StatusBadRequest, "invalid_request_error", "query and documents are required")
		return
	}
	if !validOpenRouterRerankDocuments(gjson.GetBytes(body, "documents")) {
		h.errorResponse(c, http.StatusBadRequest, "invalid_request_error", "documents must be an array of strings or {text,image} objects")
		return
	}
	reqLog = reqLog.With(zap.String("model", model))
	setOpsRequestContext(c, model, false)
	setOpsEndpointContext(c, "", int16(service.RequestTypeSync))
	if decision := h.checkSecurityAudit(c, reqLog, apiKey, subject, "openai_rerank", model, body); decision != nil && !decision.AllowNextStage {
		h.openAISecurityAuditError(c, decision)
		return
	}

	channelMapping, _ := h.gatewayService.ResolveChannelMappingAndRestrict(c.Request.Context(), apiKey.GroupID, model)
	forwardModel := openAIChannelForwardModel(channelMapping, model)
	subscription, _ := middleware2.GetSubscriptionFromContext(c)
	quotaPlatform := service.QuotaPlatform(c.Request.Context(), apiKey)
	service.SetOpsLatencyMs(c, service.OpsAuthLatencyMsKey, time.Since(requestStart).Milliseconds())
	userReleaseFunc, acquired := h.acquireResponsesUserSlot(c, subject.UserID, subject.Concurrency, false, &streamStarted, reqLog)
	if !acquired {
		return
	}
	if userReleaseFunc != nil {
		defer userReleaseFunc()
	}
	if err := h.billingCacheService.CheckBillingEligibility(c.Request.Context(), apiKey.User, apiKey, apiKey.Group, subscription, service.QuotaPlatform(c.Request.Context(), apiKey)); err != nil {
		status, code, message, retryAfter := billingErrorDetails(err)
		if retryAfter > 0 {
			c.Header("Retry-After", strconv.Itoa(retryAfter))
		}
		h.errorResponse(c, status, code, message)
		return
	}

	failedAccountIDs := make(map[int64]struct{})
	var lastFailoverErr *service.UpstreamFailoverError
	switchCount := 0
	maxAccountSwitches := h.maxAccountSwitches
	if maxAccountSwitches <= 0 {
		maxAccountSwitches = 3
	}
	routingStart := time.Now()
	rerankPricingCtx, pricingAt := h.gatewayService.WithOpenAIRequestPricingContext(c.Request.Context(), apiKey.GroupID)
	c.Request = c.Request.WithContext(rerankPricingCtx)

	for {
		selection, _, err := h.gatewayService.SelectAccountWithSchedulerForCapability(
			c.Request.Context(), apiKey.GroupID, "", "", forwardModel, failedAccountIDs,
			service.OpenAIUpstreamTransportHTTPSSE, service.OpenAIEndpointCapabilityRerank,
			false, false, true, routingPlatform,
		)
		if err != nil {
			if len(failedAccountIDs) == 0 {
				service.MarkOpsClientBusinessLimited(c, service.OpsClientBusinessLimitedReasonLocalFeatureGate)
				h.errorResponse(c, http.StatusNotFound, "not_found_error", "Rerank API requires an existing OpenAI API-key account configured for OpenRouter")
				return
			}
			if lastFailoverErr != nil {
				h.handleFailoverExhausted(c, lastFailoverErr, false)
			} else {
				h.errorResponse(c, http.StatusBadGateway, "api_error", "Upstream request failed")
			}
			return
		}
		if selection == nil || selection.Account == nil {
			h.errorResponse(c, http.StatusNotFound, "not_found_error", "Rerank API requires an existing OpenAI API-key account configured for OpenRouter")
			return
		}
		account := selection.Account
		setOpsSelectedAccount(c, account.ID, account.Platform)
		accountReleaseFunc, slotResult := h.acquireResponsesAccountSlot(c, apiKey.GroupID, "", selection, false, &streamStarted, reqLog)
		if slotResult == openAISlotAcquireProfitVetoed {
			failedAccountIDs[account.ID] = struct{}{}
			continue
		}
		if slotResult != openAISlotAcquireOK {
			return
		}
		service.SetOpsLatencyMs(c, service.OpsRoutingLatencyMsKey, time.Since(routingStart).Milliseconds())
		forwardBody := body
		if channelMapping.Mapped {
			forwardBody = h.gatewayService.ReplaceModelInBody(body, channelMapping.MappedModel)
		}
		writerSizeBeforeForward := c.Writer.Size()
		result, err := func() (*service.OpenAIForwardResult, error) {
			defer func() {
				if accountReleaseFunc != nil {
					accountReleaseFunc()
				}
			}()
			return h.gatewayService.ForwardRerank(c.Request.Context(), c, account, forwardBody, "")
		}()
		if err != nil {
			var failoverErr *service.UpstreamFailoverError
			if errors.As(err, &failoverErr) {
				if c.Writer.Size() != writerSizeBeforeForward {
					h.handleFailoverExhausted(c, failoverErr, true)
					return
				}
				h.gatewayService.ReportOpenAIAccountScheduleResult(account, openAIAccountScheduleModel(c, account, model, false, result), false, nil, err)
				failedAccountIDs[account.ID] = struct{}{}
				lastFailoverErr = failoverErr
				if switchCount >= maxAccountSwitches {
					h.handleFailoverExhausted(c, failoverErr, false)
					return
				}
				switchCount++
				continue
			}
			h.gatewayService.ReportOpenAIAccountScheduleResult(account, openAIAccountScheduleModel(c, account, model, false, result), false, nil, err)
			if c.Writer.Size() == writerSizeBeforeForward {
				h.errorResponse(c, http.StatusBadGateway, "upstream_error", "Upstream request failed")
			}
			return
		}

		h.gatewayService.ReportOpenAIAccountScheduleResult(account, openAIAccountScheduleModel(c, account, model, false, result), true, nil)
		userAgent := c.GetHeader("User-Agent")
		clientIP := ip.GetClientIP(c)
		inboundEndpoint := GetInboundEndpoint(c)
		upstreamEndpoint := GetUpstreamEndpoint(c, account.Platform)
		sessionID := service.ExtractClientSessionID(c)
		h.submitOpenAIUsageRecordTask(c.Request.Context(), result, func(ctx context.Context) {
			if err := h.gatewayService.RecordUsage(ctx, &service.OpenAIRecordUsageInput{
				Result: result, APIKey: apiKey, User: apiKey.User, Account: account, Subscription: subscription,
				InboundEndpoint: inboundEndpoint, UpstreamEndpoint: upstreamEndpoint, UserAgent: userAgent,
				IPAddress: clientIP, SessionID: sessionID, APIKeyService: h.apiKeyService,
				QuotaPlatform: quotaPlatform, PricingAt: pricingAt,
				ChannelUsageFields: clientRequestedUsageFields(c, channelMapping, model, result.UpstreamModel),
			}); err != nil {
				logger.L().With(zap.String("component", "handler.openai_gateway.rerank"), zap.Int64("account_id", account.ID)).Error("rerank.record_usage_failed", zap.Error(err))
			}
		})
		return
	}
}

func validOpenRouterRerankDocuments(value gjson.Result) bool {
	if !value.Exists() || !value.IsArray() || len(value.Array()) == 0 {
		return false
	}
	for _, document := range value.Array() {
		switch document.Type {
		case gjson.String:
			if strings.TrimSpace(document.String()) == "" {
				return false
			}
		case gjson.JSON:
			if !document.IsObject() {
				return false
			}
			text := document.Get("text")
			image := document.Get("image")
			if !text.Exists() && !image.Exists() {
				return false
			}
			if text.Exists() && text.Type != gjson.String {
				return false
			}
		default:
			return false
		}
	}
	return true
}
