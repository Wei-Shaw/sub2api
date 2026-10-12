//go:build unit

package service

// composite 账号池 per-attempt 平台策略的 service 侧单元测试：
//  1. CheckUserPlatformQuotaEligibilityForRequest：复现 CheckBillingEligibility 的
//     豁免条件（simple / 订阅），standard 模式下按平台拦截或放行；
//  2. CompositePoolAttemptChannelRestricted / CompositePoolAttemptChannelMapping：
//     attempt 局部 ctx（ResolvedTargetPlatform=选中平台）把渠道限制与映射作用域
//     收窄到选中平台——修复池请求无 resolved 时按 composite 分组跨平台行匹配而
//     绕过渠道映射/定价限制的缺陷。

import (
	"context"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

// ── CheckUserPlatformQuotaEligibilityForRequest ─────────────────────────────

func newPoolAttemptBillingService(repo UserPlatformQuotaRepository, cache BillingCache, runMode string) *BillingCacheService {
	cfg := &config.Config{RunMode: runMode}
	cfg.Billing.UserPlatformQuotaCacheTTLSeconds = 60
	return &BillingCacheService{
		cache:                 cache,
		cfg:                   cfg,
		userPlatformQuotaRepo: repo,
	}
}

func TestCheckUserPlatformQuotaEligibilityForRequest_SimpleModeSkips(t *testing.T) {
	// fakeZeroQuotaCache 在被查询时必然拦截（limit=0）：simple 模式必须跳过检查。
	fake := &fakeZeroQuotaCache{}
	s := newPoolAttemptBillingService(&fakeQuotaRepo{}, fake, config.RunModeSimple)

	err := s.CheckUserPlatformQuotaEligibilityForRequest(
		context.Background(), &User{ID: 42}, &Group{ID: 10, Platform: PlatformComposite}, nil, "openai")

	require.NoError(t, err)
	require.False(t, fake.called, "simple mode must not consult user×platform quota cache")
}

func TestCheckUserPlatformQuotaEligibilityForRequest_SubscriptionModeSkips(t *testing.T) {
	fake := &fakeZeroQuotaCache{}
	s := newPoolAttemptBillingService(&fakeQuotaRepo{}, fake, config.RunModeStandard)

	group := &Group{ID: 10, Platform: PlatformComposite, SubscriptionType: "subscription", Status: StatusActive}
	err := s.CheckUserPlatformQuotaEligibilityForRequest(
		context.Background(), &User{ID: 42}, group, &UserSubscription{Status: SubscriptionStatusActive}, "openai")

	require.NoError(t, err)
	require.False(t, fake.called, "subscription mode must bypass user×platform quota (same as CheckBillingEligibility)")
}

func TestCheckUserPlatformQuotaEligibilityForRequest_SubscriptionGroupWithoutSubscriptionChecks(t *testing.T) {
	// 订阅分组但 subscription 为 nil：与 CheckBillingEligibility 的 isSubscriptionMode
	// 判定一致（group.IsSubscriptionType() && subscription != nil），回落 standard 检查。
	fake := &fakeZeroQuotaCache{}
	s := newPoolAttemptBillingService(&fakeQuotaRepo{}, fake, config.RunModeStandard)

	group := &Group{ID: 10, Platform: PlatformComposite, SubscriptionType: "subscription", Status: StatusActive}
	err := s.CheckUserPlatformQuotaEligibilityForRequest(
		context.Background(), &User{ID: 42}, group, nil, "openai")

	require.ErrorIs(t, err, ErrUserPlatformDailyQuotaExhausted)
	require.True(t, fake.called)
}

func TestCheckUserPlatformQuotaEligibilityForRequest_StandardBlocksExhaustedPlatform(t *testing.T) {
	fake := &fakeZeroQuotaCache{}
	s := newPoolAttemptBillingService(&fakeQuotaRepo{}, fake, config.RunModeStandard)

	err := s.CheckUserPlatformQuotaEligibilityForRequest(
		context.Background(), &User{ID: 42}, &Group{ID: 10, Platform: PlatformComposite}, nil, "openai")

	require.ErrorIs(t, err, ErrUserPlatformDailyQuotaExhausted)
	require.True(t, fake.called, "standard mode must consult the user×platform quota cache")
}

func TestCheckUserPlatformQuotaEligibilityForRequest_StandardAllowsBelowLimit(t *testing.T) {
	dailyLimit := 5.0
	now := time.Now().UTC()
	cache := &fakeFullCache{entry: &UserPlatformQuotaCacheEntry{
		DailyUsageUSD:    1.0,
		DailyLimitUSD:    &dailyLimit,
		DailyWindowStart: &now,
		SchemaVersion:    UserPlatformQuotaCacheSchemaV1,
	}}
	s := newPoolAttemptBillingService(&fakeQuotaRepo{}, cache, config.RunModeStandard)

	err := s.CheckUserPlatformQuotaEligibilityForRequest(
		context.Background(), &User{ID: 42}, &Group{ID: 10, Platform: PlatformComposite}, nil, "openai")

	require.NoError(t, err)
}

func TestCheckUserPlatformQuotaEligibilityForRequest_EmptyPlatformAllows(t *testing.T) {
	fake := &fakeZeroQuotaCache{}
	s := newPoolAttemptBillingService(&fakeQuotaRepo{}, fake, config.RunModeStandard)

	err := s.CheckUserPlatformQuotaEligibilityForRequest(
		context.Background(), &User{ID: 42}, &Group{ID: 10, Platform: PlatformComposite}, nil, "")

	require.NoError(t, err)
	require.False(t, fake.called)
}

// ── CompositePoolAttemptChannelRestricted / CompositePoolAttemptChannelMapping ──

const poolAttemptGroupID = int64(4310)

// newPoolAttemptChannelService 构造 composite 分组的渠道服务：cache 装填时按映射
// 条目自身平台展开（expandMappingToCache），查找作用域由 ctx 的 ResolvedTargetPlatform
// 决定（channelLookupPlatform）。
func newPoolAttemptChannelService(t *testing.T, ch Channel) *ChannelService {
	t.Helper()
	repo := makeStandardRepo(ch, map[int64]string{poolAttemptGroupID: PlatformComposite})
	svc := newTestChannelService(repo)
	// 预热 cache（lazy load 在首次查询时也会构建，这里显式触发以便失败立现）。
	if _, err := svc.loadCache(context.Background()); err != nil {
		t.Fatalf("load channel cache: %v", err)
	}
	return svc
}

func newPoolAttemptOpenAIGateway(channelSvc *ChannelService) *OpenAIGatewayService {
	return &OpenAIGatewayService{channelService: channelSvc}
}

func poolAttemptCtx(platform string) context.Context {
	return WithResolvedTargetPlatform(context.Background(), platform)
}

func TestCompositePoolAttemptChannelRestricted_ScopesToSelectedPlatform(t *testing.T) {
	// 渠道只对 openai 平台声明 gpt-* 定价且开启模型限制。
	ch := Channel{
		ID:                 1,
		Status:             StatusActive,
		GroupIDs:           []int64{poolAttemptGroupID},
		RestrictModels:     true,
		BillingModelSource: BillingModelSourceRequested,
		ModelPricing: []ChannelModelPricing{
			{Platform: PlatformOpenAI, Models: []string{"gpt-*"}},
		},
	}
	svc := newPoolAttemptOpenAIGateway(newPoolAttemptChannelService(t, ch))
	groupID := poolAttemptGroupID

	// 选中 openai 账号：gpt-4 在 openai 作用域定价内 → 放行。
	require.False(t, svc.CompositePoolAttemptChannelRestricted(
		poolAttemptCtx(PlatformOpenAI), &groupID, &Account{Platform: PlatformOpenAI}, "gpt-4", false))

	// 选中 deepseek 账号：gpt-4 不在 deepseek 作用域定价内 → 拒绝该平台。
	// （无 resolved 的旧作用域会跨平台扫到 openai 行而误放行——即本修复目标。）
	require.True(t, svc.CompositePoolAttemptChannelRestricted(
		poolAttemptCtx(PlatformDeepseek), &groupID, &Account{Platform: PlatformDeepseek}, "gpt-4", false))

	// 无 attempt ctx（池原 ctx，无 resolved）：复现旧缺陷语义作对照——跨平台扫描
	// 把 openai 行的 gpt-* 当作命中，deepseek 账号被误放行。
	require.False(t, svc.CompositePoolAttemptChannelRestricted(
		context.Background(), &groupID, &Account{Platform: PlatformDeepseek}, "gpt-4", false))
}

func TestCompositePoolAttemptChannelRestricted_UpstreamBillingSourcePerAccount(t *testing.T) {
	// BillingModelSource=upstream：调度阶段 checkChannelPricingRestriction 不判
	// （billingModel 为空），改由逐账号 upstream 模型检查。attempt 作用域同样适用。
	ch := Channel{
		ID:                 1,
		Status:             StatusActive,
		GroupIDs:           []int64{poolAttemptGroupID},
		RestrictModels:     true,
		BillingModelSource: BillingModelSourceUpstream,
		ModelPricing: []ChannelModelPricing{
			{Platform: PlatformDeepseek, Models: []string{"deepseek-chat"}},
		},
	}
	svc := newPoolAttemptOpenAIGateway(newPoolAttemptChannelService(t, ch))
	groupID := poolAttemptGroupID
	openAIAccount := &Account{
		Platform: PlatformOpenAI, Type: AccountTypeAPIKey,
		Credentials: map[string]any{
			"api_key": "k", "base_url": "https://openai-fake.test/v1",
			"model_mapping": map[string]any{"gpt-4": "deepseek-chat"},
		},
	}

	// openai 账号把 gpt-4 映射为 deepseek-chat，但 openai 作用域没有该定价 → 拒绝。
	require.True(t, svc.CompositePoolAttemptChannelRestricted(
		poolAttemptCtx(PlatformOpenAI), &groupID, openAIAccount, "gpt-4", false))

	// deepseek 作用域存在 deepseek-chat 定价 → 放行。
	deepseekAccount := &Account{
		Platform: PlatformDeepseek, Type: AccountTypeAPIKey,
		Credentials: map[string]any{
			"api_key": "k", "base_url": "https://deepseek-fake.test/v1",
			"model_mapping": map[string]any{"gpt-4": "deepseek-chat"},
		},
	}
	require.False(t, svc.CompositePoolAttemptChannelRestricted(
		poolAttemptCtx(PlatformDeepseek), &groupID, deepseekAccount, "gpt-4", false))
}

func TestCompositePoolAttemptChannelMapping_ScopesToSelectedPlatform(t *testing.T) {
	ch := Channel{
		ID:                 1,
		Status:             StatusActive,
		GroupIDs:           []int64{poolAttemptGroupID},
		BillingModelSource: BillingModelSourceRequested,
		ModelMapping: map[string]map[string]string{
			PlatformOpenAI:   {"gpt-4": "gpt-4o"},
			PlatformDeepseek: {"deepseek-flash": "deepseek-chat"},
		},
	}
	svc := newPoolAttemptOpenAIGateway(newPoolAttemptChannelService(t, ch))
	groupID := poolAttemptGroupID

	openAIMapping := svc.CompositePoolAttemptChannelMapping(poolAttemptCtx(PlatformOpenAI), &groupID, "gpt-4")
	require.True(t, openAIMapping.Mapped)
	require.Equal(t, "gpt-4o", openAIMapping.MappedModel)

	// 同一模型在 deepseek 作用域没有映射 → 保持原模型直通，不跨平台串 openai 行。
	deepseekMapping := svc.CompositePoolAttemptChannelMapping(poolAttemptCtx(PlatformDeepseek), &groupID, "gpt-4")
	require.False(t, deepseekMapping.Mapped)
	require.Equal(t, "gpt-4", deepseekMapping.MappedModel)
}

func TestCompositePoolAttemptChannelHelpers_NilGuards(t *testing.T) {
	var svc *OpenAIGatewayService
	groupID := poolAttemptGroupID
	require.False(t, svc.CompositePoolAttemptChannelRestricted(context.Background(), &groupID, &Account{Platform: PlatformOpenAI}, "gpt-4", false))
	mapping := svc.CompositePoolAttemptChannelMapping(context.Background(), &groupID, "gpt-4")
	require.False(t, mapping.Mapped)
	require.Equal(t, "gpt-4", mapping.MappedModel)
}

// ── deniedPlatforms selector 门（sticky / list） ─────────────────────────────
//
// 复审 A 收尾：deny 不得收缩候选池（候选池 immutable），而是经请求局部
// deniedPlatforms 在 selector list/sticky 门过滤；sticky 账号因所属平台被 deny
// 暂时排除时必须按 excluded 语义早退——不 delete 粘性绑定（即使该账号不在
// excludedIDs / failedAccountIDs 里）。

type compositePoolGateStickyCache struct {
	GatewayCache
	stickyID int64
	deletes  int
}

func (f *compositePoolGateStickyCache) GetSessionAccountID(context.Context, int64, string) (int64, error) {
	if f.stickyID > 0 {
		return f.stickyID, nil
	}
	return 0, ErrStickySessionNotFound
}

func (f *compositePoolGateStickyCache) SetSessionAccountID(context.Context, int64, string, int64, time.Duration) error {
	return nil
}

func (f *compositePoolGateStickyCache) RefreshSessionTTL(context.Context, int64, string, time.Duration) error {
	return nil
}

func (f *compositePoolGateStickyCache) DeleteSessionAccountID(context.Context, int64, string) error {
	f.deletes++
	return nil
}

type compositePoolGateAccountRepo struct {
	AccountRepository
	byID map[int64]*Account
}

func (f *compositePoolGateAccountRepo) GetByID(_ context.Context, id int64) (*Account, error) {
	if account, ok := f.byID[id]; ok {
		copy := *account
		return &copy, nil
	}
	return nil, nil
}

func (f *compositePoolGateAccountRepo) ListSchedulable(context.Context) ([]Account, error) {
	return f.listByPlatform(""), nil
}

func (f *compositePoolGateAccountRepo) ListSchedulableByGroupID(context.Context, int64) ([]Account, error) {
	return f.listByPlatform(""), nil
}

func (f *compositePoolGateAccountRepo) ListSchedulableByPlatform(_ context.Context, platform string) ([]Account, error) {
	return f.listByPlatform(platform), nil
}

func (f *compositePoolGateAccountRepo) ListSchedulableByGroupIDAndPlatform(_ context.Context, _ int64, platform string) ([]Account, error) {
	return f.listByPlatform(platform), nil
}

func (f *compositePoolGateAccountRepo) ListSchedulableUngroupedByPlatform(_ context.Context, platform string) ([]Account, error) {
	return f.listByPlatform(platform), nil
}

func (f *compositePoolGateAccountRepo) listByPlatform(platform string) []Account {
	out := make([]Account, 0, len(f.byID))
	for _, account := range f.byID {
		if platform == "" || account.Platform == platform {
			out = append(out, *account)
		}
	}
	return out
}

func newCompositePoolGateService() (*OpenAIGatewayService, *compositePoolGateStickyCache) {
	stickyCache := &compositePoolGateStickyCache{stickyID: 4333}
	// mapping 声明与 routes 夹具一致：池 claims 能力门按 mapping key 判定。
	credentials := map[string]any{
		"api_key":       "k",
		"base_url":      "https://deepseek-pool-fake.test/v1",
		"model_mapping": map[string]any{"deepseek-flash": "deepseek-chat"},
	}
	svc := &OpenAIGatewayService{
		cache: stickyCache,
		accountRepo: &compositePoolGateAccountRepo{byID: map[int64]*Account{
			4333: {ID: 4333, Platform: PlatformDeepseek, Type: AccountTypeAPIKey, Status: StatusActive, Schedulable: true, Concurrency: 4, Credentials: credentials},
			4334: {ID: 4334, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Status: StatusActive, Schedulable: true, Concurrency: 4},
		}},
		cfg: &config.Config{},
	}
	return svc, stickyCache
}

// sticky 账号 X（deepseek）在平台被 deny 时：早退返回 nil 且零 delete——即使 X 不在
// excludedIDs（即失败的不是 X 自己，而是同平台的兄弟账号 Y）。
func TestCompositePoolStickyGateSkipsDeniedPlatformWithoutDelete(t *testing.T) {
	svc, stickyCache := newCompositePoolGateService()
	groupID := int64(4330)
	base := WithCompositeCandidatePlatforms(context.Background(), []string{PlatformDeepseek, PlatformOpenAI})
	deniedCtx := WithCompositePoolDeniedPlatforms(base, []string{PlatformDeepseek})

	// 非空洞前提：同夹具下 X 可正常加载（排除"加载失败导致 nil"的假阳性）。
	fetched, err := svc.getSchedulableAccount(context.Background(), 4333)
	require.NoError(t, err)
	require.NotNil(t, fetched)
	require.Equal(t, PlatformDeepseek, fetched.Platform)

	account := svc.tryStickySessionHit(deniedCtx, &groupID, PlatformOpenAI, "sess-gate", "deepseek-flash", nil, false, 0, OpenAIEndpointCapabilityChatCompletions)
	require.Nil(t, account, "denied-platform sticky account must be temporarily skipped")
	require.Equal(t, 0, stickyCache.deletes, "temporary denial must not delete the sticky binding")
}

// list 门：denied 平台从 pool 桶与单平台桶中被过滤。
func TestCompositePoolListGateFiltersDeniedPlatforms(t *testing.T) {
	svc, _ := newCompositePoolGateService()
	groupID := int64(4330)
	base := WithCompositeCandidatePlatforms(context.Background(), []string{PlatformDeepseek, PlatformOpenAI})
	deniedCtx := WithCompositePoolDeniedPlatforms(base, []string{PlatformDeepseek})

	accounts, err := svc.listSchedulableAccountsForCompositePool(deniedCtx, &groupID, []string{PlatformDeepseek, PlatformOpenAI})
	require.NoError(t, err)
	for _, account := range accounts {
		require.NotEqual(t, PlatformDeepseek, account.Platform, "denied platform must be filtered from the pool list")
	}

	single, err := svc.listSchedulableAccountsSinglePlatform(deniedCtx, &groupID, PlatformDeepseek)
	require.NoError(t, err)
	require.Empty(t, single, "single-platform listing under a denied attempt ctx must return an empty bucket")

	// 对照：无 denied overlay 时两个平台桶都可用。
	accounts, err = svc.listSchedulableAccountsForCompositePool(base, &groupID, []string{PlatformDeepseek, PlatformOpenAI})
	require.NoError(t, err)
	platforms := map[string]bool{}
	for _, account := range accounts {
		platforms[account.Platform] = true
	}
	require.True(t, platforms[PlatformDeepseek] && platforms[PlatformOpenAI])
}
