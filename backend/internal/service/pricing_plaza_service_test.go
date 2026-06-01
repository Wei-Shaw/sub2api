//go:build unit

package service

import (
	"context"
	"sort"
	"testing"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/stretchr/testify/require"
)

// =====================================================================
// Stubs scoped to plaza tests.
// =====================================================================

// groupRepoStubForPlaza implements GroupRepository for plaza tests.
// Only ListActive is exercised; other methods panic to surface mistakes.
type groupRepoStubForPlaza struct {
	groups []Group
}

func (s *groupRepoStubForPlaza) Create(context.Context, *Group) error { panic("not used") }
func (s *groupRepoStubForPlaza) GetByID(context.Context, int64) (*Group, error) {
	panic("not used")
}
func (s *groupRepoStubForPlaza) GetByIDLite(context.Context, int64) (*Group, error) {
	panic("not used")
}
func (s *groupRepoStubForPlaza) Update(context.Context, *Group) error { panic("not used") }
func (s *groupRepoStubForPlaza) Delete(context.Context, int64) error  { panic("not used") }
func (s *groupRepoStubForPlaza) DeleteCascade(context.Context, int64) ([]int64, error) {
	panic("not used")
}
func (s *groupRepoStubForPlaza) List(context.Context, pagination.PaginationParams) ([]Group, *pagination.PaginationResult, error) {
	panic("not used")
}
func (s *groupRepoStubForPlaza) ListWithFilters(context.Context, pagination.PaginationParams, string, string, string, *bool) ([]Group, *pagination.PaginationResult, error) {
	panic("not used")
}
func (s *groupRepoStubForPlaza) ListActive(context.Context) ([]Group, error) {
	out := make([]Group, 0, len(s.groups))
	for _, g := range s.groups {
		if g.Status == StatusActive {
			out = append(out, g)
		}
	}
	return out, nil
}
func (s *groupRepoStubForPlaza) ListActiveByPlatform(context.Context, string) ([]Group, error) {
	panic("not used")
}
func (s *groupRepoStubForPlaza) ExistsByName(context.Context, string) (bool, error) {
	panic("not used")
}
func (s *groupRepoStubForPlaza) GetAccountCount(context.Context, int64) (int64, int64, error) {
	panic("not used")
}
func (s *groupRepoStubForPlaza) DeleteAccountGroupsByGroupID(context.Context, int64) (int64, error) {
	panic("not used")
}
func (s *groupRepoStubForPlaza) GetAccountIDsByGroupIDs(context.Context, []int64) ([]int64, error) {
	panic("not used")
}
func (s *groupRepoStubForPlaza) BindAccountsToGroup(context.Context, int64, []int64) error {
	panic("not used")
}
func (s *groupRepoStubForPlaza) UpdateSortOrders(context.Context, []GroupSortOrderUpdate) error {
	panic("not used")
}

// paymentConfigStubForPlaza implements PlazaPaymentConfigSource.
type paymentConfigStubForPlaza struct {
	multiplier float64
	plans      []*dbent.SubscriptionPlan
}

func (s *paymentConfigStubForPlaza) GetPaymentConfig(context.Context) (*PaymentConfig, error) {
	return &PaymentConfig{BalanceRechargeMultiplier: s.multiplier}, nil
}
func (s *paymentConfigStubForPlaza) ListPlansForSale(context.Context) ([]*dbent.SubscriptionPlan, error) {
	return s.plans, nil
}

// accountRepoStubForPlaza implements PlazaAccountSource.
//
// 仅暴露广场需要的 ListActive；调用方传入的账号会原样回放（按 status=active 过滤）。
type accountRepoStubForPlaza struct {
	accounts []Account
	calls    int
}

func (s *accountRepoStubForPlaza) ListActive(context.Context) ([]Account, error) {
	s.calls++
	out := make([]Account, 0, len(s.accounts))
	for _, a := range s.accounts {
		if a.Status == StatusActive {
			out = append(out, a)
		}
	}
	return out, nil
}

// =====================================================================
// Helpers
// =====================================================================

