package kiro

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestBuildRuntimeUserAgentStable(t *testing.T) {
	key := BuildAccountKey("client-id", "", "", "", 1)
	machineID := BuildMachineID("refresh-token", "", "")
	ua1 := BuildRuntimeUserAgent(key, machineID)
	ua2 := BuildRuntimeUserAgent(key, machineID)
	amzUA := BuildRuntimeAmzUserAgent(key, machineID)

	require.Equal(t, ua1, ua2)
	require.Contains(t, ua1, "KiroIDE-")
	require.Contains(t, amzUA, "KiroIDE-")
	require.Contains(t, ua1, "KiroIDE-0.11.")
	require.Contains(t, ua1, "aws-sdk-js/1.0.34")
	require.Contains(t, ua1, "md/nodejs#22.22.0")
	require.Contains(t, ua1, machineID)
	require.Contains(t, amzUA, machineID)
}

func TestBuildKiroPayloadBasic(t *testing.T) {
	SetCachedWebSearchDescription("")
	body := []byte(`{
		"model":"claude-sonnet-4-5",
		"system":"You are a test system prompt.",
		"messages":[{"role":"user","content":"hello kiro"}],
		"tools":[{"name":"web_search","description":"", "input_schema":{"type":"object","properties":{"query":{"type":"string"}}}}]
	}`)

	payload, err := BuildKiroPayload(body, "claude-sonnet-4.5", "arn:aws:codewhisperer:us-east-1:123456789012:profile/test", "AI_EDITOR", nil)
	require.NoError(t, err)

	require.Equal(t, "claude-sonnet-4.5", gjson.GetBytes(payload, "conversationState.currentMessage.userInputMessage.modelId").String())
	require.Equal(t, "AI_EDITOR", gjson.GetBytes(payload, "conversationState.currentMessage.userInputMessage.origin").String())
	require.Equal(t, "remote_web_search", gjson.GetBytes(payload, "conversationState.currentMessage.userInputMessage.userInputMessageContext.tools.0.toolSpecification.name").String())
	require.Equal(t, remoteWebSearchDescription, gjson.GetBytes(payload, "conversationState.currentMessage.userInputMessage.userInputMessageContext.tools.0.toolSpecification.description").String())
	require.Equal(t, "hello kiro", gjson.GetBytes(payload, "conversationState.currentMessage.userInputMessage.content").String())
	systemContent := gjson.GetBytes(payload, "conversationState.history.0.userInputMessage.content").String()
	require.Contains(t, systemContent, "[Context: Current time is ")
	require.Contains(t, systemContent, "You are a test system prompt.")
	require.Equal(t, "I will follow these instructions.", gjson.GetBytes(payload, "conversationState.history.1.assistantResponseMessage.content").String())
}

func TestBuildKiroPayloadWebSearchUsesCachedDescription(t *testing.T) {
	SetCachedWebSearchDescription("cached web search description")
	t.Cleanup(func() { SetCachedWebSearchDescription("") })

	body := []byte(`{
		"model":"claude-sonnet-4-5",
		"messages":[{"role":"user","content":"hello kiro"}],
		"tools":[{"name":"web_search","description":"caller description", "input_schema":{"type":"object","properties":{"query":{"type":"string"}}}}]
	}`)

	payload, err := BuildKiroPayload(body, "claude-sonnet-4.5", "", "AI_EDITOR", nil)
	require.NoError(t, err)
	require.Equal(t, "remote_web_search", gjson.GetBytes(payload, "conversationState.currentMessage.userInputMessage.userInputMessageContext.tools.0.toolSpecification.name").String())
	require.Equal(t, "cached web search description", gjson.GetBytes(payload, "conversationState.currentMessage.userInputMessage.userInputMessageContext.tools.0.toolSpecification.description").String())
}

func TestBuildKiroPayloadAppendsChunkedWritePolicyToWriteAndEditTools(t *testing.T) {
	body := []byte(`{
		"model":"claude-sonnet-4-5",
		"messages":[{"role":"user","content":"hello"}],
		"tools":[
			{"name":"Write","description":"write file", "input_schema":{"type":"object"}},
			{"name":"Edit","description":"edit file", "input_schema":{"type":"object"}},
			{"name":"read_file","description":"read file", "input_schema":{"type":"object"}}
		]
	}`)

	payload, err := BuildKiroPayload(body, "claude-sonnet-4.5", "", "AI_EDITOR", nil)
	require.NoError(t, err)

	tools := gjson.GetBytes(payload, "conversationState.currentMessage.userInputMessage.userInputMessageContext.tools").Array()
	require.Len(t, tools, 3)
	require.Contains(t, tools[0].Get("toolSpecification.description").String(), writeToolDescriptionSuffix)
	require.Contains(t, tools[1].Get("toolSpecification.description").String(), editToolDescriptionSuffix)
	require.NotContains(t, tools[2].Get("toolSpecification.description").String(), "chunks of no more than 50 lines")
}

func TestBuildKiroPayloadChunkedWritePolicyIsIdempotentAndTruncated(t *testing.T) {
	longDescription := strings.Repeat("long description ", 900) + "\n" + writeToolDescriptionSuffix
	body := []byte(fmt.Sprintf(`{
		"model":"claude-sonnet-4-5",
		"messages":[{"role":"user","content":"hello"}],
		"tools":[{"name":"write_to_file","description":%q, "input_schema":{"type":"object"}}]
	}`, longDescription))

	payload, err := BuildKiroPayload(body, "claude-sonnet-4.5", "", "AI_EDITOR", nil)
	require.NoError(t, err)

	description := gjson.GetBytes(payload, "conversationState.currentMessage.userInputMessage.userInputMessageContext.tools.0.toolSpecification.description").String()
	require.LessOrEqual(t, len(description), kiroMaxToolDescLen)
	require.Equal(t, 1, strings.Count(description, writeToolDescriptionSuffix))
	require.Contains(t, description, writeToolDescriptionSuffix)
}

func TestBuildKiroPayloadInjectsChunkedWritePolicyIntoSystemPrompt(t *testing.T) {
	body := []byte(`{
		"model":"claude-sonnet-4-5",
		"system":"Follow user instructions.",
		"thinking":{"type":"enabled","budget_tokens":2048},
		"messages":[{"role":"user","content":"hello"}]
	}`)

	payload, err := BuildKiroPayload(body, "claude-sonnet-4.5", "", "AI_EDITOR", nil)
	require.NoError(t, err)

	systemContent := gjson.GetBytes(payload, "conversationState.history.0.userInputMessage.content").String()
	require.Contains(t, systemContent, "<thinking_mode>enabled</thinking_mode>")
	require.Contains(t, systemContent, "Follow user instructions.")
	require.Contains(t, systemContent, systemChunkedWritePolicy)
	require.Equal(t, 1, strings.Count(systemContent, systemChunkedWritePolicy))
}

