package service

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	"strings"
	"time"
)

const (
	opsAlertEvaluatorLeaderLockKeyDefault = "ops:alert:evaluator:leader"
	opsAlertEvaluatorLeaderLockTTLDefault = 30 * time.Second
)

// =========================
// Email notification config
// =========================

func (s *OpsService) GetEmailNotificationConfig(ctx context.Context) (*OpsEmailNotificationConfig, error) {
	defaultCfg := defaultOpsEmailNotificationConfig()
	if s == nil || s.settingRepo == nil {
		return defaultCfg, nil
REDACTED
	if ctx == nil {
		ctx = context.Background()
REDACTED

	raw, err := s.settingRepo.GetValue(ctx, SettingKeyOpsEmailNotificationConfig)
	if err != nil {
		if errors.Is(err, ErrSettingNotFound) {
			// Initialize defaults on first read (best-effort).
			if b, mErr := json.Marshal(defaultCfg); mErr == nil {
				_ = s.settingRepo.Set(ctx, SettingKeyOpsEmailNotificationConfig, string(b))
		REDACTED
			return defaultCfg, nil
	REDACTED
		return nil, err
REDACTED

	cfg := &OpsEmailNotificationConfig{REDACTED
	if err := json.Unmarshal([]byte(raw), cfg); err != nil {
		// Corrupted JSON should not break ops UI; fall back to defaults.
		return defaultCfg, nil
REDACTED
	normalizeOpsEmailNotificationConfig(cfg)
	return cfg, nil
REDACTED

func (s *OpsService) UpdateEmailNotificationConfig(ctx context.Context, req *OpsEmailNotificationConfigUpdateRequest) (*OpsEmailNotificationConfig, error) {
	if s == nil || s.settingRepo == nil {
		return nil, errors.New("setting repository not initialized")
REDACTED
	if ctx == nil {
		ctx = context.Background()
REDACTED
	if req == nil {
		return nil, errors.New("invalid request")
REDACTED

	cfg, err := s.GetEmailNotificationConfig(ctx)
	if err != nil {
		return nil, err
REDACTED

	if req.Alert != nil {
		cfg.Alert.Enabled = req.Alert.Enabled
		if req.Alert.Recipients != nil {
			cfg.Alert.Recipients = req.Alert.Recipients
	REDACTED
		cfg.Alert.MinSeverity = strings.TrimSpace(req.Alert.MinSeverity)
		cfg.Alert.RateLimitPerHour = req.Alert.RateLimitPerHour
		cfg.Alert.BatchingWindowSeconds = req.Alert.BatchingWindowSeconds
		cfg.Alert.IncludeResolvedAlerts = req.Alert.IncludeResolvedAlerts
REDACTED

	if req.Report != nil {
		cfg.Report.Enabled = req.Report.Enabled
		if req.Report.Recipients != nil {
			cfg.Report.Recipients = req.Report.Recipients
	REDACTED
		cfg.Report.DailySummaryEnabled = req.Report.DailySummaryEnabled
		cfg.Report.DailySummarySchedule = strings.TrimSpace(req.Report.DailySummarySchedule)
		cfg.Report.WeeklySummaryEnabled = req.Report.WeeklySummaryEnabled
		cfg.Report.WeeklySummarySchedule = strings.TrimSpace(req.Report.WeeklySummarySchedule)
		cfg.Report.ErrorDigestEnabled = req.Report.ErrorDigestEnabled
		cfg.Report.ErrorDigestSchedule = strings.TrimSpace(req.Report.ErrorDigestSchedule)
		cfg.Report.ErrorDigestMinCount = req.Report.ErrorDigestMinCount
		cfg.Report.AccountHealthEnabled = req.Report.AccountHealthEnabled
		cfg.Report.AccountHealthSchedule = strings.TrimSpace(req.Report.AccountHealthSchedule)
		cfg.Report.AccountHealthErrorRateThreshold = req.Report.AccountHealthErrorRateThreshold
REDACTED

	if err := validateOpsEmailNotificationConfig(cfg); err != nil {
		return nil, err
REDACTED

	normalizeOpsEmailNotificationConfig(cfg)
	raw, err := json.Marshal(cfg)
	if err != nil {
		return nil, err
REDACTED
	if err := s.settingRepo.Set(ctx, SettingKeyOpsEmailNotificationConfig, string(raw)); err != nil {
		return nil, err
REDACTED
	return cfg, nil
REDACTED

func defaultOpsEmailNotificationConfig() *OpsEmailNotificationConfig {
	return &OpsEmailNotificationConfig{
		Alert: OpsEmailAlertConfig{
			Enabled:               true,
			Recipients:            []string{REDACTED,
			MinSeverity:           "",
			RateLimitPerHour:      0,
			BatchingWindowSeconds: 0,
			IncludeResolvedAlerts: false,
	REDACTED,
		Report: OpsEmailReportConfig{
			Enabled:                         false,
			Recipients:                      []string{REDACTED,
			DailySummaryEnabled:             false,
			DailySummarySchedule:            "0 9 * * *",
			WeeklySummaryEnabled:            false,
			WeeklySummarySchedule:           "0 9 * * 1",
			ErrorDigestEnabled:              false,
			ErrorDigestSchedule:             "0 9 * * *",
			ErrorDigestMinCount:             10,
			AccountHealthEnabled:            false,
			AccountHealthSchedule:           "0 9 * * *",
			AccountHealthErrorRateThreshold: 10.0,
	REDACTED,
REDACTED
REDACTED

func normalizeOpsEmailNotificationConfig(cfg *OpsEmailNotificationConfig) {
	if cfg == nil {
		return
REDACTED
	if cfg.Alert.Recipients == nil {
		cfg.Alert.Recipients = []string{REDACTED
REDACTED
	if cfg.Report.Recipients == nil {
		cfg.Report.Recipients = []string{REDACTED
REDACTED

	cfg.Alert.MinSeverity = strings.TrimSpace(cfg.Alert.MinSeverity)
	cfg.Report.DailySummarySchedule = strings.TrimSpace(cfg.Report.DailySummarySchedule)
	cfg.Report.WeeklySummarySchedule = strings.TrimSpace(cfg.Report.WeeklySummarySchedule)
	cfg.Report.ErrorDigestSchedule = strings.TrimSpace(cfg.Report.ErrorDigestSchedule)
	cfg.Report.AccountHealthSchedule = strings.TrimSpace(cfg.Report.AccountHealthSchedule)

	// Fill missing schedules with defaults to avoid breaking cron logic if clients send empty strings.
	if cfg.Report.DailySummarySchedule == "" {
		cfg.Report.DailySummarySchedule = "0 9 * * *"
REDACTED
	if cfg.Report.WeeklySummarySchedule == "" {
		cfg.Report.WeeklySummarySchedule = "0 9 * * 1"
REDACTED
	if cfg.Report.ErrorDigestSchedule == "" {
		cfg.Report.ErrorDigestSchedule = "0 9 * * *"
REDACTED
	if cfg.Report.AccountHealthSchedule == "" {
		cfg.Report.AccountHealthSchedule = "0 9 * * *"
REDACTED
REDACTED

func validateOpsEmailNotificationConfig(cfg *OpsEmailNotificationConfig) error {
	if cfg == nil {
		return errors.New("invalid config")
REDACTED

	if cfg.Alert.RateLimitPerHour < 0 {
		return errors.New("alert.rate_limit_per_hour must be >= 0")
REDACTED
	if cfg.Alert.BatchingWindowSeconds < 0 {
		return errors.New("alert.batching_window_seconds must be >= 0")
REDACTED
	switch strings.TrimSpace(cfg.Alert.MinSeverity) {
	case "", "critical", "warning", "info":
	default:
		return errors.New("alert.min_severity must be one of: critical, warning, info, or empty")
REDACTED

	if cfg.Report.ErrorDigestMinCount < 0 {
		return errors.New("report.error_digest_min_count must be >= 0")
REDACTED
	if cfg.Report.AccountHealthErrorRateThreshold < 0 || cfg.Report.AccountHealthErrorRateThreshold > 100 {
		return errors.New("report.account_health_error_rate_threshold must be between 0 and 100")
REDACTED
	return nil
REDACTED

// =========================
// Alert runtime settings
// =========================

func defaultOpsAlertRuntimeSettings() *OpsAlertRuntimeSettings {
	return &OpsAlertRuntimeSettings{
		EvaluationIntervalSeconds: 60,
		DistributedLock: OpsDistributedLockSettings{
			Enabled:    true,
			Key:        opsAlertEvaluatorLeaderLockKeyDefault,
			TTLSeconds: int(opsAlertEvaluatorLeaderLockTTLDefault.Seconds()),
	REDACTED,
		Silencing: OpsAlertSilencingSettings{
			Enabled:            false,
			GlobalUntilRFC3339: "",
			GlobalReason:       "",
			Entries:            []OpsAlertSilenceEntry{REDACTED,
	REDACTED,
REDACTED
REDACTED

func normalizeOpsDistributedLockSettings(s *OpsDistributedLockSettings, defaultKey string, defaultTTLSeconds int) {
	if s == nil {
		return
REDACTED
	s.Key = strings.TrimSpace(s.Key)
	if s.Key == "" {
		s.Key = defaultKey
REDACTED
	if s.TTLSeconds <= 0 {
		s.TTLSeconds = defaultTTLSeconds
REDACTED
REDACTED

func normalizeOpsAlertSilencingSettings(s *OpsAlertSilencingSettings) {
	if s == nil {
		return
REDACTED
	s.GlobalUntilRFC3339 = strings.TrimSpace(s.GlobalUntilRFC3339)
	s.GlobalReason = strings.TrimSpace(s.GlobalReason)
	if s.Entries == nil {
		s.Entries = []OpsAlertSilenceEntry{REDACTED
REDACTED
	for i := range s.Entries {
		s.Entries[i].UntilRFC3339 = strings.TrimSpace(s.Entries[i].UntilRFC3339)
		s.Entries[i].Reason = strings.TrimSpace(s.Entries[i].Reason)
REDACTED
REDACTED

func validateOpsDistributedLockSettings(s OpsDistributedLockSettings) error {
	if strings.TrimSpace(s.Key) == "" {
		return errors.New("distributed_lock.key is required")
REDACTED
	if s.TTLSeconds <= 0 || s.TTLSeconds > int((24*time.Hour).Seconds()) {
		return errors.New("distributed_lock.ttl_seconds must be between 1 and 86400")
REDACTED
	return nil
REDACTED

func validateOpsAlertSilencingSettings(s OpsAlertSilencingSettings) error {
	parse := func(raw string) error {
		if strings.TrimSpace(raw) == "" {
			return nil
	REDACTED
		if _, err := time.Parse(time.RFC3339, raw); err != nil {
			return errors.New("silencing time must be RFC3339")
	REDACTED
		return nil
REDACTED

	if err := parse(s.GlobalUntilRFC3339); err != nil {
		return err
REDACTED
	for _, entry := range s.Entries {
		if strings.TrimSpace(entry.UntilRFC3339) == "" {
			return errors.New("silencing.entries.until_rfc3339 is required")
	REDACTED
		if _, err := time.Parse(time.RFC3339, entry.UntilRFC3339); err != nil {
			return errors.New("silencing.entries.until_rfc3339 must be RFC3339")
	REDACTED
REDACTED
	return nil
REDACTED

func (s *OpsService) GetOpsAlertRuntimeSettings(ctx context.Context) (*OpsAlertRuntimeSettings, error) {
	defaultCfg := defaultOpsAlertRuntimeSettings()
	if s == nil || s.settingRepo == nil {
		return defaultCfg, nil
REDACTED
	if ctx == nil {
		ctx = context.Background()
REDACTED

	raw, err := s.settingRepo.GetValue(ctx, SettingKeyOpsAlertRuntimeSettings)
	if err != nil {
		if errors.Is(err, ErrSettingNotFound) {
			if b, mErr := json.Marshal(defaultCfg); mErr == nil {
				_ = s.settingRepo.Set(ctx, SettingKeyOpsAlertRuntimeSettings, string(b))
		REDACTED
			return defaultCfg, nil
	REDACTED
		return nil, err
REDACTED

	cfg := &OpsAlertRuntimeSettings{REDACTED
	if err := json.Unmarshal([]byte(raw), cfg); err != nil {
		return defaultCfg, nil
REDACTED

	if cfg.EvaluationIntervalSeconds <= 0 {
		cfg.EvaluationIntervalSeconds = defaultCfg.EvaluationIntervalSeconds
REDACTED
	normalizeOpsDistributedLockSettings(&cfg.DistributedLock, opsAlertEvaluatorLeaderLockKeyDefault, defaultCfg.DistributedLock.TTLSeconds)
	normalizeOpsAlertSilencingSettings(&cfg.Silencing)

	return cfg, nil
REDACTED

func (s *OpsService) UpdateOpsAlertRuntimeSettings(ctx context.Context, cfg *OpsAlertRuntimeSettings) (*OpsAlertRuntimeSettings, error) {
	if s == nil || s.settingRepo == nil {
		return nil, errors.New("setting repository not initialized")
REDACTED
	if ctx == nil {
		ctx = context.Background()
REDACTED
	if cfg == nil {
		return nil, errors.New("invalid config")
REDACTED

	if cfg.EvaluationIntervalSeconds < 1 || cfg.EvaluationIntervalSeconds > int((24*time.Hour).Seconds()) {
		return nil, errors.New("evaluation_interval_seconds must be between 1 and 86400")
REDACTED
	if cfg.DistributedLock.Enabled {
		if err := validateOpsDistributedLockSettings(cfg.DistributedLock); err != nil {
			return nil, err
	REDACTED
REDACTED
	if cfg.Silencing.Enabled {
		if err := validateOpsAlertSilencingSettings(cfg.Silencing); err != nil {
			return nil, err
	REDACTED
REDACTED

	defaultCfg := defaultOpsAlertRuntimeSettings()
	normalizeOpsDistributedLockSettings(&cfg.DistributedLock, opsAlertEvaluatorLeaderLockKeyDefault, defaultCfg.DistributedLock.TTLSeconds)
	normalizeOpsAlertSilencingSettings(&cfg.Silencing)

	raw, err := json.Marshal(cfg)
	if err != nil {
		return nil, err
REDACTED
	if err := s.settingRepo.Set(ctx, SettingKeyOpsAlertRuntimeSettings, string(raw)); err != nil {
		return nil, err
REDACTED

	// Return a fresh copy (avoid callers holding pointers into internal slices that may be mutated).
	updated := &OpsAlertRuntimeSettings{REDACTED
	_ = json.Unmarshal(raw, updated)
	return updated, nil
REDACTED

// =========================
// Advanced settings
// =========================

func defaultOpsAdvancedSettings() *OpsAdvancedSettings {
	return &OpsAdvancedSettings{
		DataRetention: OpsDataRetentionSettings{
			CleanupEnabled:             false,
			CleanupSchedule:            opsCleanupDefaultSchedule,
			ErrorLogRetentionDays:      30,
			MinuteMetricsRetentionDays: 30,
			HourlyMetricsRetentionDays: 30,
	REDACTED,
		Aggregation: OpsAggregationSettings{
			AggregationEnabled: false,
	REDACTED,
		OpenAIAccountQuotaAutoPause:     OpsOpenAIAccountQuotaAutoPauseSettings{REDACTED,
		IgnoreCountTokensErrors:         true,  // count_tokens 404 是预期行为，默认忽略
		IgnoreContextCanceled:           true,  // Default to true - client disconnects are not errors
		IgnoreNoAvailableAccounts:       false, // Default to false - this is a real routing issue
		IgnoreInvalidApiKeyErrors:       true,  // Legacy compatibility field; admission rejects are always excluded.
		IgnoreInsufficientBalanceErrors: false, // 默认不忽略，余额不足可能需要关注
		DisplayOpenAITokenStats:         false,
		DisplayAlertEvents:              true,
		AutoRefreshEnabled:              false,
		AutoRefreshIntervalSec:          30,
REDACTED
REDACTED

func normalizeOpsAdvancedSettings(cfg *OpsAdvancedSettings) {
	if cfg == nil {
		return
REDACTED
	// Admission rejects are a security/traffic concern, not an operational
	// request-error category. Keep the legacy field true for old clients but do
	// not allow it to re-enable those rows.
	cfg.IgnoreInvalidApiKeyErrors = true
	cfg.OpenAIAccountQuotaAutoPause.DefaultThreshold5h = clampOpsQuotaAutoPauseThreshold(cfg.OpenAIAccountQuotaAutoPause.DefaultThreshold5h)
	cfg.OpenAIAccountQuotaAutoPause.DefaultThreshold7d = clampOpsQuotaAutoPauseThreshold(cfg.OpenAIAccountQuotaAutoPause.DefaultThreshold7d)
	cfg.DataRetention.CleanupSchedule = strings.TrimSpace(cfg.DataRetention.CleanupSchedule)
	if cfg.DataRetention.CleanupSchedule == "" {
		cfg.DataRetention.CleanupSchedule = opsCleanupDefaultSchedule
REDACTED
	// 保留天数：0 表示每次定时清理全部（清空所有），> 0 表示按天数保留；
	// 仅在拿到非法的负数时回填默认值，避免覆盖用户主动设的 0。
	if cfg.DataRetention.ErrorLogRetentionDays < 0 {
		cfg.DataRetention.ErrorLogRetentionDays = 30
REDACTED
	if cfg.DataRetention.MinuteMetricsRetentionDays < 0 {
		cfg.DataRetention.MinuteMetricsRetentionDays = 30
REDACTED
	if cfg.DataRetention.HourlyMetricsRetentionDays < 0 {
		cfg.DataRetention.HourlyMetricsRetentionDays = 30
REDACTED
	// Normalize auto refresh interval (default 30 seconds)
	if cfg.AutoRefreshIntervalSec <= 0 {
		cfg.AutoRefreshIntervalSec = 30
REDACTED
REDACTED

func clampOpsQuotaAutoPauseThreshold(value float64) float64 {
	if value <= 0 {
		return 0
REDACTED
	if value > 1 {
		return 1
REDACTED
	return value
REDACTED

func validateOpsAdvancedSettings(cfg *OpsAdvancedSettings) error {
	if cfg == nil {
		return errors.New("invalid config")
REDACTED
	// 保留天数：0 表示每次清理全部，1-365 表示按天数保留。
	if cfg.DataRetention.ErrorLogRetentionDays < 0 || cfg.DataRetention.ErrorLogRetentionDays > 365 {
		return errors.New("error_log_retention_days must be between 0 and 365")
REDACTED
	if cfg.DataRetention.MinuteMetricsRetentionDays < 0 || cfg.DataRetention.MinuteMetricsRetentionDays > 365 {
		return errors.New("minute_metrics_retention_days must be between 0 and 365")
REDACTED
	if cfg.DataRetention.HourlyMetricsRetentionDays < 0 || cfg.DataRetention.HourlyMetricsRetentionDays > 365 {
		return errors.New("hourly_metrics_retention_days must be between 0 and 365")
REDACTED
	if cfg.AutoRefreshIntervalSec < 15 || cfg.AutoRefreshIntervalSec > 300 {
		return errors.New("auto_refresh_interval_seconds must be between 15 and 300")
REDACTED
	return nil
REDACTED

func (s *OpsService) GetOpsAdvancedSettings(ctx context.Context) (*OpsAdvancedSettings, error) {
	_ = ctx
	cfg := s.OpsAdvancedSettingsSnapshot()
	return &cfg, nil
REDACTED

// OpsAdvancedSettingsSnapshot returns a value copy for request hot paths. It
// avoids both repository I/O and pointer escape/allocation.
func (s *OpsService) OpsAdvancedSettingsSnapshot() OpsAdvancedSettings {
	if s != nil {
		if snapshot := s.runtimeSettings.Load(); snapshot != nil {
			return snapshot.advanced
	REDACTED
REDACTED
	return *defaultOpsAdvancedSettings()
REDACTED

func (s *OpsService) UpdateOpsAdvancedSettings(ctx context.Context, cfg *OpsAdvancedSettings) (*OpsAdvancedSettings, error) {
	if s == nil || s.settingRepo == nil {
		return nil, errors.New("setting repository not initialized")
REDACTED
	if ctx == nil {
		ctx = context.Background()
REDACTED
	if cfg == nil {
		return nil, errors.New("invalid config")
REDACTED

	if err := validateOpsAdvancedSettings(cfg); err != nil {
		return nil, err
REDACTED

	normalizeOpsAdvancedSettings(cfg)
	raw, err := json.Marshal(cfg)
	if err != nil {
		return nil, err
REDACTED
	if err := s.settingRepo.Set(ctx, SettingKeyOpsAdvancedSettings, string(raw)); err != nil {
		return nil, err
REDACTED
	s.storeAdvancedSettingsSnapshot(cfg)
	// Push the new quota auto-pause settings straight into the in-memory cache that
	// the OpenAI scheduling hot path reads, so the next request observes the new value
	// without waiting for the background refresher's TTL.
	if s.quotaAutoPauseSink != nil {
		s.quotaAutoPauseSink(cfg.OpenAIAccountQuotaAutoPause)
REDACTED

	// notify cleanup service to reload schedule/enabled.
	if s.cleanupReloader != nil {
		if rerr := s.cleanupReloader.Reload(ctx); rerr != nil {
			logger.LegacyPrintf("service.ops_settings",
				"[OpsSettings] cleanup reload after advanced-settings update failed: %v", rerr)
	REDACTED
REDACTED

	updated := &OpsAdvancedSettings{REDACTED
	_ = json.Unmarshal(raw, updated)
	return updated, nil
REDACTED

// =========================
// Metric thresholds
// =========================

const SettingKeyOpsMetricThresholds = "ops_metric_thresholds"

func defaultOpsMetricThresholds() *OpsMetricThresholds {
	slaMin := 99.5
	ttftMax := 500.0
	reqErrMax := 5.0
	upstreamErrMax := 5.0
	return &OpsMetricThresholds{
		SLAPercentMin:               &slaMin,
		TTFTp99MsMax:                &ttftMax,
		RequestErrorRatePercentMax:  &reqErrMax,
		UpstreamErrorRatePercentMax: &upstreamErrMax,
REDACTED
REDACTED

func (s *OpsService) GetMetricThresholds(ctx context.Context) (*OpsMetricThresholds, error) {
	defaultCfg := defaultOpsMetricThresholds()
	if s == nil || s.settingRepo == nil {
		return defaultCfg, nil
REDACTED
	if ctx == nil {
		ctx = context.Background()
REDACTED

	raw, err := s.settingRepo.GetValue(ctx, SettingKeyOpsMetricThresholds)
	if err != nil {
		if errors.Is(err, ErrSettingNotFound) {
			if b, mErr := json.Marshal(defaultCfg); mErr == nil {
				_ = s.settingRepo.Set(ctx, SettingKeyOpsMetricThresholds, string(b))
		REDACTED
			return defaultCfg, nil
	REDACTED
		return nil, err
REDACTED

	cfg := &OpsMetricThresholds{REDACTED
	if err := json.Unmarshal([]byte(raw), cfg); err != nil {
		return defaultCfg, nil
REDACTED

	return cfg, nil
REDACTED

func (s *OpsService) UpdateMetricThresholds(ctx context.Context, cfg *OpsMetricThresholds) (*OpsMetricThresholds, error) {
	if s == nil || s.settingRepo == nil {
		return nil, errors.New("setting repository not initialized")
REDACTED
	if ctx == nil {
		ctx = context.Background()
REDACTED
	if cfg == nil {
		return nil, errors.New("invalid config")
REDACTED

	// Validate thresholds
	if cfg.SLAPercentMin != nil && (*cfg.SLAPercentMin < 0 || *cfg.SLAPercentMin > 100) {
		return nil, errors.New("sla_percent_min must be between 0 and 100")
REDACTED
	if cfg.TTFTp99MsMax != nil && *cfg.TTFTp99MsMax < 0 {
		return nil, errors.New("ttft_p99_ms_max must be >= 0")
REDACTED
	if cfg.RequestErrorRatePercentMax != nil && (*cfg.RequestErrorRatePercentMax < 0 || *cfg.RequestErrorRatePercentMax > 100) {
		return nil, errors.New("request_error_rate_percent_max must be between 0 and 100")
REDACTED
	if cfg.UpstreamErrorRatePercentMax != nil && (*cfg.UpstreamErrorRatePercentMax < 0 || *cfg.UpstreamErrorRatePercentMax > 100) {
		return nil, errors.New("upstream_error_rate_percent_max must be between 0 and 100")
REDACTED

	raw, err := json.Marshal(cfg)
	if err != nil {
		return nil, err
REDACTED
	if err := s.settingRepo.Set(ctx, SettingKeyOpsMetricThresholds, string(raw)); err != nil {
		return nil, err
REDACTED

	updated := &OpsMetricThresholds{REDACTED
	_ = json.Unmarshal(raw, updated)
	return updated, nil
REDACTED
