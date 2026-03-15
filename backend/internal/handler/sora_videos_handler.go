package handler

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"unicode/utf8"

	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	middleware2 "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/gin-gonic/gin"
)

// SoraVideosHandler handles Sora video/image async task API.
type SoraVideosHandler struct {
	taskService        *service.SoraTaskService
	gatewayService     *service.GatewayService
	objectStorage      service.SoraObjectStorage
	mediaStorage       *service.SoraMediaStorage
	soraGatewayService *service.SoraGatewayService
}

func NewSoraVideosHandler(
	taskService *service.SoraTaskService,
	gatewayService *service.GatewayService,
	objectStorage service.SoraObjectStorage,
	mediaStorage *service.SoraMediaStorage,
	soraGatewayService *service.SoraGatewayService,
) *SoraVideosHandler {
	if taskService == nil {
		return nil
	}
	return &SoraVideosHandler{
		taskService:        taskService,
		gatewayService:     gatewayService,
		objectStorage:      objectStorage,
		mediaStorage:       mediaStorage,
		soraGatewayService: soraGatewayService,
	}
}

func (h *SoraVideosHandler) CreateVideo(c *gin.Context) {
	apiKey, account, release, ok := h.selectAccount(c, "")
	if !ok {
		return
	}
	defer release()

	body, err := readBody(c)
	if err != nil {
		return
	}

	var req service.CreateVideoRequest
	if err := json.Unmarshal(body, &req); err != nil {
		soraErrorResponse(c, http.StatusBadRequest, "invalid_request_error", "Invalid request body")
		return
	}
	if req.Model == "" {
		soraErrorResponse(c, http.StatusBadRequest, "parameter_missing", "model is required")
		return
	}

	task, err := h.taskService.CreateVideoTask(c.Request.Context(), apiKey.ID, account, &req, body)
	if err != nil {
		handleTaskCreateError(c, "CreateVideo", err)
		return
	}

	c.JSON(http.StatusOK, service.TaskToResponse(task))
}

func (h *SoraVideosHandler) GetVideo(c *gin.Context) {
	apiKey, ok := h.getAPIKey(c)
	if !ok {
		return
	}

	taskID := c.Param("id")
	if taskID == "" {
		soraErrorResponse(c, http.StatusBadRequest, "parameter_missing", "video_id is required")
		return
	}

	task, err := h.taskService.GetTask(c.Request.Context(), taskID, apiKey.ID)
	if err != nil {
		soraErrorResponse(c, http.StatusNotFound, "not_found", "Task not found")
		return
	}

	c.JSON(http.StatusOK, service.TaskToResponse(task))
}

func (h *SoraVideosHandler) RemixVideo(c *gin.Context) {
	apiKey, ok := h.getAPIKey(c)
	if !ok {
		return
	}

	taskID := c.Param("id")
	if taskID == "" {
		soraErrorResponse(c, http.StatusBadRequest, "parameter_missing", "video_id is required")
		return
	}

	body, err := readBody(c)
	if err != nil {
		return
	}

	var req service.RemixRequest
	if err := json.Unmarshal(body, &req); err != nil || req.Prompt == "" {
		soraErrorResponse(c, http.StatusBadRequest, "parameter_missing", "prompt is required")
		return
	}

	originalTask, err := h.taskService.GetTask(c.Request.Context(), taskID, apiKey.ID)
	if err != nil {
		soraErrorResponse(c, http.StatusNotFound, "not_found", "Original video task not found")
		return
	}
	if originalTask.Status != service.SoraTaskCompleted {
		soraErrorResponse(c, http.StatusBadRequest, "invalid_request_error", "Original video must be completed before remix")
		return
	}

	remixTargetID := originalTask.ShareID
	if remixTargetID == "" {
		remixTargetID = originalTask.UpstreamTaskID
	}

	account, err := h.selectAccountByID(c, originalTask.AccountID)
	if err != nil {
		soraErrorResponse(c, http.StatusServiceUnavailable, "server_error", "Failed to get account")
		return
	}

	videoReq := &service.CreateVideoRequest{
		Model:         originalTask.Model,
		Prompt:        req.Prompt,
		RemixTargetID: remixTargetID,
	}
	reqBody, _ := json.Marshal(videoReq)

	task, err := h.taskService.CreateVideoTask(c.Request.Context(), apiKey.ID, account, videoReq, reqBody)
	if err != nil {
		handleTaskCreateError(c, "RemixVideo", err)
		return
	}

	c.JSON(http.StatusOK, service.TaskToResponse(task))
}

