package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
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
	}
	if ctx == nil {
		ctx = context.Background()
	}

	raw, err := s.settingRepo.GetValue(ctx, SettingKeyOpsEmailNotificationConfig)
	if err != nil {
		if errors.Is(err, ErrSettingNotFound) {
			// Initialize defaults on first read (best-effort).
			if b, mErr := json.Marshal(defaultCfg); mErr == nil {
				_ = s.settingRepo.Set(ctx, SettingKeyOpsEmailNotificationConfig, string(b))
			}
			return defaultCfg, nil
		}
		return nil, err
	}

	cfg := &OpsEmailNotificationConfig{}
	if err := json.Unmarshal([]byte(raw), cfg); err != nil {
		// Corrupted JSON should not break ops UI; fall back to defaults.
		return defaultCfg, nil
	}
	normalizeOpsEmailNotificationConfig(cfg)
	return cfg, nil
}

func (s *OpsService) UpdateEmailNotificationConfig(ctx context.Context, req *OpsEmailNotificationConfigUpdateRequest) (*OpsEmailNotificationConfig, error) {
	if s == nil || s.settingRepo == nil {
		return nil, errors.New("setting repository not initialized")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if req == nil {
		return nil, errors.New("invalid request")
	}

	cfg, err := s.GetEmailNotificationConfig(ctx)
	if err != nil {
		return nil, err
	}

	if req.Alert != nil {
		cfg.Alert.Enabled = req.Alert.Enabled
		if req.Alert.Recipients != nil {
			cfg.Alert.Recipients = req.Alert.Recipients
		}
		cfg.Alert.MinSeverity = strings.TrimSpace(req.Alert.MinSeverity)
		cfg.Alert.RateLimitPerHour = req.Alert.RateLimitPerHour
		cfg.Alert.BatchingWindowSeconds = req.Alert.BatchingWindowSeconds
		cfg.Alert.IncludeResolvedAlerts = req.Alert.IncludeResolvedAlerts
	}

	if req.Report != nil {
		cfg.Report.Enabled = req.Report.Enabled
		if req.Report.Recipients != nil {
			cfg.Report.Recipients = req.Report.Recipients
		}
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
	}

	if err := validateOpsEmailNotificationConfig(cfg); err != nil {
		return nil, err
	}

	normalizeOpsEmailNotificationConfig(cfg)
	raw, err := json.Marshal(cfg)
	if err != nil {
		return nil, err
	}
	if err := s.settingRepo.Set(ctx, SettingKeyOpsEmailNotificationConfig, string(raw)); err != nil {
		return nil, err
	}
	return cfg, nil
}

func defaultOpsEmailNotificationConfig() *OpsEmailNotificationConfig {
	return &OpsEmailNotificationConfig{
		Alert: OpsEmailAlertConfig{
			Enabled:               true,
			Recipients:            []string{},
			MinSeverity:           "",
			RateLimitPerHour:      0,
			BatchingWindowSeconds: 0,
			IncludeResolvedAlerts: false,
		},
		Report: OpsEmailReportConfig{
			Enabled:                         false,
			Recipients:                      []string{},
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
		},
	}
}

func normalizeOpsEmailNotificationConfig(cfg *OpsEmailNotificationConfig) {
	if cfg == nil {
		return
	}
	if cfg.Alert.Recipients == nil {
		cfg.Alert.Recipients = []string{}
	}
	if cfg.Report.Recipients == nil {
		cfg.Report.Recipients = []string{}
	}

	cfg.Alert.MinSeverity = strings.TrimSpace(cfg.Alert.MinSeverity)
	cfg.Report.DailySummarySchedule = strings.TrimSpace(cfg.Report.DailySummarySchedule)
	cfg.Report.WeeklySummarySchedule = strings.TrimSpace(cfg.Report.WeeklySummarySchedule)
	cfg.Report.ErrorDigestSchedule = strings.TrimSpace(cfg.Report.ErrorDigestSchedule)
	cfg.Report.AccountHealthSchedule = strings.TrimSpace(cfg.Report.AccountHealthSchedule)

	// Fill missing schedules with defaults to avoid breaking cron logic if clients send empty strings.
	if cfg.Report.DailySummarySchedule == "" {
		cfg.Report.DailySummarySchedule = "0 9 * * *"
	}
	if cfg.Report.WeeklySummarySchedule == "" {
		cfg.Report.WeeklySummarySchedule = "0 9 * * 1"
	}
	if cfg.Report.ErrorDigestSchedule == "" {
		cfg.Report.ErrorDigestSchedule = "0 9 * * *"
	}
	if cfg.Report.AccountHealthSchedule == "" {
		cfg.Report.AccountHealthSchedule = "0 9 * * *"
	}
}

func opsEmailBehaviorSettings(cfg *OpsEmailNotificationConfig) OpsEmailBehaviorSettings {
	if cfg == nil {
		cfg = defaultOpsEmailNotificationConfig()
	}
	return OpsEmailBehaviorSettings{
		Alert: OpsEmailAlertBehaviorSettings{
			MinSeverity:           cfg.Alert.MinSeverity,
			RateLimitPerHour:      cfg.Alert.RateLimitPerHour,
			BatchingWindowSeconds: cfg.Alert.BatchingWindowSeconds,
			IncludeResolvedAlerts: cfg.Alert.IncludeResolvedAlerts,
		},
		Report: OpsEmailReportBehaviorSettings{
			DailySummaryEnabled:             cfg.Report.DailySummaryEnabled,
			DailySummarySchedule:            cfg.Report.DailySummarySchedule,
			WeeklySummaryEnabled:            cfg.Report.WeeklySummaryEnabled,
			WeeklySummarySchedule:           cfg.Report.WeeklySummarySchedule,
			ErrorDigestEnabled:              cfg.Report.ErrorDigestEnabled,
			ErrorDigestSchedule:             cfg.Report.ErrorDigestSchedule,
			ErrorDigestMinCount:             cfg.Report.ErrorDigestMinCount,
			AccountHealthEnabled:            cfg.Report.AccountHealthEnabled,
			AccountHealthSchedule:           cfg.Report.AccountHealthSchedule,
			AccountHealthErrorRateThreshold: cfg.Report.AccountHealthErrorRateThreshold,
		},
	}
}

func applyOpsEmailBehaviorSettings(cfg *OpsEmailNotificationConfig, behavior OpsEmailBehaviorSettings) {
	if cfg == nil {
		return
	}
	cfg.Alert.MinSeverity = behavior.Alert.MinSeverity
	cfg.Alert.RateLimitPerHour = behavior.Alert.RateLimitPerHour
	cfg.Alert.BatchingWindowSeconds = behavior.Alert.BatchingWindowSeconds
	cfg.Alert.IncludeResolvedAlerts = behavior.Alert.IncludeResolvedAlerts
	cfg.Report.DailySummaryEnabled = behavior.Report.DailySummaryEnabled
	cfg.Report.DailySummarySchedule = behavior.Report.DailySummarySchedule
	cfg.Report.WeeklySummaryEnabled = behavior.Report.WeeklySummaryEnabled
	cfg.Report.WeeklySummarySchedule = behavior.Report.WeeklySummarySchedule
	cfg.Report.ErrorDigestEnabled = behavior.Report.ErrorDigestEnabled
	cfg.Report.ErrorDigestSchedule = behavior.Report.ErrorDigestSchedule
	cfg.Report.ErrorDigestMinCount = behavior.Report.ErrorDigestMinCount
	cfg.Report.AccountHealthEnabled = behavior.Report.AccountHealthEnabled
	cfg.Report.AccountHealthSchedule = behavior.Report.AccountHealthSchedule
	cfg.Report.AccountHealthErrorRateThreshold = behavior.Report.AccountHealthErrorRateThreshold
}

func validateOpsEmailNotificationConfig(cfg *OpsEmailNotificationConfig) error {
	if cfg == nil {
		return errors.New("invalid config")
	}

	if cfg.Alert.RateLimitPerHour < 0 || cfg.Alert.RateLimitPerHour > 10000 {
		return errors.New("alert.rate_limit_per_hour must be between 0 and 10000")
	}
	if cfg.Alert.BatchingWindowSeconds < 0 || cfg.Alert.BatchingWindowSeconds > 86400 {
		return errors.New("alert.batching_window_seconds must be between 0 and 86400")
	}
	switch strings.TrimSpace(cfg.Alert.MinSeverity) {
	case "", "critical", "warning", "info":
	default:
		return errors.New("alert.min_severity must be one of: critical, warning, info, or empty")
	}

	if cfg.Report.ErrorDigestMinCount < 0 {
		return errors.New("report.error_digest_min_count must be >= 0")
	}
	if cfg.Report.AccountHealthErrorRateThreshold < 0 || cfg.Report.AccountHealthErrorRateThreshold > 100 {
		return errors.New("report.account_health_error_rate_threshold must be between 0 and 100")
	}
	schedules := []struct {
		enabled bool
		name    string
		spec    string
	}{
		{cfg.Report.DailySummaryEnabled, "report.daily_summary_schedule", cfg.Report.DailySummarySchedule},
		{cfg.Report.WeeklySummaryEnabled, "report.weekly_summary_schedule", cfg.Report.WeeklySummarySchedule},
		{cfg.Report.ErrorDigestEnabled, "report.error_digest_schedule", cfg.Report.ErrorDigestSchedule},
		{cfg.Report.AccountHealthEnabled, "report.account_health_schedule", cfg.Report.AccountHealthSchedule},
	}
	for _, schedule := range schedules {
		if !schedule.enabled {
			continue
		}
		if _, err := opsScheduledReportCronParser.Parse(strings.TrimSpace(schedule.spec)); err != nil {
			return fmt.Errorf("%s is invalid: %w", schedule.name, err)
		}
	}
	return nil
}

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
		},
		Silencing: OpsAlertSilencingSettings{
			Enabled:            false,
			GlobalUntilRFC3339: "",
			GlobalReason:       "",
			Entries:            []OpsAlertSilenceEntry{},
		},
	}
}

