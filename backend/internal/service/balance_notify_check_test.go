//go:build unit

package service

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

// newBalanceNotifyServiceForTest constructs a BalanceNotifyService with an
// in-memory settings repo and a non-nil emailService so that the guard-clause
// nil-checks pass. The emailService is intentionally minimal — tests must
// avoid crossing scenarios that would actually dispatch emails.
func newBalanceNotifyServiceForTest() (*BalanceNotifyService, *mockSettingRepo) {
	repo := newMockSettingRepo()
	// EmailService is a concrete type; construct with the same repo so that
	// any accidental fallback reads still succeed. Tests should not trigger a
	// crossing that reaches SendEmail.
	email := NewEmailService(repo, nil)
	return NewBalanceNotifyService(email, repo, nil), repo
}

// ---------- guard clauses ----------

func TestCheckBalanceAfterDeduction_NilUser(t *testing.T) {
	s, _ := newBalanceNotifyServiceForTest()
	// Should not panic.
	s.CheckBalanceAfterDeduction(context.Background(), nil, 100, 50)
}

func TestCheckBalanceAfterDeduction_UserNotifyDisabled(t *testing.T) {
	s, repo := newBalanceNotifyServiceForTest()
	repo.data[SettingKeyBalanceLowNotifyEnabled] = "true"
	repo.data[SettingKeyBalanceLowNotifyThreshold] = "10"
	u := &User{ID: 1, BalanceNotifyEnabled: false}
	// Even with a crossing, disabled flag short-circuits.
	s.CheckBalanceAfterDeduction(context.Background(), u, 20, 15)
}

func TestCheckBalanceAfterDeduction_GlobalDisabled(t *testing.T) {
	s, repo := newBalanceNotifyServiceForTest()
	repo.data[SettingKeyBalanceLowNotifyEnabled] = "false"
	u := &User{ID: 1, BalanceNotifyEnabled: true}
	s.CheckBalanceAfterDeduction(context.Background(), u, 20, 15)
}

func TestCheckBalanceAfterDeduction_ThresholdZero(t *testing.T) {
	s, repo := newBalanceNotifyServiceForTest()
	repo.data[SettingKeyBalanceLowNotifyEnabled] = "true"
	repo.data[SettingKeyBalanceLowNotifyThreshold] = "0"
	u := &User{ID: 1, BalanceNotifyEnabled: true}
	s.CheckBalanceAfterDeduction(context.Background(), u, 20, 15)
}

func TestCheckBalanceAfterDeduction_UserThresholdOverride(t *testing.T) {
	s, repo := newBalanceNotifyServiceForTest()
	repo.data[SettingKeyBalanceLowNotifyEnabled] = "true"
	repo.data[SettingKeyBalanceLowNotifyThreshold] = "100" // global default
	customThreshold := 5.0
	u := &User{
		ID:                     1,
		BalanceNotifyEnabled:   true,
		BalanceNotifyThreshold: &customThreshold,
	}
	// User's 5.0 threshold takes precedence over global 100. 20 -> 15 does not
	// cross 5, so nothing fires (verified by absence of panic).
	s.CheckBalanceAfterDeduction(context.Background(), u, 20, 15)
}

func TestCheckBalanceAfterDeduction_NoCrossingNotFired(t *testing.T) {
	s, repo := newBalanceNotifyServiceForTest()
	repo.data[SettingKeyBalanceLowNotifyEnabled] = "true"
	repo.data[SettingKeyBalanceLowNotifyThreshold] = "10"
	u := &User{ID: 1, BalanceNotifyEnabled: true}

	// 100 -> 95, both remain above threshold=10, no crossing.
	s.CheckBalanceAfterDeduction(context.Background(), u, 100, 5)
	// 5 -> 3, both already below threshold, no crossing (only fires on first
	// cross from above-to-below).
	s.CheckBalanceAfterDeduction(context.Background(), u, 5, 2)
}

// ---------- nil-service guards on CheckAccountQuotaAfterIncrement ----------

func TestCheckAccountQuotaAfterIncrement_NilAccount(t *testing.T) {
	s, _ := newBalanceNotifyServiceForTest()
	// Should not panic.
	s.CheckAccountQuotaAfterIncrement(context.Background(), nil, 10, nil)
}

