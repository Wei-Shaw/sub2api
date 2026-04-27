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
	"time"

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
	// monitorAPIKeyMaskPrefix 脱敏时保留的明文前缀长度。
	monitorAPIKeyMaskPrefix = 4
	// monitorAPIKeyMaskSuffix 脱敏后追加的占位字符串。
	monitorAPIKeyMaskSuffix = "***"
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

// --- Request / Response ---

type channelMonitorCreateRequest struct {
	Name             string            `json:"name" binding:"required,max=100"`
	Provider         string            `json:"provider" binding:"required,oneof=openai anthropic gemini"`
	Endpoint         string            `json:"endpoint" binding:"required,max=500"`
	APIKey           string            `json:"api_key" binding:"required,max=2000"`
	PrimaryModel     string            `json:"primary_model" binding:"required,max=200"`
	ExtraModels      []string          `json:"extra_models"`
	GroupName        string            `json:"group_name" binding:"max=100"`
	Enabled          *bool             `json:"enabled"`
	IntervalSeconds  int               `json:"interval_seconds" binding:"required,min=15,max=3600"`
	TemplateID       *int64            `json:"template_id"`
	ExtraHeaders     map[string]string `json:"extra_headers"`
	BodyOverrideMode string            `json:"body_override_mode" binding:"omitempty,oneof=off merge replace"`
	BodyOverride     map[string]any    `json:"body_override"`
}

type channelMonitorUpdateRequest struct {
	Name             *string            `json:"name" binding:"omitempty,max=100"`
	Provider         *string            `json:"provider" binding:"omitempty,oneof=openai anthropic gemini"`
	Endpoint         *string            `json:"endpoint" binding:"omitempty,max=500"`
	APIKey           *string            `json:"api_key" binding:"omitempty,max=2000"`
	PrimaryModel     *string            `json:"primary_model" binding:"omitempty,max=200"`
	ExtraModels      *[]string          `json:"extra_models"`
	GroupName        *string            `json:"group_name" binding:"omitempty,max=100"`
	Enabled          *bool              `json:"enabled"`
	IntervalSeconds  *int               `json:"interval_seconds" binding:"omitempty,min=15,max=3600"`
	TemplateID       *int64             `json:"template_id"`
	ClearTemplate    bool               `json:"clear_template"`
	ExtraHeaders     *map[string]string `json:"extra_headers"`
	BodyOverrideMode *string            `json:"body_override_mode" binding:"omitempty,oneof=off merge replace"`
	BodyOverride     *map[string]any    `json:"body_override"`
}

type channelMonitorResponse struct {
	ID                  int64                                `json:"id"`
	Name                string                               `json:"name"`
	Provider            string                               `json:"provider"`
	Endpoint            string                               `json:"endpoint"`
	APIKeyMasked        string                               `json:"api_key_masked"`
	APIKeyDecryptFailed bool                                 `json:"api_key_decrypt_failed"`
	PrimaryModel        string                               `json:"primary_model"`
	ExtraModels         []string                             `json:"extra_models"`
	GroupName           string                               `json:"group_name"`
	Enabled             bool                                 `json:"enabled"`
	IntervalSeconds     int                                  `json:"interval_seconds"`
	LastCheckedAt       *string                              `json:"last_checked_at"`
	CreatedBy           int64                                `json:"created_by"`
	CreatedAt           string                               `json:"created_at"`
	UpdatedAt           string                               `json:"updated_at"`
	PrimaryStatus       string                               `json:"primary_status"`
	PrimaryLatencyMs    *int                                 `json:"primary_latency_ms"`
	Availability7d      float64                              `json:"availability_7d"`
	ExtraModelsStatus   []dto.ChannelMonitorExtraModelStatus `json:"extra_models_status"`
	TemplateID          *int64                               `json:"template_id"`
	ExtraHeaders        map[string]string                    `json:"extra_headers"`
	BodyOverrideMode    string                               `json:"body_override_mode"`
	BodyOverride        map[string]any                       `json:"body_override"`
}

type channelMonitorCheckResultResponse struct {
	Model         string `json:"model"`
	Status        string `json:"status"`
	LatencyMs     *int   `json:"latency_ms"`
	PingLatencyMs *int   `json:"ping_latency_ms"`
	Message       string `json:"message"`
	CheckedAt     string `json:"checked_at"`
}

type channelMonitorHistoryItemResponse struct {
	ID            int64  `json:"id"`
	Model         string `json:"model"`
	Status        string `json:"status"`
	LatencyMs     *int   `json:"latency_ms"`
	PingLatencyMs *int   `json:"ping_latency_ms"`
	Message       string `json:"message"`
	CheckedAt     string `json:"checked_at"`
}

// maskAPIKey 对 API Key 明文做脱敏：前 4 字符 + "***"，长度 ≤ 4 时只显示 "***"。
func maskAPIKey(plain string) string {
	if len(plain) <= monitorAPIKeyMaskPrefix {
		return monitorAPIKeyMaskSuffix
	}
	return plain[:monitorAPIKeyMaskPrefix] + monitorAPIKeyMaskSuffix
}

