package admin

import (
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/gin-gonic/gin"
)

type updateGatewayRuntimeSettingsRequest struct {
	ConnectionPoolIsolation string                                 `json:"connection_pool_isolation" binding:"required"`
	OutboundPrivacy         service.GatewayOutboundPrivacySettings `json:"outbound_privacy" binding:"required"`
	OpenAIWS                service.GatewayOpenAIWSBudgetSettings  `json:"openai_ws" binding:"required"`
}

// GetGatewayRuntimeSettings returns the live gateway isolation and WS budget.
func (h *SettingHandler) GetGatewayRuntimeSettings(c *gin.Context) {
	settings, err := h.settingService.GetGatewayRuntimeSettings(c.Request.Context())
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, settings)
}

// UpdateGatewayRuntimeSettings persists and atomically applies the gateway
// isolation/privacy settings and OpenAI account-wide WS budget.
func (h *SettingHandler) UpdateGatewayRuntimeSettings(c *gin.Context) {
	var req updateGatewayRuntimeSettingsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}
	settings := &service.GatewayRuntimeSettings{
		ConnectionPoolIsolation: req.ConnectionPoolIsolation,
		OutboundPrivacy:         req.OutboundPrivacy,
		OpenAIWS:                req.OpenAIWS,
	}
	if err := h.settingService.SetGatewayRuntimeSettings(c.Request.Context(), settings); err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	updated, err := h.settingService.GetGatewayRuntimeSettings(c.Request.Context())
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, updated)
}
