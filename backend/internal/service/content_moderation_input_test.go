package service

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

// 当数组末尾不是用户消息时（典型场景：Agent 工具循环结束于 tool/assistant），
// 应直接跳过审计——不再回溯查找历史中的某条用户消息。

func TestExtractContentModerationInput_AnthropicAgentToolLoopSkipsAudit(t *testing.T) {
	body := []byte(`{
		"messages": [
			{"role":"user","content":"调用一下天气工具"},
			{"role":"assistant","content":[{"type":"tool_use","id":"tool_1","name":"weather","input":{}}]},
			{"role":"user","content":[{"type":"tool_result","tool_use_id":"tool_1","content":"晴 25 度"}]}
		]
	}`)

	input := ExtractContentModerationInput(ContentModerationProtocolAnthropicMessages, body)

	require.Empty(t, input.Text)
	require.Empty(t, input.Images)
}

func TestExtractContentModerationInput_AnthropicFirstTurnExtractsUser(t *testing.T) {
	body := []byte(`{
		"messages": [
			{"role":"user","content":"Q1"}
		]
	}`)

	input := ExtractContentModerationInput(ContentModerationProtocolAnthropicMessages, body)

	require.Equal(t, "Q1", input.Text)
}

func TestExtractContentModerationInput_AnthropicMultiTurnExtractsLatestUser(t *testing.T) {
	body := []byte(`{
		"messages": [
			{"role":"user","content":"Q1"},
			{"role":"assistant","content":"A1"},
			{"role":"user","content":"Q2"}
		]
	}`)

	input := ExtractContentModerationInput(ContentModerationProtocolAnthropicMessages, body)

	require.Equal(t, "Q2", input.Text)
}

func TestExtractContentModerationInput_AnthropicStreamResendExtractsResend(t *testing.T) {
	body := []byte(`{
		"messages": [
			{"role":"user","content":"原问题"},
			{"role":"assistant","content":"部分回答……"},
			{"role":"user","content":"重发"}
		]
	}`)

	input := ExtractContentModerationInput(ContentModerationProtocolAnthropicMessages, body)

	require.Equal(t, "重发", input.Text)
}

func TestExtractContentModerationInput_OpenAIChatAgentToolLoopSkipsAudit(t *testing.T) {
	body := []byte(`{
		"messages": [
			{"role":"system","content":"sys"},
			{"role":"user","content":"列出我的订单"},
			{"role":"assistant","content":null,"tool_calls":[{"id":"call_1","type":"function","function":{"name":"orders","arguments":"{}"}}]},
			{"role":"tool","tool_call_id":"call_1","content":"[]"}
		]
	}`)

	input := ExtractContentModerationInput(ContentModerationProtocolOpenAIChat, body)

	require.Empty(t, input.Text)
	require.Empty(t, input.Images)
}

func TestExtractContentModerationInput_OpenAIChatMultiTurnExtractsLatestUser(t *testing.T) {
	body := []byte(`{
		"messages": [
			{"role":"user","content":"Q1"},
			{"role":"assistant","content":"A1"},
			{"role":"user","content":"Q2"}
		]
	}`)

	input := ExtractContentModerationInput(ContentModerationProtocolOpenAIChat, body)

	require.Equal(t, "Q2", input.Text)
}

func TestExtractContentModerationInput_GeminiAgentToolLoopSkipsAudit(t *testing.T) {
	body := []byte(`{
		"contents": [
			{"role":"user","parts":[{"text":"查询天气"}]},
			{"role":"model","parts":[{"functionCall":{"name":"weather","args":{}}}]},
			{"role":"user","parts":[{"functionResponse":{"name":"weather","response":{"temp":25}}}]}
		]
	}`)

	input := ExtractContentModerationInput(ContentModerationProtocolGemini, body)

	require.Empty(t, input.Text)
	require.Empty(t, input.Images)
}

func TestExtractContentModerationInput_GeminiFirstTurnExtractsUser(t *testing.T) {
	body := []byte(`{
		"contents": [
			{"role":"user","parts":[{"text":"你好"}]}
		]
	}`)

	input := ExtractContentModerationInput(ContentModerationProtocolGemini, body)

	require.Equal(t, "你好", input.Text)
}

