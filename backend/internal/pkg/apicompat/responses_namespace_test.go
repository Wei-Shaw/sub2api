package apicompat

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestFlattenResponsesNamespaces_RewritesDeclarationHistoryAndChoice(t *testing.T) {
	req := map[string]any{
		"model": "gpt-5.5",
		"tools": []any{
			map[string]any{"type": "function", "name": "plain", "description": "keep"},
			map[string]any{
				"type": "namespace",
				"name": "collaboration",
				"tools": []any{
					map[string]any{"type": "function", "name": "spawn_agent", "description": "spawn", "parameters": map[string]any{"type": "object"}},
				},
			},
		},
		"tool_choice": map[string]any{"type": "function", "name": "spawn_agent", "namespace": "collaboration"},
		"input": []any{
			map[string]any{"type": "function_call", "call_id": "call_1", "name": "spawn_agent", "namespace": "collaboration", "arguments": "{}"},
			map[string]any{"type": "message", "role": "user", "content": "hi", "name": "spawn_agent", "namespace": "collaboration"},
		},
	}

	names, changed, err := FlattenResponsesNamespaces(req)
	require.NoError(t, err)
	require.True(t, changed)
	require.Equal(t, ResponsesNamespaceName{Namespace: "collaboration", Name: "spawn_agent"}, names["collaboration__spawn_agent"])

	tools, ok := req["tools"].([]any)
	require.True(t, ok)
	require.Len(t, tools, 2)
	plainTool, ok := tools[0].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "plain", plainTool["name"])
	flatTool, ok := tools[1].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "collaboration__spawn_agent", flatTool["name"])
	require.Equal(t, "spawn", flatTool["description"])

	choice, ok := req["tool_choice"].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "collaboration__spawn_agent", choice["name"])
	require.NotContains(t, choice, "namespace")

	input, ok := req["input"].([]any)
	require.True(t, ok)
	require.Len(t, input, 2)
	call, ok := input[0].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "collaboration__spawn_agent", call["name"])
	require.NotContains(t, call, "namespace")
	message, ok := input[1].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "spawn_agent", message["name"])
	require.Equal(t, "collaboration", message["namespace"])
	require.Equal(t, "gpt-5.5", req["model"])
}

func TestFlattenResponsesNamespaces_RejectsFlatNameCollision(t *testing.T) {
	req := map[string]any{"tools": []any{
		map[string]any{"type": "function", "name": "collaboration__spawn_agent"},
		map[string]any{"type": "namespace", "name": "collaboration", "tools": []any{
			map[string]any{"type": "function", "name": "spawn_agent"},
		}},
	}}

	_, _, err := FlattenResponsesNamespaces(req)
	require.ErrorContains(t, err, "conflicts with a top-level tool")
}

func TestFlattenResponsesNamespaces_NamespaceGroupChoiceFallsBackToAuto(t *testing.T) {
	req := map[string]any{
		"tools": []any{map[string]any{
			"type": "namespace", "name": "collaboration", "tools": []any{
				map[string]any{"type": "function", "name": "spawn_agent"},
				map[string]any{"type": "function", "name": "send_message"},
			},
		}},
		"tool_choice": map[string]any{"type": "namespace", "name": "collaboration"},
	}

	_, changed, err := FlattenResponsesNamespaces(req)
	require.NoError(t, err)
	require.True(t, changed)
	require.Equal(t, "auto", req["tool_choice"])
}

func TestFlattenResponsesNamespaces_FlattensDynamicAdditionalTools(t *testing.T) {
	arguments := `{"code":"nodeRepl.write(6 * 7)","title":"Node REPL routing test","timeout_ms":10000}`
	req := map[string]any{
		"tools": []any{
			map[string]any{"type": "function", "name": "get_goal"},
			map[string]any{"type": "tool_search"},
		},
		"input": []any{
			map[string]any{"type": "tool_search_output", "call_id": "search_1", "output": map[string]any{"groups": []any{"mcp__node_repl"}}},
			map[string]any{
				"type": "additional_tools",
				"tools": []any{map[string]any{
					"type": "namespace",
					"name": "mcp__node_repl",
					"tools": []any{map[string]any{
						"type": "function",
						"name": "js",
					}},
				}},
			},
			map[string]any{
				"type":      "function_call",
				"call_id":   "call_js",
				"namespace": "mcp__node_repl",
				"name":      "js",
				"arguments": arguments,
			},
		},
	}

	names, changed, err := FlattenResponsesNamespaces(req)

	require.NoError(t, err)
	require.True(t, changed)
	require.Equal(t, ResponsesNamespaceName{Namespace: "mcp__node_repl", Name: "js"}, names["mcp__node_repl__js"])
	input := req["input"].([]any)
	additional := input[1].(map[string]any)["tools"].([]any)
	require.Len(t, additional, 1)
	require.Equal(t, "function", additional[0].(map[string]any)["type"])
	require.Equal(t, "mcp__node_repl__js", additional[0].(map[string]any)["name"])
	call := input[2].(map[string]any)
	require.Equal(t, "mcp__node_repl__js", call["name"])
	require.NotContains(t, call, "namespace")
	require.Equal(t, arguments, call["arguments"])
}

