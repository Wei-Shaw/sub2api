package repository

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
)

// 每个 concurrencyCache 代表一个进程（独立连接），共用同一个 Redis。
func newSharedRedisProcesses(t *testing.T, n int) (*miniredis.Miniredis, []*concurrencyCache) {
	t.Helper()
	server := miniredis.RunT(t)
	processes := make([]*concurrencyCache, 0, n)
	for i := 0; i < n; i++ {
		client := redis.NewClient(&redis.Options{Addr: server.Addr()})
		t.Cleanup(func() { _ = client.Close() })
		cache, ok := NewConcurrencyCache(client, 15, 900).(*concurrencyCache)
		require.True(t, ok)
		processes = append(processes, cache)
	}
	// 生产 Redis 早已完成一次性遗留等待计数清扫；预置 marker，只验证进程存活判定。
	require.NoError(t, processes[0].rdb.Set(context.Background(), legacyWaitSweepMarkerKey, "1", 0).Err())
	return server, processes
}

func requireSlotMembers(t *testing.T, cache *concurrencyCache, key string, expected ...string) {
	t.Helper()
	members, err := cache.rdb.ZRange(context.Background(), key, 0, -1).Result()
	require.NoError(t, err)
	if len(expected) == 0 {
		require.Empty(t, members, key)
		return
	}
	require.ElementsMatch(t, expected, members, key)
}

func waitCounter(t *testing.T, cache *concurrencyCache, key string) int {
	t.Helper()
	value, err := cache.rdb.Get(context.Background(), key).Int()
	if err == redis.Nil {
		return 0
	}
	require.NoError(t, err)
	return value
}

// 蓝绿发布：活跃槽进程正在服务时，候选槽进程启动，活跃槽的在途槽位与排队计数必须原样保留，
// 候选进程不能借计数归零超过账号/用户并发上限。
func TestStartupCleanupKeepsInFlightSlotsOfLivePeerProcess(t *testing.T) {
	ctx := context.Background()
	_, processes := newSharedRedisProcesses(t, 2)
	blue, green := processes[0], processes[1]

	require.NoError(t, blue.HeartbeatProcess(ctx, "rblue", "sub2api-blue"))
	acquired, err := blue.AcquireAccountSlot(ctx, 10, 1, "rblue-1")
	require.NoError(t, err)
	require.True(t, acquired)
	acquired, err = blue.AcquireUserSlot(ctx, 20, 1, "rblue-2")
	require.NoError(t, err)
	require.True(t, acquired)
	queued, err := blue.IncrementAccountWaitCount(ctx, 10, 5)
	require.NoError(t, err)
	require.True(t, queued)
	queued, err = blue.IncrementWaitCount(ctx, 20, 5)
	require.NoError(t, err)
	require.True(t, queued)

	require.NoError(t, green.HeartbeatProcess(ctx, "rgreen", "sub2api-green"))
	require.NoError(t, green.CleanupStaleProcessSlots(ctx, "rgreen"))

	requireSlotMembers(t, green, accountSlotKey(10), "rblue-1")
	requireSlotMembers(t, green, userSlotKey(20), "rblue-2")
	require.Equal(t, 1, waitCounter(t, green, accountWaitKey(10)))
	require.Equal(t, 1, waitCounter(t, green, waitQueueKey(20)))

	acquired, err = green.AcquireAccountSlot(ctx, 10, 1, "rgreen-1")
	require.NoError(t, err)
	require.False(t, acquired, "account limit must still count the live peer's in-flight request")
	acquired, err = green.AcquireUserSlot(ctx, 20, 1, "rgreen-2")
	require.NoError(t, err)
	require.False(t, acquired, "user limit must still count the live peer's in-flight request")
}

