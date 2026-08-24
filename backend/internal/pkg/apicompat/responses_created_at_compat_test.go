package apicompat

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestResponsesCreatedAtCompatibility_NonStreamingConverters(t *testing.T) {
	t.Run("shared response always marshals created_at", func(t *testing.T) {
		body, err := json.Marshal(&ResponsesResponse{})
		require.NoError(t, err)
		require.Contains(t, string(body), `"created_at"`)
	})

	t.Run("chat preserves positive upstream created and repairs missing", func(t *testing.T) {
		preserved := ChatCompletionsResponseToResponses(&ChatCompletionsResponse{Created: 123}, "model", nil, nil, false, nil)
		require.Equal(t, int64(123), preserved.CreatedAt)
		for _, input := range []*ChatCompletionsResponse{nil, {Created: 0}} {
			require.Positive(t, ChatCompletionsResponseToResponses(input, "model", nil, nil, false, nil).CreatedAt)
		}
	})

	t.Run("anthropic gets a gateway timestamp", func(t *testing.T) {
		out := AnthropicToResponsesResponse(&AnthropicResponse{ID: "msg_1", Model: "model"})
		require.Positive(t, out.CreatedAt)
	})
}

func TestResponsesCreatedAtCompatibility_StreamingLifecycleUsesOneTimestamp(t *testing.T) {
	chat := NewChatCompletionsToResponsesStreamState("model")
	chat.Created = 123
	chatEvents := append(ensureChatToResponsesCreated(chat), FinalizeChatCompletionsResponsesStream(chat)...)
	require.Len(t, chatEvents, 2)
	require.Equal(t, int64(123), chatEvents[0].Response.CreatedAt)
	require.Equal(t, chatEvents[0].Response.CreatedAt, chatEvents[1].Response.CreatedAt)

	anthropic := NewAnthropicEventToResponsesState()
	anthropic.ResponseID, anthropic.Model, anthropic.Created = "resp_1", "model", 456
	anthropic.CreatedSent = true
	anthropicEvents := append([]ResponsesStreamEvent{makeResponsesCreatedEvent(anthropic)}, makeResponsesCompletedEvent(anthropic, "incomplete", &ResponsesIncompleteDetails{Reason: "max_output_tokens"}))
	require.Equal(t, "response.created", anthropicEvents[0].Type)
	require.Equal(t, int64(456), anthropicEvents[0].Response.CreatedAt)
	require.Equal(t, "response.incomplete", anthropicEvents[1].Type)
	require.Equal(t, anthropicEvents[0].Response.CreatedAt, anthropicEvents[1].Response.CreatedAt)
}
