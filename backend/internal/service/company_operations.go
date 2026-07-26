package service

import (
	"context"
	"log/slog"
	"sort"
	"sync"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/accountid"
)

const (
	defaultCompanyReconcileInterval = 5 * time.Minute
	defaultReviewQueueAlertAge      = 24 * time.Hour
	defaultOutboxLagAlertAge        = 5 * time.Minute
)

type CompanyReconciler interface {
	Reconcile(ctx context.Context) (map[string]int64, error)
}

type CompanyOutboxHealthProvider interface {
	Health(ctx context.Context) NotificationOutboxHealth
}

type CompanyAuthorizationHealthProvider interface {
	AuthCacheInvalidationSubscriberHealth() AuthCacheInvalidationSubscriberHealth
}

type CompanyOperationsMetrics struct {
	IDCollisionRetries             uint64 `json:"id_collision_retries"`
	ReviewQueueAgeSeconds          int64  `json:"review_queue_age_seconds"`
	OutboxLagSeconds               int64  `json:"outbox_lag_seconds"`
	OutboxDeliveryFailures         uint64 `json:"outbox_delivery_failures"`
	OutboxFailedMessages           int64  `json:"outbox_failed_messages"`
	AuthorizationDatabaseFallbacks uint64 `json:"authorization_database_fallbacks"`
	DeniedIAMFinancialOperations   uint64 `json:"denied_iam_financial_operations"`
	PayerResolutionFailures        uint64 `json:"payer_resolution_failures"`
}

type CompanyOperationsAlert struct {
	Key       string `json:"key"`
	Value     int64  `json:"value"`
	Threshold int64  `json:"threshold"`
}

type CompanyOperationsSnapshot struct {
	CollectedAt    time.Time                `json:"collected_at"`
	Reconciliation map[string]int64         `json:"reconciliation"`
	Metrics        CompanyOperationsMetrics `json:"metrics"`
	Alerts         []CompanyOperationsAlert `json:"alerts"`
}

type CompanyOperationsMonitor struct {
	reconciler CompanyReconciler
	outbox     CompanyOutboxHealthProvider
	auth       CompanyAuthorizationHealthProvider
	interval   time.Duration
	reviewAge  time.Duration
	outboxAge  time.Duration

	ctx    context.Context
	cancel context.CancelFunc
	start  sync.Once
	stop   sync.Once
	wg     sync.WaitGroup
	mu     sync.RWMutex
	latest CompanyOperationsSnapshot
}

