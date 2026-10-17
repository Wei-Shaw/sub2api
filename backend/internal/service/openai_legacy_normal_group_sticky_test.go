package service

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/ctxkey"
	"github.com/stretchr/testify/require"
)

// 普通分组（ProfitControlEnabled=false）走 OpenAI 旧调度时的会话归属回归。
//
// 旧调度在普通分组里把会话归属写在**选号**那一刻（selectAccountForModelWithExclusions
// 与批量路径的 eager 绑定），与本跳成败无关：
//
//   - 归属属于「本次请求最后被选中的账号」，不是「真正成功的账号」；
//   - 因此一次只被选中、随后失败的账号可以把上一次确认成功的账号顶掉，
//     下一个同会话请求会重复落到刚刚失败的账号上。
//
// 全部使用合成账号与合成会话，不触及真实上游或生产配置。

const (
	legacyNormalGroupID      = int64(46)
	legacyNormalGroupSession = "legacy-normal-group-session"
	legacyNormalGroupModel   = "gpt-legacy-normal"
)

// legacyNormalGroupTestGroup 是一个没有开启利润控制的普通分组。
func legacyNormalGroupTestGroup() *Group {
	return &Group{
		ID:                   legacyNormalGroupID,
		Platform:             PlatformOpenAI,
		Status:               StatusActive,
		Hydrated:             true,
		RateMultiplier:       1,
		SubscriptionType:     SubscriptionTypeStandard,
		ProfitControlEnabled: false,
	}
}

// legacyNormalGroupAccounts 返回两个合成账号：1 的 Priority 更小，默认先被选中。
func legacyNormalGroupAccounts() []Account {
	accounts := make([]Account, 2)
	for i := range accounts {
		accounts[i] = Account{
			ID: int64(i + 1), Platform: PlatformOpenAI, Type: AccountTypeAPIKey,
			Status: StatusActive, Schedulable: true, Concurrency: 4,
			Priority: i + 1, GroupIDs: []int64{legacyNormalGroupID},
		}
	}
	return accounts
}

func newLegacyNormalGroupCache(boundAccountID int64) *openAILegacySuccessTestCache {
	cache := &openAILegacySuccessTestCache{bindings: map[string]GatewayStickySuccessBinding{}}
	if boundAccountID > 0 {
		cache.sessionBindings = map[string]int64{"openai:" + legacyNormalGroupSession: boundAccountID}
	}
	return cache
}

type legacyNormalGroupEnv struct {
	svc      *OpenAIGatewayService
	cache    *openAILegacySuccessTestCache
	accounts []Account
	baseCtx  context.Context
	groupID  int64
}

// newLegacyNormalGroupEnv 构造普通分组的合成旧调度环境（高级调度器关闭）。
func newLegacyNormalGroupEnv(t *testing.T, cache *openAILegacySuccessTestCache, accounts []Account, loadBatch bool) *legacyNormalGroupEnv {
	t.Helper()
	resetOpenAIAdvancedSchedulerSettingCacheForTest()
	t.Cleanup(resetOpenAIAdvancedSchedulerSettingCacheForTest)
	cfg := &config.Config{}
	cfg.Gateway.Scheduling.LoadBatchEnabled = loadBatch
	group := legacyNormalGroupTestGroup()
	svc := &OpenAIGatewayService{
		cache:              cache,
		cfg:                cfg,
		accountRepo:        schedulerTestOpenAIAccountRepo{accounts: accounts},
		rateLimitService:   newOpenAIAdvancedSchedulerRateLimitService("false"),
		concurrencyService: NewConcurrencyService(schedulerTestConcurrencyCache{}),
	}
	return &legacyNormalGroupEnv{
		svc:      svc,
		cache:    cache,
		accounts: accounts,
		baseCtx:  context.WithValue(context.Background(), ctxkey.Group, group),
		groupID:  group.ID,
	}
}

// begin 复刻 handler 在进入 failover 循环前的装配顺序。
func (e *legacyNormalGroupEnv) begin() context.Context {
	ctx, _ := e.svc.WithOpenAIRequestPricingContext(e.baseCtx, &e.groupID)
	return e.svc.BeginOpenAILegacyStickySuccess(ctx, &e.groupID, legacyNormalGroupSession, legacyNormalGroupModel)
}

func (e *legacyNormalGroupEnv) beginModel(model string) context.Context {
	ctx, _ := e.svc.WithOpenAIRequestPricingContext(e.baseCtx, &e.groupID)
	return e.svc.BeginOpenAILegacyStickySuccess(ctx, &e.groupID, legacyNormalGroupSession, model)
}

