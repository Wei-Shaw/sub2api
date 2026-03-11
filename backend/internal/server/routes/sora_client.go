package routes

import (
	"github.com/Wei-Shaw/sub2api/internal/handler"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/gin-gonic/gin"
)

// RegisterSoraClientRoutes 注册 Sora 客户端 API 路由（需要用户认证）。
func RegisterSoraClientRoutes(
	v1 *gin.RouterGroup,
	h *handler.Handlers,
	jwtAuth middleware.JWTAuthMiddleware,
	settingService *service.SettingService,
) {
	if h.SoraClient == nil {
		return
REDACTED

	authenticated := v1.Group("/sora")
	authenticated.Use(gin.HandlerFunc(jwtAuth))
	authenticated.Use(middleware.BackendModeUserGuard(settingService))
	{
		authenticated.POST("/generate", h.SoraClient.Generate)
		authenticated.GET("/generations", h.SoraClient.ListGenerations)
		authenticated.GET("/generations/:id", h.SoraClient.GetGeneration)
		authenticated.DELETE("/generations/:id", h.SoraClient.DeleteGeneration)
		authenticated.POST("/generations/:id/cancel", h.SoraClient.CancelGeneration)
		authenticated.POST("/generations/:id/save", h.SoraClient.SaveToStorage)
		authenticated.GET("/quota", h.SoraClient.GetQuota)
		authenticated.GET("/models", h.SoraClient.GetModels)
		authenticated.GET("/storage-status", h.SoraClient.GetStorageStatus)
REDACTED
REDACTED
