package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

type anthropicRequest struct {
	Model       string          `json:"model"`
	MaxTokens   int             `json:"max_tokens"`
	Messages    []anthropicMsg  `json:"messages"`
	System      json.RawMessage `json:"system,omitempty"`
	Stream      bool            `json:"stream"`
	Temperature *float64        `json:"temperature,omitempty"`
	TopP        *float64        `json:"top_p,omitempty"`
}

type anthropicMsg struct {
	Role    string          `json:"role"`
	Content json.RawMessage `json:"content"`
}

func anthropicMessagesToChatCompletions(body []byte) ([]byte, bool, string, error) {
	var req anthropicRequest
	if err := json.Unmarshal(body, &req); err != nil {
		return nil, false, "", err
	}
	if strings.TrimSpace(req.Model) == "" {
		return nil, false, "", fmt.Errorf("model is required")
	}
	if len(req.Messages) == 0 {
		return nil, false, "", fmt.Errorf("messages is required")
	}
	messages := make([]map[string]any, 0, len(req.Messages)+1)
	if sys := rawContentToText(req.System); sys != "" {
		messages = append(messages, map[string]any{"role": "system", "content": sys})
	}
	for _, msg := range req.Messages {
		role := strings.TrimSpace(msg.Role)
		if role == "" {
			role = "user"
		}
		messages = append(messages, map[string]any{
			"role":    role,
			"content": rawContentToText(msg.Content),
		})
	}
	payload := map[string]any{
		"model":    mapModel(req.Model),
		"messages": messages,
		"stream":   false,
	}
	if req.MaxTokens > 0 {
		payload["max_tokens"] = req.MaxTokens
	}
	if req.Temperature != nil {
		payload["temperature"] = *req.Temperature
	}
	if req.TopP != nil {
		payload["top_p"] = *req.TopP
	}
	out, err := json.Marshal(payload)
	return out, req.Stream, req.Model, err
}

func rawContentToText(raw json.RawMessage) string {
	if len(raw) == 0 || string(raw) == "null" {
		return ""
	}
	var text string
	if err := json.Unmarshal(raw, &text); err == nil {
		return text
	}
	var blocks []map[string]any
	if err := json.Unmarshal(raw, &blocks); err == nil {
		parts := make([]string, 0, len(blocks))
		for _, block := range blocks {
			switch asString(block["type"]) {
			case "text":
				if t := asString(block["text"]); t != "" {
					parts = append(parts, t)
				}
			case "tool_result":
				if t := rawAnyToText(block["content"]); t != "" {
					parts = append(parts, t)
				}
			}
		}
		return strings.Join(parts, "\n\n")
	}
	return string(raw)
}

func rawAnyToText(value any) string {
	switch typed := value.(type) {
	case string:
		return typed
	case []any:
		var parts []string
		for _, item := range typed {
			if m, ok := item.(map[string]any); ok && asString(m["type"]) == "text" {
				parts = append(parts, asString(m["text"]))
			}
		}
		return strings.Join(parts, "\n\n")
	default:
		return ""
	}
}

func chatCompletionToAnthropic(body []byte, fallbackModel string) (map[string]any, error) {
	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, err
	}
	id := firstNonEmpty(asString(payload["id"]), "msg_"+randomHex(12))
	model := firstNonEmpty(asString(payload["model"]), fallbackModel)
	content := ""
	stopReason := "end_turn"
	if choices, ok := payload["choices"].([]any); ok && len(choices) > 0 {
		if choice, ok := choices[0].(map[string]any); ok {
			if msg, ok := choice["message"].(map[string]any); ok {
				content = asString(msg["content"])
			}
			switch asString(choice["finish_reason"]) {
			case "length":
				stopReason = "max_tokens"
			case "tool_calls":
				stopReason = "tool_use"
			}
		}
	}
	usage := map[string]any{"input_tokens": 0, "output_tokens": 0}
	if u, ok := payload["usage"].(map[string]any); ok {
		usage["input_tokens"] = numberAsInt(u["prompt_tokens"])
		usage["output_tokens"] = numberAsInt(u["completion_tokens"])
	}
	return map[string]any{
		"id":            id,
		"type":          "message",
		"role":          "assistant",
		"model":         model,
		"content":       []map[string]any{{"type": "text", "text": content}},
		"stop_reason":   stopReason,
		"stop_sequence": nil,
		"usage":         usage,
	}, nil
}

func writeAnthropicSSE(w http.ResponseWriter, resp map[string]any) {
	id := asString(resp["id"])
	model := asString(resp["model"])
	usage, _ := resp["usage"].(map[string]any)
	text := ""
	if content, ok := resp["content"].([]map[string]any); ok && len(content) > 0 {
		text = asString(content[0]["text"])
	}
	writeSSE(w, "message_start", map[string]any{
		"type": "message_start",
		"message": map[string]any{
			"id": id, "type": "message", "role": "assistant", "model": model,
			"content": []any{}, "stop_reason": nil, "stop_sequence": nil,
			"usage": map[string]any{"input_tokens": usage["input_tokens"], "output_tokens": 0},
		},
	})
	writeSSE(w, "content_block_start", map[string]any{
		"type": "content_block_start", "index": 0,
		"content_block": map[string]any{"type": "text", "text": ""},
	})
	if text != "" {
		writeSSE(w, "content_block_delta", map[string]any{
			"type": "content_block_delta", "index": 0,
			"delta": map[string]any{"type": "text_delta", "text": text},
		})
	}
	writeSSE(w, "content_block_stop", map[string]any{"type": "content_block_stop", "index": 0})
	writeSSE(w, "message_delta", map[string]any{
		"type":  "message_delta",
		"delta": map[string]any{"stop_reason": resp["stop_reason"], "stop_sequence": nil},
		"usage": map[string]any{"output_tokens": usage["output_tokens"]},
	})
	writeSSE(w, "message_stop", map[string]any{"type": "message_stop"})
}

func writeSSE(w http.ResponseWriter, event string, payload any) {
	data, _ := json.Marshal(payload)
	_, _ = fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event, data)
	if f, ok := w.(http.Flusher); ok {
		f.Flush()
	}
}

func writeAnthropicError(w http.ResponseWriter, status int, typ, message string) {
	writeJSON(w, status, map[string]any{
		"type": "error",
		"error": map[string]any{
			"type":    typ,
			"message": message,
		},
	})
}

func numberAsInt(v any) int {
	switch typed := v.(type) {
	case json.Number:
		n, _ := typed.Int64()
		return int(n)
	case float64:
		return int(typed)
	case int:
		return typed
	default:
		return 0
	}
}

func init() {
	_ = time.Second
}