func (e *legacyNormalGroupEnv) selectAccount(t *testing.T, ctx context.Context, excluded map[int64]struct{}) int64 {
	t.Helper()
	selection, _, err := e.svc.SelectAccountWithScheduler(ctx, &e.groupID, "", legacyNormalGroupSession,
		legacyNormalGroupModel, excluded, OpenAIUpstreamTransportAny, false)
	require.NoError(t, err)
	require.NotNil(t, selection)
	require.NotNil(t, selection.Account)
	if selection.ReleaseFunc != nil {
		selection.ReleaseFunc()
	}
	return selection.Account.ID
}

func (e *legacyNormalGroupEnv) commitSuccess(ctx context.Context, accountID int64) {
	e.svc.CommitOpenAILegacyStickySuccess(ctx, &Account{ID: accountID}, &OpenAIForwardResult{})
}

// TestOpenAILegacyNormalGroupFailedAttemptKeepsSuccessfulSession 钉住核心契约：
// 普通分组里会话归属必须跟着**确认成功**的账号走，只被选中、随后失败的账号
// 不得把它夺走。
func TestOpenAILegacyNormalGroupFailedAttemptKeepsSuccessfulSession(t *testing.T) {
	for _, loadBatch := range []bool{false, true} {
		t.Run(fmt.Sprintf("load_batch_%t", loadBatch), func(t *testing.T) {
			accounts := legacyNormalGroupAccounts()
			env := newLegacyNormalGroupEnv(t, newLegacyNormalGroupCache(1), accounts, loadBatch)

			// 请求 1：A(1) 直接成功，成为这条会话确认成功过的账号。
			first := env.begin()
			require.EqualValues(t, 1, env.selectAccount(t, first, nil))
			env.commitSuccess(first, 1)

			// 请求 2：A 被管理员停用，只能落到 B(2)；B 上游失败，整条请求失败，
			// 因此不提交成功偏好。
			accounts[0].Schedulable = false
			second := env.begin()
			require.EqualValues(t, 2, env.selectAccount(t, second, nil), "A 不可调度时只剩 B")

			// 请求 3：A 恢复可调度，会话归属仍应属于上一次真正成功的 A。
			accounts[0].Schedulable = true
			third := env.begin()
			require.EqualValues(t, 1, env.selectAccount(t, third, nil),
				"只被选中、随后失败的账号不得夺走会话归属")
		})
	}
}

// TestOpenAILegacyNormalGroupFailoverSuccessOwnsSession 覆盖 A 失败、failover 到
// B 成功的主链路：下一个同会话请求必须直接从 B 开始。
func TestOpenAILegacyNormalGroupFailoverSuccessOwnsSession(t *testing.T) {
	for _, loadBatch := range []bool{false, true} {
		t.Run(fmt.Sprintf("load_batch_%t", loadBatch), func(t *testing.T) {
			accounts := legacyNormalGroupAccounts()
			env := newLegacyNormalGroupEnv(t, newLegacyNormalGroupCache(1), accounts, loadBatch)

			first := env.begin()
			require.EqualValues(t, 1, env.selectAccount(t, first, nil), "首跳是会话原本绑定的 A")
			require.EqualValues(t, 2, env.selectAccount(t, first, map[int64]struct{}{1: {}}), "A 失败后 failover 到 B")
			env.commitSuccess(first, 2)

			second := env.begin()
			require.EqualValues(t, 2, env.selectAccount(t, second, nil),
				"下一个同会话请求首跳必须落在 failover 成功的 B")
		})
	}
}

// TestOpenAILegacyNormalGroupSuccessNeverBeatsEligibility 钉住「成功偏好只是软
// 偏好」：偏好账号当前不合格时必须让位，不得被强行粘住，也不得排队等待它。
func TestOpenAILegacyNormalGroupSuccessNeverBeatsEligibility(t *testing.T) {
	for _, loadBatch := range []bool{false, true} {
		t.Run(fmt.Sprintf("load_batch_%t", loadBatch), func(t *testing.T) {
			accounts := legacyNormalGroupAccounts()
			env := newLegacyNormalGroupEnv(t, newLegacyNormalGroupCache(1), accounts, loadBatch)

			first := env.begin()
			require.EqualValues(t, 2, env.selectAccount(t, first, map[int64]struct{}{1: {}}))
			env.commitSuccess(first, 2)
			require.EqualValues(t, 2, env.selectAccount(t, env.begin(), nil), "偏好账号合格时复用")

			accounts[1].Schedulable = false
			require.EqualValues(t, 1, env.selectAccount(t, env.begin(), nil),
				"人工停用的偏好账号必须让位给仍然合格的候选")
		})
	}
}