func normalizeOpsDistributedLockSettings(s *OpsDistributedLockSettings, defaultKey string, defaultTTLSeconds int) {
	if s == nil {
		return
	}
	s.Key = strings.TrimSpace(s.Key)
	if s.Key == "" {
		s.Key = defaultKey
	}
	if s.TTLSeconds <= 0 {
		s.TTLSeconds = defaultTTLSeconds
	}
}

func normalizeOpsAlertSilencingSettings(s *OpsAlertSilencingSettings) {
	if s == nil {
		return
	}
	s.GlobalUntilRFC3339 = strings.TrimSpace(s.GlobalUntilRFC3339)
	s.GlobalReason = strings.TrimSpace(s.GlobalReason)
	if s.Entries == nil {
		s.Entries = []OpsAlertSilenceEntry{}
	}
	for i := range s.Entries {
		s.Entries[i].UntilRFC3339 = strings.TrimSpace(s.Entries[i].UntilRFC3339)
		s.Entries[i].Reason = strings.TrimSpace(s.Entries[i].Reason)
	}
}

func validateOpsDistributedLockSettings(s OpsDistributedLockSettings) error {
	if strings.TrimSpace(s.Key) == "" {
		return errors.New("distributed_lock.key is required")
	}
	if s.TTLSeconds <= 0 || s.TTLSeconds > int((24*time.Hour).Seconds()) {
		return errors.New("distributed_lock.ttl_seconds must be between 1 and 86400")
	}
	return nil
}

