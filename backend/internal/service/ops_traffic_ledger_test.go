package service

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/traffic"
	"github.com/stretchr/testify/require"
)

type trafficLedgerRepoStub struct {
	OpsRepository
	mu       sync.Mutex
	batches  [][]*OpsTrafficEvent
	failures int
}

func (r *trafficLedgerRepoStub) BatchInsertTrafficEvents(_ context.Context, events []*OpsTrafficEvent) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.batches = append(r.batches, events)
	if r.failures > 0 {
		r.failures--
		return errors.New("unavailable")
	}
	return nil
}
func testTrafficEvent() *OpsTrafficEvent {
	n := int64(5)
	return &OpsTrafficEvent{RequestID: "same-client-request", GroupID: &n, Traffic: &traffic.Usage{Downstream: &traffic.Link{Response: &traffic.Measurement{ActualBytes: &n}}}}
}

func TestOpsTrafficLedgerBatchRetryCloneAndShutdown(t *testing.T) {
	repo := &trafficLedgerRepoStub{failures: 1}
	svc := &OpsService{opsRepo: repo}
	input := testTrafficEvent()
	for i := 0; i < 70; i++ {
		require.NoError(t, svc.RecordTrafficEvent(context.Background(), input))
	}
	*input.GroupID = 99
	*input.Traffic.Downstream.Response.ActualBytes = 99
	svc.StopRuntimeSettingsRefresh()
	svc.StopTrafficLedger()
	health := svc.GetTrafficLedgerHealth()
	require.Equal(t, &OpsTrafficLedgerHealth{Scope: "instance_lifetime", Flushed: 70}, health)
	repo.mu.Lock()
	defer repo.mu.Unlock()
	require.GreaterOrEqual(t, len(repo.batches), 3)
	require.Equal(t, repo.batches[0][0].EventID, repo.batches[1][0].EventID, "retry must reuse stable server ID")
	ids := map[string]bool{}
	// First batch failed; subsequent attempts represent successful commits.
	for _, batch := range repo.batches[1:] {
		require.LessOrEqual(t, len(batch), opsTrafficBatchSize)
		for _, row := range batch {
			require.False(t, ids[row.EventID], "same client request ID must not collapse separate requests")
			ids[row.EventID] = true
			require.Equal(t, int64(5), *row.GroupID)
			require.Equal(t, int64(5), *row.Traffic.Downstream.Response.ActualBytes)
			require.False(t, row.CreatedAt.IsZero())
		}
	}
	require.Len(t, ids, 70)
}

func TestOpsTrafficLedgerFailureVisibleAndDisabled(t *testing.T) {
	repo := &trafficLedgerRepoStub{failures: 10}
	svc := &OpsService{opsRepo: repo}
	require.NoError(t, svc.RecordTrafficEvent(context.Background(), testTrafficEvent()))
	svc.StopTrafficLedger()
	require.Equal(t, int64(1), svc.GetTrafficLedgerHealth().Failed)
	require.Zero(t, svc.GetTrafficLedgerHealth().Pending)
	require.Error(t, svc.RecordTrafficEvent(context.Background(), testTrafficEvent()))
	require.Equal(t, int64(2), svc.GetTrafficLedgerHealth().Failed)
	disabled := &OpsService{opsRepo: repo, cfg: &config.Config{}}
	require.NoError(t, disabled.RecordTrafficEvent(context.Background(), testTrafficEvent()))
	require.Nil(t, disabled.GetTrafficLedgerHealth())
}

func TestOpsTrafficLedgerFullQueueRejectsImmediately(t *testing.T) {
	// No worker: saturation must return without waiting for capacity.
	ledger := &opsTrafficLedger{queue: make(chan *OpsTrafficEvent, 1)}
	require.NoError(t, ledger.enqueue(context.Background(), testTrafficEvent()))
	started := time.Now()
	require.EqualError(t, ledger.enqueue(context.Background(), testTrafficEvent()), "ops traffic ledger queue full")
	require.Less(t, time.Since(started), 200*time.Millisecond)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	require.ErrorIs(t, ledger.enqueue(ctx, testTrafficEvent()), context.Canceled)
	require.Equal(t, int64(1), ledger.pending.Load())
	require.Equal(t, int64(2), ledger.failed.Load())
	require.Len(t, ledger.queue, 1)
	// Cancellation also wins when there would be space to enqueue.
	<-ledger.queue
	ledger.pending.Add(-1)
	require.ErrorIs(t, ledger.enqueue(ctx, testTrafficEvent()), context.Canceled)
	require.Zero(t, ledger.pending.Load())
	require.Equal(t, int64(3), ledger.failed.Load())
	require.Empty(t, ledger.queue)
}