func TestBuildKiroPayloadInjectsThinkingIntoHistory(t *testing.T) {
	body := []byte(`{
		"model":"claude-sonnet-4-5",
		"thinking":{"type":"enabled","budget_tokens":2048},
		"messages":[{"role":"user","content":"hello kiro"}]
	}`)

	headers := http.Header{}
	headers.Set("Anthropic-Beta", "interleaved-thinking-2025-05-14")

	payload, err := BuildKiroPayload(body, "claude-sonnet-4.5", "", "AI_EDITOR", headers)
	require.NoError(t, err)

	require.Equal(t, "hello kiro", gjson.GetBytes(payload, "conversationState.currentMessage.userInputMessage.content").String())
	systemContent := gjson.GetBytes(payload, "conversationState.history.0.userInputMessage.content").String()
	require.Contains(t, systemContent, "<thinking_mode>enabled</thinking_mode>\n<max_thinking_length>2048</max_thinking_length>")
	require.Contains(t, systemContent, "[Context: Current time is ")
	require.Equal(t, "I will follow these instructions.", gjson.GetBytes(payload, "conversationState.history.1.assistantResponseMessage.content").String())
}

func TestBuildKiroPayloadInjectsAdaptiveThinkingForOpus46ThinkingModel(t *testing.T) {
	body := []byte(`{
		"model":"claude-opus-4-6-thinking",
		"messages":[{"role":"user","content":"hello kiro"}]
	}`)

	payload, err := BuildKiroPayload(body, "claude-opus-4.6", "", "AI_EDITOR", nil)
	require.NoError(t, err)

	systemContent := gjson.GetBytes(payload, "conversationState.history.0.userInputMessage.content").String()
	require.Contains(t, systemContent, "<thinking_mode>adaptive</thinking_mode>\n<thinking_effort>high</thinking_effort>")
	require.Contains(t, systemContent, "[Context: Current time is ")
}

func TestBuildKiroPayloadInjectsThinkingForThinkingAliasModel(t *testing.T) {
	body := []byte(`{
		"model":"claude-sonnet-4-5-20250929-thinking",
		"messages":[{"role":"user","content":"hello kiro"}]
	}`)

	payload, err := BuildKiroPayload(body, "claude-sonnet-4.5", "", "AI_EDITOR", nil)
	require.NoError(t, err)

	systemContent := gjson.GetBytes(payload, "conversationState.history.0.userInputMessage.content").String()
	require.Contains(t, systemContent, "<thinking_mode>enabled</thinking_mode>\n<max_thinking_length>20000</max_thinking_length>")
}

func TestBuildKiroPayloadHeaderOnlyThinking(t *testing.T) {
	body := []byte(`{
		"model":"claude-sonnet-4-5",
		"messages":[{"role":"user","content":"hello kiro"}]
	}`)

	headers := http.Header{}
	headers.Set("Anthropic-Beta", "oauth-2025-04-20,interleaved-thinking-2025-05-14")

	payload, err := BuildKiroPayload(body, "claude-sonnet-4.5", "", "AI_EDITOR", headers)
	require.NoError(t, err)

	systemContent := gjson.GetBytes(payload, "conversationState.history.0.userInputMessage.content").String()
	require.Contains(t, systemContent, "<thinking_mode>enabled</thinking_mode>\n<max_thinking_length>16000</max_thinking_length>")
}

func TestBuildKiroPayloadInjectsToolChoiceHints(t *testing.T) {
	body := []byte(`{
		"model":"claude-sonnet-4-5",
		"messages":[{"role":"user","content":"hello kiro"}],
		"tools":[{"name":"web_search","description":"search", "input_schema":{"type":"object","properties":{"query":{"type":"string"}}}}],
		"tool_choice":{"type":"tool","name":"web_search"}
	}`)

	payload, err := BuildKiroPayload(body, "claude-sonnet-4.5", "", "AI_EDITOR", nil)
	require.NoError(t, err)

	systemContent := gjson.GetBytes(payload, "conversationState.history.0.userInputMessage.content").String()
	require.Contains(t, systemContent, "MUST use the tool named 'remote_web_search'")
}

func TestBuildKiroPayloadInjectsRequiredToolChoiceHint(t *testing.T) {
	body := []byte(`{
		"model":"claude-sonnet-4-5",
		"messages":[{"role":"user","content":"hello kiro"}],
		"tools":[{"name":"web_search","description":"search", "input_schema":{"type":"object","properties":{"query":{"type":"string"}}}}],
		"tool_choice":{"type":"any"}
	}`)

	payload, err := BuildKiroPayload(body, "claude-sonnet-4.5", "", "AI_EDITOR", nil)
	require.NoError(t, err)

	systemContent := gjson.GetBytes(payload, "conversationState.history.0.userInputMessage.content").String()
	require.Contains(t, systemContent, "MUST use at least one of the available tools")
}

func TestBuildKiroPayloadToolChoiceNoneOmitsTools(t *testing.T) {
	body := []byte(`{
		"model":"claude-sonnet-4-5",
		"messages":[{"role":"user","content":"hello kiro"}],
		"tools":[{"name":"web_search","description":"search", "input_schema":{"type":"object","properties":{"query":{"type":"string"}}}}],
		"tool_choice":{"type":"none"}
	}`)

	payload, err := BuildKiroPayload(body, "claude-sonnet-4.5", "", "AI_EDITOR", nil)
	require.NoError(t, err)

	systemContent := gjson.GetBytes(payload, "conversationState.history.0.userInputMessage.content").String()
	require.Contains(t, systemContent, "Do not use any tools. Respond with text only.")
	require.False(t, gjson.GetBytes(payload, "conversationState.currentMessage.userInputMessage.userInputMessageContext.tools").Exists())
}

func TestParseNonStreamingEventStream(t *testing.T) {
	stream := bytes.NewBuffer(nil)
	_, _ = stream.Write(buildEventStreamFrame(t, "assistantResponseEvent", map[string]any{
		"assistantResponseEvent": map[string]any{
			"content": "hello from kiro",
		},
	}))
	_, _ = stream.Write(buildEventStreamFrame(t, "messageMetadataEvent", map[string]any{
		"messageMetadataEvent": map[string]any{
			"tokenUsage": map[string]any{
				"uncachedInputTokens":  12,
				"outputTokens":         7,
				"cacheReadInputTokens": 3,
				"totalTokens":          22,
			},
		},
	}))
	_, _ = stream.Write(buildEventStreamFrame(t, "messageStopEvent", map[string]any{
		"messageStopEvent": map[string]any{
			"stop_reason": "end_turn",
		},
	}))

	result, err := ParseNonStreamingEventStream(stream, "claude-sonnet-4-5")
	require.NoError(t, err)
	require.Equal(t, "end_turn", result.StopReason)
	require.Equal(t, 15, result.Usage.InputTokens)
	require.Equal(t, 7, result.Usage.OutputTokens)
	require.Equal(t, 22, result.Usage.TotalTokens)

	var response map[string]any
	require.NoError(t, json.Unmarshal(result.ResponseBody, &response))
	require.Equal(t, "end_turn", response["stop_reason"])
	content, _ := response["content"].([]any)
	require.NotEmpty(t, content)
	first, _ := content[0].(map[string]any)
	require.Equal(t, "text", first["type"])
	firstText, ok := first["text"].(string)
	require.True(t, ok)
	require.True(t, strings.Contains(firstText, "hello from kiro"))
}

