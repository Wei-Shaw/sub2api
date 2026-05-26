package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/require"
)

func resetViperWithJWTSecret(t *testing.T) {
	t.Helper()
	viper.Reset()
	t.Setenv("JWT_SECRET", strings.Repeat("x", 32))
}

func TestLoadForBootstrapAllowsMissingJWTSecret(t *testing.T) {
	viper.Reset()
	t.Setenv("JWT_SECRET", "")

	cfg, err := LoadForBootstrap()
	if err != nil {
		t.Fatalf("LoadForBootstrap() error: %v", err)
	}
	if cfg.JWT.Secret != "" {
		t.Fatalf("LoadForBootstrap() should keep empty jwt.secret during bootstrap")
	}
}

func TestNormalizeRunMode(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"simple", "simple"},
		{"SIMPLE", "simple"},
		{"standard", "standard"},
		{"invalid", "standard"},
		{"", "standard"},
	}

	for _, tt := range tests {
		result := NormalizeRunMode(tt.input)
		if result != tt.expected {
			t.Errorf("NormalizeRunMode(%q) = %q, want %q", tt.input, result, tt.expected)
		}
	}
}

func TestLoadDefaultSchedulingConfig(t *testing.T) {
	resetViperWithJWTSecret(t)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}

	if cfg.Gateway.Scheduling.StickySessionMaxWaiting != 3 {
		t.Fatalf("StickySessionMaxWaiting = %d, want 3", cfg.Gateway.Scheduling.StickySessionMaxWaiting)
	}
	if cfg.Gateway.Scheduling.StickySessionWaitTimeout != 120*time.Second {
		t.Fatalf("StickySessionWaitTimeout = %v, want 120s", cfg.Gateway.Scheduling.StickySessionWaitTimeout)
	}
	if cfg.Gateway.Scheduling.FallbackWaitTimeout != 30*time.Second {
		t.Fatalf("FallbackWaitTimeout = %v, want 30s", cfg.Gateway.Scheduling.FallbackWaitTimeout)
	}
	if cfg.Gateway.Scheduling.FallbackMaxWaiting != 100 {
		t.Fatalf("FallbackMaxWaiting = %d, want 100", cfg.Gateway.Scheduling.FallbackMaxWaiting)
	}
	if !cfg.Gateway.Scheduling.LoadBatchEnabled {
		t.Fatalf("LoadBatchEnabled = false, want true")
	}
	if cfg.Gateway.Scheduling.SlotCleanupInterval != 30*time.Second {
		t.Fatalf("SlotCleanupInterval = %v, want 30s", cfg.Gateway.Scheduling.SlotCleanupInterval)
	}
}

func TestLoadDefaultOpenAIWSConfig(t *testing.T) {
	resetViperWithJWTSecret(t)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}

	if !cfg.Gateway.OpenAIWS.Enabled {
		t.Fatalf("Gateway.OpenAIWS.Enabled = false, want true")
	}
	if !cfg.Gateway.OpenAIWS.ResponsesWebsocketsV2 {
		t.Fatalf("Gateway.OpenAIWS.ResponsesWebsocketsV2 = false, want true")
	}
	if cfg.Gateway.OpenAIWS.ResponsesWebsockets {
		t.Fatalf("Gateway.OpenAIWS.ResponsesWebsockets = true, want false")
	}
	if !cfg.Gateway.OpenAIWS.DynamicMaxConnsByAccountConcurrencyEnabled {
		t.Fatalf("Gateway.OpenAIWS.DynamicMaxConnsByAccountConcurrencyEnabled = false, want true")
	}
	if cfg.Gateway.OpenAIWS.OAuthMaxConnsFactor != 1.0 {
		t.Fatalf("Gateway.OpenAIWS.OAuthMaxConnsFactor = %v, want 1.0", cfg.Gateway.OpenAIWS.OAuthMaxConnsFactor)
	}
	if cfg.Gateway.OpenAIWS.APIKeyMaxConnsFactor != 1.0 {
		t.Fatalf("Gateway.OpenAIWS.APIKeyMaxConnsFactor = %v, want 1.0", cfg.Gateway.OpenAIWS.APIKeyMaxConnsFactor)
	}
	if cfg.Gateway.OpenAIWS.StickySessionTTLSeconds != 3600 {
		t.Fatalf("Gateway.OpenAIWS.StickySessionTTLSeconds = %d, want 3600", cfg.Gateway.OpenAIWS.StickySessionTTLSeconds)
	}
	if !cfg.Gateway.OpenAIWS.SessionHashReadOldFallback {
		t.Fatalf("Gateway.OpenAIWS.SessionHashReadOldFallback = false, want true")
	}
	if !cfg.Gateway.OpenAIWS.SessionHashDualWriteOld {
		t.Fatalf("Gateway.OpenAIWS.SessionHashDualWriteOld = false, want true")
	}
	if !cfg.Gateway.OpenAIWS.MetadataBridgeEnabled {
		t.Fatalf("Gateway.OpenAIWS.MetadataBridgeEnabled = false, want true")
	}
	if cfg.Gateway.OpenAIWS.StickyResponseIDTTLSeconds != 3600 {
		t.Fatalf("Gateway.OpenAIWS.StickyResponseIDTTLSeconds = %d, want 3600", cfg.Gateway.OpenAIWS.StickyResponseIDTTLSeconds)
	}
	if cfg.Gateway.OpenAIWS.FallbackCooldownSeconds != 30 {
		t.Fatalf("Gateway.OpenAIWS.FallbackCooldownSeconds = %d, want 30", cfg.Gateway.OpenAIWS.FallbackCooldownSeconds)
	}
	if cfg.Gateway.OpenAIWS.EventFlushBatchSize != 1 {
		t.Fatalf("Gateway.OpenAIWS.EventFlushBatchSize = %d, want 1", cfg.Gateway.OpenAIWS.EventFlushBatchSize)
	}
	if cfg.Gateway.OpenAIWS.EventFlushIntervalMS != 10 {
		t.Fatalf("Gateway.OpenAIWS.EventFlushIntervalMS = %d, want 10", cfg.Gateway.OpenAIWS.EventFlushIntervalMS)
	}
	if cfg.Gateway.OpenAIWS.PrewarmCooldownMS != 300 {
		t.Fatalf("Gateway.OpenAIWS.PrewarmCooldownMS = %d, want 300", cfg.Gateway.OpenAIWS.PrewarmCooldownMS)
	}
	if cfg.Gateway.OpenAIWS.RetryBackoffInitialMS != 120 {
		t.Fatalf("Gateway.OpenAIWS.RetryBackoffInitialMS = %d, want 120", cfg.Gateway.OpenAIWS.RetryBackoffInitialMS)
	}
	if cfg.Gateway.OpenAIWS.RetryBackoffMaxMS != 2000 {
		t.Fatalf("Gateway.OpenAIWS.RetryBackoffMaxMS = %d, want 2000", cfg.Gateway.OpenAIWS.RetryBackoffMaxMS)
	}
	if cfg.Gateway.OpenAIWS.RetryJitterRatio != 0.2 {
		t.Fatalf("Gateway.OpenAIWS.RetryJitterRatio = %v, want 0.2", cfg.Gateway.OpenAIWS.RetryJitterRatio)
	}
	if cfg.Gateway.OpenAIWS.RetryTotalBudgetMS != 5000 {
		t.Fatalf("Gateway.OpenAIWS.RetryTotalBudgetMS = %d, want 5000", cfg.Gateway.OpenAIWS.RetryTotalBudgetMS)
	}
	if cfg.Gateway.OpenAIWS.PayloadLogSampleRate != 0.2 {
		t.Fatalf("Gateway.OpenAIWS.PayloadLogSampleRate = %v, want 0.2", cfg.Gateway.OpenAIWS.PayloadLogSampleRate)
	}
	if !cfg.Gateway.OpenAIWS.StoreDisabledForceNewConn {
		t.Fatalf("Gateway.OpenAIWS.StoreDisabledForceNewConn = false, want true")
	}
	if cfg.Gateway.OpenAIWS.StoreDisabledConnMode != "strict" {
		t.Fatalf("Gateway.OpenAIWS.StoreDisabledConnMode = %q, want %q", cfg.Gateway.OpenAIWS.StoreDisabledConnMode, "strict")
	}
	if cfg.Gateway.OpenAIWS.ModeRouterV2Enabled {
		t.Fatalf("Gateway.OpenAIWS.ModeRouterV2Enabled = true, want false")
	}
	if cfg.Gateway.OpenAIWS.IngressModeDefault != "ctx_pool" {
		t.Fatalf("Gateway.OpenAIWS.IngressModeDefault = %q, want %q", cfg.Gateway.OpenAIWS.IngressModeDefault, "ctx_pool")
	}
}

func TestLoadOpenAIWSStickyTTLCompatibility(t *testing.T) {
	resetViperWithJWTSecret(t)
	t.Setenv("GATEWAY_OPENAI_WS_STICKY_RESPONSE_ID_TTL_SECONDS", "0")
	t.Setenv("GATEWAY_OPENAI_WS_STICKY_PREVIOUS_RESPONSE_TTL_SECONDS", "7200")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}

	if cfg.Gateway.OpenAIWS.StickyResponseIDTTLSeconds != 7200 {
		t.Fatalf("StickyResponseIDTTLSeconds = %d, want 7200", cfg.Gateway.OpenAIWS.StickyResponseIDTTLSeconds)
	}
}

func TestLoadDefaultIdempotencyConfig(t *testing.T) {
	resetViperWithJWTSecret(t)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}

	if !cfg.Idempotency.ObserveOnly {
		t.Fatalf("Idempotency.ObserveOnly = false, want true")
	}
	if cfg.Idempotency.DefaultTTLSeconds != 86400 {
		t.Fatalf("Idempotency.DefaultTTLSeconds = %d, want 86400", cfg.Idempotency.DefaultTTLSeconds)
	}
	if cfg.Idempotency.SystemOperationTTLSeconds != 3600 {
		t.Fatalf("Idempotency.SystemOperationTTLSeconds = %d, want 3600", cfg.Idempotency.SystemOperationTTLSeconds)
	}
}

