package antigravity

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// #6985: Gemini 3.x must receive thinkingLevel (not thinkingBudget),
// otherwise upstream returns zero thought summaries.
func TestBuildGenerationConfig_Gemini3ThinkingLevel(t *testing.T) {
	cases := []struct {
		name      string
		model     string
		budget    int
		wantLevel string
	}{
		{name: "tiered defaults to high", model: "gemini-3.8-flash-tiered", budget: 0, wantLevel: "high"},
		{name: "low suffix", model: "gemini-3.8-flash-low", budget: 2048, wantLevel: "low"},
		{name: "medium suffix", model: "gemini-3.8-flash-medium", budget: 2048, wantLevel: "medium"},
		{name: "small budget downgrades tiered to low", model: "gemini-3.8-flash-tiered", budget: 1024, wantLevel: "low"},
		{name: "mid budget downgrades tiered to medium", model: "gemini-3.8-flash-tiered", budget: 8192, wantLevel: "medium"},
		{name: "bare model defaults to high", model: "gemini-3-pro", budget: 0, wantLevel: "high"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := &ClaudeRequest{
				Model:     tc.model,
				MaxTokens: 64000,
				Thinking:  &ThinkingConfig{Type: "enabled", BudgetTokens: tc.budget},
			}
			cfg := buildGenerationConfig(req)
			require.NotNil(t, cfg.ThinkingConfig)
			require.True(t, cfg.ThinkingConfig.IncludeThoughts)
			require.Equal(t, tc.wantLevel, cfg.ThinkingConfig.ThinkingLevel)
			require.Equal(t, 0, cfg.ThinkingConfig.ThinkingBudget)
		})
	}

	t.Run("gemini 2.5 keeps budget without level", func(t *testing.T) {
		req := &ClaudeRequest{
			Model:     "gemini-2.5-flash",
			MaxTokens: 64000,
			Thinking:  &ThinkingConfig{Type: "enabled", BudgetTokens: 4096},
		}
		cfg := buildGenerationConfig(req)
		require.NotNil(t, cfg.ThinkingConfig)
		require.Empty(t, cfg.ThinkingConfig.ThinkingLevel)
		require.Equal(t, 4096, cfg.ThinkingConfig.ThinkingBudget)
	})
}
