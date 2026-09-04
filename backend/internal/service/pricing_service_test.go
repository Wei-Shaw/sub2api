package service

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

func TestPricingSchedulerBlankRemoteURLDoesNotStart(t *testing.T) {
	svc := NewPricingService(&config.Config{Pricing: config.PricingConfig{RemoteURL: "  \t  "}}, nil)
	defer svc.Stop()

	svc.startUpdateScheduler()
	done := make(chan struct{})
	go func() {
		svc.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(100 * time.Millisecond):
		t.Fatal("blank remote URL must not start scheduler")
	}
}

func TestPricingNonEmptyInvalidRemoteURLStillReturnsValidationError(t *testing.T) {
	svc := NewPricingService(&config.Config{Pricing: config.PricingConfig{
		RemoteURL: "://invalid",
		DataDir:   t.TempDir(),
	}}, nil)

	err := svc.ForceUpdate()

	require.Error(t, err)
	require.Contains(t, err.Error(), "invalid pricing url")
}

func TestParsePricingData_ParsesPriorityAndServiceTierFields(t *testing.T) {
	svc := &PricingService{}
	body := []byte(`{
		"gpt-5.4": {
			"input_cost_per_token": 0.0000025,
			"input_cost_per_token_priority": 0.000005,
			"output_cost_per_token": 0.000015,
			"output_cost_per_token_priority": 0.00003,
			"cache_creation_input_token_cost": 0.0000025,
			"cache_creation_input_token_cost_priority": 0.000005,
			"cache_read_input_token_cost": 0.00000025,
			"cache_read_input_token_cost_priority": 0.0000005,
			"long_context_input_token_threshold": 272000,
			"long_context_input_cost_multiplier": 2,
			"long_context_output_cost_multiplier": 1.5,
			"supports_service_tier": true,
			"supports_prompt_caching": true,
			"litellm_provider": "openai",
			"mode": "chat"
		}
	}`)

	data, err := svc.parsePricingData(body)
	require.NoError(t, err)
	pricing := data["gpt-5.4"]
	require.NotNil(t, pricing)
	require.InDelta(t, 5e-6, pricing.InputCostPerTokenPriority, 1e-12)
	require.InDelta(t, 3e-5, pricing.OutputCostPerTokenPriority, 1e-12)
	require.InDelta(t, 5e-6, pricing.CacheCreationInputTokenCostPriority, 1e-12)
	require.InDelta(t, 5e-7, pricing.CacheReadInputTokenCostPriority, 1e-12)
	require.Equal(t, 272000, pricing.LongContextInputTokenThreshold)
	require.InDelta(t, 2.0, pricing.LongContextInputCostMultiplier, 1e-12)
	require.InDelta(t, 1.5, pricing.LongContextOutputCostMultiplier, 1e-12)
	require.True(t, pricing.SupportsServiceTier)
}

func TestBillingService_GPT56CacheWritePricingUsesOfficialMultiplier(t *testing.T) {
	tests := []struct {
		model             string
		input             float64
		inputPriority     float64
		output            float64
		outputPriority    float64
		cacheRead         float64
		cacheReadPriority float64
	}{
		{model: "gpt-5.6-sol", input: 5e-6, inputPriority: 10e-6, output: 30e-6, outputPriority: 60e-6, cacheRead: 0.5e-6, cacheReadPriority: 1e-6},
		{model: "gpt-5.6-terra", input: 2e-6, inputPriority: 4e-6, output: 12e-6, outputPriority: 24e-6, cacheRead: 0.2e-6, cacheReadPriority: 0.4e-6},
		{model: "gpt-5.6-luna", input: 0.2e-6, inputPriority: 0.4e-6, output: 1.2e-6, outputPriority: 2.4e-6, cacheRead: 0.02e-6, cacheReadPriority: 0.04e-6},
	}
	for _, tt := range tests {
		t.Run(tt.model, func(t *testing.T) {
			pricingSvc := &PricingService{pricingData: map[string]*LiteLLMModelPricing{
				tt.model: {
					InputCostPerToken:               tt.input,
					InputCostPerTokenPriority:       tt.inputPriority,
					OutputCostPerToken:              tt.output,
					OutputCostPerTokenPriority:      tt.outputPriority,
					CacheReadInputTokenCost:         tt.cacheRead,
					CacheReadInputTokenCostPriority: tt.cacheReadPriority,
				},
			}}
			svc := NewBillingService(&config.Config{}, pricingSvc)

			pricing, err := svc.GetModelPricing(tt.model)
			require.NoError(t, err)
			require.InDelta(t, tt.input*1.25, pricing.CacheCreationPricePerToken, 1e-12)
			require.InDelta(t, tt.inputPriority*1.25, pricing.CacheCreationPricePerTokenPriority, 1e-12)
			// 阶梯由目录数据驱动：条目无 above/long_context 字段时不再由策略强补。
			require.Zero(t, pricing.LongContextInputThreshold)

			tokens := UsageTokens{InputTokens: 700, OutputTokens: 50, CacheCreationTokens: 200, CacheReadTokens: 100}
			standard, err := svc.CalculateCostWithServiceTier(tt.model, tokens, 1, "")
			require.NoError(t, err)
			require.InDelta(t, 200*tt.input*1.25, standard.CacheCreationCost, 1e-12)

			priority, err := svc.CalculateCostWithServiceTier(tt.model, tokens, 1, "priority")
			require.NoError(t, err)
			require.InDelta(t, 200*tt.inputPriority*1.25, priority.CacheCreationCost, 1e-12)

			flex, err := svc.CalculateCostWithServiceTier(tt.model, tokens, 1, "flex")
			require.NoError(t, err)
			require.InDelta(t, 200*tt.input*1.25*0.5, flex.CacheCreationCost, 1e-12)
		})
	}
}

// gpt56LadderCatalogJSON 三个 5.6 模型的目录条目：above_272k 绝对价 + priority 平价，
// cache_write 缺失由策略按 1.25 倍输入价补齐。
const gpt56LadderCatalogJSON = `{
	"gpt-5.6-sol": {"litellm_provider": "openai", "mode": "chat",
		"input_cost_per_token": 5e-06, "input_cost_per_token_priority": 1e-05,
		"output_cost_per_token": 3e-05, "output_cost_per_token_priority": 6e-05,
		"cache_read_input_token_cost": 5e-07, "cache_read_input_token_cost_priority": 1e-06,
		"input_cost_per_token_above_272k_tokens": 1e-05,
		"output_cost_per_token_above_272k_tokens": 4.5e-05,
		"cache_read_input_token_cost_above_272k_tokens": 1e-06},
	"gpt-5.6-terra": {"litellm_provider": "openai", "mode": "chat",
		"input_cost_per_token": 2e-06, "input_cost_per_token_priority": 4e-06,
		"output_cost_per_token": 1.2e-05, "output_cost_per_token_priority": 2.4e-05,
		"cache_read_input_token_cost": 2e-07, "cache_read_input_token_cost_priority": 4e-07,
		"input_cost_per_token_above_272k_tokens": 4e-06,
		"output_cost_per_token_above_272k_tokens": 1.8e-05,
		"cache_read_input_token_cost_above_272k_tokens": 4e-07},
	"gpt-5.6-luna": {"litellm_provider": "openai", "mode": "chat",
		"input_cost_per_token": 2e-07, "input_cost_per_token_priority": 4e-07,
		"output_cost_per_token": 1.2e-06, "output_cost_per_token_priority": 2.4e-06,
		"cache_read_input_token_cost": 2e-08, "cache_read_input_token_cost_priority": 4e-08,
		"input_cost_per_token_above_272k_tokens": 4e-07,
		"output_cost_per_token_above_272k_tokens": 1.8e-06,
		"cache_read_input_token_cost_above_272k_tokens": 4e-08}
}`

