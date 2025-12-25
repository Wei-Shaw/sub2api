package admin

import (
	"github.com/Wei-Shaw/sub2api/internal/model"
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/gin-gonic/gin"
)

// SettingHandler 系统设置处理器
type SettingHandler struct {
	settingService *service.SettingService
	emailService   *service.EmailService
}

// NewSettingHandler 创建系统设置处理器
func NewSettingHandler(settingService *service.SettingService, emailService *service.EmailService) *SettingHandler {
	return &SettingHandler{
		settingService: settingService,
		emailService:   emailService,
	}
}

// GetSettings 获取所有系统设置
// GET /api/v1/admin/settings
func (h *SettingHandler) GetSettings(c *gin.Context) {
	settings, err := h.settingService.GetAllSettings(c.Request.Context())
	if err != nil {
		response.InternalError(c, "Failed to get settings: "+err.Error())
		return
	}

	response.Success(c, settings)
}

// UpdateSettingsRequest 更新设置请求
type UpdateSettingsRequest struct {
	// 注册设置
	RegistrationEnabled bool `json:"registration_enabled"`
	EmailVerifyEnabled  bool `json:"email_verify_enabled"`

	// 邮件服务设置
	SmtpHost     string `json:"smtp_host"`
	SmtpPort     int    `json:"smtp_port"`
	SmtpUsername string `json:"smtp_username"`
	SmtpPassword string `json:"smtp_password"`
	SmtpFrom     string `json:"smtp_from_email"`
	SmtpFromName string `json:"smtp_from_name"`
	SmtpUseTLS   bool   `json:"smtp_use_tls"`

	// Cloudflare Turnstile 设置
	TurnstileEnabled   bool   `json:"turnstile_enabled"`
	TurnstileSiteKey   string `json:"turnstile_site_key"`
	TurnstileSecretKey string `json:"turnstile_secret_key"`

	// OEM设置
	SiteName     string `json:"site_name"`
	SiteLogo     string `json:"site_logo"`
	SiteSubtitle string `json:"site_subtitle"`
	ApiBaseUrl   string `json:"api_base_url"`
	ContactInfo  string `json:"contact_info"`
	DocUrl       string `json:"doc_url"`

	// 默认配置
	DefaultConcurrency int     `json:"default_concurrency"`
	DefaultBalance     float64 `json:"default_balance"`
}

// UpdateSettings 更新系统设置
// PUT /api/v1/admin/settings
func (h *SettingHandler) UpdateSettings(c *gin.Context) {
	var req UpdateSettingsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}

	// 验证参数
	if req.DefaultConcurrency < 1 {
		req.DefaultConcurrency = 1
	}
	if req.DefaultBalance < 0 {
		req.DefaultBalance = 0
	}
	if req.SmtpPort <= 0 {
		req.SmtpPort = 587
	}

	settings := &model.SystemSettings{
		RegistrationEnabled: req.RegistrationEnabled,
		EmailVerifyEnabled:  req.EmailVerifyEnabled,
		SmtpHost:            req.SmtpHost,
		SmtpPort:            req.SmtpPort,
		SmtpUsername:        req.SmtpUsername,
		SmtpPassword:        req.SmtpPassword,
		SmtpFrom:            req.SmtpFrom,
		SmtpFromName:        req.SmtpFromName,
		SmtpUseTLS:          req.SmtpUseTLS,
		TurnstileEnabled:    req.TurnstileEnabled,
		TurnstileSiteKey:    req.TurnstileSiteKey,
		TurnstileSecretKey:  req.TurnstileSecretKey,
		SiteName:            req.SiteName,
		SiteLogo:            req.SiteLogo,
		SiteSubtitle:        req.SiteSubtitle,
		ApiBaseUrl:          req.ApiBaseUrl,
		ContactInfo:         req.ContactInfo,
		DocUrl:              req.DocUrl,
		DefaultConcurrency:  req.DefaultConcurrency,
		DefaultBalance:      req.DefaultBalance,
	}

	if err := h.settingService.UpdateSettings(c.Request.Context(), settings); err != nil {
		response.InternalError(c, "Failed to update settings: "+err.Error())
		return
	}

	// 重新获取设置返回
	updatedSettings, err := h.settingService.GetAllSettings(c.Request.Context())
	if err != nil {
		response.InternalError(c, "Failed to get updated settings: "+err.Error())
		return
	}

	response.Success(c, updatedSettings)
}

// TestSmtpRequest 测试SMTP连接请求
type TestSmtpRequest struct {
	SmtpHost     string `json:"smtp_host" binding:"required"`
	SmtpPort     int    `json:"smtp_port"`
	SmtpUsername string `json:"smtp_username"`
	SmtpPassword string `json:"smtp_password"`
	SmtpUseTLS   bool   `json:"smtp_use_tls"`
}

