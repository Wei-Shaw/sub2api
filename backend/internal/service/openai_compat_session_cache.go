package service

import (
	"strings"
	"sync"
	"time"
)

const (
	openAICompatSessionCacheMaxEntries = 10000
	openAICompatSessionCacheTrimBatch  = 1000
	openAICompatSessionCacheSweepEvery = time.Minute
)

type boundedOpenAICompatSessionCache struct {
	mu        sync.RWMutex
	entries   map[string]any
	lastSweep time.Time
}

func (c *boundedOpenAICompatSessionCache) Load(key string) (any, bool) {
	if c == nil || key == "" {
		return nil, false
	}
	now := time.Now()
	c.mu.Lock()
	value, ok := c.entries[key]
	if ok && openAICompatSessionValueExpired(value, now) {
		delete(c.entries, key)
		value, ok = nil, false
	}
	c.mu.Unlock()
	return value, ok
}

func (c *boundedOpenAICompatSessionCache) Store(key string, value any) {
	if c == nil || key == "" {
		return
	}
	now := time.Now()
	c.mu.Lock()
	if c.entries == nil {
		c.entries = make(map[string]any)
	}
	if c.lastSweep.IsZero() || now.Sub(c.lastSweep) >= openAICompatSessionCacheSweepEvery {
		for cachedKey, cachedValue := range c.entries {
			if openAICompatSessionValueExpired(cachedValue, now) {
				delete(c.entries, cachedKey)
			}
		}
		c.lastSweep = now
	}
	if _, exists := c.entries[key]; !exists && len(c.entries) >= openAICompatSessionCacheMaxEntries {
		trimmed := 0
		for cachedKey := range c.entries {
			delete(c.entries, cachedKey)
			trimmed++
			if trimmed >= openAICompatSessionCacheTrimBatch {
				break
			}
		}
	}
	c.entries[key] = value
	c.mu.Unlock()
}

func (c *boundedOpenAICompatSessionCache) Delete(key string) {
	if c == nil {
		return
	}
	c.mu.Lock()
	delete(c.entries, key)
	c.mu.Unlock()
}

func (c *boundedOpenAICompatSessionCache) DeletePrefix(prefix string) {
	if c == nil || prefix == "" {
		return
	}
	c.mu.Lock()
	for key := range c.entries {
		if strings.HasPrefix(key, prefix) {
			delete(c.entries, key)
		}
	}
	c.mu.Unlock()
}

func (c *boundedOpenAICompatSessionCache) Len() int {
	if c == nil {
		return 0
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	return len(c.entries)
}

func openAICompatSessionValueExpiry(value any) time.Time {
	switch typed := value.(type) {
	case openAICompatSessionResponseBinding:
		return typed.ExpiresAt
	case openAICompatAnthropicDigestBinding:
		return typed.ExpiresAt
	default:
		return time.Time{}
	}
}

func openAICompatSessionValueExpired(value any, now time.Time) bool {
	expiresAt := openAICompatSessionValueExpiry(value)
	return !expiresAt.IsZero() && !now.Before(expiresAt)
}
