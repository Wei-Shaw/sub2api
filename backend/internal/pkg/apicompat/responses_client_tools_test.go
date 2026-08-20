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
			map[string]any{"type": "custom_tool_call", "id": "ctc_client", "call_id": "c1", "name": "exec", "input": "dir"},
			map[string]any{"type": "custom_tool_call_output", "id": "ctco_client", "call_id": "c1", "output": "ok"},
			map[string]any{"type": "tool_search_call", "id": "tsc_client", "call_id": "s1", "arguments": map[string]any{"query": "git"}},
			map[string]any{"type": "tool_search_output", "id": "tso_client", "call_id": "s1", "output": map[string]any{"groups": []string{"git"}}},
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
	require.NotContains(t, customCall, "id")
	require.JSONEq(t, `{"input":"dir"}`, requireResponsesClientToolValue[string](t, customCall["arguments"]))
	customOutput := requireResponsesClientToolValue[map[string]any](t, input[1])
	require.Equal(t, "function_call_output", customOutput["type"])
	require.NotContains(t, customOutput, "id")
	searchCall := requireResponsesClientToolValue[map[string]any](t, input[2])
	require.Equal(t, "function_call", searchCall["type"])
	require.NotContains(t, searchCall, "id")
	require.Equal(t, toolSearchProxyName, searchCall["name"])
	require.JSONEq(t, `{"query":"git"}`, requireResponsesClientToolValue[string](t, searchCall["arguments"]))
	searchOutput := requireResponsesClientToolValue[map[string]any](t, input[3])
	require.Equal(t, "function_call_output", searchOutput["type"])
	require.NotContains(t, searchOutput, "id")
	require.JSONEq(t, `{"groups":["git"]}`, requireResponsesClientToolValue[string](t, searchOutput["output"]))
	namespaceCall := requireResponsesClientToolValue[map[string]any](t, input[4])
	require.Equal(t, "team__send", namespaceCall["name"])
}

