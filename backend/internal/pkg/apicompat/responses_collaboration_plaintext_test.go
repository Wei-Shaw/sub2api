package apicompat

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func collaborationPlaintextTool(name string) map[string]any {
	return map[string]any{
		"type": "function", "name": name,
		"description": "synthetic " + name,
		"parameters": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"message": map[string]any{"type": "string", "encrypted": true, "description": "synthetic note"},
				"extra":   map[string]any{"type": "string"},
			},
			"required": []any{"message"},
		},
	}
}

func requireCollaborationValue[T any](t *testing.T, value any) T {
	t.Helper()
	typed, ok := value.(T)
	require.True(t, ok)
	return typed
}

func TestAdaptResponsesCollaborationPlaintext_LowersMarkedToolsAndRewritesReferences(t *testing.T) {
	req := map[string]any{
		"model": "gpt-5.5",
		"tools": []any{
			map[string]any{"type": "function", "name": "plain", "description": "keep"},
			map[string]any{
				"type": "namespace", "name": "collaboration",
				"tools": []any{
					collaborationPlaintextTool("spawn_agent"),
					collaborationPlaintextTool("send_message"),
					collaborationPlaintextTool("followup_task"),
					map[string]any{"type": "function", "name": "wait", "parameters": map[string]any{"type": "object"}},
				},
			},
			map[string]any{
				"type": "namespace", "name": "gmail",
				"tools": []any{collaborationPlaintextTool("send_message")},
			},
		},
		"tool_choice": map[string]any{"type": "function", "name": "followup_task", "namespace": "collaboration"},
		"input": []any{
			map[string]any{"type": "function_call", "call_id": "call_1", "name": "send_message", "namespace": "collaboration", "arguments": `{"message":"synthetic-body"}`},
			map[string]any{"type": "function_call", "call_id": "call_2", "name": "wait", "namespace": "collaboration", "arguments": "{}"},
			map[string]any{"type": "message", "role": "user", "content": "synthetic hello"},
			map[string]any{"type": "reasoning", "encrypted_content": "synthetic-opaque", "summary": []any{}},
			map[string]any{"type": "agent_message", "content": []any{map[string]any{"type": "input_text", "text": "synthetic envelope"}}},
		},
	}

	mapping, changed, err := AdaptResponsesCollaborationPlaintext(req)
	require.NoError(t, err)
	require.True(t, changed)
	require.Equal(t, ResponsesNamespaceName{Namespace: "collaboration", Name: "spawn_agent"}, mapping.Calls["collaboration__spawn_agent"])
	require.Equal(t, ResponsesNamespaceName{Namespace: "collaboration", Name: "send_message"}, mapping.Calls["collaboration__send_message"])
	require.Equal(t, ResponsesNamespaceName{Namespace: "collaboration", Name: "followup_task"}, mapping.Calls["collaboration__followup_task"])
	require.Len(t, mapping.Calls, 3)

	tools := requireCollaborationValue[[]any](t, req["tools"])
	require.Len(t, tools, 6)
	plain := requireCollaborationValue[map[string]any](t, tools[0])
	require.Equal(t, "plain", plain["name"])

	namespace := requireCollaborationValue[map[string]any](t, tools[1])
	require.Equal(t, "namespace", namespace["type"])
	require.Equal(t, "collaboration", namespace["name"])
	children := requireCollaborationValue[[]any](t, namespace["tools"])
	require.Len(t, children, 1)
	require.Equal(t, "wait", requireCollaborationValue[map[string]any](t, children[0])["name"])

	for i, name := range []string{"collaboration__spawn_agent", "collaboration__send_message", "collaboration__followup_task"} {
		flat := requireCollaborationValue[map[string]any](t, tools[2+i])
		require.Equal(t, "function", flat["type"])
		require.Equal(t, name, flat["name"])
		require.Equal(t, "synthetic "+name[len("collaboration__"):], flat["description"])
		params := requireCollaborationValue[map[string]any](t, flat["parameters"])
		props := requireCollaborationValue[map[string]any](t, params["properties"])
		message := requireCollaborationValue[map[string]any](t, props["message"])
		require.Equal(t, "string", message["type"])
		require.Equal(t, "synthetic note", message["description"])
		require.NotContains(t, message, "encrypted")
		require.Contains(t, props, "extra")
	}

	gmail := requireCollaborationValue[map[string]any](t, tools[5])
	require.Equal(t, "namespace", gmail["type"])
	gmailChildren := requireCollaborationValue[[]any](t, gmail["tools"])
	require.Len(t, gmailChildren, 1)
	gmailChild := requireCollaborationValue[map[string]any](t, gmailChildren[0])
	require.Equal(t, "send_message", gmailChild["name"])
	gmailMessage := requireCollaborationValue[map[string]any](t,
		requireCollaborationValue[map[string]any](t,
			requireCollaborationValue[map[string]any](t, gmailChild["parameters"])["properties"])["message"])
	require.Equal(t, true, gmailMessage["encrypted"], "다른 namespace는 marker 포함 그대로 보존")

	choice := requireCollaborationValue[map[string]any](t, req["tool_choice"])
	require.Equal(t, "collaboration__followup_task", choice["name"])
	require.NotContains(t, choice, "namespace")

	input := requireCollaborationValue[[]any](t, req["input"])
	call := requireCollaborationValue[map[string]any](t, input[0])
	require.Equal(t, "collaboration__send_message", call["name"])
	require.NotContains(t, call, "namespace")
	require.Equal(t, `{"message":"synthetic-body"}`, call["arguments"], "arguments 본문은 그대로")
	controlCall := requireCollaborationValue[map[string]any](t, input[1])
	require.Equal(t, "wait", controlCall["name"])
	require.Equal(t, "collaboration", controlCall["namespace"], "control tool 호출은 namespaced 그대로")
	require.Equal(t, "synthetic hello", requireCollaborationValue[map[string]any](t, input[2])["content"])
	require.Equal(t, "synthetic-opaque", requireCollaborationValue[map[string]any](t, input[3])["encrypted_content"])
	require.Equal(t, "agent_message", requireCollaborationValue[map[string]any](t, input[4])["type"])

	payload := []byte(`{"id":"resp_1","output":[{"type":"function_call","name":"collaboration__spawn_agent","call_id":"c1","arguments":"{}"},{"type":"function_call","name":"collaboration__send_message","call_id":"c2","arguments":"{}"},{"type":"function_call","name":"collaboration__followup_task","call_id":"c3","arguments":"{}"},{"type":"function_call","name":"plain","call_id":"c4","arguments":"{}"}]}`)
	restored, restoredChanged, err := RestoreResponsesCollaborationPlaintextPayload(payload, mapping)
	require.NoError(t, err)
	require.True(t, restoredChanged)
	require.JSONEq(t, `{"id":"resp_1","output":[
		{"type":"function_call","name":"spawn_agent","namespace":"collaboration","call_id":"c1","arguments":"{}","encrypted_function_args":[]},
		{"type":"function_call","name":"send_message","namespace":"collaboration","call_id":"c2","arguments":"{}","encrypted_function_args":[]},
		{"type":"function_call","name":"followup_task","namespace":"collaboration","call_id":"c3","arguments":"{}","encrypted_function_args":[]},
		{"type":"function_call","name":"plain","call_id":"c4","arguments":"{}"}]}`, string(restored))
}

