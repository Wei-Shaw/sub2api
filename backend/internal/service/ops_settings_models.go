package service

// Ops settings models stored in DB `settings` table (JSON blobs).

type OpsEmailNotificationConfig struct {
	Alert  OpsEmailAlertConfig  `json:"alert"`
	Report OpsEmailReportConfig `json:"report"`
REDACTED

type OpsEmailAlertConfig struct {
	Enabled               bool     `json:"enabled"`
	Recipients            []string `json:"recipients"`
	MinSeverity           string   `json:"min_severity"`
	RateLimitPerHour      int      `json:"rate_limit_per_hour"`
	BatchingWindowSeconds int      `json:"batching_window_seconds"`
	IncludeResolvedAlerts bool     `json:"include_resolved_alerts"`
REDACTED

type OpsEmailReportConfig struct {
	Enabled                         bool     `json:"enabled"`
	Recipients                      []string `json:"recipients"`
	DailySummaryEnabled             bool     `json:"daily_summary_enabled"`
	DailySummarySchedule            string   `json:"daily_summary_schedule"`
	WeeklySummaryEnabled            bool     `json:"weekly_summary_enabled"`
	WeeklySummarySchedule           string   `json:"weekly_summary_schedule"`
	ErrorDigestEnabled              bool     `json:"error_digest_enabled"`
	ErrorDigestSchedule             string   `json:"error_digest_schedule"`
	ErrorDigestMinCount             int      `json:"error_digest_min_count"`
	AccountHealthEnabled            bool     `json:"account_health_enabled"`
	AccountHealthSchedule           string   `json:"account_health_schedule"`
	AccountHealthErrorRateThreshold float64  `json:"account_health_error_rate_threshold"`
REDACTED

// OpsEmailNotificationConfigUpdateRequest allows partial updates, while the
// frontend can still send the full config shape.
type OpsEmailNotificationConfigUpdateRequest struct {
	Alert  *OpsEmailAlertConfig  `json:"alert"`
	Report *OpsEmailReportConfig `json:"report"`
REDACTED

type OpsDistributedLockSettings struct {
	Enabled    bool   `json:"enabled"`
	Key        string `json:"key"`
	TTLSeconds int    `json:"ttl_seconds"`
REDACTED

type OpsAlertSilenceEntry struct {
	RuleID     *int64   `json:"rule_id,omitempty"`
	Severities []string `json:"severities,omitempty"`

	UntilRFC3339 string `json:"until_rfc3339"`
	Reason       string `json:"reason"`
REDACTED

type OpsAlertSilencingSettings struct {
	Enabled bool `json:"enabled"`

	GlobalUntilRFC3339 string `json:"global_until_rfc3339"`
	GlobalReason       string `json:"global_reason"`

	Entries []OpsAlertSilenceEntry `json:"entries,omitempty"`
REDACTED

type OpsAlertRuntimeSettings struct {
	EvaluationIntervalSeconds int `json:"evaluation_interval_seconds"`

	DistributedLock OpsDistributedLockSettings `json:"distributed_lock"`
	Silencing       OpsAlertSilencingSettings  `json:"silencing"`
REDACTED

