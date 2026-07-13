//go:build unit

package service

import (
	"bytes"
	"log"
	"math"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

// captureStdLog 重定向 stdlib log 输出到 buffer,返回该 buffer;通过 t.Cleanup 还原。
// 用于断言 GetModelPricing 的 fallback warn(log.Printf)打了几次。
func captureStdLog(t *testing.T) *bytes.Buffer {
	t.Helper()
	var buf bytes.Buffer
	prevOut := log.Writer()
	prevFlags := log.Flags()
	log.SetOutput(&buf)
	log.SetFlags(0)
	t.Cleanup(func() {
		log.SetOutput(prevOut)
		log.SetFlags(prevFlags)
	})
	return &buf
}

func newTestBillingService() *BillingService {
	return NewBillingService(&config.Config{}, nil)
}

func TestCalculateCost_BasicComputation(t *testing.T) {
	svc := newTestBillingService()

	// 使用 claude-sonnet-4 的回退价格：Input $3/MTok, Output $15/MTok
	tokens := UsageTokens{
		InputTokens:  1000,
		OutputTokens: 500,
	}
	cost, err := svc.CalculateCost("claude-sonnet-4", tokens, 1.0)
	require.NoError(t, err)

	// 1000 * 3e-6 = 0.003, 500 * 15e-6 = 0.0075
	expectedInput := 1000 * 3e-6
	expectedOutput := 500 * 15e-6
	require.InDelta(t, expectedInput, cost.InputCost, 1e-10)
	require.InDelta(t, expectedOutput, cost.OutputCost, 1e-10)
	require.InDelta(t, expectedInput+expectedOutput, cost.TotalCost, 1e-10)
	require.InDelta(t, expectedInput+expectedOutput, cost.ActualCost, 1e-10)
}

func TestCalculateCost_WithCacheTokens(t *testing.T) {
	svc := newTestBillingService()

	tokens := UsageTokens{
		InputTokens:         1000,
		OutputTokens:        500,
		CacheCreationTokens: 2000,
		CacheReadTokens:     3000,
	}
	cost, err := svc.CalculateCost("claude-sonnet-4", tokens, 1.0)
	require.NoError(t, err)

	expectedCacheCreation := 2000 * 3.75e-6
	expectedCacheRead := 3000 * 0.3e-6
	require.InDelta(t, expectedCacheCreation, cost.CacheCreationCost, 1e-10)
	require.InDelta(t, expectedCacheRead, cost.CacheReadCost, 1e-10)

	expectedTotal := cost.InputCost + cost.OutputCost + expectedCacheCreation + expectedCacheRead
	require.InDelta(t, expectedTotal, cost.TotalCost, 1e-10)
}

func TestCalculateCost_RateMultiplier(t *testing.T) {
	svc := newTestBillingService()

	tokens := UsageTokens{InputTokens: 1000, OutputTokens: 500}

	cost1x, err := svc.CalculateCost("claude-sonnet-4", tokens, 1.0)
	require.NoError(t, err)

	cost2x, err := svc.CalculateCost("claude-sonnet-4", tokens, 2.0)
	require.NoError(t, err)

	// TotalCost 不受倍率影响，ActualCost 翻倍
	require.InDelta(t, cost1x.TotalCost, cost2x.TotalCost, 1e-10)
	require.InDelta(t, cost1x.ActualCost*2, cost2x.ActualCost, 1e-10)
}

func TestGetModelPricing_FallbackMatchesByFamily(t *testing.T) {
	svc := newTestBillingService()

	tests := []struct {
		model         string
		expectedInput float64
	}{
		{"claude-opus-4.5-20250101", 5e-6},
		{"claude-3-opus-20240229", 15e-6},
		{"claude-sonnet-4-20250514", 3e-6},
		{"claude-3-5-sonnet-20241022", 3e-6},
		{"claude-3-5-haiku-20241022", 1e-6},
		{"claude-3-haiku-20240307", 0.25e-6},
	}

	for _, tt := range tests {
		pricing, err := svc.GetModelPricing(tt.model)
		require.NoError(t, err, "模型 %s", tt.model)
		require.InDelta(t, tt.expectedInput, pricing.InputPricePerToken, 1e-12, "模型 %s 输入价格", tt.model)
	}
}

func TestGetModelPricing_CaseInsensitive(t *testing.T) {
	svc := newTestBillingService()

	p1, err := svc.GetModelPricing("Claude-Sonnet-4")
	require.NoError(t, err)

	p2, err := svc.GetModelPricing("claude-sonnet-4")
	require.NoError(t, err)

	require.Equal(t, p1.InputPricePerToken, p2.InputPricePerToken)
}

// issue #3394: fallback warn 应按模型名去重,每个模型每进程最多打一条,
// 避免热路径每请求刷屏 ops_system_logs。
func TestGetModelPricing_FallbackWarnLoggedOncePerModel(t *testing.T) {
	svc := newTestBillingService()
	buf := captureStdLog(t)

	// glm-5.2 不在 LiteLLM,经 strings.Contains 命中 glm-5 兜底价 → 触发 fallback warn。
	for i := 0; i < 5; i++ {
		pricing, err := svc.GetModelPricing("glm-5.2")
		require.NoError(t, err)
		require.NotNil(t, pricing)
	}

	got := strings.Count(buf.String(), "Using fallback pricing for model: glm-5.2")
	require.Equal(t, 1, got, "同一模型的 fallback warn 应只打一条,实际日志:\n%s", buf.String())
}

// 去重按"每模型"而非全局:不同模型各打一条;大小写变体经入口 ToLower 归一,视为同一条目。
func TestGetModelPricing_FallbackWarnPerModelNotGlobal(t *testing.T) {
	svc := newTestBillingService()
	buf := captureStdLog(t)

	for i := 0; i < 3; i++ {
		_, _ = svc.GetModelPricing("glm-5.2")
		_, _ = svc.GetModelPricing("GLM-5.2") // 与上一行同模型(ToLower 后),去重后不再打
		_, _ = svc.GetModelPricing("glm-4.6")
	}

	out := buf.String()
	require.Equal(t, 1, strings.Count(out, "model: glm-5.2"), out)
	require.Equal(t, 1, strings.Count(out, "model: glm-4.6"), out)
	require.Equal(t, 0, strings.Count(out, "model: GLM-5.2"), out) // 大写经 ToLower 归一,不应单独成行
}

// 回归:glm-5.2 仍解析到 glm-5 兜底价(计费金额不变,防止日志改动掩盖未来计费回归)。
func TestGetModelPricing_GLM52FallsBackToGLM5Price(t *testing.T) {
	svc := newTestBillingService()

	got, err := svc.GetModelPricing("glm-5.2")
	require.NoError(t, err)
	require.NotNil(t, got)

	// glm-5 base：Input 1e-6 / Output 3.2e-6(见 TestGetFallbackPricing_FamilyMatching)。
	require.InDelta(t, 1e-6, got.InputPricePerToken, 1e-12)
	require.InDelta(t, 3.2e-6, got.OutputPricePerToken, 1e-12)
}

func TestGetModelPricing_UnknownClaudeModelFallsBackToSonnet(t *testing.T) {
	svc := newTestBillingService()

	// 不包含 opus/sonnet/haiku 关键词的 Claude 模型会走默认 Sonnet 价格
	pricing, err := svc.GetModelPricing("claude-unknown-model")
	require.NoError(t, err)
	require.InDelta(t, 3e-6, pricing.InputPricePerToken, 1e-12)
}

func TestGetModelPricing_UnknownOpenAIModelReturnsError(t *testing.T) {
	svc := newTestBillingService()

	pricing, err := svc.GetModelPricing("gpt-unknown-model")
	require.Error(t, err)
	require.Nil(t, pricing)
	require.Contains(t, err.Error(), "pricing not found")
}

func TestGetModelPricing_OpenAIGPT54Fallback(t *testing.T) {
	svc := newTestBillingService()

	pricing, err := svc.GetModelPricing("gpt-5.4")
	require.NoError(t, err)
	require.NotNil(t, pricing)
	require.InDelta(t, 2.5e-6, pricing.InputPricePerToken, 1e-12)
	require.InDelta(t, 15e-6, pricing.OutputPricePerToken, 1e-12)
	require.InDelta(t, 0.25e-6, pricing.CacheReadPricePerToken, 1e-12)
	require.Equal(t, 272000, pricing.LongContextInputThreshold)
	require.InDelta(t, 2.0, pricing.LongContextInputMultiplier, 1e-12)
	require.InDelta(t, 1.5, pricing.LongContextOutputMultiplier, 1e-12)
}

func TestGetModelPricing_OpenAICompactAliasesFallback(t *testing.T) {
	svc := newTestBillingService()

	tests := []struct {
		model       string
		inputPrice  float64
		outputPrice float64
		cacheRead   float64
		longContext int
	}{
		{model: "gpt5.5", inputPrice: 2.5e-6, outputPrice: 15e-6, cacheRead: 0.25e-6, longContext: 272000},
		{model: "openai/gpt5.4", inputPrice: 2.5e-6, outputPrice: 15e-6, cacheRead: 0.25e-6, longContext: 272000},
		{model: "gpt5.4-mini", inputPrice: 7.5e-7, outputPrice: 4.5e-6, cacheRead: 7.5e-8, longContext: 0},
		{model: "gpt5.3codexspark", inputPrice: 1.5e-6, outputPrice: 12e-6, cacheRead: 0.15e-6, longContext: 0},
	}

	for _, tt := range tests {
		t.Run(tt.model, func(t *testing.T) {
			pricing, err := svc.GetModelPricing(tt.model)
			require.NoError(t, err)
			require.NotNil(t, pricing)
			require.InDelta(t, tt.inputPrice, pricing.InputPricePerToken, 1e-12)
			require.InDelta(t, tt.outputPrice, pricing.OutputPricePerToken, 1e-12)
			require.InDelta(t, tt.cacheRead, pricing.CacheReadPricePerToken, 1e-12)
			require.Equal(t, tt.longContext, pricing.LongContextInputThreshold)
		})
	}
}

func TestGetModelPricing_OpenAIGPT54MiniFallback(t *testing.T) {
	svc := newTestBillingService()

	pricing, err := svc.GetModelPricing("gpt-5.4-mini")
	require.NoError(t, err)
	require.NotNil(t, pricing)
	require.InDelta(t, 7.5e-7, pricing.InputPricePerToken, 1e-12)
	require.InDelta(t, 4.5e-6, pricing.OutputPricePerToken, 1e-12)
	require.InDelta(t, 7.5e-8, pricing.CacheReadPricePerToken, 1e-12)
	require.Zero(t, pricing.LongContextInputThreshold)
}

func TestCalculateCost_OpenAIGPT54LongContextAppliesWholeSessionMultipliers(t *testing.T) {
	svc := newTestBillingService()

	tokens := UsageTokens{
		InputTokens:  300000,
		OutputTokens: 4000,
	}

	cost, err := svc.CalculateCost("gpt-5.4-2026-03-05", tokens, 1.0)
	require.NoError(t, err)

	expectedInput := float64(tokens.InputTokens) * 2.5e-6 * 2.0
	expectedOutput := float64(tokens.OutputTokens) * 15e-6 * 1.5
	require.InDelta(t, expectedInput, cost.InputCost, 1e-10)
	require.InDelta(t, expectedOutput, cost.OutputCost, 1e-10)
	require.InDelta(t, expectedInput+expectedOutput, cost.TotalCost, 1e-10)
	require.InDelta(t, expectedInput+expectedOutput, cost.ActualCost, 1e-10)
}

func TestCalculateCost_OpenAIGPT55ProUsesGPT55PricingPolicy(t *testing.T) {
	svc := newTestBillingService()

	tokens := UsageTokens{
		InputTokens:  300000,
		OutputTokens: 4000,
	}

	cost, err := svc.CalculateCost("gpt-5.5-pro", tokens, 1.0)
	require.NoError(t, err)

	expectedInput := float64(tokens.InputTokens) * 2.5e-6 * 2.0
	expectedOutput := float64(tokens.OutputTokens) * 15e-6 * 1.5
	require.InDelta(t, expectedInput, cost.InputCost, 1e-10)
	require.InDelta(t, expectedOutput, cost.OutputCost, 1e-10)
	require.InDelta(t, expectedInput+expectedOutput, cost.TotalCost, 1e-10)
	require.InDelta(t, expectedInput+expectedOutput, cost.ActualCost, 1e-10)
}

// 回归测试 #2293：长上下文计费触发时，cache_read_tokens 也应应用 LongContextInputMultiplier。
// 修复前：CacheReadCost = tokens * 0.25e-6 （漏乘倍率，少计费用）。
// 修复后：CacheReadCost = tokens * 0.25e-6 * LongContextInputMultiplier(=2.0)。
func TestCalculateCost_OpenAIGPT54LongContextAppliesMultiplierToCacheRead(t *testing.T) {
	svc := newTestBillingService()

	// InputTokens + CacheReadTokens = 1000 + 300000 = 301000 > 272000 阈值
	tokens := UsageTokens{
		InputTokens:     1000,
		CacheReadTokens: 300000,
		OutputTokens:    1000,
	}

	cost, err := svc.CalculateCost("gpt-5.4-2026-03-05", tokens, 1.0)
	require.NoError(t, err)

	expectedInput := float64(tokens.InputTokens) * 2.5e-6 * 2.0
	expectedOutput := float64(tokens.OutputTokens) * 15e-6 * 1.5
	expectedCacheRead := float64(tokens.CacheReadTokens) * 0.25e-6 * 2.0

	require.InDelta(t, expectedInput, cost.InputCost, 1e-10)
	require.InDelta(t, expectedOutput, cost.OutputCost, 1e-10)
	require.InDelta(t, expectedCacheRead, cost.CacheReadCost, 1e-10,
		"cache_read_cost should be scaled by LongContextInputMultiplier when long-context pricing applies (issue #2293)")

	expectedTotal := expectedInput + expectedOutput + expectedCacheRead
	require.InDelta(t, expectedTotal, cost.TotalCost, 1e-10)
	require.InDelta(t, expectedTotal, cost.ActualCost, 1e-10)
}

// 阴性测试：未触发长上下文时，cache_read_price 不应被错误地乘以倍率。
func TestCalculateCost_OpenAIGPT54NoLongContextKeepsCacheReadAtBasePrice(t *testing.T) {
	svc := newTestBillingService()

	// InputTokens + CacheReadTokens = 1000 + 100000 = 101000 < 272000 阈值，不触发长上下文
	tokens := UsageTokens{
		InputTokens:     1000,
		CacheReadTokens: 100000,
		OutputTokens:    1000,
	}

	cost, err := svc.CalculateCost("gpt-5.4-2026-03-05", tokens, 1.0)
	require.NoError(t, err)

	expectedCacheRead := float64(tokens.CacheReadTokens) * 0.25e-6
	require.InDelta(t, expectedCacheRead, cost.CacheReadCost, 1e-10,
		"cache_read_cost should remain at base price when below long-context threshold")
}

// 回归测试 #2816 follow-up：长上下文计费触发时，cache_creation_tokens 也应应用
// LongContextInputMultiplier。computeCacheCreationCost 直接读取 pricing.* 价格，
// 不经过 computeTokenBreakdown 内的 inputPrice / cacheReadPrice 倍率修改，因此
// 修复前 cache_creation 部分会按基础价计算，少计费用约 50%（默认倍率 2.0）。
func TestCalculateCost_OpenAIGPT54LongContextAppliesMultiplierToCacheCreation(t *testing.T) {
	svc := newTestBillingService()

	// InputTokens + CacheReadTokens = 1000 + 300000 = 301000 > 272000 阈值
	tokens := UsageTokens{
		InputTokens:         1000,
		CacheReadTokens:     300000,
		CacheCreationTokens: 10000,
		OutputTokens:        1000,
	}

	cost, err := svc.CalculateCost("gpt-5.4-2026-03-05", tokens, 1.0)
	require.NoError(t, err)

	// gpt-5.4 fallback: CacheCreationPricePerToken = 2.5e-6, LongContextInputMultiplier = 2.0
	expectedCacheCreation := float64(tokens.CacheCreationTokens) * 2.5e-6 * 2.0
	require.InDelta(t, expectedCacheCreation, cost.CacheCreationCost, 1e-10,
		"cache_creation_cost should be scaled by LongContextInputMultiplier when long-context pricing applies")
}

// 阴性测试：未触发长上下文时，cache_creation_price 不应被错误地乘以倍率。
func TestCalculateCost_OpenAIGPT54NoLongContextKeepsCacheCreationAtBasePrice(t *testing.T) {
	svc := newTestBillingService()

	// InputTokens + CacheReadTokens = 1000 + 100000 = 101000 < 272000 阈值，不触发长上下文
	tokens := UsageTokens{
		InputTokens:         1000,
		CacheReadTokens:     100000,
		CacheCreationTokens: 10000,
		OutputTokens:        1000,
	}

	cost, err := svc.CalculateCost("gpt-5.4-2026-03-05", tokens, 1.0)
	require.NoError(t, err)

	expectedCacheCreation := float64(tokens.CacheCreationTokens) * 2.5e-6
	require.InDelta(t, expectedCacheCreation, cost.CacheCreationCost, 1e-10,
		"cache_creation_cost should remain at base price when below long-context threshold")
}

// 覆盖 5m / 1h ephemeral 分类计费路径：长上下文触发时两档价格都应被倍率缩放。
// 使用手工构造的 pricing（参考 TestCalculateCost_SupportsCacheBreakdown 的写法）
// 以便同时控制 SupportsCacheBreakdown + 长上下文阈值。
func TestCalculateCost_LongContextAppliesMultiplierToCacheCreation5mAnd1h(t *testing.T) {
	svc := &BillingService{
		cfg: &config.Config{},
		fallbackPrices: map[string]*ModelPricing{
			"claude-sonnet-4": {
				InputPricePerToken:          3e-6,
				OutputPricePerToken:         15e-6,
				CacheReadPricePerToken:      0.3e-6,
				SupportsCacheBreakdown:      true,
				CacheCreation5mPrice:        4e-6,
				CacheCreation1hPrice:        5e-6,
				LongContextInputThreshold:   272000,
				LongContextInputMultiplier:  2.0,
				LongContextOutputMultiplier: 1.5,
			},
		},
	}

	// InputTokens + CacheReadTokens = 1000 + 300000 = 301000 > 272000 阈值
	tokens := UsageTokens{
		InputTokens:           1000,
		CacheReadTokens:       300000,
		CacheCreation5mTokens: 8000,
		CacheCreation1hTokens: 4000,
		OutputTokens:          1000,
	}

	cost, err := svc.CalculateCost("claude-sonnet-4", tokens, 1.0)
	require.NoError(t, err)

	expected5m := float64(tokens.CacheCreation5mTokens) * 4e-6 * 2.0
	expected1h := float64(tokens.CacheCreation1hTokens) * 5e-6 * 2.0
	require.InDelta(t, expected5m+expected1h, cost.CacheCreationCost, 1e-10,
		"both 5m and 1h cache_creation prices should be scaled by LongContextInputMultiplier")
}

func TestGetFallbackPricing_FamilyMatching(t *testing.T) {
	svc := newTestBillingService()

	floatPtr := func(v float64) *float64 { return &v }

	// expectedOutput / expectedCacheRead 为 nil 时跳过该字段断言（保持与原有用例兼容）。
	tests := []struct {
		name              string
		model             string
		expectedInput     float64
		expectedOutput    *float64
		expectedCacheRead *float64
		expectNilPricing  bool
	}{
		{name: "empty model", model: "   ", expectNilPricing: true},
		{name: "claude opus 4.6", model: "claude-opus-4.6-20260201", expectedInput: 5e-6},
		{name: "claude opus 4.5 alt separator", model: "claude-opus-4-5-20260101", expectedInput: 5e-6},
		{name: "claude generic model fallback sonnet", model: "claude-foo-bar", expectedInput: 3e-6},
		{name: "gemini explicit fallback", model: "gemini-3-1-pro", expectedInput: 2e-6},
		{name: "gemini unknown no fallback", model: "gemini-2.0-pro", expectNilPricing: true},
		{name: "openai gpt5.4", model: "gpt-5.4", expectedInput: 2.5e-6},
		{name: "openai gpt5.4 mini", model: "gpt-5.4-mini", expectedInput: 7.5e-7},
		{name: "openai gpt5.3 codex", model: "gpt-5.3-codex", expectedInput: 1.5e-6},
		{name: "openai gpt5.3 codex spark", model: "gpt-5.3-codex-spark", expectedInput: 1.5e-6},
		{name: "openai legacy gpt5.1 falls back to gpt5.4", model: "gpt-5.1", expectedInput: 2.5e-6},
		{name: "openai legacy gpt5.1 codex falls back to gpt5.3 codex", model: "gpt-5.1-codex", expectedInput: 1.5e-6},
		{name: "openai legacy codex mini latest falls back to gpt5.3 codex", model: "codex-mini-latest", expectedInput: 1.5e-6},
		{name: "openai unknown no fallback", model: "gpt-unknown-model", expectNilPricing: true},
		{
			name:              "deepseek v4 pro",
			model:             "deepseek-v4-pro",
			expectedInput:     4.35e-7,
			expectedOutput:    floatPtr(8.7e-7),
			expectedCacheRead: floatPtr(3.625e-9),
		},
		{
			name:              "deepseek v4 flash",
			model:             "deepseek-v4-flash",
			expectedInput:     1.4e-7,
			expectedOutput:    floatPtr(2.8e-7),
			expectedCacheRead: floatPtr(2.8e-9),
		},
		{
			name:              "deepseek chat alias → flash",
			model:             "deepseek-chat",
			expectedInput:     1.4e-7,
			expectedOutput:    floatPtr(2.8e-7),
			expectedCacheRead: floatPtr(2.8e-9),
		},
		{
			name:              "deepseek reasoner alias → flash",
			model:             "deepseek-reasoner",
			expectedInput:     1.4e-7,
			expectedOutput:    floatPtr(2.8e-7),
			expectedCacheRead: floatPtr(2.8e-9),
		},

		// ---- 智谱 GLM（z.ai USD 口径）----
		{
			name:              "glm 5.1 flagship",
			model:             "glm-5.1",
			expectedInput:     1.4e-6,
			expectedOutput:    floatPtr(4.4e-6),
			expectedCacheRead: floatPtr(0.26e-6),
		},
		{
			name:              "glm 5 base",
			model:             "glm-5",
			expectedInput:     1e-6,
			expectedOutput:    floatPtr(3.2e-6),
			expectedCacheRead: floatPtr(0.2e-6),
		},
		{
			name:              "glm 5 turbo",
			model:             "glm-5-turbo",
			expectedInput:     1.2e-6,
			expectedOutput:    floatPtr(4e-6),
			expectedCacheRead: floatPtr(0.24e-6),
		},
		{
			name:              "glm 4.7",
			model:             "glm-4.7",
			expectedInput:     0.6e-6,
			expectedOutput:    floatPtr(2.2e-6),
			expectedCacheRead: floatPtr(0.11e-6),
		},
		{
			name:              "glm 4.6",
			model:             "glm-4.6",
			expectedInput:     0.6e-6,
			expectedOutput:    floatPtr(2.2e-6),
			expectedCacheRead: floatPtr(0.11e-6),
		},
		{
			name:              "glm 4.5",
			model:             "glm-4.5",
			expectedInput:     0.6e-6,
			expectedOutput:    floatPtr(2.2e-6),
			expectedCacheRead: floatPtr(0.11e-6),
		},
		{
			name:              "glm 4.5-x premium",
			model:             "glm-4.5-x",
			expectedInput:     2.2e-6,
			expectedOutput:    floatPtr(8.9e-6),
			expectedCacheRead: floatPtr(0.45e-6),
		},
		{
			name:              "glm 4.5-air lightweight",
			model:             "glm-4.5-air",
			expectedInput:     0.2e-6,
			expectedOutput:    floatPtr(1.1e-6),
			expectedCacheRead: floatPtr(0.03e-6),
		},
		{
			name:              "glm 4.7-flashx",
			model:             "glm-4.7-flashx",
			expectedInput:     0.07e-6,
			expectedOutput:    floatPtr(0.4e-6),
			expectedCacheRead: floatPtr(0.01e-6),
		},
		{
			name:              "glm 4.5-flash free tier",
			model:             "glm-4.5-flash",
			expectedInput:     0, // Free tier on z.ai
			expectedOutput:    floatPtr(0),
			expectedCacheRead: floatPtr(0),
		},
		{
			name:              "glm 4.7-flash free tier",
			model:             "glm-4.7-flash",
			expectedInput:     0,
			expectedOutput:    floatPtr(0),
			expectedCacheRead: floatPtr(0),
		},
		{
			name:           "glm 4-32b legacy",
			model:          "glm-4-32b-0414-128k",
			expectedInput:  0.1e-6,
			expectedOutput: floatPtr(0.1e-6),
		},
		// 关键：5.1 必须先于 5 匹配（避免被 glm-5 抢走）
		{
			name:              "glm 5.1 vs glm 5 ordering (verbatim 5.1)",
			model:             "glm-5.1",
			expectedInput:     1.4e-6, // = glm-5.1 价格
			expectedOutput:    floatPtr(4.4e-6),
			expectedCacheRead: floatPtr(0.26e-6),
		},
		{
			name:              "glm 4.5-air vs glm 4.5 ordering",
			model:             "glm-4.5-air",
			expectedInput:     0.2e-6, // = glm-4.5-air 价格（不是 glm-4.5 的 0.6e-6）
			expectedOutput:    floatPtr(1.1e-6),
			expectedCacheRead: floatPtr(0.03e-6),
		},

		// ---- 月之暗面 Kimi ----
		{
			name:              "kimi k2.6 flagship",
			model:             "kimi-k2.6",
			expectedInput:     0.95e-6,
			expectedOutput:    floatPtr(4e-6),
			expectedCacheRead: floatPtr(0.15e-6),
		},
		{
			name:              "kimi for coding explicit alias",
			model:             "kimi-for-coding",
			expectedInput:     0.95e-6,
			expectedOutput:    floatPtr(4e-6),
			expectedCacheRead: floatPtr(0.15e-6),
		},
		{
			name:              "kimi k2.5",
			model:             "kimi-k2.5",
			expectedInput:     0.60e-6,
			expectedOutput:    floatPtr(3e-6),
			expectedCacheRead: floatPtr(0.098e-6),
		},
		{
			name:              "kimi k2-thinking",
			model:             "kimi-k2-thinking",
			expectedInput:     0.56e-6,
			expectedOutput:    floatPtr(2.24e-6),
			expectedCacheRead: floatPtr(0.14e-6),
		},
		{
			name:              "kimi k2 base",
			model:             "kimi-k2",
			expectedInput:     0.56e-6,
			expectedOutput:    floatPtr(2.24e-6),
			expectedCacheRead: floatPtr(0.14e-6),
		},
		// 关键：k2.6 / k2.5 / k2-thinking 必须先于 k2 匹配
		{
			name:              "kimi k2.6 vs k2 ordering",
			model:             "kimi-k2.6",
			expectedInput:     0.95e-6, // = k2.6 不是 k2 的 0.56e-6
			expectedOutput:    floatPtr(4e-6),
			expectedCacheRead: floatPtr(0.15e-6),
		},
		{
			name:              "kimi k2 thinking hyphenated variant",
			model:             "kimi-k2-thinking-preview",
			expectedInput:     0.56e-6,
			expectedOutput:    floatPtr(2.24e-6),
			expectedCacheRead: floatPtr(0.14e-6),
		},

		// ---- MiniMax M 系列 ----
		{
			name:              "minimax m3",
			model:             "minimax-m3",
			expectedInput:     0.60e-6,
			expectedOutput:    floatPtr(2.40e-6),
			expectedCacheRead: floatPtr(0.12e-6),
		},
		{
			name:              "minimax m3 long ctx boundary keep standard tier",
			model:             "minimax-m3-long", // 仍按 standard tier (≤512K)
			expectedInput:     0.60e-6,
			expectedOutput:    floatPtr(2.40e-6),
			expectedCacheRead: floatPtr(0.12e-6),
		},
		{
			name:              "minimax m2.7",
			model:             "minimax-m2.7",
			expectedInput:     0.30e-6,
			expectedOutput:    floatPtr(1.20e-6),
			expectedCacheRead: floatPtr(0.06e-6),
		},
		{
			name:              "minimax m2.7 highspeed",
			model:             "minimax-m2.7-highspeed",
			expectedInput:     0.60e-6,
			expectedOutput:    floatPtr(2.40e-6),
			expectedCacheRead: floatPtr(0.06e-6),
		},
		{
			name:              "minimax m2.5",
			model:             "minimax-m2.5",
			expectedInput:     0.30e-6,
			expectedOutput:    floatPtr(1.20e-6),
			expectedCacheRead: floatPtr(0.03e-6),
		},
		{
			name:              "minimax m2 legacy",
			model:             "minimax-m2",
			expectedInput:     0.30e-6,
			expectedOutput:    floatPtr(1.20e-6),
			expectedCacheRead: floatPtr(0.03e-6),
		},

		// ---- 火山方舟 豆包 Embedding（多模态向量化）----
		{
			name:           "doubao embedding vision text rate",
			model:          "doubao-embedding-vision",
			expectedInput:  0.098e-6,
			expectedOutput: floatPtr(0),
		},
		{
			name:          "doubao embedding vision versioned alias",
			model:         "doubao-embedding-vision-251215",
			expectedInput: 0.098e-6,
		},

		// ---- 负向用例 ----
		{name: "qwen unknown no fallback", model: "qwen-max", expectNilPricing: true},
		// doubao-pro / doubao-embedding（纯文本）不在白名单，不回退；仅 doubao-embedding-vision 显式命中。
		{name: "doubao unknown no fallback", model: "doubao-pro", expectNilPricing: true},
		{name: "doubao text embedding no fallback", model: "doubao-embedding-text-240515", expectNilPricing: true},
		{name: "hunyuan unknown no fallback", model: "hunyuan-t1", expectNilPricing: true},
		{name: "moonshot v1 not covered", model: "moonshot-v1-8k", expectNilPricing: true},
		// kimi-k2-0905 / kimi-k2-0711 官方未公布独立价，走 kimi-k2 隐性回退（接受）——
		// 如未来官方公布独立价，需在 getFallbackPricing 加显式分支。
		{
			name:              "kimi k2-0905-preview implicit fallback to k2",
			model:             "kimi-k2-0905-preview",
			expectedInput:     0.56e-6,
			expectedOutput:    floatPtr(2.24e-6),
			expectedCacheRead: floatPtr(0.14e-6),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pricing := svc.getFallbackPricing(tt.model)
			if tt.expectNilPricing {
				require.Nil(t, pricing)
				return
			}
			require.NotNil(t, pricing)
			require.InDelta(t, tt.expectedInput, pricing.InputPricePerToken, 1e-12)
			if tt.expectedOutput != nil {
				require.InDelta(t, *tt.expectedOutput, pricing.OutputPricePerToken, 1e-12,
					"OutputPricePerToken mismatch for %s", tt.model)
			}
			if tt.expectedCacheRead != nil {
				require.InDelta(t, *tt.expectedCacheRead, pricing.CacheReadPricePerToken, 1e-14,
					"CacheReadPricePerToken mismatch for %s", tt.model)
			}
		})
	}
}

