package admin

import (
	"container/list"
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
	mu         sync.Mutex
	ttl        time.Duration
	items      map[string]snapshotCacheEntry
	sf         singleflight.Group
	order      *list.List
	positions  map[string]*list.Element
	costs      map[string]int
	cost       int
	maxEntries int
	maxCost    int
}

type snapshotCacheLoadResult struct {
	Entry snapshotCacheEntry
	Hit   bool
}

func newSnapshotCache(ttl time.Duration) *snapshotCache {
	if ttl <= 0 {
		ttl = 30 * time.Second
	}
	return &snapshotCache{
		ttl:        ttl,
		items:      make(map[string]snapshotCacheEntry),
		order:      list.New(),
		positions:  make(map[string]*list.Element),
		costs:      make(map[string]int),
		maxEntries: 256,
		maxCost:    16 << 20,
	}
}

func (c *snapshotCache) Get(key string) (snapshotCacheEntry, bool) {
	if c == nil || key == "" {
		return snapshotCacheEntry{}, false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	entry, ok := c.items[key]
	if !ok {
		return snapshotCacheEntry{}, false
	}
	if !time.Now().Before(entry.ExpiresAt) {
		c.remove(key)
		return snapshotCacheEntry{}, false
	}
	return entry, true
}

func (c *snapshotCache) Set(key string, payload any) snapshotCacheEntry {
	if c == nil {
		return snapshotCacheEntry{}
	}
	raw, err := json.Marshal(payload)
	entry := snapshotCacheEntry{
		Payload: payload,
	}
	if err == nil {
		sum := sha256.Sum256(raw)
		entry.ETag = "\"" + hex.EncodeToString(sum[:]) + "\""
	}
	if key == "" {
		return entry
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	entry.ExpiresAt = time.Now().Add(c.ttl)
	c.remove(key)
	cost := len(raw) + len(key)
	if err != nil || cost > c.maxCost || c.maxEntries <= 0 {
		return entry
	}
	// FIFO order also orders expiry because TTL is fixed. Evict only as needed,
	// without scanning the map or retaining an unbounded expiry queue.
	for oldest := c.order.Front(); oldest != nil; oldest = c.order.Front() {
		oldKey, ok := oldest.Value.(string)
		if !ok {
			// Set only inserts strings; discard a corrupt queue node defensively.
			c.order.Remove(oldest)
			continue
		}
		if len(c.items) < c.maxEntries && c.cost+cost <= c.maxCost && time.Now().Before(c.items[oldKey].ExpiresAt) {
			break
		}
		c.remove(oldKey)
	}
	c.items[key] = entry
	c.positions[key] = c.order.PushBack(key)
	c.costs[key] = cost
	c.cost += cost
	return entry
}

// remove requires mu to be held; lookup and expired deletion share that lock.
func (c *snapshotCache) remove(key string) {
	if element, ok := c.positions[key]; ok {
		c.order.Remove(element)
		delete(c.positions, key)
		c.cost -= c.costs[key]
		delete(c.costs, key)
		delete(c.items, key)
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
