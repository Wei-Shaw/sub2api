package service

import (
	"context"
	"errors"
	"math"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestNormalizeOpenAIAutoResetIndependentPolicies(t *testing.T) {
	extra := map[string]any{
		OpenAIAutoResetCreditEnabledExtraKey:       true,
		OpenAIAutoResetCredit5hDisabledExtraKey:    true,
		OpenAIAutoResetCredit7dDisabledExtraKey:    true,
		OpenAIAutoResetCreditExpiryEnabledExtraKey: true,
	}
	normalized, err := normalizeOpenAIAutoResetCreditExtra(PlatformOpenAI, AccountTypeOAuth, false, extra)
	require.NoError(t, err)
	config := ResolveOpenAIAutoResetCreditConfig(&Account{Platform: PlatformOpenAI, Type: AccountTypeOAuth, Extra: normalized})
	require.True(t, config.Disabled5h)
	require.True(t, config.Disabled7d)
	require.True(t, config.ExpiryEnabled)
	require.Equal(t, 60, config.ExpiryMinutes)
	for _, minutes := range []float64{1, 60, 1440, 525600} {
		extra[OpenAIAutoResetCreditExpiryMinutesExtraKey] = minutes
		_, err := normalizeOpenAIAutoResetCreditExtra(PlatformOpenAI, AccountTypeOAuth, false, extra)
		require.NoError(t, err)
	}
	for _, minutes := range []float64{0, -1, 1.5, 525601, math.Inf(1), math.NaN()} {
		extra[OpenAIAutoResetCreditExpiryMinutesExtraKey] = minutes
		_, err := normalizeOpenAIAutoResetCreditExtra(PlatformOpenAI, AccountTypeOAuth, false, extra)
		require.Error(t, err)
	}
	delete(extra, OpenAIAutoResetCreditExpiryMinutesExtraKey)
	for _, key := range []string{OpenAIAutoResetCredit5hDisabledExtraKey, OpenAIAutoResetCredit7dDisabledExtraKey, OpenAIAutoResetCreditExpiryEnabledExtraKey} {
		invalid := cloneOpenAIAutoResetExtra(extra)
		invalid[key] = "true"
		_, err := normalizeOpenAIAutoResetCreditExtra(PlatformOpenAI, AccountTypeOAuth, false, invalid)
		require.Error(t, err)
	}
	stripped := stripOpenAIAutoResetCreditManagedExtra(normalized, true)
	require.Empty(t, stripped)
}

func TestOpenAIQuotaAutoResetDisabledWindowsPreservePause(t *testing.T) {
	now := time.Now()
	account := &Account{Platform: PlatformOpenAI, Type: AccountTypeOAuth, Extra: map[string]any{
		OpenAIAutoResetCreditEnabledExtraKey:    true,
		OpenAIAutoResetCredit5hDisabledExtraKey: true,
		"auto_pause_5h_threshold":               0.8,
		"auto_pause_7d_disabled":                true,
		"codex_5h_used_percent":                 99.0,
		"codex_5h_reset_at":                     now.Add(time.Hour).Format(time.RFC3339),
		"codex_usage_updated_at":                now.Format(time.RFC3339),
		OpenAIAutoResetCreditStateExtraKey:      OpenAIAutoResetCreditState{Status: OpenAIAutoResetStatusAvailable, AvailableCount: 1, CheckedAt: now.Format(time.RFC3339)},
	}}
	svc := &OpenAIQuotaAutoResetService{}
	config := ResolveOpenAIAutoResetCreditConfig(account)
	require.False(t, svc.buildAssessment(account, config, 1, 0).resetReached)
	require.Equal(t, "7d", svc.buildAssessment(account, config, 1, 1).triggerWindow)
	paused, decision := shouldAutoPauseOpenAIAccountByQuota(context.Background(), account)
	require.True(t, paused, "disabled 5h reset must not bypass ordinary 5h pause")
	require.Equal(t, "5h", decision.window)
	require.Empty(t, decision.reason)
	config.Disabled5h, config.Disabled7d = false, true
	require.False(t, svc.buildAssessment(account, config, 0, 1).resetReached)
	require.Equal(t, "5h", svc.buildAssessment(account, config, 1, 1).triggerWindow)
}

func TestOpenAIQuotaAutoResetExpiryBoundaries(t *testing.T) {
	now := time.Date(2026, 11, 2, 12, 0, 0, 0, time.UTC)
	config := OpenAIAutoResetCreditConfig{ExpiryEnabled: true, ExpiryMinutes: 60}
	for _, tc := range []struct {
		name   string
		expiry string
		want   bool
	}{
		{"boundary", now.Add(time.Hour).Format(time.RFC3339), true},
		{"before", now.Add(time.Hour + time.Second).Format(time.RFC3339), false},
		{"inside", now.Add(time.Second).Format(time.RFC3339), true},
		{"expired", now.Format(time.RFC3339), false},
		{"invalid", "invalid", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			credits := &OpenAIRateLimitResetCredits{AvailableCount: 1, Credits: []OpenAIRateLimitResetCreditDetail{{ExpiresAt: tc.expiry}}}
			require.Equal(t, tc.want, openAIAutoResetExpiryReached(credits, config, now))
			credits.AvailableCount = 0
			require.False(t, openAIAutoResetExpiryReached(credits, config, now))
		})
	}
}

