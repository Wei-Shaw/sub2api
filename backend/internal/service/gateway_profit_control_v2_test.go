//go:build unit

package service

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/ctxkey"
	"github.com/stretchr/testify/require"
)

func gatewayProfitTestGroup(id int64, platform string) *Group {
	return &Group{
		ID:                   id,
		Name:                 "profit-" + platform,
		Platform:             platform,
		Status:               StatusActive,
		Hydrated:             true,
		RateMultiplier:       0.5,
		SubscriptionType:     SubscriptionTypeStandard,
		ProfitControlEnabled: true,
		ProfitMinMargin:      0,
		ProfitSafetyBuffer:   0,
	}
}

func gatewayProfitTestContext(group *Group) context.Context {
	ctx := context.WithValue(context.Background(), ctxkey.Group, group)
	ctx, _ = WithGatewayTokenRequestPricing(ctx)
	return ctx
}

func gatewayProfitTestAccount(id int64, platform string, rate float64, groupID int64) Account {
	return Account{
		ID:             id,
		Name:           "account",
		Platform:       platform,
		Type:           AccountTypeAPIKey,
		Status:         StatusActive,
		Schedulable:    true,
		Concurrency:    2,
		Priority:       1,
		RateMultiplier: &rate,
		AccountGroups:  []AccountGroup{{AccountID: id, GroupID: groupID}},
		GroupIDs:       []int64{groupID},
	}
}

func TestGatewayProfitControlInstallsForFivePlatformsOnlyOnTokenRequests(t *testing.T) {
	for _, platform := range []string{
		PlatformOpenAI,
		PlatformAnthropic,
		PlatformGemini,
		PlatformGrok,
		PlatformAntigravity,
	} {
		t.Run(platform, func(t *testing.T) {
			group := gatewayProfitTestGroup(101, platform)
			groupID := group.ID
			svc := &GatewayService{}

			tokenCtx := svc.withGatewayProfitControlGate(gatewayProfitTestContext(group), &groupID)
			gate, _ := tokenCtx.Value(openAIProfitControlGateCtxKey{}).(*openAIProfitControlGate)
			require.NotNil(t, gate)
			require.Equal(t, platform, gate.platform)
			require.InDelta(t, 0.5, gate.threshold, 1e-12)

			metadataCtx := context.WithValue(context.Background(), ctxkey.Group, group)
			metadataCtx = svc.withGatewayProfitControlGate(metadataCtx, &groupID)
			gate, _ = metadataCtx.Value(openAIProfitControlGateCtxKey{}).(*openAIProfitControlGate)
			require.Nil(t, gate, "未显式标记为 token 请求的入口不得装门")
		})
	}
}

func TestGatewayProfitControlCompositeBillingUsesScheduledMemberConfig(t *testing.T) {
	billingGroup := &Group{
		ID:               201,
		Platform:         PlatformComposite,
		Status:           StatusActive,
		Hydrated:         true,
		RateMultiplier:   0.4,
		SubscriptionType: SubscriptionTypeStandard,
	}
	memberGroup := gatewayProfitTestGroup(202, PlatformAnthropic)
	memberGroup.RateMultiplier = 99
	memberGroup.ProfitMinMargin = 0.25

	ctx := context.WithValue(context.Background(), ctxkey.Group, billingGroup)
	ctx, pricingAt := WithGatewayTokenRequestPricing(ctx)
	svc := &GatewayService{
		schedulerSnapshot: NewSchedulerSnapshotService(
			nil,
			nil,
			nil,
			profitControlGroupRepo{group: memberGroup},
			nil,
		),
	}
	ctx = svc.withGatewayProfitControlGate(ctx, &memberGroup.ID)
	gate, _ := ctx.Value(openAIProfitControlGateCtxKey{}).(*openAIProfitControlGate)
	require.NotNil(t, gate)
	require.Equal(t, memberGroup.ID, gate.groupID)
	require.Equal(t, PlatformAnthropic, gate.platform)
	require.Equal(t, pricingAt, gate.pricingAt)
	require.InDelta(t, 0.4*(1-0.25), gate.threshold, 1e-12, "D 必须取 composite 计费父分组，margin 取被调度成员分组")
}

func TestGatewayProfitControlGroupLoadFailureClearsForeignGate(t *testing.T) {
	billingGroup := &Group{
		ID:               211,
		Platform:         PlatformComposite,
		Status:           StatusActive,
		Hydrated:         true,
		RateMultiplier:   0.4,
		SubscriptionType: SubscriptionTypeStandard,
	}
	targetGroupID := int64(212)
	ctx := gatewayProfitTestContext(billingGroup)
	ctx = context.WithValue(ctx, openAIProfitControlGateCtxKey{}, &openAIProfitControlGate{
		groupID:   210,
		platform:  PlatformAnthropic,
		threshold: 0.1,
	})
	svc := &GatewayService{
		schedulerSnapshot: NewSchedulerSnapshotService(
			nil,
			nil,
			nil,
			profitControlFailingGroupRepo{},
			nil,
		),
	}

	ctx = svc.withGatewayProfitControlGate(ctx, &targetGroupID)
	gate, ok := ctx.Value(openAIProfitControlGateCtxKey{}).(*openAIProfitControlGate)
	require.True(t, ok)
	require.Nil(t, gate, "加载新分组失败时必须清除其他分组遗留的门")

	account := gatewayProfitTestAccount(213, PlatformAnthropic, 0.8, targetGroupID)
	require.True(t, svc.isGatewayAccountProfitEligible(ctx, &account), "配置读取失败按既定语义 fail-open")
}

type profitControlFailingGroupRepo struct {
	GroupRepository
}

func (profitControlFailingGroupRepo) GetByIDLite(context.Context, int64) (*Group, error) {
	return nil, errors.New("group cache unavailable")
}

// 见 profitControlGroupRepo.GetByID：利润门必须走不带账号计数聚合的 lite 读取。
func (profitControlFailingGroupRepo) GetByID(context.Context, int64) (*Group, error) {
	panic("profit control gate must read groups via GetByIDLite (no account-count aggregation)")
}

