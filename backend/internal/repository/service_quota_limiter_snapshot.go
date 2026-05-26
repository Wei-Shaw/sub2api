package repository

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/redis/go-redis/v9"
)

// 监控页/管理页快照路径与限流写入路径共享 *serviceQuotaLimiter，但严格只读：
// 任何方法都不得 ZRemRangeByScore / ZADD / EXPIRE，避免影响 PreCheck 行为。

// Snapshot 取单 key 当前用量，按 mode 分发到 rolling / fixed / concurrency 三种实现。
//
// nil receiver / nil rdb 时返回零值快照（与 Current 的 fail-open 一致）。
// 不存在的 key 返回 LimiterSnapshot{Exists: false}, nil；其他底层错误向上透传。
func (l *serviceQuotaLimiter) Snapshot(ctx context.Context, key string, window time.Duration, mode string) (service.LimiterSnapshot, error) {
	if l == nil || l.rdb == nil {
		return service.LimiterSnapshot{}, nil
	}
	// mode 字符串协议：fixed/rolling 直接对应 window 模式；mode == "" 表示 concurrency
	// （ZSET，无 window 概念）。SnapshotKey 用 IsConcurrency 字段区分，所以这里把 mode==""
	// 翻译过去，避免误走 fixed 路径对 ZSET key 跑 GET 触发 WRONGTYPE。
	sk := service.SnapshotKey{Key: key, Window: window, Mode: mode}
	if mode == "" {
		sk.IsConcurrency = true
	}
	return l.snapshotByMode(ctx, sk)
}

// snapshotByMode 根据 SnapshotKey 字段路由到具体实现，避免在多个调用点重复 if/else。
func (l *serviceQuotaLimiter) snapshotByMode(ctx context.Context, sk service.SnapshotKey) (service.LimiterSnapshot, error) {
	if sk.IsConcurrency {
		return l.snapshotConcurrency(ctx, sk.Key)
	}
	if sk.Mode == service.ServiceQuotaWindowRolling {
		return l.snapshotRolling(ctx, sk.Key, sk.Window)
	}
	return l.snapshotFixed(ctx, sk.Key)
}

// snapshotRolling 用 ZRangeByScoreWithScores 读取 [(cutoff, +inf]，把 member 末段
// delta 累加。**禁止** ZRemRangeByScore——监控页是只读路径，必须保证多次调用幂等。
//
// 比 Current 的 rolling 实现少一次 ZRemRangeByScore：未来 Increment 自然会清掉过期
// member，监控读不应承担清理职责。
//
// ResetAtUnixMs 用 now + window 表示窗口右端。rolling 模式下任何写入都会推后窗口右端，
// 但快照时刻的"最早可能清零时间"就是当前 now + window。
func (l *serviceQuotaLimiter) snapshotRolling(ctx context.Context, key string, window time.Duration) (service.LimiterSnapshot, error) {
	nowMs := time.Now().UnixMilli()
	cutoff := nowMs - window.Milliseconds()
	resetAt := nowMs + window.Milliseconds()
	res, err := l.rdb.ZRangeByScoreWithScores(ctx, key, &redis.ZRangeBy{
		Min: fmt.Sprintf("(%d", cutoff),
		Max: "+inf",
	}).Result()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return service.LimiterSnapshot{}, nil
		}
		return service.LimiterSnapshot{}, err
	}
	if len(res) == 0 {
		return service.LimiterSnapshot{}, nil
	}
	return service.LimiterSnapshot{Current: sumRollingZScores(res), Exists: true, ResetAtUnixMs: resetAt}, nil
}

// snapshotFixed 用 GET + PTTL 读 fixed-window 计数器；不存在返回 {0,false}，其他 err 透传。
//
// ResetAtUnixMs 由 PTTL 推算：>0 → now + ttl；==-1（永久 key，理论不该出现）按 0 兜底；
// ==-2（key 不存在）→ 已经在 Get 阶段被 redis.Nil 短路，进不到 PTTL。
func (l *serviceQuotaLimiter) snapshotFixed(ctx context.Context, key string) (service.LimiterSnapshot, error) {
	val, err := l.rdb.Get(ctx, key).Float64()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return service.LimiterSnapshot{}, nil
		}
		return service.LimiterSnapshot{}, err
	}
	return service.LimiterSnapshot{
		Current:       val,
		Exists:        true,
		ResetAtUnixMs: fixedResetAtUnixMs(ctx, l.rdb, key),
	}, nil
}

