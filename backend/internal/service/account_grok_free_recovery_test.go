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

func TestAccountGrokFreeRecoveryPendingBlocksSchedulingAndReportsRateLimited(t *testing.T) {
	nextProbeAt := time.Now().Add(5 * time.Minute).UTC().Truncate(time.Second)
	account := &Account{
		Platform:    PlatformGrok,
		Type:        AccountTypeOAuth,
		Status:      StatusActive,
		Schedulable: true,
		Extra: map[string]any{
			GrokFreeRecoveryPendingExtraKey:     true,
			GrokFreeRecoveryNextProbeAtExtraKey: nextProbeAt.Format(time.RFC3339Nano),
		},
	}

	require.True(t, account.IsGrokFreeRecoveryPending())
	require.False(t, account.IsSchedulable())
	require.True(t, account.IsRateLimited())
	require.Equal(t, nextProbeAt, account.GrokFreeRecoveryNextProbeAt())
}

func TestAccountIsGrokFreeOrUnknownOAuthUsesPositivePaidEvidence(t *testing.T) {
	percent := 12.5
	monthlyLimit := 100.0
	tests := []struct {
		name    string
		account *Account
		want    bool
	}{
		{
			name:    "unknown oauth fails closed as free",
			account: &Account{Platform: PlatformGrok, Type: AccountTypeOAuth},
			want:    true,
		},
		{
			name: "explicit free tier",
			account: &Account{Platform: PlatformGrok, Type: AccountTypeOAuth, Credentials: map[string]any{
				"subscription_tier":  " FREE ",
				"entitlement_status": "active",
			}},
			want: true,
		},
		{
			name: "credential supergrok",
			account: &Account{Platform: PlatformGrok, Type: AccountTypeOAuth, Credentials: map[string]any{
				"subscription_tier": "SuperGrok Heavy",
			}},
			want: false,
		},
		{
			name: "quota snapshot paid tier",
			account: &Account{Platform: PlatformGrok, Type: AccountTypeOAuth, Extra: map[string]any{
				grokQuotaSnapshotExtraKey: &xai.QuotaSnapshot{SubscriptionTier: "premium"},
			}},
			want: false,
		},
		{
			name: "billing usage percent is paid evidence",
			account: &Account{Platform: PlatformGrok, Type: AccountTypeOAuth, Extra: map[string]any{
				grokBillingExtraKey: &xai.BillingSummary{UsagePercent: &percent},
			}},
			want: false,
		},
		{
			name: "billing monthly limit is paid evidence",
			account: &Account{Platform: PlatformGrok, Type: AccountTypeOAuth, Extra: map[string]any{
				grokBillingExtraKey: &xai.BillingSummary{MonthlyLimitCents: &monthlyLimit},
			}},
			want: false,
		},
		{
			name:    "api key excluded",
			account: &Account{Platform: PlatformGrok, Type: AccountTypeAPIKey},
			want:    false,
		},
		{
			name:    "other platform excluded",
			account: &Account{Platform: PlatformOpenAI, Type: AccountTypeOAuth},
			want:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, tt.account.IsGrokFreeOrUnknownOAuth())
		})
	}
}

func TestHandleGrok429PaidOAuthPreservesUpstreamReset(t *testing.T) {
	account := &Account{
		ID:       805,
		Platform: PlatformGrok,
		Type:     AccountTypeOAuth,
		Credentials: map[string]any{
			"subscription_tier": "SuperGrok",
		},
	}
	repo := &grokQuotaAccountRepo{}
	svc := &OpenAIGatewayService{accountRepo: repo}
	before := time.Now()

	svc.handleGrokAccountUpstreamError(
		context.Background(),
		account,
		http.StatusTooManyRequests,
		http.Header{"Retry-After": []string{"45"}},
		nil,
	)

	require.WithinDuration(t, before.Add(45*time.Second), repo.lastRateLimitResetAt, time.Second)
	require.NotContains(t, repo.updates[account.ID], GrokFreeRecoveryPendingExtraKey)
	require.NotContains(t, repo.updates[account.ID], GrokFreeRecoveryNextProbeAtExtraKey)
}
