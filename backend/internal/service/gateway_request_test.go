//go:build unit

package service

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/domain"
	"github.com/stretchr/testify/require"
)

func TestParseGatewayRequest(t *testing.T) {
	body := []byte(`{"model":"claude-3-7-sonnet","stream":true,"metadata":{"user_id":"session_123e4567-e89b-12d3-a456-426614174000"},"system":[{"type":"text","text":"hello","cache_control":{"type":"ephemeral"}}],"messages":[{"content":"hi"}]}`)
	parsed, err := ParseGatewayRequest(body, "")
	require.NoError(t, err)
	require.Equal(t, "claude-3-7-sonnet", parsed.Model)
	require.True(t, parsed.Stream)
	require.Equal(t, "session_123e4567-e89b-12d3-a456-426614174000", parsed.MetadataUserID)
	require.True(t, parsed.HasSystem)
	require.NotNil(t, parsed.System)
	require.Len(t, parsed.Messages, 1)
	require.False(t, parsed.ThinkingEnabled)
}

func TestParseGatewayRequest_ThinkingEnabled(t *testing.T) {
	body := []byte(`{"model":"claude-sonnet-4-5","thinking":{"type":"enabled"},"messages":[{"content":"hi"}]}`)
	parsed, err := ParseGatewayRequest(body, "")
	require.NoError(t, err)
	require.Equal(t, "claude-sonnet-4-5", parsed.Model)
	require.True(t, parsed.ThinkingEnabled)
}

func TestParseGatewayRequest_ThinkingAdaptiveEnabled(t *testing.T) {
	body := []byte(`{"model":"claude-sonnet-4-5","thinking":{"type":"adaptive"},"messages":[{"content":"hi"}]}`)
	parsed, err := ParseGatewayRequest(body, "")
	require.NoError(t, err)
	require.Equal(t, "claude-sonnet-4-5", parsed.Model)
	require.True(t, parsed.ThinkingEnabled)
}

func TestParseGatewayRequest_MaxTokens(t *testing.T) {
	body := []byte(`{"model":"claude-haiku-4-5","max_tokens":1}`)
	parsed, err := ParseGatewayRequest(body, "")
	require.NoError(t, err)
	require.Equal(t, 1, parsed.MaxTokens)
}

func TestParseGatewayRequest_MaxTokensNonIntegralIgnored(t *testing.T) {
	body := []byte(`{"model":"claude-haiku-4-5","max_tokens":1.5}`)
	parsed, err := ParseGatewayRequest(body, "")
	require.NoError(t, err)
	require.Equal(t, 0, parsed.MaxTokens)
}

func TestParseGatewayRequest_SystemNull(t *testing.T) {
	body := []byte(`{"model":"claude-3","system":null}`)
	parsed, err := ParseGatewayRequest(body, "")
	require.NoError(t, err)
	// 显式传入 system:null 也应视为“字段已存在”，避免默认 system 被注入。
	require.True(t, parsed.HasSystem)
	require.Nil(t, parsed.System)
}

func TestParseGatewayRequest_InvalidModelType(t *testing.T) {
	body := []byte(`{"model":123}`)
	_, err := ParseGatewayRequest(body, "")
	require.Error(t, err)
}

func TestParseGatewayRequest_InvalidStreamType(t *testing.T) {
	body := []byte(`{"stream":"true"}`)
	_, err := ParseGatewayRequest(body, "")
	require.Error(t, err)
}

// ============ Gemini 原生格式解析测试 ============

func TestParseGatewayRequest_GeminiContents(t *testing.T) {
	body := []byte(`{
		"contents": [
			{"role": "user", "parts": [{"text": "Hello"}]},
			{"role": "model", "parts": [{"text": "Hi there"}]},
			{"role": "user", "parts": [{"text": "How are you?"}]}
		]
	}`)
	parsed, err := ParseGatewayRequest(body, domain.PlatformGemini)
	require.NoError(t, err)
	require.Len(t, parsed.Messages, 3, "should parse contents as Messages")
	require.False(t, parsed.HasSystem, "Gemini format should not set HasSystem")
	require.Nil(t, parsed.System, "no systemInstruction means nil System")
}

func TestParseGatewayRequest_GeminiSystemInstruction(t *testing.T) {
	body := []byte(`{
		"systemInstruction": {
			"parts": [{"text": "You are a helpful assistant."}]
		},
		"contents": [
			{"role": "user", "parts": [{"text": "Hello"}]}
		]
	}`)
	parsed, err := ParseGatewayRequest(body, domain.PlatformGemini)
	require.NoError(t, err)
	require.NotNil(t, parsed.System, "should parse systemInstruction.parts as System")
	parts, ok := parsed.System.([]any)
	require.True(t, ok)
	require.Len(t, parts, 1)
	partMap, ok := parts[0].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "You are a helpful assistant.", partMap["text"])
	require.Len(t, parsed.Messages, 1)
}

func TestParseGatewayRequest_GeminiWithModel(t *testing.T) {
	body := []byte(`{
		"model": "gemini-2.5-pro",
		"contents": [{"role": "user", "parts": [{"text": "test"}]}]
	}`)
	parsed, err := ParseGatewayRequest(body, domain.PlatformGemini)
	require.NoError(t, err)
	require.Equal(t, "gemini-2.5-pro", parsed.Model)
	require.Len(t, parsed.Messages, 1)
}

