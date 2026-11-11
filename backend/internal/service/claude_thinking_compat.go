package service

import (
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/pkg/claude"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

// normalizeClaudeThinkingForModel 按目标模型的 thinking 约束改写请求体，避免上游 400：
//   - budget_tokens 已移除的模型：enabled → adaptive，并删除 budget_tokens
//   - thinking 常开的模型（Opus 5.5 / Fable / Mythos）：disabled → 删除 thinking 字段（省略即 adaptive）
//   - Sonnet 5.5：disabled → between_tools（effort 为 xhigh/max 时 between_tools 不可用，改为删除）
//   - Opus 5 / Haiku 5.5：disabled 仅在 effort <= high 时可用，否则删除 thinking 字段
//
// 未知或旧模型原样返回。
func normalizeClaudeThinkingForModel(body []byte, modelID string) []byte {
	policy := claude.ThinkingPolicyFor(modelID)
	if policy == (claude.ThinkingPolicy{}) {
		return body
	}

	thinking := gjson.GetBytes(body, "thinking")
	if !thinking.Exists() || !thinking.IsObject() {
		return body
	}

	switch thinking.Get("type").String() {
	case "enabled":
		if policy.BudgetTokensRemoved {
			body, _ = sjson.SetBytes(body, "thinking.type", "adaptive")
			body, _ = sjson.DeleteBytes(body, "thinking.budget_tokens")
		}
	case "adaptive":
		if policy.BudgetTokensRemoved && thinking.Get("budget_tokens").Exists() {
			body, _ = sjson.DeleteBytes(body, "thinking.budget_tokens")
		}
	case "disabled":
		effortAboveHigh := claudeRequestEffortAboveHigh(body)
		switch {
		case policy.DisabledRejected:
			body, _ = sjson.DeleteBytes(body, "thinking")
		case policy.DisabledUseBetweenTools:
			if effortAboveHigh {
				body, _ = sjson.DeleteBytes(body, "thinking")
			} else {
				// between_tools 不接受任何其他字段，整体替换
				body, _ = sjson.SetRawBytes(body, "thinking", []byte(`{"type":"between_tools"}`))
			}
		case policy.DisabledOnlyUpToHigh && effortAboveHigh:
			body, _ = sjson.DeleteBytes(body, "thinking")
		}
	}
	return body
}

// claudeRequestEffortAboveHigh 判断请求的 output_config.effort 是否为 xhigh / max。
func claudeRequestEffortAboveHigh(body []byte) bool {
	effort := strings.ToLower(strings.TrimSpace(gjson.GetBytes(body, "output_config.effort").String()))
	return effort == "xhigh" || effort == "max"
}
