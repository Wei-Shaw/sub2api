package routes

import (
	"github.com/gin-gonic/gin"

	"github.com/Wei-Shaw/sub2api/internal/handler"
)

// RegisterOidcProviderRoutes 注册 OIDC Provider 对外端点 (挂在根 router，无前缀)。
//
// 路由与 design.md D11 对齐：
//
//	GET  /.well-known/openid-configuration
//	GET  /.well-known/jwks.json
//	GET  /oidc/authorize
//	POST /oidc/token
//	GET  /oidc/userinfo
//	GET  /oidc/consent
//	POST /oidc/consent
//	GET  /oidc/resource/api-keys      (Bearer + scope sub2api:apikey)
//
// 当 oidc_provider.enabled=false 时，各 handler 内部统一返回 404。
func RegisterOidcProviderRoutes(r *gin.Engine, h *handler.Handlers) {
	if h == nil || h.OidcProvider == nil {
		return
	}
	op := h.OidcProvider

	r.GET("/.well-known/openid-configuration", op.Discovery)
	r.GET("/.well-known/jwks.json", op.JWKS)

	oidc := r.Group("/oidc")
	{
		oidc.GET("/authorize", op.Authorize)
		oidc.POST("/token", op.Token)
		oidc.GET("/userinfo", op.UserInfo)
		oidc.POST("/userinfo", op.UserInfo)
		oidc.GET("/consent", op.ConsentGet)
		oidc.POST("/consent", op.ConsentPost)

		// 受保护资源端点：使用 OIDC access_token 鉴权，scope 控制可用性。
		resource := oidc.Group("/resource")
		{
			resource.GET("/api-keys", op.ListAPIKeys)
		}
	}
}
