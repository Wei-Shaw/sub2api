package apicompat

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNormalizeApplyPatchDocument_RewritesGrokEnvelope(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "extra asterisks and trailing end of file",
			in:   "*** Begin Patch ***\n*** Add File: hello.txt\n+hello\n*** End Patch ***\n*** End of File ***\n",
			want: "*** Begin Patch\n*** Add File: hello.txt\n+hello\n*** End Patch\n",
		},
		{
			name: "already canonical",
			in:   "*** Begin Patch\n*** Add File: hello.txt\n+hello\n*** End Patch\n",
			want: "*** Begin Patch\n*** Add File: hello.txt\n+hello\n*** End Patch\n",
		},
		{
			name: "keeps hunk end of file without extra asterisks",
			in:   "*** Begin Patch\n*** Update File: a.txt\n@@\n-old\n+new\n*** End of File\n*** End Patch\n",
			want: "*** Begin Patch\n*** Update File: a.txt\n@@\n-old\n+new\n*** End of File\n*** End Patch\n",
		},
		{
			name: "strips invoke wrapper",
			in:   "<invoke name=\"apply_patch\">*** Begin Patch ***\n*** Add File: a.txt\n+x\n*** End Patch ***</invoke>",
			want: "*** Begin Patch\n*** Add File: a.txt\n+x\n*** End Patch\n",
		},
		{
			name: "drops junk after end patch",
			in:   "*** Begin Patch ***\n*** Add File: a.txt\n+x\n*** End Patch ***\n\n*** End of File\nnote\n",
			want: "*** Begin Patch\n*** Add File: a.txt\n+x\n*** End Patch\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tt.want, normalizeApplyPatchDocument(tt.in))
		})
	}
}

func TestExtractCustomToolCallInput_UnwrapsAndNormalizesApplyPatch(t *testing.T) {
	t.Parallel()

	require.Equal(t, "dir", extractCustomToolCallInput(`{"input": "dir"}`))
	require.Equal(t, "console.log(1)", extractCustomToolCallInput(`console.log(1)`))
	require.Equal(t, `{"other": "x"}`, extractCustomToolCallInput(`{"other": "x"}`))
	require.Equal(t, "", extractCustomToolCallInput(`{}`))
	require.Equal(t, "", extractCustomToolCallInput(""))

	got := extractCustomToolCallInput(`{"input":"*** Begin Patch ***\n*** Add File: hello.txt\n+hello\n*** End Patch ***\n*** End of File ***"}`)
	require.Equal(t, "*** Begin Patch\n*** Add File: hello.txt\n+hello\n*** End Patch\n", got)

	got = extractCustomToolCallInput(`{"patch":"*** Begin Patch ***\n*** Add File: hello.txt\n+hello\n*** End Patch ***"}`)
	require.Equal(t, "*** Begin Patch\n*** Add File: hello.txt\n+hello\n*** End Patch\n", got)

	raw := "*** Begin Patch ***\n*** Add File: hello.txt\n+hello\n*** End Patch ***"
	require.Equal(t, "*** Begin Patch\n*** Add File: hello.txt\n+hello\n*** End Patch\n", extractCustomToolCallInput(raw))
}

func TestAdaptResponsesClientTools_LowersNativeApplyPatch(t *testing.T) {
	t.Parallel()

	req := map[string]any{
		"tools": []any{
			map[string]any{"type": "apply_patch"},
			map[string]any{"type": "function", "name": "exec_command", "parameters": map[string]any{"type": "object"}},
		},
		"tool_choice": map[string]any{"type": "apply_patch"},
		"input": []any{
			map[string]any{
				"type": "apply_patch_call", "id": "ap_1", "call_id": "call_ap",
				"operation": map[string]any{"type": "update_file", "path": "a.txt", "diff": "@@\n-a\n+b"},
			},
			map[string]any{"type": "apply_patch_call_output", "id": "apo_1", "call_id": "call_ap", "output": "patched"},
		},
	}

	mapping, changed, err := AdaptResponsesClientTools(req)
	require.NoError(t, err)
	require.True(t, changed)
	require.True(t, mapping.CustomTools[applyPatchToolName])

	tools := requireResponsesClientToolValue[[]any](t, req["tools"])
	require.Len(t, tools, 2)
	patch := requireResponsesClientToolValue[map[string]any](t, tools[0])
	require.Equal(t, "function", patch["type"])
	require.Equal(t, applyPatchToolName, patch["name"])
	require.Equal(t, applyPatchToolDescription, patch["description"])
	parameters := requireResponsesClientToolValue[json.RawMessage](t, patch["parameters"])
	require.JSONEq(t, applyPatchToolInputSchema, string(parameters))
	require.Equal(t, "exec_command", requireResponsesClientToolValue[map[string]any](t, tools[1])["name"])

	choice := requireResponsesClientToolValue[map[string]any](t, req["tool_choice"])
	require.Equal(t, "function", choice["type"])
	require.Equal(t, applyPatchToolName, choice["name"])

	input := requireResponsesClientToolValue[[]any](t, req["input"])
	call := requireResponsesClientToolValue[map[string]any](t, input[0])
	require.Equal(t, "function_call", call["type"])
	require.Equal(t, applyPatchToolName, call["name"])
	require.JSONEq(t, `{"input":"*** Begin Patch\n*** Update File: a.txt\n@@\n-a\n+b\n*** End Patch\n"}`, requireResponsesClientToolValue[string](t, call["arguments"]))
	require.NotContains(t, call, "operation")
	output := requireResponsesClientToolValue[map[string]any](t, input[1])
	require.Equal(t, "function_call_output", output["type"])
}

