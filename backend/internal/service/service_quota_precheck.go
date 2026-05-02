package service

import (
	"context"
	"errors"
	"log/slog"
	"strconv"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/metrics"
)

// matchedQuotaRule 是 PreCheck/Record 用的中间态：把规则与本次请求实际命中的 path 列表绑在一起，
// 后续按 path × limiter 做笛卡尔展开。
type matchedQuotaRule struct {
	rule  *ServiceQuotaRule
	paths []ServiceQuotaPathDef
}

// PreCheckSelect 仅做规则级过滤（enabled、counter_mode=user 时 target_user_ids 命中），
// 不做 path 匹配也不抢 concurrency / 增 rpm。返回 nil plan 表示"无规则需要参与本次请求"。
//
// 设计为只读、可在 caller 路由前调用——此时 ChannelID / AccountID 通常未知。
func (s *serviceQuotaService) PreCheckSelect(ctx context.Context, req ServiceQuotaCheckRequest) (*ServiceQuotaPreCheckPlan, error) {
	if s == nil || s.repo == nil || s.settings == nil || s.limiter == nil || req.UserID <= 0 {
		return nil, nil
	}
	if !s.isEnabled(ctx) {
		return nil, nil
	}
	rules := s.loadRulesWithCache(ctx)
	if len(rules) == 0 {
		return nil, nil
	}
	candidates := make([]*ServiceQuotaRule, 0, len(rules))
	for _, rule := range rules {
		if rule == nil || !rule.Enabled {
			continue
		}
		if !serviceQuotaTargetMatches(rule, req.UserID) {
			continue
		}
		candidates = append(candidates, rule)
	}
	if len(candidates) == 0 {
		return nil, nil
	}
	return &ServiceQuotaPreCheckPlan{
		Rules:      candidates,
		Req:        req,
		PreparedAt: time.Now(),
	}, nil
}

// PreCheckAcquire 在 caller 选定 channel + account 后调用，基于 plan 重新做 path 匹配
// （含 channel/account 维度），按 concurrency → rpm → deferred 三轮判定，命中超限返回错误，
// 否则返回已 acquire 的 *ServiceQuotaLease。
//
// plan == nil（PreCheckSelect 返回空）或匹配后无任何 path 命中时返回 (nil, nil)。
// 任何超限错误会先释放本轮已抢的 concurrency 槽位再返回，调用方无需补偿。
//
// PreCheckSelect → PreCheckAcquire 之间存在飞行窗口（caller 选 channel/account 可能 30s+），
// 这期间 admin 可能把规则 disable。plan.Rules 是阶段 A 缓存的快照指针，不会反映这段窗口
// 内的 enabled 变化——必须重读最新规则的 enabled 字段，避免对已 disable 的规则继续抢
// concurrency / 增 RPM。loadRulesWithCache 走 ServiceQuotaCache（admin Update/Delete 时
// reloadCache 会 Invalidate），命中即得最新 enabled，未命中走 singleflight + DB 重拉。
//
// 三轮循环为何不抽象成 phases interface：
//   - concurrency 持 lease（成功后注册 release callback，需要 *ServiceQuotaLease 上下文）
//   - RPM 自回滚（Increment 失败时 best-effort -1，副作用模型与 deferred 完全不同）
//   - deferred 只读（一次 SnapshotMany 批量拉用量）
//
// 三类副作用差异巨大，强行抽 phase[T] 反而增加 generics 复杂度与回调签名噪音；
// 共性仅限"四层嵌套（matched × paths × limiters × predicate）→ 调函数"，已由
// iterateLimiters helper 收敛，主流程保持线性可读。
func (s *serviceQuotaService) PreCheckAcquire(ctx context.Context, plan *ServiceQuotaPreCheckPlan, channelID, accountID int64) (*ServiceQuotaLease, error) {
	if plan == nil || len(plan.Rules) == 0 {
		return nil, nil
	}
	// Bug A：飞行窗口内 admin 可能关闭服务限额总开关。PreCheckSelect 已经过 isEnabled 闸门，
	// 但 PreCheckAcquire 历史上不复检，会继续抢 concurrency / 增 RPM。这里在入口立即短路，
	// 保持与 Record/PreCheckSelect 同样的尊重总开关行为。
	if !s.isEnabled(ctx) {
		return nil, nil
	}
	req := plan.Req
	req.ChannelID = channelID
	req.AccountID = accountID

	matched := s.matchedRulesForAcquire(ctx, plan, req)
	if len(matched) == 0 {
		return nil, nil
	}
	// matched rules 无 concurrency limiter 时，acquireConcurrency 不会被调用，
	// lease.Release 维持 nil。下游 BillingTicket.Close 已对 Release == nil 做 nil-safe。
	lease := &ServiceQuotaLease{}
	if err := s.runAcquirePhases(ctx, req, matched, lease); err != nil {
		lease.release()
		return nil, err
	}
	return lease, nil
}

