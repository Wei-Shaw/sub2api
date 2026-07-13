//go:build unit

package service

import (
	"context"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

func TestCalculateWebSearchCostDefaultAndOverride(t *testing.T) {
	t.Parallel()
	s := &BillingService{}

	// 默认价：官方 $10/1000 次 = 0.01/次
	cost := s.CalculateWebSearchCost(1, nil, 1.0)
	require.InDelta(t, 0.01, cost.TotalCost, 1e-12)
	require.InDelta(t, 0.01, cost.ActualCost, 1e-12)
	require.Equal(t, string(BillingModePerRequest), cost.BillingMode)

	// 分组覆盖价 + 倍率
	cost = s.CalculateWebSearchCost(1, float64Ptr(0.02), 2.5)
	require.InDelta(t, 0.02, cost.TotalCost, 1e-12)
	require.InDelta(t, 0.05, cost.ActualCost, 1e-12)

	// 0 = 免费（区别于 nil = 默认价）
	cost = s.CalculateWebSearchCost(1, float64Ptr(0), 3.0)
	require.Zero(t, cost.TotalCost)
	require.Zero(t, cost.ActualCost)

	// 负数倍率按 0 处理，避免按 1x 误扣
	cost = s.CalculateWebSearchCost(1, nil, -1)
	require.InDelta(t, 0.01, cost.TotalCost, 1e-12)
	require.Zero(t, cost.ActualCost)

	// 次数 <= 0 不产生费用
	cost = s.CalculateWebSearchCost(0, float64Ptr(0.02), 1.0)
	require.Zero(t, cost.TotalCost)
	require.Empty(t, cost.BillingMode)
}

func TestCalculateOpenAIRecordUsageCostWebSearchPerCall(t *testing.T) {
	t.Parallel()
	svc := &OpenAIGatewayService{billingService: &BillingService{}}
	groupID := int64(11)

	// 分组未配置单价：默认 0.01。按次搜索使用不含高峰因子的基础倍率（第 4 个倍率参数 2.0），
	// 即使 token 倍率（含高峰，3.0）更高也不采用。
	apiKey := &APIKey{ID: 1, GroupID: &groupID, Group: &Group{ID: groupID, Platform: PlatformOpenAI}}
	result := &OpenAIForwardResult{Model: "gpt-5.6-sol", UpstreamModel: "gpt-5.6-sol", WebSearchCalls: 1}
	cost, err := svc.calculateOpenAIRecordUsageCost(context.Background(), result, apiKey, []string{"gpt-5.6-sol"}, 3.0, 1.0, 1.0, 2.0, UsageTokens{}, "")
	require.NoError(t, err)
	require.Equal(t, string(BillingModePerRequest), cost.BillingMode)
	require.InDelta(t, 0.01, cost.TotalCost, 1e-12)
	require.InDelta(t, 0.02, cost.ActualCost, 1e-12)

	// 分组配置单价 0.005
	apiKey.Group.WebSearchPricePerCall = float64Ptr(0.005)
	cost, err = svc.calculateOpenAIRecordUsageCost(context.Background(), result, apiKey, []string{"gpt-5.6-sol"}, 1.0, 1.0, 1.0, 1.0, UsageTokens{}, "")
	require.NoError(t, err)
	require.InDelta(t, 0.005, cost.TotalCost, 1e-12)
	require.InDelta(t, 0.005, cost.ActualCost, 1e-12)

	// WebSearchCalls = 0 时不得走按次分支（无定价数据会返回 pricing 错误，
	// 证明回落到了 token 路径而不是被按次分支吞掉）。
	result.WebSearchCalls = 0
	_, err = svc.calculateOpenAIRecordUsageCost(context.Background(), result, apiKey, []string{"gpt-5.6-sol"}, 1.0, 1.0, 1.0, 1.0, UsageTokens{InputTokens: 10}, "")
	require.Error(t, err)
}

func TestAPIKeyService_SnapshotRoundTrip_PreservesWebSearchPricePerCall(t *testing.T) {
	svc := NewAPIKeyService(nil, nil, nil, nil, nil, nil, &config.Config{})
	groupID := int64(9)
	apiKey := &APIKey{
		ID:      1,
		UserID:  2,
		GroupID: &groupID,
		Key:     "k-websearch",
		Status:  StatusActive,
		User:    &User{ID: 2, Status: StatusActive, Role: RoleUser},
		Group: &Group{
			ID:                    groupID,
			Name:                  "openai",
			Platform:              PlatformOpenAI,
			Status:                StatusActive,
			SubscriptionType:      SubscriptionTypeStandard,
			RateMultiplier:        1,
			WebSearchPricePerCall: float64Ptr(0.008),
		},
	}

	snapshot := svc.snapshotFromAPIKey(context.Background(), apiKey)
	roundTrip := svc.snapshotToAPIKey(apiKey.Key, snapshot)

	require.NotNil(t, roundTrip)
	require.NotNil(t, roundTrip.Group)
	require.NotNil(t, roundTrip.Group.WebSearchPricePerCall)
	require.InDelta(t, 0.008, *roundTrip.Group.WebSearchPricePerCall, 1e-12)
}
