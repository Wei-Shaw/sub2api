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
REDACTED{
		{itemType: "message", id: "msg_123", strip: falseREDACTED,
		{itemType: "message", id: "item_123", strip: trueREDACTED,
		{itemType: "reasoning", id: "rs_123", strip: falseREDACTED,
		{itemType: "reasoning", id: "item_123", strip: trueREDACTED,
		{itemType: "function_call", id: "fc_123", strip: falseREDACTED,
		{itemType: "function_call", id: "call_123", strip: trueREDACTED,
		{itemType: "tool_call", id: "fc_123", strip: falseREDACTED,
		{itemType: "local_shell_call", id: "fc_123", strip: falseREDACTED,
		{itemType: "mcp_tool_call", id: "fc_123", strip: falseREDACTED,
		{itemType: "custom_tool_call", id: "ctc_123", strip: falseREDACTED,
		{itemType: "custom_tool_call", id: "fc_123", strip: trueREDACTED,
		{itemType: "tool_search_call", id: "tsc_123", strip: falseREDACTED,
		{itemType: "tool_search_call", id: "fc_123", strip: trueREDACTED,
		{itemType: "web_search_call", id: "ws_123", strip: falseREDACTED,
		{itemType: "web_search_call", id: "item_123", strip: trueREDACTED,
		{itemType: "custom_tool_call_output", id: "fc_123", strip: falseREDACTED,
		{itemType: "custom_tool_call_output", id: "ctco_123", strip: trueREDACTED,
		// Do not impose an inferred contract on output types for which there is
		// no observed upstream prefix rejection.
		{itemType: "function_call_output", id: "fco_123", strip: falseREDACTED,
		{itemType: "tool_search_output", id: "tso_123", strip: falseREDACTED,
		{itemType: "mcp_tool_call_output", id: "mcpo_123", strip: falseREDACTED,
		{itemType: "future_item", id: "item_123", strip: falseREDACTED,
REDACTED
	for _, tt := range tests {
		t.Run(tt.itemType+"/"+tt.id, func(t *testing.T) {
			require.Equal(t, tt.strip, shouldStripOpenAIResponsesInputItemID(tt.itemType, tt.id))
	REDACTED)
REDACTED
REDACTED

func TestSanitizeOpenAIResponsesInputItemIDsDoesNotCascadeAcrossIDNamespaces(t *testing.T) {
	body := []byte(`{"input":[
		{"type":"function_call","id":"item_bad_call","call_id":"call_valid","name":"lookup","arguments":"{REDACTED"REDACTED,
		{"type":"function_call_output","call_id":"call_valid","output":"preserve paired output"REDACTED,
		{"type":"function_call_output","call_id":"item_bad_call","output":"preserve opaque output"REDACTED,
		{"type":"item_reference","id":"item_bad_call"REDACTED,
		{"type":"item_reference","id":"remote_valid"REDACTED,
		{"type":"custom_tool_call","id":"ctc_valid","call_id":"ctco_bad_output","name":"apply_patch","input":"patch"REDACTED,
		{"type":"custom_tool_call_output","id":"ctco_bad_output","call_id":"ctco_bad_output","output":"preserve by call_id"REDACTED
	]REDACTED`)

	sanitized, changed, err := sanitizeOpenAIResponsesInputItemIDs(body)

REDACTED
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
REDACTED

func TestSanitizeOpenAIResponsesInputItemIDsLeavesUnrelatedReferencesUntouched(t *testing.T) {
	body := []byte(`{"previous_response_id":"resp_1","input":[{"type":"item_reference","id":"remote_item"REDACTED]REDACTED`)

	sanitized, changed, err := sanitizeOpenAIResponsesInputItemIDs(body)

REDACTED
	require.False(t, changed)
	require.Equal(t, body, sanitized)
REDACTED

func TestSanitizeOpenAIResponsesInputItemIDsPreservesReferenceToDuplicateRetainedID(t *testing.T) {
	body := []byte(`{"input":[{"type":"function_call","id":"ctc_shared","call_id":"call_1"REDACTED,{"type":"custom_tool_call","id":"ctc_shared","call_id":"call_2"REDACTED,{"type":"item_reference","id":"ctc_shared"REDACTED]REDACTED`)

	sanitized, changed, err := sanitizeOpenAIResponsesInputItemIDs(body)

REDACTED
	require.True(t, changed)
	require.False(t, gjson.GetBytes(sanitized, "input.0.id").Exists())
	require.Equal(t, "ctc_shared", gjson.GetBytes(sanitized, "input.1.id").String())
	require.Equal(t, "ctc_shared", gjson.GetBytes(sanitized, "input.2.id").String())
REDACTED

func TestSanitizeOpenAIResponsesInputItemIDsPreservesOpaqueOutputsAndReferences(t *testing.T) {
	body := []byte(`{"input":[
		{"type":"function_call","id":"item_shared","call_id":"call_real"REDACTED,
		{"type":"function_call_output","id":"item_shared","call_id":"item_shared","output":"dangling"REDACTED,
		{"type":"item_reference","id":"item_shared"REDACTED,
		{"type":"function_call_output","id":"kept_output","call_id":"call_real","output":"kept"REDACTED,
		{"type":"item_reference","id":"kept_output"REDACTED
	]REDACTED`)

	sanitized, changed, err := sanitizeOpenAIResponsesInputItemIDs(body)

REDACTED
	require.True(t, changed)
	require.Len(t, gjson.GetBytes(sanitized, "input").Array(), 5)
	require.False(t, gjson.GetBytes(sanitized, "input.0.id").Exists())
	require.Equal(t, "dangling", gjson.GetBytes(sanitized, "input.1.output").String())
	require.Equal(t, "item_shared", gjson.GetBytes(sanitized, "input.2.id").String())
	require.Equal(t, "kept_output", gjson.GetBytes(sanitized, "input.3.id").String())
	require.Equal(t, "kept_output", gjson.GetBytes(sanitized, "input.4.id").String())

	second, changedAgain, err := sanitizeOpenAIResponsesInputItemIDs(sanitized)
REDACTED
	require.False(t, changedAgain)
	require.Equal(t, sanitized, second)
REDACTED

func TestSanitizeOpenAIResponsesInputItemIDsStripsEmptyKnownIDsOnly(t *testing.T) {
	body := []byte(`{"input":[{"type":"message","id":"","content":"hello"REDACTED,{"type":"future_item","id":""REDACTED]REDACTED`)

	sanitized, changed, err := sanitizeOpenAIResponsesInputItemIDs(body)

REDACTED
	require.True(t, changed)
	require.False(t, gjson.GetBytes(sanitized, "input.0.id").Exists())
	require.True(t, gjson.GetBytes(sanitized, "input.1.id").Exists())
REDACTED

func TestSanitizeOpenAIResponsesInputItemIDsStripsOnlyNonPairCallIDs(t *testing.T) {
	body := []byte(`{"input":[
		{"type":"message","call_id":"remove_message","content":"hi"REDACTED,
		{"type":"reasoning","call_id":"remove_reasoning","id":"rs_keep","encrypted_content":"cipher","summary":[]REDACTED,
		{"type":"image_generation_call","call_id":"remove_image","id":"ig_keep","status":"completed"REDACTED,
		{"type":"function_call","call_id":"keep_function","name":"lookup","arguments":"{REDACTED"REDACTED,
		{"type":"function_call_output","call_id":"keep_function","output":"ok"REDACTED,
		{"type":"custom_tool_call","call_id":"keep_custom","name":"patch","input":"x"REDACTED,
		{"type":"custom_tool_call_output","call_id":"keep_custom","output":"ok"REDACTED,
		{"type":"tool_search_call","call_id":"keep_search","arguments":"{REDACTED"REDACTED,
		{"type":"tool_search_output","call_id":"keep_search","output":"ok"REDACTED,
		{"type":"local_shell_call","call_id":"keep_shell","name":"shell","arguments":"{REDACTED"REDACTED
	]REDACTED`)

	sanitized, changed, err := sanitizeOpenAIResponsesInputItemIDs(body)
REDACTED
	require.True(t, changed)
	for i := 0; i < 3; i++ {
		require.False(t, gjson.GetBytes(sanitized, "input."+strconv.Itoa(i)+".call_id").Exists())
REDACTED
	for i := 3; i < 10; i++ {
		require.True(t, gjson.GetBytes(sanitized, "input."+strconv.Itoa(i)+".call_id").Exists())
REDACTED
REDACTED
