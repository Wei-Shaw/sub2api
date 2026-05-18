//go:build embed

package web

import (
	"container/list"
	"crypto/sha256"
	"encoding/hex"
	"sync"
)

// CachedHTML represents one cached HTML entry.
type CachedHTML struct {
	Content []byte
	ETag    string
}

// HTMLCache is a small bounded LRU cache keyed by (path, lang).
// Backwards-compatible Get/Set keep the legacy single-entry surface.
type HTMLCache struct {
	mu              sync.Mutex
	entries         map[string]*list.Element
	order           *list.List
	maxEntries      int
	baseHTMLHash    string
	settingsVersion uint64

	// Legacy single-entry compatibility (pre-SEO code path).
	legacyHTML []byte
	legacyETag string
}

type cacheNode struct {
	key string
	val *CachedHTML
}

const defaultMaxEntries = 50

// NewHTMLCache creates a new HTML cache instance.
func NewHTMLCache() *HTMLCache {
	return &HTMLCache{
		entries:    make(map[string]*list.Element),
		order:      list.New(),
		maxEntries: defaultMaxEntries,
	}
}

// SetBaseHTML records the base HTML hash, used in ETag generation.
func (c *HTMLCache) SetBaseHTML(baseHTML []byte) {
	c.mu.Lock()
	defer c.mu.Unlock()
	h := sha256.Sum256(baseHTML)
	c.baseHTMLHash = hex.EncodeToString(h[:8])
}

// SetMaxEntries configures the LRU bound.
func (c *HTMLCache) SetMaxEntries(n int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if n < 1 {
		n = 1
	}
	c.maxEntries = n
	for c.order.Len() > c.maxEntries {
		c.evictLocked()
	}
}

// Invalidate marks ALL entries stale.
func (c *HTMLCache) Invalidate() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.settingsVersion++
	c.entries = make(map[string]*list.Element)
	c.order.Init()
	c.legacyHTML = nil
	c.legacyETag = ""
}

// GetKey returns the cached entry for a key, or nil if missing.
func (c *HTMLCache) GetKey(key string) *CachedHTML {
	c.mu.Lock()
	defer c.mu.Unlock()
	el, ok := c.entries[key]
	if !ok {
		return nil
	}
	c.order.MoveToFront(el)
	n := el.Value.(*cacheNode)
	return n.val
}

// SetKey stores html for key with an ETag derived from base + settings.
func (c *HTMLCache) SetKey(key string, html []byte, settingsJSON []byte) {
	c.mu.Lock()
	defer c.mu.Unlock()
	etag := c.generateETagLocked(settingsJSON)
	node := &cacheNode{key: key, val: &CachedHTML{Content: html, ETag: etag}}
	if el, ok := c.entries[key]; ok {
		el.Value = node
		c.order.MoveToFront(el)
		return
	}
	el := c.order.PushFront(node)
	c.entries[key] = el
	for c.order.Len() > c.maxEntries {
		c.evictLocked()
	}
}

// Get is the legacy single-entry getter (used by pre-SEO code path).
func (c *HTMLCache) Get() *CachedHTML {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.legacyHTML == nil {
		return nil
	}
	return &CachedHTML{Content: c.legacyHTML, ETag: c.legacyETag}
}

// Set is the legacy single-entry setter.
func (c *HTMLCache) Set(html []byte, settingsJSON []byte) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.legacyHTML = html
	c.legacyETag = c.generateETagLocked(settingsJSON)
}

func (c *HTMLCache) evictLocked() {
	tail := c.order.Back()
	if tail == nil {
		return
	}
	n := tail.Value.(*cacheNode)
	delete(c.entries, n.key)
	c.order.Remove(tail)
}

func (c *HTMLCache) generateETagLocked(settingsJSON []byte) string {
	h := sha256.Sum256(settingsJSON)
	return `"` + c.baseHTMLHash + "-" + hex.EncodeToString(h[:8]) + `"`
}
