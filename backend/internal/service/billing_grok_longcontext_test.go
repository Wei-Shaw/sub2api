package service

import (
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

// newGrokCatalogBillingService 构造「远程目录已命中但缺长上下文字段」的计费服务，
// 复刻生产环境：LiteLLM 目录的 grok 条目只有基础单价，长上下文规则必须由
// applyModelSpecificPricingPolicy 从本地价卡回填。
func newGrokCatalogBillingService(entries map[string]*LiteLLMModelPricing) *BillingService {
	return NewBillingService(&config.Config{}, &PricingService{pricingData: entries})
}

func grokCatalogEntry(model string, in, out, cacheRead float64) *LiteLLMModelPricing {
	return &LiteLLMModelPricing{
		InputCostPerToken:       in,
		OutputCostPerToken:      out,
		CacheReadInputTokenCost: cacheRead,
		LiteLLMProvider:         "xai",
		Mode:                    "chat",
		SupportsPromptCaching:   true,
	}
}

// 回归：远程目录命中 grok-4.5 后，长上下文价卡（200K 含边界、2x/2x）必须被回填，
// 否则 ≥200K 请求按基础单价少计费（目录条目遮蔽 fallback 导致规则失效）。
func TestCalculateCost_GrokCatalogEntryBackfillsLongContext(t *testing.T) {
	svc := newGrokCatalogBillingService(map[string]*LiteLLMModelPricing{
		"grok-4.5": grokCatalogEntry("grok-4.5", 2e-6, 6e-6, 0.3e-6),
	})

	pricing, err := svc.GetModelPricing("grok-4.5")
	require.NoError(t, err)
	require.InDelta(t, 2e-6, pricing.InputPricePerToken, 1e-12, "base rates must come from the catalog entry")
	require.Equal(t, 200000, pricing.LongContextInputThreshold)
	require.InDelta(t, 2.0, pricing.LongContextInputMultiplier, 1e-12)
	require.InDelta(t, 2.0, pricing.LongContextOutputMultiplier, 1e-12)
	require.True(t, pricing.LongContextThresholdInclusive, "xAI long-context tier is inclusive (>=200K)")
}

// 边界：恰好 200K（含边界）触发整单加倍；199999 不触发。
func TestCalculateCost_GrokLongContextInclusiveBoundary(t *testing.T) {
	svc := newGrokCatalogBillingService(map[string]*LiteLLMModelPricing{
		"grok-4.5": grokCatalogEntry("grok-4.5", 2e-6, 6e-6, 0.3e-6),
	})

	atThreshold, err := svc.CalculateCost("grok-4.5", UsageTokens{InputTokens: 200000, OutputTokens: 1000}, 1.0)
	require.NoError(t, err)
	require.True(t, atThreshold.LongContextBillingApplied, "exactly 200K input must trigger long-context billing (inclusive)")
	require.InDelta(t, 200000*2e-6*2, atThreshold.InputCost, 1e-9)
	require.InDelta(t, 1000*6e-6*2, atThreshold.OutputCost, 1e-9)

	below, err := svc.CalculateCost("grok-4.5", UsageTokens{InputTokens: 199999, OutputTokens: 1000}, 1.0)
	require.NoError(t, err)
	require.False(t, below.LongContextBillingApplied)
	require.InDelta(t, 199999*2e-6, below.InputCost, 1e-9)
	require.InDelta(t, 1000*6e-6, below.OutputCost, 1e-9)
}

// cache_read / cache_creation 计入总输入，且触发后同样按 2x 计费（与 #2293 同口径）。
func TestCalculateCost_GrokLongContextCountsCacheTokens(t *testing.T) {
	svc := newGrokCatalogBillingService(map[string]*LiteLLMModelPricing{
		"grok-4.5": grokCatalogEntry("grok-4.5", 2e-6, 6e-6, 0.3e-6),
	})

	// InputTokens + CacheReadTokens = 1000 + 199000 = 200000，恰好触发
	tokens := UsageTokens{InputTokens: 1000, CacheReadTokens: 199000, OutputTokens: 1000}
	cost, err := svc.CalculateCost("grok-4.5", tokens, 1.0)
	require.NoError(t, err)
	require.True(t, cost.LongContextBillingApplied)
	require.InDelta(t, 1000*2e-6*2, cost.InputCost, 1e-9)
	require.InDelta(t, 199000*0.3e-6*2, cost.CacheReadCost, 1e-9)
	require.InDelta(t, 1000*6e-6*2, cost.OutputCost, 1e-9)
}

// 各带长上下文档的 Grok 型号（含别名与 grok-4.20 变体）在目录缺字段时都触发加倍。
func TestCalculateCost_GrokLongContextCoversAllRateCards(t *testing.T) {
	catalog := map[string]*LiteLLMModelPricing{
		"grok-4.6":                 grokCatalogEntry("grok-4.6", 2e-6, 6e-6, 0.5e-6),
		"grok-4.3":                 grokCatalogEntry("grok-4.3", 1.25e-6, 2.5e-6, 0.2e-6),
		"grok-4.20-0309-reasoning": grokCatalogEntry("grok-4.20-0309-reasoning", 1.25e-6, 2.5e-6, 0.2e-6),
		"grok-build-0.1":           grokCatalogEntry("grok-build-0.1", 1e-6, 2e-6, 0.2e-6),
	}
	svc := newGrokCatalogBillingService(catalog)
	tokens := UsageTokens{InputTokens: 250000, OutputTokens: 1000}

	for model, inPrice := range map[string]float64{
		"grok-4.6":                 2e-6,
		"grok":                     2e-6, // 别名 → grok-4.6 价卡
		"grok-4.3":                 1.25e-6,
		"grok-4.20-0309-reasoning": 1.25e-6,
		"grok-build-0.1":           1e-6,
	} {
		t.Run(model, func(t *testing.T) {
			cost, err := svc.CalculateCost(model, tokens, 1.0)
			require.NoError(t, err)
			require.True(t, cost.LongContextBillingApplied, "model %s must bill long-context at >=200K", model)
			require.InDelta(t, 250000*inPrice*2, cost.InputCost, 1e-9)
		})
	}
}

// grok-3-mini 价卡没有长上下文档：即使输入超 200K 也按基础单价，不回填。
func TestCalculateCost_Grok3MiniHasNoLongContextTier(t *testing.T) {
	svc := newGrokCatalogBillingService(map[string]*LiteLLMModelPricing{
		"grok-3-mini": grokCatalogEntry("grok-3-mini", 0.3e-6, 0.5e-6, 0.075e-6),
	})

	cost, err := svc.CalculateCost("grok-3-mini", UsageTokens{InputTokens: 300000, OutputTokens: 1000}, 1.0)
	require.NoError(t, err)
	require.False(t, cost.LongContextBillingApplied)
	require.InDelta(t, 300000*0.3e-6, cost.InputCost, 1e-9)
	require.InDelta(t, 1000*0.5e-6, cost.OutputCost, 1e-9)
}

// 非 Grok 模型不受回填影响：目录条目无长上下文字段时保持基础计费。
func TestCalculateCost_NonGrokModelUnaffectedByGrokPolicy(t *testing.T) {
	svc := newGrokCatalogBillingService(map[string]*LiteLLMModelPricing{
		"claude-sonnet-4": {InputCostPerToken: 3e-6, OutputCostPerToken: 15e-6, LiteLLMProvider: "anthropic", Mode: "chat"},
	})

	cost, err := svc.CalculateCost("claude-sonnet-4", UsageTokens{InputTokens: 300000, OutputTokens: 1000}, 1.0)
	require.NoError(t, err)
	require.False(t, cost.LongContextBillingApplied)
	require.InDelta(t, 300000*3e-6, cost.InputCost, 1e-9)
}

// 目录解析必须保留 long_context_threshold_inclusive 字段（上游目录补齐字段后
// 不能丢语义），与 GPT 侧严格 > 的缺省语义区分。
func TestParsePricingData_PreservesLongContextThresholdInclusive(t *testing.T) {
	ps := &PricingService{}
	body := []byte(`{
		"grok-4.5": {
			"input_cost_per_token": 0.000002,
			"output_cost_per_token": 0.000006,
			"long_context_input_token_threshold": 200000,
			"long_context_input_cost_multiplier": 2,
			"long_context_output_cost_multiplier": 2,
			"long_context_threshold_inclusive": true
		}
	}`)

	data, err := ps.parsePricingData(body)
	require.NoError(t, err)
	entry := data["grok-4.5"]
	require.NotNil(t, entry)
	require.Equal(t, 200000, entry.LongContextInputTokenThreshold)
	require.True(t, entry.LongContextThresholdInclusive)

	// 目录已带完整规则时，回填必须幂等（不改变目录单价，也不重复叠加倍率）。
	svc := NewBillingService(&config.Config{}, &PricingService{pricingData: data})
	cost, err := svc.CalculateCost("grok-4.5", UsageTokens{InputTokens: 200000, OutputTokens: 1000}, 1.0)
	require.NoError(t, err)
	require.True(t, cost.LongContextBillingApplied)
	require.InDelta(t, 200000*2e-6*2, cost.InputCost, 1e-9)
}
