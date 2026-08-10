package admin

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/handler/dto"
	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/gin-gonic/gin"
)

// AdminAPIKeyHandler handles admin API key management
type AdminAPIKeyHandler struct {
	adminService  service.AdminService
	apiKeyService adminAPIKeyService
}

type adminAPIKeyService interface {
	Create(context.Context, int64, service.CreateAPIKeyRequest) (*service.APIKey, error)
	List(context.Context, int64, pagination.PaginationParams, service.APIKeyListFilters) ([]service.APIKey, *pagination.PaginationResult, error)
	GetByID(context.Context, int64) (*service.APIKey, error)
	Update(context.Context, int64, int64, service.UpdateAPIKeyRequest) (*service.APIKey, error)
	Delete(context.Context, int64, int64) error
	ValidateCustomKeyPrefix(string) error
}

// NewAdminAPIKeyHandler creates a new admin API key handler
func NewAdminAPIKeyHandler(adminService service.AdminService, apiKeyService *service.APIKeyService) *AdminAPIKeyHandler {
	return &AdminAPIKeyHandler{
		adminService:  adminService,
		apiKeyService: apiKeyService,
	}
}

// AdminCreateAPIKeyRequest represents an API key created for a target user.
type AdminCreateAPIKeyRequest struct {
	Name          string   `json:"name" binding:"required"`
	GroupID       *int64   `json:"group_id"`
	CustomKey     *string  `json:"custom_key"`
	IPWhitelist   []string `json:"ip_whitelist"`
	IPBlacklist   []string `json:"ip_blacklist"`
	Quota         *float64 `json:"quota"`
	RateLimit5h   *float64 `json:"rate_limit_5h"`
	RateLimit1d   *float64 `json:"rate_limit_1d"`
	RateLimit7d   *float64 `json:"rate_limit_7d"`
	ExpiresInDays *int     `json:"expires_in_days"`
	ExpiresAt     *string  `json:"expires_at"`
}

// AdminUpdateAPIKeyRequest represents fields an administrator can update.
type AdminUpdateAPIKeyRequest struct {
	Name                *string   `json:"name"`
	GroupID             *int64    `json:"group_id"`
	CustomKey           *string   `json:"custom_key"`
	Status              *string   `json:"status" binding:"omitempty,oneof=active inactive"`
	IPWhitelist         *[]string `json:"ip_whitelist"`
	IPBlacklist         *[]string `json:"ip_blacklist"`
	Quota               *float64  `json:"quota"`
	ResetQuota          *bool     `json:"reset_quota"`
	RateLimit5h         *float64  `json:"rate_limit_5h"`
	RateLimit1d         *float64  `json:"rate_limit_1d"`
	RateLimit7d         *float64  `json:"rate_limit_7d"`
	ResetRateLimitUsage *bool     `json:"reset_rate_limit_usage"`
	ExpiresInDays       *int      `json:"expires_in_days"`
	ExpiresAt           *string   `json:"expires_at"`
}

// List handles listing API keys owned by a target user.
// GET /api/v1/admin/users/:user_id/api-keys
func (h *AdminAPIKeyHandler) List(c *gin.Context) {
	userID, ok := parseUserID(c)
	if !ok {
		return
	}
	if _, err := h.adminService.GetUser(c.Request.Context(), userID); err != nil {
		response.ErrorFrom(c, err)
		return
	}

	page, pageSize := response.ParsePagination(c)
	params := pagination.PaginationParams{
		Page:      page,
		PageSize:  pageSize,
		SortBy:    c.DefaultQuery("sort_by", "created_at"),
		SortOrder: c.DefaultQuery("sort_order", "desc"),
	}
	filters := service.APIKeyListFilters{Status: c.Query("status")}
	if search := strings.TrimSpace(c.Query("search")); search != "" {
		if len(search) > 100 {
			search = search[:100]
		}
		filters.Search = search
	}
	if groupIDValue := c.Query("group_id"); groupIDValue != "" {
		groupID, err := strconv.ParseInt(groupIDValue, 10, 64)
		if err != nil {
			response.BadRequest(c, "Invalid group ID")
			return
		}
		filters.GroupID = &groupID
	}

	keys, result, err := h.apiKeyService.List(c.Request.Context(), userID, params, filters)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	out := make([]dto.APIKey, 0, len(keys))
	for i := range keys {
		out = append(out, *adminAPIKeyDTO(&keys[i], false))
	}
	response.Paginated(c, out, result.Total, page, pageSize)
}

