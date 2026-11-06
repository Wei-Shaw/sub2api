package repository

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/compresslevel"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/redis/go-redis/v9"
)

const (
	trafficCompressionRedisKey = "settings:traffic_compression_levels"
	trafficCompressionRedisTTL = 5 * time.Minute
)

type trafficCompressionCache struct {
	client *redis.Client
}

// NewTrafficCompressionCache creates the shared compression settings cache.
func NewTrafficCompressionCache(client *redis.Client) service.TrafficCompressionCache {
	return &trafficCompressionCache{client: client}
}

func (c *trafficCompressionCache) Get(ctx context.Context) (*compresslevel.Settings, error) {
	if c.client == nil {
		return nil, nil
	}
	raw, err := c.client.Get(ctx, trafficCompressionRedisKey).Result()
	if errors.Is(err, redis.Nil) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var levels compresslevel.Settings
	if err := json.Unmarshal([]byte(raw), &levels); err != nil {
		return nil, fmt.Errorf("decode traffic compression levels: %w", err)
	}
	if err := levels.Validate(); err != nil {
		return nil, fmt.Errorf("validate traffic compression levels: %w", err)
	}
	return &levels, nil
}

func (c *trafficCompressionCache) Set(ctx context.Context, levels compresslevel.Settings) error {
	if c.client == nil {
		return nil
	}
	payload, err := json.Marshal(levels)
	if err != nil {
		return fmt.Errorf("encode traffic compression levels: %w", err)
	}
	return c.client.Set(ctx, trafficCompressionRedisKey, payload, trafficCompressionRedisTTL).Err()
}
