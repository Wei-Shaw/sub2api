package apicompat

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

// #6569: Anthropic rejects thinking.budget_tokens >= max_tokens. The matrix
// mirrors the reported probes on claude-opus-4-6, where every effort above
// medium returned 400 without max_output_tokens (Codex never sends it).
func TestResponsesToAnthropicRequestKeepsMaxTokensAboveThinkingBudget(t *testing.T) {
	intPtr := func(v int) *int { return &v }
	tests := []struct {
		name          string
		effort        string
		maxOutput     *int
		wantBudget    int
		wantMaxTokens int
	}{
		{name: "medium default fits", effort: "medium", wantBudget: 4096, wantMaxTokens: 8192},
		{name: "high default is raised", effort: "high", wantBudget: 10240, wantMaxTokens: 11240},
		{name: "high 8192 is raised", effort: "high", maxOutput: intPtr(8192), wantBudget: 10240, wantMaxTokens: 11240},
		{name: "high 10000 is raised", effort: "high", maxOutput: intPtr(10000), wantBudget: 10240, wantMaxTokens: 11240},
		{name: "high equal to budget is raised", effort: "high", maxOutput: intPtr(10240), wantBudget: 10240, wantMaxTokens: 11240},
		{name: "high 12000 already fits", effort: "high", maxOutput: intPtr(12000), wantBudget: 10240, wantMaxTokens: 12000},
		{name: "xhigh default is raised", effort: "xhigh", wantBudget: 32768, wantMaxTokens: 33768},
		{name: "xhigh 32000 is raised", effort: "xhigh", maxOutput: intPtr(32000), wantBudget: 32768, wantMaxTokens: 33768},
		{name: "xhigh 64000 already fits", effort: "xhigh", maxOutput: intPtr(64000), wantBudget: 32768, wantMaxTokens: 64000},
		{name: "max default is raised", effort: "max", wantBudget: 32768, wantMaxTokens: 33768},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out, err := ResponsesToAnthropicRequest(&ResponsesRequest{
				Model:           "claude-opus-4-6",
				Input:           json.RawMessage(`"hi"`),
				MaxOutputTokens: tt.maxOutput,
				Reasoning:       &ResponsesReasoning{Effort: tt.effort},
			})

			require.NoError(t, err)
			require.NotNil(t, out.Thinking)
			require.Equal(t, "enabled", out.Thinking.Type)
			require.Equal(t, tt.wantBudget, out.Thinking.BudgetTokens)
			require.Equal(t, tt.wantMaxTokens, out.MaxTokens)
			require.Greater(t, out.MaxTokens, out.Thinking.BudgetTokens)
		})
	}
}

func TestResponsesToAnthropicRequestLeavesMaxTokensWithoutEnabledThinking(t *testing.T) {
	low, err := ResponsesToAnthropicRequest(&ResponsesRequest{
		Model: "claude-opus-4-6", Input: json.RawMessage(`"hi"`), Reasoning: &ResponsesReasoning{Effort: "low"},
	})
	require.NoError(t, err)
	require.Nil(t, low.Thinking)
	require.Equal(t, 8192, low.MaxTokens)

	noReasoning, err := ResponsesToAnthropicRequest(&ResponsesRequest{Model: "claude-opus-4-6", Input: json.RawMessage(`"hi"`)})
	require.NoError(t, err)
	require.Nil(t, noReasoning.Thinking)
	require.Equal(t, 8192, noReasoning.MaxTokens)
}

func TestEnsureMaxTokensAboveThinkingBudgetIgnoresNonEnabledThinking(t *testing.T) {
	adaptive := &AnthropicRequest{MaxTokens: 8192, Thinking: &AnthropicThinking{Type: "adaptive", BudgetTokens: 10240}}
	ensureMaxTokensAboveThinkingBudget(adaptive)
	require.Equal(t, 8192, adaptive.MaxTokens)

	ensureMaxTokensAboveThinkingBudget(nil)
	noThinking := &AnthropicRequest{MaxTokens: 100}
	ensureMaxTokensAboveThinkingBudget(noThinking)
	require.Equal(t, 100, noThinking.MaxTokens)
}
