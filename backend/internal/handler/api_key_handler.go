// Package handler provides HTTP request handlers for the application.
package handler

import (
	"context"
	"errors"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/handler/dto"
	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	middleware2 "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/gin-gonic/gin"
)

// APIKeyHandler handles API key-related requests
type APIKeyHandler struct {
	apiKeyService *service.APIKeyService
}

// NewAPIKeyHandler creates a new APIKeyHandler
func NewAPIKeyHandler(apiKeyService *service.APIKeyService) *APIKeyHandler {
	return &APIKeyHandler{
		apiKeyService: apiKeyService,
	}
}

// CreateAPIKeyRequest represents the create API key request payload
type CreateAPIKeyRequest struct {
	ConcurrencyLimit int      `json:"concurrency_limit" binding:"gte=0"`
	Name             string   `json:"name" binding:"required"`
	GroupID          *int64   `json:"group_id"`        // nullable
	CustomKey        *string  `json:"custom_key"`      // 可选的自定义key
	IPWhitelist      []string `json:"ip_whitelist"`    // IP 白名单
	IPBlacklist      []string `json:"ip_blacklist"`    // IP 黑名单
	Quota            *float64 `json:"quota"`           // 配额限制 (USD)
	ExpiresInDays    *int     `json:"expires_in_days"` // 过期天数

	// Rate limit fields (0 = unlimited)
	RateLimit5h *float64 `json:"rate_limit_5h"`
	RateLimit1d *float64 `json:"rate_limit_1d"`
	RateLimit7d *float64 `json:"rate_limit_7d"`
}

// UpdateAPIKeyRequest represents the update API key request payload
type UpdateAPIKeyRequest struct {
	ConcurrencyLimit *int      `json:"concurrency_limit" binding:"omitempty,gte=0"`
	Name             string    `json:"name"`
	GroupID          *int64    `json:"group_id"`
	Status           string    `json:"status" binding:"omitempty,oneof=active inactive"`
	IPWhitelist      *[]string `json:"ip_whitelist"` // IP 白名单（nil 不修改，空数组清空）
	IPBlacklist      *[]string `json:"ip_blacklist"` // IP 黑名单（nil 不修改，空数组清空）
	Quota            *float64  `json:"quota"`        // 配额限制 (USD), 0=无限制
	ExpiresAt        *string   `json:"expires_at"`   // 过期时间 (ISO 8601)
	ResetQuota       *bool     `json:"reset_quota"`  // 重置已用配额

	// Rate limit fields (nil = no change, 0 = unlimited)
	RateLimit5h         *float64 `json:"rate_limit_5h"`
	RateLimit1d         *float64 `json:"rate_limit_1d"`
	RateLimit7d         *float64 `json:"rate_limit_7d"`
	ResetRateLimitUsage *bool    `json:"reset_rate_limit_usage"` // 重置限速用量
}

func validAPIKeyLimit(v float64) bool { return !math.IsNaN(v) && !math.IsInf(v, 0) && v >= 0 }

func validateAPIKeyCreateRequest(req CreateAPIKeyRequest) error {
	if req.Quota != nil && !validAPIKeyLimit(*req.Quota) {
		return errors.New("invalid quota")
	}
	if req.RateLimit5h != nil && !validAPIKeyLimit(*req.RateLimit5h) {
		return errors.New("invalid rate_limit_5h")
	}
	if req.RateLimit1d != nil && !validAPIKeyLimit(*req.RateLimit1d) {
		return errors.New("invalid rate_limit_1d")
	}
	if req.RateLimit7d != nil && !validAPIKeyLimit(*req.RateLimit7d) {
		return errors.New("invalid rate_limit_7d")
	}
	if req.ExpiresInDays != nil && *req.ExpiresInDays <= 0 {
		return errors.New("invalid expires_in_days")
	}
	return nil
}

func validateAPIKeyUpdateRequest(req UpdateAPIKeyRequest) error {
	if req.Quota != nil && !validAPIKeyLimit(*req.Quota) {
		return errors.New("invalid quota")
	}
	if req.RateLimit5h != nil && !validAPIKeyLimit(*req.RateLimit5h) {
		return errors.New("invalid rate_limit_5h")
	}
	if req.RateLimit1d != nil && !validAPIKeyLimit(*req.RateLimit1d) {
		return errors.New("invalid rate_limit_1d")
	}
	if req.RateLimit7d != nil && !validAPIKeyLimit(*req.RateLimit7d) {
		return errors.New("invalid rate_limit_7d")
	}
	return nil
}

