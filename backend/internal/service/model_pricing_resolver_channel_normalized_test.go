//go:build unit

package service

// issue #5256 回归测试：使用记录的费用统计没有按照渠道定价的价格进行计算。
//
// 场景：管理员在渠道定价把 gpt-5.6-luna 的输入价从官方 $0.2/M 调成 $0.4/M。
// 当请求模型带 effort 后缀（gpt-5.6-luna-high）而渠道只配了基名时，渠道定价查找
// 用字面名未命中，官方兜底价却会把后缀名归一化到 gpt-5.6-luna 并命中静态价
// （pricing_service.go 的 gpt-5.6-luna 前缀分支），计费候选循环首个成功即返回
// → 落库的是官方 0.2 而不是渠道 0.4。
//
// 测试走与生产一致的 populateChannelCache → OpenAIGatewayService.RecordUsage 路径，
// 断言落库 UsageLog 的 InputCost。

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

const (
	// 1M 输入 token 下，渠道价与官方兜底价的期望费用（USD）
	channelPricingExpectedChannelCost  = 0.4
	channelPricingExpectedOfficialCost = 0.2
	// 用于验证「不相关的渠道配置不会被误命中」的对照价
	channelPricingUnrelatedCost = 0.9
)

// tokenPricingForModels 构造 token 计费模式的渠道定价；inputPerMillion 单位为 USD/1M token。
func tokenPricingForModels(models []string, inputPerMillion float64) ChannelModelPricing {
	return ChannelModelPricing{
		Platform:        PlatformOpenAI,
		Models:          models,
		BillingMode:     BillingModeToken,
		InputPrice:      float64Ptr(inputPerMillion / 1e6),
		OutputPrice:     float64Ptr(2.4e-6),
		CacheWritePrice: float64Ptr(0.5e-6),
		CacheReadPrice:  float64Ptr(0.04e-6),
	}
}

func newChannelServiceWithPricings(groupID int64, pricings []ChannelModelPricing) *ChannelService {
	ch := Channel{
		ID:           1,
		Name:         "codex-channel",
		Status:       StatusActive,
		ModelPricing: pricings,
		GroupIDs:     []int64{groupID},
	}
	cs := &ChannelService{}
	cs.cache.Store(populateChannelCache([]Channel{ch}, map[int64]string{groupID: PlatformOpenAI}))
	return cs
}

// recordUsageWithChannelPricing 用给定的渠道定价跑一次 RecordUsage，返回落库的 UsageLog。
func recordUsageWithChannelPricing(t *testing.T, requestedModel string, subscriptionGroup bool, pricings []ChannelModelPricing) *UsageLog {
	t.Helper()
	const groupID = int64(777)

	usageRepo := &openAIRecordUsageLogRepoStub{inserted: true}
	svc := newOpenAIRecordUsageServiceForTest(usageRepo, &openAIRecordUsageUserRepoStub{}, &openAIRecordUsageSubRepoStub{}, nil)
	cs := newChannelServiceWithPricings(groupID, pricings)
	svc.channelService = cs
	svc.resolver = NewModelPricingResolver(cs, svc.billingService)

	group := &Group{
		ID:             groupID,
		Platform:       PlatformOpenAI,
		RateMultiplier: 1,
	}
	if subscriptionGroup {
		group.SubscriptionType = SubscriptionTypeSubscription
	}

	err := svc.RecordUsage(context.Background(), &OpenAIRecordUsageInput{
		Result: &OpenAIForwardResult{
			RequestID:    "resp_luna_5256",
			Model:        requestedModel,
			BillingModel: requestedModel,
			Usage: OpenAIUsage{
				InputTokens:  1_000_000,
				OutputTokens: 0,
			},
			Duration: time.Second,
		},
		ChannelUsageFields: ChannelUsageFields{
			OriginalModel:      requestedModel,
			ChannelMappedModel: requestedModel,
		},
		APIKey: &APIKey{
			ID:      1,
			GroupID: i64p(groupID),
			Group:   group,
		},
		User:    &User{ID: 1},
		Account: &Account{ID: 1, Platform: PlatformOpenAI},
	})
	require.NoError(t, err)
	require.NotNil(t, usageRepo.lastLog)
	return usageRepo.lastLog
}

