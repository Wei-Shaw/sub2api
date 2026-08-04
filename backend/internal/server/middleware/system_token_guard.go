package middleware

import (
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

type routeKey struct {
	method string
	path   string
}

// systemTokenAllowedRoutes is the allowlist of endpoints accessible via system access token.
// Anything not listed is rejected with 403 — safe default when upstream adds new routes.
var systemTokenAllowedRoutes = map[routeKey]bool{
	// Profile (read-only)
	{"GET", "/api/v1/user/profile"}:                  true,
	{"GET", "/api/v1/user/aff"}:                      true,
	{"GET", "/api/v1/user/platform-quotas"}:           true,
	{"GET", "/api/v1/user/api-keys/:id/usage/daily"}:  true,

	// System token self-management (generate blocked by password check in handler)
	{"GET", "/api/v1/user/system-token"}:    true,
	{"POST", "/api/v1/user/system-token"}:   true,
	{"DELETE", "/api/v1/user/system-token"}: true,

	// API Key CRUD
	{"GET", "/api/v1/keys"}:        true,
	{"GET", "/api/v1/keys/:id"}:    true,
	{"POST", "/api/v1/keys"}:       true,
	{"PUT", "/api/v1/keys/:id"}:    true,
	{"DELETE", "/api/v1/keys/:id"}: true,

	// Groups & channels (read-only)
	{"GET", "/api/v1/groups/available"}:   true,
	{"GET", "/api/v1/groups/rates"}:       true,
	{"GET", "/api/v1/channels/available"}: true,

	// Usage (read-only)
	{"GET", "/api/v1/usage"}:                          true,
	{"GET", "/api/v1/usage/:id"}:                      true,
	{"GET", "/api/v1/usage/errors"}:                   true,
	{"GET", "/api/v1/usage/errors/:id"}:                true,
	{"GET", "/api/v1/usage/stats"}:                    true,
	{"GET", "/api/v1/usage/dashboard/stats"}:           true,
	{"GET", "/api/v1/usage/dashboard/trend"}:           true,
	{"GET", "/api/v1/usage/dashboard/models"}:          true,
	{"GET", "/api/v1/usage/dashboard/snapshot-v2"}:     true,
	{"POST", "/api/v1/usage/dashboard/api-keys-usage"}: true,

	// Announcements
	{"GET", "/api/v1/announcements"}:            true,
	{"POST", "/api/v1/announcements/:id/read"}:  true,

	// Subscriptions (read-only)
	{"GET", "/api/v1/subscriptions"}:          true,
	{"GET", "/api/v1/subscriptions/active"}:   true,
	{"GET", "/api/v1/subscriptions/progress"}: true,
	{"GET", "/api/v1/subscriptions/summary"}:  true,

	// Channel monitors (read-only)
	{"GET", "/api/v1/channel-monitors"}:            true,
	{"GET", "/api/v1/channel-monitors/:id/status"}: true,

	// Redeem history (read-only, not redeem itself)
	{"GET", "/api/v1/redeem/history"}: true,

	// Model plaza (registered under OptionalJWT, included for completeness)
	{"GET", "/api/v1/model-plaza"}: true,
}

// SystemTokenRouteGuard blocks system access tokens from sensitive endpoints.
// JWT and other auth methods pass through unaffected.
func SystemTokenRouteGuard() gin.HandlerFunc {
	return func(c *gin.Context) {
		if c.GetString("auth_method") != service.AuditAuthMethodSystemToken {
			c.Next()
			return
		}
		k := routeKey{c.Request.Method, c.FullPath()}
		if systemTokenAllowedRoutes[k] {
			c.Next()
			return
		}
		AbortWithError(c, 403, "SYSTEM_TOKEN_FORBIDDEN",
			"This endpoint is not accessible with a system access token")
	}
}
