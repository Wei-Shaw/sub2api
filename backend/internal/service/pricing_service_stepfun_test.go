package service

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

type stepFunCatalogPrice struct {
	input     float64
	output    float64
	cacheRead float64
}

var expectedStepFunCatalogPrices = map[string]stepFunCatalogPrice{
	"step-5-preview":      {input: 0.98e-6, output: 2.80e-6, cacheRead: 0.10e-6},
	"step-3.7-flash":      {input: 0.19e-6, output: 1.13e-6, cacheRead: 0.04e-6},
	"step-3.5-flash":      {input: 0.10e-6, output: 0.30e-6, cacheRead: 0.02e-6},
	"step-3.5-flash-2603": {input: 0.10e-6, output: 0.30e-6, cacheRead: 0.02e-6},
}

func TestPricingService_BuiltinStepFunCatalogFillsMissingModels(t *testing.T) {
	svc := &PricingService{cfg: &config.Config{}}
	data, _, err := svc.buildPricingData([]byte(`{
		"other-model": {"litellm_provider": "test", "mode": "chat", "input_cost_per_token": 1e-06}
	}`))
	require.NoError(t, err)

	for model, want := range expectedStepFunCatalogPrices {
		pricing := data[model]
		require.NotNil(t, pricing, model)
		require.Equal(t, "stepfun", pricing.LiteLLMProvider, model)
		require.Equal(t, "chat", pricing.Mode, model)
		require.True(t, pricing.SupportsPromptCaching, model)
		require.InDelta(t, want.input, pricing.InputCostPerToken, 1e-12, model)
		require.InDelta(t, want.output, pricing.OutputCostPerToken, 1e-12, model)
		require.InDelta(t, want.cacheRead, pricing.CacheReadInputTokenCost, 1e-12, model)
	}

	svc.pricingData = data
	require.Equal(t, []string{
		"step-3.5-flash",
		"step-3.5-flash-2603",
		"step-3.7-flash",
		"step-5-preview",
	}, svc.ListModelNamesByProvider("stepfun"))
}

func TestPricingService_BuiltinStepFunCatalogDefersToRemoteFallbackAndOverride(t *testing.T) {
	dir := t.TempDir()
	fallbackPath := filepath.Join(dir, "fallback.json")
	overridePath := filepath.Join(dir, "override.json")
	require.NoError(t, os.WriteFile(fallbackPath, []byte(`{
		"step-5-preview": {
			"litellm_provider": "stepfun", "mode": "chat",
			"input_cost_per_token": 4.44e-06,
			"output_cost_per_token": 5.55e-06,
			"cache_read_input_token_cost": 0.66e-06
		}
	}`), 0644))
	require.NoError(t, os.WriteFile(overridePath, []byte(`{
		"step-3.5-flash": {"input_cost_per_token": 7.77e-06}
	}`), 0644))

	svc := &PricingService{cfg: &config.Config{Pricing: config.PricingConfig{
		FallbackFile: fallbackPath,
		OverrideFile: overridePath,
	}}}
	data, _, err := svc.buildPricingData([]byte(`{
		"step-3.7-flash": {
			"litellm_provider": "stepfun", "mode": "chat",
			"input_cost_per_token": 1.11e-06,
			"output_cost_per_token": 2.22e-06,
			"cache_read_input_token_cost": 0.33e-06
		},
		"remote-model": {"litellm_provider": "test", "mode": "chat", "input_cost_per_token": 1e-06}
	}`))
	require.NoError(t, err)

	remote := data["step-3.7-flash"]
	require.NotNil(t, remote)
	require.InDelta(t, 1.11e-6, remote.InputCostPerToken, 1e-12, "远程目录同名模型优先")
	require.InDelta(t, 2.22e-6, remote.OutputCostPerToken, 1e-12)

	fallback := data["step-5-preview"]
	require.NotNil(t, fallback)
	require.InDelta(t, 4.44e-6, fallback.InputCostPerToken, 1e-12, "custom fallback 同名模型优先")
	require.InDelta(t, 5.55e-6, fallback.OutputCostPerToken, 1e-12)

	overridden := data["step-3.5-flash"]
	require.NotNil(t, overridden)
	require.InDelta(t, 7.77e-6, overridden.InputCostPerToken, 1e-12, "override 可修补仅由内置目录提供的模型")
	require.InDelta(t, 0.30e-6, overridden.OutputCostPerToken, 1e-12, "未覆盖字段保留内置目录价格")

	sibling := data["step-3.5-flash-2603"]
	require.NotNil(t, sibling)
	require.InDelta(t, 0.10e-6, sibling.InputCostPerToken, 1e-12, "override 不应影响其他模型")
}

func TestPricingService_BuiltinStepFunPricesMatchBillingFallback(t *testing.T) {
	catalog, _, err := (&PricingService{cfg: &config.Config{}}).buildPricingData([]byte(`{
		"other-model": {"litellm_provider": "test", "mode": "chat", "input_cost_per_token": 1e-06}
	}`))
	require.NoError(t, err)

	billing := NewBillingService(&config.Config{}, nil)
	for model, want := range expectedStepFunCatalogPrices {
		builtin := catalog[model]
		require.NotNil(t, builtin, model)

		fallback, err := billing.GetModelPricing(model)
		require.NoError(t, err, model)
		require.InDelta(t, want.input, fallback.InputPricePerToken, 1e-12, model)
		require.InDelta(t, want.output, fallback.OutputPricePerToken, 1e-12, model)
		require.InDelta(t, want.cacheRead, fallback.CacheReadPricePerToken, 1e-12, model)

		require.InDelta(t, builtin.InputCostPerToken, fallback.InputPricePerToken, 1e-12, model)
		require.InDelta(t, builtin.OutputCostPerToken, fallback.OutputPricePerToken, 1e-12, model)
		require.InDelta(t, builtin.CacheReadInputTokenCost, fallback.CacheReadPricePerToken, 1e-12, model)
	}
}
