//go:build unit

package service

import (
	"net/http"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/xai"
	"github.com/stretchr/testify/require"
)

func grokInt64PtrForTest(v int64) *int64 { return &v }
func grokIntPtrForTest(v int) *int       { return &v }

func TestGrokQuotaFetcherBuildUsageInfoUnknownUntilFirstSnapshot(t *testing.T) {
	t.Parallel()

	usage := NewGrokQuotaFetcher().BuildUsageInfo(&Account{Platform: PlatformGrok, Type: AccountTypeOAuth})
	require.Equal(t, "passive", usage.Source)
	require.Equal(t, "quota_unknown", usage.ErrorCode)
	require.Contains(t, usage.Error, "unknown until billing is probed")
}

func TestGrokQuotaFetcherUsesCredentialTierWhenBillingHasNoPlan(t *testing.T) {
	t.Parallel()

	account := &Account{
		Platform: PlatformGrok,
		Type:     AccountTypeOAuth,
		Credentials: map[string]any{
			"subscription_tier":  " FREE ",
			"entitlement_status": " active ",
		},
		Extra: map[string]any{
			grokBillingExtraKey: &xai.BillingSummary{
				PeriodType: "weekly",
				StatusCode: http.StatusOK,
				UpdatedAt:  "2030-01-01T00:00:00Z",
			},
		},
	}

	usage := NewGrokQuotaFetcher().BuildUsageInfo(account)

	require.NotNil(t, usage.GrokBilling)
	require.Equal(t, "FREE", usage.SubscriptionTier)
	require.Equal(t, "FREE", usage.SubscriptionTierRaw)
	require.Equal(t, "active", usage.GrokEntitlementStatus)
}

func TestGrokQuotaFetcherBuildUsageInfoFromSnapshot(t *testing.T) {
	t.Parallel()

	updatedAt := "2030-01-01T00:00:00Z"
	account := &Account{
		Platform: PlatformGrok,
		Type:     AccountTypeOAuth,
		Extra: map[string]any{
			grokQuotaSnapshotExtraKey: &xai.QuotaSnapshot{
				Requests: &xai.QuotaWindow{
					Limit:     grokInt64PtrForTest(100),
					Remaining: grokInt64PtrForTest(12),
					ResetAt:   updatedAt,
				},
				Tokens: &xai.QuotaWindow{
					Limit:     grokInt64PtrForTest(1000),
					Remaining: grokInt64PtrForTest(900),
				},
				RetryAfterSeconds: grokIntPtrForTest(30),
				SubscriptionTier:  "supergrok",
				EntitlementStatus: "active",
				StatusCode:        http.StatusTooManyRequests,
				LastProbeAt:       updatedAt,
				LastHeadersSeenAt: updatedAt,
				UpdatedAt:         updatedAt,
			},
		},
	}

	usage := NewGrokQuotaFetcher().BuildUsageInfo(account)
	require.Equal(t, "passive", usage.Source)
	require.Equal(t, "rate_limited", usage.ErrorCode)
	require.Equal(t, "observed", usage.GrokQuotaSnapshotState)
	require.Equal(t, "supergrok", usage.SubscriptionTier)
	require.Equal(t, "active", usage.GrokEntitlementStatus)
	require.Equal(t, int64(100), *usage.GrokRequestQuota.Limit)
	require.Equal(t, int64(12), *usage.GrokRequestQuota.Remaining)
	require.Equal(t, 30, *usage.GrokRetryAfterSeconds)
	require.NotNil(t, usage.UpdatedAt)
	require.Equal(t, updatedAt, usage.GrokLastQuotaProbeAt)
	require.Equal(t, updatedAt, usage.GrokLastHeadersSeenAt)
	require.Equal(t, http.StatusTooManyRequests, usage.GrokLastStatusCode)
	require.True(t, usage.UpdatedAt.Equal(time.Date(2030, 1, 1, 0, 0, 0, 0, time.UTC)))
}

