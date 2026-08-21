package service

import (
	"encoding/json"
	"fmt"

	"github.com/Wei-Shaw/sub2api/internal/pkg/apicompat"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

// normalizeOpenAIResponsesFunctionToolChoiceBody accepts the Chat Completions
// function-choice shape on the Responses ingress. The patch is deliberately
// applied before account-specific routing so API-key, passthrough, OAuth, and
// websocket Responses upstreams receive the same canonical shape.
func normalizeOpenAIResponsesFunctionToolChoiceBody(body []byte) ([]byte, bool, error) {
	if len(body) == 0 {
		return body, false, nil
	}

	toolChoice := gjson.GetBytes(body, "tool_choice")
	if !toolChoice.Exists() {
		return body, false, nil
	}
	normalized, changed := apicompat.NormalizeResponsesFunctionToolChoice(json.RawMessage(toolChoice.Raw))
	if !changed {
		return body, false, nil
	}

	patched, err := sjson.SetRawBytes(body, "tool_choice", normalized)
	if err != nil {
		return body, false, fmt.Errorf("set normalized Responses tool_choice: %w", err)
	}
	return patched, true, nil
}
