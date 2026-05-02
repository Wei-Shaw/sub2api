//go:build unit

package service

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// ---------- resolveBalanceThreshold ----------

func TestResolveBalanceThreshold_Fixed(t *testing.T) {
	// Fixed type always returns the raw threshold regardless of totalRecharged.
	require.Equal(t, 10.0, resolveBalanceThreshold(10, thresholdTypeFixed, 1000))
	require.Equal(t, 10.0, resolveBalanceThreshold(10, thresholdTypeFixed, 0))
	require.Equal(t, 0.0, resolveBalanceThreshold(0, thresholdTypeFixed, 1000))
}

func TestResolveBalanceThreshold_Percentage(t *testing.T) {
	// 10% of 1000 = 100
	require.Equal(t, 100.0, resolveBalanceThreshold(10, thresholdTypePercentage, 1000))
	// 50% of 200 = 100
	require.Equal(t, 100.0, resolveBalanceThreshold(50, thresholdTypePercentage, 200))
}

func TestResolveBalanceThreshold_PercentageZeroRecharged(t *testing.T) {
	// When totalRecharged is 0, percentage falls through to raw threshold
	// (treated as fixed). This is the defensive behavior.
	require.Equal(t, 10.0, resolveBalanceThreshold(10, thresholdTypePercentage, 0))
}

func TestResolveBalanceThreshold_EmptyType(t *testing.T) {
	// Empty type is treated as fixed (not percentage).
	require.Equal(t, 10.0, resolveBalanceThreshold(10, "", 1000))
}

// ---------- quotaDim.resolvedThreshold ----------

func TestResolvedThreshold_FixedNormal(t *testing.T) {
	// threshold=400 remaining, limit=1000 → usage trigger at 600
	d := quotaDim{threshold: 400, thresholdType: thresholdTypeFixed, limit: 1000}
	require.Equal(t, 600.0, d.resolvedThreshold())
}

func TestResolvedThreshold_FixedThresholdExceedsLimit(t *testing.T) {
	// threshold=1200, limit=1000 → returns negative, callers must skip
	d := quotaDim{threshold: 1200, thresholdType: thresholdTypeFixed, limit: 1000}
	require.Equal(t, -200.0, d.resolvedThreshold())
}

func TestResolvedThreshold_FixedThresholdEqualsLimit(t *testing.T) {
	// threshold=1000, limit=1000 → returns 0 (alert fires at 0 usage)
	d := quotaDim{threshold: 1000, thresholdType: thresholdTypeFixed, limit: 1000}
	require.Equal(t, 0.0, d.resolvedThreshold())
}

func TestResolvedThreshold_PercentageNormal(t *testing.T) {
	// threshold=30%, limit=1000 → usage trigger at 700 (remaining drops to 30%)
	d := quotaDim{threshold: 30, thresholdType: thresholdTypePercentage, limit: 1000}
	require.InDelta(t, 700.0, d.resolvedThreshold(), 0.001)
}

func TestResolvedThreshold_PercentageZeroPercent(t *testing.T) {
	// threshold=0%, limit=1000 → fires when remaining drops to 0 (usage=1000)
	d := quotaDim{threshold: 0, thresholdType: thresholdTypePercentage, limit: 1000}
	require.InDelta(t, 1000.0, d.resolvedThreshold(), 0.001)
}

func TestResolvedThreshold_PercentageHundredPercent(t *testing.T) {
	// threshold=100%, limit=1000 → fires immediately (remaining drops to 100% i.e. nothing used yet)
	d := quotaDim{threshold: 100, thresholdType: thresholdTypePercentage, limit: 1000}
	require.InDelta(t, 0.0, d.resolvedThreshold(), 0.001)
}

func TestResolvedThreshold_PercentageOverHundred(t *testing.T) {
	// threshold=150%, limit=1000 → returns negative (never triggers; callers skip)
	d := quotaDim{threshold: 150, thresholdType: thresholdTypePercentage, limit: 1000}
	require.Less(t, d.resolvedThreshold(), 0.0)
}

