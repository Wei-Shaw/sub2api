package repository

import (
	"context"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/port/dashboard"
	"github.com/redis/go-redis/v9"
)

const dashboardStatsCacheKey = "dashboard:stats:v1"

type dashboardCache struct {
	rdb       *redis.Client
	keyPrefix string
}

func NewDashboardCache(rdb *redis.Client, cfg *config.Config) dashboard.DashboardStatsCache {
	prefix := "sub2api:"
	if cfg != nil {
		prefix = strings.TrimSpace(cfg.Dashboard.KeyPrefix)
	}
	if prefix != "" && !strings.HasSuffix(prefix, ":") {
		prefix += ":"
	}
	return &dashboardCache{
		rdb:       rdb,
		keyPrefix: prefix,
	}
}

func (c *dashboardCache) GetDashboardStats(ctx context.Context) (string, error) {
	val, err := c.rdb.Get(ctx, c.buildKey()).Result()
	if err != nil {
		if err == redis.Nil {
			return "", dashboard.ErrDashboardStatsCacheMiss
		}
		return "", err
	}
	return val, nil
}

func (c *dashboardCache) SetDashboardStats(ctx context.Context, data string, ttl time.Duration) error {
	return c.rdb.Set(ctx, c.buildKey(), data, ttl).Err()
}

func (c *dashboardCache) buildKey() string {
	if c.keyPrefix == "" {
		return dashboardStatsCacheKey
	}
	return c.keyPrefix + dashboardStatsCacheKey
}

func (c *dashboardCache) DeleteDashboardStats(ctx context.Context) error {
	return c.rdb.Del(ctx, c.buildKey()).Err()
}
