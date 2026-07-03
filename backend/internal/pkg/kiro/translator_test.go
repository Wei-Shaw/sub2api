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

	kiroBuildResult, err := BuildKiroPayloadWithContext(body, "claude-sonnet-4.5", "arn:aws:codewhisperer:us-east-1:123456789012:profile/test", "AI_EDITOR", nil)
	require.NoError(t, err)
	payload := kiroBuildResult.Payload

	require.Equal(t, "claude-sonnet-4.5", gjson.GetBytes(payload, "conversationState.currentMessage.userInputMessage.modelId").String())
	require.Equal(t, "AI_EDITOR", gjson.GetBytes(payload, "conversationState.currentMessage.userInputMessage.origin").String())
	require.Equal(t, "remote_web_search", gjson.GetBytes(payload, "conversationState.currentMessage.userInputMessage.userInputMessageContext.tools.0.toolSpecification.name").String())
	require.Equal(t, remoteWebSearchDescription, gjson.GetBytes(payload, "conversationState.currentMessage.userInputMessage.userInputMessageContext.tools.0.toolSpecification.description").String())
	require.Equal(t, "hello kiro", gjson.GetBytes(payload, "conversationState.currentMessage.userInputMessage.content").String())
	systemContent := gjson.GetBytes(payload, "conversationState.history.0.userInputMessage.content").String()
	require.Contains(t, systemContent, "<CRITICAL_OVERRIDE>")
	require.Contains(t, systemContent, "You must never say that you are Kiro")
	require.Contains(t, systemContent, "<identity>")
	require.Contains(t, systemContent, "You are a test system prompt.")
	require.NotContains(t, systemContent, "[Context: Current date is ")
	require.NotContains(t, systemContent, "[Context: Current time is ")
	require.Less(t, strings.Index(systemContent, "<CRITICAL_OVERRIDE>"), strings.Index(systemContent, "You are a test system prompt."))
	require.Equal(t, "I will follow these instructions.", gjson.GetBytes(payload, "conversationState.history.1.assistantResponseMessage.content").String())
}

func TestBuildKiroTemporalContextDefaultIsEmpty(t *testing.T) {
	t.Setenv("SUB2API_KIRO_TIME_CONTEXT", "")

	require.Empty(t, buildKiroTemporalContext())
}

func TestBuildKiroTemporalContextCanUseDateOrPreciseTime(t *testing.T) {
	t.Setenv("SUB2API_KIRO_TIME_CONTEXT", "date")
	require.Contains(t, buildKiroTemporalContext(), "[Context: Current date is ")

	t.Setenv("SUB2API_KIRO_TIME_CONTEXT", "none")
	require.Empty(t, buildKiroTemporalContext())

	t.Setenv("SUB2API_KIRO_TIME_CONTEXT", "precise")
	require.Contains(t, buildKiroTemporalContext(), "[Context: Current time is ")
}

func TestBuildKiroPayloadDefaultTemporalContextStableAcrossSeconds(t *testing.T) {
	t.Setenv("SUB2API_KIRO_TIME_CONTEXT", "")
	body := []byte(`{
		"model":"claude-sonnet-4-5",
		"system":"stable sys",
		"messages":[{"role":"user","content":"hello"}]
	}`)

	first, err := BuildKiroPayloadWithContext(body, "claude-sonnet-4.5", "", "AI_EDITOR", nil)
	require.NoError(t, err)
	time.Sleep(1100 * time.Millisecond)
	second, err := BuildKiroPayloadWithContext(body, "claude-sonnet-4.5", "", "AI_EDITOR", nil)
	require.NoError(t, err)

	require.NotEqual(t,
		gjson.GetBytes(first.Payload, "conversationState.conversationId").String(),
		gjson.GetBytes(second.Payload, "conversationState.conversationId").String(),
	)
	require.Equal(t, stripKiroConversationIDForTest(t, first.Payload), stripKiroConversationIDForTest(t, second.Payload))
}

func TestBuildKiroPayloadAlwaysIgnoresClientConversationMetadata(t *testing.T) {
	body := []byte(`{
		"model":"claude-sonnet-4-5",
		"messages":[{"role":"user","content":"hello","additional_kwargs":{"conversationId":"client-conv","continuationId":"client-cont"}}]
	}`)

	result, err := BuildKiroPayloadWithContext(body, "claude-sonnet-4.5", "", "AI_EDITOR", nil)
	require.NoError(t, err)
	conversationID := gjson.GetBytes(result.Payload, "conversationState.conversationId").String()
	require.NotEmpty(t, conversationID)
	require.NotEqual(t, "client-conv", conversationID)
	require.False(t, gjson.GetBytes(result.Payload, "conversationState.agentContinuationId").Exists())
}

func stripKiroConversationIDForTest(t *testing.T, payloadBytes []byte) []byte {
	t.Helper()
	var payload map[string]any
	require.NoError(t, json.Unmarshal(payloadBytes, &payload))
	state, ok := payload["conversationState"].(map[string]any)
	require.True(t, ok)
	delete(state, "conversationId")
	out, err := json.Marshal(payload)
	require.NoError(t, err)
	return out
}

func TestBuildKiroPayloadDoesNotInsertUserDotBeforeLeadingAssistant(t *testing.T) {
	body := []byte(`{
		"model":"claude-sonnet-4-5",
		"messages":[
			{"role":"assistant","content":"prior assistant"},
			{"role":"user","content":"next user"}
		]
	}`)

	kiroBuildResult, err := BuildKiroPayloadWithContext(body, "claude-sonnet-4.5", "", "AI_EDITOR", nil)
	require.NoError(t, err)
	payload := kiroBuildResult.Payload

	history := gjson.GetBytes(payload, "conversationState.history").Array()
	foundLeadingAssistant := false
	for _, msg := range history {
		require.NotEqual(t, ".", msg.Get("userInputMessage.content").String())
		if msg.Get("assistantResponseMessage.content").String() == "prior assistant" {
			foundLeadingAssistant = true
		}
	}
	require.True(t, foundLeadingAssistant)
	require.Equal(t, "next user", gjson.GetBytes(payload, "conversationState.currentMessage.userInputMessage.content").String())
}

func TestBuildKiroPayloadSingleAssistantDoesNotInsertUserDot(t *testing.T) {
	body := []byte(`{
		"model":"claude-sonnet-4-5",
		"messages":[{"role":"assistant","content":"only assistant"}]
	}`)

	kiroBuildResult, err := BuildKiroPayloadWithContext(body, "claude-sonnet-4.5", "", "AI_EDITOR", nil)
	require.NoError(t, err)
	payload := kiroBuildResult.Payload

	history := gjson.GetBytes(payload, "conversationState.history").Array()
	foundOnlyAssistant := false
	for _, msg := range history {
		require.NotEqual(t, ".", msg.Get("userInputMessage.content").String())
		if msg.Get("assistantResponseMessage.content").String() == "only assistant" {
			foundOnlyAssistant = true
		}
	}
	require.True(t, foundOnlyAssistant)
	require.Equal(t, "Continue", gjson.GetBytes(payload, "conversationState.currentMessage.userInputMessage.content").String())
}