func TestAdaptResponsesCollaborationPlaintext_AdditionalToolsCarrier(t *testing.T) {
	carrier := map[string]any{
		"type": "additional_tools", "role": "developer",
		"tools": []any{
			map[string]any{
				"type": "namespace", "name": "collaboration",
				"tools": []any{collaborationPlaintextTool("send_message")},
			},
		},
	}
	req := map[string]any{
		"input": []any{
			carrier,
			map[string]any{"type": "function_call", "call_id": "c1", "name": "send_message", "namespace": "collaboration", "arguments": `{"message":"synthetic"}`},
		},
	}

	mapping, changed, err := AdaptResponsesCollaborationPlaintext(req)
	require.NoError(t, err)
	require.True(t, changed)
	require.Equal(t, ResponsesNamespaceName{Namespace: "collaboration", Name: "send_message"}, mapping.Calls["collaboration__send_message"])

	require.Equal(t, "additional_tools", carrier["type"])
	tools := requireCollaborationValue[[]any](t, carrier["tools"])
	require.Len(t, tools, 1, "namespace 자식이 전부 변환되면 빈 namespace 선언은 제거")
	flat := requireCollaborationValue[map[string]any](t, tools[0])
	require.Equal(t, "collaboration__send_message", flat["name"])

	call := requireCollaborationValue[map[string]any](t, requireCollaborationValue[[]any](t, req["input"])[1])
	require.Equal(t, "collaboration__send_message", call["name"])
	require.NotContains(t, call, "namespace")
}

func TestAdaptResponsesCollaborationPlaintext_PreservesUnmarkedAndEmptyInput(t *testing.T) {
	unmarked := map[string]any{
		"type": "function", "name": "send_message",
		"parameters": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"message": map[string]any{"type": "string"},
			},
		},
	}
	req := map[string]any{
		"tools": []any{map[string]any{
			"type": "namespace", "name": "collaboration",
			"tools": []any{unmarked, map[string]any{"type": "function", "name": "wait"}},
		}},
		"input": []any{
			map[string]any{"type": "function_call", "call_id": "c1", "name": "send_message", "namespace": "collaboration", "arguments": `{"message":"synthetic"}`},
		},
		"tool_choice": "required",
	}

	mapping, changed, err := AdaptResponsesCollaborationPlaintext(req)
	require.NoError(t, err)
	require.False(t, changed, "marker 없는 대상은 no-op")
	require.Empty(t, mapping.Calls)

	namespace := requireCollaborationValue[map[string]any](t, requireCollaborationValue[[]any](t, req["tools"])[0])
	require.Len(t, requireCollaborationValue[[]any](t, namespace["tools"]), 2)
	call := requireCollaborationValue[map[string]any](t, requireCollaborationValue[[]any](t, req["input"])[0])
	require.Equal(t, "send_message", call["name"])
	require.Equal(t, "collaboration", call["namespace"])
	require.Equal(t, "required", req["tool_choice"])

	empty := map[string]any{"model": "gpt-5.5"}
	mapping, changed, err = AdaptResponsesCollaborationPlaintext(empty)
	require.NoError(t, err)
	require.False(t, changed)
	require.Empty(t, mapping.Calls)

	mapping, changed, err = AdaptResponsesCollaborationPlaintext(nil)
	require.NoError(t, err)
	require.False(t, changed)
	require.Empty(t, mapping.Calls)
}

