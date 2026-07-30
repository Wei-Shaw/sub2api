package handler

import (
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
	Visibility  string         `json:"visibility"` // private|public；默认 private
}

// UpdateUserAccountRequest PATCH /api/v1/user/accounts/:id（K15 allowlist）
type UpdateUserAccountRequest struct {
	Name        *string        `json:"name"`
	Notes       *string        `json:"notes"`
	Credentials map[string]any `json:"credentials"`
	Status      *string        `json:"status"` // active|disabled
}

// SetVisibilityRequest PUT /api/v1/user/accounts/:id/visibility
type SetVisibilityRequest struct {
	Visibility string `json:"visibility" binding:"required"`
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
		Visibility:  req.Visibility,
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
	// 显式拒绝越权字段：若客户端误传，body 中无对应字段即可（struct 不绑定 group_ids 等）
	account, err := h.svc.Update(c.Request.Context(), subject.UserID, id, &service.UpdateUserAccountInput{
		Name:        req.Name,
		Notes:       req.Notes,
		Credentials: req.Credentials,
		Status:      req.Status,
	})
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, dto.AccountFromService(account))
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
