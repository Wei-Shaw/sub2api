package service

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func aiStudioOpenAIAccount() *Account {
	return &Account{
		Platform: PlatformOpenAI,
		Type:     AccountTypeAPIKey,
		Credentials: map[string]any{
			"api_key":  "sk-test",
			"base_url": "https://generativelanguage.googleapis.com/v1beta/openai/chat/completions",
		},
	}
}

// Codex history comes back through the Responses bridge without Gemini's
// extra_content.google.thought_signature, and AI Studio rejects any replayed
// function call that has none with 400 "Function call is missing a
// thought_signature in functionCall parts".
func TestEnsureGeminiChatThoughtSignatures_FillsMissing(t *testing.T) {
	body := []byte(`{"model":"gemini-4-neon","messages":[` +
		`{"role":"user","content":"run"},` +
		`{"role":"assistant","tool_calls":[` +
		`{"id":"call_a","type":"function","function":{"name":"exec_command","arguments":"{}"}},` +
		`{"id":"call_b","type":"function","function":{"name":"exec_command","arguments":"{}"}}]},` +
		`{"role":"tool","tool_call_id":"call_a","content":"ok"},` +
		`{"role":"tool","tool_call_id":"call_b","content":"ok"}]}`)

	out := ensureGeminiChatThoughtSignatures(aiStudioOpenAIAccount(), body)

	calls := gjson.GetBytes(out, "messages.1.tool_calls")
	require.Len(t, calls.Array(), 2)
	for _, call := range calls.Array() {
		require.Equal(t, geminiDummyThoughtSignature, call.Get("extra_content.google.thought_signature").String())
	}
	require.Equal(t, "call_a", calls.Get("0.id").String())
	require.Equal(t, "call_a", gjson.GetBytes(out, "messages.2.tool_call_id").String())
}

// Unity's LiteLLM carries a signature in the call id, but it was issued by a
// different Google backend: AI Studio rejects it with 400 "Corrupted thought
// signature" (INVALID_THOUGHT_SIGNATURE). Such a call gets the placeholder and
// keeps the id it was answered under.
func TestEnsureGeminiChatThoughtSignatures_IgnoresLiteLLMIDSuffix(t *testing.T) {
	body := []byte(`{"model":"gemini-4-neon","messages":[` +
		`{"role":"assistant","tool_calls":[{"id":"call_a__thought__REALSIG","type":"function","function":{"name":"x","arguments":"{}"}}]},` +
		`{"role":"tool","tool_call_id":"call_a__thought__REALSIG","content":"ok"}]}`)

	out := ensureGeminiChatThoughtSignatures(aiStudioOpenAIAccount(), body)

	require.Equal(t, geminiDummyThoughtSignature, gjson.GetBytes(out, "messages.0.tool_calls.0.extra_content.google.thought_signature").String())
	require.Equal(t, "call_a__thought__REALSIG", gjson.GetBytes(out, "messages.0.tool_calls.0.id").String())
}

func TestEnsureGeminiChatThoughtSignatures_KeepsExistingAndOtherUpstreams(t *testing.T) {
	withSig := []byte(`{"model":"gemini-4-neon","messages":[{"role":"assistant","tool_calls":[{"id":"call_a","type":"function","function":{"name":"x","arguments":"{}"},"extra_content":{"google":{"thought_signature":"REAL"}}}]}]}`)
	require.Equal(t, string(withSig), string(ensureGeminiChatThoughtSignatures(aiStudioOpenAIAccount(), withSig)))

	noCalls := []byte(`{"model":"gemini-4-neon","messages":[{"role":"user","content":"hi"}]}`)
	require.Equal(t, string(noCalls), string(ensureGeminiChatThoughtSignatures(aiStudioOpenAIAccount(), noCalls)))

	other := &Account{Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Credentials: map[string]any{
		"api_key": "sk-test", "base_url": "https://uai-litellm.internal.unity.com:8443",
	}}
	missing := []byte(`{"model":"gemini-4-neon","messages":[{"role":"assistant","tool_calls":[{"id":"call_a","type":"function","function":{"name":"x","arguments":"{}"}}]}]}`)
	require.Equal(t, string(missing), string(ensureGeminiChatThoughtSignatures(other, missing)))
}
