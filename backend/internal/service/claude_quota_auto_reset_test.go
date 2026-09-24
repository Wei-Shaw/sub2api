package service

import (
	"github.com/stretchr/testify/require"
	"testing"
	"time"
)

func TestClaudeResetConfigOptIn(t *testing.T) {
	a := &Account{Platform: PlatformAnthropic, Type: AccountTypeOAuth}
	require.False(t, resolveClaudeAutoResetConfig(a).Enabled)
	extra, err := normalizeOpenAIAutoResetCreditExtra(PlatformAnthropic, AccountTypeOAuth, false, map[string]any{"claude_auto_reset_credit_enabled": true, "claude_auto_reset_credit_mode": "expiring_or_exhausted"})
	require.NoError(t, err)
	a.Extra = extra
	cfg := resolveClaudeAutoResetConfig(a)
	require.True(t, cfg.Enabled)
	require.Equal(t, "expiring_or_exhausted", cfg.Mode)
	require.Equal(t, 3600, cfg.ExpiryHorizonSeconds)
	_, err = normalizeOpenAIAutoResetCreditExtra(PlatformAnthropic, AccountTypeOAuth, false, map[string]any{"claude_auto_reset_credit_mode": "threshold"})
	require.Error(t, err)
}
func TestClaudeResetNativeModelWindows(t *testing.T) {
	now := time.Now()
	u := &ClaudeUsageResponse{}
	u.SevenDaySonnet.Utilization = 100
	u.SevenDaySonnet.ResetsAt = now.Add(time.Hour).Format(time.RFC3339)
	require.True(t, claudeUsageExhausted(u, "claude-sonnet-4-6", now))
	require.False(t, claudeUsageExhausted(u, "claude-opus-4-6", now))
	u.FiveHour.Utilization = 100
	u.FiveHour.ResetsAt = now.Add(time.Hour).Format(time.RFC3339)
	require.True(t, claudeUsageExhausted(u, "claude-opus-4-6", now))
	u.FiveHour.ResetsAt = now.Add(-time.Second).Format(time.RFC3339)
	require.False(t, claudeUsageExhausted(u, "claude-opus-4-6", now))
	require.False(t, claudeUsageExhausted(nil, "claude-opus-4-6", now))
}
