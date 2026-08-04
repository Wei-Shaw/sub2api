package apicompat

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// marshalEvent marshals through the custom MarshalJSON and returns the decoded
// object plus the set of top-level keys.
func marshalEvent(t *testing.T, e ResponsesStreamEvent) map[string]any {
	t.Helper()
	b, err := json.Marshal(e)
	require.NoError(t, err)
	var m map[string]any
	require.NoError(t, json.Unmarshal(b, &m))
	return m
}

// TestWire_IndexFieldsPresentAtZero guards the omitempty trap: output_index/
// content_index/summary_index must serialize even when 0.
func TestWire_IndexFieldsPresentAtZero(t *testing.T) {
	m := marshalEvent(t, ResponsesStreamEvent{
		Type: "response.output_text.delta", OutputIndex: 0, ContentIndex: 0, ItemID: "msg_1", Delta: "hi",
	})
	require.Contains(t, m, "output_index")
	require.Contains(t, m, "content_index")
	require.EqualValues(t, 0, m["output_index"])

	r := marshalEvent(t, ResponsesStreamEvent{
		Type: "response.reasoning_summary_text.delta", OutputIndex: 0, SummaryIndex: 0, ItemID: "rs_1", Delta: "think",
	})
	require.Contains(t, r, "output_index")
	require.Contains(t, r, "summary_index")
}

// TestWire_FunctionCallItemAlwaysComplete guards that a function_call item
// always carries call_id/name/arguments, including arguments:"" on .added.
func TestWire_FunctionCallItemAlwaysComplete(t *testing.T) {
	added := marshalEvent(t, ResponsesStreamEvent{
		Type:        "response.output_item.added",
		OutputIndex: 1,
		Item:        &ResponsesOutput{Type: "function_call", ID: "fc_1", CallID: "call_a", Name: "exec", Status: "in_progress"},
	})
	item, ok := added["item"].(map[string]any)
	require.True(t, ok, "item must be an object")
	for _, k := range []string{"call_id", "name", "arguments"} {
		require.Containsf(t, item, k, "function_call item missing %q", k)
	}
	require.Equal(t, "", item["arguments"])
}

// TestWire_MessageItemContentAlwaysArray guards content:[] presence.
func TestWire_MessageItemContentAlwaysArray(t *testing.T) {
	m := marshalEvent(t, ResponsesStreamEvent{
		Type:        "response.output_item.added",
		OutputIndex: 0,
		Item:        &ResponsesOutput{Type: "message", ID: "msg_1", Role: "assistant", Status: "in_progress"},
	})
	item, ok := m["item"].(map[string]any)
	require.True(t, ok, "item must be an object")
	require.Contains(t, item, "content")
	_, ok = item["content"].([]any)
	require.True(t, ok, "content must be an array")
}

// TestWire_MessageItemFillsIDStatusAndAnnotations guards the Grok/Codex strict
// shape: message id/status and output_text annotations/logprobs must be present
// even when emitters omit them.
func TestWire_MessageItemFillsIDStatusAndAnnotations(t *testing.T) {
	m := marshalEvent(t, ResponsesStreamEvent{
		Type:        "response.output_item.done",
		OutputIndex: 0,
		Item: &ResponsesOutput{
			Type: "message",
			Role: "assistant",
			Content: []ResponsesContentPart{
				{Type: "output_text", Text: "hello"},
			},
		},
	})
	item, ok := m["item"].(map[string]any)
	require.True(t, ok, "item must be an object")
	id, _ := item["id"].(string)
	require.True(t, strings.HasPrefix(id, "msg_"), "message id must be msg_…, got %q", id)
	require.Equal(t, "completed", item["status"])
	content, ok := item["content"].([]any)
	require.True(t, ok)
	require.Len(t, content, 1)
	part, ok := content[0].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "output_text", part["type"])
	require.Equal(t, "hello", part["text"])
	require.Contains(t, part, "annotations")
	require.Contains(t, part, "logprobs")
	_, ok = part["annotations"].([]any)
	require.True(t, ok, "annotations must be an array")
	_, ok = part["logprobs"].([]any)
	require.True(t, ok, "logprobs must be an array")
}

