package service

import "strings"

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

type SystemSettings struct {
	RegistrationEnabled              bool
	EmailVerifyEnabled               bool
	RegistrationEmailSuffixWhitelist []string
	PromoCodeEnabled                 bool
	PasswordResetEnabled             bool
	FrontendURL                      string
	InvitationCodeEnabled            bool
	TotpEnabled                      bool // TOTP 双因素认证
	LoginAgreementEnabled            bool
	LoginAgreementMode               string
	LoginAgreementUpdatedAt          string
	LoginAgreementDocuments          []LoginAgreementDocument

	SMTPHost               string
	SMTPPort               int
	SMTPUsername           string
	SMTPPassword           string
	SMTPPasswordConfigured bool
	SMTPFrom               string
	SMTPFromName           string
	SMTPUseTLS             bool

	TurnstileEnabled             bool
	TurnstileSiteKey             string
	TurnstileSecretKey           string
	TurnstileSecretKeyConfigured bool

	// LinuxDo Connect OAuth 登录
	LinuxDoConnectEnabled                bool
	LinuxDoConnectClientID               string
	LinuxDoConnectClientSecret           string
	LinuxDoConnectClientSecretConfigured bool
	LinuxDoConnectRedirectURL            string

	// WeChat Connect OAuth 登录
	WeChatConnectEnabled                   bool
	WeChatConnectAppID                     string
	WeChatConnectAppSecret                 string
	WeChatConnectAppSecretConfigured       bool
	WeChatConnectOpenAppID                 string
	WeChatConnectOpenAppSecret             string
	WeChatConnectOpenAppSecretConfigured   bool
	WeChatConnectMPAppID                   string
	WeChatConnectMPAppSecret               string
	WeChatConnectMPAppSecretConfigured     bool
	WeChatConnectMobileAppID               string
	WeChatConnectMobileAppSecret           string
	WeChatConnectMobileAppSecretConfigured bool
	WeChatConnectOpenEnabled               bool
	WeChatConnectMPEnabled                 bool
	WeChatConnectMobileEnabled             bool
	WeChatConnectMode                      string
	WeChatConnectScopes                    string
	WeChatConnectRedirectURL               string
	WeChatConnectFrontendRedirectURL       string

	// Generic OIDC OAuth 登录
	OIDCConnectEnabled                bool
	OIDCConnectProviderName           string
	OIDCConnectClientID               string
	OIDCConnectClientSecret           string
	OIDCConnectClientSecretConfigured bool
	OIDCConnectIssuerURL              string
	OIDCConnectDiscoveryURL           string
	OIDCConnectAuthorizeURL           string
	OIDCConnectTokenURL               string
	OIDCConnectUserInfoURL            string
	OIDCConnectJWKSURL                string
	OIDCConnectScopes                 string
	OIDCConnectRedirectURL            string
	OIDCConnectFrontendRedirectURL    string
	OIDCConnectTokenAuthMethod        string
	OIDCConnectUsePKCE                bool
	OIDCConnectValidateIDToken        bool
	OIDCConnectAllowedSigningAlgs     string
	OIDCConnectClockSkewSeconds       int
	OIDCConnectRequireEmailVerified   bool
	OIDCConnectUserInfoEmailPath      string
	OIDCConnectUserInfoIDPath         string
	OIDCConnectUserInfoUsernamePath   string

	// GitHub / Google 邮箱快捷登录
	GitHubOAuthEnabled                bool
	GitHubOAuthClientID               string
	GitHubOAuthClientSecret           string
	GitHubOAuthClientSecretConfigured bool
	GitHubOAuthRedirectURL            string
	GitHubOAuthFrontendRedirectURL    string
	GoogleOAuthEnabled                bool
	GoogleOAuthClientID               string
	GoogleOAuthClientSecret           string
	GoogleOAuthClientSecretConfigured bool
	GoogleOAuthRedirectURL            string
	GoogleOAuthFrontendRedirectURL    string

	SiteName                    string
	SiteLogo                    string
	SiteSubtitle                string
	APIBaseURL                  string
	ContactInfo                 string
	DocURL                      string
	HomeContent                 string
	HideCcsImportButton         bool
	PurchaseSubscriptionEnabled bool
	PurchaseSubscriptionURL     string
	TableDefaultPageSize        int
	TablePageSizeOptions        []int
	CustomMenuItems             string // JSON array of custom menu items
	CustomEndpoints             string // JSON array of custom endpoints

	DefaultConcurrency           int
	DefaultBalance               float64
	RiskControlEnabled           bool
	AffiliateEnabled             bool
	AffiliateRebateRate          float64
	AffiliateRebateFreezeHours   int
	AffiliateRebateDurationDays  int
	AffiliateRebatePerInviteeCap float64
	DefaultUserRPMLimit          int
	DefaultSubscriptions         []DefaultSubscriptionSetting

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
	OpsMonitoringEnabled         bool
	OpsRealtimeMonitoringEnabled bool
	OpsQueryModeDefault          string
	OpsMetricsIntervalSeconds    int

	// Channel Monitor feature
	ChannelMonitorEnabled                bool `json:"channel_monitor_enabled"`
	ChannelMonitorDefaultIntervalSeconds int  `json:"channel_monitor_default_interval_seconds"`

	// Available Channels feature (user-facing aggregate view)
	AvailableChannelsEnabled bool `json:"available_channels_enabled"`

	// Claude Code version check
	MinClaudeCodeVersion string
	MaxClaudeCodeVersion string

	// 分组隔离：允许未分组 Key 调度（默认 false → 403）
	AllowUngroupedKeyScheduling bool

	// Backend 模式：禁用用户注册和自助服务，仅管理员可登录
	BackendModeEnabled bool

	// Gateway forwarding behavior
	EnableFingerprintUnification       bool // 是否统一 OAuth 账号的指纹头（默认 true）
	EnableMetadataPassthrough          bool // 是否透传客户端原始 metadata（默认 false）
	EnableCCHSigning                   bool // 是否对 billing header cch 进行签名（默认 false）
	EnableAnthropicCacheTTL1hInjection bool // 是否对 Anthropic OAuth/SetupToken 请求体注入 1h cache_control ttl（默认 false）

	// Web Search Emulation
	WebSearchEmulationEnabled bool // 是否启用 web search 模拟

	// Payment visible method routing
	PaymentVisibleMethodAlipaySource  string
	PaymentVisibleMethodWxpaySource   string
	PaymentVisibleMethodAlipayEnabled bool
	PaymentVisibleMethodWxpayEnabled  bool

	// OpenAI account scheduling
	OpenAIAdvancedSchedulerEnabled bool

	// Balance low notification
	BalanceLowNotifyEnabled     bool
	BalanceLowNotifyThreshold   float64
	BalanceLowNotifyRechargeURL string

	// Account quota notification
	AccountQuotaNotifyEnabled bool
	AccountQuotaNotifyEmails  []NotifyEmailEntry
}

