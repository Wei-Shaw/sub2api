package admin

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"regexp"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/handler/dto"
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/gin-gonic/gin"
)

// semverPattern 预编译 semver 格式校验正则
var semverPattern = regexp.MustCompile(`^\d+\.\d+\.\d+$`)

// menuItemIDPattern validates custom menu item IDs: alphanumeric, hyphens, underscores only.
var menuItemIDPattern = regexp.MustCompile(`^[a-zA-Z0-9_-]+$`)

// generateMenuItemID generates a short random hex ID for a custom menu item.
func generateMenuItemID() (string, error) {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generate menu item ID: %w", err)
REDACTED
	return hex.EncodeToString(b), nil
REDACTED

func scopesContainOpenID(scopes string) bool {
	for _, scope := range strings.Fields(strings.ToLower(strings.TrimSpace(scopes))) {
		if scope == "openid" {
			return true
	REDACTED
REDACTED
	return false
REDACTED

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
	REDACTED
REDACTED
	return ""
REDACTED

// SettingHandler 系统设置处理器
type SettingHandler struct {
	settingService       *service.SettingService
	emailService         *service.EmailService
	turnstileService     *service.TurnstileService
	opsService           *service.OpsService
	paymentConfigService *service.PaymentConfigService
	paymentService       *service.PaymentService
REDACTED

// NewSettingHandler 创建系统设置处理器
func NewSettingHandler(settingService *service.SettingService, emailService *service.EmailService, turnstileService *service.TurnstileService, opsService *service.OpsService, paymentConfigService *service.PaymentConfigService, paymentService *service.PaymentService) *SettingHandler {
	return &SettingHandler{
		settingService:       settingService,
		emailService:         emailService,
		turnstileService:     turnstileService,
		opsService:           opsService,
		paymentConfigService: paymentConfigService,
		paymentService:       paymentService,
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
	authSourceDefaults, err := h.settingService.GetAuthSourceDefaultSettings(c.Request.Context())
	if err != nil {
		response.ErrorFrom(c, err)
		return
REDACTED

	// Check if ops monitoring is enabled (respects config.ops.enabled)
	opsEnabled := h.opsService != nil && h.opsService.IsMonitoringEnabled(c.Request.Context())
	defaultSubscriptions := make([]dto.DefaultSubscriptionSetting, 0, len(settings.DefaultSubscriptions))
	for _, sub := range settings.DefaultSubscriptions {
		defaultSubscriptions = append(defaultSubscriptions, dto.DefaultSubscriptionSetting{
			GroupID:      sub.GroupID,
			ValidityDays: sub.ValidityDays,
	REDACTED)
REDACTED

	// Load payment config
	var paymentCfg *service.PaymentConfig
	if h.paymentConfigService != nil {
		paymentCfg, _ = h.paymentConfigService.GetPaymentConfig(c.Request.Context())
REDACTED
	if paymentCfg == nil {
		paymentCfg = &service.PaymentConfig{REDACTED
REDACTED

	payload := dto.SystemSettings{
		RegistrationEnabled:                    settings.RegistrationEnabled,
		EmailVerifyEnabled:                     settings.EmailVerifyEnabled,
		RegistrationEmailSuffixWhitelist:       settings.RegistrationEmailSuffixWhitelist,
		PromoCodeEnabled:                       settings.PromoCodeEnabled,
		PasswordResetEnabled:                   settings.PasswordResetEnabled,
		FrontendURL:                            settings.FrontendURL,
		InvitationCodeEnabled:                  settings.InvitationCodeEnabled,
		TotpEnabled:                            settings.TotpEnabled,
		TotpEncryptionKeyConfigured:            h.settingService.IsTotpEncryptionKeyConfigured(),
		SMTPHost:                               settings.SMTPHost,
		SMTPPort:                               settings.SMTPPort,
		SMTPUsername:                           settings.SMTPUsername,
		SMTPPasswordConfigured:                 settings.SMTPPasswordConfigured,
		SMTPFrom:                               settings.SMTPFrom,
		SMTPFromName:                           settings.SMTPFromName,
		SMTPUseTLS:                             settings.SMTPUseTLS,
		TurnstileEnabled:                       settings.TurnstileEnabled,
		TurnstileSiteKey:                       settings.TurnstileSiteKey,
		TurnstileSecretKeyConfigured:           settings.TurnstileSecretKeyConfigured,
		LinuxDoConnectEnabled:                  settings.LinuxDoConnectEnabled,
		LinuxDoConnectClientID:                 settings.LinuxDoConnectClientID,
		LinuxDoConnectClientSecretConfigured:   settings.LinuxDoConnectClientSecretConfigured,
		LinuxDoConnectRedirectURL:              settings.LinuxDoConnectRedirectURL,
		WeChatConnectEnabled:                   settings.WeChatConnectEnabled,
		WeChatConnectAppID:                     settings.WeChatConnectAppID,
		WeChatConnectAppSecretConfigured:       settings.WeChatConnectAppSecretConfigured,
		WeChatConnectOpenAppID:                 settings.WeChatConnectOpenAppID,
		WeChatConnectOpenAppSecretConfigured:   settings.WeChatConnectOpenAppSecretConfigured,
		WeChatConnectMPAppID:                   settings.WeChatConnectMPAppID,
		WeChatConnectMPAppSecretConfigured:     settings.WeChatConnectMPAppSecretConfigured,
		WeChatConnectMobileAppID:               settings.WeChatConnectMobileAppID,
		WeChatConnectMobileAppSecretConfigured: settings.WeChatConnectMobileAppSecretConfigured,
		WeChatConnectOpenEnabled:               settings.WeChatConnectOpenEnabled,
		WeChatConnectMPEnabled:                 settings.WeChatConnectMPEnabled,
		WeChatConnectMobileEnabled:             settings.WeChatConnectMobileEnabled,
		WeChatConnectMode:                      settings.WeChatConnectMode,
		WeChatConnectScopes:                    settings.WeChatConnectScopes,
		WeChatConnectRedirectURL:               settings.WeChatConnectRedirectURL,
		WeChatConnectFrontendRedirectURL:       settings.WeChatConnectFrontendRedirectURL,
		OIDCConnectEnabled:                     settings.OIDCConnectEnabled,
		OIDCConnectProviderName:                settings.OIDCConnectProviderName,
		OIDCConnectClientID:                    settings.OIDCConnectClientID,
		OIDCConnectClientSecretConfigured:      settings.OIDCConnectClientSecretConfigured,
		OIDCConnectIssuerURL:                   settings.OIDCConnectIssuerURL,
		OIDCConnectDiscoveryURL:                settings.OIDCConnectDiscoveryURL,
		OIDCConnectAuthorizeURL:                settings.OIDCConnectAuthorizeURL,
		OIDCConnectTokenURL:                    settings.OIDCConnectTokenURL,
		OIDCConnectUserInfoURL:                 settings.OIDCConnectUserInfoURL,
		OIDCConnectJWKSURL:                     settings.OIDCConnectJWKSURL,
		OIDCConnectScopes:                      settings.OIDCConnectScopes,
		OIDCConnectRedirectURL:                 settings.OIDCConnectRedirectURL,
		OIDCConnectFrontendRedirectURL:         settings.OIDCConnectFrontendRedirectURL,
		OIDCConnectTokenAuthMethod:             settings.OIDCConnectTokenAuthMethod,
		OIDCConnectUsePKCE:                     settings.OIDCConnectUsePKCE,
		OIDCConnectValidateIDToken:             settings.OIDCConnectValidateIDToken,
		OIDCConnectAllowedSigningAlgs:          settings.OIDCConnectAllowedSigningAlgs,
		OIDCConnectClockSkewSeconds:            settings.OIDCConnectClockSkewSeconds,
		OIDCConnectRequireEmailVerified:        settings.OIDCConnectRequireEmailVerified,
		OIDCConnectUserInfoEmailPath:           settings.OIDCConnectUserInfoEmailPath,
		OIDCConnectUserInfoIDPath:              settings.OIDCConnectUserInfoIDPath,
		OIDCConnectUserInfoUsernamePath:        settings.OIDCConnectUserInfoUsernamePath,
		SiteName:                               settings.SiteName,
		SiteLogo:                               settings.SiteLogo,
		SiteSubtitle:                           settings.SiteSubtitle,
		APIBaseURL:                             settings.APIBaseURL,
		ContactInfo:                            settings.ContactInfo,
		DocURL:                                 settings.DocURL,
		HomeContent:                            settings.HomeContent,
		HideCcsImportButton:                    settings.HideCcsImportButton,
		PurchaseSubscriptionEnabled:            settings.PurchaseSubscriptionEnabled,
		PurchaseSubscriptionURL:                settings.PurchaseSubscriptionURL,
		TableDefaultPageSize:                   settings.TableDefaultPageSize,
		TablePageSizeOptions:                   settings.TablePageSizeOptions,
		CustomMenuItems:                        dto.ParseCustomMenuItems(settings.CustomMenuItems),
		CustomEndpoints:                        dto.ParseCustomEndpoints(settings.CustomEndpoints),
		DefaultConcurrency:                     settings.DefaultConcurrency,
		DefaultBalance:                         settings.DefaultBalance,
		AffiliateRebateRate:                    settings.AffiliateRebateRate,
		AffiliateRebateFreezeHours:             settings.AffiliateRebateFreezeHours,
		AffiliateRebateDurationDays:            settings.AffiliateRebateDurationDays,
		AffiliateRebatePerInviteeCap:           settings.AffiliateRebatePerInviteeCap,
		DefaultUserRPMLimit:                    settings.DefaultUserRPMLimit,
		DefaultSubscriptions:                   defaultSubscriptions,
		EnableModelFallback:                    settings.EnableModelFallback,
		FallbackModelAnthropic:                 settings.FallbackModelAnthropic,
		FallbackModelOpenAI:                    settings.FallbackModelOpenAI,
		FallbackModelGemini:                    settings.FallbackModelGemini,
		FallbackModelAntigravity:               settings.FallbackModelAntigravity,
		EnableIdentityPatch:                    settings.EnableIdentityPatch,
		IdentityPatchPrompt:                    settings.IdentityPatchPrompt,
		OpsMonitoringEnabled:                   opsEnabled && settings.OpsMonitoringEnabled,
		OpsRealtimeMonitoringEnabled:           settings.OpsRealtimeMonitoringEnabled,
		OpsQueryModeDefault:                    settings.OpsQueryModeDefault,
		OpsMetricsIntervalSeconds:              settings.OpsMetricsIntervalSeconds,
		MinClaudeCodeVersion:                   settings.MinClaudeCodeVersion,
		MaxClaudeCodeVersion:                   settings.MaxClaudeCodeVersion,
		AllowUngroupedKeyScheduling:            settings.AllowUngroupedKeyScheduling,
		BackendModeEnabled:                     settings.BackendModeEnabled,
		EnableFingerprintUnification:           settings.EnableFingerprintUnification,
		EnableMetadataPassthrough:              settings.EnableMetadataPassthrough,
		EnableCCHSigning:                       settings.EnableCCHSigning,
		WebSearchEmulationEnabled:              settings.WebSearchEmulationEnabled,
		PaymentVisibleMethodAlipaySource:       settings.PaymentVisibleMethodAlipaySource,
		PaymentVisibleMethodWxpaySource:        settings.PaymentVisibleMethodWxpaySource,
		PaymentVisibleMethodAlipayEnabled:      settings.PaymentVisibleMethodAlipayEnabled,
		PaymentVisibleMethodWxpayEnabled:       settings.PaymentVisibleMethodWxpayEnabled,
		OpenAIAdvancedSchedulerEnabled:         settings.OpenAIAdvancedSchedulerEnabled,
		BalanceLowNotifyEnabled:                settings.BalanceLowNotifyEnabled,
		BalanceLowNotifyThreshold:              settings.BalanceLowNotifyThreshold,
		BalanceLowNotifyRechargeURL:            settings.BalanceLowNotifyRechargeURL,
		AccountQuotaNotifyEnabled:              settings.AccountQuotaNotifyEnabled,
		AccountQuotaNotifyEmails:               dto.NotifyEmailEntriesFromService(settings.AccountQuotaNotifyEmails),
		PaymentEnabled:                         paymentCfg.Enabled,
		PaymentMinAmount:                       paymentCfg.MinAmount,
		PaymentMaxAmount:                       paymentCfg.MaxAmount,
		PaymentDailyLimit:                      paymentCfg.DailyLimit,
		PaymentOrderTimeoutMin:                 paymentCfg.OrderTimeoutMin,
		PaymentMaxPendingOrders:                paymentCfg.MaxPendingOrders,
		PaymentEnabledTypes:                    paymentCfg.EnabledTypes,
		PaymentBalanceDisabled:                 paymentCfg.BalanceDisabled,
		PaymentBalanceRechargeMultiplier:       paymentCfg.BalanceRechargeMultiplier,
		PaymentRechargeFeeRate:                 paymentCfg.RechargeFeeRate,
		PaymentLoadBalanceStrat:                paymentCfg.LoadBalanceStrategy,
		PaymentProductNamePrefix:               paymentCfg.ProductNamePrefix,
		PaymentProductNameSuffix:               paymentCfg.ProductNameSuffix,
		PaymentHelpImageURL:                    paymentCfg.HelpImageURL,
		PaymentHelpText:                        paymentCfg.HelpText,
		PaymentCancelRateLimitEnabled:          paymentCfg.CancelRateLimitEnabled,
		PaymentCancelRateLimitMax:              paymentCfg.CancelRateLimitMax,
		PaymentCancelRateLimitWindow:           paymentCfg.CancelRateLimitWindow,
		PaymentCancelRateLimitUnit:             paymentCfg.CancelRateLimitUnit,
		PaymentCancelRateLimitMode:             paymentCfg.CancelRateLimitMode,

		ChannelMonitorEnabled:                settings.ChannelMonitorEnabled,
		ChannelMonitorDefaultIntervalSeconds: settings.ChannelMonitorDefaultIntervalSeconds,

		AvailableChannelsEnabled: settings.AvailableChannelsEnabled,

		AffiliateEnabled: settings.AffiliateEnabled,
REDACTED

	// OpenAI fast policy (stored under a dedicated setting key)
	if fastPolicy, err := h.settingService.GetOpenAIFastPolicySettings(c.Request.Context()); err != nil {
		slog.Error("openai_fast_policy_settings_get_failed", "error", err)
REDACTED else if fastPolicy != nil {
		payload.OpenAIFastPolicySettings = openaiFastPolicySettingsToDTO(fastPolicy)
REDACTED

	response.Success(c, systemSettingsResponseData(payload, authSourceDefaults))
REDACTED

// openaiFastPolicySettingsToDTO converts service -> dto for OpenAI fast policy.
func openaiFastPolicySettingsToDTO(s *service.OpenAIFastPolicySettings) *dto.OpenAIFastPolicySettings {
	if s == nil {
		return nil
REDACTED
	rules := make([]dto.OpenAIFastPolicyRule, len(s.Rules))
	for i, r := range s.Rules {
		rules[i] = dto.OpenAIFastPolicyRule(r)
REDACTED
	return &dto.OpenAIFastPolicySettings{Rules: rulesREDACTED
REDACTED

// openaiFastPolicySettingsFromDTO converts dto -> service for OpenAI fast policy.
//
// 规范化 ServiceTier：在 DTO 进入 service 层之前统一把空字符串归一为
// service.OpenAIFastTierAny ("all")，避免管理员保存时空串与 "all" 同时
// 表达"匹配任意 tier"造成数据库取值的二义性。其它非空值原样透传，由
// service.SetOpenAIFastPolicySettings 负责合法值校验。
func openaiFastPolicySettingsFromDTO(s *dto.OpenAIFastPolicySettings) *service.OpenAIFastPolicySettings {
	if s == nil {
		return nil
REDACTED
	rules := make([]service.OpenAIFastPolicyRule, len(s.Rules))
	for i, r := range s.Rules {
		rules[i] = service.OpenAIFastPolicyRule(r)
		tier := strings.ToLower(strings.TrimSpace(rules[i].ServiceTier))
		if tier == "" {
			tier = service.OpenAIFastTierAny
	REDACTED
		rules[i].ServiceTier = tier
REDACTED
	return &service.OpenAIFastPolicySettings{Rules: rulesREDACTED
REDACTED

// UpdateSettingsRequest 更新设置请求
type UpdateSettingsRequest struct {
	// 注册设置
	RegistrationEnabled              bool     `json:"registration_enabled"`
	EmailVerifyEnabled               bool     `json:"email_verify_enabled"`
	RegistrationEmailSuffixWhitelist []string `json:"registration_email_suffix_whitelist"`
	PromoCodeEnabled                 bool     `json:"promo_code_enabled"`
	PasswordResetEnabled             bool     `json:"password_reset_enabled"`
	FrontendURL                      string   `json:"frontend_url"`
	InvitationCodeEnabled            bool     `json:"invitation_code_enabled"`
	TotpEnabled                      bool     `json:"totp_enabled"` // TOTP 双因素认证

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

	// LinuxDo Connect OAuth 登录
	LinuxDoConnectEnabled      bool   `json:"linuxdo_connect_enabled"`
	LinuxDoConnectClientID     string `json:"linuxdo_connect_client_id"`
	LinuxDoConnectClientSecret string `json:"linuxdo_connect_client_secret"`
	LinuxDoConnectRedirectURL  string `json:"linuxdo_connect_redirect_url"`

	// WeChat Connect OAuth 登录
	WeChatConnectEnabled             bool   `json:"wechat_connect_enabled"`
	WeChatConnectAppID               string `json:"wechat_connect_app_id"`
	WeChatConnectAppSecret           string `json:"wechat_connect_app_secret"`
	WeChatConnectOpenAppID           string `json:"wechat_connect_open_app_id"`
	WeChatConnectOpenAppSecret       string `json:"wechat_connect_open_app_secret"`
	WeChatConnectMPAppID             string `json:"wechat_connect_mp_app_id"`
	WeChatConnectMPAppSecret         string `json:"wechat_connect_mp_app_secret"`
	WeChatConnectMobileAppID         string `json:"wechat_connect_mobile_app_id"`
	WeChatConnectMobileAppSecret     string `json:"wechat_connect_mobile_app_secret"`
	WeChatConnectOpenEnabled         bool   `json:"wechat_connect_open_enabled"`
	WeChatConnectMPEnabled           bool   `json:"wechat_connect_mp_enabled"`
	WeChatConnectMobileEnabled       bool   `json:"wechat_connect_mobile_enabled"`
	WeChatConnectMode                string `json:"wechat_connect_mode"`
	WeChatConnectScopes              string `json:"wechat_connect_scopes"`
	WeChatConnectRedirectURL         string `json:"wechat_connect_redirect_url"`
	WeChatConnectFrontendRedirectURL string `json:"wechat_connect_frontend_redirect_url"`

	// Generic OIDC OAuth 登录
	OIDCConnectEnabled              bool   `json:"oidc_connect_enabled"`
	OIDCConnectProviderName         string `json:"oidc_connect_provider_name"`
	OIDCConnectClientID             string `json:"oidc_connect_client_id"`
	OIDCConnectClientSecret         string `json:"oidc_connect_client_secret"`
	OIDCConnectIssuerURL            string `json:"oidc_connect_issuer_url"`
	OIDCConnectDiscoveryURL         string `json:"oidc_connect_discovery_url"`
	OIDCConnectAuthorizeURL         string `json:"oidc_connect_authorize_url"`
	OIDCConnectTokenURL             string `json:"oidc_connect_token_url"`
	OIDCConnectUserInfoURL          string `json:"oidc_connect_userinfo_url"`
	OIDCConnectJWKSURL              string `json:"oidc_connect_jwks_url"`
	OIDCConnectScopes               string `json:"oidc_connect_scopes"`
	OIDCConnectRedirectURL          string `json:"oidc_connect_redirect_url"`
	OIDCConnectFrontendRedirectURL  string `json:"oidc_connect_frontend_redirect_url"`
	OIDCConnectTokenAuthMethod      string `json:"oidc_connect_token_auth_method"`
	OIDCConnectUsePKCE              *bool  `json:"oidc_connect_use_pkce"`
	OIDCConnectValidateIDToken      *bool  `json:"oidc_connect_validate_id_token"`
	OIDCConnectAllowedSigningAlgs   string `json:"oidc_connect_allowed_signing_algs"`
	OIDCConnectClockSkewSeconds     int    `json:"oidc_connect_clock_skew_seconds"`
	OIDCConnectRequireEmailVerified bool   `json:"oidc_connect_require_email_verified"`
	OIDCConnectUserInfoEmailPath    string `json:"oidc_connect_userinfo_email_path"`
	OIDCConnectUserInfoIDPath       string `json:"oidc_connect_userinfo_id_path"`
	OIDCConnectUserInfoUsernamePath string `json:"oidc_connect_userinfo_username_path"`

	// OEM设置
	SiteName                    string                `json:"site_name"`
	SiteLogo                    string                `json:"site_logo"`
	SiteSubtitle                string                `json:"site_subtitle"`
	APIBaseURL                  string                `json:"api_base_url"`
	ContactInfo                 string                `json:"contact_info"`
	DocURL                      string                `json:"doc_url"`
	HomeContent                 string                `json:"home_content"`
	HideCcsImportButton         bool                  `json:"hide_ccs_import_button"`
	PurchaseSubscriptionEnabled *bool                 `json:"purchase_subscription_enabled"`
	PurchaseSubscriptionURL     *string               `json:"purchase_subscription_url"`
	TableDefaultPageSize        int                   `json:"table_default_page_size"`
	TablePageSizeOptions        []int                 `json:"table_page_size_options"`
	CustomMenuItems             *[]dto.CustomMenuItem `json:"custom_menu_items"`
	CustomEndpoints             *[]dto.CustomEndpoint `json:"custom_endpoints"`

	// 默认配置
	DefaultConcurrency                       int                               `json:"default_concurrency"`
	DefaultBalance                           float64                           `json:"default_balance"`
	AffiliateRebateRate                      *float64                          `json:"affiliate_rebate_rate"`
	AffiliateRebateFreezeHours               *int                              `json:"affiliate_rebate_freeze_hours"`
	AffiliateRebateDurationDays              *int                              `json:"affiliate_rebate_duration_days"`
	AffiliateRebatePerInviteeCap             *float64                          `json:"affiliate_rebate_per_invitee_cap"`
	DefaultUserRPMLimit                      int                               `json:"default_user_rpm_limit"`
	DefaultSubscriptions                     []dto.DefaultSubscriptionSetting  `json:"default_subscriptions"`
	AuthSourceDefaultEmailBalance            *float64                          `json:"auth_source_default_email_balance"`
	AuthSourceDefaultEmailConcurrency        *int                              `json:"auth_source_default_email_concurrency"`
	AuthSourceDefaultEmailSubscriptions      *[]dto.DefaultSubscriptionSetting `json:"auth_source_default_email_subscriptions"`
	AuthSourceDefaultEmailGrantOnSignup      *bool                             `json:"auth_source_default_email_grant_on_signup"`
	AuthSourceDefaultEmailGrantOnFirstBind   *bool                             `json:"auth_source_default_email_grant_on_first_bind"`
	AuthSourceDefaultLinuxDoBalance          *float64                          `json:"auth_source_default_linuxdo_balance"`
	AuthSourceDefaultLinuxDoConcurrency      *int                              `json:"auth_source_default_linuxdo_concurrency"`
	AuthSourceDefaultLinuxDoSubscriptions    *[]dto.DefaultSubscriptionSetting `json:"auth_source_default_linuxdo_subscriptions"`
	AuthSourceDefaultLinuxDoGrantOnSignup    *bool                             `json:"auth_source_default_linuxdo_grant_on_signup"`
	AuthSourceDefaultLinuxDoGrantOnFirstBind *bool                             `json:"auth_source_default_linuxdo_grant_on_first_bind"`
	AuthSourceDefaultOIDCBalance             *float64                          `json:"auth_source_default_oidc_balance"`
	AuthSourceDefaultOIDCConcurrency         *int                              `json:"auth_source_default_oidc_concurrency"`
	AuthSourceDefaultOIDCSubscriptions       *[]dto.DefaultSubscriptionSetting `json:"auth_source_default_oidc_subscriptions"`
	AuthSourceDefaultOIDCGrantOnSignup       *bool                             `json:"auth_source_default_oidc_grant_on_signup"`
	AuthSourceDefaultOIDCGrantOnFirstBind    *bool                             `json:"auth_source_default_oidc_grant_on_first_bind"`
	AuthSourceDefaultWeChatBalance           *float64                          `json:"auth_source_default_wechat_balance"`
	AuthSourceDefaultWeChatConcurrency       *int                              `json:"auth_source_default_wechat_concurrency"`
	AuthSourceDefaultWeChatSubscriptions     *[]dto.DefaultSubscriptionSetting `json:"auth_source_default_wechat_subscriptions"`
	AuthSourceDefaultWeChatGrantOnSignup     *bool                             `json:"auth_source_default_wechat_grant_on_signup"`
	AuthSourceDefaultWeChatGrantOnFirstBind  *bool                             `json:"auth_source_default_wechat_grant_on_first_bind"`
	ForceEmailOnThirdPartySignup             *bool                             `json:"force_email_on_third_party_signup"`

	// Model fallback configuration
	EnableModelFallback      bool   `json:"enable_model_fallback"`
	FallbackModelAnthropic   string `json:"fallback_model_anthropic"`
	FallbackModelOpenAI      string `json:"fallback_model_openai"`
	FallbackModelGemini      string `json:"fallback_model_gemini"`
	FallbackModelAntigravity string `json:"fallback_model_antigravity"`

	// Identity patch configuration (Claude -> Gemini)
	EnableIdentityPatch bool   `json:"enable_identity_patch"`
	IdentityPatchPrompt string `json:"identity_patch_prompt"`

	// Ops monitoring (vNext)
	OpsMonitoringEnabled         *bool   `json:"ops_monitoring_enabled"`
	OpsRealtimeMonitoringEnabled *bool   `json:"ops_realtime_monitoring_enabled"`
	OpsQueryModeDefault          *string `json:"ops_query_mode_default"`
	OpsMetricsIntervalSeconds    *int    `json:"ops_metrics_interval_seconds"`

	MinClaudeCodeVersion string `json:"min_claude_code_version"`
	MaxClaudeCodeVersion string `json:"max_claude_code_version"`

	// 分组隔离
	AllowUngroupedKeyScheduling bool `json:"allow_ungrouped_key_scheduling"`

	// Backend Mode
	BackendModeEnabled bool `json:"backend_mode_enabled"`

	// Gateway forwarding behavior
	EnableFingerprintUnification *bool `json:"enable_fingerprint_unification"`
	EnableMetadataPassthrough    *bool `json:"enable_metadata_passthrough"`
	EnableCCHSigning             *bool `json:"enable_cch_signing"`

	// Payment visible method routing
	PaymentVisibleMethodAlipaySource  *string `json:"payment_visible_method_alipay_source"`
	PaymentVisibleMethodWxpaySource   *string `json:"payment_visible_method_wxpay_source"`
	PaymentVisibleMethodAlipayEnabled *bool   `json:"payment_visible_method_alipay_enabled"`
	PaymentVisibleMethodWxpayEnabled  *bool   `json:"payment_visible_method_wxpay_enabled"`

	// OpenAI account scheduling
	OpenAIAdvancedSchedulerEnabled *bool `json:"openai_advanced_scheduler_enabled"`

	// Balance low notification
	BalanceLowNotifyEnabled     *bool                   `json:"balance_low_notify_enabled"`
	BalanceLowNotifyThreshold   *float64                `json:"balance_low_notify_threshold"`
	BalanceLowNotifyRechargeURL *string                 `json:"balance_low_notify_recharge_url"`
	AccountQuotaNotifyEnabled   *bool                   `json:"account_quota_notify_enabled"`
	AccountQuotaNotifyEmails    *[]dto.NotifyEmailEntry `json:"account_quota_notify_emails"`

	// Payment configuration (integrated into settings, full replace)
	PaymentEnabled                   *bool    `json:"payment_enabled"`
	PaymentMinAmount                 *float64 `json:"payment_min_amount"`
	PaymentMaxAmount                 *float64 `json:"payment_max_amount"`
	PaymentDailyLimit                *float64 `json:"payment_daily_limit"`
	PaymentOrderTimeoutMin           *int     `json:"payment_order_timeout_minutes"`
	PaymentMaxPendingOrders          *int     `json:"payment_max_pending_orders"`
	PaymentEnabledTypes              []string `json:"payment_enabled_types"`
	PaymentBalanceDisabled           *bool    `json:"payment_balance_disabled"`
	PaymentBalanceRechargeMultiplier *float64 `json:"payment_balance_recharge_multiplier"`
	PaymentRechargeFeeRate           *float64 `json:"payment_recharge_fee_rate"`
	PaymentLoadBalanceStrat          *string  `json:"payment_load_balance_strategy"`
	PaymentProductNamePrefix         *string  `json:"payment_product_name_prefix"`
	PaymentProductNameSuffix         *string  `json:"payment_product_name_suffix"`
	PaymentHelpImageURL              *string  `json:"payment_help_image_url"`
	PaymentHelpText                  *string  `json:"payment_help_text"`

	// Cancel rate limit
	PaymentCancelRateLimitEnabled *bool   `json:"payment_cancel_rate_limit_enabled"`
	PaymentCancelRateLimitMax     *int    `json:"payment_cancel_rate_limit_max"`
	PaymentCancelRateLimitWindow  *int    `json:"payment_cancel_rate_limit_window"`
	PaymentCancelRateLimitUnit    *string `json:"payment_cancel_rate_limit_unit"`
	PaymentCancelRateLimitMode    *string `json:"payment_cancel_rate_limit_window_mode"`

	// Channel Monitor feature switch
	ChannelMonitorEnabled                *bool `json:"channel_monitor_enabled"`
	ChannelMonitorDefaultIntervalSeconds *int  `json:"channel_monitor_default_interval_seconds"`

	// Available Channels feature switch (user-facing)
	AvailableChannelsEnabled *bool `json:"available_channels_enabled"`

	// Affiliate (邀请返利) feature switch
	AffiliateEnabled *bool `json:"affiliate_enabled"`

	// OpenAI fast/flex policy (optional, only updated when provided)
	OpenAIFastPolicySettings *dto.OpenAIFastPolicySettings `json:"openai_fast_policy_settings,omitempty"`
REDACTED

// UpdateSettings 更新系统设置
// PUT /api/v1/admin/settings
func (h *SettingHandler) UpdateSettings(c *gin.Context) {
	var req UpdateSettingsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
REDACTED

	previousSettings, err := h.settingService.GetAllSettings(c.Request.Context())
	if err != nil {
		response.ErrorFrom(c, err)
		return
REDACTED
	previousAuthSourceDefaults, err := h.settingService.GetAuthSourceDefaultSettings(c.Request.Context())
	if err != nil {
		response.ErrorFrom(c, err)
		return
REDACTED

	// 验证参数
	if req.DefaultConcurrency < 1 {
		req.DefaultConcurrency = 1
REDACTED
	if req.DefaultBalance < 0 {
		req.DefaultBalance = 0
REDACTED
	affiliateRebateRate := previousSettings.AffiliateRebateRate
	if req.AffiliateRebateRate != nil {
		affiliateRebateRate = *req.AffiliateRebateRate
REDACTED
	if affiliateRebateRate < service.AffiliateRebateRateMin {
		affiliateRebateRate = service.AffiliateRebateRateMin
REDACTED
	if affiliateRebateRate > service.AffiliateRebateRateMax {
		affiliateRebateRate = service.AffiliateRebateRateMax
REDACTED
	affiliateRebateFreezeHours := previousSettings.AffiliateRebateFreezeHours
	if req.AffiliateRebateFreezeHours != nil {
		affiliateRebateFreezeHours = *req.AffiliateRebateFreezeHours
REDACTED
	if affiliateRebateFreezeHours < 0 {
		affiliateRebateFreezeHours = service.AffiliateRebateFreezeHoursDefault
REDACTED
	if affiliateRebateFreezeHours > service.AffiliateRebateFreezeHoursMax {
		affiliateRebateFreezeHours = service.AffiliateRebateFreezeHoursMax
REDACTED
	affiliateRebateDurationDays := previousSettings.AffiliateRebateDurationDays
	if req.AffiliateRebateDurationDays != nil {
		affiliateRebateDurationDays = *req.AffiliateRebateDurationDays
REDACTED
	if affiliateRebateDurationDays < 0 {
		affiliateRebateDurationDays = service.AffiliateRebateDurationDaysDefault
REDACTED
	if affiliateRebateDurationDays > service.AffiliateRebateDurationDaysMax {
		affiliateRebateDurationDays = service.AffiliateRebateDurationDaysMax
REDACTED
	affiliateRebatePerInviteeCap := previousSettings.AffiliateRebatePerInviteeCap
	if req.AffiliateRebatePerInviteeCap != nil {
		affiliateRebatePerInviteeCap = *req.AffiliateRebatePerInviteeCap
REDACTED
	if affiliateRebatePerInviteeCap < 0 {
		affiliateRebatePerInviteeCap = service.AffiliateRebatePerInviteeCapDefault
REDACTED
	// 通用表格配置：兼容旧客户端未传字段时保留当前值。
	if req.TableDefaultPageSize <= 0 {
		req.TableDefaultPageSize = previousSettings.TableDefaultPageSize
REDACTED
	if req.TablePageSizeOptions == nil {
		req.TablePageSizeOptions = previousSettings.TablePageSizeOptions
REDACTED
	req.SMTPHost = strings.TrimSpace(req.SMTPHost)
	req.SMTPUsername = strings.TrimSpace(req.SMTPUsername)
	req.SMTPPassword = strings.TrimSpace(req.SMTPPassword)
	req.SMTPFrom = strings.TrimSpace(req.SMTPFrom)
	req.SMTPFromName = strings.TrimSpace(req.SMTPFromName)
	if req.SMTPPort <= 0 {
		req.SMTPPort = 587
REDACTED
	req.DefaultSubscriptions = normalizeDefaultSubscriptions(req.DefaultSubscriptions)
	req.AuthSourceDefaultEmailSubscriptions = normalizeOptionalDefaultSubscriptions(req.AuthSourceDefaultEmailSubscriptions)
	req.AuthSourceDefaultLinuxDoSubscriptions = normalizeOptionalDefaultSubscriptions(req.AuthSourceDefaultLinuxDoSubscriptions)
	req.AuthSourceDefaultOIDCSubscriptions = normalizeOptionalDefaultSubscriptions(req.AuthSourceDefaultOIDCSubscriptions)
	req.AuthSourceDefaultWeChatSubscriptions = normalizeOptionalDefaultSubscriptions(req.AuthSourceDefaultWeChatSubscriptions)

	// SMTP 配置保护：如果请求中 smtp_host 为空但数据库中已有配置，则保留已有 SMTP 配置
	// 防止前端加载设置失败时空表单覆盖已保存的 SMTP 配置
	if req.SMTPHost == "" && previousSettings.SMTPHost != "" {
		req.SMTPHost = previousSettings.SMTPHost
		req.SMTPPort = previousSettings.SMTPPort
		req.SMTPUsername = previousSettings.SMTPUsername
		req.SMTPFrom = previousSettings.SMTPFrom
		req.SMTPFromName = previousSettings.SMTPFromName
		req.SMTPUseTLS = previousSettings.SMTPUseTLS
REDACTED

	// Turnstile 参数验证
	if req.TurnstileEnabled {
		// 检查必填字段
		if req.TurnstileSiteKey == "" {
			response.BadRequest(c, "Turnstile Site Key is required when enabled")
			return
	REDACTED
		// 如果未提供 secret key，使用已保存的值（留空保留当前值）
		if req.TurnstileSecretKey == "" {
			if previousSettings.TurnstileSecretKey == "" {
				response.BadRequest(c, "Turnstile Secret Key is required when enabled")
				return
		REDACTED
			req.TurnstileSecretKey = previousSettings.TurnstileSecretKey
	REDACTED

		// 当 site_key 或 secret_key 任一变化时验证（避免配置错误导致无法登录）
		siteKeyChanged := previousSettings.TurnstileSiteKey != req.TurnstileSiteKey
		secretKeyChanged := previousSettings.TurnstileSecretKey != req.TurnstileSecretKey
		if siteKeyChanged || secretKeyChanged {
			if err := h.turnstileService.ValidateSecretKey(c.Request.Context(), req.TurnstileSecretKey); err != nil {
				response.ErrorFrom(c, err)
				return
		REDACTED
	REDACTED
REDACTED

	// TOTP 双因素认证参数验证
	// 只有手动配置了加密密钥才允许启用 TOTP 功能
	if req.TotpEnabled && !previousSettings.TotpEnabled {
		// 尝试启用 TOTP，检查加密密钥是否已手动配置
		if !h.settingService.IsTotpEncryptionKeyConfigured() {
			response.BadRequest(c, "Cannot enable TOTP: TOTP_ENCRYPTION_KEY environment variable must be configured first. Generate a key with 'openssl rand -hex 32' and set it in your environment.")
			return
	REDACTED
REDACTED

	// LinuxDo Connect 参数验证
	if req.LinuxDoConnectEnabled {
		req.LinuxDoConnectClientID = strings.TrimSpace(req.LinuxDoConnectClientID)
		req.LinuxDoConnectClientSecret = strings.TrimSpace(req.LinuxDoConnectClientSecret)
		req.LinuxDoConnectRedirectURL = strings.TrimSpace(req.LinuxDoConnectRedirectURL)

		if req.LinuxDoConnectClientID == "" {
			response.BadRequest(c, "LinuxDo Client ID is required when enabled")
			return
	REDACTED
		if req.LinuxDoConnectRedirectURL == "" {
			response.BadRequest(c, "LinuxDo Redirect URL is required when enabled")
			return
	REDACTED
		if err := config.ValidateAbsoluteHTTPURL(req.LinuxDoConnectRedirectURL); err != nil {
			response.BadRequest(c, "LinuxDo Redirect URL must be an absolute http(s) URL")
			return
	REDACTED

		// 如果未提供 client_secret，则保留现有值（如有）。
		if req.LinuxDoConnectClientSecret == "" {
			if previousSettings.LinuxDoConnectClientSecret == "" {
				response.BadRequest(c, "LinuxDo Client Secret is required when enabled")
				return
		REDACTED
			req.LinuxDoConnectClientSecret = previousSettings.LinuxDoConnectClientSecret
	REDACTED
REDACTED

	if req.WeChatConnectEnabled {
		req.WeChatConnectAppID = strings.TrimSpace(req.WeChatConnectAppID)
		req.WeChatConnectAppSecret = strings.TrimSpace(req.WeChatConnectAppSecret)
		req.WeChatConnectOpenAppID = strings.TrimSpace(req.WeChatConnectOpenAppID)
		req.WeChatConnectOpenAppSecret = strings.TrimSpace(req.WeChatConnectOpenAppSecret)
		req.WeChatConnectMPAppID = strings.TrimSpace(req.WeChatConnectMPAppID)
		req.WeChatConnectMPAppSecret = strings.TrimSpace(req.WeChatConnectMPAppSecret)
		req.WeChatConnectMobileAppID = strings.TrimSpace(req.WeChatConnectMobileAppID)
		req.WeChatConnectMobileAppSecret = strings.TrimSpace(req.WeChatConnectMobileAppSecret)
		req.WeChatConnectMode = strings.ToLower(strings.TrimSpace(req.WeChatConnectMode))
		req.WeChatConnectScopes = strings.TrimSpace(req.WeChatConnectScopes)
		req.WeChatConnectRedirectURL = strings.TrimSpace(req.WeChatConnectRedirectURL)
		req.WeChatConnectFrontendRedirectURL = strings.TrimSpace(req.WeChatConnectFrontendRedirectURL)
		req.WeChatConnectAppID = strings.TrimSpace(firstNonEmpty(req.WeChatConnectAppID, previousSettings.WeChatConnectAppID))
		req.WeChatConnectRedirectURL = strings.TrimSpace(firstNonEmpty(req.WeChatConnectRedirectURL, previousSettings.WeChatConnectRedirectURL))
		req.WeChatConnectFrontendRedirectURL = strings.TrimSpace(firstNonEmpty(req.WeChatConnectFrontendRedirectURL, previousSettings.WeChatConnectFrontendRedirectURL))
		if req.WeChatConnectMode == "" {
			req.WeChatConnectMode = strings.ToLower(strings.TrimSpace(previousSettings.WeChatConnectMode))
	REDACTED
		if req.WeChatConnectScopes == "" {
			req.WeChatConnectScopes = strings.TrimSpace(previousSettings.WeChatConnectScopes)
	REDACTED

		if req.WeChatConnectMPEnabled && req.WeChatConnectMobileEnabled {
			response.BadRequest(c, "WeChat Official Account and Mobile App cannot be enabled at the same time")
			return
	REDACTED
		if req.WeChatConnectMode != "" {
			switch req.WeChatConnectMode {
			case "open", "mp", "mobile":
			default:
				response.BadRequest(c, "WeChat mode must be open, mp, or mobile")
				return
		REDACTED
	REDACTED
		if !req.WeChatConnectOpenEnabled && !req.WeChatConnectMPEnabled && !req.WeChatConnectMobileEnabled {
			switch req.WeChatConnectMode {
			case "mp":
				req.WeChatConnectMPEnabled = true
			case "mobile":
				req.WeChatConnectMobileEnabled = true
			default:
				req.WeChatConnectOpenEnabled = true
		REDACTED
	REDACTED
		if req.WeChatConnectMode == "" {
			if req.WeChatConnectMPEnabled {
				req.WeChatConnectMode = "mp"
		REDACTED else if req.WeChatConnectMobileEnabled {
				req.WeChatConnectMode = "mobile"
		REDACTED else {
				req.WeChatConnectMode = "open"
		REDACTED
	REDACTED

		req.WeChatConnectOpenAppID = strings.TrimSpace(firstNonEmpty(req.WeChatConnectOpenAppID, req.WeChatConnectAppID, previousSettings.WeChatConnectOpenAppID, previousSettings.WeChatConnectAppID))
		req.WeChatConnectMPAppID = strings.TrimSpace(firstNonEmpty(req.WeChatConnectMPAppID, req.WeChatConnectAppID, previousSettings.WeChatConnectMPAppID, previousSettings.WeChatConnectAppID))
		req.WeChatConnectMobileAppID = strings.TrimSpace(firstNonEmpty(req.WeChatConnectMobileAppID, req.WeChatConnectAppID, previousSettings.WeChatConnectMobileAppID, previousSettings.WeChatConnectAppID))

		if req.WeChatConnectOpenAppSecret == "" {
			req.WeChatConnectOpenAppSecret = strings.TrimSpace(firstNonEmpty(previousSettings.WeChatConnectOpenAppSecret, previousSettings.WeChatConnectAppSecret, req.WeChatConnectAppSecret))
	REDACTED
		if req.WeChatConnectMPAppSecret == "" {
			req.WeChatConnectMPAppSecret = strings.TrimSpace(firstNonEmpty(previousSettings.WeChatConnectMPAppSecret, previousSettings.WeChatConnectAppSecret, req.WeChatConnectAppSecret))
	REDACTED
		if req.WeChatConnectMobileAppSecret == "" {
			req.WeChatConnectMobileAppSecret = strings.TrimSpace(firstNonEmpty(previousSettings.WeChatConnectMobileAppSecret, previousSettings.WeChatConnectAppSecret, req.WeChatConnectAppSecret))
	REDACTED
		if req.WeChatConnectAppSecret == "" {
			req.WeChatConnectAppSecret = strings.TrimSpace(firstNonEmpty(req.WeChatConnectOpenAppSecret, req.WeChatConnectMPAppSecret, req.WeChatConnectMobileAppSecret, previousSettings.WeChatConnectAppSecret))
	REDACTED

		if req.WeChatConnectOpenEnabled {
			if req.WeChatConnectOpenAppID == "" {
				response.BadRequest(c, "WeChat PC App ID is required when enabled")
				return
		REDACTED
			if req.WeChatConnectOpenAppSecret == "" {
				response.BadRequest(c, "WeChat PC App Secret is required when enabled")
				return
		REDACTED
	REDACTED
		if req.WeChatConnectMPEnabled {
			if req.WeChatConnectMPAppID == "" {
				response.BadRequest(c, "WeChat Official Account App ID is required when enabled")
				return
		REDACTED
			if req.WeChatConnectMPAppSecret == "" {
				response.BadRequest(c, "WeChat Official Account App Secret is required when enabled")
				return
		REDACTED
	REDACTED
		if req.WeChatConnectMobileEnabled {
			if req.WeChatConnectMobileAppID == "" {
				response.BadRequest(c, "WeChat Mobile App ID is required when enabled")
				return
		REDACTED
			if req.WeChatConnectMobileAppSecret == "" {
				response.BadRequest(c, "WeChat Mobile App Secret is required when enabled")
				return
		REDACTED
	REDACTED

		if req.WeChatConnectScopes == "" {
			if req.WeChatConnectMPEnabled {
				req.WeChatConnectScopes = service.DefaultWeChatConnectScopesForMode("mp")
		REDACTED else {
				req.WeChatConnectScopes = service.DefaultWeChatConnectScopesForMode(req.WeChatConnectMode)
		REDACTED
	REDACTED
		if req.WeChatConnectOpenEnabled || req.WeChatConnectMPEnabled {
			if req.WeChatConnectRedirectURL == "" {
				response.BadRequest(c, "WeChat Redirect URL is required when web oauth is enabled")
				return
		REDACTED
			if err := config.ValidateAbsoluteHTTPURL(req.WeChatConnectRedirectURL); err != nil {
				response.BadRequest(c, "WeChat Redirect URL must be an absolute http(s) URL")
				return
		REDACTED
			if req.WeChatConnectFrontendRedirectURL == "" {
				req.WeChatConnectFrontendRedirectURL = "/auth/wechat/callback"
		REDACTED
			if err := config.ValidateFrontendRedirectURL(req.WeChatConnectFrontendRedirectURL); err != nil {
				response.BadRequest(c, "WeChat Frontend Redirect URL is invalid")
				return
		REDACTED
	REDACTED
REDACTED

	// Generic OIDC 参数验证
	oidcUsePKCE, oidcValidateIDToken, err := h.settingService.OIDCSecurityWriteDefaults(c.Request.Context())
	if err != nil {
		response.ErrorFrom(c, err)
		return
REDACTED
	if req.OIDCConnectEnabled {
		req.OIDCConnectProviderName = strings.TrimSpace(req.OIDCConnectProviderName)
		req.OIDCConnectClientID = strings.TrimSpace(req.OIDCConnectClientID)
		req.OIDCConnectClientSecret = strings.TrimSpace(req.OIDCConnectClientSecret)
		req.OIDCConnectIssuerURL = strings.TrimSpace(req.OIDCConnectIssuerURL)
		req.OIDCConnectDiscoveryURL = strings.TrimSpace(req.OIDCConnectDiscoveryURL)
		req.OIDCConnectAuthorizeURL = strings.TrimSpace(req.OIDCConnectAuthorizeURL)
		req.OIDCConnectTokenURL = strings.TrimSpace(req.OIDCConnectTokenURL)
		req.OIDCConnectUserInfoURL = strings.TrimSpace(req.OIDCConnectUserInfoURL)
		req.OIDCConnectJWKSURL = strings.TrimSpace(req.OIDCConnectJWKSURL)
		req.OIDCConnectScopes = strings.TrimSpace(req.OIDCConnectScopes)
		req.OIDCConnectRedirectURL = strings.TrimSpace(req.OIDCConnectRedirectURL)
		req.OIDCConnectFrontendRedirectURL = strings.TrimSpace(req.OIDCConnectFrontendRedirectURL)
		req.OIDCConnectTokenAuthMethod = strings.ToLower(strings.TrimSpace(req.OIDCConnectTokenAuthMethod))
		req.OIDCConnectAllowedSigningAlgs = strings.TrimSpace(req.OIDCConnectAllowedSigningAlgs)
		req.OIDCConnectUserInfoEmailPath = strings.TrimSpace(req.OIDCConnectUserInfoEmailPath)
		req.OIDCConnectUserInfoIDPath = strings.TrimSpace(req.OIDCConnectUserInfoIDPath)
		req.OIDCConnectUserInfoUsernamePath = strings.TrimSpace(req.OIDCConnectUserInfoUsernamePath)
		req.OIDCConnectProviderName = strings.TrimSpace(firstNonEmpty(req.OIDCConnectProviderName, previousSettings.OIDCConnectProviderName, "OIDC"))
		req.OIDCConnectClientID = strings.TrimSpace(firstNonEmpty(req.OIDCConnectClientID, previousSettings.OIDCConnectClientID))
		req.OIDCConnectIssuerURL = strings.TrimSpace(firstNonEmpty(req.OIDCConnectIssuerURL, previousSettings.OIDCConnectIssuerURL))
		req.OIDCConnectDiscoveryURL = strings.TrimSpace(firstNonEmpty(req.OIDCConnectDiscoveryURL, previousSettings.OIDCConnectDiscoveryURL))
		req.OIDCConnectAuthorizeURL = strings.TrimSpace(firstNonEmpty(req.OIDCConnectAuthorizeURL, previousSettings.OIDCConnectAuthorizeURL))
		req.OIDCConnectTokenURL = strings.TrimSpace(firstNonEmpty(req.OIDCConnectTokenURL, previousSettings.OIDCConnectTokenURL))
		req.OIDCConnectUserInfoURL = strings.TrimSpace(firstNonEmpty(req.OIDCConnectUserInfoURL, previousSettings.OIDCConnectUserInfoURL))
		req.OIDCConnectJWKSURL = strings.TrimSpace(firstNonEmpty(req.OIDCConnectJWKSURL, previousSettings.OIDCConnectJWKSURL))
		req.OIDCConnectScopes = strings.TrimSpace(firstNonEmpty(req.OIDCConnectScopes, previousSettings.OIDCConnectScopes, "openid email profile"))
		req.OIDCConnectRedirectURL = strings.TrimSpace(firstNonEmpty(req.OIDCConnectRedirectURL, previousSettings.OIDCConnectRedirectURL))
		req.OIDCConnectFrontendRedirectURL = strings.TrimSpace(firstNonEmpty(req.OIDCConnectFrontendRedirectURL, previousSettings.OIDCConnectFrontendRedirectURL, "/auth/oidc/callback"))
		req.OIDCConnectTokenAuthMethod = strings.ToLower(strings.TrimSpace(firstNonEmpty(req.OIDCConnectTokenAuthMethod, previousSettings.OIDCConnectTokenAuthMethod, "client_secret_post")))
		req.OIDCConnectAllowedSigningAlgs = strings.TrimSpace(firstNonEmpty(req.OIDCConnectAllowedSigningAlgs, previousSettings.OIDCConnectAllowedSigningAlgs, "RS256,ES256,PS256"))
		req.OIDCConnectUserInfoEmailPath = strings.TrimSpace(firstNonEmpty(req.OIDCConnectUserInfoEmailPath, previousSettings.OIDCConnectUserInfoEmailPath))
		req.OIDCConnectUserInfoIDPath = strings.TrimSpace(firstNonEmpty(req.OIDCConnectUserInfoIDPath, previousSettings.OIDCConnectUserInfoIDPath))
		req.OIDCConnectUserInfoUsernamePath = strings.TrimSpace(firstNonEmpty(req.OIDCConnectUserInfoUsernamePath, previousSettings.OIDCConnectUserInfoUsernamePath))
		if req.OIDCConnectUsePKCE != nil {
			oidcUsePKCE = *req.OIDCConnectUsePKCE
	REDACTED
		if req.OIDCConnectValidateIDToken != nil {
			oidcValidateIDToken = *req.OIDCConnectValidateIDToken
	REDACTED
		if req.OIDCConnectClockSkewSeconds == 0 {
			req.OIDCConnectClockSkewSeconds = previousSettings.OIDCConnectClockSkewSeconds
			if req.OIDCConnectClockSkewSeconds == 0 {
				req.OIDCConnectClockSkewSeconds = 120
		REDACTED
	REDACTED

		if req.OIDCConnectClientID == "" {
			response.BadRequest(c, "OIDC Client ID is required when enabled")
			return
	REDACTED
		if req.OIDCConnectIssuerURL == "" {
			response.BadRequest(c, "OIDC Issuer URL is required when enabled")
			return
	REDACTED
		if err := config.ValidateAbsoluteHTTPURL(req.OIDCConnectIssuerURL); err != nil {
			response.BadRequest(c, "OIDC Issuer URL must be an absolute http(s) URL")
			return
	REDACTED
		if req.OIDCConnectDiscoveryURL != "" {
			if err := config.ValidateAbsoluteHTTPURL(req.OIDCConnectDiscoveryURL); err != nil {
				response.BadRequest(c, "OIDC Discovery URL must be an absolute http(s) URL")
				return
		REDACTED
	REDACTED
		if req.OIDCConnectAuthorizeURL != "" {
			if err := config.ValidateAbsoluteHTTPURL(req.OIDCConnectAuthorizeURL); err != nil {
				response.BadRequest(c, "OIDC Authorize URL must be an absolute http(s) URL")
				return
		REDACTED
	REDACTED
		if req.OIDCConnectTokenURL != "" {
			if err := config.ValidateAbsoluteHTTPURL(req.OIDCConnectTokenURL); err != nil {
				response.BadRequest(c, "OIDC Token URL must be an absolute http(s) URL")
				return
		REDACTED
	REDACTED
		if req.OIDCConnectUserInfoURL != "" {
			if err := config.ValidateAbsoluteHTTPURL(req.OIDCConnectUserInfoURL); err != nil {
				response.BadRequest(c, "OIDC UserInfo URL must be an absolute http(s) URL")
				return
		REDACTED
	REDACTED
		if req.OIDCConnectRedirectURL == "" {
			response.BadRequest(c, "OIDC Redirect URL is required when enabled")
			return
	REDACTED
		if err := config.ValidateAbsoluteHTTPURL(req.OIDCConnectRedirectURL); err != nil {
			response.BadRequest(c, "OIDC Redirect URL must be an absolute http(s) URL")
			return
	REDACTED
		if req.OIDCConnectFrontendRedirectURL == "" {
			response.BadRequest(c, "OIDC Frontend Redirect URL is required when enabled")
			return
	REDACTED
		if err := config.ValidateFrontendRedirectURL(req.OIDCConnectFrontendRedirectURL); err != nil {
			response.BadRequest(c, "OIDC Frontend Redirect URL is invalid")
			return
	REDACTED
		if !scopesContainOpenID(req.OIDCConnectScopes) {
			response.BadRequest(c, "OIDC scopes must contain openid")
			return
	REDACTED
		switch req.OIDCConnectTokenAuthMethod {
		case "", "client_secret_post", "client_secret_basic", "none":
		default:
			response.BadRequest(c, "OIDC Token Auth Method must be one of client_secret_post/client_secret_basic/none")
			return
	REDACTED
		if req.OIDCConnectClockSkewSeconds < 0 || req.OIDCConnectClockSkewSeconds > 600 {
			response.BadRequest(c, "OIDC clock skew seconds must be between 0 and 600")
			return
	REDACTED
		if oidcValidateIDToken && req.OIDCConnectAllowedSigningAlgs == "" {
			response.BadRequest(c, "OIDC Allowed Signing Algs is required when validate_id_token=true")
			return
	REDACTED
		if req.OIDCConnectJWKSURL != "" {
			if err := config.ValidateAbsoluteHTTPURL(req.OIDCConnectJWKSURL); err != nil {
				response.BadRequest(c, "OIDC JWKS URL must be an absolute http(s) URL")
				return
		REDACTED
	REDACTED
		if req.OIDCConnectTokenAuthMethod == "" || req.OIDCConnectTokenAuthMethod == "client_secret_post" || req.OIDCConnectTokenAuthMethod == "client_secret_basic" {
			if req.OIDCConnectClientSecret == "" {
				if previousSettings.OIDCConnectClientSecret == "" {
					response.BadRequest(c, "OIDC Client Secret is required when enabled")
					return
			REDACTED
				req.OIDCConnectClientSecret = previousSettings.OIDCConnectClientSecret
		REDACTED
	REDACTED
REDACTED

	// “购买订阅”页面配置验证
	purchaseEnabled := previousSettings.PurchaseSubscriptionEnabled
	if req.PurchaseSubscriptionEnabled != nil {
		purchaseEnabled = *req.PurchaseSubscriptionEnabled
REDACTED
	purchaseURL := previousSettings.PurchaseSubscriptionURL
	if req.PurchaseSubscriptionURL != nil {
		purchaseURL = strings.TrimSpace(*req.PurchaseSubscriptionURL)
REDACTED

	// - 启用时要求 URL 合法且非空
	// - 禁用时允许为空；若提供了 URL 也做基本校验，避免误配置
	if purchaseEnabled {
		if purchaseURL == "" {
			response.BadRequest(c, "Purchase Subscription URL is required when enabled")
			return
	REDACTED
		if err := config.ValidateAbsoluteHTTPURL(purchaseURL); err != nil {
			response.BadRequest(c, "Purchase Subscription URL must be an absolute http(s) URL")
			return
	REDACTED
REDACTED else if purchaseURL != "" {
		if err := config.ValidateAbsoluteHTTPURL(purchaseURL); err != nil {
			response.BadRequest(c, "Purchase Subscription URL must be an absolute http(s) URL")
			return
	REDACTED
REDACTED

	// Frontend URL 验证
	req.FrontendURL = strings.TrimSpace(req.FrontendURL)
	if req.FrontendURL != "" {
		if err := config.ValidateAbsoluteHTTPURL(req.FrontendURL); err != nil {
			response.BadRequest(c, "Frontend URL must be an absolute http(s) URL")
			return
	REDACTED
REDACTED

	// 自定义菜单项验证
	const (
		maxCustomMenuItems    = 20
		maxMenuItemLabelLen   = 50
		maxMenuItemURLLen     = 2048
		maxMenuItemIconSVGLen = 10 * 1024 // 10KB
		maxMenuItemIDLen      = 32
	)

	customMenuJSON := previousSettings.CustomMenuItems
	if req.CustomMenuItems != nil {
		items := *req.CustomMenuItems
		if len(items) > maxCustomMenuItems {
			response.BadRequest(c, "Too many custom menu items (max 20)")
			return
	REDACTED
		for i, item := range items {
			if strings.TrimSpace(item.Label) == "" {
				response.BadRequest(c, "Custom menu item label is required")
				return
		REDACTED
			if len(item.Label) > maxMenuItemLabelLen {
				response.BadRequest(c, "Custom menu item label is too long (max 50 characters)")
				return
		REDACTED
			if strings.TrimSpace(item.URL) == "" {
				response.BadRequest(c, "Custom menu item URL is required")
				return
		REDACTED
			if len(item.URL) > maxMenuItemURLLen {
				response.BadRequest(c, "Custom menu item URL is too long (max 2048 characters)")
				return
		REDACTED
			if err := config.ValidateAbsoluteHTTPURL(strings.TrimSpace(item.URL)); err != nil {
				response.BadRequest(c, "Custom menu item URL must be an absolute http(s) URL")
				return
		REDACTED
			if item.Visibility != "user" && item.Visibility != "admin" {
				response.BadRequest(c, "Custom menu item visibility must be 'user' or 'admin'")
				return
		REDACTED
			if len(item.IconSVG) > maxMenuItemIconSVGLen {
				response.BadRequest(c, "Custom menu item icon SVG is too large (max 10KB)")
				return
		REDACTED
			// Auto-generate ID if missing
			if strings.TrimSpace(item.ID) == "" {
				id, err := generateMenuItemID()
				if err != nil {
					response.Error(c, http.StatusInternalServerError, "Failed to generate menu item ID")
					return
			REDACTED
				items[i].ID = id
		REDACTED else if len(item.ID) > maxMenuItemIDLen {
				response.BadRequest(c, "Custom menu item ID is too long (max 32 characters)")
				return
		REDACTED else if !menuItemIDPattern.MatchString(item.ID) {
				response.BadRequest(c, "Custom menu item ID contains invalid characters (only a-z, A-Z, 0-9, - and _ are allowed)")
				return
		REDACTED
	REDACTED
		// ID uniqueness check
		seen := make(map[string]struct{REDACTED, len(items))
		for _, item := range items {
			if _, exists := seen[item.ID]; exists {
				response.BadRequest(c, "Duplicate custom menu item ID: "+item.ID)
				return
		REDACTED
			seen[item.ID] = struct{REDACTED{REDACTED
	REDACTED
		menuBytes, err := json.Marshal(items)
		if err != nil {
			response.BadRequest(c, "Failed to serialize custom menu items")
			return
	REDACTED
		customMenuJSON = string(menuBytes)
REDACTED

	// 自定义端点验证
	const (
		maxCustomEndpoints        = 10
		maxEndpointNameLen        = 50
		maxEndpointURLLen         = 2048
		maxEndpointDescriptionLen = 200
	)

	customEndpointsJSON := previousSettings.CustomEndpoints
	if req.CustomEndpoints != nil {
		endpoints := *req.CustomEndpoints
		if len(endpoints) > maxCustomEndpoints {
			response.BadRequest(c, "Too many custom endpoints (max 10)")
			return
	REDACTED
		for _, ep := range endpoints {
			if strings.TrimSpace(ep.Name) == "" {
				response.BadRequest(c, "Custom endpoint name is required")
				return
		REDACTED
			if len(ep.Name) > maxEndpointNameLen {
				response.BadRequest(c, "Custom endpoint name is too long (max 50 characters)")
				return
		REDACTED
			if strings.TrimSpace(ep.Endpoint) == "" {
				response.BadRequest(c, "Custom endpoint URL is required")
				return
		REDACTED
			if len(ep.Endpoint) > maxEndpointURLLen {
				response.BadRequest(c, "Custom endpoint URL is too long (max 2048 characters)")
				return
		REDACTED
			if err := config.ValidateAbsoluteHTTPURL(strings.TrimSpace(ep.Endpoint)); err != nil {
				response.BadRequest(c, "Custom endpoint URL must be an absolute http(s) URL")
				return
		REDACTED
			if len(ep.Description) > maxEndpointDescriptionLen {
				response.BadRequest(c, "Custom endpoint description is too long (max 200 characters)")
				return
		REDACTED
	REDACTED
		endpointBytes, err := json.Marshal(endpoints)
		if err != nil {
			response.BadRequest(c, "Failed to serialize custom endpoints")
			return
	REDACTED
		customEndpointsJSON = string(endpointBytes)
REDACTED

	// Ops metrics collector interval validation (seconds).
	if req.OpsMetricsIntervalSeconds != nil {
		v := *req.OpsMetricsIntervalSeconds
		if v < 60 {
			v = 60
	REDACTED
		if v > 3600 {
			v = 3600
	REDACTED
		req.OpsMetricsIntervalSeconds = &v
REDACTED
	defaultSubscriptions := make([]service.DefaultSubscriptionSetting, 0, len(req.DefaultSubscriptions))
	for _, sub := range req.DefaultSubscriptions {
		defaultSubscriptions = append(defaultSubscriptions, service.DefaultSubscriptionSetting{
			GroupID:      sub.GroupID,
			ValidityDays: sub.ValidityDays,
	REDACTED)
REDACTED

	// 验证最低版本号格式（空字符串=禁用，或合法 semver）
	if req.MinClaudeCodeVersion != "" {
		if !semverPattern.MatchString(req.MinClaudeCodeVersion) {
			response.Error(c, http.StatusBadRequest, "min_claude_code_version must be empty or a valid semver (e.g. 2.1.63)")
			return
	REDACTED
REDACTED

	// 验证最高版本号格式（空字符串=禁用，或合法 semver）
	if req.MaxClaudeCodeVersion != "" {
		if !semverPattern.MatchString(req.MaxClaudeCodeVersion) {
			response.Error(c, http.StatusBadRequest, "max_claude_code_version must be empty or a valid semver (e.g. 3.0.0)")
			return
	REDACTED
REDACTED

	// 交叉验证：如果同时设置了最低和最高版本号，最高版本号必须 >= 最低版本号
	if req.MinClaudeCodeVersion != "" && req.MaxClaudeCodeVersion != "" {
		if service.CompareVersions(req.MaxClaudeCodeVersion, req.MinClaudeCodeVersion) < 0 {
			response.Error(c, http.StatusBadRequest, "max_claude_code_version must be greater than or equal to min_claude_code_version")
			return
	REDACTED
REDACTED

	settings := &service.SystemSettings{
		RegistrationEnabled:              req.RegistrationEnabled,
		EmailVerifyEnabled:               req.EmailVerifyEnabled,
		RegistrationEmailSuffixWhitelist: req.RegistrationEmailSuffixWhitelist,
		PromoCodeEnabled:                 req.PromoCodeEnabled,
		PasswordResetEnabled:             req.PasswordResetEnabled,
		FrontendURL:                      req.FrontendURL,
		InvitationCodeEnabled:            req.InvitationCodeEnabled,
		TotpEnabled:                      req.TotpEnabled,
		SMTPHost:                         req.SMTPHost,
		SMTPPort:                         req.SMTPPort,
		SMTPUsername:                     req.SMTPUsername,
		SMTPPassword:                     req.SMTPPassword,
		SMTPFrom:                         req.SMTPFrom,
		SMTPFromName:                     req.SMTPFromName,
		SMTPUseTLS:                       req.SMTPUseTLS,
		TurnstileEnabled:                 req.TurnstileEnabled,
		TurnstileSiteKey:                 req.TurnstileSiteKey,
		TurnstileSecretKey:               req.TurnstileSecretKey,
		LinuxDoConnectEnabled:            req.LinuxDoConnectEnabled,
		LinuxDoConnectClientID:           req.LinuxDoConnectClientID,
		LinuxDoConnectClientSecret:       req.LinuxDoConnectClientSecret,
		LinuxDoConnectRedirectURL:        req.LinuxDoConnectRedirectURL,
		WeChatConnectEnabled:             req.WeChatConnectEnabled,
		WeChatConnectAppID:               req.WeChatConnectAppID,
		WeChatConnectAppSecret:           req.WeChatConnectAppSecret,
		WeChatConnectOpenAppID:           req.WeChatConnectOpenAppID,
		WeChatConnectOpenAppSecret:       req.WeChatConnectOpenAppSecret,
		WeChatConnectMPAppID:             req.WeChatConnectMPAppID,
		WeChatConnectMPAppSecret:         req.WeChatConnectMPAppSecret,
		WeChatConnectMobileAppID:         req.WeChatConnectMobileAppID,
		WeChatConnectMobileAppSecret:     req.WeChatConnectMobileAppSecret,
		WeChatConnectOpenEnabled:         req.WeChatConnectOpenEnabled,
		WeChatConnectMPEnabled:           req.WeChatConnectMPEnabled,
		WeChatConnectMobileEnabled:       req.WeChatConnectMobileEnabled,
		WeChatConnectMode:                req.WeChatConnectMode,
		WeChatConnectScopes:              req.WeChatConnectScopes,
		WeChatConnectRedirectURL:         req.WeChatConnectRedirectURL,
		WeChatConnectFrontendRedirectURL: req.WeChatConnectFrontendRedirectURL,
		OIDCConnectEnabled:               req.OIDCConnectEnabled,
		OIDCConnectProviderName:          req.OIDCConnectProviderName,
		OIDCConnectClientID:              req.OIDCConnectClientID,
		OIDCConnectClientSecret:          req.OIDCConnectClientSecret,
		OIDCConnectIssuerURL:             req.OIDCConnectIssuerURL,
		OIDCConnectDiscoveryURL:          req.OIDCConnectDiscoveryURL,
		OIDCConnectAuthorizeURL:          req.OIDCConnectAuthorizeURL,
		OIDCConnectTokenURL:              req.OIDCConnectTokenURL,
		OIDCConnectUserInfoURL:           req.OIDCConnectUserInfoURL,
		OIDCConnectJWKSURL:               req.OIDCConnectJWKSURL,
		OIDCConnectScopes:                req.OIDCConnectScopes,
		OIDCConnectRedirectURL:           req.OIDCConnectRedirectURL,
		OIDCConnectFrontendRedirectURL:   req.OIDCConnectFrontendRedirectURL,
		OIDCConnectTokenAuthMethod:       req.OIDCConnectTokenAuthMethod,
		OIDCConnectUsePKCE:               oidcUsePKCE,
		OIDCConnectValidateIDToken:       oidcValidateIDToken,
		OIDCConnectAllowedSigningAlgs:    req.OIDCConnectAllowedSigningAlgs,
		OIDCConnectClockSkewSeconds:      req.OIDCConnectClockSkewSeconds,
		OIDCConnectRequireEmailVerified:  req.OIDCConnectRequireEmailVerified,
		OIDCConnectUserInfoEmailPath:     req.OIDCConnectUserInfoEmailPath,
		OIDCConnectUserInfoIDPath:        req.OIDCConnectUserInfoIDPath,
		OIDCConnectUserInfoUsernamePath:  req.OIDCConnectUserInfoUsernamePath,
		SiteName:                         req.SiteName,
		SiteLogo:                         req.SiteLogo,
		SiteSubtitle:                     req.SiteSubtitle,
		APIBaseURL:                       req.APIBaseURL,
		ContactInfo:                      req.ContactInfo,
		DocURL:                           req.DocURL,
		HomeContent:                      req.HomeContent,
		HideCcsImportButton:              req.HideCcsImportButton,
		PurchaseSubscriptionEnabled:      purchaseEnabled,
		PurchaseSubscriptionURL:          purchaseURL,
		TableDefaultPageSize:             req.TableDefaultPageSize,
		TablePageSizeOptions:             req.TablePageSizeOptions,
		CustomMenuItems:                  customMenuJSON,
		CustomEndpoints:                  customEndpointsJSON,
		DefaultConcurrency:               req.DefaultConcurrency,
		DefaultBalance:                   req.DefaultBalance,
		AffiliateRebateRate:              affiliateRebateRate,
		AffiliateRebateFreezeHours:       affiliateRebateFreezeHours,
		AffiliateRebateDurationDays:      affiliateRebateDurationDays,
		AffiliateRebatePerInviteeCap:     affiliateRebatePerInviteeCap,
		DefaultUserRPMLimit:              req.DefaultUserRPMLimit,
		DefaultSubscriptions:             defaultSubscriptions,
		EnableModelFallback:              req.EnableModelFallback,
		FallbackModelAnthropic:           req.FallbackModelAnthropic,
		FallbackModelOpenAI:              req.FallbackModelOpenAI,
		FallbackModelGemini:              req.FallbackModelGemini,
		FallbackModelAntigravity:         req.FallbackModelAntigravity,
		EnableIdentityPatch:              req.EnableIdentityPatch,
		IdentityPatchPrompt:              req.IdentityPatchPrompt,
		MinClaudeCodeVersion:             req.MinClaudeCodeVersion,
		MaxClaudeCodeVersion:             req.MaxClaudeCodeVersion,
		AllowUngroupedKeyScheduling:      req.AllowUngroupedKeyScheduling,
		BackendModeEnabled:               req.BackendModeEnabled,
		OpsMonitoringEnabled: func() bool {
			if req.OpsMonitoringEnabled != nil {
				return *req.OpsMonitoringEnabled
		REDACTED
			return previousSettings.OpsMonitoringEnabled
	REDACTED(),
		OpsRealtimeMonitoringEnabled: func() bool {
			if req.OpsRealtimeMonitoringEnabled != nil {
				return *req.OpsRealtimeMonitoringEnabled
		REDACTED
			return previousSettings.OpsRealtimeMonitoringEnabled
	REDACTED(),
		OpsQueryModeDefault: func() string {
			if req.OpsQueryModeDefault != nil {
				return *req.OpsQueryModeDefault
		REDACTED
			return previousSettings.OpsQueryModeDefault
	REDACTED(),
		OpsMetricsIntervalSeconds: func() int {
			if req.OpsMetricsIntervalSeconds != nil {
				return *req.OpsMetricsIntervalSeconds
		REDACTED
			return previousSettings.OpsMetricsIntervalSeconds
	REDACTED(),
		EnableFingerprintUnification: func() bool {
			if req.EnableFingerprintUnification != nil {
				return *req.EnableFingerprintUnification
		REDACTED
			return previousSettings.EnableFingerprintUnification
	REDACTED(),
		EnableMetadataPassthrough: func() bool {
			if req.EnableMetadataPassthrough != nil {
				return *req.EnableMetadataPassthrough
		REDACTED
			return previousSettings.EnableMetadataPassthrough
	REDACTED(),
		EnableCCHSigning: func() bool {
			if req.EnableCCHSigning != nil {
				return *req.EnableCCHSigning
		REDACTED
			return previousSettings.EnableCCHSigning
	REDACTED(),
		PaymentVisibleMethodAlipaySource: func() string {
			if req.PaymentVisibleMethodAlipaySource != nil {
				return strings.TrimSpace(*req.PaymentVisibleMethodAlipaySource)
		REDACTED
			return previousSettings.PaymentVisibleMethodAlipaySource
	REDACTED(),
		PaymentVisibleMethodWxpaySource: func() string {
			if req.PaymentVisibleMethodWxpaySource != nil {
				return strings.TrimSpace(*req.PaymentVisibleMethodWxpaySource)
		REDACTED
			return previousSettings.PaymentVisibleMethodWxpaySource
	REDACTED(),
		PaymentVisibleMethodAlipayEnabled: func() bool {
			if req.PaymentVisibleMethodAlipayEnabled != nil {
				return *req.PaymentVisibleMethodAlipayEnabled
		REDACTED
			return previousSettings.PaymentVisibleMethodAlipayEnabled
	REDACTED(),
		PaymentVisibleMethodWxpayEnabled: func() bool {
			if req.PaymentVisibleMethodWxpayEnabled != nil {
				return *req.PaymentVisibleMethodWxpayEnabled
		REDACTED
			return previousSettings.PaymentVisibleMethodWxpayEnabled
	REDACTED(),
		OpenAIAdvancedSchedulerEnabled: func() bool {
			if req.OpenAIAdvancedSchedulerEnabled != nil {
				return *req.OpenAIAdvancedSchedulerEnabled
		REDACTED
			return previousSettings.OpenAIAdvancedSchedulerEnabled
	REDACTED(),
		BalanceLowNotifyEnabled: func() bool {
			if req.BalanceLowNotifyEnabled != nil {
				return *req.BalanceLowNotifyEnabled
		REDACTED
			return previousSettings.BalanceLowNotifyEnabled
	REDACTED(),
		BalanceLowNotifyThreshold: func() float64 {
			if req.BalanceLowNotifyThreshold != nil {
				return *req.BalanceLowNotifyThreshold
		REDACTED
			return previousSettings.BalanceLowNotifyThreshold
	REDACTED(),
		BalanceLowNotifyRechargeURL: func() string {
			if req.BalanceLowNotifyRechargeURL != nil {
				return *req.BalanceLowNotifyRechargeURL
		REDACTED
			return previousSettings.BalanceLowNotifyRechargeURL
	REDACTED(),
		AccountQuotaNotifyEnabled: func() bool {
			if req.AccountQuotaNotifyEnabled != nil {
				return *req.AccountQuotaNotifyEnabled
		REDACTED
			return previousSettings.AccountQuotaNotifyEnabled
	REDACTED(),
		AccountQuotaNotifyEmails: func() []service.NotifyEmailEntry {
			if req.AccountQuotaNotifyEmails != nil {
				return dto.NotifyEmailEntriesToService(*req.AccountQuotaNotifyEmails)
		REDACTED
			return previousSettings.AccountQuotaNotifyEmails
	REDACTED(),
		ChannelMonitorEnabled: func() bool {
			if req.ChannelMonitorEnabled != nil {
				return *req.ChannelMonitorEnabled
		REDACTED
			return previousSettings.ChannelMonitorEnabled
	REDACTED(),
		ChannelMonitorDefaultIntervalSeconds: func() int {
			if req.ChannelMonitorDefaultIntervalSeconds != nil {
				return *req.ChannelMonitorDefaultIntervalSeconds
		REDACTED
			return previousSettings.ChannelMonitorDefaultIntervalSeconds
	REDACTED(),
		AvailableChannelsEnabled: func() bool {
			if req.AvailableChannelsEnabled != nil {
				return *req.AvailableChannelsEnabled
		REDACTED
			return previousSettings.AvailableChannelsEnabled
	REDACTED(),
		AffiliateEnabled: func() bool {
			if req.AffiliateEnabled != nil {
				return *req.AffiliateEnabled
		REDACTED
			return previousSettings.AffiliateEnabled
	REDACTED(),
REDACTED

	authSourceDefaults := &service.AuthSourceDefaultSettings{
		Email: service.ProviderDefaultGrantSettings{
			Balance:          float64ValueOrDefault(req.AuthSourceDefaultEmailBalance, previousAuthSourceDefaults.Email.Balance),
			Concurrency:      intValueOrDefault(req.AuthSourceDefaultEmailConcurrency, previousAuthSourceDefaults.Email.Concurrency),
			Subscriptions:    defaultSubscriptionsValueOrDefault(req.AuthSourceDefaultEmailSubscriptions, previousAuthSourceDefaults.Email.Subscriptions),
			GrantOnSignup:    boolValueOrDefault(req.AuthSourceDefaultEmailGrantOnSignup, previousAuthSourceDefaults.Email.GrantOnSignup),
			GrantOnFirstBind: boolValueOrDefault(req.AuthSourceDefaultEmailGrantOnFirstBind, previousAuthSourceDefaults.Email.GrantOnFirstBind),
	REDACTED,
		LinuxDo: service.ProviderDefaultGrantSettings{
			Balance:          float64ValueOrDefault(req.AuthSourceDefaultLinuxDoBalance, previousAuthSourceDefaults.LinuxDo.Balance),
			Concurrency:      intValueOrDefault(req.AuthSourceDefaultLinuxDoConcurrency, previousAuthSourceDefaults.LinuxDo.Concurrency),
			Subscriptions:    defaultSubscriptionsValueOrDefault(req.AuthSourceDefaultLinuxDoSubscriptions, previousAuthSourceDefaults.LinuxDo.Subscriptions),
			GrantOnSignup:    boolValueOrDefault(req.AuthSourceDefaultLinuxDoGrantOnSignup, previousAuthSourceDefaults.LinuxDo.GrantOnSignup),
			GrantOnFirstBind: boolValueOrDefault(req.AuthSourceDefaultLinuxDoGrantOnFirstBind, previousAuthSourceDefaults.LinuxDo.GrantOnFirstBind),
	REDACTED,
		OIDC: service.ProviderDefaultGrantSettings{
			Balance:          float64ValueOrDefault(req.AuthSourceDefaultOIDCBalance, previousAuthSourceDefaults.OIDC.Balance),
			Concurrency:      intValueOrDefault(req.AuthSourceDefaultOIDCConcurrency, previousAuthSourceDefaults.OIDC.Concurrency),
			Subscriptions:    defaultSubscriptionsValueOrDefault(req.AuthSourceDefaultOIDCSubscriptions, previousAuthSourceDefaults.OIDC.Subscriptions),
			GrantOnSignup:    boolValueOrDefault(req.AuthSourceDefaultOIDCGrantOnSignup, previousAuthSourceDefaults.OIDC.GrantOnSignup),
			GrantOnFirstBind: boolValueOrDefault(req.AuthSourceDefaultOIDCGrantOnFirstBind, previousAuthSourceDefaults.OIDC.GrantOnFirstBind),
	REDACTED,
		WeChat: service.ProviderDefaultGrantSettings{
			Balance:          float64ValueOrDefault(req.AuthSourceDefaultWeChatBalance, previousAuthSourceDefaults.WeChat.Balance),
			Concurrency:      intValueOrDefault(req.AuthSourceDefaultWeChatConcurrency, previousAuthSourceDefaults.WeChat.Concurrency),
			Subscriptions:    defaultSubscriptionsValueOrDefault(req.AuthSourceDefaultWeChatSubscriptions, previousAuthSourceDefaults.WeChat.Subscriptions),
			GrantOnSignup:    boolValueOrDefault(req.AuthSourceDefaultWeChatGrantOnSignup, previousAuthSourceDefaults.WeChat.GrantOnSignup),
			GrantOnFirstBind: boolValueOrDefault(req.AuthSourceDefaultWeChatGrantOnFirstBind, previousAuthSourceDefaults.WeChat.GrantOnFirstBind),
	REDACTED,
		ForceEmailOnThirdPartySignup: boolValueOrDefault(req.ForceEmailOnThirdPartySignup, previousAuthSourceDefaults.ForceEmailOnThirdPartySignup),
REDACTED
	if err := h.settingService.UpdateSettingsWithAuthSourceDefaults(c.Request.Context(), settings, authSourceDefaults); err != nil {
		response.ErrorFrom(c, err)
		return
REDACTED

	// Update OpenAI fast policy (stored under dedicated key, only when provided).
	if req.OpenAIFastPolicySettings != nil {
		if err := h.settingService.SetOpenAIFastPolicySettings(c.Request.Context(), openaiFastPolicySettingsFromDTO(req.OpenAIFastPolicySettings)); err != nil {
			response.BadRequest(c, err.Error())
			return
	REDACTED
REDACTED

	// Update payment configuration (integrated into system settings).
	// Skip if no payment fields were provided (prevents accidental wipe).
	if h.paymentConfigService != nil && hasPaymentFields(req) {
		paymentReq := service.UpdatePaymentConfigRequest{
			Enabled:                   req.PaymentEnabled,
			MinAmount:                 req.PaymentMinAmount,
			MaxAmount:                 req.PaymentMaxAmount,
			DailyLimit:                req.PaymentDailyLimit,
			OrderTimeoutMin:           req.PaymentOrderTimeoutMin,
			MaxPendingOrders:          req.PaymentMaxPendingOrders,
			EnabledTypes:              req.PaymentEnabledTypes,
			BalanceDisabled:           req.PaymentBalanceDisabled,
			BalanceRechargeMultiplier: req.PaymentBalanceRechargeMultiplier,
			RechargeFeeRate:           req.PaymentRechargeFeeRate,
			LoadBalanceStrategy:       req.PaymentLoadBalanceStrat,
			ProductNamePrefix:         req.PaymentProductNamePrefix,
			ProductNameSuffix:         req.PaymentProductNameSuffix,
			HelpImageURL:              req.PaymentHelpImageURL,
			HelpText:                  req.PaymentHelpText,
			CancelRateLimitEnabled:    req.PaymentCancelRateLimitEnabled,
			CancelRateLimitMax:        req.PaymentCancelRateLimitMax,
			CancelRateLimitWindow:     req.PaymentCancelRateLimitWindow,
			CancelRateLimitUnit:       req.PaymentCancelRateLimitUnit,
			CancelRateLimitMode:       req.PaymentCancelRateLimitMode,
	REDACTED
		if err := h.paymentConfigService.UpdatePaymentConfig(c.Request.Context(), paymentReq); err != nil {
			response.ErrorFrom(c, err)
			return
	REDACTED
		// Refresh in-memory provider registry so config changes take effect immediately
		if h.paymentService != nil {
			h.paymentService.RefreshProviders(c.Request.Context())
	REDACTED
REDACTED

	h.auditSettingsUpdate(c, previousSettings, settings, previousAuthSourceDefaults, authSourceDefaults, req)

	// 重新获取设置返回
	updatedSettings, err := h.settingService.GetAllSettings(c.Request.Context())
	if err != nil {
		response.ErrorFrom(c, err)
		return
REDACTED
	updatedAuthSourceDefaults, err := h.settingService.GetAuthSourceDefaultSettings(c.Request.Context())
	if err != nil {
		response.ErrorFrom(c, err)
		return
REDACTED
	updatedDefaultSubscriptions := make([]dto.DefaultSubscriptionSetting, 0, len(updatedSettings.DefaultSubscriptions))
	for _, sub := range updatedSettings.DefaultSubscriptions {
		updatedDefaultSubscriptions = append(updatedDefaultSubscriptions, dto.DefaultSubscriptionSetting{
			GroupID:      sub.GroupID,
			ValidityDays: sub.ValidityDays,
	REDACTED)
REDACTED

	// Reload payment config for response
	var updatedPaymentCfg *service.PaymentConfig
	if h.paymentConfigService != nil {
		updatedPaymentCfg, _ = h.paymentConfigService.GetPaymentConfig(c.Request.Context())
REDACTED
	if updatedPaymentCfg == nil {
		updatedPaymentCfg = &service.PaymentConfig{REDACTED
REDACTED

	payload := dto.SystemSettings{
		RegistrationEnabled:                    updatedSettings.RegistrationEnabled,
		EmailVerifyEnabled:                     updatedSettings.EmailVerifyEnabled,
		RegistrationEmailSuffixWhitelist:       updatedSettings.RegistrationEmailSuffixWhitelist,
		PromoCodeEnabled:                       updatedSettings.PromoCodeEnabled,
		PasswordResetEnabled:                   updatedSettings.PasswordResetEnabled,
		FrontendURL:                            updatedSettings.FrontendURL,
		InvitationCodeEnabled:                  updatedSettings.InvitationCodeEnabled,
		TotpEnabled:                            updatedSettings.TotpEnabled,
		TotpEncryptionKeyConfigured:            h.settingService.IsTotpEncryptionKeyConfigured(),
		SMTPHost:                               updatedSettings.SMTPHost,
		SMTPPort:                               updatedSettings.SMTPPort,
		SMTPUsername:                           updatedSettings.SMTPUsername,
		SMTPPasswordConfigured:                 updatedSettings.SMTPPasswordConfigured,
		SMTPFrom:                               updatedSettings.SMTPFrom,
		SMTPFromName:                           updatedSettings.SMTPFromName,
		SMTPUseTLS:                             updatedSettings.SMTPUseTLS,
		TurnstileEnabled:                       updatedSettings.TurnstileEnabled,
		TurnstileSiteKey:                       updatedSettings.TurnstileSiteKey,
		TurnstileSecretKeyConfigured:           updatedSettings.TurnstileSecretKeyConfigured,
		LinuxDoConnectEnabled:                  updatedSettings.LinuxDoConnectEnabled,
		LinuxDoConnectClientID:                 updatedSettings.LinuxDoConnectClientID,
		LinuxDoConnectClientSecretConfigured:   updatedSettings.LinuxDoConnectClientSecretConfigured,
		LinuxDoConnectRedirectURL:              updatedSettings.LinuxDoConnectRedirectURL,
		WeChatConnectEnabled:                   updatedSettings.WeChatConnectEnabled,
		WeChatConnectAppID:                     updatedSettings.WeChatConnectAppID,
		WeChatConnectAppSecretConfigured:       updatedSettings.WeChatConnectAppSecretConfigured,
		WeChatConnectOpenAppID:                 updatedSettings.WeChatConnectOpenAppID,
		WeChatConnectOpenAppSecretConfigured:   updatedSettings.WeChatConnectOpenAppSecretConfigured,
		WeChatConnectMPAppID:                   updatedSettings.WeChatConnectMPAppID,
		WeChatConnectMPAppSecretConfigured:     updatedSettings.WeChatConnectMPAppSecretConfigured,
		WeChatConnectMobileAppID:               updatedSettings.WeChatConnectMobileAppID,
		WeChatConnectMobileAppSecretConfigured: updatedSettings.WeChatConnectMobileAppSecretConfigured,
		WeChatConnectOpenEnabled:               updatedSettings.WeChatConnectOpenEnabled,
		WeChatConnectMPEnabled:                 updatedSettings.WeChatConnectMPEnabled,
		WeChatConnectMobileEnabled:             updatedSettings.WeChatConnectMobileEnabled,
		WeChatConnectMode:                      updatedSettings.WeChatConnectMode,
		WeChatConnectScopes:                    updatedSettings.WeChatConnectScopes,
		WeChatConnectRedirectURL:               updatedSettings.WeChatConnectRedirectURL,
		WeChatConnectFrontendRedirectURL:       updatedSettings.WeChatConnectFrontendRedirectURL,
		OIDCConnectEnabled:                     updatedSettings.OIDCConnectEnabled,
		OIDCConnectProviderName:                updatedSettings.OIDCConnectProviderName,
		OIDCConnectClientID:                    updatedSettings.OIDCConnectClientID,
		OIDCConnectClientSecretConfigured:      updatedSettings.OIDCConnectClientSecretConfigured,
		OIDCConnectIssuerURL:                   updatedSettings.OIDCConnectIssuerURL,
		OIDCConnectDiscoveryURL:                updatedSettings.OIDCConnectDiscoveryURL,
		OIDCConnectAuthorizeURL:                updatedSettings.OIDCConnectAuthorizeURL,
		OIDCConnectTokenURL:                    updatedSettings.OIDCConnectTokenURL,
		OIDCConnectUserInfoURL:                 updatedSettings.OIDCConnectUserInfoURL,
		OIDCConnectJWKSURL:                     updatedSettings.OIDCConnectJWKSURL,
		OIDCConnectScopes:                      updatedSettings.OIDCConnectScopes,
		OIDCConnectRedirectURL:                 updatedSettings.OIDCConnectRedirectURL,
		OIDCConnectFrontendRedirectURL:         updatedSettings.OIDCConnectFrontendRedirectURL,
		OIDCConnectTokenAuthMethod:             updatedSettings.OIDCConnectTokenAuthMethod,
		OIDCConnectUsePKCE:                     updatedSettings.OIDCConnectUsePKCE,
		OIDCConnectValidateIDToken:             updatedSettings.OIDCConnectValidateIDToken,
		OIDCConnectAllowedSigningAlgs:          updatedSettings.OIDCConnectAllowedSigningAlgs,
		OIDCConnectClockSkewSeconds:            updatedSettings.OIDCConnectClockSkewSeconds,
		OIDCConnectRequireEmailVerified:        updatedSettings.OIDCConnectRequireEmailVerified,
		OIDCConnectUserInfoEmailPath:           updatedSettings.OIDCConnectUserInfoEmailPath,
		OIDCConnectUserInfoIDPath:              updatedSettings.OIDCConnectUserInfoIDPath,
		OIDCConnectUserInfoUsernamePath:        updatedSettings.OIDCConnectUserInfoUsernamePath,
		SiteName:                               updatedSettings.SiteName,
		SiteLogo:                               updatedSettings.SiteLogo,
		SiteSubtitle:                           updatedSettings.SiteSubtitle,
		APIBaseURL:                             updatedSettings.APIBaseURL,
		ContactInfo:                            updatedSettings.ContactInfo,
		DocURL:                                 updatedSettings.DocURL,
		HomeContent:                            updatedSettings.HomeContent,
		HideCcsImportButton:                    updatedSettings.HideCcsImportButton,
		PurchaseSubscriptionEnabled:            updatedSettings.PurchaseSubscriptionEnabled,
		PurchaseSubscriptionURL:                updatedSettings.PurchaseSubscriptionURL,
		TableDefaultPageSize:                   updatedSettings.TableDefaultPageSize,
		TablePageSizeOptions:                   updatedSettings.TablePageSizeOptions,
		CustomMenuItems:                        dto.ParseCustomMenuItems(updatedSettings.CustomMenuItems),
		CustomEndpoints:                        dto.ParseCustomEndpoints(updatedSettings.CustomEndpoints),
		DefaultConcurrency:                     updatedSettings.DefaultConcurrency,
		DefaultBalance:                         updatedSettings.DefaultBalance,
		AffiliateRebateRate:                    updatedSettings.AffiliateRebateRate,
		AffiliateRebateFreezeHours:             updatedSettings.AffiliateRebateFreezeHours,
		AffiliateRebateDurationDays:            updatedSettings.AffiliateRebateDurationDays,
		AffiliateRebatePerInviteeCap:           updatedSettings.AffiliateRebatePerInviteeCap,
		DefaultUserRPMLimit:                    updatedSettings.DefaultUserRPMLimit,
		DefaultSubscriptions:                   updatedDefaultSubscriptions,
		EnableModelFallback:                    updatedSettings.EnableModelFallback,
		FallbackModelAnthropic:                 updatedSettings.FallbackModelAnthropic,
		FallbackModelOpenAI:                    updatedSettings.FallbackModelOpenAI,
		FallbackModelGemini:                    updatedSettings.FallbackModelGemini,
		FallbackModelAntigravity:               updatedSettings.FallbackModelAntigravity,
		EnableIdentityPatch:                    updatedSettings.EnableIdentityPatch,
		IdentityPatchPrompt:                    updatedSettings.IdentityPatchPrompt,
		OpsMonitoringEnabled:                   updatedSettings.OpsMonitoringEnabled,
		OpsRealtimeMonitoringEnabled:           updatedSettings.OpsRealtimeMonitoringEnabled,
		OpsQueryModeDefault:                    updatedSettings.OpsQueryModeDefault,
		OpsMetricsIntervalSeconds:              updatedSettings.OpsMetricsIntervalSeconds,
		MinClaudeCodeVersion:                   updatedSettings.MinClaudeCodeVersion,
		MaxClaudeCodeVersion:                   updatedSettings.MaxClaudeCodeVersion,
		AllowUngroupedKeyScheduling:            updatedSettings.AllowUngroupedKeyScheduling,
		BackendModeEnabled:                     updatedSettings.BackendModeEnabled,
		EnableFingerprintUnification:           updatedSettings.EnableFingerprintUnification,
		EnableMetadataPassthrough:              updatedSettings.EnableMetadataPassthrough,
		EnableCCHSigning:                       updatedSettings.EnableCCHSigning,
		PaymentVisibleMethodAlipaySource:       updatedSettings.PaymentVisibleMethodAlipaySource,
		PaymentVisibleMethodWxpaySource:        updatedSettings.PaymentVisibleMethodWxpaySource,
		PaymentVisibleMethodAlipayEnabled:      updatedSettings.PaymentVisibleMethodAlipayEnabled,
		PaymentVisibleMethodWxpayEnabled:       updatedSettings.PaymentVisibleMethodWxpayEnabled,
		OpenAIAdvancedSchedulerEnabled:         updatedSettings.OpenAIAdvancedSchedulerEnabled,
		BalanceLowNotifyEnabled:                updatedSettings.BalanceLowNotifyEnabled,
		BalanceLowNotifyThreshold:              updatedSettings.BalanceLowNotifyThreshold,
		BalanceLowNotifyRechargeURL:            updatedSettings.BalanceLowNotifyRechargeURL,
		AccountQuotaNotifyEnabled:              updatedSettings.AccountQuotaNotifyEnabled,
		AccountQuotaNotifyEmails:               dto.NotifyEmailEntriesFromService(updatedSettings.AccountQuotaNotifyEmails),
		PaymentEnabled:                         updatedPaymentCfg.Enabled,
		PaymentMinAmount:                       updatedPaymentCfg.MinAmount,
		PaymentMaxAmount:                       updatedPaymentCfg.MaxAmount,
		PaymentDailyLimit:                      updatedPaymentCfg.DailyLimit,
		PaymentOrderTimeoutMin:                 updatedPaymentCfg.OrderTimeoutMin,
		PaymentMaxPendingOrders:                updatedPaymentCfg.MaxPendingOrders,
		PaymentEnabledTypes:                    updatedPaymentCfg.EnabledTypes,
		PaymentBalanceDisabled:                 updatedPaymentCfg.BalanceDisabled,
		PaymentBalanceRechargeMultiplier:       updatedPaymentCfg.BalanceRechargeMultiplier,
		PaymentRechargeFeeRate:                 updatedPaymentCfg.RechargeFeeRate,
		PaymentLoadBalanceStrat:                updatedPaymentCfg.LoadBalanceStrategy,
		PaymentProductNamePrefix:               updatedPaymentCfg.ProductNamePrefix,
		PaymentProductNameSuffix:               updatedPaymentCfg.ProductNameSuffix,
		PaymentHelpImageURL:                    updatedPaymentCfg.HelpImageURL,
		PaymentHelpText:                        updatedPaymentCfg.HelpText,
		PaymentCancelRateLimitEnabled:          updatedPaymentCfg.CancelRateLimitEnabled,
		PaymentCancelRateLimitMax:              updatedPaymentCfg.CancelRateLimitMax,
		PaymentCancelRateLimitWindow:           updatedPaymentCfg.CancelRateLimitWindow,
		PaymentCancelRateLimitUnit:             updatedPaymentCfg.CancelRateLimitUnit,
		PaymentCancelRateLimitMode:             updatedPaymentCfg.CancelRateLimitMode,

		ChannelMonitorEnabled:                updatedSettings.ChannelMonitorEnabled,
		ChannelMonitorDefaultIntervalSeconds: updatedSettings.ChannelMonitorDefaultIntervalSeconds,

		AvailableChannelsEnabled: updatedSettings.AvailableChannelsEnabled,

		AffiliateEnabled: updatedSettings.AffiliateEnabled,
REDACTED
	if fastPolicy, err := h.settingService.GetOpenAIFastPolicySettings(c.Request.Context()); err != nil {
		slog.Error("openai_fast_policy_settings_get_failed", "error", err)
REDACTED else if fastPolicy != nil {
		payload.OpenAIFastPolicySettings = openaiFastPolicySettingsToDTO(fastPolicy)
REDACTED
	response.Success(c, systemSettingsResponseData(payload, updatedAuthSourceDefaults))
REDACTED

// hasPaymentFields returns true if any payment-related field was explicitly provided.
func hasPaymentFields(req UpdateSettingsRequest) bool {
	return req.PaymentEnabled != nil || req.PaymentMinAmount != nil ||
		req.PaymentMaxAmount != nil || req.PaymentDailyLimit != nil ||
		req.PaymentOrderTimeoutMin != nil || req.PaymentMaxPendingOrders != nil ||
		req.PaymentEnabledTypes != nil || req.PaymentBalanceDisabled != nil ||
		req.PaymentBalanceRechargeMultiplier != nil || req.PaymentRechargeFeeRate != nil ||
		req.PaymentLoadBalanceStrat != nil || req.PaymentProductNamePrefix != nil ||
		req.PaymentProductNameSuffix != nil || req.PaymentHelpImageURL != nil ||
		req.PaymentHelpText != nil || req.PaymentCancelRateLimitEnabled != nil ||
		req.PaymentCancelRateLimitMax != nil || req.PaymentCancelRateLimitWindow != nil ||
		req.PaymentCancelRateLimitUnit != nil || req.PaymentCancelRateLimitMode != nil
REDACTED

func (h *SettingHandler) auditSettingsUpdate(c *gin.Context, before *service.SystemSettings, after *service.SystemSettings, beforeAuthSourceDefaults *service.AuthSourceDefaultSettings, afterAuthSourceDefaults *service.AuthSourceDefaultSettings, req UpdateSettingsRequest) {
	if before == nil || after == nil {
		return
REDACTED

	changed := diffSettings(before, after, beforeAuthSourceDefaults, afterAuthSourceDefaults, req)
	if len(changed) == 0 {
		return
REDACTED

	subject, _ := middleware.GetAuthSubjectFromContext(c)
	role, _ := middleware.GetUserRoleFromContext(c)
	slog.Info("settings updated",
		"audit", true,
		"user_id", subject.UserID,
		"role", role,
		"changed", changed,
	)
REDACTED

func diffSettings(before *service.SystemSettings, after *service.SystemSettings, beforeAuthSourceDefaults *service.AuthSourceDefaultSettings, afterAuthSourceDefaults *service.AuthSourceDefaultSettings, req UpdateSettingsRequest) []string {
	changed := make([]string, 0, 20)
	if before.RegistrationEnabled != after.RegistrationEnabled {
		changed = append(changed, "registration_enabled")
REDACTED
	if before.EmailVerifyEnabled != after.EmailVerifyEnabled {
		changed = append(changed, "email_verify_enabled")
REDACTED
	if !equalStringSlice(before.RegistrationEmailSuffixWhitelist, after.RegistrationEmailSuffixWhitelist) {
		changed = append(changed, "registration_email_suffix_whitelist")
REDACTED
	if before.PromoCodeEnabled != after.PromoCodeEnabled {
		changed = append(changed, "promo_code_enabled")
REDACTED
	if before.InvitationCodeEnabled != after.InvitationCodeEnabled {
		changed = append(changed, "invitation_code_enabled")
REDACTED
	if before.PasswordResetEnabled != after.PasswordResetEnabled {
		changed = append(changed, "password_reset_enabled")
REDACTED
	if before.FrontendURL != after.FrontendURL {
		changed = append(changed, "frontend_url")
REDACTED
	if before.TotpEnabled != after.TotpEnabled {
		changed = append(changed, "totp_enabled")
REDACTED
	if before.SMTPHost != after.SMTPHost {
		changed = append(changed, "smtp_host")
REDACTED
	if before.SMTPPort != after.SMTPPort {
		changed = append(changed, "smtp_port")
REDACTED
	if before.SMTPUsername != after.SMTPUsername {
		changed = append(changed, "smtp_username")
REDACTED
	if req.SMTPPassword != "" {
		changed = append(changed, "smtp_password")
REDACTED
	if before.SMTPFrom != after.SMTPFrom {
		changed = append(changed, "smtp_from_email")
REDACTED
	if before.SMTPFromName != after.SMTPFromName {
		changed = append(changed, "smtp_from_name")
REDACTED
	if before.SMTPUseTLS != after.SMTPUseTLS {
		changed = append(changed, "smtp_use_tls")
REDACTED
	if before.TurnstileEnabled != after.TurnstileEnabled {
		changed = append(changed, "turnstile_enabled")
REDACTED
	if before.TurnstileSiteKey != after.TurnstileSiteKey {
		changed = append(changed, "turnstile_site_key")
REDACTED
	if req.TurnstileSecretKey != "" {
		changed = append(changed, "turnstile_secret_key")
REDACTED
	if before.LinuxDoConnectEnabled != after.LinuxDoConnectEnabled {
		changed = append(changed, "linuxdo_connect_enabled")
REDACTED
	if before.LinuxDoConnectClientID != after.LinuxDoConnectClientID {
		changed = append(changed, "linuxdo_connect_client_id")
REDACTED
	if req.LinuxDoConnectClientSecret != "" {
		changed = append(changed, "linuxdo_connect_client_secret")
REDACTED
	if before.LinuxDoConnectRedirectURL != after.LinuxDoConnectRedirectURL {
		changed = append(changed, "linuxdo_connect_redirect_url")
REDACTED
	if before.WeChatConnectEnabled != after.WeChatConnectEnabled {
		changed = append(changed, "wechat_connect_enabled")
REDACTED
	if before.WeChatConnectAppID != after.WeChatConnectAppID {
		changed = append(changed, "wechat_connect_app_id")
REDACTED
	if req.WeChatConnectAppSecret != "" {
		changed = append(changed, "wechat_connect_app_secret")
REDACTED
	if before.WeChatConnectOpenAppID != after.WeChatConnectOpenAppID {
		changed = append(changed, "wechat_connect_open_app_id")
REDACTED
	if req.WeChatConnectOpenAppSecret != "" {
		changed = append(changed, "wechat_connect_open_app_secret")
REDACTED
	if before.WeChatConnectMPAppID != after.WeChatConnectMPAppID {
		changed = append(changed, "wechat_connect_mp_app_id")
REDACTED
	if req.WeChatConnectMPAppSecret != "" {
		changed = append(changed, "wechat_connect_mp_app_secret")
REDACTED
	if before.WeChatConnectMobileAppID != after.WeChatConnectMobileAppID {
		changed = append(changed, "wechat_connect_mobile_app_id")
REDACTED
	if req.WeChatConnectMobileAppSecret != "" {
		changed = append(changed, "wechat_connect_mobile_app_secret")
REDACTED
	if before.WeChatConnectOpenEnabled != after.WeChatConnectOpenEnabled {
		changed = append(changed, "wechat_connect_open_enabled")
REDACTED
	if before.WeChatConnectMPEnabled != after.WeChatConnectMPEnabled {
		changed = append(changed, "wechat_connect_mp_enabled")
REDACTED
	if before.WeChatConnectMobileEnabled != after.WeChatConnectMobileEnabled {
		changed = append(changed, "wechat_connect_mobile_enabled")
REDACTED
	if before.WeChatConnectMode != after.WeChatConnectMode {
		changed = append(changed, "wechat_connect_mode")
REDACTED
	if before.WeChatConnectScopes != after.WeChatConnectScopes {
		changed = append(changed, "wechat_connect_scopes")
REDACTED
	if before.WeChatConnectRedirectURL != after.WeChatConnectRedirectURL {
		changed = append(changed, "wechat_connect_redirect_url")
REDACTED
	if before.WeChatConnectFrontendRedirectURL != after.WeChatConnectFrontendRedirectURL {
		changed = append(changed, "wechat_connect_frontend_redirect_url")
REDACTED
	if before.OIDCConnectEnabled != after.OIDCConnectEnabled {
		changed = append(changed, "oidc_connect_enabled")
REDACTED
	if before.OIDCConnectProviderName != after.OIDCConnectProviderName {
		changed = append(changed, "oidc_connect_provider_name")
REDACTED
	if before.OIDCConnectClientID != after.OIDCConnectClientID {
		changed = append(changed, "oidc_connect_client_id")
REDACTED
	if req.OIDCConnectClientSecret != "" {
		changed = append(changed, "oidc_connect_client_secret")
REDACTED
	if before.OIDCConnectIssuerURL != after.OIDCConnectIssuerURL {
		changed = append(changed, "oidc_connect_issuer_url")
REDACTED
	if before.OIDCConnectDiscoveryURL != after.OIDCConnectDiscoveryURL {
		changed = append(changed, "oidc_connect_discovery_url")
REDACTED
	if before.OIDCConnectAuthorizeURL != after.OIDCConnectAuthorizeURL {
		changed = append(changed, "oidc_connect_authorize_url")
REDACTED
	if before.OIDCConnectTokenURL != after.OIDCConnectTokenURL {
		changed = append(changed, "oidc_connect_token_url")
REDACTED
	if before.OIDCConnectUserInfoURL != after.OIDCConnectUserInfoURL {
		changed = append(changed, "oidc_connect_userinfo_url")
REDACTED
	if before.OIDCConnectJWKSURL != after.OIDCConnectJWKSURL {
		changed = append(changed, "oidc_connect_jwks_url")
REDACTED
	if before.OIDCConnectScopes != after.OIDCConnectScopes {
		changed = append(changed, "oidc_connect_scopes")
REDACTED
	if before.OIDCConnectRedirectURL != after.OIDCConnectRedirectURL {
		changed = append(changed, "oidc_connect_redirect_url")
REDACTED
	if before.OIDCConnectFrontendRedirectURL != after.OIDCConnectFrontendRedirectURL {
		changed = append(changed, "oidc_connect_frontend_redirect_url")
REDACTED
	if before.OIDCConnectTokenAuthMethod != after.OIDCConnectTokenAuthMethod {
		changed = append(changed, "oidc_connect_token_auth_method")
REDACTED
	if before.OIDCConnectUsePKCE != after.OIDCConnectUsePKCE {
		changed = append(changed, "oidc_connect_use_pkce")
REDACTED
	if before.OIDCConnectValidateIDToken != after.OIDCConnectValidateIDToken {
		changed = append(changed, "oidc_connect_validate_id_token")
REDACTED
	if before.OIDCConnectAllowedSigningAlgs != after.OIDCConnectAllowedSigningAlgs {
		changed = append(changed, "oidc_connect_allowed_signing_algs")
REDACTED
	if before.OIDCConnectClockSkewSeconds != after.OIDCConnectClockSkewSeconds {
		changed = append(changed, "oidc_connect_clock_skew_seconds")
REDACTED
	if before.OIDCConnectRequireEmailVerified != after.OIDCConnectRequireEmailVerified {
		changed = append(changed, "oidc_connect_require_email_verified")
REDACTED
	if before.OIDCConnectUserInfoEmailPath != after.OIDCConnectUserInfoEmailPath {
		changed = append(changed, "oidc_connect_userinfo_email_path")
REDACTED
	if before.OIDCConnectUserInfoIDPath != after.OIDCConnectUserInfoIDPath {
		changed = append(changed, "oidc_connect_userinfo_id_path")
REDACTED
	if before.OIDCConnectUserInfoUsernamePath != after.OIDCConnectUserInfoUsernamePath {
		changed = append(changed, "oidc_connect_userinfo_username_path")
REDACTED
	if before.SiteName != after.SiteName {
		changed = append(changed, "site_name")
REDACTED
	if before.SiteLogo != after.SiteLogo {
		changed = append(changed, "site_logo")
REDACTED
	if before.SiteSubtitle != after.SiteSubtitle {
		changed = append(changed, "site_subtitle")
REDACTED
	if before.APIBaseURL != after.APIBaseURL {
		changed = append(changed, "api_base_url")
REDACTED
	if before.ContactInfo != after.ContactInfo {
		changed = append(changed, "contact_info")
REDACTED
	if before.DocURL != after.DocURL {
		changed = append(changed, "doc_url")
REDACTED
	if before.HomeContent != after.HomeContent {
		changed = append(changed, "home_content")
REDACTED
	if before.HideCcsImportButton != after.HideCcsImportButton {
		changed = append(changed, "hide_ccs_import_button")
REDACTED
	if before.DefaultConcurrency != after.DefaultConcurrency {
		changed = append(changed, "default_concurrency")
REDACTED
	if before.DefaultBalance != after.DefaultBalance {
		changed = append(changed, "default_balance")
REDACTED
	if before.AffiliateRebateRate != after.AffiliateRebateRate {
		changed = append(changed, "affiliate_rebate_rate")
REDACTED
	if before.AffiliateRebateFreezeHours != after.AffiliateRebateFreezeHours {
		changed = append(changed, "affiliate_rebate_freeze_hours")
REDACTED
	if before.AffiliateRebateDurationDays != after.AffiliateRebateDurationDays {
		changed = append(changed, "affiliate_rebate_duration_days")
REDACTED
	if before.AffiliateRebatePerInviteeCap != after.AffiliateRebatePerInviteeCap {
		changed = append(changed, "affiliate_rebate_per_invitee_cap")
REDACTED
	if !equalDefaultSubscriptions(before.DefaultSubscriptions, after.DefaultSubscriptions) {
		changed = append(changed, "default_subscriptions")
REDACTED
	if before.EnableModelFallback != after.EnableModelFallback {
		changed = append(changed, "enable_model_fallback")
REDACTED
	if before.FallbackModelAnthropic != after.FallbackModelAnthropic {
		changed = append(changed, "fallback_model_anthropic")
REDACTED
	if before.FallbackModelOpenAI != after.FallbackModelOpenAI {
		changed = append(changed, "fallback_model_openai")
REDACTED
	if before.FallbackModelGemini != after.FallbackModelGemini {
		changed = append(changed, "fallback_model_gemini")
REDACTED
	if before.FallbackModelAntigravity != after.FallbackModelAntigravity {
		changed = append(changed, "fallback_model_antigravity")
REDACTED
	if before.EnableIdentityPatch != after.EnableIdentityPatch {
		changed = append(changed, "enable_identity_patch")
REDACTED
	if before.IdentityPatchPrompt != after.IdentityPatchPrompt {
		changed = append(changed, "identity_patch_prompt")
REDACTED
	if before.OpsMonitoringEnabled != after.OpsMonitoringEnabled {
		changed = append(changed, "ops_monitoring_enabled")
REDACTED
	if before.OpsRealtimeMonitoringEnabled != after.OpsRealtimeMonitoringEnabled {
		changed = append(changed, "ops_realtime_monitoring_enabled")
REDACTED
	if before.OpsQueryModeDefault != after.OpsQueryModeDefault {
		changed = append(changed, "ops_query_mode_default")
REDACTED
	if before.OpsMetricsIntervalSeconds != after.OpsMetricsIntervalSeconds {
		changed = append(changed, "ops_metrics_interval_seconds")
REDACTED
	if before.MinClaudeCodeVersion != after.MinClaudeCodeVersion {
		changed = append(changed, "min_claude_code_version")
REDACTED
	if before.MaxClaudeCodeVersion != after.MaxClaudeCodeVersion {
		changed = append(changed, "max_claude_code_version")
REDACTED
	if before.AllowUngroupedKeyScheduling != after.AllowUngroupedKeyScheduling {
		changed = append(changed, "allow_ungrouped_key_scheduling")
REDACTED
	if before.BackendModeEnabled != after.BackendModeEnabled {
		changed = append(changed, "backend_mode_enabled")
REDACTED
	if before.PurchaseSubscriptionEnabled != after.PurchaseSubscriptionEnabled {
		changed = append(changed, "purchase_subscription_enabled")
REDACTED
	if before.PurchaseSubscriptionURL != after.PurchaseSubscriptionURL {
		changed = append(changed, "purchase_subscription_url")
REDACTED
	if before.TableDefaultPageSize != after.TableDefaultPageSize {
		changed = append(changed, "table_default_page_size")
REDACTED
	if !equalIntSlice(before.TablePageSizeOptions, after.TablePageSizeOptions) {
		changed = append(changed, "table_page_size_options")
REDACTED
	if before.CustomMenuItems != after.CustomMenuItems {
		changed = append(changed, "custom_menu_items")
REDACTED
	if before.CustomEndpoints != after.CustomEndpoints {
		changed = append(changed, "custom_endpoints")
REDACTED
	if before.EnableFingerprintUnification != after.EnableFingerprintUnification {
		changed = append(changed, "enable_fingerprint_unification")
REDACTED
	if before.EnableMetadataPassthrough != after.EnableMetadataPassthrough {
		changed = append(changed, "enable_metadata_passthrough")
REDACTED
	if before.EnableCCHSigning != after.EnableCCHSigning {
		changed = append(changed, "enable_cch_signing")
REDACTED
	if before.PaymentVisibleMethodAlipaySource != after.PaymentVisibleMethodAlipaySource {
		changed = append(changed, "payment_visible_method_alipay_source")
REDACTED
	if before.PaymentVisibleMethodWxpaySource != after.PaymentVisibleMethodWxpaySource {
		changed = append(changed, "payment_visible_method_wxpay_source")
REDACTED
	if before.PaymentVisibleMethodAlipayEnabled != after.PaymentVisibleMethodAlipayEnabled {
		changed = append(changed, "payment_visible_method_alipay_enabled")
REDACTED
	if before.PaymentVisibleMethodWxpayEnabled != after.PaymentVisibleMethodWxpayEnabled {
		changed = append(changed, "payment_visible_method_wxpay_enabled")
REDACTED
	if before.OpenAIAdvancedSchedulerEnabled != after.OpenAIAdvancedSchedulerEnabled {
		changed = append(changed, "openai_advanced_scheduler_enabled")
REDACTED
	// Balance & quota notification
	if before.BalanceLowNotifyEnabled != after.BalanceLowNotifyEnabled {
		changed = append(changed, "balance_low_notify_enabled")
REDACTED
	if before.BalanceLowNotifyThreshold != after.BalanceLowNotifyThreshold {
		changed = append(changed, "balance_low_notify_threshold")
REDACTED
	if before.BalanceLowNotifyRechargeURL != after.BalanceLowNotifyRechargeURL {
		changed = append(changed, "balance_low_notify_recharge_url")
REDACTED
	if before.AccountQuotaNotifyEnabled != after.AccountQuotaNotifyEnabled {
		changed = append(changed, "account_quota_notify_enabled")
REDACTED
	if !equalNotifyEmailEntries(before.AccountQuotaNotifyEmails, after.AccountQuotaNotifyEmails) {
		changed = append(changed, "account_quota_notify_emails")
REDACTED
	if before.ChannelMonitorEnabled != after.ChannelMonitorEnabled {
		changed = append(changed, "channel_monitor_enabled")
REDACTED
	if before.ChannelMonitorDefaultIntervalSeconds != after.ChannelMonitorDefaultIntervalSeconds {
		changed = append(changed, "channel_monitor_default_interval_seconds")
REDACTED
	if before.AvailableChannelsEnabled != after.AvailableChannelsEnabled {
		changed = append(changed, "available_channels_enabled")
REDACTED
	if before.AffiliateEnabled != after.AffiliateEnabled {
		changed = append(changed, "affiliate_enabled")
REDACTED
	changed = appendAuthSourceDefaultChanges(changed, beforeAuthSourceDefaults, afterAuthSourceDefaults)
	return changed
REDACTED

func appendAuthSourceDefaultChanges(changed []string, before *service.AuthSourceDefaultSettings, after *service.AuthSourceDefaultSettings) []string {
	if before == nil {
		before = &service.AuthSourceDefaultSettings{REDACTED
REDACTED
	if after == nil {
		after = &service.AuthSourceDefaultSettings{REDACTED
REDACTED

	type providerDefaultGrantField struct {
		name   string
		before service.ProviderDefaultGrantSettings
		after  service.ProviderDefaultGrantSettings
REDACTED

	fields := []providerDefaultGrantField{
		{name: "email", before: before.Email, after: after.EmailREDACTED,
		{name: "linuxdo", before: before.LinuxDo, after: after.LinuxDoREDACTED,
		{name: "oidc", before: before.OIDC, after: after.OIDCREDACTED,
		{name: "wechat", before: before.WeChat, after: after.WeChatREDACTED,
REDACTED
	for _, field := range fields {
		if field.before.Balance != field.after.Balance {
			changed = append(changed, "auth_source_default_"+field.name+"_balance")
	REDACTED
		if field.before.Concurrency != field.after.Concurrency {
			changed = append(changed, "auth_source_default_"+field.name+"_concurrency")
	REDACTED
		if !equalDefaultSubscriptions(field.before.Subscriptions, field.after.Subscriptions) {
			changed = append(changed, "auth_source_default_"+field.name+"_subscriptions")
	REDACTED
		if field.before.GrantOnSignup != field.after.GrantOnSignup {
			changed = append(changed, "auth_source_default_"+field.name+"_grant_on_signup")
	REDACTED
		if field.before.GrantOnFirstBind != field.after.GrantOnFirstBind {
			changed = append(changed, "auth_source_default_"+field.name+"_grant_on_first_bind")
	REDACTED
REDACTED
	if before.ForceEmailOnThirdPartySignup != after.ForceEmailOnThirdPartySignup {
		changed = append(changed, "force_email_on_third_party_signup")
REDACTED
	return changed
REDACTED

func normalizeDefaultSubscriptions(input []dto.DefaultSubscriptionSetting) []dto.DefaultSubscriptionSetting {
	if len(input) == 0 {
		return nil
REDACTED
	normalized := make([]dto.DefaultSubscriptionSetting, 0, len(input))
	for _, item := range input {
		if item.GroupID <= 0 || item.ValidityDays <= 0 {
			continue
	REDACTED
		if item.ValidityDays > service.MaxValidityDays {
			item.ValidityDays = service.MaxValidityDays
	REDACTED
		normalized = append(normalized, item)
REDACTED
	return normalized
REDACTED

func normalizeOptionalDefaultSubscriptions(input *[]dto.DefaultSubscriptionSetting) *[]dto.DefaultSubscriptionSetting {
	if input == nil {
		return nil
REDACTED
	normalized := normalizeDefaultSubscriptions(*input)
	return &normalized
REDACTED

func float64ValueOrDefault(value *float64, fallback float64) float64 {
	if value == nil {
		return fallback
REDACTED
	return *value
REDACTED

func intValueOrDefault(value *int, fallback int) int {
	if value == nil {
		return fallback
REDACTED
	return *value
REDACTED

func boolValueOrDefault(value *bool, fallback bool) bool {
	if value == nil {
		return fallback
REDACTED
	return *value
REDACTED

func defaultSubscriptionsValueOrDefault(input *[]dto.DefaultSubscriptionSetting, fallback []service.DefaultSubscriptionSetting) []service.DefaultSubscriptionSetting {
	if input == nil {
		return fallback
REDACTED
	result := make([]service.DefaultSubscriptionSetting, 0, len(*input))
	for _, item := range *input {
		result = append(result, service.DefaultSubscriptionSetting{
			GroupID:      item.GroupID,
			ValidityDays: item.ValidityDays,
	REDACTED)
REDACTED
	return result
REDACTED

func systemSettingsResponseData(settings dto.SystemSettings, authSourceDefaults *service.AuthSourceDefaultSettings) map[string]any {
	data := make(map[string]any)
	raw, err := json.Marshal(settings)
	if err == nil {
		_ = json.Unmarshal(raw, &data)
REDACTED
	if authSourceDefaults == nil {
		authSourceDefaults = &service.AuthSourceDefaultSettings{REDACTED
REDACTED

	data["auth_source_default_email_balance"] = authSourceDefaults.Email.Balance
	data["auth_source_default_email_concurrency"] = authSourceDefaults.Email.Concurrency
	data["auth_source_default_email_subscriptions"] = authSourceDefaults.Email.Subscriptions
	data["auth_source_default_email_grant_on_signup"] = authSourceDefaults.Email.GrantOnSignup
	data["auth_source_default_email_grant_on_first_bind"] = authSourceDefaults.Email.GrantOnFirstBind
	data["auth_source_default_linuxdo_balance"] = authSourceDefaults.LinuxDo.Balance
	data["auth_source_default_linuxdo_concurrency"] = authSourceDefaults.LinuxDo.Concurrency
	data["auth_source_default_linuxdo_subscriptions"] = authSourceDefaults.LinuxDo.Subscriptions
	data["auth_source_default_linuxdo_grant_on_signup"] = authSourceDefaults.LinuxDo.GrantOnSignup
	data["auth_source_default_linuxdo_grant_on_first_bind"] = authSourceDefaults.LinuxDo.GrantOnFirstBind
	data["auth_source_default_oidc_balance"] = authSourceDefaults.OIDC.Balance
	data["auth_source_default_oidc_concurrency"] = authSourceDefaults.OIDC.Concurrency
	data["auth_source_default_oidc_subscriptions"] = authSourceDefaults.OIDC.Subscriptions
	data["auth_source_default_oidc_grant_on_signup"] = authSourceDefaults.OIDC.GrantOnSignup
	data["auth_source_default_oidc_grant_on_first_bind"] = authSourceDefaults.OIDC.GrantOnFirstBind
	data["auth_source_default_wechat_balance"] = authSourceDefaults.WeChat.Balance
	data["auth_source_default_wechat_concurrency"] = authSourceDefaults.WeChat.Concurrency
	data["auth_source_default_wechat_subscriptions"] = authSourceDefaults.WeChat.Subscriptions
	data["auth_source_default_wechat_grant_on_signup"] = authSourceDefaults.WeChat.GrantOnSignup
	data["auth_source_default_wechat_grant_on_first_bind"] = authSourceDefaults.WeChat.GrantOnFirstBind
	data["force_email_on_third_party_signup"] = authSourceDefaults.ForceEmailOnThirdPartySignup

	return data
REDACTED

func equalStringSlice(a, b []string) bool {
	if len(a) != len(b) {
		return false
REDACTED
	for i := range a {
		if a[i] != b[i] {
			return false
	REDACTED
REDACTED
	return true
REDACTED

func equalDefaultSubscriptions(a, b []service.DefaultSubscriptionSetting) bool {
	if len(a) != len(b) {
		return false
REDACTED
	for i := range a {
		if a[i].GroupID != b[i].GroupID || a[i].ValidityDays != b[i].ValidityDays {
			return false
	REDACTED
REDACTED
	return true
REDACTED

func equalIntSlice(a, b []int) bool {
	if len(a) != len(b) {
		return false
REDACTED
	for i := range a {
		if a[i] != b[i] {
			return false
	REDACTED
REDACTED
	return true
REDACTED

func equalNotifyEmailEntries(a, b []service.NotifyEmailEntry) bool {
	if len(a) != len(b) {
		return false
REDACTED
	for i := range a {
		if a[i].Email != b[i].Email || a[i].Verified != b[i].Verified || a[i].Disabled != b[i].Disabled {
			return false
	REDACTED
REDACTED
	return true
REDACTED

// TestSMTPRequest 测试SMTP连接请求
type TestSMTPRequest struct {
	SMTPHost     string `json:"smtp_host"`
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

	req.SMTPHost = strings.TrimSpace(req.SMTPHost)
	req.SMTPUsername = strings.TrimSpace(req.SMTPUsername)

	var savedConfig *service.SMTPConfig
	if cfg, err := h.emailService.GetSMTPConfig(c.Request.Context()); err == nil && cfg != nil {
		savedConfig = cfg
REDACTED

	if req.SMTPHost == "" && savedConfig != nil {
		req.SMTPHost = savedConfig.Host
REDACTED
	if req.SMTPPort <= 0 {
		if savedConfig != nil && savedConfig.Port > 0 {
			req.SMTPPort = savedConfig.Port
	REDACTED else {
			req.SMTPPort = 587
	REDACTED
REDACTED
	if req.SMTPUsername == "" && savedConfig != nil {
		req.SMTPUsername = savedConfig.Username
REDACTED
	password := strings.TrimSpace(req.SMTPPassword)
	if password == "" && savedConfig != nil {
		password = savedConfig.Password
REDACTED
	if req.SMTPHost == "" {
		response.BadRequest(c, "SMTP host is required")
		return
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
		response.BadRequest(c, "SMTP connection test failed: "+err.Error())
		return
REDACTED

	response.Success(c, gin.H{"message": "SMTP connection successful"REDACTED)
REDACTED

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
REDACTED

// SendTestEmail 发送测试邮件
// POST /api/v1/admin/settings/send-test-email
func (h *SettingHandler) SendTestEmail(c *gin.Context) {
	var req SendTestEmailRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
REDACTED

	req.SMTPHost = strings.TrimSpace(req.SMTPHost)
	req.SMTPUsername = strings.TrimSpace(req.SMTPUsername)
	req.SMTPFrom = strings.TrimSpace(req.SMTPFrom)
	req.SMTPFromName = strings.TrimSpace(req.SMTPFromName)

	var savedConfig *service.SMTPConfig
	if cfg, err := h.emailService.GetSMTPConfig(c.Request.Context()); err == nil && cfg != nil {
		savedConfig = cfg
REDACTED

	if req.SMTPHost == "" && savedConfig != nil {
		req.SMTPHost = savedConfig.Host
REDACTED
	if req.SMTPPort <= 0 {
		if savedConfig != nil && savedConfig.Port > 0 {
			req.SMTPPort = savedConfig.Port
	REDACTED else {
			req.SMTPPort = 587
	REDACTED
REDACTED
	if req.SMTPUsername == "" && savedConfig != nil {
		req.SMTPUsername = savedConfig.Username
REDACTED
	password := strings.TrimSpace(req.SMTPPassword)
	if password == "" && savedConfig != nil {
		password = savedConfig.Password
REDACTED
	if req.SMTPFrom == "" && savedConfig != nil {
		req.SMTPFrom = savedConfig.From
REDACTED
	if req.SMTPFromName == "" && savedConfig != nil {
		req.SMTPFromName = savedConfig.FromName
REDACTED
	if req.SMTPHost == "" {
		response.BadRequest(c, "SMTP host is required")
		return
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
		response.BadRequest(c, "Failed to send test email: "+err.Error())
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

// GetOverloadCooldownSettings 获取529过载冷却配置
// GET /api/v1/admin/settings/overload-cooldown
func (h *SettingHandler) GetOverloadCooldownSettings(c *gin.Context) {
	settings, err := h.settingService.GetOverloadCooldownSettings(c.Request.Context())
	if err != nil {
		response.ErrorFrom(c, err)
		return
REDACTED

	response.Success(c, dto.OverloadCooldownSettings{
		Enabled:         settings.Enabled,
		CooldownMinutes: settings.CooldownMinutes,
REDACTED)
REDACTED

// UpdateOverloadCooldownSettingsRequest 更新529过载冷却配置请求
type UpdateOverloadCooldownSettingsRequest struct {
	Enabled         bool `json:"enabled"`
	CooldownMinutes int  `json:"cooldown_minutes"`
REDACTED

// UpdateOverloadCooldownSettings 更新529过载冷却配置
// PUT /api/v1/admin/settings/overload-cooldown
func (h *SettingHandler) UpdateOverloadCooldownSettings(c *gin.Context) {
	var req UpdateOverloadCooldownSettingsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
REDACTED

	settings := &service.OverloadCooldownSettings{
		Enabled:         req.Enabled,
		CooldownMinutes: req.CooldownMinutes,
REDACTED

	if err := h.settingService.SetOverloadCooldownSettings(c.Request.Context(), settings); err != nil {
		response.BadRequest(c, err.Error())
		return
REDACTED

	updatedSettings, err := h.settingService.GetOverloadCooldownSettings(c.Request.Context())
	if err != nil {
		response.ErrorFrom(c, err)
		return
REDACTED

	response.Success(c, dto.OverloadCooldownSettings{
		Enabled:         updatedSettings.Enabled,
		CooldownMinutes: updatedSettings.CooldownMinutes,
REDACTED)
REDACTED

// GetRateLimit429CooldownSettings 获取429默认回避配置
// GET /api/v1/admin/settings/rate-limit-429-cooldown
func (h *SettingHandler) GetRateLimit429CooldownSettings(c *gin.Context) {
	settings, err := h.settingService.GetRateLimit429CooldownSettings(c.Request.Context())
	if err != nil {
		response.ErrorFrom(c, err)
		return
REDACTED

	response.Success(c, dto.RateLimit429CooldownSettings{
		Enabled:         settings.Enabled,
		CooldownSeconds: settings.CooldownSeconds,
REDACTED)
REDACTED

// UpdateRateLimit429CooldownSettingsRequest 更新429默认回避配置请求
type UpdateRateLimit429CooldownSettingsRequest struct {
	Enabled         bool `json:"enabled"`
	CooldownSeconds int  `json:"cooldown_seconds"`
REDACTED

// UpdateRateLimit429CooldownSettings 更新429默认回避配置
// PUT /api/v1/admin/settings/rate-limit-429-cooldown
func (h *SettingHandler) UpdateRateLimit429CooldownSettings(c *gin.Context) {
	var req UpdateRateLimit429CooldownSettingsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
REDACTED

	settings := &service.RateLimit429CooldownSettings{
		Enabled:         req.Enabled,
		CooldownSeconds: req.CooldownSeconds,
REDACTED

	if err := h.settingService.SetRateLimit429CooldownSettings(c.Request.Context(), settings); err != nil {
		response.BadRequest(c, err.Error())
		return
REDACTED

	updatedSettings, err := h.settingService.GetRateLimit429CooldownSettings(c.Request.Context())
	if err != nil {
		response.ErrorFrom(c, err)
		return
REDACTED

	response.Success(c, dto.RateLimit429CooldownSettings{
		Enabled:         updatedSettings.Enabled,
		CooldownSeconds: updatedSettings.CooldownSeconds,
REDACTED)
REDACTED

// GetStreamTimeoutSettings 获取流超时处理配置
// GET /api/v1/admin/settings/stream-timeout
func (h *SettingHandler) GetStreamTimeoutSettings(c *gin.Context) {
	settings, err := h.settingService.GetStreamTimeoutSettings(c.Request.Context())
	if err != nil {
		response.ErrorFrom(c, err)
		return
REDACTED

	response.Success(c, dto.StreamTimeoutSettings{
		Enabled:                settings.Enabled,
		Action:                 settings.Action,
		TempUnschedMinutes:     settings.TempUnschedMinutes,
		ThresholdCount:         settings.ThresholdCount,
		ThresholdWindowMinutes: settings.ThresholdWindowMinutes,
REDACTED)
REDACTED

// GetRectifierSettings 获取请求整流器配置
// GET /api/v1/admin/settings/rectifier
func (h *SettingHandler) GetRectifierSettings(c *gin.Context) {
	settings, err := h.settingService.GetRectifierSettings(c.Request.Context())
	if err != nil {
		response.ErrorFrom(c, err)
		return
REDACTED

	patterns := settings.APIKeySignaturePatterns
	if patterns == nil {
		patterns = []string{REDACTED
REDACTED
	response.Success(c, dto.RectifierSettings{
		Enabled:                  settings.Enabled,
		ThinkingSignatureEnabled: settings.ThinkingSignatureEnabled,
		ThinkingBudgetEnabled:    settings.ThinkingBudgetEnabled,
		APIKeySignatureEnabled:   settings.APIKeySignatureEnabled,
		APIKeySignaturePatterns:  patterns,
REDACTED)
REDACTED

// UpdateRectifierSettingsRequest 更新整流器配置请求
type UpdateRectifierSettingsRequest struct {
	Enabled                  bool     `json:"enabled"`
	ThinkingSignatureEnabled bool     `json:"thinking_signature_enabled"`
	ThinkingBudgetEnabled    bool     `json:"thinking_budget_enabled"`
	APIKeySignatureEnabled   bool     `json:"apikey_signature_enabled"`
	APIKeySignaturePatterns  []string `json:"apikey_signature_patterns"`
REDACTED

// UpdateRectifierSettings 更新请求整流器配置
// PUT /api/v1/admin/settings/rectifier
func (h *SettingHandler) UpdateRectifierSettings(c *gin.Context) {
	var req UpdateRectifierSettingsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
REDACTED

	// 校验并清理自定义匹配关键词
	const maxPatterns = 50
	const maxPatternLen = 500
	if len(req.APIKeySignaturePatterns) > maxPatterns {
		response.BadRequest(c, "Too many signature patterns (max 50)")
		return
REDACTED
	var cleanedPatterns []string
	for _, p := range req.APIKeySignaturePatterns {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
	REDACTED
		if len(p) > maxPatternLen {
			response.BadRequest(c, "Signature pattern too long (max 500 characters)")
			return
	REDACTED
		cleanedPatterns = append(cleanedPatterns, p)
REDACTED

	settings := &service.RectifierSettings{
		Enabled:                  req.Enabled,
		ThinkingSignatureEnabled: req.ThinkingSignatureEnabled,
		ThinkingBudgetEnabled:    req.ThinkingBudgetEnabled,
		APIKeySignatureEnabled:   req.APIKeySignatureEnabled,
		APIKeySignaturePatterns:  cleanedPatterns,
REDACTED

	if err := h.settingService.SetRectifierSettings(c.Request.Context(), settings); err != nil {
		response.BadRequest(c, err.Error())
		return
REDACTED

	// 重新获取设置返回
	updatedSettings, err := h.settingService.GetRectifierSettings(c.Request.Context())
	if err != nil {
		response.ErrorFrom(c, err)
		return
REDACTED

	updatedPatterns := updatedSettings.APIKeySignaturePatterns
	if updatedPatterns == nil {
		updatedPatterns = []string{REDACTED
REDACTED
	response.Success(c, dto.RectifierSettings{
		Enabled:                  updatedSettings.Enabled,
		ThinkingSignatureEnabled: updatedSettings.ThinkingSignatureEnabled,
		ThinkingBudgetEnabled:    updatedSettings.ThinkingBudgetEnabled,
		APIKeySignatureEnabled:   updatedSettings.APIKeySignatureEnabled,
		APIKeySignaturePatterns:  updatedPatterns,
REDACTED)
REDACTED

// GetBetaPolicySettings 获取 Beta 策略配置
// GET /api/v1/admin/settings/beta-policy
func (h *SettingHandler) GetBetaPolicySettings(c *gin.Context) {
	settings, err := h.settingService.GetBetaPolicySettings(c.Request.Context())
	if err != nil {
		response.ErrorFrom(c, err)
		return
REDACTED

	rules := make([]dto.BetaPolicyRule, len(settings.Rules))
	for i, r := range settings.Rules {
		rules[i] = dto.BetaPolicyRule(r)
REDACTED
	response.Success(c, dto.BetaPolicySettings{Rules: rulesREDACTED)
REDACTED

// UpdateBetaPolicySettingsRequest 更新 Beta 策略配置请求
type UpdateBetaPolicySettingsRequest struct {
	Rules []dto.BetaPolicyRule `json:"rules"`
REDACTED

// UpdateBetaPolicySettings 更新 Beta 策略配置
// PUT /api/v1/admin/settings/beta-policy
func (h *SettingHandler) UpdateBetaPolicySettings(c *gin.Context) {
	var req UpdateBetaPolicySettingsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
REDACTED

	rules := make([]service.BetaPolicyRule, len(req.Rules))
	for i, r := range req.Rules {
		rules[i] = service.BetaPolicyRule(r)
REDACTED

	settings := &service.BetaPolicySettings{Rules: rulesREDACTED
	if err := h.settingService.SetBetaPolicySettings(c.Request.Context(), settings); err != nil {
		response.BadRequest(c, err.Error())
		return
REDACTED

	// Re-fetch to return updated settings
	updated, err := h.settingService.GetBetaPolicySettings(c.Request.Context())
	if err != nil {
		response.ErrorFrom(c, err)
		return
REDACTED

	outRules := make([]dto.BetaPolicyRule, len(updated.Rules))
	for i, r := range updated.Rules {
		outRules[i] = dto.BetaPolicyRule(r)
REDACTED
	response.Success(c, dto.BetaPolicySettings{Rules: outRulesREDACTED)
REDACTED

// UpdateStreamTimeoutSettingsRequest 更新流超时配置请求
type UpdateStreamTimeoutSettingsRequest struct {
	Enabled                bool   `json:"enabled"`
	Action                 string `json:"action"`
	TempUnschedMinutes     int    `json:"temp_unsched_minutes"`
	ThresholdCount         int    `json:"threshold_count"`
	ThresholdWindowMinutes int    `json:"threshold_window_minutes"`
REDACTED

// UpdateStreamTimeoutSettings 更新流超时处理配置
// PUT /api/v1/admin/settings/stream-timeout
func (h *SettingHandler) UpdateStreamTimeoutSettings(c *gin.Context) {
	var req UpdateStreamTimeoutSettingsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
REDACTED

	settings := &service.StreamTimeoutSettings{
		Enabled:                req.Enabled,
		Action:                 req.Action,
		TempUnschedMinutes:     req.TempUnschedMinutes,
		ThresholdCount:         req.ThresholdCount,
		ThresholdWindowMinutes: req.ThresholdWindowMinutes,
REDACTED

	if err := h.settingService.SetStreamTimeoutSettings(c.Request.Context(), settings); err != nil {
		response.BadRequest(c, err.Error())
		return
REDACTED

	// 重新获取设置返回
	updatedSettings, err := h.settingService.GetStreamTimeoutSettings(c.Request.Context())
	if err != nil {
		response.ErrorFrom(c, err)
		return
REDACTED

	response.Success(c, dto.StreamTimeoutSettings{
		Enabled:                updatedSettings.Enabled,
		Action:                 updatedSettings.Action,
		TempUnschedMinutes:     updatedSettings.TempUnschedMinutes,
		ThresholdCount:         updatedSettings.ThresholdCount,
		ThresholdWindowMinutes: updatedSettings.ThresholdWindowMinutes,
REDACTED)
REDACTED

// GetWebSearchEmulationConfig 获取 Web Search 模拟配置
// GET /api/v1/admin/settings/web-search-emulation
func (h *SettingHandler) GetWebSearchEmulationConfig(c *gin.Context) {
	cfg, err := h.settingService.GetWebSearchEmulationConfig(c.Request.Context())
	if err != nil {
		response.ErrorFrom(c, err)
		return
REDACTED
	response.Success(c, service.PopulateWebSearchUsage(c.Request.Context(), cfg))
REDACTED

// UpdateWebSearchEmulationConfig 更新 Web Search 模拟配置
// PUT /api/v1/admin/settings/web-search-emulation
func (h *SettingHandler) UpdateWebSearchEmulationConfig(c *gin.Context) {
	var cfg service.WebSearchEmulationConfig
	if err := c.ShouldBindJSON(&cfg); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
REDACTED

	if err := h.settingService.SaveWebSearchEmulationConfig(c.Request.Context(), &cfg); err != nil {
		response.ErrorFrom(c, err)
		return
REDACTED

	// Re-read (with sanitized api keys) to return current state
	updated, err := h.settingService.GetWebSearchEmulationConfig(c.Request.Context())
	if err != nil {
		response.ErrorFrom(c, err)
		return
REDACTED
	response.Success(c, service.PopulateWebSearchUsage(c.Request.Context(), updated))
REDACTED

// ResetWebSearchUsage 重置指定 provider 的配额用量
// POST /api/v1/admin/settings/web-search-emulation/reset-usage
func (h *SettingHandler) ResetWebSearchUsage(c *gin.Context) {
	var req struct {
		ProviderType string `json:"provider_type"`
REDACTED
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
REDACTED
	if req.ProviderType == "" {
		response.BadRequest(c, "provider_type is required")
		return
REDACTED
	if err := service.ResetWebSearchUsage(c.Request.Context(), req.ProviderType); err != nil {
		response.ErrorFrom(c, err)
		return
REDACTED
	response.Success(c, nil)
REDACTED

// TestWebSearchEmulation 测试 Web Search 搜索
// POST /api/v1/admin/settings/web-search-emulation/test
func (h *SettingHandler) TestWebSearchEmulation(c *gin.Context) {
	var req struct {
		Query string `json:"query"`
REDACTED
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
REDACTED
	if strings.TrimSpace(req.Query) == "" {
		req.Query = "搜索今年世界大事件"
REDACTED

	result, err := service.TestWebSearch(c.Request.Context(), req.Query)
	if err != nil {
		response.ErrorFrom(c, err)
		return
REDACTED
	response.Success(c, result)
REDACTED
