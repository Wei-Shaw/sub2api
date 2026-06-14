package admin

import (
	"context"
	"errors"
	"log/slog"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/handler/dto"
	"github.com/Wei-Shaw/sub2api/internal/handler/quotaview"
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
	adminService          service.AdminService
	concurrencyService    *service.ConcurrencyService
	userPlatformQuotaRepo service.UserPlatformQuotaRepository // T13 admin quota view
	billingCache          service.BillingCache                // T17/T18 缓存失效（PUT/POST 路径）
}

// NewUserHandler creates a new admin user handler
func NewUserHandler(
	adminService service.AdminService,
	concurrencyService *service.ConcurrencyService,
	userPlatformQuotaRepo service.UserPlatformQuotaRepository,
	billingCache service.BillingCache,
) *UserHandler {
	return &UserHandler{
		adminService:          adminService,
		concurrencyService:    concurrencyService,
		userPlatformQuotaRepo: userPlatformQuotaRepo,
		billingCache:          billingCache,
	}
}

// CreateUserRequest represents admin create user request
type CreateUserRequest struct {
	Email         string   `json:"email" binding:"required,email"`
	Password      string   `json:"password" binding:"required,min=6"`
	Username      string   `json:"username"`
	Notes         string   `json:"notes"`
	Balance       *float64 `json:"balance"`
	Concurrency   int      `json:"concurrency"`
	RPMLimit      int      `json:"rpm_limit"`
	AllowedGroups []int64  `json:"allowed_groups"`
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
	RPMLimit      *int     `json:"rpm_limit"`
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
//   - api_key_group_id: filter by the exact group bound to the user's API keys
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
	if raw := strings.TrimSpace(c.Query("api_key_group_id")); raw != "" {
		if id, parseErr := strconv.ParseInt(raw, 10, 64); parseErr == nil && id > 0 {
			filters.APIKeyGroupID = id
		}
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

	var user *service.User
	if c.Query("include_deleted") == "true" {
		user, err = h.adminService.GetUserIncludeDeleted(c.Request.Context(), userID)
	} else {
		user, err = h.adminService.GetUser(c.Request.Context(), userID)
	}
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
		RPMLimit:      req.RPMLimit,
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
		RPMLimit:      req.RPMLimit,
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
//   - type: filter by record type (balance, affiliate_balance, admin_balance, concurrency, admin_concurrency, subscription)
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

// GetUserRPMStatus 返回指定用户当前分钟的 RPM 用量
// GET /api/v1/admin/users/:id/rpm-status
func (h *UserHandler) GetUserRPMStatus(c *gin.Context) {
	userID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "Invalid user ID")
		return
	}

	status, err := h.adminService.GetUserRPMStatus(c.Request.Context(), userID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}

	response.Success(c, status)
}

// BatchUpdateConcurrency 批量修改用户并发数
// POST /api/v1/admin/users/batch-concurrency
type BatchUpdateConcurrencyRequest struct {
	UserIDs     []int64 `json:"user_ids"`
	All         bool    `json:"all"`
	Concurrency int     `json:"concurrency"`
	Mode        string  `json:"mode" binding:"required,oneof=set add"`
}

func (h *UserHandler) BatchUpdateConcurrency(c *gin.Context) {
	var req BatchUpdateConcurrencyRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}
	if !req.All && len(req.UserIDs) == 0 {
		response.BadRequest(c, "user_ids is required unless all=true")
		return
	}
	if len(req.UserIDs) > 500 {
		response.BadRequest(c, "user_ids cannot exceed 500")
		return
	}

	var userIDs []int64
	if req.All {
		// Fetch all user IDs via pagination
		page := 1
		const pageSize = 500
		for {
			users, _, err := h.adminService.ListUsers(c.Request.Context(), page, pageSize, service.UserListFilters{}, "id", "asc")
			if err != nil {
				response.ErrorFrom(c, err)
				return
			}
			for _, u := range users {
				userIDs = append(userIDs, u.ID)
			}
			if len(users) < pageSize {
				break
			}
			page++
		}
	} else {
		userIDs = req.UserIDs
	}

	if len(userIDs) == 0 {
		response.Success(c, gin.H{"affected": 0})
		return
	}

	affected, err := h.adminService.BatchUpdateConcurrency(c.Request.Context(), userIDs, req.Concurrency, req.Mode)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, gin.H{"affected": affected})
}

