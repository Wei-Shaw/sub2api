package apicompat

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

const (
	testToolImageDataURL = "data:image/png;base64,AQID"
	testToolImageRemote  = "https://example.com/tool-output.png"
)

func TestResponsesToolOutputMedia_ExtractsSupportedShapes(t *testing.T) {
	tests := []struct {
		name       string
		call       string
		outputType string
		output     string
		imageURL   string
		toolText   string
REDACTED{
		{
			name:       "image-only array",
			call:       `{"type":"function_call","call_id":"call_image","name":"view_image","arguments":"{REDACTED"REDACTED`,
			outputType: "function_call_output",
			output:     `[{"type":"input_image","image_url":"data:image/png;base64,AQID"REDACTED]`,
			imageURL:   testToolImageDataURL,
	REDACTED,
		{
			name:       "text and nested image URL",
			call:       `{"type":"function_call","call_id":"call_image","name":"view_image","arguments":"{REDACTED"REDACTED`,
			outputType: "function_call_output",
			output:     `[{"type":"input_text","text":"render complete"REDACTED,{"type":"image_url","image_url":{"url":"https://example.com/tool-output.png"REDACTEDREDACTED]`,
			imageURL:   testToolImageRemote,
			toolText:   "render complete",
	REDACTED,
		{
			name:       "top-level image object",
			call:       `{"type":"function_call","call_id":"call_image","name":"view_image","arguments":"{REDACTED"REDACTED`,
			outputType: "function_call_output",
			output:     `{"type":"input_image","image_url":"data:image/png;base64,AQID"REDACTED`,
			imageURL:   testToolImageDataURL,
	REDACTED,
		{
			name:       "content wrapper",
			call:       `{"type":"function_call","call_id":"call_image","name":"view_image","arguments":"{REDACTED"REDACTED`,
			outputType: "function_call_output",
			output:     `{"status":"ok","content":[{"type":"input_image","image_url":"data:image/png;base64,AQID"REDACTED],"unknown":{"score":0.9,"large":9007199254740993REDACTEDREDACTED`,
			imageURL:   testToolImageDataURL,
			toolText:   `"large":9007199254740993`,
	REDACTED,
		{
			name:       "JSON string output",
			call:       `{"type":"function_call","call_id":"call_image","name":"view_image","arguments":"{REDACTED"REDACTED`,
			outputType: "function_call_output",
			output:     `"[{\"type\":\"input_image\",\"image_url\":\"data:image/png;base64,AQID\"REDACTED]"`,
			imageURL:   testToolImageDataURL,
	REDACTED,
		{
			name:       "bare data URL",
			call:       `{"type":"function_call","call_id":"call_image","name":"view_image","arguments":"{REDACTED"REDACTED`,
			outputType: "function_call_output",
			output:     `"data:image/png;base64,AQID"`,
			imageURL:   testToolImageDataURL,
	REDACTED,
		{
			name:       "custom tool output",
			call:       `{"type":"custom_tool_call","call_id":"call_image","name":"view_image","input":"{REDACTED"REDACTED`,
			outputType: "custom_tool_call_output",
			output:     `[{"type":"input_image","image_url":"data:image/png;base64,AQID"REDACTED]`,
			imageURL:   testToolImageDataURL,
	REDACTED,
		{
			name:       "tool search output",
			call:       `{"type":"tool_search_call","call_id":"call_image","arguments":{"query":"image"REDACTEDREDACTED`,
			outputType: "tool_search_output",
			output:     `[{"type":"image_url","image_url":{"url":"https://example.com/tool-output.png"REDACTEDREDACTED]`,
			imageURL:   testToolImageRemote,
	REDACTED,
REDACTED

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			input := fmt.Sprintf(`[%s,{"type":%q,"call_id":"call_image","output":%sREDACTED]`, tt.call, tt.outputType, tt.output)
			messages := convertToolOutputMedia(t, input)

			require.Len(t, messages, 3)
			require.Equal(t, []string{"assistant", "tool", "user"REDACTED, chatMessageRoles(messages))
			require.Equal(t, "call_image", messages[1].ToolCallID)

			toolText := chatToolContentString(t, messages[1])
			require.Contains(t, toolText, "[Tool output media moved to the following user message]")
			require.NotContains(t, toolText, tt.imageURL)
			if tt.toolText != "" {
				require.Contains(t, toolText, tt.toolText)
		REDACTED

			parts := chatContentParts(t, messages[2])
			require.Len(t, parts, 2)
			require.Equal(t, "text", parts[0].Type)
			require.Equal(t, "[Tool output media for call call_image]", parts[0].Text)
			require.Equal(t, "image_url", parts[1].Type)
			require.NotNil(t, parts[1].ImageURL)
			require.Equal(t, tt.imageURL, parts[1].ImageURL.URL)
	REDACTED)
REDACTED
REDACTED

func TestResponsesToolOutputMedia_ParallelBatchUsesCallOrder(t *testing.T) {
	messages := convertToolOutputMedia(t, `[
		{"type":"function_call","call_id":"call_A","name":"view_image","arguments":"{REDACTED"REDACTED,
		{"type":"function_call","call_id":"call_B","name":"view_image","arguments":"{REDACTED"REDACTED,
		{"type":"function_call_output","call_id":"call_B","output":[{"type":"input_image","image_url":{"url":"https://example.com/b.png"REDACTEDREDACTED]REDACTED,
		{"type":"function_call_output","call_id":"call_A","output":[{"type":"input_image","image_url":{"url":"https://example.com/a.png"REDACTEDREDACTED]REDACTED
	]`)

	require.Len(t, messages, 4)
	require.Equal(t, []string{"assistant", "tool", "tool", "user"REDACTED, chatMessageRoles(messages))
	require.Equal(t, []string{"call_A", "call_B"REDACTED, []string{messages[1].ToolCallID, messages[2].ToolCallIDREDACTED)

	parts := chatContentParts(t, messages[3])
	require.Len(t, parts, 4)
	require.Equal(t, "[Tool output media for call call_A]", parts[0].Text)
	require.Equal(t, "https://example.com/a.png", parts[1].ImageURL.URL)
	require.Equal(t, "[Tool output media for call call_B]", parts[2].Text)
	require.Equal(t, "https://example.com/b.png", parts[3].ImageURL.URL)
REDACTED

func TestResponsesToolOutputMedia_PreservesRichSiblingWhenRewriting(t *testing.T) {
	messages := convertToolOutputMedia(t, `[
		{"type":"function_call","call_id":"call_image","name":"view_image","arguments":"{REDACTED"REDACTED,
		{"type":"function_call_output","call_id":"call_image","output":[
			{"type":"result","url":"https://example.com/result","score":0.9,"text":"complete","extra":{"count":2REDACTEDREDACTED,
			{"type":"input_image","image_url":"data:image/png;base64,AQID"REDACTED
		]REDACTED
	]`)

	toolText := chatToolContentString(t, messages[1])
	require.JSONEq(t, `[
		{"type":"result","url":"https://example.com/result","score":0.9,"text":"complete","extra":{"count":2REDACTEDREDACTED,
		{"type":"input_text","text":"[Tool output media moved to the following user message]"REDACTED
	]`, toolText)
REDACTED

func TestResponsesToolOutputMedia_InterleavedMessagesFollowMediaBatch(t *testing.T) {
	messages := convertToolOutputMedia(t, `[
		{"type":"function_call","call_id":"call_A","name":"view_image","arguments":"{REDACTED"REDACTED,
		{"type":"message","role":"developer","content":[{"type":"input_text","text":"approval saved"REDACTED]REDACTED,
		{"type":"message","role":"user","content":[{"type":"input_text","text":"continue"REDACTED]REDACTED,
		{"type":"function_call_output","call_id":"call_A","output":[{"type":"input_image","image_url":"data:image/png;base64,AQID"REDACTED]REDACTED
	]`)

	require.Equal(t, []string{"assistant", "tool", "user", "system", "user"REDACTED, chatMessageRoles(messages))
	require.Equal(t, "[Tool output media for call call_A]", chatContentParts(t, messages[2])[0].Text)
	require.JSONEq(t, `"approval saved"`, string(messages[3].Content))
	require.JSONEq(t, `"continue"`, string(messages[4].Content))
REDACTED

func TestResponsesToolOutputMedia_DropsOrphanAndUnansweredCallMedia(t *testing.T) {
	t.Run("orphan", func(t *testing.T) {
		messages := convertToolOutputMedia(t, `[
			{"type":"function_call_output","call_id":"call_ghost","output":[{"type":"input_image","image_url":"data:image/png;base64,AQID"REDACTED]REDACTED
		]`)
		require.Empty(t, messages)
REDACTED)

	t.Run("unanswered parallel call", func(t *testing.T) {
		messages := convertToolOutputMedia(t, `[
			{"type":"function_call","call_id":"call_A","name":"view_image","arguments":"{REDACTED"REDACTED,
			{"type":"function_call","call_id":"call_B","name":"view_image","arguments":"{REDACTED"REDACTED,
			{"type":"function_call_output","call_id":"call_A","output":[{"type":"input_image","image_url":"data:image/png;base64,AQID"REDACTED]REDACTED
		]`)

		require.Len(t, messages, 3)
		require.Len(t, messages[0].ToolCalls, 1)
		require.Equal(t, "call_A", messages[0].ToolCalls[0].ID)
		parts := chatContentParts(t, messages[2])
		require.Len(t, parts, 2)
		require.NotContains(t, string(messages[2].Content), "call_B")
REDACTED)
REDACTED

func TestResponsesToolOutputMedia_PreservesMediaFreeOutputBytes(t *testing.T) {
	tests := []struct {
		name   string
		output string
REDACTED{
		{
			name:   "rich unknown object",
			output: `{"type":"result","url":"https://example.com/result","score":0.9,"text":"complete","extra":{"count":2REDACTEDREDACTED`,
	REDACTED,
		{
			name:   "no-image array",
			output: `[ { "type": "input_text", "text": "ok" REDACTED, {"unknown":trueREDACTED ]`,
	REDACTED,
		{
			name:   "plain string",
			output: `"plain output"`,
	REDACTED,
		{
			name:   "JSON string without image",
			output: `"{ \"ok\": true REDACTED"`,
	REDACTED,
		{
			name:   "embedded data URL text",
			output: `"prefix data:image/png;base64,AQID suffix"`,
	REDACTED,
REDACTED

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			input := fmt.Sprintf(`[
				{"type":"function_call","call_id":"call_text","name":"exec","arguments":"{REDACTED"REDACTED,
				{"type":"function_call_output","call_id":"call_text","output":%sREDACTED
			]`, tt.output)
			messages := convertToolOutputMedia(t, input)
			require.Len(t, messages, 2)

			var expected string
			if err := json.Unmarshal([]byte(tt.output), &expected); err != nil {
				expected = tt.output
		REDACTED
			expectedContent, err := json.Marshal(expected)
		REDACTED
			require.Equal(t, string(expectedContent), string(messages[1].Content))
	REDACTED)
REDACTED
REDACTED

func TestResponsesToolOutputMedia_DuplicateCallIDIsLastWins(t *testing.T) {
	t.Run("later media replaces earlier media", func(t *testing.T) {
		messages := convertToolOutputMedia(t, `[
			{"type":"function_call","call_id":"call_image","name":"view_image","arguments":"{REDACTED"REDACTED,
			{"type":"function_call_output","call_id":"call_image","output":[{"type":"input_image","image_url":{"url":"https://example.com/first.png"REDACTEDREDACTED]REDACTED,
			{"type":"function_call_output","call_id":"call_image","output":[{"type":"input_image","image_url":{"url":"https://example.com/last.png"REDACTEDREDACTED]REDACTED
		]`)

		require.Len(t, messages, 3)
		require.NotContains(t, string(messages[1].Content), "first.png")
		require.NotContains(t, string(messages[2].Content), "first.png")
		require.Contains(t, string(messages[2].Content), "last.png")
REDACTED)

	t.Run("later text clears earlier media", func(t *testing.T) {
		messages := convertToolOutputMedia(t, `[
			{"type":"function_call","call_id":"call_image","name":"view_image","arguments":"{REDACTED"REDACTED,
			{"type":"function_call_output","call_id":"call_image","output":[{"type":"input_image","image_url":"data:image/png;base64,AQID"REDACTED]REDACTED,
			{"type":"function_call_output","call_id":"call_image","output":"latest text"REDACTED
		]`)

		require.Len(t, messages, 2)
		require.Equal(t, "latest text", chatToolContentString(t, messages[1]))
REDACTED)
REDACTED

func TestResponsesToChatCompletionsRequest_ToolContentNeverContainsExtractedMedia(t *testing.T) {
	req := &ResponsesRequest{
		Model: "vision-model",
		Input: json.RawMessage(`[
			{"type":"function_call","call_id":"call_function","name":"view_image","arguments":"{REDACTED"REDACTED,
			{"type":"custom_tool_call","call_id":"call_custom","name":"custom_image","input":"{REDACTED"REDACTED,
			{"type":"tool_search_call","call_id":"call_search","arguments":{"query":"image"REDACTEDREDACTED,
			{"type":"function_call_output","call_id":"call_function","output":[{"type":"input_image","image_url":"data:image/png;base64,AQID"REDACTED]REDACTED,
			{"type":"custom_tool_call_output","call_id":"call_custom","output":{"content":[{"type":"image_url","image_url":{"url":"https://example.com/custom.png"REDACTEDREDACTED]REDACTEDREDACTED,
			{"type":"tool_search_output","call_id":"call_search","output":"data:image/jpeg;base64,BAUG"REDACTED
		]`),
REDACTED

	out, err := ResponsesToChatCompletionsRequest(req)
REDACTED
	assertChatInvariants(t, out.Messages)

	var toolCount int
	for _, message := range out.Messages {
		if message.Role != "tool" {
			continue
	REDACTED
		toolCount++
		content := string(message.Content)
		require.NotContains(t, content, "data:image/")
		require.NotContains(t, content, "https://example.com/custom.png")
REDACTED
	require.Equal(t, 3, toolCount)
	require.Equal(t, []string{"assistant", "tool", "tool", "tool", "user"REDACTED, chatMessageRoles(out.Messages))
REDACTED

func convertToolOutputMedia(t *testing.T, input string) []ChatMessage {
REDACTED
	messages, err := responsesInputToChatMessages("", json.RawMessage(input))
REDACTED
	assertChatInvariants(t, messages)
	return messages
REDACTED

func chatToolContentString(t *testing.T, message ChatMessage) string {
REDACTED
	require.Equal(t, "tool", message.Role)
	var content string
	require.NoError(t, json.Unmarshal(message.Content, &content))
	return content
REDACTED

func chatContentParts(t *testing.T, message ChatMessage) []ChatContentPart {
REDACTED
	var parts []ChatContentPart
	require.NoError(t, json.Unmarshal(message.Content, &parts))
	for _, part := range parts {
		if part.Type == "image_url" {
			require.NotNil(t, part.ImageURL)
			require.False(t, strings.TrimSpace(part.ImageURL.URL) == "")
	REDACTED
REDACTED
	return parts
REDACTED
