package middleware

import (
	"context"
	"fmt"
	"github.com/Wei-Shaw/sub2api/internal/model"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"strings"

	"github.com/gin-gonic/gin"
)

// JWTAuth JWT认证中间件
func JWTAuth(authService *service.AuthService, userRepo interface {
	GetByID(ctx context.Context, id int64) (*model.User, error)
}) gin.HandlerFunc {
	return func(c *gin.Context) {
		tokenString, viaCookie, err := extractToken(c, authService)
		if err != nil {
			AbortWithError(c, 401, "UNAUTHORIZED", err.Error())
			return
		}

		// 如果走 Cookie 认证，则需要来源校验，避免跨站请求伪造。
		if viaCookie && !enforceCookieOrigin(c, authService.AllowedOrigins(), authService.AuthCookieRequireOrigin()) {
			AbortWithError(c, 403, "INVALID_ORIGIN", "Origin or Referer check failed")
			return
		}

		// 验证token
		claims, err := authService.ValidateToken(tokenString)
		if err != nil {
			if err == service.ErrTokenExpired {
				AbortWithError(c, 401, "TOKEN_EXPIRED", "Token has expired")
				return
			}
			AbortWithError(c, 401, "INVALID_TOKEN", "Invalid token")
			return
		}

		// 从数据库获取最新的用户信息
		user, err := userRepo.GetByID(c.Request.Context(), claims.UserID)
		if err != nil {
			AbortWithError(c, 401, "USER_NOT_FOUND", "User not found")
			return
		}

		// 检查用户状态
		if !user.IsActive() {
			AbortWithError(c, 401, "USER_INACTIVE", "User account is not active")
			return
		}

		// 将用户信息存入上下文
		c.Set(string(ContextKeyUser), user)
		if viaCookie {
			c.Set("auth_method", "cookie")
		} else {
			c.Set("auth_method", "jwt")
		}

		c.Next()
	}
}

// extractToken 同时支持 Authorization Bearer 与 HttpOnly Cookie。
// 返回值 viaCookie 用于区分认证方式并执行额外安全校验。
func extractToken(c *gin.Context, authService *service.AuthService) (string, bool, error) {
	authHeader := c.GetHeader("Authorization")
	if authHeader != "" {
		parts := strings.SplitN(authHeader, " ", 2)
		if len(parts) != 2 || parts[0] != "Bearer" {
			return "", false, fmt.Errorf("Authorization header format must be 'Bearer {token}'")
		}
		tokenString := strings.TrimSpace(parts[1])
		if tokenString == "" {
			return "", false, fmt.Errorf("Token cannot be empty")
		}
		return tokenString, false, nil
	}

	cookie, err := c.Cookie(authService.AuthCookieName())
	if err != nil || cookie == "" {
		return "", false, fmt.Errorf("Authorization required")
	}
	return cookie, true, nil
}

// GetUserFromContext 从上下文中获取用户
func GetUserFromContext(c *gin.Context) (*model.User, bool) {
	value, exists := c.Get(string(ContextKeyUser))
	if !exists {
		return nil, false
	}
	user, ok := value.(*model.User)
	return user, ok
}
