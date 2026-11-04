package apicompat

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestAnthropicStreamFragmentSnapshotsSurviveFollowingBlocks(t *testing.T) {
	for _, kind := range []struct {
		block, delta, done string
	}{
		{"text", "text_delta", "response.output_text.done"},
		{"tool_use", "input_json_delta", "response.function_call_arguments.done"},
		{"thinking", "thinking_delta", "response.reasoning_summary_text.done"},
	} {
		t.Run(kind.block, func(t *testing.T) {
			state := NewAnthropicEventToResponsesState()
			AnthropicEventToResponsesEvents(&AnthropicStreamEvent{Type: "message_start", Message: &AnthropicResponse{ID: "msg_fragments", Model: "claude-sonnet-4-6"}}, state)
			var snapshots []string
			for _, text := range []string{"first\n你好", "second\t🙂"} {
				block := &AnthropicContentBlock{Type: kind.block, ID: "toolu_fragments", Name: "lookup"}
				AnthropicEventToResponsesEvents(&AnthropicStreamEvent{Type: "content_block_start", ContentBlock: block}, state)
				// Split by bytes as upstream JSON fragments can bisect a UTF-8 rune.
				for i := range len(text) {
					delta := &AnthropicDelta{Type: kind.delta, Text: text[i : i+1], Thinking: text[i : i+1], PartialJSON: text[i : i+1]}
					AnthropicEventToResponsesEvents(&AnthropicStreamEvent{Type: "content_block_delta", Delta: delta}, state)
				}
				events := AnthropicEventToResponsesEvents(&AnthropicStreamEvent{Type: "content_block_stop"}, state)
				var found bool
				for _, event := range events {
					if event.Type == kind.done {
						found = true
						if kind.block == "tool_use" {
							snapshots = append(snapshots, event.Arguments)
						} else {
							snapshots = append(snapshots, event.Text)
						}
					}
				}
				require.True(t, found, "missing done event")
			}
			terminal := AnthropicEventToResponsesEvents(&AnthropicStreamEvent{Type: "message_stop"}, state)
			require.Equal(t, []string{"first\n你好", "second\t🙂"}, snapshots)
			output := terminal[len(terminal)-1].Response.Output
			for i, want := range snapshots {
				switch kind.block {
				case "text":
					require.Equal(t, want, output[0].Content[i].Text)
				case "tool_use":
					require.Equal(t, want, output[i].Arguments)
				case "thinking":
					require.Equal(t, want, output[i].Summary[0].Text)
				}
			}
		})
	}
}

func BenchmarkAnthropicFragmentAccumulation(b *testing.B) {
	fragment := strings.Repeat("x", 1024)
	for _, kind := range []struct{ block, delta string }{
		{"text", "text_delta"},
		{"tool_use", "input_json_delta"},
		{"thinking", "thinking_delta"},
	} {
		b.Run(kind.block, func(b *testing.B) {
			b.ReportAllocs()
			b.SetBytes(256 << 10)
			for range b.N {
				state := NewAnthropicEventToResponsesState()
				AnthropicEventToResponsesEvents(&AnthropicStreamEvent{Type: "message_start", Message: &AnthropicResponse{Model: "claude-sonnet-4-6"}}, state)
				AnthropicEventToResponsesEvents(&AnthropicStreamEvent{Type: "content_block_start", ContentBlock: &AnthropicContentBlock{Type: kind.block}}, state)
				for range 256 {
					AnthropicEventToResponsesEvents(&AnthropicStreamEvent{Type: "content_block_delta", Delta: &AnthropicDelta{Type: kind.delta, Text: fragment, PartialJSON: fragment, Thinking: fragment}}, state)
				}
				AnthropicEventToResponsesEvents(&AnthropicStreamEvent{Type: "content_block_stop"}, state)
				AnthropicEventToResponsesEvents(&AnthropicStreamEvent{Type: "message_stop"}, state)
			}
		})
	}
}
