// Package admin — support_ticket_handler.go
//
// admin 端工单 HTTP handler。覆盖：
//
//   - GET    /api/v1/admin/support/tickets                     分页 + 过滤（status/priority/category/user_id/q）
//   - GET    /api/v1/admin/support/tickets/:id                 详情（含 chat_context + 回复）
//   - POST   /api/v1/admin/support/tickets/:id/replies         admin 回复（自动 open → in_progress）
//   - PATCH  /api/v1/admin/support/tickets/:id                 修改 status/priority/category（拒 reopen closed）
//
// admin 路由不卡 feature_enabled（spec 7.2：管理员可预先编辑配置或处理已有工单）。
package admin

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

// SupportTicketHandler 处理 admin 视角的工单接口。
type SupportTicketHandler struct {
	service *service.SupportTicketService
}

// NewSupportTicketHandler 构造 admin 端工单 handler。
func NewSupportTicketHandler(svc *service.SupportTicketService) *SupportTicketHandler {
	return &SupportTicketHandler{service: svc}
}

// adminAppendReplyRequest 复用普通用户的入参形态：单字段 content。
type adminAppendReplyRequest struct {
	Content string `json:"content" binding:"required"`
}

// adminPatchTicketRequest 是 PATCH /api/v1/admin/support/tickets/:id 入参。
//
// 三个字段都用 *string：nil = 不改；非 nil = 写入。binding 用 oneof 约束枚举，
// 但仍依赖 service 层做最终校验（含 closed → 非 closed 状态机拒绝）。
type adminPatchTicketRequest struct {
	Status   *string `json:"status" binding:"omitempty,oneof=open in_progress closed"`
	Priority *string `json:"priority" binding:"omitempty,oneof=low normal high"`
	Category *string `json:"category" binding:"omitempty"`
}

// List 处理 GET /api/v1/admin/support/tickets。
//
// 支持的过滤参数（任意组合）：
//   - status: open|in_progress|closed
//   - priority: low|normal|high
//   - category: 任意字符串（无需在当前 settings.categories 内——支持查询历史分类）
//   - user_id: int64
//   - q: 关键词，service+repo 走 ILIKE on (title, content)
//
// 排序由 repo 层强制为 priority CASE-DESC, created_at DESC, id DESC。
func (h *SupportTicketHandler) List(c *gin.Context) {
	page, pageSize := response.ParsePagination(c)
	params := pagination.PaginationParams{Page: page, PageSize: pageSize}

	filters := service.SupportTicketListFilters{
		Status:   strings.TrimSpace(c.Query("status")),
		Priority: strings.TrimSpace(c.Query("priority")),
		Category: strings.TrimSpace(c.Query("category")),
		Search:   strings.TrimSpace(c.Query("q")),
	}
	if rawUID := strings.TrimSpace(c.Query("user_id")); rawUID != "" {
		uid, err := strconv.ParseInt(rawUID, 10, 64)
		if err != nil || uid <= 0 {
			response.BadRequest(c, "Invalid user_id")
			return
		}
		filters.UserID = &uid
	}
	if len(filters.Search) > 200 {
		filters.Search = filters.Search[:200]
	}

	items, paginationResult, err := h.service.ListAdminTickets(c.Request.Context(), filters, params)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}

	out := make([]dto.SupportTicketListItem, 0, len(items))
	for i := range items {
		out = append(out, *dto.SupportTicketListItemFromService(&items[i]))
	}
	response.Paginated(c, out, paginationResult.Total, page, pageSize)
}

// Get 处理 GET /api/v1/admin/support/tickets/:id。
func (h *SupportTicketHandler) Get(c *gin.Context) {
	id, err := parseAdminTicketID(c)
	if err != nil {
		response.BadRequest(c, "Invalid ticket ID")
		return
	}

	twr, err := h.service.GetAdminTicket(c.Request.Context(), id)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, dto.SupportTicketDetailFromService(twr))
}

// AppendReply 处理 POST /api/v1/admin/support/tickets/:id/replies。
//
// service 内在事务中完成"INSERT reply + 可选 open→in_progress"，handler 只关心
// 返回值与错误映射。
func (h *SupportTicketHandler) AppendReply(c *gin.Context) {
	subject, ok := middleware2.GetAuthSubjectFromContext(c)
	if !ok {
		response.Unauthorized(c, "User not found in context")
		return
	}

	id, err := parseAdminTicketID(c)
	if err != nil {
		response.BadRequest(c, "Invalid ticket ID")
		return
	}

	var req adminAppendReplyRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}

	reply, err := h.service.AppendAdminReply(c.Request.Context(), subject.UserID, id, req.Content)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, dto.SupportTicketReplyFromService(reply))
}

// Patch 处理 PATCH /api/v1/admin/support/tickets/:id。
//
// 任一字段都为 nil 时返回 400（来自 service ErrSupportTicketNoFieldsToUpdate）。
// closed → 非 closed 转移返回 409。
func (h *SupportTicketHandler) Patch(c *gin.Context) {
	id, err := parseAdminTicketID(c)
	if err != nil {
		response.BadRequest(c, "Invalid ticket ID")
		return
	}

	var req adminPatchTicketRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}

	updated, err := h.service.PatchAdmin(c.Request.Context(), id, service.AdminTicketPatch{
		Status:   req.Status,
		Priority: req.Priority,
		Category: req.Category,
	})
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	// PATCH 路径不直接返回 replies（避免反复加载）；前端如需可单独 GET 详情。
	response.Success(c, dto.SupportTicketListItemFromService(updated))
}

// parseAdminTicketID 解析 :id 路径参数。
func parseAdminTicketID(c *gin.Context) (int64, error) {
	v, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || v <= 0 {
		if err == nil {
			err = strconv.ErrSyntax
		}
		return 0, err
	}
	return v, nil
}
