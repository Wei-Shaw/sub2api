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
	"github.com/Wei-Shaw/sub2api/internal/pkg/adminaccess"
	"github.com/stretchr/testify/require"
)

type scopedRecoveryReviewRepo struct {
	accountRepoStubForClearAccountError
	reads    int
	failRead bool
}

func (r *scopedRecoveryReviewRepo) GetByID(ctx context.Context, id int64) (*Account, error) {
	r.reads++
	if r.failRead && r.reads > 1 {
		return nil, errors.New("模拟回读失败")
	}
	return r.accountRepoStubForClearAccountError.GetByID(ctx, id)
}
func (*scopedRecoveryReviewRepo) ListShadowsByParent(context.Context, int64) ([]*Account, error) {
	return nil, nil
}

type scopedRecoveryReviewBlocker struct {
	runtimeBlockRecorder
	resets int
}

func (b *scopedRecoveryReviewBlocker) ResetOpenAI403Counter(context.Context, int64) { b.resets++ }

func TestScopedAdminReviewRecoveryCommitBoundary(t *testing.T) {
	for _, mode := range []string{"commit", "read_failure", "commit_failure"} {
		t.Run(mode, func(t *testing.T) {
			db, mock, err := sqlmock.New()
			require.NoError(t, err)
			defer db.Close()
			client := dbent.NewClient(dbent.Driver(entsql.OpenDB(dialect.Postgres, db)))
			repo := &scopedRecoveryReviewRepo{accountRepoStubForClearAccountError: accountRepoStubForClearAccountError{account: &Account{ID: 31, Platform: "openai", Type: "oauth", Status: StatusError, GroupIDs: []int64{1}}}, failRead: mode == "read_failure"}
			blocker := &scopedRecoveryReviewBlocker{}
			admin := &adminServiceImpl{accountRepo: repo, entClient: client, runtimeBlocker: blocker}
			mock.ExpectBegin()
			mock.ExpectExec(`SELECT pg_advisory_xact_lock`).WithArgs(int64(31)).WillReturnResult(sqlmock.NewResult(0, 1))
			mock.ExpectExec(`SELECT id FROM accounts`).WithArgs(int64(31)).WillReturnResult(sqlmock.NewResult(0, 1))
			mock.ExpectExec(`SELECT account_id FROM account_groups`).WithArgs(int64(31)).WillReturnResult(sqlmock.NewResult(0, 1))
			if mode == "read_failure" {
				mock.ExpectRollback()
			} else if mode == "commit_failure" {
				mock.ExpectCommit().WillReturnError(errors.New("模拟提交失败"))
			} else {
				mock.ExpectCommit()
			}
			ctx := adminaccess.WithPrincipal(context.Background(), &adminaccess.Principal{Policy: adminaccess.Policy{Rules: []adminaccess.Rule{{Permissions: []string{"accounts.recover"}, GroupIDs: []int64{1}, AccountTypes: []string{"oauth"}}}}})
			_, err = admin.ScopedAdminWrite(ctx, 31, "accounts.recover", nil, nil)
			if mode == "commit" {
				require.NoError(t, err)
				require.Equal(t, []int64{31}, blocker.clearedIDs)
				require.Zero(t, blocker.resets)
			} else {
				require.Error(t, err)
				require.Empty(t, blocker.clearedIDs, "失败事务不得清理运行态阻塞")
				require.Zero(t, blocker.resets)
			}
			require.NoError(t, mock.ExpectationsWereMet())
		})
	}
}
