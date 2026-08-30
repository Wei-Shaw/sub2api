//go:build unit

package service

import (
	"context"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/pkg/ctxkey"
	"github.com/stretchr/testify/require"
)

func TestResolveUsageBillingRequestID_ForcedWebSearchBeatsClientID(t *testing.T) {
	t.Parallel()
	ctx := context.WithValue(context.Background(), ctxkey.ClientRequestID, "client-shared-id")
	got := resolveUsageBillingRequestID(ctx, "web_search:uuid-1")
	require.Equal(t, "web_search:uuid-1", got)
}

func TestResolveUsageBillingRequestID_ClientWinsOverPlainUpstream(t *testing.T) {
	t.Parallel()
	ctx := context.WithValue(context.Background(), ctxkey.ClientRequestID, "client-shared-id")
	got := resolveUsageBillingRequestID(ctx, "resp_abc")
	require.Equal(t, "client:client-shared-id", got)
}

func TestIsForcedUsageBillingRequestID(t *testing.T) {
	t.Parallel()
	require.True(t, isForcedUsageBillingRequestID("web_search:x"))
	require.True(t, isForcedUsageBillingRequestID("grok-video:task-1"))
	require.True(t, isForcedUsageBillingRequestID("grok_audio:up-1"))
	require.True(t, isForcedUsageBillingRequestID("grok_realtime:sess-1"))
	require.False(t, isForcedUsageBillingRequestID("resp_abc"))
}

func TestStableGrokAudioBillingRequestID(t *testing.T) {
	t.Parallel()
	require.Equal(t, "grok_audio:up-1", StableGrokAudioBillingRequestID("up-1"))
	require.Equal(t, "grok_audio:up-1", StableGrokAudioBillingRequestID("grok_audio:up-1"))
	got := StableGrokAudioBillingRequestID("")
	require.True(t, strings.HasPrefix(got, "grok_audio:"))
	require.Greater(t, len(got), len("grok_audio:"))
}

func TestStableGrokRealtimeBillingRequestID(t *testing.T) {
	t.Parallel()
	require.Equal(t, "grok_realtime:s1", StableGrokRealtimeBillingRequestID("s1"))
	require.Equal(t, "grok_realtime:s1", StableGrokRealtimeBillingRequestID("grok_realtime:s1"))
	got := StableGrokRealtimeBillingRequestID("")
	require.True(t, strings.HasPrefix(got, "grok_realtime:"))
}

func TestResolveUsageBillingRequestID_ForcedGrokAudioBeatsClientID(t *testing.T) {
	t.Parallel()
	ctx := context.WithValue(context.Background(), ctxkey.ClientRequestID, "client-shared-id")
	got := resolveUsageBillingRequestID(ctx, StableGrokAudioBillingRequestID("up-9"))
	require.Equal(t, "grok_audio:up-9", got)
}

func TestUsageUpstreamRequestIDPtr(t *testing.T) {
	t.Parallel()

	// 真实上游响应头 ID 原样落库（含首尾空白清理）。
	got := usageUpstreamRequestIDPtr("  req_011CRJabc  ", false)
	require.NotNil(t, got)
	require.Equal(t, "req_011CRJabc", *got)

	// 空值与本地合成的 label:uuid 形态一律不落。
	require.Nil(t, usageUpstreamRequestIDPtr("", false))
	require.Nil(t, usageUpstreamRequestIDPtr("   ", false))
	require.Nil(t, usageUpstreamRequestIDPtr("web_search:uuid-1", false))
	require.Nil(t, usageUpstreamRequestIDPtr("x_search:uuid-2", false))
	require.Nil(t, usageUpstreamRequestIDPtr("grok_audio:up-1", false))
	require.Nil(t, usageUpstreamRequestIDPtr("grok-video:task-1", false))
	require.Nil(t, usageUpstreamRequestIDPtr("grok_realtime:sess-1", false))

	// WS 轮次的 RequestID 是上游 response id，不是请求标识。
	require.Nil(t, usageUpstreamRequestIDPtr("resp_abc", true))

	// 超长防御性截断到列宽，避免整行插入失败。
	long := strings.Repeat("a", maxUsageUpstreamRequestIDLen+10)
	truncated := usageUpstreamRequestIDPtr(long, false)
	require.NotNil(t, truncated)
	require.Len(t, *truncated, maxUsageUpstreamRequestIDLen)
}
