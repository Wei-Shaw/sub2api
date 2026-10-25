package repository

import (
	"context"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
)

func newRedeemAdjustmentRepoMock(t *testing.T) (*userRepository, sqlmock.Sqlmock) {
	t.Helper()
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	driver := entsql.OpenDB(dialect.Postgres, db)
	client := dbent.NewClient(dbent.Driver(driver))
	t.Cleanup(func() { _ = client.Close() })
	return newUserRepositoryWithSQL(client, db), mock
}

// redeemAdjustmentSQLRegex 返回 ApplyRedeem*Adjustment 的期望 SQL 正则：
// 进程级 IsMySQL() 为真时仓库发 MySQL 方言语句（? / NOW(3)），否则发 PG 语句。
func redeemAdjustmentSQLRegex(column string) string {
	if IsMySQL() {
		return `UPDATE users SET ` + column + ` = GREATEST\(` + column + ` \+ \?, 0\), updated_at = NOW\(3\) WHERE id = \? AND deleted_at IS NULL`
	}
	return `UPDATE users SET ` + column + ` = GREATEST\(` + column + ` \+ \$1, 0\), updated_at = NOW\(\) WHERE id = \$2 AND deleted_at IS NULL`
}

func TestApplyRedeemBalanceAdjustment_UsesAtomicFloor(t *testing.T) {
	repo, mock := newRedeemAdjustmentRepoMock(t)
	mock.ExpectExec(redeemAdjustmentSQLRegex("balance")).
		WithArgs(-7.0, int64(42)).
		WillReturnResult(sqlmock.NewResult(0, 1))

	require.NoError(t, repo.ApplyRedeemBalanceAdjustment(context.Background(), 42, -7))
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestApplyRedeemConcurrencyAdjustment_UsesAtomicFloor(t *testing.T) {
	repo, mock := newRedeemAdjustmentRepoMock(t)
	mock.ExpectExec(redeemAdjustmentSQLRegex("concurrency")).
		WithArgs(-7, int64(42)).
		WillReturnResult(sqlmock.NewResult(0, 1))

	require.NoError(t, repo.ApplyRedeemConcurrencyAdjustment(context.Background(), 42, -7))
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestApplyRedeemAdjustment_MissingUser(t *testing.T) {
	repo, mock := newRedeemAdjustmentRepoMock(t)
	mock.ExpectExec(redeemAdjustmentSQLRegex("balance")).
		WithArgs(-1.0, int64(404)).
		WillReturnResult(sqlmock.NewResult(0, 0))

	err := repo.ApplyRedeemBalanceAdjustment(context.Background(), 404, -1)
	require.ErrorIs(t, err, service.ErrUserNotFound)
	require.NoError(t, mock.ExpectationsWereMet())
}
