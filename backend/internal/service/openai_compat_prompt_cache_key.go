package service

import (
	"encoding/json"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/pkg/apicompat"
)

const compatPromptCacheKeyPrefix = "compat_cc_"

func shouldAutoInjectPromptCacheKeyForCompat(model string) bool {
	trimmed := strings.TrimSpace(strings.ToLower(model))
	// 仅对 Codex OAuth 路径支持的 GPT-5 族开启自动注入，避免 normalizeCodexModel
	// 的默认兜底把任意模型（如 gpt-4o、claude-*）误判为 gpt-5.4。
	if !strings.Contains(trimmed, "gpt-5") && !strings.Contains(trimmed, "codex") {
		return false
	}
	switch normalizeCodexModel(trimmed) {
	case "gpt-5.4", "gpt-5.3-codex", "gpt-5.3-codex-spark":
		return true
	default:
		return false
	}
}

func deriveCompatPromptCacheKey(req *apicompat.ChatCompletionsRequest, mappedModel string) string {
	if req == nil {
		return ""
	}

	normalizedModel := normalizeCodexModel(strings.TrimSpace(mappedModel))
	if normalizedModel == "" {
		normalizedModel = normalizeCodexModel(strings.TrimSpace(req.Model))
	}
	if normalizedModel == "" {
		normalizedModel = strings.TrimSpace(req.Model)
	}

	seedParts := []string{"model=" + normalizedModel}
	if req.ReasoningEffort != "" {
		seedParts = append(seedParts, "reasoning_effort="+strings.TrimSpace(req.ReasoningEffort))
	}
	if len(req.ToolChoice) > 0 {
		seedParts = append(seedParts, "tool_choice="+normalizeCompatSeedJSON(req.ToolChoice))
	}
	if len(req.Tools) > 0 {
		if raw, err := json.Marshal(req.Tools); err == nil {
			seedParts = append(seedParts, "tools="+normalizeCompatSeedJSON(raw))
		}
	}
	if len(req.Tools) == 0 && req.BuiltinTools != nil {
		if raw, err := json.Marshal(normalizeOpenAIBuiltinTools(req.BuiltinTools)); err == nil && string(raw) != "null" && string(raw) != "[]" {
			seedParts = append(seedParts, "tools="+normalizeCompatSeedJSON(raw))
		}
	}
	if len(req.Functions) > 0 {
		if raw, err := json.Marshal(req.Functions); err == nil {
			seedParts = append(seedParts, "functions="+normalizeCompatSeedJSON(raw))
		}
	}

	firstUserCaptured := false
	for _, msg := range req.Messages {
		switch strings.TrimSpace(msg.Role) {
		case "system":
			seedParts = append(seedParts, "system="+normalizeCompatSeedJSON(msg.Content))
		case "user":
			if !firstUserCaptured {
				seedParts = append(seedParts, "first_user="+normalizeCompatSeedJSON(msg.Content))
				firstUserCaptured = true
			}
		}
	}

	return compatPromptCacheKeyPrefix + hashSensitiveValueForLog(strings.Join(seedParts, "|"))
}

func deriveCompatPromptCacheKeyFromResponsesBody(body []byte, mappedModel string) string {
	if len(body) == 0 {
		return ""
	}

	var req map[string]any
	if err := json.Unmarshal(body, &req); err != nil {
		return ""
	}

	normalizedModel := normalizeCodexModel(strings.TrimSpace(mappedModel))
	if normalizedModel == "" {
		normalizedModel = normalizeCodexModel(firstNonEmptyString(req["model"]))
	}
	if normalizedModel == "" {
		normalizedModel = firstNonEmptyString(req["model"])
	}

	seedParts := []string{"model=" + normalizedModel}
	if reasoning, ok := req["reasoning"].(map[string]any); ok {
		if effort := firstNonEmptyString(reasoning["effort"]); effort != "" {
			seedParts = append(seedParts, "reasoning_effort="+effort)
		}
	}
	if choice, ok := req["tool_choice"]; ok && choice != nil {
		seedParts = append(seedParts, "tool_choice="+normalizeCompatSeedValue(choice))
	}
	if tools, ok := req["tools"]; ok && tools != nil {
		seedParts = append(seedParts, "tools="+normalizeCompatSeedValue(tools))
	}
	if instructions := firstNonEmptyString(req["instructions"]); instructions != "" {
		seedParts = append(seedParts, "system="+normalizeCompatSeedValue(instructions))
	}

	firstUserCaptured := false
	switch input := req["input"].(type) {
	case string:
		if strings.TrimSpace(input) != "" {
			seedParts = append(seedParts, "first_user="+normalizeCompatSeedValue(input))
		}
	case []any:
		for _, rawItem := range input {
			item, ok := rawItem.(map[string]any)
			if !ok {
				continue
			}
			switch strings.TrimSpace(firstNonEmptyString(item["role"])) {
			case "system":
				seedParts = append(seedParts, "system="+normalizeCompatSeedValue(item["content"]))
			case "user":
				if !firstUserCaptured {
					seedParts = append(seedParts, "first_user="+normalizeCompatSeedValue(item["content"]))
					firstUserCaptured = true
				}
			}
		}
	}

	return compatPromptCacheKeyPrefix + hashSensitiveValueForLog(strings.Join(seedParts, "|"))
}

func normalizeCompatSeedValue(v any) string {
	raw, err := json.Marshal(v)
	if err != nil {
		return ""
	}
	return normalizeCompatSeedJSON(raw)
}

func normalizeCompatSeedJSON(v json.RawMessage) string {
	if len(v) == 0 {
		return ""
	}
	var tmp any
	if err := json.Unmarshal(v, &tmp); err != nil {
		return string(v)
	}
	out, err := json.Marshal(tmp)
	if err != nil {
		return string(v)
	}
	return string(out)
}
