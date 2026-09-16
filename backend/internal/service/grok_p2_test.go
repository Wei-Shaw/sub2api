//go:build unit

package service

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestGrokModelQuotaBlock_FiltersOnlyNamedModel(t *testing.T) {
	id := time.Now().UnixNano()%1_000_000 + 5000
	markGrokModelQuotaBlock(id, "grok-4.5", time.Now().Add(time.Hour))
	now := time.Now()
	require.True(t, isGrokModelQuotaBlocked(id, "grok-4.5", now))
	require.False(t, isGrokModelQuotaBlocked(id, "grok-4.3", now))

	accounts := []Account{
		{ID: id, Platform: PlatformGrok, Type: AccountTypeOAuth},
		{ID: id + 1, Platform: PlatformGrok, Type: AccountTypeOAuth},
	}
	filtered := filterGrokModelQuotaBlockedAccounts(accounts, "grok-4.5", now)
	require.Len(t, filtered, 1)
	require.Equal(t, id+1, filtered[0].ID)
}

func TestGrokModelQuotaBlockFiltersMappedUpstreamModel(t *testing.T) {
	id := time.Now().UnixNano()%1_000_000 + 7000
	markGrokModelQuotaBlock(id, "grok-4.5", time.Now().Add(time.Hour))
	account := Account{
		ID:       id,
		Platform: PlatformGrok,
		Type:     AccountTypeOAuth,
		Credentials: map[string]any{
			"model_mapping": map[string]any{"gpt-*": "grok-4.5"},
		},
	}

	require.Empty(t, filterGrokModelQuotaBlockedAccounts([]Account{account}, "gpt-5", time.Now()))
}

func TestIsGrokModelSpecificFreeUsage(t *testing.T) {
	require.True(t, isGrokModelSpecificFreeUsage(
		"you've used all the included free usage for model grok-4.5", "grok-4.5"))
	require.True(t, isGrokModelSpecificFreeUsage("模型额度用完 grok-4.3", "grok-4.3"))
	require.False(t, isGrokModelSpecificFreeUsage("free usage exhausted", "grok-4.5"))
}

func TestGrokStickyAffinitySeed_ScopesByModel(t *testing.T) {
	a := grokStickyAffinitySeed("session-1", []byte(`{"model":"grok-4.5"}`))
	b := grokStickyAffinitySeed("session-1", []byte(`{"model":"grok-4.3"}`))
	c := grokStickyAffinitySeed("session-1", []byte(`{"model":"grok-4.5"}`))
	require.NotEqual(t, a, b)
	require.Equal(t, a, c)
	require.Contains(t, a, "grok-affinity:v1:")
}

func TestExtractGrokModelIDsFromModelsBody(t *testing.T) {
	body := []byte(`{"object":"list","data":[{"id":"grok-4.5"},{"id":"grok-4.3"},{"id":"grok-4.5"}]}`)
	ids := extractGrokModelIDsFromModelsBody(body)
	require.Equal(t, []string{"grok-4.5", "grok-4.3"}, ids)
}

func TestAccountGrokNeedsReauth(t *testing.T) {
	require.False(t, accountGrokNeedsReauth(nil))
	require.True(t, accountGrokNeedsReauth(&Account{
		Extra: map[string]any{"grok_needs_reauth": true},
	}))
	require.True(t, accountGrokNeedsReauth(&Account{
		Status:       StatusError,
		ErrorMessage: "Grok spending limit reached; reauthorize or wait for billing reset",
	}))
}

func TestApplyGrokUpstreamFailure_ModelSpecificFreeUsage(t *testing.T) {
	repo := &grokQuotaAccountRepo{}
	svc := &OpenAIGatewayService{accountRepo: repo}
	account := &Account{ID: 9109, Platform: PlatformGrok, Type: AccountTypeOAuth}
	body := []byte(`{"error":{"code":"subscription:free-usage-exhausted","message":"You've used all the included free usage for model grok-4.5. Usage resets over a rolling 24-hour window."}}`)

	svc.handleGrokAccountUpstreamError(context.Background(), account, 400, nil, body)

	require.Zero(t, repo.tempUnschedCalls, "model-scoped free usage must not cool sibling models")
	require.True(t, isGrokModelQuotaBlocked(account.ID, "grok-4.5", time.Now()))
	require.False(t, isGrokModelQuotaBlocked(account.ID, "grok-4.3", time.Now()))
}

func TestApplyGrokUpstreamFailure_SpendingLimitRemainsRecoverable(t *testing.T) {
	repo := &grokQuotaAccountRepo{}
	svc := &OpenAIGatewayService{accountRepo: repo}
	account := &Account{ID: 9110, Platform: PlatformGrok, Type: AccountTypeOAuth}
	body := []byte(`{"code":"personal-team-blocked:spending-limit","error":"spending limit reached"}`)

	svc.handleGrokAccountUpstreamError(context.Background(), account, 403, nil, body)

	require.Equal(t, 1, repo.rateLimitedCalls)
	require.Zero(t, repo.tempUnschedCalls)
	// Without a billing-period snapshot, use a short recoverable probe cooldown.
	require.WithinDuration(t, time.Now().Add(grokSpendingLimitProbeCooldown), repo.lastRateLimitResetAt, 2*time.Second)
}
