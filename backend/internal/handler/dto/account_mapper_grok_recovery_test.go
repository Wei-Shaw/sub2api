package dto

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestAccountFromServiceShallow_ExposesGrokFreeRecoveryPending(t *testing.T) {
	nextProbeAt := time.Date(2026, 7, 18, 1, 5, 0, 0, time.UTC)
	lastProbeAt := time.Date(2026, 7, 18, 1, 0, 0, 0, time.UTC)
	src := &service.Account{
		ID:       42,
		Platform: service.PlatformGrok,
		Type:     service.AccountTypeOAuth,
		Extra: map[string]any{
			service.GrokFreeRecoveryPendingExtraKey:         true,
			service.GrokFreeRecoveryNextProbeAtExtraKey:     nextProbeAt.Format(time.RFC3339Nano),
			service.GrokFreeRecoveryLastResultAtExtraKey:    lastProbeAt.Format(time.RFC3339Nano),
			service.GrokFreeRecoveryLastProbeResultExtraKey: "http_429",
		},
	}

	got := AccountFromServiceShallow(src)
	require.NotNil(t, got)
	require.True(t, got.GrokFreeRecoveryPending)
	require.Equal(t, nextProbeAt, *got.GrokFreeRecoveryNextProbeAt)
	require.Equal(t, lastProbeAt, *got.GrokFreeRecoveryLastProbeAt)
	require.Equal(t, "http_429", got.GrokFreeRecoveryLastProbeResult)

	raw, err := json.Marshal(got)
	require.NoError(t, err)
	require.Contains(t, string(raw), `"grok_free_recovery_pending":true`)
	require.Contains(t, string(raw), `"grok_free_recovery_next_probe_at":"2026-07-18T01:05:00Z"`)
	require.Contains(t, string(raw), `"grok_free_recovery_last_probe_result":"http_429"`)
}

func TestAccountFromServiceShallow_ExposesFalseWhenGrokRecoveryIsNotPending(t *testing.T) {
	got := AccountFromServiceShallow(&service.Account{
		ID:       43,
		Platform: service.PlatformGrok,
		Type:     service.AccountTypeOAuth,
	})

	require.NotNil(t, got)
	require.False(t, got.GrokFreeRecoveryPending)

	raw, err := json.Marshal(got)
	require.NoError(t, err)
	require.Contains(t, string(raw), `"grok_free_recovery_pending":false`)
}
