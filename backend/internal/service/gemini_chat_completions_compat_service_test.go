package service

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestGeminiResponseToChatCompletionsPreservesInlineData(t *testing.T) {
	tests := []struct {
		name  string
		parts []any
		want  string
REDACTED{
		{
			name: "image only",
			parts: []any{
				map[string]any{"inlineData": map[string]any{"mimeType": "image/png", "data": "aW1hZ2U="REDACTEDREDACTED,
		REDACTED,
			want: "![image](data:image/png;base64,aW1hZ2U=)",
	REDACTED,
		{
			name: "text and image",
			parts: []any{
				map[string]any{"text": "rendered image:\n"REDACTED,
				map[string]any{"inlineData": map[string]any{"mimeType": "image/webp", "data": "d2VicA=="REDACTEDREDACTED,
		REDACTED,
			want: "rendered image:\n![image](data:image/webp;base64,d2VicA==)",
	REDACTED,
REDACTED

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			geminiResp := map[string]any{
				"candidates": []any{map[string]any{
					"content":      map[string]any{"parts": tt.partsREDACTED,
					"finishReason": "STOP",
		REDACTED
		REDACTED
			rawData, err := json.Marshal(geminiResp)
		REDACTED

			got, _, err := geminiResponseToChatCompletions(geminiResp, "gemini-test", rawData, nil)
		REDACTED
			require.Len(t, got.Choices, 1)

			var content string
			require.NoError(t, json.Unmarshal(got.Choices[0].Message.Content, &content))
			require.Equal(t, tt.want, content)
			require.Equal(t, "stop", got.Choices[0].FinishReason)
	REDACTED)
REDACTED
REDACTED

func TestGeminiResponseToChatCompletionsOmitsInvalidInlineData(t *testing.T) {
	tests := []struct {
		name       string
		inlineData map[string]any
REDACTED{
		{
			name:       "unsupported MIME type",
			inlineData: map[string]any{"mimeType": "image/svg+xml", "data": "PHN2Zz48L3N2Zz4="REDACTED,
	REDACTED,
		{
			name:       "malformed MIME type",
			inlineData: map[string]any{"mimeType": "image/png; charset=utf-8", "data": "aW1hZ2U="REDACTED,
	REDACTED,
		{
			name:       "malformed base64",
			inlineData: map[string]any{"mimeType": "image/png", "data": "not-valid-base64!!!"REDACTED,
	REDACTED,
REDACTED

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			geminiResp := map[string]any{
				"candidates": []any{map[string]any{
					"content":      map[string]any{"parts": []any{map[string]any{"text": "before"REDACTED, map[string]any{"inlineData": tt.inlineDataREDACTED, map[string]any{"text": "after"REDACTEDREDACTEDREDACTED,
					"finishReason": "STOP",
		REDACTED
		REDACTED
			rawData, err := json.Marshal(geminiResp)
		REDACTED

			got, _, err := geminiResponseToChatCompletions(geminiResp, "gemini-test", rawData, nil)
		REDACTED

			var content string
			require.NoError(t, json.Unmarshal(got.Choices[0].Message.Content, &content))
			require.Equal(t, "beforeafter", content)
	REDACTED)
REDACTED
REDACTED

func TestConvertGeminiToClaudeMessageOmitsInlineDataForAnthropicMessages(t *testing.T) {
	geminiResp := map[string]any{
		"candidates": []any{map[string]any{
			"content": map[string]any{"parts": []any{
				map[string]any{"text": "before"REDACTED,
				map[string]any{"inlineData": map[string]any{"mimeType": "image/png", "data": "aW1hZ2U="REDACTEDREDACTED,
				map[string]any{"functionCall": map[string]any{"name": "get_weather", "args": map[string]any{"city": "Paris"REDACTEDREDACTEDREDACTED,
				map[string]any{"text": "after"REDACTED,
	REDACTED
			"finishReason": "STOP",
REDACTED
REDACTED
	rawData, err := json.Marshal(geminiResp)
REDACTED

	withInlineData, _ := convertGeminiToClaudeMessage(geminiResp, "gemini-test", rawData, true)
	contentWithInlineData, ok := withInlineData["content"].([]any)
	require.True(t, ok)
	require.Len(t, contentWithInlineData, 4)
	require.Equal(t, map[string]any{"type": "text", "text": "before"REDACTED, contentWithInlineData[0])
	require.Equal(t, map[string]any{"type": "text", "text": "![image](data:image/png;base64,aW1hZ2U=)"REDACTED, contentWithInlineData[1])
	toolUse, ok := contentWithInlineData[2].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "tool_use", toolUse["type"])
	require.Equal(t, "get_weather", toolUse["name"])
	require.Equal(t, map[string]any{"type": "text", "text": "after"REDACTED, contentWithInlineData[3])

	withoutInlineData, _ := convertGeminiToClaudeMessage(geminiResp, "gemini-test", rawData, false)
	contentWithoutInlineData, ok := withoutInlineData["content"].([]any)
	require.True(t, ok)
	require.Len(t, contentWithoutInlineData, 3)
	require.Equal(t, map[string]any{"type": "text", "text": "before"REDACTED, contentWithoutInlineData[0])
	toolUseWithoutInlineData, ok := contentWithoutInlineData[1].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "tool_use", toolUseWithoutInlineData["type"])
	require.Equal(t, "get_weather", toolUseWithoutInlineData["name"])
	require.Equal(t, map[string]any{"type": "text", "text": "after"REDACTED, contentWithoutInlineData[2])
REDACTED

func TestGeminiResponseToChatCompletionsRetainsTextAndToolBehavior(t *testing.T) {
	geminiResp := map[string]any{
		"candidates": []any{map[string]any{
			"content": map[string]any{"parts": []any{
				map[string]any{"text": "checking"REDACTED,
				map[string]any{"functionCall": map[string]any{
					"name": "get_weather",
					"args": map[string]any{"city": "Paris"REDACTED,
		REDACTED
	REDACTED
			"finishReason": "STOP",
REDACTED
REDACTED
	rawData, err := json.Marshal(geminiResp)
REDACTED

	got, _, err := geminiResponseToChatCompletions(geminiResp, "gemini-test", rawData, nil)
REDACTED
	require.Len(t, got.Choices, 1)

	choice := got.Choices[0]
	var content string
	require.NoError(t, json.Unmarshal(choice.Message.Content, &content))
	require.Equal(t, "checking", content)
	require.Equal(t, "tool_calls", choice.FinishReason)
	require.Len(t, choice.Message.ToolCalls, 1)
	require.Equal(t, "get_weather", choice.Message.ToolCalls[0].Function.Name)
	require.JSONEq(t, `{"city":"Paris"REDACTED`, choice.Message.ToolCalls[0].Function.Arguments)
REDACTED
