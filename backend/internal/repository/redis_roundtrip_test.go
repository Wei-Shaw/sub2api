//go:build unit

package repository

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
)

// rttCountingHook counts Process/ProcessPipeline invocations, i.e. network
// round trips (a pipeline is one round trip regardless of command count).
type rttCountingHook struct {
	mu    sync.Mutex
	calls int
}

func (h *rttCountingHook) DialHook(next redis.DialHook) redis.DialHook { return next }

func (h *rttCountingHook) ProcessHook(next redis.ProcessHook) redis.ProcessHook {
	return func(ctx context.Context, cmd redis.Cmder) error {
		h.mu.Lock()
		h.calls++
		h.mu.Unlock()
		return next(ctx, cmd)
	}
}

func (h *rttCountingHook) ProcessPipelineHook(next redis.ProcessPipelineHook) redis.ProcessPipelineHook {
	return func(ctx context.Context, cmds []redis.Cmder) error {
		h.mu.Lock()
		h.calls++
		h.mu.Unlock()
		return next(ctx, cmds)
	}
}

func (h *rttCountingHook) reset() {
	h.mu.Lock()
	h.calls = 0
	h.mu.Unlock()
}

func (h *rttCountingHook) count() int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.calls
}

func newRedisWithRTTHook(t *testing.T) (*miniredis.Miniredis, *redis.Client, *rttCountingHook) {
	t.Helper()
	mr := miniredis.RunT(t)
	hook := &rttCountingHook{}
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	rdb.AddHook(hook)
	t.Cleanup(func() { _ = rdb.Close() })
	return mr, rdb, hook
}

func TestRPMIncrementUsesOneRoundTrip(t *testing.T) {
	mr, rdb, hook := newRedisWithRTTHook(t)
	ctx := context.Background()
	require.NoError(t, incrementRPMScript.Load(ctx, rdb).Err())
	cache := &RPMCacheImpl{rdb: rdb}

	hook.reset()
	count, err := cache.IncrementRPM(ctx, 42)
	require.NoError(t, err)
	require.Equal(t, 1, count)
	count, err = cache.IncrementRPM(ctx, 42)
	require.NoError(t, err)
	require.Equal(t, 2, count)
	require.Equal(t, 2, hook.count(), "each increment must be one round trip")

	keys := mr.Keys()
	require.Len(t, keys, 1)
	require.Regexp(t, `^rpm:42:\d+$`, keys[0])
	require.Greater(t, mr.TTL(keys[0]), time.Duration(0))
}

func TestUserRPMIncrementUsesOneRoundTrip(t *testing.T) {
	mr, rdb, hook := newRedisWithRTTHook(t)
	ctx := context.Background()
	require.NoError(t, incrementRPMScript.Load(ctx, rdb).Err())
	cache := &userRPMCacheImpl{rdb: rdb}

	hook.reset()
	groupCount, err := cache.IncrementUserGroupRPM(ctx, 7, 9)
	require.NoError(t, err)
	require.Equal(t, 1, groupCount)
	userCount, err := cache.IncrementUserRPM(ctx, 7)
	require.NoError(t, err)
	require.Equal(t, 1, userCount)
	require.Equal(t, 2, hook.count(), "each increment must be one round trip")

	keys := mr.Keys()
	require.Len(t, keys, 2)
}

func TestReleaseAccountSlotUsesTwoRoundTrips(t *testing.T) {
	_, rdb, hook := newRedisWithRTTHook(t)
	ctx := context.Background()
	require.NoError(t, releaseSlotScript.Load(ctx, rdb).Err())
	cache := NewConcurrencyCache(rdb, 15, 900).(*concurrencyCache)

	acquired, err := cache.AcquireAccountSlot(ctx, 7, 10, "req-1")
	require.NoError(t, err)
	require.True(t, acquired)

	hook.reset()
	require.NoError(t, cache.ReleaseAccountSlot(ctx, 7, "req-1"))
	require.Equal(t, 2, hook.count(), "release must be Lua + one index command")

	slotCount, err := rdb.ZCard(ctx, accountSlotKey(7)).Result()
	require.NoError(t, err)
	require.Zero(t, slotCount)
	_, err = rdb.ZScore(ctx, accountActiveIndexKey, "7").Result()
	require.ErrorIs(t, err, redis.Nil, "empty account must be removed from the active index")
}

func TestReleaseUserSlotUsesTwoRoundTrips(t *testing.T) {
	_, rdb, hook := newRedisWithRTTHook(t)
	ctx := context.Background()
	require.NoError(t, releaseSlotScript.Load(ctx, rdb).Err())
	cache := NewConcurrencyCache(rdb, 15, 900).(*concurrencyCache)

	acquired, err := cache.AcquireUserSlot(ctx, 7, 10, "req-1")
	require.NoError(t, err)
	require.True(t, acquired)

	hook.reset()
	require.NoError(t, cache.ReleaseUserSlot(ctx, 7, "req-1"))
	require.Equal(t, 2, hook.count(), "release must be Lua + one index command")

	slotCount, err := rdb.ZCard(ctx, userSlotKey(7)).Result()
	require.NoError(t, err)
	require.Zero(t, slotCount)
	_, err = rdb.ZScore(ctx, userActiveIndexKey, "7").Result()
	require.ErrorIs(t, err, redis.Nil, "empty user must be removed from the active index")
}
