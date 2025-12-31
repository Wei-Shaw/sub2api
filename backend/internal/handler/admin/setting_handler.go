package admin

import (
	"github.com/Wei-Shaw/sub2api/internal/handler/dto"
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/gin-gonic/gin"
)

// SettingHandler 系统设置处理器
type SettingHandler struct {
	settingService   *service.SettingService
	emailService     *service.EmailService
	turnstileService *service.TurnstileService
REDACTED

// NewSettingHandler 创建系统设置处理器
func NewSettingHandler(settingService *service.SettingService, emailService *service.EmailService, turnstileService *service.TurnstileService) *SettingHandler {
	return &SettingHandler{
		settingService:   settingService,
		emailService:     emailService,
		turnstileService: turnstileService,
REDACTED
REDACTED

// GetSettings 获取所有系统设置
// GET /api/v1/admin/settings
func (h *SettingHandler) GetSettings(c *gin.Context) {
	settings, err := h.settingService.GetAllSettings(c.Request.Context())
	if err != nil {
		response.ErrorFrom(c, err)
		return
REDACTED

	response.Success(c, dto.SystemSettings{
		RegistrationEnabled: settings.RegistrationEnabled,
		EmailVerifyEnabled:  settings.EmailVerifyEnabled,
		SmtpHost:            settings.SmtpHost,
		SmtpPort:            settings.SmtpPort,
		SmtpUsername:        settings.SmtpUsername,
		SmtpPassword:        settings.SmtpPassword,
		SmtpFrom:            settings.SmtpFrom,
		SmtpFromName:        settings.SmtpFromName,
		SmtpUseTLS:          settings.SmtpUseTLS,
		TurnstileEnabled:    settings.TurnstileEnabled,
		TurnstileSiteKey:    settings.TurnstileSiteKey,
		TurnstileSecretKey:  settings.TurnstileSecretKey,
		SiteName:            settings.SiteName,
		SiteLogo:            settings.SiteLogo,
		SiteSubtitle:        settings.SiteSubtitle,
		ApiBaseUrl:          settings.ApiBaseUrl,
		ContactInfo:         settings.ContactInfo,
		DocUrl:              settings.DocUrl,
		DefaultConcurrency:  settings.DefaultConcurrency,
		DefaultBalance:      settings.DefaultBalance,
REDACTED)
REDACTED

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
REDACTED

// UpdateSettings 更新系统设置
// PUT /api/v1/admin/settings
func (h *SettingHandler) UpdateSettings(c *gin.Context) {
	var req UpdateSettingsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
REDACTED

	// 验证参数
	if req.DefaultConcurrency < 1 {
		req.DefaultConcurrency = 1
REDACTED
	if req.DefaultBalance < 0 {
		req.DefaultBalance = 0
REDACTED
	if req.SmtpPort <= 0 {
		req.SmtpPort = 587
REDACTED

	// Turnstile 参数验证
	if req.TurnstileEnabled {
		// 检查必填字段
		if req.TurnstileSiteKey == "" {
			response.BadRequest(c, "Turnstile Site Key is required when enabled")
			return
	REDACTED
		if req.TurnstileSecretKey == "" {
			response.BadRequest(c, "Turnstile Secret Key is required when enabled")
			return
	REDACTED

		// 获取当前设置，检查参数是否有变化
		currentSettings, err := h.settingService.GetAllSettings(c.Request.Context())
		if err != nil {
			response.ErrorFrom(c, err)
			return
	REDACTED

		// 当 site_key 或 secret_key 任一变化时验证（避免配置错误导致无法登录）
		siteKeyChanged := currentSettings.TurnstileSiteKey != req.TurnstileSiteKey
		secretKeyChanged := currentSettings.TurnstileSecretKey != req.TurnstileSecretKey
		if siteKeyChanged || secretKeyChanged {
			if err := h.turnstileService.ValidateSecretKey(c.Request.Context(), req.TurnstileSecretKey); err != nil {
				response.ErrorFrom(c, err)
				return
		REDACTED
	REDACTED
REDACTED

	settings := &service.SystemSettings{
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
REDACTED

	if err := h.settingService.UpdateSettings(c.Request.Context(), settings); err != nil {
		response.ErrorFrom(c, err)
		return
REDACTED

	// 重新获取设置返回
	updatedSettings, err := h.settingService.GetAllSettings(c.Request.Context())
	if err != nil {
		response.ErrorFrom(c, err)
		return
REDACTED

	response.Success(c, dto.SystemSettings{
		RegistrationEnabled: updatedSettings.RegistrationEnabled,
		EmailVerifyEnabled:  updatedSettings.EmailVerifyEnabled,
		SmtpHost:            updatedSettings.SmtpHost,
		SmtpPort:            updatedSettings.SmtpPort,
		SmtpUsername:        updatedSettings.SmtpUsername,
		SmtpPassword:        updatedSettings.SmtpPassword,
		SmtpFrom:            updatedSettings.SmtpFrom,
		SmtpFromName:        updatedSettings.SmtpFromName,
		SmtpUseTLS:          updatedSettings.SmtpUseTLS,
		TurnstileEnabled:    updatedSettings.TurnstileEnabled,
		TurnstileSiteKey:    updatedSettings.TurnstileSiteKey,
		TurnstileSecretKey:  updatedSettings.TurnstileSecretKey,
		SiteName:            updatedSettings.SiteName,
		SiteLogo:            updatedSettings.SiteLogo,
		SiteSubtitle:        updatedSettings.SiteSubtitle,
		ApiBaseUrl:          updatedSettings.ApiBaseUrl,
		ContactInfo:         updatedSettings.ContactInfo,
		DocUrl:              updatedSettings.DocUrl,
		DefaultConcurrency:  updatedSettings.DefaultConcurrency,
		DefaultBalance:      updatedSettings.DefaultBalance,
REDACTED)
REDACTED

// TestSmtpRequest 测试SMTP连接请求
type TestSmtpRequest struct {
	SmtpHost     string `json:"smtp_host" binding:"required"`
	SmtpPort     int    `json:"smtp_port"`
	SmtpUsername string `json:"smtp_username"`
	SmtpPassword string `json:"smtp_password"`
	SmtpUseTLS   bool   `json:"smtp_use_tls"`
REDACTED

// TestSmtpConnection 测试SMTP连接
// POST /api/v1/admin/settings/test-smtp
func (h *SettingHandler) TestSmtpConnection(c *gin.Context) {
	var req TestSmtpRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
REDACTED

	if req.SmtpPort <= 0 {
		req.SmtpPort = 587
REDACTED

	// 如果未提供密码，从数据库获取已保存的密码
	password := req.SmtpPassword
	if password == "" {
		savedConfig, err := h.emailService.GetSmtpConfig(c.Request.Context())
		if err == nil && savedConfig != nil {
			password = savedConfig.Password
	REDACTED
REDACTED

	config := &service.SmtpConfig{
		Host:     req.SmtpHost,
		Port:     req.SmtpPort,
		Username: req.SmtpUsername,
		Password: password,
		UseTLS:   req.SmtpUseTLS,
REDACTED

	err := h.emailService.TestSmtpConnectionWithConfig(config)
	if err != nil {
		response.ErrorFrom(c, err)
		return
REDACTED

	response.Success(c, gin.H{"message": "SMTP connection successful"REDACTED)
REDACTED

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
REDACTED

// SendTestEmail 发送测试邮件
// POST /api/v1/admin/settings/send-test-email
func (h *SettingHandler) SendTestEmail(c *gin.Context) {
	var req SendTestEmailRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
REDACTED

	if req.SmtpPort <= 0 {
		req.SmtpPort = 587
REDACTED

	// 如果未提供密码，从数据库获取已保存的密码
	password := req.SmtpPassword
	if password == "" {
		savedConfig, err := h.emailService.GetSmtpConfig(c.Request.Context())
		if err == nil && savedConfig != nil {
			password = savedConfig.Password
	REDACTED
REDACTED

	config := &service.SmtpConfig{
		Host:     req.SmtpHost,
		Port:     req.SmtpPort,
		Username: req.SmtpUsername,
		Password: password,
		From:     req.SmtpFrom,
		FromName: req.SmtpFromName,
		UseTLS:   req.SmtpUseTLS,
REDACTED

	siteName := h.settingService.GetSiteName(c.Request.Context())
	subject := "[" + siteName + "] Test Email"
	body := `
<!DOCTYPE html>
<html>
<head>
    <meta charset="UTF-8">
    <style>
        body { font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif; background-color: #f5f5f5; margin: 0; padding: 20px; REDACTED
        .container { max-width: 600px; margin: 0 auto; background-color: #ffffff; border-radius: 8px; overflow: hidden; box-shadow: 0 2px 8px rgba(0,0,0,0.1); REDACTED
        .header { background: linear-gradient(135deg, #667eea 0%, #764ba2 100%); color: white; padding: 30px; text-align: center; REDACTED
        .content { padding: 40px 30px; text-align: center; REDACTED
        .success { color: #10b981; font-size: 48px; margin-bottom: 20px; REDACTED
        .footer { background-color: #f8f9fa; padding: 20px; text-align: center; color: #999; font-size: 12px; REDACTED
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
		response.ErrorFrom(c, err)
		return
REDACTED

	response.Success(c, gin.H{"message": "Test email sent successfully"REDACTED)
REDACTED

// GetAdminApiKey 获取管理员 API Key 状态
// GET /api/v1/admin/settings/admin-api-key
func (h *SettingHandler) GetAdminApiKey(c *gin.Context) {
	maskedKey, exists, err := h.settingService.GetAdminApiKeyStatus(c.Request.Context())
	if err != nil {
		response.ErrorFrom(c, err)
		return
REDACTED

	response.Success(c, gin.H{
		"exists":     exists,
		"masked_key": maskedKey,
REDACTED)
REDACTED

// RegenerateAdminApiKey 生成/重新生成管理员 API Key
// POST /api/v1/admin/settings/admin-api-key/regenerate
func (h *SettingHandler) RegenerateAdminApiKey(c *gin.Context) {
	key, err := h.settingService.GenerateAdminApiKey(c.Request.Context())
	if err != nil {
		response.ErrorFrom(c, err)
		return
REDACTED

	response.Success(c, gin.H{
		"key": key, // 完整 key 只在生成时返回一次
REDACTED)
REDACTED

// DeleteAdminApiKey 删除管理员 API Key
// DELETE /api/v1/admin/settings/admin-api-key
func (h *SettingHandler) DeleteAdminApiKey(c *gin.Context) {
	if err := h.settingService.DeleteAdminApiKey(c.Request.Context()); err != nil {
		response.ErrorFrom(c, err)
		return
REDACTED

	response.Success(c, gin.H{"message": "Admin API key deleted"REDACTED)
REDACTED
