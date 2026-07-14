package service

import (
	"testing"
	"time"
)

func TestBoundedAccountCacheCapsEntries(t *testing.T) {
	cache := newBoundedAccountCache(32)
	for id := int64(1); id <= 1000; id++ {
		cache.Store(id, id)
	}
	if got := cache.Len(); got != 32 {
		t.Fatalf("expected 32 retained entries, got %d", got)
	}
}

func TestUsageCacheCleanupExpiredAndDeleteAccount(t *testing.T) {
	cache := NewUsageCache()
	now := time.Now()
	cache.apiCache.Store(1, &apiUsageCache{timestamp: now.Add(-apiCacheTTL)})
	cache.windowStatsCache.Store(1, &windowStatsCache{timestamp: now})
	cache.antigravityCache.Store(2, &antigravityUsageCache{
		usageInfo: &UsageInfo{},
		timestamp: now.Add(-apiCacheTTL),
	})
	cache.openAIProbeCache.Store(2, now.Add(-openAIProbeCacheTTL))
	cache.grokProbeCache.Store(2, now)

	cache.cleanupExpired(now)
	if _, ok := cache.apiCache.Load(1); ok {
		t.Fatal("expired API cache entry was retained")
	}
	if _, ok := cache.antigravityCache.Load(2); ok {
		t.Fatal("expired Antigravity cache entry was retained")
	}
	if _, ok := cache.openAIProbeCache.Load(2); ok {
		t.Fatal("expired OpenAI probe entry was retained")
	}

	cache.DeleteAccount(1)
	if _, ok := cache.windowStatsCache.Load(1); ok {
		t.Fatal("account deletion retained window stats")
	}
	cache.DeleteAccount(2)
	if _, ok := cache.grokProbeCache.Load(2); ok {
		t.Fatal("account deletion retained Grok probe state")
	}
}