// 没有心跳的前缀（已死进程，或尚未带心跳的旧版本进程）照旧在启动时清掉，
// 只有存活同伴持槽的账号/用户才保留排队计数。
func TestStartupCleanupRemovesSlotsOfProcessWithoutHeartbeat(t *testing.T) {
	ctx := context.Background()
	_, processes := newSharedRedisProcesses(t, 3)
	dead, blue, green := processes[0], processes[1], processes[2]

	acquired, err := dead.AcquireAccountSlot(ctx, 10, 1, "rdead-1")
	require.NoError(t, err)
	require.True(t, acquired)
	acquired, err = dead.AcquireUserSlot(ctx, 20, 1, "rdead-2")
	require.NoError(t, err)
	require.True(t, acquired)
	_, err = dead.IncrementAccountWaitCount(ctx, 10, 5)
	require.NoError(t, err)
	_, err = dead.IncrementWaitCount(ctx, 20, 5)
	require.NoError(t, err)

	require.NoError(t, blue.HeartbeatProcess(ctx, "rblue", "sub2api-blue"))
	acquired, err = blue.AcquireAccountSlot(ctx, 11, 1, "rblue-1")
	require.NoError(t, err)
	require.True(t, acquired)
	_, err = blue.IncrementAccountWaitCount(ctx, 11, 5)
	require.NoError(t, err)

	require.NoError(t, green.HeartbeatProcess(ctx, "rgreen", "sub2api-green"))
	require.NoError(t, green.CleanupStaleProcessSlots(ctx, "rgreen"))

	requireSlotMembers(t, green, accountSlotKey(10))
	requireSlotMembers(t, green, userSlotKey(20))
	require.Zero(t, waitCounter(t, green, accountWaitKey(10)))
	require.Zero(t, waitCounter(t, green, waitQueueKey(20)))
	requireSlotMembers(t, green, accountSlotKey(11), "rblue-1")
	require.Equal(t, 1, waitCounter(t, green, accountWaitKey(11)))

	acquired, err = green.AcquireAccountSlot(ctx, 10, 1, "rgreen-1")
	require.NoError(t, err)
	require.True(t, acquired, "dead process residue must not keep holding capacity")
}

// 容器崩溃后立即重启：上一代进程的心跳还在窗口内，但同一实例身份只可能跑一个进程，
// 上一代的残留必须在新进程启动时清掉，不能等到槽位 TTL。
func TestStartupCleanupRemovesPreviousProcessOfSameInstance(t *testing.T) {
	ctx := context.Background()
	_, processes := newSharedRedisProcesses(t, 2)
	crashed, restarted := processes[0], processes[1]

	require.NoError(t, crashed.HeartbeatProcess(ctx, "rold", "sub2api-blue"))
	acquired, err := crashed.AcquireAccountSlot(ctx, 10, 1, "rold-1")
	require.NoError(t, err)
	require.True(t, acquired)
	_, err = crashed.IncrementAccountWaitCount(ctx, 10, 5)
	require.NoError(t, err)

	require.NoError(t, restarted.HeartbeatProcess(ctx, "rnew", "sub2api-blue"))
	require.NoError(t, restarted.CleanupStaleProcessSlots(ctx, "rnew"))

	requireSlotMembers(t, restarted, accountSlotKey(10))
	require.Zero(t, waitCounter(t, restarted, accountWaitKey(10)))
	requireSlotMembers(t, restarted, processHeartbeatKey, "rnew@sub2api-blue")
}

// 进程停止心跳超过存活窗口即视为已死，之后启动的进程清掉它的残留。
func TestStartupCleanupRemovesSlotsOfExpiredHeartbeat(t *testing.T) {
	ctx := context.Background()
	server, processes := newSharedRedisProcesses(t, 2)
	blue, green := processes[0], processes[1]

	start := time.Now()
	server.SetTime(start)
	require.NoError(t, blue.HeartbeatProcess(ctx, "rblue", "sub2api-blue"))
	acquired, err := blue.AcquireAccountSlot(ctx, 10, 1, "rblue-1")
	require.NoError(t, err)
	require.True(t, acquired)

	server.SetTime(start.Add((processHeartbeatWindowSeconds + 1) * time.Second))
	require.NoError(t, green.HeartbeatProcess(ctx, "rgreen", "sub2api-green"))
	require.NoError(t, green.CleanupStaleProcessSlots(ctx, "rgreen"))

	requireSlotMembers(t, green, accountSlotKey(10))
	requireSlotMembers(t, green, processHeartbeatKey, "rgreen@sub2api-green")
}
