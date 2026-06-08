package middleware

import (
	"net/http"

	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/gin-gonic/gin"
)

// ReadOnlyAdminGuard lets read-only admins inspect admin data but blocks writes.
func ReadOnlyAdminGuard() gin.HandlerFunc {
	return func(c *gin.Context) {
		role, ok := GetUserRoleFromContext(c)
		if !ok || role != service.RoleReadonly {
			c.Next()
			return
		}

		switch c.Request.Method {
		case http.MethodGet, http.MethodHead, http.MethodOptions:
			c.Next()
		default:
			AbortWithError(c, http.StatusForbidden, "READONLY_FORBIDDEN", "Read-only administrators cannot modify data")
		}
	}
}