func TestExtractThinkingBlocksIgnoresLiteralTags(t *testing.T) {
	content := strings.Join([]string{
		"Use `<thinking>` literally.",
		"Quote \"<thinking>\" and '</thinking>'.",
		"> <thinking>quoted</thinking>",
		"```",
		"<thinking>code</thinking>",
		"```",
	}, "\n")

	blocks := extractThinkingBlocks(content)
	require.Len(t, blocks, 1)
	require.Equal(t, "text", blocks[0]["type"])
	require.Equal(t, content, blocks[0]["text"])
}

func TestExtractThinkingBlocksParsesRealTags(t *testing.T) {
	blocks := extractThinkingBlocks("<thinking>\nreason</thinking>\n\nfinal text")

	require.Len(t, blocks, 2)
	require.Equal(t, "thinking", blocks[0]["type"])
	require.Equal(t, "reason", blocks[0]["thinking"])
	require.NotEmpty(t, blocks[0]["signature"])
	require.Equal(t, "text", blocks[1]["type"])
	require.Equal(t, "final text", blocks[1]["text"])
}

func TestParseNonStreamingEventStreamPureThinkingFallback(t *testing.T) {
	stream := bytes.NewBuffer(nil)
	_, _ = stream.Write(buildEventStreamFrame(t, "assistantResponseEvent", map[string]any{
		"assistantResponseEvent": map[string]any{
			"content": "<thinking>reason only</thinking>",
		},
	}))

	result, err := ParseNonStreamingEventStream(stream, "claude-sonnet-4-5")
	require.NoError(t, err)
	require.Equal(t, "max_tokens", gjson.GetBytes(result.ResponseBody, "stop_reason").String())

	content := gjson.GetBytes(result.ResponseBody, "content").Array()
	require.Len(t, content, 2)
	require.Equal(t, "thinking", content[0].Get("type").String())
	require.Equal(t, "reason only", content[0].Get("thinking").String())
	require.Equal(t, "text", content[1].Get("type").String())
	require.Equal(t, "", content[1].Get("text").String())
}

func TestParseNonStreamingEventStreamThinkingWithTextKeepsEndTurn(t *testing.T) {
	stream := bytes.NewBuffer(nil)
	_, _ = stream.Write(buildEventStreamFrame(t, "assistantResponseEvent", map[string]any{
		"assistantResponseEvent": map[string]any{
			"content": "<thinking>reason</thinking>\n\nfinal",
		},
	}))

	result, err := ParseNonStreamingEventStream(stream, "claude-sonnet-4-5")
	require.NoError(t, err)
	require.Equal(t, "end_turn", gjson.GetBytes(result.ResponseBody, "stop_reason").String())
	require.Equal(t, "thinking", gjson.GetBytes(result.ResponseBody, "content.0.type").String())
	require.Equal(t, "text", gjson.GetBytes(result.ResponseBody, "content.1.type").String())
	require.Equal(t, "final", gjson.GetBytes(result.ResponseBody, "content.1.text").String())
}

func TestParseNonStreamingEventStreamThinkingWithToolUseKeepsToolUseStopReason(t *testing.T) {
	stream := bytes.NewBuffer(nil)
	_, _ = stream.Write(buildEventStreamFrame(t, "assistantResponseEvent", map[string]any{
		"assistantResponseEvent": map[string]any{
			"content": "<thinking>reason only</thinking>",
		},
	}))
	_, _ = stream.Write(buildEventStreamFrame(t, "toolUseEvent", map[string]any{
		"toolUseEvent": map[string]any{
			"toolUseId": "toolu_search",
			"name":      "remote_web_search",
			"input":     `{"query":"golang"}`,
			"stop":      true,
		},
	}))

	result, err := ParseNonStreamingEventStream(stream, "claude-sonnet-4-5")
	require.NoError(t, err)
	require.Equal(t, "tool_use", gjson.GetBytes(result.ResponseBody, "stop_reason").String())
	require.Equal(t, "thinking", gjson.GetBytes(result.ResponseBody, "content.0.type").String())
	require.Equal(t, "tool_use", gjson.GetBytes(result.ResponseBody, "content.1.type").String())
	require.False(t, gjson.GetBytes(result.ResponseBody, "content.2.text").Exists())
}

func TestParseNonStreamingEventStreamExtractsEmbeddedToolCall(t *testing.T) {
	stream := bytes.NewBuffer(nil)
	_, _ = stream.Write(buildEventStreamFrame(t, "assistantResponseEvent", map[string]any{
		"assistantResponseEvent": map[string]any{
			"content": `Before [Called web_search with args: {"query":"golang concurrency"}] After`,
		},
	}))

	result, err := ParseNonStreamingEventStream(stream, "claude-sonnet-4-5")
	require.NoError(t, err)
	require.Equal(t, "tool_use", result.StopReason)
	require.NotContains(t, string(result.ResponseBody), "[Called")

	content := gjson.GetBytes(result.ResponseBody, "content").Array()
	require.Len(t, content, 2)
	require.Equal(t, "text", content[0].Get("type").String())
	require.Equal(t, "Before  After", content[0].Get("text").String())
	require.Equal(t, "tool_use", content[1].Get("type").String())
	require.Equal(t, "remote_web_search", content[1].Get("name").String())
	require.Equal(t, "golang concurrency", content[1].Get("input.query").String())
}

func TestParseNonStreamingEventStreamDeduplicatesToolUsesByContent(t *testing.T) {
	stream := bytes.NewBuffer(nil)
	_, _ = stream.Write(buildEventStreamFrame(t, "assistantResponseEvent", map[string]any{
		"assistantResponseEvent": map[string]any{
			"toolUses": []map[string]any{
				{
					"toolUseId": "toolu_first",
					"name":      "remote_web_search",
					"input": map[string]any{
						"query": "golang",
					},
				},
			},
		},
	}))
	_, _ = stream.Write(buildEventStreamFrame(t, "toolUseEvent", map[string]any{
		"toolUseEvent": map[string]any{
			"toolUseId": "toolu_second",
			"name":      "remote_web_search",
			"input": map[string]any{
				"query": "golang",
			},
			"stop": true,
		},
	}))

	result, err := ParseNonStreamingEventStream(stream, "claude-sonnet-4-5")
	require.NoError(t, err)

	content := gjson.GetBytes(result.ResponseBody, "content").Array()
	toolUseCount := 0
	for _, block := range content {
		if block.Get("type").String() == "tool_use" {
			toolUseCount++
		}
	}
	require.Equal(t, 1, toolUseCount)
}

func TestParseNonStreamingEventStreamSkipsTruncatedToolUse(t *testing.T) {
	stream := bytes.NewBuffer(nil)
	_, _ = stream.Write(buildEventStreamFrame(t, "toolUseEvent", map[string]any{
		"toolUseEvent": map[string]any{
			"toolUseId": "toolu_truncated",
			"name":      "write_to_file",
			"input":     `{"path":"main.go","content":"package main`,
			"stop":      true,
		},
	}))

	result, err := ParseNonStreamingEventStream(stream, "claude-sonnet-4-5")
	require.NoError(t, err)
	require.Equal(t, "end_turn", result.StopReason)

	content := gjson.GetBytes(result.ResponseBody, "content").Array()
	require.Len(t, content, 1)
	require.Equal(t, "text", content[0].Get("type").String())
	require.NotContains(t, string(result.ResponseBody), `"type":"tool_use"`)
}

