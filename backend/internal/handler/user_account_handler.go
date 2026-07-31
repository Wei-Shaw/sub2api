package handler

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/handler/dto"
	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	middleware2 "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/gin-gonic/gin"
)

// UserAccountHandler 用户自建上游账号 HTTP 接口。
type UserAccountHandler struct {
	svc service.UserAccountService
}

// NewUserAccountHandler 创建 handler。
func NewUserAccountHandler(svc service.UserAccountService) *UserAccountHandler {
	return &UserAccountHandler{svc: svc}
}

// CreateUserAccountRequest POST /api/v1/user/accounts
type CreateUserAccountRequest struct {
	Name        string         `json:"name" binding:"required"`
	Platform    string         `json:"platform" binding:"required"`
	Type        string         `json:"type" binding:"required"`
	Credentials map[string]any `json:"credentials"`
	Extra       map[string]any `json:"extra"`
	Visibility  string         `json:"visibility"` // private|public；默认 private
	// Concurrency 账号并发上限；0 或缺省时由服务层按平台默认规范化
	Concurrency int `json:"concurrency"`
}

// UpdateUserAccountRequest PATCH /api/v1/user/accounts/:id（allowlist）
// 不绑定 group_ids / proxy_id，客户端误传将被忽略。
type UpdateUserAccountRequest struct {
	Name           *string        `json:"name"`
	Notes          *string        `json:"notes"`
	Credentials    map[string]any `json:"credentials"`
	Status         *string        `json:"status"` // active|inactive|disabled
	Concurrency    *int           `json:"concurrency"`
	Schedulable    *bool          `json:"schedulable"`
	RateMultiplier *float64       `json:"rate_multiplier"`
	Extra          map[string]any `json:"extra"`
}

// SetVisibilityRequest PUT /api/v1/user/accounts/:id/visibility
type SetVisibilityRequest struct {
	Visibility string `json:"visibility" binding:"required"`
}

// SetUserAccountSchedulableRequest PUT /api/v1/user/accounts/:id/schedulable
type SetUserAccountSchedulableRequest struct {
	Schedulable bool `json:"schedulable"`
}

// BatchDeleteUserAccountsRequest POST /api/v1/user/accounts/batch-delete
type BatchDeleteUserAccountsRequest struct {
	IDs []int64 `json:"ids" binding:"required,min=1"`
}

// BatchSetUserAccountSchedulableRequest POST /api/v1/user/accounts/batch-set-schedulable
type BatchSetUserAccountSchedulableRequest struct {
	IDs         []int64 `json:"ids" binding:"required,min=1"`
	Schedulable bool    `json:"schedulable"`
}

// List GET /api/v1/user/accounts
func (h *UserAccountHandler) List(c *gin.Context) {
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
	accounts, total, err := h.svc.List(c.Request.Context(), subject.UserID, params)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	out := make([]dto.Account, 0, len(accounts))
	for i := range accounts {
		if mapped := dto.AccountFromService(&accounts[i]); mapped != nil {
			out = append(out, *mapped)
		}
	}
	response.Paginated(c, out, total, page, pageSize)
}

// Get GET /api/v1/user/accounts/:id
func (h *UserAccountHandler) Get(c *gin.Context) {
	subject, ok := middleware2.GetAuthSubjectFromContext(c)
	if !ok {
		response.Unauthorized(c, "User not authenticated")
		return
	}
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		response.BadRequest(c, "Invalid account ID")
		return
	}
	account, err := h.svc.Get(c.Request.Context(), subject.UserID, id)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, dto.AccountFromService(account))
}

// Create POST /api/v1/user/accounts
func (h *UserAccountHandler) Create(c *gin.Context) {
	subject, ok := middleware2.GetAuthSubjectFromContext(c)
	if !ok {
		response.Unauthorized(c, "User not authenticated")
		return
	}
	var req CreateUserAccountRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request body")
		return
	}
	account, err := h.svc.Create(c.Request.Context(), subject.UserID, &service.CreateUserAccountInput{
		Name:        req.Name,
		Platform:    req.Platform,
		Type:        req.Type,
		Credentials: req.Credentials,
		Extra:       req.Extra,
		Visibility:  req.Visibility,
		Concurrency: req.Concurrency,
	})
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Created(c, dto.AccountFromService(account))
}

// Update PATCH /api/v1/user/accounts/:id
func (h *UserAccountHandler) Update(c *gin.Context) {
	subject, ok := middleware2.GetAuthSubjectFromContext(c)
	if !ok {
		response.Unauthorized(c, "User not authenticated")
		return
	}
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		response.BadRequest(c, "Invalid account ID")
		return
	}
	var req UpdateUserAccountRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request body")
		return
	}
	// 显式拒绝越权字段：struct 不绑定 group_ids / proxy_id
	account, err := h.svc.Update(c.Request.Context(), subject.UserID, id, &service.UpdateUserAccountInput{
		Name:           req.Name,
		Notes:          req.Notes,
		Credentials:    req.Credentials,
		Status:         req.Status,
		Concurrency:    req.Concurrency,
		Schedulable:    req.Schedulable,
		RateMultiplier: req.RateMultiplier,
		Extra:          req.Extra,
	})
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, dto.AccountFromService(account))
}

