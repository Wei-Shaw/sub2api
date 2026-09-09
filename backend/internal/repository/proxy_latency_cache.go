package repository

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/redis/go-redis/v9"
)

const proxyLatencyKeyPrefix = "proxy:latency:"

func proxyLatencyKey(proxyID int64) string {
	return fmt.Sprintf("%s%d", proxyLatencyKeyPrefix, proxyID)
}

type proxyLatencyCache struct {
	rdb redis.UniversalClient
}

func NewProxyLatencyCache(rdb redis.UniversalClient) service.ProxyLatencyCache {
	return &proxyLatencyCache{rdb: rdb}
}

func (c *proxyLatencyCache) GetProxyLatencies(ctx context.Context, proxyIDs []int64) (map[int64]*service.ProxyLatencyInfo, error) {
	results := make(map[int64]*service.ProxyLatencyInfo)
	if len(proxyIDs) == 0 {
		return results, nil
	}

	pipe := c.rdb.Pipeline()
	commands := make([]*redis.StringCmd, 0, len(proxyIDs))
	for _, id := range proxyIDs {
		commands = append(commands, pipe.Get(ctx, proxyLatencyKey(id)))
	}
	if _, err := pipe.Exec(ctx); err != nil && err != redis.Nil {
		return results, err
	}

	for i, command := range commands {
		raw, err := command.Result()
		if err == redis.Nil {
			continue
		}
		if err != nil {
			return results, err
		}
		var info service.ProxyLatencyInfo
		if err := json.Unmarshal([]byte(raw), &info); err != nil {
			continue
		}
		results[proxyIDs[i]] = &info
	}

	return results, nil
}

func (c *proxyLatencyCache) SetProxyLatency(ctx context.Context, proxyID int64, info *service.ProxyLatencyInfo) error {
	if info == nil {
		return nil
	}
	payload, err := json.Marshal(info)
	if err != nil {
		return err
	}
	return c.rdb.Set(ctx, proxyLatencyKey(proxyID), payload, 0).Err()
}
