package service

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/apicompat"
	"github.com/Wei-Shaw/sub2api/internal/pkg/cursor"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestBuildCursorAgentTurnsPairsToolResults(t *testing.T) {
	turns, systemPrompt, actionText := buildCursorAgentTurns([]apicompat.ChatMessage{
		{Role: "system", Content: json.RawMessage(`"be terse"`)},
		{Role: "user", Content: json.RawMessage(`"weather in Paris?"`)},
		{Role: "assistant", Content: json.RawMessage(`"checking"`), ToolCalls: []apicompat.ChatToolCall{{
			ID:       "call-1",
			Type:     "function",
			Function: apicompat.ChatFunctionCall{Name: "get_weather", Arguments: `{"city":"Paris"}`},
		}}},
		{Role: "tool", ToolCallID: "call-1", Content: json.RawMessage(`"sunny, 21C"`)},
	})

	require.Equal(t, "be terse", systemPrompt)
	require.Len(t, turns, 1)
	require.Equal(t, "weather in Paris?", turns[0].UserText)
	require.Len(t, turns[0].Steps, 2)
	require.Equal(t, "checking", turns[0].Steps[0].AssistantText)

	call := turns[0].Steps[1].ToolCall
	require.NotNil(t, call)
	require.Equal(t, "call-1", call.CallID)
	require.Equal(t, "get_weather", call.Name)
	require.JSONEq(t, `{"city":"Paris"}`, string(call.ArgsJSON))
	require.NotNil(t, call.Result)
	require.Equal(t, "sunny, 21C", call.Result.ContentText)
	require.False(t, call.Result.IsError)

	// History ends with a tool result, so the Run action is the continuation
	// prompt rather than a user message.
	require.Equal(t, continuationUserText, actionText)
}

func TestBuildCursorAgentTurnsTrailingUserIsAction(t *testing.T) {
	turns, _, actionText := buildCursorAgentTurns([]apicompat.ChatMessage{
		{Role: "user", Content: json.RawMessage(`"first"`)},
		{Role: "assistant", Content: json.RawMessage(`"answer"`)},
		{Role: "user", Content: json.RawMessage(`"and more?"`)},
	})

	require.Len(t, turns, 1)
	require.Equal(t, "first", turns[0].UserText)
	require.Equal(t, "answer", turns[0].Steps[0].AssistantText)
	require.Equal(t, "and more?", actionText)
}

func TestBuildCursorAgentRunRequestAgentMode(t *testing.T) {
	req := buildCursorAgentRunRequest("m", []apicompat.ChatMessage{
		{Role: "user", Content: json.RawMessage(`"hi"`)},
	}, []apicompat.ChatTool{{
		Type: "function",
		Function: &apicompat.ChatFunction{
			Name:        "get_weather",
			Description: "weather",
			Parameters:  json.RawMessage(`{"type":"object"}`),
		},
	}}, "")

	require.Len(t, req.Tools, 1)
	require.Equal(t, "get_weather", req.Tools[0].Name)
	// A single user message has no prior history: the message itself is the
	// Run action, so there is nothing to replay.
	require.Empty(t, req.Turns)
	require.Len(t, req.Messages, 1)
	require.Equal(t, "hi", req.Messages[0].Content)
}

func TestBuildCursorAgentRunRequestToolChoiceNoneStaysAsk(t *testing.T) {
	req := buildCursorAgentRunRequest("m", []apicompat.ChatMessage{
		{Role: "user", Content: json.RawMessage(`"hi"`)},
	}, []apicompat.ChatTool{{
		Type:     "function",
		Function: &apicompat.ChatFunction{Name: "get_weather"},
	}}, "none")

	require.Empty(t, req.Tools)
	require.Empty(t, req.Turns)
	// Ask mode flattens the messages instead of replaying turns.
	require.Len(t, req.Messages, 1)
}