func TestAdaptResponsesClientTools_LowersDiscoveredToolSearchOutput(t *testing.T) {
	requestJSON := `{
		"tools":[{"type":"tool_search"}],
		"input":[
			{"type":"tool_search_call","id":"tsc_client","call_id":"call_search","arguments":{"query":"codex app"},"execution":"client","status":"completed"},
			{"type":"tool_search_output","id":"tso_client","call_id":"call_search","execution":"client","status":"completed","tools":[
				{"type":"namespace","name":"codex_app","tools":[{"type":"function","name":"load_workspace_dependencies","description":"Load workspace dependencies","parameters":{"type":"object","properties":{},"additionalProperties":false}}]},
				{"type":"namespace","name":"multi_agent_v1","tools":[
					{"type":"function","name":"spawn_agent","description":"Spawn an agent","parameters":{"type":"object","properties":{"message":{"type":"string"}},"required":["message"],"additionalProperties":false}},
					{"type":"function","name":"wait_agent","description":"Wait for agents","parameters":{"type":"object","properties":{},"additionalProperties":false}}
				]}
			]}
		]
	}`

	type adaptedRequest struct {
		req     map[string]any
		mapping ResponsesClientToolMapping
	}
	adapt := func() adaptedRequest {
		var req map[string]any
		require.NoError(t, json.Unmarshal([]byte(requestJSON), &req))
		mapping, changed, err := AdaptResponsesClientTools(req)
		require.NoError(t, err)
		require.True(t, changed)
		return adaptedRequest{req: req, mapping: mapping}
	}

	first := adapt()
	second := adapt()
	firstInput := requireResponsesClientToolValue[[]any](t, first.req["input"])
	secondInput := requireResponsesClientToolValue[[]any](t, second.req["input"])

	tools := requireResponsesClientToolValue[[]any](t, first.req["tools"])
	require.Len(t, tools, 4)
	require.Equal(t, []string{
		"tool_search",
		"codex_app__load_workspace_dependencies",
		"multi_agent_v1__spawn_agent",
		"multi_agent_v1__wait_agent",
	}, responsesClientToolNames(t, tools))
	require.Equal(t, ResponsesNamespaceName{Namespace: "multi_agent_v1", Name: "spawn_agent"}, first.mapping.NamespaceTools["multi_agent_v1__spawn_agent"])
	require.Equal(t, ResponsesNamespaceName{Namespace: "multi_agent_v1", Name: "wait_agent"}, first.mapping.NamespaceTools["multi_agent_v1__wait_agent"])

	call := requireResponsesClientToolValue[map[string]any](t, firstInput[0])
	require.Equal(t, "function_call", call["type"])
	require.Equal(t, toolSearchProxyName, call["name"])
	require.JSONEq(t, `{"query":"codex app"}`, requireResponsesClientToolValue[string](t, call["arguments"]))
	require.NotContains(t, call, "execution")

	output := requireResponsesClientToolValue[map[string]any](t, firstInput[1])
	require.Equal(t, map[string]any{
		"type":    "function_call_output",
		"call_id": "call_search",
		"output":  output["output"],
	}, output)
	outputText := requireResponsesClientToolValue[string](t, output["output"])
	require.JSONEq(t, `[
		{"type":"namespace","name":"codex_app","tools":[{"type":"function","name":"load_workspace_dependencies","description":"Load workspace dependencies","parameters":{"type":"object","properties":{},"additionalProperties":false}}]},
		{"type":"namespace","name":"multi_agent_v1","tools":[
			{"type":"function","name":"spawn_agent","description":"Spawn an agent","parameters":{"type":"object","properties":{"message":{"type":"string"}},"required":["message"],"additionalProperties":false}},
			{"type":"function","name":"wait_agent","description":"Wait for agents","parameters":{"type":"object","properties":{},"additionalProperties":false}}
		]}
	]`, outputText)
	secondOutput := requireResponsesClientToolValue[map[string]any](t, secondInput[1])
	require.Equal(t, outputText, secondOutput["output"], "tool discovery output encoding must be deterministic")

	restored, changed, err := RestoreResponsesClientToolPayload(
		[]byte(`{"output":[{"type":"function_call","name":"multi_agent_v1__spawn_agent","call_id":"call_spawn","arguments":"{\"message\":\"work\"}"}]}`),
		first.mapping,
	)
	require.NoError(t, err)
	require.True(t, changed)
	require.JSONEq(t, `{"output":[{"type":"function_call","name":"spawn_agent","namespace":"multi_agent_v1","call_id":"call_spawn","arguments":"{\"message\":\"work\"}"}]}`, string(restored))
}

func TestAdaptResponsesClientTools_PromotesDirectDiscoveryAndDeduplicatesIdenticalDeclarations(t *testing.T) {
	direct := map[string]any{
		"type": "function", "name": "inspect_result", "description": "Inspect a result",
		"parameters": map[string]any{"type": "object", "properties": map[string]any{}},
	}
	custom := map[string]any{
		"type": "custom", "name": "run_script", "description": "Run a script",
		"format": map[string]any{"type": "grammar"},
	}
	namespace := map[string]any{
		"type": "namespace", "name": "multi_agent_v1", "tools": []any{map[string]any{
			"type": "function", "name": "spawn_agent", "parameters": map[string]any{"type": "object"},
		}},
	}
	req := map[string]any{
		"tools": []any{
			map[string]any{"type": "function", "name": "static_first", "parameters": map[string]any{"type": "object"}},
			map[string]any{"type": "tool_search"},
		},
		"input": []any{
			map[string]any{"type": "tool_search_output", "status": "completed", "call_id": "search_1", "tools": []any{direct, custom, namespace}},
			map[string]any{"type": "tool_search_output", "status": "completed", "call_id": "search_2", "tools": []any{copyClientTool(direct), copyClientTool(custom), copyClientTool(namespace)}},
		},
	}

	mapping, changed, err := AdaptResponsesClientTools(req)
	require.NoError(t, err)
	require.True(t, changed)
	require.True(t, mapping.CustomTools["run_script"])
	require.Equal(t, ResponsesNamespaceName{Namespace: "multi_agent_v1", Name: "spawn_agent"}, mapping.NamespaceTools["multi_agent_v1__spawn_agent"])
	tools := requireResponsesClientToolValue[[]any](t, req["tools"])
	require.Equal(t, []string{"static_first", "tool_search", "inspect_result", "run_script", "multi_agent_v1__spawn_agent"}, responsesClientToolNames(t, tools))
	customTool := requireResponsesClientToolValue[map[string]any](t, tools[3])
	require.Equal(t, "function", customTool["type"])
	require.NotContains(t, customTool, "format")
	for _, raw := range requireResponsesClientToolValue[[]any](t, req["input"]) {
		item := requireResponsesClientToolValue[map[string]any](t, raw)
		require.Equal(t, "function_call_output", item["type"])
		require.NotContains(t, item, "tools")
		require.NotContains(t, item, "status")
	}
}

