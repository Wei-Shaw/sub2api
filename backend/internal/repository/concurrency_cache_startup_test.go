package repository

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
)

func TestStartupCleanupPreservesSharedLeasesAndWaiters(t *testing.T) {
	for _, markerPresent := range []bool{false, true} {
		name := "first-start"
		if markerPresent {
			name = "subsequent-start"
		}
		t.Run(name, func(t *testing.T) {
			ctx := context.Background()
			server := miniredis.RunT(t)
			// Startup must use Redis time, even when the process clock differs.
			server.SetTime(time.Unix(1700000000, 0))
			client := redis.NewClient(&redis.Options{Addr: server.Addr()})
			t.Cleanup(func() { _ = client.Close() })
			cache := NewConcurrencyCache(client, 15, 60).(*concurrencyCache)
			if markerPresent {
				require.NoError(t, client.Set(ctx, legacyWaitSweepMarkerKey, "1", 0).Err())
			}
			now := client.Time(ctx).Val().Unix()
			for _, spec := range []slotIndexSpec{accountSlotIndex, userSlotIndex} {
				require.NoError(t, client.ZAdd(ctx, spec.slotKey(123),
					redis.Z{Score: float64(now), Member: "peer-without-heartbeat"},
					redis.Z{Score: float64(now - 899), Member: "unprefixed-legacy-request"},
					redis.Z{Score: float64(now - 900), Member: "starting-process-expired"},
					redis.Z{Score: float64(now - 901), Member: "peer-expired"},
				).Err())
				// An expired index score does not establish that its load is dead.
				require.NoError(t, client.ZAdd(ctx, spec.indexKey, redis.Z{Score: float64(now - 1), Member: "123"}).Err())
				require.NoError(t, client.Set(ctx, spec.waitKey(123), "2", 30*time.Second).Err())
				// Queue-only candidates also need to survive startup. Preserve a TTL
				// longer than this process's configured wait TTL as well.
				require.NoError(t, client.ZAdd(ctx, spec.indexKey, redis.Z{Score: float64(now - 1), Member: "456"}).Err())
				require.NoError(t, client.Set(ctx, spec.waitKey(456), "3", 120*time.Second).Err())
			}
			for _, prefix := range []string{"starting-process-", "another-process-"} {
				require.NoError(t, cache.CleanupStaleProcessSlots(ctx, prefix))
				for _, spec := range []slotIndexSpec{accountSlotIndex, userSlotIndex} {
					require.Equal(t, []string{"unprefixed-legacy-request", "peer-without-heartbeat"}, client.ZRange(ctx, spec.slotKey(123), 0, -1).Val())
					require.Equal(t, "2", client.Get(ctx, spec.waitKey(123)).Val())
					require.Equal(t, 30*time.Second, client.TTL(ctx, spec.waitKey(123)).Val())
					require.Equal(t, "3", client.Get(ctx, spec.waitKey(456)).Val())
					require.Equal(t, 120*time.Second, client.TTL(ctx, spec.waitKey(456)).Val())
					_, err := client.ZScore(ctx, spec.indexKey, "456").Result()
					require.NoError(t, err, "queue-only member must remain discoverable")
				}
			}
			server.FastForward(121 * time.Second)
			for _, spec := range []slotIndexSpec{accountSlotIndex, userSlotIndex} {
				require.Zero(t, client.Exists(ctx, spec.waitKey(123), spec.waitKey(456)).Val())
			}
		})
	}
}

