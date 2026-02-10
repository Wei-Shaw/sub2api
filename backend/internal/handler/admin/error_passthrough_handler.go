package admin

import (
	"strconv"

	"github.com/Wei-Shaw/sub2api/internal/model"
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

// ErrorPassthroughHandler 处理错误透传规则的 HTTP 请求
type ErrorPassthroughHandler struct {
	service *service.ErrorPassthroughService
REDACTED

// NewErrorPassthroughHandler 创建错误透传规则处理器
func NewErrorPassthroughHandler(service *service.ErrorPassthroughService) *ErrorPassthroughHandler {
	return &ErrorPassthroughHandler{service: serviceREDACTED
REDACTED

// CreateErrorPassthroughRuleRequest 创建规则请求
type CreateErrorPassthroughRuleRequest struct {
	Name            string   `json:"name" binding:"required"`
	Enabled         *bool    `json:"enabled"`
	Priority        int      `json:"priority"`
	ErrorCodes      []int    `json:"error_codes"`
	Keywords        []string `json:"keywords"`
	MatchMode       string   `json:"match_mode"`
	Platforms       []string `json:"platforms"`
	PassthroughCode *bool    `json:"passthrough_code"`
	ResponseCode    *int     `json:"response_code"`
	PassthroughBody *bool    `json:"passthrough_body"`
	CustomMessage   *string  `json:"custom_message"`
	SkipMonitoring  *bool    `json:"skip_monitoring"`
	Description     *string  `json:"description"`
REDACTED

// UpdateErrorPassthroughRuleRequest 更新规则请求（部分更新，所有字段可选）
type UpdateErrorPassthroughRuleRequest struct {
	Name            *string  `json:"name"`
	Enabled         *bool    `json:"enabled"`
	Priority        *int     `json:"priority"`
	ErrorCodes      []int    `json:"error_codes"`
	Keywords        []string `json:"keywords"`
	MatchMode       *string  `json:"match_mode"`
	Platforms       []string `json:"platforms"`
	PassthroughCode *bool    `json:"passthrough_code"`
	ResponseCode    *int     `json:"response_code"`
	PassthroughBody *bool    `json:"passthrough_body"`
	CustomMessage   *string  `json:"custom_message"`
	SkipMonitoring  *bool    `json:"skip_monitoring"`
	Description     *string  `json:"description"`
REDACTED

// List 获取所有规则
// GET /api/v1/admin/error-passthrough-rules
func (h *ErrorPassthroughHandler) List(c *gin.Context) {
	rules, err := h.service.List(c.Request.Context())
	if err != nil {
		response.ErrorFrom(c, err)
		return
REDACTED
	response.Success(c, rules)
REDACTED

// GetByID 根据 ID 获取规则
// GET /api/v1/admin/error-passthrough-rules/:id
func (h *ErrorPassthroughHandler) GetByID(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "Invalid rule ID")
		return
REDACTED

	rule, err := h.service.GetByID(c.Request.Context(), id)
	if err != nil {
		response.ErrorFrom(c, err)
		return
REDACTED
	if rule == nil {
		response.NotFound(c, "Rule not found")
		return
REDACTED

	response.Success(c, rule)
REDACTED

// Create 创建规则
// POST /api/v1/admin/error-passthrough-rules
func (h *ErrorPassthroughHandler) Create(c *gin.Context) {
	var req CreateErrorPassthroughRuleRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
REDACTED

	rule := &model.ErrorPassthroughRule{
		Name:       req.Name,
		Priority:   req.Priority,
		ErrorCodes: req.ErrorCodes,
		Keywords:   req.Keywords,
		Platforms:  req.Platforms,
REDACTED

	// 设置默认值
	if req.Enabled != nil {
		rule.Enabled = *req.Enabled
REDACTED else {
		rule.Enabled = true
REDACTED
	if req.MatchMode != "" {
		rule.MatchMode = req.MatchMode
REDACTED else {
		rule.MatchMode = model.MatchModeAny
REDACTED
	if req.PassthroughCode != nil {
		rule.PassthroughCode = *req.PassthroughCode
REDACTED else {
		rule.PassthroughCode = true
REDACTED
	if req.PassthroughBody != nil {
		rule.PassthroughBody = *req.PassthroughBody
REDACTED else {
		rule.PassthroughBody = true
REDACTED
	if req.SkipMonitoring != nil {
		rule.SkipMonitoring = *req.SkipMonitoring
REDACTED
	rule.ResponseCode = req.ResponseCode
	rule.CustomMessage = req.CustomMessage
	rule.Description = req.Description

	// 确保切片不为 nil
	if rule.ErrorCodes == nil {
		rule.ErrorCodes = []int{REDACTED
REDACTED
	if rule.Keywords == nil {
		rule.Keywords = []string{REDACTED
REDACTED
	if rule.Platforms == nil {
		rule.Platforms = []string{REDACTED
REDACTED

	created, err := h.service.Create(c.Request.Context(), rule)
	if err != nil {
		if _, ok := err.(*model.ValidationError); ok {
			response.BadRequest(c, err.Error())
			return
	REDACTED
		response.ErrorFrom(c, err)
		return
REDACTED

	response.Success(c, created)
REDACTED

// Update 更新规则（支持部分更新）
// PUT /api/v1/admin/error-passthrough-rules/:id
func (h *ErrorPassthroughHandler) Update(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "Invalid rule ID")
		return
REDACTED

	var req UpdateErrorPassthroughRuleRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
REDACTED

	// 先获取现有规则
	existing, err := h.service.GetByID(c.Request.Context(), id)
	if err != nil {
		response.ErrorFrom(c, err)
		return
REDACTED
	if existing == nil {
		response.NotFound(c, "Rule not found")
		return
REDACTED

	// 部分更新：只更新请求中提供的字段
	rule := &model.ErrorPassthroughRule{
		ID:              id,
		Name:            existing.Name,
		Enabled:         existing.Enabled,
		Priority:        existing.Priority,
		ErrorCodes:      existing.ErrorCodes,
		Keywords:        existing.Keywords,
		MatchMode:       existing.MatchMode,
		Platforms:       existing.Platforms,
		PassthroughCode: existing.PassthroughCode,
		ResponseCode:    existing.ResponseCode,
		PassthroughBody: existing.PassthroughBody,
		CustomMessage:   existing.CustomMessage,
		SkipMonitoring:  existing.SkipMonitoring,
		Description:     existing.Description,
REDACTED

	// 应用请求中提供的更新
	if req.Name != nil {
		rule.Name = *req.Name
REDACTED
	if req.Enabled != nil {
		rule.Enabled = *req.Enabled
REDACTED
	if req.Priority != nil {
		rule.Priority = *req.Priority
REDACTED
	if req.ErrorCodes != nil {
		rule.ErrorCodes = req.ErrorCodes
REDACTED
	if req.Keywords != nil {
		rule.Keywords = req.Keywords
REDACTED
	if req.MatchMode != nil {
		rule.MatchMode = *req.MatchMode
REDACTED
	if req.Platforms != nil {
		rule.Platforms = req.Platforms
REDACTED
	if req.PassthroughCode != nil {
		rule.PassthroughCode = *req.PassthroughCode
REDACTED
	if req.ResponseCode != nil {
		rule.ResponseCode = req.ResponseCode
REDACTED
	if req.PassthroughBody != nil {
		rule.PassthroughBody = *req.PassthroughBody
REDACTED
	if req.CustomMessage != nil {
		rule.CustomMessage = req.CustomMessage
REDACTED
	if req.Description != nil {
		rule.Description = req.Description
REDACTED
	if req.SkipMonitoring != nil {
		rule.SkipMonitoring = *req.SkipMonitoring
REDACTED

	// 确保切片不为 nil
	if rule.ErrorCodes == nil {
		rule.ErrorCodes = []int{REDACTED
REDACTED
	if rule.Keywords == nil {
		rule.Keywords = []string{REDACTED
REDACTED
	if rule.Platforms == nil {
		rule.Platforms = []string{REDACTED
REDACTED

	updated, err := h.service.Update(c.Request.Context(), rule)
	if err != nil {
		if _, ok := err.(*model.ValidationError); ok {
			response.BadRequest(c, err.Error())
			return
	REDACTED
		response.ErrorFrom(c, err)
		return
REDACTED

	response.Success(c, updated)
REDACTED

// Delete 删除规则
// DELETE /api/v1/admin/error-passthrough-rules/:id
func (h *ErrorPassthroughHandler) Delete(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "Invalid rule ID")
		return
REDACTED

	if err := h.service.Delete(c.Request.Context(), id); err != nil {
		response.ErrorFrom(c, err)
		return
REDACTED

	response.Success(c, gin.H{"message": "Rule deleted successfully"REDACTED)
REDACTED
