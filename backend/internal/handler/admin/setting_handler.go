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
		RegistrationEnabled:      settings.RegistrationEnabled,
		EmailVerifyEnabled:       settings.EmailVerifyEnabled,
		SMTPHost:                 settings.SMTPHost,
		SMTPPort:                 settings.SMTPPort,
		SMTPUsername:             settings.SMTPUsername,
		SMTPPassword:             settings.SMTPPassword,
		SMTPFrom:                 settings.SMTPFrom,
		SMTPFromName:             settings.SMTPFromName,
		SMTPUseTLS:               settings.SMTPUseTLS,
		TurnstileEnabled:         settings.TurnstileEnabled,
		TurnstileSiteKey:         settings.TurnstileSiteKey,
		TurnstileSecretKey:       settings.TurnstileSecretKey,
		SiteName:                 settings.SiteName,
		SiteLogo:                 settings.SiteLogo,
		SiteSubtitle:             settings.SiteSubtitle,
		APIBaseURL:               settings.APIBaseURL,
		ContactInfo:              settings.ContactInfo,
		DocURL:                   settings.DocURL,
		DefaultConcurrency:       settings.DefaultConcurrency,
		DefaultBalance:           settings.DefaultBalance,
		EnableModelFallback:      settings.EnableModelFallback,
		FallbackModelAnthropic:   settings.FallbackModelAnthropic,
		FallbackModelOpenAI:      settings.FallbackModelOpenAI,
		FallbackModelGemini:      settings.FallbackModelGemini,
		FallbackModelAntigravity: settings.FallbackModelAntigravity,
REDACTED)
REDACTED

