package service

import (
	"encoding/json"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestAliasOpenAIOAuthReservedToolNames_RewritesDeclarationsAndReferences(t *testing.T) {
	reqBody := map[string]any{
		"tools": []any{
			map[string]any{"type": "function", "name": "python"},
			map[string]any{"type": "namespace", "name": "code", "tools": []any{
				map[string]any{"type": "function", "name": "shell"},
			}},
		},
		"tool_choice": map[string]any{"type": "function", "name": "python"},
		"input": []any{
			map[string]any{"type": "function_call", "name": "python", "call_id": "fc_1"},
			map[string]any{"type": "additional_tools", "tools": []any{
				map[string]any{"type": "function", "function": map[string]any{"name": "python"}},
			}},
		},
	}

	reverse, changed, err := aliasOpenAIOAuthReservedToolNames(reqBody)
	require.NoError(t, err)
	require.True(t, changed)
	require.Equal(t, "python", reverse[codexPythonToolAlias])
	tools, ok := reqBody["tools"].([]any)
	require.True(t, ok)
	require.NotEmpty(t, tools)
	firstTool, ok := tools[0].(map[string]any)
	require.True(t, ok)
	require.Equal(t, codexPythonToolAlias, firstTool["name"])
	toolChoice, ok := reqBody["tool_choice"].(map[string]any)
	require.True(t, ok)
	require.Equal(t, codexPythonToolAlias, toolChoice["name"])
	input, ok := reqBody["input"].([]any)
	require.True(t, ok)
	require.NotEmpty(t, input)
	firstInput, ok := input[0].(map[string]any)
	require.True(t, ok)
	require.Equal(t, codexPythonToolAlias, firstInput["name"])
	secondInput, ok := input[1].(map[string]any)
	require.True(t, ok)
	nestedTools, ok := secondInput["tools"].([]any)
	require.True(t, ok)
	require.NotEmpty(t, nestedTools)
	nestedTool, ok := nestedTools[0].(map[string]any)
	require.True(t, ok)
	nestedFn, ok := nestedTool["function"].(map[string]any)
	require.True(t, ok)
	require.Equal(t, codexPythonToolAlias, nestedFn["name"])
}

func TestAliasOpenAIOAuthReservedToolNames_CollisionDoesNotMutate(t *testing.T) {
	reqBody := map[string]any{"tools": []any{
		map[string]any{"type": "function", "name": "python"},
		map[string]any{"type": "function", "name": codexPythonToolAlias},
	}}
	before, err := json.Marshal(reqBody)
	require.NoError(t, err)

	reverse, changed, err := aliasOpenAIOAuthReservedToolNames(reqBody)
	require.ErrorContains(t, err, `both normalize to "python__sub2api"`)
	require.False(t, changed)
	require.Nil(t, reverse)
	after, marshalErr := json.Marshal(reqBody)
	require.NoError(t, marshalErr)
	require.JSONEq(t, string(before), string(after))
}

func TestApplyCodexOAuthTransform_ReservedPythonNameIsOAuthOnly(t *testing.T) {
	reqBody := map[string]any{
		"model": "gpt-5.5",
		"tools": []any{map[string]any{"type": "function", "name": "PYTHON"}},
	}

	result := applyCodexOAuthTransform(reqBody, true, false)
	require.NoError(t, result.Error)
	require.Equal(t, "PYTHON", result.ToolNameReverse[codexPythonToolAlias])
	tools, ok := reqBody["tools"].([]any)
	require.True(t, ok)
	require.NotEmpty(t, tools)
	tool, ok := tools[0].(map[string]any)
	require.True(t, ok)
	require.Equal(t, codexPythonToolAlias, tool["name"])

	apiKeyBody := []byte(`{"type":"response.create","tools":[{"type":"function","name":"python"}]}`)
	normalized, changed, err := normalizeOpenAIResponsesWebSocketCompatibilityBody(apiKeyBody, &Account{Platform: PlatformOpenAI, Type: AccountTypeAPIKey})
	require.NoError(t, err)
	require.False(t, changed)
	require.JSONEq(t, string(apiKeyBody), string(normalized))
}

func TestRestoreCodexToolNamesFromContext_HTTPAndWSPayloadShapes(t *testing.T) {
	c, _ := gin.CreateTestContext(nil)
	setCodexToolNameReverse(c, map[string]string{codexPythonToolAlias: "python"})

	streamEvent := restoreCodexToolNamesFromContext(c, []byte(
		`{"type":"response.output_item.done","item":{"type":"function_call","name":"python__sub2api"},"note":"python__sub2api"}`,
	))
	require.Equal(t, "python", gjson.GetBytes(streamEvent, "item.name").String())
	require.Equal(t, "python__sub2api", gjson.GetBytes(streamEvent, "note").String())

	nonStreaming := restoreCodexToolNamesFromContext(c, []byte(
		`{"id":"resp_1","output":[{"type":"function_call","name":"python__sub2api"}]}`,
	))
	require.Equal(t, "python", gjson.GetBytes(nonStreaming, "output.0.name").String())

	setCodexToolNameReverse(c, nil)
	require.JSONEq(t,
		`{"type":"response.output_item.added","item":{"name":"python__sub2api"}}`,
		string(restoreCodexToolNamesFromContext(c, []byte(`{"type":"response.output_item.added","item":{"name":"python__sub2api"}}`))),
	)
}

func TestAliasOpenAIOAuthReservedToolNames_SessionUpdateOnlyTouchesFunctionProtocolNodes(t *testing.T) {
	body := []byte(`{"type":"session.update","session":{"tools":[{"type":"function","name":"python"},{"type":"image_generation","name":"python"},{"type":"namespace","name":"python","tools":[{"type":"function","name":"shell"}]}]},"metadata":{"name":"python"},"sequence":900719925474099312345}`)

	aliased, reverse, changed, err := aliasOpenAIOAuthReservedToolNamesBody(body)
	require.NoError(t, err)
	require.True(t, changed)
	require.Equal(t, "python", reverse[codexPythonToolAlias])
	require.Equal(t, codexPythonToolAlias, gjson.GetBytes(aliased, "session.tools.0.name").String())
	require.Equal(t, "python", gjson.GetBytes(aliased, "session.tools.1.name").String())
	require.Equal(t, "python", gjson.GetBytes(aliased, "session.tools.2.name").String())
	require.Equal(t, "shell", gjson.GetBytes(aliased, "session.tools.2.tools.0.name").String())
	require.Equal(t, "python", gjson.GetBytes(aliased, "metadata.name").String())
	require.Equal(t, "900719925474099312345", gjson.GetBytes(aliased, "sequence").Raw)
}

func TestRestoreCodexToolNamesInJSON_OnlyTouchesResponseToolCallNodesAndPreservesNumbers(t *testing.T) {
	reverse := map[string]string{codexPythonToolAlias: "python"}
	body := []byte(`{"type":"response.completed","response":{"output":[{"type":"function_call","name":"python__sub2api"},{"type":"message","name":"python__sub2api","content":[]}]},"metadata":{"name":"python__sub2api"},"sequence":900719925474099312345}`)

	restored := restoreCodexToolNamesInJSON(body, reverse)
	require.Equal(t, "python", gjson.GetBytes(restored, "response.output.0.name").String())
	require.Equal(t, codexPythonToolAlias, gjson.GetBytes(restored, "response.output.1.name").String())
	require.Equal(t, codexPythonToolAlias, gjson.GetBytes(restored, "metadata.name").String())
	require.Equal(t, "900719925474099312345", gjson.GetBytes(restored, "sequence").Raw)
}

func TestRestoreCodexToolNamesInJSON_ExplicitHTTPAndSSEToolCallProtocols(t *testing.T) {
	reverse := map[string]string{codexPythonToolAlias: "python"}
	tests := []struct {
		name string
		body string
		path string
	}{
		{
			name: "chat http",
			body: `{"choices":[{"message":{"tool_calls":[{"type":"function","function":{"name":"python__sub2api"}}]}}],"metadata":{"name":"python__sub2api"}}`,
			path: "choices.0.message.tool_calls.0.function.name",
		},
		{
			name: "chat sse",
			body: `{"choices":[{"delta":{"tool_calls":[{"type":"function","function":{"name":"python__sub2api"}}]}}],"metadata":{"name":"python__sub2api"}}`,
			path: "choices.0.delta.tool_calls.0.function.name",
		},
		{
			name: "messages tool use",
			body: `{"type":"content_block_start","content":[{"type":"tool_use","name":"python__sub2api"}],"metadata":{"name":"python__sub2api"}}`,
			path: "content.0.name",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			restored := restoreCodexToolNamesInJSON([]byte(tt.body), reverse)
			require.Equal(t, "python", gjson.GetBytes(restored, tt.path).String())
			require.Equal(t, codexPythonToolAlias, gjson.GetBytes(restored, "metadata.name").String())
		})
	}
}

