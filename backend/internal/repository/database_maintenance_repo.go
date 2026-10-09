package repository

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/service"
)

const databaseMaintenanceSettingKey = "database_maintenance_latest_job"
const databaseMaintenanceLockKey int64 = 126948986839650

type databaseMaintenanceRepository struct{ db *sql.DB }

func NewDatabaseMaintenanceRepository(db *sql.DB) service.DatabaseMaintenanceRepository {
	return &databaseMaintenanceRepository{db: db}
}

func (r *databaseMaintenanceRepository) Stats(ctx context.Context) (*service.DatabaseStats, error) {
	stats := &service.DatabaseStats{Tables: []service.DatabaseTableStats{}}
	if err := r.db.QueryRowContext(ctx, `SELECT current_database(), pg_database_size(current_database())`).Scan(&stats.Name, &stats.SizeBytes); err != nil {
		return nil, err
	}
	// Aggregate partitions under their logical parent without counting bytes twice.
	rows, err := r.db.QueryContext(ctx, `
WITH RECURSIVE relations AS (
 SELECT c.oid AS root, c.oid AS relid FROM pg_class c
 JOIN pg_namespace n ON n.oid = c.relnamespace
 WHERE n.nspname = 'public' AND c.relkind IN ('r', 'p', 'm') AND NOT c.relispartition
 UNION ALL
 SELECT r.root, i.inhrelid FROM relations r JOIN pg_inherits i ON i.inhparent = r.relid
), sizes AS (
 SELECT r.root,
 SUM(pg_table_size(r.relid))::bigint AS data_bytes,
 SUM(pg_indexes_size(r.relid))::bigint AS index_bytes,
 SUM(pg_total_relation_size(r.relid))::bigint AS total_bytes,
 SUM(COALESCE(s.n_live_tup, 0))::bigint AS live_rows,
 SUM(COALESCE(s.n_dead_tup, 0))::bigint AS dead_rows,
 MAX(GREATEST(s.last_vacuum, s.last_autovacuum)) AS last_vacuum
 FROM relations r LEFT JOIN pg_stat_user_tables s ON s.relid = r.relid
 GROUP BY r.root
)
SELECT 'public', c.relname, s.data_bytes, s.index_bytes, s.total_bytes, s.live_rows, s.dead_rows, s.last_vacuum
FROM sizes s JOIN pg_class c ON c.oid = s.root ORDER BY s.total_bytes DESC, c.relname`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var t service.DatabaseTableStats
		if err := rows.Scan(&t.Schema, &t.Name, &t.DataBytes, &t.IndexBytes, &t.TotalBytes, &t.LiveRows, &t.DeadRows, &t.LastVacuum); err != nil {
			return nil, err
		}
		_, t.CleanupSupported = service.DatabaseCleanupColumn(t.Name)
		stats.Tables = append(stats.Tables, t)
	}
	return stats, rows.Err()
}

func (r *databaseMaintenanceRepository) LatestJob(ctx context.Context) (*service.DatabaseMaintenanceJob, error) {
	var raw string
	err := r.db.QueryRowContext(ctx, `SELECT value FROM settings WHERE key = $1`, databaseMaintenanceSettingKey).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var job service.DatabaseMaintenanceJob
	if err := json.Unmarshal([]byte(raw), &job); err != nil {
		return nil, err
	}
	return &job, nil
}

func (r *databaseMaintenanceRepository) Acquire(ctx context.Context) (service.DatabaseMaintenanceSession, error) {
	conn, err := r.db.Conn(ctx)
	if err != nil {
		return nil, err
	}
	var locked bool
	if err := conn.QueryRowContext(ctx, `SELECT pg_try_advisory_lock($1)`, databaseMaintenanceLockKey).Scan(&locked); err != nil {
		_ = conn.Close()
		return nil, err
	}
	if !locked {
		_ = conn.Close()
		return nil, infraerrors.Conflict("DATABASE_MAINTENANCE_BUSY", "database maintenance is already running")
	}
	session := &databaseMaintenanceSession{conn: conn}
	// Bound lock waits, especially FULL, so production traffic cannot queue indefinitely.
	if _, err := conn.ExecContext(ctx, `SET lock_timeout = '5s'`); err != nil {
		session.Close()
		return nil, err
	}
	return session, nil
}

