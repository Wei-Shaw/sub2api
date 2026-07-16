package admin

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/handler/dto"
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/gin-gonic/gin"
)

// UpdateSettingsRequest 更新设置请求
type UpdateSettingsRequest struct {
	// 注册设置
	RegistrationEnabled              bool                         `json:"registration_enabled"`
	EmailVerifyEnabled               bool                         `json:"email_verify_enabled"`
	RegistrationEmailSuffixWhitelist []string                     `json:"registration_email_suffix_whitelist"`
	PromoCodeEnabled                 bool                         `json:"promo_code_enabled"`
	PasswordResetEnabled             bool                         `json:"password_reset_enabled"`
	FrontendURL                      string                       `json:"frontend_url"`
	InvitationCodeEnabled            bool                         `json:"invitation_code_enabled"`
	TotpEnabled                      bool                         `json:"totp_enabled"`             // TOTP 双因素认证
	SessionBindingEnabled            bool                         `json:"session_binding_enabled"`  // 会话 IP/UA 绑定
	AuditLogRetentionDays            int                          `json:"audit_log_retention_days"` // 审计日志保留天数
	LoginAgreementEnabled            bool                         `json:"login_agreement_enabled"`
	LoginAgreementMode               string                       `json:"login_agreement_mode"`
	LoginAgreementUpdatedAt          string                       `json:"login_agreement_updated_at"`
	LoginAgreementDocuments          []dto.LoginAgreementDocument `json:"login_agreement_documents"`

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

	// API Key IP 访问控制设置
	APIKeyACLTrustForwardedIP *bool `json:"api_key_acl_trust_forwarded_ip"`

	// LinuxDo Connect OAuth 登录
	LinuxDoConnectEnabled      bool   `json:"linuxdo_connect_enabled"`
	LinuxDoConnectClientID     string `json:"linuxdo_connect_client_id"`
	LinuxDoConnectClientSecret string `json:"linuxdo_connect_client_secret"`
	LinuxDoConnectRedirectURL  string `json:"linuxdo_connect_redirect_url"`

	// DingTalk Connect OAuth 登录
	DingTalkConnectEnabled                 bool   `json:"dingtalk_connect_enabled"`
	DingTalkConnectClientID                string `json:"dingtalk_connect_client_id"`
	DingTalkConnectClientSecret            string `json:"dingtalk_connect_client_secret"`
	DingTalkConnectRedirectURL             string `json:"dingtalk_connect_redirect_url"`
	DingTalkConnectCorpRestrictionPolicy   string `json:"dingtalk_connect_corp_restriction_policy"`
	DingTalkConnectInternalCorpID          string `json:"dingtalk_connect_internal_corp_id"`
	DingTalkConnectBypassRegistration      bool   `json:"dingtalk_connect_bypass_registration"`
	DingTalkConnectSyncCorpEmail           bool   `json:"dingtalk_connect_sync_corp_email"`
	DingTalkConnectSyncDisplayName         bool   `json:"dingtalk_connect_sync_display_name"`
	DingTalkConnectSyncDept                bool   `json:"dingtalk_connect_sync_dept"`
	DingTalkConnectSyncCorpEmailAttrKey    string `json:"dingtalk_connect_sync_corp_email_attr_key"`
	DingTalkConnectSyncDisplayNameAttrKey  string `json:"dingtalk_connect_sync_display_name_attr_key"`
	DingTalkConnectSyncDeptAttrKey         string `json:"dingtalk_connect_sync_dept_attr_key"`
	DingTalkConnectSyncCorpEmailAttrName   string `json:"dingtalk_connect_sync_corp_email_attr_name"`
	DingTalkConnectSyncDisplayNameAttrName string `json:"dingtalk_connect_sync_display_name_attr_name"`
	DingTalkConnectSyncDeptAttrName        string `json:"dingtalk_connect_sync_dept_attr_name"`

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

	GitHubOAuthEnabled             bool   `json:"github_oauth_enabled"`
	GitHubOAuthClientID            string `json:"github_oauth_client_id"`
	GitHubOAuthClientSecret        string `json:"github_oauth_client_secret"`
	GitHubOAuthRedirectURL         string `json:"github_oauth_redirect_url"`
	GitHubOAuthFrontendRedirectURL string `json:"github_oauth_frontend_redirect_url"`
	GoogleOAuthEnabled             bool   `json:"google_oauth_enabled"`
	GoogleOAuthClientID            string `json:"google_oauth_client_id"`
	GoogleOAuthClientSecret        string `json:"google_oauth_client_secret"`
	GoogleOAuthRedirectURL         string `json:"google_oauth_redirect_url"`
	GoogleOAuthFrontendRedirectURL string `json:"google_oauth_frontend_redirect_url"`

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
	DefaultConcurrency                        int                               `json:"default_concurrency"`
	DefaultBalance                            float64                           `json:"default_balance"`
	AffiliateRebateRate                       *float64                          `json:"affiliate_rebate_rate"`
	AffiliateRebateFreezeHours                *int                              `json:"affiliate_rebate_freeze_hours"`
	AffiliateRebateDurationDays               *int                              `json:"affiliate_rebate_duration_days"`
	AffiliateRebatePerInviteeCap              *float64                          `json:"affiliate_rebate_per_invitee_cap"`
	AdminRechargeRebateEnabled                *bool                             `json:"affiliate_admin_recharge_enabled"`
	DefaultUserRPMLimit                       int                               `json:"default_user_rpm_limit"`
	DefaultSubscriptions                      []dto.DefaultSubscriptionSetting  `json:"default_subscriptions"`
	AuthSourceDefaultEmailBalance             *float64                          `json:"auth_source_default_email_balance"`
	AuthSourceDefaultEmailConcurrency         *int                              `json:"auth_source_default_email_concurrency"`
	AuthSourceDefaultEmailSubscriptions       *[]dto.DefaultSubscriptionSetting `json:"auth_source_default_email_subscriptions"`
	AuthSourceDefaultEmailGrantOnSignup       *bool                             `json:"auth_source_default_email_grant_on_signup"`
	AuthSourceDefaultEmailGrantOnFirstBind    *bool                             `json:"auth_source_default_email_grant_on_first_bind"`
	AuthSourceDefaultLinuxDoBalance           *float64                          `json:"auth_source_default_linuxdo_balance"`
	AuthSourceDefaultLinuxDoConcurrency       *int                              `json:"auth_source_default_linuxdo_concurrency"`
	AuthSourceDefaultLinuxDoSubscriptions     *[]dto.DefaultSubscriptionSetting `json:"auth_source_default_linuxdo_subscriptions"`
	AuthSourceDefaultLinuxDoGrantOnSignup     *bool                             `json:"auth_source_default_linuxdo_grant_on_signup"`
	AuthSourceDefaultLinuxDoGrantOnFirstBind  *bool                             `json:"auth_source_default_linuxdo_grant_on_first_bind"`
	AuthSourceDefaultOIDCBalance              *float64                          `json:"auth_source_default_oidc_balance"`
	AuthSourceDefaultOIDCConcurrency          *int                              `json:"auth_source_default_oidc_concurrency"`
	AuthSourceDefaultOIDCSubscriptions        *[]dto.DefaultSubscriptionSetting `json:"auth_source_default_oidc_subscriptions"`
	AuthSourceDefaultOIDCGrantOnSignup        *bool                             `json:"auth_source_default_oidc_grant_on_signup"`
	AuthSourceDefaultOIDCGrantOnFirstBind     *bool                             `json:"auth_source_default_oidc_grant_on_first_bind"`
	AuthSourceDefaultWeChatBalance            *float64                          `json:"auth_source_default_wechat_balance"`
	AuthSourceDefaultWeChatConcurrency        *int                              `json:"auth_source_default_wechat_concurrency"`
	AuthSourceDefaultWeChatSubscriptions      *[]dto.DefaultSubscriptionSetting `json:"auth_source_default_wechat_subscriptions"`
	AuthSourceDefaultWeChatGrantOnSignup      *bool                             `json:"auth_source_default_wechat_grant_on_signup"`
	AuthSourceDefaultWeChatGrantOnFirstBind   *bool                             `json:"auth_source_default_wechat_grant_on_first_bind"`
	AuthSourceDefaultGitHubBalance            *float64                          `json:"auth_source_default_github_balance"`
	AuthSourceDefaultGitHubConcurrency        *int                              `json:"auth_source_default_github_concurrency"`
	AuthSourceDefaultGitHubSubscriptions      *[]dto.DefaultSubscriptionSetting `json:"auth_source_default_github_subscriptions"`
	AuthSourceDefaultGitHubGrantOnSignup      *bool                             `json:"auth_source_default_github_grant_on_signup"`
	AuthSourceDefaultGitHubGrantOnFirstBind   *bool                             `json:"auth_source_default_github_grant_on_first_bind"`
	AuthSourceDefaultGoogleBalance            *float64                          `json:"auth_source_default_google_balance"`
	AuthSourceDefaultGoogleConcurrency        *int                              `json:"auth_source_default_google_concurrency"`
	AuthSourceDefaultGoogleSubscriptions      *[]dto.DefaultSubscriptionSetting `json:"auth_source_default_google_subscriptions"`
	AuthSourceDefaultGoogleGrantOnSignup      *bool                             `json:"auth_source_default_google_grant_on_signup"`
	AuthSourceDefaultGoogleGrantOnFirstBind   *bool                             `json:"auth_source_default_google_grant_on_first_bind"`
	AuthSourceDefaultDingTalkBalance          *float64                          `json:"auth_source_default_dingtalk_balance"`
	AuthSourceDefaultDingTalkConcurrency      *int                              `json:"auth_source_default_dingtalk_concurrency"`
	AuthSourceDefaultDingTalkSubscriptions    *[]dto.DefaultSubscriptionSetting `json:"auth_source_default_dingtalk_subscriptions"`
	AuthSourceDefaultDingTalkGrantOnSignup    *bool                             `json:"auth_source_default_dingtalk_grant_on_signup"`
	AuthSourceDefaultDingTalkGrantOnFirstBind *bool                             `json:"auth_source_default_dingtalk_grant_on_first_bind"`
	ForceEmailOnThirdPartySignup              *bool                             `json:"force_email_on_third_party_signup"`

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
	EnableFingerprintUnification           *bool   `json:"enable_fingerprint_unification"`
	EnableMetadataPassthrough              *bool   `json:"enable_metadata_passthrough"`
	EnableCCHSigning                       *bool   `json:"enable_cch_signing"`
	EnableClaudeOAuthSystemPromptInjection *bool   `json:"enable_claude_oauth_system_prompt_injection"`
	ClaudeOAuthSystemPrompt                *string `json:"claude_oauth_system_prompt"`
	ClaudeOAuthSystemPromptBlocks          *string `json:"claude_oauth_system_prompt_blocks"`
	EnableAnthropicCacheTTL1hInjection     *bool   `json:"enable_anthropic_cache_ttl_1h_injection"`
	RewriteMessageCacheControl             *bool   `json:"rewrite_message_cache_control"`
	EnableClientDatelineNormalization      *bool   `json:"enable_client_dateline_normalization"`
	AntigravityUserAgentVersion            *string `json:"antigravity_user_agent_version"`
	OpenAICodexUserAgent                   *string `json:"openai_codex_user_agent"`

	// codex_cli_only 加固（global-only）
	MinCodexVersion                      string `json:"min_codex_version"`
	MaxCodexVersion                      string `json:"max_codex_version"`
	CodexCLIOnlyBlacklist                string `json:"codex_cli_only_blacklist"`
	CodexCLIOnlyWhitelist                string `json:"codex_cli_only_whitelist"`
	CodexCLIOnlyAllowAppServerClients    *bool  `json:"codex_cli_only_allow_app_server_clients"`
	CodexCLIOnlyEngineFingerprintSignals string `json:"codex_cli_only_engine_fingerprint_signals"`

	// Payment visible method routing
	PaymentVisibleMethodAlipaySource  *string `json:"payment_visible_method_alipay_source"`
	PaymentVisibleMethodWxpaySource   *string `json:"payment_visible_method_wxpay_source"`
	PaymentVisibleMethodAlipayEnabled *bool   `json:"payment_visible_method_alipay_enabled"`
	PaymentVisibleMethodWxpayEnabled  *bool   `json:"payment_visible_method_wxpay_enabled"`

	// OpenAI account scheduling
	OpenAILowUpstreamRatePriorityEnabled               *bool    `json:"openai_low_upstream_rate_priority_enabled"`
	OpenAIOAuthSchedulingRateMultiplier                *float64 `json:"openai_oauth_scheduling_rate_multiplier"`
	OpenAIAdvancedSchedulerEnabled                     *bool    `json:"openai_advanced_scheduler_enabled"`
	OpenAIAdvancedSchedulerStickyWeightedEnabled       *bool    `json:"openai_advanced_scheduler_sticky_weighted_enabled"`
	OpenAIAdvancedSchedulerSubscriptionPriorityEnabled *bool    `json:"openai_advanced_scheduler_subscription_priority_enabled"`
	OpenAIAdvancedSchedulerLBTopK                      *string  `json:"openai_advanced_scheduler_lb_top_k"`
	OpenAIAdvancedSchedulerWeightPriority              *string  `json:"openai_advanced_scheduler_weight_priority"`
	OpenAIAdvancedSchedulerWeightLoad                  *string  `json:"openai_advanced_scheduler_weight_load"`
	OpenAIAdvancedSchedulerWeightQueue                 *string  `json:"openai_advanced_scheduler_weight_queue"`
	OpenAIAdvancedSchedulerWeightErrorRate             *string  `json:"openai_advanced_scheduler_weight_error_rate"`
	OpenAIAdvancedSchedulerWeightTTFT                  *string  `json:"openai_advanced_scheduler_weight_ttft"`
	OpenAIAdvancedSchedulerWeightReset                 *string  `json:"openai_advanced_scheduler_weight_reset"`
	OpenAIAdvancedSchedulerWeightQuotaHeadroom         *string  `json:"openai_advanced_scheduler_weight_quota_headroom"`
	OpenAIAdvancedSchedulerWeightUpstreamCost          *string  `json:"openai_advanced_scheduler_weight_upstream_cost"`
	OpenAIAdvancedSchedulerWeightPreviousResponse      *string  `json:"openai_advanced_scheduler_weight_previous_response"`
	OpenAIAdvancedSchedulerWeightSessionSticky         *string  `json:"openai_advanced_scheduler_weight_session_sticky"`

	// 余额不足提醒
	BalanceLowNotifyEnabled         *bool                   `json:"balance_low_notify_enabled"`
	BalanceLowNotifyThreshold       *float64                `json:"balance_low_notify_threshold"`
	BalanceLowNotifyRechargeURL     *string                 `json:"balance_low_notify_recharge_url"`
	SubscriptionExpiryNotifyEnabled *bool                   `json:"subscription_expiry_notify_enabled"`
	AccountQuotaNotifyEnabled       *bool                   `json:"account_quota_notify_enabled"`
	AccountQuotaNotifyEmails        *[]dto.NotifyEmailEntry `json:"account_quota_notify_emails"`

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
	PaymentSubscriptionUSDToCNYRate  *float64 `json:"payment_subscription_usd_to_cny_rate"`
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

	// Force Alipay mobile clients to use QR code payment instead of mobile redirect
	PaymentAlipayForceQRCode *bool `json:"payment_alipay_force_qrcode"`

	// Channel Monitor feature switch
	ChannelMonitorEnabled                *bool `json:"channel_monitor_enabled"`
	ChannelMonitorDefaultIntervalSeconds *int  `json:"channel_monitor_default_interval_seconds"`

	// Available Channels feature switch (user-facing)
	AvailableChannelsEnabled *bool `json:"available_channels_enabled"`

	// Affiliate (邀请返利) feature switch
	AffiliateEnabled *bool `json:"affiliate_enabled"`

	// 风控中心功能开关
	RiskControlEnabled *bool `json:"risk_control_enabled"`

	// cyber 会话屏蔽开关 + TTL
	CyberSessionBlockEnabled    *bool `json:"cyber_session_block_enabled"`
	CyberSessionBlockTTLSeconds *int  `json:"cyber_session_block_ttl_seconds"`

	// OpenAI fast/flex policy (optional, only updated when provided)
	OpenAIFastPolicySettings *dto.OpenAIFastPolicySettings `json:"openai_fast_policy_settings,omitempty"`

	// 系统全局 platform quota 默认值（整体替换语义：nil = 不修改，non-nil = 整体覆盖）。
	DefaultPlatformQuotas map[string]*service.DefaultPlatformQuotaSetting `json:"default_platform_quotas"`

	// auth-source 层 platform quota 覆盖（override 语义：nil = 不修改，non-nil = 整体覆盖该 source 的 quota 配置）。
	AuthSourceEmailPlatformQuotas    map[string]*service.DefaultPlatformQuotaSetting `json:"auth_source_default_email_platform_quotas"`
	AuthSourceLinuxDoPlatformQuotas  map[string]*service.DefaultPlatformQuotaSetting `json:"auth_source_default_linuxdo_platform_quotas"`
	AuthSourceOIDCPlatformQuotas     map[string]*service.DefaultPlatformQuotaSetting `json:"auth_source_default_oidc_platform_quotas"`
	AuthSourceWeChatPlatformQuotas   map[string]*service.DefaultPlatformQuotaSetting `json:"auth_source_default_wechat_platform_quotas"`
	AuthSourceGitHubPlatformQuotas   map[string]*service.DefaultPlatformQuotaSetting `json:"auth_source_default_github_platform_quotas"`
	AuthSourceGooglePlatformQuotas   map[string]*service.DefaultPlatformQuotaSetting `json:"auth_source_default_google_platform_quotas"`
	AuthSourceDingTalkPlatformQuotas map[string]*service.DefaultPlatformQuotaSetting `json:"auth_source_default_dingtalk_platform_quotas"`

	AllowUserViewErrorRequests *bool `json:"allow_user_view_error_requests"`
}