func TestParseGatewayRequest_GeminiIgnoresAnthropicFields(t *testing.T) {
	// Gemini 格式下 system/messages 字段应被忽略
	body := []byte(`{
		"system": "should be ignored",
		"messages": [{"role": "user", "content": "ignored"}],
		"contents": [{"role": "user", "parts": [{"text": "real content"}]}]
	}`)
	parsed, err := ParseGatewayRequest(body, domain.PlatformGemini)
	require.NoError(t, err)
	require.False(t, parsed.HasSystem, "Gemini protocol should not parse Anthropic system field")
	require.Nil(t, parsed.System, "no systemInstruction = nil System")
	require.Len(t, parsed.Messages, 1, "should use contents, not messages")
}

func TestParseGatewayRequest_GeminiEmptyContents(t *testing.T) {
	body := []byte(`{"contents": []}`)
	parsed, err := ParseGatewayRequest(body, domain.PlatformGemini)
	require.NoError(t, err)
	require.Empty(t, parsed.Messages)
}

func TestParseGatewayRequest_GeminiNoContents(t *testing.T) {
	body := []byte(`{"model": "gemini-2.5-flash"}`)
	parsed, err := ParseGatewayRequest(body, domain.PlatformGemini)
	require.NoError(t, err)
	require.Nil(t, parsed.Messages)
	require.Equal(t, "gemini-2.5-flash", parsed.Model)
}

func TestParseGatewayRequest_AnthropicIgnoresGeminiFields(t *testing.T) {
	// Anthropic 格式下 contents/systemInstruction 字段应被忽略
	body := []byte(`{
		"system": "real system",
		"messages": [{"role": "user", "content": "real content"}],
		"contents": [{"role": "user", "parts": [{"text": "ignored"}]}],
		"systemInstruction": {"parts": [{"text": "ignored"}]}
	}`)
	parsed, err := ParseGatewayRequest(body, domain.PlatformAnthropic)
	require.NoError(t, err)
	require.True(t, parsed.HasSystem)
	require.Equal(t, "real system", parsed.System)
	require.Len(t, parsed.Messages, 1)
	msg, ok := parsed.Messages[0].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "real content", msg["content"])
}

func TestFilterThinkingBlocks(t *testing.T) {
	containsThinkingBlock := func(body []byte) bool {
		var req map[string]any
		if err := json.Unmarshal(body, &req); err != nil {
			return false
		}
		messages, ok := req["messages"].([]any)
		if !ok {
			return false
		}
		for _, msg := range messages {
			msgMap, ok := msg.(map[string]any)
			if !ok {
				continue
			}
			content, ok := msgMap["content"].([]any)
			if !ok {
				continue
			}
			for _, block := range content {
				blockMap, ok := block.(map[string]any)
				if !ok {
					continue
				}
				blockType, _ := blockMap["type"].(string)
				if blockType == "thinking" {
					return true
				}
				if blockType == "" {
					if _, hasThinking := blockMap["thinking"]; hasThinking {
						return true
					}
				}
			}
		}
		return false
	}

	tests := []struct {
		name         string
		input        string
		shouldFilter bool
		expectError  bool
	}{
		{
			name:         "filters thinking blocks",
			input:        `{"model":"claude-3-5-sonnet-20241022","messages":[{"role":"user","content":[{"type":"text","text":"Hello"},{"type":"thinking","thinking":"internal","signature":"invalid"},{"type":"text","text":"World"}]}]}`,
			shouldFilter: true,
		},
		{
			name:         "does not filter signed thinking blocks when thinking adaptive",
			input:        `{"thinking":{"type":"adaptive"},"messages":[{"role":"assistant","content":[{"type":"thinking","thinking":"ok","signature":"sig_real_123"},{"type":"text","text":"B"}]}]}`,
			shouldFilter: false,
		},
		{
			name:         "filters unsigned thinking blocks when thinking adaptive",
			input:        `{"thinking":{"type":"adaptive"},"messages":[{"role":"assistant","content":[{"type":"thinking","thinking":"internal","signature":""},{"type":"text","text":"B"}]}]}`,
			shouldFilter: true,
		},
		{
			name:         "handles no thinking blocks",
			input:        `{"model":"claude-3-5-sonnet-20241022","messages":[{"role":"user","content":[{"type":"text","text":"Hello"}]}]}`,
			shouldFilter: false,
		},
		{
			name:         "handles invalid JSON gracefully",
			input:        `{invalid json`,
			shouldFilter: false,
			expectError:  true,
		},
		{
			name:         "handles multiple messages with thinking blocks",
			input:        `{"messages":[{"role":"user","content":[{"type":"text","text":"A"}]},{"role":"assistant","content":[{"type":"thinking","thinking":"think"},{"type":"text","text":"B"}]}]}`,
			shouldFilter: true,
		},
		{
			name:         "filters thinking blocks without type discriminator",
			input:        `{"messages":[{"role":"assistant","content":[{"thinking":{"text":"internal"}},{"type":"text","text":"B"}]}]}`,
			shouldFilter: true,
		},
		{
			name:         "does not filter tool_use input fields named thinking",
			input:        `{"messages":[{"role":"user","content":[{"type":"tool_use","id":"t1","name":"foo","input":{"thinking":"keepme","x":1}},{"type":"text","text":"Hello"}]}]}`,
			shouldFilter: false,
		},
		{
			name:         "handles empty messages array",
			input:        `{"messages":[]}`,
			shouldFilter: false,
		},
		{
			name:         "handles missing messages field",
			input:        `{"model":"claude-3"}`,
			shouldFilter: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := FilterThinkingBlocks([]byte(tt.input))

			if tt.expectError {
				// For invalid JSON, should return original
				require.Equal(t, tt.input, string(result))
				return
			}

			if tt.shouldFilter {
				require.False(t, containsThinkingBlock(result))
			} else {
				// Ensure we don't rewrite JSON when no filtering is needed.
				require.Equal(t, tt.input, string(result))
			}

			// Verify valid JSON returned (unless input was invalid)
			var parsed map[string]any
			err := json.Unmarshal(result, &parsed)
			require.NoError(t, err)
		})
	}
}