// runAcquirePhases 执行 PreCheckAcquire 的三轮判定：
//
//	concurrency → RPM → deferred(tpm/tpd/daily_usd)
//
// 先抢并发额度避免后续判断时占用未释放。前两轮（concurrency / RPM）必须逐个：
// concurrency 走 Lua 原子脚本 + 副作用 lease，RPM CountOnArrival=true 走 Increment + 失败回滚——
// 两类操作都是写命令，pipeline 也不能合并。
//
// 第三轮（deferred）全是只读 Current → 批量 SnapshotMany 一次拉全部用量。
//
// 任何一轮短路返回 error，调用方负责释放 lease（与历史行为一致）。
func (s *serviceQuotaService) runAcquirePhases(ctx context.Context, req ServiceQuotaCheckRequest, matched []matchedQuotaRule, lease *ServiceQuotaLease) error {
	concurrencyStep := limiterPhase{predicate: isConcurrencyLimiter, fn: func(rule *ServiceQuotaRule, p ServiceQuotaPathDef, lim ServiceQuotaLimiterDef) error {
		return s.acquireConcurrency(ctx, req, rule, p, lim, lease)
	}}
	rpmStep := limiterPhase{predicate: isRPMLimiter, fn: func(rule *ServiceQuotaRule, p ServiceQuotaPathDef, lim ServiceQuotaLimiterDef) error {
		return s.checkRPM(ctx, req, rule, p, lim)
	}}
	for _, step := range []limiterPhase{concurrencyStep, rpmStep} {
		if err := iterateLimiters(matched, step.predicate, step.fn); err != nil {
			return err
		}
	}
	return s.checkDeferredBatch(ctx, req, matched)
}

// matchedRulesForAcquire 是 PreCheckAcquire 内"重读 enabled + path 匹配 + fallback 覆盖"的
// 抽取，让主函数体保持在 30 行内。返回空切片表示本次请求与所有规则都不相关。
func (s *serviceQuotaService) matchedRulesForAcquire(ctx context.Context, plan *ServiceQuotaPreCheckPlan, req ServiceQuotaCheckRequest) []matchedQuotaRule {
	enabledByID := s.latestEnabledByID(ctx, plan.Rules)
	matched := make([]matchedQuotaRule, 0, len(plan.Rules))
	for _, rule := range plan.Rules {
		if !enabledByID[rule.ID] {
			continue
		}
		paths := findMatchingPaths(rule, req)
		logPathMatchTrace("svcquota.acquire", rule, paths, req)
		if len(paths) == 0 {
			continue
		}
		matched = append(matched, matchedQuotaRule{rule: rule, paths: paths})
	}
	if len(matched) == 0 {
		return nil
	}
	return applyServiceQuotaFallbackOverride(matched)
}

