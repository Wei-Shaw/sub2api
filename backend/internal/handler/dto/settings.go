package dto

// SystemSettings represents the admin settings API response payload.
type SystemSettings struct {
	RegistrationEnabled bool `json:"registration_enabled"`
	EmailVerifyEnabled  bool `json:"email_verify_enabled"`
	PromoCodeEnabled    bool `json:"promo_code_enabled"`

	SMTPHost               string `json:"smtp_host"`
	SMTPPort               int    `json:"smtp_port"`
	SMTPUsername           string `json:"smtp_username"`
	SMTPPasswordConfigured bool   `json:"smtp_password_configured"`
	SMTPFrom               string `json:"smtp_from_email"`
	SMTPFromName           string `json:"smtp_from_name"`
	SMTPUseTLS             bool   `json:"smtp_use_tls"`

	TurnstileEnabled             bool   `json:"turnstile_enabled"`
	TurnstileSiteKey             string `json:"turnstile_site_key"`
	TurnstileSecretKeyConfigured bool   `json:"turnstile_secret_key_configured"`

	LinuxDoConnectEnabled                bool   `json:"linuxdo_connect_enabled"`
	LinuxDoConnectClientID               string `json:"linuxdo_connect_client_id"`
	LinuxDoConnectClientSecretConfigured bool   `json:"linuxdo_connect_client_secret_configured"`
	LinuxDoConnectRedirectURL            string `json:"linuxdo_connect_redirect_url"`

	SiteName            string `json:"site_name"`
	SiteLogo            string `json:"site_logo"`
	SiteSubtitle        string `json:"site_subtitle"`
	APIBaseURL          string `json:"api_base_url"`
	ContactInfo         string `json:"contact_info"`
	DocURL              string `json:"doc_url"`
	HomeContent         string `json:"home_content"`
	HideCcsImportButton bool   `json:"hide_ccs_import_button"`

	DefaultConcurrency int     `json:"default_concurrency"`
	DefaultBalance     float64 `json:"default_balance"`

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
	SoraBaseURL                        string   `json:"sora_base_url"`
	SoraTimeout                        int      `json:"sora_timeout"`
	SoraMaxRetries                     int      `json:"sora_max_retries"`
	SoraPollInterval                   float64  `json:"sora_poll_interval"`
	SoraCallLogicMode                  string   `json:"sora_call_logic_mode"`
	SoraCacheEnabled                   bool     `json:"sora_cache_enabled"`
	SoraCacheBaseDir                   string   `json:"sora_cache_base_dir"`
	SoraCacheVideoDir                  string   `json:"sora_cache_video_dir"`
	SoraCacheMaxBytes                  int64    `json:"sora_cache_max_bytes"`
	SoraCacheAllowedHosts              []string `json:"sora_cache_allowed_hosts"`
	SoraCacheUserDirEnabled            bool     `json:"sora_cache_user_dir_enabled"`
	SoraWatermarkFreeEnabled           bool     `json:"sora_watermark_free_enabled"`
	SoraWatermarkFreeParseMethod       string   `json:"sora_watermark_free_parse_method"`
	SoraWatermarkFreeCustomParseURL    string   `json:"sora_watermark_free_custom_parse_url"`
	SoraWatermarkFreeCustomParseToken  string   `json:"sora_watermark_free_custom_parse_token"`
	SoraWatermarkFreeFallbackOnFailure bool     `json:"sora_watermark_free_fallback_on_failure"`
	SoraTokenRefreshEnabled            bool     `json:"sora_token_refresh_enabled"`

	// Ops monitoring (vNext)
	OpsMonitoringEnabled         bool   `json:"ops_monitoring_enabled"`
	OpsRealtimeMonitoringEnabled bool   `json:"ops_realtime_monitoring_enabled"`
	OpsQueryModeDefault          string `json:"ops_query_mode_default"`
	OpsMetricsIntervalSeconds    int    `json:"ops_metrics_interval_seconds"`
REDACTED

type PublicSettings struct {
	RegistrationEnabled bool   `json:"registration_enabled"`
	EmailVerifyEnabled  bool   `json:"email_verify_enabled"`
	PromoCodeEnabled    bool   `json:"promo_code_enabled"`
	TurnstileEnabled    bool   `json:"turnstile_enabled"`
	TurnstileSiteKey    string `json:"turnstile_site_key"`
	SiteName            string `json:"site_name"`
	SiteLogo            string `json:"site_logo"`
	SiteSubtitle        string `json:"site_subtitle"`
	APIBaseURL          string `json:"api_base_url"`
	ContactInfo         string `json:"contact_info"`
	DocURL              string `json:"doc_url"`
	HomeContent         string `json:"home_content"`
	HideCcsImportButton bool   `json:"hide_ccs_import_button"`
	LinuxDoOAuthEnabled bool   `json:"linuxdo_oauth_enabled"`
	Version             string `json:"version"`
REDACTED

// StreamTimeoutSettings 流超时处理配置 DTO
type StreamTimeoutSettings struct {
	Enabled                bool   `json:"enabled"`
	Action                 string `json:"action"`
	TempUnschedMinutes     int    `json:"temp_unsched_minutes"`
	ThresholdCount         int    `json:"threshold_count"`
	ThresholdWindowMinutes int    `json:"threshold_window_minutes"`
REDACTED