func TestFilterThinkingBlocksForRetry_DisablesThinkingAndPreservesAsText(t *testing.T) {
	input := []byte(`{
		"model":"claude-3-5-sonnet-20241022",
		"thinking":{"type":"enabled","budget_tokens":1024},
		"messages":[
			{"role":"user","content":[{"type":"text","text":"Hi"}]},
			{"role":"assistant","content":[
				{"type":"thinking","thinking":"Let me think...","signature":"bad_sig"},
				{"type":"text","text":"Answer"}
			]}
		]
	}`)

	out := FilterThinkingBlocksForRetry(input)

	var req map[string]any
	require.NoError(t, json.Unmarshal(out, &req))
	_, hasThinking := req["thinking"]
	require.False(t, hasThinking)

	msgs, ok := req["messages"].([]any)
	require.True(t, ok)
	require.Len(t, msgs, 2)

	assistant, ok := msgs[1].(map[string]any)
	require.True(t, ok)
	content, ok := assistant["content"].([]any)
	require.True(t, ok)
	require.Len(t, content, 2)

	first, ok := content[0].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "text", first["type"])
	require.Equal(t, "Let me think...", first["text"])
}

func TestFilterThinkingBlocksForRetry_DisablesThinkingEvenWithoutThinkingBlocks(t *testing.T) {
	input := []byte(`{
		"model":"claude-3-5-sonnet-20241022",
		"thinking":{"type":"enabled","budget_tokens":1024},
		"messages":[
			{"role":"user","content":[{"type":"text","text":"Hi"}]},
			{"role":"assistant","content":[{"type":"text","text":"Prefill"}]}
		]
	}`)

	out := FilterThinkingBlocksForRetry(input)

	var req map[string]any
	require.NoError(t, json.Unmarshal(out, &req))
	_, hasThinking := req["thinking"]
	require.False(t, hasThinking)
}

func TestFilterThinkingBlocksForRetry_RemovesRedactedThinkingAndKeepsValidContent(t *testing.T) {
	input := []byte(`{
		"thinking":{"type":"enabled","budget_tokens":1024},
		"messages":[
			{"role":"assistant","content":[
				{"type":"redacted_thinking","data":"..."},
				{"type":"text","text":"Visible"}
			]}
		]
	}`)

	out := FilterThinkingBlocksForRetry(input)

	var req map[string]any
	require.NoError(t, json.Unmarshal(out, &req))
	_, hasThinking := req["thinking"]
	require.False(t, hasThinking)

	msgs, ok := req["messages"].([]any)
	require.True(t, ok)
	msg0, ok := msgs[0].(map[string]any)
	require.True(t, ok)
	content, ok := msg0["content"].([]any)
	require.True(t, ok)
	require.Len(t, content, 1)
	content0, ok := content[0].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "text", content0["type"])
	require.Equal(t, "Visible", content0["text"])
}

func TestFilterThinkingBlocksForRetry_EmptyContentGetsPlaceholder(t *testing.T) {
	input := []byte(`{
		"thinking":{"type":"enabled"},
		"messages":[
			{"role":"assistant","content":[{"type":"redacted_thinking","data":"..."}]}
		]
	}`)

	out := FilterThinkingBlocksForRetry(input)

	var req map[string]any
	require.NoError(t, json.Unmarshal(out, &req))
	msgs, ok := req["messages"].([]any)
	require.True(t, ok)
	msg0, ok := msgs[0].(map[string]any)
	require.True(t, ok)
	content, ok := msg0["content"].([]any)
	require.True(t, ok)
	require.Len(t, content, 1)
	content0, ok := content[0].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "text", content0["type"])
	require.NotEmpty(t, content0["text"])
}

func TestFilterThinkingBlocksForRetry_StripsEmptyTextBlocks(t *testing.T) {
	// Empty text blocks cause upstream 400: "text content blocks must be non-empty"
	input := []byte(`{
		"messages":[
			{"role":"user","content":[{"type":"text","text":"hello"},{"type":"text","text":""}]},
			{"role":"assistant","content":[{"type":"text","text":""}]}
		]
	}`)

	out := FilterThinkingBlocksForRetry(input)

	var req map[string]any
	require.NoError(t, json.Unmarshal(out, &req))
	msgs, ok := req["messages"].([]any)
	require.True(t, ok)

	// First message: empty text block stripped, "hello" preserved
	msg0 := msgs[0].(map[string]any)
	content0 := msg0["content"].([]any)
	require.Len(t, content0, 1)
	require.Equal(t, "hello", content0[0].(map[string]any)["text"])

	// Second message: only had empty text block → gets placeholder
	msg1 := msgs[1].(map[string]any)
	content1 := msg1["content"].([]any)
	require.Len(t, content1, 1)
	block1 := content1[0].(map[string]any)
	require.Equal(t, "text", block1["type"])
	require.NotEmpty(t, block1["text"])
}

