package basispoints

import (
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestReplayCacheIdleExpiryAndLRURefresh(t *testing.T) {
	now := time.Date(2026, time.September, 26, 12, 0, 0, 0, time.UTC)
	cache := &ReplayCache{now: func() time.Time { return now }}
	cache.put("scope", "active", object{"type": "function_call", "call_id": "active", "name": "demo", "arguments": object{}})
	cache.put("scope", "idle", object{"type": "function_call", "call_id": "idle", "name": "demo", "arguments": object{}})

	now = now.Add(replayCacheIdleTTL - time.Minute)
	if cache.get("scope", "active") == nil {
		t.Fatal("active replay entry expired before idle TTL")
	}

	now = now.Add(2 * time.Minute)
	if cache.get("scope", "idle") != nil {
		t.Fatal("idle replay entry survived its TTL")
	}
	if cache.get("scope", "active") == nil {
		t.Fatal("cache hit did not refresh idle lifetime")
	}

	now = now.Add(replayCacheIdleTTL)
	if cache.get("scope", "active") != nil {
		t.Fatal("replay entry survived exact idle expiry")
	}
	if len(cache.entries) != 0 || cache.bytes != 0 || cache.order.Len() != 0 {
		t.Fatalf("expired replay state retained: entries=%d bytes=%d order=%d", len(cache.entries), cache.bytes, cache.order.Len())
	}
}

func TestReplayCachePutReclaimsIdleEntries(t *testing.T) {
	now := time.Now()
	cache := &ReplayCache{now: func() time.Time { return now }}
	cache.put("scope", "old", object{"id": "old"})
	now = now.Add(replayCacheIdleTTL)
	cache.put("scope", "new", object{"id": "new"})
	require.Len(t, cache.entries, 1)
	require.Nil(t, cache.get("scope", "old"))
	require.Equal(t, "new", cache.get("scope", "new")["id"])
}

func TestReplayCacheMismatchedCallDoesNotRefreshExpiry(t *testing.T) {
	now := time.Now()
	cache := &ReplayCache{now: func() time.Time { return now }}
	call := object{"type": "custom_tool_call", "call_id": "call", "name": "exec", "input": "original"}
	cache.put("scope", "call", object{"id": "native"}, call)
	now = now.Add(replayCacheIdleTTL - time.Second)
	call["input"] = "different"
	require.Nil(t, cache.getForCall("scope", "call", call))
	now = now.Add(time.Second)
	require.Nil(t, cache.get("scope", "call"))
	require.Zero(t, cache.bytes)
}

func TestReplayCacheReplacementInvalidatesStaleIdentity(t *testing.T) {
	for _, replacement := range []object{
		{"arguments": strings.Repeat("x", replayCacheEntryBytes)},
		{"invalid": make(chan int)},
	} {
		cache := new(ReplayCache)
		cache.put("scope", "call", object{"id": "old-native"})
		cache.put("scope", "call", replacement)
		require.Nil(t, cache.get("scope", "call"))
		require.Empty(t, cache.entries)
		require.Zero(t, cache.bytes)
		require.Zero(t, cache.order.Len())
	}
}

func TestReplayCacheBudgetCountsKeysAndReplacements(t *testing.T) {
	cache := new(ReplayCache)
	// Small payloads with large scope keys must still consume the byte budget.
	scope := strings.Repeat("s", 1<<20)
	for i := range 20 {
		cache.put(scope, fmt.Sprint(i), object{"id": i})
	}
	require.LessOrEqual(t, cache.bytes, replayCacheMaxBytes)
	require.Less(t, len(cache.entries), 20)
	require.Nil(t, cache.get(scope, "0"))
	before := cache.bytes
	cache.put(scope, "19", object{"id": 19})
	require.Equal(t, before, cache.bytes, "replacement must release its old weight")

	cache = new(ReplayCache)
	value := object{"arguments": strings.Repeat("x", 900<<10)}
	for i := range 20 {
		cache.put("scope", fmt.Sprint(i), value)
	}
	require.LessOrEqual(t, cache.bytes, replayCacheMaxBytes)
	require.Nil(t, cache.get("scope", "0"))
	require.NotNil(t, cache.get("scope", "19"))
}

func TestReplayCacheConcurrentSnapshotsPreserveNumbers(t *testing.T) {
	cache := new(ReplayCache)
	var wg sync.WaitGroup
	for worker := range 16 {
		wg.Go(func() {
			scope := fmt.Sprint(worker)
			for iteration := range 32 {
				number := json.Number("9007199254740993")
				nested := object{"sequence": number}
				cache.put(scope, "call", object{"metadata": nested})
				nested["sequence"] = "caller mutation"
				got := cache.get(scope, "call")
				if got == nil {
					t.Errorf("worker %d lost entry at iteration %d", worker, iteration)
					return
				}
				metadata, ok := got["metadata"].(object)
				if !ok {
					t.Errorf("worker %d received invalid metadata", worker)
					return
				}
				if metadata["sequence"] != number {
					t.Errorf("worker %d lost numeric precision or snapshot isolation", worker)
				}
				metadata["sequence"] = "returned mutation"
				next, ok := cache.get(scope, "call")["metadata"].(object)
				if !ok || next["sequence"] != number {
					t.Errorf("worker %d mutated a cached snapshot", worker)
				}
			}
		})
	}
	wg.Wait()
}