// TestWire_ReasoningItemSummaryAlwaysArray guards summary:[] presence.
func TestWire_ReasoningItemSummaryAlwaysArray(t *testing.T) {
	m := marshalEvent(t, ResponsesStreamEvent{
		Type:        "response.output_item.added",
		OutputIndex: 0,
		Item:        &ResponsesOutput{Type: "reasoning", ID: "rs_1", Status: "in_progress"},
	})
	item, ok := m["item"].(map[string]any)
	require.True(t, ok, "item must be an object")
	require.Contains(t, item, "summary")
	_, ok = item["summary"].([]any)
	require.True(t, ok, "summary must be an array")
}

// TestWire_ContentPartCarriesAnnotationsLogprobs guards the output_text part shape.
func TestWire_ContentPartCarriesAnnotationsLogprobs(t *testing.T) {
	m := marshalEvent(t, ResponsesStreamEvent{
		Type: "response.content_part.added", OutputIndex: 0, ContentIndex: 0, ItemID: "msg_1",
		Part: &ResponsesContentPart{
			Type:        "output_text",
			Text:        "",
			Annotations: []json.RawMessage{json.RawMessage(`{"type":"url_citation","url":"https://example.com"}`)},
		},
	})
	part, ok := m["part"].(map[string]any)
	require.True(t, ok, "part must be an object")
	require.Equal(t, "output_text", part["type"])
	require.Contains(t, part, "text")
	require.Contains(t, part, "annotations")
	require.Contains(t, part, "logprobs")
	annotations, ok := part["annotations"].([]any)
	require.True(t, ok)
	require.Len(t, annotations, 1)
	require.Equal(t, "https://example.com", annotations[0].(map[string]any)["url"])
}

func TestWire_ContentPartPreservesNonOutputTextShape(t *testing.T) {
	m := marshalEvent(t, ResponsesStreamEvent{
		Type: "response.content_part.added", OutputIndex: 0, ContentIndex: 0, ItemID: "msg_1",
		Part: &ResponsesContentPart{Type: "input_image", ImageURL: "data:image/png;base64,abc"},
	})
	part, ok := m["part"].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "input_image", part["type"])
	require.Equal(t, "data:image/png;base64,abc", part["image_url"])
	require.NotContains(t, part, "annotations")
	require.NotContains(t, part, "logprobs")
}

// TestWire_ArgumentsDonePresentEvenEmpty guards arguments presence on done.
func TestWire_ArgumentsDonePresentEvenEmpty(t *testing.T) {
	m := marshalEvent(t, ResponsesStreamEvent{
		Type: "response.function_call_arguments.done", OutputIndex: 1, ItemID: "fc_1", CallID: "call_a", Name: "exec", Arguments: "",
	})
	require.Contains(t, m, "arguments")
	require.Equal(t, "", m["arguments"])
}

// TestWire_CustomToolCallInputIndexPresentAtZero guards the omitempty trap for
// custom_tool_call_input.delta/done: output_index must serialize even when 0
// (custom tool call as the first output item).
func TestWire_CustomToolCallInputIndexPresentAtZero(t *testing.T) {
	d := marshalEvent(t, ResponsesStreamEvent{
		Type: "response.custom_tool_call_input.delta", OutputIndex: 0, ItemID: "ct_1", Delta: "dir",
	})
	require.Contains(t, d, "output_index")
	require.EqualValues(t, 0, d["output_index"])
	require.Equal(t, "dir", d["delta"])

	done := marshalEvent(t, ResponsesStreamEvent{
		Type: "response.custom_tool_call_input.done", OutputIndex: 0, ItemID: "ct_1", CallID: "call_1", Name: "exec", Input: "dir",
	})
	require.Contains(t, done, "output_index")
	require.EqualValues(t, 0, done["output_index"])
	require.Equal(t, "dir", done["input"])
	require.NotContains(t, done, "delta")
}

