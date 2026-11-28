package repository

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/redis/go-redis/v9"
)

// Inject delay once per client exchange, not per command inside a pipeline.
// This isolates RTT sensitivity from Redis execution cost on the local server.
type queueStatsLatencyHook struct{ delay time.Duration }

func (h queueStatsLatencyHook) DialHook(next redis.DialHook) redis.DialHook { return next }
func (h queueStatsLatencyHook) ProcessHook(next redis.ProcessHook) redis.ProcessHook {
	return func(ctx context.Context, cmd redis.Cmder) error {
		time.Sleep(h.delay)
		return next(ctx, cmd)
	}
}
func (h queueStatsLatencyHook) ProcessPipelineHook(next redis.ProcessPipelineHook) redis.ProcessPipelineHook {
	return func(ctx context.Context, cmds []redis.Cmder) error {
		time.Sleep(h.delay)
		return next(ctx, cmds)
	}
}

func BenchmarkAPIKeyQueueStats(b *testing.B) {
	for _, delay := range []time.Duration{0, time.Millisecond, 5 * time.Millisecond} {
		for _, size := range []int{1, 20, 100} {
			b.Run(fmt.Sprintf("delay=%s/keys=%d", delay, size), func(b *testing.B) {
				client := newBenchmarkRedisClient(b)
				b.Cleanup(func() {
					if err := client.Close(); err != nil {
						b.Errorf("close benchmark Redis client: %v", err)
					}
				})
				client.AddHook(queueStatsLatencyHook{delay})
				cache := NewConcurrencyCache(client, 15, 60)
				svc := service.NewConcurrencyService(cache)
				ids := make([]int64, size)
				for i := range ids {
					ids[i] = int64(i + 1)
				}
				ctx := context.Background()
				if _, err := svc.GetAPIKeyQueueStatsBatch(ctx, ids); err != nil {
					b.Fatal(err)
				}
				b.ResetTimer()
				for i := 0; i < b.N; i++ {
					if _, err := svc.GetAPIKeyQueueStatsBatch(ctx, ids); err != nil {
						b.Fatal(err)
					}
				}
			})
		}
	}
}
