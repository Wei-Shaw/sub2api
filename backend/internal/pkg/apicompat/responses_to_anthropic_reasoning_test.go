package apicompat

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestResponsesToAnthropic_ReasoningUsesModelCapabilities(t *testing.T) {
	maxTokens := 20000
	tests := []struct {
		name             string
		model            string
		effort           string
		wantOutputEffort string
		wantThinkingType string
		wantThinking     bool
	}{
		{
			name:             "haiku 4.5 uses manual thinking",
			model:            "claude-haiku-4-5-20251001",
			effort:           "high",
			wantThinkingType: "enabled",
			wantThinking:     true,
		},
		{
			name:             "sonnet 4.6 uses adaptive thinking",
			model:            "claude-sonnet-4-6",
			effort:           "high",
			wantOutputEffort: "high",
			wantThinkingType: "adaptive",
			wantThinking:     true,
		},
		{
			name:             "opus 4.5 clamps unsupported max to high",
			model:            "claude-opus-4-5-20251101",
			effort:           "xhigh",
			wantOutputEffort: "high",
			wantThinkingType: "adaptive",
			wantThinking:     true,
		},
		{
			name:   "unknown model omits optional reasoning fields",
			model:  "claude-future-9",
			effort: "high",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out, err := ResponsesToAnthropicRequest(&ResponsesRequest{
				Model:           tt.model,
				Input:           json.RawMessage("\"hello\""),
				MaxOutputTokens: &maxTokens,
				Reasoning:       &ResponsesReasoning{Effort: tt.effort},
			})
			require.NoError(t, err)

			if tt.wantOutputEffort == "" {
				require.Nil(t, out.OutputConfig)
			} else {
				require.NotNil(t, out.OutputConfig)
				require.Equal(t, tt.wantOutputEffort, out.OutputConfig.Effort)
			}

			if !tt.wantThinking {
				require.Nil(t, out.Thinking)
				return
			}
			require.NotNil(t, out.Thinking)
			require.Equal(t, tt.wantThinkingType, out.Thinking.Type)
		})
	}
}

func TestResponsesToAnthropic_ManualThinkingDisablesWhenMaxTokensTooSmall(t *testing.T) {
	maxTokens := 1024
	out, err := ResponsesToAnthropicRequest(&ResponsesRequest{
		Model:           "claude-haiku-4-5-20251001",
		Input:           json.RawMessage("\"hello\""),
		MaxOutputTokens: &maxTokens,
		Reasoning:       &ResponsesReasoning{Effort: "high"},
	})
	require.NoError(t, err)
	require.Nil(t, out.OutputConfig)
	require.Nil(t, out.Thinking)
}

func TestResponsesToAnthropic_ReapplyUsesFinalMappedModel(t *testing.T) {
	maxTokens := 20000
	out, err := ResponsesToAnthropicRequest(&ResponsesRequest{
		Model:           "claude-haiku-4-5-20251001",
		Input:           json.RawMessage("\"hello\""),
		MaxOutputTokens: &maxTokens,
		Reasoning:       &ResponsesReasoning{Effort: "high"},
	})
	require.NoError(t, err)
	require.Nil(t, out.OutputConfig)
	require.Equal(t, "enabled", out.Thinking.Type)

	ReapplyResponsesReasoningToAnthropic(out, "claude-sonnet-4-6", "high")
	require.NotNil(t, out.OutputConfig)
	require.Equal(t, "high", out.OutputConfig.Effort)
	require.NotNil(t, out.Thinking)
	require.Equal(t, "adaptive", out.Thinking.Type)
}
