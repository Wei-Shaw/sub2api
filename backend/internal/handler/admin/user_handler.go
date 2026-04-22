package admin

import (
	"context"
	"strconv"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/handler/dto"
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/gin-gonic/gin"
)

// UserWithConcurrency wraps AdminUser with current concurrency info
type UserWithConcurrency struct {
	dto.AdminUser
	CurrentConcurrency int `json:"current_concurrency"`
}

// UserHandler handles admin user management
type UserHandler struct {
	adminService       service.AdminService
	concurrencyService *service.ConcurrencyService
}

// NewUserHandler creates a new admin user handler
func NewUserHandler(adminService service.AdminService, concurrencyService *service.ConcurrencyService) *UserHandler {
	return &UserHandler{
		adminService:       adminService,
		concurrencyService: concurrencyService,
	}
}

// CreateUserRequest represents admin create user request
type CreateUserRequest struct {
	Email         string  `json:"email" binding:"required,email"`
	Password      string  `json:"password" binding:"required,min=6"`
	Username      string  `json:"username"`
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
	Notes         *string  `json:"notes"`
	Balance       *float64 `json:"balance"`
	Concurrency   *int     `json:"concurrency"`
	Status        string   `json:"status" binding:"omitempty,oneof=active disabled"`
	AllowedGroups *[]int64 `json:"allowed_groups"`
	// GroupRates 用户专属分组倍率配置
	// map[groupID]*rate，nil 表示删除该分组的专属倍率
	GroupRates map[int64]*float64 `json:"group_rates"`
}

// UpdateBalanceRequest represents balance update request
type UpdateBalanceRequest struct {
	Balance   float64 `json:"balance" binding:"required,gt=0"`
	Operation string  `json:"operation" binding:"required,oneof=set add subtract"`
	Notes     string  `json:"notes"`
}

type BindUserAuthIdentityRequest struct {
	ProviderType    string                              `json:"provider_type"`
	ProviderKey     string                              `json:"provider_key"`
	ProviderSubject string                              `json:"provider_subject"`
	Issuer          *string                             `json:"issuer"`
	Metadata        map[string]any                      `json:"metadata"`
	Channel         *BindUserAuthIdentityChannelRequest `json:"channel"`
}

type BindUserAuthIdentityChannelRequest struct {
	Channel        string         `json:"channel"`
	ChannelAppID   string         `json:"channel_app_id"`
	ChannelSubject string         `json:"channel_subject"`
	Metadata       map[string]any `json:"metadata"`
}

// List handles listing all users with pagination
// GET /api/v1/admin/users
// Query params:
//   - status: filter by user status
//   - role: filter by user role
//   - search: search in email, username
//   - attr[{id}]: filter by custom attribute value, e.g. attr[1]=company
//   - group_name: fuzzy filter by allowed group name
func (h *UserHandler) List(c *gin.Context) {
	page, pageSize := response.ParsePagination(c)

	search := c.Query("search")
	// 标准化和验证 search 参数
	search = strings.TrimSpace(search)
	if runes := []rune(search); len(runes) > 100 {
		search = string(runes[:100])
	}

	filters := service.UserListFilters{
		Status:     c.Query("status"),
		Role:       c.Query("role"),
		Search:     search,
		GroupName:  strings.TrimSpace(c.Query("group_name")),
		Attributes: parseAttributeFilters(c),
	}
	sortBy := c.DefaultQuery("sort_by", "created_at")
	sortOrder := c.DefaultQuery("sort_order", "desc")
	if raw, ok := c.GetQuery("include_subscriptions"); ok {
		includeSubscriptions := parseBoolQueryWithDefault(raw, true)
		filters.IncludeSubscriptions = &includeSubscriptions
	}

	users, total, err := h.adminService.ListUsers(c.Request.Context(), page, pageSize, filters, sortBy, sortOrder)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}

	// Batch get current concurrency (nil map if unavailable)
	var loadInfo map[int64]*service.UserLoadInfo
	if len(users) > 0 && h.concurrencyService != nil {
		usersConcurrency := make([]service.UserWithConcurrency, len(users))
		for i := range users {
			usersConcurrency[i] = service.UserWithConcurrency{
				ID:             users[i].ID,
				MaxConcurrency: users[i].Concurrency,
			}
		}
		loadInfo, _ = h.concurrencyService.GetUsersLoadBatch(c.Request.Context(), usersConcurrency)
	}

	// Build response with concurrency info
	out := make([]UserWithConcurrency, len(users))
	for i := range users {
		out[i] = UserWithConcurrency{
			AdminUser: *dto.UserFromServiceAdmin(&users[i]),
		}
		if info := loadInfo[users[i].ID]; info != nil {
			out[i].CurrentConcurrency = info.CurrentConcurrency
		}
	}

	response.Paginated(c, out, total, page, pageSize)
}

// parseAttributeFilters extracts attribute filters from query params
// Format: attr[{attributeID}]=value, e.g. attr[1]=company&attr[2]=developer
func parseAttributeFilters(c *gin.Context) map[int64]string {
	result := make(map[int64]string)

	// Get all query params and look for attr[*] pattern
	for key, values := range c.Request.URL.Query() {
		if len(values) == 0 || values[0] == "" {
			continue
		}
		// Check if key matches pattern attr[{id}]
		if len(key) > 5 && key[:5] == "attr[" && key[len(key)-1] == ']' {
			idStr := key[5 : len(key)-1]
			id, err := strconv.ParseInt(idStr, 10, 64)
			if err == nil && id > 0 {
				result[id] = values[0]
			}
		}
	}

	return result
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
		response.ErrorFrom(c, err)
		return
	}

	response.Success(c, dto.UserFromServiceAdmin(user))
}

