package service

// 增量2A：OpenAI 兼容族 selector 跨平台候选池（composite account_pool）测试。
//
// 覆盖：
//   a. 双平台同模型按原 priority/load（LRU）跨平台选中；
//   b. 同 session 连续请求与负载变化保持 sticky；
//   c. sticky 缓存命名空间保持 openai: 前缀，legacy 读回退/双写不变；
//   d. 失败排除首账号后跨平台选中并粘新账号，池不塌缩；
//   e. 换 group 不复用绑定、换 model 重新资格检查；
//   f. claims 谓词：官方 deepseek 空 mapping 参与、opencode_go 需显式 mapping、
//      无关平台空 mapping 不入池；
//   g. 全部候选瞬态不可用时保持无容量错误语义（ErrNoAvailableAccounts）；
//   h. previous_response_id pin 归属保持：pool 内仍优先同账号；账号失效按原策略
//      删绑定；池外账号保留绑定且不跨平台带走 previous_response_id。

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

// ---- 测试专用 fake ----

// compositePoolTestCache 按 (groupID, cacheKey) 记录 sticky 绑定，并原样记录全部
// Set/Delete 的 cacheKey，供命名空间（openai: 前缀）断言使用。
type compositePoolTestCache struct {
	GatewayCache
	mu       sync.Mutex
	bindings map[string]int64
	setKeys  []string
	delKeys  []string
}

func compositePoolCacheKey(groupID int64, cacheKey string) string {
	return strconv.FormatInt(groupID, 10) + "|" + cacheKey
}

func (c *compositePoolTestCache) GetSessionAccountID(_ context.Context, groupID int64, cacheKey string) (int64, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if id, ok := c.bindings[compositePoolCacheKey(groupID, cacheKey)]; ok {
		return id, nil
	}
	return 0, ErrStickySessionNotFound
}

func (c *compositePoolTestCache) SetSessionAccountID(_ context.Context, groupID int64, cacheKey string, accountID int64, _ time.Duration) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.bindings == nil {
		c.bindings = make(map[string]int64)
	}
	c.bindings[compositePoolCacheKey(groupID, cacheKey)] = accountID
	c.setKeys = append(c.setKeys, cacheKey)
	return nil
}

func (c *compositePoolTestCache) RefreshSessionTTL(_ context.Context, _ int64, _ string, _ time.Duration) error {
	return nil
}

func (c *compositePoolTestCache) DeleteSessionAccountID(_ context.Context, groupID int64, cacheKey string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.bindings, compositePoolCacheKey(groupID, cacheKey))
	c.delKeys = append(c.delKeys, cacheKey)
	return nil
}

func (c *compositePoolTestCache) binding(groupID int64, cacheKey string) (int64, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	id, ok := c.bindings[compositePoolCacheKey(groupID, cacheKey)]
	return id, ok
}

// compositePoolTestRepo 平台过滤语义与生产 repo 一致；组维度按账号 GroupIDs
// 过滤（对齐生产 ListSchedulableByGroupIDAndPlatform 的 SQL 语义），指针接收者
// 支持用例中途改账号状态（解绑组、移除 mapping）。
type compositePoolTestRepo struct {
	AccountRepository
	accounts []Account
}

func (r *compositePoolTestRepo) GetByID(_ context.Context, id int64) (*Account, error) {
	for i := range r.accounts {
		if r.accounts[i].ID == id {
			return &r.accounts[i], nil
		}
	}
	return nil, errors.New("account not found")
}

func accountInGroup(account *Account, groupID int64) bool {
	if account == nil {
		return false
	}
	return openAIStickyAccountMatchesGroup(account, &groupID)
}

func (r *compositePoolTestRepo) listByPlatform(platform string) []Account {
	var out []Account
	for i := range r.accounts {
		if r.accounts[i].Platform == platform {
			out = append(out, r.accounts[i])
		}
	}
	return out
}

func (r *compositePoolTestRepo) listByPlatformAndGroup(platform string, groupID int64) []Account {
	var out []Account
	for i := range r.accounts {
		if r.accounts[i].Platform == platform && accountInGroup(&r.accounts[i], groupID) {
			out = append(out, r.accounts[i])
		}
	}
	return out
}

func (r *compositePoolTestRepo) ListSchedulableByPlatform(_ context.Context, platform string) ([]Account, error) {
	return r.listByPlatform(platform), nil
}

func (r *compositePoolTestRepo) ListSchedulableByGroupIDAndPlatform(_ context.Context, groupID int64, platform string) ([]Account, error) {
	return r.listByPlatformAndGroup(platform, groupID), nil
}

func (r *compositePoolTestRepo) ListSchedulableUngroupedByPlatform(_ context.Context, platform string) ([]Account, error) {
	var out []Account
	for i := range r.accounts {
		if r.accounts[i].Platform == platform && openAIStickyAccountMatchesGroup(&r.accounts[i], nil) {
			out = append(out, r.accounts[i])
		}
	}
	return out, nil
}

func compositePoolTestAccount(id int64, platform string, groupID int64, priority int, mapping map[string]any) Account {
	account := Account{
		ID:          id,
		Platform:    platform,
		Type:        AccountTypeAPIKey,
		Status:      StatusActive,
		Schedulable: true,
		Concurrency: 1,
		Priority:    priority,
		GroupIDs:    []int64{groupID},
	}
	if mapping != nil {
		account.Credentials = map[string]any{"model_mapping": mapping}
	}
	return account
}

func newCompositePoolTestService(accounts []Account, cache GatewayCache) *OpenAIGatewayService {
	cfg := &config.Config{}
	cfg.Gateway.Scheduling.LoadBatchEnabled = false
	cfg.Gateway.OpenAIWS.SessionHashReadOldFallback = true
	cfg.Gateway.OpenAIWS.SessionHashDualWriteOld = true
	return &OpenAIGatewayService{
		accountRepo:        &compositePoolTestRepo{accounts: accounts},
		cache:              cache,
		cfg:                cfg,
		concurrencyService: NewConcurrencyService(schedulerTestConcurrencyCache{}),
	}
}

func newCompositePoolAdvancedTestService(t *testing.T, accounts []Account, cache GatewayCache) *OpenAIGatewayService {
	t.Helper()
	resetOpenAIAdvancedSchedulerSettingCacheForTest()
	svc := newCompositePoolTestService(accounts, cache)
	svc.rateLimitService = newOpenAIAdvancedSchedulerRateLimitService("true")
	require.True(t, svc.isOpenAIAdvancedSchedulerEnabled(context.Background()))
	return svc
}

// ---- a. 跨平台按 priority / load 选中 ----

func TestOpenAICompositePool_SelectsAcrossPlatformsByPriorityAndLoad(t *testing.T) {
	resetOpenAIAdvancedSchedulerSettingCacheForTest()
	defer resetOpenAIAdvancedSchedulerSettingCacheForTest()

	groupID := int64(22001)
	// 公开模型 my-model 由两个平台账号分别经显式映射 claim（claims 规则 A），
	// 保证同模型可跨平台比较 priority/load。
	model := "my-model"
	pool := []string{PlatformDeepseek, PlatformOpenAI}
	ctx := WithCompositeCandidatePlatforms(context.Background(), pool)

	build := func(deepseekPriority, openaiPriority int, deepseekUsed, openaiUsed *time.Time) (*OpenAIGatewayService, *compositePoolTestRepo) {
		repo := &compositePoolTestRepo{accounts: []Account{
			compositePoolTestAccount(33001, PlatformDeepseek, groupID, deepseekPriority, map[string]any{"my-model": "deepseek-chat"}),
			compositePoolTestAccount(33002, PlatformOpenAI, groupID, openaiPriority, map[string]any{"my-model": "gpt-4o"}),
		}}
		repo.accounts[0].LastUsedAt = deepseekUsed
		repo.accounts[1].LastUsedAt = openaiUsed
		return newCompositePoolTestService(repo.accounts, &compositePoolTestCache{}), repo
	}

	// priority 更小者跨平台获胜：openai(0) 压过 deepseek(5)。
	svc, _ := build(5, 0, nil, nil)
	selected, err := svc.SelectAccountForModelWithExclusions(ctx, &groupID, "", model, nil)
	require.NoError(t, err)
	require.EqualValues(t, 33002, selected.ID)

	// 反转 priority：deepseek(0) 获胜。
	svc, _ = build(0, 5, nil, nil)
	selected, err = svc.SelectAccountForModelWithExclusions(ctx, &groupID, "", model, nil)
	require.NoError(t, err)
	require.EqualValues(t, 33001, selected.ID)

	// 同 priority 时 LRU：更久未用（或从未使用）的账号获胜。
	now := time.Now()
	svc, _ = build(0, 0, &now, nil)
	selected, err = svc.SelectAccountForModelWithExclusions(ctx, &groupID, "", model, nil)
	require.NoError(t, err)
	require.EqualValues(t, 33002, selected.ID)

	stale := now.Add(-2 * time.Hour)
	recent := now.Add(-1 * time.Hour)
	svc, _ = build(0, 0, &stale, &recent)
	selected, err = svc.SelectAccountForModelWithExclusions(ctx, &groupID, "", model, nil)
	require.NoError(t, err)
	require.EqualValues(t, 33001, selected.ID)
}

