//go:build integration

package repository

import (
	"context"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

// TestAccountRepositorySetRateLimitedIfUnchanged exercises the atomic generation
// CAS backing the Ollama Cloud usage probe write-back (SetRateLimitedIfUnchanged).
// It must only write when the row still carries the exact UpdatedAt /
// RateLimitedAt / RateLimitResetAt the caller observed, so a stale async reset can
// never overwrite a newer 429, an admin clear, or a re-armed generation.
func TestAccountRepositorySetRateLimitedIfUnchanged(t *testing.T) {
	ctx := context.Background()
	tx := testEntTx(t)
	repo := newAccountRepositoryWithSQL(tx.Client(), tx, nil)

	makeAcct := func(name string) *service.Account {
		return mustCreateAccount(t, tx.Client(), &service.Account{
			Name: name, Platform: service.PlatformOpenAI, Type: service.AccountTypeAPIKey,
			Credentials: map[string]any{"base_url": "https://www.ollama.com", "api_key": name},
		})
	}

	// --- Matching generation applies the new reset. ---
	acct := makeAcct("cas-match")
	base := time.Now().Add(5 * time.Second)
	require.NoError(t, repo.SetRateLimited(ctx, acct.ID, base))

	cur, err := repo.GetByID(ctx, acct.ID)
	require.NoError(t, err)
	require.NotNil(t, cur)
	require.NotNil(t, cur.RateLimitedAt)
	require.NotNil(t, cur.RateLimitResetAt)
	expUpdated, expLimited, expReset := cur.UpdatedAt, cur.RateLimitedAt, cur.RateLimitResetAt

	newReset := time.Now().Add(2 * time.Hour)
	updated, err := repo.SetRateLimitedIfUnchanged(ctx, acct.ID, expUpdated, expLimited, expReset, newReset)
	require.NoError(t, err)
	require.True(t, updated, "a write matching the observed generation must apply")

	after, err := repo.GetByID(ctx, acct.ID)
	require.NoError(t, err)
	require.NotNil(t, after.RateLimitResetAt)
	require.WithinDuration(t, newReset, *after.RateLimitResetAt, 2*time.Second)

	// --- Stale UpdatedAt (row version moved by the write above) must NOT apply. ---
	stale := time.Now().Add(3 * time.Hour)
	updated, err = repo.SetRateLimitedIfUnchanged(ctx, acct.ID, expUpdated, expLimited, expReset, stale)
	require.NoError(t, err)
	require.False(t, updated, "a write whose row version moved on must not apply")

	// --- Current UpdatedAt but OLD limited/reset must NOT apply: the rate-limit
	//     generation was re-armed by the successful write, so the CAS must reject
	//     purely on the limited/reset predicates (isolated from UpdatedAt). ---
	updated, err = repo.SetRateLimitedIfUnchanged(ctx, acct.ID, after.UpdatedAt, expLimited, expReset, stale)
	require.NoError(t, err)
	require.False(t, updated, "re-armed limited/reset generation must not accept the old one even with the current row version")

	// --- Current UpdatedAt but nil limited/reset must NOT apply against a set
	//     generation (admin-clear expectation only matches an unset state). ---
	clearedReset := time.Now().Add(30 * time.Minute)
	updated, err = repo.SetRateLimitedIfUnchanged(ctx, acct.ID, after.UpdatedAt, nil, nil, clearedReset)
	require.NoError(t, err)
	require.False(t, updated, "nil-limited expectation must not match a set generation")

	// --- Stale UpdatedAt alone (with nil/nil) must also fail on the version. ---
	updated, err = repo.SetRateLimitedIfUnchanged(ctx, acct.ID, expUpdated, nil, nil, clearedReset)
	require.NoError(t, err)
	require.False(t, updated, "stale row version must not apply even with nil limited/reset")

	// --- Never-limited account: nil generation CAS applies. ---
	fresh := makeAcct("cas-fresh")
	freshCur, err := repo.GetByID(ctx, fresh.ID)
	require.NoError(t, err)
	require.Nil(t, freshCur.RateLimitedAt)
	require.Nil(t, freshCur.RateLimitResetAt)
	firstReset := time.Now().Add(time.Hour)
	updated, err = repo.SetRateLimitedIfUnchanged(ctx, fresh.ID, freshCur.UpdatedAt, nil, nil, firstReset)
	require.NoError(t, err)
	require.True(t, updated, "nil generation CAS must apply to a never-limited account")

	// Admin-clear style: clear, then the old (set) generation must no longer match.
	require.NoError(t, repo.ClearRateLimit(ctx, fresh.ID))
	afterClear, err := repo.GetByID(ctx, fresh.ID)
	require.NoError(t, err)
	updated, err = repo.SetRateLimitedIfUnchanged(ctx, fresh.ID, afterClear.UpdatedAt, nil, nil, time.Now().Add(time.Hour))
	require.NoError(t, err)
	require.True(t, updated, "cleared account with nil generation accepts a fresh write")
}