// TestWire_CompletedCarriesStrictMessageShape ensures response.completed nests
// a fully-shaped message/output_text (the Grok failure mode: missing annotations).
func TestWire_CompletedCarriesStrictMessageShape(t *testing.T) {
	m := marshalEvent(t, ResponsesStreamEvent{
		Type: "response.completed",
		Response: &ResponsesResponse{
			ID:     "resp_1",
			Object: "response",
			Model:  "gpt-test",
			Status: "completed",
			Output: []ResponsesOutput{{
				Type: "message",
				Role: "assistant",
				Content: []ResponsesContentPart{
					{Type: "output_text", Text: "你好"},
				},
			}},
		},
	})
	require.Contains(t, m, "response")
	resp, ok := m["response"].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "resp_1", resp["id"])
	output, ok := resp["output"].([]any)
	require.True(t, ok)
	require.Len(t, output, 1)
	item, ok := output[0].(map[string]any)
	require.True(t, ok)
	id, _ := item["id"].(string)
	require.True(t, strings.HasPrefix(id, "msg_"), "got %q", id)
	require.Equal(t, "completed", item["status"])
	content, ok := item["content"].([]any)
	require.True(t, ok)
	part, ok := content[0].(map[string]any)
	require.True(t, ok)
	require.Contains(t, part, "annotations")
	require.Contains(t, part, "logprobs")
	require.Equal(t, "你好", part["text"])
}

func TestWire_WebSearchActionSurvivesItemAndCompletedEvents(t *testing.T) {
	item := ResponsesOutput{
		Type:   "web_search_call",
		ID:     "ws_1",
		Status: "completed",
		Action: &WebSearchAction{Type: "search", Query: "strict wire"},
	}

	done := marshalEvent(t, ResponsesStreamEvent{
		Type: "response.output_item.done", OutputIndex: 0, Item: &item,
	})
	doneItem, ok := done["item"].(map[string]any)
	require.True(t, ok)
	doneAction, ok := doneItem["action"].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "search", doneAction["type"])
	require.Equal(t, "strict wire", doneAction["query"])

	completed := marshalEvent(t, ResponsesStreamEvent{
		Type: "response.completed",
		Response: &ResponsesResponse{
			ID: "resp_web", Object: "response", Status: "completed", Output: []ResponsesOutput{item},
		},
	})
	response := completed["response"].(map[string]any)
	output := response["output"].([]any)
	completedItem := output[0].(map[string]any)
	completedAction, ok := completedItem["action"].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "search", completedAction["type"])
	require.Equal(t, "strict wire", completedAction["query"])
}

func TestResponsesOutputUnmarshal_ToolSearchObjectArguments(t *testing.T) {
	var item ResponsesOutput
	require.NoError(t, json.Unmarshal([]byte(`{
		"type":"tool_search_call",
		"id":"item_1",
		"call_id":"call_1",
		"execution":"client",
		"arguments":{"query":"gmail","limit":2}
	}`), &item))
	require.Equal(t, "tool_search_call", item.Type)
	require.Equal(t, `{"query":"gmail","limit":2}`, item.Arguments)

	wire, err := json.Marshal(item)
	require.NoError(t, err)
	var decoded map[string]any
	require.NoError(t, json.Unmarshal(wire, &decoded))
	args, ok := decoded["arguments"].(map[string]any)
	require.True(t, ok, "tool_search_call arguments must remain an object")
	require.Equal(t, "gmail", args["query"])
}

func TestResponsesResponseUnmarshal_ToolSearchObjectArguments(t *testing.T) {
	var response ResponsesResponse
	require.NoError(t, json.Unmarshal([]byte(`{
		"id":"response_1",
		"object":"response",
		"status":"completed",
		"output":[{
			"type":"tool_search_call",
			"id":"item_1",
			"call_id":"call_1",
			"arguments":{"query":"gmail"}
		}]
	}`), &response))
	require.Len(t, response.Output, 1)
	require.Equal(t, `{"query":"gmail"}`, response.Output[0].Arguments)
}

func TestResponsesStreamEventUnmarshal_ToolSearchObjectArguments(t *testing.T) {
	var event ResponsesStreamEvent
	require.NoError(t, json.Unmarshal([]byte(`{
		"type":"response.output_item.done",
		"item":{
			"type":"tool_search_call",
			"id":"item_1",
			"call_id":"call_1",
			"arguments":{"query":"gmail"}
		}
	}`), &event))
	require.NotNil(t, event.Item)
	require.Equal(t, `{"query":"gmail"}`, event.Item.Arguments)
}
