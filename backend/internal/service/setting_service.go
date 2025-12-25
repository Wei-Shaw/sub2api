package service

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/model"
	"github.com/Wei-Shaw/sub2api/internal/service/ports"
	"strconv"

	"gorm.io/gorm"
)

var (
	ErrRegistrationDisabled = errors.New("registration is currently disabled")
)

// SettingService 系统设置服务
type SettingService struct {
	settingRepo ports.SettingRepository
	cfg         *config.Config
}

// NewSettingService 创建系统设置服务实例
func NewSettingService(settingRepo ports.SettingRepository, cfg *config.Config) *SettingService {
	return &SettingService{
		settingRepo: settingRepo,
		cfg:         cfg,
	}
}

// GetAllSettings 获取所有系统设置
func (s *SettingService) GetAllSettings(ctx context.Context) (*model.SystemSettings, error) {
	settings, err := s.settingRepo.GetAll(ctx)
	if err != nil {
		return nil, fmt.Errorf("get all settings: %w", err)
	}

	return s.parseSettings(settings), nil
}

// GetPublicSettings 获取公开设置（无需登录）
func (s *SettingService) GetPublicSettings(ctx context.Context) (*model.PublicSettings, error) {
	keys := []string{
		model.SettingKeyRegistrationEnabled,
		model.SettingKeyEmailVerifyEnabled,
		model.SettingKeyTurnstileEnabled,
		model.SettingKeyTurnstileSiteKey,
		model.SettingKeySiteName,
		model.SettingKeySiteLogo,
		model.SettingKeySiteSubtitle,
		model.SettingKeyApiBaseUrl,
		model.SettingKeyContactInfo,
		model.SettingKeyDocUrl,
	}

	settings, err := s.settingRepo.GetMultiple(ctx, keys)
	if err != nil {
		return nil, fmt.Errorf("get public settings: %w", err)
	}

	return &model.PublicSettings{
		RegistrationEnabled: settings[model.SettingKeyRegistrationEnabled] == "true",
		EmailVerifyEnabled:  settings[model.SettingKeyEmailVerifyEnabled] == "true",
		TurnstileEnabled:    settings[model.SettingKeyTurnstileEnabled] == "true",
		TurnstileSiteKey:    settings[model.SettingKeyTurnstileSiteKey],
		SiteName:            s.getStringOrDefault(settings, model.SettingKeySiteName, "Sub2API"),
		SiteLogo:            settings[model.SettingKeySiteLogo],
		SiteSubtitle:        s.getStringOrDefault(settings, model.SettingKeySiteSubtitle, "Subscription to API Conversion Platform"),
		ApiBaseUrl:          settings[model.SettingKeyApiBaseUrl],
		ContactInfo:         settings[model.SettingKeyContactInfo],
		DocUrl:              settings[model.SettingKeyDocUrl],
	}, nil
}

// UpdateSettings 更新系统设置
func (s *SettingService) UpdateSettings(ctx context.Context, settings *model.SystemSettings) error {
	updates := make(map[string]string)

	// 注册设置
	updates[model.SettingKeyRegistrationEnabled] = strconv.FormatBool(settings.RegistrationEnabled)
	updates[model.SettingKeyEmailVerifyEnabled] = strconv.FormatBool(settings.EmailVerifyEnabled)

	// 邮件服务设置（只有非空才更新密码）
	updates[model.SettingKeySmtpHost] = settings.SmtpHost
	updates[model.SettingKeySmtpPort] = strconv.Itoa(settings.SmtpPort)
	updates[model.SettingKeySmtpUsername] = settings.SmtpUsername
	if settings.SmtpPassword != "" {
		updates[model.SettingKeySmtpPassword] = settings.SmtpPassword
	}
	updates[model.SettingKeySmtpFrom] = settings.SmtpFrom
	updates[model.SettingKeySmtpFromName] = settings.SmtpFromName
	updates[model.SettingKeySmtpUseTLS] = strconv.FormatBool(settings.SmtpUseTLS)

	// Cloudflare Turnstile 设置（只有非空才更新密钥）
	updates[model.SettingKeyTurnstileEnabled] = strconv.FormatBool(settings.TurnstileEnabled)
	updates[model.SettingKeyTurnstileSiteKey] = settings.TurnstileSiteKey
	if settings.TurnstileSecretKey != "" {
		updates[model.SettingKeyTurnstileSecretKey] = settings.TurnstileSecretKey
	}

	// OEM设置
	updates[model.SettingKeySiteName] = settings.SiteName
	updates[model.SettingKeySiteLogo] = settings.SiteLogo
	updates[model.SettingKeySiteSubtitle] = settings.SiteSubtitle
	updates[model.SettingKeyApiBaseUrl] = settings.ApiBaseUrl
	updates[model.SettingKeyContactInfo] = settings.ContactInfo
	updates[model.SettingKeyDocUrl] = settings.DocUrl

	// 默认配置
	updates[model.SettingKeyDefaultConcurrency] = strconv.Itoa(settings.DefaultConcurrency)
	updates[model.SettingKeyDefaultBalance] = strconv.FormatFloat(settings.DefaultBalance, 'f', 8, 64)

	return s.settingRepo.SetMultiple(ctx, updates)
}