// newPlazaServiceForTest 构造一个能跑同步测试的 PlazaService。
//
// pricingData: LiteLLM 注入数据，nil 表示一律命中 fallback。
// accounts: 模型集合源；广场会读 account.Credentials["model_mapping"] 的 keys。
// groups: 候选分组集合（已过滤前的全集；resolveEligibleGroups 内部还会过滤）。
// payment: 价格倍数 / 套餐源。
func newPlazaServiceForTest(
	t *testing.T,
	pricingData map[string]*LiteLLMModelPricing,
	accounts []Account,
	groups []Group,
	payment *paymentConfigStubForPlaza,
) (*PlazaService, *accountRepoStubForPlaza) {
	t.Helper()
	pricingSvc := &PricingService{pricingData: pricingData}
	billingSvc := NewBillingService(&config.Config{}, pricingSvc)
	accountRepo := &accountRepoStubForPlaza{accounts: append([]Account(nil), accounts...)}
	groupRepo := &groupRepoStubForPlaza{groups: groups}
	return NewPlazaService(pricingSvc, billingSvc, groupRepo, accountRepo, payment), accountRepo
}

func plazaPtrFloat(v float64) *float64 { return &v }

// makeAccount 构造一个仅含 model_mapping 的最小测试账号。
//
// modelNames 为 mapping 的 keys（target 取同名，便于调试可读）。
func makeAccount(id int64, platform string, groupIDs []int64, modelNames ...string) Account {
	mapping := make(map[string]any, len(modelNames))
	for _, m := range modelNames {
		mapping[m] = m
	}
	return Account{
		ID:       id,
		Platform: platform,
		Status:   StatusActive,
		GroupIDs: groupIDs,
		Credentials: map[string]any{
			"model_mapping": mapping,
		},
	}
}

// =====================================================================
// ListModelRows
// =====================================================================

func TestPlazaService_ListModelRows_TokenOnlyGroup(t *testing.T) {
	groups := []Group{{
		ID:               1,
		Name:             "Public Standard",
		Platform:         PlatformAnthropic,
		Status:           StatusActive,
		IsExclusive:      false,
		SubscriptionType: SubscriptionTypeStandard,
		RateMultiplier:   2.0,
	}}

	// 同名模型在两个账号出现，应去重
	accounts := []Account{
		makeAccount(100, PlatformAnthropic, []int64{1}, "claude-sonnet-4"),
		makeAccount(101, PlatformAnthropic, []int64{1}, "claude-sonnet-4", "claude-3-5-haiku"),
	}

	svc, _ := newPlazaServiceForTest(t, nil, accounts, groups, &paymentConfigStubForPlaza{multiplier: 0.14})

	rows, meta, err := svc.ListModelRows(context.Background(), PlazaModelFilter{})
	require.NoError(t, err)
	require.InDelta(t, 0.14, meta.BalanceRechargeMultiplier, 1e-9)
	require.Equal(t, "USD", meta.ModelNative)
	require.Equal(t, "CNY", meta.PlanNative)

	// 应该有两条 token 行，按字典序：claude-3-5-haiku, claude-sonnet-4
	require.Len(t, rows, 2)
	require.Equal(t, "claude-3-5-haiku", rows[0].Model)
	require.Equal(t, "claude-sonnet-4", rows[1].Model)
	for _, r := range rows {
		require.Equal(t, "token", r.Type)
		require.Equal(t, int64(1), r.GroupID)
		require.InDelta(t, 2.0, r.Multiplier, 1e-9)
		// site = base × multiplier
		require.InDelta(t, r.InputPricePerMTok*2.0, r.SiteInputPricePerMTok, 1e-9)
		require.InDelta(t, r.OutputPricePerMTok*2.0, r.SiteOutputPricePerMTok, 1e-9)
		// discount = (1 - 1/2) × 100 = 50
		require.InDelta(t, 50.0, r.DiscountPercent, 1e-9)
	}
}

