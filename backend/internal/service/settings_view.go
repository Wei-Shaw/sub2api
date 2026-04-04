package service

type SystemSettings struct {
	RegistrationEnabled              bool
	EmailVerifyEnabled               bool
	RegistrationEmailSuffixWhitelist []string
	PromoCodeEnabled                 bool
	PasswordResetEnabled             bool
	FrontendURL                      string
	InvitationCodeEnabled            bool
	TotpEnabled                      bool // TOTP 双因素认证

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
	CustomMenuItems             string // JSON array of custom menu items
	CustomEndpoints             string // JSON array of custom endpoints

	DefaultConcurrency   int
	DefaultBalance       float64
	DefaultSubscriptions []DefaultSubscriptionSetting

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

	// Claude Code version check
	MinClaudeCodeVersion string
	MaxClaudeCodeVersion string

	// 分组隔离：允许未分组 Key 调度（默认 false → 403）
	AllowUngroupedKeyScheduling bool

	// Backend 模式：禁用用户注册和自助服务，仅管理员可登录
	BackendModeEnabled bool

	// Gateway forwarding behavior
	EnableFingerprintUnification bool // 是否统一 OAuth 账号的指纹头（默认 true）
	EnableMetadataPassthrough    bool // 是否透传客户端原始 metadata（默认 false）
}

type DefaultSubscriptionSetting struct {
	GroupID      int64 `json:"group_id"`
	ValidityDays int   `json:"validity_days"`
}

type PublicSettings struct {
	RegistrationEnabled              bool
	EmailVerifyEnabled               bool
	RegistrationEmailSuffixWhitelist []string
	PromoCodeEnabled                 bool
	PasswordResetEnabled             bool
	InvitationCodeEnabled            bool
	TotpEnabled                      bool // TOTP 双因素认证
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
	CustomMenuItems             string // JSON array of custom menu items
	CustomEndpoints             string // JSON array of custom endpoints

	LinuxDoOAuthEnabled bool
	BackendModeEnabled  bool
	Version             string
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
	BetaToken    string `json:"beta_token"`              // beta token 值
	Action       string `json:"action"`                  // "pass" | "filter" | "block"
	Scope        string `json:"scope"`                   // "all" | "oauth" | "apikey" | "bedrock"
	ErrorMessage string `json:"error_message,omitempty"` // 自定义错误消息 (action=block 时生效)
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

// DefaultOverloadCooldownSettings 返回默认的过载冷却配置（启用，10分钟）
func DefaultOverloadCooldownSettings() *OverloadCooldownSettings {
	return &OverloadCooldownSettings{
		Enabled:         true,
		CooldownMinutes: 10,
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