// ---- b. 同 session 连续请求与负载变化保持 sticky ----

func TestOpenAICompositePool_StickySessionPersistsAcrossRequestsAndLoadChange(t *testing.T) {
	resetOpenAIAdvancedSchedulerSettingCacheForTest()
	defer resetOpenAIAdvancedSchedulerSettingCacheForTest()

	groupID := int64(22002)
	model := "my-model"
	sessionHash := "sesspoolb00000001"
	ctx := WithCompositeCandidatePlatforms(context.Background(), []string{PlatformDeepseek, PlatformOpenAI})

	repo := &compositePoolTestRepo{accounts: []Account{
		compositePoolTestAccount(33001, PlatformDeepseek, groupID, 0, map[string]any{"my-model": "deepseek-chat"}),
		compositePoolTestAccount(33002, PlatformOpenAI, groupID, 0, map[string]any{"my-model": "gpt-4o"}),
	}}
	cache := &compositePoolTestCache{}
	svc := newCompositePoolTestService(repo.accounts, cache)

	// 首次：候选池按序选中 deepseek 并写绑定。
	first, err := svc.SelectAccountForModelWithExclusions(ctx, &groupID, sessionHash, model, nil)
	require.NoError(t, err)
	require.EqualValues(t, 33001, first.ID)
	bound, ok := cache.binding(groupID, "openai:"+sessionHash)
	require.True(t, ok)
	require.EqualValues(t, 33001, bound)

	// 负载变化（sticky 账号刚被使用）：同 session 仍粘原账号。
	now := time.Now()
	repo.accounts[0].LastUsedAt = &now
	second, err := svc.SelectAccountForModelWithExclusions(ctx, &groupID, sessionHash, model, nil)
	require.NoError(t, err)
	require.EqualValues(t, 33001, second.ID)

	// 对照：无 session 时负载/LRU 生效，选中另一个平台账号。
	third, err := svc.SelectAccountForModelWithExclusions(ctx, &groupID, "", model, nil)
	require.NoError(t, err)
	require.EqualValues(t, 33002, third.ID)
}

// ---- c. sticky 命名空间（openai:）与 legacy 读回退/双写不变 ----

func TestOpenAICompositePool_StickyCacheKeyOpenAINamespaceAndLegacyCompat(t *testing.T) {
	resetOpenAIAdvancedSchedulerSettingCacheForTest()
	defer resetOpenAIAdvancedSchedulerSettingCacheForTest()

	groupID := int64(22003)
	sessionHash := "sesspoolc00000001"
	legacyHash, _ := deriveOpenAISessionHashes("legacy-seed-c")

	build := func(cache GatewayCache) *OpenAIGatewayService {
		repo := &compositePoolTestRepo{accounts: []Account{
			compositePoolTestAccount(34001, PlatformOpenAI, groupID, 0, map[string]any{"my-model": "gpt-4o"}),
		}}
		return newCompositePoolTestService(repo.accounts, cache)
	}

	// dual-write：主键 + legacy 键都写，且保持 openai: 前缀。
	cache := &compositePoolTestCache{}
	svc := build(cache)
	ctx := withOpenAILegacySessionHash(
		WithCompositeCandidatePlatforms(context.Background(), []string{PlatformOpenAI}),
		legacyHash,
	)
	require.NoError(t, svc.BindStickySession(ctx, &groupID, sessionHash, 34001))
	require.Contains(t, cache.setKeys, "openai:"+sessionHash)
	require.Contains(t, cache.setKeys, "openai:"+legacyHash)
	for _, key := range cache.setKeys {
		require.True(t, strings.HasPrefix(key, "openai:"), "sticky key must keep openai: prefix, got %q", key)
	}

	// legacy 读回退：主键 miss、legacy 键 hit。
	legacyOnlyCache := &compositePoolTestCache{bindings: map[string]int64{
		compositePoolCacheKey(groupID, "openai:"+legacyHash): 34001,
	}}
	svc2 := build(legacyOnlyCache)
	_, readTotalBefore, _ := openAIStickyCompatStats()
	got, err := svc2.getStickySessionAccountID(ctx, &groupID, sessionHash)
	require.NoError(t, err)
	require.EqualValues(t, 34001, got)
	_, readTotalAfter, _ := openAIStickyCompatStats()
	require.Greater(t, readTotalAfter, readTotalBefore, "legacy read fallback should be counted")

	// 候选数 1→2 命名空间不变：池选中后写入的仍全部是 openai: 前缀。
	poolCache := &compositePoolTestCache{}
	svc3 := newCompositePoolTestService([]Account{
		compositePoolTestAccount(34001, PlatformDeepseek, groupID, 0, map[string]any{"my-model": "deepseek-chat"}),
		compositePoolTestAccount(34002, PlatformOpenAI, groupID, 0, map[string]any{"my-model": "gpt-4o"}),
	}, poolCache)
	poolCtx := WithCompositeCandidatePlatforms(context.Background(), []string{PlatformDeepseek, PlatformOpenAI})
	_, err = svc3.SelectAccountForModelWithExclusions(poolCtx, &groupID, sessionHash, "my-model", nil)
	require.NoError(t, err)
	require.NotEmpty(t, poolCache.setKeys)
	for _, key := range poolCache.setKeys {
		require.True(t, strings.HasPrefix(key, "openai:"), "pool sticky key must keep openai: prefix, got %q", key)
	}
}

// ---- d. 失败排除后跨平台选中并粘新账号，池不塌缩 ----

func TestOpenAICompositePool_FailoverExcludesFailedAccountAndSticksToNewPlatform(t *testing.T) {
	resetOpenAIAdvancedSchedulerSettingCacheForTest()
	defer resetOpenAIAdvancedSchedulerSettingCacheForTest()

	groupID := int64(22004)
	model := "my-model"
	sessionHash := "sesspoold00000001"
	pool := []string{PlatformDeepseek, PlatformOpenAI}
	ctx := WithCompositeCandidatePlatforms(context.Background(), pool)

	repo := &compositePoolTestRepo{accounts: []Account{
		compositePoolTestAccount(33001, PlatformDeepseek, groupID, 0, map[string]any{"my-model": "deepseek-chat"}),
		compositePoolTestAccount(33002, PlatformOpenAI, groupID, 0, map[string]any{"my-model": "gpt-4o"}),
	}}
	cache := &compositePoolTestCache{}
	svc := newCompositePoolTestService(repo.accounts, cache)

	first, err := svc.SelectAccountForModelWithExclusions(ctx, &groupID, sessionHash, model, nil)
	require.NoError(t, err)
	require.EqualValues(t, 33001, first.ID)

	// 排除失败账号后跨平台选中 openai，并更新同组绑定。
	excluded := map[int64]struct{}{33001: {}}
	second, err := svc.SelectAccountForModelWithExclusions(ctx, &groupID, sessionHash, model, excluded)
	require.NoError(t, err)
	require.EqualValues(t, 33002, second.ID)
	bound, ok := cache.binding(groupID, "openai:"+sessionHash)
	require.True(t, ok)
	require.EqualValues(t, 33002, bound)

	// 后续请求粘新账号。
	third, err := svc.SelectAccountForModelWithExclusions(ctx, &groupID, sessionHash, model, nil)
	require.NoError(t, err)
	require.EqualValues(t, 33002, third.ID)

	// 池 immutable：ctx 候选池完整、未写 ResolvedTargetPlatform（不塌缩）。
	require.ElementsMatch(t, pool, CompositeCandidatePlatformsFromContext(ctx))
	_, resolved := ResolvedTargetPlatformFromContext(ctx)
	require.False(t, resolved)
}

