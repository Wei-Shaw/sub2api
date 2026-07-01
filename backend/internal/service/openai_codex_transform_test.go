package service

import (
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestApplyCodexOAuthTransform_ToolContinuationPreservesInput(t *testing.T) {
	// 续链场景：保留 item_reference 与 id，但不再强制 store=true。

	reqBody := map[string]any{
		"model": "gpt-5.2",
		"input": []any{
			map[string]any{"type": "item_reference", "id": "ref1", "text": "x"REDACTED,
			map[string]any{"type": "function_call_output", "call_id": "call_1", "output": "ok", "id": "o1"REDACTED,
	REDACTED,
		"tool_choice": "auto",
REDACTED

	applyCodexOAuthTransform(reqBody, false, false)

	// 未显式设置 store=true，默认为 false。
	store, ok := reqBody["store"].(bool)
	require.True(t, ok)
	require.False(t, store)

	input, ok := reqBody["input"].([]any)
	require.True(t, ok)
	require.Len(t, input, 2)

	// 校验 input[0] 为 map，避免断言失败导致测试中断。
	first, ok := input[0].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "item_reference", first["type"])
	require.Equal(t, "ref1", first["id"])

	// 校验 input[1] 为 map，确保后续字段断言安全。
	second, ok := input[1].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "o1", second["id"])
	require.Equal(t, "fc_1", second["call_id"])
REDACTED

func TestApplyCodexOAuthTransform_MessagesBridgePromptCacheKeyIsHeaderOnly(t *testing.T) {
	reqBody := map[string]any{
		"model":            "gpt-5.5",
		"prompt_cache_key": "anthropic-metadata-session-1",
		"input": []any{
			map[string]any{
				"type": "message",
				"role": "developer",
				"content": []any{
					map[string]any{
						"type": "input_text",
						"text": openAICompatClaudeCodeTodoGuardMarker,
				REDACTED,
			REDACTED,
		REDACTED,
			map[string]any{
				"type":    "message",
				"role":    "user",
				"content": "hello",
		REDACTED,
	REDACTED,
REDACTED

	result := applyCodexOAuthTransformWithOptions(reqBody, codexOAuthTransformOptions{
		SkipDefaultInstructions: true,
		PreserveToolCallIDs:     true,
REDACTED)

	require.Equal(t, "anthropic-metadata-session-1", result.PromptCacheKey)
	require.True(t, result.Modified)
	require.NotContains(t, reqBody, "prompt_cache_key")
REDACTED

func TestApplyCodexOAuthTransform_ToolContinuationPreservesNativeMessageAndReasoningIDs(t *testing.T) {
	reqBody := map[string]any{
		"model": "gpt-5.2",
		"input": []any{
			map[string]any{"type": "message", "id": "msg_0", "role": "user", "content": "hi"REDACTED,
			map[string]any{"type": "item_reference", "id": "rs_123"REDACTED,
	REDACTED,
		"tool_choice": "auto",
REDACTED

	applyCodexOAuthTransform(reqBody, false, false)

	input, ok := reqBody["input"].([]any)
	require.True(t, ok)
	require.Len(t, input, 2)

	first, ok := input[0].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "msg_0", first["id"])

	second, ok := input[1].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "rs_123", second["id"])
REDACTED

func TestApplyCodexOAuthTransform_ToolContinuationNormalizesToolReferenceIDsOnly(t *testing.T) {
	reqBody := map[string]any{
		"model": "gpt-5.2",
		"input": []any{
			map[string]any{"type": "item_reference", "id": "call_1"REDACTED,
			map[string]any{"type": "function_call_output", "call_id": "call_1", "output": "ok"REDACTED,
	REDACTED,
		"tool_choice": "auto",
REDACTED

	applyCodexOAuthTransform(reqBody, false, false)

	input, ok := reqBody["input"].([]any)
	require.True(t, ok)
	require.Len(t, input, 2)

	first, ok := input[0].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "fc_1", first["id"])

	second, ok := input[1].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "fc_1", second["call_id"])
REDACTED

func TestApplyCodexOAuthTransform_ToolSearchOutputPreservesCallID(t *testing.T) {
	reqBody := map[string]any{
		"model": "gpt-5.2",
		"input": []any{
			map[string]any{"type": "tool_search_output", "call_id": "call_1", "output": "ok"REDACTED,
	REDACTED,
REDACTED

	applyCodexOAuthTransform(reqBody, false, false)

	input, ok := reqBody["input"].([]any)
	require.True(t, ok)
	require.Len(t, input, 1)

	first, ok := input[0].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "tool_search_output", first["type"])
	require.Equal(t, "fc_1", first["call_id"])
REDACTED

func TestApplyCodexOAuthTransform_CustomAndMCPToolOutputsPreserveCallID(t *testing.T) {
	reqBody := map[string]any{
		"model": "gpt-5.2",
		"input": []any{
			map[string]any{"type": "custom_tool_call_output", "call_id": "call_custom", "output": "ok"REDACTED,
			map[string]any{"type": "mcp_tool_call_output", "call_id": "call_mcp", "output": "ok"REDACTED,
	REDACTED,
REDACTED

	applyCodexOAuthTransform(reqBody, false, false)

	input, ok := reqBody["input"].([]any)
	require.True(t, ok)
	require.Len(t, input, 2)

	first, ok := input[0].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "fc_custom", first["call_id"])

	second, ok := input[1].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "fc_mcp", second["call_id"])
REDACTED

func TestApplyCodexOAuthTransform_ImageAndWebSearchCallsDoNotGainCallID(t *testing.T) {
	reqBody := map[string]any{
		"model": "gpt-5.2",
		"input": []any{
			map[string]any{"type": "image_generation_call", "id": "ig_123", "status": "completed"REDACTED,
			map[string]any{"type": "web_search_call", "call_id": "call_bad", "status": "completed"REDACTED,
	REDACTED,
		"tool_choice": "auto",
REDACTED

	applyCodexOAuthTransform(reqBody, false, false)

	input, ok := reqBody["input"].([]any)
	require.True(t, ok)
	require.Len(t, input, 2)

	first, ok := input[0].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "ig_123", first["id"])
	_, hasCallID := first["call_id"]
	require.False(t, hasCallID)

	second, ok := input[1].(map[string]any)
	require.True(t, ok)
	_, hasCallID = second["call_id"]
	require.False(t, hasCallID)
REDACTED

func TestApplyCodexOAuthTransform_ConvertsToolRoleMessageToFunctionCallOutput(t *testing.T) {
	reqBody := map[string]any{
		"model": "gpt-5.4",
		"input": []any{
			map[string]any{
				"type":         "message",
				"role":         "tool",
				"tool_call_id": "call_1",
				"content":      "ok",
		REDACTED,
	REDACTED,
REDACTED

	applyCodexOAuthTransform(reqBody, true, false)

	input, ok := reqBody["input"].([]any)
	require.True(t, ok)
	require.Len(t, input, 1)

	item, ok := input[0].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "function_call_output", item["type"])
	require.Equal(t, "fc_1", item["call_id"])
	require.Equal(t, "ok", item["output"])
	_, hasRole := item["role"]
	require.False(t, hasRole)
REDACTED

func TestApplyCodexOAuthTransform_StringifiesNonStringMessageContentText(t *testing.T) {
	reqBody := map[string]any{
		"model": "gpt-5.4",
		"input": []any{
			map[string]any{
				"type": "message",
				"role": "user",
				"content": []any{
					map[string]any{"type": "input_text", "text": []any{"a", "b"REDACTEDREDACTED,
			REDACTED,
		REDACTED,
	REDACTED,
REDACTED

	applyCodexOAuthTransform(reqBody, true, false)

	input, ok := reqBody["input"].([]any)
	require.True(t, ok)
	item, ok := input[0].(map[string]any)
	require.True(t, ok)
	content, ok := item["content"].([]any)
	require.True(t, ok)
	part, ok := content[0].(map[string]any)
	require.True(t, ok)
	require.Equal(t, `["a","b"]`, part["text"])
REDACTED

func TestApplyCodexOAuthTransform_DowngradesUnknownToolChoice(t *testing.T) {
	reqBody := map[string]any{
		"model": "gpt-5.4",
		"tools": []any{
			map[string]any{"type": "function", "name": "shell"REDACTED,
	REDACTED,
		"tool_choice": map[string]any{"type": "custom"REDACTED,
REDACTED

	applyCodexOAuthTransform(reqBody, true, false)

	require.Equal(t, "auto", reqBody["tool_choice"])
REDACTED

func TestApplyCodexOAuthTransform_PreservesKnownToolChoice(t *testing.T) {
	reqBody := map[string]any{
		"model": "gpt-5.4",
		"tools": []any{
			map[string]any{"type": "custom", "name": "shell"REDACTED,
	REDACTED,
		"tool_choice": map[string]any{"type": "custom"REDACTED,
REDACTED

	applyCodexOAuthTransform(reqBody, true, false)

	choice, ok := reqBody["tool_choice"].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "custom", choice["type"])
REDACTED

func TestApplyCodexOAuthTransform_NormalizesLegacyFunctionToolChoice(t *testing.T) {
	reqBody := map[string]any{
		"model": "gpt-5.4",
		"tools": []any{
			map[string]any{"type": "function", "name": "shell"REDACTED,
	REDACTED,
		"tool_choice": map[string]any{
			"type":     "function",
			"function": map[string]any{"name": "shell"REDACTED,
	REDACTED,
REDACTED

	applyCodexOAuthTransform(reqBody, true, false)

	choice, ok := reqBody["tool_choice"].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "function", choice["type"])
	require.Equal(t, "shell", choice["name"])
	require.NotContains(t, choice, "function")
REDACTED

func TestApplyCodexOAuthTransform_DowngradesMissingFunctionToolChoice(t *testing.T) {
	reqBody := map[string]any{
		"model": "gpt-5.4",
		"tools": []any{
			map[string]any{"type": "function", "name": "shell"REDACTED,
	REDACTED,
		"tool_choice": map[string]any{
			"type":     "function",
			"function": map[string]any{"name": "missing"REDACTED,
	REDACTED,
REDACTED

	applyCodexOAuthTransform(reqBody, true, false)

	require.Equal(t, "auto", reqBody["tool_choice"])
REDACTED

func TestApplyCodexOAuthTransform_AddsFallbackNameForFunctionCallInput(t *testing.T) {
	reqBody := map[string]any{
		"model": "gpt-5.4",
		"input": []any{
			map[string]any{"type": "message", "role": "user", "content": "run tool"REDACTED,
			map[string]any{"type": "function_call", "call_id": "call_1", "arguments": "{REDACTED"REDACTED,
	REDACTED,
REDACTED

	applyCodexOAuthTransform(reqBody, true, false)

	input, ok := reqBody["input"].([]any)
	require.True(t, ok)
	require.Len(t, input, 2)
	item, ok := input[1].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "function_call", item["type"])
	require.Equal(t, "tool", item["name"])
	require.Equal(t, "fc_1", item["call_id"])
REDACTED

func TestApplyCodexOAuthTransform_PreservesFunctionCallInputName(t *testing.T) {
	reqBody := map[string]any{
		"model": "gpt-5.4",
		"input": []any{
			map[string]any{"type": "custom_tool_call", "call_id": "call_1", "name": "shell", "input": "pwd"REDACTED,
	REDACTED,
REDACTED

	applyCodexOAuthTransform(reqBody, true, false)

	input, ok := reqBody["input"].([]any)
	require.True(t, ok)
	require.Len(t, input, 1)
	item, ok := input[0].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "shell", item["name"])
	require.Equal(t, "fc_1", item["call_id"])
REDACTED

func TestApplyCodexOAuthTransform_PreservesMCPToolCallIDAndName(t *testing.T) {
	reqBody := map[string]any{
		"model": "gpt-5.4",
		"input": []any{
			map[string]any{
				"type":      "mcp_tool_call",
				"call_id":   "call_abc",
				"name":      "remote_tool",
				"arguments": "{REDACTED",
		REDACTED,
	REDACTED,
REDACTED

	applyCodexOAuthTransform(reqBody, true, false)

	input, ok := reqBody["input"].([]any)
	require.True(t, ok)
	require.Len(t, input, 1)
	item, ok := input[0].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "mcp_tool_call", item["type"])
	require.Equal(t, "remote_tool", item["name"])
	require.Equal(t, "fc_abc", item["call_id"])
REDACTED

func TestCodexInputItemRequiresNameTypesAllowCallID(t *testing.T) {
	for _, typ := range []string{"function_call", "custom_tool_call", "mcp_tool_call"REDACTED {
		require.True(t, codexInputItemRequiresName(typ), typ)
		require.True(t, isCodexToolCallItemType(typ), typ)
REDACTED
REDACTED

func TestApplyCodexOAuthTransform_ExplicitStoreFalsePreserved(t *testing.T) {
	// 续链场景：显式 store=false 不再强制为 true，保持 false。

	reqBody := map[string]any{
		"model": "gpt-5.1",
		"store": false,
		"input": []any{
			map[string]any{"type": "function_call_output", "call_id": "call_1"REDACTED,
	REDACTED,
		"tool_choice": "auto",
REDACTED

	applyCodexOAuthTransform(reqBody, false, false)

	store, ok := reqBody["store"].(bool)
	require.True(t, ok)
	require.False(t, store)
REDACTED

func TestApplyCodexOAuthTransform_ExplicitStoreTrueForcedFalse(t *testing.T) {
	// 显式 store=true 也会强制为 false。

	reqBody := map[string]any{
		"model": "gpt-5.1",
		"store": true,
		"input": []any{
			map[string]any{"type": "function_call_output", "call_id": "call_1"REDACTED,
	REDACTED,
		"tool_choice": "auto",
REDACTED

	applyCodexOAuthTransform(reqBody, false, false)

	store, ok := reqBody["store"].(bool)
	require.True(t, ok)
	require.False(t, store)
REDACTED

func TestApplyCodexOAuthTransform_CompactForcesNonStreaming(t *testing.T) {
	reqBody := map[string]any{
		"model":  "gpt-5.1-codex",
		"store":  true,
		"stream": true,
REDACTED

	result := applyCodexOAuthTransform(reqBody, true, true)

	_, hasStore := reqBody["store"]
	require.False(t, hasStore)
	_, hasStream := reqBody["stream"]
	require.False(t, hasStream)
	require.True(t, result.Modified)
REDACTED

func TestApplyCodexOAuthTransform_NonContinuationDefaultsStoreFalseAndStripsIDs(t *testing.T) {
	// 非续链场景：未设置 store 时默认 false，并移除 input 中的 id。

	reqBody := map[string]any{
		"model": "gpt-5.1",
		"input": []any{
			map[string]any{"type": "text", "id": "t1", "text": "hi"REDACTED,
	REDACTED,
REDACTED

	applyCodexOAuthTransform(reqBody, false, false)

	store, ok := reqBody["store"].(bool)
	require.True(t, ok)
	require.False(t, store)

	input, ok := reqBody["input"].([]any)
	require.True(t, ok)
	require.Len(t, input, 1)
	// 校验 input[0] 为 map，避免类型不匹配触发 errcheck。
	item, ok := input[0].(map[string]any)
	require.True(t, ok)
	_, hasID := item["id"]
	require.False(t, hasID)
REDACTED

func TestFilterCodexInput_RemovesItemReferenceWhenNotPreserved(t *testing.T) {
	input := []any{
		map[string]any{"type": "item_reference", "id": "ref1"REDACTED,
		map[string]any{"type": "text", "id": "t1", "text": "hi"REDACTED,
REDACTED

	filtered := filterCodexInput(input, false)
	require.Len(t, filtered, 1)
	// 校验 filtered[0] 为 map，确保字段检查可靠。
	item, ok := filtered[0].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "text", item["type"])
	_, hasID := item["id"]
	require.False(t, hasID)
REDACTED

func TestApplyCodexOAuthTransform_NormalizeCodexTools_PreservesResponsesFunctionTools(t *testing.T) {
	reqBody := map[string]any{
		"model": "gpt-5.1",
		"tools": []any{
			map[string]any{
				"type":        "function",
				"name":        "bash",
				"description": "desc",
				"parameters":  map[string]any{"type": "object"REDACTED,
		REDACTED,
			map[string]any{
				"type":     "function",
				"function": nil,
		REDACTED,
	REDACTED,
REDACTED

	applyCodexOAuthTransform(reqBody, false, false)

	tools, ok := reqBody["tools"].([]any)
	require.True(t, ok)
	require.Len(t, tools, 1)

	first, ok := tools[0].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "function", first["type"])
	require.Equal(t, "bash", first["name"])
REDACTED

func TestNormalizeOpenAIResponsesImageGenerationTools_RewritesLegacyFields(t *testing.T) {
	reqBody := map[string]any{
		"tools": []any{
			map[string]any{
				"type":        "image_generation",
				"format":      "png",
				"compression": 60,
		REDACTED,
	REDACTED,
REDACTED

	modified := normalizeOpenAIResponsesImageGenerationTools(reqBody)
	require.True(t, modified)

	tools, ok := reqBody["tools"].([]any)
	require.True(t, ok)
	first, ok := tools[0].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "png", first["output_format"])
	require.Equal(t, 60, first["output_compression"])
	_, hasFormat := first["format"]
	require.False(t, hasFormat)
	_, hasCompression := first["compression"]
	require.False(t, hasCompression)
REDACTED

func TestEnsureOpenAIResponsesImageGenerationTool_NoTools(t *testing.T) {
	reqBody := map[string]any{
		"model": "gpt-5.4",
		"input": "draw a cat",
REDACTED

	modified := ensureOpenAIResponsesImageGenerationTool(reqBody)
	require.True(t, modified)

	tools, ok := reqBody["tools"].([]any)
	require.True(t, ok)
	require.Len(t, tools, 1)
	tool, ok := tools[0].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "image_generation", tool["type"])
	require.Equal(t, "png", tool["output_format"])
REDACTED

func TestEnsureOpenAIResponsesImageGenerationTool_SkipsSpark(t *testing.T) {
	reqBody := map[string]any{
		"model": "gpt-5.3-codex-spark",
		"input": "draw a cat",
REDACTED

	modified := ensureOpenAIResponsesImageGenerationTool(reqBody)
	require.False(t, modified)
	require.NotContains(t, reqBody, "tools")
REDACTED

func TestEnsureOpenAIResponsesImageGenerationTool_AppendsToExistingTools(t *testing.T) {
	reqBody := map[string]any{
		"model": "gpt-5.4",
		"tools": []any{
			map[string]any{"type": "web_search"REDACTED,
	REDACTED,
REDACTED

	modified := ensureOpenAIResponsesImageGenerationTool(reqBody)
	require.True(t, modified)

	tools, ok := reqBody["tools"].([]any)
	require.True(t, ok)
	require.Len(t, tools, 2)
	first, ok := tools[0].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "web_search", first["type"])
	second, ok := tools[1].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "image_generation", second["type"])
	require.Equal(t, "png", second["output_format"])
REDACTED

func TestEnsureOpenAIResponsesImageGenerationTool_PreservesExistingImageTool(t *testing.T) {
	reqBody := map[string]any{
		"model": "gpt-5.4",
		"tools": []any{
			map[string]any{"type": "image_generation", "output_format": "webp"REDACTED,
			map[string]any{"type": "web_search"REDACTED,
	REDACTED,
REDACTED

	modified := ensureOpenAIResponsesImageGenerationTool(reqBody)
	require.False(t, modified)

	tools, ok := reqBody["tools"].([]any)
	require.True(t, ok)
	require.Len(t, tools, 2)
	tool, ok := tools[0].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "webp", tool["output_format"])
REDACTED

func TestApplyCodexImageGenerationBridgeInstructions_AppendsBridgeOnce(t *testing.T) {
	reqBody := map[string]any{
		"model":        "gpt-5.4",
		"instructions": "existing instructions",
		"tools": []any{
			map[string]any{"type": "image_generation", "output_format": "png"REDACTED,
	REDACTED,
REDACTED

	modified := applyCodexImageGenerationBridgeInstructions(reqBody)
	require.True(t, modified)

	instructions, ok := reqBody["instructions"].(string)
	require.True(t, ok)
	require.Contains(t, instructions, "existing instructions")
	require.Contains(t, instructions, codexImageGenerationBridgeMarker)
	require.Contains(t, instructions, "Responses native `image_generation` tool")

	modified = applyCodexImageGenerationBridgeInstructions(reqBody)
	require.False(t, modified)
REDACTED

func TestApplyCodexImageGenerationBridgeInstructions_SkipsSpark(t *testing.T) {
	reqBody := map[string]any{
		"model":        "gpt-5.3-codex-spark",
		"instructions": "existing instructions",
		"tools": []any{
			map[string]any{"type": "image_generation", "output_format": "png"REDACTED,
	REDACTED,
REDACTED

	modified := applyCodexImageGenerationBridgeInstructions(reqBody)
	require.False(t, modified)
	require.Equal(t, "existing instructions", reqBody["instructions"])
REDACTED

func TestApplyCodexImageGenerationBridgeInstructions_SkipsWithoutImageTool(t *testing.T) {
	reqBody := map[string]any{
		"instructions": "existing instructions",
		"tools": []any{
			map[string]any{"type": "web_search"REDACTED,
	REDACTED,
REDACTED

	modified := applyCodexImageGenerationBridgeInstructions(reqBody)
	require.False(t, modified)
	require.Equal(t, "existing instructions", reqBody["instructions"])
REDACTED

func TestValidateCodexSparkInputRejectsInputImage(t *testing.T) {
	reqBody := map[string]any{
		"model": "gpt-5.3-codex-spark",
		"input": []any{
			map[string]any{
				"role": "user",
				"content": []any{
					map[string]any{"type": "input_text", "text": "describe"REDACTED,
					map[string]any{"type": "input_image", "image_url": "data:image/png;base64,aGVsbG8="REDACTED,
			REDACTED,
		REDACTED,
	REDACTED,
REDACTED

	err := validateCodexSparkInput(reqBody, "gpt-5.3-codex-spark")
REDACTED
	require.Contains(t, err.Error(), "does not support image input")
REDACTED

func TestValidateCodexSparkInputRejectsChatImageURL(t *testing.T) {
	reqBody := map[string]any{
		"model": "gpt-5.3-codex-spark",
		"messages": []any{
			map[string]any{
				"role": "user",
				"content": []any{
					map[string]any{"type": "text", "text": "describe"REDACTED,
					map[string]any{"type": "image_url", "image_url": map[string]any{"url": "data:image/png;base64,aGVsbG8="REDACTEDREDACTED,
			REDACTED,
		REDACTED,
	REDACTED,
REDACTED

	err := validateCodexSparkInput(reqBody, "gpt-5.3-codex-spark")
REDACTED
REDACTED

func TestValidateCodexSparkInputAllowsTextOnly(t *testing.T) {
	reqBody := map[string]any{
		"model": "gpt-5.3-codex-spark",
		"input": []any{
			map[string]any{
				"role": "user",
				"content": []any{
					map[string]any{"type": "input_text", "text": "hello"REDACTED,
			REDACTED,
		REDACTED,
	REDACTED,
REDACTED

	require.NoError(t, validateCodexSparkInput(reqBody, "gpt-5.3-codex-spark"))
REDACTED

func TestApplyCodexOAuthTransform_AddsSparkImageUnsupportedInstructions(t *testing.T) {
	reqBody := map[string]any{
		"model":        "gpt-5.3-codex-spark",
		"instructions": "existing instructions",
		"input":        "hello",
REDACTED

	result := applyCodexOAuthTransform(reqBody, true, false)
	require.True(t, result.Modified)

	instructions, ok := reqBody["instructions"].(string)
	require.True(t, ok)
	require.Contains(t, instructions, "existing instructions")
	require.Contains(t, instructions, codexSparkImageUnsupportedMarker)
	require.Contains(t, instructions, "does not support image generation")
	require.Contains(t, instructions, "switch to a non-Spark Codex model")
	require.NotContains(t, instructions, codexImageGenerationBridgeMarker)
REDACTED

func TestApplyCodexOAuthTransform_DoesNotAddSparkImageUnsupportedForNonSpark(t *testing.T) {
	reqBody := map[string]any{
		"model":        "gpt-5.4",
		"instructions": "existing instructions",
		"input":        "hello",
REDACTED

	applyCodexOAuthTransform(reqBody, true, false)
	instructions, ok := reqBody["instructions"].(string)
	require.True(t, ok)
	require.NotContains(t, instructions, codexSparkImageUnsupportedMarker)
REDACTED

// gpt-5.3-codex-spark rejects the image_generation tool upstream (HTTP 400
// invalid_request_error, param=tools). Codex CLI advertises that tool by default,
// so the OAuth transform must strip it for spark while keeping the rest.
func TestApplyCodexOAuthTransform_StripsImageGenerationToolForSpark(t *testing.T) {
	reqBody := map[string]any{
		"model": "gpt-5.3-codex-spark",
		"input": "hello",
		"tools": []any{
			map[string]any{"type": "function", "name": "shell"REDACTED,
			map[string]any{"type": "image_generation", "output_format": "png"REDACTED,
	REDACTED,
REDACTED

	result := applyCodexOAuthTransform(reqBody, true, false)
	require.True(t, result.Modified)
	require.False(t, hasOpenAIImageGenerationTool(reqBody))

	tools, ok := reqBody["tools"].([]any)
	require.True(t, ok)
	require.Len(t, tools, 1)
	first, ok := tools[0].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "function", first["type"])
	require.Equal(t, "shell", first["name"])
REDACTED

// Spark reasoning-effort aliases (e.g. -low/-high) normalize to gpt-5.3-codex-spark,
// so they must be stripped too.
func TestApplyCodexOAuthTransform_StripsImageGenerationToolForSparkAlias(t *testing.T) {
	reqBody := map[string]any{
		"model": "gpt-5.3-codex-spark-high",
		"input": "hello",
		"tools": []any{
			map[string]any{"type": "image_generation", "output_format": "png"REDACTED,
	REDACTED,
REDACTED

	result := applyCodexOAuthTransform(reqBody, true, false)
	require.True(t, result.Modified)
	require.False(t, hasOpenAIImageGenerationTool(reqBody))
	// tools became empty after stripping the only entry; the key is dropped.
	_, hasTools := reqBody["tools"]
	require.False(t, hasTools)
REDACTED

// Non-spark Codex models support image_generation; the tool must be preserved.
func TestApplyCodexOAuthTransform_KeepsImageGenerationToolForNonSpark(t *testing.T) {
	reqBody := map[string]any{
		"model": "gpt-5.3-codex",
		"input": "hello",
		"tools": []any{
			map[string]any{"type": "image_generation", "output_format": "png"REDACTED,
	REDACTED,
REDACTED

	applyCodexOAuthTransform(reqBody, true, false)
	require.True(t, hasOpenAIImageGenerationTool(reqBody))
REDACTED

func TestNormalizeOpenAIResponsesImageOnlyModel_BuildsImageToolRequest(t *testing.T) {
	reqBody := map[string]any{
		"model":         "gpt-image-2",
		"prompt":        "draw a cat",
		"size":          "1024x1024",
		"output_format": "png",
REDACTED

	modified := normalizeOpenAIResponsesImageOnlyModel(reqBody)
	require.True(t, modified)
	require.Equal(t, openAIImagesResponsesMainModel, reqBody["model"])
	require.Equal(t, "draw a cat", reqBody["input"])
	_, hasPrompt := reqBody["prompt"]
	require.False(t, hasPrompt)
	_, hasTopLevelSize := reqBody["size"]
	require.False(t, hasTopLevelSize)

	tools, ok := reqBody["tools"].([]any)
	require.True(t, ok)
	require.Len(t, tools, 1)
	tool, ok := tools[0].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "image_generation", tool["type"])
	require.Equal(t, "gpt-image-2", tool["model"])
	require.Equal(t, "1024x1024", tool["size"])
	require.Equal(t, "png", tool["output_format"])

	choice, ok := reqBody["tool_choice"].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "image_generation", choice["type"])
REDACTED

func TestNormalizeOpenAIResponsesImageOnlyModel_PreservesExistingImageTool(t *testing.T) {
	reqBody := map[string]any{
		"model": "gpt-image-2",
		"input": "draw a cat",
		"tools": []any{
			map[string]any{
				"type":  "image_generation",
				"model": "gpt-image-1.5",
		REDACTED,
	REDACTED,
		"tool_choice": "auto",
REDACTED

	modified := normalizeOpenAIResponsesImageOnlyModel(reqBody)
	require.True(t, modified)
	require.Equal(t, openAIImagesResponsesMainModel, reqBody["model"])
	require.Equal(t, "auto", reqBody["tool_choice"])

	tools, ok := reqBody["tools"].([]any)
	require.True(t, ok)
	require.Len(t, tools, 1)
	tool, ok := tools[0].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "gpt-image-1.5", tool["model"])
REDACTED

func TestValidateOpenAIResponsesImageModel_RejectsImageOnlyModel(t *testing.T) {
	err := validateOpenAIResponsesImageModel(map[string]any{
		"tools": []any{
			map[string]any{"type": "image_generation"REDACTED,
	REDACTED,
REDACTED, "gpt-image-2")

	require.ErrorContains(t, err, `/v1/responses image_generation requests require a Responses-capable text model`)
REDACTED

func TestApplyCodexOAuthTransform_EmptyInput(t *testing.T) {
	// 空 input 应保持为空且不触发异常。

	reqBody := map[string]any{
		"model": "gpt-5.1",
		"input": []any{REDACTED,
REDACTED

	applyCodexOAuthTransform(reqBody, false, false)

	input, ok := reqBody["input"].([]any)
	require.True(t, ok)
	require.Len(t, input, 0)
REDACTED

func TestNormalizeCodexModel_Gpt53(t *testing.T) {
	cases := map[string]string{
		"gpt-5.4":                   "gpt-5.4",
		"gpt5.5":                    "gpt-5.5",
		"openai/gpt5.5":             "gpt-5.5",
		"gpt-5.5-pro":               "gpt-5.5-pro",
		"gpt5.5-pro":                "gpt-5.5-pro",
		"openai/gpt5.5-pro":         "gpt-5.5-pro",
		"gpt-5.5-pro-high":          "gpt-5.5-pro",
		"codex-auto-review":         "codex-auto-review",
		"gpt5.4":                    "gpt-5.4",
		"gpt-5.4-high":              "gpt-5.4",
		"gpt-5.4-chat-latest":       "gpt-5.4",
		"gpt 5.4":                   "gpt-5.4",
		"gpt-5.4-mini":              "gpt-5.4-mini",
		"gpt5.4-mini":               "gpt-5.4-mini",
		"gpt5.4mini":                "gpt-5.4-mini",
		"gpt 5.4 mini":              "gpt-5.4-mini",
		"gpt-5.3":                   "gpt-5.3-codex",
		"gpt5.3":                    "gpt-5.3-codex",
		"gpt-5.3-codex":             "gpt-5.3-codex",
		"gpt5.3-codex":              "gpt-5.3-codex",
		"gpt5.3codex":               "gpt-5.3-codex",
		"gpt-5.3-codex-xhigh":       "gpt-5.3-codex",
		"gpt-5.3-codex-spark":       "gpt-5.3-codex-spark",
		"gpt5.3-codex-spark":        "gpt-5.3-codex-spark",
		"gpt5.3codexspark":          "gpt-5.3-codex-spark",
		"gpt 5.3 codex spark":       "gpt-5.3-codex-spark",
		"gpt-5.3-codex-spark-high":  "gpt-5.3-codex-spark",
		"gpt-5.3-codex-spark-xhigh": "gpt-5.3-codex-spark",
		"gpt 5.3 codex":             "gpt-5.3-codex",
REDACTED

	for input, expected := range cases {
		require.Equal(t, expected, normalizeCodexModel(input))
REDACTED
REDACTED

func TestNormalizeCodexModel_RemovedModelsFallbackToSupportedTargets(t *testing.T) {
	cases := map[string]string{
		"":                   "gpt-5.4",
		"gpt-5":              "gpt-5.4",
		"gpt-5-mini":         "gpt-5.4",
		"gpt-5-nano":         "gpt-5.4",
		"gpt-5.1":            "gpt-5.4",
		"gpt-5.1-codex":      "gpt-5.3-codex",
		"gpt-5.1-codex-max":  "gpt-5.3-codex",
		"gpt-5.1-codex-mini": "gpt-5.3-codex",
		"gpt-5.2-codex":      "gpt-5.2",
		"codex-mini-latest":  "gpt-5.3-codex",
		"gpt-5-codex":        "gpt-5.3-codex",
REDACTED

	for input, expected := range cases {
		require.Equal(t, expected, normalizeCodexModel(input))
REDACTED
REDACTED

func TestApplyCodexOAuthTransform_PreservesBareSparkModel(t *testing.T) {
	reqBody := map[string]any{
		"model": "gpt-5.3-codex-spark",
		"input": []any{REDACTED,
REDACTED

	result := applyCodexOAuthTransform(reqBody, false, false)

	require.Equal(t, "gpt-5.3-codex-spark", reqBody["model"])
	require.Equal(t, "gpt-5.3-codex-spark", result.NormalizedModel)
	store, ok := reqBody["store"].(bool)
	require.True(t, ok)
	require.False(t, store)
REDACTED

func TestApplyCodexOAuthTransform_TrimmedModelWithoutPolicyRewrite(t *testing.T) {
	reqBody := map[string]any{
		"model": "  gpt-5.3-codex-spark  ",
		"input": []any{REDACTED,
REDACTED

	result := applyCodexOAuthTransform(reqBody, false, false)

	require.Equal(t, "gpt-5.3-codex-spark", reqBody["model"])
	require.Equal(t, "gpt-5.3-codex-spark", result.NormalizedModel)
	require.True(t, result.Modified)
REDACTED

func TestApplyCodexOAuthTransform_CodexCLI_PreservesExistingInstructions(t *testing.T) {
	// Codex CLI 场景：已有 instructions 时不修改

	reqBody := map[string]any{
		"model":        "gpt-5.1",
		"instructions": "existing instructions",
REDACTED

	result := applyCodexOAuthTransform(reqBody, true, false) // isCodexCLI=true

	instructions, ok := reqBody["instructions"].(string)
	require.True(t, ok)
	require.Equal(t, "existing instructions", instructions)
	// Modified 仍可能为 true（因为其他字段被修改），但 instructions 应保持不变
	_ = result
REDACTED

func TestApplyCodexOAuthTransform_CodexCLI_SuppliesDefaultWhenEmpty(t *testing.T) {
	// Codex CLI 场景：无 instructions 时补充默认值

	reqBody := map[string]any{
		"model": "gpt-5.1",
		// 没有 instructions 字段
REDACTED

	result := applyCodexOAuthTransform(reqBody, true, false) // isCodexCLI=true

	instructions, ok := reqBody["instructions"].(string)
	require.True(t, ok)
	require.NotEmpty(t, instructions)
	require.True(t, result.Modified)
REDACTED

func TestApplyCodexOAuthTransform_GPT55SuppliesModelSpecificInstructions(t *testing.T) {
	reqBody := map[string]any{
		"model":        "gpt-5.5",
		"instructions": "   ",
REDACTED

	result := applyCodexOAuthTransform(reqBody, true, false)

	instructions, ok := reqBody["instructions"].(string)
	require.True(t, ok)
	require.Contains(t, instructions, "You are Codex, a coding agent based on GPT-5")
	require.NotContains(t, instructions, "You are GPT-5.1 running in the Codex CLI")
	require.True(t, result.Modified)
REDACTED

func TestApplyCodexOAuthTransform_NonCodexCLI_PreservesExistingInstructions(t *testing.T) {
	// 非 Codex CLI 场景：已有 instructions 时保留客户端的值，不再覆盖

	reqBody := map[string]any{
		"model":        "gpt-5.1",
		"instructions": "old instructions",
REDACTED

	applyCodexOAuthTransform(reqBody, false, false) // isCodexCLI=false

	instructions, ok := reqBody["instructions"].(string)
	require.True(t, ok)
	require.Equal(t, "old instructions", instructions)
REDACTED

func TestApplyCodexOAuthTransform_StringInputConvertedToArray(t *testing.T) {
	reqBody := map[string]any{"model": "gpt-5.4", "input": "Hello, world!"REDACTED
	result := applyCodexOAuthTransform(reqBody, false, false)
	require.True(t, result.Modified)
	input, ok := reqBody["input"].([]any)
	require.True(t, ok)
	require.Len(t, input, 1)
	msg, ok := input[0].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "message", msg["type"])
	require.Equal(t, "user", msg["role"])
	require.Equal(t, "Hello, world!", msg["content"])
REDACTED

func TestApplyCodexOAuthTransform_EmptyStringInputBecomesEmptyArray(t *testing.T) {
	reqBody := map[string]any{"model": "gpt-5.4", "input": ""REDACTED
	result := applyCodexOAuthTransform(reqBody, false, false)
	require.True(t, result.Modified)
	input, ok := reqBody["input"].([]any)
	require.True(t, ok)
	require.Len(t, input, 0)
REDACTED

func TestApplyCodexOAuthTransform_WhitespaceStringInputBecomesEmptyArray(t *testing.T) {
	reqBody := map[string]any{"model": "gpt-5.4", "input": "   "REDACTED
	result := applyCodexOAuthTransform(reqBody, false, false)
	require.True(t, result.Modified)
	input, ok := reqBody["input"].([]any)
	require.True(t, ok)
	require.Len(t, input, 0)
REDACTED

func TestApplyCodexOAuthTransform_StringInputWithToolsField(t *testing.T) {
	reqBody := map[string]any{
		"model": "gpt-5.4",
		"input": "Run the tests",
		"tools": []any{map[string]any{"type": "function", "name": "bash"REDACTEDREDACTED,
REDACTED
	applyCodexOAuthTransform(reqBody, false, false)
	input, ok := reqBody["input"].([]any)
	require.True(t, ok)
	require.Len(t, input, 1)
REDACTED

func TestExtractSystemMessagesFromInput(t *testing.T) {
	t.Run("no system messages", func(t *testing.T) {
		reqBody := map[string]any{
			"input": []any{
				map[string]any{"role": "user", "content": "hello"REDACTED,
		REDACTED,
	REDACTED
		result := extractSystemMessagesFromInput(reqBody)
		require.False(t, result)
		input, ok := reqBody["input"].([]any)
		require.True(t, ok)
		require.Len(t, input, 1)
		_, hasInstructions := reqBody["instructions"]
		require.False(t, hasInstructions)
REDACTED)

	t.Run("string content system message", func(t *testing.T) {
		reqBody := map[string]any{
			"input": []any{
				map[string]any{"role": "system", "content": "You are an assistant."REDACTED,
				map[string]any{"role": "user", "content": "hello"REDACTED,
		REDACTED,
	REDACTED
		result := extractSystemMessagesFromInput(reqBody)
		require.True(t, result)
		input, ok := reqBody["input"].([]any)
		require.True(t, ok)
		require.Len(t, input, 2)
		msg, ok := input[0].(map[string]any)
		require.True(t, ok)
		require.Equal(t, "developer", msg["role"])
		require.Equal(t, "You are an assistant.", msg["content"])
		user, ok := input[1].(map[string]any)
		require.True(t, ok)
		require.Equal(t, "user", user["role"])
		require.Equal(t, "You are an assistant.", reqBody["instructions"])
REDACTED)

	t.Run("array content system message", func(t *testing.T) {
		reqBody := map[string]any{
			"input": []any{
				map[string]any{
					"role": "system",
					"content": []any{
						map[string]any{"type": "text", "text": "Be helpful."REDACTED,
				REDACTED,
			REDACTED,
		REDACTED,
	REDACTED
		result := extractSystemMessagesFromInput(reqBody)
		require.True(t, result)
		require.Equal(t, "Be helpful.", reqBody["instructions"])
		input, ok := reqBody["input"].([]any)
		require.True(t, ok)
		require.Len(t, input, 1)
		msg, ok := input[0].(map[string]any)
		require.True(t, ok)
		require.Equal(t, "developer", msg["role"])
		require.Equal(t, []any{
			map[string]any{"type": "text", "text": "Be helpful."REDACTED,
	REDACTED, msg["content"])
		require.Equal(t, "Be helpful.", reqBody["instructions"])
REDACTED)

	t.Run("multiple system messages concatenated", func(t *testing.T) {
		reqBody := map[string]any{
			"input": []any{
				map[string]any{"role": "system", "content": "First."REDACTED,
				map[string]any{"role": "system", "content": "Second."REDACTED,
				map[string]any{"role": "user", "content": "hi"REDACTED,
		REDACTED,
	REDACTED
		result := extractSystemMessagesFromInput(reqBody)
		require.True(t, result)
		require.Equal(t, "First.\n\nSecond.", reqBody["instructions"])
		input, ok := reqBody["input"].([]any)
		require.True(t, ok)
		require.Len(t, input, 3)
		first, ok := input[0].(map[string]any)
		require.True(t, ok)
		require.Equal(t, "developer", first["role"])
		second, ok := input[1].(map[string]any)
		require.True(t, ok)
		require.Equal(t, "developer", second["role"])
		user, ok := input[2].(map[string]any)
		require.True(t, ok)
		require.Equal(t, "user", user["role"])
		require.Equal(t, "First.\n\nSecond.", reqBody["instructions"])
REDACTED)

	t.Run("mixed system and non-system preserves non-system", func(t *testing.T) {
		reqBody := map[string]any{
			"input": []any{
				map[string]any{"role": "user", "content": "hello"REDACTED,
				map[string]any{"role": "system", "content": "Sys prompt."REDACTED,
				map[string]any{"role": "assistant", "content": "Hi there"REDACTED,
		REDACTED,
	REDACTED
		result := extractSystemMessagesFromInput(reqBody)
		require.True(t, result)
		input, ok := reqBody["input"].([]any)
		require.True(t, ok)
		require.Len(t, input, 3)
		first, ok := input[0].(map[string]any)
		require.True(t, ok)
		require.Equal(t, "user", first["role"])
		second, ok := input[1].(map[string]any)
		require.True(t, ok)
		require.Equal(t, "developer", second["role"])
		third, ok := input[2].(map[string]any)
		require.True(t, ok)
		require.Equal(t, "assistant", third["role"])
		require.Equal(t, "Sys prompt.", reqBody["instructions"])
REDACTED)

	t.Run("existing instructions prepended", func(t *testing.T) {
		reqBody := map[string]any{
			"input": []any{
				map[string]any{"role": "system", "content": "Extracted."REDACTED,
				map[string]any{"role": "user", "content": "hi"REDACTED,
		REDACTED,
			"instructions": "Existing instructions.",
	REDACTED
		result := extractSystemMessagesFromInput(reqBody)
		require.True(t, result)
		require.Equal(t, "Extracted.\n\nExisting instructions.", reqBody["instructions"])
		input, ok := reqBody["input"].([]any)
		require.True(t, ok)
		msg, ok := input[0].(map[string]any)
		require.True(t, ok)
		require.Equal(t, "developer", msg["role"])
REDACTED)
REDACTED

// TestApplyCodexOAuthTransform_StripsPromptCacheRetention is a regression
// test: some clients (e.g. Cursor cloud via the Responses-shape compat path)
// send prompt_cache_retention, but the ChatGPT internal Codex endpoint
// rejects it with "Unsupported parameter: prompt_cache_retention".
func TestApplyCodexOAuthTransform_StripsPromptCacheRetention(t *testing.T) {
	reqBody := map[string]any{
		"model":                  "gpt-5.1",
		"prompt_cache_retention": "24h",
		"input": []any{
			map[string]any{"role": "user", "content": "hi"REDACTED,
	REDACTED,
REDACTED

	applyCodexOAuthTransform(reqBody, false, false)

	_, stillThere := reqBody["prompt_cache_retention"]
	require.False(t, stillThere,
		"prompt_cache_retention must be stripped before forwarding to Codex upstream")
REDACTED

func TestApplyCodexOAuthTransform_StripsChatGPTInternalUnsupportedFields(t *testing.T) {
	reqBody := map[string]any{
		"model":                  "gpt-5.4",
		"user":                   "user_123",
		"metadata":               map[string]any{"trace_id": "abc"REDACTED,
		"prompt_cache_retention": "24h",
		"safety_identifier":      "sid",
		"stream_options":         map[string]any{"include_usage": trueREDACTED,
		"input": []any{
			map[string]any{"role": "user", "content": "hi"REDACTED,
	REDACTED,
REDACTED

	result := applyCodexOAuthTransform(reqBody, true, false)

	require.True(t, result.Modified)
	for _, field := range openAIChatGPTInternalUnsupportedFields {
		require.NotContains(t, reqBody, field)
REDACTED
REDACTED

func TestApplyCodexOAuthTransform_ExtractsSystemMessages(t *testing.T) {
	reqBody := map[string]any{
		"model": "gpt-5.1",
		"input": []any{
			map[string]any{"role": "system", "content": "You are a coding assistant."REDACTED,
			map[string]any{"role": "user", "content": "Write a function."REDACTED,
	REDACTED,
REDACTED

	result := applyCodexOAuthTransform(reqBody, false, false)

	require.True(t, result.Modified)

	input, ok := reqBody["input"].([]any)
	require.True(t, ok)
	require.Len(t, input, 2)
	system, ok := input[0].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "developer", system["role"])
	require.Equal(t, "You are a coding assistant.", system["content"])
	user, ok := input[1].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "user", user["role"])
	require.Equal(t, "You are a coding assistant.", reqBody["instructions"])
REDACTED

func TestApplyCodexOAuthTransform_JsonObjectKeepsJsonInstructionInInput(t *testing.T) {
	reqBody := map[string]any{
		"model": "gpt-5.4",
		"input": []any{
			map[string]any{
				"role":    "system",
				"content": "You are an assistant. Output JSON only.",
		REDACTED,
			map[string]any{
				"role":    "user",
				"content": "symbol data without the keyword",
		REDACTED,
	REDACTED,
		"text": map[string]any{
			"format": map[string]any{
				"type": "json_object",
		REDACTED,
	REDACTED,
REDACTED

	result := applyCodexOAuthTransform(reqBody, false, false)

	require.True(t, result.Modified)
	instructions, ok := reqBody["instructions"].(string)
	require.True(t, ok)
	require.Contains(t, instructions, "JSON")
	input, ok := reqBody["input"].([]any)
	require.True(t, ok)
	require.Len(t, input, 2)
	developer, ok := input[0].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "developer", developer["role"])
	require.Contains(t, developer["content"], "JSON")
	user, ok := input[1].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "user", user["role"])
REDACTED

func TestIsInstructionsEmpty(t *testing.T) {
	tests := []struct {
		name     string
		reqBody  map[string]any
		expected bool
REDACTED{
		{"missing field", map[string]any{REDACTED, trueREDACTED,
		{"nil value", map[string]any{"instructions": nilREDACTED, trueREDACTED,
		{"empty string", map[string]any{"instructions": ""REDACTED, trueREDACTED,
		{"whitespace only", map[string]any{"instructions": "   "REDACTED, trueREDACTED,
		{"non-string", map[string]any{"instructions": 123REDACTED, trueREDACTED,
		{"valid string", map[string]any{"instructions": "hello"REDACTED, falseREDACTED,
REDACTED

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isInstructionsEmpty(tt.reqBody)
			require.Equal(t, tt.expected, result)
	REDACTED)
REDACTED
REDACTED

// TestFilterCodexInput_PreservesReasoningStripsID covers the core OAuth-path
// reasoning contract (replaces the earlier "drops reasoning" test, whose
// premise was wrong). A reasoning item carrying encrypted_content is the
// official channel for replaying reasoning context across turns under
// store=false, so it must survive the filter with encrypted_content intact;
// only its rs_* id is stripped (always, independent of PreserveReferences)
// because a bare rs_* id replayed under store=false 404s upstream. Contracts
// 1/2/3, verified end-to-end against chatgpt.com codex (gpt-5.5). See issue
// #1957.
func TestFilterCodexInput_PreservesReasoningStripsID(t *testing.T) {
	build := func() []any {
		return []any{
			map[string]any{
				"type":              "reasoning",
				"id":                "rs_0672f12450da0b9c0169f07220a6c08198b68c2455ced99344",
				"encrypted_content": "gAAAAAB-enc-payload",
				"summary":           []any{REDACTED,
		REDACTED,
	REDACTED
REDACTED

	for _, preserve := range []bool{true, falseREDACTED {
		preserve := preserve
		t.Run(fmt.Sprintf("preserveReferences=%v", preserve), func(t *testing.T) {
			filtered := filterCodexInput(build(), preserve)
			require.Len(t, filtered, 1)

			item, ok := filtered[0].(map[string]any)
			require.True(t, ok)
			// Contract 2: the reasoning item survives the filter.
			require.Equal(t, "reasoning", item["type"])
			// Contract 2: encrypted_content (cross-turn channel) preserved verbatim.
			require.Equal(t, "gAAAAAB-enc-payload", item["encrypted_content"])
			// Contract 1/3: rs_* id stripped unconditionally, even when
			// PreserveReferences=true (id lookup, not the item, triggers the 404).
			_, hasID := item["id"]
			require.False(t, hasID)
			// summary passed through untouched.
			summary, ok := item["summary"].([]any)
			require.True(t, ok)
			require.Len(t, summary, 0)
	REDACTED)
REDACTED
REDACTED

// TestFilterCodexInput_BareReasoningStripsIDBackfillsSummary covers contract 1
// plus 5: a reasoning item carrying only an rs_* id (no encrypted_content) is
// kept as an empty shell with the id stripped, and a missing summary is
// backfilled to [] so upstream does not reject it with 400 "Missing required
// parameter 'input[N].summary'". Verified against chatgpt.com codex (gpt-5.5).
func TestFilterCodexInput_BareReasoningStripsIDBackfillsSummary(t *testing.T) {
	input := []any{
		map[string]any{
			"type": "reasoning",
			"id":   "rs_0672f12450da0b9c0169f07220a6c08198b68c2455ced99344",
	REDACTED,
REDACTED

	filtered := filterCodexInput(input, false)
	require.Len(t, filtered, 1)

	item, ok := filtered[0].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "reasoning", item["type"])
	// Contract 1: id stripped.
	_, hasID := item["id"]
	require.False(t, hasID)
	// Contract 5: summary backfilled to an empty array.
	summary, ok := item["summary"].([]any)
	require.True(t, ok)
	require.Len(t, summary, 0)
REDACTED

// TestFilterCodexInput_ReasoningBackfillsMissingSummary isolates contract 5:
// even when a reasoning item carries other content (here encrypted_content),
// a missing summary field is always added as [] before forwarding upstream.
func TestFilterCodexInput_ReasoningBackfillsMissingSummary(t *testing.T) {
	input := []any{
		map[string]any{
			"type":              "reasoning",
			"id":                "rs_abc",
			"encrypted_content": "gAAAAAB-enc",
	REDACTED,
REDACTED

	filtered := filterCodexInput(input, false)
	require.Len(t, filtered, 1)

	item, ok := filtered[0].(map[string]any)
	require.True(t, ok)
	summary, ok := item["summary"].([]any)
	require.True(t, ok)
	require.Len(t, summary, 0)
	// encrypted_content still preserved alongside the backfilled summary.
	require.Equal(t, "gAAAAAB-enc", item["encrypted_content"])
REDACTED

// TestFilterCodexInput_PreservesReasoningSummaryAndContent verifies that a
// non-empty summary is not overwritten and that arbitrary reasoning fields
// (e.g. content) survive verbatim — only the id is removed.
func TestFilterCodexInput_PreservesReasoningSummaryAndContent(t *testing.T) {
	summary := []any{
		map[string]any{"type": "summary_text", "text": "Considered the options."REDACTED,
REDACTED
	content := []any{
		map[string]any{"type": "reasoning_text", "text": "internal chain"REDACTED,
REDACTED
	input := []any{
		map[string]any{
			"type":              "reasoning",
			"id":                "rs_abc",
			"summary":           summary,
			"content":           content,
			"encrypted_content": "gAAAAAB-enc",
	REDACTED,
REDACTED

	filtered := filterCodexInput(input, false)
	require.Len(t, filtered, 1)

	item, ok := filtered[0].(map[string]any)
	require.True(t, ok)
	// Non-empty summary preserved verbatim (not replaced with []).
	require.Equal(t, summary, item["summary"])
	// content preserved verbatim.
	require.Equal(t, content, item["content"])
	require.Equal(t, "gAAAAAB-enc", item["encrypted_content"])
	_, hasID := item["id"]
	require.False(t, hasID)
REDACTED

// TestFilterCodexInput_PreservesReasoningInMixedInput exercises contract 7:
// reasoning items are stripped of their rs_* ids but kept (with
// encrypted_content) while message / function_call / function_call_output
// items flow through unchanged, with tool-call pairing (call_id) intact.
func TestFilterCodexInput_PreservesReasoningInMixedInput(t *testing.T) {
	build := func() []any {
		return []any{
			map[string]any{"type": "message", "id": "msg_0", "role": "user", "content": "hi"REDACTED,
			map[string]any{
				"type":              "reasoning",
				"id":                "rs_1",
				"encrypted_content": "gAAAAAB-enc-1",
				"summary":           []any{REDACTED,
		REDACTED,
			map[string]any{
				"type":    "reasoning",
				"id":      "rs_2",
				"summary": []any{REDACTED,
		REDACTED,
			// call_id already in fc_ form so the unrelated call_->fc_
			// normalization does not obscure the pairing assertion.
			map[string]any{"type": "function_call", "id": "fc_1", "call_id": "fc_1", "name": "tool", "arguments": "{REDACTED"REDACTED,
			map[string]any{"type": "function_call_output", "call_id": "fc_1", "output": "{REDACTED"REDACTED,
	REDACTED
REDACTED

	for _, preserve := range []bool{true, falseREDACTED {
		preserve := preserve
		t.Run(fmt.Sprintf("preserveReferences=%v", preserve), func(t *testing.T) {
			filtered := filterCodexInput(build(), preserve)
			// Nothing is dropped: both reasoning items are now preserved.
			require.Len(t, filtered, 5)

			byType := make(map[string][]map[string]any)
			for _, raw := range filtered {
				item, ok := raw.(map[string]any)
				require.True(t, ok)
				typ, _ := item["type"].(string)
				byType[typ] = append(byType[typ], item)
				// No surviving item may carry an rs_* id.
				if id, ok := item["id"].(string); ok {
					require.False(t, strings.HasPrefix(id, "rs_"),
						"no item carrying an rs_* id should survive the filter")
			REDACTED
		REDACTED

			// Both reasoning items kept, ids stripped, summary present.
			require.Len(t, byType["reasoning"], 2)
			for _, r := range byType["reasoning"] {
				_, hasID := r["id"]
				require.False(t, hasID)
				_, hasSummary := r["summary"]
				require.True(t, hasSummary)
		REDACTED
			require.Equal(t, "gAAAAAB-enc-1", byType["reasoning"][0]["encrypted_content"])

			// message / function_call(+output) untouched by reasoning handling.
			require.Len(t, byType["message"], 1)
			// Contract 7: tool-call pairing by call_id is unaffected.
			require.Len(t, byType["function_call"], 1)
			require.Equal(t, "fc_1", byType["function_call"][0]["call_id"])
			require.Len(t, byType["function_call_output"], 1)
			require.Equal(t, "fc_1", byType["function_call_output"][0]["call_id"])
	REDACTED)
REDACTED
REDACTED
