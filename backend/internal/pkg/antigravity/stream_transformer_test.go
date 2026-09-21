package antigravity

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

const malformedFunctionCallFixture = `{"response":{"candidates":[{"content":{"role":"model","parts":[{"thoughtSignature":"test-signature"}]},"finishReason":"MALFORMED_FUNCTION_CALL"}],"usageMetadata":{"promptTokenCount":12,"candidatesTokenCount":3}},"responseId":"test-response"}`

func TestStreamingMalformedFunctionCall(t *testing.T) {
	for _, prefix := range []string{"", `{"text":"partial"}`, `{"thought":true,"text":"thinking"}`, `{"functionCall":{"name":"lookup","args":{}}}`} {
		t.Run(prefix, func(t *testing.T) {
			p := NewStreamingProcessor("test-model")
			if prefix != "" {
				p.ProcessLine(`data: {"response":{"candidates":[{"content":{"parts":[` + prefix + `]}}]}}`)
			}
			out := string(p.ProcessLine("data: " + malformedFunctionCallFixture))
			require.Contains(t, out, "event: error")
			require.ErrorIs(t, p.Err(), ErrMalformedFunctionCall)
			var event map[string]any
			payload := strings.TrimSpace(strings.SplitN(out, "data: ", 2)[1])
			require.NoError(t, json.Unmarshal([]byte(payload), &event))
			require.Equal(t, "error", event["type"])
			require.Equal(t, "api_error", event["error"].(map[string]any)["type"])
			require.Contains(t, out, "MALFORMED_FUNCTION_CALL")
			require.NotContains(t, out, "message_delta")
			require.NotContains(t, out, "message_stop")
			require.NotContains(t, out, "test-signature")
			for i := 0; i < 2; i++ {
				tail, usage := p.Finish()
				require.Empty(t, tail, "EOF must not turn failure into success")
				require.Equal(t, 12, usage.InputTokens)
				require.Equal(t, 3, usage.OutputTokens)
				require.Empty(t, p.ProcessLine("data: "+malformedFunctionCallFixture), "terminal error must be emitted once")
			}
		})
	}
}

func TestStreamingNormalFinishReasons(t *testing.T) {
	for _, tc := range []struct{ reason, part, want string }{
		{"STOP", `{"text":"ok"}`, "end_turn"},
		{"MAX_TOKENS", `{"text":"partial"}`, "max_tokens"},
		{"STOP", `{"functionCall":{"name":"lookup","args":{}}}`, "tool_use"},
		{"STOP", `{"thoughtSignature":"test-signature"}`, "end_turn"},
	} {
		t.Run(tc.want+tc.part, func(t *testing.T) {
			p := NewStreamingProcessor("test-model")
			out := string(p.ProcessLine(`data: {"response":{"candidates":[{"content":{"parts":[` + tc.part + `]},"finishReason":"` + tc.reason + `"}]}}`))
			require.Contains(t, out, `"stop_reason":"`+tc.want+`"`)
			require.NotContains(t, out, "event: error")
			tail, _ := p.Finish()
			require.Empty(t, tail)
		})
	}
}

func TestNonStreamingMalformedFunctionCall(t *testing.T) {
	for _, wrapped := range []bool{true, false} {
		body := malformedFunctionCallFixture
		if !wrapped {
			var response map[string]json.RawMessage
			require.NoError(t, json.Unmarshal([]byte(body), &response))
			body = string(response["response"])
		}
		out, _, err := TransformGeminiToClaude([]byte(body), "test-model")
		require.ErrorIs(t, err, ErrMalformedFunctionCall)
		require.Empty(t, out)
	}
	// A malformed final chunk must not leak its own otherwise-valid tool call.
	body := strings.ReplaceAll(malformedFunctionCallFixture, `{"thoughtSignature":"test-signature"}`, `{"functionCall":{"name":"must_not_execute","args":{}}}`)
	out := NewStreamingProcessor("test-model").ProcessLine("data: " + body)
	require.NotContains(t, string(out), "must_not_execute")
	require.Contains(t, string(out), "event: error")
}