func TestAdaptResponsesCollaborationPlaintext_RejectsAliasConflicts(t *testing.T) {
	for _, blocker := range []map[string]any{
		{"type": "function", "name": "collaboration__send_message"},
		{"type": "custom", "name": "collaboration__send_message"},
	} {
		req := map[string]any{"tools": []any{
			blocker,
			map[string]any{"type": "namespace", "name": "collaboration", "tools": []any{collaborationPlaintextTool("send_message")}},
		}}
		_, _, err := AdaptResponsesCollaborationPlaintext(req)
		require.ErrorContains(t, err, "conflicts")
	}

	crossCarrier := map[string]any{
		"tools": []any{map[string]any{"type": "namespace", "name": "collaboration", "tools": []any{collaborationPlaintextTool("spawn_agent")}}},
		"input": []any{map[string]any{"type": "additional_tools", "tools": []any{
			map[string]any{"type": "function", "name": "collaboration__spawn_agent"},
		}}},
	}
	_, _, err := AdaptResponsesCollaborationPlaintext(crossCarrier)
	require.ErrorContains(t, err, "conflicts")
}

func TestAdaptResponsesCollaborationPlaintext_RejectsMalformedTargets(t *testing.T) {
	cases := map[string]map[string]any{
		"parameters not object": {
			"type": "function", "name": "send_message", "parameters": "synthetic-junk",
		},
		"encrypted non-string message": {
			"type": "function", "name": "send_message",
			"parameters": map[string]any{"type": "object", "properties": map[string]any{
				"message": map[string]any{"type": "object", "encrypted": true},
			}},
		},
		"non-boolean encrypted marker": {
			"type": "function", "name": "send_message",
			"parameters": map[string]any{"type": "object", "properties": map[string]any{
				"message": map[string]any{"type": "string", "encrypted": "yes"},
			}},
		},
	}
	for label, tool := range cases {
		t.Run(label, func(t *testing.T) {
			req := map[string]any{"tools": []any{map[string]any{
				"type": "namespace", "name": "collaboration", "tools": []any{tool},
			}}}
			_, _, err := AdaptResponsesCollaborationPlaintext(req)
			require.Error(t, err)
		})
	}
}

func TestAdaptResponsesCollaborationPlaintext_ChoiceErrorLeavesRequestUntouched(t *testing.T) {
	req := map[string]any{
		"tools": []any{map[string]any{
			"type": "namespace", "name": "collaboration",
			"tools": []any{collaborationPlaintextTool("send_message")},
		}},
		"tool_choice": map[string]any{"type": "allowed_tools", "mode": "auto", "tools": []any{
			map[string]any{"type": "function", "name": "send_message", "namespace": "collaboration"},
			map[string]any{"type": "namespace", "name": "collaboration"},
		}},
	}
	before, err := json.Marshal(req)
	require.NoError(t, err)

	_, changed, err := AdaptResponsesCollaborationPlaintext(req)
	require.Error(t, err)
	require.False(t, changed)

	after, err := json.Marshal(req)
	require.NoError(t, err)
	require.JSONEq(t, string(before), string(after), "오류 경로에서 req 전체가 불변이어야 함")
}

func TestAdaptResponsesCollaborationPlaintext_StringSchemaPreservedOnNoop(t *testing.T) {
	stringChild := map[string]any{
		"type": "function", "name": "send_message",
		"parameters": `{"type":"object","properties":{"message":{"type":"string"}},"big":9007199254740993}`,
	}
	req := map[string]any{"tools": []any{map[string]any{
		"type": "namespace", "name": "collaboration", "tools": []any{stringChild},
	}}}

	mapping, changed, err := AdaptResponsesCollaborationPlaintext(req)
	require.NoError(t, err)
	require.False(t, changed)
	require.Empty(t, mapping.Calls)
	require.Equal(t, `{"type":"object","properties":{"message":{"type":"string"}},"big":9007199254740993}`, stringChild["parameters"], "no-op 경로에서 string schema 원본 타입·내용 유지")
}

func TestAdaptResponsesCollaborationPlaintext_LateCarrierErrorLeavesRequestUntouched(t *testing.T) {
	markedStringChild := map[string]any{
		"type": "function", "name": "spawn_agent",
		"parameters": `{"type":"object","properties":{"message":{"type":"string","encrypted":true}}}`,
	}
	req := map[string]any{
		"tools": []any{map[string]any{
			"type": "namespace", "name": "collaboration", "tools": []any{markedStringChild},
		}},
		"input": []any{map[string]any{
			"type": "additional_tools",
			"tools": []any{map[string]any{
				"type": "namespace", "name": "collaboration",
				"tools": []any{map[string]any{"type": "function", "name": "send_message", "parameters": "synthetic-junk"}},
			}},
		}},
	}
	before, err := json.Marshal(req)
	require.NoError(t, err)

	_, _, err = AdaptResponsesCollaborationPlaintext(req)
	require.Error(t, err)

	after, err := json.Marshal(req)
	require.NoError(t, err)
	require.JSONEq(t, string(before), string(after), "뒤 carrier 오류 시 앞 carrier까지 포함해 req 불변")
	require.IsType(t, "", markedStringChild["parameters"])
}

