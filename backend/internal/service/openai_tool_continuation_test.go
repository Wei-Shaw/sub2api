package service

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNeedsToolContinuationSignals(t *testing.T) {
	// 覆盖所有触发续链的信号来源，确保判定逻辑完整。
	cases := []struct {
		name string
		body map[string]any
		want bool
	}{
		{name: "nil", body: nil, want: false},
		{name: "previous_response_id", body: map[string]any{"previous_response_id": "resp_1"}, want: true},
		{name: "previous_response_id_blank", body: map[string]any{"previous_response_id": "  "}, want: false},
		{name: "function_call_output", body: map[string]any{"input": []any{map[string]any{"type": "function_call_output"}}}, want: true},
		{name: "tool_search_output", body: map[string]any{"input": []any{map[string]any{"type": "tool_search_output"}}}, want: true},
		{name: "custom_tool_call_output", body: map[string]any{"input": []any{map[string]any{"type": "custom_tool_call_output"}}}, want: true},
		{name: "mcp_tool_call_output", body: map[string]any{"input": []any{map[string]any{"type": "mcp_tool_call_output"}}}, want: true},
		{name: "item_reference", body: map[string]any{"input": []any{map[string]any{"type": "item_reference"}}}, want: true},
		{name: "tools", body: map[string]any{"tools": []any{map[string]any{"type": "function"}}}, want: true},
		{name: "tools_empty", body: map[string]any{"tools": []any{}}, want: false},
		{name: "tools_invalid", body: map[string]any{"tools": "bad"}, want: false},
		{name: "tool_choice", body: map[string]any{"tool_choice": "auto"}, want: true},
		{name: "tool_choice_object", body: map[string]any{"tool_choice": map[string]any{"type": "function"}}, want: true},
		{name: "tool_choice_empty_object", body: map[string]any{"tool_choice": map[string]any{}}, want: false},
		{name: "none", body: map[string]any{"input": []any{map[string]any{"type": "text", "text": "hi"}}}, want: false},
	}

	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, NeedsToolContinuation(tt.body))
		})
	}
}

func TestHasFunctionCallOutput(t *testing.T) {
	// 仅当 input 中存在 function_call_output 才视为续链输出。
	require.False(t, HasFunctionCallOutput(nil))
	require.True(t, HasFunctionCallOutput(map[string]any{
		"input": []any{map[string]any{"type": "function_call_output"}},
	}))
	require.False(t, HasFunctionCallOutput(map[string]any{
		"input": "text",
	}))
}

func TestHasToolCallContext(t *testing.T) {
	// tool_call/function_call 必须包含 call_id，才能作为可关联上下文。
	require.False(t, HasToolCallContext(nil))
	require.True(t, HasToolCallContext(map[string]any{
		"input": []any{map[string]any{"type": "tool_call", "call_id": "call_1"}},
	}))
	require.True(t, HasToolCallContext(map[string]any{
		"input": []any{map[string]any{"type": "function_call", "call_id": "call_2"}},
	}))
	require.False(t, HasToolCallContext(map[string]any{
		"input": []any{map[string]any{"type": "tool_call"}},
	}))
}

func TestFunctionCallOutputCallIDs(t *testing.T) {
	// 仅提取非空 call_id，去重后返回。
	require.Empty(t, FunctionCallOutputCallIDs(nil))
	callIDs := FunctionCallOutputCallIDs(map[string]any{
		"input": []any{
			map[string]any{"type": "function_call_output", "call_id": "call_1"},
			map[string]any{"type": "function_call_output", "call_id": ""},
			map[string]any{"type": "function_call_output", "call_id": "call_1"},
		},
	})
	require.ElementsMatch(t, []string{"call_1"}, callIDs)
}

func TestHasFunctionCallOutputMissingCallID(t *testing.T) {
	require.False(t, HasFunctionCallOutputMissingCallID(nil))
	require.True(t, HasFunctionCallOutputMissingCallID(map[string]any{
		"input": []any{map[string]any{"type": "function_call_output"}},
	}))
	require.False(t, HasFunctionCallOutputMissingCallID(map[string]any{
		"input": []any{map[string]any{"type": "function_call_output", "call_id": "call_1"}},
	}))
}

func TestHasItemReferenceForCallIDs(t *testing.T) {
	// item_reference 需要覆盖所有 call_id 才视为可关联上下文。
	require.False(t, HasItemReferenceForCallIDs(nil, []string{"call_1"}))
	require.False(t, HasItemReferenceForCallIDs(map[string]any{}, []string{"call_1"}))
	req := map[string]any{
		"input": []any{
			map[string]any{"type": "item_reference", "id": "call_1"},
			map[string]any{"type": "item_reference", "id": "call_2"},
		},
	}
	require.True(t, HasItemReferenceForCallIDs(req, []string{"call_1"}))
	require.True(t, HasItemReferenceForCallIDs(req, []string{"call_1", "call_2"}))
	require.False(t, HasItemReferenceForCallIDs(req, []string{"call_1", "call_3"}))
}

