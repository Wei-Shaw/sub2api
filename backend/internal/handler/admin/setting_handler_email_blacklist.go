package admin

import (
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/gin-gonic/gin"
)

// RegistrationEmailBlacklistResponse 精确邮箱黑名单
type RegistrationEmailBlacklistResponse struct {
	Emails []string `json:"emails"`
	Max    int      `json:"max"`
}

// UpdateRegistrationEmailBlacklistRequest 整体替换精确邮箱黑名单；emails 必须显式提交，
// 传空数组表示清空。
type UpdateRegistrationEmailBlacklistRequest struct {
	Emails *[]string `json:"emails"`
}

// UpdateRegistrationEmailBlacklistResponse 保存后的名单，added 为相对旧名单新增的项。
type UpdateRegistrationEmailBlacklistResponse struct {
	Emails []string `json:"emails"`
	Added  []string `json:"added"`
	Max    int      `json:"max"`
}

// GetRegistrationEmailBlacklist 获取精确邮箱黑名单
// GET /api/v1/admin/settings/registration-email-blacklist
func (h *SettingHandler) GetRegistrationEmailBlacklist(c *gin.Context) {
	emails, err := h.settingService.GetRegistrationEmailBlacklist(c.Request.Context())
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, RegistrationEmailBlacklistResponse{
		Emails: emails,
		Max:    service.MaxRegistrationEmailBlacklistSize,
	})
}

// UpdateRegistrationEmailBlacklist 整体替换精确邮箱黑名单
// PUT /api/v1/admin/settings/registration-email-blacklist
func (h *SettingHandler) UpdateRegistrationEmailBlacklist(c *gin.Context) {
	var req UpdateRegistrationEmailBlacklistRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}
	if req.Emails == nil {
		response.BadRequest(c, "emails is required")
		return
	}
	saved, added, err := h.settingService.SetRegistrationEmailBlacklist(c.Request.Context(), *req.Emails)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, UpdateRegistrationEmailBlacklistResponse{
		Emails: saved,
		Added:  added,
		Max:    service.MaxRegistrationEmailBlacklistSize,
	})
}
