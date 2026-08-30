package domain

import "time"

// UpstreamPriceMonitorMode controls whether a completed reconciliation is
// presentation-only or may be handed to the pricing applier. The monitor is
// seeded in observe mode and the service never treats enabled as permission to
// mutate prices.
type UpstreamPriceMonitorMode string

const (
	UpstreamPriceMonitorModeObserve   UpstreamPriceMonitorMode = "observe"
	UpstreamPriceMonitorModeAutoApply UpstreamPriceMonitorMode = "auto_apply"
)

type UpstreamPriceMonitorRunTrigger string

const (
	UpstreamPriceMonitorRunTriggerScheduled UpstreamPriceMonitorRunTrigger = "scheduled"
	UpstreamPriceMonitorRunTriggerManual    UpstreamPriceMonitorRunTrigger = "manual"
	UpstreamPriceMonitorRunTriggerProbe     UpstreamPriceMonitorRunTrigger = "active_probe"
)

type UpstreamPriceMonitorRunStatus string

const (
	UpstreamPriceMonitorRunStatusRunning   UpstreamPriceMonitorRunStatus = "running"
	UpstreamPriceMonitorRunStatusCompleted UpstreamPriceMonitorRunStatus = "completed"
	UpstreamPriceMonitorRunStatusPartial   UpstreamPriceMonitorRunStatus = "partial"
	UpstreamPriceMonitorRunStatusFailed    UpstreamPriceMonitorRunStatus = "failed"
)

type UpstreamPriceEvidenceStatus string

const (
	UpstreamPriceEvidenceStatusTrusted      UpstreamPriceEvidenceStatus = "trusted"
	UpstreamPriceEvidenceStatusPending      UpstreamPriceEvidenceStatus = "pending"
	UpstreamPriceEvidenceStatusMismatch     UpstreamPriceEvidenceStatus = "mismatch"
	UpstreamPriceEvidenceStatusStale        UpstreamPriceEvidenceStatus = "stale"
	UpstreamPriceEvidenceStatusUnobservable UpstreamPriceEvidenceStatus = "unobservable"
)

type UpstreamPriceEvidenceSource string

const (
	UpstreamPriceEvidenceSourceUserRequest UpstreamPriceEvidenceSource = "user_request"
	UpstreamPriceEvidenceSourceActiveProbe UpstreamPriceEvidenceSource = "active_probe"
	UpstreamPriceEvidenceSourcePricePage   UpstreamPriceEvidenceSource = "price_page"
)

type UpstreamPriceReconciliationStatus string

const (
	UpstreamPriceReconciliationBaseline     UpstreamPriceReconciliationStatus = "baseline"
	UpstreamPriceReconciliationMatched      UpstreamPriceReconciliationStatus = "matched"
	UpstreamPriceReconciliationNoActivity   UpstreamPriceReconciliationStatus = "no_activity"
	UpstreamPriceReconciliationMismatch     UpstreamPriceReconciliationStatus = "mismatch"
	UpstreamPriceReconciliationRemoteReset  UpstreamPriceReconciliationStatus = "remote_reset"
	UpstreamPriceReconciliationMixedContext UpstreamPriceReconciliationStatus = "mixed_context"
)

// DefaultX5M5XDomesticModels is the deliberately closed token-model allowlist.
// Matching is case-insensitive, while the canonical upstream spelling is
// preserved in persisted evidence and management responses.
var DefaultX5M5XDomesticModels = []string{
	"deepseek-v4-flash-0731",
	"deepseek-v4-pro-0813",
	"deepseek-v4-flash-vision-exp",
	"kimi-k2.6",
	"kimi-k2.7-code",
	"kimi-k3",
	"glm-5.1",
	"glm-5.2",
	"glm-5.3",
	"glm-5.3-flash",
	"MiniMax-M2.7",
	"MiniMax-M2.7-highspeed",
	"MiniMax-M3",
	"qwen3.7-max",
	"qwen3.8-max",
	"mimo-v2.5",
	"mimo-v2.5-pro",
	"hy3",
}

// UpstreamPriceMonitorConfig is the singleton persisted control-plane
// configuration. AccountIDs refer to existing production accounts; the
// monitor never stores another copy of their API keys.
type UpstreamPriceMonitorConfig struct {
	Enabled                    bool                     `json:"enabled"`
	Mode                       UpstreamPriceMonitorMode `json:"mode"`
	IntervalMinutes            int                      `json:"interval_minutes"`
	Markup                     float64                  `json:"markup"`
	DisplayMultiplierDecimals  int                      `json:"display_multiplier_decimals"`
	AccountIDs                 []int64                  `json:"account_ids"`
	ChannelIDs                 []int64                  `json:"channel_ids"`
	DomesticModels             []string                 `json:"domestic_models"`
	PassiveSampleMaxAgeMinutes int                      `json:"passive_sample_max_age_minutes"`
	ActiveProbeEnabled         bool                     `json:"active_probe_enabled"`
	UpdatedAt                  time.Time                `json:"updated_at"`
}