func NewCompanyOperationsMonitor(
	reconciler CompanyReconciler,
	outbox CompanyOutboxHealthProvider,
	auth CompanyAuthorizationHealthProvider,
	cfg *config.Config,
) *CompanyOperationsMonitor {
	interval := defaultCompanyReconcileInterval
	reviewAge := defaultReviewQueueAlertAge
	outboxAge := defaultOutboxLagAlertAge
	if cfg != nil {
		if cfg.Company.ReconcileIntervalSeconds > 0 {
			interval = time.Duration(cfg.Company.ReconcileIntervalSeconds) * time.Second
		}
		if cfg.Company.ReviewQueueAlertSeconds > 0 {
			reviewAge = time.Duration(cfg.Company.ReviewQueueAlertSeconds) * time.Second
		}
		if cfg.Company.OutboxLagAlertSeconds > 0 {
			outboxAge = time.Duration(cfg.Company.OutboxLagAlertSeconds) * time.Second
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	return &CompanyOperationsMonitor{
		reconciler: reconciler,
		outbox:     outbox,
		auth:       auth,
		interval:   interval,
		reviewAge:  reviewAge,
		outboxAge:  outboxAge,
		ctx:        ctx,
		cancel:     cancel,
		latest:     CompanyOperationsSnapshot{Reconciliation: map[string]int64{}, Alerts: []CompanyOperationsAlert{}},
	}
}

func (m *CompanyOperationsMonitor) Start() {
	if m == nil || m.reconciler == nil {
		return
	}
	m.start.Do(func() {
		m.wg.Add(1)
		go m.run()
	})
}

func (m *CompanyOperationsMonitor) Stop() {
	if m == nil {
		return
	}
	m.stop.Do(func() {
		m.cancel()
		m.wg.Wait()
	})
}

func (m *CompanyOperationsMonitor) run() {
	defer m.wg.Done()
	ticker := time.NewTicker(m.interval)
	defer ticker.Stop()
	for {
		if _, err := m.Collect(m.ctx); err != nil && m.ctx.Err() == nil {
			slog.Error("company account reconciliation failed", "error", err)
		}
		select {
		case <-m.ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func (m *CompanyOperationsMonitor) Collect(ctx context.Context) (CompanyOperationsSnapshot, error) {
	if m == nil || m.reconciler == nil {
		return CompanyOperationsSnapshot{}, nil
	}
	reconciliation, err := m.reconciler.Reconcile(ctx)
	if err != nil {
		return CompanyOperationsSnapshot{}, err
	}
	runtimeMetrics := CurrentOrganizationRuntimeMetrics()
	idMetrics := accountid.CurrentMetrics()
	metrics := CompanyOperationsMetrics{
		IDCollisionRetries:           idMetrics.CollisionRetries,
		DeniedIAMFinancialOperations: runtimeMetrics.DeniedIAMFinancialOps,
		PayerResolutionFailures:      runtimeMetrics.PayerResolutionFailures,
		ReviewQueueAgeSeconds:        reconciliation["oldest_review_queue_age_seconds"],
	}
	if m.outbox != nil {
		health := m.outbox.Health(ctx)
		metrics.OutboxLagSeconds = health.OldestLag
		metrics.OutboxDeliveryFailures = health.Failures
		metrics.OutboxFailedMessages = health.Failed
	}
	if m.auth != nil {
		metrics.AuthorizationDatabaseFallbacks = m.auth.AuthCacheInvalidationSubscriberHealth().DatabaseFallbacks
	}
	snapshot := CompanyOperationsSnapshot{
		CollectedAt:    time.Now().UTC(),
		Reconciliation: reconciliation,
		Metrics:        metrics,
		Alerts:         m.buildAlerts(reconciliation, metrics),
	}
	m.mu.Lock()
	m.latest = snapshot
	m.mu.Unlock()
	for _, alert := range snapshot.Alerts {
		slog.Warn("company account operational alert", "metric", alert.Key, "value", alert.Value, "threshold", alert.Threshold)
	}
	return snapshot, nil
}

func (m *CompanyOperationsMonitor) Latest() CompanyOperationsSnapshot {
	if m == nil {
		return CompanyOperationsSnapshot{}
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.latest
}

func (m *CompanyOperationsMonitor) buildAlerts(reconciliation map[string]int64, metrics CompanyOperationsMetrics) []CompanyOperationsAlert {
	alerts := make([]CompanyOperationsAlert, 0)
	for key, value := range reconciliation {
		if key != "oldest_review_queue_age_seconds" && value > 0 {
			alerts = append(alerts, CompanyOperationsAlert{Key: "reconciliation." + key, Value: value, Threshold: 0})
		}
	}
	add := func(key string, value, threshold int64) {
		if value > threshold {
			alerts = append(alerts, CompanyOperationsAlert{Key: key, Value: value, Threshold: threshold})
		}
	}
	add("id_collision_retries", int64(metrics.IDCollisionRetries), 0)
	add("review_queue_age_seconds", metrics.ReviewQueueAgeSeconds, int64(m.reviewAge.Seconds()))
	add("outbox_lag_seconds", metrics.OutboxLagSeconds, int64(m.outboxAge.Seconds()))
	add("outbox_delivery_failures", int64(metrics.OutboxDeliveryFailures), 0)
	add("outbox_failed_messages", metrics.OutboxFailedMessages, 0)
	add("authorization_database_fallbacks", int64(metrics.AuthorizationDatabaseFallbacks), 0)
	add("denied_iam_financial_operations", int64(metrics.DeniedIAMFinancialOperations), 0)
	add("payer_resolution_failures", int64(metrics.PayerResolutionFailures), 0)
	sort.Slice(alerts, func(i, j int) bool { return alerts[i].Key < alerts[j].Key })
	return alerts
}
