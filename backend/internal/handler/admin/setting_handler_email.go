package admin

import (
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/handler/dto"
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/gin-gonic/gin"
)

// TestSMTPRequest 测试SMTP连接请求
type TestSMTPRequest struct {
	SMTPHost     string `json:"smtp_host"`
	SMTPPort     int    `json:"smtp_port"`
	SMTPUsername string `json:"smtp_username"`
	SMTPPassword string `json:"smtp_password"`
	SMTPUseTLS   bool   `json:"smtp_use_tls"`
}

// TestSMTPConnection 测试SMTP连接
// POST /api/v1/admin/settings/test-smtp
func (h *SettingHandler) TestSMTPConnection(c *gin.Context) {
	var req TestSMTPRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}

	req.SMTPHost = strings.TrimSpace(req.SMTPHost)
	req.SMTPUsername = strings.TrimSpace(req.SMTPUsername)

	var savedConfig *service.SMTPConfig
	if cfg, err := h.emailService.GetSMTPConfig(c.Request.Context()); err == nil && cfg != nil {
		savedConfig = cfg
	}

	if req.SMTPHost == "" && savedConfig != nil {
		req.SMTPHost = savedConfig.Host
	}
	if req.SMTPPort <= 0 {
		if savedConfig != nil && savedConfig.Port > 0 {
			req.SMTPPort = savedConfig.Port
		} else {
			req.SMTPPort = 587
		}
	}
	if req.SMTPUsername == "" && savedConfig != nil {
		req.SMTPUsername = savedConfig.Username
	}
	password := strings.TrimSpace(req.SMTPPassword)
	if password == "" && savedConfig != nil {
		password = savedConfig.Password
	}
	if req.SMTPHost == "" {
		response.BadRequest(c, "SMTP host is required")
		return
	}

	config := &service.SMTPConfig{
		Host:     req.SMTPHost,
		Port:     req.SMTPPort,
		Username: req.SMTPUsername,
		Password: password,
		UseTLS:   req.SMTPUseTLS,
	}

	err := h.emailService.TestSMTPConnectionWithConfig(config)
	if err != nil {
		response.BadRequest(c, "SMTP connection test failed: "+err.Error())
		return
	}

	response.Success(c, gin.H{"message": "SMTP connection successful"})
}

// SendTestEmailRequest 发送测试邮件请求
type SendTestEmailRequest struct {
	Email        string `json:"email" binding:"required,email"`
	SMTPHost     string `json:"smtp_host"`
	SMTPPort     int    `json:"smtp_port"`
	SMTPUsername string `json:"smtp_username"`
	SMTPPassword string `json:"smtp_password"`
	SMTPFrom     string `json:"smtp_from_email"`
	SMTPFromName string `json:"smtp_from_name"`
	SMTPUseTLS   bool   `json:"smtp_use_tls"`
}

// SendTestEmail 发送测试邮件
// POST /api/v1/admin/settings/send-test-email
func (h *SettingHandler) SendTestEmail(c *gin.Context) {
	var req SendTestEmailRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}

	req.SMTPHost = strings.TrimSpace(req.SMTPHost)
	req.SMTPUsername = strings.TrimSpace(req.SMTPUsername)
	req.SMTPFrom = strings.TrimSpace(req.SMTPFrom)
	req.SMTPFromName = strings.TrimSpace(req.SMTPFromName)

	var savedConfig *service.SMTPConfig
	if cfg, err := h.emailService.GetSMTPConfig(c.Request.Context()); err == nil && cfg != nil {
		savedConfig = cfg
	}

	if req.SMTPHost == "" && savedConfig != nil {
		req.SMTPHost = savedConfig.Host
	}
	if req.SMTPPort <= 0 {
		if savedConfig != nil && savedConfig.Port > 0 {
			req.SMTPPort = savedConfig.Port
		} else {
			req.SMTPPort = 587
		}
	}
	if req.SMTPUsername == "" && savedConfig != nil {
		req.SMTPUsername = savedConfig.Username
	}
	password := strings.TrimSpace(req.SMTPPassword)
	if password == "" && savedConfig != nil {
		password = savedConfig.Password
	}
	if req.SMTPFrom == "" && savedConfig != nil {
		req.SMTPFrom = savedConfig.From
	}
	if req.SMTPFromName == "" && savedConfig != nil {
		req.SMTPFromName = savedConfig.FromName
	}
	if req.SMTPHost == "" {
		response.BadRequest(c, "SMTP host is required")
		return
	}

	config := &service.SMTPConfig{
		Host:     req.SMTPHost,
		Port:     req.SMTPPort,
		Username: req.SMTPUsername,
		Password: password,
		From:     req.SMTPFrom,
		FromName: req.SMTPFromName,
		UseTLS:   req.SMTPUseTLS,
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

// ListEmailTemplates returns all editable notification email templates.
// GET /api/v1/admin/settings/email-templates
func (h *SettingHandler) ListEmailTemplates(c *gin.Context) {
	if h.notificationEmailService == nil {
		response.InternalError(c, "notification email service is not configured")
		return
	}
	events := h.notificationEmailService.ListEventInfos()
	templates, err := h.notificationEmailService.ListTemplates(c.Request.Context())
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, dto.EmailTemplateListResponse{
		Events:       emailTemplateEventOptionsToDTO(events),
		Locales:      h.notificationEmailService.SupportedLocales(),
		Templates:    emailTemplateSummariesToDTO(templates),
		Placeholders: emailTemplatePlaceholderUnion(events),
	})
}

