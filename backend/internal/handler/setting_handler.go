package handler

import (
	"github.com/Wei-Shaw/sub2api/internal/handler/dto"
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/gin-gonic/gin"
)

// SettingHandler 公开设置处理器（无需认证）
type SettingHandler struct {
	settingService *service.SettingService
	version        string
REDACTED

// NewSettingHandler 创建公开设置处理器
func NewSettingHandler(settingService *service.SettingService, version string) *SettingHandler {
	return &SettingHandler{
		settingService: settingService,
		version:        version,
REDACTED
REDACTED

// GetPublicSettings 获取公开设置
// GET /api/v1/settings/public
func (h *SettingHandler) GetPublicSettings(c *gin.Context) {
	settings, err := h.settingService.GetPublicSettings(c.Request.Context())
	if err != nil {
		response.ErrorFrom(c, err)
		return
REDACTED

	response.Success(c, dto.PublicSettings{
		RegistrationEnabled: settings.RegistrationEnabled,
		EmailVerifyEnabled:  settings.EmailVerifyEnabled,
		TurnstileEnabled:    settings.TurnstileEnabled,
		TurnstileSiteKey:    settings.TurnstileSiteKey,
		SiteName:            settings.SiteName,
		SiteLogo:            settings.SiteLogo,
		SiteSubtitle:        settings.SiteSubtitle,
		ApiBaseUrl:          settings.ApiBaseUrl,
		ContactInfo:         settings.ContactInfo,
		DocUrl:              settings.DocUrl,
		Version:             h.version,
REDACTED)
REDACTED
