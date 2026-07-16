// Package handler — support_ticket_notification_handler.go
//
// 用户视角的"工单通知 & 未读计数" HTTP handler。覆盖：
//
//   - GET  /api/v1/support/tickets/unread-count           未读工单数（走 readRepo 聚合）
//   - GET  /api/v1/support/tickets/notifications          铃铛通知列表（支持 only_unread 过滤 + 分页）
//   - POST /api/v1/support/tickets/notifications/:id/read 标记单条已读
//   - POST /api/v1/support/tickets/notifications/read-all 批量标记所有已读
//
// 所有路由都要求登录（路由层挂 JWTAuthMiddleware）。所有响应格式统一走
// response.Success / response.ErrorFrom，与其他 handler 保持一致。
//
// admin 端有一组几乎相同的 handler（handler/admin/support_ticket_notification_handler.go），
// 差异仅在于 readRepo count 走 CountAdminUnreadTickets 而不是 CountUserUnreadTickets——
// 通知 CRUD 走同一 service 方法，因为 support_ticket_notification 表按 recipient_user_id
// 隔离，同一用户只会看到属于自己的通知条目。
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

// SupportTicketNotificationHandler 依赖两个 service：
//
//   - ticketService：提供 CountUserUnreadTickets（读 readRepo 聚合）；
//   - notifService：提供 List/Mark* 通知 CRUD。
//
// 拆两个而非合成一个是因为"未读工单数"和"通知条目"是**两条独立的数据流**——
// 前者是"tickets × replies × reads" 三表聚合，后者是 support_ticket_notification 单表操作。
type SupportTicketNotificationHandler struct {
	ticketService *service.SupportTicketService
	notifService  *service.SupportTicketNotificationService
}

// NewSupportTicketNotificationHandler 构造用户端工单通知 handler。
func NewSupportTicketNotificationHandler(
	ticketService *service.SupportTicketService,
	notifService *service.SupportTicketNotificationService,
) *SupportTicketNotificationHandler {
	return &SupportTicketNotificationHandler{ticketService: ticketService, notifService: notifService}
}

// UnreadCount 处理 GET /api/v1/support/tickets/unread-count。
//
// 语义：返回当前用户名下"有 admin 回复但用户尚未看过"的工单数（详见 SupportTicketReadRepository）。
// 使用场景：前端 Sidebar 红点、AnnouncementBell 徽标数字。
func (h *SupportTicketNotificationHandler) UnreadCount(c *gin.Context) {
	subject, ok := middleware2.GetAuthSubjectFromContext(c)
	if !ok {
		response.Unauthorized(c, "User not found in context")
		return
	}
	count, err := h.ticketService.CountUserUnreadTickets(c.Request.Context(), subject.UserID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, dto.SupportTicketUnreadCountResponse{Count: count})
}

// List 处理 GET /api/v1/support/tickets/notifications。
//
// Query 参数：
//   - page / page_size：分页（走 response.ParsePagination，与其他 handler 一致）；
//   - only_unread：`true` 时只返回 is_read=false 的条目；缺省或非 true 返回全部。
//
// 响应：分页信息 + items（DTO 展平结构）。
func (h *SupportTicketNotificationHandler) List(c *gin.Context) {
	subject, ok := middleware2.GetAuthSubjectFromContext(c)
	if !ok {
		response.Unauthorized(c, "User not found in context")
		return
	}
	page, pageSize := response.ParsePagination(c)
	params := pagination.PaginationParams{Page: page, PageSize: pageSize}
	onlyUnread := c.Query("only_unread") == "true"

	items, pageResult, err := h.notifService.ListNotifications(
		c.Request.Context(),
		subject.UserID,
		onlyUnread,
		params,
	)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Paginated(c, dto.SupportTicketNotificationItemsFromService(items), pageResult.Total, page, pageSize)
}

// MarkOneRead 处理 POST /api/v1/support/tickets/notifications/:id/read。
//
// 权限校验完全下沉到 service/repo 层：(id, recipientUserID) 二元定位；
// 不属于当前用户的通知直接 404（不泄露"该 id 是否存在"）。
func (h *SupportTicketNotificationHandler) MarkOneRead(c *gin.Context) {
	subject, ok := middleware2.GetAuthSubjectFromContext(c)
	if !ok {
		response.Unauthorized(c, "User not found in context")
		return
	}
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		response.BadRequest(c, "Invalid notification ID")
		return
	}
	if err := h.notifService.MarkOneRead(c.Request.Context(), id, subject.UserID); err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, gin.H{"id": id})
}

// MarkAllRead 处理 POST /api/v1/support/tickets/notifications/read-all。
//
// 幂等：重复调用返回 affected=0 但 HTTP 200；用于前端"一键清空未读"按钮。
func (h *SupportTicketNotificationHandler) MarkAllRead(c *gin.Context) {
	subject, ok := middleware2.GetAuthSubjectFromContext(c)
	if !ok {
		response.Unauthorized(c, "User not found in context")
		return
	}
	affected, err := h.notifService.MarkAllRead(c.Request.Context(), subject.UserID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, dto.SupportTicketMarkAllReadResponse{Affected: affected})
}