// 基线：请求模型与渠道定价 key 完全一致 → 按渠道价计。
func TestChannelPricing_ExactModelMatch(t *testing.T) {
	log := recordUsageWithChannelPricing(t, "gpt-5.6-luna", false, []ChannelModelPricing{
		tokenPricingForModels([]string{"gpt-5.6-luna"}, channelPricingExpectedChannelCost),
	})
	require.InDelta(t, channelPricingExpectedChannelCost, log.InputCost, 1e-9)
}

// issue #5256 主回归：请求模型带 effort 后缀、渠道只配基名（无通配符）→ 仍应按渠道价计。
// 修复前此处得到 0.2（官方兜底价）。
func TestChannelPricing_SuffixedModelUsesNormalizedChannelPricing(t *testing.T) {
	log := recordUsageWithChannelPricing(t, "gpt-5.6-luna-high", false, []ChannelModelPricing{
		tokenPricingForModels([]string{"gpt-5.6-luna"}, channelPricingExpectedChannelCost),
	})
	require.InDelta(t, channelPricingExpectedChannelCost, log.InputCost, 1e-9,
		"suffixed request model should fall back to the normalized channel pricing; got %v (%v = official fallback)",
		log.InputCost, channelPricingExpectedOfficialCost)
}

// 同一根因的另一种变体名：上游返回带日期后缀的模型名
// （isCodexDateSuffix，如 gpt-5.6-luna-2026-08-01），渠道只配基名 → 仍应按渠道价计。
func TestChannelPricing_DateSuffixedModelUsesNormalizedChannelPricing(t *testing.T) {
	log := recordUsageWithChannelPricing(t, "gpt-5.6-luna-2026-08-01", false, []ChannelModelPricing{
		tokenPricingForModels([]string{"gpt-5.6-luna"}, channelPricingExpectedChannelCost),
	})
	require.InDelta(t, channelPricingExpectedChannelCost, log.InputCost, 1e-9,
		"date-suffixed request model should fall back to the normalized channel pricing; got %v", log.InputCost)
}

// 精确匹配优先：同时配了变体名与基名时，请求变体名必须命中变体的显式配价，
// 不能被归一化后的基名覆盖。
func TestChannelPricing_ExactVariantWinsOverNormalizedBaseName(t *testing.T) {
	log := recordUsageWithChannelPricing(t, "gpt-5.6-luna-high", false, []ChannelModelPricing{
		tokenPricingForModels([]string{"gpt-5.6-luna-high"}, channelPricingUnrelatedCost),
		tokenPricingForModels([]string{"gpt-5.6-luna"}, channelPricingExpectedChannelCost),
	})
	require.InDelta(t, channelPricingUnrelatedCost, log.InputCost, 1e-9,
		"explicit per-variant channel pricing must win over the normalized base name")
}

// 订阅型分组走同一条渠道定价解析路径。
func TestChannelPricing_SuffixedModelSubscriptionGroup(t *testing.T) {
	log := recordUsageWithChannelPricing(t, "gpt-5.6-luna-high", true, []ChannelModelPricing{
		tokenPricingForModels([]string{"gpt-5.6-luna"}, channelPricingExpectedChannelCost),
	})
	require.InDelta(t, channelPricingExpectedChannelCost, log.InputCost, 1e-9)
}

// 反向保护：渠道只配了不相关的模型时，归一化查找不得误命中该配置，
// 应落回官方兜底价。
func TestChannelPricing_UnrelatedChannelModelNotMatched(t *testing.T) {
	log := recordUsageWithChannelPricing(t, "gpt-5.6-luna-high", false, []ChannelModelPricing{
		tokenPricingForModels([]string{"gpt-5.4"}, channelPricingUnrelatedCost),
	})
	require.InDelta(t, channelPricingExpectedOfficialCost, log.InputCost, 1e-9,
		"normalized lookup must not match an unrelated channel pricing entry")
}
