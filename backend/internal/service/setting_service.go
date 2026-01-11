package service

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/config"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
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
	onUpdate    func() // Callback when settings are updated (for cache invalidation)
	version     string // Application version
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
		SettingKeyAPIBaseURL,
		SettingKeyContactInfo,
		SettingKeyDocURL,
		SettingKeyHomeContent,
		SettingKeyLinuxDoConnectEnabled,
REDACTED

	settings, err := s.settingRepo.GetMultiple(ctx, keys)
	if err != nil {
		return nil, fmt.Errorf("get public settings: %w", err)
REDACTED

	linuxDoEnabled := false
	if raw, ok := settings[SettingKeyLinuxDoConnectEnabled]; ok {
		linuxDoEnabled = raw == "true"
REDACTED else {
		linuxDoEnabled = s.cfg != nil && s.cfg.LinuxDo.Enabled
REDACTED

	return &PublicSettings{
		RegistrationEnabled: settings[SettingKeyRegistrationEnabled] == "true",
		EmailVerifyEnabled:  settings[SettingKeyEmailVerifyEnabled] == "true",
		TurnstileEnabled:    settings[SettingKeyTurnstileEnabled] == "true",
		TurnstileSiteKey:    settings[SettingKeyTurnstileSiteKey],
		SiteName:            s.getStringOrDefault(settings, SettingKeySiteName, "Sub2API"),
		SiteLogo:            settings[SettingKeySiteLogo],
		SiteSubtitle:        s.getStringOrDefault(settings, SettingKeySiteSubtitle, "Subscription to API Conversion Platform"),
		APIBaseURL:          settings[SettingKeyAPIBaseURL],
		ContactInfo:         settings[SettingKeyContactInfo],
		DocURL:              settings[SettingKeyDocURL],
		HomeContent:         settings[SettingKeyHomeContent],
		LinuxDoOAuthEnabled: linuxDoEnabled,
REDACTED, nil
REDACTED

// SetOnUpdateCallback sets a callback function to be called when settings are updated
// This is used for cache invalidation (e.g., HTML cache in frontend server)
func (s *SettingService) SetOnUpdateCallback(callback func()) {
	s.onUpdate = callback
REDACTED

// SetVersion sets the application version for injection into public settings
func (s *SettingService) SetVersion(version string) {
	s.version = version
REDACTED

// GetPublicSettingsForInjection returns public settings in a format suitable for HTML injection
// This implements the web.PublicSettingsProvider interface
func (s *SettingService) GetPublicSettingsForInjection(ctx context.Context) (any, error) {
	settings, err := s.GetPublicSettings(ctx)
	if err != nil {
		return nil, err
REDACTED

	// Return a struct that matches the frontend's expected format
	return &struct {
		RegistrationEnabled bool   `json:"registration_enabled"`
		EmailVerifyEnabled  bool   `json:"email_verify_enabled"`
		TurnstileEnabled    bool   `json:"turnstile_enabled"`
		TurnstileSiteKey    string `json:"turnstile_site_key,omitempty"`
		SiteName            string `json:"site_name"`
		SiteLogo            string `json:"site_logo,omitempty"`
		SiteSubtitle        string `json:"site_subtitle,omitempty"`
		APIBaseURL          string `json:"api_base_url,omitempty"`
		ContactInfo         string `json:"contact_info,omitempty"`
		DocURL              string `json:"doc_url,omitempty"`
		HomeContent         string `json:"home_content,omitempty"`
		LinuxDoOAuthEnabled bool   `json:"linuxdo_oauth_enabled"`
		Version             string `json:"version,omitempty"`
REDACTED{
		RegistrationEnabled: settings.RegistrationEnabled,
		EmailVerifyEnabled:  settings.EmailVerifyEnabled,
		TurnstileEnabled:    settings.TurnstileEnabled,
		TurnstileSiteKey:    settings.TurnstileSiteKey,
		SiteName:            settings.SiteName,
		SiteLogo:            settings.SiteLogo,
		SiteSubtitle:        settings.SiteSubtitle,
		APIBaseURL:          settings.APIBaseURL,
		ContactInfo:         settings.ContactInfo,
		DocURL:              settings.DocURL,
		HomeContent:         settings.HomeContent,
		LinuxDoOAuthEnabled: settings.LinuxDoOAuthEnabled,
		Version:             s.version,
REDACTED, nil
REDACTED

// UpdateSettings 更新系统设置
func (s *SettingService) UpdateSettings(ctx context.Context, settings *SystemSettings) error {
	updates := make(map[string]string)

	// 注册设置
	updates[SettingKeyRegistrationEnabled] = strconv.FormatBool(settings.RegistrationEnabled)
	updates[SettingKeyEmailVerifyEnabled] = strconv.FormatBool(settings.EmailVerifyEnabled)

	// 邮件服务设置（只有非空才更新密码）
	updates[SettingKeySMTPHost] = settings.SMTPHost
	updates[SettingKeySMTPPort] = strconv.Itoa(settings.SMTPPort)
	updates[SettingKeySMTPUsername] = settings.SMTPUsername
	if settings.SMTPPassword != "" {
		updates[SettingKeySMTPPassword] = settings.SMTPPassword
REDACTED
	updates[SettingKeySMTPFrom] = settings.SMTPFrom
	updates[SettingKeySMTPFromName] = settings.SMTPFromName
	updates[SettingKeySMTPUseTLS] = strconv.FormatBool(settings.SMTPUseTLS)

	// Cloudflare Turnstile 设置（只有非空才更新密钥）
	updates[SettingKeyTurnstileEnabled] = strconv.FormatBool(settings.TurnstileEnabled)
	updates[SettingKeyTurnstileSiteKey] = settings.TurnstileSiteKey
	if settings.TurnstileSecretKey != "" {
		updates[SettingKeyTurnstileSecretKey] = settings.TurnstileSecretKey
REDACTED

	// LinuxDo Connect OAuth 登录（终端用户 SSO）
	updates[SettingKeyLinuxDoConnectEnabled] = strconv.FormatBool(settings.LinuxDoConnectEnabled)
	updates[SettingKeyLinuxDoConnectClientID] = settings.LinuxDoConnectClientID
	updates[SettingKeyLinuxDoConnectRedirectURL] = settings.LinuxDoConnectRedirectURL
	if settings.LinuxDoConnectClientSecret != "" {
		updates[SettingKeyLinuxDoConnectClientSecret] = settings.LinuxDoConnectClientSecret
REDACTED

	// OEM设置
	updates[SettingKeySiteName] = settings.SiteName
	updates[SettingKeySiteLogo] = settings.SiteLogo
	updates[SettingKeySiteSubtitle] = settings.SiteSubtitle
	updates[SettingKeyAPIBaseURL] = settings.APIBaseURL
	updates[SettingKeyContactInfo] = settings.ContactInfo
	updates[SettingKeyDocURL] = settings.DocURL
	updates[SettingKeyHomeContent] = settings.HomeContent

	// 默认配置
	updates[SettingKeyDefaultConcurrency] = strconv.Itoa(settings.DefaultConcurrency)
	updates[SettingKeyDefaultBalance] = strconv.FormatFloat(settings.DefaultBalance, 'f', 8, 64)

	// Model fallback configuration
	updates[SettingKeyEnableModelFallback] = strconv.FormatBool(settings.EnableModelFallback)
	updates[SettingKeyFallbackModelAnthropic] = settings.FallbackModelAnthropic
	updates[SettingKeyFallbackModelOpenAI] = settings.FallbackModelOpenAI
	updates[SettingKeyFallbackModelGemini] = settings.FallbackModelGemini
	updates[SettingKeyFallbackModelAntigravity] = settings.FallbackModelAntigravity

	// Identity patch configuration (Claude -> Gemini)
	updates[SettingKeyEnableIdentityPatch] = strconv.FormatBool(settings.EnableIdentityPatch)
	updates[SettingKeyIdentityPatchPrompt] = settings.IdentityPatchPrompt

	err := s.settingRepo.SetMultiple(ctx, updates)
	if err == nil && s.onUpdate != nil {
		s.onUpdate() // Invalidate cache after settings update
REDACTED
	return err
REDACTED

// IsRegistrationEnabled 检查是否开放注册
func (s *SettingService) IsRegistrationEnabled(ctx context.Context) bool {
	value, err := s.settingRepo.GetValue(ctx, SettingKeyRegistrationEnabled)
	if err != nil {
		// 安全默认：如果设置不存在或查询出错，默认关闭注册
		return false
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
		SettingKeySMTPPort:            "587",
		SettingKeySMTPUseTLS:          "false",
		// Model fallback defaults
		SettingKeyEnableModelFallback:      "false",
		SettingKeyFallbackModelAnthropic:   "claude-3-5-sonnet-20241022",
		SettingKeyFallbackModelOpenAI:      "gpt-4o",
		SettingKeyFallbackModelGemini:      "gemini-2.5-pro",
		SettingKeyFallbackModelAntigravity: "gemini-2.5-pro",
		// Identity patch defaults
		SettingKeyEnableIdentityPatch: "true",
		SettingKeyIdentityPatchPrompt: "",
REDACTED

	return s.settingRepo.SetMultiple(ctx, defaults)
REDACTED

// parseSettings 解析设置到结构体
func (s *SettingService) parseSettings(settings map[string]string) *SystemSettings {
	result := &SystemSettings{
		RegistrationEnabled:          settings[SettingKeyRegistrationEnabled] == "true",
		EmailVerifyEnabled:           settings[SettingKeyEmailVerifyEnabled] == "true",
		SMTPHost:                     settings[SettingKeySMTPHost],
		SMTPUsername:                 settings[SettingKeySMTPUsername],
		SMTPFrom:                     settings[SettingKeySMTPFrom],
		SMTPFromName:                 settings[SettingKeySMTPFromName],
		SMTPUseTLS:                   settings[SettingKeySMTPUseTLS] == "true",
		SMTPPasswordConfigured:       settings[SettingKeySMTPPassword] != "",
		TurnstileEnabled:             settings[SettingKeyTurnstileEnabled] == "true",
		TurnstileSiteKey:             settings[SettingKeyTurnstileSiteKey],
		TurnstileSecretKeyConfigured: settings[SettingKeyTurnstileSecretKey] != "",
		SiteName:                     s.getStringOrDefault(settings, SettingKeySiteName, "Sub2API"),
		SiteLogo:                     settings[SettingKeySiteLogo],
		SiteSubtitle:                 s.getStringOrDefault(settings, SettingKeySiteSubtitle, "Subscription to API Conversion Platform"),
		APIBaseURL:                   settings[SettingKeyAPIBaseURL],
		ContactInfo:                  settings[SettingKeyContactInfo],
		DocURL:                       settings[SettingKeyDocURL],
		HomeContent:                  settings[SettingKeyHomeContent],
REDACTED

	// 解析整数类型
	if port, err := strconv.Atoi(settings[SettingKeySMTPPort]); err == nil {
		result.SMTPPort = port
REDACTED else {
		result.SMTPPort = 587
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

	// 敏感信息直接返回，方便测试连接时使用
	result.SMTPPassword = settings[SettingKeySMTPPassword]
	result.TurnstileSecretKey = settings[SettingKeyTurnstileSecretKey]

	// LinuxDo Connect 设置：
	// - 兼容 config.yaml/env（避免老部署因为未迁移到数据库设置而被意外关闭）
	// - 支持在后台“系统设置”中覆盖并持久化（存储于 DB）
	linuxDoBase := config.LinuxDoConnectConfig{REDACTED
	if s.cfg != nil {
		linuxDoBase = s.cfg.LinuxDo
REDACTED

	if raw, ok := settings[SettingKeyLinuxDoConnectEnabled]; ok {
		result.LinuxDoConnectEnabled = raw == "true"
REDACTED else {
		result.LinuxDoConnectEnabled = linuxDoBase.Enabled
REDACTED

	if v, ok := settings[SettingKeyLinuxDoConnectClientID]; ok && strings.TrimSpace(v) != "" {
		result.LinuxDoConnectClientID = strings.TrimSpace(v)
REDACTED else {
		result.LinuxDoConnectClientID = linuxDoBase.ClientID
REDACTED

	if v, ok := settings[SettingKeyLinuxDoConnectRedirectURL]; ok && strings.TrimSpace(v) != "" {
		result.LinuxDoConnectRedirectURL = strings.TrimSpace(v)
REDACTED else {
		result.LinuxDoConnectRedirectURL = linuxDoBase.RedirectURL
REDACTED

	result.LinuxDoConnectClientSecret = strings.TrimSpace(settings[SettingKeyLinuxDoConnectClientSecret])
	if result.LinuxDoConnectClientSecret == "" {
		result.LinuxDoConnectClientSecret = strings.TrimSpace(linuxDoBase.ClientSecret)
REDACTED
	result.LinuxDoConnectClientSecretConfigured = result.LinuxDoConnectClientSecret != ""

	// Model fallback settings
	result.EnableModelFallback = settings[SettingKeyEnableModelFallback] == "true"
	result.FallbackModelAnthropic = s.getStringOrDefault(settings, SettingKeyFallbackModelAnthropic, "claude-3-5-sonnet-20241022")
	result.FallbackModelOpenAI = s.getStringOrDefault(settings, SettingKeyFallbackModelOpenAI, "gpt-4o")
	result.FallbackModelGemini = s.getStringOrDefault(settings, SettingKeyFallbackModelGemini, "gemini-2.5-pro")
	result.FallbackModelAntigravity = s.getStringOrDefault(settings, SettingKeyFallbackModelAntigravity, "gemini-2.5-pro")

	// Identity patch settings (default: enabled, to preserve existing behavior)
	if v, ok := settings[SettingKeyEnableIdentityPatch]; ok && v != "" {
		result.EnableIdentityPatch = v == "true"
REDACTED else {
		result.EnableIdentityPatch = true
REDACTED
	result.IdentityPatchPrompt = settings[SettingKeyIdentityPatchPrompt]

	return result
REDACTED

// GetLinuxDoConnectOAuthConfig 返回用于登录的“最终生效” LinuxDo Connect 配置。
//
// 优先级：
// - 若对应系统设置键存在，则覆盖 config.yaml/env 的值
// - 否则回退到 config.yaml/env 的值
func (s *SettingService) GetLinuxDoConnectOAuthConfig(ctx context.Context) (config.LinuxDoConnectConfig, error) {
	if s == nil || s.cfg == nil {
		return config.LinuxDoConnectConfig{REDACTED, infraerrors.ServiceUnavailable("CONFIG_NOT_READY", "config not loaded")
REDACTED

	effective := s.cfg.LinuxDo

	keys := []string{
		SettingKeyLinuxDoConnectEnabled,
		SettingKeyLinuxDoConnectClientID,
		SettingKeyLinuxDoConnectClientSecret,
		SettingKeyLinuxDoConnectRedirectURL,
REDACTED
	settings, err := s.settingRepo.GetMultiple(ctx, keys)
	if err != nil {
		return config.LinuxDoConnectConfig{REDACTED, fmt.Errorf("get linuxdo connect settings: %w", err)
REDACTED

	if raw, ok := settings[SettingKeyLinuxDoConnectEnabled]; ok {
		effective.Enabled = raw == "true"
REDACTED
	if v, ok := settings[SettingKeyLinuxDoConnectClientID]; ok && strings.TrimSpace(v) != "" {
		effective.ClientID = strings.TrimSpace(v)
REDACTED
	if v, ok := settings[SettingKeyLinuxDoConnectClientSecret]; ok && strings.TrimSpace(v) != "" {
		effective.ClientSecret = strings.TrimSpace(v)
REDACTED
	if v, ok := settings[SettingKeyLinuxDoConnectRedirectURL]; ok && strings.TrimSpace(v) != "" {
		effective.RedirectURL = strings.TrimSpace(v)
REDACTED

	if !effective.Enabled {
		return config.LinuxDoConnectConfig{REDACTED, infraerrors.NotFound("OAUTH_DISABLED", "oauth login is disabled")
REDACTED

	// 基础健壮性校验（避免把用户重定向到一个必然失败或不安全的 OAuth 流程里）。
	if strings.TrimSpace(effective.ClientID) == "" {
		return config.LinuxDoConnectConfig{REDACTED, infraerrors.InternalServer("OAUTH_CONFIG_INVALID", "oauth client id not configured")
REDACTED
	if strings.TrimSpace(effective.AuthorizeURL) == "" {
		return config.LinuxDoConnectConfig{REDACTED, infraerrors.InternalServer("OAUTH_CONFIG_INVALID", "oauth authorize url not configured")
REDACTED
	if strings.TrimSpace(effective.TokenURL) == "" {
		return config.LinuxDoConnectConfig{REDACTED, infraerrors.InternalServer("OAUTH_CONFIG_INVALID", "oauth token url not configured")
REDACTED
	if strings.TrimSpace(effective.UserInfoURL) == "" {
		return config.LinuxDoConnectConfig{REDACTED, infraerrors.InternalServer("OAUTH_CONFIG_INVALID", "oauth userinfo url not configured")
REDACTED
	if strings.TrimSpace(effective.RedirectURL) == "" {
		return config.LinuxDoConnectConfig{REDACTED, infraerrors.InternalServer("OAUTH_CONFIG_INVALID", "oauth redirect url not configured")
REDACTED
	if strings.TrimSpace(effective.FrontendRedirectURL) == "" {
		return config.LinuxDoConnectConfig{REDACTED, infraerrors.InternalServer("OAUTH_CONFIG_INVALID", "oauth frontend redirect url not configured")
REDACTED

	if err := config.ValidateAbsoluteHTTPURL(effective.AuthorizeURL); err != nil {
		return config.LinuxDoConnectConfig{REDACTED, infraerrors.InternalServer("OAUTH_CONFIG_INVALID", "oauth authorize url invalid")
REDACTED
	if err := config.ValidateAbsoluteHTTPURL(effective.TokenURL); err != nil {
		return config.LinuxDoConnectConfig{REDACTED, infraerrors.InternalServer("OAUTH_CONFIG_INVALID", "oauth token url invalid")
REDACTED
	if err := config.ValidateAbsoluteHTTPURL(effective.UserInfoURL); err != nil {
		return config.LinuxDoConnectConfig{REDACTED, infraerrors.InternalServer("OAUTH_CONFIG_INVALID", "oauth userinfo url invalid")
REDACTED
	if err := config.ValidateAbsoluteHTTPURL(effective.RedirectURL); err != nil {
		return config.LinuxDoConnectConfig{REDACTED, infraerrors.InternalServer("OAUTH_CONFIG_INVALID", "oauth redirect url invalid")
REDACTED
	if err := config.ValidateFrontendRedirectURL(effective.FrontendRedirectURL); err != nil {
		return config.LinuxDoConnectConfig{REDACTED, infraerrors.InternalServer("OAUTH_CONFIG_INVALID", "oauth frontend redirect url invalid")
REDACTED

	method := strings.ToLower(strings.TrimSpace(effective.TokenAuthMethod))
	switch method {
	case "", "client_secret_post", "client_secret_basic":
		if strings.TrimSpace(effective.ClientSecret) == "" {
			return config.LinuxDoConnectConfig{REDACTED, infraerrors.InternalServer("OAUTH_CONFIG_INVALID", "oauth client secret not configured")
	REDACTED
	case "none":
		if !effective.UsePKCE {
			return config.LinuxDoConnectConfig{REDACTED, infraerrors.InternalServer("OAUTH_CONFIG_INVALID", "oauth pkce must be enabled when token_auth_method=none")
	REDACTED
	default:
		return config.LinuxDoConnectConfig{REDACTED, infraerrors.InternalServer("OAUTH_CONFIG_INVALID", "oauth token_auth_method invalid")
REDACTED

	return effective, nil
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

// IsIdentityPatchEnabled 检查是否启用身份补丁（Claude -> Gemini systemInstruction 注入）
func (s *SettingService) IsIdentityPatchEnabled(ctx context.Context) bool {
	value, err := s.settingRepo.GetValue(ctx, SettingKeyEnableIdentityPatch)
	if err != nil {
		// 默认开启，保持兼容
		return true
REDACTED
	return value == "true"
REDACTED

// GetIdentityPatchPrompt 获取自定义身份补丁提示词（为空表示使用内置默认模板）
func (s *SettingService) GetIdentityPatchPrompt(ctx context.Context) string {
	value, err := s.settingRepo.GetValue(ctx, SettingKeyIdentityPatchPrompt)
	if err != nil {
		return ""
REDACTED
	return value
REDACTED

// GenerateAdminAPIKey 生成新的管理员 API Key
func (s *SettingService) GenerateAdminAPIKey(ctx context.Context) (string, error) {
	// 生成 32 字节随机数 = 64 位十六进制字符
	bytes := make([]byte, 32)
	if _, err := rand.Read(bytes); err != nil {
		return "", fmt.Errorf("generate random bytes: %w", err)
REDACTED

	key := AdminAPIKeyPrefix + hex.EncodeToString(bytes)

	// 存储到 settings 表
	if err := s.settingRepo.Set(ctx, SettingKeyAdminAPIKey, key); err != nil {
		return "", fmt.Errorf("save admin api key: %w", err)
REDACTED

	return key, nil
REDACTED

// GetAdminAPIKeyStatus 获取管理员 API Key 状态
// 返回脱敏的 key、是否存在、错误
func (s *SettingService) GetAdminAPIKeyStatus(ctx context.Context) (maskedKey string, exists bool, err error) {
	key, err := s.settingRepo.GetValue(ctx, SettingKeyAdminAPIKey)
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

// GetAdminAPIKey 获取完整的管理员 API Key（仅供内部验证使用）
// 如果未配置返回空字符串和 nil 错误，只有数据库错误时才返回 error
func (s *SettingService) GetAdminAPIKey(ctx context.Context) (string, error) {
	key, err := s.settingRepo.GetValue(ctx, SettingKeyAdminAPIKey)
	if err != nil {
		if errors.Is(err, ErrSettingNotFound) {
			return "", nil // 未配置，返回空字符串
	REDACTED
		return "", err // 数据库错误
REDACTED
	return key, nil
REDACTED

// DeleteAdminAPIKey 删除管理员 API Key
func (s *SettingService) DeleteAdminAPIKey(ctx context.Context) error {
	return s.settingRepo.Delete(ctx, SettingKeyAdminAPIKey)
REDACTED

// IsModelFallbackEnabled 检查是否启用模型兜底机制
func (s *SettingService) IsModelFallbackEnabled(ctx context.Context) bool {
	value, err := s.settingRepo.GetValue(ctx, SettingKeyEnableModelFallback)
	if err != nil {
		return false // Default: disabled
REDACTED
	return value == "true"
REDACTED

// GetFallbackModel 获取指定平台的兜底模型
func (s *SettingService) GetFallbackModel(ctx context.Context, platform string) string {
	var key string
	var defaultModel string

	switch platform {
	case PlatformAnthropic:
		key = SettingKeyFallbackModelAnthropic
		defaultModel = "claude-3-5-sonnet-20241022"
	case PlatformOpenAI:
		key = SettingKeyFallbackModelOpenAI
		defaultModel = "gpt-4o"
	case PlatformGemini:
		key = SettingKeyFallbackModelGemini
		defaultModel = "gemini-2.5-pro"
	case PlatformAntigravity:
		key = SettingKeyFallbackModelAntigravity
		defaultModel = "gemini-2.5-pro"
	default:
		return ""
REDACTED

	value, err := s.settingRepo.GetValue(ctx, key)
	if err != nil || value == "" {
		return defaultModel
REDACTED
	return value
REDACTED
