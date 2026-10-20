package handler

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	pkghttputil "github.com/Wei-Shaw/sub2api/internal/pkg/httputil"
	middleware2 "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// handleSeedanceMedia is the Seedance-specific gateway lifecycle.  The
// account is selected from a seedance group, the task is bound to that account,
// and completion billing is claimed exactly once on a successful status poll.
func (h *OpenAIGatewayHandler) handleSeedanceMedia(c *gin.Context, endpoint service.GrokMediaEndpoint, requestID string) {
	streamStarted := false
	defer h.recoverResponsesPanic(c, &streamStarted)
	started := time.Now()
	apiKey, ok := middleware2.GetAPIKeyFromContext(c)
	if !ok || apiKey == nil {
		h.errorResponse(c, http.StatusUnauthorized, "authentication_error", "Invalid API key")
		return
	}
	subject, ok := middleware2.GetAuthSubjectFromContext(c)
	if !ok {
		h.errorResponse(c, http.StatusInternalServerError, "api_error", "User context not found")
		return
	}
	reqLog := requestLogger(c, "handler.openai_gateway.seedance_media",
		zap.Int64("user_id", subject.UserID), zap.Int64("api_key_id", apiKey.ID), zap.Any("group_id", apiKey.GroupID))
	if !h.ensureResponsesDependencies(c, reqLog) {
		return
	}

	var body []byte
	var err error
	if endpoint.RequiresRequestBody() {
		body, err = pkghttputil.ReadRequestBodyWithPrealloc(c.Request)
		if err != nil || len(body) == 0 {
			h.errorResponse(c, http.StatusBadRequest, "invalid_request_error", "Request body must be a non-empty JSON object")
			return
		}
	}
	requestInfo := service.SeedanceVideoRequestInfo{}
	if endpoint.IsGenerationRequest() {
		requestInfo, err = service.ParseSeedanceVideoRequest(body)
		if err != nil {
			h.errorResponse(c, http.StatusBadRequest, "invalid_request_error", err.Error())
			return
		}
		if !service.GroupAllowsImageGeneration(apiKey.Group) {
			h.errorResponse(c, http.StatusForbidden, "permission_error", "Video generation is not enabled for this group")
			return
		}
		if apiKey.Group == nil || apiKey.Group.GetVideoPriceForModel(requestInfo.Model, requestInfo.Resolution) == nil {
			h.errorResponse(c, http.StatusBadRequest, "configuration_error", "Seedance video price is not configured for this model and resolution")
			return
		}
		// Seedance requests carry user-controlled prompts/images just like the
		// existing image/video endpoints.  Audit before acquiring slots, billing,
		// account selection, or contacting Ark.
		moderationInfo := service.ParseGrokMediaRequest(c.GetHeader("Content-Type"), body)
		if moderationBody := moderationInfo.ModerationBody(); len(moderationBody) > 0 {
			decision := h.checkSecurityAudit(c, reqLog, apiKey, subject, service.ContentModerationProtocolOpenAIImages, requestInfo.Model, moderationBody)
			if decision != nil && !decision.AllowNextStage {
				h.openAISecurityAuditError(c, decision)
				return
			}
		}
	}
	if endpoint.IsVideoLookupRequest() && strings.TrimSpace(requestID) == "" {
		h.errorResponse(c, http.StatusBadRequest, "invalid_request_error", "request_id is required")
		return
	}
	setOpsRequestContext(c, requestInfo.Model, false)
	setOpsEndpointContext(c, "", int16(service.RequestTypeSync))
	service.SetOpsLatencyMs(c, service.OpsAuthLatencyMsKey, time.Since(started).Milliseconds())

	if h.errorPassthroughService != nil {
		service.BindErrorPassthroughService(c, h.errorPassthroughService)
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
			c.Header("Retry-After", retryAfterHeader(retryAfter))
		}
		h.errorResponse(c, status, code, message)
		return
	}

	requestCtx := service.WithOpenAIProfitControlSuppressed(c.Request.Context())
	failed := make(map[int64]struct{})
	sessionHash := h.gatewayService.GenerateExplicitSessionHash(c, body)
	boundAccountID := int64(0)
	if endpoint.IsVideoLookupRequest() {
		sessionHash = service.SeedanceVideoRequestSessionHash(requestID, subject.UserID, apiKey.ID)
		boundAccountID, err = h.gatewayService.ResolveSeedanceVideoRequestAccount(requestCtx, apiKey.GroupID, requestID, subject.UserID, apiKey.ID)
		if err != nil || boundAccountID <= 0 {
			h.errorResponse(c, http.StatusNotFound, "not_found_error", "Video request not found")
			return
		}
	}
	maxSwitches := h.maxAccountSwitches
	if maxSwitches <= 0 {
		maxSwitches = 3
	}
	for switchCount := 0; ; switchCount++ {
		var selection *service.AccountSelectionResult
		var decision service.OpenAIAccountScheduleDecision
		if boundAccountID > 0 {
			selection, decision, err = h.gatewayService.SelectSeedanceVideoRequestAccount(requestCtx, apiKey.GroupID, sessionHash, boundAccountID, requestInfo.Model)
		} else {
			selection, decision, err = h.gatewayService.SelectAccountWithSchedulerForCapability(
				requestCtx, apiKey.GroupID, "", sessionHash, requestInfo.Model, failed,
				service.OpenAIUpstreamTransportHTTPSSE, service.OpenAIEndpointCapabilityGrokMediaGeneration,
				false, false, false, service.PlatformSeedance,
			)
		}
		if err != nil || selection == nil || selection.Account == nil {
			if endpoint.IsVideoLookupRequest() {
				h.errorResponse(c, http.StatusNotFound, "not_found_error", "Video request not found")
			} else {
				h.errorResponse(c, http.StatusServiceUnavailable, "seedance_no_available_account", "No eligible Seedance account")
			}
			return
		}
		account := selection.Account
		setOpsSelectedAccount(c, account.ID, account.Platform)
		var accountRelease func()
		accountRelease, slotResult := h.acquireResponsesAccountSlot(c, apiKey.GroupID, "", selection, false, &streamStarted, reqLog)
		if slotResult != openAISlotAcquireOK {
			return
		}
		result, forwardErr := func() (*service.OpenAIForwardResult, error) {
			defer func() {
				if accountRelease != nil {
					accountRelease()
				}
			}()
			return h.gatewayService.ForwardGrokMedia(requestCtx, c, account, endpoint, requestID, body, c.GetHeader("Content-Type"), service.PlatformSeedance)
		}()
		if forwardErr != nil {
			var failover *service.UpstreamFailoverError
			if errors.As(forwardErr, &failover) && endpoint.IsGenerationRequest() && switchCount < maxSwitches {
				failed[account.ID] = struct{}{}
				continue
			}
			if !service.IsResponseCommitted(c) {
				h.errorResponse(c, http.StatusBadGateway, "upstream_error", "Seedance upstream request failed")
			}
			return
		}
		h.gatewayService.ReportOpenAIAccountScheduleResult(account, requestInfo.Model, true, nil)
		if endpoint == service.GrokMediaEndpointVideosGenerations && strings.TrimSpace(result.ResponseID) != "" {
			_ = h.gatewayService.BindSeedanceVideoRequestAccount(requestCtx, apiKey.GroupID, result.ResponseID, subject.UserID, apiKey.ID, account.ID)
			pending := service.GrokVideoPendingBilling{
				Model: requestInfo.Model, BillingModel: requestInfo.Model, UpstreamModel: result.UpstreamModel,
				VideoResolution: requestInfo.Resolution, VideoDurationSeconds: requestInfo.DurationSeconds,
				OriginalModel: requestInfo.Model, CreatedAt: service.GrokVideoPendingCreatedAtNow(),
			}
			if err := h.gatewayService.StoreSeedanceVideoPendingBilling(requestCtx, result.ResponseID, subject.UserID, apiKey.ID, pending); err != nil {
				reqLog.Warn("seedance_media.store_pending_billing_failed", zap.Error(err))
			}
		} else if endpoint.IsVideoLookupRequest() {
			if billed := prepareSeedanceVideoCompletionBilling(requestCtx, h, reqLog, apiKey, subject, requestID, result); billed != nil {
				recordSeedanceMediaUsage(c, h, reqLog, apiKey, subject, subscription, account, billed, billed.Model, body, requestID)
			}
		}
		_ = decision
		return
	}
}

