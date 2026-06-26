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

	reqLog := requestLogger(
		c,
		"handler.openai_gateway.audio_transcriptions",
		zap.Int64("user_id", subject.UserID),
		zap.Int64("api_key_id", apiKey.ID),
		zap.Any("group_id", apiKey.GroupID),
		zap.String("model", service.OpenAIAudioTranscriptionModel),
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
	parsed, err := h.gatewayService.ParseOpenAIAudioTranscriptionRequest(body, c.GetHeader("Content-Type"))
	if err != nil {
		var reqErr *service.OpenAIAudioTranscriptionRequestError
		if errors.As(err, &reqErr) {
			h.openAIAudioTranscriptionErrorResponse(c, reqErr.Status, reqErr.Type, reqErr.Message, reqErr.Param)
			return
		}
		h.errorResponse(c, http.StatusBadRequest, "invalid_request_error", err.Error())
		return
	}

	setOpsRequestContext(c, service.OpenAIAudioTranscriptionModel, false)
	setOpsEndpointContext(c, "", int16(service.RequestTypeSync))

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

	requestPayloadHash := service.HashOpenAIAudioTranscriptionPayload(parsed.Model, parsed.FileBytes, parsed.Prompt, parsed.ExtraFields...)
	sessionHash := h.gatewayService.GenerateExplicitSessionHash(c, []byte(requestPayloadHash))
	failedAccountIDs := make(map[int64]struct{})
	var lastFailoverErr *service.UpstreamFailoverError
	switchCount := 0
	maxAccountSwitches := h.maxAccountSwitches
	if maxAccountSwitches <= 0 {
		maxAccountSwitches = 3
	}
	routingStart := time.Now()

	for {
		selection, _, err := h.gatewayService.SelectAccountWithSchedulerForCapability(
			c.Request.Context(),
			apiKey.GroupID,
			"",
			sessionHash,
			"",
			failedAccountIDs,
			service.OpenAIUpstreamTransportHTTPSSE,
			service.OpenAIEndpointCapabilityAudioTranscribe,
			false,
		)
		if err != nil {
			reqLog.Warn("openai_audio_transcriptions.account_select_failed",
				zap.Error(err),
				zap.Int("excluded_account_count", len(failedAccountIDs)),
			)
			if len(failedAccountIDs) == 0 {
				markOpsRoutingCapacityLimitedIfNoAvailable(c, err)
				h.errorResponse(c, http.StatusServiceUnavailable, "api_error", "Service temporarily unavailable")
				return
			}
			if lastFailoverErr != nil {
				h.handleFailoverExhausted(c, lastFailoverErr, false)
			} else {
				h.errorResponse(c, http.StatusBadGateway, "api_error", "No billable compatible accounts")
			}
			return
		}
		if selection == nil || selection.Account == nil {
			markOpsRoutingCapacityLimited(c)
			h.errorResponse(c, http.StatusServiceUnavailable, "api_error", "No available accounts")
			return
		}
		account := selection.Account
		if account.Type == service.AccountTypeOAuth && !h.gatewayService.CanBillOpenAIAudioDuration(c.Request.Context(), apiKey, service.OpenAIAudioTranscriptionModel, parsed.DurationSeconds) {
			failedAccountIDs[account.ID] = struct{}{}
			reqLog.Warn("openai_audio_transcriptions.oauth_account_skipped_without_duration_billing",
				zap.Int64("account_id", account.ID),
				zap.Bool("duration_parsed", parsed.DurationParsed),
				zap.Int("duration_seconds", parsed.DurationSeconds),
			)
			continue
		}

		sessionHash = ensureOpenAIPoolModeSessionHash(sessionHash, account)
		setOpsSelectedAccount(c, account.ID, account.Platform)
		accountReleaseFunc, accountAcquired := h.acquireResponsesAccountSlot(c, apiKey.GroupID, sessionHash, selection, false, &streamStarted, reqLog)
		if !accountAcquired {
			return
		}

		service.SetOpsLatencyMs(c, service.OpsRoutingLatencyMsKey, time.Since(routingStart).Milliseconds())
		forwardStart := time.Now()
		output, err := func() (*service.OpenAIAudioTranscriptionOutput, error) {
			defer func() {
				if accountReleaseFunc != nil {
					accountReleaseFunc()
				}
			}()
			return h.gatewayService.ForwardAudioTranscription(c.Request.Context(), c, account, parsed)
		}()
		forwardDurationMs := time.Since(forwardStart).Milliseconds()
		upstreamLatencyMs, _ := getContextInt64(c, service.OpsUpstreamLatencyMsKey)
		responseLatencyMs := forwardDurationMs
		if upstreamLatencyMs > 0 && forwardDurationMs > upstreamLatencyMs {
			responseLatencyMs = forwardDurationMs - upstreamLatencyMs
		}
		service.SetOpsLatencyMs(c, service.OpsResponseLatencyMsKey, responseLatencyMs)

		if err != nil {
			var upstreamErr *service.OpenAIAudioTranscriptionUpstreamError
			if errors.As(err, &upstreamErr) {
				h.gatewayService.ReportOpenAIAccountScheduleResult(account.ID, true, nil)
				h.gatewayService.WriteOpenAIAudioTranscriptionUpstreamError(c, upstreamErr)
				return
			}
			var failoverErr *service.UpstreamFailoverError
			if errors.As(err, &failoverErr) {
				h.gatewayService.ReportOpenAIAccountScheduleResult(account.ID, false, nil)
				h.gatewayService.RecordOpenAIAccountSwitch()
				failedAccountIDs[account.ID] = struct{}{}
				lastFailoverErr = failoverErr
				if switchCount >= maxAccountSwitches {
					h.handleFailoverExhausted(c, failoverErr, false)
					return
				}
				switchCount++
				if h.gatewayService.ShouldStopOpenAIOAuth429Failover(account, failoverErr.StatusCode, switchCount) {
					h.handleFailoverExhausted(c, failoverErr, false)
					return
				}
				reqLog.Warn("openai_audio_transcriptions.upstream_failover_switching",
					zap.Int64("account_id", account.ID),
					zap.Int("upstream_status", failoverErr.StatusCode),
					zap.Int("switch_count", switchCount),
					zap.Int("max_switches", maxAccountSwitches),
				)
				continue
			}
			h.gatewayService.ReportOpenAIAccountScheduleResult(account.ID, false, nil)
			reqLog.Warn("openai_audio_transcriptions.forward_failed",
				zap.Int64("account_id", account.ID),
				zap.Error(err),
			)
			h.errorResponse(c, http.StatusBadGateway, "upstream_error", "Upstream request failed")
			return
		}

		h.gatewayService.ReportOpenAIAccountScheduleResult(account.ID, true, nil)
		if err := h.gatewayService.EnsureOpenAIAudioTranscriptionBillable(output); err != nil {
			reqLog.Error("openai_audio_transcriptions.unbillable_success_rejected",
				zap.Int64("account_id", account.ID),
				zap.Error(err),
			)
			h.errorResponse(c, http.StatusBadGateway, "upstream_error", "Upstream transcription usage unavailable")
			return
		}

		userAgent := c.GetHeader("User-Agent")
		clientIP := ip.GetClientIP(c)
		inboundEndpoint := GetInboundEndpoint(c)
		upstreamEndpoint := GetUpstreamEndpoint(c, account.Platform)
		channelUsage := service.ChannelUsageFields{
			OriginalModel:      service.OpenAIAudioTranscriptionModel,
			ChannelMappedModel: service.OpenAIAudioTranscriptionModel,
			BillingModelSource: service.BillingModelSourceRequested,
		}
		if mapping, _ := h.gatewayService.ResolveChannelMappingAndRestrict(c.Request.Context(), apiKey.GroupID, service.OpenAIAudioTranscriptionModel); mapping.ChannelID > 0 {
			channelUsage.ChannelID = mapping.ChannelID
		}

		if err := h.gatewayService.RecordUsage(context.Background(), &service.OpenAIRecordUsageInput{
			Result:             output.Result,
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
			ChannelUsageFields: channelUsage,
		}); err != nil {
			logger.L().With(
				zap.String("component", "handler.openai_gateway.audio_transcriptions"),
				zap.Int64("user_id", subject.UserID),
				zap.Int64("api_key_id", apiKey.ID),
				zap.Any("group_id", apiKey.GroupID),
				zap.String("model", service.OpenAIAudioTranscriptionModel),
				zap.Int64("account_id", account.ID),
			).Error("openai_audio_transcriptions.record_usage_failed", zap.Error(err))
			h.errorResponse(c, http.StatusInternalServerError, "api_error", "Failed to record usage")
			return
		}

		h.gatewayService.WriteOpenAIAudioTranscriptionResponse(c, output)
		reqLog.Debug("openai_audio_transcriptions.request_completed",
			zap.Int64("account_id", account.ID),
			zap.Int("switch_count", switchCount),
		)
		return
	}
}

func (h *OpenAIGatewayHandler) openAIAudioTranscriptionErrorResponse(c *gin.Context, status int, errType, message, param string) {
	errObj := gin.H{
		"type":    errType,
		"message": message,
	}
	if param != "" {
		errObj["param"] = param
	}
	c.JSON(status, gin.H{"error": errObj})
}