func TestCheckAccountQuotaAfterIncrement_ZeroCost(t *testing.T) {
	s, _ := newBalanceNotifyServiceForTest()
	a := &Account{ID: 1, Platform: PlatformAnthropic, Type: AccountTypeAPIKey}
	s.CheckAccountQuotaAfterIncrement(context.Background(), a, 0, nil)
}

func TestCheckAccountQuotaAfterIncrement_NegativeCost(t *testing.T) {
	s, _ := newBalanceNotifyServiceForTest()
	a := &Account{ID: 1, Platform: PlatformAnthropic, Type: AccountTypeAPIKey}
	s.CheckAccountQuotaAfterIncrement(context.Background(), a, -5, nil)
}

func TestCheckAccountQuotaAfterIncrement_GlobalDisabled(t *testing.T) {
	s, repo := newBalanceNotifyServiceForTest()
	repo.data[SettingKeyAccountQuotaNotifyEnabled] = "false"
	a := &Account{
		ID:       1,
		Platform: PlatformAnthropic,
		Type:     AccountTypeAPIKey,
		Extra: map[string]any{
			"quota_notify_daily_enabled":   true,
			"quota_notify_daily_threshold": 100.0,
			"quota_daily_limit":            1000.0,
			"quota_daily_used":             950.0,
		},
	}
	// Global disabled → no processing even if a dim would cross.
	s.CheckAccountQuotaAfterIncrement(context.Background(), a, 100, nil)
}

// ---------- sanity: internal helpers still work ----------

func TestGetBalanceNotifyConfig_AllFields(t *testing.T) {
	s, repo := newBalanceNotifyServiceForTest()
	repo.data[SettingKeyBalanceLowNotifyEnabled] = "true"
	repo.data[SettingKeyBalanceLowNotifyThreshold] = "12.5"
	repo.data[SettingKeyBalanceLowNotifyRechargeURL] = "https://example.com/pay"

	enabled, threshold, url := s.getBalanceNotifyConfig(context.Background())
	require.True(t, enabled)
	require.Equal(t, 12.5, threshold)
	require.Equal(t, "https://example.com/pay", url)
}

func TestGetBalanceNotifyConfig_Disabled(t *testing.T) {
	s, repo := newBalanceNotifyServiceForTest()
	repo.data[SettingKeyBalanceLowNotifyEnabled] = "false"

	enabled, _, _ := s.getBalanceNotifyConfig(context.Background())
	require.False(t, enabled)
}

func TestGetBalanceNotifyConfig_InvalidThreshold(t *testing.T) {
	s, repo := newBalanceNotifyServiceForTest()
	repo.data[SettingKeyBalanceLowNotifyEnabled] = "true"
	repo.data[SettingKeyBalanceLowNotifyThreshold] = "not-a-number"

	enabled, threshold, _ := s.getBalanceNotifyConfig(context.Background())
	require.True(t, enabled)
	require.Equal(t, 0.0, threshold)
}

func TestIsAccountQuotaNotifyEnabled(t *testing.T) {
	s, repo := newBalanceNotifyServiceForTest()

	// Missing key → false
	require.False(t, s.isAccountQuotaNotifyEnabled(context.Background()))

	// Explicit "false"
	repo.data[SettingKeyAccountQuotaNotifyEnabled] = "false"
	require.False(t, s.isAccountQuotaNotifyEnabled(context.Background()))

	// Explicit "true"
	repo.data[SettingKeyAccountQuotaNotifyEnabled] = "true"
	require.True(t, s.isAccountQuotaNotifyEnabled(context.Background()))
}

func TestGetSiteName_FallsBackToDefault(t *testing.T) {
	s, _ := newBalanceNotifyServiceForTest()
	name := s.getSiteName(context.Background())
	require.Equal(t, defaultSiteName, name)
}

func TestGetSiteName_Configured(t *testing.T) {
	s, repo := newBalanceNotifyServiceForTest()
	repo.data[SettingKeySiteName] = "My Site"
	require.Equal(t, "My Site", s.getSiteName(context.Background()))
}
