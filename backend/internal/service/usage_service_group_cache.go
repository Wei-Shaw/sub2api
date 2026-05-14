package service

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"

	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
)

const (
	groupCacheHitRate7dKey = "groups:cache_hit_rate:7d"
	groupCacheHitRate7dTTL = 10 * time.Minute
)

// GetGroupCacheHitRates7d returns map[group_id] -> hit_rate (0..1) over the
// past 7 days (cache_read / (input + cache_read), aggregated across all users).
//
// Redis-cached for 10 minutes under groupCacheHitRate7dKey. Redis errors are
// non-fatal: the call degrades to a direct repository read. The repo already
// filters denom == 0 via SQL HAVING; service-side filter is defensive.
func (s *UsageService) GetGroupCacheHitRates7d(ctx context.Context) (map[int64]float64, error) {
	if s.redisClient != nil {
		cached, err := s.loadGroupCacheHitRates7dFromRedis(ctx)
		if err == nil && cached != nil {
			return cached, nil
		}
		if err != nil && !errors.Is(err, redis.Nil) {
			logger.L().Warn("groups cache hit rate redis read failed", zap.Error(err))
		}
	}

	rows, err := s.usageRepo.GetGroupCacheHitRates7d(ctx)
	if err != nil {
		return nil, err
	}

	out := make(map[int64]float64, len(rows))
	for _, r := range rows {
		if r.Denom <= 0 {
			continue
		}
		out[r.GroupID] = r.CacheRead / r.Denom
	}

	if s.redisClient != nil {
		if err := s.storeGroupCacheHitRates7dToRedis(ctx, out); err != nil {
			logger.L().Warn("groups cache hit rate redis write failed", zap.Error(err))
		}
	}

	return out, nil
}

func (s *UsageService) loadGroupCacheHitRates7dFromRedis(ctx context.Context) (map[int64]float64, error) {
	raw, err := s.redisClient.Get(ctx, groupCacheHitRate7dKey).Bytes()
	if err != nil {
		return nil, err
	}
	out := map[int64]float64{}
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, err
	}
	return out, nil
}

func (s *UsageService) storeGroupCacheHitRates7dToRedis(ctx context.Context, data map[int64]float64) error {
	raw, err := json.Marshal(data)
	if err != nil {
		return err
	}
	return s.redisClient.Set(ctx, groupCacheHitRate7dKey, raw, groupCacheHitRate7dTTL).Err()
}
