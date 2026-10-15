package repository

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
)

func TestAccountAdmissionBurstRotation(t *testing.T) {
	for _, limit := range []int{1, 2, 3} {
		t.Run(fmt.Sprint(limit), func(t *testing.T) {
			cache, _ := newContinuationWaitTestCache(t)
			ctx := context.Background()
			ok, err := cache.IncrementAccountWaitCount(ctx, 7, 10)
			require.NoError(t, err)
			require.True(t, ok)
			ok, err = cache.IncrementAccountContinuationWaitCount(ctx, 7, 10)
			require.NoError(t, err)
			require.True(t, ok)
			for cycle := range 2 {
				for i := range limit {
					ok, err = cache.AcquireAccountSlotForClass(ctx, 7, 1, "new", service.AccountWaitClassNewSession, limit)
					require.NoError(t, err)
					require.False(t, ok)
					id := fmt.Sprintf("cont-%d-%d", cycle, i)
					ok, err = cache.AcquireAccountSlotForClass(ctx, 7, 1, id, service.AccountWaitClassContinuation, limit)
					require.NoError(t, err)
					require.True(t, ok)
					ok, err = cache.AcquireAccountSlotForClass(ctx, 7, 1, id, service.AccountWaitClassContinuation, limit)
					require.NoError(t, err)
					require.True(t, ok, "同一次获槽重试不重复计数")
					ok, err = cache.AcquireAccountSlotForClass(ctx, 7, 1, "full", service.AccountWaitClassContinuation, limit)
					require.NoError(t, err)
					require.False(t, ok, "满槽失败不计数")
					require.NoError(t, cache.ReleaseAccountSlot(ctx, 7, id))
				}
				ok, err = cache.AcquireAccountSlotForClass(ctx, 7, 1, "extra", service.AccountWaitClassContinuation, limit)
				require.NoError(t, err)
				require.False(t, ok, "有空槽也必须让一次新会话")
				ok, err = cache.AcquireAccountSlotForClass(ctx, 7, 1, "new", service.AccountWaitClassNewSession, limit)
				require.NoError(t, err)
				require.True(t, ok)
				state, err := cache.GetAccountAdmissionState(ctx, 7)
				require.NoError(t, err)
				require.Zero(t, state.ContinuationBurst)
				require.NoError(t, cache.ReleaseAccountSlot(ctx, 7, "new"))
			}
		})
	}
}

func TestAccountAdmissionReuseCountsButRefreshDoesNot(t *testing.T) {
	cache, client := newContinuationWaitTestCache(t)
	ctx := context.Background()
	ok, err := cache.AcquireAccountSlotForClass(ctx, 8, 1, "ws", service.AccountWaitClassNewSession, 2)
	require.NoError(t, err)
	require.True(t, ok)
	ok, err = cache.IncrementAccountWaitCount(ctx, 8, 10)
	require.NoError(t, err)
	require.True(t, ok)
	for _, turn := range []string{"turn-2", "turn-3"} {
		for range 2 {
			ok, err = cache.ReuseAccountSlot(ctx, 8, "ws", turn, 2)
			require.NoError(t, err)
			require.True(t, ok, "同一次复用的 Redis 重试幂等")
			ok, err = cache.RefreshAccountSlot(ctx, 8, "ws")
			require.NoError(t, err)
			require.True(t, ok)
		}
	}
	state, err := cache.GetAccountAdmissionState(ctx, 8)
	require.NoError(t, err)
	require.Equal(t, 2, state.ContinuationBurst)
	ok, err = cache.ReuseAccountSlot(ctx, 8, "ws", "turn-4", 2)
	require.ErrorIs(t, err, service.ErrAccountSlotYieldToNewSession)
	require.False(t, ok)
	require.Equal(t, int64(1), client.ZCard(ctx, accountSlotKey(8)).Val(), "拒绝复用后由持有者释放，不打断执行中轮次")
	require.NoError(t, cache.ReleaseAccountSlot(ctx, 8, "ws"))
	ok, err = cache.ReuseAccountSlot(ctx, 8, "ws", "turn-3", 2)
	require.NoError(t, err)
	require.False(t, ok, "释放后即使重放成功 token 也不能复活")
	require.Zero(t, client.Exists(ctx, accountSlotReuseKey(8, "ws")).Val())
}