func TestGatewayProfitControlLegacyMixedAndRoutedSelection(t *testing.T) {
	t.Run("legacy single-platform selection", func(t *testing.T) {
		group := gatewayProfitTestGroup(111, PlatformGrok)
		cheap := gatewayProfitTestAccount(1, PlatformGrok, 0.2, group.ID)
		expensive := gatewayProfitTestAccount(2, PlatformGrok, 0.8, group.ID)
		repo := &mockAccountRepoForPlatform{
			accounts:     []Account{expensive, cheap},
			accountsByID: map[int64]*Account{cheap.ID: &cheap, expensive.ID: &expensive},
		}
		svc := &GatewayService{
			accountRepo: repo,
			cache:       &mockGatewayCacheForPlatform{},
			cfg:         testConfig(),
		}

		selected, err := svc.SelectAccountForModelWithExclusions(
			gatewayProfitTestContext(group), &group.ID, "", "", nil,
		)
		require.NoError(t, err)
		require.Equal(t, cheap.ID, selected.ID)

		_, err = svc.SelectAccountForModelWithExclusions(
			gatewayProfitTestContext(group), &group.ID, "", "", map[int64]struct{}{cheap.ID: {}},
		)
		require.Error(t, err)
		require.ErrorIs(t, err, ErrNoAvailableAccounts)
	})

	t.Run("mixed routing filters the routed account", func(t *testing.T) {
		group := gatewayProfitTestGroup(112, PlatformAnthropic)
		group.ModelRoutingEnabled = true
		group.ModelRouting = map[string][]int64{"claude-test": {2, 1}}
		cheap := gatewayProfitTestAccount(1, PlatformAntigravity, 0.2, group.ID)
		cheap.Extra = map[string]any{"mixed_scheduling": true}
		cheap.Credentials = map[string]any{"model_mapping": map[string]any{"claude-test": "claude-test"}}
		expensive := gatewayProfitTestAccount(2, PlatformAnthropic, 0.8, group.ID)
		repo := &mockAccountRepoForPlatform{
			accounts:     []Account{expensive, cheap},
			accountsByID: map[int64]*Account{cheap.ID: &cheap, expensive.ID: &expensive},
		}
		svc := &GatewayService{
			accountRepo: repo,
			cache:       &mockGatewayCacheForPlatform{},
			cfg:         testConfig(),
		}

		selected, err := svc.SelectAccountForModelWithExclusions(
			gatewayProfitTestContext(group), &group.ID, "", "claude-test", nil,
		)
		require.NoError(t, err)
		require.Equal(t, cheap.ID, selected.ID)
	})
}

func TestGatewayProfitControlLoadAwareSelectionAndFailover(t *testing.T) {
	group := gatewayProfitTestGroup(121, PlatformGrok)
	cheap := gatewayProfitTestAccount(1, PlatformGrok, 0.2, group.ID)
	expensive := gatewayProfitTestAccount(2, PlatformGrok, 0.8, group.ID)
	repo := &mockAccountRepoForPlatform{
		accounts:     []Account{expensive, cheap},
		accountsByID: map[int64]*Account{cheap.ID: &cheap, expensive.ID: &expensive},
	}
	cfg := &config.Config{RunMode: config.RunModeStandard}
	cfg.Gateway.Scheduling.LoadBatchEnabled = true
	svc := &GatewayService{
		accountRepo:        repo,
		cache:              &mockGatewayCacheForPlatform{},
		cfg:                cfg,
		concurrencyService: NewConcurrencyService(stubConcurrencyCache{}),
	}

	result, err := svc.SelectAccountWithLoadAwareness(
		gatewayProfitTestContext(group), &group.ID, "", "", nil, "", 0,
	)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Equal(t, cheap.ID, result.Account.ID)
	if result.ReleaseFunc != nil {
		result.ReleaseFunc()
	}

	result, err = svc.SelectAccountWithLoadAwareness(
		gatewayProfitTestContext(group),
		&group.ID,
		"",
		"",
		map[int64]struct{}{cheap.ID: {}},
		"",
		0,
	)
	require.Nil(t, result)
	require.Error(t, err)
	require.ErrorIs(t, err, ErrNoAvailableAccounts)
}

// 策略变更：门下终检通过不再建立或覆盖任何绑定，粘性只由确认成功
// 的提交维护；原账号价格恢复后也不因为它是旧绑定就自动抢回。
func TestGatewayProfitControlStickySuccessTakesOverAfterRateSwap(t *testing.T) {
	group := gatewayProfitTestGroup(131, PlatformAnthropic)
	expensive := gatewayProfitTestAccount(1, PlatformAnthropic, 0.8, group.ID)
	cheap := gatewayProfitTestAccount(2, PlatformAnthropic, 0.2, group.ID)
	repo := &mockAccountRepoForPlatform{
		accounts:     []Account{expensive, cheap},
		accountsByID: map[int64]*Account{expensive.ID: &expensive, cheap.ID: &cheap},
	}
	cache := &mockGatewayCacheForPlatform{
		sessionBindings: map[string]int64{"sticky-profit": expensive.ID},
	}
	svc := &GatewayService{
		accountRepo: repo,
		cache:       cache,
		cfg:         testConfig(),
	}
	ctx := gatewayProfitTestContext(group)
	ctx = svc.BeginGatewayStickySuccess(ctx, &group.ID, "sticky-profit", "claude-test")
	require.True(t, GatewayStickySuccessActive(ctx), "门下必须装配成功偏好状态")

	selected, err := svc.SelectAccountForModelWithExclusions(ctx, &group.ID, "sticky-profit", "", nil)
	require.NoError(t, err)
	require.Equal(t, cheap.ID, selected.ID)
	require.Equal(t, expensive.ID, cache.sessionBindings["sticky-profit"], "候选过滤不得覆盖旧粘性绑定")

	require.NoError(t, svc.BindStickySessionAfterProfitAdmission(
		svc.withGatewayProfitControlGate(ctx, &group.ID),
		&group.ID,
		"sticky-profit",
		cheap.ID,
	))
	require.Equal(t, expensive.ID, cache.sessionBindings["sticky-profit"], "门下终检通过不得写入任何绑定")
	require.Zero(t, cache.deletedSessions["sticky-profit"])

	// 上游真正成功：成功偏好指向 B，旧键同步到 B。
	svc.CommitGatewayStickySuccess(ctx, nil, &cheap, &ForwardResult{}, nil)
	require.Equal(t, cheap.ID, cache.sessionBindings["sticky-profit"], "成功提交后旧键同步到成功账号")
	require.Equal(t, cheap.ID, cache.successBindings[gatewayStickySuccessTestKey(group.ID, "sticky-profit", "claude-test")].AccountID)

	recovered := expensive
	recoveredRate := 0.2
	recovered.RateMultiplier = &recoveredRate
	repo.accounts[0] = recovered
	repo.accountsByID[recovered.ID] = &repo.accounts[0]

	nextCtx := gatewayProfitTestContext(group)
	nextCtx = svc.BeginGatewayStickySuccess(nextCtx, &group.ID, "sticky-profit", "claude-test")
	selected, err = svc.SelectAccountForModelWithExclusions(nextCtx, &group.ID, "sticky-profit", "", nil)
	require.NoError(t, err)
	require.Equal(t, cheap.ID, selected.ID, "原账号倍率恢复后也不自动抢回旧绑定")
	require.Zero(t, cache.deletedSessions["sticky-profit"])

	// 模型隔离：另一模型没有成功偏好，回落旧绑定（此时旧键已是 B）。
	otherCtx := svc.BeginGatewayStickySuccess(gatewayProfitTestContext(group), &group.ID, "sticky-profit", "claude-other")
	other := gatewayStickySuccessFromContext(otherCtx)
	require.NotNil(t, other)
	require.Zero(t, other.expected.AccountID, "一种模型的成功偏好不得污染另一种模型")
	require.Equal(t, cheap.ID, other.originalID)
}