func TestExtractContentModerationInput_GeminiMultiTurnExtractsLatestUser(t *testing.T) {
	body := []byte(`{
		"contents": [
			{"role":"user","parts":[{"text":"Q1"}]},
			{"role":"model","parts":[{"text":"A1"}]},
			{"role":"user","parts":[{"text":"Q2"}]}
		]
	}`)

	input := ExtractContentModerationInput(ContentModerationProtocolGemini, body)

	require.Equal(t, "Q2", input.Text)
}

func TestExtractContentModerationInput_ResponsesAgentToolLoopSkipsAudit(t *testing.T) {
	body := []byte(`{
		"input":[
			{"type":"message","role":"user","content":[{"type":"input_text","text":"运行测试"}]},
			{"type":"function_call","call_id":"call_1","name":"run_tests","arguments":"{}"},
			{"type":"function_call_output","call_id":"call_1","output":"all passed"}
		]
	}`)

	input := ExtractContentModerationInput(ContentModerationProtocolOpenAIResponses, body)

	require.Empty(t, input.Text)
	require.Empty(t, input.Images)
}

func TestExtractContentModerationInput_ResponsesLastUserMessageExtracted(t *testing.T) {
	body := []byte(`{
		"input":[
			{"type":"message","role":"user","content":[{"type":"input_text","text":"first"}]},
			{"type":"message","role":"assistant","content":[{"type":"output_text","text":"answer"}]},
			{"type":"message","role":"user","content":[{"type":"input_text","text":"latest"}]}
		]
	}`)

	input := ExtractContentModerationInput(ContentModerationProtocolOpenAIResponses, body)

	require.Equal(t, "latest", input.Text)
}

func TestExtractContentModerationInput_ResponsesLastIsAssistantSkipped(t *testing.T) {
	body := []byte(`{
		"input":[
			{"type":"message","role":"user","content":[{"type":"input_text","text":"q1"}]},
			{"type":"message","role":"assistant","content":[{"type":"output_text","text":"a1"}]}
		]
	}`)

	input := ExtractContentModerationInput(ContentModerationProtocolOpenAIResponses, body)

	require.Empty(t, input.Text)
	require.Empty(t, input.Images)
}

func TestExtractContentModerationInput_ResponsesCollectsCurrentUserSuffix(t *testing.T) {
	body := []byte(`{
		"input":[
			{"type":"message","role":"developer","content":[{"type":"input_text","text":"policy"}]},
			{"type":"message","role":"user","content":[
				{"type":"input_text","text":"first segment"},
				{"type":"input_text","text":"second segment"}
			]},
			{"type":"input_text","text":"final segment"}
		]
	}`)

	input := ExtractContentModerationInput(ContentModerationProtocolOpenAIResponses, body)

	require.Equal(t, "first segment second segment final segment", input.Text)
	require.Empty(t, input.Images)
	require.Equal(t, contentModerationInputClassText, input.Classification())
	require.False(t, input.MustFailClosed())
}

func TestExtractContentModerationInput_ResponsesCollectsTextAndImages(t *testing.T) {
	body := []byte(`{
		"input":[
			{"type":"message","role":"user","content":[
				{"type":"input_text","text":"check both"},
				{"type":"input_image","image_url":"https://example.com/one.png"}
			]},
			{"type":"input_image","image_url":"data:image/png;base64,dHdv"}
		]
	}`)

	input := ExtractContentModerationInput(ContentModerationProtocolOpenAIResponses, body)

	require.Equal(t, "check both", input.Text)
	require.Equal(t, []string{"https://example.com/one.png", "data:image/png;base64,dHdv"}, input.Images)
	require.Equal(t, contentModerationInputClassTextImage, input.Classification())
}

func TestExtractContentModerationInput_ResponsesCollectsImageOnly(t *testing.T) {
	body := []byte(`{
		"input":[
			{"type":"message","role":"user","content":[
				{"type":"input_image","image_url":"https://example.com/only.png"}
			]}
		]
	}`)

	input := ExtractContentModerationInput(ContentModerationProtocolOpenAIResponses, body)

	require.Empty(t, input.Text)
	require.Equal(t, []string{"https://example.com/only.png"}, input.Images)
	require.Equal(t, contentModerationInputClassImage, input.Classification())
	require.False(t, input.MustFailClosed())
}

