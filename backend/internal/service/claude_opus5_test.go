package service

import (
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/domain"
	"github.com/Wei-Shaw/sub2api/internal/pkg/claude"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Claude Opus 5 官方定价（USD per token）：$5 输入 / $25 输出 per MTok。
const (
	opus5InputPricePerToken         = 5e-6
	opus5OutputPricePerToken        = 25e-6
	opus5CacheCreationPricePerToken = 6.25e-6
	opus5CacheReadPricePerToken     = 0.5e-6
)

// TestClaudeOpus5_FamilyFallbackDoesNotUseOpus4Rates 覆盖定价数据里还没有
// claude-opus-5 条目的场景（远端价格表滞后于模型发布）。
// 修复前 matchByModelFamily 会把 claude-opus-5 归到 "opus-4" 系列，
// 按 $15/$75 计费 —— 输入与输出双双 3 倍超收。
func TestClaudeOpus5_FamilyFallbackDoesNotUseOpus4Rates(t *testing.T) {
	svc := NewBillingService(&config.Config{}, &PricingService{
		pricingData: map[string]*LiteLLMModelPricing{
			// 有 4.8（同价），故意不放 claude-opus-5
			"claude-opus-4-8": {
				InputCostPerToken:           opus5InputPricePerToken,
				OutputCostPerToken:          opus5OutputPricePerToken,
				CacheCreationInputTokenCost: opus5CacheCreationPricePerToken,
				CacheReadInputTokenCost:     opus5CacheReadPricePerToken,
			},
			// 修复前会被误命中的旧 Opus 条目（$15/$75）
			"claude-opus-4-1":        {InputCostPerToken: 15e-6, OutputCostPerToken: 75e-6},
			"claude-opus-4-20250514": {InputCostPerToken: 15e-6, OutputCostPerToken: 75e-6},
			"claude-3-opus-20240229": {InputCostPerToken: 15e-6, OutputCostPerToken: 75e-6},
		},
	})

	for _, model := range []string{"claude-opus-5", "us.anthropic.claude-opus-5-v1"} {
		t.Run(model, func(t *testing.T) {
			pricing, err := svc.GetModelPricing(model)
			require.NoError(t, err)
			require.NotNil(t, pricing)
			assert.InDelta(t, opus5InputPricePerToken, pricing.InputPricePerToken, 1e-12)
			assert.InDelta(t, opus5OutputPricePerToken, pricing.OutputPricePerToken, 1e-12)
		})
	}
}

// TestClaudeOpus5_HardcodedFallbackPricing 覆盖动态价格服务完全不可用时的
// 硬编码兜底表。同时锁定不能被 "opus-5" 子串误伤的相邻型号。
func TestClaudeOpus5_HardcodedFallbackPricing(t *testing.T) {
	// pricingService 为 nil，强制走硬编码兜底表
	svc := NewBillingService(&config.Config{}, nil)

	tests := []struct {
		model  string
		input  float64
		output float64
	}{
		{"claude-opus-5", opus5InputPricePerToken, opus5OutputPricePerToken},
		{"us.anthropic.claude-opus-5-v1", opus5InputPricePerToken, opus5OutputPricePerToken},
		// 4.8 与 5 同价；修复前兜底表缺失，会掉到 claude-3-opus 的 $15/$75
		{"claude-opus-4-8", opus5InputPricePerToken, opus5OutputPricePerToken},
		// 相邻型号不能被 "opus-5" 误匹配
		{"claude-opus-4-5-20251101", 5e-6, 25e-6},
		{"claude-opus-4-1-20250805", 15e-6, 75e-6},
		{"claude-3-opus-20240229", 15e-6, 75e-6},
	}
	for _, tt := range tests {
		t.Run(tt.model, func(t *testing.T) {
			pricing, err := svc.GetModelPricing(tt.model)
			require.NoError(t, err)
			require.NotNil(t, pricing)
			assert.InDelta(t, tt.input, pricing.InputPricePerToken, 1e-12)
			assert.InDelta(t, tt.output, pricing.OutputPricePerToken, 1e-12)
		})
	}

	opus5, err := svc.GetModelPricing("claude-opus-5")
	require.NoError(t, err)
	assert.InDelta(t, opus5CacheCreationPricePerToken, opus5.CacheCreationPricePerToken, 1e-12)
	assert.InDelta(t, opus5CacheReadPricePerToken, opus5.CacheReadPricePerToken, 1e-12)
}

// TestClaudeOpus5_BedrockCapabilityGates 锁定只有主版本号的模型 ID
// （claude-opus-5 / claude-sonnet-5）能被版本闸门识别。
// 修复前 claudeVersionRe 强制要求 major-minor，这类 ID 完全不匹配，
// 会被当成旧模型降级。
func TestClaudeOpus5_BedrockCapabilityGates(t *testing.T) {
	tests := []struct {
		modelID       string
		claude45Newer bool
		toolSearch    bool
		opus47Newer   bool
	}{
		{"claude-opus-5", true, true, true},
		{"us.anthropic.claude-opus-5-v1", true, true, true},
		{"eu.anthropic.claude-opus-5-v1", true, true, true},
		{"claude-sonnet-5", true, true, false},
		{"us.anthropic.claude-sonnet-5-v1", true, true, false},
		// 回归保护：旧模型不能因为 minor 可选而被误判为新版本
		{"anthropic.claude-opus-4-1-v1", false, false, false},
		{"anthropic.claude-sonnet-4-0-v1", false, false, false},
		{"anthropic.claude-3-opus-20240229-v1:0", false, false, false},
		{"us.anthropic.claude-opus-4-8-v1", true, true, true},
		// Haiku 不支持 tool search
		{"us.anthropic.claude-haiku-4-5-20251001-v1:0", true, false, false},
	}
	for _, tt := range tests {
		t.Run(tt.modelID, func(t *testing.T) {
			assert.Equal(t, tt.claude45Newer, isBedrockClaude45OrNewer(tt.modelID), "isBedrockClaude45OrNewer")
			assert.Equal(t, tt.toolSearch, bedrockModelSupportsToolSearch(tt.modelID), "bedrockModelSupportsToolSearch")
			assert.Equal(t, tt.opus47Newer, isBedrockOpus47OrNewer(tt.modelID), "isBedrockOpus47OrNewer")
		})
	}
}

// TestClaudeOpus5_BedrockThinkingConvertedToAdaptive 验证 Opus 5 在 Bedrock 路径上
// 会把 thinking.type=enabled 转成 adaptive 并移除 budget_tokens。
// 上游 Opus 5 已移除 budget_tokens，透传过去会直接 400。
func TestClaudeOpus5_BedrockThinkingConvertedToAdaptive(t *testing.T) {
	body := []byte(`{"thinking":{"type":"enabled","budget_tokens":10000}}`)
	got := sanitizeBedrockThinking(body, "us.anthropic.claude-opus-5-v1")

	assert.JSONEq(t, `{"thinking":{"type":"adaptive"}}`, string(got))
}

// TestClaudeOpus5_CatalogAndBedrockMapping 锁定模型清单与 Bedrock 默认映射。
func TestClaudeOpus5_CatalogAndBedrockMapping(t *testing.T) {
	assert.Contains(t, claude.DefaultModelIDs(), "claude-opus-5")

	mapped, ok := domain.DefaultBedrockModelMapping["claude-opus-5"]
	require.True(t, ok, "claude-opus-5 missing from DefaultBedrockModelMapping")
	assert.Equal(t, "us.anthropic.claude-opus-5-v1", mapped)
}