func TestAdaptResponsesClientTools_RejectsDiscoveredSchemaAndNamespaceCollisions(t *testing.T) {
	tests := []struct {
		name        string
		staticTools []any
		discovered  []any
	}{
		{
			name: "direct schema collision",
			staticTools: []any{map[string]any{
				"type": "function", "name": "inspect", "parameters": map[string]any{"type": "object"},
			}},
			discovered: []any{map[string]any{
				"type": "function", "name": "inspect", "parameters": map[string]any{"type": "string"},
			}},
		},
		{
			name: "namespace schema collision",
			staticTools: []any{map[string]any{
				"type": "namespace", "name": "multi_agent_v1", "tools": []any{map[string]any{
					"type": "function", "name": "spawn_agent", "parameters": map[string]any{"type": "object"},
				}},
			}},
			discovered: []any{map[string]any{
				"type": "namespace", "name": "multi_agent_v1", "tools": []any{map[string]any{
					"type": "function", "name": "spawn_agent", "parameters": map[string]any{"type": "string"},
				}},
			}},
		},
		{
			name:        "flattened namespace collision",
			staticTools: []any{map[string]any{"type": "function", "name": "multi_agent_v1__spawn_agent"}},
			discovered: []any{map[string]any{
				"type": "namespace", "name": "multi_agent_v1", "tools": []any{map[string]any{"type": "function", "name": "spawn_agent"}},
			}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := map[string]any{
				"tools": append(tt.staticTools, map[string]any{"type": "tool_search"}),
				"input": []any{map[string]any{
					"type": "tool_search_output", "status": "completed", "tools": tt.discovered,
				}},
			}
			_, _, err := AdaptResponsesClientTools(req)
			require.ErrorContains(t, err, "conflicts")
		})
	}
}

func TestAdaptResponsesClientTools_DoesNotPromoteUnusableDiscoveries(t *testing.T) {
	req := map[string]any{
		"tools": []any{map[string]any{"type": "tool_search"}},
		"input": []any{
			map[string]any{"type": "tool_search_output", "call_id": "search_in_progress", "status": "in_progress", "tools": []any{map[string]any{"type": "function", "name": "not_ready"}}},
			map[string]any{"type": "tool_search_output", "call_id": "search_malformed", "status": "completed", "tools": []any{map[string]any{"type": "function"}}},
		},
	}

	_, changed, err := AdaptResponsesClientTools(req)
	require.NoError(t, err)
	require.True(t, changed, "the static tool_search declaration is still lowered")
	tools := requireResponsesClientToolValue[[]any](t, req["tools"])
	require.Equal(t, []string{"tool_search"}, responsesClientToolNames(t, tools))
}

func responsesClientToolNames(t *testing.T, tools []any) []string {
	t.Helper()
	names := make([]string, 0, len(tools))
	for _, raw := range tools {
		tool := requireResponsesClientToolValue[map[string]any](t, raw)
		names = append(names, requireResponsesClientToolValue[string](t, tool["name"]))
	}
	return names
}

