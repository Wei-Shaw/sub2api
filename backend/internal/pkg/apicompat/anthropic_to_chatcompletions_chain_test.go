package apicompat

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestAnthropicToCCChain_BasicText 验证 Anthropic→Responses→CC 的请求侧链式转换：
// 一段简单的 user 文本消息能完整保留到 ChatCompletionsRequest.messages。
func TestAnthropicToCCChain_BasicText(t *testing.T) {
	req := &AnthropicRequest{
		Model:     "deepseek-chat",
		MaxTokens: 256,
		Messages: []AnthropicMessage{
			{Role: "user", Content: json.RawMessage(`"Hello"`)},
		},
	}

	respReq, err := AnthropicToResponses(req)
	require.NoError(t, err)

	ccReq, err := ResponsesToChatCompletionsRequest(respReq)
	require.NoError(t, err)
	assert.Equal(t, "deepseek-chat", ccReq.Model)
	require.GreaterOrEqual(t, len(ccReq.Messages), 1)
	last := ccReq.Messages[len(ccReq.Messages)-1]
	assert.Equal(t, "user", last.Role)
	assert.Contains(t, string(last.Content), "Hello")
}

// TestAnthropicToCCChain_SystemBlock 验证 Anthropic system 数组形态会落地为
// CC 的 system message。
func TestAnthropicToCCChain_SystemBlock(t *testing.T) {
	req := &AnthropicRequest{
		Model:     "deepseek-chat",
		MaxTokens: 64,
		System:    json.RawMessage(`[{"type":"text","text":"You are helpful."}]`),
		Messages: []AnthropicMessage{
			{Role: "user", Content: json.RawMessage(`"Hi"`)},
		},
	}
	respReq, err := AnthropicToResponses(req)
	require.NoError(t, err)
	ccReq, err := ResponsesToChatCompletionsRequest(respReq)
	require.NoError(t, err)
	require.GreaterOrEqual(t, len(ccReq.Messages), 2)
	first := ccReq.Messages[0]
	assert.Equal(t, "system", first.Role)
	assert.Contains(t, string(first.Content), "You are helpful")
}

// TestAnthropicToCCChain_ToolUseHistory 验证 assistant 的 tool_use 与后续 user 的
// tool_result 都能在链路中正确转换为 CC 的 assistant.tool_calls + tool message。
func TestAnthropicToCCChain_ToolUseHistory(t *testing.T) {
	req := &AnthropicRequest{
		Model:     "deepseek-chat",
		MaxTokens: 64,
		Tools: []AnthropicTool{{
			Name:        "lookup",
			Description: "fake",
			InputSchema: json.RawMessage(`{"type":"object","properties":{"q":{"type":"string"}}}`),
		}},
		Messages: []AnthropicMessage{
			{Role: "user", Content: json.RawMessage(`"search docs"`)},
			{Role: "assistant", Content: json.RawMessage(`[
				{"type":"tool_use","id":"toolu_1","name":"lookup","input":{"q":"foo"}}
			]`)},
			{Role: "user", Content: json.RawMessage(`[
				{"type":"tool_result","tool_use_id":"toolu_1","content":"answer"}
			]`)},
		},
	}
	respReq, err := AnthropicToResponses(req)
	require.NoError(t, err)
	ccReq, err := ResponsesToChatCompletionsRequest(respReq)
	require.NoError(t, err)
	require.GreaterOrEqual(t, len(ccReq.Messages), 3)

	var sawToolCall, sawToolMsg bool
	for _, m := range ccReq.Messages {
		if m.Role == "assistant" && len(m.ToolCalls) > 0 {
			sawToolCall = true
			assert.Equal(t, "lookup", m.ToolCalls[0].Function.Name)
		}
		if m.Role == "tool" && m.ToolCallID != "" {
			sawToolMsg = true
			assert.Contains(t, string(m.Content), "answer")
		}
	}
	assert.True(t, sawToolCall, "expected an assistant tool_calls message")
	assert.True(t, sawToolMsg, "expected a tool role message carrying the tool_result")
}

// TestAnthropicToCCChain_ImageBlock 验证 image 块经 Anthropic→Responses→CC 后落到
// CC 的 image_url.content 形态（base64 dataURI）。
func TestAnthropicToCCChain_ImageBlock(t *testing.T) {
	req := &AnthropicRequest{
		Model:     "deepseek-chat",
		MaxTokens: 64,
		Messages: []AnthropicMessage{
			{Role: "user", Content: json.RawMessage(`[
				{"type":"text","text":"What is in this image?"},
				{"type":"image","source":{"type":"base64","media_type":"image/png","data":"AAA"}}
			]`)},
		},
	}
	respReq, err := AnthropicToResponses(req)
	require.NoError(t, err)
	ccReq, err := ResponsesToChatCompletionsRequest(respReq)
	require.NoError(t, err)
	require.GreaterOrEqual(t, len(ccReq.Messages), 1)
	bodyJSON, _ := json.Marshal(ccReq.Messages[len(ccReq.Messages)-1])
	assert.Contains(t, string(bodyJSON), "image_url")
	assert.Contains(t, string(bodyJSON), "data:image/png;base64,AAA")
}

// TestAnthropicToCCChain_MultiTurn 多轮对话历史的角色顺序应当被保持。
func TestAnthropicToCCChain_MultiTurn(t *testing.T) {
	req := &AnthropicRequest{
		Model:     "deepseek-chat",
		MaxTokens: 64,
		Messages: []AnthropicMessage{
			{Role: "user", Content: json.RawMessage(`"q1"`)},
			{Role: "assistant", Content: json.RawMessage(`"a1"`)},
			{Role: "user", Content: json.RawMessage(`"q2"`)},
		},
	}
	respReq, err := AnthropicToResponses(req)
	require.NoError(t, err)
	ccReq, err := ResponsesToChatCompletionsRequest(respReq)
	require.NoError(t, err)
	require.GreaterOrEqual(t, len(ccReq.Messages), 3)
	roles := []string{}
	for _, m := range ccReq.Messages {
		if m.Role == "system" {
			continue
		}
		roles = append(roles, m.Role)
	}
	assert.Equal(t, []string{"user", "assistant", "user"}, roles)
}
