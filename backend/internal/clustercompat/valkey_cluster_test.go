package clustercompat_test

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/repository"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
)

func TestValkeyClusterCompatibility(t *testing.T) {
	if os.Getenv("SUB2API_TEST_VALKEY_CLUSTER_DESTRUCTIVE") != "1" {
		t.Skip("set SUB2API_TEST_VALKEY_CLUSTER_DESTRUCTIVE=1 for a disposable cluster")
	}
	addresses := splitAddresses(os.Getenv("SUB2API_TEST_VALKEY_CLUSTER_ADDRESSES"))
	require.NotEmpty(t, addresses)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	client := redis.NewClusterClient(&redis.ClusterOptions{Addrs: addresses})
	t.Cleanup(func() { require.NoError(t, client.Close()) })
	require.NoError(t, client.ForEachMaster(ctx, func(ctx context.Context, node *redis.Client) error {
		return node.FlushDB(ctx).Err()
	}))
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cleanupCancel()
		_ = client.ForEachMaster(cleanupCtx, func(ctx context.Context, node *redis.Client) error {
			return node.FlushDB(ctx).Err()
		})
	})

	t.Run("cluster wide legacy scan", func(t *testing.T) {
		require.NoError(t, client.Set(ctx, "wait:account:901", "1", time.Hour).Err())
		require.NoError(t, client.Set(ctx, "concurrency:wait:902", "1", time.Hour).Err())

		cache := repository.NewConcurrencyCache(client, 15, 900)
		require.NoError(t, cache.CleanupStaleProcessSlots(ctx, "active-request:"))
		for _, key := range []string{"wait:account:901", "concurrency:wait:902"} {
			exists, err := client.Exists(ctx, key).Result()
			require.NoError(t, err)
			require.Zero(t, exists)
		}
	})

	t.Run("multi key concurrency scripts", func(t *testing.T) {
		cache := repository.NewConcurrencyCache(client, 15, 900)
		acquired, err := cache.AcquireAccountSlot(ctx, 101, 2, "request-1")
		require.NoError(t, err)
		require.True(t, acquired)

		live := cache.(service.LiveConcurrencyCache)
		acquired, err = live.AcquireLiveLease(ctx, 101, 2, 202, 2, 303, "lease-1", false)
		require.NoError(t, err)
		require.True(t, acquired)
		require.NoError(t, live.ReleaseLiveLease(ctx, 101, 202, 303, "lease-1"))
	})

	t.Run("batch image queue scripts", func(t *testing.T) {
		queue := repository.NewBatchImageQueue(client, &config.Config{})
		const batchID = "imgbatch_cluster_smoke"
		require.NoError(t, queue.Enqueue(ctx, batchID))
		reserved, err := queue.Reserve(ctx, time.Second)
		require.NoError(t, err)
		require.Equal(t, batchID, reserved.BatchID)
		require.NoError(t, queue.Ack(ctx, batchID))

		lock, acquired, err := queue.TryAcquireJobLock(ctx, batchID, time.Minute)
		require.NoError(t, err)
		require.True(t, acquired)
		require.NoError(t, lock.Release(ctx))
	})

	t.Run("scheduler scripts", func(t *testing.T) {
		cache := repository.NewSchedulerCache(client)
		bucket := service.SchedulerBucket{GroupID: 77, Platform: "openai", Mode: "standard"}
		token, err := cache.CaptureBucketWriteToken(ctx, bucket)
		require.NoError(t, err)
		require.NoError(t, cache.SetSnapshot(ctx, bucket, token, []service.Account{{
			ID:       1001,
			Name:     "cluster-smoke",
			Platform: "openai",
		}}))
		accounts, hit, err := cache.GetSnapshot(ctx, bucket)
		require.NoError(t, err)
		require.True(t, hit)
		require.Len(t, accounts, 1)
		require.Equal(t, int64(1001), accounts[0].ID)
	})

	t.Run("cross slot batch reads", func(t *testing.T) {
		latencies := repository.NewProxyLatencyCache(client)
		require.NoError(t, latencies.SetProxyLatency(ctx, 11, &service.ProxyLatencyInfo{}))
		require.NoError(t, latencies.SetProxyLatency(ctx, 12, &service.ProxyLatencyInfo{}))
		values, err := latencies.GetProxyLatencies(ctx, []int64{11, 12})
		require.NoError(t, err)
		require.Len(t, values, 2)

		sessions := repository.NewSessionLimitCache(client, 5)
		allowed, err := sessions.RegisterSession(ctx, 21, "session-21", 2, 5*time.Minute)
		require.NoError(t, err)
		require.True(t, allowed)
		allowed, err = sessions.RegisterSession(ctx, 22, "session-22", 2, 5*time.Minute)
		require.NoError(t, err)
		require.True(t, allowed)
		require.NoError(t, sessions.SetWindowCost(ctx, 21, 1.5))
		require.NoError(t, sessions.SetWindowCost(ctx, 22, 2.5))
		costs, err := sessions.GetWindowCostBatch(ctx, []int64{21, 22})
		require.NoError(t, err)
		require.Equal(t, 1.5, costs[21])
		require.Equal(t, 2.5, costs[22])
	})

	t.Run("pubsub invalidation", func(t *testing.T) {
		const channel = "sub2api:cluster-smoke:invalidate"
		pubsub := client.Subscribe(ctx, channel)
		t.Cleanup(func() { require.NoError(t, pubsub.Close()) })
		_, err := pubsub.Receive(ctx)
		require.NoError(t, err)
		require.NoError(t, client.Publish(ctx, channel, "refresh").Err())
		message, err := pubsub.ReceiveMessage(ctx)
		require.NoError(t, err)
		require.Equal(t, "refresh", message.Payload)
	})
}

func splitAddresses(raw string) []string {
	parts := strings.Split(raw, ",")
	addresses := make([]string, 0, len(parts))
	for _, part := range parts {
		if part = strings.TrimSpace(part); part != "" {
			addresses = append(addresses, part)
		}
	}
	return addresses
}
