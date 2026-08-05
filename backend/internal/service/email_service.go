package service

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"errors"
	"fmt"
	"html"
	"log/slog"
	"math/big"
	"net"
	"net/smtp"
	"net/url"
	"strconv"
	"strings"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

var (
	ErrEmailNotConfigured    = infraerrors.ServiceUnavailable("EMAIL_NOT_CONFIGURED", "email service not configured")
	ErrInvalidVerifyCode     = infraerrors.BadRequest("INVALID_VERIFY_CODE", "invalid or expired verification code")
	ErrVerifyCodeTooFrequent = infraerrors.TooManyRequests("VERIFY_CODE_TOO_FREQUENT", "please wait before requesting a new code")
	ErrVerifyCodeMaxAttempts = infraerrors.TooManyRequests("VERIFY_CODE_MAX_ATTEMPTS", "too many failed attempts, please request a new code")

	// Password reset errors
	ErrInvalidResetToken = infraerrors.BadRequest("INVALID_RESET_TOKEN", "invalid or expired password reset token")
)

// EmailCache defines cache operations for email service
type EmailCache interface {
	GetVerificationCode(ctx context.Context, email string) (*VerificationCodeData, error)
	SetVerificationCode(ctx context.Context, email string, data *VerificationCodeData, ttl time.Duration) error
	DeleteVerificationCode(ctx context.Context, email string) error

	// Notify email verification code methods
	GetNotifyVerifyCode(ctx context.Context, email string) (*VerificationCodeData, error)
	SetNotifyVerifyCode(ctx context.Context, email string, data *VerificationCodeData, ttl time.Duration) error
	DeleteNotifyVerifyCode(ctx context.Context, email string) error

	// Password reset token methods
	GetPasswordResetToken(ctx context.Context, email string) (*PasswordResetTokenData, error)
	SetPasswordResetToken(ctx context.Context, email string, data *PasswordResetTokenData, ttl time.Duration) error
	DeletePasswordResetToken(ctx context.Context, email string) error

	// Password reset email cooldown methods
	// Returns true if in cooldown period (email was sent recently)
	IsPasswordResetEmailInCooldown(ctx context.Context, email string) bool
	SetPasswordResetEmailCooldown(ctx context.Context, email string, ttl time.Duration) error

	// Notify code rate limiting per user
	IncrNotifyCodeUserRate(ctx context.Context, userID int64, window time.Duration) (int64, error)
	GetNotifyCodeUserRate(ctx context.Context, userID int64) (int64, error)
}

// VerificationCodeData represents verification code data
type VerificationCodeData struct {
	Code      string
	Attempts  int
	CreatedAt time.Time
	ExpiresAt time.Time // absolute expiry; used to preserve remaining TTL when updating attempts
}

// PasswordResetTokenData represents password reset token data
type PasswordResetTokenData struct {
	Token     string
	CreatedAt time.Time
}

const (
	verifyCodeTTL         = 15 * time.Minute
	verifyCodeCooldown    = 1 * time.Minute
	maxVerifyCodeAttempts = 5

	// Password reset token settings
	passwordResetTokenTTL = 30 * time.Minute

	// Password reset email cooldown (prevent email bombing)
	passwordResetEmailCooldown = 30 * time.Second
)

// SMTPConfig SMTP配置
type SMTPConfig struct {
	Host     string
	Port     int
	Username string
	Password string
	From     string
	FromName string
	UseTLS   bool
}

// EmailService 邮件服务
type EmailService struct {
	settingRepo              SettingRepository
	cache                    EmailCache
	notificationEmailService *NotificationEmailService
}

// NewEmailService 创建邮件服务实例
func NewEmailService(settingRepo SettingRepository, cache EmailCache) *EmailService {
	return &EmailService{
		settingRepo: settingRepo,
		cache:       cache,
	}
}

func (s *EmailService) SetNotificationEmailService(notificationEmailService *NotificationEmailService) {
	s.notificationEmailService = notificationEmailService
}

func firstEmailLocale(locales []string) string {
	if len(locales) == 0 {
		return ""
	}
	return strings.TrimSpace(locales[0])
}

