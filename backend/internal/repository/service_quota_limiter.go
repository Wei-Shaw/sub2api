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
	// ExpireNX：仅在 key 还没有 TTL 时设置（第一次写入才装；后续写入维持原 TTL，
	// 让对齐到 wall-clock 边界的语义在整窗口内稳定）。
	_ = l.rdb.ExpireNX(ctx, key, fixedWindowTTL(time.Now(), window)).Err()
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

// resetPatternScanCount 是 SCAN 单次 COUNT 提示值，控制每次 SCAN 命令返回的批量大小。
// 选 256 是常规折衷：太小会拉长批数（多次 RTT），太大会让单次 SCAN 阻塞 redis 主线程过久。
const resetPatternScanCount = 256

// ResetPattern 通过 SCAN+DEL 批量清理 pattern 命中的所有 limiter key。
//
// 用法见 service.ServiceQuotaLimiter.ResetPattern 接口注释。底层用非阻塞的 SCAN 游标
// 而非 KEYS（KEYS 会阻塞主线程），分批 DEL（每批最多 resetPatternScanCount 条）。
// 任何中途错误立即返回，已删除的批次不会回滚——按 best-effort 语义可接受
// （上层调用方仅 log warn）。
func (l *serviceQuotaLimiter) ResetPattern(ctx context.Context, pattern string) error {
	if l == nil || l.rdb == nil || pattern == "" {
		return nil
	}
	var cursor uint64
	for {
		keys, next, err := l.rdb.Scan(ctx, cursor, pattern, resetPatternScanCount).Result()
		if err != nil {
			return fmt.Errorf("service quota limiter scan %q: %w", pattern, err)
		}
		if len(keys) > 0 {
			if err := l.rdb.Del(ctx, keys...).Err(); err != nil {
				return fmt.Errorf("service quota limiter del batch (pattern=%q, n=%d): %w", pattern, len(keys), err)
			}
		}
		if next == 0 {
			return nil
		}
		cursor = next
	}
}

// fixedWindowTTL 返回当前 key 距离下一个 wall-clock window 边界的剩余时长，
// 用作 ExpireNX 的 TTL 让"固定窗口"按整分钟/整小时/UTC 整天对齐。
//
// 算法：以 epoch（1970-01-01 UTC 00:00:00）为零点，把 now 切到最近的 window
// 整数倍上，剩余距离即下一个边界的 TTL。
//
//   - RPM/TPM (window=1min)：对齐到下一个分钟整点（now=12:34:18 → TTL=42s）
//   - 自定义短窗 (window=10s)：对齐到 epoch 起的 10s 网格
//   - TPD/daily_usd (window=24h)：对齐到 UTC 当日 00:00（与上游 OpenAI/Anthropic
//     billing 周期一致；不区分服务器本地时区）
//
// 上限保护：原实现对 >=24h 的窗口设 48h TTL 是为了防止 "key 被永久持有"；
// 对齐后 24h 窗口 TTL 自然 ≤ 24h，不再需要这个 cap。但保留下界 1s 确保
// IncrByFloat 紧跟 ExpireNX 之间即使发生时钟跳变也不会写出 0/负的 TTL。
//
// 边界 case：now 恰好落在边界（UnixNano % window == 0），此时 TTL == window
// 而非 0——这一刻被视作"新窗口的起点"，整个窗口长度可用。
func fixedWindowTTL(now time.Time, window time.Duration) time.Duration {
	if window <= 0 {
		return time.Second
	}
	remainder := time.Duration(now.UnixNano() % int64(window))
	if remainder == 0 {
		return window
	}
	return window - remainder
}
