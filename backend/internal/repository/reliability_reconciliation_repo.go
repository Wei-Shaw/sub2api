package repository

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
)

// reliabilityReconciliationRepository is intentionally a read-only
// projection repository. It owns no mutation method and its sole query is a
// SELECT over already-persisted task, reservation, ledger, outbox, and asset
// state, so operator dry-runs cannot charge, release, or dispatch anything.
type reliabilityReconciliationRepository struct {
	db *sql.DB
}

var _ service.ReliabilityReconciliationSource = (*reliabilityReconciliationRepository)(nil)

func NewReliabilityReconciliationRepository(db *sql.DB) service.ReliabilityReconciliationSource {
	return &reliabilityReconciliationRepository{db: db}
}

func (r *reliabilityReconciliationRepository) ReliabilitySnapshot(ctx context.Context, now time.Time, limit int) (service.ReliabilityReconciliationSnapshot, error) {
	if r == nil || r.db == nil {
		return service.ReliabilityReconciliationSnapshot{}, fmt.Errorf("reliability reconciliation database is required")
	}
	if now.IsZero() {
		now = time.Now().UTC()
	} else {
		now = now.UTC()
	}
	if limit <= 0 {
		limit = 100
	}

	rows, err := r.db.QueryContext(ctx, `
		SELECT
			vt.id,
			vt.status,
			vt.dispatch_state,
			vt.settlement_status,
			COALESCE(br.status, ''),
			br.expires_at,
			latest_ledger.balance_after::text,
			CASE
				WHEN latest_ledger.id IS NULL THEN NULL
				WHEN EXISTS (
					SELECT 1
					FROM billing_transactions newer
					WHERE newer.user_id = latest_ledger.user_id
					  AND (newer.created_at, newer.id) > (latest_ledger.created_at, latest_ledger.id)
				) THEN NULL
				ELSE u.balance::text
			END,
			COALESCE(dead_outbox.count, 0),
			(vt.local_asset_path IS NOT NULL AND vt.local_asset_path <> ''),
			COALESCE(vt.result_url, ''),
			vt.completed_at
		FROM video_tasks vt
		LEFT JOIN billing_reservations br ON br.id = vt.reservation_id
		LEFT JOIN users u ON u.id = vt.created_by AND u.deleted_at IS NULL
		LEFT JOIN LATERAL (
			SELECT id, user_id, created_at, balance_after
			FROM billing_transactions
			WHERE source_type = 'video_task' AND source_id = vt.id
			ORDER BY created_at DESC, id DESC
			LIMIT 1
		) AS latest_ledger ON TRUE
		LEFT JOIN LATERAL (
			SELECT COUNT(*) AS count
			FROM domain_outbox
			WHERE aggregate_type = 'video_task'
			  AND aggregate_id = vt.id
			  AND status = 'dead'
		) AS dead_outbox ON TRUE
		-- Prefer known anomaly candidates inside the bounded window so healthy
		-- recent traffic cannot permanently starve stale operational drift.
		ORDER BY
			CASE
				WHEN vt.status = 'succeeded'
					AND vt.settlement_status NOT IN ('settled', 'not_required') THEN 0
				WHEN vt.status IN ('succeeded', 'failed', 'cancelled')
					AND COALESCE(br.status, '') = 'active' THEN 0
				WHEN COALESCE(br.status, '') = 'active'
					AND br.expires_at IS NOT NULL
					AND br.expires_at <= $2 THEN 0
				WHEN COALESCE(dead_outbox.count, 0) > 0 THEN 0
				-- Include remote-only successes so expiry-aware undeliverable rows
				-- (non-empty but expired result_url) are not treated as healthy.
				WHEN vt.status = 'succeeded'
					AND (vt.local_asset_path IS NULL OR vt.local_asset_path = '') THEN 0
				ELSE 1
			END,
			vt.updated_at DESC,
			vt.id DESC
		LIMIT $1
	`, limit, now)
	if err != nil {
		return service.ReliabilityReconciliationSnapshot{}, fmt.Errorf("query reliability reconciliation snapshot: %w", err)
	}
	defer func() { _ = rows.Close() }()

	snapshot := service.ReliabilityReconciliationSnapshot{Rows: make([]service.ReliabilityReconciliationRow, 0, limit)}
	for rows.Next() {
		var (
			row          service.ReliabilityReconciliationRow
			expiresAt    sql.NullTime
			ledgerRaw    sql.NullString
			projectedRaw sql.NullString
			resultURL    string
			completedAt  sql.NullTime
		)
		if err := rows.Scan(
			&row.TaskID,
			&row.Status,
			&row.DispatchState,
			&row.SettlementStatus,
			&row.ReservationStatus,
			&expiresAt,
			&ledgerRaw,
			&projectedRaw,
			&row.DeadOutboxCount,
			&row.LocalAssetAvailable,
			&resultURL,
			&completedAt,
		); err != nil {
			return service.ReliabilityReconciliationSnapshot{}, fmt.Errorf("scan reliability reconciliation snapshot: %w", err)
		}
		if expiresAt.Valid {
			row.ReservationExpiresAt = expiresAt.Time.UTC()
		}
		var completedPtr *time.Time
		if completedAt.Valid {
			completed := completedAt.Time.UTC()
			completedPtr = &completed
		}
		// Keep URLs out of the public reconciliation row; only the availability bit leaves this layer.
		row.RemoteAssetAvailable = service.ReliabilityRemoteAssetAvailable(resultURL, completedPtr, now)
		ledger, err := reliabilitySnapshotMoney(ledgerRaw)
		if err != nil {
			return service.ReliabilityReconciliationSnapshot{}, fmt.Errorf("parse ledger balance for task %d: %w", row.TaskID, err)
		}
		projected, err := reliabilitySnapshotMoney(projectedRaw)
		if err != nil {
			return service.ReliabilityReconciliationSnapshot{}, fmt.Errorf("parse projected balance for task %d: %w", row.TaskID, err)
		}
		row.LedgerBalanceAfter = ledger
		row.ProjectedBalance = projected
		snapshot.Rows = append(snapshot.Rows, row)
	}
	if err := rows.Err(); err != nil {
		return service.ReliabilityReconciliationSnapshot{}, fmt.Errorf("iterate reliability reconciliation snapshot: %w", err)
	}
	return snapshot, nil
}

func reliabilitySnapshotMoney(raw sql.NullString) (*service.Money, error) {
	if !raw.Valid || strings.TrimSpace(raw.String) == "" {
		return nil, nil
	}
	money, err := service.NewMoney(raw.String, service.CurrencyUSD)
	if err != nil {
		return nil, err
	}
	return &money, nil
}
