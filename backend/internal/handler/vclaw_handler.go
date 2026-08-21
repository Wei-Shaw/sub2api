package handler

import (
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/gin-gonic/gin"
)

type VClawHandler struct {
	claimService *service.VClawClaimService
}

func NewVClawHandler(claimService *service.VClawClaimService) *VClawHandler {
	return &VClawHandler{claimService: claimService}
}

type VClawClaimRequest struct {
	ClaimCode          string `json:"claim_code"`
	DeviceHash         string `json:"device_hash" binding:"required"`
	FingerprintVersion int    `json:"fingerprint_version" binding:"required"`
	InstallID          string `json:"install_id"`
	Platform           string `json:"platform" binding:"required"`
	Arch               string `json:"arch" binding:"required"`
	AppVersion         string `json:"app_version"`
}

// Claim creates or resumes the DLG binding for a V-Claw device.
// POST /api/v1/vclaw/claim
func (h *VClawHandler) Claim(c *gin.Context) {
	var req VClawClaimRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}
	if h == nil || h.claimService == nil {
		response.InternalError(c, "Service unavailable")
		return
	}
	result, err := h.claimService.Claim(c.Request.Context(), service.VClawClaimRequest{
		ClaimCode: req.ClaimCode,
		Device: service.VClawDeviceInput{
			DeviceHash:         req.DeviceHash,
			FingerprintVersion: req.FingerprintVersion,
			InstallID:          req.InstallID,
			Platform:           req.Platform,
			Arch:               req.Arch,
			AppVersion:         req.AppVersion,
		},
	})
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, result)
}
