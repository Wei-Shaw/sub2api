package service

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestTrimOpenAIEncryptedReasoningItems_ContentNull(t *testing.T) {
	reqBody := map[string]any{
		"model": "grok-4.5",
		"input": []any{
			map[string]any{"type": "message", "role": "user", "content": "hi"REDACTED,
			map[string]any{
				"type":              "reasoning",
				"summary":           []any{map[string]any{"type": "summary_text", "text": "thinking..."REDACTEDREDACTED,
				"content":           nil,
				"encrypted_content": nil,
		REDACTED,
			map[string]any{"type": "message", "role": "assistant", "content": "Hello!"REDACTED,
	REDACTED,
REDACTED

	changed := trimOpenAIEncryptedReasoningItems(reqBody)
	require.True(t, changed)

	input, ok := reqBody["input"].([]any)
	require.True(t, ok)
	require.Len(t, input, 3)

	reasoning, ok := input[1].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "reasoning", reasoning["type"])
	assert.NotNil(t, reasoning["summary"])
	_, hasContent := reasoning["content"]
	assert.False(t, hasContent, "content: null should be stripped")
	_, hasEncrypted := reasoning["encrypted_content"]
	assert.False(t, hasEncrypted, "encrypted_content should be stripped")
REDACTED

func TestTrimOpenAIEncryptedReasoningItems_ContentNullOnly(t *testing.T) {
	reqBody := map[string]any{
		"model": "grok-4.5",
		"input": []any{
			map[string]any{
				"type":    "reasoning",
				"summary": []any{map[string]any{"type": "summary_text", "text": "ok"REDACTEDREDACTED,
				"content": nil,
		REDACTED,
	REDACTED,
REDACTED

	changed := trimOpenAIEncryptedReasoningItems(reqBody)
	require.True(t, changed)

	input, ok := reqBody["input"].([]any)
	require.True(t, ok)
	require.Len(t, input, 1)

	reasoning, ok := input[0].(map[string]any)
	require.True(t, ok)
	_, hasContent := reasoning["content"]
	assert.False(t, hasContent, "content: null should be stripped even without encrypted_content")
REDACTED

func TestTrimOpenAIEncryptedReasoningItems_ContentNonNull(t *testing.T) {
	reqBody := map[string]any{
		"model": "grok-4.5",
		"input": []any{
			map[string]any{
				"type":    "reasoning",
				"summary": []any{map[string]any{"type": "summary_text", "text": "ok"REDACTEDREDACTED,
				"content": "some actual content",
		REDACTED,
	REDACTED,
REDACTED

	changed := trimOpenAIEncryptedReasoningItems(reqBody)
	assert.False(t, changed, "non-null content should not be stripped")

	input, ok := reqBody["input"].([]any)
	require.True(t, ok)
	reasoning, ok := input[0].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "some actual content", reasoning["content"])
REDACTED

func TestTrimOpenAIEncryptedReasoningItems_NoReasoningItems(t *testing.T) {
	reqBody := map[string]any{
		"model": "grok-4.5",
		"input": []any{
			map[string]any{"type": "message", "role": "user", "content": "hi"REDACTED,
	REDACTED,
REDACTED

	changed := trimOpenAIEncryptedReasoningItems(reqBody)
	assert.False(t, changed)
REDACTED

func TestSanitizeOpenAICrossModeFailoverReasoning_DropsWholeEncryptedItem(t *testing.T) {
	body := []byte(`{"model":"gpt-5.1","input":[` +
		`{"type":"message","role":"user","content":"hi"REDACTED,` +
		`{"type":"reasoning","id":"rs_kiro_1","encrypted_content":"ENC","summary":[{"type":"summary_text","text":"t"REDACTED]REDACTED,` +
		`{"type":"message","role":"assistant","content":"yo"REDACTED` +
		`]REDACTED`)

	sanitized, changed, err := SanitizeOpenAICrossModeFailoverReasoning(body)
REDACTED
	require.True(t, changed)
	// The whole reasoning item is gone — id and summary go with encrypted_content,
	// unlike trimOpenAIEncryptedReasoningItems which keeps the skeleton.
	require.NotContains(t, string(sanitized), "reasoning")
	require.NotContains(t, string(sanitized), "rs_kiro_1")
	require.NotContains(t, string(sanitized), "summary_text")
	require.Equal(t, int64(2), gjson.GetBytes(sanitized, "input.#").Int())
REDACTED

func TestSanitizeOpenAICrossModeFailoverReasoning_NoEncryptedIsNoop(t *testing.T) {
	body := []byte(`{"model":"gpt-5.1","input":[{"type":"reasoning","summary":[{"type":"summary_text","text":"t"REDACTED]REDACTED]REDACTED`)
	sanitized, changed, err := SanitizeOpenAICrossModeFailoverReasoning(body)
REDACTED
	require.False(t, changed, "reasoning without encrypted_content must be preserved")
	require.Equal(t, string(body), string(sanitized))
REDACTED

func TestSanitizeOpenAICrossModeFailoverReasoning_NoInputIsNoop(t *testing.T) {
	body := []byte(`{"model":"gpt-5.1"REDACTED`)
	sanitized, changed, err := SanitizeOpenAICrossModeFailoverReasoning(body)
REDACTED
	require.False(t, changed)
	require.Equal(t, string(body), string(sanitized))
REDACTED

func TestSanitizeOpenAICrossModeFailoverReasoning_PreservesLargeIntegers(t *testing.T) {
	body := []byte(`{"model":"gpt-5.1","input":[` +
		`{"type":"reasoning","id":"rs_kiro_1","encrypted_content":"ENC"REDACTED,` +
		`{"type":"message","role":"user","content":"hi"REDACTED` +
		`],"tools":[{"type":"function","function":{"name":"lookup","parameters":{"type":"object","properties":{"id":{"const":9007199254740993REDACTEDREDACTEDREDACTEDREDACTEDREDACTED]REDACTED`)

	sanitized, changed, err := SanitizeOpenAICrossModeFailoverReasoning(body)
REDACTED
	require.True(t, changed)
	require.Contains(t, string(sanitized), `"const":9007199254740993`,
		"sanitization must not round JSON integers through float64")
REDACTED

func TestTrimOpenAIEncryptedReasoningItems_ContentNullDropsBareSkeleton(t *testing.T) {
	reqBody := map[string]any{
		"input": []any{
			map[string]any{"type": "reasoning", "content": nilREDACTED,
	REDACTED,
REDACTED

	changed := trimOpenAIEncryptedReasoningItems(reqBody)
	require.True(t, changed)
	_, hasInput := reqBody["input"]
	assert.False(t, hasInput, "bare reasoning skeleton should be dropped, emptying input")
REDACTED