type databaseMaintenanceSession struct{ conn *sql.Conn }

func (s *databaseMaintenanceSession) SaveJob(ctx context.Context, job *service.DatabaseMaintenanceJob) error {
	raw, err := json.Marshal(job)
	if err != nil {
		return err
	}
	_, err = s.conn.ExecContext(ctx, `INSERT INTO settings (key, value, updated_at) VALUES ($1, $2, NOW())
 ON CONFLICT (key) DO UPDATE SET value = EXCLUDED.value, updated_at = EXCLUDED.updated_at`, databaseMaintenanceSettingKey, string(raw))
	return err
}

func quoteDatabaseIdentifier(name string) string {
	return `"` + strings.ReplaceAll(name, `"`, `""`) + `"`
}

func (s *databaseMaintenanceSession) CleanupBatch(ctx context.Context, table string, cutoff time.Time) (int64, error) {
	column, ok := service.DatabaseCleanupColumn(table)
	if !ok {
		return 0, fmt.Errorf("unsupported cleanup table")
	}
	if table == "usage_logs" {
		// Preserve custom-group rollup invalidation, including partitioned usage logs.
		return cleanupUsageLogsBatchWithRollupInvalidation(ctx, s.conn, cutoff)
	}
	where := quoteDatabaseIdentifier(column) + " < $1"
	if column == "bucket_date" {
		where += "::date"
	}
	name := `"public".` + quoteDatabaseIdentifier(table)
	query := fmt.Sprintf(`WITH victims AS (
 SELECT tableoid, ctid FROM %s WHERE %s ORDER BY %s LIMIT 5000
 ) DELETE FROM %s WHERE (tableoid, ctid) IN (SELECT tableoid, ctid FROM victims)`, name, where, quoteDatabaseIdentifier(column), name)
	result, err := s.conn.ExecContext(ctx, query, cutoff.UTC())
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

func (s *databaseMaintenanceSession) RecomputeUsage(ctx context.Context, cutoff time.Time) error {
	tx, err := s.conn.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if err := lockGroupUsageRollupState(ctx, tx); err != nil {
		return err
	}
	// A retention cleanup covers all history before the cutoff. Rebuild the
	// boundary buckets too, preserving retained records from the same day/hour.
	repo := newDashboardAggregationRepositoryWithSQL(tx)
	if err := repo.RecomputeRange(ctx, time.Date(1, time.January, 1, 0, 0, 0, 0, time.UTC), cutoff); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *databaseMaintenanceSession) Vacuum(ctx context.Context, table string, full bool) error {
	name := `"public".` + quoteDatabaseIdentifier(table)
	// PostgreSQL silently skips tables without maintenance privileges. Require
	// ownership (or database ownership/superuser) rather than claiming success.
	var allowed bool
	err := s.conn.QueryRowContext(ctx, `SELECT pg_has_role(c.relowner, 'USAGE')
 OR pg_has_role(d.datdba, 'USAGE') OR (SELECT rolsuper FROM pg_roles WHERE rolname = current_user)
 FROM pg_class c, pg_database d WHERE c.oid = to_regclass($1) AND d.datname = current_database()`, name).Scan(&allowed)
	if err != nil {
		return err
	}
	if !allowed {
		return fmt.Errorf("database role cannot vacuum this table")
	}
	command := "VACUUM (ANALYZE) "
	if full {
		command = "VACUUM (FULL, ANALYZE) "
	}
	_, err = s.conn.ExecContext(ctx, command+name)
	return err
}

func (s *databaseMaintenanceSession) Close() {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, resetErr := s.conn.ExecContext(ctx, `RESET lock_timeout`)
	var unlocked bool
	unlockErr := s.conn.QueryRowContext(ctx, `SELECT pg_advisory_unlock($1)`, databaseMaintenanceLockKey).Scan(&unlocked)
	if resetErr != nil || unlockErr != nil || !unlocked {
		// Never return a connection carrying a session-level lock to the pool.
		_ = s.conn.Raw(func(any) error { return driver.ErrBadConn })
	}
	_ = s.conn.Close()
}
