//go:build embed

package web

import (
	"crypto/sha256"
	"encoding/hex"
	"sync"
	"time"
)

// htmlCacheTTL 是「注入进 index.html 的公开配置」缓存的最长存活时间。
//
// 缓存失效有两条路径：
//  1. 进程内的设置写入 → settingService.SetOnUpdateCallback → Invalidate()，立即失效；
//  2. 本 TTL —— 覆盖**触达不到本进程**的写入：直接改库、运维脚本、另一个副本进程。
//
// 没有 TTL 时，第 2 类写入会让注入值**永久滞留旧值**：前端按过期的开关去选数据源/渲染分支，
// 而 /api/v1/settings/public 返回的却是新值，两处真值长期不一致，极难排查。
// 加上 TTL 后，带外写入在有界时间内自动收敛，且进程内写入路径的即时性完全不受影响。
const htmlCacheTTL = 30 * time.Second

// HTMLCache manages the cached index.html with injected settings
type HTMLCache struct {
	mu              sync.RWMutex
	cachedHTML      []byte
	etag            string
	baseHTMLHash    string // Hash of the original index.html (immutable after build)
	settingsVersion uint64 // Incremented when settings change
	expiresAt       time.Time
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
	c.etag = ""
	c.expiresAt = time.Time{}
}

// Get returns the cached HTML or nil if cache is stale
func (c *HTMLCache) Get() *CachedHTML {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if c.cachedHTML == nil {
		return nil
	}
	// 到点即视为未命中：这样绕过 settingService 的带外改设置无需重启进程即可收敛。
	if !c.expiresAt.IsZero() && time.Now().After(c.expiresAt) {
		return nil
	}
	return &CachedHTML{
		Content: c.cachedHTML,
		ETag:    c.etag,
	}
}

// Set updates the cache with new rendered HTML
func (c *HTMLCache) Set(html []byte, settingsJSON []byte) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.cachedHTML = html
	c.etag = c.generateETag(settingsJSON)
	c.expiresAt = time.Now().Add(htmlCacheTTL)
}

// generateETag creates an ETag from base HTML hash + settings hash
func (c *HTMLCache) generateETag(settingsJSON []byte) string {
	settingsHash := sha256.Sum256(settingsJSON)
	return `"` + c.baseHTMLHash + "-" + hex.EncodeToString(settingsHash[:8]) + `"`
}