type DefaultSubscriptionSetting struct {
	GroupID      int64 `json:"group_id"`
	ValidityDays int   `json:"validity_days"`
}

type PublicSettings struct {
	RegistrationEnabled              bool
	EmailVerifyEnabled               bool
	ForceEmailOnThirdPartySignup     bool
	RegistrationEmailSuffixWhitelist []string
	PromoCodeEnabled                 bool
	PasswordResetEnabled             bool
	InvitationCodeEnabled            bool
	TotpEnabled                      bool // TOTP 双因素认证
	LoginAgreementEnabled            bool
	LoginAgreementMode               string
	LoginAgreementUpdatedAt          string
	LoginAgreementRevision           string
	LoginAgreementDocuments          []LoginAgreementDocument
	TurnstileEnabled                 bool
	TurnstileSiteKey                 string
	SiteName                         string
	SiteLogo                         string
	SiteSubtitle                     string
	APIBaseURL                       string
	ContactInfo                      string
	DocURL                           string
	HomeContent                      string
	HideCcsImportButton              bool

	PurchaseSubscriptionEnabled bool
	PurchaseSubscriptionURL     string
	TableDefaultPageSize        int
	TablePageSizeOptions        []int
	CustomMenuItems             string // JSON array of custom menu items
	CustomEndpoints             string // JSON array of custom endpoints

	LinuxDoOAuthEnabled      bool
	WeChatOAuthEnabled       bool
	WeChatOAuthOpenEnabled   bool
	WeChatOAuthMPEnabled     bool
	WeChatOAuthMobileEnabled bool
	BackendModeEnabled       bool
	PaymentEnabled           bool
	OIDCOAuthEnabled         bool
	OIDCOAuthProviderName    string
	GitHubOAuthEnabled       bool
	GoogleOAuthEnabled       bool
	Version                  string

	BalanceLowNotifyEnabled     bool
	AccountQuotaNotifyEnabled   bool
	BalanceLowNotifyThreshold   float64
	BalanceLowNotifyRechargeURL string

	// Channel Monitor feature
	ChannelMonitorEnabled                bool `json:"channel_monitor_enabled"`
	ChannelMonitorDefaultIntervalSeconds int  `json:"channel_monitor_default_interval_seconds"`

	// Available Channels feature (user-facing aggregate view)
	AvailableChannelsEnabled bool `json:"available_channels_enabled"`

	// Affiliate (邀请返利) feature toggle
	AffiliateEnabled bool `json:"affiliate_enabled"`

	// 风控中心功能开关
	RiskControlEnabled bool `json:"risk_control_enabled"`
}