// Get handles getting one API key owned by a target user.
// GET /api/v1/admin/users/:user_id/api-keys/:key_id
func (h *AdminAPIKeyHandler) Get(c *gin.Context) {
	_, key, ok := h.getOwnedAPIKey(c)
	if !ok {
		return
	}
	response.Success(c, adminAPIKeyDTO(key, false))
}

// Create handles creating an API key for a target user.
// POST /api/v1/admin/users/:user_id/api-keys
func (h *AdminAPIKeyHandler) Create(c *gin.Context) {
	userID, ok := parseUserID(c)
	if !ok {
		return
	}

	var req AdminCreateAPIKeyRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}
	if req.ExpiresInDays != nil && req.ExpiresAt != nil {
		response.ErrorFrom(c, service.ErrAPIKeyExpiryConflict)
		return
	}
	if req.GroupID != nil && *req.GroupID <= 0 {
		response.BadRequest(c, "group_id must be greater than zero")
		return
	}
	if req.CustomKey != nil && *req.CustomKey != "" {
		if err := h.apiKeyService.ValidateCustomKeyPrefix(*req.CustomKey); err != nil {
			response.ErrorFrom(c, err)
			return
		}
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
	if req.ExpiresAt != nil {
		expiresAt, err := time.Parse(time.RFC3339, *req.ExpiresAt)
		if err != nil {
			response.BadRequest(c, "Invalid expires_at format: "+err.Error())
			return
		}
		svcReq.ExpiresAt = &expiresAt
	}

	payload := struct {
		UserID int64                    `json:"user_id"`
		Body   AdminCreateAPIKeyRequest `json:"body"`
	}{UserID: userID, Body: req}
	executeAdminIdempotentJSON(c, "admin.user_api_keys.create", payload, service.DefaultWriteIdempotencyTTL(), func(ctx context.Context) (any, error) {
		targetGroupID := svcReq.GroupID
		createReq := svcReq
		createReq.GroupID = nil
		key, err := h.apiKeyService.Create(ctx, userID, createReq)
		if err != nil {
			return nil, err
		}
		if targetGroupID != nil {
			result, bindErr := h.adminService.AdminUpdateAPIKeyGroupID(ctx, key.ID, targetGroupID)
			if bindErr != nil {
				if rollbackErr := h.apiKeyService.Delete(ctx, key.ID, userID); rollbackErr != nil {
					return nil, fmt.Errorf("bind api key group: %w (rollback failed: %v)", bindErr, rollbackErr)
				}
				return nil, bindErr
			}
			key = result.APIKey
		}
		return adminAPIKeyDTO(key, true), nil
	})
}

