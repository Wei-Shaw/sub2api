package repository

import (
	"context"
	"fmt"
	"strconv"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/redis/go-redis/v9"
)

const proxyGroupCacheGenKeyPrefix = "proxy_group:cache_gen:"

type proxyGroupCacheVersionStore struct {
	rdb *redis.Client
}

// NewProxyGroupCacheVersionStore returns a Redis-backed ProxyGroupCacheVersionStore.
// rdb may be nil (all operations no-op / gen=0) for local/dev without Redis.
func NewProxyGroupCacheVersionStore(rdb *redis.Client) service.ProxyGroupCacheVersionStore {
	return &proxyGroupCacheVersionStore{rdb: rdb}
}

func proxyGroupCacheGenKey(groupID int64) string {
	return proxyGroupCacheGenKeyPrefix + strconv.FormatInt(groupID, 10)
}

func (s *proxyGroupCacheVersionStore) BumpGeneration(ctx context.Context, groupID int64) (int64, error) {
	if s == nil || s.rdb == nil || groupID <= 0 {
		return 0, nil
	}
	n, err := s.rdb.Incr(ctx, proxyGroupCacheGenKey(groupID)).Result()
	if err != nil {
		return 0, fmt.Errorf("bump proxy group cache gen %d: %w", groupID, err)
	}
	return n, nil
}

func (s *proxyGroupCacheVersionStore) GetGeneration(ctx context.Context, groupID int64) (int64, error) {
	if s == nil || s.rdb == nil || groupID <= 0 {
		return 0, nil
	}
	val, err := s.rdb.Get(ctx, proxyGroupCacheGenKey(groupID)).Result()
	if err == redis.Nil {
		return 0, nil
	}
	if err != nil {
		return 0, fmt.Errorf("get proxy group cache gen %d: %w", groupID, err)
	}
	n, err := strconv.ParseInt(val, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("parse proxy group cache gen %d: %w", groupID, err)
	}
	return n, nil
}
