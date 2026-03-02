package service

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"golang.org/x/sync/singleflight"
)

var (
	ErrRegistrationDisabled   = infraerrors.Forbidden("REGISTRATION_DISABLED", "registration is currently disabled")
	ErrSettingNotFound        = infraerrors.NotFound("SETTING_NOT_FOUND", "setting not found")
	ErrSoraS3ProfileNotFound  = infraerrors.NotFound("SORA_S3_PROFILE_NOT_FOUND", "sora s3 profile not found")
	ErrSoraS3ProfileExists    = infraerrors.Conflict("SORA_S3_PROFILE_EXISTS", "sora s3 profile already exists")
	ErrDefaultSubGroupInvalid = infraerrors.BadRequest(
		"DEFAULT_SUBSCRIPTION_GROUP_INVALID",
		"default subscription group must exist and be subscription type",
	)
	ErrDefaultSubGroupDuplicate = infraerrors.BadRequest(
		"DEFAULT_SUBSCRIPTION_GROUP_DUPLICATE",
		"default subscription group cannot be duplicated",
	)
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

// cachedMinVersion 缓存最低 Claude Code 版本号（进程内缓存，60s TTL）
type cachedMinVersion struct {
	value     string // 空字符串 = 不检查
	expiresAt int64  // unix nano
REDACTED

// minVersionCache 最低版本号进程内缓存
var minVersionCache atomic.Value // *cachedMinVersion

// minVersionSF 防止缓存过期时 thundering herd
var minVersionSF singleflight.Group

// minVersionCacheTTL 缓存有效期
const minVersionCacheTTL = 60 * time.Second

// minVersionErrorTTL DB 错误时的短缓存，快速重试
const minVersionErrorTTL = 5 * time.Second

// minVersionDBTimeout singleflight 内 DB 查询超时，独立于请求 context
const minVersionDBTimeout = 5 * time.Second

// DefaultSubscriptionGroupReader validates group references used by default subscriptions.
type DefaultSubscriptionGroupReader interface {
	GetByID(ctx context.Context, id int64) (*Group, error)
REDACTED

// SettingService 系统设置服务
type SettingService struct {
	settingRepo           SettingRepository
	defaultSubGroupReader DefaultSubscriptionGroupReader
	cfg                   *config.Config
	onUpdate              func() // Callback when settings are updated (for cache invalidation)
	onS3Update            func() // Callback when Sora S3 settings are updated
	version               string // Application version
REDACTED

// NewSettingService 创建系统设置服务实例
func NewSettingService(settingRepo SettingRepository, cfg *config.Config) *SettingService {
	return &SettingService{
		settingRepo: settingRepo,
		cfg:         cfg,
REDACTED
REDACTED

// SetDefaultSubscriptionGroupReader injects an optional group reader for default subscription validation.
func (s *SettingService) SetDefaultSubscriptionGroupReader(reader DefaultSubscriptionGroupReader) {
	s.defaultSubGroupReader = reader
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
		SettingKeyPromoCodeEnabled,
		SettingKeyPasswordResetEnabled,
		SettingKeyInvitationCodeEnabled,
		SettingKeyTotpEnabled,
		SettingKeyTurnstileEnabled,
		SettingKeyTurnstileSiteKey,
		SettingKeySiteName,
		SettingKeySiteLogo,
		SettingKeySiteSubtitle,
		SettingKeyAPIBaseURL,
		SettingKeyContactInfo,
		SettingKeyDocURL,
		SettingKeyHomeContent,
		SettingKeyHideCcsImportButton,
		SettingKeyPurchaseSubscriptionEnabled,
		SettingKeyPurchaseSubscriptionURL,
		SettingKeySoraClientEnabled,
		SettingKeyCustomMenuItems,
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

	// Password reset requires email verification to be enabled
	emailVerifyEnabled := settings[SettingKeyEmailVerifyEnabled] == "true"
	passwordResetEnabled := emailVerifyEnabled && settings[SettingKeyPasswordResetEnabled] == "true"

	return &PublicSettings{
		RegistrationEnabled:         settings[SettingKeyRegistrationEnabled] == "true",
		EmailVerifyEnabled:          emailVerifyEnabled,
		PromoCodeEnabled:            settings[SettingKeyPromoCodeEnabled] != "false", // 默认启用
		PasswordResetEnabled:        passwordResetEnabled,
		InvitationCodeEnabled:       settings[SettingKeyInvitationCodeEnabled] == "true",
		TotpEnabled:                 settings[SettingKeyTotpEnabled] == "true",
		TurnstileEnabled:            settings[SettingKeyTurnstileEnabled] == "true",
		TurnstileSiteKey:            settings[SettingKeyTurnstileSiteKey],
		SiteName:                    s.getStringOrDefault(settings, SettingKeySiteName, "Sub2API"),
		SiteLogo:                    settings[SettingKeySiteLogo],
		SiteSubtitle:                s.getStringOrDefault(settings, SettingKeySiteSubtitle, "Subscription to API Conversion Platform"),
		APIBaseURL:                  settings[SettingKeyAPIBaseURL],
		ContactInfo:                 settings[SettingKeyContactInfo],
		DocURL:                      settings[SettingKeyDocURL],
		HomeContent:                 settings[SettingKeyHomeContent],
		HideCcsImportButton:         settings[SettingKeyHideCcsImportButton] == "true",
		PurchaseSubscriptionEnabled: settings[SettingKeyPurchaseSubscriptionEnabled] == "true",
		PurchaseSubscriptionURL:     strings.TrimSpace(settings[SettingKeyPurchaseSubscriptionURL]),
		SoraClientEnabled:           settings[SettingKeySoraClientEnabled] == "true",
		CustomMenuItems:             settings[SettingKeyCustomMenuItems],
		LinuxDoOAuthEnabled:         linuxDoEnabled,
REDACTED, nil
REDACTED

// SetOnUpdateCallback sets a callback function to be called when settings are updated
// This is used for cache invalidation (e.g., HTML cache in frontend server)
func (s *SettingService) SetOnUpdateCallback(callback func()) {
	s.onUpdate = callback
REDACTED

// SetOnS3UpdateCallback 设置 Sora S3 配置变更时的回调函数（用于刷新 S3 客户端缓存）。
func (s *SettingService) SetOnS3UpdateCallback(callback func()) {
	s.onS3Update = callback
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
		RegistrationEnabled         bool            `json:"registration_enabled"`
		EmailVerifyEnabled          bool            `json:"email_verify_enabled"`
		PromoCodeEnabled            bool            `json:"promo_code_enabled"`
		PasswordResetEnabled        bool            `json:"password_reset_enabled"`
		InvitationCodeEnabled       bool            `json:"invitation_code_enabled"`
		TotpEnabled                 bool            `json:"totp_enabled"`
		TurnstileEnabled            bool            `json:"turnstile_enabled"`
		TurnstileSiteKey            string          `json:"turnstile_site_key,omitempty"`
		SiteName                    string          `json:"site_name"`
		SiteLogo                    string          `json:"site_logo,omitempty"`
		SiteSubtitle                string          `json:"site_subtitle,omitempty"`
		APIBaseURL                  string          `json:"api_base_url,omitempty"`
		ContactInfo                 string          `json:"contact_info,omitempty"`
		DocURL                      string          `json:"doc_url,omitempty"`
		HomeContent                 string          `json:"home_content,omitempty"`
		HideCcsImportButton         bool            `json:"hide_ccs_import_button"`
		PurchaseSubscriptionEnabled bool            `json:"purchase_subscription_enabled"`
		PurchaseSubscriptionURL     string          `json:"purchase_subscription_url,omitempty"`
		SoraClientEnabled           bool            `json:"sora_client_enabled"`
		CustomMenuItems             json.RawMessage `json:"custom_menu_items"`
		LinuxDoOAuthEnabled         bool            `json:"linuxdo_oauth_enabled"`
		Version                     string          `json:"version,omitempty"`
REDACTED{
		RegistrationEnabled:         settings.RegistrationEnabled,
		EmailVerifyEnabled:          settings.EmailVerifyEnabled,
		PromoCodeEnabled:            settings.PromoCodeEnabled,
		PasswordResetEnabled:        settings.PasswordResetEnabled,
		InvitationCodeEnabled:       settings.InvitationCodeEnabled,
		TotpEnabled:                 settings.TotpEnabled,
		TurnstileEnabled:            settings.TurnstileEnabled,
		TurnstileSiteKey:            settings.TurnstileSiteKey,
		SiteName:                    settings.SiteName,
		SiteLogo:                    settings.SiteLogo,
		SiteSubtitle:                settings.SiteSubtitle,
		APIBaseURL:                  settings.APIBaseURL,
		ContactInfo:                 settings.ContactInfo,
		DocURL:                      settings.DocURL,
		HomeContent:                 settings.HomeContent,
		HideCcsImportButton:         settings.HideCcsImportButton,
		PurchaseSubscriptionEnabled: settings.PurchaseSubscriptionEnabled,
		PurchaseSubscriptionURL:     settings.PurchaseSubscriptionURL,
		SoraClientEnabled:           settings.SoraClientEnabled,
		CustomMenuItems:             filterUserVisibleMenuItems(settings.CustomMenuItems),
		LinuxDoOAuthEnabled:         settings.LinuxDoOAuthEnabled,
		Version:                     s.version,
REDACTED, nil
REDACTED

// filterUserVisibleMenuItems filters out admin-only menu items from a raw JSON
// array string, returning only items with visibility != "admin".
func filterUserVisibleMenuItems(raw string) json.RawMessage {
	raw = strings.TrimSpace(raw)
	if raw == "" || raw == "[]" {
		return json.RawMessage("[]")
REDACTED
	var items []struct {
		Visibility string `json:"visibility"`
REDACTED
	if err := json.Unmarshal([]byte(raw), &items); err != nil {
		return json.RawMessage("[]")
REDACTED

	// Parse full items to preserve all fields
	var fullItems []json.RawMessage
	if err := json.Unmarshal([]byte(raw), &fullItems); err != nil {
		return json.RawMessage("[]")
REDACTED

	var filtered []json.RawMessage
	for i, item := range items {
		if item.Visibility != "admin" {
			filtered = append(filtered, fullItems[i])
	REDACTED
REDACTED
	if len(filtered) == 0 {
		return json.RawMessage("[]")
REDACTED
	result, err := json.Marshal(filtered)
	if err != nil {
		return json.RawMessage("[]")
REDACTED
	return result
REDACTED

// GetFrameSrcOrigins returns deduplicated http(s) origins from purchase_subscription_url
// and all custom_menu_items URLs. Used by the router layer for CSP frame-src injection.
func (s *SettingService) GetFrameSrcOrigins(ctx context.Context) ([]string, error) {
	settings, err := s.GetPublicSettings(ctx)
	if err != nil {
		return nil, err
REDACTED

	seen := make(map[string]struct{REDACTED)
	var origins []string

	addOrigin := func(rawURL string) {
		if origin := extractOriginFromURL(rawURL); origin != "" {
			if _, ok := seen[origin]; !ok {
				seen[origin] = struct{REDACTED{REDACTED
				origins = append(origins, origin)
		REDACTED
	REDACTED
REDACTED

	// purchase subscription URL
	if settings.PurchaseSubscriptionEnabled {
		addOrigin(settings.PurchaseSubscriptionURL)
REDACTED

	// all custom menu items (including admin-only, since CSP must allow all iframes)
	for _, item := range parseCustomMenuItemURLs(settings.CustomMenuItems) {
		addOrigin(item)
REDACTED

	return origins, nil
REDACTED

// extractOriginFromURL returns the scheme+host origin from rawURL.
// Only http and https schemes are accepted.
func extractOriginFromURL(rawURL string) string {
	rawURL = strings.TrimSpace(rawURL)
	if rawURL == "" {
		return ""
REDACTED
	u, err := url.Parse(rawURL)
	if err != nil || u.Host == "" {
		return ""
REDACTED
	if u.Scheme != "http" && u.Scheme != "https" {
		return ""
REDACTED
	return u.Scheme + "://" + u.Host
REDACTED

// parseCustomMenuItemURLs extracts URLs from a raw JSON array of custom menu items.
func parseCustomMenuItemURLs(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" || raw == "[]" {
		return nil
REDACTED
	var items []struct {
		URL string `json:"url"`
REDACTED
	if err := json.Unmarshal([]byte(raw), &items); err != nil {
		return nil
REDACTED
	urls := make([]string, 0, len(items))
	for _, item := range items {
		if item.URL != "" {
			urls = append(urls, item.URL)
	REDACTED
REDACTED
	return urls
REDACTED

// UpdateSettings 更新系统设置
func (s *SettingService) UpdateSettings(ctx context.Context, settings *SystemSettings) error {
	if err := s.validateDefaultSubscriptionGroups(ctx, settings.DefaultSubscriptions); err != nil {
		return err
REDACTED

	updates := make(map[string]string)

	// 注册设置
	updates[SettingKeyRegistrationEnabled] = strconv.FormatBool(settings.RegistrationEnabled)
	updates[SettingKeyEmailVerifyEnabled] = strconv.FormatBool(settings.EmailVerifyEnabled)
	updates[SettingKeyPromoCodeEnabled] = strconv.FormatBool(settings.PromoCodeEnabled)
	updates[SettingKeyPasswordResetEnabled] = strconv.FormatBool(settings.PasswordResetEnabled)
	updates[SettingKeyInvitationCodeEnabled] = strconv.FormatBool(settings.InvitationCodeEnabled)
	updates[SettingKeyTotpEnabled] = strconv.FormatBool(settings.TotpEnabled)

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

	// LinuxDo Connect OAuth 登录
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
	updates[SettingKeyHideCcsImportButton] = strconv.FormatBool(settings.HideCcsImportButton)
	updates[SettingKeyPurchaseSubscriptionEnabled] = strconv.FormatBool(settings.PurchaseSubscriptionEnabled)
	updates[SettingKeyPurchaseSubscriptionURL] = strings.TrimSpace(settings.PurchaseSubscriptionURL)
	updates[SettingKeySoraClientEnabled] = strconv.FormatBool(settings.SoraClientEnabled)
	updates[SettingKeyCustomMenuItems] = settings.CustomMenuItems

	// 默认配置
	updates[SettingKeyDefaultConcurrency] = strconv.Itoa(settings.DefaultConcurrency)
	updates[SettingKeyDefaultBalance] = strconv.FormatFloat(settings.DefaultBalance, 'f', 8, 64)
	defaultSubsJSON, err := json.Marshal(settings.DefaultSubscriptions)
	if err != nil {
		return fmt.Errorf("marshal default subscriptions: %w", err)
REDACTED
	updates[SettingKeyDefaultSubscriptions] = string(defaultSubsJSON)

	// Model fallback configuration
	updates[SettingKeyEnableModelFallback] = strconv.FormatBool(settings.EnableModelFallback)
	updates[SettingKeyFallbackModelAnthropic] = settings.FallbackModelAnthropic
	updates[SettingKeyFallbackModelOpenAI] = settings.FallbackModelOpenAI
	updates[SettingKeyFallbackModelGemini] = settings.FallbackModelGemini
	updates[SettingKeyFallbackModelAntigravity] = settings.FallbackModelAntigravity

	// Identity patch configuration (Claude -> Gemini)
	updates[SettingKeyEnableIdentityPatch] = strconv.FormatBool(settings.EnableIdentityPatch)
	updates[SettingKeyIdentityPatchPrompt] = settings.IdentityPatchPrompt

	// Ops monitoring (vNext)
	updates[SettingKeyOpsMonitoringEnabled] = strconv.FormatBool(settings.OpsMonitoringEnabled)
	updates[SettingKeyOpsRealtimeMonitoringEnabled] = strconv.FormatBool(settings.OpsRealtimeMonitoringEnabled)
	updates[SettingKeyOpsQueryModeDefault] = string(ParseOpsQueryMode(settings.OpsQueryModeDefault))
	if settings.OpsMetricsIntervalSeconds > 0 {
		updates[SettingKeyOpsMetricsIntervalSeconds] = strconv.Itoa(settings.OpsMetricsIntervalSeconds)
REDACTED

	// Claude Code version check
	updates[SettingKeyMinClaudeCodeVersion] = settings.MinClaudeCodeVersion

	err = s.settingRepo.SetMultiple(ctx, updates)
	if err == nil {
		// 先使 inflight singleflight 失效，再刷新缓存，缩小旧值覆盖新值的竞态窗口
		minVersionSF.Forget("min_version")
		minVersionCache.Store(&cachedMinVersion{
			value:     settings.MinClaudeCodeVersion,
			expiresAt: time.Now().Add(minVersionCacheTTL).UnixNano(),
	REDACTED)
		if s.onUpdate != nil {
			s.onUpdate() // Invalidate cache after settings update
	REDACTED
REDACTED
	return err
REDACTED

func (s *SettingService) validateDefaultSubscriptionGroups(ctx context.Context, items []DefaultSubscriptionSetting) error {
	if len(items) == 0 {
		return nil
REDACTED

	checked := make(map[int64]struct{REDACTED, len(items))
	for _, item := range items {
		if item.GroupID <= 0 {
			continue
	REDACTED
		if _, ok := checked[item.GroupID]; ok {
			return ErrDefaultSubGroupDuplicate.WithMetadata(map[string]string{
				"group_id": strconv.FormatInt(item.GroupID, 10),
		REDACTED)
	REDACTED
		checked[item.GroupID] = struct{REDACTED{REDACTED
		if s.defaultSubGroupReader == nil {
			continue
	REDACTED

		group, err := s.defaultSubGroupReader.GetByID(ctx, item.GroupID)
		if err != nil {
			if errors.Is(err, ErrGroupNotFound) {
				return ErrDefaultSubGroupInvalid.WithMetadata(map[string]string{
					"group_id": strconv.FormatInt(item.GroupID, 10),
			REDACTED)
		REDACTED
			return fmt.Errorf("get default subscription group %d: %w", item.GroupID, err)
	REDACTED
		if !group.IsSubscriptionType() {
			return ErrDefaultSubGroupInvalid.WithMetadata(map[string]string{
				"group_id": strconv.FormatInt(item.GroupID, 10),
		REDACTED)
	REDACTED
REDACTED

	return nil
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

// IsPromoCodeEnabled 检查是否启用优惠码功能
func (s *SettingService) IsPromoCodeEnabled(ctx context.Context) bool {
	value, err := s.settingRepo.GetValue(ctx, SettingKeyPromoCodeEnabled)
	if err != nil {
		return true // 默认启用
REDACTED
	return value != "false"
REDACTED

// IsInvitationCodeEnabled 检查是否启用邀请码注册功能
func (s *SettingService) IsInvitationCodeEnabled(ctx context.Context) bool {
	value, err := s.settingRepo.GetValue(ctx, SettingKeyInvitationCodeEnabled)
	if err != nil {
		return false // 默认关闭
REDACTED
	return value == "true"
REDACTED

// IsPasswordResetEnabled 检查是否启用密码重置功能
// 要求：必须同时开启邮件验证
func (s *SettingService) IsPasswordResetEnabled(ctx context.Context) bool {
	// Password reset requires email verification to be enabled
	if !s.IsEmailVerifyEnabled(ctx) {
		return false
REDACTED
	value, err := s.settingRepo.GetValue(ctx, SettingKeyPasswordResetEnabled)
	if err != nil {
		return false // 默认关闭
REDACTED
	return value == "true"
REDACTED

// IsTotpEnabled 检查是否启用 TOTP 双因素认证功能
func (s *SettingService) IsTotpEnabled(ctx context.Context) bool {
	value, err := s.settingRepo.GetValue(ctx, SettingKeyTotpEnabled)
	if err != nil {
		return false // 默认关闭
REDACTED
	return value == "true"
REDACTED

// IsTotpEncryptionKeyConfigured 检查 TOTP 加密密钥是否已手动配置
// 只有手动配置了密钥才允许在管理后台启用 TOTP 功能
func (s *SettingService) IsTotpEncryptionKeyConfigured() bool {
	return s.cfg.Totp.EncryptionKeyConfigured
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

// GetDefaultSubscriptions 获取新用户默认订阅配置列表。
func (s *SettingService) GetDefaultSubscriptions(ctx context.Context) []DefaultSubscriptionSetting {
	value, err := s.settingRepo.GetValue(ctx, SettingKeyDefaultSubscriptions)
	if err != nil {
		return nil
REDACTED
	return parseDefaultSubscriptions(value)
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
		SettingKeyRegistrationEnabled:         "true",
		SettingKeyEmailVerifyEnabled:          "false",
		SettingKeyPromoCodeEnabled:            "true", // 默认启用优惠码功能
		SettingKeySiteName:                    "Sub2API",
		SettingKeySiteLogo:                    "",
		SettingKeyPurchaseSubscriptionEnabled: "false",
		SettingKeyPurchaseSubscriptionURL:     "",
		SettingKeySoraClientEnabled:           "false",
		SettingKeyCustomMenuItems:             "[]",
		SettingKeyDefaultConcurrency:          strconv.Itoa(s.cfg.Default.UserConcurrency),
		SettingKeyDefaultBalance:              strconv.FormatFloat(s.cfg.Default.UserBalance, 'f', 8, 64),
		SettingKeyDefaultSubscriptions:        "[]",
		SettingKeySMTPPort:                    "587",
		SettingKeySMTPUseTLS:                  "false",
		// Model fallback defaults
		SettingKeyEnableModelFallback:      "false",
		SettingKeyFallbackModelAnthropic:   "claude-3-5-sonnet-20241022",
		SettingKeyFallbackModelOpenAI:      "gpt-4o",
		SettingKeyFallbackModelGemini:      "gemini-2.5-pro",
		SettingKeyFallbackModelAntigravity: "gemini-2.5-pro",
		// Identity patch defaults
		SettingKeyEnableIdentityPatch: "true",
		SettingKeyIdentityPatchPrompt: "",

		// Ops monitoring defaults (vNext)
		SettingKeyOpsMonitoringEnabled:         "true",
		SettingKeyOpsRealtimeMonitoringEnabled: "true",
		SettingKeyOpsQueryModeDefault:          "auto",
		SettingKeyOpsMetricsIntervalSeconds:    "60",

		// Claude Code version check (default: empty = disabled)
		SettingKeyMinClaudeCodeVersion: "",
REDACTED

	return s.settingRepo.SetMultiple(ctx, defaults)
REDACTED

// parseSettings 解析设置到结构体
func (s *SettingService) parseSettings(settings map[string]string) *SystemSettings {
	emailVerifyEnabled := settings[SettingKeyEmailVerifyEnabled] == "true"
	result := &SystemSettings{
		RegistrationEnabled:          settings[SettingKeyRegistrationEnabled] == "true",
		EmailVerifyEnabled:           emailVerifyEnabled,
		PromoCodeEnabled:             settings[SettingKeyPromoCodeEnabled] != "false", // 默认启用
		PasswordResetEnabled:         emailVerifyEnabled && settings[SettingKeyPasswordResetEnabled] == "true",
		InvitationCodeEnabled:        settings[SettingKeyInvitationCodeEnabled] == "true",
		TotpEnabled:                  settings[SettingKeyTotpEnabled] == "true",
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
		HideCcsImportButton:          settings[SettingKeyHideCcsImportButton] == "true",
		PurchaseSubscriptionEnabled:  settings[SettingKeyPurchaseSubscriptionEnabled] == "true",
		PurchaseSubscriptionURL:      strings.TrimSpace(settings[SettingKeyPurchaseSubscriptionURL]),
		SoraClientEnabled:            settings[SettingKeySoraClientEnabled] == "true",
		CustomMenuItems:              settings[SettingKeyCustomMenuItems],
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
	result.DefaultSubscriptions = parseDefaultSubscriptions(settings[SettingKeyDefaultSubscriptions])

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

	// Ops monitoring settings (default: enabled, fail-open)
	result.OpsMonitoringEnabled = !isFalseSettingValue(settings[SettingKeyOpsMonitoringEnabled])
	result.OpsRealtimeMonitoringEnabled = !isFalseSettingValue(settings[SettingKeyOpsRealtimeMonitoringEnabled])
	result.OpsQueryModeDefault = string(ParseOpsQueryMode(settings[SettingKeyOpsQueryModeDefault]))
	result.OpsMetricsIntervalSeconds = 60
	if raw := strings.TrimSpace(settings[SettingKeyOpsMetricsIntervalSeconds]); raw != "" {
		if v, err := strconv.Atoi(raw); err == nil {
			if v < 60 {
				v = 60
		REDACTED
			if v > 3600 {
				v = 3600
		REDACTED
			result.OpsMetricsIntervalSeconds = v
	REDACTED
REDACTED

	// Claude Code version check
	result.MinClaudeCodeVersion = settings[SettingKeyMinClaudeCodeVersion]

	return result
REDACTED

func isFalseSettingValue(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "false", "0", "off", "disabled":
		return true
	default:
		return false
REDACTED
REDACTED

func parseDefaultSubscriptions(raw string) []DefaultSubscriptionSetting {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
REDACTED

	var items []DefaultSubscriptionSetting
	if err := json.Unmarshal([]byte(raw), &items); err != nil {
		return nil
REDACTED

	normalized := make([]DefaultSubscriptionSetting, 0, len(items))
	for _, item := range items {
		if item.GroupID <= 0 || item.ValidityDays <= 0 {
			continue
	REDACTED
		if item.ValidityDays > MaxValidityDays {
			item.ValidityDays = MaxValidityDays
	REDACTED
		normalized = append(normalized, item)
REDACTED

	return normalized
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

// GetLinuxDoConnectOAuthConfig 返回用于登录的"最终生效" LinuxDo Connect 配置。
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

// GetStreamTimeoutSettings 获取流超时处理配置
func (s *SettingService) GetStreamTimeoutSettings(ctx context.Context) (*StreamTimeoutSettings, error) {
	value, err := s.settingRepo.GetValue(ctx, SettingKeyStreamTimeoutSettings)
	if err != nil {
		if errors.Is(err, ErrSettingNotFound) {
			return DefaultStreamTimeoutSettings(), nil
	REDACTED
		return nil, fmt.Errorf("get stream timeout settings: %w", err)
REDACTED
	if value == "" {
		return DefaultStreamTimeoutSettings(), nil
REDACTED

	var settings StreamTimeoutSettings
	if err := json.Unmarshal([]byte(value), &settings); err != nil {
		return DefaultStreamTimeoutSettings(), nil
REDACTED

	// 验证并修正配置值
	if settings.TempUnschedMinutes < 1 {
		settings.TempUnschedMinutes = 1
REDACTED
	if settings.TempUnschedMinutes > 60 {
		settings.TempUnschedMinutes = 60
REDACTED
	if settings.ThresholdCount < 1 {
		settings.ThresholdCount = 1
REDACTED
	if settings.ThresholdCount > 10 {
		settings.ThresholdCount = 10
REDACTED
	if settings.ThresholdWindowMinutes < 1 {
		settings.ThresholdWindowMinutes = 1
REDACTED
	if settings.ThresholdWindowMinutes > 60 {
		settings.ThresholdWindowMinutes = 60
REDACTED

	// 验证 action
	switch settings.Action {
	case StreamTimeoutActionTempUnsched, StreamTimeoutActionError, StreamTimeoutActionNone:
		// valid
	default:
		settings.Action = StreamTimeoutActionTempUnsched
REDACTED

	return &settings, nil
REDACTED

// GetMinClaudeCodeVersion 获取最低 Claude Code 版本号要求
// 使用进程内 atomic.Value 缓存，60 秒 TTL，热路径零锁开销
// singleflight 防止缓存过期时 thundering herd
// 返回空字符串表示不做版本检查
func (s *SettingService) GetMinClaudeCodeVersion(ctx context.Context) string {
	if cached, ok := minVersionCache.Load().(*cachedMinVersion); ok {
		if time.Now().UnixNano() < cached.expiresAt {
			return cached.value
	REDACTED
REDACTED
	// singleflight: 同一时刻只有一个 goroutine 查询 DB，其余复用结果
	result, _, _ := minVersionSF.Do("min_version", func() (any, error) {
		// 二次检查，避免排队的 goroutine 重复查询
		if cached, ok := minVersionCache.Load().(*cachedMinVersion); ok {
			if time.Now().UnixNano() < cached.expiresAt {
				return cached.value, nil
		REDACTED
	REDACTED
		// 使用独立 context：断开请求取消链，避免客户端断连导致空值被长期缓存
		dbCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), minVersionDBTimeout)
		defer cancel()
		value, err := s.settingRepo.GetValue(dbCtx, SettingKeyMinClaudeCodeVersion)
		if err != nil {
			// fail-open: DB 错误时不阻塞请求，但记录日志并使用短 TTL 快速重试
			slog.Warn("failed to get min claude code version setting, skipping version check", "error", err)
			minVersionCache.Store(&cachedMinVersion{
				value:     "",
				expiresAt: time.Now().Add(minVersionErrorTTL).UnixNano(),
		REDACTED)
			return "", nil
	REDACTED
		minVersionCache.Store(&cachedMinVersion{
			value:     value,
			expiresAt: time.Now().Add(minVersionCacheTTL).UnixNano(),
	REDACTED)
		return value, nil
REDACTED)
	if s, ok := result.(string); ok {
		return s
REDACTED
	return ""
REDACTED

// SetStreamTimeoutSettings 设置流超时处理配置
func (s *SettingService) SetStreamTimeoutSettings(ctx context.Context, settings *StreamTimeoutSettings) error {
	if settings == nil {
		return fmt.Errorf("settings cannot be nil")
REDACTED

	// 验证配置值
	if settings.TempUnschedMinutes < 1 || settings.TempUnschedMinutes > 60 {
		return fmt.Errorf("temp_unsched_minutes must be between 1-60")
REDACTED
	if settings.ThresholdCount < 1 || settings.ThresholdCount > 10 {
		return fmt.Errorf("threshold_count must be between 1-10")
REDACTED
	if settings.ThresholdWindowMinutes < 1 || settings.ThresholdWindowMinutes > 60 {
		return fmt.Errorf("threshold_window_minutes must be between 1-60")
REDACTED

	switch settings.Action {
	case StreamTimeoutActionTempUnsched, StreamTimeoutActionError, StreamTimeoutActionNone:
		// valid
	default:
		return fmt.Errorf("invalid action: %s", settings.Action)
REDACTED

	data, err := json.Marshal(settings)
	if err != nil {
		return fmt.Errorf("marshal stream timeout settings: %w", err)
REDACTED

	return s.settingRepo.Set(ctx, SettingKeyStreamTimeoutSettings, string(data))
REDACTED

type soraS3ProfilesStore struct {
	ActiveProfileID string                   `json:"active_profile_id"`
	Items           []soraS3ProfileStoreItem `json:"items"`
REDACTED

type soraS3ProfileStoreItem struct {
	ProfileID                string `json:"profile_id"`
	Name                     string `json:"name"`
	Enabled                  bool   `json:"enabled"`
	Endpoint                 string `json:"endpoint"`
	Region                   string `json:"region"`
	Bucket                   string `json:"bucket"`
	AccessKeyID              string `json:"access_key_id"`
	SecretAccessKey          string `json:"secret_access_key"`
	Prefix                   string `json:"prefix"`
	ForcePathStyle           bool   `json:"force_path_style"`
	CDNURL                   string `json:"cdn_url"`
	DefaultStorageQuotaBytes int64  `json:"default_storage_quota_bytes"`
	UpdatedAt                string `json:"updated_at"`
REDACTED

// GetSoraS3Settings 获取 Sora S3 存储配置（兼容旧单配置语义：返回当前激活配置）
func (s *SettingService) GetSoraS3Settings(ctx context.Context) (*SoraS3Settings, error) {
	profiles, err := s.ListSoraS3Profiles(ctx)
	if err != nil {
		return nil, err
REDACTED

	activeProfile := pickActiveSoraS3Profile(profiles.Items, profiles.ActiveProfileID)
	if activeProfile == nil {
		return &SoraS3Settings{REDACTED, nil
REDACTED

	return &SoraS3Settings{
		Enabled:                   activeProfile.Enabled,
		Endpoint:                  activeProfile.Endpoint,
		Region:                    activeProfile.Region,
		Bucket:                    activeProfile.Bucket,
		AccessKeyID:               activeProfile.AccessKeyID,
		SecretAccessKey:           activeProfile.SecretAccessKey,
		SecretAccessKeyConfigured: activeProfile.SecretAccessKeyConfigured,
		Prefix:                    activeProfile.Prefix,
		ForcePathStyle:            activeProfile.ForcePathStyle,
		CDNURL:                    activeProfile.CDNURL,
		DefaultStorageQuotaBytes:  activeProfile.DefaultStorageQuotaBytes,
REDACTED, nil
REDACTED

// SetSoraS3Settings 更新 Sora S3 存储配置（兼容旧单配置语义：写入当前激活配置）
func (s *SettingService) SetSoraS3Settings(ctx context.Context, settings *SoraS3Settings) error {
	if settings == nil {
		return fmt.Errorf("settings cannot be nil")
REDACTED

	store, err := s.loadSoraS3ProfilesStore(ctx)
	if err != nil {
		return err
REDACTED

	now := time.Now().UTC().Format(time.RFC3339)
	activeIndex := findSoraS3ProfileIndex(store.Items, store.ActiveProfileID)
	if activeIndex < 0 {
		activeID := "default"
		if hasSoraS3ProfileID(store.Items, activeID) {
			activeID = fmt.Sprintf("default-%d", time.Now().Unix())
	REDACTED
		store.Items = append(store.Items, soraS3ProfileStoreItem{
			ProfileID: activeID,
			Name:      "Default",
			UpdatedAt: now,
	REDACTED)
		store.ActiveProfileID = activeID
		activeIndex = len(store.Items) - 1
REDACTED

	active := store.Items[activeIndex]
	active.Enabled = settings.Enabled
	active.Endpoint = strings.TrimSpace(settings.Endpoint)
	active.Region = strings.TrimSpace(settings.Region)
	active.Bucket = strings.TrimSpace(settings.Bucket)
	active.AccessKeyID = strings.TrimSpace(settings.AccessKeyID)
	active.Prefix = strings.TrimSpace(settings.Prefix)
	active.ForcePathStyle = settings.ForcePathStyle
	active.CDNURL = strings.TrimSpace(settings.CDNURL)
	active.DefaultStorageQuotaBytes = maxInt64(settings.DefaultStorageQuotaBytes, 0)
	if settings.SecretAccessKey != "" {
		active.SecretAccessKey = settings.SecretAccessKey
REDACTED
	active.UpdatedAt = now
	store.Items[activeIndex] = active

	return s.persistSoraS3ProfilesStore(ctx, store)
REDACTED

// ListSoraS3Profiles 获取 Sora S3 多配置列表
func (s *SettingService) ListSoraS3Profiles(ctx context.Context) (*SoraS3ProfileList, error) {
	store, err := s.loadSoraS3ProfilesStore(ctx)
	if err != nil {
		return nil, err
REDACTED
	return convertSoraS3ProfilesStore(store), nil
REDACTED

// CreateSoraS3Profile 创建 Sora S3 配置
func (s *SettingService) CreateSoraS3Profile(ctx context.Context, profile *SoraS3Profile, setActive bool) (*SoraS3Profile, error) {
	if profile == nil {
		return nil, fmt.Errorf("profile cannot be nil")
REDACTED

	profileID := strings.TrimSpace(profile.ProfileID)
	if profileID == "" {
		return nil, infraerrors.BadRequest("SORA_S3_PROFILE_ID_REQUIRED", "profile_id is required")
REDACTED
	name := strings.TrimSpace(profile.Name)
	if name == "" {
		return nil, infraerrors.BadRequest("SORA_S3_PROFILE_NAME_REQUIRED", "name is required")
REDACTED

	store, err := s.loadSoraS3ProfilesStore(ctx)
	if err != nil {
		return nil, err
REDACTED
	if hasSoraS3ProfileID(store.Items, profileID) {
		return nil, ErrSoraS3ProfileExists
REDACTED

	now := time.Now().UTC().Format(time.RFC3339)
	store.Items = append(store.Items, soraS3ProfileStoreItem{
		ProfileID:                profileID,
		Name:                     name,
		Enabled:                  profile.Enabled,
		Endpoint:                 strings.TrimSpace(profile.Endpoint),
		Region:                   strings.TrimSpace(profile.Region),
		Bucket:                   strings.TrimSpace(profile.Bucket),
		AccessKeyID:              strings.TrimSpace(profile.AccessKeyID),
		SecretAccessKey:          profile.SecretAccessKey,
		Prefix:                   strings.TrimSpace(profile.Prefix),
		ForcePathStyle:           profile.ForcePathStyle,
		CDNURL:                   strings.TrimSpace(profile.CDNURL),
		DefaultStorageQuotaBytes: maxInt64(profile.DefaultStorageQuotaBytes, 0),
		UpdatedAt:                now,
REDACTED)

	if setActive || store.ActiveProfileID == "" {
		store.ActiveProfileID = profileID
REDACTED

	if err := s.persistSoraS3ProfilesStore(ctx, store); err != nil {
		return nil, err
REDACTED

	profiles := convertSoraS3ProfilesStore(store)
	created := findSoraS3ProfileByID(profiles.Items, profileID)
	if created == nil {
		return nil, ErrSoraS3ProfileNotFound
REDACTED
	return created, nil
REDACTED

// UpdateSoraS3Profile 更新 Sora S3 配置
func (s *SettingService) UpdateSoraS3Profile(ctx context.Context, profileID string, profile *SoraS3Profile) (*SoraS3Profile, error) {
	if profile == nil {
		return nil, fmt.Errorf("profile cannot be nil")
REDACTED

	targetID := strings.TrimSpace(profileID)
	if targetID == "" {
		return nil, infraerrors.BadRequest("SORA_S3_PROFILE_ID_REQUIRED", "profile_id is required")
REDACTED

	store, err := s.loadSoraS3ProfilesStore(ctx)
	if err != nil {
		return nil, err
REDACTED

	targetIndex := findSoraS3ProfileIndex(store.Items, targetID)
	if targetIndex < 0 {
		return nil, ErrSoraS3ProfileNotFound
REDACTED

	target := store.Items[targetIndex]
	name := strings.TrimSpace(profile.Name)
	if name == "" {
		return nil, infraerrors.BadRequest("SORA_S3_PROFILE_NAME_REQUIRED", "name is required")
REDACTED
	target.Name = name
	target.Enabled = profile.Enabled
	target.Endpoint = strings.TrimSpace(profile.Endpoint)
	target.Region = strings.TrimSpace(profile.Region)
	target.Bucket = strings.TrimSpace(profile.Bucket)
	target.AccessKeyID = strings.TrimSpace(profile.AccessKeyID)
	target.Prefix = strings.TrimSpace(profile.Prefix)
	target.ForcePathStyle = profile.ForcePathStyle
	target.CDNURL = strings.TrimSpace(profile.CDNURL)
	target.DefaultStorageQuotaBytes = maxInt64(profile.DefaultStorageQuotaBytes, 0)
	if profile.SecretAccessKey != "" {
		target.SecretAccessKey = profile.SecretAccessKey
REDACTED
	target.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
	store.Items[targetIndex] = target

	if err := s.persistSoraS3ProfilesStore(ctx, store); err != nil {
		return nil, err
REDACTED

	profiles := convertSoraS3ProfilesStore(store)
	updated := findSoraS3ProfileByID(profiles.Items, targetID)
	if updated == nil {
		return nil, ErrSoraS3ProfileNotFound
REDACTED
	return updated, nil
REDACTED

// DeleteSoraS3Profile 删除 Sora S3 配置
func (s *SettingService) DeleteSoraS3Profile(ctx context.Context, profileID string) error {
	targetID := strings.TrimSpace(profileID)
	if targetID == "" {
		return infraerrors.BadRequest("SORA_S3_PROFILE_ID_REQUIRED", "profile_id is required")
REDACTED

	store, err := s.loadSoraS3ProfilesStore(ctx)
	if err != nil {
		return err
REDACTED

	targetIndex := findSoraS3ProfileIndex(store.Items, targetID)
	if targetIndex < 0 {
		return ErrSoraS3ProfileNotFound
REDACTED

	store.Items = append(store.Items[:targetIndex], store.Items[targetIndex+1:]...)
	if store.ActiveProfileID == targetID {
		store.ActiveProfileID = ""
		if len(store.Items) > 0 {
			store.ActiveProfileID = store.Items[0].ProfileID
	REDACTED
REDACTED

	return s.persistSoraS3ProfilesStore(ctx, store)
REDACTED

// SetActiveSoraS3Profile 设置激活的 Sora S3 配置
func (s *SettingService) SetActiveSoraS3Profile(ctx context.Context, profileID string) (*SoraS3Profile, error) {
	targetID := strings.TrimSpace(profileID)
	if targetID == "" {
		return nil, infraerrors.BadRequest("SORA_S3_PROFILE_ID_REQUIRED", "profile_id is required")
REDACTED

	store, err := s.loadSoraS3ProfilesStore(ctx)
	if err != nil {
		return nil, err
REDACTED

	targetIndex := findSoraS3ProfileIndex(store.Items, targetID)
	if targetIndex < 0 {
		return nil, ErrSoraS3ProfileNotFound
REDACTED

	store.ActiveProfileID = targetID
	store.Items[targetIndex].UpdatedAt = time.Now().UTC().Format(time.RFC3339)
	if err := s.persistSoraS3ProfilesStore(ctx, store); err != nil {
		return nil, err
REDACTED

	profiles := convertSoraS3ProfilesStore(store)
	active := pickActiveSoraS3Profile(profiles.Items, profiles.ActiveProfileID)
	if active == nil {
		return nil, ErrSoraS3ProfileNotFound
REDACTED
	return active, nil
REDACTED

func (s *SettingService) loadSoraS3ProfilesStore(ctx context.Context) (*soraS3ProfilesStore, error) {
	raw, err := s.settingRepo.GetValue(ctx, SettingKeySoraS3Profiles)
	if err == nil {
		trimmed := strings.TrimSpace(raw)
		if trimmed == "" {
			return &soraS3ProfilesStore{REDACTED, nil
	REDACTED
		var store soraS3ProfilesStore
		if unmarshalErr := json.Unmarshal([]byte(trimmed), &store); unmarshalErr != nil {
			legacy, legacyErr := s.getLegacySoraS3Settings(ctx)
			if legacyErr != nil {
				return nil, fmt.Errorf("unmarshal sora s3 profiles: %w", unmarshalErr)
		REDACTED
			if isEmptyLegacySoraS3Settings(legacy) {
				return &soraS3ProfilesStore{REDACTED, nil
		REDACTED
			now := time.Now().UTC().Format(time.RFC3339)
			return &soraS3ProfilesStore{
				ActiveProfileID: "default",
				Items: []soraS3ProfileStoreItem{
					{
						ProfileID:                "default",
						Name:                     "Default",
						Enabled:                  legacy.Enabled,
						Endpoint:                 strings.TrimSpace(legacy.Endpoint),
						Region:                   strings.TrimSpace(legacy.Region),
						Bucket:                   strings.TrimSpace(legacy.Bucket),
						AccessKeyID:              strings.TrimSpace(legacy.AccessKeyID),
						SecretAccessKey:          legacy.SecretAccessKey,
						Prefix:                   strings.TrimSpace(legacy.Prefix),
						ForcePathStyle:           legacy.ForcePathStyle,
						CDNURL:                   strings.TrimSpace(legacy.CDNURL),
						DefaultStorageQuotaBytes: maxInt64(legacy.DefaultStorageQuotaBytes, 0),
						UpdatedAt:                now,
				REDACTED,
			REDACTED,
		REDACTED, nil
	REDACTED
		normalized := normalizeSoraS3ProfilesStore(store)
		return &normalized, nil
REDACTED

	if !errors.Is(err, ErrSettingNotFound) {
		return nil, fmt.Errorf("get sora s3 profiles: %w", err)
REDACTED

	legacy, legacyErr := s.getLegacySoraS3Settings(ctx)
	if legacyErr != nil {
		return nil, legacyErr
REDACTED
	if isEmptyLegacySoraS3Settings(legacy) {
		return &soraS3ProfilesStore{REDACTED, nil
REDACTED

	now := time.Now().UTC().Format(time.RFC3339)
	return &soraS3ProfilesStore{
		ActiveProfileID: "default",
		Items: []soraS3ProfileStoreItem{
			{
				ProfileID:                "default",
				Name:                     "Default",
				Enabled:                  legacy.Enabled,
				Endpoint:                 strings.TrimSpace(legacy.Endpoint),
				Region:                   strings.TrimSpace(legacy.Region),
				Bucket:                   strings.TrimSpace(legacy.Bucket),
				AccessKeyID:              strings.TrimSpace(legacy.AccessKeyID),
				SecretAccessKey:          legacy.SecretAccessKey,
				Prefix:                   strings.TrimSpace(legacy.Prefix),
				ForcePathStyle:           legacy.ForcePathStyle,
				CDNURL:                   strings.TrimSpace(legacy.CDNURL),
				DefaultStorageQuotaBytes: maxInt64(legacy.DefaultStorageQuotaBytes, 0),
				UpdatedAt:                now,
		REDACTED,
	REDACTED,
REDACTED, nil
REDACTED

func (s *SettingService) persistSoraS3ProfilesStore(ctx context.Context, store *soraS3ProfilesStore) error {
	if store == nil {
		return fmt.Errorf("sora s3 profiles store cannot be nil")
REDACTED

	normalized := normalizeSoraS3ProfilesStore(*store)
	data, err := json.Marshal(normalized)
	if err != nil {
		return fmt.Errorf("marshal sora s3 profiles: %w", err)
REDACTED

	updates := map[string]string{
		SettingKeySoraS3Profiles: string(data),
REDACTED

	active := pickActiveSoraS3ProfileFromStore(normalized.Items, normalized.ActiveProfileID)
	if active == nil {
		updates[SettingKeySoraS3Enabled] = "false"
		updates[SettingKeySoraS3Endpoint] = ""
		updates[SettingKeySoraS3Region] = ""
		updates[SettingKeySoraS3Bucket] = ""
		updates[SettingKeySoraS3AccessKeyID] = ""
		updates[SettingKeySoraS3Prefix] = ""
		updates[SettingKeySoraS3ForcePathStyle] = "false"
		updates[SettingKeySoraS3CDNURL] = ""
		updates[SettingKeySoraDefaultStorageQuotaBytes] = "0"
		updates[SettingKeySoraS3SecretAccessKey] = ""
REDACTED else {
		updates[SettingKeySoraS3Enabled] = strconv.FormatBool(active.Enabled)
		updates[SettingKeySoraS3Endpoint] = strings.TrimSpace(active.Endpoint)
		updates[SettingKeySoraS3Region] = strings.TrimSpace(active.Region)
		updates[SettingKeySoraS3Bucket] = strings.TrimSpace(active.Bucket)
		updates[SettingKeySoraS3AccessKeyID] = strings.TrimSpace(active.AccessKeyID)
		updates[SettingKeySoraS3Prefix] = strings.TrimSpace(active.Prefix)
		updates[SettingKeySoraS3ForcePathStyle] = strconv.FormatBool(active.ForcePathStyle)
		updates[SettingKeySoraS3CDNURL] = strings.TrimSpace(active.CDNURL)
		updates[SettingKeySoraDefaultStorageQuotaBytes] = strconv.FormatInt(maxInt64(active.DefaultStorageQuotaBytes, 0), 10)
		updates[SettingKeySoraS3SecretAccessKey] = active.SecretAccessKey
REDACTED

	if err := s.settingRepo.SetMultiple(ctx, updates); err != nil {
		return err
REDACTED

	if s.onUpdate != nil {
		s.onUpdate()
REDACTED
	if s.onS3Update != nil {
		s.onS3Update()
REDACTED
	return nil
REDACTED

func (s *SettingService) getLegacySoraS3Settings(ctx context.Context) (*SoraS3Settings, error) {
	keys := []string{
		SettingKeySoraS3Enabled,
		SettingKeySoraS3Endpoint,
		SettingKeySoraS3Region,
		SettingKeySoraS3Bucket,
		SettingKeySoraS3AccessKeyID,
		SettingKeySoraS3SecretAccessKey,
		SettingKeySoraS3Prefix,
		SettingKeySoraS3ForcePathStyle,
		SettingKeySoraS3CDNURL,
		SettingKeySoraDefaultStorageQuotaBytes,
REDACTED

	values, err := s.settingRepo.GetMultiple(ctx, keys)
	if err != nil {
		return nil, fmt.Errorf("get legacy sora s3 settings: %w", err)
REDACTED

	result := &SoraS3Settings{
		Enabled:                   values[SettingKeySoraS3Enabled] == "true",
		Endpoint:                  values[SettingKeySoraS3Endpoint],
		Region:                    values[SettingKeySoraS3Region],
		Bucket:                    values[SettingKeySoraS3Bucket],
		AccessKeyID:               values[SettingKeySoraS3AccessKeyID],
		SecretAccessKey:           values[SettingKeySoraS3SecretAccessKey],
		SecretAccessKeyConfigured: values[SettingKeySoraS3SecretAccessKey] != "",
		Prefix:                    values[SettingKeySoraS3Prefix],
		ForcePathStyle:            values[SettingKeySoraS3ForcePathStyle] == "true",
		CDNURL:                    values[SettingKeySoraS3CDNURL],
REDACTED
	if v, parseErr := strconv.ParseInt(values[SettingKeySoraDefaultStorageQuotaBytes], 10, 64); parseErr == nil {
		result.DefaultStorageQuotaBytes = v
REDACTED
	return result, nil
REDACTED

func normalizeSoraS3ProfilesStore(store soraS3ProfilesStore) soraS3ProfilesStore {
	seen := make(map[string]struct{REDACTED, len(store.Items))
	normalized := soraS3ProfilesStore{
		ActiveProfileID: strings.TrimSpace(store.ActiveProfileID),
		Items:           make([]soraS3ProfileStoreItem, 0, len(store.Items)),
REDACTED
	now := time.Now().UTC().Format(time.RFC3339)

	for idx := range store.Items {
		item := store.Items[idx]
		item.ProfileID = strings.TrimSpace(item.ProfileID)
		if item.ProfileID == "" {
			item.ProfileID = fmt.Sprintf("profile-%d", idx+1)
	REDACTED
		if _, exists := seen[item.ProfileID]; exists {
			continue
	REDACTED
		seen[item.ProfileID] = struct{REDACTED{REDACTED

		item.Name = strings.TrimSpace(item.Name)
		if item.Name == "" {
			item.Name = item.ProfileID
	REDACTED
		item.Endpoint = strings.TrimSpace(item.Endpoint)
		item.Region = strings.TrimSpace(item.Region)
		item.Bucket = strings.TrimSpace(item.Bucket)
		item.AccessKeyID = strings.TrimSpace(item.AccessKeyID)
		item.Prefix = strings.TrimSpace(item.Prefix)
		item.CDNURL = strings.TrimSpace(item.CDNURL)
		item.DefaultStorageQuotaBytes = maxInt64(item.DefaultStorageQuotaBytes, 0)
		item.UpdatedAt = strings.TrimSpace(item.UpdatedAt)
		if item.UpdatedAt == "" {
			item.UpdatedAt = now
	REDACTED
		normalized.Items = append(normalized.Items, item)
REDACTED

	if len(normalized.Items) == 0 {
		normalized.ActiveProfileID = ""
		return normalized
REDACTED

	if findSoraS3ProfileIndex(normalized.Items, normalized.ActiveProfileID) >= 0 {
		return normalized
REDACTED

	normalized.ActiveProfileID = normalized.Items[0].ProfileID
	return normalized
REDACTED

func convertSoraS3ProfilesStore(store *soraS3ProfilesStore) *SoraS3ProfileList {
	if store == nil {
		return &SoraS3ProfileList{REDACTED
REDACTED
	items := make([]SoraS3Profile, 0, len(store.Items))
	for idx := range store.Items {
		item := store.Items[idx]
		items = append(items, SoraS3Profile{
			ProfileID:                 item.ProfileID,
			Name:                      item.Name,
			IsActive:                  item.ProfileID == store.ActiveProfileID,
			Enabled:                   item.Enabled,
			Endpoint:                  item.Endpoint,
			Region:                    item.Region,
			Bucket:                    item.Bucket,
			AccessKeyID:               item.AccessKeyID,
			SecretAccessKey:           item.SecretAccessKey,
			SecretAccessKeyConfigured: item.SecretAccessKey != "",
			Prefix:                    item.Prefix,
			ForcePathStyle:            item.ForcePathStyle,
			CDNURL:                    item.CDNURL,
			DefaultStorageQuotaBytes:  item.DefaultStorageQuotaBytes,
			UpdatedAt:                 item.UpdatedAt,
	REDACTED)
REDACTED
	return &SoraS3ProfileList{
		ActiveProfileID: store.ActiveProfileID,
		Items:           items,
REDACTED
REDACTED

func pickActiveSoraS3Profile(items []SoraS3Profile, activeProfileID string) *SoraS3Profile {
	for idx := range items {
		if items[idx].ProfileID == activeProfileID {
			return &items[idx]
	REDACTED
REDACTED
	if len(items) == 0 {
		return nil
REDACTED
	return &items[0]
REDACTED

func findSoraS3ProfileByID(items []SoraS3Profile, profileID string) *SoraS3Profile {
	for idx := range items {
		if items[idx].ProfileID == profileID {
			return &items[idx]
	REDACTED
REDACTED
	return nil
REDACTED

func pickActiveSoraS3ProfileFromStore(items []soraS3ProfileStoreItem, activeProfileID string) *soraS3ProfileStoreItem {
	for idx := range items {
		if items[idx].ProfileID == activeProfileID {
			return &items[idx]
	REDACTED
REDACTED
	if len(items) == 0 {
		return nil
REDACTED
	return &items[0]
REDACTED

func findSoraS3ProfileIndex(items []soraS3ProfileStoreItem, profileID string) int {
	for idx := range items {
		if items[idx].ProfileID == profileID {
			return idx
	REDACTED
REDACTED
	return -1
REDACTED

func hasSoraS3ProfileID(items []soraS3ProfileStoreItem, profileID string) bool {
	return findSoraS3ProfileIndex(items, profileID) >= 0
REDACTED

func isEmptyLegacySoraS3Settings(settings *SoraS3Settings) bool {
	if settings == nil {
		return true
REDACTED
	if settings.Enabled {
		return false
REDACTED
	if strings.TrimSpace(settings.Endpoint) != "" {
		return false
REDACTED
	if strings.TrimSpace(settings.Region) != "" {
		return false
REDACTED
	if strings.TrimSpace(settings.Bucket) != "" {
		return false
REDACTED
	if strings.TrimSpace(settings.AccessKeyID) != "" {
		return false
REDACTED
	if settings.SecretAccessKey != "" {
		return false
REDACTED
	if strings.TrimSpace(settings.Prefix) != "" {
		return false
REDACTED
	if strings.TrimSpace(settings.CDNURL) != "" {
		return false
REDACTED
	return settings.DefaultStorageQuotaBytes == 0
REDACTED

func maxInt64(value int64, min int64) int64 {
	if value < min {
		return min
REDACTED
	return value
REDACTED
