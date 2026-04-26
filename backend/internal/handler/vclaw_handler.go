package handler

import (
	"log/slog"

	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/gin-gonic/gin"
)

// VClawHandler handles the V-Claw machine claim flow.
type VClawHandler struct {
	claimService *service.VClawClaimService
}

// NewVClawHandler creates a new VClawHandler.
func NewVClawHandler(claimService *service.VClawClaimService) *VClawHandler {
	return &VClawHandler{claimService: claimService}
}

// VClawClaimRequest is the public request body for the machine claim endpoint.
type VClawClaimRequest struct {
	ClaimCode string `json:"claim_code"`
	Device    struct {
		DeviceHash         string `json:"device_hash" binding:"required"`
		FingerprintVersion int    `json:"fingerprint_version" binding:"required"`
		InstallID          string `json:"install_id"`
		Platform           string `json:"platform" binding:"required"`
		Arch               string `json:"arch" binding:"required"`
		AppVersion         string `json:"app_version"`
	} `json:"device" binding:"required"`
}

// Claim handles the V-Claw claim/resume flow.
// POST /api/v1/vclaw/claim
func (h *VClawHandler) Claim(c *gin.Context) {
	var req VClawClaimRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}

	result, err := h.claimService.Claim(c.Request.Context(), service.VClawClaimRequest{
		ClaimCode: req.ClaimCode,
		Device: service.VClawDeviceInput{
			DeviceHash:         req.Device.DeviceHash,
			FingerprintVersion: req.Device.FingerprintVersion,
			InstallID:          req.Device.InstallID,
			Platform:           req.Device.Platform,
			Arch:               req.Device.Arch,
			AppVersion:         req.Device.AppVersion,
		},
	})
	if err != nil {
		slog.Warn("vclaw claim request rejected", "error", err)
		response.ErrorFrom(c, err)
		return
	}

	response.Success(c, result)
}
