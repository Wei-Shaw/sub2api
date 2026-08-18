package modelcatalog

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDefaultCatalogLoads(t *testing.T) {
	cat := Default()
	require.NotNil(t, cat)
	require.NotNil(t, cat.Lookup("gemini-3.6-flash"))
	require.NotNil(t, cat.Lookup("gemini-3.7-flash-high"))
	require.NotNil(t, cat.Lookup("claude-sonnet-4-6"))
	require.NotNil(t, cat.Lookup("gpt-5.6"))
	require.NotNil(t, cat.Lookup("grok-4.6"))
}

func TestGeminiFlashTiersShareBasePriceCard(t *testing.T) {
	for _, model := range []string{
		"gemini-3.6-flash-high",
		"gemini-3.6-flash-low",
		"gemini-3.6-flash-medium",
		"gemini-3.6-flash-tiered",
	} {
		require.Equal(t, "gemini-3.6-flash", PriceCardID(model), model)
		require.True(t, Locked(model), model)
	}
	for _, model := range []string{
		"gemini-3.7-flash",
		"gemini-3.7-flash-high",
		"gemini-3.7-flash-low",
		"gemini-3.7-flash-medium",
		"gemini-3.7-flash-tiered",
		"models/gemini-3.7-flash-high",
	} {
		require.Equal(t, "gemini-3.7-flash", PriceCardID(model), model)
		require.True(t, Locked(model), model)
	}

	flash36 := Lookup("gemini-3.6-flash").Rates()
	require.InDelta(t, 1.5e-6, flash36.Input, 1e-12)
	require.InDelta(t, 7.5e-6, flash36.Output, 1e-12)
	require.InDelta(t, 0.15e-6, flash36.CacheRead, 1e-12)

	flash37 := Lookup("gemini-3.7-flash-tiered").Rates()
	require.InDelta(t, 0.75e-6, flash37.Input, 1e-12)
	require.InDelta(t, 3.75e-6, flash37.Output, 1e-12)
	require.InDelta(t, 0.075e-6, flash37.CacheRead, 1e-12)
}

func TestCatalogRejectsDuplicateAndMissingPriceRef(t *testing.T) {
	_, err := Load([]byte(`{"version":1,"models":[{"id":"a","lock_price":true,"price":{"input_per_mtok":1}},{"id":"a"}]}`))
	require.Error(t, err)

	_, err = Load([]byte(`{"version":1,"models":[{"id":"a","price_ref":"missing"}]}`))
	require.Error(t, err)
}

func TestUnknownModelIsNotLocked(t *testing.T) {
	require.Nil(t, Lookup("definitely-not-a-model"))
	require.False(t, Locked("definitely-not-a-model"))
	require.Empty(t, PriceCardID("definitely-not-a-model"))
}

func TestSharedRateCardIDOnlyRemapsThinkingTiers(t *testing.T) {
	require.Equal(t, "gemini-3.6-flash", SharedRateCardID("gemini-3.6-flash-high"))
	require.Equal(t, "gemini-3.7-flash", SharedRateCardID("gemini-3.7-flash-tiered"))
	require.Empty(t, SharedRateCardID("gemini-3.6-flash"))
	require.Empty(t, SharedRateCardID("gpt-5.5"))
	require.Empty(t, SharedRateCardID("gpt-5.5-pro"))
	require.Empty(t, SharedRateCardID("codex-auto-review"))
	require.Empty(t, SharedRateCardID("deepseek-chat"))
	require.Empty(t, SharedRateCardID("claude-sonnet-4-6"))
}

func TestCatalogDoesNotAliasDistinctSoldModels(t *testing.T) {
	for _, model := range []string{"gpt-5.5", "gpt-5.5-pro", "codex-auto-review", "deepseek-chat", "deepseek-reasoner"} {
		require.Nil(t, Lookup(model), model)
		require.False(t, Locked(model), model)
		require.Empty(t, PriceCardID(model), model)
	}
}