func validateOpsAlertSilencingSettings(s OpsAlertSilencingSettings) error {
	parse := func(raw string) error {
		if strings.TrimSpace(raw) == "" {
			return nil
		}
		if _, err := time.Parse(time.RFC3339, raw); err != nil {
			return errors.New("silencing time must be RFC3339")
		}
		return nil
	}

	if err := parse(s.GlobalUntilRFC3339); err != nil {
		return err
	}
	for _, entry := range s.Entries {
		if strings.TrimSpace(entry.UntilRFC3339) == "" {
			return errors.New("silencing.entries.until_rfc3339 is required")
		}
		if _, err := time.Parse(time.RFC3339, entry.UntilRFC3339); err != nil {
			return errors.New("silencing.entries.until_rfc3339 must be RFC3339")
		}
	}
	return nil
}

func (s *OpsService) GetOpsAlertRuntimeSettings(ctx context.Context) (*OpsAlertRuntimeSettings, error) {
	defaultCfg := defaultOpsAlertRuntimeSettings()
	if s == nil || s.settingRepo == nil {
		return defaultCfg, nil
	}
	if ctx == nil {
		ctx = context.Background()
	}

	raw, err := s.settingRepo.GetValue(ctx, SettingKeyOpsAlertRuntimeSettings)
	if err != nil {
		if errors.Is(err, ErrSettingNotFound) {
			if b, mErr := json.Marshal(defaultCfg); mErr == nil {
				_ = s.settingRepo.Set(ctx, SettingKeyOpsAlertRuntimeSettings, string(b))
			}
			return defaultCfg, nil
		}
		return nil, err
	}

	cfg := &OpsAlertRuntimeSettings{}
	if err := json.Unmarshal([]byte(raw), cfg); err != nil {
		return defaultCfg, nil
	}

	if cfg.EvaluationIntervalSeconds <= 0 {
		cfg.EvaluationIntervalSeconds = defaultCfg.EvaluationIntervalSeconds
	}
	normalizeOpsDistributedLockSettings(&cfg.DistributedLock, opsAlertEvaluatorLeaderLockKeyDefault, defaultCfg.DistributedLock.TTLSeconds)
	normalizeOpsAlertSilencingSettings(&cfg.Silencing)

	return cfg, nil
}