// doubao-embedding-vision 是首个图文不同价的 embedding：文本 ¥0.7/MTok、图片 ¥1.8/MTok。
// 验证回退表同时携带文本与图片两档单价，且能被带版本后缀 / 大小写别名命中。
func TestGetModelPricing_DoubaoEmbeddingVisionImageInputRate(t *testing.T) {
	svc := newTestBillingService()

	for _, model := range []string{
		"doubao-embedding-vision",
		"doubao-embedding-vision-251215",
		"Doubao-Embedding-Vision",
	} {
		pricing, err := svc.GetModelPricing(model)
		require.NoError(t, err, "model %s should resolve fallback pricing", model)
		require.NotNil(t, pricing)
		require.InDelta(t, 0.098e-6, pricing.InputPricePerToken, 1e-12, "text input rate for %s", model)
		require.InDelta(t, 0.252e-6, pricing.ImageInputPricePerToken, 1e-12, "image input rate for %s", model)
		require.Zero(t, pricing.OutputPricePerToken, "embedding has no output cost for %s", model)
	}
}

// 验证双档计费：InputCost = 文本token×文本价 + 图片token×图片价；
// 且 ImageInputTokens=0 时走原单价路径，ImageInputTokens>InputTokens 时不负计文本。
func TestCalculateCost_DoubaoEmbeddingVisionDifferentialInput(t *testing.T) {
	svc := newTestBillingService()

	// 图文混合：prompt_tokens=1340，其中 image_tokens=28、text_tokens=1312。
	mixed := UsageTokens{InputTokens: 1340, ImageInputTokens: 28}
	cost, err := svc.CalculateCost("doubao-embedding-vision", mixed, 1.0)
	require.NoError(t, err)
	wantMixed := float64(1312)*0.098e-6 + float64(28)*0.252e-6
	require.InDelta(t, wantMixed, cost.InputCost, 1e-15)
	require.InDelta(t, wantMixed, cost.TotalCost, 1e-15)
	require.Zero(t, cost.OutputCost)

	// 纯文本：全部按文本档计费，与原单价路径一致。
	textOnly := UsageTokens{InputTokens: 1340}
	costText, err := svc.CalculateCost("doubao-embedding-vision", textOnly, 1.0)
	require.NoError(t, err)
	require.InDelta(t, float64(1340)*0.098e-6, costText.InputCost, 1e-15)

	// 健壮性：ImageInputTokens 超过 InputTokens 时，文本置 0、计费 token 不超过 InputTokens。
	weird := UsageTokens{InputTokens: 10, ImageInputTokens: 50}
	costWeird, err := svc.CalculateCost("doubao-embedding-vision", weird, 1.0)
	require.NoError(t, err)
	require.InDelta(t, float64(10)*0.252e-6, costWeird.InputCost, 1e-15)
}
func TestCalculateCostWithLongContext_BelowThreshold(t *testing.T) {
	svc := newTestBillingService()

	tokens := UsageTokens{
		InputTokens:     50000,
		OutputTokens:    1000,
		CacheReadTokens: 100000,
	}
	// 总输入 150k < 200k 阈值，应走正常计费
	cost, err := svc.CalculateCostWithLongContext("claude-sonnet-4", tokens, 1.0, 200000, 2.0)
	require.NoError(t, err)

	normalCost, err := svc.CalculateCost("claude-sonnet-4", tokens, 1.0)
	require.NoError(t, err)

	require.InDelta(t, normalCost.ActualCost, cost.ActualCost, 1e-10)
}

