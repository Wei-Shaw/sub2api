package handler

import (
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

// OpenAIVideo handles the asynchronous OpenAI Videos API. The task binding is
// deliberately resolved before scheduling so status/content requests stay on
// the account that created the upstream task.
func (h *OpenAIGatewayHandler) OpenAIVideo(c *gin.Context) {
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

	isCreate := c.Request.Method == http.MethodPost
	isContent := strings.HasSuffix(strings.TrimSuffix(c.Request.URL.Path, "/"), "/content")
	endpoint := service.OpenAIVideoStatus
	requestID := strings.TrimSpace(c.Param("request_id"))
	if isCreate {
		endpoint = service.OpenAIVideoCreate
	} else if isContent {
		endpoint = service.OpenAIVideoContent
	}
	if !isCreate && requestID == "" {
		h.errorResponse(c, http.StatusBadRequest, "invalid_request_error", "video_id is required")
		return
	}

	reqLog := requestLogger(c, "handler.openai_gateway.openai_video",
		zap.Int64("user_id", subject.UserID), zap.Int64("api_key_id", apiKey.ID),
		zap.Any("group_id", apiKey.GroupID), zap.String("endpoint", string(endpoint)),
		zap.String("video_id", requestID))
	if !h.ensureResponsesDependencies(c, reqLog) {
		return
	}

	var body []byte
	var err error
	if isCreate {
		body, err = pkghttputil.ReadRequestBodyWithPrealloc(c.Request)
		if err != nil || len(body) == 0 {
			h.errorResponse(c, http.StatusBadRequest, "invalid_request_error", "Request body is required")
			return
		}
	}

	model := strings.TrimSpace(gjson.GetBytes(body, "model").String())
	if isCreate && model == "" {
		h.errorResponse(c, http.StatusBadRequest, "invalid_request_error", "The model field is required.")
		return
	}
	if isCreate && !service.GroupAllowsVideoGeneration(apiKey.Group) {
		h.errorResponse(c, http.StatusForbidden, "permission_error", "Video generation is not enabled for this group")
		return
	}

	ctx := service.WithOpenAIProfitControlSuppressed(c.Request.Context())
	sessionHash := h.gatewayService.GenerateExplicitSessionHash(c, body)
	if !isCreate {
		bound, resolveErr := h.gatewayService.ResolveVideoTaskAccount(ctx, apiKey.GroupID, service.VideoTaskPlatformOpenAI, requestID, subject.UserID, apiKey.ID)
		if resolveErr != nil || bound <= 0 {
			h.errorResponse(c, http.StatusNotFound, "not_found_error", "Video request not found")
			return
		}
		sessionHash = service.VideoTaskSessionHash(service.VideoTaskPlatformOpenAI, requestID, subject.UserID, apiKey.ID)
	}

	selection, _, err := h.gatewayService.SelectAccountWithSchedulerForVideo(ctx, apiKey.GroupID, sessionHash, model, nil)
	if err != nil || selection == nil || selection.Account == nil {
		if errors.Is(err, service.ErrNoAvailableAccounts) {
			markOpsRoutingCapacityLimited(c)
		}
		h.errorResponse(c, http.StatusServiceUnavailable, "api_error", "No available OpenAI video accounts")
		return
	}
	account := selection.Account
	release, slotStatus := h.acquireResponsesAccountSlot(c, apiKey.GroupID, sessionHash, selection, false, &streamStarted, reqLog)
	if slotStatus != openAISlotAcquireOK {
		return
	}
	if release != nil {
		defer release()
	}
	setOpsSelectedAccount(c, account.ID, account.Platform)
	var videoHold *service.GrokVideoPendingBilling
	if isCreate {
		resolution := openAIVideoRequestResolution(body)
		duration := openAIVideoRequestDuration(body)
		cost, pricingErr := h.gatewayService.CalculateVideoTaskCost(ctx, apiKey, account, model, resolution, duration)
		if pricingErr != nil {
			h.errorResponse(c, http.StatusBadRequest, "billing_error", "Unable to determine video task price")
			return
		}
		holdID := service.NewVideoTaskHoldID()
		if reserveErr := h.gatewayService.ReserveVideoTaskBalance(ctx, subject.UserID, apiKey.ID, holdID, cost, service.HashUsageRequestPayload(body)); reserveErr != nil {
			status, code, message, retryAfter := billingErrorDetails(reserveErr)
			if retryAfter > 0 {
				c.Header("Retry-After", strconv.Itoa(retryAfter))
			}
			h.errorResponse(c, status, code, message)
			return
		}
		videoHold = &service.GrokVideoPendingBilling{HoldID: holdID, HoldAmount: cost}
	}

	result, err := h.gatewayService.ForwardOpenAIVideo(ctx, c, account, endpoint, requestID, body, c.GetHeader("Content-Type"))
	if err != nil {
		if videoHold != nil {
			_ = h.gatewayService.ReleaseVideoTaskBalance(ctx, videoHold, subject.UserID, apiKey.ID)
		}
		var failoverErr *service.UpstreamFailoverError
		if errors.As(err, &failoverErr) {
			h.handleFailoverExhausted(c, failoverErr, false)
			return
		}
		if !service.IsResponseCommitted(c) {
			h.errorResponse(c, http.StatusBadGateway, "upstream_error", "Upstream request failed")
		}
		return
	}

	if isCreate && result != nil && strings.TrimSpace(result.ResponseID) != "" {
		if err := h.gatewayService.BindVideoTaskAccount(ctx, apiKey.GroupID, service.VideoTaskPlatformOpenAI, result.ResponseID, subject.UserID, apiKey.ID, account.ID); err != nil {
			reqLog.Warn("openai_video.bind_task_account_failed", zap.Error(err))
		}
		pending := service.GrokVideoPendingBilling{UserID: subject.UserID, APIKeyID: apiKey.ID, AccountID: account.ID, Platform: service.VideoTaskPlatformOpenAI, Model: model, BillingModel: firstNonEmptyString(result.BillingModel, model), UpstreamModel: result.UpstreamModel, VideoResolution: firstNonEmptyString(result.VideoResolution, openAIVideoRequestResolution(body)), VideoDurationSeconds: result.VideoDurationSeconds, OriginalModel: clientRequestedModel(c, model), CreatedAt: service.GrokVideoPendingCreatedAtNow(), UpstreamCreatedAtUnix: result.VideoCreatedAtUnix}
		if pending.VideoDurationSeconds <= 0 {
			pending.VideoDurationSeconds = openAIVideoRequestDuration(body)
		}
		if videoHold != nil {
			pending.HoldID, pending.HoldAmount = videoHold.HoldID, videoHold.HoldAmount
		}
		if err := h.gatewayService.StoreVideoTaskPendingBilling(ctx, service.VideoTaskPlatformOpenAI, result.ResponseID, subject.UserID, apiKey.ID, pending); err != nil {
			reqLog.Warn("openai_video.store_pending_billing_failed", zap.Error(err))
		}
	}

	// Status/content requests are intentionally read-only. The background
	// reconciler is the sole owner of asynchronous video settlement.
}

func openAIVideoRequestResolution(body []byte) string {
	return service.NormalizeVideoBillingResolutionOrDefault(gjson.GetBytes(body, "size").String())
}

func openAIVideoRequestDuration(body []byte) int {
	seconds := strings.TrimSpace(gjson.GetBytes(body, "seconds").String())
	value, _ := strconv.Atoi(seconds)
	return service.NormalizeVideoBillingDurationSecondsOrDefault(value)
}