func TestFilterThinkingBlocksForRetry_StripsNestedEmptyTextInToolResult(t *testing.T) {
	// Empty text blocks nested inside tool_result content should also be stripped
	input := []byte(`{
		"messages":[
			{"role":"user","content":[
				{"type":"tool_result","tool_use_id":"t1","content":[
					{"type":"text","text":"valid result"},
					{"type":"text","text":""}
				]}
			]}
		]
	}`)

	out := FilterThinkingBlocksForRetry(input)

	var req map[string]any
	require.NoError(t, json.Unmarshal(out, &req))
	msgs := req["messages"].([]any)
	msg0 := msgs[0].(map[string]any)
	content0 := msg0["content"].([]any)
	require.Len(t, content0, 1)
	toolResult := content0[0].(map[string]any)
	require.Equal(t, "tool_result", toolResult["type"])
	nestedContent := toolResult["content"].([]any)
	require.Len(t, nestedContent, 1)
	require.Equal(t, "valid result", nestedContent[0].(map[string]any)["text"])
}

func TestFilterThinkingBlocksForRetry_NestedAllEmptyGetsEmptySlice(t *testing.T) {
	// If all nested content blocks in tool_result are empty text, content becomes empty slice
	input := []byte(`{
		"messages":[
			{"role":"user","content":[
				{"type":"tool_result","tool_use_id":"t1","content":[
					{"type":"text","text":""}
				]},
				{"type":"text","text":"hello"}
			]}
		]
	}`)

	out := FilterThinkingBlocksForRetry(input)

	var req map[string]any
	require.NoError(t, json.Unmarshal(out, &req))
	msgs := req["messages"].([]any)
	msg0 := msgs[0].(map[string]any)
	content0 := msg0["content"].([]any)
	require.Len(t, content0, 2)
	toolResult := content0[0].(map[string]any)
	nestedContent := toolResult["content"].([]any)
	require.Len(t, nestedContent, 0)
}

func TestStripEmptyTextBlocks(t *testing.T) {
	t.Run("strips top-level empty text", func(t *testing.T) {
		input := []byte(`{"messages":[{"role":"user","content":[{"type":"text","text":"hello"},{"type":"text","text":""}]}]}`)
		out := StripEmptyTextBlocks(input)
		var req map[string]any
		require.NoError(t, json.Unmarshal(out, &req))
		msgs := req["messages"].([]any)
		content := msgs[0].(map[string]any)["content"].([]any)
		require.Len(t, content, 1)
		require.Equal(t, "hello", content[0].(map[string]any)["text"])
	})

	t.Run("strips nested empty text in tool_result", func(t *testing.T) {
		input := []byte(`{"messages":[{"role":"user","content":[{"type":"tool_result","tool_use_id":"t1","content":[{"type":"text","text":"ok"},{"type":"text","text":""}]}]}]}`)
		out := StripEmptyTextBlocks(input)
		var req map[string]any
		require.NoError(t, json.Unmarshal(out, &req))
		msgs := req["messages"].([]any)
		content := msgs[0].(map[string]any)["content"].([]any)
		toolResult := content[0].(map[string]any)
		nestedContent := toolResult["content"].([]any)
		require.Len(t, nestedContent, 1)
		require.Equal(t, "ok", nestedContent[0].(map[string]any)["text"])
	})

	t.Run("no-op when no empty text", func(t *testing.T) {
		input := []byte(`{"messages":[{"role":"user","content":[{"type":"text","text":"hello"}]}]}`)
		out := StripEmptyTextBlocks(input)
		require.Equal(t, input, out)
	})

	t.Run("preserves non-map blocks in content", func(t *testing.T) {
		// tool_result content can be a string; non-map blocks should pass through unchanged
		input := []byte(`{"messages":[{"role":"user","content":[{"type":"tool_result","tool_use_id":"t1","content":"string content"},{"type":"text","text":""}]}]}`)
		out := StripEmptyTextBlocks(input)
		var req map[string]any
		require.NoError(t, json.Unmarshal(out, &req))
		msgs := req["messages"].([]any)
		content := msgs[0].(map[string]any)["content"].([]any)
		require.Len(t, content, 1)
		toolResult := content[0].(map[string]any)
		require.Equal(t, "tool_result", toolResult["type"])
		require.Equal(t, "string content", toolResult["content"])
	})

	t.Run("handles deeply nested tool_result", func(t *testing.T) {
		// Recursive: tool_result containing another tool_result with empty text
		input := []byte(`{"messages":[{"role":"user","content":[{"type":"tool_result","tool_use_id":"t1","content":[{"type":"tool_result","tool_use_id":"t2","content":[{"type":"text","text":""},{"type":"text","text":"deep"}]}]}]}]}`)
		out := StripEmptyTextBlocks(input)
		var req map[string]any
		require.NoError(t, json.Unmarshal(out, &req))
		msgs := req["messages"].([]any)
		content := msgs[0].(map[string]any)["content"].([]any)
		outer := content[0].(map[string]any)
		innerContent := outer["content"].([]any)
		inner := innerContent[0].(map[string]any)
		deepContent := inner["content"].([]any)
		require.Len(t, deepContent, 1)
		require.Equal(t, "deep", deepContent[0].(map[string]any)["text"])
	})
}

func TestFilterThinkingBlocksForRetry_PreservesNonEmptyTextBlocks(t *testing.T) {
	// Non-empty text blocks should pass through unchanged
	input := []byte(`{
		"messages":[
			{"role":"user","content":[{"type":"text","text":"hello"},{"type":"text","text":"world"}]}
		]
	}`)

	out := FilterThinkingBlocksForRetry(input)

	// Fast path: no thinking content, no empty content, no empty text blocks → unchanged
	require.Equal(t, input, out)
}

