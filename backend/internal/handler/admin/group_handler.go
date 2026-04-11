package admin

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/handler/dto"
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/pkg/timezone"
	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/gin-gonic/gin"
)

// GroupHandler handles admin group management
type GroupHandler struct {
	adminService         service.AdminService
	dashboardService     *service.DashboardService
	groupCapacityService *service.GroupCapacityService
}

type optionalLimitField struct {
	set   bool
	value *float64
}

func (f *optionalLimitField) UnmarshalJSON(data []byte) error {
	f.set = true

	trimmed := bytes.TrimSpace(data)
	if bytes.Equal(trimmed, []byte("null")) {
		f.value = nil
		return nil
	}

	var number float64
	if err := json.Unmarshal(trimmed, &number); err == nil {
		f.value = &number
		return nil
	}

	var text string
	if err := json.Unmarshal(trimmed, &text); err == nil {
		text = strings.TrimSpace(text)
		if text == "" {
			f.value = nil
			return nil
		}
		number, err = strconv.ParseFloat(text, 64)
		if err != nil {
			return fmt.Errorf("invalid numeric limit value %q: %w", text, err)
		}
		f.value = &number
		return nil
	}

	return fmt.Errorf("invalid limit value: %s", string(trimmed))
}

func (f optionalLimitField) ToServiceInput() *float64 {
	if !f.set {
		return nil
	}
	if f.value != nil {
		return f.value
	}
	zero := 0.0
	return &zero
}

// NewGroupHandler creates a new admin group handler
func NewGroupHandler(adminService service.AdminService, dashboardService *service.DashboardService, groupCapacityService *service.GroupCapacityService) *GroupHandler {
	return &GroupHandler{
		adminService:         adminService,
		dashboardService:     dashboardService,
		groupCapacityService: groupCapacityService,
	}
}

// CreateGroupRequest represents create group request
type CreateGroupRequest struct {
	Name             string             `json:"name" binding:"required"`
	Description      string             `json:"description"`
	Platform         string             `json:"platform" binding:"omitempty,oneof=anthropic openai gemini antigravity"`
	RateMultiplier   float64            `json:"rate_multiplier"`
	IsExclusive      bool               `json:"is_exclusive"`
	SubscriptionType string             `json:"subscription_type" binding:"omitempty,oneof=standard subscription"`
	DailyLimitUSD    optionalLimitField `json:"daily_limit_usd"`
	WeeklyLimitUSD   optionalLimitField `json:"weekly_limit_usd"`
	MonthlyLimitUSD  optionalLimitField `json:"monthly_limit_usd"`
	// 图片生成计费配置（antigravity 和 gemini 平台使用，负数表示清除配置）
	ImagePrice1K                    *float64 `json:"image_price_1k"`
	ImagePrice2K                    *float64 `json:"image_price_2k"`
	ImagePrice4K                    *float64 `json:"image_price_4k"`
	ClaudeCodeOnly                  bool     `json:"claude_code_only"`
	FallbackGroupID                 *int64   `json:"fallback_group_id"`
	FallbackGroupIDOnInvalidRequest *int64   `json:"fallback_group_id_on_invalid_request"`
	// 模型路由配置（仅 anthropic 平台使用）
	ModelRouting        map[string][]int64 `json:"model_routing"`
	ModelRoutingEnabled bool               `json:"model_routing_enabled"`
	MCPXMLInject        *bool              `json:"mcp_xml_inject"`
	// 支持的模型系列（仅 antigravity 平台使用）
	SupportedModelScopes []string `json:"supported_model_scopes"`
	// OpenAI Messages 调度配置（仅 openai 平台使用）
	AllowMessagesDispatch       bool                                      `json:"allow_messages_dispatch"`
	RequireOAuthOnly            bool                                      `json:"require_oauth_only"`
	RequirePrivacySet           bool                                      `json:"require_privacy_set"`
	DefaultMappedModel          string                                    `json:"default_mapped_model"`
	MessagesDispatchModelConfig service.OpenAIMessagesDispatchModelConfig `json:"messages_dispatch_model_config"`
	// 从指定分组复制账号（创建后自动绑定）
	CopyAccountsFromGroupIDs []int64 `json:"copy_accounts_from_group_ids"`
}