func TestAliasOpenAIOAuthReservedToolNames_PromptCompatibilityRunsFirst(t *testing.T) {
	body := []byte(`{"model":"gpt-5.5","prompt":[{"type":"function_call","name":"python","call_id":"fc_1"}],"functions":[{"name":"python"}],"function_call":{"name":"python"},"sequence":900719925474099312345}`)
	reqBody, err := getOpenAIRequestBodyMap(nil, body)
	require.NoError(t, err)
	result := applyCodexOAuthTransform(reqBody, true, false)
	require.NoError(t, result.Error)
	input, ok := reqBody["input"].([]any)
	require.True(t, ok)
	require.NotEmpty(t, input)
	firstInput, ok := input[0].(map[string]any)
	require.True(t, ok)
	require.Equal(t, codexPythonToolAlias, firstInput["name"])
	tools, ok := reqBody["tools"].([]any)
	require.True(t, ok)
	require.NotEmpty(t, tools)
	tool, ok := tools[0].(map[string]any)
	require.True(t, ok)
	require.Equal(t, codexPythonToolAlias, tool["name"])
	toolChoice, ok := reqBody["tool_choice"].(map[string]any)
	require.True(t, ok)
	require.Equal(t, codexPythonToolAlias, toolChoice["name"])
	encoded, err := json.Marshal(reqBody)
	require.NoError(t, err)
	require.Equal(t, "900719925474099312345", gjson.GetBytes(encoded, "sequence").Raw)
}

