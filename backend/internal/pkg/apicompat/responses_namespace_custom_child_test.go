package apicompat

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

// Codex CLI 0.147+ 把 exec（唯一能跑命令/读文件的编排工具，输入是自由文本
// JavaScript）声明为 namespace 内的 custom 子工具。摊平只放行 function 会把它静默
// 丢弃，模型于是收到一组碰不到文件系统的工具，表现为"没有提供文件读取或终端执行
// 工具"式的拒绝。
//
// 下列用例按真实请求形态钉死：exec 必须摊平、必须降级为带自由文本 schema 的
// function、回程必须还原成带 namespace 的 custom_tool_call。
func TestFlattenResponsesNamespaces_KeepsCustomChild(t *testing.T) {
	req := map[string]any{
		"model": "gpt-5.6-sol",
		"tools": []any{
			map[string]any{
				"type": "namespace",
				"name": "functions",
				"tools": []any{
					map[string]any{"type": "custom", "name": "exec", "description": "Run JavaScript code."},
					map[string]any{"type": "function", "name": "wait", "description": "Wait on a cell.", "parameters": map[string]any{"type": "object"}},
				},
			},
		},
	}

	names, changed, err := FlattenResponsesNamespaces(req)
	require.NoError(t, err)
	require.True(t, changed)
	require.Equal(t, ResponsesNamespaceName{Namespace: "functions", Name: "exec"}, names["functions__exec"])
	require.Equal(t, ResponsesNamespaceName{Namespace: "functions", Name: "wait"}, names["functions__wait"])

	tools, ok := req["tools"].([]any)
	require.True(t, ok)
	require.Len(t, tools, 2, "custom 子工具不得被丢弃")

	execTool, ok := tools[0].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "functions__exec", execTool["name"])
	require.Equal(t, "custom", execTool["type"], "摊平阶段必须保留 custom 类型，降级由 AdaptResponsesClientTools 统一处理")
}

// 摊平后的 custom 子工具必须被降级循环登记进 CustomTools，否则回程还原不出
// custom_tool_call。
func TestAdaptResponsesClientTools_LowersNamespacedCustomChild(t *testing.T) {
	req := map[string]any{
		"model": "gpt-5.6-sol",
		"tools": []any{
			map[string]any{
				"type": "namespace",
				"name": "functions",
				"tools": []any{
					map[string]any{"type": "custom", "name": "exec", "description": "Run JavaScript code."},
				},
			},
		},
	}

	mapping, changed, err := AdaptResponsesClientTools(req)
	require.NoError(t, err)
	require.True(t, changed)
	require.True(t, mapping.CustomTools["functions__exec"], "摊平名必须登记进 CustomTools")
	require.Equal(t, ResponsesNamespaceName{Namespace: "functions", Name: "exec"}, mapping.NamespaceTools["functions__exec"])

	tools, ok := req["tools"].([]any)
	require.True(t, ok)
	require.Len(t, tools, 1)
	execTool, ok := tools[0].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "function", execTool["type"], "上行给上游时必须已降级为 function")
	require.Equal(t, "functions__exec", execTool["name"])
	require.NotNil(t, execTool["parameters"], "降级后必须带自由文本 schema")
}