// IsRegistrationEnabled 检查是否开放注册
func (s *SettingService) IsRegistrationEnabled(ctx context.Context) bool {
	value, err := s.settingRepo.GetValue(ctx, model.SettingKeyRegistrationEnabled)
	if err != nil {
		// 默认开放注册
		return true
	}
	return value == "true"
}

// IsEmailVerifyEnabled 检查是否开启邮件验证
func (s *SettingService) IsEmailVerifyEnabled(ctx context.Context) bool {
	value, err := s.settingRepo.GetValue(ctx, model.SettingKeyEmailVerifyEnabled)
	if err != nil {
		return false
	}
	return value == "true"
}

// GetSiteName 获取网站名称
func (s *SettingService) GetSiteName(ctx context.Context) string {
	value, err := s.settingRepo.GetValue(ctx, model.SettingKeySiteName)
	if err != nil || value == "" {
		return "Sub2API"
	}
	return value
}

// GetDefaultConcurrency 获取默认并发量
func (s *SettingService) GetDefaultConcurrency(ctx context.Context) int {
	value, err := s.settingRepo.GetValue(ctx, model.SettingKeyDefaultConcurrency)
	if err != nil {
		return s.cfg.Default.UserConcurrency
	}
	if v, err := strconv.Atoi(value); err == nil && v > 0 {
		return v
	}
	return s.cfg.Default.UserConcurrency
}

// GetDefaultBalance 获取默认余额
func (s *SettingService) GetDefaultBalance(ctx context.Context) float64 {
	value, err := s.settingRepo.GetValue(ctx, model.SettingKeyDefaultBalance)
	if err != nil {
		return s.cfg.Default.UserBalance
	}
	if v, err := strconv.ParseFloat(value, 64); err == nil && v >= 0 {
		return v
	}
	return s.cfg.Default.UserBalance
}

// InitializeDefaultSettings 初始化默认设置
func (s *SettingService) InitializeDefaultSettings(ctx context.Context) error {
	// 检查是否已有设置
	_, err := s.settingRepo.GetValue(ctx, model.SettingKeyRegistrationEnabled)
	if err == nil {
		// 已有设置，不需要初始化
		return nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return fmt.Errorf("check existing settings: %w", err)
	}

	// 初始化默认设置
	defaults := map[string]string{
		model.SettingKeyRegistrationEnabled: "true",
		model.SettingKeyEmailVerifyEnabled:  "false",
		model.SettingKeySiteName:            "Sub2API",
		model.SettingKeySiteLogo:            "",
		model.SettingKeyDefaultConcurrency:  strconv.Itoa(s.cfg.Default.UserConcurrency),
		model.SettingKeyDefaultBalance:      strconv.FormatFloat(s.cfg.Default.UserBalance, 'f', 8, 64),
		model.SettingKeySmtpPort:            "587",
		model.SettingKeySmtpUseTLS:          "false",
	}

	return s.settingRepo.SetMultiple(ctx, defaults)
}

