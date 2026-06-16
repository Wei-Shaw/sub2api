package service

import (
	"math"

	pkgerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

func normalizeLimiters(input *ServiceQuotaRuleInput) error {
	if len(input.Limiters) == 0 {
		return pkgerrors.BadRequest("SERVICE_QUOTA_INVALID_LIMITERS", "at least one limiter is required")
	}
	seen := make(map[string]struct{}, len(input.Limiters))
	for i := range input.Limiters {
		l := &input.Limiters[i]
		if err := validateLimiterType(l, seen); err != nil {
			return err
		}
		seen[l.LimiterType] = struct{}{}
		if err := normalizeLimiterTokenComponents(l); err != nil {
			return err
		}
		normalizeLimiterCountOnArrival(l)
		normalizeLimiterValue(l)
	}
	return nil
}

// normalizeLimiterValue 对整数型 limiter（rpm/tpm/tpd/concurrency）做 LimitValue 取整，
// 截断浮点漂移让"1000000 来回往返不再变成 999999.999997"。
//
// daily_usd 是金额类型本身就允许小数，保持原值不动（项目 CLAUDE.md 强制金额走 decimal，
// 但当前 storage / API 还是 float64，等彻底改 string-based 时再统一处理）。
//
// 这是数据底线：前端 NumericInput integer prop 是 UX 屏障，但任何来源（API 直调 / 旧客户端 /
// 数据导入）传非整数都会被这里拉回整数，不让浮点污染落库。
//
// 用 math.Round（四舍五入）而非 math.Floor / math.Trunc：用户传 999.9 显然意图是 1000，
// 而不是 999；项目里 IEEE 754 浮点漂移的实际场景都是 ±1e-9 量级，round 的结果与意图一致。
func normalizeLimiterValue(l *ServiceQuotaLimiterInput) {
	if !ServiceQuotaLimiterTypeIsInteger(l.LimiterType) {
		return
	}
	l.LimitValue = math.Round(l.LimitValue)
}

// validateLimiterType 校验单个 limiter 的"硬性"字段：
//   - LimiterType 在合法枚举内
//   - LimitValue > 0（rpm/tpm/tpd/concurrency/daily_usd 都要求严格正数）
//   - WindowMode 合法（concurrency 强制为 fixed；空值默认 fixed）
//   - 同 rule 内 LimiterType 不重复（seen 集合由调用方维护与回写，避免 helper 改副作用）
//
// 拆出来让 normalizeLimiters 主循环只剩 "校验 → 标记 seen → normalize" 三段，
// 不再被 4 段 switch + if 撑成 ~30 行。WindowMode 的强制 fixed 写回 *l.WindowMode 是必要副作用，
// 保留在 helper 内（与原 inline 逻辑等价）。
func validateLimiterType(l *ServiceQuotaLimiterInput, seen map[string]struct{}) error {
	switch l.LimiterType {
	case ServiceQuotaLimiterRPM, ServiceQuotaLimiterTPM, ServiceQuotaLimiterTPD,
		ServiceQuotaLimiterDailyUSD, ServiceQuotaLimiterConcurrency:
	default:
		return pkgerrors.BadRequest("SERVICE_QUOTA_INVALID_LIMITER_TYPE", "invalid limiter_type").WithMetadata(map[string]string{
			"limiter_type": l.LimiterType,
		})
	}
	if l.LimitValue <= 0 {
		return pkgerrors.BadRequest("SERVICE_QUOTA_INVALID_LIMIT_VALUE", "limit_value must be > 0").WithMetadata(map[string]string{
			"limiter_type": l.LimiterType,
		})
	}
	if l.WindowMode == "" || l.LimiterType == ServiceQuotaLimiterConcurrency {
		l.WindowMode = ServiceQuotaWindowFixed
	}
	switch l.WindowMode {
	case ServiceQuotaWindowFixed, ServiceQuotaWindowRolling:
	default:
		return pkgerrors.BadRequest("SERVICE_QUOTA_INVALID_WINDOW_MODE", "invalid window_mode").WithMetadata(map[string]string{
			"window_mode": l.WindowMode,
		})
	}
	if _, dup := seen[l.LimiterType]; dup {
		return pkgerrors.BadRequest("SERVICE_QUOTA_DUPLICATE_LIMITER", "duplicate limiter_type in rule").WithMetadata(map[string]string{
			"limiter_type": l.LimiterType,
		})
	}
	return nil
}

// normalizeLimiterCountOnArrival 归一化 count_on_arrival：仅 RPM 保留传入值，
// 其他 limiter type 一律置 nil，避免管理员误填污染存储。
func normalizeLimiterCountOnArrival(l *ServiceQuotaLimiterInput) {
	if !ServiceQuotaLimiterTypeUsesCountOnArrival(l.LimiterType) {
		l.CountOnArrival = nil
	}
}

// normalizeLimiterTokenComponents 校验并归一化 token_components：
//   - TPM/TPD：至少 1 项；只允许 4 个合法值；空时填充默认值
//   - 其他类型：强制置 nil（DB 仍存默认值，但 API 层对外不暴露这个字段）
func normalizeLimiterTokenComponents(l *ServiceQuotaLimiterInput) error {
	if !ServiceQuotaLimiterTypeUsesTokenComponents(l.LimiterType) {
		l.TokenComponents = nil
		return nil
	}
	if len(l.TokenComponents) == 0 {
		l.TokenComponents = append([]string(nil), ServiceQuotaTokenComponentsDefault...)
		return nil
	}
	seen := make(map[string]struct{}, len(l.TokenComponents))
	out := make([]string, 0, len(l.TokenComponents))
	for _, c := range l.TokenComponents {
		if !IsValidServiceQuotaTokenComponent(c) {
			return pkgerrors.BadRequest("SERVICE_QUOTA_INVALID_TOKEN_COMPONENT", "invalid token_component").WithMetadata(map[string]string{
				"limiter_type":    l.LimiterType,
				"token_component": c,
			})
		}
		if _, dup := seen[c]; dup {
			continue
		}
		seen[c] = struct{}{}
		out = append(out, c)
	}
	l.TokenComponents = out
	return nil
}
