package service

import (
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"testing"
	"time"
)

type reliabilityReconciliationSourceStub struct {
	snapshot ReliabilityReconciliationSnapshot
	err      error
	writes   int
}

func (s *reliabilityReconciliationSourceStub) ReliabilitySnapshot(context.Context, time.Time, int) (ReliabilityReconciliationSnapshot, error) {
	return s.snapshot, s.err
}

func TestReliabilityReconcilerDryRunReportsEveryInvariantWithoutWritesOrSecrets(t *testing.T) {
	now := time.Date(2026, 7, 10, 8, 0, 0, 0, time.UTC)
	ledgerBalance := MustUSD("8.50000000")
	projectedBalance := MustUSD("8.49999999")
	source := &reliabilityReconciliationSourceStub{snapshot: ReliabilityReconciliationSnapshot{Rows: []ReliabilityReconciliationRow{
		{TaskID: 101, Status: VideoStatusSucceeded, SettlementStatus: VideoSettlementStatusPending, RemoteAssetAvailable: true},
		{TaskID: 102, Status: VideoStatusFailed, ReservationStatus: BillingReservationStatusActive},
		{TaskID: 103, Status: VideoStatusQueued, DispatchState: VideoDispatchStatePending, ReservationStatus: BillingReservationStatusActive, ReservationExpiresAt: now.Add(-time.Second)},
		{TaskID: 104, Status: VideoStatusSubmitted, DispatchState: VideoDispatchStateAccepted, ReservationStatus: BillingReservationStatusActive, ReservationExpiresAt: now.Add(-time.Second)},
		{TaskID: 105, Status: VideoStatusRunning, DispatchState: VideoDispatchStateUnknown, ReservationStatus: BillingReservationStatusActive, ReservationExpiresAt: now.Add(-time.Second)},
		{TaskID: 106, LedgerBalanceAfter: &ledgerBalance, ProjectedBalance: &projectedBalance},
		{TaskID: 107, DeadOutboxCount: 1},
		{TaskID: 108, Status: VideoStatusSucceeded, SettlementStatus: VideoSettlementStatusSettled, LocalAssetAvailable: false, RemoteAssetAvailable: false},
		// The source projection itself has no URL, payload or credential field.
		{TaskID: 109, Status: VideoStatusSucceeded, SettlementStatus: VideoSettlementStatusSettled, LocalAssetAvailable: true},
	}}}

	findings, err := NewReliabilityReconciler(source).DryRun(context.Background(), now, 100)
	if err != nil {
		t.Fatalf("DryRun() error = %v", err)
	}
	if source.writes != 0 {
		t.Fatalf("DryRun() performed %d writes", source.writes)
	}

	want := []ReliabilityFinding{
		{Severity: ReliabilitySeverityCritical, Code: ReliabilityCodeSuccessUnsettled, TaskID: 101, RecommendedAction: ReliabilityActionReviewRequired},
		{Severity: ReliabilitySeverityCritical, Code: ReliabilityCodeTerminalActiveReservation, TaskID: 102, RecommendedAction: ReliabilityActionReviewRequired},
		{Severity: ReliabilitySeverityWarning, Code: ReliabilityCodeExpiredQueuedUndispatched, TaskID: 103, RecommendedAction: ReliabilityActionAutoRelease},
		{Severity: ReliabilitySeverityCritical, Code: ReliabilityCodeExpiredDispatchedReservation, TaskID: 104, RecommendedAction: ReliabilityActionReviewRequired},
		{Severity: ReliabilitySeverityCritical, Code: ReliabilityCodeExpiredDispatchedReservation, TaskID: 105, RecommendedAction: ReliabilityActionReviewRequired},
		{Severity: ReliabilitySeverityCritical, Code: ReliabilityCodeLedgerBalanceDrift, TaskID: 106, RecommendedAction: ReliabilityActionReviewRequired},
		{Severity: ReliabilitySeverityCritical, Code: ReliabilityCodeDeadOutbox, TaskID: 107, RecommendedAction: ReliabilityActionReviewRequired},
		{Severity: ReliabilitySeverityWarning, Code: ReliabilityCodeSuccessWithoutDeliverable, TaskID: 108, RecommendedAction: ReliabilityActionReviewRequired},
	}
	if !reflect.DeepEqual(findings, want) {
		t.Fatalf("DryRun() findings = %#v, want %#v", findings, want)
	}

	encoded, err := json.Marshal(findings)
	if err != nil {
		t.Fatalf("Marshal(findings): %v", err)
	}
	for _, forbidden := range []string{"must-not-leak", "provider.invalid", "token=", "http://", "https://", "credential"} {
		if strings.Contains(string(encoded), forbidden) {
			t.Fatalf("findings leaked %q in %s", forbidden, encoded)
		}
	}
	var projection []map[string]any
	if err := json.Unmarshal(encoded, &projection); err != nil {
		t.Fatalf("Unmarshal(findings): %v", err)
	}
	for _, finding := range projection {
		if len(finding) != 4 {
			t.Fatalf("finding projection has fields %v, want exactly four", finding)
		}
	}
}

