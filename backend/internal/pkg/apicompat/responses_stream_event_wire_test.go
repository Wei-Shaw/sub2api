package apicompat

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

// marshalEvent marshals through the custom MarshalJSON and returns the decoded
// object plus the set of top-level keys.
func marshalEvent(t *testing.T, e ResponsesStreamEvent) map[string]any {
REDACTED
	b, err := json.Marshal(e)
REDACTED
	var m map[string]any
	require.NoError(t, json.Unmarshal(b, &m))
	return m
REDACTED

// TestWire_IndexFieldsPresentAtZero guards the omitempty trap: output_index/
// content_index/summary_index must serialize even when 0.
func TestWire_IndexFieldsPresentAtZero(t *testing.T) {
	m := marshalEvent(t, ResponsesStreamEvent{
		Type: "response.output_text.delta", OutputIndex: 0, ContentIndex: 0, ItemID: "msg_1", Delta: "hi",
REDACTED)
	require.Contains(t, m, "output_index")
	require.Contains(t, m, "content_index")
	require.EqualValues(t, 0, m["output_index"])

	r := marshalEvent(t, ResponsesStreamEvent{
		Type: "response.reasoning_summary_text.delta", OutputIndex: 0, SummaryIndex: 0, ItemID: "rs_1", Delta: "think",
REDACTED)
	require.Contains(t, r, "output_index")
	require.Contains(t, r, "summary_index")
REDACTED

// TestWire_FunctionCallItemAlwaysComplete guards that a function_call item
// always carries call_id/name/arguments, including arguments:"" on .added.
func TestWire_FunctionCallItemAlwaysComplete(t *testing.T) {
	added := marshalEvent(t, ResponsesStreamEvent{
		Type:        "response.output_item.added",
		OutputIndex: 1,
		Item:        &ResponsesOutput{Type: "function_call", ID: "fc_1", CallID: "call_a", Name: "exec", Status: "in_progress"REDACTED,
REDACTED)
	item, ok := added["item"].(map[string]any)
	require.True(t, ok, "item must be an object")
	for _, k := range []string{"call_id", "name", "arguments"REDACTED {
		require.Containsf(t, item, k, "function_call item missing %q", k)
REDACTED
	require.Equal(t, "", item["arguments"])
REDACTED

// TestWire_MessageItemContentAlwaysArray guards content:[] presence.
func TestWire_MessageItemContentAlwaysArray(t *testing.T) {
	m := marshalEvent(t, ResponsesStreamEvent{
		Type:        "response.output_item.added",
		OutputIndex: 0,
		Item:        &ResponsesOutput{Type: "message", ID: "msg_1", Role: "assistant", Status: "in_progress"REDACTED,
REDACTED)
	item, ok := m["item"].(map[string]any)
	require.True(t, ok, "item must be an object")
	require.Contains(t, item, "content")
	_, ok = item["content"].([]any)
	require.True(t, ok, "content must be an array")
REDACTED

// TestWire_ReasoningItemSummaryAlwaysArray guards summary:[] presence.
func TestWire_ReasoningItemSummaryAlwaysArray(t *testing.T) {
	m := marshalEvent(t, ResponsesStreamEvent{
		Type:        "response.output_item.added",
		OutputIndex: 0,
		Item:        &ResponsesOutput{Type: "reasoning", ID: "rs_1", Status: "in_progress"REDACTED,
REDACTED)
	item, ok := m["item"].(map[string]any)
	require.True(t, ok, "item must be an object")
	require.Contains(t, item, "summary")
	_, ok = item["summary"].([]any)
	require.True(t, ok, "summary must be an array")
REDACTED

// TestWire_ContentPartCarriesAnnotationsLogprobs guards the output_text part shape.
func TestWire_ContentPartCarriesAnnotationsLogprobs(t *testing.T) {
	m := marshalEvent(t, ResponsesStreamEvent{
		Type: "response.content_part.added", OutputIndex: 0, ContentIndex: 0, ItemID: "msg_1",
		Part: &ResponsesContentPart{Type: "output_text", Text: ""REDACTED,
REDACTED)
	part, ok := m["part"].(map[string]any)
	require.True(t, ok, "part must be an object")
	require.Equal(t, "output_text", part["type"])
	require.Contains(t, part, "text")
	require.Contains(t, part, "annotations")
	require.Contains(t, part, "logprobs")
REDACTED

// TestWire_ArgumentsDonePresentEvenEmpty guards arguments presence on done.
func TestWire_ArgumentsDonePresentEvenEmpty(t *testing.T) {
	m := marshalEvent(t, ResponsesStreamEvent{
		Type: "response.function_call_arguments.done", OutputIndex: 1, ItemID: "fc_1", CallID: "call_a", Name: "exec", Arguments: "",
REDACTED)
	require.Contains(t, m, "arguments")
	require.Equal(t, "", m["arguments"])
REDACTED

// TestWire_CustomToolCallInputIndexPresentAtZero guards the omitempty trap for
// custom_tool_call_input.delta/done: output_index must serialize even when 0
// (custom tool call as the first output item).
func TestWire_CustomToolCallInputIndexPresentAtZero(t *testing.T) {
	d := marshalEvent(t, ResponsesStreamEvent{
		Type: "response.custom_tool_call_input.delta", OutputIndex: 0, ItemID: "ct_1", Delta: "dir",
REDACTED)
	require.Contains(t, d, "output_index")
	require.EqualValues(t, 0, d["output_index"])
	require.Equal(t, "dir", d["delta"])

	done := marshalEvent(t, ResponsesStreamEvent{
		Type: "response.custom_tool_call_input.done", OutputIndex: 0, ItemID: "ct_1", CallID: "call_1", Name: "exec", Input: "dir",
REDACTED)
	require.Contains(t, done, "output_index")
	require.EqualValues(t, 0, done["output_index"])
	require.Equal(t, "dir", done["input"])
	require.NotContains(t, done, "delta")
REDACTED

// TestWire_UnknownEventFallsBackToDefault ensures non-streamed event types keep
// default marshalling (the response object is preserved).
func TestWire_UnknownEventFallsBackToDefault(t *testing.T) {
	m := marshalEvent(t, ResponsesStreamEvent{
		Type:     "response.completed",
		Response: &ResponsesResponse{ID: "resp_1", Object: "response", Status: "completed"REDACTED,
REDACTED)
	require.Contains(t, m, "response")
REDACTED
