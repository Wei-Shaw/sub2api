package dto

import (
	"encoding/json"
	"strings"
)

// CustomMenuItem represents a user-configured custom menu entry.
type CustomMenuItem struct {
	ID         string `json:"id"`
	Label      string `json:"label"`
	IconSVG    string `json:"icon_svg"`
	URL        string `json:"url"`
	Visibility string `json:"visibility"` // "user" or "admin"
	SortOrder  int    `json:"sort_order"`
}

// CustomEndpoint represents an admin-configured API endpoint for quick copy.
type CustomEndpoint struct {
	Name        string `json:"name"`
	Endpoint    string `json:"endpoint"`
	Description string `json:"description"`
}

// SystemSettings represents the admin settings API response payload.
type SystemSettings struct {
	RegistrationEnabled              bool     `json:"registration_enabled"`
	EmailVerifyEnabled               bool     `json:"email_verify_enabled"`
	RegistrationEmailSuffixWhitelist []string `json:"registration_email_suffix_whitelist"`
	PromoCodeEnabled                 bool     `json:"promo_code_enabled"`
	PasswordResetEnabled             bool     `json:"password_reset_enabled"`
	FrontendURL                      string   `json:"frontend_url"`
	InvitationCodeEnabled            bool     `json:"invitation_code_enabled"`
	TotpEnabled                      bool     `json:"totp_enabled"`                   // TOTP 双因素认证
	TotpEncryptionKeyConfigured      bool     `json:"totp_encryption_key_configured"` // TOTP 加密密钥是否已配置

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

	OIDCConnectEnabled                bool   `json:"oidc_connect_enabled"`
	OIDCConnectProviderName           string `json:"oidc_connect_provider_name"`
	OIDCConnectClientID               string `json:"oidc_connect_client_id"`
	OIDCConnectClientSecretConfigured bool   `json:"oidc_connect_client_secret_configured"`
	OIDCConnectIssuerURL              string `json:"oidc_connect_issuer_url"`
	OIDCConnectDiscoveryURL           string `json:"oidc_connect_discovery_url"`
	OIDCConnectAuthorizeURL           string `json:"oidc_connect_authorize_url"`
	OIDCConnectTokenURL               string `json:"oidc_connect_token_url"`
	OIDCConnectUserInfoURL            string `json:"oidc_connect_userinfo_url"`
	OIDCConnectJWKSURL                string `json:"oidc_connect_jwks_url"`
	OIDCConnectScopes                 string `json:"oidc_connect_scopes"`
	OIDCConnectRedirectURL            string `json:"oidc_connect_redirect_url"`
	OIDCConnectFrontendRedirectURL    string `json:"oidc_connect_frontend_redirect_url"`
	OIDCConnectTokenAuthMethod        string `json:"oidc_connect_token_auth_method"`
	OIDCConnectUsePKCE                bool   `json:"oidc_connect_use_pkce"`
	OIDCConnectValidateIDToken        bool   `json:"oidc_connect_validate_id_token"`
	OIDCConnectAllowedSigningAlgs     string `json:"oidc_connect_allowed_signing_algs"`
	OIDCConnectClockSkewSeconds       int    `json:"oidc_connect_clock_skew_seconds"`
	OIDCConnectRequireEmailVerified   bool   `json:"oidc_connect_require_email_verified"`
	OIDCConnectUserInfoEmailPath      string `json:"oidc_connect_userinfo_email_path"`
	OIDCConnectUserInfoIDPath         string `json:"oidc_connect_userinfo_id_path"`
	OIDCConnectUserInfoUsernamePath   string `json:"oidc_connect_userinfo_username_path"`

	SiteName                    string           `json:"site_name"`
	SiteLogo                    string           `json:"site_logo"`
	SiteSubtitle                string           `json:"site_subtitle"`
	APIBaseURL                  string           `json:"api_base_url"`
	ContactInfo                 string           `json:"contact_info"`
	DocURL                      string           `json:"doc_url"`
	HomeContent                 string           `json:"home_content"`
	HideCcsImportButton         bool             `json:"hide_ccs_import_button"`
	PurchaseSubscriptionEnabled bool             `json:"purchase_subscription_enabled"`
	PurchaseSubscriptionURL     string           `json:"purchase_subscription_url"`
	TableDefaultPageSize        int              `json:"table_default_page_size"`
	TablePageSizeOptions        []int            `json:"table_page_size_options"`
	CustomMenuItems             []CustomMenuItem `json:"custom_menu_items"`
	CustomEndpoints             []CustomEndpoint `json:"custom_endpoints"`

	DefaultConcurrency   int                          `json:"default_concurrency"`
	DefaultBalance       float64                      `json:"default_balance"`
	DefaultSubscriptions []DefaultSubscriptionSetting `json:"default_subscriptions"`

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
	OpsMonitoringEnabled         bool   `json:"ops_monitoring_enabled"`
	OpsRealtimeMonitoringEnabled bool   `json:"ops_realtime_monitoring_enabled"`
	OpsQueryModeDefault          string `json:"ops_query_mode_default"`
	OpsMetricsIntervalSeconds    int    `json:"ops_metrics_interval_seconds"`

	MinClaudeCodeVersion string `json:"min_claude_code_version"`
	MaxClaudeCodeVersion string `json:"max_claude_code_version"`

	// 分组隔离
	AllowUngroupedKeyScheduling bool `json:"allow_ungrouped_key_scheduling"`

	// Backend Mode
	BackendModeEnabled bool `json:"backend_mode_enabled"`

	// Gateway forwarding behavior
	EnableFingerprintUnification bool `json:"enable_fingerprint_unification"`
	EnableMetadataPassthrough    bool `json:"enable_metadata_passthrough"`
	EnableCCHSigning             bool `json:"enable_cch_signing"`

	// Web Search Emulation
	WebSearchEmulationEnabled bool `json:"web_search_emulation_enabled"`

	// Payment configuration
	PaymentEnabled           bool     `json:"payment_enabled"`
	PaymentMinAmount         float64  `json:"payment_min_amount"`
	PaymentMaxAmount         float64  `json:"payment_max_amount"`
	PaymentDailyLimit        float64  `json:"payment_daily_limit"`
	PaymentOrderTimeoutMin   int      `json:"payment_order_timeout_minutes"`
	PaymentMaxPendingOrders  int      `json:"payment_max_pending_orders"`
	PaymentEnabledTypes      []string `json:"payment_enabled_types"`
	PaymentBalanceDisabled   bool     `json:"payment_balance_disabled"`
	PaymentLoadBalanceStrat  string   `json:"payment_load_balance_strategy"`
	PaymentProductNamePrefix string   `json:"payment_product_name_prefix"`
	PaymentProductNameSuffix string   `json:"payment_product_name_suffix"`
	PaymentHelpImageURL      string   `json:"payment_help_image_url"`
	PaymentHelpText          string   `json:"payment_help_text"`

	// Cancel rate limit
	PaymentCancelRateLimitEnabled bool   `json:"payment_cancel_rate_limit_enabled"`
	PaymentCancelRateLimitMax     int    `json:"payment_cancel_rate_limit_max"`
	PaymentCancelRateLimitWindow  int    `json:"payment_cancel_rate_limit_window"`
	PaymentCancelRateLimitUnit    string `json:"payment_cancel_rate_limit_unit"`
	PaymentCancelRateLimitMode    string `json:"payment_cancel_rate_limit_window_mode"`

	// Balance low notification
	BalanceLowNotifyEnabled   bool     `json:"balance_low_notify_enabled"`
	BalanceLowNotifyThreshold float64  `json:"balance_low_notify_threshold"`
	AccountQuotaNotifyEmails  []string `json:"account_quota_notify_emails"`
}