// fixedResetAtUnixMs 读取 key 的 PTTL 并换算成 Unix 毫秒时间戳。
// PTTL>0 → now + ttl；PTTL<=0（永久 / 已过期 / 错误）一律返回 0，让前端隐藏倒计时。
func fixedResetAtUnixMs(ctx context.Context, rdb redis.Cmdable, key string) int64 {
	ttl, err := rdb.PTTL(ctx, key).Result()
	if err != nil || ttl <= 0 {
		return 0
	}
	return time.Now().Add(ttl).UnixMilli()
}

// snapshotConcurrency 用 ZCount 在 (cutoff, +inf] 范围内统计当前持有的并发槽位。
// cutoff 复用 concurrencyAcquireLeakWindow（与 Acquire 的过期判定一致），
// 避免快照与限流读到的活跃槽位数偏离。
func (l *serviceQuotaLimiter) snapshotConcurrency(ctx context.Context, key string) (service.LimiterSnapshot, error) {
	cutoff := time.Now().Unix() - int64(concurrencyAcquireLeakWindow)
	count, err := l.rdb.ZCount(ctx, key, fmt.Sprintf("(%d", cutoff), "+inf").Result()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return service.LimiterSnapshot{}, nil
		}
		return service.LimiterSnapshot{}, err
	}
	if count == 0 {
		return service.LimiterSnapshot{}, nil
	}
	return service.LimiterSnapshot{Current: float64(count), Exists: true}, nil
}

// SnapshotMany 用一条 Redis Pipeline 批量读取多个 key 的快照。
//
// 设计要点：
//   - 单 key 失败（除 redis.Nil 外）降级为 {0,false} 并记 warn 日志，不让一个键拖垮整个批次。
//   - pipe.Exec 的返回值 err 可能聚合自单条命令错误，不能直接 return；按 cmder 逐个解析。
//   - 顺序保持与入参 keys 一一对应，便于上层按下标拼装结果。
//
// receiver 或 rdb 为 nil 时整批返回零值切片，与 Snapshot 单点的 fail-open 行为一致。
func (l *serviceQuotaLimiter) SnapshotMany(ctx context.Context, keys []service.SnapshotKey) ([]service.LimiterSnapshot, error) {
	if len(keys) == 0 {
		return nil, nil
	}
	out := make([]service.LimiterSnapshot, len(keys))
	if l == nil || l.rdb == nil {
		return out, nil
	}

	pipe := l.rdb.Pipeline()
	cmders := dispatchSnapshotCmds(ctx, pipe, keys)
	// pipe.Exec 在任意一条命令出错时返回非 nil err（聚合），但我们逐条解析来区分
	// redis.Nil 与真实错误，因此这里允许 err != nil 继续走下面的 collect 流程。
	_, _ = pipe.Exec(ctx)

	collectSnapshotResults(ctx, keys, cmders, out)
	return out, nil
}

// snapshotCmder 是 SnapshotMany Pipeline 内每条命令的统一句柄，便于 collect 时
// 按 limiter 类型区分解析方式。Mode 同 SnapshotKey.Mode；IsConcurrency 用于走 ZCount。
//
// rollingResetAtMs 是该行 rolling 模式下计算好的窗口右端（now + window），保存下来
// 让 collect 阶段不必重新算时间——避免 dispatch 与 collect 之间窗口推进引起的轻微漂移。
// fixed 模式下 PTTL 必须额外发一条命令，因此持有 ttl PTTL 句柄。
type snapshotCmder struct {
	mode             string
	isConcurrency    bool
	zcount           *redis.IntCmd
	zrangeScores     *redis.ZSliceCmd
	get              *redis.StringCmd
	pttl             *redis.DurationCmd
	rollingResetAtMs int64
}

// dispatchSnapshotCmds 为每个 SnapshotKey 派发对应的 Redis 命令并保存句柄。
// 不在这里读结果，统一由 collectSnapshotResults 处理，便于错误降级集中。
//
// fixed 模式额外发一条 PTTL 命令拿过期时刻；rolling/concurrency 不需要（rolling 由
// dispatch 时刻 now+window 推算，concurrency 不展示倒计时）。
func dispatchSnapshotCmds(ctx context.Context, pipe redis.Pipeliner, keys []service.SnapshotKey) []snapshotCmder {
	cmders := make([]snapshotCmder, len(keys))
	for i, sk := range keys {
		c := snapshotCmder{mode: sk.Mode, isConcurrency: sk.IsConcurrency}
		switch {
		case sk.IsConcurrency:
			cutoff := time.Now().Unix() - int64(concurrencyAcquireLeakWindow)
			c.zcount = pipe.ZCount(ctx, sk.Key, fmt.Sprintf("(%d", cutoff), "+inf")
		case sk.Mode == service.ServiceQuotaWindowRolling:
			nowMs := time.Now().UnixMilli()
			cutoff := nowMs - sk.Window.Milliseconds()
			c.zrangeScores = pipe.ZRangeByScoreWithScores(ctx, sk.Key, &redis.ZRangeBy{
				Min: fmt.Sprintf("(%d", cutoff),
				Max: "+inf",
			})
			c.rollingResetAtMs = nowMs + sk.Window.Milliseconds()
		default:
			c.get = pipe.Get(ctx, sk.Key)
			c.pttl = pipe.PTTL(ctx, sk.Key)
		}
		cmders[i] = c
	}
	return cmders
}

