//go:build integration

package repository

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestWeb3IdentityAddressCanBeReusedAfterUserSoftDelete(t *testing.T) {
	ctx := context.Background()
	suffix := time.Now().UnixNano()
	address := fmt.Sprintf("0x%040x", suffix)
	firstEmail := fmt.Sprintf("web3-soft-delete-first-%d@example.com", suffix)
	secondEmail := fmt.Sprintf("web3-soft-delete-second-%d@example.com", suffix)

	t.Cleanup(func() {
		_, _ = integrationDB.ExecContext(context.Background(), "DELETE FROM web3_identities WHERE address = $1", address)
		_, _ = integrationDB.ExecContext(context.Background(), "DELETE FROM users WHERE email IN ($1, $2)", firstEmail, secondEmail)
	})

	identityRepo := &web3IdentityRepository{db: integrationDB}
	userRepo := NewUserRepository(integrationEntClient, integrationDB)
	firstUserID, err := identityRepo.CreateUserWithIdentity(ctx, service.Web3UserCreateInput{
		Email: firstEmail, PasswordHash: "test-hash", Username: "first",
		Role: service.RoleUser, Status: service.StatusActive, Concurrency: 1,
		Address: address,
	})
	require.NoError(t, err)
	require.NoError(t, userRepo.Delete(ctx, firstUserID))
	var deletedAt time.Time
	require.NoError(t, integrationDB.QueryRowContext(ctx, `
		SELECT deleted_at
		FROM web3_identities
		WHERE user_id = $1
	`, firstUserID).Scan(&deletedAt))
	require.False(t, deletedAt.IsZero())

	exists, err := identityRepo.ExistsByAddress(ctx, address)
	require.NoError(t, err)
	require.False(t, exists)

	secondUserID, err := identityRepo.CreateUserWithIdentity(ctx, service.Web3UserCreateInput{
		Email: secondEmail, PasswordHash: "test-hash", Username: "second",
		Role: service.RoleUser, Status: service.StatusActive, Concurrency: 1,
		Address: address,
	})
	require.NoError(t, err)
	require.NotEqual(t, firstUserID, secondUserID)

	var totalCount, activeCount int
	require.NoError(t, integrationDB.QueryRowContext(ctx, `
		SELECT COUNT(*), COUNT(*) FILTER (WHERE deleted_at IS NULL)
		FROM web3_identities
		WHERE address = $1
	`, address).Scan(&totalCount, &activeCount))
	require.Equal(t, 2, totalCount)
	require.Equal(t, 1, activeCount)
}

func TestWeb3IdentitySoftDeleteRollsBackWithUserDeletionTransaction(t *testing.T) {
	ctx := context.Background()
	suffix := time.Now().UnixNano()
	address := fmt.Sprintf("0x%040x", suffix)
	email := fmt.Sprintf("web3-soft-delete-rollback-%d@example.com", suffix)

	t.Cleanup(func() {
		_, _ = integrationDB.ExecContext(context.Background(), "DELETE FROM web3_identities WHERE address = $1", address)
		_, _ = integrationDB.ExecContext(context.Background(), "DELETE FROM users WHERE email = $1", email)
	})

	identityRepo := &web3IdentityRepository{db: integrationDB}
	userID, err := identityRepo.CreateUserWithIdentity(ctx, service.Web3UserCreateInput{
		Email: email, PasswordHash: "test-hash", Username: "rollback",
		Role: service.RoleUser, Status: service.StatusActive, Concurrency: 1,
		Address: address,
	})
	require.NoError(t, err)

	tx, err := integrationEntClient.Tx(ctx)
	require.NoError(t, err)
	txUserRepo := newUserRepositoryWithSQL(tx.Client(), integrationDB)
	require.NoError(t, txUserRepo.Delete(ctx, userID))
	require.NoError(t, tx.Rollback())

	exists, err := identityRepo.ExistsByAddress(ctx, address)
	require.NoError(t, err)
	require.True(t, exists)

	var deletedAt *time.Time
	require.NoError(t, integrationDB.QueryRowContext(ctx, `
		SELECT deleted_at
		FROM web3_identities
		WHERE user_id = $1
	`, userID).Scan(&deletedAt))
	require.Nil(t, deletedAt)
}

func TestWeb3IdentityConcurrentReuseAfterSoftDeleteAllowsOneActiveRegistration(t *testing.T) {
	ctx := context.Background()
	suffix := time.Now().UnixNano()
	address := fmt.Sprintf("0x%040x", suffix)
	oldEmail := fmt.Sprintf("web3-concurrent-old-%d@example.com", suffix)
	firstEmail := fmt.Sprintf("web3-concurrent-first-%d@example.com", suffix)
	secondEmail := fmt.Sprintf("web3-concurrent-second-%d@example.com", suffix)

	t.Cleanup(func() {
		_, _ = integrationDB.ExecContext(context.Background(), "DELETE FROM web3_identities WHERE address = $1", address)
		_, _ = integrationDB.ExecContext(context.Background(), "DELETE FROM users WHERE email IN ($1, $2, $3)", oldEmail, firstEmail, secondEmail)
	})

	identityRepo := &web3IdentityRepository{db: integrationDB}
	userRepo := NewUserRepository(integrationEntClient, integrationDB)
	oldUserID, err := identityRepo.CreateUserWithIdentity(ctx, service.Web3UserCreateInput{
		Email: oldEmail, PasswordHash: "test-hash", Username: "old",
		Role: service.RoleUser, Status: service.StatusActive, Concurrency: 1,
		Address: address,
	})
	require.NoError(t, err)
	require.NoError(t, userRepo.Delete(ctx, oldUserID))

	start := make(chan struct{})
	results := make(chan error, 2)
	for index, email := range []string{firstEmail, secondEmail} {
		index, email := index, email
		go func() {
			<-start
			_, createErr := identityRepo.CreateUserWithIdentity(ctx, service.Web3UserCreateInput{
				Email: email, PasswordHash: "test-hash", Username: fmt.Sprintf("user-%d", index),
				Role: service.RoleUser, Status: service.StatusActive, Concurrency: 1,
				Address: address,
			})
			results <- createErr
		}()
	}
	close(start)

	var successCount, conflictCount int
	for range 2 {
		createErr := <-results
		switch {
		case createErr == nil:
			successCount++
		case errors.Is(createErr, service.ErrWeb3IdentityExists):
			conflictCount++
		default:
			require.NoError(t, createErr)
		}
	}
	require.Equal(t, 1, successCount)
	require.Equal(t, 1, conflictCount)

	var activeIdentityCount, newUserCount int
	require.NoError(t, integrationDB.QueryRowContext(ctx, `
		SELECT COUNT(*)
		FROM web3_identities
		WHERE address = $1 AND deleted_at IS NULL
	`, address).Scan(&activeIdentityCount))
	require.NoError(t, integrationDB.QueryRowContext(ctx, `
		SELECT COUNT(*)
		FROM users
		WHERE email IN ($1, $2)
	`, firstEmail, secondEmail).Scan(&newUserCount))
	require.Equal(t, 1, activeIdentityCount)
	require.Equal(t, 1, newUserCount)
}
