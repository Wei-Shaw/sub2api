package handler

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"mime"
	"mime/multipart"
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
	"go.uber.org/zap"
)

type openAITranscriptionRequest struct {
	Model string
}

// Transcriptions handles OpenAI-compatible multipart audio transcription.
// POST /v1/audio/transcriptions
func (h *OpenAIGatewayHandler) Transcriptions(c *gin.Context) {
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
	reqLog := requestLogger(c, "handler.openai_gateway.transcriptions",
		zap.Int64("user_id", subject.UserID),
		zap.Int64("api_key_id", apiKey.ID),
		zap.Any("group_id", apiKey.GroupID),
	)
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
	parsed, err := parseOpenAITranscriptionRequest(body, c.GetHeader("Content-Type"))
	if err != nil {
		h.errorResponse(c, http.StatusBadRequest, "invalid_request_error", err.Error())
		return
	}

	reqModel := parsed.Model
	ensureCompositeTargetPlatform(c, apiKey, reqModel)
	if !compositeTargetPlatformAllowed(c, apiKey, reqModel, service.PlatformOpenAI) {
		h.errorResponse(c, http.StatusBadRequest, "invalid_request_error", "Model is not supported by this OpenAI-compatible endpoint for composite groups")
		return
	}
	setOpsRequestContext(c, reqModel, false)
	setOpsEndpointContext(c, "", int16(service.RequestTypeSync))
	channelMapping, _ := h.gatewayService.ResolveChannelMappingAndRestrict(c.Request.Context(), apiKey.GroupID, reqModel)
	routingModel := channelMapping.MappedModel
	if strings.TrimSpace(routingModel) == "" {
		routingModel = reqModel
	}

	subscription, _ := middleware2.GetSubscriptionFromContext(c)
	userRelease, acquired := h.acquireResponsesUserSlot(c, subject.UserID, subject.Concurrency, false, &streamStarted, reqLog)
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
		h.errorResponse(c, status, code, message)
		return
	}
	service.SetOpsLatencyMs(c, service.OpsAuthLatencyMsKey, time.Since(requestStart).Milliseconds())

	pricingCtx, pricingAt := h.gatewayService.WithOpenAIRequestPricingContext(c.Request.Context(), apiKey.GroupID)
	c.Request = c.Request.WithContext(pricingCtx)
	failedAccountIDs := make(map[int64]struct{})
	var lastFailoverErr *service.UpstreamFailoverError
	maxAccountSwitches := h.maxAccountSwitches
	if maxAccountSwitches <= 0 {
		maxAccountSwitches = 3
	}

	for switchCount := 0; ; {
		selection, _, selectErr := h.gatewayService.SelectAccountWithSchedulerForCapability(
			c.Request.Context(), apiKey.GroupID, "", "", routingModel, failedAccountIDs,
			service.OpenAIUpstreamTransportHTTPSSE, service.OpenAIEndpointCapabilityTranscriptions,
			false, false, true,
		)
		if selectErr != nil || selection == nil || selection.Account == nil {
			if len(failedAccountIDs) == 0 {
				cls := classifyNoAccountErrorFromGin(c, h.gatewayService, apiKey, reqModel, routingModel, service.PlatformOpenAI)
				h.errorResponse(c, cls.Status, cls.ErrType, cls.Message)
				return
			}
			if lastFailoverErr != nil {
				h.handleFailoverExhausted(c, lastFailoverErr, false)
			} else {
				h.errorResponse(c, http.StatusBadGateway, "upstream_error", "Upstream request failed")
			}
			return
		}
		account := selection.Account
		setOpsSelectedAccount(c, account.ID, account.Platform)
		accountRelease, slotStatus := h.acquireResponsesAccountSlot(c, apiKey.GroupID, "", selection, false, &streamStarted, reqLog)
		if slotStatus == openAISlotAcquireProfitVetoed {
			failedAccountIDs[account.ID] = struct{}{}
			continue
		}
		if slotStatus != openAISlotAcquireOK {
			return
		}

		writerSizeBeforeForward := c.Writer.Size()
		result, forwardErr := func() (*service.OpenAIForwardResult, error) {
			defer accountRelease()
			return h.gatewayService.ForwardTranscriptions(c.Request.Context(), c, account, body, c.GetHeader("Content-Type"), routingModel)
		}()
		if forwardErr != nil {
			var failoverErr *service.UpstreamFailoverError
			if errors.As(forwardErr, &failoverErr) {
				if c.Writer.Size() != writerSizeBeforeForward {
					h.handleFailoverExhausted(c, failoverErr, true)
					return
				}
				h.gatewayService.ReportOpenAIAccountScheduleResult(account, openAIAccountScheduleModel(c, account, routingModel, false, result), false, nil, forwardErr)
				failedAccountIDs[account.ID] = struct{}{}
				lastFailoverErr = failoverErr
				if switchCount >= maxAccountSwitches {
					h.handleFailoverExhausted(c, failoverErr, false)
					return
				}
				switchCount++
				continue
			}
			h.gatewayService.ReportOpenAIAccountScheduleResult(account, routingModel, false, nil, forwardErr)
			if c.Writer.Size() == writerSizeBeforeForward {
				h.errorResponse(c, http.StatusBadGateway, "upstream_error", "Upstream request failed")
			}
			return
		}

		result.Model = reqModel
		result.BillingModel = routingModel
		h.gatewayService.ReportOpenAIAccountScheduleResult(account, openAIAccountScheduleModel(c, account, routingModel, false, result), true, nil)
		h.recordOpenAITranscriptionUsage(c, apiKey, &subject, account, subscription, body, result, channelMapping, reqModel, pricingAt)
		return
	}
}

