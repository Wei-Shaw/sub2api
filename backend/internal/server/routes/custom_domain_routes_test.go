//go:build unit

package routes

import (
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/handler"
	adminhandler "github.com/Wei-Shaw/sub2api/internal/handler/admin"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestCustomDomainRoutesAreRegistered(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	handlers := &handler.Handlers{
		CustomDomain: handler.NewCustomDomainHandler(nil),
		Admin: &handler.AdminHandlers{
			CustomDomain: adminhandler.NewCustomDomainHandler(nil),
		},
	}
	pass := func(c *gin.Context) { c.Next() }

	RegisterUserRoutes(
		router.Group("/api/v1"),
		handlers,
		middleware.JWTAuthMiddleware(pass),
		middleware.AuditLogMiddleware(pass),
		nil,
	)
	RegisterAdminRoutes(
		router.Group("/api/v1"),
		handlers,
		middleware.AdminAuthMiddleware(pass),
		middleware.AuditLogMiddleware(pass),
		middleware.StepUpAuthMiddleware(pass),
		nil,
	)

	routes := make(map[string]struct{})
	for _, route := range router.Routes() {
		routes[route.Method+" "+route.Path] = struct{}{}
	}
	for _, expected := range []string{
		"GET /api/v1/custom-domains",
		"POST /api/v1/custom-domains",
		"POST /api/v1/custom-domains/:id/verify",
		"DELETE /api/v1/custom-domains/:id",
		"GET /api/v1/admin/custom-domains/config",
		"PUT /api/v1/admin/custom-domains/config",
		"GET /api/v1/admin/custom-domains",
		"POST /api/v1/admin/custom-domains",
		"PUT /api/v1/admin/custom-domains/:id/access",
		"POST /api/v1/admin/custom-domains/:id/verify",
		"POST /api/v1/admin/custom-domains/:id/disable",
		"POST /api/v1/admin/custom-domains/:id/enable",
		"DELETE /api/v1/admin/custom-domains/:id",
	} {
		require.Contains(t, routes, expected)
	}
}
