// Package admin — support_ticket_notification_handler.go
//
// admin 视角的"工单通知 & 未读计数" HTTP handler。覆盖：
//
//   - GET  /api/v1/admin/support/tickets/unread-count           管理员视角未读工单数
//   - GET  /api/v1/admin/support/tickets/notifications          admin 铃铛通知列表
//   - POST /api/v1/admin/support/tickets/notifications/:id/read 标记单条已读
//   - POST /api/v1/admin/support/tickets/notifications/read-all 批量标记所有已读
//
// 与用户端 handler 相比只有一处差异：unread-count 走 CountAdminUnreadTickets
// （管理员视角谓词——工单创建 / 用户回复晚于 last_read_at），其余 3 个端点
// 完全复用 SupportTicketNotificationService 上的同名方法，因为通知条目按
// recipient_user_id 隔离（admin 自己作为 recipient），service 层无需感知
// 调用者是 user 还是 admin。
//
// admin 路由不卡 support_ticket_enabled feature gate（spec 7.2：管理员可
// 预先看到堆积的历史工单未读，即使 feature 临时关掉）。
package admin

import (
	"strconv"

	"github.com/Wei-Shaw/sub2api/internal/handler/dto"
	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	middleware2 "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/gin-gonic/gin"
)

// SupportTicketNotificationHandler 是 admin 视角的工单通知 handler。
// 与用户端 handler.SupportTicketNotificationHandler 的差异仅在 UnreadCount 走
// admin 视角的聚合谓词；其余 CRUD 完全对称。
type SupportTicketNotificationHandler struct {
	ticketService *service.SupportTicketService
	notifService  *service.SupportTicketNotificationService
}

// NewSupportTicketNotificationHandler 构造 admin 端工单通知 handler。
func NewSupportTicketNotificationHandler(
	ticketService *service.SupportTicketService,
	notifService *service.SupportTicketNotificationService,
) *SupportTicketNotificationHandler {
	return &SupportTicketNotificationHandler{ticketService: ticketService, notifService: notifService}
}

// UnreadCount 处理 GET /api/v1/admin/support/tickets/unread-count。
//
// admin 视角谓词：全站"新工单 or 用户回复且当前 admin 尚未看过"的工单数量。
// 每个 admin 各自维护独立读游标——所以两个 admin 同时点击同一工单，各自的
// count 都会独立下降 1。
func (h *SupportTicketNotificationHandler) UnreadCount(c *gin.Context) {
	subject, ok := middleware2.GetAuthSubjectFromContext(c)
	if !ok {
		response.Unauthorized(c, "User not found in context")
		return
	}
	count, err := h.ticketService.CountAdminUnreadTickets(c.Request.Context(), subject.UserID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, dto.SupportTicketUnreadCountResponse{Count: count})
}

// List 处理 GET /api/v1/admin/support/tickets/notifications。
//
// 用法与用户端对称，见 handler.SupportTicketNotificationHandler.List。
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

// MarkOneRead 处理 POST /api/v1/admin/support/tickets/notifications/:id/read。
//
// 与用户端一致：(id, recipientUserID) 二元定位；不属于当前 admin 的条目 → 404。
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

// MarkAllRead 处理 POST /api/v1/admin/support/tickets/notifications/read-all。
//
// 幂等：重复调用返回 affected=0 但 HTTP 200。
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
