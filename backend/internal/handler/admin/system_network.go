package admin

import (
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/gin-gonic/gin"
)

// GetNetworkTraffic is registered only under the authenticated admin group.
func (h *SystemHandler) GetNetworkTraffic(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	response.Success(c, h.networkStats.Snapshot())
}