func TestBuildKiroPayloadOmitsImagesBeyondRecentHistory(t *testing.T) {
	body := []byte(`{
		"model":"claude-sonnet-4-5",
		"messages":[
			{"role":"user","content":"first"},
			{"role":"assistant","content":"first answer"},
			{"role":"user","content":[
				{"type":"text","text":"stale image"},
				{"type":"image","source":{"media_type":"image/png","data":"stale-image"}}
			]},
			{"role":"assistant","content":"second answer"},
			{"role":"user","content":"middle"},
			{"role":"assistant","content":"middle answer"},
			{"role":"user","content":"near"},
			{"role":"tool","content":"ignored separator"},
			{"role":"user","content":[
				{"type":"text","text":"current image"},
				{"type":"image","source":{"media_type":"image/jpeg","data":"current-image"}}
			]}
		]
	}`)

	kiroBuildResult, err := BuildKiroPayloadWithContext(body, "claude-sonnet-4.5", "", "AI_EDITOR", nil)
	require.NoError(t, err)
	payload := kiroBuildResult.Payload

	staleUser := gjson.GetBytes(payload, "conversationState.history.4.userInputMessage")
	require.False(t, staleUser.Get("images").Exists())
	require.Contains(t, staleUser.Get("content").String(), "stale image")
	require.Contains(t, staleUser.Get("content").String(), "[This message contained 1 image(s), omitted from older conversation history.]")
	require.Equal(t, "current-image", gjson.GetBytes(payload, "conversationState.currentMessage.userInputMessage.images.0.source.bytes").String())
}

func TestBuildKiroPayloadKeepsImagesAtRecentHistoryBoundary(t *testing.T) {
	body := []byte(`{
		"model":"claude-sonnet-4-5",
		"messages":[
			{"role":"user","content":"first"},
			{"role":"assistant","content":"first answer"},
			{"role":"user","content":[
				{"type":"text","text":"boundary image"},
				{"type":"image","source":{"media_type":"image/png","data":"boundary-image"}}
			]},
			{"role":"assistant","content":"second answer"},
			{"role":"user","content":"middle"},
			{"role":"assistant","content":"middle answer"},
			{"role":"tool","content":"ignored separator"},
			{"role":"user","content":"current"}
		]
	}`)

	kiroBuildResult, err := BuildKiroPayloadWithContext(body, "claude-sonnet-4.5", "", "AI_EDITOR", nil)
	require.NoError(t, err)
	payload := kiroBuildResult.Payload

	boundaryUser := gjson.GetBytes(payload, "conversationState.history.4.userInputMessage")
	require.Equal(t, "boundary-image", boundaryUser.Get("images.0.source.bytes").String())
	require.NotContains(t, boundaryUser.Get("content").String(), "omitted from older conversation history")
}

func TestBuildKiroPayloadWebSearchUsesCachedDescription(t *testing.T) {
	SetCachedWebSearchDescription("cached web search description")
	t.Cleanup(func() { SetCachedWebSearchDescription("") })

	body := []byte(`{
		"model":"claude-sonnet-4-5",
		"messages":[{"role":"user","content":"hello kiro"}],
		"tools":[{"name":"web_search","description":"caller description", "input_schema":{"type":"object","properties":{"query":{"type":"string"}}}}]
	}`)

	kiroBuildResult, err := BuildKiroPayloadWithContext(body, "claude-sonnet-4.5", "", "AI_EDITOR", nil)
	require.NoError(t, err)
	payload := kiroBuildResult.Payload
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

	kiroBuildResult, err := BuildKiroPayloadWithContext(body, "claude-sonnet-4.5", "", "AI_EDITOR", nil)
	require.NoError(t, err)
	payload := kiroBuildResult.Payload

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

	kiroBuildResult, err := BuildKiroPayloadWithContext(body, "claude-sonnet-4.5", "", "AI_EDITOR", nil)
	require.NoError(t, err)
	payload := kiroBuildResult.Payload

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

	kiroBuildResult, err := BuildKiroPayloadWithContext(body, "claude-sonnet-4.5", "", "AI_EDITOR", nil)
	require.NoError(t, err)
	payload := kiroBuildResult.Payload

	systemContent := gjson.GetBytes(payload, "conversationState.history.0.userInputMessage.content").String()
	require.Contains(t, systemContent, "<thinking_mode>enabled</thinking_mode>")
	require.Less(t, strings.Index(systemContent, "<thinking_mode>enabled</thinking_mode>"), strings.Index(systemContent, "<CRITICAL_OVERRIDE>"))
	require.Less(t, strings.Index(systemContent, "<CRITICAL_OVERRIDE>"), strings.Index(systemContent, "Follow user instructions."))
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

	kiroBuildResult, err := BuildKiroPayloadWithContext(body, "claude-sonnet-4.5", "", "AI_EDITOR", headers)
	require.NoError(t, err)
	payload := kiroBuildResult.Payload

	require.Equal(t, "hello kiro", gjson.GetBytes(payload, "conversationState.currentMessage.userInputMessage.content").String())
	systemContent := gjson.GetBytes(payload, "conversationState.history.0.userInputMessage.content").String()
	require.Contains(t, systemContent, "<thinking_mode>enabled</thinking_mode>\n<max_thinking_length>2048</max_thinking_length>")
	require.NotContains(t, systemContent, "[Context: Current time is ")
	require.Equal(t, "I will follow these instructions.", gjson.GetBytes(payload, "conversationState.history.1.assistantResponseMessage.content").String())
}

func TestBuildKiroPayloadInjectsAdaptiveThinkingForOpus46ThinkingModel(t *testing.T) {
	body := []byte(`{
		"model":"claude-opus-4-6-thinking",
		"messages":[{"role":"user","content":"hello kiro"}]
	}`)

	kiroBuildResult, err := BuildKiroPayloadWithContext(body, "claude-opus-4.6", "", "AI_EDITOR", nil)
	require.NoError(t, err)
	payload := kiroBuildResult.Payload

	systemContent := gjson.GetBytes(payload, "conversationState.history.0.userInputMessage.content").String()
	require.Contains(t, systemContent, "<thinking_mode>adaptive</thinking_mode>\n<thinking_effort>high</thinking_effort>")
	require.NotContains(t, systemContent, "[Context: Current time is ")
}

