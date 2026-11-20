package routes

import (
	"github.com/Wei-Shaw/sub2api/internal/handler"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/gin-gonic/gin"
)

// 管理密钥生命周期只接受管理员会话；写操作沿用二次验证。
func registerScopedAdminKeyRoutes(settings *gin.RouterGroup, h *handler.Handlers, stepUp middleware.StepUpAuthMiddleware) {
	keys := settings.Group("/admin-api-keys")
	keys.GET("", h.Admin.Setting.ListScopedAdminKeys)
	keys.POST("", gin.HandlerFunc(stepUp), h.Admin.Setting.CreateScopedAdminKey)
	keys.PUT("/:id", gin.HandlerFunc(stepUp), h.Admin.Setting.UpdateScopedAdminKey)
	keys.POST("/:id/rotate", gin.HandlerFunc(stepUp), h.Admin.Setting.RotateScopedAdminKey)
	keys.DELETE("/:id", gin.HandlerFunc(stepUp), h.Admin.Setting.RevokeScopedAdminKey)
	keys.DELETE("/:id/record", gin.HandlerFunc(stepUp), h.Admin.Setting.DeleteScopedAdminKeyRecord)
}