// 门下失败、客户端断开与取消都不得建立或续期成功偏好。
func TestGatewayStickySuccessOnlyCommitsConfirmedSuccess(t *testing.T) {
	group := gatewayProfitTestGroup(132, PlatformAnthropic)
	cache := &mockGatewayCacheForPlatform{sessionBindings: map[string]int64{}}
	svc := &GatewayService{cache: cache, cfg: testConfig()}
	ctx := svc.BeginGatewayStickySuccess(gatewayProfitTestContext(group), &group.ID, "s", "m")
	require.True(t, GatewayStickySuccessActive(ctx))
	account := gatewayProfitTestAccount(7, PlatformAnthropic, 0.2, group.ID)

	svc.CommitGatewayStickySuccess(ctx, nil, &account, nil, nil)
	svc.CommitGatewayStickySuccess(ctx, nil, &account, &ForwardResult{}, errors.New("upstream 502"))
	svc.CommitGatewayStickySuccess(ctx, nil, &account, &ForwardResult{ClientDisconnect: true}, nil)
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	svc.CommitGatewayStickySuccess(canceled, nil, &account, &ForwardResult{}, nil)
	require.Empty(t, cache.successBindings)
	require.Empty(t, cache.sessionBindings, "失败/断连/取消都不得给账号续期旧绑定")

	svc.CommitGatewayStickySuccess(ctx, nil, &account, &ForwardResult{}, nil)
	first := cache.successBindings[gatewayStickySuccessTestKey(group.ID, "s", "m")]
	require.Equal(t, account.ID, first.AccountID)
	require.NotEmpty(t, first.Revision)

	// 下一个请求里同账号再次成功：按成功规则续期，换 revision。
	nextCtx := svc.BeginGatewayStickySuccess(gatewayProfitTestContext(group), &group.ID, "s", "m")
	require.Equal(t, first, gatewayStickySuccessFromContext(nextCtx).expected)
	svc.CommitGatewayStickySuccess(nextCtx, nil, &account, &ForwardResult{}, nil)
	renewed := cache.successBindings[gatewayStickySuccessTestKey(group.ID, "s", "m")]
	require.Equal(t, account.ID, renewed.AccountID)
	require.NotEqual(t, first.Revision, renewed.Revision, "同账号再次成功必须换 revision")

	// 同会话两个在途请求：先开始的那个晚完成，仍持有更早的 expected，CAS 未命中，
	// 不得覆盖另一个请求已提交的较新偏好。
	stale := gatewayProfitTestAccount(8, PlatformAnthropic, 0.2, group.ID)
	svc.CommitGatewayStickySuccess(ctx, nil, &stale, &ForwardResult{}, nil)
	require.Equal(t, renewed, cache.successBindings[gatewayStickySuccessTestKey(group.ID, "s", "m")],
		"CAS 未命中时晚完成的旧请求不得覆盖较新的成功偏好")
	require.Equal(t, account.ID, cache.sessionBindings["s"], "CAS 未命中时旧键也不得被改写")

	// 分组隔离：另一个分组读不到本分组的偏好。
	otherGroup := gatewayProfitTestGroup(133, PlatformAnthropic)
	otherSvc := &GatewayService{cache: cache, cfg: testConfig()}
	otherCtx := otherSvc.BeginGatewayStickySuccess(gatewayProfitTestContext(otherGroup), &otherGroup.ID, "s", "m")
	require.Zero(t, gatewayStickySuccessFromContext(otherCtx).expected.AccountID)
}

// 无利润门时不装配成功偏好，粘性保持官方原行为。
func TestGatewayStickySuccessNotInstalledWithoutProfitGate(t *testing.T) {
	group := gatewayProfitTestGroup(134, PlatformAnthropic)
	group.ProfitControlEnabled = false
	cache := &mockGatewayCacheForPlatform{sessionBindings: map[string]int64{}}
	svc := &GatewayService{cache: cache, cfg: testConfig()}
	ctx := svc.BeginGatewayStickySuccess(gatewayProfitTestContext(group), &group.ID, "s", "m")
	require.False(t, GatewayStickySuccessActive(ctx))

	// 未标记为 token 计费请求（媒体/元数据路径）同样不装配。
	group.ProfitControlEnabled = true
	plain := context.WithValue(context.Background(), ctxkey.Group, group)
	require.False(t, GatewayStickySuccessActive(svc.BeginGatewayStickySuccess(plain, &group.ID, "s", "m")))
}

// 缓存读失败只影响软偏好：不装配状态，不改变选号与响应。
func TestGatewayStickySuccessReadFailureDegradesSoftly(t *testing.T) {
	group := gatewayProfitTestGroup(135, PlatformAnthropic)
	cache := &failingStickySuccessGatewayCache{
		mockGatewayCacheForPlatform: &mockGatewayCacheForPlatform{sessionBindings: map[string]int64{"s": 9}},
	}
	svc := &GatewayService{cache: cache, cfg: testConfig()}
	ctx := svc.BeginGatewayStickySuccess(gatewayProfitTestContext(group), &group.ID, "s", "m")
	require.False(t, GatewayStickySuccessActive(ctx), "偏好读失败时不装配状态")

	// 未装配状态时候选来源回落旧绑定，不因为门开就丢掉粘性。
	id, err := svc.stickySessionCandidateID(ctx, &group.ID, "s")
	require.NoError(t, err)
	require.Equal(t, int64(9), id)
}

type failingStickySuccessGatewayCache struct {
	*mockGatewayCacheForPlatform
}

func (c *failingStickySuccessGatewayCache) GetGatewayStickySuccess(context.Context, int64, string, string) (GatewayStickySuccessBinding, error) {
	return GatewayStickySuccessBinding{}, errors.New("redis unavailable")
}

type gatewayProfitSnapshotCache struct {
	SchedulerCache
	account *Account
	err     error
}