func TestAdaptResponsesCollaborationPlaintext_StringSchemaConverted(t *testing.T) {
	child := map[string]any{
		"type": "function", "name": "send_message",
		"parameters": `{"type":"object","properties":{"message":{"type":"string","encrypted":true}},"big":9007199254740993}`,
	}
	req := map[string]any{"tools": []any{map[string]any{
		"type": "namespace", "name": "collaboration", "tools": []any{child},
	}}}

	mapping, changed, err := AdaptResponsesCollaborationPlaintext(req)
	require.NoError(t, err)
	require.True(t, changed)
	require.Contains(t, mapping.Calls, "collaboration__send_message")

	tools := requireCollaborationValue[[]any](t, req["tools"])
	require.Len(t, tools, 1, "자식 전부 변환 시 빈 namespace 제거")
	flat := requireCollaborationValue[map[string]any](t, tools[0])
	require.Equal(t, "collaboration__send_message", flat["name"])
	params := requireCollaborationValue[map[string]any](t, flat["parameters"])
	message := requireCollaborationValue[map[string]any](t,
		requireCollaborationValue[map[string]any](t, params["properties"])["message"])
	require.NotContains(t, message, "encrypted")
	encoded, err := json.Marshal(params)
	require.NoError(t, err)
	require.Contains(t, string(encoded), "9007199254740993", "변환된 string schema의 큰 정수 정밀도 보존")
}

func TestAdaptResponsesCollaborationPlaintext_ChildrenKeyNamespace(t *testing.T) {
	req := map[string]any{"tools": []any{map[string]any{
		"type": "namespace", "name": "collaboration",
		"children": []any{
			collaborationPlaintextTool("send_message"),
			map[string]any{"type": "function", "name": "wait"},
		},
	}}}

	mapping, changed, err := AdaptResponsesCollaborationPlaintext(req)
	require.NoError(t, err)
	require.True(t, changed)
	require.Contains(t, mapping.Calls, "collaboration__send_message")

	tools := requireCollaborationValue[[]any](t, req["tools"])
	require.Len(t, tools, 2)
	namespace := requireCollaborationValue[map[string]any](t, tools[0])
	require.Equal(t, "namespace", namespace["type"])
	children := requireCollaborationValue[[]any](t, namespace["children"])
	require.Len(t, children, 1)
	require.Equal(t, "wait", requireCollaborationValue[map[string]any](t, children[0])["name"])
	require.NotContains(t, namespace, "tools")
	flat := requireCollaborationValue[map[string]any](t, tools[1])
	require.Equal(t, "collaboration__send_message", flat["name"])
}

func TestAdaptResponsesCollaborationPlaintext_AmbiguousChildrenKeys(t *testing.T) {
	marked := map[string]any{"tools": []any{map[string]any{
		"type": "namespace", "name": "collaboration",
		"tools":    []any{collaborationPlaintextTool("send_message")},
		"children": []any{map[string]any{"type": "function", "name": "wait"}},
	}}}
	before, err := json.Marshal(marked)
	require.NoError(t, err)
	_, _, err = AdaptResponsesCollaborationPlaintext(marked)
	require.Error(t, err)
	after, err := json.Marshal(marked)
	require.NoError(t, err)
	require.JSONEq(t, string(before), string(after), "모호 선언 거부 시 req 불변")

	unmarkedChild := map[string]any{
		"type": "function", "name": "send_message",
		"parameters": map[string]any{"type": "object", "properties": map[string]any{
			"message": map[string]any{"type": "string"},
		}},
	}
	unmarked := map[string]any{"tools": []any{map[string]any{
		"type": "namespace", "name": "collaboration",
		"tools":    []any{unmarkedChild},
		"children": []any{map[string]any{"type": "function", "name": "wait"}},
	}}}
	before, err = json.Marshal(unmarked)
	require.NoError(t, err)
	mapping, changed, err := AdaptResponsesCollaborationPlaintext(unmarked)
	require.NoError(t, err)
	require.False(t, changed, "변환 대상이 없으면 모호 선언도 그대로 통과")
	require.Empty(t, mapping.Calls)
	after, err = json.Marshal(unmarked)
	require.NoError(t, err)
	require.JSONEq(t, string(before), string(after))
}

