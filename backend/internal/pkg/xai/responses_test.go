package xai

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNormalizeResponsesBodyMapDropsUnsupportedFieldsAndNormalizesTools(t *testing.T) {
	req := map[string]any{
		"model":                  "client-model",
		"stream":                 false,
		"previous_response_id":   "resp_123",
		"prompt_cache_retention": "24h",
		"safety_identifier":      "user-1",
		"stream_options":         map[string]any{"include_usage": true},
		"tool_choice":            "auto",
		"parallel_tool_calls":    true,
		"tools": []any{
			map[string]any{"type": "tool_search"},
			map[string]any{"type": "image_generation"},
			map[string]any{"type": "custom", "name": "apply_patch"},
			map[string]any{"type": "custom", "name": "lookup"},
			map[string]any{"type": "web_search", "external_web_access": true},
			map[string]any{
				"type": "namespace",
				"tools": []any{
					map[string]any{"type": "function", "name": "nested"},
				},
			},
		},
	}

	NormalizeResponsesBodyMap(req, "grok-4.3-fast", true, "session-1", false)

	require.Equal(t, "grok-4.3-fast", req["model"])
	require.Equal(t, true, req["stream"])
	require.Equal(t, "session-1", req["prompt_cache_key"])
	require.NotContains(t, req, "previous_response_id")
	require.NotContains(t, req, "prompt_cache_retention")
	require.NotContains(t, req, "safety_identifier")
	require.NotContains(t, req, "stream_options")

	tools := req["tools"].([]any)
	require.Len(t, tools, 3)

	customTool := tools[0].(map[string]any)
	require.Equal(t, "function", customTool["type"])
	require.Equal(t, "lookup", customTool["name"])
	require.Equal(t, map[string]any{}, customTool["parameters"])

	webSearchTool := tools[1].(map[string]any)
	require.Equal(t, "web_search", webSearchTool["type"])
	require.NotContains(t, webSearchTool, "external_web_access")

	nestedTool := tools[2].(map[string]any)
	require.Equal(t, "function", nestedTool["type"])
	require.Equal(t, "nested", nestedTool["name"])
	require.Equal(t, map[string]any{}, nestedTool["parameters"])
}

func TestNormalizeResponsesBodyMapDropsEmptyToolsAndToolChoice(t *testing.T) {
	req := map[string]any{
		"tools": []any{
			map[string]any{"type": "tool_search"},
			map[string]any{"type": "custom", "name": "apply_patch"},
		},
		"tool_choice":         "auto",
		"parallel_tool_calls": true,
	}

	NormalizeResponsesBodyMap(req, "grok-4.3", true, "", false)

	require.NotContains(t, req, "tools")
	require.NotContains(t, req, "tool_choice")
	require.NotContains(t, req, "parallel_tool_calls")
}

func TestNormalizeResponsesBodyMapFiltersReasoningForUnsupportedModel(t *testing.T) {
	req := map[string]any{
		"model":     "grok-code-fast-1",
		"reasoning": map[string]any{"effort": "high"},
		"include":   []any{"reasoning.encrypted_content", "output_text"},
	}

	NormalizeResponsesBodyMap(req, "", true, "", false)

	require.NotContains(t, req, "reasoning")
	require.Equal(t, []any{"output_text"}, req["include"])
}

func TestNormalizeResponsesBodyMapKeepsAndMergesReasoningForSupportedModel(t *testing.T) {
	req := map[string]any{
		"model":     "grok-4.3-fast",
		"reasoning": map[string]any{"effort": "high"},
		"input": []any{
			map[string]any{"type": "reasoning", "summary": []any{"a"}, "content": nil, "encrypted_content": nil},
			map[string]any{"type": "reasoning", "summary": []any{"b"}},
			map[string]any{"type": "message", "role": "user"},
		},
	}

	NormalizeResponsesBodyMap(req, "", true, "", false)

	require.Equal(t, map[string]any{"effort": "high"}, req["reasoning"])
	input := req["input"].([]any)
	require.Len(t, input, 2)
	reasoningItem := input[0].(map[string]any)
	require.Equal(t, []any{"a", "b"}, reasoningItem["summary"])
	require.NotContains(t, reasoningItem, "content")
	require.NotContains(t, reasoningItem, "encrypted_content")
}

func TestOutputItemCollectorPatchesCompletedEvent(t *testing.T) {
	collector := &OutputItemCollector{}

	_, patched := collector.ProcessData([]byte(`{"type":"response.output_item.done","output_index":1,"item":{"id":"item_b"}}`))
	require.False(t, patched)
	_, patched = collector.ProcessData([]byte(`{"type":"response.output_item.done","output_index":0,"item":{"id":"item_a"}}`))
	require.False(t, patched)

	data, patched := collector.ProcessData([]byte(`{"type":"response.completed","response":{"id":"resp_1","output":[]}}`))
	require.True(t, patched)

	var event map[string]any
	require.NoError(t, json.Unmarshal(data, &event))
	response := event["response"].(map[string]any)
	output := response["output"].([]any)
	require.Equal(t, "item_a", output[0].(map[string]any)["id"])
	require.Equal(t, "item_b", output[1].(map[string]any)["id"])
}

func TestNormalizeResponsesBodyMapKeepsGrokCLIFields(t *testing.T) {
	req := map[string]any{
		"previous_response_id":   "resp_123",
		"prompt_cache_retention": "24h",
		"safety_identifier":      "user-1",
		"stream_options":         map[string]any{"include_usage": true},
		"tools": []any{
			map[string]any{"type": "image_generation"},
		},
	}

	NormalizeResponsesBodyMap(req, "grok-build", true, "session-1", true)

	require.NotContains(t, req, "previous_response_id")
	require.Equal(t, "24h", req["prompt_cache_retention"])
	require.Equal(t, "user-1", req["safety_identifier"])
	require.Equal(t, map[string]any{"include_usage": true}, req["stream_options"])
	require.Len(t, req["tools"].([]any), 1)
}

func TestPatchSSEBodyWithOutputItems(t *testing.T) {
	body := strings.Join([]string{
		`event: response.output_item.done`,
		`data: {"type":"response.output_item.done","output_index":0,"item":{"id":"item_a"}}`,
		``,
		`event: response.completed`,
		`data: {"type":"response.completed","response":{"id":"resp_1"}}`,
		``,
	}, "\n")

	patched := PatchSSEBodyWithOutputItems([]byte(body))

	require.Contains(t, string(patched), `"output":[{"id":"item_a"}]`)
}
