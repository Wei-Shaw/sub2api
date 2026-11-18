package apicompat

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// 端到端模拟：Codex 0.164 Responses Lite 请求（exec 在 functions namespace 内）
// -> 转 Chat 请求 -> 模拟 DeepSeek 返回 functions__exec -> 还原 Responses 响应
// -> 下一轮历史回放。校验 Codex 能按 namespace+name 路由该 custom 调用。
func TestE2E_CodexNamespaceCustomExecThroughChatBridge(t *testing.T) {
	codexRequest := `{
	  "model":"gpt-6-sol","stream":true,"parallel_tool_calls":false,
	  "input":[
	    {"type":"additional_tools","role":"developer","tools":[
	      {"type":"namespace","name":"functions","tools":[
	        {"type":"custom","name":"exec","description":"Run raw JavaScript in a V8 isolate.","format":{"type":"grammar","syntax":"lark","definition":"start: SOURCE"}},
	        {"type":"function","name":"wait","description":"Wait on a yielded exec cell.","parameters":{"type":"object","properties":{"cell_id":{"type":"string"}},"required":["cell_id"]}},
	        {"type":"function","name":"request_user_input","description":"Ask the user.","parameters":{"type":"object","properties":{"prompt":{"type":"string"}}}}
	      ]}
	    ]},
	    {"type":"message","role":"user","content":[{"type":"input_text","text":"run pwd"}]}
	  ]
	}`
	var req ResponsesRequest
	require.NoError(t, json.Unmarshal([]byte(codexRequest), &req))

	effective, err := EffectiveResponsesTools(&req)
	require.NoError(t, err)
	chatReq, err := ResponsesToChatCompletionsRequest(&req)
	require.NoError(t, err)

	names := make([]string, 0, len(chatReq.Tools))
	for _, tool := range chatReq.Tools {
		names = append(names, tool.Function.Name)
	}
	assert.Equal(t, []string{"functions__exec", "functions__wait", "functions__request_user_input"}, names,
		"exec 必须作为可调用工具发给上游，不能丢")
	assert.JSONEq(t, customToolInputSchema, string(chatReq.Tools[0].Function.Parameters))

	namespaceTools := NamespaceToolNames(effective)
	require.Len(t, namespaceTools, 3)

	upstream := `{"id":"chatcmpl-1","object":"chat.completion","created":1,"model":"deepseek-v4.1-flash",
	  "choices":[{"index":0,"finish_reason":"tool_calls","message":{"role":"assistant","content":null,
	    "tool_calls":[{"id":"call_exec_1","type":"function","function":{"name":"functions__exec",
	      "arguments":"{\"input\":\"const r = await tools.exec_command({cmd: \\\"pwd\\\"}); text(r.output);\"}"}}]}}],
	  "usage":{"prompt_tokens":100,"completion_tokens":20,"total_tokens":120}}`
	var chatResp ChatCompletionsResponse
	require.NoError(t, json.Unmarshal([]byte(upstream), &chatResp))

	resp := ChatCompletionsResponseToResponses(&chatResp, req.Model,
		CustomToolNames(effective), FunctionToolNames(effective), HasToolSearchTool(effective), namespaceTools)

	require.Len(t, resp.Output, 1)
	item := resp.Output[0]
	assert.Equal(t, "custom_tool_call", item.Type, "必须还原成 custom_tool_call，Codex 才走 freeform handler")
	assert.Equal(t, "functions", item.Namespace)
	assert.Equal(t, "exec", item.Name)
	assert.Contains(t, item.Input, "exec_command")

	wire, err := json.Marshal(item)
	require.NoError(t, err)
	assert.Contains(t, string(wire), `"namespace":"functions"`)

	// 真实 Codex 每轮都会重发 additional_tools 声明；历史与会话工具表必须同时存在。
	history := `[
	  {"type":"additional_tools","role":"developer","tools":[
	    {"type":"namespace","name":"functions","tools":[
	      {"type":"custom","name":"exec","description":"Run raw JavaScript in a V8 isolate.","format":{"type":"grammar","syntax":"lark","definition":"start: SOURCE"}},
	      {"type":"function","name":"wait","parameters":{"type":"object","properties":{"cell_id":{"type":"string"}},"required":["cell_id"]}}
	    ]}
	  ]},
	  {"type":"custom_tool_call","call_id":"call_exec_1","namespace":"functions","name":"exec","input":"pwd"},
	  {"type":"custom_tool_call_output","call_id":"call_exec_1","output":"/workspace"}
	]`
	req.Input = json.RawMessage(history)
	nextReq, err := ResponsesToChatCompletionsRequest(&req)
	require.NoError(t, err)
	var assistant, toolMsg *ChatMessage
	for i := range nextReq.Messages {
		if nextReq.Messages[i].Role == "assistant" && len(nextReq.Messages[i].ToolCalls) > 0 {
			assistant = &nextReq.Messages[i]
		}
		if nextReq.Messages[i].Role == "tool" {
			toolMsg = &nextReq.Messages[i]
		}
	}
	require.NotNil(t, assistant)
	require.NotNil(t, toolMsg)
	assert.Equal(t, "functions__exec", assistant.ToolCalls[0].Function.Name, "历史必须使用与当前工具表一致的摊平名")
	assert.JSONEq(t, `{"input":"pwd"}`, assistant.ToolCalls[0].Function.Arguments)
	assert.Equal(t, "call_exec_1", toolMsg.ToolCallID)
}