func TestPlazaService_ListModelRows_ImageRowFromLiteLLMMode(t *testing.T) {
	groups := []Group{{
		ID:               2,
		Name:             "Image Group",
		Platform:         PlatformGemini,
		Status:           StatusActive,
		SubscriptionType: SubscriptionTypeStandard,
		RateMultiplier:   1.0,
		// 不配置 ImagePrice*K → 走 LiteLLM 默认价（基础 $0.10），tier 系数仍生效
		AllowImageGeneration: true,
		ImageRateIndependent: true,
		ImageRateMultiplier:  3.0,
	}}

	accounts := []Account{makeAccount(200, PlatformGemini, []int64{2}, "img-model")}

	pricingData := map[string]*LiteLLMModelPricing{
		"img-model": {
			Mode:               "image_generation",
			OutputCostPerImage: 0.10,
		},
	}

	svc, _ := newPlazaServiceForTest(t, pricingData, accounts, groups, &paymentConfigStubForPlaza{multiplier: 1.0})

	rows, _, err := svc.ListModelRows(context.Background(), PlazaModelFilter{})
	require.NoError(t, err)
	require.Len(t, rows, 1)
	r := rows[0]
	require.Equal(t, "image", r.Type)
	require.NotNil(t, r.BaseImagePrices)
	require.NotNil(t, r.SiteImagePrices)
	// 基础价：1K=0.10, 2K=0.15, 4K=0.20
	require.InDelta(t, 0.10, r.BaseImagePrices.Tier1K, 1e-9)
	require.InDelta(t, 0.15, r.BaseImagePrices.Tier2K, 1e-9)
	require.InDelta(t, 0.20, r.BaseImagePrices.Tier4K, 1e-9)
	// 站点价：基础 × image_rate_multiplier(3.0)
	require.InDelta(t, 0.30, r.SiteImagePrices.Tier1K, 1e-9)
	require.InDelta(t, 0.45, r.SiteImagePrices.Tier2K, 1e-9)
	require.InDelta(t, 0.60, r.SiteImagePrices.Tier4K, 1e-9)
	require.InDelta(t, 3.0, r.Multiplier, 1e-9)
}

func TestPlazaService_ListModelRows_ImageRowGroupOverride(t *testing.T) {
	groups := []Group{{
		ID:                   3,
		Name:                 "Override Group",
		Platform:             PlatformGemini,
		Status:               StatusActive,
		SubscriptionType:     SubscriptionTypeStandard,
		RateMultiplier:       1.5,
		AllowImageGeneration: true,
		ImagePrice1K:         plazaPtrFloat(0.20),
		ImagePrice2K:         plazaPtrFloat(0.40),
		ImagePrice4K:         plazaPtrFloat(0.80),
	}}
	accounts := []Account{makeAccount(300, PlatformGemini, []int64{3}, "img-x")}
	pricingData := map[string]*LiteLLMModelPricing{
		"img-x": {Mode: "image_generation"},
	}
	svc, _ := newPlazaServiceForTest(t, pricingData, accounts, groups, &paymentConfigStubForPlaza{multiplier: 1.0})

	rows, _, err := svc.ListModelRows(context.Background(), PlazaModelFilter{})
	require.NoError(t, err)
	require.Len(t, rows, 1)
	r := rows[0]
	require.Equal(t, "image", r.Type)
	// 分组覆盖优先于 LiteLLM 默认价；使用分组 RateMultiplier 1.5
	require.InDelta(t, 0.20, r.BaseImagePrices.Tier1K, 1e-9)
	require.InDelta(t, 0.30, r.SiteImagePrices.Tier1K, 1e-9)
	require.InDelta(t, 0.40, r.BaseImagePrices.Tier2K, 1e-9)
	require.InDelta(t, 0.60, r.SiteImagePrices.Tier2K, 1e-9)
	require.InDelta(t, 1.5, r.Multiplier, 1e-9)
}

