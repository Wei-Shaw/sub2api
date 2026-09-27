//go:build unit

package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/usagestats"
	"github.com/stretchr/testify/require"
)

func windowCostTestAccounts(n int, windowStart time.Time) []*Account {
	windowEnd := windowStart.Add(5 * time.Hour)
	accounts := make([]*Account, 0, n)
	for i := 1; i <= n; i++ {
		accounts = append(accounts, &Account{
			ID:                 int64(i),
			Platform:           PlatformAnthropic,
			Type:               AccountTypeOAuth,
			Extra:              map[string]any{"window_cost_limit": 100.0},
			SessionWindowStart: &windowStart,
			SessionWindowEnd:   &windowEnd,
		})
	}
	return accounts
}

func TestAccountUsageServiceGetWindowCostBatchUsesOneQueryPerWindowStart(t *testing.T) {
	windowStart := time.Now().Add(-time.Hour).Truncate(time.Hour)
	accounts := windowCostTestAccounts(50, windowStart)
	batchResult := make(map[int64]*usagestats.AccountStats, len(accounts))
	for _, account := range accounts {
		batchResult[account.ID] = &usagestats.AccountStats{StandardCost: float64(account.ID)}
	}
	delete(batchResult, 50) // no usage in the window
	repo := &usageLogWindowBatchRepoStub{batchResult: batchResult}
	cache := &sessionLimitCacheHotpathStub{batchData: map[int64]float64{1: 9.5}}
	svc := &AccountUsageService{usageLogRepo: repo}

	costs := svc.GetWindowCostBatch(context.Background(), accounts, cache)

	t.Logf("window cost queries for 50 accounts: batch=%d single=%d", repo.batchCalls.Load(), repo.singleCalls.Load())
	require.EqualValues(t, 1, repo.batchCalls.Load())
	require.Zero(t, repo.singleCalls.Load())
	require.Len(t, costs, 50)
	require.InDelta(t, 9.5, costs[1], 1e-9, "cache hit wins over the database")
	require.InDelta(t, 2.0, costs[2], 1e-9)
	require.Zero(t, costs[50])
	require.Len(t, cache.setData, 49, "misses are written back to the cache")
	require.NotContains(t, cache.setData, int64(1))
}

func TestAccountUsageServiceGetWindowCostBatchGroupsByWindowStart(t *testing.T) {
	start := time.Now().Add(-time.Hour).Truncate(time.Hour)
	accounts := append(windowCostTestAccounts(2, start), windowCostTestAccounts(3, start.Add(-time.Hour))[2:]...)
	repo := &usageLogWindowBatchRepoStub{}

	costs := (&AccountUsageService{usageLogRepo: repo}).GetWindowCostBatch(context.Background(), accounts, nil)

	require.EqualValues(t, 2, repo.batchCalls.Load())
	require.Zero(t, repo.singleCalls.Load())
	require.Len(t, costs, 3)
}

func TestAccountUsageServiceGetWindowCostBatchFallsBackWhenBatchFails(t *testing.T) {
	accounts := windowCostTestAccounts(3, time.Now().Add(-time.Hour).Truncate(time.Hour))
	repo := &usageLogWindowBatchRepoStub{
		batchErr:     errors.New("batch failed"),
		singleResult: map[int64]*usagestats.AccountStats{2: {StandardCost: 4.25}},
	}
	cache := &sessionLimitCacheHotpathStub{batchErr: errors.New("redis down")}

	costs := (&AccountUsageService{usageLogRepo: repo}).GetWindowCostBatch(context.Background(), accounts, cache)

	require.EqualValues(t, 1, repo.batchCalls.Load())
	require.EqualValues(t, 3, repo.singleCalls.Load())
	require.Len(t, costs, 3)
	require.InDelta(t, 4.25, costs[2], 1e-9)
}
