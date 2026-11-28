package apicompat

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// terminalEvents returns every response-level terminal event in order.
func terminalEvents(events []ResponsesStreamEvent) []ResponsesStreamEvent {
	var out []ResponsesStreamEvent
	for _, e := range events {
		switch e.Type {
		case "response.completed", "response.incomplete", "response.failed", "response.done":
			out = append(out, e)
		}
	}
	return out
}

// A Chat Completions stream truncated by max_tokens (finish_reason=length) must
// terminate with response.incomplete, matching response.status; a client that
// switches on the event type otherwise treats the truncation as a normal finish.
func TestChatBridgeTerminalEvent_LengthEmitsIncomplete(t *testing.T) {
	events := collectStreamEvents(t, []string{
		`{"choices":[{"index":0,"delta":{"role":"assistant","content":"partial ans"}}]}`,
		`{"choices":[{"index":0,"delta":{},"finish_reason":"length"}]}`,
	})

	terminals := terminalEvents(events)
	require.Len(t, terminals, 1, "exactly one terminal event")
	term := terminals[0]
	require.Equal(t, "response.incomplete", term.Type)
	require.Equal(t, term.Type, events[len(events)-1].Type, "terminal event must be last")
	require.NotNil(t, term.Response)
	require.Equal(t, "incomplete", term.Response.Status)
	require.NotNil(t, term.Response.IncompleteDetails)
	require.Equal(t, "max_output_tokens", term.Response.IncompleteDetails.Reason)
	require.Len(t, term.Response.Output, 1)
	require.Equal(t, "partial ans", term.Response.Output[0].Content[0].Text)

	sse, err := ResponsesEventToSSE(term)
	require.NoError(t, err)
	require.True(t, strings.HasPrefix(sse, "event: response.incomplete\n"), sse)
}

// Invariant: every non-length finish keeps response.completed / status=completed.
func TestChatBridgeTerminalEvent_NonLengthKeepsCompleted(t *testing.T) {
	cases := map[string][]string{
		"stop": {
			`{"choices":[{"index":0,"delta":{"role":"assistant","content":"done"}}]}`,
			`{"choices":[{"index":0,"delta":{},"finish_reason":"stop"}]}`,
		},
		"tool_calls": {
			`{"choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"id":"call_a","type":"function","function":{"name":"exec","arguments":"{}"}}]}}]}`,
			`{"choices":[{"index":0,"delta":{},"finish_reason":"tool_calls"}]}`,
		},
		"no_finish_reason": {
			`{"choices":[{"index":0,"delta":{"role":"assistant","content":"eof"}}]}`,
		},
	}
	for name, chunks := range cases {
		t.Run(name, func(t *testing.T) {
			terminals := terminalEvents(collectStreamEvents(t, chunks))
			require.Len(t, terminals, 1)
			require.Equal(t, "response.completed", terminals[0].Type)
			require.NotNil(t, terminals[0].Response)
			require.Equal(t, "completed", terminals[0].Response.Status)
			require.Nil(t, terminals[0].Response.IncompleteDetails)
		})
	}
}

// Round trip through the repo's own Responses→Chat converter: the renamed
// terminal event is still recognized and maps back to finish_reason=length.
func TestChatBridgeTerminalEvent_IncompleteRoundTripsToLength(t *testing.T) {
	events := collectStreamEvents(t, []string{
		`{"choices":[{"index":0,"delta":{"role":"assistant","content":"cut"}}]}`,
		`{"choices":[{"index":0,"delta":{},"finish_reason":"length"}]}`,
	})
	state := NewResponsesEventToChatState()
	state.Model = "gpt-4o"
	var finish string
	for i := range events {
		for _, chunk := range ResponsesEventToChatChunks(&events[i], state) {
			for _, choice := range chunk.Choices {
				if choice.FinishReason != nil {
					finish = *choice.FinishReason
				}
			}
		}
	}
	require.Equal(t, "length", finish)
}