type LoginAgreementDocument struct {
	ID        string `json:"id"`
	Title     string `json:"title"`
	ContentMD string `json:"content_md"`
}

type WeChatConnectOAuthConfig struct {
	Enabled             bool
	LegacyAppID         string
	LegacyAppSecret     string
	OpenAppID           string
	OpenAppSecret       string
	MPAppID             string
	MPAppSecret         string
	MobileAppID         string
	MobileAppSecret     string
	OpenEnabled         bool
	MPEnabled           bool
	MobileEnabled       bool
	Mode                string
	Scopes              string
	RedirectURL         string
	FrontendRedirectURL string
}

func (cfg WeChatConnectOAuthConfig) SupportsMode(mode string) bool {
	switch normalizeWeChatConnectModeSetting(mode) {
	case "mp":
		return cfg.MPEnabled
	case "mobile":
		return cfg.MobileEnabled
	default:
		return cfg.OpenEnabled
	}
}

func (cfg WeChatConnectOAuthConfig) ScopeForMode(mode string) string {
	switch normalizeWeChatConnectModeSetting(mode) {
	case "mp":
		return normalizeWeChatConnectScopeSetting(cfg.Scopes, "mp")
	case "mobile":
		return ""
	}
	return defaultWeChatConnectScopeForMode("open")
}

func (cfg WeChatConnectOAuthConfig) AppIDForMode(mode string) string {
	switch normalizeWeChatConnectModeSetting(mode) {
	case "mp":
		return strings.TrimSpace(firstNonEmpty(cfg.MPAppID, cfg.LegacyAppID))
	case "mobile":
		return strings.TrimSpace(firstNonEmpty(cfg.MobileAppID, cfg.LegacyAppID))
	}
	return strings.TrimSpace(firstNonEmpty(cfg.OpenAppID, cfg.LegacyAppID))
}

func (cfg WeChatConnectOAuthConfig) AppSecretForMode(mode string) string {
	switch normalizeWeChatConnectModeSetting(mode) {
	case "mp":
		return strings.TrimSpace(firstNonEmpty(cfg.MPAppSecret, cfg.LegacyAppSecret))
	case "mobile":
		return strings.TrimSpace(firstNonEmpty(cfg.MobileAppSecret, cfg.LegacyAppSecret))
	}
	return strings.TrimSpace(firstNonEmpty(cfg.OpenAppSecret, cfg.LegacyAppSecret))
}

// StreamTimeoutSettings 流超时处理配置（仅控制超时后的处理方式，超时判定由网关配置控制）
type StreamTimeoutSettings struct {
	// Enabled 是否启用流超时处理
	Enabled bool `json:"enabled"`
	// Action 超时后的处理方式: "temp_unsched" | "error" | "none"
	Action string `json:"action"`
	// TempUnschedMinutes 临时不可调度持续时间（分钟）
	TempUnschedMinutes int `json:"temp_unsched_minutes"`
	// ThresholdCount 触发阈值次数（累计多少次超时才触发）
	ThresholdCount int `json:"threshold_count"`
	// ThresholdWindowMinutes 阈值窗口时间（分钟）
	ThresholdWindowMinutes int `json:"threshold_window_minutes"`
}

