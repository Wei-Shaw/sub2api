package repository

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io/fs"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/require"
)

func TestApplyMigrations_NilDB(t *testing.T) {
	err := ApplyMigrations(context.Background(), nil)
	require.Error(t, err)
	require.Contains(t, err.Error(), "nil sql db")
}

func TestApplyMigrations_DelegatesToApplyMigrationsFS(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	mock.ExpectQuery("SELECT pg_try_advisory_lock\\(\\$1\\)").
		WithArgs(migrationsAdvisoryLockID).
		WillReturnError(errors.New("lock failed"))

	err = ApplyMigrations(context.Background(), db)
	require.Error(t, err)
	require.Contains(t, err.Error(), "acquire migrations lock")
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestLatestMigrationBaseline(t *testing.T) {
	t.Run("empty_fs_returns_baseline", func(t *testing.T) {
		version, description, hash, err := latestMigrationBaseline(fstest.MapFS{})
		require.NoError(t, err)
		require.Equal(t, "baseline", version)
		require.Equal(t, "baseline", description)
		require.Equal(t, "", hash)
	})

	t.Run("uses_latest_sorted_sql_file", func(t *testing.T) {
		fsys := fstest.MapFS{
			"001_init.sql": &fstest.MapFile{Data: []byte("CREATE TABLE t1(id int);")},
			"010_final.sql": &fstest.MapFile{
				Data: []byte("CREATE TABLE t2(id int);"),
			},
		}
		version, description, hash, err := latestMigrationBaseline(fsys)
		require.NoError(t, err)
		require.Equal(t, "010_final", version)
		require.Equal(t, "010_final", description)
		require.Len(t, hash, 64)
	})

	t.Run("read_file_error", func(t *testing.T) {
		fsys := fstest.MapFS{
			"010_bad.sql": &fstest.MapFile{Mode: fs.ModeDir},
		}
		_, _, _, err := latestMigrationBaseline(fsys)
		require.Error(t, err)
	})
}

func TestIsMigrationChecksumCompatible_AdditionalCases(t *testing.T) {
	require.False(t, isMigrationChecksumCompatible("unknown.sql", "db", "file"))

	var (
		name string
		rule migrationChecksumCompatibilityRule
	)
	for n, r := range migrationChecksumCompatibilityRules {
		name = n
		rule = r
		break
	}
	require.NotEmpty(t, name)

	require.False(t, isMigrationChecksumCompatible(name, "db-not-accepted", "file-not-match"))
	require.False(t, isMigrationChecksumCompatible(name, "db-not-accepted", rule.fileChecksum))

	var accepted string
	for checksum := range rule.acceptedDBChecksum {
		accepted = checksum
		break
	}
	require.NotEmpty(t, accepted)
	require.True(t, isMigrationChecksumCompatible(name, accepted, rule.fileChecksum))
}

func TestMigrationChecksumCompatibilityRules_CoverEditedUpgradeCompatibilityMigrations(t *testing.T) {
	for _, name := range []string{
		"109_auth_identity_compat_backfill.sql",
		"110_pending_auth_and_provider_default_grants.sql",
		"112_add_payment_order_provider_key_snapshot.sql",
		"115_auth_identity_legacy_external_backfill.sql",
		"116_auth_identity_legacy_external_safety_reports.sql",
		"118_wechat_dual_mode_and_auth_source_defaults.sql",
		"120_enforce_payment_orders_out_trade_no_unique_notx.sql",
		"123_fix_legacy_auth_source_grant_on_signup_defaults.sql",
	} {
		rule, ok := migrationChecksumCompatibilityRules[name]
		require.Truef(t, ok, "missing compatibility rule for %s", name)
		require.NotEmpty(t, rule.fileChecksum)
		require.NotEmpty(t, rule.acceptedDBChecksum)
	}
}

func TestEnsureAtlasBaselineAligned(t *testing.T) {
	t.Run("skip_when_no_legacy_table", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		defer func() { _ = db.Close() }()

		mock.ExpectQuery("SELECT EXISTS \\(").
			WithArgs("schema_migrations").
			WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(false))

		err = ensureAtlasBaselineAligned(context.Background(), db, fstest.MapFS{})
		require.NoError(t, err)
		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("create_atlas_and_insert_baseline_when_empty", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		defer func() { _ = db.Close() }()

		mock.ExpectQuery("SELECT EXISTS \\(").
			WithArgs("schema_migrations").
			WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))
		mock.ExpectQuery("SELECT EXISTS \\(").
			WithArgs("atlas_schema_revisions").
			WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(false))
		mock.ExpectExec("CREATE TABLE IF NOT EXISTS atlas_schema_revisions").
			WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectQuery("SELECT COUNT\\(\\*\\) FROM atlas_schema_revisions").
			WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))
		mock.ExpectExec("INSERT INTO atlas_schema_revisions").
			WithArgs("002_next", "002_next", 1, sqlmock.AnyArg()).
			WillReturnResult(sqlmock.NewResult(1, 1))

		fsys := fstest.MapFS{
			"001_init.sql": &fstest.MapFile{Data: []byte("CREATE TABLE t1(id int);")},
			"002_next.sql": &fstest.MapFile{Data: []byte("CREATE TABLE t2(id int);")},
		}
		err = ensureAtlasBaselineAligned(context.Background(), db, fsys)
		require.NoError(t, err)
		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("error_when_checking_legacy_table", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		defer func() { _ = db.Close() }()

		mock.ExpectQuery("SELECT EXISTS \\(").
			WithArgs("schema_migrations").
			WillReturnError(errors.New("exists failed"))

		err = ensureAtlasBaselineAligned(context.Background(), db, fstest.MapFS{})
		require.Error(t, err)
		require.Contains(t, err.Error(), "check schema_migrations")
		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("error_when_counting_atlas_rows", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		defer func() { _ = db.Close() }()

		mock.ExpectQuery("SELECT EXISTS \\(").
			WithArgs("schema_migrations").
			WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))
		mock.ExpectQuery("SELECT EXISTS \\(").
			WithArgs("atlas_schema_revisions").
			WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))
		mock.ExpectQuery("SELECT COUNT\\(\\*\\) FROM atlas_schema_revisions").
			WillReturnError(errors.New("count failed"))

		err = ensureAtlasBaselineAligned(context.Background(), db, fstest.MapFS{})
		require.Error(t, err)
		require.Contains(t, err.Error(), "count atlas_schema_revisions")
		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("error_when_creating_atlas_table", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		defer func() { _ = db.Close() }()

		mock.ExpectQuery("SELECT EXISTS \\(").
			WithArgs("schema_migrations").
			WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))
		mock.ExpectQuery("SELECT EXISTS \\(").
			WithArgs("atlas_schema_revisions").
			WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(false))
		mock.ExpectExec("CREATE TABLE IF NOT EXISTS atlas_schema_revisions").
			WillReturnError(errors.New("create failed"))

		err = ensureAtlasBaselineAligned(context.Background(), db, fstest.MapFS{})
		require.Error(t, err)
		require.Contains(t, err.Error(), "create atlas_schema_revisions")
		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("error_when_inserting_baseline", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		defer func() { _ = db.Close() }()

		mock.ExpectQuery("SELECT EXISTS \\(").
			WithArgs("schema_migrations").
			WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))
		mock.ExpectQuery("SELECT EXISTS \\(").
			WithArgs("atlas_schema_revisions").
			WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))
		mock.ExpectQuery("SELECT COUNT\\(\\*\\) FROM atlas_schema_revisions").
			WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))
		mock.ExpectExec("INSERT INTO atlas_schema_revisions").
			WithArgs("001_init", "001_init", 1, sqlmock.AnyArg()).
			WillReturnError(errors.New("insert failed"))

		fsys := fstest.MapFS{
			"001_init.sql": &fstest.MapFile{Data: []byte("CREATE TABLE t(id int);")},
		}
		err = ensureAtlasBaselineAligned(context.Background(), db, fsys)
		require.Error(t, err)
		require.Contains(t, err.Error(), "insert atlas baseline")
		require.NoError(t, mock.ExpectationsWereMet())
	})
}