func (c *gatewayProfitSnapshotCache) GetAccount(context.Context, int64) (*Account, error) {
	return c.account, c.err
}

type gatewayProfitAccountRepo struct {
	AccountRepository
	account *Account
	err     error
}

func (r gatewayProfitAccountRepo) GetByID(context.Context, int64) (*Account, error) {
	return r.account, r.err
}

func TestGatewayProfitControlTerminalRefreshUsesReplacementObject(t *testing.T) {
	selected := gatewayProfitTestAccount(141, PlatformGemini, 0.2, 1)
	replacement := selected
	expensiveRate := 0.8
	replacement.RateMultiplier = &expensiveRate

	snapshot := NewSchedulerSnapshotService(
		&gatewayProfitSnapshotCache{account: &replacement},
		nil,
		gatewayProfitAccountRepo{},
		nil,
		nil,
	)
	ctx := context.WithValue(context.Background(), openAIProfitControlGateCtxKey{}, &openAIProfitControlGate{
		groupID:   1,
		platform:  PlatformGemini,
		threshold: 0.5,
	})

	latest, vetoed, reason := profitControlVetoLatest(ctx, &selected, snapshot)
	require.Same(t, &replacement, latest)
	require.True(t, vetoed)
	require.Equal(t, openAIProfitFilterReasonThreshold, reason)
	require.InDelta(t, 0.2, *selected.RateMultiplier, 1e-12, "测试必须替换缓存对象，不能原地修改旧指针")
}

func TestGatewayProfitControlTerminalRefreshFallsBackFromCacheToDatabase(t *testing.T) {
	selected := gatewayProfitTestAccount(145, PlatformAnthropic, 0.2, 1)
	replacement := selected
	expensiveRate := 0.8
	replacement.RateMultiplier = &expensiveRate

	snapshot := NewSchedulerSnapshotService(
		&gatewayProfitSnapshotCache{err: errors.New("cache unavailable")},
		nil,
		gatewayProfitAccountRepo{account: &replacement},
		nil,
		nil,
	)
	ctx := context.WithValue(context.Background(), openAIProfitControlGateCtxKey{}, &openAIProfitControlGate{
		groupID:   1,
		platform:  PlatformAnthropic,
		threshold: 0.5,
	})

	latest, vetoed, reason := profitControlVetoLatest(ctx, &selected, snapshot)
	require.Same(t, &replacement, latest)
	require.True(t, vetoed, "缓存读取失败时必须继续从数据库重读，不能直接使用选号旧对象")
	require.Equal(t, openAIProfitFilterReasonThreshold, reason)
}

func TestGatewayProfitControlTerminalRefreshFailureFallsBackToSelectedObject(t *testing.T) {
	selected := gatewayProfitTestAccount(151, PlatformAntigravity, 0.2, 1)
	snapshot := NewSchedulerSnapshotService(
		&gatewayProfitSnapshotCache{err: errors.New("cache unavailable")},
		nil,
		gatewayProfitAccountRepo{err: errors.New("database unavailable")},
		nil,
		nil,
	)
	ctx := context.WithValue(context.Background(), openAIProfitControlGateCtxKey{}, &openAIProfitControlGate{
		groupID:   1,
		platform:  PlatformAntigravity,
		threshold: 0.5,
	})

	latest, vetoed, reason := profitControlVetoLatest(ctx, &selected, snapshot)
	require.Same(t, &selected, latest)
	require.False(t, vetoed)
	require.Empty(t, reason)
}

// 选号结果携带门：门安装在调度栈局部 ctx 上，handler 必须经
// ContextWithSelectionProfitGate 重放后终检与准入后绑定才可见（评审修复回归）。
func TestGatewayProfitControlSelectionCarriesGateToHandlerContext(t *testing.T) {
	group := gatewayProfitTestGroup(1, PlatformAnthropic)
	svc := &GatewayService{}
	expensive := gatewayProfitTestAccount(161, PlatformAnthropic, 0.9, group.ID)

	gateCtx := svc.withGatewayProfitControlGate(gatewayProfitTestContext(group), &group.ID)
	selection, err := svc.newSelectionResult(gateCtx, &expensive, true, nil, nil)
	require.NoError(t, err)
	require.True(t, selection.ProfitGateActive(), "选号结果必须携带调度栈内生效的门")

	// 修复前的缺陷形态：handler 原始 ctx 不含门，终检退化为空操作。
	_, vetoed, _ := svc.GatewayProfitControlVetoLatest(context.Background(), &expensive)
	require.False(t, vetoed, "对照组：不重放门时终检确实看不到门")

	handlerCtx := ContextWithSelectionProfitGate(context.Background(), selection)
	latest, vetoed, reason := svc.GatewayProfitControlVetoLatest(handlerCtx, &expensive)
	require.True(t, vetoed, "重放门后终检必须真实生效")
	require.Equal(t, openAIProfitFilterReasonThreshold, reason)
	require.NotNil(t, latest)

	// 无门选号不携带门，重放为无操作。
	plain, err := svc.newSelectionResult(context.Background(), &expensive, true, nil, nil)
	require.NoError(t, err)
	require.False(t, plain.ProfitGateActive())
	require.Equal(t, context.Background(), ContextWithSelectionProfitGate(context.Background(), plain))
}

// 生图意图不关门（H1/H2 回归锚点）：/v1/responses 混合请求即使带生图声明，
// token 定价上下文照常装配，共享门照常安装并否决越线账号。
func TestGatewayProfitControlImageIntentDoesNotDisableGate(t *testing.T) {
	group := gatewayProfitTestGroup(2, PlatformAnthropic)
	svc := &GatewayService{}
	expensive := gatewayProfitTestAccount(162, PlatformAnthropic, 0.9, group.ID)

	ctx := gatewayProfitTestContext(group)
	ctx = WithOpenAIImageGenerationIntent(ctx)
	gateCtx := svc.withGatewayProfitControlGate(ctx, &group.ID)
	require.False(t, svc.isGatewayAccountProfitEligible(gateCtx, &expensive),
		"请求体里的生图声明（含被动 image_gen namespace）不得关闭利润门")
}