func TestBillingService_GPT56UsesLongContextPricingAcrossModelsAndTiers(t *testing.T) {
	models := []struct {
		name               string
		input, cached      float64
		cacheWrite, output float64
	}{
		{name: "gpt-5.6-sol", input: 5e-6, cached: 0.5e-6, cacheWrite: 6.25e-6, output: 30e-6},
		{name: "gpt-5.6-terra", input: 2e-6, cached: 0.2e-6, cacheWrite: 2.5e-6, output: 12e-6},
		{name: "gpt-5.6-luna", input: 0.2e-6, cached: 0.02e-6, cacheWrite: 0.25e-6, output: 1.2e-6},
	}
	tiers := []struct {
		name       string
		priceScale float64
	}{
		{name: "standard", priceScale: 1},
		{name: "priority", priceScale: 2},
		{name: "flex", priceScale: 0.5},
	}
	tokens := UsageTokens{
		InputTokens:         100000,
		CacheCreationTokens: 100000,
		CacheReadTokens:     73000,
		OutputTokens:        10,
	}

	for _, model := range models {
		for _, tier := range tiers {
			t.Run(model.name+"/"+tier.name, func(t *testing.T) {
				svc := NewBillingService(&config.Config{}, newStubPricingServiceFromJSON(t, gpt56LadderCatalogJSON))
				serviceTier := ""
				if tier.name != "standard" {
					serviceTier = tier.name
				}
				cost, err := svc.CalculateCostWithServiceTier(model.name, tokens, 1, serviceTier)
				require.NoError(t, err)
				require.InDelta(t, float64(tokens.InputTokens)*model.input*tier.priceScale*2, cost.InputCost, 1e-12)
				require.InDelta(t, float64(tokens.CacheCreationTokens)*model.cacheWrite*tier.priceScale*2, cost.CacheCreationCost, 1e-12)
				require.InDelta(t, float64(tokens.CacheReadTokens)*model.cached*tier.priceScale*2, cost.CacheReadCost, 1e-12)
				require.InDelta(t, float64(tokens.OutputTokens)*model.output*tier.priceScale*1.5, cost.OutputCost, 1e-12)
			})
		}
	}
}

func TestBillingService_GPT56LongContextBoundaryIsExclusive(t *testing.T) {
	svc := NewBillingService(&config.Config{}, newStubPricingServiceFromJSON(t, gpt56LadderCatalogJSON))
	tokens := UsageTokens{InputTokens: 100000, CacheCreationTokens: 100000, CacheReadTokens: 72000, OutputTokens: 10}

	cost, err := svc.CalculateCost("gpt-5.6-sol", tokens, 1)
	require.NoError(t, err)
	require.InDelta(t, 100000*5e-6, cost.InputCost, 1e-12)
	require.InDelta(t, 100000*6.25e-6, cost.CacheCreationCost, 1e-12)
	require.InDelta(t, 72000*0.5e-6, cost.CacheReadCost, 1e-12)
	require.InDelta(t, 10*30e-6, cost.OutputCost, 1e-12)
}

func TestPricingService_BareGPT56AliasDeterministicallyUsesSol(t *testing.T) {
	pricingSvc := &PricingService{pricingData: map[string]*LiteLLMModelPricing{
		"gpt-5.6-sol":   {InputCostPerToken: 5e-6},
		"gpt-5.6-terra": {InputCostPerToken: 2e-6},
		"gpt-5.6-luna":  {InputCostPerToken: 0.2e-6},
		"gpt-5.4":       {InputCostPerToken: 2.5e-6},
	}}

	for i := 0; i < 100; i++ {
		for _, alias := range []string{"gpt-5.6", "openai/gpt-5.6"} {
			pricing := pricingSvc.GetModelPricing(alias)
			require.NotNil(t, pricing)
			require.InDelta(t, 5e-6, pricing.InputCostPerToken, 1e-12, "iteration=%d alias=%s", i, alias)
		}
	}

	billingSvc := NewBillingService(&config.Config{}, pricingSvc)
	for _, alias := range []string{"gpt-5.6", "openai/gpt-5.6"} {
		pricing, err := billingSvc.GetModelPricing(alias)
		require.NoError(t, err)
		require.InDelta(t, 5e-6, pricing.InputPricePerToken, 1e-12)
		require.InDelta(t, 6.25e-6, pricing.CacheCreationPricePerToken, 1e-12)
	}
}

func TestDefaultPricingIncludesOfficialGPT56Rates(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", "resources", "model-pricing", "model_prices_and_context_window.json"))
	require.NoError(t, err)

	pricingSvc := &PricingService{}
	pricingData, err := pricingSvc.parsePricingData(data)
	require.NoError(t, err)
	pricingSvc.pricingData = pricingData
	billingSvc := NewBillingService(&config.Config{}, pricingSvc)

	tests := []struct {
		model                                                             string
		input, cached, cacheWrite, output                                 float64
		inputPriority, cachedPriority, cacheWritePriority, outputPriority float64
	}{
		{model: "gpt-5.6-sol", input: 5e-6, cached: 0.5e-6, cacheWrite: 6.25e-6, output: 30e-6, inputPriority: 10e-6, cachedPriority: 1e-6, cacheWritePriority: 12.5e-6, outputPriority: 60e-6},
		{model: "gpt-5.6-terra", input: 2e-6, cached: 0.2e-6, cacheWrite: 2.5e-6, output: 12e-6, inputPriority: 4e-6, cachedPriority: 0.4e-6, cacheWritePriority: 5e-6, outputPriority: 24e-6},
		{model: "gpt-5.6-luna", input: 0.2e-6, cached: 0.02e-6, cacheWrite: 0.25e-6, output: 1.2e-6, inputPriority: 0.4e-6, cachedPriority: 0.04e-6, cacheWritePriority: 0.5e-6, outputPriority: 2.4e-6},
	}
	for _, tt := range tests {
		t.Run(tt.model, func(t *testing.T) {
			pricing, err := billingSvc.GetModelPricing(tt.model)
			require.NoError(t, err)
			require.InDelta(t, tt.input, pricing.InputPricePerToken, 1e-12)
			require.InDelta(t, tt.cached, pricing.CacheReadPricePerToken, 1e-12)
			require.InDelta(t, tt.cacheWrite, pricing.CacheCreationPricePerToken, 1e-12)
			require.InDelta(t, tt.output, pricing.OutputPricePerToken, 1e-12)
			require.InDelta(t, tt.inputPriority, pricing.InputPricePerTokenPriority, 1e-12)
			require.InDelta(t, tt.cachedPriority, pricing.CacheReadPricePerTokenPriority, 1e-12)
			require.InDelta(t, tt.cacheWritePriority, pricing.CacheCreationPricePerTokenPriority, 1e-12)
			require.InDelta(t, tt.outputPriority, pricing.OutputPricePerTokenPriority, 1e-12)
			require.Equal(t, 272000, pricing.LongContextInputThreshold)
			require.InDelta(t, 2.0, pricing.LongContextInputMultiplier, 1e-12)
			require.InDelta(t, 1.5, pricing.LongContextOutputMultiplier, 1e-12)
		})
	}
}

