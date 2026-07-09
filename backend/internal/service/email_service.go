package service

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"crypto/tls"
	"encoding/hex"
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
REDACTED

// VerificationCodeData represents verification code data
type VerificationCodeData struct {
	Code      string
	Attempts  int
	CreatedAt time.Time
	ExpiresAt time.Time // absolute expiry; used to preserve remaining TTL when updating attempts
REDACTED

// PasswordResetTokenData represents password reset token data
type PasswordResetTokenData struct {
	Token     string
	CreatedAt time.Time
REDACTED

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
REDACTED

// EmailService 邮件服务
type EmailService struct {
	settingRepo              SettingRepository
	cache                    EmailCache
	notificationEmailService *NotificationEmailService
REDACTED

// NewEmailService 创建邮件服务实例
func NewEmailService(settingRepo SettingRepository, cache EmailCache) *EmailService {
	return &EmailService{
		settingRepo: settingRepo,
		cache:       cache,
REDACTED
REDACTED

func (s *EmailService) SetNotificationEmailService(notificationEmailService *NotificationEmailService) {
	s.notificationEmailService = notificationEmailService
REDACTED

func firstEmailLocale(locales []string) string {
	if len(locales) == 0 {
		return ""
REDACTED
	return strings.TrimSpace(locales[0])
REDACTED

func emailRecipientName(email string) string {
	trimmed := strings.TrimSpace(email)
	if trimmed == "" {
		return ""
REDACTED
	if at := strings.Index(trimmed, "@"); at > 0 {
		return trimmed[:at]
REDACTED
	return trimmed
REDACTED

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
REDACTED

	settings, err := s.settingRepo.GetMultiple(ctx, keys)
	if err != nil {
		return nil, fmt.Errorf("get smtp settings: %w", err)
REDACTED

	host := strings.TrimSpace(settings[SettingKeySMTPHost])
	if host == "" {
		return nil, ErrEmailNotConfigured
REDACTED

	port := 587 // 默认端口
	if portStr := settings[SettingKeySMTPPort]; portStr != "" {
		if p, err := strconv.Atoi(portStr); err == nil {
			port = p
	REDACTED
REDACTED

	useTLS := settings[SettingKeySMTPUseTLS] == "true"

	return &SMTPConfig{
		Host:     host,
		Port:     port,
		Username: strings.TrimSpace(settings[SettingKeySMTPUsername]),
		Password: strings.TrimSpace(settings[SettingKeySMTPPassword]),
		From:     strings.TrimSpace(settings[SettingKeySMTPFrom]),
		FromName: strings.TrimSpace(settings[SettingKeySMTPFromName]),
		UseTLS:   useTLS,
REDACTED, nil
REDACTED

// SendEmail 发送邮件（使用数据库中保存的配置）
func (s *EmailService) SendEmail(ctx context.Context, to, subject, body string) error {
	config, err := s.GetSMTPConfig(ctx)
	if err != nil {
		return err
REDACTED
	return s.SendEmailWithConfig(config, to, subject, body)
REDACTED

const smtpDialTimeout = 10 * time.Second
const smtpIOTimeout = 20 * time.Second

// SendEmailWithConfig 使用指定配置发送邮件
func (s *EmailService) SendEmailWithConfig(config *SMTPConfig, to, subject, body string) error {
	// Sanitize all SMTP header fields to prevent header injection (CR/LF removal).
	to = sanitizeEmailHeader(to)
	subject = sanitizeEmailHeader(subject)

	from := sanitizeEmailHeader(config.From)
	if config.FromName != "" {
		from = fmt.Sprintf("%s <%s>", sanitizeEmailHeader(config.FromName), sanitizeEmailHeader(config.From))
REDACTED

	msg := fmt.Sprintf("From: %s\r\nTo: %s\r\nSubject: %s\r\nMIME-Version: 1.0\r\nContent-Type: text/html; charset=UTF-8\r\n\r\n%s",
		from, to, subject, body)

	addr := fmt.Sprintf("%s:%d", config.Host, config.Port)
	auth := smtp.PlainAuth("", config.Username, config.Password, config.Host)

	if config.UseTLS {
		return s.sendMailTLS(addr, auth, config.From, to, []byte(msg), config.Host)
REDACTED

	return s.sendMailPlain(addr, auth, config.From, to, []byte(msg), config.Host)
REDACTED

// sendMailPlain sends mail without TLS using a dialer with timeout.
func (s *EmailService) sendMailPlain(addr string, auth smtp.Auth, from, to string, msg []byte, host string) error {
	dialer := &net.Dialer{Timeout: smtpDialTimeoutREDACTED
	conn, err := dialer.Dial("tcp", addr)
	if err != nil {
		return fmt.Errorf("smtp dial: %w", err)
REDACTED
	_ = conn.SetDeadline(time.Now().Add(smtpIOTimeout))
	defer func() { _ = conn.Close() REDACTED()

	client, err := smtp.NewClient(conn, host)
	if err != nil {
		return fmt.Errorf("new smtp client: %w", err)
REDACTED
	defer func() { _ = client.Close() REDACTED()

	// Opportunistic STARTTLS: upgrade to encrypted connection if the server supports it.
	// This mirrors the behavior of smtp.SendMail which we replaced for timeout support.
	if ok, _ := client.Extension("STARTTLS"); ok {
		if err = client.StartTLS(&tls.Config{ServerName: host, MinVersion: tls.VersionTLS12REDACTED); err != nil {
			return fmt.Errorf("starttls: %w", err)
	REDACTED
REDACTED

	if err = client.Auth(auth); err != nil {
		return fmt.Errorf("smtp auth: %w", err)
REDACTED
	if err = client.Mail(from); err != nil {
		return fmt.Errorf("smtp mail: %w", err)
REDACTED
	if err = client.Rcpt(to); err != nil {
		return fmt.Errorf("smtp rcpt: %w", err)
REDACTED
	w, err := client.Data()
	if err != nil {
		return fmt.Errorf("smtp data: %w", err)
REDACTED
	if _, err = w.Write(msg); err != nil {
		return fmt.Errorf("write msg: %w", err)
REDACTED
	if err = w.Close(); err != nil {
		return fmt.Errorf("close writer: %w", err)
REDACTED
	_ = client.Quit()
	return nil
REDACTED

// sendMailTLS 使用TLS发送邮件
func (s *EmailService) sendMailTLS(addr string, auth smtp.Auth, from, to string, msg []byte, host string) error {
	tlsConfig := &tls.Config{
		ServerName: host,
		// 强制 TLS 1.2+，避免协议降级导致的弱加密风险。
		MinVersion: tls.VersionTLS12,
REDACTED

	dialer := &net.Dialer{Timeout: smtpDialTimeoutREDACTED
	conn, err := tls.DialWithDialer(dialer, "tcp", addr, tlsConfig)
	if err != nil {
		return fmt.Errorf("tls dial: %w", err)
REDACTED
	_ = conn.SetDeadline(time.Now().Add(smtpIOTimeout))
	defer func() { _ = conn.Close() REDACTED()

	client, err := smtp.NewClient(conn, host)
	if err != nil {
		return fmt.Errorf("new smtp client: %w", err)
REDACTED
	defer func() { _ = client.Close() REDACTED()

	if err = client.Auth(auth); err != nil {
		return fmt.Errorf("smtp auth: %w", err)
REDACTED

	if err = client.Mail(from); err != nil {
		return fmt.Errorf("smtp mail: %w", err)
REDACTED

	if err = client.Rcpt(to); err != nil {
		return fmt.Errorf("smtp rcpt: %w", err)
REDACTED

	w, err := client.Data()
	if err != nil {
		return fmt.Errorf("smtp data: %w", err)
REDACTED

	_, err = w.Write(msg)
	if err != nil {
		return fmt.Errorf("write msg: %w", err)
REDACTED

	err = w.Close()
	if err != nil {
		return fmt.Errorf("close writer: %w", err)
REDACTED

	// Email is sent successfully after w.Close(), ignore Quit errors
	// Some SMTP servers return non-standard responses on QUIT
	_ = client.Quit()
	return nil
REDACTED

// GenerateVerifyCode 生成6位数字验证码
func (s *EmailService) GenerateVerifyCode() (string, error) {
	const digits = "0123456789"
	code := make([]byte, 6)
	for i := range code {
		num, err := rand.Int(rand.Reader, big.NewInt(int64(len(digits))))
		if err != nil {
			return "", err
	REDACTED
		code[i] = digits[num.Int64()]
REDACTED
	return string(code), nil
REDACTED

// SendVerifyCode 发送验证码邮件
func (s *EmailService) SendVerifyCode(ctx context.Context, email, siteName string, locale ...string) error {
	// 检查是否在冷却期内
	existing, err := s.cache.GetVerificationCode(ctx, email)
	if err == nil && existing != nil {
		if time.Since(existing.CreatedAt) < verifyCodeCooldown {
			return ErrVerifyCodeTooFrequent
	REDACTED
REDACTED

	// 生成验证码
	code, err := s.GenerateVerifyCode()
	if err != nil {
		return fmt.Errorf("generate code: %w", err)
REDACTED

	// 保存验证码到 Redis
	data := &VerificationCodeData{
		Code:      code,
		Attempts:  0,
		CreatedAt: time.Now(),
		ExpiresAt: time.Now().Add(verifyCodeTTL),
REDACTED
	if err := s.cache.SetVerificationCode(ctx, email, data, verifyCodeTTL); err != nil {
		return fmt.Errorf("save verify code: %w", err)
REDACTED

	if s.notificationEmailService != nil {
		err := s.notificationEmailService.Send(ctx, NotificationEmailSendInput{
			Event:          NotificationEmailEventAuthVerifyCode,
			Locale:         firstEmailLocale(locale),
			RecipientEmail: email,
			RecipientName:  emailRecipientName(email),
			Variables: map[string]string{
				"verification_code":  code,
				"expires_in_minutes": strconv.Itoa(int(verifyCodeTTL / time.Minute)),
		REDACTED,
	REDACTED)
		if err == nil {
			return nil
	REDACTED
		if !shouldFallbackNotificationEmail(err) {
			return err
	REDACTED
		slog.Warn("failed to send templated verification email, falling back to legacy template", "recipient_hash", notificationEmailHash(email), "error", err)
REDACTED

	// 构建邮件内容
	subject := fmt.Sprintf("[%s] Email Verification Code", siteName)
	body := s.buildVerifyCodeEmailBody(code, siteName)

	// 发送邮件
	if err := s.SendEmail(ctx, email, subject, body); err != nil {
		return fmt.Errorf("send email: %w", err)
REDACTED

	return nil
REDACTED

// VerifyCode 验证验证码
func (s *EmailService) VerifyCode(ctx context.Context, email, code string) error {
	data, err := s.cache.GetVerificationCode(ctx, email)
	if err != nil || data == nil {
		return ErrInvalidVerifyCode
REDACTED

	// 检查是否已达到最大尝试次数
	if data.Attempts >= maxVerifyCodeAttempts {
		return ErrVerifyCodeMaxAttempts
REDACTED

	// 验证码不匹配 (constant-time comparison to prevent timing attacks)
	if subtle.ConstantTimeCompare([]byte(data.Code), []byte(code)) != 1 {
		data.Attempts++
		remaining := time.Until(data.ExpiresAt)
		if remaining <= 0 {
			return ErrInvalidVerifyCode
	REDACTED
		if err := s.cache.SetVerificationCode(ctx, email, data, remaining); err != nil {
			slog.Error("failed to update verification attempt count", "email", email, "error", err)
	REDACTED
		if data.Attempts >= maxVerifyCodeAttempts {
			return ErrVerifyCodeMaxAttempts
	REDACTED
		return ErrInvalidVerifyCode
REDACTED

	// 验证成功，删除验证码
	if err := s.cache.DeleteVerificationCode(ctx, email); err != nil {
		slog.Error("failed to delete verification code after success", "email", email, "error", err)
REDACTED
	return nil
REDACTED

// buildVerifyCodeEmailBody 构建验证码邮件HTML内容
func (s *EmailService) buildVerifyCodeEmailBody(code, siteName string) string {
	return fmt.Sprintf(`
<!DOCTYPE html>
<html>
<head>
    <meta charset="UTF-8">
    <style>
        body { font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, Oxygen, Ubuntu, sans-serif; background-color: #f5f5f5; margin: 0; padding: 20px; REDACTED
        .container { max-width: 600px; margin: 0 auto; background-color: #ffffff; border-radius: 8px; overflow: hidden; box-shadow: 0 2px 8px rgba(0,0,0,0.1); REDACTED
        .header { background: linear-gradient(135deg, #667eea 0%%, #764ba2 100%%); color: white; padding: 30px; text-align: center; REDACTED
        .header h1 { margin: 0; font-size: 24px; REDACTED
        .content { padding: 40px 30px; text-align: center; REDACTED
        .code { font-size: 36px; font-weight: bold; letter-spacing: 8px; color: #333; background-color: #f8f9fa; padding: 20px 30px; border-radius: 8px; display: inline-block; margin: 20px 0; font-family: monospace; REDACTED
        .info { color: #666; font-size: 14px; line-height: 1.6; margin-top: 20px; REDACTED
        .footer { background-color: #f8f9fa; padding: 20px; text-align: center; color: #999; font-size: 12px; REDACTED
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
REDACTED

// TestSMTPConnectionWithConfig 使用指定配置测试SMTP连接
func (s *EmailService) TestSMTPConnectionWithConfig(config *SMTPConfig) error {
	addr := fmt.Sprintf("%s:%d", config.Host, config.Port)

	if config.UseTLS {
		tlsConfig := &tls.Config{
			ServerName: config.Host,
			// 与发送逻辑一致，显式要求 TLS 1.2+。
			MinVersion: tls.VersionTLS12,
	REDACTED
		conn, err := tls.Dial("tcp", addr, tlsConfig)
		if err != nil {
			return fmt.Errorf("tls connection failed: %w", err)
	REDACTED
		defer func() { _ = conn.Close() REDACTED()

		client, err := smtp.NewClient(conn, config.Host)
		if err != nil {
			return fmt.Errorf("smtp client creation failed: %w", err)
	REDACTED
		defer func() { _ = client.Close() REDACTED()

		auth := smtp.PlainAuth("", config.Username, config.Password, config.Host)
		if err = client.Auth(auth); err != nil {
			return fmt.Errorf("smtp authentication failed: %w", err)
	REDACTED

		return client.Quit()
REDACTED

	// 非TLS连接测试
	client, err := smtp.Dial(addr)
	if err != nil {
		return fmt.Errorf("smtp connection failed: %w", err)
REDACTED
	defer func() { _ = client.Close() REDACTED()

	auth := smtp.PlainAuth("", config.Username, config.Password, config.Host)
	if err = client.Auth(auth); err != nil {
		return fmt.Errorf("smtp authentication failed: %w", err)
REDACTED

	return client.Quit()
REDACTED

// GeneratePasswordResetToken generates a secure 32-byte random token (64 hex characters)
func (s *EmailService) GeneratePasswordResetToken() (string, error) {
	bytes := make([]byte, 32)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
REDACTED
	return hex.EncodeToString(bytes), nil
REDACTED

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
REDACTED else {
		// Generate new token
		token, err = s.GeneratePasswordResetToken()
		if err != nil {
			return fmt.Errorf("generate token: %w", err)
	REDACTED
		needSaveToken = true
REDACTED

	// Save token to Redis (only if new token generated)
	if needSaveToken {
		data := &PasswordResetTokenData{
			Token:     token,
			CreatedAt: time.Now(),
	REDACTED
		if err := s.cache.SetPasswordResetToken(ctx, email, data, passwordResetTokenTTL); err != nil {
			return fmt.Errorf("save reset token: %w", err)
	REDACTED
REDACTED

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
		REDACTED,
	REDACTED)
		if err == nil {
			return nil
	REDACTED
		if !shouldFallbackNotificationEmail(err) {
			return err
	REDACTED
		slog.Warn("failed to send templated password reset email, falling back to legacy template", "recipient_hash", notificationEmailHash(email), "error", err)
REDACTED

	// Build email content
	subject := fmt.Sprintf("[%s] 密码重置请求", siteName)
	body := s.buildPasswordResetEmailBody(fullResetURL, siteName)

	// Send email
	if err := s.SendEmail(ctx, email, subject, body); err != nil {
		return fmt.Errorf("send email: %w", err)
REDACTED

	return nil
REDACTED

// SendPasswordResetEmailWithCooldown sends password reset email with cooldown check (called by queue worker)
// This method wraps SendPasswordResetEmail with email cooldown to prevent email bombing
func (s *EmailService) SendPasswordResetEmailWithCooldown(ctx context.Context, email, siteName, resetURL string, locale ...string) error {
	// Check email cooldown to prevent email bombing
	if s.cache.IsPasswordResetEmailInCooldown(ctx, email) {
		slog.Info("password reset email skipped due to cooldown", "email", email)
		return nil // Silent success to prevent revealing cooldown to attackers
REDACTED

	// Send email using core method
	if err := s.SendPasswordResetEmail(ctx, email, siteName, resetURL, firstEmailLocale(locale)); err != nil {
		return err
REDACTED

	// Set cooldown marker (Redis TTL handles expiration)
	if err := s.cache.SetPasswordResetEmailCooldown(ctx, email, passwordResetEmailCooldown); err != nil {
		slog.Error("failed to set password reset cooldown", "email", email, "error", err)
REDACTED

	return nil
REDACTED

// VerifyPasswordResetToken verifies the password reset token without consuming it
func (s *EmailService) VerifyPasswordResetToken(ctx context.Context, email, token string) error {
	data, err := s.cache.GetPasswordResetToken(ctx, email)
	if err != nil || data == nil {
		return ErrInvalidResetToken
REDACTED

	// Use constant-time comparison to prevent timing attacks
	if subtle.ConstantTimeCompare([]byte(data.Token), []byte(token)) != 1 {
		return ErrInvalidResetToken
REDACTED

	return nil
REDACTED

// ConsumePasswordResetToken verifies and deletes the token (one-time use)
func (s *EmailService) ConsumePasswordResetToken(ctx context.Context, email, token string) error {
	// Verify first
	if err := s.VerifyPasswordResetToken(ctx, email, token); err != nil {
		return err
REDACTED

	// Delete after verification (one-time use)
	if err := s.cache.DeletePasswordResetToken(ctx, email); err != nil {
		slog.Error("failed to delete password reset token after consumption", "email", email, "error", err)
REDACTED
	return nil
REDACTED

// buildPasswordResetEmailBody builds the HTML content for password reset email
func (s *EmailService) buildPasswordResetEmailBody(resetURL, siteName string) string {
	return fmt.Sprintf(`
<!DOCTYPE html>
<html>
<head>
    <meta charset="UTF-8">
    <style>
        body { font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, Oxygen, Ubuntu, sans-serif; background-color: #f5f5f5; margin: 0; padding: 20px; REDACTED
        .container { max-width: 600px; margin: 0 auto; background-color: #ffffff; border-radius: 8px; overflow: hidden; box-shadow: 0 2px 8px rgba(0,0,0,0.1); REDACTED
        .header { background: linear-gradient(135deg, #667eea 0%%, #764ba2 100%%); color: white; padding: 30px; text-align: center; REDACTED
        .header h1 { margin: 0; font-size: 24px; REDACTED
        .content { padding: 40px 30px; text-align: center; REDACTED
        .button { display: inline-block; background: linear-gradient(135deg, #667eea 0%%, #764ba2 100%%); color: white; padding: 14px 32px; text-decoration: none; border-radius: 8px; font-size: 16px; font-weight: 600; margin: 20px 0; REDACTED
        .button:hover { opacity: 0.9; REDACTED
        .info { color: #666; font-size: 14px; line-height: 1.6; margin-top: 20px; REDACTED
        .link-fallback { color: #666; font-size: 12px; word-break: break-all; margin-top: 20px; padding: 15px; background-color: #f8f9fa; border-radius: 4px; REDACTED
        .footer { background-color: #f8f9fa; padding: 20px; text-align: center; color: #999; font-size: 12px; REDACTED
        .warning { color: #e74c3c; font-weight: 500; REDACTED
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
REDACTED