// UpdateSettingsRequest 更新设置请求
type UpdateSettingsRequest struct {
	// 注册设置
	RegistrationEnabled bool `json:"registration_enabled"`
	EmailVerifyEnabled  bool `json:"email_verify_enabled"`

	// 邮件服务设置
	SMTPHost     string `json:"smtp_host"`
	SMTPPort     int    `json:"smtp_port"`
	SMTPUsername string `json:"smtp_username"`
	SMTPPassword string `json:"smtp_password"`
	SMTPFrom     string `json:"smtp_from_email"`
	SMTPFromName string `json:"smtp_from_name"`
	SMTPUseTLS   bool   `json:"smtp_use_tls"`

	// Cloudflare Turnstile 设置
	TurnstileEnabled   bool   `json:"turnstile_enabled"`
	TurnstileSiteKey   string `json:"turnstile_site_key"`
	TurnstileSecretKey string `json:"turnstile_secret_key"`

	// OEM设置
	SiteName     string `json:"site_name"`
	SiteLogo     string `json:"site_logo"`
	SiteSubtitle string `json:"site_subtitle"`
	APIBaseURL   string `json:"api_base_url"`
	ContactInfo  string `json:"contact_info"`
	DocURL       string `json:"doc_url"`

	// 默认配置
	DefaultConcurrency int     `json:"default_concurrency"`
	DefaultBalance     float64 `json:"default_balance"`

	// Model fallback configuration
	EnableModelFallback      bool   `json:"enable_model_fallback"`
	FallbackModelAnthropic   string `json:"fallback_model_anthropic"`
	FallbackModelOpenAI      string `json:"fallback_model_openai"`
	FallbackModelGemini      string `json:"fallback_model_gemini"`
	FallbackModelAntigravity string `json:"fallback_model_antigravity"`
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
	if req.SMTPPort <= 0 {
		req.SMTPPort = 587
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
		RegistrationEnabled:      req.RegistrationEnabled,
		EmailVerifyEnabled:       req.EmailVerifyEnabled,
		SMTPHost:                 req.SMTPHost,
		SMTPPort:                 req.SMTPPort,
		SMTPUsername:             req.SMTPUsername,
		SMTPPassword:             req.SMTPPassword,
		SMTPFrom:                 req.SMTPFrom,
		SMTPFromName:             req.SMTPFromName,
		SMTPUseTLS:               req.SMTPUseTLS,
		TurnstileEnabled:         req.TurnstileEnabled,
		TurnstileSiteKey:         req.TurnstileSiteKey,
		TurnstileSecretKey:       req.TurnstileSecretKey,
		SiteName:                 req.SiteName,
		SiteLogo:                 req.SiteLogo,
		SiteSubtitle:             req.SiteSubtitle,
		APIBaseURL:               req.APIBaseURL,
		ContactInfo:              req.ContactInfo,
		DocURL:                   req.DocURL,
		DefaultConcurrency:       req.DefaultConcurrency,
		DefaultBalance:           req.DefaultBalance,
		EnableModelFallback:      req.EnableModelFallback,
		FallbackModelAnthropic:   req.FallbackModelAnthropic,
		FallbackModelOpenAI:      req.FallbackModelOpenAI,
		FallbackModelGemini:      req.FallbackModelGemini,
		FallbackModelAntigravity: req.FallbackModelAntigravity,
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
		RegistrationEnabled:      updatedSettings.RegistrationEnabled,
		EmailVerifyEnabled:       updatedSettings.EmailVerifyEnabled,
		SMTPHost:                 updatedSettings.SMTPHost,
		SMTPPort:                 updatedSettings.SMTPPort,
		SMTPUsername:             updatedSettings.SMTPUsername,
		SMTPPassword:             updatedSettings.SMTPPassword,
		SMTPFrom:                 updatedSettings.SMTPFrom,
		SMTPFromName:             updatedSettings.SMTPFromName,
		SMTPUseTLS:               updatedSettings.SMTPUseTLS,
		TurnstileEnabled:         updatedSettings.TurnstileEnabled,
		TurnstileSiteKey:         updatedSettings.TurnstileSiteKey,
		TurnstileSecretKey:       updatedSettings.TurnstileSecretKey,
		SiteName:                 updatedSettings.SiteName,
		SiteLogo:                 updatedSettings.SiteLogo,
		SiteSubtitle:             updatedSettings.SiteSubtitle,
		APIBaseURL:               updatedSettings.APIBaseURL,
		ContactInfo:              updatedSettings.ContactInfo,
		DocURL:                   updatedSettings.DocURL,
		DefaultConcurrency:       updatedSettings.DefaultConcurrency,
		DefaultBalance:           updatedSettings.DefaultBalance,
		EnableModelFallback:      updatedSettings.EnableModelFallback,
		FallbackModelAnthropic:   updatedSettings.FallbackModelAnthropic,
		FallbackModelOpenAI:      updatedSettings.FallbackModelOpenAI,
		FallbackModelGemini:      updatedSettings.FallbackModelGemini,
		FallbackModelAntigravity: updatedSettings.FallbackModelAntigravity,
REDACTED)
REDACTED

// TestSMTPRequest 测试SMTP连接请求
type TestSMTPRequest struct {
	SMTPHost     string `json:"smtp_host" binding:"required"`
	SMTPPort     int    `json:"smtp_port"`
	SMTPUsername string `json:"smtp_username"`
	SMTPPassword string `json:"smtp_password"`
	SMTPUseTLS   bool   `json:"smtp_use_tls"`
REDACTED

// TestSMTPConnection 测试SMTP连接
// POST /api/v1/admin/settings/test-smtp
func (h *SettingHandler) TestSMTPConnection(c *gin.Context) {
	var req TestSMTPRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
REDACTED

	if req.SMTPPort <= 0 {
		req.SMTPPort = 587
REDACTED

	// 如果未提供密码，从数据库获取已保存的密码
	password := req.SMTPPassword
	if password == "" {
		savedConfig, err := h.emailService.GetSMTPConfig(c.Request.Context())
		if err == nil && savedConfig != nil {
			password = savedConfig.Password
	REDACTED
REDACTED

	config := &service.SMTPConfig{
		Host:     req.SMTPHost,
		Port:     req.SMTPPort,
		Username: req.SMTPUsername,
		Password: password,
		UseTLS:   req.SMTPUseTLS,
REDACTED

	err := h.emailService.TestSMTPConnectionWithConfig(config)
	if err != nil {
		response.ErrorFrom(c, err)
		return
REDACTED

	response.Success(c, gin.H{"message": "SMTP connection successful"REDACTED)
REDACTED

// SendTestEmailRequest 发送测试邮件请求
type SendTestEmailRequest struct {
	Email        string `json:"email" binding:"required,email"`
	SMTPHost     string `json:"smtp_host" binding:"required"`
	SMTPPort     int    `json:"smtp_port"`
	SMTPUsername string `json:"smtp_username"`
	SMTPPassword string `json:"smtp_password"`
	SMTPFrom     string `json:"smtp_from_email"`
	SMTPFromName string `json:"smtp_from_name"`
	SMTPUseTLS   bool   `json:"smtp_use_tls"`
REDACTED

// SendTestEmail 发送测试邮件
// POST /api/v1/admin/settings/send-test-email
func (h *SettingHandler) SendTestEmail(c *gin.Context) {
	var req SendTestEmailRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
REDACTED

	if req.SMTPPort <= 0 {
		req.SMTPPort = 587
REDACTED

	// 如果未提供密码，从数据库获取已保存的密码
	password := req.SMTPPassword
	if password == "" {
		savedConfig, err := h.emailService.GetSMTPConfig(c.Request.Context())
		if err == nil && savedConfig != nil {
			password = savedConfig.Password
	REDACTED
REDACTED

	config := &service.SMTPConfig{
		Host:     req.SMTPHost,
		Port:     req.SMTPPort,
		Username: req.SMTPUsername,
		Password: password,
		From:     req.SMTPFrom,
		FromName: req.SMTPFromName,
		UseTLS:   req.SMTPUseTLS,
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

// GetAdminAPIKey 获取管理员 API Key 状态
// GET /api/v1/admin/settings/admin-api-key
func (h *SettingHandler) GetAdminAPIKey(c *gin.Context) {
	maskedKey, exists, err := h.settingService.GetAdminAPIKeyStatus(c.Request.Context())
	if err != nil {
		response.ErrorFrom(c, err)
		return
REDACTED

	response.Success(c, gin.H{
		"exists":     exists,
		"masked_key": maskedKey,
REDACTED)
REDACTED

// RegenerateAdminAPIKey 生成/重新生成管理员 API Key
// POST /api/v1/admin/settings/admin-api-key/regenerate
func (h *SettingHandler) RegenerateAdminAPIKey(c *gin.Context) {
	key, err := h.settingService.GenerateAdminAPIKey(c.Request.Context())
	if err != nil {
		response.ErrorFrom(c, err)
		return
REDACTED

	response.Success(c, gin.H{
		"key": key, // 完整 key 只在生成时返回一次
REDACTED)
REDACTED

// DeleteAdminAPIKey 删除管理员 API Key
// DELETE /api/v1/admin/settings/admin-api-key
func (h *SettingHandler) DeleteAdminAPIKey(c *gin.Context) {
	if err := h.settingService.DeleteAdminAPIKey(c.Request.Context()); err != nil {
		response.ErrorFrom(c, err)
		return
REDACTED

	response.Success(c, gin.H{"message": "Admin API key deleted"REDACTED)
REDACTED
