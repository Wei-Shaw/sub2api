package repository

import (
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestChannelMonitorV2DisplayModelIsPlatformScoped(t *testing.T) {
	cfg := service.ChannelMonitorV2Config{Platforms: []service.ChannelMonitorV2PlatformConfig{
		{Platform: "openai", Enabled: true, Models: []string{"shared", "gpt-5"REDACTEDREDACTED,
		{Platform: "grok", Enabled: true, Models: []string{"grok-4"REDACTEDREDACTED,
		// Empty models list must NOT collapse everything into __other__.
		{Platform: "anthropic", Enabled: true, Models: []string{REDACTEDREDACTED,
REDACTEDREDACTED
	require.Equal(t, "shared", channelMonitorV2DisplayModel(cfg, "openai", "shared"))
	require.Equal(t, service.ChannelMonitorV2OtherModel, channelMonitorV2DisplayModel(cfg, "grok", "shared"))
	require.Equal(t, "claude-sonnet-4", channelMonitorV2DisplayModel(cfg, "anthropic", "claude-sonnet-4"))
	// Unconfigured platform still surfaces the real model name.
	require.Equal(t, "gemini-2.5-pro", channelMonitorV2DisplayModel(cfg, "gemini", "gemini-2.5-pro"))
	require.True(t, channelMonitorV2ModelSelected(service.ChannelMonitorV2Filter{Models: []string{service.ChannelMonitorV2OtherModelREDACTEDREDACTED, cfg, "grok", "shared"))
REDACTED

func TestChannelMonitorV2MatrixDimensionKey(t *testing.T) {
	cfg := service.ChannelMonitorV2Config{Platforms: []service.ChannelMonitorV2PlatformConfig{{Platform: "openai", Enabled: true, Models: []string{"gpt-5"REDACTEDREDACTEDREDACTEDREDACTED
	key := channelMonitorV2MatrixDimensionKey(service.ChannelMonitorV2GroupByPlatformGroupModel, cfg, "openai", 7, "gpt-5")
	require.Equal(t, channelMonitorV2MatrixKey{platform: "openai", groupID: 7, model: "gpt-5"REDACTED, key)
	key = channelMonitorV2MatrixDimensionKey(service.ChannelMonitorV2GroupByPlatformModel, cfg, "openai", 7, "unlisted")
	require.Equal(t, channelMonitorV2MatrixKey{platform: "openai", model: service.ChannelMonitorV2OtherModelREDACTED, key)
	key = channelMonitorV2MatrixDimensionKey(service.ChannelMonitorV2GroupByPlatform, cfg, "openai", 7, "gpt-5")
	require.Equal(t, channelMonitorV2MatrixKey{platform: "openai"REDACTED, key)
REDACTED

func TestChannelMonitorV2HistogramPercentilesAreMergedFromCounts(t *testing.T) {
	// 100 samples: 50@100, 40@500, 10@1000
	// target = int64(total*p + 0.999999) truncates: p50→50, p90→90, p95→95
	// cumulative hits: p50@100, p90@500 (50+40), p95@1000
	histogram := map[int64]int64{100: 50, 500: 40, 1000: 10REDACTED
	require.Equal(t, int64(100), *histPercentile(histogram, .5))
	require.Equal(t, int64(500), *histPercentile(histogram, .9))
	require.Equal(t, int64(1000), *histPercentile(histogram, .95))
	require.Nil(t, histPercentile(nil, .95))
	// latencyMetric exposes avg + p50 + p90 + p95
	lat := latencyMetric(1000, 10, histogram)
	require.NotNil(t, lat.AvgMs)
	require.NotNil(t, lat.P50Ms)
	require.NotNil(t, lat.P90Ms)
	require.NotNil(t, lat.P95Ms)
	require.Equal(t, int64(100), *lat.P50Ms)
	require.Equal(t, int64(500), *lat.P90Ms)
	require.Equal(t, int64(1000), *lat.P95Ms)
REDACTED

func TestChannelMonitorV2MetricIncludesSuccessRate(t *testing.T) {
	acc := newMetricAccumulator()
	acc.success, acc.errors = 80, 20
	metric := acc.metric(1, false)
	require.Equal(t, int64(100), metric.RequestCount)
	require.InDelta(t, 0.8, metric.SuccessRate, 0.0001)
	require.InDelta(t, 0.2, metric.ErrorRate, 0.0001)
	require.Nil(t, metric.UpstreamAffectedRequests)

	adminMetric := acc.metric(1, true)
	require.NotNil(t, adminMetric.UpstreamAffectedRequests)
REDACTED

func TestChannelMonitorV2WhereUsesConfiguredScopeAndEmptyFilterMeansAllConfigured(t *testing.T) {
	filter := service.ChannelMonitorV2Filter{Start: time.Unix(1, 0), End: time.Unix(2, 0)REDACTED
	cfg := service.ChannelMonitorV2Config{
		Platforms: []service.ChannelMonitorV2PlatformConfig{{Platform: "openai", Enabled: trueREDACTED, {Platform: "grok", Enabled: falseREDACTEDREDACTED,
		GroupIDs:  []int64{3, 4REDACTED,
REDACTED
	where, args := channelMonitorV2Where(filter, cfg, "m")
	require.Contains(t, where, "m.platform = ANY($3)")
	require.Contains(t, where, "m.group_id = ANY($4)")
	require.Len(t, args, 4)
REDACTED

func TestChannelMonitorV2WhereRejectsGroupFilterOutsideConfiguredScope(t *testing.T) {
	filter := service.ChannelMonitorV2Filter{
		Start: time.Unix(1, 0), End: time.Unix(2, 0), GroupIDs: []int64{9REDACTED,
REDACTED
	cfg := service.ChannelMonitorV2Config{
		Platforms: []service.ChannelMonitorV2PlatformConfig{{Platform: "openai", Enabled: trueREDACTEDREDACTED,
		GroupIDs:  []int64{3, 4REDACTED,
REDACTED
	where, args := channelMonitorV2Where(filter, cfg, "m")
	require.Contains(t, where, "FALSE")
	require.NotContains(t, where, "m.group_id = ANY")
	require.Len(t, args, 3)
REDACTED

func TestChannelMonitorV2ErrorAggregationCountsFinalUserErrorsOnly(t *testing.T) {
	query := strings.ToLower(channelMonitorV2ErrorAggregationSQL)
	require.Contains(t, query, "not current_error.is_count_tokens")
	require.Contains(t, query, "error_type = 'cyber_policy'")
	require.Contains(t, query, "distinct on")
	require.Contains(t, query, "not exists")
	require.Contains(t, query, "upstream_affected_requests")
	require.Contains(t, query, "jsonb_array_length(upstream_errors) > 0")
REDACTED

func TestChannelMonitorV2UsageSuccessExcludesCyberBillingRows(t *testing.T) {
	for _, query := range []string{channelMonitorV2UsageMetricsSQL, channelMonitorV2UserMetricsSQLREDACTED {
		require.Contains(t, query, "COALESCE(ul.request_type, 0) NOT IN (4, 6)")
REDACTED
REDACTED

func TestChannelMonitorV2RatesUseCoveredWindow(t *testing.T) {
	start := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
	filter := service.ChannelMonitorV2Filter{Start: start, End: start.Add(24 * time.Hour)REDACTED
	coverage := service.ChannelMonitorV2Coverage{CoverageStart: start.Add(6 * time.Hour), DataThrough: start.Add(18 * time.Hour)REDACTED
	require.Equal(t, 12*60.0, channelMonitorV2CoveredMinutes(filter, coverage))
	effective := channelMonitorV2CommonCoverageFilter(filter, coverage)
	require.Equal(t, coverage.CoverageStart, effective.Start)
	require.Equal(t, coverage.DataThrough, effective.End)
REDACTED

func TestChannelMonitorV2TierRetentionPolicy(t *testing.T) {
	require.Equal(t, 3*24*time.Hour, channelMonitorV2RetentionUser1m)
	require.Equal(t, 7*24*time.Hour, channelMonitorV2RetentionMetrics1m)
	require.Equal(t, 7*24*time.Hour, channelMonitorV2RetentionError1m)
	require.Equal(t, 7*24*time.Hour, channelMonitorV2RetentionHistogram1m)
	require.Equal(t, 7*24*time.Hour, channelMonitorV2RetentionRollup5m)
	require.Equal(t, 30*24*time.Hour, channelMonitorV2RetentionRollup1h)
	require.Equal(t, 45*24*time.Hour, channelMonitorV2RetentionRollup12h)
	require.Equal(t, 90*24*time.Hour, channelMonitorV2RetentionRollup1d)
	require.Equal(t, channelMonitorV2RetentionRollup1d, channelMonitorV2MaxRetention())
	require.Contains(t, channelMonitorV2WatermarkSQL, "INTERVAL '90 days'")

	// Every fixed rollup second must appear with a retention rule.
	wantSeconds := map[int]time.Duration{
		300:   channelMonitorV2RetentionRollup5m,
		3600:  channelMonitorV2RetentionRollup1h,
		43200: channelMonitorV2RetentionRollup12h,
		86400: channelMonitorV2RetentionRollup1d,
REDACTED
	seen := map[int]time.Duration{REDACTED
	for _, rule := range channelMonitorV2RetentionRules {
		if rule.bucketSeconds == 0 {
			require.True(t, rule.retention > 0)
			continue
	REDACTED
		if prev, ok := seen[rule.bucketSeconds]; ok {
			require.Equal(t, prev, rule.retention)
	REDACTED
		seen[rule.bucketSeconds] = rule.retention
REDACTED
	for seconds, want := range wantSeconds {
		got, ok := seen[seconds]
		require.Truef(t, ok, "missing retention rule for bucket_seconds=%d", seconds)
		require.Equal(t, want, got)
REDACTED

	now := time.Date(2026, 8, 7, 12, 0, 0, 0, time.UTC)
	require.Equal(t, now.Add(-7*24*time.Hour), channelMonitorV2RetentionCutoff(now, channelMonitorV2RetentionMetrics1m))
	require.Equal(t, now.Add(-90*24*time.Hour), channelMonitorV2RetentionCutoff(now, channelMonitorV2MaxRetention()))
REDACTED

// Needles present in service.ClassifyChannelMonitorV2Error must appear in the
// aggregation SQL CASE so rollup categories match drilldown classification.
func TestChannelMonitorV2SQLTaxonomyContainsGoNeedles(t *testing.T) {
	sql := channelMonitorV2ErrorAggregationSQL
	needles := []string{
		"blocked keyword",
		"invalid_api_key",
		"max_tokens",
		"invalid_request",
		"model not supported",
		"billing hard limit",
		"no healthy upstream account",
		"rate_limit",
		"gateway timeout",
		"connection refused",
		"unexpected eof",
REDACTED
	for _, needle := range needles {
		require.Containsf(t, strings.ToLower(sql), strings.ToLower(needle), "SQL taxonomy missing Go needle %q", needle)
REDACTED
REDACTED

func TestApplyIgnoredErrorsAdjustsRatesKeepsAbsoluteVolume(t *testing.T) {
	m := service.ChannelMonitorV2Metric{
		RequestCount:  100,
		ErrorRequests: 20,
		ErrorRate:     0.20,
		SuccessRate:   0.80,
REDACTED
	// Success absolute still 80 → success rate stays 0.80 even after ignoring 5 errors.
	m.SuccessRequests = 80
	applyIgnoredErrors(&m, 5)
	require.Equal(t, int64(100), m.RequestCount)
	require.Equal(t, int64(20), m.ErrorRequests)
	require.InDelta(t, 0.15, m.ErrorRate, 0.0001)
	require.InDelta(t, 0.80, m.SuccessRate, 0.0001)

	// Clamp ignored > errors: scored error_rate → 0; success stays true ratio.
	m2 := service.ChannelMonitorV2Metric{RequestCount: 10, ErrorRequests: 2, SuccessRequests: 8, ErrorRate: 0.2, SuccessRate: 0.8REDACTED
	applyIgnoredErrors(&m2, 99)
	require.InDelta(t, 0.0, m2.ErrorRate, 0.0001)
	require.InDelta(t, 0.8, m2.SuccessRate, 0.0001)

	// No-op when ignored is zero
	m3 := service.ChannelMonitorV2Metric{RequestCount: 10, ErrorRequests: 2, SuccessRequests: 8, ErrorRate: 0.2, SuccessRate: 0.8REDACTED
	applyIgnoredErrors(&m3, 0)
	require.InDelta(t, 0.2, m3.ErrorRate, 0.0001)
	require.InDelta(t, 0.8, m3.SuccessRate, 0.0001)
REDACTED

func TestRedactChannelMonitorV2MetricZerosVolume(t *testing.T) {
	// Service helper is in service package; covered there. Keep a smoke note that
	// rates survive a manual zeroing of volume fields used by the UI contract.
	m := service.ChannelMonitorV2Metric{
		RequestCount: 100, ErrorRequests: 10, SuccessRequests: 90,
		TokenCount: 1000, RPM: 5, TPM: 50, ErrorRate: 0.1, SuccessRate: 0.9, CacheRate: 0.4,
REDACTED
	// Mimic redact: zero volume only
	m.RequestCount, m.ErrorRequests, m.SuccessRequests, m.TokenCount = 0, 0, 0, 0
	require.Equal(t, 0.1, m.ErrorRate)
	require.Equal(t, 5.0, m.RPM)
REDACTED

func TestChannelMonitorV2CatalogFilterClearsMultiSelectDimensions(t *testing.T) {
	start := time.Unix(1, 0)
	end := time.Unix(2, 0)
	filter := service.ChannelMonitorV2Filter{
		Start: start, End: end, Bucket: time.Minute,
		Platforms: []string{"openai"REDACTED, GroupIDs: []int64{3REDACTED, Models: []string{"gpt-5"REDACTED,
REDACTED
	catalog := channelMonitorV2CatalogFilter(filter)
	require.Nil(t, catalog.Platforms)
	require.Nil(t, catalog.GroupIDs)
	require.Nil(t, catalog.Models)
	// Time window / coverage-related fields remain.
	require.Equal(t, start, catalog.Start)
	require.Equal(t, end, catalog.End)
	require.Equal(t, time.Minute, catalog.Bucket)

	cfg := service.ChannelMonitorV2Config{
		Platforms: []service.ChannelMonitorV2PlatformConfig{
			{Platform: "openai", Enabled: trueREDACTED,
			{Platform: "grok", Enabled: trueREDACTED,
	REDACTED,
		GroupIDs: []int64{3, 4REDACTED,
REDACTED
	catalogWhere, catalogArgs := channelMonitorV2Where(catalog, cfg, "m")
	_, metricArgs := channelMonitorV2Where(filter, cfg, "m")

	// Catalog WHERE still applies config scope (enabled platforms + group allow-list).
	require.Contains(t, catalogWhere, "m.platform = ANY")
	require.Contains(t, catalogWhere, "m.group_id = ANY")
	require.Len(t, catalogArgs, 4) // start, end, platforms, groups
	require.Len(t, metricArgs, 4)

	// Metrics WHERE is narrower once multi-select platforms/groups are applied.
	require.NotEqual(t, catalogArgs, metricArgs)
	// Group seeding without multi-select uses full config allow-list.
	require.Equal(t, []int64{3, 4REDACTED, configuredChannelMonitorV2GroupIDs(catalog, cfg))
	require.Equal(t, []int64{3REDACTED, configuredChannelMonitorV2GroupIDs(filter, cfg))
REDACTED