func TestCalculateCostWithLongContext_AboveThreshold_CacheExceedsThreshold(t *testing.T) {
	svc := newTestBillingService()

	// 缓存 210k + 输入 10k = 220k > 200k 阈值
	// 缓存已超阈值：范围内 200k 缓存，范围外 10k 缓存 + 10k 输入
	tokens := UsageTokens{
		InputTokens:     10000,
		OutputTokens:    1000,
		CacheReadTokens: 210000,
	}
	cost, err := svc.CalculateCostWithLongContext("claude-sonnet-4", tokens, 1.0, 200000, 2.0)
	require.NoError(t, err)

	// 范围内：200k cache + 0 input + 1k output
	inRange, _ := svc.CalculateCost("claude-sonnet-4", UsageTokens{
		InputTokens:     0,
		OutputTokens:    1000,
		CacheReadTokens: 200000,
	}, 1.0)

	// 范围外：10k cache + 10k input，倍率 2.0
	outRange, _ := svc.CalculateCost("claude-sonnet-4", UsageTokens{
		InputTokens:     10000,
		CacheReadTokens: 10000,
	}, 2.0)

	require.InDelta(t, inRange.ActualCost+outRange.ActualCost, cost.ActualCost, 1e-10)
}

func TestCalculateCostWithLongContext_AboveThreshold_CacheBelowThreshold(t *testing.T) {
	svc := newTestBillingService()

	// 缓存 100k + 输入 150k = 250k > 200k 阈值
	// 缓存未超阈值：范围内 100k 缓存 + 100k 输入，范围外 50k 输入
	tokens := UsageTokens{
		InputTokens:     150000,
		OutputTokens:    1000,
		CacheReadTokens: 100000,
	}
	cost, err := svc.CalculateCostWithLongContext("claude-sonnet-4", tokens, 1.0, 200000, 2.0)
	require.NoError(t, err)

	require.True(t, cost.ActualCost > 0, "费用应大于 0")

	// 正常费用不含长上下文
	normalCost, _ := svc.CalculateCost("claude-sonnet-4", tokens, 1.0)
	require.True(t, cost.ActualCost > normalCost.ActualCost, "长上下文费用应高于正常费用")
}

