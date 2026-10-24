package repository

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
)

func testAPIKeyAdmissionCompetition(t *testing.T, clients []*redis.Client) {
	t.Helper()
	ctx, cancel := service.WithAPIKeyAdmissionOwner(context.Background())
	defer cancel()
	services := make([]*service.ConcurrencyService, len(clients))
	for i, client := range clients {
		services[i] = service.NewConcurrencyService(NewConcurrencyCache(client, 1, 60))
	}
	const limit = 5
	for _, keyLimit := range []int{limit, 0} {
		start := make(chan struct{})
		results := make(chan *service.AcquireResult, 64)
		errors := make(chan error, 64)
		var workers sync.WaitGroup
		for i := 0; i < 64; i++ {
			workers.Add(1)
			go func(i int) {
				defer workers.Done()
				<-start
				result, err := services[i%len(services)].AcquireAPIKeySlot(ctx, 901, keyLimit)
				errors <- err
				results <- result
			}(i)
		}
		close(start)
		workers.Wait()
		close(results)
		close(errors)
		for err := range errors {
			require.NoError(t, err)
		}
		var releases []func()
		for result := range results {
			require.NotNil(t, result)
			if result.Acquired {
				releases = append(releases, result.ReleaseFunc)
			}
		}
		want := limit
		if keyLimit == 0 {
			want = 64
		}
		require.Len(t, releases, want, "no slot may be over-admitted while winners still hold their leases")
		counts, err := services[0].GetAPIKeyConcurrencyBatch(ctx, []int64{901})
		require.NoError(t, err)
		require.Equal(t, want, counts[901])
		other, err := services[1].AcquireAPIKeySlot(ctx, 902, 1)
		require.NoError(t, err)
		require.True(t, other.Acquired, "another key is independent")
		other.ReleaseFunc()
		for _, release := range releases {
			release()
			release()
		}
		next, err := services[1].AcquireAPIKeySlot(ctx, 901, 1)
		require.NoError(t, err)
		require.True(t, next.Acquired, "release restores admission")
		next.ReleaseFunc()
	}
}

func TestAPIKeyAdmissionAtomicCompetition(t *testing.T) {
	server := miniredis.RunT(t)
	clients := []*redis.Client{redis.NewClient(&redis.Options{Addr: server.Addr()}), redis.NewClient(&redis.Options{Addr: server.Addr()})}
	for _, client := range clients {
		t.Cleanup(func() { _ = client.Close() })
	}
	testAPIKeyAdmissionCompetition(t, clients)
	testAPIKeyLiveAdmission(t, clients)
}

func testAPIKeyLiveAdmission(t *testing.T, clients []*redis.Client) {
	t.Helper()
	ctx := context.Background()
	cache, cacheOK := NewConcurrencyCache(clients[0], 1, 60).(*concurrencyCache)
	require.True(t, cacheOK)
	other, otherOK := NewConcurrencyCache(clients[1], 1, 60).(*concurrencyCache)
	require.True(t, otherOK)
	start := make(chan struct{})
	results := make(chan error, 2)
	for i, client := range []*concurrencyCache{cache, other} {
		go func(i int, client *concurrencyCache) {
			<-start
			ok, err := client.AcquireLiveLease(ctx, int64(910+i), 0, int64(920+i), 0, 904, 2, []string{"pending-sdp-a", "pending-sdp-b"}[i], true)
			if err == nil && !ok {
				err = service.ErrAPIKeyConcurrencyLimit
			}
			results <- err
		}(i, client)
	}
	close(start)
	require.NoError(t, <-results)
	require.NoError(t, <-results)
	counts, err := cache.GetAPIKeyConcurrencyBatch(ctx, []int64{904})
	require.NoError(t, err)
	require.Equal(t, 2, counts[904], "two pending SDP requests each own exactly one member")
	ok, err := other.AcquireLiveLease(ctx, 912, 0, 922, 0, 904, 2, "third-sdp", true)
	require.ErrorIs(t, err, service.ErrAPIKeyConcurrencyLimit)
	require.False(t, ok, "regular-slot handoff allowance must not bypass key capacity")
	ok, err = other.AcquireAPIKeySlot(ctx, 904, 2, "normal")
	require.NoError(t, err)
	require.False(t, ok)
	require.NoError(t, cache.ReleaseLiveLease(ctx, 910, 920, 904, "pending-sdp-a"))
	ok, err = other.AcquireAPIKeySlot(ctx, 904, 2, "normal")
	require.NoError(t, err)
	require.True(t, ok)
	counts, err = cache.GetAPIKeyConcurrencyBatch(ctx, []int64{904})
	require.NoError(t, err)
	require.Equal(t, 2, counts[904], "one Live plus one ordinary request fills limit two")
	require.NoError(t, other.ReleaseAPIKeySlot(ctx, 904, "normal"))
	require.NoError(t, cache.ReleaseLiveLease(ctx, 911, 921, 904, "pending-sdp-b"))
}

