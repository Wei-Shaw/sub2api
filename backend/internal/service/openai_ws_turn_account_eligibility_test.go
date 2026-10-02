package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	coderws "github.com/coder/websocket"
	"github.com/stretchr/testify/require"
)

func newWSTurnEligibilityAccount() *Account {
	return &Account{
		ID:          9001,
		Name:        "ws-turn-eligibility",
		Platform:    PlatformOpenAI,
		Type:        AccountTypeAPIKey,
		Status:      StatusActive,
		Schedulable: true,
		Concurrency: 1,
		Credentials: map[string]any{"api_key": "sk-test"},
	}
}

func newWSTurnEligibilityService(cache GatewayCache) *OpenAIGatewayService {
	return &OpenAIGatewayService{
		cfg:   &config.Config{},
		cache: cache,
	}
}

func wsTurnModelRateLimits(model string) map[string]any {
	return map[string]any{
		model: map[string]any{
			"rate_limit_reset_at": time.Now().Add(time.Hour).Format(time.RFC3339),
		},
	}
}

func requireWSTurnCloseError(t *testing.T, err error, wantReason string) {
	t.Helper()
	require.Error(t, err)
	var closeErr *OpenAIWSClientCloseError
	require.True(t, errors.As(err, &closeErr), "必须是客户端 close error")
	require.Equal(t, coderws.StatusTryAgainLater, closeErr.StatusCode(), "close code 必须是客户端会重试的 TryAgainLater")
	require.Contains(t, closeErr.Reason(), "no longer schedulable")
	require.Contains(t, closeErr.Reason(), wantReason)
	require.Contains(t, closeErr.Reason(), "please reconnect")
}

// 账号被临时下线（temp_unschedulable_until 未到期）后，下一轮必须断开客户端 WS。
func TestEnforceOpenAIWSTurnAccountEligibility_TempUnschedulable(t *testing.T) {
	svc := newWSTurnEligibilityService(&stubGatewayCache{})
	account := newWSTurnEligibilityAccount()

	reason, err := svc.EnforceOpenAIWSTurnAccountEligibility(context.Background(), account, nil, "", "gpt-5.1")
	require.Empty(t, reason)
	require.NoError(t, err)

	until := time.Now().Add(time.Hour)
	account.TempUnschedulableUntil = &until

	reason, err = svc.EnforceOpenAIWSTurnAccountEligibility(context.Background(), account, nil, "", "gpt-5.1")
	require.Equal(t, OpenAIWSTurnAccountIneligibleNotSchedulable, reason)
	requireWSTurnCloseError(t, err, OpenAIWSTurnAccountIneligibleNotSchedulable)
}

// 按模型限流：被限流的模型断开，同账号上未被限流的模型不受影响。
func TestEnforceOpenAIWSTurnAccountEligibility_ModelRateLimitedIsPerModel(t *testing.T) {
	svc := newWSTurnEligibilityService(&stubGatewayCache{})
	account := newWSTurnEligibilityAccount()
	account.Extra = map[string]any{modelRateLimitsKey: wsTurnModelRateLimits("gpt-5.1")}

	reason, err := svc.EnforceOpenAIWSTurnAccountEligibility(context.Background(), account, nil, "", "gpt-5.1")
	require.Equal(t, OpenAIWSTurnAccountIneligibleModelRateLimited, reason)
	requireWSTurnCloseError(t, err, OpenAIWSTurnAccountIneligibleModelRateLimited)

	reason, err = svc.EnforceOpenAIWSTurnAccountEligibility(context.Background(), account, nil, "", "gpt-5.1-codex-mini")
	require.Empty(t, reason, "限流的是别的模型时不应断开")
	require.NoError(t, err)
}

// 管理员停调度（schedulable=false）与 status 非 active 都要断开。
func TestEnforceOpenAIWSTurnAccountEligibility_AdminDisabled(t *testing.T) {
	t.Run("schedulable=false", func(t *testing.T) {
		svc := newWSTurnEligibilityService(&stubGatewayCache{})
		account := newWSTurnEligibilityAccount()
		account.Schedulable = false

		reason, err := svc.EnforceOpenAIWSTurnAccountEligibility(context.Background(), account, nil, "", "gpt-5.1")
		require.Equal(t, OpenAIWSTurnAccountIneligibleNotSchedulable, reason)
		requireWSTurnCloseError(t, err, OpenAIWSTurnAccountIneligibleNotSchedulable)
	})

	t.Run("status=error", func(t *testing.T) {
		svc := newWSTurnEligibilityService(&stubGatewayCache{})
		account := newWSTurnEligibilityAccount()
		account.Status = StatusError

		reason, err := svc.EnforceOpenAIWSTurnAccountEligibility(context.Background(), account, nil, "", "gpt-5.1")
		require.Equal(t, OpenAIWSTurnAccountIneligibleNotSchedulable, reason)
		requireWSTurnCloseError(t, err, OpenAIWSTurnAccountIneligibleNotSchedulable)
	})
}