// ---- e. 换 group 不复用绑定；换 model 重新资格检查 ----

func TestOpenAICompositePool_StickyGroupIsolationAndModelRequalification(t *testing.T) {
	resetOpenAIAdvancedSchedulerSettingCacheForTest()
	defer resetOpenAIAdvancedSchedulerSettingCacheForTest()

	groupA := int64(22005)
	groupB := int64(22006)
	sessionHash := "sesspoole00000001"
	ctx := WithCompositeCandidatePlatforms(context.Background(), []string{PlatformDeepseek, PlatformOpenAI})

	repo := &compositePoolTestRepo{accounts: []Account{
		compositePoolTestAccount(33001, PlatformDeepseek, groupA, 0, map[string]any{"model-a": "x-a"}),
		compositePoolTestAccount(33002, PlatformOpenAI, groupA, 0, map[string]any{"model-b": "x-b"}),
	}}
	cache := &compositePoolTestCache{}
	svc := newCompositePoolTestService(repo.accounts, cache)

	// group A 的绑定对 group B 不可见。
	require.NoError(t, svc.BindStickySession(ctx, &groupA, sessionHash, 33001))
	boundA, ok := cache.binding(groupA, "openai:"+sessionHash)
	require.True(t, ok)
	require.EqualValues(t, 33001, boundA)
	_, ok = cache.binding(groupB, "openai:"+sessionHash)
	require.False(t, ok)

	// 换 model：原 sticky 账号不 claim 新模型 → 重新资格检查选中 A2 并改绑。
	selected, err := svc.SelectAccountForModelWithExclusions(ctx, &groupA, sessionHash, "model-b", nil)
	require.NoError(t, err)
	require.EqualValues(t, 33002, selected.ID)
	boundA, ok = cache.binding(groupA, "openai:"+sessionHash)
	require.True(t, ok)
	require.EqualValues(t, 33002, boundA)

	// 再换回 model-a：sticky（A2）不 claim model-a → 重新选中 A1。
	selected, err = svc.SelectAccountForModelWithExclusions(ctx, &groupA, sessionHash, "model-a", nil)
	require.NoError(t, err)
	require.EqualValues(t, 33001, selected.ID)
}

// ---- f. claims 谓词约束池内模型能力 ----

func TestOpenAICompositePool_ClaimsPredicateGatesPoolMembership(t *testing.T) {
	resetOpenAIAdvancedSchedulerSettingCacheForTest()
	defer resetOpenAIAdvancedSchedulerSettingCacheForTest()

	poolCtx := WithCompositeCandidatePlatforms(context.Background(), []string{PlatformKimi, PlatformDeepseek, PlatformOpenCodeGo})
	deepseekAccount := compositePoolTestAccount(35001, PlatformDeepseek, 22007, 0, nil)
	kimiAccount := compositePoolTestAccount(35002, PlatformKimi, 22007, 0, nil)
	opencodeMapped := compositePoolTestAccount(35003, PlatformOpenCodeGo, 22007, 0, map[string]any{"my-model": "deepseek-chat"})

	// 官方 deepseek 空 mapping：native detector 命中 → 可 claim。
	// 使用受支持的 DeepSeek 模型名（空 mapping native claim 受官方白名单约束）。
	require.True(t, openAISchedulingModelSupported(poolCtx, &deepseekAccount, "deepseek-flash"))
	// 无关平台空 mapping：不得凭 IsModelSupported 的 allow-all 冒领。
	require.False(t, openAISchedulingModelSupported(poolCtx, &kimiAccount, "deepseek-chat"))
	require.True(t, kimiAccount.IsModelSupported("deepseek-chat"), "旧语义 allow-all 仍成立，池路径必须改用 claims 谓词")
	// opencode_go：显式 mapping 命中可入池，空/未命中不入。
	require.True(t, openAISchedulingModelSupported(poolCtx, &opencodeMapped, "my-model"))
	require.False(t, openAISchedulingModelSupported(poolCtx, &opencodeMapped, "deepseek-chat"))
	// 非 pool 请求保持 IsModelSupported 原语义。
	require.True(t, openAISchedulingModelSupported(context.Background(), &kimiAccount, "deepseek-chat"))

	// 资格检查命名否决点。
	reason := openAICompatibleAccountEligibilityFailureReason(poolCtx, &kimiAccount, PlatformOpenAI, "deepseek-chat", false, "")
	require.Equal(t, "model_not_supported", reason)

	// 选号层：kimi 被过滤，deepseek 入选。
	groupID := int64(22007)
	repo := &compositePoolTestRepo{accounts: []Account{deepseekAccount, kimiAccount}}
	cache := &compositePoolTestCache{}
	svc := newCompositePoolTestService(repo.accounts, cache)
	selection, err := svc.SelectAccountForModelWithExclusions(poolCtx, &groupID, "", "deepseek-flash", nil)
	require.NoError(t, err)
	require.EqualValues(t, 35001, selection.ID)
}

// ---- g. 全瞬态不可用保持无容量错误语义 ----

func TestOpenAICompositePool_AllTransientUnavailableKeepsNoCapacityError(t *testing.T) {
	resetOpenAIAdvancedSchedulerSettingCacheForTest()
	defer resetOpenAIAdvancedSchedulerSettingCacheForTest()

	groupID := int64(22008)
	ctx := WithCompositeCandidatePlatforms(context.Background(), []string{PlatformDeepseek, PlatformOpenAI})

	rateLimitedUntil := time.Now().Add(time.Hour)
	deepseekAccount := compositePoolTestAccount(33001, PlatformDeepseek, 22008, 0, nil)
	deepseekAccount.RateLimitResetAt = &rateLimitedUntil
	openaiAccount := compositePoolTestAccount(33002, PlatformOpenAI, 22008, 0, nil)
	openaiAccount.RateLimitResetAt = &rateLimitedUntil

	svc := newCompositePoolTestService([]Account{deepseekAccount, openaiAccount}, &compositePoolTestCache{})
	_, err := svc.SelectAccountForModelWithExclusions(ctx, &groupID, "", "deepseek-chat", nil)
	require.Error(t, err)
	require.True(t, errors.Is(err, ErrNoAvailableAccounts), "pool 全部瞬态不可用应保持无容量错误，got %v", err)
	require.False(t, errors.Is(err, ErrNoAvailableCompactAccounts))
	require.Contains(t, err.Error(), "no available OpenAI accounts supporting model: deepseek-chat")
}

// ---- h. previous_response_id pin 归属保持 ----

