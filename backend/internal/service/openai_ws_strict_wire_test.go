package service

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestOpenAIWSStrictWireNormalizerKeepsMessageLifecycleConsistent(t *testing.T) {
	normalizer := newOpenAIWSStrictWireNormalizer()

	added, changed := normalizer.normalize([]byte(`{
		"type":"response.output_item.added",
		"output_index":0,
		"item":{"type":"message","role":"assistant","phase":"final_answer","content":[]}
	}`))
	require.True(t, changed)
	messageID := gjson.GetBytes(added, "item.id").String()
	require.True(t, strings.HasPrefix(messageID, "msg_"))
	require.Equal(t, "in_progress", gjson.GetBytes(added, "item.status").String())

	done, changed := normalizer.normalize([]byte(`{
		"type":"response.output_item.done",
		"output_index":0,
		"item":{"type":"message","status":"completed","phase":"final_answer","role":"assistant","content":[{"type":"output_text","text":"MATRIX_TEXT_OK","annotations":[],"logprobs":[]}]}
	}`))
	require.True(t, changed)
	require.Equal(t, messageID, gjson.GetBytes(done, "item.id").String())

	completed, changed := normalizer.normalize([]byte(`{
		"type":"response.completed",
		"response":{"status":"completed","output":[{"type":"message","content":[{"type":"output_text","text":"MATRIX_TEXT_OK"}],"vendor_extension":{"keep":true}}]}
	}`))
	require.True(t, changed)
	require.Equal(t, messageID, gjson.GetBytes(completed, "response.output.0.id").String())
	require.Equal(t, "completed", gjson.GetBytes(completed, "response.output.0.status").String())
	require.Equal(t, "final_answer", gjson.GetBytes(completed, "response.output.0.phase").String())
	require.Equal(t, "MATRIX_TEXT_OK", gjson.GetBytes(completed, "response.output.0.content.0.text").String())
	require.True(t, gjson.GetBytes(completed, "response.output.0.content.0.annotations").IsArray())
	require.True(t, gjson.GetBytes(completed, "response.output.0.content.0.logprobs").IsArray())
	require.True(t, gjson.GetBytes(completed, "response.output.0.vendor_extension.keep").Bool())
}

func TestOpenAIWSStrictWireNormalizerRepairsFunctionCallTerminalItem(t *testing.T) {
	normalizer := newOpenAIWSStrictWireNormalizer()

	done, changed := normalizer.normalize([]byte(`{
		"type":"response.output_item.done",
		"output_index":0,
		"item":{"id":"fc_1","type":"function_call","status":"completed","call_id":"call_1","name":"lookup_order_status","arguments":"{}"}
	}`))
	require.False(t, changed, "a complete item needs no repair")
	require.Equal(t, "fc_1", gjson.GetBytes(done, "item.id").String())

	completed, changed := normalizer.normalize([]byte(`{
		"type":"response.completed",
		"response":{"status":"completed","output":[{"type":"function_call","call_id":"call_1","name":"lookup_order_status","arguments":"{}"}]}
	}`))
	require.True(t, changed)
	require.Equal(t, "fc_1", gjson.GetBytes(completed, "response.output.0.id").String())
	require.Equal(t, "completed", gjson.GetBytes(completed, "response.output.0.status").String())
	require.Equal(t, "lookup_order_status", gjson.GetBytes(completed, "response.output.0.name").String())
}

func TestOpenAIWSStrictWireNormalizerDoesNotTouchUnrelatedEvents(t *testing.T) {
	normalizer := newOpenAIWSStrictWireNormalizer()
	raw := []byte(`{"type":"response.output_text.delta","output_index":0,"delta":"hello"}`)
	got, changed := normalizer.normalize(raw)
	require.False(t, changed)
	require.Equal(t, string(raw), string(got))
}

func TestOpenAIWSStrictWireNormalizerMapsTerminalResponseStatusToItemStatus(t *testing.T) {
	for _, tc := range []struct {
		name           string
		eventType      string
		responseStatus string
		wantStatus     string
	}{
		{name: "failed", eventType: "response.failed", responseStatus: "failed", wantStatus: "incomplete"},
		{name: "cancelled", eventType: "response.cancelled", responseStatus: "cancelled", wantStatus: "incomplete"},
		{name: "incomplete", eventType: "response.incomplete", responseStatus: "incomplete", wantStatus: "incomplete"},
		{name: "completed", eventType: "response.completed", responseStatus: "completed", wantStatus: "completed"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			normalizer := newOpenAIWSStrictWireNormalizer()
			event := []byte(`{
				"type":"` + tc.eventType + `",
				"response":{"status":"` + tc.responseStatus + `","output":[{"type":"message","role":"assistant","content":[{"type":"output_text","text":"partial"}]}]}
			}`)
			normalized, changed := normalizer.normalize(event)
			require.True(t, changed)
			require.Equal(t, tc.wantStatus, gjson.GetBytes(normalized, "response.output.0.status").String())
		})
	}
}

func TestOpenAIWSStrictWireNormalizerKeepsFinishedItemStatusOnFailedResponse(t *testing.T) {
	normalizer := newOpenAIWSStrictWireNormalizer()

	_, changed := normalizer.normalize([]byte(`{
		"type":"response.output_item.done",
		"output_index":0,
		"item":{"id":"msg_1","type":"message","status":"completed","role":"assistant","content":[{"type":"output_text","text":"done","annotations":[],"logprobs":[]}]}
	}`))
	require.False(t, changed, "a complete item needs no repair")

	failed, changed := normalizer.normalize([]byte(`{
		"type":"response.failed",
		"response":{"status":"failed","output":[{"type":"message","role":"assistant","content":[{"type":"output_text","text":"done"}]}]}
	}`))
	require.True(t, changed)
	require.Equal(t, "msg_1", gjson.GetBytes(failed, "response.output.0.id").String())
	require.Equal(t, "completed", gjson.GetBytes(failed, "response.output.0.status").String(),
		"an item that already reported completed must not be downgraded by the response status")
}

func TestOpenAIWSStrictWireNormalizerLeavesUnknownResponseStatusAlone(t *testing.T) {
	normalizer := newOpenAIWSStrictWireNormalizer()

	normalized, _ := normalizer.normalize([]byte(`{
		"type":"response.completed",
		"response":{"status":"some_vendor_state","output":[{"type":"message","role":"assistant","content":[{"type":"output_text","text":"hi"}]}]}
	}`))
	require.False(t, gjson.GetBytes(normalized, "response.output.0.status").Exists(),
		"an unmappable response status must not be copied onto the item")
}