// 无门时准入后绑定回退官方 eager 语义；门下未接管成功偏好的调用方（OpenAI 家族
// 自带调度器的入口、count_tokens）逐字保持原语义——门下选号内部不 eager 绑定，
// 终检绑定是它们唯一的写入点。只有被成功偏好接管的会话改为空操作。
func TestGatewayProfitControlAfterAdmissionBindSemantics(t *testing.T) {
	groupID := int64(3)
	expensiveID := int64(171)
	cheapID := int64(172)
	gate := &openAIProfitControlGate{groupID: groupID, platform: PlatformAnthropic, threshold: 0.5}
	gatedCtx := func() context.Context {
		return context.WithValue(context.Background(), openAIProfitControlGateCtxKey{}, gate)
	}

	t.Run("eager without gate", func(t *testing.T) {
		cache := &mockGatewayCacheForPlatform{sessionBindings: map[string]int64{"s": expensiveID}}
		svc := &GatewayService{cache: cache}
		require.NoError(t, svc.BindStickySessionAfterProfitAdmission(context.Background(), &groupID, "s", cheapID))
		require.Equal(t, cheapID, cache.sessionBindings["s"], "无门时保持既有 eager 绑定行为")
	})

	t.Run("gated read failure is conservative", func(t *testing.T) {
		// mock 的 miss 返回非 sentinel 错误，等价于 Redis 读失败：门下保守不写。
		cache := &mockGatewayCacheForPlatform{sessionBindings: map[string]int64{}}
		svc := &GatewayService{cache: cache}
		require.NoError(t, svc.BindStickySessionAfterProfitAdmission(gatedCtx(), &groupID, "absent", cheapID))
		require.NotContains(t, cache.sessionBindings, "absent")
	})

	t.Run("gated sentinel miss binds", func(t *testing.T) {
		cache := &sentinelMissGatewayCache{mockGatewayCacheForPlatform: &mockGatewayCacheForPlatform{sessionBindings: map[string]int64{}}}
		svc := &GatewayService{cache: cache}
		require.NoError(t, svc.BindStickySessionAfterProfitAdmission(gatedCtx(), &groupID, "fresh", cheapID))
		require.Equal(t, cheapID, cache.sessionBindings["fresh"], "门下无既有绑定（sentinel miss）应建立粘性")
	})

	t.Run("gated does not replace a different binding", func(t *testing.T) {
		cache := &sentinelMissGatewayCache{mockGatewayCacheForPlatform: &mockGatewayCacheForPlatform{sessionBindings: map[string]int64{"s": expensiveID}}}
		svc := &GatewayService{cache: cache}
		require.NoError(t, svc.BindStickySessionAfterProfitAdmission(gatedCtx(), &groupID, "s", cheapID))
		require.Equal(t, expensiveID, cache.sessionBindings["s"], "未接管的门下调用方保持「不覆盖既有不同绑定」")
	})

	t.Run("managed session never binds", func(t *testing.T) {
		group := gatewayProfitTestGroup(groupID, PlatformAnthropic)
		cache := &sentinelMissGatewayCache{mockGatewayCacheForPlatform: &mockGatewayCacheForPlatform{sessionBindings: map[string]int64{"s": expensiveID}}}
		svc := &GatewayService{cache: cache}
		managed := svc.BeginGatewayStickySuccess(gatewayProfitTestContext(group), &groupID, "s", "m")
		require.True(t, gatewayStickySuccessManaged(managed, &groupID, "s"))
		require.NoError(t, svc.BindStickySessionAfterProfitAdmission(managed, &groupID, "s", cheapID))
		require.Equal(t, expensiveID, cache.sessionBindings["s"], "成功偏好接管时终检通过不得覆盖既有绑定")
		require.NoError(t, svc.BindStickySessionAfterProfitAdmission(managed, &groupID, "s", expensiveID))
		require.Equal(t, expensiveID, cache.sessionBindings["s"])

		// 同一个请求里换一个会话哈希就不在接管范围内：最小边界按会话哈希判定，
		// 不因为装配过状态就把整条请求的所有绑定点都改掉。
		require.False(t, gatewayStickySuccessManaged(managed, &groupID, "s2"))
		require.NoError(t, svc.BindStickySessionAfterProfitAdmission(managed, &groupID, "s2", cheapID))
		require.Equal(t, cheapID, cache.sessionBindings["s2"], "未接管的会话仍按原门下语义建立绑定")
	})
}

// OpenAI 家族在非 OpenAI 平台（grok/kimi/deepseek/minimax/zhipu 等）不装配通用
// 成功偏好：这些分组开启利润控制后，终检绑定必须仍然是有效的旧键写入点，
// 否则门下会彻底失去粘性。
func TestGatewayProfitControlAfterAdmissionStillBindsForUnmanagedPlatforms(t *testing.T) {
	group := gatewayProfitTestGroup(136, PlatformGrok)
	cache := &sentinelMissGatewayCache{mockGatewayCacheForPlatform: &mockGatewayCacheForPlatform{sessionBindings: map[string]int64{}}}
	svc := &GatewayService{cache: cache, cfg: testConfig()}

	// OpenAI 家族入口不调用 BeginGatewayStickySuccess，ctx 里只有利润门。
	ctx := svc.withGatewayProfitControlGate(gatewayProfitTestContext(group), &group.ID)
	require.True(t, gatewayProfitControlGateActive(ctx), "grok 分组同样受利润门保护")
	require.False(t, gatewayStickySuccessManaged(ctx, &group.ID, "grok-session"))

	require.NoError(t, svc.BindStickySessionAfterProfitAdmission(ctx, &group.ID, "grok-session", 91))
	require.Equal(t, int64(91), cache.sessionBindings["grok-session"],
		"未接入成功偏好的平台在门下必须保留终检绑定这个唯一写入点")

	// 既有绑定仍然不被替换：原门下语义逐字保留。
	require.NoError(t, svc.BindStickySessionAfterProfitAdmission(ctx, &group.ID, "grok-session", 92))
	require.Equal(t, int64(91), cache.sessionBindings["grok-session"])
	require.Zero(t, len(cache.successBindings), "未接管路径不得写入成功偏好")
}

// sentinelMissGatewayCache 让 miss 返回与真实仓库一致的 ErrStickySessionNotFound。
type sentinelMissGatewayCache struct {
	*mockGatewayCacheForPlatform
}

func (c *sentinelMissGatewayCache) GetSessionAccountID(ctx context.Context, groupID int64, sessionHash string) (int64, error) {
	if id, ok := c.sessionBindings[sessionHash]; ok {
		return id, nil
	}
	return 0, ErrStickySessionNotFound
}