func TestGPT56DedicatedFallbacksUseOfficialRates(t *testing.T) {
	tests := []struct {
		model                             string
		input, cached, cacheWrite, output float64
	}{
		{model: "gpt-5.6-sol", input: 5e-6, cached: 0.5e-6, cacheWrite: 6.25e-6, output: 30e-6},
		{model: "gpt-5.6-terra", input: 2e-6, cached: 0.2e-6, cacheWrite: 2.5e-6, output: 12e-6},
		{model: "gpt-5.6-luna", input: 0.2e-6, cached: 0.02e-6, cacheWrite: 0.25e-6, output: 1.2e-6},
	}

	for _, tt := range tests {
		t.Run(tt.model+"/pricing_service", func(t *testing.T) {
			pricingSvc := &PricingService{pricingData: map[string]*LiteLLMModelPricing{
				"gpt-5.1-codex": {InputCostPerToken: 1.25e-6},
			}}
			svc := NewBillingService(&config.Config{}, pricingSvc)
			pricing, err := svc.GetModelPricing(tt.model + "-preview")
			require.NoError(t, err)
			assertGPT56FallbackPricing(t, pricing, tt.input, tt.cached, tt.cacheWrite, tt.output)
		})

		t.Run(tt.model+"/billing_service", func(t *testing.T) {
			svc := NewBillingService(&config.Config{}, nil)
			pricing, err := svc.GetModelPricing(tt.model)
			require.NoError(t, err)
			assertGPT56FallbackPricing(t, pricing, tt.input, tt.cached, tt.cacheWrite, tt.output)
		})
	}
}

func assertGPT56FallbackPricing(t *testing.T, pricing *ModelPricing, input, cached, cacheWrite, output float64) {
	t.Helper()
	require.InDelta(t, input, pricing.InputPricePerToken, 1e-12)
	require.InDelta(t, cached, pricing.CacheReadPricePerToken, 1e-12)
	require.InDelta(t, cacheWrite, pricing.CacheCreationPricePerToken, 1e-12)
	require.InDelta(t, output, pricing.OutputPricePerToken, 1e-12)
	// 静态兜底只兜基础价；阶梯由目录数据（above_272k 折算或显式字段）驱动。
	require.Zero(t, pricing.LongContextInputThreshold)
}

func TestParsePricingData_KeepsImageOnlyPricing(t *testing.T) {
	svc := &PricingService{}
	body := []byte(`{
		"image-only-model": {
			"output_cost_per_image": 0.034,
			"litellm_provider": "vertex_ai-language-models",
			"mode": "image_generation"
		}
	}`)

	data, err := svc.parsePricingData(body)
	require.NoError(t, err)
	pricing := data["image-only-model"]
	require.NotNil(t, pricing)
	require.InDelta(t, 0.034, pricing.OutputCostPerImage, 1e-12)
	require.Equal(t, "image_generation", pricing.Mode)
	// 仅有图片价的条目必须标记 token 价缺失，供 token 计费路径 fail-closed。
	require.True(t, pricing.TokenPricingAbsent)
}

func TestBillingService_GetModelPricing_FailsClosedForImageOnlyEntries(t *testing.T) {
	pricingSvc := &PricingService{}
	data, err := pricingSvc.parsePricingData([]byte(`{
		"imagen-9.0-generate": {
			"output_cost_per_image": 0.04,
			"litellm_provider": "vertex_ai-image-models",
			"mode": "image_generation"
		},
		"gemini-image-with-token-price": {
			"input_cost_per_token": 0.0,
			"output_cost_per_token": 0.0,
			"output_cost_per_image": 0.034,
			"litellm_provider": "vertex_ai-language-models",
			"mode": "image_generation"
		}
	}`))
	require.NoError(t, err)
	pricingSvc.pricingData = data
	billingSvc := NewBillingService(&config.Config{}, pricingSvc)

	// image-only 条目不得进入 token 计费（否则 token 流量按 $0 计费），
	// 必须落到 fallback / ErrModelPricingUnavailable 的 fail-closed 路径。
	_, err = billingSvc.GetModelPricing("imagen-9.0-generate")
	require.ErrorIs(t, err, ErrModelPricingUnavailable)

	// 显式 0 token 价的免费条目保持历史行为：正常返回。
	pricing, err := billingSvc.GetModelPricing("gemini-image-with-token-price")
	require.NoError(t, err)
	require.Zero(t, pricing.InputPricePerToken)

	// 图片计费路径不受影响：仍能读到 image-only 条目的图片单价。
	raw := pricingSvc.GetModelPricing("imagen-9.0-generate")
	require.NotNil(t, raw)
	require.InDelta(t, 0.04, raw.OutputCostPerImage, 1e-12)
}

func TestPricingService_MergesFallbackOnlyModels(t *testing.T) {
	dir := t.TempDir()
	fallbackFile := filepath.Join(dir, "fallback.json")
	require.NoError(t, os.WriteFile(fallbackFile, []byte(`{
		"remote-model": {
			"input_cost_per_token": 0.000001,
			"litellm_provider": "test",
			"mode": "chat"
		},
		"gemini-3.1-flash-lite-image": {
			"output_cost_per_image": 0.034,
			"litellm_provider": "vertex_ai-language-models",
			"mode": "image_generation"
		}
	}`), 0644))

	svc := &PricingService{cfg: &config.Config{}}
	svc.cfg.Pricing.FallbackFile = fallbackFile
	remoteData, err := svc.parsePricingData([]byte(`{
		"remote-model": {
			"input_cost_per_token": 0.000002,
			"litellm_provider": "test",
			"mode": "chat"
		}
	}`))
	require.NoError(t, err)

	merged := svc.mergeFallbackPricingData(remoteData)
	require.InDelta(t, 0.000002, merged["remote-model"].InputCostPerToken, 1e-12)
	require.NotNil(t, merged["gemini-3.1-flash-lite-image"])
	require.InDelta(t, 0.034, merged["gemini-3.1-flash-lite-image"].OutputCostPerImage, 1e-12)
}