func TestOpenAICompositePool_PreviousResponseIDPinStaysWithOwnerAccount(t *testing.T) {
	resetOpenAIAdvancedSchedulerSettingCacheForTest()
	defer resetOpenAIAdvancedSchedulerSettingCacheForTest()

	groupID := int64(22009)
	responseID := "resp_pool_h_0001"
	model := "gpt-5.1"
	pool := []string{PlatformDeepseek, PlatformOpenAI}

	newRepo := func() *compositePoolTestRepo {
		// A1（openai，空 mapping）native claim gpt-5.1；A2（deepseek）显式映射 claim 同模型。
		return &compositePoolTestRepo{accounts: []Account{
			compositePoolTestAccount(33001, PlatformOpenAI, groupID, 0, nil),
			compositePoolTestAccount(33002, PlatformDeepseek, groupID, 0, map[string]any{"gpt-5.1": "deepseek-chat"}),
		}}
	}

	buildSvc := func(t *testing.T, repo *compositePoolTestRepo) (*OpenAIGatewayService, OpenAIWSStateStore) {
		t.Helper()
		svc := newCompositePoolAdvancedTestService(t, repo.accounts, &compositePoolTestCache{})
		store := svc.getOpenAIWSStateStore()
		require.NotNil(t, store)
		return svc, store
	}

	ctx := WithCompositeCandidatePlatforms(context.Background(), pool)

	// 1) pool 内有效 pin：仍优先同账号。
	repo := newRepo()
	svc, store := buildSvc(t, repo)
	require.NoError(t, store.BindResponseAccount(ctx, groupID, responseID, 33001, time.Hour))
	selection, decision, err := svc.SelectAccountWithScheduler(ctx, &groupID, responseID, "", model, nil, OpenAIUpstreamTransportAny, false)
	require.NoError(t, err)
	require.NotNil(t, selection)
	require.EqualValues(t, 33001, selection.Account.ID)
	require.True(t, decision.StickyPreviousHit)
	require.Equal(t, openAIAccountScheduleLayerPreviousResponse, decision.Layer)
	bound, err := store.GetResponseAccount(ctx, groupID, responseID)
	require.NoError(t, err)
	require.EqualValues(t, 33001, bound)

	// 2) pinned 账号失效：按原策略删除响应绑定，回落池内另一平台。
	repo = newRepo()
	repo.accounts[0].Schedulable = false
	svc, store = buildSvc(t, repo)
	require.NoError(t, store.BindResponseAccount(ctx, groupID, responseID, 33001, time.Hour))
	selection, decision, err = svc.SelectAccountWithScheduler(ctx, &groupID, responseID, "", model, nil, OpenAIUpstreamTransportAny, false)
	require.NoError(t, err)
	require.NotNil(t, selection)
	require.EqualValues(t, 33002, selection.Account.ID)
	require.False(t, decision.StickyPreviousHit)
	bound, err = store.GetResponseAccount(ctx, groupID, responseID)
	require.NoError(t, err)
	require.EqualValues(t, 0, bound, "失效账号的响应绑定应按原策略删除")

	// 3) pinned 账号在池外（账号健康）：本次不使用 pin、不删除绑定，
	//    previous_response_id 不得悄悄跨平台带走。pin 层已抢到的槽必须先释放
	//    （ReleaseFunc），再以池内账号重新出号——槽位/租约不得泄漏。
	repo = newRepo()
	acquiredIDs, releasedIDs := []int64{}, []int64{}
	svc = newCompositePoolAdvancedTestService(t, repo.accounts, &compositePoolTestCache{})
	svc.concurrencyService = NewConcurrencyService(schedulerTestConcurrencyCache{
		acquiredIDs: &acquiredIDs,
		releasedIDs: &releasedIDs,
	})
	store = svc.getOpenAIWSStateStore()
	require.NotNil(t, store)
	require.NoError(t, store.BindResponseAccount(ctx, groupID, responseID, 33001, time.Hour))
	narrowCtx := WithCompositeCandidatePlatforms(context.Background(), []string{PlatformDeepseek})
	selection, decision, err = svc.SelectAccountWithScheduler(narrowCtx, &groupID, responseID, "", model, nil, OpenAIUpstreamTransportAny, false)
	require.NoError(t, err)
	require.NotNil(t, selection)
	require.EqualValues(t, 33002, selection.Account.ID)
	require.False(t, decision.StickyPreviousHit)
	bound, err = store.GetResponseAccount(ctx, groupID, responseID)
	require.NoError(t, err)
	require.EqualValues(t, 33001, bound, "池外健康账号的响应绑定应保留")
	require.NotEmpty(t, acquiredIDs, "pin 命中路径应曾尝试抢槽")
	require.Contains(t, releasedIDs, int64(33001), "池外 pin 账号已占槽必须先释放")
	require.NotContains(t, releasedIDs, int64(33002), "最终选中账号的槽位不得被释放")
	if release := selection.ReleaseFunc; release != nil {
		t.Cleanup(release)
	}

	// 4) pinned 账号被显式排除（failover 重选限制）：pin 不得使用，绑定保留，
	//    改选池内剩余账号。
	repo = newRepo()
	svc, store = buildSvc(t, repo)
	require.NoError(t, store.BindResponseAccount(ctx, groupID, responseID, 33001, time.Hour))
	excluded := map[int64]struct{}{33001: {}}
	selection, decision, err = svc.SelectAccountWithScheduler(ctx, &groupID, responseID, "", model, excluded, OpenAIUpstreamTransportAny, false)
	require.NoError(t, err)
	require.NotNil(t, selection)
	require.EqualValues(t, 33002, selection.Account.ID)
	require.False(t, decision.StickyPreviousHit)
	bound, err = store.GetResponseAccount(ctx, groupID, responseID)
	require.NoError(t, err)
	require.EqualValues(t, 33001, bound, "被排除账号的响应绑定不应被改写")

	// 5) pin miss + 同 session：状态请求重选仍遵循 sticky 语义——绑定跟随
	//    实际选中的新账号（同组），不产生跨组/跨平台漂移。
	repo = newRepo()
	repo.accounts[0].Schedulable = false
	svc, store = buildSvc(t, repo)
	sessionHash := "sesspoolh00000001"
	require.NoError(t, store.BindResponseAccount(ctx, groupID, responseID, 33001, time.Hour))
	require.NoError(t, svc.BindStickySession(ctx, &groupID, sessionHash, 33001))
	selection, decision, err = svc.SelectAccountWithScheduler(ctx, &groupID, responseID, sessionHash, model, nil, OpenAIUpstreamTransportAny, false)
	require.NoError(t, err)
	require.NotNil(t, selection)
	require.EqualValues(t, 33002, selection.Account.ID)
	require.False(t, decision.StickyPreviousHit)
	poolCache, cacheAsserted := svc.cache.(*compositePoolTestCache)
	require.True(t, cacheAsserted)
	bound, ok := poolCache.binding(groupID, "openai:"+sessionHash)
	require.True(t, ok)
	require.EqualValues(t, 33002, bound, "状态请求重选后 session 绑定应跟随新账号")
}

// ---- 高级调度器 sticky 的池成员校验 ----

func TestOpenAICompositePool_AdvancedSchedulerStickyRespectsPoolMembership(t *testing.T) {
	groupID := int64(22010)
	sessionHash := "sesspoolf00000001"
	model := "gpt-5.1"

	repo := &compositePoolTestRepo{accounts: []Account{
		compositePoolTestAccount(33001, PlatformOpenAI, groupID, 0, nil),
		compositePoolTestAccount(33002, PlatformDeepseek, groupID, 5, map[string]any{"gpt-5.1": "deepseek-chat"}),
		compositePoolTestAccount(33003, PlatformKimi, groupID, 0, map[string]any{"gpt-5.1": "kimi-chat"}),
	}}

	// sticky 命中池内账号：保持粘连。
	cache := &compositePoolTestCache{}
	svc := newCompositePoolAdvancedTestService(t, repo.accounts, cache)
	ctx := WithCompositeCandidatePlatforms(context.Background(), []string{PlatformDeepseek, PlatformOpenAI})
	require.NoError(t, svc.BindStickySession(ctx, &groupID, sessionHash, 33001))
	selection, decision, err := svc.SelectAccountWithScheduler(ctx, &groupID, "", sessionHash, model, nil, OpenAIUpstreamTransportAny, false)
	require.NoError(t, err)
	require.EqualValues(t, 33001, selection.Account.ID)
	require.True(t, decision.StickySessionHit)

	// sticky 指向池外账号（kimi 不在候选池）：绑定清理，回落池内选号。
	cache2 := &compositePoolTestCache{}
	svc2 := newCompositePoolAdvancedTestService(t, repo.accounts, cache2)
	require.NoError(t, svc2.BindStickySession(ctx, &groupID, sessionHash, 33003))
	selection, decision, err = svc2.SelectAccountWithScheduler(ctx, &groupID, "", sessionHash, model, nil, OpenAIUpstreamTransportAny, false)
	require.NoError(t, err)
	require.NotNil(t, selection)
	require.NotEqualValues(t, 33003, selection.Account.ID)
	require.False(t, decision.StickySessionHit)
	require.Contains(t, cache2.delKeys, "openai:"+sessionHash)
}

