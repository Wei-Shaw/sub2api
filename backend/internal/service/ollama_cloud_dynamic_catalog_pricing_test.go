//go:build unit

package service

// 生产形态复现/回归：动态定价目录已加载（目录含 gpt-5.4）时，8 个 ollama 独占模型
// 经 BillingService 两级查价的实际解析结果，逐名钉死。
//
// 真实调用顺序（文件:行号 证据）：
//   - BillingService.getModelPricingAtOnce（billing_service.go:1351）先跑
//     PricingService.GetModelPricing（动态目录 + 模糊兜底，billing_service.go:1357），
//     命中即短路 fallbackPrices（billing_service.go:1366-1394）；miss 才走
//     getFallbackPricing（billing_service.go:1398）。
//   - PricingService.GetModelPricing（pricing_service.go:1128）对任何 "gpt-" 前缀名
//     进 matchOpenAIModel（pricing_service.go:1151-1153），其末端 catch-all 回落
//     pricingData[openai.DefaultTestModel] = "gpt-5.4"（pricing_service.go:1542-1547；
//     internal/pkg/openai/constants.go:50）。
//   - gpt-oss 是开放权重模型、不是 OpenAI 模型：目录无其条目时，名字会被这条
//     catch-all 按 gpt-5.4 价卡（$2.5/$15 per MTok）计费，而不是 fallbackPrices 的
//     ollama 价卡（$0.15/$0.60 与 $0.07/$0.30，billing_service.go:848-859）。
//
// 期望值与 TestGetModelPricing_OllamaCloudRealModelList 一致（来源 fallbackPrices
// 的 ollama 条目）；差异仅在目录状态：这里模拟「动态目录已加载且含 gpt-5.4」的
// 生产常态（openAILadderCatalogJSON 即真实目录同步形态）。

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestGetModelPricing_OllamaCloudModelsWithDynamicCatalogLoaded(t *testing.T) {
	// 生产形态：动态目录已加载且含 gpt-5.4（$2.5/$15 per MTok）。
	bs := newTestBillingServiceWithOpenAILadderCatalog(t)

	tests := []struct {
		model     string
		wantInput float64 // USD per token，等于 fallbackPrices 里的 ollama 价卡
		wantOut   float64
	}{
		{"gpt-oss:120b", 0.15e-6, 0.60e-6},
		{"gpt-oss:20b", 0.07e-6, 0.30e-6},
		{"gemma4:31b", 0.14e-6, 0.40e-6},
		{"mistral-large-3:675b", 0.50e-6, 1.50e-6},
		{"qwen3.5:397b", 0.60e-6, 3.60e-6},
		{"nemotron-3-super", 0.015e-6, 0.60e-6},
		{"nemotron-3-nano:30b", 0.06e-6, 0.24e-6},
		{"nemotron-3-ultra", 0.10e-6, 3.00e-6},
	}

	for _, tt := range tests {
		t.Run(tt.model, func(t *testing.T) {
			pricing, err := bs.getModelPricingAt(tt.model, ollamaPricingAt)
			require.NoError(t, err, "model %s must be priceable with the dynamic catalog loaded", tt.model)
			require.NotNil(t, pricing)
			require.InDelta(t, tt.wantInput, pricing.InputPricePerToken, 1e-12,
				"model %s must bill at its ollama card, not an OpenAI family catch-all", tt.model)
			require.InDelta(t, tt.wantOut, pricing.OutputPricePerToken, 1e-12,
				"model %s must bill at its ollama card, not an OpenAI family catch-all", tt.model)
		})
	}
}

// TestGetModelPricing_OpenAIChainUnaffectedByOllamaExclusion 回归护栏：把 gpt-oss*
// 排除出 OpenAI 回退链后，真实 OpenAI 名字的解析必须不受影响 —— 目录精确命中 /
// 家族 catch-all / 静态兜底三态各钉一例（值 = 目录 gpt-5.4 卡 $2.5/$15）。
func TestGetModelPricing_OpenAIChainUnaffectedByOllamaExclusion(t *testing.T) {
	bs := newTestBillingServiceWithOpenAILadderCatalog(t)

	tests := []struct {
		model     string
		hit       string
		wantInput float64
		wantOut   float64
	}{
		// 目录精确命中：gpt-5.4 本尊走目录价卡。
		{"gpt-5.4", "catalog-exact", 2.5e-6, 1.5e-5},
		// 目录无 gpt-4o 条目：经 OpenAI 链 catch-all 落 DefaultTestModel 目录卡 ——
		// 该 catch-all 对真实 OpenAI 名必须保持可达。
		{"gpt-4o", "openai-catch-all", 2.5e-6, 1.5e-5},
		// gpt-5.5 → 业务静态 GPT-5.4 兜底价（openAIGPT54FallbackPricing，$2.5/$15）。
		{"gpt-5.5", "static-gpt54-fallback", 2.5e-6, 1.5e-5},
	}

	for _, tt := range tests {
		t.Run(tt.model, func(t *testing.T) {
			pricing, err := bs.getModelPricingAt(tt.model, ollamaPricingAt)
			require.NoError(t, err, "model %s (hit %s)", tt.model, tt.hit)
			require.NotNil(t, pricing)
			require.InDelta(t, tt.wantInput, pricing.InputPricePerToken, 1e-12,
				"model %s (hit %s) input price must stay on the OpenAI chain", tt.model, tt.hit)
			require.InDelta(t, tt.wantOut, pricing.OutputPricePerToken, 1e-12,
				"model %s (hit %s) output price must stay on the OpenAI chain", tt.model, tt.hit)
		})
	}
}
