package repository

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/redis/go-redis/v9"
)

type serviceQuotaLimiter struct{ rdb *redis.Client }

var rollingMemberSeq atomic.Uint64

func NewServiceQuotaLimiter(rdb *redis.Client) service.ServiceQuotaLimiter {
	return &serviceQuotaLimiter{rdb: rdb}
}

func (l *serviceQuotaLimiter) Current(ctx context.Context, key string, window time.Duration, mode string) (float64, error) {
	if l == nil || l.rdb == nil {
		return 0, nil
	}
	if mode == service.ServiceQuotaWindowRolling {
		nowMs := time.Now().UnixMilli()
		cutoff := strconv.FormatInt(nowMs-window.Milliseconds(), 10)
		_, _ = l.rdb.ZRemRangeByScore(ctx, key, "-inf", cutoff).Result()
		members, err := l.rdb.ZRange(ctx, key, 0, -1).Result()
		if err != nil {
			return 0, err
		}
		return sumRollingMembers(members), nil
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
		now := time.Now()
		nowMs := now.UnixMilli()
		cutoff := strconv.FormatInt(nowMs-window.Milliseconds(), 10)
		member := rollingMember(now, delta)
		pipe := l.rdb.TxPipeline()
		pipe.ZRemRangeByScore(ctx, key, "-inf", cutoff)
		pipe.ZAdd(ctx, key, redis.Z{Score: float64(nowMs), Member: member})
		pipe.Expire(ctx, key, window*2)
		rangeCmd := pipe.ZRange(ctx, key, 0, -1)
		if _, err := pipe.Exec(ctx); err != nil {
			return 0, err
		}
		return sumRollingMembers(rangeCmd.Val()), nil
	}
	val, err := l.rdb.IncrByFloat(ctx, key, delta).Result()
	if err != nil {
		return 0, err
	}
	_ = l.rdb.ExpireNX(ctx, key, fixedWindowTTL(window)).Err()
	return val, nil
}

func rollingMember(now time.Time, delta float64) string {
	seq := rollingMemberSeq.Add(1)
	return fmt.Sprintf("%d:%d:%s", now.UnixNano(), seq, strconv.FormatFloat(delta, 'f', -1, 64))
}

// Members use the layout "<ts>:<seq>:<delta>" so the rolling window's true
// aggregate (tokens, USD) is the sum of parsed deltas — a plain ZCARD would
// only count events. Unparseable residue from an older encoding degrades to
// zero instead of breaking reads.
func sumRollingMembers(members []string) float64 {
	var sum float64
	for _, m := range members {
		idx := strings.LastIndex(m, ":")
		if idx < 0 {
			continue
		}
		v, err := strconv.ParseFloat(m[idx+1:], 64)
		if err != nil {
			continue
		}
		sum += v
	}
	return sum
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
