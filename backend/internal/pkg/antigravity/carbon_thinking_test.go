package antigravity

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCarbonGenerationThinkingLevels(t *testing.T) {
	for _, tc := range []struct {
		name, body, level string
		thoughts          bool
	}{
		{"default", `{}`, "", false},
		{"low", `{"output_config":{"effort":"low"}}`, "low", false},
		{"medium", `{"output_config":{"effort":"medium"}}`, "medium", false},
		{"high", `{"output_config":{"effort":"high"}}`, "high", false},
		{"disabled wins", `{"thinking":{"type":"disabled"},"output_config":{"effort":"high"}}`, "low", false},
		{"low budget", `{"thinking":{"type":"enabled","budget_tokens":1024}}`, "low", true},
		{"medium budget", `{"thinking":{"type":"enabled","budget_tokens":4096}}`, "medium", true},
		{"high budget", `{"thinking":{"type":"enabled","budget_tokens":32768}}`, "high", true},
		{"adaptive default", `{"thinking":{"type":"adaptive"}}`, "", true},
		{"effort wins", `{"thinking":{"type":"enabled","budget_tokens":32768},"output_config":{"effort":"medium"}}`, "medium", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var req ClaudeRequest
			require.NoError(t, json.Unmarshal([]byte(tc.body), &req))
			req.Model = "gemini-4-carbon"
			req.MaxTokens = 1000
			config := buildGenerationConfig(&req)
			data, err := json.Marshal(config)
			require.NoError(t, err)
			require.NotContains(t, string(data), "thinkingBudget")
			require.Equal(t, 1000, config.MaxOutputTokens, "Carbon budgets must not raise output limits")
			if tc.level == "" && !tc.thoughts {
				require.Nil(t, config.ThinkingConfig)
				return
			}
			require.NotNil(t, config.ThinkingConfig)
			require.Equal(t, tc.level, config.ThinkingConfig.ThinkingLevel)
			require.Equal(t, tc.thoughts, config.ThinkingConfig.IncludeThoughts)
		})
	}
}

func TestCarbonTierPreservesUpstreamSelection(t *testing.T) {
	for _, tier := range []string{"low", "medium", "high"} {
		cfg := buildGenerationConfig(&ClaudeRequest{Model: "gemini-4-carbon-" + tier, Thinking: &ThinkingConfig{Type: "enabled", BudgetTokens: 32768}, OutputConfig: &ClaudeOutputConfig{Effort: "low"}})
		require.NotNil(t, cfg.ThinkingConfig)
		require.True(t, cfg.ThinkingConfig.IncludeThoughts)
		require.Empty(t, cfg.ThinkingConfig.ThinkingLevel)
		require.Zero(t, cfg.ThinkingConfig.ThinkingBudget)
	}
	// Other models retain the existing budget behavior.
	cfg := buildGenerationConfig(&ClaudeRequest{Model: "claude-sonnet-4-6", Thinking: &ThinkingConfig{Type: "enabled", BudgetTokens: 4096}})
	require.Equal(t, 4096, cfg.ThinkingConfig.ThinkingBudget)
	require.Empty(t, cfg.ThinkingConfig.ThinkingLevel)
}

func TestCarbonTransformUsesMappedModelForThinking(t *testing.T) {
	req := &ClaudeRequest{Model: "client-alias", MaxTokens: 4096, Messages: []ClaudeMessage{{Role: "user", Content: json.RawMessage(`"hello"`)}}, Thinking: &ThinkingConfig{Type: "enabled", BudgetTokens: 4096}}
	data, err := TransformClaudeToGeminiWithOptions(req, "project", "gemini-4-carbon", TransformOptions{})
	require.NoError(t, err)
	require.Contains(t, string(data), `"thinkingLevel":"medium"`)
	require.NotContains(t, string(data), `thinkingBudget`)
	require.NotContains(t, string(data), `stopSequences`)
}