type DefaultSubscriptionSetting struct {
	GroupID      int64 `json:"group_id"`
	ValidityDays int   `json:"validity_days"`
}

type PublicSettings struct {
	RegistrationEnabled              bool             `json:"registration_enabled"`
	EmailVerifyEnabled               bool             `json:"email_verify_enabled"`
	RegistrationEmailSuffixWhitelist []string         `json:"registration_email_suffix_whitelist"`
	PromoCodeEnabled                 bool             `json:"promo_code_enabled"`
	PasswordResetEnabled             bool             `json:"password_reset_enabled"`
	InvitationCodeEnabled            bool             `json:"invitation_code_enabled"`
	TotpEnabled                      bool             `json:"totp_enabled"` // TOTP 双因素认证
	TurnstileEnabled                 bool             `json:"turnstile_enabled"`
	TurnstileSiteKey                 string           `json:"turnstile_site_key"`
	SiteName                         string           `json:"site_name"`
	SiteLogo                         string           `json:"site_logo"`
	SiteSubtitle                     string           `json:"site_subtitle"`
	APIBaseURL                       string           `json:"api_base_url"`
	ContactInfo                      string           `json:"contact_info"`
	DocURL                           string           `json:"doc_url"`
	HomeContent                      string           `json:"home_content"`
	HideCcsImportButton              bool             `json:"hide_ccs_import_button"`
	PurchaseSubscriptionEnabled      bool             `json:"purchase_subscription_enabled"`
	PurchaseSubscriptionURL          string           `json:"purchase_subscription_url"`
	TableDefaultPageSize             int              `json:"table_default_page_size"`
	TablePageSizeOptions             []int            `json:"table_page_size_options"`
	CustomMenuItems                  []CustomMenuItem `json:"custom_menu_items"`
	CustomEndpoints                  []CustomEndpoint `json:"custom_endpoints"`
	LinuxDoOAuthEnabled              bool             `json:"linuxdo_oauth_enabled"`
	OIDCOAuthEnabled                 bool             `json:"oidc_oauth_enabled"`
	OIDCOAuthProviderName            string           `json:"oidc_oauth_provider_name"`
	SoraClientEnabled                bool             `json:"sora_client_enabled"`
	BackendModeEnabled               bool             `json:"backend_mode_enabled"`
	PaymentEnabled                   bool             `json:"payment_enabled"`
	Version                          string           `json:"version"`
	BalanceLowNotifyEnabled          bool             `json:"balance_low_notify_enabled"`
	AccountQuotaNotifyEnabled        bool             `json:"account_quota_notify_enabled"`
	BalanceLowNotifyThreshold        float64          `json:"balance_low_notify_threshold"`
}

