//go:build integration

package service

import (
	"context"
	"database/sql"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// This integration test protects the DI startup contract for OpsAlertService.
//
// Background:
//   - OpsMetricsCollector previously called alertService.Start()/Evaluate() directly.
//   - Those direct calls were removed, so OpsAlertService must now start via DI
//     (ProvideOpsAlertService in wire.go) and run its own evaluation ticker.
//
// What we validate here:
//  1. When we construct via the Wire provider functions (ProvideOpsAlertService +
//     ProvideOpsMetricsCollector), OpsAlertService starts automatically.
//  2. Its evaluation loop continues to tick even if OpsMetricsCollector is stopped,
//     proving the alert evaluator is independent.
//  3. The evaluation path can trigger alert logic (CreateAlertEvent called).
func TestOpsAlertService_StartedViaWireProviders_RunsIndependentTicker(t *testing.T) {
	oldInterval := opsAlertEvalInterval
	opsAlertEvalInterval = 25 * time.Millisecond
	t.Cleanup(func() { opsAlertEvalInterval = oldInterval REDACTED)

	repo := newFakeOpsRepository()
	opsService := NewOpsService(repo, nil)

	// Start via the Wire provider function (the production DI path).
	alertService := ProvideOpsAlertService(opsService, nil, nil)
	t.Cleanup(alertService.Stop)

	// Construct via ProvideOpsMetricsCollector (wire.go). Stop immediately to ensure
	// the alert ticker keeps running without the metrics collector.
	collector := ProvideOpsMetricsCollector(opsService, NewConcurrencyService(nil))
	collector.Stop()

	// Wait for at least one evaluation (run() calls evaluateOnce immediately).
	require.Eventually(t, func() bool {
		return repo.listRulesCalls.Load() >= 1
REDACTED, 1*time.Second, 5*time.Millisecond)

	// Confirm the evaluation loop keeps ticking after the metrics collector is stopped.
	callsAfterCollectorStop := repo.listRulesCalls.Load()
	require.Eventually(t, func() bool {
		return repo.listRulesCalls.Load() >= callsAfterCollectorStop+2
REDACTED, 1*time.Second, 5*time.Millisecond)

	// Confirm the evaluation logic actually fires an alert event at least once.
	select {
	case <-repo.eventCreatedCh:
		// ok
	case <-time.After(2 * time.Second):
		t.Fatalf("expected OpsAlertService to create an alert event, but none was created (ListAlertRules calls=%d)", repo.listRulesCalls.Load())
REDACTED
REDACTED

func newFakeOpsRepository() *fakeOpsRepository {
	return &fakeOpsRepository{
		eventCreatedCh: make(chan struct{REDACTED),
REDACTED
REDACTED

// fakeOpsRepository is a lightweight in-memory stub of OpsRepository for integration tests.
// It avoids real DB/Redis usage and provides deterministic responses fast.
type fakeOpsRepository struct {
	listRulesCalls atomic.Int64

	mu             sync.Mutex
	activeEvent    *OpsAlertEvent
	latestEvent    *OpsAlertEvent
	nextEventID    int64
	eventCreatedCh chan struct{REDACTED
	eventOnce      sync.Once
REDACTED

func (r *fakeOpsRepository) CreateErrorLog(ctx context.Context, log *OpsErrorLog) error {
	return nil
REDACTED

func (r *fakeOpsRepository) ListErrorLogsLegacy(ctx context.Context, filters OpsErrorLogFilters) ([]OpsErrorLog, error) {
	return nil, nil
REDACTED

func (r *fakeOpsRepository) ListErrorLogs(ctx context.Context, filter *ErrorLogFilter) ([]*ErrorLog, int64, error) {
	return nil, 0, nil
REDACTED

func (r *fakeOpsRepository) GetLatestSystemMetric(ctx context.Context) (*OpsMetrics, error) {
	return &OpsMetrics{WindowMinutes: 1REDACTED, sql.ErrNoRows
REDACTED

func (r *fakeOpsRepository) CreateSystemMetric(ctx context.Context, metric *OpsMetrics) error {
	return nil
REDACTED

func (r *fakeOpsRepository) GetWindowStats(ctx context.Context, startTime, endTime time.Time) (*OpsWindowStats, error) {
	return &OpsWindowStats{REDACTED, nil
REDACTED

func (r *fakeOpsRepository) GetProviderStats(ctx context.Context, startTime, endTime time.Time) ([]*ProviderStats, error) {
	return nil, nil
REDACTED

func (r *fakeOpsRepository) GetLatencyHistogram(ctx context.Context, startTime, endTime time.Time) ([]*LatencyHistogramItem, error) {
	return nil, nil
REDACTED

func (r *fakeOpsRepository) GetErrorDistribution(ctx context.Context, startTime, endTime time.Time) ([]*ErrorDistributionItem, error) {
	return nil, nil
REDACTED

func (r *fakeOpsRepository) ListRecentSystemMetrics(ctx context.Context, windowMinutes, limit int) ([]OpsMetrics, error) {
	if limit <= 0 {
		limit = 1
REDACTED
	now := time.Now()
	metrics := make([]OpsMetrics, 0, limit)
	for i := 0; i < limit; i++ {
		metrics = append(metrics, OpsMetrics{
			WindowMinutes:   windowMinutes,
			CPUUsagePercent: 99,
			UpdatedAt:       now.Add(-time.Duration(i) * opsMetricsInterval),
	REDACTED)
REDACTED
	return metrics, nil
REDACTED

func (r *fakeOpsRepository) ListSystemMetricsRange(ctx context.Context, windowMinutes int, startTime, endTime time.Time, limit int) ([]OpsMetrics, error) {
	return nil, nil
REDACTED

func (r *fakeOpsRepository) ListAlertRules(ctx context.Context) ([]OpsAlertRule, error) {
	call := r.listRulesCalls.Add(1)
	// Delay enabling rules slightly so the test can stop OpsMetricsCollector first,
	// then observe the alert evaluator ticking independently.
	if call < 5 {
		return nil, nil
REDACTED
	return []OpsAlertRule{
		{
			ID:               1,
			Name:             "cpu too high (test)",
			Enabled:          true,
			MetricType:       OpsMetricCPUUsagePercent,
			Operator:         ">",
			Threshold:        0,
			WindowMinutes:    1,
			SustainedMinutes: 1,
			Severity:         "P1",
			NotifyEmail:      false,
			NotifyWebhook:    false,
			CooldownMinutes:  0,
	REDACTED,
REDACTED, nil
REDACTED

func (r *fakeOpsRepository) GetActiveAlertEvent(ctx context.Context, ruleID int64) (*OpsAlertEvent, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.activeEvent == nil {
		return nil, nil
REDACTED
	if r.activeEvent.RuleID != ruleID {
		return nil, nil
REDACTED
	if r.activeEvent.Status != OpsAlertStatusFiring {
		return nil, nil
REDACTED
	clone := *r.activeEvent
	return &clone, nil
REDACTED

func (r *fakeOpsRepository) GetLatestAlertEvent(ctx context.Context, ruleID int64) (*OpsAlertEvent, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.latestEvent == nil || r.latestEvent.RuleID != ruleID {
		return nil, nil
REDACTED
	clone := *r.latestEvent
	return &clone, nil
REDACTED

func (r *fakeOpsRepository) CreateAlertEvent(ctx context.Context, event *OpsAlertEvent) error {
	if event == nil {
		return nil
REDACTED
	r.mu.Lock()
	defer r.mu.Unlock()

	r.nextEventID++
	event.ID = r.nextEventID

	clone := *event
	r.latestEvent = &clone
	if clone.Status == OpsAlertStatusFiring {
		r.activeEvent = &clone
REDACTED

	r.eventOnce.Do(func() { close(r.eventCreatedCh) REDACTED)
	return nil
REDACTED

func (r *fakeOpsRepository) UpdateAlertEventStatus(ctx context.Context, eventID int64, status string, resolvedAt *time.Time) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.activeEvent != nil && r.activeEvent.ID == eventID {
		r.activeEvent.Status = status
		r.activeEvent.ResolvedAt = resolvedAt
REDACTED
	if r.latestEvent != nil && r.latestEvent.ID == eventID {
		r.latestEvent.Status = status
		r.latestEvent.ResolvedAt = resolvedAt
REDACTED
	return nil
REDACTED

func (r *fakeOpsRepository) UpdateAlertEventNotifications(ctx context.Context, eventID int64, emailSent, webhookSent bool) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.activeEvent != nil && r.activeEvent.ID == eventID {
		r.activeEvent.EmailSent = emailSent
		r.activeEvent.WebhookSent = webhookSent
REDACTED
	if r.latestEvent != nil && r.latestEvent.ID == eventID {
		r.latestEvent.EmailSent = emailSent
		r.latestEvent.WebhookSent = webhookSent
REDACTED
	return nil
REDACTED

func (r *fakeOpsRepository) CountActiveAlerts(ctx context.Context) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.activeEvent == nil {
		return 0, nil
REDACTED
	return 1, nil
REDACTED

func (r *fakeOpsRepository) GetOverviewStats(ctx context.Context, startTime, endTime time.Time) (*OverviewStats, error) {
	return &OverviewStats{REDACTED, nil
REDACTED

func (r *fakeOpsRepository) GetCachedLatestSystemMetric(ctx context.Context) (*OpsMetrics, error) {
	return nil, nil
REDACTED

func (r *fakeOpsRepository) SetCachedLatestSystemMetric(ctx context.Context, metric *OpsMetrics) error {
	return nil
REDACTED

func (r *fakeOpsRepository) GetCachedDashboardOverview(ctx context.Context, timeRange string) (*DashboardOverviewData, error) {
	return nil, nil
REDACTED

func (r *fakeOpsRepository) SetCachedDashboardOverview(ctx context.Context, timeRange string, data *DashboardOverviewData, ttl time.Duration) error {
	return nil
REDACTED

func (r *fakeOpsRepository) PingRedis(ctx context.Context) error {
	return nil
REDACTED
