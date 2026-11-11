package claude

import (
	"regexp"
	"strconv"
	"strings"
)

// ModelVersion 描述从 Claude 模型 ID 中解析出的系列与版本号。
type ModelVersion struct {
	Family string // opus / sonnet / haiku / fable / mythos
	Major  int
	Minor  int
}

// AtLeast 判断版本是否 >= major.minor。
func (v ModelVersion) AtLeast(major, minor int) bool {
	return v.Major > major || (v.Major == major && v.Minor >= minor)
}

// modelVersionRe 匹配新命名格式的 Claude 模型 ID（claude-{family}-{major}[-.{minor}]）。
//   - minor 最多两位数字，避免把 claude-sonnet-4-20250514 的日期误识别为 minor
//   - 支持任意前缀（Bedrock 的 "us.anthropic."、Vertex 资源名等）
//   - 旧命名（claude-3-5-sonnet）不匹配，调用方应走原有逻辑
var modelVersionRe = regexp.MustCompile(`claude-(opus|sonnet|haiku|fable|mythos)-(\d{1,2})(?:[-.](\d{1,2}))?(?:[^0-9]|$)`)

// ParseModelVersion 解析 Claude 模型 ID 中的系列与版本号。
func ParseModelVersion(modelID string) (ModelVersion, bool) {
	m := modelVersionRe.FindStringSubmatch(strings.ToLower(modelID))
	if m == nil {
		return ModelVersion{}, false
	}
	major, _ := strconv.Atoi(m[2])
	minor := 0
	if m[3] != "" {
		minor, _ = strconv.Atoi(m[3])
	}
	return ModelVersion{Family: m[1], Major: major, Minor: minor}, true
}

// IsGeneration5OrNewer 判断是否为 Claude 5 代及之后的模型（含 Fable / Mythos 全系列）。
func IsGeneration5OrNewer(modelID string) bool {
	v, ok := ParseModelVersion(modelID)
	if !ok {
		return false
	}
	if v.Family == "fable" || v.Family == "mythos" {
		return true
	}
	return v.Major >= 5
}

// ThinkingPolicy 描述某个模型对 thinking 参数的约束。
type ThinkingPolicy struct {
	// BudgetTokensRemoved: {type:"enabled", budget_tokens} 会返回 400，需改写为 adaptive。
	BudgetTokensRemoved bool
	// DisabledRejected: {type:"disabled"} 在任何 effort 下都会返回 400（thinking 常开）。
	DisabledRejected bool
	// DisabledOnlyUpToHigh: {type:"disabled"} 仅在 effort <= high 时接受。
	DisabledOnlyUpToHigh bool
	// DisabledUseBetweenTools: 关闭 thinking 需改用 {type:"between_tools"}（仅 effort <= high）。
	DisabledUseBetweenTools bool
}

// ThinkingPolicyFor 返回模型的 thinking 约束（未知/旧模型返回零值，表示保持原样）。
//
// 依据 Anthropic 官方模型文档（2026-11）：
//   - Opus 4.7 / 4.8 / Sonnet 5：移除 budget_tokens；disabled 可用
//   - Opus 5 / Haiku 5.5：移除 budget_tokens；disabled 仅 effort <= high
//   - Sonnet 5.5：移除 budget_tokens；disabled 返回 400，需改用 between_tools
//   - Opus 5.5 / Fable 5.x / Mythos 5.x：thinking 常开，budget_tokens 与 disabled 均返回 400
func ThinkingPolicyFor(modelID string) ThinkingPolicy {
	v, ok := ParseModelVersion(modelID)
	if !ok {
		return ThinkingPolicy{}
	}
	switch v.Family {
	case "fable", "mythos":
		return ThinkingPolicy{BudgetTokensRemoved: true, DisabledRejected: true}
	case "opus":
		switch {
		case v.AtLeast(5, 5):
			return ThinkingPolicy{BudgetTokensRemoved: true, DisabledRejected: true}
		case v.AtLeast(5, 0):
			return ThinkingPolicy{BudgetTokensRemoved: true, DisabledOnlyUpToHigh: true}
		case v.AtLeast(4, 7):
			return ThinkingPolicy{BudgetTokensRemoved: true}
		}
	case "sonnet":
		switch {
		case v.AtLeast(5, 5):
			return ThinkingPolicy{BudgetTokensRemoved: true, DisabledUseBetweenTools: true}
		case v.AtLeast(5, 0):
			return ThinkingPolicy{BudgetTokensRemoved: true}
		}
	case "haiku":
		if v.AtLeast(5, 5) {
			return ThinkingPolicy{BudgetTokensRemoved: true, DisabledOnlyUpToHigh: true}
		}
	}
	return ThinkingPolicy{}
}
