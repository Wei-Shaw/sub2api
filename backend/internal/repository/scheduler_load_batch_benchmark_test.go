package repository

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/redis/go-redis/v9"
)

var schedulerLoadBatchBenchmarkSink int

func TestSchedulerLoadBatchRealRedisParity(t *testing.T) {
	rdb := newBenchmarkRedisClient(t)
	defer func() { _ = rdb.Close() }()
	ctx := context.Background()

	for _, size := range []int{1_000, 8_000, 20_000} {
		t.Run(fmt.Sprintf("n_%d", size), func(t *testing.T) {
			baseID := time.Now().UnixNano()
			accounts := make([]service.AccountWithConcurrency, size)
			for start := 0; start < size; start += 1_000 {
				end := min(start+1_000, size)
				pipe := rdb.Pipeline()
				for i := start; i < end; i++ {
					id := baseID + int64(i) + 1
					accounts[i] = service.AccountWithConcurrency{ID: id, MaxConcurrency: i%8 + 1}
					members := []redis.Z{{Score: float64(time.Now().Add(-benchSlotTTL - time.Second).Unix()), Member: "expired"}}
					for slot := 0; slot < i%4; slot++ {
						members = append(members, redis.Z{Score: float64(time.Now().Unix()), Member: fmt.Sprintf("live-%d", slot)})
					}
					pipe.ZAdd(ctx, accountSlotKey(id), members...)
					if i%5 != 0 {
						pipe.Set(ctx, accountWaitKey(id), strconv.Itoa(i%5-1), benchSlotTTL)
					}
				}
				if _, err := pipe.Exec(ctx); err != nil {
					t.Fatalf("seed Redis data: %v", err)
				}
			}
			defer cleanupSchedulerLoadBatchBenchmark(ctx, rdb, accounts)

			pipelineCache := newConcurrencyCache(rdb, benchSlotTTLMinutes, int(benchSlotTTL.Seconds()))
			pipelineStarted := time.Now()
			want, err := pipelineCache.getAccountsLoadBatchPipeline(ctx, accounts)
			if err != nil {
				t.Fatalf("pipeline load: %v", err)
			}
			pipelineDuration := time.Since(pipelineStarted)

			for start := 0; start < size; start += 1_000 {
				end := min(start+1_000, size)
				pipe := rdb.Pipeline()
				for _, account := range accounts[start:end] {
					pipe.ZAdd(ctx, accountSlotKey(account.ID), redis.Z{
						Score:  float64(time.Now().Add(-benchSlotTTL - time.Second).Unix()),
						Member: "expired-script",
					})
				}
				if _, err := pipe.Exec(ctx); err != nil {
					t.Fatalf("reseed expired slots: %v", err)
				}
			}

			scriptCache := newConcurrencyCache(rdb, benchSlotTTLMinutes, int(benchSlotTTL.Seconds()))
			scriptStarted := time.Now()
			got, err := scriptCache.getAccountsLoadBatchScript(ctx, accounts, defaultAccountLoadBatchScriptChunkSize)
			if err != nil {
				t.Fatalf("script load: %v", err)
			}
			scriptDuration := time.Since(scriptStarted)
			if len(got) != len(want) {
				t.Fatalf("result size: script=%d pipeline=%d", len(got), len(want))
			}
			for _, account := range accounts {
				if *got[account.ID] != *want[account.ID] {
					t.Fatalf("account %d mismatch: script=%+v pipeline=%+v", account.ID, got[account.ID], want[account.ID])
				}
			}
			t.Logf("accounts=%d pipeline=%s script256=%s speedup=%.2fx", size, pipelineDuration, scriptDuration, float64(pipelineDuration)/float64(scriptDuration))
		})
	}
}

func BenchmarkSchedulerLoadBatch(b *testing.B) {
	rdb := newBenchmarkRedisClient(b)
	defer func() { _ = rdb.Close() }()
	ctx := context.Background()

	for _, size := range []int{64, 256, 1_000, 3_131, 8_000, 20_000} {
		size := size
		b.Run(fmt.Sprintf("pipeline/n_%d", size), func(b *testing.B) {
			cache, accounts := prepareSchedulerLoadBatchBenchmark(b, rdb, size, false, 0)
			defer cleanupSchedulerLoadBatchBenchmark(ctx, rdb, accounts)
			b.ReportAllocs()
			b.ResetTimer()
			b.ReportMetric(float64(1+3*size), "redis_cmds/op")
			for range b.N {
				result, err := cache.GetAccountsLoadBatch(ctx, accounts)
				if err != nil || len(result) != size {
					b.Fatalf("pipeline load: count=%d err=%v", len(result), err)
				}
				schedulerLoadBatchBenchmarkSink = len(result)
			}
		})

		for _, chunkSize := range []int{128, 256, 500, 1_000} {
			chunkSize := chunkSize
			b.Run(fmt.Sprintf("script%d/n_%d", chunkSize, size), func(b *testing.B) {
				cache, accounts := prepareSchedulerLoadBatchBenchmark(b, rdb, size, true, chunkSize)
				defer cleanupSchedulerLoadBatchBenchmark(ctx, rdb, accounts)
				b.ReportAllocs()
				b.ResetTimer()
				b.ReportMetric(float64(1+(size+chunkSize-1)/chunkSize), "redis_cmds/op")
				for range b.N {
					result, err := cache.GetAccountsLoadBatch(ctx, accounts)
					if err != nil || len(result) != size {
						b.Fatalf("script load: count=%d err=%v", len(result), err)
					}
					schedulerLoadBatchBenchmarkSink = len(result)
				}
			})
		}
	}
}

