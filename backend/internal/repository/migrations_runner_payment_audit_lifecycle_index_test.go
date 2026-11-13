package repository

import (
	"context"
	"database/sql"
	"testing"
	"testing/fstest"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/require"
)

// 并发建唯一索引失败会留下 INVALID 索引，重跑时 IF NOT EXISTS 会跳过重建，
// 而此时旧的全量唯一索引已经被删除，返利 claim 会直接报 “no unique or exclusion
// constraint matching”。这里锁定“先清理 INVALID 索引再重建”的执行顺序。
func TestApplyMigrationsFS_NonTransactionalMigration_PaymentAuditLifecycleIndexDropsInvalidIndexBeforeRetry(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	prepareMigrationsBootstrapExpectations(mock)
	mock.ExpectQuery("SELECT checksum FROM schema_migrations WHERE filename = \\$1").
		WithArgs(paymentAuditLifecycleUniqueIndexMigration).
		WillReturnError(sql.ErrNoRows)
	mock.ExpectQuery("SELECT EXISTS \\(").
		WithArgs(paymentAuditLifecycleUniqueIndex).
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))
	mock.ExpectExec("DROP INDEX CONCURRENTLY IF EXISTS idx_payment_audit_logs_order_action_once_uniq").
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec("CREATE UNIQUE INDEX CONCURRENTLY IF NOT EXISTS idx_payment_audit_logs_order_action_once_uniq").
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec("DROP INDEX CONCURRENTLY IF EXISTS idx_payment_audit_logs_order_action_uniq").
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec("INSERT INTO schema_migrations \\(filename, checksum\\) VALUES \\(\\$1, \\$2\\)").
		WithArgs(paymentAuditLifecycleUniqueIndexMigration, sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectExec("SELECT pg_advisory_unlock\\(\\$1\\)").
		WithArgs(migrationsAdvisoryLockID).
		WillReturnResult(sqlmock.NewResult(0, 1))

	fsys := fstest.MapFS{
		paymentAuditLifecycleUniqueIndexMigration: &fstest.MapFile{
			Data: []byte(`
CREATE UNIQUE INDEX CONCURRENTLY IF NOT EXISTS idx_payment_audit_logs_order_action_once_uniq
    ON payment_audit_logs (order_id, action)
    WHERE action IN ('SUBSCRIPTION_ASSIGNED', 'AFFILIATE_REBATE_APPLIED', 'AFFILIATE_REBATE_SKIPPED');

DROP INDEX CONCURRENTLY IF EXISTS idx_payment_audit_logs_order_action_uniq;
`),
		},
	}

	err = applyMigrationsFS(context.Background(), db, fsys)
	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}
