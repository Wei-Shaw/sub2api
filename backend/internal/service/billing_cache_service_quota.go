package service

// 用户每日配额（feature issue #1750）相关的 BillingCacheService 方法拆分文件。
// 主 struct 与 cacheWriteIncrQuotaUsage 任务派发在 billing_cache_service.go；
// 本文件只承载配额特定的前置检查、异步累加、QuotaService 注入。

import (
	"context"
	"log/slog"

	"github.com/Wei-Shaw/sub2api/internal/pkg/timezone"
)

// quotaLogComponent 是 quota 相关异步累加日志统一的 component 标签。
// 集中一处避免各方法重复写字符串字面量，便于未来调整日志分类。
const quotaLogComponent = "service.quota"

// quotaServiceBox 包装 QuotaService 接口以便配合 atomic.Pointer 使用。
// Go 的 atomic.Pointer 需要具名指针类型；接口类型不能直接存入，
// 因此用 struct 包一层，Store 时传 *quotaServiceBox，Load 返回 *quotaServiceBox。
type quotaServiceBox struct{ svc QuotaService }

// SetQuotaService 注入 QuotaService（仅在服务启动期调用一次，线程安全）。
// 契约见 docs/DAILY_QUOTA_CONTRACT.md §6：BillingCacheService 依赖 QuotaService。
//
// 用 atomic.Pointer 存储是因为存在构造序依赖循环：BillingCacheService 先被其他 service 依赖，
// 之后才创建 QuotaService（QuotaService 内部依赖 BillingCache 读用量）。若改成构造函数注入
// 需要破除环，成本较高，目前选择 init-time setter + 原子读写保证 race detector 清洁。
//
// 防御：传 nil 直接忽略，避免误传把已注入的服务静默清空导致热路径全放行。
func (s *BillingCacheService) SetQuotaService(qs QuotaService) {
	if qs == nil {
		return
	}
	s.quotaServicePtr.Store(&quotaServiceBox{svc: qs})
}

// quotaService 读取当前注入的 QuotaService；未注入时返回 nil。
// 所有热路径（checkQuotaEligibility / processQuotaUsageTask / QueueIncrQuotaUsage）
// 都走这个 getter，避免直接访问 atomic.Pointer 字段。
func (s *BillingCacheService) quotaService() QuotaService {
	box := s.quotaServicePtr.Load()
	if box == nil {
		return nil
	}
	return box.svc
}

// QueueIncrQuotaUsage 异步累加用户今日配额用量。
//
// 用在 finalizePostUsageBilling 余额分支（订阅分支不累加，见契约 §10）。
// 队列满时直接丢弃（不同步回退），配额丢失不会导致超扣，容忍极小比例误差。
func (s *BillingCacheService) QueueIncrQuotaUsage(userID, groupID int64, amount float64) {
	if s.cache == nil || s.quotaService() == nil || userID <= 0 || amount <= 0 {
		return
	}
	s.enqueueCacheWrite(cacheWriteTask{
		kind:    cacheWriteIncrQuotaUsage,
		userID:  userID,
		groupID: groupID,
		amount:  amount,
	})
}

