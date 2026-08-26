package apicompat

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func requireObjectInputSchema(t *testing.T, schema json.RawMessage) map[string]json.RawMessage {
	t.Helper()

	require.NotEmpty(t, schema)

	var parsed map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(schema, &parsed))
	require.JSONEq(t, `"object"`, string(parsed["type"]))
	require.Contains(t, parsed, "properties")

	var properties map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(parsed["properties"], &properties))

	return parsed
}

func TestResponsesToAnthropicRequest_AdditionalToolsItem(t *testing.T) {
	req := &ResponsesRequest{
		Model: "gpt-test",
		Input: json.RawMessage(`[
			{"type":"additional_tools","role":"developer","tools":[
				{"type":"custom","name":"exec","description":"Run shell commands","format":{"type":"text"}},
				{"type":"function","name":"wait","parameters":{"type":"object","properties":{}}}
			]},
			{"type":"message","role":"user","content":[{"type":"input_text","text":"write a file"}]}
		]`),
		ToolChoice: json.RawMessage(`{"type":"custom","name":"exec"}`),
	}

	out, err := ResponsesToAnthropicRequest(req)
	require.NoError(t, err)
	require.Len(t, out.Tools, 2)
	assert.Equal(t, "exec", out.Tools[0].Name)
	assert.Equal(t, "Run shell commands", out.Tools[0].Description)
	assert.JSONEq(t, customToolInputSchema, string(out.Tools[0].InputSchema))
	assert.Equal(t, "wait", out.Tools[1].Name)
	assert.JSONEq(t, `{"type":"tool","name":"exec"}`, string(out.ToolChoice))
}

func TestResponsesToAnthropic_CustomToolCallContinuation(t *testing.T) {
	req := &ResponsesRequest{
		Model: "gpt-test",
		Input: json.RawMessage(`[
			{"type":"custom_tool_call","call_id":"toolu_exec","name":"exec","input":"python --version"},
			{"type":"custom_tool_call_output","call_id":"toolu_exec","output":"Python 3.12.0"}
		]`),
	}

	out, err := ResponsesToAnthropicRequest(req)
	require.NoError(t, err)
	require.Len(t, out.Messages, 2)

	require.Equal(t, "assistant", out.Messages[0].Role)
	var toolUse []AnthropicContentBlock
	require.NoError(t, json.Unmarshal(out.Messages[0].Content, &toolUse))
	require.Len(t, toolUse, 1)
	assert.Equal(t, "tool_use", toolUse[0].Type)
	assert.Equal(t, "toolu_exec", toolUse[0].ID)
	assert.Equal(t, "exec", toolUse[0].Name)
	assert.JSONEq(t, `{"input":"python --version"}`, string(toolUse[0].Input))

	require.Equal(t, "user", out.Messages[1].Role)
	var toolResult []AnthropicContentBlock
	require.NoError(t, json.Unmarshal(out.Messages[1].Content, &toolResult))
	require.Len(t, toolResult, 1)
	assert.Equal(t, "tool_result", toolResult[0].Type)
	assert.Equal(t, "toolu_exec", toolResult[0].ToolUseID)
	assert.JSONEq(t, `"Python 3.12.0"`, string(toolResult[0].Content))
}

func TestResponsesToAnthropic_CustomToolCallOutputAllowsArrayOutput(t *testing.T) {
	req := &ResponsesRequest{
		Model: "gpt-test",
		Input: json.RawMessage(`[
			{"type":"custom_tool_call","call_id":"call_exec","name":"exec","input":"pwd"},
			{"type":"custom_tool_call_output","call_id":"call_exec","output":[{"type":"text","text":"/tmp"}]}
		]`),
	}

	out, err := ResponsesToAnthropicRequest(req)
	require.NoError(t, err)
	require.Len(t, out.Messages, 2)

	var toolResult []AnthropicContentBlock
	require.NoError(t, json.Unmarshal(out.Messages[1].Content, &toolResult))
	require.Len(t, toolResult, 1)
	assert.Equal(t, "tool_result", toolResult[0].Type)
	assert.Equal(t, "call_exec", toolResult[0].ToolUseID)
	assert.JSONEq(t, `[{"type":"text","text":"/tmp"}]`, string(toolResult[0].Content))
}

