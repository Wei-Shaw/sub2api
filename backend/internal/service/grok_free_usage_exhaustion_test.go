package service

import (
	"net/http"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/xai"
	"github.com/stretchr/testify/require"
)

const grokFreeUsageExhaustedTestBody = `{"code":"subscription:free-usage-exhausted","error":"You've used all the included free usage for model grok-4.5 for now. Usage resets over a rolling 24-hour window - tokens (actual/limit): 880175/500000."}`

func TestParseGrokFreeUsageExhaustion(t *testing.T) {
	result, exhausted := parseGrokFreeUsageExhaustion(http.StatusTooManyRequests, []byte(grokFreeUsageExhaustedTestBody))

	require.True(t, exhausted)
	require.True(t, result.hasTokenPair)
	require.EqualValues(t, 880_175, result.actual)
	require.EqualValues(t, xai.GrokFreeRolling24hTokenLimit, result.limit)
}

func TestParseGrokFreeUsageExhaustionRejectsGeneric429(t *testing.T) {
	_, exhausted := parseGrokFreeUsageExhaustion(
		http.StatusTooManyRequests,
		[]byte(`{"error":{"message":"rate limit exceeded"}}`),
	)

	require.False(t, exhausted)
}

func TestParseGrokQuotaSnapshotWithBodyUses24HourCooldown(t *testing.T) {
	now := time.Date(2026, time.August, 6, 3, 0, 0, 0, time.UTC)
	snapshot := parseGrokQuotaSnapshotWithBody(nil, http.StatusTooManyRequests, []byte(grokFreeUsageExhaustedTestBody), now)

	require.NotNil(t, snapshot)
	require.Equal(t, "free", snapshot.SubscriptionTier)
	require.Equal(t, "upstream_error_body", snapshot.ObservationSource)
	require.NotNil(t, snapshot.Tokens)
	require.NotNil(t, snapshot.Tokens.Limit)
	require.NotNil(t, snapshot.Tokens.Remaining)
	require.NotNil(t, snapshot.Tokens.ResetUnix)
	require.EqualValues(t, xai.GrokFreeRolling24hTokenLimit, *snapshot.Tokens.Limit)
	require.Zero(t, *snapshot.Tokens.Remaining)
	require.Equal(t, now.Add(24*time.Hour).Unix(), *snapshot.Tokens.ResetUnix)
}