func TestPlazaService_ListModelRows_LiteLLMMissFallbackHit(t *testing.T) {
	groups := []Group{{
		ID:               4,
		Name:             "Fallback Group",
		Platform:         PlatformAnthropic,
		Status:           StatusActive,
		SubscriptionType: SubscriptionTypeStandard,
		RateMultiplier:   1.0,
	}}
	accounts := []Account{makeAccount(400, PlatformAnthropic, []int64{4}, "claude-3-5-sonnet")}
	// 不注入 LiteLLM 数据 → BillingService 用 fallback ("claude-3-5-sonnet")
	svc, _ := newPlazaServiceForTest(t, nil, accounts, groups, &paymentConfigStubForPlaza{multiplier: 1.0})

	rows, _, err := svc.ListModelRows(context.Background(), PlazaModelFilter{})
	require.NoError(t, err)
	require.Len(t, rows, 1)
	require.Equal(t, "claude-3-5-sonnet", rows[0].Model)
	require.Greater(t, rows[0].InputPricePerMTok, 0.0)
	require.Greater(t, rows[0].OutputPricePerMTok, 0.0)
}

func TestPlazaService_ListModelRows_DropOnFullMiss(t *testing.T) {
	groups := []Group{{
		ID:               5,
		Name:             "Misses",
		Platform:         PlatformAnthropic,
		Status:           StatusActive,
		SubscriptionType: SubscriptionTypeStandard,
		RateMultiplier:   1.0,
	}}
	accounts := []Account{makeAccount(500, PlatformAnthropic, []int64{5}, "completely-unknown-model")}
	svc, _ := newPlazaServiceForTest(t, nil, accounts, groups, &paymentConfigStubForPlaza{multiplier: 1.0})
	rows, _, err := svc.ListModelRows(context.Background(), PlazaModelFilter{})
	require.NoError(t, err)
	require.Empty(t, rows)
}

func TestPlazaService_ListModelRows_GroupEligibility(t *testing.T) {
	cases := []struct {
		name string
		g    Group
		want bool
	}{
		{"active+standard+public", Group{ID: 10, Status: StatusActive, SubscriptionType: SubscriptionTypeStandard, RateMultiplier: 1, IsExclusive: false}, true},
		{"exclusive", Group{ID: 11, Status: StatusActive, SubscriptionType: SubscriptionTypeStandard, RateMultiplier: 1, IsExclusive: true}, false},
		{"disabled", Group{ID: 12, Status: StatusDisabled, SubscriptionType: SubscriptionTypeStandard, RateMultiplier: 1, IsExclusive: false}, false},
		{"subscription type", Group{ID: 13, Status: StatusActive, SubscriptionType: SubscriptionTypeSubscription, RateMultiplier: 1, IsExclusive: false}, false},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			g := tc.g
			g.Name = tc.name
			g.Platform = PlatformAnthropic
			accounts := []Account{makeAccount(600+g.ID, PlatformAnthropic, []int64{g.ID}, "claude-3-5-sonnet")}
			svc, _ := newPlazaServiceForTest(t, nil, accounts, []Group{g}, &paymentConfigStubForPlaza{multiplier: 1})
			rows, _, err := svc.ListModelRows(context.Background(), PlazaModelFilter{})
			require.NoError(t, err)
			if tc.want {
				require.NotEmpty(t, rows, "expected rows for %s", tc.name)
			} else {
				require.Empty(t, rows, "expected zero rows for %s", tc.name)
			}
		})
	}
}

