package admin

import (
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

func (h *SettingHandler) GetClientRetryRules(c *gin.Context) {
	view, err := h.settingService.GetClientRetryRules(c.Request.Context())
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, view)
}

func (h *SettingHandler) UpdateClientRetryRules(c *gin.Context) {
	var req struct {
		Rules             []service.ClientRetryRule `json:"rules"`
		AutoUpdateEnabled *bool                     `json:"auto_update_enabled"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}
	if req.AutoUpdateEnabled != nil {
		if err := h.settingService.SetClientRetryRulesAutoUpdate(c.Request.Context(), *req.AutoUpdateEnabled); err != nil {
			response.ErrorFrom(c, err)
			return
		}
	}
	view, err := h.settingService.UpdateLocalClientRetryRules(c.Request.Context(), req.Rules)
	if err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	response.Success(c, view)
}

func (h *SettingHandler) CheckClientRetryRulesUpdate(c *gin.Context) {
	view, err := h.settingService.CheckClientRetryRulesUpdate(c.Request.Context())
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, view)
}

func (h *SettingHandler) RollbackClientRetryRules(c *gin.Context) {
	view, err := h.settingService.RollbackClientRetryRules(c.Request.Context())
	if err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	response.Success(c, view)
}
