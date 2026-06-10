// Package handler — support_ticket_handler.go
//
// 用户端工单 HTTP handler。覆盖：
//
//   - POST   /api/v1/support/tickets               用户新建工单
//   - GET    /api/v1/support/tickets               用户分页拉取自己的工单列表（不含 chat_context）
//   - GET    /api/v1/support/tickets/:id           用户取自己的工单详情（含 chat_context + 回复时间线）
//   - POST   /api/v1/support/tickets/:id/replies   用户追加回复
//   - POST   /api/v1/support/tickets/:id/close     用户关闭自己的工单
//   - GET    /api/v1/support/categories            用户拉取可选分类与默认优先级（新建页下拉用）
//
// 所有路由都需要登录态（在路由层注入 JWTAuthMiddleware）。错误映射依赖
// service 层 sentinel + infraerrors 类型签名 + response.ErrorFrom。
package handler

import (
	"strconv"

	"github.com/Wei-Shaw/sub2api/internal/handler/dto"
	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	middleware2 "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/gin-gonic/gin"
)

// SupportTicketHandler 处理用户视角的工单接口。
type SupportTicketHandler struct {
	service *service.SupportTicketService
}

// NewSupportTicketHandler 构造用户端工单 handler。
func NewSupportTicketHandler(svc *service.SupportTicketService) *SupportTicketHandler {
	return &SupportTicketHandler{service: svc}
}

// CreateSupportTicketRequest 是 POST /api/v1/support/tickets 入参。
//
// Priority 可选：未传走 settings.default_priority；ChatContext 用 *string 区分
// "未提供"与"显式空字符串"，但 service 层会把空白视为未提供。
type CreateSupportTicketRequest struct {
	Title       string  `json:"title" binding:"required"`
	Content     string  `json:"content" binding:"required"`
	Category    string  `json:"category" binding:"required"`
	Priority    string  `json:"priority" binding:"omitempty,oneof=low normal high"`
	ChatContext *string `json:"chat_context,omitempty"`
}

// AppendReplyRequest 是 POST /api/v1/support/tickets/:id/replies 与
// POST /api/v1/admin/support/tickets/:id/replies 共用的入参。
type AppendReplyRequest struct {
	Content string `json:"content" binding:"required"`
}

// Create 处理 POST /api/v1/support/tickets。
//
// 错误映射（由 service sentinel 决定）：
//   - feature_disabled / 非法分类 / 长度超限 → 由 ErrorFrom 自动映射 404 / 400。
//   - 创建成功返回 SupportTicketDetail（首次创建无回复，replies 为空数组）。
func (h *SupportTicketHandler) Create(c *gin.Context) {
	subject, ok := middleware2.GetAuthSubjectFromContext(c)
	if !ok {
		response.Unauthorized(c, "User not found in context")
		return
	}

	var req CreateSupportTicketRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}

	tk, err := h.service.CreateTicket(c.Request.Context(), service.CreateTicketInput{
		UserID:      subject.UserID,
		Title:       req.Title,
		Content:     req.Content,
		Category:    req.Category,
		Priority:    req.Priority,
		ChatContext: req.ChatContext,
	})
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}

	// 创建后直接返回详情形态（含 ChatContext，replies 为空）：避免前端再发一次 GET。
	response.Success(c, dto.SupportTicketDetailFromService(&service.SupportTicketWithReplies{
		Ticket:  *tk,
		Replies: nil,
	}))
}

// List 处理 GET /api/v1/support/tickets。
func (h *SupportTicketHandler) List(c *gin.Context) {
	subject, ok := middleware2.GetAuthSubjectFromContext(c)
	if !ok {
		response.Unauthorized(c, "User not found in context")
		return
	}

	page, pageSize := response.ParsePagination(c)
	params := pagination.PaginationParams{Page: page, PageSize: pageSize}

	items, paginationResult, err := h.service.ListUserTickets(c.Request.Context(), subject.UserID, params)
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

// Get 处理 GET /api/v1/support/tickets/:id。
//
// service 强制 owner 校验：非 owner / 不存在统一 404，避免泄露存在性。
func (h *SupportTicketHandler) Get(c *gin.Context) {
	subject, ok := middleware2.GetAuthSubjectFromContext(c)
	if !ok {
		response.Unauthorized(c, "User not found in context")
		return
	}

	id, err := parseInt64Param(c, "id")
	if err != nil {
		response.BadRequest(c, "Invalid ticket ID")
		return
	}

	twr, err := h.service.GetUserTicket(c.Request.Context(), subject.UserID, id)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, dto.SupportTicketDetailFromService(twr))
}

// AppendReply 处理 POST /api/v1/support/tickets/:id/replies。
func (h *SupportTicketHandler) AppendReply(c *gin.Context) {
	subject, ok := middleware2.GetAuthSubjectFromContext(c)
	if !ok {
		response.Unauthorized(c, "User not found in context")
		return
	}

	id, err := parseInt64Param(c, "id")
	if err != nil {
		response.BadRequest(c, "Invalid ticket ID")
		return
	}

	var req AppendReplyRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}

	reply, err := h.service.AppendUserReply(c.Request.Context(), subject.UserID, id, req.Content)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, dto.SupportTicketReplyFromService(reply))
}

// Close 处理 POST /api/v1/support/tickets/:id/close。
func (h *SupportTicketHandler) Close(c *gin.Context) {
	subject, ok := middleware2.GetAuthSubjectFromContext(c)
	if !ok {
		response.Unauthorized(c, "User not found in context")
		return
	}

	id, err := parseInt64Param(c, "id")
	if err != nil {
		response.BadRequest(c, "Invalid ticket ID")
		return
	}

	if err := h.service.CloseUserTicket(c.Request.Context(), subject.UserID, id); err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, gin.H{"message": "ok"})
}

// ListCategories 处理 GET /api/v1/support/categories。
//
// 当 feature_enabled = false 时返回 404（service 层抛 ErrSupportFeatureDisabled）。
func (h *SupportTicketHandler) ListCategories(c *gin.Context) {
	cats, defaultPriority, err := h.service.ListCategories(c.Request.Context())
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, dto.SupportTicketCategoriesResponse{
		Categories:      cats,
		DefaultPriority: defaultPriority,
	})
}

// parseInt64Param 把路径参数解析为正整数。负数或解析失败返回错误。
//
// 与 strconv.ParseInt 直接调用相比，多了一道"必须 > 0"的语义校验，避免 0 / 负
// id 漏到 service 层（虽然 service 也会查不到记录，但 handler 层 fail-fast 更
// 友好）。
func parseInt64Param(c *gin.Context, name string) (int64, error) {
	v, err := strconv.ParseInt(c.Param(name), 10, 64)
	if err != nil || v <= 0 {
		if err == nil {
			err = strconv.ErrSyntax
		}
		return 0, err
	}
	return v, nil
}