func TestResolvedThreshold_ZeroLimit(t *testing.T) {
	// limit=0 → returns 0 to avoid division and false alerts on unlimited quotas
	d := quotaDim{threshold: 100, thresholdType: thresholdTypeFixed, limit: 0}
	require.Equal(t, 0.0, d.resolvedThreshold())
}

func TestResolvedThreshold_NegativeLimit(t *testing.T) {
	// Negative limit treated as 0
	d := quotaDim{threshold: 100, thresholdType: thresholdTypeFixed, limit: -10}
	require.Equal(t, 0.0, d.resolvedThreshold())
}

// ---------- sanitizeEmailHeader ----------

func TestSanitizeEmailHeader_CRLF(t *testing.T) {
	require.Equal(t, "Subject injected", sanitizeEmailHeader("Subject\r\n injected"))
}

func TestSanitizeEmailHeader_OnlyCR(t *testing.T) {
	require.Equal(t, "foobar", sanitizeEmailHeader("foo\rbar"))
}

func TestSanitizeEmailHeader_OnlyLF(t *testing.T) {
	require.Equal(t, "foobar", sanitizeEmailHeader("foo\nbar"))
}

func TestSanitizeEmailHeader_Clean(t *testing.T) {
	require.Equal(t, "Sub2API", sanitizeEmailHeader("Sub2API"))
}

func TestSanitizeEmailHeader_Empty(t *testing.T) {
	require.Equal(t, "", sanitizeEmailHeader(""))
}

func TestSanitizeEmailHeader_MultipleNewlines(t *testing.T) {
	require.Equal(t, "abc", sanitizeEmailHeader("a\r\nb\r\nc"))
}

// ---------- buildQuotaDims ----------

func TestBuildQuotaDims_AllDimensionsReturned(t *testing.T) {
	// Use an account with quota notify config across all 3 dimensions.
	a := &Account{
		Platform: PlatformAnthropic,
		Type:     AccountTypeAPIKey,
		Extra: map[string]any{
			"quota_notify_daily_enabled":         true,
			"quota_notify_daily_threshold":       100.0,
			"quota_notify_daily_threshold_type":  thresholdTypeFixed,
			"quota_notify_weekly_enabled":        true,
			"quota_notify_weekly_threshold":      20.0,
			"quota_notify_weekly_threshold_type": thresholdTypePercentage,
			"quota_notify_total_enabled":         false,
			"quota_daily_limit":                  500.0,
			"quota_weekly_limit":                 2000.0,
			"quota_limit":                        10000.0,
			"quota_daily_used":                   50.0,
			"quota_weekly_used":                  300.0,
			"quota_used":                         1000.0,
		},
	}

	dims := buildQuotaDims(a)
	require.Len(t, dims, 3)

	// Daily
	require.Equal(t, quotaDimDaily, dims[0].name)
	require.True(t, dims[0].enabled)
	require.Equal(t, 100.0, dims[0].threshold)
	require.Equal(t, thresholdTypeFixed, dims[0].thresholdType)
	require.Equal(t, 500.0, dims[0].limit)
	require.Equal(t, 50.0, dims[0].currentUsed)

	// Weekly
	require.Equal(t, quotaDimWeekly, dims[1].name)
	require.True(t, dims[1].enabled)
	require.Equal(t, 20.0, dims[1].threshold)
	require.Equal(t, thresholdTypePercentage, dims[1].thresholdType)
	require.Equal(t, 2000.0, dims[1].limit)

	// Total
	require.Equal(t, quotaDimTotal, dims[2].name)
	require.False(t, dims[2].enabled)
	require.Equal(t, 10000.0, dims[2].limit)
	require.Equal(t, 1000.0, dims[2].currentUsed)
}

func TestBuildQuotaDims_EmptyExtra(t *testing.T) {
	// Missing fields default to zero/disabled.
	a := &Account{
		Platform: PlatformAnthropic,
		Type:     AccountTypeAPIKey,
		Extra:    map[string]any{},
	}
	dims := buildQuotaDims(a)
	require.Len(t, dims, 3)
	for _, d := range dims {
		require.False(t, d.enabled)
		require.Equal(t, 0.0, d.threshold)
		require.Equal(t, 0.0, d.limit)
	}
}

