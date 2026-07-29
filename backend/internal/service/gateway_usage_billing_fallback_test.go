//go:build unit

package service

import (
	"context"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"

	"github.com/stretchr/testify/require"
)

// composite 分组的公开别名经 BillingModelSource 来源覆盖成为计费模型后有两类错计：
// 任意别名（如 team/best）查无价静默落 $0；含家族词的别名（如 all/claude）被价格表
// 家族模糊匹配错计（Opus 流量按 Sonnet 兜底价）。compositeBillableModel 要求别名必须
// 有显式渠道定价才可参与计费，否则回退实际转发的具体模型。
func TestCompositeBillableModel(t *testing.T) {
	svc := &GatewayService{billingService: NewBillingService(&config.Config{}, nil)}
	apiKey := &APIKey{}
	ctx := context.Background()

	// 别名无渠道定价（含家族词也一样）→ 回退具体模型
	require.Equal(t, "claude-opus-4-7",
		svc.compositeBillableModel(ctx, apiKey, "all/claude", "claude-opus-4-7"))
	require.Equal(t, "claude-sonnet-4",
		svc.compositeBillableModel(ctx, apiKey, "team/best", "claude-sonnet-4"))

	// 未发生来源覆盖（计费模型已是具体模型）→ 原样返回
	require.Equal(t, "claude-sonnet-4",
		svc.compositeBillableModel(ctx, apiKey, "claude-sonnet-4", "claude-sonnet-4"))

	// 具体模型缺失 → 保持原值（走后续通用兜底/既有路径）
	require.Equal(t, "all/claude",
		svc.compositeBillableModel(ctx, apiKey, "all/claude", ""))
}

// billableModelWithFallback 是通用安全网：选定计费模型查不到任何价格时回退到
// 实际转发的具体模型；已定价流量（含家族兜底可解析的名字）不受影响。
func TestBillableModelWithFallback(t *testing.T) {
	svc := &GatewayService{billingService: NewBillingService(&config.Config{}, nil)}
	apiKey := &APIKey{}
	ctx := context.Background()

	// 完全无价的别名 → 回退到具体转发模型（claude-sonnet-4 有内置回退价格）
	require.Equal(t, "claude-sonnet-4",
		svc.billableModelWithFallback(ctx, apiKey, "team/best", "", "claude-sonnet-4"))

	// 已定价模型不回退，候选被忽略
	require.Equal(t, "claude-sonnet-4",
		svc.billableModelWithFallback(ctx, apiKey, "claude-sonnet-4", "claude-opus-4"))

	// 所有候选都无价 → 保持原值，走既有 warn + 零成本路径
	require.Equal(t, "team/best",
		svc.billableModelWithFallback(ctx, apiKey, "team/best", "another/alias", ""))

	// 空计费模型 + 有价候选 → 取候选
	require.Equal(t, "claude-sonnet-4",
		svc.billableModelWithFallback(ctx, apiKey, "", "claude-sonnet-4"))
}

func TestHasResolvableTokenPricing(t *testing.T) {
	svc := &GatewayService{billingService: NewBillingService(&config.Config{}, nil)}
	apiKey := &APIKey{}
	ctx := context.Background()

	require.True(t, svc.hasResolvableTokenPricing(ctx, "claude-sonnet-4", apiKey))
	// 注意：含家族词的名字（all/claude）会被价格表家族兜底解析为"有价"，
	// 这正是 compositeBillableModel 必须先于通用兜底拦截别名的原因。
	require.True(t, svc.hasResolvableTokenPricing(ctx, "all/claude", apiKey))
	require.False(t, svc.hasResolvableTokenPricing(ctx, "team/best", apiKey))
	require.False(t, svc.hasResolvableTokenPricing(ctx, "", apiKey))

	// billingService 缺失时 fail-closed（不误判有价）
	empty := &GatewayService{}
	require.False(t, empty.hasResolvableTokenPricing(ctx, "claude-sonnet-4", apiKey))
}
