package routes

import (
	"github.com/Wei-Shaw/sub2api/internal/handler"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/gin-gonic/gin"
)

// RegisterModelPlazaRoutes 注册模型广场路由。
//
// 挂 OptionalJWT：匿名可访问（开关与 require_auth 由 handler fail-closed 判定），
// 带 token 则识别用户以展示专属分组与个人倍率。
// BackendModeUserGuard 保证 backend 模式下广场不对非管理员开放（匿名无 role → 403）。
func RegisterModelPlazaRoutes(
	v1 *gin.RouterGroup,
	h *handler.Handlers,
	optionalJWT middleware.OptionalJWTAuthMiddleware,
	settingService *service.SettingService,
	panelRateLimiter *middleware.PanelRateLimiter,
) {
	plaza := v1.Group("/model-plaza")
	plaza.Use(panelRateLimiter.PublicIP())
	plaza.Use(gin.HandlerFunc(optionalJWT))
	plaza.Use(middleware.BackendModeUserGuard(settingService))
	{
		plaza.GET("", h.ModelPlaza.Get)
	}
}
