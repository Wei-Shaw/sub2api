package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/metrics"
)
// matchedQuotaRule 是 PreCheck/Record 用的中间态：把规则与本次请求实际命中的 path 列表绑在一起，
// 后续按 path × limiter 做笛卡尔展开。
type matchedQuotaRule struct {
	rule  *ServiceQuotaRule
	paths []ServiceQuotaPathDef
}

// PreCheck 兼容旧 caller 的一阶段入口。
//
// 新代码请使用 BillingCacheService.PrepareBillingCheck（caller 路由前）+
// BillingTicket.Consume（选定 channel/account 后）的两阶段路径，参见
// service_quota_types.go 中 ServiceQuotaService.PreCheckSelect / PreCheckAcquire 的注释。
//
// 内部实现：直接复用 PreCheckSelect → PreCheckAcquire，二者共享同一份匹配/抢槽位逻辑。
func (s *serviceQuotaService) PreCheck(ctx context.Context, req ServiceQuotaCheckRequest) (*ServiceQuotaLease, error) {
	plan, err := s.PreCheckSelect(ctx, req)
	if err != nil {
		return nil, err
	}
	if plan == nil {
		return nil, nil
	}
	return s.PreCheckAcquire(ctx, plan, req.ChannelID, req.AccountID)
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
//   - deferred 只读（Current 即可，不写计数）
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

	// 初始化 no-op Release：当 matched rules 仅含 RPM/deferred 而无 concurrency limiter 时，
	// acquireConcurrency 不会被调用，Release 仍为 nil。下游 BillingTicket.Close() 直接调
	// t.lease.Release()（非 nil-safe），会 panic。给 Release 一个空 func，acquireConcurrency
	// 包装 prev 时会顺带调用一次 no-op，幂等无害。
	lease := &ServiceQuotaLease{Release: func() {}}
	// 三轮：concurrency → RPM → deferred(tpm/tpd/daily_usd)。先抢并发额度避免后续判断时占用未释放。
	steps := []limiterPhase{
		{predicate: isConcurrencyLimiter, fn: func(rule *ServiceQuotaRule, p ServiceQuotaPathDef, lim ServiceQuotaLimiterDef) error {
			return s.acquireConcurrency(ctx, req, rule, p, lim, lease)
		}},
		{predicate: isRPMLimiter, fn: func(rule *ServiceQuotaRule, p ServiceQuotaPathDef, lim ServiceQuotaLimiterDef) error {
			return s.checkRPM(ctx, req, rule, p, lim)
		}},
		{predicate: isDeferredLimiter, fn: func(rule *ServiceQuotaRule, p ServiceQuotaPathDef, lim ServiceQuotaLimiterDef) error {
			return s.checkDeferred(ctx, req, rule, p, lim)
		}},
	}
	for _, step := range steps {
		if err := iterateLimiters(matched, step.predicate, step.fn); err != nil {
			lease.release()
			return nil, err
		}
	}
	return lease, nil
}

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
	for _, m := range applyServiceQuotaFallbackOverride(matched) {
		for _, p := range m.paths {
			for _, lim := range m.rule.Limiters {
				delta := serviceQuotaRecordDelta(lim, req)
				if delta <= 0 {
					continue
				}
				key := s.counterKey(req.ServiceQuotaCheckRequest, m.rule, p, lim)
				if _, err := s.limiter.Increment(ctx, key, delta, serviceQuotaWindow(lim), lim.WindowMode); err != nil {
					s.logLimiterFailure("record", m.rule, lim, key, err)
				}
			}
		}
	}
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
		if len(paths) == 0 {
			continue
		}
		out = append(out, matchedQuotaRule{rule: rule, paths: paths})
	}
	return out, true
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
	used, err := s.limiter.Increment(ctx, key, 1, time.Minute, lim.WindowMode)
	if err != nil {
		// fail-open: Increment 报错时，已经 +1 的计数会成为下次请求的 “幽灵 +1”。
		// best-effort 反向 Increment(-1) 抵消：fixed window 走 IncrByFloat(-1) 干净回退；
		// rolling window 通过追加一条 delta=-1 的 member 与原 +1 在 sumRollingMembers 求和时
		// 相互抵消，等价于回滚。任何回退失败都忽略，不阻塞主流程。
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

func (s *serviceQuotaService) checkDeferred(ctx context.Context, req ServiceQuotaCheckRequest, rule *ServiceQuotaRule, path ServiceQuotaPathDef, lim ServiceQuotaLimiterDef) error {
	key := s.counterKey(req, rule, path, lim)
	used, err := s.limiter.Current(ctx, key, serviceQuotaWindow(lim), lim.WindowMode)
	if err != nil {
		s.recordFailOpen(rule, lim, "check_deferred", key, err)
		return nil
	}
	if used >= lim.LimitValue {
		recordPreCheckResult(rule, lim, metrics.ServiceQuotaPreCheckResultDenied)
		return serviceQuotaExceeded(rule, lim, used)
	}
	recordPreCheckResult(rule, lim, metrics.ServiceQuotaPreCheckResultAllowed)
	return nil
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

// applyServiceQuotaFallbackOverride 在限流器层级处理 is_fallback 语义：
// 同一 limiter_type 若有任意非 fallback 规则命中并配置了该类型限流器，则丢弃 fallback 规则中该类型的限流器
// （而不是整条 fallback 规则）。如果丢空了 fallback 规则的所有限流器，则该 fallback 规则整条被忽略。
func applyServiceQuotaFallbackOverride(matched []matchedQuotaRule) []matchedQuotaRule {
	covered := map[string]bool{}
	for _, m := range matched {
		if m.rule.IsFallback {
			continue
		}
		for _, lim := range m.rule.Limiters {
			covered[lim.LimiterType] = true
		}
	}
	out := make([]matchedQuotaRule, 0, len(matched))
	for _, m := range matched {
		if !m.rule.IsFallback {
			out = append(out, m)
			continue
		}
		kept := make([]ServiceQuotaLimiterDef, 0, len(m.rule.Limiters))
		for _, lim := range m.rule.Limiters {
			if covered[lim.LimiterType] {
				continue
			}
			kept = append(kept, lim)
		}
		if len(kept) == 0 {
			continue
		}
		clone := *m.rule
		clone.Limiters = kept
		out = append(out, matchedQuotaRule{rule: &clone, paths: m.paths})
	}
	return out
}


func serviceQuotaWindow(lim ServiceQuotaLimiterDef) time.Duration {
	switch lim.LimiterType {
	case ServiceQuotaLimiterRPM, ServiceQuotaLimiterTPM:
		return time.Minute
	case ServiceQuotaLimiterTPD, ServiceQuotaLimiterDailyUSD:
		return 24 * time.Hour
	default:
		return time.Minute
	}
}

// serviceQuotaRecordDelta 计算单个 limiter 在一次请求中要增加的计数。
//
// TPM/TPD：按 limiter.TokenComponents 求和——每个限流器可独立选择 input/output/cache_creation/cache_read 子集。
//   - 默认 {input, output, cache_creation}（DB DEFAULT + ServiceQuotaTokenComponentsDefault）
//   - TokenComponents 为空时退化为默认值，向前兼容老规则
//
// daily_usd：按金额；其他 limiter type（rpm/concurrency）走单独路径，不入 Record。
func serviceQuotaRecordDelta(lim ServiceQuotaLimiterDef, req ServiceQuotaRecordRequest) float64 {
	switch lim.LimiterType {
	case ServiceQuotaLimiterTPM, ServiceQuotaLimiterTPD:
		components := lim.TokenComponents
		if len(components) == 0 {
			components = ServiceQuotaTokenComponentsDefault
		}
		return float64(req.SumTokens(components))
	case ServiceQuotaLimiterDailyUSD:
		return req.Cost
	default:
		return 0
	}
}

// serviceQuotaExceeded 构造 429 拒绝错误。
//
// metadata 必须携带足够字段让前端在不发二次请求的前提下定位是哪条规则的哪个 limiter 超了：
//   - rule_id：规则 ID
//   - rule_name：规则可读名（缺省 fallback 为 "unnamed-{id}"）
//   - limiter_type：rpm/tpm/tpd/daily_usd/concurrency
//   - limiter_window：fixed/rolling/none（concurrency 没有窗口语义，统一返回 "none"）
//   - limit_value：阈值
//   - limit：阈值（保留旧 key 以兼容老前端）
//   - used：当前已用量
func serviceQuotaExceeded(rule *ServiceQuotaRule, lim ServiceQuotaLimiterDef, used float64) error {
	return ErrServiceQuotaExceeded.WithMetadata(map[string]string{
		"rule_id":        strconv.FormatInt(rule.ID, 10),
		"rule_name":      ruleDisplayName(rule),
		"limiter_type":   lim.LimiterType,
		"limiter_window": limiterWindowLabel(lim),
		"limit_value":    strconv.FormatFloat(lim.LimitValue, 'f', -1, 64),
		"limit":          strconv.FormatFloat(lim.LimitValue, 'f', -1, 64),
		"used":           strconv.FormatFloat(used, 'f', -1, 64),
	})
}

// ruleDisplayName 返回规则的展示名，rule.Name 为 nil 或空字符串时回退为 "unnamed-{id}"。
func ruleDisplayName(rule *ServiceQuotaRule) string {
	if rule == nil {
		return "unnamed-0"
	}
	if rule.Name != nil {
		if name := strings.TrimSpace(*rule.Name); name != "" {
			return name
		}
	}
	return "unnamed-" + strconv.FormatInt(rule.ID, 10)
}

// limiterWindowLabel 返回前端可消费的 window 标签：concurrency 统一为 "none"。
func limiterWindowLabel(lim ServiceQuotaLimiterDef) string {
	if lim.LimiterType == ServiceQuotaLimiterConcurrency {
		return "none"
	}
	if lim.WindowMode == "" {
		return ServiceQuotaWindowFixed
	}
	return lim.WindowMode
}
