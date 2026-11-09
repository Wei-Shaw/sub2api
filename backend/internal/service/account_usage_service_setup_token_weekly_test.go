package service

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestAccountUsageService_SetupTokenWeeklyUsage(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	sampledAt := now.Add(-10 * time.Minute)
	resetAt := now.Add(48 * time.Hour)
	fableResetAt := now.Add(72 * time.Hour)

	for _, scenario := range []string{"without_5h", "active_5h", "expired_5h", "zero_weekly_usage", "no_samples"} {
		t.Run(scenario, func(t *testing.T) {
			account := Account{
				ID: 8508, Platform: PlatformAnthropic, Type: AccountTypeSetupToken,
				Extra: map[string]any{
					"passive_usage_7d_utilization":    0.42,
					"passive_usage_7d_reset":          resetAt.Unix(),
					"passive_usage_7d_oi_utilization": 0.73,
					"passive_usage_7d_oi_reset":       fableResetAt.Unix(),
					"passive_usage_sampled_at":        sampledAt.Format(time.RFC3339),
				},
			}
			weeklyUtilization := 42.0
			fableUtilization := 73.0
			switch scenario {
			case "active_5h":
				end := now.Add(2 * time.Hour)
				account.SessionWindowEnd = &end
				account.Extra["session_window_utilization"] = 0.15
			case "expired_5h":
				end := now.Add(-time.Hour)
				account.SessionWindowEnd = &end
				account.Extra["session_window_utilization"] = 0.95
			case "zero_weekly_usage":
				weeklyUtilization, fableUtilization = 0, 0
				account.Extra["passive_usage_7d_utilization"] = 0.0
				account.Extra["passive_usage_7d_oi_utilization"] = 0.0
			case "no_samples":
				account.Extra = nil
			}

			repo := &stubOpenAIAccountRepo{accounts: []Account{account}}
			// No usage fetcher: setup-token queries must use the persisted sample,
			// including explicit refreshes, without requiring a profile-scoped token.
			svc := &AccountUsageService{
				accountRepo: repo, usageLogRepo: &usageBatchLogRepoStub{}, cache: NewUsageCache(),
			}
			ctx := t.Context()
			usage, err := svc.GetUsage(ctx, account.ID)
			require.NoError(t, err)
			forced, err := svc.GetUsage(ctx, account.ID, true)
			require.NoError(t, err)
			passive, err := svc.GetPassiveUsage(ctx, account.ID)
			require.NoError(t, err)
			batch, failures, err := svc.GetUsageBatch(ctx, []int64{account.ID}, true)
			require.NoError(t, err)
			require.Empty(t, failures)

			for name, result := range map[string]*UsageInfo{
				"explicit": usage, "forced": forced, "passive": passive, "batch": batch[account.ID],
			} {
				t.Run(name, func(t *testing.T) {
					require.NotNil(t, result)
					require.Equal(t, "passive", result.Source)
					if scenario == "no_samples" {
						require.Nil(t, result.SevenDay)
						require.Nil(t, result.SevenDayFable)
						require.Nil(t, result.UpdatedAt)
						return
					}
					require.NotNil(t, result.UpdatedAt)
					require.True(t, sampledAt.Equal(*result.UpdatedAt))
					require.NotNil(t, result.SevenDay)
					require.InDelta(t, weeklyUtilization, result.SevenDay.Utilization, 1e-9)
					require.NotNil(t, result.SevenDay.ResetsAt)
					require.True(t, resetAt.Equal(*result.SevenDay.ResetsAt))
					require.Positive(t, result.SevenDay.RemainingSeconds)
					require.NotNil(t, result.SevenDayFable)
					require.InDelta(t, fableUtilization, result.SevenDayFable.Utilization, 1e-9)
					require.NotNil(t, result.SevenDayFable.ResetsAt)
					require.True(t, fableResetAt.Equal(*result.SevenDayFable.ResetsAt))
					require.Positive(t, result.SevenDayFable.RemainingSeconds)
					if scenario == "active_5h" {
						require.NotNil(t, result.FiveHour)
						require.InDelta(t, 15.0, result.FiveHour.Utilization, 1e-9)
					}
				})
			}
		})
	}
}

func TestAccountUsageService_NonAnthropicSetupTokenPreservesEstimate(t *testing.T) {
	resetAt := time.Now().Add(time.Hour)
	account := Account{
		ID: 8509, Platform: PlatformOpenAI, Type: AccountTypeSetupToken,
		SessionWindowEnd: &resetAt, SessionWindowStatus: "allowed_warning",
	}
	svc := &AccountUsageService{
		accountRepo:  &stubOpenAIAccountRepo{accounts: []Account{account}},
		usageLogRepo: &usageBatchLogRepoStub{},
		cache:        NewUsageCache(),
	}
	usage, err := svc.GetUsage(t.Context(), account.ID)
	require.NoError(t, err)
	require.NotNil(t, usage.FiveHour)
	require.Equal(t, 80.0, usage.FiveHour.Utilization)
	require.Nil(t, usage.SevenDay)
	require.Empty(t, usage.Source)
}