// GetUserPlatformQuotas GET /admin/users/:id/platform-quotas
// admin 视角：D14 lazy 归零 + 暴露 *_window_start 调试字段
func (h *UserHandler) GetUserPlatformQuotas(c *gin.Context) {
	idStr := c.Param("id")
	userID, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		response.BadRequest(c, "invalid user id")
		return
	}
	if h.userPlatformQuotaRepo == nil {
		response.Success(c, map[string]any{"platform_quotas": []any{}})
		return
	}
	// 校验用户存在：与 PUT/POST 路径一致，不存在返回 404 而非空数组（避免 admin 界面误判用户存在）。
	if _, err := h.adminService.GetUser(c.Request.Context(), userID); err != nil {
		response.ErrorFrom(c, err)
		return
	}
	records, err := h.userPlatformQuotaRepo.ListByUser(c.Request.Context(), userID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	now := time.Now().UTC()
	out := make([]map[string]any, 0, len(records))
	for _, r := range records {
		out = append(out, quotaview.LazyZeroQuotaForResponse(r, now, true)) // true = 暴露 window_start
	}
	response.Success(c, map[string]any{"platform_quotas": out})
}

// UpdateUserPlatformQuotasRequest is the body for PUT /admin/users/:id/platform-quotas.
type UpdateUserPlatformQuotasRequest struct {
	Quotas []PlatformQuotaInput `json:"quotas" binding:"required"`
}

// PlatformQuotaInput 单平台限额输入；limit 字段为 nil 表示不限制。
type PlatformQuotaInput struct {
	Platform        string   `json:"platform" binding:"required"`
	DailyLimitUSD   *float64 `json:"daily_limit_usd"`
	WeeklyLimitUSD  *float64 `json:"weekly_limit_usd"`
	MonthlyLimitUSD *float64 `json:"monthly_limit_usd"`
}

// platform 合法性由 service.IsAllowedQuotaPlatform / service.AllowedQuotaPlatforms 统一判断（单一源）。

