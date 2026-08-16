//go:build unit

package repository

import (
	"context"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestUserRepositoryGetTokenVersionReadsPersistentStamp(t *testing.T) {
	repo, mock := newRedeemAdjustmentRepoMock(t)
	mock.ExpectQuery(`SELECT token_version FROM users WHERE id = \$1 AND deleted_at IS NULL`).
		WithArgs(int64(42)).
		WillReturnRows(sqlmock.NewRows([]string{"token_version"}).AddRow(int64(9)))

	version, err := repo.GetTokenVersion(context.Background(), 42)
	require.NoError(t, err)
	require.Equal(t, int64(9), version)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestUserRepositoryIncrementTokenVersionIsSingleAtomicStatement(t *testing.T) {
	repo, mock := newRedeemAdjustmentRepoMock(t)
	mock.ExpectQuery(`(?s)UPDATE users\s+SET token_version = token_version \+ 1\s+WHERE id = \$1 AND deleted_at IS NULL\s+RETURNING token_version`).
		WithArgs(int64(42)).
		WillReturnRows(sqlmock.NewRows([]string{"token_version"}).AddRow(int64(10)))

	version, err := repo.IncrementTokenVersion(context.Background(), 42)
	require.NoError(t, err)
	require.Equal(t, int64(10), version)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestUserRepositoryIncrementTokenVersionRejectsMissingUser(t *testing.T) {
	repo, mock := newRedeemAdjustmentRepoMock(t)
	mock.ExpectQuery(`(?s)UPDATE users\s+SET token_version = token_version \+ 1\s+WHERE id = \$1 AND deleted_at IS NULL\s+RETURNING token_version`).
		WithArgs(int64(404)).
		WillReturnRows(sqlmock.NewRows([]string{"token_version"}))

	_, err := repo.IncrementTokenVersion(context.Background(), 404)
	require.ErrorIs(t, err, service.ErrUserNotFound)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestUserRepositoryGetByIDHydratesPersistentTokenVersion(t *testing.T) {
	repo, _ := newUserEntRepo(t)
	ctx := context.Background()
	user := &service.User{
		Email:        "persistent-stamp@example.com",
		Username:     "persistent-stamp",
		PasswordHash: "hash",
		Role:         service.RoleUser,
		Status:       service.StatusActive,
	}
	require.NoError(t, repo.Create(ctx, user))
	_, err := repo.sql.ExecContext(ctx, `UPDATE users SET token_version = 12 WHERE id = $1`, user.ID)
	require.NoError(t, err)

	loaded, err := repo.GetByID(ctx, user.ID)
	require.NoError(t, err)
	require.True(t, loaded.TokenVersionResolved)
	require.Equal(t, int64(12), loaded.TokenVersion)
}

func TestUserRepositoryPasswordUpdateAdvancesTokenVersionInTransaction(t *testing.T) {
	repo, _ := newUserEntRepo(t)
	ctx := context.Background()
	user := &service.User{
		Email:        "password-stamp@example.com",
		Username:     "password-stamp",
		PasswordHash: "old-hash",
		Role:         service.RoleUser,
		Status:       service.StatusActive,
	}
	require.NoError(t, repo.Create(ctx, user))
	user.PasswordHash = "new-hash"
	require.NoError(t, repo.Update(ctx, user, service.UserUpdateFields{PasswordHash: true}))
	require.True(t, user.TokenVersionResolved)
	require.Equal(t, int64(1), user.TokenVersion)

	loaded, err := repo.GetByID(ctx, user.ID)
	require.NoError(t, err)
	require.Equal(t, "new-hash", loaded.PasswordHash)
	require.Equal(t, int64(1), loaded.TokenVersion)
}

func TestUserRepositoryRoleAndStatusTransitionsAdvanceTokenVersion(t *testing.T) {
	repo, _ := newUserEntRepo(t)
	ctx := context.Background()
	user := &service.User{
		Email:        "authorization-stamp@example.com",
		Username:     "authorization-stamp",
		PasswordHash: "hash",
		Role:         service.RoleUser,
		Status:       service.StatusActive,
	}
	require.NoError(t, repo.Create(ctx, user))

	user.Role = service.RoleAdmin
	require.NoError(t, repo.Update(ctx, user, service.UserUpdateFields{Role: true}))
	require.Equal(t, int64(1), user.TokenVersion)

	user.Role = service.RoleUser
	require.NoError(t, repo.Update(ctx, user, service.UserUpdateFields{Role: true}))
	require.Equal(t, int64(2), user.TokenVersion, "demotion and later restoration must not revive pre-demotion tokens")

	user.Status = service.StatusDisabled
	require.NoError(t, repo.Update(ctx, user, service.UserUpdateFields{Status: true}))
	require.Equal(t, int64(3), user.TokenVersion)

	user.Status = service.StatusActive
	require.NoError(t, repo.Update(ctx, user, service.UserUpdateFields{Status: true}))
	require.Equal(t, int64(4), user.TokenVersion, "disable and later re-enable must not revive pre-disable tokens")

	loaded, err := repo.GetByID(ctx, user.ID)
	require.NoError(t, err)
	require.Equal(t, service.RoleUser, loaded.Role)
	require.Equal(t, service.StatusActive, loaded.Status)
	require.Equal(t, int64(4), loaded.TokenVersion)

	// Idempotent writes are not a security-boundary transition and should not
	// churn every active session merely because a caller resubmits the same value.
	require.NoError(t, repo.Update(ctx, user, service.UserUpdateFields{Role: true, Status: true}))
	require.Equal(t, int64(4), user.TokenVersion)
}