func channelMonitorToResponse(m *monitorservice.ChannelMonitor) *channelMonitorResponse {
	if m == nil {
		return nil
	}
	extras := m.ExtraModels
	if extras == nil {
		extras = []string{}
	}
	headers := m.ExtraHeaders
	if headers == nil {
		headers = map[string]string{}
	}
	resp := &channelMonitorResponse{
		ID:                  m.ID,
		Name:                m.Name,
		Provider:            m.Provider,
		Endpoint:            m.Endpoint,
		APIKeyMasked:        maskAPIKey(m.APIKey),
		APIKeyDecryptFailed: m.APIKeyDecryptFailed,
		PrimaryModel:        m.PrimaryModel,
		ExtraModels:         extras,
		GroupName:           m.GroupName,
		Enabled:             m.Enabled,
		IntervalSeconds:     m.IntervalSeconds,
		CreatedBy:           m.CreatedBy,
		CreatedAt:           m.CreatedAt.UTC().Format(time.RFC3339),
		UpdatedAt:           m.UpdatedAt.UTC().Format(time.RFC3339),
		TemplateID:          m.TemplateID,
		ExtraHeaders:        headers,
		BodyOverrideMode:    m.BodyOverrideMode,
		BodyOverride:        m.BodyOverride,
		ExtraModelsStatus:   []dto.ChannelMonitorExtraModelStatus{},
	}
	if m.LastCheckedAt != nil {
		s := m.LastCheckedAt.UTC().Format(time.RFC3339)
		resp.LastCheckedAt = &s
	}
	return resp
}

func checkResultToResponse(r *monitorservice.CheckResult) channelMonitorCheckResultResponse {
	return channelMonitorCheckResultResponse{
		Model:         r.Model,
		Status:        r.Status,
		LatencyMs:     r.LatencyMs,
		PingLatencyMs: r.PingLatencyMs,
		Message:       r.Message,
		CheckedAt:     r.CheckedAt.UTC().Format(time.RFC3339),
	}
}

func historyEntryToResponse(e *monitorservice.ChannelMonitorHistoryEntry) channelMonitorHistoryItemResponse {
	return channelMonitorHistoryItemResponse{
		ID:            e.ID,
		Model:         e.Model,
		Status:        e.Status,
		LatencyMs:     e.LatencyMs,
		PingLatencyMs: e.PingLatencyMs,
		Message:       e.Message,
		CheckedAt:     e.CheckedAt.UTC().Format(time.RFC3339),
	}
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
	if h.monitorService == nil {
		response.ErrorWithDetails(c, http.StatusServiceUnavailable, "monitor service unavailable", "MONITOR_DISABLED", nil)
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
	out := make([]*channelMonitorResponse, 0, len(items))
	for _, m := range items {
		out = append(out, buildListItemResponse(m, summaries[m.ID]))
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

// buildListItemResponse 把 monitor + summary 装成 admin list 的响应行。
func buildListItemResponse(m *monitorservice.ChannelMonitor, summary monitorservice.MonitorStatusSummary) *channelMonitorResponse {
	resp := channelMonitorToResponse(m)
	resp.PrimaryStatus = summary.PrimaryStatus
	resp.PrimaryLatencyMs = summary.PrimaryLatencyMs
	resp.Availability7d = summary.Availability7d
	resp.ExtraModelsStatus = make([]dto.ChannelMonitorExtraModelStatus, 0, len(summary.ExtraModels))
	for _, e := range summary.ExtraModels {
		resp.ExtraModelsStatus = append(resp.ExtraModelsStatus, dto.ChannelMonitorExtraModelStatus{
			Model:     e.Model,
			Status:    e.Status,
			LatencyMs: e.LatencyMs,
		})
	}
	return resp
}

// GetByID GET /admin/monitors/:id
func (h *AdminHandler) GetByID(c *gin.Context) {
	if h.monitorService == nil {
		response.ErrorWithDetails(c, http.StatusServiceUnavailable, "monitor service unavailable", "MONITOR_DISABLED", nil)
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
	response.Success(c, channelMonitorToResponse(m))
}

// Create POST /admin/monitors
func (h *AdminHandler) Create(c *gin.Context) {
	if h.monitorService == nil {
		response.ErrorWithDetails(c, http.StatusServiceUnavailable, "monitor service unavailable", "MONITOR_DISABLED", nil)
		return
	}
	var req channelMonitorCreateRequest
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
	c.JSON(http.StatusCreated, response.Response{Code: 0, Message: "success", Data: channelMonitorToResponse(m)})
}

// Update PUT /admin/monitors/:id
func (h *AdminHandler) Update(c *gin.Context) {
	if h.monitorService == nil {
		response.ErrorWithDetails(c, http.StatusServiceUnavailable, "monitor service unavailable", "MONITOR_DISABLED", nil)
		return
	}
	id, ok := ParseChannelMonitorID(c)
	if !ok {
		return
	}
	var req channelMonitorUpdateRequest
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
	response.Success(c, channelMonitorToResponse(m))
}

// Delete DELETE /admin/monitors/:id
func (h *AdminHandler) Delete(c *gin.Context) {
	if h.monitorService == nil {
		response.ErrorWithDetails(c, http.StatusServiceUnavailable, "monitor service unavailable", "MONITOR_DISABLED", nil)
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
	if h.monitorService == nil {
		response.ErrorWithDetails(c, http.StatusServiceUnavailable, "monitor service unavailable", "MONITOR_DISABLED", nil)
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
	out := make([]channelMonitorCheckResultResponse, 0, len(results))
	for _, r := range results {
		out = append(out, checkResultToResponse(r))
	}
	response.Success(c, gin.H{"results": out})
}

// History GET /admin/monitors/:id/history
func (h *AdminHandler) History(c *gin.Context) {
	if h.monitorService == nil {
		response.ErrorWithDetails(c, http.StatusServiceUnavailable, "monitor service unavailable", "MONITOR_DISABLED", nil)
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
	out := make([]channelMonitorHistoryItemResponse, 0, len(entries))
	for _, e := range entries {
		out = append(out, historyEntryToResponse(e))
	}
	response.Success(c, gin.H{"items": out})
}