// UpdateUserPlatformQuotas PUT /admin/users/:id/platform-quotas
// 全量替换该用户所有平台限额。
func (h *UserHandler) UpdateUserPlatformQuotas(c *gin.Context) {
	if h.userPlatformQuotaRepo == nil {
		response.Error(c, 503, "platform quota service not available")
		return
	}

	userID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "Invalid user ID")
		return
	}

	var req UpdateUserPlatformQuotasRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}

	if len(req.Quotas) > 4 {
		response.BadRequest(c, "quotas length must be <= 4")
		return
	}
	seen := make(map[string]struct{}, len(req.Quotas))
	for _, q := range req.Quotas {
		if !service.IsAllowedQuotaPlatform(q.Platform) {
			response.BadRequest(c, "invalid platform: "+q.Platform)
			return
		}
		if _, dup := seen[q.Platform]; dup {
			response.BadRequest(c, "duplicate platform: "+q.Platform)
			return
		}
		seen[q.Platform] = struct{}{}
		// daily_limit_usd / weekly_limit_usd / monthly_limit_usd 的语义：
		//   nil / not set → 无限额（完全放行）
		//   0            → 完全禁用（任何请求都会被拒绝，因为 usage >= 0 恒成立）
		//   > 0          → USD 限额上限
		// 拦截 NaN / ±Inf：客户端可发送超大数（如 1e308 × 2）使 JSON 反序列化得到 +Inf，
		// 进入 DB 后 cache check 中 usage >= limit 永不成立，limit 等同失效。
		for _, f := range []struct {
			name string
			val  *float64
		}{
			{"daily_limit_usd", q.DailyLimitUSD},
			{"weekly_limit_usd", q.WeeklyLimitUSD},
			{"monthly_limit_usd", q.MonthlyLimitUSD},
		} {
			if f.val == nil {
				continue
			}
			v := *f.val
			if v < 0 {
				response.BadRequest(c, f.name+" must be >= 0")
				return
			}
			if math.IsNaN(v) || math.IsInf(v, 0) {
				response.BadRequest(c, f.name+" must be a finite number")
				return
			}
		}
	}

	records := make([]service.UserPlatformQuotaRecord, 0, len(req.Quotas))
	for _, q := range req.Quotas {
		records = append(records, service.UserPlatformQuotaRecord{
			UserID:          userID,
			Platform:        q.Platform,
			DailyLimitUSD:   q.DailyLimitUSD,
			WeeklyLimitUSD:  q.WeeklyLimitUSD,
			MonthlyLimitUSD: q.MonthlyLimitUSD,
		})
	}

	ctx := c.Request.Context()
	// 校验用户是否存在，避免 FK 违反导致 500；用户不存在时返回 404。
	if _, err := h.adminService.GetUser(ctx, userID); err != nil {
		response.ErrorFrom(c, err)
		return
	}
	// 在 UpsertForUser 之前抓取 before snapshot 用于审计 before/after 对比。
	// ListByUser 失败不阻断主操作（best-effort），仅记录降级 warn。
	beforeRecords, beforeErr := h.userPlatformQuotaRepo.ListByUser(ctx, userID)
	if beforeErr != nil {
		slog.Warn("quota audit before snapshot failed", "user_id", userID, "err", beforeErr)
	}
	if err := h.userPlatformQuotaRepo.UpsertForUser(ctx, userID, records); err != nil {
		response.ErrorFrom(c, err)
		return
	}

	beforeByPlatform := make(map[string]service.UserPlatformQuotaRecord, len(beforeRecords))
	for _, r := range beforeRecords {
		beforeByPlatform[r.Platform] = r
	}
	afterPlatforms := make(map[string]struct{}, len(records))
	for _, r := range records {
		afterPlatforms[r.Platform] = struct{}{}
	}
	changes := make([]map[string]any, 0, len(records))
	for _, r := range records {
		entry := map[string]any{
			"platform":          r.Platform,
			"daily_limit_usd":   r.DailyLimitUSD,
			"weekly_limit_usd":  r.WeeklyLimitUSD,
			"monthly_limit_usd": r.MonthlyLimitUSD,
		}
		if prev, ok := beforeByPlatform[r.Platform]; ok {
			entry["before_daily_limit_usd"] = prev.DailyLimitUSD
			entry["before_weekly_limit_usd"] = prev.WeeklyLimitUSD
			entry["before_monthly_limit_usd"] = prev.MonthlyLimitUSD
		}
		changes = append(changes, entry)
	}
	// 补 removed 条目：before 存在但 after 缺失 = 该平台被软删除。
	// 缺少这条记录，审计消费方无法察觉"管理员把某平台从配额列表移除"的操作（合规盲区）。
	for _, prev := range beforeRecords {
		if _, kept := afterPlatforms[prev.Platform]; kept {
			continue
		}
		changes = append(changes, map[string]any{
			"platform":                 prev.Platform,
			"removed":                  true,
			"before_daily_limit_usd":   prev.DailyLimitUSD,
			"before_weekly_limit_usd":  prev.WeeklyLimitUSD,
			"before_monthly_limit_usd": prev.MonthlyLimitUSD,
		})
	}
	// before_snapshot_available 让审计消费方能识别 changes 中是否带 before_* 字段；
	// false 时所有 entry 都会缺失 before_*_limit_usd，仅有 after 视图。
	slog.Info("admin.quota_updated",
		"actor_admin_id", getAdminIDFromContext(c),
		"target_user_id", userID,
		"platform_count", len(records),
		"before_snapshot_available", beforeErr == nil,
		"changes", changes)

	// 失效 cache：对全部允许的 platform 统一 invalidate。
	// Trade-off：精确失效（仅 req 涉及平台 + 被软删平台）需 upsert 前额外 ListByUser，
	// 增加一次 DB 查询和逻辑复杂度。由于 AllowedQuotaPlatforms 只有 4 个元素，
	// 全量 invalidate 的额外开销可接受，且能可靠覆盖软删除场景。
	if h.billingCache != nil {
		for _, p := range service.AllowedQuotaPlatforms {
			if err := h.billingCache.DeleteUserPlatformQuotaCache(ctx, userID, p); err != nil {
				slog.Error("ALERT: quota cache invalidation failed after UpsertForUser; limit 生效可能延迟至 sentinel TTL(最长 1h),需人工确认或重试失效", "user_id", userID, "platform", p, "err", err)
			}
		}
	}

	// 返回最新状态
	now := time.Now().UTC()
	records2, err := h.userPlatformQuotaRepo.ListByUser(ctx, userID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	out := make([]map[string]any, 0, len(records2))
	for i := range records2 {
		out = append(out, quotaview.LazyZeroQuotaForResponse(records2[i], now, true))
	}
	response.Success(c, map[string]any{"platform_quotas": out})
}