func (s *OpsService) UpdateOpsAlertRuntimeSettings(ctx context.Context, cfg *OpsAlertRuntimeSettings) (*OpsAlertRuntimeSettings, error) {
	if s == nil || s.settingRepo == nil {
		return nil, errors.New("setting repository not initialized")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if cfg == nil {
		return nil, errors.New("invalid config")
	}

	if err := validateOpsAlertRuntimeSettings(cfg); err != nil {
		return nil, err
	}

	defaultCfg := defaultOpsAlertRuntimeSettings()
	normalizeOpsDistributedLockSettings(&cfg.DistributedLock, opsAlertEvaluatorLeaderLockKeyDefault, defaultCfg.DistributedLock.TTLSeconds)
	normalizeOpsAlertSilencingSettings(&cfg.Silencing)

	raw, err := json.Marshal(cfg)
	if err != nil {
		return nil, err
	}
	if err := s.settingRepo.Set(ctx, SettingKeyOpsAlertRuntimeSettings, string(raw)); err != nil {
		return nil, err
	}

	// Return a fresh copy (avoid callers holding pointers into internal slices that may be mutated).
	updated := &OpsAlertRuntimeSettings{}
	_ = json.Unmarshal(raw, updated)
	return updated, nil
}

func validateOpsAlertRuntimeSettings(cfg *OpsAlertRuntimeSettings) error {
	if cfg == nil {
		return errors.New("invalid config")
	}
	if cfg.EvaluationIntervalSeconds < 1 || cfg.EvaluationIntervalSeconds > int((24*time.Hour).Seconds()) {
		return errors.New("evaluation_interval_seconds must be between 1 and 86400")
	}
	if cfg.DistributedLock.Enabled {
		if err := validateOpsDistributedLockSettings(cfg.DistributedLock); err != nil {
			return err
		}
	}
	if cfg.Silencing.Enabled {
		if err := validateOpsAlertSilencingSettings(cfg.Silencing); err != nil {
			return err
		}
	}
	return nil
}

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
		},
		Aggregation: OpsAggregationSettings{
			AggregationEnabled: false,
		},
		OpenAIAccountQuotaAutoPause:     OpsOpenAIAccountQuotaAutoPauseSettings{},
		IgnoreCountTokensErrors:         true,  // count_tokens 404 是预期行为，默认忽略
		IgnoreContextCanceled:           true,  // Default to true - client disconnects are not errors
		IgnoreNoAvailableAccounts:       false, // Default to false - this is a real routing issue
		IgnoreInvalidApiKeyErrors:       true,  // Legacy compatibility field; admission rejects are always excluded.
		IgnoreInsufficientBalanceErrors: false, // 默认不忽略，余额不足可能需要关注
		DisplayOpenAITokenStats:         false,
		DisplayAlertEvents:              true,
		AutoRefreshEnabled:              false,
		AutoRefreshIntervalSec:          30,
	}
}

