package routes

import (
	"github.com/Wei-Shaw/sub2api/internal/handler"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"

	"github.com/gin-gonic/gin"
)

// RegisterAuthRoutes 注册认证相关路由
func RegisterAuthRoutes(
	v1 *gin.RouterGroup,
	h *handler.Handlers,
	jwtAuth middleware.JWTAuthMiddleware,
) {
	// 公开接口
	auth := v1.Group("/auth")
	{
		auth.POST("/register", h.Auth.Register)
		auth.POST("/login", h.Auth.Login)
		auth.POST("/send-verify-code", h.Auth.SendVerifyCode)
		auth.GET("/oauth/linuxdo/start", h.Auth.LinuxDoOAuthStart)
		auth.GET("/oauth/linuxdo/callback", h.Auth.LinuxDoOAuthCallback)
REDACTED

	// 公开设置（无需认证）
	settings := v1.Group("/settings")
	{
		settings.GET("/public", h.Setting.GetPublicSettings)
REDACTED

	// 需要认证的当前用户信息
	authenticated := v1.Group("")
	authenticated.Use(gin.HandlerFunc(jwtAuth))
	{
		authenticated.GET("/auth/me", h.Auth.GetCurrentUser)
REDACTED
REDACTED
