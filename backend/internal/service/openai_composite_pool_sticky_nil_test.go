package service

// 回归：sticky 命中的账号经 getSchedulableAccount 返回 (nil, nil) 时不得 panic。
//
// getSchedulableAccount 合法返回 (nil, nil)（snapshot miss / 调度阈值暂停 /
// grok 免费额度耗尽均收敛为 nil, nil）。composite 平台 denied 检查
// compositePoolPlatformDenied(ctx, account.Platform) 若位于 nil 判定之前，
// 读取 account.Platform 即 nil deref —— 非 composite 池请求同样 500。
//
// 覆盖：
//   a. legacy 无 batch 路径（SelectAccountForModelWithExclusions →
//      tryStickySessionHit）：nil 账号跳过 sticky 尝试（不删绑定），正常选号
//      命中健康替代账号；
//   b. legacy 负载 batch 路径（SelectAccountWithLoadAwareness → Layer 1
//      sticky）：nil 账号降级落 Layer 2，选中健康替代账号；
//   c. 非 pool / pool / pool+denied 三种 ctx 均不 panic；
//   d. advanced scheduler 路径在 getSchedulableAccount 之后先做 err/nil 双判
//      （openai_account_scheduler.go sticky 链路），本就 nil 安全，作回归对照；
//      不为测试改变任何绑定策略。

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

// stickyNilAccountRepo 让指定账号在 GetByID 时返回 (nil, nil)，其余行为与
// compositePoolTestRepo 完全一致，模拟 getSchedulableAccount 的合法 nil 返回。
// 该账号对调度 listing 同样不可见：生产 listSchedulableAccountsSinglePlatform
// 对阈值暂停 / grok 额度耗尽账号本就做同样过滤，listing 与 GetByID 保持一致。
type stickyNilAccountRepo struct {
	*compositePoolTestRepo
	nilIDs map[int64]struct{}
}

func (r *stickyNilAccountRepo) GetByID(ctx context.Context, id int64) (*Account, error) {
	if _, vanished := r.nilIDs[id]; vanished {
		return nil, nil
	}
	return r.compositePoolTestRepo.GetByID(ctx, id)
}

func (r *stickyNilAccountRepo) visibleAccounts(accounts []Account) []Account {
	out := make([]Account, 0, len(accounts))
	for _, acc := range accounts {
		if _, vanished := r.nilIDs[acc.ID]; vanished {
			continue
		}
		out = append(out, acc)
	}
	return out
}

func (r *stickyNilAccountRepo) ListSchedulableByPlatform(ctx context.Context, platform string) ([]Account, error) {
	accounts, err := r.compositePoolTestRepo.ListSchedulableByPlatform(ctx, platform)
	return r.visibleAccounts(accounts), err
}

func (r *stickyNilAccountRepo) ListSchedulableByGroupIDAndPlatform(ctx context.Context, groupID int64, platform string) ([]Account, error) {
	accounts, err := r.compositePoolTestRepo.ListSchedulableByGroupIDAndPlatform(ctx, groupID, platform)
	return r.visibleAccounts(accounts), err
}

func (r *stickyNilAccountRepo) ListSchedulableUngroupedByPlatform(ctx context.Context, platform string) ([]Account, error) {
	accounts, err := r.compositePoolTestRepo.ListSchedulableUngroupedByPlatform(ctx, platform)
	return r.visibleAccounts(accounts), err
}

// newStickyNilDerefTestService 基于既有 composite pool fixture 构造服务，并把
// accountRepo 换成对指定账号返回 (nil, nil) 的包装 repo。
func newStickyNilDerefTestService(accounts []Account, cache GatewayCache, vanishedAccountID int64) *OpenAIGatewayService {
	svc := newCompositePoolTestService(accounts, cache)
	svc.accountRepo = &stickyNilAccountRepo{
		compositePoolTestRepo: &compositePoolTestRepo{accounts: accounts},
		nilIDs:                map[int64]struct{}{vanishedAccountID: {}},
	}
	return svc
}

func stickyNilDerefTestContexts() map[string]func() context.Context {
	return map[string]func() context.Context{
		// 非 composite 池：plain ctx 也曾 panic（deref 发生在 denied 判定取参之前）。
		"plain_ctx_no_pool": func() context.Context { return context.Background() },
		"composite_pool_ctx": func() context.Context {
			return WithCompositeCandidatePlatforms(context.Background(), []string{PlatformDeepseek, PlatformOpenAI})
		},
		"composite_pool_denied_ctx": func() context.Context {
			ctx := WithCompositeCandidatePlatforms(context.Background(), []string{PlatformDeepseek, PlatformOpenAI})
			return WithCompositePoolDeniedPlatforms(ctx, []string{PlatformDeepseek})
		},
	}
}

