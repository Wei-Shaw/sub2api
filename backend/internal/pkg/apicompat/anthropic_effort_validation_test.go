package apicompat

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestAnthropicToResponsesDeferredEffortValidation(t *testing.T) {
	for _, effort := range []string{"", "low", "high", "max"} {
		t.Run(effort, func(t *testing.T) {
			req := &AnthropicRequest{
				Model:        "gpt-6.1-sol",
				MaxTokens:    1,
				Thinking:     &AnthropicThinking{Type: "disabled"},
				OutputConfig: &AnthropicOutputConfig{Effort: effort},
			}
			_, err := AnthropicToResponses(req)
			require.Error(t, err)
			_, err = AnthropicToResponsesWithMinOutputTokens(req, 32)
			require.Error(t, err)
			converted, err := AnthropicToResponsesWithOptions(req, AnthropicToResponsesOptions{
				MinOutputTokens:                32,
				DeferReasoningEffortValidation: true,
			})
			require.NoError(t, err)
			require.Equal(t, "gpt-6.1-sol", converted.Model)
			require.Equal(t, "none", converted.Reasoning.Effort)
			require.Empty(t, converted.Reasoning.Summary)
			require.Equal(t, 32, *converted.MaxOutputTokens)
			require.Equal(t, "disabled", req.Thinking.Type)
			require.Equal(t, effort, req.OutputConfig.Effort)
		})
	}
}
