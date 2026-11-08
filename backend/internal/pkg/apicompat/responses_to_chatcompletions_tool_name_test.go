package apicompat

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestResponsesEventToChatChunks_ArgumentsDeltaOmitsEmptyName pins that
// streamed arguments-only deltas never re-emit the tool name as an empty
// string. OpenAI-compatible clients accumulate the name from the first
// tool-call delta; a trailing "name":"" would overwrite the accumulated name
// and break tool dispatch ("unknown tool").
func TestResponsesEventToChatChunks_ArgumentsDeltaOmitsEmptyName(t *testing.T) {
	state := NewResponsesEventToChatState()

	added := ResponsesEventToChatChunks(&ResponsesStreamEvent{
		Type:        "response.output_item.added",
		OutputIndex: 0,
		Item: &ResponsesOutput{
			Type:   "function_call",
			CallID: "call_a",
			Name:   "exec",
		},
	}, state)
	require.Len(t, added, 1)
	first, err := json.Marshal(added[0])
	require.NoError(t, err)
	require.Contains(t, string(first), `"name":"exec"`)

	for _, fragment := range []string{`{"cmd":"ls"}`, `{"cmd":"ls","flags":"-la"}`} {
		deltas := ResponsesEventToChatChunks(&ResponsesStreamEvent{
			Type:        "response.function_call_arguments.delta",
			OutputIndex: 0,
			Delta:       fragment,
		}, state)
		require.Len(t, deltas, 1)
		raw, err := json.Marshal(deltas[0])
		require.NoError(t, err)
		require.NotContains(t, string(raw), `"name"`, "arguments delta must not carry a name field")
		require.Contains(t, string(raw), `"arguments"`)
	}
}
