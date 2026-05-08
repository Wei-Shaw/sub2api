//go:build embed

package web

import (
	"crypto/sha256"
	"encoding/hex"
	"sync"
)

// HTMLCache manages the cached index.html with injected settings
type HTMLCache struct {
	mu              sync.RWMutex
	cachedHTML      map[string][]byte
	etag            map[string]string
	baseHTMLHash    string // Hash of the original index.html (immutable after build)
	settingsVersion uint64 // Incremented when settings change
}

// CachedHTML represents the cache state
type CachedHTML struct {
	Content []byte
	ETag    string
}

// NewHTMLCache creates a new HTML cache instance
func NewHTMLCache() *HTMLCache {
	return &HTMLCache{}
}

// SetBaseHTML initializes the cache with the base HTML template
func (c *HTMLCache) SetBaseHTML(baseHTML []byte) {
	c.mu.Lock()
	defer c.mu.Unlock()

	hash := sha256.Sum256(baseHTML)
	c.baseHTMLHash = hex.EncodeToString(hash[:8]) // First 8 bytes for brevity
}

// Invalidate marks the cache as stale
func (c *HTMLCache) Invalidate() {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.settingsVersion++
	c.cachedHTML = nil
	c.etag = nil
}

// Get returns the cached HTML or nil if cache is stale
func (c *HTMLCache) Get(cacheKey string) *CachedHTML {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if c.cachedHTML == nil {
		return nil
	}
	content, ok := c.cachedHTML[cacheKey]
	if !ok {
		return nil
	}
	return &CachedHTML{
		Content: content,
		ETag:    c.etag[cacheKey],
	}
}

// Set updates the cache with new rendered HTML
func (c *HTMLCache) Set(cacheKey string, html []byte, settingsJSON []byte) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.cachedHTML == nil {
		c.cachedHTML = make(map[string][]byte)
	}
	if c.etag == nil {
		c.etag = make(map[string]string)
	}
	c.cachedHTML[cacheKey] = html
	c.etag[cacheKey] = c.generateETag(cacheKey, settingsJSON)
}

// generateETag creates an ETag from base HTML hash + settings hash
func (c *HTMLCache) generateETag(cacheKey string, settingsJSON []byte) string {
	settingsHash := sha256.Sum256(append([]byte(cacheKey+"|"), settingsJSON...))
	return `"` + c.baseHTMLHash + "-" + hex.EncodeToString(settingsHash[:8]) + `"`
}