func TestPlazaService_ListModelRows_FilterAndSearch(t *testing.T) {
	groups := []Group{
		{ID: 20, Name: "Anthropic Pool", Platform: PlatformAnthropic, Status: StatusActive, SubscriptionType: SubscriptionTypeStandard, RateMultiplier: 1},
		{ID: 21, Name: "Gemini Pool", Platform: PlatformGemini, Status: StatusActive, SubscriptionType: SubscriptionTypeStandard, RateMultiplier: 1},
	}
	accounts := []Account{
		makeAccount(700, PlatformAnthropic, []int64{20}, "claude-3-5-sonnet", "claude-3-5-haiku"),
		makeAccount(701, PlatformGemini, []int64{21}, "gemini-3.1-pro"),
	}
	svc, _ := newPlazaServiceForTest(t, nil, accounts, groups, &paymentConfigStubForPlaza{multiplier: 1})

	all, _, err := svc.ListModelRows(context.Background(), PlazaModelFilter{})
	require.NoError(t, err)
	require.Len(t, all, 3)

	rowsByGroup, _, err := svc.ListModelRows(context.Background(), PlazaModelFilter{GroupID: 21})
	require.NoError(t, err)
	require.Len(t, rowsByGroup, 1)
	require.Equal(t, "gemini-3.1-pro", rowsByGroup[0].Model)

	rowsByPlatform, _, err := svc.ListModelRows(context.Background(), PlazaModelFilter{Platform: PlatformAnthropic})
	require.NoError(t, err)
	require.Len(t, rowsByPlatform, 2)

	rowsBySearch, _, err := svc.ListModelRows(context.Background(), PlazaModelFilter{Q: "HAIKU"})
	require.NoError(t, err)
	require.Len(t, rowsBySearch, 1)
	require.Equal(t, "claude-3-5-haiku", rowsBySearch[0].Model)
}

func TestPlazaService_ListModelRows_ZeroAccounts(t *testing.T) {
	groups := []Group{{
		ID: 30, Name: "Lonely", Platform: PlatformAnthropic,
		Status: StatusActive, SubscriptionType: SubscriptionTypeStandard, RateMultiplier: 1,
	}}
	svc, _ := newPlazaServiceForTest(t, nil, []Account{}, groups, &paymentConfigStubForPlaza{multiplier: 1})
	rows, _, err := svc.ListModelRows(context.Background(), PlazaModelFilter{})
	require.NoError(t, err)
	require.Empty(t, rows)
}

func TestPlazaService_ListModelRows_WildcardKeysSkipped(t *testing.T) {
	groups := []Group{{
		ID: 31, Name: "WC", Platform: PlatformAnthropic,
		Status: StatusActive, SubscriptionType: SubscriptionTypeStandard, RateMultiplier: 1,
	}}
	// 通配符 key 应被剔除，仅保留 claude-3-5-sonnet
	accounts := []Account{makeAccount(710, PlatformAnthropic, []int64{31}, "claude-*", "claude-3-5-sonnet")}
	svc, _ := newPlazaServiceForTest(t, nil, accounts, groups, &paymentConfigStubForPlaza{multiplier: 1})
	rows, _, err := svc.ListModelRows(context.Background(), PlazaModelFilter{})
	require.NoError(t, err)
	require.Len(t, rows, 1)
	require.Equal(t, "claude-3-5-sonnet", rows[0].Model)
}

// =====================================================================
// ListPlanCards
// =====================================================================

func TestPlazaService_ListPlanCards_BasicAndFilters(t *testing.T) {
	groups := []Group{
		{ID: 40, Name: "Alpha", Platform: PlatformAnthropic, Status: StatusActive, SubscriptionType: SubscriptionTypeStandard, RateMultiplier: 1},
		// Beta 不活跃 → 关联的套餐应被剔除
		{ID: 41, Name: "Beta Inactive", Platform: PlatformAnthropic, Status: StatusDisabled, SubscriptionType: SubscriptionTypeStandard, RateMultiplier: 1},
	}
	accounts := []Account{makeAccount(800, PlatformAnthropic, []int64{40}, "claude-3-5-sonnet", "claude-3-5-haiku")}

	plans := []*dbent.SubscriptionPlan{
		{ID: 1, Name: "Plan A", GroupID: 40, Price: 99, ValidityDays: 30, ValidityUnit: "day", SortOrder: 1, ForSale: true},
		{ID: 2, Name: "Plan B Inactive Group", GroupID: 41, Price: 199, ValidityDays: 30, ValidityUnit: "day", SortOrder: 2, ForSale: true},
		{ID: 3, Name: "Plan C Missing Group", GroupID: 999, Price: 299, ValidityDays: 30, ValidityUnit: "day", SortOrder: 3, ForSale: true},
	}

	svc, _ := newPlazaServiceForTest(t, nil, accounts, groups, &paymentConfigStubForPlaza{multiplier: 0.14, plans: plans})

	cards, meta, err := svc.ListPlanCards(context.Background())
	require.NoError(t, err)
	require.Equal(t, "CNY", meta.PlanNative)
	require.Len(t, cards, 1)
	c := cards[0]
	require.Equal(t, "Plan A", c.Name)
	require.Equal(t, int64(40), c.GroupID)
	require.Equal(t, "Alpha", c.GroupName)
	// account 给该分组贡献了两个模型
	sort.Strings(c.Models)
	require.Equal(t, []string{"claude-3-5-haiku", "claude-3-5-sonnet"}, c.Models)
	require.Equal(t, 0, c.ModelsOverflow)
}