func (h *OpenAIGatewayHandler) recordOpenAITranscriptionUsage(
	c *gin.Context,
	apiKey *service.APIKey,
	subject *middleware2.AuthSubject,
	account *service.Account,
	subscription *service.UserSubscription,
	body []byte,
	result *service.OpenAIForwardResult,
	channelMapping service.ChannelMappingResult,
	requestModel string,
	pricingAt time.Time,
) {
	if h == nil || c == nil || apiKey == nil || subject == nil || account == nil || result == nil {
		return
	}
	userAgent := c.GetHeader("User-Agent")
	clientIP := ip.GetClientIP(c)
	inboundEndpoint := GetInboundEndpoint(c)
	upstreamEndpoint := GetUpstreamEndpoint(c, account.Platform)
	quotaPlatform := service.QuotaPlatform(c.Request.Context(), apiKey)
	sessionID := service.ExtractClientSessionID(c)
	h.submitOpenAIUsageRecordTask(c.Request.Context(), result, func(ctx context.Context) {
		if err := h.gatewayService.RecordUsage(ctx, &service.OpenAIRecordUsageInput{
			Result: result, APIKey: apiKey, User: apiKey.User, Account: account, Subscription: subscription,
			InboundEndpoint: inboundEndpoint, UpstreamEndpoint: upstreamEndpoint, UserAgent: userAgent,
			IPAddress: clientIP, RequestPayloadHash: service.HashUsageRequestPayload(body), APIKeyService: h.apiKeyService,
			QuotaPlatform: quotaPlatform, SessionID: sessionID,
			ChannelUsageFields: clientRequestedUsageFields(c, channelMapping, requestModel, result.UpstreamModel), PricingAt: pricingAt,
		}); err != nil {
			logger.L().With(zap.String("component", "handler.openai_gateway.transcriptions"), zap.Int64("user_id", subject.UserID), zap.Int64("account_id", account.ID)).Error("openai_transcriptions.record_usage_failed", zap.Error(err))
		}
	})
}

// parseOpenAITranscriptionRequest validates the OpenAI multipart shape without
// consuming the request body that must later be forwarded upstream intact.
func parseOpenAITranscriptionRequest(body []byte, contentType string) (openAITranscriptionRequest, error) {
	mediaType, params, err := mime.ParseMediaType(contentType)
	if err != nil || !strings.EqualFold(mediaType, "multipart/form-data") || strings.TrimSpace(params["boundary"]) == "" {
		return openAITranscriptionRequest{}, fmt.Errorf("Content-Type must be multipart/form-data")
	}
	form, err := multipart.NewReader(bytes.NewReader(body), params["boundary"]).ReadForm(32 << 20)
	if err != nil {
		return openAITranscriptionRequest{}, fmt.Errorf("invalid multipart form")
	}
	defer form.RemoveAll()
	modelValues := form.Value["model"]
	model := ""
	if len(modelValues) > 0 {
		model = strings.TrimSpace(modelValues[0])
	}
	if model == "" {
		return openAITranscriptionRequest{}, fmt.Errorf("model is required")
	}
	if len(form.File["file"]) == 0 || strings.TrimSpace(form.File["file"][0].Filename) == "" {
		return openAITranscriptionRequest{}, fmt.Errorf("file is required")
	}
	return openAITranscriptionRequest{Model: model}, nil
}