// StreamTimeoutAction 流超时处理方式常量
const (
	StreamTimeoutActionTempUnsched = "temp_unsched" // 临时不可调度
	StreamTimeoutActionError       = "error"        // 标记为错误状态
	StreamTimeoutActionNone        = "none"         // 不处理
)

// DefaultStreamTimeoutSettings 返回默认的流超时配置
func DefaultStreamTimeoutSettings() *StreamTimeoutSettings {
	return &StreamTimeoutSettings{
		Enabled:                false,
		Action:                 StreamTimeoutActionTempUnsched,
		TempUnschedMinutes:     5,
		ThresholdCount:         3,
		ThresholdWindowMinutes: 10,
	}
}

// RectifierSettings 请求整流器配置
type RectifierSettings struct {
	Enabled                  bool     `json:"enabled"`                    // 总开关
	ThinkingSignatureEnabled bool     `json:"thinking_signature_enabled"` // Thinking 签名整流
	ThinkingBudgetEnabled    bool     `json:"thinking_budget_enabled"`    // Thinking Budget 整流
	APIKeySignatureEnabled   bool     `json:"apikey_signature_enabled"`   // API Key 签名整流开关
	APIKeySignaturePatterns  []string `json:"apikey_signature_patterns"`  // API Key 自定义匹配关键词
}

// DefaultRectifierSettings 返回默认的整流器配置（全部启用）
func DefaultRectifierSettings() *RectifierSettings {
	return &RectifierSettings{
		Enabled:                  true,
		ThinkingSignatureEnabled: true,
		ThinkingBudgetEnabled:    true,
	}
}

// Beta Policy 策略常量
const (
	BetaPolicyActionPass   = "pass"   // 透传，不做任何处理
	BetaPolicyActionFilter = "filter" // 过滤，从 beta header 中移除该 token
	BetaPolicyActionBlock  = "block"  // 拦截，直接返回错误

	BetaPolicyScopeAll     = "all"     // 所有账号类型
	BetaPolicyScopeOAuth   = "oauth"   // 仅 OAuth 账号
	BetaPolicyScopeAPIKey  = "apikey"  // 仅 API Key 账号
	BetaPolicyScopeBedrock = "bedrock" // 仅 AWS Bedrock 账号
)

// BetaPolicyRule 单条 Beta 策略规则
type BetaPolicyRule struct {
	BetaToken            string   `json:"beta_token"`                       // beta token 值
	Action               string   `json:"action"`                           // "pass" | "filter" | "block"
	Scope                string   `json:"scope"`                            // "all" | "oauth" | "apikey" | "bedrock"
	ErrorMessage         string   `json:"error_message,omitempty"`          // 自定义错误消息 (action=block 时生效)
	ModelWhitelist       []string `json:"model_whitelist,omitempty"`        // 模型匹配模式列表（为空=对所有模型生效）
	FallbackAction       string   `json:"fallback_action,omitempty"`        // 未匹配白名单的模型的处理方式
	FallbackErrorMessage string   `json:"fallback_error_message,omitempty"` // 未匹配白名单时的自定义错误消息 (fallback_action=block 时生效)
}

// BetaPolicySettings Beta 策略配置
type BetaPolicySettings struct {
	Rules []BetaPolicyRule `json:"rules"`
}

// OverloadCooldownSettings 529过载冷却配置
type OverloadCooldownSettings struct {
	// Enabled 是否在收到529时暂停账号调度
	Enabled bool `json:"enabled"`
	// CooldownMinutes 冷却时长（分钟）
	CooldownMinutes int `json:"cooldown_minutes"`
}

// RateLimit429CooldownSettings 429默认回避配置
type RateLimit429CooldownSettings struct {
	// Enabled 是否在无法解析上游重置时间时应用默认429回避
	Enabled bool `json:"enabled"`
	// CooldownSeconds 默认回避时长（秒）
	CooldownSeconds int `json:"cooldown_seconds"`
}