func TestCalculateCostWithLongContext_DisabledThreshold(t *testing.T) {
	svc := newTestBillingService()

	tokens := UsageTokens{InputTokens: 300000, CacheReadTokens: 0}

	// threshold <= 0 应禁用长上下文计费
	cost1, err := svc.CalculateCostWithLongContext("claude-sonnet-4", tokens, 1.0, 0, 2.0)
	require.NoError(t, err)

	cost2, err := svc.CalculateCost("claude-sonnet-4", tokens, 1.0)
	require.NoError(t, err)

	require.InDelta(t, cost2.ActualCost, cost1.ActualCost, 1e-10)
}

func TestCalculateCostWithLongContext_ExtraMultiplierLessEqualOne(t *testing.T) {
	svc := newTestBillingService()

	tokens := UsageTokens{InputTokens: 300000}

	// extraMultiplier <= 1 应禁用长上下文计费
	cost, err := svc.CalculateCostWithLongContext("claude-sonnet-4", tokens, 1.0, 200000, 1.0)
	require.NoError(t, err)

	normalCost, err := svc.CalculateCost("claude-sonnet-4", tokens, 1.0)
	require.NoError(t, err)

	require.InDelta(t, normalCost.ActualCost, cost.ActualCost, 1e-10)
}

func TestCalculateImageCost(t *testing.T) {
	svc := newTestBillingService()

	price := 0.134
	cfg := &ImagePriceConfig{Price1K: &price}
	cost := svc.CalculateImageCost("gpt-image-1", "1K", 3, cfg, 1.0)

	require.InDelta(t, 0.134*3, cost.TotalCost, 1e-10)
	require.InDelta(t, 0.134*3, cost.ActualCost, 1e-10)
}

