package routes

import (
	"github.com/Wei-Shaw/sub2api/internal/handler"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

func RegisterDeveloperFileRoutes(
	v1 *gin.RouterGroup,
	h *handler.Handlers,
	auth middleware.DeveloperKeyAuthMiddleware,
	settingService *service.SettingService,
	rateLimiter *middleware.PanelRateLimiter,
) {
	files := v1.Group("/file")
	files.Use(gin.HandlerFunc(auth))
	files.Use(middleware.BackendModeUserGuard(settingService))
	files.Use(rateLimiter.DeveloperFile())
	{
		files.POST("/", h.DeveloperFile.Upload)
		files.DELETE("/", h.DeveloperFile.Delete)
	}
}