func emailRecipientName(email string) string {
	trimmed := strings.TrimSpace(email)
	if trimmed == "" {
		return ""
	}
	if at := strings.Index(trimmed, "@"); at > 0 {
		return trimmed[:at]
	}
	return trimmed
}

// GetSMTPConfig 从数据库获取SMTP配置
func (s *EmailService) GetSMTPConfig(ctx context.Context) (*SMTPConfig, error) {
	keys := []string{
		SettingKeySMTPHost,
		SettingKeySMTPPort,
		SettingKeySMTPUsername,
		SettingKeySMTPPassword,
		SettingKeySMTPFrom,
		SettingKeySMTPFromName,
		SettingKeySMTPUseTLS,
	}

	settings, err := s.settingRepo.GetMultiple(ctx, keys)
	if err != nil {
		return nil, fmt.Errorf("get smtp settings: %w", err)
	}

	host := strings.TrimSpace(settings[SettingKeySMTPHost])
	if host == "" {
		return nil, ErrEmailNotConfigured
	}

	port := 587 // 默认端口
	if portStr := settings[SettingKeySMTPPort]; portStr != "" {
		if p, err := strconv.Atoi(portStr); err == nil {
			port = p
		}
	}

	useTLS := settings[SettingKeySMTPUseTLS] == "true"

	return &SMTPConfig{
		Host:     host,
		Port:     port,
		Username: strings.TrimSpace(settings[SettingKeySMTPUsername]),
		Password: strings.TrimSpace(settings[SettingKeySMTPPassword]),
		From:     strings.TrimSpace(settings[SettingKeySMTPFrom]),
		FromName: strings.TrimSpace(settings[SettingKeySMTPFromName]),
		UseTLS:   useTLS,
	}, nil
}

// SendEmail 发送邮件（使用数据库中保存的配置）
func (s *EmailService) SendEmail(ctx context.Context, to, subject, body string) error {
	config, err := s.GetSMTPConfig(ctx)
	if err != nil {
		return err
	}
	return s.SendEmailWithConfig(config, to, subject, body)
}

const smtpDialTimeout = 10 * time.Second
const smtpIOTimeout = 20 * time.Second

// SendEmailWithConfig 使用指定配置发送邮件
func (s *EmailService) SendEmailWithConfig(config *SMTPConfig, to, subject, body string) error {
	message, err := buildSMTPMessage(config, to, subject, body)
	if err != nil {
		return err
	}

	client, err := s.connectSMTP(config)
	if err != nil {
		return err
	}
	defer func() { _ = client.Close() }()

	auth := smtp.PlainAuth("", config.Username, config.Password, config.Host)
	if err = client.Auth(auth); err != nil {
		return fmt.Errorf("smtp auth: %w", err)
	}
	if err = client.Mail(message.envelopeFrom); err != nil {
		return fmt.Errorf("smtp mail: %w", err)
	}
	if err = client.Rcpt(message.envelopeTo); err != nil {
		return fmt.Errorf("smtp rcpt: %w", err)
	}
	w, err := client.Data()
	if err != nil {
		return fmt.Errorf("smtp data: %w", err)
	}
	if _, err = w.Write(message.data); err != nil {
		return fmt.Errorf("write msg: %w", err)
	}
	if err = w.Close(); err != nil {
		return fmt.Errorf("close writer: %w", err)
	}
	// Email is sent successfully after w.Close(), ignore Quit errors
	// Some SMTP servers return non-standard responses on QUIT
	_ = client.Quit()
	return nil
}

// smtpTestRootCAs 仅供单元测试注入自签 CA，生产环境始终为 nil（走系统信任链）。
var smtpTestRootCAs *x509.CertPool

func smtpTLSConfig(host string) *tls.Config {
	return &tls.Config{
		ServerName: host,
		// 强制 TLS 1.2+，避免协议降级导致的弱加密风险。
		MinVersion: tls.VersionTLS12,
		RootCAs:    smtpTestRootCAs,
	}
}

