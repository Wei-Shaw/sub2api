package service

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestContentModerationInputMemoCaptureLifecycle(t *testing.T) {
	body := []byte(`{"messages":[{"role":"user","content":"latest prompt"}]}`)
	memo := NewContentModerationInputMemo(ContentModerationProtocolOpenAIChat, body)

	resolved, excerpt := memo.Capture()
	require.False(t, resolved)
	require.Empty(t, excerpt)

	input := memo.Resolve()
	require.Equal(t, "latest prompt", input.Text)
	body[0] = '['

	resolved, excerpt = memo.Capture()
	require.True(t, resolved)
	require.Equal(t, "latest prompt", excerpt)
	require.Equal(t, input, memo.Resolve(), "Resolve must be idempotent after releasing the body reference")
}

func TestContentModerationInputMemoCapturesResolvedEmptyInput(t *testing.T) {
	memo := NewContentModerationInputMemo(ContentModerationProtocolOpenAIResponses, []byte(`{"input":[]}`))
	require.True(t, memo.Resolve().IsEmpty())

	resolved, excerpt := memo.Capture()
	require.True(t, resolved)
	require.Empty(t, excerpt)
}

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

func TestExtractLatestUserContentModerationInput_ResponsesAgentToolLoop(t *testing.T) {
	body := []byte(`{
		"input":[
			{"type":"message","role":"user","content":[{"type":"input_text","text":"run the tests"}]},
			{"type":"function_call","call_id":"call_1","name":"run_tests","arguments":"{}"},
			{"type":"function_call_output","call_id":"call_1","output":"all passed"}
		]
	}`)

	input := ExtractLatestUserContentModerationInput(ContentModerationProtocolOpenAIResponses, body)

	require.Equal(t, "run the tests", input.Text)
}

func TestExtractLatestUserContentModerationInput_ResponsesAssistantEnding(t *testing.T) {
	body := []byte(`{
		"input":[
			{"type":"message","role":"user","content":[{"type":"input_text","text":"latest user prompt"}]},
			{"type":"message","role":"assistant","content":[{"type":"output_text","text":"assistant answer"}]}
		]
	}`)

	input := ExtractLatestUserContentModerationInput(ContentModerationProtocolOpenAIResponses, body)

	require.Equal(t, "latest user prompt", input.Text)
}

func TestExtractLatestUserContentModerationInput_ResponsesFinalUserUnchanged(t *testing.T) {
	body := []byte(`{
		"input":[
			{"type":"message","role":"user","content":[{"type":"input_text","text":"first"}]},
			{"type":"message","role":"user","content":[{"type":"input_text","text":"latest"}]}
		]
	}`)

	input := ExtractLatestUserContentModerationInput(ContentModerationProtocolOpenAIResponses, body)

	require.Equal(t, "latest", input.Text)
}

func TestExtractLatestUserContentModerationInput_ResponsesWithoutUser(t *testing.T) {
	body := []byte(`{
		"input":[
			{"type":"message","role":"developer","content":[{"type":"input_text","text":"system instructions"}]},
			{"type":"function_call_output","call_id":"call_1","output":"all passed"},
			{"type":"message","role":"assistant","content":[{"type":"output_text","text":"assistant answer"}]}
		]
	}`)

	input := ExtractLatestUserContentModerationInput(ContentModerationProtocolOpenAIResponses, body)

	require.Empty(t, input.Text)
	require.Empty(t, input.Images)
}

func TestExtractLatestUserContentModerationInput_ResponsesExplicitlySkipsToolText(t *testing.T) {
	body := []byte(`{
		"input":[
			{"type":"message","role":"user","content":[{"type":"input_text","text":"actual user prompt"}]},
			{"type":"function_call_output","call_id":"call_1","content":[{"type":"input_text","text":"tool output must not be recorded"}]}
		]
	}`)

	input := ExtractLatestUserContentModerationInput(ContentModerationProtocolOpenAIResponses, body)

	require.Equal(t, "actual user prompt", input.Text)
}

func TestExtractLatestUserContentModerationInput_ResponsesToolObjectWithoutUser(t *testing.T) {
	body := []byte(`{
		"input":{"type":"function_call_output","call_id":"call_1","content":[{"type":"input_text","text":"tool output must not be recorded"}]}
	}`)

	input := ExtractLatestUserContentModerationInput(ContentModerationProtocolOpenAIResponses, body)

	require.Empty(t, input.Text)
	require.Empty(t, input.Images)
}

func TestExtractLatestUserContentModerationInput_ResponsesImageOnlyUserStopsSearch(t *testing.T) {
	body := []byte(`{
		"input":[
			{"type":"message","role":"user","content":[{"type":"input_text","text":"older user prompt"}]},
			{"type":"message","role":"user","content":[{"type":"input_image","image_url":"https://example.com/latest.png"}]}
		]
	}`)

	input := ExtractLatestUserContentModerationInput(ContentModerationProtocolOpenAIResponses, body)

	require.Empty(t, input.Text, "an image-only latest user item must not fall back to older text")
	require.Equal(t, []string{"https://example.com/latest.png"}, input.Images)
}

func TestExtractLatestUserContentModerationInput_OtherToolLoops(t *testing.T) {
	tests := []struct {
		name     string
		protocol string
		body     string
		expected string
	}{
		{
			name:     "openai chat",
			protocol: ContentModerationProtocolOpenAIChat,
			body: `{"messages":[
				{"role":"user","content":"list my orders"},
				{"role":"assistant","tool_calls":[{"id":"call_1","type":"function","function":{"name":"orders","arguments":"{}"}}]},
				{"role":"tool","tool_call_id":"call_1","content":"[]"}
			]}`,
			expected: "list my orders",
		},
		{
			name:     "anthropic messages",
			protocol: ContentModerationProtocolAnthropicMessages,
			body: `{"messages":[
				{"role":"user","content":"check the weather"},
				{"role":"assistant","content":[{"type":"tool_use","id":"tool_1","name":"weather","input":{}}]},
				{"role":"user","content":[{"type":"tool_result","tool_use_id":"tool_1","content":"sunny"}]}
			]}`,
			expected: "check the weather",
		},
		{
			name:     "gemini",
			protocol: ContentModerationProtocolGemini,
			body: `{"contents":[
				{"role":"user","parts":[{"text":"check the weather"}]},
				{"role":"model","parts":[{"functionCall":{"name":"weather","args":{}}}]},
				{"role":"user","parts":[{"functionResponse":{"name":"weather","response":{"temp":25}}}]}
			]}`,
			expected: "check the weather",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			input := ExtractLatestUserContentModerationInput(tt.protocol, []byte(tt.body))
			require.Equal(t, tt.expected, input.Text)
		})
	}
}