// processQuotaUsageTask 执行配额用量累加（total + 可选 rule）。
//
// ---- TOCTOU 设计取舍（与 checkQuotaEligibility 对称） ----
// 这里是 check-then-incr 流水线的 "incr" 半段，与 checkQuotaEligibility 的 "check"
// 非原子组合。高并发下，同一用户 N 个并发请求可能在 check 阶段都看到 "未超限"，
// 然后各自 incr 导致实际累计值短暂超过上限（最多 N × 单次费用）。
// 这是已知的设计取舍：权衡 Redis 原子 CAS 的实现成本与极小比例的超出容忍度，
// 选择异步累加 + 软上限。详见契约 §10 和 CLAUDE.md §11「fail open 策略」。
//
// ---- 失败策略 ----
// Redis/DB 任意读/写失败只记日志，不回源也不重试。配额是"执行力"不是"账本"，
// 丢失极小比例的累加对计费不影响（余额扣减走独立路径）。
func (s *BillingCacheService) processQuotaUsageTask(ctx context.Context, task cacheWriteTask) {
	if s.cache == nil || task.amount <= 0 {
		return
	}
	date := quotaDateKey(timezone.Now())
	if err := s.cache.IncrQuotaUsedTotal(ctx, task.userID, date, task.amount); err != nil {
		slog.Warn("incr quota used total failed",
			"component", quotaLogComponent,
			"user_id", task.userID,
			"date", date,
			"error", err,
		)
	}
	// 命中规则时还要累加规则用量（通过 QuotaService 匹配）
	qs := s.quotaService()
	if qs == nil || task.groupID <= 0 {
		return
	}
	resolved, err := qs.Resolve(ctx, task.userID)
	if err != nil || resolved == nil || !resolved.Enabled {
		return
	}
	rule := qs.MatchRule(resolved, task.groupID)
	if rule == nil {
		return
	}
	if err := s.cache.IncrQuotaUsedRule(ctx, task.userID, rule.ID, date, task.amount); err != nil {
		slog.Warn("incr quota used rule failed",
			"component", quotaLogComponent,
			"user_id", task.userID,
			"rule_id", rule.ID,
			"date", date,
			"error", err,
		)
	}
}

// checkQuotaEligibility 配额前置检查（余额分支内调用）。
//
// 不可拆分（~40 行 > CLAUDE.md §10 的 30 行阈值）：total 校验 与 rule 校验共享
// date / resetAt 上下文 + 相同的 fail-open 分支结构；拆成两个函数要么重复传递参数，
// 要么引入中间结构体承载共享状态，都比保留线性流水线更难读。
//
// ---- TOCTOU 设计取舍（与 processQuotaUsageTask 对称） ----
// 这是 check-then-incr 流水线的 "check" 半段：先读 Redis 当前用量与上限对比，
// 然后请求完成时由 processQuotaUsageTask 异步 "incr"。两段非原子。
// 高并发下，同一用户 N 个并发请求可能都在 check 阶段读到 "未超限"，然后各自累加
// 导致实际用量短暂超过上限（最多 N × 单次费用）。这是已知设计取舍：
// 权衡 Redis 原子 CAS 的实现成本与极小比例的超出容忍度，
// 选择软上限 + 异步累加。详见契约 §10 和 CLAUDE.md §11「fail open 策略」。
//
// ---- 失败安全（fail open） ----
// 任何 Resolve / Redis 读取错误都返回 nil 放行，
// 与 checkAPIKeyRateLimits "Don't block requests on DB errors" 对齐。
// 配额只是软限制，宁可短暂漏扣也不阻断付费请求。
func (s *BillingCacheService) checkQuotaEligibility(ctx context.Context, userID int64, group *Group) error {
	qs := s.quotaService()
	if qs == nil || userID <= 0 {
		return nil
	}
	resolved, err := qs.Resolve(ctx, userID)
	if err != nil || resolved == nil || !resolved.Enabled {
		return nil // fail open：解析失败/未启用都放行
	}

	date := quotaDateKey(timezone.Now())
	resetAt := nextQuotaResetTime()

	// 总限额校验
	if resolved.DailyLimit != nil && *resolved.DailyLimit > 0 {
		used, readErr := s.cache.GetQuotaUsedTotal(ctx, userID, date)
		if readErr != nil {
			return nil // fail open：读 Redis 失败放行
		}
		if used >= *resolved.DailyLimit {
			return QuotaExceededTotalError(*resolved.DailyLimit, used, resetAt)
		}
	}

	// 规则限额校验：仅在有 group 时执行
	if group == nil {
		return nil
	}
	rule := qs.MatchRule(resolved, group.ID)
	if rule == nil {
		return nil
	}
	used, readErr := s.cache.GetQuotaUsedRule(ctx, userID, rule.ID, date)
	if readErr != nil {
		return nil // fail open
	}
	if used >= rule.DailyLimitUSD {
		return QuotaExceededRuleError(rule.ID, rule.DailyLimitUSD, used, resetAt)
	}
	return nil
}