func TestAdaptResponsesClientTools_NormalizesUniqueBareNamespaceHistory(t *testing.T) {
	req := map[string]any{
		"model": "gpt-5.6-sol",
		"tools": []any{map[string]any{
			"type": "namespace", "name": "functions",
			"tools": []any{
				map[string]any{"type": "custom", "name": "exec"},
				map[string]any{"type": "function", "name": "wait", "parameters": map[string]any{"type": "object"}},
			},
		}},
		"input": []any{
			map[string]any{"type": "custom_tool_call", "name": "exec", "call_id": "custom_1", "input": "run()"},
			map[string]any{"type": "custom_tool_call_output", "call_id": "custom_1", "output": "ok"},
			map[string]any{"type": "function_call", "name": "exec", "call_id": "legacy_1", "arguments": "{\"input\":\"legacy()\"}"},
			map[string]any{"type": "function_call_output", "call_id": "legacy_1", "output": "legacy ok"},
			map[string]any{"type": "function_call", "namespace": "functions", "name": "wait", "call_id": "qualified_1", "arguments": "{}"},
			map[string]any{"type": "function_call", "name": "functions__wait", "call_id": "flat_1", "arguments": "{}"},
		},
	}

	mapping, changed, err := AdaptResponsesClientTools(req)
	require.NoError(t, err)
	require.True(t, changed)
	require.True(t, mapping.CustomTools["functions__exec"])

	input := req["input"].([]any)
	customCall := input[0].(map[string]any)
	require.Equal(t, "function_call", customCall["type"])
	require.Equal(t, "functions__exec", customCall["name"])
	require.JSONEq(t, "{\"input\":\"run()\"}", customCall["arguments"].(string))

	customOutput := input[1].(map[string]any)
	require.Equal(t, "function_call_output", customOutput["type"])
	require.Equal(t, "custom_1", customOutput["call_id"])
	require.Equal(t, "ok", customOutput["output"])

	legacyCall := input[2].(map[string]any)
	require.Equal(t, "function_call", legacyCall["type"])
	require.Equal(t, "functions__exec", legacyCall["name"])
	legacyOutput := input[3].(map[string]any)
	require.Equal(t, "legacy_1", legacyOutput["call_id"])
	require.Equal(t, "legacy ok", legacyOutput["output"])

	qualifiedCall := input[4].(map[string]any)
	require.Equal(t, "functions__wait", qualifiedCall["name"])
	require.NotContains(t, qualifiedCall, "namespace")
	require.Equal(t, "functions__wait", input[5].(map[string]any)["name"])
}

func TestFlattenResponsesNamespaces_DoesNotInferAmbiguousBareHistory(t *testing.T) {
	tests := []struct {
		name  string
		tools []any
	}{
		{
			name: "top-level tool owns bare name",
			tools: []any{
				map[string]any{"type": "function", "name": "exec", "parameters": map[string]any{"type": "object"}},
				map[string]any{"type": "namespace", "name": "functions", "tools": []any{map[string]any{"type": "custom", "name": "exec"}}},
			},
		},
		{
			name: "two namespaces own bare name",
			tools: []any{
				map[string]any{"type": "namespace", "name": "functions", "tools": []any{map[string]any{"type": "custom", "name": "exec"}}},
				map[string]any{"type": "namespace", "name": "other", "tools": []any{map[string]any{"type": "function", "name": "exec"}}},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := map[string]any{
				"tools": tt.tools,
				"input": []any{map[string]any{"type": "function_call", "name": "exec", "call_id": "call_1", "arguments": "{}"}},
			}

			_, changed, err := FlattenResponsesNamespaces(req)
			require.NoError(t, err)
			require.True(t, changed)
			call := req["input"].([]any)[0].(map[string]any)
			require.Equal(t, "exec", call["name"])
			require.NotContains(t, call, "namespace")
		})
	}
}

// 非流式回程：上游按摊平名回 function_call，客户端必须收到
// custom_tool_call + namespace=functions + name=exec。
func TestRestoreResponsesClientToolPayload_NamespacedCustomChild(t *testing.T) {
	mapping := ResponsesClientToolMapping{
		CustomTools:    map[string]bool{"functions__exec": true},
		NamespaceTools: map[string]ResponsesNamespaceName{"functions__exec": {Namespace: "functions", Name: "exec"}},
	}
	payload := []byte(`{"type":"response.completed","response":{"output":[{"type":"function_call","id":"fc_1","call_id":"call_1","name":"functions__exec","arguments":"{\"input\":\"await tools.exec_command({cmd:'ls'})\"}"}]}}`)

	restored, changed, err := RestoreResponsesClientToolPayload(payload, mapping)
	require.NoError(t, err)
	require.True(t, changed)

	var got struct {
		Response struct {
			Output []struct {
				Type      string `json:"type"`
				Name      string `json:"name"`
				Namespace string `json:"namespace"`
				Input     string `json:"input"`
				Arguments string `json:"arguments"`
			} `json:"output"`
		} `json:"response"`
	}
	require.NoError(t, json.Unmarshal(restored, &got))
	require.Len(t, got.Response.Output, 1)
	item := got.Response.Output[0]
	require.Equal(t, "custom_tool_call", item.Type)
	require.Equal(t, "exec", item.Name, "必须还原为 namespace 子工具名，不能留摊平名")
	require.Equal(t, "functions", item.Namespace)
	require.Equal(t, "await tools.exec_command({cmd:'ls'})", item.Input)
	require.Empty(t, item.Arguments)
}

