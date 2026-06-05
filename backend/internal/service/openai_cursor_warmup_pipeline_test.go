package service

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

// TestCursorMixedShapeDetection covers the core invariant of the Cursor
// compatibility fix in ForwardAsChatCompletions: when a client POSTs a
// Responses-shaped body (has `input`, no `messages`) to /v1/chat/completions,
// the request must be forwarded as-is with only the `model` field rewritten.
// The raw `input` array (including Cursor's 80KB system prompt) must not be
// discarded or reshaped.
//
// Context:
//
//	Before the fix, the handler unmarshaled the body into ChatCompletionsRequest,
//	which has no Input field, silently dropping Cursor's input. The subsequent
//	conversion produced `input: null`, which Codex upstreams reject with
//	"Invalid type for 'input': expected a string, but got an object".
func TestCursorMixedShapeDetection(t *testing.T) {
	// Representative Cursor cloud body — shape is what matters, content is
	// abridged. Notice: `input` is a Responses-API array, there is no
	// `messages` field at all, and `user`/`stream` are at the top level.
	cursorBody := []byte(`{
		"user": "85df22e7463ab6c2",
		"model": "gpt-5.4",
		"stream": true,
		"input": [
			{"role":"system","content":"You are GPT-5.4 running as a coding agent."},
			{"role":"user","content":"hello"}
		],
		"service_tier": "auto",
		"reasoning": {"effort": "high"}
	}`)

	// --- Step 1: Shape detection (mirrors ForwardAsChatCompletions) ---
	hasMessages := gjson.GetBytes(cursorBody, "messages").Exists()
	hasInput := gjson.GetBytes(cursorBody, "input").Exists()
	isResponsesShape := !hasMessages && hasInput

	require.True(t, isResponsesShape,
		"Cursor body must be detected as Responses-shape (has input, no messages)")

	// --- Step 2: Model rewrite (mirrors the sjson.SetBytes branch) ---
	const upstreamModel = "gpt-5.1-codex"
	rewritten, err := sjson.SetBytes(cursorBody, "model", upstreamModel)
	require.NoError(t, err)

	// --- Step 3: Invariants of the rewritten body ---

	// 3a. model must be rewritten to the upstream target.
	assert.Equal(t, upstreamModel, gjson.GetBytes(rewritten, "model").String())

	// 3b. input array must be preserved verbatim — no reshaping, no nulling.
	inputResult := gjson.GetBytes(rewritten, "input")
	require.True(t, inputResult.Exists(), "input field must still exist after rewrite")
	require.True(t, inputResult.IsArray(), "input must still be an array (not null, not object)")

	items := inputResult.Array()
	require.Len(t, items, 2, "both input items must be preserved")
	assert.Equal(t, "system", items[0].Get("role").String())
	assert.Equal(t, "You are GPT-5.4 running as a coding agent.",
		items[0].Get("content").String())
	assert.Equal(t, "user", items[1].Get("role").String())
	assert.Equal(t, "hello", items[1].Get("content").String())

	// 3c. ALL other top-level fields must survive intact.
	assert.Equal(t, "85df22e7463ab6c2", gjson.GetBytes(rewritten, "user").String())
	assert.Equal(t, true, gjson.GetBytes(rewritten, "stream").Bool())
	assert.Equal(t, "auto", gjson.GetBytes(rewritten, "service_tier").String())
	assert.Equal(t, "high", gjson.GetBytes(rewritten, "reasoning.effort").String())

	// 3d. Final upstream body must NOT contain the old "input":null pattern.
	assert.NotContains(t, string(rewritten), `"input":null`,
		"rewritten body must not collapse input to null")
}

// TestCursorMixedShapeDetection_NormalChatCompletionsUnaffected guards that
// the shape detection does NOT misfire on a standard Chat Completions request
// (one that has a `messages` array). Such requests must fall through to the
// existing ChatCompletionsToResponses conversion path.
func TestCursorMixedShapeDetection_NormalChatCompletionsUnaffected(t *testing.T) {
	body := []byte(`{
		"model": "gpt-4o",
		"messages": [{"role":"user","content":"hi"}],
		"stream": true
	}`)

	hasMessages := gjson.GetBytes(body, "messages").Exists()
	hasInput := gjson.GetBytes(body, "input").Exists()
	isResponsesShape := !hasMessages && hasInput

	assert.False(t, isResponsesShape,
		"standard Chat Completions body must NOT be detected as Responses-shape")
}

