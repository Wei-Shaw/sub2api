package service

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// #6804: frequency-type 429s must use short backoff, never window cooldown.
func TestCnProviderResponseIsFrequencyLimit(t *testing.T) {
	require.True(t, cnProviderResponseIsFrequencyLimit([]byte(`{"error":{"code":"1302","message":"[1302][您的账户已达到速率限制，请您控制请求频率]"}}`)))
	require.True(t, cnProviderResponseIsFrequencyLimit([]byte(`rate_limit_error: frequency exceeded`)))
	require.False(t, cnProviderResponseIsFrequencyLimit([]byte(`{"error":{"code":"insufficient_quota","message":"quota exhausted"}}`)))
	require.False(t, cnProviderResponseIsFrequencyLimit(nil))
}

func TestCnProviderQuotaNearlyExhausted(t *testing.T) {
	mkAccount := func(extra map[string]any) *Account {
		return &Account{Extra: extra}
	}
	require.True(t, cnProviderQuotaNearlyExhausted(mkAccount(map[string]any{"zhipu_5h_used_percent": 97.0})))
	require.True(t, cnProviderQuotaNearlyExhausted(mkAccount(map[string]any{"codex_5h_used_percent": 95.0})))
	require.False(t, cnProviderQuotaNearlyExhausted(mkAccount(map[string]any{"zhipu_5h_used_percent": 7.0, "zhipu_weekly_used_percent": 1.0})))
	require.False(t, cnProviderQuotaNearlyExhausted(mkAccount(map[string]any{})))
	require.False(t, cnProviderQuotaNearlyExhausted(nil))
}
