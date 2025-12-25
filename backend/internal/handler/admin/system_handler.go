package admin

import (
	"net/http"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/pkg/sysutil"
	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/gin-gonic/gin"
)

// SystemHandler handles system-related operations
type SystemHandler struct {
	updateSvc *service.UpdateService
}

// NewSystemHandler creates a new SystemHandler
func NewSystemHandler(updateSvc *service.UpdateService) *SystemHandler {
	return &SystemHandler{
		updateSvc: updateSvc,
	}
}

// GetVersion returns the current version
// GET /api/v1/admin/system/version
func (h *SystemHandler) GetVersion(c *gin.Context) {
	info, _ := h.updateSvc.CheckUpdate(c.Request.Context(), false)
	response.Success(c, gin.H{
		"version": info.CurrentVersion,
	})
}

// CheckUpdates checks for available updates
// GET /api/v1/admin/system/check-updates
func (h *SystemHandler) CheckUpdates(c *gin.Context) {
	force := c.Query("force") == "true"
	info, err := h.updateSvc.CheckUpdate(c.Request.Context(), force)
	if err != nil {
		response.Error(c, http.StatusInternalServerError, err.Error())
		return
	}
	response.Success(c, info)
}

// PerformUpdate downloads and applies the update
// POST /api/v1/admin/system/update
func (h *SystemHandler) PerformUpdate(c *gin.Context) {
	if err := h.updateSvc.PerformUpdate(c.Request.Context()); err != nil {
		response.Error(c, http.StatusInternalServerError, err.Error())
		return
	}
	response.Success(c, gin.H{
		"message":      "Update completed. Please restart the service.",
		"need_restart": true,
	})
}

// Rollback restores the previous version
// POST /api/v1/admin/system/rollback
func (h *SystemHandler) Rollback(c *gin.Context) {
	if err := h.updateSvc.Rollback(); err != nil {
		response.Error(c, http.StatusInternalServerError, err.Error())
		return
	}
	response.Success(c, gin.H{
		"message":      "Rollback completed. Please restart the service.",
		"need_restart": true,
	})
}

// RestartService restarts the systemd service
// POST /api/v1/admin/system/restart
func (h *SystemHandler) RestartService(c *gin.Context) {
	// Schedule service restart in background after sending response
	// This ensures the client receives the success response before the service restarts
	go func() {
		// Wait a moment to ensure the response is sent
		time.Sleep(500 * time.Millisecond)
		sysutil.RestartServiceAsync()
	}()

	response.Success(c, gin.H{
		"message": "Service restart initiated",
	})
}
