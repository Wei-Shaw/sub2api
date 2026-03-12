package service

import (
	"encoding/json"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/pkg/antigravity"
	"github.com/stretchr/testify/require"
)

func TestCleanGeminiNativeThoughtSignatures_ReplacesNestedThoughtSignatures(t *testing.T) {
	input := []byte(`{
		"contents": [
			{
				"role": "user",
				"parts": [{"text": "hello"REDACTED]
		REDACTED,
			{
				"role": "model",
				"parts": [
					{"text": "thinking", "thought": true, "thoughtSignature": "sig_1"REDACTED,
					{"functionCall": {"name": "toolA", "args": {"k": "v"REDACTEDREDACTED, "thoughtSignature": "sig_2"REDACTED
				]
		REDACTED
		],
		"cachedContent": {
			"parts": [{"text": "cached", "thoughtSignature": "sig_3"REDACTED]
	REDACTED,
		"signature": "keep_me"
REDACTED`)

	cleaned := CleanGeminiNativeThoughtSignatures(input)

REDACTED
	require.NoError(t, json.Unmarshal(cleaned, &got))

	require.NotContains(t, string(cleaned), `"thoughtSignature":"sig_1"`)
	require.NotContains(t, string(cleaned), `"thoughtSignature":"sig_2"`)
	require.NotContains(t, string(cleaned), `"thoughtSignature":"sig_3"`)
	require.Contains(t, string(cleaned), `"thoughtSignature":"`+antigravity.DummyThoughtSignature+`"`)
	require.Contains(t, string(cleaned), `"signature":"keep_me"`)
REDACTED

func TestCleanGeminiNativeThoughtSignatures_InvalidJSONReturnsOriginal(t *testing.T) {
	input := []byte(`{"contents":[invalid-json]REDACTED`)

	cleaned := CleanGeminiNativeThoughtSignatures(input)

	require.Equal(t, input, cleaned)
REDACTED

func TestReplaceThoughtSignaturesRecursive_OnlyReplacesTargetField(t *testing.T) {
	input := map[string]any{
		"thoughtSignature": "sig_root",
		"signature":        "keep_signature",
		"nested": []any{
			map[string]any{
				"thoughtSignature": "sig_nested",
				"signature":        "keep_nested_signature",
		REDACTED,
	REDACTED,
REDACTED

	got, ok := replaceThoughtSignaturesRecursive(input).(map[string]any)
	require.True(t, ok)
	require.Equal(t, antigravity.DummyThoughtSignature, got["thoughtSignature"])
	require.Equal(t, "keep_signature", got["signature"])

	nested, ok := got["nested"].([]any)
	require.True(t, ok)
	nestedMap, ok := nested[0].(map[string]any)
	require.True(t, ok)
	require.Equal(t, antigravity.DummyThoughtSignature, nestedMap["thoughtSignature"])
	require.Equal(t, "keep_nested_signature", nestedMap["signature"])
REDACTED
