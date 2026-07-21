//go:build unit

package repository

import (
	"context"
	"errors"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
)

type accountLoadBatchPipelineHook struct {
	evalCommands atomic.Int64
	rejectEval   bool
}

func (h *accountLoadBatchPipelineHook) DialHook(next redis.DialHook) redis.DialHook {
	return next
}

func (h *accountLoadBatchPipelineHook) ProcessHook(next redis.ProcessHook) redis.ProcessHook {
	return next
}

func (h *accountLoadBatchPipelineHook) ProcessPipelineHook(next redis.ProcessPipelineHook) redis.ProcessPipelineHook {
	return func(ctx context.Context, cmds []redis.Cmder) error {
		for _, cmd := range cmds {
			if cmd.Name() != "eval" {
				continue
			}
			h.evalCommands.Add(1)
			if h.rejectEval {
				return errors.New("NOPERM this user has no permissions to run the 'eval' command")
			}
		}
		return next(ctx, cmds)
	}
}

func newAccountLoadBatchUnitCache(t *testing.T, now time.Time) (*concurrencyCache, *miniredis.Miniredis) {
	t.Helper()
	mr := miniredis.RunT(t)
	mr.SetTime(now)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })
	cache := NewConcurrencyCache(rdb, 1, 60).(*concurrencyCache)
	return cache, mr
}

func seedAccountLoadBatchUnitData(t *testing.T, cache *concurrencyCache, now time.Time) []service.AccountWithConcurrency {
	t.Helper()
	ctx := context.Background()
	accounts := []service.AccountWithConcurrency{
		{ID: 101, MaxConcurrency: 4},
		{ID: 102, MaxConcurrency: 2},
		{ID: 103, MaxConcurrency: 0},
	}
	require.NoError(t, cache.rdb.ZAdd(ctx, accountSlotKey(101),
		redis.Z{Score: float64(now.Unix()), Member: "live-101"},
		redis.Z{Score: float64(now.Unix() - 61), Member: "expired-101"},
	).Err())
	require.NoError(t, cache.rdb.Set(ctx, accountWaitKey(101), "2", time.Minute).Err())
	require.NoError(t, cache.rdb.ZAdd(ctx, accountSlotKey(102),
		redis.Z{Score: float64(now.Unix()), Member: "live-102-a"},
		redis.Z{Score: float64(now.Unix()), Member: "live-102-b"},
	).Err())
	require.NoError(t, cache.rdb.Set(ctx, accountWaitKey(103), "invalid", time.Minute).Err())
	return accounts
}

func TestConcurrencyCacheAccountLoadBatchScriptMatchesPipeline(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	legacy, _ := newAccountLoadBatchUnitCache(t, now)
	legacyAccounts := seedAccountLoadBatchUnitData(t, legacy, now)
	want, err := legacy.getAccountsLoadBatchPipeline(context.Background(), legacyAccounts)
	require.NoError(t, err)

	scripted, _ := newAccountLoadBatchUnitCache(t, now)
	scriptedAccounts := seedAccountLoadBatchUnitData(t, scripted, now)
	got, err := scripted.getAccountsLoadBatchScript(context.Background(), scriptedAccounts, 2)
	require.NoError(t, err)
	require.Equal(t, want, got)
	require.Equal(t, int64(1), scripted.rdb.ZCard(context.Background(), accountSlotKey(101)).Val())
}

