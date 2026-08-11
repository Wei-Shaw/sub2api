package admin

import (
	"context"
	"strconv"

	"github.com/Wei-Shaw/sub2api/internal/handler/dto"
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	middleware2 "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/gin-gonic/gin"
)

// AdminAPIKeyHandler handles admin API key management
type AdminAPIKeyHandler struct {
	adminService  service.AdminService
	apiKeyService *service.APIKeyService
}

// NewAdminAPIKeyHandler creates a new admin API key handler
func NewAdminAPIKeyHandler(adminService service.AdminService, apiKeyService *service.APIKeyService) *AdminAPIKeyHandler {
	return &AdminAPIKeyHandler{
		adminService:  adminService,
		apiKeyService: apiKeyService,
	}
}

// AdminUpdateAPIKeyGroupRequest represents the request to update an API key.
type AdminUpdateAPIKeyGroupRequest struct {
	GroupID             *int64 `json:"group_id"`               // nil=不修改, 0=解绑, >0=绑定到目标分组
	ResetRateLimitUsage *bool  `json:"reset_rate_limit_usage"` // true=重置 5h/1d/7d 限速用量
}

// UpdateGroup handles updating an API key's admin-managed fields.
// PUT /api/v1/admin/api-keys/:id
func (h *AdminAPIKeyHandler) UpdateGroup(c *gin.Context) {
	keyID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "Invalid API key ID")
		return
	}

	var req AdminUpdateAPIKeyGroupRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}

	var resetKey *service.APIKey
	if req.ResetRateLimitUsage != nil && *req.ResetRateLimitUsage {
		resetKey, err = h.adminService.AdminResetAPIKeyRateLimitUsage(c.Request.Context(), keyID)
		if err != nil {
			response.ErrorFrom(c, err)
			return
		}
	}

	result, err := h.adminService.AdminUpdateAPIKeyGroupID(c.Request.Context(), keyID, req.GroupID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	if resetKey != nil && req.GroupID == nil {
		result.APIKey = resetKey
	}

	resp := struct {
		APIKey                 *dto.APIKey `json:"api_key"`
		AutoGrantedGroupAccess bool        `json:"auto_granted_group_access"`
		GrantedGroupID         *int64      `json:"granted_group_id,omitempty"`
		GrantedGroupName       string      `json:"granted_group_name,omitempty"`
	}{
		APIKey:                 dto.APIKeyFromService(result.APIKey),
		AutoGrantedGroupAccess: result.AutoGrantedGroupAccess,
		GrantedGroupID:         result.GrantedGroupID,
		GrantedGroupName:       result.GrantedGroupName,
	}
	response.Success(c, resp)
}

// AdminCreateUserAPIKeyRequest represents the request for an admin to create an
// API key on behalf of a specific user. It mirrors the self-service
// CreateAPIKeyRequest; the two cannot be shared because the handler/admin
// package cannot import handler (import cycle). Numeric/expiry limits are
// validated once, in service.APIKeyService.Create.
type AdminCreateUserAPIKeyRequest struct {
	Name          string   `json:"name" binding:"required"`
	GroupID       *int64   `json:"group_id"`        // 可选，绑定分组
	CustomKey     *string  `json:"custom_key"`      // 可选，自定义 key
	IPWhitelist   []string `json:"ip_whitelist"`    // IP 白名单
	IPBlacklist   []string `json:"ip_blacklist"`    // IP 黑名单
	Quota         *float64 `json:"quota"`           // 配额限制 (USD)
	ExpiresInDays *int     `json:"expires_in_days"` // 过期天数

	// Rate limit fields (0 = unlimited)
	RateLimit5h *float64 `json:"rate_limit_5h"`
	RateLimit1d *float64 `json:"rate_limit_1d"`
	RateLimit7d *float64 `json:"rate_limit_7d"`
}

// CreateForUser lets an admin create an API key on behalf of a target user.
// POST /api/v1/admin/users/:id/api-keys
//
// The key is created under the target user's ownership and is subject to the
// same group-binding and limit validation as a user self-service creation
// (performed inside service.APIKeyService.Create). The plaintext key is
// returned once in the response. Wrapped in the admin idempotency helper so a
// double-submit or client retry does not create duplicate keys.
func (h *AdminAPIKeyHandler) CreateForUser(c *gin.Context) {
	userID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "Invalid user ID")
		return
	}

	// 记录代建对象，便于审计 "谁给谁建了 key"（URL 里只有 :id 模板，不落实际值）。
	middleware2.SetAuditExtra(c, map[string]any{"target_user_id": userID})

	// 目标用户必须存在且处于活跃状态。给已禁用/封禁用户建 key 只会产出一个
	// 运行时鉴权必然拒绝的死凭证，徒增困惑与审计噪音，这里提前拦截并给出明确错误。
	targetUser, err := h.adminService.GetUser(c.Request.Context(), userID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	if !targetUser.IsActive() {
		response.BadRequest(c, "target user is not active")
		return
	}

	var req AdminCreateUserAPIKeyRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}

	svcReq := service.CreateAPIKeyRequest{
		Name:          req.Name,
		GroupID:       req.GroupID,
		CustomKey:     req.CustomKey,
		IPWhitelist:   req.IPWhitelist,
		IPBlacklist:   req.IPBlacklist,
		ExpiresInDays: req.ExpiresInDays,
	}
	if req.Quota != nil {
		svcReq.Quota = *req.Quota
	}
	if req.RateLimit5h != nil {
		svcReq.RateLimit5h = *req.RateLimit5h
	}
	if req.RateLimit1d != nil {
		svcReq.RateLimit1d = *req.RateLimit1d
	}
	if req.RateLimit7d != nil {
		svcReq.RateLimit7d = *req.RateLimit7d
	}

	// 幂等指纹必须包含目标 userID：Route 只是模板(":id")、payload 仅是请求体，
	// 若不并入 userID，同一管理员用同一 Idempotency-Key 对不同用户发相同 body
	// 会被判为 replay，把前一个用户的明文 key 回放给后一个用户。
	idemPayload := struct {
		UserID int64                        `json:"user_id"`
		Req    AdminCreateUserAPIKeyRequest `json:"req"`
	}{UserID: userID, Req: req}

	executeAdminIdempotentJSON(c, "admin.users.api_keys.create", idemPayload, service.DefaultWriteIdempotencyTTL(), func(ctx context.Context) (any, error) {
		key, err := h.apiKeyService.Create(ctx, userID, svcReq)
		if err != nil {
			return nil, err
		}
		return dto.APIKeyFromService(key), nil
	})
}

// GetUserAvailableGroups returns the groups the target user is actually allowed
// to bind an API key to (standard groups in their allowed set + subscription
// groups with an active subscription). The admin create-key form uses this to
// avoid offering groups that would be rejected by APIKeyService.Create.
// GET /api/v1/admin/users/:id/available-groups
func (h *AdminAPIKeyHandler) GetUserAvailableGroups(c *gin.Context) {
	userID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "Invalid user ID")
		return
	}

	groups, err := h.apiKeyService.GetAvailableGroups(c.Request.Context(), userID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}

	out := make([]dto.Group, 0, len(groups))
	for i := range groups {
		out = append(out, *dto.GroupFromService(&groups[i]))
	}
	response.Success(c, out)
}
