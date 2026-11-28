package service

import (
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/pkg/antigravity"
	"github.com/stretchr/testify/require"
)

func levelFamilyTestAccount(mapping map[string]any) *Account {
	return &Account{
		ID:          77,
		Platform:    PlatformAntigravity,
		Type:        AccountTypeOAuth,
		Credentials: map[string]any{"model_mapping": mapping},
	}
}

func TestAntigravityLevelFamilyPicksTierFromEffort(t *testing.T) {
	account := levelFamilyTestAccount(map[string]any{
		"gemini-4-carbon":   "gemini-4-carbon-high",
		"gemini-4-hydrogen": "gemini-4-hydrogen-high",
		"claude-opus-5-5":   "claude-opus-5-5-high",
	})
	svc := &AntigravityGatewayService{}
	cases := []struct {
		model, level, want string
	}{
		{"gemini-4-carbon", "low", "gemini-4-carbon-low"},
		{"gemini-4-carbon", "medium", "gemini-4-carbon-medium"},
		{"gemini-4-carbon", "", "gemini-4-carbon-high"},
		{"gemini-4-carbon", "xhigh", "gemini-4-carbon-high"},
		{"gemini-4-hydrogen", "xhigh", "gemini-4-hydrogen-xhigh"},
		{"gemini-4-hydrogen", "low", "gemini-4-hydrogen-low"},
		{"claude-opus-5-5", "medium", "claude-opus-5-5-medium"},
		{"claude-opus-5-5", "xhigh", "claude-opus-5-5-high"},
	}
	for _, tc := range cases {
		require.Equal(t, tc.want, svc.getMappedModelForThinkingLevel(account, tc.model, "", tc.level), "%s at %q", tc.model, tc.level)
	}
}

func TestAntigravityLevelFamilyRejectsTieredNamesAndHonoursAliases(t *testing.T) {
	account := levelFamilyTestAccount(map[string]any{
		"gemini-4-carbon": "gemini-4-carbon-high",
		"gemini-4-argon":  "gemini-4-carbon-low",
	})
	svc := &AntigravityGatewayService{}
	require.Empty(t, svc.getMappedModelForThinkingLevel(account, "gemini-4-carbon-high", "", "high"), "a tiered name the account does not list is rejected")
	require.Empty(t, svc.getMappedModelForThinkingLevel(account, "gemini-4-neon", "", "high"), "a family the account does not list stays unsupported")
	require.Equal(t, "gemini-4-carbon-low", svc.getMappedModelForThinkingLevel(account, "gemini-4-argon", "", "high"), "an alias to another model is kept")
}

// Accounts on the default mapping keep accepting tiered IDs: a tiered ID keeps
// its own level whatever effort the request states, a bare ID follows the effort.
func TestAntigravityLevelFamilyOnDefaultMapping(t *testing.T) {
	account := &Account{ID: 1, Platform: PlatformAntigravity, Type: AccountTypeOAuth, Credentials: map[string]any{}}
	svc := &AntigravityGatewayService{}
	require.Equal(t, "gemini-4-carbon-low", svc.getMappedModelForThinkingLevel(account, "gemini-4-carbon", "", "low"))
	require.Equal(t, "gemini-4-carbon-medium", svc.getMappedModelForThinkingLevel(account, "gemini-4-carbon-medium", "", "low"))
	require.Equal(t, "gemini-4-neon-xhigh", svc.getMappedModelForThinkingLevel(account, "gemini-4-neon", "", "xhigh"))
	require.Equal(t, "claude-opus-5-5-high", svc.getMappedModelForThinkingLevel(account, "claude-opus-5-5", "", ""))
}

func TestAntigravityFamilyLevelFromRequestBodies(t *testing.T) {
	require.Equal(t, "low", antigravityFamilyLevelFromClaudeBody([]byte(`{"output_config":{"effort":"low"}}`), &antigravity.ThinkingConfig{Type: "adaptive"}))
	require.Equal(t, "xhigh", antigravityFamilyLevelFromClaudeBody([]byte(`{"output_config":{"effort":"max"}}`), nil))
	require.Equal(t, "medium", antigravityFamilyLevelFromClaudeBody([]byte(`{}`), &antigravity.ThinkingConfig{Type: "enabled", BudgetTokens: 4096}))
	require.Equal(t, "low", antigravityFamilyLevelFromClaudeBody([]byte(`{}`), &antigravity.ThinkingConfig{Type: "disabled"}))
	require.Equal(t, "", antigravityFamilyLevelFromClaudeBody([]byte(`{}`), &antigravity.ThinkingConfig{Type: "adaptive"}))
	require.Equal(t, "xhigh", antigravityFamilyLevelFromGeminiBody([]byte(`{"generationConfig":{"thinkingConfig":{"thinkingLevel":"xhigh"}}}`)))
	require.Equal(t, "low", antigravityFamilyLevelFromGeminiBody([]byte(`{"generationConfig":{"thinkingConfig":{"thinkingBudget":0}}}`)))
	require.Equal(t, "", antigravityFamilyLevelFromGeminiBody([]byte(`{"generationConfig":{"thinkingConfig":{"thinkingBudget":-1}}}`)))
	require.Equal(t, "", antigravityFamilyLevelFromGeminiBody([]byte(`{}`)))
}
