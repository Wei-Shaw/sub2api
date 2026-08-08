package service

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"
)

const (
	ChannelMonitorV2OtherModel      = "__other__"
	ChannelMonitorV2TaxonomyVersion = 1
)

var (
	ErrChannelMonitorV2InvalidRange   = errors.New("invalid channel monitor v2 range")
	ErrChannelMonitorV2InvalidGroupBy = errors.New("invalid channel monitor v2 group_by")
	ErrChannelMonitorV2InvalidConfig  = errors.New("invalid channel monitor v2 config")
	ErrChannelMonitorV2ConfigConflict = errors.New("channel monitor v2 config was modified")
)

type ChannelMonitorV2GroupBy string

const (
	ChannelMonitorV2GroupByPlatform           ChannelMonitorV2GroupBy = "platform"
	ChannelMonitorV2GroupByPlatformGroup      ChannelMonitorV2GroupBy = "platform_group"
	ChannelMonitorV2GroupByPlatformModel      ChannelMonitorV2GroupBy = "platform_model"
	ChannelMonitorV2GroupByPlatformGroupModel ChannelMonitorV2GroupBy = "platform_group_model"
)

type ChannelMonitorV2PlatformConfig struct {
	Platform string   `json:"platform"`
	Enabled  bool     `json:"enabled"`
	Models   []string `json:"models"`
REDACTED

type ChannelMonitorV2Config struct {
	Version                int                              `json:"version"`
	Enabled                bool                             `json:"enabled"`
	RefreshIntervalSeconds int                              `json:"refresh_interval_seconds"`
	Platforms              []ChannelMonitorV2PlatformConfig `json:"platforms"`
	GroupIDs               []int64                          `json:"group_ids"`
	HealthThresholds       ChannelMonitorV2HealthThresholds `json:"health_thresholds"`
	// IgnoredErrorCategories are excluded from error_rate / health scoring.
	// They still appear in the error breakdown with ignored=true (greyed in UI).
	// Unknown categories always roll into "other" via the taxonomy classifier.
	IgnoredErrorCategories []string  `json:"ignored_error_categories"`
	UpdatedAt              time.Time `json:"updated_at"`
	UpdatedBy              *int64    `json:"updated_by,omitempty"`
REDACTED

// ChannelMonitorV2ErrorCategories is the ordered, versioned taxonomy used by the
// classifier and admin settings UI. Unmatched errors become "other".
var ChannelMonitorV2ErrorCategories = []string{
	"content_policy",
	"authentication",
	"context_limit",
	"invalid_request",
	"model_unsupported",
	"group_access",
	"quota_or_balance",
	"account_pool_unavailable",
	"rate_or_capacity",
	"timeout",
	"transport_or_stream",
	"upstream_forbidden",
	"not_found",
	"client_cancelled",
	"upstream_5xx",
	"internal",
	"other",
REDACTED

type ChannelMonitorV2Filter struct {
	Range     string
	Platforms []string
	GroupIDs  []int64
	Models    []string
	Start     time.Time
	End       time.Time
	Bucket    time.Duration
REDACTED

type ChannelMonitorV2Metric struct {
	SuccessRequests          int64                   `json:"success_requests"`
	ErrorRequests            int64                   `json:"error_requests"`
	RequestCount             int64                   `json:"request_count"`
	InputTokens              int64                   `json:"input_tokens"`
	OutputTokens             int64                   `json:"output_tokens"`
	CacheCreationTokens      int64                   `json:"cache_creation_tokens"`
	CacheReadTokens          int64                   `json:"cache_read_tokens"`
	TokenCount               int64                   `json:"token_count"`
	RPM                      float64                 `json:"rpm"`
	TPM                      float64                 `json:"tpm"`
	ErrorRate                float64                 `json:"error_rate"`
	SuccessRate              float64                 `json:"success_rate"`
	CacheRate                float64                 `json:"cache_rate"`
	CacheRateNumerator       int64                   `json:"cache_rate_numerator"`
	CacheRateDenominator     int64                   `json:"cache_rate_denominator"`
	TTFT                     ChannelMonitorV2Latency `json:"ttft"`
	Duration                 ChannelMonitorV2Latency `json:"duration"`
	UpstreamAffectedRequests *int64                  `json:"upstream_affected_requests,omitempty"`
	UpstreamAttemptCount     *int64                  `json:"upstream_attempt_count,omitempty"`
REDACTED

type ChannelMonitorV2Latency struct {
	SampleCount int64    `json:"sample_count"`
	P50Ms       *int64   `json:"p50_ms"`
	P90Ms       *int64   `json:"p90_ms"`
	P95Ms       *int64   `json:"p95_ms"`
	AvgMs       *float64 `json:"avg_ms"`
REDACTED

type ChannelMonitorV2Health struct {
	Overall   string `json:"overall"`
	ErrorRate string `json:"error_rate"`
	TTFT      string `json:"ttft"`
	Cache     string `json:"cache"`
	// Score is 0–100 when samples are sufficient; omitted/null when unknown.
	// Overall blends error-rate, TTFT p50, and cache rate (weights in Thresholds).
	Score          *float64                         `json:"score,omitempty"`
	ErrorRateScore *float64                         `json:"error_rate_score,omitempty"`
	TTFTScore      *float64                         `json:"ttft_score,omitempty"`
	CacheScore     *float64                         `json:"cache_score,omitempty"`
	MinimumSample  int64                            `json:"minimum_sample"`
	Thresholds     ChannelMonitorV2HealthThresholds `json:"thresholds"`
REDACTED

type ChannelMonitorV2HealthThresholds struct {
	// MinimumSample is required before scoring request/latency/cache signals.
	MinimumSample int64 `json:"minimum_sample"`
	// WarningErrorRate / CriticalErrorRate map to discrete bands for legacy UI.
	WarningErrorRate  float64 `json:"warning_error_rate"`
	CriticalErrorRate float64 `json:"critical_error_rate"`
	// TargetTTFTMs is the primary TTFT p50 budget (ms). At or below this is full TTFT score.
	TargetTTFTMs   int64 `json:"target_ttft_ms"`
	WarningTTFTMs  int64 `json:"warning_ttft_ms"`
	CriticalTTFTMs int64 `json:"critical_ttft_ms"`
	// WarningCacheRate / CriticalCacheRate: cache rate below these → warning/critical bands.
	// Higher cache rate is better; defaults 20% warning / 5% critical.
	WarningCacheRate  float64 `json:"warning_cache_rate"`
	CriticalCacheRate float64 `json:"critical_cache_rate"`
	// ErrorWeight + TTFTWeight + CacheWeight should sum to 1.0.
	ErrorWeight float64 `json:"error_weight"`
	TTFTWeight  float64 `json:"ttft_weight"`
	CacheWeight float64 `json:"cache_weight"`
REDACTED

type ChannelMonitorV2Coverage struct {
	RequestedStart time.Time `json:"requested_start"`
	// RequestedEnd is the exclusive upper bound of the UI-selected window
	// (filter.End). Charts/matrices should plot [RequestedStart, RequestedEnd)
	// even when CoverageStart is later (partial backfill).
	RequestedEnd          time.Time `json:"requested_end"`
	CoverageStart         time.Time `json:"coverage_start"`
	DataThrough           time.Time `json:"data_through"`
	ComputedAt            time.Time `json:"computed_at"`
	AggregationLagSeconds int64     `json:"aggregation_lag_seconds"`
	// CoverageComplete is true when aggregated history reaches the requested
	// window start (backfill / product coverage for this range is done).
	// Trailing lag — data_through behind filter.End because UI ends align to
	// the next bucket and may sit slightly in the future — does not make this
	// false once history is covered.
	CoverageComplete bool `json:"coverage_complete"`
	BucketSeconds    int  `json:"bucket_seconds"`
	// Bootstrap is set while the first-upgrade historical backfill toward the
	// product windows (90m / 24h / 7d / 30d) is still running. Omitted or
	// inactive once the 30d UI range is fully covered; longer retention (90d)
	// may continue silently without a progress banner.
	Bootstrap *ChannelMonitorV2Bootstrap `json:"bootstrap,omitempty"`
REDACTED

// ChannelMonitorV2Bootstrap reports first-upgrade aggregation progress for the UI.
// Progress is measured against the longest product window (30d), not full 90d retention.
type ChannelMonitorV2Bootstrap struct {
	// Active is true while historical backfill has not yet covered the 30d product window.
	Active bool `json:"active"`
	// ProgressPercent is 0–100 of history covered from now back toward TargetStart.
	ProgressPercent int `json:"progress_percent"`
	// CoveredFrom is the earliest minute already recomputed (backfill cursor).
	CoveredFrom time.Time `json:"covered_from,omitempty"`
	// TargetStart is now−30d (product bootstrap goal for 90m/24h/7d/30d).
	TargetStart time.Time `json:"target_start,omitempty"`
REDACTED

// ChannelMonitorV2AggregationWatermark is the durable aggregator cursor state.
type ChannelMonitorV2AggregationWatermark struct {
	UsageCoverageStart time.Time
	ErrorCoverageStart time.Time
	DataThrough        time.Time
	LastSuccessfulAt   time.Time
	// BackfillCursor is the earliest start already recomputed; zero when never set.
	BackfillCursor time.Time
	// HasData is true once any successful recompute wrote data_through.
	HasData bool
REDACTED

type ChannelMonitorV2TrendPoint struct {
	BucketStart time.Time              `json:"bucket_start"`
	Metrics     ChannelMonitorV2Metric `json:"metrics"`
	Health      ChannelMonitorV2Health `json:"health"`
REDACTED

type ChannelMonitorV2Snapshot struct {
	Config   ChannelMonitorV2Config       `json:"config"`
	Coverage ChannelMonitorV2Coverage     `json:"coverage"`
	Metrics  ChannelMonitorV2Metric       `json:"metrics"`
	Health   ChannelMonitorV2Health       `json:"health"`
	Trend    []ChannelMonitorV2TrendPoint `json:"trend"`
REDACTED

type ChannelMonitorV2Dimension struct {
	Value        string `json:"value"`
	Label        string `json:"label"`
	Platform     string `json:"platform,omitempty"`
	RequestCount int64  `json:"request_count"`
REDACTED

type ChannelMonitorV2GroupDimension struct {
	ID           int64  `json:"id"`
	Name         string `json:"name"`
	Platform     string `json:"platform,omitempty"`
	RequestCount int64  `json:"request_count"`
REDACTED

type ChannelMonitorV2Dimensions struct {
	Platforms []ChannelMonitorV2Dimension      `json:"platforms"`
	Groups    []ChannelMonitorV2GroupDimension `json:"groups"`
	Models    []ChannelMonitorV2Dimension      `json:"models"`
REDACTED

type ChannelMonitorV2ModelRow struct {
	Platform string                 `json:"platform"`
	Model    string                 `json:"model"`
	Metrics  ChannelMonitorV2Metric `json:"metrics"`
	Health   ChannelMonitorV2Health `json:"health"`
REDACTED

type ChannelMonitorV2MatrixRow struct {
	Platform  string                       `json:"platform"`
	GroupID   *int64                       `json:"group_id,omitempty"`
	GroupName string                       `json:"group_name,omitempty"`
	Model     string                       `json:"model,omitempty"`
	Metrics   ChannelMonitorV2Metric       `json:"metrics"`
	Health    ChannelMonitorV2Health       `json:"health"`
	Buckets   []ChannelMonitorV2TrendPoint `json:"buckets"`
REDACTED

type ChannelMonitorV2Matrix struct {
	GroupBy  ChannelMonitorV2GroupBy     `json:"group_by"`
	Coverage ChannelMonitorV2Coverage    `json:"coverage"`
	Items    []ChannelMonitorV2MatrixRow `json:"items"`
REDACTED

type ChannelMonitorV2ErrorRow struct {
	Category string                        `json:"category"`
	Count    int64                         `json:"count"`
	Rate     float64                       `json:"rate"`
	Details  []ChannelMonitorV2ErrorDetail `json:"details,omitempty"`
	// Ignored is true when this category is excluded from error_rate scoring
	// via config.ignored_error_categories. Still shown in breakdown (UI greys it).
	Ignored bool `json:"ignored"`
REDACTED

type ChannelMonitorV2ErrorDetail struct {
	Platform           string `json:"platform,omitempty"`
	Model              string `json:"model,omitempty"`
	ErrorType          string `json:"error_type,omitempty"`
	StatusCode         int    `json:"status_code,omitempty"`
	UpstreamStatusCode int    `json:"upstream_status_code,omitempty"`
	Message            string `json:"message,omitempty"`
	Count              int64  `json:"count"`
REDACTED

type ChannelMonitorV2UserRow struct {
	UserID       *int64                 `json:"user_id,omitempty"`
	Rank         int                    `json:"rank"`
	Email        string                 `json:"email,omitempty"`
	Username     string                 `json:"username,omitempty"`
	DisplayLabel string                 `json:"display_label"`
	IsSelf       bool                   `json:"is_self"`
	CanDrilldown bool                   `json:"can_drilldown"`
	Metrics      ChannelMonitorV2Metric `json:"metrics"`
REDACTED

type ChannelMonitorV2List[T any] struct {
	Coverage ChannelMonitorV2Coverage `json:"coverage"`
	Items    []T                      `json:"items"`
REDACTED

type ChannelMonitorV2Repository interface {
	GetConfig(ctx context.Context) (*ChannelMonitorV2Config, error)
	UpdateConfig(ctx context.Context, config ChannelMonitorV2Config, expectedVersion int) (*ChannelMonitorV2Config, error)
	GetDimensions(ctx context.Context, filter ChannelMonitorV2Filter, config ChannelMonitorV2Config) (*ChannelMonitorV2Dimensions, error)
	GetSnapshot(ctx context.Context, filter ChannelMonitorV2Filter, config ChannelMonitorV2Config, includeAdmin bool) (*ChannelMonitorV2Snapshot, error)
	GetModels(ctx context.Context, filter ChannelMonitorV2Filter, config ChannelMonitorV2Config, includeAdmin bool) (*ChannelMonitorV2List[ChannelMonitorV2ModelRow], error)
	GetMatrix(ctx context.Context, filter ChannelMonitorV2Filter, config ChannelMonitorV2Config, groupBy ChannelMonitorV2GroupBy, includeAdmin bool) (*ChannelMonitorV2Matrix, error)
	// GetErrors loads category rates. When includeAdmin is false, implementations
	// must omit error Details (no ops_error_logs sample scan) for privacy.
	GetErrors(ctx context.Context, filter ChannelMonitorV2Filter, config ChannelMonitorV2Config, includeAdmin bool) (*ChannelMonitorV2List[ChannelMonitorV2ErrorRow], error)
	GetUsers(ctx context.Context, filter ChannelMonitorV2Filter, config ChannelMonitorV2Config, includeAdmin bool) (*ChannelMonitorV2List[ChannelMonitorV2UserRow], error)
	// GetAggregationWatermark loads durable backfill / coverage cursors for the
	// passive aggregator (and bootstrap progress). Missing row → zero value, nil error.
	GetAggregationWatermark(ctx context.Context) (*ChannelMonitorV2AggregationWatermark, error)
	RecomputeRange(ctx context.Context, start, end time.Time) error
REDACTED

// ChannelMonitorV2BootstrapProductWindow is the longest UI range that must be
// filled on first upgrade before the bootstrap banner disappears (30d).
const ChannelMonitorV2BootstrapProductWindow = 30 * 24 * time.Hour

// ChannelMonitorV2BootstrapProgress builds the optional UI progress payload.
// now and coveredFrom should be UTC minute-truncated when possible.
// When history is complete for the 30d product window, returns nil (no banner).
func ChannelMonitorV2BootstrapProgress(now, coveredFrom time.Time, hasData bool) *ChannelMonitorV2Bootstrap {
	now = now.UTC().Truncate(time.Minute)
	targetStart := now.Add(-ChannelMonitorV2BootstrapProductWindow)
	if !hasData {
		return &ChannelMonitorV2Bootstrap{
			Active:          true,
			ProgressPercent: 0,
			TargetStart:     targetStart,
	REDACTED
REDACTED
	if coveredFrom.IsZero() {
		// Have data_through but no cursor yet — treat as barely started.
		return &ChannelMonitorV2Bootstrap{
			Active:          true,
			ProgressPercent: 0,
			TargetStart:     targetStart,
	REDACTED
REDACTED
	coveredFrom = coveredFrom.UTC().Truncate(time.Minute)
	if !coveredFrom.After(targetStart) {
		// 30d product windows fully covered; hide progress (90d retention may continue).
		return nil
REDACTED
	total := now.Sub(targetStart).Seconds()
	if total <= 0 {
		return nil
REDACTED
	done := now.Sub(coveredFrom).Seconds()
	if done < 0 {
		done = 0
REDACTED
	if done > total {
		done = total
REDACTED
	// Round to nearest percent; any non-zero history shows at least 1% so the
	// bar moves after the first 2h bootstrap tick (2h/30d ≈ 0.3%).
	pct := int((done/total)*100 + 0.5)
	if done > 0 && pct < 1 {
		pct = 1
REDACTED
	if pct > 100 {
		pct = 100
REDACTED
	if pct >= 100 {
		// Cursor still slightly after target due to rounding — keep active until
		// coveredFrom <= targetStart so the banner does not flash 100% then vanish.
		pct = 99
REDACTED
	return &ChannelMonitorV2Bootstrap{
		Active:          true,
		ProgressPercent: pct,
		CoveredFrom:     coveredFrom,
		TargetStart:     targetStart,
REDACTED
REDACTED

type ChannelMonitorV2Service struct {
	repo     ChannelMonitorV2Repository
	settings channelMonitorRuntimeReader
	now      func() time.Time
REDACTED

func NewChannelMonitorV2Service(repo ChannelMonitorV2Repository) *ChannelMonitorV2Service {
	return &ChannelMonitorV2Service{repo: repo, now: func() time.Time { return time.Now().UTC() REDACTEDREDACTED
REDACTED

// SetRuntimeReader wires optional settings for privacy flags (hide throughput).
func (s *ChannelMonitorV2Service) SetRuntimeReader(r channelMonitorRuntimeReader) {
	if s == nil {
		return
REDACTED
	s.settings = r
REDACTED

func (s *ChannelMonitorV2Service) hideThroughputForViewer(ctx context.Context, admin bool) bool {
	if admin {
		return false
REDACTED
	// Privacy is fail-closed: an absent or unavailable settings reader must not
	// expose fleet-scale rates to ordinary users.
	if s == nil || s.settings == nil {
		return true
REDACTED
	return s.settings.GetChannelMonitorRuntime(ctx).HideThroughput
REDACTED

func (s *ChannelMonitorV2Service) GetConfig(ctx context.Context) (*ChannelMonitorV2Config, error) {
	return s.repo.GetConfig(ctx)
REDACTED

func (s *ChannelMonitorV2Service) getEnabledConfig(ctx context.Context) (*ChannelMonitorV2Config, error) {
	cfg, err := s.repo.GetConfig(ctx)
	if err != nil {
		return nil, err
REDACTED
	if cfg == nil || !cfg.Enabled {
		return nil, ErrChannelMonitorDisabled
REDACTED
	return cfg, nil
REDACTED

func (s *ChannelMonitorV2Service) UpdateConfig(ctx context.Context, cfg ChannelMonitorV2Config, expectedVersion int, actorID int64) (*ChannelMonitorV2Config, error) {
	if err := normalizeChannelMonitorV2Config(&cfg); err != nil {
		return nil, err
REDACTED
	cfg.UpdatedBy = &actorID
	return s.repo.UpdateConfig(ctx, cfg, expectedVersion)
REDACTED

func (s *ChannelMonitorV2Service) ParseFilter(rangeValue string, platforms, models []string, groupIDs []int64) (ChannelMonitorV2Filter, error) {
	now := s.now().UTC()
	var window, bucket time.Duration
	switch strings.TrimSpace(rangeValue) {
	case "", "90m":
		rangeValue, window, bucket = "90m", 90*time.Minute, 5*time.Minute
	case "24h":
		window, bucket = 24*time.Hour, time.Hour
	case "7d":
		window, bucket = 7*24*time.Hour, 12*time.Hour
	case "30d":
		window, bucket = 30*24*time.Hour, 24*time.Hour
	default:
		return ChannelMonitorV2Filter{REDACTED, fmt.Errorf("%w: %s", ErrChannelMonitorV2InvalidRange, rangeValue)
REDACTED
	start, end := now.Add(-window), now
	if bucket > time.Minute {
		// Align display windows to a fixed number of whole buckets. The trailing
		// bucket may point slightly into the future; SQL simply has no future rows,
		// while the current partial bucket can still be shown and recomputed often.
		end = now.Truncate(bucket).Add(bucket)
		start = end.Add(-window)
REDACTED
	return ChannelMonitorV2Filter{
		Range: rangeValue, Platforms: normalizeStringSet(platforms), Models: normalizeStringSet(models), GroupIDs: normalizeInt64Set(groupIDs),
		Start: start, End: end, Bucket: bucket,
REDACTED, nil
REDACTED

func (s *ChannelMonitorV2Service) Dimensions(ctx context.Context, filter ChannelMonitorV2Filter) (*ChannelMonitorV2Dimensions, error) {
	cfg, err := s.getEnabledConfig(ctx)
	if err != nil {
		return nil, err
REDACTED
	dims, err := s.repo.GetDimensions(ctx, filter, *cfg)
	if err != nil {
		return nil, err
REDACTED
	// Dimension request_count is operational volume; strip for non-admin callers
	// at the API edge. Dimensions is shared by user/admin routes — redaction is
	// applied in the handler for user routes only, so keep raw here.
	return dims, nil
REDACTED

func (s *ChannelMonitorV2Service) Snapshot(ctx context.Context, filter ChannelMonitorV2Filter, admin bool) (*ChannelMonitorV2Snapshot, error) {
	cfg, err := s.getEnabledConfig(ctx)
	if err != nil {
		return nil, err
REDACTED
	snap, err := s.repo.GetSnapshot(ctx, filter, *cfg, admin)
	if err != nil {
		return nil, err
REDACTED
	if !admin && snap != nil {
		redactChannelMonitorV2Snapshot(snap, s.hideThroughputForViewer(ctx, admin))
REDACTED
	return snap, nil
REDACTED

func (s *ChannelMonitorV2Service) Models(ctx context.Context, filter ChannelMonitorV2Filter, admin bool) (*ChannelMonitorV2List[ChannelMonitorV2ModelRow], error) {
	cfg, err := s.getEnabledConfig(ctx)
	if err != nil {
		return nil, err
REDACTED
	list, err := s.repo.GetModels(ctx, filter, *cfg, admin)
	if err != nil {
		return nil, err
REDACTED
	if !admin && list != nil {
		hideTP := s.hideThroughputForViewer(ctx, admin)
		for i := range list.Items {
			redactChannelMonitorV2Metric(&list.Items[i].Metrics, hideTP)
	REDACTED
REDACTED
	return list, nil
REDACTED

func (s *ChannelMonitorV2Service) Matrix(ctx context.Context, filter ChannelMonitorV2Filter, groupBy ChannelMonitorV2GroupBy, admin bool) (*ChannelMonitorV2Matrix, error) {
	if !groupBy.Valid() {
		return nil, fmt.Errorf("%w: %s", ErrChannelMonitorV2InvalidGroupBy, groupBy)
REDACTED
	cfg, err := s.getEnabledConfig(ctx)
	if err != nil {
		return nil, err
REDACTED
	matrix, err := s.repo.GetMatrix(ctx, filter, *cfg, groupBy, admin)
	if err != nil {
		return nil, err
REDACTED
	if !admin && matrix != nil {
		hideTP := s.hideThroughputForViewer(ctx, admin)
		for i := range matrix.Items {
			redactChannelMonitorV2Metric(&matrix.Items[i].Metrics, hideTP)
			for j := range matrix.Items[i].Buckets {
				redactChannelMonitorV2Metric(&matrix.Items[i].Buckets[j].Metrics, hideTP)
		REDACTED
	REDACTED
REDACTED
	return matrix, nil
REDACTED

func ParseChannelMonitorV2GroupBy(value string) (ChannelMonitorV2GroupBy, error) {
	groupBy := ChannelMonitorV2GroupBy(strings.TrimSpace(value))
	if groupBy == "" {
		// Default presentation: platform / group (matches operator mental model).
		groupBy = ChannelMonitorV2GroupByPlatformGroup
REDACTED
	if !groupBy.Valid() {
		return "", fmt.Errorf("%w: %s", ErrChannelMonitorV2InvalidGroupBy, value)
REDACTED
	return groupBy, nil
REDACTED

func (g ChannelMonitorV2GroupBy) Valid() bool {
	switch g {
	case ChannelMonitorV2GroupByPlatform, ChannelMonitorV2GroupByPlatformGroup, ChannelMonitorV2GroupByPlatformModel, ChannelMonitorV2GroupByPlatformGroupModel:
		return true
	default:
		return false
REDACTED
REDACTED

func (s *ChannelMonitorV2Service) Errors(ctx context.Context, filter ChannelMonitorV2Filter) (*ChannelMonitorV2List[ChannelMonitorV2ErrorRow], error) {
	return s.ErrorsForViewer(ctx, filter, false)
REDACTED

// ErrorsForViewer returns the error breakdown.
// Non-admin callers receive category rates + ignored flags only: absolute Count
// is zeroed and Details (upstream messages / status codes / volume) are omitted.
func (s *ChannelMonitorV2Service) ErrorsForViewer(ctx context.Context, filter ChannelMonitorV2Filter, admin bool) (*ChannelMonitorV2List[ChannelMonitorV2ErrorRow], error) {
	cfg, err := s.getEnabledConfig(ctx)
	if err != nil {
		return nil, err
REDACTED
	list, err := s.repo.GetErrors(ctx, filter, *cfg, admin)
	if err != nil {
		return nil, err
REDACTED
	if !admin && list != nil {
		for i := range list.Items {
			list.Items[i].Count = 0
			list.Items[i].Details = nil
	REDACTED
REDACTED
	return list, nil
REDACTED

// RedactChannelMonitorV2Dimensions clears absolute request counts on filter chips.
func RedactChannelMonitorV2Dimensions(dims *ChannelMonitorV2Dimensions) {
	if dims == nil {
		return
REDACTED
	for i := range dims.Platforms {
		dims.Platforms[i].RequestCount = 0
REDACTED
	for i := range dims.Models {
		dims.Models[i].RequestCount = 0
REDACTED
	for i := range dims.Groups {
		dims.Groups[i].RequestCount = 0
REDACTED
REDACTED

func redactChannelMonitorV2Snapshot(snap *ChannelMonitorV2Snapshot, hideThroughput bool) {
	if snap == nil {
		return
REDACTED
	redactChannelMonitorV2Metric(&snap.Metrics, hideThroughput)
	for i := range snap.Trend {
		redactChannelMonitorV2Metric(&snap.Trend[i].Metrics, hideThroughput)
REDACTED
	// Public snapshot only needs display thresholds + refresh cadence, not
	// operational allow-lists (group_ids, model inventories, ignored categories).
	redactChannelMonitorV2PublicConfig(&snap.Config)
REDACTED

// redactChannelMonitorV2PublicConfig strips operator-policy fields from config
// embedded in user-facing snapshots. Full config remains on admin /config.
func redactChannelMonitorV2PublicConfig(cfg *ChannelMonitorV2Config) {
	if cfg == nil {
		return
REDACTED
	cfg.GroupIDs = nil
	cfg.IgnoredErrorCategories = nil
	cfg.UpdatedBy = nil
	for i := range cfg.Platforms {
		cfg.Platforms[i].Models = nil
REDACTED
REDACTED

// redactChannelMonitorV2Metric zeros absolute volume counters while keeping rates
// (error_rate, success_rate, cache_rate) and latency percentiles.
// When hideThroughput is true, also zeros RPM/TPM so users cannot reverse-estimate
// fleet scale from rate × window length.
func redactChannelMonitorV2Metric(m *ChannelMonitorV2Metric, hideThroughput bool) {
	if m == nil {
		return
REDACTED
	m.SuccessRequests = 0
	m.ErrorRequests = 0
	m.RequestCount = 0
	m.InputTokens = 0
	m.OutputTokens = 0
	m.CacheCreationTokens = 0
	m.CacheReadTokens = 0
	m.TokenCount = 0
	m.CacheRateNumerator = 0
	m.CacheRateDenominator = 0
	// Latency sample_count is also a volume signal.
	m.TTFT.SampleCount = 0
	m.Duration.SampleCount = 0
	m.UpstreamAffectedRequests = nil
	m.UpstreamAttemptCount = nil
	if hideThroughput {
		m.RPM = 0
		m.TPM = 0
REDACTED
REDACTED

func (s *ChannelMonitorV2Service) Users(ctx context.Context, filter ChannelMonitorV2Filter, viewerID int64, admin bool) (*ChannelMonitorV2List[ChannelMonitorV2UserRow], error) {
	cfg, err := s.getEnabledConfig(ctx)
	if err != nil {
		return nil, err
REDACTED
	result, err := s.repo.GetUsers(ctx, filter, *cfg, admin)
	if err != nil {
		return nil, err
REDACTED
	if result == nil {
		result = &ChannelMonitorV2List[ChannelMonitorV2UserRow]{REDACTED
REDACTED
	selfIndex := -1
	for i := range result.Items {
		result.Items[i].Rank = i + 1
		if result.Items[i].UserID != nil && *result.Items[i].UserID == viewerID {
			selfIndex = i
			result.Items[i].IsSelf = true
	REDACTED
REDACTED
	// Viewer with no traffic in this window is still shown (and highlighted) so
	// ranking always answers "where am I?" — not only when already in the top list.
	if selfIndex < 0 && viewerID > 0 {
		id := viewerID
		selfRow := ChannelMonitorV2UserRow{
			UserID:       &id,
			Rank:         0, // unranked / no traffic in window
			IsSelf:       true,
			CanDrilldown: true,
			DisplayLabel: "Me",
			Metrics:      ChannelMonitorV2Metric{REDACTED,
	REDACTED
		result.Items = append(result.Items, selfRow)
		selfIndex = len(result.Items) - 1
REDACTED
	result.Items = channelMonitorV2TopUsersWithSelf(result.Items, selfIndex, 10)
	hideTP := s.hideThroughputForViewer(ctx, admin)
	if admin {
		// Keep identity for admin; still mark self for UI highlight.
		for i := range result.Items {
			if result.Items[i].UserID != nil && *result.Items[i].UserID == viewerID {
				result.Items[i].IsSelf = true
				if result.Items[i].DisplayLabel == "" || result.Items[i].DisplayLabel == "Me" {
					// Prefer real label when available from repo.
					if result.Items[i].Username != "" {
						result.Items[i].DisplayLabel = result.Items[i].Username
				REDACTED else if result.Items[i].Email != "" {
						result.Items[i].DisplayLabel = result.Items[i].Email
				REDACTED else {
						result.Items[i].DisplayLabel = "Me"
				REDACTED
			REDACTED
		REDACTED
	REDACTED
		return result, nil
REDACTED
	for i := range result.Items {
		redactChannelMonitorV2Metric(&result.Items[i].Metrics, hideTP)
REDACTED
	for i := range result.Items {
		row := &result.Items[i]
		if row.UserID != nil && *row.UserID == viewerID {
			row.IsSelf, row.CanDrilldown, row.DisplayLabel = true, true, "Me"
			continue
	REDACTED
		row.UserID, row.Email, row.Username, row.CanDrilldown = nil, "", "", false
		row.DisplayLabel = fmt.Sprintf("Other user #%d", i+1)
REDACTED
	return result, nil
REDACTED

func channelMonitorV2TopUsersWithSelf(items []ChannelMonitorV2UserRow, selfIndex int, limit int) []ChannelMonitorV2UserRow {
	if limit <= 0 {
		return items
REDACTED
	if len(items) <= limit {
		return items
REDACTED
	out := append([]ChannelMonitorV2UserRow(nil), items[:limit]...)
	if selfIndex >= limit && selfIndex < len(items) {
		out = append(out, items[selfIndex])
REDACTED
	return out
REDACTED

func normalizeChannelMonitorV2Config(cfg *ChannelMonitorV2Config) error {
	if cfg.RefreshIntervalSeconds == 0 {
		cfg.RefreshIntervalSeconds = 300
REDACTED
	if cfg.RefreshIntervalSeconds != 60 && cfg.RefreshIntervalSeconds != 300 {
		return fmt.Errorf("%w: refresh_interval_seconds must be 60 or 300", ErrChannelMonitorV2InvalidConfig)
REDACTED
	var err error
	cfg.GroupIDs, err = normalizeChannelMonitorV2GroupIDs(cfg.GroupIDs)
	if err != nil {
		return err
REDACTED
	cfg.IgnoredErrorCategories = normalizeChannelMonitorV2IgnoredCategories(cfg.IgnoredErrorCategories)
	cfg.HealthThresholds = NormalizeChannelMonitorV2HealthThresholds(cfg.HealthThresholds)
	seen := make(map[string]struct{REDACTED, len(cfg.Platforms))
	for i := range cfg.Platforms {
		p := &cfg.Platforms[i]
		p.Platform = strings.ToLower(strings.TrimSpace(p.Platform))
		if p.Platform == "" {
			return fmt.Errorf("%w: empty platform", ErrChannelMonitorV2InvalidConfig)
	REDACTED
		if _, ok := seen[p.Platform]; ok {
			return fmt.Errorf("%w: duplicate platform %s", ErrChannelMonitorV2InvalidConfig, p.Platform)
	REDACTED
		seen[p.Platform] = struct{REDACTED{REDACTED
		p.Models = normalizeStringSet(p.Models)
REDACTED
	sort.Slice(cfg.Platforms, func(i, j int) bool { return cfg.Platforms[i].Platform < cfg.Platforms[j].Platform REDACTED)
	return nil
REDACTED

// DefaultChannelMonitorV2IgnoredErrorCategories are factory defaults for
// ignored_error_categories: excluded from error_rate / health scoring only.
// Operators can clear or extend via admin config.
var DefaultChannelMonitorV2IgnoredErrorCategories = []string{
	"authentication",
	"client_cancelled",
	"content_policy",
	"context_limit",
	"group_access",
	"model_unsupported",
	"not_found",
	"quota_or_balance",
REDACTED

func DefaultChannelMonitorV2HealthThresholds() ChannelMonitorV2HealthThresholds {
	return ChannelMonitorV2HealthThresholds{
		MinimumSample:     50,
		WarningErrorRate:  0.05,
		CriticalErrorRate: 0.20,
		TargetTTFTMs:      3000,
		WarningTTFTMs:     3000,
		CriticalTTFTMs:    10000,
		// A zero/zero cache threshold means cache misses do not affect health
		// until an operator explicitly configures cache scoring.
		WarningCacheRate:  0,
		CriticalCacheRate: 0,
		ErrorWeight:       0.60,
		TTFTWeight:        0.20,
		CacheWeight:       0.20,
REDACTED
REDACTED

func NormalizeChannelMonitorV2HealthThresholds(in ChannelMonitorV2HealthThresholds) ChannelMonitorV2HealthThresholds {
	def := DefaultChannelMonitorV2HealthThresholds()
	if in.MinimumSample <= 0 {
		in.MinimumSample = def.MinimumSample
REDACTED
	if in.MinimumSample < 1 {
		in.MinimumSample = 1
REDACTED
	if in.MinimumSample > 10000 {
		in.MinimumSample = 10000
REDACTED
	if in.WarningErrorRate <= 0 {
		in.WarningErrorRate = def.WarningErrorRate
REDACTED
	if in.CriticalErrorRate <= 0 {
		in.CriticalErrorRate = def.CriticalErrorRate
REDACTED
	if in.CriticalErrorRate < in.WarningErrorRate {
		in.CriticalErrorRate = in.WarningErrorRate
REDACTED
	if in.TargetTTFTMs <= 0 {
		in.TargetTTFTMs = def.TargetTTFTMs
REDACTED
	if in.WarningTTFTMs <= 0 {
		in.WarningTTFTMs = def.WarningTTFTMs
REDACTED
	if in.WarningTTFTMs < in.TargetTTFTMs {
		in.WarningTTFTMs = in.TargetTTFTMs + 1
REDACTED
	if in.CriticalTTFTMs <= 0 {
		in.CriticalTTFTMs = def.CriticalTTFTMs
REDACTED
	if in.CriticalTTFTMs < in.WarningTTFTMs {
		in.CriticalTTFTMs = in.WarningTTFTMs
REDACTED
	if in.WarningCacheRate < 0 {
		in.WarningCacheRate = 0
REDACTED
	if in.CriticalCacheRate < 0 {
		in.CriticalCacheRate = 0
REDACTED
	if in.WarningCacheRate > 1 {
		in.WarningCacheRate = 1
REDACTED
	if in.CriticalCacheRate > 1 {
		in.CriticalCacheRate = 1
REDACTED
	if in.CriticalCacheRate > in.WarningCacheRate {
		in.CriticalCacheRate = in.WarningCacheRate
REDACTED
	if in.ErrorWeight <= 0 && in.TTFTWeight <= 0 && in.CacheWeight <= 0 {
		in.ErrorWeight, in.TTFTWeight, in.CacheWeight = def.ErrorWeight, def.TTFTWeight, def.CacheWeight
REDACTED
	return in
REDACTED

func normalizeChannelMonitorV2IgnoredCategories(values []string) []string {
	allowed := make(map[string]struct{REDACTED, len(ChannelMonitorV2ErrorCategories))
	for _, c := range ChannelMonitorV2ErrorCategories {
		allowed[c] = struct{REDACTED{REDACTED
REDACTED
	seen := make(map[string]struct{REDACTED, len(values))
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.ToLower(strings.TrimSpace(value))
		if value == "" {
			continue
	REDACTED
		if _, ok := allowed[value]; !ok {
			// Unknown labels are dropped so a typo cannot silently create a new category.
			continue
	REDACTED
		if _, ok := seen[value]; ok {
			continue
	REDACTED
		seen[value] = struct{REDACTED{REDACTED
		out = append(out, value)
REDACTED
	sort.Strings(out)
	return out
REDACTED

// ChannelMonitorV2IgnoredCategorySet returns a set for O(1) membership checks.
func ChannelMonitorV2IgnoredCategorySet(cfg ChannelMonitorV2Config) map[string]struct{REDACTED {
	set := make(map[string]struct{REDACTED, len(cfg.IgnoredErrorCategories))
	for _, c := range cfg.IgnoredErrorCategories {
		set[c] = struct{REDACTED{REDACTED
REDACTED
	return set
REDACTED

func normalizeStringSet(values []string) []string {
	seen := make(map[string]struct{REDACTED, len(values))
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
	REDACTED
		if _, ok := seen[value]; ok {
			continue
	REDACTED
		seen[value] = struct{REDACTED{REDACTED
		out = append(out, value)
REDACTED
	sort.Strings(out)
	return out
REDACTED

func normalizeInt64Set(values []int64) []int64 {
	seen := make(map[int64]struct{REDACTED, len(values))
	out := make([]int64, 0, len(values))
	for _, value := range values {
		if value <= 0 {
			continue
	REDACTED
		if _, ok := seen[value]; ok {
			continue
	REDACTED
		seen[value] = struct{REDACTED{REDACTED
		out = append(out, value)
REDACTED
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] REDACTED)
	return out
REDACTED

func normalizeChannelMonitorV2GroupIDs(values []int64) ([]int64, error) {
	seen := make(map[int64]struct{REDACTED, len(values))
	out := make([]int64, 0, len(values))
	for _, value := range values {
		if value <= 0 {
			return nil, fmt.Errorf("%w: group_ids must be positive", ErrChannelMonitorV2InvalidConfig)
	REDACTED
		if _, ok := seen[value]; ok {
			continue
	REDACTED
		seen[value] = struct{REDACTED{REDACTED
		out = append(out, value)
REDACTED
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] REDACTED)
	return out, nil
REDACTED

func ChannelMonitorV2HealthFor(metrics ChannelMonitorV2Metric) ChannelMonitorV2Health {
	return ChannelMonitorV2HealthForWithThresholds(metrics, DefaultChannelMonitorV2HealthThresholds())
REDACTED

func ChannelMonitorV2HealthForWithThresholds(metrics ChannelMonitorV2Metric, thresholds ChannelMonitorV2HealthThresholds) ChannelMonitorV2Health {
	thresholds = NormalizeChannelMonitorV2HealthThresholds(thresholds)
	result := ChannelMonitorV2Health{
		Overall: "unknown", ErrorRate: "unknown", TTFT: "unknown", Cache: "unknown",
		MinimumSample: thresholds.MinimumSample, Thresholds: thresholds,
REDACTED

	type scored struct {
		score  float64
		weight float64
		band   string
REDACTED
	parts := make([]scored, 0, 3)

	if metrics.RequestCount >= result.MinimumSample {
		s := errorRateScore(metrics.ErrorRate, thresholds.CriticalErrorRate)
		result.ErrorRateScore = &s
		result.ErrorRate = healthBand(metrics.ErrorRate, thresholds.WarningErrorRate, thresholds.CriticalErrorRate)
		parts = append(parts, scored{score: s, weight: thresholds.ErrorWeight, band: result.ErrorRateREDACTED)
REDACTED
	// Prefer p50 for TTFT scoring; fall back to p95 only if p50 is missing.
	if metrics.TTFT.SampleCount >= result.MinimumSample {
		var ttftMs *int64
		if metrics.TTFT.P50Ms != nil {
			ttftMs = metrics.TTFT.P50Ms
	REDACTED else if metrics.TTFT.P95Ms != nil {
			ttftMs = metrics.TTFT.P95Ms
	REDACTED
		if ttftMs != nil {
			s := ttftP50Score(float64(*ttftMs), float64(thresholds.TargetTTFTMs), float64(thresholds.CriticalTTFTMs))
			result.TTFTScore = &s
			result.TTFT = healthBand(float64(*ttftMs), float64(thresholds.WarningTTFTMs), float64(thresholds.CriticalTTFTMs))
			parts = append(parts, scored{score: s, weight: thresholds.TTFTWeight, band: result.TTFTREDACTED)
	REDACTED
REDACTED
	// Cache: need a meaningful denominator; higher rate is better.
	if metrics.CacheRateDenominator >= result.MinimumSample {
		s := cacheRateScore(metrics.CacheRate)
		if thresholds.WarningCacheRate <= 0 && thresholds.CriticalCacheRate <= 0 {
			// A zero/zero cache threshold means "do not penalize cache misses".
			s = 100
	REDACTED
		result.CacheScore = &s
		// Invert for healthBand (lower is worse): use (1 - rate) against warning/critical floors.
		result.Cache = cacheRateBand(metrics.CacheRate, thresholds.WarningCacheRate, thresholds.CriticalCacheRate)
		parts = append(parts, scored{score: s, weight: thresholds.CacheWeight, band: result.CacheREDACTED)
REDACTED

	if len(parts) == 0 {
		return result
REDACTED
	var weightSum, scoreSum float64
	for _, p := range parts {
		weightSum += p.weight
		scoreSum += p.weight * p.score
REDACTED
	if weightSum <= 0 {
		return result
REDACTED
	overall := scoreSum / weightSum
	result.Score = &overall
	result.Overall = scoreBand(overall)
	return result
REDACTED

// errorRateScore maps error rate to 0–100. 0% → 100; at/above critical → 0 (linear).
func errorRateScore(errorRate, critical float64) float64 {
	if critical <= 0 {
		critical = 0.05
REDACTED
	if errorRate <= 0 {
		return 100
REDACTED
	if errorRate >= critical {
		return 0
REDACTED
	return 100 * (1 - errorRate/critical)
REDACTED

// ttftP50Score maps TTFT p50 ms to 0–100.
// At/below target → 100; at/above critical → 0; linear in between.
func ttftP50Score(p50Ms, targetMs, criticalMs float64) float64 {
	if targetMs <= 0 {
		targetMs = 2500
REDACTED
	if criticalMs <= targetMs {
		criticalMs = targetMs * 2.4
REDACTED
	if p50Ms <= targetMs {
		return 100
REDACTED
	if p50Ms >= criticalMs {
		return 0
REDACTED
	return 100 * (1 - (p50Ms-targetMs)/(criticalMs-targetMs))
REDACTED

// cacheRateScore maps cache hit rate to 0–100 (higher is better, linear).
func cacheRateScore(cacheRate float64) float64 {
	if cacheRate <= 0 {
		return 0
REDACTED
	if cacheRate >= 1 {
		return 100
REDACTED
	return 100 * cacheRate
REDACTED

// cacheRateBand: below critical → critical; below warning → warning; else healthy.
func cacheRateBand(cacheRate, warning, critical float64) string {
	if cacheRate < critical {
		return "critical"
REDACTED
	if cacheRate < warning {
		return "warning"
REDACTED
	return "healthy"
REDACTED

// scoreBand maps continuous 0–100 scores to coarse labels for legacy consumers.
func scoreBand(score float64) string {
	switch {
	case score >= 80:
		return "healthy"
	case score >= 50:
		return "warning"
	default:
		return "critical"
REDACTED
REDACTED

func healthBand(value, warning, critical float64) string {
	if value >= critical {
		return "critical"
REDACTED
	if value >= warning {
		return "warning"
REDACTED
	return "healthy"
REDACTED