// 成功偏好状态必须绑定实际生效的分组：handler 的兜底分组循环、prompt-too-long
// 切换与调度器内部的 Claude Code 降级都会在同一个请求 ctx 上换分组，上一分组的
// 状态既不能作为候选来源，也不能被成功提交写回。
func TestGatewayStickySuccessStateDoesNotCrossGroups(t *testing.T) {
	const session = "cross-group-session"
	const model = "claude-test"

	t.Run("fallback group with gate closed clears the previous state", func(t *testing.T) {
		primary := gatewayProfitTestGroup(201, PlatformAnthropic)
		svc := &GatewayService{cache: &mockGatewayCacheForPlatform{}, cfg: testConfig()}
		ctx := svc.BeginGatewayStickySuccess(gatewayProfitTestContext(primary), &primary.ID, session, model)
		require.True(t, GatewayStickySuccessActive(ctx))

		fallback := gatewayProfitTestGroup(202, PlatformAnthropic)
		fallback.ProfitControlEnabled = false
		ctx = context.WithValue(ctx, ctxkey.Group, fallback)
		ctx = svc.BeginGatewayStickySuccess(ctx, &fallback.ID, session, model)
		require.False(t, GatewayStickySuccessActive(ctx), "门关的兜底分组必须清除上一分组的状态")
		require.False(t, gatewayStickySuccessManaged(ctx, &fallback.ID, session))
		require.False(t, gatewayStickySuccessManaged(ctx, &primary.ID, session))

		// 其余 context value 保留：清除只针对成功偏好这一个 key。
		group, ok := ctx.Value(ctxkey.Group).(*Group)
		require.True(t, ok)
		require.Equal(t, fallback.ID, group.ID)
	})

	t.Run("会话或模型为空同样清除", func(t *testing.T) {
		primary := gatewayProfitTestGroup(203, PlatformAnthropic)
		svc := &GatewayService{cache: &mockGatewayCacheForPlatform{}, cfg: testConfig()}
		ctx := svc.BeginGatewayStickySuccess(gatewayProfitTestContext(primary), &primary.ID, session, model)
		require.True(t, GatewayStickySuccessActive(ctx))
		require.False(t, GatewayStickySuccessActive(svc.BeginGatewayStickySuccess(ctx, &primary.ID, "", model)))
		require.False(t, GatewayStickySuccessActive(svc.BeginGatewayStickySuccess(ctx, &primary.ID, session, "")))
	})

	t.Run("偏好读失败同样清除", func(t *testing.T) {
		primary := gatewayProfitTestGroup(204, PlatformAnthropic)
		svc := &GatewayService{cache: &mockGatewayCacheForPlatform{}, cfg: testConfig()}
		ctx := svc.BeginGatewayStickySuccess(gatewayProfitTestContext(primary), &primary.ID, session, model)
		require.True(t, GatewayStickySuccessActive(ctx))

		failing := &GatewayService{
			cache: &failingStickySuccessGatewayCache{mockGatewayCacheForPlatform: &mockGatewayCacheForPlatform{}},
			cfg:   testConfig(),
		}
		fallback := gatewayProfitTestGroup(205, PlatformAnthropic)
		ctx = context.WithValue(ctx, ctxkey.Group, fallback)
		require.False(t, GatewayStickySuccessActive(failing.BeginGatewayStickySuccess(ctx, &fallback.ID, session, model)))
	})

	t.Run("fallback group with gate open isolates both keys", func(t *testing.T) {
		primary := gatewayProfitTestGroup(206, PlatformAnthropic)
		fallback := gatewayProfitTestGroup(207, PlatformAnthropic)
		cache := &mockGatewayCacheForPlatform{sessionBindings: map[string]int64{}}
		svc := &GatewayService{cache: cache, cfg: testConfig()}
		account := gatewayProfitTestAccount(11, PlatformAnthropic, 0.2, primary.ID)

		// 主分组成功一次。
		primaryCtx := svc.BeginGatewayStickySuccess(gatewayProfitTestContext(primary), &primary.ID, session, model)
		svc.CommitGatewayStickySuccess(primaryCtx, nil, &account, &ForwardResult{}, nil)
		require.Equal(t, account.ID, cache.successBindings[gatewayStickySuccessTestKey(primary.ID, session, model)].AccountID)

		// prompt-too-long 切到同样开门的兜底分组：新一轮 Begin 以兜底分组重建状态。
		fallbackCtx := context.WithValue(primaryCtx, ctxkey.Group, fallback)
		fallbackCtx = svc.BeginGatewayStickySuccess(fallbackCtx, &fallback.ID, session, model)
		state := gatewayStickySuccessFromContext(fallbackCtx)
		require.NotNil(t, state)
		require.Equal(t, fallback.ID, state.groupID, "状态必须绑定兜底分组")
		require.Zero(t, state.expected.AccountID, "兜底分组读不到主分组的偏好")

		other := gatewayProfitTestAccount(12, PlatformAnthropic, 0.2, fallback.ID)
		svc.CommitGatewayStickySuccess(fallbackCtx, nil, &other, &ForwardResult{}, nil)
		require.Equal(t, other.ID, cache.successBindings[gatewayStickySuccessTestKey(fallback.ID, session, model)].AccountID)
		require.Equal(t, account.ID, cache.successBindings[gatewayStickySuccessTestKey(primary.ID, session, model)].AccountID,
			"兜底分组成功不得回写主分组偏好")
	})

	t.Run("scheduler downgrade to another group is neither read nor written", func(t *testing.T) {
		primary := gatewayProfitTestGroup(208, PlatformAnthropic)
		downgraded := gatewayProfitTestGroup(209, PlatformAnthropic)
		cache := &mockGatewayCacheForPlatform{
			sessionBindings: map[string]int64{session: 31},
			successBindings: map[string]GatewayStickySuccessBinding{
				gatewayStickySuccessTestKey(primary.ID, session, model): {AccountID: 31, Revision: "primary"},
			},
		}
		svc := &GatewayService{cache: cache, cfg: testConfig()}
		ctx := svc.BeginGatewayStickySuccess(gatewayProfitTestContext(primary), &primary.ID, session, model)
		require.True(t, gatewayStickySuccessManaged(ctx, &primary.ID, session))

		// 读取侧：调度器降级到另一分组后，原分组状态不得作为候选来源。
		require.False(t, gatewayStickySuccessManaged(ctx, &downgraded.ID, session))
		id, err := svc.stickySessionCandidateID(ctx, &downgraded.ID, session)
		require.NoError(t, err)
		require.Equal(t, int64(31), id, "未接管时回落到旧绑定读取，语义与修复前一致")

		// 提交侧：选号结果报告生效分组是降级分组 → 不提交，也不写旧键。
		selection := &AccountSelectionResult{effectiveGroupID: downgraded.ID}
		effective, known := selection.EffectiveGroupID()
		require.True(t, known)
		require.Equal(t, downgraded.ID, effective)
		account := gatewayProfitTestAccount(32, PlatformAnthropic, 0.2, downgraded.ID)
		svc.CommitGatewayStickySuccess(ctx, selection, &account, &ForwardResult{}, nil)
		require.Equal(t, int64(31), cache.successBindings[gatewayStickySuccessTestKey(primary.ID, session, model)].AccountID,
			"降级分组的成功不得写回原分组偏好")
		require.Empty(t, cache.successBindings[gatewayStickySuccessTestKey(downgraded.ID, session, model)].AccountID,
			"也不得凭原分组状态写降级分组的偏好")
		require.Equal(t, int64(31), cache.sessionBindings[session], "旧键同样不得被写错组")

		// 分组一致时照常提交。
		matching := &AccountSelectionResult{effectiveGroupID: primary.ID}
		svc.CommitGatewayStickySuccess(ctx, matching, &account, &ForwardResult{}, nil)
		require.Equal(t, account.ID, cache.successBindings[gatewayStickySuccessTestKey(primary.ID, session, model)].AccountID)
	})

	t.Run("shared account across two groups does not leak the preference", func(t *testing.T) {
		groupA := gatewayProfitTestGroup(210, PlatformAnthropic)
		groupB := gatewayProfitTestGroup(211, PlatformAnthropic)
		shared := gatewayProfitTestAccount(41, PlatformAnthropic, 0.2, groupA.ID)
		shared.AccountGroups = append(shared.AccountGroups, AccountGroup{AccountID: shared.ID, GroupID: groupB.ID})
		shared.GroupIDs = append(shared.GroupIDs, groupB.ID)
		// 旧键在仓库层是按分组建键的（buildSessionKey），这里用同样按分组建键的
		// 替身，避免 mockGatewayCacheForPlatform 只按会话建键造成的假通过。
		cache := newGroupScopedStickyCache()
		svc := &GatewayService{cache: cache, cfg: testConfig()}

		ctxA := svc.BeginGatewayStickySuccess(gatewayProfitTestContext(groupA), &groupA.ID, session, model)
		svc.CommitGatewayStickySuccess(ctxA, &AccountSelectionResult{effectiveGroupID: groupA.ID}, &shared, &ForwardResult{}, nil)
		require.Equal(t, shared.ID, cache.successBindings[gatewayStickySuccessTestKey(groupA.ID, session, model)].AccountID)
		require.Equal(t, shared.ID, cache.binding(groupA.ID, session))

		// 同一账号也在 B 组，但 B 组读不到 A 组的偏好，也读不到 A 组的旧键。
		ctxB := svc.BeginGatewayStickySuccess(gatewayProfitTestContext(groupB), &groupB.ID, session, model)
		stateB := gatewayStickySuccessFromContext(ctxB)
		require.NotNil(t, stateB)
		require.Equal(t, groupB.ID, stateB.groupID)
		require.Zero(t, stateB.expected.AccountID)
		candidate, managed := gatewayStickySuccessCandidate(ctxB, &groupB.ID, session)
		require.True(t, managed)
		require.Zero(t, candidate, "共享账号不得让 A 组的粘性泄漏成 B 组的候选")
		require.Empty(t, cache.successBindings[gatewayStickySuccessTestKey(groupB.ID, session, model)].AccountID)

		// B 组自己成功后，两组的偏好与旧键各自独立。
		svc.CommitGatewayStickySuccess(ctxB, &AccountSelectionResult{effectiveGroupID: groupB.ID}, &shared, &ForwardResult{}, nil)
		require.Equal(t, shared.ID, cache.binding(groupB.ID, session))
		require.Equal(t, shared.ID, cache.successBindings[gatewayStickySuccessTestKey(groupB.ID, session, model)].AccountID)
		require.NotEqual(t,
			cache.successBindings[gatewayStickySuccessTestKey(groupA.ID, session, model)].Revision,
			cache.successBindings[gatewayStickySuccessTestKey(groupB.ID, session, model)].Revision,
			"两组的偏好是两条独立记录")
	})
}

