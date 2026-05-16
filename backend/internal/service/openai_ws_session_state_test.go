//go:build unit

package service

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestShouldForceNewConnOnStoreDisabled_Off(t *testing.T) {
	assert.False(t, shouldForceNewConnOnStoreDisabled("off", ""))
	assert.False(t, shouldForceNewConnOnStoreDisabled("off", "prewarm_timeout"))
}

func TestShouldForceNewConnOnStoreDisabled_Default(t *testing.T) {
	assert.True(t, shouldForceNewConnOnStoreDisabled("", ""))
	assert.True(t, shouldForceNewConnOnStoreDisabled("always", ""))
}

func TestShouldForceNewConnOnStoreDisabled_Adaptive(t *testing.T) {
	assert.True(t, shouldForceNewConnOnStoreDisabled("adaptive", "policy_violation"))
	assert.True(t, shouldForceNewConnOnStoreDisabled("adaptive", "message_too_big"))
	assert.True(t, shouldForceNewConnOnStoreDisabled("adaptive", "auth_failed"))
	assert.True(t, shouldForceNewConnOnStoreDisabled("adaptive", "write_request"))
	assert.True(t, shouldForceNewConnOnStoreDisabled("adaptive", "write"))
	assert.False(t, shouldForceNewConnOnStoreDisabled("adaptive", ""))
	assert.False(t, shouldForceNewConnOnStoreDisabled("adaptive", "other_reason"))
}

func TestShouldForceNewConnOnStoreDisabled_PrewarmPrefix(t *testing.T) {
	assert.False(t, shouldForceNewConnOnStoreDisabled("adaptive", "prewarm_timeout"))
	assert.False(t, shouldForceNewConnOnStoreDisabled("adaptive", "prewarm_error"))
}

func TestCloneOpenAIWSPayloadBytes_Nil(t *testing.T) {
	assert.Nil(t, cloneOpenAIWSPayloadBytes(nil))
}

func TestCloneOpenAIWSPayloadBytes_Empty(t *testing.T) {
	assert.Nil(t, cloneOpenAIWSPayloadBytes([]byte{}))
}

func TestCloneOpenAIWSPayloadBytes_CopiesData(t *testing.T) {
	original := []byte(`{"key":"value"}`)
	clone := cloneOpenAIWSPayloadBytes(original)
	assert.Equal(t, original, clone)
	// Mutating clone should not affect original
	clone[0] = 'x'
	assert.NotEqual(t, original, clone)
}

func TestCloneOpenAIWSRawMessages_Nil(t *testing.T) {
	assert.Nil(t, cloneOpenAIWSRawMessages(nil))
}

func TestCloneOpenAIWSRawMessages_ClonesEach(t *testing.T) {
	items := []json.RawMessage{
		json.RawMessage(`{"a":1}`),
		json.RawMessage(`{"b":2}`),
	}
	clone := cloneOpenAIWSRawMessages(items)
	require.Len(t, clone, 2)
	assert.Equal(t, items[0], clone[0])
	// Mutating clone should not affect original
	clone[0][0] = 'x'
	assert.NotEqual(t, items[0], clone[0])
}

func TestNormalizeOpenAIWSJSONForCompare_Valid(t *testing.T) {
	input := []byte(`  {"b":2,"a":1}  `)
	result, err := normalizeOpenAIWSJSONForCompare(input)
	require.NoError(t, err)
	// Should produce canonical JSON (sorted keys via marshal/unmarshal)
	assert.Contains(t, string(result), `"a"`)
	assert.Contains(t, string(result), `"b"`)
}

func TestNormalizeOpenAIWSJSONForCompare_EmptyInput(t *testing.T) {
	_, err := normalizeOpenAIWSJSONForCompare(nil)
	assert.Error(t, err)

	_, err = normalizeOpenAIWSJSONForCompare([]byte("   "))
	assert.Error(t, err)
}

func TestNormalizeOpenAIWSJSONForCompare_InvalidJSON(t *testing.T) {
	_, err := normalizeOpenAIWSJSONForCompare([]byte(`not json`))
	assert.Error(t, err)
}

func TestNormalizeOpenAIWSJSONForCompareOrRaw_FallbackOnError(t *testing.T) {
	input := []byte(`not json`)
	result := normalizeOpenAIWSJSONForCompareOrRaw(input)
	assert.Equal(t, []byte("not json"), result)
}

