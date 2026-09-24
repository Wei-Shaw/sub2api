package service

import (
	"context"
	"errors"
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

func TestClaudeResetMustClearAllRequestBlocks(t *testing.T) {
	now := time.Now()
	u := &ClaudeUsageResponse{}
	u.FiveHour.Utilization = 100
	u.FiveHour.ResetsAt = now.Add(time.Hour).Format(time.RFC3339)
	u.SevenDay.Utilization = 100
	u.SevenDay.ResetsAt = now.Add(24 * time.Hour).Format(time.RFC3339)
	require.False(t, claudeGrantClearsAllBlocks(u, []string{"five_hour"}, "claude-opus-4-6", now))
	require.True(t, claudeGrantClearsAllBlocks(u, []string{"five_hour", "seven_day"}, "claude-opus-4-6", now))
	u.SevenDaySonnet.Utilization = 100
	u.SevenDaySonnet.ResetsAt = now.Add(time.Hour).Format(time.RFC3339)
	require.True(t, claudeGrantClearsAllBlocks(u, []string{"five_hour", "seven_day"}, "claude-opus-4-6", now))
	require.False(t, claudeGrantClearsAllBlocks(u, []string{"five_hour", "seven_day"}, "claude-sonnet-4-6", now))
}

type claudeRecoveryRepo struct {
	*claudeAutoIntegrationRepo
	clears         int
	limited, reset time.Time
}

func (r *claudeRecoveryRepo) ClearClaudeRateLimitIfObserved(_ context.Context, _ int64, limited, reset time.Time) (bool, error) {
	r.clears++
	r.limited = limited
	r.reset = reset
	return true, nil
}

type claudeRecoveryUsage struct {
	usage *ClaudeUsageResponse
	fail  bool
}

func (f claudeRecoveryUsage) FetchUsage(context.Context, string, string) (*ClaudeUsageResponse, error) {
	if f.fail {
		return nil, errors.New("network")
	}
	return f.usage, nil
}
func (f claudeRecoveryUsage) FetchUsageWithOptions(ctx context.Context, _ *ClaudeUsageFetchOptions) (*ClaudeUsageResponse, error) {
	return f.FetchUsage(ctx, "", "")
}
func TestClaudeResetRecoveryVerifiedWindowOnly(t *testing.T) {
	for _, tc := range []struct {
		name      string
		remaining bool
		unrelated bool
		failure   bool
		want      bool
	}{{"verified", false, false, false, true}, {"other quota blocked", true, false, false, false}, {"different cooldown", false, true, false, false}, {"unknown", false, false, true, false}} {
		t.Run(tc.name, func(t *testing.T) {
			w, _ := newClaudeAutoIntegration(t, true, 75)
			baseRepo, ok := w.accounts.(*claudeAutoIntegrationRepo)
			require.True(t, ok)
			r := &claudeRecoveryRepo{claudeAutoIntegrationRepo: baseRepo}
			w.accounts = r
			now := time.Now().UTC().Truncate(time.Second)
			limited := now.Add(-time.Minute)
			reset := now.Add(time.Hour)
			r.account.RateLimitedAt = &limited
			r.account.RateLimitResetAt = &reset
			before := &ClaudeUsageResponse{}
			before.FiveHour.Utilization = 100
			before.FiveHour.ResetsAt = reset.Format(time.RFC3339)
			after := &ClaudeUsageResponse{}
			after.FiveHour.Utilization = 0
			after.FiveHour.ResetsAt = reset.Format(time.RFC3339)
			if tc.remaining {
				after.SevenDay.Utilization = 100
				after.SevenDay.ResetsAt = reset.Format(time.RFC3339)
			}
			if tc.unrelated {
				different := now.Add(2 * time.Hour)
				r.account.RateLimitResetAt = &different
			}
			w.fetcher = claudeRecoveryUsage{usage: after, fail: tc.failure}
			require.Equal(t, tc.want, w.recoverVerifiedQuota(context.Background(), r.account, before, []string{"five_hour"}))
			if tc.want {
				require.Equal(t, 1, r.clears)
				require.Equal(t, limited, r.limited)
				require.Equal(t, reset, r.reset)
			} else {
				require.Zero(t, r.clears)
			}
		})
	}
}
