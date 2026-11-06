package repository

import (
	"context"
	"encoding/json"
	"errors"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func expectMaintenanceAcquire(mock sqlmock.Sqlmock) {
	mock.ExpectQuery(regexp.QuoteMeta("SELECT pg_try_advisory_lock($1)")).WithArgs(databaseMaintenanceLockKey).WillReturnRows(sqlmock.NewRows([]string{"locked"}).AddRow(true))
	mock.ExpectExec("SET lock_timeout").WillReturnResult(sqlmock.NewResult(0, 0))
}
func expectMaintenanceClose(mock sqlmock.Sqlmock) {
	mock.ExpectExec("RESET lock_timeout").WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectQuery(regexp.QuoteMeta("SELECT pg_advisory_unlock($1)")).WithArgs(databaseMaintenanceLockKey).WillReturnRows(sqlmock.NewRows([]string{"unlocked"}).AddRow(true))
}

func TestDatabaseMaintenanceStats(t *testing.T) {
	db, mock := newSQLMock(t)
	mock.ExpectQuery("SELECT current_database").WillReturnRows(sqlmock.NewRows([]string{"name", "size"}).AddRow("sub2api", 100000))
	mock.ExpectQuery("WITH RECURSIVE relations").WillReturnRows(sqlmock.NewRows([]string{"schema", "name", "data", "indexes", "total", "live", "dead", "vacuum"}).AddRow("public", "usage_logs", 60000, 20000, 80000, 100, 20, nil).AddRow("public", "users", 10000, 1000, 11000, 10, 0, time.Now()))
	stats, err := NewDatabaseMaintenanceRepository(db).Stats(context.Background())
	require.NoError(t, err)
	require.Equal(t, int64(100000), stats.SizeBytes)
	require.True(t, stats.Tables[0].CleanupSupported)
	require.False(t, stats.Tables[1].CleanupSupported)
	require.Nil(t, stats.Tables[0].LastVacuum)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestDatabaseMaintenanceBusy(t *testing.T) {
	db, mock := newSQLMock(t)
	mock.ExpectQuery("SELECT pg_try_advisory_lock").WillReturnRows(sqlmock.NewRows([]string{"locked"}).AddRow(false))
	_, err := NewDatabaseMaintenanceRepository(db).Acquire(context.Background())
	require.ErrorContains(t, err, "already running")
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestDatabaseMaintenanceVacuumOutsideTransactionAndQuoted(t *testing.T) {
	for name, full := range map[string]bool{"regular": false, "full": true} {
		t.Run(name, func(t *testing.T) {
			db, mock := newSQLMock(t)
			expectMaintenanceAcquire(mock)
			session, err := NewDatabaseMaintenanceRepository(db).Acquire(context.Background())
			require.NoError(t, err)
			mock.ExpectQuery("SELECT pg_has_role").WithArgs(`"public"."strange""name"`).WillReturnRows(sqlmock.NewRows([]string{"allowed"}).AddRow(true))
			command := `VACUUM (ANALYZE) "public"."strange""name"`
			if full {
				command = `VACUUM (FULL, ANALYZE) "public"."strange""name"`
			}
			// Any transaction attempt is an unexpected call.
			mock.ExpectExec(regexp.QuoteMeta(command)).WillReturnResult(sqlmock.NewResult(0, 0))
			require.NoError(t, session.Vacuum(context.Background(), `strange"name`, full))
			expectMaintenanceClose(mock)
			session.Close()
			require.NoError(t, mock.ExpectationsWereMet())
		})
	}
}
func TestDatabaseMaintenanceCleanupUsesBoundCutoffAndWhitelist(t *testing.T) {
	db, mock := newSQLMock(t)
	expectMaintenanceAcquire(mock)
	session, err := NewDatabaseMaintenanceRepository(db).Acquire(context.Background())
	require.NoError(t, err)
	cutoff := time.Now().UTC().AddDate(0, 0, -30)
	mock.ExpectExec(`WITH victims AS \(.*SELECT tableoid, ctid FROM "public"\."ops_system_logs" WHERE "created_at" < \$1.*LIMIT 5000.*DELETE FROM "public"\."ops_system_logs"`).WithArgs(cutoff).WillReturnResult(sqlmock.NewResult(0, 5000))
	n, err := session.CleanupBatch(context.Background(), "ops_system_logs", cutoff)
	require.NoError(t, err)
	require.Equal(t, int64(5000), n)
	_, err = session.CleanupBatch(context.Background(), "users", cutoff)
	require.Error(t, err)
	expectMaintenanceClose(mock)
	session.Close()
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestDatabaseMaintenanceJobPersistence(t *testing.T) {
	db, mock := newSQLMock(t)
	repo := NewDatabaseMaintenanceRepository(db)
	mock.ExpectQuery("SELECT value FROM settings").WithArgs(databaseMaintenanceSettingKey).WillReturnRows(sqlmock.NewRows([]string{"value"}))
	job, err := repo.LatestJob(context.Background())
	require.NoError(t, err)
	require.Nil(t, job)
	original := &service.DatabaseMaintenanceJob{ID: "job-123", Status: "running", CreatedBy: 5}
	raw, err := json.Marshal(original)
	require.NoError(t, err)
	expectMaintenanceAcquire(mock)
	session, err := repo.Acquire(context.Background())
	require.NoError(t, err)
	mock.ExpectExec("INSERT INTO settings").WithArgs(databaseMaintenanceSettingKey, string(raw)).WillReturnResult(sqlmock.NewResult(1, 1))
	require.NoError(t, session.SaveJob(context.Background(), original))
	expectMaintenanceClose(mock)
	session.Close()
	mock.ExpectQuery("SELECT value FROM settings").WithArgs(databaseMaintenanceSettingKey).WillReturnRows(sqlmock.NewRows([]string{"value"}).AddRow(string(raw)))
	job, err = repo.LatestJob(context.Background())
	require.NoError(t, err)
	require.Equal(t, original, job)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestDatabaseMaintenanceRejectsVacuumWithoutPrivileges(t *testing.T) {
	db, mock := newSQLMock(t)
	expectMaintenanceAcquire(mock)
	session, err := NewDatabaseMaintenanceRepository(db).Acquire(context.Background())
	require.NoError(t, err)
	mock.ExpectQuery("SELECT pg_has_role").WithArgs(`"public"."users"`).WillReturnRows(sqlmock.NewRows([]string{"allowed"}).AddRow(false))
	require.ErrorContains(t, session.Vacuum(context.Background(), "users", true), "cannot vacuum")
	expectMaintenanceClose(mock)
	session.Close()
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestDatabaseMaintenanceDiscardConnectionIfUnlockFails(t *testing.T) {
	db, mock := newSQLMock(t)
	expectMaintenanceAcquire(mock)
	session, err := NewDatabaseMaintenanceRepository(db).Acquire(context.Background())
	require.NoError(t, err)
	mock.ExpectExec("RESET lock_timeout").WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectQuery("SELECT pg_advisory_unlock").WillReturnError(errors.New("connection error"))
	mock.ExpectClose()
	session.Close()
	require.Zero(t, db.Stats().OpenConnections)
	require.NoError(t, mock.ExpectationsWereMet())
}
