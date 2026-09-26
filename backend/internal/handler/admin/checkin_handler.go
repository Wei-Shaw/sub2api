package admin

import (
	"strconv"

	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/gin-gonic/gin"
)

type CheckInHandler struct {
	service *service.CheckInService
}

func NewCheckInHandler(checkInService *service.CheckInService) *CheckInHandler {
	return &CheckInHandler{service: checkInService}
}

// GetStats returns the check-in summary and daily trend.
// GET /api/v1/admin/checkin/stats
func (h *CheckInHandler) GetStats(c *gin.Context) {
	days := 30
	if raw := c.Query("days"); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil {
			days = parsed
		}
	}
	stats, err := h.service.GetAdminStats(c.Request.Context(), days)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, stats)
}

type UpdateCheckInEnabledRequest struct {
	Enabled *bool `json:"enabled" binding:"required"`
}

// SetEnabled turns user check-in on or off without changing history.
// PUT /api/v1/admin/checkin/enabled
func (h *CheckInHandler) SetEnabled(c *gin.Context) {
	var req UpdateCheckInEnabledRequest
	if err := c.ShouldBindJSON(&req); err != nil || req.Enabled == nil {
		response.BadRequest(c, "Invalid request")
		return
	}
	cfg, err := h.service.SetEnabled(c.Request.Context(), *req.Enabled)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, cfg)
}

// ResetAllCycles resets reward-tier progress but keeps balances and history.
// POST /api/v1/admin/checkin/reset-cycles
func (h *CheckInHandler) ResetAllCycles(c *gin.Context) {
	result, err := h.service.ResetAllCycles(c.Request.Context())
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, result)
}
