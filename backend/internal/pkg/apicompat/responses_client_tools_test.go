package apicompat

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestAdaptResponsesClientTools_LowersDeclarationsHistoryChoiceAndNamespaces(t *testing.T) {
	req := map[string]any{
		"tools": []any{
			map[string]any{"type": "custom", "name": "exec", "format": map[string]any{"type": "grammar"}},
			map[string]any{"type": "tool_search"},
			map[string]any{"type": "namespace", "name": "team", "tools": []any{map[string]any{"type": "function", "name": "send"}}},
		},
		"tool_choice": map[string]any{"type": "custom", "name": "exec"},
		"input": []any{
			map[string]any{"type": "custom_tool_call", "call_id": "c1", "name": "exec", "input": "dir"},
			map[string]any{"type": "custom_tool_call_output", "call_id": "c1", "output": "ok"},
			map[string]any{"type": "tool_search_call", "call_id": "s1", "arguments": map[string]any{"query": "git"}},
			map[string]any{"type": "tool_search_output", "call_id": "s1", "output": map[string]any{"groups": []string{"git"}}},
			map[string]any{"type": "function_call", "call_id": "n1", "namespace": "team", "name": "send", "arguments": "{}"},
		},
	}

	mapping, changed, err := AdaptResponsesClientTools(req)
	require.NoError(t, err)
	require.True(t, changed)
	require.True(t, mapping.CustomTools["exec"])
	require.True(t, mapping.ToolSearch)
	require.Equal(t, ResponsesNamespaceName{Namespace: "team", Name: "send"}, mapping.NamespaceTools["team__send"])

	tools := requireResponsesClientToolValue[[]any](t, req["tools"])
	require.Len(t, tools, 3)
	exec := requireResponsesClientToolValue[map[string]any](t, tools[0])
	require.Equal(t, "function", exec["type"])
	parameters := requireResponsesClientToolValue[json.RawMessage](t, exec["parameters"])
	require.JSONEq(t, customToolInputSchema, string(parameters))
	search := requireResponsesClientToolValue[map[string]any](t, tools[1])
	require.Equal(t, toolSearchProxyName, search["name"])
	namespaceTool := requireResponsesClientToolValue[map[string]any](t, tools[2])
	require.Equal(t, "team__send", namespaceTool["name"])

	choice := requireResponsesClientToolValue[map[string]any](t, req["tool_choice"])
	require.Equal(t, "function", choice["type"])
	input := requireResponsesClientToolValue[[]any](t, req["input"])
	customCall := requireResponsesClientToolValue[map[string]any](t, input[0])
	require.Equal(t, "function_call", customCall["type"])
	require.JSONEq(t, `{"input":"dir"}`, requireResponsesClientToolValue[string](t, customCall["arguments"]))
	customOutput := requireResponsesClientToolValue[map[string]any](t, input[1])
	require.Equal(t, "function_call_output", customOutput["type"])
	searchCall := requireResponsesClientToolValue[map[string]any](t, input[2])
	require.Equal(t, "function_call", searchCall["type"])
	require.Equal(t, toolSearchProxyName, searchCall["name"])
	require.JSONEq(t, `{"query":"git"}`, requireResponsesClientToolValue[string](t, searchCall["arguments"]))
	searchOutput := requireResponsesClientToolValue[map[string]any](t, input[3])
	require.Equal(t, "function_call_output", searchOutput["type"])
	require.JSONEq(t, `{"groups":["git"]}`, requireResponsesClientToolValue[string](t, searchOutput["output"]))
	namespaceCall := requireResponsesClientToolValue[map[string]any](t, input[4])
	require.Equal(t, "team__send", namespaceCall["name"])
}

func requireResponsesClientToolValue[T any](t *testing.T, value any) T {
	t.Helper()
	typed, ok := value.(T)
	require.True(t, ok, "unexpected value type %T", value)
	return typed
}

