package config

import (
	"strings"
	"testing"
	"time"

	"github.com/spf13/viper"
)

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
	viper.Reset()

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
	viper.Reset()
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
	viper.Reset()

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
	if cfg.Security.ResponseHeaders.Enabled {
		t.Fatalf("ResponseHeaders.Enabled = true, want false")
REDACTED
REDACTED

func TestValidateLinuxDoFrontendRedirectURL(t *testing.T) {
	viper.Reset()

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
	viper.Reset()

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
	viper.Reset()

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
	viper.Reset()

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
	viper.Reset()

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
	viper.Reset()

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
	viper.Reset()

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
	viper.Reset()

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