// connectSMTP 按配置建立 SMTP 会话，发送与测试连接共用此路径，
// 保证"测试连接成功 ⇔ 实际发信可用"：
//   - UseTLS=true：先尝试隐式 TLS（465 语义）；若服务器以明文应答
//     （587/25 等提交端口的 STARTTLS 语义），自动改走"明文连接 + 强制 STARTTLS"。
//     两种方式都无法建立加密连接时报错，绝不明文继续。
//   - UseTLS=false：明文连接后若服务器支持 STARTTLS 则机会式升级，
//     与 smtp.SendMail 的默认行为一致。
func (s *EmailService) connectSMTP(config *SMTPConfig) (*smtp.Client, error) {
	addr := fmt.Sprintf("%s:%d", config.Host, config.Port)
	dialer := &net.Dialer{Timeout: smtpDialTimeout}
	tlsConfig := smtpTLSConfig(config.Host)

	if config.UseTLS {
		conn, err := tls.DialWithDialer(dialer, "tcp", addr, tlsConfig)
		if err == nil {
			return newSMTPClient(conn, config.Host)
		}
		var recordErr tls.RecordHeaderError
		if !errors.As(err, &recordErr) {
			return nil, fmt.Errorf("tls dial: %w", err)
		}
		// SMTP 服务器先发问候语：明文问候会让 TLS 握手立刻返回
		// RecordHeaderError，据此可靠判定对端期望 STARTTLS。
		return s.connectSMTPStartTLS(dialer, addr, config.Host, tlsConfig, true)
	}

	return s.connectSMTPStartTLS(dialer, addr, config.Host, tlsConfig, false)
}

// connectSMTPStartTLS 建立明文连接并按需升级 STARTTLS。
// mandatory 为 true 时服务器必须支持 STARTTLS，否则报错。
func (s *EmailService) connectSMTPStartTLS(dialer *net.Dialer, addr, host string, tlsConfig *tls.Config, mandatory bool) (*smtp.Client, error) {
	conn, err := dialer.Dial("tcp", addr)
	if err != nil {
		return nil, fmt.Errorf("smtp dial: %w", err)
	}
	client, err := newSMTPClient(conn, host)
	if err != nil {
		return nil, err
	}
	if ok, _ := client.Extension("STARTTLS"); !ok {
		if mandatory {
			_ = client.Close()
			return nil, errors.New("smtp server does not support STARTTLS")
		}
		return client, nil
	}
	if err := client.StartTLS(tlsConfig); err != nil {
		_ = client.Close()
		return nil, fmt.Errorf("starttls: %w", err)
	}
	return client, nil
}

func newSMTPClient(conn net.Conn, host string) (*smtp.Client, error) {
	_ = conn.SetDeadline(time.Now().Add(smtpIOTimeout))
	client, err := smtp.NewClient(conn, host)
	if err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("new smtp client: %w", err)
	}
	return client, nil
}

// GenerateVerifyCode 生成6位数字验证码
func (s *EmailService) GenerateVerifyCode() (string, error) {
	const digits = "0123456789"
	code := make([]byte, 6)
	for i := range code {
		num, err := rand.Int(rand.Reader, big.NewInt(int64(len(digits))))
		if err != nil {
			return "", err
		}
		code[i] = digits[num.Int64()]
	}
	return string(code), nil
}

