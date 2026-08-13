package apicompat

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestChatCompletionsToResponsesPreservesXSearchTool(t *testing.T) {
	enabled := true
	req := &ChatCompletionsRequest{
		Model: "grok-4.5",
		Messages: []ChatMessage{
			{Role: "user", Content: json.RawMessage(`"latest xAI post"`)REDACTED,
	REDACTED,
		Tools: []ChatTool{{
			Type:                     "x_search",
			AllowedXHandles:          []string{"xai"REDACTED,
			ExcludedXHandles:         []string{"spam"REDACTED,
			FromDate:                 "2026-08-01",
			ToDate:                   "2026-08-10",
			EnableImageUnderstanding: &enabled,
			EnableVideoUnderstanding: &enabled,
REDACTED
		ToolChoice: json.RawMessage(`{"type":"x_search"REDACTED`),
REDACTED

	resp, err := ChatCompletionsToResponses(req)
REDACTED
	require.Len(t, resp.Tools, 1)
	require.Equal(t, "x_search", resp.Tools[0].Type)
	require.Equal(t, []string{"xai"REDACTED, resp.Tools[0].AllowedXHandles)
	require.Equal(t, []string{"spam"REDACTED, resp.Tools[0].ExcludedXHandles)
	require.Equal(t, "2026-08-01", resp.Tools[0].FromDate)
	require.Equal(t, "2026-08-10", resp.Tools[0].ToDate)
	require.NotNil(t, resp.Tools[0].EnableImageUnderstanding)
	require.True(t, *resp.Tools[0].EnableImageUnderstanding)
	require.NotNil(t, resp.Tools[0].EnableVideoUnderstanding)
	require.True(t, *resp.Tools[0].EnableVideoUnderstanding)
	require.JSONEq(t, `{"type":"x_search"REDACTED`, string(resp.ToolChoice))
REDACTED

func TestResponsesToChatCompletionsPreservesXSearchTool(t *testing.T) {
	enabled := true
	req := &ResponsesRequest{
		Model: "grok-4.5",
		Input: json.RawMessage(`"latest xAI post"`),
		Tools: []ResponsesTool{{
			Type:                     "x_search",
			AllowedXHandles:          []string{"xai"REDACTED,
			ExcludedXHandles:         []string{"spam"REDACTED,
			FromDate:                 "2026-08-01",
			ToDate:                   "2026-08-10",
			EnableImageUnderstanding: &enabled,
			EnableVideoUnderstanding: &enabled,
REDACTED
		ToolChoice: json.RawMessage(`{"type":"x_search"REDACTED`),
REDACTED

	chat, err := ResponsesToChatCompletionsRequest(req)
REDACTED
	require.Len(t, chat.Tools, 1)
	require.Equal(t, "x_search", chat.Tools[0].Type)
	require.Equal(t, []string{"xai"REDACTED, chat.Tools[0].AllowedXHandles)
	require.Equal(t, []string{"spam"REDACTED, chat.Tools[0].ExcludedXHandles)
	require.JSONEq(t, `{"type":"x_search"REDACTED`, string(chat.ToolChoice))
REDACTED

func TestResponsesToChatCompletionsXSearchToolChoiceString(t *testing.T) {
	chat, err := ResponsesToChatCompletionsRequest(&ResponsesRequest{
		Model:      "grok-4.5",
		Input:      json.RawMessage(`"latest xAI post"`),
		Tools:      []ResponsesTool{{Type: "x_search"REDACTEDREDACTED,
		ToolChoice: json.RawMessage(`"x_search"`),
REDACTED)
REDACTED
	require.JSONEq(t, `"x_search"`, string(chat.ToolChoice))
REDACTED
