//go:build unit

package service

import (
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

func TestBuildGrokQuotaSnapshotUpdatesMarksOnlyFreeOAuth429(t *testing.T) {
	now := time.Now()
	snapshot := &xai.QuotaSnapshot{StatusCode: http.StatusTooManyRequests}
	tests := []struct {
		name    string
		account *Account
		pending bool
	}{
		{name: "free oauth", account: &Account{Platform: PlatformGrok, Type: AccountTypeOAuth}, pending: true},
		{name: "supergrok", account: &Account{Platform: PlatformGrok, Type: AccountTypeOAuth, Credentials: map[string]any{"subscription_tier": "SuperGrok"}}},
		{name: "api key", account: &Account{Platform: PlatformGrok, Type: AccountTypeAPIKey}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			updates, pending := buildGrokQuotaSnapshotUpdatesForResponse(tt.account, snapshot, now, nil)

			require.Equal(t, tt.pending, pending)
			require.Contains(t, updates, grokQuotaSnapshotExtraKey)
			if tt.pending {
				require.Equal(t, true, updates[GrokFreeRecoveryPendingExtraKey])
				require.Contains(t, updates, GrokFreeRecoveryNextProbeAtExtraKey)
			} else {
				require.NotContains(t, updates, GrokFreeRecoveryPendingExtraKey)
			}
		})
	}
}

func TestBuildGrokQuotaSnapshotUpdatesFreeExhaustedOverridesStalePaidMetadata(t *testing.T) {
	percent := 100.0
	now := time.Now()
	snapshot := &xai.QuotaSnapshot{StatusCode: http.StatusTooManyRequests}
	account := &Account{
		Platform: PlatformGrok,
		Type:     AccountTypeOAuth,
		Extra: map[string]any{
			grokBillingExtraKey: &xai.BillingSummary{UsagePercent: &percent, Plan: "SuperGrok"},
		},
	}
	body := []byte(`{"code":"subscription:free-usage-exhausted","error":"free usage exhausted"}`)

	updates, pending := buildGrokQuotaSnapshotUpdatesForResponse(account, snapshot, now, body)

	require.True(t, pending)
	require.Equal(t, true, updates[GrokFreeRecoveryPendingExtraKey])
	require.Contains(t, updates, GrokFreeRecoveryNextProbeAtExtraKey)

	_, pending = buildGrokQuotaSnapshotUpdatesForResponse(
		&Account{Platform: PlatformGrok, Type: AccountTypeAPIKey},
		snapshot,
		now,
		body,
	)
	require.False(t, pending, "API-key accounts must keep the normal 429 policy")

	_, pending = buildGrokQuotaSnapshotUpdatesForResponse(
		account,
		&xai.QuotaSnapshot{StatusCode: http.StatusForbidden},
		now,
		body,
	)
	require.False(t, pending, "the Free exhaustion code only applies to HTTP 429")
}

func TestGrokResponseIndicatesFreeUsageExhaustedRequiresExactStructuredCode(t *testing.T) {
	require.True(t, grokResponseIndicatesFreeUsageExhausted([]byte(`{"code":"subscription:free-usage-exhausted"}`)))
	require.True(t, grokResponseIndicatesFreeUsageExhausted([]byte(`{"error":{"code":"subscription:free-usage-exhausted"}}`)))
	require.True(t, grokResponseIndicatesFreeUsageExhausted([]byte(`{"code":"subscription-free-usage-exhausted"}`)))
	require.False(t, grokResponseIndicatesFreeUsageExhausted([]byte(`{"error":"subscription:free-usage-exhausted"}`)))
	require.False(t, grokResponseIndicatesFreeUsageExhausted([]byte(`not-json`)))
}

func TestAccountIsGrokFreeOrUnknownOAuthUsesPositivePaidEvidence(t *testing.T) {
	percent := 12.5
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
				"subscription_tier": "free",
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
			name:    "api key excluded",
			account: &Account{Platform: PlatformGrok, Type: AccountTypeAPIKey},
			want:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, tt.account.IsGrokFreeOrUnknownOAuth())
		})
	}
}