// BindAuthIdentity manually binds a canonical auth identity to a user.
// POST /api/v1/admin/users/:id/auth-identities
func (h *UserHandler) BindAuthIdentity(c *gin.Context) {
	userID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "Invalid user ID")
		return
	}

	var req BindUserAuthIdentityRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}

	input := service.AdminBindAuthIdentityInput{
		ProviderType:    req.ProviderType,
		ProviderKey:     req.ProviderKey,
		ProviderSubject: req.ProviderSubject,
		Issuer:          req.Issuer,
		Metadata:        req.Metadata,
	}
	if req.Channel != nil {
		input.Channel = &service.AdminBindAuthIdentityChannelInput{
			Channel:        req.Channel.Channel,
			ChannelAppID:   req.Channel.ChannelAppID,
			ChannelSubject: req.Channel.ChannelSubject,
			Metadata:       req.Channel.Metadata,
		}
	}

	result, err := h.adminService.BindUserAuthIdentity(c.Request.Context(), userID, input)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, result)
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
		Notes:         req.Notes,
		Balance:       req.Balance,
		Concurrency:   req.Concurrency,
		AllowedGroups: req.AllowedGroups,
	})
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}

	response.Success(c, dto.UserFromServiceAdmin(user))
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
		Notes:         req.Notes,
		Balance:       req.Balance,
		Concurrency:   req.Concurrency,
		Status:        req.Status,
		AllowedGroups: req.AllowedGroups,
		GroupRates:    req.GroupRates,
	})
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}

	response.Success(c, dto.UserFromServiceAdmin(user))
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
		response.ErrorFrom(c, err)
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

	idempotencyPayload := struct {
		UserID int64                `json:"user_id"`
		Body   UpdateBalanceRequest `json:"body"`
	}{
		UserID: userID,
		Body:   req,
	}
	executeAdminIdempotentJSON(c, "admin.users.balance.update", idempotencyPayload, service.DefaultWriteIdempotencyTTL(), func(ctx context.Context) (any, error) {
		user, execErr := h.adminService.UpdateUserBalance(ctx, userID, req.Balance, req.Operation, req.Notes)
		if execErr != nil {
			return nil, execErr
		}
		return dto.UserFromServiceAdmin(user), nil
	})
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
	sortBy := c.DefaultQuery("sort_by", "created_at")
	sortOrder := c.DefaultQuery("sort_order", "desc")

	keys, total, err := h.adminService.GetUserAPIKeys(c.Request.Context(), userID, page, pageSize, sortBy, sortOrder)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}

	out := make([]dto.APIKey, 0, len(keys))
	for i := range keys {
		out = append(out, *dto.APIKeyFromService(&keys[i]))
	}
	response.Paginated(c, out, total, page, pageSize)
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
		response.ErrorFrom(c, err)
		return
	}

	response.Success(c, stats)
}

// GetBalanceHistory handles getting user's balance/concurrency change history
// GET /api/v1/admin/users/:id/balance-history
// Query params:
//   - type: filter by record type (balance, admin_balance, concurrency, admin_concurrency, subscription)
func (h *UserHandler) GetBalanceHistory(c *gin.Context) {
	userID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "Invalid user ID")
		return
	}

	page, pageSize := response.ParsePagination(c)
	codeType := c.Query("type")

	codes, total, totalRecharged, err := h.adminService.GetUserBalanceHistory(c.Request.Context(), userID, page, pageSize, codeType)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}

	// Convert to admin DTO (includes notes field for admin visibility)
	out := make([]dto.AdminRedeemCode, 0, len(codes))
	for i := range codes {
		out = append(out, *dto.RedeemCodeFromServiceAdmin(&codes[i]))
	}

	// Custom response with total_recharged alongside pagination
	pages := int((total + int64(pageSize) - 1) / int64(pageSize))
	if pages < 1 {
		pages = 1
	}
	response.Success(c, gin.H{
		"items":           out,
		"total":           total,
		"page":            page,
		"page_size":       pageSize,
		"pages":           pages,
		"total_recharged": totalRecharged,
	})
}

// ReplaceGroupRequest represents the request to replace a user's exclusive group
type ReplaceGroupRequest struct {
	OldGroupID int64 `json:"old_group_id" binding:"required,gt=0"`
	NewGroupID int64 `json:"new_group_id" binding:"required,gt=0"`
}

// ReplaceGroup handles replacing a user's exclusive group
// POST /api/v1/admin/users/:id/replace-group
func (h *UserHandler) ReplaceGroup(c *gin.Context) {
	userID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "Invalid user ID")
		return
	}

	var req ReplaceGroupRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}

	result, err := h.adminService.ReplaceUserGroup(c.Request.Context(), userID, req.OldGroupID, req.NewGroupID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}

	response.Success(c, gin.H{
		"migrated_keys": result.MigratedKeys,
	})
}
