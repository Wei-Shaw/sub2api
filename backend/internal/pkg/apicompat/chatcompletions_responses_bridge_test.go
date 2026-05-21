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

func chatMessageRoles(messages []ChatMessage) []string {
	roles := make([]string, 0, len(messages))
	for _, message := range messages {
		roles = append(roles, message.Role)
REDACTED
	return roles
REDACTED