// SendVerifyCode 发送验证码邮件
func (s *EmailService) SendVerifyCode(ctx context.Context, email, siteName string, locale ...string) error {
	// 检查是否在冷却期内
	existing, err := s.cache.GetVerificationCode(ctx, email)
	if err == nil && existing != nil {
		if time.Since(existing.CreatedAt) < verifyCodeCooldown {
			return ErrVerifyCodeTooFrequent
		}
	}

	// 生成验证码
	code, err := s.GenerateVerifyCode()
	if err != nil {
		return fmt.Errorf("generate code: %w", err)
	}

	// 保存验证码到 Redis
	data := &VerificationCodeData{
		Code:      code,
		Attempts:  0,
		CreatedAt: time.Now(),
		ExpiresAt: time.Now().Add(verifyCodeTTL),
	}
	if err := s.cache.SetVerificationCode(ctx, email, data, verifyCodeTTL); err != nil {
		return fmt.Errorf("save verify code: %w", err)
	}

	if s.notificationEmailService != nil {
		err := s.notificationEmailService.Send(ctx, NotificationEmailSendInput{
			Event:          NotificationEmailEventAuthVerifyCode,
			Locale:         firstEmailLocale(locale),
			RecipientEmail: email,
			RecipientName:  emailRecipientName(email),
			Variables: map[string]string{
				"verification_code":  code,
				"expires_in_minutes": strconv.Itoa(int(verifyCodeTTL / time.Minute)),
			},
		})
		if err == nil {
			return nil
		}
		if !shouldFallbackNotificationEmail(err) {
			return err
		}
		slog.Warn("failed to send templated verification email, falling back to legacy template", "recipient_hash", notificationEmailHash(email), "error", err)
	}

	// 构建邮件内容
	subject := fmt.Sprintf("[%s] Email Verification Code", siteName)
	body := s.buildVerifyCodeEmailBody(code, siteName)

	// 发送邮件
	if err := s.SendEmail(ctx, email, subject, body); err != nil {
		return fmt.Errorf("send email: %w", err)
	}

	return nil
}

// VerifyCode 验证验证码
func (s *EmailService) VerifyCode(ctx context.Context, email, code string) error {
	data, err := s.cache.GetVerificationCode(ctx, email)
	if err != nil || data == nil {
		return ErrInvalidVerifyCode
	}

	// 检查是否已达到最大尝试次数
	if data.Attempts >= maxVerifyCodeAttempts {
		return ErrVerifyCodeMaxAttempts
	}

	// 验证码不匹配 (constant-time comparison to prevent timing attacks)
	if subtle.ConstantTimeCompare([]byte(data.Code), []byte(code)) != 1 {
		data.Attempts++
		remaining := time.Until(data.ExpiresAt)
		if remaining <= 0 {
			return ErrInvalidVerifyCode
		}
		if err := s.cache.SetVerificationCode(ctx, email, data, remaining); err != nil {
			slog.Error("failed to update verification attempt count", "email", email, "error", err)
		}
		if data.Attempts >= maxVerifyCodeAttempts {
			return ErrVerifyCodeMaxAttempts
		}
		return ErrInvalidVerifyCode
	}

	// 验证成功，删除验证码
	if err := s.cache.DeleteVerificationCode(ctx, email); err != nil {
		slog.Error("failed to delete verification code after success", "email", email, "error", err)
	}
	return nil
}

// buildVerifyCodeEmailBody 构建验证码邮件HTML内容
func (s *EmailService) buildVerifyCodeEmailBody(code, siteName string) string {
	return fmt.Sprintf(`
<!DOCTYPE html>
<html>
<head>
    <meta charset="UTF-8">
    <style>
        body { font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, Oxygen, Ubuntu, sans-serif; background-color: #f5f5f5; margin: 0; padding: 20px; }
        .container { max-width: 600px; margin: 0 auto; background-color: #ffffff; border-radius: 8px; overflow: hidden; box-shadow: 0 2px 8px rgba(0,0,0,0.1); }
        .header { background: linear-gradient(135deg, #667eea 0%%, #764ba2 100%%); color: white; padding: 30px; text-align: center; }
        .header h1 { margin: 0; font-size: 24px; }
        .content { padding: 40px 30px; text-align: center; }
        .code { font-size: 36px; font-weight: bold; letter-spacing: 8px; color: #333; background-color: #f8f9fa; padding: 20px 30px; border-radius: 8px; display: inline-block; margin: 20px 0; font-family: monospace; }
        .info { color: #666; font-size: 14px; line-height: 1.6; margin-top: 20px; }
        .footer { background-color: #f8f9fa; padding: 20px; text-align: center; color: #999; font-size: 12px; }
    </style>
</head>
<body>
    <div class="container">
        <div class="header">
            <h1>%s</h1>
        </div>
        <div class="content">
            <p style="font-size: 18px; color: #333;">Your verification code is:</p>
            <div class="code">%s</div>
            <div class="info">
                <p>This code will expire in <strong>15 minutes</strong>.</p>
                <p>If you did not request this code, please ignore this email.</p>
            </div>
        </div>
        <div class="footer">
            <p>This is an automated message, please do not reply.</p>
        </div>
    </div>
</body>
</html>
`, html.EscapeString(siteName), code)
}

