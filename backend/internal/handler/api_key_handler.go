package handler

import (
	"strconv"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/model"
	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/gin-gonic/gin"
)

// APIKeyHandler handles API key-related requests
type APIKeyHandler struct {
	apiKeyService *service.ApiKeyService
}

// NewAPIKeyHandler creates a new APIKeyHandler
func NewAPIKeyHandler(apiKeyService *service.ApiKeyService) *APIKeyHandler {
	return &APIKeyHandler{
		apiKeyService: apiKeyService,
	}
}

// CreateAPIKeyRequest represents the create API key request payload
type CreateAPIKeyRequest struct {
	Name      string  `json:"name" binding:"required"`
	GroupID   *int64  `json:"group_id"`   // nullable
	CustomKey *string `json:"custom_key"` // 可选的自定义key
}

// UpdateAPIKeyRequest represents the update API key request payload
type UpdateAPIKeyRequest struct {
	Name    string `json:"name"`
	GroupID *int64 `json:"group_id"`
	Status  string `json:"status" binding:"omitempty,oneof=active inactive"`
}

// ApiKeyResponse API key 响应载体：
// - key 字段仅在创建时返回明文
// - masked_key 用于后续列表/详情展示
type ApiKeyResponse struct {
	ID        int64        `json:"id"`
	UserID    int64        `json:"user_id"`
	Key       string       `json:"key,omitempty"`
	MaskedKey string       `json:"masked_key"`
	Name      string       `json:"name"`
	GroupID   *int64       `json:"group_id"`
	Status    string       `json:"status"`
	CreatedAt time.Time    `json:"created_at"`
	UpdatedAt time.Time    `json:"updated_at"`
	Group     *model.Group `json:"group,omitempty"`
}

// List handles listing user's API keys with pagination
// GET /api/v1/api-keys
func (h *APIKeyHandler) List(c *gin.Context) {
	userValue, exists := c.Get("user")
	if !exists {
		response.Unauthorized(c, "User not authenticated")
		return
	}

	user, ok := userValue.(*model.User)
	if !ok {
		response.InternalError(c, "Invalid user context")
		return
	}

	page, pageSize := response.ParsePagination(c)
	params := pagination.PaginationParams{Page: page, PageSize: pageSize}

	keys, result, err := h.apiKeyService.List(c.Request.Context(), user.ID, params)
	if err != nil {
		response.InternalError(c, "Failed to list API keys: "+err.Error())
		return
	}

	items := make([]ApiKeyResponse, len(keys))
	for i := range keys {
		items[i] = h.buildApiKeyResponse(&keys[i], "")
	}
	response.Paginated(c, items, result.Total, page, pageSize)
}

// GetByID handles getting a single API key
// GET /api/v1/api-keys/:id
func (h *APIKeyHandler) GetByID(c *gin.Context) {
	userValue, exists := c.Get("user")
	if !exists {
		response.Unauthorized(c, "User not authenticated")
		return
	}

	user, ok := userValue.(*model.User)
	if !ok {
		response.InternalError(c, "Invalid user context")
		return
	}

	keyID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "Invalid key ID")
		return
	}

	key, err := h.apiKeyService.GetByID(c.Request.Context(), keyID)
	if err != nil {
		response.NotFound(c, "API key not found")
		return
	}

	// 验证所有权
	if key.UserID != user.ID {
		response.Forbidden(c, "Not authorized to access this key")
		return
	}

	response.Success(c, h.buildApiKeyResponse(key, ""))
}

// Create handles creating a new API key
// POST /api/v1/api-keys
func (h *APIKeyHandler) Create(c *gin.Context) {
	userValue, exists := c.Get("user")
	if !exists {
		response.Unauthorized(c, "User not authenticated")
		return
	}

	user, ok := userValue.(*model.User)
	if !ok {
		response.InternalError(c, "Invalid user context")
		return
	}

	var req CreateAPIKeyRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}

	svcReq := service.CreateApiKeyRequest{
		Name:      req.Name,
		GroupID:   req.GroupID,
		CustomKey: req.CustomKey,
	}
	key, rawKey, err := h.apiKeyService.Create(c.Request.Context(), user.ID, svcReq)
	if err != nil {
		response.InternalError(c, "Failed to create API key: "+err.Error())
		return
	}

	// 仅创建时返回原始 key，其他接口统一返回脱敏值。
	response.Success(c, h.buildApiKeyResponse(key, rawKey))
}

// Update handles updating an API key
// PUT /api/v1/api-keys/:id
func (h *APIKeyHandler) Update(c *gin.Context) {
	userValue, exists := c.Get("user")
	if !exists {
		response.Unauthorized(c, "User not authenticated")
		return
	}

	user, ok := userValue.(*model.User)
	if !ok {
		response.InternalError(c, "Invalid user context")
		return
	}

	keyID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "Invalid key ID")
		return
	}

	var req UpdateAPIKeyRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}

	svcReq := service.UpdateApiKeyRequest{}
	if req.Name != "" {
		svcReq.Name = &req.Name
	}
	svcReq.GroupID = req.GroupID
	if req.Status != "" {
		svcReq.Status = &req.Status
	}

	key, err := h.apiKeyService.Update(c.Request.Context(), keyID, user.ID, svcReq)
	if err != nil {
		response.InternalError(c, "Failed to update API key: "+err.Error())
		return
	}

	response.Success(c, h.buildApiKeyResponse(key, ""))
}

// Delete handles deleting an API key
// DELETE /api/v1/api-keys/:id
func (h *APIKeyHandler) Delete(c *gin.Context) {
	userValue, exists := c.Get("user")
	if !exists {
		response.Unauthorized(c, "User not authenticated")
		return
	}

	user, ok := userValue.(*model.User)
	if !ok {
		response.InternalError(c, "Invalid user context")
		return
	}

	keyID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "Invalid key ID")
		return
	}

	err = h.apiKeyService.Delete(c.Request.Context(), keyID, user.ID)
	if err != nil {
		response.InternalError(c, "Failed to delete API key: "+err.Error())
		return
	}

	response.Success(c, gin.H{"message": "API key deleted successfully"})
}

// GetAvailableGroups 获取用户可以绑定的分组列表
// GET /api/v1/groups/available
func (h *APIKeyHandler) GetAvailableGroups(c *gin.Context) {
	userValue, exists := c.Get("user")
	if !exists {
		response.Unauthorized(c, "User not authenticated")
		return
	}

	user, ok := userValue.(*model.User)
	if !ok {
		response.InternalError(c, "Invalid user context")
		return
	}

	groups, err := h.apiKeyService.GetAvailableGroups(c.Request.Context(), user.ID)
	if err != nil {
		response.InternalError(c, "Failed to get available groups: "+err.Error())
		return
	}

	response.Success(c, groups)
}

// buildApiKeyResponse 统一封装返回结构，避免重复暴露明文 key。
func (h *APIKeyHandler) buildApiKeyResponse(key *model.ApiKey, rawKey string) ApiKeyResponse {
	return ApiKeyResponse{
		ID:        key.ID,
		UserID:    key.UserID,
		Key:       rawKey,
		// 前端展示统一使用 masked_key，避免暴露完整密钥。
		MaskedKey: h.apiKeyService.MaskKey(rawKey, key),
		Name:      key.Name,
		GroupID:   key.GroupID,
		Status:    key.Status,
		CreatedAt: key.CreatedAt,
		UpdatedAt: key.UpdatedAt,
		Group:     key.Group,
	}
}