func TestApplyMigrationsFS_ChecksumMismatchRejected(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	prepareMigrationsBootstrapExpectations(mock)
	mock.ExpectQuery("SELECT checksum FROM schema_migrations WHERE filename = \\$1").
		WithArgs("001_init.sql").
		WillReturnRows(sqlmock.NewRows([]string{"checksum"}).AddRow("mismatched-checksum"))
	mock.ExpectExec("SELECT pg_advisory_unlock\\(\\$1\\)").
		WithArgs(migrationsAdvisoryLockID).
		WillReturnResult(sqlmock.NewResult(0, 1))

	fsys := fstest.MapFS{
		"001_init.sql": &fstest.MapFile{Data: []byte("CREATE TABLE t(id int);")},
	}
	err = applyMigrationsFS(context.Background(), db, fsys)
	require.Error(t, err)
	require.Contains(t, err.Error(), "checksum mismatch")
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestApplyMigrationsFS_CheckMigrationQueryError(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	prepareMigrationsBootstrapExpectations(mock)
	mock.ExpectQuery("SELECT checksum FROM schema_migrations WHERE filename = \\$1").
		WithArgs("001_err.sql").
		WillReturnError(errors.New("query failed"))
	mock.ExpectExec("SELECT pg_advisory_unlock\\(\\$1\\)").
		WithArgs(migrationsAdvisoryLockID).
		WillReturnResult(sqlmock.NewResult(0, 1))

	fsys := fstest.MapFS{
		"001_err.sql": &fstest.MapFile{Data: []byte("SELECT 1;")},
	}
	err = applyMigrationsFS(context.Background(), db, fsys)
	require.Error(t, err)
	require.Contains(t, err.Error(), "check migration 001_err.sql")
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestApplyMigrationsFS_SkipEmptyAndAlreadyApplied(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	prepareMigrationsBootstrapExpectations(mock)

	alreadySQL := "CREATE TABLE t(id int);"
	checksum := migrationChecksum(alreadySQL)
	mock.ExpectQuery("SELECT checksum FROM schema_migrations WHERE filename = \\$1").
		WithArgs("001_already.sql").
		WillReturnRows(sqlmock.NewRows([]string{"checksum"}).AddRow(checksum))
	mock.ExpectExec("SELECT pg_advisory_unlock\\(\\$1\\)").
		WithArgs(migrationsAdvisoryLockID).
		WillReturnResult(sqlmock.NewResult(0, 1))

	fsys := fstest.MapFS{
		"000_empty.sql":   &fstest.MapFile{Data: []byte("   \n\t ")},
		"001_already.sql": &fstest.MapFile{Data: []byte(alreadySQL)},
	}
	err = applyMigrationsFS(context.Background(), db, fsys)
	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestApplyMigrationsFS_ReadMigrationError(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	prepareMigrationsBootstrapExpectations(mock)
	mock.ExpectExec("SELECT pg_advisory_unlock\\(\\$1\\)").
		WithArgs(migrationsAdvisoryLockID).
		WillReturnResult(sqlmock.NewResult(0, 1))

	fsys := fstest.MapFS{
		"001_bad.sql": &fstest.MapFile{Mode: fs.ModeDir},
	}
	err = applyMigrationsFS(context.Background(), db, fsys)
	require.Error(t, err)
	require.Contains(t, err.Error(), "read migration 001_bad.sql")
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestPgAdvisoryLockAndUnlock_ErrorBranches(t *testing.T) {
	t.Run("context_cancelled_while_not_locked", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		defer func() { _ = db.Close() }()

		mock.ExpectQuery("SELECT pg_try_advisory_lock\\(\\$1\\)").
			WithArgs(migrationsAdvisoryLockID).
			WillReturnRows(sqlmock.NewRows([]string{"pg_try_advisory_lock"}).AddRow(false))

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Millisecond)
		defer cancel()
		err = pgAdvisoryLock(ctx, db)
		require.Error(t, err)
		require.Contains(t, err.Error(), "acquire migrations lock")
		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("unlock_exec_error", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		defer func() { _ = db.Close() }()

		mock.ExpectExec("SELECT pg_advisory_unlock\\(\\$1\\)").
			WithArgs(migrationsAdvisoryLockID).
			WillReturnError(errors.New("unlock failed"))

		err = pgAdvisoryUnlock(context.Background(), db)
		require.Error(t, err)
		require.Contains(t, err.Error(), "release migrations lock")
		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("acquire_lock_after_retry", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		defer func() { _ = db.Close() }()

		mock.ExpectQuery("SELECT pg_try_advisory_lock\\(\\$1\\)").
			WithArgs(migrationsAdvisoryLockID).
			WillReturnRows(sqlmock.NewRows([]string{"pg_try_advisory_lock"}).AddRow(false))
		mock.ExpectQuery("SELECT pg_try_advisory_lock\\(\\$1\\)").
			WithArgs(migrationsAdvisoryLockID).
			WillReturnRows(sqlmock.NewRows([]string{"pg_try_advisory_lock"}).AddRow(true))

		ctx, cancel := context.WithTimeout(context.Background(), migrationsLockRetryInterval*3)
		defer cancel()
		start := time.Now()
		err = pgAdvisoryLock(ctx, db)
		require.NoError(t, err)
		require.GreaterOrEqual(t, time.Since(start), migrationsLockRetryInterval)
		require.NoError(t, mock.ExpectationsWereMet())
	})
}

func migrationChecksum(content string) string {
	sum := sha256.Sum256([]byte(strings.TrimSpace(content)))
	return hex.EncodeToString(sum[:])
}
