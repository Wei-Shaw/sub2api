package repository

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
)

func newTemporaryBalanceRepoMock(t *testing.T) (*userRepository, sqlmock.Sqlmock) {
	t.Helper()
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	driver := entsql.OpenDB(dialect.Postgres, db)
	client := dbent.NewClient(dbent.Driver(driver))
	t.Cleanup(func() { _ = client.Close() })
	return newUserRepositoryWithSQL(client, db), mock
}

func TestUserRepositoryGetTemporaryBalanceReturnsActiveOnlyWhenUnexpired(t *testing.T) {
	repo, mock := newTemporaryBalanceRepoMock(t)
	expiresAt := time.Date(2026, 9, 6, 23, 59, 59, 0, time.UTC)
	mock.ExpectQuery(`(?s)SELECT id, temporary_balance, temporary_balance_expires_at, CASE.*temporary_balance_expires_at > CURRENT_TIMESTAMP.*FROM users.*WHERE id = ANY\(\$1\) AND deleted_at IS NULL`).
		WithArgs(sqlmock.AnyArg()).
		WillReturnRows(sqlmock.NewRows([]string{"id", "temporary_balance", "temporary_balance_expires_at", "active_temporary_balance"}).AddRow(42, 12.5, expiresAt, 12.5))

	grant, err := repo.GetTemporaryBalance(context.Background(), 42)
	require.NoError(t, err)
	require.Equal(t, int64(42), grant.UserID)
	require.InDelta(t, 12.5, grant.Amount, 0.000001)
	require.InDelta(t, 12.5, grant.ActiveAmount, 0.000001)
	require.NotNil(t, grant.ExpiresAt)
	require.Equal(t, expiresAt, grant.ExpiresAt.UTC())
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestUserRepositoryGetUserBalanceSnapshotReadsAllColumnsAtomically(t *testing.T) {
	repo, mock := newTemporaryBalanceRepoMock(t)
	expiresAt := time.Date(2026, 9, 6, 23, 59, 59, 0, time.UTC)
	mock.ExpectQuery(`(?s)SELECT balance,.*temporary_balance,.*temporary_balance_expires_at,.*active_temporary_balance,.*available_balance.*FROM users.*WHERE id = \$1 AND deleted_at IS NULL`).
		WithArgs(int64(42)).
		WillReturnRows(sqlmock.NewRows([]string{"balance", "temporary_balance", "temporary_balance_expires_at", "active_temporary_balance", "available_balance"}).
			AddRow(4.0, 6.0, expiresAt, 6.0, 10.0))

	snapshot, err := repo.GetUserBalanceSnapshot(context.Background(), 42)
	require.NoError(t, err)
	require.Equal(t, 4.0, snapshot.Balance)
	require.Equal(t, 6.0, snapshot.TemporaryBalance)
	require.Equal(t, 6.0, snapshot.ActiveTemporaryBalance)
	require.Equal(t, 10.0, snapshot.AvailableBalance)
	require.NotNil(t, snapshot.TemporaryBalanceExpiresAt)
	require.Equal(t, expiresAt, *snapshot.TemporaryBalanceExpiresAt)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestUserRepositoryGrantTemporaryBalanceAtomicallyAddsAndAudits(t *testing.T) {
	repo, mock := newTemporaryBalanceRepoMock(t)
	expiresAt := time.Date(2026, 9, 6, 23, 59, 59, 0, time.UTC)
	mock.ExpectQuery(`(?s)WITH prior AS \(.*FOR UPDATE.*UPDATE users.*temporary_balance.*temporary_balance_expires_at.*INSERT INTO temporary_balance_audits.*SELECT.*FROM changed.*SELECT id, temporary_balance, temporary_balance_expires_at`).
		WithArgs(10.25, expiresAt, int64(9), int64(42), "campaign").
		WillReturnRows(sqlmock.NewRows([]string{"id", "temporary_balance", "temporary_balance_expires_at"}).AddRow(42, 20.25, expiresAt))

	grant, err := repo.GrantTemporaryBalance(context.Background(), 42, 10.25, expiresAt, 9, "campaign")
	require.NoError(t, err)
	require.Equal(t, int64(42), grant.UserID)
	require.InDelta(t, 20.25, grant.Amount, 0.000001)
	require.Equal(t, expiresAt, grant.ExpiresAt.UTC())
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestUserRepositoryGrantTemporaryBalanceMissingUser(t *testing.T) {
	repo, mock := newTemporaryBalanceRepoMock(t)
	expiresAt := time.Now().UTC().Add(time.Hour)
	mock.ExpectQuery(`(?s)WITH prior AS \(.*UPDATE users.*INSERT INTO temporary_balance_audits`).
		WithArgs(1.0, sqlmock.AnyArg(), sqlmock.AnyArg(), int64(404), "").
		WillReturnError(sql.ErrNoRows)

	_, err := repo.GrantTemporaryBalance(context.Background(), 404, 1, expiresAt, 0, "")
	require.ErrorIs(t, err, service.ErrUserNotFound)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestUserRepositoryClearExpiredTemporaryBalancesAuditsAndClearsBatch(t *testing.T) {
	repo, mock := newTemporaryBalanceRepoMock(t)
	mock.ExpectQuery(`(?s)WITH expired AS \(.*temporary_balance_expires_at <= CURRENT_TIMESTAMP.*UPDATE users.*temporary_balance = 0.*INSERT INTO temporary_balance_audits.*SELECT COUNT\(\*\)`).
		WithArgs(100).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(3))

	cleared, err := repo.ClearExpiredTemporaryBalances(context.Background(), 100)
	require.NoError(t, err)
	require.Equal(t, 3, cleared)
	require.NoError(t, mock.ExpectationsWereMet())
}
