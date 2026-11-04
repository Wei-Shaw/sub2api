package service

import (
	"encoding/json"

	"github.com/Wei-Shaw/sub2api/internal/shared/antigravity"
	"github.com/Wei-Shaw/sub2api/internal/shared/apicompat"
	"github.com/tidwall/gjson"
)

// Rewrite the input once, instead of copying its entire history for every
// system text part. Keep user messages and unknown fields unchanged.
func convertAntigravityCompatResponsesToAnthropic(request *apicompat.ResponsesRequest) (*apicompat.AnthropicRequest, error) {
	copyRequest := *request
	copyRequest.Instructions = antigravity.NeutralizeSystemIdentity(request.Instructions)
	hasSystem := false
	parseRawJSONView(request.Input).ForEach(func(_, item gjson.Result) bool {
		role := item.Get("role").String()
		hasSystem = (role == "system" || role == "developer") &&
			antigravitySystemContentNeedsNormalization(item.Get("content"))
		return !hasSystem
	})
	if hasSystem {
		var items []map[string]json.RawMessage
		if json.Unmarshal(request.Input, &items) == nil {
			changed := false
			for _, item := range items {
				var role string
				_ = json.Unmarshal(item["role"], &role)
				if role != "system" && role != "developer" {
					continue
				}
				if content, ok := neutralizeAntigravitySystemContent(item["content"]); ok {
					item["content"] = content
					changed = true
				}
			}
			if changed {
				var err error
				copyRequest.Input, err = json.Marshal(items)
				if err != nil {
					return nil, err
				}
			}
		}
	}
	return apicompat.ResponsesToAnthropicRequest(&copyRequest)
}

func antigravitySystemContentNeedsNormalization(content gjson.Result) bool {
	needsChange := func(text gjson.Result) bool {
		return text.Type == gjson.String && antigravity.NeutralizeSystemIdentity(text.Str) != text.Str
	}
	if content.Type == gjson.String {
		return needsChange(content)
	}
	changed := false
	if content.IsArray() {
		content.ForEach(func(_, part gjson.Result) bool {
			switch part.Get("type").String() {
			case "text", "input_text", "output_text":
				changed = needsChange(part.Get("text"))
			}
			return !changed
		})
	}
	return changed
}

func neutralizeAntigravitySystemContent(raw json.RawMessage) (json.RawMessage, bool) {
	var text string
	if json.Unmarshal(raw, &text) == nil {
		if neutral := antigravity.NeutralizeSystemIdentity(text); neutral != text {
			out, _ := json.Marshal(neutral)
			return out, true
		}
		return raw, false
	}
	var parts []map[string]json.RawMessage
	if json.Unmarshal(raw, &parts) != nil {
		return raw, false
	}
	changed := false
	for _, part := range parts {
		var kind string
		_ = json.Unmarshal(part["type"], &kind)
		if kind != "text" && kind != "input_text" && kind != "output_text" {
			continue
		}
		var partText string
		if json.Unmarshal(part["text"], &partText) == nil {
			if neutral := antigravity.NeutralizeSystemIdentity(partText); neutral != partText {
				part["text"], _ = json.Marshal(neutral)
				changed = true
			}
		}
	}
	if !changed {
		return raw, false
	}
	out, _ := json.Marshal(parts)
	return out, true
}
