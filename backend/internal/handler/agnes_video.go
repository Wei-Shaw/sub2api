package handler

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"strings"

	pkghttputil "github.com/Wei-Shaw/sub2api/internal/pkg/httputil"
	middleware2 "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
	"go.uber.org/zap"
)

// AgnesVideoGeneration handles POST /v1/videos for Agnes video models.
func (h *OpenAIGatewayHandler) AgnesVideoGeneration(c *gin.Context) {
	h.handleAgnesVideo(c, "")
}

// AgnesVideoStatus handles GET /v1/videos/{request_id}.  Agnes uses the
// provider-root /agnesapi?video_id=... endpoint for current jobs; the service
// layer translates this compatibility route before opening the upstream request.
func (h *OpenAIGatewayHandler) AgnesVideoStatus(c *gin.Context) {
	h.handleAgnesVideo(c, c.Param("request_id"))
}

// AgnesVideoRootStatus handles the provider-root compatibility form used by
// clients that are configured with an Agnes-shaped base URL.  A Sub2API
// channel still needs to authenticate and schedule the request before sending
// it to Agnes, so this route is deliberately protected by the normal gateway
// API-key middleware at registration time.
func (h *OpenAIGatewayHandler) AgnesVideoRootStatus(c *gin.Context) {
	videoID := strings.TrimSpace(c.Query("video_id"))
	if videoID == "" {
		h.errorResponse(c, http.StatusBadRequest, "invalid_request_error", "video_id is required")
		return
	}
	h.handleAgnesVideo(c, videoID)
}

