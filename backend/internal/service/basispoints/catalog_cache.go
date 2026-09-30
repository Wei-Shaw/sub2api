package basispoints

import (
	"container/list"
	"encoding/json"
	"fmt"
	"sort"
	"sync"
	"time"
)

// CatalogCache holds immutable, versioned client declarations, not responses.
// Only callers with an account/key/trusted-session scope may opt into reuse.
// Explicit tools replace the catalog (including []); omitted tools inherit it.
// Additional tools merge through Prepare's duplicate/schema validation.
type CatalogCache struct {
	mu      sync.Mutex
	entries map[string]*list.Element
	order   list.List
	bytes   int
	version uint64
	now     func() time.Time
}
type catalogEntry struct {
	scope   string
	raw     []byte
	version uint64
	touched time.Time
}

func (c *CatalogCache) clock() time.Time {
	if c.now != nil {
		return c.now()
	}
	return time.Now()
}
func (c *CatalogCache) remove(e *list.Element) {
	v, _ := e.Value.(catalogEntry)
	delete(c.entries, v.scope)
	c.bytes -= len(v.raw)
	c.order.Remove(e)
}
func (c *CatalogCache) prune(now time.Time) {
	for e := c.order.Front(); e != nil; e = c.order.Front() {
		entry, _ := e.Value.(catalogEntry)
		if now.Sub(entry.touched) < replayCacheIdleTTL {
			break
		}
		c.remove(e)
	}
}
func (c *CatalogCache) snapshot(scope string) ([]byte, uint64) {
	c.mu.Lock()
	defer c.mu.Unlock()
	now := c.clock()
	c.prune(now)
	e := c.entries[scope]
	if e == nil {
		return nil, 0
	}
	v, _ := e.Value.(catalogEntry)
	v.touched = now
	e.Value = v
	c.order.MoveToBack(e)
	return v.raw, v.version
}
func (c *CatalogCache) commit(scope string, expected uint64, raw []byte) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	now := c.clock()
	c.prune(now)
	current := uint64(0)
	old := c.entries[scope]
	if old != nil {
		entry, _ := old.Value.(catalogEntry)
		current = entry.version
	}
	if current != expected {
		return false
	}
	if c.entries == nil {
		c.entries = make(map[string]*list.Element)
	}
	if old != nil {
		c.remove(old)
	}
	c.version++
	c.entries[scope] = c.order.PushBack(catalogEntry{scope: scope, raw: raw, version: c.version, touched: now})
	c.bytes += len(raw)
	for len(c.entries) > 512 || c.bytes > 16<<20 {
		c.remove(c.order.Front())
	}
	return true
}

func PrepareWithCatalog(raw []byte, scope string, replay *ReplayCache, cache *CatalogCache) ([]byte, *Bridge, error) {
	return prepareWithCatalog(raw, scope, replay, cache, nil)
}

func prepareWithCatalog(raw []byte, scope string, replay *ReplayCache, cache *CatalogCache, nativeToolImages map[string]bool) ([]byte, *Bridge, error) {
	if cache == nil || scope == "" {
		return prepare(raw, scope, replay, nativeToolImages)
	}
	var source object
	if decode(raw, &source) != nil || source == nil {
		return nil, nil, fmt.Errorf("invalid Basispoints request JSON")
	}
	if text(source["tool_choice"]) == "none" {
		return prepare(raw, scope, replay, nativeToolImages)
	}
	_, explicit := source["tools"]
	for attempt := 0; attempt < 8; attempt++ {
		previous, version := cache.snapshot(scope)
		var candidate object
		if err := decode(raw, &candidate); err != nil {
			return nil, nil, err
		}
		if !explicit && previous != nil {
			var inherited []any
			if err := decode(previous, &inherited); err != nil {
				return nil, nil, err
			}
			candidate["tools"] = inherited
		}
		encoded, err := json.Marshal(candidate)
		if err != nil {
			return nil, nil, err
		}
		body, b, err := prepare(encoded, scope, replay, nativeToolImages)
		if err != nil {
			return nil, nil, err
		}
		keys := make([]string, 0, len(b.tools))
		for key := range b.tools {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		declarations := make([]any, 0, len(keys))
		for _, key := range keys {
			info := b.tools[key]
			var declaration any = info.Catalog
			if info.Namespace != "" {
				declaration = object{"type": "namespace", "name": info.Namespace, "tools": []any{declaration}}
			}
			declarations = append(declarations, declaration)
		}
		saved, err := json.Marshal(declarations)
		if err != nil {
			return nil, nil, err
		}
		if len(saved) > 1<<20 {
			return nil, nil, fmt.Errorf("basispoints session tool catalog exceeds 1 MiB")
		}
		if cache.commit(scope, version, saved) {
			return body, b, nil
		}
		// A concurrent explicit replacement is authoritative. This request keeps
		// its own validated catalog, but cannot overwrite the newer snapshot.
		if explicit {
			return body, b, nil
		}
		// Concurrent incremental declarations merge against a fresh snapshot.
	}
	return nil, nil, fmt.Errorf("basispoints tool catalog changed concurrently; retry request")
}

// Reprepare applies uploaded attachment references using this request's validated
// catalog. It neither reads nor replaces a newer shared session catalog.
func (b *Bridge) Reprepare(raw []byte) ([]byte, *Bridge, error) {
	var source object
	if err := decode(raw, &source); err != nil || source == nil {
		return nil, nil, fmt.Errorf("invalid Basispoints request JSON")
	}
	if _, explicit := source["tools"]; !explicit {
		keys := make([]string, 0, len(b.tools))
		for key := range b.tools {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		declarations := make([]any, 0, len(keys))
		for _, key := range keys {
			info := b.tools[key]
			var declaration any = info.Catalog
			if info.Namespace != "" {
				declaration = object{"type": "namespace", "name": info.Namespace, "tools": []any{declaration}}
			}
			declarations = append(declarations, declaration)
		}
		source["tools"] = declarations
	}
	encoded, err := json.Marshal(source)
	if err != nil {
		return nil, nil, err
	}
	return prepare(encoded, b.scope, b.replay, b.nativeToolImages)
}
