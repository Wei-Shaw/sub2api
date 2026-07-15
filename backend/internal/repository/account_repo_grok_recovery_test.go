package repository

import (
	"context"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
)

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
	mock.ExpectExec(`(?s)UPDATE accounts.*COALESCE\(extra ->> \$2, 'false'\) = 'true'.*extra ->> \$3 = \$6.*rate_limited_at IS NULL OR rate_limited_at <= \$5`).
		WithArgs(
			int64(91),
			service.GrokFreeRecoveryPendingExtraKey,
			service.GrokFreeRecoveryNextProbeAtExtraKey,
			service.GrokFreeRecoveryLastProbeAtExtraKey,
			probeStartedAt,
			nextProbeAt.Format(time.RFC3339Nano),
		).
		WillReturnResult(sqlmock.NewResult(0, 0))

	cleared, err := repo.ClearGrokFreeRecoveryIfUnchanged(context.Background(), 91, probeStartedAt, nextProbeAt)

	require.NoError(t, err)
	require.False(t, cleared)
	require.NoError(t, mock.ExpectationsWereMet())
}