func DefaultUpstreamPriceMonitorConfig() UpstreamPriceMonitorConfig {
	return UpstreamPriceMonitorConfig{
		Enabled:                    false,
		Mode:                       UpstreamPriceMonitorModeObserve,
		IntervalMinutes:            15,
		Markup:                     1.20,
		DisplayMultiplierDecimals:  3,
		DomesticModels:             append([]string(nil), DefaultX5M5XDomesticModels...),
		PassiveSampleMaxAgeMinutes: 60,
		ActiveProbeEnabled:         false,
	}
}

// UpstreamPriceUsageCounters uses the x5m5x /v1/usage accounting dimensions.
// Cost is deliberately excluded from exact-match equality: the local cost was
// computed from our current price configuration, while ActualCost is the
// upstream truth that the monitor is trying to infer.
type UpstreamPriceUsageCounters struct {
	Requests            int64   `json:"requests"`
	InputTokens         int64   `json:"input_tokens"`
	OutputTokens        int64   `json:"output_tokens"`
	CacheCreationTokens int64   `json:"cache_creation_tokens"`
	CacheReadTokens     int64   `json:"cache_read_tokens"`
	ActualCost          float64 `json:"actual_cost"`
}

type UpstreamPriceRemoteUsageSnapshot struct {
	AccountID  int64                                 `json:"account_id"`
	LedgerDate string                                `json:"ledger_date"`
	CapturedAt time.Time                             `json:"captured_at"`
	Models     map[string]UpstreamPriceUsageCounters `json:"models"`
}

// UpstreamPriceBillingSnapshot is a sanitized copy of
// /v1/sub2api/billing. It contains no key or authorization material.
type UpstreamPriceBillingSnapshot struct {
	ResolvedRateMultiplier  float64   `json:"resolved_rate_multiplier"`
	EffectiveRateMultiplier float64   `json:"effective_rate_multiplier"`
	PeakRateEnabled         bool      `json:"peak_rate_enabled"`
	PeakStart               string    `json:"peak_start,omitempty"`
	PeakEnd                 string    `json:"peak_end,omitempty"`
	PeakRateMultiplier      *float64  `json:"peak_rate_multiplier,omitempty"`
	AppliedPeakMultiplier   float64   `json:"applied_peak_multiplier"`
	Timezone                string    `json:"timezone,omitempty"`
	ObservedAt              time.Time `json:"observed_at"`
}

// UpstreamPriceLocalAggregate is read from committed usage_logs between two
// monotonically increasing ID watermarks for one account and effective
// upstream model.
type UpstreamPriceLocalAggregate struct {
	ModelName            string                     `json:"model_name"`
	Counters             UpstreamPriceUsageCounters `json:"counters"`
	FirstUsageAt         *time.Time                 `json:"first_usage_at,omitempty"`
	LastUsageAt          *time.Time                 `json:"last_usage_at,omitempty"`
	DistinctServiceTiers int                        `json:"distinct_service_tiers"`
	HasSpecialContext    bool                       `json:"has_special_context"`
}

type UpstreamPriceUsageCheckpoint struct {
	AccountID            int64                      `json:"account_id"`
	ModelName            string                     `json:"model_name"`
	AccountIdentityHash  string                     `json:"account_identity_hash"`
	Remote               UpstreamPriceUsageCounters `json:"remote"`
	LedgerDate           string                     `json:"ledger_date"`
	BillingContextHash   string                     `json:"billing_context_hash"`
	LocalUsageLogID      int64                      `json:"local_usage_log_id"`
	CapturedAt           time.Time                  `json:"captured_at"`
	ActiveProbePending   bool                       `json:"active_probe_pending"`
	ActiveProbeStartedAt *time.Time                 `json:"active_probe_started_at,omitempty"`
	Revision             int64                      `json:"revision"`
}

// UpstreamPriceVector stores USD prices per one million tokens, or absolute
// per-request tier prices. Nil means unobserved, never zero.
type UpstreamPriceVector struct {
	InputPerMillion      *float64 `json:"input_per_million,omitempty"`
	OutputPerMillion     *float64 `json:"output_per_million,omitempty"`
	CacheWritePerMillion *float64 `json:"cache_write_per_million,omitempty"`
	CacheReadPerMillion  *float64 `json:"cache_read_per_million,omitempty"`
	PerRequestLTE256K    *float64 `json:"per_request_lte_256k,omitempty"`
	PerRequest256K512K   *float64 `json:"per_request_256k_512k,omitempty"`
	PerRequestGT512K     *float64 `json:"per_request_gt_512k,omitempty"`
}

