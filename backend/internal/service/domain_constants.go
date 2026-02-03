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

// Platform constants
const (
	PlatformAnthropic   = domain.PlatformAnthropic
	PlatformOpenAI      = domain.PlatformOpenAI
	PlatformGemini      = domain.PlatformGemini
	PlatformAntigravity = domain.PlatformAntigravity
)

// Account type constants
const (
	AccountTypeOAuth      = domain.AccountTypeOAuth      // OAuth类型账号（full scope: profile + inference）
	AccountTypeSetupToken = domain.AccountTypeSetupToken // Setup Token类型账号（inference only scope）
	AccountTypeAPIKey     = domain.AccountTypeAPIKey     // API Key类型账号
)

// Redeem type constants
const (
	RedeemTypeBalance      = domain.RedeemTypeBalance
	RedeemTypeConcurrency  = domain.RedeemTypeConcurrency
	RedeemTypeSubscription = domain.RedeemTypeSubscription
	RedeemTypeInvitation   = domain.RedeemTypeInvitation
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

// Setting keys
const (
	// 注册设置
	SettingKeyRegistrationEnabled   = "registration_enabled"    // 是否开放注册
	SettingKeyEmailVerifyEnabled    = "email_verify_enabled"    // 是否开启邮件验证
	SettingKeyPromoCodeEnabled      = "promo_code_enabled"      // 是否启用优惠码功能
	SettingKeyPasswordResetEnabled  = "password_reset_enabled"  // 是否启用忘记密码功能（需要先开启邮件验证）
	SettingKeyInvitationCodeEnabled = "invitation_code_enabled" // 是否启用邀请码注册

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

	// OEM设置
	SettingKeySiteName                    = "site_name"                     // 网站名称
	SettingKeySiteLogo                    = "site_logo"                     // 网站Logo (base64)
	SettingKeySiteSubtitle                = "site_subtitle"                 // 网站副标题
	SettingKeyAPIBaseURL                  = "api_base_url"                  // API端点地址（用于客户端配置和导入）
	SettingKeyContactInfo                 = "contact_info"                  // 客服联系方式
	SettingKeyDocURL                      = "doc_url"                       // 文档链接
	SettingKeyHomeContent                 = "home_content"                  // 首页内容（支持 Markdown/HTML，或 URL 作为 iframe src）
	SettingKeyHideCcsImportButton         = "hide_ccs_import_button"        // 是否隐藏 API Keys 页面的导入 CCS 按钮
	SettingKeyPurchaseSubscriptionEnabled = "purchase_subscription_enabled" // 是否展示“购买订阅”页面入口
	SettingKeyPurchaseSubscriptionURL     = "purchase_subscription_url"     // “购买订阅”页面 URL（作为 iframe src）

	// 默认配置
	SettingKeyDefaultConcurrency = "default_concurrency" // 新用户默认并发量
	SettingKeyDefaultBalance     = "default_balance"     // 新用户默认余额

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

	// =========================
	// Stream Timeout Handling
	// =========================

	// SettingKeyStreamTimeoutSettings stores JSON config for stream timeout handling.
	SettingKeyStreamTimeoutSettings = "stream_timeout_settings"
)

// AdminAPIKeyPrefix is the prefix for admin API keys (distinct from user "sk-" keys).
const AdminAPIKeyPrefix = "admin-"
