// Package platforms 提供平台标识字符串（"openai" / "Anthropic" / " gemini "
// 等）的统一规范化与比较工具。
//
// 在引入本包之前，channel-management 内部至少存在 3 种不同的"平台是否相等"
// 实现：
//
//   - service/channel_cache.go::isPlatformPricingMatch — 严格 ==，不 trim
//   - service/account_stats_pricing.go::isAccountStatsPlatformMatch — 任一
//     方为空字符串即视为通配
//   - internal/pricing/server.go::platformMatches — trim + lower 后 ==
//
// 三种语义并存导致跨平台同名模型在不同读路径上的命中行为不一致 —— 是定价漂
// 移与"为什么测试环境正确，生产却用错平台计费"类问题的根因。
//
// 决策: 选 **trim + lowercase + 严格相等** 作为单源语义。
//
//   - 平台标识本来就是机器值（"openai" / "anthropic"），大小写空格差异完全
//     是 caller 错误数据的痕迹，统一 normalize 是无歧义改进。
//   - "空字符串通配"是危险默认：调用方写 `Platform: ""`（往往是忘填）会被
//     静默扩展到所有平台，违反"显式优于隐式"。如果业务真的需要"任意平台"
//     的特殊语义（参见 account_stats_pricing 的 rule pricing），应在 **调用
//     方** 显式写 `if pricing.Platform == "" { return true }`，而不是让
//     本包通用函数承担这一特殊行为。
//
// 依赖范围: 不依赖业务包，可在 service / pricing / cache / handler 任意位
// 置 import。
package platforms

import "strings"

// Normalize 把平台标识规范化：去首尾空白后转小写。
//
// 对 "" / 仅含空白的输入返回 ""，调用方据此判断"未配置"。
func Normalize(s string) string {
	return strings.ToLower(strings.TrimSpace(s))
}

// Match 报告两个平台标识是否相等（normalized 后严格相等）。
//
// 注意: " " / "" / "OpenAI" / "openai" 之间会按 normalize 后比较。**不**
// 提供"空字符串通配"语义；如果调用方需要那种行为，必须在调用现场显式判断
// 空串。
func Match(a, b string) bool {
	return Normalize(a) == Normalize(b)
}

// MatchAny 报告 candidates 中是否存在任一与 target 相等的平台。空 candidates
// 永远返回 false。
func MatchAny(candidates []string, target string) bool {
	if len(candidates) == 0 {
		return false
	}
	t := Normalize(target)
	for _, c := range candidates {
		if Normalize(c) == t {
			return true
		}
	}
	return false
}
