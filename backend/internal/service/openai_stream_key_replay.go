package service

import (
	"context"

	"github.com/tidwall/gjson"
)

// OpenAIStreamKeyReplayAllowed reports whether an API-key Responses request can
// be replayed before any output reaches the client.
func OpenAIStreamKeyReplayAllowed(ctx context.Context, body []byte) bool {
	if ctx == nil || ctx.Err() != nil || !plainOpenAIStreamReplayTurn(body) || gjson.GetBytes(body, "prompt").Exists() {
		return false
	}
	tools := gjson.GetBytes(body, "tools")
	if tools.Exists() && !tools.IsArray() {
		return false
	}
	for _, tool := range tools.Array() {
		switch tool.Get("type").String() {
		case "function", "custom":
		default:
			return false
		}
	}
	return true
}

func plainOpenAIStreamReplayTurn(body []byte) bool {
	if !gjson.ValidBytes(body) {
		return false
	}
	root := gjson.ParseBytes(body)
	if root.Get("previous_response_id").Exists() || root.Get("conversation").Exists() || root.Get("background").Bool() {
		return false
	}
	input := root.Get("input")
	if input.Type == gjson.String {
		return true
	}
	if !input.IsArray() {
		return false
	}
	for _, item := range input.Array() {
		typ, role := item.Get("type").String(), item.Get("role").String()
		if (typ != "" && typ != "message") || item.Get("id").Exists() || item.Get("encrypted_content").Exists() ||
			(role != "user" && role != "assistant" && role != "developer" && role != "system") {
			return false
		}
		content := item.Get("content")
		if content.Type == gjson.String {
			continue
		}
		if !content.IsArray() {
			return false
		}
		for _, part := range content.Array() {
			partType := part.Get("type").String()
			if (partType != "input_text" && partType != "output_text" && partType != "text") || part.Get("text").Type != gjson.String {
				return false
			}
		}
	}
	return true
}
