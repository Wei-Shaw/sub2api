package routes

import (
	"github.com/Wei-Shaw/sub2api/internal/handler"

	"github.com/gin-gonic/gin"
)

// RegisterProviderPricingRoutes registers public provider integration endpoints.
func RegisterProviderPricingRoutes(r *gin.Engine, h *handler.Handlers) {
	if h == nil || h.ProviderPricing == nil {
		return
	}
	r.GET("/api/provider/pricing", h.ProviderPricing.Get)
}
