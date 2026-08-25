package handler

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

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
		model = "sora-2"
	}
	if isCreate && !service.GroupAllowsImageGeneration(apiKey.Group) {
		h.errorResponse(c, http.StatusForbidden, "permission_error", service.ImageGenerationPermissionMessage())
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

	selection, _, err := h.gatewayService.SelectAccountWithSchedulerForCapability(
		ctx, apiKey.GroupID, "", sessionHash, model, nil,
		service.OpenAIUpstreamTransportHTTPSSE, "", false, false, false, service.PlatformOpenAI,
	)
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

	result, err := h.gatewayService.ForwardOpenAIVideo(ctx, c, account, endpoint, requestID, body, c.GetHeader("Content-Type"))
	if err != nil {
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
		pending := service.GrokVideoPendingBilling{Model: model, BillingModel: firstNonEmptyString(result.BillingModel, model), UpstreamModel: result.UpstreamModel, VideoResolution: result.VideoResolution, VideoDurationSeconds: result.VideoDurationSeconds, OriginalModel: clientRequestedModel(c, model), CreatedAt: service.GrokVideoPendingCreatedAtNow()}
		if err := h.gatewayService.StoreVideoTaskPendingBilling(ctx, service.VideoTaskPlatformOpenAI, result.ResponseID, subject.UserID, apiKey.ID, pending); err != nil {
			reqLog.Warn("openai_video.store_pending_billing_failed", zap.Error(err))
		}
	}

	if !isCreate && (endpoint == service.OpenAIVideoStatus || endpoint == service.OpenAIVideoContent) && result != nil {
		// A successful content download is also proof that the asynchronous task
		// completed; content responses are binary and carry no status JSON.
		if endpoint == service.OpenAIVideoContent {
			result.VideoCount = 1
		}
		if billResult := prepareOpenAIVideoCompletionBilling(ctx, h, reqLog, apiKey, subject.UserID, apiKey.ID, requestID, result); billResult != nil {
			subscription, _ := middleware2.GetSubscriptionFromContext(c)
			recordGrokMediaUsage(c, h, reqLog, apiKey, subject, subscription, account, billResult, billResult.Model, nil, requestID)
		}
	}
}

func prepareOpenAIVideoCompletionBilling(ctx context.Context, h *OpenAIGatewayHandler, reqLog *zap.Logger, apiKey *service.APIKey, userID, apiKeyID int64, requestID string, result *service.OpenAIForwardResult) *service.OpenAIForwardResult {
	if result == nil || result.VideoCount <= 0 || strings.TrimSpace(requestID) == "" {
		return nil
	}
	pending, err := h.gatewayService.LoadVideoTaskPendingBilling(ctx, service.VideoTaskPlatformOpenAI, requestID, userID, apiKeyID)
	if err != nil || pending == nil {
		reqLog.Warn("openai_video.pending_billing_missing", zap.Error(err))
		return nil
	}
	claimed, err := h.gatewayService.ClaimVideoTaskBilling(ctx, service.VideoTaskPlatformOpenAI, requestID, userID, apiKeyID)
	if err != nil || !claimed {
		return nil
	}
	merged := *result
	merged.Model = firstNonEmptyString(merged.Model, pending.BillingModel, pending.Model)
	merged.BillingModel = firstNonEmptyString(merged.BillingModel, pending.BillingModel, pending.Model, merged.Model)
	merged.UpstreamModel = firstNonEmptyString(merged.UpstreamModel, pending.UpstreamModel)
	merged.VideoResolution = firstNonEmptyString(merged.VideoResolution, pending.VideoResolution)
	if merged.VideoDurationSeconds <= 0 {
		merged.VideoDurationSeconds = pending.VideoDurationSeconds
	}
	merged.VideoResolution = service.NormalizeVideoBillingResolutionOrDefault(merged.VideoResolution)
	merged.VideoDurationSeconds = service.NormalizeVideoBillingDurationSecondsOrDefault(merged.VideoDurationSeconds)
	merged.ResponseID = firstNonEmptyString(merged.ResponseID, requestID)
	merged.RequestID = service.StableVideoTaskBillingRequestID(service.VideoTaskPlatformOpenAI, requestID)
	if e2e := service.GrokVideoE2EDuration(pending.CreatedAt, time.Now()); e2e > 0 {
		merged.Duration = e2e
	}
	return &merged
}
