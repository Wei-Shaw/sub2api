package monitorhandler

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/plugins/channel-management/internal/errors"
	"github.com/Wei-Shaw/sub2api/plugins/channel-management/internal/response"
	monitorservice "github.com/Wei-Shaw/sub2api/plugins/channel-management/monitor/service"

	"github.com/gin-gonic/gin"
)

// TemplateHandler 是 channel-monitor request template 的管理后台 handler。
// plugin.go 将其挂在 /api/v1/plugin/channel-management/admin/channel-monitor-templates
// 下，对应前端 channelMonitorTemplate.ts 的 7 个调用点（CRUD + apply + listMonitors）。
//
// 鉴权：manifest 声明为 AuthTypeAdmin，host gateway 已经在进入 plugin 引擎之前
// 做过 admin 身份校验；本文件不再重复校验，只对 service 就绪状态做 503 兜底，
// 风格与 AdminHandler / UserHandler.requireService 一致。
type TemplateHandler struct {
	templateService *monitorservice.ChannelMonitorRequestTemplateService
}

// NewTemplateHandler 构造 handler。service 允许为 nil（--no-http / 未 wire 时），
// 调用时返回 503 + MONITOR_DISABLED，与 monitor admin handler 处理方式一致。
func NewTemplateHandler(svc *monitorservice.ChannelMonitorRequestTemplateService) *TemplateHandler {
	return &TemplateHandler{templateService: svc}
}

// requireService service 未注入时写 503 并返回 false。与 AdminHandler / UserHandler
// 的 requireService 同义，但挂在 TemplateHandler 上避免跨 receiver 共享。
func (h *TemplateHandler) requireService(c *gin.Context) bool {
	if h.templateService == nil {
		writeMonitorServiceUnavailable(c)
		return false
	}
	return true
}

// --- DTO ---

type channelMonitorTemplateCreateRequest struct {
	Name             string            `json:"name" binding:"required,max=100"`
	Provider         string            `json:"provider" binding:"required,oneof=openai anthropic gemini"`
	APIMode          string            `json:"api_mode" binding:"omitempty,oneof=chat_completions responses"`
	Description      string            `json:"description" binding:"max=500"`
	ExtraHeaders     map[string]string `json:"extra_headers"`
	BodyOverrideMode string            `json:"body_override_mode" binding:"omitempty,oneof=off merge replace"`
	BodyOverride     map[string]any    `json:"body_override"`
}

type channelMonitorTemplateUpdateRequest struct {
	Name             *string            `json:"name" binding:"omitempty,max=100"`
	APIMode          *string            `json:"api_mode" binding:"omitempty,oneof=chat_completions responses"`
	Description      *string            `json:"description" binding:"omitempty,max=500"`
	ExtraHeaders     *map[string]string `json:"extra_headers"`
	BodyOverrideMode *string            `json:"body_override_mode" binding:"omitempty,oneof=off merge replace"`
	BodyOverride     *map[string]any    `json:"body_override"`
}

type channelMonitorTemplateResponse struct {
	ID                 int64             `json:"id"`
	Name               string            `json:"name"`
	Provider           string            `json:"provider"`
	APIMode            string            `json:"api_mode"`
	Description        string            `json:"description"`
	ExtraHeaders       map[string]string `json:"extra_headers"`
	BodyOverrideMode   string            `json:"body_override_mode"`
	BodyOverride       map[string]any    `json:"body_override"`
	CreatedAt          string            `json:"created_at"`
	UpdatedAt          string            `json:"updated_at"`
	AssociatedMonitors int64             `json:"associated_monitors"`
}

// toResponse 把 service 层模板对象转成前端 JSON shape，附带 associated_monitors 统计。
func (h *TemplateHandler) toResponse(c *gin.Context, t *monitorservice.ChannelMonitorRequestTemplate) *channelMonitorTemplateResponse {
	if t == nil {
		return nil
	}
	headers := t.ExtraHeaders
	if headers == nil {
		headers = map[string]string{}
	}
	// 统计失败（例如 DB 暂断）不应阻塞主响应 —— host handler 也用 `_` 吞掉 err。
	count, _ := h.templateService.CountAssociatedMonitors(c.Request.Context(), t.ID)
	return &channelMonitorTemplateResponse{
		ID:                 t.ID,
		Name:               t.Name,
		Provider:           t.Provider,
		APIMode:            t.APIMode,
		Description:        t.Description,
		ExtraHeaders:       headers,
		BodyOverrideMode:   t.BodyOverrideMode,
		BodyOverride:       t.BodyOverride,
		CreatedAt:          t.CreatedAt.UTC().Format(time.RFC3339),
		UpdatedAt:          t.UpdatedAt.UTC().Format(time.RFC3339),
		AssociatedMonitors: count,
	}
}

// parseTemplateID 提取并校验 :id。写 400 并返回 ok=false 表示调用方应立即 return。
func parseTemplateID(c *gin.Context) (int64, bool) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		response.ErrorFrom(c, errors.BadRequest("INVALID_TEMPLATE_ID", "invalid template id"))
		return 0, false
	}
	return id, true
}

// --- Handlers ---

