package apicompat

import (
	"bytes"
	"encoding/json"

	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

func IsDeepSeekWebTool(typ string) bool {
	return typ == "deepseek_web_search" || typ == "deepseek_read_url"
}

type chatServerToolStep struct {
	Index            int                  `json:"index"`
	ReasoningContent string               `json:"reasoning_content"`
	Calls            []chatServerToolCall `json:"calls"`
}

type chatServerToolCall struct {
	Index     int                        `json:"index"`
	ID        string                     `json:"id"`
	Type      string                     `json:"type"`
	Kind      string                     `json:"kind"`
	Action    map[string]json.RawMessage `json:"action"`
	Status    json.RawMessage            `json:"status,omitempty"`
	Page      json.RawMessage            `json:"page,omitempty"`
	Sources   json.RawMessage            `json:"sources,omitempty"`
	Error     json.RawMessage            `json:"error,omitempty"`
	Signature json.RawMessage            `json:"signature,omitempty"`
}

func deepSeekChatWebCall(item map[string]json.RawMessage) *chatServerToolCall {
	var action map[string]json.RawMessage
	if rawString(item["id"]) == "" || json.Unmarshal(item["action"], &action) != nil || action == nil {
		return nil
	}
	call := &chatServerToolCall{
		ID: rawString(item["id"]), Type: "deepseek_web_search", Kind: rawString(item["kind"]),
		Action: action, Status: item["status"], Page: item["page"], Sources: action["sources"],
		Error: item["error"], Signature: item["signature"],
	}
	delete(action, "sources")
	switch call.Kind {
	case "read_link":
		call.Type = "deepseek_read_url"
		action["type"] = json.RawMessage(`"read_link"`)
	case "":
		call.Kind = "search"
	}
	return call
}

// Keep the provider's complete declaration across protocols, including fields
// unknown to the gateway. Ordinary tools continue to use their protocol DTO.
func deepSeekToolDeclaration(typ string, raw []byte) json.RawMessage {
	if IsDeepSeekWebTool(typ) {
		return bytes.Clone(raw)
	}
	return nil
}

func (t *ChatTool) UnmarshalJSON(raw []byte) error {
	type wire ChatTool
	if err := json.Unmarshal(raw, (*wire)(t)); err != nil {
		return err
	}
	t.deepSeekDeclaration = deepSeekToolDeclaration(t.Type, raw)
	return nil
}

func (t ChatTool) MarshalJSON() ([]byte, error) {
	if t.deepSeekDeclaration != nil {
		return t.deepSeekDeclaration, nil
	}
	type wire ChatTool
	return json.Marshal(wire(t))
}

func (t ResponsesTool) MarshalJSON() ([]byte, error) {
	if t.deepSeekDeclaration != nil {
		return t.deepSeekDeclaration, nil
	}
	type wire ResponsesTool
	return json.Marshal(wire(t))
}

func (t *AnthropicTool) UnmarshalJSON(raw []byte) error {
	type wire AnthropicTool
	if err := json.Unmarshal(raw, (*wire)(t)); err != nil {
		return err
	}
	t.deepSeekDeclaration = deepSeekToolDeclaration(t.Type, raw)
	return nil
}

func (t AnthropicTool) MarshalJSON() ([]byte, error) {
	if raw := t.deepSeekDeclaration; raw != nil {
		if t.Name != "" && gjson.GetBytes(raw, "name").String() == "" {
			return sjson.SetBytes(raw, "name", t.Name)
		}
		return raw, nil
	}
	type wire AnthropicTool
	return json.Marshal(wire(t))
}
