//go:build unit

package service

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/accountid"
	"github.com/stretchr/testify/require"
)

type companyReconcilerStub struct {
	checks map[string]int64
	calls  atomic.Int64
}

func (s *companyReconcilerStub) Reconcile(context.Context) (map[string]int64, error) {
	s.calls.Add(1)
	result := make(map[string]int64, len(s.checks))
	for key, value := range s.checks {
		result[key] = value
	}
	return result, nil
}

type companyOutboxHealthStub struct{ health NotificationOutboxHealth }

func (s companyOutboxHealthStub) Health(context.Context) NotificationOutboxHealth { return s.health }

type companyAuthorizationHealthStub struct {
	health AuthCacheInvalidationSubscriberHealth
}

func (s companyAuthorizationHealthStub) AuthCacheInvalidationSubscriberHealth() AuthCacheInvalidationSubscriberHealth {
	return s.health
}

func TestCompanyOperationsMonitorCollectsMetricsAndAlerts(t *testing.T) {
	accountid.RecordCollisionRetry()
	_ = GuardIAMFinancialOperation(&User{IdentityType: IdentityTypeIAM})
	reconciler := &companyReconcilerStub{checks: map[string]int64{
		"pending_reservation_mismatch":    2,
		"owner_cardinality_violation":     0,
		"oldest_review_queue_age_seconds": 20,
	}}
	cfg := &config.Config{}
	cfg.Company.ReconcileIntervalSeconds = 300
	cfg.Company.ReviewQueueAlertSeconds = 10
	cfg.Company.OutboxLagAlertSeconds = 5
	monitor := NewCompanyOperationsMonitor(
		reconciler,
		companyOutboxHealthStub{health: NotificationOutboxHealth{Failures: 3, Failed: 1, OldestLag: 8}},
		companyAuthorizationHealthStub{health: AuthCacheInvalidationSubscriberHealth{DatabaseFallbacks: 4}},
		cfg,
	)

	snapshot, err := monitor.Collect(context.Background())
	require.NoError(t, err)
	require.EqualValues(t, 2, snapshot.Reconciliation["pending_reservation_mismatch"])
	require.GreaterOrEqual(t, snapshot.Metrics.IDCollisionRetries, uint64(1))
	require.EqualValues(t, 20, snapshot.Metrics.ReviewQueueAgeSeconds)
	require.EqualValues(t, 8, snapshot.Metrics.OutboxLagSeconds)
	require.EqualValues(t, 3, snapshot.Metrics.OutboxDeliveryFailures)
	require.EqualValues(t, 1, snapshot.Metrics.OutboxFailedMessages)
	require.EqualValues(t, 4, snapshot.Metrics.AuthorizationDatabaseFallbacks)
	require.GreaterOrEqual(t, snapshot.Metrics.DeniedIAMFinancialOperations, uint64(1))

	alertKeys := make([]string, 0, len(snapshot.Alerts))
	for _, alert := range snapshot.Alerts {
		alertKeys = append(alertKeys, alert.Key)
	}
	require.Contains(t, alertKeys, "reconciliation.pending_reservation_mismatch")
	require.Contains(t, alertKeys, "id_collision_retries")
	require.Contains(t, alertKeys, "review_queue_age_seconds")
	require.Contains(t, alertKeys, "outbox_lag_seconds")
	require.Contains(t, alertKeys, "outbox_delivery_failures")
	require.Contains(t, alertKeys, "outbox_failed_messages")
	require.Contains(t, alertKeys, "authorization_database_fallbacks")
	require.Contains(t, alertKeys, "denied_iam_financial_operations")
}

func TestCompanyOperationsMonitorRunsOnSchedule(t *testing.T) {
	reconciler := &companyReconcilerStub{checks: map[string]int64{}}
	monitor := NewCompanyOperationsMonitor(reconciler, nil, nil, nil)
	monitor.interval = 10 * time.Millisecond
	monitor.Start()
	t.Cleanup(monitor.Stop)
	require.Eventually(t, func() bool { return reconciler.calls.Load() >= 2 }, time.Second, 10*time.Millisecond)
	require.False(t, monitor.Latest().CollectedAt.IsZero())
}
