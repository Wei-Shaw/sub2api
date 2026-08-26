package kiro

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
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
	require.Contains(t, ua1, "KiroIDE-0.12.301")
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
	require.Contains(t, systemContent, "If no identity is provided, say that you are Claude.")
	require.Contains(t, systemContent, "You are Claude, a senior software engineer")
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

func TestBuildKiroPayloadDoesNotInjectClaudeThinkingTagsForGPTModels(t *testing.T) {
	body := []byte(`{
		"model":"gpt-5.6-terra",
		"thinking":{"type":"enabled","budget_tokens":16000},
		"messages":[{"role":"user","content":"hello gpt"}]
	}`)
	headers := http.Header{}
	headers.Set("Anthropic-Beta", "interleaved-thinking-2025-05-14")

	kiroBuildResult, err := BuildKiroPayloadWithContext(body, "gpt-5.6-terra", "", "AI_EDITOR", headers)
	require.NoError(t, err)

	systemContent := gjson.GetBytes(kiroBuildResult.Payload, "conversationState.history.0.userInputMessage.content").String()
	require.Contains(t, systemContent, "You are Claude, a senior software engineer")
	require.NotContains(t, systemContent, "<thinking_mode>")
	require.NotContains(t, systemContent, "<max_thinking_length>")
	require.NotContains(t, systemContent, "<thinking_effort>")
	require.False(t, kiroBuildResult.Context.ThinkingEnabled)
	require.False(t, gjson.GetBytes(kiroBuildResult.Payload, "additionalModelRequestFields").Exists())
}

