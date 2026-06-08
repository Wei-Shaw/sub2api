package handler

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/ctxkey"
	pkghttputil "github.com/Wei-Shaw/sub2api/internal/pkg/httputil"
	"github.com/Wei-Shaw/sub2api/internal/pkg/ip"
	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	middleware2 "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
	"go.uber.org/zap"
)

// Videos handles xAI-compatible video generation requests.
// POST /v1/videos/generations
func (h *OpenAIGatewayHandler) Videos(c *gin.Context) {
	streamStarted := false
	defer h.recoverResponsesPanic(c, &streamStarted)

	requestStart := time.Now()

	apiKey, ok := middleware2.GetAPIKeyFromContext(c)
	if !ok {
		h.errorResponse(c, http.StatusUnauthorized, "authentication_error", "Invalid API key")
		return
	}
	if platform := service.PlatformFromAPIKey(apiKey); platform == service.PlatformXAI {
		c.Request = c.Request.WithContext(context.WithValue(c.Request.Context(), ctxkey.Platform, platform))
	}

	subject, ok := middleware2.GetAuthSubjectFromContext(c)
	if !ok {
		h.errorResponse(c, http.StatusInternalServerError, "api_error", "User context not found")
		return
	}
	reqLog := requestLogger(
		c,
		"handler.openai_gateway.videos",
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
	if !gjson.ValidBytes(body) {
		h.errorResponse(c, http.StatusBadRequest, "invalid_request_error", "Failed to parse request body")
		return
	}

	model := strings.TrimSpace(gjson.GetBytes(body, "model").String())
	if model == "" {
		h.errorResponse(c, http.StatusBadRequest, "invalid_request_error", "model is required")
		return
	}
	reqLog = reqLog.With(zap.String("model", model))
	setOpsRequestContext(c, model, false)
	setOpsEndpointContext(c, "", int16(service.RequestTypeFromLegacy(false, false)))

	if !service.GroupAllowsImageGeneration(apiKey.Group) {
		h.errorResponse(c, http.StatusForbidden, "permission_error", service.ImageGenerationPermissionMessage())
		return
	}
	if decision := h.checkContentModeration(c, reqLog, apiKey, subject, service.ContentModerationProtocolOpenAIImages, model, body); decision != nil && decision.Blocked {
		h.errorResponse(c, contentModerationStatus(decision), contentModerationErrorCode(decision), decision.Message)
		return
	}

	imageReleaseFunc, acquired := h.acquireImageGenerationSlot(c, streamStarted)
	if !acquired {
		return
	}
	if imageReleaseFunc != nil {
		defer imageReleaseFunc()
	}

	channelMapping, _ := h.gatewayService.ResolveChannelMappingAndRestrict(c.Request.Context(), apiKey.GroupID, model)
	if h.errorPassthroughService != nil {
		service.BindErrorPassthroughService(c, h.errorPassthroughService)
	}
	subscription, _ := middleware2.GetSubscriptionFromContext(c)

	service.SetOpsLatencyMs(c, service.OpsAuthLatencyMsKey, time.Since(requestStart).Milliseconds())
	routingStart := time.Now()

	userReleaseFunc, acquired := h.acquireResponsesUserSlot(c, subject.UserID, subject.Concurrency, false, &streamStarted, reqLog)
	if !acquired {
		return
	}
	if userReleaseFunc != nil {
		defer userReleaseFunc()
	}

	if err := h.billingCacheService.CheckBillingEligibility(c.Request.Context(), apiKey.User, apiKey, apiKey.Group, subscription, service.QuotaPlatform(c.Request.Context(), apiKey)); err != nil {
		reqLog.Info("openai.videos.billing_eligibility_check_failed", zap.Error(err))
		status, code, message, retryAfter := billingErrorDetails(err)
		if retryAfter > 0 {
			c.Header("Retry-After", strconv.Itoa(retryAfter))
		}
		h.handleStreamingAwareError(c, status, code, message, streamStarted)
		return
	}

	sessionHash := h.gatewayService.GenerateExplicitSessionHash(c, body)
	requestCtx := c.Request.Context()
	failedAccountIDs := make(map[int64]struct{})

	selection, scheduleDecision, err := h.gatewayService.SelectAccountWithSchedulerForCapability(
		requestCtx,
		apiKey.GroupID,
		"",
		sessionHash,
		model,
		failedAccountIDs,
		service.OpenAIUpstreamTransportAny,
		"",
		false,
	)
	if err != nil || selection == nil || selection.Account == nil {
		markOpsRoutingCapacityLimitedIfNoAvailable(c, err)
		reqLog.Warn("openai.videos.account_select_failed", zap.Error(err))
		h.handleStreamingAwareError(c, http.StatusServiceUnavailable, "api_error", "No available compatible accounts", streamStarted)
		return
	}
	reqLog.Debug("openai.videos.account_schedule_decision",
		zap.String("layer", scheduleDecision.Layer),
		zap.Bool("sticky_session_hit", scheduleDecision.StickySessionHit),
		zap.Int("candidate_count", scheduleDecision.CandidateCount),
		zap.Int("top_k", scheduleDecision.TopK),
		zap.Int64("latency_ms", scheduleDecision.LatencyMs),
		zap.Float64("load_skew", scheduleDecision.LoadSkew),
	)

	account := selection.Account
	setOpsSelectedAccount(c, account.ID, account.Platform)
	accountReleaseFunc, acquired := h.acquireResponsesAccountSlot(c, apiKey.GroupID, sessionHash, selection, false, &streamStarted, reqLog)
	if !acquired {
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
		return h.gatewayService.ForwardXAIVideosGeneration(requestCtx, c, account, body, channelMapping.MappedModel, forwardStart)
	}()
	forwardDurationMs := time.Since(forwardStart).Milliseconds()
	upstreamLatencyMs, _ := getContextInt64(c, service.OpsUpstreamLatencyMsKey)
	responseLatencyMs := forwardDurationMs
	if upstreamLatencyMs > 0 && forwardDurationMs > upstreamLatencyMs {
		responseLatencyMs = forwardDurationMs - upstreamLatencyMs
	}
	service.SetOpsLatencyMs(c, service.OpsResponseLatencyMsKey, responseLatencyMs)

	if err != nil {
		var imageUpstreamErr *service.OpenAIImagesUpstreamError
		if errors.As(err, &imageUpstreamErr) {
			h.gatewayService.ReportOpenAIAccountScheduleResult(account.ID, true, nil)
			reqLog.Warn("openai.videos.upstream_user_error",
				zap.Int64("account_id", account.ID),
				zap.Int("status_code", imageUpstreamErr.StatusCode),
				zap.String("error_type", imageUpstreamErr.ErrorType),
				zap.String("error_code", imageUpstreamErr.Code),
				zap.Error(err),
			)
			return
		}
		h.gatewayService.ReportOpenAIAccountScheduleResult(account.ID, false, nil)
		upstreamErrorAlreadyCommunicated := openAIForwardErrorAlreadyCommunicated(c, writerSizeBeforeForward, err)
		wroteFallback := false
		if !upstreamErrorAlreadyCommunicated {
			wroteFallback = h.ensureForwardErrorResponse(c, streamStarted)
		}
		reqLog.Warn("openai.videos.forward_failed",
			zap.Int64("account_id", account.ID),
			zap.Bool("fallback_error_response_written", wroteFallback),
			zap.Bool("upstream_error_response_already_written", upstreamErrorAlreadyCommunicated),
			zap.Error(err),
		)
		return
	}
	h.gatewayService.ReportOpenAIAccountScheduleResult(account.ID, true, nil)

	userAgent := c.GetHeader("User-Agent")
	clientIP := ip.GetClientIP(c)
	requestPayloadHash := service.HashUsageRequestPayload(body)
	inboundEndpoint := GetInboundEndpoint(c)
	upstreamEndpoint := GetUpstreamEndpoint(c, account.Platform)
	upstreamModel := ""
	if result != nil {
		upstreamModel = result.UpstreamModel
	}
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
			ChannelUsageFields: channelMapping.ToUsageFields(model, upstreamModel),
		}); err != nil {
			logger.L().With(
				zap.String("component", "handler.openai_gateway.videos"),
				zap.Int64("user_id", subject.UserID),
				zap.Int64("api_key_id", apiKey.ID),
				zap.Any("group_id", apiKey.GroupID),
				zap.String("model", model),
				zap.Int64("account_id", account.ID),
			).Error("openai.videos.record_usage_failed", zap.Error(err))
		}
	})
}