func TestConcurrencyCacheAccountLoadBatchScriptMatchesPipelineAcrossChunks(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	seed := func(t *testing.T, cache *concurrencyCache, size int) []service.AccountWithConcurrency {
		t.Helper()
		accounts := make([]service.AccountWithConcurrency, size)
		for start := 0; start < size; start += 1_000 {
			end := min(start+1_000, size)
			pipe := cache.rdb.Pipeline()
			for i := start; i < end; i++ {
				id := int64(i + 1)
				maxConcurrency := i%8 + 1
				if i%31 == 0 {
					maxConcurrency = 0
				}
				accounts[i] = service.AccountWithConcurrency{ID: id, MaxConcurrency: maxConcurrency}

				members := []redis.Z{{Score: float64(now.Unix() - 61), Member: "expired"}}
				for slot := 0; slot < i%4; slot++ {
					members = append(members, redis.Z{Score: float64(now.Unix()), Member: fmt.Sprintf("live-%d", slot)})
				}
				pipe.ZAdd(context.Background(), accountSlotKey(id), members...)
				switch i % 5 {
				case 1:
					pipe.Set(context.Background(), accountWaitKey(id), "2", time.Minute)
				case 2:
					pipe.Set(context.Background(), accountWaitKey(id), "-1", time.Minute)
				case 3:
					pipe.Set(context.Background(), accountWaitKey(id), "invalid", time.Minute)
				}
			}
			_, err := pipe.Exec(context.Background())
			require.NoError(t, err)
		}
		return accounts
	}

	legacy, _ := newAccountLoadBatchUnitCache(t, now)
	legacyAccounts := seed(t, legacy, 1_000)
	want, err := legacy.getAccountsLoadBatchPipeline(context.Background(), legacyAccounts)
	require.NoError(t, err)

	scripted, _ := newAccountLoadBatchUnitCache(t, now)
	scriptedAccounts := seed(t, scripted, 1_000)
	got, err := scripted.getAccountsLoadBatchScript(context.Background(), scriptedAccounts, defaultAccountLoadBatchScriptChunkSize)
	require.NoError(t, err)
	require.Equal(t, want, got)
	for _, index := range []int{0, 256, len(scriptedAccounts) - 1} {
		id := scriptedAccounts[index].ID
		require.Equal(t, int64(index%4), scripted.rdb.ZCard(context.Background(), accountSlotKey(id)).Val())
	}
}

func TestConcurrencyCacheAccountLoadBatchScriptMatchesNegativeWaitingCount(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	account := service.AccountWithConcurrency{ID: 104, MaxConcurrency: 3}

	legacy, _ := newAccountLoadBatchUnitCache(t, now)
	require.NoError(t, legacy.rdb.Set(context.Background(), accountWaitKey(account.ID), "-1", time.Minute).Err())
	want, err := legacy.getAccountsLoadBatchPipeline(context.Background(), []service.AccountWithConcurrency{account})
	require.NoError(t, err)

	scripted, _ := newAccountLoadBatchUnitCache(t, now)
	require.NoError(t, scripted.rdb.Set(context.Background(), accountWaitKey(account.ID), "-1", time.Minute).Err())
	got, err := scripted.getAccountsLoadBatchScript(context.Background(), []service.AccountWithConcurrency{account}, 1)
	require.NoError(t, err)

	require.Equal(t, want, got)
	require.Equal(t, -33, got[account.ID].LoadRate)
}

func TestConcurrencyCacheAccountLoadBatchScriptUsesBoundedChunks(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	cache, _ := newAccountLoadBatchUnitCache(t, now)
	cache.loadBatchScriptEnabled = true
	cache.loadBatchScriptChunkSize = 2
	hook := &accountLoadBatchPipelineHook{}
	cache.rdb.AddHook(hook)
	accounts := []service.AccountWithConcurrency{
		{ID: 201, MaxConcurrency: 1},
		{ID: 202, MaxConcurrency: 1},
		{ID: 203, MaxConcurrency: 1},
		{ID: 204, MaxConcurrency: 1},
		{ID: 205, MaxConcurrency: 1},
	}

	got, err := cache.GetAccountsLoadBatch(context.Background(), accounts)
	require.NoError(t, err)
	require.Len(t, got, len(accounts))
	require.Equal(t, int64(3), hook.evalCommands.Load())
}

func TestConcurrencyCacheAccountLoadBatchScriptFallsBackOnce(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	cache, _ := newAccountLoadBatchUnitCache(t, now)
	cache.loadBatchScriptEnabled = true
	hook := &accountLoadBatchPipelineHook{rejectEval: true}
	cache.rdb.AddHook(hook)
	accounts := seedAccountLoadBatchUnitData(t, cache, now)

	for range 2 {
		got, err := cache.GetAccountsLoadBatch(context.Background(), accounts)
		require.NoError(t, err)
		require.Len(t, got, len(accounts))
	}
	require.True(t, cache.loadBatchScriptDisabled.Load())
	require.Equal(t, int64(1), hook.evalCommands.Load())
}

func TestProvideConcurrencyCacheReadsLoadBatchScriptConfig(t *testing.T) {
	cache, _ := newAccountLoadBatchUnitCache(t, time.Now())
	cfg := &config.Config{}
	cfg.Gateway.ConcurrencySlotTTLMinutes = 1
	cfg.Gateway.Scheduling.LoadBatchScriptEnabled = true

	provided := ProvideConcurrencyCache(cache.rdb, cfg).(*concurrencyCache)
	require.True(t, provided.loadBatchScriptEnabled)
	require.Equal(t, defaultAccountLoadBatchScriptChunkSize, provided.loadBatchScriptChunkSize)
}
