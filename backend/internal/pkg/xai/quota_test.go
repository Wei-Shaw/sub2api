//go:build unit

package xai

import (
	"net/http"
	"testing"
	"time"

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

func TestParseQuotaHeadersAcceptsXAITierAliases(t *testing.T) {
	t.Parallel()

	headers := http.Header{}
	headers.Set("x-xai-user-tier", "supergrok-heavy")
	headers.Set("x-xai-user-entitlement-status", "enabled")

	snapshot := ParseQuotaHeaders(headers, http.StatusOK)
	require.NotNil(t, snapshot)
	require.True(t, snapshot.HeadersObserved)
	require.Equal(t, "supergrok-heavy", snapshot.SubscriptionTier)
	require.Equal(t, "enabled", snapshot.EntitlementStatus)
	require.Equal(t, "supergrok-heavy", snapshot.Headers["x-xai-user-tier"])
	require.Equal(t, "enabled", snapshot.Headers["x-xai-user-entitlement-status"])
}

func TestParseQuotaHeadersAcceptsRateLimitAliases(t *testing.T) {
	t.Parallel()

	headers := http.Header{}
	headers.Set("x-rate-limit-limit-tokens", "500000")
	headers.Set("x-rate-limit-remaining-tokens", "100")
	headers.Set("x-rate-limit-reset-tokens", "1893456000")

	snapshot := ParseQuotaHeaders(headers, http.StatusOK)
	require.NotNil(t, snapshot)
	require.NotNil(t, snapshot.Tokens)
	require.Equal(t, int64(500000), *snapshot.Tokens.Limit)
	require.Equal(t, int64(100), *snapshot.Tokens.Remaining)
	require.Equal(t, int64(1893456000), *snapshot.Tokens.ResetUnix)
	require.Contains(t, snapshot.Headers, "x-rate-limit-limit-tokens")
}

func TestParseResetHeaderRelativeSecondsNotMisreadAsEpoch(t *testing.T) {
	t.Parallel()

	headers := http.Header{}
	// xAI may return the reset window as a relative number of seconds ("60").
	// It must resolve to ~now+60s, NOT 1970-01-01 (epoch 60).
	headers.Set("x-ratelimit-reset-requests", "60")
	headers.Set("x-ratelimit-remaining-requests", "0")

	before := time.Now().Unix()
	snapshot := ParseQuotaHeaders(headers, http.StatusTooManyRequests)
	require.NotNil(t, snapshot)
	require.NotNil(t, snapshot.Requests)
	require.NotNil(t, snapshot.Requests.ResetUnix)
	got := *snapshot.Requests.ResetUnix
	require.GreaterOrEqual(t, got, before+59)
	require.LessOrEqual(t, got, time.Now().Unix()+61)
}

func TestParseResetHeaderDurationWindow(t *testing.T) {
	t.Parallel()

	headers := http.Header{}
	headers.Set("x-ratelimit-reset-requests", "6m0s")
	headers.Set("x-ratelimit-remaining-requests", "0")

	before := time.Now().Unix()
	snapshot := ParseQuotaHeaders(headers, http.StatusTooManyRequests)
	require.NotNil(t, snapshot)
	require.NotNil(t, snapshot.Requests)
	require.NotNil(t, snapshot.Requests.ResetUnix)
	got := *snapshot.Requests.ResetUnix
	require.GreaterOrEqual(t, got, before+359)
	require.LessOrEqual(t, got, time.Now().Unix()+361)
}

func TestParseResetHeaderSubsecondDurationCeilsToFutureSecond(t *testing.T) {
	t.Parallel()

	headers := http.Header{}
	headers.Set("x-rate-limit-reset-tokens", "250ms")
	headers.Set("x-rate-limit-remaining-tokens", "0")

	before := time.Now().Unix()
	snapshot := ParseQuotaHeaders(headers, http.StatusTooManyRequests)
	require.NotNil(t, snapshot)
	require.NotNil(t, snapshot.Tokens)
	require.NotNil(t, snapshot.Tokens.ResetUnix)
	require.GreaterOrEqual(t, *snapshot.Tokens.ResetUnix, before)
	require.LessOrEqual(t, *snapshot.Tokens.ResetUnix, time.Now().Unix()+2)
}

func TestParseResetHeaderMillisecondsEpochNormalized(t *testing.T) {
	t.Parallel()

	headers := http.Header{}
	headers.Set("x-ratelimit-reset-tokens", "1893456000000") // ms epoch
	headers.Set("x-ratelimit-remaining-tokens", "0")

	snapshot := ParseQuotaHeaders(headers, http.StatusTooManyRequests)
	require.NotNil(t, snapshot)
	require.NotNil(t, snapshot.Tokens)
	require.Equal(t, int64(1893456000), *snapshot.Tokens.ResetUnix)
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

func TestIsGrokFreeRolling24hTokenLimit(t *testing.T) {
	t.Parallel()

	require.True(t, IsGrokFreeRolling24hTokenLimit(GrokFreeRolling24hTokenLimit))
	require.True(t, IsGrokFreeRolling24hTokenLimit(500_000))
	require.True(t, IsGrokFreeRolling24hTokenLimit(1_000_000), "observed Free limit variants remain classifiable")
	require.True(t, IsGrokFreeRolling24hTokenLimit(2_000_000), "legacy snapshots remain classifiable")
	require.False(t, IsGrokFreeRolling24hTokenLimit(3_000_000))
}
