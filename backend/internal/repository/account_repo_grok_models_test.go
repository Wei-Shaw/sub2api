//go:build unit

package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"regexp"
	"testing"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	"github.com/DATA-DOG/go-sqlmock"
)

const grokCatalogCASQuery = `UPDATE accounts
SET extra = jsonb_set(COALESCE(extra, '{}'::jsonb), '{grok_observed_models}', $3::jsonb, true),
    updated_at = NOW()
WHERE id = $1 AND deleted_at IS NULL AND platform = 'grok'
  AND COALESCE(extra -> 'grok_observed_models', 'null'::jsonb) = $2::jsonb`

func TestGrokCatalogCASWritesSingleKeyAndReportsRows(t *testing.T) {
	t.Parallel()

	nextJSON := jsonRaw(`{"fetched_at":"2026-10-25T02:00:01Z","models":["grok-4.7"]}`)
	cases := []struct {
		name      string
		affected  int64
		execErr   error
		resultErr error
		want      bool
		wantErr   string
	}{
		{name: "applied", affected: 1, want: true},
		{name: "conflict", affected: 0, want: false},
		{name: "db error", execErr: errors.New("db down"), wantErr: "db down"},
		{name: "rows affected error", affected: 1, resultErr: errors.New("rows failed"), wantErr: "rows failed"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			var queries []string
			repo, mock := newGrokCatalogCASRepo(t, &queries)
			expectation := mock.ExpectExec(regexp.QuoteMeta(grokCatalogCASQuery)).
				WithArgs(int64(25), "null", string(nextJSON))
			if tc.execErr != nil {
				expectation.WillReturnError(tc.execErr)
			} else if tc.resultErr != nil {
				expectation.WillReturnResult(sqlmock.NewErrorResult(tc.resultErr))
			} else {
				expectation.WillReturnResult(sqlmock.NewResult(0, tc.affected))
			}

			applied, err := repo.CompareAndSwapGrokObservedModels(context.Background(), 25, nil, nextJSON)

			if tc.wantErr != "" {
				require.EqualError(t, err, tc.wantErr)
				require.False(t, applied)
			} else {
				require.NoError(t, err)
				require.Equal(t, tc.want, applied)
			}
			require.NoError(t, mock.ExpectationsWereMet())
			require.Len(t, queries, 1)
			require.NotContains(t, queries[0], "credentials")
			require.NotContains(t, queries[0], "rate_multiplier")
			require.NotContains(t, queries[0], "status")
			require.NotContains(t, queries[0], "scheduler_outbox")
			require.NotContains(t, queries[0], "grok-4.7")
			require.NotContains(t, queries[0], "unrelated")
		})
	}
}

