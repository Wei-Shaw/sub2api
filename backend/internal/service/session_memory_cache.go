package service

import (
	"container/list"
	"hash/maphash"
	"strings"
	"sync"
	"time"
)

const (
	sessionMemoryCacheMaxEntries = 16_384
	sessionMemoryCacheMaxBytes   = 32 << 20
	sessionMemoryCacheSweep      = time.Minute
	// Includes the map/list/index entries and interface/string headers. The
	// entry cap separately bounds bookkeeping even for very small values.
	sessionMemoryCacheEntryCost = 512
)

type sessionMemoryValue interface {
	sessionExpiresAt() time.Time
	sessionCost() int
	cloneSessionValue() sessionMemoryValue
}

type sessionMemoryEntry struct {
	key       string
	hash      uint64
	value     sessionMemoryValue
	expiresAt time.Time
	cost      int
}

// sessionMemoryCache keeps the original string keys and bounds both their
// count and retained bytes. Cleanup stops when a sweep finds the cache empty;
// there is no goroutine waiting for the lifetime of a service or test instance.
// The zero value is ready for use.
type sessionMemoryCache struct {
	mu         sync.Mutex
	entries    map[string]*list.Element
	prefixes   map[uint64][]*list.Element
	lru        list.List
	seed       maphash.Seed
	bytes      int
	maxEntries int
	maxBytes   int
	sweep      time.Duration
	timer      *time.Timer
	closed     bool
}

func (c *sessionMemoryCache) initLocked() {
	if c.entries != nil {
		return
	}
	c.entries = make(map[string]*list.Element)
	c.prefixes = make(map[uint64][]*list.Element)
	c.seed = maphash.MakeSeed()
	if c.maxEntries <= 0 {
		c.maxEntries = sessionMemoryCacheMaxEntries
	}
	if c.maxBytes <= 0 {
		c.maxBytes = sessionMemoryCacheMaxBytes
	}
	if c.sweep <= 0 {
		c.sweep = sessionMemoryCacheSweep
	}
}

func (c *sessionMemoryCache) Store(key string, value sessionMemoryValue) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return
	}
	c.initLocked()
	if current := c.entries[key]; current != nil {
		c.removeLocked(current)
	}
	cost := sessionMemoryCacheEntryCost + len(key) + value.sessionCost()
	expiresAt := value.sessionExpiresAt()
	if cost > c.maxBytes || !expiresAt.After(time.Now()) {
		return
	}
	for len(c.entries) >= c.maxEntries || c.bytes+cost > c.maxBytes {
		c.removeLocked(c.lru.Back())
	}
	// Never let a short slice of a request or response pin its backing body.
	key = strings.Clone(key)
	entry := &sessionMemoryEntry{
		key: key, hash: maphash.String(c.seed, key), value: value.cloneSessionValue(),
		expiresAt: expiresAt, cost: cost,
	}
	element := c.lru.PushFront(entry)
	c.entries[key] = element
	c.prefixes[entry.hash] = append(c.prefixes[entry.hash], element)
	c.bytes += cost
	if c.timer == nil {
		c.timer = time.AfterFunc(c.sweep, c.sweepExpired)
	}
}

func (c *sessionMemoryCache) Load(key string) (sessionMemoryValue, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	element := c.entries[key]
	if element == nil {
		return nil, false
	}
	entry, _ := element.Value.(*sessionMemoryEntry)
	if !entry.expiresAt.After(time.Now()) {
		c.removeLocked(element)
		return nil, false
	}
	c.lru.MoveToFront(element)
	return entry.value, true
}

func (c *sessionMemoryCache) Delete(key string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if element := c.entries[key]; element != nil {
		c.removeLocked(element)
	}
}

func (c *sessionMemoryCache) removeLocked(element *list.Element) {
	entry, _ := element.Value.(*sessionMemoryEntry)
	delete(c.entries, entry.key)
	indexed := c.prefixes[entry.hash]
	for i, candidate := range indexed {
		if candidate == element {
			indexed[i] = indexed[len(indexed)-1]
			indexed[len(indexed)-1] = nil
			indexed = indexed[:len(indexed)-1]
			break
		}
	}
	if len(indexed) == 0 {
		delete(c.prefixes, entry.hash)
	} else {
		c.prefixes[entry.hash] = indexed
	}
	c.bytes -= entry.cost
	c.lru.Remove(element)
}

func (c *sessionMemoryCache) sweepExpired() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.timer = nil
	if c.closed {
		return
	}
	now := time.Now()
	for element := c.lru.Back(); element != nil; {
		previous := element.Prev()
		entry, _ := element.Value.(*sessionMemoryEntry)
		if !entry.expiresAt.After(now) {
			c.removeLocked(element)
		}
		element = previous
	}
	if len(c.entries) > 0 {
		c.timer = time.AfterFunc(c.sweep, c.sweepExpired)
	}
}

func (c *sessionMemoryCache) Close() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.closed = true
	if c.timer != nil {
		c.timer.Stop()
		c.timer = nil
	}
	c.entries = nil
	c.prefixes = nil
	c.lru.Init()
	c.bytes = 0
}

func (c *sessionMemoryCache) ItemCount() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.entries)
}

// longestDigestPrefix uses incrementally hashed prefixes to avoid allocating
// and hashing ns+chain[:end] for every message. Keys remain unchanged, and a
// hash hit is checked against the full original key, including its namespace.
// Hashing and temporary storage are O(len(chain)); map lookups are O(messages).
func (c *sessionMemoryCache) longestDigestPrefix(ns, chain string) (sessionMemoryValue, string, bool) {
	c.mu.Lock()
	if len(c.entries) == 0 {
		c.mu.Unlock()
		return nil, "", false
	}
	seed := c.seed
	c.mu.Unlock()

	type prefix struct {
		hash uint64
		end  int
	}
	prefixes := make([]prefix, 0, strings.Count(chain, "-")+1)
	var hash maphash.Hash
	hash.SetSeed(seed)
	_, _ = hash.WriteString(ns)
	start := 0
	for i := 0; i < len(chain); i++ {
		if chain[i] != '-' {
			continue
		}
		_, _ = hash.WriteString(chain[start:i])
		prefixes = append(prefixes, prefix{hash: hash.Sum64(), end: i})
		// Include the separator only in subsequent, longer prefixes.
		_, _ = hash.WriteString("-")
		start = i + 1
	}
	_, _ = hash.WriteString(chain[start:])
	prefixes = append(prefixes, prefix{hash: hash.Sum64(), end: len(chain)})

	c.mu.Lock()
	defer c.mu.Unlock()
	now := time.Now()
	for i := len(prefixes) - 1; i >= 0; i-- {
		candidate := prefixes[i]
		for _, element := range c.prefixes[candidate.hash] {
			entry, _ := element.Value.(*sessionMemoryEntry)
			if !entry.expiresAt.After(now) {
				// The active timer reclaims this entry. Avoid editing the
				// collision bucket while it is being iterated.
				continue
			}
			if len(entry.key) != len(ns)+candidate.end || !strings.HasPrefix(entry.key, ns) || entry.key[len(ns):] != chain[:candidate.end] {
				continue
			}
			c.lru.MoveToFront(element)
			return entry.value, chain[:candidate.end], true
		}
	}
	return nil, "", false
}