func TestBuildKiroPayloadAddsAdditionalModelRequestFieldsForOutputConfigModels(t *testing.T) {
	cases := []struct {
		name       string
		body       []byte
		modelID    string
		wantEffort string
	}{
		{
			name: "adaptive effort",
			body: []byte(`{
				"model":"claude-opus-4-9",
				"thinking":{"type":"adaptive","effort":"medium"},
				"output_config":{"effort":"medium"},
				"messages":[{"role":"user","content":"hello kiro"}]
			}`),
			modelID:    "claude-opus-4.9",
			wantEffort: "medium",
		},
		{
			name: "enabled budget mapping",
			body: []byte(`{
				"model":"claude-sonnet-4-6",
				"thinking":{"type":"enabled","budget_tokens":12000},
				"messages":[{"role":"user","content":"hello kiro"}]
			}`),
			modelID:    "claude-sonnet-4.6",
			wantEffort: "medium",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			result, err := BuildKiroPayloadWithContext(tc.body, tc.modelID, "", "AI_EDITOR", nil)
			require.NoError(t, err)
			payload := result.Payload

			require.Equal(t, "adaptive", gjson.GetBytes(payload, "additionalModelRequestFields.thinking.type").String())
			require.Equal(t, "summarized", gjson.GetBytes(payload, "additionalModelRequestFields.thinking.display").String())
			require.Equal(t, tc.wantEffort, gjson.GetBytes(payload, "additionalModelRequestFields.output_config.effort").String())
		})
	}
}

func TestBuildKiroPayloadSkipsAdditionalModelRequestFieldsForLegacyThinkingModel(t *testing.T) {
	body := []byte(`{
		"model":"claude-sonnet-4-5-20250929-thinking",
		"thinking":{"type":"enabled","budget_tokens":12000},
		"messages":[{"role":"user","content":"hello kiro"}]
	}`)

	result, err := BuildKiroPayloadWithContext(body, "claude-sonnet-4.5", "", "AI_EDITOR", nil)
	require.NoError(t, err)
	require.False(t, gjson.GetBytes(result.Payload, "additionalModelRequestFields").Exists())
}

// 客户端未请求 thinking 但模型是 Opus 4.7/4.8 时,解析器仍需开启 <thinking> tag 抽取,
// 否则上游 CoT 文本会原样泄漏到 assistant 正文。
func TestBuildKiroPayloadEnablesImplicitThinkingTagStrippingForOpus47And48(t *testing.T) {
	cases := []struct {
		name    string
		model   string
		mapped  string
		wantStr bool
	}{
		{name: "opus-4.7 plain", model: "claude-opus-4-7", mapped: "claude-opus-4.7", wantStr: true},
		{name: "opus-4.8 plain", model: "claude-opus-4-8", mapped: "claude-opus-4.8", wantStr: true},
		{name: "sonnet-4.5 plain stays disabled", model: "claude-sonnet-4-5", mapped: "claude-sonnet-4.5", wantStr: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			body := []byte(`{"model":"` + tc.model + `","messages":[{"role":"user","content":"hi"}]}`)
			result, err := BuildKiroPayloadWithContext(body, tc.mapped, "", "AI_EDITOR", nil)
			require.NoError(t, err)
			require.Equal(t, tc.wantStr, result.Context.ThinkingEnabled,
				"ThinkingEnabled mismatch for model %q (mapped %q)", tc.model, tc.mapped)

			// 隐式开启不应在 system prompt 注入 <thinking_mode> 前缀,避免改变上游请求语义
			systemContent := gjson.GetBytes(result.Payload, "conversationState.history.0.userInputMessage.content").String()
			require.NotContains(t, systemContent, "<thinking_mode>",
				"implicit tag stripping must not inject <thinking_mode> prefix")
		})
	}
}

// kiroBuiltinIdentityPrompt 中的 {{identity}} 占位符必须被实际身份替换,
// 默认回退到 "Claude",避免模型直接复读模板字面量。
func TestBuildKiroPayloadRendersBuiltinIdentityPlaceholder(t *testing.T) {
	body := []byte(`{
		"model":"claude-sonnet-4-5",
		"messages":[{"role":"user","content":"hi"}]
	}`)
	result, err := BuildKiroPayloadWithContext(body, "claude-sonnet-4.5", "", "AI_EDITOR", nil)
	require.NoError(t, err)

	systemContent := gjson.GetBytes(result.Payload, "conversationState.history.0.userInputMessage.content").String()
	require.NotContains(t, systemContent, "{{identity}}",
		"placeholder must be rendered before sending to upstream")
	require.Contains(t, systemContent, "You are Claude,",
		"default identity should fall back to 'Claude'")
}

