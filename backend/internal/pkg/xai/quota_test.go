//go:build unit

package xai

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestParseQuotaHeaders(t *testing.T) {
	t.Parallel()

	headers := http.Header{}
	headers.Set("x-ratelimit-limit-requests", "100")
	headers.Set("x-ratelimit-remaining-requests", "25")
	headers.Set("x-ratelimit-reset-requests", "1893456000")
	headers.Set("x-ratelimit-limit-tokens", "1000000")
	headers.Set("x-ratelimit-remaining-tokens", "750000")
	headers.Set("retry-after", "60")
	headers.Set("xai-subscription-tier", "supergrok")
	headers.Set("xai-entitlement-status", "active")
	headers.Set("authorization", "should-not-be-copied")

	snapshot := ParseQuotaHeaders(headers, http.StatusTooManyRequests)
	require.NotNil(t, snapshot)
	require.Equal(t, http.StatusTooManyRequests, snapshot.StatusCode)
	require.True(t, snapshot.HeadersObserved)
	require.NotEmpty(t, snapshot.LastHeadersSeenAt)
	require.Equal(t, int64(100), *snapshot.Requests.Limit)
	require.Equal(t, int64(25), *snapshot.Requests.Remaining)
	require.Equal(t, int64(1893456000), *snapshot.Requests.ResetUnix)
	require.Equal(t, "2030-01-01T00:00:00Z", snapshot.Requests.ResetAt)
	require.Equal(t, int64(1000000), *snapshot.Tokens.Limit)
	require.Equal(t, int64(750000), *snapshot.Tokens.Remaining)
	require.Equal(t, 60, *snapshot.RetryAfterSeconds)
	require.Equal(t, "supergrok", snapshot.SubscriptionTier)
	require.Equal(t, "active", snapshot.EntitlementStatus)
	require.Contains(t, snapshot.Headers, "x-ratelimit-limit-requests")
	require.NotContains(t, snapshot.Headers, "authorization")
}

func TestParseQuotaHeadersCredits(t *testing.T) {
	t.Parallel()

	headers := http.Header{}
	headers.Set("xai-monthly-credits-remaining", "3.5")
	headers.Set("xai-monthly-credits-limit", "10")
	headers.Set("xai-pay-as-you-go-credits-limit", "25")
	headers.Set("xai-prepaid-credit-balance", "12.75")

	snapshot := ParseQuotaHeaders(headers, http.StatusOK)
	require.NotNil(t, snapshot)
	require.True(t, snapshot.HeadersObserved)
	require.Len(t, snapshot.Credits, 3)
	require.Equal(t, "monthly_credits", snapshot.Credits[0].CreditType)
	require.Equal(t, 3.5, *snapshot.Credits[0].Remaining)
	require.Equal(t, 10.0, *snapshot.Credits[0].Limit)
	require.Equal(t, "pay_as_you_go", snapshot.Credits[1].CreditType)
	require.Equal(t, 25.0, *snapshot.Credits[1].Limit)
	require.Equal(t, "prepaid_credits", snapshot.Credits[2].CreditType)
	require.Equal(t, 12.75, *snapshot.Credits[2].Amount)
}

func TestParseQuotaHeadersReturnsNilForMissingHeaders(t *testing.T) {
	t.Parallel()

	require.Nil(t, ParseQuotaHeaders(http.Header{}, http.StatusOK))
}

func TestObserveQuotaHeadersRecordsNoHeaderProbe(t *testing.T) {
	t.Parallel()

	snapshot := ObserveQuotaHeaders(http.Header{}, http.StatusOK, "active_probe")
	require.NotNil(t, snapshot)
	require.False(t, snapshot.HeadersObserved)
	require.Equal(t, http.StatusOK, snapshot.StatusCode)
	require.Equal(t, "active_probe", snapshot.ObservationSource)
	require.NotEmpty(t, snapshot.LastProbeAt)
	require.Empty(t, snapshot.LastHeadersSeenAt)
	require.Empty(t, snapshot.Headers)
	require.Nil(t, snapshot.Requests)
	require.Nil(t, snapshot.Tokens)
}
