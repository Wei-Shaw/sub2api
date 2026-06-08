// Package monitorhandler hosts the channel-monitor HTTP handlers exposed by
// the channel-management plugin. There are two surfaces:
//
//   - AdminHandler   — privileged CRUD + run-now + history endpoints
//     mounted under /api/v1/plugin/channel-management/admin/monitors
//   - UserHandler    — read-only list / detail endpoints mounted under
//     /api/v1/plugin/channel-management/monitors
package monitorhandler

import (
	"net/http"
	"strconv"
	"strings"

	pluginsdk "github.com/Wei-Shaw/sub2api/plugin-sdk"
	"github.com/Wei-Shaw/sub2api/plugins/channel-management/internal/errors"
	"github.com/Wei-Shaw/sub2api/plugins/channel-management/internal/response"
	"github.com/Wei-Shaw/sub2api/plugins/channel-management/monitor/dto"
	monitorservice "github.com/Wei-Shaw/sub2api/plugins/channel-management/monitor/service"

	"github.com/gin-gonic/gin"
)

const (
	// monitorMaxPageSize 列表分页上限。
	monitorMaxPageSize = 100
)

// AdminHandler is the channel-monitor admin REST handler. The plugin wires
// it under /api/v1/plugin/channel-management/admin/monitors.
type AdminHandler struct {
	monitorService *monitorservice.ChannelMonitorService
}

// NewAdminHandler builds the admin handler. It is safe to pass a nil
// service when running in --no-http mode; routes will simply 503.
func NewAdminHandler(svc *monitorservice.ChannelMonitorService) *AdminHandler {
	return &AdminHandler{monitorService: svc}
}

// ParseChannelMonitorID extracts and validates the :id route parameter.
// Writes a 400 response and returns ok=false on invalid input.
func ParseChannelMonitorID(c *gin.Context) (int64, bool) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		response.ErrorFrom(c, errors.BadRequest("INVALID_MONITOR_ID", "invalid monitor id"))
		return 0, false
	}
	return id, true
}

// parseListEnabled parses the `enabled` query param. true/false → *bool;
// empty / unrecognised → nil.
func parseListEnabled(raw string) *bool {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "true", "1", "yes":
		v := true
		return &v
	case "false", "0", "no":
		v := false
		return &v
	default:
		return nil
	}
}

// parseHistoryLimit parses the limit query for the history endpoint, with
// service-level default + max bounds.
func parseHistoryLimit(raw string) int {
	if strings.TrimSpace(raw) == "" {
		return monitorservice.MonitorHistoryDefaultLimit
	}
	v, err := strconv.Atoi(raw)
	if err != nil || v <= 0 {
		return monitorservice.MonitorHistoryDefaultLimit
	}
	if v > monitorservice.MonitorHistoryMaxLimit {
		return monitorservice.MonitorHistoryMaxLimit
	}
	return v
}

// --- Handlers ---

// List GET /admin/monitors
func (h *AdminHandler) List(c *gin.Context) {
	if !h.requireService(c) {
		return
	}
	page, pageSize := response.ParsePagination(c)
	if pageSize > monitorMaxPageSize {
		pageSize = monitorMaxPageSize
	}

	params := monitorservice.ChannelMonitorListParams{
		Page:     page,
		PageSize: pageSize,
		Provider: strings.TrimSpace(c.Query("provider")),
		Enabled:  parseListEnabled(c.Query("enabled")),
		Search:   strings.TrimSpace(c.Query("search")),
	}

	items, total, err := h.monitorService.List(c.Request.Context(), params)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}

	summaries := h.batchSummaryFor(c, items)
	out := make([]*dto.ChannelMonitorResponse, 0, len(items))
	for _, m := range items {
		resp := dto.MonitorToResponse(m)
		dto.ApplyMonitorSummary(resp, summaries[m.ID])
		out = append(out, resp)
	}
	response.Paginated(c, out, total, page, pageSize)
}

// batchSummaryFor 批量聚合 latest + 7d 可用率，避免每行 2 次 SQL（消除 N+1）。
func (h *AdminHandler) batchSummaryFor(c *gin.Context, items []*monitorservice.ChannelMonitor) map[int64]monitorservice.MonitorStatusSummary {
	ids := make([]int64, 0, len(items))
	primaryByID := make(map[int64]string, len(items))
	extrasByID := make(map[int64][]string, len(items))
	for _, m := range items {
		ids = append(ids, m.ID)
		primaryByID[m.ID] = m.PrimaryModel
		extrasByID[m.ID] = m.ExtraModels
	}
	return h.monitorService.BatchMonitorStatusSummary(c.Request.Context(), ids, primaryByID, extrasByID)
}