func TestOpsTrafficLedgerConcurrentEnqueueAndStop(t *testing.T) {
	repo := &trafficLedgerRepoStub{}
	ledger := newOpsTrafficLedger(repo, 8)
	const producers, eventsPerProducer = 16, 64
	start := make(chan struct{})
	var wg sync.WaitGroup
	for i := 0; i < producers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			for j := 0; j < eventsPerProducer; j++ {
				_ = ledger.enqueue(context.Background(), testTrafficEvent())
				if pending := ledger.pending.Load(); pending < 0 {
					t.Errorf("negative pending count: %d", pending)
				}
			}
		}()
	}
	stopped := make(chan struct{})
	go func() { <-start; ledger.stop(); close(stopped) }()
	close(start)
	wg.Wait()
	<-stopped
	ledger.stop()
	require.Zero(t, ledger.pending.Load())
	require.GreaterOrEqual(t, ledger.failed.Load(), int64(0))
	require.Equal(t, int64(producers*eventsPerProducer), ledger.failed.Load()+ledger.flushed.Load())
}

func TestOpsTrafficLedgerDimensionsBounded(t *testing.T) {
	input := testTrafficEvent()
	input.RequestID = strings.Repeat("界", 100)
	input.Platform = " OpenAI "
	event := cloneOpsTrafficEvent(input)
	require.LessOrEqual(t, len(event.RequestID), 128)
	require.Equal(t, "openai", event.Platform)
}

func TestOpsTrafficLedgerRetentionUsesUUIDAndRawRetention(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()
	cutoff := time.Now().UTC()
	mock.ExpectExec(`(?s)SELECT event_id FROM ops_traffic_events.*WHERE created_at < \$1.*DELETE FROM ops_traffic_events.*WHERE event_id IN`).WithArgs(cutoff, 10).WillReturnResult(sqlmock.NewResult(0, 2))
	mock.ExpectExec(`(?s)SELECT event_id FROM ops_traffic_events`).WithArgs(cutoff, 10).WillReturnResult(sqlmock.NewResult(0, 0))
	count, err := deleteOldRowsByID(context.Background(), db, "ops_traffic_events", "created_at", cutoff, 10, false)
	require.NoError(t, err)
	require.Equal(t, int64(2), count)
	mock.ExpectQuery(`SELECT COUNT\(\*\) FROM ops_traffic_events`).WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(4))
	mock.ExpectExec(`TRUNCATE TABLE ops_traffic_events`).WillReturnResult(sqlmock.NewResult(0, 0))
	count, err = opsCleanupRunOne(context.Background(), db, true, time.Time{}, "ops_traffic_events", "created_at", false, 10)
	require.NoError(t, err)
	require.Equal(t, int64(4), count)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestOpsTrafficLedgerCleanupWiringUsesErrorLogRetention(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()
	svc := &OpsCleanupService{db: db, cfg: &config.Config{}, effective: opsCleanupEffectiveConfig{
		OpsCleanupConfig:       config.OpsCleanupConfig{ErrorLogRetentionDays: 7, MinuteMetricsRetentionDays: -1, HourlyMetricsRetentionDays: -1},
		SystemLogRetentionDays: -1,
	}}
	for _, table := range []string{"ops_error_logs", "ops_traffic_events", "ops_ingress_reject_aggregates", "ops_alert_events"} {
		mock.ExpectExec("DELETE FROM "+table).WithArgs(sqlmock.AnyArg(), opsCleanupBatchSize).WillReturnResult(sqlmock.NewResult(0, 0))
	}
	_, err = svc.runCleanupOnce(context.Background())
	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}
