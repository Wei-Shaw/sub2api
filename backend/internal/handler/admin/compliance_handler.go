package admin

import (
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/pkg/ip"
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/gin-gonic/gin"
)

type ComplianceHandler struct {
	settingService *service.SettingService
}

func NewComplianceHandler(settingService *service.SettingService) *ComplianceHandler {
	return &ComplianceHandler{settingService: settingService}
}

type AcceptAdminComplianceRequest struct {
	Phrase   string `json:"phrase" binding:"required"`
	Language string `json:"language"`
}

func (h *ComplianceHandler) GetStatus(c *gin.Context) {
	subject, ok := middleware.GetAuthSubjectFromContext(c)
	if !ok {
		response.Unauthorized(c, "User not authenticated")
		return
	}

	status, err := h.settingService.GetAdminComplianceStatus(c.Request.Context(), subject.UserID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, status)
}

func (h *ComplianceHandler) Accept(c *gin.Context) {
	var req AcceptAdminComplianceRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}

	subject, ok := middleware.GetAuthSubjectFromContext(c)
	if !ok {
		response.Unauthorized(c, "User not authenticated")
		return
	}

	status, err := h.settingService.AcceptAdminCompliance(c.Request.Context(), service.AdminComplianceAcceptInput{
		AdminUserID: subject.UserID,
		Phrase:      req.Phrase,
		Language:    req.Language,
		IPAddress:   ip.GetClientIP(c),
		UserAgent:   strings.TrimSpace(c.GetHeader("User-Agent")),
	})
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, status)
}
