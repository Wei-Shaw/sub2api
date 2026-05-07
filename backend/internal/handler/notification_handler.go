package handler

import (
	"strconv"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/gin-gonic/gin"
)

// NotificationHandler exposes the user-side notification inbox.
type NotificationHandler struct {
	notificationService *service.NotificationService
}

// NewNotificationHandler creates a new NotificationHandler.
func NewNotificationHandler(notificationService *service.NotificationService) *NotificationHandler {
	return &NotificationHandler{notificationService: notificationService}
}

// List returns paginated notifications for the authenticated user.
// GET /api/v1/notifications
func (h *NotificationHandler) List(c *gin.Context) {
	subject, ok := requireAuth(c)
	if !ok {
		return
	}
	page, pageSize := response.ParsePagination(c)
	unread := strings.EqualFold(c.Query("unread_only"), "true")
	notifications, total, err := h.notificationService.ListForUser(c.Request.Context(), subject.UserID, service.UserNotificationListParams{
		Page:       page,
		PageSize:   pageSize,
		Category:   c.Query("category"),
		UnreadOnly: unread,
	})
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Paginated(c, notifications, int64(total), page, pageSize)
}

// UnreadCount returns the count of unread notifications.
// GET /api/v1/notifications/unread-count
func (h *NotificationHandler) UnreadCount(c *gin.Context) {
	subject, ok := requireAuth(c)
	if !ok {
		return
	}
	n, err := h.notificationService.CountUnread(c.Request.Context(), subject.UserID, c.Query("category"))
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, gin.H{"count": n})
}

// MarkRead marks a single notification as read.
// POST /api/v1/notifications/:id/read
func (h *NotificationHandler) MarkRead(c *gin.Context) {
	subject, ok := requireAuth(c)
	if !ok {
		return
	}
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		response.BadRequest(c, "Invalid notification ID")
		return
	}
	if err := h.notificationService.MarkRead(c.Request.Context(), subject.UserID, id); err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, gin.H{"message": "marked"})
}

// MarkAllRead marks all of the user's notifications as read (optionally filtered by category).
// POST /api/v1/notifications/read-all
func (h *NotificationHandler) MarkAllRead(c *gin.Context) {
	subject, ok := requireAuth(c)
	if !ok {
		return
	}
	count, err := h.notificationService.MarkAllRead(c.Request.Context(), subject.UserID, c.Query("category"))
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, gin.H{"count": count})
}
