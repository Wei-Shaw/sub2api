//go:build unit

package service

import (
	"context"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/pkg/ctxkey"
	"github.com/stretchr/testify/require"
)

// loadAwareRestrictionFixture 构造走负载感知路径（load_batch 开启 + 并发服务存在）的
// GatewayService：分组 10 绑定 upstream 计费基准的渠道，定价列表只允许 claude-sonnet-4-6。
// 账号 1 把 claude-fable-5-1 映射为自身（不在定价列表），账号 2 映射为 claude-sonnet-4-6（在列表）。
type loadAwareRestrictionFixture struct {
	svc              *GatewayService
	ctx              context.Context
	groupID          int64
	cache            *mockGatewayCacheForPlatform
	concurrencyCache *mockConcurrencyCache
}

func newLoadAwareRestrictionFixture(t *testing.T, restrict bool, sessionBindings map[string]int64, routing map[string][]int64) *loadAwareRestrictionFixture {
	t.Helper()

	groupID := int64(10)
	ch := Channel{
		ID:                 1,
		Status:             StatusActive,
		GroupIDs:           []int64{groupID},
		RestrictModels:     restrict,
		BillingModelSource: BillingModelSourceUpstream,
		ModelPricing: []ChannelModelPricing{
			{Platform: PlatformAnthropic, Models: []string{"claude-sonnet-4-6"}},
		},
	}
	channelSvc := newTestChannelService(makeStandardRepo(ch, map[int64]string{groupID: PlatformAnthropic}))

	accountRepo := &mockAccountRepoForPlatform{
		accounts: []Account{
			{
				ID: 1, Platform: PlatformAnthropic, Priority: 1, Status: StatusActive, Schedulable: true, Concurrency: 5,
				Credentials: map[string]any{
					"model_mapping": map[string]any{"claude-fable-5-1": "claude-fable-5-1"},
				},
			},
			{
				ID: 2, Platform: PlatformAnthropic, Priority: 2, Status: StatusActive, Schedulable: true, Concurrency: 5,
				Credentials: map[string]any{
					"model_mapping": map[string]any{"claude-fable-5-1": "claude-sonnet-4-6"},
				},
			},
		},
		accountsByID: map[int64]*Account{},
	}
	for i := range accountRepo.accounts {
		accountRepo.accountsByID[accountRepo.accounts[i].ID] = &accountRepo.accounts[i]
	}

	group := &Group{
		ID:                  groupID,
		Platform:            PlatformAnthropic,
		Status:              StatusActive,
		Hydrated:            true,
		ModelRoutingEnabled: len(routing) > 0,
		ModelRouting:        routing,
	}
	groupRepo := &mockGroupRepoForGateway{groups: map[int64]*Group{groupID: group}}

	cfg := testConfig()
	cfg.Gateway.Scheduling.LoadBatchEnabled = true
	cache := &mockGatewayCacheForPlatform{sessionBindings: sessionBindings}
	concurrencyCache := &mockConcurrencyCache{}

	svc := &GatewayService{
		accountRepo:        accountRepo,
		groupRepo:          groupRepo,
		channelService:     channelSvc,
		cache:              cache,
		cfg:                cfg,
		concurrencyService: NewConcurrencyService(concurrencyCache),
	}

	return &loadAwareRestrictionFixture{
		svc:              svc,
		ctx:              context.WithValue(context.Background(), ctxkey.Group, group),
		groupID:          groupID,
		cache:            cache,
		concurrencyCache: concurrencyCache,
	}
}

func TestSelectAccountWithLoadAwareness_UpstreamRestrictionSkipsDisallowedAccount(t *testing.T) {
	t.Parallel()

	f := newLoadAwareRestrictionFixture(t, true, nil, nil)
	result, err := f.svc.SelectAccountWithLoadAwareness(f.ctx, &f.groupID, "", "claude-fable-5-1", nil, "", 0)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.NotNil(t, result.Account)
	require.Equal(t, int64(2), result.Account.ID, "上游模型不在渠道定价列表的高优先级账号必须被跳过")
	require.Equal(t, 1, f.concurrencyCache.loadBatchCalls, "应经过 Layer 2 负载感知选择")
}

func TestSelectAccountWithLoadAwareness_UpstreamRestrictionRejectsWhenAllAccountsDisallowed(t *testing.T) {
	t.Parallel()

	f := newLoadAwareRestrictionFixture(t, true, nil, nil)
	// 账号 2 也改为映射到不在定价列表的模型
	f.svc.accountRepo.(*mockAccountRepoForPlatform).accounts[1].Credentials = map[string]any{
		"model_mapping": map[string]any{"claude-fable-5-1": "claude-fable-5-1"},
	}

	result, err := f.svc.SelectAccountWithLoadAwareness(f.ctx, &f.groupID, "", "claude-fable-5-1", nil, "", 0)
	require.ErrorIs(t, err, ErrNoAvailableAccounts)
	require.ErrorContains(t, err, "channel pricing restriction")
	require.Nil(t, result)
	require.Equal(t, 0, f.concurrencyCache.acquireAccountCalls, "没有合规候选时不应尝试占用任何账号槽位")
}

func TestSelectAccountWithLoadAwareness_UpstreamRestrictionIgnoresStickyAccount(t *testing.T) {
	t.Parallel()

	f := newLoadAwareRestrictionFixture(t, true, map[string]int64{"sticky": 1}, nil)
	result, err := f.svc.SelectAccountWithLoadAwareness(f.ctx, &f.groupID, "sticky", "claude-fable-5-1", nil, "", 0)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.NotNil(t, result.Account)
	require.Equal(t, int64(2), result.Account.ID, "粘性账号的上游模型不在定价列表时不得沿用粘性会话")
	require.Equal(t, int64(2), f.cache.sessionBindings["sticky"], "粘性会话应重新绑定到合规账号")
}

func TestSelectAccountWithLoadAwareness_UpstreamRestrictionFiltersRoutedAccounts(t *testing.T) {
	t.Parallel()

	f := newLoadAwareRestrictionFixture(t, true, nil, map[string][]int64{"claude-fable-5-1": {1}})
	result, err := f.svc.SelectAccountWithLoadAwareness(f.ctx, &f.groupID, "", "claude-fable-5-1", nil, "", 0)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.NotNil(t, result.Account)
	require.Equal(t, int64(2), result.Account.ID, "模型路由指向的账号被渠道限制时应回退到合规账号")
}

func TestSelectAccountWithLoadAwareness_UpstreamRestrictionRoutedStickyAccountNotHonored(t *testing.T) {
	t.Parallel()

	f := newLoadAwareRestrictionFixture(t, true, map[string]int64{"sticky": 1}, map[string][]int64{"claude-fable-5-1": {1, 2}})
	result, err := f.svc.SelectAccountWithLoadAwareness(f.ctx, &f.groupID, "sticky", "claude-fable-5-1", nil, "", 0)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.NotNil(t, result.Account)
	require.Equal(t, int64(2), result.Account.ID, "路由列表内的粘性账号被渠道限制时不得沿用")
}

func TestSelectAccountWithLoadAwareness_RestrictModelsDisabledKeepsPriorityOrder(t *testing.T) {
	t.Parallel()

	f := newLoadAwareRestrictionFixture(t, false, nil, nil)
	result, err := f.svc.SelectAccountWithLoadAwareness(f.ctx, &f.groupID, "", "claude-fable-5-1", nil, "", 0)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.NotNil(t, result.Account)
	require.Equal(t, int64(1), result.Account.ID, "未开启限制时不应因定价列表过滤账号")
}
