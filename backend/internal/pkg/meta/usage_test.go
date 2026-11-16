//go:build unit

package meta

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestIsSubscriptionUsageEvent(t *testing.T) {
	typed := []byte(`{"type":"response.subscription_usage","subscription":{"tier":"1","window":{"resets_at":1792272519,"used_percent":10}}}`)
	require.True(t, IsSubscriptionUsageEvent(SubscriptionUsageEventType, typed))
	require.True(t, IsSubscriptionUsageEvent("", []byte(`{"subscription":{"tier":"1","window":{"resets_at":1,"used_percent":2}}}`)))
	require.False(t, IsSubscriptionUsageEvent("response.completed", []byte(`{"subscription":{"tier":"1"}}`)))
	require.False(t, IsSubscriptionUsageEvent("", []byte(`{"usage":{"input_tokens":1}}`)))
	require.False(t, IsSubscriptionUsageEvent("", []byte("not json")))
}

func TestParseSubscriptionUsage(t *testing.T) {
	valid := []byte(`{"type":"response.subscription_usage","subscription":{"tier":"1234","weekly":{"resets_at":1792368000,"used_percent":80},"window":{"resets_at":1792272519,"used_percent":10,"window_duration_mins":300}}}`)
	usage, ok := ParseSubscriptionUsage(valid)
	require.True(t, ok)
	require.NotNil(t, usage)
	require.Equal(t, "1234", usage.Tier)
	require.NotNil(t, usage.Window)
	require.Equal(t, int64(1792272519), usage.Window.ResetsAt)
	require.InDelta(t, 10.0, usage.Window.UsedPercent, 0.001)
	require.NotNil(t, usage.Weekly)
	require.InDelta(t, 80.0, usage.Weekly.UsedPercent, 0.001)

	_, ok = ParseSubscriptionUsage([]byte(`{"type":"response.subscription_usage"}`))
	require.False(t, ok)

	_, ok = ParseSubscriptionUsage([]byte(`{"type":"response.subscription_usage","subscription":{"window":{"resets_at":0,"used_percent":5}}}`))
	require.False(t, ok)

	_, ok = ParseSubscriptionUsage([]byte(`{"type":"response.subscription_usage","subscription":{"windows":{"resets_at":1}}}`))
	require.False(t, ok)
}
