//go:build unit

package antigravity

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestStreamingProcessorAbnormalTermination(t *testing.T) {
	cases := []struct {
		name    string
		payload string
		reason  string
	}{
		{
			name:    "malformed function call",
			payload: `{"response":{"candidates":[{"content":{"parts":[{"text":"planning","thought":true}]},"finishReason":"MALFORMED_FUNCTION_CALL"}]}}`,
			reason:  "malformed_function_call",
		},
		{
			name:    "unexpected tool call",
			payload: `{"response":{"candidates":[{"content":{"parts":[{"text":"partial"}]},"finishReason":"UNEXPECTED_TOOL_CALL"}]}}`,
			reason:  "unexpected_tool_call",
		},
		{
			name:    "too many tool calls",
			payload: `{"response":{"candidates":[{"content":{"parts":[{"text":"partial"}]},"finishReason":"TOO_MANY_TOOL_CALLS"}]}}`,
			reason:  "too_many_tool_calls",
		},
		{
			name:    "thinking only with STOP",
			payload: `{"response":{"candidates":[{"content":{"parts":[{"text":"planning","thought":true}]},"finishReason":"STOP"}]}}`,
			reason:  "thinking_only_response",
		},
		{
			name:    "thinking only without finish reason",
			payload: `{"response":{"candidates":[{"content":{"parts":[{"text":"planning","thought":true}]}}]}}`,
			reason:  "upstream_stream_truncated",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			processor := NewStreamingProcessor("gemini-3.1-pro")
			out := processor.ProcessLine("data: " + tc.payload)
			final, _ := processor.Finish()

			require.Equal(t, tc.reason, processor.AbnormalReason())
			stream := string(out) + string(final)
			require.NotContains(t, stream, "event: message_delta")
			require.NotContains(t, stream, `"type":"message_stop"`)
		})
	}
}

func TestStreamingProcessorKeepsValidTerminalBehavior(t *testing.T) {
	tests := []struct {
		name       string
		payload    string
		finish     bool
		wantReason string
	}{
		{
			name:    "text without finish reason",
			payload: `{"response":{"candidates":[{"content":{"parts":[{"text":"answer"}]}}]}}`,
			finish:  true,
		},
		{
			name:    "empty STOP remains a finished step",
			payload: `{"response":{"candidates":[{"content":{"parts":[]},"finishReason":"STOP"}]}}`,
		},
		{
			name:    "thinking with MAX_TOKENS",
			payload: `{"response":{"candidates":[{"content":{"parts":[{"text":"planning","thought":true}]},"finishReason":"MAX_TOKENS"}]}}`,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			processor := NewStreamingProcessor("gemini-3.1-pro")
			out := processor.ProcessLine("data: " + tc.payload)
			if tc.finish {
				final, _ := processor.Finish()
				out = append(out, final...)
			}

			require.Equal(t, tc.wantReason, processor.AbnormalReason())
			stream := string(out)
			require.Contains(t, stream, `"type":"message_stop"`)
			if strings.Contains(tc.payload, `"finishReason":"MAX_TOKENS"`) {
				require.Contains(t, stream, `"stop_reason":"max_tokens"`)
			}
		})
	}
}
