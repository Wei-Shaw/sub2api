package admin

import (
	"strconv"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/handler/dto"
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/gin-gonic/gin"
)

// GroupHandler handles admin group management
type GroupHandler struct {
	adminService service.AdminService
REDACTED

// NewGroupHandler creates a new admin group handler
func NewGroupHandler(adminService service.AdminService) *GroupHandler {
	return &GroupHandler{
		adminService: adminService,
REDACTED
REDACTED

// CreateGroupRequest represents create group request
type CreateGroupRequest struct {
	Name             string   `json:"name" binding:"required"`
	Description      string   `json:"description"`
	Platform         string   `json:"platform" binding:"omitempty,oneof=anthropic openai gemini antigravity sora"`
	RateMultiplier   float64  `json:"rate_multiplier"`
	IsExclusive      bool     `json:"is_exclusive"`
	SubscriptionType string   `json:"subscription_type" binding:"omitempty,oneof=standard subscription"`
	DailyLimitUSD    *float64 `json:"daily_limit_usd"`
	WeeklyLimitUSD   *float64 `json:"weekly_limit_usd"`
	MonthlyLimitUSD  *float64 `json:"monthly_limit_usd"`
	// 图片生成计费配置（antigravity 和 gemini 平台使用，负数表示清除配置）
	ImagePrice1K                    *float64 `json:"image_price_1k"`
	ImagePrice2K                    *float64 `json:"image_price_2k"`
	ImagePrice4K                    *float64 `json:"image_price_4k"`
	SoraImagePrice360               *float64 `json:"sora_image_price_360"`
	SoraImagePrice540               *float64 `json:"sora_image_price_540"`
	SoraVideoPricePerRequest        *float64 `json:"sora_video_price_per_request"`
	SoraVideoPricePerRequestHD      *float64 `json:"sora_video_price_per_request_hd"`
	ClaudeCodeOnly                  bool     `json:"claude_code_only"`
	FallbackGroupID                 *int64   `json:"fallback_group_id"`
	FallbackGroupIDOnInvalidRequest *int64   `json:"fallback_group_id_on_invalid_request"`
	// 模型路由配置（仅 anthropic 平台使用）
	ModelRouting        map[string][]int64 `json:"model_routing"`
	ModelRoutingEnabled bool               `json:"model_routing_enabled"`
	MCPXMLInject        *bool              `json:"mcp_xml_inject"`
	// 支持的模型系列（仅 antigravity 平台使用）
	SupportedModelScopes []string `json:"supported_model_scopes"`
	// Sora 存储配额
	SoraStorageQuotaBytes int64 `json:"sora_storage_quota_bytes"`
	// 从指定分组复制账号（创建后自动绑定）
	CopyAccountsFromGroupIDs []int64 `json:"copy_accounts_from_group_ids"`
REDACTED

// UpdateGroupRequest represents update group request
type UpdateGroupRequest struct {
	Name             string   `json:"name"`
	Description      string   `json:"description"`
	Platform         string   `json:"platform" binding:"omitempty,oneof=anthropic openai gemini antigravity sora"`
	RateMultiplier   *float64 `json:"rate_multiplier"`
	IsExclusive      *bool    `json:"is_exclusive"`
	Status           string   `json:"status" binding:"omitempty,oneof=active inactive"`
	SubscriptionType string   `json:"subscription_type" binding:"omitempty,oneof=standard subscription"`
	DailyLimitUSD    *float64 `json:"daily_limit_usd"`
	WeeklyLimitUSD   *float64 `json:"weekly_limit_usd"`
	MonthlyLimitUSD  *float64 `json:"monthly_limit_usd"`
	// 图片生成计费配置（antigravity 和 gemini 平台使用，负数表示清除配置）
	ImagePrice1K                    *float64 `json:"image_price_1k"`
	ImagePrice2K                    *float64 `json:"image_price_2k"`
	ImagePrice4K                    *float64 `json:"image_price_4k"`
	SoraImagePrice360               *float64 `json:"sora_image_price_360"`
	SoraImagePrice540               *float64 `json:"sora_image_price_540"`
	SoraVideoPricePerRequest        *float64 `json:"sora_video_price_per_request"`
	SoraVideoPricePerRequestHD      *float64 `json:"sora_video_price_per_request_hd"`
	ClaudeCodeOnly                  *bool    `json:"claude_code_only"`
	FallbackGroupID                 *int64   `json:"fallback_group_id"`
	FallbackGroupIDOnInvalidRequest *int64   `json:"fallback_group_id_on_invalid_request"`
	// 模型路由配置（仅 anthropic 平台使用）
	ModelRouting        map[string][]int64 `json:"model_routing"`
	ModelRoutingEnabled *bool              `json:"model_routing_enabled"`
	MCPXMLInject        *bool              `json:"mcp_xml_inject"`
	// 支持的模型系列（仅 antigravity 平台使用）
	SupportedModelScopes *[]string `json:"supported_model_scopes"`
	// Sora 存储配额
	SoraStorageQuotaBytes *int64 `json:"sora_storage_quota_bytes"`
	// 从指定分组复制账号（同步操作：先清空当前分组的账号绑定，再绑定源分组的账号）
	CopyAccountsFromGroupIDs []int64 `json:"copy_accounts_from_group_ids"`
REDACTED

// List handles listing all groups with pagination
// GET /api/v1/admin/groups
func (h *GroupHandler) List(c *gin.Context) {
	page, pageSize := response.ParsePagination(c)
	platform := c.Query("platform")
	status := c.Query("status")
	search := c.Query("search")
	// 标准化和验证 search 参数
	search = strings.TrimSpace(search)
	if len(search) > 100 {
		search = search[:100]
REDACTED
	isExclusiveStr := c.Query("is_exclusive")

	var isExclusive *bool
	if isExclusiveStr != "" {
		val := isExclusiveStr == "true"
		isExclusive = &val
REDACTED

	groups, total, err := h.adminService.ListGroups(c.Request.Context(), page, pageSize, platform, status, search, isExclusive)
	if err != nil {
		response.ErrorFrom(c, err)
		return
REDACTED

	outGroups := make([]dto.AdminGroup, 0, len(groups))
	for i := range groups {
		outGroups = append(outGroups, *dto.GroupFromServiceAdmin(&groups[i]))
REDACTED
	response.Paginated(c, outGroups, total, page, pageSize)
REDACTED

// GetAll handles getting all active groups without pagination
// GET /api/v1/admin/groups/all
func (h *GroupHandler) GetAll(c *gin.Context) {
	platform := c.Query("platform")

	var groups []service.Group
	var err error

	if platform != "" {
		groups, err = h.adminService.GetAllGroupsByPlatform(c.Request.Context(), platform)
REDACTED else {
		groups, err = h.adminService.GetAllGroups(c.Request.Context())
REDACTED

	if err != nil {
		response.ErrorFrom(c, err)
		return
REDACTED

	outGroups := make([]dto.AdminGroup, 0, len(groups))
	for i := range groups {
		outGroups = append(outGroups, *dto.GroupFromServiceAdmin(&groups[i]))
REDACTED
	response.Success(c, outGroups)
REDACTED

// GetByID handles getting a group by ID
// GET /api/v1/admin/groups/:id
func (h *GroupHandler) GetByID(c *gin.Context) {
	groupID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "Invalid group ID")
		return
REDACTED

	group, err := h.adminService.GetGroup(c.Request.Context(), groupID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
REDACTED

	response.Success(c, dto.GroupFromServiceAdmin(group))
REDACTED

// Create handles creating a new group
// POST /api/v1/admin/groups
func (h *GroupHandler) Create(c *gin.Context) {
	var req CreateGroupRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
REDACTED

	group, err := h.adminService.CreateGroup(c.Request.Context(), &service.CreateGroupInput{
		Name:                            req.Name,
		Description:                     req.Description,
		Platform:                        req.Platform,
		RateMultiplier:                  req.RateMultiplier,
		IsExclusive:                     req.IsExclusive,
		SubscriptionType:                req.SubscriptionType,
		DailyLimitUSD:                   req.DailyLimitUSD,
		WeeklyLimitUSD:                  req.WeeklyLimitUSD,
		MonthlyLimitUSD:                 req.MonthlyLimitUSD,
		ImagePrice1K:                    req.ImagePrice1K,
		ImagePrice2K:                    req.ImagePrice2K,
		ImagePrice4K:                    req.ImagePrice4K,
		SoraImagePrice360:               req.SoraImagePrice360,
		SoraImagePrice540:               req.SoraImagePrice540,
		SoraVideoPricePerRequest:        req.SoraVideoPricePerRequest,
		SoraVideoPricePerRequestHD:      req.SoraVideoPricePerRequestHD,
		ClaudeCodeOnly:                  req.ClaudeCodeOnly,
		FallbackGroupID:                 req.FallbackGroupID,
		FallbackGroupIDOnInvalidRequest: req.FallbackGroupIDOnInvalidRequest,
		ModelRouting:                    req.ModelRouting,
		ModelRoutingEnabled:             req.ModelRoutingEnabled,
		MCPXMLInject:                    req.MCPXMLInject,
		SupportedModelScopes:            req.SupportedModelScopes,
		SoraStorageQuotaBytes:           req.SoraStorageQuotaBytes,
		CopyAccountsFromGroupIDs:        req.CopyAccountsFromGroupIDs,
REDACTED)
	if err != nil {
		response.ErrorFrom(c, err)
		return
REDACTED

	response.Success(c, dto.GroupFromServiceAdmin(group))
REDACTED

// Update handles updating a group
// PUT /api/v1/admin/groups/:id
func (h *GroupHandler) Update(c *gin.Context) {
	groupID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "Invalid group ID")
		return
REDACTED

	var req UpdateGroupRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
REDACTED

	group, err := h.adminService.UpdateGroup(c.Request.Context(), groupID, &service.UpdateGroupInput{
		Name:                            req.Name,
		Description:                     req.Description,
		Platform:                        req.Platform,
		RateMultiplier:                  req.RateMultiplier,
		IsExclusive:                     req.IsExclusive,
		Status:                          req.Status,
		SubscriptionType:                req.SubscriptionType,
		DailyLimitUSD:                   req.DailyLimitUSD,
		WeeklyLimitUSD:                  req.WeeklyLimitUSD,
		MonthlyLimitUSD:                 req.MonthlyLimitUSD,
		ImagePrice1K:                    req.ImagePrice1K,
		ImagePrice2K:                    req.ImagePrice2K,
		ImagePrice4K:                    req.ImagePrice4K,
		SoraImagePrice360:               req.SoraImagePrice360,
		SoraImagePrice540:               req.SoraImagePrice540,
		SoraVideoPricePerRequest:        req.SoraVideoPricePerRequest,
		SoraVideoPricePerRequestHD:      req.SoraVideoPricePerRequestHD,
		ClaudeCodeOnly:                  req.ClaudeCodeOnly,
		FallbackGroupID:                 req.FallbackGroupID,
		FallbackGroupIDOnInvalidRequest: req.FallbackGroupIDOnInvalidRequest,
		ModelRouting:                    req.ModelRouting,
		ModelRoutingEnabled:             req.ModelRoutingEnabled,
		MCPXMLInject:                    req.MCPXMLInject,
		SupportedModelScopes:            req.SupportedModelScopes,
		SoraStorageQuotaBytes:           req.SoraStorageQuotaBytes,
		CopyAccountsFromGroupIDs:        req.CopyAccountsFromGroupIDs,
REDACTED)
	if err != nil {
		response.ErrorFrom(c, err)
		return
REDACTED

	response.Success(c, dto.GroupFromServiceAdmin(group))
REDACTED

// Delete handles deleting a group
// DELETE /api/v1/admin/groups/:id
func (h *GroupHandler) Delete(c *gin.Context) {
	groupID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "Invalid group ID")
		return
REDACTED

	err = h.adminService.DeleteGroup(c.Request.Context(), groupID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
REDACTED

	response.Success(c, gin.H{"message": "Group deleted successfully"REDACTED)
REDACTED

// GetStats handles getting group statistics
// GET /api/v1/admin/groups/:id/stats
func (h *GroupHandler) GetStats(c *gin.Context) {
	groupID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "Invalid group ID")
		return
REDACTED

	// Return mock data for now
	response.Success(c, gin.H{
		"total_api_keys":  0,
		"active_api_keys": 0,
		"total_requests":  0,
		"total_cost":      0.0,
REDACTED)
	_ = groupID // TODO: implement actual stats
REDACTED

// GetGroupAPIKeys handles getting API keys in a group
// GET /api/v1/admin/groups/:id/api-keys
func (h *GroupHandler) GetGroupAPIKeys(c *gin.Context) {
	groupID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "Invalid group ID")
		return
REDACTED

	page, pageSize := response.ParsePagination(c)

	keys, total, err := h.adminService.GetGroupAPIKeys(c.Request.Context(), groupID, page, pageSize)
	if err != nil {
		response.ErrorFrom(c, err)
		return
REDACTED

	outKeys := make([]dto.APIKey, 0, len(keys))
	for i := range keys {
		outKeys = append(outKeys, *dto.APIKeyFromService(&keys[i]))
REDACTED
	response.Paginated(c, outKeys, total, page, pageSize)
REDACTED

// UpdateSortOrderRequest represents the request to update group sort orders
type UpdateSortOrderRequest struct {
	Updates []struct {
		ID        int64 `json:"id" binding:"required"`
		SortOrder int   `json:"sort_order"`
REDACTED `json:"updates" binding:"required,min=1"`
REDACTED

// UpdateSortOrder handles updating group sort orders
// PUT /api/v1/admin/groups/sort-order
func (h *GroupHandler) UpdateSortOrder(c *gin.Context) {
	var req UpdateSortOrderRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
REDACTED

	updates := make([]service.GroupSortOrderUpdate, 0, len(req.Updates))
	for _, u := range req.Updates {
		updates = append(updates, service.GroupSortOrderUpdate{
			ID:        u.ID,
			SortOrder: u.SortOrder,
	REDACTED)
REDACTED

	if err := h.adminService.UpdateGroupSortOrders(c.Request.Context(), updates); err != nil {
		response.ErrorFrom(c, err)
		return
REDACTED

	response.Success(c, gin.H{"message": "Sort order updated successfully"REDACTED)
REDACTED
