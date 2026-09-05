package handler

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"time"

	pkghttputil "github.com/Wei-Shaw/sub2api/internal/pkg/httputil"
	"github.com/Wei-Shaw/sub2api/internal/pkg/ip"
	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	middleware2 "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// AudioTranscriptions handles the OpenAI-compatible Audio Transcriptions API.
// POST /v1/audio/transcriptions
func (h *OpenAIGatewayHandler) AudioTranscriptions(c *gin.Context) {
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
	if !audioTranscriptionEnabledForAPIKey(apiKey) {
		h.errorResponse(c, http.StatusForbidden, "permission_error", "Audio transcription is not enabled for this group")
		return
	}
	reqLog := requestLogger(
		c,
		"handler.openai_gateway.audio_transcriptions",
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
	if len(body) == 0 {
		h.errorResponse(c, http.StatusBadRequest, "invalid_request_error", "Request body is empty")
		return
	}
	parsed, err := service.ParseOpenAIAudioTranscriptionRequest(c.GetHeader("Content-Type"), body)
	if err != nil {
		status := http.StatusBadRequest
		var reqErr *service.OpenAIAudioTranscriptionRequestError
		if errors.As(err, &reqErr) && reqErr.StatusCode > 0 {
			status = reqErr.StatusCode
		}
		h.errorResponse(c, status, "invalid_request_error", err.Error())
		return
	}
	reqModel := parsed.Model
	ensureCompositeTargetPlatform(c, apiKey, reqModel)
	if !compositeTargetPlatformAllowed(c, apiKey, reqModel, service.PlatformOpenAI) {
		h.errorResponse(c, http.StatusBadRequest, "invalid_request_error", "Model is not supported by this OpenAI-compatible endpoint for composite groups")
		return
	}
	reqLog = reqLog.With(zap.String("model", reqModel))
	setOpsRequestContext(c, reqModel, false)
	setOpsEndpointContext(c, "", int16(service.RequestTypeSync))
	moderationBody := parsed.ModerationBody()
	if decision := h.checkSecurityAudit(c, reqLog, apiKey, subject, service.ContentModerationProtocolOpenAIAudioTranscription, reqModel, moderationBody); decision != nil && !decision.AllowNextStage {
		h.openAISecurityAuditError(c, decision)
		return
	}

	channelMapping, _ := h.gatewayService.ResolveChannelMappingAndRestrict(c.Request.Context(), apiKey.GroupID, reqModel)
	channelMappedModel := ""
	if channelMapping.Mapped {
		channelMappedModel = channelMapping.MappedModel
	}

	subscription, _ := middleware2.GetSubscriptionFromContext(c)
	service.SetOpsLatencyMs(c, service.OpsAuthLatencyMsKey, time.Since(requestStart).Milliseconds())

	userReleaseFunc, acquired := h.acquireResponsesUserSlot(c, subject.UserID, subject.Concurrency, false, &streamStarted, reqLog)
	if !acquired {
		return
	}
	if userReleaseFunc != nil {
		defer userReleaseFunc()
	}

	if err := h.billingCacheService.CheckBillingEligibility(c.Request.Context(), apiKey.User, apiKey, apiKey.Group, subscription, service.QuotaPlatform(c.Request.Context(), apiKey)); err != nil {
		reqLog.Info("openai_audio_transcriptions.billing_check_failed", zap.Error(err))
		status, code, message, retryAfter := billingErrorDetails(err)
		if retryAfter > 0 {
			c.Header("Retry-After", strconv.Itoa(retryAfter))
		}
		h.errorResponse(c, status, code, message)
		return
	}

	profitVetoCount := 0
	failedAccountIDs := make(map[int64]struct{})
	var lastFailoverErr *service.UpstreamFailoverError
	switchCount := 0
	maxAccountSwitches := h.maxAccountSwitches
	if maxAccountSwitches <= 0 {
		maxAccountSwitches = 3
	}
	var oauth429FailoverState service.OpenAIOAuth429FailoverState
	routingStart := time.Now()

	pricingCtx, pricingAt := h.gatewayService.WithOpenAIRequestPricingContext(c.Request.Context(), apiKey.GroupID)
	c.Request = c.Request.WithContext(pricingCtx)

	for {
		// The ASR model name is a client-side alias unrelated to the account
		// model mapping, so it is not passed to the scheduler: it would exclude
		// every OAuth account whose mapping does not list it.
		selection, _, err := h.gatewayService.SelectAccountWithSchedulerForCapability(
			c.Request.Context(),
			apiKey.GroupID,
			"",
			"",
			"",
			failedAccountIDs,
			service.OpenAIUpstreamTransportHTTPSSE,
			service.OpenAIEndpointCapabilityAudioTranscriptions,
			false,
			false,
			false,
			service.PlatformOpenAI,
		)
		if err != nil || selection == nil || selection.Account == nil {
			if failoverClientGone(c) {
				reqLog.Info("openai_audio_transcriptions.account_select_aborted_client_disconnected", zap.Error(err))
				return
			}
			if len(failedAccountIDs) == 0 {
				reqLog.Warn("openai_audio_transcriptions.account_select_failed", zap.Error(err))
				cls := classifyNoAccountErrorFromGin(c, h.gatewayService, apiKey, "", reqModel, service.PlatformOpenAI)
				if !cls.ModelNotFound {
					markOpsRoutingCapacityLimitedIfNoAvailable(c, err)
				}
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

		accountReleaseFunc, slotResult := h.acquireResponsesAccountSlot(c, apiKey.GroupID, "", selection, false, &streamStarted, reqLog)
		if slotResult == openAISlotAcquireProfitVetoed {
			if !recordOpenAIProfitVeto(failedAccountIDs, account.ID, &profitVetoCount) {
				h.handleOpenAIProfitVetoExhausted(c, streamStarted, reqLog, profitVetoCount)
				return
			}
			continue
		}
		if slotResult != openAISlotAcquireOK {
			return
		}

		service.SetOpsLatencyMs(c, service.OpsRoutingLatencyMsKey, time.Since(routingStart).Milliseconds())
		forwardStart := time.Now()
		writerSizeBeforeForward := c.Writer.Size()
		result, err := func() (*service.OpenAIForwardResult, error) {
			defer func() {
				if accountReleaseFunc != nil {
					accountReleaseFunc()
				}
			}()
			return h.gatewayService.ForwardAudioTranscription(c.Request.Context(), c, account, parsed, channelMappedModel)
		}()
		service.SetOpsLatencyMs(c, service.OpsResponseLatencyMsKey, time.Since(forwardStart).Milliseconds())

		if err != nil {
			var failoverErr *service.UpstreamFailoverError
			if !errors.As(err, &failoverErr) {
				h.gatewayService.ReportOpenAIAccountScheduleResult(account, openAIAccountScheduleModel(c, account, reqModel, false, result), false, nil, err)
				if c.Writer.Size() == writerSizeBeforeForward {
					h.errorResponse(c, http.StatusBadGateway, "upstream_error", "Upstream request failed")
				}
				reqLog.Warn("openai_audio_transcriptions.forward_failed", zap.Int64("account_id", account.ID), zap.Error(err))
				return
			}
			h.gatewayService.ReportOpenAIAccountScheduleResult(account, openAIAccountScheduleModel(c, account, reqModel, false, result), false, nil, err)
			if c.Writer.Size() != writerSizeBeforeForward {
				h.handleFailoverExhausted(c, failoverErr, true)
				return
			}
			if failoverClientGone(c) {
				reqLog.Info("openai_audio_transcriptions.failover_aborted_client_disconnected",
					zap.Int64("account_id", account.ID),
					zap.Int("upstream_status", failoverErr.StatusCode),
				)
				return
			}
			h.gatewayService.RecordOpenAIAccountSwitch()
			failedAccountIDs[account.ID] = struct{}{}
			lastFailoverErr = failoverErr
			if switchCount >= maxAccountSwitches {
				h.handleFailoverExhausted(c, failoverErr, false)
				return
			}
			switchCount++
			if h.gatewayService.ShouldStopOpenAIOAuth429Failover(account, failoverErr.StatusCode, switchCount, &oauth429FailoverState) {
				h.handleFailoverExhausted(c, failoverErr, false)
				return
			}
			reqLog.Warn("openai_audio_transcriptions.upstream_failover_switching",
				zap.Int64("account_id", account.ID),
				zap.Int("upstream_status", failoverErr.StatusCode),
				zap.Int("switch_count", switchCount),
			)
			continue
		}

		h.gatewayService.ReportOpenAIAccountScheduleResult(account, openAIAccountScheduleModel(c, account, reqModel, false, result), true, nil)
		h.recordAudioTranscriptionUsage(c, apiKey, account, subscription, channelMapping, reqModel, moderationBody, result, subject.UserID, pricingAt)
		reqLog.Debug("openai_audio_transcriptions.request_completed",
			zap.Int64("account_id", account.ID),
			zap.Int("switch_count", switchCount),
		)
		return
	}
}

// recordAudioTranscriptionUsage bills a served transcription by audio duration.
// The upstream returns no usage to reconcile against, so the record goes through
// the mandatory pool like other per-unit media charges.
func (h *OpenAIGatewayHandler) recordAudioTranscriptionUsage(
	c *gin.Context,
	apiKey *service.APIKey,
	account *service.Account,
	subscription *service.UserSubscription,
	channelMapping service.ChannelMappingResult,
	requestedModel string,
	moderationBody []byte,
	result *service.OpenAIForwardResult,
	userID int64,
	pricingAt time.Time,
) {
	if result == nil {
		return
	}
	userAgent := c.GetHeader("User-Agent")
	clientIP := ip.GetClientIP(c)
	sessionID := service.ExtractClientSessionID(c)
	requestPayloadHash := service.HashUsageRequestPayload(moderationBody)
	inboundEndpoint := GetInboundEndpoint(c)
	upstreamEndpoint := resolveOpenAIUpstreamEndpoint(c, account, result)
	quotaPlatform := service.QuotaPlatform(c.Request.Context(), apiKey)
	usageFields := clientRequestedUsageFields(c, channelMapping, requestedModel, result.UpstreamModel)

	h.submitMandatoryUsageRecordTask(c.Request.Context(), func(ctx context.Context) {
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
			QuotaPlatform:      quotaPlatform,
			SessionID:          sessionID,
			ChannelUsageFields: usageFields,
			PricingAt:          pricingAt,
		}); err != nil {
			logger.L().With(
				zap.String("component", "handler.openai_gateway.audio_transcriptions"),
				zap.Int64("user_id", userID),
				zap.Int64("api_key_id", apiKey.ID),
				zap.Any("group_id", apiKey.GroupID),
				zap.String("model", requestedModel),
				zap.Int64("account_id", account.ID),
			).Error("openai_audio_transcriptions.record_usage_failed", zap.Error(err))
		}
	})
}

func audioTranscriptionEnabledForAPIKey(apiKey *service.APIKey) bool {
	return apiKey != nil &&
		apiKey.Group != nil &&
		(apiKey.Group.Platform == service.PlatformOpenAI || apiKey.Group.Platform == service.PlatformComposite) &&
		apiKey.Group.AllowAudioTranscription
}
