package handler

import (
	"context"
	"errors"
	"net/http"
	"os"
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

func (h *OpenAIGatewayHandler) ImagesAsync(c *gin.Context) {
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
	if h.imageTaskService == nil {
		h.errorResponse(c, http.StatusServiceUnavailable, "api_error", "Async image generation is unavailable")
		return
	}
	reqLog := requestLogger(c, "handler.openai_gateway.images_async",
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
	if isMultipartImagesContentType(c.GetHeader("Content-Type")) {
		setOpsRequestContext(c, "", false, nil)
	} else {
		setOpsRequestContext(c, "", false, body)
	}

	parsed, err := h.gatewayService.ParseOpenAIImagesRequest(c, body)
	if err != nil {
		h.errorResponse(c, http.StatusBadRequest, "invalid_request_error", err.Error())
		return
	}
	parsed.Stream = false
	if parsed.Multipart {
		setOpsRequestContext(c, parsed.Model, false, nil)
	} else {
		setOpsRequestContext(c, parsed.Model, false, body)
	}
	setOpsEndpointContext(c, "", int16(service.RequestTypeFromLegacy(false, false)))

	subscription, _ := middleware2.GetSubscriptionFromContext(c)
	if err := h.billingCacheService.CheckBillingEligibility(c.Request.Context(), apiKey.User, apiKey, apiKey.Group, subscription); err != nil {
		status, code, message, retryAfter := billingErrorDetails(err)
		if retryAfter > 0 {
			c.Header("Retry-After", strconv.Itoa(retryAfter))
		}
		h.errorResponse(c, status, code, message)
		return
	}

	task, err := h.imageTaskService.CreatePending(c.Request.Context(), subject.UserID, apiKey.ID, parsed.Endpoint, parsed.Model, parsed.Prompt)
	if err != nil {
		reqLog.Error("openai.images_async.create_task_failed", zap.Error(err))
		h.errorResponse(c, http.StatusInternalServerError, "api_error", "Failed to create image task")
		return
	}

	channelMapping, _ := h.gatewayService.ResolveChannelMappingAndRestrict(c.Request.Context(), apiKey.GroupID, parsed.Model)
	userAgent := c.GetHeader("User-Agent")
	clientIP := ip.GetClientIP(c)
	requestPayloadHash := service.HashUsageRequestPayload(body)
	if parsed.Multipart {
		requestPayloadHash = service.HashUsageRequestPayload([]byte(parsed.StickySessionSeed()))
	}
	sessionHash := h.gatewayService.GenerateExplicitSessionHash(c, body)
	service.SetOpsLatencyMs(c, service.OpsAuthLatencyMsKey, time.Since(requestStart).Milliseconds())

	go h.runImageAsyncTask(task.TaskID, apiKey, subject.UserID, subscription, body, parsed, channelMapping, userAgent, clientIP, requestPayloadHash, sessionHash)

	c.JSON(http.StatusAccepted, gin.H{
		"task_id":    task.TaskID,
		"status":     task.Status,
		"expires_at": task.ExpiresAt.Format(time.RFC3339),
	})
}

func (h *OpenAIGatewayHandler) ImageAsyncTaskStatus(c *gin.Context) {
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
	task, err := h.imageTaskService.GetForAPIKey(c.Request.Context(), c.Param("task_id"), subject.UserID, apiKey.ID)
	if err != nil {
		if errors.Is(err, service.ErrImageTaskNotFound) {
			h.errorResponse(c, http.StatusNotFound, "not_found_error", "Image task not found")
			return
		}
		h.errorResponse(c, http.StatusInternalServerError, "api_error", "Failed to load image task")
		return
	}
	c.JSON(http.StatusOK, imageTaskResponse(c, task))
}

func (h *OpenAIGatewayHandler) ImageAsyncTaskDownload(c *gin.Context) {
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
	task, err := h.imageTaskService.GetForAPIKey(c.Request.Context(), c.Param("task_id"), subject.UserID, apiKey.ID)
	if err != nil {
		if errors.Is(err, service.ErrImageTaskNotFound) {
			h.errorResponse(c, http.StatusNotFound, "not_found_error", "Image task not found")
			return
		}
		h.errorResponse(c, http.StatusInternalServerError, "api_error", "Failed to load image task")
		return
	}
	if task.Status != service.ImageTaskStatusSucceeded || strings.TrimSpace(task.FilePath) == "" {
		h.errorResponse(c, http.StatusConflict, "invalid_request_error", "Image task is not ready")
		return
	}
	if _, err := os.Stat(task.FilePath); err != nil {
		h.errorResponse(c, http.StatusGone, "not_found_error", "Image file has expired")
		return
	}
	c.Header("Content-Type", task.MimeType)
	c.Header("Content-Disposition", `attachment; filename="`+task.TaskID+imageDownloadExtension(task.MimeType)+`"`)
	c.File(task.FilePath)
}

func (h *OpenAIGatewayHandler) runImageAsyncTask(
	taskID string,
	apiKey *service.APIKey,
	userID int64,
	subscription *service.UserSubscription,
	body []byte,
	parsed *service.OpenAIImagesRequest,
	channelMapping service.ChannelMappingResult,
	userAgent string,
	clientIP string,
	requestPayloadHash string,
	sessionHash string,
) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
	defer cancel()
	if err := h.imageTaskService.MarkRunning(ctx, taskID); err != nil {
		logger.L().Warn("openai.images_async.mark_running_failed", zap.String("task_id", taskID), zap.Error(err))
	}

	failedAccountIDs := make(map[int64]struct{})
	var lastErr error
	for switchCount := 0; switchCount <= h.maxAccountSwitches; switchCount++ {
		selection, _, err := h.gatewayService.SelectAccountWithSchedulerForImages(ctx, apiKey.GroupID, sessionHash, parsed.Model, failedAccountIDs, parsed.RequiredCapability)
		if err != nil || selection == nil || selection.Account == nil {
			lastErr = imageAsyncFinalError(err, lastErr)
			break
		}
		account := selection.Account
		release := selection.ReleaseFunc
		result, assets, err := h.gatewayService.ForwardImagesBuffered(ctx, account, body, parsed, channelMapping.MappedModel)
		if release != nil {
			release()
		}
		if err != nil {
			var failoverErr *service.UpstreamFailoverError
			if errors.As(err, &failoverErr) {
				failedAccountIDs[account.ID] = struct{}{}
				lastErr = imageAsyncTaskError(failoverErr)
				continue
			}
			lastErr = err
			break
		}
		if len(assets) == 0 {
			lastErr = errors.New("no images returned")
			break
		}
		if _, err := h.imageTaskService.SaveResult(ctx, taskID, assets[0].Data, assets[0].MimeType); err != nil {
			lastErr = err
			break
		}
		if result != nil {
			_ = h.gatewayService.RecordUsage(ctx, &service.OpenAIRecordUsageInput{
				Result:             result,
				APIKey:             apiKey,
				User:               apiKey.User,
				Account:            account,
				Subscription:       subscription,
				InboundEndpoint:    parsed.Endpoint,
				UpstreamEndpoint:   parsed.Endpoint,
				UserAgent:          userAgent,
				IPAddress:          clientIP,
				RequestPayloadHash: requestPayloadHash,
				APIKeyService:      h.apiKeyService,
				ChannelUsageFields: channelMapping.ToUsageFields(parsed.Model, result.UpstreamModel),
			})
		}
		return
	}
	_ = h.imageTaskService.MarkFailed(context.Background(), taskID, lastErr)
	if lastErr != nil {
		logger.L().Warn("openai.images_async.task_failed", zap.String("task_id", taskID), zap.Int64("user_id", userID), zap.Error(lastErr))
	}
}

func imageAsyncTaskError(err error) error {
	if err == nil {
		return nil
	}
	var failoverErr *service.UpstreamFailoverError
	if errors.As(err, &failoverErr) && failoverErr != nil {
		msg := strings.TrimSpace(service.ExtractUpstreamErrorMessage(failoverErr.ResponseBody))
		if msg != "" {
			if imageAsyncIsGenericUpstreamMessage(msg) {
				if policyMsg := imageAsyncPolicyViolationMessage(failoverErr.ResponseBody); policyMsg != "" {
					return errors.New(policyMsg)
				}
			}
			return errors.New(msg)
		}
		if policyMsg := imageAsyncPolicyViolationMessage(failoverErr.ResponseBody); policyMsg != "" {
			return errors.New(policyMsg)
		}
	}
	return err
}

func imageAsyncIsGenericUpstreamMessage(message string) bool {
	switch strings.ToLower(strings.TrimSpace(message)) {
	case "upstream request failed", "upstream error":
		return true
	default:
		return false
	}
}

func imageAsyncPolicyViolationMessage(body []byte) string {
	lower := strings.ToLower(string(body))
	if strings.Contains(lower, "content_policy") ||
		strings.Contains(lower, "policy_violation") ||
		strings.Contains(lower, "safety system") ||
		strings.Contains(lower, "violates our content policy") {
		return "Prompt violates content policy"
	}
	return ""
}

func imageAsyncFinalError(current error, previous error) error {
	if previous != nil && imageAsyncIsNoAvailableAccountError(current) {
		return previous
	}
	if current != nil {
		return current
	}
	return previous
}

func imageAsyncIsNoAvailableAccountError(err error) bool {
	msg := strings.ToLower(strings.TrimSpace(errorString(err)))
	return strings.Contains(msg, "no available") && strings.Contains(msg, "account")
}

func errorString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

func imageTaskResponse(c *gin.Context, task *service.ImageTask) gin.H {
	out := gin.H{
		"task_id":    task.TaskID,
		"status":     task.Status,
		"expires_at": task.ExpiresAt.Format(time.RFC3339),
	}
	if task.Status == service.ImageTaskStatusSucceeded {
		out["download_url"] = "/api/v1/images/async/tasks/" + task.TaskID + "/download"
		out["mime_type"] = task.MimeType
		out["byte_size"] = task.ByteSize
	}
	if task.Status == service.ImageTaskStatusFailed {
		out["error_message"] = task.ErrorMessage
	}
	return out
}

func imageDownloadExtension(mimeType string) string {
	switch strings.ToLower(strings.TrimSpace(mimeType)) {
	case "image/jpeg":
		return ".jpg"
	case "image/webp":
		return ".webp"
	default:
		return ".png"
	}
}
