package repository

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"testing"
	"testing/fstest"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/require"
)

const (
	testTargetRelationOID int64 = 42
	testIndexOID          int64 = 84
	testIndexDefinition         = `CREATE INDEX test_definition`
)

func TestValidateMigrationExecutionMode(t *testing.T) {
	t.Run("事务迁移包含CONCURRENTLY会被拒绝", func(t *testing.T) {
		nonTx, err := validateMigrationExecutionMode("001_add_idx.sql", "CREATE INDEX CONCURRENTLY idx_a ON t(a);")
		require.False(t, nonTx)
		require.Error(t, err)
	})

	t.Run("notx迁移要求CREATE使用IF NOT EXISTS", func(t *testing.T) {
		nonTx, err := validateMigrationExecutionMode("001_add_idx_notx.sql", "CREATE INDEX CONCURRENTLY idx_a ON t(a);")
		require.False(t, nonTx)
		require.Error(t, err)
	})

	t.Run("notx迁移要求DROP使用IF EXISTS", func(t *testing.T) {
		nonTx, err := validateMigrationExecutionMode("001_drop_idx_notx.sql", "DROP INDEX CONCURRENTLY idx_a;")
		require.False(t, nonTx)
		require.Error(t, err)
	})

	t.Run("notx迁移禁止事务控制语句", func(t *testing.T) {
		nonTx, err := validateMigrationExecutionMode("001_add_idx_notx.sql", "BEGIN; CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_a ON t(a); COMMIT;")
		require.False(t, nonTx)
		require.Error(t, err)
	})

	t.Run("notx迁移禁止混用非CONCURRENTLY语句", func(t *testing.T) {
		nonTx, err := validateMigrationExecutionMode("001_add_idx_notx.sql", "CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_a ON t(a); UPDATE t SET a = 1;")
		require.False(t, nonTx)
		require.Error(t, err)
	})

	t.Run("notx迁移允许幂等并发索引语句", func(t *testing.T) {
		nonTx, err := validateMigrationExecutionMode("001_add_idx_notx.sql", `
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_a ON t(a);
DROP INDEX CONCURRENTLY IF EXISTS idx_b;
`)
		require.True(t, nonTx)
		require.NoError(t, err)
	})

	t.Run("字符串和注释里的关键字不改变执行模式", func(t *testing.T) {
		nonTx, err := validateMigrationExecutionMode("001_regular.sql", `
INSERT INTO audit_log(message) VALUES ('CREATE INDEX CONCURRENTLY; still data');
/* CREATE INDEX CONCURRENTLY IF NOT EXISTS ignored ON t(a); */
`)
		require.False(t, nonTx)
		require.NoError(t, err)
	})

	t.Run("notx迁移拒绝Unicode转义索引名", func(t *testing.T) {
		nonTx, err := validateMigrationExecutionMode(
			"001_unicode_index_notx.sql",
			`CREATE INDEX CONCURRENTLY IF NOT EXISTS U&"idx\0061" ON t(a);`,
		)
		require.False(t, nonTx)
		require.ErrorContains(t, err, "unicode escaped index identifiers")
	})
}

func TestSplitSQLStatements_PostgreSQLSyntax(t *testing.T) {
	statements, err := splitSQLStatements(`
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_a ON t(a) WHERE value = 'one;two';
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_b ON t(b) WHERE value = E'one\';two';
/* outer; /* nested; */ done; */
CREATE INDEX CONCURRENTLY IF NOT EXISTS "idx;quoted" ON t(c); -- trailing;
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_d ON t(($tag$one;two$tag$));
`)
	require.NoError(t, err)
	require.Len(t, statements, 4)
	require.Contains(t, statements[0], "'one;two'")
	require.Contains(t, statements[1], `E'one\';two'`)
	require.Contains(t, statements[2], `"idx;quoted"`)
	require.Contains(t, statements[3], `$tag$one;two$tag$`)

	_, err = splitSQLStatements("SELECT 'unterminated")
	require.ErrorContains(t, err, "unterminated string literal")
	_, err = splitSQLStatements("SELECT 1 /* unterminated")
	require.ErrorContains(t, err, "unterminated block comment")

	identifierDollar, err := splitSQLStatements(`CREATE INDEX CONCURRENTLY IF NOT EXISTS foo$tag$bar ON t(a);`)
	require.NoError(t, err)
	require.Len(t, identifierDollar, 1)
	definition, ok, err := parseCreateConcurrentIndex(identifierDollar[0])
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, "foo$tag$bar", definition.name)

	definition, ok, err = parseCreateConcurrentIndex(
		`CrEaTe UNIQUE INDEX CONCURRENTLY /* before */ IF /* middle */ NOT EXISTS /* after */ idx_strict ON t(a)`,
	)
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t,
		`CrEaTe UNIQUE INDEX CONCURRENTLY /* after */ idx_strict ON t(a)`,
		definition.strictStatement,
	)
	definition, ok, err = parseCreateConcurrentIndex("CREATE\fINDEX\fCONCURRENTLY\fIF\fNOT\fEXISTS\fidx_formfeed\fON\ft(a)")
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, "idx_formfeed", definition.name)

	unicodeString, err := splitSQLStatements("SELECT U&'safe\\' UESCAPE '!'; DROP INDEX CONCURRENTLY IF EXISTS old_idx;")
	require.NoError(t, err)
	require.Len(t, unicodeString, 2)

	longName := strings.Repeat("a", postgresIdentifierMaxBytes+1)
	_, _, err = parseCreateConcurrentIndex("CREATE INDEX CONCURRENTLY IF NOT EXISTS " + longName + " ON t(a)")
	require.ErrorContains(t, err, "63-byte limit")
}

