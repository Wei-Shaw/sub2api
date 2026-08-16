package securityaudit

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"
)

const (
	promptResultCacheVersion     = "v1"
	defaultPromptResultCacheSize = 4096
	defaultPromptResultCacheTTL  = 15 * time.Minute
)

// PromptResultCache stores only normalized scanner results. The raw prompt is
// never retained, and every key is scoped to the current audit policy and
// endpoint configuration.
type PromptResultCache interface {
	Get(key string) (*NormalizedResult, bool)
	Set(key string, result *NormalizedResult)
}

type promptResultCacheEntry struct {
	result    *NormalizedResult
	expiresAt time.Time
}

type memoryPromptResultCache struct {
	mu         sync.Mutex
	maxEntries int
	ttl        time.Duration
	entries    map[string]promptResultCacheEntry
}

func NewPromptResultCache() PromptResultCache {
	return newPromptResultCache(defaultPromptResultCacheSize, defaultPromptResultCacheTTL)
}

func newPromptResultCache(maxEntries int, ttl time.Duration) PromptResultCache {
	if maxEntries <= 0 {
		maxEntries = defaultPromptResultCacheSize
	}
	if ttl <= 0 {
		ttl = defaultPromptResultCacheTTL
	}
	return &memoryPromptResultCache{
		maxEntries: maxEntries,
		ttl:        ttl,
		entries:    make(map[string]promptResultCacheEntry),
	}
}

func (c *memoryPromptResultCache) Get(key string) (*NormalizedResult, bool) {
	if c == nil || strings.TrimSpace(key) == "" {
		return nil, false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	entry, ok := c.entries[key]
	if !ok {
		return nil, false
	}
	if !entry.expiresAt.After(time.Now()) {
		delete(c.entries, key)
		return nil, false
	}
	return cloneNormalizedResult(entry.result), true
}

func (c *memoryPromptResultCache) Set(key string, result *NormalizedResult) {
	if c == nil || strings.TrimSpace(key) == "" || result == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	now := time.Now()
	if _, exists := c.entries[key]; !exists && len(c.entries) >= c.maxEntries {
		c.evictOne(now)
	}
	c.entries[key] = promptResultCacheEntry{
		result:    cloneNormalizedResult(result),
		expiresAt: now.Add(c.ttl),
	}
}

func (c *memoryPromptResultCache) evictOne(now time.Time) {
	var oldestKey string
	var oldest time.Time
	for key, entry := range c.entries {
		if !entry.expiresAt.After(now) {
			delete(c.entries, key)
			return
		}
		if oldestKey == "" || entry.expiresAt.Before(oldest) {
			oldestKey, oldest = key, entry.expiresAt
		}
	}
	if oldestKey != "" {
		delete(c.entries, oldestKey)
	}
}

func cloneNormalizedResult(result *NormalizedResult) *NormalizedResult {
	if result == nil {
		return nil
	}
	clone := *result
	clone.Categories = append([]string(nil), result.Categories...)
	clone.MatchedScanners = append([]string(nil), result.MatchedScanners...)
	clone.UnknownCategories = append([]string(nil), result.UnknownCategories...)
	clone.ScannerScores = cloneFloatMap(result.ScannerScores)
	clone.ScannerEvidence = cloneStringMap(result.ScannerEvidence)
	return &clone
}

func cloneFloatMap(source map[string]float64) map[string]float64 {
	if source == nil {
		return nil
	}
	clone := make(map[string]float64, len(source))
	for key, value := range source {
		clone[key] = value
	}
	return clone
}

func cloneStringMap(source map[string]string) map[string]string {
	if source == nil {
		return nil
	}
	clone := make(map[string]string, len(source))
	for key, value := range source {
		clone[key] = value
	}
	return clone
}

func promptResultCacheKey(cfg ActiveConfig, endpoint ActiveEndpoint, scanners []string, chunk string) string {
	orderedScanners := append([]string(nil), scanners...)
	sort.Strings(orderedScanners)
	policy := fmt.Sprintf("%d|%s|%s|%s|%s|%d|%d|%s|%s",
		cfg.ConfigVersion, endpoint.ID, endpoint.Protocol, endpoint.BaseURL,
		endpoint.Model, endpoint.TimeoutMS, endpoint.InputLimit,
		strings.Join(orderedScanners, ","), endpoint.Token)
	chunkHash := sha256.Sum256([]byte(chunk))
	digest := sha256.Sum256([]byte(promptResultCacheVersion + "|" + policy + "|" + hex.EncodeToString(chunkHash[:])))
	return promptResultCacheVersion + ":" + hex.EncodeToString(digest[:])
}

func getPromptResultFromCache(cache PromptResultCache, cfg ActiveConfig, endpoints []ActiveEndpoint, scanners []string, chunk string) *NormalizedResult {
	if cache == nil {
		return nil
	}
	for _, endpoint := range endpoints {
		if result, ok := cache.Get(promptResultCacheKey(cfg, endpoint, scanners, chunk)); ok {
			return result
		}
	}
	return nil
}

func setPromptResultCache(cache PromptResultCache, cfg ActiveConfig, endpoints []ActiveEndpoint, scanners []string, chunk string, result *NormalizedResult) {
	if cache == nil || result == nil {
		return
	}
	for _, endpoint := range endpoints {
		if endpoint.ID == result.GuardEndpointID {
			cache.Set(promptResultCacheKey(cfg, endpoint, scanners, chunk), result)
			return
		}
	}
	if len(endpoints) > 0 {
		cache.Set(promptResultCacheKey(cfg, endpoints[0], scanners, chunk), result)
	}
}
