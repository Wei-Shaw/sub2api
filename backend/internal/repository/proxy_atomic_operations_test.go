package repository

import (
	"context"
	"errors"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestProxyGroupCreateWithMembersRollsBackAsOneUnit(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	repo := newProxyGroupRepositoryWithSQL(nil, db)
	now := time.Now()

	mock.ExpectBegin()
	mock.ExpectQuery(`(?s)INSERT INTO proxy_groups`).
		WithArgs("pool", nil, service.ProxyGroupStrategyRoundRobin, false, service.StatusActive, nil, nil).
		WillReturnRows(sqlmock.NewRows([]string{"id", "created_at", "updated_at"}).AddRow(int64(7), now, now))
	mock.ExpectExec(regexp.QuoteMeta("UPDATE proxies SET group_id=NULL, updated_at=NOW()")).
		WithArgs(int64(7)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(regexp.QuoteMeta("UPDATE proxies SET group_id=$1, updated_at=NOW()")).
		WithArgs(int64(7), sqlmock.AnyArg()).
		WillReturnError(errors.New("membership write failed"))
	mock.ExpectRollback()

	group := &service.ProxyGroup{Name: "pool", Strategy: service.ProxyGroupStrategyRoundRobin, Status: service.StatusActive}
	err = repo.CreateWithMembers(context.Background(), group, []int64{11, 12})
	require.ErrorContains(t, err, "membership write failed")
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestProxyGroupDeleteIfUnusedChecksAndDeletesInTransaction(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	repo := newProxyGroupRepositoryWithSQL(nil, db)

	mock.ExpectBegin()
	mock.ExpectExec(`LOCK TABLE accounts, proxies`).WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectQuery(`SELECT EXISTS`).WithArgs(int64(9)).
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))
	mock.ExpectQuery(`(?s)SELECT EXISTS.*FROM proxies.*UNION ALL.*FROM accounts`).WithArgs(int64(9)).
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(false))
	mock.ExpectExec(`UPDATE proxy_groups SET deleted_at=NOW\(\), updated_at=NOW\(\)`).
		WithArgs(int64(9)).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	require.NoError(t, repo.DeleteIfUnused(context.Background(), 9))
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestDeleteWithAccountBindingsRollsBackOnReferenceFailure(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	repo := newProxyRepositoryWithSQL(nil, db)

	mock.ExpectBegin()
	mock.ExpectQuery(`(?s)UPDATE accounts.*SET proxy_id = NULL.*RETURNING id`).WithArgs(int64(5)).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(int64(11)).AddRow(int64(12)))
	mock.ExpectQuery(`(?s)UPDATE accounts.*SET proxy_fallback_origin_id = NULL.*RETURNING id`).WithArgs(int64(5)).
		WillReturnError(errors.New("fallback cleanup failed"))
	mock.ExpectRollback()

	_, err = repo.DeleteWithAccountBindings(context.Background(), 5)
	require.ErrorContains(t, err, "fallback cleanup failed")
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestDeleteWithAccountBindingsCommitsInvalidationWithDelete(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	repo := newProxyRepositoryWithSQL(nil, db)

	mock.ExpectBegin()
	mock.ExpectQuery(`(?s)UPDATE accounts.*SET proxy_id = NULL.*RETURNING id`).WithArgs(int64(5)).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(int64(12)).AddRow(int64(11)))
	mock.ExpectQuery(`(?s)UPDATE accounts.*SET proxy_fallback_origin_id = NULL.*RETURNING id`).WithArgs(int64(5)).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(int64(13)))
	mock.ExpectExec(`(?s)UPDATE proxies.*SET backup_proxy_id = NULL`).WithArgs(int64(5)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`(?s)UPDATE proxies.*SET deleted_at = NOW\(\)`).WithArgs(int64(5)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(regexp.QuoteMeta("INSERT INTO scheduler_outbox (event_type, account_id, group_id, payload)")).
		WithArgs(service.SchedulerOutboxEventAccountBulkChanged, nil, nil, accountIDsPayloadMatcher{want: []int64{11, 12, 13}}).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()

	unbound, err := repo.DeleteWithAccountBindings(context.Background(), 5)
	require.NoError(t, err)
	require.Equal(t, int64(2), unbound)
	require.NoError(t, mock.ExpectationsWereMet())
}
