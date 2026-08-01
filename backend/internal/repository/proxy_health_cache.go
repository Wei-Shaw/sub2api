package repository

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/redis/go-redis/v9"
)

const proxyHealthKeyPrefix = "proxy:health:"

func proxyHealthKey(proxyID int64) string {
	return fmt.Sprintf("%s%d", proxyHealthKeyPrefix, proxyID)
}

type proxyHealthCache struct {
	rdb *redis.Client
}

// NewProxyHealthCache constructs a Redis-backed ProxyHealthCache.
func NewProxyHealthCache(rdb *redis.Client) service.ProxyHealthCache {
	return &proxyHealthCache{rdb: rdb}
}

func (c *proxyHealthCache) GetProxyHealth(ctx context.Context, proxyID int64) (*service.ProxyHealthMeta, error) {
	if c == nil || c.rdb == nil || proxyID <= 0 {
		return nil, nil
	}
	raw, err := c.rdb.Get(ctx, proxyHealthKey(proxyID)).Result()
	if err == redis.Nil {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var meta service.ProxyHealthMeta
	if err := json.Unmarshal([]byte(raw), &meta); err != nil {
		return nil, err
	}
	return &meta, nil
}

func (c *proxyHealthCache) SetProxyHealth(ctx context.Context, proxyID int64, meta *service.ProxyHealthMeta) error {
	if c == nil || c.rdb == nil || meta == nil || proxyID <= 0 {
		return nil
	}
	payload, err := json.Marshal(meta)
	if err != nil {
		return err
	}
	// No TTL: health counters must survive process restarts until thresholds fire.
	return c.rdb.Set(ctx, proxyHealthKey(proxyID), payload, 0).Err()
}
