//go:build unit

package routes

import (
	"net/http"
	"testing"

	userhandler "github.com/Wei-Shaw/sub2api/internal/handler"
	adminhandler "github.com/Wei-Shaw/sub2api/internal/handler/admin"
	servermiddleware "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestPaymentLegacyRoutesAreRegistered(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	v1 := router.Group("/api/v1")
	RegisterPaymentRoutes(
		v1,
		userhandler.NewPaymentHandler(nil, nil, nil),
		nil,
		adminhandler.NewPaymentHandler(nil, nil),
		servermiddleware.JWTAuthMiddleware(func(c *gin.Context) { c.Next() }),
		servermiddleware.AdminAuthMiddleware(func(c *gin.Context) { c.Next() }),
		nil,
	)

	expected := []struct {
		method string
		path   string
	}{
		{http.MethodGet, "/api/v1/subscription-plans"},
		{http.MethodGet, "/api/v1/admin/subscription-plans"},
		{http.MethodPost, "/api/v1/admin/subscription-plans"},
		{http.MethodPut, "/api/v1/admin/subscription-plans/:id"},
		{http.MethodDelete, "/api/v1/admin/subscription-plans/:id"},
	}
	for _, route := range expected {
		require.True(t, routeExists(router.Routes(), route.method, route.path), "%s %s", route.method, route.path)
	}
}

func TestSub2ApiAdminPaymentLegacyRoutesAreRegistered(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	admin := router.Group("/api/v1/admin")
	h := &userhandler.Handlers{Admin: &userhandler.AdminHandlers{
		Group:        adminhandler.NewGroupHandler(nil, nil, nil),
		User:         adminhandler.NewUserHandler(nil, nil, nil, nil, nil),
		Subscription: adminhandler.NewSubscriptionHandler(nil),
		Payment:      adminhandler.NewPaymentHandler(nil, nil),
	}}

	registerGroupRoutes(admin, h)

	expected := []struct {
		method string
		path   string
	}{
		{http.MethodGet, "/api/v1/admin/sub2api/dashboard"},
		{http.MethodGet, "/api/v1/admin/sub2api/orders"},
		{http.MethodGet, "/api/v1/admin/sub2api/orders/:id"},
		{http.MethodPost, "/api/v1/admin/sub2api/orders/:id/cancel"},
		{http.MethodPost, "/api/v1/admin/sub2api/orders/:id/retry"},
	}
	for _, route := range expected {
		require.True(t, routeExists(router.Routes(), route.method, route.path), "%s %s", route.method, route.path)
	}
}

func routeExists(routes gin.RoutesInfo, method string, path string) bool {
	for _, route := range routes {
		if route.Method == method && route.Path == path {
			return true
		}
	}
	return false
}