// encodeCursorToolCallBody builds an upstream response body carrying one exec
// MCP tool call frame plus a turn_ended frame (agent.v1 wire shapes).
func encodeCursorToolCallBody(t *testing.T, callID, name, argsJSON string) []byte {
	t.Helper()

	var mcpArgs cursor.ProtobufWriter
	mcpArgs.String(1, name)   // McpArgs.name
	mcpArgs.String(3, callID) // McpArgs.tool_call_id
	// McpArgs.args: one map entry per top-level argument, value encoded as
	// google.protobuf.Value.
	var argObj map[string]json.RawMessage
	require.NoError(t, json.Unmarshal([]byte(argsJSON), &argObj))
	for key, value := range argObj {
		encodedValue, err := cursor.EncodeProtoJSONValue(value)
		require.NoError(t, err)
		var entry cursor.ProtobufWriter
		entry.String(1, key)
		entry.Bytes(2, encodedValue)
		mcpArgs.Bytes(2, entry.Result())
	}
	var exec cursor.ProtobufWriter
	exec.Varint(1, 3)                // ExecServerMessage.id
	exec.Bytes(11, mcpArgs.Result()) // ExecServerMessage.mcp_args
	var server cursor.ProtobufWriter
	server.Bytes(2, exec.Result()) // AgentServerMessage.exec_server_message
	frame, err := cursor.EncodeFrame(server.Result(), false)
	require.NoError(t, err)

	var ended cursor.ProtobufWriter
	ended.Varint(2, 5) // TurnEndedUpdate.output_tokens
	var endUpdate cursor.ProtobufWriter
	endUpdate.Bytes(14, ended.Result()) // InteractionUpdate.turn_ended
	var endServer cursor.ProtobufWriter
	endServer.Bytes(1, endUpdate.Result()) // AgentServerMessage.interaction_update
	endFrame, err := cursor.EncodeFrame(endServer.Result(), false)
	require.NoError(t, err)

	return append(frame, endFrame...)
}

func TestForwardAsChatCompletionsToolRoundTrip(t *testing.T) {
	gin.SetMode(gin.TestMode)
	account := cursorAccountWithFreshToken(41)
	svc := NewCursorGatewayService(nil, nil)
	svc.availableModels = func(context.Context, cursor.Credentials) ([]cursor.AvailableModel, error) {
		return nil, fmt.Errorf("catalog unused")
	}

	var captured cursor.AgentRunRequest
	svc.streamChat = func(_ context.Context, _ cursor.Credentials, req cursor.AgentRunRequest) (*http.Response, error) {
		captured = req
		body := encodeCursorToolCallBody(t, "call-77", "get_weather", `{"city":"Paris"}`)
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(bytes.NewReader(body))}, nil
	}

	body := []byte(`{
		"model": "claude-opus-5",
		"stream": true,
		"messages": [
			{"role": "user", "content": "weather in Paris?"}
		],
		"tools": [{
			"type": "function",
			"function": {
				"name": "get_weather",
				"description": "weather lookup",
				"parameters": {"type": "object", "properties": {"city": {"type": "string"}}}
			}
		}]
	}`)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(body))

	_, err := svc.ForwardAsChatCompletions(context.Background(), c, account, body)
	require.NoError(t, err)

	// Request side: Agent mode engaged with the declared tool table.
	require.Len(t, captured.Tools, 1)
	require.Equal(t, "get_weather", captured.Tools[0].Name)

	// Response side: tool call surfaced as delta.tool_calls with a
	// finish_reason of tool_calls, never a plain stop.
	output := rec.Body.String()
	require.Contains(t, output, `"tool_calls"`)
	require.Contains(t, output, `"get_weather"`)
	require.Contains(t, output, `"call-77"`)
	// Arguments are JSON-escaped inside the SSE payload.
	require.Contains(t, output, `{\"city\":\"Paris\"}`)
	require.Contains(t, output, `"finish_reason":"tool_calls"`)
	require.NotContains(t, output, `"finish_reason":"stop"`)
}