// parseSettings 解析设置到结构体
func (s *SettingService) parseSettings(settings map[string]string) *model.SystemSettings {
	result := &model.SystemSettings{
		RegistrationEnabled: settings[model.SettingKeyRegistrationEnabled] == "true",
		EmailVerifyEnabled:  settings[model.SettingKeyEmailVerifyEnabled] == "true",
		SmtpHost:            settings[model.SettingKeySmtpHost],
		SmtpUsername:        settings[model.SettingKeySmtpUsername],
		// 仅返回“是否已配置”，不返回明文密码
		SmtpPasswordSet:     settings[model.SettingKeySmtpPassword] != "",
		SmtpFrom:            settings[model.SettingKeySmtpFrom],
		SmtpFromName:        settings[model.SettingKeySmtpFromName],
		SmtpUseTLS:          settings[model.SettingKeySmtpUseTLS] == "true",
		TurnstileEnabled:    settings[model.SettingKeyTurnstileEnabled] == "true",
		TurnstileSiteKey:    settings[model.SettingKeyTurnstileSiteKey],
		// 仅返回“是否已配置”，不返回明文密钥
		TurnstileSecretKeySet: settings[model.SettingKeyTurnstileSecretKey] != "",
		SiteName:            s.getStringOrDefault(settings, model.SettingKeySiteName, "Sub2API"),
		SiteLogo:            settings[model.SettingKeySiteLogo],
		SiteSubtitle:        s.getStringOrDefault(settings, model.SettingKeySiteSubtitle, "Subscription to API Conversion Platform"),
		ApiBaseUrl:          settings[model.SettingKeyApiBaseUrl],
		ContactInfo:         settings[model.SettingKeyContactInfo],
		DocUrl:              settings[model.SettingKeyDocUrl],
	}

	// 解析整数类型
	if port, err := strconv.Atoi(settings[model.SettingKeySmtpPort]); err == nil {
		result.SmtpPort = port
	} else {
		result.SmtpPort = 587
	}

	if concurrency, err := strconv.Atoi(settings[model.SettingKeyDefaultConcurrency]); err == nil {
		result.DefaultConcurrency = concurrency
	} else {
		result.DefaultConcurrency = s.cfg.Default.UserConcurrency
	}

	// 解析浮点数类型
	if balance, err := strconv.ParseFloat(settings[model.SettingKeyDefaultBalance], 64); err == nil {
		result.DefaultBalance = balance
	} else {
		result.DefaultBalance = s.cfg.Default.UserBalance
	}

	return result
}

// getStringOrDefault 获取字符串值或默认值
func (s *SettingService) getStringOrDefault(settings map[string]string, key, defaultValue string) string {
	if value, ok := settings[key]; ok && value != "" {
		return value
	}
	return defaultValue
}

// IsTurnstileEnabled 检查是否启用 Turnstile 验证
func (s *SettingService) IsTurnstileEnabled(ctx context.Context) bool {
	value, err := s.settingRepo.GetValue(ctx, model.SettingKeyTurnstileEnabled)
	if err != nil {
		return false
	}
	return value == "true"
}

// GetTurnstileSecretKey 获取 Turnstile Secret Key
func (s *SettingService) GetTurnstileSecretKey(ctx context.Context) string {
	value, err := s.settingRepo.GetValue(ctx, model.SettingKeyTurnstileSecretKey)
	if err != nil {
		return ""
	}
	return value
}

// GenerateAdminApiKey 生成新的管理员 API Key
func (s *SettingService) GenerateAdminApiKey(ctx context.Context) (string, error) {
	// 生成 32 字节随机数 = 64 位十六进制字符
	bytes := make([]byte, 32)
	if _, err := rand.Read(bytes); err != nil {
		return "", fmt.Errorf("generate random bytes: %w", err)
	}

	key := model.AdminApiKeyPrefix + hex.EncodeToString(bytes)

	// 存储到 settings 表
	if err := s.settingRepo.Set(ctx, model.SettingKeyAdminApiKey, key); err != nil {
		return "", fmt.Errorf("save admin api key: %w", err)
	}

	return key, nil
}

// GetAdminApiKeyStatus 获取管理员 API Key 状态
// 返回脱敏的 key、是否存在、错误
func (s *SettingService) GetAdminApiKeyStatus(ctx context.Context) (maskedKey string, exists bool, err error) {
	key, err := s.settingRepo.GetValue(ctx, model.SettingKeyAdminApiKey)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return "", false, nil
		}
		return "", false, err
	}
	if key == "" {
		return "", false, nil
	}

	// 脱敏：显示前 10 位和后 4 位
	if len(key) > 14 {
		maskedKey = key[:10] + "..." + key[len(key)-4:]
	} else {
		maskedKey = key
	}

	return maskedKey, true, nil
}

// GetAdminApiKey 获取完整的管理员 API Key（仅供内部验证使用）
// 如果未配置返回空字符串和 nil 错误，只有数据库错误时才返回 error
func (s *SettingService) GetAdminApiKey(ctx context.Context) (string, error) {
	key, err := s.settingRepo.GetValue(ctx, model.SettingKeyAdminApiKey)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return "", nil // 未配置，返回空字符串
		}
		return "", err // 数据库错误
	}
	return key, nil
}

// DeleteAdminApiKey 删除管理员 API Key
func (s *SettingService) DeleteAdminApiKey(ctx context.Context) error {
	return s.settingRepo.Delete(ctx, model.SettingKeyAdminApiKey)
}
