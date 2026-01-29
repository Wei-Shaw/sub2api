package service

type SystemSettings struct {
	RegistrationEnabled bool
	EmailVerifyEnabled  bool
	PromoCodeEnabled    bool

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

	SiteName            string
	SiteLogo            string
	SiteSubtitle        string
	APIBaseURL          string
	ContactInfo         string
	DocURL              string
	HomeContent         string
	HideCcsImportButton bool

	DefaultConcurrency int
	DefaultBalance     float64

	// Model fallback configuration
	EnableModelFallback      bool   `json:"enable_model_fallback"`
	FallbackModelAnthropic   string `json:"fallback_model_anthropic"`
	FallbackModelOpenAI      string `json:"fallback_model_openai"`
	FallbackModelGemini      string `json:"fallback_model_gemini"`
	FallbackModelAntigravity string `json:"fallback_model_antigravity"`

	// Identity patch configuration (Claude -> Gemini)
	EnableIdentityPatch bool   `json:"enable_identity_patch"`
	IdentityPatchPrompt string `json:"identity_patch_prompt"`

	// Sora configuration
	SoraBaseURL                        string
	SoraTimeout                        int
	SoraMaxRetries                     int
	SoraPollInterval                   float64
	SoraCallLogicMode                  string
	SoraCacheEnabled                   bool
	SoraCacheBaseDir                   string
	SoraCacheVideoDir                  string
	SoraCacheMaxBytes                  int64
	SoraCacheAllowedHosts              []string
	SoraCacheUserDirEnabled            bool
	SoraWatermarkFreeEnabled           bool
	SoraWatermarkFreeParseMethod       string
	SoraWatermarkFreeCustomParseURL    string
	SoraWatermarkFreeCustomParseToken  string
	SoraWatermarkFreeFallbackOnFailure bool
	SoraTokenRefreshEnabled            bool

	// Ops monitoring (vNext)
	OpsMonitoringEnabled         bool
	OpsRealtimeMonitoringEnabled bool
	OpsQueryModeDefault          string
	OpsMetricsIntervalSeconds    int
REDACTED

type PublicSettings struct {
	RegistrationEnabled bool
	EmailVerifyEnabled  bool
	PromoCodeEnabled    bool
	TurnstileEnabled    bool
	TurnstileSiteKey    string
	SiteName            string
	SiteLogo            string
	SiteSubtitle        string
	APIBaseURL          string
	ContactInfo         string
	DocURL              string
	HomeContent         string
	HideCcsImportButton bool
	LinuxDoOAuthEnabled bool
	Version             string
REDACTED

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
REDACTED

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
REDACTED
REDACTED