func TestAdaptResponsesClientTools_DropsNativeApplyPatchWhenCustomExists(t *testing.T) {
	t.Parallel()

	req := map[string]any{
		"tools": []any{
			map[string]any{"type": "custom", "name": "apply_patch", "description": "custom patch"},
			map[string]any{"type": "apply_patch"},
		},
	}

	mapping, changed, err := AdaptResponsesClientTools(req)
	require.NoError(t, err)
	require.True(t, changed)
	require.True(t, mapping.CustomTools[applyPatchToolName])
	tools := requireResponsesClientToolValue[[]any](t, req["tools"])
	require.Len(t, tools, 1)
	patch := requireResponsesClientToolValue[map[string]any](t, tools[0])
	require.Equal(t, "function", patch["type"])
	require.Equal(t, "custom patch", patch["description"])
	parameters := requireResponsesClientToolValue[json.RawMessage](t, patch["parameters"])
	require.JSONEq(t, applyPatchToolInputSchema, string(parameters))
}

func TestRestoreResponsesClientToolPayload_NormalizesGrokApplyPatch(t *testing.T) {
	t.Parallel()

	mapping := ResponsesClientToolMapping{CustomTools: map[string]bool{"apply_patch": true}}
	payload := []byte(`{"id":"resp","output":[{"type":"function_call","id":"fc_1","call_id":"c1","name":"apply_patch","arguments":"{\"input\":\"*** Begin Patch ***\\n*** Add File: hello.txt\\n+hello\\n*** End Patch ***\\n*** End of File ***\"}"}]}`)

	restored, changed, err := RestoreResponsesClientToolPayload(payload, mapping)
	require.NoError(t, err)
	require.True(t, changed)
	require.JSONEq(t, `{"id":"resp","output":[{"type":"custom_tool_call","id":"ctc_1","call_id":"c1","name":"apply_patch","input":"*** Begin Patch\n*** Add File: hello.txt\n+hello\n*** End Patch\n"}]}`, string(restored))
}

func TestResponsesClientToolStreamRestorer_NormalizesApplyPatch(t *testing.T) {
	t.Parallel()

	restorer := NewResponsesClientToolStreamRestorer(ResponsesClientToolMapping{CustomTools: map[string]bool{"apply_patch": true}})
	added := restorer.Restore(ResponsesStreamEvent{Type: "response.output_item.added", SequenceNumber: 1, OutputIndex: 0, Item: &ResponsesOutput{Type: "function_call", ID: "fc_1", CallID: "c1", Name: "apply_patch", Status: "in_progress"}})
	require.Equal(t, "custom_tool_call", added[0].Item.Type)
	require.Equal(t, "ctc_1", added[0].Item.ID)

	arguments := "{\"input\":\"*** Begin Patch ***\\n*** Add File: hello.txt\\n+hello\\n*** End Patch ***\"}"
	done := restorer.Restore(ResponsesStreamEvent{Type: "response.function_call_arguments.done", SequenceNumber: 2, ItemID: "fc_1", CallID: "c1", Name: "apply_patch", Arguments: arguments})
	require.Len(t, done, 2)
	require.Equal(t, "response.custom_tool_call_input.delta", done[0].Type)
	require.Equal(t, "*** Begin Patch\n*** Add File: hello.txt\n+hello\n*** End Patch\n", done[0].Delta)
	require.Equal(t, "response.custom_tool_call_input.done", done[1].Type)
	require.Equal(t, "*** Begin Patch\n*** Add File: hello.txt\n+hello\n*** End Patch\n", done[1].Input)

	closed := restorer.Restore(ResponsesStreamEvent{Type: "response.output_item.done", SequenceNumber: 3, OutputIndex: 0, Item: &ResponsesOutput{Type: "function_call", ID: "fc_1", CallID: "c1", Name: "apply_patch", Arguments: arguments, Status: "completed"}})
	require.Equal(t, "custom_tool_call", closed[0].Item.Type)
	require.Equal(t, "*** Begin Patch\n*** Add File: hello.txt\n+hello\n*** End Patch\n", closed[0].Item.Input)
}
