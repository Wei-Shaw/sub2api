package service

import (
	"strconv"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestOpenAIResponsesInputItemIDPrefixUsesObservedOutputContracts(t *testing.T) {
	tests := []struct {
		itemType string
		id       string
		strip    bool
	}{
		{itemType: "message", id: "msg_123", strip: false},
		{itemType: "message", id: "item_123", strip: true},
		{itemType: "reasoning", id: "rs_123", strip: false},
		{itemType: "reasoning", id: "item_123", strip: true},
		{itemType: "function_call", id: "fc_123", strip: false},
		{itemType: "function_call", id: "call_123", strip: true},
		{itemType: "tool_call", id: "fc_123", strip: false},
		{itemType: "local_shell_call", id: "fc_123", strip: false},
		{itemType: "mcp_tool_call", id: "fc_123", strip: false},
		{itemType: "custom_tool_call", id: "ctc_123", strip: false},
		{itemType: "custom_tool_call", id: "fc_123", strip: true},
		{itemType: "tool_search_call", id: "tsc_123", strip: false},
		{itemType: "tool_search_call", id: "fc_123", strip: true},
		{itemType: "web_search_call", id: "ws_123", strip: false},
		{itemType: "web_search_call", id: "item_123", strip: true},
		{itemType: "custom_tool_call_output", id: "fc_123", strip: false},
		{itemType: "custom_tool_call_output", id: "ctco_123", strip: true},
		// Do not impose an inferred contract on output types for which there is
		// no observed upstream prefix rejection.
		{itemType: "function_call_output", id: "fco_123", strip: false},
		{itemType: "tool_search_output", id: "tso_123", strip: false},
		{itemType: "mcp_tool_call_output", id: "mcpo_123", strip: false},
		{itemType: "future_item", id: "item_123", strip: false},
	}
	for _, tt := range tests {
		t.Run(tt.itemType+"/"+tt.id, func(t *testing.T) {
			require.Equal(t, tt.strip, shouldStripOpenAIResponsesInputItemID(tt.itemType, tt.id))
		})
	}
}

func TestSanitizeOpenAIResponsesInputItemIDsDoesNotCascadeAcrossIDNamespaces(t *testing.T) {
	body := []byte(`{"input":[
		{"type":"function_call","id":"item_bad_call","call_id":"call_valid","name":"lookup","arguments":"{}"},
		{"type":"function_call_output","call_id":"call_valid","output":"preserve paired output"},
		{"type":"function_call_output","call_id":"item_bad_call","output":"preserve opaque output"},
		{"type":"item_reference","id":"item_bad_call"},
		{"type":"item_reference","id":"remote_valid"},
		{"type":"custom_tool_call","id":"ctc_valid","call_id":"ctco_bad_output","name":"apply_patch","input":"patch"},
		{"type":"custom_tool_call_output","id":"ctco_bad_output","call_id":"ctco_bad_output","output":"preserve by call_id"}
	]}`)

	sanitized, changed, err := sanitizeOpenAIResponsesInputItemIDs(body)

	require.NoError(t, err)
	require.True(t, changed)
	items := gjson.GetBytes(sanitized, "input").Array()
	require.Len(t, items, 7)
	require.False(t, items[0].Get("id").Exists())
	require.Equal(t, "call_valid", items[0].Get("call_id").String())
	require.Equal(t, "preserve paired output", items[1].Get("output").String())
	require.Equal(t, "preserve opaque output", items[2].Get("output").String())
	require.Equal(t, "item_bad_call", items[3].Get("id").String())
	require.Equal(t, "remote_valid", items[4].Get("id").String())
	require.Equal(t, "ctc_valid", items[5].Get("id").String())
	require.False(t, items[6].Get("id").Exists())
	require.Equal(t, "ctco_bad_output", items[6].Get("call_id").String())
}

func TestSanitizeOpenAIResponsesInputItemIDsLeavesUnrelatedReferencesUntouched(t *testing.T) {
	body := []byte(`{"previous_response_id":"resp_1","input":[{"type":"item_reference","id":"remote_item"}]}`)

	sanitized, changed, err := sanitizeOpenAIResponsesInputItemIDs(body)

	require.NoError(t, err)
	require.False(t, changed)
	require.Equal(t, body, sanitized)
}

func TestSanitizeOpenAIResponsesInputItemIDsPreservesReferenceToDuplicateRetainedID(t *testing.T) {
	body := []byte(`{"input":[{"type":"function_call","id":"ctc_shared","call_id":"call_1"},{"type":"custom_tool_call","id":"ctc_shared","call_id":"call_2"},{"type":"item_reference","id":"ctc_shared"}]}`)

	sanitized, changed, err := sanitizeOpenAIResponsesInputItemIDs(body)

	require.NoError(t, err)
	require.True(t, changed)
	require.False(t, gjson.GetBytes(sanitized, "input.0.id").Exists())
	require.Equal(t, "ctc_shared", gjson.GetBytes(sanitized, "input.1.id").String())
	require.Equal(t, "ctc_shared", gjson.GetBytes(sanitized, "input.2.id").String())
}

func TestSanitizeOpenAIResponsesInputItemIDsPreservesOpaqueOutputsAndReferences(t *testing.T) {
	body := []byte(`{"input":[
		{"type":"function_call","id":"item_shared","call_id":"call_real"},
		{"type":"function_call_output","id":"item_shared","call_id":"item_shared","output":"dangling"},
		{"type":"item_reference","id":"item_shared"},
		{"type":"function_call_output","id":"kept_output","call_id":"call_real","output":"kept"},
		{"type":"item_reference","id":"kept_output"}
	]}`)

	sanitized, changed, err := sanitizeOpenAIResponsesInputItemIDs(body)

	require.NoError(t, err)
	require.True(t, changed)
	require.Len(t, gjson.GetBytes(sanitized, "input").Array(), 5)
	require.False(t, gjson.GetBytes(sanitized, "input.0.id").Exists())
	require.Equal(t, "dangling", gjson.GetBytes(sanitized, "input.1.output").String())
	require.Equal(t, "item_shared", gjson.GetBytes(sanitized, "input.2.id").String())
	require.Equal(t, "kept_output", gjson.GetBytes(sanitized, "input.3.id").String())
	require.Equal(t, "kept_output", gjson.GetBytes(sanitized, "input.4.id").String())

	second, changedAgain, err := sanitizeOpenAIResponsesInputItemIDs(sanitized)
	require.NoError(t, err)
	require.False(t, changedAgain)
	require.Equal(t, sanitized, second)
}

func TestSanitizeOpenAIResponsesInputItemIDsStripsEmptyKnownIDsOnly(t *testing.T) {
	body := []byte(`{"input":[{"type":"message","id":"","content":"hello"},{"type":"future_item","id":""}]}`)

	sanitized, changed, err := sanitizeOpenAIResponsesInputItemIDs(body)

	require.NoError(t, err)
	require.True(t, changed)
	require.False(t, gjson.GetBytes(sanitized, "input.0.id").Exists())
	require.True(t, gjson.GetBytes(sanitized, "input.1.id").Exists())
}

func TestSanitizeOpenAIResponsesInputItemIDsStripsOnlyNonPairCallIDs(t *testing.T) {
	body := []byte(`{"input":[
		{"type":"message","call_id":"remove_message","content":"hi"},
		{"type":"reasoning","call_id":"remove_reasoning","id":"rs_keep","encrypted_content":"cipher","summary":[]},
		{"type":"image_generation_call","call_id":"remove_image","id":"ig_keep","status":"completed"},
		{"type":"function_call","call_id":"keep_function","name":"lookup","arguments":"{}"},
		{"type":"function_call_output","call_id":"keep_function","output":"ok"},
		{"type":"custom_tool_call","call_id":"keep_custom","name":"patch","input":"x"},
		{"type":"custom_tool_call_output","call_id":"keep_custom","output":"ok"},
		{"type":"tool_search_call","call_id":"keep_search","arguments":"{}"},
		{"type":"tool_search_output","call_id":"keep_search","output":"ok"},
		{"type":"local_shell_call","call_id":"keep_shell","name":"shell","arguments":"{}"}
	]}`)

	sanitized, changed, err := sanitizeOpenAIResponsesInputItemIDs(body)
	require.NoError(t, err)
	require.True(t, changed)
	for i := 0; i < 3; i++ {
		require.False(t, gjson.GetBytes(sanitized, "input."+strconv.Itoa(i)+".call_id").Exists())
	}
	for i := 3; i < 10; i++ {
		require.True(t, gjson.GetBytes(sanitized, "input."+strconv.Itoa(i)+".call_id").Exists())
	}
}