// UpdateSettings 更新系统设置
// PUT /api/v1/admin/settings
func (h *SettingHandler) UpdateSettings(c *gin.Context) {
	var req UpdateSettingsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}

	previousSettings, err := h.settingService.GetAllSettings(c.Request.Context())
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	previousAuthSourceDefaults, err := h.settingService.GetAuthSourceDefaultSettings(c.Request.Context())
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}

	// 验证参数
	if req.DefaultConcurrency < 1 {
		req.DefaultConcurrency = 1
	}
	if req.DefaultBalance < 0 {
		req.DefaultBalance = 0
	}
	affiliateRebateRate := previousSettings.AffiliateRebateRate
	if req.AffiliateRebateRate != nil {
		affiliateRebateRate = *req.AffiliateRebateRate
	}
	if affiliateRebateRate < service.AffiliateRebateRateMin {
		affiliateRebateRate = service.AffiliateRebateRateMin
	}
	if affiliateRebateRate > service.AffiliateRebateRateMax {
		affiliateRebateRate = service.AffiliateRebateRateMax
	}
	affiliateRebateFreezeHours := previousSettings.AffiliateRebateFreezeHours
	if req.AffiliateRebateFreezeHours != nil {
		affiliateRebateFreezeHours = *req.AffiliateRebateFreezeHours
	}
	if affiliateRebateFreezeHours < 0 {
		affiliateRebateFreezeHours = service.AffiliateRebateFreezeHoursDefault
	}
	if affiliateRebateFreezeHours > service.AffiliateRebateFreezeHoursMax {
		affiliateRebateFreezeHours = service.AffiliateRebateFreezeHoursMax
	}
	affiliateRebateDurationDays := previousSettings.AffiliateRebateDurationDays
	if req.AffiliateRebateDurationDays != nil {
		affiliateRebateDurationDays = *req.AffiliateRebateDurationDays
	}
	if affiliateRebateDurationDays < 0 {
		affiliateRebateDurationDays = service.AffiliateRebateDurationDaysDefault
	}
	if affiliateRebateDurationDays > service.AffiliateRebateDurationDaysMax {
		affiliateRebateDurationDays = service.AffiliateRebateDurationDaysMax
	}
	affiliateRebatePerInviteeCap := previousSettings.AffiliateRebatePerInviteeCap
	if req.AffiliateRebatePerInviteeCap != nil {
		affiliateRebatePerInviteeCap = *req.AffiliateRebatePerInviteeCap
	}
	if affiliateRebatePerInviteeCap < 0 {
		affiliateRebatePerInviteeCap = service.AffiliateRebatePerInviteeCapDefault
	}
	adminRechargeRebateEnabled := previousSettings.AdminRechargeRebateEnabled
	if req.AdminRechargeRebateEnabled != nil {
		adminRechargeRebateEnabled = *req.AdminRechargeRebateEnabled
	}
	// 通用表格配置：兼容旧客户端未传字段时保留当前值。
	if req.TableDefaultPageSize <= 0 {
		req.TableDefaultPageSize = previousSettings.TableDefaultPageSize
	}
	if req.TablePageSizeOptions == nil {
		req.TablePageSizeOptions = previousSettings.TablePageSizeOptions
	}
	req.SMTPHost = strings.TrimSpace(req.SMTPHost)
	req.SMTPUsername = strings.TrimSpace(req.SMTPUsername)
	req.SMTPPassword = strings.TrimSpace(req.SMTPPassword)
	req.SMTPFrom = strings.TrimSpace(req.SMTPFrom)
	req.SMTPFromName = strings.TrimSpace(req.SMTPFromName)
	if req.SMTPPort <= 0 {
		req.SMTPPort = 587
	}
	req.DefaultSubscriptions = normalizeDefaultSubscriptions(req.DefaultSubscriptions)
	req.AuthSourceDefaultEmailSubscriptions = normalizeOptionalDefaultSubscriptions(req.AuthSourceDefaultEmailSubscriptions)
	req.AuthSourceDefaultLinuxDoSubscriptions = normalizeOptionalDefaultSubscriptions(req.AuthSourceDefaultLinuxDoSubscriptions)
	req.AuthSourceDefaultOIDCSubscriptions = normalizeOptionalDefaultSubscriptions(req.AuthSourceDefaultOIDCSubscriptions)
	req.AuthSourceDefaultWeChatSubscriptions = normalizeOptionalDefaultSubscriptions(req.AuthSourceDefaultWeChatSubscriptions)
	req.AuthSourceDefaultDingTalkSubscriptions = normalizeOptionalDefaultSubscriptions(req.AuthSourceDefaultDingTalkSubscriptions)

	// SMTP 配置保护：如果请求中 smtp_host 为空但数据库中已有配置，则保留已有 SMTP 配置
	// 防止前端加载设置失败时空表单覆盖已保存的 SMTP 配置
	if req.SMTPHost == "" && previousSettings.SMTPHost != "" {
		req.SMTPHost = previousSettings.SMTPHost
		req.SMTPPort = previousSettings.SMTPPort
		req.SMTPUsername = previousSettings.SMTPUsername
		req.SMTPFrom = previousSettings.SMTPFrom
		req.SMTPFromName = previousSettings.SMTPFromName
		req.SMTPUseTLS = previousSettings.SMTPUseTLS
	}

	// Turnstile 参数验证
	if req.TurnstileEnabled {
		// 检查必填字段
		if req.TurnstileSiteKey == "" {
			response.BadRequest(c, "Turnstile Site Key is required when enabled")
			return
		}
		// 如果未提供 secret key，使用已保存的值（留空保留当前值）
		if req.TurnstileSecretKey == "" {
			if previousSettings.TurnstileSecretKey == "" {
				response.BadRequest(c, "Turnstile Secret Key is required when enabled")
				return
			}
			req.TurnstileSecretKey = previousSettings.TurnstileSecretKey
		}

		// 当 site_key 或 secret_key 任一变化时验证（避免配置错误导致无法登录）
		siteKeyChanged := previousSettings.TurnstileSiteKey != req.TurnstileSiteKey
		secretKeyChanged := previousSettings.TurnstileSecretKey != req.TurnstileSecretKey
		if siteKeyChanged || secretKeyChanged {
			if err := h.turnstileService.ValidateSecretKey(c.Request.Context(), req.TurnstileSecretKey); err != nil {
				response.ErrorFrom(c, err)
				return
			}
		}
	}

	// TOTP 双因素认证参数验证
	// 只有手动配置了加密密钥才允许启用 TOTP 功能
	if req.TotpEnabled && !previousSettings.TotpEnabled {
		// 尝试启用 TOTP，检查加密密钥是否已手动配置
		if !h.settingService.IsTotpEncryptionKeyConfigured() {
			response.BadRequest(c, "Cannot enable TOTP: TOTP_ENCRYPTION_KEY environment variable must be configured first. Generate a key with 'openssl rand -hex 32' and set it in your environment.")
			return
		}
	}
	loginAgreementMode := strings.ToLower(strings.TrimSpace(req.LoginAgreementMode))
	if loginAgreementMode == "" {
		loginAgreementMode = strings.ToLower(strings.TrimSpace(previousSettings.LoginAgreementMode))
	}
	switch loginAgreementMode {
	case "", "modal":
		loginAgreementMode = "modal"
	case "checkbox":
	default:
		response.BadRequest(c, "Login agreement mode must be modal or checkbox")
		return
	}
	loginAgreementUpdatedAt := strings.TrimSpace(req.LoginAgreementUpdatedAt)
	if loginAgreementUpdatedAt == "" {
		loginAgreementUpdatedAt = strings.TrimSpace(previousSettings.LoginAgreementUpdatedAt)
	}
	loginAgreementDocuments := loginAgreementDocumentsToService(req.LoginAgreementDocuments)
	if len(loginAgreementDocuments) == 0 {
		loginAgreementDocuments = previousSettings.LoginAgreementDocuments
	}
	for _, doc := range loginAgreementDocuments {
		if strings.TrimSpace(doc.Title) == "" {
			response.BadRequest(c, "Login agreement document title is required")
			return
		}
		if len(doc.Title) > 80 {
			response.BadRequest(c, "Login agreement document title is too long (max 80 characters)")
			return
		}
		if len(doc.ContentMD) > 200*1024 {
			response.BadRequest(c, "Login agreement document content is too large (max 200KB)")
			return
		}
	}
	if req.LoginAgreementEnabled && len(loginAgreementDocuments) == 0 {
		response.BadRequest(c, "Login agreement documents are required when enabled")
		return
	}

	// LinuxDo Connect 参数验证
	if req.LinuxDoConnectEnabled {
		req.LinuxDoConnectClientID = strings.TrimSpace(req.LinuxDoConnectClientID)
		req.LinuxDoConnectClientSecret = strings.TrimSpace(req.LinuxDoConnectClientSecret)
		req.LinuxDoConnectRedirectURL = strings.TrimSpace(req.LinuxDoConnectRedirectURL)

		if req.LinuxDoConnectClientID == "" {
			response.BadRequest(c, "LinuxDo Client ID is required when enabled")
			return
		}
		if req.LinuxDoConnectRedirectURL == "" {
			response.BadRequest(c, "LinuxDo Redirect URL is required when enabled")
			return
		}
		if err := config.ValidateAbsoluteHTTPURL(req.LinuxDoConnectRedirectURL); err != nil {
			response.BadRequest(c, "LinuxDo Redirect URL must be an absolute http(s) URL")
			return
		}

		// 如果未提供 client_secret，则保留现有值（如有）。
		if req.LinuxDoConnectClientSecret == "" {
			if previousSettings.LinuxDoConnectClientSecret == "" {
				response.BadRequest(c, "LinuxDo Client Secret is required when enabled")
				return
			}
			req.LinuxDoConnectClientSecret = previousSettings.LinuxDoConnectClientSecret
		}
	}

	// DingTalk Connect 参数验证
	// 防御性：任何写入路径上把已废弃的 corp_restriction_policy=whitelist 入参 coerce 为 none，
	// 避免任何直连 admin API 的客户端把死值写回 DB（前端 UI 已无此选项）。
	req.DingTalkConnectCorpRestrictionPolicy = service.CoerceDingTalkCorpPolicyForWrite(req.DingTalkConnectCorpRestrictionPolicy)

	if req.DingTalkConnectEnabled {
		req.DingTalkConnectClientID = strings.TrimSpace(req.DingTalkConnectClientID)
		req.DingTalkConnectClientSecret = strings.TrimSpace(req.DingTalkConnectClientSecret)
		req.DingTalkConnectRedirectURL = strings.TrimSpace(req.DingTalkConnectRedirectURL)
		req.DingTalkConnectCorpRestrictionPolicy = strings.TrimSpace(req.DingTalkConnectCorpRestrictionPolicy)
		req.DingTalkConnectInternalCorpID = strings.TrimSpace(req.DingTalkConnectInternalCorpID)

		if req.DingTalkConnectClientID == "" {
			response.BadRequest(c, "DingTalk Client ID is required when enabled")
			return
		}
		if req.DingTalkConnectRedirectURL == "" {
			response.BadRequest(c, "DingTalk Redirect URL is required when enabled")
			return
		}
		if err := config.ValidateAbsoluteHTTPURL(req.DingTalkConnectRedirectURL); err != nil {
			response.BadRequest(c, "DingTalk Redirect URL must be an absolute http(s) URL")
			return
		}

		// 如果未提供 client_secret，则保留现有值（如有）。
		if req.DingTalkConnectClientSecret == "" {
			if previousSettings.DingTalkConnectClientSecret == "" {
				response.BadRequest(c, "DingTalk Client Secret is required when enabled")
				return
			}
			req.DingTalkConnectClientSecret = previousSettings.DingTalkConnectClientSecret
		}

		// Corp 策略校验（V1/V4 fail-closed）
		dingTalkCfg := config.DingTalkConnectConfig{
			Enabled:               true,
			DingTalkAppKind:       "internal_app", // 硬编码：settings 层仅支持 internal_app
			AppType:               "internal",     // 对于 internal_only 策略的默认值
			CorpRestrictionPolicy: req.DingTalkConnectCorpRestrictionPolicy,
			InternalCorpID:        req.DingTalkConnectInternalCorpID,
		}
		// 若未填 corp_restriction_policy，保留已有配置
		if dingTalkCfg.CorpRestrictionPolicy == "" {
			dingTalkCfg.CorpRestrictionPolicy = previousSettings.DingTalkConnectCorpRestrictionPolicy
		}
		// 对于 internal_only 策略，app_type 必须为 internal（V1 校验）
		if dingTalkCfg.CorpRestrictionPolicy == "internal_only" {
			dingTalkCfg.AppType = "internal"
		} else {
			dingTalkCfg.AppType = "public"
		}
		if err := config.ValidateDingTalkConfig(dingTalkCfg); err != nil {
			response.ErrorWithDetails(c, http.StatusBadRequest, err.Error(), mapDingTalkValidateError(err), nil)
			return
		}

		// bypass_registration 仅在 internal_only 模式下有意义；其它策略下强制为 false，
		// 防止 admin 在切换 policy 时把 bypass 残留在 DB 中（前端 UI 也已隐藏该开关）。
		if dingTalkCfg.CorpRestrictionPolicy != "internal_only" {
			req.DingTalkConnectBypassRegistration = false
			// 身份同步三开关同理：仅 internal_only 模式下有意义，其它策略强制 false。
			req.DingTalkConnectSyncCorpEmail = false
			req.DingTalkConnectSyncDisplayName = false
			req.DingTalkConnectSyncDept = false
		}
		// 身份同步目标 attr key：trimSpace + 空值 fallback 到默认值
		req.DingTalkConnectSyncCorpEmailAttrKey = strings.TrimSpace(req.DingTalkConnectSyncCorpEmailAttrKey)
		if req.DingTalkConnectSyncCorpEmailAttrKey == "" {
			req.DingTalkConnectSyncCorpEmailAttrKey = "dingtalk_email"
		}
		req.DingTalkConnectSyncDisplayNameAttrKey = strings.TrimSpace(req.DingTalkConnectSyncDisplayNameAttrKey)
		if req.DingTalkConnectSyncDisplayNameAttrKey == "" {
			req.DingTalkConnectSyncDisplayNameAttrKey = "dingtalk_name"
		}
		req.DingTalkConnectSyncDeptAttrKey = strings.TrimSpace(req.DingTalkConnectSyncDeptAttrKey)
		if req.DingTalkConnectSyncDeptAttrKey == "" {
			req.DingTalkConnectSyncDeptAttrKey = "dingtalk_department"
		}
		// 身份同步目标 attr 显示名称：trim + 空值 fallback 到默认中文名
		req.DingTalkConnectSyncCorpEmailAttrName = strings.TrimSpace(req.DingTalkConnectSyncCorpEmailAttrName)
		if req.DingTalkConnectSyncCorpEmailAttrName == "" {
			req.DingTalkConnectSyncCorpEmailAttrName = "钉钉企业邮箱"
		}
		req.DingTalkConnectSyncDisplayNameAttrName = strings.TrimSpace(req.DingTalkConnectSyncDisplayNameAttrName)
		if req.DingTalkConnectSyncDisplayNameAttrName == "" {
			req.DingTalkConnectSyncDisplayNameAttrName = "钉钉姓名"
		}
		req.DingTalkConnectSyncDeptAttrName = strings.TrimSpace(req.DingTalkConnectSyncDeptAttrName)
		if req.DingTalkConnectSyncDeptAttrName == "" {
			req.DingTalkConnectSyncDeptAttrName = "钉钉部门"
		}
	}

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
		}
		if req.WeChatConnectScopes == "" {
			req.WeChatConnectScopes = strings.TrimSpace(previousSettings.WeChatConnectScopes)
		}

		if req.WeChatConnectMPEnabled && req.WeChatConnectMobileEnabled {
			response.BadRequest(c, "WeChat Official Account and Mobile App cannot be enabled at the same time")
			return
		}
		if req.WeChatConnectMode != "" {
			switch req.WeChatConnectMode {
			case "open", "mp", "mobile":
			default:
				response.BadRequest(c, "WeChat mode must be open, mp, or mobile")
				return
			}
		}
		if !req.WeChatConnectOpenEnabled && !req.WeChatConnectMPEnabled && !req.WeChatConnectMobileEnabled {
			switch req.WeChatConnectMode {
			case "mp":
				req.WeChatConnectMPEnabled = true
			case "mobile":
				req.WeChatConnectMobileEnabled = true
			default:
				req.WeChatConnectOpenEnabled = true
			}
		}
		if req.WeChatConnectMode == "" {
			if req.WeChatConnectMPEnabled {
				req.WeChatConnectMode = "mp"
			} else if req.WeChatConnectMobileEnabled {
				req.WeChatConnectMode = "mobile"
			} else {
				req.WeChatConnectMode = "open"
			}
		}

		req.WeChatConnectOpenAppID = strings.TrimSpace(firstNonEmpty(req.WeChatConnectOpenAppID, req.WeChatConnectAppID, previousSettings.WeChatConnectOpenAppID, previousSettings.WeChatConnectAppID))
		req.WeChatConnectMPAppID = strings.TrimSpace(firstNonEmpty(req.WeChatConnectMPAppID, req.WeChatConnectAppID, previousSettings.WeChatConnectMPAppID, previousSettings.WeChatConnectAppID))
		req.WeChatConnectMobileAppID = strings.TrimSpace(firstNonEmpty(req.WeChatConnectMobileAppID, req.WeChatConnectAppID, previousSettings.WeChatConnectMobileAppID, previousSettings.WeChatConnectAppID))

		if req.WeChatConnectOpenAppSecret == "" {
			req.WeChatConnectOpenAppSecret = strings.TrimSpace(firstNonEmpty(previousSettings.WeChatConnectOpenAppSecret, previousSettings.WeChatConnectAppSecret, req.WeChatConnectAppSecret))
		}
		if req.WeChatConnectMPAppSecret == "" {
			req.WeChatConnectMPAppSecret = strings.TrimSpace(firstNonEmpty(previousSettings.WeChatConnectMPAppSecret, previousSettings.WeChatConnectAppSecret, req.WeChatConnectAppSecret))
		}
		if req.WeChatConnectMobileAppSecret == "" {
			req.WeChatConnectMobileAppSecret = strings.TrimSpace(firstNonEmpty(previousSettings.WeChatConnectMobileAppSecret, previousSettings.WeChatConnectAppSecret, req.WeChatConnectAppSecret))
		}
		if req.WeChatConnectAppSecret == "" {
			req.WeChatConnectAppSecret = strings.TrimSpace(firstNonEmpty(req.WeChatConnectOpenAppSecret, req.WeChatConnectMPAppSecret, req.WeChatConnectMobileAppSecret, previousSettings.WeChatConnectAppSecret))
		}

		if req.WeChatConnectOpenEnabled {
			if req.WeChatConnectOpenAppID == "" {
				response.BadRequest(c, "WeChat PC App ID is required when enabled")
				return
			}
			if req.WeChatConnectOpenAppSecret == "" {
				response.BadRequest(c, "WeChat PC App Secret is required when enabled")
				return
			}
		}
		if req.WeChatConnectMPEnabled {
			if req.WeChatConnectMPAppID == "" {
				response.BadRequest(c, "WeChat Official Account App ID is required when enabled")
				return
			}
			if req.WeChatConnectMPAppSecret == "" {
				response.BadRequest(c, "WeChat Official Account App Secret is required when enabled")
				return
			}
		}
		if req.WeChatConnectMobileEnabled {
			if req.WeChatConnectMobileAppID == "" {
				response.BadRequest(c, "WeChat Mobile App ID is required when enabled")
				return
			}
			if req.WeChatConnectMobileAppSecret == "" {
				response.BadRequest(c, "WeChat Mobile App Secret is required when enabled")
				return
			}
		}

		if req.WeChatConnectScopes == "" {
			if req.WeChatConnectMPEnabled {
				req.WeChatConnectScopes = service.DefaultWeChatConnectScopesForMode("mp")
			} else {
				req.WeChatConnectScopes = service.DefaultWeChatConnectScopesForMode(req.WeChatConnectMode)
			}
		}
		if req.WeChatConnectOpenEnabled || req.WeChatConnectMPEnabled {
			if req.WeChatConnectRedirectURL == "" {
				response.BadRequest(c, "WeChat Redirect URL is required when web oauth is enabled")
				return
			}
			if err := config.ValidateAbsoluteHTTPURL(req.WeChatConnectRedirectURL); err != nil {
				response.BadRequest(c, "WeChat Redirect URL must be an absolute http(s) URL")
				return
			}
			if req.WeChatConnectFrontendRedirectURL == "" {
				req.WeChatConnectFrontendRedirectURL = "/auth/wechat/callback"
			}
			if err := config.ValidateFrontendRedirectURL(req.WeChatConnectFrontendRedirectURL); err != nil {
				response.BadRequest(c, "WeChat Frontend Redirect URL is invalid")
				return
			}
		}
	}

	// Generic OIDC 参数验证
	oidcUsePKCE, oidcValidateIDToken, err := h.settingService.OIDCSecurityWriteDefaults(c.Request.Context())
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
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
		}
		if req.OIDCConnectValidateIDToken != nil {
			oidcValidateIDToken = *req.OIDCConnectValidateIDToken
		}
		if req.OIDCConnectClockSkewSeconds == 0 {
			req.OIDCConnectClockSkewSeconds = previousSettings.OIDCConnectClockSkewSeconds
			if req.OIDCConnectClockSkewSeconds == 0 {
				req.OIDCConnectClockSkewSeconds = 120
			}
		}

		if req.OIDCConnectClientID == "" {
			response.BadRequest(c, "OIDC Client ID is required when enabled")
			return
		}
		if req.OIDCConnectIssuerURL == "" {
			response.BadRequest(c, "OIDC Issuer URL is required when enabled")
			return
		}
		if err := config.ValidateAbsoluteHTTPURL(req.OIDCConnectIssuerURL); err != nil {
			response.BadRequest(c, "OIDC Issuer URL must be an absolute http(s) URL")
			return
		}
		if req.OIDCConnectDiscoveryURL != "" {
			if err := config.ValidateAbsoluteHTTPURL(req.OIDCConnectDiscoveryURL); err != nil {
				response.BadRequest(c, "OIDC Discovery URL must be an absolute http(s) URL")
				return
			}
		}
		if req.OIDCConnectAuthorizeURL != "" {
			if err := config.ValidateAbsoluteHTTPURL(req.OIDCConnectAuthorizeURL); err != nil {
				response.BadRequest(c, "OIDC Authorize URL must be an absolute http(s) URL")
				return
			}
		}
		if req.OIDCConnectTokenURL != "" {
			if err := config.ValidateAbsoluteHTTPURL(req.OIDCConnectTokenURL); err != nil {
				response.BadRequest(c, "OIDC Token URL must be an absolute http(s) URL")
				return
			}
		}
		if req.OIDCConnectUserInfoURL != "" {
			if err := config.ValidateAbsoluteHTTPURL(req.OIDCConnectUserInfoURL); err != nil {
				response.BadRequest(c, "OIDC UserInfo URL must be an absolute http(s) URL")
				return
			}
		}
		if req.OIDCConnectRedirectURL == "" {
			response.BadRequest(c, "OIDC Redirect URL is required when enabled")
			return
		}
		if err := config.ValidateAbsoluteHTTPURL(req.OIDCConnectRedirectURL); err != nil {
			response.BadRequest(c, "OIDC Redirect URL must be an absolute http(s) URL")
			return
		}
		if req.OIDCConnectFrontendRedirectURL == "" {
			response.BadRequest(c, "OIDC Frontend Redirect URL is required when enabled")
			return
		}
		if err := config.ValidateFrontendRedirectURL(req.OIDCConnectFrontendRedirectURL); err != nil {
			response.BadRequest(c, "OIDC Frontend Redirect URL is invalid")
			return
		}
		if !scopesContainOpenID(req.OIDCConnectScopes) {
			response.BadRequest(c, "OIDC scopes must contain openid")
			return
		}
		switch req.OIDCConnectTokenAuthMethod {
		case "", "client_secret_post", "client_secret_basic", "none":
		default:
			response.BadRequest(c, "OIDC Token Auth Method must be one of client_secret_post/client_secret_basic/none")
			return
		}
		if req.OIDCConnectClockSkewSeconds < 0 || req.OIDCConnectClockSkewSeconds > 600 {
			response.BadRequest(c, "OIDC clock skew seconds must be between 0 and 600")
			return
		}
		if oidcValidateIDToken && req.OIDCConnectAllowedSigningAlgs == "" {
			response.BadRequest(c, "OIDC Allowed Signing Algs is required when validate_id_token=true")
			return
		}
		if req.OIDCConnectJWKSURL != "" {
			if err := config.ValidateAbsoluteHTTPURL(req.OIDCConnectJWKSURL); err != nil {
				response.BadRequest(c, "OIDC JWKS URL must be an absolute http(s) URL")
				return
			}
		}
		if req.OIDCConnectTokenAuthMethod == "" || req.OIDCConnectTokenAuthMethod == "client_secret_post" || req.OIDCConnectTokenAuthMethod == "client_secret_basic" {
			if req.OIDCConnectClientSecret == "" {
				if previousSettings.OIDCConnectClientSecret == "" {
					response.BadRequest(c, "OIDC Client Secret is required when enabled")
					return
				}
				req.OIDCConnectClientSecret = previousSettings.OIDCConnectClientSecret
			}
		}
	}

	// “购买订阅”页面配置验证
	purchaseEnabled := previousSettings.PurchaseSubscriptionEnabled
	if req.PurchaseSubscriptionEnabled != nil {
		purchaseEnabled = *req.PurchaseSubscriptionEnabled
	}
	purchaseURL := previousSettings.PurchaseSubscriptionURL
	if req.PurchaseSubscriptionURL != nil {
		purchaseURL = strings.TrimSpace(*req.PurchaseSubscriptionURL)
	}

	// - 启用时要求 URL 合法且非空
	// - 禁用时允许为空；若提供了 URL 也做基本校验，避免误配置
	if purchaseEnabled {
		if purchaseURL == "" {
			response.BadRequest(c, "Purchase Subscription URL is required when enabled")
			return
		}
		if err := config.ValidateAbsoluteHTTPURL(purchaseURL); err != nil {
			response.BadRequest(c, "Purchase Subscription URL must be an absolute http(s) URL")
			return
		}
	} else if purchaseURL != "" {
		if err := config.ValidateAbsoluteHTTPURL(purchaseURL); err != nil {
			response.BadRequest(c, "Purchase Subscription URL must be an absolute http(s) URL")
			return
		}
	}

	// Frontend URL 验证
	req.FrontendURL = strings.TrimSpace(req.FrontendURL)
	if req.FrontendURL != "" {
		if err := config.ValidateAbsoluteHTTPURL(req.FrontendURL); err != nil {
			response.BadRequest(c, "Frontend URL must be an absolute http(s) URL")
			return
		}
	}

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
		}
		for i, item := range items {
			if strings.TrimSpace(item.Label) == "" {
				response.BadRequest(c, "Custom menu item label is required")
				return
			}
			if len(item.Label) > maxMenuItemLabelLen {
				response.BadRequest(c, "Custom menu item label is too long (max 50 characters)")
				return
			}
			urlTrimmed := strings.TrimSpace(item.URL)
			if strings.HasPrefix(urlTrimmed, "md:") {
				// Markdown page mode: URL = "md:<slug>"
				slug := strings.TrimPrefix(urlTrimmed, "md:")
				if slug == "" {
					response.BadRequest(c, "Custom menu item markdown slug cannot be empty (use md:slug format)")
					return
				}
			} else {
				if urlTrimmed == "" {
					response.BadRequest(c, "Custom menu item URL is required (use md:slug for markdown pages)")
					return
				}
				if len(item.URL) > maxMenuItemURLLen {
					response.BadRequest(c, "Custom menu item URL is too long (max 2048 characters)")
					return
				}
				if err := config.ValidateAbsoluteHTTPURL(urlTrimmed); err != nil {
					response.BadRequest(c, "Custom menu item URL must be an absolute http(s) URL or md:<slug>")
					return
				}
			}
			if item.Visibility != "user" && item.Visibility != "admin" {
				response.BadRequest(c, "Custom menu item visibility must be 'user' or 'admin'")
				return
			}
			if len(item.IconSVG) > maxMenuItemIconSVGLen {
				response.BadRequest(c, "Custom menu item icon SVG is too large (max 10KB)")
				return
			}
			// Auto-generate ID if missing
			if strings.TrimSpace(item.ID) == "" {
				id, err := generateMenuItemID()
				if err != nil {
					response.Error(c, http.StatusInternalServerError, "Failed to generate menu item ID")
					return
				}
				items[i].ID = id
			} else if len(item.ID) > maxMenuItemIDLen {
				response.BadRequest(c, "Custom menu item ID is too long (max 32 characters)")
				return
			} else if !menuItemIDPattern.MatchString(item.ID) {
				response.BadRequest(c, "Custom menu item ID contains invalid characters (only a-z, A-Z, 0-9, - and _ are allowed)")
				return
			}
		}
		// ID uniqueness check
		seen := make(map[string]struct{}, len(items))
		for _, item := range items {
			if _, exists := seen[item.ID]; exists {
				response.BadRequest(c, "Duplicate custom menu item ID: "+item.ID)
				return
			}
			seen[item.ID] = struct{}{}
		}
		menuBytes, err := json.Marshal(items)
		if err != nil {
			response.BadRequest(c, "Failed to serialize custom menu items")
			return
		}
		customMenuJSON = string(menuBytes)
	}

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
		}
		for _, ep := range endpoints {
			if strings.TrimSpace(ep.Name) == "" {
				response.BadRequest(c, "Custom endpoint name is required")
				return
			}
			if len(ep.Name) > maxEndpointNameLen {
				response.BadRequest(c, "Custom endpoint name is too long (max 50 characters)")
				return
			}
			if strings.TrimSpace(ep.Endpoint) == "" {
				response.BadRequest(c, "Custom endpoint URL is required")
				return
			}
			if len(ep.Endpoint) > maxEndpointURLLen {
				response.BadRequest(c, "Custom endpoint URL is too long (max 2048 characters)")
				return
			}
			if err := config.ValidateAbsoluteHTTPURL(strings.TrimSpace(ep.Endpoint)); err != nil {
				response.BadRequest(c, "Custom endpoint URL must be an absolute http(s) URL")
				return
			}
			if len(ep.Description) > maxEndpointDescriptionLen {
				response.BadRequest(c, "Custom endpoint description is too long (max 200 characters)")
				return
			}
		}
		endpointBytes, err := json.Marshal(endpoints)
		if err != nil {
			response.BadRequest(c, "Failed to serialize custom endpoints")
			return
		}
		customEndpointsJSON = string(endpointBytes)
	}

	// Ops metrics collector interval validation (seconds).
	if req.OpsMetricsIntervalSeconds != nil {
		v := *req.OpsMetricsIntervalSeconds
		if v < 60 {
			v = 60
		}
		if v > 3600 {
			v = 3600
		}
		req.OpsMetricsIntervalSeconds = &v
	}
	defaultSubscriptions := make([]service.DefaultSubscriptionSetting, 0, len(req.DefaultSubscriptions))
	for _, sub := range req.DefaultSubscriptions {
		defaultSubscriptions = append(defaultSubscriptions, service.DefaultSubscriptionSetting{
			GroupID:      sub.GroupID,
			ValidityDays: sub.ValidityDays,
		})
	}

	// 验证最低版本号格式（空字符串=禁用，或合法 semver）
	if req.MinClaudeCodeVersion != "" {
		if !semverPattern.MatchString(req.MinClaudeCodeVersion) {
			response.Error(c, http.StatusBadRequest, "min_claude_code_version must be empty or a valid semver (e.g. 2.1.63)")
			return
		}
	}

	// 验证最高版本号格式（空字符串=禁用，或合法 semver）
	if req.MaxClaudeCodeVersion != "" {
		if !semverPattern.MatchString(req.MaxClaudeCodeVersion) {
			response.Error(c, http.StatusBadRequest, "max_claude_code_version must be empty or a valid semver (e.g. 3.0.0)")
			return
		}
	}
	if req.AntigravityUserAgentVersion != nil {
		normalized := strings.TrimSpace(*req.AntigravityUserAgentVersion)
		req.AntigravityUserAgentVersion = &normalized
		if normalized != "" && !semverPattern.MatchString(normalized) {
			response.Error(c, http.StatusBadRequest, "antigravity_user_agent_version must be empty or a valid semver (e.g. 1.23.2)")
			return
		}
	}
	if req.OpenAICodexUserAgent != nil {
		normalized := strings.TrimSpace(*req.OpenAICodexUserAgent)
		req.OpenAICodexUserAgent = &normalized
		// 仅做长度上限保护，不限制具体格式（运维需要可自由调整 codex 版本号）
		if len(normalized) > 512 {
			response.Error(c, http.StatusBadRequest, "openai_codex_user_agent must be at most 512 characters")
			return
		}
	}

	// codex_cli_only 加固：最低/最高 Codex 版本（空=禁用，或合法 semver；max>=min）
	if req.MinCodexVersion != "" && !semverPattern.MatchString(req.MinCodexVersion) {
		response.Error(c, http.StatusBadRequest, "min_codex_version must be empty or a valid semver (e.g. 0.141.0)")
		return
	}
	if req.MaxCodexVersion != "" && !semverPattern.MatchString(req.MaxCodexVersion) {
		response.Error(c, http.StatusBadRequest, "max_codex_version must be empty or a valid semver (e.g. 0.200.0)")
		return
	}
	if req.MinCodexVersion != "" && req.MaxCodexVersion != "" && service.CompareVersions(req.MaxCodexVersion, req.MinCodexVersion) < 0 {
		response.Error(c, http.StatusBadRequest, "max_codex_version must be greater than or equal to min_codex_version")
		return
	}
	// codex_cli_only 黑/白名单：非空须为合法 []AllowedClientEntry JSON。
	// 黑名单 OR 宽 deny（允许 originator-only）；白名单双因子 AND，额外要求每条可命中（非空 originator + ua_contains）。
	if err := service.ValidateCodexClientEntriesJSON(req.CodexCLIOnlyBlacklist); err != nil {
		response.Error(c, http.StatusBadRequest, "codex_cli_only_blacklist "+err.Error())
		return
	}
	if err := service.ValidateCodexWhitelistEntriesJSON(req.CodexCLIOnlyWhitelist); err != nil {
		response.Error(c, http.StatusBadRequest, "codex_cli_only_whitelist "+err.Error())
		return
	}
	if err := service.ValidateEngineFingerprintSignalsJSON(req.CodexCLIOnlyEngineFingerprintSignals); err != nil {
		response.Error(c, http.StatusBadRequest, "codex_cli_only_engine_fingerprint_signals "+err.Error())
		return
	}

	// 交叉验证：如果同时设置了最低和最高版本号，最高版本号必须 >= 最低版本号
	if req.MinClaudeCodeVersion != "" && req.MaxClaudeCodeVersion != "" {
		if service.CompareVersions(req.MaxClaudeCodeVersion, req.MinClaudeCodeVersion) < 0 {
			response.Error(c, http.StatusBadRequest, "max_claude_code_version must be greater than or equal to min_claude_code_version")
			return
		}
	}

	// cyber 会话屏蔽 TTL 校验：提供时必须 > 0
	if req.CyberSessionBlockTTLSeconds != nil && *req.CyberSessionBlockTTLSeconds <= 0 {
		response.BadRequest(c, "cyber_session_block_ttl_seconds must be > 0")
		return
	}

	settings := &service.SystemSettings{
		// 系统全局 platform quota 默认值（整体替换语义）
		DefaultPlatformQuotas: req.DefaultPlatformQuotas,

		RegistrationEnabled:              req.RegistrationEnabled,
		EmailVerifyEnabled:               req.EmailVerifyEnabled,
		RegistrationEmailSuffixWhitelist: req.RegistrationEmailSuffixWhitelist,
		PromoCodeEnabled:                 req.PromoCodeEnabled,
		PasswordResetEnabled:             req.PasswordResetEnabled,
		FrontendURL:                      req.FrontendURL,
		InvitationCodeEnabled:            req.InvitationCodeEnabled,
		TotpEnabled:                      req.TotpEnabled,
		SessionBindingEnabled:            req.SessionBindingEnabled,
		AuditLogRetentionDays:            req.AuditLogRetentionDays,
		LoginAgreementEnabled:            req.LoginAgreementEnabled,
		LoginAgreementMode:               loginAgreementMode,
		LoginAgreementUpdatedAt:          loginAgreementUpdatedAt,
		LoginAgreementDocuments:          loginAgreementDocuments,
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
		APIKeyACLTrustForwardedIP: func() bool {
			if req.APIKeyACLTrustForwardedIP != nil {
				return *req.APIKeyACLTrustForwardedIP
			}
			return previousSettings.APIKeyACLTrustForwardedIP
		}(),
		LinuxDoConnectEnabled:                  req.LinuxDoConnectEnabled,
		LinuxDoConnectClientID:                 req.LinuxDoConnectClientID,
		LinuxDoConnectClientSecret:             req.LinuxDoConnectClientSecret,
		LinuxDoConnectRedirectURL:              req.LinuxDoConnectRedirectURL,
		DingTalkConnectEnabled:                 req.DingTalkConnectEnabled,
		DingTalkConnectClientID:                req.DingTalkConnectClientID,
		DingTalkConnectClientSecret:            req.DingTalkConnectClientSecret,
		DingTalkConnectRedirectURL:             req.DingTalkConnectRedirectURL,
		DingTalkConnectCorpRestrictionPolicy:   req.DingTalkConnectCorpRestrictionPolicy,
		DingTalkConnectInternalCorpID:          req.DingTalkConnectInternalCorpID,
		DingTalkConnectBypassRegistration:      req.DingTalkConnectBypassRegistration,
		DingTalkConnectSyncCorpEmail:           req.DingTalkConnectSyncCorpEmail,
		DingTalkConnectSyncDisplayName:         req.DingTalkConnectSyncDisplayName,
		DingTalkConnectSyncDept:                req.DingTalkConnectSyncDept,
		DingTalkConnectSyncCorpEmailAttrKey:    req.DingTalkConnectSyncCorpEmailAttrKey,
		DingTalkConnectSyncDisplayNameAttrKey:  req.DingTalkConnectSyncDisplayNameAttrKey,
		DingTalkConnectSyncDeptAttrKey:         req.DingTalkConnectSyncDeptAttrKey,
		DingTalkConnectSyncCorpEmailAttrName:   req.DingTalkConnectSyncCorpEmailAttrName,
		DingTalkConnectSyncDisplayNameAttrName: req.DingTalkConnectSyncDisplayNameAttrName,
		DingTalkConnectSyncDeptAttrName:        req.DingTalkConnectSyncDeptAttrName,
		WeChatConnectEnabled:                   req.WeChatConnectEnabled,
		WeChatConnectAppID:                     req.WeChatConnectAppID,
		WeChatConnectAppSecret:                 req.WeChatConnectAppSecret,
		WeChatConnectOpenAppID:                 req.WeChatConnectOpenAppID,
		WeChatConnectOpenAppSecret:             req.WeChatConnectOpenAppSecret,
		WeChatConnectMPAppID:                   req.WeChatConnectMPAppID,
		WeChatConnectMPAppSecret:               req.WeChatConnectMPAppSecret,
		WeChatConnectMobileAppID:               req.WeChatConnectMobileAppID,
		WeChatConnectMobileAppSecret:           req.WeChatConnectMobileAppSecret,
		WeChatConnectOpenEnabled:               req.WeChatConnectOpenEnabled,
		WeChatConnectMPEnabled:                 req.WeChatConnectMPEnabled,
		WeChatConnectMobileEnabled:             req.WeChatConnectMobileEnabled,
		WeChatConnectMode:                      req.WeChatConnectMode,
		WeChatConnectScopes:                    req.WeChatConnectScopes,
		WeChatConnectRedirectURL:               req.WeChatConnectRedirectURL,
		WeChatConnectFrontendRedirectURL:       req.WeChatConnectFrontendRedirectURL,
		OIDCConnectEnabled:                     req.OIDCConnectEnabled,
		OIDCConnectProviderName:                req.OIDCConnectProviderName,
		OIDCConnectClientID:                    req.OIDCConnectClientID,
		OIDCConnectClientSecret:                req.OIDCConnectClientSecret,
		OIDCConnectIssuerURL:                   req.OIDCConnectIssuerURL,
		OIDCConnectDiscoveryURL:                req.OIDCConnectDiscoveryURL,
		OIDCConnectAuthorizeURL:                req.OIDCConnectAuthorizeURL,
		OIDCConnectTokenURL:                    req.OIDCConnectTokenURL,
		OIDCConnectUserInfoURL:                 req.OIDCConnectUserInfoURL,
		OIDCConnectJWKSURL:                     req.OIDCConnectJWKSURL,
		OIDCConnectScopes:                      req.OIDCConnectScopes,
		OIDCConnectRedirectURL:                 req.OIDCConnectRedirectURL,
		OIDCConnectFrontendRedirectURL:         req.OIDCConnectFrontendRedirectURL,
		OIDCConnectTokenAuthMethod:             req.OIDCConnectTokenAuthMethod,
		OIDCConnectUsePKCE:                     oidcUsePKCE,
		OIDCConnectValidateIDToken:             oidcValidateIDToken,
		OIDCConnectAllowedSigningAlgs:          req.OIDCConnectAllowedSigningAlgs,
		OIDCConnectClockSkewSeconds:            req.OIDCConnectClockSkewSeconds,
		OIDCConnectRequireEmailVerified:        req.OIDCConnectRequireEmailVerified,
		OIDCConnectUserInfoEmailPath:           req.OIDCConnectUserInfoEmailPath,
		OIDCConnectUserInfoIDPath:              req.OIDCConnectUserInfoIDPath,
		OIDCConnectUserInfoUsernamePath:        req.OIDCConnectUserInfoUsernamePath,
		GitHubOAuthEnabled:                     req.GitHubOAuthEnabled,
		GitHubOAuthClientID:                    req.GitHubOAuthClientID,
		GitHubOAuthClientSecret:                req.GitHubOAuthClientSecret,
		GitHubOAuthRedirectURL:                 req.GitHubOAuthRedirectURL,
		GitHubOAuthFrontendRedirectURL:         req.GitHubOAuthFrontendRedirectURL,
		GoogleOAuthEnabled:                     req.GoogleOAuthEnabled,
		GoogleOAuthClientID:                    req.GoogleOAuthClientID,
		GoogleOAuthClientSecret:                req.GoogleOAuthClientSecret,
		GoogleOAuthRedirectURL:                 req.GoogleOAuthRedirectURL,
		GoogleOAuthFrontendRedirectURL:         req.GoogleOAuthFrontendRedirectURL,
		SiteName:                               req.SiteName,
		SiteLogo:                               req.SiteLogo,
		SiteSubtitle:                           req.SiteSubtitle,
		APIBaseURL:                             req.APIBaseURL,
		ContactInfo:                            req.ContactInfo,
		DocURL:                                 req.DocURL,
		HomeContent:                            req.HomeContent,
		HideCcsImportButton:                    req.HideCcsImportButton,
		PurchaseSubscriptionEnabled:            purchaseEnabled,
		PurchaseSubscriptionURL:                purchaseURL,
		TableDefaultPageSize:                   req.TableDefaultPageSize,
		TablePageSizeOptions:                   req.TablePageSizeOptions,
		CustomMenuItems:                        customMenuJSON,
		CustomEndpoints:                        customEndpointsJSON,
		DefaultConcurrency:                     req.DefaultConcurrency,
		DefaultBalance:                         req.DefaultBalance,
		AffiliateRebateRate:                    affiliateRebateRate,
		AffiliateRebateFreezeHours:             affiliateRebateFreezeHours,
		AffiliateRebateDurationDays:            affiliateRebateDurationDays,
		AffiliateRebatePerInviteeCap:           affiliateRebatePerInviteeCap,
		AdminRechargeRebateEnabled:             adminRechargeRebateEnabled,
		DefaultUserRPMLimit:                    req.DefaultUserRPMLimit,
		DefaultSubscriptions:                   defaultSubscriptions,
		EnableModelFallback:                    req.EnableModelFallback,
		FallbackModelAnthropic:                 req.FallbackModelAnthropic,
		FallbackModelOpenAI:                    req.FallbackModelOpenAI,
		FallbackModelGemini:                    req.FallbackModelGemini,
		FallbackModelAntigravity:               req.FallbackModelAntigravity,
		EnableIdentityPatch:                    req.EnableIdentityPatch,
		IdentityPatchPrompt:                    req.IdentityPatchPrompt,
		MinClaudeCodeVersion:                   req.MinClaudeCodeVersion,
		MaxClaudeCodeVersion:                   req.MaxClaudeCodeVersion,
		AllowUngroupedKeyScheduling:            req.AllowUngroupedKeyScheduling,
		BackendModeEnabled:                     req.BackendModeEnabled,
		AllowUserViewErrorRequests: func() bool {
			if req.AllowUserViewErrorRequests != nil {
				return *req.AllowUserViewErrorRequests
			}
			return previousSettings.AllowUserViewErrorRequests
		}(),
		OpsMonitoringEnabled: func() bool {
			if req.OpsMonitoringEnabled != nil {
				return *req.OpsMonitoringEnabled
			}
			return previousSettings.OpsMonitoringEnabled
		}(),
		OpsRealtimeMonitoringEnabled: func() bool {
			if req.OpsRealtimeMonitoringEnabled != nil {
				return *req.OpsRealtimeMonitoringEnabled
			}
			return previousSettings.OpsRealtimeMonitoringEnabled
		}(),
		OpsQueryModeDefault: func() string {
			if req.OpsQueryModeDefault != nil {
				return *req.OpsQueryModeDefault
			}
			return previousSettings.OpsQueryModeDefault
		}(),
		OpsMetricsIntervalSeconds: func() int {
			if req.OpsMetricsIntervalSeconds != nil {
				return *req.OpsMetricsIntervalSeconds
			}
			return previousSettings.OpsMetricsIntervalSeconds
		}(),
		EnableFingerprintUnification: func() bool {
			if req.EnableFingerprintUnification != nil {
				return *req.EnableFingerprintUnification
			}
			return previousSettings.EnableFingerprintUnification
		}(),
		EnableMetadataPassthrough: func() bool {
			if req.EnableMetadataPassthrough != nil {
				return *req.EnableMetadataPassthrough
			}
			return previousSettings.EnableMetadataPassthrough
		}(),
		EnableCCHSigning: func() bool {
			if req.EnableCCHSigning != nil {
				return *req.EnableCCHSigning
			}
			return previousSettings.EnableCCHSigning
		}(),
		EnableClaudeOAuthSystemPromptInjection: func() bool {
			if req.EnableClaudeOAuthSystemPromptInjection != nil {
				return *req.EnableClaudeOAuthSystemPromptInjection
			}
			return previousSettings.EnableClaudeOAuthSystemPromptInjection
		}(),
		ClaudeOAuthSystemPrompt: func() string {
			if req.ClaudeOAuthSystemPrompt != nil {
				return *req.ClaudeOAuthSystemPrompt
			}
			return previousSettings.ClaudeOAuthSystemPrompt
		}(),
		ClaudeOAuthSystemPromptBlocks: func() string {
			if req.ClaudeOAuthSystemPromptBlocks != nil {
				return *req.ClaudeOAuthSystemPromptBlocks
			}
			return previousSettings.ClaudeOAuthSystemPromptBlocks
		}(),
		EnableAnthropicCacheTTL1hInjection: func() bool {
			if req.EnableAnthropicCacheTTL1hInjection != nil {
				return *req.EnableAnthropicCacheTTL1hInjection
			}
			return previousSettings.EnableAnthropicCacheTTL1hInjection
		}(),
		RewriteMessageCacheControl: func() bool {
			if req.RewriteMessageCacheControl != nil {
				return *req.RewriteMessageCacheControl
			}
			return previousSettings.RewriteMessageCacheControl
		}(),
		EnableClientDatelineNormalization: func() bool {
			if req.EnableClientDatelineNormalization != nil {
				return *req.EnableClientDatelineNormalization
			}
			return previousSettings.EnableClientDatelineNormalization
		}(),
		AntigravityUserAgentVersion: func() string {
			if req.AntigravityUserAgentVersion != nil {
				return *req.AntigravityUserAgentVersion
			}
			return previousSettings.AntigravityUserAgentVersion
		}(),
		OpenAICodexUserAgent: func() string {
			if req.OpenAICodexUserAgent != nil {
				return *req.OpenAICodexUserAgent
			}
			return previousSettings.OpenAICodexUserAgent
		}(),
		MinCodexVersion:       strings.TrimSpace(req.MinCodexVersion),
		MaxCodexVersion:       strings.TrimSpace(req.MaxCodexVersion),
		CodexCLIOnlyBlacklist: strings.TrimSpace(req.CodexCLIOnlyBlacklist),
		CodexCLIOnlyWhitelist: strings.TrimSpace(req.CodexCLIOnlyWhitelist),
		CodexCLIOnlyAllowAppServerClients: func() bool {
			if req.CodexCLIOnlyAllowAppServerClients != nil {
				return *req.CodexCLIOnlyAllowAppServerClients
			}
			return previousSettings.CodexCLIOnlyAllowAppServerClients
		}(),
		CodexCLIOnlyEngineFingerprintSignals: strings.TrimSpace(req.CodexCLIOnlyEngineFingerprintSignals),
		PaymentVisibleMethodAlipaySource: func() string {
			if req.PaymentVisibleMethodAlipaySource != nil {
				return strings.TrimSpace(*req.PaymentVisibleMethodAlipaySource)
			}
			return previousSettings.PaymentVisibleMethodAlipaySource
		}(),
		PaymentVisibleMethodWxpaySource: func() string {
			if req.PaymentVisibleMethodWxpaySource != nil {
				return strings.TrimSpace(*req.PaymentVisibleMethodWxpaySource)
			}
			return previousSettings.PaymentVisibleMethodWxpaySource
		}(),
		PaymentVisibleMethodAlipayEnabled: func() bool {
			if req.PaymentVisibleMethodAlipayEnabled != nil {
				return *req.PaymentVisibleMethodAlipayEnabled
			}
			return previousSettings.PaymentVisibleMethodAlipayEnabled
		}(),
		PaymentVisibleMethodWxpayEnabled: func() bool {
			if req.PaymentVisibleMethodWxpayEnabled != nil {
				return *req.PaymentVisibleMethodWxpayEnabled
			}
			return previousSettings.PaymentVisibleMethodWxpayEnabled
		}(),
		OpenAILowUpstreamRatePriorityEnabled: func() bool {
			if req.OpenAILowUpstreamRatePriorityEnabled != nil {
				return *req.OpenAILowUpstreamRatePriorityEnabled
			}
			return previousSettings.OpenAILowUpstreamRatePriorityEnabled
		}(),
		OpenAIOAuthSchedulingRateMultiplier: func() float64 {
			if req.OpenAIOAuthSchedulingRateMultiplier != nil {
				return *req.OpenAIOAuthSchedulingRateMultiplier
			}
			return previousSettings.OpenAIOAuthSchedulingRateMultiplier
		}(),
		OpenAIAdvancedSchedulerEnabled: func() bool {
			if req.OpenAIAdvancedSchedulerEnabled != nil {
				return *req.OpenAIAdvancedSchedulerEnabled
			}
			return previousSettings.OpenAIAdvancedSchedulerEnabled
		}(),
		OpenAIAdvancedSchedulerStickyWeightedEnabled: func() bool {
			if req.OpenAIAdvancedSchedulerStickyWeightedEnabled != nil {
				return *req.OpenAIAdvancedSchedulerStickyWeightedEnabled
			}
			return previousSettings.OpenAIAdvancedSchedulerStickyWeightedEnabled
		}(),
		OpenAIAdvancedSchedulerSubscriptionPriorityEnabled: func() bool {
			if req.OpenAIAdvancedSchedulerSubscriptionPriorityEnabled != nil {
				return *req.OpenAIAdvancedSchedulerSubscriptionPriorityEnabled
			}
			return previousSettings.OpenAIAdvancedSchedulerSubscriptionPriorityEnabled
		}(),
		OpenAIAdvancedSchedulerLBTopK:                 stringSetting(req.OpenAIAdvancedSchedulerLBTopK, previousSettings.OpenAIAdvancedSchedulerLBTopK),
		OpenAIAdvancedSchedulerWeightPriority:         stringSetting(req.OpenAIAdvancedSchedulerWeightPriority, previousSettings.OpenAIAdvancedSchedulerWeightPriority),
		OpenAIAdvancedSchedulerWeightLoad:             stringSetting(req.OpenAIAdvancedSchedulerWeightLoad, previousSettings.OpenAIAdvancedSchedulerWeightLoad),
		OpenAIAdvancedSchedulerWeightQueue:            stringSetting(req.OpenAIAdvancedSchedulerWeightQueue, previousSettings.OpenAIAdvancedSchedulerWeightQueue),
		OpenAIAdvancedSchedulerWeightErrorRate:        stringSetting(req.OpenAIAdvancedSchedulerWeightErrorRate, previousSettings.OpenAIAdvancedSchedulerWeightErrorRate),
		OpenAIAdvancedSchedulerWeightTTFT:             stringSetting(req.OpenAIAdvancedSchedulerWeightTTFT, previousSettings.OpenAIAdvancedSchedulerWeightTTFT),
		OpenAIAdvancedSchedulerWeightReset:            stringSetting(req.OpenAIAdvancedSchedulerWeightReset, previousSettings.OpenAIAdvancedSchedulerWeightReset),
		OpenAIAdvancedSchedulerWeightQuotaHeadroom:    stringSetting(req.OpenAIAdvancedSchedulerWeightQuotaHeadroom, previousSettings.OpenAIAdvancedSchedulerWeightQuotaHeadroom),
		OpenAIAdvancedSchedulerWeightUpstreamCost:     stringSetting(req.OpenAIAdvancedSchedulerWeightUpstreamCost, previousSettings.OpenAIAdvancedSchedulerWeightUpstreamCost),
		OpenAIAdvancedSchedulerWeightPreviousResponse: stringSetting(req.OpenAIAdvancedSchedulerWeightPreviousResponse, previousSettings.OpenAIAdvancedSchedulerWeightPreviousResponse),
		OpenAIAdvancedSchedulerWeightSessionSticky:    stringSetting(req.OpenAIAdvancedSchedulerWeightSessionSticky, previousSettings.OpenAIAdvancedSchedulerWeightSessionSticky),
		BalanceLowNotifyEnabled: func() bool {
			if req.BalanceLowNotifyEnabled != nil {
				return *req.BalanceLowNotifyEnabled
			}
			return previousSettings.BalanceLowNotifyEnabled
		}(),
		BalanceLowNotifyThreshold: func() float64 {
			if req.BalanceLowNotifyThreshold != nil {
				return *req.BalanceLowNotifyThreshold
			}
			return previousSettings.BalanceLowNotifyThreshold
		}(),
		BalanceLowNotifyRechargeURL: func() string {
			if req.BalanceLowNotifyRechargeURL != nil {
				return *req.BalanceLowNotifyRechargeURL
			}
			return previousSettings.BalanceLowNotifyRechargeURL
		}(),
		SubscriptionExpiryNotifyEnabled: func() bool {
			if req.SubscriptionExpiryNotifyEnabled != nil {
				return *req.SubscriptionExpiryNotifyEnabled
			}
			return previousSettings.SubscriptionExpiryNotifyEnabled
		}(),
		AccountQuotaNotifyEnabled: func() bool {
			if req.AccountQuotaNotifyEnabled != nil {
				return *req.AccountQuotaNotifyEnabled
			}
			return previousSettings.AccountQuotaNotifyEnabled
		}(),
		AccountQuotaNotifyEmails: func() []service.NotifyEmailEntry {
			if req.AccountQuotaNotifyEmails != nil {
				return dto.NotifyEmailEntriesToService(*req.AccountQuotaNotifyEmails)
			}
			return previousSettings.AccountQuotaNotifyEmails
		}(),
		ChannelMonitorEnabled: func() bool {
			if req.ChannelMonitorEnabled != nil {
				return *req.ChannelMonitorEnabled
			}
			return previousSettings.ChannelMonitorEnabled
		}(),
		ChannelMonitorDefaultIntervalSeconds: func() int {
			if req.ChannelMonitorDefaultIntervalSeconds != nil {
				return *req.ChannelMonitorDefaultIntervalSeconds
			}
			return previousSettings.ChannelMonitorDefaultIntervalSeconds
		}(),
		AvailableChannelsEnabled: func() bool {
			if req.AvailableChannelsEnabled != nil {
				return *req.AvailableChannelsEnabled
			}
			return previousSettings.AvailableChannelsEnabled
		}(),
		AffiliateEnabled: func() bool {
			if req.AffiliateEnabled != nil {
				return *req.AffiliateEnabled
			}
			return previousSettings.AffiliateEnabled
		}(),
		RiskControlEnabled: func() bool {
			if req.RiskControlEnabled != nil {
				return *req.RiskControlEnabled
			}
			return previousSettings.RiskControlEnabled
		}(),
		CyberSessionBlockEnabled: func() bool {
			if req.CyberSessionBlockEnabled != nil {
				return *req.CyberSessionBlockEnabled
			}
			return previousSettings.CyberSessionBlockEnabled
		}(),
		CyberSessionBlockTTLSeconds: func() int {
			if req.CyberSessionBlockTTLSeconds != nil {
				return *req.CyberSessionBlockTTLSeconds
			}
			return previousSettings.CyberSessionBlockTTLSeconds
		}(),
	}

	// req.AuthSourceXxxPlatformQuotas 为 nil 表示本次请求未包含该 source 的 quota 配置（保留 previousAuthSourceDefaults 中的值）；
	// non-nil（含 empty map）表示整体覆盖：empty map = 清空该 source 的所有 quota 配置。
	authSourceDefaults := &service.AuthSourceDefaultSettings{
		Email: service.ProviderDefaultGrantSettings{
			Balance:          float64ValueOrDefault(req.AuthSourceDefaultEmailBalance, previousAuthSourceDefaults.Email.Balance),
			Concurrency:      intValueOrDefault(req.AuthSourceDefaultEmailConcurrency, previousAuthSourceDefaults.Email.Concurrency),
			Subscriptions:    defaultSubscriptionsValueOrDefault(req.AuthSourceDefaultEmailSubscriptions, previousAuthSourceDefaults.Email.Subscriptions),
			GrantOnSignup:    boolValueOrDefault(req.AuthSourceDefaultEmailGrantOnSignup, previousAuthSourceDefaults.Email.GrantOnSignup),
			GrantOnFirstBind: boolValueOrDefault(req.AuthSourceDefaultEmailGrantOnFirstBind, previousAuthSourceDefaults.Email.GrantOnFirstBind),
			PlatformQuotas:   platformQuotasValueOrDefault(req.AuthSourceEmailPlatformQuotas, previousAuthSourceDefaults.Email.PlatformQuotas),
		},
		LinuxDo: service.ProviderDefaultGrantSettings{
			Balance:          float64ValueOrDefault(req.AuthSourceDefaultLinuxDoBalance, previousAuthSourceDefaults.LinuxDo.Balance),
			Concurrency:      intValueOrDefault(req.AuthSourceDefaultLinuxDoConcurrency, previousAuthSourceDefaults.LinuxDo.Concurrency),
			Subscriptions:    defaultSubscriptionsValueOrDefault(req.AuthSourceDefaultLinuxDoSubscriptions, previousAuthSourceDefaults.LinuxDo.Subscriptions),
			GrantOnSignup:    boolValueOrDefault(req.AuthSourceDefaultLinuxDoGrantOnSignup, previousAuthSourceDefaults.LinuxDo.GrantOnSignup),
			GrantOnFirstBind: boolValueOrDefault(req.AuthSourceDefaultLinuxDoGrantOnFirstBind, previousAuthSourceDefaults.LinuxDo.GrantOnFirstBind),
			PlatformQuotas:   platformQuotasValueOrDefault(req.AuthSourceLinuxDoPlatformQuotas, previousAuthSourceDefaults.LinuxDo.PlatformQuotas),
		},
		OIDC: service.ProviderDefaultGrantSettings{
			Balance:          float64ValueOrDefault(req.AuthSourceDefaultOIDCBalance, previousAuthSourceDefaults.OIDC.Balance),
			Concurrency:      intValueOrDefault(req.AuthSourceDefaultOIDCConcurrency, previousAuthSourceDefaults.OIDC.Concurrency),
			Subscriptions:    defaultSubscriptionsValueOrDefault(req.AuthSourceDefaultOIDCSubscriptions, previousAuthSourceDefaults.OIDC.Subscriptions),
			GrantOnSignup:    boolValueOrDefault(req.AuthSourceDefaultOIDCGrantOnSignup, previousAuthSourceDefaults.OIDC.GrantOnSignup),
			GrantOnFirstBind: boolValueOrDefault(req.AuthSourceDefaultOIDCGrantOnFirstBind, previousAuthSourceDefaults.OIDC.GrantOnFirstBind),
			PlatformQuotas:   platformQuotasValueOrDefault(req.AuthSourceOIDCPlatformQuotas, previousAuthSourceDefaults.OIDC.PlatformQuotas),
		},
		WeChat: service.ProviderDefaultGrantSettings{
			Balance:          float64ValueOrDefault(req.AuthSourceDefaultWeChatBalance, previousAuthSourceDefaults.WeChat.Balance),
			Concurrency:      intValueOrDefault(req.AuthSourceDefaultWeChatConcurrency, previousAuthSourceDefaults.WeChat.Concurrency),
			Subscriptions:    defaultSubscriptionsValueOrDefault(req.AuthSourceDefaultWeChatSubscriptions, previousAuthSourceDefaults.WeChat.Subscriptions),
			GrantOnSignup:    boolValueOrDefault(req.AuthSourceDefaultWeChatGrantOnSignup, previousAuthSourceDefaults.WeChat.GrantOnSignup),
			GrantOnFirstBind: boolValueOrDefault(req.AuthSourceDefaultWeChatGrantOnFirstBind, previousAuthSourceDefaults.WeChat.GrantOnFirstBind),
			PlatformQuotas:   platformQuotasValueOrDefault(req.AuthSourceWeChatPlatformQuotas, previousAuthSourceDefaults.WeChat.PlatformQuotas),
		},
		GitHub: service.ProviderDefaultGrantSettings{
			Balance:          float64ValueOrDefault(req.AuthSourceDefaultGitHubBalance, previousAuthSourceDefaults.GitHub.Balance),
			Concurrency:      intValueOrDefault(req.AuthSourceDefaultGitHubConcurrency, previousAuthSourceDefaults.GitHub.Concurrency),
			Subscriptions:    defaultSubscriptionsValueOrDefault(req.AuthSourceDefaultGitHubSubscriptions, previousAuthSourceDefaults.GitHub.Subscriptions),
			GrantOnSignup:    boolValueOrDefault(req.AuthSourceDefaultGitHubGrantOnSignup, previousAuthSourceDefaults.GitHub.GrantOnSignup),
			GrantOnFirstBind: boolValueOrDefault(req.AuthSourceDefaultGitHubGrantOnFirstBind, previousAuthSourceDefaults.GitHub.GrantOnFirstBind),
			PlatformQuotas:   platformQuotasValueOrDefault(req.AuthSourceGitHubPlatformQuotas, previousAuthSourceDefaults.GitHub.PlatformQuotas),
		},
		Google: service.ProviderDefaultGrantSettings{
			Balance:          float64ValueOrDefault(req.AuthSourceDefaultGoogleBalance, previousAuthSourceDefaults.Google.Balance),
			Concurrency:      intValueOrDefault(req.AuthSourceDefaultGoogleConcurrency, previousAuthSourceDefaults.Google.Concurrency),
			Subscriptions:    defaultSubscriptionsValueOrDefault(req.AuthSourceDefaultGoogleSubscriptions, previousAuthSourceDefaults.Google.Subscriptions),
			GrantOnSignup:    boolValueOrDefault(req.AuthSourceDefaultGoogleGrantOnSignup, previousAuthSourceDefaults.Google.GrantOnSignup),
			GrantOnFirstBind: boolValueOrDefault(req.AuthSourceDefaultGoogleGrantOnFirstBind, previousAuthSourceDefaults.Google.GrantOnFirstBind),
			PlatformQuotas:   platformQuotasValueOrDefault(req.AuthSourceGooglePlatformQuotas, previousAuthSourceDefaults.Google.PlatformQuotas),
		},
		DingTalk: service.ProviderDefaultGrantSettings{
			Balance:          float64ValueOrDefault(req.AuthSourceDefaultDingTalkBalance, previousAuthSourceDefaults.DingTalk.Balance),
			Concurrency:      intValueOrDefault(req.AuthSourceDefaultDingTalkConcurrency, previousAuthSourceDefaults.DingTalk.Concurrency),
			Subscriptions:    defaultSubscriptionsValueOrDefault(req.AuthSourceDefaultDingTalkSubscriptions, previousAuthSourceDefaults.DingTalk.Subscriptions),
			GrantOnSignup:    boolValueOrDefault(req.AuthSourceDefaultDingTalkGrantOnSignup, previousAuthSourceDefaults.DingTalk.GrantOnSignup),
			GrantOnFirstBind: boolValueOrDefault(req.AuthSourceDefaultDingTalkGrantOnFirstBind, previousAuthSourceDefaults.DingTalk.GrantOnFirstBind),
			PlatformQuotas:   platformQuotasValueOrDefault(req.AuthSourceDingTalkPlatformQuotas, previousAuthSourceDefaults.DingTalk.PlatformQuotas),
		},
		ForceEmailOnThirdPartySignup: boolValueOrDefault(req.ForceEmailOnThirdPartySignup, previousAuthSourceDefaults.ForceEmailOnThirdPartySignup),
	}
	if err := h.settingService.UpdateSettingsWithAuthSourceDefaults(c.Request.Context(), settings, authSourceDefaults); err != nil {
		response.ErrorFrom(c, err)
		return
	}

	// Update OpenAI fast policy (stored under dedicated key, only when provided).
	if req.OpenAIFastPolicySettings != nil {
		if err := h.settingService.SetOpenAIFastPolicySettings(c.Request.Context(), openaiFastPolicySettingsFromDTO(req.OpenAIFastPolicySettings)); err != nil {
			response.BadRequest(c, err.Error())
			return
		}
	}

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
			SubscriptionUSDToCNYRate:  req.PaymentSubscriptionUSDToCNYRate,
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
			AlipayForceQRCode:         req.PaymentAlipayForceQRCode,
		}
		if err := h.paymentConfigService.UpdatePaymentConfig(c.Request.Context(), paymentReq); err != nil {
			response.ErrorFrom(c, err)
			return
		}
		// Refresh in-memory provider registry so config changes take effect immediately
		if h.paymentService != nil {
			h.paymentService.RefreshProviders(c.Request.Context())
		}
	}

	h.auditSettingsUpdate(c, previousSettings, settings, previousAuthSourceDefaults, authSourceDefaults, req)

	// 重新获取设置返回
	updatedSettings, err := h.settingService.GetAllSettings(c.Request.Context())
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	h.ensureDingTalkSyncAttributes(c.Request.Context(), updatedSettings)
	updatedAuthSourceDefaults, err := h.settingService.GetAuthSourceDefaultSettings(c.Request.Context())
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	updatedDefaultSubscriptions := make([]dto.DefaultSubscriptionSetting, 0, len(updatedSettings.DefaultSubscriptions))
	for _, sub := range updatedSettings.DefaultSubscriptions {
		updatedDefaultSubscriptions = append(updatedDefaultSubscriptions, dto.DefaultSubscriptionSetting{
			GroupID:      sub.GroupID,
			ValidityDays: sub.ValidityDays,
		})
	}

	// Reload payment config for response
	var updatedPaymentCfg *service.PaymentConfig
	if h.paymentConfigService != nil {
		updatedPaymentCfg, _ = h.paymentConfigService.GetPaymentConfig(c.Request.Context())
	}
	if updatedPaymentCfg == nil {
		updatedPaymentCfg = &service.PaymentConfig{}
	}

	payload := dto.SystemSettings{
		RegistrationEnabled:                                    updatedSettings.RegistrationEnabled,
		EmailVerifyEnabled:                                     updatedSettings.EmailVerifyEnabled,
		RegistrationEmailSuffixWhitelist:                       updatedSettings.RegistrationEmailSuffixWhitelist,
		PromoCodeEnabled:                                       updatedSettings.PromoCodeEnabled,
		PasswordResetEnabled:                                   updatedSettings.PasswordResetEnabled,
		FrontendURL:                                            updatedSettings.FrontendURL,
		InvitationCodeEnabled:                                  updatedSettings.InvitationCodeEnabled,
		TotpEnabled:                                            updatedSettings.TotpEnabled,
		TotpEncryptionKeyConfigured:                            h.settingService.IsTotpEncryptionKeyConfigured(),
		SessionBindingEnabled:                                  updatedSettings.SessionBindingEnabled,
		AuditLogRetentionDays:                                  updatedSettings.AuditLogRetentionDays,
		LoginAgreementEnabled:                                  updatedSettings.LoginAgreementEnabled,
		LoginAgreementMode:                                     updatedSettings.LoginAgreementMode,
		LoginAgreementUpdatedAt:                                updatedSettings.LoginAgreementUpdatedAt,
		LoginAgreementDocuments:                                loginAgreementDocumentsToDTO(updatedSettings.LoginAgreementDocuments),
		SMTPHost:                                               updatedSettings.SMTPHost,
		SMTPPort:                                               updatedSettings.SMTPPort,
		SMTPUsername:                                           updatedSettings.SMTPUsername,
		SMTPPasswordConfigured:                                 updatedSettings.SMTPPasswordConfigured,
		SMTPFrom:                                               updatedSettings.SMTPFrom,
		SMTPFromName:                                           updatedSettings.SMTPFromName,
		SMTPUseTLS:                                             updatedSettings.SMTPUseTLS,
		TurnstileEnabled:                                       updatedSettings.TurnstileEnabled,
		TurnstileSiteKey:                                       updatedSettings.TurnstileSiteKey,
		TurnstileSecretKeyConfigured:                           updatedSettings.TurnstileSecretKeyConfigured,
		APIKeyACLTrustForwardedIP:                              updatedSettings.APIKeyACLTrustForwardedIP,
		LinuxDoConnectEnabled:                                  updatedSettings.LinuxDoConnectEnabled,
		LinuxDoConnectClientID:                                 updatedSettings.LinuxDoConnectClientID,
		LinuxDoConnectClientSecretConfigured:                   updatedSettings.LinuxDoConnectClientSecretConfigured,
		LinuxDoConnectRedirectURL:                              updatedSettings.LinuxDoConnectRedirectURL,
		DingTalkConnectEnabled:                                 updatedSettings.DingTalkConnectEnabled,
		DingTalkConnectClientID:                                updatedSettings.DingTalkConnectClientID,
		DingTalkConnectClientSecretConfigured:                  updatedSettings.DingTalkConnectClientSecretConfigured,
		DingTalkConnectRedirectURL:                             updatedSettings.DingTalkConnectRedirectURL,
		DingTalkConnectCorpRestrictionPolicy:                   updatedSettings.DingTalkConnectCorpRestrictionPolicy,
		DingTalkConnectInternalCorpID:                          updatedSettings.DingTalkConnectInternalCorpID,
		DingTalkConnectBypassRegistration:                      updatedSettings.DingTalkConnectBypassRegistration,
		DingTalkConnectSyncCorpEmail:                           updatedSettings.DingTalkConnectSyncCorpEmail,
		DingTalkConnectSyncDisplayName:                         updatedSettings.DingTalkConnectSyncDisplayName,
		DingTalkConnectSyncDept:                                updatedSettings.DingTalkConnectSyncDept,
		DingTalkConnectSyncCorpEmailAttrKey:                    updatedSettings.DingTalkConnectSyncCorpEmailAttrKey,
		DingTalkConnectSyncDisplayNameAttrKey:                  updatedSettings.DingTalkConnectSyncDisplayNameAttrKey,
		DingTalkConnectSyncDeptAttrKey:                         updatedSettings.DingTalkConnectSyncDeptAttrKey,
		DingTalkConnectSyncCorpEmailAttrName:                   updatedSettings.DingTalkConnectSyncCorpEmailAttrName,
		DingTalkConnectSyncDisplayNameAttrName:                 updatedSettings.DingTalkConnectSyncDisplayNameAttrName,
		DingTalkConnectSyncDeptAttrName:                        updatedSettings.DingTalkConnectSyncDeptAttrName,
		WeChatConnectEnabled:                                   updatedSettings.WeChatConnectEnabled,
		WeChatConnectAppID:                                     updatedSettings.WeChatConnectAppID,
		WeChatConnectAppSecretConfigured:                       updatedSettings.WeChatConnectAppSecretConfigured,
		WeChatConnectOpenAppID:                                 updatedSettings.WeChatConnectOpenAppID,
		WeChatConnectOpenAppSecretConfigured:                   updatedSettings.WeChatConnectOpenAppSecretConfigured,
		WeChatConnectMPAppID:                                   updatedSettings.WeChatConnectMPAppID,
		WeChatConnectMPAppSecretConfigured:                     updatedSettings.WeChatConnectMPAppSecretConfigured,
		WeChatConnectMobileAppID:                               updatedSettings.WeChatConnectMobileAppID,
		WeChatConnectMobileAppSecretConfigured:                 updatedSettings.WeChatConnectMobileAppSecretConfigured,
		WeChatConnectOpenEnabled:                               updatedSettings.WeChatConnectOpenEnabled,
		WeChatConnectMPEnabled:                                 updatedSettings.WeChatConnectMPEnabled,
		WeChatConnectMobileEnabled:                             updatedSettings.WeChatConnectMobileEnabled,
		WeChatConnectMode:                                      updatedSettings.WeChatConnectMode,
		WeChatConnectScopes:                                    updatedSettings.WeChatConnectScopes,
		WeChatConnectRedirectURL:                               updatedSettings.WeChatConnectRedirectURL,
		WeChatConnectFrontendRedirectURL:                       updatedSettings.WeChatConnectFrontendRedirectURL,
		OIDCConnectEnabled:                                     updatedSettings.OIDCConnectEnabled,
		OIDCConnectProviderName:                                updatedSettings.OIDCConnectProviderName,
		OIDCConnectClientID:                                    updatedSettings.OIDCConnectClientID,
		OIDCConnectClientSecretConfigured:                      updatedSettings.OIDCConnectClientSecretConfigured,
		OIDCConnectIssuerURL:                                   updatedSettings.OIDCConnectIssuerURL,
		OIDCConnectDiscoveryURL:                                updatedSettings.OIDCConnectDiscoveryURL,
		OIDCConnectAuthorizeURL:                                updatedSettings.OIDCConnectAuthorizeURL,
		OIDCConnectTokenURL:                                    updatedSettings.OIDCConnectTokenURL,
		OIDCConnectUserInfoURL:                                 updatedSettings.OIDCConnectUserInfoURL,
		OIDCConnectJWKSURL:                                     updatedSettings.OIDCConnectJWKSURL,
		OIDCConnectScopes:                                      updatedSettings.OIDCConnectScopes,
		OIDCConnectRedirectURL:                                 updatedSettings.OIDCConnectRedirectURL,
		OIDCConnectFrontendRedirectURL:                         updatedSettings.OIDCConnectFrontendRedirectURL,
		OIDCConnectTokenAuthMethod:                             updatedSettings.OIDCConnectTokenAuthMethod,
		OIDCConnectUsePKCE:                                     updatedSettings.OIDCConnectUsePKCE,
		OIDCConnectValidateIDToken:                             updatedSettings.OIDCConnectValidateIDToken,
		OIDCConnectAllowedSigningAlgs:                          updatedSettings.OIDCConnectAllowedSigningAlgs,
		OIDCConnectClockSkewSeconds:                            updatedSettings.OIDCConnectClockSkewSeconds,
		OIDCConnectRequireEmailVerified:                        updatedSettings.OIDCConnectRequireEmailVerified,
		OIDCConnectUserInfoEmailPath:                           updatedSettings.OIDCConnectUserInfoEmailPath,
		OIDCConnectUserInfoIDPath:                              updatedSettings.OIDCConnectUserInfoIDPath,
		OIDCConnectUserInfoUsernamePath:                        updatedSettings.OIDCConnectUserInfoUsernamePath,
		GitHubOAuthEnabled:                                     updatedSettings.GitHubOAuthEnabled,
		GitHubOAuthClientID:                                    updatedSettings.GitHubOAuthClientID,
		GitHubOAuthClientSecretConfigured:                      updatedSettings.GitHubOAuthClientSecretConfigured,
		GitHubOAuthRedirectURL:                                 updatedSettings.GitHubOAuthRedirectURL,
		GitHubOAuthFrontendRedirectURL:                         updatedSettings.GitHubOAuthFrontendRedirectURL,
		GoogleOAuthEnabled:                                     updatedSettings.GoogleOAuthEnabled,
		GoogleOAuthClientID:                                    updatedSettings.GoogleOAuthClientID,
		GoogleOAuthClientSecretConfigured:                      updatedSettings.GoogleOAuthClientSecretConfigured,
		GoogleOAuthRedirectURL:                                 updatedSettings.GoogleOAuthRedirectURL,
		GoogleOAuthFrontendRedirectURL:                         updatedSettings.GoogleOAuthFrontendRedirectURL,
		SiteName:                                               updatedSettings.SiteName,
		SiteLogo:                                               updatedSettings.SiteLogo,
		SiteSubtitle:                                           updatedSettings.SiteSubtitle,
		APIBaseURL:                                             updatedSettings.APIBaseURL,
		ContactInfo:                                            updatedSettings.ContactInfo,
		DocURL:                                                 updatedSettings.DocURL,
		HomeContent:                                            updatedSettings.HomeContent,
		HideCcsImportButton:                                    updatedSettings.HideCcsImportButton,
		PurchaseSubscriptionEnabled:                            updatedSettings.PurchaseSubscriptionEnabled,
		PurchaseSubscriptionURL:                                updatedSettings.PurchaseSubscriptionURL,
		TableDefaultPageSize:                                   updatedSettings.TableDefaultPageSize,
		TablePageSizeOptions:                                   updatedSettings.TablePageSizeOptions,
		CustomMenuItems:                                        dto.ParseCustomMenuItems(updatedSettings.CustomMenuItems),
		CustomEndpoints:                                        dto.ParseCustomEndpoints(updatedSettings.CustomEndpoints),
		DefaultConcurrency:                                     updatedSettings.DefaultConcurrency,
		DefaultBalance:                                         updatedSettings.DefaultBalance,
		AffiliateRebateRate:                                    updatedSettings.AffiliateRebateRate,
		AffiliateRebateFreezeHours:                             updatedSettings.AffiliateRebateFreezeHours,
		AffiliateRebateDurationDays:                            updatedSettings.AffiliateRebateDurationDays,
		AffiliateRebatePerInviteeCap:                           updatedSettings.AffiliateRebatePerInviteeCap,
		AdminRechargeRebateEnabled:                             updatedSettings.AdminRechargeRebateEnabled,
		DefaultUserRPMLimit:                                    updatedSettings.DefaultUserRPMLimit,
		DefaultSubscriptions:                                   updatedDefaultSubscriptions,
		EnableModelFallback:                                    updatedSettings.EnableModelFallback,
		FallbackModelAnthropic:                                 updatedSettings.FallbackModelAnthropic,
		FallbackModelOpenAI:                                    updatedSettings.FallbackModelOpenAI,
		FallbackModelGemini:                                    updatedSettings.FallbackModelGemini,
		FallbackModelAntigravity:                               updatedSettings.FallbackModelAntigravity,
		EnableIdentityPatch:                                    updatedSettings.EnableIdentityPatch,
		IdentityPatchPrompt:                                    updatedSettings.IdentityPatchPrompt,
		OpsMonitoringEnabled:                                   updatedSettings.OpsMonitoringEnabled,
		OpsRealtimeMonitoringEnabled:                           updatedSettings.OpsRealtimeMonitoringEnabled,
		OpsQueryModeDefault:                                    updatedSettings.OpsQueryModeDefault,
		OpsMetricsIntervalSeconds:                              updatedSettings.OpsMetricsIntervalSeconds,
		MinClaudeCodeVersion:                                   updatedSettings.MinClaudeCodeVersion,
		MaxClaudeCodeVersion:                                   updatedSettings.MaxClaudeCodeVersion,
		AllowUngroupedKeyScheduling:                            updatedSettings.AllowUngroupedKeyScheduling,
		BackendModeEnabled:                                     updatedSettings.BackendModeEnabled,
		EnableFingerprintUnification:                           updatedSettings.EnableFingerprintUnification,
		EnableMetadataPassthrough:                              updatedSettings.EnableMetadataPassthrough,
		EnableCCHSigning:                                       updatedSettings.EnableCCHSigning,
		EnableClaudeOAuthSystemPromptInjection:                 updatedSettings.EnableClaudeOAuthSystemPromptInjection,
		ClaudeOAuthSystemPrompt:                                updatedSettings.ClaudeOAuthSystemPrompt,
		ClaudeOAuthSystemPromptBlocks:                          updatedSettings.ClaudeOAuthSystemPromptBlocks,
		EnableAnthropicCacheTTL1hInjection:                     updatedSettings.EnableAnthropicCacheTTL1hInjection,
		RewriteMessageCacheControl:                             updatedSettings.RewriteMessageCacheControl,
		EnableClientDatelineNormalization:                      updatedSettings.EnableClientDatelineNormalization,
		AntigravityUserAgentVersion:                            updatedSettings.AntigravityUserAgentVersion,
		OpenAICodexUserAgent:                                   updatedSettings.OpenAICodexUserAgent,
		MinCodexVersion:                                        updatedSettings.MinCodexVersion,
		MaxCodexVersion:                                        updatedSettings.MaxCodexVersion,
		CodexCLIOnlyBlacklist:                                  updatedSettings.CodexCLIOnlyBlacklist,
		CodexCLIOnlyWhitelist:                                  updatedSettings.CodexCLIOnlyWhitelist,
		CodexCLIOnlyAllowAppServerClients:                      updatedSettings.CodexCLIOnlyAllowAppServerClients,
		CodexCLIOnlyEngineFingerprintSignals:                   updatedSettings.CodexCLIOnlyEngineFingerprintSignals,
		PaymentVisibleMethodAlipaySource:                       updatedSettings.PaymentVisibleMethodAlipaySource,
		PaymentVisibleMethodWxpaySource:                        updatedSettings.PaymentVisibleMethodWxpaySource,
		PaymentVisibleMethodAlipayEnabled:                      updatedSettings.PaymentVisibleMethodAlipayEnabled,
		PaymentVisibleMethodWxpayEnabled:                       updatedSettings.PaymentVisibleMethodWxpayEnabled,
		OpenAILowUpstreamRatePriorityEnabled:                   updatedSettings.OpenAILowUpstreamRatePriorityEnabled,
		OpenAIOAuthSchedulingRateMultiplier:                    updatedSettings.OpenAIOAuthSchedulingRateMultiplier,
		OpenAIAdvancedSchedulerEnabled:                         updatedSettings.OpenAIAdvancedSchedulerEnabled,
		OpenAIAdvancedSchedulerStickyWeightedEnabled:           updatedSettings.OpenAIAdvancedSchedulerStickyWeightedEnabled,
		OpenAIAdvancedSchedulerSubscriptionPriorityEnabled:     updatedSettings.OpenAIAdvancedSchedulerSubscriptionPriorityEnabled,
		OpenAIAdvancedSchedulerLBTopK:                          updatedSettings.OpenAIAdvancedSchedulerLBTopK,
		OpenAIAdvancedSchedulerWeightPriority:                  updatedSettings.OpenAIAdvancedSchedulerWeightPriority,
		OpenAIAdvancedSchedulerWeightLoad:                      updatedSettings.OpenAIAdvancedSchedulerWeightLoad,
		OpenAIAdvancedSchedulerWeightQueue:                     updatedSettings.OpenAIAdvancedSchedulerWeightQueue,
		OpenAIAdvancedSchedulerWeightErrorRate:                 updatedSettings.OpenAIAdvancedSchedulerWeightErrorRate,
		OpenAIAdvancedSchedulerWeightTTFT:                      updatedSettings.OpenAIAdvancedSchedulerWeightTTFT,
		OpenAIAdvancedSchedulerWeightReset:                     updatedSettings.OpenAIAdvancedSchedulerWeightReset,
		OpenAIAdvancedSchedulerWeightQuotaHeadroom:             updatedSettings.OpenAIAdvancedSchedulerWeightQuotaHeadroom,
		OpenAIAdvancedSchedulerWeightUpstreamCost:              updatedSettings.OpenAIAdvancedSchedulerWeightUpstreamCost,
		OpenAIAdvancedSchedulerWeightPreviousResponse:          updatedSettings.OpenAIAdvancedSchedulerWeightPreviousResponse,
		OpenAIAdvancedSchedulerWeightSessionSticky:             updatedSettings.OpenAIAdvancedSchedulerWeightSessionSticky,
		OpenAIAdvancedSchedulerEffectiveLBTopK:                 updatedSettings.OpenAIAdvancedSchedulerEffectiveLBTopK,
		OpenAIAdvancedSchedulerEffectiveWeightPriority:         updatedSettings.OpenAIAdvancedSchedulerEffectiveWeightPriority,
		OpenAIAdvancedSchedulerEffectiveWeightLoad:             updatedSettings.OpenAIAdvancedSchedulerEffectiveWeightLoad,
		OpenAIAdvancedSchedulerEffectiveWeightQueue:            updatedSettings.OpenAIAdvancedSchedulerEffectiveWeightQueue,
		OpenAIAdvancedSchedulerEffectiveWeightErrorRate:        updatedSettings.OpenAIAdvancedSchedulerEffectiveWeightErrorRate,
		OpenAIAdvancedSchedulerEffectiveWeightTTFT:             updatedSettings.OpenAIAdvancedSchedulerEffectiveWeightTTFT,
		OpenAIAdvancedSchedulerEffectiveWeightReset:            updatedSettings.OpenAIAdvancedSchedulerEffectiveWeightReset,
		OpenAIAdvancedSchedulerEffectiveWeightQuotaHeadroom:    updatedSettings.OpenAIAdvancedSchedulerEffectiveWeightQuotaHeadroom,
		OpenAIAdvancedSchedulerEffectiveWeightUpstreamCost:     updatedSettings.OpenAIAdvancedSchedulerEffectiveWeightUpstreamCost,
		OpenAIAdvancedSchedulerEffectiveWeightPreviousResponse: updatedSettings.OpenAIAdvancedSchedulerEffectiveWeightPreviousResponse,
		OpenAIAdvancedSchedulerEffectiveWeightSessionSticky:    updatedSettings.OpenAIAdvancedSchedulerEffectiveWeightSessionSticky,
		BalanceLowNotifyEnabled:                                updatedSettings.BalanceLowNotifyEnabled,
		BalanceLowNotifyThreshold:                              updatedSettings.BalanceLowNotifyThreshold,
		BalanceLowNotifyRechargeURL:                            updatedSettings.BalanceLowNotifyRechargeURL,
		SubscriptionExpiryNotifyEnabled:                        updatedSettings.SubscriptionExpiryNotifyEnabled,
		AccountQuotaNotifyEnabled:                              updatedSettings.AccountQuotaNotifyEnabled,
		AccountQuotaNotifyEmails:                               dto.NotifyEmailEntriesFromService(updatedSettings.AccountQuotaNotifyEmails),
		PaymentEnabled:                                         updatedPaymentCfg.Enabled,
		PaymentMinAmount:                                       updatedPaymentCfg.MinAmount,
		PaymentMaxAmount:                                       updatedPaymentCfg.MaxAmount,
		PaymentDailyLimit:                                      updatedPaymentCfg.DailyLimit,
		PaymentOrderTimeoutMin:                                 updatedPaymentCfg.OrderTimeoutMin,
		PaymentMaxPendingOrders:                                updatedPaymentCfg.MaxPendingOrders,
		PaymentEnabledTypes:                                    updatedPaymentCfg.EnabledTypes,
		PaymentBalanceDisabled:                                 updatedPaymentCfg.BalanceDisabled,
		PaymentBalanceRechargeMultiplier:                       updatedPaymentCfg.BalanceRechargeMultiplier,
		PaymentSubscriptionUSDToCNYRate:                        updatedPaymentCfg.SubscriptionUSDToCNYRate,
		PaymentRechargeFeeRate:                                 updatedPaymentCfg.RechargeFeeRate,
		PaymentLoadBalanceStrat:                                updatedPaymentCfg.LoadBalanceStrategy,
		PaymentProductNamePrefix:                               updatedPaymentCfg.ProductNamePrefix,
		PaymentProductNameSuffix:                               updatedPaymentCfg.ProductNameSuffix,
		PaymentHelpImageURL:                                    updatedPaymentCfg.HelpImageURL,
		PaymentHelpText:                                        updatedPaymentCfg.HelpText,
		PaymentCancelRateLimitEnabled:                          updatedPaymentCfg.CancelRateLimitEnabled,
		PaymentCancelRateLimitMax:                              updatedPaymentCfg.CancelRateLimitMax,
		PaymentCancelRateLimitWindow:                           updatedPaymentCfg.CancelRateLimitWindow,
		PaymentCancelRateLimitUnit:                             updatedPaymentCfg.CancelRateLimitUnit,
		PaymentCancelRateLimitMode:                             updatedPaymentCfg.CancelRateLimitMode,
		PaymentAlipayForceQRCode:                               updatedPaymentCfg.AlipayForceQRCode,

		ChannelMonitorEnabled:                updatedSettings.ChannelMonitorEnabled,
		ChannelMonitorDefaultIntervalSeconds: updatedSettings.ChannelMonitorDefaultIntervalSeconds,

		AvailableChannelsEnabled: updatedSettings.AvailableChannelsEnabled,

		AffiliateEnabled: updatedSettings.AffiliateEnabled,

		RiskControlEnabled:          updatedSettings.RiskControlEnabled,
		CyberSessionBlockEnabled:    updatedSettings.CyberSessionBlockEnabled,
		CyberSessionBlockTTLSeconds: updatedSettings.CyberSessionBlockTTLSeconds,
		AllowUserViewErrorRequests:  updatedSettings.AllowUserViewErrorRequests,
	}
	if fastPolicy, err := h.settingService.GetOpenAIFastPolicySettings(c.Request.Context()); err != nil {
		slog.Error("openai_fast_policy_settings_get_failed", "error", err)
	} else if fastPolicy != nil {
		payload.OpenAIFastPolicySettings = openaiFastPolicySettingsToDTO(fastPolicy)
	}

	// Default platform quotas（JSON map）—— 与 GetSettings 一致，避免保存后响应缺失该字段
	if platformQuotas, err := h.settingService.GetDefaultPlatformQuotas(c.Request.Context()); err != nil {
		slog.Error("default_platform_quotas_get_failed", "error", err)
	} else {
		payload.DefaultPlatformQuotas = platformQuotas
	}
	response.Success(c, systemSettingsResponseData(payload, updatedAuthSourceDefaults))
}

