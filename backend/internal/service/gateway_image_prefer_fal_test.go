//go:build unit

package service

import (
	"context"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/ctxkey"
	"github.com/stretchr/testify/require"
)

// newFalUpstreamPricingResolver 构造一个让 fal 账号能通过 falAccountPricingConfigured 校验的解析器：
// 在 channel cache 里给 (groupID, upstreamModel) 写入 image/per-request 模式的定价。
func newFalUpstreamPricingResolver(t *testing.T, groupID int64, upstreamModel string, price float64) *ModelPricingResolver {
	t.Helper()
	cache := newEmptyChannelCache()
	cache.pricingByGroupModel[channelModelKey{groupID: groupID, model: upstreamModel}] = &ChannelModelPricing{
		BillingMode:     BillingModeImage,
		PerRequestPrice: &price,
	}
	cache.channelByGroupID[groupID] = &Channel{ID: groupID, Status: StatusActive}
	cache.groupPlatform[groupID] = ""
	cache.loadedAt = time.Now()
	cs := &ChannelService{}
	cs.cache.Store(cache)
	return NewModelPricingResolver(cs, NewBillingService(&config.Config{}, nil))
}

// withTestGroup 把一个 hydrated group 注入 ctx，让 resolveGroupByID 直接命中而不走 groupRepo。
func withTestGroup(ctx context.Context, groupID int64) context.Context {
	g := &Group{
		ID:       groupID,
		Name:     "test-group",
		Platform: PlatformOpenAI,
		Status:   StatusActive,
		Hydrated: true,
	}
	return context.WithValue(ctx, ctxkey.Group, g)
}

// TestSelectImageAccountMixed_PreferFal_ReversesOrdering 覆盖 spec D6：
// preferPlatform="fal" 时反转候选池排序为 fal 优先 + openai 兜底。
func TestSelectImageAccountMixed_PreferFal_ReversesOrdering(t *testing.T) {
	const t2iModel = "gpt-image-2"
	const upstream = "openai/gpt-image-2"
	groupID := int64(7777)
	ctx := withTestGroup(context.Background(), groupID)

	makeFal := func(id int64, prio int) Account {
		return falImageAccount(id, prio, map[string]any{t2iModel: upstream})
	}

	newSvc := func(repo *mockAccountRepoForPlatform) *GatewayService {
		return &GatewayService{
			accountRepo: repo,
			cache:       &mockGatewayCacheForPlatform{},
			cfg:         testConfig(),
			resolver:    newFalUpstreamPricingResolver(t, groupID, upstream, 0.25),
		}
	}

	t.Run("default_prefers_openai_when_higher_priority", func(t *testing.T) {
		repo := newImageMixedRepo([]Account{
			openaiImageAccount(1, 1),
			makeFal(2, 2),
		})
		svc := newSvc(repo)
		acc, err := svc.SelectImageAccountMixed(ctx, &groupID, "", t2iModel, nil, OpenAIImagesCapabilityBasic, "", "")
		require.NoError(t, err)
		require.NotNil(t, acc)
		require.Equal(t, PlatformOpenAI, acc.Platform, "default 应保持 openai 优先")
		require.Equal(t, int64(1), acc.ID)
	})

	t.Run("prefer_fal_overrides_openai_priority", func(t *testing.T) {
		repo := newImageMixedRepo([]Account{
			openaiImageAccount(1, 1), // openai 优先级更高（数字更小）
			makeFal(2, 5),            // fal 优先级很低
		})
		svc := newSvc(repo)
		acc, err := svc.SelectImageAccountMixed(ctx, &groupID, "", t2iModel, nil, OpenAIImagesCapabilityBasic, "", "fal")
		require.NoError(t, err)
		require.NotNil(t, acc)
		require.Equal(t, PlatformFal, acc.Platform, "prefer_fal=true 时即便 fal 优先级更低也应被选中")
		require.Equal(t, int64(2), acc.ID)
	})

	t.Run("prefer_fal_falls_back_to_openai_when_fal_pool_empty", func(t *testing.T) {
		repo := newImageMixedRepo([]Account{
			openaiImageAccount(1, 1),
		})
		svc := newSvc(repo)
		acc, err := svc.SelectImageAccountMixed(ctx, &groupID, "", t2iModel, nil, OpenAIImagesCapabilityBasic, "", "fal")
		require.NoError(t, err)
		require.NotNil(t, acc)
		require.Equal(t, PlatformOpenAI, acc.Platform, "fal 子池为空时应自动兜底 openai")
		require.Equal(t, int64(1), acc.ID)
	})

	t.Run("prefer_fal_falls_back_to_openai_when_fal_excluded", func(t *testing.T) {
		repo := newImageMixedRepo([]Account{
			openaiImageAccount(1, 5),
			makeFal(2, 1),
		})
		svc := newSvc(repo)
		excluded := map[int64]struct{}{2: {}}
		acc, err := svc.SelectImageAccountMixed(ctx, &groupID, "", t2iModel, excluded, OpenAIImagesCapabilityBasic, "", "fal")
		require.NoError(t, err)
		require.NotNil(t, acc)
		require.Equal(t, PlatformOpenAI, acc.Platform, "fal 被排除后应兜底 openai")
		require.Equal(t, int64(1), acc.ID)
	})

	t.Run("prefer_fal_picks_among_multiple_fal_by_priority", func(t *testing.T) {
		repo := newImageMixedRepo([]Account{
			openaiImageAccount(1, 1),
			makeFal(2, 5),
			makeFal(3, 3),
			makeFal(4, 5),
		})
		svc := newSvc(repo)
		acc, err := svc.SelectImageAccountMixed(ctx, &groupID, "", t2iModel, nil, OpenAIImagesCapabilityBasic, "", "fal")
		require.NoError(t, err)
		require.NotNil(t, acc)
		require.Equal(t, PlatformFal, acc.Platform)
		require.Equal(t, int64(3), acc.ID, "fal 子池内仍按优先级选号")
	})

	t.Run("prefer_fal_ignored_when_value_empty_or_openai", func(t *testing.T) {
		repo := newImageMixedRepo([]Account{
			openaiImageAccount(1, 1),
			makeFal(2, 5),
		})
		svc := newSvc(repo)
		for _, prefer := range []string{"", "openai", "OPENAI", "  "} {
			acc, err := svc.SelectImageAccountMixed(ctx, &groupID, "", t2iModel, nil, OpenAIImagesCapabilityBasic, "", prefer)
			require.NoError(t, err, "prefer=%q", prefer)
			require.NotNil(t, acc, "prefer=%q", prefer)
			require.Equalf(t, PlatformOpenAI, acc.Platform, "prefer=%q 应保持 default 行为", prefer)
		}
	})
}
