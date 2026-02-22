package config

import (
	"strings"
	"testing"
	"time"

	"github.com/spf13/viper"
)

func resetViperWithJWTSecret(t *testing.T) {
REDACTED
	viper.Reset()
	t.Setenv("JWT_SECRET", strings.Repeat("x", 32))
REDACTED

func TestLoadForBootstrapAllowsMissingJWTSecret(t *testing.T) {
	viper.Reset()
	t.Setenv("JWT_SECRET", "")

	cfg, err := LoadForBootstrap()
	if err != nil {
		t.Fatalf("LoadForBootstrap() error: %v", err)
REDACTED
	if cfg.JWT.Secret != "" {
		t.Fatalf("LoadForBootstrap() should keep empty jwt.secret during bootstrap")
REDACTED
REDACTED

func TestNormalizeRunMode(t *testing.T) {
	tests := []struct {
		input    string
		expected string
REDACTED{
		{"simple", "simple"REDACTED,
		{"SIMPLE", "simple"REDACTED,
		{"standard", "standard"REDACTED,
		{"invalid", "standard"REDACTED,
		{"", "standard"REDACTED,
REDACTED

	for _, tt := range tests {
		result := NormalizeRunMode(tt.input)
		if result != tt.expected {
			t.Errorf("NormalizeRunMode(%q) = %q, want %q", tt.input, result, tt.expected)
	REDACTED
REDACTED
REDACTED

func TestLoadDefaultSchedulingConfig(t *testing.T) {
	resetViperWithJWTSecret(t)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error: %v", err)
REDACTED

	if cfg.Gateway.Scheduling.StickySessionMaxWaiting != 3 {
		t.Fatalf("StickySessionMaxWaiting = %d, want 3", cfg.Gateway.Scheduling.StickySessionMaxWaiting)
REDACTED
	if cfg.Gateway.Scheduling.StickySessionWaitTimeout != 120*time.Second {
		t.Fatalf("StickySessionWaitTimeout = %v, want 120s", cfg.Gateway.Scheduling.StickySessionWaitTimeout)
REDACTED
	if cfg.Gateway.Scheduling.FallbackWaitTimeout != 30*time.Second {
		t.Fatalf("FallbackWaitTimeout = %v, want 30s", cfg.Gateway.Scheduling.FallbackWaitTimeout)
REDACTED
	if cfg.Gateway.Scheduling.FallbackMaxWaiting != 100 {
		t.Fatalf("FallbackMaxWaiting = %d, want 100", cfg.Gateway.Scheduling.FallbackMaxWaiting)
REDACTED
	if !cfg.Gateway.Scheduling.LoadBatchEnabled {
		t.Fatalf("LoadBatchEnabled = false, want true")
REDACTED
	if cfg.Gateway.Scheduling.SlotCleanupInterval != 30*time.Second {
		t.Fatalf("SlotCleanupInterval = %v, want 30s", cfg.Gateway.Scheduling.SlotCleanupInterval)
REDACTED
REDACTED

func TestLoadSchedulingConfigFromEnv(t *testing.T) {
	resetViperWithJWTSecret(t)
	t.Setenv("GATEWAY_SCHEDULING_STICKY_SESSION_MAX_WAITING", "5")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error: %v", err)
REDACTED

	if cfg.Gateway.Scheduling.StickySessionMaxWaiting != 5 {
		t.Fatalf("StickySessionMaxWaiting = %d, want 5", cfg.Gateway.Scheduling.StickySessionMaxWaiting)
REDACTED
REDACTED

func TestLoadDefaultSecurityToggles(t *testing.T) {
	resetViperWithJWTSecret(t)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error: %v", err)
REDACTED

	if cfg.Security.URLAllowlist.Enabled {
		t.Fatalf("URLAllowlist.Enabled = true, want false")
REDACTED
	if !cfg.Security.URLAllowlist.AllowInsecureHTTP {
		t.Fatalf("URLAllowlist.AllowInsecureHTTP = false, want true")
REDACTED
	if !cfg.Security.URLAllowlist.AllowPrivateHosts {
		t.Fatalf("URLAllowlist.AllowPrivateHosts = false, want true")
REDACTED
	if !cfg.Security.ResponseHeaders.Enabled {
		t.Fatalf("ResponseHeaders.Enabled = false, want true")
REDACTED
REDACTED

func TestLoadDefaultServerMode(t *testing.T) {
	resetViperWithJWTSecret(t)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error: %v", err)
REDACTED

	if cfg.Server.Mode != "release" {
		t.Fatalf("Server.Mode = %q, want %q", cfg.Server.Mode, "release")
REDACTED
REDACTED

func TestLoadDefaultDatabaseSSLMode(t *testing.T) {
	resetViperWithJWTSecret(t)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error: %v", err)
REDACTED

	if cfg.Database.SSLMode != "prefer" {
		t.Fatalf("Database.SSLMode = %q, want %q", cfg.Database.SSLMode, "prefer")
REDACTED
REDACTED

func TestValidateLinuxDoFrontendRedirectURL(t *testing.T) {
	resetViperWithJWTSecret(t)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error: %v", err)
REDACTED

	cfg.LinuxDo.Enabled = true
	cfg.LinuxDo.ClientID = "test-client"
	cfg.LinuxDo.ClientSecret = "test-secret"
	cfg.LinuxDo.RedirectURL = "https://example.com/api/v1/auth/oauth/linuxdo/callback"
	cfg.LinuxDo.TokenAuthMethod = "client_secret_post"
	cfg.LinuxDo.UsePKCE = false

	cfg.LinuxDo.FrontendRedirectURL = "javascript:alert(1)"
	err = cfg.Validate()
	if err == nil {
		t.Fatalf("Validate() expected error for javascript scheme, got nil")
REDACTED
	if !strings.Contains(err.Error(), "linuxdo_connect.frontend_redirect_url") {
		t.Fatalf("Validate() expected frontend_redirect_url error, got: %v", err)
REDACTED
REDACTED

func TestValidateLinuxDoPKCERequiredForPublicClient(t *testing.T) {
	resetViperWithJWTSecret(t)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error: %v", err)
REDACTED

	cfg.LinuxDo.Enabled = true
	cfg.LinuxDo.ClientID = "test-client"
	cfg.LinuxDo.ClientSecret = ""
	cfg.LinuxDo.RedirectURL = "https://example.com/api/v1/auth/oauth/linuxdo/callback"
	cfg.LinuxDo.FrontendRedirectURL = "/auth/linuxdo/callback"
	cfg.LinuxDo.TokenAuthMethod = "none"
	cfg.LinuxDo.UsePKCE = false

	err = cfg.Validate()
	if err == nil {
		t.Fatalf("Validate() expected error when token_auth_method=none and use_pkce=false, got nil")
REDACTED
	if !strings.Contains(err.Error(), "linuxdo_connect.use_pkce") {
		t.Fatalf("Validate() expected use_pkce error, got: %v", err)
REDACTED
REDACTED

func TestLoadDefaultDashboardCacheConfig(t *testing.T) {
	resetViperWithJWTSecret(t)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error: %v", err)
REDACTED

	if !cfg.Dashboard.Enabled {
		t.Fatalf("Dashboard.Enabled = false, want true")
REDACTED
	if cfg.Dashboard.KeyPrefix != "sub2api:" {
		t.Fatalf("Dashboard.KeyPrefix = %q, want %q", cfg.Dashboard.KeyPrefix, "sub2api:")
REDACTED
	if cfg.Dashboard.StatsFreshTTLSeconds != 15 {
		t.Fatalf("Dashboard.StatsFreshTTLSeconds = %d, want 15", cfg.Dashboard.StatsFreshTTLSeconds)
REDACTED
	if cfg.Dashboard.StatsTTLSeconds != 30 {
		t.Fatalf("Dashboard.StatsTTLSeconds = %d, want 30", cfg.Dashboard.StatsTTLSeconds)
REDACTED
	if cfg.Dashboard.StatsRefreshTimeoutSeconds != 30 {
		t.Fatalf("Dashboard.StatsRefreshTimeoutSeconds = %d, want 30", cfg.Dashboard.StatsRefreshTimeoutSeconds)
REDACTED
REDACTED

func TestValidateDashboardCacheConfigEnabled(t *testing.T) {
	resetViperWithJWTSecret(t)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error: %v", err)
REDACTED

	cfg.Dashboard.Enabled = true
	cfg.Dashboard.StatsFreshTTLSeconds = 10
	cfg.Dashboard.StatsTTLSeconds = 5
	err = cfg.Validate()
	if err == nil {
		t.Fatalf("Validate() expected error for stats_fresh_ttl_seconds > stats_ttl_seconds, got nil")
REDACTED
	if !strings.Contains(err.Error(), "dashboard_cache.stats_fresh_ttl_seconds") {
		t.Fatalf("Validate() expected stats_fresh_ttl_seconds error, got: %v", err)
REDACTED
REDACTED

func TestValidateDashboardCacheConfigDisabled(t *testing.T) {
	resetViperWithJWTSecret(t)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error: %v", err)
REDACTED

	cfg.Dashboard.Enabled = false
	cfg.Dashboard.StatsTTLSeconds = -1
	err = cfg.Validate()
	if err == nil {
		t.Fatalf("Validate() expected error for negative stats_ttl_seconds, got nil")
REDACTED
	if !strings.Contains(err.Error(), "dashboard_cache.stats_ttl_seconds") {
		t.Fatalf("Validate() expected stats_ttl_seconds error, got: %v", err)
REDACTED
REDACTED

func TestLoadDefaultDashboardAggregationConfig(t *testing.T) {
	resetViperWithJWTSecret(t)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error: %v", err)
REDACTED

	if !cfg.DashboardAgg.Enabled {
		t.Fatalf("DashboardAgg.Enabled = false, want true")
REDACTED
	if cfg.DashboardAgg.IntervalSeconds != 60 {
		t.Fatalf("DashboardAgg.IntervalSeconds = %d, want 60", cfg.DashboardAgg.IntervalSeconds)
REDACTED
	if cfg.DashboardAgg.LookbackSeconds != 120 {
		t.Fatalf("DashboardAgg.LookbackSeconds = %d, want 120", cfg.DashboardAgg.LookbackSeconds)
REDACTED
	if cfg.DashboardAgg.BackfillEnabled {
		t.Fatalf("DashboardAgg.BackfillEnabled = true, want false")
REDACTED
	if cfg.DashboardAgg.BackfillMaxDays != 31 {
		t.Fatalf("DashboardAgg.BackfillMaxDays = %d, want 31", cfg.DashboardAgg.BackfillMaxDays)
REDACTED
	if cfg.DashboardAgg.Retention.UsageLogsDays != 90 {
		t.Fatalf("DashboardAgg.Retention.UsageLogsDays = %d, want 90", cfg.DashboardAgg.Retention.UsageLogsDays)
REDACTED
	if cfg.DashboardAgg.Retention.HourlyDays != 180 {
		t.Fatalf("DashboardAgg.Retention.HourlyDays = %d, want 180", cfg.DashboardAgg.Retention.HourlyDays)
REDACTED
	if cfg.DashboardAgg.Retention.DailyDays != 730 {
		t.Fatalf("DashboardAgg.Retention.DailyDays = %d, want 730", cfg.DashboardAgg.Retention.DailyDays)
REDACTED
	if cfg.DashboardAgg.RecomputeDays != 2 {
		t.Fatalf("DashboardAgg.RecomputeDays = %d, want 2", cfg.DashboardAgg.RecomputeDays)
REDACTED
REDACTED

func TestValidateDashboardAggregationConfigDisabled(t *testing.T) {
	resetViperWithJWTSecret(t)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error: %v", err)
REDACTED

	cfg.DashboardAgg.Enabled = false
	cfg.DashboardAgg.IntervalSeconds = -1
	err = cfg.Validate()
	if err == nil {
		t.Fatalf("Validate() expected error for negative dashboard_aggregation.interval_seconds, got nil")
REDACTED
	if !strings.Contains(err.Error(), "dashboard_aggregation.interval_seconds") {
		t.Fatalf("Validate() expected interval_seconds error, got: %v", err)
REDACTED
REDACTED

func TestValidateDashboardAggregationBackfillMaxDays(t *testing.T) {
	resetViperWithJWTSecret(t)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error: %v", err)
REDACTED

	cfg.DashboardAgg.BackfillEnabled = true
	cfg.DashboardAgg.BackfillMaxDays = 0
	err = cfg.Validate()
	if err == nil {
		t.Fatalf("Validate() expected error for dashboard_aggregation.backfill_max_days, got nil")
REDACTED
	if !strings.Contains(err.Error(), "dashboard_aggregation.backfill_max_days") {
		t.Fatalf("Validate() expected backfill_max_days error, got: %v", err)
REDACTED
REDACTED

func TestLoadDefaultUsageCleanupConfig(t *testing.T) {
	resetViperWithJWTSecret(t)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error: %v", err)
REDACTED

	if !cfg.UsageCleanup.Enabled {
		t.Fatalf("UsageCleanup.Enabled = false, want true")
REDACTED
	if cfg.UsageCleanup.MaxRangeDays != 31 {
		t.Fatalf("UsageCleanup.MaxRangeDays = %d, want 31", cfg.UsageCleanup.MaxRangeDays)
REDACTED
	if cfg.UsageCleanup.BatchSize != 5000 {
		t.Fatalf("UsageCleanup.BatchSize = %d, want 5000", cfg.UsageCleanup.BatchSize)
REDACTED
	if cfg.UsageCleanup.WorkerIntervalSeconds != 10 {
		t.Fatalf("UsageCleanup.WorkerIntervalSeconds = %d, want 10", cfg.UsageCleanup.WorkerIntervalSeconds)
REDACTED
	if cfg.UsageCleanup.TaskTimeoutSeconds != 1800 {
		t.Fatalf("UsageCleanup.TaskTimeoutSeconds = %d, want 1800", cfg.UsageCleanup.TaskTimeoutSeconds)
REDACTED
REDACTED

func TestValidateUsageCleanupConfigEnabled(t *testing.T) {
	resetViperWithJWTSecret(t)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error: %v", err)
REDACTED

	cfg.UsageCleanup.Enabled = true
	cfg.UsageCleanup.MaxRangeDays = 0
	err = cfg.Validate()
	if err == nil {
		t.Fatalf("Validate() expected error for usage_cleanup.max_range_days, got nil")
REDACTED
	if !strings.Contains(err.Error(), "usage_cleanup.max_range_days") {
		t.Fatalf("Validate() expected max_range_days error, got: %v", err)
REDACTED
REDACTED

func TestValidateUsageCleanupConfigDisabled(t *testing.T) {
	resetViperWithJWTSecret(t)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error: %v", err)
REDACTED

	cfg.UsageCleanup.Enabled = false
	cfg.UsageCleanup.BatchSize = -1
	err = cfg.Validate()
	if err == nil {
		t.Fatalf("Validate() expected error for usage_cleanup.batch_size, got nil")
REDACTED
	if !strings.Contains(err.Error(), "usage_cleanup.batch_size") {
		t.Fatalf("Validate() expected batch_size error, got: %v", err)
REDACTED
REDACTED

func TestConfigAddressHelpers(t *testing.T) {
	server := ServerConfig{Host: "127.0.0.1", Port: 9000REDACTED
	if server.Address() != "127.0.0.1:9000" {
		t.Fatalf("ServerConfig.Address() = %q", server.Address())
REDACTED

	dbCfg := DatabaseConfig{
		Host:     "localhost",
		Port:     5432,
		User:     "postgres",
		Password: "",
		DBName:   "sub2api",
		SSLMode:  "disable",
REDACTED
	if !strings.Contains(dbCfg.DSN(), "password=") {
REDACTED else {
		t.Fatalf("DatabaseConfig.DSN() should not include password when empty")
REDACTED

	dbCfg.Password = "secret"
	if !strings.Contains(dbCfg.DSN(), "password=secret") {
		t.Fatalf("DatabaseConfig.DSN() missing password")
REDACTED

	dbCfg.Password = ""
	if strings.Contains(dbCfg.DSNWithTimezone("UTC"), "password=") {
		t.Fatalf("DatabaseConfig.DSNWithTimezone() should omit password when empty")
REDACTED

	if !strings.Contains(dbCfg.DSNWithTimezone(""), "TimeZone=Asia/Shanghai") {
		t.Fatalf("DatabaseConfig.DSNWithTimezone() should use default timezone")
REDACTED
	if !strings.Contains(dbCfg.DSNWithTimezone("UTC"), "TimeZone=UTC") {
		t.Fatalf("DatabaseConfig.DSNWithTimezone() should use provided timezone")
REDACTED

	redis := RedisConfig{Host: "redis", Port: 6379REDACTED
	if redis.Address() != "redis:6379" {
		t.Fatalf("RedisConfig.Address() = %q", redis.Address())
REDACTED
REDACTED

func TestNormalizeStringSlice(t *testing.T) {
	values := normalizeStringSlice([]string{" a ", "", "b", "   ", "c"REDACTED)
	if len(values) != 3 || values[0] != "a" || values[1] != "b" || values[2] != "c" {
		t.Fatalf("normalizeStringSlice() unexpected result: %#v", values)
REDACTED
	if normalizeStringSlice(nil) != nil {
		t.Fatalf("normalizeStringSlice(nil) expected nil slice")
REDACTED
REDACTED

func TestGetServerAddressFromEnv(t *testing.T) {
	t.Setenv("SERVER_HOST", "127.0.0.1")
	t.Setenv("SERVER_PORT", "9090")

	address := GetServerAddress()
	if address != "127.0.0.1:9090" {
		t.Fatalf("GetServerAddress() = %q", address)
REDACTED
REDACTED

func TestValidateAbsoluteHTTPURL(t *testing.T) {
	if err := ValidateAbsoluteHTTPURL("https://example.com/path"); err != nil {
		t.Fatalf("ValidateAbsoluteHTTPURL valid url error: %v", err)
REDACTED
	if err := ValidateAbsoluteHTTPURL(""); err == nil {
		t.Fatalf("ValidateAbsoluteHTTPURL should reject empty url")
REDACTED
	if err := ValidateAbsoluteHTTPURL("/relative"); err == nil {
		t.Fatalf("ValidateAbsoluteHTTPURL should reject relative url")
REDACTED
	if err := ValidateAbsoluteHTTPURL("ftp://example.com"); err == nil {
		t.Fatalf("ValidateAbsoluteHTTPURL should reject ftp scheme")
REDACTED
	if err := ValidateAbsoluteHTTPURL("https://example.com/#frag"); err == nil {
		t.Fatalf("ValidateAbsoluteHTTPURL should reject fragment")
REDACTED
REDACTED

func TestValidateServerFrontendURL(t *testing.T) {
	resetViperWithJWTSecret(t)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error: %v", err)
REDACTED

	cfg.Server.FrontendURL = "https://example.com"
	if err := cfg.Validate(); err != nil {
		t.Fatalf("Validate() frontend_url valid error: %v", err)
REDACTED

	cfg.Server.FrontendURL = "https://example.com/path"
	if err := cfg.Validate(); err != nil {
		t.Fatalf("Validate() frontend_url with path valid error: %v", err)
REDACTED

	cfg.Server.FrontendURL = "https://example.com?utm=1"
	if err := cfg.Validate(); err == nil {
		t.Fatalf("Validate() should reject server.frontend_url with query")
REDACTED

	cfg.Server.FrontendURL = "https://user:pass@example.com"
	if err := cfg.Validate(); err == nil {
		t.Fatalf("Validate() should reject server.frontend_url with userinfo")
REDACTED

	cfg.Server.FrontendURL = "/relative"
	if err := cfg.Validate(); err == nil {
		t.Fatalf("Validate() should reject relative server.frontend_url")
REDACTED
REDACTED

func TestValidateFrontendRedirectURL(t *testing.T) {
	if err := ValidateFrontendRedirectURL("/auth/callback"); err != nil {
		t.Fatalf("ValidateFrontendRedirectURL relative error: %v", err)
REDACTED
	if err := ValidateFrontendRedirectURL("https://example.com/auth"); err != nil {
		t.Fatalf("ValidateFrontendRedirectURL absolute error: %v", err)
REDACTED
	if err := ValidateFrontendRedirectURL("example.com/path"); err == nil {
		t.Fatalf("ValidateFrontendRedirectURL should reject non-absolute url")
REDACTED
	if err := ValidateFrontendRedirectURL("//evil.com"); err == nil {
		t.Fatalf("ValidateFrontendRedirectURL should reject // prefix")
REDACTED
	if err := ValidateFrontendRedirectURL("javascript:alert(1)"); err == nil {
		t.Fatalf("ValidateFrontendRedirectURL should reject javascript scheme")
REDACTED
REDACTED

func TestWarnIfInsecureURL(t *testing.T) {
	warnIfInsecureURL("test", "http://example.com")
	warnIfInsecureURL("test", "bad://url")
	warnIfInsecureURL("test", "://invalid")
REDACTED

func TestGenerateJWTSecretDefaultLength(t *testing.T) {
	secret, err := generateJWTSecret(0)
	if err != nil {
		t.Fatalf("generateJWTSecret error: %v", err)
REDACTED
	if len(secret) == 0 {
		t.Fatalf("generateJWTSecret returned empty string")
REDACTED
REDACTED

func TestValidateOpsCleanupScheduleRequired(t *testing.T) {
	resetViperWithJWTSecret(t)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error: %v", err)
REDACTED
	cfg.Ops.Cleanup.Enabled = true
	cfg.Ops.Cleanup.Schedule = ""
	err = cfg.Validate()
	if err == nil {
		t.Fatalf("Validate() expected error for ops.cleanup.schedule")
REDACTED
	if !strings.Contains(err.Error(), "ops.cleanup.schedule") {
		t.Fatalf("Validate() expected ops.cleanup.schedule error, got: %v", err)
REDACTED
REDACTED

func TestValidateConcurrencyPingInterval(t *testing.T) {
	resetViperWithJWTSecret(t)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error: %v", err)
REDACTED
	cfg.Concurrency.PingInterval = 3
	err = cfg.Validate()
	if err == nil {
		t.Fatalf("Validate() expected error for concurrency.ping_interval")
REDACTED
	if !strings.Contains(err.Error(), "concurrency.ping_interval") {
		t.Fatalf("Validate() expected concurrency.ping_interval error, got: %v", err)
REDACTED
REDACTED

func TestProvideConfig(t *testing.T) {
	resetViperWithJWTSecret(t)
	if _, err := ProvideConfig(); err != nil {
		t.Fatalf("ProvideConfig() error: %v", err)
REDACTED
REDACTED

func TestValidateConfigWithLinuxDoEnabled(t *testing.T) {
	resetViperWithJWTSecret(t)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error: %v", err)
REDACTED

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

	if err := cfg.Validate(); err != nil {
		t.Fatalf("Validate() unexpected error: %v", err)
REDACTED
REDACTED

func TestValidateJWTSecretStrength(t *testing.T) {
	if !isWeakJWTSecret("change-me-in-production") {
		t.Fatalf("isWeakJWTSecret should detect weak secret")
REDACTED
	if isWeakJWTSecret("StrongSecretValue") {
		t.Fatalf("isWeakJWTSecret should accept strong secret")
REDACTED
REDACTED

func TestGenerateJWTSecretWithLength(t *testing.T) {
	secret, err := generateJWTSecret(16)
	if err != nil {
		t.Fatalf("generateJWTSecret error: %v", err)
REDACTED
	if len(secret) == 0 {
		t.Fatalf("generateJWTSecret returned empty string")
REDACTED
REDACTED

func TestDatabaseDSNWithTimezone_WithPassword(t *testing.T) {
	d := &DatabaseConfig{
		Host:     "localhost",
		Port:     5432,
		User:     "u",
		Password: "p",
		DBName:   "db",
		SSLMode:  "prefer",
REDACTED
	got := d.DSNWithTimezone("UTC")
	if !strings.Contains(got, "password=p") {
		t.Fatalf("DSNWithTimezone should include password: %q", got)
REDACTED
	if !strings.Contains(got, "TimeZone=UTC") {
		t.Fatalf("DSNWithTimezone should include TimeZone=UTC: %q", got)
REDACTED
REDACTED

func TestValidateAbsoluteHTTPURLMissingHost(t *testing.T) {
	if err := ValidateAbsoluteHTTPURL("https://"); err == nil {
		t.Fatalf("ValidateAbsoluteHTTPURL should reject missing host")
REDACTED
REDACTED

func TestValidateFrontendRedirectURLInvalidChars(t *testing.T) {
	if err := ValidateFrontendRedirectURL("/auth/\ncallback"); err == nil {
		t.Fatalf("ValidateFrontendRedirectURL should reject invalid chars")
REDACTED
	if err := ValidateFrontendRedirectURL("http://"); err == nil {
		t.Fatalf("ValidateFrontendRedirectURL should reject missing host")
REDACTED
	if err := ValidateFrontendRedirectURL("mailto:user@example.com"); err == nil {
		t.Fatalf("ValidateFrontendRedirectURL should reject mailto")
REDACTED
REDACTED

func TestWarnIfInsecureURLHTTPS(t *testing.T) {
	warnIfInsecureURL("secure", "https://example.com")
REDACTED

func TestValidateJWTSecret_UTF8Bytes(t *testing.T) {
	resetViperWithJWTSecret(t)
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error: %v", err)
REDACTED

	// 31 bytes (< 32) even though it's 31 characters.
	cfg.JWT.Secret = strings.Repeat("a", 31)
	err = cfg.Validate()
	if err == nil {
		t.Fatalf("Validate() should reject 31-byte secret")
REDACTED
	if !strings.Contains(err.Error(), "at least 32 bytes") {
		t.Fatalf("Validate() error = %v", err)
REDACTED

	// 32 bytes OK.
	cfg.JWT.Secret = strings.Repeat("a", 32)
	err = cfg.Validate()
	if err != nil {
		t.Fatalf("Validate() should accept 32-byte secret: %v", err)
REDACTED
REDACTED

func TestValidateConfigErrors(t *testing.T) {
	buildValid := func(t *testing.T) *Config {
	REDACTED
		resetViperWithJWTSecret(t)
		cfg, err := Load()
		if err != nil {
			t.Fatalf("Load() error: %v", err)
	REDACTED
		return cfg
REDACTED

	cases := []struct {
		name    string
		mutate  func(*Config)
		wantErr string
REDACTED{
		{
			name:    "jwt secret required",
			mutate:  func(c *Config) { c.JWT.Secret = "" REDACTED,
			wantErr: "jwt.secret is required",
	REDACTED,
		{
			name:    "jwt secret min bytes",
			mutate:  func(c *Config) { c.JWT.Secret = strings.Repeat("a", 31) REDACTED,
			wantErr: "jwt.secret must be at least 32 bytes",
	REDACTED,
		{
			name:    "subscription maintenance worker_count non-negative",
			mutate:  func(c *Config) { c.SubscriptionMaintenance.WorkerCount = -1 REDACTED,
			wantErr: "subscription_maintenance.worker_count",
	REDACTED,
		{
			name:    "subscription maintenance queue_size non-negative",
			mutate:  func(c *Config) { c.SubscriptionMaintenance.QueueSize = -1 REDACTED,
			wantErr: "subscription_maintenance.queue_size",
	REDACTED,
		{
			name:    "jwt expire hour positive",
			mutate:  func(c *Config) { c.JWT.ExpireHour = 0 REDACTED,
			wantErr: "jwt.expire_hour must be positive",
	REDACTED,
		{
			name:    "jwt expire hour max",
			mutate:  func(c *Config) { c.JWT.ExpireHour = 200 REDACTED,
			wantErr: "jwt.expire_hour must be <= 168",
	REDACTED,
		{
			name:    "csp policy required",
			mutate:  func(c *Config) { c.Security.CSP.Enabled = true; c.Security.CSP.Policy = "" REDACTED,
			wantErr: "security.csp.policy",
	REDACTED,
		{
			name: "linuxdo client id required",
			mutate: func(c *Config) {
				c.LinuxDo.Enabled = true
				c.LinuxDo.ClientID = ""
		REDACTED,
			wantErr: "linuxdo_connect.client_id",
	REDACTED,
		{
			name: "linuxdo token auth method",
			mutate: func(c *Config) {
				c.LinuxDo.Enabled = true
				c.LinuxDo.ClientID = "client"
				c.LinuxDo.ClientSecret = "secret"
				c.LinuxDo.AuthorizeURL = "https://example.com/authorize"
				c.LinuxDo.TokenURL = "https://example.com/token"
				c.LinuxDo.UserInfoURL = "https://example.com/userinfo"
				c.LinuxDo.RedirectURL = "https://example.com/callback"
				c.LinuxDo.FrontendRedirectURL = "/auth/callback"
				c.LinuxDo.TokenAuthMethod = "invalid"
		REDACTED,
			wantErr: "linuxdo_connect.token_auth_method",
	REDACTED,
		{
			name:    "billing circuit breaker threshold",
			mutate:  func(c *Config) { c.Billing.CircuitBreaker.FailureThreshold = 0 REDACTED,
			wantErr: "billing.circuit_breaker.failure_threshold",
	REDACTED,
		{
			name:    "billing circuit breaker reset",
			mutate:  func(c *Config) { c.Billing.CircuitBreaker.ResetTimeoutSeconds = 0 REDACTED,
			wantErr: "billing.circuit_breaker.reset_timeout_seconds",
	REDACTED,
		{
			name:    "billing circuit breaker half open",
			mutate:  func(c *Config) { c.Billing.CircuitBreaker.HalfOpenRequests = 0 REDACTED,
			wantErr: "billing.circuit_breaker.half_open_requests",
	REDACTED,
		{
			name:    "database max open conns",
			mutate:  func(c *Config) { c.Database.MaxOpenConns = 0 REDACTED,
			wantErr: "database.max_open_conns",
	REDACTED,
		{
			name:    "database max lifetime",
			mutate:  func(c *Config) { c.Database.ConnMaxLifetimeMinutes = -1 REDACTED,
			wantErr: "database.conn_max_lifetime_minutes",
	REDACTED,
		{
			name:    "database idle exceeds open",
			mutate:  func(c *Config) { c.Database.MaxIdleConns = c.Database.MaxOpenConns + 1 REDACTED,
			wantErr: "database.max_idle_conns cannot exceed",
	REDACTED,
		{
			name:    "redis dial timeout",
			mutate:  func(c *Config) { c.Redis.DialTimeoutSeconds = 0 REDACTED,
			wantErr: "redis.dial_timeout_seconds",
	REDACTED,
		{
			name:    "redis read timeout",
			mutate:  func(c *Config) { c.Redis.ReadTimeoutSeconds = 0 REDACTED,
			wantErr: "redis.read_timeout_seconds",
	REDACTED,
		{
			name:    "redis write timeout",
			mutate:  func(c *Config) { c.Redis.WriteTimeoutSeconds = 0 REDACTED,
			wantErr: "redis.write_timeout_seconds",
	REDACTED,
		{
			name:    "redis pool size",
			mutate:  func(c *Config) { c.Redis.PoolSize = 0 REDACTED,
			wantErr: "redis.pool_size",
	REDACTED,
		{
			name:    "redis idle exceeds pool",
			mutate:  func(c *Config) { c.Redis.MinIdleConns = c.Redis.PoolSize + 1 REDACTED,
			wantErr: "redis.min_idle_conns cannot exceed",
	REDACTED,
		{
			name:    "dashboard cache disabled negative",
			mutate:  func(c *Config) { c.Dashboard.Enabled = false; c.Dashboard.StatsTTLSeconds = -1 REDACTED,
			wantErr: "dashboard_cache.stats_ttl_seconds",
	REDACTED,
		{
			name:    "dashboard cache fresh ttl positive",
			mutate:  func(c *Config) { c.Dashboard.Enabled = true; c.Dashboard.StatsFreshTTLSeconds = 0 REDACTED,
			wantErr: "dashboard_cache.stats_fresh_ttl_seconds",
	REDACTED,
		{
			name:    "dashboard aggregation enabled interval",
			mutate:  func(c *Config) { c.DashboardAgg.Enabled = true; c.DashboardAgg.IntervalSeconds = 0 REDACTED,
			wantErr: "dashboard_aggregation.interval_seconds",
	REDACTED,
		{
			name: "dashboard aggregation backfill positive",
			mutate: func(c *Config) {
				c.DashboardAgg.Enabled = true
				c.DashboardAgg.BackfillEnabled = true
				c.DashboardAgg.BackfillMaxDays = 0
		REDACTED,
			wantErr: "dashboard_aggregation.backfill_max_days",
	REDACTED,
		{
			name:    "dashboard aggregation retention",
			mutate:  func(c *Config) { c.DashboardAgg.Enabled = true; c.DashboardAgg.Retention.UsageLogsDays = 0 REDACTED,
			wantErr: "dashboard_aggregation.retention.usage_logs_days",
	REDACTED,
		{
			name:    "dashboard aggregation disabled interval",
			mutate:  func(c *Config) { c.DashboardAgg.Enabled = false; c.DashboardAgg.IntervalSeconds = -1 REDACTED,
			wantErr: "dashboard_aggregation.interval_seconds",
	REDACTED,
		{
			name:    "usage cleanup max range",
			mutate:  func(c *Config) { c.UsageCleanup.Enabled = true; c.UsageCleanup.MaxRangeDays = 0 REDACTED,
			wantErr: "usage_cleanup.max_range_days",
	REDACTED,
		{
			name:    "usage cleanup worker interval",
			mutate:  func(c *Config) { c.UsageCleanup.Enabled = true; c.UsageCleanup.WorkerIntervalSeconds = 0 REDACTED,
			wantErr: "usage_cleanup.worker_interval_seconds",
	REDACTED,
		{
			name:    "usage cleanup batch size",
			mutate:  func(c *Config) { c.UsageCleanup.Enabled = true; c.UsageCleanup.BatchSize = 0 REDACTED,
			wantErr: "usage_cleanup.batch_size",
	REDACTED,
		{
			name:    "usage cleanup disabled negative",
			mutate:  func(c *Config) { c.UsageCleanup.Enabled = false; c.UsageCleanup.BatchSize = -1 REDACTED,
			wantErr: "usage_cleanup.batch_size",
	REDACTED,
		{
			name:    "gateway max body size",
			mutate:  func(c *Config) { c.Gateway.MaxBodySize = 0 REDACTED,
			wantErr: "gateway.max_body_size",
	REDACTED,
		{
			name:    "gateway max idle conns",
			mutate:  func(c *Config) { c.Gateway.MaxIdleConns = 0 REDACTED,
			wantErr: "gateway.max_idle_conns",
	REDACTED,
		{
			name:    "gateway max idle conns per host",
			mutate:  func(c *Config) { c.Gateway.MaxIdleConnsPerHost = 0 REDACTED,
			wantErr: "gateway.max_idle_conns_per_host",
	REDACTED,
		{
			name:    "gateway idle timeout",
			mutate:  func(c *Config) { c.Gateway.IdleConnTimeoutSeconds = 0 REDACTED,
			wantErr: "gateway.idle_conn_timeout_seconds",
	REDACTED,
		{
			name:    "gateway max upstream clients",
			mutate:  func(c *Config) { c.Gateway.MaxUpstreamClients = 0 REDACTED,
			wantErr: "gateway.max_upstream_clients",
	REDACTED,
		{
			name:    "gateway client idle ttl",
			mutate:  func(c *Config) { c.Gateway.ClientIdleTTLSeconds = 0 REDACTED,
			wantErr: "gateway.client_idle_ttl_seconds",
	REDACTED,
		{
			name:    "gateway concurrency slot ttl",
			mutate:  func(c *Config) { c.Gateway.ConcurrencySlotTTLMinutes = 0 REDACTED,
			wantErr: "gateway.concurrency_slot_ttl_minutes",
	REDACTED,
		{
			name:    "gateway max conns per host",
			mutate:  func(c *Config) { c.Gateway.MaxConnsPerHost = -1 REDACTED,
			wantErr: "gateway.max_conns_per_host",
	REDACTED,
		{
			name:    "gateway connection isolation",
			mutate:  func(c *Config) { c.Gateway.ConnectionPoolIsolation = "invalid" REDACTED,
			wantErr: "gateway.connection_pool_isolation",
	REDACTED,
		{
			name:    "gateway stream keepalive range",
			mutate:  func(c *Config) { c.Gateway.StreamKeepaliveInterval = 4 REDACTED,
			wantErr: "gateway.stream_keepalive_interval",
	REDACTED,
		{
			name:    "gateway stream data interval range",
			mutate:  func(c *Config) { c.Gateway.StreamDataIntervalTimeout = 5 REDACTED,
			wantErr: "gateway.stream_data_interval_timeout",
	REDACTED,
		{
			name:    "gateway stream data interval negative",
			mutate:  func(c *Config) { c.Gateway.StreamDataIntervalTimeout = -1 REDACTED,
			wantErr: "gateway.stream_data_interval_timeout must be non-negative",
	REDACTED,
		{
			name:    "gateway max line size",
			mutate:  func(c *Config) { c.Gateway.MaxLineSize = 1024 REDACTED,
			wantErr: "gateway.max_line_size must be at least",
	REDACTED,
		{
			name:    "gateway max line size negative",
			mutate:  func(c *Config) { c.Gateway.MaxLineSize = -1 REDACTED,
			wantErr: "gateway.max_line_size must be non-negative",
	REDACTED,
		{
			name:    "gateway usage record worker count",
			mutate:  func(c *Config) { c.Gateway.UsageRecord.WorkerCount = 0 REDACTED,
			wantErr: "gateway.usage_record.worker_count",
	REDACTED,
		{
			name:    "gateway usage record queue size",
			mutate:  func(c *Config) { c.Gateway.UsageRecord.QueueSize = 0 REDACTED,
			wantErr: "gateway.usage_record.queue_size",
	REDACTED,
		{
			name:    "gateway usage record timeout",
			mutate:  func(c *Config) { c.Gateway.UsageRecord.TaskTimeoutSeconds = 0 REDACTED,
			wantErr: "gateway.usage_record.task_timeout_seconds",
	REDACTED,
		{
			name:    "gateway usage record overflow policy",
			mutate:  func(c *Config) { c.Gateway.UsageRecord.OverflowPolicy = "invalid" REDACTED,
			wantErr: "gateway.usage_record.overflow_policy",
	REDACTED,
		{
			name:    "gateway usage record sample percent range",
			mutate:  func(c *Config) { c.Gateway.UsageRecord.OverflowSamplePercent = 101 REDACTED,
			wantErr: "gateway.usage_record.overflow_sample_percent",
	REDACTED,
		{
			name: "gateway usage record sample percent required for sample policy",
			mutate: func(c *Config) {
				c.Gateway.UsageRecord.OverflowPolicy = UsageRecordOverflowPolicySample
				c.Gateway.UsageRecord.OverflowSamplePercent = 0
		REDACTED,
			wantErr: "gateway.usage_record.overflow_sample_percent must be positive",
	REDACTED,
		{
			name: "gateway usage record auto scale max gte min",
			mutate: func(c *Config) {
				c.Gateway.UsageRecord.AutoScaleMinWorkers = 256
				c.Gateway.UsageRecord.AutoScaleMaxWorkers = 128
		REDACTED,
			wantErr: "gateway.usage_record.auto_scale_max_workers",
	REDACTED,
		{
			name: "gateway usage record worker in auto scale range",
			mutate: func(c *Config) {
				c.Gateway.UsageRecord.AutoScaleMinWorkers = 200
				c.Gateway.UsageRecord.AutoScaleMaxWorkers = 300
				c.Gateway.UsageRecord.WorkerCount = 128
		REDACTED,
			wantErr: "gateway.usage_record.worker_count must be between auto_scale_min_workers and auto_scale_max_workers",
	REDACTED,
		{
			name: "gateway usage record auto scale queue thresholds order",
			mutate: func(c *Config) {
				c.Gateway.UsageRecord.AutoScaleUpQueuePercent = 50
				c.Gateway.UsageRecord.AutoScaleDownQueuePercent = 50
		REDACTED,
			wantErr: "gateway.usage_record.auto_scale_down_queue_percent must be less",
	REDACTED,
		{
			name:    "gateway usage record auto scale up step",
			mutate:  func(c *Config) { c.Gateway.UsageRecord.AutoScaleUpStep = 0 REDACTED,
			wantErr: "gateway.usage_record.auto_scale_up_step",
	REDACTED,
		{
			name:    "gateway usage record auto scale interval",
			mutate:  func(c *Config) { c.Gateway.UsageRecord.AutoScaleCheckIntervalSeconds = 0 REDACTED,
			wantErr: "gateway.usage_record.auto_scale_check_interval_seconds",
	REDACTED,
		{
			name:    "gateway scheduling sticky waiting",
			mutate:  func(c *Config) { c.Gateway.Scheduling.StickySessionMaxWaiting = 0 REDACTED,
			wantErr: "gateway.scheduling.sticky_session_max_waiting",
	REDACTED,
		{
			name:    "gateway scheduling outbox poll",
			mutate:  func(c *Config) { c.Gateway.Scheduling.OutboxPollIntervalSeconds = 0 REDACTED,
			wantErr: "gateway.scheduling.outbox_poll_interval_seconds",
	REDACTED,
		{
			name:    "gateway scheduling outbox failures",
			mutate:  func(c *Config) { c.Gateway.Scheduling.OutboxLagRebuildFailures = 0 REDACTED,
			wantErr: "gateway.scheduling.outbox_lag_rebuild_failures",
	REDACTED,
		{
			name: "gateway outbox lag rebuild",
			mutate: func(c *Config) {
				c.Gateway.Scheduling.OutboxLagWarnSeconds = 10
				c.Gateway.Scheduling.OutboxLagRebuildSeconds = 5
		REDACTED,
			wantErr: "gateway.scheduling.outbox_lag_rebuild_seconds",
	REDACTED,
		{
			name:    "log level invalid",
			mutate:  func(c *Config) { c.Log.Level = "trace" REDACTED,
			wantErr: "log.level",
	REDACTED,
		{
			name:    "log format invalid",
			mutate:  func(c *Config) { c.Log.Format = "plain" REDACTED,
			wantErr: "log.format",
	REDACTED,
		{
			name: "log output disabled",
			mutate: func(c *Config) {
				c.Log.Output.ToStdout = false
				c.Log.Output.ToFile = false
		REDACTED,
			wantErr: "log.output.to_stdout and log.output.to_file cannot both be false",
	REDACTED,
		{
			name:    "log rotation size",
			mutate:  func(c *Config) { c.Log.Rotation.MaxSizeMB = 0 REDACTED,
			wantErr: "log.rotation.max_size_mb",
	REDACTED,
		{
			name: "log sampling enabled invalid",
			mutate: func(c *Config) {
				c.Log.Sampling.Enabled = true
				c.Log.Sampling.Initial = 0
		REDACTED,
			wantErr: "log.sampling.initial",
	REDACTED,
		{
			name:    "ops metrics collector ttl",
			mutate:  func(c *Config) { c.Ops.MetricsCollectorCache.TTL = -1 REDACTED,
			wantErr: "ops.metrics_collector_cache.ttl",
	REDACTED,
		{
			name:    "ops cleanup retention",
			mutate:  func(c *Config) { c.Ops.Cleanup.ErrorLogRetentionDays = -1 REDACTED,
			wantErr: "ops.cleanup.error_log_retention_days",
	REDACTED,
		{
			name:    "ops cleanup minute retention",
			mutate:  func(c *Config) { c.Ops.Cleanup.MinuteMetricsRetentionDays = -1 REDACTED,
			wantErr: "ops.cleanup.minute_metrics_retention_days",
	REDACTED,
REDACTED

	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			cfg := buildValid(t)
			tt.mutate(cfg)
			err := cfg.Validate()
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("Validate() error = %v, want %q", err, tt.wantErr)
		REDACTED
	REDACTED)
REDACTED
REDACTED

func TestValidateConfig_AutoScaleDisabledIgnoreAutoScaleFields(t *testing.T) {
	resetViperWithJWTSecret(t)
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error: %v", err)
REDACTED

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
REDACTED
REDACTED

func TestValidateConfig_LogRequiredAndRotationBounds(t *testing.T) {
	resetViperWithJWTSecret(t)

	cases := []struct {
		name    string
		mutate  func(*Config)
		wantErr string
REDACTED{
		{
			name: "log level required",
			mutate: func(c *Config) {
				c.Log.Level = ""
		REDACTED,
			wantErr: "log.level is required",
	REDACTED,
		{
			name: "log format required",
			mutate: func(c *Config) {
				c.Log.Format = ""
		REDACTED,
			wantErr: "log.format is required",
	REDACTED,
		{
			name: "log stacktrace required",
			mutate: func(c *Config) {
				c.Log.StacktraceLevel = ""
		REDACTED,
			wantErr: "log.stacktrace_level is required",
	REDACTED,
		{
			name: "log max backups non-negative",
			mutate: func(c *Config) {
				c.Log.Rotation.MaxBackups = -1
		REDACTED,
			wantErr: "log.rotation.max_backups must be non-negative",
	REDACTED,
		{
			name: "log max age non-negative",
			mutate: func(c *Config) {
				c.Log.Rotation.MaxAgeDays = -1
		REDACTED,
			wantErr: "log.rotation.max_age_days must be non-negative",
	REDACTED,
		{
			name: "sampling thereafter non-negative when disabled",
			mutate: func(c *Config) {
				c.Log.Sampling.Enabled = false
				c.Log.Sampling.Thereafter = -1
		REDACTED,
			wantErr: "log.sampling.thereafter must be non-negative",
	REDACTED,
REDACTED

	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			cfg, err := Load()
			if err != nil {
				t.Fatalf("Load() error: %v", err)
		REDACTED
			tt.mutate(cfg)
			err = cfg.Validate()
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("Validate() error = %v, want %q", err, tt.wantErr)
		REDACTED
	REDACTED)
REDACTED
REDACTED

func TestSoraCurlCFFISidecarDefaults(t *testing.T) {
	resetViperWithJWTSecret(t)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error: %v", err)
REDACTED

	if !cfg.Sora.Client.CurlCFFISidecar.Enabled {
		t.Fatalf("Sora curl_cffi sidecar should be enabled by default")
REDACTED
	if cfg.Sora.Client.CloudflareChallengeCooldownSeconds <= 0 {
		t.Fatalf("Sora cloudflare challenge cooldown should be positive by default")
REDACTED
	if cfg.Sora.Client.CurlCFFISidecar.BaseURL == "" {
		t.Fatalf("Sora curl_cffi sidecar base_url should not be empty by default")
REDACTED
	if cfg.Sora.Client.CurlCFFISidecar.Impersonate == "" {
		t.Fatalf("Sora curl_cffi sidecar impersonate should not be empty by default")
REDACTED
	if !cfg.Sora.Client.CurlCFFISidecar.SessionReuseEnabled {
		t.Fatalf("Sora curl_cffi sidecar session reuse should be enabled by default")
REDACTED
	if cfg.Sora.Client.CurlCFFISidecar.SessionTTLSeconds <= 0 {
		t.Fatalf("Sora curl_cffi sidecar session ttl should be positive by default")
REDACTED
REDACTED

func TestValidateSoraCurlCFFISidecarRequired(t *testing.T) {
	resetViperWithJWTSecret(t)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error: %v", err)
REDACTED

	cfg.Sora.Client.CurlCFFISidecar.Enabled = false
	err = cfg.Validate()
	if err == nil || !strings.Contains(err.Error(), "sora.client.curl_cffi_sidecar.enabled must be true") {
		t.Fatalf("Validate() error = %v, want sidecar enabled error", err)
REDACTED
REDACTED

func TestValidateSoraCurlCFFISidecarBaseURLRequired(t *testing.T) {
	resetViperWithJWTSecret(t)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error: %v", err)
REDACTED

	cfg.Sora.Client.CurlCFFISidecar.BaseURL = "   "
	err = cfg.Validate()
	if err == nil || !strings.Contains(err.Error(), "sora.client.curl_cffi_sidecar.base_url is required") {
		t.Fatalf("Validate() error = %v, want sidecar base_url required error", err)
REDACTED
REDACTED

func TestValidateSoraCurlCFFISidecarSessionTTLNonNegative(t *testing.T) {
	resetViperWithJWTSecret(t)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error: %v", err)
REDACTED

	cfg.Sora.Client.CurlCFFISidecar.SessionTTLSeconds = -1
	err = cfg.Validate()
	if err == nil || !strings.Contains(err.Error(), "sora.client.curl_cffi_sidecar.session_ttl_seconds must be non-negative") {
		t.Fatalf("Validate() error = %v, want sidecar session ttl error", err)
REDACTED
REDACTED

func TestValidateSoraCloudflareChallengeCooldownNonNegative(t *testing.T) {
	resetViperWithJWTSecret(t)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error: %v", err)
REDACTED

	cfg.Sora.Client.CloudflareChallengeCooldownSeconds = -1
	err = cfg.Validate()
	if err == nil || !strings.Contains(err.Error(), "sora.client.cloudflare_challenge_cooldown_seconds must be non-negative") {
		t.Fatalf("Validate() error = %v, want cloudflare cooldown error", err)
REDACTED
REDACTED

func TestLoad_DefaultGatewayUsageRecordConfig(t *testing.T) {
	resetViperWithJWTSecret(t)
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error: %v", err)
REDACTED
	if cfg.Gateway.UsageRecord.WorkerCount != 128 {
		t.Fatalf("worker_count = %d, want 128", cfg.Gateway.UsageRecord.WorkerCount)
REDACTED
	if cfg.Gateway.UsageRecord.QueueSize != 16384 {
		t.Fatalf("queue_size = %d, want 16384", cfg.Gateway.UsageRecord.QueueSize)
REDACTED
	if cfg.Gateway.UsageRecord.TaskTimeoutSeconds != 5 {
		t.Fatalf("task_timeout_seconds = %d, want 5", cfg.Gateway.UsageRecord.TaskTimeoutSeconds)
REDACTED
	if cfg.Gateway.UsageRecord.OverflowPolicy != UsageRecordOverflowPolicySample {
		t.Fatalf("overflow_policy = %s, want %s", cfg.Gateway.UsageRecord.OverflowPolicy, UsageRecordOverflowPolicySample)
REDACTED
	if cfg.Gateway.UsageRecord.OverflowSamplePercent != 10 {
		t.Fatalf("overflow_sample_percent = %d, want 10", cfg.Gateway.UsageRecord.OverflowSamplePercent)
REDACTED
	if !cfg.Gateway.UsageRecord.AutoScaleEnabled {
		t.Fatalf("auto_scale_enabled = false, want true")
REDACTED
	if cfg.Gateway.UsageRecord.AutoScaleMinWorkers != 128 {
		t.Fatalf("auto_scale_min_workers = %d, want 128", cfg.Gateway.UsageRecord.AutoScaleMinWorkers)
REDACTED
	if cfg.Gateway.UsageRecord.AutoScaleMaxWorkers != 512 {
		t.Fatalf("auto_scale_max_workers = %d, want 512", cfg.Gateway.UsageRecord.AutoScaleMaxWorkers)
REDACTED
	if cfg.Gateway.UsageRecord.AutoScaleUpQueuePercent != 70 {
		t.Fatalf("auto_scale_up_queue_percent = %d, want 70", cfg.Gateway.UsageRecord.AutoScaleUpQueuePercent)
REDACTED
	if cfg.Gateway.UsageRecord.AutoScaleDownQueuePercent != 15 {
		t.Fatalf("auto_scale_down_queue_percent = %d, want 15", cfg.Gateway.UsageRecord.AutoScaleDownQueuePercent)
REDACTED
	if cfg.Gateway.UsageRecord.AutoScaleUpStep != 32 {
		t.Fatalf("auto_scale_up_step = %d, want 32", cfg.Gateway.UsageRecord.AutoScaleUpStep)
REDACTED
	if cfg.Gateway.UsageRecord.AutoScaleDownStep != 16 {
		t.Fatalf("auto_scale_down_step = %d, want 16", cfg.Gateway.UsageRecord.AutoScaleDownStep)
REDACTED
	if cfg.Gateway.UsageRecord.AutoScaleCheckIntervalSeconds != 3 {
		t.Fatalf("auto_scale_check_interval_seconds = %d, want 3", cfg.Gateway.UsageRecord.AutoScaleCheckIntervalSeconds)
REDACTED
	if cfg.Gateway.UsageRecord.AutoScaleCooldownSeconds != 10 {
		t.Fatalf("auto_scale_cooldown_seconds = %d, want 10", cfg.Gateway.UsageRecord.AutoScaleCooldownSeconds)
REDACTED
REDACTED
