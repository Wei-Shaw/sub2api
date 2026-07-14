package provider

import (
	"strconv"
	"testing"
	"time"
)

func TestAirwallexTokenCacheBoundsEntries(t *testing.T) {
	cache := newAirwallexTokenCache()
	for i := 0; i < airwallexTokenCacheMaxEntries+100; i++ {
		_, release := cache.acquire(strconv.Itoa(i))
		release()
	}

	cache.mu.Lock()
	count := len(cache.entries)
	cache.mu.Unlock()
	if count != airwallexTokenCacheMaxEntries {
		t.Fatalf("expected %d token states, got %d", airwallexTokenCacheMaxEntries, count)
	}
}

func TestAirwallexTokenCachePrunesExpiredState(t *testing.T) {
	now := time.Unix(1730000000, 0)
	cache := newAirwallexTokenCache()
	cache.now = func() time.Time { return now }
	state, release := cache.acquire("expired")
	state.expiresAt = now.Add(time.Minute)
	release()

	now = now.Add(time.Minute)
	_, release = cache.acquire("current")
	release()
	cache.mu.Lock()
	_, retained := cache.entries["expired"]
	cache.mu.Unlock()
	if retained {
		t.Fatal("expired token state was retained")
	}
}
