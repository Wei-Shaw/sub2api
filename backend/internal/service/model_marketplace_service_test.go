package service

import (
	"context"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

type modelMarketplaceAccountRepoStub struct {
	AccountRepository
	byGroup map[int64][]Account
}

func (s *modelMarketplaceAccountRepoStub) ListSchedulableByGroupID(ctx context.Context, groupID int64) ([]Account, error) {
	accounts := s.byGroup[groupID]
	out := make([]Account, len(accounts))
	copy(out, accounts)
	return out, nil
}

func TestModelMarketplaceListPricingIncludesCatalogAndFallbackModels(t *testing.T) {
	pricingSvc := &PricingService{
		pricingData: map[string]*LiteLLMModelPricing{
			"deepseek-v4-pro": {
				InputCostPerToken:       4.35e-7,
				OutputCostPerToken:      8.7e-7,
				CacheReadInputTokenCost: 3.625e-9,
				LiteLLMProvider:         "deepseek",
				Mode:                    "chat",
				MaxInputTokens:          64000,
			},
		},
		lastUpdated: time.Date(2026, 6, 25, 12, 0, 0, 0, time.UTC),
		localHash:   "hash-123",
	}
	billingSvc := NewBillingService(&config.Config{}, pricingSvc)
	svc := NewModelMarketplaceService(pricingSvc, billingSvc, nil, nil)

	res, err := svc.ListPricing(context.Background(), 0, ModelMarketplaceListRequest{Query: "glm", Page: 1, PageSize: 20})
	require.NoError(t, err)
	require.Greater(t, len(res.Items), 0)
	require.Equal(t, "hash-123", res.Catalog.LocalHash)
	require.Equal(t, 1, res.Catalog.ModelCount)

	var glm *ModelMarketplaceItem
	for i := range res.Items {
		if res.Items[i].Model == "glm-5.1" {
			glm = &res.Items[i]
			break
		}
	}
	require.NotNil(t, glm, "fallback-only GLM model should be listed")
	require.Equal(t, "zhipu", glm.Provider)
	require.Equal(t, "智谱 / GLM", glm.ProviderLabel)
	require.Equal(t, "sub2api_fallback", glm.Pricing.Source)
	require.Contains(t, glm.SupportedEndpoints, "openai")
	require.True(t, strings.Contains(glm.CalculationNote, "ActualCost = TotalCost * rate_multiplier"))
}

func TestModelMarketplaceListPricingFiltersAndCalculatesDisplayPrice(t *testing.T) {
	pricingSvc := &PricingService{
		pricingData: map[string]*LiteLLMModelPricing{
			"deepseek-v4-pro": {
				InputCostPerToken:       4.35e-7,
				OutputCostPerToken:      8.7e-7,
				CacheReadInputTokenCost: 3.625e-9,
				LiteLLMProvider:         "deepseek",
				Mode:                    "chat",
			},
			"gpt-5.4": {
				InputCostPerToken:               2.5e-6,
				InputCostPerTokenPriority:       5e-6,
				OutputCostPerToken:              15e-6,
				OutputCostPerTokenPriority:      30e-6,
				CacheReadInputTokenCost:         0.25e-6,
				LiteLLMProvider:                 "openai",
				Mode:                            "chat",
				SupportsServiceTier:             true,
				SupportsPromptCaching:           true,
				LongContextInputTokenThreshold:  272000,
				LongContextInputCostMultiplier:  2,
				LongContextOutputCostMultiplier: 1.5,
			},
		},
	}
	billingSvc := NewBillingService(&config.Config{}, pricingSvc)
	svc := NewModelMarketplaceService(pricingSvc, billingSvc, nil, nil)

	res, err := svc.ListPricing(context.Background(), 0, ModelMarketplaceListRequest{
		Provider:    "openai",
		BillingMode: "token",
		ServiceTier: "priority",
		Unit:        "1M",
		Page:        1,
		PageSize:    10,
	})
	require.NoError(t, err)
	require.NotEmpty(t, res.Items)
	require.Equal(t, "openai", res.Items[0].Provider)

	var gpt54 *ModelMarketplaceItem
	for i := range res.Items {
		if res.Items[i].Model == "gpt-5.4" {
			gpt54 = &res.Items[i]
			break
		}
	}
	require.NotNil(t, gpt54)
	require.Equal(t, "priority", gpt54.Pricing.ServiceTier)
	require.InDelta(t, 5.0, gpt54.Pricing.Input.DisplayUSD, 1e-9)
	require.InDelta(t, 30.0, gpt54.Pricing.Output.DisplayUSD, 1e-9)
	require.NotNil(t, gpt54.Pricing.LongContext)
	require.Equal(t, 272000, gpt54.Pricing.LongContext.InputTokenThreshold)
}

func TestModelMarketplaceBuildItemsScopesSelectedGroupToGatewayModelsList(t *testing.T) {
	groupID := int64(12)
	pricingSvc := &PricingService{pricingData: map[string]*LiteLLMModelPricing{
		"gpt-5.5":             {InputCostPerToken: 1e-6, LiteLLMProvider: "openai", Mode: "chat"},
		"codex-private-model": {InputCostPerToken: 2e-6, LiteLLMProvider: "openai", Mode: "chat"},
		"deepseek-v4-pro":     {InputCostPerToken: 3e-6, LiteLLMProvider: "deepseek", Mode: "chat"},
	}}
	billingSvc := NewBillingService(&config.Config{}, pricingSvc)
	gatewaySvc := &GatewayService{accountRepo: &modelMarketplaceAccountRepoStub{byGroup: map[int64][]Account{
		groupID: {{
			ID:       1,
			Platform: PlatformOpenAI,
			Credentials: map[string]any{
				"model_mapping": map[string]any{
					"codex-private-model": "codex-private-model",
				},
			},
		}},
	}}}
	svc := NewModelMarketplaceService(pricingSvc, billingSvc, nil, gatewaySvc)

	items := svc.buildItems(context.Background(), modelMarketplaceGroupContext{
		selectedID:   &groupID,
		selectedName: "CodexPLUS",
		selectedGroup: &Group{
			ID:             groupID,
			Name:           "CodexPLUS",
			Platform:       PlatformOpenAI,
			RateMultiplier: 0.7,
		},
		rate: 0.7,
	}, "standard", "1M")

	require.Equal(t, []string{"codex-private-model"}, modelMarketplaceItemModels(items))
	require.Equal(t, int64(12), *items[0].Pricing.GroupID)
	require.Equal(t, "CodexPLUS", items[0].Pricing.GroupName)
	require.InDelta(t, 1.4, items[0].Pricing.Input.DisplayUSD, 1e-9)
}

func TestModelMarketplaceBuildItemsUsesCustomModelsListForVisibility(t *testing.T) {
	groupID := int64(13)
	pricingSvc := &PricingService{pricingData: map[string]*LiteLLMModelPricing{
		"gpt-5.5":         {InputCostPerToken: 1e-6, LiteLLMProvider: "openai", Mode: "chat"},
		"gpt-5.4":         {InputCostPerToken: 2e-6, LiteLLMProvider: "openai", Mode: "chat"},
		"deepseek-v4-pro": {InputCostPerToken: 3e-6, LiteLLMProvider: "deepseek", Mode: "chat"},
	}}
	billingSvc := NewBillingService(&config.Config{}, pricingSvc)
	svc := NewModelMarketplaceService(pricingSvc, billingSvc, nil, nil)

	items := svc.buildItems(context.Background(), modelMarketplaceGroupContext{
		selectedID:   &groupID,
		selectedName: "CodexPLUS",
		selectedGroup: &Group{
			ID:             groupID,
			Name:           "CodexPLUS",
			Platform:       PlatformOpenAI,
			RateMultiplier: 0.7,
			ModelsListConfig: GroupModelsListConfig{
				Enabled: true,
				Models:  []string{"gpt-5.5"},
			},
		},
		rate: 0.7,
	}, "standard", "1M")

	require.Equal(t, []string{"gpt-5.5"}, modelMarketplaceItemModels(items))
}

func modelMarketplaceItemModels(items []ModelMarketplaceItem) []string {
	models := make([]string, 0, len(items))
	for _, item := range items {
		models = append(models, item.Model)
	}
	sort.Strings(models)
	return models
}
