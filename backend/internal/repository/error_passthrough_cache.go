package repository

import (
	"context"
	"encoding/json"
	"log"
	"sync"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/model"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/redis/go-redis/v9"
)

const (
	errorPassthroughCacheKey  = "error_passthrough_rules"
	errorPassthroughPubSubKey = "error_passthrough_rules_updated"
	errorPassthroughCacheTTL  = 24 * time.Hour
)

type errorPassthroughCache struct {
	rdb        *redis.Client
	localCache []*model.ErrorPassthroughRule
	localMu    sync.RWMutex
}

// NewErrorPassthroughCache 创建错误透传规则缓存
func NewErrorPassthroughCache(rdb *redis.Client) service.ErrorPassthroughCache {
	return &errorPassthroughCache{
		rdb: rdb,
	}
}

// Get 从缓存获取规则列表
func (c *errorPassthroughCache) Get(ctx context.Context) ([]*model.ErrorPassthroughRule, bool) {
	// 先检查本地缓存
	c.localMu.RLock()
	if c.localCache != nil {
		rules := c.localCache
		c.localMu.RUnlock()
		return rules, true
	}
	c.localMu.RUnlock()

	// 从 Redis 获取
	data, err := c.rdb.Get(ctx, errorPassthroughCacheKey).Bytes()
	if err != nil {
		if err != redis.Nil {
			log.Printf("[ErrorPassthroughCache] Failed to get from Redis: %v", err)
		}
		return nil, false
	}

	var rules []*model.ErrorPassthroughRule
	if err := json.Unmarshal(data, &rules); err != nil {
		log.Printf("[ErrorPassthroughCache] Failed to unmarshal rules: %v", err)
		return nil, false
	}

	// 更新本地缓存
	c.localMu.Lock()
	c.localCache = rules
	c.localMu.Unlock()

	return rules, true
}

// Set 设置缓存
func (c *errorPassthroughCache) Set(ctx context.Context, rules []*model.ErrorPassthroughRule) error {
	data, err := json.Marshal(rules)
	if err != nil {
		return err
	}

	if err := c.rdb.Set(ctx, errorPassthroughCacheKey, data, errorPassthroughCacheTTL).Err(); err != nil {
		return err
	}

	// 更新本地缓存
	c.localMu.Lock()
	c.localCache = rules
	c.localMu.Unlock()

	return nil
}

// Invalidate 使缓存失效
func (c *errorPassthroughCache) Invalidate(ctx context.Context) error {
	// 清除本地缓存
	c.localMu.Lock()
	c.localCache = nil
	c.localMu.Unlock()

	// 清除 Redis 缓存
	return c.rdb.Del(ctx, errorPassthroughCacheKey).Err()
}

// NotifyUpdate 通知其他实例刷新缓存
func (c *errorPassthroughCache) NotifyUpdate(ctx context.Context) error {
	return c.rdb.Publish(ctx, errorPassthroughPubSubKey, "refresh").Err()
}

// SubscribeUpdates 订阅缓存更新通知
func (c *errorPassthroughCache) SubscribeUpdates(ctx context.Context, handler func()) {
	go func() {
		sub := c.rdb.Subscribe(ctx, errorPassthroughPubSubKey)
		defer func() { _ = sub.Close() }()

		ch := sub.Channel()
		for {
			select {
			case <-ctx.Done():
				return
			case msg := <-ch:
				if msg == nil {
					return
				}
				// 清除本地缓存，下次访问时会从 Redis 或数据库重新加载
				c.localMu.Lock()
				c.localCache = nil
				c.localMu.Unlock()

				// 调用处理函数
				handler()
			}
		}
	}()
}