// 健康账号不断开，粘性绑定也不动。
func TestEnforceOpenAIWSTurnAccountEligibility_HealthyAccountKeepsConnection(t *testing.T) {
	cache := &stubGatewayCache{}
	svc := newWSTurnEligibilityService(cache)
	account := newWSTurnEligibilityAccount()
	require.NoError(t, svc.BindStickySession(context.Background(), nil, "sess-healthy", account.ID))

	for turn := 2; turn <= 5; turn++ {
		reason, err := svc.EnforceOpenAIWSTurnAccountEligibility(context.Background(), account, nil, "sess-healthy", "gpt-5.1")
		require.Empty(t, reason)
		require.NoError(t, err)
	}
	require.Equal(t, account.ID, cache.sessionBindings["openai:sess-healthy"], "健康账号的粘性绑定不能被动")
	require.Zero(t, cache.deletedSessions["openai:sess-healthy"])
}

// 断开时必须解除粘性绑定，否则重连会又粘回同一个不可用的号。
func TestEnforceOpenAIWSTurnAccountEligibility_ReleasesStickyBinding(t *testing.T) {
	cache := &stubGatewayCache{}
	svc := newWSTurnEligibilityService(cache)
	account := newWSTurnEligibilityAccount()
	require.NoError(t, svc.BindStickySession(context.Background(), nil, "sess-blocked", account.ID))
	require.Equal(t, account.ID, cache.sessionBindings["openai:sess-blocked"])

	account.Schedulable = false
	reason, err := svc.EnforceOpenAIWSTurnAccountEligibility(context.Background(), account, nil, "sess-blocked", "gpt-5.1")
	require.Equal(t, OpenAIWSTurnAccountIneligibleNotSchedulable, reason)
	require.Error(t, err)

	_, exists := cache.sessionBindings["openai:sess-blocked"]
	require.False(t, exists, "断开前必须解除粘性绑定")
	require.Equal(t, 1, cache.deletedSessions["openai:sess-blocked"])
}

// 粘性绑定已指向别的账号（别处已换号）时不要误删。
func TestEnforceOpenAIWSTurnAccountEligibility_KeepsStickyBindingOfAnotherAccount(t *testing.T) {
	cache := &stubGatewayCache{}
	svc := newWSTurnEligibilityService(cache)
	account := newWSTurnEligibilityAccount()
	require.NoError(t, svc.BindStickySession(context.Background(), nil, "sess-moved", account.ID+1))

	account.Schedulable = false
	_, err := svc.EnforceOpenAIWSTurnAccountEligibility(context.Background(), account, nil, "sess-moved", "gpt-5.1")
	require.Error(t, err)
	require.Equal(t, account.ID+1, cache.sessionBindings["openai:sess-moved"])
	require.Zero(t, cache.deletedSessions["openai:sess-moved"])
}

// 资格判定必须读最新账号状态，而不是握手时的连接内快照。
func TestEnforceOpenAIWSTurnAccountEligibility_UsesFreshAccountState(t *testing.T) {
	staleSnapshot := newWSTurnEligibilityAccount()
	fresh := newWSTurnEligibilityAccount()
	until := time.Now().Add(time.Hour)
	fresh.TempUnschedulableUntil = &until

	svc := newWSTurnEligibilityService(&stubGatewayCache{})
	svc.accountRepo = &stubOpenAIAccountRepo{accounts: []Account{*fresh}}

	reason, err := svc.EnforceOpenAIWSTurnAccountEligibility(context.Background(), staleSnapshot, nil, "", "gpt-5.1")
	require.Equal(t, OpenAIWSTurnAccountIneligibleNotSchedulable, reason, "连接内的旧快照仍可调度，必须以最新状态为准")
	require.Error(t, err)
}

