package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/usagestats"
	"github.com/stretchr/testify/require"
)

type windowCostRepo struct {
	usageBatchLogRepoStub
	starts    []time.Time
	failStart time.Time
}

func (r *windowCostRepo) GetAccountWindowStats(_ context.Context, _ int64, start time.Time) (*usagestats.AccountStats, error) {
	r.starts = append(r.starts, start)
	if start.Equal(r.failStart) {
		return nil, errors.New("test query failure")
	}
	return &usagestats.AccountStats{Requests: 1, Tokens: 100, Cost: float64(len(r.starts)), UserCost: 99}, nil
}

func TestAccountUsageService_ClaudeWindowCosts(t *testing.T) {
	for _, accountType := range []string{AccountTypeOAuth, AccountTypeSetupToken} {
		t.Run(accountType, func(t *testing.T) {
			now := time.Now().UTC().Truncate(time.Second)
			fiveReset, weeklyReset := now.Add(time.Hour), now.Add(48*time.Hour)
			account := Account{ID: 8510, Platform: PlatformAnthropic, Type: accountType,
				SessionWindowEnd: &fiveReset,
				Extra: map[string]any{
					"session_window_utilization":   0.25,
					"passive_usage_7d_utilization": 0.4,
					"passive_usage_7d_reset":       weeklyReset.Unix(),
				},
			}
			repo := &windowCostRepo{}
			svc := &AccountUsageService{accountRepo: &stubOpenAIAccountRepo{accounts: []Account{account}}, usageLogRepo: repo, cache: NewUsageCache()}
			usage, err := svc.GetPassiveUsage(t.Context(), account.ID)
			require.NoError(t, err)
			require.Len(t, repo.starts, 2)
			require.True(t, fiveReset.Add(-5*time.Hour).Equal(repo.starts[0]))
			require.True(t, weeklyReset.Add(-7*24*time.Hour).Equal(repo.starts[1]))
			require.Equal(t, 1.0, usage.FiveHour.WindowStats.Cost)
			require.Equal(t, 2.0, usage.SevenDay.WindowStats.Cost)

			// Cached costs remain separated, and a changed reset invalidates the weekly entry.
			svc.addWindowStats(t.Context(), &account, usage)
			require.Len(t, repo.starts, 2)
			nextReset := weeklyReset.Add(time.Hour)
			usage.SevenDay.ResetsAt = &nextReset
			svc.addWindowStats(t.Context(), &account, usage)
			require.Len(t, repo.starts, 3)
			require.True(t, nextReset.Add(-7*24*time.Hour).Equal(repo.starts[2]))
			require.Equal(t, 1.0, usage.FiveHour.WindowStats.Cost)
			require.Equal(t, 3.0, usage.SevenDay.WindowStats.Cost)
			entries := 0
			svc.cache.windowStatsCache.Range(func(_, _ any) bool { entries++; return true })
			require.Equal(t, 2, entries)
		})
	}
}

func TestAccountUsageService_ClaudeWindowCostsMissingOrExpired(t *testing.T) {
	now := time.Now()
	expired, futureStart := now.Add(-time.Hour), now.Add(8*24*time.Hour)
	for name, reset := range map[string]*time.Time{"missing": nil, "expired": &expired, "future_start": &futureStart} {
		t.Run(name, func(t *testing.T) {
			repo := &windowCostRepo{}
			svc := &AccountUsageService{usageLogRepo: repo, cache: NewUsageCache()}
			usage := &UsageInfo{SevenDay: &UsageProgress{ResetsAt: reset}}
			svc.addWindowStats(t.Context(), &Account{ID: 8511, Platform: PlatformAnthropic, Type: AccountTypeSetupToken}, usage)
			require.Empty(t, repo.starts)
			require.Nil(t, usage.SevenDay.WindowStats)
		})
	}
}

func TestAccountUsageService_ClaudeWindowCostsIndependentFailure(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	fiveReset, weeklyReset := now.Add(time.Hour), now.Add(48*time.Hour)
	repo := &windowCostRepo{failStart: fiveReset.Add(-5 * time.Hour)}
	svc := &AccountUsageService{usageLogRepo: repo, cache: NewUsageCache()}
	usage := &UsageInfo{
		FiveHour:       &UsageProgress{ResetsAt: &fiveReset},
		SevenDay:       &UsageProgress{ResetsAt: &weeklyReset},
		SevenDaySonnet: &UsageProgress{ResetsAt: &weeklyReset},
		SevenDayFable:  &UsageProgress{ResetsAt: &weeklyReset},
	}
	svc.addWindowStats(t.Context(), &Account{ID: 8512, Platform: PlatformAnthropic, Type: AccountTypeOAuth}, usage)
	require.Nil(t, usage.FiveHour.WindowStats)
	require.NotNil(t, usage.SevenDay.WindowStats)
	require.Nil(t, usage.SevenDaySonnet.WindowStats)
	require.Nil(t, usage.SevenDayFable.WindowStats)
}
