package apicompat

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

const refusalText = "I can't help with that."

func decodeResponsesEvent(t *testing.T, raw string) ResponsesStreamEvent {
	t.Helper()
	var evt ResponsesStreamEvent
	require.NoError(t, json.Unmarshal([]byte(raw), &evt))
	return evt
}

func chatMessageJSON(t *testing.T, resp *ChatCompletionsResponse) map[string]any {
	t.Helper()
	raw, err := json.Marshal(resp)
	require.NoError(t, err)
	var decoded struct {
		Choices []struct {
			Message map[string]any `json:"message"`
		} `json:"choices"`
	}
	require.NoError(t, json.Unmarshal(raw, &decoded))
	require.Len(t, decoded.Choices, 1)
	return decoded.Choices[0].Message
}

// A refusal arrives as a "refusal" content part, not output_text. Dropping it
// leaves the Chat client with an empty assistant message.
func TestResponsesToChatCompletions_RefusalPreserved(t *testing.T) {
	var resp ResponsesResponse
	require.NoError(t, json.Unmarshal([]byte(`{
		"id":"resp_refusal",
		"status":"completed",
		"output":[{"type":"message","role":"assistant","content":[{"type":"refusal","refusal":"`+refusalText+`"}]}]
	}`), &resp))

	chat := ResponsesToChatCompletions(&resp, "gpt-4o")

	msg := chatMessageJSON(t, chat)
	require.Equal(t, refusalText, msg["refusal"])
	require.NotContains(t, msg, "content")
	require.Equal(t, "stop", chat.Choices[0].FinishReason)
}

func TestResponsesEventToChatChunks_RefusalDelta(t *testing.T) {
	state := NewResponsesEventToChatState()
	events := []string{
		`{"type":"response.created","response":{"id":"resp_refusal","model":"gpt-4o"}}`,
		`{"type":"response.output_item.added","output_index":0,"item":{"type":"message","role":"assistant"}}`,
		`{"type":"response.refusal.delta","output_index":0,"content_index":0,"delta":"I can't "}`,
		`{"type":"response.refusal.delta","output_index":0,"content_index":0,"delta":"help with that."}`,
		`{"type":"response.refusal.done","output_index":0,"content_index":0,"refusal":"` + refusalText + `"}`,
		`{"type":"response.completed","response":{"id":"resp_refusal","status":"completed"}}`,
	}

	var refusal strings.Builder
	var finishReason string
	for _, raw := range events {
		evt := decodeResponsesEvent(t, raw)
		for _, chunk := range ResponsesEventToChatChunks(&evt, state) {
			encoded, err := json.Marshal(chunk)
			require.NoError(t, err)
			var decoded struct {
				Choices []struct {
					Delta struct {
						Refusal *string `json:"refusal"`
					} `json:"delta"`
					FinishReason *string `json:"finish_reason"`
				} `json:"choices"`
			}
			require.NoError(t, json.Unmarshal(encoded, &decoded))
			for _, choice := range decoded.Choices {
				if choice.Delta.Refusal != nil {
					_, _ = refusal.WriteString(*choice.Delta.Refusal)
				}
				if choice.FinishReason != nil {
					finishReason = *choice.FinishReason
				}
			}
		}
	}

	require.Equal(t, refusalText, refusal.String())
	require.Equal(t, "stop", finishReason)
}

// Non-streaming Chat requests are served from a forced upstream stream; when
// the terminal event carries an empty output array the message is rebuilt from
// the accumulated deltas, which must include refusal deltas.
func TestBufferedResponseAccumulator_RefusalRebuiltFromDeltas(t *testing.T) {
	acc := NewBufferedResponseAccumulator()
	for _, raw := range []string{
		`{"type":"response.output_item.added","output_index":0,"item":{"type":"message","role":"assistant"}}`,
		`{"type":"response.refusal.delta","output_index":0,"content_index":0,"delta":"I can't "}`,
		`{"type":"response.refusal.delta","output_index":0,"content_index":0,"delta":"help with that."}`,
	} {
		evt := decodeResponsesEvent(t, raw)
		acc.ProcessEvent(&evt)
	}
	require.True(t, acc.HasContent())

	final := &ResponsesResponse{ID: "resp_refusal", Status: "completed"}
	acc.SupplementResponseOutput(final)
	chat := ResponsesToChatCompletions(final, "gpt-4o")

	require.Equal(t, refusalText, chatMessageJSON(t, chat)["refusal"])
}