// getSchedulableAccount 返回 (nil, nil)（账号被调度门控拦下或查无此号）时同样断开。
func TestEnforceOpenAIWSTurnAccountEligibility_NilSchedulableAccountClosesConnection(t *testing.T) {
	svc := newWSTurnEligibilityService(&stubGatewayCache{})
	svc.accountRepo = &wsTurnEligibilityNilAccountRepo{}

	reason, err := svc.EnforceOpenAIWSTurnAccountEligibility(context.Background(), newWSTurnEligibilityAccount(), nil, "", "gpt-5.1")
	require.Equal(t, OpenAIWSTurnAccountIneligibleNotSchedulable, reason)
	requireWSTurnCloseError(t, err, OpenAIWSTurnAccountIneligibleNotSchedulable)
}

// 已删除账号的真实错误形态是 ErrAccountNotFound（Redis 快照缺失会回源仓储，
// 仓储再返回 not found），不能和缓存抖动一起 fail-open。
func TestEnforceOpenAIWSTurnAccountEligibility_AccountNotFoundErrorClosesConnection(t *testing.T) {
	svc := newWSTurnEligibilityService(&stubGatewayCache{})
	svc.accountRepo = &wsTurnEligibilityNotFoundAccountRepo{}

	reason, err := svc.EnforceOpenAIWSTurnAccountEligibility(context.Background(), newWSTurnEligibilityAccount(), nil, "", "gpt-5.1")
	require.Equal(t, OpenAIWSTurnAccountIneligibleAccountMissing, reason)
	requireWSTurnCloseError(t, err, OpenAIWSTurnAccountIneligibleAccountMissing)
}

// 账号被删除时也要解除粘性绑定，否则重连还会粘回这个不存在的号。
func TestEnforceOpenAIWSTurnAccountEligibility_AccountNotFoundReleasesStickyBinding(t *testing.T) {
	cache := &stubGatewayCache{}
	svc := newWSTurnEligibilityService(cache)
	svc.accountRepo = &wsTurnEligibilityNotFoundAccountRepo{}
	account := newWSTurnEligibilityAccount()
	require.NoError(t, svc.BindStickySession(context.Background(), nil, "sess-deleted", account.ID))

	_, err := svc.EnforceOpenAIWSTurnAccountEligibility(context.Background(), account, nil, "sess-deleted", "gpt-5.1")
	require.Error(t, err)
	_, exists := cache.sessionBindings["openai:sess-deleted"]
	require.False(t, exists)
}

// 账号级 model_mapping 造成的模型名差：限流记在映射后的模型上，客户端发的是映射前
// 的模型，资格检查必须对得上映射后的 key。
func TestEnforceOpenAIWSTurnAccountEligibility_AccountModelMappingIsBlocked(t *testing.T) {
	svc := newWSTurnEligibilityService(&stubGatewayCache{})
	account := newWSTurnEligibilityAccount()
	account.Credentials = map[string]any{
		"api_key":       "sk-test",
		"model_mapping": map[string]any{"gpt-5.1": "gpt-5.1-mapped"},
	}
	account.Extra = map[string]any{modelRateLimitsKey: wsTurnModelRateLimits("gpt-5.1-mapped")}
	require.Equal(t, "gpt-5.1-mapped", account.GetMappedModel("gpt-5.1"), "前置条件：账号映射生效")

	reason, err := svc.EnforceOpenAIWSTurnAccountEligibility(context.Background(), account, nil, "", "gpt-5.1")
	require.Equal(t, OpenAIWSTurnAccountIneligibleModelRateLimited, reason)
	requireWSTurnCloseError(t, err, OpenAIWSTurnAccountIneligibleModelRateLimited)
}

