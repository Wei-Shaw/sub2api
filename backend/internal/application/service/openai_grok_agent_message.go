package service

import (
	"encoding/json"

	"github.com/Wei-Shaw/sub2api/internal/shared/apicompat"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

// Normalize delegated messages without decoding and re-encoding unrelated
// history or losing the order of plaintext encrypted_content fragments.
func normalizeGrokAgentMessages(body []byte) ([]byte, error) {
	input := parseRawJSONView(body).Get("input")
	if !input.IsArray() {
		return body, nil
	}
	hasAgent := false
	input.ForEach(func(_, item gjson.Result) bool {
		hasAgent = item.Get("type").String() == "agent_message"
		return !hasAgent
	})
	if !hasAgent {
		return body, nil
	}
	items := make([]json.RawMessage, 0)
	input.ForEach(func(_, item gjson.Result) bool {
		if item.Get("type").String() != "agent_message" {
			items = append(items, json.RawMessage(item.Raw))
			return true
		}
		text := apicompat.AgentMessageText(json.RawMessage(item.Get("content").Raw))
		if text != "" {
			message, _ := json.Marshal(struct {
				Type    string `json:"type"`
				Role    string `json:"role"`
				Content string `json:"content"`
			}{"message", "user", text})
			items = append(items, message)
		}
		return true
	})
	encoded, err := json.Marshal(items)
	if err != nil {
		return nil, err
	}
	return sjson.SetRawBytes(body, "input", encoded)
}
