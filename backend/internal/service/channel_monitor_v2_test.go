package service

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type channelMonitorV2RepoStub struct {
	config  serviceChannelMonitorV2ConfigAlias
	users   *ChannelMonitorV2List[ChannelMonitorV2UserRow]
	matrix  *ChannelMonitorV2Matrix
	errors  *ChannelMonitorV2List[ChannelMonitorV2ErrorRow]
	snap    *ChannelMonitorV2Snapshot
	group   ChannelMonitorV2GroupBy
	admin   bool
REDACTED

// Alias keeps composite literals readable without introducing another package.
type serviceChannelMonitorV2ConfigAlias = ChannelMonitorV2Config

func (s *channelMonitorV2RepoStub) GetConfig(context.Context) (*ChannelMonitorV2Config, error) {
	cfg := ChannelMonitorV2Config(s.config)
	return &cfg, nil
REDACTED
func (s *channelMonitorV2RepoStub) UpdateConfig(context.Context, ChannelMonitorV2Config, int) (*ChannelMonitorV2Config, error) {
	return nil, nil
REDACTED
func (s *channelMonitorV2RepoStub) GetDimensions(context.Context, ChannelMonitorV2Filter, ChannelMonitorV2Config) (*ChannelMonitorV2Dimensions, error) {
	return nil, nil
REDACTED
func (s *channelMonitorV2RepoStub) GetSnapshot(_ context.Context, _ ChannelMonitorV2Filter, _ ChannelMonitorV2Config, admin bool) (*ChannelMonitorV2Snapshot, error) {
	s.admin = admin
	if s.snap == nil {
		return nil, nil
REDACTED
	// Shallow copy so public redaction does not mutate fixture.
	cfg := s.snap.Config
	cfg.Platforms = append([]ChannelMonitorV2PlatformConfig(nil), s.snap.Config.Platforms...)
	for i := range cfg.Platforms {
		cfg.Platforms[i].Models = append([]string(nil), s.snap.Config.Platforms[i].Models...)
REDACTED
	cfg.GroupIDs = append([]int64(nil), s.snap.Config.GroupIDs...)
	cfg.IgnoredErrorCategories = append([]string(nil), s.snap.Config.IgnoredErrorCategories...)
	out := *s.snap
	out.Config = cfg
	return &out, nil
REDACTED
func (s *channelMonitorV2RepoStub) GetModels(context.Context, ChannelMonitorV2Filter, ChannelMonitorV2Config, bool) (*ChannelMonitorV2List[ChannelMonitorV2ModelRow], error) {
	return nil, nil
REDACTED
func (s *channelMonitorV2RepoStub) GetMatrix(_ context.Context, _ ChannelMonitorV2Filter, _ ChannelMonitorV2Config, groupBy ChannelMonitorV2GroupBy, admin bool) (*ChannelMonitorV2Matrix, error) {
	s.group, s.admin = groupBy, admin
	return s.matrix, nil
REDACTED
func (s *channelMonitorV2RepoStub) GetErrors(_ context.Context, _ ChannelMonitorV2Filter, _ ChannelMonitorV2Config, admin bool) (*ChannelMonitorV2List[ChannelMonitorV2ErrorRow], error) {
	s.admin = admin
	if s.errors != nil {
		// Return a shallow copy so service redaction does not mutate the fixture.
		out := &ChannelMonitorV2List[ChannelMonitorV2ErrorRow]{
			Coverage: s.errors.Coverage,
			Items:    append([]ChannelMonitorV2ErrorRow(nil), s.errors.Items...),
	REDACTED
		for i := range out.Items {
			if len(out.Items[i].Details) > 0 {
				out.Items[i].Details = append([]ChannelMonitorV2ErrorDetail(nil), out.Items[i].Details...)
		REDACTED
	REDACTED
		return out, nil
REDACTED
	return nil, nil
REDACTED
func (s *channelMonitorV2RepoStub) GetUsers(context.Context, ChannelMonitorV2Filter, ChannelMonitorV2Config, bool) (*ChannelMonitorV2List[ChannelMonitorV2UserRow], error) {
	return s.users, nil
REDACTED
func (s *channelMonitorV2RepoStub) RecomputeRange(context.Context, time.Time, time.Time) error {
	return nil
REDACTED
func (s *channelMonitorV2RepoStub) GetAggregationWatermark(context.Context) (*ChannelMonitorV2AggregationWatermark, error) {
	return &ChannelMonitorV2AggregationWatermark{REDACTED, nil
REDACTED

func TestChannelMonitorV2BootstrapProgress(t *testing.T) {
	now := time.Date(2026, 8, 7, 12, 0, 0, 0, time.UTC)

	t.Run("no data shows active zero progress", func(t *testing.T) {
		b := ChannelMonitorV2BootstrapProgress(now, time.Time{REDACTED, false)
		require.NotNil(t, b)
		require.True(t, b.Active)
		require.Equal(t, 0, b.ProgressPercent)
		require.Equal(t, now.Add(-ChannelMonitorV2BootstrapProductWindow), b.TargetStart)
REDACTED)

	t.Run("2h seed is small percent of 30d", func(t *testing.T) {
		covered := now.Add(-2 * time.Hour)
		b := ChannelMonitorV2BootstrapProgress(now, covered, true)
		require.NotNil(t, b)
		require.True(t, b.Active)
		require.Greater(t, b.ProgressPercent, 0)
		require.Less(t, b.ProgressPercent, 5)
REDACTED)

	t.Run("30d covered hides bootstrap", func(t *testing.T) {
		covered := now.Add(-ChannelMonitorV2BootstrapProductWindow)
		b := ChannelMonitorV2BootstrapProgress(now, covered, true)
		require.Nil(t, b)
REDACTED)

	t.Run("beyond 30d also hides", func(t *testing.T) {
		covered := now.Add(-60 * 24 * time.Hour)
		b := ChannelMonitorV2BootstrapProgress(now, covered, true)
		require.Nil(t, b)
REDACTED)
REDACTED

func TestChannelMonitorV2ParseFilterDefaultsAndBuckets(t *testing.T) {
	svc := &ChannelMonitorV2Service{now: func() time.Time { return time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC) REDACTEDREDACTED

	filter, err := svc.ParseFilter("", []string{"openai", "openai", ""REDACTED, []string{"gpt-5"REDACTED, []int64{2, 1, 2, 0REDACTED)
REDACTED
	require.Equal(t, "90m", filter.Range)
	require.Equal(t, 5*time.Minute, filter.Bucket)
	require.Equal(t, []string{"openai"REDACTED, filter.Platforms)
	require.Equal(t, []int64{1, 2REDACTED, filter.GroupIDs)
	require.Equal(t, 90*time.Minute, filter.End.Sub(filter.Start))

	filter, err = svc.ParseFilter("30d", nil, nil, nil)
REDACTED
	require.Equal(t, 24*time.Hour, filter.Bucket)
	_, err = svc.ParseFilter("15d", nil, nil, nil)
	require.ErrorIs(t, err, ErrChannelMonitorV2InvalidRange)
REDACTED

func TestParseChannelMonitorV2GroupBy(t *testing.T) {
	groupBy, err := ParseChannelMonitorV2GroupBy("")
REDACTED
	require.Equal(t, ChannelMonitorV2GroupByPlatformGroup, groupBy)
	for _, value := range []ChannelMonitorV2GroupBy{ChannelMonitorV2GroupByPlatform, ChannelMonitorV2GroupByPlatformGroup, ChannelMonitorV2GroupByPlatformModel, ChannelMonitorV2GroupByPlatformGroupModelREDACTED {
		parsed, parseErr := ParseChannelMonitorV2GroupBy(string(value))
		require.NoError(t, parseErr)
		require.Equal(t, value, parsed)
REDACTED
	_, err = ParseChannelMonitorV2GroupBy("group")
	require.ErrorIs(t, err, ErrChannelMonitorV2InvalidGroupBy)
REDACTED

func TestChannelMonitorV2MatrixForwardsGroupingAndAdminScope(t *testing.T) {
	want := &ChannelMonitorV2Matrix{GroupBy: ChannelMonitorV2GroupByPlatformModelREDACTED
	repo := &channelMonitorV2RepoStub{config: ChannelMonitorV2Config{Enabled: trueREDACTED, matrix: wantREDACTED
	result, err := NewChannelMonitorV2Service(repo).Matrix(context.Background(), ChannelMonitorV2Filter{REDACTED, ChannelMonitorV2GroupByPlatformModel, true)
REDACTED
	require.Same(t, want, result)
	require.Equal(t, ChannelMonitorV2GroupByPlatformModel, repo.group)
	require.True(t, repo.admin)

	_, err = NewChannelMonitorV2Service(repo).Matrix(context.Background(), ChannelMonitorV2Filter{REDACTED, "bad", false)
	require.ErrorIs(t, err, ErrChannelMonitorV2InvalidGroupBy)
REDACTED

func TestChannelMonitorV2ConfigValidation(t *testing.T) {
	cfg := ChannelMonitorV2Config{
		Platforms: []ChannelMonitorV2PlatformConfig{
			{Platform: " OpenAI ", Enabled: true, Models: []string{"gpt-5", "gpt-5", ""REDACTEDREDACTED,
			{Platform: "anthropic", Enabled: trueREDACTED,
	REDACTED,
		GroupIDs: []int64{3, 1, 3REDACTED,
REDACTED
	require.NoError(t, normalizeChannelMonitorV2Config(&cfg))
	require.Equal(t, 300, cfg.RefreshIntervalSeconds)
	require.Equal(t, "anthropic", cfg.Platforms[0].Platform)
	require.Equal(t, []int64{1, 3REDACTED, cfg.GroupIDs)

	cfg.RefreshIntervalSeconds = 120
	require.ErrorIs(t, normalizeChannelMonitorV2Config(&cfg), ErrChannelMonitorV2InvalidConfig)

	cfg.RefreshIntervalSeconds = 60
	cfg.GroupIDs = []int64{0REDACTED
	require.ErrorIs(t, normalizeChannelMonitorV2Config(&cfg), ErrChannelMonitorV2InvalidConfig)
REDACTED

func TestChannelMonitorV2ErrorTaxonomyPriority(t *testing.T) {
	tests := []struct {
		name string
		in   ChannelMonitorV2ErrorInput
		want string
REDACTED{
		{"cyber", ChannelMonitorV2ErrorInput{ErrorType: "cyber_policy", StatusCode: 200REDACTED, "content_policy"REDACTED,
		{"auth before forbidden", ChannelMonitorV2ErrorInput{StatusCode: 403, Message: "invalid API key"REDACTED, "authentication"REDACTED,
		{"context", ChannelMonitorV2ErrorInput{Message: "maximum prompt length exceeded"REDACTED, "context_limit"REDACTED,
		{"unsupported", ChannelMonitorV2ErrorInput{Message: "not supported by any configured account"REDACTED, "model_unsupported"REDACTED,
		{"pool", ChannelMonitorV2ErrorInput{Message: "No available accounts"REDACTED, "account_pool_unavailable"REDACTED,
		{"timeout", ChannelMonitorV2ErrorInput{Message: "error code: 524"REDACTED, "timeout"REDACTED,
		{"upstream", ChannelMonitorV2ErrorInput{ErrorOwner: "provider", StatusCode: 502, UpstreamStatusCode: 502REDACTED, "upstream_5xx"REDACTED,
		{"other", ChannelMonitorV2ErrorInput{StatusCode: 400, Message: "unknown"REDACTED, "other"REDACTED,
REDACTED
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) { require.Equal(t, tt.want, ClassifyChannelMonitorV2Error(tt.in)) REDACTED)
REDACTED
REDACTED

func TestChannelMonitorV2HealthBlendsErrorTTFTAndCache(t *testing.T) {
	// error 3%/5% → 40; ttft p50 2s → 100; cache 50% → 50
	// overall = (0.6*40 + 0.2*100 + 0.2*50) / 1.0 = 54 → warning
	p50 := int64(2000)
	p95 := int64(9000)
	thresholds := ChannelMonitorV2HealthThresholds{
		MinimumSample:     20,
		WarningErrorRate:  0.02,
		CriticalErrorRate: 0.05,
		TargetTTFTMs:      2500,
		WarningTTFTMs:     2501,
		CriticalTTFTMs:    6000,
		WarningCacheRate:  0.20,
		CriticalCacheRate: 0.05,
		ErrorWeight:       0.60,
		TTFTWeight:        0.20,
		CacheWeight:       0.20,
REDACTED
	metrics := ChannelMonitorV2Metric{
		RequestCount:         100,
		ErrorRate:            0.03,
		CacheRate:            0.50,
		CacheRateDenominator: 100,
		TTFT:                 ChannelMonitorV2Latency{SampleCount: 100, P50Ms: &p50, P95Ms: &p95REDACTED,
REDACTED
	health := ChannelMonitorV2HealthForWithThresholds(metrics, thresholds)
	require.Equal(t, "warning", health.ErrorRate)
	require.Equal(t, "healthy", health.TTFT)
	require.Equal(t, "healthy", health.Cache)
	require.NotNil(t, health.Score)
	require.NotNil(t, health.CacheScore)
	require.InDelta(t, 50.0, *health.CacheScore, 0.01)
	require.InDelta(t, 54.0, *health.Score, 0.01)
	require.Equal(t, "warning", health.Overall)

	// Perfect signals → 100
	p50OK := int64(1000)
	metrics.ErrorRate = 0
	metrics.CacheRate = 1
	metrics.TTFT.P50Ms = &p50OK
	health = ChannelMonitorV2HealthForWithThresholds(metrics, thresholds)
	require.Equal(t, "healthy", health.Overall)
	require.NotNil(t, health.Score)
	require.InDelta(t, 100.0, *health.Score, 0.01)

	// Small samples stay unknown
	health = ChannelMonitorV2HealthFor(ChannelMonitorV2Metric{RequestCount: 2REDACTED)
	require.Equal(t, "unknown", health.Overall)
	require.Nil(t, health.Score)
REDACTED

func TestChannelMonitorV2DefaultHealthThresholdsAreTolerant(t *testing.T) {
	p50 := int64(2500)
	health := ChannelMonitorV2HealthFor(ChannelMonitorV2Metric{
		RequestCount:         100,
		ErrorRate:            0.03,
		CacheRate:            0,
		CacheRateDenominator: 100,
		TTFT:                 ChannelMonitorV2Latency{SampleCount: 100, P50Ms: &p50REDACTED,
REDACTED)
	require.Equal(t, "healthy", health.ErrorRate)
	require.Equal(t, "healthy", health.TTFT)
	require.Equal(t, "healthy", health.Cache)
	require.Equal(t, "healthy", health.Overall)
	require.NotNil(t, health.CacheScore)
	require.InDelta(t, 100.0, *health.CacheScore, 0.01)
REDACTED

func TestErrorRateTTFTAndCacheScoreHelpers(t *testing.T) {
	require.InDelta(t, 100.0, errorRateScore(0, 0.05), 0.001)
	require.InDelta(t, 0.0, errorRateScore(0.05, 0.05), 0.001)
	require.InDelta(t, 40.0, errorRateScore(0.03, 0.05), 0.001)

	require.InDelta(t, 100.0, ttftP50Score(2500, 2500, 6000), 0.001)
	require.InDelta(t, 100.0, ttftP50Score(1000, 2500, 6000), 0.001)
	require.InDelta(t, 0.0, ttftP50Score(6000, 2500, 6000), 0.001)
	require.InDelta(t, 50.0, ttftP50Score(4250, 2500, 6000), 0.001)

	require.InDelta(t, 0.0, cacheRateScore(0), 0.001)
	require.InDelta(t, 100.0, cacheRateScore(1), 0.001)
	require.InDelta(t, 50.0, cacheRateScore(0.5), 0.001)
	require.Equal(t, "critical", cacheRateBand(0.02, 0.20, 0.05))
	require.Equal(t, "warning", cacheRateBand(0.10, 0.20, 0.05))
	require.Equal(t, "healthy", cacheRateBand(0.50, 0.20, 0.05))
REDACTED

func TestChannelMonitorV2UsersRemovesOtherUserIdentity(t *testing.T) {
	selfID, otherID := int64(7), int64(9)
	repo := &channelMonitorV2RepoStub{config: ChannelMonitorV2Config{Enabled: trueREDACTED, users: &ChannelMonitorV2List[ChannelMonitorV2UserRow]{Items: []ChannelMonitorV2UserRow{
		{UserID: &otherID, Email: "other@example.com", Username: "other"REDACTED,
		{UserID: &selfID, Email: "self@example.com", Username: "self"REDACTED,
REDACTEDREDACTEDREDACTED
	result, err := NewChannelMonitorV2Service(repo).Users(context.Background(), ChannelMonitorV2Filter{REDACTED, selfID, false)
REDACTED
	require.Nil(t, result.Items[0].UserID)
	require.Empty(t, result.Items[0].Email)
	require.Equal(t, "Other user #1", result.Items[0].DisplayLabel)
	require.Equal(t, selfID, *result.Items[1].UserID)
	require.True(t, result.Items[1].IsSelf)
	require.Equal(t, "Me", result.Items[1].DisplayLabel)
REDACTED

func TestChannelMonitorV2UsersAppendsSelfWhenMissingFromRanking(t *testing.T) {
	selfID, otherID := int64(7), int64(9)
	repo := &channelMonitorV2RepoStub{config: ChannelMonitorV2Config{Enabled: trueREDACTED, users: &ChannelMonitorV2List[ChannelMonitorV2UserRow]{Items: []ChannelMonitorV2UserRow{
		{UserID: &otherID, Email: "other@example.com", Username: "other", Metrics: ChannelMonitorV2Metric{RequestCount: 10REDACTEDREDACTED,
REDACTEDREDACTEDREDACTED
	result, err := NewChannelMonitorV2Service(repo).Users(context.Background(), ChannelMonitorV2Filter{REDACTED, selfID, false)
REDACTED
	require.Len(t, result.Items, 2)
	self := result.Items[1]
	require.True(t, self.IsSelf)
	require.Equal(t, "Me", self.DisplayLabel)
	require.Equal(t, selfID, *self.UserID)
	require.Equal(t, 0, self.Rank) // unranked / no traffic in window
REDACTED

func TestChannelMonitorV2TopUsersKeepsSelfOutsideLimit(t *testing.T) {
	items := make([]ChannelMonitorV2UserRow, 0, 12)
	for i := 1; i <= 12; i++ {
		id := int64(i)
		items = append(items, ChannelMonitorV2UserRow{UserID: &id, Rank: i, DisplayLabel: fmt.Sprintf("u%d", i)REDACTED)
REDACTED
	// self is rank 12 (index 11)
	out := channelMonitorV2TopUsersWithSelf(items, 11, 10)
	require.Len(t, out, 11)
	require.Equal(t, int64(12), *out[10].UserID)
	require.Equal(t, 12, out[10].Rank)
REDACTED

func TestChannelMonitorV2ReadAPIsRejectDisabledConfig(t *testing.T) {
	repo := &channelMonitorV2RepoStub{config: ChannelMonitorV2Config{Enabled: falseREDACTEDREDACTED
	_, err := NewChannelMonitorV2Service(repo).Matrix(context.Background(), ChannelMonitorV2Filter{REDACTED, ChannelMonitorV2GroupByPlatform, false)
	require.ErrorIs(t, err, ErrChannelMonitorDisabled)
REDACTED

func TestNormalizeChannelMonitorV2IgnoredCategories(t *testing.T) {
	got := normalizeChannelMonitorV2IgnoredCategories([]string{"timeout", "TIMEOUT", "not-a-real", "", "timeout", "other"REDACTED)
	require.Equal(t, []string{"other", "timeout"REDACTED, got)
REDACTED

func TestApplyIgnoredErrorsAdjustsRateOnly(t *testing.T) {
	// applyIgnoredErrors lives in repository package; mirror the scored-rate formula.
	// scored errors = total_errors - ignored; error_rate = scored / request_count
	// success_rate stays SuccessRequests / RequestCount (ignored ≠ success).
	m := ChannelMonitorV2Metric{RequestCount: 100, ErrorRequests: 20, SuccessRequests: 80, ErrorRate: 0.2, SuccessRate: 0.8REDACTED
	ignored := int64(5)
	counted := m.ErrorRequests - ignored
	errorRate := float64(counted) / float64(m.RequestCount)
	successRate := float64(m.SuccessRequests) / float64(m.RequestCount)
	require.InDelta(t, 0.15, errorRate, 0.0001)
	require.InDelta(t, 0.80, successRate, 0.0001)
	require.Equal(t, int64(20), m.ErrorRequests)
	require.Equal(t, int64(100), m.RequestCount)
REDACTED

func TestErrorsForViewerStripsDetailsAndCountsForNonAdmin(t *testing.T) {
	fixture := &ChannelMonitorV2List[ChannelMonitorV2ErrorRow]{
		Items: []ChannelMonitorV2ErrorRow{{
			Category: "timeout",
			Count:    42,
			Rate:     0.4,
			Details: []ChannelMonitorV2ErrorDetail{{
				Platform:           "anthropic",
				Model:              "claude",
				ErrorType:          "upstream",
				StatusCode:         504,
				UpstreamStatusCode: 504,
				Message:            "gateway timeout sk-secret",
				Count:              12,
	REDACTED
REDACTED
REDACTED
	repo := &channelMonitorV2RepoStub{config: ChannelMonitorV2Config{Enabled: trueREDACTED, errors: fixtureREDACTED
	svc := NewChannelMonitorV2Service(repo)

	userList, err := svc.ErrorsForViewer(context.Background(), ChannelMonitorV2Filter{REDACTED, false)
REDACTED
	require.False(t, repo.admin)
	require.Len(t, userList.Items, 1)
	require.Zero(t, userList.Items[0].Count)
	require.Empty(t, userList.Items[0].Details)
	require.InDelta(t, 0.4, userList.Items[0].Rate, 0.0001)

	adminList, err := svc.ErrorsForViewer(context.Background(), ChannelMonitorV2Filter{REDACTED, true)
REDACTED
	require.True(t, repo.admin)
	require.Equal(t, int64(42), adminList.Items[0].Count)
	require.Len(t, adminList.Items[0].Details, 1)
	require.Contains(t, adminList.Items[0].Details[0].Message, "gateway timeout")
REDACTED

func TestSnapshotRedactsPublicConfigPolicyFields(t *testing.T) {
	updatedBy := int64(9)
	repo := &channelMonitorV2RepoStub{
		config: ChannelMonitorV2Config{Enabled: trueREDACTED,
		snap: &ChannelMonitorV2Snapshot{
			Config: ChannelMonitorV2Config{
				Version:                3,
				Enabled:                true,
				RefreshIntervalSeconds: 300,
				Platforms: []ChannelMonitorV2PlatformConfig{
					{Platform: "openai", Enabled: true, Models: []string{"gpt-5"REDACTEDREDACTED,
			REDACTED,
				GroupIDs:               []int64{1, 2REDACTED,
				IgnoredErrorCategories: []string{"timeout"REDACTED,
				UpdatedBy:              &updatedBy,
		REDACTED,
			Metrics: ChannelMonitorV2Metric{RequestCount: 100, ErrorRate: 0.1, SuccessRate: 0.9, RPM: 5REDACTED,
	REDACTED,
REDACTED
	svc := NewChannelMonitorV2Service(repo)
	snap, err := svc.Snapshot(context.Background(), ChannelMonitorV2Filter{REDACTED, false)
REDACTED
	require.Empty(t, snap.Config.GroupIDs)
	require.Empty(t, snap.Config.IgnoredErrorCategories)
	require.Nil(t, snap.Config.UpdatedBy)
	require.Empty(t, snap.Config.Platforms[0].Models)
	require.Equal(t, 300, snap.Config.RefreshIntervalSeconds)
	require.Zero(t, snap.Metrics.RequestCount)
	require.InDelta(t, 0.1, snap.Metrics.ErrorRate, 0.0001)
	require.InDelta(t, 5.0, snap.Metrics.RPM, 0.0001)
REDACTED

func TestRedactChannelMonitorV2MetricKeepsRates(t *testing.T) {
	p50 := int64(100)
	m := ChannelMonitorV2Metric{
		SuccessRequests: 90, ErrorRequests: 10, RequestCount: 100,
		InputTokens: 1, OutputTokens: 2, CacheCreationTokens: 3, CacheReadTokens: 4,
		TokenCount: 10, RPM: 12, TPM: 34, ErrorRate: 0.1, SuccessRate: 0.9,
		CacheRate: 0.5, CacheRateNumerator: 4, CacheRateDenominator: 8,
		TTFT: ChannelMonitorV2Latency{SampleCount: 50, P50Ms: &p50REDACTED,
REDACTED
	upstream := int64(3)
	m.UpstreamAffectedRequests = &upstream
	redactChannelMonitorV2Metric(&m, false)
	require.Zero(t, m.RequestCount)
	require.Zero(t, m.ErrorRequests)
	require.Zero(t, m.TokenCount)
	require.Zero(t, m.CacheRateNumerator)
	require.Zero(t, m.TTFT.SampleCount)
	require.Nil(t, m.UpstreamAffectedRequests)
	require.InDelta(t, 0.1, m.ErrorRate, 0.0001)
	require.InDelta(t, 12.0, m.RPM, 0.0001)
	require.InDelta(t, 34.0, m.TPM, 0.0001)
	require.NotNil(t, m.TTFT.P50Ms)
	require.Equal(t, int64(100), *m.TTFT.P50Ms)

	redactChannelMonitorV2Metric(&m, true)
	require.Zero(t, m.RPM)
	require.Zero(t, m.TPM)
	require.InDelta(t, 0.1, m.ErrorRate, 0.0001)
REDACTED

func TestChannelMonitorV2HealthTTFTAtTargetIsHealthy(t *testing.T) {
	p50 := int64(2500)
	m := ChannelMonitorV2Metric{
		RequestCount: 100, ErrorRate: 0,
		CacheRate: 1, CacheRateDenominator: 100,
		TTFT: ChannelMonitorV2Latency{SampleCount: 100, P50Ms: &p50REDACTED,
REDACTED
	h := ChannelMonitorV2HealthFor(m)
	require.Equal(t, "healthy", h.TTFT)
	require.NotNil(t, h.TTFTScore)
	require.InDelta(t, 100.0, *h.TTFTScore, 0.01)
REDACTED