// UpdateGroupRequest represents update group request
type UpdateGroupRequest struct {
	Name             string             `json:"name"`
	Description      string             `json:"description"`
	Platform         string             `json:"platform" binding:"omitempty,oneof=anthropic openai gemini antigravity"`
	RateMultiplier   *float64           `json:"rate_multiplier"`
	IsExclusive      *bool              `json:"is_exclusive"`
	Status           string             `json:"status" binding:"omitempty,oneof=active inactive"`
	SubscriptionType string             `json:"subscription_type" binding:"omitempty,oneof=standard subscription"`
	DailyLimitUSD    optionalLimitField `json:"daily_limit_usd"`
	WeeklyLimitUSD   optionalLimitField `json:"weekly_limit_usd"`
	MonthlyLimitUSD  optionalLimitField `json:"monthly_limit_usd"`
	// 图片生成计费配置（antigravity 和 gemini 平台使用，负数表示清除配置）
	ImagePrice1K                    *float64 `json:"image_price_1k"`
	ImagePrice2K                    *float64 `json:"image_price_2k"`
	ImagePrice4K                    *float64 `json:"image_price_4k"`
	ClaudeCodeOnly                  *bool    `json:"claude_code_only"`
	FallbackGroupID                 *int64   `json:"fallback_group_id"`
	FallbackGroupIDOnInvalidRequest *int64   `json:"fallback_group_id_on_invalid_request"`
	// 模型路由配置（仅 anthropic 平台使用）
	ModelRouting        map[string][]int64 `json:"model_routing"`
	ModelRoutingEnabled *bool              `json:"model_routing_enabled"`
	MCPXMLInject        *bool              `json:"mcp_xml_inject"`
	// 支持的模型系列（仅 antigravity 平台使用）
	SupportedModelScopes *[]string `json:"supported_model_scopes"`
	// OpenAI Messages 调度配置（仅 openai 平台使用）
	AllowMessagesDispatch       *bool                                      `json:"allow_messages_dispatch"`
	RequireOAuthOnly            *bool                                      `json:"require_oauth_only"`
	RequirePrivacySet           *bool                                      `json:"require_privacy_set"`
	DefaultMappedModel          *string                                    `json:"default_mapped_model"`
	MessagesDispatchModelConfig *service.OpenAIMessagesDispatchModelConfig `json:"messages_dispatch_model_config"`
	// 从指定分组复制账号（同步操作：先清空当前分组的账号绑定，再绑定源分组的账号）
	CopyAccountsFromGroupIDs []int64 `json:"copy_accounts_from_group_ids"`
}

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
	}
	isExclusiveStr := c.Query("is_exclusive")
	sortBy := c.DefaultQuery("sort_by", "sort_order")
	sortOrder := c.DefaultQuery("sort_order", "asc")

	var isExclusive *bool
	if isExclusiveStr != "" {
		val := isExclusiveStr == "true"
		isExclusive = &val
	}

	groups, total, err := h.adminService.ListGroups(c.Request.Context(), page, pageSize, platform, status, search, isExclusive, sortBy, sortOrder)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}

	outGroups := make([]dto.AdminGroup, 0, len(groups))
	for i := range groups {
		outGroups = append(outGroups, *dto.GroupFromServiceAdmin(&groups[i]))
	}
	response.Paginated(c, outGroups, total, page, pageSize)
}

// GetAll handles getting all active groups without pagination
// GET /api/v1/admin/groups/all
func (h *GroupHandler) GetAll(c *gin.Context) {
	platform := c.Query("platform")

	var groups []service.Group
	var err error

	if platform != "" {
		groups, err = h.adminService.GetAllGroupsByPlatform(c.Request.Context(), platform)
	} else {
		groups, err = h.adminService.GetAllGroups(c.Request.Context())
	}

	if err != nil {
		response.ErrorFrom(c, err)
		return
	}

	outGroups := make([]dto.AdminGroup, 0, len(groups))
	for i := range groups {
		outGroups = append(outGroups, *dto.GroupFromServiceAdmin(&groups[i]))
	}
	response.Success(c, outGroups)
}

// GetByID handles getting a group by ID
// GET /api/v1/admin/groups/:id
func (h *GroupHandler) GetByID(c *gin.Context) {
	groupID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "Invalid group ID")
		return
	}

	group, err := h.adminService.GetGroup(c.Request.Context(), groupID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}

	response.Success(c, dto.GroupFromServiceAdmin(group))
}