func TestAdaptResponsesClientTools_RejectsAmbiguousNames(t *testing.T) {
	cases := []map[string]any{
		{"tools": []any{map[string]any{"type": "custom", "name": "same"}, map[string]any{"type": "function", "name": "same"}}},
		{"tools": []any{map[string]any{"type": "tool_search"}, map[string]any{"type": "function", "name": "tool_search"}}},
		{"tools": []any{map[string]any{"type": "function", "name": "team__send"}, map[string]any{"type": "namespace", "name": "team", "tools": []any{map[string]any{"type": "function", "name": "send"}}}}},
	}
	for _, req := range cases {
		_, _, err := AdaptResponsesClientTools(req)
		require.Error(t, err)
	}
}

func TestRestoreResponsesClientToolPayload_RestoresClientAndNamespaceCalls(t *testing.T) {
	mapping := ResponsesClientToolMapping{
		CustomTools: map[string]bool{"exec": true}, ToolSearch: true,
		NamespaceTools: map[string]ResponsesNamespaceName{"team__send": {Namespace: "team", Name: "send"}},
	}
	payload := []byte(`{"id":"resp","output":[{"type":"function_call","id":"i1","call_id":"c1","name":"exec","arguments":"{\"input\":\"dir\"}","namespace":"ignore"},{"type":"function_call","id":"i2","call_id":"s1","name":"tool_search","arguments":"{\"query\":\"git\"}"},{"type":"function_call","id":"i3","call_id":"n1","name":"team__send","arguments":"{}"}]}`)

	restored, changed, err := RestoreResponsesClientToolPayload(payload, mapping)
	require.NoError(t, err)
	require.True(t, changed)
	require.JSONEq(t, `{"id":"resp","output":[{"type":"custom_tool_call","id":"i1","call_id":"c1","name":"exec","input":"dir"},{"type":"tool_search_call","id":"i2","call_id":"s1","execution":"client","arguments":{"query":"git"}},{"type":"function_call","id":"i3","call_id":"n1","name":"send","namespace":"team","arguments":"{}"}]}`, string(restored))
}

func TestResponsesClientToolStreamRestorer_CustomToolBuffersWrapperAndSequences(t *testing.T) {
	restorer := NewResponsesClientToolStreamRestorer(ResponsesClientToolMapping{CustomTools: map[string]bool{"exec": true}})
	added := restorer.Restore(ResponsesStreamEvent{Type: "response.output_item.added", SequenceNumber: 7, OutputIndex: 0, Item: &ResponsesOutput{Type: "function_call", ID: "i1", CallID: "c1", Name: "exec", Status: "in_progress"}})
	require.Len(t, added, 1)
	require.Equal(t, 7, added[0].SequenceNumber)
	require.Equal(t, "custom_tool_call", added[0].Item.Type)
	require.Empty(t, restorer.Restore(ResponsesStreamEvent{Type: "response.function_call_arguments.delta", SequenceNumber: 8, ItemID: "i1", Delta: `{"input":"di`}))
	done := restorer.Restore(ResponsesStreamEvent{Type: "response.function_call_arguments.done", SequenceNumber: 9, ItemID: "i1", CallID: "c1", Name: "exec", Arguments: `{"input":"dir"}`})
	require.Len(t, done, 2)
	require.Equal(t, 8, done[0].SequenceNumber)
	require.Equal(t, "response.custom_tool_call_input.delta", done[0].Type)
	require.Equal(t, "dir", done[0].Delta)
	require.Equal(t, 9, done[1].SequenceNumber)
	require.Equal(t, "response.custom_tool_call_input.done", done[1].Type)
	require.Equal(t, "dir", done[1].Input)
	closed := restorer.Restore(ResponsesStreamEvent{Type: "response.output_item.done", SequenceNumber: 10, OutputIndex: 0, Item: &ResponsesOutput{Type: "function_call", ID: "i1", CallID: "c1", Name: "exec", Arguments: `{"input":"dir"}`, Status: "completed"}})
	require.Equal(t, 10, closed[0].SequenceNumber)
	require.Equal(t, "custom_tool_call", closed[0].Item.Type)
	require.Equal(t, "dir", closed[0].Item.Input)
}