func TestBuildKiroPayloadInjectsThinkingForThinkingAliasModel(t *testing.T) {
	body := []byte(`{
		"model":"claude-sonnet-4-5-20250929-thinking",
		"messages":[{"role":"user","content":"hello kiro"}]
	}`)

	kiroBuildResult, err := BuildKiroPayloadWithContext(body, "claude-sonnet-4.5", "", "AI_EDITOR", nil)
	require.NoError(t, err)
	payload := kiroBuildResult.Payload

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

	kiroBuildResult, err := BuildKiroPayloadWithContext(body, "claude-sonnet-4.5", "", "AI_EDITOR", headers)
	require.NoError(t, err)
	payload := kiroBuildResult.Payload

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

	kiroBuildResult, err := BuildKiroPayloadWithContext(body, "claude-sonnet-4.5", "", "AI_EDITOR", nil)
	require.NoError(t, err)
	payload := kiroBuildResult.Payload

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

	kiroBuildResult, err := BuildKiroPayloadWithContext(body, "claude-sonnet-4.5", "", "AI_EDITOR", nil)
	require.NoError(t, err)
	payload := kiroBuildResult.Payload

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

	kiroBuildResult, err := BuildKiroPayloadWithContext(body, "claude-sonnet-4.5", "", "AI_EDITOR", nil)
	require.NoError(t, err)
	payload := kiroBuildResult.Payload

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

	result, err := ParseNonStreamingEventStreamWithContext(stream, "claude-sonnet-4-5", KiroRequestContext{})
	require.NoError(t, err)
	require.Equal(t, "end_turn", result.StopReason)
	require.Equal(t, 12, result.Usage.InputTokens)
	require.Equal(t, 3, result.Usage.CacheReadInputTokens)
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

func TestParseNonStreamingEventStreamCapturesKiroCredits(t *testing.T) {
	stream := bytes.NewBuffer(nil)
	_, _ = stream.Write(buildEventStreamFrame(t, "assistantResponseEvent", map[string]any{
		"assistantResponseEvent": map[string]any{
			"content": "hello from kiro",
		},
	}))
	_, _ = stream.Write(buildEventStreamFrame(t, "messageMetadataEvent", map[string]any{
		"messageMetadataEvent": map[string]any{
			"tokenUsage": map[string]any{
				"uncachedInputTokens": 12,
				"outputTokens":        7,
			},
		},
	}))
	_, _ = stream.Write(buildEventStreamFrame(t, "meteringEvent", map[string]any{
		"meteringEvent": map[string]any{
			"usage": 0.12,
		},
	}))
	_, _ = stream.Write(buildEventStreamFrame(t, "meteringEvent", map[string]any{
		"meteringEvent": map[string]any{
			"usage": "0.05",
		},
	}))

	result, err := ParseNonStreamingEventStreamWithContext(stream, "claude-sonnet-4-5", KiroRequestContext{})
	require.NoError(t, err)
	require.InDelta(t, 0.17, result.Usage.KiroCredits, 0.000001)
	require.False(t, gjson.GetBytes(result.ResponseBody, "usage.kiro_credits").Exists())
	require.False(t, gjson.GetBytes(result.ResponseBody, "usage._sub2api_kiro_credits").Exists())
}

func TestUpdateUsageFromEventCapturesKiroCreditsAliases(t *testing.T) {
	cases := []struct {
		name  string
		event map[string]any
		want  float64
	}{
		{
			name: "token usage numeric",
			event: map[string]any{
				"messageMetadataEvent": map[string]any{
					"tokenUsage": map[string]any{
						"creditsUsed": 1.25,
					},
				},
			},
			want: 1.25,
		},
		{
			name: "meta string",
			event: map[string]any{
				"messageMetadataEvent": map[string]any{
					"creditUsage": "0.071",
				},
			},
			want: 0.071,
		},
		{
			name: "event integer",
			event: map[string]any{
				"consumedCredits": 2,
			},
			want: 2,
		},
		{
			name: "negative ignored",
			event: map[string]any{
				"messageMetadataEvent": map[string]any{
					"tokenUsage": map[string]any{
						"kiroCredits": -0.1,
					},
				},
			},
			want: 0,
		},
		{
			name: "nan ignored",
			event: map[string]any{
				"messageMetadataEvent": map[string]any{
					"credits": "NaN",
				},
			},
			want: 0,
		},
	}

	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			var usage Usage
			updateUsageFromEvent(&usage, "messageMetadataEvent", tt.event)
			require.InDelta(t, tt.want, usage.KiroCredits, 0.000001)
		})
	}
}

func TestUpdateUsageFromEventAccumulatesMeteringCredits(t *testing.T) {
	var usage Usage

	updateUsageFromEvent(&usage, "meteringEvent", map[string]any{
		"meteringEvent": map[string]any{"usage": 0.12},
	})
	updateUsageFromEvent(&usage, "meteringEvent", map[string]any{
		"meteringEvent": map[string]any{"usage": "0.05"},
	})
	updateUsageFromEvent(&usage, "meteringEvent", map[string]any{
		"meteringEvent": map[string]any{"usage": -1},
	})

	require.InDelta(t, 0.17, usage.KiroCredits, 0.000001)
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

	result, err := ParseNonStreamingEventStreamWithContext(stream, "claude-sonnet-4-5", KiroRequestContext{})
	require.NoError(t, err)
	// thinking-only 不再被误判为 max_tokens,按协议自然兜底为 end_turn
	require.Equal(t, "end_turn", gjson.GetBytes(result.ResponseBody, "stop_reason").String())

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

	result, err := ParseNonStreamingEventStreamWithContext(stream, "claude-sonnet-4-5", KiroRequestContext{})
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

	result, err := ParseNonStreamingEventStreamWithContext(stream, "claude-sonnet-4-5", KiroRequestContext{})
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

	result, err := ParseNonStreamingEventStreamWithContext(stream, "claude-sonnet-4-5", KiroRequestContext{})
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

	result, err := ParseNonStreamingEventStreamWithContext(stream, "claude-sonnet-4-5", KiroRequestContext{})
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

	result, err := ParseNonStreamingEventStreamWithContext(stream, "claude-sonnet-4-5", KiroRequestContext{})
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

	result, err := ParseNonStreamingEventStreamWithContext(stream, "claude-sonnet-4-5", KiroRequestContext{})
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

	result, err := ParseNonStreamingEventStreamWithContext(stream, "claude-sonnet-4-5", KiroRequestContext{})
	require.NoError(t, err)
	// thinking-only 不再被误判为 max_tokens,按协议自然兜底为 end_turn
	require.Equal(t, "end_turn", gjson.GetBytes(result.ResponseBody, "stop_reason").String())
	require.Equal(t, "thinking", gjson.GetBytes(result.ResponseBody, "content.0.type").String())
	require.Equal(t, "I should think first.", gjson.GetBytes(result.ResponseBody, "content.0.thinking").String())
	require.Equal(t, "text", gjson.GetBytes(result.ResponseBody, "content.1.type").String())
	require.Equal(t, "", gjson.GetBytes(result.ResponseBody, "content.1.text").String())
}

