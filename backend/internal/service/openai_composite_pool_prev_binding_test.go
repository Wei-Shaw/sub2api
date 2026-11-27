package service

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// composite 账号池要求续链命中归属账号：归属账号限流等瞬态不可用时只跳过本次复用，
// 不删除续链绑定，否则冷却结束后整条会话链被永久拒绝。永久不可用（关闭调度）仍删除。
func TestSelectAccountByPreviousResponseIDKeepsBindingOnTransientOwnerStateInPool(t *testing.T) {
	groupID := int64(31)
	poolCtx := WithCompositeCandidatePlatforms(context.Background(), []string{PlatformDeepseek, PlatformKimi})
	rateLimitedUntil := time.Now().Add(30 * time.Minute)
	newSvc := func(account Account) (*OpenAIGatewayService, OpenAIWSStateStore) {
		cache := &stubGatewayCache{}
		store := NewOpenAIWSStateStore(cache)
		return &OpenAIGatewayService{
			accountRepo:        stubOpenAIAccountRepo{accounts: []Account{account}},
			cache:              cache,
			cfg:                newOpenAIWSV2TestConfig(),
			concurrencyService: NewConcurrencyService(stubConcurrencyCache{}),
			openaiWSStateStore: store,
		}, store
	}

	rateLimited := Account{ID: 41, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Status: StatusActive,
		Schedulable: true, Concurrency: 1, RateLimitResetAt: &rateLimitedUntil}
	svc, store := newSvc(rateLimited)
	require.NoError(t, store.BindResponseAccount(poolCtx, groupID, "resp_pool_rl", rateLimited.ID, time.Hour))
	selection, err := svc.SelectAccountByPreviousResponseID(poolCtx, &groupID, "resp_pool_rl", "gpt-5.1", nil, false)
	require.NoError(t, err)
	require.Nil(t, selection)
	bound, err := store.GetResponseAccount(poolCtx, groupID, "resp_pool_rl")
	require.NoError(t, err)
	require.Equal(t, rateLimited.ID, bound, "瞬态不可用不得删除池内续链绑定")
	require.True(t, svc.HasPreviousResponseBinding(poolCtx, &groupID, "resp_pool_rl"))

	disabled := Account{ID: 42, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Status: StatusActive,
		Schedulable: false, Concurrency: 1}
	svc, store = newSvc(disabled)
	require.NoError(t, store.BindResponseAccount(poolCtx, groupID, "resp_pool_off", disabled.ID, time.Hour))
	selection, err = svc.SelectAccountByPreviousResponseID(poolCtx, &groupID, "resp_pool_off", "gpt-5.1", nil, false)
	require.NoError(t, err)
	require.Nil(t, selection)
	bound, err = store.GetResponseAccount(poolCtx, groupID, "resp_pool_off")
	require.NoError(t, err)
	require.Zero(t, bound, "关闭调度属永久不可用，删除绑定")
	require.False(t, svc.HasPreviousResponseBinding(poolCtx, &groupID, "resp_pool_off"))
}
