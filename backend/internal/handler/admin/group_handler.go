package admin

import (
	"strconv"

	"github.com/Wei-Shaw/sub2api/internal/model"
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/gin-gonic/gin"
)

// GroupHandler handles admin group management
type GroupHandler struct {
	adminService service.AdminService
}

// NewGroupHandler creates a new admin group handler
func NewGroupHandler(adminService service.AdminService) *GroupHandler {
	return &GroupHandler{
		adminService: adminService,
	}
}

// CreateGroupRequest represents create group request
type CreateGroupRequest struct {
	Name             string   `json:"name" binding:"required"`
	Description      string   `json:"description"`
	Platform         string   `json:"platform" binding:"omitempty,oneof=anthropic openai gemini"`
	RateMultiplier   float64  `json:"rate_multiplier"`
	IsExclusive      bool     `json:"is_exclusive"`
	SubscriptionType string   `json:"subscription_type" binding:"omitempty,oneof=standard subscription"`
	DailyLimitUSD    *float64 `json:"daily_limit_usd"`
	WeeklyLimitUSD   *float64 `json:"weekly_limit_usd"`
	MonthlyLimitUSD  *float64 `json:"monthly_limit_usd"`
}

// UpdateGroupRequest represents update group request
type UpdateGroupRequest struct {
	Name             string   `json:"name"`
	Description      string   `json:"description"`
	Platform         string   `json:"platform" binding:"omitempty,oneof=anthropic openai gemini"`
	RateMultiplier   *float64 `json:"rate_multiplier"`
	IsExclusive      *bool    `json:"is_exclusive"`
	Status           string   `json:"status" binding:"omitempty,oneof=active inactive"`
	SubscriptionType string   `json:"subscription_type" binding:"omitempty,oneof=standard subscription"`
	DailyLimitUSD    *float64 `json:"daily_limit_usd"`
	WeeklyLimitUSD   *float64 `json:"weekly_limit_usd"`
	MonthlyLimitUSD  *float64 `json:"monthly_limit_usd"`
}

// List handles listing all groups with pagination
// GET /api/v1/admin/groups
func (h *GroupHandler) List(c *gin.Context) {
	page, pageSize := response.ParsePagination(c)
	platform := c.Query("platform")
	status := c.Query("status")
	isExclusiveStr := c.Query("is_exclusive")

	var isExclusive *bool
	if isExclusiveStr != "" {
		val := isExclusiveStr == "true"
		isExclusive = &val
	}

	groups, total, err := h.adminService.ListGroups(c.Request.Context(), page, pageSize, platform, status, isExclusive)
	if err != nil {
		response.InternalError(c, "Failed to list groups: "+err.Error())
		return
	}

	response.Paginated(c, groups, total, page, pageSize)
}

// GetAll handles getting all active groups without pagination
// GET /api/v1/admin/groups/all
func (h *GroupHandler) GetAll(c *gin.Context) {
	platform := c.Query("platform")

	var groups []model.Group
	var err error

	if platform != "" {
		groups, err = h.adminService.GetAllGroupsByPlatform(c.Request.Context(), platform)
	} else {
		groups, err = h.adminService.GetAllGroups(c.Request.Context())
	}

	if err != nil {
		response.InternalError(c, "Failed to get groups: "+err.Error())
		return
	}

	response.Success(c, groups)
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
		response.NotFound(c, "Group not found")
		return
	}

	response.Success(c, group)
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
		Name:             req.Name,
		Description:      req.Description,
		Platform:         req.Platform,
		RateMultiplier:   req.RateMultiplier,
		IsExclusive:      req.IsExclusive,
		SubscriptionType: req.SubscriptionType,
		DailyLimitUSD:    req.DailyLimitUSD,
		WeeklyLimitUSD:   req.WeeklyLimitUSD,
		MonthlyLimitUSD:  req.MonthlyLimitUSD,
	})
	if err != nil {
		response.BadRequest(c, "Failed to create group: "+err.Error())
		return
	}

	response.Success(c, group)
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
		Name:             req.Name,
		Description:      req.Description,
		Platform:         req.Platform,
		RateMultiplier:   req.RateMultiplier,
		IsExclusive:      req.IsExclusive,
		Status:           req.Status,
		SubscriptionType: req.SubscriptionType,
		DailyLimitUSD:    req.DailyLimitUSD,
		WeeklyLimitUSD:   req.WeeklyLimitUSD,
		MonthlyLimitUSD:  req.MonthlyLimitUSD,
	})
	if err != nil {
		response.InternalError(c, "Failed to update group: "+err.Error())
		return
	}

	response.Success(c, group)
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
		response.InternalError(c, "Failed to delete group: "+err.Error())
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
		response.InternalError(c, "Failed to get group API keys: "+err.Error())
		return
	}

	response.Paginated(c, keys, total, page, pageSize)
}