func TestParseNonStreamingEventStreamDropsIncompleteEmbeddedToolTail(t *testing.T) {
	stream := bytes.NewBuffer(nil)
	_, _ = stream.Write(buildEventStreamFrame(t, "assistantResponseEvent", map[string]any{
		"assistantResponseEvent": map[string]any{
			"content": `Before [Called web_search with args: {"query":"golang`,
		},
	}))

	result, err := ParseNonStreamingEventStream(stream, "claude-sonnet-4-5")
	require.NoError(t, err)
	require.Equal(t, "end_turn", result.StopReason)
	require.NotContains(t, string(result.ResponseBody), "[Called")
	require.Equal(t, "Before ", gjson.GetBytes(result.ResponseBody, "content.0.text").String())
}

func TestParseNonStreamingEventStreamThinkingOnlyResponse(t *testing.T) {
	stream := bytes.NewBuffer(nil)
	_, _ = stream.Write(buildEventStreamFrame(t, "reasoningContentEvent", map[string]any{
		"reasoningContentEvent": map[string]any{
			"text": "I should think first.",
		},
	}))

	result, err := ParseNonStreamingEventStream(stream, "claude-sonnet-4-5")
	require.NoError(t, err)
	require.Equal(t, "max_tokens", gjson.GetBytes(result.ResponseBody, "stop_reason").String())
	require.Equal(t, "thinking", gjson.GetBytes(result.ResponseBody, "content.0.type").String())
	require.Equal(t, "I should think first.", gjson.GetBytes(result.ResponseBody, "content.0.thinking").String())
	require.Equal(t, "text", gjson.GetBytes(result.ResponseBody, "content.1.type").String())
	require.Equal(t, "", gjson.GetBytes(result.ResponseBody, "content.1.text").String())
}

func TestStreamEventStreamAsAnthropicExtractsEmbeddedToolCall(t *testing.T) {
	stream := bytes.NewBuffer(nil)
	_, _ = stream.Write(buildEventStreamFrame(t, "assistantResponseEvent", map[string]any{
		"assistantResponseEvent": map[string]any{
			"content": `Before [Called web_search with args: {"query":"gol`,
		},
	}))
	_, _ = stream.Write(buildEventStreamFrame(t, "assistantResponseEvent", map[string]any{
		"assistantResponseEvent": map[string]any{
			"content": `ang"}] After`,
		},
	}))

	var out bytes.Buffer
	result, err := StreamEventStreamAsAnthropic(context.Background(), stream, &out, "claude-sonnet-4-5", 9)
	require.NoError(t, err)
	require.Equal(t, "tool_use", result.StopReason)

	output := out.String()
	require.NotContains(t, output, "[Called")
	require.Contains(t, output, `"text":"Before "`)
	require.Contains(t, output, `"text":" After"`)
	require.Contains(t, output, `"name":"remote_web_search"`)
	require.Contains(t, output, `"partial_json":"{\"query\":\"golang\"}"`)
}

func TestStreamEventStreamAsAnthropicSkipsLeadingWhitespaceOnlyChunk(t *testing.T) {
	stream := bytes.NewBuffer(nil)
	_, _ = stream.Write(buildEventStreamFrame(t, "assistantResponseEvent", map[string]any{
		"assistantResponseEvent": map[string]any{
			"content": "\n",
		},
	}))
	_, _ = stream.Write(buildEventStreamFrame(t, "assistantResponseEvent", map[string]any{
		"assistantResponseEvent": map[string]any{
			"content": "Hello from Kiro",
		},
	}))

	var out bytes.Buffer
	result, err := StreamEventStreamAsAnthropic(context.Background(), stream, &out, "claude-sonnet-4-5", 9)
	require.NoError(t, err)
	require.Equal(t, "end_turn", result.StopReason)

	output := out.String()
	require.Contains(t, output, `"text":"Hello from Kiro"`)
	require.NotContains(t, output, `"delta":{"text":"\n","type":"text_delta"}`)
	require.NotContains(t, output, `"delta":{"text":"","type":"text_delta"}`)
}

func TestStreamEventStreamAsAnthropicSkipsTrailingWhitespaceOnlyChunk(t *testing.T) {
	stream := bytes.NewBuffer(nil)
	_, _ = stream.Write(buildEventStreamFrame(t, "assistantResponseEvent", map[string]any{
		"assistantResponseEvent": map[string]any{
			"content": "Hello from Kiro",
		},
	}))
	_, _ = stream.Write(buildEventStreamFrame(t, "assistantResponseEvent", map[string]any{
		"assistantResponseEvent": map[string]any{
			"content": "\n",
		},
	}))
	_, _ = stream.Write(buildEventStreamFrame(t, "assistantResponseEvent", map[string]any{
		"assistantResponseEvent": map[string]any{
			"content": "\n\n",
		},
	}))

	var out bytes.Buffer
	result, err := StreamEventStreamAsAnthropic(context.Background(), stream, &out, "claude-sonnet-4-5", 9)
	require.NoError(t, err)
	require.Equal(t, "end_turn", result.StopReason)

	output := out.String()
	require.Contains(t, output, `"text":"Hello from Kiro"`)
	require.NotContains(t, output, `"text":"\n"`)
	require.NotContains(t, output, `"text":"\n\n"`)
}

func TestStreamEventStreamAsAnthropicDelaysMessageStartUntilContent(t *testing.T) {
	pr, pw := io.Pipe()
	var out bytes.Buffer
	errCh := make(chan error, 1)

	go func() {
		_, err := StreamEventStreamAsAnthropic(context.Background(), pr, &out, "claude-sonnet-4-5", 9)
		errCh <- err
	}()

	_, err := pw.Write(buildEventStreamFrame(t, "messageMetadataEvent", map[string]any{
		"messageMetadataEvent": map[string]any{
			"tokenUsage": map[string]any{
				"uncachedInputTokens": 9,
			},
		},
	}))
	require.NoError(t, err)

	time.Sleep(50 * time.Millisecond)
	require.Empty(t, out.String())

	_, err = pw.Write(buildEventStreamFrame(t, "toolUseEvent", map[string]any{
		"toolUseEvent": map[string]any{
			"toolUseId": "toolu_delayed",
			"name":      "remote_web_search",
			"input": map[string]any{
				"query": "golang",
			},
			"stop": true,
		},
	}))
	require.NoError(t, err)
	require.NoError(t, pw.Close())
	require.NoError(t, <-errCh)

	output := out.String()
	require.Contains(t, output, "event: message_start")
	require.Contains(t, output, `"name":"remote_web_search"`)
	require.Contains(t, output, `"partial_json":"{\"query\":\"golang\"}`)
	messageStartIdx := strings.Index(output, "event: message_start")
	toolUseIdx := strings.Index(output, `"name":"remote_web_search"`)
	require.NotEqual(t, -1, messageStartIdx)
	require.NotEqual(t, -1, toolUseIdx)
	require.Less(t, messageStartIdx, toolUseIdx)
}

