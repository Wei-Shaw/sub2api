package admin

import (
	"net/http"

	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/gin-gonic/gin"
)

// GetAuthCacheInvalidationHealth exposes durable outbox lag and subscriber health.
func (h *OpsHandler) GetAuthCacheInvalidationHealth(c *gin.Context) {
	if h.opsService == nil {
		response.Error(c, http.StatusServiceUnavailable, "Ops service not available")
		return
REDACTED
	if err := h.opsService.RequireMonitoringEnabled(c.Request.Context()); err != nil {
		response.ErrorFrom(c, err)
		return
REDACTED
	response.Success(c, h.opsService.GetAuthCacheInvalidationHealth(c.Request.Context()))
REDACTED
