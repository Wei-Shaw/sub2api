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
	userService        *service.UserService
}

func NewSystemTokenHandler(systemTokenService *service.SystemTokenService, userService *service.UserService) *SystemTokenHandler {
	return &SystemTokenHandler{
		systemTokenService: systemTokenService,
		userService:        userService,
	}
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

type generateSystemTokenRequest struct {
	Password string `json:"password"`
}

// Generate creates a new system access token (replaces any existing one).
// Requires the user's current password as a second factor.
func (h *SystemTokenHandler) Generate(c *gin.Context) {
	subject, ok := middleware2.GetAuthSubjectFromContext(c)
	if !ok {
		response.Error(c, http.StatusUnauthorized, "unauthorized")
		return
	}

	var req generateSystemTokenRequest
	if err := c.ShouldBindJSON(&req); err != nil || req.Password == "" {
		response.Error(c, http.StatusBadRequest, "password is required")
		return
	}

	user, err := h.userService.GetByID(c.Request.Context(), subject.UserID)
	if err != nil {
		response.Error(c, http.StatusInternalServerError, err.Error())
		return
	}
	if !user.CheckPassword(req.Password) {
		response.Error(c, http.StatusForbidden, "incorrect password")
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