func TestGetModelPricing_Gpt53CodexSparkUsesGpt51CodexPricing(t *testing.T) {
	sparkPricing := &LiteLLMModelPricing{InputCostPerToken: 1}
	gpt53Pricing := &LiteLLMModelPricing{InputCostPerToken: 9}

	svc := &PricingService{
		pricingData: map[string]*LiteLLMModelPricing{
			"gpt-5.1-codex": sparkPricing,
			"gpt-5.3":       gpt53Pricing,
		},
	}

	got := svc.GetModelPricing("gpt-5.3-codex-spark")
	require.Same(t, sparkPricing, got)
}

func TestGetModelPricing_Gpt53CodexFallbackStillUsesGpt52Codex(t *testing.T) {
	gpt52CodexPricing := &LiteLLMModelPricing{InputCostPerToken: 2}

	svc := &PricingService{
		pricingData: map[string]*LiteLLMModelPricing{
			"gpt-5.2-codex": gpt52CodexPricing,
		},
	}

	got := svc.GetModelPricing("gpt-5.3-codex")
	require.Same(t, gpt52CodexPricing, got)
}

func TestGetModelPricing_OpenAIFallbackMatchedLoggedAsInfo(t *testing.T) {
	logSink, restore := captureStructuredLog(t)
	defer restore()

	gpt52CodexPricing := &LiteLLMModelPricing{InputCostPerToken: 2}
	svc := &PricingService{
		pricingData: map[string]*LiteLLMModelPricing{
			"gpt-5.2-codex": gpt52CodexPricing,
		},
	}

	got := svc.GetModelPricing("gpt-5.3-codex")
	require.Same(t, gpt52CodexPricing, got)

	require.True(t, logSink.ContainsMessageAtLevel("[Pricing] OpenAI fallback matched gpt-5.3-codex -> gpt-5.2-codex", "info"))
	require.False(t, logSink.ContainsMessageAtLevel("[Pricing] OpenAI fallback matched gpt-5.3-codex -> gpt-5.2-codex", "warn"))
}

func TestGetModelPricing_Gpt54UsesStaticFallbackWhenRemoteMissing(t *testing.T) {
	svc := &PricingService{
		pricingData: map[string]*LiteLLMModelPricing{
			"gpt-5.1-codex": &LiteLLMModelPricing{InputCostPerToken: 1.25e-6},
		},
	}

	got := svc.GetModelPricing("gpt-5.4")
	require.NotNil(t, got)
	require.InDelta(t, 2.5e-6, got.InputCostPerToken, 1e-12)
	require.InDelta(t, 1.5e-5, got.OutputCostPerToken, 1e-12)
	require.InDelta(t, 2.5e-7, got.CacheReadInputTokenCost, 1e-12)
	// 静态兜底只兜基础价，不携带长上下文阶梯（阶梯由目录数据驱动）。
	require.Zero(t, got.LongContextInputTokenThreshold)
}

func TestGetModelPricing_OpenAICompactAliasUsesStaticFallback(t *testing.T) {
	svc := &PricingService{
		pricingData: map[string]*LiteLLMModelPricing{
			"gpt-5.1-codex": {InputCostPerToken: 1.25e-6},
		},
	}

	got := svc.GetModelPricing("openai/gpt5.5")
	require.NotNil(t, got)
	require.InDelta(t, 2.5e-6, got.InputCostPerToken, 1e-12)
	require.InDelta(t, 1.5e-5, got.OutputCostPerToken, 1e-12)
}

func TestPricingService_Gemini36FlashThinkingTiersUseBasePricing(t *testing.T) {
	basePricing := &LiteLLMModelPricing{
		InputCostPerToken:       1.5e-6,
		OutputCostPerToken:      7.5e-6,
		CacheReadInputTokenCost: 0.15e-6,
	}
	svc := &PricingService{pricingData: map[string]*LiteLLMModelPricing{
		"gemini-3.6-flash": basePricing,
	}}

	for _, model := range []string{
		"gemini-3.6-flash",
		"gemini-3.6-flash-high",
		"gemini-3.6-flash-low",
		"gemini-3.6-flash-medium",
		"gemini-3.6-flash-tiered",
	} {
		t.Run(model, func(t *testing.T) {
			require.Same(t, basePricing, svc.GetModelPricing(model))
		})
	}
}

func TestPricingService_Gemini36FlashTierSpecificPricingTakesPrecedence(t *testing.T) {
	basePricing := &LiteLLMModelPricing{InputCostPerToken: 1.5e-6}
	tierPricing := &LiteLLMModelPricing{InputCostPerToken: 2e-6}
	svc := &PricingService{pricingData: map[string]*LiteLLMModelPricing{
		"gemini-3.6-flash":     basePricing,
		"gemini-3.6-flash-low": tierPricing,
	}}

	require.Same(t, tierPricing, svc.GetModelPricing("models/gemini-3.6-flash-low"))
}

func TestBillingService_Gemini36FlashThinkingTierFallbacksAreBillable(t *testing.T) {
	svc := NewBillingService(&config.Config{}, nil)
	tokens := UsageTokens{InputTokens: 1_000_000, OutputTokens: 1_000_000, CacheReadTokens: 1_000_000}

	for _, model := range []string{
		"gemini-3.6-flash",
		"gemini-3.6-flash-high",
		"gemini-3.6-flash-low",
		"gemini-3.6-flash-medium",
		"gemini-3.6-flash-tiered",
	} {
		t.Run(model, func(t *testing.T) {
			cost, err := svc.CalculateCost(model, tokens, 1)
			require.NoError(t, err)
			require.InDelta(t, 1.5, cost.InputCost, 1e-12)
			require.InDelta(t, 7.5, cost.OutputCost, 1e-12)
			require.InDelta(t, 0.15, cost.CacheReadCost, 1e-12)
			require.InDelta(t, 9.15, cost.TotalCost, 1e-12)
		})
	}
}

