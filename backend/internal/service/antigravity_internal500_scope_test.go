//go:build unit

package service

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// --- mock: records model-level rate limits as well as account-level penalties ---

type internal500ScopeRepoStub struct {
	internal500AccountRepoStub
	modelLimitCalls []modelLimitCall
}

type modelLimitCall struct {
	accountID int64
	scope     string
	resetAt   time.Time
}

func (r *internal500ScopeRepoStub) SetModelRateLimit(_ context.Context, id int64, scope string, resetAt time.Time, _ ...string) error {
	r.modelLimitCalls = append(r.modelLimitCalls, modelLimitCall{accountID: id, scope: scope, resetAt: resetAt})
	return nil
}

// An INTERNAL 500 on an image model is the image endpoint failing, not the account: it must not
// take the account's text models (Argon, Flash) down with it.
func TestInternal500OnImageModelCoolsOnlyThatModel(t *testing.T) {
	repo := &internal500ScopeRepoStub{}
	cache := &mockInternal500Cache{incrementCount: 1}
	svc := &AntigravityGatewayService{accountRepo: repo, internal500Cache: cache}
	account := &Account{ID: 2, Name: "ag", Platform: PlatformAntigravity}

	before := time.Now()
	svc.handleInternal500ForModel(context.Background(), "[test]", account, "gemini-3.1-flash-image")

	require.Empty(t, cache.incrementCalls, "an image 500 must not count toward the account penalty")
	require.Empty(t, repo.tempUnschedCalls, "an image 500 must not make the account unschedulable")
	require.Empty(t, repo.setErrorCalls)
	require.Len(t, repo.modelLimitCalls, 1)
	call := repo.modelLimitCalls[0]
	require.Equal(t, "gemini-3.1-flash-image", call.scope, "only the image model is cooled, not antigravity:gemini")
	require.WithinDuration(t, before.Add(defaultInternal500ImageCooldown), call.resetAt, 2*time.Second)
}

func TestInternal500OnTextModelKeepsTheAccountPenalty(t *testing.T) {
	repo := &internal500ScopeRepoStub{}
	cache := &mockInternal500Cache{incrementCount: 1}
	svc := &AntigravityGatewayService{accountRepo: repo, internal500Cache: cache}
	account := &Account{ID: 2, Name: "ag", Platform: PlatformAntigravity}

	svc.handleInternal500ForModel(context.Background(), "[test]", account, "gemini-4-argon-high")

	require.Len(t, cache.incrementCalls, 1)
	require.Len(t, repo.tempUnschedCalls, 1)
	require.Empty(t, repo.modelLimitCalls)
}

func TestInternal500ImageCooldownFromEnv(t *testing.T) {
	t.Setenv(internal500ImageCooldownEnv, "45")
	require.Equal(t, 45*time.Second, internal500ImageCooldown())
	t.Setenv(internal500ImageCooldownEnv, "bad")
	require.Equal(t, defaultInternal500ImageCooldown, internal500ImageCooldown())
}

// The account penalty tiers default to upstream's values and can be shortened per deployment.
func TestInternal500PenaltyTiersFromEnv(t *testing.T) {
	tier1, tier2, disable := internal500PenaltyTiers()
	require.Equal(t, internal500PenaltyTier1Duration, tier1)
	require.Equal(t, internal500PenaltyTier2Duration, tier2)
	require.True(t, disable, "upstream disables the account on the third round")

	t.Setenv(internal500Tier1Env, "120")
	t.Setenv(internal500Tier2Env, "600")
	t.Setenv(internal500DisableEnv, "false")
	tier1, tier2, disable = internal500PenaltyTiers()
	require.Equal(t, 2*time.Minute, tier1)
	require.Equal(t, 10*time.Minute, tier2)
	require.False(t, disable)
}

func TestInternal500PenaltyUsesConfiguredTiers(t *testing.T) {
	t.Setenv(internal500Tier1Env, "120")
	t.Setenv(internal500Tier2Env, "600")
	t.Setenv(internal500DisableEnv, "false")

	for _, tc := range []struct {
		count int64
		want  time.Duration
	}{{1, 2 * time.Minute}, {2, 10 * time.Minute}, {3, 10 * time.Minute}, {7, 10 * time.Minute}} {
		repo := &internal500AccountRepoStub{}
		svc := &AntigravityGatewayService{accountRepo: repo}
		before := time.Now()
		svc.applyInternal500Penalty(context.Background(), "[test]", &Account{ID: 9, Name: "ag"}, tc.count)
		require.Empty(t, repo.setErrorCalls, "count=%d must not disable the account when disabled by env", tc.count)
		require.Len(t, repo.tempUnschedCalls, 1, "count=%d", tc.count)
		require.WithinDuration(t, before.Add(tc.want), repo.tempUnschedCalls[0].until, 2*time.Second, "count=%d", tc.count)
	}
}