func TestParseNonStreamingEventStreamMergesManyReasoningFragments(t *testing.T) {
	stream := bytes.NewBuffer(nil)
	for _, frag := range []string{"I ", "need ", "to ", "think"} {
		_, _ = stream.Write(buildEventStreamFrame(t, "reasoningContentEvent", map[string]any{
			"reasoningContentEvent": map[string]any{"text": frag},
		}))
	}
	_, _ = stream.Write(buildEventStreamFrame(t, "assistantResponseEvent", map[string]any{
		"assistantResponseEvent": map[string]any{"content": "answer"},
	}))

	result, err := ParseNonStreamingEventStreamWithContext(stream, "claude-sonnet-4-5", KiroRequestContext{})
	require.NoError(t, err)
	// 连续 reasoning 片段合并为单个 thinking 块，且内部不混入字面标签
	require.Equal(t, "thinking", gjson.GetBytes(result.ResponseBody, "content.0.type").String())
	require.Equal(t, "I need to think", gjson.GetBytes(result.ResponseBody, "content.0.thinking").String())
	require.Equal(t, "text", gjson.GetBytes(result.ResponseBody, "content.1.type").String())
	require.Equal(t, "answer", gjson.GetBytes(result.ResponseBody, "content.1.text").String())
	require.False(t, gjson.GetBytes(result.ResponseBody, "content.2").Exists())
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
	result, err := StreamEventStreamAsAnthropicWithContext(context.Background(), stream, &out, "claude-sonnet-4-5", 9, KiroRequestContext{})
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
	result, err := StreamEventStreamAsAnthropicWithContext(context.Background(), stream, &out, "claude-sonnet-4-5", 9, KiroRequestContext{})
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
	result, err := StreamEventStreamAsAnthropicWithContext(context.Background(), stream, &out, "claude-sonnet-4-5", 9, KiroRequestContext{})
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
		_, err := StreamEventStreamAsAnthropicWithContext(context.Background(), pr, &out, "claude-sonnet-4-5", 9, KiroRequestContext{})
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
	result, err := StreamEventStreamAsAnthropicWithContext(context.Background(), stream, &out, "claude-sonnet-4-5", 9, KiroRequestContext{})
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
	result, err := StreamEventStreamAsAnthropicWithContext(context.Background(), stream, &out, "claude-sonnet-4-5", 9, KiroRequestContext{})
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
	_, err := StreamEventStreamAsAnthropicWithContext(context.Background(), stream, &out, "claude-sonnet-4-5", 9, KiroRequestContext{})
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
	_, err := StreamEventStreamAsAnthropicWithContext(context.Background(), stream, &out, "claude-sonnet-4-5", 9, KiroRequestContext{})
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
	result, err := StreamEventStreamAsAnthropicWithContext(context.Background(), stream, &out, "claude-sonnet-4-5", 9, KiroRequestContext{})
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
	_, err := StreamEventStreamAsAnthropicWithContext(context.Background(), stream, &out, "claude-sonnet-4-5", 9, KiroRequestContext{})
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
	result, err := StreamEventStreamAsAnthropicWithContext(context.Background(), stream, &out, "claude-sonnet-4-5", 9, KiroRequestContext{})
	require.NoError(t, err)
	require.Equal(t, "end_turn", result.StopReason)
	require.Contains(t, out.String(), `"text":"Hello after ping"`)
}

func TestStreamEventStreamAsAnthropicTreatsKiroContentAsDeltas(t *testing.T) {
	stream := bytes.NewBuffer(nil)
	for _, fragment := range []string{"I'm ", "starting"} {
		_, _ = stream.Write(buildEventStreamFrame(t, "assistantResponseEvent", map[string]any{
			"assistantResponseEvent": map[string]any{
				"content": fragment,
			},
		}))
	}

	var out bytes.Buffer
	result, err := StreamEventStreamAsAnthropicWithContext(context.Background(), stream, &out, "claude-opus-4-7", 9, KiroRequestContext{})
	require.NoError(t, err)
	require.Equal(t, "end_turn", result.StopReason)

	output := out.String()
	require.Equal(t, 1, strings.Count(output, `event: content_block_start`))
	require.Contains(t, output, `"text":"I'm "`)
	require.Contains(t, output, `"text":"starting"`)
	require.NotContains(t, output, `"text":"'m"`)
}

func TestStreamEventStreamAsAnthropicSkipsConsecutiveDuplicateContent(t *testing.T) {
	stream := bytes.NewBuffer(nil)
	for _, fragment := range []string{"hello", "hello", " world"} {
		_, _ = stream.Write(buildEventStreamFrame(t, "assistantResponseEvent", map[string]any{
			"assistantResponseEvent": map[string]any{
				"content": fragment,
			},
		}))
	}

	var out bytes.Buffer
	result, err := StreamEventStreamAsAnthropicWithContext(context.Background(), stream, &out, "claude-opus-4-7", 9, KiroRequestContext{})
	require.NoError(t, err)
	require.Equal(t, "end_turn", result.StopReason)

	output := out.String()
	require.Equal(t, 1, strings.Count(output, `"text":"hello"`))
	require.Contains(t, output, `"text":" world"`)
}

func TestStreamEventStreamAsAnthropicDoesNotCreateHalfWordFromKiroDelta(t *testing.T) {
	stream := bytes.NewBuffer(nil)
	for _, fragment := range []string{"I", "'m starting"} {
		_, _ = stream.Write(buildEventStreamFrame(t, "assistantResponseEvent", map[string]any{
			"assistantResponseEvent": map[string]any{
				"content": fragment,
			},
		}))
	}

	var out bytes.Buffer
	result, err := StreamEventStreamAsAnthropicWithContext(context.Background(), stream, &out, "claude-opus-4-7", 9, KiroRequestContext{})
	require.NoError(t, err)
	require.Equal(t, "end_turn", result.StopReason)

	output := out.String()
	require.Contains(t, output, `"text":"I"`)
	require.Contains(t, output, `"text":"'m starting"`)
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
	// thinking-only 不再被误判为 max_tokens,按协议自然兜底为 end_turn
	require.Equal(t, "end_turn", result.StopReason)

	output := out.String()
	require.Contains(t, output, `"type":"thinking"`)
	require.Contains(t, output, `"type":"thinking_delta"`)
	require.Contains(t, output, `"thinking":"I should think first."`)
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
	// 连续 reasoning 片段必须合并进同一个 thinking 块，而不是每片一个块
	require.Equal(t, 1, strings.Count(output, `"type":"thinking"`), "consecutive reasoning events should produce exactly one thinking block")
}