func TestFilterSignatureSensitiveBlocksForRetry_DowngradesTools(t *testing.T) {
	input := []byte(`{
		"thinking":{"type":"enabled","budget_tokens":1024},
		"messages":[
			{"role":"assistant","content":[
				{"type":"tool_use","id":"t1","name":"Bash","input":{"command":"ls"}},
				{"type":"tool_result","tool_use_id":"t1","content":"ok","is_error":false}
			]}
		]
	}`)

	out := FilterSignatureSensitiveBlocksForRetry(input)

	var req map[string]any
	require.NoError(t, json.Unmarshal(out, &req))
	_, hasThinking := req["thinking"]
	require.False(t, hasThinking)

	msgs, ok := req["messages"].([]any)
	require.True(t, ok)
	msg0, ok := msgs[0].(map[string]any)
	require.True(t, ok)
	content, ok := msg0["content"].([]any)
	require.True(t, ok)
	require.Len(t, content, 2)
	content0, ok := content[0].(map[string]any)
	require.True(t, ok)
	content1, ok := content[1].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "text", content0["type"])
	require.Equal(t, "text", content1["type"])
	require.Contains(t, content0["text"], "tool_use")
	require.Contains(t, content1["text"], "tool_result")
}

// ============ Group 6b: context_management.edits 清理测试 ============

// removeThinkingDependentContextStrategies — 边界用例

func TestRemoveThinkingDependentContextStrategies_NoContextManagement(t *testing.T) {
	input := []byte(`{"thinking":{"type":"enabled"},"messages":[]}`)
	out := removeThinkingDependentContextStrategies(input)
	require.Equal(t, input, out, "无 context_management 字段时应原样返回")
}

func TestRemoveThinkingDependentContextStrategies_EmptyEdits(t *testing.T) {
	input := []byte(`{"context_management":{"edits":[]},"messages":[]}`)
	out := removeThinkingDependentContextStrategies(input)
	require.Equal(t, input, out, "edits 为空数组时应原样返回")
}

func TestRemoveThinkingDependentContextStrategies_NoClearThinkingEntry(t *testing.T) {
	input := []byte(`{"context_management":{"edits":[{"type":"other_strategy"}]},"messages":[]}`)
	out := removeThinkingDependentContextStrategies(input)
	require.Equal(t, input, out, "edits 中无 clear_thinking_20251015 时应原样返回")
}

func TestRemoveThinkingDependentContextStrategies_RemovesSingleEntry(t *testing.T) {
	input := []byte(`{"context_management":{"edits":[{"type":"clear_thinking_20251015"}]},"messages":[]}`)
	out := removeThinkingDependentContextStrategies(input)

	var req map[string]any
	require.NoError(t, json.Unmarshal(out, &req))
	cm, ok := req["context_management"].(map[string]any)
	require.True(t, ok)
	_, hasEdits := cm["edits"]
	require.False(t, hasEdits, "所有 edits 均为 clear_thinking_20251015 时应删除 edits 键")
}

func TestRemoveThinkingDependentContextStrategies_MixedEntries(t *testing.T) {
	input := []byte(`{"context_management":{"edits":[{"type":"clear_thinking_20251015"},{"type":"other_strategy","param":1}]},"messages":[]}`)
	out := removeThinkingDependentContextStrategies(input)

	var req map[string]any
	require.NoError(t, json.Unmarshal(out, &req))
	cm, ok := req["context_management"].(map[string]any)
	require.True(t, ok)
	edits, ok := cm["edits"].([]any)
	require.True(t, ok)
	require.Len(t, edits, 1, "仅移除 clear_thinking_20251015，保留其他条目")
	edit0, ok := edits[0].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "other_strategy", edit0["type"])
}

// FilterThinkingBlocksForRetry — 包含 context_management 的场景

func TestFilterThinkingBlocksForRetry_RemovesClearThinkingStrategy_FastPath(t *testing.T) {
	// 快速路径：messages 中无 thinking 块，仅有顶层 thinking 字段
	// 这条路径曾因提前 return 跳过 removeThinkingDependentContextStrategies 而存在 bug
	input := []byte(`{
		"thinking":{"type":"enabled","budget_tokens":1024},
		"context_management":{"edits":[{"type":"clear_thinking_20251015"}]},
		"messages":[
			{"role":"user","content":[{"type":"text","text":"Hello"}]}
		]
	}`)

	out := FilterThinkingBlocksForRetry(input)

	var req map[string]any
	require.NoError(t, json.Unmarshal(out, &req))
	_, hasThinking := req["thinking"]
	require.False(t, hasThinking, "顶层 thinking 应被移除")

	cm, ok := req["context_management"].(map[string]any)
	require.True(t, ok)
	_, hasEdits := cm["edits"]
	require.False(t, hasEdits, "fast path 下 clear_thinking_20251015 应被移除，edits 键应被删除")
}

func TestFilterThinkingBlocksForRetry_RemovesClearThinkingStrategy_WithThinkingBlocks(t *testing.T) {
	// 完整路径：messages 中有 thinking 块（非 fast path）
	input := []byte(`{
		"thinking":{"type":"enabled","budget_tokens":1024},
		"context_management":{"edits":[{"type":"clear_thinking_20251015"},{"type":"keep_this"}]},
		"messages":[
			{"role":"assistant","content":[
				{"type":"thinking","thinking":"some thought","signature":"sig"},
				{"type":"text","text":"Answer"}
			]}
		]
	}`)

	out := FilterThinkingBlocksForRetry(input)

	var req map[string]any
	require.NoError(t, json.Unmarshal(out, &req))
	_, hasThinking := req["thinking"]
	require.False(t, hasThinking, "顶层 thinking 应被移除")

	cm, ok := req["context_management"].(map[string]any)
	require.True(t, ok)
	edits, ok := cm["edits"].([]any)
	require.True(t, ok)
	require.Len(t, edits, 1, "仅移除 clear_thinking_20251015，保留 keep_this")
	edit0, ok := edits[0].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "keep_this", edit0["type"])
}

