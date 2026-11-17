package service

import (
	"context"
	"fmt"
	"math/rand"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

func TestSessionMemoryCacheExpiresWithoutReads(t *testing.T) {
	cache := &sessionMemoryCache{sweep: 5 * time.Millisecond}
	t.Cleanup(cache.Close)
	for i := 0; i < 100; i++ {
		cache.Store(fmt.Sprintf("session-%d", i), &sessionEntry{uuid: "session", expiresAt: time.Now().Add(30 * time.Millisecond)})
	}
	require.Equal(t, 100, cache.ItemCount())
	// ItemCount only observes the map; it does not trigger lazy expiration.
	require.Eventually(t, func() bool { return cache.ItemCount() == 0 }, time.Second, 5*time.Millisecond)
	cache.mu.Lock()
	defer cache.mu.Unlock()
	require.Zero(t, cache.bytes)
	require.Empty(t, cache.prefixes)
	require.Nil(t, cache.timer, "the last expiry must stop scheduling cleanup")
}

func TestOpenAICompatSessionCachesExpireWithoutRevisit(t *testing.T) {
	cfg := &config.Config{}
	cfg.Gateway.OpenAIWS.StickyResponseIDTTLSeconds = 1
	s := &OpenAIGatewayService{cfg: cfg}
	s.openaiCompatAnthropicDigestSessions.sweep = 5 * time.Millisecond
	s.openaiCompatSessionResponses.sweep = 5 * time.Millisecond
	t.Cleanup(s.CloseOpenAIWSPool)
	account := &Account{ID: 1}
	for i := 0; i < 100; i++ {
		key := fmt.Sprintf("session-%d", i)
		s.bindOpenAICompatAnthropicDigestPromptCacheKey(account, 1, "u:"+key, key, "")
		s.bindOpenAICompatSessionResponseID(context.Background(), nil, account, key, "resp-test")
	}
	require.Equal(t, 100, s.openaiCompatAnthropicDigestSessions.ItemCount())
	require.Equal(t, 100, s.openaiCompatSessionResponses.ItemCount())
	require.Eventually(t, func() bool {
		return s.openaiCompatAnthropicDigestSessions.ItemCount() == 0 && s.openaiCompatSessionResponses.ItemCount() == 0
	}, 2*time.Second, 10*time.Millisecond)
}

func TestSessionMemoryCacheBoundsAndLRU(t *testing.T) {
	cache := &sessionMemoryCache{maxEntries: 2, maxBytes: 2048}
	t.Cleanup(cache.Close)
	value := &sessionEntry{uuid: "value", expiresAt: time.Now().Add(time.Hour)}
	cache.Store("first", value)
	cache.Store("second", value)
	_, ok := cache.Load("first")
	require.True(t, ok)
	cache.Store("third", value)
	_, ok = cache.Load("second")
	require.False(t, ok)
	require.Equal(t, 2, cache.ItemCount())
	_, _, ok = cache.longestDigestPrefix("", "second-u:next")
	require.False(t, ok, "eviction must also remove the prefix index")

	// An individually oversized key or value must never enter the cache.
	cache.Store(strings.Repeat("x", 2048), value)
	cache.Store("large-value", &sessionEntry{uuid: strings.Repeat("x", 2048), expiresAt: value.expiresAt})
	require.Equal(t, 2, cache.ItemCount())
	cache.Store("third", &sessionEntry{uuid: strings.Repeat("x", 1400), expiresAt: value.expiresAt})
	require.Equal(t, 1, cache.ItemCount(), "byte pressure must evict independently of the entry cap")
	_, ok = cache.Load("third")
	require.True(t, ok)
	cache.mu.Lock()
	require.LessOrEqual(t, cache.bytes, cache.maxBytes)
	cache.mu.Unlock()
}

func TestSessionMemoryCacheLongestPrefixKeepsRawKeySemantics(t *testing.T) {
	cache := &sessionMemoryCache{}
	t.Cleanup(cache.Close)
	deadline := time.Now().Add(time.Hour)
	// Use the historical raw namespace+chain format, without a rewritten key.
	for _, chain := range []string{"u:a", "u:ab", "u:a-a:b", "u:a-a:b-u:c"} {
		cache.Store("account|key|"+chain, &sessionEntry{uuid: chain, expiresAt: deadline})
	}
	cache.Store("other|key|u:a-a:b-u:c-a:d", &sessionEntry{uuid: "wrong tenant", expiresAt: deadline})
	for _, tc := range []struct{ query, matched string }{
		{"u:a", "u:a"},
		{"u:ab-u:next", "u:ab"},
		{"u:abc-u:next", ""},
		{"u:a-a:b-u:c-a:d", "u:a-a:b-u:c"},
		{"u:a-a:b-u:changed", "u:a-a:b"},
	} {
		value, matched, found := cache.longestDigestPrefix("account|key|", tc.query)
		require.Equal(t, tc.matched != "", found, tc.query)
		require.Equal(t, tc.matched, matched, tc.query)
		if found {
			entry, ok := value.(*sessionEntry)
			require.True(t, ok)
			require.Equal(t, matched, entry.uuid)
		}
	}
	cache.Delete("account|key|u:a-a:b-u:c")
	_, matched, found := cache.longestDigestPrefix("account|key|", "u:a-a:b-u:c-a:d")
	require.True(t, found)
	require.Equal(t, "u:a-a:b", matched)
}

func TestSessionMemoryCacheExpiredLongestPrefixFallsBack(t *testing.T) {
	cache := &sessionMemoryCache{}
	t.Cleanup(cache.Close)
	cache.Store("ns|u:a", &sessionEntry{uuid: "short", expiresAt: time.Now().Add(time.Hour)})
	cache.Store("ns|u:a-a:b", &sessionEntry{uuid: "long", expiresAt: time.Now().Add(time.Hour)})
	cache.mu.Lock()
	entry, ok := cache.entries["ns|u:a-a:b"].Value.(*sessionMemoryEntry)
	if ok {
		entry.expiresAt = time.Now().Add(-time.Second)
	}
	cache.mu.Unlock()
	require.True(t, ok)
	value, matched, found := cache.longestDigestPrefix("ns|", "u:a-a:b-u:c")
	require.True(t, found)
	require.Equal(t, "u:a", matched)
	session, ok := value.(*sessionEntry)
	require.True(t, ok)
	require.Equal(t, "short", session.uuid)
}

func TestSessionMemoryCacheDigestIndexMatchesLegacyLookup(t *testing.T) {
	cache := &sessionMemoryCache{}
	t.Cleanup(cache.Close)
	rng := rand.New(rand.NewSource(42))
	legacy := make(map[string]bool)
	for i := 0; i < 500; i++ {
		ns := fmt.Sprintf("%d|", rng.Intn(3))
		var parts []string
		for j := 0; j <= rng.Intn(12); j++ {
			parts = append(parts, fmt.Sprintf("u:%d", rng.Intn(4)))
		}
		chain := strings.Join(parts, "-")
		cache.Store(ns+chain, &sessionEntry{uuid: chain, expiresAt: time.Now().Add(time.Hour)})
		legacy[ns+chain] = true
		query := chain + "-a:next-u:later"
		if rng.Intn(3) == 0 {
			cache.Delete(ns + chain)
			delete(legacy, ns+chain)
		}
		want := query
		for !legacy[ns+want] {
			end := strings.LastIndexByte(want, '-')
			if end < 0 {
				want = ""
				break
			}
			want = want[:end]
		}
		_, got, found := cache.longestDigestPrefix(ns, query)
		require.Equal(t, want != "", found)
		require.Equal(t, want, got)
	}
}

func TestSessionMemoryCacheConcurrentClose(t *testing.T) {
	cache := &sessionMemoryCache{maxEntries: 32, maxBytes: 8192, sweep: time.Millisecond}
	var wg sync.WaitGroup
	started := make(chan struct{}, 8)
	for g := 0; g < 8; g++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for i := 0; i < 100; i++ {
				key := fmt.Sprintf("ns|u:%d-a:%d", id, i)
				cache.Store(key, &sessionEntry{uuid: key, expiresAt: time.Now().Add(2 * time.Millisecond)})
				if i == 0 {
					started <- struct{}{}
				}
				cache.Load(key)
				cache.longestDigestPrefix("ns|", key[3:]+"-u:next")
				cache.Delete(key)
			}
		}(g)
	}
	for g := 0; g < 8; g++ {
		<-started
	}
	cache.Close()
	wg.Wait()
	cache.Close()
	require.Zero(t, cache.ItemCount())
	cache.mu.Lock()
	defer cache.mu.Unlock()
	require.Nil(t, cache.timer)
	require.Zero(t, cache.bytes)
}

func BenchmarkDigestSessionLookupMiss(b *testing.B) {
	for _, n := range []int{100, 1000, 5000} {
		chain := strings.TrimSuffix(strings.Repeat("u:0123456789abc-", n), "-")
		b.Run(fmt.Sprintf("OpenAICompat/messages_%d", n), func(b *testing.B) {
			s := &OpenAIGatewayService{}
			b.Cleanup(s.CloseOpenAIWSPool)
			account := &Account{ID: 1}
			s.bindOpenAICompatAnthropicDigestPromptCacheKey(account, 1, "u:unrelated", "cache-key", "")
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				s.findOpenAICompatAnthropicDigestPromptCacheKey(account, 1, chain)
			}
		})
		b.Run(fmt.Sprintf("SharedStore/messages_%d", n), func(b *testing.B) {
			s := NewDigestSessionStore()
			b.Cleanup(s.Close)
			s.Save(1, "prefix", "u:unrelated", "uuid", 1, "")
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				s.Find(1, "prefix", chain)
			}
		})
	}
}