func TestLoadIdempotencyConfigFromEnv(t *testing.T) {
	resetViperWithJWTSecret(t)
	t.Setenv("IDEMPOTENCY_OBSERVE_ONLY", "false")
	t.Setenv("IDEMPOTENCY_DEFAULT_TTL_SECONDS", "600")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	if cfg.Idempotency.ObserveOnly {
		t.Fatalf("Idempotency.ObserveOnly = true, want false")
	}
	if cfg.Idempotency.DefaultTTLSeconds != 600 {
		t.Fatalf("Idempotency.DefaultTTLSeconds = %d, want 600", cfg.Idempotency.DefaultTTLSeconds)
	}
}

func TestLoadSchedulingConfigFromEnv(t *testing.T) {
	resetViperWithJWTSecret(t)
	t.Setenv("GATEWAY_SCHEDULING_STICKY_SESSION_MAX_WAITING", "5")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}

	if cfg.Gateway.Scheduling.StickySessionMaxWaiting != 5 {
		t.Fatalf("StickySessionMaxWaiting = %d, want 5", cfg.Gateway.Scheduling.StickySessionMaxWaiting)
	}
}

func TestLoadWeChatConnectConfigFromLegacyEnv(t *testing.T) {
	resetViperWithJWTSecret(t)
	t.Setenv("WECHAT_OAUTH_OPEN_APP_ID", "wx-open-app")
	t.Setenv("WECHAT_OAUTH_OPEN_APP_SECRET", "wx-open-secret")
	t.Setenv("WECHAT_OAUTH_MP_APP_ID", "wx-mp-app")
	t.Setenv("WECHAT_OAUTH_MP_APP_SECRET", "wx-mp-secret")
	t.Setenv("WECHAT_OAUTH_FRONTEND_REDIRECT_URL", "/auth/wechat/legacy-callback")

	cfg, err := Load()
	require.NoError(t, err)
	require.True(t, cfg.WeChat.Enabled)
	require.True(t, cfg.WeChat.OpenEnabled)
	require.True(t, cfg.WeChat.MPEnabled)
	require.False(t, cfg.WeChat.MobileEnabled)
	require.Equal(t, "open", cfg.WeChat.Mode)
	require.Equal(t, "wx-open-app", cfg.WeChat.OpenAppID)
	require.Equal(t, "wx-open-secret", cfg.WeChat.OpenAppSecret)
	require.Equal(t, "wx-mp-app", cfg.WeChat.MPAppID)
	require.Equal(t, "wx-mp-secret", cfg.WeChat.MPAppSecret)
	require.Equal(t, "/auth/wechat/legacy-callback", cfg.WeChat.FrontendRedirectURL)
}

func TestLoadDefaultOIDCSecurityDefaults(t *testing.T) {
	resetViperWithJWTSecret(t)

	cfg, err := Load()
	require.NoError(t, err)
	require.True(t, cfg.OIDC.UsePKCE)
	require.True(t, cfg.OIDC.ValidateIDToken)
	require.False(t, cfg.OIDC.UsePKCEExplicit)
	require.False(t, cfg.OIDC.ValidateIDTokenExplicit)
}

func TestLoadExplicitOIDCSecurityDefaultsFromEnvMarksFlagsExplicit(t *testing.T) {
	resetViperWithJWTSecret(t)
	t.Setenv("OIDC_CONNECT_USE_PKCE", "false")
	t.Setenv("OIDC_CONNECT_VALIDATE_ID_TOKEN", "false")

	cfg, err := Load()
	require.NoError(t, err)
	require.False(t, cfg.OIDC.UsePKCE)
	require.False(t, cfg.OIDC.ValidateIDToken)
	require.True(t, cfg.OIDC.UsePKCEExplicit)
	require.True(t, cfg.OIDC.ValidateIDTokenExplicit)
}

func TestLoadForcedCodexInstructionsTemplate(t *testing.T) {
	resetViperWithJWTSecret(t)

	tempDir := t.TempDir()
	templatePath := filepath.Join(tempDir, "codex-instructions.md.tmpl")
	configPath := filepath.Join(tempDir, "config.yaml")

	require.NoError(t, os.WriteFile(templatePath, []byte("server-prefix\n\n{{ .ExistingInstructions }}"), 0o644))
	yamlSafePath := filepath.ToSlash(templatePath)
	require.NoError(t, os.WriteFile(configPath, []byte("gateway:\n  forced_codex_instructions_template_file: \""+yamlSafePath+"\"\n"), 0o644))
	t.Setenv("DATA_DIR", tempDir)

	cfg, err := Load()
	require.NoError(t, err)
	require.Equal(t, yamlSafePath, cfg.Gateway.ForcedCodexInstructionsTemplateFile)
	require.Equal(t, "server-prefix\n\n{{ .ExistingInstructions }}", cfg.Gateway.ForcedCodexInstructionsTemplate)
}

func TestLoadDefaultSecurityToggles(t *testing.T) {
	resetViperWithJWTSecret(t)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}

	if cfg.Security.URLAllowlist.Enabled {
		t.Fatalf("URLAllowlist.Enabled = true, want false")
	}
	if !cfg.Security.URLAllowlist.AllowInsecureHTTP {
		t.Fatalf("URLAllowlist.AllowInsecureHTTP = false, want true")
	}
	if !cfg.Security.URLAllowlist.AllowPrivateHosts {
		t.Fatalf("URLAllowlist.AllowPrivateHosts = false, want true")
	}
	if !cfg.Security.ResponseHeaders.Enabled {
		t.Fatalf("ResponseHeaders.Enabled = false, want true")
	}
}

func TestLoadDefaultServerMode(t *testing.T) {
	resetViperWithJWTSecret(t)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}

	if cfg.Server.Mode != "release" {
		t.Fatalf("Server.Mode = %q, want %q", cfg.Server.Mode, "release")
	}
}

func TestLoadDefaultJWTAccessTokenExpireMinutes(t *testing.T) {
	resetViperWithJWTSecret(t)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}

	if cfg.JWT.ExpireHour != 24 {
		t.Fatalf("JWT.ExpireHour = %d, want 24", cfg.JWT.ExpireHour)
	}
	if cfg.JWT.AccessTokenExpireMinutes != 0 {
		t.Fatalf("JWT.AccessTokenExpireMinutes = %d, want 0", cfg.JWT.AccessTokenExpireMinutes)
	}
}

func TestLoadJWTAccessTokenExpireMinutesFromEnv(t *testing.T) {
	resetViperWithJWTSecret(t)
	t.Setenv("JWT_ACCESS_TOKEN_EXPIRE_MINUTES", "90")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}

	if cfg.JWT.AccessTokenExpireMinutes != 90 {
		t.Fatalf("JWT.AccessTokenExpireMinutes = %d, want 90", cfg.JWT.AccessTokenExpireMinutes)
	}
}

func TestLoadDefaultDatabaseSSLMode(t *testing.T) {
	resetViperWithJWTSecret(t)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}

	if cfg.Database.SSLMode != "prefer" {
		t.Fatalf("Database.SSLMode = %q, want %q", cfg.Database.SSLMode, "prefer")
	}
}

func TestValidateLinuxDoFrontendRedirectURL(t *testing.T) {
	resetViperWithJWTSecret(t)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}

	cfg.LinuxDo.Enabled = true
	cfg.LinuxDo.ClientID = "test-client"
	cfg.LinuxDo.ClientSecret = "test-secret"
	cfg.LinuxDo.RedirectURL = "https://example.com/api/v1/auth/oauth/linuxdo/callback"
	cfg.LinuxDo.TokenAuthMethod = "client_secret_post"
	cfg.LinuxDo.UsePKCE = true

	cfg.LinuxDo.FrontendRedirectURL = "javascript:alert(1)"
	err = cfg.Validate()
	if err == nil {
		t.Fatalf("Validate() expected error for javascript scheme, got nil")
	}
	if !strings.Contains(err.Error(), "linuxdo_connect.frontend_redirect_url") {
		t.Fatalf("Validate() expected frontend_redirect_url error, got: %v", err)
	}
}

func TestValidateLinuxDoAllowsDisablingPKCEForCompatibility(t *testing.T) {
	resetViperWithJWTSecret(t)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}

	cfg.LinuxDo.Enabled = true
	cfg.LinuxDo.ClientID = "test-client"
	cfg.LinuxDo.ClientSecret = ""
	cfg.LinuxDo.RedirectURL = "https://example.com/api/v1/auth/oauth/linuxdo/callback"
	cfg.LinuxDo.FrontendRedirectURL = "/auth/linuxdo/callback"
	cfg.LinuxDo.TokenAuthMethod = "none"
	cfg.LinuxDo.UsePKCE = false

	err = cfg.Validate()
	if err != nil {
		t.Fatalf("Validate() expected LinuxDo config without PKCE to pass for compatibility, got: %v", err)
	}
}

func TestValidateOIDCScopesMustContainOpenID(t *testing.T) {
	resetViperWithJWTSecret(t)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}

	cfg.OIDC.Enabled = true
	cfg.OIDC.ClientID = "oidc-client"
	cfg.OIDC.ClientSecret = "oidc-secret"
	cfg.OIDC.IssuerURL = "https://issuer.example.com"
	cfg.OIDC.AuthorizeURL = "https://issuer.example.com/auth"
	cfg.OIDC.TokenURL = "https://issuer.example.com/token"
	cfg.OIDC.JWKSURL = "https://issuer.example.com/jwks"
	cfg.OIDC.RedirectURL = "https://example.com/api/v1/auth/oauth/oidc/callback"
	cfg.OIDC.FrontendRedirectURL = "/auth/oidc/callback"
	cfg.OIDC.Scopes = "profile email"
	cfg.OIDC.UsePKCE = true

	err = cfg.Validate()
	if err == nil {
		t.Fatalf("Validate() expected error when scopes do not include openid, got nil")
	}
	if !strings.Contains(err.Error(), "oidc_connect.scopes") {
		t.Fatalf("Validate() expected oidc_connect.scopes error, got: %v", err)
	}
}