// Create handles creating a new group
// POST /api/v1/admin/groups
func (h *GroupHandler) Create(c *gin.Context) {
	var req CreateGroupRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}

	group, err := h.adminService.CreateGroup(c.Request.Context(), &service.CreateGroupInput{
		Name:                            req.Name,
		Description:                     req.Description,
		Platform:                        req.Platform,
		RateMultiplier:                  req.RateMultiplier,
		IsExclusive:                     req.IsExclusive,
		SubscriptionType:                req.SubscriptionType,
		DailyLimitUSD:                   req.DailyLimitUSD.ToServiceInput(),
		WeeklyLimitUSD:                  req.WeeklyLimitUSD.ToServiceInput(),
		MonthlyLimitUSD:                 req.MonthlyLimitUSD.ToServiceInput(),
		ImagePrice1K:                    req.ImagePrice1K,
		ImagePrice2K:                    req.ImagePrice2K,
		ImagePrice4K:                    req.ImagePrice4K,
		ClaudeCodeOnly:                  req.ClaudeCodeOnly,
		FallbackGroupID:                 req.FallbackGroupID,
		FallbackGroupIDOnInvalidRequest: req.FallbackGroupIDOnInvalidRequest,
		ModelRouting:                    req.ModelRouting,
		ModelRoutingEnabled:             req.ModelRoutingEnabled,
		MCPXMLInject:                    req.MCPXMLInject,
		SupportedModelScopes:            req.SupportedModelScopes,
		AllowMessagesDispatch:           req.AllowMessagesDispatch,
		RequireOAuthOnly:                req.RequireOAuthOnly,
		RequirePrivacySet:               req.RequirePrivacySet,
		DefaultMappedModel:              req.DefaultMappedModel,
		MessagesDispatchModelConfig:     req.MessagesDispatchModelConfig,
		CopyAccountsFromGroupIDs:        req.CopyAccountsFromGroupIDs,
	})
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}

	response.Success(c, dto.GroupFromServiceAdmin(group))
}

// Update handles updating a group
// PUT /api/v1/admin/groups/:id
func (h *GroupHandler) Update(c *gin.Context) {
	groupID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "Invalid group ID")
		return
	}

	var req UpdateGroupRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}

	group, err := h.adminService.UpdateGroup(c.Request.Context(), groupID, &service.UpdateGroupInput{
		Name:                            req.Name,
		Description:                     req.Description,
		Platform:                        req.Platform,
		RateMultiplier:                  req.RateMultiplier,
		IsExclusive:                     req.IsExclusive,
		Status:                          req.Status,
		SubscriptionType:                req.SubscriptionType,
		DailyLimitUSD:                   req.DailyLimitUSD.ToServiceInput(),
		WeeklyLimitUSD:                  req.WeeklyLimitUSD.ToServiceInput(),
		MonthlyLimitUSD:                 req.MonthlyLimitUSD.ToServiceInput(),
		ImagePrice1K:                    req.ImagePrice1K,
		ImagePrice2K:                    req.ImagePrice2K,
		ImagePrice4K:                    req.ImagePrice4K,
		ClaudeCodeOnly:                  req.ClaudeCodeOnly,
		FallbackGroupID:                 req.FallbackGroupID,
		FallbackGroupIDOnInvalidRequest: req.FallbackGroupIDOnInvalidRequest,
		ModelRouting:                    req.ModelRouting,
		ModelRoutingEnabled:             req.ModelRoutingEnabled,
		MCPXMLInject:                    req.MCPXMLInject,
		SupportedModelScopes:            req.SupportedModelScopes,
		AllowMessagesDispatch:           req.AllowMessagesDispatch,
		RequireOAuthOnly:                req.RequireOAuthOnly,
		RequirePrivacySet:               req.RequirePrivacySet,
		DefaultMappedModel:              req.DefaultMappedModel,
		MessagesDispatchModelConfig:     req.MessagesDispatchModelConfig,
		CopyAccountsFromGroupIDs:        req.CopyAccountsFromGroupIDs,
	})
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}

	response.Success(c, dto.GroupFromServiceAdmin(group))
}

// Delete handles deleting a group
// DELETE /api/v1/admin/groups/:id
func (h *GroupHandler) Delete(c *gin.Context) {
	groupID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "Invalid group ID")
		return
	}

	err = h.adminService.DeleteGroup(c.Request.Context(), groupID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}

	response.Success(c, gin.H{"message": "Group deleted successfully"})
}

// GetStats handles getting group statistics
// GET /api/v1/admin/groups/:id/stats
func (h *GroupHandler) GetStats(c *gin.Context) {
	groupID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "Invalid group ID")
		return
	}

	// Return mock data for now
	response.Success(c, gin.H{
		"total_api_keys":  0,
		"active_api_keys": 0,
		"total_requests":  0,
		"total_cost":      0.0,
	})
	_ = groupID // TODO: implement actual stats
}

