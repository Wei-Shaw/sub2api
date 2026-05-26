package admin

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

type opsSystemLogCleanupRequest struct {
	StartTime string `json:"start_time"`
	EndTime   string `json:"end_time"`

	Level           string `json:"level"`
	Component       string `json:"component"`
	RequestID       string `json:"request_id"`
	ClientRequestID string `json:"client_request_id"`
	UserID          *int64 `json:"user_id"`
	AccountID       *int64 `json:"account_id"`
	Platform        string `json:"platform"`
	Model           string `json:"model"`
	Query           string `json:"q"`
}

// ListSystemLogs returns indexed system logs.
// GET /api/v1/admin/ops/system-logs
func (h *OpsHandler) ListSystemLogs(c *gin.Context) {
	if h.opsService == nil {
		response.Error(c, http.StatusServiceUnavailable, "Ops service not available")
		return
	}
	if err := h.opsService.RequireMonitoringEnabled(c.Request.Context()); err != nil {
		response.ErrorFrom(c, err)
		return
	}

	page, pageSize := response.ParsePagination(c)
	if pageSize > 200 {
		pageSize = 200
	}

	start, end, err := parseOpsTimeRange(c, "1h")
	if err != nil {
		response.BadRequest(c, err.Error())
		return
	}

	filter := &service.OpsSystemLogFilter{
		Page:            page,
		PageSize:        pageSize,
		StartTime:       &start,
		EndTime:         &end,
		Level:           strings.TrimSpace(c.Query("level")),
		Component:       strings.TrimSpace(c.Query("component")),
		RequestID:       strings.TrimSpace(c.Query("request_id")),
		ClientRequestID: strings.TrimSpace(c.Query("client_request_id")),
		Platform:        strings.TrimSpace(c.Query("platform")),
		Model:           strings.TrimSpace(c.Query("model")),
		Query:           strings.TrimSpace(c.Query("q")),
	}
	if v := strings.TrimSpace(c.Query("user_id")); v != "" {
		id, parseErr := strconv.ParseInt(v, 10, 64)
		if parseErr != nil || id <= 0 {
			response.BadRequest(c, "Invalid user_id")
			return
		}
		filter.UserID = &id
	}
	if v := strings.TrimSpace(c.Query("account_id")); v != "" {
		id, parseErr := strconv.ParseInt(v, 10, 64)
		if parseErr != nil || id <= 0 {
			response.BadRequest(c, "Invalid account_id")
			return
		}
		filter.AccountID = &id
	}

	result, err := h.opsService.ListSystemLogs(c.Request.Context(), filter)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Paginated(c, result.Logs, int64(result.Total), result.Page, result.PageSize)
}

// CleanupSystemLogs deletes indexed system logs by filter.
// POST /api/v1/admin/ops/system-logs/cleanup
func (h *OpsHandler) CleanupSystemLogs(c *gin.Context) {
	if h.opsService == nil {
		response.Error(c, http.StatusServiceUnavailable, "Ops service not available")
		return
	}
	if err := h.opsService.RequireMonitoringEnabled(c.Request.Context()); err != nil {
		response.ErrorFrom(c, err)
		return
	}

	subject, ok := middleware.GetAuthSubjectFromContext(c)
	if !ok || subject.UserID <= 0 {
		response.Error(c, http.StatusUnauthorized, "Unauthorized")
		return
	}

	var req opsSystemLogCleanupRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request body")
		return
	}

	parseTS := func(raw string) (*time.Time, error) {
		raw = strings.TrimSpace(raw)
		if raw == "" {
			return nil, nil
		}
		if t, err := time.Parse(time.RFC3339Nano, raw); err == nil {
			return &t, nil
		}
		t, err := time.Parse(time.RFC3339, raw)
		if err != nil {
			return nil, err
		}
		return &t, nil
	}
	start, err := parseTS(req.StartTime)
	if err != nil {
		response.BadRequest(c, "Invalid start_time")
		return
	}
	end, err := parseTS(req.EndTime)
	if err != nil {
		response.BadRequest(c, "Invalid end_time")
		return
	}

	filter := &service.OpsSystemLogCleanupFilter{
		StartTime:       start,
		EndTime:         end,
		Level:           strings.TrimSpace(req.Level),
		Component:       strings.TrimSpace(req.Component),
		RequestID:       strings.TrimSpace(req.RequestID),
		ClientRequestID: strings.TrimSpace(req.ClientRequestID),
		UserID:          req.UserID,
		AccountID:       req.AccountID,
		Platform:        strings.TrimSpace(req.Platform),
		Model:           strings.TrimSpace(req.Model),
		Query:           strings.TrimSpace(req.Query),
	}

	deleted, err := h.opsService.CleanupSystemLogs(c.Request.Context(), filter, subject.UserID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, gin.H{"deleted": deleted})
}

// GetSystemLogIngestionHealth returns sink health metrics.
// GET /api/v1/admin/ops/system-logs/health
func (h *OpsHandler) GetSystemLogIngestionHealth(c *gin.Context) {
	if h.opsService == nil {
		response.Error(c, http.StatusServiceUnavailable, "Ops service not available")
		return
	}
	if err := h.opsService.RequireMonitoringEnabled(c.Request.Context()); err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, h.opsService.GetSystemLogSinkHealth())
}