// TestSMTPConnectionWithConfig 使用指定配置测试SMTP连接。
// 与 SendEmailWithConfig 共用 connectSMTP 建连（含 STARTTLS 升级逻辑），
// 避免出现"测试连接失败但实际发信成功"的不一致。
func (s *EmailService) TestSMTPConnectionWithConfig(config *SMTPConfig) error {
	client, err := s.connectSMTP(config)
	if err != nil {
		return fmt.Errorf("smtp connection failed: %w", err)
	}
	defer func() { _ = client.Close() }()

	auth := smtp.PlainAuth("", config.Username, config.Password, config.Host)
	if err := client.Auth(auth); err != nil {
		return fmt.Errorf("smtp authentication failed: %w", err)
	}

	// 认证成功即视为连接可用；与发送路径一致，忽略 QUIT 的非标准响应。
	_ = client.Quit()
	return nil
}

// GeneratePasswordResetToken generates a secure 32-byte random token (64 hex characters)
func (s *EmailService) GeneratePasswordResetToken() (string, error) {
	bytes := make([]byte, 32)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return hex.EncodeToString(bytes), nil
}

// SendPasswordResetEmail sends a password reset email with a reset link
func (s *EmailService) SendPasswordResetEmail(ctx context.Context, email, siteName, resetURL string, locale ...string) error {
	var token string
	var needSaveToken bool

	// Check if token already exists
	existing, err := s.cache.GetPasswordResetToken(ctx, email)
	if err == nil && existing != nil {
		// Token exists, reuse it (allows resending email without generating new token)
		token = existing.Token
		needSaveToken = false
	} else {
		// Generate new token
		token, err = s.GeneratePasswordResetToken()
		if err != nil {
			return fmt.Errorf("generate token: %w", err)
		}
		needSaveToken = true
	}

	// Save token to Redis (only if new token generated)
	if needSaveToken {
		data := &PasswordResetTokenData{
			Token:     token,
			CreatedAt: time.Now(),
		}
		if err := s.cache.SetPasswordResetToken(ctx, email, data, passwordResetTokenTTL); err != nil {
			return fmt.Errorf("save reset token: %w", err)
		}
	}

	// Build full reset URL with URL-encoded token and email
	fullResetURL := fmt.Sprintf("%s?email=%s&token=%s", resetURL, url.QueryEscape(email), url.QueryEscape(token))

	if s.notificationEmailService != nil {
		err := s.notificationEmailService.Send(ctx, NotificationEmailSendInput{
			Event:          NotificationEmailEventAuthPasswordReset,
			Locale:         firstEmailLocale(locale),
			RecipientEmail: email,
			RecipientName:  emailRecipientName(email),
			Variables: map[string]string{
				"reset_url":          fullResetURL,
				"expires_in_minutes": strconv.Itoa(int(passwordResetTokenTTL / time.Minute)),
			},
		})
		if err == nil {
			return nil
		}
		if !shouldFallbackNotificationEmail(err) {
			return err
		}
		slog.Warn("failed to send templated password reset email, falling back to legacy template", "recipient_hash", notificationEmailHash(email), "error", err)
	}

	// Build email content
	subject := fmt.Sprintf("[%s] 密码重置请求", siteName)
	body := s.buildPasswordResetEmailBody(fullResetURL, siteName)

	// Send email
	if err := s.SendEmail(ctx, email, subject, body); err != nil {
		return fmt.Errorf("send email: %w", err)
	}

	return nil
}

// SendPasswordResetEmailWithCooldown sends password reset email with cooldown check (called by queue worker)
// This method wraps SendPasswordResetEmail with email cooldown to prevent email bombing
func (s *EmailService) SendPasswordResetEmailWithCooldown(ctx context.Context, email, siteName, resetURL string, locale ...string) error {
	// Check email cooldown to prevent email bombing
	if s.cache.IsPasswordResetEmailInCooldown(ctx, email) {
		slog.Info("password reset email skipped due to cooldown", "email", email)
		return nil // Silent success to prevent revealing cooldown to attackers
	}

	// Send email using core method
	if err := s.SendPasswordResetEmail(ctx, email, siteName, resetURL, firstEmailLocale(locale)); err != nil {
		return err
	}

	// Set cooldown marker (Redis TTL handles expiration)
	if err := s.cache.SetPasswordResetEmailCooldown(ctx, email, passwordResetEmailCooldown); err != nil {
		slog.Error("failed to set password reset cooldown", "email", email, "error", err)
	}

	return nil
}

