//go:build unit

package service

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func boolPtrForImageBillingTest(v bool) *bool { return &v }

func TestImageCountBillingApplies(t *testing.T) {
	enabled := &APIKey{Group: &Group{ImageCountBilling: true}}
	disabled := &APIKey{Group: &Group{ImageCountBilling: false}}

	require.True(t, imageCountBillingApplies(enabled, true))
	require.False(t, imageCountBillingApplies(enabled, false), "混合请求不能受开关影响")
	require.False(t, imageCountBillingApplies(disabled, true))
	require.False(t, imageCountBillingApplies(nil, true))
}

// F1：composite 分组配置了 token 定价又开启按张计费时，图片不能按 $0 结算。
func TestCalculateRecordUsageCost_ImageCountBillingGroupTokenPricing(t *testing.T) {
	groupID := int64(901)
	model := "gpt-image-2"
	resolver := newOpenAITokenImageChannelPricingResolverForTest(t, groupID, model)
	group := &Group{
		ID:                   groupID,
		Platform:             PlatformComposite,
		Status:               StatusActive,
		Hydrated:             true,
		AllowImageGeneration: true,
		ImageCountBilling:    true,
		ImagePrice1K:         f64p(0.5),
		ModelPricing: []ChannelModelPricing{{
			Models:      []string{model},
			BillingMode: BillingModeToken,
			InputPrice:  f64p(3e-6),
			OutputPrice: f64p(15e-6),
		}},
	}
	apiKey := &APIKey{ID: 1, GroupID: &groupID, Group: group}
	svc := &GatewayService{billingService: resolver.billingService, resolver: resolver}

	cost := svc.calculateRecordUsageCost(context.Background(), &ForwardResult{
		Model: model, ImageCount: 2, ImageSize: "1K", ImageEndpoint: true,
		Usage: ClaudeUsage{InputTokens: 1000, OutputTokens: 1000},
	}, apiKey, model, 1, 1, time.Time{})

	require.NotNil(t, cost)
	require.InDelta(t, 1.0, cost.TotalCost, 1e-12, "应回落分组图片价，而不是 token 定价的 $0")
	require.Equal(t, string(BillingModeImage), cost.BillingMode)
}

// F3：渠道 token 定价 + 开关开启时必须强制按张计费，与 OpenAI 流水线一致。
func TestCalculateRecordUsageCost_ImageCountBillingChannelTokenPricing(t *testing.T) {
	groupID := int64(902)
	model := "gpt-image-2"
	resolver := newOpenAITokenImageChannelPricingResolverForTest(t, groupID, model)
	group := &Group{
		ID:                   groupID,
		Platform:             PlatformOpenAI,
		Status:               StatusActive,
		Hydrated:             true,
		AllowImageGeneration: true,
		ImageCountBilling:    true,
		ImagePrice1K:         f64p(0.4),
	}
	apiKey := &APIKey{ID: 2, GroupID: &groupID, Group: group}
	svc := &GatewayService{billingService: resolver.billingService, resolver: resolver}

	cost := svc.calculateRecordUsageCost(context.Background(), &ForwardResult{
		Model: model, ImageCount: 2, ImageSize: "1K", ImageEndpoint: true,
		Usage: ClaudeUsage{InputTokens: 1_000_000, OutputTokens: 10_000},
	}, apiKey, model, 1, 1, time.Time{})

	require.NotNil(t, cost)
	require.InDelta(t, 0.8, cost.TotalCost, 1e-12)
	require.Equal(t, string(BillingModeImage), cost.BillingMode)
}

// F4：开关只作用于图片端点，混合请求仍按 token 计费。
func TestCalculateRecordUsageCost_ImageCountBillingIgnoresMixedRequests(t *testing.T) {
	groupID := int64(903)
	model := "gpt-image-2"
	resolver := newOpenAITokenImageChannelPricingResolverForTest(t, groupID, model)
	group := &Group{
		ID:                   groupID,
		Platform:             PlatformComposite,
		Status:               StatusActive,
		Hydrated:             true,
		AllowImageGeneration: true,
		ImageCountBilling:    true,
		ImagePrice1K:         f64p(0.4),
	}
	apiKey := &APIKey{ID: 3, GroupID: &groupID, Group: group}
	svc := &GatewayService{billingService: resolver.billingService, resolver: resolver}

	cost := svc.calculateRecordUsageCost(context.Background(), &ForwardResult{
		Model: model, ImageCount: 1, ImageSize: "1K", ImageEndpoint: false,
		Usage: ClaudeUsage{InputTokens: 1_000_000, OutputTokens: 10_000},
	}, apiKey, model, 1, 1, time.Time{})

	require.NotNil(t, cost)
	require.Equal(t, string(BillingModeToken), cost.BillingMode)
	require.Greater(t, cost.TotalCost, 0.0)
}

