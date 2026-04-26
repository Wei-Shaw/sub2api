package repository

import (
	"context"
	"encoding/json"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/redis/go-redis/v9"
)

const (
	serviceQuotaRulesKey   = "svcquota:rules:all"
	serviceQuotaEnabledKey = "svcquota:enabled"
	// serviceQuotaCacheTTL 是缓存兜底过期时间。多实例场景下写路径只 Del key，
	// 短 TTL 进一步降低实例间因为消息丢失而服务陈旧数据的窗口。
	serviceQuotaCacheTTL = 60 * time.Second
)

type serviceQuotaCache struct {
	rdb *redis.Client
}

// NewServiceQuotaCache 构造一个基于 Redis 的服务限额缓存实现。
func NewServiceQuotaCache(rdb *redis.Client) service.ServiceQuotaCache {
	return &serviceQuotaCache{rdb: rdb}
}

// GetRules 读规则缓存。命中返回 (rules, true, nil)；未命中返回 (nil, false, nil)。
func (c *serviceQuotaCache) GetRules(ctx context.Context) ([]*service.ServiceQuotaRule, bool, error) {
	data, err := c.rdb.Get(ctx, serviceQuotaRulesKey).Bytes()
	if err == redis.Nil {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	var rules []*service.ServiceQuotaRule
	if err := json.Unmarshal(data, &rules); err != nil {
		return nil, false, err
	}
	return rules, true, nil
}

// SetRules 把序列化后的规则写入缓存。仅供 singleflight 读路径回填使用，
// 业务写路径请使用 InvalidateRules / Invalidate。
func (c *serviceQuotaCache) SetRules(ctx context.Context, rules []*service.ServiceQuotaRule) error {
	data, err := json.Marshal(rules)
	if err != nil {
		return err
	}
	return c.rdb.Set(ctx, serviceQuotaRulesKey, data, serviceQuotaCacheTTL).Err()
}

// InvalidateRules 删除规则缓存 key。多实例部署下，写完 DB 后只清缓存，
// 让其他实例下次读时通过 singleflight 重新从 DB 拉取，避免实例间长达
// TTL 的不一致窗口。
func (c *serviceQuotaCache) InvalidateRules(ctx context.Context) error {
	return c.rdb.Del(ctx, serviceQuotaRulesKey).Err()
}

// GetEnabled 读全局开关缓存。未命中返回 (nil, nil)。
func (c *serviceQuotaCache) GetEnabled(ctx context.Context) (*bool, error) {
	val, err := c.rdb.Get(ctx, serviceQuotaEnabledKey).Result()
	if err == redis.Nil {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	enabled := val == "true"
	return &enabled, nil
}

// SetEnabled 把全局开关写入缓存。仅供读路径回填使用，
// 业务写路径请使用 InvalidateEnabled / Invalidate。
func (c *serviceQuotaCache) SetEnabled(ctx context.Context, enabled bool) error {
	val := "false"
	if enabled {
		val = "true"
	}
	return c.rdb.Set(ctx, serviceQuotaEnabledKey, val, serviceQuotaCacheTTL).Err()
}

// InvalidateEnabled 删除全局开关缓存 key。
func (c *serviceQuotaCache) InvalidateEnabled(ctx context.Context) error {
	return c.rdb.Del(ctx, serviceQuotaEnabledKey).Err()
}

// Invalidate 同时失效规则与开关两个 key，是写路径的便捷入口。
func (c *serviceQuotaCache) Invalidate(ctx context.Context) error {
	return c.rdb.Del(ctx, serviceQuotaRulesKey, serviceQuotaEnabledKey).Err()
}