// TestSmtpConnection 测试SMTP连接
// POST /api/v1/admin/settings/test-smtp
func (h *SettingHandler) TestSmtpConnection(c *gin.Context) {
	var req TestSmtpRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}

	if req.SmtpPort <= 0 {
		req.SmtpPort = 587
	}

	// 如果未提供密码，从数据库获取已保存的密码
	password := req.SmtpPassword
	if password == "" {
		savedConfig, err := h.emailService.GetSmtpConfig(c.Request.Context())
		if err == nil && savedConfig != nil {
			password = savedConfig.Password
		}
	}

	config := &service.SmtpConfig{
		Host:     req.SmtpHost,
		Port:     req.SmtpPort,
		Username: req.SmtpUsername,
		Password: password,
		UseTLS:   req.SmtpUseTLS,
	}

	err := h.emailService.TestSmtpConnectionWithConfig(config)
	if err != nil {
		response.BadRequest(c, "SMTP connection test failed: "+err.Error())
		return
	}

	response.Success(c, gin.H{"message": "SMTP connection successful"})
}

// SendTestEmailRequest 发送测试邮件请求
type SendTestEmailRequest struct {
	Email        string `json:"email" binding:"required,email"`
	SmtpHost     string `json:"smtp_host" binding:"required"`
	SmtpPort     int    `json:"smtp_port"`
	SmtpUsername string `json:"smtp_username"`
	SmtpPassword string `json:"smtp_password"`
	SmtpFrom     string `json:"smtp_from_email"`
	SmtpFromName string `json:"smtp_from_name"`
	SmtpUseTLS   bool   `json:"smtp_use_tls"`
}

// SendTestEmail 发送测试邮件
// POST /api/v1/admin/settings/send-test-email
func (h *SettingHandler) SendTestEmail(c *gin.Context) {
	var req SendTestEmailRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}

	if req.SmtpPort <= 0 {
		req.SmtpPort = 587
	}

	// 如果未提供密码，从数据库获取已保存的密码
	password := req.SmtpPassword
	if password == "" {
		savedConfig, err := h.emailService.GetSmtpConfig(c.Request.Context())
		if err == nil && savedConfig != nil {
			password = savedConfig.Password
		}
	}

	config := &service.SmtpConfig{
		Host:     req.SmtpHost,
		Port:     req.SmtpPort,
		Username: req.SmtpUsername,
		Password: password,
		From:     req.SmtpFrom,
		FromName: req.SmtpFromName,
		UseTLS:   req.SmtpUseTLS,
	}

	siteName := h.settingService.GetSiteName(c.Request.Context())
	subject := "[" + siteName + "] Test Email"
	body := `
<!DOCTYPE html>
<html>
<head>
    <meta charset="UTF-8">
    <style>
        body { font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif; background-color: #f5f5f5; margin: 0; padding: 20px; }
        .container { max-width: 600px; margin: 0 auto; background-color: #ffffff; border-radius: 8px; overflow: hidden; box-shadow: 0 2px 8px rgba(0,0,0,0.1); }
        .header { background: linear-gradient(135deg, #667eea 0%, #764ba2 100%); color: white; padding: 30px; text-align: center; }
        .content { padding: 40px 30px; text-align: center; }
        .success { color: #10b981; font-size: 48px; margin-bottom: 20px; }
        .footer { background-color: #f8f9fa; padding: 20px; text-align: center; color: #999; font-size: 12px; }
    </style>
</head>
<body>
    <div class="container">
        <div class="header">
            <h1>` + siteName + `</h1>
        </div>
        <div class="content">
            <div class="success">✓</div>
            <h2>Email Configuration Successful!</h2>
            <p>This is a test email to verify your SMTP settings are working correctly.</p>
        </div>
        <div class="footer">
            <p>This is an automated test message.</p>
        </div>
    </div>
</body>
</html>
`

	if err := h.emailService.SendEmailWithConfig(config, req.Email, subject, body); err != nil {
		response.BadRequest(c, "Failed to send test email: "+err.Error())
		return
	}

	response.Success(c, gin.H{"message": "Test email sent successfully"})
}

// GetAdminApiKey 获取管理员 API Key 状态
// GET /api/v1/admin/settings/admin-api-key
func (h *SettingHandler) GetAdminApiKey(c *gin.Context) {
	maskedKey, exists, err := h.settingService.GetAdminApiKeyStatus(c.Request.Context())
	if err != nil {
		response.InternalError(c, "Failed to get admin API key status: "+err.Error())
		return
	}

	response.Success(c, gin.H{
		"exists":     exists,
		"masked_key": maskedKey,
	})
}

// RegenerateAdminApiKey 生成/重新生成管理员 API Key
// POST /api/v1/admin/settings/admin-api-key/regenerate
func (h *SettingHandler) RegenerateAdminApiKey(c *gin.Context) {
	key, err := h.settingService.GenerateAdminApiKey(c.Request.Context())
	if err != nil {
		response.InternalError(c, "Failed to generate admin API key: "+err.Error())
		return
	}

	response.Success(c, gin.H{
		"key": key, // 完整 key 只在生成时返回一次
	})
}

// DeleteAdminApiKey 删除管理员 API Key
// DELETE /api/v1/admin/settings/admin-api-key
func (h *SettingHandler) DeleteAdminApiKey(c *gin.Context) {
	if err := h.settingService.DeleteAdminApiKey(c.Request.Context()); err != nil {
		response.InternalError(c, "Failed to delete admin API key: "+err.Error())
		return
	}

	response.Success(c, gin.H{"message": "Admin API key deleted"})
}
