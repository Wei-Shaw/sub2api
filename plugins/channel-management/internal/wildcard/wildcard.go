// Package wildcard 提供模型 / 模式字符串中尾部通配符 "*" 的统一拆分与匹配工具。
//
// 在引入本包之前，channel-management 插件里的多处实现各自手写
// `strings.HasSuffix(p, "*") + strings.TrimSuffix(p, "*")` 序列：
//
//   - service/channel_cache.go::expandPricingToCache / expandMappingToCache
//   - service/account_stats_pricing.go::findAccountStatsPricingForModel
//   - service/cache_writer.go::writeMapping
//   - service/channel_validation.go::toModelEntry
//   - internal/domain/channel_view.go::splitWildcardSuffix（vendored snapshot,
//     现在内部委托到本包，保持 byte-level fidelity 的同时单点收敛实现）
//
// 单源化让"通配符是什么"这件事只有一份定义，避免跨文件出现"是否大小写敏感、
// 是否允许中间出现 *"等语义漂移。
//
// 设计约束:
//
//   - 仅支持 **尾部** "*"。中间 / 前缀通配符不在本包语义内（业务上也未需要）。
//   - 大小写敏感。caller 应在传入前自行 strings.ToLower / strings.TrimSpace
//     做规范化；本包保持原样以便调用方自行决定保留 original case 用于显示。
//   - 不依赖业务包，可在 service / cache / pricing / handler 任意位置 import。
package wildcard

import "strings"

// Suffix 是模式中表示通配符的尾部标记。导出为常量，让调用方写
// `strings.TrimSuffix(p, wildcard.Suffix)` 时显式表达意图。
const Suffix = "*"

// SplitSuffix 将模式拆分为 (prefix, isWildcard)。
//
//	"claude-opus-*" → ("claude-opus-", true)
//	"claude-opus-4" → ("claude-opus-4", false)
//	"*"             → ("", true)
//	""              → ("", false)
//
// 返回的 prefix 保持原始大小写。空字符串视为非通配符（业务上"完全不写"
// 与"显式写 *"的语义不同，前者通常是 bug 而非通配意图）。
func SplitSuffix(pattern string) (prefix string, isWildcard bool) {
	if !strings.HasSuffix(pattern, Suffix) {
		return pattern, false
	}
	return strings.TrimSuffix(pattern, Suffix), true
}

// IsWildcard 判断 pattern 是否带通配符尾。等价于 SplitSuffix 的第二个返回值，
// 当 caller 只需要布尔结果时用这个避免丢弃 prefix。
func IsWildcard(pattern string) bool {
	_, isWild := SplitSuffix(pattern)
	return isWild
}

// TrimSuffix 去掉尾部 "*"（若存在），等价于 SplitSuffix 的第一个返回值。
// 为可读性单独提供：调用方读 `wildcard.TrimSuffix(p)` 比读
// `prefix, _ := wildcard.SplitSuffix(p)` 更直观。
func TrimSuffix(pattern string) string {
	prefix, _ := SplitSuffix(pattern)
	return prefix
}

// Match 报告 target 是否匹配 pattern。
//
//	pattern = "*"             → 始终匹配（任意 target）
//	pattern = "abc*"          → strings.HasPrefix(target, "abc")
//	pattern = "abc"（无 *）   → target == "abc"（严格相等）
//	pattern = ""              → 仅匹配 target == ""
//
// 大小写敏感：caller 必须在调用前自行 ToLower 等做规范化。这样保持本包
// "纯通配工具"职责，不混入大小写策略；channel-management 内部业务统一在
// 调用现场 lowercase（与 channel_cache.go / account_stats_pricing.go 现有
// 行为一致）。
func Match(pattern, target string) bool {
	prefix, isWild := SplitSuffix(pattern)
	if isWild {
		return strings.HasPrefix(target, prefix)
	}
	return target == pattern
}