func TestDefaultPricingIncludesGemini36FlashRates(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", "resources", "model-pricing", "model_prices_and_context_window.json"))
	require.NoError(t, err)

	pricingSvc := &PricingService{}
	pricingData, err := pricingSvc.parsePricingData(data)
	require.NoError(t, err)
	pricingSvc.pricingData = pricingData
	billingSvc := NewBillingService(&config.Config{}, pricingSvc)

	for _, model := range []string{"gemini-3.6-flash", "gemini-3.6-flash-low", "gemini-3.6-flash-high"} {
		t.Run(model, func(t *testing.T) {
			pricing, err := billingSvc.GetModelPricing(model)
			require.NoError(t, err)
			require.InDelta(t, 1.5e-6, pricing.InputPricePerToken, 1e-12)
			require.InDelta(t, 7.5e-6, pricing.OutputPricePerToken, 1e-12)
			require.InDelta(t, 0.15e-6, pricing.CacheReadPricePerToken, 1e-12)
		})
	}
}

func TestDefaultPricingUsesCurrentCodexAutoReviewBaseRates(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", "resources", "model-pricing", "model_prices_and_context_window.json"))
	require.NoError(t, err)

	svc := &PricingService{}
	pricingData, err := svc.parsePricingData(data)
	require.NoError(t, err)
	svc.pricingData = pricingData

	got := svc.GetModelPricing("codex-auto-review")
	require.NotNil(t, got)
	require.InDelta(t, 0.2e-6, got.InputCostPerToken, 1e-12)
	require.InDelta(t, 1.2e-6, got.OutputCostPerToken, 1e-12)
	require.InDelta(t, 0.02e-6, got.CacheReadInputTokenCost, 1e-12)

	// Auto-review is an internal Codex model. Do not infer public GPT-5.6 API
	// service-tier, cache-write, or long-context pricing without an upstream
	// usage contract for this dedicated model.
	require.Zero(t, got.InputCostPerTokenPriority)
	require.Zero(t, got.OutputCostPerTokenPriority)
	require.Zero(t, got.CacheReadInputTokenCostPriority)
	require.Zero(t, got.CacheCreationInputTokenCost)
	require.Zero(t, got.CacheCreationInputTokenCostPriority)
	require.Zero(t, got.LongContextInputTokenThreshold)
}

func TestGetModelPricing_Gpt54MiniUsesDedicatedStaticFallbackWhenRemoteMissing(t *testing.T) {
	svc := &PricingService{
		pricingData: map[string]*LiteLLMModelPricing{
			"gpt-5.1-codex": {InputCostPerToken: 1.25e-6},
		},
	}

	got := svc.GetModelPricing("gpt-5.4-mini")
	require.NotNil(t, got)
	require.InDelta(t, 7.5e-7, got.InputCostPerToken, 1e-12)
	require.InDelta(t, 4.5e-6, got.OutputCostPerToken, 1e-12)
	require.InDelta(t, 7.5e-8, got.CacheReadInputTokenCost, 1e-12)
	require.Zero(t, got.LongContextInputTokenThreshold)
}

func TestGetModelPricing_Gpt54NanoUsesDedicatedStaticFallbackWhenRemoteMissing(t *testing.T) {
	svc := &PricingService{
		pricingData: map[string]*LiteLLMModelPricing{
			"gpt-5.1-codex": {InputCostPerToken: 1.25e-6},
		},
	}

	got := svc.GetModelPricing("gpt-5.4-nano")
	require.NotNil(t, got)
	require.InDelta(t, 2e-7, got.InputCostPerToken, 1e-12)
	require.InDelta(t, 1.25e-6, got.OutputCostPerToken, 1e-12)
	require.InDelta(t, 2e-8, got.CacheReadInputTokenCost, 1e-12)
	require.Zero(t, got.LongContextInputTokenThreshold)
}

func TestGetModelPricing_ImageModelDoesNotFallbackToTextModel(t *testing.T) {
	imagePricing := &LiteLLMModelPricing{InputCostPerToken: 3}
	textPricing := &LiteLLMModelPricing{InputCostPerToken: 9}

	svc := &PricingService{
		pricingData: map[string]*LiteLLMModelPricing{
			"gpt-image-2": imagePricing,
			"gpt-5.4":     textPricing,
		},
	}

	got := svc.GetModelPricing("gpt-image-3")
	require.Same(t, imagePricing, got)
}

func TestParsePricingData_PreservesPriorityAndServiceTierFields(t *testing.T) {
	raw := map[string]any{
		"gpt-5.4": map[string]any{
			"input_cost_per_token":                 2.5e-6,
			"input_cost_per_token_priority":        5e-6,
			"output_cost_per_token":                15e-6,
			"output_cost_per_token_priority":       30e-6,
			"cache_read_input_token_cost":          0.25e-6,
			"cache_read_input_token_cost_priority": 0.5e-6,
			"supports_service_tier":                true,
			"supports_prompt_caching":              true,
			"litellm_provider":                     "openai",
			"mode":                                 "chat",
		},
	}
	body, err := json.Marshal(raw)
	require.NoError(t, err)

	svc := &PricingService{}
	pricingMap, err := svc.parsePricingData(body)
	require.NoError(t, err)

	pricing := pricingMap["gpt-5.4"]
	require.NotNil(t, pricing)
	require.InDelta(t, 2.5e-6, pricing.InputCostPerToken, 1e-12)
	require.InDelta(t, 5e-6, pricing.InputCostPerTokenPriority, 1e-12)
	require.InDelta(t, 15e-6, pricing.OutputCostPerToken, 1e-12)
	require.InDelta(t, 30e-6, pricing.OutputCostPerTokenPriority, 1e-12)
	require.InDelta(t, 0.25e-6, pricing.CacheReadInputTokenCost, 1e-12)
	require.InDelta(t, 0.5e-6, pricing.CacheReadInputTokenCostPriority, 1e-12)
	require.True(t, pricing.SupportsServiceTier)
}

func TestParsePricingData_PreservesServiceTierPriorityFields(t *testing.T) {
	svc := &PricingService{}
	pricingData, err := svc.parsePricingData([]byte(`{
		"gpt-5.4": {
			"input_cost_per_token": 0.0000025,
			"input_cost_per_token_priority": 0.000005,
			"output_cost_per_token": 0.000015,
			"output_cost_per_token_priority": 0.00003,
			"cache_read_input_token_cost": 0.00000025,
			"cache_read_input_token_cost_priority": 0.0000005,
			"supports_service_tier": true,
			"litellm_provider": "openai",
			"mode": "chat"
		}
	}`))
	require.NoError(t, err)

	pricing := pricingData["gpt-5.4"]
	require.NotNil(t, pricing)
	require.InDelta(t, 0.0000025, pricing.InputCostPerToken, 1e-12)
	require.InDelta(t, 0.000005, pricing.InputCostPerTokenPriority, 1e-12)
	require.InDelta(t, 0.000015, pricing.OutputCostPerToken, 1e-12)
	require.InDelta(t, 0.00003, pricing.OutputCostPerTokenPriority, 1e-12)
	require.InDelta(t, 0.00000025, pricing.CacheReadInputTokenCost, 1e-12)
	require.InDelta(t, 0.0000005, pricing.CacheReadInputTokenCostPriority, 1e-12)
	require.True(t, pricing.SupportsServiceTier)
}