func TestGetRequestTargetGroup(t *testing.T) {
	tests := []struct {
		name string
		body map[string]any
		want AccountTargetGroup
	}{
		{name: "nil", body: nil, want: TargetGroupActive},
		{name: "missing input", body: map[string]any{}, want: TargetGroupActive},
		{name: "empty input", body: map[string]any{"input": []any{}}, want: TargetGroupActive},
		{name: "last is function_call_output", body: map[string]any{"input": []any{map[string]any{"type": "message"}, map[string]any{"type": "function_call_output"}}}, want: TargetGroupExhausted},
		{name: "last type whitespace and mixed case", body: map[string]any{"input": []any{map[string]any{"type": "  Function_Call_Output  "}}}, want: TargetGroupExhausted},
		{name: "function_call_output not last", body: map[string]any{"input": []any{map[string]any{"type": "function_call_output"}, map[string]any{"type": "message"}}}, want: TargetGroupActive},
		{name: "last is message", body: map[string]any{"input": []any{map[string]any{"type": "message"}}}, want: TargetGroupActive},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, GetRequestTargetGroup(tt.body))
		})
	}
}

func TestNeedsSysToolContinuation(t *testing.T) {
	tests := []struct {
		name string
		body map[string]any
		want bool
	}{
		{name: "nil", body: nil, want: false},
		{name: "empty input", body: map[string]any{"input": []any{}}, want: false},
		{name: "last user message", body: map[string]any{"input": []any{map[string]any{"type": "message", "role": "user"}}}, want: true},
		{name: "last role-based user item without explicit type", body: map[string]any{"input": []any{map[string]any{"role": "user", "content": []any{map[string]any{"type": "input_text", "text": "hi"}}}}}, want: true},
		{name: "last user message whitespace and mixed case", body: map[string]any{"input": []any{map[string]any{"type": "  MeSsaGe ", "role": "  UsEr  "}}}, want: true},
		{name: "message but non user", body: map[string]any{"input": []any{map[string]any{"type": "message", "role": "assistant"}}}, want: false},
		{name: "last item reference", body: map[string]any{"input": []any{map[string]any{"type": "item_reference", "id": "x"}}}, want: true},
		{name: "last function_call_output", body: map[string]any{"input": []any{map[string]any{"type": "function_call_output", "call_id": "c1"}}}, want: false},
		{name: "other type", body: map[string]any{"input": []any{map[string]any{"type": "foo"}}}, want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, NeedsSysToolContinuation(tt.body))
		})
	}
}

func TestAppendMinimalSysToolContinuation(t *testing.T) {
	req := map[string]any{
		"input": []any{
			map[string]any{"type": "message", "role": "user"},
		},
	}

	AppendMinimalSysToolContinuation(req)

	input, ok := req["input"].([]any)
	require.True(t, ok)
	require.Len(t, input, 3)

	toolCall, ok := input[1].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "function_call", toolCall["type"])
	require.Equal(t, sysDummyToolCallID, toolCall["call_id"])
	require.Equal(t, sysDummyToolName, toolCall["name"])
	require.Equal(t, "{}", toolCall["arguments"])

	functionCallOutput, ok := input[2].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "function_call_output", functionCallOutput["type"])
	require.Equal(t, sysDummyToolCallID, functionCallOutput["call_id"])
	require.Equal(t, sysDummyToolOutput, functionCallOutput["output"])
}

func TestAppendMinimalSysToolContinuation_RoleBasedUserItemWithoutType(t *testing.T) {
	req := map[string]any{
		"input": []any{
			map[string]any{"role": "user", "content": []any{map[string]any{"type": "input_text", "text": "hello"}}},
		},
	}

	AppendMinimalSysToolContinuation(req)

	input, ok := req["input"].([]any)
	require.True(t, ok)
	require.Len(t, input, 3)
	toolCall, ok := input[1].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "function_call", toolCall["type"])
	functionCallOutput, ok := input[2].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "function_call_output", functionCallOutput["type"])
}

func TestAppendMinimalSysToolContinuation_MissingInput(t *testing.T) {
	req := map[string]any{}

	AppendMinimalSysToolContinuation(req)

	input, ok := req["input"].([]any)
	require.True(t, ok)
	require.Len(t, input, 2)
}

func TestAppendMinimalSysToolContinuation_InvalidInputTypeDoesNotOverwrite(t *testing.T) {
	req := map[string]any{"input": "keep-me"}

	AppendMinimalSysToolContinuation(req)

	require.Equal(t, "keep-me", req["input"])
}