func TestValidateOIDCAllowsIssuerOnlyEndpointsWithDiscoveryFallback(t *testing.T) {
	resetViperWithJWTSecret(t)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}

	cfg.OIDC.Enabled = true
	cfg.OIDC.ClientID = "oidc-client"
	cfg.OIDC.ClientSecret = "oidc-secret"
	cfg.OIDC.IssuerURL = "https://issuer.example.com"
	cfg.OIDC.AuthorizeURL = ""
	cfg.OIDC.TokenURL = ""
	cfg.OIDC.JWKSURL = ""
	cfg.OIDC.RedirectURL = "https://example.com/api/v1/auth/oauth/oidc/callback"
	cfg.OIDC.FrontendRedirectURL = "/auth/oidc/callback"
	cfg.OIDC.Scopes = "openid email profile"
	cfg.OIDC.ValidateIDToken = true
	cfg.OIDC.UsePKCE = true

	err = cfg.Validate()
	if err != nil {
		t.Fatalf("Validate() expected issuer-only OIDC config to pass with discovery fallback, got: %v", err)
	}
}

func TestValidateOIDCAllowsExplicitCompatibilityOverridesForPKCEAndIDTokenValidation(t *testing.T) {
	resetViperWithJWTSecret(t)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}

	cfg.OIDC.Enabled = true
	cfg.OIDC.ClientID = "oidc-client"
	cfg.OIDC.ClientSecret = "oidc-secret"
	cfg.OIDC.IssuerURL = "https://issuer.example.com"
	cfg.OIDC.AuthorizeURL = "https://issuer.example.com/auth"
	cfg.OIDC.TokenURL = "https://issuer.example.com/token"
	cfg.OIDC.UserInfoURL = "https://issuer.example.com/userinfo"
	cfg.OIDC.RedirectURL = "https://example.com/api/v1/auth/oauth/oidc/callback"
	cfg.OIDC.FrontendRedirectURL = "/auth/oidc/callback"
	cfg.OIDC.Scopes = "openid email profile"
	cfg.OIDC.UsePKCE = false
	cfg.OIDC.ValidateIDToken = false
	cfg.OIDC.JWKSURL = ""
	cfg.OIDC.AllowedSigningAlgs = ""

	err = cfg.Validate()
	if err != nil {
		t.Fatalf("Validate() expected OIDC config without PKCE/id_token validation to pass for compatibility, got: %v", err)
	}
}

func TestLoadDefaultDashboardCacheConfig(t *testing.T) {
	resetViperWithJWTSecret(t)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}

	if !cfg.Dashboard.Enabled {
		t.Fatalf("Dashboard.Enabled = false, want true")
	}
	if cfg.Dashboard.KeyPrefix != "sub2api:" {
		t.Fatalf("Dashboard.KeyPrefix = %q, want %q", cfg.Dashboard.KeyPrefix, "sub2api:")
	}
	if cfg.Dashboard.StatsFreshTTLSeconds != 15 {
		t.Fatalf("Dashboard.StatsFreshTTLSeconds = %d, want 15", cfg.Dashboard.StatsFreshTTLSeconds)
	}
	if cfg.Dashboard.StatsTTLSeconds != 30 {
		t.Fatalf("Dashboard.StatsTTLSeconds = %d, want 30", cfg.Dashboard.StatsTTLSeconds)
	}
	if cfg.Dashboard.StatsRefreshTimeoutSeconds != 30 {
		t.Fatalf("Dashboard.StatsRefreshTimeoutSeconds = %d, want 30", cfg.Dashboard.StatsRefreshTimeoutSeconds)
	}
}

func TestValidateDashboardCacheConfigEnabled(t *testing.T) {
	resetViperWithJWTSecret(t)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}

	cfg.Dashboard.Enabled = true
	cfg.Dashboard.StatsFreshTTLSeconds = 10
	cfg.Dashboard.StatsTTLSeconds = 5
	err = cfg.Validate()
	if err == nil {
		t.Fatalf("Validate() expected error for stats_fresh_ttl_seconds > stats_ttl_seconds, got nil")
	}
	if !strings.Contains(err.Error(), "dashboard_cache.stats_fresh_ttl_seconds") {
		t.Fatalf("Validate() expected stats_fresh_ttl_seconds error, got: %v", err)
	}
}

func TestValidateDashboardCacheConfigDisabled(t *testing.T) {
	resetViperWithJWTSecret(t)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}

	cfg.Dashboard.Enabled = false
	cfg.Dashboard.StatsTTLSeconds = -1
	err = cfg.Validate()
	if err == nil {
		t.Fatalf("Validate() expected error for negative stats_ttl_seconds, got nil")
	}
	if !strings.Contains(err.Error(), "dashboard_cache.stats_ttl_seconds") {
		t.Fatalf("Validate() expected stats_ttl_seconds error, got: %v", err)
	}
}

func TestLoadDefaultDashboardAggregationConfig(t *testing.T) {
	resetViperWithJWTSecret(t)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}

	if !cfg.DashboardAgg.Enabled {
		t.Fatalf("DashboardAgg.Enabled = false, want true")
	}
	if cfg.DashboardAgg.IntervalSeconds != 60 {
		t.Fatalf("DashboardAgg.IntervalSeconds = %d, want 60", cfg.DashboardAgg.IntervalSeconds)
	}
	if cfg.DashboardAgg.LookbackSeconds != 120 {
		t.Fatalf("DashboardAgg.LookbackSeconds = %d, want 120", cfg.DashboardAgg.LookbackSeconds)
	}
	if cfg.DashboardAgg.BackfillEnabled {
		t.Fatalf("DashboardAgg.BackfillEnabled = true, want false")
	}
	if cfg.DashboardAgg.BackfillMaxDays != 31 {
		t.Fatalf("DashboardAgg.BackfillMaxDays = %d, want 31", cfg.DashboardAgg.BackfillMaxDays)
	}
	if cfg.DashboardAgg.Retention.UsageLogsDays != 90 {
		t.Fatalf("DashboardAgg.Retention.UsageLogsDays = %d, want 90", cfg.DashboardAgg.Retention.UsageLogsDays)
	}
	if cfg.DashboardAgg.Retention.UsageBillingDedupDays != 365 {
		t.Fatalf("DashboardAgg.Retention.UsageBillingDedupDays = %d, want 365", cfg.DashboardAgg.Retention.UsageBillingDedupDays)
	}
	if cfg.DashboardAgg.Retention.HourlyDays != 180 {
		t.Fatalf("DashboardAgg.Retention.HourlyDays = %d, want 180", cfg.DashboardAgg.Retention.HourlyDays)
	}
	if cfg.DashboardAgg.Retention.DailyDays != 730 {
		t.Fatalf("DashboardAgg.Retention.DailyDays = %d, want 730", cfg.DashboardAgg.Retention.DailyDays)
	}
	if cfg.DashboardAgg.RecomputeDays != 2 {
		t.Fatalf("DashboardAgg.RecomputeDays = %d, want 2", cfg.DashboardAgg.RecomputeDays)
	}
}

func TestValidateDashboardAggregationConfigDisabled(t *testing.T) {
	resetViperWithJWTSecret(t)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}

	cfg.DashboardAgg.Enabled = false
	cfg.DashboardAgg.IntervalSeconds = -1
	err = cfg.Validate()
	if err == nil {
		t.Fatalf("Validate() expected error for negative dashboard_aggregation.interval_seconds, got nil")
	}
	if !strings.Contains(err.Error(), "dashboard_aggregation.interval_seconds") {
		t.Fatalf("Validate() expected interval_seconds error, got: %v", err)
	}
}

func TestValidateDashboardAggregationBackfillMaxDays(t *testing.T) {
	resetViperWithJWTSecret(t)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}

	cfg.DashboardAgg.BackfillEnabled = true
	cfg.DashboardAgg.BackfillMaxDays = 0
	err = cfg.Validate()
	if err == nil {
		t.Fatalf("Validate() expected error for dashboard_aggregation.backfill_max_days, got nil")
	}
	if !strings.Contains(err.Error(), "dashboard_aggregation.backfill_max_days") {
		t.Fatalf("Validate() expected backfill_max_days error, got: %v", err)
	}
}

func TestLoadDefaultUsageCleanupConfig(t *testing.T) {
	resetViperWithJWTSecret(t)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}

	if !cfg.UsageCleanup.Enabled {
		t.Fatalf("UsageCleanup.Enabled = false, want true")
	}
	if cfg.UsageCleanup.MaxRangeDays != 31 {
		t.Fatalf("UsageCleanup.MaxRangeDays = %d, want 31", cfg.UsageCleanup.MaxRangeDays)
	}
	if cfg.UsageCleanup.BatchSize != 5000 {
		t.Fatalf("UsageCleanup.BatchSize = %d, want 5000", cfg.UsageCleanup.BatchSize)
	}
	if cfg.UsageCleanup.WorkerIntervalSeconds != 10 {
		t.Fatalf("UsageCleanup.WorkerIntervalSeconds = %d, want 10", cfg.UsageCleanup.WorkerIntervalSeconds)
	}
	if cfg.UsageCleanup.TaskTimeoutSeconds != 1800 {
		t.Fatalf("UsageCleanup.TaskTimeoutSeconds = %d, want 1800", cfg.UsageCleanup.TaskTimeoutSeconds)
	}
}