// ---------------------------------------------------------------------------
// ListModelNamesByProvider
// ---------------------------------------------------------------------------

func TestListModelNamesByProvider_ReturnsMatchingModels(t *testing.T) {
	svc := &PricingService{
		pricingData: map[string]*LiteLLMModelPricing{
			"claude-opus-4-5-20251101": {LiteLLMProvider: "anthropic", InputCostPerToken: 1.5e-5},
			"claude-sonnet-4-5":        {LiteLLMProvider: "anthropic", InputCostPerToken: 3e-6},
			"gpt-4o":                   {LiteLLMProvider: "openai", InputCostPerToken: 5e-6},
			"gemini-2.5-pro":           {LiteLLMProvider: "google", InputCostPerToken: 1.25e-6},
		},
	}

	got := svc.ListModelNamesByProvider("anthropic")
	require.ElementsMatch(t, []string{"claude-opus-4-5-20251101", "claude-sonnet-4-5"}, got)
	// Must be sorted
	require.Equal(t, "claude-opus-4-5-20251101", got[0])
	require.Equal(t, "claude-sonnet-4-5", got[1])
}

func TestListModelNamesByProvider_CaseInsensitive(t *testing.T) {
	svc := &PricingService{
		pricingData: map[string]*LiteLLMModelPricing{
			"gpt-4o": {LiteLLMProvider: "OpenAI", InputCostPerToken: 5e-6},
		},
	}

	got := svc.ListModelNamesByProvider("openai")
	require.Equal(t, []string{"gpt-4o"}, got)

	got2 := svc.ListModelNamesByProvider("OPENAI")
	require.Equal(t, []string{"gpt-4o"}, got2)
}

func TestListModelNamesByProvider_NoMatch(t *testing.T) {
	svc := &PricingService{
		pricingData: map[string]*LiteLLMModelPricing{
			"gpt-4o": {LiteLLMProvider: "openai", InputCostPerToken: 5e-6},
		},
	}

	got := svc.ListModelNamesByProvider("anthropic")
	require.NotNil(t, got)
	require.Empty(t, got)
}

func TestListModelNamesByProvider_EmptyCatalog(t *testing.T) {
	svc := &PricingService{
		pricingData: map[string]*LiteLLMModelPricing{},
	}

	got := svc.ListModelNamesByProvider("openai")
	require.NotNil(t, got)
	require.Empty(t, got)
}

// --- above_XXXk 绝对价字段折算为阈值+倍率 ---

func TestParsePricingData_DerivesLongContextFromAboveTierFields(t *testing.T) {
	svc := &PricingService{}
	data, err := svc.parsePricingData([]byte(`{
		"gpt-above": {"litellm_provider": "openai", "mode": "chat",
			"input_cost_per_token": 5e-06, "output_cost_per_token": 3e-05,
			"cache_read_input_token_cost": 5e-07,
			"input_cost_per_token_above_272k_tokens": 1e-05,
			"output_cost_per_token_above_272k_tokens": 4.5e-05,
			"cache_read_input_token_cost_above_272k_tokens": 1e-06,
			"input_cost_per_token_above_272k_tokens_flex": 5e-06,
			"output_cost_per_token_above_272k_tokens_flex": 2.25e-05},
		"gemini-above": {"litellm_provider": "vertex_ai-language-models", "mode": "chat",
			"input_cost_per_token": 1.25e-06, "output_cost_per_token": 1e-05,
			"input_cost_per_token_above_200k_tokens": 2.5e-06,
			"output_cost_per_token_above_200k_tokens": 1.5e-05},
		"explicit-wins": {"litellm_provider": "openai", "mode": "chat",
			"input_cost_per_token": 5e-06, "output_cost_per_token": 3e-05,
			"long_context_input_cost_multiplier": 1,
			"long_context_output_cost_multiplier": 1,
			"input_cost_per_token_above_272k_tokens": 1e-05,
			"output_cost_per_token_above_272k_tokens": 4.5e-05},
		"no-surcharge": {"litellm_provider": "openai", "mode": "chat",
			"input_cost_per_token": 5e-06, "output_cost_per_token": 3e-05,
			"input_cost_per_token_above_272k_tokens": 5e-06,
			"output_cost_per_token_above_272k_tokens": 3e-05},
		"cache-only-above": {"litellm_provider": "openai", "mode": "chat",
			"input_cost_per_token": 5e-06, "output_cost_per_token": 3e-05,
			"cache_read_input_token_cost_above_272k_tokens": 1e-06},
		"multi-threshold": {"litellm_provider": "openai", "mode": "chat",
			"input_cost_per_token": 1e-06, "output_cost_per_token": 2e-06,
			"input_cost_per_token_above_128k_tokens": 2e-06,
			"input_cost_per_token_above_272k_tokens": 4e-06}
	}`))
	require.NoError(t, err)

	openai := data["gpt-above"]
	require.Equal(t, 272000, openai.LongContextInputTokenThreshold, "阈值取自字段名（_flex 变体不参与）")
	require.InDelta(t, 2.0, openai.LongContextInputCostMultiplier, 1e-12)
	require.InDelta(t, 1.5, openai.LongContextOutputCostMultiplier, 1e-12)

	gemini := data["gemini-above"]
	require.Equal(t, 200000, gemini.LongContextInputTokenThreshold)
	require.InDelta(t, 2.0, gemini.LongContextInputCostMultiplier, 1e-12)
	require.InDelta(t, 1.5, gemini.LongContextOutputCostMultiplier, 1e-12)

	explicit := data["explicit-wins"]
	require.Zero(t, explicit.LongContextInputTokenThreshold, "显式 long_context_* 字段优先，不做折算")
	require.InDelta(t, 1.0, explicit.LongContextInputCostMultiplier, 1e-12)

	require.Zero(t, data["no-surcharge"].LongContextInputTokenThreshold, "above 价不高于基础价视为无附加费")
	require.Zero(t, data["cache-only-above"].LongContextInputTokenThreshold, "仅 cache 侧 above 字段不构成阶梯")
	require.Equal(t, 128000, data["multi-threshold"].LongContextInputTokenThreshold, "多阈值取最小")
}

func TestGetModelPricing_XAIThresholdInclusive(t *testing.T) {
	svc := NewBillingService(&config.Config{}, newStubPricingServiceFromJSON(t, `{
		"grok-4.5": {"litellm_provider": "xai", "mode": "chat",
			"input_cost_per_token": 2e-06, "output_cost_per_token": 6e-06,
			"input_cost_per_token_above_200k_tokens": 4e-06,
			"output_cost_per_token_above_200k_tokens": 1.2e-05}
	}`))
	pricing, err := svc.GetModelPricing("grok-4.5")
	require.NoError(t, err)
	require.Equal(t, 200000, pricing.LongContextInputThreshold)
	require.True(t, pricing.LongContextThresholdInclusive, "xAI 阈值语义为达到即进高档")
}

