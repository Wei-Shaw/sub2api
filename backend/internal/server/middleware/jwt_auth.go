package middleware

import (
	"context"
	"errors"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/gin-gonic/gin"
)

// NewJWTAuthMiddleware 创建 JWT 认证中间件
func NewJWTAuthMiddleware(
	authService *service.AuthService,
	userService *service.UserService,
	settingService *service.SettingService,
	auditService *service.AuditLogService,
	systemTokenService *service.SystemTokenService,
) JWTAuthMiddleware {
	return JWTAuthMiddleware(jwtAuth(authService, userService, userService, settingService, auditService, systemTokenService))
}

type jwtUserReader interface {
	GetByID(ctx context.Context, id int64) (*service.User, error)
}

type userActivityToucher interface {
	TouchLastActiveForUser(ctx context.Context, user *service.User)
}

// jwtAuth JWT认证中间件实现
func jwtAuth(
	authService *service.AuthService,
	userService jwtUserReader,
	activityToucher userActivityToucher,
	settingService *service.SettingService,
	auditService *service.AuditLogService,
	systemTokenService *service.SystemTokenService,
) gin.HandlerFunc {
	return func(c *gin.Context) {
		// 从Authorization header中提取token
		authHeader := c.GetHeader("Authorization")
		if authHeader == "" {
			AbortWithError(c, 401, "UNAUTHORIZED", "Authorization header is required")
			return
		}

		// 验证Bearer scheme
		parts := strings.SplitN(authHeader, " ", 2)
		if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
			AbortWithError(c, 401, "INVALID_AUTH_HEADER", "Authorization header format must be 'Bearer {token}'")
			return
		}

		tokenString := strings.TrimSpace(parts[1])
		if tokenString == "" {
			AbortWithError(c, 401, "EMPTY_TOKEN", "Token cannot be empty")
			return
		}

		// 系统访问令牌 (System Access Token) 快捷路径
		if service.IsSystemToken(tokenString) {
			if !service.IsValidSystemTokenFormat(tokenString) {
				AbortWithError(c, 401, "INVALID_SYSTEM_TOKEN", "Invalid system access token")
				return
			}
			authenticateWithSystemToken(c, systemTokenService, userService, activityToucher, tokenString)
			return
		}

		// 验证token
		claims, err := authService.ValidateToken(tokenString)
		if err != nil {
			if errors.Is(err, service.ErrTokenExpired) {
				AbortWithError(c, 401, "TOKEN_EXPIRED", "Token has expired")
				return
			}
			AbortWithError(c, 401, "INVALID_TOKEN", "Invalid token")
			return
		}

		// 从数据库获取最新的用户信息
		user, err := userService.GetByID(c.Request.Context(), claims.UserID)
		if err != nil {
			AbortWithError(c, 401, "USER_NOT_FOUND", "User not found")
			return
		}

		// 检查用户状态
		if !user.IsActive() {
			AbortWithError(c, 401, "USER_INACTIVE", "User account is not active")
			return
		}

		// Security: Validate TokenVersion to ensure token hasn't been invalidated
		// This check ensures tokens issued before a password change are rejected
		if claims.TokenVersion != user.TokenVersion {
			AbortWithError(c, 401, "TOKEN_REVOKED", "Token has been revoked (password changed)")
			return
		}

		// 会话绑定校验：IP/UA 任一变化即撤销会话（功能可在系统设置中关闭）
		if !enforceSessionBinding(c, authService, settingService, auditService, claims) {
			return
		}

		c.Set(string(ContextKeyUser), AuthSubject{
			UserID:      user.ID,
			Concurrency: user.Concurrency,
		})
		c.Set(string(ContextKeyUserRole), user.Role)
		c.Set(ContextKeyAuthEmail, user.Email)
		c.Set(ContextKeySessionID, claims.SessionID)
		if activityToucher != nil {
			activityToucher.TouchLastActiveForUser(c.Request.Context(), user)
		}

		c.Next()
	}
}

// authenticateWithSystemToken validates a system access token and sets auth context.
func authenticateWithSystemToken(
	c *gin.Context,
	systemTokenService *service.SystemTokenService,
	userService jwtUserReader,
	activityToucher userActivityToucher,
	token string,
) {
	userID, err := systemTokenService.GetUserIDByToken(c.Request.Context(), token)
	if err != nil {
		AbortWithError(c, 401, "INVALID_SYSTEM_TOKEN", "Invalid system access token")
		return
	}

	user, err := userService.GetByID(c.Request.Context(), userID)
	if err != nil {
		AbortWithError(c, 401, "USER_NOT_FOUND", "User not found")
		return
	}
	if !user.IsActive() {
		AbortWithError(c, 401, "USER_INACTIVE", "User account is not active")
		return
	}

	c.Set(string(ContextKeyUser), AuthSubject{
		UserID:      user.ID,
		Concurrency: user.Concurrency,
	})
	c.Set(string(ContextKeyUserRole), user.Role)
	c.Set(ContextKeyAuthEmail, user.Email)
	c.Set("auth_method", service.AuditAuthMethodSystemToken)
	if activityToucher != nil {
		activityToucher.TouchLastActiveForUser(c.Request.Context(), user)
	}
	c.Next()
}

// Deprecated: prefer GetAuthSubjectFromContext in auth_subject.go.