// ResetUserPlatformQuotaWindowRequest is the body for POST /admin/users/:id/platform-quotas/reset.
type ResetUserPlatformQuotaWindowRequest struct {
	Platform string `json:"platform" binding:"required"`
	Window   string `json:"window" binding:"required"`
}

var allowedWindowsForQuotaReset = map[string]struct{}{
	"daily":   {},
	"weekly":  {},
	"monthly": {},
}

// ResetUserPlatformQuotaWindow POST /admin/users/:id/platform-quotas/reset
// 立即归零指定 (platform, window) 的用量并更新 window_start。
func (h *UserHandler) ResetUserPlatformQuotaWindow(c *gin.Context) {
	if h.userPlatformQuotaRepo == nil {
		response.Error(c, 503, "platform quota service not available")
		return
	}

	userID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "Invalid user ID")
		return
	}

	var req ResetUserPlatformQuotaWindowRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}

	if !service.IsAllowedQuotaPlatform(req.Platform) {
		response.BadRequest(c, "invalid platform: "+req.Platform)
		return
	}
	if _, ok := allowedWindowsForQuotaReset[req.Window]; !ok {
		response.BadRequest(c, "invalid window: "+req.Window)
		return
	}

	ctx := c.Request.Context()
	// 校验用户是否存在，避免对不存在的用户执行操作返回误导性的 500。
	if _, err := h.adminService.GetUser(ctx, userID); err != nil {
		response.ErrorFrom(c, err)
		return
	}
	now := time.Now().UTC()
	if err := h.userPlatformQuotaRepo.ResetExpiredWindow(ctx, userID, req.Platform, req.Window, now); err != nil {
		if errors.Is(err, service.ErrUserPlatformQuotaNotFound) {
			response.NotFound(c, "user platform quota not found")
			return
		}
		response.ErrorFrom(c, err)
		return
	}

	slog.Info("admin.quota_window_reset",
		"actor_admin_id", getAdminIDFromContext(c),
		"target_user_id", userID,
		"platform", req.Platform,
		"window", req.Window)

	if h.billingCache != nil {
		if err := h.billingCache.DeleteUserPlatformQuotaCache(ctx, userID, req.Platform); err != nil {
			slog.Error("ALERT: quota cache invalidation failed after ResetExpiredWindow; 窗口重置可能延迟至 sentinel TTL(最长 1h)", "user_id", userID, "platform", req.Platform, "err", err)
		}
	}

	records, err := h.userPlatformQuotaRepo.ListByUser(ctx, userID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	out := make([]map[string]any, 0, len(records))
	for i := range records {
		out = append(out, quotaview.LazyZeroQuotaForResponse(records[i], now, true))
	}
	response.Success(c, map[string]any{"platform_quotas": out})
}
