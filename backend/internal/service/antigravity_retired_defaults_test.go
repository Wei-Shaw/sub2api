package service

import (
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/domain"
	"github.com/stretchr/testify/require"
)

func TestAntigravityDefaultBareModelsStillResolveAfterCatalogRetirement(t *testing.T) {
	svc := &AntigravityGatewayService{}
	for _, model := range []string{"gemini-3.6-flash", "gemini-3.7-flash", "gemini-3.8-flash", "gemini-4-argon"} {
		account := &Account{Platform: PlatformAntigravity}
		require.Equal(t, model+"-high", svc.getMappedModel(account, model))
		if model != "gemini-4-argon" {
			require.Equal(t, model+"-low", svc.getMappedModelForThinkingLevel(account, model, "low"))
		}
	}
	// Explicitly configured mappings are administrator-owned, not rewritten
	// by changes to the defaults. Catalog refresh remains a manual action.
	account := &Account{Platform: PlatformAntigravity, Credentials: map[string]any{"model_mapping": map[string]any{"claude-opus-4-8": "custom-opus"}}}
	require.Equal(t, "custom-opus", account.GetMappedModel("claude-opus-4-8"))
	for _, model := range []string{"claude-fable-5-1", "claude-fable-5", "gemini-2.5-flash-image", "gemini-3.6-flash-tiered", "gemini-3.7-flash-tiered"} {
		require.NotContains(t, domain.DefaultAntigravityModelMapping, model)
		require.NotContains(t, antigravityDefaultPassthroughModels, model)
	}
}
