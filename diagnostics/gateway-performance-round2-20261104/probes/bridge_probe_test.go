package apicompat

import (
	"fmt"
	"strings"
	"testing"
)

var round2BridgeEventSink []ResponsesStreamEvent

func BenchmarkRound2ChatToolArguments(b *testing.B) {
	for _, size := range []int{64 << 10, 256 << 10, 1 << 20} {
		arguments := `{"input":"` + strings.Repeat("x", size) + `"}`
		idx := 0
		b.Run(fmt.Sprintf("%dKiB", size>>10), func(b *testing.B) {
			b.ReportAllocs()
			b.SetBytes(int64(len(arguments)))
			for range b.N {
				state := NewChatCompletionsToResponsesStreamState("gpt-5.1")
				for offset := 0; offset < len(arguments); offset += 1024 {
					call := ChatToolCall{Index: &idx, Type: "function"}
					if offset == 0 {
						call.ID, call.Function.Name = "call_round2", "lookup"
					}
					call.Function.Arguments = arguments[offset:min(offset+1024, len(arguments))]
					round2BridgeEventSink = ChatCompletionsChunkToResponsesEvents(&ChatCompletionsChunk{Choices: []ChatChunkChoice{{Delta: ChatDelta{ToolCalls: []ChatToolCall{call}}}}}, state)
				}
				if err := state.ValidateToolCallArguments(); err != nil {
					b.Fatal(err)
				}
				round2BridgeEventSink = FinalizeChatCompletionsResponsesStream(state)
				if state.ToolCalls[0].Function.Arguments != arguments {
					b.Fatal("arguments changed")
				}
			}
		})
	}
}
