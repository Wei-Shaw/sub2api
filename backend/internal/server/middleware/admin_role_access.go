package middleware

import (
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

// AdminRoleAccessMiddleware applies route-level permissions after adminAuth has
// authenticated an admin-console principal. Full admins keep unrestricted access;
// marketing users get a constrained sales/support surface.
func AdminRoleAccessMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		role, ok := GetUserRoleFromContext(c)
		if !ok || role == "" {
			AbortWithError(c, 401, "UNAUTHORIZED", "Authorization required")
			return
		}

		if role == service.RoleAdmin {
			c.Next()
			return
		}

		if role != service.RoleMarketing {
			AbortWithError(c, 403, "FORBIDDEN", "Admin access required")
			return
		}

		if isMarketingAdminPathAllowed(c.Request.Method, c.Request.URL.Path) {
			c.Next()
			return
		}

		AbortWithError(c, 403, "FORBIDDEN", "Marketing role cannot access this admin resource")
	}
}

func isMarketingAdminPathAllowed(method, requestPath string) bool {
	path := normalizeAdminPath(requestPath)
	if path == "" {
		return false
	}

	if method == "GET" {
		return hasAnyPrefix(path,
			"/admin/dashboard",
			"/admin/ops",
			"/admin/users",
			"/admin/groups/", // group-scoped subscription/support lookups
			"/admin/subscriptions",
			"/admin/usage",
			"/admin/payment/dashboard",
			"/admin/payment/config",
			"/admin/payment/orders",
			"/admin/payment/plans",
			"/admin/payment/providers",
			"/admin/redeem-codes",
			"/admin/promo-codes",
			"/admin/affiliates",
		)
	}

	// Dashboard batch endpoints are read-only aggregation queries despite using POST.
	if method == "POST" && (path == "/admin/dashboard/users-usage" || path == "/admin/dashboard/api-keys-usage") {
		return true
	}

	// Subscription administration is the primary marketing operation: grant,
	// extend, reset, or revoke package access for customer-care workflows.
	if hasAnyPrefix(path, "/admin/subscriptions") {
		switch method {
		case "POST", "DELETE":
			return true
		}
	}

	// Voucher/coupon campaigns directly support selling token/packages.
	if hasAnyPrefix(path, "/admin/redeem-codes", "/admin/promo-codes") {
		switch method {
		case "POST", "PUT", "DELETE":
			return true
		}
	}

	// Marketing owns packaging/pricing operations, but not payment provider/config
	// credentials. Order support actions are needed for customer-care workflows.
	if hasAnyPrefix(path, "/admin/payment/plans") {
		switch method {
		case "POST", "PUT", "DELETE":
			return true
		}
	}
	if method == "POST" && hasAnyPrefix(path, "/admin/payment/orders") {
		return true
	}

	return false
}

func normalizeAdminPath(requestPath string) string {
	idx := strings.Index(requestPath, "/admin")
	if idx < 0 {
		return ""
	}
	path := requestPath[idx:]
	if path == "" {
		return ""
	}
	return strings.TrimRight(path, "/")
}

func hasAnyPrefix(path string, prefixes ...string) bool {
	for _, prefix := range prefixes {
		prefix = strings.TrimRight(prefix, "/")
		if path == prefix || strings.HasPrefix(path, prefix+"/") {
			return true
		}
	}
	return false
}
