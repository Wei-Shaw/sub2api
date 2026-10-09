//go:build integration

package repository

import (
	"context"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestAccountRepositoryClearClaudeRateLimitIfUnchanged(t *testing.T) {
	ctx := context.Background()
	tx := testEntTx(t)
	repo := newAccountRepositoryWithSQL(tx.Client(), tx, nil)
	account := mustCreateAccount(t, tx.Client(), &service.Account{
		Name: "openai-recovery", Platform: service.PlatformAnthropic,
		Type: service.AccountTypeOAuth, Status: service.StatusActive, Schedulable: true,
	})
	unrelatedUntil := time.Now().Add(2 * time.Hour)
	require.NoError(t, repo.SetOverloaded(ctx, account.ID, unrelatedUntil))
	require.NoError(t, repo.SetTempUnschedulable(ctx, account.ID, unrelatedUntil, "other_blocker"))
	require.NoError(t, repo.SetRateLimited(ctx, account.ID, time.Now().Add(4*24*time.Hour)))

	observed, err := repo.GetByID(ctx, account.ID)
	require.NoError(t, err)
	require.NotNil(t, observed.RateLimitedAt)
	require.NotNil(t, observed.RateLimitResetAt)
	cleared, err := repo.ClearClaudeRateLimitIfUnchanged(ctx, account.ID, observed.UpdatedAt, *observed.RateLimitedAt, *observed.RateLimitResetAt)
	require.NoError(t, err)
	require.True(t, cleared)

	after, err := repo.GetByID(ctx, account.ID)
	require.NoError(t, err)
	require.Nil(t, after.RateLimitedAt)
	require.Nil(t, after.RateLimitResetAt)
	require.NotNil(t, after.OverloadUntil)
	require.NotNil(t, after.TempUnschedulableUntil)
	require.Equal(t, "other_blocker", after.TempUnschedulableReason)

	require.NoError(t, repo.SetRateLimited(ctx, account.ID, time.Now().Add(5*24*time.Hour)))
	rearmed, err := repo.GetByID(ctx, account.ID)
	require.NoError(t, err)
	cleared, err = repo.ClearClaudeRateLimitIfUnchanged(ctx, account.ID, observed.UpdatedAt, *observed.RateLimitedAt, *observed.RateLimitResetAt)
	require.NoError(t, err)
	require.False(t, cleared, "a new 429 generation must survive an older quota response")

	require.NoError(t, repo.UpdateCredentials(ctx, account.ID, map[string]any{"access_token": "replacement"}))
	cleared, err = repo.ClearClaudeRateLimitIfUnchanged(ctx, account.ID, rearmed.UpdatedAt, *rearmed.RateLimitedAt, *rearmed.RateLimitResetAt)
	require.NoError(t, err)
	require.False(t, cleared, "a row edit must invalidate an in-flight quota response")

	current, err := repo.GetByID(ctx, account.ID)
	require.NoError(t, err)
	_, err = tx.Client().Account.UpdateOneID(account.ID).SetType(service.AccountTypeAPIKey).Save(ctx)
	require.NoError(t, err)
	cleared, err = repo.ClearClaudeRateLimitIfUnchanged(ctx, account.ID, current.UpdatedAt, *current.RateLimitedAt, *current.RateLimitResetAt)
	require.NoError(t, err)
	require.False(t, cleared, "an account type change must not be recovered")
}