func TestGrokQuotaFetcherSnapshotErrorOverridesSuccessfulBillingStatus(t *testing.T) {
	t.Parallel()

	updatedAt := "2030-01-01T00:00:00Z"
	account := &Account{
		Platform: PlatformGrok,
		Type:     AccountTypeOAuth,
		Extra: map[string]any{
			grokBillingExtraKey: &xai.BillingSummary{
				PeriodType: "weekly",
				StatusCode: http.StatusOK,
				UpdatedAt:  updatedAt,
			},
			grokQuotaSnapshotExtraKey: &xai.QuotaSnapshot{
				StatusCode: http.StatusTooManyRequests,
				UpdatedAt:  updatedAt,
			},
		},
	}

	usage := NewGrokQuotaFetcher().BuildUsageInfo(account)

	require.Equal(t, "rate_limited", usage.ErrorCode)
	require.Equal(t, http.StatusTooManyRequests, usage.GrokLastStatusCode)
}

func TestGrokQuotaFetcherNewerSuccessfulActiveProbeClearsBillingForbidden(t *testing.T) {
	t.Parallel()

	billingAt := "2030-01-01T00:00:00Z"
	probeAt := "2030-01-01T00:05:00Z"
	account := &Account{
		Platform: PlatformGrok,
		Type:     AccountTypeOAuth,
		Credentials: map[string]any{
			"entitlement_status": "forbidden",
		},
		Extra: map[string]any{
			grokBillingExtraKey: &xai.BillingSummary{
				StatusCode: http.StatusForbidden,
				UpdatedAt:  billingAt,
			},
			grokQuotaSnapshotExtraKey: &xai.QuotaSnapshot{
				StatusCode:        http.StatusOK,
				ObservationSource: "active_probe",
				LastProbeAt:       probeAt,
				UpdatedAt:         probeAt,
			},
		},
	}

	usage := NewGrokQuotaFetcher().BuildUsageInfo(account)

	require.False(t, usage.IsForbidden)
	require.Empty(t, usage.ForbiddenType)
	require.Empty(t, usage.ErrorCode)
	require.Empty(t, usage.GrokEntitlementStatus)
	require.Equal(t, http.StatusOK, usage.GrokLastStatusCode)
	require.Equal(t, probeAt, usage.GrokLastQuotaProbeAt)
	require.NotNil(t, usage.UpdatedAt)
	require.True(t, usage.UpdatedAt.Equal(time.Date(2030, 1, 1, 0, 5, 0, 0, time.UTC)))
}

func TestGrokQuotaFetcherSameSecondSuccessfulActiveProbeClearsBillingForbidden(t *testing.T) {
	t.Parallel()

	observedAt := "2030-01-01T00:05:00Z"
	account := &Account{
		Platform: PlatformGrok,
		Type:     AccountTypeOAuth,
		Extra: map[string]any{
			grokBillingExtraKey: &xai.BillingSummary{
				StatusCode: http.StatusForbidden,
				UpdatedAt:  observedAt,
			},
			grokQuotaSnapshotExtraKey: &xai.QuotaSnapshot{
				StatusCode:        http.StatusOK,
				ObservationSource: "active_probe",
				LastProbeAt:       observedAt,
				UpdatedAt:         observedAt,
			},
		},
	}

	usage := NewGrokQuotaFetcher().BuildUsageInfo(account)

	require.False(t, usage.IsForbidden)
	require.Empty(t, usage.ForbiddenType)
	require.Empty(t, usage.ErrorCode)
	require.Equal(t, http.StatusOK, usage.GrokLastStatusCode)
}

