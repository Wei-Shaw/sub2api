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
	assert.JSONEq(t, `{"type":"object","properties":{}}`, string(tool.InputSchema))

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
	assert.JSONEq(t, `{"type":"object","properties":{}}`, string(tools[1].InputSchema))

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

// Codex 的 codex_app 命名空间工具（如 automation_update）把 parameters 根节点声明为
// 对象分支的 oneOf/anyOf；Anthropic 拒绝 input_schema 顶层的 oneOf/anyOf/allOf，
// 转换时必须摊平成单个 object schema。
func TestResponsesToAnthropic_ObjectUnionRootFlattened(t *testing.T) {
	tools := convertResponsesToAnthropicTools([]ResponsesTool{{
		Type: "function",
		Name: "codex_app__automation_update",
		Parameters: json.RawMessage(`{
			"oneOf": [
				{"type":"object","properties":{"id":{"type":"string"}}},
				{"anyOf":[{"type":"object"},{"type":"object","properties":{}}]}
			]
		}`),
	}})

	require.Len(t, tools, 1)
	schema := requireObjectInputSchema(t, tools[0].InputSchema)
	assert.NotContains(t, schema, "oneOf")
	assert.NotContains(t, schema, "anyOf")
	assert.NotContains(t, schema, "allOf")
	assert.JSONEq(t, `{"id":{"type":"string"}}`, string(schema["properties"]))
	assert.NotContains(t, schema, "required")
}

func TestResponsesToAnthropic_ObjectUnionRootMergesBranches(t *testing.T) {
	tools := convertResponsesToAnthropicTools([]ResponsesTool{{
		Type: "function",
		Name: "codex_app__automation_update",
		Parameters: json.RawMessage(`{
			"anyOf": [
				{"type":"object","properties":{"mode":{"enum":["view"]},"id":{"type":"string"}},"required":["mode","id"]},
				{"type":"object","properties":{"mode":{"enum":["update"]},"prompt":{"type":"string"}},"required":["mode","prompt"]}
			]
		}`),
	}})

	require.Len(t, tools, 1)
	schema := requireObjectInputSchema(t, tools[0].InputSchema)
	assert.NotContains(t, schema, "anyOf")

	// 属性取各分支并集；同名属性保留两侧约束（嵌套联合本身是允许的）。
	var properties map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(schema["properties"], &properties))
	require.Contains(t, properties, "id")
	require.Contains(t, properties, "prompt")
	var mode map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(properties["mode"], &mode))
	assert.Contains(t, mode, "anyOf")

	// required 取各分支交集，只保留所有分支都要求的字段。
	assert.JSONEq(t, `["mode"]`, string(schema["required"]))
}

func TestResponsesToAnthropic_AllOfRootKeepsUnionRequired(t *testing.T) {
	tools := convertResponsesToAnthropicTools([]ResponsesTool{{
		Type: "function",
		Name: "strict_tool",
		Parameters: json.RawMessage(`{
			"allOf": [
				{"type":"object","properties":{"path":{"type":"string"}},"required":["path"]},
				{"type":"object","properties":{"encoding":{"type":"string"}},"required":["encoding"]}
			]
		}`),
	}})

	require.Len(t, tools, 1)
	schema := requireObjectInputSchema(t, tools[0].InputSchema)
	assert.NotContains(t, schema, "allOf")
	assert.JSONEq(t, `["path","encoding"]`, string(schema["required"]))
}

// 非对象分支无法用 Anthropic 的 object schema 表达，直接丢弃分支但保留属性词汇。
func TestResponsesToAnthropic_RootUnionDropsNonObjectBranches(t *testing.T) {
	tools := convertResponsesToAnthropicTools([]ResponsesTool{{
		Type:       "function",
		Name:       "mixed_tool",
		Parameters: json.RawMessage(`{"oneOf":[{"type":"object","properties":{"path":{"type":"string"}}},{"type":"string"}]}`),
	}})

	require.Len(t, tools, 1)
	schema := requireObjectInputSchema(t, tools[0].InputSchema)
	assert.NotContains(t, schema, "oneOf")
	var properties map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(schema["properties"], &properties))
	assert.Contains(t, properties, "path")
}