func TestApplyMigrationsFS_NonTransactionalMigration(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	prepareMigrationsBootstrapExpectations(mock)
	mock.ExpectQuery("SELECT checksum FROM schema_migrations WHERE filename = \\$1").
		WithArgs("001_add_idx_notx.sql").
		WillReturnError(sql.ErrNoRows)
	expectIndexSchema(mock, `"t"`, "public")
	expectStepIntentAbsent(mock, "001_add_idx_notx.sql", 1, &qualifiedIndexName{schema: "public", name: "idx_t_a", targetRelationOID: testTargetRelationOID})
	expectExistingIndex(mock, "public", "idx_t_a", false, 0)
	expectStepIntentCreate(mock, "001_add_idx_notx.sql", 1, &qualifiedIndexName{schema: "public", name: "idx_t_a", targetRelationOID: testTargetRelationOID})
	mock.ExpectExec("CREATE INDEX CONCURRENTLY idx_t_a ON t\\(a\\)").
		WillReturnResult(sqlmock.NewResult(0, 0))
	expectIndexHealthy(mock, "public", "idx_t_a", true)
	expectStepIndexComplete(mock, "001_add_idx_notx.sql", 1, &qualifiedIndexName{schema: "public", name: "idx_t_a", targetRelationOID: testTargetRelationOID})
	expectFinalizeNonTransactionalMigration(mock, "001_add_idx_notx.sql")
	mock.ExpectExec("SELECT pg_advisory_unlock\\(\\$1\\)").
		WithArgs(migrationsAdvisoryLockID).
		WillReturnResult(sqlmock.NewResult(0, 1))

	fsys := fstest.MapFS{
		"001_add_idx_notx.sql": &fstest.MapFile{
			Data: []byte("CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_t_a ON t(a);"),
		},
	}

	err = applyMigrationsFS(context.Background(), db, fsys)
	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestApplyMigrationsFS_NonTransactionalMigration_MultiStatements(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	prepareMigrationsBootstrapExpectations(mock)
	mock.ExpectQuery("SELECT checksum FROM schema_migrations WHERE filename = \\$1").
		WithArgs("001_add_multi_idx_notx.sql").
		WillReturnError(sql.ErrNoRows)
	expectIndexSchema(mock, `"t"`, "public")
	expectStepIntentAbsent(mock, "001_add_multi_idx_notx.sql", 1, &qualifiedIndexName{schema: "public", name: "idx_t_a", targetRelationOID: testTargetRelationOID})
	expectExistingIndex(mock, "public", "idx_t_a", false, 0)
	expectStepIntentCreate(mock, "001_add_multi_idx_notx.sql", 1, &qualifiedIndexName{schema: "public", name: "idx_t_a", targetRelationOID: testTargetRelationOID})
	mock.ExpectExec("CREATE INDEX CONCURRENTLY idx_t_a ON t\\(a\\)").
		WillReturnResult(sqlmock.NewResult(0, 0))
	expectIndexHealthy(mock, "public", "idx_t_a", true)
	expectStepIndexComplete(mock, "001_add_multi_idx_notx.sql", 1, &qualifiedIndexName{schema: "public", name: "idx_t_a", targetRelationOID: testTargetRelationOID})
	expectIndexSchema(mock, `"t"`, "public")
	expectStepIntentAbsent(mock, "001_add_multi_idx_notx.sql", 2, &qualifiedIndexName{schema: "public", name: "idx_t_b", targetRelationOID: testTargetRelationOID})
	expectExistingIndex(mock, "public", "idx_t_b", false, 0)
	expectStepIntentCreate(mock, "001_add_multi_idx_notx.sql", 2, &qualifiedIndexName{schema: "public", name: "idx_t_b", targetRelationOID: testTargetRelationOID})
	mock.ExpectExec("CREATE INDEX CONCURRENTLY idx_t_b ON t\\(b\\)").
		WillReturnResult(sqlmock.NewResult(0, 0))
	expectIndexHealthy(mock, "public", "idx_t_b", true)
	expectStepIndexComplete(mock, "001_add_multi_idx_notx.sql", 2, &qualifiedIndexName{schema: "public", name: "idx_t_b", targetRelationOID: testTargetRelationOID})
	expectFinalizeNonTransactionalMigration(mock, "001_add_multi_idx_notx.sql")
	mock.ExpectExec("SELECT pg_advisory_unlock\\(\\$1\\)").
		WithArgs(migrationsAdvisoryLockID).
		WillReturnResult(sqlmock.NewResult(0, 1))

	fsys := fstest.MapFS{
		"001_add_multi_idx_notx.sql": &fstest.MapFile{
			Data: []byte(`
-- first
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_t_a ON t(a);
-- second
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_t_b ON t(b);
`),
		},
	}

	err = applyMigrationsFS(context.Background(), db, fsys)
	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestApplyMigrationsFS_NonTransactionalMigration_LatestAPIKeyIPIndexDropsInvalidIndexBeforeRetry(t *testing.T) {
	const migrationName = "174_add_usage_logs_api_key_latest_ip_index_notx.sql"
	const indexName = "idx_usage_logs_api_key_latest_ip"

	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	prepareMigrationsBootstrapExpectations(mock)
	mock.ExpectQuery("SELECT checksum FROM schema_migrations WHERE filename = \\$1").
		WithArgs(migrationName).
		WillReturnError(sql.ErrNoRows)
	expectIndexSchema(mock, `"usage_logs"`, "public")
	expectStepIntentAbsent(mock, migrationName, 1, &qualifiedIndexName{schema: "public", name: indexName, targetRelationOID: testTargetRelationOID})
	expectExistingIndex(mock, "public", indexName, true, testTargetRelationOID)
	expectStepIntentCreate(mock, migrationName, 1, &qualifiedIndexName{schema: "public", name: indexName, targetRelationOID: testTargetRelationOID})
	mock.ExpectExec(`DROP INDEX CONCURRENTLY IF EXISTS "public"\."idx_usage_logs_api_key_latest_ip"`).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec("CREATE INDEX CONCURRENTLY idx_usage_logs_api_key_latest_ip").
		WillReturnResult(sqlmock.NewResult(0, 0))
	expectIndexHealthy(mock, "public", indexName, true)
	expectStepIndexComplete(mock, migrationName, 1, &qualifiedIndexName{schema: "public", name: indexName, targetRelationOID: testTargetRelationOID})
	expectFinalizeNonTransactionalMigration(mock, migrationName)
	mock.ExpectExec("SELECT pg_advisory_unlock\\(\\$1\\)").
		WithArgs(migrationsAdvisoryLockID).
		WillReturnResult(sqlmock.NewResult(0, 1))

	fsys := fstest.MapFS{
		migrationName: &fstest.MapFile{
			Data: []byte(`
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_usage_logs_api_key_latest_ip
    ON usage_logs (api_key_id, created_at DESC, id DESC)
    INCLUDE (ip_address)
    WHERE ip_address IS NOT NULL AND ip_address <> '';
`),
		},
	}

	err = applyMigrationsFS(context.Background(), db, fsys)
	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestApplyMigrationsFS_PaymentOrdersOutTradeNoUniqueMigration_FailsFastOnDuplicatePrecheck(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	prepareMigrationsBootstrapExpectations(mock)
	mock.ExpectQuery("SELECT checksum FROM schema_migrations WHERE filename = \\$1").
		WithArgs("120_enforce_payment_orders_out_trade_no_unique_notx.sql").
		WillReturnError(sql.ErrNoRows)
	mock.ExpectQuery("SELECT out_trade_no, COUNT\\(\\*\\) AS duplicate_count FROM payment_orders").
		WillReturnRows(sqlmock.NewRows([]string{"out_trade_no", "duplicate_count"}).AddRow("dup-out-trade-no", 2))
	mock.ExpectExec("SELECT pg_advisory_unlock\\(\\$1\\)").
		WithArgs(migrationsAdvisoryLockID).
		WillReturnResult(sqlmock.NewResult(0, 1))

	fsys := fstest.MapFS{
		"120_enforce_payment_orders_out_trade_no_unique_notx.sql": &fstest.MapFile{
			Data: []byte(`
CREATE UNIQUE INDEX CONCURRENTLY IF NOT EXISTS paymentorder_out_trade_no_unique
    ON payment_orders (out_trade_no)
    WHERE out_trade_no <> '';

DROP INDEX CONCURRENTLY IF EXISTS paymentorder_out_trade_no;
`),
		},
	}

	err = applyMigrationsFS(context.Background(), db, fsys)
	require.Error(t, err)
	require.Contains(t, err.Error(), "duplicate out_trade_no")
	require.Contains(t, err.Error(), "dup-out-trade-no")
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestApplyMigrationsFS_PaymentOrdersOutTradeNoUniqueMigration_DropsInvalidIndexBeforeRetry(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	prepareMigrationsBootstrapExpectations(mock)
	mock.ExpectQuery("SELECT checksum FROM schema_migrations WHERE filename = \\$1").
		WithArgs("120_enforce_payment_orders_out_trade_no_unique_notx.sql").
		WillReturnError(sql.ErrNoRows)
	mock.ExpectQuery("SELECT out_trade_no, COUNT\\(\\*\\) AS duplicate_count FROM payment_orders").
		WillReturnRows(sqlmock.NewRows([]string{"out_trade_no", "duplicate_count"}))
	expectIndexSchema(mock, `"payment_orders"`, "public")
	expectStepIntentAbsent(mock, "120_enforce_payment_orders_out_trade_no_unique_notx.sql", 1, &qualifiedIndexName{schema: "public", name: "paymentorder_out_trade_no_unique", targetRelationOID: testTargetRelationOID})
	expectExistingIndex(mock, "public", "paymentorder_out_trade_no_unique", true, testTargetRelationOID)
	expectStepIntentCreate(mock, "120_enforce_payment_orders_out_trade_no_unique_notx.sql", 1, &qualifiedIndexName{schema: "public", name: "paymentorder_out_trade_no_unique", targetRelationOID: testTargetRelationOID})
	mock.ExpectExec(`DROP INDEX CONCURRENTLY IF EXISTS "public"\."paymentorder_out_trade_no_unique"`).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec("CREATE UNIQUE INDEX CONCURRENTLY paymentorder_out_trade_no_unique").
		WillReturnResult(sqlmock.NewResult(0, 0))
	expectIndexHealthy(mock, "public", "paymentorder_out_trade_no_unique", true)
	expectStepIndexComplete(mock, "120_enforce_payment_orders_out_trade_no_unique_notx.sql", 1, &qualifiedIndexName{schema: "public", name: "paymentorder_out_trade_no_unique", targetRelationOID: testTargetRelationOID})
	expectStepIntentAbsent(mock, "120_enforce_payment_orders_out_trade_no_unique_notx.sql", 2, nil)
	expectStepIntentCreate(mock, "120_enforce_payment_orders_out_trade_no_unique_notx.sql", 2, nil)
	mock.ExpectExec("DROP INDEX CONCURRENTLY IF EXISTS paymentorder_out_trade_no").
		WillReturnResult(sqlmock.NewResult(0, 0))
	expectFinalizeNonTransactionalMigration(mock, "120_enforce_payment_orders_out_trade_no_unique_notx.sql")
	mock.ExpectExec("SELECT pg_advisory_unlock\\(\\$1\\)").
		WithArgs(migrationsAdvisoryLockID).
		WillReturnResult(sqlmock.NewResult(0, 1))

	fsys := fstest.MapFS{
		"120_enforce_payment_orders_out_trade_no_unique_notx.sql": &fstest.MapFile{
			Data: []byte(`
CREATE UNIQUE INDEX CONCURRENTLY IF NOT EXISTS paymentorder_out_trade_no_unique
    ON payment_orders (out_trade_no)
    WHERE out_trade_no <> '';

DROP INDEX CONCURRENTLY IF EXISTS paymentorder_out_trade_no;
`),
		},
	}

	err = applyMigrationsFS(context.Background(), db, fsys)
	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestApplyMigrationsFS_SchedulerOutboxPendingDedupKeyMigration_DropsInvalidIndexBeforeRetry(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	prepareMigrationsBootstrapExpectations(mock)
	mock.ExpectQuery("SELECT checksum FROM schema_migrations WHERE filename = \\$1").
		WithArgs("153_scheduler_outbox_pending_dedup_key_index_notx.sql").
		WillReturnError(sql.ErrNoRows)
	expectIndexSchema(mock, `"scheduler_outbox"`, "public")
	expectStepIntentAbsent(mock, "153_scheduler_outbox_pending_dedup_key_index_notx.sql", 1, &qualifiedIndexName{schema: "public", name: "idx_scheduler_outbox_pending_dedup_key", targetRelationOID: testTargetRelationOID})
	expectExistingIndex(mock, "public", "idx_scheduler_outbox_pending_dedup_key", true, testTargetRelationOID)
	expectStepIntentCreate(mock, "153_scheduler_outbox_pending_dedup_key_index_notx.sql", 1, &qualifiedIndexName{schema: "public", name: "idx_scheduler_outbox_pending_dedup_key", targetRelationOID: testTargetRelationOID})
	mock.ExpectExec(`DROP INDEX CONCURRENTLY IF EXISTS "public"\."idx_scheduler_outbox_pending_dedup_key"`).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec("CREATE UNIQUE INDEX CONCURRENTLY idx_scheduler_outbox_pending_dedup_key").
		WillReturnResult(sqlmock.NewResult(0, 0))
	expectIndexHealthy(mock, "public", "idx_scheduler_outbox_pending_dedup_key", true)
	expectStepIndexComplete(mock, "153_scheduler_outbox_pending_dedup_key_index_notx.sql", 1, &qualifiedIndexName{schema: "public", name: "idx_scheduler_outbox_pending_dedup_key", targetRelationOID: testTargetRelationOID})
	expectFinalizeNonTransactionalMigration(mock, "153_scheduler_outbox_pending_dedup_key_index_notx.sql")
	mock.ExpectExec("SELECT pg_advisory_unlock\\(\\$1\\)").
		WithArgs(migrationsAdvisoryLockID).
		WillReturnResult(sqlmock.NewResult(0, 1))

	fsys := fstest.MapFS{
		"153_scheduler_outbox_pending_dedup_key_index_notx.sql": &fstest.MapFile{
			Data: []byte(`
CREATE UNIQUE INDEX CONCURRENTLY IF NOT EXISTS idx_scheduler_outbox_pending_dedup_key
    ON scheduler_outbox (dedup_key)
    WHERE dedup_key IS NOT NULL;
`),
		},
	}

	err = applyMigrationsFS(context.Background(), db, fsys)
	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestApplyMigrationsFS_NonTransactionalMigration_GenericInvalidIndexRecovery(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	const migrationName = "999_future_index_notx.sql"
	const statement = "CREATE INDEX CONCURRENTLY IF NOT EXISTS future_idx ON future_table(value);"
	fsys := fstest.MapFS{
		migrationName: &fstest.MapFile{Data: []byte(statement)},
	}

	prepareMigrationsBootstrapExpectations(mock)
	mock.ExpectQuery("SELECT checksum FROM schema_migrations WHERE filename = \\$1").
		WithArgs(migrationName).
		WillReturnError(sql.ErrNoRows)
	expectIndexSchema(mock, `"future_table"`, "public")
	expectStepIntentAbsent(mock, migrationName, 1, &qualifiedIndexName{schema: "public", name: "future_idx", targetRelationOID: testTargetRelationOID})
	expectExistingIndex(mock, "public", "future_idx", false, 0)
	expectStepIntentCreate(mock, migrationName, 1, &qualifiedIndexName{schema: "public", name: "future_idx", targetRelationOID: testTargetRelationOID})
	mock.ExpectExec("CREATE INDEX CONCURRENTLY future_idx").
		WillReturnError(errors.New("build canceled after PostgreSQL left an invalid index"))
	mock.ExpectExec("SELECT pg_advisory_unlock\\(\\$1\\)").
		WithArgs(migrationsAdvisoryLockID).
		WillReturnResult(sqlmock.NewResult(0, 1))

	err = applyMigrationsFS(context.Background(), db, fsys)
	require.ErrorContains(t, err, "build canceled")

	prepareMigrationsBootstrapExpectations(mock)
	mock.ExpectQuery("SELECT checksum FROM schema_migrations WHERE filename = \\$1").
		WithArgs(migrationName).
		WillReturnError(sql.ErrNoRows)
	expectIndexSchema(mock, `"future_table"`, "public")
	expectStepIntentExistingPending(mock, migrationName, checksumText(statement), checksumText(strings.TrimSuffix(statement, ";")), 1, &qualifiedIndexName{schema: "public", name: "future_idx", targetRelationOID: testTargetRelationOID})
	expectExistingIndex(mock, "public", "future_idx", true, testTargetRelationOID)
	mock.ExpectExec(`DROP INDEX CONCURRENTLY IF EXISTS "public"\."future_idx"`).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec("CREATE INDEX CONCURRENTLY future_idx").
		WillReturnResult(sqlmock.NewResult(0, 0))
	expectIndexHealthy(mock, "public", "future_idx", true)
	expectStepIndexComplete(mock, migrationName, 1, &qualifiedIndexName{schema: "public", name: "future_idx", targetRelationOID: testTargetRelationOID})
	expectFinalizeNonTransactionalMigration(mock, migrationName)
	mock.ExpectExec("SELECT pg_advisory_unlock\\(\\$1\\)").
		WithArgs(migrationsAdvisoryLockID).
		WillReturnResult(sqlmock.NewResult(0, 1))

	err = applyMigrationsFS(context.Background(), db, fsys)
	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestApplyMigrationsFS_NonTransactionalMigration_DoesNotRecordUnhealthyIndex(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	prepareMigrationsBootstrapExpectations(mock)
	mock.ExpectQuery("SELECT checksum FROM schema_migrations WHERE filename = \\$1").
		WithArgs("001_unhealthy_notx.sql").
		WillReturnError(sql.ErrNoRows)
	expectIndexSchema(mock, `"audit"."events"`, "audit")
	expectStepIntentAbsent(mock, "001_unhealthy_notx.sql", 1, &qualifiedIndexName{schema: "audit", name: "quoted_idx", targetRelationOID: testTargetRelationOID})
	expectExistingIndex(mock, "audit", "quoted_idx", false, 0)
	expectStepIntentCreate(mock, "001_unhealthy_notx.sql", 1, &qualifiedIndexName{schema: "audit", name: "quoted_idx", targetRelationOID: testTargetRelationOID})
	mock.ExpectExec(`CREATE INDEX CONCURRENTLY "quoted_idx"`).
		WillReturnResult(sqlmock.NewResult(0, 0))
	expectIndexHealthy(mock, "audit", "quoted_idx", false)
	mock.ExpectExec("SELECT pg_advisory_unlock\\(\\$1\\)").
		WithArgs(migrationsAdvisoryLockID).
		WillReturnResult(sqlmock.NewResult(0, 1))

	fsys := fstest.MapFS{
		"001_unhealthy_notx.sql": &fstest.MapFile{
			Data: []byte(`CREATE INDEX CONCURRENTLY IF NOT EXISTS "quoted_idx" ON "audit"."events"(value);`),
		},
	}
	err = applyMigrationsFS(context.Background(), db, fsys)
	require.ErrorContains(t, err, "is not valid and ready")
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestApplyMigrationsFS_NonTransactionalMigration_RejectsIndexNameCollisionOnAnotherTable(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	prepareMigrationsBootstrapExpectations(mock)
	mock.ExpectQuery("SELECT checksum FROM schema_migrations WHERE filename = \\$1").
		WithArgs("001_collision_notx.sql").
		WillReturnError(sql.ErrNoRows)
	expectIndexSchema(mock, `"target_table"`, "public")
	expectStepIntentAbsent(mock, "001_collision_notx.sql", 1, &qualifiedIndexName{schema: "public", name: "shared_idx", targetRelationOID: testTargetRelationOID})
	expectExistingIndex(mock, "public", "shared_idx", true, 99)
	mock.ExpectExec("SELECT pg_advisory_unlock\\(\\$1\\)").
		WithArgs(migrationsAdvisoryLockID).
		WillReturnResult(sqlmock.NewResult(0, 1))

	fsys := fstest.MapFS{
		"001_collision_notx.sql": &fstest.MapFile{
			Data: []byte(`CREATE INDEX CONCURRENTLY IF NOT EXISTS shared_idx ON target_table(value);`),
		},
	}
	err = applyMigrationsFS(context.Background(), db, fsys)
	require.ErrorContains(t, err, "index name collision")
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestApplyMigrationsFS_NonTransactionalMigration_ReusesHealthyIndexOnlyWithMatchingIntent(t *testing.T) {
	const (
		migrationName = "999_crash_after_index_success_notx.sql"
		statement     = "CREATE UNIQUE INDEX CONCURRENTLY IF NOT EXISTS future_unique_idx ON future_table(value);"
	)
	index := &qualifiedIndexName{schema: "public", name: "future_unique_idx", targetRelationOID: testTargetRelationOID}
	fsys := fstest.MapFS{migrationName: &fstest.MapFile{Data: []byte(statement)}}

	t.Run("completed proof preserves the exact healthy unique index", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		defer func() { _ = db.Close() }()

		prepareMigrationsBootstrapExpectations(mock)
		mock.ExpectQuery("SELECT checksum FROM schema_migrations WHERE filename = \\$1").
			WithArgs(migrationName).
			WillReturnError(sql.ErrNoRows)
		expectIndexSchema(mock, `"future_table"`, "public")
		expectStepIntentExistingCompleted(mock, migrationName, checksumText(statement), checksumText(strings.TrimSuffix(statement, ";")), 1, index)
		expectIndexHealthy(mock, "public", "future_unique_idx", true)
		expectFinalizeNonTransactionalMigration(mock, migrationName)
		mock.ExpectExec("SELECT pg_advisory_unlock\\(\\$1\\)").
			WithArgs(migrationsAdvisoryLockID).
			WillReturnResult(sqlmock.NewResult(0, 1))

		err = applyMigrationsFS(context.Background(), db, fsys)
		require.NoError(t, err)
		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("healthy index without an earlier intent fails closed", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		defer func() { _ = db.Close() }()

		prepareMigrationsBootstrapExpectations(mock)
		mock.ExpectQuery("SELECT checksum FROM schema_migrations WHERE filename = \\$1").
			WithArgs(migrationName).
			WillReturnError(sql.ErrNoRows)
		expectIndexSchema(mock, `"future_table"`, "public")
		expectStepIntentAbsent(mock, migrationName, 1, index)
		expectIndexHealthy(mock, "public", "future_unique_idx", true)
		mock.ExpectExec("SELECT pg_advisory_unlock\\(\\$1\\)").
			WithArgs(migrationsAdvisoryLockID).
			WillReturnResult(sqlmock.NewResult(0, 1))

		err = applyMigrationsFS(context.Background(), db, fsys)
		require.ErrorContains(t, err, "without a matching migration recovery intent")
		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("pending intent preserves an ambiguous healthy unique index and fails closed", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		defer func() { _ = db.Close() }()

		prepareMigrationsBootstrapExpectations(mock)
		mock.ExpectQuery("SELECT checksum FROM schema_migrations WHERE filename = \\$1").
			WithArgs(migrationName).
			WillReturnError(sql.ErrNoRows)
		expectIndexSchema(mock, `"future_table"`, "public")
		expectStepIntentExistingPending(mock, migrationName, checksumText(statement), checksumText(strings.TrimSuffix(statement, ";")), 1, index)
		expectIndexHealthy(mock, "public", "future_unique_idx", true)
		mock.ExpectExec("SELECT pg_advisory_unlock\\(\\$1\\)").
			WithArgs(migrationsAdvisoryLockID).
			WillReturnResult(sqlmock.NewResult(0, 1))

		err = applyMigrationsFS(context.Background(), db, fsys)
		require.ErrorContains(t, err, "refusing to drop an unproven live index")
		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("pending intent without an index resumes with strict create", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		defer func() { _ = db.Close() }()

		prepareMigrationsBootstrapExpectations(mock)
		mock.ExpectQuery("SELECT checksum FROM schema_migrations WHERE filename = \\$1").
			WithArgs(migrationName).
			WillReturnError(sql.ErrNoRows)
		expectIndexSchema(mock, `"future_table"`, "public")
		expectStepIntentExistingPending(mock, migrationName, checksumText(statement), checksumText(strings.TrimSuffix(statement, ";")), 1, index)
		expectExistingIndex(mock, "public", "future_unique_idx", false, 0)
		mock.ExpectExec("CREATE UNIQUE INDEX CONCURRENTLY future_unique_idx").
			WillReturnResult(sqlmock.NewResult(0, 0))
		expectIndexHealthy(mock, "public", "future_unique_idx", true)
		expectStepIndexComplete(mock, migrationName, 1, index)
		expectFinalizeNonTransactionalMigration(mock, migrationName)
		mock.ExpectExec("SELECT pg_advisory_unlock\\(\\$1\\)").
			WithArgs(migrationsAdvisoryLockID).
			WillReturnResult(sqlmock.NewResult(0, 1))

		err = applyMigrationsFS(context.Background(), db, fsys)
		require.NoError(t, err)
		require.NoError(t, mock.ExpectationsWereMet())
	})

	for _, tc := range []struct {
		name       string
		indexOID   int64
		healthy    bool
		definition string
	}{
		{name: "different index OID", indexOID: testIndexOID + 1, healthy: true, definition: testIndexDefinition},
		{name: "different index definition", indexOID: testIndexOID, healthy: true, definition: testIndexDefinition + " changed"},
		{name: "index no longer valid ready or live", indexOID: testIndexOID, healthy: false, definition: testIndexDefinition},
	} {
		t.Run("completed proof with "+tc.name+" fails closed", func(t *testing.T) {
			db, mock, err := sqlmock.New()
			require.NoError(t, err)
			defer func() { _ = db.Close() }()

			prepareMigrationsBootstrapExpectations(mock)
			mock.ExpectQuery("SELECT checksum FROM schema_migrations WHERE filename = \\$1").
				WithArgs(migrationName).
				WillReturnError(sql.ErrNoRows)
			expectIndexSchema(mock, `"future_table"`, "public")
			expectStepIntentExistingCompleted(mock, migrationName, checksumText(statement), checksumText(strings.TrimSuffix(statement, ";")), 1, index)
			expectIndexState(mock, "public", "future_unique_idx", tc.indexOID, testTargetRelationOID, tc.healthy, tc.definition)
			mock.ExpectExec("SELECT pg_advisory_unlock\\(\\$1\\)").
				WithArgs(migrationsAdvisoryLockID).
				WillReturnResult(sqlmock.NewResult(0, 1))

			err = applyMigrationsFS(context.Background(), db, fsys)
			require.ErrorContains(t, err, "completed index proof")
			require.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

func TestLoadNonTransactionalStepIntent_FailsClosedOnCorruptJournal(t *testing.T) {
	expected := nonTransactionalStepIntent{
		filename:          "999_corrupt_notx.sql",
		migrationChecksum: "migration-checksum",
		statementIndex:    1,
		statementChecksum: "statement-checksum",
		index:             &qualifiedIndexName{schema: "public", name: "corrupt_idx", targetRelationOID: testTargetRelationOID},
	}

	for _, tc := range []struct {
		name              string
		migrationChecksum string
		statementChecksum string
		indexSchema       string
		createdIndexOID   any
		createdIndexDef   any
		expectedError     string
	}{
		{
			name: "immutable metadata mismatch", migrationChecksum: "wrong",
			statementChecksum: expected.statementChecksum, indexSchema: "public",
			expectedError: "does not match the current immutable statement",
		},
		{
			name: "half populated proof", migrationChecksum: expected.migrationChecksum,
			statementChecksum: expected.statementChecksum, indexSchema: "public",
			createdIndexOID: testIndexOID, expectedError: "index proof is incomplete",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db, mock, err := sqlmock.New()
			require.NoError(t, err)
			defer func() { _ = db.Close() }()

			mock.ExpectQuery("SELECT migration_checksum, statement_checksum, index_schema, index_name, target_relation_oid,").
				WithArgs(expected.filename, expected.statementIndex).
				WillReturnRows(sqlmock.NewRows([]string{
					"migration_checksum", "statement_checksum", "index_schema", "index_name", "target_relation_oid",
					"created_index_oid", "created_index_definition",
				}).AddRow(tc.migrationChecksum, tc.statementChecksum, tc.indexSchema, expected.index.name,
					expected.index.targetRelationOID, tc.createdIndexOID, tc.createdIndexDef))

			_, err = loadNonTransactionalStepIntent(context.Background(), db, expected)
			require.ErrorContains(t, err, tc.expectedError)
			require.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

func TestApplyMigrationsFS_NonTransactionalMigration_ProofPersistenceMustAffectIntent(t *testing.T) {
	const (
		migrationName = "999_proof_failure_notx.sql"
		statement     = "CREATE INDEX CONCURRENTLY IF NOT EXISTS proof_idx ON proof_table(value);"
	)
	index := &qualifiedIndexName{schema: "public", name: "proof_idx", targetRelationOID: testTargetRelationOID}
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	prepareMigrationsBootstrapExpectations(mock)
	mock.ExpectQuery("SELECT checksum FROM schema_migrations WHERE filename = \\$1").
		WithArgs(migrationName).
		WillReturnError(sql.ErrNoRows)
	expectIndexSchema(mock, `"proof_table"`, "public")
	expectStepIntentAbsent(mock, migrationName, 1, index)
	expectExistingIndex(mock, "public", "proof_idx", false, 0)
	expectStepIntentCreate(mock, migrationName, 1, index)
	mock.ExpectExec("CREATE INDEX CONCURRENTLY proof_idx").
		WillReturnResult(sqlmock.NewResult(0, 0))
	expectIndexHealthy(mock, "public", "proof_idx", true)
	mock.ExpectExec("UPDATE schema_migration_steps").
		WithArgs(migrationName, sqlmock.AnyArg(), 1, sqlmock.AnyArg(), "public", "proof_idx", testTargetRelationOID, testIndexOID, testIndexDefinition).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec("SELECT pg_advisory_unlock\\(\\$1\\)").
		WithArgs(migrationsAdvisoryLockID).
		WillReturnResult(sqlmock.NewResult(0, 1))

	err = applyMigrationsFS(context.Background(), db, fstest.MapFS{
		migrationName: &fstest.MapFile{Data: []byte(statement)},
	})
	require.ErrorContains(t, err, "affected 0 rows instead of 1")
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestApplyMigrationsFS_TransactionalMigration(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()
	// The advisory lock and all migration work must share one session. This also
	// proves startup cannot self-deadlock when deployments cap the pool at one.
	db.SetMaxOpenConns(1)

	prepareMigrationsBootstrapExpectations(mock)
	mock.ExpectQuery("SELECT checksum FROM schema_migrations WHERE filename = \\$1").
		WithArgs("001_add_col.sql").
		WillReturnError(sql.ErrNoRows)
	mock.ExpectBegin()
	mock.ExpectExec("ALTER TABLE t ADD COLUMN name TEXT").
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec("INSERT INTO schema_migrations \\(filename, checksum\\) VALUES \\(\\$1, \\$2\\)").
		WithArgs("001_add_col.sql", sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()
	mock.ExpectExec("SELECT pg_advisory_unlock\\(\\$1\\)").
		WithArgs(migrationsAdvisoryLockID).
		WillReturnResult(sqlmock.NewResult(0, 1))

	fsys := fstest.MapFS{
		"001_add_col.sql": &fstest.MapFile{
			Data: []byte("ALTER TABLE t ADD COLUMN name TEXT;"),
		},
	}

	err = applyMigrationsFS(context.Background(), db, fsys)
	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}

func prepareMigrationsBootstrapExpectations(mock sqlmock.Sqlmock) {
	mock.ExpectQuery("SELECT pg_try_advisory_lock\\(\\$1\\)").
		WithArgs(migrationsAdvisoryLockID).
		WillReturnRows(sqlmock.NewRows([]string{"pg_try_advisory_lock"}).AddRow(true))
	mock.ExpectExec("CREATE TABLE IF NOT EXISTS schema_migrations").
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec("CREATE TABLE IF NOT EXISTS schema_migration_steps").
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec("ALTER TABLE schema_migration_steps").
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectQuery("SELECT EXISTS \\(").
		WithArgs("schema_migrations").
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))
	mock.ExpectQuery("SELECT EXISTS \\(").
		WithArgs("atlas_schema_revisions").
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))
	mock.ExpectQuery("SELECT COUNT\\(\\*\\) FROM atlas_schema_revisions").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
}

func expectExistingIndex(mock sqlmock.Sqlmock, schema, name string, exists bool, targetRelationOID int64) {
	expectation := mock.ExpectQuery("SELECT idx.oid::bigint").WithArgs(schema, name)
	rows := sqlmock.NewRows([]string{"index_oid", "indrelid", "healthy", "definition"})
	if exists {
		rows.AddRow(testIndexOID, targetRelationOID, false, testIndexDefinition)
	}
	expectation.WillReturnRows(rows)
}

func expectIndexSchema(mock sqlmock.Sqlmock, tableReference, schema string) {
	mock.ExpectQuery("SELECT ns.nspname").
		WithArgs(tableReference).
		WillReturnRows(sqlmock.NewRows([]string{"nspname", "oid"}).AddRow(schema, testTargetRelationOID))
}

func expectIndexHealthy(mock sqlmock.Sqlmock, schema, name string, healthy bool) {
	expectIndexState(mock, schema, name, testIndexOID, testTargetRelationOID, healthy, testIndexDefinition)
}

func expectIndexState(mock sqlmock.Sqlmock, schema, name string, indexOID, targetRelationOID int64, healthy bool, definition string) {
	mock.ExpectQuery("SELECT idx.oid::bigint").
		WithArgs(schema, name).
		WillReturnRows(sqlmock.NewRows([]string{"index_oid", "indrelid", "healthy", "definition"}).
			AddRow(indexOID, targetRelationOID, healthy, definition))
}

func expectStepIntentAbsent(mock sqlmock.Sqlmock, filename string, statementIndex int, index *qualifiedIndexName) {
	mock.ExpectQuery("SELECT migration_checksum, statement_checksum, index_schema, index_name, target_relation_oid,").
		WithArgs(filename, statementIndex).
		WillReturnError(sql.ErrNoRows)
}

func expectStepIntentCreate(mock sqlmock.Sqlmock, filename string, statementIndex int, index *qualifiedIndexName) {
	var schema, name, oid any
	if index != nil {
		schema = index.schema
		name = index.name
		oid = index.targetRelationOID
	}
	mock.ExpectExec("INSERT INTO schema_migration_steps").
		WithArgs(filename, sqlmock.AnyArg(), statementIndex, sqlmock.AnyArg(), schema, name, oid).
		WillReturnResult(sqlmock.NewResult(0, 1))
}

func expectStepIntentExistingPending(mock sqlmock.Sqlmock, filename, migrationChecksum, statementChecksum string, statementIndex int, index *qualifiedIndexName) {
	var schema, name, oid any
	if index != nil {
		schema = index.schema
		name = index.name
		oid = index.targetRelationOID
	}
	mock.ExpectQuery("SELECT migration_checksum, statement_checksum, index_schema, index_name, target_relation_oid,").
		WithArgs(filename, statementIndex).
		WillReturnRows(sqlmock.NewRows([]string{
			"migration_checksum", "statement_checksum", "index_schema", "index_name", "target_relation_oid",
			"created_index_oid", "created_index_definition",
		}).AddRow(migrationChecksum, statementChecksum, schema, name, oid, nil, nil))
}

func expectStepIntentExistingCompleted(mock sqlmock.Sqlmock, filename, migrationChecksum, statementChecksum string, statementIndex int, index *qualifiedIndexName) {
	var schema, name, oid any
	if index != nil {
		schema = index.schema
		name = index.name
		oid = index.targetRelationOID
	}
	mock.ExpectQuery("SELECT migration_checksum, statement_checksum, index_schema, index_name, target_relation_oid,").
		WithArgs(filename, statementIndex).
		WillReturnRows(sqlmock.NewRows([]string{
			"migration_checksum", "statement_checksum", "index_schema", "index_name", "target_relation_oid",
			"created_index_oid", "created_index_definition",
		}).AddRow(migrationChecksum, statementChecksum, schema, name, oid, testIndexOID, testIndexDefinition))
}

func expectStepIndexComplete(mock sqlmock.Sqlmock, filename string, statementIndex int, index *qualifiedIndexName) {
	mock.ExpectExec("UPDATE schema_migration_steps").
		WithArgs(filename, sqlmock.AnyArg(), statementIndex, sqlmock.AnyArg(), index.schema, index.name, index.targetRelationOID, testIndexOID, testIndexDefinition).
		WillReturnResult(sqlmock.NewResult(0, 1))
}

func expectFinalizeNonTransactionalMigration(mock sqlmock.Sqlmock, filename string) {
	mock.ExpectBegin()
	mock.ExpectExec("INSERT INTO schema_migrations \\(filename, checksum\\) VALUES \\(\\$1, \\$2\\)").
		WithArgs(filename, sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectExec("DELETE FROM schema_migration_steps WHERE filename = \\$1").
		WithArgs(filename).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
}