// GetVideoContent returns video content based on storage configuration.
func (h *SoraVideosHandler) GetVideoContent(c *gin.Context) {
	apiKey, ok := h.getAPIKey(c)
	if !ok {
		return
	}

	taskID := c.Param("id")
	if taskID == "" {
		soraErrorResponse(c, http.StatusBadRequest, "parameter_missing", "video_id is required")
		return
	}

	task, err := h.taskService.GetTask(c.Request.Context(), taskID, apiKey.ID)
	if err != nil {
		soraErrorResponse(c, http.StatusNotFound, "not_found", "Task not found")
		return
	}

	switch task.Status {
	case service.SoraTaskCompleted:
		contentURL := h.resolveContentURL(c, task)
		if contentURL == "" {
			soraErrorResponse(c, http.StatusNotFound, "not_found", "Video URL not available")
			return
		}
		c.Redirect(http.StatusFound, contentURL)

	case service.SoraTaskFailed:
		c.JSON(http.StatusGone, gin.H{
			"id":     task.ID,
			"object": task.ObjectType,
			"status": task.Status,
			"error": gin.H{
				"message": task.ErrorMessage,
				"type":    task.ErrorType,
			},
		})

	default:
		c.JSON(http.StatusAccepted, service.TaskToResponse(task))
	}
}

func (h *SoraVideosHandler) resolveContentURL(c *gin.Context, task *service.SoraTask) string {
	if task.StoredKey == "" {
		return task.VideoURL
	}

	switch task.StorageType {
	case "s3", "gdrive":
		if h.objectStorage != nil {
			accessURL, err := h.objectStorage.GetAccessURL(c.Request.Context(), task.StoredKey)
			if err != nil {
				logger.LegacyPrintf("handler.sora_videos",
					"[GetVideoContent] task=%s get access URL error: %v, fallback to upstream", task.ID, err)
				return task.VideoURL
			}
			return accessURL
		}
		return task.VideoURL

	case "local":
		return "/sora/media" + task.StoredKey

	default:
		return task.VideoURL
	}
}

func (h *SoraVideosHandler) CreateImage(c *gin.Context) {
	apiKey, account, release, ok := h.selectAccount(c, "")
	if !ok {
		return
	}
	defer release()

	body, err := readBody(c)
	if err != nil {
		return
	}

	var req service.CreateImageRequest
	if err := json.Unmarshal(body, &req); err != nil {
		soraErrorResponse(c, http.StatusBadRequest, "invalid_request_error", "Invalid request body")
		return
	}
	if req.Prompt == "" {
		soraErrorResponse(c, http.StatusBadRequest, "parameter_missing", "prompt is required")
		return
	}
	if req.Model == "" {
		req.Model = inferImageModel(req.Size)
	}

	task, err := h.taskService.CreateImageGeneration(c.Request.Context(), apiKey.ID, account, &req, body)
	if err != nil {
		handleTaskCreateError(c, "CreateImage", err)
		return
	}

	c.JSON(http.StatusOK, service.TaskToResponse(task))
}

