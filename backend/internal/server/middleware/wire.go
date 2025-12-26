package middleware

import (
	"github.com/gin-gonic/gin"
	"github.com/google/wire"
)

// JWTAuthMiddleware JWT 认证中间件类型
type JWTAuthMiddleware gin.HandlerFunc

// AdminAuthMiddleware 管理员认证中间件类型
type AdminAuthMiddleware gin.HandlerFunc

// ApiKeyAuthMiddleware API Key 认证中间件类型
type ApiKeyAuthMiddleware gin.HandlerFunc

// ProviderSet 中间件层的依赖注入
var ProviderSet = wire.NewSet(
	NewJWTAuthMiddleware,
	NewAdminAuthMiddleware,
	NewApiKeyAuthMiddleware,
)
