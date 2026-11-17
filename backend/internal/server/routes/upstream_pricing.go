package routes

import (
	"github.com/Wei-Shaw/sub2api/internal/handler"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/gin-gonic/gin"
)

// Called inside the existing admin authentication, audit and rate-limit group.
func registerUpstreamPricingRoutes(admin *gin.RouterGroup, h *handler.Handlers) {
	if h.Admin.UpstreamPricing == nil {
		return
	}
	p := admin.Group("/upstream-pricing/accounts/:id")
	v := h.Admin.UpstreamPricing
	admin.GET("/upstream-pricing/comparison", v.Comparison)
	p.GET("", v.Get)
	p.GET("/activity", v.Activity)
	p.POST("/browser-imports", v.StartBrowserImport)
	p.GET("/browser-imports/:importId", v.BrowserImportStatus)
	p.GET("/usage/:usageId/timing", v.Timing)
	p.POST("/groups", v.Groups)
	p.POST("/preview", v.Preview)
	p.POST("/apply", v.Apply)
	p.POST("/sync", v.Sync)
	p.PUT("/schedule", v.Schedule)
	p.POST("/rollback-preview", v.RollbackPreview)
}

func RegisterUpstreamBrowserRoutes(v1 *gin.RouterGroup, h *handler.Handlers, limiter *middleware.PanelRateLimiter) {
	if h.Admin.UpstreamPricing != nil {
		v1.GET("/upstream-pricing/browser-connector.user.js", limiter.PublicIP(), h.Admin.UpstreamPricing.BrowserConnector)
		v1.POST("/upstream-pricing/browser-import/:action", limiter.PublicIP(), h.Admin.UpstreamPricing.BrowserImportReceive)
	}
}