// 上游模型归一造成的口径差（Codex 协议账号把 gpt-5.1 归一成 gpt-5.4 再发上游）：
// 限流记在 gpt-5.4 上，客户端本轮发 gpt-5.1，只按客户端模型查会一直放行。
func TestEnforceOpenAIWSTurnAccountEligibility_UpstreamModelNormalizationIsBlocked(t *testing.T) {
	svc := newWSTurnEligibilityService(&stubGatewayCache{})
	account := newWSTurnEligibilityAccount()
	account.Type = AccountTypeOAuth
	account.Credentials = map[string]any{"access_token": "at-test"}
	account.Extra = map[string]any{modelRateLimitsKey: wsTurnModelRateLimits("gpt-5.4")}
	require.Equal(t, "gpt-5.1", account.GetMappedModel("gpt-5.1"), "前置条件：不靠账号 model_mapping")
	require.Equal(t, "gpt-5.4", canonicalOpenAIAccountSchedulingModel(account, "gpt-5.1"),
		"前置条件：实际发往上游的是归一后的模型")

	reason, err := svc.EnforceOpenAIWSTurnAccountEligibility(context.Background(), account, nil, "", "gpt-5.1")
	require.Equal(t, OpenAIWSTurnAccountIneligibleModelRateLimited, reason)
	requireWSTurnCloseError(t, err, OpenAIWSTurnAccountIneligibleModelRateLimited)

	// 同账号上没被限流的模型不受影响。
	reason, err = svc.EnforceOpenAIWSTurnAccountEligibility(context.Background(), account, nil, "", "gpt-5.1-codex")
	require.Empty(t, reason)
	require.NoError(t, err)
}

// 快照读失败要 fail-open：缓存抖动不能批量踢掉正在跑的长连接。
func TestEnforceOpenAIWSTurnAccountEligibility_SnapshotReadFailureFailsOpen(t *testing.T) {
	svc := newWSTurnEligibilityService(&stubGatewayCache{})
	svc.accountRepo = &wsTurnEligibilityFailingAccountRepo{}

	reason, err := svc.EnforceOpenAIWSTurnAccountEligibility(context.Background(), newWSTurnEligibilityAccount(), nil, "", "gpt-5.1")
	require.Empty(t, reason)
	require.NoError(t, err)
}

type wsTurnEligibilityNilAccountRepo struct {
	AccountRepository
}

func (r *wsTurnEligibilityNilAccountRepo) GetByID(context.Context, int64) (*Account, error) {
	return nil, nil
}

type wsTurnEligibilityNotFoundAccountRepo struct {
	AccountRepository
}

// 仓储真实返回的是带 cause 的克隆（translatePersistenceError → notFound.WithCause），
// 这里照抄这个形态，顺带钉死 errors.Is 能穿过整条错误链。
func (r *wsTurnEligibilityNotFoundAccountRepo) GetByID(context.Context, int64) (*Account, error) {
	return nil, ErrAccountNotFound.WithCause(errors.New("ent: account not found"))
}

type wsTurnEligibilityFailingAccountRepo struct {
	AccountRepository
}

func (r *wsTurnEligibilityFailingAccountRepo) GetByID(context.Context, int64) (*Account, error) {
	return nil, errors.New("snapshot unavailable")
}

// newWSTurnWildcardMappingAccount 返回一个账号级 model_mapping 为 {"*":"gpt-5.1"} 的
// Codex 协议账号：通配映射会把任何模型（含最终上游模型 gpt-5.4 自己）映回 gpt-5.1，
// 是「二次映射」漏检/误拦的最小复现条件。
func newWSTurnWildcardMappingAccount() *Account {
	account := newWSTurnEligibilityAccount()
	account.Type = AccountTypeOAuth
	account.Credentials = map[string]any{
		"access_token":  "at-test",
		"model_mapping": map[string]any{"*": "gpt-5.1"},
	}
	return account
}

// 通配映射 + 最终键限流：账号 model_mapping 为 {"*":"gpt-5.1"}，限流记在实际发往
// 上游的模型（gpt-5.4）上，客户端本轮发 gpt-5.1。
//
// 资格复核若把最终键再过一次账号映射，gpt-5.4 会被通配规则映回 gpt-5.1，查的是
// model_rate_limits[gpt-5.1]，限流永远查不到，不可用的号照常放行。
func TestEnforceOpenAIWSTurnAccountEligibility_WildcardMappingDoesNotRemapFinalKey(t *testing.T) {
	svc := newWSTurnEligibilityService(&stubGatewayCache{})
	account := newWSTurnWildcardMappingAccount()
	account.Extra = map[string]any{modelRateLimitsKey: wsTurnModelRateLimits("gpt-5.4")}
	require.Equal(t, "gpt-5.1", account.GetMappedModel("gpt-5.4"),
		"前置条件：通配映射会把最终键 gpt-5.4 再映回 gpt-5.1，二次映射即漏检")
	require.Equal(t, "gpt-5.4", canonicalOpenAIAccountSchedulingModel(account, "gpt-5.1"),
		"前置条件：实际发往上游的是 gpt-5.4")

	reason, err := svc.EnforceOpenAIWSTurnAccountEligibility(context.Background(), account, nil, "", "gpt-5.1")
	require.Equal(t, OpenAIWSTurnAccountIneligibleModelRateLimited, reason)
	requireWSTurnCloseError(t, err, OpenAIWSTurnAccountIneligibleModelRateLimited)
}

