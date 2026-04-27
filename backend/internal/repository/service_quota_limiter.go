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

const (
	// concurrencyAcquireLeakWindow 是 Acquire 槽位被认为泄漏（持有方崩溃未 Release）
	// 后可被新请求自动回收的时长，单位秒；与历史值保持一致以避免行为漂移。
	concurrencyAcquireLeakWindow = 300

	// concurrencyAcquireKeyTTL 是 Acquire ZSET key 的过期时间，
	// 超过这个时间没有新请求触达就让 Redis 自然清掉，避免长期空 key。
	concurrencyAcquireKeyTTL = 10 * time.Minute
)

// concurrencyAcquireScript 把 Acquire 的 4 个步骤（清过期 → 计数 → ZADD → EXPIRE）
// 合成一条 Lua 脚本以保证整体原子性。
//
// 不用 Pipeline / Tx 的原因：高并发下两个请求都看到 ZCARD = limit-1 时
// 都会通过判定，最终 ZADD 后实际持有数会超过 limit。Lua 单线程执行可以杜绝
// 这种 check-then-act 竞态。
//
// KEYS[1] = ZSET key（如 svcquota:v2:<rule>:<path>:concurrency:<target>）
// ARGV[1] = 过期分数下限（now - leakWindow），ZREMRANGEBYSCORE 用
// ARGV[2] = limit（最大并发数）
// ARGV[3] = now（新槽位 score）
// ARGV[4] = member（请求标识）
// ARGV[5] = key 的 TTL（秒）
//
// 返回：1 = 已占用槽位；0 = 达到上限被拒。
var concurrencyAcquireScript = redis.NewScript(`
local key = KEYS[1]
local cutoff = ARGV[1]
local limit = tonumber(ARGV[2])
local now = tonumber(ARGV[3])
local member = ARGV[4]
local ttl = tonumber(ARGV[5])

redis.call('ZREMRANGEBYSCORE', key, '-inf', cutoff)

local count = redis.call('ZCARD', key)
if count >= limit then
	return 0
end

redis.call('ZADD', key, now, member)
redis.call('EXPIRE', key, ttl)
return 1
`)

// NewServiceQuotaLimiter 构造一个基于 Redis 的服务限额 limiter。
// 当 rdb 为 nil 时，所有方法都返回放行结果（fail-open），便于在没有 Redis
// 的部署中静默降级而不阻塞主流程。
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
		sum += parseRollingMemberDelta(m)
	}
	return sum
}

// sumRollingZScores 解析 ZRangeByScoreWithScores 返回的 redis.Z 切片，把
// member 末段 delta 累加。与 sumRollingMembers 共享同一解析规则
// （parseRollingMemberDelta），避免快照路径与 Current/Increment 路径解耦。
func sumRollingZScores(zs []redis.Z) float64 {
	var sum float64
	for _, z := range zs {
		m, ok := z.Member.(string)
		if !ok {
			continue
		}
		sum += parseRollingMemberDelta(m)
	}
	return sum
}

// parseRollingMemberDelta 解析 "<ts>:<seq>:<delta>" 末段 float。
// 任何格式问题（缺分隔符、ParseFloat 失败）都降级返回 0，与历史 sumRollingMembers
// 行为保持一致——监控读路径不应因 legacy 残留 member 报错。
func parseRollingMemberDelta(m string) float64 {
	idx := strings.LastIndex(m, ":")
	if idx < 0 {
		return 0
	}
	v, err := strconv.ParseFloat(m[idx+1:], 64)
	if err != nil {
		return 0
	}
	return v
}

// Acquire 以原子方式占用一个并发槽位。
//
// 执行 concurrencyAcquireScript（清过期 + 计数判定 + ZADD + EXPIRE 单条 Lua），
// 杜绝旧实现里 ZCARD → ZADD 之间的 check-then-act 竞态。limit <= 0 时退化
// 为放行，确保规则配置缺失不会卡住主流程。
func (l *serviceQuotaLimiter) Acquire(ctx context.Context, key, member string, limit int64) (bool, error) {
	if l == nil || l.rdb == nil || limit <= 0 {
		return true, nil
	}
	now := time.Now().Unix()
	cutoff := strconv.FormatInt(now-concurrencyAcquireLeakWindow, 10)
	ttlSeconds := strconv.FormatInt(int64(concurrencyAcquireKeyTTL.Seconds()), 10)
	res, err := concurrencyAcquireScript.Run(
		ctx,
		l.rdb,
		[]string{key},
		cutoff,
		limit,
		now,
		member,
		ttlSeconds,
	).Int64()
	if err != nil {
		return false, fmt.Errorf("service quota limiter acquire: %w", err)
	}
	return res == 1, nil
}

func (l *serviceQuotaLimiter) Release(ctx context.Context, key, member string) error {
	if l == nil || l.rdb == nil {
		return nil
	}
	return l.rdb.ZRem(ctx, key, member).Err()
}

// Reset 直接 DEL 整个 limiter key，用于管理员手动重置计数。
//
// 三种 limiter（fixed STRING / rolling ZSET / concurrency ZSET）共用同一 DEL：
// fixed/rolling 计数立即归零；concurrency 槽位被强制清空（已在飞的请求 Release
// 时再 ZRem 不存在的成员是 no-op，幂等无害）。
func (l *serviceQuotaLimiter) Reset(ctx context.Context, key string) error {
	if l == nil || l.rdb == nil {
		return nil
	}
	return l.rdb.Del(ctx, key).Err()
}

func fixedWindowTTL(window time.Duration) time.Duration {
	if window >= 24*time.Hour {
		return 48 * time.Hour
	}
	return window
}
