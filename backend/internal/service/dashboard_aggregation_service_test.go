package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

type dashboardAggregationRepoTestStub struct {
	aggregateCalls       int
	lastStart            time.Time
	lastEnd              time.Time
	watermark            time.Time
	aggregateErr         error
	cleanupAggregatesErr error
	cleanupUsageErr      error
REDACTED

func (s *dashboardAggregationRepoTestStub) AggregateRange(ctx context.Context, start, end time.Time) error {
	s.aggregateCalls++
	s.lastStart = start
	s.lastEnd = end
	return s.aggregateErr
REDACTED

func (s *dashboardAggregationRepoTestStub) GetAggregationWatermark(ctx context.Context) (time.Time, error) {
	return s.watermark, nil
REDACTED

func (s *dashboardAggregationRepoTestStub) UpdateAggregationWatermark(ctx context.Context, aggregatedAt time.Time) error {
	return nil
REDACTED

func (s *dashboardAggregationRepoTestStub) CleanupAggregates(ctx context.Context, hourlyCutoff, dailyCutoff time.Time) error {
	return s.cleanupAggregatesErr
REDACTED

func (s *dashboardAggregationRepoTestStub) CleanupUsageLogs(ctx context.Context, cutoff time.Time) error {
	return s.cleanupUsageErr
REDACTED

func (s *dashboardAggregationRepoTestStub) EnsureUsageLogsPartitions(ctx context.Context, now time.Time) error {
	return nil
REDACTED

func TestDashboardAggregationService_RunScheduledAggregation_EpochUsesRetentionStart(t *testing.T) {
	repo := &dashboardAggregationRepoTestStub{watermark: time.Unix(0, 0).UTC()REDACTED
	svc := &DashboardAggregationService{
		repo: repo,
		cfg: config.DashboardAggregationConfig{
			Enabled:         true,
			IntervalSeconds: 60,
			LookbackSeconds: 120,
			Retention: config.DashboardAggregationRetentionConfig{
				UsageLogsDays: 1,
				HourlyDays:    1,
				DailyDays:     1,
		REDACTED,
	REDACTED,
REDACTED

	svc.runScheduledAggregation()

	require.Equal(t, 1, repo.aggregateCalls)
	require.False(t, repo.lastEnd.IsZero())
	require.Equal(t, truncateToDayUTC(repo.lastEnd.AddDate(0, 0, -1)), repo.lastStart)
REDACTED

func TestDashboardAggregationService_CleanupRetentionFailure_DoesNotRecord(t *testing.T) {
	repo := &dashboardAggregationRepoTestStub{cleanupAggregatesErr: errors.New("清理失败")REDACTED
	svc := &DashboardAggregationService{
		repo: repo,
		cfg: config.DashboardAggregationConfig{
			Retention: config.DashboardAggregationRetentionConfig{
				UsageLogsDays: 1,
				HourlyDays:    1,
				DailyDays:     1,
		REDACTED,
	REDACTED,
REDACTED

	svc.maybeCleanupRetention(context.Background(), time.Now().UTC())

	require.Nil(t, svc.lastRetentionCleanup.Load())
REDACTED

func TestDashboardAggregationService_TriggerBackfill_TooLarge(t *testing.T) {
	repo := &dashboardAggregationRepoTestStub{REDACTED
	svc := &DashboardAggregationService{
		repo: repo,
		cfg: config.DashboardAggregationConfig{
			BackfillEnabled: true,
			BackfillMaxDays: 1,
	REDACTED,
REDACTED

	start := time.Now().AddDate(0, 0, -3)
	end := time.Now()
	err := svc.TriggerBackfill(start, end)
	require.ErrorIs(t, err, ErrDashboardBackfillTooLarge)
	require.Equal(t, 0, repo.aggregateCalls)
REDACTED