// groupScopedStickyCache 按 (group, session) 建旧粘性键，与仓库层 buildSessionKey
// 的布局一致；mockGatewayCacheForPlatform 只按会话建键，不能用来验证分组隔离。
type groupScopedStickyCache struct {
	*mockGatewayCacheForPlatform
	scoped map[string]int64
}

func newGroupScopedStickyCache() *groupScopedStickyCache {
	return &groupScopedStickyCache{
		mockGatewayCacheForPlatform: &mockGatewayCacheForPlatform{sessionBindings: map[string]int64{}},
		scoped:                      map[string]int64{},
	}
}

func (c *groupScopedStickyCache) key(groupID int64, sessionHash string) string {
	return fmt.Sprintf("%d/%s", groupID, sessionHash)
}

func (c *groupScopedStickyCache) binding(groupID int64, sessionHash string) int64 {
	return c.scoped[c.key(groupID, sessionHash)]
}

func (c *groupScopedStickyCache) GetSessionAccountID(_ context.Context, groupID int64, sessionHash string) (int64, error) {
	if id, ok := c.scoped[c.key(groupID, sessionHash)]; ok {
		return id, nil
	}
	return 0, ErrStickySessionNotFound
}

func (c *groupScopedStickyCache) SetSessionAccountID(_ context.Context, groupID int64, sessionHash string, accountID int64, _ time.Duration) error {
	c.scoped[c.key(groupID, sessionHash)] = accountID
	return nil
}

func (c *groupScopedStickyCache) DeleteSessionAccountID(_ context.Context, groupID int64, sessionHash string) error {
	delete(c.scoped, c.key(groupID, sessionHash))
	return nil
}

// countingStickySuccessCache 统计成功偏好的读取次数。
type countingStickySuccessCache struct {
	*mockGatewayCacheForPlatform
	reads int
}

func (c *countingStickySuccessCache) GetGatewayStickySuccess(ctx context.Context, groupID int64, sessionHash, model string) (GatewayStickySuccessBinding, error) {
	c.reads++
	return c.mockGatewayCacheForPlatform.GetGatewayStickySuccess(ctx, groupID, sessionHash, model)
}