func TestCalculateVideoCostUsesSeparateConfig(t *testing.T) {
	svc := newTestBillingService()

	imagePrice := 0.4
	videoPrice := 0.08
	imageCost := svc.CalculateImageCost("grok-imagine-video", "2K", 1, &ImagePriceConfig{Price2K: &imagePrice}, 1.0)
	videoCost := svc.CalculateVideoCost("grok-imagine-video", "480p", 1, 10, &VideoPriceConfig{Price480P: &videoPrice}, 0.5)

	require.InDelta(t, 0.4, imageCost.TotalCost, 1e-10)
	require.InDelta(t, 0.8, videoCost.TotalCost, 1e-10)
	require.InDelta(t, 0.4, videoCost.ActualCost, 1e-10)
	require.Equal(t, string(BillingModeVideo), videoCost.BillingMode)
}

func TestCalculateVideoCostBillsPerSecond(t *testing.T) {
	svc := newTestBillingService()

	oneSecond := svc.CalculateVideoCost("grok-imagine-video", "720p", 1, 1, nil, 1.0)
	fifteenSeconds := svc.CalculateVideoCost("grok-imagine-video", "720p", 1, 15, nil, 1.0)
	// duration <=0 时按上游默认 8 秒计费，超出上限按 15 秒收敛。
	defaultDuration := svc.CalculateVideoCost("grok-imagine-video", "720p", 1, 0, nil, 1.0)
	clampedDuration := svc.CalculateVideoCost("grok-imagine-video", "720p", 1, 999, nil, 1.0)

	require.InDelta(t, 0.07, oneSecond.TotalCost, 1e-10)
	require.InDelta(t, 0.07*15, fifteenSeconds.TotalCost, 1e-10)
	require.InDelta(t, 0.07*8, defaultDuration.TotalCost, 1e-10)
	require.InDelta(t, 0.07*15, clampedDuration.TotalCost, 1e-10)
}

func TestCalculateGrokImagineImageCostUsesDefaultRateCard(t *testing.T) {
	svc := newTestBillingService()

	standard1K := svc.CalculateImageCost("grok-imagine-image", "1K", 1, nil, 1.0)
	standard2K := svc.CalculateImageCost("grok-imagine-image", "2K", 1, nil, 1.0)
	quality1K := svc.CalculateImageCost("grok-imagine-image-quality", "1K", 1, nil, 1.0)
	quality2K := svc.CalculateImageCost("grok-imagine-image-quality", "2K", 1, nil, 1.0)

	require.InDelta(t, 0.02, standard1K.TotalCost, 1e-10)
	require.InDelta(t, 0.02, standard2K.TotalCost, 1e-10)
	require.InDelta(t, 0.05, quality1K.TotalCost, 1e-10)
	require.InDelta(t, 0.07, quality2K.TotalCost, 1e-10)
}

func TestCalculateGrokImagineVideoCostUsesDefaultRateCard(t *testing.T) {
	svc := newTestBillingService()

	// 默认价目为 xAI 官方每秒价格，按 1 秒时长验证每秒单价。
	standard480P := svc.CalculateVideoCost("grok-imagine-video", "480p", 1, 1, nil, 1.0)
	standard720P := svc.CalculateVideoCost("grok-imagine-video", "720p", 1, 1, nil, 1.0)
	video15_480P := svc.CalculateVideoCost("grok-imagine-video-1.5", "480p", 1, 1, nil, 1.0)
	video15_720P := svc.CalculateVideoCost("grok-imagine-video-1.5", "720p", 1, 1, nil, 1.0)
	video15_1080P := svc.CalculateVideoCost("grok-imagine-video-1.5", "1080p", 1, 1, nil, 1.0)

	require.InDelta(t, 0.05, standard480P.TotalCost, 1e-10)
	require.InDelta(t, 0.07, standard720P.TotalCost, 1e-10)
	require.InDelta(t, 0.08, video15_480P.TotalCost, 1e-10)
	require.InDelta(t, 0.14, video15_720P.TotalCost, 1e-10)
	require.InDelta(t, 0.25, video15_1080P.TotalCost, 1e-10)
}

func TestIsModelSupported(t *testing.T) {
	svc := newTestBillingService()

	require.True(t, svc.IsModelSupported("claude-sonnet-4"))
	require.True(t, svc.IsModelSupported("Claude-Opus-4.5"))
	require.True(t, svc.IsModelSupported("claude-3-haiku"))
	require.False(t, svc.IsModelSupported("gpt-4o"))
	require.False(t, svc.IsModelSupported("gemini-pro"))
}

func TestCalculateCost_ZeroTokens(t *testing.T) {
	svc := newTestBillingService()

	cost, err := svc.CalculateCost("claude-sonnet-4", UsageTokens{}, 1.0)
	require.NoError(t, err)
	require.Equal(t, 0.0, cost.TotalCost)
	require.Equal(t, 0.0, cost.ActualCost)
}

func TestCalculateCostWithConfig(t *testing.T) {
	cfg := &config.Config{}
	cfg.Default.RateMultiplier = 1.5
	svc := NewBillingService(cfg, nil)

	tokens := UsageTokens{InputTokens: 1000, OutputTokens: 500}
	cost, err := svc.CalculateCostWithConfig("claude-sonnet-4", tokens)
	require.NoError(t, err)

	expected, _ := svc.CalculateCost("claude-sonnet-4", tokens, 1.5)
	require.InDelta(t, expected.ActualCost, cost.ActualCost, 1e-10)
}

func TestCalculateCostWithConfig_ZeroMultiplier(t *testing.T) {
	cfg := &config.Config{}
	cfg.Default.RateMultiplier = 0
	svc := NewBillingService(cfg, nil)

	tokens := UsageTokens{InputTokens: 1000}
	cost, err := svc.CalculateCostWithConfig("claude-sonnet-4", tokens)
	require.NoError(t, err)

	// 倍率 <=0 时默认 1.0
	expected, _ := svc.CalculateCost("claude-sonnet-4", tokens, 1.0)
	require.InDelta(t, expected.ActualCost, cost.ActualCost, 1e-10)
}

func TestGetEstimatedCost(t *testing.T) {
	svc := newTestBillingService()

	est, err := svc.GetEstimatedCost("claude-sonnet-4", 1000, 500)
	require.NoError(t, err)
	require.True(t, est > 0)
}