func TestStreamEventStreamAsAnthropicStreamsToolUseFragments(t *testing.T) {
	stream := bytes.NewBuffer(nil)
	_, _ = stream.Write(buildEventStreamFrame(t, "toolUseEvent", map[string]any{
		"toolUseEvent": map[string]any{
			"toolUseId": "toolu_stream",
			"name":      "write_file",
			"input":     `{"path":"/tmp/a.txt",`,
		},
	}))
	_, _ = stream.Write(buildEventStreamFrame(t, "toolUseEvent", map[string]any{
		"toolUseEvent": map[string]any{
			"toolUseId": "toolu_stream",
			"name":      "write_file",
			"input":     `"content":"hello"}`,
		},
	}))
	_, _ = stream.Write(buildEventStreamFrame(t, "toolUseEvent", map[string]any{
		"toolUseEvent": map[string]any{
			"toolUseId": "toolu_stream",
			"name":      "write_file",
			"stop":      true,
		},
	}))

	var out bytes.Buffer
	result, err := StreamEventStreamAsAnthropic(context.Background(), stream, &out, "claude-sonnet-4-5", 9)
	require.NoError(t, err)
	require.Equal(t, "tool_use", result.StopReason)

	output := out.String()
	require.Equal(t, 1, strings.Count(output, `"id":"toolu_stream"`))
	require.Contains(t, output, `"partial_json":"{\"path\":\"/tmp/a.txt\","`)
	require.Contains(t, output, `"partial_json":"\"content\":\"hello\"}"`)
	require.Contains(t, output, `event: content_block_stop`)
}

func TestStreamEventStreamAsAnthropicStreamsIncompleteToolUseFragment(t *testing.T) {
	stream := bytes.NewBuffer(nil)
	_, _ = stream.Write(buildEventStreamFrame(t, "toolUseEvent", map[string]any{
		"toolUseEvent": map[string]any{
			"toolUseId": "toolu_incomplete",
			"name":      "write_file",
			"input":     `{"path":`,
			"stop":      true,
		},
	}))

	var out bytes.Buffer
	result, err := StreamEventStreamAsAnthropic(context.Background(), stream, &out, "claude-sonnet-4-5", 9)
	require.NoError(t, err)
	require.Equal(t, "tool_use", result.StopReason)
	require.Contains(t, out.String(), `"partial_json":"{\"path\":"`)
}

func TestStreamEventStreamAsAnthropicStopsPreviousToolWhenIDChanges(t *testing.T) {
	stream := bytes.NewBuffer(nil)
	_, _ = stream.Write(buildEventStreamFrame(t, "toolUseEvent", map[string]any{
		"toolUseEvent": map[string]any{
			"toolUseId": "toolu_one",
			"name":      "write_file",
			"input":     `{"path":"a"}`,
		},
	}))
	_, _ = stream.Write(buildEventStreamFrame(t, "toolUseEvent", map[string]any{
		"toolUseEvent": map[string]any{
			"toolUseId": "toolu_two",
			"name":      "read_file",
			"input":     `{"path":"b"}`,
			"stop":      true,
		},
	}))

	var out bytes.Buffer
	_, err := StreamEventStreamAsAnthropic(context.Background(), stream, &out, "claude-sonnet-4-5", 9)
	require.NoError(t, err)

	output := out.String()
	firstStart := strings.Index(output, `"id":"toolu_one"`)
	firstStop := strings.Index(output[firstStart:], `event: content_block_stop`)
	secondStart := strings.Index(output, `"id":"toolu_two"`)
	require.NotEqual(t, -1, firstStart)
	require.NotEqual(t, -1, firstStop)
	require.NotEqual(t, -1, secondStart)
	require.Less(t, firstStart+firstStop, secondStart)
}

func TestStreamEventStreamAsAnthropicClosesToolBeforeText(t *testing.T) {
	stream := bytes.NewBuffer(nil)
	_, _ = stream.Write(buildEventStreamFrame(t, "toolUseEvent", map[string]any{
		"toolUseEvent": map[string]any{
			"toolUseId": "toolu_before_text",
			"name":      "write_file",
			"input":     `{"path":"a"}`,
		},
	}))
	_, _ = stream.Write(buildEventStreamFrame(t, "assistantResponseEvent", map[string]any{
		"assistantResponseEvent": map[string]any{
			"content": "done",
		},
	}))

	var out bytes.Buffer
	_, err := StreamEventStreamAsAnthropic(context.Background(), stream, &out, "claude-sonnet-4-5", 9)
	require.NoError(t, err)

	output := out.String()
	toolStart := strings.Index(output, `"id":"toolu_before_text"`)
	toolStop := strings.Index(output[toolStart:], `event: content_block_stop`)
	textDelta := strings.Index(output, `"text":"done"`)
	require.NotEqual(t, -1, toolStart)
	require.NotEqual(t, -1, toolStop)
	require.NotEqual(t, -1, textDelta)
	require.Less(t, toolStart+toolStop, textDelta)
}

func TestStreamEventStreamAsAnthropicClosesThinkingBeforeTool(t *testing.T) {
	stream := bytes.NewBuffer(nil)
	_, _ = stream.Write(buildEventStreamFrame(t, "reasoningContentEvent", map[string]any{
		"reasoningContentEvent": map[string]any{
			"text": "thinking first",
		},
	}))
	_, _ = stream.Write(buildEventStreamFrame(t, "toolUseEvent", map[string]any{
		"toolUseEvent": map[string]any{
			"toolUseId": "toolu_after_thinking",
			"name":      "write_file",
			"input":     `{"path":"a"}`,
			"stop":      true,
		},
	}))

	var out bytes.Buffer
	_, err := StreamEventStreamAsAnthropicWithContext(context.Background(), stream, &out, "claude-sonnet-4-5", 9, KiroRequestContext{ThinkingEnabled: true})
	require.NoError(t, err)

	output := out.String()
	thinkingDelta := strings.Index(output, `"thinking":"thinking first"`)
	toolStart := strings.Index(output, `"id":"toolu_after_thinking"`)
	require.NotEqual(t, -1, thinkingDelta)
	thinkingStop := strings.Index(output[thinkingDelta:], `event: content_block_stop`)
	require.NotEqual(t, -1, thinkingStop)
	require.NotEqual(t, -1, toolStart)
	require.Less(t, thinkingDelta+thinkingStop, toolStart)
}

func TestStreamEventStreamAsAnthropicClosesOpenToolAtEOF(t *testing.T) {
	stream := bytes.NewBuffer(nil)
	_, _ = stream.Write(buildEventStreamFrame(t, "toolUseEvent", map[string]any{
		"toolUseEvent": map[string]any{
			"toolUseId": "toolu_eof",
			"name":      "write_file",
			"input":     `{"path":"a"}`,
		},
	}))

	var out bytes.Buffer
	result, err := StreamEventStreamAsAnthropic(context.Background(), stream, &out, "claude-sonnet-4-5", 9)
	require.NoError(t, err)
	require.Equal(t, "tool_use", result.StopReason)
	require.Contains(t, out.String(), `event: content_block_stop`)
}