func TestValidateUsageCleanupConfigEnabled(t *testing.T) {
	resetViperWithJWTSecret(t)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}

	cfg.UsageCleanup.Enabled = true
	cfg.UsageCleanup.MaxRangeDays = 0
	err = cfg.Validate()
	if err == nil {
		t.Fatalf("Validate() expected error for usage_cleanup.max_range_days, got nil")
	}
	if !strings.Contains(err.Error(), "usage_cleanup.max_range_days") {
		t.Fatalf("Validate() expected max_range_days error, got: %v", err)
	}
}

func TestValidateUsageCleanupConfigDisabled(t *testing.T) {
	resetViperWithJWTSecret(t)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}

	cfg.UsageCleanup.Enabled = false
	cfg.UsageCleanup.BatchSize = -1
	err = cfg.Validate()
	if err == nil {
		t.Fatalf("Validate() expected error for usage_cleanup.batch_size, got nil")
	}
	if !strings.Contains(err.Error(), "usage_cleanup.batch_size") {
		t.Fatalf("Validate() expected batch_size error, got: %v", err)
	}
}

func TestConfigAddressHelpers(t *testing.T) {
	server := ServerConfig{Host: "127.0.0.1", Port: 9000}
	if server.Address() != "127.0.0.1:9000" {
		t.Fatalf("ServerConfig.Address() = %q", server.Address())
	}

	dbCfg := DatabaseConfig{
		Host:     "localhost",
		Port:     5432,
		User:     "postgres",
		Password: "",
		DBName:   "sub2api",
		SSLMode:  "disable",
	}
	if !strings.Contains(dbCfg.DSN(), "password=") {
	} else {
		t.Fatalf("DatabaseConfig.DSN() should not include password when empty")
	}

	dbCfg.Password = "secret"
	if !strings.Contains(dbCfg.DSN(), "password=secret") {
		t.Fatalf("DatabaseConfig.DSN() missing password")
	}

	dbCfg.Password = ""
	if strings.Contains(dbCfg.DSNWithTimezone("UTC"), "password=") {
		t.Fatalf("DatabaseConfig.DSNWithTimezone() should omit password when empty")
	}

	if !strings.Contains(dbCfg.DSNWithTimezone(""), "TimeZone=Asia/Shanghai") {
		t.Fatalf("DatabaseConfig.DSNWithTimezone() should use default timezone")
	}
	if !strings.Contains(dbCfg.DSNWithTimezone("UTC"), "TimeZone=UTC") {
		t.Fatalf("DatabaseConfig.DSNWithTimezone() should use provided timezone")
	}

	redis := RedisConfig{Host: "redis", Port: 6379}
	if redis.Address() != "redis:6379" {
		t.Fatalf("RedisConfig.Address() = %q", redis.Address())
	}
}

func TestNormalizeStringSlice(t *testing.T) {
	values := normalizeStringSlice([]string{" a ", "", "b", "   ", "c"})
	if len(values) != 3 || values[0] != "a" || values[1] != "b" || values[2] != "c" {
		t.Fatalf("normalizeStringSlice() unexpected result: %#v", values)
	}
	if normalizeStringSlice(nil) != nil {
		t.Fatalf("normalizeStringSlice(nil) expected nil slice")
	}
}

func TestGetServerAddressFromEnv(t *testing.T) {
	t.Setenv("SERVER_HOST", "127.0.0.1")
	t.Setenv("SERVER_PORT", "9090")

	address := GetServerAddress()
	if address != "127.0.0.1:9090" {
		t.Fatalf("GetServerAddress() = %q", address)
	}
}

func TestValidateAbsoluteHTTPURL(t *testing.T) {
	if err := ValidateAbsoluteHTTPURL("https://example.com/path"); err != nil {
		t.Fatalf("ValidateAbsoluteHTTPURL valid url error: %v", err)
	}
	if err := ValidateAbsoluteHTTPURL(""); err == nil {
		t.Fatalf("ValidateAbsoluteHTTPURL should reject empty url")
	}
	if err := ValidateAbsoluteHTTPURL("/relative"); err == nil {
		t.Fatalf("ValidateAbsoluteHTTPURL should reject relative url")
	}
	if err := ValidateAbsoluteHTTPURL("ftp://example.com"); err == nil {
		t.Fatalf("ValidateAbsoluteHTTPURL should reject ftp scheme")
	}
	if err := ValidateAbsoluteHTTPURL("https://example.com/#frag"); err == nil {
		t.Fatalf("ValidateAbsoluteHTTPURL should reject fragment")
	}
}

func TestValidateServerFrontendURL(t *testing.T) {
	resetViperWithJWTSecret(t)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}

	cfg.Server.FrontendURL = "https://example.com"
	if err := cfg.Validate(); err != nil {
		t.Fatalf("Validate() frontend_url valid error: %v", err)
	}

	cfg.Server.FrontendURL = "https://example.com/path"
	if err := cfg.Validate(); err != nil {
		t.Fatalf("Validate() frontend_url with path valid error: %v", err)
	}

	cfg.Server.FrontendURL = "https://example.com?utm=1"
	if err := cfg.Validate(); err == nil {
		t.Fatalf("Validate() should reject server.frontend_url with query")
	}

	cfg.Server.FrontendURL = "https://user:pass@example.com"
	if err := cfg.Validate(); err == nil {
		t.Fatalf("Validate() should reject server.frontend_url with userinfo")
	}

	cfg.Server.FrontendURL = "/relative"
	if err := cfg.Validate(); err == nil {
		t.Fatalf("Validate() should reject relative server.frontend_url")
	}
}

func TestValidateFrontendRedirectURL(t *testing.T) {
	if err := ValidateFrontendRedirectURL("/auth/callback"); err != nil {
		t.Fatalf("ValidateFrontendRedirectURL relative error: %v", err)
	}
	if err := ValidateFrontendRedirectURL("https://example.com/auth"); err != nil {
		t.Fatalf("ValidateFrontendRedirectURL absolute error: %v", err)
	}
	if err := ValidateFrontendRedirectURL("example.com/path"); err == nil {
		t.Fatalf("ValidateFrontendRedirectURL should reject non-absolute url")
	}
	if err := ValidateFrontendRedirectURL("//evil.com"); err == nil {
		t.Fatalf("ValidateFrontendRedirectURL should reject // prefix")
	}
	if err := ValidateFrontendRedirectURL("javascript:alert(1)"); err == nil {
		t.Fatalf("ValidateFrontendRedirectURL should reject javascript scheme")
	}
}

func TestWarnIfInsecureURL(t *testing.T) {
	warnIfInsecureURL("test", "http://example.com")
	warnIfInsecureURL("test", "bad://url")
	warnIfInsecureURL("test", "://invalid")
}

func TestGenerateJWTSecretDefaultLength(t *testing.T) {
	secret, err := generateJWTSecret(0)
	if err != nil {
		t.Fatalf("generateJWTSecret error: %v", err)
	}
	if len(secret) == 0 {
		t.Fatalf("generateJWTSecret returned empty string")
	}
}

func TestValidateOpsCleanupScheduleRequired(t *testing.T) {
	resetViperWithJWTSecret(t)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	cfg.Ops.Cleanup.Enabled = true
	cfg.Ops.Cleanup.Schedule = ""
	err = cfg.Validate()
	if err == nil {
		t.Fatalf("Validate() expected error for ops.cleanup.schedule")
	}
	if !strings.Contains(err.Error(), "ops.cleanup.schedule") {
		t.Fatalf("Validate() expected ops.cleanup.schedule error, got: %v", err)
	}
}

func TestValidateConcurrencyPingInterval(t *testing.T) {
	resetViperWithJWTSecret(t)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	cfg.Concurrency.PingInterval = 3
	err = cfg.Validate()
	if err == nil {
		t.Fatalf("Validate() expected error for concurrency.ping_interval")
	}
	if !strings.Contains(err.Error(), "concurrency.ping_interval") {
		t.Fatalf("Validate() expected concurrency.ping_interval error, got: %v", err)
	}
}

func TestProvideConfig(t *testing.T) {
	resetViperWithJWTSecret(t)
	if _, err := ProvideConfig(); err != nil {
		t.Fatalf("ProvideConfig() error: %v", err)
	}
}

func TestValidateConfigWithLinuxDoEnabled(t *testing.T) {
	resetViperWithJWTSecret(t)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}

	cfg.Security.CSP.Enabled = true
	cfg.Security.CSP.Policy = "default-src 'self'"

	cfg.LinuxDo.Enabled = true
	cfg.LinuxDo.ClientID = "client"
	cfg.LinuxDo.ClientSecret = "secret"
	cfg.LinuxDo.AuthorizeURL = "https://example.com/oauth2/authorize"
	cfg.LinuxDo.TokenURL = "https://example.com/oauth2/token"
	cfg.LinuxDo.UserInfoURL = "https://example.com/oauth2/userinfo"
	cfg.LinuxDo.RedirectURL = "https://example.com/api/v1/auth/oauth/linuxdo/callback"
	cfg.LinuxDo.FrontendRedirectURL = "/auth/linuxdo/callback"
	cfg.LinuxDo.TokenAuthMethod = "client_secret_post"
	cfg.LinuxDo.UsePKCE = true

	if err := cfg.Validate(); err != nil {
		t.Fatalf("Validate() unexpected error: %v", err)
	}
}

func TestValidateJWTSecretStrength(t *testing.T) {
	if !isWeakJWTSecret("change-me-in-production") {
		t.Fatalf("isWeakJWTSecret should detect weak secret")
	}
	if isWeakJWTSecret("StrongSecretValue") {
		t.Fatalf("isWeakJWTSecret should accept strong secret")
	}
}

