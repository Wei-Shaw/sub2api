package apicompat

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResponsesToAnthropicRequest_ThinkingBudgetRespectsMaxTokens(t *testing.T) {
	t.Run("default_max_tokens_raised_for_high_effort", func(t *testing.T) {
		req := &ResponsesRequest{
			Model:     "claude-sonnet-4-6",
			Input:     json.RawMessage(`"hi"`),
			Reasoning: &ResponsesReasoning{Effort: "high"},
		}

		out, err := ResponsesToAnthropicRequest(req)
		require.NoError(t, err)
		require.NotNil(t, out.Thinking)
		assert.Equal(t, 10240, out.Thinking.BudgetTokens)
		assert.Greater(t, out.MaxTokens, out.Thinking.BudgetTokens)
	})

	t.Run("default_max_tokens_raised_for_max_effort", func(t *testing.T) {
		req := &ResponsesRequest{
			Model:     "claude-sonnet-4-6",
			Input:     json.RawMessage(`"hi"`),
			Reasoning: &ResponsesReasoning{Effort: "max"},
		}

		out, err := ResponsesToAnthropicRequest(req)
		require.NoError(t, err)
		require.NotNil(t, out.Thinking)
		assert.Equal(t, 32768, out.Thinking.BudgetTokens)
		assert.Greater(t, out.MaxTokens, out.Thinking.BudgetTokens)
	})

	t.Run("explicit_small_max_output_tokens_caps_budget", func(t *testing.T) {
		small := 4096
		req := &ResponsesRequest{
			Model:           "claude-sonnet-4-6",
			Input:           json.RawMessage(`"hi"`),
			MaxOutputTokens: &small,
			Reasoning:       &ResponsesReasoning{Effort: "high"},
		}

		out, err := ResponsesToAnthropicRequest(req)
		require.NoError(t, err)
		require.NotNil(t, out.Thinking)
		assert.Equal(t, 4096, out.MaxTokens)
		assert.Equal(t, 4095, out.Thinking.BudgetTokens)
	})

	t.Run("medium_effort_keeps_defaults", func(t *testing.T) {
		req := &ResponsesRequest{
			Model:     "claude-sonnet-4-6",
			Input:     json.RawMessage(`"hi"`),
			Reasoning: &ResponsesReasoning{Effort: "medium"},
		}

		out, err := ResponsesToAnthropicRequest(req)
		require.NoError(t, err)
		require.NotNil(t, out.Thinking)
		assert.Equal(t, 8192, out.MaxTokens)
		assert.Equal(t, 4096, out.Thinking.BudgetTokens)
		assert.Greater(t, out.MaxTokens, out.Thinking.BudgetTokens)
	})

	t.Run("low_effort_has_no_thinking", func(t *testing.T) {
		req := &ResponsesRequest{
			Model:     "claude-sonnet-4-6",
			Input:     json.RawMessage(`"hi"`),
			Reasoning: &ResponsesReasoning{Effort: "low"},
		}

		out, err := ResponsesToAnthropicRequest(req)
		require.NoError(t, err)
		assert.Nil(t, out.Thinking)
		assert.Equal(t, 8192, out.MaxTokens)
	})
}

func TestResponsesToAnthropicRequest_Claude5UsesAdaptiveThinking(t *testing.T) {
	tests := []struct {
		name   string
		model  string
		effort string
	}{
		{"sonnet_5_high", "claude-sonnet-5", "high"},
		{"opus_5_max", "claude-opus-5", "max"},
		{"fable_5_medium", "claude-fable-5", "medium"},
		{"sonnet_5_dated_suffix", "claude-sonnet-5-20260825", "medium"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := &ResponsesRequest{
				Model:     tt.model,
				Input:     json.RawMessage(`"hi"`),
				Reasoning: &ResponsesReasoning{Effort: tt.effort},
			}

			out, err := ResponsesToAnthropicRequest(req)
			require.NoError(t, err)
			require.NotNil(t, out.Thinking)
			assert.Equal(t, "adaptive", out.Thinking.Type)
			assert.Zero(t, out.Thinking.BudgetTokens)
			require.NotNil(t, out.OutputConfig)
			assert.Equal(t, mapResponsesEffortToAnthropic(tt.effort), out.OutputConfig.Effort)
			assert.Equal(t, 8192, out.MaxTokens)
		})
	}
}

func TestResponsesToAnthropicRequest_LegacyClaudeKeepsEnabledThinking(t *testing.T) {
	req := &ResponsesRequest{
		Model:     "claude-haiku-4-5",
		Input:     json.RawMessage(`"hi"`),
		Reasoning: &ResponsesReasoning{Effort: "high"},
	}

	out, err := ResponsesToAnthropicRequest(req)
	require.NoError(t, err)
	require.NotNil(t, out.Thinking)
	assert.Equal(t, "enabled", out.Thinking.Type)
	assert.Equal(t, 10240, out.Thinking.BudgetTokens)
}
