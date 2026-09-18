package service

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/google/uuid"

	"github.com/Wei-Shaw/sub2api/internal/pkg/apicompat"
)

// Codex remote compaction v2 用 input 里的 {"type":"compaction_trigger"} 表达
// 「请压缩这段对话」，并要求响应的 output 里恰好一个 type=compaction 的 item。
//
// 走 CC 直转的第三方 OpenAI 兼容上游（force_chat_completions，或探测判定上游
// 没有 /v1/responses）两头都接不住这套协议：
//
//   - 请求方向：compaction_trigger 不是 message，被 buildChatMessagesFromItems
//     按「无 Chat 等价物的 item」跳过 → 上游从未收到「请总结」的指令，于是按
//     普通一轮对话回复；
//   - 响应方向：CC 回复被映射成 reasoning + message 两个 item，其中没有
//     compaction → Codex 报 "remote compaction v2 expected exactly one
//     compaction output item, got 0 from 2 output items"。
//
// 此外，下一轮 Codex 回传的 compaction item 同样会被跳过，压缩后的历史整段丢失。
//
// 这里按 Grok compact 桥（openai_gateway_grok_compact.go）已有的思路补齐同一层
// 语义，区别是通用 CC 上游没有 reasoning.encrypted_content 可搬运，因此
// compaction item 只携带可见的 summary 文本。

// compatCompactSummaryTag 包裹回放给上游的历史摘要，与 Grok compact 桥保持一致，
// 便于模型识别「这段是此前压缩产生的摘要」。
const compatCompactSummaryTag = "conversation_summary"

// isCompatCompactionRequest 判断这是不是一次 Codex remote compaction 请求。
func isCompatCompactionRequest(body []byte) bool {
	return HasCompactionTriggerInInput(body)
}

// rewriteCompatCompactRequestBody 把 remote compaction 请求体改写成上游看得懂的
// 普通 Responses 请求：
//
//   - compaction_trigger  → 携带总结指令的 user 消息
//   - compaction / compaction_summary（历史轮的压缩结果）→ <conversation_summary>
//     user 消息，避免被下游按未知 item 丢弃
//
// 改写在 Responses 层完成，后续沿用既有的 ResponsesToChatCompletionsRequest，
// 不需要给 apicompat 增加 compaction 语义。
func rewriteCompatCompactRequestBody(body []byte) ([]byte, error) {
	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, fmt.Errorf("decode compact request: %w", err)
	}
	items, ok := payload["input"].([]any)
	if !ok {
		return body, nil
	}

	converted := make([]any, 0, len(items)+1)
	changed := false
	for _, raw := range items {
		item, ok := raw.(map[string]any)
		if !ok {
			converted = append(converted, raw)
			continue
		}
		switch itemType := strings.TrimSpace(stringValue(item["type"])); {
		case itemType == "compaction_trigger":
			changed = true
			converted = append(converted, compatCompactUserMessage(grokCompactSummaryPrompt))
		case isOpenAICompactionType(itemType):
			changed = true
			if summary := compactSummaryText(item["summary"]); summary != "" {
				converted = append(converted, compatCompactUserMessage(
					"<"+compatCompactSummaryTag+">\n"+summary+"\n</"+compatCompactSummaryTag+">",
				))
			}
		default:
			converted = append(converted, raw)
		}
	}
	if !changed {
		return body, nil
	}

	payload["input"] = converted
	// 压缩是一次纯总结调用：强制非流式（与 Grok compact 一致，便于把最终结果
	// 收敛成单个 compaction item），并禁止模型改去调工具。
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

func compatCompactUserMessage(text string) map[string]any {
	return map[string]any{
		"type": "message",
		"role": "user",
		"content": []any{map[string]any{
			"type": "input_text",
			"text": text,
		}},
	}
}

// buildCompatCompactResponse 把上游的 Chat Completions 回复收敛成 Codex remote
// compaction v2 期望的形态：output 里恰好一个 type=compaction 的 item。
//
// 通用 CC 上游没有 encrypted_content，摘要全部放在可见的 summary 里；回放时由
// rewriteCompatCompactRequestBody 还原成 <conversation_summary> 消息，round trip
// 自洽。
func buildCompatCompactResponse(resp *apicompat.ChatCompletionsResponse, model string) (*apicompat.ResponsesResponse, error) {
	if resp == nil {
		return nil, fmt.Errorf("compact response is nil")
	}
	summary := ""
	if len(resp.Choices) > 0 {
		message := resp.Choices[0].Message
		summary = strings.TrimSpace(chatMessagePlainText(message.Content))
		if summary == "" {
			// 只回了推理、没回正文的上游（部分国产模型的 thinking 形态）以推理
			// 文本兜底，总比交回空摘要好。
			summary = strings.TrimSpace(message.ReasoningContent)
		}
	}
	if summary == "" {
		return nil, fmt.Errorf("compact response carries no summary text")
	}

	id := strings.TrimSpace(resp.ID)
	if id == "" {
		id = "resp_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	}
	out := &apicompat.ResponsesResponse{
		ID:     id,
		Object: "response",
		Model:  model,
		Status: "completed",
		Output: []apicompat.ResponsesOutput{{
			Type:   "compaction",
			ID:     "cmp_" + strings.ReplaceAll(uuid.NewString(), "-", ""),
			Status: "completed",
			Summary: []apicompat.ResponsesSummary{{
				Type: "summary_text",
				Text: summary,
			}},
		}},
	}
	if resp.Usage != nil {
		out.Usage = apicompat.ChatUsageToResponsesUsage(resp.Usage)
	}
	return out, nil
}

// chatMessagePlainText 把 Chat 消息的 content 取成纯文本：字符串形态直接返回，
// 多模态数组形态拼接其中的 text 片段。
func chatMessagePlainText(content json.RawMessage) string {
	if len(content) == 0 {
		return ""
	}
	var text string
	if err := json.Unmarshal(content, &text); err == nil {
		return text
	}
	var parts []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}
	if err := json.Unmarshal(content, &parts); err != nil {
		return ""
	}
	texts := make([]string, 0, len(parts))
	for _, part := range parts {
		if trimmed := strings.TrimSpace(part.Text); trimmed != "" {
			texts = append(texts, trimmed)
		}
	}
	return strings.Join(texts, "\n")
}