func TestGenerateJWTSecretWithLength(t *testing.T) {
	secret, err := generateJWTSecret(16)
	if err != nil {
		t.Fatalf("generateJWTSecret error: %v", err)
	}
	if len(secret) == 0 {
		t.Fatalf("generateJWTSecret returned empty string")
	}
}

func TestDatabaseDSNWithTimezone_WithPassword(t *testing.T) {
	d := &DatabaseConfig{
		Host:     "localhost",
		Port:     5432,
		User:     "u",
		Password: "p",
		DBName:   "db",
		SSLMode:  "prefer",
	}
	got := d.DSNWithTimezone("UTC")
	if !strings.Contains(got, "password=p") {
		t.Fatalf("DSNWithTimezone should include password: %q", got)
	}
	if !strings.Contains(got, "TimeZone=UTC") {
		t.Fatalf("DSNWithTimezone should include TimeZone=UTC: %q", got)
	}
}

func TestValidateAbsoluteHTTPURLMissingHost(t *testing.T) {
	if err := ValidateAbsoluteHTTPURL("https://"); err == nil {
		t.Fatalf("ValidateAbsoluteHTTPURL should reject missing host")
	}
}

func TestValidateFrontendRedirectURLInvalidChars(t *testing.T) {
	if err := ValidateFrontendRedirectURL("/auth/\ncallback"); err == nil {
		t.Fatalf("ValidateFrontendRedirectURL should reject invalid chars")
	}
	if err := ValidateFrontendRedirectURL("http://"); err == nil {
		t.Fatalf("ValidateFrontendRedirectURL should reject missing host")
	}
	if err := ValidateFrontendRedirectURL("mailto:user@example.com"); err == nil {
		t.Fatalf("ValidateFrontendRedirectURL should reject mailto")
	}
}

func TestWarnIfInsecureURLHTTPS(t *testing.T) {
	warnIfInsecureURL("secure", "https://example.com")
}

func TestValidateJWTSecret_UTF8Bytes(t *testing.T) {
	resetViperWithJWTSecret(t)
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}

	// 31 bytes (< 32) even though it's 31 characters.
	cfg.JWT.Secret = strings.Repeat("a", 31)
	err = cfg.Validate()
	if err == nil {
		t.Fatalf("Validate() should reject 31-byte secret")
	}
	if !strings.Contains(err.Error(), "at least 32 bytes") {
		t.Fatalf("Validate() error = %v", err)
	}

	// 32 bytes OK.
	cfg.JWT.Secret = strings.Repeat("a", 32)
	err = cfg.Validate()
	if err != nil {
		t.Fatalf("Validate() should accept 32-byte secret: %v", err)
	}
}

