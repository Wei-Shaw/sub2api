package service

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/google/uuid"

	"github.com/Wei-Shaw/sub2api/internal/pkg/apicompat"
)

// grokCompactSummaryPrompt 是 grok-build 的 build_compaction_prompt(None, false)。
// Grok 不提供 OpenAI 兼容的 /responses/compact 端点，所以压缩降级为一次普通
// Responses 轮次，由最后一条 user item 要求模型产出摘要。
//
// 该指令与 Anthropic 协议族（Kiro/Anthropic/Bedrock/Vertex）逐字相同——两者的
// 上游都没有原生 compact 端点，降级手法一致——故收敛到 apicompat 共享。
const grokCompactSummaryPrompt = apicompat.CompactionSummaryPrompt

func buildGrokCompactRequestBody(body []byte) ([]byte, error) {
	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, fmt.Errorf("decode compact request: %w", err)
	}

	input, err := normalizeGrokCompactInput(payload["input"])
	if err != nil {
		return nil, err
	}
	input = append(input, map[string]any{
		"type": "message",
		"role": "user",
		"content": []any{map[string]any{
			"type": "input_text",
			"text": grokCompactSummaryPrompt,
		}},
	})
	payload["input"] = input
	payload["include"] = []any{"reasoning.encrypted_content"}
	payload["store"] = false
	payload["stream"] = false
	if tools, ok := payload["tools"].([]any); ok && len(tools) > 0 {
		payload["tool_choice"] = "none"
	}

	encoded, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("encode compact request: %w", err)
	}
	return encoded, nil
}

func normalizeGrokCompactInput(value any) ([]any, error) {
	switch input := value.(type) {
	case nil:
		return []any{}, nil
	case []any:
		return input, nil
	case string:
		return []any{map[string]any{
			"type": "message",
			"role": "user",
			"content": []any{map[string]any{
				"type": "input_text",
				"text": input,
			}},
		}}, nil
	case map[string]any:
		return []any{input}, nil
	default:
		return nil, fmt.Errorf("compact input must be a string, object, or array")
	}
}

// convertOpenAICompactInputsForGrok reverses compact output items from prior
// turns. The encrypted blob originated as Grok reasoning and must be replayed
// under that type. The visible summary is added as conversation context.
func convertOpenAICompactInputsForGrok(body []byte) ([]byte, error) {
	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, err
	}
	items, ok := payload["input"].([]any)
	if !ok {
		return body, nil
	}

	changed := false
	converted := make([]any, 0, len(items))
	for _, raw := range items {
		item, ok := raw.(map[string]any)
		if !ok || !isOpenAICompactionType(stringValue(item["type"])) {
			converted = append(converted, raw)
			continue
		}
		changed = true
		if encrypted := strings.TrimSpace(stringValue(item["encrypted_content"])); encrypted != "" {
			converted = append(converted, map[string]any{
				"type":              "reasoning",
				"summary":           []any{},
				"encrypted_content": encrypted,
			})
		}
		if summary := compactSummaryText(item["summary"]); summary != "" {
			converted = append(converted, map[string]any{
				"type": "message",
				"role": "user",
				"content": []any{map[string]any{
					"type": "input_text",
					"text": "<conversation_summary>\n" + summary + "\n</conversation_summary>",
				}},
			})
		}
	}
	if !changed {
		return body, nil
	}
	payload["input"] = converted
	encoded, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	return encoded, nil
}

func convertGrokResponseToOpenAICompact(body []byte) ([]byte, error) {
	var response map[string]any
	if err := json.Unmarshal(body, &response); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}
	output, ok := response["output"].([]any)
	if !ok {
		return nil, fmt.Errorf("response has no output array")
	}

	var encrypted string
	var summaryParts []string
	for _, raw := range output {
		item, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		switch strings.TrimSpace(stringValue(item["type"])) {
		case "reasoning":
			if value := strings.TrimSpace(stringValue(item["encrypted_content"])); value != "" {
				encrypted = value
			}
		case "message":
			if content, ok := item["content"].([]any); ok {
				for _, rawContent := range content {
					part, ok := rawContent.(map[string]any)
					if !ok {
						continue
					}
					if text := strings.TrimSpace(stringValue(part["text"])); text != "" {
						summaryParts = append(summaryParts, text)
					}
				}
			}
		}
	}
	if encrypted == "" {
		return nil, fmt.Errorf("response has no reasoning.encrypted_content")
	}

	compactItem := map[string]any{
		"id":                "cmp_" + strings.ReplaceAll(uuid.NewString(), "-", ""),
		"type":              "compaction",
		"status":            "completed",
		"encrypted_content": encrypted,
	}
	if summary := strings.TrimSpace(strings.Join(summaryParts, "\n")); summary != "" {
		compactItem["summary"] = []any{map[string]any{
			"type": "summary_text",
			"text": summary,
		}}
	}
	response["output"] = []any{compactItem}
	response["status"] = "completed"
	delete(response, "output_text")

	encoded, err := json.Marshal(response)
	if err != nil {
		return nil, fmt.Errorf("encode compact response: %w", err)
	}
	return encoded, nil
}

func compactSummaryText(value any) string {
	parts, ok := value.([]any)
	if !ok {
		return ""
	}
	texts := make([]string, 0, len(parts))
	for _, raw := range parts {
		part, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		if text := strings.TrimSpace(stringValue(part["text"])); text != "" {
			texts = append(texts, text)
		}
	}
	return strings.Join(texts, "\n")
}

func isOpenAICompactionType(value string) bool {
	switch strings.TrimSpace(value) {
	case "compaction", "compaction_summary":
		return true
	default:
		return false
	}
}

func stringValue(value any) string {
	text, _ := value.(string)
	return text
}
