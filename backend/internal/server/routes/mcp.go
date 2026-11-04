package routes

import (
	"time"

	"github.com/Wei-Shaw/sub2api/internal/handler"
	"github.com/Wei-Shaw/sub2api/internal/managementmcp"
	ratelimit "github.com/Wei-Shaw/sub2api/internal/middleware"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
)

// MCP limits fail closed and count an IPv6 /64 as one client, so rotating
// addresses within one allocation does not multiply the budget.
var mcpRateLimitOptions = ratelimit.RateLimitOptions{FailureMode: ratelimit.RateLimitFailClose, IPv6PrefixBits: 64}

// RegisterMCPRoutes exposes MCP over HTTP and reuses the normal management
// routes in a private router, preserving ownership checks, guards and audit.
func RegisterMCPRoutes(v1 *gin.RouterGroup, h *handler.Handlers, audit middleware.AuditLogMiddleware,
	stepUp middleware.StepUpAuthMiddleware, settings *service.SettingService,
	limiter *middleware.PanelRateLimiter, allowedOrigins []string, redisClient *redis.Client,
) {
	inner := gin.New()
	inner.Use(gin.Recovery())
	api := inner.Group("/api/v1")
	RegisterUserRoutes(api, h, middleware.JWTAuthMiddleware(managementmcp.DispatchAuth(false)), audit, settings, limiter)
	RegisterAdminRoutes(api, h, middleware.AdminAuthMiddleware(managementmcp.DispatchAuth(true)), audit, stepUp, settings, limiter)
	transport := managementmcp.NewHandler(inner, allowedOrigins)
	var handlers []gin.HandlerFunc
	if redisClient != nil {
		// Bound token probes per network before authentication reaches the database.
		handlers = append(handlers, ratelimit.NewRateLimiter(redisClient).LimitWithOptions("mcp-transport", 600, time.Minute, mcpRateLimitOptions))
	}
	handlers = append(handlers,
		managementmcp.Authenticate(h.MCPOAuth.Service(), allowedOrigins, h.MCPOAuth.ResourceURL),
		limiter.MCPTransport(), middleware.BackendModeUserGuard(settings))
	mcp := v1.Group("/mcp", handlers...)
	for _, method := range []string{"POST", "GET", "DELETE"} {
		mcp.Handle(method, "", gin.WrapH(transport))
	}
}

// RegisterMCPOAuthRoutes exposes OAuth discovery and public-client protocol
// endpoints. Browser consent and grant management live in the JWT user router.
func RegisterMCPOAuthRoutes(r *gin.Engine, v1 *gin.RouterGroup, h *handler.Handlers, redisClient *redis.Client) {
	r.GET("/.well-known/oauth-protected-resource/api/v1/mcp", h.MCPOAuth.ResourceMetadata)
	r.GET("/.well-known/oauth-protected-resource", h.MCPOAuth.ResourceMetadata)
	r.GET("/.well-known/oauth-authorization-server", h.MCPOAuth.AuthorizationMetadata)
	limit := ratelimit.NewRateLimiter(redisClient)
	oauth := v1.Group("/mcp/oauth")
	oauth.POST("/register", limit.LimitWithOptions("mcp-oauth-register", 10, time.Minute, mcpRateLimitOptions), h.MCPOAuth.Register)
	oauth.POST("/token", limit.LimitWithOptions("mcp-oauth-token", 60, time.Minute, mcpRateLimitOptions), h.MCPOAuth.Token)
	oauth.POST("/revoke", limit.LimitWithOptions("mcp-oauth-revoke", 60, time.Minute, mcpRateLimitOptions), h.MCPOAuth.RevokeToken)
}