func (h *SoraVideosHandler) EditImage(c *gin.Context) {
	apiKey, account, release, ok := h.selectAccount(c, "")
	if !ok {
		return
	}
	defer release()

	body, err := readBody(c)
	if err != nil {
		return
	}

	var req service.EditImageRequest
	if err := json.Unmarshal(body, &req); err != nil {
		soraErrorResponse(c, http.StatusBadRequest, "invalid_request_error", "Invalid request body")
		return
	}
	if req.Image == "" {
		soraErrorResponse(c, http.StatusBadRequest, "parameter_missing", "image is required")
		return
	}
	if req.Prompt == "" {
		soraErrorResponse(c, http.StatusBadRequest, "parameter_missing", "prompt is required")
		return
	}
	if req.Model == "" {
		req.Model = inferImageModel(req.Size)
	}

	imageReq := &service.CreateImageRequest{
		Model:          req.Model,
		Prompt:         req.Prompt,
		Image:          req.Image,
		Size:           req.Size,
		ResponseFormat: req.ResponseFormat,
		N:              1,
	}

	task, err := h.taskService.CreateImageGeneration(c.Request.Context(), apiKey.ID, account, imageReq, body)
	if err != nil {
		handleTaskCreateError(c, "EditImage", err)
		return
	}

	c.JSON(http.StatusOK, service.TaskToResponse(task))
}

// ── Internal helpers ──

func (h *SoraVideosHandler) getAPIKey(c *gin.Context) (*service.APIKey, bool) {
	apiKey, ok := middleware2.GetAPIKeyFromContext(c)
	if !ok {
		soraErrorResponse(c, http.StatusUnauthorized, "authentication_error", "Invalid API key")
		return nil, false
	}
	return apiKey, true
}

func (h *SoraVideosHandler) selectAccount(c *gin.Context, model string) (*service.APIKey, *service.Account, func(), bool) {
	apiKey, ok := h.getAPIKey(c)
	if !ok {
		return nil, nil, nil, false
	}

	selection, err := h.gatewayService.SelectAccountWithLoadAwareness(
		c.Request.Context(), apiKey.GroupID, "", model, nil, "",
	)
	if err != nil {
		soraErrorResponse(c, http.StatusServiceUnavailable, "server_error", "No available accounts")
		return nil, nil, nil, false
	}

	releaseFunc := func() {}
	if selection.ReleaseFunc != nil {
		releaseFunc = selection.ReleaseFunc
	}
	return apiKey, selection.Account, releaseFunc, true
}

func (h *SoraVideosHandler) selectAccountByID(c *gin.Context, accountID int64) (*service.Account, error) {
	return h.taskService.GetAccountByID(c.Request.Context(), accountID)
}

func readBody(c *gin.Context) ([]byte, error) {
	body, err := c.GetRawData()
	if err != nil {
		soraErrorResponse(c, http.StatusBadRequest, "invalid_request_error", "Failed to read request body")
		return nil, err
	}
	if len(body) == 0 {
		soraErrorResponse(c, http.StatusBadRequest, "invalid_request_error", "Request body is empty")
		return nil, fmt.Errorf("empty body")
	}
	if !utf8.Valid(body) {
		soraErrorResponse(c, http.StatusBadRequest, "invalid_request_error", "Request body must be valid UTF-8")
		return nil, fmt.Errorf("invalid utf-8")
	}
	return body, nil
}

func soraErrorResponse(c *gin.Context, status int, errType, message string) {
	c.JSON(status, gin.H{
		"error": gin.H{
			"message": message,
			"type":    errType,
		},
	})
}

// handleTaskCreateError writes the appropriate error response, transparently
// forwarding upstream HTTP status codes when available.
func handleTaskCreateError(c *gin.Context, logTag string, err error) {
	logger.LegacyPrintf("handler.sora_videos", "[%s] error: %v", logTag, err)
	var ue *service.SoraUpstreamError
	if errors.As(err, &ue) {
		c.Data(ue.StatusCode, "application/json", ue.Body)
		return
	}
	soraErrorResponse(c, http.StatusInternalServerError, "server_error", "Failed to create task")
}

func inferImageModel(size string) string {
	switch size {
	case "540x360":
		return "gpt-image-landscape"
	case "360x540":
		return "gpt-image-portrait"
	default:
		return "gpt-image"
	}
}
