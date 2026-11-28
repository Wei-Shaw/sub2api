package antigravity

import (
	"strings"
	"sync"
	"time"
)

const (
	// thoughtSignatureCacheTTL bounds how long a tool-call signature can be replayed.
	thoughtSignatureCacheTTL = 2 * time.Hour
	// thoughtSignatureCacheMaxEntries bounds the cache size.
	thoughtSignatureCacheMaxEntries = 8192
	maxCachedThoughtSignatureBytes  = 64 << 10
)

// ThoughtSignatureScope prevents a client-supplied tool call ID from replaying
// a signature from a different API key or model.
type ThoughtSignatureScope struct {
	APIKeyID int64
	Model    string
}

type thoughtSignatureKey struct {
	scope  ThoughtSignatureScope
	toolID string
}

// ThoughtSignatureCache caches upstream Gemini thought signatures for tool calls.
//
// Gemini 3 requires functionCall parts to carry the thoughtSignature produced by the
// previous turn. OpenAI-compatible clients cannot carry that signature in their
// protocol, so the proxy remembers it server-side and replays it on the next turn.
// This mirrors Antigravity-Manager's signature cache/replay behavior.
type ThoughtSignatureCache struct {
	mu      sync.RWMutex
	entries map[thoughtSignatureKey]thoughtSignatureEntry
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
		entries: make(map[thoughtSignatureKey]thoughtSignatureEntry),
		ttl:     ttl,
		max:     max,
	}
}

var defaultThoughtSignatureCache = NewThoughtSignatureCache(thoughtSignatureCacheTTL, thoughtSignatureCacheMaxEntries)

// CacheToolThoughtSignature remembers the thoughtSignature returned with a tool call.
func CacheToolThoughtSignature(scope ThoughtSignatureScope, toolID, signature string) {
	defaultThoughtSignatureCache.Set(scope, toolID, signature)
}

// LookupToolThoughtSignature returns the cached thoughtSignature for a tool call ID.
func LookupToolThoughtSignature(scope ThoughtSignatureScope, toolID string) string {
	return defaultThoughtSignatureCache.Get(scope, toolID)
}

// Set stores a non-empty, non-sentinel signature for the given tool call ID.
func (c *ThoughtSignatureCache) Set(scope ThoughtSignatureScope, toolID, signature string) {
	if c == nil || scope.APIKeyID <= 0 || scope.Model == "" {
		return
	}
	key := thoughtSignatureKey{scope: scope, toolID: strings.TrimSpace(toolID)}
	sig := strings.TrimSpace(signature)
	if key.toolID == "" || len(key.toolID) > 1024 || sig == "" || len(sig) > maxCachedThoughtSignatureBytes || sig == DummyThoughtSignature {
		return
	}
	now := time.Now()

	c.mu.Lock()
	defer c.mu.Unlock()
	if _, exists := c.entries[key]; !exists && len(c.entries) >= c.max {
		c.evictLocked(now)
	}
	c.entries[key] = thoughtSignatureEntry{signature: sig, storedAt: now}
}

// Get returns the cached signature, or "" when absent/expired.
func (c *ThoughtSignatureCache) Get(scope ThoughtSignatureScope, toolID string) string {
	if c == nil || scope.APIKeyID <= 0 || scope.Model == "" {
		return ""
	}
	key := thoughtSignatureKey{scope: scope, toolID: strings.TrimSpace(toolID)}
	if key.toolID == "" {
		return ""
	}

	c.mu.RLock()
	entry, ok := c.entries[key]
	c.mu.RUnlock()
	if !ok || time.Since(entry.storedAt) > c.ttl {
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