func newResetExpiryTestFixture(t *testing.T) (*OpenAIQuotaAutoResetService, *autoResetTestAccountRepo, *autoResetTestQuota) {
	t.Helper()
	svc, repo, quota := newResetThresholdTestFixture(t, nil, 1)
	repo.setExtraForTest(OpenAIAutoResetCredit5hDisabledExtraKey, true)
	repo.setExtraForTest(OpenAIAutoResetCredit7dDisabledExtraKey, true)
	repo.setExtraForTest(OpenAIAutoResetCreditExpiryEnabledExtraKey, true)
	repo.setExtraForTest(OpenAIAutoResetCreditExpiryMinutesExtraKey, 60)
	quota.usage.RateLimit.SecondaryWindow.UsedPercent = 0
	expiry := time.Now().Add(30 * time.Minute).UTC().Format(time.RFC3339)
	quota.usage.RateLimitResetCredits.Credits = []OpenAIRateLimitResetCreditDetail{{ExpiresAt: expiry}}
	quota.usage.autoResetCandidates = []openAIAutoResetCreditCandidate{{ID: "expiry-card", ExpiresAt: expiry}}
	repo.setExtraForTest(openaiQuotaResetCreditsKey, quota.usage.RateLimitResetCredits)
	return svc, repo, quota
}

func TestOpenAIQuotaAutoResetExpiryUsesCardWithoutUsageAndAcrossCycles(t *testing.T) {
	svc, repo, quota := newResetExpiryTestFixture(t)
	require.NoError(t, svc.evaluateAccount(context.Background(), 501))
	require.Equal(t, int32(1), quota.resetCalls.Load())
	require.Equal(t, "expiry", repo.stateForTest().TriggerWindow)
	// The upstream may briefly keep returning the same card after resetting both windows.
	quota.usage.RateLimit.SecondaryWindow.ResetAt += 86400
	repo.setExtraForTest(OpenAIAutoResetCreditStateExtraKey, nil)
	require.NoError(t, svc.evaluateAccount(context.Background(), 501))
	require.Equal(t, int32(1), quota.resetCalls.Load(), "quota cycle changes cannot redeem the same card again")
}

func TestOpenAIQuotaAutoResetExpiryRechecksCurrentPolicyAndFreshDetails(t *testing.T) {
	for _, mode := range []string{"disabled", "lead shortened", "expired", "no credit"} {
		t.Run(mode, func(t *testing.T) {
			svc, repo, quota := newResetExpiryTestFixture(t)
			quota.queryHook = func() {
				switch mode {
				case "disabled":
					repo.setExtraForTest(OpenAIAutoResetCreditExpiryEnabledExtraKey, false)
				case "lead shortened":
					repo.setExtraForTest(OpenAIAutoResetCreditExpiryMinutesExtraKey, 1)
				case "expired":
					quota.usage.RateLimitResetCredits.Credits[0].ExpiresAt = time.Now().Add(-time.Minute).Format(time.RFC3339)
				case "no credit":
					quota.usage.RateLimitResetCredits.AvailableCount = 0
				}
			}
			require.NoError(t, svc.evaluateAccount(context.Background(), 501))
			require.Zero(t, quota.resetCalls.Load())
		})
	}
}

func TestOpenAIQuotaAutoResetCandidateSkipsExpiredButPreservesUncertainCard(t *testing.T) {
	now := time.Now()
	candidates := []openAIAutoResetCreditCandidate{{ID: "expired", ExpiresAt: now.Add(-time.Minute).Format(time.RFC3339)}, {ID: "valid", ExpiresAt: now.Add(time.Hour).Format(time.RFC3339)}}
	got, err := selectOpenAIAutoResetCandidate(candidates, 2, nil, "cycle", now)
	require.NoError(t, err)
	require.Equal(t, "valid", got.ID)
	_, err = selectOpenAIAutoResetCandidate(candidates, 2, &OpenAIAutoResetCreditState{Status: OpenAIAutoResetStatusFailed, AttemptCreditHash: shortOpenAIAutoResetHash("missing"), AttemptCycleHash: "old-cycle"}, "new-cycle", now)
	require.Error(t, err)
}