func TestForwardAsChatCompletionsToolRoundTripNonStream(t *testing.T) {
	gin.SetMode(gin.TestMode)
	account := cursorAccountWithFreshToken(42)
	svc := NewCursorGatewayService(nil, nil)
	svc.availableModels = func(context.Context, cursor.Credentials) ([]cursor.AvailableModel, error) {
		return nil, fmt.Errorf("catalog unused")
	}
	svc.streamChat = func(_ context.Context, _ cursor.Credentials, _ cursor.AgentRunRequest) (*http.Response, error) {
		body := encodeCursorToolCallBody(t, "call-78", "get_weather", `{"city":"Paris"}`)
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(bytes.NewReader(body))}, nil
	}

	body := []byte(`{
		"model": "claude-opus-5",
		"stream": false,
		"messages": [{"role": "user", "content": "weather?"}],
		"tools": [{"type": "function", "function": {"name": "get_weather"}}]
	}`)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(body))

	_, err := svc.ForwardAsChatCompletions(context.Background(), c, account, body)
	require.NoError(t, err)

	var parsed struct {
		Choices []struct {
			Message struct {
				ToolCalls []apicompat.ChatToolCall `json:"tool_calls"`
			} `json:"message"`
			FinishReason string `json:"finish_reason"`
		} `json:"choices"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &parsed))
	require.Len(t, parsed.Choices, 1)
	require.Equal(t, "tool_calls", parsed.Choices[0].FinishReason)
	require.Len(t, parsed.Choices[0].Message.ToolCalls, 1)
	require.Equal(t, "get_weather", parsed.Choices[0].Message.ToolCalls[0].Function.Name)
	require.JSONEq(t, `{"city":"Paris"}`, parsed.Choices[0].Message.ToolCalls[0].Function.Arguments)
}

func TestForwardAsAnthropicToolRoundTrip(t *testing.T) {
	gin.SetMode(gin.TestMode)
	account := cursorAccountWithFreshToken(43)
	svc := NewCursorGatewayService(nil, nil)
	svc.availableModels = func(context.Context, cursor.Credentials) ([]cursor.AvailableModel, error) {
		return nil, fmt.Errorf("catalog unused")
	}

	var captured cursor.AgentRunRequest
	svc.streamChat = func(_ context.Context, _ cursor.Credentials, req cursor.AgentRunRequest) (*http.Response, error) {
		captured = req
		body := encodeCursorToolCallBody(t, "toolu_1", "get_weather", `{"city":"Paris"}`)
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(bytes.NewReader(body))}, nil
	}

	body := []byte(`{
		"model": "claude-opus-5",
		"stream": true,
		"max_tokens": 1024,
		"messages": [{"role": "user", "content": "weather in Paris?"}],
		"tools": [{
			"name": "get_weather",
			"description": "weather lookup",
			"input_schema": {"type": "object", "properties": {"city": {"type": "string"}}}
		}]
	}`)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", bytes.NewReader(body))

	_, err := svc.ForwardAsAnthropic(context.Background(), c, account, body)
	require.NoError(t, err)

	// Agent mode engaged from Anthropic tools.
	require.Len(t, captured.Tools, 1)
	require.Equal(t, "get_weather", captured.Tools[0].Name)

	output := rec.Body.String()
	require.Contains(t, output, `"tool_use"`)
	require.Contains(t, output, `"get_weather"`)
	require.Contains(t, output, `"toolu_1"`)
	require.Contains(t, output, `"stop_reason":"tool_use"`)
}

func TestForwardAsResponsesToolRoundTrip(t *testing.T) {
	gin.SetMode(gin.TestMode)
	account := cursorAccountWithFreshToken(44)
	svc := NewCursorGatewayService(nil, nil)
	svc.availableModels = func(context.Context, cursor.Credentials) ([]cursor.AvailableModel, error) {
		return nil, fmt.Errorf("catalog unused")
	}
	svc.streamChat = func(_ context.Context, _ cursor.Credentials, req cursor.AgentRunRequest) (*http.Response, error) {
		require.Len(t, req.Tools, 1)
		body := encodeCursorToolCallBody(t, "fc_1", "get_weather", `{"city":"Paris"}`)
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(bytes.NewReader(body))}, nil
	}

	body := []byte(`{
		"model": "claude-opus-5",
		"stream": true,
		"input": [
			{"role": "user", "content": [{"type": "input_text", "text": "weather in Paris?"}]}
		],
		"tools": [{
			"type": "function",
			"name": "get_weather",
			"description": "weather lookup",
			"parameters": {"type": "object", "properties": {"city": {"type": "string"}}}
		}]
	}`)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewReader(body))

	_, err := svc.ForwardAsResponses(context.Background(), c, account, body)
	require.NoError(t, err)

	output := rec.Body.String()
	require.Contains(t, output, `"function_call"`)
	require.Contains(t, output, `"get_weather"`)
	require.Contains(t, output, `"fc_1"`)
}

func TestNonStreamCursorAsAnthropicCarriesToolCalls(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)

	body := encodeCursorToolCallBody(t, "toolu_2", "get_weather", `{"city":"Paris"}`)
	_, err := NewCursorGatewayService(nil, nil).nonStreamCursorAsAnthropic(c, bytes.NewReader(body), "claude-opus-5", nil, time.Now())
	require.NoError(t, err)

	output := rec.Body.String()
	require.Contains(t, output, `"type":"tool_use"`)
	require.Contains(t, output, `"get_weather"`)
	require.Contains(t, output, `"stop_reason":"tool_use"`)
}
