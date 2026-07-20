//go:build unit

package service

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/xai"
	"github.com/stretchr/testify/require"
)

func grokFreeUsageExhaustedBody() []byte {
	return []byte(`{
		"code":"subscription:free-usage-exhausted",
		"error":"You've used all the included free usage for model grok-4.5-build-free for now. Usage resets over a rolling 24-hour window - tokens (actual/limit): 1065554/1000000."
	}`)
}

func TestParseGrokFreeUsageExhausted(t *testing.T) {
	t.Parallel()

	details, ok := parseGrokFreeUsageExhausted([]byte(`{
		"code":"subscription:free-usage-exhausted",
		"error":"Usage resets over a rolling 24-hour window - tokens (actual/limit): 1,065,554/1,000,000."
	}`))

	require.True(t, ok)
	require.EqualValues(t, 1_065_554, details.Actual)
	require.EqualValues(t, 1_000_000, details.Limit)
}

func TestParseGrokFreeUsageExhaustedRejectsUnrelatedOrMalformedPayloads(t *testing.T) {
	t.Parallel()

	for _, body := range [][]byte{
		[]byte(`{"code":"other","error":"tokens (actual/limit): 10/5"}`),
		[]byte(`{"code":"subscription:free-usage-exhausted","error":"missing usage values"}`),
		[]byte(`{"code":"subscription:free-usage-exhausted","error":"tokens (actual/limit): 10/0"}`),
		[]byte(`not-json`),
	} {
		_, ok := parseGrokFreeUsageExhausted(body)
		require.False(t, ok, string(body))
	}
}

func TestEnrichGrokQuotaSnapshotFromFreeUsageError(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 7, 18, 7, 0, 0, 0, time.UTC)
	resetAt := now.Add(45 * time.Minute).Unix()
	snapshot := &xai.QuotaSnapshot{
		StatusCode: http.StatusTooManyRequests,
		Tokens:     &xai.QuotaWindow{ResetUnix: &resetAt},
		UpdatedAt:  now.Format(time.RFC3339),
	}

	got := enrichGrokQuotaSnapshotFromError(snapshot, []byte(`{
		"code":"subscription:free-usage-exhausted",
		"error":"tokens (actual/limit): 1065554/1000000"
	}`), http.StatusTooManyRequests, now)

	require.Same(t, snapshot, got)
	require.Equal(t, grokFreeUsageExhaustedErrorCode, got.ProviderErrorCode)
	require.Equal(t, "free", got.SubscriptionTier)
	require.Equal(t, "error_body", got.ObservationSource)
	require.EqualValues(t, 1_000_000, *got.Tokens.Limit)
	require.Zero(t, *got.Tokens.Remaining)
	require.Equal(t, resetAt, *got.Tokens.ResetUnix)
}

func TestEnrichGrokQuotaSnapshotKeepsFreeUsageCodeWhenUsageValuesAreMissing(t *testing.T) {
	t.Parallel()

	snapshot := enrichGrokQuotaSnapshotFromError(nil, []byte(`{
		"code":"subscription:free-usage-exhausted",
		"error":"free allowance exhausted"
	}`), http.StatusForbidden, time.Now())

	require.NotNil(t, snapshot)
	require.Equal(t, grokFreeUsageExhaustedErrorCode, snapshot.ProviderErrorCode)
	require.Equal(t, "free", snapshot.SubscriptionTier)
	require.Equal(t, http.StatusForbidden, snapshot.StatusCode)
	require.Nil(t, snapshot.Tokens)
}

