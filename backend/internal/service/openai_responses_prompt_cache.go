package service

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/pkg/apicompat"
)

const openAIResponsesPromptCacheKeyPrefix = "sub2api-pc-v1-"

type openAIResponsesPromptCacheSeed struct {
	APIKeyID          int64                          `json:"api_key_id"`
	Model             string                         `json:"model"`
	Tools             []apicompat.ResponsesTool      `json:"tools,omitempty"`
	ToolChoice        json.RawMessage                `json:"tool_choice,omitempty"`
	ParallelToolCalls *bool                          `json:"parallel_tool_calls,omitempty"`
	Reasoning         *apicompat.ResponsesReasoning  `json:"reasoning,omitempty"`
	Text              *apicompat.ResponsesText       `json:"text,omitempty"`
	Input             []apicompat.ResponsesInputItem `json:"input"`
}

func deriveOpenAIResponsesPromptCacheKey(
	apiKeyID int64,
	model string,
	req *apicompat.ResponsesRequest,
) (string, bool, error) {
	if apiKeyID <= 0 {
		return "", false, fmt.Errorf("derive prompt cache key: downstream API key ID is required")
	}
	model = strings.TrimSpace(model)
	if model == "" {
		return "", false, fmt.Errorf("derive prompt cache key: mapped model is required")
	}
	if req == nil {
		return "", false, fmt.Errorf("derive prompt cache key: responses request is required")
	}

	prefix, found, err := responsesInputThroughFirstExplicitPromptCacheBreakpoint(req.Input)
	if err != nil {
		return "", false, err
	}
	if !found {
		return "", false, nil
	}
	seed, err := json.Marshal(openAIResponsesPromptCacheSeed{
		APIKeyID:          apiKeyID,
		Model:             strings.ToLower(model),
		Tools:             req.Tools,
		ToolChoice:        req.ToolChoice,
		ParallelToolCalls: req.ParallelToolCalls,
		Reasoning:         req.Reasoning,
		Text:              req.Text,
		Input:             prefix,
	})
	if err != nil {
		return "", false, fmt.Errorf("derive prompt cache key: marshal canonical prefix: %w", err)
	}
	digest := sha256.Sum256(seed)
	return fmt.Sprintf("%s%x", openAIResponsesPromptCacheKeyPrefix, digest[:16]), true, nil
}

func removeOpenAIResponsesPromptCacheBreakpoints(req *apicompat.ResponsesRequest) error {
	if req == nil {
		return fmt.Errorf("remove prompt cache breakpoints: responses request is required")
	}

	var items []apicompat.ResponsesInputItem
	if err := json.Unmarshal(req.Input, &items); err != nil {
		return fmt.Errorf("remove prompt cache breakpoints: decode responses input: %w", err)
	}
	changed := false
	for itemIndex := range items {
		item := &items[itemIndex]
		if item.Type != "message" || len(item.Content) == 0 {
			continue
		}

		var parts []apicompat.ResponsesContentPart
		if err := json.Unmarshal(item.Content, &parts); err != nil {
			return fmt.Errorf("remove prompt cache breakpoints: decode message content: %w", err)
		}
		contentChanged := false
		for partIndex := range parts {
			if parts[partIndex].PromptCacheBreakpoint == nil {
				continue
			}
			parts[partIndex].PromptCacheBreakpoint = nil
			contentChanged = true
			changed = true
		}
		if !contentChanged {
			continue
		}
		content, err := json.Marshal(parts)
		if err != nil {
			return fmt.Errorf("remove prompt cache breakpoints: marshal message content: %w", err)
		}
		item.Content = content
	}
	if !changed {
		return nil
	}

	input, err := json.Marshal(items)
	if err != nil {
		return fmt.Errorf("remove prompt cache breakpoints: marshal responses input: %w", err)
	}
	req.Input = input
	return nil
}

func responsesInputThroughFirstExplicitPromptCacheBreakpoint(raw json.RawMessage) ([]apicompat.ResponsesInputItem, bool, error) {
	var items []apicompat.ResponsesInputItem
	if err := json.Unmarshal(raw, &items); err != nil {
		return nil, false, fmt.Errorf("derive prompt cache key: decode responses input: %w", err)
	}

	prefix := make([]apicompat.ResponsesInputItem, 0, len(items))
	for _, item := range items {
		if item.Type != "message" || len(item.Content) == 0 {
			prefix = append(prefix, item)
			continue
		}

		var parts []apicompat.ResponsesContentPart
		if err := json.Unmarshal(item.Content, &parts); err != nil {
			return nil, false, fmt.Errorf("derive prompt cache key: decode message content: %w", err)
		}
		for index, part := range parts {
			if part.PromptCacheBreakpoint == nil {
				continue
			}
			if part.PromptCacheBreakpoint.Mode != "explicit" {
				return nil, false, fmt.Errorf("derive prompt cache key: unsupported breakpoint mode %q", part.PromptCacheBreakpoint.Mode)
			}
			switch part.Type {
			case "input_text", "input_image", "input_file":
			default:
				return nil, false, fmt.Errorf("derive prompt cache key: unsupported breakpoint content type %q", part.Type)
			}

			truncated := item
			content, err := json.Marshal(parts[:index+1])
			if err != nil {
				return nil, false, fmt.Errorf("derive prompt cache key: marshal breakpoint content: %w", err)
			}
			truncated.Content = content
			prefix = append(prefix, truncated)
			return prefix, true, nil
		}
		prefix = append(prefix, item)
	}
	return nil, false, nil
}