type UpstreamPriceEvidence struct {
	ID                         int64                             `json:"id"`
	RunID                      int64                             `json:"run_id"`
	AccountID                  int64                             `json:"account_id"`
	ModelName                  string                            `json:"model"`
	BillingMode                string                            `json:"billing_mode"`
	Status                     UpstreamPriceEvidenceStatus       `json:"status"`
	Source                     UpstreamPriceEvidenceSource       `json:"source"`
	ReconciliationStatus       UpstreamPriceReconciliationStatus `json:"reconciliation_status"`
	ContextKey                 string                            `json:"context_key"`
	ObservedAt                 time.Time                         `json:"observed_at"`
	SampleCount                int                               `json:"sample_count"`
	LocalDelta                 UpstreamPriceUsageCounters        `json:"local_delta"`
	RemoteDelta                UpstreamPriceUsageCounters        `json:"remote_delta"`
	Prices                     UpstreamPriceVector               `json:"prices"`
	CurrentPrices              UpstreamPriceVector               `json:"current_prices"`
	SuggestedPrices            UpstreamPriceVector               `json:"suggested_prices"`
	DisplayMultiplierCurrent   *float64                          `json:"display_multiplier_current,omitempty"`
	DisplayMultiplierSuggested *float64                          `json:"display_multiplier_suggested,omitempty"`
	LastError                  string                            `json:"last_error,omitempty"`
	CreatedAt                  time.Time                         `json:"created_at"`
}

type UpstreamPriceObservation struct {
	InputTokens         int64
	OutputTokens        int64
	CacheCreationTokens int64
	CacheReadTokens     int64
	ActualCost          float64
}

type UpstreamPriceMonitorRun struct {
	ID                int64                          `json:"id"`
	Trigger           UpstreamPriceMonitorRunTrigger `json:"trigger"`
	Status            UpstreamPriceMonitorRunStatus  `json:"status"`
	Mode              UpstreamPriceMonitorMode       `json:"mode"`
	DryRun            bool                           `json:"dry_run"`
	StartedAt         time.Time                      `json:"started_at"`
	FinishedAt        *time.Time                     `json:"finished_at,omitempty"`
	MatchedModels     int                            `json:"matched_models"`
	MismatchedModels  int                            `json:"mismatched_models"`
	ProbedModels      int                            `json:"probed_models"`
	ProbeCost         float64                        `json:"probe_cost"`
	SnapshotHash      string                         `json:"snapshot_hash,omitempty"`
	Summary           map[string]any                 `json:"summary,omitempty"`
	Error             string                         `json:"error,omitempty"`
	AppliedAt         *time.Time                     `json:"applied_at,omitempty"`
	RollbackAvailable bool                           `json:"rollback_available"`
}

type UpstreamPriceMonitorRunPage struct {
	Items []UpstreamPriceMonitorRun `json:"items"`
	Total int64                     `json:"total"`
}

type UpstreamPriceMonitorRuntime struct {
	Status              string     `json:"status"`
	LastRunAt           *time.Time `json:"last_run_at,omitempty"`
	NextRunAt           *time.Time `json:"next_run_at,omitempty"`
	ConsecutiveFailures int        `json:"consecutive_failures"`
	LastError           string     `json:"last_error,omitempty"`
	TodayProbeCost      float64    `json:"today_probe_cost"`
	Coverage            struct {
		Trusted int `json:"trusted"`
		Total   int `json:"total"`
	} `json:"coverage"`
}

type UpstreamPriceModelStatus string

const (
	UpstreamPriceModelStatusManaged          UpstreamPriceModelStatus = "managed"
	UpstreamPriceModelStatusDiscovered       UpstreamPriceModelStatus = "discovered"
	UpstreamPriceModelStatusSuspectedRetired UpstreamPriceModelStatus = "suspected_retired"
	UpstreamPriceModelStatusIgnored          UpstreamPriceModelStatus = "ignored"
	UpstreamPriceModelStatusRetired          UpstreamPriceModelStatus = "retired"
)

type UpstreamPriceDiscoveredModel struct {
	ModelName         string `json:"model"`
	SeenAccountCount  int    `json:"seen_account_count"`
	DomesticCandidate bool   `json:"domestic_candidate"`
}

type UpstreamPriceModelCatalogEntry struct {
	ModelName            string                   `json:"model"`
	Status               UpstreamPriceModelStatus `json:"status"`
	DomesticCandidate    bool                     `json:"domestic_candidate"`
	SeenAccountCount     int                      `json:"seen_account_count"`
	ExpectedAccountCount int                      `json:"expected_account_count"`
	MissingRuns          int                      `json:"missing_runs"`
	DiscoveryComplete    bool                     `json:"discovery_complete"`
	FirstSeenAt          *time.Time               `json:"first_seen_at,omitempty"`
	LastSeenAt           *time.Time               `json:"last_seen_at,omitempty"`
	LastMissingAt        *time.Time               `json:"last_missing_at,omitempty"`
	UpdatedAt            time.Time                `json:"updated_at"`
}
