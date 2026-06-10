package middleware

import (
	"net/http"
	"regexp"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

var adminUsageViewerPathRules = []struct {
	method string
	path   *regexp.Regexp
}{
	{method: http.MethodGet, path: regexp.MustCompile(`^/api/v1/admin/compliance$`)},
	{method: http.MethodPost, path: regexp.MustCompile(`^/api/v1/admin/compliance/accept$`)},
	{method: http.MethodGet, path: regexp.MustCompile(`^/api/v1/admin/accounts$`)},
	{method: http.MethodGet, path: regexp.MustCompile(`^/api/v1/admin/accounts/usage-viewer-summary$`)},
	{method: http.MethodGet, path: regexp.MustCompile(`^/api/v1/admin/accounts/[0-9]+$`)},
	{method: http.MethodGet, path: regexp.MustCompile(`^/api/v1/admin/accounts/[0-9]+/usage$`)},
	{method: http.MethodGet, path: regexp.MustCompile(`^/api/v1/admin/accounts/[0-9]+/today-stats$`)},
	{method: http.MethodPost, path: regexp.MustCompile(`^/api/v1/admin/accounts/today-stats/batch$`)},
}

// AdminUsageViewerScope limits usage_viewer accounts to account usage read endpoints.
// Full administrators and admin API key requests continue to have unrestricted admin access.
func AdminUsageViewerScope() gin.HandlerFunc {
	return func(c *gin.Context) {
		role, ok := GetUserRoleFromContext(c)
		if !ok {
			AbortWithError(c, http.StatusUnauthorized, "UNAUTHORIZED", "User not found in context")
			return
		}
		if role == service.RoleAdmin {
			c.Next()
			return
		}
		if role != service.RoleUsageViewer {
			AbortWithError(c, http.StatusForbidden, "FORBIDDEN", "Admin access required")
			return
		}
		if isAdminUsageViewerAllowed(c.Request.Method, c.Request.URL.Path) {
			c.Next()
			return
		}
		AbortWithError(c, http.StatusForbidden, "FORBIDDEN", "Usage viewer access is limited to account usage")
	}
}

func isAdminUsageViewerAllowed(method string, path string) bool {
	for _, rule := range adminUsageViewerPathRules {
		if method == rule.method && rule.path.MatchString(path) {
			return true
		}
	}
	return false
}