// 流式回程：output_item 与合成的 custom_tool_call_input.done 都必须带回
// namespace 身份。
func TestResponsesClientToolStreamRestorer_NamespacedCustomChild(t *testing.T) {
	mapping := ResponsesClientToolMapping{
		CustomTools:    map[string]bool{"functions__exec": true},
		NamespaceTools: map[string]ResponsesNamespaceName{"functions__exec": {Namespace: "functions", Name: "exec"}},
	}
	restorer := NewResponsesClientToolStreamRestorer(mapping)

	added := restorer.Restore(ResponsesStreamEvent{
		Type: "response.output_item.added", OutputIndex: 0, SequenceNumber: 1,
		Item: &ResponsesOutput{Type: "function_call", ID: "fc_1", CallID: "call_1", Name: "functions__exec"},
	})
	require.Len(t, added, 1)
	require.Equal(t, "custom_tool_call", added[0].Item.Type)
	require.Equal(t, "exec", added[0].Item.Name)
	require.Equal(t, "functions", added[0].Item.Namespace)

	require.Empty(t, restorer.Restore(ResponsesStreamEvent{
		Type: "response.function_call_arguments.delta", OutputIndex: 0, ItemID: "fc_1",
		Delta: `{"input":"ls -la"}`,
	}), "function 参数增量应被吞掉，改由 custom_tool_call_input 事件表达")

	done := restorer.Restore(ResponsesStreamEvent{
		Type: "response.function_call_arguments.done", OutputIndex: 0, ItemID: "fc_1", Name: "functions__exec",
	})
	require.NotEmpty(t, done)
	last := done[len(done)-1]
	require.Equal(t, "response.custom_tool_call_input.done", last.Type)
	require.Equal(t, "exec", last.Name, "合成事件同样必须还原 namespace 子工具名")
	require.Equal(t, "ls -la", last.Input)
}

// Anthropic 桥路径（Kiro 等）的状态机会自己把 custom 工具的 item 产出为
// custom_tool_call 并直接发 custom_tool_call_input.*，不经过还原器的降级还原。
// 这些事件仍带摊平名，必须放行进还原器修正 namespace 身份。
//
// 用 RestoreEvent（而非 Restore）入口，才能覆盖 clientToolLifecycleEvent 与
// clientToolEventPayload 两处门控——回归前它们会把这些事件原样放过。
func TestResponsesClientToolStreamRestorer_BridgeEmittedCustomToolCall(t *testing.T) {
	mapping := ResponsesClientToolMapping{
		CustomTools:    map[string]bool{"functions__exec": true},
		NamespaceTools: map[string]ResponsesNamespaceName{"functions__exec": {Namespace: "functions", Name: "exec"}},
	}
	restorer := NewResponsesClientToolStreamRestorer(mapping)

	itemPayloads, _, err := restorer.RestoreEvent([]byte(
		`{"type":"response.output_item.done","sequence_number":1,"output_index":0,` +
			`"item":{"type":"custom_tool_call","id":"fc_1","call_id":"call_1","name":"functions__exec","input":"ls -la"}}`))
	require.NoError(t, err)
	require.Len(t, itemPayloads, 1)

	var item struct {
		Item struct {
			Type      string `json:"type"`
			Name      string `json:"name"`
			Namespace string `json:"namespace"`
			Input     string `json:"input"`
		} `json:"item"`
	}
	require.NoError(t, json.Unmarshal(itemPayloads[0], &item))
	require.Equal(t, "custom_tool_call", item.Item.Type)
	require.Equal(t, "exec", item.Item.Name, "桥路径产出的 item 必须还原 namespace 子工具名")
	require.Equal(t, "functions", item.Item.Namespace)
	require.Equal(t, "ls -la", item.Item.Input)

	inputPayloads, _, err := restorer.RestoreEvent([]byte(
		`{"type":"response.custom_tool_call_input.done","sequence_number":2,"output_index":0,` +
			`"item_id":"fc_1","call_id":"call_1","name":"functions__exec","input":"ls -la"}`))
	require.NoError(t, err)
	require.Len(t, inputPayloads, 1)

	var inputEvent struct {
		Name string `json:"name"`
	}
	require.NoError(t, json.Unmarshal(inputPayloads[0], &inputEvent))
	require.Equal(t, "exec", inputEvent.Name, "input.done 必须还原 namespace 子工具名")
}