func TestPlazaService_ListPlanCards_ModelsCap(t *testing.T) {
	const groupID int64 = 50
	groups := []Group{{ID: groupID, Name: "Big", Platform: PlatformAnthropic, Status: StatusActive, SubscriptionType: SubscriptionTypeStandard, RateMultiplier: 1}}

	names := make([]string, 0, 75)
	for i := 0; i < 75; i++ {
		names = append(names, "model-"+intToFixed(i, 3))
	}
	accounts := []Account{makeAccount(900, PlatformAnthropic, []int64{groupID}, names...)}

	plans := []*dbent.SubscriptionPlan{{ID: 7, Name: "Big Plan", GroupID: groupID, Price: 1, ValidityDays: 30, ValidityUnit: "day", ForSale: true}}

	svc, _ := newPlazaServiceForTest(t, nil, accounts, groups, &paymentConfigStubForPlaza{multiplier: 1, plans: plans})
	cards, _, err := svc.ListPlanCards(context.Background())
	require.NoError(t, err)
	require.Len(t, cards, 1)
	require.Len(t, cards[0].Models, plazaPlanModelsCap)
	require.Equal(t, 75-plazaPlanModelsCap, cards[0].ModelsOverflow)
}

// =====================================================================
// CurrencyMeta
// =====================================================================

func TestPlazaService_CurrencyMetaReflectsMultiplier(t *testing.T) {
	groups := []Group{{ID: 60, Name: "G", Platform: PlatformAnthropic, Status: StatusActive, SubscriptionType: SubscriptionTypeStandard, RateMultiplier: 1}}
	svc, _ := newPlazaServiceForTest(t, nil, []Account{}, groups, &paymentConfigStubForPlaza{multiplier: 0.14})
	_, meta, err := svc.ListModelRows(context.Background(), PlazaModelFilter{})
	require.NoError(t, err)
	require.InDelta(t, 0.14, meta.BalanceRechargeMultiplier, 1e-9)

	svc2, _ := newPlazaServiceForTest(t, nil, []Account{}, groups, &paymentConfigStubForPlaza{multiplier: 0})
	_, meta2, err := svc2.ListModelRows(context.Background(), PlazaModelFilter{})
	require.NoError(t, err)
	// 0 / 负数被规整为 1
	require.InDelta(t, 1.0, meta2.BalanceRechargeMultiplier, 1e-9)
}

// =====================================================================
// Cache
// =====================================================================

func TestPlazaService_ListModelRows_CacheHit(t *testing.T) {
	groups := []Group{{ID: 70, Name: "Cache", Platform: PlatformAnthropic, Status: StatusActive, SubscriptionType: SubscriptionTypeStandard, RateMultiplier: 1}}
	svc, accountRepo := newPlazaServiceForTest(t, nil, []Account{}, groups, &paymentConfigStubForPlaza{multiplier: 1})

	for i := 0; i < 3; i++ {
		_, _, err := svc.ListModelRows(context.Background(), PlazaModelFilter{})
		require.NoError(t, err)
	}
	require.Equal(t, 1, accountRepo.calls, "account repo should be hit only once thanks to cache")
}

// intToFixed renders an integer with at least width digits (zero-padded).
func intToFixed(n, width int) string {
	s := ""
	for n > 0 {
		s = string(rune('0'+n%10)) + s
		n /= 10
	}
	for len(s) < width {
		s = "0" + s
	}
	return s
}
