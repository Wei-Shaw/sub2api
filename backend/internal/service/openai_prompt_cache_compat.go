package service

import (
	"encoding/json"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/pkg/openai"
)

// sanitizeGPTPromptCacheHints removes explicit cache controls rejected by GPT
// upstreams. It runs after model mapping so aliases and future GPT models use
// the same compatibility policy while automatic prompt_cache_key caching stays
// available.
func sanitizeGPTPromptCacheHints(body []byte, upstreamModel string) ([]byte, bool, error) {
	model := openai.CanonicalizeOpenAIModelAliasSpelling(upstreamModel)
	if !strings.HasPrefix(model, "gpt-") {
		return body, false, nil
	}

	var request map[string]json.RawMessage
	if err := json.Unmarshal(body, &request); err != nil || request == nil {
		return body, false, nil
	}

	changed := false
	for _, field := range []string{"prompt_cache_options", "prompt_cache_breakpoint"} {
		if _, exists := request[field]; exists {
			delete(request, field)
			changed = true
		}
	}
	for _, field := range []string{"messages", "input"} {
		if cleaned, fieldChanged := stripPromptCacheBreakpointsFromMessages(request[field]); fieldChanged {
			request[field] = cleaned
			changed = true
		}
	}
	if !changed {
		return body, false, nil
	}

	cleaned, err := json.Marshal(request)
	if err != nil {
		return nil, false, err
	}
	return cleaned, true, nil
}

// Only protocol message and content positions are visited. Tool schemas,
// function arguments, metadata and arbitrary user JSON are intentionally left
// untouched even when they contain a field with the same name.
func stripPromptCacheBreakpointsFromMessages(raw json.RawMessage) (json.RawMessage, bool) {
	var items []json.RawMessage
	if json.Unmarshal(raw, &items) != nil {
		return raw, false
	}

	changed := false
	for i, item := range items {
		var message map[string]json.RawMessage
		if json.Unmarshal(item, &message) != nil || message == nil {
			continue
		}
		var itemType string
		_ = json.Unmarshal(message["type"], &itemType)
		if itemType != "" && itemType != "message" {
			continue
		}

		itemChanged := false
		if _, exists := message["prompt_cache_breakpoint"]; exists {
			delete(message, "prompt_cache_breakpoint")
			itemChanged = true
		}

		var content []json.RawMessage
		if json.Unmarshal(message["content"], &content) == nil {
			contentChanged := false
			for j, block := range content {
				var fields map[string]json.RawMessage
				if json.Unmarshal(block, &fields) != nil || fields == nil {
					continue
				}
				if _, exists := fields["prompt_cache_breakpoint"]; exists {
					delete(fields, "prompt_cache_breakpoint")
					content[j], _ = json.Marshal(fields)
					contentChanged = true
				}
			}
			if contentChanged {
				message["content"], _ = json.Marshal(content)
				itemChanged = true
			}
		}
		if itemChanged {
			items[i], _ = json.Marshal(message)
			changed = true
		}
	}
	if !changed {
		return raw, false
	}

	cleaned, _ := json.Marshal(items)
	return cleaned, true
}