func TestResponsesClientToolStreamRestorer_ToolSearchAndFunction(t *testing.T) {
	restorer := NewResponsesClientToolStreamRestorer(ResponsesClientToolMapping{ToolSearch: true})
	search := restorer.Restore(ResponsesStreamEvent{Type: "response.output_item.added", SequenceNumber: 0, OutputIndex: 0, Item: &ResponsesOutput{Type: "function_call", ID: "s1", CallID: "c1", Name: "tool_search", Status: "in_progress"}})
	require.Equal(t, "tool_search_call", search[0].Item.Type)
	require.Empty(t, restorer.Restore(ResponsesStreamEvent{Type: "response.function_call_arguments.delta", SequenceNumber: 1, ItemID: "s1", Delta: `{"query":"git"}`}))
	require.Empty(t, restorer.Restore(ResponsesStreamEvent{Type: "response.function_call_arguments.done", SequenceNumber: 2, ItemID: "s1", Arguments: `{"query":"git"}`}))
	closed := restorer.Restore(ResponsesStreamEvent{Type: "response.output_item.done", SequenceNumber: 3, OutputIndex: 0, Item: &ResponsesOutput{Type: "function_call", ID: "s1", CallID: "c1", Name: "tool_search", Status: "completed"}})
	require.Equal(t, 1, closed[0].SequenceNumber)
	require.Equal(t, "tool_search_call", closed[0].Item.Type)
	require.JSONEq(t, `{"query":"git"}`, string(toolSearchCallArgumentsJSON(closed[0].Item.Arguments)))

	function := restorer.Restore(ResponsesStreamEvent{Type: "response.function_call_arguments.done", SequenceNumber: 4, ItemID: "plain", Name: "plain", Arguments: "{}"})
	require.Len(t, function, 1)
	require.Equal(t, "response.function_call_arguments.done", function[0].Type)
	require.Equal(t, 2, function[0].SequenceNumber)
}

func TestResponsesClientToolStreamRestorer_RestoresNamespaceLifecycle(t *testing.T) {
	restorer := NewResponsesClientToolStreamRestorer(ResponsesClientToolMapping{
		NamespaceTools: map[string]ResponsesNamespaceName{
			"browser__open": {Namespace: "browser", Name: "open"},
		},
	})

	added, changed, err := restorer.RestoreEvent([]byte(`{"type":"response.output_item.added","sequence_number":4,"output_index":0,"item":{"type":"function_call","id":"i1","call_id":"c1","name":"browser__open","arguments":"","status":"in_progress"}}`))
	require.NoError(t, err)
	require.True(t, changed)
	require.Len(t, added, 1)
	require.Equal(t, "open", gjson.GetBytes(added[0], "item.name").String())
	require.Equal(t, "browser", gjson.GetBytes(added[0], "item.namespace").String())

	done, changed, err := restorer.RestoreEvent([]byte(`{"type":"response.function_call_arguments.done","sequence_number":5,"output_index":0,"item_id":"i1","name":"browser__open","arguments":"{}"}`))
	require.NoError(t, err)
	require.True(t, changed)
	require.Len(t, done, 1)
	require.Equal(t, "open", gjson.GetBytes(done[0], "name").String())
}

func TestResponsesClientToolStreamRestorer_RawEventsPreserveUnknownFieldsAndOutputFallback(t *testing.T) {
	restorer := NewResponsesClientToolStreamRestorer(ResponsesClientToolMapping{CustomTools: map[string]bool{"exec": true}})
	passthrough, changed, err := restorer.RestoreEvent([]byte(`{"type":"response.created","sequence_number":4,"response":{"id":"r"},"upstream_extension":{"keep":true}}`))
	require.NoError(t, err)
	require.False(t, changed)
	require.Len(t, passthrough, 1)
	require.Contains(t, string(passthrough[0]), `"upstream_extension":{"keep":true}`)

	restorer.Restore(ResponsesStreamEvent{Type: "response.output_item.added", SequenceNumber: 5, OutputIndex: 9, Item: &ResponsesOutput{Type: "function_call", ID: "item", CallID: "call", Name: "exec"}})
	// Some upstreams omit every tool identity field on later argument chunks.
	require.Empty(t, restorer.Restore(ResponsesStreamEvent{Type: "response.function_call_arguments.delta", SequenceNumber: 6, OutputIndex: 9, Delta: `{"input":"pwd"}`}))
	done := restorer.Restore(ResponsesStreamEvent{Type: "response.function_call_arguments.done", SequenceNumber: 7, OutputIndex: 9})
	require.Len(t, done, 2)
	require.Equal(t, "pwd", done[1].Input)
}