// ---- sticky 解绑/映射移除后的组归属与资格复检 ----
//
// 绑定的 sticky 账号从同组解绑（GroupIDs 变更）或 model_mapping 不再 claim 后，
// 命中缓存的 sticky 账号不得因「平台属于池 + 可调度」而继续命中：
// 组归属经 openAIAccountMatchesSchedulingGroup / openAIStickyAccountMatchesGroup
// （GroupIDs/AccountGroups）复检，模型资格经 claims 谓词复检；随后绑定清理。

func TestOpenAICompositePool_StickyMissAfterAccountLeavesGroupOrLosesMapping(t *testing.T) {
	groupID := int64(22011)
	otherGroup := int64(22099)
	sessionHash := "sesspoolg00000001"
	model := "my-model"

	newAccounts := func() []Account {
		return []Account{
			compositePoolTestAccount(33001, PlatformDeepseek, groupID, 0, map[string]any{"my-model": "deepseek-chat"}),
			compositePoolTestAccount(33002, PlatformOpenAI, groupID, 5, map[string]any{"my-model": "gpt-4o"}),
		}
	}
	ctx := WithCompositeCandidatePlatforms(context.Background(), []string{PlatformDeepseek, PlatformOpenAI})

	t.Run("advanced_scheduler_sticky", func(t *testing.T) {
		repo := &compositePoolTestRepo{accounts: newAccounts()}
		cache := &compositePoolTestCache{}
		svc := newCompositePoolAdvancedTestService(t, repo.accounts, cache)
		require.NoError(t, svc.BindStickySession(ctx, &groupID, sessionHash, 33001))

		// 基线：sticky 命中。
		selection, decision, err := svc.SelectAccountWithScheduler(ctx, &groupID, "", sessionHash, model, nil, OpenAIUpstreamTransportAny, false)
		require.NoError(t, err)
		require.EqualValues(t, 33001, selection.Account.ID)
		require.True(t, decision.StickySessionHit)

		// 账号被移出本组：sticky 不得继续命中；绑定先清理、再随重选改指组内
		// 另一平台账号（跨平台 failover 粘新账号）。
		repo.accounts[0].GroupIDs = []int64{otherGroup}
		selection, decision, err = svc.SelectAccountWithScheduler(ctx, &groupID, "", sessionHash, model, nil, OpenAIUpstreamTransportAny, false)
		require.NoError(t, err)
		require.NotNil(t, selection)
		require.NotEqualValues(t, 33001, selection.Account.ID)
		require.False(t, decision.StickySessionHit)
		rebound, bound := cache.binding(groupID, "openai:"+sessionHash)
		require.True(t, bound)
		require.NotEqualValues(t, 33001, rebound, "解绑账号的绑定应被清理并改指新账号")

		// mapping 移除（不再 claim my-model）：sticky 同样不得命中；资格失格属
		// 瞬态语义，绑定按原策略保留在原账号上等待恢复，同时报无容量错误。
		repo.accounts[1].Credentials = nil
		_, decision, err = svc.SelectAccountWithScheduler(ctx, &groupID, "", sessionHash, model, nil, OpenAIUpstreamTransportAny, false)
		require.Error(t, err, "组内已无可 claim 该模型的账号，应报无容量")
		require.True(t, errors.Is(err, ErrNoAvailableAccounts))
		require.False(t, decision.StickySessionHit)
		stillBound, bound := cache.binding(groupID, "openai:"+sessionHash)
		require.True(t, bound)
		require.EqualValues(t, 33002, stillBound)
	})

	t.Run("legacy_sticky", func(t *testing.T) {
		repo := &compositePoolTestRepo{accounts: newAccounts()}
		cache := &compositePoolTestCache{}
		svc := newCompositePoolTestService(repo.accounts, cache)
		require.NoError(t, svc.BindStickySession(ctx, &groupID, sessionHash, 33001))

		selection, err := svc.SelectAccountForModelWithExclusions(ctx, &groupID, sessionHash, model, nil)
		require.NoError(t, err)
		require.EqualValues(t, 33001, selection.ID)

		// 移出本组：legacy sticky 命中路径同样复检组归属并清理绑定，
		// 随后重选改指组内另一平台账号。
		repo.accounts[0].GroupIDs = []int64{otherGroup}
		selection, err = svc.SelectAccountForModelWithExclusions(ctx, &groupID, sessionHash, model, nil)
		require.NoError(t, err)
		require.NotEqualValues(t, 33001, selection.ID)
		rebound, bound := cache.binding(groupID, "openai:"+sessionHash)
		require.True(t, bound)
		require.EqualValues(t, 33002, rebound, "解绑账号的绑定应被清理并改指新账号")
	})
}

// ---- 显式 force 平台语义：resolved 平台优先于池，池不得绕过 ----

func TestOpenAICompositePool_ExplicitForcePlatformOverridesPool(t *testing.T) {
	groupID := int64(22012)
	pool := []string{PlatformKimi, PlatformOpenAI}
	resolvedCtx := WithResolvedTargetPlatform(
		WithCompositeCandidatePlatforms(context.Background(), pool),
		PlatformDeepseek,
	)

	// resolved 存在时池视为不活跃：平台门回退到单平台相等语义。
	require.False(t, openAICompositePoolActive(resolvedCtx))
	deepseekAccount := compositePoolTestAccount(36001, PlatformDeepseek, groupID, 0, map[string]any{"my-model": "deepseek-chat"})
	kimiAccount := compositePoolTestAccount(36002, PlatformKimi, groupID, 0, map[string]any{"my-model": "kimi-chat"})
	require.True(t, openAISchedulingPlatformMatchesAccount(resolvedCtx, &deepseekAccount, PlatformDeepseek))
	require.False(t, openAISchedulingPlatformMatchesAccount(resolvedCtx, &kimiAccount, PlatformDeepseek))

	// listSchedulableAccounts 只读取 resolved 平台桶，不合并候选池。
	repo := &compositePoolTestRepo{accounts: []Account{deepseekAccount, kimiAccount}}
	svc := newCompositePoolTestService(repo.accounts, &compositePoolTestCache{})
	accounts, err := svc.listSchedulableAccounts(resolvedCtx, &groupID, PlatformDeepseek)
	require.NoError(t, err)
	require.Len(t, accounts, 1)
	require.EqualValues(t, 36001, accounts[0].ID)

	// 选号结果同样只落在 resolved 平台账号上。
	selected, err := svc.selectAccountForModelWithExclusions(resolvedCtx, &groupID, PlatformDeepseek, "", "my-model", nil, false, 0, "", false)
	require.NoError(t, err)
	require.EqualValues(t, 36001, selected.ID)
}

// ---- 选号期 passthrough 例外（review#6）：残留旧 mapping 不误踢、构池不放宽 ----

