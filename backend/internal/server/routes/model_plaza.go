package routes

import (
	"github.com/Wei-Shaw/sub2api/internal/handler"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/gin-gonic/gin"
)

// RegisterModelPlazaRoutes 注册模型广场路由。
//
// 模型价格属于登录后的用户面板能力：统一要求 JWT，按当前用户展示
// 可访问分组、专属分组与个人倍率。
// BackendModeUserGuard 保证 backend 模式下广场不对非管理员开放（匿名无 role → 403）。
func RegisterModelPlazaRoutes(
	v1 *gin.RouterGroup,
	h *handler.Handlers,
	jwtAuth middleware.JWTAuthMiddleware,
	settingService *service.SettingService,
	panelRateLimiter *middleware.PanelRateLimiter,
) {
	plaza := v1.Group("/model-plaza")
	plaza.Use(panelRateLimiter.PublicIP())
	plaza.Use(gin.HandlerFunc(jwtAuth))
	plaza.Use(middleware.BackendModeUserGuard(settingService))
	{
		plaza.GET("", h.ModelPlaza.Get)
	}
}
