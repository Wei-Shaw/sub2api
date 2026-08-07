package service

import "strings"

// 模型能力判定（服务端单边维护点）。
//
// /v1/models 列表的 reasoning_effort 档位与附件能力都从这里来：网关把能力字段
// 写进模型列表响应，客户端（opencode 插件等）只消费字段、不做本地推断。
// 新增模型时只需在这里补名单，客户端零改动。
//
// 与 thinking_protocol.go 的关系：那边判定「thinking block 回传契约」，这里判定
// 「客户端可声明的能力」；名单同源演进（deepseek / glm / grok / gpt 前缀一致）。

// ModelReasoningEfforts 返回模型支持的 reasoning_effort 档位，nil 表示不支持
// 可配置 reasoning effort。档位即 OpenAI 兼容协议里的 reasoning_effort 取值，
// 客户端（如 opencode variant）直接映射，无需再归一。
//
// 名单依据（实测 + 上游文档）：
//   - deepseek-*：原生支持 low/medium/high/max，网关实测透传全部接受
//   - glm-*：z.ai 原生尺度 high/max（low/medium/high 会被
//     NormalizeGLMOpenAIReasoningEffort 归并为 high），对外只报两档
//   - gpt-* / codex* / o 系列：OpenAI 原生 low/medium/high/xhigh，5.6 系另加 max
//   - grok 名单：low/medium/high（与 grokModelSupportsConfigurableReasoning 同源）
//   - 其它模型（longcat 等长尾）：不支持，返回 nil
func ModelReasoningEfforts(modelID string) []string {
	id := strings.ToLower(strings.TrimSpace(modelID))
	switch {
	case strings.HasPrefix(id, "deepseek-"):
		return []string{"low", "medium", "high", "max"}
	case strings.HasPrefix(id, "glm-"):
		return []string{"high", "max"}
	case strings.HasPrefix(id, "gpt-"), strings.HasPrefix(id, "codex"), isOForwardSeries(id):
		if strings.Contains(id, "5.6") {
			return []string{"low", "medium", "high", "xhigh", "max"}
		}
		return []string{"low", "medium", "high", "xhigh"}
	case isConfigurableReasoningGrok(id):
		return []string{"low", "medium", "high"}
	default:
		return nil
	}
}

// ModelSupportsAttachments 判断模型族是否支持图片输入。
// true = 视觉模型族（claude / gpt / o 系列 / gemini / grok）；
// 其余（deepseek / glm / kimi / minimax / qwen / 长尾模型）默认 false——
// 保守禁图，避免 opencode 允许贴图后请求被上游 400。
func ModelSupportsAttachments(modelID string) bool {
	id := strings.ToLower(strings.TrimSpace(modelID))
	switch {
	case strings.HasPrefix(id, "claude-"),
		strings.HasPrefix(id, "gpt-"),
		strings.HasPrefix(id, "gemini-"),
		strings.HasPrefix(id, "grok-"),
		isOForwardSeries(id):
		return true
	default:
		return false
	}
}

// isOForwardSeries 匹配 o 系列（o1 / o3 / o4 等）但排除 o 开头的长尾误命中。
func isOForwardSeries(id string) bool {
	if !strings.HasPrefix(id, "o") {
		return false
	}
	rest := strings.TrimPrefix(id, "o")
	if rest == "" {
		return false
	}
	return rest[0] >= '0' && rest[0] <= '9'
}

// isConfigurableReasoningGrok 与 handler 包 grokModelSupportsConfigurableReasoning
// 同源：grok 支持可配置 reasoning effort 的型号名单。
func isConfigurableReasoningGrok(id string) bool {
	switch id {
	case "grok-4.5", "grok-4.5-latest", "grok", "grok-latest", "grok-build", "grok-build-latest", "grok-build-0.1":
		return true
	default:
		return false
	}
}

// CapModelReasoningEfforts 把模型原生档位按 group 上限（MaxReasoningEffort）截断。
// 空上限 = 不限制。未知档位按不限制处理。
// 档位排序复用 openai_reasoning_effort_policy.go 的 reasoningEffortRank
// （minimal < low < medium < high < xhigh < max，经 NormalizeMaxReasoningEffort 归一）。
func CapModelReasoningEfforts(efforts []string, maxEffort string) []string {
	maxRank, hasMax := reasoningEffortRank(maxEffort)
	if !hasMax {
		return efforts
	}
	out := make([]string, 0, len(efforts))
	for _, e := range efforts {
		rank, recognized := reasoningEffortRank(e)
		if !recognized || rank <= maxRank {
			out = append(out, e)
		}
	}
	return out
}
