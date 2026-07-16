package service

import (
	"encoding/json"
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
	// 所有 Codex 工具输出都应视为续链输出，避免 WS 续链时丢失 previous_response_id。
	require.False(t, HasFunctionCallOutput(nil))
	for _, typ := range []string{
		"function_call_output",
		"tool_search_output",
		"custom_tool_call_output",
		"mcp_tool_call_output",
	} {
		require.True(t, HasFunctionCallOutput(map[string]any{
			"input": []any{map[string]any{"type": typ}},
		}), typ)
	}
	require.False(t, HasFunctionCallOutput(map[string]any{
		"input": "text",
	}))
}

func TestHasToolCallContext(t *testing.T) {
	// 工具调用上下文必须包含 call_id，才能作为可关联上下文。
	require.False(t, HasToolCallContext(nil))
	for _, typ := range []string{
		"tool_call",
		"function_call",
		"local_shell_call",
		"tool_search_call",
		"custom_tool_call",
		"mcp_tool_call",
	} {
		require.True(t, HasToolCallContext(map[string]any{
			"input": []any{map[string]any{"type": typ, "call_id": "call_1"}},
		}), typ)
	}
	require.False(t, HasToolCallContext(map[string]any{
		"input": []any{map[string]any{"type": "tool_call"}},
	}))
}

func TestFunctionCallOutputCallIDs(t *testing.T) {
	// 仅提取工具输出的非空 call_id，去重后返回。
	require.Empty(t, FunctionCallOutputCallIDs(nil))
	callIDs := FunctionCallOutputCallIDs(map[string]any{
		"input": []any{
			map[string]any{"type": "function_call_output", "call_id": "call_1"},
			map[string]any{"type": "tool_search_output", "call_id": "call_search"},
			map[string]any{"type": "custom_tool_call_output", "call_id": "call_custom"},
			map[string]any{"type": "mcp_tool_call_output", "call_id": "call_mcp"},
			map[string]any{"type": "function_call_output", "call_id": ""},
			map[string]any{"type": "function_call_output", "call_id": "call_1"},
		},
	})
	require.ElementsMatch(t, []string{"call_1", "call_search", "call_custom", "call_mcp"}, callIDs)
}

