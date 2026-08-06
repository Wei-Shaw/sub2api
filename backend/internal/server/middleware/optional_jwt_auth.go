package middleware

import (
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/gin-gonic/gin"
)

// NewOptionalJWTAuthMiddleware 创建可选 JWT 认证中间件。
//
// 无 Authorization header 时直接放行（匿名，context 中不设置 AuthSubject）；
// 带 header 则委托严格 JWT 校验（token 版本 / 用户状态 / 会话绑定），失败返回 401——
// 前端 API client 对 401 会自动走 refresh-token 重试，因此不做静默降级。
func NewOptionalJWTAuthMiddleware(
	authService *service.AuthService,
	userService *service.UserService,
	settingService *service.SettingService,
	auditService *service.AuditLogService,
) OptionalJWTAuthMiddleware {
	strict := jwtAuth(authService, userService, userService, settingService, auditService)
	return OptionalJWTAuthMiddleware(func(c *gin.Context) {
		if strings.TrimSpace(c.GetHeader("Authorization")) == "" {
			c.Next()
			return
		}
		strict(c)
	})
}
