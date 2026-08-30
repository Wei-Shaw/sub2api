package routes

import (
	"testing"

	basehandler "github.com/Wei-Shaw/sub2api/internal/handler"
	adminhandler "github.com/Wei-Shaw/sub2api/internal/handler/admin"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestRegisterDisplayPricingRoutesIncludesProviderCRUD(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	admin := router.Group("/api/v1/admin")
	handlers := &basehandler.Handlers{Admin: &basehandler.AdminHandlers{
		DisplayPricing: adminhandler.NewDisplayPricingHandler(nil, nil),
	}}
	registerDisplayPricingRoutes(admin, handlers)

	want := map[string]bool{
		"POST /api/v1/admin/display-pricing/providers":             false,
		"PUT /api/v1/admin/display-pricing/providers/:provider":    false,
		"DELETE /api/v1/admin/display-pricing/providers/:provider": false,
		"POST /api/v1/admin/display-pricing/official-sync/preview": false,
		"POST /api/v1/admin/display-pricing/official-sync/apply":   false,
	}
	for _, route := range router.Routes() {
		key := route.Method + " " + route.Path
		if _, ok := want[key]; ok {
			want[key] = true
		}
	}
	for route, registered := range want {
		require.True(t, registered, route)
	}
}
