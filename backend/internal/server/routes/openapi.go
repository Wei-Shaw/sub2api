package routes

import (
	"github.com/Wei-Shaw/sub2api/internal/handler"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"

	"github.com/gin-gonic/gin"
)

// RegisterOpenAPIRoutes 注册 /api/v1/openapi/* 路由（M2M 数据集成接口）。
// 所有路由套用 adminAuth 中间件鉴权。
func RegisterOpenAPIRoutes(
	v1 *gin.RouterGroup,
	h *handler.Handlers,
	adminAuth middleware.AdminAuthMiddleware,
) {
	openapi := v1.Group("/openapi")
	openapi.Use(gin.HandlerFunc(adminAuth))
	{
		users := openapi.Group("/users")
		{
			users.POST("", h.OpenAPI.CreateUser)
			users.GET("/:email", h.OpenAPI.GetUserByEmail)
			users.PATCH("/:email/balance", h.OpenAPI.AdjustBalance)

			users.POST("/:email/keys", h.OpenAPI.CreateKey)
			users.GET("/:email/keys", h.OpenAPI.ListKeys)
			users.PATCH("/:email/keys/:key_id", h.OpenAPI.UpdateKey)
			users.DELETE("/:email/keys/:key_id", h.OpenAPI.DeleteKey)

			users.GET("/:email/usage", h.OpenAPI.ListUsage)
			users.GET("/:email/usage/stats", h.OpenAPI.UsageStats)
		}
	}
}