func TestListSupportedModels(t *testing.T) {
	svc := newTestBillingService()

	models := svc.ListSupportedModels()
	require.NotEmpty(t, models)
	require.GreaterOrEqual(t, len(models), 6)
}

func TestGetPricingServiceStatus_NilService(t *testing.T) {
	svc := newTestBillingService()

	status := svc.GetPricingServiceStatus()
	require.NotNil(t, status)
	require.Equal(t, "using fallback", status["last_updated"])
}

func TestForceUpdatePricing_NilService(t *testing.T) {
	svc := newTestBillingService()

	err := svc.ForceUpdatePricing()
	require.Error(t, err)
	require.Contains(t, err.Error(), "not initialized")
}

func TestCalculateCostWithLongContext_PropagatesError(t *testing.T) {
	// 使用空的 fallback prices 让 GetModelPricing 失败
	svc := &BillingService{
		cfg:            &config.Config{},
		fallbackPrices: make(map[string]*ModelPricing),
	}

	tokens := UsageTokens{InputTokens: 300000, CacheReadTokens: 0}
	_, err := svc.CalculateCostWithLongContext("unknown-model", tokens, 1.0, 200000, 2.0)
	require.Error(t, err)
	require.Contains(t, err.Error(), "pricing not found")
}

func TestGetModelPricing_Grok45OfficialFallback(t *testing.T) {
	svc := newTestBillingService()

	for _, model := range []string{"grok", "grok-latest", "grok-4.5", "grok-4.5-latest", "grok-build-latest"} {
		model := model
		t.Run(model, func(t *testing.T) {
			pricing, err := svc.GetModelPricing(model)
			require.NoError(t, err)
			require.InDelta(t, 2e-6, pricing.InputPricePerToken, 1e-12)
			require.InDelta(t, 6e-6, pricing.OutputPricePerToken, 1e-12)
			require.InDelta(t, 0.5e-6, pricing.CacheReadPricePerToken, 1e-12)
			require.False(t, pricing.SupportsCacheBreakdown)
		})
	}
}

func TestGetModelPricing_GrokCatalogFallbacks(t *testing.T) {
	svc := newTestBillingService()

	tests := []struct {
		name      string
		models    []string
		input     float64
		cacheRead float64
		output    float64
	}{
		{
			name: "Grok 4.3 family",
			models: []string{
				"grok-4.3",
				"grok-4.20-0309-reasoning",
				"grok-4.20-0309-non-reasoning",
				"grok-4.20-multi-agent-0309",
				"grok-4.20-reasoning",
				"grok-4.20-non-reasoning",
			},
			input:     1.25e-6,
			cacheRead: 0.2e-6,
			output:    2.5e-6,
		},
		{
			name: "Grok coding and Composer family",
			models: []string{
				"grok-build",
				"grok-build-0.1",
				"grok-composer",
				"grok-composer-2.5-fast",
				"composer-2.5",
			},
			input:     1e-6,
			cacheRead: 0.2e-6,
			output:    2e-6,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for _, model := range tt.models {
				pricing, err := svc.GetModelPricing(model)
				require.NoError(t, err, "model %s", model)
				require.InDelta(t, tt.input, pricing.InputPricePerToken, 1e-12, "model %s input", model)
				require.InDelta(t, tt.cacheRead, pricing.CacheReadPricePerToken, 1e-12, "model %s cached input", model)
				require.InDelta(t, tt.output, pricing.OutputPricePerToken, 1e-12, "model %s output", model)
			}
		})
	}
}

func TestCalculateCost_SupportsCacheBreakdown(t *testing.T) {
	svc := &BillingService{
		cfg: &config.Config{},
		fallbackPrices: map[string]*ModelPricing{
			"claude-sonnet-4": {
				InputPricePerToken:     3e-6,
				OutputPricePerToken:    15e-6,
				SupportsCacheBreakdown: true,
				CacheCreation5mPrice:   4e-6, // per token
				CacheCreation1hPrice:   5e-6, // per token
			},
		},
	}

	tokens := UsageTokens{
		InputTokens:           1000,
		OutputTokens:          500,
		CacheCreation5mTokens: 100000,
		CacheCreation1hTokens: 50000,
	}
	cost, err := svc.CalculateCost("claude-sonnet-4", tokens, 1.0)
	require.NoError(t, err)

	expected5m := float64(tokens.CacheCreation5mTokens) * 4e-6
	expected1h := float64(tokens.CacheCreation1hTokens) * 5e-6
	require.InDelta(t, expected5m+expected1h, cost.CacheCreationCost, 1e-10)
}

func TestCalculateCost_LargeTokenCount(t *testing.T) {
	svc := newTestBillingService()

	tokens := UsageTokens{
		InputTokens:  1_000_000,
		OutputTokens: 1_000_000,
	}
	cost, err := svc.CalculateCost("claude-sonnet-4", tokens, 1.0)
	require.NoError(t, err)

	// Input: 1M * 3e-6 = $3, Output: 1M * 15e-6 = $15
	require.InDelta(t, 3.0, cost.InputCost, 1e-6)
	require.InDelta(t, 15.0, cost.OutputCost, 1e-6)
	require.False(t, math.IsNaN(cost.TotalCost))
	require.False(t, math.IsInf(cost.TotalCost, 0))
}

func TestServiceTierCostMultiplier(t *testing.T) {
	require.InDelta(t, 2.0, serviceTierCostMultiplier("priority"), 1e-12)
	require.InDelta(t, 2.0, serviceTierCostMultiplier(" Priority "), 1e-12)
	require.InDelta(t, 0.5, serviceTierCostMultiplier("flex"), 1e-12)
	require.InDelta(t, 1.0, serviceTierCostMultiplier(""), 1e-12)
	require.InDelta(t, 1.0, serviceTierCostMultiplier("default"), 1e-12)
}

func TestCalculateCostWithServiceTier_OpenAIPriorityUsesPriorityPricing(t *testing.T) {
	svc := newTestBillingService()
	tokens := UsageTokens{InputTokens: 100, OutputTokens: 50, CacheReadTokens: 20}

	baseCost, err := svc.CalculateCost("gpt-5.1-codex", tokens, 1.0)
	require.NoError(t, err)

	priorityCost, err := svc.CalculateCostWithServiceTier("gpt-5.1-codex", tokens, 1.0, "priority")
	require.NoError(t, err)

	require.InDelta(t, baseCost.InputCost*2, priorityCost.InputCost, 1e-10)
	require.InDelta(t, baseCost.OutputCost*2, priorityCost.OutputCost, 1e-10)
	require.InDelta(t, baseCost.CacheReadCost*2, priorityCost.CacheReadCost, 1e-10)
	require.InDelta(t, baseCost.TotalCost*2, priorityCost.TotalCost, 1e-10)
}

func TestCalculateCostWithServiceTier_FlexAppliesHalfMultiplier(t *testing.T) {
	svc := newTestBillingService()
	tokens := UsageTokens{InputTokens: 100, OutputTokens: 50, CacheCreationTokens: 40, CacheReadTokens: 20}

	baseCost, err := svc.CalculateCost("gpt-5.4", tokens, 1.0)
	require.NoError(t, err)

	flexCost, err := svc.CalculateCostWithServiceTier("gpt-5.4", tokens, 1.0, "flex")
	require.NoError(t, err)

	require.InDelta(t, baseCost.InputCost*0.5, flexCost.InputCost, 1e-10)
	require.InDelta(t, baseCost.OutputCost*0.5, flexCost.OutputCost, 1e-10)
	require.InDelta(t, baseCost.CacheCreationCost*0.5, flexCost.CacheCreationCost, 1e-10)
	require.InDelta(t, baseCost.CacheReadCost*0.5, flexCost.CacheReadCost, 1e-10)
	require.InDelta(t, baseCost.TotalCost*0.5, flexCost.TotalCost, 1e-10)
}

func TestCalculateCostWithServiceTier_Gpt54MiniPriorityFallsBackToTierMultiplier(t *testing.T) {
	svc := newTestBillingService()
	tokens := UsageTokens{InputTokens: 120, OutputTokens: 30, CacheCreationTokens: 12, CacheReadTokens: 8}

	baseCost, err := svc.CalculateCost("gpt-5.4-mini", tokens, 1.0)
	require.NoError(t, err)

	priorityCost, err := svc.CalculateCostWithServiceTier("gpt-5.4-mini", tokens, 1.0, "priority")
	require.NoError(t, err)

	require.InDelta(t, baseCost.InputCost*2, priorityCost.InputCost, 1e-10)
	require.InDelta(t, baseCost.OutputCost*2, priorityCost.OutputCost, 1e-10)
	require.InDelta(t, baseCost.CacheCreationCost*2, priorityCost.CacheCreationCost, 1e-10)
	require.InDelta(t, baseCost.CacheReadCost*2, priorityCost.CacheReadCost, 1e-10)
	require.InDelta(t, baseCost.TotalCost*2, priorityCost.TotalCost, 1e-10)
}