func TestValidateConfigErrors(t *testing.T) {
	buildValid := func(t *testing.T) *Config {
		t.Helper()
		resetViperWithJWTSecret(t)
		cfg, err := Load()
		if err != nil {
			t.Fatalf("Load() error: %v", err)
		}
		return cfg
	}

	cases := []struct {
		name    string
		mutate  func(*Config)
		wantErr string
	}{
		{
			name:    "jwt secret required",
			mutate:  func(c *Config) { c.JWT.Secret = "" },
			wantErr: "jwt.secret is required",
		},
		{
			name:    "jwt secret min bytes",
			mutate:  func(c *Config) { c.JWT.Secret = strings.Repeat("a", 31) },
			wantErr: "jwt.secret must be at least 32 bytes",
		},
		{
			name:    "subscription maintenance worker_count non-negative",
			mutate:  func(c *Config) { c.SubscriptionMaintenance.WorkerCount = -1 },
			wantErr: "subscription_maintenance.worker_count",
		},
		{
			name:    "subscription maintenance queue_size non-negative",
			mutate:  func(c *Config) { c.SubscriptionMaintenance.QueueSize = -1 },
			wantErr: "subscription_maintenance.queue_size",
		},
		{
			name:    "jwt expire hour positive",
			mutate:  func(c *Config) { c.JWT.ExpireHour = 0 },
			wantErr: "jwt.expire_hour must be positive",
		},
		{
			name:    "jwt expire hour max",
			mutate:  func(c *Config) { c.JWT.ExpireHour = 200 },
			wantErr: "jwt.expire_hour must be <= 168",
		},
		{
			name:    "jwt access token expire minutes non-negative",
			mutate:  func(c *Config) { c.JWT.AccessTokenExpireMinutes = -1 },
			wantErr: "jwt.access_token_expire_minutes must be non-negative",
		},
		{
			name:    "csp policy required",
			mutate:  func(c *Config) { c.Security.CSP.Enabled = true; c.Security.CSP.Policy = "" },
			wantErr: "security.csp.policy",
		},
		{
			name: "linuxdo client id required",
			mutate: func(c *Config) {
				c.LinuxDo.Enabled = true
				c.LinuxDo.UsePKCE = true
				c.LinuxDo.ClientID = ""
			},
			wantErr: "linuxdo_connect.client_id",
		},
		{
			name: "linuxdo token auth method",
			mutate: func(c *Config) {
				c.LinuxDo.Enabled = true
				c.LinuxDo.UsePKCE = true
				c.LinuxDo.ClientID = "client"
				c.LinuxDo.ClientSecret = "secret"
				c.LinuxDo.AuthorizeURL = "https://example.com/authorize"
				c.LinuxDo.TokenURL = "https://example.com/token"
				c.LinuxDo.UserInfoURL = "https://example.com/userinfo"
				c.LinuxDo.RedirectURL = "https://example.com/callback"
				c.LinuxDo.FrontendRedirectURL = "/auth/callback"
				c.LinuxDo.TokenAuthMethod = "invalid"
			},
			wantErr: "linuxdo_connect.token_auth_method",
		},
		{
			name:    "billing circuit breaker threshold",
			mutate:  func(c *Config) { c.Billing.CircuitBreaker.FailureThreshold = 0 },
			wantErr: "billing.circuit_breaker.failure_threshold",
		},
		{
			name:    "billing circuit breaker reset",
			mutate:  func(c *Config) { c.Billing.CircuitBreaker.ResetTimeoutSeconds = 0 },
			wantErr: "billing.circuit_breaker.reset_timeout_seconds",
		},
		{
			name:    "billing circuit breaker half open",
			mutate:  func(c *Config) { c.Billing.CircuitBreaker.HalfOpenRequests = 0 },
			wantErr: "billing.circuit_breaker.half_open_requests",
		},
		{
			name:    "database max open conns",
			mutate:  func(c *Config) { c.Database.MaxOpenConns = 0 },
			wantErr: "database.max_open_conns",
		},
		{
			name:    "database max lifetime",
			mutate:  func(c *Config) { c.Database.ConnMaxLifetimeMinutes = -1 },
			wantErr: "database.conn_max_lifetime_minutes",
		},
		{
			name:    "database idle exceeds open",
			mutate:  func(c *Config) { c.Database.MaxIdleConns = c.Database.MaxOpenConns + 1 },
			wantErr: "database.max_idle_conns cannot exceed",
		},
		{
			name:    "redis dial timeout",
			mutate:  func(c *Config) { c.Redis.DialTimeoutSeconds = 0 },
			wantErr: "redis.dial_timeout_seconds",
		},
		{
			name:    "redis read timeout",
			mutate:  func(c *Config) { c.Redis.ReadTimeoutSeconds = 0 },
			wantErr: "redis.read_timeout_seconds",
		},
		{
			name:    "redis write timeout",
			mutate:  func(c *Config) { c.Redis.WriteTimeoutSeconds = 0 },
			wantErr: "redis.write_timeout_seconds",
		},
		{
			name:    "redis pool size",
			mutate:  func(c *Config) { c.Redis.PoolSize = 0 },
			wantErr: "redis.pool_size",
		},
		{
			name:    "redis idle exceeds pool",
			mutate:  func(c *Config) { c.Redis.MinIdleConns = c.Redis.PoolSize + 1 },
			wantErr: "redis.min_idle_conns cannot exceed",
		},
		{
			name:    "dashboard cache disabled negative",
			mutate:  func(c *Config) { c.Dashboard.Enabled = false; c.Dashboard.StatsTTLSeconds = -1 },
			wantErr: "dashboard_cache.stats_ttl_seconds",
		},
		{
			name:    "dashboard cache fresh ttl positive",
			mutate:  func(c *Config) { c.Dashboard.Enabled = true; c.Dashboard.StatsFreshTTLSeconds = 0 },
			wantErr: "dashboard_cache.stats_fresh_ttl_seconds",
		},
		{
			name:    "dashboard aggregation enabled interval",
			mutate:  func(c *Config) { c.DashboardAgg.Enabled = true; c.DashboardAgg.IntervalSeconds = 0 },
			wantErr: "dashboard_aggregation.interval_seconds",
		},
		{
			name: "dashboard aggregation backfill positive",
			mutate: func(c *Config) {
				c.DashboardAgg.Enabled = true
				c.DashboardAgg.BackfillEnabled = true
				c.DashboardAgg.BackfillMaxDays = 0
			},
			wantErr: "dashboard_aggregation.backfill_max_days",
		},
		{
			name:    "dashboard aggregation retention",
			mutate:  func(c *Config) { c.DashboardAgg.Enabled = true; c.DashboardAgg.Retention.UsageLogsDays = 0 },
			wantErr: "dashboard_aggregation.retention.usage_logs_days",
		},
		{
			name: "dashboard aggregation dedup retention",
			mutate: func(c *Config) {
				c.DashboardAgg.Enabled = true
				c.DashboardAgg.Retention.UsageBillingDedupDays = 0
			},
			wantErr: "dashboard_aggregation.retention.usage_billing_dedup_days",
		},
		{
			name: "dashboard aggregation dedup retention smaller than usage logs",
			mutate: func(c *Config) {
				c.DashboardAgg.Enabled = true
				c.DashboardAgg.Retention.UsageLogsDays = 30
				c.DashboardAgg.Retention.UsageBillingDedupDays = 29
			},
			wantErr: "dashboard_aggregation.retention.usage_billing_dedup_days",
		},
		{
			name:    "dashboard aggregation disabled interval",
			mutate:  func(c *Config) { c.DashboardAgg.Enabled = false; c.DashboardAgg.IntervalSeconds = -1 },
			wantErr: "dashboard_aggregation.interval_seconds",
		},
		{
			name:    "usage cleanup max range",
			mutate:  func(c *Config) { c.UsageCleanup.Enabled = true; c.UsageCleanup.MaxRangeDays = 0 },
			wantErr: "usage_cleanup.max_range_days",
		},
		{
			name:    "usage cleanup worker interval",
			mutate:  func(c *Config) { c.UsageCleanup.Enabled = true; c.UsageCleanup.WorkerIntervalSeconds = 0 },
			wantErr: "usage_cleanup.worker_interval_seconds",
		},
		{
			name:    "usage cleanup batch size",
			mutate:  func(c *Config) { c.UsageCleanup.Enabled = true; c.UsageCleanup.BatchSize = 0 },
			wantErr: "usage_cleanup.batch_size",
		},
		{
			name:    "usage cleanup disabled negative",
			mutate:  func(c *Config) { c.UsageCleanup.Enabled = false; c.UsageCleanup.BatchSize = -1 },
			wantErr: "usage_cleanup.batch_size",
		},
		{
			name:    "gateway max body size",
			mutate:  func(c *Config) { c.Gateway.MaxBodySize = 0 },
			wantErr: "gateway.max_body_size",
		},
		{
			name:    "gateway max idle conns",
			mutate:  func(c *Config) { c.Gateway.MaxIdleConns = 0 },
			wantErr: "gateway.max_idle_conns",
		},
		{
			name:    "gateway max idle conns per host",
			mutate:  func(c *Config) { c.Gateway.MaxIdleConnsPerHost = 0 },
			wantErr: "gateway.max_idle_conns_per_host",
		},
		{
			name:    "gateway idle timeout",
			mutate:  func(c *Config) { c.Gateway.IdleConnTimeoutSeconds = 0 },
			wantErr: "gateway.idle_conn_timeout_seconds",
		},
		{
			name:    "gateway max upstream clients",
			mutate:  func(c *Config) { c.Gateway.MaxUpstreamClients = 0 },
			wantErr: "gateway.max_upstream_clients",
		},
		{
			name:    "gateway client idle ttl",
			mutate:  func(c *Config) { c.Gateway.ClientIdleTTLSeconds = 0 },
			wantErr: "gateway.client_idle_ttl_seconds",
		},
		{
			name:    "gateway concurrency slot ttl",
			mutate:  func(c *Config) { c.Gateway.ConcurrencySlotTTLMinutes = 0 },
			wantErr: "gateway.concurrency_slot_ttl_minutes",
		},
		{
			name:    "gateway max conns per host",
			mutate:  func(c *Config) { c.Gateway.MaxConnsPerHost = -1 },
			wantErr: "gateway.max_conns_per_host",
		},
		{
			name:    "gateway connection isolation",
			mutate:  func(c *Config) { c.Gateway.ConnectionPoolIsolation = "invalid" },
			wantErr: "gateway.connection_pool_isolation",
		},
		{
			name:    "gateway stream keepalive range",
			mutate:  func(c *Config) { c.Gateway.StreamKeepaliveInterval = 4 },
			wantErr: "gateway.stream_keepalive_interval",
		},
		{
			name:    "gateway openai ws oauth max conns factor",
			mutate:  func(c *Config) { c.Gateway.OpenAIWS.OAuthMaxConnsFactor = 0 },
			wantErr: "gateway.openai_ws.oauth_max_conns_factor",
		},
		{
			name:    "gateway openai ws apikey max conns factor",
			mutate:  func(c *Config) { c.Gateway.OpenAIWS.APIKeyMaxConnsFactor = 0 },
			wantErr: "gateway.openai_ws.apikey_max_conns_factor",
		},
		{
			name:    "gateway stream data interval range",
			mutate:  func(c *Config) { c.Gateway.StreamDataIntervalTimeout = 5 },
			wantErr: "gateway.stream_data_interval_timeout",
		},
		{
			name:    "gateway stream data interval negative",
			mutate:  func(c *Config) { c.Gateway.StreamDataIntervalTimeout = -1 },
			wantErr: "gateway.stream_data_interval_timeout must be non-negative",
		},
		{
			name:    "gateway image stream keepalive range",
			mutate:  func(c *Config) { c.Gateway.ImageStreamKeepaliveInterval = 4 },
			wantErr: "gateway.image_stream_keepalive_interval",
		},
		{
			name:    "gateway image stream keepalive negative",
			mutate:  func(c *Config) { c.Gateway.ImageStreamKeepaliveInterval = -1 },
			wantErr: "gateway.image_stream_keepalive_interval must be non-negative",
		},
		{
			name:    "gateway image stream data interval range",
			mutate:  func(c *Config) { c.Gateway.ImageStreamDataIntervalTimeout = 30 },
			wantErr: "gateway.image_stream_data_interval_timeout",
		},
		{
			name:    "gateway image stream data interval negative",
			mutate:  func(c *Config) { c.Gateway.ImageStreamDataIntervalTimeout = -1 },
			wantErr: "gateway.image_stream_data_interval_timeout must be non-negative",
		},
		{
			name:    "gateway image concurrency max negative",
			mutate:  func(c *Config) { c.Gateway.ImageConcurrency.MaxConcurrentRequests = -1 },
			wantErr: "gateway.image_concurrency.max_concurrent_requests must be non-negative",
		},
		{
			name:    "gateway image concurrency overflow mode invalid",
			mutate:  func(c *Config) { c.Gateway.ImageConcurrency.OverflowMode = "queue" },
			wantErr: "gateway.image_concurrency.overflow_mode",
		},
		{
			name:    "gateway image concurrency wait timeout negative",
			mutate:  func(c *Config) { c.Gateway.ImageConcurrency.WaitTimeoutSeconds = -1 },
			wantErr: "gateway.image_concurrency.wait_timeout_seconds must be non-negative",
		},
		{
			name:    "gateway image concurrency max waiting negative",
			mutate:  func(c *Config) { c.Gateway.ImageConcurrency.MaxWaitingRequests = -1 },
			wantErr: "gateway.image_concurrency.max_waiting_requests must be non-negative",
		},
		{
			name:    "gateway max line size",
			mutate:  func(c *Config) { c.Gateway.MaxLineSize = 1024 },
			wantErr: "gateway.max_line_size must be at least",
		},
		{
			name:    "gateway max line size negative",
			mutate:  func(c *Config) { c.Gateway.MaxLineSize = -1 },
			wantErr: "gateway.max_line_size must be non-negative",
		},
		{
			name:    "gateway usage record worker count",
			mutate:  func(c *Config) { c.Gateway.UsageRecord.WorkerCount = 0 },
			wantErr: "gateway.usage_record.worker_count",
		},
		{
			name:    "gateway usage record queue size",
			mutate:  func(c *Config) { c.Gateway.UsageRecord.QueueSize = 0 },
			wantErr: "gateway.usage_record.queue_size",
		},
		{
			name:    "gateway usage record timeout",
			mutate:  func(c *Config) { c.Gateway.UsageRecord.TaskTimeoutSeconds = 0 },
			wantErr: "gateway.usage_record.task_timeout_seconds",
		},
		{
			name:    "gateway usage record overflow policy",
			mutate:  func(c *Config) { c.Gateway.UsageRecord.OverflowPolicy = "invalid" },
			wantErr: "gateway.usage_record.overflow_policy",
		},
		{
			name:    "gateway usage record sample percent range",
			mutate:  func(c *Config) { c.Gateway.UsageRecord.OverflowSamplePercent = 101 },
			wantErr: "gateway.usage_record.overflow_sample_percent",
		},
		{
			name: "gateway usage record sample percent required for sample policy",
			mutate: func(c *Config) {
				c.Gateway.UsageRecord.OverflowPolicy = UsageRecordOverflowPolicySample
				c.Gateway.UsageRecord.OverflowSamplePercent = 0
			},
			wantErr: "gateway.usage_record.overflow_sample_percent must be positive",
		},
		{
			name: "gateway usage record auto scale max gte min",
			mutate: func(c *Config) {
				c.Gateway.UsageRecord.AutoScaleMinWorkers = 256
				c.Gateway.UsageRecord.AutoScaleMaxWorkers = 128
			},
			wantErr: "gateway.usage_record.auto_scale_max_workers",
		},
		{
			name: "gateway usage record worker in auto scale range",
			mutate: func(c *Config) {
				c.Gateway.UsageRecord.AutoScaleMinWorkers = 200
				c.Gateway.UsageRecord.AutoScaleMaxWorkers = 300
				c.Gateway.UsageRecord.WorkerCount = 128
			},
			wantErr: "gateway.usage_record.worker_count must be between auto_scale_min_workers and auto_scale_max_workers",
		},
		{
			name: "gateway usage record auto scale queue thresholds order",
			mutate: func(c *Config) {
				c.Gateway.UsageRecord.AutoScaleUpQueuePercent = 50
				c.Gateway.UsageRecord.AutoScaleDownQueuePercent = 50
			},
			wantErr: "gateway.usage_record.auto_scale_down_queue_percent must be less",
		},
		{
			name:    "gateway usage record auto scale up step",
			mutate:  func(c *Config) { c.Gateway.UsageRecord.AutoScaleUpStep = 0 },
			wantErr: "gateway.usage_record.auto_scale_up_step",
		},
		{
			name:    "gateway usage record auto scale interval",
			mutate:  func(c *Config) { c.Gateway.UsageRecord.AutoScaleCheckIntervalSeconds = 0 },
			wantErr: "gateway.usage_record.auto_scale_check_interval_seconds",
		},
		{
			name:    "gateway user group rate cache ttl",
			mutate:  func(c *Config) { c.Gateway.UserGroupRateCacheTTLSeconds = 0 },
			wantErr: "gateway.user_group_rate_cache_ttl_seconds",
		},
		{
			name:    "gateway models list cache ttl range",
			mutate:  func(c *Config) { c.Gateway.ModelsListCacheTTLSeconds = 31 },
			wantErr: "gateway.models_list_cache_ttl_seconds",
		},
		{
			name:    "gateway scheduling sticky waiting",
			mutate:  func(c *Config) { c.Gateway.Scheduling.StickySessionMaxWaiting = 0 },
			wantErr: "gateway.scheduling.sticky_session_max_waiting",
		},
		{
			name:    "gateway scheduling outbox poll",
			mutate:  func(c *Config) { c.Gateway.Scheduling.OutboxPollIntervalSeconds = 0 },
			wantErr: "gateway.scheduling.outbox_poll_interval_seconds",
		},
		{
			name:    "gateway scheduling outbox failures",
			mutate:  func(c *Config) { c.Gateway.Scheduling.OutboxLagRebuildFailures = 0 },
			wantErr: "gateway.scheduling.outbox_lag_rebuild_failures",
		},
		{
			name: "gateway outbox lag rebuild",
			mutate: func(c *Config) {
				c.Gateway.Scheduling.OutboxLagWarnSeconds = 10
				c.Gateway.Scheduling.OutboxLagRebuildSeconds = 5
			},
			wantErr: "gateway.scheduling.outbox_lag_rebuild_seconds",
		},
		{
			name:    "log level invalid",
			mutate:  func(c *Config) { c.Log.Level = "trace" },
			wantErr: "log.level",
		},
		{
			name:    "log format invalid",
			mutate:  func(c *Config) { c.Log.Format = "plain" },
			wantErr: "log.format",
		},
		{
			name: "log output disabled",
			mutate: func(c *Config) {
				c.Log.Output.ToStdout = false
				c.Log.Output.ToFile = false
			},
			wantErr: "log.output.to_stdout and log.output.to_file cannot both be false",
		},
		{
			name:    "log rotation size",
			mutate:  func(c *Config) { c.Log.Rotation.MaxSizeMB = 0 },
			wantErr: "log.rotation.max_size_mb",
		},
		{
			name: "log sampling enabled invalid",
			mutate: func(c *Config) {
				c.Log.Sampling.Enabled = true
				c.Log.Sampling.Initial = 0
			},
			wantErr: "log.sampling.initial",
		},
		{
			name:    "ops metrics collector ttl",
			mutate:  func(c *Config) { c.Ops.MetricsCollectorCache.TTL = -1 },
			wantErr: "ops.metrics_collector_cache.ttl",
		},
		{
			name:    "ops cleanup retention",
			mutate:  func(c *Config) { c.Ops.Cleanup.ErrorLogRetentionDays = -1 },
			wantErr: "ops.cleanup.error_log_retention_days",
		},
		{
			name:    "ops cleanup minute retention",
			mutate:  func(c *Config) { c.Ops.Cleanup.MinuteMetricsRetentionDays = -1 },
			wantErr: "ops.cleanup.minute_metrics_retention_days",
		},
	}

	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			cfg := buildValid(t)
			tt.mutate(cfg)
			err := cfg.Validate()
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("Validate() error = %v, want %q", err, tt.wantErr)
			}
		})
	}
}