func TestGrokFreeUsageExhaustedCooldownPolicy(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 7, 18, 7, 0, 0, 0, time.UTC)
	freeSnapshot := func() *xai.QuotaSnapshot {
		return &xai.QuotaSnapshot{
			StatusCode:        http.StatusTooManyRequests,
			ProviderErrorCode: grokFreeUsageExhaustedErrorCode,
			UpdatedAt:         now.Format(time.RFC3339),
		}
	}

	t.Run("uses dedicated fallback without upstream boundary", func(t *testing.T) {
		resetAt, limited := grokRateLimitResetAt(freeSnapshot(), now)
		require.True(t, limited)
		require.Equal(t, now.Add(grokFreeUsageExhaustedCooldown), resetAt)
	})

	t.Run("prefers retry-after", func(t *testing.T) {
		retryAfter := 45
		snapshot := freeSnapshot()
		snapshot.RetryAfterSeconds = &retryAfter
		resetAt, limited := grokRateLimitResetAt(snapshot, now)
		require.True(t, limited)
		require.Equal(t, now.Add(45*time.Second), resetAt)
	})

	t.Run("prefers future token reset", func(t *testing.T) {
		remaining := int64(0)
		resetUnix := now.Add(2 * time.Hour).Unix()
		snapshot := freeSnapshot()
		snapshot.Tokens = &xai.QuotaWindow{Remaining: &remaining, ResetUnix: &resetUnix}
		resetAt, limited := grokRateLimitResetAt(snapshot, now)
		require.True(t, limited)
		require.Equal(t, time.Unix(resetUnix, 0), resetAt)
	})

	t.Run("keeps generic headerless 429 fallback", func(t *testing.T) {
		resetAt, limited := grokRateLimitResetAt(&xai.QuotaSnapshot{StatusCode: http.StatusTooManyRequests}, now)
		require.True(t, limited)
		require.Equal(t, now.Add(grokRateLimitFallbackCooldown), resetAt)
	})

	t.Run("keeps free exhaustion above generic adaptive cap", func(t *testing.T) {
		previousReset := now.Add(-time.Second)
		previousLimited := previousReset.Add(-grokFreeUsageExhaustedCooldown)
		account := &Account{
			Platform:         PlatformGrok,
			Type:             AccountTypeOAuth,
			RateLimitedAt:    &previousLimited,
			RateLimitResetAt: &previousReset,
		}
		resetAt, limited := grokRateLimitResetAtForAccount(account, freeSnapshot(), now)
		require.True(t, limited)
		// Free exhaustion uses a dedicated 24h fallback; adaptive 429 backoff
		// (capped at 1h) must not shorten that window.
		require.Equal(t, now.Add(grokFreeUsageExhaustedCooldown), resetAt)
		require.True(t, resetAt.After(now.Add(grokRateLimitMaxAdaptiveCooldown)))
	})
}

func TestHandleGrokAccountUpstreamErrorPersistsFreeUsageExhaustion(t *testing.T) {
	t.Parallel()

	account := healthyGrokQuotaOAuthAccount(701)
	repo := &grokQuotaAccountRepo{mockAccountRepoForPlatform: &mockAccountRepoForPlatform{
		accountsByID: map[int64]*Account{account.ID: account},
	}}
	svc := &OpenAIGatewayService{accountRepo: repo}
	before := time.Now()

	svc.handleGrokAccountUpstreamError(
		context.Background(), account, http.StatusTooManyRequests, nil, grokFreeUsageExhaustedBody(),
	)

	require.Equal(t, 1, repo.rateLimitedCalls)
	require.WithinDuration(t, before.Add(grokFreeUsageExhaustedCooldown), repo.lastRateLimitResetAt, time.Second)
	stored, ok := repo.updates[account.ID][grokQuotaSnapshotExtraKey].(*xai.QuotaSnapshot)
	require.True(t, ok)
	require.Equal(t, grokFreeUsageExhaustedErrorCode, stored.ProviderErrorCode)
	require.Equal(t, "free", stored.SubscriptionTier)
	require.EqualValues(t, 1_000_000, *stored.Tokens.Limit)
	require.Zero(t, *stored.Tokens.Remaining)
	require.NotNil(t, stored.Tokens.ResetUnix)
	require.WithinDuration(t, repo.lastRateLimitResetAt, time.Unix(*stored.Tokens.ResetUnix, 0), time.Second)
}

func TestExtractUpstreamErrorCodeSupportsTopLevelCode(t *testing.T) {
	t.Parallel()

	require.Equal(
		t,
		grokFreeUsageExhaustedErrorCode,
		extractUpstreamErrorCode([]byte(`{"code":"subscription:free-usage-exhausted","error":"exhausted"}`)),
	)
}