func TestCalculateCostWithServiceTier_Gpt54NanoFlexAppliesHalfMultiplier(t *testing.T) {
	svc := newTestBillingService()
	tokens := UsageTokens{InputTokens: 100, OutputTokens: 50, CacheCreationTokens: 40, CacheReadTokens: 20}

	baseCost, err := svc.CalculateCost("gpt-5.4-nano", tokens, 1.0)
	require.NoError(t, err)

	flexCost, err := svc.CalculateCostWithServiceTier("gpt-5.4-nano", tokens, 1.0, "flex")
	require.NoError(t, err)

	require.InDelta(t, baseCost.InputCost*0.5, flexCost.InputCost, 1e-10)
	require.InDelta(t, baseCost.OutputCost*0.5, flexCost.OutputCost, 1e-10)
	require.InDelta(t, baseCost.CacheCreationCost*0.5, flexCost.CacheCreationCost, 1e-10)
	require.InDelta(t, baseCost.CacheReadCost*0.5, flexCost.CacheReadCost, 1e-10)
	require.InDelta(t, baseCost.TotalCost*0.5, flexCost.TotalCost, 1e-10)
}

func TestCalculateCostWithServiceTier_PriorityFallsBackToTierMultiplierWithoutExplicitPriorityPrice(t *testing.T) {
	svc := newTestBillingService()
	tokens := UsageTokens{InputTokens: 120, OutputTokens: 30, CacheCreationTokens: 12, CacheReadTokens: 8}

	baseCost, err := svc.CalculateCost("claude-sonnet-4", tokens, 1.0)
	require.NoError(t, err)

	priorityCost, err := svc.CalculateCostWithServiceTier("claude-sonnet-4", tokens, 1.0, "priority")
	require.NoError(t, err)

	require.InDelta(t, baseCost.InputCost*2, priorityCost.InputCost, 1e-10)
	require.InDelta(t, baseCost.OutputCost*2, priorityCost.OutputCost, 1e-10)
	require.InDelta(t, baseCost.CacheCreationCost*2, priorityCost.CacheCreationCost, 1e-10)
	require.InDelta(t, baseCost.CacheReadCost*2, priorityCost.CacheReadCost, 1e-10)
	require.InDelta(t, baseCost.TotalCost*2, priorityCost.TotalCost, 1e-10)
}

func TestBillingServiceGetModelPricing_UsesDynamicPriorityFields(t *testing.T) {
	pricingSvc := &PricingService{
		pricingData: map[string]*LiteLLMModelPricing{
			"gpt-5.4": {
				InputCostPerToken:               2.5e-6,
				InputCostPerTokenPriority:       5e-6,
				OutputCostPerToken:              15e-6,
				OutputCostPerTokenPriority:      30e-6,
				CacheCreationInputTokenCost:     2.5e-6,
				CacheReadInputTokenCost:         0.25e-6,
				CacheReadInputTokenCostPriority: 0.5e-6,
				LongContextInputTokenThreshold:  272000,
				LongContextInputCostMultiplier:  2.0,
				LongContextOutputCostMultiplier: 1.5,
			},
		},
	}
	svc := NewBillingService(&config.Config{}, pricingSvc)

	pricing, err := svc.GetModelPricing("gpt-5.4")
	require.NoError(t, err)
	require.InDelta(t, 2.5e-6, pricing.InputPricePerToken, 1e-12)
	require.InDelta(t, 5e-6, pricing.InputPricePerTokenPriority, 1e-12)
	require.InDelta(t, 15e-6, pricing.OutputPricePerToken, 1e-12)
	require.InDelta(t, 30e-6, pricing.OutputPricePerTokenPriority, 1e-12)
	require.InDelta(t, 0.25e-6, pricing.CacheReadPricePerToken, 1e-12)
	require.InDelta(t, 0.5e-6, pricing.CacheReadPricePerTokenPriority, 1e-12)
	require.Equal(t, 272000, pricing.LongContextInputThreshold)
	require.InDelta(t, 2.0, pricing.LongContextInputMultiplier, 1e-12)
	require.InDelta(t, 1.5, pricing.LongContextOutputMultiplier, 1e-12)
}

func TestBillingServiceGetModelPricing_OpenAIFallbackGpt52Variants(t *testing.T) {
	svc := newTestBillingService()

	gpt52, err := svc.GetModelPricing("gpt-5.2")
	require.NoError(t, err)
	require.NotNil(t, gpt52)
	require.InDelta(t, 1.75e-6, gpt52.InputPricePerToken, 1e-12)
	require.InDelta(t, 3.5e-6, gpt52.InputPricePerTokenPriority, 1e-12)

	gpt52Codex, err := svc.GetModelPricing("gpt-5.2-codex")
	require.NoError(t, err)
	require.NotNil(t, gpt52Codex)
	require.InDelta(t, 1.75e-6, gpt52Codex.InputPricePerToken, 1e-12)
	require.InDelta(t, 3.5e-6, gpt52Codex.InputPricePerTokenPriority, 1e-12)
	require.InDelta(t, 28e-6, gpt52Codex.OutputPricePerTokenPriority, 1e-12)
}

func TestCalculateCostWithServiceTier_PriorityFallsBackToTierMultiplierWhenExplicitPriceMissing(t *testing.T) {
	svc := NewBillingService(&config.Config{}, &PricingService{
		pricingData: map[string]*LiteLLMModelPricing{
			"custom-no-priority": {
				InputCostPerToken:           1e-6,
				OutputCostPerToken:          2e-6,
				CacheCreationInputTokenCost: 0.5e-6,
				CacheReadInputTokenCost:     0.25e-6,
			},
		},
	})
	tokens := UsageTokens{InputTokens: 100, OutputTokens: 50, CacheCreationTokens: 40, CacheReadTokens: 20}

	baseCost, err := svc.CalculateCost("custom-no-priority", tokens, 1.0)
	require.NoError(t, err)

	priorityCost, err := svc.CalculateCostWithServiceTier("custom-no-priority", tokens, 1.0, "priority")
	require.NoError(t, err)

	require.InDelta(t, baseCost.InputCost*2, priorityCost.InputCost, 1e-10)
	require.InDelta(t, baseCost.OutputCost*2, priorityCost.OutputCost, 1e-10)
	require.InDelta(t, baseCost.CacheCreationCost*2, priorityCost.CacheCreationCost, 1e-10)
	require.InDelta(t, baseCost.CacheReadCost*2, priorityCost.CacheReadCost, 1e-10)
	require.InDelta(t, baseCost.TotalCost*2, priorityCost.TotalCost, 1e-10)
}

func TestGetModelPricing_OpenAIGpt52FallbacksExposePriorityPrices(t *testing.T) {
	svc := newTestBillingService()

	gpt52, err := svc.GetModelPricing("gpt-5.2")
	require.NoError(t, err)
	require.InDelta(t, 1.75e-6, gpt52.InputPricePerToken, 1e-12)
	require.InDelta(t, 3.5e-6, gpt52.InputPricePerTokenPriority, 1e-12)
	require.InDelta(t, 14e-6, gpt52.OutputPricePerToken, 1e-12)
	require.InDelta(t, 28e-6, gpt52.OutputPricePerTokenPriority, 1e-12)

	gpt52Codex, err := svc.GetModelPricing("gpt-5.2-codex")
	require.NoError(t, err)
	require.InDelta(t, 1.75e-6, gpt52Codex.InputPricePerToken, 1e-12)
	require.InDelta(t, 3.5e-6, gpt52Codex.InputPricePerTokenPriority, 1e-12)
	require.InDelta(t, 14e-6, gpt52Codex.OutputPricePerToken, 1e-12)
	require.InDelta(t, 28e-6, gpt52Codex.OutputPricePerTokenPriority, 1e-12)
}

func TestGetModelPricing_MapsDynamicPriorityFieldsIntoBillingPricing(t *testing.T) {
	svc := NewBillingService(&config.Config{}, &PricingService{
		pricingData: map[string]*LiteLLMModelPricing{
			"dynamic-tier-model": {
				InputCostPerToken:                   1e-6,
				InputCostPerTokenPriority:           2e-6,
				OutputCostPerToken:                  3e-6,
				OutputCostPerTokenPriority:          6e-6,
				CacheCreationInputTokenCost:         4e-6,
				CacheCreationInputTokenCostAbove1hr: 5e-6,
				CacheReadInputTokenCost:             7e-7,
				CacheReadInputTokenCostPriority:     8e-7,
				LongContextInputTokenThreshold:      999,
				LongContextInputCostMultiplier:      1.5,
				LongContextOutputCostMultiplier:     1.25,
			},
		},
	})

	pricing, err := svc.GetModelPricing("dynamic-tier-model")
	require.NoError(t, err)
	require.InDelta(t, 1e-6, pricing.InputPricePerToken, 1e-12)
	require.InDelta(t, 2e-6, pricing.InputPricePerTokenPriority, 1e-12)
	require.InDelta(t, 3e-6, pricing.OutputPricePerToken, 1e-12)
	require.InDelta(t, 6e-6, pricing.OutputPricePerTokenPriority, 1e-12)
	require.InDelta(t, 4e-6, pricing.CacheCreation5mPrice, 1e-12)
	require.InDelta(t, 5e-6, pricing.CacheCreation1hPrice, 1e-12)
	require.True(t, pricing.SupportsCacheBreakdown)
	require.InDelta(t, 7e-7, pricing.CacheReadPricePerToken, 1e-12)
	require.InDelta(t, 8e-7, pricing.CacheReadPricePerTokenPriority, 1e-12)
	require.Equal(t, 999, pricing.LongContextInputThreshold)
	require.InDelta(t, 1.5, pricing.LongContextInputMultiplier, 1e-12)
	require.InDelta(t, 1.25, pricing.LongContextOutputMultiplier, 1e-12)
}

