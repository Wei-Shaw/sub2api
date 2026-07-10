package service

import (
	"context"
	"fmt"
	"sort"
	"time"
)

const (
	ReliabilitySeverityWarning  = "warning"
	ReliabilitySeverityCritical = "critical"

	ReliabilityActionAutoRelease    = "auto_release"
	ReliabilityActionReviewRequired = "review_required"

	ReliabilityCodeSuccessUnsettled             = "video_success_unsettled"
	ReliabilityCodeTerminalActiveReservation    = "video_terminal_active_reservation"
	ReliabilityCodeExpiredQueuedUndispatched    = "video_expired_queued_undispatched"
	ReliabilityCodeExpiredDispatchedReservation = "video_expired_dispatched_reservation"
	ReliabilityCodeLedgerBalanceDrift           = "billing_ledger_balance_drift"
	ReliabilityCodeDeadOutbox                   = "domain_outbox_dead"
	ReliabilityCodeSuccessWithoutDeliverable    = "video_success_without_deliverable_asset"
)

// ReliabilityFinding is intentionally the complete public projection of a
// dry-run finding. It cannot carry provider payloads, URLs or credentials.
type ReliabilityFinding struct {
	Severity          string `json:"severity"`
	Code              string `json:"code"`
	TaskID            int64  `json:"task_id"`
	RecommendedAction string `json:"recommended_action"`
}

// ReliabilityReconciliationRow is a read-only, already-aggregated view. It has
// no URL, payload or credential field, so sensitive provider evidence cannot
// cross the reconciliation boundary.
type ReliabilityReconciliationRow struct {
	TaskID               int64
	Status               string
	DispatchState        string
	SettlementStatus     string
	ReservationStatus    string
	ReservationExpiresAt time.Time
	LedgerBalanceAfter   *Money
	ProjectedBalance     *Money
	DeadOutboxCount      int64
	LocalAssetAvailable  bool
	RemoteAssetAvailable bool
}

type ReliabilityReconciliationSnapshot struct {
	Rows []ReliabilityReconciliationRow
}

// ReliabilityReconciliationSource deliberately has no mutation methods and no
// provider dependency, making DryRun incapable of writes or network calls.
type ReliabilityReconciliationSource interface {
	ReliabilitySnapshot(ctx context.Context, now time.Time, limit int) (ReliabilityReconciliationSnapshot, error)
}

type ReliabilityReconciler struct {
	source ReliabilityReconciliationSource
}

func NewReliabilityReconciler(source ReliabilityReconciliationSource) *ReliabilityReconciler {
	return &ReliabilityReconciler{source: source}
}

func (r *ReliabilityReconciler) DryRun(ctx context.Context, now time.Time, limit int) ([]ReliabilityFinding, error) {
	if r == nil || r.source == nil {
		return nil, fmt.Errorf("reliability reconciliation source is required")
	}
	if now.IsZero() {
		now = time.Now().UTC()
	} else {
		now = now.UTC()
	}
	if limit <= 0 {
		limit = 100
	}
	snapshot, err := r.source.ReliabilitySnapshot(ctx, now, limit)
	if err != nil {
		return nil, fmt.Errorf("read reliability reconciliation snapshot: %w", err)
	}

	findings := make([]ReliabilityFinding, 0, len(snapshot.Rows))
	for _, row := range snapshot.Rows {
		if row.TaskID <= 0 {
			continue
		}
		if row.Status == VideoStatusSucceeded && row.SettlementStatus != VideoSettlementStatusSettled && row.SettlementStatus != VideoSettlementStatusNotNeeded {
			findings = append(findings, reliabilityFinding(ReliabilitySeverityCritical, ReliabilityCodeSuccessUnsettled, row.TaskID, ReliabilityActionReviewRequired))
		}
		if IsTerminalVideoStatus(row.Status) && row.ReservationStatus == BillingReservationStatusActive {
			findings = append(findings, reliabilityFinding(ReliabilitySeverityCritical, ReliabilityCodeTerminalActiveReservation, row.TaskID, ReliabilityActionReviewRequired))
		}
		if row.ReservationStatus == BillingReservationStatusActive && !row.ReservationExpiresAt.IsZero() && !row.ReservationExpiresAt.After(now) {
			if row.Status == VideoStatusQueued && row.DispatchState == VideoDispatchStatePending {
				findings = append(findings, reliabilityFinding(ReliabilitySeverityWarning, ReliabilityCodeExpiredQueuedUndispatched, row.TaskID, ReliabilityActionAutoRelease))
			} else {
				findings = append(findings, reliabilityFinding(ReliabilitySeverityCritical, ReliabilityCodeExpiredDispatchedReservation, row.TaskID, ReliabilityActionReviewRequired))
			}
		}
		if row.LedgerBalanceAfter != nil && row.ProjectedBalance != nil {
			comparison, compareErr := row.LedgerBalanceAfter.Compare(*row.ProjectedBalance)
			if compareErr != nil {
				return nil, fmt.Errorf("compare ledger balance for task %d: %w", row.TaskID, compareErr)
			}
			if comparison != 0 {
				findings = append(findings, reliabilityFinding(ReliabilitySeverityCritical, ReliabilityCodeLedgerBalanceDrift, row.TaskID, ReliabilityActionReviewRequired))
			}
		}
		if row.DeadOutboxCount > 0 {
			findings = append(findings, reliabilityFinding(ReliabilitySeverityCritical, ReliabilityCodeDeadOutbox, row.TaskID, ReliabilityActionReviewRequired))
		}
		if row.Status == VideoStatusSucceeded && !row.LocalAssetAvailable && !row.RemoteAssetAvailable {
			findings = append(findings, reliabilityFinding(ReliabilitySeverityWarning, ReliabilityCodeSuccessWithoutDeliverable, row.TaskID, ReliabilityActionReviewRequired))
		}
	}

	// Stable output makes review packages and repeated dry-runs diffable.
	sort.SliceStable(findings, func(i, j int) bool {
		if findings[i].TaskID != findings[j].TaskID {
			return findings[i].TaskID < findings[j].TaskID
		}
		return findings[i].Code < findings[j].Code
	})
	return findings, nil
}

func reliabilityFinding(severity, code string, taskID int64, action string) ReliabilityFinding {
	return ReliabilityFinding{Severity: severity, Code: code, TaskID: taskID, RecommendedAction: action}
}