func TestValidateConfig_OpenAIWSRules(t *testing.T) {
	buildValid := func(t *testing.T) *Config {
		t.Helper()
		resetViperWithJWTSecret(t)
		cfg, err := Load()
		require.NoError(t, err)
		return cfg
	}

	t.Run("sticky response id ttl 兼容旧键回填", func(t *testing.T) {
		cfg := buildValid(t)
		cfg.Gateway.OpenAIWS.StickyResponseIDTTLSeconds = 0
		cfg.Gateway.OpenAIWS.StickyPreviousResponseTTLSeconds = 7200

		require.NoError(t, cfg.Validate())
		require.Equal(t, 7200, cfg.Gateway.OpenAIWS.StickyResponseIDTTLSeconds)
	})

	cases := []struct {
		name    string
		mutate  func(*Config)
		wantErr string
	}{
		{
			name:    "max_conns_per_account 必须为正数",
			mutate:  func(c *Config) { c.Gateway.OpenAIWS.MaxConnsPerAccount = 0 },
			wantErr: "gateway.openai_ws.max_conns_per_account",
		},
		{
			name:    "min_idle_per_account 不能为负数",
			mutate:  func(c *Config) { c.Gateway.OpenAIWS.MinIdlePerAccount = -1 },
			wantErr: "gateway.openai_ws.min_idle_per_account",
		},
		{
			name:    "max_idle_per_account 不能为负数",
			mutate:  func(c *Config) { c.Gateway.OpenAIWS.MaxIdlePerAccount = -1 },
			wantErr: "gateway.openai_ws.max_idle_per_account",
		},
		{
			name: "min_idle_per_account 不能大于 max_idle_per_account",
			mutate: func(c *Config) {
				c.Gateway.OpenAIWS.MinIdlePerAccount = 3
				c.Gateway.OpenAIWS.MaxIdlePerAccount = 2
			},
			wantErr: "gateway.openai_ws.min_idle_per_account must be <= max_idle_per_account",
		},
		{
			name: "max_idle_per_account 不能大于 max_conns_per_account",
			mutate: func(c *Config) {
				c.Gateway.OpenAIWS.MaxConnsPerAccount = 2
				c.Gateway.OpenAIWS.MinIdlePerAccount = 1
				c.Gateway.OpenAIWS.MaxIdlePerAccount = 3
			},
			wantErr: "gateway.openai_ws.max_idle_per_account must be <= max_conns_per_account",
		},
		{
			name:    "dial_timeout_seconds 必须为正数",
			mutate:  func(c *Config) { c.Gateway.OpenAIWS.DialTimeoutSeconds = 0 },
			wantErr: "gateway.openai_ws.dial_timeout_seconds",
		},
		{
			name:    "read_timeout_seconds 必须为正数",
			mutate:  func(c *Config) { c.Gateway.OpenAIWS.ReadTimeoutSeconds = 0 },
			wantErr: "gateway.openai_ws.read_timeout_seconds",
		},
		{
			name:    "write_timeout_seconds 必须为正数",
			mutate:  func(c *Config) { c.Gateway.OpenAIWS.WriteTimeoutSeconds = 0 },
			wantErr: "gateway.openai_ws.write_timeout_seconds",
		},
		{
			name:    "pool_target_utilization 必须在 (0,1]",
			mutate:  func(c *Config) { c.Gateway.OpenAIWS.PoolTargetUtilization = 0 },
			wantErr: "gateway.openai_ws.pool_target_utilization",
		},
		{
			name:    "queue_limit_per_conn 必须为正数",
			mutate:  func(c *Config) { c.Gateway.OpenAIWS.QueueLimitPerConn = 0 },
			wantErr: "gateway.openai_ws.queue_limit_per_conn",
		},
		{
			name:    "fallback_cooldown_seconds 不能为负数",
			mutate:  func(c *Config) { c.Gateway.OpenAIWS.FallbackCooldownSeconds = -1 },
			wantErr: "gateway.openai_ws.fallback_cooldown_seconds",
		},
		{
			name:    "store_disabled_conn_mode 必须为 strict|adaptive|off",
			mutate:  func(c *Config) { c.Gateway.OpenAIWS.StoreDisabledConnMode = "invalid" },
			wantErr: "gateway.openai_ws.store_disabled_conn_mode",
		},
		{
			name:    "ingress_mode_default 必须为 off|ctx_pool|passthrough",
			mutate:  func(c *Config) { c.Gateway.OpenAIWS.IngressModeDefault = "invalid" },
			wantErr: "gateway.openai_ws.ingress_mode_default",
		},
		{
			name:    "payload_log_sample_rate 必须在 [0,1] 范围内",
			mutate:  func(c *Config) { c.Gateway.OpenAIWS.PayloadLogSampleRate = 1.2 },
			wantErr: "gateway.openai_ws.payload_log_sample_rate",
		},
		{
			name:    "retry_total_budget_ms 不能为负数",
			mutate:  func(c *Config) { c.Gateway.OpenAIWS.RetryTotalBudgetMS = -1 },
			wantErr: "gateway.openai_ws.retry_total_budget_ms",
		},
		{
			name:    "lb_top_k 必须为正数",
			mutate:  func(c *Config) { c.Gateway.OpenAIWS.LBTopK = 0 },
			wantErr: "gateway.openai_ws.lb_top_k",
		},
		{
			name:    "sticky_session_ttl_seconds 必须为正数",
			mutate:  func(c *Config) { c.Gateway.OpenAIWS.StickySessionTTLSeconds = 0 },
			wantErr: "gateway.openai_ws.sticky_session_ttl_seconds",
		},
		{
			name: "sticky_response_id_ttl_seconds 必须为正数",
			mutate: func(c *Config) {
				c.Gateway.OpenAIWS.StickyResponseIDTTLSeconds = 0
				c.Gateway.OpenAIWS.StickyPreviousResponseTTLSeconds = 0
			},
			wantErr: "gateway.openai_ws.sticky_response_id_ttl_seconds",
		},
		{
			name:    "sticky_previous_response_ttl_seconds 不能为负数",
			mutate:  func(c *Config) { c.Gateway.OpenAIWS.StickyPreviousResponseTTLSeconds = -1 },
			wantErr: "gateway.openai_ws.sticky_previous_response_ttl_seconds",
		},
		{
			name:    "scheduler_score_weights 不能为负数",
			mutate:  func(c *Config) { c.Gateway.OpenAIWS.SchedulerScoreWeights.Queue = -0.1 },
			wantErr: "gateway.openai_ws.scheduler_score_weights.* must be non-negative",
		},
		{
			name: "scheduler_score_weights 不能全为 0",
			mutate: func(c *Config) {
				c.Gateway.OpenAIWS.SchedulerScoreWeights.Priority = 0
				c.Gateway.OpenAIWS.SchedulerScoreWeights.Load = 0
				c.Gateway.OpenAIWS.SchedulerScoreWeights.Queue = 0
				c.Gateway.OpenAIWS.SchedulerScoreWeights.ErrorRate = 0
				c.Gateway.OpenAIWS.SchedulerScoreWeights.TTFT = 0
			},
			wantErr: "gateway.openai_ws.scheduler_score_weights must not all be zero",
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			cfg := buildValid(t)
			tc.mutate(cfg)

			err := cfg.Validate()
			require.Error(t, err)
			require.Contains(t, err.Error(), tc.wantErr)
		})
	}
}

