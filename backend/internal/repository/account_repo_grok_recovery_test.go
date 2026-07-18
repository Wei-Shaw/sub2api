package repository

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
)

func TestSetGrokFreeRecoveryPendingPersistsLatchLeaseAndOutboxAtomically(t *testing.T) {
	resetAt := time.Now().UTC().Add(10 * time.Minute).Truncate(time.Microsecond)
	nextProbeAt := resetAt.Add(-5 * time.Minute).Format(time.RFC3339Nano)
	exec := &recordingSQLExecutor{result: rowsAffectedResult(1)}
	repo := newAccountRepositoryWithSQL(nil, exec, nil)

	err := repo.SetGrokFreeRecoveryPending(context.Background(), 42, map[string]any{
		service.GrokFreeRecoveryNextProbeAtExtraKey: nextProbeAt,
		"grok_quota_snapshot":                       map[string]any{"status_code": 429},
	}, resetAt)

	require.NoError(t, err)
	require.Len(t, exec.execQueries, 1, "recovery latch, lease, and outbox must share one statement")
	normalized := normalizeSQLWhitespace(exec.execQueries[0])
	require.Contains(t, normalized, "WITH updated AS ( UPDATE accounts AS a")
	require.Contains(t, normalized, "extra = COALESCE(a.extra, '{}'::jsonb) || $1::jsonb")
	require.Contains(t, normalized, "rate_limited_at = CASE")
	require.Contains(t, normalized, "rate_limit_reset_at = CASE")
	require.Contains(t, normalized, "a.rate_limit_reset_at < $2")
	require.Contains(t, normalized, "INSERT INTO scheduler_outbox")
	require.Len(t, exec.execArgs[0], 6)
	require.Equal(t, resetAt, exec.execArgs[0][1])
	require.Equal(t, int64(42), exec.execArgs[0][2])
	require.Equal(t, service.PlatformGrok, exec.execArgs[0][3])
	require.Equal(t, service.AccountTypeOAuth, exec.execArgs[0][4])
	require.Equal(t, service.SchedulerOutboxEventAccountChanged, exec.execArgs[0][5])

	var payload map[string]any
	err = json.Unmarshal([]byte(exec.execArgs[0][0].(string)), &payload)
	require.NoError(t, err)
	require.Equal(t, true, payload[service.GrokFreeRecoveryPendingExtraKey])
	require.Equal(t, nextProbeAt, payload[service.GrokFreeRecoveryNextProbeAtExtraKey])
}

func TestSetGrokFreeRecoveryPendingDoesNotSplitWriteOnAtomicFailure(t *testing.T) {
	wantErr := errors.New("scheduler outbox insert failed")
	exec := &recordingSQLExecutor{err: wantErr}
	repo := newAccountRepositoryWithSQL(nil, exec, nil)

	err := repo.SetGrokFreeRecoveryPending(
		context.Background(),
		42,
		map[string]any{service.GrokFreeRecoveryPendingExtraKey: true},
		time.Now().Add(10*time.Minute),
	)

	require.ErrorIs(t, err, wantErr)
	require.Len(t, exec.execQueries, 1)
	require.Contains(t, normalizeSQLWhitespace(exec.execQueries[0]), "INSERT INTO scheduler_outbox")
}

