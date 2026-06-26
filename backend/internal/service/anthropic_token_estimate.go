package service

import (
	"encoding/json"
	"strings"

	"github.com/tidwall/gjson"
)

func EstimateAnthropicMessagesTokens(reqBody []byte) int {
	total := 0

	system := gjson.GetBytes(reqBody, "system")
	switch {
	case system.Type == gjson.String:
		total += estimateTokensForText(system.String())
	case system.IsArray():
		system.ForEach(func(_, block gjson.Result) bool {
			total += estimateTokensForText(block.Get("text").String())
			return true
		})
	}

	gjson.GetBytes(reqBody, "messages").ForEach(func(_, message gjson.Result) bool {
		total += estimateAnthropicContentTokens(message.Get("content"))
		return true
	})

	gjson.GetBytes(reqBody, "tools").ForEach(func(_, tool gjson.Result) bool {
		total += estimateTokensForText(tool.Get("name").String())
		total += estimateTokensForText(tool.Get("description").String())
		if schema := tool.Get("input_schema"); schema.Exists() {
			total += estimateTokensForText(compactJSONForTokenEstimate(schema.Raw))
		}
		return true
	})

	if total < 0 {
		return 0
	}
	return total
}

func estimateAnthropicContentTokens(content gjson.Result) int {
	if content.Type == gjson.String {
		return estimateTokensForText(content.String())
	}
	if !content.IsArray() {
		return 0
	}

	total := 0
	content.ForEach(func(_, block gjson.Result) bool {
		switch block.Get("type").String() {
		case "text":
			total += estimateTokensForText(block.Get("text").String())
		case "tool_use":
			total += estimateTokensForText(block.Get("name").String())
			if input := block.Get("input"); input.Exists() {
				total += estimateTokensForText(compactJSONForTokenEstimate(input.Raw))
			}
		case "tool_result":
			total += estimateAnthropicContentTokens(block.Get("content"))
		}
		return true
	})
	return total
}

func compactJSONForTokenEstimate(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	var v any
	if err := json.Unmarshal([]byte(raw), &v); err != nil {
		return raw
	}
	out, err := json.Marshal(v)
	if err != nil {
		return raw
	}
	return string(out)
}
