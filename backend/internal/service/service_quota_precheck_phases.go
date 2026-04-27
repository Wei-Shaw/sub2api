package service

import (
	"context"
	"fmt"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/metrics"
)

// limiterPhase 把 PreCheckAcquire 三轮步骤的 (predicate, fn) 组成可枚举的 step，
// 让主流程的循环结构线性可读，预留新增 phase（例如 burst limiter）的扩展点。
type limiterPhase struct {
	predicate func(ServiceQuotaLimiterDef) bool
	fn        func(rule *ServiceQuotaRule, p ServiceQuotaPathDef, lim ServiceQuotaLimiterDef) error
}

func isConcurrencyLimiter(lim ServiceQuotaLimiterDef) bool {
	return lim.LimiterType == ServiceQuotaLimiterConcurrency
}

func isRPMLimiter(lim ServiceQuotaLimiterDef) bool {
	return lim.LimiterType == ServiceQuotaLimiterRPM
}

func isDeferredLimiter(lim ServiceQuotaLimiterDef) bool {
	k := lim.LimiterType
	return k == ServiceQuotaLimiterTPM || k == ServiceQuotaLimiterTPD || k == ServiceQuotaLimiterDailyUSD
}

// iterateLimiters 封装 PreCheckAcquire 三轮判定共有的 matched × paths × limiters 四层嵌套。
//
// predicate 决定本轮关心哪些 limiter（只有 predicate(lim) 为 true 时才调 fn）。
// fn 任意返回 error 立即终止整个迭代并向上冒泡（调用方负责释放已抢资源）。
//
// 这是纯遍历适配器，不持有 ctx / req 等运行时状态——这些都通过 fn 闭包透传，
// 保持 helper 与具体阶段语义解耦。
func iterateLimiters(matched []matchedQuotaRule, predicate func(ServiceQuotaLimiterDef) bool, fn func(rule *ServiceQuotaRule, p ServiceQuotaPathDef, lim ServiceQuotaLimiterDef) error) error {
	for _, m := range matched {
		for _, p := range m.paths {
			for _, lim := range m.rule.Limiters {
				if !predicate(lim) {
					continue
				}
				if err := fn(m.rule, p, lim); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func (s *serviceQuotaService) acquireConcurrency(ctx context.Context, req ServiceQuotaCheckRequest, rule *ServiceQuotaRule, path ServiceQuotaPathDef, lim ServiceQuotaLimiterDef, lease *ServiceQuotaLease) error {
	member := fmt.Sprintf("%d:%d", req.UserID, time.Now().UnixNano())
	key := s.counterKey(req, rule, path, lim)
	ok, err := s.limiter.Acquire(ctx, key, member, int64(lim.LimitValue))
	if err != nil {
		s.recordFailOpen(rule, lim, "acquire_concurrency", key, err)
		return nil
	}
	if !ok {
		recordPreCheckResult(rule, lim, metrics.ServiceQuotaPreCheckResultDenied)
		return serviceQuotaExceeded(rule, lim, 0)
	}
	recordPreCheckResult(rule, lim, metrics.ServiceQuotaPreCheckResultAllowed)
	prev := lease.Release
	lease.Release = func() {
		if prev != nil {
			prev()
		}
		if rerr := s.limiter.Release(context.Background(), key, member); rerr != nil {
			s.logLimiterFailure("release_concurrency", rule, lim, key, rerr)
		}
	}
	return nil
}

func (s *serviceQuotaService) checkRPM(ctx context.Context, req ServiceQuotaCheckRequest, rule *ServiceQuotaRule, path ServiceQuotaPathDef, lim ServiceQuotaLimiterDef) error {
	key := s.counterKey(req, rule, path, lim)
	if lim.ShouldIncrementOnArrival() {
		return s.checkRPMIncrement(ctx, rule, lim, key)
	}
	return s.checkRPMReadOnly(ctx, rule, lim, key)
}

// checkRPMIncrement 是 count_on_arrival=true 的旧"到达即计"路径：Increment(+1) → 判超限 → 失败回滚。
//
// fail-open: Increment 报错时，已经 +1 的计数会成为下次请求的 "幽灵 +1"。
// best-effort 反向 Increment(-1) 抵消：fixed window 走 IncrByFloat(-1) 干净回退；
// rolling window 通过追加一条 delta=-1 的 member 与原 +1 在 sumRollingMembers 求和时
// 相互抵消，等价于回滚。任何回退失败都忽略，不阻塞主流程。
func (s *serviceQuotaService) checkRPMIncrement(ctx context.Context, rule *ServiceQuotaRule, lim ServiceQuotaLimiterDef, key string) error {
	used, err := s.limiter.Increment(ctx, key, 1, time.Minute, lim.WindowMode)
	if err != nil {
		_, _ = s.limiter.Increment(ctx, key, -1, time.Minute, lim.WindowMode)
		s.recordFailOpen(rule, lim, "check_rpm", key, err)
		return nil
	}
	if used > lim.LimitValue {
		recordPreCheckResult(rule, lim, metrics.ServiceQuotaPreCheckResultDenied)
		return serviceQuotaExceeded(rule, lim, used)
	}
	recordPreCheckResult(rule, lim, metrics.ServiceQuotaPreCheckResultAllowed)
	return nil
}

// checkRPMReadOnly 是 count_on_arrival=false 的"成功才计"路径：只读当前计数判超限，不写入。
// 计数会在 Record 阶段（请求成功落账后）由 serviceQuotaRecordDelta 触发 +1。
//
// 与 checkDeferredBatch 的区别：
//   - checkDeferredBatch 用 used >= LimitValue 判超限（TPM/TPD/daily_usd 是后置 token/usd 累加，
//     此处只读"达到上限"即拒绝下一次请求）
//   - checkRPMReadOnly 用 used >= LimitValue：当前请求若被允许则成功后会再 +1，与到达即计的
//     "used > LimitValue" 拒绝点严格等价（已计 N 次，下次到达让 N+1 > limit 即拒）
func (s *serviceQuotaService) checkRPMReadOnly(ctx context.Context, rule *ServiceQuotaRule, lim ServiceQuotaLimiterDef, key string) error {
	used, err := s.limiter.Current(ctx, key, time.Minute, lim.WindowMode)
	if err != nil {
		s.recordFailOpen(rule, lim, "check_rpm_readonly", key, err)
		return nil
	}
	if used >= lim.LimitValue {
		recordPreCheckResult(rule, lim, metrics.ServiceQuotaPreCheckResultDenied)
		return serviceQuotaExceeded(rule, lim, used)
	}
	recordPreCheckResult(rule, lim, metrics.ServiceQuotaPreCheckResultAllowed)
	return nil
}

// deferredCheckEntry 把 deferred 轮一次批读所需的 (rule, path, limiter, key, snapshotKey) 元组打包，
// 让 dispatch 与 evaluate 两阶段共享同一索引，下标 i 对应 SnapshotMany 返回切片的第 i 个元素。
type deferredCheckEntry struct {
	rule        *ServiceQuotaRule
	path        ServiceQuotaPathDef
	limiter     ServiceQuotaLimiterDef
	key         string
	snapshotKey SnapshotKey
}

// checkDeferredBatch 是 PreCheckAcquire 第三轮的批量实现：
// 把所有 (matched × paths × deferred-limiters) 收集成一个 SnapshotKey 切片，
// 一次 SnapshotMany 把全部当前用量拿回来，再逐个判超限。
//
// 与逐个 checkDeferred 的等价性：
//   - 命中超限走同一个 serviceQuotaExceeded（错误码、metadata 完全一致）
//   - SnapshotMany 单条失败降级为 {0, false} 并记 warn —— 等价于 Current 失败时的 fail-open
//     （recordFailOpen 在原路径会增计 metrics；批量路径下这部分降级在 limiter 层日志已记录，
//     这里不再重复打 metrics 避免双计，与"批量 read fail-open"语义一致）
//   - 短路返回首个超限错误，与 iterateLimiters 主循环短路语义保持一致
//
// 性能：N 个 deferred limiter 从 N 次 RTT 降到 1 次（pipeline）。matched / paths / limiters
// 数量通常不大（< 50），列表预分配 cap=16 即可覆盖绝大多数场景。
func (s *serviceQuotaService) checkDeferredBatch(ctx context.Context, req ServiceQuotaCheckRequest, matched []matchedQuotaRule) error {
	entries := s.collectDeferredEntries(req, matched)
	if len(entries) == 0 {
		return nil
	}
	keys := make([]SnapshotKey, len(entries))
	for i, e := range entries {
		keys[i] = e.snapshotKey
	}
	snaps, err := s.limiter.SnapshotMany(ctx, keys)
	if err != nil {
		// SnapshotMany 当前实现总返回 nil err（单条降级），保留这条防御让接口实现切换时不破语义。
		for _, e := range entries {
			s.recordFailOpen(e.rule, e.limiter, "check_deferred_batch", e.key, err)
		}
		return nil
	}
	for i, e := range entries {
		used := snaps[i].Current
		if used >= e.limiter.LimitValue {
			recordPreCheckResult(e.rule, e.limiter, metrics.ServiceQuotaPreCheckResultDenied)
			return serviceQuotaExceeded(e.rule, e.limiter, used)
		}
		recordPreCheckResult(e.rule, e.limiter, metrics.ServiceQuotaPreCheckResultAllowed)
	}
	return nil
}

// collectDeferredEntries 展开 matched × paths × deferred-limiters 笛卡尔积。
// 与 iterateLimiters 同样的遍历顺序，让批量路径与逐个路径在"是否命中"上完全等价。
func (s *serviceQuotaService) collectDeferredEntries(req ServiceQuotaCheckRequest, matched []matchedQuotaRule) []deferredCheckEntry {
	out := make([]deferredCheckEntry, 0, 16)
	for _, m := range matched {
		for _, p := range m.paths {
			for _, lim := range m.rule.Limiters {
				if !isDeferredLimiter(lim) {
					continue
				}
				key := s.counterKey(req, m.rule, p, lim)
				out = append(out, deferredCheckEntry{
					rule:    m.rule,
					path:    p,
					limiter: lim,
					key:     key,
					snapshotKey: SnapshotKey{
						Key:    key,
						Window: serviceQuotaWindow(lim),
						Mode:   lim.WindowMode,
					},
				})
			}
		}
	}
	return out
}
