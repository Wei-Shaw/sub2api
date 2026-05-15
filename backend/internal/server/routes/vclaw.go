package routes

import (
	"time"

	"github.com/Wei-Shaw/sub2api/internal/handler"
	"github.com/Wei-Shaw/sub2api/internal/middleware"

	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
)

// RegisterVClawRoutes registers the public V-Claw claim routes.
func RegisterVClawRoutes(v1 *gin.RouterGroup, h *handler.Handlers, redisClient *redis.Client) {
	rateLimiter := middleware.NewRateLimiter(redisClient)

	vclaw := v1.Group("/vclaw")
	{
		vclaw.POST("/claim", rateLimiter.LimitWithOptions("vclaw-claim", 5, time.Minute, middleware.RateLimitOptions{
			FailureMode: middleware.RateLimitFailClose,
		}), h.VClaw.Claim)
	}
}

// RegisterProviderCatalogRoutes registers the public provider-catalog route.
// This endpoint is served at the root level (not under /api/v1) because the
// v-claw-provider plugin expects it at {baseUrl}/provider-catalog.
func RegisterProviderCatalogRoutes(r *gin.Engine, h *handler.Handlers) {
	if h.ProviderCatalog != nil {
		r.GET("/provider-catalog", h.ProviderCatalog.GetCatalog)
	}
}