// List GET /admin/channel-monitor-templates?provider=anthropic
func (h *TemplateHandler) List(c *gin.Context) {
	if !h.requireService(c) {
		return
	}
	items, err := h.templateService.List(c.Request.Context(), monitorservice.ChannelMonitorRequestTemplateListParams{
		Provider: strings.TrimSpace(c.Query("provider")),
		APIMode:  strings.TrimSpace(c.Query("api_mode")),
	})
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	out := make([]*channelMonitorTemplateResponse, 0, len(items))
	for _, t := range items {
		out = append(out, h.toResponse(c, t))
	}
	response.Success(c, gin.H{"items": out})
}

// Get GET /admin/channel-monitor-templates/:id
func (h *TemplateHandler) Get(c *gin.Context) {
	if !h.requireService(c) {
		return
	}
	id, ok := parseTemplateID(c)
	if !ok {
		return
	}
	t, err := h.templateService.Get(c.Request.Context(), id)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, h.toResponse(c, t))
}

// Create POST /admin/channel-monitor-templates
func (h *TemplateHandler) Create(c *gin.Context) {
	if !h.requireService(c) {
		return
	}
	var req channelMonitorTemplateCreateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.ErrorFrom(c, errors.BadRequest("VALIDATION_ERROR", err.Error()))
		return
	}
	t, err := h.templateService.Create(c.Request.Context(), monitorservice.ChannelMonitorRequestTemplateCreateParams{
		Name:             req.Name,
		Provider:         req.Provider,
		APIMode:          req.APIMode,
		Description:      req.Description,
		ExtraHeaders:     req.ExtraHeaders,
		BodyOverrideMode: req.BodyOverrideMode,
		BodyOverride:     req.BodyOverride,
	})
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	// 与 AdminHandler.Create 保持一致：201 Created + 标准 envelope。
	c.JSON(http.StatusCreated, response.Response{Code: 0, Message: "success", Data: h.toResponse(c, t)})
}

// Update PUT /admin/channel-monitor-templates/:id
func (h *TemplateHandler) Update(c *gin.Context) {
	if !h.requireService(c) {
		return
	}
	id, ok := parseTemplateID(c)
	if !ok {
		return
	}
	var req channelMonitorTemplateUpdateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.ErrorFrom(c, errors.BadRequest("VALIDATION_ERROR", err.Error()))
		return
	}
	t, err := h.templateService.Update(c.Request.Context(), id, monitorservice.ChannelMonitorRequestTemplateUpdateParams{
		Name:             req.Name,
		APIMode:          req.APIMode,
		Description:      req.Description,
		ExtraHeaders:     req.ExtraHeaders,
		BodyOverrideMode: req.BodyOverrideMode,
		BodyOverride:     req.BodyOverride,
	})
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, h.toResponse(c, t))
}

// Delete DELETE /admin/channel-monitor-templates/:id
func (h *TemplateHandler) Delete(c *gin.Context) {
	if !h.requireService(c) {
		return
	}
	id, ok := parseTemplateID(c)
	if !ok {
		return
	}
	if err := h.templateService.Delete(c.Request.Context(), id); err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, nil)
}

// channelMonitorTemplateApplyRequest 是 Apply 端点的请求体。
// MonitorIDs 必填非空：picker 勾选的要覆盖的监控 id 子集。service 层 SQL
// WHERE template_id = :id 会把不关联的 id 过滤掉，返回的 affected 是真正写入的数量。
type channelMonitorTemplateApplyRequest struct {
	MonitorIDs []int64 `json:"monitor_ids" binding:"required,min=1"`
}

// Apply POST /admin/channel-monitor-templates/:id/apply
// 把模板当前配置覆盖到 monitor_ids 子集里的关联监控。
func (h *TemplateHandler) Apply(c *gin.Context) {
	if !h.requireService(c) {
		return
	}
	id, ok := parseTemplateID(c)
	if !ok {
		return
	}
	var req channelMonitorTemplateApplyRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.ErrorFrom(c, errors.BadRequest("VALIDATION_ERROR", err.Error()))
		return
	}
	affected, err := h.templateService.ApplyToMonitors(c.Request.Context(), id, req.MonitorIDs)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, gin.H{"affected": affected})
}

// associatedMonitorBriefResponse 是 AssociatedMonitors 端点的列表元素。
// 字段名与前端 AssociatedMonitorBrief interface 对齐（id/name/provider/enabled）。
type associatedMonitorBriefResponse struct {
	ID       int64  `json:"id"`
	Name     string `json:"name"`
	Provider string `json:"provider"`
	APIMode  string `json:"api_mode"`
	Enabled  bool   `json:"enabled"`
}

// AssociatedMonitors GET /admin/channel-monitor-templates/:id/monitors
// 列出模板当前关联的所有监控（picker 弹窗用）。
func (h *TemplateHandler) AssociatedMonitors(c *gin.Context) {
	if !h.requireService(c) {
		return
	}
	id, ok := parseTemplateID(c)
	if !ok {
		return
	}
	items, err := h.templateService.ListAssociatedMonitors(c.Request.Context(), id)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	out := make([]associatedMonitorBriefResponse, 0, len(items))
	for _, m := range items {
		out = append(out, associatedMonitorBriefResponse{
			ID: m.ID, Name: m.Name, Provider: m.Provider, APIMode: m.APIMode, Enabled: m.Enabled,
		})
	}
	response.Success(c, gin.H{"items": out})
}
