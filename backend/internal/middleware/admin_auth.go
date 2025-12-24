package middleware

import (
	"context"
	"crypto/subtle"
	"fmt"
	"strings"
	"sub2api/internal/model"
	"sub2api/internal/service"

	"github.com/gin-gonic/gin"
)

// AdminAuth 管理员认证中间件
// 支持两种认证方式（通过不同的 header 区分）：
// 1. Admin API Key: x-api-key: <admin-api-key>
// 2. JWT Token: Authorization: Bearer <jwt-token> (需要管理员角色)
func AdminAuth(
	authService *service.AuthService,
	userRepo interface {
		GetByID(ctx context.Context, id int64) (*model.User, error)
		GetFirstAdmin(ctx context.Context) (*model.User, error)
	},
	settingService *service.SettingService,
) gin.HandlerFunc {
	return func(c *gin.Context) {
		// 检查 x-api-key header（Admin API Key 认证）
		apiKey := c.GetHeader("x-api-key")
		if apiKey != "" {
			if !validateAdminApiKey(c, apiKey, settingService, userRepo) {
				return
			}
			c.Next()
			return
		}

		tokenString, viaCookie, err := extractAdminToken(c, authService)
		if err == nil {
			// Cookie 认证必须通过来源校验，避免跨站提交。
			if viaCookie && !enforceCookieOrigin(c, authService.AllowedOrigins(), authService.AuthCookieRequireOrigin()) {
				AbortWithError(c, 403, "INVALID_ORIGIN", "Origin or Referer check failed")
				return
			}
			if !validateJWTForAdmin(c, tokenString, authService, userRepo) {
				return
			}
			if viaCookie {
				c.Set("auth_method", "cookie")
			} else {
				c.Set("auth_method", "jwt")
			}
			c.Next()
			return
		}

		// 无有效认证信息
		AbortWithError(c, 401, "UNAUTHORIZED", "Authorization required")
	}
}

// validateAdminApiKey 验证管理员 API Key
func validateAdminApiKey(
	c *gin.Context,
	key string,
	settingService *service.SettingService,
	userRepo interface {
		GetFirstAdmin(ctx context.Context) (*model.User, error)
	},
) bool {
	storedKey, err := settingService.GetAdminApiKey(c.Request.Context())
	if err != nil {
		AbortWithError(c, 500, "INTERNAL_ERROR", "Internal server error")
		return false
	}

	// 未配置或不匹配，统一返回相同错误（避免信息泄露）
	if storedKey == "" || subtle.ConstantTimeCompare([]byte(key), []byte(storedKey)) != 1 {
		AbortWithError(c, 401, "INVALID_ADMIN_KEY", "Invalid admin API key")
		return false
	}

	// 获取真实的管理员用户
	admin, err := userRepo.GetFirstAdmin(c.Request.Context())
	if err != nil {
		AbortWithError(c, 500, "INTERNAL_ERROR", "No admin user found")
		return false
	}

	c.Set(string(ContextKeyUser), admin)
	c.Set("auth_method", "admin_api_key")
	return true
}

// validateJWTForAdmin 验证 JWT 并检查管理员权限
func validateJWTForAdmin(
	c *gin.Context,
	token string,
	authService *service.AuthService,
	userRepo interface {
		GetByID(ctx context.Context, id int64) (*model.User, error)
	},
) bool {
	// 验证 JWT token
	claims, err := authService.ValidateToken(token)
	if err != nil {
		if err == service.ErrTokenExpired {
			AbortWithError(c, 401, "TOKEN_EXPIRED", "Token has expired")
			return false
		}
		AbortWithError(c, 401, "INVALID_TOKEN", "Invalid token")
		return false
	}

	// 从数据库获取用户
	user, err := userRepo.GetByID(c.Request.Context(), claims.UserID)
	if err != nil {
		AbortWithError(c, 401, "USER_NOT_FOUND", "User not found")
		return false
	}

	// 检查用户状态
	if !user.IsActive() {
		AbortWithError(c, 401, "USER_INACTIVE", "User account is not active")
		return false
	}

	// 检查管理员权限
	if user.Role != model.RoleAdmin {
		AbortWithError(c, 403, "FORBIDDEN", "Admin access required")
		return false
	}

	c.Set(string(ContextKeyUser), user)

	return true
}

// extractAdminToken 支持 Bearer 与 Cookie 两种方式，便于后台统一鉴权。
func extractAdminToken(c *gin.Context, authService *service.AuthService) (string, bool, error) {
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
