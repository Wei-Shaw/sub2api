package apicompat

import (
	"encoding/json"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/pkg/minimax"
	"github.com/stretchr/testify/require"
)

func TestMiniMaxM31ReasoningConversions(t *testing.T) {
	for _, effort := range []string{"", "low", "medium", "high", "xhigh", "max"} {
		t.Run(effort, func(t *testing.T) {
			request := &AnthropicRequest{Model: minimax.ModelM31FlashPreview, MaxTokens: 16384, Messages: []AnthropicMessage{{Role: "user", Content: json.RawMessage(`"hi"`)}}}
			if effort != "" {
				request.OutputConfig = &AnthropicOutputConfig{Effort: effort}
			}
			responses, err := AnthropicToResponses(request)
			require.NoError(t, err)
			chat, err := AnthropicToChatCompletionsRequest(request)
			require.NoError(t, err)
			require.Equal(t, effort, chat.ReasoningEffort)
			if effort == "" {
				require.Nil(t, responses.Reasoning)
			} else {
				require.Equal(t, effort, responses.Reasoning.Effort)
			}
			roundTrip, err := ResponsesToAnthropicRequest(responses)
			require.NoError(t, err)
			if effort == "" {
				require.Nil(t, roundTrip.OutputConfig)
				require.Nil(t, roundTrip.Thinking)
			} else {
				require.Equal(t, effort, roundTrip.OutputConfig.Effort)
				require.Equal(t, "adaptive", roundTrip.Thinking.Type)
				require.Zero(t, roundTrip.Thinking.BudgetTokens)
			}
		})
	}
}

func TestMiniMaxM31ConvertersRejectDisabledThinking(t *testing.T) {
	request := &AnthropicRequest{Model: minimax.ModelM31FlashPreview, Thinking: &AnthropicThinking{Type: "disabled"}}
	_, err := AnthropicToResponses(request)
	require.Error(t, err)
	_, err = AnthropicToChatCompletionsRequest(request)
	require.Error(t, err)
	_, err = ResponsesToAnthropicRequest(&ResponsesRequest{Model: minimax.ModelM31FlashPreview, Input: json.RawMessage(`"hi"`), Reasoning: &ResponsesReasoning{Effort: "none"}})
	require.Error(t, err)
}