func TestGrokCatalogCASNormalizesEmptyJSONAndRejectsInvalidJSON(t *testing.T) {
	t.Parallel()

	t.Run("empty values are null", func(t *testing.T) {
		t.Parallel()
		repo, mock := newGrokCatalogCASRepo(t, nil)
		mock.ExpectExec(regexp.QuoteMeta(grokCatalogCASQuery)).
			WithArgs(int64(25), "null", "null").
			WillReturnResult(sqlmock.NewResult(0, 1))

		applied, err := repo.CompareAndSwapGrokObservedModels(context.Background(), 25, jsonRaw(" \n\t"), nil)

		require.NoError(t, err)
		require.True(t, applied)
		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("invalid json does not execute", func(t *testing.T) {
		t.Parallel()
		repo, mock := newGrokCatalogCASRepo(t, nil)

		applied, err := repo.CompareAndSwapGrokObservedModels(context.Background(), 25, jsonRaw("{"), jsonRaw(`{"models":["grok-4.7"]}`))

		require.ErrorIs(t, err, errGrokCatalogInvalidJSON)
		require.False(t, applied)
		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("existing json keeps its bytes", func(t *testing.T) {
		t.Parallel()
		expected := jsonRaw(`{"models":["grok-4.5"],"fetched_at":"2026-10-25T02:00:01Z"}`)
		nextJSON := jsonRaw(`{"fetched_at":"2026-10-25T03:00:01Z","models":["grok-4.7"]}`)
		repo, mock := newGrokCatalogCASRepo(t, nil)
		mock.ExpectExec(regexp.QuoteMeta(grokCatalogCASQuery)).
			WithArgs(int64(25), string(expected), string(nextJSON)).
			WillReturnResult(sqlmock.NewResult(0, 0))

		applied, err := repo.CompareAndSwapGrokObservedModels(context.Background(), 25, expected, nextJSON)

		require.NoError(t, err)
		require.False(t, applied)
		require.NoError(t, mock.ExpectationsWereMet())
	})
}

func TestGrokCatalogCASUsesTransactionClientAndSkipsCache(t *testing.T) {
	t.Parallel()

	nextJSON := jsonRaw(`{"models":["grok-4.7"]}`)
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	client := dbent.NewClient(dbent.Driver(entsql.OpenDB(dialect.Postgres, db)))
	t.Cleanup(func() { _ = client.Close() })
	mock.ExpectBegin()
	tx, err := client.Tx(context.Background())
	require.NoError(t, err)
	exec := &recordingSQLExecutor{err: errors.New("must use transaction client")}
	cache := &grokCatalogCacheSpy{}
	repo := newAccountRepositoryWithSQL(client, exec, cache)
	mock.ExpectExec(regexp.QuoteMeta(grokCatalogCASQuery)).
		WithArgs(int64(25), "null", string(nextJSON)).
		WillReturnResult(sqlmock.NewResult(0, 1))

	applied, err := repo.CompareAndSwapGrokObservedModels(dbent.NewTxContext(context.Background(), tx), 25, nil, nextJSON)

	require.NoError(t, err)
	require.True(t, applied)
	require.Empty(t, exec.execQueries)
	require.Zero(t, cache.setAccounts)
	require.Zero(t, cache.bucketOps)
	mock.ExpectRollback()
	require.NoError(t, tx.Rollback())
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestGrokCatalogCASAppliesOnCallerTransactionNotBaseClient(t *testing.T) {
	t.Parallel()

	nextJSON := jsonRaw(`{"models":["grok-4.7"]}`)
	baseDB, baseMock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = baseDB.Close() })
	baseClient := dbent.NewClient(dbent.Driver(entsql.OpenDB(dialect.Postgres, baseDB)))
	t.Cleanup(func() { _ = baseClient.Close() })

	txDB, txMock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = txDB.Close() })
	txClient := dbent.NewClient(dbent.Driver(entsql.OpenDB(dialect.Postgres, txDB)))
	t.Cleanup(func() { _ = txClient.Close() })
	txMock.ExpectBegin()
	tx, err := txClient.Tx(context.Background())
	require.NoError(t, err)
	txMock.ExpectExec(regexp.QuoteMeta(grokCatalogCASQuery)).
		WithArgs(int64(25), "null", string(nextJSON)).
		WillReturnResult(sqlmock.NewResult(0, 1))

	exec := &recordingSQLExecutor{err: errors.New("must use transaction client")}
	cache := &grokCatalogCacheSpy{}
	repo := newAccountRepositoryWithSQL(baseClient, exec, cache)

	applied, err := repo.CompareAndSwapGrokObservedModels(dbent.NewTxContext(context.Background(), tx), 25, nil, nextJSON)

	require.NoError(t, err)
	require.True(t, applied)
	require.Empty(t, exec.execQueries)
	require.Zero(t, cache.setAccounts)
	require.Zero(t, cache.bucketOps)
	require.NoError(t, baseMock.ExpectationsWereMet())
	txMock.ExpectRollback()
	require.NoError(t, tx.Rollback())
	require.NoError(t, txMock.ExpectationsWereMet())
}

func TestGrokCatalogCASRefreshesSingleAccountOutsideTransaction(t *testing.T) {
	t.Parallel()

	nextJSON := jsonRaw(`{"models":["grok-4.7"],"fetched_at":"2026-10-25T02:00:01Z"}`)
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	client := dbent.NewClient(dbent.Driver(entsql.OpenDB(dialect.Postgres, db)))
	t.Cleanup(func() { _ = client.Close() })
	cache := &grokCatalogCacheSpy{}
	repo := newAccountRepositoryWithSQL(client, db, cache)
	mock.ExpectExec(regexp.QuoteMeta(grokCatalogCASQuery)).
		WithArgs(int64(25), "null", string(nextJSON)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery(`(?s)SELECT .* FROM "accounts" WHERE .*`).
		WithArgs(int64(25)).
		WillReturnRows(updatedAccountRows(25, `{"unrelated":"keep","grok_observed_models":{"models":["grok-4.7"],"fetched_at":"2026-10-25T02:00:01Z"}}`))
	mock.ExpectQuery(`(?s)account_groups`).
		WillReturnRows(sqlmock.NewRows([]string{"account_id", "group_id", "priority", "created_at"}))

	applied, err := repo.CompareAndSwapGrokObservedModels(context.Background(), 25, nil, nextJSON)

	require.NoError(t, err)
	require.True(t, applied)
	require.Equal(t, 1, cache.setAccounts)
	require.Zero(t, cache.bucketOps)
	require.NotNil(t, cache.lastAccount)
	require.Equal(t, "keep", cache.lastAccount.Extra["unrelated"])
	require.NoError(t, mock.ExpectationsWereMet())
}

func newGrokCatalogCASRepo(t *testing.T, queries *[]string) (*accountRepository, sqlmock.Sqlmock) {
	t.Helper()
	var (
		db   *sql.DB
		mock sqlmock.Sqlmock
		err  error
	)
	if queries == nil {
		db, mock, err = sqlmock.New()
	} else {
		db, mock, err = sqlmock.New(sqlmock.QueryMatcherOption(grokCatalogQueryCapture{queries: queries}))
	}
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	client := dbent.NewClient(dbent.Driver(entsql.OpenDB(dialect.Postgres, db)))
	t.Cleanup(func() { _ = client.Close() })
	return newAccountRepositoryWithSQL(client, db, nil), mock
}

type grokCatalogQueryCapture struct {
	queries *[]string
}

func (m grokCatalogQueryCapture) Match(expectedSQL, actualSQL string) error {
	if m.queries != nil {
		*m.queries = append(*m.queries, actualSQL)
	}
	return sqlmock.QueryMatcherRegexp.Match(expectedSQL, actualSQL)
}

type grokCatalogCacheSpy struct {
	setAccounts int
	bucketOps   int
	lastAccount *service.Account
}

func (s *grokCatalogCacheSpy) GetSnapshot(context.Context, service.SchedulerBucket) ([]*service.Account, bool, error) {
	return nil, false, nil
}

func (s *grokCatalogCacheSpy) CaptureBucketWriteToken(context.Context, service.SchedulerBucket) (service.SchedulerBucketWriteToken, error) {
	s.bucketOps++
	return service.SchedulerBucketWriteToken{}, nil
}

func (s *grokCatalogCacheSpy) SetSnapshot(context.Context, service.SchedulerBucket, service.SchedulerBucketWriteToken, []service.Account) error {
	s.bucketOps++
	return nil
}

func (s *grokCatalogCacheSpy) RetireBucket(context.Context, service.SchedulerBucket) error {
	s.bucketOps++
	return nil
}

func (s *grokCatalogCacheSpy) ReopenBucket(context.Context, service.SchedulerBucket) (service.SchedulerBucketWriteToken, error) {
	s.bucketOps++
	return service.SchedulerBucketWriteToken{}, nil
}

func (s *grokCatalogCacheSpy) TryAcquireGroupLifecycleLease(context.Context, int64, time.Duration) (service.SchedulerGroupLifecycleLease, bool, error) {
	s.bucketOps++
	return service.SchedulerGroupLifecycleLease{}, false, nil
}

func (s *grokCatalogCacheSpy) ReleaseGroupLifecycleLease(context.Context, service.SchedulerGroupLifecycleLease) error {
	s.bucketOps++
	return nil
}

func (s *grokCatalogCacheSpy) GetAccount(context.Context, int64) (*service.Account, error) {
	return nil, nil
}

func (s *grokCatalogCacheSpy) SetAccount(_ context.Context, account *service.Account) error {
	s.setAccounts++
	s.lastAccount = account
	return nil
}

func (s *grokCatalogCacheSpy) DeleteAccount(context.Context, int64) error { return nil }

func (s *grokCatalogCacheSpy) UpdateLastUsed(context.Context, map[int64]time.Time) error {
	return nil
}

func (s *grokCatalogCacheSpy) TryLockBucket(context.Context, service.SchedulerBucket, time.Duration) (bool, error) {
	s.bucketOps++
	return false, nil
}

func (s *grokCatalogCacheSpy) UnlockBucket(context.Context, service.SchedulerBucket) error {
	s.bucketOps++
	return nil
}

func (s *grokCatalogCacheSpy) ListBuckets(context.Context) ([]service.SchedulerBucket, error) {
	return nil, nil
}

func (s *grokCatalogCacheSpy) GetOutboxWatermark(context.Context) (int64, error) { return 0, nil }

func (s *grokCatalogCacheSpy) SetOutboxWatermark(context.Context, int64) error { return nil }

func jsonRaw(s string) json.RawMessage {
	return json.RawMessage(s)
}
