package apicompat

import (
	"encoding/json"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/stretchr/testify/require"
)

func TestCustomToolInputStreamEverySplit(t *testing.T) {
	for _, arguments := range []string{
		`{"input":"hello world"}`,
		` { "input" : "line\n\t\r\b\f\\\"\/end" } `,
		`{"input":"中文😀é"}`,
		`{"input":"\u4e2d\u6587\ud83d\ude00\uD800x\udc00"}`,
		`{"other":{"input":"ignored"},"in\u0070ut":"actual"}`,
		`{"input":""}`,
	} {
		t.Run(arguments, func(t *testing.T) {
			want := extractCustomToolCallInput(arguments)
			for split := 0; split <= len(arguments); split++ {
				var stream customToolInputStream
				first := stream.delta(arguments[:split])
				second := stream.delta(arguments)
				require.True(t, utf8.ValidString(first))
				require.True(t, utf8.ValidString(second))
				require.Equal(t, want, first+second, "split %d", split)
				require.Empty(t, stream.delta(arguments), "must not replay input")
			}
			var stream customToolInputStream
			var got strings.Builder
			for i := 1; i <= len(arguments); i++ {
				delta := stream.delta(arguments[:i])
				require.True(t, utf8.ValidString(delta))
				got.WriteString(delta)
			}
			require.Equal(t, want, got.String())
		})
	}
}

func TestResponsesClientToolStreamRestorerParallelIncrementalInput(t *testing.T) {
	r := NewResponsesClientToolStreamRestorer(ResponsesClientToolMapping{CustomTools: map[string]bool{"exec": true}})
	for i, id := range []string{"fc_a", "fc_b"} {
		r.Restore(ResponsesStreamEvent{Type: "response.output_item.added", OutputIndex: i, Item: &ResponsesOutput{Type: "function_call", ID: id, CallID: id, Name: "exec"}})
	}
	for i, id := range []string{"fc_a", "fc_b"} {
		events := r.Restore(ResponsesStreamEvent{Type: "response.function_call_arguments.delta", ItemID: id, Delta: `{"input":"` + id})
		require.Len(t, events, 1, "client must receive input before upstream completes")
		require.Equal(t, id, events[0].Delta)
		require.Equal(t, "ctc_"+id[3:], events[0].ItemID)
		require.Equal(t, i, events[0].OutputIndex)
		require.Equal(t, i+2, events[0].SequenceNumber)
	}
	for _, id := range []string{"fc_b", "fc_a"} {
		// Exercise upstreams which omit arguments.done entirely.
		events := r.Restore(ResponsesStreamEvent{Type: "response.output_item.done", Item: &ResponsesOutput{Type: "function_call", ID: id, CallID: id, Name: "exec", Arguments: `{"input":"` + id + `!"}`}})
		require.Len(t, events, 3)
		require.Equal(t, "!", events[0].Delta)
		require.Equal(t, id+"!", events[1].Input)
		require.Equal(t, id+"!", events[2].Item.Input)
	}
	require.NoError(t, r.err)
}

func TestResponsesClientToolStreamRestorerFallbackAndConflictingFinal(t *testing.T) {
	for _, arguments := range []string{`raw code`, `{"input":42}`, `{"other":"value"}`, `{}`} {
		r := NewResponsesClientToolStreamRestorer(ResponsesClientToolMapping{CustomTools: map[string]bool{"exec": true}})
		r.Restore(ResponsesStreamEvent{Type: "response.output_item.added", Item: &ResponsesOutput{Type: "function_call", ID: "fc_a", Name: "exec"}})
		require.Empty(t, r.Restore(ResponsesStreamEvent{Type: "response.function_call_arguments.delta", ItemID: "fc_a", Delta: arguments}))
		done := r.Restore(ResponsesStreamEvent{Type: "response.function_call_arguments.done", ItemID: "fc_a", Arguments: arguments})
		require.Equal(t, extractCustomToolCallInput(arguments), done[len(done)-1].Input)
		require.NoError(t, r.err)
	}
	for _, final := range []string{`{"input":"different"}`, `{"input":"hello"`, `{"input":"hello","input":"changed"}`} {
		r := NewResponsesClientToolStreamRestorer(ResponsesClientToolMapping{CustomTools: map[string]bool{"exec": true}})
		r.Restore(ResponsesStreamEvent{Type: "response.output_item.added", Item: &ResponsesOutput{Type: "function_call", ID: "fc_a", Name: "exec"}})
		r.Restore(ResponsesStreamEvent{Type: "response.function_call_arguments.delta", ItemID: "fc_a", Delta: `{"input":"hello`})
		payload, err := json.Marshal(ResponsesStreamEvent{Type: "response.function_call_arguments.done", ItemID: "fc_a", Arguments: final})
		require.NoError(t, err)
		out, _, err := r.RestoreEvent(payload)
		require.ErrorContains(t, err, "changed after streamed prefix")
		require.Empty(t, out)
	}
}

func FuzzCustomToolInputStream(f *testing.F) {
	for _, seed := range []string{"hello", "中文😀", "line\nquote\"slash\\", ""} {
		f.Add(seed, uint8(1))
	}
	f.Fuzz(func(t *testing.T, input string, chunk uint8) {
		arguments, err := json.Marshal(map[string]string{"input": input})
		require.NoError(t, err)
		var stream customToolInputStream
		var got strings.Builder
		step := int(chunk) + 1
		for i := step; i < len(arguments); i += step {
			got.WriteString(stream.delta(string(arguments[:i])))
		}
		got.WriteString(stream.delta(string(arguments)))
		require.Equal(t, extractCustomToolCallInput(string(arguments)), got.String())
	})
}

func TestResponsesClientToolStreamRestorerRejectsChangedCompletedInput(t *testing.T) {
	for _, final := range []string{`{"input":"changed"}`, `{"input":"hello!"}`} {
		r := NewResponsesClientToolStreamRestorer(ResponsesClientToolMapping{CustomTools: map[string]bool{"exec": true}})
		r.Restore(ResponsesStreamEvent{Type: "response.output_item.added", Item: &ResponsesOutput{Type: "function_call", ID: "fc_a", Name: "exec"}})
		r.Restore(ResponsesStreamEvent{Type: "response.function_call_arguments.delta", ItemID: "fc_a", Delta: `{"input":"hel`})
		done := r.Restore(ResponsesStreamEvent{Type: "response.function_call_arguments.done", ItemID: "fc_a", Arguments: `{"input":"hello"}`})
		require.Len(t, done, 2)
		require.Equal(t, "lo", done[0].Delta)
		payload, err := json.Marshal(ResponsesStreamEvent{Type: "response.output_item.done", Item: &ResponsesOutput{Type: "function_call", ID: "fc_a", Name: "exec", Arguments: final}})
		require.NoError(t, err)
		out, _, err := r.RestoreEvent(payload)
		require.ErrorContains(t, err, "changed after streamed prefix")
		require.Empty(t, out)
	}
}