func TestStreamEventStreamAsAnthropicStreamsToolUseMapInput(t *testing.T) {
	stream := bytes.NewBuffer(nil)
	_, _ = stream.Write(buildEventStreamFrame(t, "toolUseEvent", map[string]any{
		"toolUseEvent": map[string]any{
			"toolUseId": "toolu_map",
			"name":      "remote_web_search",
			"input": map[string]any{
				"query": "golang",
			},
			"stop": true,
		},
	}))

	var out bytes.Buffer
	_, err := StreamEventStreamAsAnthropic(context.Background(), stream, &out, "claude-sonnet-4-5", 9)
	require.NoError(t, err)
	require.Contains(t, out.String(), `"partial_json":"{\"query\":\"golang\"}"`)
}

func TestStreamEventStreamAsAnthropicIgnoresPingFrames(t *testing.T) {
	stream := bytes.NewBuffer(nil)
	_, _ = stream.Write(buildEventStreamFrame(t, "ping", map[string]any{}))
	_, _ = stream.Write(buildEventStreamFrame(t, "assistantResponseEvent", map[string]any{
		"assistantResponseEvent": map[string]any{
			"content": "Hello after ping",
		},
	}))

	var out bytes.Buffer
	result, err := StreamEventStreamAsAnthropic(context.Background(), stream, &out, "claude-sonnet-4-5", 9)
	require.NoError(t, err)
	require.Equal(t, "end_turn", result.StopReason)
	require.Contains(t, out.String(), `"text":"Hello after ping"`)
}

func TestStreamEventStreamAsAnthropicThinkingOnlyResponse(t *testing.T) {
	stream := bytes.NewBuffer(nil)
	_, _ = stream.Write(buildEventStreamFrame(t, "reasoningContentEvent", map[string]any{
		"reasoningContentEvent": map[string]any{
			"text": "I should think first.",
		},
	}))

	var out bytes.Buffer
	result, err := StreamEventStreamAsAnthropicWithContext(context.Background(), stream, &out, "claude-sonnet-4-5", 9, KiroRequestContext{ThinkingEnabled: true})
	require.NoError(t, err)
	require.Equal(t, "max_tokens", result.StopReason)

	output := out.String()
	require.Contains(t, output, `"type":"thinking"`)
	require.Contains(t, output, `"type":"thinking_delta"`)
	require.Contains(t, output, `"thinking":"I should think first."`)
	require.Contains(t, output, `"text":" "`)
	require.Contains(t, output, `event: message_delta`)
	require.Contains(t, output, `event: message_stop`)
}

func TestStreamEventStreamAsAnthropicParsesMultipleReasoningEventsWhenEnabled(t *testing.T) {
	stream := bytes.NewBuffer(nil)
	_, _ = stream.Write(buildEventStreamFrame(t, "reasoningContentEvent", map[string]any{
		"reasoningContentEvent": map[string]any{"text": "first thought"},
	}))
	_, _ = stream.Write(buildEventStreamFrame(t, "reasoningContentEvent", map[string]any{
		"reasoningContentEvent": map[string]any{"text": "second thought"},
	}))
	_, _ = stream.Write(buildEventStreamFrame(t, "assistantResponseEvent", map[string]any{
		"assistantResponseEvent": map[string]any{"content": "final"},
	}))

	var out bytes.Buffer
	result, err := StreamEventStreamAsAnthropicWithContext(context.Background(), stream, &out, "claude-sonnet-4-5", 9, KiroRequestContext{ThinkingEnabled: true})
	require.NoError(t, err)
	require.Equal(t, "end_turn", result.StopReason)

	output := out.String()
	require.Contains(t, output, `"thinking":"first thought"`)
	require.Contains(t, output, `"thinking":"second thought"`)
	require.Contains(t, output, `"text":"final"`)
}

func TestStreamEventStreamAsAnthropicParsesTaggedThinkingWhenEnabled(t *testing.T) {
	stream := bytes.NewBuffer(nil)
	_, _ = stream.Write(buildEventStreamFrame(t, "assistantResponseEvent", map[string]any{
		"assistantResponseEvent": map[string]any{
			"content": "<thinking>\nreason</thinking>\n\nfinal",
		},
	}))

	var out bytes.Buffer
	result, err := StreamEventStreamAsAnthropicWithContext(context.Background(), stream, &out, "claude-sonnet-4-5", 9, KiroRequestContext{ThinkingEnabled: true})
	require.NoError(t, err)
	require.Equal(t, "end_turn", result.StopReason)

	output := out.String()
	thinkingDelta := strings.Index(output, `"thinking":"reason"`)
	textDelta := strings.Index(output, `"text":"final"`)
	require.NotEqual(t, -1, thinkingDelta)
	require.NotEqual(t, -1, textDelta)
	require.Less(t, thinkingDelta, textDelta)
	require.NotContains(t, output, `\u003c/thinking\u003e`)
}

func TestStreamEventStreamAsAnthropicBuffersSplitThinkingTags(t *testing.T) {
	stream := bytes.NewBuffer(nil)
	for _, chunk := range []string{"\n\n<think", "ing>\nrea", "son</thinking>", "\n\nfinal"} {
		_, _ = stream.Write(buildEventStreamFrame(t, "assistantResponseEvent", map[string]any{
			"assistantResponseEvent": map[string]any{"content": chunk},
		}))
	}

	var out bytes.Buffer
	_, err := StreamEventStreamAsAnthropicWithContext(context.Background(), stream, &out, "claude-sonnet-4-5", 9, KiroRequestContext{ThinkingEnabled: true})
	require.NoError(t, err)

	output := out.String()
	thinkingStart := strings.Index(output, `"type":"thinking"`)
	textDelta := strings.Index(output, `"text":"final"`)
	require.NotEqual(t, -1, thinkingStart)
	require.NotEqual(t, -1, textDelta)
	require.Less(t, thinkingStart, textDelta)
	require.NotContains(t, output, `\u003cthink`)
	require.NotContains(t, output, `\u003c/thinking\u003e`)
	require.NotContains(t, output, `"text":"\n\n"`)
}

func TestStreamEventStreamAsAnthropicTreatsThinkingTagsAsTextWhenDisabled(t *testing.T) {
	stream := bytes.NewBuffer(nil)
	_, _ = stream.Write(buildEventStreamFrame(t, "assistantResponseEvent", map[string]any{
		"assistantResponseEvent": map[string]any{
			"content": "<thinking>reason</thinking>\n\nfinal",
		},
	}))

	var out bytes.Buffer
	result, err := StreamEventStreamAsAnthropic(context.Background(), stream, &out, "claude-sonnet-4-5", 9)
	require.NoError(t, err)
	require.Equal(t, "end_turn", result.StopReason)

	output := out.String()
	require.Contains(t, output, `\u003cthinking\u003ereason\u003c/thinking\u003e`)
	require.NotContains(t, output, `"type":"thinking_delta"`)
}

func TestStreamEventStreamAsAnthropicIgnoresReasoningContentWhenThinkingDisabled(t *testing.T) {
	stream := bytes.NewBuffer(nil)
	_, _ = stream.Write(buildEventStreamFrame(t, "reasoningContentEvent", map[string]any{
		"reasoningContentEvent": map[string]any{"text": "hidden reasoning"},
	}))

	var out bytes.Buffer
	result, err := StreamEventStreamAsAnthropic(context.Background(), stream, &out, "claude-sonnet-4-5", 9)
	require.NoError(t, err)
	require.Equal(t, "end_turn", result.StopReason)
	require.NotContains(t, out.String(), "hidden reasoning")
	require.NotContains(t, out.String(), `"type":"thinking"`)
}

