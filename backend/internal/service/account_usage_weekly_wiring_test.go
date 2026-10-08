package service

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/usagestats"
	"github.com/stretchr/testify/require"
)

type weeklyWiringRepo struct {
	UsageLogRepository
	starts []time.Time
	get    func(time.Time) (*usagestats.AccountStats, error)
}

func (r *weeklyWiringRepo) GetAccountWindowStats(_ context.Context, _ int64, start time.Time) (*usagestats.AccountStats, error) {
	r.starts = append(r.starts, start)
	return r.get(start)
}

func TestClaudeWeeklyStatsPassiveAccountTypes(t *testing.T) {
	for _, typ := range []string{AccountTypeOAuth, AccountTypeSetupToken} {
		t.Run(typ, func(t *testing.T) {
			now := time.Now().UTC().Truncate(time.Second)
			reset := now.Add(48 * time.Hour)
			fiveHourStart, fiveHourEnd := now.Add(-time.Hour), now.Add(4*time.Hour)
			repo := &weeklyWiringRepo{get: func(start time.Time) (*usagestats.AccountStats, error) {
				if start.Equal(reset.Add(-claudeWeeklyWindow)) {
					return &usagestats.AccountStats{Requests: 2, Tokens: 300, Cost: 12, StandardCost: 24, UserCost: 48}, nil
				}
				return &usagestats.AccountStats{Requests: 1, Cost: 2}, nil
			}}
			svc := &AccountUsageService{usageLogRepo: repo, cache: NewUsageCache()}
			info, err := svc.getPassiveUsageForAccount(context.Background(), &Account{
				ID: 42, Platform: PlatformAnthropic, Type: typ,
				SessionWindowStart: &fiveHourStart, SessionWindowEnd: &fiveHourEnd,
				Extra: map[string]any{"session_window_utilization": 0.25, "passive_usage_7d_utilization": 0.4, "passive_usage_7d_reset": reset.Unix(), "passive_usage_7d_oi_utilization": 0.2, "passive_usage_7d_oi_reset": reset.Unix()},
			})
			require.NoError(t, err)
			require.Equal(t, 40.0, info.SevenDay.Utilization)
			require.Equal(t, 12.0, info.SevenDay.WindowStats.Cost)
			require.Equal(t, 24.0, info.SevenDay.WindowStats.StandardCost)
			require.Equal(t, 48.0, info.SevenDay.WindowStats.UserCost)
			require.Equal(t, 2.0, info.FiveHour.WindowStats.Cost)
			require.Nil(t, info.SevenDayFable.WindowStats)
			require.Len(t, repo.starts, 2)
			data, err := json.Marshal(info)
			require.NoError(t, err)
			require.Contains(t, string(data), `"window_stats":{"requests":2,"tokens":300,"cost":12,"standard_cost":24,"user_cost":48}`)
		})
	}
}

func TestClaudeWeeklyStatsActiveCachedResponse(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	reset := now.Add(48 * time.Hour)
	response := &ClaudeUsageResponse{}
	response.SevenDay.Utilization = 40
	response.SevenDay.ResetsAt = reset.Format(time.RFC3339)
	response.SevenDaySonnet.Utilization = 20
	response.SevenDaySonnet.ResetsAt = reset.Format(time.RFC3339)
	cache := NewUsageCache()
	cache.apiCache.Store(int64(42), &apiUsageCache{response: response, timestamp: now})
	repo := &weeklyWiringRepo{get: func(time.Time) (*usagestats.AccountStats, error) {
		return &usagestats.AccountStats{Requests: 2, Cost: 12}, nil
	}}
	svc := &AccountUsageService{accountRepo: &sessionWindowSyncRepo{}, usageLogRepo: repo, cache: cache}
	// Deliberately no usageFetcher/credentials: this fixture cannot make an upstream request.
	info, err := svc.getUsageForAccount(context.Background(), &Account{ID: 42, Platform: PlatformAnthropic, Type: AccountTypeOAuth}, false)
	require.NoError(t, err)
	require.Equal(t, 12.0, info.SevenDay.WindowStats.Cost)
	require.Nil(t, info.SevenDaySonnet.WindowStats)
}

func TestClaudeWeeklyStatsIndependentFailurePaths(t *testing.T) {
	for _, failWeekly := range []bool{false, true} {
		t.Run(map[bool]string{false: "five hour fails", true: "weekly fails"}[failWeekly], func(t *testing.T) {
			now := time.Now()
			reset := now.Add(48 * time.Hour)
			start := reset.Add(-claudeWeeklyWindow)
			repo := &weeklyWiringRepo{get: func(queryStart time.Time) (*usagestats.AccountStats, error) {
				if queryStart.Equal(start) == failWeekly {
					return nil, errors.New("unavailable")
				}
				return &usagestats.AccountStats{Requests: 1, Cost: 12}, nil
			}}
			svc := &AccountUsageService{usageLogRepo: repo, cache: NewUsageCache()}
			info := &UsageInfo{FiveHour: &UsageProgress{}, SevenDay: &UsageProgress{ResetsAt: &reset, Utilization: 40}}
			svc.addWindowStats(context.Background(), &Account{ID: 42, Platform: PlatformAnthropic, Type: AccountTypeOAuth}, info)
			if failWeekly {
				require.Nil(t, info.SevenDay.WindowStats)
				require.NotNil(t, info.FiveHour.WindowStats)
			} else {
				require.NotNil(t, info.SevenDay.WindowStats)
				require.Nil(t, info.FiveHour.WindowStats)
			}
		})
	}
}

func TestClaudeWeeklyStatsWithoutFiveHourAndUnsupportedAccounts(t *testing.T) {
	for _, tt := range []struct {
		platform, typ string
		want          bool
	}{
		{PlatformAnthropic, AccountTypeOAuth, true}, {PlatformAnthropic, AccountTypeSetupToken, true},
		{PlatformAnthropic, AccountTypeAPIKey, false}, {PlatformAnthropic, AccountTypeBedrock, false}, {PlatformOpenAI, AccountTypeOAuth, false},
	} {
		t.Run(tt.platform+tt.typ, func(t *testing.T) {
			reset := time.Now().Add(48 * time.Hour)
			repo := &weeklyWiringRepo{get: func(time.Time) (*usagestats.AccountStats, error) {
				return &usagestats.AccountStats{Requests: 1, Cost: 12}, nil
			}}
			svc := &AccountUsageService{usageLogRepo: repo, cache: NewUsageCache()}
			info := &UsageInfo{SevenDay: &UsageProgress{ResetsAt: &reset, Utilization: 40}, SevenDaySonnet: &UsageProgress{ResetsAt: &reset}, SevenDayFable: &UsageProgress{ResetsAt: &reset}}
			svc.addWindowStats(context.Background(), &Account{ID: 42, Platform: tt.platform, Type: tt.typ}, info)
			require.Equal(t, tt.want, info.SevenDay.WindowStats != nil)
			require.Nil(t, info.SevenDaySonnet.WindowStats)
			require.Nil(t, info.SevenDayFable.WindowStats)
		})
	}
}