func TestOpenAIQuotaAutoResetPreservesPendingAttemptAcrossScans(t *testing.T) {
	for _, mode := range []string{"cached", "fresh below threshold", "fresh no credit"} {
		t.Run(mode, func(t *testing.T) {
			svc, repo, quota := newResetExpiryTestFixture(t)
			pending := &OpenAIAutoResetCreditState{
				Status: OpenAIAutoResetStatusFailed, TriggerWindow: "expiry", AvailableCount: 1,
				CheckedAt:         time.Now().Format(time.RFC3339),
				AttemptCreditHash: shortOpenAIAutoResetHash("uncertain-card"), AttemptCycleHash: "legacy-cycle",
			}
			repo.setExtraForTest(OpenAIAutoResetCreditStateExtraKey, pending)
			repo.setExtraForTest(OpenAIAutoResetCreditExpiryEnabledExtraKey, false)
			if mode != "cached" {
				repo.setExtraForTest("codex_usage_updated_at", time.Now().Add(-time.Hour).Format(time.RFC3339))
			}
			if mode == "fresh no credit" {
				repo.setExtraForTest(OpenAIAutoResetCredit7dDisabledExtraKey, false)
				quota.usage.RateLimit.SecondaryWindow.UsedPercent = 100
				quota.usage.RateLimitResetCredits.AvailableCount = 0
			}
			require.NoError(t, svc.evaluateAccount(context.Background(), 501))
			state := repo.stateForTest()
			require.True(t, openAIAutoResetAttemptPending(state))
			require.Equal(t, pending.AttemptCreditHash, state.AttemptCreditHash)
			require.Equal(t, pending.AttemptCycleHash, state.AttemptCycleHash)
			if mode == "cached" {
				require.Zero(t, quota.queryCalls.Load())
			} else {
				require.Equal(t, int32(1), quota.queryCalls.Load())
			}
			repo.setExtraForTest(OpenAIAutoResetCreditExpiryEnabledExtraKey, true)
			quota.usage.RateLimitResetCredits.AvailableCount = 1
			require.Error(t, svc.evaluateAccount(context.Background(), 501), "a different card must not replace an unresolved attempt")
			require.Zero(t, quota.resetCalls.Load())
			require.Equal(t, pending.AttemptCycleHash, repo.stateForTest().AttemptCycleHash)
		})
	}
}

func TestOpenAIQuotaAutoResetCompletedCardDoesNotBlockNextCard(t *testing.T) {
	for _, mode := range []string{"cached scan", "query failure"} {
		t.Run(mode, func(t *testing.T) {
			svc, repo, quota := newResetExpiryTestFixture(t)
			require.NoError(t, svc.evaluateAccount(context.Background(), 501))
			require.Equal(t, int32(1), quota.resetCalls.Load())
			expiry := time.Now().Add(2 * time.Hour).UTC().Format(time.RFC3339)
			quota.usage.RateLimitResetCredits.Credits = []OpenAIRateLimitResetCreditDetail{{ExpiresAt: expiry}}
			quota.usage.autoResetCandidates = []openAIAutoResetCreditCandidate{{ID: "second-card", ExpiresAt: expiry}}
			repo.setExtraForTest(openaiQuotaResetCreditsKey, quota.usage.RateLimitResetCredits)
			if mode == "query failure" {
				repo.setExtraForTest("codex_usage_updated_at", time.Now().Add(-time.Hour).Format(time.RFC3339))
				quota.queryErr = errors.New("upstream unavailable")
				require.Error(t, svc.evaluateAccount(context.Background(), 501))
				state := repo.stateForTest()
				require.False(t, openAIAutoResetAttemptPending(state))
				state.LastResultAt = time.Now().Add(-2 * time.Minute).Format(time.RFC3339)
				repo.setExtraForTest(OpenAIAutoResetCreditStateExtraKey, state)
				quota.queryErr = nil
			} else {
				require.NoError(t, svc.evaluateAccount(context.Background(), 501))
				require.False(t, openAIAutoResetAttemptPending(repo.stateForTest()))
			}
			repo.setExtraForTest(OpenAIAutoResetCreditExpiryMinutesExtraKey, 180)
			require.NoError(t, svc.evaluateAccount(context.Background(), 501))
			require.Equal(t, int32(2), quota.resetCalls.Load())
			require.Equal(t, "second-card", quota.resetArgs[1][0])
		})
	}
}