func TestReliabilityReconcilerNeverSuggestsAutoReleaseAfterDispatch(t *testing.T) {
	now := time.Date(2026, 7, 10, 8, 0, 0, 0, time.UTC)
	rows := []ReliabilityReconciliationRow{
		{TaskID: 1, Status: VideoStatusQueued, DispatchState: VideoDispatchStateDispatching, ReservationStatus: BillingReservationStatusActive, ReservationExpiresAt: now.Add(-time.Hour)},
		{TaskID: 2, Status: VideoStatusSubmitted, DispatchState: VideoDispatchStateUnknown, ReservationStatus: BillingReservationStatusActive, ReservationExpiresAt: now.Add(-time.Hour)},
		{TaskID: 3, Status: VideoStatusRunning, DispatchState: VideoDispatchStateAccepted, ReservationStatus: BillingReservationStatusActive, ReservationExpiresAt: now.Add(-time.Hour)},
	}
	findings, err := NewReliabilityReconciler(&reliabilityReconciliationSourceStub{snapshot: ReliabilityReconciliationSnapshot{Rows: rows}}).DryRun(context.Background(), now, 10)
	if err != nil {
		t.Fatalf("DryRun() error = %v", err)
	}
	for _, finding := range findings {
		if finding.RecommendedAction == ReliabilityActionAutoRelease {
			t.Fatalf("dispatched task %d received unsafe auto-release recommendation", finding.TaskID)
		}
	}
}

type recordingReliabilityMetricRegistry struct {
	definitions []ReliabilityMetricDefinition
}

func (r *recordingReliabilityMetricRegistry) RegisterReliabilityMetric(definition ReliabilityMetricDefinition) {
	r.definitions = append(r.definitions, definition)
}

func TestRegisterReliabilityMetricsRegistersSpecMetrics(t *testing.T) {
	registry := &recordingReliabilityMetricRegistry{}
	metrics := RegisterReliabilityMetrics(registry)
	if metrics == nil {
		t.Fatal("RegisterReliabilityMetrics() returned nil")
	}

	want := []ReliabilityMetricDefinition{
		{Name: "video_finalization_total", Kind: ReliabilityMetricCounter, Labels: []string{"status"}},
		{Name: "video_finalization_conflict_total", Kind: ReliabilityMetricCounter},
		{Name: "billing_reservation_active_total", Kind: ReliabilityMetricGauge},
		{Name: "billing_reservation_overrun_total", Kind: ReliabilityMetricCounter},
		{Name: "billing_settlement_retry_total", Kind: ReliabilityMetricCounter},
		{Name: "domain_outbox_pending_total", Kind: ReliabilityMetricGauge, Labels: []string{"event_type"}},
		{Name: "domain_outbox_dead_total", Kind: ReliabilityMetricGauge, Labels: []string{"event_type"}},
		{Name: "domain_outbox_oldest_age_seconds", Kind: ReliabilityMetricGauge},
		{Name: "video_dispatch_unknown_total", Kind: ReliabilityMetricCounter, Labels: []string{"provider"}},
	}
	if !reflect.DeepEqual(registry.definitions, want) {
		t.Fatalf("registered metrics = %#v, want %#v", registry.definitions, want)
	}

	// The default registry is deliberately a no-op and must always be safe.
	RegisterReliabilityMetrics(nil).Add("video_finalization_total", 1, map[string]string{"status": VideoStatusSucceeded})
	RegisterReliabilityMetrics(nil).Set("domain_outbox_oldest_age_seconds", 3, nil)
}
