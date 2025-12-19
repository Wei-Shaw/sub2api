package service

import (
	"context"
	"crypto/rand"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"net/smtp"
	"strconv"
	"sub2api/internal/model"
	"sub2api/internal/service/ports"
	"time"

	"github.com/redis/go-redis/v9"
)

var (
	ErrEmailNotConfigured    = errors.New("email service not configured")
	ErrInvalidVerifyCode     = errors.New("invalid or expired verification code")
	ErrVerifyCodeTooFrequent = errors.New("please wait before requesting a new code")
	ErrVerifyCodeMaxAttempts = errors.New("too many failed attempts, please request a new code")
)

const (
	verifyCodeKeyPrefix   = "email_verify:"
	verifyCodeTTL         = 15 * time.Minute
	verifyCodeCooldown    = 1 * time.Minute
	maxVerifyCodeAttempts = 5
)

// verifyCodeData Redis 中存储的验证码数据
type verifyCodeData struct {
	Code      string    `json:"code"`
	Attempts  int       `json:"attempts"`
	CreatedAt time.Time `json:"created_at"`
REDACTED

// SmtpConfig SMTP配置
type SmtpConfig struct {
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
	settingRepo ports.SettingRepository
	rdb         *redis.Client
REDACTED

// NewEmailService 创建邮件服务实例
func NewEmailService(settingRepo ports.SettingRepository, rdb *redis.Client) *EmailService {
	return &EmailService{
		settingRepo: settingRepo,
		rdb:         rdb,
REDACTED
REDACTED

// GetSmtpConfig 从数据库获取SMTP配置
func (s *EmailService) GetSmtpConfig(ctx context.Context) (*SmtpConfig, error) {
	keys := []string{
		model.SettingKeySmtpHost,
		model.SettingKeySmtpPort,
		model.SettingKeySmtpUsername,
		model.SettingKeySmtpPassword,
		model.SettingKeySmtpFrom,
		model.SettingKeySmtpFromName,
		model.SettingKeySmtpUseTLS,
REDACTED

	settings, err := s.settingRepo.GetMultiple(ctx, keys)
	if err != nil {
		return nil, fmt.Errorf("get smtp settings: %w", err)
REDACTED

	host := settings[model.SettingKeySmtpHost]
	if host == "" {
		return nil, ErrEmailNotConfigured
REDACTED

	port := 587 // 默认端口
	if portStr := settings[model.SettingKeySmtpPort]; portStr != "" {
		if p, err := strconv.Atoi(portStr); err == nil {
			port = p
	REDACTED
REDACTED

	useTLS := settings[model.SettingKeySmtpUseTLS] == "true"

	return &SmtpConfig{
		Host:     host,
		Port:     port,
		Username: settings[model.SettingKeySmtpUsername],
		Password: settings[model.SettingKeySmtpPassword],
		From:     settings[model.SettingKeySmtpFrom],
		FromName: settings[model.SettingKeySmtpFromName],
		UseTLS:   useTLS,
REDACTED, nil
REDACTED

// SendEmail 发送邮件（使用数据库中保存的配置）
func (s *EmailService) SendEmail(ctx context.Context, to, subject, body string) error {
	config, err := s.GetSmtpConfig(ctx)
	if err != nil {
		return err
REDACTED
	return s.SendEmailWithConfig(config, to, subject, body)
REDACTED

// SendEmailWithConfig 使用指定配置发送邮件
func (s *EmailService) SendEmailWithConfig(config *SmtpConfig, to, subject, body string) error {
	from := config.From
	if config.FromName != "" {
		from = fmt.Sprintf("%s <%s>", config.FromName, config.From)
REDACTED

	msg := fmt.Sprintf("From: %s\r\nTo: %s\r\nSubject: %s\r\nMIME-Version: 1.0\r\nContent-Type: text/html; charset=UTF-8\r\n\r\n%s",
		from, to, subject, body)

	addr := fmt.Sprintf("%s:%d", config.Host, config.Port)
	auth := smtp.PlainAuth("", config.Username, config.Password, config.Host)

	if config.UseTLS {
		return s.sendMailTLS(addr, auth, config.From, to, []byte(msg), config.Host)
REDACTED

	return smtp.SendMail(addr, auth, config.From, []string{toREDACTED, []byte(msg))
REDACTED

// sendMailTLS 使用TLS发送邮件
func (s *EmailService) sendMailTLS(addr string, auth smtp.Auth, from, to string, msg []byte, host string) error {
	tlsConfig := &tls.Config{
		ServerName: host,
REDACTED

	conn, err := tls.Dial("tcp", addr, tlsConfig)
	if err != nil {
		return fmt.Errorf("tls dial: %w", err)
REDACTED
	defer conn.Close()

	client, err := smtp.NewClient(conn, host)
	if err != nil {
		return fmt.Errorf("new smtp client: %w", err)
REDACTED
	defer client.Close()

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
func (s *EmailService) SendVerifyCode(ctx context.Context, email, siteName string) error {
	key := verifyCodeKeyPrefix + email

	// 检查是否在冷却期内
	existing, err := s.getVerifyCodeData(ctx, key)
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
	data := &verifyCodeData{
		Code:      code,
		Attempts:  0,
		CreatedAt: time.Now(),
REDACTED
	if err := s.setVerifyCodeData(ctx, key, data); err != nil {
		return fmt.Errorf("save verify code: %w", err)
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
	key := verifyCodeKeyPrefix + email

	data, err := s.getVerifyCodeData(ctx, key)
	if err != nil || data == nil {
		return ErrInvalidVerifyCode
REDACTED

	// 检查是否已达到最大尝试次数
	if data.Attempts >= maxVerifyCodeAttempts {
		return ErrVerifyCodeMaxAttempts
REDACTED

	// 验证码不匹配
	if data.Code != code {
		data.Attempts++
		_ = s.setVerifyCodeData(ctx, key, data)
		if data.Attempts >= maxVerifyCodeAttempts {
			return ErrVerifyCodeMaxAttempts
	REDACTED
		return ErrInvalidVerifyCode
REDACTED

	// 验证成功，删除验证码
	s.rdb.Del(ctx, key)
	return nil
REDACTED

// getVerifyCodeData 从 Redis 获取验证码数据
func (s *EmailService) getVerifyCodeData(ctx context.Context, key string) (*verifyCodeData, error) {
	val, err := s.rdb.Get(ctx, key).Result()
	if err != nil {
		return nil, err
REDACTED
	var data verifyCodeData
	if err := json.Unmarshal([]byte(val), &data); err != nil {
		return nil, err
REDACTED
	return &data, nil
REDACTED

// setVerifyCodeData 保存验证码数据到 Redis
func (s *EmailService) setVerifyCodeData(ctx context.Context, key string, data *verifyCodeData) error {
	val, err := json.Marshal(data)
	if err != nil {
		return err
REDACTED
	return s.rdb.Set(ctx, key, val, verifyCodeTTL).Err()
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
`, siteName, code)
REDACTED

// TestSmtpConnectionWithConfig 使用指定配置测试SMTP连接
func (s *EmailService) TestSmtpConnectionWithConfig(config *SmtpConfig) error {
	addr := fmt.Sprintf("%s:%d", config.Host, config.Port)

	if config.UseTLS {
		tlsConfig := &tls.Config{ServerName: config.HostREDACTED
		conn, err := tls.Dial("tcp", addr, tlsConfig)
		if err != nil {
			return fmt.Errorf("tls connection failed: %w", err)
	REDACTED
		defer conn.Close()

		client, err := smtp.NewClient(conn, config.Host)
		if err != nil {
			return fmt.Errorf("smtp client creation failed: %w", err)
	REDACTED
		defer client.Close()

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
	defer client.Close()

	auth := smtp.PlainAuth("", config.Username, config.Password, config.Host)
	if err = client.Auth(auth); err != nil {
		return fmt.Errorf("smtp authentication failed: %w", err)
REDACTED

	return client.Quit()
REDACTED