func TestNormalizeOpenAIWSJSONForCompareOrRaw_NormalizesValid(t *testing.T) {
	input := []byte(`  {"key": "value"}  `)
	result := normalizeOpenAIWSJSONForCompareOrRaw(input)
	assert.NotEqual(t, input, result)
	assert.Contains(t, string(result), `"key"`)
}

func TestOpenAIWSRawItemsHasPrefix_EmptyPrefix(t *testing.T) {
	items := []json.RawMessage{json.RawMessage(`{"a":1}`)}
	assert.True(t, openAIWSRawItemsHasPrefix(items, nil))
	assert.True(t, openAIWSRawItemsHasPrefix(items, []json.RawMessage{}))
}

func TestOpenAIWSRawItemsHasPrefix_PrefixLongerThanItems(t *testing.T) {
	items := []json.RawMessage{json.RawMessage(`{"a":1}`)}
	prefix := []json.RawMessage{json.RawMessage(`{"a":1}`), json.RawMessage(`{"b":2}`)}
	assert.False(t, openAIWSRawItemsHasPrefix(items, prefix))
}

func TestOpenAIWSRawItemsHasPrefix_MatchingPrefix(t *testing.T) {
	items := []json.RawMessage{
		json.RawMessage(`{"a":1}`),
		json.RawMessage(`{"b":2}`),
		json.RawMessage(`{"c":3}`),
	}
	prefix := []json.RawMessage{
		json.RawMessage(`{"a":1}`),
		json.RawMessage(`{"b":2}`),
	}
	assert.True(t, openAIWSRawItemsHasPrefix(items, prefix))
}

func TestOpenAIWSRawItemsHasFunctionCallOutput_None(t *testing.T) {
	items := []json.RawMessage{
		json.RawMessage(`{"type":"input_text","text":"hello"}`),
	}
	assert.False(t, openAIWSRawItemsHasFunctionCallOutput(items))
}

func TestOpenAIWSRawItemsHasFunctionCallOutput_HasOne(t *testing.T) {
	items := []json.RawMessage{
		json.RawMessage(`{"type":"input_text","text":"hello"}`),
		json.RawMessage(`{"type":"function_call_output","call_id":"call_123","output":"result"}`),
	}
	assert.True(t, openAIWSRawItemsHasFunctionCallOutput(items))
}

func TestShouldInferIngressFunctionCallOutputPreviousResponseID_Extended(t *testing.T) {
	tests := []struct {
		name                    string
		storeDisabled           bool
		turn                    int
		hasFunctionCallOutput   bool
		hasToolCallContext      bool
		currentPrevID           string
		expectedPrevID          string
		want                    bool
	}{
		{"all conditions met", true, 2, true, false, "", "resp_123", true},
		{"store not disabled", false, 2, true, false, "", "resp_123", false},
		{"first turn", true, 1, true, false, "", "resp_123", false},
		{"no function call output", true, 2, false, false, "", "resp_123", false},
		{"has tool call context", true, 2, true, true, "", "resp_123", false},
		{"already has previous_response_id", true, 2, true, false, "resp_existing", "resp_123", false},
		{"empty expected ID", true, 2, true, false, "", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			signals := ToolContinuationSignals{
				HasFunctionCallOutput: tt.hasFunctionCallOutput,
				HasToolCallContext:    tt.hasToolCallContext,
			}
			got := shouldInferIngressFunctionCallOutputPreviousResponseID(
				tt.storeDisabled, tt.turn, signals, tt.currentPrevID, tt.expectedPrevID,
			)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestSetOpenAIWSTurnMetadata_NilPayload(t *testing.T) {
	// Should not panic
	setOpenAIWSTurnMetadata(nil, `{"key":"value"}`)
}

func TestSetOpenAIWSTurnMetadata_EmptyMetadata(t *testing.T) {
	payload := map[string]any{"model": "test"}
	setOpenAIWSTurnMetadata(payload, "")
	_, exists := payload["client_metadata"]
	assert.False(t, exists)
}

func TestSetOpenAIWSTurnMetadata_ValidMetadata(t *testing.T) {
	payload := map[string]any{"model": "test"}
	setOpenAIWSTurnMetadata(payload, `{"session_id":"sess_123"}`)
	_, exists := payload["client_metadata"]
	assert.True(t, exists)
}
