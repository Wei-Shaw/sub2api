//go:build unit

package repository

import (
	"context"
	"regexp"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	dbent "github.com/Wei-Shaw/sub2api/ent"
	_ "github.com/Wei-Shaw/sub2api/ent/runtime"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
)

// SetError 会同时写 status=error 与 schedulable=false；ClearError 必须只对
// 仍处于 error 状态的账号恢复 schedulable=true，手动暂停的 active 账号保持暂停。
func TestD03ClearErrorRestoresSchedulableOnlyForErrorAccounts(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	client := dbent.NewClient(dbent.Driver(entsql.OpenDB(dialect.Postgres, db)))
	t.Cleanup(func() { _ = client.Close() })

	// 1) 仅 status=error 的行恢复 schedulable=true
	mock.ExpectExec(`(?s)^UPDATE "accounts" SET .*"schedulable" = \$\d+ WHERE .*"accounts"\."id" = \$\d+ AND .*"accounts"\."status" = \$\d+`).
		WithArgs(sqlmock.AnyArg(), true, int64(42), service.StatusError).
		WillReturnResult(sqlmock.NewResult(0, 1))
	// 2) 原有的 status/error_message 重置，不触碰 schedulable
	mock.ExpectExec(`(?s)^UPDATE "accounts" SET "updated_at" = \$1, "status" = \$2, "error_message" = \$3 WHERE`).
		WithArgs(sqlmock.AnyArg(), service.StatusActive, "", int64(42)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(regexp.QuoteMeta("INSERT INTO scheduler_outbox")).
		WillReturnResult(sqlmock.NewResult(1, 1))

	repo := newAccountRepositoryWithSQL(client, db, nil)
	require.NoError(t, repo.ClearError(context.Background(), 42))
	require.NoError(t, mock.ExpectationsWereMet())
}
