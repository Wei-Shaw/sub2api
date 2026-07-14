package repository

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

// The read-only production fixture's largest group contains 3,131 accounts.
const schedulerBenchmarkLargeGroupSize = 3131

func BenchmarkSchedulerCacheLargeGroupRead(b *testing.B) {
	ctx := context.Background()
	cache, cleanup := newSchedulerBenchmarkCache(b)
	defer cleanup()
	bucket := service.SchedulerBucket{GroupID: 1562, Platform: service.PlatformOpenAI, Mode: service.SchedulerModeSingle}
	accounts := schedulerBenchmarkAccounts(schedulerBenchmarkLargeGroupSize)
	if err := cache.SetSnapshot(ctx, bucket, accounts); err != nil {
		b.Fatal(err)
	}
	if err := cache.SetCandidateIndex(ctx, bucket, accounts); err != nil {
		b.Fatal(err)
	}
	b.ReportMetric(schedulerBenchmarkLargeGroupSize, "group_accounts")

	b.Run("legacy_full_snapshot", func(b *testing.B) {
		b.ReportAllocs()
		for range b.N {
			result, hit, err := cache.GetSnapshot(ctx, bucket)
			if err != nil || !hit || len(result) != schedulerBenchmarkLargeGroupSize {
				b.Fatalf("legacy read: count=%d hit=%v err=%v", len(result), hit, err)
			}
		}
	})

	b.Run("v2_top_64", func(b *testing.B) {
		b.ReportAllocs()
		for range b.N {
			page, hit, err := cache.GetCandidatePage(ctx, bucket, 0, service.DefaultSchedulerCandidateFetchLimit)
			if err != nil || !hit || len(page.Accounts) != service.DefaultSchedulerCandidateFetchLimit {
				b.Fatalf("v2 read: count=%d hit=%v err=%v", len(page.Accounts), hit, err)
			}
		}
	})
}

func BenchmarkSchedulerCacheLargeGroupUpdate(b *testing.B) {
	ctx := context.Background()
	accounts := schedulerBenchmarkAccounts(schedulerBenchmarkLargeGroupSize)
	b.ReportMetric(schedulerBenchmarkLargeGroupSize, "group_accounts")

	b.Run("legacy_rebuild_group", func(b *testing.B) {
		cache, cleanup := newSchedulerBenchmarkCache(b)
		defer cleanup()
		bucket := service.SchedulerBucket{GroupID: 1562, Platform: service.PlatformOpenAI, Mode: service.SchedulerModeSingle}
		b.ResetTimer()
		for i := range b.N {
			accounts[0].Priority = i % 100
			if err := cache.SetSnapshot(ctx, bucket, accounts); err != nil {
				b.Fatal(err)
			}
		}
	})

	b.Run("v2_replace_one_account", func(b *testing.B) {
		cache, cleanup := newSchedulerBenchmarkCache(b)
		defer cleanup()
		bucket := service.SchedulerBucket{GroupID: 1562, Platform: service.PlatformOpenAI, Mode: service.SchedulerModeSingle}
		if err := cache.SetCandidateIndex(ctx, bucket, accounts); err != nil {
			b.Fatal(err)
		}
		b.ResetTimer()
		for i := range b.N {
			account := accounts[0]
			account.Priority = i % 100
			if err := cache.ReplaceAccountCandidates(ctx, &account, []service.SchedulerBucket{bucket}); err != nil {
				b.Fatal(err)
			}
		}
	})
}

func newSchedulerBenchmarkCache(b *testing.B) (*schedulerCache, func()) {
	b.Helper()
	mr, err := miniredis.Run()
	if err != nil {
		b.Fatal(err)
	}
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	cache := newSchedulerCacheWithChunkSizes(client, 128, 256).(*schedulerCache)
	return cache, func() {
		_ = client.Close()
		mr.Close()
	}
}

func schedulerBenchmarkAccounts(count int) []service.Account {
	accounts := make([]service.Account, 0, count)
	now := time.Now().UTC().Truncate(time.Second)
	for i := 0; i < count; i++ {
		lastUsed := now.Add(-time.Duration(i) * time.Second)
		accounts = append(accounts, service.Account{
			ID:          int64(i + 1),
			Name:        fmt.Sprintf("benchmark-%d", i+1),
			Platform:    service.PlatformOpenAI,
			Type:        service.AccountTypeOAuth,
			Concurrency: 3,
			Priority:    50,
			Status:      service.StatusActive,
			Schedulable: true,
			LastUsedAt:  &lastUsed,
		})
	}
	return accounts
}