func normalizeOpsAdvancedSettings(cfg *OpsAdvancedSettings) {
	if cfg == nil {
		return
	}
	// Admission rejects are a security/traffic concern, not an operational
	// request-error category. Keep the legacy field true for old clients but do
	// not allow it to re-enable those rows.
	cfg.IgnoreInvalidApiKeyErrors = true
	cfg.OpenAIAccountQuotaAutoPause.DefaultThreshold5h = clampOpsQuotaAutoPauseThreshold(cfg.OpenAIAccountQuotaAutoPause.DefaultThreshold5h)
	cfg.OpenAIAccountQuotaAutoPause.DefaultThreshold7d = clampOpsQuotaAutoPauseThreshold(cfg.OpenAIAccountQuotaAutoPause.DefaultThreshold7d)
	cfg.DataRetention.CleanupSchedule = strings.TrimSpace(cfg.DataRetention.CleanupSchedule)
	if cfg.DataRetention.CleanupSchedule == "" {
		cfg.DataRetention.CleanupSchedule = opsCleanupDefaultSchedule
	}
	// 保留天数：0 表示每次定时清理全部（清空所有），> 0 表示按天数保留；
	// 仅在拿到非法的负数时回填默认值，避免覆盖用户主动设的 0。
	if cfg.DataRetention.ErrorLogRetentionDays < 0 {
		cfg.DataRetention.ErrorLogRetentionDays = 30
	}
	if cfg.DataRetention.MinuteMetricsRetentionDays < 0 {
		cfg.DataRetention.MinuteMetricsRetentionDays = 30
	}
	if cfg.DataRetention.HourlyMetricsRetentionDays < 0 {
		cfg.DataRetention.HourlyMetricsRetentionDays = 30
	}
	// Normalize auto refresh interval (default 30 seconds)
	if cfg.AutoRefreshIntervalSec <= 0 {
		cfg.AutoRefreshIntervalSec = 30
	}
}

func clampOpsQuotaAutoPauseThreshold(value float64) float64 {
	if value <= 0 {
		return 0
	}
	if value > 1 {
		return 1
	}
	return value
}

func validateOpsAdvancedSettings(cfg *OpsAdvancedSettings) error {
	if cfg == nil {
		return errors.New("invalid config")
	}
	// 保留天数：0 表示每次清理全部，1-365 表示按天数保留。
	if cfg.DataRetention.ErrorLogRetentionDays < 0 || cfg.DataRetention.ErrorLogRetentionDays > 365 {
		return errors.New("error_log_retention_days must be between 0 and 365")
	}
	if cfg.DataRetention.MinuteMetricsRetentionDays < 0 || cfg.DataRetention.MinuteMetricsRetentionDays > 365 {
		return errors.New("minute_metrics_retention_days must be between 0 and 365")
	}
	if cfg.DataRetention.HourlyMetricsRetentionDays < 0 || cfg.DataRetention.HourlyMetricsRetentionDays > 365 {
		return errors.New("hourly_metrics_retention_days must be between 0 and 365")
	}
	if cfg.AutoRefreshIntervalSec < 15 || cfg.AutoRefreshIntervalSec > 300 {
		return errors.New("auto_refresh_interval_seconds must be between 15 and 300")
	}
	return nil
}