func TestBuildAssistantMessageStructUsesSpacePlaceholderForToolOnly(t *testing.T) {
	msg := gjson.Parse(`{
		"role":"assistant",
		"content":[
			{"type":"tool_use","id":"toolu_01ABC","name":"read_file","input":{"path":"/tmp/test.txt"}}
		]
	}`)

	result := buildAssistantMessageStruct(msg, nil)
	require.Equal(t, " ", result.Content)
	require.Len(t, result.ToolUses, 1)
	require.Equal(t, "read_file", result.ToolUses[0].Name)
	require.Equal(t, "/tmp/test.txt", result.ToolUses[0].Input["path"])
}

func TestBuildKiroPayloadAddsPlaceholderToolForHistoryToolUse(t *testing.T) {
	body := []byte(`{
		"model":"claude-sonnet-4-5",
		"messages":[
			{"role":"assistant","content":[{"type":"tool_use","id":"toolu_01","name":"read_file","input":{"path":"/tmp/a.txt"}}]},
			{"role":"user","content":[{"type":"tool_result","tool_use_id":"toolu_01","content":"ok"},{"type":"text","text":"continue"}]}
		]
	}`)

	payload, err := BuildKiroPayload(body, "claude-sonnet-4.5", "", "AI_EDITOR", nil)
	require.NoError(t, err)
	tools := gjson.GetBytes(payload, "conversationState.currentMessage.userInputMessage.userInputMessageContext.tools").Array()
	require.Len(t, tools, 1)
	require.Equal(t, "read_file", tools[0].Get("toolSpecification.name").String())
	require.Equal(t, "Tool used in conversation history", tools[0].Get("toolSpecification.description").String())
	require.Equal(t, "object", tools[0].Get("toolSpecification.inputSchema.json.type").String())
}

func TestBuildKiroPayloadNormalizesToolJSONSchema(t *testing.T) {
	body := []byte(`{
		"model":"claude-sonnet-4-5",
		"messages":[{"role":"user","content":"hello"}],
		"tools":[{
			"name":"bad_schema",
			"description":"bad schema",
			"input_schema":{
				"properties":null,
				"required":null,
				"additionalProperties":"sometimes",
				"items":{"properties":null,"required":[1,"ok"],"additionalProperties":7}
			}
		}]
	}`)

	payload, err := BuildKiroPayload(body, "claude-sonnet-4.5", "", "AI_EDITOR", nil)
	require.NoError(t, err)
	schema := gjson.GetBytes(payload, "conversationState.currentMessage.userInputMessage.userInputMessageContext.tools.0.toolSpecification.inputSchema.json")
	require.Equal(t, "object", schema.Get("type").String())
	require.True(t, schema.Get("properties").IsObject())
	require.True(t, schema.Get("required").IsArray())
	require.Len(t, schema.Get("required").Array(), 0)
	require.True(t, schema.Get("additionalProperties").Bool())
	require.Equal(t, "object", schema.Get("items.type").String())
	require.Equal(t, "ok", schema.Get("items.required.0").String())
	require.True(t, schema.Get("items.additionalProperties").Bool())
}

func TestBuildKiroPayloadFiltersCurrentOrphanToolResult(t *testing.T) {
	body := []byte(`{
		"model":"claude-sonnet-4-5",
		"messages":[{"role":"user","content":[{"type":"tool_result","tool_use_id":"missing","content":"orphaned"}]}]
	}`)

	payload, err := BuildKiroPayload(body, "claude-sonnet-4.5", "", "AI_EDITOR", nil)
	require.NoError(t, err)
	require.False(t, gjson.GetBytes(payload, "conversationState.currentMessage.userInputMessage.userInputMessageContext.toolResults").Exists())
}

func TestBuildKiroPayloadRemovesHistoryOrphanToolUse(t *testing.T) {
	body := []byte(`{
		"model":"claude-sonnet-4-5",
		"messages":[
			{"role":"assistant","content":[{"type":"tool_use","id":"toolu_orphan","name":"read_file","input":{"path":"/tmp/a.txt"}}]},
			{"role":"user","content":"continue"}
		]
	}`)

	payload, err := BuildKiroPayload(body, "claude-sonnet-4.5", "", "AI_EDITOR", nil)
	require.NoError(t, err)
	history := gjson.GetBytes(payload, "conversationState.history").Array()
	foundAssistantWithoutToolUses := false
	for _, msg := range history {
		if msg.Get("assistantResponseMessage").Exists() && msg.Get("assistantResponseMessage.content").String() == " " {
			foundAssistantWithoutToolUses = true
			require.False(t, msg.Get("assistantResponseMessage.toolUses").Exists())
		}
	}
	require.True(t, foundAssistantWithoutToolUses)
	require.False(t, gjson.GetBytes(payload, "conversationState.currentMessage.userInputMessage.userInputMessageContext.tools").Exists())
}

func TestMergeAdjacentMessagesUsesDoubleNewline(t *testing.T) {
	messages := gjson.Parse(`[
		{"role":"user","content":"first"},
		{"role":"user","content":"second"}
	]`).Array()

	merged := mergeAdjacentMessages(messages)
	require.Len(t, merged, 1)
	require.Equal(t, "first\n\nsecond", merged[0].Get("content.0.text").String())
}

func TestLongToolNamesUseHashSuffixAndDoNotCollide(t *testing.T) {
	nameA := strings.Repeat("tool_prefix_", 8) + "alpha"
	nameB := strings.Repeat("tool_prefix_", 8) + "bravo"
	shortA := shortenToolNameIfNeeded(nameA)
	shortB := shortenToolNameIfNeeded(nameB)

	require.Len(t, shortA, kiroMaxToolNameLen)
	require.Len(t, shortB, kiroMaxToolNameLen)
	require.NotEqual(t, shortA, shortB)
	require.Regexp(t, `_[0-9a-f]{8}$`, shortA)
	require.Regexp(t, `_[0-9a-f]{8}$`, shortB)
}