// GetEmailTemplate returns one editable notification email template.
// GET /api/v1/admin/settings/email-templates/:event/:locale
func (h *SettingHandler) GetEmailTemplate(c *gin.Context) {
	if h.notificationEmailService == nil {
		response.InternalError(c, "notification email service is not configured")
		return
	}
	tmpl, err := h.notificationEmailService.GetTemplate(c.Request.Context(), c.Param("event"), c.Param("locale"))
	if err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	response.Success(c, emailTemplateDetailToDTO(tmpl))
}

// UpdateEmailTemplate saves an override for one event/locale template.
// PUT /api/v1/admin/settings/email-templates/:event/:locale
func (h *SettingHandler) UpdateEmailTemplate(c *gin.Context) {
	if h.notificationEmailService == nil {
		response.InternalError(c, "notification email service is not configured")
		return
	}
	var req dto.UpdateEmailTemplateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}
	tmpl, err := h.notificationEmailService.UpdateTemplate(c.Request.Context(), c.Param("event"), c.Param("locale"), req.Subject, req.HTML)
	if err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	response.Success(c, emailTemplateDetailToDTO(tmpl))
}

// RestoreOfficialEmailTemplate removes an override and returns the built-in template.
// POST /api/v1/admin/settings/email-templates/:event/:locale/restore-official
func (h *SettingHandler) RestoreOfficialEmailTemplate(c *gin.Context) {
	if h.notificationEmailService == nil {
		response.InternalError(c, "notification email service is not configured")
		return
	}
	tmpl, err := h.notificationEmailService.RestoreOfficialTemplate(c.Request.Context(), c.Param("event"), c.Param("locale"))
	if err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	response.Success(c, emailTemplateDetailToDTO(tmpl))
}

// PreviewEmailTemplate renders a template with safe sample variables without saving it.
// POST /api/v1/admin/settings/email-templates/preview
func (h *SettingHandler) PreviewEmailTemplate(c *gin.Context) {
	if h.notificationEmailService == nil {
		response.InternalError(c, "notification email service is not configured")
		return
	}
	var req dto.PreviewEmailTemplateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}
	preview, err := h.notificationEmailService.PreviewTemplate(c.Request.Context(), service.NotificationEmailPreviewInput{
		Event:     req.Event,
		Locale:    req.Locale,
		Subject:   req.Subject,
		HTML:      req.HTML,
		Variables: req.Variables,
	})
	if err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	response.Success(c, dto.EmailTemplatePreviewResponse{Subject: preview.Subject, HTML: preview.HTML})
}

func emailTemplateEventOptionsToDTO(events []service.NotificationEmailEventInfo) []dto.EmailTemplateEventOption {
	items := make([]dto.EmailTemplateEventOption, 0, len(events))
	for _, event := range events {
		items = append(items, dto.EmailTemplateEventOption{
			Value:       event.Event,
			Label:       event.Label,
			Description: event.Description,
			Category:    event.Category,
			Optional:    event.Optional,
		})
	}
	return items
}

func emailTemplateSummariesToDTO(templates []service.NotificationEmailTemplate) []dto.EmailTemplateSummary {
	items := make([]dto.EmailTemplateSummary, 0, len(templates))
	for _, tmpl := range templates {
		items = append(items, dto.EmailTemplateSummary{
			Event:     tmpl.Event,
			Locale:    tmpl.Locale,
			Subject:   tmpl.Subject,
			IsCustom:  tmpl.IsCustom,
			UpdatedAt: emailTemplateUpdatedAt(tmpl),
		})
	}
	return items
}

func emailTemplateDetailToDTO(tmpl service.NotificationEmailTemplate) dto.EmailTemplateDetail {
	return dto.EmailTemplateDetail{
		Event:        tmpl.Event,
		Locale:       tmpl.Locale,
		Subject:      tmpl.Subject,
		HTML:         tmpl.HTML,
		IsCustom:     tmpl.IsCustom,
		UpdatedAt:    emailTemplateUpdatedAt(tmpl),
		Placeholders: tmpl.Placeholders,
	}
}

func emailTemplateUpdatedAt(tmpl service.NotificationEmailTemplate) string {
	if tmpl.UpdatedAt == nil {
		return ""
	}
	return tmpl.UpdatedAt.Format("2006-01-02T15:04:05Z07:00")
}

func emailTemplatePlaceholderUnion(events []service.NotificationEmailEventInfo) []string {
	seen := make(map[string]struct{})
	placeholders := make([]string, 0)
	for _, event := range events {
		for _, placeholder := range event.Placeholders {
			if _, ok := seen[placeholder]; ok {
				continue
			}
			seen[placeholder] = struct{}{}
			placeholders = append(placeholders, placeholder)
		}
	}
	return placeholders
}