// GPT-5.6 一律不下发 additionalModelRequestFields，即使客户端显式请求了
// reasoning effort。原因：Kiro 协议里没有 reasoning.effort 字段（下发会被上游
// 静默忽略），而已确认接受 additionalModelRequestFields 的模型白名单不含 GPT
// 系列，向未确认模型下发会触发 400 "additionalModelRequestFields is not supported"。
// 待抓包确认字段名与模型支持情况后再实现。
func TestBuildKiroPayloadDoesNotSendAdditionalFieldsForGPTModels(t *testing.T) {
	bodies := []string{
		`{"model":"gpt-5.6-sol","reasoning_effort":"high","messages":[{"role":"user","content":"hi"}]}`,
		`{"model":"gpt-5.6-sol","reasoning":{"effort":"low"},"messages":[{"role":"user","content":"hi"}]}`,
		`{"model":"gpt-5.6-sol","output_config":{"effort":"medium"},"messages":[{"role":"user","content":"hi"}]}`,
		`{"model":"gpt-5.6-sol","reasoning_effort":"max","messages":[{"role":"user","content":"hi"}]}`,
		`{"model":"gpt-5.6-sol","thinking":{"type":"enabled","budget_tokens":16000},"messages":[{"role":"user","content":"hi"}]}`,
		`{"model":"gpt-5.6-sol","messages":[{"role":"user","content":"hi"}]}`,
	}
	for _, body := range bodies {
		result, err := BuildKiroPayloadWithContext([]byte(body), "gpt-5.6-sol", "", "AI_EDITOR", nil)
		require.NoError(t, err)
		require.False(t, gjson.GetBytes(result.Payload, "additionalModelRequestFields").Exists(),
			"GPT 模型不得下发 additionalModelRequestFields: %s", body)
	}

	// 对照：Claude 4.6+ 的 output_config 路径不受影响。
	claude, err := BuildKiroPayloadWithContext(
		[]byte(`{"model":"claude-opus-4-6-thinking","messages":[{"role":"user","content":"hi"}]}`),
		"claude-opus-4.6", "", "AI_EDITOR", nil)
	require.NoError(t, err)
	require.Equal(t, "high", gjson.GetBytes(claude.Payload, "additionalModelRequestFields.output_config.effort").String())
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

func TestBuildKiroPayloadInjectsAdaptiveThinkingForOpus5ThinkingModel(t *testing.T) {
	body := []byte(`{
		"model":"claude-opus-5-thinking",
		"messages":[{"role":"user","content":"hello kiro"}]
	}`)

	kiroBuildResult, err := BuildKiroPayloadWithContext(body, "claude-opus-5", "", "AI_EDITOR", nil)
	require.NoError(t, err)
	payload := kiroBuildResult.Payload

	systemContent := gjson.GetBytes(payload, "conversationState.history.0.userInputMessage.content").String()
	require.Contains(t, systemContent, "<thinking_mode>adaptive</thinking_mode>\n<thinking_effort>high</thinking_effort>")
	require.Equal(t, "adaptive", gjson.GetBytes(payload, "additionalModelRequestFields.thinking.type").String())
	require.Equal(t, "high", gjson.GetBytes(payload, "additionalModelRequestFields.output_config.effort").String())
	require.True(t, kiroBuildResult.Context.ThinkingEnabled)
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
		{name: "opus-5 plain", model: "claude-opus-5", mapped: "claude-opus-5", wantStr: true},
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

func TestParseNonStreamingEventStreamPreservesLargeIntegerInMapInput(t *testing.T) {
	stream := bytes.NewBuffer(nil)
	_, _ = stream.Write(buildEventStreamFrame(t, "toolUseEvent", map[string]any{
		"toolUseEvent": map[string]any{
			"toolUseId": "toolu_non_stream_map_large_integer",
			"name":      "custom_tool",
			"input": map[string]any{
				"id": json.Number("9007199254740993"),
			},
			"stop": true,
		},
	}))

	result, err := ParseNonStreamingEventStreamWithContext(stream, "claude-sonnet-4-5", KiroRequestContext{})
	require.NoError(t, err)
	require.Equal(t, "tool_use", result.StopReason)
	require.Equal(t, "9007199254740993", gjson.GetBytes(result.ResponseBody, "content.0.input.id").Raw)
}

// Kiro 上游只发 meteringEvent(credits),不发 tokenUsage,所以非流式解析出的
// InputTokens 恒为 0。流式路径靠 inputTokens 参数种入初值,非流式没有对应入口,
// 需由 requestCtx.EstimatedInputTokens 兜底,否则响应体 usage.input_tokens 为 0。
func TestParseNonStreamingEventStreamFallsBackToEstimatedInputTokens(t *testing.T) {
	// 复刻真实 Kiro 流：仅正文 + credits，无 tokenUsage。
	newStream := func() *bytes.Buffer {
		b := bytes.NewBuffer(nil)
		_, _ = b.Write(buildEventStreamFrame(t, "assistantResponseEvent", map[string]any{
			"assistantResponseEvent": map[string]any{"content": "OK"},
		}))
		_, _ = b.Write(buildEventStreamFrame(t, "meteringEvent", map[string]any{
			"meteringEvent": map[string]any{"unit": "credit", "usage": 0.0283},
		}))
		return b
	}

	// 无兜底（零值）时保持原行为：输出 0。
	bare, err := ParseNonStreamingEventStreamWithContext(newStream(), "gpt-5.6-sol", KiroRequestContext{})
	require.NoError(t, err)
	require.Equal(t, 0, bare.Usage.InputTokens)
	require.Equal(t, int64(0), gjson.GetBytes(bare.ResponseBody, "usage.input_tokens").Int())

	// 有兜底时填入预估值，响应体同步生效。
	fallback, err := ParseNonStreamingEventStreamWithContext(newStream(), "gpt-5.6-sol", KiroRequestContext{
		EstimatedInputTokens: 22,
	})
	require.NoError(t, err)
	require.Equal(t, 22, fallback.Usage.InputTokens)
	require.Equal(t, int64(22), gjson.GetBytes(fallback.ResponseBody, "usage.input_tokens").Int())

	// 缓存模拟生效时其取值优先，兜底不得覆盖（207 = 预估减去缓存部分）。
	withCache, err := ParseNonStreamingEventStreamWithContext(newStream(), "gpt-5.6-sol", KiroRequestContext{
		EstimatedInputTokens: 1962,
		CacheEmulationUsage: &Usage{
			InputTokens:                207,
			CacheCreationInputTokens:   1755,
			CacheCreation5mInputTokens: 1755,
		},
	})
	require.NoError(t, err)
	require.Equal(t, 207, withCache.Usage.InputTokens)
	require.Equal(t, int64(207), gjson.GetBytes(withCache.ResponseBody, "usage.input_tokens").Int())
	require.Equal(t, int64(1755), gjson.GetBytes(withCache.ResponseBody, "usage.cache_creation_input_tokens").Int())
}

func TestParseNonStreamingEventStreamRejectsTrailingJSONValueInToolInput(t *testing.T) {
	stream := bytes.NewBuffer(nil)
	_, _ = stream.Write(buildEventStreamFrame(t, "toolUseEvent", map[string]any{
		"toolUseEvent": map[string]any{
			"toolUseId": "toolu_non_stream_trailing_input",
			"name":      "custom_tool",
			"input":     `{"value":"valid first object"} {}`,
			"stop":      true,
		},
	}))

	result, err := ParseNonStreamingEventStreamWithContext(stream, "claude-sonnet-4-5", KiroRequestContext{})
	require.NoError(t, err)
	require.Equal(t, "end_turn", result.StopReason)
	require.NotContains(t, string(result.ResponseBody), `"id":"toolu_non_stream_trailing_input"`)
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
	require.Equal(t, 1, strings.Count(output, `"type":"input_json_delta"`))
	partial := extractStreamedToolInputJSON(t, output, "toolu_stream")
	var input map[string]any
	require.NoError(t, json.Unmarshal([]byte(partial), &input))
	require.Equal(t, map[string]any{"path": "/tmp/a.txt", "content": "hello"}, input)
	require.Contains(t, output, `event: content_block_stop`)
}

func TestStreamEventStreamAsAnthropicEmitsEmptyInputToolUse(t *testing.T) {
	const toolUseID = "toolu_exit_plan_mode"
	stream := bytes.NewBuffer(buildEventStreamFrame(t, "toolUseEvent", map[string]any{
		"toolUseEvent": map[string]any{
			"toolUseId": toolUseID,
			"name":      "ExitPlanMode",
			"stop":      true,
		},
	}))

	var out bytes.Buffer
	result, err := StreamEventStreamAsAnthropicWithContext(context.Background(), stream, &out, "claude-opus-4-6", 9, KiroRequestContext{})
	require.NoError(t, err)
	require.Equal(t, "tool_use", result.StopReason)

	output := out.String()
	require.Equal(t, 1, strings.Count(output, `"id":"`+toolUseID+`"`))
	require.Contains(t, output, `"name":"ExitPlanMode"`)
	require.Contains(t, output, `"stop_reason":"tool_use"`)
	require.JSONEq(t, `{}`, extractStreamedToolInputJSON(t, output, toolUseID))
}

func TestStreamEventStreamAsAnthropicAcceptsOpenCodeWriteFilePath(t *testing.T) {
	const toolUseID = "toolu_opencode_write"
	stream := bytes.NewBuffer(nil)
	_, _ = stream.Write(buildEventStreamFrame(t, "toolUseEvent", map[string]any{
		"name":      "write",
		"toolUseId": toolUseID,
	}))
	for _, fragment := range []string{
		`{"fileP`,
		`ath":"/tmp/hello",`,
		`"content":"hello"}`,
	} {
		_, _ = stream.Write(buildEventStreamFrame(t, "toolUseEvent", map[string]any{
			"name":      "write",
			"toolUseId": toolUseID,
			"input":     fragment,
		}))
	}
	_, _ = stream.Write(buildEventStreamFrame(t, "toolUseEvent", map[string]any{
		"name":      "write",
		"toolUseId": toolUseID,
		"stop":      true,
	}))

	var out bytes.Buffer
	result, err := StreamEventStreamAsAnthropicWithContext(context.Background(), stream, &out, "claude-opus-4-6", 9, KiroRequestContext{})
	require.NoError(t, err)
	require.Equal(t, "tool_use", result.StopReason)

	output := out.String()
	require.Equal(t, 1, strings.Count(output, `"id":"`+toolUseID+`"`))
	require.Contains(t, output, `"name":"write"`)
	require.Contains(t, output, `"stop_reason":"tool_use"`)
	require.JSONEq(t, `{"filePath":"/tmp/hello","content":"hello"}`, extractStreamedToolInputJSON(t, output, toolUseID))
}

func TestStreamEventStreamAsAnthropicUsesToolStopReasonWhenValidToolWasEmitted(t *testing.T) {
	stream := bytes.NewBuffer(nil)
	_, _ = stream.Write(buildEventStreamFrame(t, "toolUseEvent", map[string]any{
		"stopReason": "end_turn",
		"toolUseEvent": map[string]any{
			"toolUseId": "toolu_valid_end_turn",
			"name":      "write_file",
			"input":     `{"path":"main.go","content":"package main"}`,
			"stop":      true,
		},
	}))

	var out bytes.Buffer
	result, err := StreamEventStreamAsAnthropicWithContext(context.Background(), stream, &out, "claude-sonnet-4-5", 9, KiroRequestContext{})
	require.NoError(t, err)
	require.Equal(t, "tool_use", result.StopReason)
	require.Contains(t, out.String(), `"id":"toolu_valid_end_turn"`)
	require.Contains(t, out.String(), `"stop_reason":"tool_use"`)
}

func TestStreamEventStreamAsAnthropicPreservesMaxTokensWhenValidToolWasEmitted(t *testing.T) {
	stream := bytes.NewBuffer(nil)
	_, _ = stream.Write(buildEventStreamFrame(t, "toolUseEvent", map[string]any{
		"stopReason": "max_tokens",
		"toolUseEvent": map[string]any{
			"toolUseId": "toolu_valid_max_tokens",
			"name":      "write_file",
			"input":     `{"path":"main.go","content":"package main"}`,
			"stop":      true,
		},
	}))

	_, _ = stream.Write(buildEventStreamFrame(t, "messageMetadataEvent", map[string]any{
		"stopReason": "end_turn",
	}))

	var out bytes.Buffer
	result, err := StreamEventStreamAsAnthropicWithContext(context.Background(), stream, &out, "claude-sonnet-4-5", 9, KiroRequestContext{})
	require.NoError(t, err)
	require.Equal(t, "max_tokens", result.StopReason)
	require.Contains(t, out.String(), `"id":"toolu_valid_max_tokens"`)
	require.Contains(t, out.String(), `"stop_reason":"max_tokens"`)
}

func TestStreamEventStreamAsAnthropicPreservesStopSequenceWhenValidToolWasEmitted(t *testing.T) {
	stream := bytes.NewBuffer(nil)
	_, _ = stream.Write(buildEventStreamFrame(t, "assistantResponseEvent", map[string]any{
		"assistantResponseEvent": map[string]any{
			"content": "before<STOP>after",
		},
	}))
	_, _ = stream.Write(buildEventStreamFrame(t, "toolUseEvent", map[string]any{
		"toolUseEvent": map[string]any{
			"toolUseId": "toolu_valid_stop_sequence",
			"name":      "custom_tool",
			"input":     `{"ok":true}`,
			"stop":      true,
		},
	}))

	_, _ = stream.Write(buildEventStreamFrame(t, "messageMetadataEvent", map[string]any{
		"stopReason": "end_turn",
	}))

	var out bytes.Buffer
	result, err := StreamEventStreamAsAnthropicWithContext(context.Background(), stream, &out, "claude-sonnet-4-5", 9, KiroRequestContext{
		StopSequences: []string{"<STOP>"},
	})
	require.NoError(t, err)
	require.Equal(t, "stop_sequence", result.StopReason)
	require.Contains(t, out.String(), `"id":"toolu_valid_stop_sequence"`)
	require.Contains(t, out.String(), `"stop_reason":"stop_sequence"`)
	messageDelta := extractSSEEventData(t, out.String(), "message_delta")
	require.Equal(t, "<STOP>", gjson.GetBytes(messageDelta, "delta.stop_sequence").String())
	require.NotContains(t, out.String(), "after")
}

func TestStreamEventStreamAsAnthropicSkipsIncompleteToolUseFragment(t *testing.T) {
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
	require.Equal(t, "end_turn", result.StopReason)
	require.NotContains(t, out.String(), `"id":"toolu_incomplete"`)
	require.NotContains(t, out.String(), `"type":"input_json_delta"`)
}

func TestStreamEventStreamAsAnthropicSkipsIncompleteToolUseDespiteUpstreamToolUseStopReason(t *testing.T) {
	stream := bytes.NewBuffer(nil)
	_, _ = stream.Write(buildEventStreamFrame(t, "toolUseEvent", map[string]any{
		"stopReason": "tool_use",
		"toolUseEvent": map[string]any{
			"toolUseId": "toolu_incomplete_stop_reason",
			"name":      "write_file",
			"input":     `{"path":`,
			"stop":      true,
		},
	}))

	var out bytes.Buffer
	result, err := StreamEventStreamAsAnthropicWithContext(context.Background(), stream, &out, "claude-sonnet-4-5", 9, KiroRequestContext{})
	require.NoError(t, err)
	require.Equal(t, "end_turn", result.StopReason)
	require.NotContains(t, out.String(), `"id":"toolu_incomplete_stop_reason"`)
}

func TestStreamEventStreamAsAnthropicKeepsToolNameFromSeparateStartFrame(t *testing.T) {
	stream := bytes.NewBuffer(nil)
	_, _ = stream.Write(buildEventStreamFrame(t, "toolUseEvent", map[string]any{
		"toolUseEvent": map[string]any{
			"toolUseId": "toolu_separate_name",
			"name":      "write_file",
		},
	}))
	_, _ = stream.Write(buildEventStreamFrame(t, "toolUseEvent", map[string]any{
		"toolUseEvent": map[string]any{
			"toolUseId": "toolu_separate_name",
			"input":     `{"path":"main.go","content":"package main"}`,
			"stop":      true,
		},
	}))

	var out bytes.Buffer
	result, err := StreamEventStreamAsAnthropicWithContext(context.Background(), stream, &out, "claude-sonnet-4-5", 9, KiroRequestContext{})
	require.NoError(t, err)
	require.Equal(t, "tool_use", result.StopReason)
	require.Contains(t, out.String(), `"id":"toolu_separate_name"`)
	require.Contains(t, out.String(), `"name":"write_file"`)
	require.Equal(t, `{"content":"package main","path":"main.go"}`, extractStreamedToolInputJSON(t, out.String(), "toolu_separate_name"))
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

func TestStreamEventStreamAsAnthropicSkipsPayloadWithTrailingJSONValue(t *testing.T) {
	payload := []byte(`{"toolUseEvent":{"toolUseId":"toolu_trailing_payload","name":"custom_tool","input":{"ok":true},"stop":true}} {}`)
	stream := bytes.NewBuffer(buildRawEventStreamFrame(t, "toolUseEvent", payload))

	var out bytes.Buffer
	result, err := StreamEventStreamAsAnthropicWithContext(context.Background(), stream, &out, "claude-sonnet-4-5", 9, KiroRequestContext{})
	require.NoError(t, err)
	require.Equal(t, "end_turn", result.StopReason)
	require.NotContains(t, out.String(), `"id":"toolu_trailing_payload"`)
}

func TestStreamEventStreamAsAnthropicPreservesLargeIntegerInMapInput(t *testing.T) {
	stream := bytes.NewBuffer(nil)
	_, _ = stream.Write(buildEventStreamFrame(t, "toolUseEvent", map[string]any{
		"toolUseEvent": map[string]any{
			"toolUseId": "toolu_map_large_integer",
			"name":      "custom_tool",
			"input": map[string]any{
				"id": json.Number("9007199254740993"),
			},
			"stop": true,
		},
	}))

	var out bytes.Buffer
	result, err := StreamEventStreamAsAnthropicWithContext(context.Background(), stream, &out, "claude-sonnet-4-5", 9, KiroRequestContext{})
	require.NoError(t, err)
	require.Equal(t, "tool_use", result.StopReason)
	partial := extractStreamedToolInputJSON(t, out.String(), "toolu_map_large_integer")
	require.Equal(t, `{"id":9007199254740993}`, partial)
}

func TestStreamEventStreamAsAnthropicMapSnapshotReplacesFragments(t *testing.T) {
	stream := bytes.NewBuffer(nil)
	_, _ = stream.Write(buildEventStreamFrame(t, "toolUseEvent", map[string]any{
		"toolUseEvent": map[string]any{
			"toolUseId": "toolu_snapshot",
			"name":      "remote_web_search",
			"input":     `{"query":"stale`,
		},
	}))
	_, _ = stream.Write(buildEventStreamFrame(t, "toolUseEvent", map[string]any{
		"toolUseEvent": map[string]any{
			"toolUseId": "toolu_snapshot",
			"name":      "remote_web_search",
			"input":     map[string]any{"query": "golang"},
			"stop":      true,
		},
	}))

	var out bytes.Buffer
	result, err := StreamEventStreamAsAnthropicWithContext(context.Background(), stream, &out, "claude-sonnet-4-5", 9, KiroRequestContext{})
	require.NoError(t, err)
	require.Equal(t, "tool_use", result.StopReason)
	partial := extractStreamedToolInputJSON(t, out.String(), "toolu_snapshot")
	require.JSONEq(t, `{"query":"golang"}`, partial)
	require.NotContains(t, partial, "stale")
}

func TestStreamEventStreamAsAnthropicSkipsOversizedToolInput(t *testing.T) {
	stream := bytes.NewBuffer(nil)
	fragmentSize := maxEventMsgSize/2 + 1024
	_, _ = stream.Write(buildEventStreamFrame(t, "toolUseEvent", map[string]any{
		"toolUseEvent": map[string]any{
			"toolUseId": "toolu_oversized",
			"name":      "ExitPlanMode",
			"input":     `{"plan":"` + strings.Repeat("a", fragmentSize),
		},
	}))
	_, _ = stream.Write(buildEventStreamFrame(t, "toolUseEvent", map[string]any{
		"toolUseEvent": map[string]any{
			"toolUseId": "toolu_oversized",
			"name":      "ExitPlanMode",
			"input":     strings.Repeat("b", fragmentSize) + `"}`,
			"stop":      true,
		},
	}))

	var out bytes.Buffer
	result, err := StreamEventStreamAsAnthropicWithContext(context.Background(), stream, &out, "claude-sonnet-4-5", 9, KiroRequestContext{})
	require.NoError(t, err)
	require.Equal(t, "end_turn", result.StopReason)
	require.Zero(t, result.Usage.OutputTokens)
	require.NotContains(t, out.String(), `"id":"toolu_oversized"`)
	require.NotContains(t, out.String(), `"type":"input_json_delta"`)
}

func TestStreamEventStreamAsAnthropicMapSnapshotRecoversOversizedFragments(t *testing.T) {
	stream := bytes.NewBuffer(nil)
	fragmentSize := maxEventMsgSize/2 + 1024
	_, _ = stream.Write(buildEventStreamFrame(t, "toolUseEvent", map[string]any{
		"toolUseEvent": map[string]any{
			"toolUseId": "toolu_oversized_snapshot_recovery",
			"name":      "custom_tool",
			"input":     `{"value":"` + strings.Repeat("a", fragmentSize),
		},
	}))
	_, _ = stream.Write(buildEventStreamFrame(t, "toolUseEvent", map[string]any{
		"toolUseEvent": map[string]any{
			"toolUseId": "toolu_oversized_snapshot_recovery",
			"name":      "custom_tool",
			"input":     strings.Repeat("b", fragmentSize),
		},
	}))
	_, _ = stream.Write(buildEventStreamFrame(t, "toolUseEvent", map[string]any{
		"toolUseEvent": map[string]any{
			"toolUseId": "toolu_oversized_snapshot_recovery",
			"name":      "custom_tool",
			"input":     map[string]any{"value": "snapshot"},
			"stop":      true,
		},
	}))

	var out bytes.Buffer
	result, err := StreamEventStreamAsAnthropicWithContext(context.Background(), stream, &out, "claude-sonnet-4-5", 9, KiroRequestContext{})
	require.NoError(t, err)
	require.Equal(t, "tool_use", result.StopReason)
	require.JSONEq(t, `{"value":"snapshot"}`, extractStreamedToolInputJSON(t, out.String(), "toolu_oversized_snapshot_recovery"))
}

func TestStreamEventStreamAsAnthropicRepairsControlCharsInToolInput(t *testing.T) {
	stream := bytes.NewBuffer(nil)
	// 上游把带真实换行的大段参数逐帧透传(plan 模式 ExitPlanMode.plan 常见)。
	_, _ = stream.Write(buildEventStreamFrame(t, "toolUseEvent", map[string]any{
		"toolUseEvent": map[string]any{
			"toolUseId": "toolu_plan",
			"name":      "ExitPlanMode",
			"input":     "{\"plan\":\"step 1\nstep 2\"}",
		},
	}))
	_, _ = stream.Write(buildEventStreamFrame(t, "toolUseEvent", map[string]any{
		"toolUseEvent": map[string]any{
			"toolUseId": "toolu_plan",
			"name":      "ExitPlanMode",
			"stop":      true,
		},
	}))

	var out bytes.Buffer
	_, err := StreamEventStreamAsAnthropicWithContext(context.Background(), stream, &out, "claude-sonnet-4-5", 9, KiroRequestContext{})
	require.NoError(t, err)

	partial := extractStreamedToolInputJSON(t, out.String(), "toolu_plan")
	var decoded map[string]any
	require.NoError(t, json.Unmarshal([]byte(partial), &decoded))
	require.Equal(t, "step 1\nstep 2", decoded["plan"])
}

func TestStreamEventStreamAsAnthropicPreservesCommaClosersInsideToolInputString(t *testing.T) {
	stream := bytes.NewBuffer(nil)
	plan := "run printf ',}' and ',]'\nthen verify"
	_, _ = stream.Write(buildEventStreamFrame(t, "toolUseEvent", map[string]any{
		"toolUseEvent": map[string]any{
			"toolUseId": "toolu_plan_commas",
			"name":      "ExitPlanMode",
			"input":     "{\"plan\":\"" + plan + "\"}",
			"stop":      true,
		},
	}))

	var out bytes.Buffer
	_, err := StreamEventStreamAsAnthropicWithContext(context.Background(), stream, &out, "claude-sonnet-4-5", 9, KiroRequestContext{})
	require.NoError(t, err)

	partial := extractStreamedToolInputJSON(t, out.String(), "toolu_plan_commas")
	var decoded map[string]any
	require.NoError(t, json.Unmarshal([]byte(partial), &decoded))
	require.Equal(t, plan, decoded["plan"])
}

func extractSSEEventData(t *testing.T, sse, eventType string) []byte {
	t.Helper()
	for _, block := range strings.Split(sse, "\n\n") {
		lines := strings.Split(block, "\n")
		if len(lines) == 0 || lines[0] != "event: "+eventType {
			continue
		}
		for _, line := range lines[1:] {
			if strings.HasPrefix(line, "data: ") {
				return []byte(strings.TrimPrefix(line, "data: "))
			}
		}
	}
	require.FailNow(t, "SSE event not found", "event type: %s", eventType)
	return nil
}

func extractStreamedToolInputJSON(t *testing.T, sse, toolUseID string) string {
	t.Helper()
	var sb strings.Builder
	targetIndex := -1
	for _, block := range strings.Split(sse, "\n\n") {
		line := ""
		for _, l := range strings.Split(block, "\n") {
			if strings.HasPrefix(l, "data: ") {
				line = strings.TrimPrefix(l, "data: ")
			}
		}
		if line == "" {
			continue
		}
		var evt map[string]any
		if err := json.Unmarshal([]byte(line), &evt); err != nil {
			continue
		}
		switch evt["type"] {
		case "content_block_start":
			if cb, _ := evt["content_block"].(map[string]any); cb != nil && cb["id"] == toolUseID {
				if idx, ok := evt["index"].(float64); ok {
					targetIndex = int(idx)
				}
			}
		case "content_block_delta":
			if idx, ok := evt["index"].(float64); ok && int(idx) == targetIndex {
				if delta, _ := evt["delta"].(map[string]any); delta != nil {
					if pj, ok := delta["partial_json"].(string); ok {
						_, _ = sb.WriteString(pj)
					}
				}
			}
		}
	}
	require.NotEqual(t, -1, targetIndex)
	return sb.String()
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

func TestAssistantToolSnapshotRequiresIDAndName(t *testing.T) {
	tests := []struct {
		name string
		tool map[string]any
	}{
		{
			name: "missing id",
			tool: map[string]any{"name": "custom_tool", "input": map[string]any{"value": true}},
		},
		{
			name: "missing name",
			tool: map[string]any{"toolUseId": "toolu_missing_name", "input": map[string]any{"value": true}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name+" non-streaming", func(t *testing.T) {
			stream := bytes.NewBuffer(buildEventStreamFrame(t, "assistantResponseEvent", map[string]any{
				"assistantResponseEvent": map[string]any{"toolUses": []map[string]any{tt.tool}},
			}))

			result, err := ParseNonStreamingEventStreamWithContext(stream, "claude-sonnet-4-5", KiroRequestContext{})
			require.NoError(t, err)
			require.Equal(t, "end_turn", result.StopReason)
			require.NotContains(t, string(result.ResponseBody), `"type":"tool_use"`)
		})

		t.Run(tt.name+" streaming", func(t *testing.T) {
			stream := bytes.NewBuffer(buildEventStreamFrame(t, "assistantResponseEvent", map[string]any{
				"assistantResponseEvent": map[string]any{"toolUses": []map[string]any{tt.tool}},
			}))

			var out bytes.Buffer
			result, err := StreamEventStreamAsAnthropicWithContext(context.Background(), stream, &out, "claude-sonnet-4-5", 9, KiroRequestContext{})
			require.NoError(t, err)
			require.Equal(t, "end_turn", result.StopReason)
			require.NotContains(t, out.String(), `"type":"tool_use"`)
		})
	}
}

func TestStreamEventStreamAsAnthropicInvalidAssistantSnapshotDoesNotDiscardBufferedSameID(t *testing.T) {
	stream := bytes.NewBuffer(nil)
	_, _ = stream.Write(buildEventStreamFrame(t, "toolUseEvent", map[string]any{
		"toolUseEvent": map[string]any{
			"toolUseId": "toolu_invalid_snapshot_same_id",
			"name":      "custom_tool",
			"input":     `{"value":"stream"}`,
		},
	}))
	_, _ = stream.Write(buildEventStreamFrame(t, "assistantResponseEvent", map[string]any{
		"assistantResponseEvent": map[string]any{
			"toolUses": []map[string]any{
				{
					"toolUseId": "toolu_invalid_snapshot_same_id",
					"input":     map[string]any{"value": "invalid-snapshot"},
				},
			},
		},
	}))
	_, _ = stream.Write(buildEventStreamFrame(t, "toolUseEvent", map[string]any{
		"toolUseEvent": map[string]any{
			"toolUseId": "toolu_invalid_snapshot_same_id",
			"stop":      true,
		},
	}))

	var out bytes.Buffer
	result, err := StreamEventStreamAsAnthropicWithContext(context.Background(), stream, &out, "claude-sonnet-4-5", 9, KiroRequestContext{})
	require.NoError(t, err)
	require.Equal(t, "tool_use", result.StopReason)
	require.Equal(t, 1, strings.Count(out.String(), `"id":"toolu_invalid_snapshot_same_id"`))
	require.JSONEq(t, `{"value":"stream"}`, extractStreamedToolInputJSON(t, out.String(), "toolu_invalid_snapshot_same_id"))
}

func TestStreamEventStreamAsAnthropicIgnoresStreamingDuplicateAfterAssistantToolSnapshot(t *testing.T) {
	stream := bytes.NewBuffer(nil)
	_, _ = stream.Write(buildEventStreamFrame(t, "assistantResponseEvent", map[string]any{
		"assistantResponseEvent": map[string]any{
			"toolUses": []map[string]any{
				{
					"toolUseId": "toolu_same_id_snapshot_first",
					"name":      "custom_tool",
					"input":     map[string]any{"value": "snapshot"},
				},
			},
		},
	}))
	_, _ = stream.Write(buildEventStreamFrame(t, "toolUseEvent", map[string]any{
		"toolUseEvent": map[string]any{
			"toolUseId": "toolu_same_id_snapshot_first",
			"name":      "custom_tool",
			"input":     map[string]any{"value": "stale-stream"},
			"stop":      true,
		},
	}))

	var out bytes.Buffer
	result, err := StreamEventStreamAsAnthropicWithContext(context.Background(), stream, &out, "claude-sonnet-4-5", 9, KiroRequestContext{})
	require.NoError(t, err)
	require.Equal(t, "tool_use", result.StopReason)
	require.Equal(t, 1, strings.Count(out.String(), `"id":"toolu_same_id_snapshot_first"`))
	require.JSONEq(t, `{"value":"snapshot"}`, extractStreamedToolInputJSON(t, out.String(), "toolu_same_id_snapshot_first"))
}

func TestStreamEventStreamAsAnthropicAssistantToolSnapshotReplacesBufferedSameID(t *testing.T) {
	stream := bytes.NewBuffer(nil)
	_, _ = stream.Write(buildEventStreamFrame(t, "toolUseEvent", map[string]any{
		"toolUseEvent": map[string]any{
			"toolUseId": "toolu_same_id_stream_first",
			"name":      "custom_tool",
			"input":     `{"value":"stale-stream"}`,
		},
	}))
	_, _ = stream.Write(buildEventStreamFrame(t, "assistantResponseEvent", map[string]any{
		"assistantResponseEvent": map[string]any{
			"toolUses": []map[string]any{
				{
					"toolUseId": "toolu_same_id_stream_first",
					"name":      "custom_tool",
					"input":     map[string]any{"value": "snapshot"},
				},
			},
		},
	}))

	var out bytes.Buffer
	result, err := StreamEventStreamAsAnthropicWithContext(context.Background(), stream, &out, "claude-sonnet-4-5", 9, KiroRequestContext{})
	require.NoError(t, err)
	require.Equal(t, "tool_use", result.StopReason)
	require.Equal(t, 1, strings.Count(out.String(), `"id":"toolu_same_id_stream_first"`))
	require.JSONEq(t, `{"value":"snapshot"}`, extractStreamedToolInputJSON(t, out.String(), "toolu_same_id_stream_first"))
}

func TestStreamEventStreamAsAnthropicAssistantToolSnapshotRecoversInvalidStoppedStream(t *testing.T) {
	stream := bytes.NewBuffer(nil)
	_, _ = stream.Write(buildEventStreamFrame(t, "toolUseEvent", map[string]any{
		"toolUseEvent": map[string]any{
			"toolUseId": "toolu_same_id_recovery",
			"name":      "custom_tool",
			"input":     `{"value":`,
			"stop":      true,
		},
	}))
	_, _ = stream.Write(buildEventStreamFrame(t, "assistantResponseEvent", map[string]any{
		"assistantResponseEvent": map[string]any{
			"toolUses": []map[string]any{
				{
					"toolUseId": "toolu_same_id_recovery",
					"name":      "custom_tool",
					"input":     map[string]any{"value": "snapshot"},
				},
			},
		},
	}))

	var out bytes.Buffer
	result, err := StreamEventStreamAsAnthropicWithContext(context.Background(), stream, &out, "claude-sonnet-4-5", 9, KiroRequestContext{})
	require.NoError(t, err)
	require.Equal(t, "tool_use", result.StopReason)
	require.Equal(t, 1, strings.Count(out.String(), `"id":"toolu_same_id_recovery"`))
	require.JSONEq(t, `{"value":"snapshot"}`, extractStreamedToolInputJSON(t, out.String(), "toolu_same_id_recovery"))
}

func TestStreamEventStreamAsAnthropicDeduplicatesRestoredToolNamesByContent(t *testing.T) {
	longName := strings.Repeat("long_tool_name_", 6)
	shortName := shortenToolNameIfNeeded(longName)
	stream := bytes.NewBuffer(nil)
	_, _ = stream.Write(buildEventStreamFrame(t, "assistantResponseEvent", map[string]any{
		"assistantResponseEvent": map[string]any{
			"toolUses": []map[string]any{
				{
					"toolUseId": "toolu_short_aggregate",
					"name":      shortName,
					"input":     map[string]any{"value": "same"},
				},
			},
		},
	}))
	_, _ = stream.Write(buildEventStreamFrame(t, "toolUseEvent", map[string]any{
		"toolUseEvent": map[string]any{
			"toolUseId": "toolu_short_stream",
			"name":      shortName,
			"input":     map[string]any{"value": "same"},
			"stop":      true,
		},
	}))

	var out bytes.Buffer
	result, err := StreamEventStreamAsAnthropicWithContext(context.Background(), stream, &out, "claude-sonnet-4-5", 9, KiroRequestContext{
		ToolNameMap: map[string]string{shortName: longName},
	})
	require.NoError(t, err)
	require.Equal(t, "tool_use", result.StopReason)
	require.Equal(t, 1, strings.Count(out.String(), `"type":"tool_use"`))
	require.Equal(t, 1, strings.Count(out.String(), `"name":"`+longName+`"`))
}

func TestStreamEventStreamAsAnthropicNormalizesWebSearchToolName(t *testing.T) {
	stream := bytes.NewBuffer(nil)
	_, _ = stream.Write(buildEventStreamFrame(t, "toolUseEvent", map[string]any{
		"toolUseEvent": map[string]any{
			"toolUseId": "toolu_web_search_name",
			"name":      "web_search",
			"input":     `{"query":"golang"}`,
			"stop":      true,
		},
	}))

	var out bytes.Buffer
	result, err := StreamEventStreamAsAnthropicWithContext(context.Background(), stream, &out, "claude-sonnet-4-5", 9, KiroRequestContext{})
	require.NoError(t, err)
	require.Equal(t, "tool_use", result.StopReason)
	require.Contains(t, out.String(), `"name":"remote_web_search"`)
	require.NotContains(t, out.String(), `"name":"web_search"`)
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

func TestNormalizeStreamingToolInput(t *testing.T) {
	tests := []struct {
		name     string
		toolName string
		raw      string
		want     map[string]any
		wantOK   bool
	}{
		{
			name:     "repairs literal control characters and trailing comma",
			toolName: "ExitPlanMode",
			raw:      "{\"plan\":\"line one\nline two\t\x00\",}",
			want:     map[string]any{"plan": "line one\nline two\t\x00"},
			wantOK:   true,
		},
		{
			name:     "preserves comma closers inside strings",
			toolName: "ExitPlanMode",
			raw:      "{\"plan\":\"keep ,} and ,]\nnext\",}",
			want:     map[string]any{"plan": "keep ,} and ,]\nnext"},
			wantOK:   true,
		},
		{
			name:     "preserves backslash before literal newline",
			toolName: "ExitPlanMode",
			raw:      "{\"plan\":\"echo \\\nnext\"}",
			want:     map[string]any{"plan": "echo \\\nnext"},
			wantOK:   true,
		},
		{
			name:     "preserves large integer",
			toolName: "custom_tool",
			raw:      `{"id":9007199254740993}`,
			want:     map[string]any{"id": json.Number("9007199254740993")},
			wantOK:   true,
		},
		{
			name:     "accepts empty object for unknown tool",
			toolName: "custom_tool",
			raw:      `{}`,
			want:     map[string]any{},
			wantOK:   true,
		},
		{
			name:     "accepts OpenCode camelCase write path",
			toolName: "write",
			raw:      `{"filePath":"/tmp/hello","content":"hello"}`,
			want:     map[string]any{"filePath": "/tmp/hello", "content": "hello"},
			wantOK:   true,
		},
		{
			name:     "accepts snake case write path",
			toolName: "write",
			raw:      `{"file_path":"/tmp/hello","content":"hello"}`,
			want:     map[string]any{"file_path": "/tmp/hello", "content": "hello"},
			wantOK:   true,
		},
		{
			name:     "accepts legacy write path",
			toolName: "write",
			raw:      `{"path":"/tmp/hello","content":"hello"}`,
			want:     map[string]any{"path": "/tmp/hello", "content": "hello"},
			wantOK:   true,
		},
		{
			name:     "rejects write missing path",
			toolName: "write",
			raw:      `{"content":"hello"}`,
			wantOK:   false,
		},
		{
			name:     "rejects write missing content",
			toolName: "write",
			raw:      `{"filePath":"/tmp/hello"}`,
			wantOK:   false,
		},
		{
			name:     "rejects synthetically completable truncation",
			toolName: "write_to_file",
			raw:      `{"path":"main.go","content":"package main`,
			wantOK:   false,
		},
		{
			name:     "rejects missing required field",
			toolName: "write_to_file",
			raw:      `{"path":"main.go"}`,
			wantOK:   false,
		},
		{name: "rejects array", toolName: "custom_tool", raw: `[]`, wantOK: false},
		{name: "rejects scalar", toolName: "custom_tool", raw: `"value"`, wantOK: false},
		{name: "rejects null", toolName: "custom_tool", raw: `null`, wantOK: false},
		// 空/空白输入归一化为 {}，与无参工具调用语义一致；有必填参数的工具则拒绝空输入
		{name: "accepts empty input for tool without requirements", toolName: "custom_tool", raw: ` `, want: map[string]any{}, wantOK: true},
		{name: "rejects empty input for tool with requirements", toolName: "write", raw: ` `, wantOK: false},
		{name: "rejects malformed syntax", toolName: "custom_tool", raw: `{"x":}`, wantOK: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			normalized, input, ok := normalizeStreamingToolInput(tt.toolName, tt.raw)
			require.Equal(t, tt.wantOK, ok)
			if !tt.wantOK {
				require.Empty(t, normalized)
				require.Nil(t, input)
				return
			}
			require.Equal(t, tt.want, input)
			var decoded map[string]any
			decoder := json.NewDecoder(strings.NewReader(normalized))
			decoder.UseNumber()
			require.NoError(t, decoder.Decode(&decoded))
			require.Equal(t, tt.want, decoded)
		})
	}
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
		"claude-opus-5":                       "claude-opus-5",
		"claude-opus-5-thinking":              "claude-opus-5",
		"claude-sonnet-4-6":                   "claude-sonnet-4.6",
		"claude-sonnet-4-6-thinking":          "claude-sonnet-4.6",
		"claude-sonnet-4.6":                   "claude-sonnet-4.6",
		"claude-sonnet-5":                     "claude-sonnet-5",
		"claude-sonnet-5-thinking":            "claude-sonnet-5",
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
		"gpt-5.6-sol":                         "gpt-5.6-sol",
		"gpt-5.6-terra":                       "gpt-5.6-terra",
		"gpt-5.6-luna":                        "gpt-5.6-luna",
	}

	for input, want := range cases {
		if got := MapModel(input); got != want {
			t.Fatalf("MapModel(%q) = %q, want %q", input, got, want)
		}
	}

	rejected := []string{
		"gpt-5.6",
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

func TestKiroMaxOutputTokensForGPT56Models(t *testing.T) {
	t.Parallel()

	for _, model := range []string{"gpt-5.6-sol", "gpt-5.6-terra", "gpt-5.6-luna"} {
		require.Equal(t, 128000, kiroMaxOutputTokensForModel(model), model)
	}
	require.Equal(t, kiroDefaultMaxOutputTokens, kiroMaxOutputTokensForModel("gpt-5.6"))
}

func TestKiroMaxOutputTokensForOpus5(t *testing.T) {
	t.Parallel()

	require.Equal(t, 128000, kiroMaxOutputTokensForModel("claude-opus-5"))
	require.Equal(t, 128000, kiroMaxOutputTokensForModel("claude-opus-5-thinking"))
}

func TestIsOutputConfigPathModelSupportsFutureVersions(t *testing.T) {
	t.Parallel()

	cases := map[string]bool{
		"claude-opus-4.6":            true,
		"claude-opus-4-9-thinking":   true,
		"claude-sonnet-5-0-thinking": true,
		"claude-opus-5":              true,
		"claude-opus-5-thinking":     true,
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
	credits, ok := usageMap["_sub2api_kiro_credits"].(float64)
	require.True(t, ok)
	require.InDelta(t, 0.17, credits, 0.000001)
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

func TestStreamEventStreamAsAnthropicStopsAfterToolBlockWriteError(t *testing.T) {
	stream := bytes.NewBuffer(buildEventStreamFrame(t, "toolUseEvent", map[string]any{
		"toolUseEvent": map[string]any{
			"toolUseId": "toolu_write_error",
			"name":      "bash",
			"input":     `{"command":"echo hello"}`,
			"stop":      true,
		},
	}))
	writeErr := errors.New("forced write failure")
	writer := &failAfterWritesWriter{failAt: 3, err: writeErr}

	result, err := StreamEventStreamAsAnthropicWithContext(context.Background(), stream, writer, "claude-sonnet-4-5", 10, KiroRequestContext{})
	require.Nil(t, result)
	require.ErrorIs(t, err, writeErr)
	require.Contains(t, writer.String(), `event: content_block_start`)
	require.NotContains(t, writer.String(), `event: content_block_stop`)
	require.NotContains(t, writer.String(), `event: message_delta`)
	require.NotContains(t, writer.String(), `event: message_stop`)
}

type failAfterWritesWriter struct {
	buffer bytes.Buffer
	writes int
	failAt int
	err    error
}

func (w *failAfterWritesWriter) Write(p []byte) (int, error) {
	w.writes++
	if w.writes == w.failAt {
		return 0, w.err
	}
	return w.buffer.Write(p)
}

func (w *failAfterWritesWriter) String() string {
	return w.buffer.String()
}

func buildEventStreamFrame(t *testing.T, eventType string, payload any) []byte {
	t.Helper()
	payloadBytes, err := json.Marshal(payload)
	require.NoError(t, err)
	return buildRawEventStreamFrame(t, eventType, payloadBytes)
}

func buildRawEventStreamFrame(t *testing.T, eventType string, payloadBytes []byte) []byte {
	t.Helper()

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