func TestExtractContentModerationInput_ResponsesClassifiesToolContinuation(t *testing.T) {
	body := []byte(`{
		"input":[
			{"type":"message","role":"user","content":[{"type":"input_text","text":"run tests"}]},
			{"type":"function_call","call_id":"call_1","name":"run_tests","arguments":"{}"},
			{"type":"function_call_output","call_id":"call_1","output":"all passed"}
		]
	}`)

	input := ExtractContentModerationInput(ContentModerationProtocolOpenAIResponses, body)

	require.True(t, input.IsEmpty())
	require.Equal(t, contentModerationInputClassToolContinuation, input.Classification())
	require.Equal(t, responsesItemKindToolOutput, input.tailItemKind)
	require.False(t, input.MustFailClosed())
}

func TestExtractContentModerationInput_ResponsesLongHistoryEndingInToolOutputSkipsHistoricalUser(t *testing.T) {
	body := []byte(`{
		"input":[
			{"type":"message","role":"user","content":[{"type":"input_text","text":"historical prompt"}]},
			{"type":"reasoning","encrypted_content":"opaque"},
			{"type":"message","role":"assistant","content":[{"type":"output_text","text":"working"}]},
			{"type":"function_call","call_id":"call_9","name":"shell","arguments":"{}"},
			{"type":"function_call_output","call_id":"call_9","output":"very long tool output"}
		]
	}`)

	input := ExtractContentModerationInput(ContentModerationProtocolOpenAIResponses, body)

	require.True(t, input.IsEmpty())
	require.NotContains(t, input.Text, "historical prompt")
	require.Equal(t, contentModerationInputClassToolContinuation, input.Classification())
	require.False(t, input.MustFailClosed())
}

func TestExtractContentModerationInput_ResponsesTrulyEmptyInput(t *testing.T) {
	tests := []struct {
		name string
		body string
	}{
		{name: "missing input", body: `{}`},
		{name: "empty array", body: `{"input":[]}`},
		{name: "empty user content", body: `{"input":[{"type":"message","role":"user","content":[]}]}`},
		{name: "blank string", body: `{"input":"   "}`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			input := ExtractContentModerationInput(ContentModerationProtocolOpenAIResponses, []byte(tt.body))

			require.True(t, input.IsEmpty())
			require.Equal(t, contentModerationInputClassEmpty, input.Classification())
			require.False(t, input.MustFailClosed())
		})
	}
}

func TestExtractContentModerationInput_ResponsesUnreviewableNonEmptyFailsClosed(t *testing.T) {
	tests := []struct {
		name string
		body string
	}{
		{name: "invalid json", body: `{"input":`},
		{name: "unsupported user file", body: `{"input":[{"type":"message","role":"user","content":[{"type":"input_file","file_id":"file_1"}]}]}`},
		{name: "unknown tail item", body: `{"input":[{"type":"unknown","payload":{"value":1}}]}`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			input := ExtractContentModerationInput(ContentModerationProtocolOpenAIResponses, []byte(tt.body))

			require.True(t, input.IsEmpty())
			require.True(t, input.MustFailClosed())
			require.Contains(t, []string{contentModerationInputClassInvalidJSON, contentModerationInputClassUnsupported}, input.Classification())
		})
	}
}

func TestContentModerationCheck_ResponsesUnreviewableNonEmptyFailsClosedWithoutAuditCall(t *testing.T) {
	cfg := defaultContentModerationConfig()
	cfg.Enabled = true
	cfg.Mode = ContentModerationModePreBlock
	cfg.APIKeys = []string{"sk-test"}
	rawCfg, err := json.Marshal(cfg)
	require.NoError(t, err)

	svc := NewContentModerationService(
		&contentModerationTestSettingRepo{values: map[string]string{
			SettingKeyRiskControlEnabled:      "true",
			SettingKeyContentModerationConfig: string(rawCfg),
		}},
		&contentModerationTestRepo{},
		&contentModerationTestHashCache{},
		nil,
		nil,
		nil,
		nil,
		nil,
	)

	decision, err := svc.Check(context.Background(), ContentModerationCheckInput{
		RequestID: "req-fail-closed",
		Endpoint:  "/responses",
		Protocol:  ContentModerationProtocolOpenAIResponses,
		Body:      []byte(`{"input":[{"type":"message","role":"user","content":[{"type":"input_file","file_id":"file_1"}]}]}`),
	})

	require.NoError(t, err)
	require.False(t, decision.Allowed)
	require.True(t, decision.Blocked)
	require.Equal(t, ContentModerationActionError, decision.Action)
	require.Equal(t, cfg.BlockStatus, decision.StatusCode)
}
