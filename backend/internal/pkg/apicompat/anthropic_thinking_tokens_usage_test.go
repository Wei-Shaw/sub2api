//go:build unit

package apicompat

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestAnthropicUsageOutputTokensDetailsRoundtrip(t *testing.T) {
	var usage AnthropicUsage
	require.NoError(t, json.Unmarshal([]byte(`{"input_tokens":12,"output_tokens":120,"output_tokens_details":{"thinking_tokens":40}}`), &usage))
	require.NotNil(t, usage.OutputTokensDetails)
	require.Equal(t, 40, usage.OutputTokensDetails.ThinkingTokens)

	encoded, err := json.Marshal(usage)
	require.NoError(t, err)
	require.Contains(t, string(encoded), `"output_tokens_details":{"thinking_tokens":40}`)

	encoded, err = json.Marshal(AnthropicUsage{InputTokens: 1, OutputTokens: 2})
	require.NoError(t, err)
	require.NotContains(t, string(encoded), "output_tokens_details")
}

func TestAnthropicThinkingTokensBecomeReasoningTokens(t *testing.T) {
	for _, test := range []struct {
		name    string
		details *AnthropicOutputTokensDetails
		want    int
	}{
		{"reported", &AnthropicOutputTokensDetails{ThinkingTokens: 40}, 40},
		{"bounded by output_tokens", &AnthropicOutputTokensDetails{ThinkingTokens: 500}, 120},
		{"reported zero", &AnthropicOutputTokensDetails{ThinkingTokens: 0}, 0},
		{"absent", nil, 0},
	} {
		t.Run(test.name, func(t *testing.T) {
			resp := &AnthropicResponse{
				ID:         "msg_thinking",
				Type:       "message",
				Role:       "assistant",
				Model:      "claude-sonnet-4-5",
				StopReason: AnthropicStopReasonPtr("end_turn"),
				Content:    []AnthropicContentBlock{{Type: "text", Text: "ok"}},
				Usage:      AnthropicUsage{InputTokens: 12, OutputTokens: 120, OutputTokensDetails: test.details},
			}
			responses := AnthropicToResponsesResponse(resp)
			chat := ResponsesToChatCompletions(responses, "claude-sonnet-4-5")
			require.NotNil(t, chat.Usage)
			require.Equal(t, 120, chat.Usage.CompletionTokens)
			if test.want == 0 {
				require.Nil(t, responses.Usage.OutputTokensDetails)
				require.Nil(t, chat.Usage.CompletionTokensDetails)
				return
			}
			require.NotNil(t, responses.Usage.OutputTokensDetails)
			require.Equal(t, test.want, responses.Usage.OutputTokensDetails.ReasoningTokens)
			require.NotNil(t, chat.Usage.CompletionTokensDetails)
			require.Equal(t, test.want, chat.Usage.CompletionTokensDetails.ReasoningTokens)
		})
	}
}

func TestAnthropicStreamThinkingTokensUseLatestReportedValue(t *testing.T) {
	for _, test := range []struct {
		name  string
		delta string
		want  int
	}{
		{"delta reports thinking", `,"output_tokens_details":{"thinking_tokens":40}`, 40},
		{"delta reports zero", `,"output_tokens_details":{"thinking_tokens":0}`, 0},
		{"delta omits details", ``, 25},
	} {
		t.Run(test.name, func(t *testing.T) {
			state := NewAnthropicEventToResponsesState()
			var completed *ResponsesUsage
			for _, payload := range []string{
				`{"type":"message_start","message":{"id":"msg_s","type":"message","role":"assistant","content":[],"model":"claude-sonnet-4-5","usage":{"input_tokens":12,"output_tokens":1,"output_tokens_details":{"thinking_tokens":25}}}}`,
				`{"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}`,
				`{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"ok"}}`,
				`{"type":"content_block_stop","index":0}`,
				`{"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"output_tokens":120` + test.delta + `}}`,
				`{"type":"message_stop"}`,
			} {
				var event AnthropicStreamEvent
				require.NoError(t, json.Unmarshal([]byte(payload), &event))
				for _, out := range AnthropicEventToResponsesEvents(&event, state) {
					if out.Type == "response.completed" {
						completed = out.Response.Usage
					}
				}
			}
			require.NotNil(t, completed)
			require.Equal(t, 120, completed.OutputTokens)
			if test.want == 0 {
				require.Nil(t, completed.OutputTokensDetails)
				return
			}
			require.NotNil(t, completed.OutputTokensDetails)
			require.Equal(t, test.want, completed.OutputTokensDetails.ReasoningTokens)
		})
	}
}
