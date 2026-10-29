package routes

import (
	"github.com/Wei-Shaw/sub2api/internal/handler"
	servermiddleware "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/gin-gonic/gin"
)

// RegisterOAuthProviderRoutes registers the OAuth authorization server control plane.
// Resource endpoints continue to be registered by the existing user routes,
// where the OAuth-aware middleware applies the resource scopes.
func RegisterOAuthProviderRoutes(v1 *gin.RouterGroup, h *handler.Handlers, jwtAuth servermiddleware.JWTAuthMiddleware) {
	if h == nil || h.OAuth == nil {
		return
	}
	oauth := v1.Group("/oauth")
	{
		oauth.GET("/.well-known", h.OAuth.Discovery)
		oauth.GET("/authorize", h.OAuth.Authorize)
		oauth.GET("/authorize/transaction/:id", h.OAuth.AuthorizationTransaction)
		oauth.POST("/authorize/approve", gin.HandlerFunc(jwtAuth), h.OAuth.Approve)
		oauth.POST("/token", h.OAuth.Token)
		oauth.POST("/revoke", h.OAuth.Revoke)
		oauth.GET("/grants/current", gin.HandlerFunc(jwtAuth), h.OAuth.GrantsCurrent)
		oauth.GET("/grants", gin.HandlerFunc(jwtAuth), h.OAuth.GrantsList)
		oauth.DELETE("/grants/:clientId", gin.HandlerFunc(jwtAuth), h.OAuth.RevokeGrantByID)
	}
}