// CAS 的期望版本必须在实际分组的选号前确定，并在同一轮 failover 中保持不变：
// 同一请求内同一 (有效分组, 会话, 模型) 只读一次 expected，中途换号重选不得
// 重读到更新的 revision，否则晚到覆盖保护会被削弱。
func TestGatewayStickySuccessReadsExpectedOncePerRequestKey(t *testing.T) {
	group := gatewayProfitTestGroup(212, PlatformAnthropic)
	cache := &countingStickySuccessCache{
		mockGatewayCacheForPlatform: &mockGatewayCacheForPlatform{
			sessionBindings: map[string]int64{},
			successBindings: map[string]GatewayStickySuccessBinding{
				gatewayStickySuccessTestKey(group.ID, "s", "m"): {AccountID: 51, Revision: "v1"},
			},
		},
	}
	svc := &GatewayService{cache: cache, cfg: testConfig()}
	gated := svc.withGatewayProfitControlGate(gatewayProfitTestContext(group), &group.ID)

	ctx := svc.armGatewayStickySuccess(gated, &group.ID, "s", "m")
	first := gatewayStickySuccessFromContext(ctx)
	require.NotNil(t, first)
	require.Equal(t, "v1", first.expected.Revision)
	require.Equal(t, 1, cache.reads)

	// 另一个请求在本轮 failover 期间提交了新偏好。
	cache.successBindings[gatewayStickySuccessTestKey(group.ID, "s", "m")] = GatewayStickySuccessBinding{AccountID: 52, Revision: "v2"}

	// 同键重装：复用同一份状态，不重读。
	again := svc.armGatewayStickySuccess(ctx, &group.ID, "s", "m")
	require.Same(t, first, gatewayStickySuccessFromContext(again))
	require.Equal(t, 1, cache.reads, "同一请求同一键不得重复读取 expected")
	require.Equal(t, "v1", gatewayStickySuccessFromContext(again).expected.Revision)

	// 本请求此时提交必须因 CAS 未命中而放弃：晚到保护仍然有效。
	account := gatewayProfitTestAccount(53, PlatformAnthropic, 0.2, group.ID)
	svc.CommitGatewayStickySuccess(again, &AccountSelectionResult{effectiveGroupID: group.ID}, &account, &ForwardResult{}, nil)
	require.Equal(t, int64(52), cache.successBindings[gatewayStickySuccessTestKey(group.ID, "s", "m")].AccountID,
		"持有旧 expected 的请求不得覆盖已提交的较新偏好")

	// 换模型或换分组是另一个键，必须重新读。
	svc.armGatewayStickySuccess(ctx, &group.ID, "s", "other-model")
	require.Equal(t, 2, cache.reads)
	other := gatewayProfitTestGroup(213, PlatformAnthropic)
	svc.armGatewayStickySuccess(svc.withGatewayProfitControlGate(gatewayProfitTestContext(other), &other.ID), &other.ID, "s", "m")
	require.Equal(t, 3, cache.reads)

	// 门关（例如降级到未开利润控制的分组）时清除状态，也不读。
	require.False(t, GatewayStickySuccessActive(svc.armGatewayStickySuccess(context.Background(), &group.ID, "s", "m")))
	require.Equal(t, 3, cache.reads)
}

// 身份模型（渠道映射前的客户端请求模型）优先于调度模型（账号资格/渠道映射/出站
// 所用模型）决定成功偏好键；ctx 没有登记身份时才回落到调度模型。
//
// 这钉住本 PR 的模型维度约束：Gemini 原生入口传给调度器的就是映射后的调度模型，
// composite 路由也会在调度栈内改写它，都不得改变粘性键。
func TestGatewayStickySuccessKeyPrefersIdentityModelOverSchedulingModel(t *testing.T) {
	group := gatewayProfitTestGroup(214, PlatformAnthropic)
	cache := &countingStickySuccessCache{
		mockGatewayCacheForPlatform: &mockGatewayCacheForPlatform{
			sessionBindings: map[string]int64{},
			successBindings: map[string]GatewayStickySuccessBinding{
				gatewayStickySuccessTestKey(group.ID, "s", "alias-a"):  {AccountID: 61, Revision: "ra"},
				gatewayStickySuccessTestKey(group.ID, "s", "upstream"): {AccountID: 62, Revision: "ru"},
			},
		},
	}
	svc := &GatewayService{cache: cache, cfg: testConfig()}
	gated := svc.withGatewayProfitControlGate(gatewayProfitTestContext(group), &group.ID)

	// 登记身份模型后，即使调度模型是映射后的 "upstream"，键仍按身份模型 "alias-a"。
	identity := WithGatewayStickyIdentityModel(gated, "alias-a")
	ctx := svc.armGatewayStickySuccess(identity, &group.ID, "s", "upstream")
	state := gatewayStickySuccessFromContext(ctx)
	require.NotNil(t, state)
	require.Equal(t, "alias-a", state.model, "粘性键必须用身份模型")
	require.Equal(t, int64(61), state.originalID)
	require.Equal(t, "ra", state.expected.Revision)
	require.Equal(t, 1, cache.reads)

	// 调度栈内层用另一个调度模型重复装配（composite 改写 / 渠道映射）：身份未变，
	// 必须原样复用同一份状态，不换键、不重读 expected。
	inner := svc.armGatewayStickySuccess(ctx, &group.ID, "s", "another-upstream")
	require.Same(t, state, gatewayStickySuccessFromContext(inner))
	require.Equal(t, 1, cache.reads, "同一身份模型在内层重复选号时不得重读 expected")

	// 提交按身份模型的键写入，映射后的调度模型键不受影响。
	account := gatewayProfitTestAccount(63, PlatformAnthropic, 0.2, group.ID)
	svc.CommitGatewayStickySuccess(inner, &AccountSelectionResult{effectiveGroupID: group.ID}, &account, &ForwardResult{}, nil)
	require.Equal(t, int64(63), cache.successBindings[gatewayStickySuccessTestKey(group.ID, "s", "alias-a")].AccountID)
	require.Equal(t, int64(62), cache.successBindings[gatewayStickySuccessTestKey(group.ID, "s", "upstream")].AccountID,
		"另一个模型的偏好不得被覆盖")

	// 另一个身份模型是另一条键：必须重新读 expected。
	svc.armGatewayStickySuccess(WithGatewayStickyIdentityModel(gated, "alias-b"), &group.ID, "s", "upstream")
	require.Equal(t, 2, cache.reads)

	// 没有登记身份模型时逐字回落到调度模型（web search 等调用方行为不变）。
	fallback := svc.armGatewayStickySuccess(gated, &group.ID, "s", "upstream")
	fallbackState := gatewayStickySuccessFromContext(fallback)
	require.NotNil(t, fallbackState)
	require.Equal(t, "upstream", fallbackState.model)
	require.Equal(t, int64(62), fallbackState.originalID)

	// 空身份模型不覆盖已登记的身份，也不新建载体。
	require.Equal(t, "alias-a", gatewayStickySuccessModel(WithGatewayStickyIdentityModel(identity, "   "), "upstream"))
	require.Equal(t, "upstream", gatewayStickySuccessModel(gated, " upstream "))
}