func TestAccountAdmissionConcurrentClientsShareOneBurst(t *testing.T) {
	cache, client := newContinuationWaitTestCache(t)
	other, supported := NewConcurrencyCache(client, 15, 900).(*concurrencyCache)
	require.True(t, supported)
	ctx := context.Background()
	ok, err := cache.IncrementAccountWaitCount(ctx, 9, 10)
	require.NoError(t, err)
	require.True(t, ok)
	ok, err = cache.IncrementAccountContinuationWaitCount(ctx, 9, 10)
	require.NoError(t, err)
	require.True(t, ok)
	run := func(class service.AccountWaitClass) int32 {
		var acquired atomic.Int32
		var wg sync.WaitGroup
		for i := range 20 {
			wg.Go(func() {
				c := cache
				if i%2 == 0 {
					c = other
				}
				ok, err := c.AcquireAccountSlotForClass(ctx, 9, 30, fmt.Sprintf("%s-%d", class, i), class, 2)
				if err != nil {
					t.Error(err)
				} else if ok {
					acquired.Add(1)
				}
			})
		}
		wg.Wait()
		return acquired.Load()
	}
	require.Equal(t, int32(2), run(service.AccountWaitClassContinuation))
	require.Equal(t, int32(1), run(service.AccountWaitClassNewSession), "只有一个新会话可使用本次优先机会")
}

func TestAccountAdmissionNoCompetitionAndQueueReset(t *testing.T) {
	cache, _ := newContinuationWaitTestCache(t)
	ctx := context.Background()
	for i := range 6 {
		ok, err := cache.AcquireAccountSlotForClass(ctx, 10, 10, fmt.Sprint(i), service.AccountWaitClassContinuation, 2)
		require.NoError(t, err)
		require.True(t, ok, "无新会话等待时不受 N 限制")
	}
	state, err := cache.GetAccountAdmissionState(ctx, 10)
	require.NoError(t, err)
	require.Zero(t, state.ContinuationBurst)
	ok, err := cache.IncrementAccountWaitCount(ctx, 10, 10)
	require.NoError(t, err)
	require.True(t, ok)
	for _, id := range []string{"6", "7"} {
		ok, err = cache.AcquireAccountSlotForClass(ctx, 10, 10, id, service.AccountWaitClassContinuation, 2)
		require.NoError(t, err)
		require.True(t, ok)
	}
	require.NoError(t, cache.DecrementAccountWaitCount(ctx, 10))
	state, err = cache.GetAccountAdmissionState(ctx, 10)
	require.NoError(t, err)
	require.Zero(t, state.ContinuationBurst)
	ok, err = cache.IncrementAccountWaitCount(ctx, 10, 10)
	require.NoError(t, err)
	require.True(t, ok)
	ok, err = cache.AcquireAccountSlotForClass(ctx, 10, 10, "8", service.AccountWaitClassContinuation, 2)
	require.NoError(t, err)
	require.True(t, ok, "新队列从 0 开始")
	ok, err = cache.AcquireAccountSlotForClass(ctx, 11, 1, "independent", service.AccountWaitClassContinuation, 2)
	require.NoError(t, err)
	require.True(t, ok, "账号之间不共用计数")
}

func TestAccountAdmissionDisabledPreservesContinuationPriority(t *testing.T) {
	cache, _ := newContinuationWaitTestCache(t)
	ctx := context.Background()
	ok, err := cache.IncrementAccountWaitCount(ctx, 12, 10)
	require.NoError(t, err)
	require.True(t, ok)
	ok, err = cache.IncrementAccountContinuationWaitCount(ctx, 12, 10)
	require.NoError(t, err)
	require.True(t, ok)
	for i := range 5 {
		ok, err = cache.AcquireAccountSlotForClass(ctx, 12, 10, fmt.Sprint(i), service.AccountWaitClassContinuation, 0)
		require.NoError(t, err)
		require.True(t, ok)
	}
	ok, err = cache.AcquireAccountSlotForClass(ctx, 12, 10, "new", service.AccountWaitClassNewSession, 0)
	require.NoError(t, err)
	require.False(t, ok)
	ok, err = cache.AcquireAccountSlotForClass(ctx, 12, 10, "legacy", service.AccountWaitClassLegacy, 2)
	require.NoError(t, err)
	require.True(t, ok, "未分类路径保持原行为")
}

