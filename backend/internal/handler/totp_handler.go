package handler

import (
	"github.com/gin-gonic/gin"

	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	middleware2 "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
)

// TotpHandler handles TOTP-related requests
type TotpHandler struct {
	totpService *service.TotpService
}

// NewTotpHandler creates a new TotpHandler
func NewTotpHandler(totpService *service.TotpService) *TotpHandler {
	return &TotpHandler{
		totpService: totpService,
	}
}

// TotpStatusResponse represents the TOTP status response
type TotpStatusResponse struct {
	Enabled        bool   `json:"enabled"`
	EnabledAt      *int64 `json:"enabled_at,omitempty"` // Unix timestamp
	FeatureEnabled bool   `json:"feature_enabled"`
}

// GetStatus returns the TOTP status for the current user
// GET /api/v1/user/totp/status
func (h *TotpHandler) GetStatus(c *gin.Context) {
	subject, ok := middleware2.GetAuthSubjectFromContext(c)
	if !ok {
		response.Unauthorized(c, "User not authenticated")
		return
	}

	status, err := h.totpService.GetStatus(c.Request.Context(), subject.UserID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}

	resp := TotpStatusResponse{
		Enabled:        status.Enabled,
		FeatureEnabled: status.FeatureEnabled,
	}

	if status.EnabledAt != nil {
		ts := status.EnabledAt.Unix()
		resp.EnabledAt = &ts
	}

	response.Success(c, resp)
}

// TotpSetupRequest represents the request to initiate TOTP setup
type TotpSetupRequest struct {
	EmailCode string `json:"email_code"`
	Password  string `json:"password"`
}

// TotpSetupResponse represents the TOTP setup response
type TotpSetupResponse struct {
	Secret     string `json:"secret"`
	QRCodeURL  string `json:"qr_code_url"`
	SetupToken string `json:"setup_token"`
	Countdown  int    `json:"countdown"`
}

// InitiateSetup starts the TOTP setup process
// POST /api/v1/user/totp/setup
func (h *TotpHandler) InitiateSetup(c *gin.Context) {
	subject, ok := middleware2.GetAuthSubjectFromContext(c)
	if !ok {
		response.Unauthorized(c, "User not authenticated")
		return
	}

	var req TotpSetupRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		// Allow empty body (optional params)
		req = TotpSetupRequest{}
	}

	result, err := h.totpService.InitiateSetup(c.Request.Context(), subject.UserID, req.EmailCode, req.Password)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}

	response.Success(c, TotpSetupResponse{
		Secret:     result.Secret,
		QRCodeURL:  result.QRCodeURL,
		SetupToken: result.SetupToken,
		Countdown:  result.Countdown,
	})
}

// TotpEnableRequest represents the request to enable TOTP
type TotpEnableRequest struct {
	TotpCode   string `json:"totp_code" binding:"required,len=6"`
	SetupToken string `json:"setup_token" binding:"required"`
}

// Enable completes the TOTP setup
// POST /api/v1/user/totp/enable
func (h *TotpHandler) Enable(c *gin.Context) {
	subject, ok := middleware2.GetAuthSubjectFromContext(c)
	if !ok {
		response.Unauthorized(c, "User not authenticated")
		return
	}

	var req TotpEnableRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}

	if err := h.totpService.CompleteSetup(c.Request.Context(), subject.UserID, req.TotpCode, req.SetupToken); err != nil {
		response.ErrorFrom(c, err)
		return
	}

	response.Success(c, gin.H{"success": true})
}

// TotpDisableRequest represents the request to disable TOTP
type TotpDisableRequest struct {
	EmailCode string `json:"email_code"`
	Password  string `json:"password"`
}

// Disable disables TOTP for the current user
// POST /api/v1/user/totp/disable
func (h *TotpHandler) Disable(c *gin.Context) {
	subject, ok := middleware2.GetAuthSubjectFromContext(c)
	if !ok {
		response.Unauthorized(c, "User not authenticated")
		return
	}

	var req TotpDisableRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}

	if err := h.totpService.Disable(c.Request.Context(), subject.UserID, req.EmailCode, req.Password); err != nil {
		response.ErrorFrom(c, err)
		return
	}

	response.Success(c, gin.H{"success": true})
}

// GetVerificationMethod returns the verification method for TOTP operations
// GET /api/v1/user/totp/verification-method
func (h *TotpHandler) GetVerificationMethod(c *gin.Context) {
	method := h.totpService.GetVerificationMethod(c.Request.Context())
	response.Success(c, method)
}

// SendVerifyCode sends an email verification code for TOTP operations
// POST /api/v1/user/totp/send-code
func (h *TotpHandler) SendVerifyCode(c *gin.Context) {
	subject, ok := middleware2.GetAuthSubjectFromContext(c)
	if !ok {
		response.Unauthorized(c, "User not authenticated")
		return
	}

	if err := h.totpService.SendVerifyCode(c.Request.Context(), subject.UserID); err != nil {
		response.ErrorFrom(c, err)
		return
	}

	response.Success(c, gin.H{"success": true})
}
