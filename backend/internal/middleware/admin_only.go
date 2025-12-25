package middleware

import (
	"github.com/Wei-Shaw/sub2api/internal/model"

	"github.com/gin-gonic/gin"
)

// AdminOnly 管理员权限中间件
// 必须在JWTAuth中间件之后使用
func AdminOnly() gin.HandlerFunc {
	return func(c *gin.Context) {
		// 从上下文获取用户
		user, exists := GetUserFromContext(c)
		if !exists {
			AbortWithError(c, 401, "UNAUTHORIZED", "User not found in context")
			return
		}

		// 检查是否为管理员
		if user.Role != model.RoleAdmin {
			AbortWithError(c, 403, "FORBIDDEN", "Admin access required")
			return
		}

		c.Next()
	}
}