func (h *OpenAIGatewayHandler) handleAgnesVideo(c *gin.Context, videoID string) {
	streamStarted := false
	defer h.recoverResponsesPanic(c, &streamStarted)

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
		"handler.openai_gateway.agnes_video",
		zap.Int64("user_id", subject.UserID),
		zap.Int64("api_key_id", apiKey.ID),
		zap.Any("group_id", apiKey.GroupID),
	)
	if !h.ensureResponsesDependencies(c, reqLog) {
		return
	}

	createRequest := strings.TrimSpace(videoID) == ""
	var body []byte
	var err error
	if createRequest {
		body, err = pkghttputil.ReadRequestBodyWithPrealloc(c.Request)
		if err != nil {
			h.errorResponse(c, http.StatusBadRequest, "invalid_request_error", "Failed to read request body")
			return
		}
		if len(body) == 0 || !gjson.ValidBytes(body) {
			h.errorResponse(c, http.StatusBadRequest, "invalid_request_error", "Request body must be valid JSON")
			return
		}
	}
	requestInfo := service.ParseGrokMediaRequest(c.GetHeader("Content-Type"), body)

	requestModel := strings.TrimSpace(c.Query("model"))
	if requestModel == "" {
		requestModel = strings.TrimSpace(c.Query("model_name"))
	}
	if createRequest {
		requestModel = strings.TrimSpace(requestInfo.Model)
	}
	if requestModel == "" {
		requestModel = "agnes-video-v2.0"
	}
	if !strings.HasPrefix(strings.ToLower(strings.TrimSpace(requestModel)), "agnes-video") {
		h.errorResponse(c, http.StatusNotFound, "not_found_error", "Agnes video model is required")
		return
	}
	if !service.GroupAllowsImageGeneration(apiKey.Group) {
		h.errorResponse(c, http.StatusForbidden, "permission_error", service.ImageGenerationPermissionMessage())
		return
	}
	if createRequest {
		if decision := h.checkSecurityAudit(c, reqLog, apiKey, subject, service.ContentModerationProtocolOpenAIImages, requestModel, requestInfo.ModerationBody()); decision != nil && !decision.AllowNextStage {
			h.openAISecurityAuditError(c, decision)
			return
		}
		imageReleaseFunc, acquired := h.acquireImageGenerationSlot(c, streamStarted)
		if !acquired {
			return
		}
		if imageReleaseFunc != nil {
			defer imageReleaseFunc()
		}
	}
	setOpsRequestContext(c, requestModel, false)
	setOpsEndpointContext(c, "", int16(service.RequestTypeSync))

	if h.billingCacheService != nil {
		subscription, _ := middleware2.GetSubscriptionFromContext(c)
		if err := h.billingCacheService.CheckBillingEligibility(c.Request.Context(), apiKey.User, apiKey, apiKey.Group, subscription, service.QuotaPlatform(c.Request.Context(), apiKey)); err != nil {
			status, code, message, retryAfter := billingErrorDetails(err)
			if retryAfter > 0 {
				c.Header("Retry-After", formatRetryAfter(retryAfter))
			}
			h.errorResponse(c, status, code, message)
			return
		}
	}
	userReleaseFunc, acquired := h.acquireResponsesUserSlot(c, subject.UserID, subject.Concurrency, false, &streamStarted, reqLog)
	if !acquired {
		return
	}
	if userReleaseFunc != nil {
		defer userReleaseFunc()
	}

	channelMapping, _ := h.gatewayService.ResolveChannelMappingAndRestrict(c.Request.Context(), apiKey.GroupID, requestModel)
	sessionSeed := body
	if !createRequest {
		sessionSeed = []byte(videoID)
	}
	sessionHash := h.gatewayService.GenerateExplicitSessionHash(c, sessionSeed)
	selection, _, err := h.gatewayService.SelectAccountWithSchedulerForCapability(
		c.Request.Context(),
		apiKey.GroupID,
		"",
		sessionHash,
		requestModel,
		nil,
		service.OpenAIUpstreamTransportHTTPSSE,
		"",
		false,
		false,
		false,
		service.PlatformOpenAI,
	)
	if err != nil || selection == nil || selection.Account == nil {
		if errors.Is(err, service.ErrNoAvailableAccounts) || selection == nil {
			h.errorResponse(c, http.StatusServiceUnavailable, "no_available_account", "No available Agnes video account")
		} else {
			h.errorResponse(c, http.StatusBadGateway, "upstream_error", "Unable to select Agnes video account")
		}
		return
	}

	account := selection.Account
	setOpsSelectedAccount(c, account.ID, account.Platform)
	accountReleaseFunc, slotResult := h.acquireResponsesAccountSlot(c, apiKey.GroupID, sessionHash, selection, false, &streamStarted, reqLog)
	if slotResult != openAISlotAcquireOK {
		return
	}
	account = selection.Account
	if accountReleaseFunc != nil {
		defer accountReleaseFunc()
	}

	result, forwardErr := h.gatewayService.ForwardAgnesVideo(
		c.Request.Context(),
		c,
		account,
		requestModel,
		videoID,
		body,
		c.GetHeader("Content-Type"),
		channelMapping.MappedModel,
	)
	if forwardErr != nil {
		h.gatewayService.ReportOpenAIAccountScheduleResult(account.ID, account.GetMappedModel(requestModel), false, nil)
		if service.IsResponseCommitted(c) {
			reqLog.Warn("agnes_video.upstream_response_error", zap.Int64("account_id", account.ID), zap.Error(forwardErr))
			return
		}
		h.errorResponse(c, http.StatusBadGateway, "upstream_error", "Agnes video upstream request failed")
		return
	}
	h.gatewayService.ReportOpenAIAccountScheduleResult(account.ID, account.GetMappedModel(requestModel), true, nil)

	// Status polling is intentionally not billed as a second generation.  The
	// create request still records a normal OpenAI usage row so the existing
	// channel/group accounting remains visible.
	if createRequest && result != nil {
		subscription, _ := middleware2.GetSubscriptionFromContext(c)
		requestBodyHash := service.HashUsageRequestPayload(body)
		h.submitOpenAIUsageRecordTask(c.Request.Context(), result, func(ctx context.Context) {
			if err := h.gatewayService.RecordUsage(ctx, &service.OpenAIRecordUsageInput{
				Result:             result,
				APIKey:             apiKey,
				User:               apiKey.User,
				Account:            account,
				Subscription:       subscription,
				InboundEndpoint:    GetInboundEndpoint(c),
				UpstreamEndpoint:   GetUpstreamEndpoint(c, account.Platform),
				UserAgent:          c.GetHeader("User-Agent"),
				IPAddress:          c.ClientIP(),
				RequestPayloadHash: requestBodyHash,
				APIKeyService:      h.apiKeyService,
				QuotaPlatform:      service.QuotaPlatform(ctx, apiKey),
				ChannelUsageFields: channelMapping.ToUsageFields(requestModel, result.UpstreamModel),
			}); err != nil {
				reqLog.Warn("agnes_video.record_usage_failed", zap.Int64("account_id", account.ID), zap.Error(err))
			}
		})
	}
}

func formatRetryAfter(seconds int) string {
	if seconds <= 0 {
		return ""
	}
	return strconv.Itoa(seconds)
}
