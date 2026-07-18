// Package routes — inbox_migration.go
//
// general-inbox PR-6：灰度开关打开（config.Inbox.V1Enabled）后，旧的工单通知
// REST 端点（/support/tickets/notifications*、/admin/support/tickets/notifications*）
// 统一返回 410 Gone，引导前端切换到 /inbox/* + WebSocket。
//
// 采用"包装 handler"而非组级中间件：通知端点与 /tickets/:id 等混挂在同一 group 下，
// 组级中间件会误伤同组其他路由，因此只对这 4+4 个端点逐个包装。
package routes

import (
	"net/http"

	"github.com/Wei-Shaw/sub2api/internal/pkg/response"

	"github.com/gin-gonic/gin"
)

// inboxMigratedMessage 是 410 响应的人类可读提示。
const inboxMigratedMessage = "This endpoint has been retired. Use /api/v1/inbox/catchup, /api/v1/inbox/ack, /api/v1/inbox/unread-count and the inbox WebSocket instead."

// inboxMigrated 根据灰度开关决定端点行为：
//   - enabled=false：返回原始 handler（旧通知表路径保持工作）；
//   - enabled=true ：返回一个直接 410 Gone 的 handler（旧端点退役）。
func inboxMigrated(enabled bool, real gin.HandlerFunc) gin.HandlerFunc {
	if !enabled {
		return real
	}
	return func(c *gin.Context) {
		response.Error(c, http.StatusGone, inboxMigratedMessage)
		c.Abort()
	}
}
