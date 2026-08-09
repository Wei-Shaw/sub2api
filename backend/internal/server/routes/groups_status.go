package routes

import (
	"github.com/Wei-Shaw/sub2api/internal/handler"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"

	"github.com/gin-gonic/gin"
)

// RegisterGroupsStatusRoutes registers the public aggregate status endpoint.
// It is intentionally anonymous: the response contains only non-exclusive
// groups and aggregate account counts.
func RegisterGroupsStatusRoutes(
	v1 *gin.RouterGroup,
	h *handler.Handlers,
	panelRateLimiter *middleware.PanelRateLimiter,
) {
	status := v1.Group("/groups-status")
	status.Use(panelRateLimiter.PublicIP())
	status.GET("", h.GroupsStatus.Get)
}