// F3：显式 long_context 字段以"字段存在"为准——显式 0 也能压住 above 折算，关闭阶梯。
func TestParsePricingData_ExplicitZeroThresholdDisablesLadder(t *testing.T) {
	svc := &PricingService{}
	data, err := svc.parsePricingData([]byte(`{
		"gpt-5.5": {"litellm_provider": "openai", "mode": "chat",
			"input_cost_per_token": 5e-06, "output_cost_per_token": 3e-05,
			"long_context_input_token_threshold": 0,
			"input_cost_per_token_above_272k_tokens": 1e-05,
			"output_cost_per_token_above_272k_tokens": 4.5e-05}
	}`))
	require.NoError(t, err)
	require.Zero(t, data["gpt-5.5"].LongContextInputTokenThreshold)
	require.Zero(t, data["gpt-5.5"].LongContextInputCostMultiplier)
}

// cache 侧 above 档随输入倍率计费、不单独折算；缺基础价的 cache above 字段无法参与计费，
// 该缓存分项按 0 计，属于数据契约违规，必须有哨兵 WARN。服务档变体缺基础价时回落
// 标准基础价，不算孤儿。
func TestParsePricingData_WarnsOrphanCacheTierFields(t *testing.T) {
	logSink, restore := captureStructuredLog(t)
	defer restore()

	svc := &PricingService{}
	data, err := svc.parsePricingData([]byte(`{
		"gemini-orphan": {"litellm_provider": "vertex_ai-language-models", "mode": "chat",
			"input_cost_per_token": 1.25e-06, "output_cost_per_token": 1e-05,
			"cache_read_input_token_cost": 1.25e-07,
			"input_cost_per_token_above_200k_tokens": 2.5e-06,
			"output_cost_per_token_above_200k_tokens": 1.5e-05,
			"cache_read_input_token_cost_above_200k_tokens": 2.5e-07,
			"cache_creation_input_token_cost_above_200k_tokens": 2.5e-07},
		"gemini-complete": {"litellm_provider": "vertex_ai-language-models", "mode": "chat",
			"input_cost_per_token": 1.25e-06, "output_cost_per_token": 1e-05,
			"cache_read_input_token_cost": 1.25e-07,
			"cache_creation_input_token_cost": 1.25e-06,
			"input_cost_per_token_above_200k_tokens": 2.5e-06,
			"output_cost_per_token_above_200k_tokens": 1.5e-05,
			"cache_read_input_token_cost_above_200k_tokens": 2.5e-07,
			"cache_creation_input_token_cost_above_200k_tokens": 2.5e-06},
		"priority-variant-without-own-base": {"litellm_provider": "openai", "mode": "chat",
			"input_cost_per_token": 5e-06, "output_cost_per_token": 3e-05,
			"cache_read_input_token_cost": 5e-07,
			"input_cost_per_token_above_272k_tokens": 1e-05,
			"output_cost_per_token_above_272k_tokens": 4.5e-05,
			"cache_read_input_token_cost_above_272k_tokens_priority": 2e-06},
		"priority-variant-orphan": {"litellm_provider": "openai", "mode": "chat",
			"input_cost_per_token": 5e-06, "output_cost_per_token": 3e-05,
			"cache_creation_input_token_cost_above_272k_tokens_priority": 2.5e-05},
		"hourly-tier-with-5m-base": {"litellm_provider": "anthropic", "mode": "chat",
			"input_cost_per_token": 3e-06, "output_cost_per_token": 1.5e-05,
			"cache_creation_input_token_cost": 3.75e-06,
			"cache_creation_input_token_cost_above_1hr_above_200k_tokens": 1.2e-05},
		"hourly-tier-orphan": {"litellm_provider": "anthropic", "mode": "chat",
			"input_cost_per_token": 3e-06, "output_cost_per_token": 1.5e-05,
			"cache_creation_input_token_cost_above_1hr_above_200k_tokens": 1.2e-05}
	}`))
	require.NoError(t, err)

	require.Equal(t, 200000, data["gemini-orphan"].LongContextInputTokenThreshold, "孤儿 cache 字段不影响 input/output 阶梯折算")
	require.Zero(t, data["gemini-orphan"].CacheCreationInputTokenCost)
	require.InDelta(t, 1.25e-6, data["gemini-complete"].CacheCreationInputTokenCost, 1e-12)

	require.True(t, logSink.ContainsMessageAtLevel("gemini-orphan(cache_creation_input_token_cost_above_200k_tokens)", "warn"))
	require.True(t, logSink.ContainsMessage("priority-variant-orphan(cache_creation_input_token_cost_above_272k_tokens_priority)"))
	require.True(t, logSink.ContainsMessage("hourly-tier-orphan(cache_creation_input_token_cost_above_1hr_above_200k_tokens)"))
	require.False(t, logSink.ContainsMessage("gemini-complete"))
	require.False(t, logSink.ContainsMessage("priority-variant-without-own-base"))
	require.False(t, logSink.ContainsMessage("hourly-tier-with-5m-base"), "1h 档缺 above_1hr 基础价时计费回落 5m 价，不算孤儿")
}

// 基础价与 above 档来自不同价格版本时（如基础价被手工 pin、above 档随上游更新）会折算出
// 只有一侧带附加费的阶梯，必须有哨兵 WARN；显式 long_context_* 字段是部署方意图，不告警。
func TestParsePricingData_WarnsLopsidedLongContextLadder(t *testing.T) {
	logSink, restore := captureStructuredLog(t)
	defer restore()

	svc := &PricingService{}
	data, err := svc.parsePricingData([]byte(`{
		"mixed-versions": {"litellm_provider": "openai", "mode": "chat",
			"input_cost_per_token": 5e-06, "output_cost_per_token": 3e-05,
			"input_cost_per_token_above_272k_tokens": 8e-06,
			"output_cost_per_token_above_272k_tokens": 3e-05},
		"consistent": {"litellm_provider": "openai", "mode": "chat",
			"input_cost_per_token": 4e-06, "output_cost_per_token": 2e-05,
			"input_cost_per_token_above_272k_tokens": 8e-06,
			"output_cost_per_token_above_272k_tokens": 3e-05},
		"explicit-input-only": {"litellm_provider": "openai", "mode": "chat",
			"input_cost_per_token": 4e-06, "output_cost_per_token": 2e-05,
			"long_context_input_token_threshold": 272000,
			"long_context_input_cost_multiplier": 2}
	}`))
	require.NoError(t, err)

	require.Equal(t, 272000, data["mixed-versions"].LongContextInputTokenThreshold, "单侧阶梯仍按折算结果计费，只告警不丢弃")
	require.InDelta(t, 1.6, data["mixed-versions"].LongContextInputCostMultiplier, 1e-12)
	require.True(t, logSink.ContainsMessageAtLevel("mixed-versions(input x1.60, output x1.00)", "warn"))
	require.False(t, logSink.ContainsMessage("consistent"))
	require.False(t, logSink.ContainsMessage("explicit-input-only"))
}

