// Package routes — inbox_migration.go
//
// general-inbox：通用信箱已默认启用，旧的工单通知 REST 端点
// （/support/tickets/notifications*、/admin/support/tickets/notifications*）
// 统一返回 410 Gone，引导前端使用 /inbox/* + WebSocket。
package routes

import (
	"net/http"

	"github.com/Wei-Shaw/sub2api/internal/pkg/response"

	"github.com/gin-gonic/gin"
)

// inboxMigratedMessage 是 410 响应的人类可读提示。
const inboxMigratedMessage = "This endpoint has been retired. Use /api/v1/inbox/catchup, /api/v1/inbox/ack, /api/v1/inbox/unread-count and the inbox WebSocket instead."

// inboxMigratedHandler 是一个恒返回 410 Gone 的退役端点处理器。
func inboxMigratedHandler(c *gin.Context) {
	response.Error(c, http.StatusGone, inboxMigratedMessage)
	c.Abort()
}