func TestFilterThinkingBlocksForRetry_NoContextManagement_Unaffected(t *testing.T) {
	// 无 context_management 时不应报错，且 thinking 正常被移除
	input := []byte(`{
		"thinking":{"type":"enabled"},
		"messages":[{"role":"user","content":[{"type":"text","text":"Hi"}]}]
	}`)

	out := FilterThinkingBlocksForRetry(input)

	var req map[string]any
	require.NoError(t, json.Unmarshal(out, &req))
	_, hasThinking := req["thinking"]
	require.False(t, hasThinking)
	_, hasCM := req["context_management"]
	require.False(t, hasCM)
}

// FilterSignatureSensitiveBlocksForRetry — 包含 context_management 的场景

func TestFilterSignatureSensitiveBlocksForRetry_RemovesClearThinkingStrategy(t *testing.T) {
	input := []byte(`{
		"thinking":{"type":"enabled","budget_tokens":1024},
		"context_management":{"edits":[{"type":"clear_thinking_20251015"}]},
		"messages":[
			{"role":"assistant","content":[
				{"type":"thinking","thinking":"thought","signature":"sig"}
			]}
		]
	}`)

	out := FilterSignatureSensitiveBlocksForRetry(input)

	var req map[string]any
	require.NoError(t, json.Unmarshal(out, &req))
	_, hasThinking := req["thinking"]
	require.False(t, hasThinking, "顶层 thinking 应被移除")

	cm, ok := req["context_management"].(map[string]any)
	require.True(t, ok)
	if rawEdits, hasEdits := cm["edits"]; hasEdits {
		edits, ok := rawEdits.([]any)
		require.True(t, ok)
		for _, e := range edits {
			em, ok := e.(map[string]any)
			require.True(t, ok)
			require.NotEqual(t, "clear_thinking_20251015", em["type"], "clear_thinking_20251015 应被移除")
		}
	}
}

func TestFilterSignatureSensitiveBlocksForRetry_PreservesNonThinkingStrategies(t *testing.T) {
	input := []byte(`{
		"thinking":{"type":"enabled"},
		"context_management":{"edits":[{"type":"clear_thinking_20251015"},{"type":"other_edit"}]},
		"messages":[
			{"role":"assistant","content":[
				{"type":"thinking","thinking":"t","signature":"s"}
			]}
		]
	}`)

	out := FilterSignatureSensitiveBlocksForRetry(input)

	var req map[string]any
	require.NoError(t, json.Unmarshal(out, &req))

	cm, ok := req["context_management"].(map[string]any)
	require.True(t, ok)
	edits, ok := cm["edits"].([]any)
	require.True(t, ok)
	require.Len(t, edits, 1, "仅移除 clear_thinking_20251015，保留 other_edit")
	edit0, ok := edits[0].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "other_edit", edit0["type"])
}

func TestFilterSignatureSensitiveBlocksForRetry_NoThinkingField_ContextManagementUntouched(t *testing.T) {
	// 没有顶层 thinking 字段时，context_management 不应被修改
	input := []byte(`{
		"context_management":{"edits":[{"type":"clear_thinking_20251015"}]},
		"messages":[
			{"role":"assistant","content":[
				{"type":"thinking","thinking":"t","signature":"s"}
			]}
		]
	}`)

	out := FilterSignatureSensitiveBlocksForRetry(input)

	var req map[string]any
	require.NoError(t, json.Unmarshal(out, &req))
	cm, ok := req["context_management"].(map[string]any)
	require.True(t, ok)
	edits, ok := cm["edits"].([]any)
	require.True(t, ok)
	require.Len(t, edits, 1, "无顶层 thinking 时 context_management 不应被修改")
}

// ============ Group 7: ParseGatewayRequest 补充单元测试 ============

// Task 7.1 — 类型校验边界测试
func TestParseGatewayRequest_TypeValidation(t *testing.T) {
	tests := []struct {
		name      string
		body      string
		wantErr   bool
		errSubstr string // 期望的错误信息子串（为空则不检查）
	}{
		{
			name:      "model 为 int",
			body:      `{"model":123}`,
			wantErr:   true,
			errSubstr: "invalid model field type",
		},
		{
			name:      "model 为 array",
			body:      `{"model":[]}`,
			wantErr:   true,
			errSubstr: "invalid model field type",
		},
		{
			name:      "model 为 bool",
			body:      `{"model":true}`,
			wantErr:   true,
			errSubstr: "invalid model field type",
		},
		{
			name:      "model 为 null — gjson Null 类型触发类型校验错误",
			body:      `{"model":null}`,
			wantErr:   true, // gjson: Exists()=true, Type=Null != String → 返回错误
			errSubstr: "invalid model field type",
		},
		{
			name:      "stream 为 string",
			body:      `{"stream":"true"}`,
			wantErr:   true,
			errSubstr: "invalid stream field type",
		},
		{
			name:      "stream 为 int",
			body:      `{"stream":1}`,
			wantErr:   true,
			errSubstr: "invalid stream field type",
		},
		{
			name:      "stream 为 null — gjson Null 类型触发类型校验错误",
			body:      `{"stream":null}`,
			wantErr:   true, // gjson: Exists()=true, Type=Null != True && != False → 返回错误
			errSubstr: "invalid stream field type",
		},
		{
			name:      "model 为 object",
			body:      `{"model":{}}`,
			wantErr:   true,
			errSubstr: "invalid model field type",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ParseGatewayRequest([]byte(tt.body), "")
			if tt.wantErr {
				require.Error(t, err)
				if tt.errSubstr != "" {
					require.Contains(t, err.Error(), tt.errSubstr)
				}
			} else {
				require.NoError(t, err)
			}
		})
	}
}

