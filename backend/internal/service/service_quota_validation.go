package service

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"

	pkgerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

// normalizeAndValidate 是规则写入前的总入口：先做字段级硬校验（validateRuleFields），
// 再做业务级校验（target_user_ids 实存、limiters / paths 链路一致性等）。
//
// 校验环节按"前端能否拦下"分两层：
//  1. 字段级（validateRuleFields）：纯字段规则，前端 form validator 也能拦——这里是 API 兜底
//  2. 业务级（normalizeLimiters / normalizePaths / validateTargetUserIDsExist）：需要查 DB / 跨字段判定
//
// limiter / path 维度的细节拆到 service_quota_validation_limiters.go / _paths.go，
// 主流程保持线性。
func (s *serviceQuotaService) normalizeAndValidate(ctx context.Context, input *ServiceQuotaRuleInput) error {
	if input == nil {
		return pkgerrors.BadRequest("SERVICE_QUOTA_INVALID_RULE", "invalid service quota rule")
	}
	if err := normalizeRuleHeader(input); err != nil {
		return err
	}
	// 先做字段级硬校验，一次性收集所有字段错误（前端按 metadata.fields JSON 渲染高亮）。
	// 字段错误 reason = SERVICE_QUOTA_VALIDATION_ERROR，与旧 SERVICE_QUOTA_INVALID_TARGET 等单字段错误码区分；
	// 旧错误码保留给"业务级校验"（链路 mismatch / 重复 path 等）。
	if err := validateRuleFields(input); err != nil {
		return err
	}
	if err := s.validateTargetUserIDsExist(ctx, input.TargetUserIDs); err != nil {
		return err
	}
	if err := normalizeLimiters(input); err != nil {
		return err
	}
	return s.normalizePaths(ctx, input)
}

// normalizeRuleHeader 归一化规则的"头部"字段（CounterMode / Enabled / TargetUserIDs）。
// 拆出来让 normalizeAndValidate 主流程保持线性 ≤30 行：Header 归一化属于"前置预处理"，
// 与后续字段/业务校验是独立子阶段，单独可测。
//
// CounterMode 非法时直接返回错误（这是字段级硬校验的唯一例外，因为它决定后续 TargetUserIDs
// 是否抹空，必须先于 validateRuleFields 处理）；其他字段做幂等归一（默认值填充 / 非 user
// 模式抹掉 TargetUserIDs）。
func normalizeRuleHeader(input *ServiceQuotaRuleInput) error {
	if input.CounterMode == "" {
		input.CounterMode = ServiceQuotaCounterModePerUser
	}
	switch input.CounterMode {
	case ServiceQuotaCounterModeUser, ServiceQuotaCounterModePerUser, ServiceQuotaCounterModeShared:
	default:
		return pkgerrors.BadRequest("SERVICE_QUOTA_INVALID_COUNTER_MODE", "invalid counter_mode")
	}
	if input.Enabled == nil {
		enabled := true
		input.Enabled = &enabled
	}
	input.TargetUserIDs = sanitizeTargetUserIDs(input.TargetUserIDs)
	if input.CounterMode != ServiceQuotaCounterModeUser {
		input.TargetUserIDs = nil
	}
	return nil
}

// validateTargetUserIDsExist 检查 target_user_ids 中每个 ID 在 users 表中（且未软删除）实际存在。
// 缺失任何 ID 时返回 BadRequest，并在 metadata.missing_ids 给出排序后的逗号分隔列表，
// 让前端可以高亮错误的用户输入；外部 checker 不可用时（s.users == nil）跳过校验。
//
// 调用前 ids 已经过 sanitizeTargetUserIDs 去重 + 去 <=0。
func (s *serviceQuotaService) validateTargetUserIDsExist(ctx context.Context, ids []int64) error {
	if s == nil || s.users == nil || len(ids) == 0 {
		return nil
	}
	exists, err := s.users.ExistsByIDs(ctx, ids)
	if err != nil {
		return fmt.Errorf("service_quota: validate target_user_ids: %w", err)
	}
	missing := make([]int64, 0)
	for _, id := range ids {
		if !exists[id] {
			missing = append(missing, id)
		}
	}
	if len(missing) == 0 {
		return nil
	}
	sort.Slice(missing, func(i, j int) bool { return missing[i] < missing[j] })
	parts := make([]string, len(missing))
	for i, id := range missing {
		parts[i] = strconv.FormatInt(id, 10)
	}
	return pkgerrors.BadRequest(
		"SERVICE_QUOTA_INVALID_TARGET_USERS",
		"some target_user_ids do not refer to existing users",
	).WithMetadata(map[string]string{
		"missing_ids": strings.Join(parts, ","),
	})
}

func sanitizeTargetUserIDs(ids []int64) []int64 {
	if len(ids) == 0 {
		return nil
	}
	seen := make(map[int64]struct{}, len(ids))
	out := make([]int64, 0, len(ids))
	for _, id := range ids {
		if id <= 0 {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	return out
}

func serviceQuotaTargetMatches(rule *ServiceQuotaRule, userID int64) bool {
	if rule.CounterMode != ServiceQuotaCounterModeUser {
		return true
	}
	for _, id := range rule.TargetUserIDs {
		if id == userID {
			return true
		}
	}
	return false
}