func TestCodexToolNameReverse_WSSessionReplacementDoesNotChangeActiveTurn(t *testing.T) {
	c, _ := gin.CreateTestContext(nil)
	setCodexToolNameReverse(c, nil)
	first := []byte(`{"type":"response.create","tools":[{"type":"function","name":"python"}]}`)
	updateCodexToolNameReverseForWSFrame(c, first, map[string]string{codexPythonToolAlias: "python"})

	update := []byte(`{"type":"session.update","session":{"tools":[{"type":"function","name":"python__sub2api"}]}}`)
	updateCodexToolNameReverseForWSFrame(c, update, nil)
	currentOutput := restoreCodexToolNamesFromContext(c, []byte(`{"type":"response.output_item.done","item":{"type":"function_call","name":"python__sub2api"}}`))
	require.Equal(t, "python", gjson.GetBytes(currentOutput, "item.name").String())
	sessionEcho := restoreCodexToolNamesFromContext(c, []byte(`{"type":"session.updated","session":{"tools":[{"type":"function","name":"python__sub2api"}]}}`))
	require.Equal(t, codexPythonToolAlias, gjson.GetBytes(sessionEcho, "session.tools.0.name").String())

	next := []byte(`{"type":"response.create","input":"next"}`)
	updateCodexToolNameReverseForWSFrame(c, next, nil)
	nextOutput := restoreCodexToolNamesFromContext(c, []byte(`{"type":"response.output_item.done","item":{"type":"function_call","name":"python__sub2api"}}`))
	require.Equal(t, codexPythonToolAlias, gjson.GetBytes(nextOutput, "item.name").String())

	sessionPython := []byte(`{"type":"session.update","session":{"tools":[{"type":"function","name":"python"}]}}`)
	updateCodexToolNameReverseForWSFrame(c, sessionPython, map[string]string{codexPythonToolAlias: "python"})
	explicitLiteral := []byte(`{"type":"response.create","input":[{"type":"additional_tools","tools":[{"type":"function","name":"python__sub2api"}]}]}`)
	updateCodexToolNameReverseForWSFrame(c, explicitLiteral, nil)
	literalOutput := restoreCodexToolNamesFromContext(c, []byte(`{"type":"response.output_item.done","item":{"type":"function_call","name":"python__sub2api"}}`))
	require.Equal(t, codexPythonToolAlias, gjson.GetBytes(literalOutput, "item.name").String())
	updateCodexToolNameReverseForWSFrame(c, next, nil)
	inheritedOutput := restoreCodexToolNamesFromContext(c, []byte(`{"type":"response.output_item.done","item":{"type":"function_call","name":"python__sub2api"}}`))
	require.Equal(t, "python", gjson.GetBytes(inheritedOutput, "item.name").String())
}

func TestDecodeOpenAIJSONUseNumberRejectsTrailingDocument(t *testing.T) {
	var decoded map[string]any
	require.Error(t, decodeOpenAIJSONUseNumber([]byte(`{"name":"python"}{"extra":true}`), &decoded))
}

func TestRestoreCodexToolNamesFromSSEContextUsesEventLineTypeWithoutAddingType(t *testing.T) {
	c, _ := gin.CreateTestContext(nil)
	setCodexToolNameReverse(c, map[string]string{codexPythonToolAlias: codexReservedPythonToolName})
	payload := []byte(`{"item":{"type":"function_call","name":"python__sub2api"},"metadata":{"name":"python__sub2api"}}`)

	restored := restoreCodexToolNamesFromSSEContext(c, payload, "response.output_item.done")

	require.Equal(t, codexReservedPythonToolName, gjson.GetBytes(restored, "item.name").String())
	require.Equal(t, codexPythonToolAlias, gjson.GetBytes(restored, "metadata.name").String())
	require.False(t, gjson.GetBytes(restored, "type").Exists())
}