func TestAccountAdmissionExpiredQueueAndLease(t *testing.T) {
	cache, client, server := newRefreshTestCache(t)
	ctx := context.Background()
	ok, err := cache.AcquireAccountSlotForClass(ctx, 13, 1, "ws", service.AccountWaitClassNewSession, 2)
	require.NoError(t, err)
	require.True(t, ok)
	ok, err = cache.IncrementAccountWaitCount(ctx, 13, 10)
	require.NoError(t, err)
	require.True(t, ok)
	ok, err = cache.ReuseAccountSlot(ctx, 13, "ws", "2", 2)
	require.NoError(t, err)
	require.True(t, ok)
	require.Positive(t, client.TTL(ctx, accountContinuationBurstKey(13)).Val())
	server.FastForward(16 * time.Minute)
	ok, err = cache.IncrementAccountWaitCount(ctx, 13, 10)
	require.NoError(t, err)
	require.True(t, ok)
	state, err := cache.GetAccountAdmissionState(ctx, 13)
	require.NoError(t, err)
	require.Zero(t, state.ContinuationBurst, "过期队列没有历史欠额")
	ok, err = cache.ReuseAccountSlot(ctx, 13, "ws", "3", 2)
	require.NoError(t, err)
	require.False(t, ok, "槽已过期，不创建槽或增加计数")
	state, err = cache.GetAccountAdmissionState(ctx, 13)
	require.NoError(t, err)
	require.Zero(t, state.ContinuationBurst)
}

func TestAccountAdmissionRespectsLiveSlots(t *testing.T) {
	cache, client := newContinuationWaitTestCache(t)
	ctx := context.Background()
	ok, err := cache.IncrementAccountWaitCount(ctx, 14, 10)
	require.NoError(t, err)
	require.True(t, ok)
	require.NoError(t, client.ZAdd(ctx, liveAccountSlotKey(14), redis.Z{Score: float64(time.Now().Unix()), Member: "live"}).Err())
	ok, err = cache.AcquireAccountSlotForClass(ctx, 14, 1, "cont", service.AccountWaitClassContinuation, 2)
	require.NoError(t, err)
	require.False(t, ok)
	state, err := cache.GetAccountAdmissionState(ctx, 14)
	require.NoError(t, err)
	require.Zero(t, state.ContinuationBurst)
	require.NoError(t, client.Del(ctx, liveAccountSlotKey(14)).Err())
	ok, err = cache.AcquireAccountSlotForClass(ctx, 14, 1, "cont", service.AccountWaitClassContinuation, 2)
	require.NoError(t, err)
	require.True(t, ok)
}

func TestAccountAdmissionQueueRestartClearsStaleBurst(t *testing.T) {
	cache, client, server := newRefreshTestCache(t)
	ctx := context.Background()
	require.NoError(t, client.Set(ctx, accountWaitKey(15), 1, time.Second).Err())
	require.NoError(t, client.Set(ctx, accountContinuationBurstKey(15), 2, time.Minute).Err())
	server.FastForward(2 * time.Second)
	ok, err := cache.IncrementAccountWaitCount(ctx, 15, 10)
	require.NoError(t, err)
	require.True(t, ok)
	state, err := cache.GetAccountAdmissionState(ctx, 15)
	require.NoError(t, err)
	require.Zero(t, state.ContinuationBurst)

	require.NoError(t, client.Set(ctx, accountContinuationBurstKey(15), 1, time.Second).Err())
	ok, err = cache.IncrementAccountWaitCount(ctx, 15, 10)
	require.NoError(t, err)
	require.True(t, ok)
	server.FastForward(2 * time.Second)
	state, err = cache.GetAccountAdmissionState(ctx, 15)
	require.NoError(t, err)
	require.Equal(t, 1, state.ContinuationBurst, "追加等待者延长队列 TTL 时同步延长计数 TTL")
	require.NoError(t, cache.DecrementAccountWaitCount(ctx, 15))
	state, err = cache.GetAccountAdmissionState(ctx, 15)
	require.NoError(t, err)
	require.Equal(t, 1, state.ContinuationBurst, "部分等待者离队不清零")
}