func TestAdaptResponsesCollaborationPlaintext_UnmarkedSiblingCarrierUntouched(t *testing.T) {
	unmarkedChild := map[string]any{
		"type": "function", "name": "send_message",
		"parameters": map[string]any{"type": "object", "properties": map[string]any{
			"message": map[string]any{"type": "string"},
		}},
	}
	carrierItem := map[string]any{
		"type": "additional_tools",
		"tools": []any{map[string]any{
			"type": "namespace", "name": "collaboration",
			"tools": []any{unmarkedChild},
		}},
	}
	carrierBefore, err := json.Marshal(carrierItem)
	require.NoError(t, err)
	req := map[string]any{
		"tools": []any{map[string]any{
			"type": "namespace", "name": "collaboration",
			"tools": []any{collaborationPlaintextTool("send_message")},
		}},
		"input": []any{carrierItem},
	}

	mapping, changed, err := AdaptResponsesCollaborationPlaintext(req)
	require.NoError(t, err)
	require.True(t, changed)
	require.Contains(t, mapping.Calls, "collaboration__send_message")

	carrierAfter, err := json.Marshal(carrierItem)
	require.NoError(t, err)
	require.JSONEq(t, string(carrierBefore), string(carrierAfter), "unmarked 동명 선언이 있는 carrier는 통째로 불변")
	require.Equal(t, "send_message", unmarkedChild["name"])
}

func TestAdaptResponsesCollaborationPlaintext_AmbiguousNamespaceSkippedWhileOtherConverts(t *testing.T) {
	ambiguous := map[string]any{
		"type": "namespace", "name": "collaboration",
		"tools": []any{map[string]any{
			"type": "function", "name": "spawn_agent",
			"parameters": map[string]any{"type": "object", "properties": map[string]any{
				"message": map[string]any{"type": "string"},
			}},
		}},
		"children": []any{map[string]any{"type": "function", "name": "wait"}},
	}
	ambiguousBefore, err := json.Marshal(ambiguous)
	require.NoError(t, err)
	req := map[string]any{"tools": []any{
		map[string]any{
			"type": "namespace", "name": "collaboration",
			"tools": []any{collaborationPlaintextTool("spawn_agent")},
		},
		ambiguous,
	}}

	mapping, changed, err := AdaptResponsesCollaborationPlaintext(req)
	require.NoError(t, err)
	require.True(t, changed)
	require.Equal(t, ResponsesNamespaceName{Namespace: "collaboration", Name: "spawn_agent"}, mapping.Calls["collaboration__spawn_agent"])
	require.Len(t, mapping.Calls, 1)

	ambiguousAfter, err := json.Marshal(ambiguous)
	require.NoError(t, err)
	require.JSONEq(t, string(ambiguousBefore), string(ambiguousAfter), "분류가 skip한 모호 namespace는 하강 단계도 건드리지 않음")
}

func TestAdaptResponsesCollaborationPlaintext_MarkedInBothCarriers(t *testing.T) {
	carrierItem := map[string]any{
		"type": "additional_tools",
		"tools": []any{map[string]any{
			"type": "namespace", "name": "collaboration",
			"tools": []any{collaborationPlaintextTool("send_message")},
		}},
	}
	req := map[string]any{
		"tools": []any{map[string]any{
			"type": "namespace", "name": "collaboration",
			"tools": []any{collaborationPlaintextTool("send_message")},
		}},
		"input": []any{carrierItem},
	}

	mapping, changed, err := AdaptResponsesCollaborationPlaintext(req)
	require.NoError(t, err)
	require.True(t, changed)
	require.Len(t, mapping.Calls, 1)

	topTools := requireCollaborationValue[[]any](t, req["tools"])
	require.Len(t, topTools, 1)
	require.Equal(t, "collaboration__send_message", requireCollaborationValue[map[string]any](t, topTools[0])["name"])
	carrierTools := requireCollaborationValue[[]any](t, carrierItem["tools"])
	require.Len(t, carrierTools, 1)
	require.Equal(t, "collaboration__send_message", requireCollaborationValue[map[string]any](t, carrierTools[0])["name"], "양쪽 carrier 각각 자체 선언이 변환")
}

func TestAdaptResponsesCollaborationPlaintext_ChoiceRewritesKnownShapesOnly(t *testing.T) {
	req := map[string]any{
		"tools": []any{map[string]any{
			"type": "namespace", "name": "collaboration",
			"tools": []any{collaborationPlaintextTool("send_message")},
		}},
		"tool_choice": map[string]any{"type": "allowed_tools", "mode": "auto", "tools": []any{
			map[string]any{"type": "function", "name": "send_message", "namespace": "collaboration"},
			map[string]any{"type": "mcp", "name": "send_message", "namespace": "collaboration"},
			map[string]any{"name": "send_message", "namespace": "collaboration"},
		}, "meta": map[string]any{"name": "send_message", "namespace": "collaboration"}},
	}

	_, changed, err := AdaptResponsesCollaborationPlaintext(req)
	require.NoError(t, err)
	require.True(t, changed)

	choice := requireCollaborationValue[map[string]any](t, req["tool_choice"])
	inner := requireCollaborationValue[[]any](t, choice["tools"])
	fn := requireCollaborationValue[map[string]any](t, inner[0])
	require.Equal(t, "collaboration__send_message", fn["name"])
	require.NotContains(t, fn, "namespace")
	mcp := requireCollaborationValue[map[string]any](t, inner[1])
	require.Equal(t, "send_message", mcp["name"])
	require.Equal(t, "collaboration", mcp["namespace"], "mcp 참조는 shape 불변")
	typeless := requireCollaborationValue[map[string]any](t, inner[2])
	require.Equal(t, "send_message", typeless["name"])
	require.Equal(t, "collaboration", typeless["namespace"], "type 없는 참조는 shape 불변")
	meta := requireCollaborationValue[map[string]any](t, choice["meta"])
	require.Equal(t, "send_message", meta["name"])
	require.Equal(t, "collaboration", meta["namespace"], "알 수 없는 필드 내부 name/namespace 불변")
	require.Equal(t, "auto", choice["mode"])
}