func TestClearRateLimitPreservesGrokFreeRecoveryPendingInSQL(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	driver := entsql.OpenDB(dialect.Postgres, db)
	client := dbent.NewClient(dbent.Driver(driver))
	t.Cleanup(func() { _ = client.Close() })
	repo := newAccountRepositoryWithSQL(client, db, nil)

	mock.ExpectExec(`(?s)UPDATE accounts.*rate_limited_at = CASE.*COALESCE\(extra ->> \$2, 'false'\) = 'true' THEN rate_limited_at.*rate_limit_reset_at = CASE.*THEN rate_limit_reset_at.*extra = CASE.*THEN COALESCE\(extra, '\{\}'::jsonb\)`).
		WithArgs(
			int64(73),
			service.GrokFreeRecoveryPendingExtraKey,
			service.GrokFreeRecoveryNextProbeAtExtraKey,
			service.GrokFreeRecoveryLastProbeAtExtraKey,
		).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`INSERT INTO scheduler_outbox`).
		WithArgs(service.SchedulerOutboxEventAccountChanged, int64(73), nil, nil, sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(0, 1))

	require.NoError(t, repo.ClearRateLimit(context.Background(), 73))
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestClearGrokFreeRecoveryIfUnchanged_CASMismatchDoesNotClear(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	driver := entsql.OpenDB(dialect.Postgres, db)
	client := dbent.NewClient(dbent.Driver(driver))
	t.Cleanup(func() { _ = client.Close() })
	repo := newAccountRepositoryWithSQL(client, db, nil)

	probeStartedAt := time.Now().UTC().Truncate(time.Microsecond)
	nextProbeAt := probeStartedAt.Add(5 * time.Minute)
	mock.ExpectExec(`(?s)WITH updated AS.*UPDATE accounts.*COALESCE\(extra ->> \$2, 'false'\) = 'true'.*extra ->> \$3 = \$6.*rate_limited_at IS NULL OR rate_limited_at <= \$5.*RETURNING id.*INSERT INTO scheduler_outbox`).
		WithArgs(
			int64(91),
			service.GrokFreeRecoveryPendingExtraKey,
			service.GrokFreeRecoveryNextProbeAtExtraKey,
			service.GrokFreeRecoveryLastProbeAtExtraKey,
			probeStartedAt,
			nextProbeAt.Format(time.RFC3339Nano),
			service.SchedulerOutboxEventAccountChanged,
		).
		WillReturnResult(sqlmock.NewResult(0, 0))

	cleared, err := repo.ClearGrokFreeRecoveryIfUnchanged(context.Background(), 91, probeStartedAt, nextProbeAt)

	require.NoError(t, err)
	require.False(t, cleared)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestClearGrokFreeRecoveryIfUnchanged_ClearsAndEnqueuesAtomically(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	driver := entsql.OpenDB(dialect.Postgres, db)
	client := dbent.NewClient(dbent.Driver(driver))
	t.Cleanup(func() { _ = client.Close() })
	repo := newAccountRepositoryWithSQL(client, db, nil)

	probeStartedAt := time.Now().UTC().Truncate(time.Microsecond)
	nextProbeAt := probeStartedAt.Add(5 * time.Minute)
	mock.ExpectExec(`(?s)WITH updated AS.*UPDATE accounts.*RETURNING id.*INSERT INTO scheduler_outbox.*SELECT \$7, updated.id`).
		WithArgs(
			int64(92),
			service.GrokFreeRecoveryPendingExtraKey,
			service.GrokFreeRecoveryNextProbeAtExtraKey,
			service.GrokFreeRecoveryLastProbeAtExtraKey,
			probeStartedAt,
			nextProbeAt.Format(time.RFC3339Nano),
			service.SchedulerOutboxEventAccountChanged,
		).
		WillReturnResult(sqlmock.NewResult(0, 1))

	cleared, err := repo.ClearGrokFreeRecoveryIfUnchanged(context.Background(), 92, probeStartedAt, nextProbeAt)

	require.NoError(t, err)
	require.True(t, cleared)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestClearGrokFreeRecoveryIfUnchanged_OutboxFailureRollsBackStatement(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	driver := entsql.OpenDB(dialect.Postgres, db)
	client := dbent.NewClient(dbent.Driver(driver))
	t.Cleanup(func() { _ = client.Close() })
	repo := newAccountRepositoryWithSQL(client, db, nil)

	probeStartedAt := time.Now().UTC().Truncate(time.Microsecond)
	nextProbeAt := probeStartedAt.Add(5 * time.Minute)
	wantErr := errors.New("scheduler outbox insert failed")
	mock.ExpectExec(`(?s)WITH updated AS.*UPDATE accounts.*RETURNING id.*INSERT INTO scheduler_outbox`).
		WithArgs(
			int64(93),
			service.GrokFreeRecoveryPendingExtraKey,
			service.GrokFreeRecoveryNextProbeAtExtraKey,
			service.GrokFreeRecoveryLastProbeAtExtraKey,
			probeStartedAt,
			nextProbeAt.Format(time.RFC3339Nano),
			service.SchedulerOutboxEventAccountChanged,
		).
		WillReturnError(wantErr)

	cleared, err := repo.ClearGrokFreeRecoveryIfUnchanged(context.Background(), 93, probeStartedAt, nextProbeAt)

	require.False(t, cleared)
	require.ErrorIs(t, err, wantErr)
	require.NoError(t, mock.ExpectationsWereMet())
}