// List handles listing user's API keys with pagination
// GET /api/v1/api-keys
func (h *APIKeyHandler) List(c *gin.Context) {
	subject, ok := middleware2.GetAuthSubjectFromContext(c)
	if !ok {
		response.Unauthorized(c, "User not authenticated")
		return
	}

	page, pageSize := response.ParsePagination(c)
	params := pagination.PaginationParams{
		Page:      page,
		PageSize:  pageSize,
		SortBy:    c.DefaultQuery("sort_by", "created_at"),
		SortOrder: c.DefaultQuery("sort_order", "desc"),
	}

	// Parse filter parameters
	var filters service.APIKeyListFilters
	if search := strings.TrimSpace(c.Query("search")); search != "" {
		if len(search) > 100 {
			search = search[:100]
		}
		filters.Search = search
	}
	filters.Status = c.Query("status")
	if groupIDStr := c.Query("group_id"); groupIDStr != "" {
		gid, err := strconv.ParseInt(groupIDStr, 10, 64)
		if err == nil {
			filters.GroupID = &gid
		}
	}

	keys, result, err := h.apiKeyService.List(c.Request.Context(), subject.UserID, params, filters)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}

	out := make([]dto.APIKey, 0, len(keys))
	for i := range keys {
		out = append(out, *dto.APIKeyFromService(&keys[i]))
	}
	response.Paginated(c, out, result.Total, page, pageSize)
}

// GetByID handles getting a single API key
// GET /api/v1/api-keys/:id
func (h *APIKeyHandler) GetByID(c *gin.Context) {
	subject, ok := middleware2.GetAuthSubjectFromContext(c)
	if !ok {
		response.Unauthorized(c, "User not authenticated")
		return
	}

	keyID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "Invalid key ID")
		return
	}

	key, err := h.apiKeyService.GetByID(c.Request.Context(), keyID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}

	// 验证所有权
	if key.UserID != subject.UserID {
		response.NotFound(c, "API key not found")
		return
	}

	response.Success(c, dto.APIKeyFromService(key))
}

// Create handles creating a new API key
// POST /api/v1/api-keys
func (h *APIKeyHandler) Create(c *gin.Context) {
	subject, ok := middleware2.GetAuthSubjectFromContext(c)
	if !ok {
		response.Unauthorized(c, "User not authenticated")
		return
	}

	var req CreateAPIKeyRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}
	if err := validateAPIKeyCreateRequest(req); err != nil {
		response.BadRequest(c, "Invalid request: numeric limits must be finite and non-negative, and expires_in_days must be greater than zero")
		return
	}

	svcReq := service.CreateAPIKeyRequest{
		ConcurrencyLimit: req.ConcurrencyLimit,
		Name:             req.Name,
		GroupID:          req.GroupID,
		CustomKey:        req.CustomKey,
		IPWhitelist:      req.IPWhitelist,
		IPBlacklist:      req.IPBlacklist,
		ExpiresInDays:    req.ExpiresInDays,
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

	executeUserIdempotentJSON(c, "user.api_keys.create", req, service.DefaultWriteIdempotencyTTL(), func(ctx context.Context) (any, error) {
		key, err := h.apiKeyService.Create(ctx, subject.UserID, svcReq)
		if err != nil {
			return nil, err
		}
		return dto.APIKeyFromService(key), nil
	})
}

// Update handles updating an API key
// PUT /api/v1/api-keys/:id
func (h *APIKeyHandler) Update(c *gin.Context) {
	subject, ok := middleware2.GetAuthSubjectFromContext(c)
	if !ok {
		response.Unauthorized(c, "User not authenticated")
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
	if err := validateAPIKeyUpdateRequest(req); err != nil {
		response.BadRequest(c, "Invalid request: numeric limits must be finite and non-negative")
		return
	}

	svcReq := service.UpdateAPIKeyRequest{
		ConcurrencyLimit:    req.ConcurrencyLimit,
		IPWhitelist:         req.IPWhitelist,
		IPBlacklist:         req.IPBlacklist,
		Quota:               req.Quota,
		ResetQuota:          req.ResetQuota,
		RateLimit5h:         req.RateLimit5h,
		RateLimit1d:         req.RateLimit1d,
		RateLimit7d:         req.RateLimit7d,
		ResetRateLimitUsage: req.ResetRateLimitUsage,
	}
	if req.Name != "" {
		svcReq.Name = &req.Name
	}
	svcReq.GroupID = req.GroupID
	if req.Status != "" {
		svcReq.Status = &req.Status
	}
	// Parse expires_at if provided
	if req.ExpiresAt != nil {
		if *req.ExpiresAt == "" {
			// Empty string means clear expiration
			svcReq.ExpiresAt = nil
			svcReq.ClearExpiration = true
		} else {
			t, err := time.Parse(time.RFC3339, *req.ExpiresAt)
			if err != nil {
				response.BadRequest(c, "Invalid expires_at format: "+err.Error())
				return
			}
			svcReq.ExpiresAt = &t
		}
	}

	key, err := h.apiKeyService.Update(c.Request.Context(), keyID, subject.UserID, svcReq)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}

	response.Success(c, dto.APIKeyFromService(key))
}

