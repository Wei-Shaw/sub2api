package apicompat

import (
	"encoding/json"
	"testing"
)

var benchmarkResponsesWire []byte

func BenchmarkResponsesStreamEventWire(b *testing.B) {
	for _, eventType := range []string{"response.output_text.delta", "response.reasoning_summary_text.delta", "response.function_call_arguments.delta", "response.custom_tool_call_input.done"} {
		b.Run(eventType, func(b *testing.B) {
			event := ResponsesStreamEvent{Type: eventType, ItemID: "item_wire", CallID: "call_wire", Name: "lookup", Delta: "a sample delta 你好\n", Input: "a sample input"}
			b.ReportAllocs()
			for range b.N {
				var err error
				benchmarkResponsesWire, err = json.Marshal(event)
				if err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