// TestOpenAILegacyNormalGroupLateCompletionKeepsNewerSuccess 钉住 CAS 围栏：
// 同会话两个在途请求，较早开始的那个晚完成时不得覆盖已提交的新成功。
func TestOpenAILegacyNormalGroupLateCompletionKeepsNewerSuccess(t *testing.T) {
	accounts := legacyNormalGroupAccounts()
	env := newLegacyNormalGroupEnv(t, newLegacyNormalGroupCache(1), accounts, false)

	early := env.begin()
	late := env.begin()
	env.commitSuccess(late, 2)
	env.commitSuccess(early, 1)
	require.Equal(t, 1, env.cache.writes, "晚完成的旧请求必须输掉 CAS")

	require.EqualValues(t, 2, env.selectAccount(t, env.begin(), nil),
		"晚完成的旧请求不得覆盖另一个请求已提交的新成功偏好")
}

// TestOpenAILegacyNormalGroupFailureAndCancelNeverCommit 钉住提交判据：普通分组
// 同样只在确认成功终态写偏好，失败 / 客户端断开 / 取消一律不写、不续期。
func TestOpenAILegacyNormalGroupFailureAndCancelNeverCommit(t *testing.T) {
	env := newLegacyNormalGroupEnv(t, newLegacyNormalGroupCache(1), legacyNormalGroupAccounts(), false)
	ctx := env.begin()

	for _, result := range []*OpenAIForwardResult{
		nil,
		{ClientDisconnect: true},
		{OpenAIWSMode: true, UpstreamTerminalEvent: "response.failed"},
	} {
		env.svc.CommitOpenAILegacyStickySuccess(ctx, &Account{ID: 2}, result)
	}
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	env.svc.CommitOpenAILegacyStickySuccess(canceled, &Account{ID: 2}, &OpenAIForwardResult{})
	require.Zero(t, env.cache.writes, "没有确认成功终态就不得写成功偏好")

	env.commitSuccess(ctx, 2)
	require.Equal(t, 1, env.cache.writes)
}

// TestOpenAILegacyNormalGroupPreferenceIsolatedByModelAndGroup 钉住隔离维度：
// 成功偏好按 有效分组 / 会话 / 映射前客户端模型 三维隔离。
func TestOpenAILegacyNormalGroupPreferenceIsolatedByModelAndGroup(t *testing.T) {
	env := newLegacyNormalGroupEnv(t, newLegacyNormalGroupCache(1), legacyNormalGroupAccounts(), false)

	first := env.begin()
	env.commitSuccess(first, 2)

	alias := env.beginModel("alias-b")
	id, err := env.svc.getStickySessionAccountID(alias, &env.groupID, legacyNormalGroupSession)
	require.NoError(t, err)
	require.EqualValues(t, 1, id, "另一个客户端模型保留自己的候选，不继承别名的偏好")

	otherGroup := int64(legacyNormalGroupID + 1)
	_, managed := openAILegacyStickySuccessCandidate(env.begin(), &otherGroup, legacyNormalGroupSession)
	require.False(t, managed, "另一个分组不得继承本分组的偏好")
}

// TestOpenAILegacyNormalGroupKeepsLegacyBindingSemantics 钉住边界：普通分组只
// 接管**候选来源**，旧 sticky 键的写入 / 续期 / 清理仍是官方 eager 语义，
// WS、count_tokens、previous_response 归属链与 Guardian 等仍读旧键的路径不受影响。
func TestOpenAILegacyNormalGroupKeepsLegacyBindingSemantics(t *testing.T) {
	env := newLegacyNormalGroupEnv(t, newLegacyNormalGroupCache(1), legacyNormalGroupAccounts(), false)
	ctx := env.begin()
	key := "openai:" + legacyNormalGroupSession

	require.NoError(t, env.svc.setStickySessionAccountID(ctx, &env.groupID, legacyNormalGroupSession, 2, time.Hour))
	require.EqualValues(t, 2, env.cache.sessionBindings[key], "普通分组的旧键写入必须原样生效")

	require.NoError(t, env.svc.refreshStickySessionTTL(ctx, &env.groupID, legacyNormalGroupSession, time.Hour))

	require.NoError(t, env.svc.deleteStickySessionAccountID(ctx, &env.groupID, legacyNormalGroupSession))
	_, exists := env.cache.sessionBindings[key]
	require.False(t, exists, "普通分组的旧键清理必须原样生效")
	require.Zero(t, env.cache.writes, "旧键操作不得写成功偏好")
}
