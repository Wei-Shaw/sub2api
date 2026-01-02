package service

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"strconv"

	"github.com/Wei-Shaw/sub2api/internal/config"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/infrastructure/errors"
)

var (
	ErrRegistrationDisabled = infraerrors.Forbidden("REGISTRATION_DISABLED", "registration is currently disabled")
	ErrSettingNotFound      = infraerrors.NotFound("SETTING_NOT_FOUND", "setting not found")
)

type SettingRepository interface {
	Get(ctx context.Context, key string) (*Setting, error)
	GetValue(ctx context.Context, key string) (string, error)
	Set(ctx context.Context, key, value string) error
	GetMultiple(ctx context.Context, keys []string) (map[string]string, error)
	SetMultiple(ctx context.Context, settings map[string]string) error
	GetAll(ctx context.Context) (map[string]string, error)
	Delete(ctx context.Context, key string) error
REDACTED

// SettingService 系统设置服务
type SettingService struct {
	settingRepo SettingRepository
	cfg         *config.Config
REDACTED

// NewSettingService 创建系统设置服务实例
func NewSettingService(settingRepo SettingRepository, cfg *config.Config) *SettingService {
	return &SettingService{
		settingRepo: settingRepo,
		cfg:         cfg,
REDACTED
REDACTED

// GetAllSettings 获取所有系统设置
func (s *SettingService) GetAllSettings(ctx context.Context) (*SystemSettings, error) {
	settings, err := s.settingRepo.GetAll(ctx)
	if err != nil {
		return nil, fmt.Errorf("get all settings: %w", err)
REDACTED

	return s.parseSettings(settings), nil
REDACTED

// GetPublicSettings 获取公开设置（无需登录）
func (s *SettingService) GetPublicSettings(ctx context.Context) (*PublicSettings, error) {
	keys := []string{
		SettingKeyRegistrationEnabled,
		SettingKeyEmailVerifyEnabled,
		SettingKeyTurnstileEnabled,
		SettingKeyTurnstileSiteKey,
		SettingKeySiteName,
		SettingKeySiteLogo,
		SettingKeySiteSubtitle,
		SettingKeyApiBaseUrl,
		SettingKeyContactInfo,
		SettingKeyDocUrl,
REDACTED

	settings, err := s.settingRepo.GetMultiple(ctx, keys)
	if err != nil {
		return nil, fmt.Errorf("get public settings: %w", err)
REDACTED

	return &PublicSettings{
		RegistrationEnabled: settings[SettingKeyRegistrationEnabled] == "true",
		EmailVerifyEnabled:  settings[SettingKeyEmailVerifyEnabled] == "true",
		TurnstileEnabled:    settings[SettingKeyTurnstileEnabled] == "true",
		TurnstileSiteKey:    settings[SettingKeyTurnstileSiteKey],
		SiteName:            s.getStringOrDefault(settings, SettingKeySiteName, "Sub2API"),
		SiteLogo:            settings[SettingKeySiteLogo],
		SiteSubtitle:        s.getStringOrDefault(settings, SettingKeySiteSubtitle, "Subscription to API Conversion Platform"),
		ApiBaseUrl:          settings[SettingKeyApiBaseUrl],
		ContactInfo:         settings[SettingKeyContactInfo],
		DocUrl:              settings[SettingKeyDocUrl],
REDACTED, nil
REDACTED

// UpdateSettings 更新系统设置
func (s *SettingService) UpdateSettings(ctx context.Context, settings *SystemSettings) error {
	updates := make(map[string]string)

	// 注册设置
	updates[SettingKeyRegistrationEnabled] = strconv.FormatBool(settings.RegistrationEnabled)
	updates[SettingKeyEmailVerifyEnabled] = strconv.FormatBool(settings.EmailVerifyEnabled)

	// 邮件服务设置（只有非空才更新密码）
	updates[SettingKeySmtpHost] = settings.SmtpHost
	updates[SettingKeySmtpPort] = strconv.Itoa(settings.SmtpPort)
	updates[SettingKeySmtpUsername] = settings.SmtpUsername
	if settings.SmtpPassword != "" {
		updates[SettingKeySmtpPassword] = settings.SmtpPassword
REDACTED
	updates[SettingKeySmtpFrom] = settings.SmtpFrom
	updates[SettingKeySmtpFromName] = settings.SmtpFromName
	updates[SettingKeySmtpUseTLS] = strconv.FormatBool(settings.SmtpUseTLS)

	// Cloudflare Turnstile 设置（只有非空才更新密钥）
	updates[SettingKeyTurnstileEnabled] = strconv.FormatBool(settings.TurnstileEnabled)
	updates[SettingKeyTurnstileSiteKey] = settings.TurnstileSiteKey
	if settings.TurnstileSecretKey != "" {
		updates[SettingKeyTurnstileSecretKey] = settings.TurnstileSecretKey
REDACTED

	// OEM设置
	updates[SettingKeySiteName] = settings.SiteName
	updates[SettingKeySiteLogo] = settings.SiteLogo
	updates[SettingKeySiteSubtitle] = settings.SiteSubtitle
	updates[SettingKeyApiBaseUrl] = settings.ApiBaseUrl
	updates[SettingKeyContactInfo] = settings.ContactInfo
	updates[SettingKeyDocUrl] = settings.DocUrl

	// 默认配置
	updates[SettingKeyDefaultConcurrency] = strconv.Itoa(settings.DefaultConcurrency)
	updates[SettingKeyDefaultBalance] = strconv.FormatFloat(settings.DefaultBalance, 'f', 8, 64)

	return s.settingRepo.SetMultiple(ctx, updates)
REDACTED

// IsRegistrationEnabled 检查是否开放注册
func (s *SettingService) IsRegistrationEnabled(ctx context.Context) bool {
	value, err := s.settingRepo.GetValue(ctx, SettingKeyRegistrationEnabled)
	if err != nil {
		// 默认开放注册
		return true
REDACTED
	return value == "true"
REDACTED

// IsEmailVerifyEnabled 检查是否开启邮件验证
func (s *SettingService) IsEmailVerifyEnabled(ctx context.Context) bool {
	value, err := s.settingRepo.GetValue(ctx, SettingKeyEmailVerifyEnabled)
	if err != nil {
		return false
REDACTED
	return value == "true"
REDACTED

// GetSiteName 获取网站名称
func (s *SettingService) GetSiteName(ctx context.Context) string {
	value, err := s.settingRepo.GetValue(ctx, SettingKeySiteName)
	if err != nil || value == "" {
		return "Sub2API"
REDACTED
	return value
REDACTED

// GetDefaultConcurrency 获取默认并发量
func (s *SettingService) GetDefaultConcurrency(ctx context.Context) int {
	value, err := s.settingRepo.GetValue(ctx, SettingKeyDefaultConcurrency)
	if err != nil {
		return s.cfg.Default.UserConcurrency
REDACTED
	if v, err := strconv.Atoi(value); err == nil && v > 0 {
		return v
REDACTED
	return s.cfg.Default.UserConcurrency
REDACTED

// GetDefaultBalance 获取默认余额
func (s *SettingService) GetDefaultBalance(ctx context.Context) float64 {
	value, err := s.settingRepo.GetValue(ctx, SettingKeyDefaultBalance)
	if err != nil {
		return s.cfg.Default.UserBalance
REDACTED
	if v, err := strconv.ParseFloat(value, 64); err == nil && v >= 0 {
		return v
REDACTED
	return s.cfg.Default.UserBalance
REDACTED

// InitializeDefaultSettings 初始化默认设置
func (s *SettingService) InitializeDefaultSettings(ctx context.Context) error {
	// 检查是否已有设置
	_, err := s.settingRepo.GetValue(ctx, SettingKeyRegistrationEnabled)
	if err == nil {
		// 已有设置，不需要初始化
		return nil
REDACTED
	if !errors.Is(err, ErrSettingNotFound) {
		return fmt.Errorf("check existing settings: %w", err)
REDACTED

	// 初始化默认设置
	defaults := map[string]string{
		SettingKeyRegistrationEnabled: "true",
		SettingKeyEmailVerifyEnabled:  "false",
		SettingKeySiteName:            "Sub2API",
		SettingKeySiteLogo:            "",
		SettingKeyDefaultConcurrency:  strconv.Itoa(s.cfg.Default.UserConcurrency),
		SettingKeyDefaultBalance:      strconv.FormatFloat(s.cfg.Default.UserBalance, 'f', 8, 64),
		SettingKeySmtpPort:            "587",
		SettingKeySmtpUseTLS:          "false",
REDACTED

	return s.settingRepo.SetMultiple(ctx, defaults)
REDACTED

// parseSettings 解析设置到结构体
func (s *SettingService) parseSettings(settings map[string]string) *SystemSettings {
	result := &SystemSettings{
		RegistrationEnabled: settings[SettingKeyRegistrationEnabled] == "true",
		EmailVerifyEnabled:  settings[SettingKeyEmailVerifyEnabled] == "true",
		SmtpHost:            settings[SettingKeySmtpHost],
		SmtpUsername:        settings[SettingKeySmtpUsername],
		SmtpFrom:            settings[SettingKeySmtpFrom],
		SmtpFromName:        settings[SettingKeySmtpFromName],
		SmtpUseTLS:          settings[SettingKeySmtpUseTLS] == "true",
		SmtpPasswordConfigured: settings[SettingKeySmtpPassword] != "",
		TurnstileEnabled:    settings[SettingKeyTurnstileEnabled] == "true",
		TurnstileSiteKey:    settings[SettingKeyTurnstileSiteKey],
		TurnstileSecretKeyConfigured: settings[SettingKeyTurnstileSecretKey] != "",
		SiteName:            s.getStringOrDefault(settings, SettingKeySiteName, "Sub2API"),
		SiteLogo:            settings[SettingKeySiteLogo],
		SiteSubtitle:        s.getStringOrDefault(settings, SettingKeySiteSubtitle, "Subscription to API Conversion Platform"),
		ApiBaseUrl:          settings[SettingKeyApiBaseUrl],
		ContactInfo:         settings[SettingKeyContactInfo],
		DocUrl:              settings[SettingKeyDocUrl],
REDACTED

	// 解析整数类型
	if port, err := strconv.Atoi(settings[SettingKeySmtpPort]); err == nil {
		result.SmtpPort = port
REDACTED else {
		result.SmtpPort = 587
REDACTED

	if concurrency, err := strconv.Atoi(settings[SettingKeyDefaultConcurrency]); err == nil {
		result.DefaultConcurrency = concurrency
REDACTED else {
		result.DefaultConcurrency = s.cfg.Default.UserConcurrency
REDACTED

	// 解析浮点数类型
	if balance, err := strconv.ParseFloat(settings[SettingKeyDefaultBalance], 64); err == nil {
		result.DefaultBalance = balance
REDACTED else {
		result.DefaultBalance = s.cfg.Default.UserBalance
REDACTED

	return result
REDACTED

// getStringOrDefault 获取字符串值或默认值
func (s *SettingService) getStringOrDefault(settings map[string]string, key, defaultValue string) string {
	if value, ok := settings[key]; ok && value != "" {
		return value
REDACTED
	return defaultValue
REDACTED

// IsTurnstileEnabled 检查是否启用 Turnstile 验证
func (s *SettingService) IsTurnstileEnabled(ctx context.Context) bool {
	value, err := s.settingRepo.GetValue(ctx, SettingKeyTurnstileEnabled)
	if err != nil {
		return false
REDACTED
	return value == "true"
REDACTED

// GetTurnstileSecretKey 获取 Turnstile Secret Key
func (s *SettingService) GetTurnstileSecretKey(ctx context.Context) string {
	value, err := s.settingRepo.GetValue(ctx, SettingKeyTurnstileSecretKey)
	if err != nil {
		return ""
REDACTED
	return value
REDACTED

// GenerateAdminApiKey 生成新的管理员 API Key
func (s *SettingService) GenerateAdminApiKey(ctx context.Context) (string, error) {
	// 生成 32 字节随机数 = 64 位十六进制字符
	bytes := make([]byte, 32)
	if _, err := rand.Read(bytes); err != nil {
		return "", fmt.Errorf("generate random bytes: %w", err)
REDACTED

	key := AdminApiKeyPrefix + hex.EncodeToString(bytes)

	// 存储到 settings 表
	if err := s.settingRepo.Set(ctx, SettingKeyAdminApiKey, key); err != nil {
		return "", fmt.Errorf("save admin api key: %w", err)
REDACTED

	return key, nil
REDACTED

// GetAdminApiKeyStatus 获取管理员 API Key 状态
// 返回脱敏的 key、是否存在、错误
func (s *SettingService) GetAdminApiKeyStatus(ctx context.Context) (maskedKey string, exists bool, err error) {
	key, err := s.settingRepo.GetValue(ctx, SettingKeyAdminApiKey)
	if err != nil {
		if errors.Is(err, ErrSettingNotFound) {
			return "", false, nil
	REDACTED
		return "", false, err
REDACTED
	if key == "" {
		return "", false, nil
REDACTED

	// 脱敏：显示前 10 位和后 4 位
	if len(key) > 14 {
		maskedKey = key[:10] + "..." + key[len(key)-4:]
REDACTED else {
		maskedKey = key
REDACTED

	return maskedKey, true, nil
REDACTED

// GetAdminApiKey 获取完整的管理员 API Key（仅供内部验证使用）
// 如果未配置返回空字符串和 nil 错误，只有数据库错误时才返回 error
func (s *SettingService) GetAdminApiKey(ctx context.Context) (string, error) {
	key, err := s.settingRepo.GetValue(ctx, SettingKeyAdminApiKey)
	if err != nil {
		if errors.Is(err, ErrSettingNotFound) {
			return "", nil // 未配置，返回空字符串
	REDACTED
		return "", err // 数据库错误
REDACTED
	return key, nil
REDACTED

// DeleteAdminApiKey 删除管理员 API Key
func (s *SettingService) DeleteAdminApiKey(ctx context.Context) error {
	return s.settingRepo.Delete(ctx, SettingKeyAdminApiKey)
REDACTED
