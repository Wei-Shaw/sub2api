package service

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/metrics"
)

// snapshotForAccountsTimeout 是 SnapshotForAccounts 内部 SnapshotMany 的最大耗时。
//
// 调度热路径不能被慢 Redis 拖累——预过滤本身只是 quality-of-service 改进，超时则 fail-open，
// 错放交给后续 Consume 阶段（PreCheckAcquire）兜底。
const snapshotForAccountsTimeout = 200 * time.Millisecond

// snapshotForAccountsEntry 把单条 (account, rule, path, limiter) 元组及其 SnapshotKey
// 在 batch 中的下标拍平，便于一次 SnapshotMany 后按 used >= limit 反向标记账号。
type snapshotForAccountsEntry struct {
	accountID   int64
	scopeKind   string
	rule        *ServiceQuotaRule
	limiter     ServiceQuotaLimiterDef
	limit       float64
	snapshotKey SnapshotKey
}

// SnapshotForAccounts 实现 ServiceQuotaService 接口：批量预测候选账号被 service_quota 阻塞的情况。
//
// 三层 short-circuit + fail-open 见接口注释；本实现只识别 path scope = account / channel
// 的命中，user / group / platform 命中由 Consume 阶段返回 429。
func (s *serviceQuotaService) SnapshotForAccounts(ctx context.Context, base ServiceQuotaCheckRequest, plan *ServiceQuotaPreCheckPlan, candidates []int64) (map[int64]*ServiceQuotaPredictedBlock, error) {
	if !s.shouldRunSnapshotForAccounts(ctx, plan, candidates) {
		return nil, nil
	}
	entries := s.collectSnapshotForAccountsEntries(base, plan, candidates)
	if len(entries) == 0 {
		return nil, nil
	}
	keys := make([]SnapshotKey, len(entries))
	for i, e := range entries {
		keys[i] = e.snapshotKey
	}
	subCtx, cancel := context.WithTimeout(ctx, snapshotForAccountsTimeout)
	defer cancel()
	snaps, err := s.limiter.SnapshotMany(subCtx, keys)
	if err != nil {
		s.logSnapshotForAccountsFailOpen(err)
		return nil, nil
	}
	return reduceSnapshotForAccounts(entries, snaps), nil
}

// shouldRunSnapshotForAccounts 包裹三层 short-circuit：feature disabled / plan 空 / candidates 空。
//
// 抽到 helper 让主流程保持线性，避免主函数同时持有"三个判断 + entries 收集 + Redis 调用"
// 超过 30 行限制。
func (s *serviceQuotaService) shouldRunSnapshotForAccounts(ctx context.Context, plan *ServiceQuotaPreCheckPlan, candidates []int64) bool {
	if s == nil || s.limiter == nil {
		return false
	}
	if !s.isEnabled(ctx) {
		return false
	}
	if plan == nil || len(plan.Rules) == 0 {
		return false
	}
	if len(candidates) == 0 {
		return false
	}
	return true
}

// collectSnapshotForAccountsEntries 把 candidates × matched-paths × limiters 笛卡尔展开成
// 一个待 batch 读取的 SnapshotKey 列表。
//
// 每个 candidate 临时改写 base.AccountID 后调用 findMatchingPaths 复用 path 匹配（含 model_pattern）；
// 仅保留 scope = account / channel 的 path（其他 scope 不进结果）。
//
// concurrency limiter 也一并入 batch —— 单次 SnapshotMany 已经覆盖，零额外成本。
func (s *serviceQuotaService) collectSnapshotForAccountsEntries(base ServiceQuotaCheckRequest, plan *ServiceQuotaPreCheckPlan, candidates []int64) []snapshotForAccountsEntry {
	out := make([]snapshotForAccountsEntry, 0, len(candidates)*2)
	for _, accountID := range candidates {
		req := base
		req.AccountID = accountID
		matched := matchedRulesForCandidate(plan.Rules, req)
		if len(matched) == 0 {
			continue
		}
		out = appendEntriesForAccount(out, s, req, accountID, matched)
	}
	return out
}