// Task 7.2 — 可选字段缺失测试
func TestParseGatewayRequest_OptionalFieldsMissing(t *testing.T) {
	tests := []struct {
		name            string
		body            string
		wantModel       string
		wantStream      bool
		wantMetadataUID string
		wantHasSystem   bool
		wantThinking    bool
		wantMaxTokens   int
		wantMessagesNil bool
		wantMessagesLen int
	}{
		{
			name:            "完全空 JSON — 所有字段零值",
			body:            `{}`,
			wantModel:       "",
			wantStream:      false,
			wantMetadataUID: "",
			wantHasSystem:   false,
			wantThinking:    false,
			wantMaxTokens:   0,
			wantMessagesNil: true,
		},
		{
			name:            "metadata 无 user_id",
			body:            `{"model":"test"}`,
			wantModel:       "test",
			wantMetadataUID: "",
			wantHasSystem:   false,
			wantThinking:    false,
		},
		{
			name:         "thinking 非 enabled（type=disabled）",
			body:         `{"model":"test","thinking":{"type":"disabled"}}`,
			wantModel:    "test",
			wantThinking: false,
		},
		{
			name:         "thinking 字段缺失",
			body:         `{"model":"test"}`,
			wantModel:    "test",
			wantThinking: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parsed, err := ParseGatewayRequest([]byte(tt.body), "")
			require.NoError(t, err)

			require.Equal(t, tt.wantModel, parsed.Model)
			require.Equal(t, tt.wantStream, parsed.Stream)
			require.Equal(t, tt.wantMetadataUID, parsed.MetadataUserID)
			require.Equal(t, tt.wantHasSystem, parsed.HasSystem)
			require.Equal(t, tt.wantThinking, parsed.ThinkingEnabled)
			require.Equal(t, tt.wantMaxTokens, parsed.MaxTokens)

			if tt.wantMessagesNil {
				require.Nil(t, parsed.Messages)
			}
			if tt.wantMessagesLen > 0 {
				require.Len(t, parsed.Messages, tt.wantMessagesLen)
			}
		})
	}
}

// Task 7.3 — Gemini 协议分支测试
// 已有测试覆盖：
// - TestParseGatewayRequest_GeminiSystemInstruction: 正常 systemInstruction+contents
// - TestParseGatewayRequest_GeminiNoContents: 缺失 contents
// - TestParseGatewayRequest_GeminiContents: 正常 contents（无 systemInstruction）
// 因此跳过。

// Task 7.4 — max_tokens 边界测试
func TestParseGatewayRequest_MaxTokensBoundary(t *testing.T) {
	tests := []struct {
		name          string
		body          string
		wantMaxTokens int
		wantErr       bool
	}{
		{
			name:          "正常整数",
			body:          `{"max_tokens":1024}`,
			wantMaxTokens: 1024,
		},
		{
			name:          "浮点数（非整数）被忽略",
			body:          `{"max_tokens":10.5}`,
			wantMaxTokens: 0,
		},
		{
			name:          "负整数可以通过",
			body:          `{"max_tokens":-1}`,
			wantMaxTokens: -1,
		},
		{
			name:          "超大值不 panic",
			body:          `{"max_tokens":9999999999999999}`,
			wantMaxTokens: 10000000000000000, // float64 精度导致 9999999999999999 → 1e16
		},
		{
			name:          "null 值被忽略",
			body:          `{"max_tokens":null}`,
			wantMaxTokens: 0, // gjson Type=Null != Number → 条件不满足，跳过
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parsed, err := ParseGatewayRequest([]byte(tt.body), "")
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tt.wantMaxTokens, parsed.MaxTokens)
		})
	}
}

// ============ Task 7.5: Benchmark 测试 ============

// parseGatewayRequestOld 是基于完整 json.Unmarshal 的旧实现，用于 benchmark 对比基线。
// 核心路径：先 Unmarshal 到 map[string]any，再逐字段提取。
func parseGatewayRequestOld(body []byte, protocol string) (*ParsedRequest, error) {
	parsed := &ParsedRequest{
		Body: body,
	}

	var req map[string]any
	if err := json.Unmarshal(body, &req); err != nil {
		return nil, err
	}

	// model
	if raw, ok := req["model"]; ok {
		s, ok := raw.(string)
		if !ok {
			return nil, fmt.Errorf("invalid model field type")
		}
		parsed.Model = s
	}

	// stream
	if raw, ok := req["stream"]; ok {
		b, ok := raw.(bool)
		if !ok {
			return nil, fmt.Errorf("invalid stream field type")
		}
		parsed.Stream = b
	}

	// metadata.user_id
	if meta, ok := req["metadata"].(map[string]any); ok {
		if uid, ok := meta["user_id"].(string); ok {
			parsed.MetadataUserID = uid
		}
	}

	// thinking.type
	if thinking, ok := req["thinking"].(map[string]any); ok {
		if thinkType, ok := thinking["type"].(string); ok && thinkType == "enabled" {
			parsed.ThinkingEnabled = true
		}
	}

	// max_tokens
	if raw, ok := req["max_tokens"]; ok {
		if n, ok := parseIntegralNumber(raw); ok {
			parsed.MaxTokens = n
		}
	}

	// system / messages（按协议分支）
	switch protocol {
	case domain.PlatformGemini:
		if sysInst, ok := req["systemInstruction"].(map[string]any); ok {
			if parts, ok := sysInst["parts"].([]any); ok {
				parsed.System = parts
			}
		}
		if contents, ok := req["contents"].([]any); ok {
			parsed.Messages = contents
		}
	default:
		if system, ok := req["system"]; ok {
			parsed.HasSystem = true
			parsed.System = system
		}
		if messages, ok := req["messages"].([]any); ok {
			parsed.Messages = messages
		}
	}

	return parsed, nil
}

