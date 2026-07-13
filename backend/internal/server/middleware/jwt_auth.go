package middleware

import (
	"context"
	"errors"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/gin-gonic/gin"
)

// NewJWTAuthMiddleware 创建 JWT 认证中间件
func NewJWTAuthMiddleware(authService *service.AuthService, userService *service.UserService) JWTAuthMiddleware {
	return JWTAuthMiddleware(jwtAuth(authService, userService, userService))
}

// NewOptionalJWTAuthMiddleware 创建"可选"JWT 认证中间件。
//
// 与 NewJWTAuthMiddleware 的区别：缺/坏/过期 token 不会 401 abort，
// 而是直接放行进入下一个 handler——只是不在 gin.Context 里写 AuthSubject。
//
// 适用场景：路由本身允许匿名访问、但 handler 需要"如果带了 token 就识别用户"
// 来做差异化逻辑（限流 key 选 user 还是 IP / 是否允许匿名 LLM 等）。
//
// 当前唯一调用方：POST /api/v1/support/chat（客服浮窗 SSE，匿名 / 登录都可走，
// 由 handler 内部按 settings.support_chat_anonymous_llm 决定）。**没有这个中间件**
// 时，该路由因为不挂强 jwtAuth、又没有别的途径写 AuthSubject，导致**已登录用户**
// 也被识别为匿名，AnonymousLLM=false 时一律 401，这就是 "明明登录了还提示没登录" 的根因。
func NewOptionalJWTAuthMiddleware(authService *service.AuthService, userService *service.UserService) OptionalJWTAuthMiddleware {
	return OptionalJWTAuthMiddleware(optionalJWTAuth(authService, userService, userService))
}

type jwtUserReader interface {
	GetByID(ctx context.Context, id int64) (*service.User, error)
}

type userActivityToucher interface {
	TouchLastActiveForUser(ctx context.Context, user *service.User)
}

// jwtAuth JWT认证中间件实现
func jwtAuth(authService *service.AuthService, userService jwtUserReader, activityToucher userActivityToucher) gin.HandlerFunc {
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

		c.Set(string(ContextKeyUser), AuthSubject{
			UserID:      user.ID,
			Concurrency: user.Concurrency,
		})
		c.Set(string(ContextKeyUserRole), user.Role)
		if activityToucher != nil {
			activityToucher.TouchLastActiveForUser(c.Request.Context(), user)
		}

		c.Next()
	}
}

// optionalJWTAuth 与 jwtAuth 的"宽松版"：所有失败路径都静默放行，仅在 token
// 解析全部成功且用户活跃且 TokenVersion 一致时才把 AuthSubject 写进 context。
//
// 与 jwtAuth 的字段写入完全一致（ContextKeyUser / ContextKeyUserRole +
// TouchLastActiveForUser），保证调用方 GetAuthSubjectFromContext / GetUserRoleFromContext
// 在登录态下行为与强 jwtAuth 等价。
//
// 失败原因（缺 header / 格式错 / token 过期 / 用户不存在 / inactive / TokenVersion
// 不匹配）都不打 log——这是 anonymous-allowed 路由的常态，打 log 会被无登录态访客刷屏。
// 如以后需要审计，再补 metric 计数即可。
func optionalJWTAuth(authService *service.AuthService, userService jwtUserReader, activityToucher userActivityToucher) gin.HandlerFunc {
	return func(c *gin.Context) {
		authHeader := c.GetHeader("Authorization")
		if authHeader == "" {
			c.Next()
			return
		}
		parts := strings.SplitN(authHeader, " ", 2)
		if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
			c.Next()
			return
		}
		tokenString := strings.TrimSpace(parts[1])
		if tokenString == "" {
			c.Next()
			return
		}
		claims, err := authService.ValidateToken(tokenString)
		if err != nil {
			c.Next()
			return
		}
		user, err := userService.GetByID(c.Request.Context(), claims.UserID)
		if err != nil {
			c.Next()
			return
		}
		if !user.IsActive() {
			c.Next()
			return
		}
		if claims.TokenVersion != user.TokenVersion {
			c.Next()
			return
		}
		c.Set(string(ContextKeyUser), AuthSubject{
			UserID:      user.ID,
			Concurrency: user.Concurrency,
		})
		c.Set(string(ContextKeyUserRole), user.Role)
		if activityToucher != nil {
			activityToucher.TouchLastActiveForUser(c.Request.Context(), user)
		}
		c.Next()
	}
}

// Deprecated: prefer GetAuthSubjectFromContext in auth_subject.go.
