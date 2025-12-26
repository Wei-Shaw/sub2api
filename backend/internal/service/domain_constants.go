package service

// Status constants
const (
	StatusActive   = "active"
	StatusDisabled = "disabled"
	StatusError    = "error"
	StatusUnused   = "unused"
	StatusUsed     = "used"
	StatusExpired  = "expired"
)

// Role constants
const (
	RoleAdmin = "admin"
	RoleUser  = "user"
)

// Platform constants
const (
	PlatformAnthropic = "anthropic"
	PlatformOpenAI    = "openai"
	PlatformGemini    = "gemini"
)

// Account type constants
const (
	AccountTypeOAuth      = "oauth"       // OAuth类型账号（full scope: profile + inference）
	AccountTypeSetupToken = "setup-token" // Setup Token类型账号（inference only scope）
	AccountTypeApiKey     = "apikey"      // API Key类型账号
)

// Redeem type constants
const (
	RedeemTypeBalance      = "balance"
	RedeemTypeConcurrency  = "concurrency"
	RedeemTypeSubscription = "subscription"
)

// Admin adjustment type constants
const (
	AdjustmentTypeAdminBalance     = "admin_balance"     // 管理员调整余额
	AdjustmentTypeAdminConcurrency = "admin_concurrency" // 管理员调整并发数
)

// Group subscription type constants
const (
	SubscriptionTypeStandard     = "standard"     // 标准计费模式（按余额扣费）
	SubscriptionTypeSubscription = "subscription" // 订阅模式（按限额控制）
)

// Subscription status constants
const (
	SubscriptionStatusActive    = "active"
	SubscriptionStatusExpired   = "expired"
	SubscriptionStatusSuspended = "suspended"
)

// Setting keys
const (
	// 注册设置
	SettingKeyRegistrationEnabled = "registration_enabled" // 是否开放注册
	SettingKeyEmailVerifyEnabled  = "email_verify_enabled" // 是否开启邮件验证

	// 邮件服务设置
	SettingKeySmtpHost     = "smtp_host"      // SMTP服务器地址
	SettingKeySmtpPort     = "smtp_port"      // SMTP端口
	SettingKeySmtpUsername = "smtp_username"  // SMTP用户名
	SettingKeySmtpPassword = "smtp_password"  // SMTP密码（加密存储）
	SettingKeySmtpFrom     = "smtp_from"      // 发件人地址
	SettingKeySmtpFromName = "smtp_from_name" // 发件人名称
	SettingKeySmtpUseTLS   = "smtp_use_tls"   // 是否使用TLS

	// Cloudflare Turnstile 设置
	SettingKeyTurnstileEnabled   = "turnstile_enabled"    // 是否启用 Turnstile 验证
	SettingKeyTurnstileSiteKey   = "turnstile_site_key"   // Turnstile Site Key
	SettingKeyTurnstileSecretKey = "turnstile_secret_key" // Turnstile Secret Key

	// OEM设置
	SettingKeySiteName     = "site_name"     // 网站名称
	SettingKeySiteLogo     = "site_logo"     // 网站Logo (base64)
	SettingKeySiteSubtitle = "site_subtitle" // 网站副标题
	SettingKeyApiBaseUrl   = "api_base_url"  // API端点地址（用于客户端配置和导入）
	SettingKeyContactInfo  = "contact_info"  // 客服联系方式
	SettingKeyDocUrl       = "doc_url"       // 文档链接

	// 默认配置
	SettingKeyDefaultConcurrency = "default_concurrency" // 新用户默认并发量
	SettingKeyDefaultBalance     = "default_balance"     // 新用户默认余额

	// 管理员 API Key
	SettingKeyAdminApiKey = "admin_api_key" // 全局管理员 API Key（用于外部系统集成）
)

// Admin API Key prefix (distinct from user "sk-" keys)
const AdminApiKeyPrefix = "admin-"
