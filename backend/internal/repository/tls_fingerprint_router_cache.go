package repository

import (
	"context"
	"encoding/json"
	"log/slog"
	"sync"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/model"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/redis/go-redis/v9"
)

const (
	tlsFPRouterCacheKey  = "tls_fingerprint_routers"
	tlsFPRouterPubSubKey = "tls_fingerprint_routers_updated"
	tlsFPRouterCacheTTL  = 24 * time.Hour
)

type tlsFingerprintRouterCache struct {
	rdb        *redis.Client
	localCache []*model.TLSFingerprintRouter
	localMu    sync.RWMutex
}

// NewTLSFingerprintRouterCache 创建 TLS 路由器缓存。
func NewTLSFingerprintRouterCache(rdb *redis.Client) service.TLSFingerprintRouterCache {
	return &tlsFingerprintRouterCache{rdb: rdb}
}

// Get 从缓存获取 TLS 路由器列表。
func (c *tlsFingerprintRouterCache) Get(ctx context.Context) ([]*model.TLSFingerprintRouter, bool) {
	c.localMu.RLock()
	if c.localCache != nil {
		routers := c.localCache
		c.localMu.RUnlock()
		return routers, true
	}
	c.localMu.RUnlock()

	data, err := c.rdb.Get(ctx, tlsFPRouterCacheKey).Bytes()
	if err != nil {
		if err != redis.Nil {
			slog.Warn("tls_fp_router_cache_get_failed", "error", err)
		}
		return nil, false
	}

	var routers []*model.TLSFingerprintRouter
	if err := json.Unmarshal(data, &routers); err != nil {
		slog.Warn("tls_fp_router_cache_unmarshal_failed", "error", err)
		return nil, false
	}

	c.localMu.Lock()
	c.localCache = routers
	c.localMu.Unlock()
	return routers, true
}

// Set 设置 TLS 路由器缓存。
func (c *tlsFingerprintRouterCache) Set(ctx context.Context, routers []*model.TLSFingerprintRouter) error {
	data, err := json.Marshal(routers)
	if err != nil {
		return err
	}
	if err := c.rdb.Set(ctx, tlsFPRouterCacheKey, data, tlsFPRouterCacheTTL).Err(); err != nil {
		return err
	}
	c.localMu.Lock()
	c.localCache = routers
	c.localMu.Unlock()
	return nil
}

// Invalidate 使 TLS 路由器缓存失效。
func (c *tlsFingerprintRouterCache) Invalidate(ctx context.Context) error {
	c.localMu.Lock()
	c.localCache = nil
	c.localMu.Unlock()
	return c.rdb.Del(ctx, tlsFPRouterCacheKey).Err()
}

// NotifyUpdate 通知其他实例刷新 TLS 路由器缓存。
func (c *tlsFingerprintRouterCache) NotifyUpdate(ctx context.Context) error {
	return c.rdb.Publish(ctx, tlsFPRouterPubSubKey, "refresh").Err()
}

// SubscribeUpdates 订阅 TLS 路由器缓存更新通知。
func (c *tlsFingerprintRouterCache) SubscribeUpdates(ctx context.Context, handler func()) {
	go func() {
		sub := c.rdb.Subscribe(ctx, tlsFPRouterPubSubKey)
		defer func() { _ = sub.Close() }()
		ch := sub.Channel()
		for {
			select {
			case <-ctx.Done():
				slog.Debug("tls_fp_router_cache_subscriber_stopped", "reason", "context_done")
				return
			case msg := <-ch:
				if msg == nil {
					slog.Warn("tls_fp_router_cache_subscriber_stopped", "reason", "channel_closed")
					return
				}
				c.localMu.Lock()
				c.localCache = nil
				c.localMu.Unlock()
				handler()
			}
		}
	}()
}
