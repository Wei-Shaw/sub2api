//go:build integration

package repository

import (
	"context"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestDatabaseMaintenancePostgres(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	db := integrationDB
	// Isolated fixtures in the disposable integration database.
	_, err := db.ExecContext(ctx, `CREATE TABLE maintenance_size_fixture (id bigint, payload text) PARTITION BY RANGE(id);
 CREATE TABLE maintenance_size_fixture_a PARTITION OF maintenance_size_fixture FOR VALUES FROM (0) TO (10000);
 CREATE TABLE maintenance_size_fixture_b PARTITION OF maintenance_size_fixture FOR VALUES FROM (10000) TO (20000);
 INSERT INTO maintenance_size_fixture SELECT n, repeat(md5(n::text), 20) FROM generate_series(1,15000) n;
 CREATE INDEX ON maintenance_size_fixture(id);
 ANALYZE maintenance_size_fixture;
 INSERT INTO ops_system_logs (created_at,level,message) VALUES (NOW()-INTERVAL '40 days','info','maintenance-old-fixture'),(NOW(),'info','maintenance-new-fixture')`)
	require.NoError(t, err)
	t.Cleanup(func() {
		_, _ = db.ExecContext(context.Background(), `DROP TABLE IF EXISTS maintenance_size_fixture;
 DELETE FROM ops_system_logs WHERE message IN ('maintenance-old-fixture','maintenance-new-fixture');
 DELETE FROM settings WHERE key = 'database_maintenance_latest_job'`)
	})
	repo := NewDatabaseMaintenanceRepository(db)
	stats, err := repo.Stats(ctx)
	require.NoError(t, err)
	require.Positive(t, stats.SizeBytes)
	var root *service.DatabaseTableStats
	for i := range stats.Tables {
		table := &stats.Tables[i]
		require.NotEqual(t, "maintenance_size_fixture_a", table.Name)
		require.NotEqual(t, "maintenance_size_fixture_b", table.Name)
		if table.Name == "maintenance_size_fixture" {
			root = table
		}
	}
	require.NotNil(t, root)
	var expected int64
	require.NoError(t, db.QueryRowContext(ctx, `SELECT pg_total_relation_size('maintenance_size_fixture_a') + pg_total_relation_size('maintenance_size_fixture_b')`).Scan(&expected))
	require.Equal(t, expected, root.TotalBytes)
	session, err := repo.Acquire(ctx)
	require.NoError(t, err)
	defer session.Close()
	_, err = repo.Acquire(ctx)
	require.ErrorContains(t, err, "already running")
	cutoff := time.Now().UTC().AddDate(0, 0, -30)
	n, err := session.CleanupBatch(ctx, "ops_system_logs", cutoff)
	require.NoError(t, err)
	require.Equal(t, int64(1), n)
	var remaining int
	require.NoError(t, db.QueryRowContext(ctx, `SELECT COUNT(*) FROM ops_system_logs WHERE message='maintenance-new-fixture'`).Scan(&remaining))
	require.Equal(t, 1, remaining)
	job := &service.DatabaseMaintenanceJob{ID: "integration-job", Status: "running", CreatedBy: 1}
	require.NoError(t, session.SaveJob(ctx, job))
	persisted, err := repo.LatestJob(ctx)
	require.NoError(t, err)
	require.Equal(t, job.ID, persisted.ID)
	require.NoError(t, session.RecomputeUsage(ctx, cutoff))
	require.NoError(t, session.Vacuum(ctx, "maintenance_size_fixture", false))
	require.NoError(t, session.Vacuum(ctx, "maintenance_size_fixture", true))
	// Session settings are reset and the advisory lock is released on Close.
}
