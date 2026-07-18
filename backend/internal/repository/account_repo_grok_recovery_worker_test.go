package repository

import (
	"context"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestClaimDueGrokFreeRecoveryCandidatesUsesAtomicSkipLockedPage(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	now := time.Date(2026, 7, 18, 1, 0, 0, 0, time.UTC)
	next := now.Add(5 * time.Minute)
	lease := now.Add(10 * time.Minute)
	var capturedSQL string
	mock.ExpectQuery("WITH eligible AS").
		WithArgs(
			service.GrokFreeRecoveryPendingExtraKey,
			service.GrokFreeRecoveryNextProbeAtExtraKey,
			service.GrokFreeRecoveryLastProbeAtExtraKey,
			service.GrokFreeRecoveryLastProbeResultExtraKey,
			now, now.Format(time.RFC3339Nano), next, next.Format(time.RFC3339Nano), lease, 100,
			service.SchedulerOutboxEventAccountChanged,
			grokRecoveryRFC3339Pattern,
		).
		WillReturnRows(sqlmock.NewRows([]string{"id"}))
	repo := newAccountRepositoryWithSQL(nil, captureQuerySQL{db: db, captured: &capturedSQL}, nil)

	accounts, err := repo.ClaimDueGrokFreeRecoveryCandidates(context.Background(), now, next, lease, 100)

	require.NoError(t, err)
	require.Empty(t, accounts)
	normalized := normalizeSQLWhitespace(capturedSQL)
	require.Contains(t, normalized, "FOR UPDATE OF a SKIP LOCKED")
	require.Contains(t, normalized, "LIMIT $10")
	require.Contains(t, normalized, "UPDATE accounts a")
	require.Contains(t, normalized, "INSERT INTO scheduler_outbox")
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestListGrokFreeProactiveCandidatesGuardsMalformedTimestampBeforeCast(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	now := time.Date(2026, 7, 18, 1, 0, 0, 0, time.UTC)
	var capturedSQL string
	mock.ExpectQuery("SELECT a.id").
		WithArgs(
			int64(500), now,
			service.GrokFreeRecoveryPendingExtraKey,
			service.GrokFreeProactiveNextProbeAtExtraKey,
			100,
			grokRecoveryRFC3339Pattern,
		).
		WillReturnRows(sqlmock.NewRows([]string{"id"}))
	repo := newAccountRepositoryWithSQL(nil, captureQuerySQL{db: db, captured: &capturedSQL}, nil)

	accounts, err := repo.ListGrokFreeProactiveCandidates(context.Background(), now, 500, 100)

	require.NoError(t, err)
	require.Empty(t, accounts)
	normalized := normalizeSQLWhitespace(capturedSQL)
	require.Contains(t, normalized, "a.id > $1")
	require.Contains(t, normalized, "CASE WHEN a.extra ->> $4 ~ $6 THEN (a.extra ->> $4)::timestamptz <= $2 ELSE TRUE END")
	require.NotContains(t, normalized, "OR (a.extra ->> $4)::timestamptz")
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestClaimGrokFreeProactiveCandidatesGuardsMalformedTimestampBeforeCast(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	now := time.Date(2026, 7, 18, 1, 0, 0, 0, time.UTC)
	next := now.Add(5 * time.Minute)
	lease := now.Add(10 * time.Minute)
	var capturedSQL string
	mock.ExpectQuery("WITH candidates AS").
		WithArgs(
			sqlmock.AnyArg(),
			service.GrokFreeRecoveryPendingExtraKey,
			service.GrokFreeRecoveryNextProbeAtExtraKey,
			service.GrokFreeRecoveryLastProbeAtExtraKey,
			service.GrokFreeRecoveryLastProbeResultExtraKey,
			service.GrokFreeProactiveNextProbeAtExtraKey,
			now, now.Format(time.RFC3339Nano), next.Format(time.RFC3339Nano), lease,
			service.SchedulerOutboxEventAccountChanged,
			grokRecoveryRFC3339Pattern,
		).
		WillReturnRows(sqlmock.NewRows([]string{"id"}))
	repo := newAccountRepositoryWithSQL(nil, captureQuerySQL{db: db, captured: &capturedSQL}, nil)

	accounts, err := repo.ClaimGrokFreeProactiveCandidates(context.Background(), []int64{7, 8}, now, next, lease)

	require.NoError(t, err)
	require.Empty(t, accounts)
	normalized := normalizeSQLWhitespace(capturedSQL)
	require.Contains(t, normalized, "CASE WHEN a.extra ->> $6 ~ $12 THEN (a.extra ->> $6)::timestamptz <= $7 ELSE TRUE END")
	require.Contains(t, normalized, "FOR UPDATE OF a SKIP LOCKED")
	require.Contains(t, normalized, "INSERT INTO scheduler_outbox")
	require.NoError(t, mock.ExpectationsWereMet())
}