// 同一套通配映射下，限流的是别的模型（gpt-5.2）时不能误拦：候选键只有
// gpt-5.1 / gpt-5.4，都查不到这条限制。
func TestEnforceOpenAIWSTurnAccountEligibility_WildcardMappingDoesNotOverBlock(t *testing.T) {
	svc := newWSTurnEligibilityService(&stubGatewayCache{})
	account := newWSTurnWildcardMappingAccount()
	account.Extra = map[string]any{modelRateLimitsKey: wsTurnModelRateLimits("gpt-5.2")}

	reason, err := svc.EnforceOpenAIWSTurnAccountEligibility(context.Background(), account, nil, "", "gpt-5.1")
	require.Empty(t, reason, "限流的是别的模型，不能误拦")
	require.NoError(t, err)
}

// 模型级 runtime blocker 记的同样是最终上游模型名，判定时也不能二次映射。
func TestEnforceOpenAIWSTurnAccountEligibility_WildcardMappingDoesNotRemapRuntimeBlockerKey(t *testing.T) {
	svc := newWSTurnEligibilityService(&stubGatewayCache{})
	account := newWSTurnWildcardMappingAccount()

	state := svc.getOpenAIAccountModelTransientState()
	require.NotNil(t, state)
	now := time.Now()
	for i := 0; i < 3; i++ {
		state.recordFailure(account.ID, openAIAccountModelTransientModel("gpt-5.4"), now)
	}
	require.True(t, state.isBlocked(account.ID, openAIAccountModelTransientModel("gpt-5.4"), now),
		"前置条件：blocker 记在最终上游模型名上")

	reason, err := svc.EnforceOpenAIWSTurnAccountEligibility(context.Background(), account, nil, "", "gpt-5.1")
	require.Equal(t, OpenAIWSTurnAccountIneligibleRuntimeBlocked, reason)
	requireWSTurnCloseError(t, err, OpenAIWSTurnAccountIneligibleRuntimeBlocked)
}

// newWSTurnPassthroughAccount 返回一个 openai_passthrough=true 的 Codex 协议账号，
// 且不配任何账号级 model_mapping。
func newWSTurnPassthroughAccount() *Account {
	account := newWSTurnEligibilityAccount()
	account.Type = AccountTypeOAuth
	account.Credentials = map[string]any{"access_token": "at-test"}
	account.Extra = map[string]any{"openai_passthrough": true}
	return account
}

// openai_passthrough 开关不得影响 WS 的最终上游模型推导。
//
// 账号 openai_passthrough=true 时，HTTP Forward 的
// resolveOpenAIAccountUpstreamModelForRequest 原样返回客户端模型，于是
// canonicalOpenAIAccountSchedulingModel(account, "gpt-5.1") == "gpt-5.1"；但 WS 的
// ctx_pool / http_bridge 入口无视该开关，照样把 gpt-5.1 归一成 gpt-5.4 发上游，
// 限流也记在 gpt-5.4 上。资格复核若用 canonical 推键，两个候选都是 gpt-5.1，限流
// 永远查不到。
func TestEnforceOpenAIWSTurnAccountEligibility_PassthroughAccountUsesWSUpstreamModel(t *testing.T) {
	svc := newWSTurnEligibilityService(&stubGatewayCache{})
	account := newWSTurnPassthroughAccount()
	account.Extra[modelRateLimitsKey] = wsTurnModelRateLimits("gpt-5.4")

	require.True(t, account.IsOpenAIPassthroughEnabled(), "前置条件：账号开了 openai_passthrough")
	require.True(t, account.UsesOpenAICodexProtocol(), "前置条件：Codex 协议账号才会做模型归一")
	require.Equal(t, "gpt-5.1", account.GetMappedModel("gpt-5.1"), "前置条件：不靠账号 model_mapping")
	require.Equal(t, "gpt-5.1", canonicalOpenAIAccountSchedulingModel(account, "gpt-5.1"),
		"前置条件：passthrough 开关让 Forward 口径停在客户端模型上")
	require.Equal(t, "gpt-5.4", openAIWSUpstreamModelForAccount(account, "gpt-5.1"),
		"前置条件：WS 入口无视 passthrough 开关，实际发上游的是 gpt-5.4")

	reason, err := svc.EnforceOpenAIWSTurnAccountEligibility(context.Background(), account, nil, "", "gpt-5.1")
	require.Equal(t, OpenAIWSTurnAccountIneligibleModelRateLimited, reason)
	requireWSTurnCloseError(t, err, OpenAIWSTurnAccountIneligibleModelRateLimited)
}

