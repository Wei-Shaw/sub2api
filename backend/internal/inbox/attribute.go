package inbox

import (
	"context"
	"sync"
	"time"
)

// AttributeProvider 提供用户属性，用于广播 targeting 求值（fan-out on read）。
// 键为属性名（如 "role" / "plan" / "country"），值为 string / bool / 数值。
type AttributeProvider interface {
	Fetch(ctx context.Context, userID int64) (map[string]any, error)
}

// AttributeProviderFunc 把普通函数适配为 AttributeProvider。
type AttributeProviderFunc func(ctx context.Context, userID int64) (map[string]any, error)

// Fetch 实现 AttributeProvider。
func (f AttributeProviderFunc) Fetch(ctx context.Context, userID int64) (map[string]any, error) {
	return f(ctx, userID)
}

// noopAttributeProvider 恒返回空属性。此时仅 {"op":"all_users"} 类广播会命中。
type noopAttributeProvider struct{}

// NewNoopAttributeProvider 返回一个恒空的属性提供者（降级 / 测试用）。
func NewNoopAttributeProvider() AttributeProvider { return noopAttributeProvider{} }

func (noopAttributeProvider) Fetch(context.Context, int64) (map[string]any, error) {
	return map[string]any{}, nil
}

// cachedAttrs 是缓存条目。
type cachedAttrs struct {
	attrs     map[string]any
	expiresAt time.Time
}

// cachingAttributeProvider 为底层 provider 增加带 TTL 的内存缓存，避免广播 fan-out
// 时对每个连接重复查询用户属性。
type cachingAttributeProvider struct {
	inner AttributeProvider
	ttl   time.Duration
	now   func() time.Time

	mu    sync.Mutex
	cache map[int64]cachedAttrs
}

// NewCachingAttributeProvider 包装 inner，缓存 ttl 时长。ttl<=0 时退化为直通。
func NewCachingAttributeProvider(inner AttributeProvider, ttl time.Duration) AttributeProvider {
	if inner == nil {
		inner = NewNoopAttributeProvider()
	}
	if ttl <= 0 {
		return inner
	}
	return &cachingAttributeProvider{
		inner: inner,
		ttl:   ttl,
		now:   time.Now,
		cache: make(map[int64]cachedAttrs),
	}
}

// Fetch 实现 AttributeProvider，命中未过期缓存则直接返回。
func (c *cachingAttributeProvider) Fetch(ctx context.Context, userID int64) (map[string]any, error) {
	now := c.now()
	c.mu.Lock()
	if e, ok := c.cache[userID]; ok && now.Before(e.expiresAt) {
		c.mu.Unlock()
		return e.attrs, nil
	}
	c.mu.Unlock()

	attrs, err := c.inner.Fetch(ctx, userID)
	if err != nil {
		return nil, err
	}

	c.mu.Lock()
	c.cache[userID] = cachedAttrs{attrs: attrs, expiresAt: now.Add(c.ttl)}
	c.mu.Unlock()
	return attrs, nil
}