// logPathMatchTrace 在 acquire / record 路径输出 path 匹配诊断日志。
//
// 暂时使用 INFO 级别（增加单请求日志量）以诊断生产 rule 4 不计数等"path 不命中"问题；
// 一旦真因定位下个 release 即应降级或删除（本函数集中一处便于摘除）。
//
// op 形如 "svcquota.acquire" / "svcquota.record"。paths 长度为 0 视为 miss，否则视为 match。
func logPathMatchTrace(op string, rule *ServiceQuotaRule, paths []ServiceQuotaPathDef, req ServiceQuotaCheckRequest) {
	if rule == nil {
		return
	}
	if len(paths) == 0 {
		slog.Info(op+".miss",
			"rule_id", rule.ID,
			"rule_name", rule.Name,
			"req_platform", req.Platform,
			"req_channel", req.ChannelID,
			"req_account", req.AccountID,
			"req_model", req.Model,
			"path_count", len(rule.Paths),
		)
		return
	}
	slog.Info(op+".match",
		"rule_id", rule.ID,
		"rule_name", rule.Name,
		"matched_paths", len(paths),
		"req_platform", req.Platform,
		"req_channel", req.ChannelID,
		"req_account", req.AccountID,
		"req_model", req.Model,
	)
}

// latestEnabledByID 重新走 ServiceQuotaCache（未命中时 singleflight 拉 DB）取最新规则集合，
// 仅返回 plan 中出现过的 rule_id → enabled 映射。
//
// PreCheckAcquire 的"飞行窗口 enabled 变更"修复依赖此 helper：admin 在 Update/Delete
// 时调 reloadCache 会 InvalidateRules，下次读必拿到 DB 最新值。读路径出错或返回空集合时，
// 退化为按 plan 快照中 rule.Enabled 判定（保守降级，与历史行为一致）。
func (s *serviceQuotaService) latestEnabledByID(ctx context.Context, planRules []*ServiceQuotaRule) map[int64]bool {
	out := make(map[int64]bool, len(planRules))
	if latest := s.loadRulesWithCache(ctx); len(latest) > 0 {
		current := make(map[int64]bool, len(latest))
		for _, r := range latest {
			if r != nil {
				current[r.ID] = r.Enabled
			}
		}
		for _, r := range planRules {
			if r == nil {
				continue
			}
			if v, ok := current[r.ID]; ok {
				out[r.ID] = v
				continue
			}
			// 规则在飞行窗口被删除：current 中已不存在 → 判 disabled，跳过抢占。
			out[r.ID] = false
		}
		return out
	}
	for _, r := range planRules {
		if r == nil {
			continue
		}
		out[r.ID] = r.Enabled
	}
	return out
}

func (s *serviceQuotaService) Record(ctx context.Context, req ServiceQuotaRecordRequest) {
	matched, ok := s.matchedRules(ctx, req.ServiceQuotaCheckRequest)
	if !ok || len(matched) == 0 {
		return
	}
	// 复用 PreCheckAcquire 同款 iterateLimiters：matched × paths × limiters 三层遍历用同一 helper，
	// 让"哪些 limiter 在 Record 阶段写计数"的语义统一收敛到 isRecordCountedLimiter；fn 永远返回 nil
	// （Record 单条失败只 log warn，不中止后续 limiter 的写入）。
	final := applyServiceQuotaFallbackOverride(matched)
	_ = iterateLimiters(final, isRecordCountedLimiter, func(rule *ServiceQuotaRule, p ServiceQuotaPathDef, lim ServiceQuotaLimiterDef) error {
		delta := serviceQuotaRecordDelta(lim, req)
		if delta <= 0 {
			// 例如 daily_usd cost=0 / RPM count_on_arrival=true 这两种"虽然类型路由到 Record 但本次无需写"的场景。
			return nil
		}
		key := s.counterKey(req.ServiceQuotaCheckRequest, rule, p, lim)
		if _, err := s.limiter.Increment(ctx, key, delta, serviceQuotaWindow(lim), lim.WindowMode); err != nil {
			s.logLimiterFailure("record", rule, lim, key, err)
		}
		return nil
	})
}

