package dto

import (
	"encoding/json"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestAccountFromServiceShallow_ExposesGrokFreeRecoveryPending(t *testing.T) {
	src := &service.Account{
		ID:       42,
		Platform: service.PlatformGrok,
		Type:     service.AccountTypeOAuth,
		Extra: map[string]any{
			service.GrokFreeRecoveryPendingExtraKey: true,
		},
	}

	got := AccountFromServiceShallow(src)
	require.NotNil(t, got)
	require.True(t, got.GrokFreeRecoveryPending)

	raw, err := json.Marshal(got)
	require.NoError(t, err)
	require.Contains(t, string(raw), `"grok_free_recovery_pending":true`)
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