// ---------------------------------------------------------------------------
// GetModelPricingWithChannel
// ---------------------------------------------------------------------------

func TestGetModelPricingWithChannel_NilChannelPricing_ReturnsOriginal(t *testing.T) {
	svc := newTestBillingService()

	pricing, err := svc.GetModelPricingWithChannel("claude-sonnet-4", nil)
	require.NoError(t, err)
	require.NotNil(t, pricing)

	// Should be identical to GetModelPricing
	original, err := svc.GetModelPricing("claude-sonnet-4")
	require.NoError(t, err)
	require.InDelta(t, original.InputPricePerToken, pricing.InputPricePerToken, 1e-12)
	require.InDelta(t, original.OutputPricePerToken, pricing.OutputPricePerToken, 1e-12)
	require.InDelta(t, original.CacheCreationPricePerToken, pricing.CacheCreationPricePerToken, 1e-12)
	require.InDelta(t, original.CacheReadPricePerToken, pricing.CacheReadPricePerToken, 1e-12)
}

func TestGetModelPricingWithChannel_OverrideInputPriceOnly(t *testing.T) {
	svc := newTestBillingService()

	chPricing := &ChannelModelPricing{
		InputPrice: testPtrFloat64(99e-6),
	}
	pricing, err := svc.GetModelPricingWithChannel("claude-sonnet-4", chPricing)
	require.NoError(t, err)

	// InputPrice overridden (both normal and priority)
	require.InDelta(t, 99e-6, pricing.InputPricePerToken, 1e-12)
	require.InDelta(t, 99e-6, pricing.InputPricePerTokenPriority, 1e-12)

	// OutputPrice unchanged (claude-sonnet-4 fallback = 15e-6)
	require.InDelta(t, 15e-6, pricing.OutputPricePerToken, 1e-12)
}

func TestGetModelPricingWithChannel_OverrideOutputPriceOnly(t *testing.T) {
	svc := newTestBillingService()

	chPricing := &ChannelModelPricing{
		OutputPrice: testPtrFloat64(88e-6),
	}
	pricing, err := svc.GetModelPricingWithChannel("claude-sonnet-4", chPricing)
	require.NoError(t, err)

	// OutputPrice overridden
	require.InDelta(t, 88e-6, pricing.OutputPricePerToken, 1e-12)
	require.InDelta(t, 88e-6, pricing.OutputPricePerTokenPriority, 1e-12)

	// InputPrice unchanged (claude-sonnet-4 fallback = 3e-6)
	require.InDelta(t, 3e-6, pricing.InputPricePerToken, 1e-12)
}

func TestGetModelPricingWithChannel_OverrideAllFields(t *testing.T) {
	svc := newTestBillingService()

	chPricing := &ChannelModelPricing{
		InputPrice:       testPtrFloat64(10e-6),
		OutputPrice:      testPtrFloat64(20e-6),
		CacheWritePrice:  testPtrFloat64(5e-6),
		CacheReadPrice:   testPtrFloat64(1e-6),
		ImageOutputPrice: testPtrFloat64(50e-6),
	}
	pricing, err := svc.GetModelPricingWithChannel("claude-sonnet-4", chPricing)
	require.NoError(t, err)

	require.InDelta(t, 10e-6, pricing.InputPricePerToken, 1e-12)
	require.InDelta(t, 10e-6, pricing.InputPricePerTokenPriority, 1e-12)
	require.InDelta(t, 20e-6, pricing.OutputPricePerToken, 1e-12)
	require.InDelta(t, 20e-6, pricing.OutputPricePerTokenPriority, 1e-12)
	require.InDelta(t, 5e-6, pricing.CacheCreationPricePerToken, 1e-12)
	require.InDelta(t, 5e-6, pricing.CacheCreation5mPrice, 1e-12)
	require.InDelta(t, 5e-6, pricing.CacheCreation1hPrice, 1e-12)
	require.InDelta(t, 1e-6, pricing.CacheReadPricePerToken, 1e-12)
	require.InDelta(t, 1e-6, pricing.CacheReadPricePerTokenPriority, 1e-12)
	require.InDelta(t, 50e-6, pricing.ImageOutputPricePerToken, 1e-12)
}

func TestGetModelPricingWithChannel_CacheWritePriceAffects5mAnd1h(t *testing.T) {
	svc := newTestBillingService()

	chPricing := &ChannelModelPricing{
		CacheWritePrice: testPtrFloat64(7e-6),
	}
	pricing, err := svc.GetModelPricingWithChannel("claude-sonnet-4", chPricing)
	require.NoError(t, err)

	// CacheWritePrice should set all three: CacheCreationPricePerToken, 5m, and 1h
	require.InDelta(t, 7e-6, pricing.CacheCreationPricePerToken, 1e-12)
	require.InDelta(t, 7e-6, pricing.CacheCreation5mPrice, 1e-12)
	require.InDelta(t, 7e-6, pricing.CacheCreation1hPrice, 1e-12)
}

func TestGetModelPricingWithChannel_CacheReadPriceAffectsPriority(t *testing.T) {
	svc := newTestBillingService()

	chPricing := &ChannelModelPricing{
		CacheReadPrice: testPtrFloat64(2e-6),
	}
	pricing, err := svc.GetModelPricingWithChannel("claude-sonnet-4", chPricing)
	require.NoError(t, err)

	// CacheReadPrice should set both normal and priority
	require.InDelta(t, 2e-6, pricing.CacheReadPricePerToken, 1e-12)
	require.InDelta(t, 2e-6, pricing.CacheReadPricePerTokenPriority, 1e-12)
}

func TestGetModelPricingWithChannel_UnknownModelReturnsError(t *testing.T) {
	svc := newTestBillingService()

	chPricing := &ChannelModelPricing{
		InputPrice: testPtrFloat64(1e-6),
	}
	pricing, err := svc.GetModelPricingWithChannel("totally-unknown-model", chPricing)
	require.Error(t, err)
	require.Nil(t, pricing)
	require.Contains(t, err.Error(), "pricing not found")
}

func TestGetModelPricingWithChannel_NilImageOutputPriceZerosAndMarksExplicit(t *testing.T) {
	svc := newTestBillingService()

	chPricing := &ChannelModelPricing{
		InputPrice:  testPtrFloat64(10e-6),
		OutputPrice: testPtrFloat64(20e-6),
		// ImageOutputPrice intentionally nil
	}
	pricing, err := svc.GetModelPricingWithChannel("claude-sonnet-4", chPricing)
	require.NoError(t, err)

	require.Equal(t, 0.0, pricing.ImageOutputPricePerToken)
	require.True(t, pricing.ImageOutputPriceExplicit)
}

func TestComputeTokenBreakdown_ExplicitZeroImagePrice_NoFallback(t *testing.T) {
	svc := newTestBillingService()

	pricing := &ModelPricing{
		InputPricePerToken:       3e-6,
		OutputPricePerToken:      15e-6,
		ImageOutputPricePerToken: 0,
		ImageOutputPriceExplicit: true,
	}
	tokens := UsageTokens{
		InputTokens:       100,
		OutputTokens:      200,
		ImageOutputTokens: 50,
	}
	bd := svc.computeTokenBreakdown(pricing, tokens, 1.0, "", false)

	// ImageOutputTokens should NOT fall back to outputPrice
	require.Equal(t, 0.0, bd.ImageOutputCost)
	// textOutputTokens = 200 - 50 = 150
	require.InDelta(t, 150*15e-6, bd.OutputCost, 1e-12)
}

func TestComputeTokenBreakdown_NonExplicitZeroImagePrice_FallsBackToOutput(t *testing.T) {
	svc := newTestBillingService()

	pricing := &ModelPricing{
		InputPricePerToken:       3e-6,
		OutputPricePerToken:      15e-6,
		ImageOutputPricePerToken: 0,
		ImageOutputPriceExplicit: false,
	}
	tokens := UsageTokens{
		InputTokens:       100,
		OutputTokens:      200,
		ImageOutputTokens: 50,
	}
	bd := svc.computeTokenBreakdown(pricing, tokens, 1.0, "", false)

	// Should fall back to outputPrice since not explicit
	require.InDelta(t, 50*15e-6, bd.ImageOutputCost, 1e-12)
	// textOutputTokens = 200 - 50 = 150
	require.InDelta(t, 150*15e-6, bd.OutputCost, 1e-12)
}