func TestResponsesToAnthropic_CustomGrammarToolUsesObjectSchema(t *testing.T) {
	body := []byte(`{
		"model": "gpt-5.2",
		"input": "apply this patch",
		"tools": [{
			"type": "custom",
			"name": "apply_patch",
			"description": "Apply a patch to the working tree",
			"format": {
				"type": "grammar",
				"syntax": "lark",
				"definition": "start: /.+/"
			}
		}]
	}`)

	var req ResponsesRequest
	require.NoError(t, json.Unmarshal(body, &req))

	anthropicReq, err := ResponsesToAnthropicRequest(&req)
	require.NoError(t, err)
	require.Len(t, anthropicReq.Tools, 1)

	tool := anthropicReq.Tools[0]
	assert.Empty(t, tool.Type)
	assert.Equal(t, "apply_patch", tool.Name)
	assert.Equal(t, "Apply a patch to the working tree", tool.Description)
	requireObjectInputSchema(t, tool.InputSchema)
	assert.JSONEq(t, customToolInputSchema, string(tool.InputSchema))

	wire, err := json.Marshal(tool)
	require.NoError(t, err)
	assert.NotContains(t, string(wire), `"type":"custom"`)
	assert.NotContains(t, string(wire), `"format"`)
	assert.NotContains(t, string(wire), `"grammar"`)
}

func TestResponsesToAnthropic_CustomToolPreservesSchemaParameters(t *testing.T) {
	tools := convertResponsesToAnthropicTools([]ResponsesTool{{
		Type:        "custom",
		Name:        "edit_file",
		Description: "Edit a file",
		Parameters:  json.RawMessage(`{"type":"object","properties":{"patch":{"type":"string"}},"required":["patch"]}`),
	}})

	require.Len(t, tools, 1)
	assert.Empty(t, tools[0].Type)
	assert.Equal(t, "edit_file", tools[0].Name)

	schema := requireObjectInputSchema(t, tools[0].InputSchema)
	assert.JSONEq(t, `{"patch":{"type":"string"}}`, string(schema["properties"]))
	assert.JSONEq(t, `["patch"]`, string(schema["required"]))
}

func TestResponsesToAnthropic_FunctionToolSchemaUnchanged(t *testing.T) {
	parameters := json.RawMessage(`{"type":"object","properties":{"city":{"type":"string"}},"required":["city"]}`)
	tools := convertResponsesToAnthropicTools([]ResponsesTool{{
		Type:        "function",
		Name:        "get_weather",
		Description: "Get weather",
		Parameters:  parameters,
	}})

	require.Len(t, tools, 1)
	assert.Empty(t, tools[0].Type)
	assert.Equal(t, "get_weather", tools[0].Name)
	assert.Equal(t, "Get weather", tools[0].Description)
	assert.JSONEq(t, string(parameters), string(tools[0].InputSchema))
}

func TestResponsesToAnthropic_MixedToolsProduceValidAnthropicTools(t *testing.T) {
	tools := convertResponsesToAnthropicTools([]ResponsesTool{
		{
			Type:       "function",
			Name:       "read_file",
			Parameters: json.RawMessage(`{"type":"object","properties":{"path":{"type":"string"}}}`),
		},
		{
			Type: "custom",
			Name: "apply_patch",
		},
		{
			Type: "web_search",
		},
	})

	require.Len(t, tools, 3)
	assert.Empty(t, tools[0].Type)
	assert.Equal(t, "read_file", tools[0].Name)
	requireObjectInputSchema(t, tools[0].InputSchema)

	assert.Empty(t, tools[1].Type)
	assert.Equal(t, "apply_patch", tools[1].Name)
	assert.JSONEq(t, customToolInputSchema, string(tools[1].InputSchema))

	assert.Equal(t, "web_search_20250305", tools[2].Type)
	assert.Equal(t, "web_search", tools[2].Name)
	assert.Empty(t, tools[2].InputSchema)
	serverToolWire, err := json.Marshal(tools[2])
	require.NoError(t, err)
	assert.NotContains(t, string(serverToolWire), `"input_schema"`)
}

func TestResponsesToAnthropic_DefaultToolNormalizesInputSchema(t *testing.T) {
	tools := convertResponsesToAnthropicTools([]ResponsesTool{{
		Type: "local_shell",
		Name: "shell",
	}})

	require.Len(t, tools, 1)
	assert.Equal(t, "local_shell", tools[0].Type)
	assert.Equal(t, "shell", tools[0].Name)
	assert.JSONEq(t, `{"type":"object","properties":{}}`, string(tools[0].InputSchema))
}
