package admin

import (
	"strconv"

	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

type openCodeGoUsageAutoRefreshRequest struct {
	Enabled *bool `json:"enabled" binding:"required"`
}

func (h *AccountHandler) GetOpenCodeGoUsageSettings(c *gin.Context) {
	if h.opencodeGoUsage == nil {
		response.ErrorFrom(c, service.ErrOpenCodeGoUsageUnavailable)
		return
	}
	settings, err := h.opencodeGoUsage.GetSettings(c.Request.Context())
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, settings)
}

func (h *AccountHandler) UpdateOpenCodeGoUsageSettings(c *gin.Context) {
	if h.opencodeGoUsage == nil {
		response.ErrorFrom(c, service.ErrOpenCodeGoUsageUnavailable)
		return
	}
	var req service.OpenCodeGoUsageSettings
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}
	if err := h.opencodeGoUsage.UpdateSettings(c.Request.Context(), &req); err != nil {
		response.ErrorFrom(c, err)
		return
	}
	settings, err := h.opencodeGoUsage.GetSettings(c.Request.Context())
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, settings)
}

func (h *AccountHandler) GetOpenCodeGoUsage(c *gin.Context) {
	if !h.requireOpenCodeGoUsage(c) {
		return
	}
	accountID, ok := openCodeGoUsageAccountID(c)
	if !ok {
		return
	}
	state, err := h.opencodeGoUsage.GetState(c.Request.Context(), accountID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, state)
}

func (h *AccountHandler) SetOpenCodeGoUsageAutoRefresh(c *gin.Context) {
	if !h.requireOpenCodeGoUsage(c) {
		return
	}
	accountID, ok := openCodeGoUsageAccountID(c)
	if !ok {
		return
	}
	var req openCodeGoUsageAutoRefreshRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}
	state, err := h.opencodeGoUsage.SetAutoRefresh(c.Request.Context(), accountID, *req.Enabled)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, state)
}

func (h *AccountHandler) RefreshOpenCodeGoUsage(c *gin.Context) {
	if !h.requireOpenCodeGoUsage(c) {
		return
	}
	accountID, ok := openCodeGoUsageAccountID(c)
	if !ok {
		return
	}
	state, err := h.opencodeGoUsage.Refresh(c.Request.Context(), accountID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, state)
}

func (h *AccountHandler) requireOpenCodeGoUsage(c *gin.Context) bool {
	if h != nil && h.opencodeGoUsage != nil {
		return true
	}
	response.ErrorFrom(c, service.ErrOpenCodeGoUsageUnavailable)
	return false
}

func openCodeGoUsageAccountID(c *gin.Context) (int64, bool) {
	if c == nil {
		return 0, false
	}
	accountID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || accountID <= 0 {
		response.BadRequest(c, "Invalid account ID")
		return 0, false
	}
	return accountID, true
}