// VerifyPasswordResetToken verifies the password reset token without consuming it
func (s *EmailService) VerifyPasswordResetToken(ctx context.Context, email, token string) error {
	data, err := s.cache.GetPasswordResetToken(ctx, email)
	if err != nil || data == nil {
		return ErrInvalidResetToken
	}

	// Use constant-time comparison to prevent timing attacks
	if subtle.ConstantTimeCompare([]byte(data.Token), []byte(token)) != 1 {
		return ErrInvalidResetToken
	}

	return nil
}

// ConsumePasswordResetToken verifies and deletes the token (one-time use)
func (s *EmailService) ConsumePasswordResetToken(ctx context.Context, email, token string) error {
	// Verify first
	if err := s.VerifyPasswordResetToken(ctx, email, token); err != nil {
		return err
	}

	// Delete after verification (one-time use)
	if err := s.cache.DeletePasswordResetToken(ctx, email); err != nil {
		slog.Error("failed to delete password reset token after consumption", "email", email, "error", err)
	}
	return nil
}

// buildPasswordResetEmailBody builds the HTML content for password reset email
func (s *EmailService) buildPasswordResetEmailBody(resetURL, siteName string) string {
	return fmt.Sprintf(`
<!DOCTYPE html>
<html>
<head>
    <meta charset="UTF-8">
    <style>
        body { font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, Oxygen, Ubuntu, sans-serif; background-color: #f5f5f5; margin: 0; padding: 20px; }
        .container { max-width: 600px; margin: 0 auto; background-color: #ffffff; border-radius: 8px; overflow: hidden; box-shadow: 0 2px 8px rgba(0,0,0,0.1); }
        .header { background: linear-gradient(135deg, #667eea 0%%, #764ba2 100%%); color: white; padding: 30px; text-align: center; }
        .header h1 { margin: 0; font-size: 24px; }
        .content { padding: 40px 30px; text-align: center; }
        .button { display: inline-block; background: linear-gradient(135deg, #667eea 0%%, #764ba2 100%%); color: white; padding: 14px 32px; text-decoration: none; border-radius: 8px; font-size: 16px; font-weight: 600; margin: 20px 0; }
        .button:hover { opacity: 0.9; }
        .info { color: #666; font-size: 14px; line-height: 1.6; margin-top: 20px; }
        .link-fallback { color: #666; font-size: 12px; word-break: break-all; margin-top: 20px; padding: 15px; background-color: #f8f9fa; border-radius: 4px; }
        .footer { background-color: #f8f9fa; padding: 20px; text-align: center; color: #999; font-size: 12px; }
        .warning { color: #e74c3c; font-weight: 500; }
    </style>
</head>
<body>
    <div class="container">
        <div class="header">
            <h1>%s</h1>
        </div>
        <div class="content">
            <p style="font-size: 18px; color: #333;">密码重置请求</p>
            <p style="color: #666;">您已请求重置密码。请点击下方按钮设置新密码：</p>
            <a href="%s" class="button">重置密码</a>
            <div class="info">
                <p>此链接将在 <strong>30 分钟</strong>后失效。</p>
                <p class="warning">如果您没有请求重置密码，请忽略此邮件。您的密码将保持不变。</p>
            </div>
            <div class="link-fallback">
                <p>如果按钮无法点击，请复制以下链接到浏览器中打开：</p>
                <p>%s</p>
            </div>
        </div>
        <div class="footer">
            <p>这是一封自动发送的邮件，请勿回复。</p>
        </div>
    </div>
</body>
</html>
`, html.EscapeString(siteName), html.EscapeString(resetURL), html.EscapeString(resetURL))
}
