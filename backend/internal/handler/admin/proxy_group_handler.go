package admin

import (
	"context"
	"strconv"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/handler/dto"
	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/gin-gonic/gin"
)

// ProxyGroupHandler 管理端代理组 CRUD。
type ProxyGroupHandler struct {
	svc *service.ProxyGroupService
}

// NewProxyGroupHandler 创建代理组 handler。
func NewProxyGroupHandler(svc *service.ProxyGroupService) *ProxyGroupHandler {
	return &ProxyGroupHandler{svc: svc}
}

type CreateProxyGroupRequest struct {
	Name            string  `json:"name" binding:"required"`
	Description     string  `json:"description"`
	Strategy        string  `json:"strategy" binding:"omitempty,oneof=round_robin random sticky"`
	StickyByAccount bool    `json:"sticky_by_account"`
	Status          string  `json:"status" binding:"omitempty,oneof=active inactive"`
	ProxyIDs        []int64 `json:"proxy_ids"`
}

type UpdateProxyGroupRequest struct {
	Name            *string  `json:"name"`
	Description     *string  `json:"description"`
	Strategy        *string  `json:"strategy" binding:"omitempty,oneof=round_robin random sticky"`
	StickyByAccount *bool    `json:"sticky_by_account"`
	Status          *string  `json:"status" binding:"omitempty,oneof=active inactive"`
	ProxyIDs        *[]int64 `json:"proxy_ids"` // nil=不改成员；非 nil（含空）=替换
}

type SetProxyGroupMembersRequest struct {
	ProxyIDs []int64 `json:"proxy_ids"`
}

// List GET /api/v1/admin/proxy-groups
func (h *ProxyGroupHandler) List(c *gin.Context) {
	page, pageSize := response.ParsePagination(c)
	params := pagination.PaginationParams{
		Page:      page,
		PageSize:  pageSize,
		SortBy:    c.DefaultQuery("sort_by", "id"),
		SortOrder: c.DefaultQuery("sort_order", "desc"),
	}
	groups, result, err := h.svc.List(c.Request.Context(), params)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	out := make([]dto.ProxyGroupWithProxies, 0, len(groups))
	for i := range groups {
		if item := dto.ProxyGroupWithProxiesFromService(&groups[i]); item != nil {
			out = append(out, *item)
		}
	}
	total := int64(0)
	if result != nil {
		total = result.Total
	}
	response.Paginated(c, out, total, page, pageSize)
}

// GetAll GET /api/v1/admin/proxy-groups/all
func (h *ProxyGroupHandler) GetAll(c *gin.Context) {
	groups, err := h.svc.ListActive(c.Request.Context())
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	out := make([]dto.ProxyGroup, 0, len(groups))
	for i := range groups {
		if item := dto.ProxyGroupFromService(&groups[i]); item != nil {
			out = append(out, *item)
		}
	}
	response.Success(c, out)
}

// GetByID GET /api/v1/admin/proxy-groups/:id
func (h *ProxyGroupHandler) GetByID(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "Invalid proxy group ID")
		return
	}
	group, err := h.svc.GetByID(c.Request.Context(), id)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, dto.ProxyGroupWithProxiesFromService(group))
}

// Create POST /api/v1/admin/proxy-groups
func (h *ProxyGroupHandler) Create(c *gin.Context) {
	var req CreateProxyGroupRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}
	executeAdminIdempotentJSON(c, "admin.proxy_groups.create", req, service.DefaultWriteIdempotencyTTL(), func(ctx context.Context) (any, error) {
		group, err := h.svc.Create(ctx, service.CreateProxyGroupInput{
			Name:            strings.TrimSpace(req.Name),
			Description:     strings.TrimSpace(req.Description),
			Strategy:        strings.TrimSpace(req.Strategy),
			StickyByAccount: req.StickyByAccount,
			Status:          strings.TrimSpace(req.Status),
			ProxyIDs:        req.ProxyIDs,
		})
		if err != nil {
			return nil, err
		}
		return dto.ProxyGroupWithProxiesFromService(group), nil
	})
}

// Update PUT /api/v1/admin/proxy-groups/:id
func (h *ProxyGroupHandler) Update(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "Invalid proxy group ID")
		return
	}
	var req UpdateProxyGroupRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}
	input := service.UpdateProxyGroupInput{
		Name:            trimPtr(req.Name),
		Description:     trimPtr(req.Description),
		Strategy:        trimPtr(req.Strategy),
		StickyByAccount: req.StickyByAccount,
		Status:          trimPtr(req.Status),
		ProxyIDs:        req.ProxyIDs,
	}
	group, err := h.svc.Update(c.Request.Context(), id, input)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, dto.ProxyGroupWithProxiesFromService(group))
}

// Delete DELETE /api/v1/admin/proxy-groups/:id
func (h *ProxyGroupHandler) Delete(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "Invalid proxy group ID")
		return
	}
	if err := h.svc.Delete(c.Request.Context(), id); err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, gin.H{"ok": true})
}

// SetMembers PUT /api/v1/admin/proxy-groups/:id/members
func (h *ProxyGroupHandler) SetMembers(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "Invalid proxy group ID")
		return
	}
	var req SetProxyGroupMembersRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}
	if req.ProxyIDs == nil {
		req.ProxyIDs = []int64{}
	}
	group, err := h.svc.SetMembers(c.Request.Context(), id, req.ProxyIDs)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, dto.ProxyGroupWithProxiesFromService(group))
}

func trimPtr(v *string) *string {
	if v == nil {
		return nil
	}
	s := strings.TrimSpace(*v)
	return &s
}