func TestAdaptResponsesCollaborationPlaintext_DuplicateTargetDeclarations(t *testing.T) {
	different := collaborationPlaintextTool("send_message")
	different["description"] = "synthetic different schema"
	conflict := map[string]any{"tools": []any{map[string]any{
		"type": "namespace", "name": "collaboration",
		"tools": []any{collaborationPlaintextTool("send_message"), different},
	}}}
	_, _, err := AdaptResponsesCollaborationPlaintext(conflict)
	require.ErrorContains(t, err, "different")

	identical := map[string]any{"tools": []any{map[string]any{
		"type": "namespace", "name": "collaboration",
		"tools": []any{collaborationPlaintextTool("send_message"), collaborationPlaintextTool("send_message")},
	}}}
	mapping, changed, err := AdaptResponsesCollaborationPlaintext(identical)
	require.NoError(t, err)
	require.True(t, changed)
	require.Len(t, mapping.Calls, 1)
	tools := requireCollaborationValue[[]any](t, identical["tools"])
	require.Len(t, tools, 1, "동일 선언 중복은 하나의 flat function으로 dedupe")
	require.Equal(t, "collaboration__send_message", requireCollaborationValue[map[string]any](t, tools[0])["name"])
}

func TestAdaptResponsesCollaborationPlaintext_NamespaceToolChoiceIsExplicitError(t *testing.T) {
	req := map[string]any{
		"tools": []any{map[string]any{
			"type": "namespace", "name": "collaboration", "tools": []any{
				collaborationPlaintextTool("send_message"),
				map[string]any{"type": "function", "name": "wait"},
			},
		}},
		"tool_choice": map[string]any{"type": "namespace", "name": "collaboration"},
	}

	_, _, err := AdaptResponsesCollaborationPlaintext(req)
	require.ErrorContains(t, err, "tool_choice")

	namespace := requireCollaborationValue[map[string]any](t, requireCollaborationValue[[]any](t, req["tools"])[0])
	require.Equal(t, "namespace", namespace["type"])
	require.Len(t, requireCollaborationValue[[]any](t, namespace["tools"]), 2, "오류 시 선언부를 부분 변환하지 않음")
}

func TestAdaptResponsesCollaborationPlaintext_NestedAllowedToolsChoice(t *testing.T) {
	req := map[string]any{
		"tools": []any{map[string]any{
			"type": "namespace", "name": "collaboration", "tools": []any{collaborationPlaintextTool("send_message")},
		}},
		"tool_choice": map[string]any{"type": "allowed_tools", "mode": "auto", "tools": []any{
			map[string]any{"type": "function", "name": "send_message", "namespace": "collaboration"},
		}},
	}

	mapping, changed, err := AdaptResponsesCollaborationPlaintext(req)
	require.NoError(t, err)
	require.True(t, changed)
	require.NotEmpty(t, mapping.Calls)

	choice := requireCollaborationValue[map[string]any](t, req["tool_choice"])
	inner := requireCollaborationValue[map[string]any](t, requireCollaborationValue[[]any](t, choice["tools"])[0])
	require.Equal(t, "collaboration__send_message", inner["name"])
	require.NotContains(t, inner, "namespace")
}

func TestRestoreResponsesCollaborationPlaintextPayload_StreamEvents(t *testing.T) {
	mapping := ResponsesCollaborationPlaintextMapping{Calls: map[string]ResponsesNamespaceName{
		"collaboration__send_message": {Namespace: "collaboration", Name: "send_message"},
	}}

	for _, eventType := range []string{"response.output_item.added", "response.output_item.done"} {
		t.Run(eventType, func(t *testing.T) {
			payload := []byte(`{"type":"` + eventType + `","sequence_number":3,"output_index":0,"item":{"type":"function_call","id":"fc_1","call_id":"c1","name":"collaboration__send_message","arguments":"{}"}}`)
			got, changed, err := RestoreResponsesCollaborationPlaintextPayload(payload, mapping)
			require.NoError(t, err)
			require.True(t, changed)
			require.JSONEq(t, `{"type":"`+eventType+`","sequence_number":3,"output_index":0,"item":{"type":"function_call","id":"fc_1","call_id":"c1","name":"send_message","namespace":"collaboration","arguments":"{}","encrypted_function_args":[]}}`, string(got))
		})
	}

	for _, eventType := range []string{"response.function_call_arguments.delta", "response.function_call_arguments.done"} {
		t.Run(eventType, func(t *testing.T) {
			payload := []byte(`{"type":"` + eventType + `","item_id":"fc_1","output_index":0,"name":"collaboration__send_message","delta":"{}"}`)
			got, changed, err := RestoreResponsesCollaborationPlaintextPayload(payload, mapping)
			require.NoError(t, err)
			require.True(t, changed)
			require.JSONEq(t, `{"type":"`+eventType+`","item_id":"fc_1","output_index":0,"name":"send_message","delta":"{}"}`, string(got))
		})
	}

	completed := []byte(`{"type":"response.completed","response":{"id":"resp_9","output":[{"type":"function_call","name":"collaboration__send_message","call_id":"c1","arguments":"{}"}],"unrelated":{"keep":1}}}`)
	got, changed, err := RestoreResponsesCollaborationPlaintextPayload(completed, mapping)
	require.NoError(t, err)
	require.True(t, changed)
	require.JSONEq(t, `{"type":"response.completed","response":{"id":"resp_9","output":[{"type":"function_call","name":"send_message","namespace":"collaboration","call_id":"c1","arguments":"{}","encrypted_function_args":[]}],"unrelated":{"keep":1}}}`, string(got))
}