func BenchmarkSchedulerLoadBatchConcurrent(b *testing.B) {
	rdb := newBenchmarkRedisClient(b)
	defer func() { _ = rdb.Close() }()
	ctx := context.Background()

	for _, size := range []int{1_000, 8_000, 20_000} {
		for _, scriptEnabled := range []bool{false, true} {
			path := "pipeline"
			if scriptEnabled {
				path = "script256"
			}
			for _, concurrency := range []int{1, 4, 16} {
				b.Run(fmt.Sprintf("%s/n_%d/c_%d", path, size, concurrency), func(b *testing.B) {
					cache, accounts := prepareSchedulerLoadBatchBenchmark(b, rdb, size, scriptEnabled, 256)
					defer cleanupSchedulerLoadBatchBenchmark(ctx, rdb, accounts)
					durations := make([]time.Duration, 0, b.N*concurrency)
					b.ReportAllocs()
					b.ResetTimer()
					for range b.N {
						start := make(chan struct{})
						results := make(chan time.Duration, concurrency)
						errs := make(chan error, concurrency)
						var wg sync.WaitGroup
						for range concurrency {
							wg.Add(1)
							go func() {
								defer wg.Done()
								<-start
								startedAt := time.Now()
								result, err := cache.GetAccountsLoadBatch(ctx, accounts)
								results <- time.Since(startedAt)
								if err != nil {
									errs <- err
									return
								}
								if len(result) != size {
									errs <- fmt.Errorf("count=%d want=%d", len(result), size)
								}
							}()
						}
						close(start)
						wg.Wait()
						close(results)
						close(errs)
						for err := range errs {
							b.Fatalf("concurrent load: %v", err)
						}
						for duration := range results {
							durations = append(durations, duration)
						}
					}
					b.StopTimer()
					sort.Slice(durations, func(i, j int) bool { return durations[i] < durations[j] })
					b.ReportMetric(float64(concurrency), "requests/op")
					b.ReportMetric(durationPercentileMillis(durations, 50), "p50_ms")
					b.ReportMetric(durationPercentileMillis(durations, 95), "p95_ms")
					b.ReportMetric(durationPercentileMillis(durations, 99), "p99_ms")
				})
			}
		}
	}
}

func durationPercentileMillis(values []time.Duration, percentile int) float64 {
	if len(values) == 0 {
		return 0
	}
	index := (len(values)*percentile + 99) / 100
	index = min(max(index, 1), len(values)) - 1
	return float64(values[index]) / float64(time.Millisecond)
}

func prepareSchedulerLoadBatchBenchmark(b *testing.B, rdb *redis.Client, size int, scriptEnabled bool, chunkSize int) (*concurrencyCache, []service.AccountWithConcurrency) {
	b.Helper()
	baseID := time.Now().UnixNano()
	accounts := make([]service.AccountWithConcurrency, size)
	pipe := rdb.Pipeline()
	for i := range accounts {
		id := baseID + int64(i) + 1
		accounts[i] = service.AccountWithConcurrency{ID: id, MaxConcurrency: 10}
		pipe.ZAdd(context.Background(), accountSlotKey(id), redis.Z{Score: float64(time.Now().Unix()), Member: "benchmark-live"})
	}
	if _, err := pipe.Exec(context.Background()); err != nil {
		b.Fatalf("seed load keys: %v", err)
	}
	cache := newConcurrencyCache(rdb, benchSlotTTLMinutes, int(benchSlotTTL.Seconds()))
	cache.loadBatchScriptEnabled = scriptEnabled
	if chunkSize > 0 {
		cache.loadBatchScriptChunkSize = chunkSize
	}
	return cache, accounts
}

func cleanupSchedulerLoadBatchBenchmark(ctx context.Context, rdb *redis.Client, accounts []service.AccountWithConcurrency) {
	keys := make([]string, 0, len(accounts)*2)
	for _, account := range accounts {
		id := strconv.FormatInt(account.ID, 10)
		keys = append(keys, accountSlotKeyPrefix+id, accountWaitKeyPrefix+id)
	}
	for start := 0; start < len(keys); start += 1_000 {
		end := min(start+1_000, len(keys))
		_ = rdb.Unlink(ctx, keys[start:end]...).Err()
	}
}
