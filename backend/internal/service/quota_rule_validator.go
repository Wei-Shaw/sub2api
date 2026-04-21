package service

import (
	"context"
	"errors"
	"fmt"
	"strconv"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

// normalizedRule 规则校验归一化结果
type normalizedRule struct {
	groupIDs   []int64
	dailyLimit float64
	period     string
}

// validateAndNormalizeRule 校验并归一化规则字段：
//   - group_ids 非空、正数、去重、升序
//   - daily_limit_usd > 0
//   - period 仅允许 daily（MVP 只支持每日）
//   - 分组存在 + 非订阅分组
//   - 与同用户其他规则 group_ids 不重叠（excludeRuleID 用于 Update 场景）
func (s *quotaService) validateAndNormalizeRule(
	ctx context.Context,
	userID, excludeRuleID int64,
	groupIDs []int64,
	dailyLimit float64,
	period string,
) (*normalizedRule, error) {
	if dailyLimit <= 0 {
		return nil, infraerrors.BadRequest("QUOTA_LIMIT_INVALID", "daily_limit_usd must be > 0")
	}
	if period == "" {
		period = QuotaPeriodDaily
	}
	if period != QuotaPeriodDaily {
		return nil, infraerrors.BadRequest("QUOTA_PERIOD_UNSUPPORTED", "only daily period is supported")
	}

	cleaned := normalizeGroupIDs(groupIDs)
	if len(cleaned) == 0 {
		return nil, infraerrors.BadRequest("QUOTA_GROUP_IDS_EMPTY", "group_ids must not be empty")
	}
	if err := s.checkGroupsValidForQuota(ctx, cleaned); err != nil {
		return nil, err
	}
	if err := s.checkGroupsNotOverlap(ctx, userID, excludeRuleID, cleaned); err != nil {
		return nil, err
	}
	return &normalizedRule{groupIDs: cleaned, dailyLimit: dailyLimit, period: period}, nil
}

// normalizeRuleForReplace 校验批量替换用的单条规则并归一化字段。
// 与 validateAndNormalizeRule 的区别：不检查与已有规则的重叠（批量替换会先 DELETE 旧规则，
// 故"库内重叠"天然不成立；批次内重叠由调用方 ReplaceUserRules 统一扫描）。
func (s *quotaService) normalizeRuleForReplace(ctx context.Context, input CreateRuleRequest) (CreateRuleRequest, error) {
	period := input.Period
	if period == "" {
		period = QuotaPeriodDaily
	}
	if period != QuotaPeriodDaily {
		return CreateRuleRequest{}, infraerrors.BadRequest("QUOTA_PERIOD_UNSUPPORTED", "only daily period is supported")
	}
	if input.DailyLimitUSD <= 0 {
		return CreateRuleRequest{}, infraerrors.BadRequest("QUOTA_LIMIT_INVALID", "daily_limit_usd must be > 0")
	}
	cleaned := normalizeGroupIDs(input.GroupIDs)
	if len(cleaned) == 0 {
		return CreateRuleRequest{}, infraerrors.BadRequest("QUOTA_GROUP_IDS_EMPTY", "group_ids must not be empty")
	}
	if err := s.checkGroupsValidForQuota(ctx, cleaned); err != nil {
		return CreateRuleRequest{}, err
	}
	return CreateRuleRequest{
		GroupIDs:      cleaned,
		DailyLimitUSD: input.DailyLimitUSD,
		Period:        period,
	}, nil
}

func (s *quotaService) checkGroupsValidForQuota(ctx context.Context, groupIDs []int64) error {
	for _, gid := range groupIDs {
		g, err := s.groupRepo.GetByIDLite(ctx, gid)
		if err != nil {
			if errors.Is(err, ErrGroupNotFound) {
				return ErrRuleGroupNotFound.WithMetadata(map[string]string{
					"group_id": strconv.FormatInt(gid, 10),
				})
			}
			return fmt.Errorf("quota load group %d: %w", gid, err)
		}
		if g.IsSubscriptionType() {
			return ErrRuleGroupSubscription.WithMetadata(map[string]string{
				"group_id": strconv.FormatInt(gid, 10),
			})
		}
	}
	return nil
}

func (s *quotaService) checkGroupsNotOverlap(ctx context.Context, userID, excludeRuleID int64, groupIDs []int64) error {
	existing, err := s.ruleRepo.ListByUser(ctx, userID)
	if err != nil {
		return fmt.Errorf("quota load rules for overlap check: %w", err)
	}
	set := make(map[int64]struct{}, len(groupIDs))
	for _, g := range groupIDs {
		set[g] = struct{}{}
	}
	for _, r := range existing {
		if r == nil || r.ID == excludeRuleID {
			continue
		}
		for _, g := range r.GroupIDs {
			if _, ok := set[g]; ok {
				return ErrRuleGroupsOverlap.WithMetadata(map[string]string{
					"group_id":         strconv.FormatInt(g, 10),
					"conflict_rule_id": strconv.FormatInt(r.ID, 10),
				})
			}
		}
	}
	return nil
}