func TestRestoreResponsesCollaborationPlaintextPayload_EchoedToolsAndChoice(t *testing.T) {
	mapping := ResponsesCollaborationPlaintextMapping{Calls: map[string]ResponsesNamespaceName{
		"collaboration__send_message": {Namespace: "collaboration", Name: "send_message"},
	}}

	payload := []byte(`{"id":"resp_1","tools":[{"type":"function","name":"plain"},{"type":"function","name":"collaboration__send_message","parameters":{"type":"object","properties":{"message":{"type":"string"}}}}],"tool_choice":{"type":"function","name":"collaboration__send_message"},"output":[]}`)
	got, changed, err := RestoreResponsesCollaborationPlaintextPayload(payload, mapping)
	require.NoError(t, err)
	require.True(t, changed)
	require.JSONEq(t, `{"id":"resp_1","tools":[{"type":"function","name":"plain"},{"type":"namespace","name":"collaboration","tools":[{"type":"function","name":"send_message","parameters":{"type":"object","properties":{"message":{"type":"string","encrypted":true}}}}]}],"tool_choice":{"type":"function","name":"send_message","namespace":"collaboration"},"output":[]}`, string(got))
}

func TestRestoreResponsesCollaborationPlaintextPayload_ChoiceRestoresKnownShapesOnly(t *testing.T) {
	mapping := ResponsesCollaborationPlaintextMapping{Calls: map[string]ResponsesNamespaceName{
		"collaboration__send_message": {Namespace: "collaboration", Name: "send_message"},
	}}

	payload := []byte(`{"tool_choice":{"type":"allowed_tools","mode":"auto","tools":[{"type":"function","name":"collaboration__send_message"},{"type":"mcp","name":"collaboration__send_message"}],"meta":{"name":"collaboration__send_message"}}}`)
	got, changed, err := RestoreResponsesCollaborationPlaintextPayload(payload, mapping)
	require.NoError(t, err)
	require.True(t, changed)
	require.JSONEq(t, `{"tool_choice":{"type":"allowed_tools","mode":"auto","tools":[{"type":"function","name":"send_message","namespace":"collaboration"},{"type":"mcp","name":"collaboration__send_message"}],"meta":{"name":"collaboration__send_message"}}}`, string(got),
		"function reference만 복원, 비관련 type·알 수 없는 필드 내부 name은 그대로")
}

func TestRestoreResponsesCollaborationPlaintextPayload_EchoedToolsMergeIntoExistingNamespace(t *testing.T) {
	mapping := ResponsesCollaborationPlaintextMapping{Calls: map[string]ResponsesNamespaceName{
		"collaboration__spawn_agent": {Namespace: "collaboration", Name: "spawn_agent"},
	}}

	payload := []byte(`{"type":"response.completed","response":{"tools":[{"type":"function","name":"collaboration__spawn_agent","parameters":{"type":"object","properties":{"message":{"type":"string"}}}},{"type":"namespace","name":"collaboration","tools":[{"type":"function","name":"wait"}]}]}}`)
	got, changed, err := RestoreResponsesCollaborationPlaintextPayload(payload, mapping)
	require.NoError(t, err)
	require.True(t, changed)
	require.JSONEq(t, `{"type":"response.completed","response":{"tools":[{"type":"namespace","name":"collaboration","tools":[{"type":"function","name":"wait"},{"type":"function","name":"spawn_agent","parameters":{"type":"object","properties":{"message":{"type":"string","encrypted":true}}}}]}]}}`, string(got))
}

