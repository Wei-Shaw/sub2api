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
	serviceQuotaCacheTTL   = 5 * time.Minute
)

type serviceQuotaCache struct {
	rdb *redis.Client
}

func NewServiceQuotaCache(rdb *redis.Client) service.ServiceQuotaCache {
	return &serviceQuotaCache{rdb: rdb}
}

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

func (c *serviceQuotaCache) SetRules(ctx context.Context, rules []*service.ServiceQuotaRule) error {
	data, err := json.Marshal(rules)
	if err != nil {
		return err
	}
	return c.rdb.Set(ctx, serviceQuotaRulesKey, data, serviceQuotaCacheTTL).Err()
}

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

func (c *serviceQuotaCache) SetEnabled(ctx context.Context, enabled bool) error {
	val := "false"
	if enabled {
		val = "true"
	}
	return c.rdb.Set(ctx, serviceQuotaEnabledKey, val, serviceQuotaCacheTTL).Err()
}

func (c *serviceQuotaCache) Invalidate(ctx context.Context) error {
	return c.rdb.Del(ctx, serviceQuotaRulesKey, serviceQuotaEnabledKey).Err()
}
