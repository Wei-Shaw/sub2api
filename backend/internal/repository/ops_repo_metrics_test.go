package repository

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

// systemMetricsInsertArgs is a 40-element arg list matching the INSERT into
// ops_system_metrics. The first two positions are fixed; the rest default to
// sqlmock.AnyArg() and can be overridden by the caller.
func systemMetricsInsertArgs(createdAt time.Time, window int, overrides map[int]driver.Value) []driver.Value {
	args := make([]driver.Value, 40)
	for i := range args {
		args[i] = sqlmock.AnyArg()
	}
	args[0] = createdAt
	args[1] = window
	for idx, val := range overrides {
		if idx >= 0 && idx < len(args) {
			args[idx] = val
		}
	}
	return args
}

func TestOpsInsertSystemMetricsPreservesExplicitZeroDBConnActive(t *testing.T) {
	db, mock := newSQLMock(t)
	repo := &opsRepository{db: db}

	zero := 0
	four := 4
	goroutines := 42
	queueDepth := 0
	dbOK := true
	redisOK := true

	input := &service.OpsInsertSystemMetricsInput{
		CreatedAt:             time.Date(2026, 9, 4, 12, 0, 0, 0, time.UTC),
		WindowMinutes:         1,
		DBConnActive:          &zero, // bug case: explicit 0
		DBConnIdle:            &four,
		DBConnWaiting:         &zero,
		RedisConnTotal:        &four,
		RedisConnIdle:         &four,
		GoroutineCount:        &goroutines,
		ConcurrencyQueueDepth: &queueDepth,
		DBOK:                  &dbOK,
		RedisOK:               &redisOK,
	}

	// $36 = db_conn_active, $37 = db_conn_idle, $38 = db_conn_waiting
	args := systemMetricsInsertArgs(input.CreatedAt, input.WindowMinutes, map[int]driver.Value{
		35: sql.NullInt64{Int64: 0, Valid: true},
		36: sql.NullInt64{Int64: 4, Valid: true},
		37: sql.NullInt64{Int64: 0, Valid: true},
		38: sql.NullInt64{Int64: 42, Valid: true},
		39: sql.NullInt64{Int64: 0, Valid: true},
	})

	mock.ExpectExec(`(?s)INSERT INTO ops_system_metrics.*VALUES.*\$1,\$2,\$3,\$4,`).
		WithArgs(args...).
		WillReturnResult(sqlmock.NewResult(1, 1))

	require.NoError(t, repo.InsertSystemMetrics(context.Background(), input))
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestOpsInsertSystemMetricsNilPointerRemainsNull(t *testing.T) {
	db, mock := newSQLMock(t)
	repo := &opsRepository{db: db}

	input := &service.OpsInsertSystemMetricsInput{
		CreatedAt:     time.Date(2026, 9, 4, 12, 0, 0, 0, time.UTC),
		WindowMinutes: 1,
		// All pool/goroutine fields left nil — should persist as SQL NULL.
	}

	args := systemMetricsInsertArgs(input.CreatedAt, input.WindowMinutes, map[int]driver.Value{
		34: sql.NullInt64{Valid: false},
		35: sql.NullInt64{Valid: false},
		36: sql.NullInt64{Valid: false},
		37: sql.NullInt64{Valid: false},
		38: sql.NullInt64{Valid: false},
		39: sql.NullInt64{Valid: false},
	})

	mock.ExpectExec(`(?s)INSERT INTO ops_system_metrics.*VALUES.*\$1,\$2,\$3,\$4,`).
		WithArgs(args...).
		WillReturnResult(sqlmock.NewResult(1, 1))

	require.NoError(t, repo.InsertSystemMetrics(context.Background(), input))
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestOpsGetLatestSystemMetricsRoundTripsZeroDBConnActive(t *testing.T) {
	db, mock := newSQLMock(t)
	repo := &opsRepository{db: db}

	rows := sqlmock.NewRows([]string{
		"id", "created_at", "window_minutes",
		"cpu_usage_percent", "memory_used_mb", "memory_total_mb", "memory_usage_percent",
		"db_ok", "redis_ok",
		"redis_conn_total", "redis_conn_idle",
		"db_conn_active", "db_conn_idle", "db_conn_waiting",
		"goroutine_count", "concurrency_queue_depth", "account_switch_count",
	}).AddRow(
		int64(1),
		time.Date(2026, 9, 4, 12, 0, 0, 0, time.UTC),
		1,
		sql.NullFloat64{Valid: false},
		sql.NullInt64{Valid: false},
		sql.NullInt64{Valid: false},
		sql.NullFloat64{Valid: false},
		sql.NullBool{Valid: false},
		sql.NullBool{Valid: false},
		sql.NullInt64{Int64: 4, Valid: true},
		sql.NullInt64{Int64: 4, Valid: true},
		sql.NullInt64{Int64: 0, Valid: true}, // db_conn_active = 0 (the bug case)
		sql.NullInt64{Int64: 4, Valid: true},
		sql.NullInt64{Int64: 0, Valid: true},
		sql.NullInt64{Int64: 42, Valid: true},
		sql.NullInt64{Int64: 0, Valid: true},
		sql.NullInt64{Int64: 0, Valid: true},
	)

	mock.ExpectQuery(`(?s)SELECT.*FROM ops_system_metrics.*WHERE window_minutes = \$1.*LIMIT 1`).
		WithArgs(1).
		WillReturnRows(rows)

	snap, err := repo.GetLatestSystemMetrics(context.Background(), 1)
	require.NoError(t, err)
	require.NotNil(t, snap)
	require.NotNil(t, snap.DBConnActive, "DBConnActive must not be nil when stored 0 is observed")
	require.Zero(t, *snap.DBConnActive)
	require.NotNil(t, snap.DBConnIdle)
	require.Equal(t, 4, *snap.DBConnIdle)
	require.NotNil(t, snap.DBConnWaiting)
	require.Zero(t, *snap.DBConnWaiting)
	require.NoError(t, mock.ExpectationsWereMet())
}
