package antigravity

import (
	"strings"
	"sync"
	"time"
)

const (
	// thoughtSignatureCacheTTL bounds how long a tool-call signature can be replayed.
	thoughtSignatureCacheTTL = 2 * time.Hour
	// thoughtSignatureCacheMaxEntries bounds the cache size (FIFO eviction beyond it).
	thoughtSignatureCacheMaxEntries = 8192
)

// ThoughtSignatureCache caches upstream Gemini thought signatures by tool call ID.
//
// Gemini 3 requires functionCall parts to carry the thoughtSignature produced by the
// previous turn. OpenAI-compatible clients cannot carry that signature in their
// protocol, so the proxy remembers it server-side and replays it on the next turn.
// This mirrors Antigravity-Manager's signature cache/replay behavior.
type ThoughtSignatureCache struct {
	mu      sync.RWMutex
	entries map[string]thoughtSignatureEntry
	ttl     time.Duration
	max     int
}

type thoughtSignatureEntry struct {
	signature string
	storedAt  time.Time
}

// NewThoughtSignatureCache creates a signature cache with the given TTL and capacity.
func NewThoughtSignatureCache(ttl time.Duration, max int) *ThoughtSignatureCache {
	if ttl <= 0 {
		ttl = thoughtSignatureCacheTTL
	}
	if max <= 0 {
		max = thoughtSignatureCacheMaxEntries
	}
	return &ThoughtSignatureCache{
		entries: make(map[string]thoughtSignatureEntry),
		ttl:     ttl,
		max:     max,
	}
}

var defaultThoughtSignatureCache = NewThoughtSignatureCache(thoughtSignatureCacheTTL, thoughtSignatureCacheMaxEntries)

// CacheToolThoughtSignature remembers the thoughtSignature returned with a tool call.
func CacheToolThoughtSignature(toolID, signature string) {
	defaultThoughtSignatureCache.Set(toolID, signature)
}

// LookupToolThoughtSignature returns the cached thoughtSignature for a tool call ID.
func LookupToolThoughtSignature(toolID string) string {
	return defaultThoughtSignatureCache.Get(toolID)
}

// Set stores a non-empty, non-sentinel signature for the given tool call ID.
func (c *ThoughtSignatureCache) Set(toolID, signature string) {
	if c == nil {
		return
	}
	key := strings.TrimSpace(toolID)
	sig := strings.TrimSpace(signature)
	if key == "" || sig == "" || sig == DummyThoughtSignature {
		return
	}
	now := time.Now()

	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.entries) >= c.max {
		c.evictLocked(now)
	}
	c.entries[key] = thoughtSignatureEntry{signature: sig, storedAt: now}
}

// Get returns the cached signature, or "" when absent/expired.
func (c *ThoughtSignatureCache) Get(toolID string) string {
	if c == nil {
		return ""
	}
	key := strings.TrimSpace(toolID)
	if key == "" {
		return ""
	}

	c.mu.RLock()
	entry, ok := c.entries[key]
	c.mu.RUnlock()
	if !ok {
		return ""
	}
	if c.ttl > 0 && time.Since(entry.storedAt) > c.ttl {
		c.mu.Lock()
		delete(c.entries, key)
		c.mu.Unlock()
		return ""
	}
	return entry.signature
}

// evictLocked removes expired entries; if the cache is still full it drops arbitrary
// entries until there is room. Callers must hold c.mu.
func (c *ThoughtSignatureCache) evictLocked(now time.Time) {
	for key, entry := range c.entries {
		if c.ttl > 0 && now.Sub(entry.storedAt) > c.ttl {
			delete(c.entries, key)
		}
	}
	for key := range c.entries {
		if len(c.entries) < c.max {
			return
		}
		delete(c.entries, key)
	}
}