func TestAdaptResponsesClientTools_ToolSearchOutputEdgeCases(t *testing.T) {
	unencodableOutput := make(chan struct{})
	tests := []struct {
		name             string
		item             map[string]any
		wantOutput       any
		wantOutputExists bool
		wantPrivateKeys  []string
		wantExactOutput  bool
		wantErr          bool
	}{
		{
			name:             "absent tools and output is rejected",
			item:             map[string]any{"type": "tool_search_output", "call_id": "call_empty", "status": "completed"},
			wantOutputExists: false,
			wantErr:          true,
		},
		{
			name: "preexisting string output wins",
			item: map[string]any{
				"type": "tool_search_output", "call_id": "call_legacy", "output": "legacy",
				"tools": []any{map[string]any{"type": "function", "name": "ignored"}}, "execution": "client",
			},
			wantOutput:       "legacy",
			wantOutputExists: true,
			wantExactOutput:  true,
		},
		{
			name: "preexisting object output remains legacy representation",
			item: map[string]any{
				"type": "tool_search_output", "call_id": "call_object", "output": map[string]any{"groups": []any{"github"}},
				"tools": []any{map[string]any{"type": "function", "name": "ignored"}},
			},
			wantOutput:       `{"groups":["github"]}`,
			wantOutputExists: true,
			wantExactOutput:  true,
		},
		{
			name: "unencodable preexisting output is rejected",
			item: map[string]any{
				"type": "tool_search_output", "call_id": "call_bad_output", "output": unencodableOutput,
				"tools": []any{map[string]any{"type": "function", "name": "retained"}}, "status": "completed", "execution": "client",
			},
			wantOutput: unencodableOutput,
			wantErr:    true,
		},
		{
			name: "empty tools array is a valid empty output",
			item: map[string]any{
				"type": "tool_search_output", "call_id": "call_empty_tools",
				"tools": []any{}, "status": "completed", "execution": "client",
			},
			wantOutput:       `[]`,
			wantOutputExists: true,
			wantExactOutput:  true,
		},
		{
			name: "non-array tools value is serialized directly",
			item: map[string]any{
				"type": "tool_search_output", "call_id": "call_malformed",
				"tools": map[string]any{"unexpected": true}, "status": "completed", "execution": "client",
			},
			wantOutput:       `{"unexpected":true}`,
			wantOutputExists: true,
			wantExactOutput:  true,
		},
		{
			name: "unencodable tools is rejected",
			item: map[string]any{
				"type": "tool_search_output", "call_id": "call_unencodable", "tools": make(chan struct{}), "status": "completed",
			},
			wantErr: true,
		},
		{
			name: "missing call id is rejected",
			item: map[string]any{
				"type": "tool_search_output", "tools": []any{}, "status": "completed",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := map[string]any{
				"tools": []any{map[string]any{"type": "tool_search"}},
				"input": []any{tt.item},
			}
			_, changed, err := AdaptResponsesClientTools(req)
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.True(t, changed)
			input := requireResponsesClientToolValue[[]any](t, req["input"])
			output := requireResponsesClientToolValue[map[string]any](t, input[0])
			require.Equal(t, "function_call_output", output["type"])
			actualOutput, outputExists := output["output"]
			require.Equal(t, tt.wantOutputExists, outputExists)
			if tt.wantOutputExists {
				require.Equal(t, tt.wantOutput, actualOutput)
			}
			if tt.wantExactOutput {
				require.Equal(t, map[string]any{
					"type":    "function_call_output",
					"call_id": output["call_id"],
					"output":  tt.wantOutput,
				}, output)
			}
			if len(tt.wantPrivateKeys) > 0 {
				for _, key := range tt.wantPrivateKeys {
					require.Contains(t, output, key)
				}
			} else {
				require.NotContains(t, output, "tools")
				require.NotContains(t, output, "status")
				require.NotContains(t, output, "execution")
			}
		})
	}
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

func TestAdaptResponsesClientToolsWithInheritedMapping_LowersFollowupHistoryWithoutTools(t *testing.T) {
	req := map[string]any{
		"input": []any{
			map[string]any{
				"type": "custom_tool_call", "name": "exec",
				"call_id": "call_1", "input": "pwd",
			},
			map[string]any{
				"type": "custom_tool_call_output", "call_id": "call_1",
				"id":     "ctco_client_output_1",
				"output": []any{map[string]any{"type": "input_text", "text": "ok"}},
			},
		},
	}
	inherited := ResponsesClientToolMapping{CustomTools: map[string]bool{"exec": true}}

	mapping, changed, err := AdaptResponsesClientToolsWithInheritedMapping(req, inherited)

	require.NoError(t, err)
	require.True(t, changed)
	require.Equal(t, inherited, mapping)
	items := requireResponsesClientToolValue[[]any](t, req["input"])
	call := requireResponsesClientToolValue[map[string]any](t, items[0])
	require.Equal(t, "function_call", call["type"])
	require.JSONEq(t, `{"input":"pwd"}`, requireResponsesClientToolValue[string](t, call["arguments"]))
	require.NotContains(t, call, "input")
	output := requireResponsesClientToolValue[map[string]any](t, items[1])
	require.Equal(t, "function_call_output", output["type"])
	require.NotContains(t, output, "id")
	require.JSONEq(t, `[{"text":"ok","type":"input_text"}]`, requireResponsesClientToolValue[string](t, output["output"]))
}

func TestAdaptResponsesClientToolsWithInheritedMapping_PromotesOmittedToolsDiscoveryIntoEffectiveDeclarations(t *testing.T) {
	req := map[string]any{
		"input": []any{map[string]any{
			"type": "tool_search_output", "call_id": "call_search", "status": "completed", "execution": "client",
			"tools": []any{map[string]any{
				"type": "namespace", "name": "multi_agent_v1", "tools": []any{map[string]any{
					"type": "function", "name": "spawn_agent", "parameters": map[string]any{"type": "object"},
				}},
			}},
		}},
	}
	inherited := ResponsesClientToolMapping{
		ToolSearch: true,
		NamespaceTools: map[string]ResponsesNamespaceName{
			"codex_app__read_resource": {Namespace: "codex_app", Name: "read_resource"},
		},
	}
	lowered := []any{
		map[string]any{"type": "function", "name": "static_first", "parameters": map[string]any{"type": "object"}},
		map[string]any{"type": "function", "name": "tool_search", "parameters": json.RawMessage(toolSearchProxySchema)},
		map[string]any{"type": "function", "name": "codex_app__read_resource", "parameters": map[string]any{"type": "object"}},
	}

	mapping, changed, err := AdaptResponsesClientToolsWithInheritedMapping(req, inherited, lowered)
	require.NoError(t, err)
	require.True(t, changed)
	require.True(t, mapping.ToolSearch)
	require.Equal(t, ResponsesNamespaceName{Namespace: "codex_app", Name: "read_resource"}, mapping.NamespaceTools["codex_app__read_resource"])
	require.Equal(t, ResponsesNamespaceName{Namespace: "multi_agent_v1", Name: "spawn_agent"}, mapping.NamespaceTools["multi_agent_v1__spawn_agent"])
	tools := requireResponsesClientToolValue[[]any](t, req["tools"])
	require.Equal(t, []string{
		"static_first", "tool_search", "codex_app__read_resource", "multi_agent_v1__spawn_agent",
	}, responsesClientToolNames(t, tools))
	output := requireResponsesClientToolValue[map[string]any](t, requireResponsesClientToolValue[[]any](t, req["input"])[0])
	require.Equal(t, "function_call_output", output["type"])
	require.IsType(t, "", output["output"])
	require.NotContains(t, output, "tools")
	require.NotContains(t, output, "status")
	require.NotContains(t, output, "execution")
}

func TestAdaptResponsesClientToolsWithInheritedMapping_ExplicitToolsReplaceInheritedMapping(t *testing.T) {
	req := map[string]any{
		"tools": []any{},
		"input": []any{map[string]any{
			"type": "custom_tool_call", "name": "exec", "input": "pwd",
		}},
	}

	mapping, changed, err := AdaptResponsesClientToolsWithInheritedMapping(
		req,
		ResponsesClientToolMapping{CustomTools: map[string]bool{"exec": true}},
	)

	require.NoError(t, err)
	require.False(t, changed)
	require.Empty(t, mapping)
	items := requireResponsesClientToolValue[[]any](t, req["input"])
	call := requireResponsesClientToolValue[map[string]any](t, items[0])
	require.Equal(t, "custom_tool_call", call["type"])
}

func TestAdaptResponsesClientToolsWithInheritedMapping_ExplicitToolResetDoesNotPromoteDiscovery(t *testing.T) {
	for _, reset := range []any{nil, []any{}} {
		req := map[string]any{
			"tools": reset,
			"input": []any{map[string]any{
				"type": "tool_search_output", "call_id": "call_reset", "status": "completed",
				"tools": []any{map[string]any{"type": "function", "name": "must_not_promote"}},
			}},
		}
		mapping, changed, err := AdaptResponsesClientToolsWithInheritedMapping(
			req,
			ResponsesClientToolMapping{ToolSearch: true},
			[]any{map[string]any{"type": "function", "name": "tool_search"}},
		)
		require.NoError(t, err)
		require.False(t, changed)
		require.Empty(t, mapping)
		item := requireResponsesClientToolValue[map[string]any](t, requireResponsesClientToolValue[[]any](t, req["input"])[0])
		require.Equal(t, "tool_search_output", item["type"])
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

	delta, changed, err := restorer.RestoreEvent([]byte(`{"type":"response.function_call_arguments.delta","sequence_number":5,"output_index":0,"item_id":"i1","name":"browser__open","delta":"{\"url\":"}`))
	require.NoError(t, err)
	require.True(t, changed)
	require.Len(t, delta, 1)
	require.Equal(t, "open", gjson.GetBytes(delta[0], "name").String())

	done, changed, err := restorer.RestoreEvent([]byte(`{"type":"response.function_call_arguments.done","sequence_number":6,"output_index":0,"item_id":"i1","name":"browser__open","arguments":"{}"}`))
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

func TestResponsesClientToolStreamRestorer_RestoresAllTerminalEvents(t *testing.T) {
	for _, eventType := range []string{
		"response.completed",
		"response.done",
		"response.incomplete",
		"response.failed",
		"response.cancelled",
		"response.canceled",
	} {
		t.Run(eventType, func(t *testing.T) {
			restorer := NewResponsesClientToolStreamRestorer(ResponsesClientToolMapping{CustomTools: map[string]bool{"exec": true}})
			payload := []byte(`{"type":"` + eventType + `","sequence_number":7,"response":{"id":"resp_tools","output":[{"type":"function_call","id":"item_exec","call_id":"call_exec","name":"exec","arguments":"{\"input\":\"pwd\"}"}]}}`)

			restored, changed, err := restorer.RestoreEvent(payload)

			require.NoError(t, err)
			require.True(t, changed)
			require.Len(t, restored, 1)
			require.Equal(t, eventType, gjson.GetBytes(restored[0], "type").String())
			require.Equal(t, int64(7), gjson.GetBytes(restored[0], "sequence_number").Int())
			require.Equal(t, "custom_tool_call", gjson.GetBytes(restored[0], "response.output.0.type").String())
			require.Equal(t, "pwd", gjson.GetBytes(restored[0], "response.output.0.input").String())
			require.False(t, gjson.GetBytes(restored[0], "response.output.0.arguments").Exists())
		})
	}
}
