package service

import (
	"context"
	"fmt"
	"log/slog"
	"strconv"

	pkgerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

// snapshotRuleLimiterModes 取规则 update 前的 limiter_type → window_mode 映射。
// 通过 loadRulesWithCache 复用现有缓存，找不到该规则时返回 nil（视作"无旧 limiter"）。
func (s *serviceQuotaService) snapshotRuleLimiterModes(ctx context.Context, ruleID int64) map[string]string {
	if s == nil || s.repo == nil {
		return nil
	}
	rules := s.loadRulesWithCache(ctx)
	for _, r := range rules {
		if r == nil || r.ID != ruleID {
			continue
		}
		out := make(map[string]string, len(r.Limiters))
		for _, lim := range r.Limiters {
			out[lim.LimiterType] = lim.WindowMode
		}
		return out
	}
	return nil
}

// snapshotRuleEnabled 取规则 update 前的 enabled 字段。仿 snapshotRuleLimiterModes 风格，
// 通过 loadRulesWithCache 复用现有缓存；找不到该规则（首次创建场景不会调用 update，但作保护）
// 或读取失败时返回 nil，UpdateRule 内会把 nil 视作"无旧值，跳过 enabled 翻转处理"。
func (s *serviceQuotaService) snapshotRuleEnabled(ctx context.Context, ruleID int64) *bool {
	if s == nil || s.repo == nil {
		return nil
	}
	rules := s.loadRulesWithCache(ctx)
	for _, r := range rules {
		if r == nil || r.ID != ruleID {
			continue
		}
		v := r.Enabled
		return &v
	}
	return nil
}

// resetCountersForModeSwitches 对比新旧 limiter 的 window_mode，找出切换过的 limiter_type，
// 调 ResetPattern 清掉旧的 STRING / ZSET 残留，避免 WRONGTYPE 静默 fail-open。
//
// 同 limiter_type 在 input 中可能因校验环节降级 mode（例如 concurrency 强制为 fixed），
// 这种"被强制归一"的变化也算切换；这里直接按字符串比较，简洁且安全。
func (s *serviceQuotaService) resetCountersForModeSwitches(ctx context.Context, ruleID int64, oldModes map[string]string, newLimiters []ServiceQuotaLimiterInput) {
	if s == nil || s.limiter == nil || len(oldModes) == 0 {
		return
	}
	for _, lim := range newLimiters {
		oldMode, ok := oldModes[lim.LimiterType]
		if !ok || oldMode == lim.WindowMode {
			continue
		}
		limiterType := lim.LimiterType
		pattern := BuildServiceQuotaCounterKeyPattern(ruleID, &limiterType)
		if err := s.limiter.ResetPattern(ctx, pattern); err != nil {
			slog.Warn("service quota: reset counters on window_mode switch failed",
				"rule_id", ruleID,
				"limiter_type", lim.LimiterType,
				"old_mode", oldMode,
				"new_mode", lim.WindowMode,
				"pattern", pattern,
				"error", err,
			)
		}
	}
}

// resetAllCountersForRule 删除指定 rule 名下所有 counter key。规则被删除后调用。
//
// pattern 用 BuildServiceQuotaCounterKey 的前缀 svcquota:v2:<rule_id>:* —— 会命中
// path × limiter_type × scope_user 全部笛卡尔积。失败仅 warn，依赖 Redis TTL 兜底。
func (s *serviceQuotaService) resetAllCountersForRule(ctx context.Context, ruleID int64) {
	if s == nil || s.limiter == nil || ruleID <= 0 {
		return
	}
	pattern := BuildServiceQuotaCounterKeyPattern(ruleID, nil)
	if err := s.limiter.ResetPattern(ctx, pattern); err != nil {
		slog.Warn("service quota: reset counters on rule delete failed",
			"rule_id", ruleID,
			"pattern", pattern,
			"error", err,
		)
	}
}

// ResetCountersForUser 实现 ServiceQuotaService 接口：fire-and-forget 清理某用户的全部 counter key。
//
// pattern: "svcquota:v2:*:*:*:<user_id>"——永远不会误命中 shared key（target=shared，
// glob 不会让数字 user_id 匹配 "shared"）。复用 task #15 的 goAsync helper：
// 用 context.WithoutCancel 解除 admin 请求 cancel 联动 + 顶层 panic recover。
func (s *serviceQuotaService) ResetCountersForUser(ctx context.Context, userID int64) {
	if s == nil || s.limiter == nil || userID <= 0 {
		return
	}
	pattern := BuildServiceQuotaCounterKeyPatternForUser(userID)
	s.goAsync(ctx, "reset_counters_user_delete", userID, func(asyncCtx context.Context) {
		if err := s.limiter.ResetPattern(asyncCtx, pattern); err != nil {
			slog.Warn("service quota: reset counters on user delete failed",
				"user_id", userID,
				"pattern", pattern,
				"error", err,
			)
		}
	})
}

// SnapshotPathIDsByOwner 同步查 path_ids 用于"删 DB 前快照"。失败返回 error，调用方决定是否继续（不吞错避免静默漏清）。
func (s *serviceQuotaService) SnapshotPathIDsByOwner(ctx context.Context, owner string, ownerID int64) ([]int64, error) {
	if s == nil || s.repo == nil {
		return nil, nil
	}
	return s.repo.FetchPathIDsByOwner(ctx, owner, ownerID)
}

// ResetCountersForPaths 实现 ServiceQuotaService 接口：fire-and-forget 清理一批 path_id 的全部 counter key。
//
// 单条 path 一次 ResetPattern("svcquota:v2:*:<path_id>:*:*")；批量调用顺序无关，单条失败只 warn。
// 整批包在一个 goroutine 里减少调度开销（path 多时仍然顺序跑，频次低无所谓延迟）。
//
// 调用方约定：必须先快照 pathIDs 再删 DB——否则 FK CASCADE 删完 service_quota_paths
// 就查不到 path_ids 了。snapshot 拷贝由调用方在外部做（goroutine 接收的是值而非引用）。
func (s *serviceQuotaService) ResetCountersForPaths(ctx context.Context, pathIDs []int64) {
	if s == nil || s.limiter == nil || len(pathIDs) == 0 {
		return
	}
	pathIDsCopy := append([]int64(nil), pathIDs...)
	s.goAsync(ctx, "reset_counters_paths_delete", 0, func(asyncCtx context.Context) {
		for _, pathID := range pathIDsCopy {
			if pathID <= 0 {
				continue
			}
			pattern := BuildServiceQuotaCounterKeyPatternForPath(pathID)
			if err := s.limiter.ResetPattern(asyncCtx, pattern); err != nil {
				slog.Warn("service quota: reset counters on path-owner delete failed",
					"path_id", pathID,
					"pattern", pattern,
					"error", err,
				)
			}
		}
	})
}

// ResetLimiterCounter 用 BuildServiceQuotaCounterKey 重建 Redis key 后调 limiter.Reset。
//
// 不做规则有效性二次校验（前端传过来的就是当前监控页 row 上看到的 ID）；如果规则
// 已被删除/限流器已被去掉，DEL 一个不存在的 key 也是 no-op，幂等无副作用。
func (s *serviceQuotaService) ResetLimiterCounter(ctx context.Context, ruleID, pathID int64, limiterType string, scopeUserID *int64) error {
	if s == nil || s.limiter == nil {
		return pkgerrors.NotFound("SERVICE_QUOTA_UNAVAILABLE", "service quota limiter is not available")
	}
	if ruleID <= 0 || pathID <= 0 {
		return pkgerrors.BadRequest("SERVICE_QUOTA_INVALID_RESET_TARGET", "rule_id and path_id are required")
	}
	switch limiterType {
	case ServiceQuotaLimiterRPM, ServiceQuotaLimiterTPM, ServiceQuotaLimiterTPD,
		ServiceQuotaLimiterDailyUSD, ServiceQuotaLimiterConcurrency:
	default:
		return pkgerrors.BadRequest("SERVICE_QUOTA_INVALID_LIMITER_TYPE", "invalid limiter_type")
	}
	key := BuildServiceQuotaCounterKey(ruleID, pathID, limiterType, scopeUserID)
	if err := s.limiter.Reset(ctx, key); err != nil {
		return fmt.Errorf("service_quota: reset counter %s: %w", key, err)
	}
	return nil
}

// serviceQuotaCounterKeyTargetShared 是 counter key 中"共享计数器"的目标段。
// counter_mode=shared 与未指定 user 的旁路读取都用这个值，确保 counterKey 与
// BuildServiceQuotaCounterKey 对同一逻辑路径输出完全相同的 Redis key。
const serviceQuotaCounterKeyTargetShared = "shared"

// BuildServiceQuotaCounterKey 拼接 path × limiter 的 Redis 计数 key。
//
// 入参 scopeUserID == nil → target 段写 "shared"；非 nil → 写 *scopeUserID。
// 写路径（counterKey）内部根据 rule.CounterMode 判定要不要传 userID；只读路径
// （Snapshot 类）可以直接显式构造 key，无需感知 rule 结构。
//
// v2 前缀与历史 counterKey 保持一致，让监控/快照与限流读写命中同一计数器。
func BuildServiceQuotaCounterKey(ruleID, pathID int64, limiterType string, scopeUserID *int64) string {
	target := serviceQuotaCounterKeyTargetShared
	if scopeUserID != nil {
		target = strconv.FormatInt(*scopeUserID, 10)
	}
	return fmt.Sprintf("svcquota:v2:%d:%d:%s:%s", ruleID, pathID, limiterType, target)
}

// BuildServiceQuotaCounterKeyPattern 是与 BuildServiceQuotaCounterKey 配对的"批量删除"
// 模式构造器。limiterType == nil 时匹配该 rule 名下全部 limiter / path / scope_user
// 的笛卡尔积；非 nil 时只命中指定 limiter_type 的全部 path × scope_user。
//
// 单点公式：禁止其他文件再用 fmt.Sprintf 拼 svcquota:v2:... 这种字面量；改 key 结构时
// 只改这里 + BuildServiceQuotaCounterKey 即可保证 reset 与 read/write 路径同步。
func BuildServiceQuotaCounterKeyPattern(ruleID int64, limiterType *string) string {
	if limiterType == nil {
		return fmt.Sprintf("svcquota:v2:%d:*", ruleID)
	}
	return fmt.Sprintf("svcquota:v2:%d:*:%s:*", ruleID, *limiterType)
}

// BuildServiceQuotaCounterKeyPatternForUser 匹配某 user 在所有规则/path/limiter 下的 counter key。
//
// 用于 user 被删除时清理 counter_mode=user / per_user 写出的 user-scoped 计数：
// 实际 key 形态是 svcquota:v2:<rule>:<path>:<type>:<user_id>，所以 pattern 用
// "前 4 段全 wildcard + 末段 user_id" 即可命中所有规则的该用户计数器。
//
// 不会误命中 shared key（target=shared），因为 user_id 是十进制数，glob "shared" 永远不等于。
func BuildServiceQuotaCounterKeyPatternForUser(userID int64) string {
	return fmt.Sprintf("svcquota:v2:*:*:*:%d", userID)
}

// BuildServiceQuotaCounterKeyPatternForPath 匹配某 path 在所有规则/limiter/scope 下的 counter key。
//
// 用于 channel/group/account 被删除导致 path 行被 CASCADE 清掉时，配合预查到的 path_id
// 清理残留 Redis 计数：key 形态 svcquota:v2:<rule>:<path>:<type>:<scope>，
// 把 path_id 段定死、其他全 wildcard。
func BuildServiceQuotaCounterKeyPatternForPath(pathID int64) string {
	return fmt.Sprintf("svcquota:v2:*:%d:*:*", pathID)
}

// counterKey 返回 path × limiter 的独立计数 key。v2 前缀使旧 key 自然失效。
//
// 内部委托给 BuildServiceQuotaCounterKey：counter_mode=user/per_user 时把
// req.UserID 传进去，shared 时传 nil；保持与历史 key 完全一致。
func (s *serviceQuotaService) counterKey(req ServiceQuotaCheckRequest, rule *ServiceQuotaRule, path ServiceQuotaPathDef, lim ServiceQuotaLimiterDef) string {
	var scopeUserID *int64
	if rule.CounterMode == ServiceQuotaCounterModeUser || rule.CounterMode == ServiceQuotaCounterModePerUser {
		uid := req.UserID
		scopeUserID = &uid
	}
	return BuildServiceQuotaCounterKey(rule.ID, path.ID, lim.LimiterType, scopeUserID)
}