// ---------- buildQuotaDimsFromState ----------

func TestBuildQuotaDimsFromState_UsesStateValues(t *testing.T) {
	// Usage values should come from the state, not the account.
	a := &Account{
		Platform: PlatformAnthropic,
		Type:     AccountTypeAPIKey,
		Extra: map[string]any{
			"quota_notify_daily_enabled":   true,
			"quota_notify_daily_threshold": 100.0,
			"quota_daily_used":             999.0, // should be ignored
			"quota_daily_limit":            999.0, // should be ignored
		},
	}
	state := &AccountQuotaState{
		DailyUsed:   77.0,
		DailyLimit:  500.0,
		WeeklyUsed:  88.0,
		WeeklyLimit: 2000.0,
		TotalUsed:   99.0,
		TotalLimit:  10000.0,
	}
	dims := buildQuotaDimsFromState(a, state)
	require.Len(t, dims, 3)
	// Settings from account (enabled, threshold, thresholdType)
	require.True(t, dims[0].enabled)
	require.Equal(t, 100.0, dims[0].threshold)
	// Usage from state
	require.Equal(t, 77.0, dims[0].currentUsed)
	require.Equal(t, 500.0, dims[0].limit)
	require.Equal(t, 88.0, dims[1].currentUsed)
	require.Equal(t, 2000.0, dims[1].limit)
	require.Equal(t, 99.0, dims[2].currentUsed)
	require.Equal(t, 10000.0, dims[2].limit)
}

// ---------- collectBalanceNotifyRecipients ----------

func TestCollectBalanceNotifyRecipients_Empty(t *testing.T) {
	s := &BalanceNotifyService{}
	u := &User{BalanceNotifyExtraEmails: nil}
	require.Empty(t, s.collectBalanceNotifyRecipients(u))
}

func TestCollectBalanceNotifyRecipients_FiltersDisabledAndUnverified(t *testing.T) {
	s := &BalanceNotifyService{}
	u := &User{
		BalanceNotifyExtraEmails: []NotifyEmailEntry{
			{Email: "a@example.com", Verified: true, Disabled: false},
			{Email: "b@example.com", Verified: true, Disabled: true},   // disabled
			{Email: "c@example.com", Verified: false, Disabled: false}, // unverified
			{Email: "d@example.com", Verified: true, Disabled: false},
		},
	}
	got := s.collectBalanceNotifyRecipients(u)
	require.Equal(t, []string{"a@example.com", "d@example.com"}, got)
}

func TestCollectBalanceNotifyRecipients_DeduplicatesCaseInsensitive(t *testing.T) {
	s := &BalanceNotifyService{}
	u := &User{
		BalanceNotifyExtraEmails: []NotifyEmailEntry{
			{Email: "User@Example.com", Verified: true},
			{Email: "user@example.com", Verified: true},
			{Email: "USER@EXAMPLE.COM", Verified: true},
		},
	}
	got := s.collectBalanceNotifyRecipients(u)
	require.Len(t, got, 1)
	// The original casing of the first entry is preserved.
	require.Equal(t, "User@Example.com", got[0])
}

func TestCollectBalanceNotifyRecipients_SkipsEmpty(t *testing.T) {
	s := &BalanceNotifyService{}
	u := &User{
		BalanceNotifyExtraEmails: []NotifyEmailEntry{
			{Email: "  ", Verified: true},
			{Email: "", Verified: true},
			{Email: "valid@example.com", Verified: true},
		},
	}
	got := s.collectBalanceNotifyRecipients(u)
	require.Equal(t, []string{"valid@example.com"}, got)
}

func TestCollectBalanceNotifyRecipients_TrimsWhitespace(t *testing.T) {
	s := &BalanceNotifyService{}
	u := &User{
		BalanceNotifyExtraEmails: []NotifyEmailEntry{
			{Email: "  trimmed@example.com  ", Verified: true},
		},
	}
	got := s.collectBalanceNotifyRecipients(u)
	require.Equal(t, []string{"trimmed@example.com"}, got)
}