// GetByID GET /admin/monitors/:id
func (h *AdminHandler) GetByID(c *gin.Context) {
	if !h.requireService(c) {
		return
	}
	id, ok := ParseChannelMonitorID(c)
	if !ok {
		return
	}
	m, err := h.monitorService.Get(c.Request.Context(), id)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, dto.MonitorToResponse(m))
}

// Create POST /admin/monitors
func (h *AdminHandler) Create(c *gin.Context) {
	if !h.requireService(c) {
		return
	}
	var req dto.ChannelMonitorCreateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.ErrorFrom(c, errors.BadRequest("VALIDATION_ERROR", err.Error()))
		return
	}

	meta := pluginsdk.RequestMetadata(c.Request)
	enabled := true
	if req.Enabled != nil {
		enabled = *req.Enabled
	}

	m, err := h.monitorService.Create(c.Request.Context(), monitorservice.ChannelMonitorCreateParams{
		Name:             req.Name,
		Provider:         req.Provider,
		Endpoint:         req.Endpoint,
		APIKey:           req.APIKey,
		PrimaryModel:     req.PrimaryModel,
		ExtraModels:      req.ExtraModels,
		GroupName:        req.GroupName,
		Enabled:          enabled,
		IntervalSeconds:  req.IntervalSeconds,
		CreatedBy:        meta.UserID,
		TemplateID:       req.TemplateID,
		ExtraHeaders:     req.ExtraHeaders,
		BodyOverrideMode: req.BodyOverrideMode,
		BodyOverride:     req.BodyOverride,
	})
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	c.JSON(http.StatusCreated, response.Response{Code: 0, Message: "success", Data: dto.MonitorToResponse(m)})
}

// Update PUT /admin/monitors/:id
func (h *AdminHandler) Update(c *gin.Context) {
	if !h.requireService(c) {
		return
	}
	id, ok := ParseChannelMonitorID(c)
	if !ok {
		return
	}
	var req dto.ChannelMonitorUpdateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.ErrorFrom(c, errors.BadRequest("VALIDATION_ERROR", err.Error()))
		return
	}

	m, err := h.monitorService.Update(c.Request.Context(), id, monitorservice.ChannelMonitorUpdateParams{
		Name:             req.Name,
		Provider:         req.Provider,
		Endpoint:         req.Endpoint,
		APIKey:           req.APIKey,
		PrimaryModel:     req.PrimaryModel,
		ExtraModels:      req.ExtraModels,
		GroupName:        req.GroupName,
		Enabled:          req.Enabled,
		IntervalSeconds:  req.IntervalSeconds,
		TemplateID:       req.TemplateID,
		ClearTemplate:    req.ClearTemplate,
		ExtraHeaders:     req.ExtraHeaders,
		BodyOverrideMode: req.BodyOverrideMode,
		BodyOverride:     req.BodyOverride,
	})
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, dto.MonitorToResponse(m))
}

// Delete DELETE /admin/monitors/:id
func (h *AdminHandler) Delete(c *gin.Context) {
	if !h.requireService(c) {
		return
	}
	id, ok := ParseChannelMonitorID(c)
	if !ok {
		return
	}
	if err := h.monitorService.Delete(c.Request.Context(), id); err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, nil)
}

// RunNow POST /admin/monitors/:id/run
func (h *AdminHandler) RunNow(c *gin.Context) {
	if !h.requireService(c) {
		return
	}
	id, ok := ParseChannelMonitorID(c)
	if !ok {
		return
	}
	results, err := h.monitorService.RunCheck(c.Request.Context(), id)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	out := make([]dto.ChannelMonitorCheckResultResponse, 0, len(results))
	for _, r := range results {
		out = append(out, dto.CheckResultToResponse(r))
	}
	response.Success(c, gin.H{"results": out})
}

// History GET /admin/monitors/:id/history
func (h *AdminHandler) History(c *gin.Context) {
	if !h.requireService(c) {
		return
	}
	id, ok := ParseChannelMonitorID(c)
	if !ok {
		return
	}
	limit := parseHistoryLimit(c.Query("limit"))
	model := strings.TrimSpace(c.Query("model"))

	entries, err := h.monitorService.ListHistory(c.Request.Context(), id, model, limit)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	out := make([]dto.ChannelMonitorHistoryItemResponse, 0, len(entries))
	for _, e := range entries {
		out = append(out, dto.HistoryEntryToResponse(e))
	}
	response.Success(c, gin.H{"items": out})
}