func (s *OpsService) GetOpsAdvancedSettings(ctx context.Context) (*OpsAdvancedSettings, error) {
	_ = ctx
	cfg := s.OpsAdvancedSettingsSnapshot()
	return &cfg, nil
}

// OpsAdvancedSettingsSnapshot returns a value copy for request hot paths. It
// avoids both repository I/O and pointer escape/allocation.
func (s *OpsService) OpsAdvancedSettingsSnapshot() OpsAdvancedSettings {
	if s != nil {
		if snapshot := s.runtimeSettings.Load(); snapshot != nil {
			return snapshot.advanced
		}
	}
	return *defaultOpsAdvancedSettings()
}

func (s *OpsService) UpdateOpsAdvancedSettings(ctx context.Context, cfg *OpsAdvancedSettings) (*OpsAdvancedSettings, error) {
	if s == nil || s.settingRepo == nil {
		return nil, errors.New("setting repository not initialized")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if cfg == nil {
		return nil, errors.New("invalid config")
	}

	if err := validateOpsAdvancedSettings(cfg); err != nil {
		return nil, err
	}

	normalizeOpsAdvancedSettings(cfg)
	raw, err := json.Marshal(cfg)
	if err != nil {
		return nil, err
	}
	if err := s.settingRepo.Set(ctx, SettingKeyOpsAdvancedSettings, string(raw)); err != nil {
		return nil, err
	}
	s.storeAdvancedSettingsSnapshot(cfg)
	// Push the new quota auto-pause settings straight into the in-memory cache that
	// the OpenAI scheduling hot path reads, so the next request observes the new value
	// without waiting for the background refresher's TTL.
	if s.quotaAutoPauseSink != nil {
		s.quotaAutoPauseSink(cfg.OpenAIAccountQuotaAutoPause)
	}

	// notify cleanup service to reload schedule/enabled.
	if s.cleanupReloader != nil {
		if rerr := s.cleanupReloader.Reload(ctx); rerr != nil {
			logger.LegacyPrintf("service.ops_settings",
				"[OpsSettings] cleanup reload after advanced-settings update failed: %v", rerr)
		}
	}

	updated := &OpsAdvancedSettings{}
	_ = json.Unmarshal(raw, updated)
	return updated, nil
}

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
	}
}

func (s *OpsService) GetMetricThresholds(ctx context.Context) (*OpsMetricThresholds, error) {
	defaultCfg := defaultOpsMetricThresholds()
	if s == nil || s.settingRepo == nil {
		return defaultCfg, nil
	}
	if ctx == nil {
		ctx = context.Background()
	}

	raw, err := s.settingRepo.GetValue(ctx, SettingKeyOpsMetricThresholds)
	if err != nil {
		if errors.Is(err, ErrSettingNotFound) {
			if b, mErr := json.Marshal(defaultCfg); mErr == nil {
				_ = s.settingRepo.Set(ctx, SettingKeyOpsMetricThresholds, string(b))
			}
			return defaultCfg, nil
		}
		return nil, err
	}

	cfg := &OpsMetricThresholds{}
	if err := json.Unmarshal([]byte(raw), cfg); err != nil {
		return defaultCfg, nil
	}
	if err := validateOpsMetricThresholds(cfg); err != nil {
		return defaultCfg, nil
	}

	return cfg, nil
}

