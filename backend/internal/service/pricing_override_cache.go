// Package service — pricing_override_cache.go
//
// PricingOverrideCache is the host-side in-memory cache of pricing overrides
// supplied by plugins implementing the PricingExtension data layer (P2).
// It is a thread-safe map keyed by (group_id, platform, model); the cache
// is rebuilt from the plugin's ListPricingOverrides on each plugin start
// and kept fresh by the plugin's WatchPricingOverrides stream.
//
// This file deliberately does not depend on the SDK proto types or the
// channel cache reader; the pricing extension client is responsible for
// translating between proto messages and PricingOverride values, and the
// resolver is responsible for looking up cached entries during pricing
// resolution. Keeping the cache transport-agnostic means future tests
// (and alternative producers — e.g. a future host admin UI override) can
// populate it without dragging in gRPC machinery.
//
// Failure model: the cache is best-effort. If the producer (plugin) is
// down or the host has restarted before the first List completes, Get
// returns (nil, false) and callers fall back to the existing pricing
// path (channel cache reader → LiteLLM → fallback). The cache MUST NOT
// be the only source of pricing data.

package service

import (
	"strings"
	"sync"
	"time"
)

// PricingOverrideKey uniquely identifies one cache entry.
//
// GroupID == 0 means "applies to all groups" (rare; reserved for global
// overrides). Platform and Model are normalised to lowercase by the
// cache itself so callers may pass either form.
type PricingOverrideKey struct {
	GroupID  int64
	Platform string
	Model    string
}

// PricingOverrideInterval mirrors the proto PricingInterval shape but lives
// in the service package so non-proto producers (tests, future host-admin
// overrides) can populate the cache without importing pluginsdk.
type PricingOverrideInterval struct {
	MinTokens        int64
	MaxTokens        int64 // 0 = unbounded
	InputPrice       float64
	OutputPrice      float64
	CacheWritePrice  float64
	CacheReadPrice   float64
	ImageOutputPrice float64
	PerRequestPrice  float64
}

// PricingOverride is the cache value type. Field semantics mirror the
// proto PricingOverride message; see plugin-sdk/proto/sdk.proto for the
// authoritative documentation.
type PricingOverride struct {
	Key              PricingOverrideKey
	BillingMode      string // "token" / "per_request" / "image"
	InputPrice       float64
	OutputPrice      float64
	CacheWritePrice  float64
	CacheReadPrice   float64
	ImageOutputPrice float64
	PerRequestPrice  float64
	Intervals        []PricingOverrideInterval

	// SourcePlugin records which plugin produced the entry. Used for
	// observability (admin UI listing, logs). Empty for non-plugin
	// producers.
	SourcePlugin string
	// UpdatedAt is the host-local time the cache observed the value. Used
	// for stale-data warnings; not part of the cache key.
	UpdatedAt time.Time
}

// PricingOverrideCache is a goroutine-safe in-memory map of pricing overrides.
//
// Implementation note: a sync.RWMutex around a plain map is intentional. The
// expected steady-state workload is read-heavy (every request goes through
// pricing resolution), with sporadic writes from the plugin Watch stream and
// a wholesale ReplaceAll on plugin (re)start. RWMutex gives concurrent reads
// without contention; sync.Map's tradeoffs (more allocations, no Len) are a
// net loss here.
type PricingOverrideCache struct {
	mu      sync.RWMutex
	entries map[PricingOverrideKey]PricingOverride
	version string
}

// NewPricingOverrideCache constructs an empty cache.
func NewPricingOverrideCache() *PricingOverrideCache {
	return &PricingOverrideCache{
		entries: make(map[PricingOverrideKey]PricingOverride),
	}
}

// normalizePricingKey lower-cases the platform/model halves so callers can
// pass either form without per-call allocation pressure on the hot path.
// GroupID is left untouched.
func normalizePricingKey(key PricingOverrideKey) PricingOverrideKey {
	return PricingOverrideKey{
		GroupID:  key.GroupID,
		Platform: strings.ToLower(strings.TrimSpace(key.Platform)),
		Model:    strings.ToLower(strings.TrimSpace(key.Model)),
	}
}

// Get returns the cached override for (groupID, platform, model). It is
// safe to call from multiple goroutines.
//
// Returned (nil, false) means "no override registered" — callers must
// fall back to their existing pricing path. The pointer return type is
// retained even though the underlying value is by-value so future
// extensions (e.g. nil sentinel for negative cache) do not change the
// signature.
func (c *PricingOverrideCache) Get(groupID int64, platform, model string) (*PricingOverride, bool) {
	if c == nil {
		return nil, false
	}
	key := normalizePricingKey(PricingOverrideKey{
		GroupID:  groupID,
		Platform: platform,
		Model:    model,
	})
	c.mu.RLock()
	defer c.mu.RUnlock()
	v, ok := c.entries[key]
	if !ok {
		return nil, false
	}
	// Return a copy so callers cannot mutate the cached slice in-place.
	cp := v
	if len(v.Intervals) > 0 {
		cp.Intervals = append([]PricingOverrideInterval(nil), v.Intervals...)
	}
	return &cp, true
}

// Set inserts or replaces an override entry. The override is keyed by
// override.Key; Platform/Model are normalised before insertion.
func (c *PricingOverrideCache) Set(override PricingOverride) {
	if c == nil {
		return
	}
	override.Key = normalizePricingKey(override.Key)
	if override.UpdatedAt.IsZero() {
		override.UpdatedAt = time.Now()
	}
	if len(override.Intervals) > 0 {
		// Defensive copy so subsequent caller mutation does not race readers.
		override.Intervals = append([]PricingOverrideInterval(nil), override.Intervals...)
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries[override.Key] = override
}

// Delete removes the override matching key. No-op when the key is absent.
func (c *PricingOverrideCache) Delete(key PricingOverrideKey) {
	if c == nil {
		return
	}
	key = normalizePricingKey(key)
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.entries, key)
}

// ReplaceAll atomically swaps the cache contents with overrides. Used
// during full re-sync (plugin start, Watch reconnect with stale version).
// The version string is recorded so callers can pass it as the resume
// point on the next Watch call.
func (c *PricingOverrideCache) ReplaceAll(overrides []PricingOverride, version string) {
	if c == nil {
		return
	}
	next := make(map[PricingOverrideKey]PricingOverride, len(overrides))
	now := time.Now()
	for _, o := range overrides {
		o.Key = normalizePricingKey(o.Key)
		if o.UpdatedAt.IsZero() {
			o.UpdatedAt = now
		}
		if len(o.Intervals) > 0 {
			o.Intervals = append([]PricingOverrideInterval(nil), o.Intervals...)
		}
		next[o.Key] = o
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries = next
	c.version = version
}

// Version returns the version string recorded by the most recent
// ReplaceAll. Empty string means the cache has never been bulk-loaded.
func (c *PricingOverrideCache) Version() string {
	if c == nil {
		return ""
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.version
}

// Len returns the number of cached entries. Primarily for tests and
// admin diagnostics; not on any hot path.
func (c *PricingOverrideCache) Len() int {
	if c == nil {
		return 0
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	return len(c.entries)
}
