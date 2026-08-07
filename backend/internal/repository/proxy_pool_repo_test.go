package repository

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/lib/pq"
	"github.com/stretchr/testify/require"
)

func TestProxyPoolRepositoryPersistsGrokQualityHealth(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	checkedAt := time.Now().UTC()
	httpStatus := 401
	mock.ExpectExec("UPDATE proxies[\\s\\S]+pool_grok_quality_status").
		WithArgs(
			int64(9), int64(7), service.ProxyPoolHealthHealthy, 0, checkedAt,
			"pass", checkedAt, httpStatus, "HTTP 401 (target reachable)",
		).
		WillReturnResult(sqlmock.NewResult(0, 1))

	repo := NewProxyPoolRepository(db)
	err = repo.UpdateProxyPoolHealth(context.Background(), 7, 9, service.ProxyPoolHealthSnapshot{
		Health:                service.ProxyPoolHealthHealthy,
		CheckedAt:             checkedAt,
		GrokQualityStatus:     "pass",
		GrokQualityCheckedAt:  &checkedAt,
		GrokQualityHTTPStatus: &httpStatus,
		GrokQualityMessage:    "HTTP 401 (target reachable)",
	})

	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestProxyPoolRepositoryCreateTranslatesDuplicateName(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	mock.ExpectQuery("INSERT INTO proxy_pools").
		WillReturnError(&pq.Error{Code: "23505"})

	repo := NewProxyPoolRepository(db)
	_, err = repo.CreatePool(context.Background(), &service.ProxyPool{
		Name:                  "primary",
		Status:                service.ProxyPoolStatusActive,
		HealthIntervalSeconds: 300,
		FailureThreshold:      2,
		AutoRebind:            true,
	})

	require.ErrorIs(t, err, service.ErrProxyPoolNameExists)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestProxyPoolRepositoryUpdateTranslatesDuplicateName(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	mock.ExpectExec("UPDATE proxy_pools").
		WillReturnError(&pq.Error{Code: "23505"})

	repo := NewProxyPoolRepository(db)
	err = repo.UpdatePool(context.Background(), &service.ProxyPool{
		ID:                    1,
		Name:                  "primary",
		Status:                service.ProxyPoolStatusActive,
		HealthIntervalSeconds: 300,
		FailureThreshold:      2,
		AutoRebind:            true,
	})

	require.True(t, errors.Is(err, service.ErrProxyPoolNameExists))
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestProxyPoolRepositoryDeleteRefreshesOnlyAffectedAccounts(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	mock.ExpectBegin()
	mock.ExpectQuery("UPDATE accounts").
		WithArgs(int64(7)).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(int64(11)).AddRow(int64(12)))
	mock.ExpectExec("UPDATE proxies").
		WithArgs(int64(7)).
		WillReturnResult(sqlmock.NewResult(0, 2))
	mock.ExpectExec("UPDATE proxy_pools SET deleted_at").
		WithArgs(int64(7)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("INSERT INTO scheduler_outbox").
		WithArgs(service.SchedulerOutboxEventAccountBulkChanged, nil, nil, []byte(`{"account_ids":[11,12]}`)).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()

	repo := NewProxyPoolRepository(db)
	require.NoError(t, repo.DeletePool(context.Background(), 7))
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestProxyPoolRepositoryUnbindRefreshesOnlyAffectedAccounts(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	mock.ExpectBegin()
	mock.ExpectQuery("UPDATE accounts").
		WithArgs(int64(7), sqlmock.AnyArg()).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(int64(11)))
	mock.ExpectExec("INSERT INTO scheduler_outbox").
		WithArgs(service.SchedulerOutboxEventAccountBulkChanged, nil, nil, []byte(`{"account_ids":[11]}`)).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()

	repo := NewProxyPoolRepository(db)
	affected, err := repo.UnbindAccountsFromPool(context.Background(), 7, []int64{11, 99})
	require.NoError(t, err)
	require.Equal(t, int64(1), affected)
	require.NoError(t, mock.ExpectationsWereMet())
}
