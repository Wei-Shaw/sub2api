//go:build unit

package service

// 计费批 ⑧ 前半：Ollama Cloud 模型定价数据（B2-②）+ ":tag" 模型名两级查价
// fallback（B2-①）。
//
// 模型清单来源：curl -s https://ollama.com/v1/models（2026-09-12 实测 HTTP 200，
// 20 个模型，owned_by 全部为 ollama）。价格来源：https://ollama.com/pricing 官方
// 定价页（见 /tmp/ollama-pricing-data.md 换算表）。
//
// 命中路径钉死约定：
//   - "literal"：字面名直接命中（fallback 表精确键 + 专属前缀/子串规则）；
//   - "family"：既有家族规则命中（tag 透明包含于子串/前缀规则）；
//   - "tag-stripped"：字面 miss 后按剥 tag 名第二级命中（B2-① 新增路径）。
//
// bedrock 回归护栏：bedrock canonical key（us.anthropic.claude-sonnet-4-5-20250929-v1:0）
// 本身含 ":0"，字面名必须命中字面条目，剥 tag 只能是字面 miss 后的第二级。

import (
	"context"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

// ollamaPricingAt 固定为周一低谷（2026-08-24 12:00 UTC，DeepSeek 低谷时段），
// 使 applyModelSpecificPricingPolicyEx / deepseekPeakMultiplierAt 的输出确定。
var ollamaPricingAt = time.Date(2026, 8, 24, 12, 0, 0, 0, time.UTC)

// TestGetModelPricing_OllamaCloudRealModelList 覆盖 /v1/models 实测得到的全部
// 20 个模型名：断言可定价（非 nil）且输入/输出单价均 > 0，并按上面的约定钉死
// 每个名字的具体命中路径（期望值即该路径唯一解析到的价卡）。
func TestGetModelPricing_OllamaCloudRealModelList(t *testing.T) {
	bs := newTestBillingService()

	tests := []struct {
		model     string
		path      string
		wantInput float64 // USD per token（低谷/标准档）
		wantOut   float64
	}{
		// --- ollama 独占 8 款（B2-② 新增，字面命中） ---
		{"gpt-oss:120b", "literal", 0.15e-6, 0.60e-6},
		{"gpt-oss:20b", "literal", 0.07e-6, 0.30e-6},
		{"gemma4:31b", "literal", 0.14e-6, 0.40e-6},
		{"mistral-large-3:675b", "literal", 0.50e-6, 1.50e-6},
		{"qwen3.5:397b", "literal", 0.60e-6, 3.60e-6},
		{"nemotron-3-super", "literal", 0.015e-6, 0.60e-6},
		{"nemotron-3-nano:30b", "literal", 0.06e-6, 0.24e-6},
		{"nemotron-3-ultra", "literal", 0.10e-6, 3.00e-6},
		// --- 与别家同名、复用既有条目的 12 款（本批不改其数值） ---
		{"glm-5.1", "literal", 1.4e-6, 4.4e-6},
		{"glm-5.2", "literal", 1.4e-6, 4.4e-6},
		{"glm-5.3", "literal", 1.4e-6, 4.4e-6},
		{"glm-5.3-flash", "literal", 0.15e-6, 0.5e-6},
		{"kimi-k3", "literal", 3e-6, 15e-6},
		{"kimi-k2.6", "literal", 0.95e-6, 4e-6},
		// 已知误配：kimi-k2.7-code 经 Contains("kimi-k2") 落 kimi-k2 卡，
		// 与 Ollama 实收不符 —— 全局按名计费的独立议题，此处仅钉现状防漂移。
		{"kimi-k2.7-code", "family", 0.56e-6, 2.24e-6},
		{"minimax-m2.7", "literal", 0.30e-6, 1.20e-6},
		{"minimax-m3", "literal", 0.60e-6, 2.40e-6},
		{"deepseek-v4.1-flash", "family", 0.15e-6, 0.60e-6}, // HasPrefix("deepseek-") 兜底，恰好同价
		// 带 tag 名经既有子串规则命中（tag 透明包含，未剥 tag）。
		{"deepseek-v4-flash:0731", "family", 0.15e-6, 0.60e-6},
		{"deepseek-v4-pro:0813", "family", 0.66e-6, 1.98e-6},
		// --- 仓库测试里出现过的真实 "-cloud" 变体形态（前缀/子串规则一并覆盖） ---
		{"gpt-oss:120b-cloud", "literal", 0.15e-6, 0.60e-6},
		{"gemma4:31b-cloud", "literal", 0.14e-6, 0.40e-6},
		{"qwen3.5:397b-cloud", "literal", 0.60e-6, 3.60e-6},
	}

	for _, tt := range tests {
		t.Run(tt.model, func(t *testing.T) {
			require.NotEmpty(t, tt.path, "hit path must be pinned explicitly")

			// 固定时点：值级断言，钉死命中路径解析到的唯一价卡。
			pricing, err := bs.getModelPricingAt(tt.model, ollamaPricingAt)
			require.NoError(t, err, "model %s (path %s) must be priceable", tt.model, tt.path)
			require.NotNil(t, pricing)
			require.InDelta(t, tt.wantInput, pricing.InputPricePerToken, 1e-12,
				"model %s input price must match its pinned hit path (%s)", tt.model, tt.path)
			require.InDelta(t, tt.wantOut, pricing.OutputPricePerToken, 1e-12,
				"model %s output price must match its pinned hit path (%s)", tt.model, tt.path)

			// 公开入口（无显式时点）：验收口径 —— 非 nil 且输入/输出单价均 > 0。
			public, err := bs.GetModelPricing(tt.model)
			require.NoError(t, err)
			require.NotNil(t, public)
			require.Greater(t, public.InputPricePerToken, 0.0, "model %s", tt.model)
			require.Greater(t, public.OutputPricePerToken, 0.0, "model %s", tt.model)
		})
	}
}

// TestNormalizePricingModelNameForLookup 钉住剥 tag helper 的语义边界，并钉住
// normalizeChannelPricingModelName（纯 cache key 变换）不做任何 tag 预变换。
func TestNormalizePricingModelNameForLookup(t *testing.T) {
	tests := []struct {
		in       string
		want     string
		wantOK   bool
		wantSame string // normalizeChannelPricingModelName 的结果必须保持原样（cache key 不做变换）
	}{
		{"deepseek-v4-flash:0731", "deepseek-v4-flash", true, "deepseek-v4-flash:0731"},
		{"gpt-oss:120b-cloud", "gpt-oss", true, "gpt-oss:120b-cloud"},
		{"nemotron-3-super", "", false, "nemotron-3-super"},
		{"", "", false, ""},
		{":0731", "", false, ":0731"},   // ":" 在首位，剥后为空
		{"foo:", "", false, "foo:"},     // 尾随 ":"，无 tag 可剥
		{"a:b:c", "a:b", true, "a:b:c"}, // 只剥最后一个 ":" 后缀
		{"Foo:0731", "Foo", true, "foo:0731"},
	}
	for _, tt := range tests {
		got, ok := normalizePricingModelNameForLookup(tt.in)
		require.Equal(t, tt.wantOK, ok, "input %q", tt.in)
		require.Equal(t, tt.want, got, "input %q", tt.in)
		// cache key 归一化禁止接入剥 tag：结果必须保留 ":"（bedrock "…-v1:0" 不塌缩）。
		require.Equal(t, tt.wantSame, normalizeChannelPricingModelName(tt.in),
			"normalizeChannelPricingModelName must not transform tags (input %q)", tt.in)
	}
}

// TestGetModelPricing_LiteralTaggedKeyWinsOverStripped 字面优先：定价目录同时含
// "foo:0" 与 "foo" 时，请求 "foo:0" 必须命中 "foo:0" 的价（不得剥成 "foo"）。
func TestGetModelPricing_LiteralTaggedKeyWinsOverStripped(t *testing.T) {
	pricingSvc := newStubPricingServiceFromJSON(t, `{
		"foo:0": {"litellm_provider": "openai", "mode": "chat",
			"input_cost_per_token": 1e-06, "output_cost_per_token": 2e-06},
		"foo": {"litellm_provider": "openai", "mode": "chat",
			"input_cost_per_token": 3e-06, "output_cost_per_token": 4e-06}
	}`)
	bs := NewBillingService(&config.Config{}, pricingSvc)

	pricing, err := bs.getModelPricingAt("foo:0", ollamaPricingAt)
	require.NoError(t, err)
	require.InDelta(t, 1e-6, pricing.InputPricePerToken, 1e-12,
		"literal tagged key must win over the stripped name")
	require.InDelta(t, 2e-6, pricing.OutputPricePerToken, 1e-12)
}

// TestGetModelPricing_TagStrippedFallbackHitsBaseCard 字面 miss 且名含 tag 时，
// 按剥 tag 名完整重跑一次查找链并命中基名价卡（spec 的 qwen3-coder:0731 场景）。
func TestGetModelPricing_TagStrippedFallbackHitsBaseCard(t *testing.T) {
	pricingSvc := newStubPricingServiceFromJSON(t, `{
		"qwen3-coder": {"litellm_provider": "openai", "mode": "chat",
			"input_cost_per_token": 5e-07, "output_cost_per_token": 6e-06}
	}`)
	bs := NewBillingService(&config.Config{}, pricingSvc)

	pricing, err := bs.getModelPricingAt("qwen3-coder:0731", ollamaPricingAt)
	require.NoError(t, err, "tagged name must fall back to the stripped base name")
	require.InDelta(t, 5e-7, pricing.InputPricePerToken, 1e-12)
	require.InDelta(t, 6e-6, pricing.OutputPricePerToken, 1e-12)
}

// TestGetModelPricing_TagStrippedFallbackStillFailClosed 两个层级都 miss 时保持
// fail-closed（ErrModelPricingUnavailable），不得因剥 tag 引入静默零价。
func TestGetModelPricing_TagStrippedFallbackStillFailClosed(t *testing.T) {
	bs := newTestBillingService()

	_, err := bs.GetModelPricing("qwen3-coder:0731")
	require.ErrorIs(t, err, ErrModelPricingUnavailable)

	_, err = bs.GetModelPricing("nemotron-3-mini:9b") // 未列出的 nemotron 型号拒绝
	require.ErrorIs(t, err, ErrModelPricingUnavailable)
}

// TestGetModelPricing_BedrockCanonicalKeyDoubleEntry bedrock 回归：canonical key
// （含 ":0"）与剥后名双 key 并存时，各命中各自条目 —— 字面优先，不做破坏性剥离。
func TestGetModelPricing_BedrockCanonicalKeyDoubleEntry(t *testing.T) {
	pricingSvc := newStubPricingServiceFromJSON(t, `{
		"us.anthropic.claude-sonnet-4-5-20250929-v1:0": {"litellm_provider": "bedrock", "mode": "chat",
			"input_cost_per_token": 1e-06, "output_cost_per_token": 2e-06},
		"us.anthropic.claude-sonnet-4-5-20250929-v1": {"litellm_provider": "bedrock", "mode": "chat",
			"input_cost_per_token": 3e-06, "output_cost_per_token": 4e-06}
	}`)
	bs := NewBillingService(&config.Config{}, pricingSvc)

	tagged, err := bs.getModelPricingAt("us.anthropic.claude-sonnet-4-5-20250929-v1:0", ollamaPricingAt)
	require.NoError(t, err)
	require.InDelta(t, 1e-6, tagged.InputPricePerToken, 1e-12,
		"bedrock canonical key with ':0' must hit its own entry, never the stripped one")

	bare, err := bs.getModelPricingAt("us.anthropic.claude-sonnet-4-5-20250929-v1", ollamaPricingAt)
	require.NoError(t, err)
	require.InDelta(t, 3e-6, bare.InputPricePerToken, 1e-12,
		"bare bedrock name must hit its own entry")
}

// TestGetModelPricing_BedrockFallbackFamilyUnaffected 无动态目录时，bedrock
// canonical key 经既有 Claude 家族规则命中 sonnet-4 卡，剥 tag 改动不得影响该路径。
func TestGetModelPricing_BedrockFallbackFamilyUnaffected(t *testing.T) {
	bs := newTestBillingService()

	pricing, err := bs.getModelPricingAt("us.anthropic.claude-sonnet-4-5-20250929-v1:0", ollamaPricingAt)
	require.NoError(t, err)
	require.InDelta(t, 3e-6, pricing.InputPricePerToken, 1e-12) // claude-sonnet-4 卡
	require.InDelta(t, 15e-6, pricing.OutputPricePerToken, 1e-12)
}

// TestChannelPricing_TaggedModelFallsBackToStrippedKey 渠道价路径（B2-① 第 2 条）：
// 渠道只配基名、请求名带 tag 时经剥 tag 重试命中渠道价，而不是落官方兜底价。
func TestChannelPricing_TaggedModelFallsBackToStrippedKey(t *testing.T) {
	groupID := int64(888)
	cs := newChannelServiceWithPricings(groupID, []ChannelModelPricing{
		tokenPricingForModels([]string{"deepseek-v4-flash"}, 0.4),
	})
	resolver := NewModelPricingResolver(cs, newTestBillingService())

	resolved := resolver.Resolve(context.Background(), PricingInput{Model: "deepseek-v4-flash:0731", GroupID: &groupID})
	require.Equal(t, PricingSourceChannel, resolved.Source,
		"tagged request name must reach the channel pricing configured on the bare name")
	require.NotNil(t, resolved.BasePricing)
	require.InDelta(t, 0.4e-6, resolved.BasePricing.InputPricePerToken, 1e-12)
}

// TestChannelPricing_LiteralTaggedConfigWins 渠道价字面优先：同时配了带 tag 名与
// 基名时，请求带 tag 名必须命中带 tag 名的显式配价。
func TestChannelPricing_LiteralTaggedConfigWins(t *testing.T) {
	groupID := int64(889)
	cs := newChannelServiceWithPricings(groupID, []ChannelModelPricing{
		tokenPricingForModels([]string{"deepseek-v4-flash:0731"}, 0.9),
		tokenPricingForModels([]string{"deepseek-v4-flash"}, 0.4),
	})
	resolver := NewModelPricingResolver(cs, newTestBillingService())

	resolved := resolver.Resolve(context.Background(), PricingInput{Model: "deepseek-v4-flash:0731", GroupID: &groupID})
	require.Equal(t, PricingSourceChannel, resolved.Source)
	require.NotNil(t, resolved.BasePricing)
	require.InDelta(t, 0.9e-6, resolved.BasePricing.InputPricePerToken, 1e-12,
		"explicit per-tag channel pricing must win over the stripped base name")
}

// TestChannelPricing_TagStripMustNotMatchUnrelated 反向保护：剥 tag 后的备选名
// 不得命中渠道里不相关的配置，未命中时落回官方兜底价。
func TestChannelPricing_TagStripMustNotMatchUnrelated(t *testing.T) {
	groupID := int64(890)
	cs := newChannelServiceWithPricings(groupID, []ChannelModelPricing{
		tokenPricingForModels([]string{"gpt-5.4"}, 0.9),
	})
	resolver := NewModelPricingResolver(cs, newTestBillingService())

	resolved := resolver.Resolve(context.Background(), PricingInput{Model: "deepseek-v4-flash:0731", GroupID: &groupID})
	require.NotEqual(t, PricingSourceChannel, resolved.Source,
		"stripped lookup must not match an unrelated channel pricing entry")
	require.NotNil(t, resolved.BasePricing)
	require.InDelta(t, 0.15e-6, resolved.BasePricing.InputPricePerToken, 1e-12,
		"should land on the official deepseek-v4-flash fallback card")
}

// TestApplyChannelOverrides_TaggedModelInheritsLookup B2-① 第 4 条：渠道覆盖
// applyChannelOverrides 内部走 lookupChannelPricingNormalized，自动继承剥 tag 重试。
func TestApplyChannelOverrides_TaggedModelInheritsLookup(t *testing.T) {
	groupID := int64(891)
	cs := newChannelServiceWithPricings(groupID, []ChannelModelPricing{
		tokenPricingForModels([]string{"deepseek-v4-flash"}, 0.4),
	})
	resolver := NewModelPricingResolver(cs, newTestBillingService())

	resolved := &ResolvedPricing{Mode: BillingModeToken, Source: PricingSourceFallback}
	resolved.BasePricing = &ModelPricing{InputPricePerToken: 0.15e-6, OutputPricePerToken: 0.6e-6}
	resolver.applyChannelOverrides(context.Background(), groupID, "deepseek-v4-flash:0731", resolved)

	require.Equal(t, PricingSourceChannel, resolved.Source)
	require.NotNil(t, resolved.BasePricing)
	require.InDelta(t, 0.4e-6, resolved.BasePricing.InputPricePerToken, 1e-12)
}

// TestMatchGroupModelPricing_TaggedModelExactFallback 分组价路径（B2-① 第 3 条）：
// 精确配置 miss 后按剥 tag 名再跑一轮 exact 匹配。
func TestMatchGroupModelPricing_TaggedModelExactFallback(t *testing.T) {
	group := &Group{
		ModelPricing: []ChannelModelPricing{
			tokenPricingForModels([]string{"deepseek-v4-flash"}, 0.4),
		},
	}

	cp := matchGroupModelPricing(group, "deepseek-v4-flash:0731")
	require.NotNil(t, cp, "tagged request name must fall back to the exact bare-name group pricing")
	require.NotNil(t, cp.InputPrice)
	require.InDelta(t, 0.4e-6, *cp.InputPrice, 1e-12)
}

// TestMatchGroupModelPricing_TaggedNilWhenUnmatched 分组价剥 tag 重试不得命中
// 不相关配置：无任何可命中 pattern 时保持 nil。
func TestMatchGroupModelPricing_TaggedNilWhenUnmatched(t *testing.T) {
	group := &Group{
		ModelPricing: []ChannelModelPricing{
			tokenPricingForModels([]string{"gpt-5.4"}, 0.9),
		},
	}

	require.Nil(t, matchGroupModelPricing(group, "deepseek-v4-flash:0731"))
}

// TestMatchGroupModelPricing_WildcardStillMatchesTagged 通配 "prefix*" 本就能
// 前缀命中带 tag 名，剥 tag 改动不得破坏该路径。
func TestMatchGroupModelPricing_WildcardStillMatchesTagged(t *testing.T) {
	group := &Group{
		ModelPricing: []ChannelModelPricing{
			tokenPricingForModels([]string{"gpt-oss:120b*"}, 0.2),
		},
	}

	cp := matchGroupModelPricing(group, "gpt-oss:120b-cloud")
	require.NotNil(t, cp)
	require.NotNil(t, cp.InputPrice)
	require.InDelta(t, 0.2e-6, *cp.InputPrice, 1e-12)
}

// TestMatchGroupModelPricing_BedrockDoubleKey 分组价 bedrock 回归：带 ":0" 与
// 不带的双 pattern 并存时，各自命中各自条目。
func TestMatchGroupModelPricing_BedrockDoubleKey(t *testing.T) {
	taggedEntry := tokenPricingForModels([]string{"us.anthropic.claude-sonnet-4-5-20250929-v1:0"}, 1.0)
	bareEntry := tokenPricingForModels([]string{"us.anthropic.claude-sonnet-4-5-20250929-v1"}, 3.0)
	group := &Group{ModelPricing: []ChannelModelPricing{taggedEntry, bareEntry}}

	tagged := matchGroupModelPricing(group, "us.anthropic.claude-sonnet-4-5-20250929-v1:0")
	require.NotNil(t, tagged)
	require.InDelta(t, 1e-6, *tagged.InputPrice, 1e-12)

	bare := matchGroupModelPricing(group, "us.anthropic.claude-sonnet-4-5-20250929-v1")
	require.NotNil(t, bare)
	require.InDelta(t, 3e-6, *bare.InputPrice, 1e-12)
}

// TestOllamaTaggedModelPricingConsistentAcrossSources 一致性（spec 验收）：同名
// 模型在基础价 / 渠道价 / 分组价三种配置下命中同一价格档（0.15e-6 = 官方
// deepseek-v4-flash 低谷卡）。
func TestOllamaTaggedModelPricingConsistentAcrossSources(t *testing.T) {
	const model = "deepseek-v4-flash:0731"
	const tierInput = 0.15e-6
	groupID := int64(892)
	bs := newTestBillingService()

	// 基础价（家族匹配）
	base, err := bs.getModelPricingAt(model, ollamaPricingAt)
	require.NoError(t, err)
	require.InDelta(t, tierInput, base.InputPricePerToken, 1e-12)

	// 渠道价（剥 tag 命中）
	cs := newChannelServiceWithPricings(groupID, []ChannelModelPricing{
		tokenPricingForModels([]string{"deepseek-v4-flash"}, 0.15),
	})
	resolver := NewModelPricingResolver(cs, bs)
	chResolved := resolver.Resolve(context.Background(), PricingInput{Model: model, GroupID: &groupID})
	require.NotNil(t, chResolved.BasePricing)
	require.InDelta(t, tierInput, chResolved.BasePricing.InputPricePerToken, 1e-12)

	// 分组价（剥 tag exact 命中）
	group := &Group{ModelPricing: []ChannelModelPricing{
		tokenPricingForModels([]string{"deepseek-v4-flash"}, 0.15),
	}}
	gp := matchGroupModelPricing(group, model)
	require.NotNil(t, gp)
	require.InDelta(t, tierInput, *gp.InputPrice, 1e-12)
}

// TestGetModelPricing_OllamaModelsPublicEntryMatchesFixed 公开入口 GetModelPricing
// （内部 timezone.Now）与 getModelPricingAt(固定时点) 在非时点敏感的 ollama 模型上
// 解析到同一价卡。
func TestGetModelPricing_OllamaModelsPublicEntryMatchesFixed(t *testing.T) {
	bs := newTestBillingService()

	for _, model := range []string{"gpt-oss:120b", "nemotron-3-ultra", "mistral-large-3:675b"} {
		fixed, err := bs.getModelPricingAt(model, ollamaPricingAt)
		require.NoError(t, err, "model %s", model)
		nowPricing, err := bs.GetModelPricing(model)
		require.NoError(t, err, "model %s", model)
		require.Equal(t, fixed.InputPricePerToken, nowPricing.InputPricePerToken, "model %s", model)
		require.Equal(t, fixed.OutputPricePerToken, nowPricing.OutputPricePerToken, "model %s", model)
	}
}