func TestCalculateOpenAIRecordUsageCost_ImageCountBillingIgnoresMixedRequests(t *testing.T) {
	groupID := int64(904)
	model := "gpt-image-2"
	resolver := newOpenAITokenImageChannelPricingResolverForTest(t, groupID, model)
	group := &Group{
		ID:                   groupID,
		Platform:             PlatformOpenAI,
		Status:               StatusActive,
		Hydrated:             true,
		AllowImageGeneration: true,
		ImageCountBilling:    true,
		ImagePrice1K:         f64p(0.4),
	}
	apiKey := &APIKey{ID: 4, GroupID: &groupID, Group: group}
	svc := &OpenAIGatewayService{billingService: resolver.billingService, resolver: resolver}

	cost, err := svc.calculateOpenAIRecordUsageCost(context.Background(), &OpenAIForwardResult{
		Model: model, ImageCount: 1, ImageSize: "1K", ImageEndpoint: false,
	}, apiKey, []string{model}, 1, 1, 1, 1,
		UsageTokens{InputTokens: 1_000_000, OutputTokens: 10_000},
		"", nil, false, time.Time{})

	require.NoError(t, err)
	require.NotNil(t, cost)
	require.Equal(t, string(BillingModeToken), cost.BillingMode)
	require.Greater(t, cost.TotalCost, 0.0)
}

func TestGroupService_ImageCountBillingValidation(t *testing.T) {
	ctx := context.Background()

	t.Run("创建入口固定 anthropic 平台，拒绝开关", func(t *testing.T) {
		svc := NewGroupService(nil, nil)
		_, err := svc.Create(ctx, CreateGroupRequest{Name: "g", ImageCountBilling: true})
		require.ErrorContains(t, err, "image_count_billing")
	})

	t.Run("更新时未开启图片生成则拒绝", func(t *testing.T) {
		repo := &groupRepoStubForAdmin{getByID: &Group{
			ID: 1, Platform: PlatformComposite, AllowImageGeneration: false,
		}}
		svc := NewGroupService(repo, nil)
		_, err := svc.Update(ctx, 1, UpdateGroupRequest{ImageCountBilling: boolPtrForImageBillingTest(true)})
		require.ErrorContains(t, err, "image_count_billing")
	})

	t.Run("更新时不支持的平台拒绝", func(t *testing.T) {
		repo := &groupRepoStubForAdmin{getByID: &Group{
			ID: 2, Platform: PlatformAnthropic, AllowImageGeneration: true,
		}}
		svc := NewGroupService(repo, nil)
		_, err := svc.Update(ctx, 2, UpdateGroupRequest{ImageCountBilling: boolPtrForImageBillingTest(true)})
		require.ErrorContains(t, err, "image_count_billing")
	})

	t.Run("合法开启", func(t *testing.T) {
		repo := &groupRepoStubForAdmin{getByID: &Group{
			ID: 3, Platform: PlatformComposite, AllowImageGeneration: true,
		}}
		svc := NewGroupService(repo, nil)
		updated, err := svc.Update(ctx, 3, UpdateGroupRequest{ImageCountBilling: boolPtrForImageBillingTest(true)})
		require.NoError(t, err)
		require.True(t, updated.ImageCountBilling)
	})

	t.Run("关闭图片生成时自动清理开关", func(t *testing.T) {
		repo := &groupRepoStubForAdmin{getByID: &Group{
			ID: 4, Platform: PlatformComposite, AllowImageGeneration: false, ImageCountBilling: true,
		}}
		svc := NewGroupService(repo, nil)
		updated, err := svc.Update(ctx, 4, UpdateGroupRequest{})
		require.NoError(t, err)
		require.False(t, updated.ImageCountBilling)
	})
}