// isRecordCountedLimiter 判断 limiter 是否会被 Record 路径写计数。
//
// 直接复用 ServiceQuotaLimiterDef.ShouldIncrementOnRecord（task #1 引入的单点判定）：
//   - TPM/TPD/daily_usd 恒 true
//   - RPM 仅 CountOnArrival=false 时 true
//   - concurrency 恒 false
//
// 与 isConcurrencyLimiter / isRPMLimiter / isDeferredLimiter 同样作为 iterateLimiters 的 predicate
// 使用，让 PreCheckAcquire 的三轮判定与 Record 共用同一遍历骨架。
func isRecordCountedLimiter(lim ServiceQuotaLimiterDef) bool {
	return lim.ShouldIncrementOnRecord()
}

func (s *serviceQuotaService) matchedRules(ctx context.Context, req ServiceQuotaCheckRequest) ([]matchedQuotaRule, bool) {
	if s == nil || s.repo == nil || s.settings == nil || s.limiter == nil || req.UserID <= 0 {
		return nil, false
	}
	if !s.isEnabled(ctx) {
		return nil, false
	}
	rules := s.loadRulesWithCache(ctx)
	if rules == nil {
		return nil, false
	}
	out := make([]matchedQuotaRule, 0, len(rules))
	for _, rule := range rules {
		if !rule.Enabled || !serviceQuotaTargetMatches(rule, req.UserID) {
			continue
		}
		paths := findMatchingPaths(rule, req)
		logPathMatchTrace("svcquota.record", rule, paths, req)
		if len(paths) == 0 {
			continue
		}
		out = append(out, matchedQuotaRule{rule: rule, paths: paths})
	}
	return out, true
}

// recordFailOpen 在限流器报错降级时同时落日志 + 累加 metrics。
//
// reason 区分 timeout（context.DeadlineExceeded / context.Canceled）和 redis_error（其他），
// 让运维侧能区分网络超时与 redis 连接 / Lua 脚本异常。
func (s *serviceQuotaService) recordFailOpen(rule *ServiceQuotaRule, lim ServiceQuotaLimiterDef, op, key string, err error) {
	s.logLimiterFailure(op, rule, lim, key, err)
	reason := metrics.ServiceQuotaFailOpenReasonRedisError
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
		reason = metrics.ServiceQuotaFailOpenReasonTimeout
	}
	metrics.ServiceQuotaFailOpenTotal.WithLabelValues(lim.LimiterType, reason).Inc()
	recordPreCheckResult(rule, lim, metrics.ServiceQuotaPreCheckResultFailOpen)
}

// recordPreCheckResult 累加 (rule, limiter, result) 到 PreCheck 指标桶。
func recordPreCheckResult(rule *ServiceQuotaRule, lim ServiceQuotaLimiterDef, result string) {
	ruleID := "0"
	if rule != nil {
		ruleID = strconv.FormatInt(rule.ID, 10)
	}
	metrics.ServiceQuotaPreCheckTotal.WithLabelValues(ruleID, lim.LimiterType, result).Inc()
}

// Fail-open is intentional: when Redis is unreachable we allow the request
// through rather than return 5xx, but we must surface the degradation so the
// operator knows service quota rules are silently inert.
func (s *serviceQuotaService) logLimiterFailure(op string, rule *ServiceQuotaRule, lim ServiceQuotaLimiterDef, key string, err error) {
	slog.Warn("service quota limiter unavailable, rule not enforced",
		"op", op,
		"rule_id", rule.ID,
		"limiter_id", lim.ID,
		"limiter_type", lim.LimiterType,
		"window_mode", lim.WindowMode,
		"key", key,
		"error", err,
	)
}

func (l *ServiceQuotaLease) release() {
	if l != nil && l.Release != nil {
		l.Release()
	}
}