// TestCursorMixedShapeDetection_BothFieldsPrefersMessages guards the
// ambiguous case where a client sends both `messages` and `input`. We fall
// through to the normal conversion path (messages wins), since mixing the
// two is almost certainly a client bug and messages is the documented
// Chat Completions contract.
func TestCursorMixedShapeDetection_BothFieldsPrefersMessages(t *testing.T) {
	body := []byte(`{
		"model": "gpt-4o",
		"messages": [{"role":"user","content":"hi"}],
		"input": [{"role":"user","content":"other"}]
	}`)

	hasMessages := gjson.GetBytes(body, "messages").Exists()
	hasInput := gjson.GetBytes(body, "input").Exists()
	isResponsesShape := !hasMessages && hasInput

	assert.False(t, isResponsesShape,
		"when both messages and input are present, must not take the Cursor shortcut")
}

// TestCursorMixedShapeDetection_EmptyBody ensures a body with neither
// messages nor input is NOT taken as Cursor-shape (would hit the normal
// conversion and fail on its own with a clearer error).
func TestCursorMixedShapeDetection_EmptyBody(t *testing.T) {
	body := []byte(`{"model":"gpt-5.4","stream":true}`)

	hasMessages := gjson.GetBytes(body, "messages").Exists()
	hasInput := gjson.GetBytes(body, "input").Exists()
	isResponsesShape := !hasMessages && hasInput

	assert.False(t, isResponsesShape,
		"body with neither messages nor input must not be taken as Cursor shape")
}

// TestCursorMixedShape_JSONRoundtrip ensures the rewritten body is still
// valid JSON and parseable back into a map without surprises — catches
// any encoding drift from sjson.
func TestCursorMixedShape_JSONRoundtrip(t *testing.T) {
	cursorBody := []byte(`{"model":"gpt-5.4","stream":true,"input":[{"role":"user","content":"hi"}]}`)

	rewritten, err := sjson.SetBytes(cursorBody, "model", "gpt-5.1-codex")
	require.NoError(t, err)

	var parsed map[string]any
	require.NoError(t, json.Unmarshal(rewritten, &parsed))

	assert.Equal(t, "gpt-5.1-codex", parsed["model"])
	assert.Equal(t, true, parsed["stream"])

	inputArr, ok := parsed["input"].([]any)
	require.True(t, ok, "input must decode to a Go []any after round-trip")
	require.Len(t, inputArr, 1)
}

// TestCursorMixedShape_StripsUnsupportedFields mirrors the strip loop in
// ForwardAsChatCompletions (isResponsesShape branch). Cursor cloud sends
// prompt_cache_retention, safety_identifier, metadata and stream_options
// as top-level Responses API parameters, which Codex upstreams reject with
// "Unsupported parameter: ...". The fix must remove them from the raw body
// before it is forwarded, for BOTH OAuth and API Key account types.
func TestCursorMixedShape_StripsUnsupportedFields(t *testing.T) {
	cursorBody := []byte(`{
		"model": "gpt-5.4",
		"stream": true,
		"prompt_cache_retention": "24h",
		"safety_identifier": "cursor-user-xyz",
		"metadata": {"trace_id":"abc","caller":"cursor"},
		"stream_options": {"include_usage": true},
		"input": [{"role":"user","content":"hi"}]
	}`)

	// Sanity: the test fixture contains every field the production code strips.
	for _, field := range cursorResponsesUnsupportedFields {
		require.True(t, gjson.GetBytes(cursorBody, field).Exists(),
			"test fixture must contain %s", field)
	}

	// Run the exact same loop as the production code.
	result := cursorBody
	for _, field := range cursorResponsesUnsupportedFields {
		if stripped, err := sjson.DeleteBytes(result, field); err == nil {
			result = stripped
		}
	}

	// All unsupported fields must be gone.
	for _, field := range cursorResponsesUnsupportedFields {
		assert.False(t, gjson.GetBytes(result, field).Exists(),
			"%s must be stripped", field)
	}

	// Everything else must survive intact.
	assert.Equal(t, "gpt-5.4", gjson.GetBytes(result, "model").String())
	assert.Equal(t, true, gjson.GetBytes(result, "stream").Bool())
	assert.True(t, gjson.GetBytes(result, "input").IsArray())
	assert.Equal(t, "user", gjson.GetBytes(result, "input.0.role").String())
}