// GetUsageSummary returns today's and cumulative cost for all groups.
// GET /api/v1/admin/groups/usage-summary?timezone=Asia/Shanghai
func (h *GroupHandler) GetUsageSummary(c *gin.Context) {
	userTZ := c.Query("timezone")
	now := timezone.NowInUserLocation(userTZ)
	todayStart := timezone.StartOfDayInUserLocation(now, userTZ)

	results, err := h.dashboardService.GetGroupUsageSummary(c.Request.Context(), todayStart)
	if err != nil {
		response.Error(c, 500, "Failed to get group usage summary")
		return
	}

	response.Success(c, results)
}

// GetCapacitySummary returns aggregated capacity (concurrency/sessions/RPM) for all active groups.
// GET /api/v1/admin/groups/capacity-summary
func (h *GroupHandler) GetCapacitySummary(c *gin.Context) {
	results, err := h.groupCapacityService.GetAllGroupCapacity(c.Request.Context())
	if err != nil {
		response.Error(c, 500, "Failed to get group capacity summary")
		return
	}
	response.Success(c, results)
}

// GetGroupAPIKeys handles getting API keys in a group
// GET /api/v1/admin/groups/:id/api-keys
func (h *GroupHandler) GetGroupAPIKeys(c *gin.Context) {
	groupID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "Invalid group ID")
		return
	}

	page, pageSize := response.ParsePagination(c)

	keys, total, err := h.adminService.GetGroupAPIKeys(c.Request.Context(), groupID, page, pageSize)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}

	outKeys := make([]dto.APIKey, 0, len(keys))
	for i := range keys {
		outKeys = append(outKeys, *dto.APIKeyFromService(&keys[i]))
	}
	response.Paginated(c, outKeys, total, page, pageSize)
}

// GetGroupRateMultipliers handles getting rate multipliers for users in a group
// GET /api/v1/admin/groups/:id/rate-multipliers
func (h *GroupHandler) GetGroupRateMultipliers(c *gin.Context) {
	groupID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "Invalid group ID")
		return
	}

	entries, err := h.adminService.GetGroupRateMultipliers(c.Request.Context(), groupID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}

	if entries == nil {
		entries = []service.UserGroupRateEntry{}
	}
	response.Success(c, entries)
}

// ClearGroupRateMultipliers handles clearing all rate multipliers for a group
// DELETE /api/v1/admin/groups/:id/rate-multipliers
func (h *GroupHandler) ClearGroupRateMultipliers(c *gin.Context) {
	groupID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "Invalid group ID")
		return
	}

	if err := h.adminService.ClearGroupRateMultipliers(c.Request.Context(), groupID); err != nil {
		response.ErrorFrom(c, err)
		return
	}

	response.Success(c, gin.H{"message": "Rate multipliers cleared successfully"})
}

// BatchSetGroupRateMultipliersRequest represents batch set rate multipliers request
type BatchSetGroupRateMultipliersRequest struct {
	Entries []service.GroupRateMultiplierInput `json:"entries" binding:"required"`
}

// BatchSetGroupRateMultipliers handles batch setting rate multipliers for a group
// PUT /api/v1/admin/groups/:id/rate-multipliers
func (h *GroupHandler) BatchSetGroupRateMultipliers(c *gin.Context) {
	groupID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "Invalid group ID")
		return
	}

	var req BatchSetGroupRateMultipliersRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}

	if err := h.adminService.BatchSetGroupRateMultipliers(c.Request.Context(), groupID, req.Entries); err != nil {
		response.ErrorFrom(c, err)
		return
	}

	response.Success(c, gin.H{"message": "Rate multipliers updated successfully"})
}

// UpdateSortOrderRequest represents the request to update group sort orders
type UpdateSortOrderRequest struct {
	Updates []struct {
		ID        int64 `json:"id" binding:"required"`
		SortOrder int   `json:"sort_order"`
	} `json:"updates" binding:"required,min=1"`
}

// UpdateSortOrder handles updating group sort orders
// PUT /api/v1/admin/groups/sort-order
func (h *GroupHandler) UpdateSortOrder(c *gin.Context) {
	var req UpdateSortOrderRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}

	updates := make([]service.GroupSortOrderUpdate, 0, len(req.Updates))
	for _, u := range req.Updates {
		updates = append(updates, service.GroupSortOrderUpdate{
			ID:        u.ID,
			SortOrder: u.SortOrder,
		})
	}

	if err := h.adminService.UpdateGroupSortOrders(c.Request.Context(), updates); err != nil {
		response.ErrorFrom(c, err)
		return
	}

	response.Success(c, gin.H{"message": "Sort order updated successfully"})
}