func (s *OpsService) UpdateMetricThresholds(ctx context.Context, cfg *OpsMetricThresholds) (*OpsMetricThresholds, error) {
	if s == nil || s.settingRepo == nil {
		return nil, errors.New("setting repository not initialized")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if cfg == nil {
		return nil, errors.New("invalid config")
	}

	if err := validateOpsMetricThresholds(cfg); err != nil {
		return nil, err
	}

	raw, err := json.Marshal(cfg)
	if err != nil {
		return nil, err
	}
	if err := s.settingRepo.Set(ctx, SettingKeyOpsMetricThresholds, string(raw)); err != nil {
		return nil, err
	}

	updated := &OpsMetricThresholds{}
	_ = json.Unmarshal(raw, updated)
	return updated, nil
}

func validateOpsMetricThresholds(cfg *OpsMetricThresholds) error {
	if cfg == nil {
		return errors.New("invalid config")
	}
	if cfg.SLAPercentMin != nil && (*cfg.SLAPercentMin <= 0 || *cfg.SLAPercentMin > 100) {
		return errors.New("sla_percent_min must be greater than 0 and at most 100")
	}
	if cfg.TTFTp99MsMax != nil && *cfg.TTFTp99MsMax <= 0 {
		return errors.New("ttft_p99_ms_max must be greater than 0")
	}
	if cfg.RequestErrorRatePercentMax != nil && (*cfg.RequestErrorRatePercentMax <= 0 || *cfg.RequestErrorRatePercentMax > 100) {
		return errors.New("request_error_rate_percent_max must be greater than 0 and at most 100")
	}
	if cfg.UpstreamErrorRatePercentMax != nil && (*cfg.UpstreamErrorRatePercentMax <= 0 || *cfg.UpstreamErrorRatePercentMax > 100) {
		return errors.New("upstream_error_rate_percent_max must be greater than 0 and at most 100")
	}
	return nil
}

func (s *OpsService) GetOpsMonitoringSettings(ctx context.Context) (*OpsMonitoringSettings, error) {
	if s == nil || s.settingRepo == nil {
		return nil, errors.New("setting repository not initialized")
	}
	runtimeCfg, err := s.GetOpsAlertRuntimeSettings(ctx)
	if err != nil {
		return nil, err
	}
	emailCfg, err := s.GetEmailNotificationConfig(ctx)
	if err != nil {
		return nil, err
	}
	advancedCfg, err := s.GetOpsAdvancedSettings(ctx)
	if err != nil {
		return nil, err
	}
	thresholds, err := s.GetMetricThresholds(ctx)
	if err != nil {
		return nil, err
	}
	return &OpsMonitoringSettings{
		Runtime:          *runtimeCfg,
		EmailBehavior:    opsEmailBehaviorSettings(emailCfg),
		Advanced:         *advancedCfg,
		MetricThresholds: *thresholds,
		ScheduleInfo:     s.opsScheduleInfo(emailCfg, time.Now()),
	}, nil
}

func (s *OpsService) UpdateOpsMonitoringSettings(ctx context.Context, req *OpsMonitoringSettings) (*OpsMonitoringSettings, error) {
	if s == nil || s.settingRepo == nil {
		return nil, errors.New("setting repository not initialized")
	}
	if req == nil {
		return nil, errors.New("invalid config")
	}
	if ctx == nil {
		ctx = context.Background()
	}

	runtimeCfg := req.Runtime
	advancedCfg := req.Advanced
	thresholds := req.MetricThresholds
	emailCfg, err := s.GetEmailNotificationConfig(ctx)
	if err != nil {
		return nil, err
	}
	applyOpsEmailBehaviorSettings(emailCfg, req.EmailBehavior)

	if err := validateOpsAlertRuntimeSettings(&runtimeCfg); err != nil {
		return nil, err
	}
	if err := validateOpsEmailNotificationConfig(emailCfg); err != nil {
		return nil, err
	}
	if err := validateOpsAdvancedSettings(&advancedCfg); err != nil {
		return nil, err
	}
	if err := validateOpsMetricThresholds(&thresholds); err != nil {
		return nil, err
	}

	defaultRuntime := defaultOpsAlertRuntimeSettings()
	normalizeOpsDistributedLockSettings(&runtimeCfg.DistributedLock, opsAlertEvaluatorLeaderLockKeyDefault, defaultRuntime.DistributedLock.TTLSeconds)
	normalizeOpsAlertSilencingSettings(&runtimeCfg.Silencing)
	normalizeOpsEmailNotificationConfig(emailCfg)
	normalizeOpsAdvancedSettings(&advancedCfg)

	runtimeRaw, err := json.Marshal(&runtimeCfg)
	if err != nil {
		return nil, err
	}
	emailRaw, err := json.Marshal(emailCfg)
	if err != nil {
		return nil, err
	}
	advancedRaw, err := json.Marshal(&advancedCfg)
	if err != nil {
		return nil, err
	}
	thresholdsRaw, err := json.Marshal(&thresholds)
	if err != nil {
		return nil, err
	}
	if err := s.settingRepo.SetMultiple(ctx, map[string]string{
		SettingKeyOpsAlertRuntimeSettings:    string(runtimeRaw),
		SettingKeyOpsEmailNotificationConfig: string(emailRaw),
		SettingKeyOpsAdvancedSettings:        string(advancedRaw),
		SettingKeyOpsMetricThresholds:        string(thresholdsRaw),
	}); err != nil {
		return nil, err
	}

	s.storeAdvancedSettingsSnapshot(&advancedCfg)
	if s.quotaAutoPauseSink != nil {
		s.quotaAutoPauseSink(advancedCfg.OpenAIAccountQuotaAutoPause)
	}
	if s.cleanupReloader != nil {
		if reloadErr := s.cleanupReloader.Reload(ctx); reloadErr != nil {
			logger.LegacyPrintf("service.ops_settings",
				"[OpsSettings] cleanup reload after atomic settings update failed: %v", reloadErr)
		}
	}

	return &OpsMonitoringSettings{
		Runtime:          runtimeCfg,
		EmailBehavior:    opsEmailBehaviorSettings(emailCfg),
		Advanced:         advancedCfg,
		MetricThresholds: thresholds,
		ScheduleInfo:     s.opsScheduleInfo(emailCfg, time.Now()),
	}, nil
}

func (s *OpsService) opsScheduleInfo(cfg *OpsEmailNotificationConfig, now time.Time) OpsScheduleInfo {
	loc := time.Local
	timezone := loc.String()
	if s != nil && s.cfg != nil && strings.TrimSpace(s.cfg.Timezone) != "" {
		candidate := strings.TrimSpace(s.cfg.Timezone)
		if parsed, err := time.LoadLocation(candidate); err == nil {
			timezone = candidate
			loc = parsed
		}
	}
	if timezone == "" || timezone == "Local" {
		timezone = loc.String()
	}
	info := OpsScheduleInfo{Timezone: timezone, NextRuns: map[string]time.Time{}}
	if cfg == nil {
		return info
	}
	defs := []struct {
		enabled bool
		name    string
		spec    string
	}{
		{cfg.Report.DailySummaryEnabled, "daily_summary", cfg.Report.DailySummarySchedule},
		{cfg.Report.WeeklySummaryEnabled, "weekly_summary", cfg.Report.WeeklySummarySchedule},
		{cfg.Report.ErrorDigestEnabled, "error_digest", cfg.Report.ErrorDigestSchedule},
		{cfg.Report.AccountHealthEnabled, "account_health", cfg.Report.AccountHealthSchedule},
	}
	base := now.In(loc)
	for _, def := range defs {
		if !def.enabled {
			continue
		}
		schedule, err := opsScheduledReportCronParser.Parse(strings.TrimSpace(def.spec))
		if err != nil {
			continue
		}
		if next := schedule.Next(base); !next.IsZero() {
			info.NextRuns[def.name] = next
		}
	}
	return info
}
