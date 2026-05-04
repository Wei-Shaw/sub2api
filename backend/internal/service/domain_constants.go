package service

import "github.com/Wei-Shaw/sub2api/internal/domain"

// Status constants
const (
	StatusActive   = domain.StatusActive
	StatusDisabled = domain.StatusDisabled
	StatusError    = domain.StatusError
	StatusUnused   = domain.StatusUnused
	StatusUsed     = domain.StatusUsed
	StatusExpired  = domain.StatusExpired
)

// Role constants
const (
	RoleAdmin = domain.RoleAdmin
	RoleUser  = domain.RoleUser
)

// Affiliate rebate settings
const (
	AffiliateRebateRateDefault          = 20.0
	AffiliateRebateRateMin              = 0.0
	AffiliateRebateRateMax              = 100.0
	AffiliateEnabledDefault             = false // 邀请返利总开关默认关闭
	AffiliateRebateFreezeHoursDefault   = 0     // 0 = 不冻结（向后兼容）
	AffiliateRebateFreezeHoursMax       = 720   // 最大 30 天
	AffiliateRebateDurationDaysDefault  = 0     // 0 = 永久有效
	AffiliateRebateDurationDaysMax      = 3650  // ~10 年
	AffiliateRebatePerInviteeCapDefault = 0.0   // 0 = 无上限
)

// Platform constants
const (
	PlatformAnthropic   = domain.PlatformAnthropic
	PlatformOpenAI      = domain.PlatformOpenAI
	PlatformGemini      = domain.PlatformGemini
	PlatformAntigravity = domain.PlatformAntigravity
)

// Account type constants
const (
	AccountTypeOAuth          = domain.AccountTypeOAuth          // OAuth类型账号（full scope: profile + inference）
	AccountTypeSetupToken     = domain.AccountTypeSetupToken     // Setup Token类型账号（inference only scope）
	AccountTypeAPIKey         = domain.AccountTypeAPIKey         // API Key类型账号
	AccountTypeUpstream       = domain.AccountTypeUpstream       // 上游透传类型账号（通过 Base URL + API Key 连接上游）
	AccountTypeBedrock        = domain.AccountTypeBedrock        // AWS Bedrock 类型账号（通过 SigV4 签名或 API Key 连接 Bedrock，由 credentials.auth_mode 区分）
	AccountTypeServiceAccount = domain.AccountTypeServiceAccount // Google Service Account 类型账号（用于 Vertex AI）
)

// Redeem type constants
const (
	RedeemTypeBalance          = domain.RedeemTypeBalance
	RedeemTypeConcurrency      = domain.RedeemTypeConcurrency
	RedeemTypeSubscription     = domain.RedeemTypeSubscription
	RedeemTypeInvitation       = domain.RedeemTypeInvitation
	RedeemTypeAffiliateBalance = "affiliate_balance"
)

// PromoCode status constants
const (
	PromoCodeStatusActive   = domain.PromoCodeStatusActive
	PromoCodeStatusDisabled = domain.PromoCodeStatusDisabled
)

// Admin adjustment type constants
const (
	AdjustmentTypeAdminBalance     = domain.AdjustmentTypeAdminBalance     // 管理员调整余额
	AdjustmentTypeAdminConcurrency = domain.AdjustmentTypeAdminConcurrency // 管理员调整并发数
)

// Group subscription type constants
const (
	SubscriptionTypeStandard     = domain.SubscriptionTypeStandard     // 标准计费模式（按余额扣费）
	SubscriptionTypeSubscription = domain.SubscriptionTypeSubscription // 订阅模式（按限额控制）
)

// Subscription status constants
const (
	SubscriptionStatusActive    = domain.SubscriptionStatusActive
	SubscriptionStatusExpired   = domain.SubscriptionStatusExpired
	SubscriptionStatusSuspended = domain.SubscriptionStatusSuspended
)

