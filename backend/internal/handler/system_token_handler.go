package handler

import (
	"net/http"

	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	middleware2 "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/gin-gonic/gin"
)

// SystemTokenHandler handles system access token (系统访问令牌) management.
type SystemTokenHandler struct {
	systemTokenService *service.SystemTokenService
}

func NewSystemTokenHandler(systemTokenService *service.SystemTokenService) *SystemTokenHandler {
	return &SystemTokenHandler{systemTokenService: systemTokenService}
}

// GetStatus returns whether the current user has a system access token.
func (h *SystemTokenHandler) GetStatus(c *gin.Context) {
	subject, ok := middleware2.GetAuthSubjectFromContext(c)
	if !ok {
		response.Error(c, http.StatusUnauthorized, "unauthorized")
		return
	}

	hasToken, err := h.systemTokenService.HasToken(c.Request.Context(), subject.UserID)
	if err != nil {
		response.Error(c, http.StatusInternalServerError, err.Error())
		return
	}

	response.Success(c, gin.H{"has_token": hasToken})
}

// Generate creates a new system access token (replaces any existing one).
func (h *SystemTokenHandler) Generate(c *gin.Context) {
	subject, ok := middleware2.GetAuthSubjectFromContext(c)
	if !ok {
		response.Error(c, http.StatusUnauthorized, "unauthorized")
		return
	}

	token, err := h.systemTokenService.Generate(c.Request.Context(), subject.UserID)
	if err != nil {
		response.Error(c, http.StatusInternalServerError, err.Error())
		return
	}

	response.Success(c, gin.H{"token": token})
}

// Revoke deletes the current user's system access token.
func (h *SystemTokenHandler) Revoke(c *gin.Context) {
	subject, ok := middleware2.GetAuthSubjectFromContext(c)
	if !ok {
		response.Error(c, http.StatusUnauthorized, "unauthorized")
		return
	}

	if err := h.systemTokenService.Revoke(c.Request.Context(), subject.UserID); err != nil {
		response.Error(c, http.StatusInternalServerError, err.Error())
		return
	}

	response.Success(c, gin.H{"message": "system token revoked"})
}