// matchedRulesForCandidate 对单个候选账号做 path 匹配（含 model_pattern / channel_id），
// 复用 findMatchingPaths 单点实现避免漂移；空命中直接返回 nil 让 caller 跳过。
func matchedRulesForCandidate(rules []*ServiceQuotaRule, req ServiceQuotaCheckRequest) []matchedQuotaRule {
	matched := make([]matchedQuotaRule, 0, len(rules))
	for _, rule := range rules {
		if rule == nil || !rule.Enabled {
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
	// 复用 fallback 覆盖语义，与 PreCheckAcquire 主路径完全一致——避免出现"只有 fallback 命中
	// 调度阶段就剔除"的过于严格行为。
	return applyServiceQuotaFallbackOverride(matched)
}

// appendEntriesForAccount 在 entries 末尾追加单个账号在所有命中 path × limiter 上的 batch 项。
//
// scope 判定走 serviceQuotaPathScopeKind（铁律：AccountID 优先级最高），仅 account / channel
// 命中才入 batch；concurrency limiter 走 IsConcurrency=true 的 SnapshotKey 由底层 limiter 适配。
func appendEntriesForAccount(out []snapshotForAccountsEntry, s *serviceQuotaService, req ServiceQuotaCheckRequest, accountID int64, matched []matchedQuotaRule) []snapshotForAccountsEntry {
	for _, m := range matched {
		for _, p := range m.paths {
			scope := serviceQuotaPathScopeKind(p)
			if scope != ServiceQuotaPredictedBlockScopeAccount && scope != ServiceQuotaPredictedBlockScopeChannel {
				continue
			}
			for _, lim := range m.rule.Limiters {
				key := s.counterKey(req, m.rule, p, lim)
				out = append(out, snapshotForAccountsEntry{
					accountID:   accountID,
					scopeKind:   scope,
					rule:        m.rule,
					limiter:     lim,
					limit:       lim.LimitValue,
					snapshotKey: snapshotKeyForLimiter(key, lim),
				})
			}
		}
	}
	return out
}

// snapshotKeyForLimiter 把单个 limiter 的 (key, type) 转换成 batch 用 SnapshotKey。
// concurrency 走 ZSET 模式（IsConcurrency=true），其他类型走 fixed/rolling 模式。
// 复用 serviceQuotaWindow 单点实现，避免 window 计算漂移。
func snapshotKeyForLimiter(key string, lim ServiceQuotaLimiterDef) SnapshotKey {
	if lim.LimiterType == ServiceQuotaLimiterConcurrency {
		return SnapshotKey{Key: key, IsConcurrency: true}
	}
	return SnapshotKey{
		Key:    key,
		Window: serviceQuotaWindow(lim),
		Mode:   lim.WindowMode,
	}
}

// reduceSnapshotForAccounts 把 entries × snaps 一一对位规约为 accountID -> 首个命中描述。
//
// 单账号被多条 (rule, limiter) 同时阻塞时只保留第一条（信息足够让调度跳过该账号，
// 没必要为每个账号收集全部命中规则）；map 不保留任何内部 path / limiter 数组指针，
// 让结果跨 goroutine 共享时不带竞态隐患。
func reduceSnapshotForAccounts(entries []snapshotForAccountsEntry, snaps []LimiterSnapshot) map[int64]*ServiceQuotaPredictedBlock {
	if len(entries) == 0 || len(snaps) != len(entries) {
		return nil
	}
	out := make(map[int64]*ServiceQuotaPredictedBlock)
	for i, e := range entries {
		if _, already := out[e.accountID]; already {
			continue
		}
		used := snaps[i].Current
		if used < e.limit {
			continue
		}
		out[e.accountID] = &ServiceQuotaPredictedBlock{
			RuleID:      e.rule.ID,
			LimiterType: e.limiter.LimiterType,
			ScopeKind:   e.scopeKind,
			Used:        used,
			Limit:       e.limit,
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// logSnapshotForAccountsFailOpen 在 Redis 故障时既写日志也累加 metric。
//
// 复用现有 ServiceQuotaFailOpenTotal（label=limiter_type 用 "_batch" 占位标识批量路径），
// 不引入专属 metric——调度层 fail-open 与 PreCheck fail-open 是同一类降级，运维统一观测。
func (s *serviceQuotaService) logSnapshotForAccountsFailOpen(err error) {
	reason := metrics.ServiceQuotaFailOpenReasonRedisError
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
		reason = metrics.ServiceQuotaFailOpenReasonTimeout
	}
	metrics.ServiceQuotaFailOpenTotal.WithLabelValues("snapshot_for_accounts", reason).Inc()
	slog.Warn("service_quota: SnapshotForAccounts fail-open, scheduling proceeds without prefilter",
		"error", err,
		"reason", reason,
	)
}
