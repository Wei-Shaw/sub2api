//go:build unit

package service

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestHandleGrok429MarksFreeOAuthPendingUntilProbe(t *testing.T) {
	account := &Account{
		ID:          801,
		Platform:    PlatformGrok,
		Type:        AccountTypeOAuth,
		Status:      StatusActive,
		Schedulable: true,
	}
	repo := &grokQuotaAccountRepo{}
	svc := &OpenAIGatewayService{accountRepo: repo}
	before := time.Now()

	svc.handleGrokAccountUpstreamError(
		context.Background(),
		account,
		http.StatusTooManyRequests,
		nil,
		nil,
	)

	require.Equal(t, 1, repo.updateCalls)
	updates := repo.updates[account.ID]
	require.Equal(t, true, updates[GrokFreeRecoveryPendingExtraKey])
	require.Contains(t, updates, grokQuotaSnapshotExtraKey)

	nextProbeRaw, ok := updates[GrokFreeRecoveryNextProbeAtExtraKey].(string)
	require.True(t, ok)
	nextProbeAt, err := time.Parse(time.RFC3339Nano, nextProbeRaw)
	require.NoError(t, err)
	require.WithinDuration(t, before.Add(5*time.Minute), nextProbeAt, time.Second)

	require.Equal(t, 1, repo.rateLimitedCalls)
	require.False(t, repo.lastRateLimitResetAt.Before(before.Add(10*time.Minute-time.Second)))
	require.True(t, svc.isOpenAIAccountRuntimeBlocked(account))
}

func TestHandleGrok429DoesNotEnrollPaidOAuthOrAPIKeyInFreeRecovery(t *testing.T) {
	tests := []struct {
		name    string
		account *Account
	}{
		{
			name: "supergrok oauth",
			account: &Account{
				ID:       802,
				Platform: PlatformGrok,
				Type:     AccountTypeOAuth,
				Credentials: map[string]any{
					"subscription_tier": "SuperGrok",
				},
			},
		},
		{
			name: "api key",
			account: &Account{
				ID:       803,
				Platform: PlatformGrok,
				Type:     AccountTypeAPIKey,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &grokQuotaAccountRepo{}
			svc := &OpenAIGatewayService{accountRepo: repo}

			svc.handleGrokAccountUpstreamError(
				context.Background(),
				tt.account,
				http.StatusTooManyRequests,
				nil,
				nil,
			)

			require.Equal(t, 1, repo.updateCalls)
			updates := repo.updates[tt.account.ID]
			require.NotContains(t, updates, GrokFreeRecoveryPendingExtraKey)
			require.NotContains(t, updates, GrokFreeRecoveryNextProbeAtExtraKey)
			require.Equal(t, 1, repo.rateLimitedCalls)
		})
	}
}

func TestAccountIsSchedulableRejectsPendingGrokFreeRecovery(t *testing.T) {
	account := &Account{
		ID:          804,
		Platform:    PlatformGrok,
		Type:        AccountTypeOAuth,
		Status:      StatusActive,
		Schedulable: true,
		Extra: map[string]any{
			GrokFreeRecoveryPendingExtraKey: true,
		},
	}

	require.True(t, account.IsGrokFreeRecoveryPending())
	require.False(t, account.IsSchedulable())
}