// 对照组：同样的 passthrough 账号，限流的是别的模型（gpt-5.2）时不能误拦。
func TestEnforceOpenAIWSTurnAccountEligibility_PassthroughAccountDoesNotOverBlock(t *testing.T) {
	svc := newWSTurnEligibilityService(&stubGatewayCache{})
	account := newWSTurnPassthroughAccount()
	account.Extra[modelRateLimitsKey] = wsTurnModelRateLimits("gpt-5.2")

	reason, err := svc.EnforceOpenAIWSTurnAccountEligibility(context.Background(), account, nil, "", "gpt-5.1")
	require.Empty(t, reason, "限流的是别的模型，不能误拦")
	require.NoError(t, err)
}

// runtime blocker 的键与写入侧（ctx_pool 拨号失败记
// canonicalOpenAIAccountSchedulingModel(account, 客户端模型)）对齐：passthrough 账号
// 上两侧都停在客户端模型 gpt-5.1，判定必须自洽。
func TestEnforceOpenAIWSTurnAccountEligibility_PassthroughRuntimeBlockerMatchesWriteKey(t *testing.T) {
	svc := newWSTurnEligibilityService(&stubGatewayCache{})
	account := newWSTurnPassthroughAccount()

	writeKey := canonicalOpenAIAccountSchedulingModel(account, "gpt-5.1")
	require.Equal(t, "gpt-5.1", writeKey, "前置条件：passthrough 账号的写入键是客户端模型本身")

	state := svc.getOpenAIAccountModelTransientState()
	require.NotNil(t, state)
	now := time.Now()
	for i := 0; i < 3; i++ {
		state.recordFailure(account.ID, openAIAccountModelTransientModel(writeKey), now)
	}
	require.True(t, state.isBlocked(account.ID, openAIAccountModelTransientModel(writeKey), now),
		"前置条件：blocker 记在写入侧的键上")

	reason, err := svc.EnforceOpenAIWSTurnAccountEligibility(context.Background(), account, nil, "", "gpt-5.1")
	require.Equal(t, OpenAIWSTurnAccountIneligibleRuntimeBlocked, reason)
	requireWSTurnCloseError(t, err, OpenAIWSTurnAccountIneligibleRuntimeBlocked)
}

// http_bridge / ctx_pool 的 turn 内上游错误按「实际发往上游的模型」记 runtime
// blocker（gpt-5.4），与拨号失败那条路的键不同。passthrough 账号上这两个键会
// 分叉，检查侧取并集才不会漏。
func TestEnforceOpenAIWSTurnAccountEligibility_PassthroughRuntimeBlockerOnUpstreamKey(t *testing.T) {
	svc := newWSTurnEligibilityService(&stubGatewayCache{})
	account := newWSTurnPassthroughAccount()

	writeKey := openAIWSUpstreamModelForAccount(account, "gpt-5.1")
	require.Equal(t, "gpt-5.4", writeKey, "前置条件：WS turn 内失败记的是实际上游模型")

	state := svc.getOpenAIAccountModelTransientState()
	require.NotNil(t, state)
	now := time.Now()
	for i := 0; i < 3; i++ {
		state.recordFailure(account.ID, openAIAccountModelTransientModel(writeKey), now)
	}
	require.True(t, state.isBlocked(account.ID, openAIAccountModelTransientModel(writeKey), now),
		"前置条件：blocker 记在 gpt-5.4 上")

	reason, err := svc.EnforceOpenAIWSTurnAccountEligibility(context.Background(), account, nil, "", "gpt-5.1")
	require.Equal(t, OpenAIWSTurnAccountIneligibleRuntimeBlocked, reason)
	requireWSTurnCloseError(t, err, OpenAIWSTurnAccountIneligibleRuntimeBlocked)
}