func TestOpenAICompositePool_PassthroughStaleMappingSelectionException(t *testing.T) {
	resetOpenAIAdvancedSchedulerSettingCacheForTest()
	defer resetOpenAIAdvancedSchedulerSettingCacheForTest()

	newPassthroughAccount := func() Account {
		// passthrough 开启 + credentials 残留旧的非空 mapping（#4936 场景），
		// 残留 mapping 不 claim 请求模型。
		account := compositePoolTestAccount(37001, PlatformOpenAI, 22013, 0, map[string]any{"old-model": "legacy-upstream"})
		account.Extra = map[string]any{"openai_passthrough": true}
		return account
	}

	// 构池/ownership 谓词保持严格：passthrough 残留 mapping 未命中不得 claim。
	passthroughAccount := newPassthroughAccount()
	require.True(t, passthroughAccount.IsOpenAIPassthroughEnabled())
	require.False(t, CompositeAccountClaimsModel(&passthroughAccount, "my-model"),
		"构池谓词不得因 passthrough 放宽，否则 passthrough 会 claim 所有模型")

	// 候选池含 openai：选号期 passthrough 例外生效，残留 mapping 未命中仍入选。
	groupID := int64(22013)
	poolCtx := WithCompositeCandidatePlatforms(context.Background(), []string{PlatformOpenAI})
	strictPeer := compositePoolTestAccount(37002, PlatformOpenAI, 22013, 0, map[string]any{"my-model": "gpt-4o"})
	svc := newCompositePoolTestService([]Account{passthroughAccount, strictPeer}, &compositePoolTestCache{})
	selected, err := svc.SelectAccountForModelWithExclusions(poolCtx, &groupID, "", "my-model", nil)
	require.NoError(t, err)
	require.EqualValues(t, 37001, selected.ID,
		"passthrough 残留 mapping 未命中应经选号期例外入选（peer priority 相同，按池序先到）")

	// 无 openai 候选：passthrough openai 账号不得越池，选中池内 deepseek。
	repoCtx := WithCompositeCandidatePlatforms(context.Background(), []string{PlatformDeepseek})
	deepseekPeer := compositePoolTestAccount(37003, PlatformDeepseek, 22013, 0, map[string]any{"my-model": "deepseek-chat"})
	svc2 := newCompositePoolTestService([]Account{newPassthroughAccount(), deepseekPeer}, &compositePoolTestCache{})
	selected, err = svc2.SelectAccountForModelWithExclusions(repoCtx, &groupID, "", "my-model", nil)
	require.NoError(t, err)
	require.EqualValues(t, 37003, selected.ID, "passthrough 不得越出候选池")

	// 池内无任何可服务账号（仅 passthrough 残留 mapping 且其平台不在池）：无容量。
	onlyPassthroughCtx := WithCompositeCandidatePlatforms(context.Background(), []string{PlatformDeepseek})
	svc3 := newCompositePoolTestService([]Account{newPassthroughAccount()}, &compositePoolTestCache{})
	_, err = svc3.SelectAccountForModelWithExclusions(onlyPassthroughCtx, &groupID, "", "my-model", nil)
	require.Error(t, err)
	require.True(t, errors.Is(err, ErrNoAvailableAccounts))
}

// ---- composite account_pool 下 CN/OpenCode 的 previous_response_id 归属 ----
//
// 底层事实：bindHTTPResponseAccount（openai_gateway_response_handling.go）对
// CN/OpenCode native HTTP Responses 同样经 OpenAIWSStateStore.BindResponseAccount
// 写绑定，但 resolveAccountByPreviousResponseIDForCapability 原来只认 OpenAI 平台。
// 放宽仅限「池上下文 + OpenAI 兼容族 APIKey」；OAuth/SetupToken/WS/单平台规则
// 不扩大，models/group/pool/health/capability/excluded 全部原门保留。