func TestHasFunctionCallOutputMissingCallID(t *testing.T) {
	require.False(t, HasFunctionCallOutputMissingCallID(nil))
	require.True(t, HasFunctionCallOutputMissingCallID(map[string]any{
		"input": []any{map[string]any{"type": "function_call_output"}},
	}))
	require.True(t, HasFunctionCallOutputMissingCallID(map[string]any{
		"input": []any{map[string]any{"type": "tool_search_output"}},
	}))
	require.False(t, HasFunctionCallOutputMissingCallID(map[string]any{
		"input": []any{map[string]any{"type": "tool_search_output", "call_id": "call_1"}},
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

func TestValidateFunctionCallOutputContextBytesMatchesMapValidation(t *testing.T) {
	// handler 预校验走 raw JSON 扫描，语义必须与 service 内部 map 校验保持一致。
	cases := []struct {
		name string
		body map[string]any
	}{
		{
			name: "no_input",
			body: map[string]any{"model": "gpt-5.4"},
		},
		{
			name: "missing_call_id",
			body: map[string]any{"input": []any{map[string]any{"type": "function_call_output"}}},
		},
		{
			name: "call_id_without_reference",
			body: map[string]any{"input": []any{map[string]any{"type": "function_call_output", "call_id": "call_1"}}},
		},
		{
			name: "matching_reference",
			body: map[string]any{"input": []any{
				map[string]any{"type": "function_call_output", "call_id": "call_1"},
				map[string]any{"type": "item_reference", "id": "call_1"},
			}},
		},
		{
			name: "partial_reference",
			body: map[string]any{"input": []any{
				map[string]any{"type": "function_call_output", "call_id": "call_1"},
				map[string]any{"type": "tool_search_output", "call_id": "call_2"},
				map[string]any{"type": "item_reference", "id": "call_1"},
			}},
		},
		{
			name: "tool_context",
			body: map[string]any{"input": []any{
				map[string]any{"type": "function_call_output", "call_id": "call_1"},
				map[string]any{"type": "function_call", "call_id": "call_1"},
			}},
		},
		{
			name: "all_codex_tool_outputs",
			body: map[string]any{"input": []any{
				map[string]any{"type": "function_call_output", "call_id": "call_function"},
				map[string]any{"type": "tool_search_output", "call_id": "call_search"},
				map[string]any{"type": "custom_tool_call_output", "call_id": "call_custom"},
				map[string]any{"type": "mcp_tool_call_output", "call_id": "call_mcp"},
				map[string]any{"type": "item_reference", "id": "call_function"},
				map[string]any{"type": "item_reference", "id": "call_search"},
				map[string]any{"type": "item_reference", "id": "call_custom"},
				map[string]any{"type": "item_reference", "id": "call_mcp"},
			}},
		},
	}

	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			bodyBytes, err := json.Marshal(tt.body)
			require.NoError(t, err)

			require.Equal(t, ValidateFunctionCallOutputContext(tt.body), ValidateFunctionCallOutputContextBytes(bodyBytes))
		})
	}
}

func TestAnalyzeToolCallOutputContextCoverageBytes(t *testing.T) {
	cases := []struct {
		name         string
		body         map[string]any
		hasOutput    bool
		coversAllIDs bool
	}{
		{
			name:         "no_input",
			body:         map[string]any{"model": "gpt-5.1"},
			hasOutput:    false,
			coversAllIDs: false,
		},
		{
			name: "no_tool_output",
			body: map[string]any{"input": []any{
				map[string]any{"type": "message", "content": "hi"},
			}},
			hasOutput:    false,
			coversAllIDs: false,
		},
		{
			name: "all_outputs_covered_by_context",
			body: map[string]any{"input": []any{
				map[string]any{"type": "function_call", "call_id": "call_a"},
				map[string]any{"type": "function_call_output", "call_id": "call_a"},
			}},
			hasOutput:    true,
			coversAllIDs: true,
		},
		{
			name: "all_outputs_covered_by_item_reference",
			body: map[string]any{"input": []any{
				map[string]any{"type": "function_call_output", "call_id": "call_a"},
				map[string]any{"type": "item_reference", "id": "call_a"},
			}},
			hasOutput:    true,
			coversAllIDs: true,
		},
		{
			// 关键回归用例：input 内存在某一个上下文项，但另一个输出的 call_id
			// 只能由上游会话链（previous_response_id）解析——不可剥离。
			name: "partial_coverage_not_movable",
			body: map[string]any{"input": []any{
				map[string]any{"type": "function_call", "call_id": "call_a"},
				map[string]any{"type": "function_call_output", "call_id": "call_a"},
				map[string]any{"type": "function_call_output", "call_id": "call_b"},
			}},
			hasOutput:    true,
			coversAllIDs: false,
		},
		{
			name: "unrelated_context_does_not_cover",
			body: map[string]any{"input": []any{
				map[string]any{"type": "function_call", "call_id": "call_x"},
				map[string]any{"type": "function_call_output", "call_id": "call_b"},
			}},
			hasOutput:    true,
			coversAllIDs: false,
		},
		{
			name: "output_missing_call_id_not_movable",
			body: map[string]any{"input": []any{
				map[string]any{"type": "function_call", "call_id": "call_a"},
				map[string]any{"type": "function_call_output"},
				map[string]any{"type": "function_call_output", "call_id": "call_a"},
			}},
			hasOutput:    true,
			coversAllIDs: false,
		},
		{
			name: "mixed_context_and_reference_cover_all",
			body: map[string]any{"input": []any{
				map[string]any{"type": "function_call", "call_id": "call_a"},
				map[string]any{"type": "function_call_output", "call_id": "call_a"},
				map[string]any{"type": "function_call_output", "call_id": "call_b"},
				map[string]any{"type": "item_reference", "id": "call_b"},
			}},
			hasOutput:    true,
			coversAllIDs: true,
		},
		{
			name: "all_codex_output_types_covered",
			body: map[string]any{"input": []any{
				map[string]any{"type": "tool_search_output", "call_id": "call_s"},
				map[string]any{"type": "tool_search_call", "call_id": "call_s"},
				map[string]any{"type": "mcp_tool_call_output", "call_id": "call_m"},
				map[string]any{"type": "mcp_tool_call", "call_id": "call_m"},
			}},
			hasOutput:    true,
			coversAllIDs: true,
		},
	}

	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			bodyBytes, err := json.Marshal(tt.body)
			require.NoError(t, err)

			coverage := AnalyzeToolCallOutputContextCoverageBytes(bodyBytes)
			require.Equal(t, tt.hasOutput, coverage.HasFunctionCallOutput, "HasFunctionCallOutput")
			require.Equal(t, tt.coversAllIDs, coverage.ContextCoversAllCallIDs, "ContextCoversAllCallIDs")
		})
	}
}