// Update handles updating an API key owned by a target user.
// PUT /api/v1/admin/users/:user_id/api-keys/:key_id
func (h *AdminAPIKeyHandler) Update(c *gin.Context) {
	userID, existingKey, ok := h.getOwnedAPIKey(c)
	if !ok {
		return
	}
	keyID, _ := strconv.ParseInt(c.Param("key_id"), 10, 64)

	var req AdminUpdateAPIKeyRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}
	if req.GroupID != nil && *req.GroupID < 0 {
		response.BadRequest(c, "group_id must be non-negative")
		return
	}
	if req.ExpiresInDays != nil && req.ExpiresAt != nil {
		response.ErrorFrom(c, service.ErrAPIKeyExpiryConflict)
		return
	}
	if req.ExpiresInDays != nil && *req.ExpiresInDays <= 0 {
		response.ErrorFrom(c, service.ErrAPIKeyInvalidExpiry)
		return
	}
	if req.CustomKey != nil {
		if *req.CustomKey == "" {
			response.BadRequest(c, "custom_key cannot be empty")
			return
		}
		if err := h.apiKeyService.ValidateCustomKeyPrefix(*req.CustomKey); err != nil {
			response.ErrorFrom(c, err)
			return
		}
	}
	clearGroup := req.GroupID != nil && *req.GroupID == 0
	if clearGroup {
		req.GroupID = nil
	}

	svcReq := service.UpdateAPIKeyRequest{
		Name:                req.Name,
		GroupID:             req.GroupID,
		CustomKey:           req.CustomKey,
		Status:              req.Status,
		IPWhitelist:         req.IPWhitelist,
		IPBlacklist:         req.IPBlacklist,
		Quota:               req.Quota,
		ResetQuota:          req.ResetQuota,
		RateLimit5h:         req.RateLimit5h,
		RateLimit1d:         req.RateLimit1d,
		RateLimit7d:         req.RateLimit7d,
		ResetRateLimitUsage: req.ResetRateLimitUsage,
	}
	svcReq.ClearGroup = clearGroup
	if req.ExpiresInDays != nil {
		expiresAt := time.Now().AddDate(0, 0, *req.ExpiresInDays)
		svcReq.ExpiresAt = &expiresAt
	}
	if req.ExpiresAt != nil {
		if *req.ExpiresAt == "" {
			svcReq.ClearExpiration = true
		} else {
			expiresAt, err := time.Parse(time.RFC3339, *req.ExpiresAt)
			if err != nil {
				response.BadRequest(c, "Invalid expires_at format: "+err.Error())
				return
			}
			svcReq.ExpiresAt = &expiresAt
		}
	}

	key, err := h.apiKeyService.Update(c.Request.Context(), keyID, userID, svcReq)
	if errors.Is(err, service.ErrGroupNotAllowed) && svcReq.GroupID != nil {
		targetGroupID := svcReq.GroupID
		if _, bindErr := h.adminService.AdminUpdateAPIKeyGroupID(c.Request.Context(), keyID, targetGroupID); bindErr != nil {
			response.ErrorFrom(c, bindErr)
			return
		}
		svcReq.GroupID = nil
		key, err = h.apiKeyService.Update(c.Request.Context(), keyID, userID, svcReq)
		if err != nil {
			rollbackGroupID := existingKey.GroupID
			if rollbackGroupID == nil {
				ungrouped := int64(0)
				rollbackGroupID = &ungrouped
			}
			_, _ = h.adminService.AdminUpdateAPIKeyGroupID(c.Request.Context(), keyID, rollbackGroupID)
		}
	}
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, adminAPIKeyDTO(key, false))
}

// Delete handles deleting an API key owned by a target user.
// DELETE /api/v1/admin/users/:user_id/api-keys/:key_id
func (h *AdminAPIKeyHandler) Delete(c *gin.Context) {
	userID, _, ok := h.getOwnedAPIKey(c)
	if !ok {
		return
	}
	keyID, _ := strconv.ParseInt(c.Param("key_id"), 10, 64)
	if err := h.apiKeyService.Delete(c.Request.Context(), keyID, userID); err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, gin.H{"message": "API key deleted successfully"})
}

func (h *AdminAPIKeyHandler) getOwnedAPIKey(c *gin.Context) (int64, *service.APIKey, bool) {
	userID, ok := parseUserID(c)
	if !ok {
		return 0, nil, false
	}
	keyID, ok := parsePositiveID(c, "key_id", "API key")
	if !ok {
		return 0, nil, false
	}
	key, err := h.apiKeyService.GetByID(c.Request.Context(), keyID)
	if err != nil {
		response.ErrorFrom(c, err)
		return 0, nil, false
	}
	if key.UserID != userID {
		response.NotFound(c, "API key not found")
		return 0, nil, false
	}
	return userID, key, true
}

func parsePositiveID(c *gin.Context, param, resource string) (int64, bool) {
	id, err := strconv.ParseInt(c.Param(param), 10, 64)
	if err != nil || id <= 0 {
		response.BadRequest(c, "Invalid "+resource+" ID")
		return 0, false
	}
	return id, true
}

func parseUserID(c *gin.Context) (int64, bool) {
	param := "user_id"
	if c.Param(param) == "" {
		param = "id"
	}
	return parsePositiveID(c, param, "user")
}

func adminAPIKeyDTO(key *service.APIKey, includeCredential bool) *dto.APIKey {
	out := dto.APIKeyFromService(key)
	if out != nil && !includeCredential {
		out.Key = service.MaskAuditCredential(out.Key)
	}
	return out
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
		APIKey:                 adminAPIKeyDTO(result.APIKey, false),
		AutoGrantedGroupAccess: result.AutoGrantedGroupAccess,
		GrantedGroupID:         result.GrantedGroupID,
		GrantedGroupName:       result.GrantedGroupName,
	}
	response.Success(c, resp)
}
