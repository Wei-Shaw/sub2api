package apicompat

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// requireReasoningBeforeOtherItems enforces the Responses stream contract: a
// reasoning output item, once opened, must be closed before any other output
// item is opened, and no reasoning item may be opened after another item type
// has already started. Upstream chat streams occasionally deliver
// reasoning_content after tool_calls; the bridge must not turn that into a
// reasoning item that opens mid-item, which strict clients reject with
// "invalid JSON arguments" / "changed output item identity".
func requireReasoningBeforeOtherItems(t *testing.T, events []ResponsesStreamEvent) {
	t.Helper()
	var opened, closed []string
	for _, e := range events {
		switch e.Type {
		case "response.output_item.added":
			require.NotNil(t, e.Item)
			require.Emptyf(t, opened, "output item %q added while %q is still open", e.Item.Type, opened)
			opened = append(opened, e.Item.Type)
		case "response.output_item.done":
			require.NotNil(t, e.Item)
			require.NotEmptyf(t, opened, "output item %q closed without being added", e.Item.Type)
			require.Equalf(t, opened[0], e.Item.Type, "closed item type does not match the open item")
			opened = opened[1:]
			closed = append(closed, e.Item.Type)
		case "response.reasoning_summary_text.delta", "response.reasoning_summary_text.done",
			"response.reasoning_summary_part.added", "response.reasoning_summary_part.done":
			require.NotEmptyf(t, opened, "%s emitted with no open output item", e.Type)
			require.Equalf(t, "reasoning", opened[0], "%s emitted outside a reasoning item", e.Type)
		}
	}
	require.Empty(t, opened, "stream ended with output items still open")
	// Once a non-reasoning item is done, no reasoning item may appear after it.
	seenOther := false
	for _, item := range closed {
		if item == "reasoning" {
			require.Falsef(t, seenOther, "reasoning item closed after a %q item", item)
		} else {
			seenOther = true
		}
	}
}

// TestStream_LateReasoningAfterToolCallDoesNotOpenNewItem covers the observed
// production failure: a chat upstream streams tool_calls first and only later
// streams reasoning_content. The bridge used to open a fresh reasoning item
// (output_index after the function_call) and close it mid-stream, which no
// strict client tolerates. The reasoning text must still reach
// response.completed.
func TestStream_LateReasoningAfterToolCallDoesNotOpenNewItem(t *testing.T) {
	events := collectStreamEvents(t, []string{
		`{"choices":[{"index":0,"delta":{"role":"assistant","tool_calls":[{"index":0,"id":"call_x1","type":"function","function":{"name":"exec","arguments":"{\"command\":"}}]}}]}`,
		`{"choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"function":{"arguments":"\"ls /tmp\"}"}}]}}]}`,
		`{"choices":[{"index":0,"delta":{"reasoning_content":"thinking late..."}}]}`,
		`{"choices":[{"index":0,"delta":{},"finish_reason":"tool_calls"}],"usage":{"prompt_tokens":1,"completion_tokens":2,"total_tokens":3}}`,
	})

	requireReasoningBeforeOtherItems(t, events)

	var reasoningItems, functionItems int
	var argsDone string
	for _, e := range events {
		if e.Type == "response.output_item.added" && e.Item != nil && e.Item.Type == "reasoning" {
			reasoningItems++
		}
		if e.Type == "response.output_item.added" && e.Item != nil && e.Item.Type == "function_call" {
			functionItems++
		}
		if e.Type == "response.function_call_arguments.done" {
			argsDone = e.Arguments
		}
	}
	require.Zero(t, reasoningItems, "late reasoning must not open a new output item after tool calls")
	require.Equal(t, 1, functionItems, "the function call item must still be streamed")
	require.JSONEq(t, `{"command":"ls /tmp"}`, argsDone)

	// The text is not dropped: it still lands on the response object, which
	// lists reasoning first just like a well-ordered stream does.
	var completed *ResponsesResponse
	for i := range events {
		if events[i].Type == "response.completed" {
			completed = events[i].Response
		}
	}
	require.NotNil(t, completed)
	require.NotEmpty(t, completed.Output)
	require.Equal(t, "reasoning", completed.Output[0].Type)
	require.Len(t, completed.Output[0].Summary, 1)
	require.Equal(t, "thinking late...", completed.Output[0].Summary[0].Text)
}

// TestStream_ReasoningBeforeToolCallKeepsItemOrder is the regression guard for
// the well-ordered case: reasoning first, then tool calls. Nothing about the
// fix may change this path.
func TestStream_ReasoningBeforeToolCallKeepsItemOrder(t *testing.T) {
	events := collectStreamEvents(t, []string{
		`{"choices":[{"index":0,"delta":{"role":"assistant","reasoning_content":"thinking first"}}]}`,
		`{"choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"id":"call_x1","type":"function","function":{"name":"exec","arguments":"{\"command\":\"ls\"}"}}]}}]}`,
		`{"choices":[{"index":0,"delta":{},"finish_reason":"tool_calls"}],"usage":{"prompt_tokens":1,"completion_tokens":2,"total_tokens":3}}`,
	})

	requireReasoningBeforeOtherItems(t, events)

	var order []string
	for _, e := range events {
		if e.Type == "response.output_item.added" && e.Item != nil {
			order = append(order, e.Item.Type)
		}
	}
	require.Equal(t, []string{"reasoning", "function_call"}, order)
}

// TestStream_LateReasoningAfterContentDoesNotOpenNewItem is the same contract
// for the message path: reasoning that trails the first content delta must not
// open an output item after the message item.
func TestStream_LateReasoningAfterContentDoesNotOpenNewItem(t *testing.T) {
	events := collectStreamEvents(t, []string{
		`{"choices":[{"index":0,"delta":{"role":"assistant","content":"hello"}}]}`,
		`{"choices":[{"index":0,"delta":{"reasoning_content":"thinking late..."}}]}`,
		`{"choices":[{"index":0,"delta":{"content":""},"finish_reason":"stop"}],"usage":{"prompt_tokens":1,"completion_tokens":2,"total_tokens":3}}`,
	})

	requireReasoningBeforeOtherItems(t, events)

	var reasoningItems int
	for _, e := range events {
		if e.Type == "response.output_item.added" && e.Item != nil && e.Item.Type == "reasoning" {
			reasoningItems++
		}
	}
	require.Zero(t, reasoningItems)
}
