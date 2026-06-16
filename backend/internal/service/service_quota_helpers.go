package service

import (
	"strconv"
	"strings"
	"time"
)

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
// daily_usd：按金额。
//
// RPM：仅当 CountOnArrival=false 时（"成功才计"语义）由 Record 阶段 +1；
// CountOnArrival=true 时已在 PreCheckAcquire/checkRPMIncrement 写入，Record 不再重复计数。
//
// concurrency：不通过 Record 计数（走 Acquire/Release ZSET 路径）。
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
	case ServiceQuotaLimiterRPM:
		if lim.ShouldIncrementOnRecord() {
			return 1
		}
		return 0
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