// buildSmallJSON 构建 ~500B 的小型测试 JSON
func buildSmallJSON() []byte {
	return []byte(`{"model":"claude-sonnet-4-5","stream":true,"max_tokens":4096,"metadata":{"user_id":"user-abc123"},"thinking":{"type":"enabled","budget_tokens":2048},"system":"You are a helpful assistant.","messages":[{"role":"user","content":"What is the meaning of life?"},{"role":"assistant","content":"The meaning of life is a philosophical question."},{"role":"user","content":"Can you elaborate?"}]}`)
}

// buildLargeJSON 构建 ~50KB 的大型测试 JSON（大量 messages）
func buildLargeJSON() []byte {
	var b strings.Builder
	b.WriteString(`{"model":"claude-sonnet-4-5","stream":true,"max_tokens":8192,"metadata":{"user_id":"user-xyz789"},"system":[{"type":"text","text":"You are a detailed assistant.","cache_control":{"type":"ephemeral"}}],"messages":[`)

	msgCount := 200
	for i := 0; i < msgCount; i++ {
		if i > 0 {
			b.WriteByte(',')
		}
		if i%2 == 0 {
			b.WriteString(fmt.Sprintf(`{"role":"user","content":"This is user message number %d with some extra padding text to make the message reasonably long for benchmarking purposes. Lorem ipsum dolor sit amet."}`, i))
		} else {
			b.WriteString(fmt.Sprintf(`{"role":"assistant","content":[{"type":"text","text":"This is assistant response number %d. I will provide a detailed answer with multiple sentences to simulate real conversation content for benchmark testing."}]}`, i))
		}
	}

	b.WriteString(`]}`)
	return []byte(b.String())
}

func BenchmarkParseGatewayRequest_Old_Small(b *testing.B) {
	data := buildSmallJSON()
	b.SetBytes(int64(len(data)))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = parseGatewayRequestOld(data, "")
	}
}

func BenchmarkParseGatewayRequest_New_Small(b *testing.B) {
	data := buildSmallJSON()
	b.SetBytes(int64(len(data)))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = ParseGatewayRequest(data, "")
	}
}

func BenchmarkParseGatewayRequest_Old_Large(b *testing.B) {
	data := buildLargeJSON()
	b.SetBytes(int64(len(data)))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = parseGatewayRequestOld(data, "")
	}
}

func TestParseGatewayRequest_OutputEffort(t *testing.T) {
	tests := []struct {
		name       string
		body       string
		wantEffort string
	}{
		{
			name:       "output_config.effort present",
			body:       `{"model":"claude-opus-4-6","output_config":{"effort":"medium"},"messages":[]}`,
			wantEffort: "medium",
		},
		{
			name:       "output_config.effort max",
			body:       `{"model":"claude-opus-4-6","output_config":{"effort":"max"},"messages":[]}`,
			wantEffort: "max",
		},
		{
			name:       "output_config.effort xhigh",
			body:       `{"model":"claude-opus-4-7","output_config":{"effort":"xhigh"},"messages":[]}`,
			wantEffort: "xhigh",
		},
		{
			name:       "output_config without effort",
			body:       `{"model":"claude-opus-4-6","output_config":{},"messages":[]}`,
			wantEffort: "",
		},
		{
			name:       "no output_config",
			body:       `{"model":"claude-opus-4-6","messages":[]}`,
			wantEffort: "",
		},
		{
			name:       "effort with whitespace trimmed",
			body:       `{"model":"claude-opus-4-6","output_config":{"effort":" high "},"messages":[]}`,
			wantEffort: "high",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parsed, err := ParseGatewayRequest([]byte(tt.body), "")
			require.NoError(t, err)
			require.Equal(t, tt.wantEffort, parsed.OutputEffort)
		})
	}
}

func TestNormalizeClaudeOutputEffort(t *testing.T) {
	tests := []struct {
		input string
		want  *string
	}{
		{"low", strPtr("low")},
		{"medium", strPtr("medium")},
		{"high", strPtr("high")},
		{"max", strPtr("max")},
		{"LOW", strPtr("low")},
		{"Max", strPtr("max")},
		{" medium ", strPtr("medium")},
		{"xhigh", strPtr("xhigh")},
		{"XHIGH", strPtr("xhigh")},
		{"", nil},
		{"unknown", nil},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := NormalizeClaudeOutputEffort(tt.input)
			if tt.want == nil {
				require.Nil(t, got)
			} else {
				require.NotNil(t, got)
				require.Equal(t, *tt.want, *got)
			}
		})
	}
}

func BenchmarkParseGatewayRequest_New_Large(b *testing.B) {
	data := buildLargeJSON()
	b.SetBytes(int64(len(data)))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = ParseGatewayRequest(data, "")
	}
}
