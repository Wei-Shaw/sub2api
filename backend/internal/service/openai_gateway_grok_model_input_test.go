package service

import (
	"net/http"
	"strconv"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestSanitizeGrokResponsesModelInputNormalizesReplayHistory(t *testing.T) {
	body := []byte(`{
		"input":[
			{"role":"system","content":"rules"REDACTED,
			{"type":"message","role":"assistant","id":"msg_partial","content":[{"type":"output_text","text":"done"REDACTED]REDACTED,
			{"type":"reasoning","id":"rs_1","status":"completed","content":null,"summary":[{"type":"summary_text","text":"keep"REDACTED]REDACTED,
			{"type":"message","role":"assistant","id":"msg_complete","status":"completed","content":[{"type":"output_text","text":"kept"REDACTED]REDACTED
		]REDACTED`)

	patched, err := sanitizeGrokResponsesModelInput(body)
REDACTED
	require.Equal(t, "message", gjson.GetBytes(patched, "input.0.type").String())
	require.Equal(t, "done", gjson.GetBytes(patched, "input.1.content").String())
	require.False(t, gjson.GetBytes(patched, "input.1.id").Exists())
	require.False(t, gjson.GetBytes(patched, "input.2.status").Exists())
	require.False(t, gjson.GetBytes(patched, "input.2.content").Exists())
	require.Equal(t, "keep", gjson.GetBytes(patched, "input.2.summary.0.text").String())
	require.Equal(t, "msg_complete", gjson.GetBytes(patched, "input.3.id").String())
	require.Equal(t, "completed", gjson.GetBytes(patched, "input.3.status").String())
REDACTED

func TestSanitizeGrokResponsesModelInputStripsOnlyNonPairCallIDs(t *testing.T) {
	body := []byte(`{"input":[
		{"type":"message","role":"user","call_id":"remove_message","content":"continue"REDACTED,
		{"type":"reasoning","call_id":"remove_reasoning","summary":[]REDACTED,
		{"type":"image_generation_call","call_id":"remove_image","id":"ig_1","status":"completed"REDACTED,
		{"type":"function_call","call_id":"keep_function","name":"lookup","arguments":"{REDACTED"REDACTED,
		{"type":"function_call_output","call_id":"keep_function","output":"ok"REDACTED
	]REDACTED`)

	patched, err := sanitizeGrokResponsesModelInput(body)
REDACTED
	for i := 0; i < 3; i++ {
		require.False(t, gjson.GetBytes(patched, "input."+strconv.Itoa(i)+".call_id").Exists())
REDACTED
	require.Equal(t, "keep_function", gjson.GetBytes(patched, "input.3.call_id").String())
	require.Equal(t, "keep_function", gjson.GetBytes(patched, "input.4.call_id").String())
REDACTED

func TestSanitizeGrokResponsesModelInputPairsCallsInTwoPasses(t *testing.T) {
	body := []byte(`{
		"input":[
			{"type":"function_call_output","output":{"before":true,"large_id":9007199254740993REDACTEDREDACTED,
			{"type":"function_call","function":{"name":"lookup","arguments":{"query":"x","large_id":9007199254740993REDACTEDREDACTEDREDACTED,
			{"type":"custom_tool_call","id":"custom_1","name":"apply_patch","input":"patch"REDACTED,
			{"type":"custom_tool_call_output","tool_call_id":"custom_1","output":["ok"]REDACTED,
			{"type":"tool_search_call","id":"search_1","arguments":{"query":"repo"REDACTEDREDACTED,
			{"type":"tool_search_output","call_id":"search_1","results":{"groups":["repo"]REDACTEDREDACTED,
			{"role":"tool","tool_call_id":"orphan","content":{"value":1REDACTEDREDACTED
		]REDACTED`)

	patched, err := sanitizeGrokResponsesModelInput(body)
REDACTED
	generated := gjson.GetBytes(patched, "input.1.call_id").String()
	require.NotEmpty(t, generated)
	require.Equal(t, generated, gjson.GetBytes(patched, "input.0.call_id").String())
	require.JSONEq(t, `{"before":true,"large_id":9007199254740993REDACTED`, gjson.GetBytes(patched, "input.0.output").String())
	require.Equal(t, "9007199254740993", gjson.Get(gjson.GetBytes(patched, "input.0.output").String(), "large_id").Raw)
	require.Equal(t, "lookup", gjson.GetBytes(patched, "input.1.name").String())
	require.JSONEq(t, `{"query":"x","large_id":9007199254740993REDACTED`, gjson.GetBytes(patched, "input.1.arguments").String())
	require.Equal(t, "9007199254740993", gjson.Get(gjson.GetBytes(patched, "input.1.arguments").String(), "large_id").Raw)
	customCallID := gjson.GetBytes(patched, "input.2.call_id").String()
	require.NotEmpty(t, customCallID)
	require.NotEqual(t, "custom_1", customCallID)
	require.JSONEq(t, `{"input":"patch"REDACTED`, gjson.GetBytes(patched, "input.2.arguments").String())
	require.Equal(t, customCallID, gjson.GetBytes(patched, "input.3.call_id").String())
	require.JSONEq(t, `["ok"]`, gjson.GetBytes(patched, "input.3.output").String())
	require.Equal(t, "tool_search", gjson.GetBytes(patched, "input.4.name").String())
	searchCallID := gjson.GetBytes(patched, "input.4.call_id").String()
	require.NotEmpty(t, searchCallID)
	require.NotEqual(t, "search_1", searchCallID)
	require.Equal(t, searchCallID, gjson.GetBytes(patched, "input.5.call_id").String())
	require.JSONEq(t, `{"groups":["repo"]REDACTED`, gjson.GetBytes(patched, "input.5.output").String())
	require.Equal(t, "function_call_output", gjson.GetBytes(patched, "input.6.type").String())
	require.Equal(t, "orphan", gjson.GetBytes(patched, "input.6.call_id").String())
REDACTED

func TestSanitizeGrokResponsesModelInputMapsAllItemAliasesAndRejectsConflicts(t *testing.T) {
	body := []byte(`{"input":[
		{"type":"function_call","id":"fc_alias","call_id":"call_canonical","name":"one","arguments":"{REDACTED"REDACTED,
		{"type":"function_call_output","id":"fc_alias","output":"ok"REDACTED,
		{"type":"function_call","id":"duplicate_item","name":"two","arguments":"{REDACTED"REDACTED,
		{"type":"function_call","id":"duplicate_item","name":"three","arguments":"{REDACTED"REDACTED,
		{"type":"function_call_output","id":"duplicate_item","output":"ambiguous"REDACTED
	]REDACTED`)

	patched, err := sanitizeGrokResponsesModelInput(body)
REDACTED
	require.Equal(t, "call_canonical", gjson.GetBytes(patched, "input.0.call_id").String())
	require.Equal(t, "call_canonical", gjson.GetBytes(patched, "input.1.call_id").String())
	require.False(t, gjson.GetBytes(patched, "input.0.id").Exists())
	secondCallID := gjson.GetBytes(patched, "input.2.call_id").String()
	thirdCallID := gjson.GetBytes(patched, "input.3.call_id").String()
	ambiguousOutputID := gjson.GetBytes(patched, "input.4.call_id").String()
	require.NotEmpty(t, secondCallID)
	require.NotEmpty(t, thirdCallID)
	require.NotEmpty(t, ambiguousOutputID)
	require.NotEqual(t, "duplicate_item", secondCallID)
	require.NotEqual(t, secondCallID, thirdCallID)
	require.NotEqual(t, secondCallID, ambiguousOutputID)
	require.NotEqual(t, thirdCallID, ambiguousOutputID)
REDACTED

func TestPatchGrokResponsesBodyCombinesAdditionalToolsAndDropsOrphanControls(t *testing.T) {
	body := []byte(`{
		"input":[{"type":"additional_tools","tools":[{"type":"function","name":"lookup"REDACTED]REDACTED,{"role":"user","content":"hi"REDACTED],
		"tools":[{"type":"function","name":"lookup"REDACTED],
		"tool_choice":"auto","parallel_tool_calls":true
REDACTED`)
	patched, err := patchGrokResponsesBody(body, "grok-4.5")
REDACTED
	require.Equal(t, int64(1), gjson.GetBytes(patched, "tools.#").Int())
	require.Equal(t, "lookup", gjson.GetBytes(patched, "tools.0.name").String())
	require.Equal(t, "message", gjson.GetBytes(patched, "input.0.type").String())

	withoutTools, err := patchGrokResponsesBody([]byte(`{"input":"hi","tool_choice":"auto","parallel_tool_calls":trueREDACTED`), "grok-4.5")
REDACTED
	require.False(t, gjson.GetBytes(withoutTools, "tool_choice").Exists())
	require.False(t, gjson.GetBytes(withoutTools, "parallel_tool_calls").Exists())

	for _, malformedTools := range []string{"null", `{REDACTED`, `"invalid"`REDACTED {
		malformedBody := []byte(`{"input":"hi","tools":` + malformedTools + `,"tool_choice":"auto","parallel_tool_calls":trueREDACTED`)
		patched, err := patchGrokResponsesBody(malformedBody, "grok-4.5")
	REDACTED
		require.False(t, gjson.GetBytes(patched, "tool_choice").Exists(), string(patched))
		require.False(t, gjson.GetBytes(patched, "parallel_tool_calls").Exists(), string(patched))
REDACTED
REDACTED

func TestSanitizeGrokCompactionReplayBodyPreservesVisibleSummary(t *testing.T) {
	body := []byte(`{
		"previous_response_id":"resp_stale",
		"input":[{"type":"compaction","encrypted_content":"opaque","summary":[{"type":"summary_text","text":"visible history"REDACTED]REDACTED,{"type":"message","role":"user","content":"continue"REDACTED]
REDACTED`)
	patched, changed, err := sanitizeGrokCompactionReplayBody(body)
REDACTED
	require.True(t, changed)
	require.False(t, gjson.GetBytes(patched, "previous_response_id").Exists())
	require.False(t, gjson.GetBytes(patched, `input.#(type=="reasoning")`).Exists())
	require.Contains(t, gjson.GetBytes(patched, "input.0.content.0.text").String(), "visible history")
	require.Equal(t, "continue", gjson.GetBytes(patched, "input.1.content").String())
	require.True(t, isGrokCompactionReplayDecodeError(http.StatusBadRequest, []byte(`{"error":{"message":"failed to deserialize compaction summary history"REDACTEDREDACTED`)))
	require.True(t, isGrokCompactionReplayDecodeError(http.StatusBadRequest, []byte(`{"error":{"type":"invalid_request_error"REDACTED,"detail":"could not decode compaction blob"REDACTED`)))
REDACTED

func TestGrokStructuredErrorCandidatesDoNotShadowTopLevelMessages(t *testing.T) {
	shadowedInvalidEncrypted := []byte(`{"code":"invalid-argument","error":{"type":"invalid_request_error"REDACTED,"message":"Could not decrypt encrypted_content because it was modified"REDACTED`)
	require.True(t, isGrokInvalidEncryptedContentResponse(http.StatusBadRequest, shadowedInvalidEncrypted))
	require.True(t, isGrokCompactionReplayDecodeError(http.StatusBadRequest, []byte(`{"error":{"type":"invalid_request_error"REDACTED,"message":"could not decode compaction history"REDACTED`)))
REDACTED

func TestGrokDecoderCompatibility422FailsOverWithoutCooldown(t *testing.T) {
	body := []byte(`{"detail":"data did not match any variant of untagged enum ModelInput at input[3]"REDACTED`)
	require.True(t, isGrokDecoderCompatibilityError(http.StatusUnprocessableEntity, body))
	require.True(t, isGrokDecoderCompatibilityError(http.StatusUnprocessableEntity, []byte(`{"message":"could not decode ModelInput at input.3"REDACTED`)))
	require.True(t, isGrokDecoderCompatibilityError(http.StatusUnprocessableEntity, []byte(`{"error":{"type":"invalid_request_error"REDACTED,"message":"could not deserialize ModelInput at input[3]"REDACTED`)))
	require.True(t, isGrokDecoderCompatibilityError(http.StatusUnprocessableEntity, []byte(`{"error":"Failed to deserialize the JSON body into the target type: messages[1]: data did not match any variant of untagged enum Content at line 1 column 6577"REDACTED`)))
	require.True(t, (&OpenAIGatewayService{REDACTED).shouldFailoverGrokUpstreamError(http.StatusUnprocessableEntity, body))
	decision := classifyGrokUpstreamFailure(http.StatusUnprocessableEntity, body, "grok-4.5")
	require.False(t, decision.ShouldCooldown)
	require.Equal(t, GrokFailureNone, decision.Class)

	require.False(t, isGrokDecoderCompatibilityError(http.StatusUnprocessableEntity, []byte(`{"error":{"message":"invalid user parameter"REDACTEDREDACTED`)))
	require.False(t, isGrokDecoderCompatibilityError(http.StatusUnprocessableEntity, []byte(`{"error":"messages[1].content is required"REDACTED`)))
	require.False(t, isGrokDecoderCompatibilityError(http.StatusBadRequest, body))
REDACTED