// SetSchedulable PUT /api/v1/user/accounts/:id/schedulable
func (h *UserAccountHandler) SetSchedulable(c *gin.Context) {
	subject, ok := middleware2.GetAuthSubjectFromContext(c)
	if !ok {
		response.Unauthorized(c, "User not authenticated")
		return
	}
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		response.BadRequest(c, "Invalid account ID")
		return
	}
	var req SetUserAccountSchedulableRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request body")
		return
	}
	account, err := h.svc.SetSchedulable(c.Request.Context(), subject.UserID, id, req.Schedulable)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, dto.AccountFromService(account))
}

// BatchDelete POST /api/v1/user/accounts/batch-delete
func (h *UserAccountHandler) BatchDelete(c *gin.Context) {
	subject, ok := middleware2.GetAuthSubjectFromContext(c)
	if !ok {
		response.Unauthorized(c, "User not authenticated")
		return
	}
	var req BatchDeleteUserAccountsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request body")
		return
	}
	result, err := h.svc.BatchDeleteOwned(c.Request.Context(), subject.UserID, req.IDs)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, result)
}

// BatchSetSchedulable POST /api/v1/user/accounts/batch-set-schedulable
func (h *UserAccountHandler) BatchSetSchedulable(c *gin.Context) {
	subject, ok := middleware2.GetAuthSubjectFromContext(c)
	if !ok {
		response.Unauthorized(c, "User not authenticated")
		return
	}
	var req BatchSetUserAccountSchedulableRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request body")
		return
	}
	result, err := h.svc.BatchSetSchedulableOwned(c.Request.Context(), subject.UserID, req.IDs, req.Schedulable)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, result)
}

// ExportData GET /api/v1/user/accounts/data?ids=1,2,3
// 仅导出本人账号；不含代理。
func (h *UserAccountHandler) ExportData(c *gin.Context) {
	subject, ok := middleware2.GetAuthSubjectFromContext(c)
	if !ok {
		response.Unauthorized(c, "User not authenticated")
		return
	}
	ids, err := parsePositiveIDList(c.Query("ids"))
	if err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	payload, err := h.svc.ExportOwned(c.Request.Context(), subject.UserID, ids)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, payload)
}

// ImportDataRequest POST /api/v1/user/accounts/data
type ImportDataRequest struct {
	Data service.UserAccountDataPayload `json:"data"`
}

// ImportData POST /api/v1/user/accounts/data
// 导入为本人自建账号（忽略 proxies；分组由 Ensure 私有组自动处理）。
func (h *UserAccountHandler) ImportData(c *gin.Context) {
	subject, ok := middleware2.GetAuthSubjectFromContext(c)
	if !ok {
		response.Unauthorized(c, "User not authenticated")
		return
	}
	var req ImportDataRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request body")
		return
	}
	result, err := h.svc.ImportOwned(c.Request.Context(), subject.UserID, &req.Data)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, result)
}

func parsePositiveIDList(raw string) ([]int64, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}
	parts := strings.Split(raw, ",")
	out := make([]int64, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		id, err := strconv.ParseInt(p, 10, 64)
		if err != nil || id <= 0 {
			return nil, fmt.Errorf("invalid id %q", p)
		}
		out = append(out, id)
	}
	return out, nil
}

// SetVisibility PUT /api/v1/user/accounts/:id/visibility
func (h *UserAccountHandler) SetVisibility(c *gin.Context) {
	subject, ok := middleware2.GetAuthSubjectFromContext(c)
	if !ok {
		response.Unauthorized(c, "User not authenticated")
		return
	}
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		response.BadRequest(c, "Invalid account ID")
		return
	}
	var req SetVisibilityRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request body")
		return
	}
	account, err := h.svc.SetVisibility(c.Request.Context(), subject.UserID, id, strings.TrimSpace(req.Visibility))
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, dto.AccountFromService(account))
}

// Delete DELETE /api/v1/user/accounts/:id
func (h *UserAccountHandler) Delete(c *gin.Context) {
	subject, ok := middleware2.GetAuthSubjectFromContext(c)
	if !ok {
		response.Unauthorized(c, "User not authenticated")
		return
	}
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		response.BadRequest(c, "Invalid account ID")
		return
	}
	if err := h.svc.Delete(c.Request.Context(), subject.UserID, id); err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, gin.H{"ok": true})
}
