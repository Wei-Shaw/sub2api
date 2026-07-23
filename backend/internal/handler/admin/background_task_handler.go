package admin

import (
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

type BackgroundTaskHandler struct {
	service *service.BackgroundTaskService
}

func NewBackgroundTaskHandler(backgroundTasks *service.BackgroundTaskService) *BackgroundTaskHandler {
	return &BackgroundTaskHandler{service: backgroundTasks}
}

type createOpenAIQuotaResetTaskRequest struct {
	ExpectedExpiresAt string `json:"expected_expires_at" binding:"required"`
	LeadTimeMinutes   int    `json:"lead_time_minutes" binding:"required"`
}

func (h *BackgroundTaskHandler) CreateOpenAIQuotaReset(c *gin.Context) {
	accountID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || accountID <= 0 {
		response.BadRequest(c, "Invalid account ID")
		return
	}
	var req createOpenAIQuotaResetTaskRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}
	expectedExpiresAt, err := time.Parse(time.RFC3339, req.ExpectedExpiresAt)
	if err != nil {
		response.BadRequest(c, "expected_expires_at must be RFC3339")
		return
	}
	actorID, ok := backgroundTaskActorID(c)
	if !ok {
		return
	}
	creationRequestKey, ok := backgroundTaskCreationRequestKey(c)
	if !ok {
		return
	}
	task, created, err := h.service.CreateOpenAIQuotaReset(
		c.Request.Context(), accountID, expectedExpiresAt, req.LeadTimeMinutes, creationRequestKey, actorID,
	)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	payload := gin.H{"task": service.PublicBackgroundTask(task), "created": created}
	if created {
		response.Created(c, payload)
		return
	}
	response.Success(c, payload)
}

func backgroundTaskCreationRequestKey(c *gin.Context) (string, bool) {
	key := strings.TrimSpace(c.GetHeader("Idempotency-Key"))
	if key == "" {
		response.BadRequest(c, "Idempotency-Key header is required")
		return "", false
	}
	if len(key) > service.BackgroundTaskCreationRequestKeyMaxLength {
		response.BadRequest(c, "Idempotency-Key header must not exceed 128 bytes")
		return "", false
	}
	return key, true
}

func (h *BackgroundTaskHandler) List(c *gin.Context) {
	page, pageSize := response.ParsePagination(c)
	status := service.BackgroundTaskStatus(c.Query("status"))
	if status != "" && !status.Valid() {
		response.BadRequest(c, "invalid background task status")
		return
	}
	result, err := h.service.List(c.Request.Context(), service.BackgroundTaskListFilter{
		TaskType: c.Query("task_type"), Status: status,
		ResourceType: c.Query("resource_type"), ResourceID: c.Query("resource_id"),
		Page: page, PageSize: pageSize,
	})
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	items := make([]*service.BackgroundTaskPublic, 0, len(result.Items))
	for _, task := range result.Items {
		items = append(items, service.PublicBackgroundTask(task))
	}
	response.Paginated(c, items, result.Total, result.Page, result.PageSize)
}

func (h *BackgroundTaskHandler) Cancel(c *gin.Context) {
	taskID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || taskID <= 0 {
		response.BadRequest(c, "Invalid task ID")
		return
	}
	actorID, ok := backgroundTaskActorID(c)
	if !ok {
		return
	}
	task, err := h.service.Cancel(c.Request.Context(), taskID, actorID)
	if err != nil {
		backgroundTaskError(c, err)
		return
	}
	response.Success(c, service.PublicBackgroundTask(task))
}

func (h *BackgroundTaskHandler) Retry(c *gin.Context) {
	taskID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || taskID <= 0 {
		response.BadRequest(c, "Invalid task ID")
		return
	}
	if _, ok := backgroundTaskActorID(c); !ok {
		return
	}
	task, err := h.service.RetryIndeterminate(c.Request.Context(), taskID)
	if err != nil {
		backgroundTaskError(c, err)
		return
	}
	response.Success(c, service.PublicBackgroundTask(task))
}

func backgroundTaskActorID(c *gin.Context) (int64, bool) {
	subject, ok := middleware.GetAuthSubjectFromContext(c)
	if !ok || subject.UserID <= 0 {
		response.Unauthorized(c, "authentication required")
		return 0, false
	}
	return subject.UserID, true
}

func backgroundTaskError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, service.ErrBackgroundTaskNotFound):
		response.NotFound(c, "background task not found")
	case errors.Is(err, service.ErrBackgroundTaskCannotCancel),
		errors.Is(err, service.ErrBackgroundTaskCannotRetry),
		errors.Is(err, service.ErrBackgroundTaskConflict):
		response.Error(c, http.StatusConflict, err.Error())
	default:
		response.ErrorFrom(c, err)
	}
}
