package admin

import (
	"strconv"

	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/gin-gonic/gin"
)

// UserHandler handles admin user management
type UserHandler struct {
	adminService service.AdminService
}

// NewUserHandler creates a new admin user handler
func NewUserHandler(adminService service.AdminService) *UserHandler {
	return &UserHandler{
		adminService: adminService,
	}
}

// CreateUserRequest represents admin create user request
type CreateUserRequest struct {
	Email         string  `json:"email" binding:"required,email"`
	Password      string  `json:"password" binding:"required,min=6"`
	Username      string  `json:"username"`
	Wechat        string  `json:"wechat"`
	Notes         string  `json:"notes"`
	Balance       float64 `json:"balance"`
	Concurrency   int     `json:"concurrency"`
	AllowedGroups []int64 `json:"allowed_groups"`
}

// UpdateUserRequest represents admin update user request
// 使用指针类型来区分"未提供"和"设置为0"
type UpdateUserRequest struct {
	Email         string   `json:"email" binding:"omitempty,email"`
	Password      string   `json:"password" binding:"omitempty,min=6"`
	Username      *string  `json:"username"`
	Wechat        *string  `json:"wechat"`
	Notes         *string  `json:"notes"`
	Balance       *float64 `json:"balance"`
	Concurrency   *int     `json:"concurrency"`
	Status        string   `json:"status" binding:"omitempty,oneof=active disabled"`
	AllowedGroups *[]int64 `json:"allowed_groups"`
}

// UpdateBalanceRequest represents balance update request
type UpdateBalanceRequest struct {
	Balance   float64 `json:"balance" binding:"required,gt=0"`
	Operation string  `json:"operation" binding:"required,oneof=set add subtract"`
	Notes     string  `json:"notes"`
}

// List handles listing all users with pagination
// GET /api/v1/admin/users
func (h *UserHandler) List(c *gin.Context) {
	page, pageSize := response.ParsePagination(c)
	status := c.Query("status")
	role := c.Query("role")
	search := c.Query("search")

	users, total, err := h.adminService.ListUsers(c.Request.Context(), page, pageSize, status, role, search)
	if err != nil {
		response.InternalError(c, "Failed to list users: "+err.Error())
		return
	}

	response.Paginated(c, users, total, page, pageSize)
}

// GetByID handles getting a user by ID
// GET /api/v1/admin/users/:id
func (h *UserHandler) GetByID(c *gin.Context) {
	userID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "Invalid user ID")
		return
	}

	user, err := h.adminService.GetUser(c.Request.Context(), userID)
	if err != nil {
		response.NotFound(c, "User not found")
		return
	}

	response.Success(c, user)
}

// Create handles creating a new user
// POST /api/v1/admin/users
func (h *UserHandler) Create(c *gin.Context) {
	var req CreateUserRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}

	user, err := h.adminService.CreateUser(c.Request.Context(), &service.CreateUserInput{
		Email:         req.Email,
		Password:      req.Password,
		Username:      req.Username,
		Wechat:        req.Wechat,
		Notes:         req.Notes,
		Balance:       req.Balance,
		Concurrency:   req.Concurrency,
		AllowedGroups: req.AllowedGroups,
	})
	if err != nil {
		response.BadRequest(c, "Failed to create user: "+err.Error())
		return
	}

	response.Success(c, user)
}

// Update handles updating a user
// PUT /api/v1/admin/users/:id
func (h *UserHandler) Update(c *gin.Context) {
	userID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "Invalid user ID")
		return
	}

	var req UpdateUserRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}

	// 使用指针类型直接传递，nil 表示未提供该字段
	user, err := h.adminService.UpdateUser(c.Request.Context(), userID, &service.UpdateUserInput{
		Email:         req.Email,
		Password:      req.Password,
		Username:      req.Username,
		Wechat:        req.Wechat,
		Notes:         req.Notes,
		Balance:       req.Balance,
		Concurrency:   req.Concurrency,
		Status:        req.Status,
		AllowedGroups: req.AllowedGroups,
	})
	if err != nil {
		response.InternalError(c, "Failed to update user: "+err.Error())
		return
	}

	response.Success(c, user)
}

// Delete handles deleting a user
// DELETE /api/v1/admin/users/:id
func (h *UserHandler) Delete(c *gin.Context) {
	userID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "Invalid user ID")
		return
	}

	err = h.adminService.DeleteUser(c.Request.Context(), userID)
	if err != nil {
		response.InternalError(c, "Failed to delete user: "+err.Error())
		return
	}

	response.Success(c, gin.H{"message": "User deleted successfully"})
}

// UpdateBalance handles updating user balance
// POST /api/v1/admin/users/:id/balance
func (h *UserHandler) UpdateBalance(c *gin.Context) {
	userID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "Invalid user ID")
		return
	}

	var req UpdateBalanceRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}

	user, err := h.adminService.UpdateUserBalance(c.Request.Context(), userID, req.Balance, req.Operation, req.Notes)
	if err != nil {
		response.InternalError(c, "Failed to update balance: "+err.Error())
		return
	}

	response.Success(c, user)
}

// GetUserAPIKeys handles getting user's API keys
// GET /api/v1/admin/users/:id/api-keys
func (h *UserHandler) GetUserAPIKeys(c *gin.Context) {
	userID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "Invalid user ID")
		return
	}

	page, pageSize := response.ParsePagination(c)

	keys, total, err := h.adminService.GetUserAPIKeys(c.Request.Context(), userID, page, pageSize)
	if err != nil {
		response.InternalError(c, "Failed to get user API keys: "+err.Error())
		return
	}

	response.Paginated(c, keys, total, page, pageSize)
}

// GetUserUsage handles getting user's usage statistics
// GET /api/v1/admin/users/:id/usage
func (h *UserHandler) GetUserUsage(c *gin.Context) {
	userID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "Invalid user ID")
		return
	}

	period := c.DefaultQuery("period", "month")

	stats, err := h.adminService.GetUserUsageStats(c.Request.Context(), userID, period)
	if err != nil {
		response.InternalError(c, "Failed to get user usage: "+err.Error())
		return
	}

	response.Success(c, stats)
}
