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
		"glm-5.2": {
			InputCostPerToken:    1.4e-6,
			OutputCostPerToken:   4.4e-6,
			LiteLLMProvider:      "zhipu",
			Mode:                 "chat",
			CatalogPricingSource: "official:z.ai",
			CatalogSourceKind:    "official_vendor",
			CatalogSourceURL:     "https://docs.z.ai/guides/overview/pricing",
			CatalogSourceVersion: "2026-07-20",
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
	require.Len(t, first.Models, 3)
	require.Equal(t, "glm-5.2", first.Models[0].CanonicalKey)
	require.Equal(t, "glm-5.2", first.Models[0].Aliases[0].Alias)
	require.True(t, first.Models[0].PricingValid)
	require.Equal(t, "official:z.ai", first.Models[0].PricingSource)
	require.Equal(t, LegacyPricingCatalogSourceSet, first.Models[1].PricingSource)
	require.False(t, first.Models[2].PricingValid)
	require.Equal(t, CatalogNormalizerVersion, first.Normalizer)
	require.Equal(t, MixedPricingCatalogSourceSet, first.SyncRun.SourceSet)

	var metadata map[string]string
	require.NoError(t, json.Unmarshal(first.Models[0].SourceMetadata, &metadata))
	require.Equal(t, OfficialVendorPricingCatalogSourceSet, metadata["source_set"])
	require.Equal(t, "official_vendor", metadata["source_kind"])
	require.Equal(t, "https://docs.z.ai/guides/overview/pricing", metadata["source_url"])
	require.Equal(t, "2026-07-20", metadata["source_version"])

	metadata = nil
	require.NoError(t, json.Unmarshal(first.Models[1].SourceMetadata, &metadata))
	require.Equal(t, LegacyPricingCatalogSourceSet, metadata["source_set"])
}

func TestPricingServiceBuildCatalogRevisionStageRejectsEmptyLegacyCatalog(t *testing.T) {
	pricing := &PricingService{pricingData: map[string]*LiteLLMModelPricing{}}
	_, err := pricing.BuildCatalogRevisionStage(CatalogScopeGlobal, 1)
	require.Error(t, err)
}

func TestPricingServiceBuildCatalogRevisionStageRejectsIncompleteOfficialSource(t *testing.T) {
	pricing := &PricingService{pricingData: map[string]*LiteLLMModelPricing{
		"glm-5.2": {
			InputCostPerToken:    1.4e-6,
			OutputCostPerToken:   4.4e-6,
			CatalogPricingSource: "official:z.ai",
			CatalogSourceKind:    "official_vendor",
		},
	}}

	_, err := pricing.BuildCatalogRevisionStage(CatalogScopeGlobal, 1)
	require.ErrorContains(t, err, "official source URL")
}