// LinuxDoConnectSyntheticEmailDomain 是 LinuxDo Connect 用户的合成邮箱后缀（RFC 保留域名）。
const LinuxDoConnectSyntheticEmailDomain = "@linuxdo-connect.invalid"

// OIDCConnectSyntheticEmailDomain 是 OIDC 用户的合成邮箱后缀（RFC 保留域名）。
const OIDCConnectSyntheticEmailDomain = "@oidc-connect.invalid"

// WeChatConnectSyntheticEmailDomain 是 WeChat Connect 用户的合成邮箱后缀（RFC 保留域名）。
const WeChatConnectSyntheticEmailDomain = "@wechat-connect.invalid"

// Setting keys
const (
	// 注册设置
	SettingKeyRegistrationEnabled              = "registration_enabled"                // 是否开放注册
	SettingKeyEmailVerifyEnabled               = "email_verify_enabled"                // 是否开启邮件验证
	SettingKeyRegistrationEmailSuffixWhitelist = "registration_email_suffix_whitelist" // 注册邮箱后缀白名单（JSON 数组）
	SettingKeyPromoCodeEnabled                 = "promo_code_enabled"                  // 是否启用优惠码功能
	SettingKeyPasswordResetEnabled             = "password_reset_enabled"              // 是否启用忘记密码功能（需要先开启邮件验证）
	SettingKeyFrontendURL                      = "frontend_url"                        // 前端基础URL，用于生成邮件中的重置密码链接
	SettingKeyInvitationCodeEnabled            = "invitation_code_enabled"             // 是否启用邀请码注册
	SettingKeyAffiliateEnabled                 = "affiliate_enabled"                   // 邀请返利功能总开关
	SettingKeyAffiliateRebateRate              = "affiliate_rebate_rate"               // 邀请返利比例（百分比，0-100）
	SettingKeyAffiliateRebateFreezeHours       = "affiliate_rebate_freeze_hours"       // 返利冻结期（小时，0=不冻结）
	SettingKeyAffiliateRebateDurationDays      = "affiliate_rebate_duration_days"      // 返利有效期（天，0=永久）
	SettingKeyAffiliateRebatePerInviteeCap     = "affiliate_rebate_per_invitee_cap"    // 单人返利上限（0=无上限）

	// 邮件服务设置
	SettingKeySMTPHost     = "smtp_host"      // SMTP服务器地址
	SettingKeySMTPPort     = "smtp_port"      // SMTP端口
	SettingKeySMTPUsername = "smtp_username"  // SMTP用户名
	SettingKeySMTPPassword = "smtp_password"  // SMTP密码（加密存储）
	SettingKeySMTPFrom     = "smtp_from"      // 发件人地址
	SettingKeySMTPFromName = "smtp_from_name" // 发件人名称
	SettingKeySMTPUseTLS   = "smtp_use_tls"   // 是否使用TLS

	// Cloudflare Turnstile 设置
	SettingKeyTurnstileEnabled   = "turnstile_enabled"    // 是否启用 Turnstile 验证
	SettingKeyTurnstileSiteKey   = "turnstile_site_key"   // Turnstile Site Key
	SettingKeyTurnstileSecretKey = "turnstile_secret_key" // Turnstile Secret Key

	// TOTP 双因素认证设置
	SettingKeyTotpEnabled = "totp_enabled" // 是否启用 TOTP 2FA 功能

	// LinuxDo Connect OAuth 登录设置
	SettingKeyLinuxDoConnectEnabled      = "linuxdo_connect_enabled"
	SettingKeyLinuxDoConnectClientID     = "linuxdo_connect_client_id"
	SettingKeyLinuxDoConnectClientSecret = "linuxdo_connect_client_secret"
	SettingKeyLinuxDoConnectRedirectURL  = "linuxdo_connect_redirect_url"

	// WeChat Connect OAuth 登录设置
	SettingKeyWeChatConnectEnabled             = "wechat_connect_enabled"
	SettingKeyWeChatConnectAppID               = "wechat_connect_app_id"
	SettingKeyWeChatConnectAppSecret           = "wechat_connect_app_secret"
	SettingKeyWeChatConnectOpenAppID           = "wechat_connect_open_app_id"
	SettingKeyWeChatConnectOpenAppSecret       = "wechat_connect_open_app_secret"
	SettingKeyWeChatConnectMPAppID             = "wechat_connect_mp_app_id"
	SettingKeyWeChatConnectMPAppSecret         = "wechat_connect_mp_app_secret"
	SettingKeyWeChatConnectMobileAppID         = "wechat_connect_mobile_app_id"
	SettingKeyWeChatConnectMobileAppSecret     = "wechat_connect_mobile_app_secret"
	SettingKeyWeChatConnectOpenEnabled         = "wechat_connect_open_enabled"
	SettingKeyWeChatConnectMPEnabled           = "wechat_connect_mp_enabled"
	SettingKeyWeChatConnectMobileEnabled       = "wechat_connect_mobile_enabled"
	SettingKeyWeChatConnectMode                = "wechat_connect_mode"
	SettingKeyWeChatConnectScopes              = "wechat_connect_scopes"
	SettingKeyWeChatConnectRedirectURL         = "wechat_connect_redirect_url"
	SettingKeyWeChatConnectFrontendRedirectURL = "wechat_connect_frontend_redirect_url"

	// Generic OIDC OAuth 登录设置
	SettingKeyOIDCConnectEnabled              = "oidc_connect_enabled"
	SettingKeyOIDCConnectProviderName         = "oidc_connect_provider_name"
	SettingKeyOIDCConnectClientID             = "oidc_connect_client_id"
	SettingKeyOIDCConnectClientSecret         = "oidc_connect_client_secret"
	SettingKeyOIDCConnectIssuerURL            = "oidc_connect_issuer_url"
	SettingKeyOIDCConnectDiscoveryURL         = "oidc_connect_discovery_url"
	SettingKeyOIDCConnectAuthorizeURL         = "oidc_connect_authorize_url"
	SettingKeyOIDCConnectTokenURL             = "oidc_connect_token_url"
	SettingKeyOIDCConnectUserInfoURL          = "oidc_connect_userinfo_url"
	SettingKeyOIDCConnectJWKSURL              = "oidc_connect_jwks_url"
	SettingKeyOIDCConnectScopes               = "oidc_connect_scopes"
	SettingKeyOIDCConnectRedirectURL          = "oidc_connect_redirect_url"
	SettingKeyOIDCConnectFrontendRedirectURL  = "oidc_connect_frontend_redirect_url"
	SettingKeyOIDCConnectTokenAuthMethod      = "oidc_connect_token_auth_method"
	SettingKeyOIDCConnectUsePKCE              = "oidc_connect_use_pkce"
	SettingKeyOIDCConnectValidateIDToken      = "oidc_connect_validate_id_token"
	SettingKeyOIDCConnectAllowedSigningAlgs   = "oidc_connect_allowed_signing_algs"
	SettingKeyOIDCConnectClockSkewSeconds     = "oidc_connect_clock_skew_seconds"
	SettingKeyOIDCConnectRequireEmailVerified = "oidc_connect_require_email_verified"
	SettingKeyOIDCConnectUserInfoEmailPath    = "oidc_connect_userinfo_email_path"
	SettingKeyOIDCConnectUserInfoIDPath       = "oidc_connect_userinfo_id_path"
	SettingKeyOIDCConnectUserInfoUsernamePath = "oidc_connect_userinfo_username_path"

	// OEM设置
	SettingKeySiteName                    = "site_name"                     // 网站名称
	SettingKeySiteLogo                    = "site_logo"                     // 网站Logo (base64)
	SettingKeySiteSubtitle                = "site_subtitle"                 // 网站副标题
	SettingKeyAPIBaseURL                  = "api_base_url"                  // API端点地址（用于客户端配置和导入）
	SettingKeyContactInfo                 = "contact_info"                  // 客服联系方式
	SettingKeyDocURL                      = "doc_url"                       // 文档链接
	SettingKeyHomeContent                 = "home_content"                  // 首页内容（支持 Markdown/HTML，或 URL 作为 iframe src）
	SettingKeyHideCcsImportButton         = "hide_ccs_import_button"        // 是否隐藏 API Keys 页面的导入 CCS 按钮
	SettingKeyPurchaseSubscriptionEnabled = "purchase_subscription_enabled" // 是否展示"购买订阅"页面入口
	SettingKeyPurchaseSubscriptionURL     = "purchase_subscription_url"     // "购买订阅"页面 URL（作为 iframe src）
	SettingKeyTableDefaultPageSize        = "table_default_page_size"       // 表格默认每页条数
	SettingKeyTablePageSizeOptions        = "table_page_size_options"       // 表格可选每页条数（JSON 数组）
	SettingKeyCustomMenuItems             = "custom_menu_items"             // 自定义菜单项（JSON 数组）
	SettingKeyCustomEndpoints             = "custom_endpoints"              // 自定义端点列表（JSON 数组）

	// 默认配置
	SettingKeyDefaultConcurrency   = "default_concurrency"    // 新用户默认并发量
	SettingKeyDefaultBalance       = "default_balance"        // 新用户默认余额
	SettingKeyDefaultSubscriptions = "default_subscriptions"  // 新用户默认订阅列表（JSON）
	SettingKeyDefaultUserRPMLimit  = "default_user_rpm_limit" // 新用户默认 RPM 限制（0 = 不限制）

	// 第三方认证来源默认授予配置
	SettingKeyAuthSourceDefaultEmailBalance            = "auth_source_default_email_balance"
	SettingKeyAuthSourceDefaultEmailConcurrency        = "auth_source_default_email_concurrency"
	SettingKeyAuthSourceDefaultEmailSubscriptions      = "auth_source_default_email_subscriptions"
	SettingKeyAuthSourceDefaultEmailGrantOnSignup      = "auth_source_default_email_grant_on_signup"
	SettingKeyAuthSourceDefaultEmailGrantOnFirstBind   = "auth_source_default_email_grant_on_first_bind"
	SettingKeyAuthSourceDefaultLinuxDoBalance          = "auth_source_default_linuxdo_balance"
	SettingKeyAuthSourceDefaultLinuxDoConcurrency      = "auth_source_default_linuxdo_concurrency"
	SettingKeyAuthSourceDefaultLinuxDoSubscriptions    = "auth_source_default_linuxdo_subscriptions"
	SettingKeyAuthSourceDefaultLinuxDoGrantOnSignup    = "auth_source_default_linuxdo_grant_on_signup"
	SettingKeyAuthSourceDefaultLinuxDoGrantOnFirstBind = "auth_source_default_linuxdo_grant_on_first_bind"
	SettingKeyAuthSourceDefaultOIDCBalance             = "auth_source_default_oidc_balance"
	SettingKeyAuthSourceDefaultOIDCConcurrency         = "auth_source_default_oidc_concurrency"
	SettingKeyAuthSourceDefaultOIDCSubscriptions       = "auth_source_default_oidc_subscriptions"
	SettingKeyAuthSourceDefaultOIDCGrantOnSignup       = "auth_source_default_oidc_grant_on_signup"
	SettingKeyAuthSourceDefaultOIDCGrantOnFirstBind    = "auth_source_default_oidc_grant_on_first_bind"
	SettingKeyAuthSourceDefaultWeChatBalance           = "auth_source_default_wechat_balance"
	SettingKeyAuthSourceDefaultWeChatConcurrency       = "auth_source_default_wechat_concurrency"
	SettingKeyAuthSourceDefaultWeChatSubscriptions     = "auth_source_default_wechat_subscriptions"
	SettingKeyAuthSourceDefaultWeChatGrantOnSignup     = "auth_source_default_wechat_grant_on_signup"
	SettingKeyAuthSourceDefaultWeChatGrantOnFirstBind  = "auth_source_default_wechat_grant_on_first_bind"
	SettingKeyForceEmailOnThirdPartySignup             = "force_email_on_third_party_signup"

	// 管理员 API Key
	SettingKeyAdminAPIKey = "admin_api_key" // 全局管理员 API Key（用于外部系统集成）

	// Gemini 配额策略（JSON）
	SettingKeyGeminiQuotaPolicy = "gemini_quota_policy"

	// Model fallback settings
	SettingKeyEnableModelFallback      = "enable_model_fallback"
	SettingKeyFallbackModelAnthropic   = "fallback_model_anthropic"
	SettingKeyFallbackModelOpenAI      = "fallback_model_openai"
	SettingKeyFallbackModelGemini      = "fallback_model_gemini"
	SettingKeyFallbackModelAntigravity = "fallback_model_antigravity"

	// Request identity patch (Claude -> Gemini systemInstruction injection)
	SettingKeyEnableIdentityPatch = "enable_identity_patch"
	SettingKeyIdentityPatchPrompt = "identity_patch_prompt"

	// =========================
	// Ops Monitoring (vNext)
	// =========================

	// SettingKeyOpsMonitoringEnabled is a DB-backed soft switch to enable/disable ops module at runtime.
	SettingKeyOpsMonitoringEnabled = "ops_monitoring_enabled"

	// SettingKeyOpsRealtimeMonitoringEnabled controls realtime features (e.g. WS/QPS push).
	SettingKeyOpsRealtimeMonitoringEnabled = "ops_realtime_monitoring_enabled"

	// SettingKeyOpsQueryModeDefault controls the default query mode for ops dashboard (auto/raw/preagg).
	SettingKeyOpsQueryModeDefault = "ops_query_mode_default"

	// SettingKeyOpsEmailNotificationConfig stores JSON config for ops email notifications.
	SettingKeyOpsEmailNotificationConfig = "ops_email_notification_config"

	// SettingKeyOpsAlertRuntimeSettings stores JSON config for ops alert evaluator runtime settings.
	SettingKeyOpsAlertRuntimeSettings = "ops_alert_runtime_settings"

	// SettingKeyOpsMetricsIntervalSeconds controls the ops metrics collector interval (>=60).
	SettingKeyOpsMetricsIntervalSeconds = "ops_metrics_interval_seconds"

	// SettingKeyOpsAdvancedSettings stores JSON config for ops advanced settings (data retention, aggregation).
	SettingKeyOpsAdvancedSettings = "ops_advanced_settings"

	// SettingKeyOpsRuntimeLogConfig stores JSON config for runtime log settings.
	SettingKeyOpsRuntimeLogConfig = "ops_runtime_log_config"

	// =========================
	// Channel Monitor (渠道监控)
	// =========================

	// SettingKeyChannelMonitorEnabled is a DB-backed soft switch for the channel monitor feature.
	// When false: runner skips scheduling and user-facing endpoints return an empty list.
	SettingKeyChannelMonitorEnabled = "channel_monitor_enabled"

	// SettingKeyChannelMonitorDefaultIntervalSeconds controls the default interval (seconds)
	// pre-filled when creating a new channel monitor from the admin UI. Range: [15, 3600].
	SettingKeyChannelMonitorDefaultIntervalSeconds = "channel_monitor_default_interval_seconds"

	// SettingKeyAvailableChannelsEnabled is a DB-backed soft switch for the "Available Channels"
	// user-facing aggregate view. When false: user endpoint returns an empty list and the
	// sidebar entry is hidden. Defaults to false (opt-in feature).
	SettingKeyAvailableChannelsEnabled = "available_channels_enabled"

	// =========================
	// Overload Cooldown (529)
	// =========================

	// SettingKeyOverloadCooldownSettings stores JSON config for 529 overload cooldown handling.
	SettingKeyOverloadCooldownSettings = "overload_cooldown_settings"

	// =========================
	// Stream Timeout Handling
	// =========================

	// SettingKeyStreamTimeoutSettings stores JSON config for stream timeout handling.
	SettingKeyStreamTimeoutSettings = "stream_timeout_settings"

	// =========================
	// Request Rectifier (请求整流器)
	// =========================

	// SettingKeyRectifierSettings stores JSON config for rectifier settings (thinking signature + budget).
	SettingKeyRectifierSettings = "rectifier_settings"

	// =========================
	// Beta Policy Settings
	// =========================

	// SettingKeyBetaPolicySettings stores JSON config for beta policy rules.
	SettingKeyBetaPolicySettings = "beta_policy_settings"

	// SettingKeyOpenAIFastPolicySettings stores JSON config for OpenAI
	// service_tier (fast/flex) policy rules. Mirrors BetaPolicySettings but
	// targets OpenAI's body-level service_tier field instead of Claude's
	// anthropic-beta header.
	SettingKeyOpenAIFastPolicySettings = "openai_fast_policy_settings"

	// =========================
	// Claude Code Version Check
	// =========================

	// SettingKeyMinClaudeCodeVersion 最低 Claude Code 版本号要求 (semver, 如 "2.1.0"，空值=不检查)
	SettingKeyMinClaudeCodeVersion = "min_claude_code_version"

	// SettingKeyMaxClaudeCodeVersion 最高 Claude Code 版本号限制 (semver, 如 "3.0.0"，空值=不检查)
	SettingKeyMaxClaudeCodeVersion = "max_claude_code_version"

	// SettingKeyAllowUngroupedKeyScheduling 允许未分组 API Key 调度（默认 false：未分组 Key 返回 403）
	SettingKeyAllowUngroupedKeyScheduling = "allow_ungrouped_key_scheduling"

	// SettingKeyBackendModeEnabled Backend 模式：禁用用户注册和自助服务，仅管理员可登录
	SettingKeyBackendModeEnabled = "backend_mode_enabled"

	// Gateway Forwarding Behavior
	// SettingKeyEnableFingerprintUnification 是否统一 OAuth 账号的 X-Stainless-* 指纹头（默认 true）
	SettingKeyEnableFingerprintUnification = "enable_fingerprint_unification"
	// SettingKeyEnableMetadataPassthrough 是否透传客户端原始 metadata.user_id（默认 false）
	SettingKeyEnableMetadataPassthrough = "enable_metadata_passthrough"
	// SettingKeyEnableCCHSigning 是否对 billing header 中的 cch 进行 xxHash64 签名（默认 false）
	SettingKeyEnableCCHSigning = "enable_cch_signing"
	// SettingKeyEnableAnthropicCacheTTL1hInjection 是否对 Anthropic OAuth/SetupToken 请求体注入 1h cache_control ttl（默认 false）
	SettingKeyEnableAnthropicCacheTTL1hInjection = "enable_anthropic_cache_ttl_1h_injection"

	// Balance Low Notification
	SettingKeyBalanceLowNotifyEnabled     = "balance_low_notify_enabled"      // 全局开关
	SettingKeyBalanceLowNotifyThreshold   = "balance_low_notify_threshold"    // 默认阈值（USD）
	SettingKeyBalanceLowNotifyRechargeURL = "balance_low_notify_recharge_url" // 充值页面 URL

	// Account Quota Notification
	SettingKeyAccountQuotaNotifyEnabled = "account_quota_notify_enabled" // 全局开关
	SettingKeyAccountQuotaNotifyEmails  = "account_quota_notify_emails"  // 管理员通知邮箱列表（JSON 数组）

	// Web Search Emulation
	SettingKeyWebSearchEmulationConfig = "web_search_emulation_config" // JSON 配置
)

// AdminAPIKeyPrefix is the prefix for admin API keys (distinct from user "sk-" keys).
const AdminAPIKeyPrefix = "admin-"