func TestValidateConfig_AutoScaleDisabledIgnoreAutoScaleFields(t *testing.T) {
	resetViperWithJWTSecret(t)
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}

	cfg.Gateway.UsageRecord.AutoScaleEnabled = false
	cfg.Gateway.UsageRecord.WorkerCount = 64

	// 自动扩缩容关闭时，这些字段应被忽略，不应导致校验失败。
	cfg.Gateway.UsageRecord.AutoScaleMinWorkers = 0
	cfg.Gateway.UsageRecord.AutoScaleMaxWorkers = 0
	cfg.Gateway.UsageRecord.AutoScaleUpQueuePercent = 0
	cfg.Gateway.UsageRecord.AutoScaleDownQueuePercent = 100
	cfg.Gateway.UsageRecord.AutoScaleUpStep = 0
	cfg.Gateway.UsageRecord.AutoScaleDownStep = 0
	cfg.Gateway.UsageRecord.AutoScaleCheckIntervalSeconds = 0
	cfg.Gateway.UsageRecord.AutoScaleCooldownSeconds = -1

	if err := cfg.Validate(); err != nil {
		t.Fatalf("Validate() should ignore auto scale fields when disabled: %v", err)
	}
}

func TestValidateConfig_LogRequiredAndRotationBounds(t *testing.T) {
	resetViperWithJWTSecret(t)

	cases := []struct {
		name    string
		mutate  func(*Config)
		wantErr string
	}{
		{
			name: "log level required",
			mutate: func(c *Config) {
				c.Log.Level = ""
			},
			wantErr: "log.level is required",
		},
		{
			name: "log format required",
			mutate: func(c *Config) {
				c.Log.Format = ""
			},
			wantErr: "log.format is required",
		},
		{
			name: "log stacktrace required",
			mutate: func(c *Config) {
				c.Log.StacktraceLevel = ""
			},
			wantErr: "log.stacktrace_level is required",
		},
		{
			name: "log max backups non-negative",
			mutate: func(c *Config) {
				c.Log.Rotation.MaxBackups = -1
			},
			wantErr: "log.rotation.max_backups must be non-negative",
		},
		{
			name: "log max age non-negative",
			mutate: func(c *Config) {
				c.Log.Rotation.MaxAgeDays = -1
			},
			wantErr: "log.rotation.max_age_days must be non-negative",
		},
		{
			name: "sampling thereafter non-negative when disabled",
			mutate: func(c *Config) {
				c.Log.Sampling.Enabled = false
				c.Log.Sampling.Thereafter = -1
			},
			wantErr: "log.sampling.thereafter must be non-negative",
		},
	}

	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			cfg, err := Load()
			if err != nil {
				t.Fatalf("Load() error: %v", err)
			}
			tt.mutate(cfg)
			err = cfg.Validate()
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("Validate() error = %v, want %q", err, tt.wantErr)
			}
		})
	}
}

func TestLoad_DefaultGatewayUsageRecordConfig(t *testing.T) {
	resetViperWithJWTSecret(t)
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	if cfg.Gateway.UsageRecord.WorkerCount != 128 {
		t.Fatalf("worker_count = %d, want 128", cfg.Gateway.UsageRecord.WorkerCount)
	}
	if cfg.Gateway.UsageRecord.QueueSize != 16384 {
		t.Fatalf("queue_size = %d, want 16384", cfg.Gateway.UsageRecord.QueueSize)
	}
	if cfg.Gateway.UsageRecord.TaskTimeoutSeconds != 5 {
		t.Fatalf("task_timeout_seconds = %d, want 5", cfg.Gateway.UsageRecord.TaskTimeoutSeconds)
	}
	if cfg.Gateway.UsageRecord.OverflowPolicy != UsageRecordOverflowPolicySample {
		t.Fatalf("overflow_policy = %s, want %s", cfg.Gateway.UsageRecord.OverflowPolicy, UsageRecordOverflowPolicySample)
	}
	if cfg.Gateway.UsageRecord.OverflowSamplePercent != 10 {
		t.Fatalf("overflow_sample_percent = %d, want 10", cfg.Gateway.UsageRecord.OverflowSamplePercent)
	}
	if !cfg.Gateway.UsageRecord.AutoScaleEnabled {
		t.Fatalf("auto_scale_enabled = false, want true")
	}
	if cfg.Gateway.UsageRecord.AutoScaleMinWorkers != 128 {
		t.Fatalf("auto_scale_min_workers = %d, want 128", cfg.Gateway.UsageRecord.AutoScaleMinWorkers)
	}
	if cfg.Gateway.UsageRecord.AutoScaleMaxWorkers != 512 {
		t.Fatalf("auto_scale_max_workers = %d, want 512", cfg.Gateway.UsageRecord.AutoScaleMaxWorkers)
	}
	if cfg.Gateway.UsageRecord.AutoScaleUpQueuePercent != 70 {
		t.Fatalf("auto_scale_up_queue_percent = %d, want 70", cfg.Gateway.UsageRecord.AutoScaleUpQueuePercent)
	}
	if cfg.Gateway.UsageRecord.AutoScaleDownQueuePercent != 15 {
		t.Fatalf("auto_scale_down_queue_percent = %d, want 15", cfg.Gateway.UsageRecord.AutoScaleDownQueuePercent)
	}
	if cfg.Gateway.UsageRecord.AutoScaleUpStep != 32 {
		t.Fatalf("auto_scale_up_step = %d, want 32", cfg.Gateway.UsageRecord.AutoScaleUpStep)
	}
	if cfg.Gateway.UsageRecord.AutoScaleDownStep != 16 {
		t.Fatalf("auto_scale_down_step = %d, want 16", cfg.Gateway.UsageRecord.AutoScaleDownStep)
	}
	if cfg.Gateway.UsageRecord.AutoScaleCheckIntervalSeconds != 3 {
		t.Fatalf("auto_scale_check_interval_seconds = %d, want 3", cfg.Gateway.UsageRecord.AutoScaleCheckIntervalSeconds)
	}
	if cfg.Gateway.UsageRecord.AutoScaleCooldownSeconds != 10 {
		t.Fatalf("auto_scale_cooldown_seconds = %d, want 10", cfg.Gateway.UsageRecord.AutoScaleCooldownSeconds)
	}
}

func TestLoad_DefaultGatewayImageStreamConfig(t *testing.T) {
	resetViperWithJWTSecret(t)
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	if cfg.Gateway.StreamDataIntervalTimeout != 180 {
		t.Fatalf("stream_data_interval_timeout = %d, want 180", cfg.Gateway.StreamDataIntervalTimeout)
	}
	if cfg.Gateway.StreamKeepaliveInterval != 10 {
		t.Fatalf("stream_keepalive_interval = %d, want 10", cfg.Gateway.StreamKeepaliveInterval)
	}
	if cfg.Gateway.ImageStreamDataIntervalTimeout != 900 {
		t.Fatalf("image_stream_data_interval_timeout = %d, want 900", cfg.Gateway.ImageStreamDataIntervalTimeout)
	}
	if cfg.Gateway.ImageStreamKeepaliveInterval != 10 {
		t.Fatalf("image_stream_keepalive_interval = %d, want 10", cfg.Gateway.ImageStreamKeepaliveInterval)
	}
	if cfg.Gateway.ImageConcurrency.Enabled {
		t.Fatalf("image_concurrency.enabled = true, want false")
	}
	if cfg.Gateway.ImageConcurrency.MaxConcurrentRequests != 0 {
		t.Fatalf("image_concurrency.max_concurrent_requests = %d, want 0", cfg.Gateway.ImageConcurrency.MaxConcurrentRequests)
	}
	if cfg.Gateway.ImageConcurrency.OverflowMode != ImageConcurrencyOverflowModeReject {
		t.Fatalf("image_concurrency.overflow_mode = %q, want %q", cfg.Gateway.ImageConcurrency.OverflowMode, ImageConcurrencyOverflowModeReject)
	}
	if cfg.Gateway.ImageConcurrency.WaitTimeoutSeconds != 30 {
		t.Fatalf("image_concurrency.wait_timeout_seconds = %d, want 30", cfg.Gateway.ImageConcurrency.WaitTimeoutSeconds)
	}
	if cfg.Gateway.ImageConcurrency.MaxWaitingRequests != 100 {
		t.Fatalf("image_concurrency.max_waiting_requests = %d, want 100", cfg.Gateway.ImageConcurrency.MaxWaitingRequests)
	}
	if cfg.Gateway.ImageStreamDataIntervalTimeout <= cfg.Gateway.StreamDataIntervalTimeout {
		t.Fatalf("image stream timeout = %d, want greater than ordinary stream timeout %d", cfg.Gateway.ImageStreamDataIntervalTimeout, cfg.Gateway.StreamDataIntervalTimeout)
	}
}