// Delete handles deleting an API key
// DELETE /api/v1/api-keys/:id
func (h *APIKeyHandler) Delete(c *gin.Context) {
	subject, ok := middleware2.GetAuthSubjectFromContext(c)
	if !ok {
		response.Unauthorized(c, "User not authenticated")
		return
	}

	keyID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "Invalid key ID")
		return
	}

	err = h.apiKeyService.Delete(c.Request.Context(), keyID, subject.UserID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}

	response.Success(c, gin.H{"message": "API key deleted successfully"})
}

// GetAvailableGroups 获取用户可以绑定的分组列表
// GET /api/v1/groups/available
func (h *APIKeyHandler) GetAvailableGroups(c *gin.Context) {
	subject, ok := middleware2.GetAuthSubjectFromContext(c)
	if !ok {
		response.Unauthorized(c, "User not authenticated")
		return
	}

	groups, err := h.apiKeyService.GetAvailableGroups(c.Request.Context(), subject.UserID)
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

const (
	apiKeyQueueStatsMaxIDs = 100
)

type apiKeyQueuePolicyDTO struct {
	MaxWaiting     int `json:"max_waiting"`
	TimeoutSeconds int `json:"timeout_seconds"`
}

type apiKeyQueueItemDTO struct {
	ID                 int64 `json:"id"`
	CurrentConcurrency int   `json:"current_concurrency"`
	CurrentWaiting     int   `json:"current_waiting"`
}

type apiKeyQueueStatsResponse struct {
	QueuePolicy apiKeyQueuePolicyDTO `json:"queue_policy"`
	Items       []apiKeyQueueItemDTO `json:"items"`
}

// parseAPIKeyQueueStatsIDs 校验正整数、去重并限制数量，顺序保持请求顺序。
func parseAPIKeyQueueStatsIDs(raw string) ([]int64, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return []int64{}, nil
	}
	seen := make(map[int64]struct{})
	ids := make([]int64, 0, 8)
	for _, part := range strings.Split(raw, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		id, err := strconv.ParseInt(part, 10, 64)
		if err != nil || id <= 0 {
			return nil, errors.New("ids must be positive integers")
		}
		if _, ok := seen[id]; ok {
			continue
		}
		if len(ids) >= apiKeyQueueStatsMaxIDs {
			return nil, errors.New("too many ids")
		}
		seen[id] = struct{}{}
		ids = append(ids, id)
	}
	return ids, nil
}

// GetConcurrencyQueue 只读返回本进程的 Key 队列策略与指定 Key 的并发/等待统计。
// GET /api/v1/keys/concurrency?ids=1,2
func (h *APIKeyHandler) GetConcurrencyQueue(c *gin.Context) {
	subject, ok := middleware2.GetAuthSubjectFromContext(c)
	if !ok {
		response.Unauthorized(c, "User not authenticated")
		return
	}

	ids, err := parseAPIKeyQueueStatsIDs(c.Query("ids"))
	if err != nil {
		response.BadRequest(c, err.Error())
		return
	}

	policy := h.apiKeyService.APIKeyQueuePolicy()
	out := apiKeyQueueStatsResponse{
		QueuePolicy: apiKeyQueuePolicyDTO{
			MaxWaiting:     policy.MaxWaiting,
			TimeoutSeconds: int(policy.Timeout / time.Second),
		},
		Items: []apiKeyQueueItemDTO{},
	}
	if len(ids) == 0 {
		response.Success(c, out)
		return
	}

	owned, err := h.apiKeyService.VerifyOwnership(c.Request.Context(), subject.UserID, ids)
	if err != nil {
		response.InternalError(c, "Failed to verify API key ownership")
		return
	}
	ownedSet := make(map[int64]struct{}, len(owned))
	for _, id := range owned {
		ownedSet[id] = struct{}{}
	}
	for _, id := range ids {
		if _, ok := ownedSet[id]; !ok {
			// Uniform error; never disclose whether the key exists for another user.
			response.Error(c, http.StatusNotFound, "API key not found")
			return
		}
	}

	stats, err := h.apiKeyService.GetAPIKeyQueueStatsBatch(c.Request.Context(), ids)
	if err != nil {
		response.Error(c, http.StatusServiceUnavailable, "API key queue statistics unavailable")
		return
	}
	for _, id := range ids {
		counts := stats[id]
		out.Items = append(out.Items, apiKeyQueueItemDTO{
			ID:                 id,
			CurrentConcurrency: counts.Active,
			CurrentWaiting:     counts.Waiting,
		})
	}
	response.Success(c, out)
}

// GetUserGroupRates 获取当前用户的专属分组倍率配置
// GET /api/v1/groups/rates
func (h *APIKeyHandler) GetUserGroupRates(c *gin.Context) {
	subject, ok := middleware2.GetAuthSubjectFromContext(c)
	if !ok {
		response.Unauthorized(c, "User not authenticated")
		return
	}

	rates, err := h.apiKeyService.GetUserGroupRates(c.Request.Context(), subject.UserID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}

	response.Success(c, rates)
}