func TestBuildKiroPayloadMapsLongToolNameConsistently(t *testing.T) {
	longName := strings.Repeat("mcp__very_long_server__", 4) + "read_file"
	body := []byte(fmt.Sprintf(`{
		"model":"claude-sonnet-4-5",
		"system":"Follow tool choice.",
		"tool_choice":{"type":"tool","name":%q},
		"messages":[
			{"role":"assistant","content":[{"type":"tool_use","id":"toolu_01","name":%q,"input":{"path":"/tmp/a.txt"}}]},
			{"role":"user","content":[{"type":"tool_result","tool_use_id":"toolu_01","content":"ok"},{"type":"text","text":"continue"}]}
		],
		"tools":[{"name":%q,"description":"read","input_schema":{"type":"object","properties":{"path":{"type":"string"}}}}]
	}`, longName, longName, longName))

	result, err := BuildKiroPayloadWithContext(body, "claude-sonnet-4.5", "", "AI_EDITOR", nil)
	require.NoError(t, err)
	require.Len(t, result.Context.ToolNameMap, 1)
	var shortName string
	for short, original := range result.Context.ToolNameMap {
		shortName = short
		require.Equal(t, longName, original)
	}
	require.NotEmpty(t, shortName)
	require.Equal(t, shortName, gjson.GetBytes(result.Payload, "conversationState.currentMessage.userInputMessage.userInputMessageContext.tools.0.toolSpecification.name").String())
	require.Contains(t, gjson.GetBytes(result.Payload, "conversationState.history.0.userInputMessage.content").String(), "MUST use the tool named '"+shortName+"'")

	found := false
	for _, msg := range gjson.GetBytes(result.Payload, "conversationState.history").Array() {
		for _, toolUse := range msg.Get("assistantResponseMessage.toolUses").Array() {
			if toolUse.Get("toolUseId").String() == "toolu_01" {
				found = true
				require.Equal(t, shortName, toolUse.Get("name").String())
			}
		}
	}
	require.True(t, found)
}

func TestParseNonStreamingEventStreamRestoresShortToolName(t *testing.T) {
	longName := strings.Repeat("long_tool_name_", 6)
	shortName := shortenToolNameIfNeeded(longName)
	stream := bytes.NewBuffer(nil)
	_, _ = stream.Write(buildEventStreamFrame(t, "toolUseEvent", map[string]any{
		"toolUseEvent": map[string]any{
			"toolUseId": "toolu_long",
			"name":      shortName,
			"input":     `{"path":"/tmp/a.txt"}`,
			"stop":      true,
		},
	}))

	result, err := ParseNonStreamingEventStreamWithContext(stream, "claude-sonnet-4-5", KiroRequestContext{
		ToolNameMap: map[string]string{shortName: longName},
	})
	require.NoError(t, err)
	require.Equal(t, longName, gjson.GetBytes(result.ResponseBody, "content.0.name").String())
}

func TestStreamEventStreamAsAnthropicRestoresShortToolName(t *testing.T) {
	longName := strings.Repeat("long_tool_name_", 6)
	shortName := shortenToolNameIfNeeded(longName)
	stream := bytes.NewBuffer(nil)
	_, _ = stream.Write(buildEventStreamFrame(t, "toolUseEvent", map[string]any{
		"toolUseEvent": map[string]any{
			"toolUseId": "toolu_long",
			"name":      shortName,
			"input":     `{"path":"/tmp/a.txt"}`,
			"stop":      true,
		},
	}))

	var out bytes.Buffer
	_, err := StreamEventStreamAsAnthropicWithContext(context.Background(), stream, &out, "claude-sonnet-4-5", 1, KiroRequestContext{
		ToolNameMap: map[string]string{shortName: longName},
	})
	require.NoError(t, err)
	require.Contains(t, out.String(), `"name":"`+longName+`"`)
	require.NotContains(t, out.String(), `"name":"`+shortName+`"`)
}

func TestRepairJSONKeepsStringBracesWhileRepairingTrailingComma(t *testing.T) {
	raw := `{"key":"value with {nested}",}`
	repaired := repairJSON(raw)

	var parsed map[string]string
	require.NoError(t, json.Unmarshal([]byte(repaired), &parsed))
	require.Equal(t, "value with {nested}", parsed["key"])
}

func TestMapModel_MatchesKiroReferenceMapping(t *testing.T) {
	t.Parallel()

	cases := map[string]string{
		"claude-opus-4-7":                     "claude-opus-4.7",
		"claude-opus-4-7-thinking":            "claude-opus-4.7",
		"claude-opus-4.7":                     "claude-opus-4.7",
		"claude-sonnet-4-6":                   "claude-sonnet-4.6",
		"claude-sonnet-4-6-thinking":          "claude-sonnet-4.6",
		"claude-sonnet-4.6":                   "claude-sonnet-4.6",
		"claude-sonnet-4-5-20250929":          "claude-sonnet-4.5",
		"claude-sonnet-4-5-20250929-thinking": "claude-sonnet-4.5",
		"claude-sonnet-4.5":                   "claude-sonnet-4.5",
		"claude-opus-4-6":                     "claude-opus-4.6",
		"claude-opus-4-6-thinking":            "claude-opus-4.6",
		"claude-opus-4.6":                     "claude-opus-4.6",
		"claude-opus-4-5-20251101":            "claude-opus-4.5",
		"claude-opus-4-5-20251101-thinking":   "claude-opus-4.5",
		"claude-opus-4.5":                     "claude-opus-4.5",
		"claude-haiku-4-5-20251001":           "claude-haiku-4.5",
		"claude-haiku-4-5-20251001-thinking":  "claude-haiku-4.5",
		"claude-haiku-4.5":                    "claude-haiku-4.5",
	}

	for input, want := range cases {
		if got := MapModel(input); got != want {
			t.Fatalf("MapModel(%q) = %q, want %q", input, got, want)
		}
	}

	rejected := []string{
		"claude-sonnet-4-6-chat",
		" claude-sonnet-4-6-thinking-chat ",
		"claude-sonnet-4-6-agentic",
		" claude-sonnet-4-6-thinking-agentic ",
		"claude-3-5-sonnet-20241022",
		"claude-opus-4-20250514",
		"claude-sonnet-4",
		"claude-opus-4-5",
		"claude-sonnet-4-5",
		"claude-haiku-4-5",
	}
	for _, input := range rejected {
		if got := MapModel(input); got != "" {
			t.Fatalf("MapModel(%q) = %q, want empty", input, got)
		}
	}
}

func TestMapModel_ReturnsEmptyForUnsupportedModels(t *testing.T) {
	t.Parallel()

	cases := []string{
		"auto",
		"gpt-4",
		"gpt-4o",
		"deepseek-3-2",
		"minimax-m2-1",
		"qwen3-coder-next",
	}

	for _, input := range cases {
		if got := MapModel(input); got != "" {
			t.Fatalf("MapModel(%q) = %q, want empty string", input, got)
		}
	}
}

func buildEventStreamFrame(t *testing.T, eventType string, payload any) []byte {
	t.Helper()
	payloadBytes, err := json.Marshal(payload)
	require.NoError(t, err)

	headers := bytes.NewBuffer(nil)
	_ = headers.WriteByte(byte(len(":event-type")))
	_, _ = headers.WriteString(":event-type")
	_ = headers.WriteByte(7)
	require.NoError(t, binary.Write(headers, binary.BigEndian, uint16(len(eventType))))
	_, _ = headers.WriteString(eventType)

	totalLength := uint32(12 + headers.Len() + len(payloadBytes) + 4)
	frame := bytes.NewBuffer(nil)
	require.NoError(t, binary.Write(frame, binary.BigEndian, totalLength))
	require.NoError(t, binary.Write(frame, binary.BigEndian, uint32(headers.Len())))
	require.NoError(t, binary.Write(frame, binary.BigEndian, uint32(0)))
	_, _ = frame.Write(headers.Bytes())
	_, _ = frame.Write(payloadBytes)
	require.NoError(t, binary.Write(frame, binary.BigEndian, uint32(0)))
	return frame.Bytes()
}
