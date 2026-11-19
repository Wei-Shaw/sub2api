//go:build unit

package service

import (
	"context"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/pkg/ctxkey"
	"github.com/stretchr/testify/require"
)

// userAffinityFixture 构造走负载感知路径（load_batch 开启 + 并发服务存在）的
// GatewayService：anthropic 分组内两个可调度账号，账号 1 优先级更高。
type userAffinityFixture struct {
	svc   *GatewayService
	ctx   context.Context
	group int64
	cache *mockGatewayCacheForPlatform
}

func newUserAffinityFixture(t *testing.T, enabled bool, accounts []Account, bindings map[string]int64) *userAffinityFixture {
	t.Helper()

	groupID := int64(10)
	channelSvc := newTestChannelService(makeStandardRepo(Channel{
		ID:       1,
		Status:   StatusActive,
		GroupIDs: []int64{groupID},
	}, map[int64]string{groupID: PlatformAnthropic}))

	accountRepo := &mockAccountRepoForPlatform{accounts: accounts, accountsByID: map[int64]*Account{}}
	for i := range accountRepo.accounts {
		accountRepo.accountsByID[accountRepo.accounts[i].ID] = &accountRepo.accounts[i]
	}

	group := &Group{ID: groupID, Platform: PlatformAnthropic, Status: StatusActive, Hydrated: true}
	groupRepo := &mockGroupRepoForGateway{groups: map[int64]*Group{groupID: group}}

	cfg := testConfig()
	cfg.Gateway.Scheduling.LoadBatchEnabled = true
	cfg.Gateway.Scheduling.UserAffinityEnabled = enabled
	cache := &mockGatewayCacheForPlatform{sessionBindings: bindings}

	return &userAffinityFixture{
		svc: &GatewayService{
			accountRepo:        accountRepo,
			groupRepo:          groupRepo,
			channelService:     channelSvc,
			cache:              cache,
			cfg:                cfg,
			concurrencyService: NewConcurrencyService(&mockConcurrencyCache{}),
		},
		ctx:   context.WithValue(context.Background(), ctxkey.Group, group),
		group: groupID,
		cache: cache,
	}
}

func userAffinityTestAccounts() []Account {
	return []Account{
		{ID: 1, Platform: PlatformAnthropic, Priority: 1, Status: StatusActive, Schedulable: true, Concurrency: 5},
		{ID: 2, Platform: PlatformAnthropic, Priority: 2, Status: StatusActive, Schedulable: true, Concurrency: 5},
	}
}

// 默认关闭：不读取、不写入用户亲和绑定，调度行为保持不变。
func TestSelectAccountWithLoadAwareness_UserAffinityDisabledByDefault(t *testing.T) {
	t.Parallel()

	f := newUserAffinityFixture(t, false, userAffinityTestAccounts(), nil)
	for _, session := range []string{"s1", "s2"} {
		result, err := f.svc.SelectAccountWithLoadAwareness(f.ctx, &f.group, session, "", nil, "", 42)
		require.NoError(t, err)
		require.NotNil(t, result)
		require.Equal(t, int64(1), result.Account.ID, "关闭亲和时按原有优先级选择")
	}
	require.Equal(t, int64(1), f.cache.sessionBindings["s1"], "粘性会话绑定行为保持不变")
	require.NotContains(t, f.cache.sessionBindings, "uaff:user:42", "关闭亲和时不得写入用户绑定")
}

// 开启后：同一用户的新会话复用用户绑定的账号，即使其他账号优先级更高。
func TestSelectAccountWithLoadAwareness_UserAffinityRoutesUserToBoundAccount(t *testing.T) {
	t.Parallel()

	f := newUserAffinityFixture(t, true, userAffinityTestAccounts(), map[string]int64{
		"uaff:user:42": 2, // 用户 42 此前落在优先级较低的账号 2
	})
	result, err := f.svc.SelectAccountWithLoadAwareness(f.ctx, &f.group, "fresh-session", "", nil, "", 42)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Equal(t, int64(2), result.Account.ID, "新会话必须复用用户绑定的账号而非优先级最高者")
	require.Equal(t, int64(2), f.cache.sessionBindings["fresh-session"], "命中亲和后粘性会话应重绑到该账号")
}

// 开启后：绑定账号退出可调度集合时回退正常调度，并把用户绑定重绑到新选中的账号。
func TestSelectAccountWithLoadAwareness_UserAffinityFallsThroughAndRebinds(t *testing.T) {
	t.Parallel()

	accounts := userAffinityTestAccounts()
	accounts[1].Schedulable = false // 账号 2 不可调度 → 不在候选列表
	f := newUserAffinityFixture(t, true, accounts, map[string]int64{
		"uaff:user:42": 2,
	})
	result, err := f.svc.SelectAccountWithLoadAwareness(f.ctx, &f.group, "fresh-session", "", nil, "", 42)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Equal(t, int64(1), result.Account.ID, "亲和账号不可调度时必须回退到剩余可调度账号")
	require.Equal(t, int64(1), f.cache.sessionBindings["uaff:user:42"], "用户绑定必须重绑到新选中的账号")
}

func TestUserAffinityCacheKey(t *testing.T) {
	t.Parallel()

	require.Empty(t, userAffinityCacheKey(false, FormatMetadataUserID("device-abc", "", "session-1", "2.1.78"), 7), "关闭时一律不解析")
	require.Empty(t, userAffinityCacheKey(true, "", 0), "无稳定身份时不解析")
	require.Equal(t, "uaff:user:7", userAffinityCacheKey(true, "", 7))

	deviceUserID := FormatMetadataUserID("device-abc", "", "session-1", "2.1.78")
	require.Equal(t, "uaff:device:device-abc", userAffinityCacheKey(true, deviceUserID, 0))
	require.Equal(t, "uaff:user:7", userAffinityCacheKey(true, deviceUserID, 7), "sub2api 用户 ID 优先于客户端 DeviceID")
	require.Empty(t, userAffinityCacheKey(true, "not-a-user-id", 0), "无法解析的 metadata.user_id 不解析")
}