func TestStartupCleanupBoundsOnlyNonexpiringWaiters(t *testing.T) {
	for _, markerPresent := range []bool{false, true} {
		ctx := context.Background()
		server := miniredis.RunT(t)
		client := redis.NewClient(&redis.Options{Addr: server.Addr()})
		t.Cleanup(func() { _ = client.Close() })
		cache := NewConcurrencyCache(client, 15, 60).(*concurrencyCache)
		if markerPresent {
			require.NoError(t, client.Set(ctx, legacyWaitSweepMarkerKey, "1", 0).Err())
		}
		for _, spec := range []slotIndexSpec{accountSlotIndex, userSlotIndex} {
			require.NoError(t, client.ZAdd(ctx, spec.indexKey, redis.Z{Score: 1, Member: "123"}).Err())
			require.NoError(t, client.Set(ctx, spec.waitKey(123), "2", 0).Err())
			if !markerPresent {
				// Legacy counters written before the active index existed.
				require.NoError(t, client.Set(ctx, spec.waitKey(456), "3", 0).Err())
			}
		}
		require.NoError(t, cache.CleanupStaleProcessSlots(ctx, "starting-process-"))
		server.FastForward(10 * time.Second)
		require.NoError(t, cache.CleanupStaleProcessSlots(ctx, "another-process-"))
		for _, spec := range []slotIndexSpec{accountSlotIndex, userSlotIndex} {
			require.Equal(t, "2", client.Get(ctx, spec.waitKey(123)).Val())
			require.Equal(t, 50*time.Second, client.TTL(ctx, spec.waitKey(123)).Val(), "restarts must not refresh an existing TTL")
			_, err := client.ZScore(ctx, spec.indexKey, "123").Result()
			require.NoError(t, err)
			if !markerPresent {
				require.Equal(t, "3", client.Get(ctx, spec.waitKey(456)).Val())
				require.Equal(t, 50*time.Second, client.TTL(ctx, spec.waitKey(456)).Val())
			}
		}
		server.FastForward(51 * time.Second)
		for _, spec := range []slotIndexSpec{accountSlotIndex, userSlotIndex} {
			require.Zero(t, client.Exists(ctx, spec.waitKey(123), spec.waitKey(456)).Val())
		}
	}
}

// afterSlotCleanupHook starts a peer request after startup observes an empty
// slot set, before it can reconcile the separately stored active index.
type afterSlotCleanupHook struct {
	key   string
	after func()
}

func (h *afterSlotCleanupHook) DialHook(next redis.DialHook) redis.DialHook { return next }
func (h *afterSlotCleanupHook) ProcessPipelineHook(next redis.ProcessPipelineHook) redis.ProcessPipelineHook {
	return next
}
func (h *afterSlotCleanupHook) ProcessHook(next redis.ProcessHook) redis.ProcessHook {
	return func(ctx context.Context, cmd redis.Cmder) error {
		err := next(ctx, cmd)
		args := cmd.Args()
		if err == nil && h.after != nil && (cmd.Name() == "eval" || cmd.Name() == "evalsha") && len(args) > 3 && args[3] == h.key {
			after := h.after
			h.after = nil
			after()
		}
		return err
	}
}

func TestStartupCleanupDoesNotRemoveConcurrentPeerIndex(t *testing.T) {
	for _, spec := range []slotIndexSpec{accountSlotIndex, userSlotIndex} {
		t.Run(spec.indexKey, func(t *testing.T) {
			ctx := context.Background()
			server := miniredis.RunT(t)
			client := redis.NewClient(&redis.Options{Addr: server.Addr()})
			peer := redis.NewClient(&redis.Options{Addr: server.Addr()})
			t.Cleanup(func() { _ = client.Close(); _ = peer.Close() })
			cache := NewConcurrencyCache(client, 15, 60).(*concurrencyCache)
			now := client.Time(ctx).Val().Unix()
			require.NoError(t, client.ZAdd(ctx, spec.slotKey(123), redis.Z{Score: float64(now - 901), Member: "dead-peer"}).Err())
			require.NoError(t, client.ZAdd(ctx, spec.indexKey, redis.Z{Score: 1, Member: "123"}).Err())
			started := false
			client.AddHook(&afterSlotCleanupHook{key: spec.slotKey(123), after: func() {
				started = true
				require.NoError(t, peer.ZAdd(ctx, spec.slotKey(123), redis.Z{Score: float64(now), Member: "new-peer-request"}).Err())
				require.NoError(t, peer.ZAdd(ctx, spec.indexKey, redis.Z{Score: float64(now + 900), Member: "123"}).Err())
			}})
			require.NoError(t, cache.CleanupStaleProcessSlots(ctx, "starting-process-"))
			require.True(t, started)
			require.Equal(t, []string{"new-peer-request"}, client.ZRange(ctx, spec.slotKey(123), 0, -1).Val())
			_, err := client.ZScore(ctx, spec.indexKey, "123").Result()
			require.NoError(t, err, "startup must not erase a concurrently registered peer")
		})
	}
}