func retryAfterHeader(seconds int) string { return strconv.Itoa(seconds) }

func prepareSeedanceVideoCompletionBilling(ctx context.Context, h *OpenAIGatewayHandler, reqLog *zap.Logger, apiKey *service.APIKey, subject middleware2.AuthSubject, taskID string, result *service.OpenAIForwardResult) *service.OpenAIForwardResult {
	if result == nil || result.VideoCount <= 0 || strings.TrimSpace(taskID) == "" {
		return nil
	}
	pending, err := h.gatewayService.LoadSeedanceVideoPendingBilling(ctx, taskID, subject.UserID, apiKey.ID)
	if err != nil {
		reqLog.Warn("seedance_media.load_pending_billing_failed", zap.Error(err))
	}
	if pending == nil && result.VideoDurationSeconds <= 0 {
		return nil
	}
	claimed, err := h.gatewayService.ClaimSeedanceVideoBilling(ctx, taskID, subject.UserID, apiKey.ID)
	if err != nil || !claimed {
		return nil
	}
	merged := *result
	if pending != nil {
		if merged.Model == "" {
			merged.Model = pending.BillingModel
		}
		if merged.BillingModel == "" {
			merged.BillingModel = pending.BillingModel
		}
		if merged.UpstreamModel == "" {
			merged.UpstreamModel = pending.UpstreamModel
		}
		if merged.VideoResolution == "" {
			merged.VideoResolution = pending.VideoResolution
		}
		if merged.VideoDurationSeconds <= 0 {
			merged.VideoDurationSeconds = pending.VideoDurationSeconds
		}
	}
	if merged.Model == "" {
		merged.Model = "seedance"
	}
	if merged.BillingModel == "" {
		merged.BillingModel = merged.Model
	}
	merged.RequestID = service.StableSeedanceVideoBillingRequestID(taskID)
	merged.ResponseID = taskID
	merged.VideoCount = 1
	merged.ImageCount = 0
	merged.VideoResolution = service.NormalizeVideoBillingResolutionOrDefault(merged.VideoResolution)
	merged.VideoDurationSeconds = service.NormalizeVideoBillingDurationSecondsOrDefault(merged.VideoDurationSeconds)
	return &merged
}