func TestOpenAICompositePool_PreviousResponseIDCNAndOpenCodePoolOwners(t *testing.T) {
	resetOpenAIAdvancedSchedulerSettingCacheForTest()
	defer resetOpenAIAdvancedSchedulerSettingCacheForTest()

	groupID := int64(22014)
	responseID := "resp_pool_cn_0001"
	model := "my-model"

	newAccounts := func() []Account {
		return []Account{
			compositePoolTestAccount(38001, PlatformDeepseek, groupID, 0, map[string]any{"my-model": "deepseek-chat"}),
			compositePoolTestAccount(38002, PlatformKimi, groupID, 0, map[string]any{"my-model": "kimi-chat"}),
			compositePoolTestAccount(38003, PlatformOpenCodeGo, groupID, 0, map[string]any{"my-model": "oc-chat"}),
		}
	}
	buildSvc := func(t *testing.T, accounts []Account) (*OpenAIGatewayService, OpenAIWSStateStore) {
		t.Helper()
		svc := newCompositePoolAdvancedTestService(t, accounts, &compositePoolTestCache{})
		store := svc.getOpenAIWSStateStore()
		require.NotNil(t, store)
		return svc, store
	}
	poolCtx := WithCompositeCandidatePlatforms(context.Background(), []string{PlatformDeepseek, PlatformKimi, PlatformOpenCodeGo})

	// 1) CN（deepseek APIKey）有效绑定：命中并原 key 续接。
	repo := &compositePoolTestRepo{accounts: newAccounts()}
	svc, store := buildSvc(t, repo.accounts)
	require.NoError(t, store.BindResponseAccount(poolCtx, groupID, responseID, 38001, time.Hour))
	selection, decision, err := svc.SelectAccountWithScheduler(poolCtx, &groupID, responseID, "", model, nil, OpenAIUpstreamTransportAny, false)
	require.NoError(t, err)
	require.NotNil(t, selection)
	require.EqualValues(t, 38001, selection.Account.ID)
	require.True(t, decision.StickyPreviousHit, "CN 有效 owner 应命中 pin")
	bound, err := store.GetResponseAccount(poolCtx, groupID, responseID)
	require.NoError(t, err)
	require.EqualValues(t, 38001, bound, "命中后原 key 续接同账号")

	// 2) OpenCode APIKey 有效绑定：同样命中。
	repo = &compositePoolTestRepo{accounts: newAccounts()}
	svc, store = buildSvc(t, repo.accounts)
	require.NoError(t, store.BindResponseAccount(poolCtx, groupID, responseID, 38003, time.Hour))
	selection, decision, err = svc.SelectAccountWithScheduler(poolCtx, &groupID, responseID, "", model, nil, OpenAIUpstreamTransportAny, false)
	require.NoError(t, err)
	require.NotNil(t, selection)
	require.EqualValues(t, 38003, selection.Account.ID)
	require.True(t, decision.StickyPreviousHit, "OpenCode 有效 owner 应命中 pin")

	// 3) 池外：绑定指向 kimi 但本次池只有 deepseek——无 hit、绑定保留、不跨池带走。
	repo = &compositePoolTestRepo{accounts: newAccounts()}
	svc, store = buildSvc(t, repo.accounts)
	require.NoError(t, store.BindResponseAccount(poolCtx, groupID, responseID, 38002, time.Hour))
	narrowCtx := WithCompositeCandidatePlatforms(context.Background(), []string{PlatformDeepseek})
	selection, decision, err = svc.SelectAccountWithScheduler(narrowCtx, &groupID, responseID, "", model, nil, OpenAIUpstreamTransportAny, false)
	require.NoError(t, err)
	require.NotNil(t, selection)
	require.EqualValues(t, 38001, selection.Account.ID)
	require.False(t, decision.StickyPreviousHit)
	bound, err = store.GetResponseAccount(poolCtx, groupID, responseID)
	require.NoError(t, err)
	require.EqualValues(t, 38002, bound, "池外健康账号绑定保留")

	// 4) 解绑（移出本组）：无 hit、绑定保留、重选遵循组内 sticky 语义。
	repo = &compositePoolTestRepo{accounts: newAccounts()}
	svc, store = buildSvc(t, repo.accounts)
	require.NoError(t, store.BindResponseAccount(poolCtx, groupID, responseID, 38001, time.Hour))
	repo.accounts[0].GroupIDs = []int64{22099}
	selection, decision, err = svc.SelectAccountWithScheduler(poolCtx, &groupID, responseID, "", model, nil, OpenAIUpstreamTransportAny, false)
	require.NoError(t, err)
	require.NotNil(t, selection)
	require.NotEqualValues(t, 38001, selection.Account.ID)
	require.False(t, decision.StickyPreviousHit)
	bound, err = store.GetResponseAccount(poolCtx, groupID, responseID)
	require.NoError(t, err)
	require.EqualValues(t, 38001, bound, "解绑账号绑定保留（组归属复检在选号层拒绝）")

	// 5) mapping 不支持：绑定账号不再 claim 请求模型——无 hit、绑定保留。
	repo = &compositePoolTestRepo{accounts: newAccounts()}
	svc, store = buildSvc(t, repo.accounts)
	require.NoError(t, store.BindResponseAccount(poolCtx, groupID, responseID, 38001, time.Hour))
	repo.accounts[0].Credentials = map[string]any{"model_mapping": map[string]any{"gpt-5.1": "x"}}
	selection, decision, err = svc.SelectAccountWithScheduler(poolCtx, &groupID, responseID, "", model, nil, OpenAIUpstreamTransportAny, false)
	require.NoError(t, err)
	require.NotNil(t, selection)
	require.NotEqualValues(t, 38001, selection.Account.ID)
	require.False(t, decision.StickyPreviousHit)
	bound, err = store.GetResponseAccount(poolCtx, groupID, responseID)
	require.NoError(t, err)
	require.EqualValues(t, 38001, bound, "mapping 失格属瞬态，绑定保留")

	// 6) expired（AutoPauseOnExpired 到期）：无 hit 且按失效原策略删除绑定。
	repo = &compositePoolTestRepo{accounts: newAccounts()}
	svc, store = buildSvc(t, repo.accounts)
	require.NoError(t, store.BindResponseAccount(poolCtx, groupID, responseID, 38001, time.Hour))
	expiredAt := time.Now().Add(-time.Hour)
	repo.accounts[0].AutoPauseOnExpired = true
	repo.accounts[0].ExpiresAt = &expiredAt
	selection, decision, err = svc.SelectAccountWithScheduler(poolCtx, &groupID, responseID, "", model, nil, OpenAIUpstreamTransportAny, false)
	require.NoError(t, err)
	require.NotNil(t, selection)
	require.False(t, decision.StickyPreviousHit)
	bound, err = store.GetResponseAccount(poolCtx, groupID, responseID)
	require.NoError(t, err)
	require.EqualValues(t, 0, bound, "失效账号绑定按原策略删除")

	// 7) excluded：无 hit、绑定保留。
	repo = &compositePoolTestRepo{accounts: newAccounts()}
	svc, store = buildSvc(t, repo.accounts)
	require.NoError(t, store.BindResponseAccount(poolCtx, groupID, responseID, 38001, time.Hour))
	excluded := map[int64]struct{}{38001: {}}
	selection, decision, err = svc.SelectAccountWithScheduler(poolCtx, &groupID, responseID, "", model, excluded, OpenAIUpstreamTransportAny, false)
	require.NoError(t, err)
	require.NotNil(t, selection)
	require.NotEqualValues(t, 38001, selection.Account.ID)
	require.False(t, decision.StickyPreviousHit)
	bound, err = store.GetResponseAccount(poolCtx, groupID, responseID)
	require.NoError(t, err)
	require.EqualValues(t, 38001, bound, "被排除账号绑定保留")

	// 8) 单平台（kimi、无池）：pin 层不尝试，规则不扩大。
	repo = &compositePoolTestRepo{accounts: newAccounts()}
	svc, store = buildSvc(t, repo.accounts)
	require.NoError(t, store.BindResponseAccount(poolCtx, groupID, responseID, 38002, time.Hour))
	singleCtx := context.Background()
	selection, decision, err = svc.selectAccountWithScheduler(singleCtx, &groupID, responseID, "", model, nil, OpenAIUpstreamTransportAny, "", "", false, PlatformKimi, false, true)
	require.NoError(t, err)
	require.NotNil(t, selection)
	require.False(t, decision.StickyPreviousHit, "单平台 kimi 不得启用 OpenAI previous_response pin")
	bound, err = store.GetResponseAccount(poolCtx, groupID, responseID)
	require.NoError(t, err)
	require.EqualValues(t, 38002, bound)

	// 9) OpenAI OAuth（非 APIKey、非 WSv2）：状态归属 WSv2 session，规则不变。
	oauthAccount := compositePoolTestAccount(38004, PlatformOpenAI, groupID, 0, nil)
	oauthAccount.Type = AccountTypeOAuth
	repo = &compositePoolTestRepo{accounts: append(newAccounts(), oauthAccount)}
	svc, store = buildSvc(t, repo.accounts)
	require.NoError(t, store.BindResponseAccount(poolCtx, groupID, responseID, 38004, time.Hour))
	selection, decision, err = svc.SelectAccountWithScheduler(poolCtx, &groupID, responseID, "", model, nil, OpenAIUpstreamTransportAny, false)
	require.NoError(t, err)
	require.NotNil(t, selection)
	require.False(t, decision.StickyPreviousHit, "OAuth 续链状态属 WSv2 session，HTTP 池路径不得复用")
	bound, err = store.GetResponseAccount(poolCtx, groupID, responseID)
	require.NoError(t, err)
	require.EqualValues(t, 38004, bound, "OAuth 绑定保留，不删除")

	// 10) Grok APIKey 有效 owner（HTTP 绑定）：与 CN/OpenCode 同权命中。
	// IsMultiProtocolAPIKeyProvider 不含 grok，兼容族判定必须用 IsOpenAICompatible。
	grokAccount := compositePoolTestAccount(38005, PlatformGrok, groupID, 0, map[string]any{"my-model": "grok-chat"})
	repo = &compositePoolTestRepo{accounts: append(newAccounts(), grokAccount)}
	svc, store = buildSvc(t, repo.accounts)
	grokPoolCtx := WithCompositeCandidatePlatforms(context.Background(), []string{PlatformDeepseek, PlatformGrok})
	require.NoError(t, store.BindResponseAccount(grokPoolCtx, groupID, responseID, 38005, time.Hour))
	selection, decision, err = svc.SelectAccountWithScheduler(grokPoolCtx, &groupID, responseID, "", model, nil, OpenAIUpstreamTransportAny, false)
	require.NoError(t, err)
	require.NotNil(t, selection)
	require.EqualValues(t, 38005, selection.Account.ID)
	require.True(t, decision.StickyPreviousHit, "grok APIKey 有效 owner 应命中 pin")
	bound, err = store.GetResponseAccount(grokPoolCtx, groupID, responseID)
	require.NoError(t, err)
	require.EqualValues(t, 38005, bound, "grok 命中后原 key 续接")

	// 11) Grok 池外（本次池仅 deepseek）：无 hit，绑定保留——不得仅因非 OpenAI 被删。
	repo = &compositePoolTestRepo{accounts: append(newAccounts(), grokAccount)}
	svc, store = buildSvc(t, repo.accounts)
	require.NoError(t, store.BindResponseAccount(grokPoolCtx, groupID, responseID, 38005, time.Hour))
	selection, decision, err = svc.SelectAccountWithScheduler(narrowCtx, &groupID, responseID, "", model, nil, OpenAIUpstreamTransportAny, false)
	require.NoError(t, err)
	require.NotNil(t, selection)
	require.False(t, decision.StickyPreviousHit)
	bound, err = store.GetResponseAccount(grokPoolCtx, groupID, responseID)
	require.NoError(t, err)
	require.EqualValues(t, 38005, bound, "grok 池外健康 owner 绑定保留，删除不得仅因非 OpenAI")
}

// ---- legacy 调度（高级调度器关闭）+ active pool 的 previous_response_id 归属 ----
//
// 与 TestOpenAICompositePool_PreviousResponseIDCNAndOpenCodePoolOwners 对偶：
// legacy 分支原本完全忽略 prevID，现仅对「active pool + 非空 prevID」复用同一
// pin 选择与复检 helper；非池 legacy 行为不变。覆盖 CN/OpenCode/grok valid pin
// （含适用 capability）、pin 缺失、池外、excluded、owner 不可用。

