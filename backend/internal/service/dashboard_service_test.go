package service

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/usagestats"
	"github.com/stretchr/testify/require"
)

type usageRepoStub struct {
	UsageLogRepository
	stats      *usagestats.DashboardStats
	rangeStats *usagestats.DashboardStats
	err        error
	rangeErr   error
	calls      int32
	rangeCalls int32
	rangeStart time.Time
	rangeEnd   time.Time
	onCall     chan struct{REDACTED
REDACTED

func (s *usageRepoStub) GetDashboardStats(ctx context.Context) (*usagestats.DashboardStats, error) {
	atomic.AddInt32(&s.calls, 1)
	if s.onCall != nil {
		select {
		case s.onCall <- struct{REDACTED{REDACTED:
		default:
	REDACTED
REDACTED
	if s.err != nil {
		return nil, s.err
REDACTED
	return s.stats, nil
REDACTED

func (s *usageRepoStub) GetDashboardStatsWithRange(ctx context.Context, start, end time.Time) (*usagestats.DashboardStats, error) {
	atomic.AddInt32(&s.rangeCalls, 1)
	s.rangeStart = start
	s.rangeEnd = end
	if s.rangeErr != nil {
		return nil, s.rangeErr
REDACTED
	if s.rangeStats != nil {
		return s.rangeStats, nil
REDACTED
	return s.stats, nil
REDACTED

type dashboardCacheStub struct {
	get       func(ctx context.Context) (string, error)
	set       func(ctx context.Context, data string, ttl time.Duration) error
	del       func(ctx context.Context) error
	getCalls  int32
	setCalls  int32
	delCalls  int32
	lastSetMu sync.Mutex
	lastSet   string
REDACTED

func (c *dashboardCacheStub) GetDashboardStats(ctx context.Context) (string, error) {
	atomic.AddInt32(&c.getCalls, 1)
	if c.get != nil {
		return c.get(ctx)
REDACTED
	return "", ErrDashboardStatsCacheMiss
REDACTED

func (c *dashboardCacheStub) SetDashboardStats(ctx context.Context, data string, ttl time.Duration) error {
	atomic.AddInt32(&c.setCalls, 1)
	c.lastSetMu.Lock()
	c.lastSet = data
	c.lastSetMu.Unlock()
	if c.set != nil {
		return c.set(ctx, data, ttl)
REDACTED
	return nil
REDACTED

func (c *dashboardCacheStub) DeleteDashboardStats(ctx context.Context) error {
	atomic.AddInt32(&c.delCalls, 1)
	if c.del != nil {
		return c.del(ctx)
REDACTED
	return nil
REDACTED

type dashboardAggregationRepoStub struct {
	watermark time.Time
	err       error
REDACTED

func (s *dashboardAggregationRepoStub) AggregateRange(ctx context.Context, start, end time.Time) error {
	return nil
REDACTED

func (s *dashboardAggregationRepoStub) RecomputeRange(ctx context.Context, start, end time.Time) error {
	return nil
REDACTED

func (s *dashboardAggregationRepoStub) GetAggregationWatermark(ctx context.Context) (time.Time, error) {
	if s.err != nil {
		return time.Time{REDACTED, s.err
REDACTED
	return s.watermark, nil
REDACTED

func (s *dashboardAggregationRepoStub) UpdateAggregationWatermark(ctx context.Context, aggregatedAt time.Time) error {
	return nil
REDACTED

func (s *dashboardAggregationRepoStub) CleanupAggregates(ctx context.Context, hourlyCutoff, dailyCutoff time.Time) error {
	return nil
REDACTED

func (s *dashboardAggregationRepoStub) CleanupUsageLogs(ctx context.Context, cutoff time.Time) error {
	return nil
REDACTED

func (s *dashboardAggregationRepoStub) CleanupUsageBillingDedup(ctx context.Context, cutoff time.Time) error {
	return nil
REDACTED

func (s *dashboardAggregationRepoStub) EnsureUsageLogsPartitions(ctx context.Context, now time.Time) error {
	return nil
REDACTED

func (c *dashboardCacheStub) readLastEntry(t *testing.T) dashboardStatsCacheEntry {
REDACTED
	c.lastSetMu.Lock()
	data := c.lastSet
	c.lastSetMu.Unlock()

	var entry dashboardStatsCacheEntry
	err := json.Unmarshal([]byte(data), &entry)
REDACTED
	return entry
REDACTED

func TestDashboardService_CacheHitFresh(t *testing.T) {
	stats := &usagestats.DashboardStats{
		TotalUsers:     10,
		StatsUpdatedAt: time.Unix(0, 0).UTC().Format(time.RFC3339),
		StatsStale:     true,
REDACTED
	entry := dashboardStatsCacheEntry{
		Stats:     stats,
		UpdatedAt: time.Now().Unix(),
REDACTED
	payload, err := json.Marshal(entry)
REDACTED

	cache := &dashboardCacheStub{
		get: func(ctx context.Context) (string, error) {
			return string(payload), nil
	REDACTED,
REDACTED
	repo := &usageRepoStub{
		stats: &usagestats.DashboardStats{TotalUsers: 99REDACTED,
REDACTED
	aggRepo := &dashboardAggregationRepoStub{watermark: time.Unix(0, 0).UTC()REDACTED
	cfg := &config.Config{
		Dashboard: config.DashboardCacheConfig{Enabled: trueREDACTED,
		DashboardAgg: config.DashboardAggregationConfig{
			Enabled: true,
	REDACTED,
REDACTED
	svc := NewDashboardService(repo, aggRepo, cache, cfg)

	got, err := svc.GetDashboardStats(context.Background())
REDACTED
	require.Equal(t, stats, got)
	require.Equal(t, int32(0), atomic.LoadInt32(&repo.calls))
	require.Equal(t, int32(1), atomic.LoadInt32(&cache.getCalls))
	require.Equal(t, int32(0), atomic.LoadInt32(&cache.setCalls))
REDACTED

func TestDashboardService_CacheMiss_StoresCache(t *testing.T) {
	stats := &usagestats.DashboardStats{
		TotalUsers:     7,
		StatsUpdatedAt: time.Unix(0, 0).UTC().Format(time.RFC3339),
		StatsStale:     true,
REDACTED
	cache := &dashboardCacheStub{
		get: func(ctx context.Context) (string, error) {
			return "", ErrDashboardStatsCacheMiss
	REDACTED,
REDACTED
	repo := &usageRepoStub{stats: statsREDACTED
	aggRepo := &dashboardAggregationRepoStub{watermark: time.Unix(0, 0).UTC()REDACTED
	cfg := &config.Config{
		Dashboard: config.DashboardCacheConfig{Enabled: trueREDACTED,
		DashboardAgg: config.DashboardAggregationConfig{
			Enabled: true,
	REDACTED,
REDACTED
	svc := NewDashboardService(repo, aggRepo, cache, cfg)

	got, err := svc.GetDashboardStats(context.Background())
REDACTED
	require.Equal(t, stats, got)
	require.Equal(t, int32(1), atomic.LoadInt32(&repo.calls))
	require.Equal(t, int32(1), atomic.LoadInt32(&cache.getCalls))
	require.Equal(t, int32(1), atomic.LoadInt32(&cache.setCalls))
	entry := cache.readLastEntry(t)
	require.Equal(t, stats, entry.Stats)
	require.WithinDuration(t, time.Now(), time.Unix(entry.UpdatedAt, 0), time.Second)
REDACTED

func TestDashboardService_CacheDisabled_SkipsCache(t *testing.T) {
	stats := &usagestats.DashboardStats{
		TotalUsers:     3,
		StatsUpdatedAt: time.Unix(0, 0).UTC().Format(time.RFC3339),
		StatsStale:     true,
REDACTED
	cache := &dashboardCacheStub{
		get: func(ctx context.Context) (string, error) {
			return "", nil
	REDACTED,
REDACTED
	repo := &usageRepoStub{stats: statsREDACTED
	aggRepo := &dashboardAggregationRepoStub{watermark: time.Unix(0, 0).UTC()REDACTED
	cfg := &config.Config{
		Dashboard: config.DashboardCacheConfig{Enabled: falseREDACTED,
		DashboardAgg: config.DashboardAggregationConfig{
			Enabled: true,
	REDACTED,
REDACTED
	svc := NewDashboardService(repo, aggRepo, cache, cfg)

	got, err := svc.GetDashboardStats(context.Background())
REDACTED
	require.Equal(t, stats, got)
	require.Equal(t, int32(1), atomic.LoadInt32(&repo.calls))
	require.Equal(t, int32(0), atomic.LoadInt32(&cache.getCalls))
	require.Equal(t, int32(0), atomic.LoadInt32(&cache.setCalls))
REDACTED

func TestDashboardService_CacheHitStale_TriggersAsyncRefresh(t *testing.T) {
	staleStats := &usagestats.DashboardStats{
		TotalUsers:     11,
		StatsUpdatedAt: time.Unix(0, 0).UTC().Format(time.RFC3339),
		StatsStale:     true,
REDACTED
	entry := dashboardStatsCacheEntry{
		Stats:     staleStats,
		UpdatedAt: time.Now().Add(-defaultDashboardStatsFreshTTL * 2).Unix(),
REDACTED
	payload, err := json.Marshal(entry)
REDACTED

	cache := &dashboardCacheStub{
		get: func(ctx context.Context) (string, error) {
			return string(payload), nil
	REDACTED,
REDACTED
	refreshCh := make(chan struct{REDACTED, 1)
	repo := &usageRepoStub{
		stats:  &usagestats.DashboardStats{TotalUsers: 22REDACTED,
		onCall: refreshCh,
REDACTED
	aggRepo := &dashboardAggregationRepoStub{watermark: time.Unix(0, 0).UTC()REDACTED
	cfg := &config.Config{
		Dashboard: config.DashboardCacheConfig{Enabled: trueREDACTED,
		DashboardAgg: config.DashboardAggregationConfig{
			Enabled: true,
	REDACTED,
REDACTED
	svc := NewDashboardService(repo, aggRepo, cache, cfg)

	got, err := svc.GetDashboardStats(context.Background())
REDACTED
	require.Equal(t, staleStats, got)

	select {
	case <-refreshCh:
	case <-time.After(1 * time.Second):
		t.Fatal("等待异步刷新超时")
REDACTED
	require.Eventually(t, func() bool {
		return atomic.LoadInt32(&cache.setCalls) >= 1
REDACTED, 1*time.Second, 10*time.Millisecond)
REDACTED

func TestDashboardService_CacheParseError_EvictsAndRefetches(t *testing.T) {
	cache := &dashboardCacheStub{
		get: func(ctx context.Context) (string, error) {
			return "not-json", nil
	REDACTED,
REDACTED
	stats := &usagestats.DashboardStats{TotalUsers: 9REDACTED
	repo := &usageRepoStub{stats: statsREDACTED
	aggRepo := &dashboardAggregationRepoStub{watermark: time.Unix(0, 0).UTC()REDACTED
	cfg := &config.Config{
		Dashboard: config.DashboardCacheConfig{Enabled: trueREDACTED,
		DashboardAgg: config.DashboardAggregationConfig{
			Enabled: true,
	REDACTED,
REDACTED
	svc := NewDashboardService(repo, aggRepo, cache, cfg)

	got, err := svc.GetDashboardStats(context.Background())
REDACTED
	require.Equal(t, stats, got)
	require.Equal(t, int32(1), atomic.LoadInt32(&cache.delCalls))
	require.Equal(t, int32(1), atomic.LoadInt32(&repo.calls))
REDACTED

func TestDashboardService_CacheParseError_RepoFailure(t *testing.T) {
	cache := &dashboardCacheStub{
		get: func(ctx context.Context) (string, error) {
			return "not-json", nil
	REDACTED,
REDACTED
	repo := &usageRepoStub{err: errors.New("db down")REDACTED
	aggRepo := &dashboardAggregationRepoStub{watermark: time.Unix(0, 0).UTC()REDACTED
	cfg := &config.Config{
		Dashboard: config.DashboardCacheConfig{Enabled: trueREDACTED,
		DashboardAgg: config.DashboardAggregationConfig{
			Enabled: true,
	REDACTED,
REDACTED
	svc := NewDashboardService(repo, aggRepo, cache, cfg)

	_, err := svc.GetDashboardStats(context.Background())
REDACTED
	require.Equal(t, int32(1), atomic.LoadInt32(&cache.delCalls))
REDACTED

func TestDashboardService_StatsUpdatedAtEpochWhenMissing(t *testing.T) {
	stats := &usagestats.DashboardStats{REDACTED
	repo := &usageRepoStub{stats: statsREDACTED
	aggRepo := &dashboardAggregationRepoStub{watermark: time.Unix(0, 0).UTC()REDACTED
	cfg := &config.Config{Dashboard: config.DashboardCacheConfig{Enabled: falseREDACTEDREDACTED
	svc := NewDashboardService(repo, aggRepo, nil, cfg)

	got, err := svc.GetDashboardStats(context.Background())
REDACTED
	require.Equal(t, "1970-01-01T00:00:00Z", got.StatsUpdatedAt)
	require.True(t, got.StatsStale)
REDACTED

func TestDashboardService_StatsStaleFalseWhenFresh(t *testing.T) {
	aggNow := time.Now().UTC().Truncate(time.Second)
	stats := &usagestats.DashboardStats{REDACTED
	repo := &usageRepoStub{stats: statsREDACTED
	aggRepo := &dashboardAggregationRepoStub{watermark: aggNowREDACTED
	cfg := &config.Config{
		Dashboard: config.DashboardCacheConfig{Enabled: falseREDACTED,
		DashboardAgg: config.DashboardAggregationConfig{
			Enabled:         true,
			IntervalSeconds: 60,
			LookbackSeconds: 120,
	REDACTED,
REDACTED
	svc := NewDashboardService(repo, aggRepo, nil, cfg)

	got, err := svc.GetDashboardStats(context.Background())
REDACTED
	require.Equal(t, aggNow.Format(time.RFC3339), got.StatsUpdatedAt)
	require.False(t, got.StatsStale)
REDACTED

func TestDashboardService_AggDisabled_UsesUsageLogsFallback(t *testing.T) {
	expected := &usagestats.DashboardStats{TotalUsers: 42REDACTED
	repo := &usageRepoStub{
		rangeStats: expected,
		err:        errors.New("should not call aggregated stats"),
REDACTED
	cfg := &config.Config{
		Dashboard: config.DashboardCacheConfig{Enabled: falseREDACTED,
		DashboardAgg: config.DashboardAggregationConfig{
			Enabled: false,
			Retention: config.DashboardAggregationRetentionConfig{
				UsageLogsDays: 7,
		REDACTED,
	REDACTED,
REDACTED
	svc := NewDashboardService(repo, nil, nil, cfg)

	got, err := svc.GetDashboardStats(context.Background())
REDACTED
	require.Equal(t, int64(42), got.TotalUsers)
	require.Equal(t, int32(0), atomic.LoadInt32(&repo.calls))
	require.Equal(t, int32(1), atomic.LoadInt32(&repo.rangeCalls))
	require.False(t, repo.rangeEnd.IsZero())
	require.Equal(t, truncateToDayUTC(repo.rangeEnd.AddDate(0, 0, -7)), repo.rangeStart)
REDACTED
