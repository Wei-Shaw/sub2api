package handler

import (
	"github.com/Wei-Shaw/sub2api/internal/handler/dto"
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/gin-gonic/gin"
)

type InviteLoginRequest struct {
	InvitationCode string `json:"invitation_code" binding:"required"`
	DeviceHash     string `json:"device_hash" binding:"required"`
}

// InviteLogin handles DLG device-bound login.
// POST /api/v1/auth/invite-login
func (h *AuthHandler) InviteLogin(c *gin.Context) {
	var req InviteLoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}
	result, err := h.authService.InviteLogin(c.Request.Context(), service.InviteLoginInput{
		InvitationCode: req.InvitationCode,
		DeviceHash:     req.DeviceHash,
	})
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	if result == nil || result.TokenPair == nil || result.User == nil {
		response.InternalError(c, "Failed to generate token")
		return
	}
	response.Success(c, AuthResponse{
		AccessToken:  result.TokenPair.AccessToken,
		RefreshToken: result.TokenPair.RefreshToken,
		ExpiresIn:    result.TokenPair.ExpiresIn,
		TokenType:    "Bearer",
		User:         dto.UserFromService(result.User),
	})
}