// hasPaymentFields returns true if any payment-related field was explicitly provided.
// mapDingTalkValidateError maps ValidateDingTalkConfig errors to machine-readable reason codes.
func mapDingTalkValidateError(err error) string {
	switch {
	case errors.Is(err, config.ErrDingTalkV1AppTypeMismatch):
		return "dingtalk_apptype_mismatch"
	case errors.Is(err, config.ErrDingTalkV4InvalidAppKind):
		return "dingtalk_app_kind_invalid"
	default:
		return "dingtalk_corp_config_invalid"
	}
}

func hasPaymentFields(req UpdateSettingsRequest) bool {
	return req.PaymentEnabled != nil || req.PaymentMinAmount != nil ||
		req.PaymentMaxAmount != nil || req.PaymentDailyLimit != nil ||
		req.PaymentOrderTimeoutMin != nil || req.PaymentMaxPendingOrders != nil ||
		req.PaymentEnabledTypes != nil || req.PaymentBalanceDisabled != nil ||
		req.PaymentBalanceRechargeMultiplier != nil || req.PaymentSubscriptionUSDToCNYRate != nil ||
		req.PaymentRechargeFeeRate != nil ||
		req.PaymentLoadBalanceStrat != nil || req.PaymentProductNamePrefix != nil ||
		req.PaymentProductNameSuffix != nil || req.PaymentHelpImageURL != nil ||
		req.PaymentHelpText != nil || req.PaymentCancelRateLimitEnabled != nil ||
		req.PaymentCancelRateLimitMax != nil || req.PaymentCancelRateLimitWindow != nil ||
		req.PaymentCancelRateLimitUnit != nil || req.PaymentCancelRateLimitMode != nil ||
		req.PaymentAlipayForceQRCode != nil
}