// OverloadCooldownSettings 529过载冷却配置 DTO
type OverloadCooldownSettings struct {
	Enabled         bool `json:"enabled"`
	CooldownMinutes int  `json:"cooldown_minutes"`
}

// StreamTimeoutSettings 流超时处理配置 DTO
type StreamTimeoutSettings struct {
	Enabled                bool   `json:"enabled"`
	Action                 string `json:"action"`
	TempUnschedMinutes     int    `json:"temp_unsched_minutes"`
	ThresholdCount         int    `json:"threshold_count"`
	ThresholdWindowMinutes int    `json:"threshold_window_minutes"`
}

// RectifierSettings 请求整流器配置 DTO
type RectifierSettings struct {
	Enabled                  bool     `json:"enabled"`
	ThinkingSignatureEnabled bool     `json:"thinking_signature_enabled"`
	ThinkingBudgetEnabled    bool     `json:"thinking_budget_enabled"`
	APIKeySignatureEnabled   bool     `json:"apikey_signature_enabled"`
	APIKeySignaturePatterns  []string `json:"apikey_signature_patterns"`
}

// BetaPolicyRule Beta 策略规则 DTO
type BetaPolicyRule struct {
	BetaToken            string   `json:"beta_token"`
	Action               string   `json:"action"`
	Scope                string   `json:"scope"`
	ErrorMessage         string   `json:"error_message,omitempty"`
	ModelWhitelist       []string `json:"model_whitelist,omitempty"`
	FallbackAction       string   `json:"fallback_action,omitempty"`
	FallbackErrorMessage string   `json:"fallback_error_message,omitempty"`
}

// BetaPolicySettings Beta 策略配置 DTO
type BetaPolicySettings struct {
	Rules []BetaPolicyRule `json:"rules"`
}

// ParseCustomMenuItems parses a JSON string into a slice of CustomMenuItem.
// Returns empty slice on empty/invalid input.
func ParseCustomMenuItems(raw string) []CustomMenuItem {
	raw = strings.TrimSpace(raw)
	if raw == "" || raw == "[]" {
		return []CustomMenuItem{}
	}
	var items []CustomMenuItem
	if err := json.Unmarshal([]byte(raw), &items); err != nil {
		return []CustomMenuItem{}
	}
	return items
}

// ParseUserVisibleMenuItems parses custom menu items and filters out admin-only entries.
func ParseUserVisibleMenuItems(raw string) []CustomMenuItem {
	items := ParseCustomMenuItems(raw)
	filtered := make([]CustomMenuItem, 0, len(items))
	for _, item := range items {
		if item.Visibility != "admin" {
			filtered = append(filtered, item)
		}
	}
	return filtered
}

// ParseCustomEndpoints parses a JSON string into a slice of CustomEndpoint.
// Returns empty slice on empty/invalid input.
func ParseCustomEndpoints(raw string) []CustomEndpoint {
	raw = strings.TrimSpace(raw)
	if raw == "" || raw == "[]" {
		return []CustomEndpoint{}
	}
	var items []CustomEndpoint
	if err := json.Unmarshal([]byte(raw), &items); err != nil {
		return []CustomEndpoint{}
	}
	return items
}