func TestOpenAICompositePool_LegacySchedulerPoolPreviousResponseIDOwners(t *testing.T) {
	resetOpenAIAdvancedSchedulerSettingCacheForTest()
	defer resetOpenAIAdvancedSchedulerSettingCacheForTest()

	groupID := int64(22015)
	responseID := "resp_pool_legacy_0001"
	model := "my-model"

	newAccounts := func() []Account {
		return []Account{
			compositePoolTestAccount(39001, PlatformDeepseek, groupID, 0, map[string]any{"my-model": "deepseek-chat"}),
			compositePoolTestAccount(39002, PlatformKimi, groupID, 0, map[string]any{"my-model": "kimi-chat"}),
			compositePoolTestAccount(39003, PlatformOpenCodeGo, groupID, 0, map[string]any{"my-model": "oc-chat"}),
		}
	}
	// legacy：不注入 advanced 设置（默认关闭）。
	buildSvc := func(t *testing.T, accounts []Account) (*OpenAIGatewayService, OpenAIWSStateStore) {
		t.Helper()
		svc := newCompositePoolTestService(accounts, &compositePoolTestCache{})
		require.False(t, svc.isOpenAIAdvancedSchedulerEnabled(context.Background()))
		store := svc.getOpenAIWSStateStore()
		require.NotNil(t, store)
		return svc, store
	}
	poolCtx := WithCompositeCandidatePlatforms(context.Background(), []string{PlatformDeepseek, PlatformKimi, PlatformOpenCodeGo})

	// 1) CN valid pin（含适用 capability=Responses）：legacy 命中并原 key 续接。
	repo := &compositePoolTestRepo{accounts: newAccounts()}
	svc, store := buildSvc(t, repo.accounts)
	require.NoError(t, store.BindResponseAccount(poolCtx, groupID, responseID, 39001, time.Hour))
	selection, decision, err := svc.selectAccountWithScheduler(poolCtx, &groupID, responseID, "", model, nil,
		OpenAIUpstreamTransportAny, OpenAIEndpointCapabilityResponses, "", false, PlatformOpenAI, false, true)
	require.NoError(t, err)
	require.NotNil(t, selection)
	require.EqualValues(t, 39001, selection.Account.ID)
	require.True(t, decision.StickyPreviousHit, "legacy+pool 的 CN 有效 owner 应命中 pin")
	bound, err := store.GetResponseAccount(poolCtx, groupID, responseID)
	require.NoError(t, err)
	require.EqualValues(t, 39001, bound)

	// 2) OpenCode valid pin：legacy 命中。
	repo = &compositePoolTestRepo{accounts: newAccounts()}
	svc, store = buildSvc(t, repo.accounts)
	require.NoError(t, store.BindResponseAccount(poolCtx, groupID, responseID, 39003, time.Hour))
	selection, decision, err = svc.SelectAccountWithScheduler(poolCtx, &groupID, responseID, "", model, nil, OpenAIUpstreamTransportAny, false)
	require.NoError(t, err)
	require.NotNil(t, selection)
	require.EqualValues(t, 39003, selection.Account.ID)
	require.True(t, decision.StickyPreviousHit)

	// 3) grok valid pin：legacy 命中（兼容族判定含 grok）。
	grokAccount := compositePoolTestAccount(39004, PlatformGrok, groupID, 0, map[string]any{"my-model": "grok-chat"})
	repo = &compositePoolTestRepo{accounts: append(newAccounts(), grokAccount)}
	svc, store = buildSvc(t, repo.accounts)
	grokPoolCtx := WithCompositeCandidatePlatforms(context.Background(), []string{PlatformDeepseek, PlatformGrok})
	require.NoError(t, store.BindResponseAccount(grokPoolCtx, groupID, responseID, 39004, time.Hour))
	selection, decision, err = svc.SelectAccountWithScheduler(grokPoolCtx, &groupID, responseID, "", model, nil, OpenAIUpstreamTransportAny, false)
	require.NoError(t, err)
	require.NotNil(t, selection)
	require.EqualValues(t, 39004, selection.Account.ID)
	require.True(t, decision.StickyPreviousHit)

	// 4) pin 缺失（未绑定）：无 hit，回落 LB 按 priority 选中。
	repo = &compositePoolTestRepo{accounts: newAccounts()}
	svc, _ = buildSvc(t, repo.accounts)
	repo.accounts[1].Priority = -1
	selection, decision, err = svc.SelectAccountWithScheduler(poolCtx, &groupID, responseID, "", model, nil, OpenAIUpstreamTransportAny, false)
	require.NoError(t, err)
	require.NotNil(t, selection)
	require.EqualValues(t, 39002, selection.Account.ID)
	require.False(t, decision.StickyPreviousHit)

	// 5) 池外：绑定指向 kimi 但本次池只有 deepseek——无 hit、绑定保留。
	repo = &compositePoolTestRepo{accounts: newAccounts()}
	svc, store = buildSvc(t, repo.accounts)
	require.NoError(t, store.BindResponseAccount(poolCtx, groupID, responseID, 39002, time.Hour))
	narrowCtx := WithCompositeCandidatePlatforms(context.Background(), []string{PlatformDeepseek})
	selection, decision, err = svc.SelectAccountWithScheduler(narrowCtx, &groupID, responseID, "", model, nil, OpenAIUpstreamTransportAny, false)
	require.NoError(t, err)
	require.NotNil(t, selection)
	require.EqualValues(t, 39001, selection.Account.ID)
	require.False(t, decision.StickyPreviousHit)
	bound, err = store.GetResponseAccount(poolCtx, groupID, responseID)
	require.NoError(t, err)
	require.EqualValues(t, 39002, bound, "池外健康 owner 绑定保留，不得仅因非 OpenAI 被删")

	// 6) excluded：无 hit、绑定保留。
	repo = &compositePoolTestRepo{accounts: newAccounts()}
	svc, store = buildSvc(t, repo.accounts)
	require.NoError(t, store.BindResponseAccount(poolCtx, groupID, responseID, 39001, time.Hour))
	excluded := map[int64]struct{}{39001: {}}
	selection, decision, err = svc.SelectAccountWithScheduler(poolCtx, &groupID, responseID, "", model, excluded, OpenAIUpstreamTransportAny, false)
	require.NoError(t, err)
	require.NotNil(t, selection)
	require.NotEqualValues(t, 39001, selection.Account.ID)
	require.False(t, decision.StickyPreviousHit)
	bound, err = store.GetResponseAccount(poolCtx, groupID, responseID)
	require.NoError(t, err)
	require.EqualValues(t, 39001, bound)

	// 7) owner 不可用：无 hit、绑定按失效原策略删除、LB 选中另一账号。
	repo = &compositePoolTestRepo{accounts: newAccounts()}
	svc, store = buildSvc(t, repo.accounts)
	require.NoError(t, store.BindResponseAccount(poolCtx, groupID, responseID, 39001, time.Hour))
	repo.accounts[0].Schedulable = false
	selection, decision, err = svc.SelectAccountWithScheduler(poolCtx, &groupID, responseID, "", model, nil, OpenAIUpstreamTransportAny, false)
	require.NoError(t, err)
	require.NotNil(t, selection)
	require.EqualValues(t, 39002, selection.Account.ID)
	require.False(t, decision.StickyPreviousHit)
	bound, err = store.GetResponseAccount(poolCtx, groupID, responseID)
	require.NoError(t, err)
	require.EqualValues(t, 0, bound, "失效 owner 绑定按原策略删除")

	// 8) 非池 legacy（无池 ctx）：pin 层不进入；platform=openai 的 legacy 在
	//    CN-only repo 上本就无容量（原行为），CN 绑定不受影响。
	repo = &compositePoolTestRepo{accounts: newAccounts()}
	svc, store = buildSvc(t, repo.accounts)
	require.NoError(t, store.BindResponseAccount(poolCtx, groupID, responseID, 39001, time.Hour))
	_, decision, err = svc.SelectAccountWithScheduler(context.Background(), &groupID, responseID, "", model, nil, OpenAIUpstreamTransportAny, false)
	require.Error(t, err)
	require.True(t, errors.Is(err, ErrNoAvailableAccounts))
	require.False(t, decision.StickyPreviousHit, "非池 legacy 不进入 pin 层")
	bound, err = store.GetResponseAccount(poolCtx, groupID, responseID)
	require.NoError(t, err)
	require.EqualValues(t, 39001, bound)
}
