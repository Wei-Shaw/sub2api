package routes

import (
	"github.com/Wei-Shaw/sub2api/internal/inbox"
	middleware2 "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/gin-gonic/gin"
)

// RegisterInboxRoutes 注册通用信箱的用户态与管理态路由。
//
//	用户态（JWT 鉴权）：
//	  GET  /api/v1/inbox/catchup        断线/首屏补齐
//	  POST /api/v1/inbox/ack            抬升已读水位
//	  GET  /api/v1/inbox/unread-count   未读徽标计数
//	  GET  /api/v1/inbox/ws             实时推送 WebSocket（handler 内自鉴权）
//
//	管理态（Admin 鉴权）：
//	  POST /api/v1/admin/inbox/broadcast        发布广播
//	  GET  /api/v1/admin/inbox/broadcasts       广播审计查询
//	  GET  /api/v1/admin/inbox/direct-messages  单播审计查询
func RegisterInboxRoutes(
	v1 *gin.RouterGroup,
	h *inbox.Handler,
	ws *inbox.WSHandler,
	jwtAuth middleware2.JWTAuthMiddleware,
	adminAuth middleware2.AdminAuthMiddleware,
) {
	if h == nil {
		return
	}

	grp := v1.Group("/inbox")
	grp.Use(gin.HandlerFunc(jwtAuth))
	{
		grp.GET("/catchup", h.Catchup)
		grp.POST("/ack", h.Ack)
		grp.GET("/unread-count", h.UnreadCount)
	}

	// WebSocket 端点在 handler 内部自行鉴权（浏览器握手无法携带 Authorization header），
	// 因此不挂 jwtAuth 中间件。
	if ws != nil {
		v1.GET("/inbox/ws", ws.Handle)
	}

	admin := v1.Group("/admin/inbox")
	admin.Use(gin.HandlerFunc(adminAuth))
	{
		admin.POST("/broadcast", h.Broadcast)
		admin.GET("/broadcasts", h.AdminListBroadcasts)
		admin.GET("/direct-messages", h.AdminListDirectMessages)
	}
}
