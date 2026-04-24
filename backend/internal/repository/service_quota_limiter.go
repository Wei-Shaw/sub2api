package repository

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/redis/go-redis/v9"
)

type serviceQuotaLimiter struct{ rdb *redis.Client }

func NewServiceQuotaLimiter(rdb *redis.Client) service.ServiceQuotaLimiter {
	return &serviceQuotaLimiter{rdb: rdb}
}

func (l *serviceQuotaLimiter) Current(ctx context.Context, key string, window time.Duration, mode string) (float64, error) {
	if l == nil || l.rdb == nil {
		return 0, nil
	}
	if mode == service.ServiceQuotaWindowRolling {
		now := time.Now().UnixMilli()
		_, _ = l.rdb.ZRemRangeByScore(ctx, key, "-inf", strconv.FormatInt(now-window.Milliseconds(), 10)).Result()
		count, err := l.rdb.ZCard(ctx, key).Result()
		return float64(count), err
	}
	val, err := l.rdb.Get(ctx, key).Result()
	if errors.Is(err, redis.Nil) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	return strconv.ParseFloat(val, 64)
}

func (l *serviceQuotaLimiter) Increment(ctx context.Context, key string, delta float64, window time.Duration, mode string) (float64, error) {
	if l == nil || l.rdb == nil {
		return 0, nil
	}
	if mode == service.ServiceQuotaWindowRolling {
		now := time.Now().UnixMilli()
		member := fmt.Sprintf("%d:%f", now, delta)
		pipe := l.rdb.TxPipeline()
		pipe.ZRemRangeByScore(ctx, key, "-inf", strconv.FormatInt(now-window.Milliseconds(), 10))
		pipe.ZAdd(ctx, key, redis.Z{Score: float64(now), Member: member})
		pipe.Expire(ctx, key, window*2)
		card := pipe.ZCard(ctx, key)
		_, err := pipe.Exec(ctx)
		return float64(card.Val()), err
	}
	val, err := l.rdb.IncrByFloat(ctx, key, delta).Result()
	if err != nil {
		return 0, err
	}
	_ = l.rdb.ExpireNX(ctx, key, fixedWindowTTL(window)).Err()
	return val, nil
}

func (l *serviceQuotaLimiter) Acquire(ctx context.Context, key, member string, limit int64) (bool, error) {
	if l == nil || l.rdb == nil || limit <= 0 {
		return true, nil
	}
	now := time.Now().Unix()
	_, _ = l.rdb.ZRemRangeByScore(ctx, key, "-inf", strconv.FormatInt(now-300, 10)).Result()
	count, err := l.rdb.ZCard(ctx, key).Result()
	if err != nil || count >= limit {
		return false, err
	}
	err = l.rdb.ZAdd(ctx, key, redis.Z{Score: float64(now), Member: member}).Err()
	_ = l.rdb.Expire(ctx, key, 10*time.Minute).Err()
	return err == nil, err
}

func (l *serviceQuotaLimiter) Release(ctx context.Context, key, member string) error {
	if l == nil || l.rdb == nil {
		return nil
	}
	return l.rdb.ZRem(ctx, key, member).Err()
}

func fixedWindowTTL(window time.Duration) time.Duration {
	if window >= 24*time.Hour {
		return 48 * time.Hour
	}
	return window
}