func TestFlattenResponsesNamespaces_RejectsDynamicNamespaceCollision(t *testing.T) {
	req := map[string]any{
		"tools": []any{map[string]any{"type": "function", "name": "mcp__node_repl__js"}},
		"input": []any{map[string]any{
			"type": "additional_tools",
			"tools": []any{map[string]any{
				"type":  "namespace",
				"name":  "mcp__node_repl",
				"tools": []any{map[string]any{"type": "function", "name": "js"}},
			}},
		}},
	}

	_, _, err := FlattenResponsesNamespaces(req)

	require.ErrorContains(t, err, "conflicts with a top-level tool")
}

func TestFlattenResponsesNamespacesExcept_PreservesBuiltInNamespaceAndChoice(t *testing.T) {
	req := map[string]any{
		"tools": []any{
			map[string]any{"type": "namespace", "name": "image_gen", "tools": []any{
				map[string]any{"type": "function", "name": "imagegen"},
			}},
			map[string]any{"type": "namespace", "name": "collaboration", "tools": []any{
				map[string]any{"type": "function", "name": "spawn_agent"},
			}},
		},
		"tool_choice": map[string]any{"type": "namespace", "name": "image_gen"},
	}

	names, changed, err := FlattenResponsesNamespacesExcept(req, map[string]bool{"image_gen": true})
	require.NoError(t, err)
	require.True(t, changed)
	require.Contains(t, names, "collaboration__spawn_agent")
	tools, ok := req["tools"].([]any)
	require.True(t, ok)
	require.Len(t, tools, 2)
	preservedTool, ok := tools[0].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "namespace", preservedTool["type"])
	require.Equal(t, "image_gen", preservedTool["name"])
	flatTool, ok := tools[1].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "function", flatTool["type"])
	require.Equal(t, "collaboration__spawn_agent", flatTool["name"])
	require.Equal(t, map[string]any{"type": "namespace", "name": "image_gen"}, req["tool_choice"])
}

func TestFlattenResponsesNamespaces_RejectsNamespaceCollision(t *testing.T) {
	req := map[string]any{"tools": []any{
		map[string]any{"type": "namespace", "name": "a", "tools": []any{
			map[string]any{"type": "function", "name": "b__c"},
		}},
		map[string]any{"type": "namespace", "name": "a__b", "tools": []any{
			map[string]any{"type": "function", "name": "c"},
		}},
	}}

	_, _, err := FlattenResponsesNamespaces(req)
	require.ErrorContains(t, err, "both flatten")
}

func TestRestoreResponsesNamespaceCalls_RewritesOnlyFunctionCalls(t *testing.T) {
	payload := []byte(`{"type":"response.completed","response":{"output":[{"type":"function_call","name":"collaboration__spawn_agent","call_id":"call_1","arguments":"{}","extra":"keep"},{"type":"function_call","name":"plain","arguments":"{}"},{"type":"message","name":"collaboration__spawn_agent","content":"<tag>&value</tag>"}]}}`)
	names := map[string]ResponsesNamespaceName{
		"collaboration__spawn_agent": {Namespace: "collaboration", Name: "spawn_agent"},
	}

	got, changed, err := RestoreResponsesNamespaceCalls(payload, names)
	require.NoError(t, err)
	require.True(t, changed)
	require.JSONEq(t, `{"type":"response.completed","response":{"output":[{"type":"function_call","name":"spawn_agent","namespace":"collaboration","call_id":"call_1","arguments":"{}","extra":"keep"},{"type":"function_call","name":"plain","arguments":"{}"},{"type":"message","name":"collaboration__spawn_agent","content":"<tag>&value</tag>"}]}}`, string(got))
	require.Contains(t, string(got), "<tag>&value</tag>")
	require.NotContains(t, string(got), `\u003c`)
}

func TestRestoreResponsesNamespaceCalls_RewritesLifecycleItems(t *testing.T) {
	for _, eventType := range []string{"response.output_item.added", "response.output_item.done"} {
		t.Run(eventType, func(t *testing.T) {
			payload := []byte(`{"type":"` + eventType + `","item":{"type":"function_call","name":"collaboration__spawn_agent","arguments":"{}"}}`)
			got, changed, err := RestoreResponsesNamespaceCalls(payload, map[string]ResponsesNamespaceName{
				"collaboration__spawn_agent": {Namespace: "collaboration", Name: "spawn_agent"},
			})
			require.NoError(t, err)
			require.True(t, changed)
			require.JSONEq(t, `{"type":"`+eventType+`","item":{"type":"function_call","name":"spawn_agent","namespace":"collaboration","arguments":"{}"}}`, string(got))
		})
	}
}