func TestResponsesToAnthropic_NestedUnionPreserved(t *testing.T) {
	parameters := json.RawMessage(`{"type":"object","properties":{"value":{"anyOf":[{"type":"string"},{"type":"array","items":{"type":"string"}}]}}}`)
	tools := convertResponsesToAnthropicTools([]ResponsesTool{{
		Type:       "function",
		Name:       "get_value",
		Parameters: parameters,
	}})

	require.Len(t, tools, 1)
	assert.JSONEq(t, string(parameters), string(tools[0].InputSchema))
}

// 根节点自带 properties/required 时，与分支约束合并而不是被覆盖。
func TestResponsesToAnthropic_RootUnionKeepsRootPropertiesAndRequired(t *testing.T) {
	tools := convertResponsesToAnthropicTools([]ResponsesTool{{
		Type: "function",
		Name: "merge_root_tool",
		Parameters: json.RawMessage(`{
			"type":"object",
			"properties":{"shared":{"type":"string"}},
			"required":["shared"],
			"oneOf":[{"type":"object","properties":{"extra":{"type":"integer"}}}]
		}`),
	}})

	require.Len(t, tools, 1)
	schema := requireObjectInputSchema(t, tools[0].InputSchema)
	assert.NotContains(t, schema, "oneOf")
	var properties map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(schema["properties"], &properties))
	assert.Contains(t, properties, "shared")
	assert.Contains(t, properties, "extra")
	assert.JSONEq(t, `["shared"]`, string(schema["required"]))
}

// 端到端：Codex 命名空间工具经降级/摊平后，最终 Anthropic 工具 schema 顶部不再有联合关键字。
func TestResponsesToAnthropic_CodexNamespaceRootUnionTool(t *testing.T) {
	body := []byte(`{
		"model": "claude-opus-5",
		"input": "create an automation",
		"tools": [{
			"type": "namespace",
			"name": "codex_app",
			"tools": [{
				"type": "function",
				"name": "automation_update",
				"description": "Create, update, view, or delete recurring automations",
				"parameters": {
					"oneOf": [
						{"type":"object","properties":{"mode":{"enum":["view"]},"id":{"type":"string"}},"required":["mode","id"]},
						{"type":"object","properties":{"mode":{"enum":["update"]},"id":{"type":"string"}},"required":["mode","id"]}
					]
				}
			}]
		}]
	}`)

	var requestBody map[string]any
	require.NoError(t, json.Unmarshal(body, &requestBody))

	_, changed, err := AdaptResponsesClientTools(requestBody)
	require.NoError(t, err)
	require.True(t, changed)

	adapted, err := json.Marshal(requestBody)
	require.NoError(t, err)

	var responsesReq ResponsesRequest
	require.NoError(t, json.Unmarshal(adapted, &responsesReq))

	anthropicReq, err := ResponsesToAnthropicRequest(&responsesReq)
	require.NoError(t, err)
	require.Len(t, anthropicReq.Tools, 1)

	tool := anthropicReq.Tools[0]
	assert.Equal(t, "codex_app__automation_update", tool.Name)
	schema := requireObjectInputSchema(t, tool.InputSchema)
	assert.NotContains(t, schema, "oneOf")
	assert.NotContains(t, schema, "anyOf")
	assert.NotContains(t, schema, "allOf")
	assert.JSONEq(t, `["mode","id"]`, string(schema["required"]))

	wire, err := json.Marshal(tool)
	require.NoError(t, err)
	assert.NotContains(t, string(wire), `"oneOf"`)
	assert.NotContains(t, string(wire), `"allOf"`)
	// 同名属性两侧的约束用嵌套联合保留，只允许出现在 properties 内部。
	assert.Contains(t, string(wire), `"mode":{"anyOf":[{"enum":["view"]},{"enum":["update"]}]}`)
}