func TestRestoreResponsesCollaborationPlaintextPayload_RejectsNonEmptyEncryptedArgs(t *testing.T) {
	mapping := ResponsesCollaborationPlaintextMapping{Calls: map[string]ResponsesNamespaceName{
		"collaboration__send_message": {Namespace: "collaboration", Name: "send_message"},
	}}

	payload := []byte(`{"output":[{"type":"function_call","name":"collaboration__send_message","call_id":"c1","arguments":"{}","encrypted_function_args":["synthetic-ciphertext-ref"]}]}`)
	got, changed, err := RestoreResponsesCollaborationPlaintextPayload(payload, mapping)
	require.Error(t, err)
	require.False(t, changed)
	require.Equal(t, payload, got, "오류 시 원본 payload를 그대로 반환")
	require.NotContains(t, err.Error(), "synthetic-ciphertext-ref", "오류에 ciphertext 노출 금지")

	malformed := []byte(`{"output":[{"type":"function_call","name":"collaboration__send_message","call_id":"c1","arguments":"{}","encrypted_function_args":"synthetic"}]}`)
	_, _, err = RestoreResponsesCollaborationPlaintextPayload(malformed, mapping)
	require.Error(t, err)

	nullable := []byte(`{"output":[{"type":"function_call","name":"collaboration__send_message","call_id":"c1","arguments":"{}","encrypted_function_args":null}]}`)
	got, changed, err = RestoreResponsesCollaborationPlaintextPayload(nullable, mapping)
	require.NoError(t, err)
	require.True(t, changed)
	require.JSONEq(t, `{"output":[{"type":"function_call","name":"send_message","namespace":"collaboration","call_id":"c1","arguments":"{}","encrypted_function_args":[]}]}`, string(got))

	unrelated := []byte(`{"output":[{"type":"function_call","name":"plain","call_id":"c1","arguments":"{}","encrypted_function_args":["synthetic-ref"]}]}`)
	got, changed, err = RestoreResponsesCollaborationPlaintextPayload(unrelated, mapping)
	require.NoError(t, err)
	require.False(t, changed)
	require.Equal(t, unrelated, got, "mapping 밖 호출의 encrypted metadata는 그대로")
}

func TestRestoreResponsesCollaborationPlaintextPayload_PreservesNumbersAndBytes(t *testing.T) {
	mapping := ResponsesCollaborationPlaintextMapping{Calls: map[string]ResponsesNamespaceName{
		"collaboration__spawn_agent": {Namespace: "collaboration", Name: "spawn_agent"},
	}}

	payload := []byte(`{"output":[{"type":"function_call","name":"collaboration__spawn_agent","call_id":"c1","arguments":"{}","seq":9007199254740993}]}`)
	got, changed, err := RestoreResponsesCollaborationPlaintextPayload(payload, mapping)
	require.NoError(t, err)
	require.True(t, changed)
	require.Contains(t, string(got), "9007199254740993", "큰 정수 정밀도 보존")

	noop := []byte(`{"output":[{"type":"function_call","name":"unknown_alias","call_id":"c1","arguments":"{}","seq":9007199254740993}],"note":"synthetic"}`)
	got, changed, err = RestoreResponsesCollaborationPlaintextPayload(noop, mapping)
	require.NoError(t, err)
	require.False(t, changed)
	require.Equal(t, noop, got, "변환 없으면 원래 []byte 그대로")

	other := ResponsesCollaborationPlaintextMapping{Calls: map[string]ResponsesNamespaceName{
		"collaboration__send_message": {Namespace: "collaboration", Name: "send_message"},
	}}
	got, changed, err = RestoreResponsesCollaborationPlaintextPayload(payload, other)
	require.NoError(t, err)
	require.False(t, changed)
	require.Equal(t, payload, got, "독립 mapping 간 누출 없음")

	got, changed, err = RestoreResponsesCollaborationPlaintextPayload(payload, ResponsesCollaborationPlaintextMapping{})
	require.NoError(t, err)
	require.False(t, changed)
	require.Equal(t, payload, got)
}

func TestAdaptResponsesCollaborationPlaintext_NamespaceObjectInsideUnknownWrapperPreserved(t *testing.T) {
	req := map[string]any{
		"tools": []any{
			map[string]any{
				"type": "namespace",
				"name": "collaboration",
				"tools": []any{
					collaborationPlaintextTool("send_message"),
				},
			},
		},
		"tool_choice": map[string]any{
			"type": "allowed_tools",
			"meta": map[string]any{
				"inner": map[string]any{"type": "namespace", "name": "collaboration"},
			},
			"decorations": []any{
				map[string]any{"type": "namespace", "name": "collaboration"},
			},
			"tools": []any{
				map[string]any{"type": "function", "namespace": "collaboration", "name": "send_message"},
			},
		},
	}
	mapping, changed, err := AdaptResponsesCollaborationPlaintext(req)
	require.NoError(t, err, "알 수 없는 위치의 namespace 모양 객체는 거부하지 않는다")
	require.True(t, changed)
	require.Equal(t, ResponsesNamespaceName{Namespace: "collaboration", Name: "send_message"}, mapping.Calls["collaboration__send_message"])

	choice := req["tool_choice"].(map[string]any)
	inner := choice["meta"].(map[string]any)["inner"].(map[string]any)
	require.Equal(t, map[string]any{"type": "namespace", "name": "collaboration"}, inner, "meta 안의 namespace 객체는 미접촉")
	deco := choice["decorations"].([]any)[0].(map[string]any)
	require.Equal(t, map[string]any{"type": "namespace", "name": "collaboration"}, deco, "unknown wrapper 아래 namespace 객체는 미접촉")
	ref := choice["tools"].([]any)[0].(map[string]any)
	require.Equal(t, "collaboration__send_message", ref["name"], "알려진 위치의 function 참조는 여전히 변환")
	require.Empty(t, ref["namespace"])
}
