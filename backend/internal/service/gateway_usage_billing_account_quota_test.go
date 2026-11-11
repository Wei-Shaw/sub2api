package service

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestBuildUsageBillingCommand_AccountQuotaPeriodHints(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		accountType string
		extra       map[string]any
		wantQuota   bool
		wantDaily   bool
		wantWeekly  bool
	}{
		{name: "no_limits", extra: map[string]any{}},
		{name: "total_only", extra: map[string]any{"quota_limit": 10.0}, wantQuota: true},
		{name: "daily_only", extra: map[string]any{"quota_daily_limit": 5.0}, wantQuota: true, wantDaily: true},
		{name: "weekly_only", extra: map[string]any{"quota_weekly_limit": 5.0}, wantQuota: true, wantWeekly: true},
		{
			name:      "all_dimensions",
			extra:     map[string]any{"quota_limit": 10.0, "quota_daily_limit": 5.0, "quota_weekly_limit": 7.0},
			wantQuota: true, wantDaily: true, wantWeekly: true,
		},
		{
			name:      "non_positive_period_limits",
			extra:     map[string]any{"quota_limit": 10.0, "quota_daily_limit": 0.0, "quota_weekly_limit": -1.0},
			wantQuota: true,
		},
		{
			name:        "service_account_total_only",
			accountType: AccountTypeServiceAccount,
			extra:       map[string]any{"quota_limit": 10.0},
			wantQuota:   true,
		},
		{
			name:        "service_account_daily",
			accountType: AccountTypeServiceAccount,
			extra:       map[string]any{"quota_limit": 10.0, "quota_daily_limit": 5.0},
			wantQuota:   true, wantDaily: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			accountType := tt.accountType
			if accountType == "" {
				accountType = AccountTypeAPIKey
			}
			p := &postUsageBillingParams{
				Cost:                  &CostBreakdown{TotalCost: 1, ActualCost: 1},
				User:                  &User{ID: 1},
				APIKey:                &APIKey{ID: 2},
				Account:               &Account{ID: 3, Type: accountType, Extra: tt.extra},
				AccountRateMultiplier: 1,
			}

			cmd := buildUsageBillingCommand("req-1", nil, p)
			require.NotNil(t, cmd)
			require.Equal(t, tt.wantQuota, cmd.AccountQuotaCost > 0)
			require.Equal(t, tt.wantDaily, cmd.AccountQuotaHasDailyLimit)
			require.Equal(t, tt.wantWeekly, cmd.AccountQuotaHasWeeklyLimit)
		})
	}
}

func TestUsageBillingCommandFingerprintIgnoresAccountQuotaPeriodHints(t *testing.T) {
	t.Parallel()

	base := UsageBillingCommand{
		RequestID:        "req-1",
		UserID:           1,
		APIKeyID:         2,
		AccountID:        3,
		AccountType:      AccountTypeAPIKey,
		BalanceCost:      1,
		AccountQuotaCost: 1,
	}
	hinted := base
	hinted.AccountQuotaHasDailyLimit = true
	hinted.AccountQuotaHasWeeklyLimit = true

	base.Normalize()
	hinted.Normalize()
	require.NotEmpty(t, base.RequestFingerprint)
	require.Equal(t, base.RequestFingerprint, hinted.RequestFingerprint)
}