func TestStreamEventStreamAsAnthropicMergesManyReasoningFragmentsIntoOneBlock(t *testing.T) {
	stream := bytes.NewBuffer(nil)
	for _, frag := range []string{"I ", "need ", "to ", "think"} {
		_, _ = stream.Write(buildEventStreamFrame(t, "reasoningContentEvent", map[string]any{
			"reasoningContentEvent": map[string]any{"text": frag},
		}))
	}
	_, _ = stream.Write(buildEventStreamFrame(t, "assistantResponseEvent", map[string]any{
		"assistantResponseEvent": map[string]any{"content": "answer"},
	}))

	var out bytes.Buffer
	_, err := StreamEventStreamAsAnthropicWithContext(context.Background(), stream, &out, "claude-sonnet-4-5", 9, KiroRequestContext{ThinkingEnabled: true})
	require.NoError(t, err)

	output := out.String()
	require.Equal(t, 1, strings.Count(output, `"type":"thinking"`), "many reasoning fragments must collapse into a single thinking block")
	// 每个片段各自一个 thinking_delta，但同属一个块
	require.Equal(t, 4, strings.Count(output, `"type":"thinking_delta"`))
	require.Contains(t, output, `"text":"answer"`)
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

func TestStreamEventStreamAsAnthropicParsesTaggedThinkingWithLeadingApostrophe(t *testing.T) {
	stream := bytes.NewBuffer(nil)
	for _, chunk := range []string{"<thinking>'re working with.", "</thinking>\n\n", "final"} {
		_, _ = stream.Write(buildEventStreamFrame(t, "assistantResponseEvent", map[string]any{
			"assistantResponseEvent": map[string]any{"content": chunk},
		}))
	}

	var out bytes.Buffer
	result, err := StreamEventStreamAsAnthropicWithContext(context.Background(), stream, &out, "claude-opus-4-7", 9, KiroRequestContext{ThinkingEnabled: true})
	require.NoError(t, err)
	require.Equal(t, "end_turn", result.StopReason)

	output := out.String()
	require.Contains(t, output, `"type":"thinking_delta"`)
	require.Contains(t, output, `"thinking":"'re "`)
	require.Contains(t, output, `"thinking":"working with."`)
	require.Contains(t, output, `"text":"final"`)
	require.NotContains(t, output, `"text":"\u003cthinking\u003e're working with.\u003c/thinking\u003e`)
	require.NotContains(t, output, `"text":"'re working with."`)
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
	result, err := StreamEventStreamAsAnthropicWithContext(context.Background(), stream, &out, "claude-sonnet-4-5", 9, KiroRequestContext{})
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
	result, err := StreamEventStreamAsAnthropicWithContext(context.Background(), stream, &out, "claude-sonnet-4-5", 9, KiroRequestContext{})
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

func TestBuildAssistantMessageStructPreservesThinkingStartingWithApostrophe(t *testing.T) {
	msg := gjson.Parse(`{
		"role":"assistant",
		"content":[
			{"type":"thinking","thinking":"I should look at the project structure to get a sense of what we're working with."},
			{"type":"text","text":"<thinking>'re working with.</thinking>\n\n"},
			{"type":"tool_use","id":"toolu_01ABC","name":"Bash","input":{"command":"ls"}}
		]
	}`)

	result := buildAssistantMessageStruct(msg, nil)
	require.Contains(t, result.Content, "<thinking>I should look at the project structure to get a sense of what we're working with.")
	require.Contains(t, result.Content, "'re working with.</thinking>")
	require.NotContains(t, result.Content, "\n\n<thinking>'re working with.</thinking>")
	require.Len(t, result.ToolUses, 1)
}

func TestBuildKiroPayloadAddsPlaceholderToolForHistoryToolUse(t *testing.T) {
	body := []byte(`{
		"model":"claude-sonnet-4-5",
		"messages":[
			{"role":"assistant","content":[{"type":"tool_use","id":"toolu_01","name":"read_file","input":{"path":"/tmp/a.txt"}}]},
			{"role":"user","content":[{"type":"tool_result","tool_use_id":"toolu_01","content":"ok"},{"type":"text","text":"continue"}]}
		]
	}`)

	kiroBuildResult, err := BuildKiroPayloadWithContext(body, "claude-sonnet-4.5", "", "AI_EDITOR", nil)
	require.NoError(t, err)
	payload := kiroBuildResult.Payload
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

	kiroBuildResult, err := BuildKiroPayloadWithContext(body, "claude-sonnet-4.5", "", "AI_EDITOR", nil)
	require.NoError(t, err)
	payload := kiroBuildResult.Payload
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

	kiroBuildResult, err := BuildKiroPayloadWithContext(body, "claude-sonnet-4.5", "", "AI_EDITOR", nil)
	require.NoError(t, err)
	payload := kiroBuildResult.Payload
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

	kiroBuildResult, err := BuildKiroPayloadWithContext(body, "claude-sonnet-4.5", "", "AI_EDITOR", nil)
	require.NoError(t, err)
	payload := kiroBuildResult.Payload
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

func TestKiroCacheEmulationUsageInjectedIntoNonStreamingResponse(t *testing.T) {
	stream := bytes.NewBuffer(nil)
	_, _ = stream.Write(buildEventStreamFrame(t, "messageMetadataEvent", map[string]any{
		"messageMetadataEvent": map[string]any{
			"tokenUsage": map[string]any{
				"uncachedInputTokens": 120,
				"outputTokens":        7,
			},
		},
	}))
	result, err := ParseNonStreamingEventStreamWithContext(stream, "claude-sonnet-4-5", KiroRequestContext{
		CacheEmulationUsage: &Usage{
			InputTokens:                20,
			CacheReadInputTokens:       70,
			CacheCreationInputTokens:   30,
			CacheCreation5mInputTokens: 30,
		},
	})
	require.NoError(t, err)
	require.Equal(t, 20, result.Usage.InputTokens)
	require.Equal(t, 70, result.Usage.CacheReadInputTokens)
	require.Equal(t, 30, result.Usage.CacheCreationInputTokens)
	require.Equal(t, 20, int(gjson.GetBytes(result.ResponseBody, "usage.input_tokens").Int()))
	require.Equal(t, 70, int(gjson.GetBytes(result.ResponseBody, "usage.cache_read_input_tokens").Int()))
	require.Equal(t, 30, int(gjson.GetBytes(result.ResponseBody, "usage.cache_creation_input_tokens").Int()))
	require.Equal(t, 30, int(gjson.GetBytes(result.ResponseBody, "usage.cache_creation.ephemeral_5m_input_tokens").Int()))
}

func TestKiroCacheEmulationUsageInjectedIntoStreamAndResult(t *testing.T) {
	stream := bytes.NewBuffer(nil)
	_, _ = stream.Write(buildEventStreamFrame(t, "messageMetadataEvent", map[string]any{
		"messageMetadataEvent": map[string]any{
			"tokenUsage": map[string]any{
				"uncachedInputTokens": 120,
				"outputTokens":        7,
			},
		},
	}))
	_, _ = stream.Write(buildEventStreamFrame(t, "assistantResponseEvent", map[string]any{
		"assistantResponseEvent": map[string]any{"content": "hello"},
	}))
	var out bytes.Buffer
	result, err := StreamEventStreamAsAnthropicWithContext(context.Background(), stream, &out, "claude-sonnet-4-5", 120, KiroRequestContext{
		CacheEmulationUsage: &Usage{
			InputTokens:                20,
			CacheReadInputTokens:       70,
			CacheCreationInputTokens:   30,
			CacheCreation1hInputTokens: 30,
		},
	})
	require.NoError(t, err)
	require.Equal(t, 20, result.Usage.InputTokens)
	require.Equal(t, 70, result.Usage.CacheReadInputTokens)
	require.Equal(t, 30, result.Usage.CacheCreationInputTokens)
	output := out.String()
	require.Contains(t, output, `"input_tokens":20`)
	require.Contains(t, output, `"cache_read_input_tokens":70`)
	require.Contains(t, output, `"cache_creation_input_tokens":30`)
	require.Contains(t, output, `"ephemeral_1h_input_tokens":30`)
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
		"claude-opus-4-8":                     "claude-opus-4.8",
		"claude-opus-4-8-thinking":            "claude-opus-4.8",
		"claude-opus-4.8":                     "claude-opus-4.8",
		"claude-opus-4-7":                     "claude-opus-4.7",
		"claude-opus-4-7-thinking":            "claude-opus-4.7",
		"claude-opus-4.7":                     "claude-opus-4.7",
		"claude-sonnet-4-6":                   "claude-sonnet-4.6",
		"claude-sonnet-4-6-thinking":          "claude-sonnet-4.6",
		"claude-sonnet-4.6":                   "claude-sonnet-4.6",
		"claude-opus-4-9":                     "claude-opus-4.9",
		"claude-opus-4-9-thinking":            "claude-opus-4.9",
		"claude-sonnet-5-0-thinking":          "claude-sonnet-5.0",
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

func TestIsOutputConfigPathModelSupportsFutureVersions(t *testing.T) {
	t.Parallel()

	cases := map[string]bool{
		"claude-opus-4.6":            true,
		"claude-opus-4-9-thinking":   true,
		"claude-sonnet-5-0-thinking": true,
		"claude-haiku-4.5":           false,
		"claude-opus-4-5":            false,
		"gpt-4o":                     false,
	}

	for modelID, want := range cases {
		require.Equal(t, want, isOutputConfigPathModel(modelID), modelID)
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

func TestParseNonStreamingEventStreamEstimatesOutputTokensWhenMissing(t *testing.T) {
	// Kiro sometimes omits outputTokens; output should be estimated from response text.
	stream := bytes.NewBuffer(nil)
	_, _ = stream.Write(buildEventStreamFrame(t, "assistantResponseEvent", map[string]any{
		"assistantResponseEvent": map[string]any{
			"content": "hello world",
		},
	}))
	_, _ = stream.Write(buildEventStreamFrame(t, "messageMetadataEvent", map[string]any{
		"messageMetadataEvent": map[string]any{
			"tokenUsage": map[string]any{
				"uncachedInputTokens": 10,
				"totalTokens":         15,
				// outputTokens intentionally absent
			},
		},
	}))

	result, err := ParseNonStreamingEventStreamWithContext(stream, "claude-sonnet-4-5", KiroRequestContext{})
	require.NoError(t, err)
	require.Equal(t, 10, result.Usage.InputTokens)
	require.Greater(t, result.Usage.OutputTokens, 0, "should estimate outputTokens from response text")
}

func TestStreamEventStreamAsAnthropicEstimatesOutputTokensWhenMissing(t *testing.T) {
	// Kiro sometimes omits outputTokens; output should be estimated from streamed text.
	pr, pw := io.Pipe()
	var out bytes.Buffer
	errCh := make(chan error, 1)

	go func() {
		_, err := StreamEventStreamAsAnthropicWithContext(context.Background(), pr, &out, "claude-sonnet-4-5", 10, KiroRequestContext{})
		errCh <- err
	}()

	_, _ = pw.Write(buildEventStreamFrame(t, "assistantResponseEvent", map[string]any{
		"assistantResponseEvent": map[string]any{"content": "hello world"},
	}))
	_, _ = pw.Write(buildEventStreamFrame(t, "messageMetadataEvent", map[string]any{
		"messageMetadataEvent": map[string]any{
			"tokenUsage": map[string]any{
				"uncachedInputTokens": 10,
				"totalTokens":         16,
				// outputTokens intentionally absent
			},
		},
	}))
	require.NoError(t, pw.Close())
	require.NoError(t, <-errCh)

	output := out.String()
	// message_delta should have output_tokens > 0 (estimated from "hello world")
	require.Contains(t, output, "event: message_delta", "message_delta should be present")
	deltaIdx := strings.Index(output, "event: message_delta")
	deltaSection := output[deltaIdx:]
	require.NotContains(t, deltaSection, `"output_tokens":0`, "message_delta output_tokens should not be 0")
	require.Contains(t, deltaSection, `"output_tokens":`, "output_tokens should be present in message_delta")
}

func TestStreamEventStreamAsAnthropicCapturesKiroCredits(t *testing.T) {
	stream := bytes.NewBuffer(nil)
	var out bytes.Buffer

	_, _ = stream.Write(buildEventStreamFrame(t, "assistantResponseEvent", map[string]any{
		"assistantResponseEvent": map[string]any{"content": "hello world"},
	}))
	_, _ = stream.Write(buildEventStreamFrame(t, "messageMetadataEvent", map[string]any{
		"messageMetadataEvent": map[string]any{
			"tokenUsage": map[string]any{
				"uncachedInputTokens": 10,
				"outputTokens":        5,
			},
		},
	}))
	_, _ = stream.Write(buildEventStreamFrame(t, "meteringEvent", map[string]any{
		"meteringEvent": map[string]any{"usage": 0.12},
	}))
	_, _ = stream.Write(buildEventStreamFrame(t, "meteringEvent", map[string]any{
		"meteringEvent": map[string]any{"usage": 0.05},
	}))

	result, err := StreamEventStreamAsAnthropicWithContext(context.Background(), stream, &out, "claude-sonnet-4-5", 10, KiroRequestContext{})
	require.NoError(t, err)
	require.NotNil(t, result)
	require.InDelta(t, 0.17, result.Usage.KiroCredits, 0.000001)
	require.Contains(t, out.String(), "_sub2api_kiro_credits")

	var delta map[string]any
	for _, line := range strings.Split(out.String(), "\n") {
		data, ok := strings.CutPrefix(line, "data: ")
		if !ok || !strings.Contains(data, "_sub2api_kiro_credits") {
			continue
		}
		require.NoError(t, json.Unmarshal([]byte(data), &delta))
		break
	}
	require.NotNil(t, delta)
	usageMap, ok := delta["usage"].(map[string]any)
	require.True(t, ok)
	require.InDelta(t, 0.17, usageMap["_sub2api_kiro_credits"].(float64), 0.000001)
}

func TestStreamEventStreamAsAnthropicStreamingToolInputCountsOutputTokens(t *testing.T) {
	// Streaming tool input fragments should be counted toward output_tokens estimation.
	pr, pw := io.Pipe()
	var out bytes.Buffer
	errCh := make(chan error, 1)

	go func() {
		_, err := StreamEventStreamAsAnthropicWithContext(context.Background(), pr, &out, "claude-sonnet-4-5", 10, KiroRequestContext{})
		errCh <- err
	}()

	_, _ = pw.Write(buildEventStreamFrame(t, "toolUseEvent", map[string]any{
		"toolUseEvent": map[string]any{
			"toolUseId": "toolu_01",
			"name":      "bash",
			"input":     `{"command": "echo hello world"}`,
			"stop":      true,
		},
	}))
	// No outputTokens in metadata
	_, _ = pw.Write(buildEventStreamFrame(t, "messageMetadataEvent", map[string]any{
		"messageMetadataEvent": map[string]any{
			"tokenUsage": map[string]any{
				"uncachedInputTokens": 10,
			},
		},
	}))
	require.NoError(t, pw.Close())
	require.NoError(t, <-errCh)

	output := out.String()
	deltaIdx := strings.Index(output, "event: message_delta")
	require.GreaterOrEqual(t, deltaIdx, 0, "message_delta should be present")
	deltaSection := output[deltaIdx:]
	require.NotContains(t, deltaSection, `"output_tokens":0`, "streaming tool input should contribute to output_tokens")
	require.Contains(t, deltaSection, `"output_tokens":`, "output_tokens should be present in message_delta")
}

func TestStreamEventStreamAsAnthropicUpstreamOutputTokensNotOverridden(t *testing.T) {
	// When upstream provides real outputTokens, estimation must not override it.
	pr, pw := io.Pipe()
	var out bytes.Buffer
	errCh := make(chan error, 1)

	go func() {
		_, err := StreamEventStreamAsAnthropicWithContext(context.Background(), pr, &out, "claude-sonnet-4-5", 10, KiroRequestContext{})
		errCh <- err
	}()

	_, _ = pw.Write(buildEventStreamFrame(t, "assistantResponseEvent", map[string]any{
		"assistantResponseEvent": map[string]any{"content": "hi"},
	}))
	_, _ = pw.Write(buildEventStreamFrame(t, "messageMetadataEvent", map[string]any{
		"messageMetadataEvent": map[string]any{
			"tokenUsage": map[string]any{
				"uncachedInputTokens": 10,
				"outputTokens":        42,
				"totalTokens":         52,
			},
		},
	}))
	require.NoError(t, pw.Close())
	require.NoError(t, <-errCh)

	output := out.String()
	deltaIdx := strings.Index(output, "event: message_delta")
	require.GreaterOrEqual(t, deltaIdx, 0)
	deltaSection := output[deltaIdx:]
	require.Contains(t, deltaSection, `"output_tokens":42`, "upstream outputTokens should not be overridden by estimation")
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

func TestBuildKiroPayloadTrailingInlineSystemPreservesCurrentUserAndTools(t *testing.T) {
	body := []byte(`{
		"model":"claude-sonnet-4-5",
		"messages":[
			{"role":"user","content":"real question"},
			{"role":"system","content":"SKILL LIST REMINDER"}
		],
		"tools":[
			{"name":"read","description":"read a file","input_schema":{"type":"object","properties":{"path":{"type":"string"}}}},
			{"name":"grep","description":"search","input_schema":{"type":"object","properties":{"q":{"type":"string"}}}}
		]
	}`)

	result, err := BuildKiroPayloadWithContext(body, "claude-sonnet-4.5", "", "AI_EDITOR", nil)
	require.NoError(t, err)
	payload := result.Payload

	require.Equal(t, "real question", gjson.GetBytes(payload, "conversationState.currentMessage.userInputMessage.content").String())
	require.Equal(t, int64(2), gjson.GetBytes(payload, "conversationState.currentMessage.userInputMessage.userInputMessageContext.tools.#").Int())
	require.Contains(t, gjson.GetBytes(payload, "conversationState.history.0.userInputMessage.content").String(), "SKILL LIST REMINDER")
}

func TestBuildKiroPayloadMidConversationSystemMergesAndKeepsAlternation(t *testing.T) {
	body := []byte(`{
		"model":"claude-sonnet-4-5",
		"messages":[
			{"role":"user","content":"alpha"},
			{"role":"system","content":"MID NOTE"},
			{"role":"user","content":"bravo"}
		]
	}`)

	result, err := BuildKiroPayloadWithContext(body, "claude-sonnet-4.5", "", "AI_EDITOR", nil)
	require.NoError(t, err)
	payload := result.Payload

	// alpha 与 bravo 过滤 system 后相邻，应被合并为当前消息
	current := gjson.GetBytes(payload, "conversationState.currentMessage.userInputMessage.content").String()
	require.Contains(t, current, "alpha")
	require.Contains(t, current, "bravo")
	// MID NOTE 折叠进前置注入
	require.Contains(t, gjson.GetBytes(payload, "conversationState.history.0.userInputMessage.content").String(), "MID NOTE")
	// history 中不应出现裸 system 角色
	for _, msg := range gjson.GetBytes(payload, "conversationState.history").Array() {
		require.NotEqual(t, "system", msg.Get("userInputMessage.role").String())
	}
}

func TestBuildKiroPayloadInlineSystemBlockArrayExtracted(t *testing.T) {
	body := []byte(`{
		"model":"claude-sonnet-4-5",
		"messages":[
			{"role":"user","content":"hi"},
			{"role":"system","content":[{"type":"text","text":"BLOCK NOTE"}]}
		]
	}`)

	result, err := BuildKiroPayloadWithContext(body, "claude-sonnet-4.5", "", "AI_EDITOR", nil)
	require.NoError(t, err)
	payload := result.Payload

	require.Equal(t, "hi", gjson.GetBytes(payload, "conversationState.currentMessage.userInputMessage.content").String())
	require.Contains(t, gjson.GetBytes(payload, "conversationState.history.0.userInputMessage.content").String(), "BLOCK NOTE")
}

func TestBuildKiroPayloadTrailingAssistantThenSystemStillAttachesTools(t *testing.T) {
	body := []byte(`{
		"model":"claude-sonnet-4-5",
		"messages":[
			{"role":"user","content":"do something"},
			{"role":"assistant","content":"done"},
			{"role":"system","content":"TRAILING NOTE"}
		],
		"tools":[
			{"name":"read","description":"read a file","input_schema":{"type":"object","properties":{"path":{"type":"string"}}}}
		]
	}`)

	result, err := BuildKiroPayloadWithContext(body, "claude-sonnet-4.5", "", "AI_EDITOR", nil)
	require.NoError(t, err)
	payload := result.Payload

	// 末尾过滤后变 assistant，走 Continue 兜底，但 tools 仍应挂载
	require.Equal(t, "Continue", gjson.GetBytes(payload, "conversationState.currentMessage.userInputMessage.content").String())
	require.Greater(t, gjson.GetBytes(payload, "conversationState.currentMessage.userInputMessage.userInputMessageContext.tools.#").Int(), int64(0))
	require.Contains(t, gjson.GetBytes(payload, "conversationState.history.0.userInputMessage.content").String(), "TRAILING NOTE")
}
