package apicompat

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Codex-style Responses requests omit max_output_tokens; the converted
// Anthropic request must leave room for thinking plus the visible answer.
func TestResponsesToAnthropicRequest_DefaultMaxTokens(t *testing.T) {
	for _, tc := range []struct{ model, effort string }{
		{"claude-opus-5-5", "high"},  // adaptive thinking
		{"claude-sonnet-5", "xhigh"}, // budgeted thinking (max → 32768)
	} {
		req := &ResponsesRequest{
			Model:     tc.model,
			Input:     json.RawMessage(`"hi"`),
			Reasoning: &ResponsesReasoning{Effort: tc.effort},
		}
		out, err := ResponsesToAnthropicRequest(req)
		require.NoError(t, err, tc.model)
		assert.Equal(t, 64000, out.MaxTokens, tc.model)
		if out.Thinking != nil && out.Thinking.BudgetTokens > 0 {
			assert.Greater(t, out.MaxTokens, out.Thinking.BudgetTokens, tc.model)
		}
	}
}

func TestResponsesToAnthropicRequest_ExplicitMaxOutputTokensWins(t *testing.T) {
	v := 2048
	req := &ResponsesRequest{Model: "claude-opus-5-5", Input: json.RawMessage(`"hi"`), MaxOutputTokens: &v}
	out, err := ResponsesToAnthropicRequest(req)
	require.NoError(t, err)
	assert.Equal(t, 2048, out.MaxTokens)
}
