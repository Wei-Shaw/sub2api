package admin

import (
	"strconv"
	"strings"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/gin-gonic/gin"
)

// ChannelMonitorRequestTemplateHandler 请求模板管理后台 handler。
type ChannelMonitorRequestTemplateHandler struct {
	templateService *service.ChannelMonitorRequestTemplateService
REDACTED

// NewChannelMonitorRequestTemplateHandler 创建 handler。
func NewChannelMonitorRequestTemplateHandler(templateService *service.ChannelMonitorRequestTemplateService) *ChannelMonitorRequestTemplateHandler {
	return &ChannelMonitorRequestTemplateHandler{templateService: templateServiceREDACTED
REDACTED

// --- DTO ---

type channelMonitorTemplateCreateRequest struct {
	Name             string            `json:"name" binding:"required,max=100"`
	Provider         string            `json:"provider" binding:"required,oneof=openai anthropic gemini"`
	Description      string            `json:"description" binding:"max=500"`
	ExtraHeaders     map[string]string `json:"extra_headers"`
	BodyOverrideMode string            `json:"body_override_mode" binding:"omitempty,oneof=off merge replace"`
	BodyOverride     map[string]any    `json:"body_override"`
REDACTED

type channelMonitorTemplateUpdateRequest struct {
	Name             *string            `json:"name" binding:"omitempty,max=100"`
	Description      *string            `json:"description" binding:"omitempty,max=500"`
	ExtraHeaders     *map[string]string `json:"extra_headers"`
	BodyOverrideMode *string            `json:"body_override_mode" binding:"omitempty,oneof=off merge replace"`
	BodyOverride     *map[string]any    `json:"body_override"`
REDACTED

type channelMonitorTemplateResponse struct {
	ID                 int64             `json:"id"`
	Name               string            `json:"name"`
	Provider           string            `json:"provider"`
	Description        string            `json:"description"`
	ExtraHeaders       map[string]string `json:"extra_headers"`
	BodyOverrideMode   string            `json:"body_override_mode"`
	BodyOverride       map[string]any    `json:"body_override"`
	CreatedAt          string            `json:"created_at"`
	UpdatedAt          string            `json:"updated_at"`
	AssociatedMonitors int64             `json:"associated_monitors"`
REDACTED

func (h *ChannelMonitorRequestTemplateHandler) toResponse(c *gin.Context, t *service.ChannelMonitorRequestTemplate) *channelMonitorTemplateResponse {
	if t == nil {
		return nil
REDACTED
	headers := t.ExtraHeaders
	if headers == nil {
		headers = map[string]string{REDACTED
REDACTED
	count, _ := h.templateService.CountAssociatedMonitors(c.Request.Context(), t.ID)
	return &channelMonitorTemplateResponse{
		ID:                 t.ID,
		Name:               t.Name,
		Provider:           t.Provider,
		Description:        t.Description,
		ExtraHeaders:       headers,
		BodyOverrideMode:   t.BodyOverrideMode,
		BodyOverride:       t.BodyOverride,
		CreatedAt:          t.CreatedAt.UTC().Format(time.RFC3339),
		UpdatedAt:          t.UpdatedAt.UTC().Format(time.RFC3339),
		AssociatedMonitors: count,
REDACTED
REDACTED

// parseTemplateID 提取并校验 :id。
func parseTemplateID(c *gin.Context) (int64, bool) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		response.ErrorFrom(c, infraerrors.BadRequest("INVALID_TEMPLATE_ID", "invalid template id"))
		return 0, false
REDACTED
	return id, true
REDACTED

// --- Handlers ---

// List GET /api/v1/admin/channel-monitor-templates?provider=anthropic
func (h *ChannelMonitorRequestTemplateHandler) List(c *gin.Context) {
	items, err := h.templateService.List(c.Request.Context(), service.ChannelMonitorRequestTemplateListParams{
		Provider: strings.TrimSpace(c.Query("provider")),
REDACTED)
	if err != nil {
		response.ErrorFrom(c, err)
		return
REDACTED
	out := make([]*channelMonitorTemplateResponse, 0, len(items))
	for _, t := range items {
		out = append(out, h.toResponse(c, t))
REDACTED
	response.Success(c, gin.H{"items": outREDACTED)
REDACTED

// Get GET /api/v1/admin/channel-monitor-templates/:id
func (h *ChannelMonitorRequestTemplateHandler) Get(c *gin.Context) {
	id, ok := parseTemplateID(c)
	if !ok {
		return
REDACTED
	t, err := h.templateService.Get(c.Request.Context(), id)
	if err != nil {
		response.ErrorFrom(c, err)
		return
REDACTED
	response.Success(c, h.toResponse(c, t))
REDACTED

// Create POST /api/v1/admin/channel-monitor-templates
func (h *ChannelMonitorRequestTemplateHandler) Create(c *gin.Context) {
	var req channelMonitorTemplateCreateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.ErrorFrom(c, infraerrors.BadRequest("VALIDATION_ERROR", err.Error()))
		return
REDACTED
	t, err := h.templateService.Create(c.Request.Context(), service.ChannelMonitorRequestTemplateCreateParams{
		Name:             req.Name,
		Provider:         req.Provider,
		Description:      req.Description,
		ExtraHeaders:     req.ExtraHeaders,
		BodyOverrideMode: req.BodyOverrideMode,
		BodyOverride:     req.BodyOverride,
REDACTED)
	if err != nil {
		response.ErrorFrom(c, err)
		return
REDACTED
	response.Created(c, h.toResponse(c, t))
REDACTED

// Update PUT /api/v1/admin/channel-monitor-templates/:id
func (h *ChannelMonitorRequestTemplateHandler) Update(c *gin.Context) {
	id, ok := parseTemplateID(c)
	if !ok {
		return
REDACTED
	var req channelMonitorTemplateUpdateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.ErrorFrom(c, infraerrors.BadRequest("VALIDATION_ERROR", err.Error()))
		return
REDACTED
	t, err := h.templateService.Update(c.Request.Context(), id, service.ChannelMonitorRequestTemplateUpdateParams{
		Name:             req.Name,
		Description:      req.Description,
		ExtraHeaders:     req.ExtraHeaders,
		BodyOverrideMode: req.BodyOverrideMode,
		BodyOverride:     req.BodyOverride,
REDACTED)
	if err != nil {
		response.ErrorFrom(c, err)
		return
REDACTED
	response.Success(c, h.toResponse(c, t))
REDACTED

// Delete DELETE /api/v1/admin/channel-monitor-templates/:id
func (h *ChannelMonitorRequestTemplateHandler) Delete(c *gin.Context) {
	id, ok := parseTemplateID(c)
	if !ok {
		return
REDACTED
	if err := h.templateService.Delete(c.Request.Context(), id); err != nil {
		response.ErrorFrom(c, err)
		return
REDACTED
	response.Success(c, nil)
REDACTED

// Apply POST /api/v1/admin/channel-monitor-templates/:id/apply
// 一键把模板当前配置覆盖到所有关联监控上。
func (h *ChannelMonitorRequestTemplateHandler) Apply(c *gin.Context) {
	id, ok := parseTemplateID(c)
	if !ok {
		return
REDACTED
	affected, err := h.templateService.ApplyToMonitors(c.Request.Context(), id)
	if err != nil {
		response.ErrorFrom(c, err)
		return
REDACTED
	response.Success(c, gin.H{"affected": affectedREDACTED)
REDACTED
