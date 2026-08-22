package apicompat

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResponsesInputToChatMessages_DeveloperRoleMapsToSystem(t *testing.T) {
	messages, err := responsesInputToChatMessages("", json.RawMessage(`[{"role":"developer","content":"follow project instructions"REDACTED]`))
REDACTED
	require.Len(t, messages, 1)

	assert.Equal(t, "system", messages[0].Role)
	assert.JSONEq(t, `"follow project instructions"`, string(messages[0].Content))
REDACTED

func TestResponsesInputToChatMessages_SkipsInvalidHistoricalFunctionCall(t *testing.T) {
	input := json.RawMessage(`[
		{"type":"function_call","call_id":"call_bad","name":"exec_command","arguments":"{\"cmd\": \"ssh root@HOST"REDACTED,
		{"type":"function_call_output","call_id":"call_bad","output":"failed to parse function arguments"REDACTED,
		{"type":"function_call","call_id":"call_ok","name":"exec_command","arguments":"{REDACTED"REDACTED,
		{"type":"function_call_output","call_id":"call_ok","output":"ok"REDACTED,
		{"role":"user","content":"continue"REDACTED
	]`)

	messages, err := responsesInputToChatMessages("", input)
REDACTED
	require.Len(t, messages, 3)
	require.Equal(t, "assistant", messages[0].Role)
	require.Len(t, messages[0].ToolCalls, 1)
	require.Equal(t, "call_ok", messages[0].ToolCalls[0].ID)
	require.Equal(t, "tool", messages[1].Role)
	require.Equal(t, "call_ok", messages[1].ToolCallID)
	require.Equal(t, "user", messages[2].Role)
REDACTED

func TestResponsesInputToChatMessages_SkipsInvalidEmptyCallIDOutput(t *testing.T) {
	input := json.RawMessage(`[
		{"type":"function_call","call_id":"","name":"exec_command","arguments":"{\"cmd\": \"ssh root@HOST"REDACTED,
		{"type":"function_call_output","call_id":"","output":"failed to parse function arguments"REDACTED,
		{"role":"user","content":"continue"REDACTED
	]`)

	messages, err := responsesInputToChatMessages("", input)
REDACTED
	require.Len(t, messages, 1)
	require.Equal(t, "user", messages[0].Role)
REDACTED

func TestChatCompletionsResponseToResponses_SkipsInvalidFunctionArguments(t *testing.T) {
	resp := &ChatCompletionsResponse{
		Model: "deepseek-v4-flash",
		Choices: []ChatChoice{{
			Message: ChatMessage{
				Role: "assistant",
				ToolCalls: []ChatToolCall{
					{ID: "call_bad", Type: "function", Function: ChatFunctionCall{Name: "exec_command", Arguments: `{"cmd": "ssh root@HOST`REDACTEDREDACTED,
					{ID: "call_ok", Type: "function", Function: ChatFunctionCall{Name: "exec_command", Arguments: `{REDACTED`REDACTEDREDACTED,
			REDACTED,
		REDACTED,
			FinishReason: "length",
REDACTED
REDACTED

	out := ChatCompletionsResponseToResponses(resp, "deepseek-v4-flash", nil, false, nil)
	require.Equal(t, "incomplete", out.Status)
	require.Len(t, out.Output, 1)
	require.Equal(t, "function_call", out.Output[0].Type)
	require.Equal(t, "call_ok", out.Output[0].CallID)
	require.Equal(t, `{REDACTED`, out.Output[0].Arguments)
REDACTED

func TestResponsesInputToChatMessages_KeepsChatCompletionRoles(t *testing.T) {
	input := json.RawMessage(`[
		{"role":"system","content":"system message"REDACTED,
		{"role":"user","content":"user message"REDACTED,
		{"role":"assistant","content":"assistant message"REDACTED,
		{"role":"tool","content":"tool message"REDACTED
	]`)

	messages, err := responsesInputToChatMessages("", input)
REDACTED
	require.Len(t, messages, 4)

	assert.Equal(t, []string{"system", "user", "assistant", "tool"REDACTED, chatMessageRoles(messages))
REDACTED

func TestResponsesInputToChatMessages_EmptyRoleFallsBackToUser(t *testing.T) {
	messages, err := responsesInputToChatMessages("", json.RawMessage(`[{"role":"","content":"hello"REDACTED]`))
REDACTED
	require.Len(t, messages, 1)

	assert.Equal(t, "user", messages[0].Role)
REDACTED

func TestResponsesInputToChatMessages_DeveloperRoleTrimAndCaseInsensitive(t *testing.T) {
	input := json.RawMessage(`[
		{"role":" Developer ","content":"one"REDACTED,
		{"role":"\tDEVELOPER\n","content":"two"REDACTED
	]`)

	messages, err := responsesInputToChatMessages("", input)
REDACTED
	require.Len(t, messages, 2)

	assert.Equal(t, []string{"system", "system"REDACTED, chatMessageRoles(messages))
REDACTED

func TestResponsesToChatCompletionsRequest_InstructionsAndInputDeveloperRole(t *testing.T) {
	req := &ResponsesRequest{
		Model:        "gpt-4o",
		Instructions: "Use concise answers.",
		Input: json.RawMessage(`[
			{"role":"developer","content":[{"type":"input_text","text":"Prefer JSON."REDACTED]REDACTED,
			{"role":"user","content":"Hello"REDACTED
		]`),
REDACTED

	out, err := ResponsesToChatCompletionsRequest(req)
REDACTED
	require.Len(t, out.Messages, 3)

	assert.Equal(t, []string{"system", "system", "user"REDACTED, chatMessageRoles(out.Messages))
	assert.JSONEq(t, `"Use concise answers."`, string(out.Messages[0].Content))
	assert.JSONEq(t, `"Prefer JSON."`, string(out.Messages[1].Content))
	assert.JSONEq(t, `"Hello"`, string(out.Messages[2].Content))
REDACTED

func TestResponsesToChatCompletionsRequest_TextFormatJsonObject(t *testing.T) {
	req := &ResponsesRequest{
		Model: "gpt-4o",
		Input: json.RawMessage(`[
			{"role":"user","content":"Return JSON"REDACTED
		]`),
		Text: &ResponsesText{
			Format: json.RawMessage(`{"type":"json_object"REDACTED`),
	REDACTED,
REDACTED

	out, err := ResponsesToChatCompletionsRequest(req)
REDACTED
	assert.JSONEq(t, `{"type":"json_object"REDACTED`, string(out.ResponseFormat))
REDACTED

func TestResponsesToChatCompletionsRequest_TextFormatJsonSchema(t *testing.T) {
	req := &ResponsesRequest{
		Model: "gpt-4o",
		Input: json.RawMessage(`[
			{"role":"user","content":"Return structured JSON"REDACTED
		]`),
		Text: &ResponsesText{
			Format: json.RawMessage(`{
				"type":"json_schema",
				"name":"answer",
				"schema":{
					"type":"object",
					"properties":{"ok":{"type":"boolean"REDACTEDREDACTED,
					"required":["ok"],
					"additionalProperties":false
			REDACTED,
				"strict":true
		REDACTED`),
	REDACTED,
REDACTED

	out, err := ResponsesToChatCompletionsRequest(req)
REDACTED
	assert.JSONEq(t, `{
		"type":"json_schema",
		"json_schema":{
			"name":"answer",
			"schema":{
				"type":"object",
				"properties":{"ok":{"type":"boolean"REDACTEDREDACTED,
				"required":["ok"],
				"additionalProperties":false
		REDACTED,
			"strict":true
	REDACTED
REDACTED`, string(out.ResponseFormat))
REDACTED

func TestResponsesToChatCompletionsRequest_ParallelToolCalls(t *testing.T) {
	parallel := false
	req := &ResponsesRequest{
		Model: "gpt-4o",
		Input: json.RawMessage(`[
			{"role":"user","content":"Use tools"REDACTED
		]`),
		ParallelToolCalls: &parallel,
REDACTED

	out, err := ResponsesToChatCompletionsRequest(req)
REDACTED
	require.NotNil(t, out.ParallelToolCalls)
	assert.False(t, *out.ParallelToolCalls)

	payload, err := json.Marshal(out)
REDACTED
	assert.Contains(t, string(payload), `"parallel_tool_calls":false`)
REDACTED

func chatMessageRoles(messages []ChatMessage) []string {
	roles := make([]string, 0, len(messages))
	for _, message := range messages {
		roles = append(roles, message.Role)
REDACTED
	return roles
REDACTED