// DefaultOverloadCooldownSettings 返回默认的过载冷却配置（启用，10分钟）
func DefaultOverloadCooldownSettings() *OverloadCooldownSettings {
	return &OverloadCooldownSettings{
		Enabled:         true,
		CooldownMinutes: 10,
	}
}

// DefaultRateLimit429CooldownSettings 返回默认的429回避配置（启用，5秒）
func DefaultRateLimit429CooldownSettings() *RateLimit429CooldownSettings {
	return &RateLimit429CooldownSettings{
		Enabled:         true,
		CooldownSeconds: 5,
	}
}

// DefaultBetaPolicySettings 返回默认的 Beta 策略配置
func DefaultBetaPolicySettings() *BetaPolicySettings {
	return &BetaPolicySettings{
		Rules: []BetaPolicyRule{
			{
				BetaToken: "fast-mode-2026-02-01",
				Action:    BetaPolicyActionFilter,
				Scope:     BetaPolicyScopeAll,
			},
			{
				BetaToken: "context-1m-2025-08-07",
				Action:    BetaPolicyActionFilter,
				Scope:     BetaPolicyScopeAll,
			},
		},
	}
}

// OpenAI Fast Policy 策略常量
// OpenAI 的 "fast 模式" 通过请求体中的 service_tier 字段识别：
//   - "priority"（客户端可传 "fast"，归一化为 "priority"）：fast 模式
//   - "flex"：低优先级模式
//   - 省略：normal 默认
//
// 本策略复用 BetaPolicyAction*/BetaPolicyScope* 常量语义，只是匹配键从
// anthropic-beta header 换成 body 的 service_tier 字段。
const (
	OpenAIFastTierAny      = "all"      // 匹配任意已识别的 service_tier
	OpenAIFastTierPriority = "priority" // 仅匹配 fast（priority）
	OpenAIFastTierFlex     = "flex"     // 仅匹配 flex
)

// OpenAIFastPolicyRule 单条 OpenAI fast/flex 策略规则
type OpenAIFastPolicyRule struct {
	ServiceTier          string   `json:"service_tier"`                     // "priority" | "flex" | "auto" | "default" | "scale" | "all"
	Action               string   `json:"action"`                           // "pass" | "filter" | "block"
	Scope                string   `json:"scope"`                            // "all" | "oauth" | "apikey" | "bedrock"
	ErrorMessage         string   `json:"error_message,omitempty"`          // 自定义错误消息 (action=block 时生效)
	ModelWhitelist       []string `json:"model_whitelist,omitempty"`        // 模型匹配模式列表（为空=对所有模型生效）
	FallbackAction       string   `json:"fallback_action,omitempty"`        // 未匹配白名单的模型的处理方式
	FallbackErrorMessage string   `json:"fallback_error_message,omitempty"` // 未匹配白名单时的自定义错误消息 (fallback_action=block 时生效)
}

// OpenAIFastPolicySettings OpenAI fast 策略配置
type OpenAIFastPolicySettings struct {
	Rules []OpenAIFastPolicyRule `json:"rules"`
}

// DefaultOpenAIFastPolicySettings 返回默认的 OpenAI fast 策略配置。
// 默认对所有模型的 priority（fast）请求执行 filter，即剔除 service_tier 字段，
// 让上游按 normal 优先级处理。
//
// 为什么 ModelWhitelist 为空（=对所有模型生效）：
// codex 客户端的 service_tier=fast 是用户级开关，与 model 字段正交。即使
// 用户使用 gpt-4 + fast，priority 配额仍会被消耗。如果默认规则只锁
// gpt-5.5*，"用 gpt-4 + fast 透传 priority 上游" 这条路径就会绕过策略。
// 与 codex 真实语义对齐，默认对所有模型生效；管理员若需要只针对特定
// 模型，可在 admin UI 中显式配置 model_whitelist。
func DefaultOpenAIFastPolicySettings() *OpenAIFastPolicySettings {
	return &OpenAIFastPolicySettings{
		Rules: []OpenAIFastPolicyRule{
			{
				ServiceTier:    OpenAIFastTierPriority,
				Action:         BetaPolicyActionFilter,
				Scope:          BetaPolicyScopeAll,
				ModelWhitelist: []string{},
				FallbackAction: BetaPolicyActionPass,
			},
		},
	}
}