// 出厂回退快照必须满足数据契约：没有孤儿 cache above 字段、没有单侧阶梯，且 Gemini pro 系的
// 缓存写入基础价等于标准输入价（含 priority 变体）。快照是随目录同步刷新的文本，这里防止刷新时静默回退。
func TestDefaultCatalogSnapshot_CacheTierContract(t *testing.T) {
	logSink, restore := captureStructuredLog(t)
	defer restore()

	body, err := os.ReadFile(filepath.Join("..", "..", "resources", "model-pricing", "model_prices_and_context_window.json"))
	require.NoError(t, err)

	var rawEntries map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(body, &rawEntries))
	for name, raw := range rawEntries {
		require.Empty(t, orphanCacheTierFields(raw), "快照条目 %s 带孤儿 cache above 字段", name)
	}

	svc := &PricingService{}
	data, err := svc.parsePricingData(body)
	require.NoError(t, err)
	require.False(t, logSink.ContainsMessage("carry cache above-tier prices"), "快照不应触发孤儿 cache 字段哨兵")
	require.False(t, logSink.ContainsMessage("one-sided long-context ladder"), "快照不应触发单侧阶梯哨兵")
	for _, model := range []string{
		"gemini-2.5-pro", "gemini-3-pro-preview", "gemini-3.1-pro-preview",
		"gemini-3.1-pro-high", "gemini-3.1-pro-low", "gemini-3.1-pro-preview-customtools",
	} {
		pricing := data[model]
		require.NotNil(t, pricing, model)
		require.Positive(t, pricing.InputCostPerToken, model)
		require.InDelta(t, pricing.InputCostPerToken, pricing.CacheCreationInputTokenCost, 1e-15, "%s 缓存写入基础价应等于标准输入价", model)
		require.Equal(t, 200000, pricing.LongContextInputTokenThreshold, model)
		if pricing.InputCostPerTokenPriority > 0 {
			require.InDelta(t, pricing.InputCostPerTokenPriority, pricing.CacheCreationInputTokenCostPriority, 1e-15, "%s priority 缓存写入价应等于 priority 输入价", model)
		}
	}
}

// F1：显式字段只写了一侧倍率时，缺失侧按 1 计而不是乘 0 免费。
func TestCalculateCost_PartialLongContextMultiplierDefaultsToOne(t *testing.T) {
	tokens := UsageTokens{InputTokens: 300000, OutputTokens: 1000, CacheReadTokens: 10000}

	t.Run("only input multiplier", func(t *testing.T) {
		svc := NewBillingService(&config.Config{}, newStubPricingServiceFromJSON(t, `{
			"partial-in": {"litellm_provider": "openai", "mode": "chat",
				"input_cost_per_token": 2e-06, "output_cost_per_token": 1e-05,
				"cache_read_input_token_cost": 2e-07,
				"long_context_input_token_threshold": 272000,
				"long_context_input_cost_multiplier": 2.0}
		}`))
		cost, err := svc.CalculateCost("partial-in", tokens, 1.0)
		require.NoError(t, err)
		require.True(t, cost.LongContextBillingApplied)
		require.InDelta(t, 300000*2e-6*2, cost.InputCost, 1e-10)
		require.InDelta(t, 1000*1e-5, cost.OutputCost, 1e-10, "缺失的 output 倍率按 1 计，不得为 0")
		require.InDelta(t, 10000*2e-7*2, cost.CacheReadCost, 1e-10)
	})

	t.Run("only output multiplier", func(t *testing.T) {
		svc := NewBillingService(&config.Config{}, newStubPricingServiceFromJSON(t, `{
			"partial-out": {"litellm_provider": "openai", "mode": "chat",
				"input_cost_per_token": 2e-06, "output_cost_per_token": 1e-05,
				"cache_read_input_token_cost": 2e-07,
				"long_context_input_token_threshold": 272000,
				"long_context_output_cost_multiplier": 1.5}
		}`))
		cost, err := svc.CalculateCost("partial-out", tokens, 1.0)
		require.NoError(t, err)
		require.True(t, cost.LongContextBillingApplied)
		require.InDelta(t, 300000*2e-6, cost.InputCost, 1e-10, "缺失的 input 倍率按 1 计，不得为 0")
		require.InDelta(t, 1000*1e-5*1.5, cost.OutputCost, 1e-10)
		require.InDelta(t, 10000*2e-7, cost.CacheReadCost, 1e-10, "cache_read 跟随 input 倍率，同样按 1 计")
	})
}

// 行为声明：目录带 above_200k 的 Claude sonnet 条目同样获得数据驱动的整单阶梯
// （与 Anthropic 官方 1M 长上下文定价一致），受分组长上下文开关约束。
func TestCalculateCost_ClaudeSonnetCatalogLadderIsDataDriven(t *testing.T) {
	svc := NewBillingService(&config.Config{}, newStubPricingServiceFromJSON(t, `{
		"claude-sonnet-4-5": {"litellm_provider": "anthropic", "mode": "chat",
			"input_cost_per_token": 3e-06, "output_cost_per_token": 1.5e-05,
			"cache_read_input_token_cost": 3e-07,
			"input_cost_per_token_above_200k_tokens": 6e-06,
			"output_cost_per_token_above_200k_tokens": 2.25e-05,
			"cache_read_input_token_cost_above_200k_tokens": 6e-07}
	}`))

	pricing, err := svc.GetModelPricing("claude-sonnet-4-5")
	require.NoError(t, err)
	require.Equal(t, 200000, pricing.LongContextInputThreshold)
	require.False(t, pricing.LongContextThresholdInclusive, "anthropic 为严格大于")

	over := UsageTokens{InputTokens: 250000, OutputTokens: 1000}
	cost, err := svc.CalculateCost("claude-sonnet-4-5", over, 1.0)
	require.NoError(t, err)
	require.True(t, cost.LongContextBillingApplied)
	require.InDelta(t, 250000*3e-6*2, cost.InputCost, 1e-10)
	require.InDelta(t, 1000*1.5e-5*1.5, cost.OutputCost, 1e-10)

	under := UsageTokens{InputTokens: 200000, OutputTokens: 1000}
	cost, err = svc.CalculateCost("claude-sonnet-4-5", under, 1.0)
	require.NoError(t, err)
	require.False(t, cost.LongContextBillingApplied, "恰好 200000 不进高档（严格大于）")
}