func TestAPIKeyAdmissionPruningAndSameRequest(t *testing.T) {
	server := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: server.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })
	cache, cacheOK := NewConcurrencyCache(rdb, 1, 60).(*concurrencyCache)
	require.True(t, cacheOK)
	ctx := context.Background()
	for i := 0; i < 2; i++ {
		ok, err := cache.AcquireAPIKeySlot(ctx, 903, 1, "same-request")
		require.NoError(t, err)
		require.True(t, ok)
	}
	require.EqualValues(t, 1, rdb.ZCard(ctx, apiKeySlotKey(903)).Val())
	ok, err := cache.AcquireAPIKeySlot(ctx, 903, 1, "other-request")
	require.NoError(t, err)
	require.False(t, ok)
	require.NoError(t, rdb.ZAdd(ctx, apiKeySlotKey(903), redis.Z{Score: float64(time.Now().Add(-2 * time.Minute).Unix()), Member: "same-request"}).Err())
	ok, err = cache.AcquireAPIKeySlot(ctx, 903, 1, "other-request")
	require.NoError(t, err)
	require.True(t, ok, "stale member must be pruned before count")
}

func TestAPIKeyAdmissionUnavailableRedis(t *testing.T) {
	server := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: server.Addr()})
	cache := NewConcurrencyCache(rdb, 1, 60)
	require.NoError(t, rdb.Close())
	ctx, cancel := service.WithAPIKeyAdmissionOwner(context.Background())
	defer cancel()
	result, err := service.NewConcurrencyService(cache).AcquireAPIKeySlot(ctx, 903, 1)
	require.ErrorIs(t, err, redis.ErrClosed)
	require.Nil(t, result)
}

func TestAPIKeyAdmissionLiveSessionsShareLimit(t *testing.T) {
	server := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: server.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })
	cache, cacheOK := NewConcurrencyCache(rdb, 1, 60).(*concurrencyCache)
	require.True(t, cacheOK)
	ctx := context.Background()
	ok, err := cache.AcquireLiveLease(ctx, 10, 5, 20, 5, 30, 1, "live", false)
	require.NoError(t, err)
	require.True(t, ok)
	ok, err = cache.AcquireAPIKeySlot(ctx, 30, 1, "http")
	require.NoError(t, err)
	require.False(t, ok)
	ok, err = cache.AcquireLiveLease(ctx, 11, 5, 21, 5, 30, 1, "live-other", false)
	require.ErrorIs(t, err, service.ErrAPIKeyConcurrencyLimit)
	require.False(t, ok)
	require.NoError(t, cache.ReleaseLiveLease(ctx, 10, 20, 30, "live"))
	ok, err = cache.AcquireAPIKeySlot(ctx, 30, 1, "http")
	require.NoError(t, err)
	require.True(t, ok)
	ok, err = cache.AcquireLiveLease(ctx, 11, 5, 21, 5, 30, 1, "live-other", false)
	require.ErrorIs(t, err, service.ErrAPIKeyConcurrencyLimit)
	require.False(t, ok)
}
