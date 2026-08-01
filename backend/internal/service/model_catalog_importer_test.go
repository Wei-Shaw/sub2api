package service

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestPricingServiceBuildCatalogRevisionStageIsDeterministicAndExplicit(t *testing.T) {
	pricing := &PricingService{pricingData: map[string]*LiteLLMModelPricing{
		"gpt-5.6": {
			InputCostPerToken:  5e-6,
			OutputCostPerToken: 3e-5,
			LiteLLMProvider:    "openai",
			Mode:               "chat",
		},
		"image-only": {
			OutputCostPerImage: 1.25,
			TokenPricingAbsent: true,
			LiteLLMProvider:    "openai",
			Mode:               "image_generation",
		},
	}}

	first, err := pricing.BuildCatalogRevisionStage(CatalogScopeGlobal, 7)
	require.NoError(t, err)
	second, err := pricing.BuildCatalogRevisionStage(CatalogScopeGlobal, 7)
	require.NoError(t, err)
	require.Equal(t, first.NormalizedHash, second.NormalizedHash)
	require.Equal(t, first.Models, second.Models)
	require.Len(t, first.Models, 2)
	require.Equal(t, "gpt-5.6", first.Models[0].CanonicalKey)
	require.Equal(t, "gpt-5.6", first.Models[0].Aliases[0].Alias)
	require.True(t, first.Models[0].PricingValid)
	require.False(t, first.Models[1].PricingValid)
	require.Equal(t, CatalogNormalizerVersion, first.Normalizer)

	var metadata map[string]string
	require.NoError(t, json.Unmarshal(first.Models[0].SourceMetadata, &metadata))
	require.Equal(t, "legacy-pricing", metadata["source_set"])
}

func TestPricingServiceBuildCatalogRevisionStageRejectsEmptyLegacyCatalog(t *testing.T) {
	pricing := &PricingService{pricingData: map[string]*LiteLLMModelPricing{}}
	_, err := pricing.BuildCatalogRevisionStage(CatalogScopeGlobal, 1)
	require.Error(t, err)
}
