//go:build unit

package repository

import (
	"context"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	dbent "github.com/Wei-Shaw/sub2api/ent"
	_ "github.com/Wei-Shaw/sub2api/ent/runtime"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
)

// 清空账号全部分组时也必须写入 account_groups_changed outbox 事件，
// payload 带旧分组，否则旧分组的调度快照会继续包含该账号。
func TestD02BindGroupsEmptyListEnqueuesGroupsChangedOutbox(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	client := dbent.NewClient(dbent.Driver(entsql.OpenDB(dialect.Postgres, db)))
	t.Cleanup(func() { _ = client.Close() })

	mock.ExpectQuery(`FROM "account_groups"`).
		WillReturnRows(sqlmock.NewRows([]string{"account_id", "group_id", "priority", "created_at"}).
			AddRow(int64(7), int64(3), 1, time.Now()))
	mock.ExpectBegin()
	mock.ExpectExec(`DELETE FROM "account_groups"`).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	mock.ExpectExec(regexp.QuoteMeta("INSERT INTO scheduler_outbox")).
		WithArgs(service.SchedulerOutboxEventAccountGroupsChanged, int64(7), nil, []byte(`{"group_ids":[3]}`)).
		WillReturnResult(sqlmock.NewResult(1, 1))

	repo := newAccountRepositoryWithSQL(client, db, nil)
	require.NoError(t, repo.BindGroups(context.Background(), 7, []int64{}))
	require.NoError(t, mock.ExpectationsWereMet())
}
