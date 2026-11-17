//go:build integration

package repository

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestRequestLogRetention_PartitionBoundaryKeepsRecentRows(t *testing.T) {
	ctx := context.Background()
	tx := testTx(t)
	// Temporary tables shadow the real schema and disappear on rollback.
	_, err := tx.ExecContext(ctx, `
		CREATE TEMP TABLE usage_logs (id bigint, created_at timestamptz) PARTITION BY RANGE (created_at);
		CREATE TEMP TABLE retention_july PARTITION OF usage_logs FOR VALUES FROM ('2026-07-01') TO ('2026-08-01');
		CREATE TEMP TABLE retention_august PARTITION OF usage_logs FOR VALUES FROM ('2026-08-01') TO ('2026-09-01');
		INSERT INTO usage_logs VALUES
			(1, '2026-07-17 23:59:59+00'),
			(2, '2026-07-18 00:00:00+00'),
			(3, '2026-08-01 00:00:00+00');
	`)
	require.NoError(t, err)
	var collidingCTIDs bool
	require.NoError(t, tx.QueryRowContext(ctx, `SELECT a.ctid = b.ctid FROM usage_logs a, usage_logs b WHERE a.id = 1 AND b.id = 3`).Scan(&collidingCTIDs))
	require.True(t, collidingCTIDs, "fixture must exercise identical row locations in different partitions")
	repo := newDashboardAggregationRepositoryWithSQL(tx)
	cutoff := time.Date(2026, 7, 18, 0, 0, 0, 0, time.UTC)
	for range 2 {
		require.NoError(t, repo.cleanupUsageLogsBatches(ctx, cutoff))
		rows, err := tx.QueryContext(ctx, `SELECT id FROM usage_logs ORDER BY id`)
		require.NoError(t, err)
		var ids []int64
		for rows.Next() {
			var id int64
			require.NoError(t, rows.Scan(&id))
			ids = append(ids, id)
		}
		require.NoError(t, rows.Err())
		require.NoError(t, rows.Close())
		require.Equal(t, []int64{2, 3}, ids)
	}
}