// collectSnapshotResults 按下标读取 cmders 结果并填入 out。
// 单条 redis.Nil → {0,false} 不记日志；其他 err → {0,false} + warn 日志。
func collectSnapshotResults(ctx context.Context, keys []service.SnapshotKey, cmders []snapshotCmder, out []service.LimiterSnapshot) {
	for i, c := range cmders {
		snap, err := readSnapshotCmd(c)
		if err != nil {
			logSnapshotCmdError(ctx, keys[i], err)
			out[i] = service.LimiterSnapshot{}
			continue
		}
		out[i] = snap
	}
}

// readSnapshotCmd 把单条 Pipeline 命令的结果转成 LimiterSnapshot。
// redis.Nil 视为 key 不存在 → {0,false}, nil；其他 err 直接返回。
//
// ResetAtUnixMs：concurrency 恒为 0；rolling 用 dispatch 阶段缓存的窗口右端；
// fixed 用 PTTL 推算（PTTL<=0 / err 时返回 0，前端隐藏倒计时）。
func readSnapshotCmd(c snapshotCmder) (service.LimiterSnapshot, error) {
	switch {
	case c.isConcurrency:
		val, err := c.zcount.Result()
		if errors.Is(err, redis.Nil) {
			return service.LimiterSnapshot{}, nil
		}
		if err != nil {
			return service.LimiterSnapshot{}, err
		}
		if val == 0 {
			return service.LimiterSnapshot{}, nil
		}
		return service.LimiterSnapshot{Current: float64(val), Exists: true}, nil
	case c.mode == service.ServiceQuotaWindowRolling:
		zs, err := c.zrangeScores.Result()
		if errors.Is(err, redis.Nil) {
			return service.LimiterSnapshot{}, nil
		}
		if err != nil {
			return service.LimiterSnapshot{}, err
		}
		if len(zs) == 0 {
			return service.LimiterSnapshot{}, nil
		}
		return service.LimiterSnapshot{Current: sumRollingZScores(zs), Exists: true, ResetAtUnixMs: c.rollingResetAtMs}, nil
	default:
		val, err := c.get.Float64()
		if errors.Is(err, redis.Nil) {
			return service.LimiterSnapshot{}, nil
		}
		if err != nil {
			// Get 偶尔会因为 value 不是合法 float 而返回非 redis.Nil 错误（数据被外部
			// 工具污染）；统一交给 caller 走降级 + warn，不直接 panic。
			return service.LimiterSnapshot{}, err
		}
		return service.LimiterSnapshot{Current: val, Exists: true, ResetAtUnixMs: pipelinePTTLToUnixMs(c.pttl)}, nil
	}
}

// pipelinePTTLToUnixMs 把 Pipeline 内的 PTTL 命令结果换算成 Unix 毫秒时间戳。
// 错误 / PTTL<=0（永久 key 或已过期）均返回 0，与 fixedResetAtUnixMs 行为一致。
func pipelinePTTLToUnixMs(cmd *redis.DurationCmd) int64 {
	if cmd == nil {
		return 0
	}
	ttl, err := cmd.Result()
	if err != nil || ttl <= 0 {
		return 0
	}
	return time.Now().Add(ttl).UnixMilli()
}

// logSnapshotCmdError 把单条 SnapshotMany 命令的错误结构化记到 warn。
// 摘要 key 防止过长 key 占满日志；mode/concurrency 字段帮助排查批次异常。
func logSnapshotCmdError(ctx context.Context, sk service.SnapshotKey, err error) {
	slog.WarnContext(ctx, "service quota limiter snapshot pipeline cmd failed",
		"key_summary", summarizeSnapshotKey(sk.Key),
		"mode", sk.Mode,
		"is_concurrency", sk.IsConcurrency,
		"error", err,
	)
}

// summarizeSnapshotKey 在日志里截断长 key，避免一次批量请求把日志撑爆。
// 完整 key 在 metrics / trace 里另行记录，这里只保留判别用的前缀。
func summarizeSnapshotKey(key string) string {
	const maxLen = 64
	if len(key) <= maxLen {
		return key
	}
	return key[:maxLen] + "...(" + strconv.Itoa(len(key)) + ")"
}