// ensureDingTalkSyncAttributes 在保存 settings 后，按 admin 配置的 (attr key, attr name)
// 兜底 upsert 对应 user attribute definition：不存在则创建；存在但 name 不同则更新 name
// （type/options/required 不变）。仅 internal_only + 对应 sync 开关开启时执行。
// 失败仅记录日志，不阻塞 settings 保存。
func (h *SettingHandler) ensureDingTalkSyncAttributes(ctx context.Context, settings *service.SystemSettings) {
	if h.userAttributeService == nil || settings == nil {
		return
	}
	if settings.DingTalkConnectCorpRestrictionPolicy != "internal_only" {
		return
	}
	if settings.DingTalkConnectSyncDisplayName {
		h.ensureUserAttributeDefinition(ctx, settings.DingTalkConnectSyncDisplayNameAttrKey, settings.DingTalkConnectSyncDisplayNameAttrName, "钉钉 internal_only 登录时同步的钉钉姓名", service.AttributeTypeText)
	}
	if settings.DingTalkConnectSyncCorpEmail {
		h.ensureUserAttributeDefinition(ctx, settings.DingTalkConnectSyncCorpEmailAttrKey, settings.DingTalkConnectSyncCorpEmailAttrName, "钉钉 internal_only 登录时同步的企业邮箱", service.AttributeTypeEmail)
	}
	if settings.DingTalkConnectSyncDept {
		h.ensureUserAttributeDefinition(ctx, settings.DingTalkConnectSyncDeptAttrKey, settings.DingTalkConnectSyncDeptAttrName, "钉钉 internal_only 登录时同步的完整部门路径（如：公司/研发部）", service.AttributeTypeText)
	}
}

func (h *SettingHandler) ensureUserAttributeDefinition(ctx context.Context, key, name, description string, attrType service.UserAttributeType) {
	key = strings.TrimSpace(key)
	if key == "" {
		return
	}
	existing, err := h.userAttributeService.GetDefinitionByKey(ctx, key)
	if err == nil && existing != nil {
		if strings.TrimSpace(name) != "" && existing.Name != name {
			if _, err := h.userAttributeService.UpdateDefinition(ctx, existing.ID, service.UpdateAttributeDefinitionInput{
				Name: &name,
			}); err != nil {
				slog.Warn("dingtalk: update user attribute definition name failed", "key", key, "err", err.Error())
				return
			}
			slog.Info("dingtalk: updated user attribute definition name", "key", key, "name", name)
		}
		return
	}
	if _, err := h.userAttributeService.CreateDefinition(ctx, service.CreateAttributeDefinitionInput{
		Key:         key,
		Name:        name,
		Description: description,
		Type:        attrType,
		Enabled:     true,
	}); err != nil {
		slog.Warn("dingtalk: ensure user attribute definition failed", "key", key, "err", err.Error())
		return
	}
	slog.Info("dingtalk: created user attribute definition", "key", key, "name", name, "type", attrType)
}