func TestGrokQuotaFetcherDoesNotClearBillingForbiddenWithoutNewerSuccessfulActiveProbe(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		snapshot xai.QuotaSnapshot
	}{
		{
			name: "older active probe",
			snapshot: xai.QuotaSnapshot{
				StatusCode:        http.StatusOK,
				ObservationSource: "active_probe",
				LastProbeAt:       "2030-01-01T00:04:59Z",
				UpdatedAt:         "2030-01-01T00:04:59Z",
			},
		},
		{
			name: "newer passive response",
			snapshot: xai.QuotaSnapshot{
				StatusCode:        http.StatusOK,
				ObservationSource: "upstream_response",
				UpdatedAt:         "2030-01-01T00:05:01Z",
			},
		},
		{
			name: "newer failed active probe",
			snapshot: xai.QuotaSnapshot{
				StatusCode:        http.StatusTooManyRequests,
				ObservationSource: "active_probe",
				LastProbeAt:       "2030-01-01T00:05:01Z",
				UpdatedAt:         "2030-01-01T00:05:01Z",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			account := &Account{
				Platform: PlatformGrok,
				Type:     AccountTypeOAuth,
				Extra: map[string]any{
					grokBillingExtraKey: &xai.BillingSummary{
						StatusCode: http.StatusForbidden,
						UpdatedAt:  "2030-01-01T00:05:00Z",
					},
					grokQuotaSnapshotExtraKey: tt.snapshot,
				},
			}

			usage := NewGrokQuotaFetcher().BuildUsageInfo(account)

			require.True(t, usage.IsForbidden)
			require.Equal(t, "forbidden", usage.ForbiddenType)
			require.Equal(t, "forbidden", usage.ErrorCode)
		})
	}
}

func TestGrokQuotaFetcherBuildUsageInfoFromNoHeadersProbe(t *testing.T) {
	t.Parallel()

	probedAt := "2030-01-01T00:00:00Z"
	account := &Account{
		Platform: PlatformGrok,
		Type:     AccountTypeOAuth,
		Extra: map[string]any{
			grokQuotaSnapshotExtraKey: xai.QuotaSnapshot{
				StatusCode:        http.StatusOK,
				HeadersObserved:   false,
				ObservationSource: "active_probe",
				LastProbeAt:       probedAt,
				UpdatedAt:         probedAt,
			},
		},
	}

	usage := NewGrokQuotaFetcher().BuildUsageInfo(account)
	require.Equal(t, "quota_unknown", usage.ErrorCode)
	require.Equal(t, "no_headers", usage.GrokQuotaSnapshotState)
	require.Contains(t, usage.Error, "No xAI quota headers observed")
	require.Equal(t, probedAt, usage.GrokLastQuotaProbeAt)
	require.Empty(t, usage.GrokLastHeadersSeenAt)
	require.Equal(t, http.StatusOK, usage.GrokLastStatusCode)
	require.Nil(t, usage.GrokRequestQuota)
	require.Nil(t, usage.GrokTokenQuota)
}

func TestGrokQuotaFetcherClassifiesForbiddenAndReauth(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		statusCode  int
		wantReauth  bool
		wantForbid  bool
		wantCode    string
		wantEntitle string
	}{
		{name: "reauth", statusCode: http.StatusUnauthorized, wantReauth: true, wantCode: "unauthenticated"},
		{name: "forbidden", statusCode: http.StatusForbidden, wantForbid: true, wantCode: "forbidden", wantEntitle: "forbidden"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			account := &Account{
				Platform: PlatformGrok,
				Type:     AccountTypeOAuth,
				Extra: map[string]any{
					grokQuotaSnapshotExtraKey: xai.QuotaSnapshot{
						StatusCode:      tt.statusCode,
						HeadersObserved: true,
						UpdatedAt:       time.Now().UTC().Format(time.RFC3339),
					},
				},
			}
			usage := NewGrokQuotaFetcher().BuildUsageInfo(account)
			require.Equal(t, tt.wantReauth, usage.NeedsReauth)
			require.Equal(t, tt.wantForbid, usage.IsForbidden)
			require.Equal(t, tt.wantCode, usage.ErrorCode)
			require.Equal(t, tt.wantEntitle, usage.GrokEntitlementStatus)
		})
	}
}
