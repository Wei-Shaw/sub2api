package service

import (
	"sort"
	"strconv"
	"time"
)

// QuotaExceededTotalError 构造超限错误（总限额）
func QuotaExceededTotalError(limit, used float64, resetAt time.Time) error {
	return ErrQuotaExceeded.WithMetadata(map[string]string{
		"scope":     QuotaScopeTotal,
		"limit_usd": formatQuotaAmount(limit),
		"used_usd":  formatQuotaAmount(used),
		"reset_at":  resetAt.Format(time.RFC3339),
	})
}

// QuotaExceededRuleError 构造超限错误（规则限额）
func QuotaExceededRuleError(ruleID int64, limit, used float64, resetAt time.Time) error {
	return ErrQuotaExceeded.WithMetadata(map[string]string{
		"scope":     QuotaScopeRule,
		"rule_id":   strconv.FormatInt(ruleID, 10),
		"limit_usd": formatQuotaAmount(limit),
		"used_usd":  formatQuotaAmount(used),
		"reset_at":  resetAt.Format(time.RFC3339),
	})
}

// formatQuotaAmount 与其它金额字段对齐，使用 decimal(20,8) 同精度字符串
func formatQuotaAmount(v float64) string {
	return strconv.FormatFloat(v, 'f', 8, 64)
}

// normalizeGroupIDs 去重 + 升序 + 去除非正数
func normalizeGroupIDs(in []int64) []int64 {
	seen := make(map[int64]struct{}, len(in))
	out := make([]int64, 0, len(in))
	for _, v := range in {
		if v <= 0 {
			continue
		}
		if _, ok := seen[v]; ok {
			continue
		}
		seen[v] = struct{}{}
		out = append(out, v)
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}
