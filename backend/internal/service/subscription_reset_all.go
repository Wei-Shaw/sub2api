package service

import (
	"context"
	"time"

	"entgo.io/ent/dialect/sql"
	"github.com/Wei-Shaw/sub2api/ent/predicate"
	"github.com/Wei-Shaw/sub2api/ent/usersubscription"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

// 全站一键重置订阅额度。
//
// 现有批量入口（BulkAction / BulkAssign）都要求显式列出 ID，且单次上限
// MaxBulkSubscriptionActions（100）。活跃订阅超过该数量时，「把全站额度重置
// 一遍」无法表达 —— 只能分批收集 ID 反复提交，中途没有进度、失败也没有断点。
//
// 范围口径：有效订阅（status=active 且尚未到期、未软删），且**已勾选窗口**中
// 至少一个窗口有用量。判「已勾选窗口有用量」而非「任一窗口有用量」，是为了让
// 「将影响 N 个」与实际清零数一一对应；已清零的订阅不再命中口径，故本操作
// 天然幂等、可中断续跑。
//
// 执行形态：预估（只读）→ 分批执行（每批 ≤ MaxBulkSubscriptionActions）→
// 由调用方驱动进度。

// ResetAllQuotaPreview 返回按当前勾选窗口口径统计的订阅数（只读预估）。
func (s *SubscriptionService) ResetAllQuotaPreview(ctx context.Context, daily, weekly, monthly bool) (int, error) {
	if !daily && !weekly && !monthly {
		return 0, ErrInvalidInput
	}
	return s.entClient.UserSubscription.Query().
		Where(resetAllQuotaScopePredicates(daily, weekly, monthly)...).
		Count(ctx)
}

// ResetAllQuotaResult 一批执行的结果。
type ResetAllQuotaResult struct {
	Processed    int               `json:"processed"`
	Remaining    int               `json:"remaining"`
	SuccessCount int               `json:"success_count"`
	FailedCount  int               `json:"failed_count"`
	Failures     []ResetAllFailure `json:"failures"`
}

// ResetAllFailure 单条失败明细。失败项必须可查，不得静默吞掉。
type ResetAllFailure struct {
	SubscriptionID int64  `json:"subscription_id"`
	Error          string `json:"error"`
}

// ResetAllQuotaBatch 处理一批符合口径的订阅（最多 limit 条），返回本批结果与剩余数量。
func (s *SubscriptionService) ResetAllQuotaBatch(ctx context.Context, daily, weekly, monthly bool, limit int) (*ResetAllQuotaResult, error) {
	if !daily && !weekly && !monthly {
		return nil, ErrInvalidInput
	}
	if limit <= 0 || limit > MaxBulkSubscriptionActions {
		limit = MaxBulkSubscriptionActions
	}
	ids, err := s.entClient.UserSubscription.Query().
		Where(resetAllQuotaScopePredicates(daily, weekly, monthly)...).
		Order(usersubscription.ByID(sql.OrderAsc())).
		Limit(limit).
		IDs(ctx)
	if err != nil {
		return nil, err
	}
	result := s.resetAllQuotaByIDs(ctx, ids, daily, weekly, monthly)
	// 重新统计剩余（已清零的订阅不再命中口径，无需额外游标）。
	remaining, err := s.ResetAllQuotaPreview(ctx, daily, weekly, monthly)
	if err != nil {
		return nil, err
	}
	result.Remaining = remaining
	return result, nil
}

// ResetAllQuotaBatchByIDs 对显式 ID 集合执行重置（供调用方「重试失败项」使用）。
func (s *SubscriptionService) ResetAllQuotaBatchByIDs(ctx context.Context, ids []int64, daily, weekly, monthly bool) (*ResetAllQuotaResult, error) {
	if !daily && !weekly && !monthly {
		return nil, ErrInvalidInput
	}
	if len(ids) == 0 {
		return nil, infraerrors.BadRequest("INVALID_INPUT", "subscription_ids must not be empty")
	}
	if len(ids) > MaxBulkSubscriptionActions {
		return nil, infraerrors.BadRequest("INVALID_BULK_SUBSCRIPTIONS", "subscription_ids must contain at most 100 IDs")
	}
	return s.resetAllQuotaByIDs(ctx, ids, daily, weekly, monthly), nil
}

// resetAllQuotaByIDs 逐条执行重置并汇总结果。失败项记录明细，不中断整批。
func (s *SubscriptionService) resetAllQuotaByIDs(ctx context.Context, ids []int64, daily, weekly, monthly bool) *ResetAllQuotaResult {
	result := &ResetAllQuotaResult{Processed: len(ids), Failures: make([]ResetAllFailure, 0)}
	for _, id := range ids {
		if _, err := s.AdminResetQuota(ctx, id, daily, weekly, monthly); err != nil {
			result.FailedCount++
			if len(result.Failures) < 100 {
				result.Failures = append(result.Failures, ResetAllFailure{SubscriptionID: id, Error: err.Error()})
			}
			continue
		}
		result.SuccessCount++
	}
	return result
}

// resetAllQuotaScopePredicates 组装「有效订阅 + 所选窗口中至少一个窗口有用量」的
// 范围谓词。有效口径与列表接口 status=active 分支一致：status=active 且尚未到期。
func resetAllQuotaScopePredicates(daily, weekly, monthly bool) []predicate.UserSubscription {
	base := []predicate.UserSubscription{
		usersubscription.StatusEQ(SubscriptionStatusActive),
		usersubscription.ExpiresAtGT(time.Now()),
	}
	windowPreds := make([]predicate.UserSubscription, 0, 3)
	if daily {
		windowPreds = append(windowPreds, usersubscription.DailyUsageUsdGT(0))
	}
	if weekly {
		windowPreds = append(windowPreds, usersubscription.WeeklyUsageUsdGT(0))
	}
	if monthly {
		windowPreds = append(windowPreds, usersubscription.MonthlyUsageUsdGT(0))
	}
	return append(base, usersubscription.Or(windowPreds...))
}
