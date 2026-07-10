package admin

import (
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

func (h *SettingHandler) GetFalUpscaleSettings(c *gin.Context) {
	settings := h.settingService.GetFalUpscaleSettings(c.Request.Context())
	response.Success(c, gin.H{
		"endpoint":        settings.Endpoint,
		"timeout_seconds": settings.TimeoutSeconds,
		"token_set":       strings.TrimSpace(settings.Token) != "",
	})
}

type UpdateFalUpscaleSettingsRequest struct {
	Endpoint       string `json:"endpoint"`
	Token          string `json:"token"`
	TimeoutSeconds int    `json:"timeout_seconds"`
}

func (h *SettingHandler) UpdateFalUpscaleSettings(c *gin.Context) {
	var req UpdateFalUpscaleSettingsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}
	if err := h.settingService.SetFalUpscaleSettings(c.Request.Context(), &service.FalUpscaleSettings{
		Endpoint:       req.Endpoint,
		Token:          req.Token,
		TimeoutSeconds: req.TimeoutSeconds,
	}); err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	h.GetFalUpscaleSettings(c)
}
