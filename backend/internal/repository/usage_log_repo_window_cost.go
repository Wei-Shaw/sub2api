package repository

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/lib/pq"
)

// GetAccountWindowCostsBatch returns the durable incremental scheduling cost for
// accounts that share a window start. A state row is rebuilt from usage_logs only
// on its first read or after the account window/source rows change.
func (r *usageLogRepository) GetAccountWindowCostsBatch(ctx context.Context, accountIDs []int64, startTime time.Time) (map[int64]float64, error) {
	result := make(map[int64]float64)
	ids := normalizePositiveInt64IDs(accountIDs)
	if len(ids) == 0 {
		return result, nil
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })

	rows, err := r.sql.QueryContext(ctx, `
		SELECT account_id, standard_cost
		FROM account_window_cost_state
		WHERE account_id = ANY($1)
			AND window_start = $2
			AND initialized IS TRUE
	`, pq.Array(ids), startTime)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var accountID int64
		var cost float64
		if err := rows.Scan(&accountID, &cost); err != nil {
			_ = rows.Close()
			return nil, err
		}
		result[accountID] = cost
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return nil, err
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}

	missing := make([]int64, 0, len(ids)-len(result))
	for _, accountID := range ids {
		if _, ok := result[accountID]; ok {
			continue
		}
		missing = append(missing, accountID)
	}
	if len(missing) == 0 {
		return result, nil
	}

	initialized, err := r.initializeAccountWindowCosts(ctx, missing, startTime)
	if err != nil {
		return nil, err
	}
	for accountID, cost := range initialized {
		result[accountID] = cost
	}
	return result, nil
}

func (r *usageLogRepository) initializeAccountWindowCosts(ctx context.Context, accountIDs []int64, startTime time.Time) (_ map[int64]float64, err error) {
	if r == nil || r.db == nil {
		return nil, errors.New("usage log repository db is nil")
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() {
		if tx != nil {
			_ = tx.Rollback()
		}
	}()

	if _, err := tx.ExecContext(ctx, `
		INSERT INTO account_window_cost_state (
			account_id, window_start, standard_cost, initialized, updated_at
		)
		SELECT input.account_id, $2, 0, FALSE, NOW()
		FROM UNNEST($1::bigint[]) AS input(account_id)
		ORDER BY input.account_id
		ON CONFLICT (account_id) DO NOTHING
	`, pq.Array(accountIDs), startTime); err != nil {
		return nil, err
	}

	rows, err := tx.QueryContext(ctx, `
		SELECT account_id, window_start, standard_cost, initialized
		FROM account_window_cost_state
		WHERE account_id = ANY($1)
		ORDER BY account_id
		FOR UPDATE
	`, pq.Array(accountIDs))
	if err != nil {
		return nil, err
	}
	result := make(map[int64]float64, len(accountIDs))
	rebuildIDs := make([]int64, 0, len(accountIDs))
	for rows.Next() {
		var (
			accountID   int64
			storedStart time.Time
			storedCost  float64
			initialized bool
		)
		if err := rows.Scan(&accountID, &storedStart, &storedCost, &initialized); err != nil {
			_ = rows.Close()
			return nil, err
		}
		if initialized && storedStart.Equal(startTime) {
			result[accountID] = storedCost
			continue
		}
		rebuildIDs = append(rebuildIDs, accountID)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return nil, err
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	if len(result)+len(rebuildIDs) != len(accountIDs) {
		return nil, fmt.Errorf("account window cost state count mismatch: got=%d want=%d", len(result)+len(rebuildIDs), len(accountIDs))
	}

	if len(rebuildIDs) > 0 {
		// Reset provisional trigger totals before scanning. The ordered row locks
		// serialize concurrent INSERT triggers: committed rows are included by this
		// scan, while in-flight rows wait and increment state after commit.
		if _, err := tx.ExecContext(ctx, `
			UPDATE account_window_cost_state
			SET window_start = $2,
				standard_cost = 0,
				initialized = FALSE,
				updated_at = NOW()
			WHERE account_id = ANY($1)
		`, pq.Array(rebuildIDs), startTime); err != nil {
			return nil, err
		}

		rows, err = tx.QueryContext(ctx, `
			WITH targets AS (
				SELECT UNNEST($1::bigint[]) AS account_id
			), totals AS (
				SELECT usage_logs.account_id, SUM(usage_logs.total_cost) AS standard_cost
				FROM usage_logs
				JOIN targets USING (account_id)
				WHERE usage_logs.created_at >= $2
				GROUP BY usage_logs.account_id
			)
			UPDATE account_window_cost_state AS state
			SET standard_cost = COALESCE(totals.standard_cost, 0),
				initialized = TRUE,
				updated_at = NOW()
			FROM targets
			LEFT JOIN totals USING (account_id)
			WHERE state.account_id = targets.account_id
			RETURNING state.account_id, state.standard_cost
		`, pq.Array(rebuildIDs), startTime)
		if err != nil {
			return nil, err
		}
		for rows.Next() {
			var accountID int64
			var cost float64
			if err := rows.Scan(&accountID, &cost); err != nil {
				_ = rows.Close()
				return nil, err
			}
			result[accountID] = cost
		}
		if err := rows.Err(); err != nil {
			_ = rows.Close()
			return nil, err
		}
		if err := rows.Close(); err != nil {
			return nil, err
		}
	}
	if len(result) != len(accountIDs) {
		return nil, fmt.Errorf("initialized account window cost count mismatch: got=%d want=%d", len(result), len(accountIDs))
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}
	tx = nil
	return result, nil
}

var _ interface {
	GetAccountWindowCostsBatch(context.Context, []int64, time.Time) (map[int64]float64, error)
} = (*usageLogRepository)(nil)
