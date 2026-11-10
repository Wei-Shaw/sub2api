//go:build unit

package service

import (
	"context"
	"errors"
	"testing"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	"github.com/DATA-DOG/go-sqlmock"
	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/stretchr/testify/require"
)

func TestScopedAdminAfterCommitEffects(t *testing.T) {
	for _, mode := range []string{"commit", "rollback", "commit_failure"} {
		t.Run(mode, func(t *testing.T) {
			db, mock, err := sqlmock.New()
			require.NoError(t, err)
			defer db.Close()
			client := dbent.NewClient(dbent.Driver(entsql.OpenDB(dialect.Postgres, db)))
			mock.ExpectBegin()
			tx, err := client.Tx(context.Background())
			require.NoError(t, err)
			calls := 0
			AfterAccountCommit(dbent.NewTxContext(context.Background(), tx), func(ctx context.Context) {
				require.Nil(t, dbent.TxFromContext(ctx), "提交后回调不能复用已关闭的事务")
				calls++
			})
			require.Zero(t, calls)
			switch mode {
			case "commit":
				mock.ExpectCommit()
				require.NoError(t, tx.Commit())
				require.Equal(t, 1, calls)
			case "rollback":
				mock.ExpectRollback()
				require.NoError(t, tx.Rollback())
				require.Zero(t, calls)
			case "commit_failure":
				mock.ExpectCommit().WillReturnError(errors.New("提交失败"))
				require.Error(t, tx.Commit())
				require.Zero(t, calls)
			}
			require.NoError(t, mock.ExpectationsWereMet())
		})
	}
	calls := 0
	AfterAccountCommit(context.Background(), func(context.Context) { calls++ })
	require.Equal(t, 1, calls, "无事务时保留原管理员同步语义")
}
