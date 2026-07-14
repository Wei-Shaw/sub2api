package admin

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strings"
	"sync"
	"time"

	"golang.org/x/sync/singleflight"
)

type snapshotCacheEntry struct {
	ETag      string
	Payload   any
	ExpiresAt time.Time
}

type snapshotCache struct {
	mu       sync.RWMutex
	ttl      time.Duration
	maxItems int
	items    map[string]snapshotCacheEntry
	sf       singleflight.Group
}

type snapshotCacheLoadResult struct {
	Entry snapshotCacheEntry
	Hit   bool
}

func newSnapshotCache(ttl time.Duration) *snapshotCache {
	return newSnapshotCacheWithLimit(ttl, 512)
}

func newSnapshotCacheWithLimit(ttl time.Duration, maxItems int) *snapshotCache {
	if ttl <= 0 {
		ttl = 30 * time.Second
	}
	if maxItems <= 0 {
		maxItems = 512
	}
	return &snapshotCache{
		ttl:      ttl,
		maxItems: maxItems,
		items:    make(map[string]snapshotCacheEntry),
	}
}

func (c *snapshotCache) Get(key string) (snapshotCacheEntry, bool) {
	if c == nil || key == "" {
		return snapshotCacheEntry{}, false
	}
	now := time.Now()

	c.mu.RLock()
	entry, ok := c.items[key]
	c.mu.RUnlock()
	if !ok {
		return snapshotCacheEntry{}, false
	}
	if now.After(entry.ExpiresAt) {
		c.mu.Lock()
		delete(c.items, key)
		c.mu.Unlock()
		return snapshotCacheEntry{}, false
	}
	return entry, true
}

func (c *snapshotCache) Set(key string, payload any) snapshotCacheEntry {
	if c == nil {
		return snapshotCacheEntry{}
	}
	entry := snapshotCacheEntry{
		ETag:      buildETagFromAny(payload),
		Payload:   payload,
		ExpiresAt: time.Now().Add(c.ttl),
	}
	if key == "" {
		return entry
	}
	c.mu.Lock()
	c.pruneLocked(time.Now(), key)
	c.items[key] = entry
	c.mu.Unlock()
	return entry
}

func (c *snapshotCache) pruneLocked(now time.Time, incomingKey string) {
	for key, item := range c.items {
		if now.After(item.ExpiresAt) {
			delete(c.items, key)
		}
	}
	if _, exists := c.items[incomingKey]; exists || len(c.items) < c.maxItems {
		return
	}

	var oldestKey string
	var oldestExpiry time.Time
	for key, item := range c.items {
		if oldestKey == "" || item.ExpiresAt.Before(oldestExpiry) {
			oldestKey = key
			oldestExpiry = item.ExpiresAt
		}
	}
	if oldestKey != "" {
		delete(c.items, oldestKey)
	}
}

func (c *snapshotCache) GetOrLoad(key string, load func() (any, error)) (snapshotCacheEntry, bool, error) {
	if load == nil {
		return snapshotCacheEntry{}, false, nil
	}
	if entry, ok := c.Get(key); ok {
		return entry, true, nil
	}
	if c == nil || key == "" {
		payload, err := load()
		if err != nil {
			return snapshotCacheEntry{}, false, err
		}
		return c.Set(key, payload), false, nil
	}

	value, err, _ := c.sf.Do(key, func() (any, error) {
		if entry, ok := c.Get(key); ok {
			return snapshotCacheLoadResult{Entry: entry, Hit: true}, nil
		}
		payload, err := load()
		if err != nil {
			return nil, err
		}
		return snapshotCacheLoadResult{Entry: c.Set(key, payload), Hit: false}, nil
	})
	if err != nil {
		return snapshotCacheEntry{}, false, err
	}
	result, ok := value.(snapshotCacheLoadResult)
	if !ok {
		return snapshotCacheEntry{}, false, nil
	}
	return result.Entry, result.Hit, nil
}

func buildETagFromAny(payload any) string {
	raw, err := json.Marshal(payload)
	if err != nil {
		return ""
	}
	sum := sha256.Sum256(raw)
	return "\"" + hex.EncodeToString(sum[:]) + "\""
}

func parseBoolQueryWithDefault(raw string, def bool) bool {
	value := strings.TrimSpace(strings.ToLower(raw))
	if value == "" {
		return def
	}
	switch value {
	case "1", "true", "yes", "on":
		return true
	case "0", "false", "no", "off":
		return false
	default:
		return def
	}
}
