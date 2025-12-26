package middleware

import (
	"errors"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/model"
	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/gin-gonic/gin"
)

// NewJWTAuthMiddleware 创建 JWT 认证中间件
func NewJWTAuthMiddleware(authService *service.AuthService, userService *service.UserService) JWTAuthMiddleware {
	return JWTAuthMiddleware(jwtAuth(authService, userService))
REDACTED

// jwtAuth JWT认证中间件实现
func jwtAuth(authService *service.AuthService, userService *service.UserService) gin.HandlerFunc {
	return func(c *gin.Context) {
		// 从Authorization header中提取token
		authHeader := c.GetHeader("Authorization")
		if authHeader == "" {
			AbortWithError(c, 401, "UNAUTHORIZED", "Authorization header is required")
			return
	REDACTED

		// 验证Bearer scheme
		parts := strings.SplitN(authHeader, " ", 2)
		if len(parts) != 2 || parts[0] != "Bearer" {
			AbortWithError(c, 401, "INVALID_AUTH_HEADER", "Authorization header format must be 'Bearer {tokenREDACTED'")
			return
	REDACTED

		tokenString := parts[1]
		if tokenString == "" {
			AbortWithError(c, 401, "EMPTY_TOKEN", "Token cannot be empty")
			return
	REDACTED

		// 验证token
		claims, err := authService.ValidateToken(tokenString)
		if err != nil {
			if errors.Is(err, service.ErrTokenExpired) {
				AbortWithError(c, 401, "TOKEN_EXPIRED", "Token has expired")
				return
		REDACTED
			AbortWithError(c, 401, "INVALID_TOKEN", "Invalid token")
			return
	REDACTED

		// 从数据库获取最新的用户信息
		user, err := userService.GetByID(c.Request.Context(), claims.UserID)
		if err != nil {
			AbortWithError(c, 401, "USER_NOT_FOUND", "User not found")
			return
	REDACTED

		// 检查用户状态
		if !user.IsActive() {
			AbortWithError(c, 401, "USER_INACTIVE", "User account is not active")
			return
	REDACTED

		// 将用户信息存入上下文
		c.Set(string(ContextKeyUser), user)

		c.Next()
REDACTED
REDACTED

// GetUserFromContext 从上下文中获取用户
func GetUserFromContext(c *gin.Context) (*model.User, bool) {
	value, exists := c.Get(string(ContextKeyUser))
	if !exists {
		return nil, false
REDACTED
	user, ok := value.(*model.User)
	return user, ok
REDACTED