// stickyNilDerefAccounts：44001 (deepseek) 是 sticky 指向、"查询即消失"的账号；
// 44002 (openai) 是健康替代账号。
func stickyNilDerefAccounts(groupID int64) []Account {
	return []Account{
		compositePoolTestAccount(44001, PlatformDeepseek, groupID, 0, map[string]any{"my-model": "deepseek-chat"}),
		compositePoolTestAccount(44002, PlatformOpenAI, groupID, 0, map[string]any{"my-model": "gpt-4o"}),
	}
}

// ---- a. legacy 无 batch：tryStickySessionHit 命中 (nil, nil) 账号 ----

func TestOpenAICompositePoolStickyNilAccount_LegacyNoBatchFallsBack(t *testing.T) {
	resetOpenAIAdvancedSchedulerSettingCacheForTest()
	defer resetOpenAIAdvancedSchedulerSettingCacheForTest()

	groupID := int64(22021)
	model := "my-model"
	sessionHash := "sessnilstick00001"

	for name, newCtx := range stickyNilDerefTestContexts() {
		t.Run(name, func(t *testing.T) {
			cache := &compositePoolTestCache{}
			svc := newStickyNilDerefTestService(stickyNilDerefAccounts(groupID), cache, 44001)
			ctx := newCtx()
			require.NoError(t, svc.BindStickySession(ctx, &groupID, sessionHash, 44001))

			account, err := svc.SelectAccountForModelWithExclusions(ctx, &groupID, sessionHash, model, nil)
			require.NoError(t, err)
			require.NotNil(t, account)
			require.EqualValues(t, 44002, account.ID)
			// sticky 指向 (nil, nil) 账号：跳过 sticky 尝试而非清绑定；
			// 全程不得出现对该 session 的 delete（绑定由正常选号接管）。
			require.NotContains(t, cache.delKeys, "openai:"+sessionHash)
		})
	}
}

// ---- b. legacy 负载 batch：Layer 1 sticky 命中 (nil, nil) 账号 ----

func TestOpenAICompositePoolStickyNilAccount_LegacyLoadBatchFallsToLayer2(t *testing.T) {
	resetOpenAIAdvancedSchedulerSettingCacheForTest()
	defer resetOpenAIAdvancedSchedulerSettingCacheForTest()

	groupID := int64(22022)
	model := "my-model"
	sessionHash := "sessnilstick00002"

	for name, newCtx := range stickyNilDerefTestContexts() {
		t.Run(name, func(t *testing.T) {
			cache := &compositePoolTestCache{}
			svc := newStickyNilDerefTestService(stickyNilDerefAccounts(groupID), cache, 44001)
			svc.cfg.Gateway.Scheduling.LoadBatchEnabled = true
			ctx := newCtx()
			require.NoError(t, svc.BindStickySession(ctx, &groupID, sessionHash, 44001))

			result, err := svc.SelectAccountWithLoadAwareness(ctx, &groupID, sessionHash, model, nil)
			require.NoError(t, err)
			require.NotNil(t, result)
			require.NotNil(t, result.Account)
			require.EqualValues(t, 44002, result.Account.ID)
		})
	}
}

// ---- d. advanced scheduler 回归对照（本就 nil 安全，不为测试改绑定策略） ----

func TestOpenAICompositePoolStickyNilAccount_AdvancedSchedulerControl(t *testing.T) {
	groupID := int64(22023)
	model := "my-model"
	sessionHash := "sessnilstick00003"

	cache := &compositePoolTestCache{}
	svc := newStickyNilDerefTestService(stickyNilDerefAccounts(groupID), cache, 44001)
	svc.rateLimitService = newOpenAIAdvancedSchedulerRateLimitService("true")
	svc.cfg.Gateway.Scheduling.LoadBatchEnabled = true
	ctx := WithCompositeCandidatePlatforms(context.Background(), []string{PlatformDeepseek, PlatformOpenAI})
	require.NoError(t, svc.BindStickySession(ctx, &groupID, sessionHash, 44001))

	selection, decision, err := svc.SelectAccountWithScheduler(ctx, &groupID, "", sessionHash, model, nil, OpenAIUpstreamTransportAny, false)
	require.NoError(t, err)
	require.NotNil(t, selection)
	require.NotNil(t, selection.Account)
	require.EqualValues(t, 44002, selection.Account.ID)
	require.False(t, decision.StickySessionHit)
}
