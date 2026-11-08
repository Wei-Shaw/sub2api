//go:build unit

package service

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

// nameFilteringAPIKeyRepoStub 是一个**按名字精确过滤**的 APIKey 仓储桩。
//
// 这点很关键：项目里已有的 monitorUsageAPIKeyRepoStub 会无视查询串、原样返回
// 所有既有 Key，用它来测「改名后能否找回旧 Key」会**全部假通过**——因为不管
// 查的是新名还是旧名，它都返回同一条。本桩刻意模拟真实仓储的「按名搜索」语义。
type nameFilteringAPIKeyRepoStub struct {
	APIKeyRepository
	keys    []APIKey
	created []APIKey
	nextID  int64
}

func (s *nameFilteringAPIKeyRepoStub) SearchAPIKeys(
	_ context.Context,
	userID int64,
	query string,
	_ int,
) ([]APIKey, error) {
	out := make([]APIKey, 0, 1)
	for _, k := range s.keys {
		if k.UserID == userID && k.Name == query {
			out = append(out, k)
		}
	}
	return out, nil
}

func (s *nameFilteringAPIKeyRepoStub) Create(_ context.Context, key *APIKey) error {
	if s.nextID == 0 {
		s.nextID = 500
	}
	key.ID = s.nextID
	s.nextID++
	s.created = append(s.created, *key)
	s.keys = append(s.keys, *key)
	return nil
}

func newAttributionStubs(existing ...APIKey) (*nameFilteringAPIKeyRepoStub, *intelligenceCheckUsageUserRepoStub) {
	return &nameFilteringAPIKeyRepoStub{keys: existing},
		&intelligenceCheckUsageUserRepoStub{admin: &User{ID: 1}}
}

// 改名后的核心不变量：库里只有【旧名】那条 Key 时必须复用它，绝不能新建。
//
// 这是本次把两个专用 Key 改短名的安全前提——若丢失旧名兜底，升级瞬间系统会按新名
// 查不到既有 Key，于是再建一个，列表里出现两个含义相同的 Key，而旧的那个还挂着
// 全部历史记账行（usage_logs.api_key_id 指向它），删不得也合并不了。
func TestResolveInternalUsageAttributionReusesLegacyNamedKey(t *testing.T) {
	const newName = "渠道监控·系统"
	const legacyName = "渠道监控记账专用Key（系统自动创建，请勿删除）"

	// 旧名是**历史事实**，不是随手写的常量：库里既有的那条 Key 就叫这个名字，
	// 且它挂着全部历史记账行。这里刻意用字面量断言常量，好在有人顺手把常量改掉时
	// 立刻被测试挡住；若两边都用常量引用，这种改动就会静默通过。
	require.Equal(t, legacyName, channelMonitorUsageKeyLegacyName)

	repo, users := newAttributionStubs(APIKey{
		ID: 77, UserID: 1, Name: legacyName, Status: StatusDisabled,
	})

	attr, err := resolveInternalUsageAttribution(context.Background(), users, repo, newName, legacyName)
	require.NoError(t, err)
	require.Equal(t, int64(77), attr.APIKeyID, "必须复用旧名那条 Key，而不是新建")
	require.Empty(t, repo.created, "不得创建任何新 Key")
}

func TestResolveInternalUsageAttributionPrefersCurrentNameOverLegacy(t *testing.T) {
	const newName = "渠道监控·系统"
	const legacyName = "渠道监控记账专用Key（系统自动创建，请勿删除）"

	// 两个名字同时存在（例如改名后又跑过一轮旧逻辑）。
	// 必须优先命中**当前名**，否则会把新记账继续挂到那条准备退役的 Key 上。
	repo, users := newAttributionStubs(
		APIKey{ID: 88, UserID: 1, Name: newName, Status: StatusDisabled},
		APIKey{ID: 77, UserID: 1, Name: legacyName, Status: StatusDisabled},
	)

	attr, err := resolveInternalUsageAttribution(context.Background(), users, repo, newName, legacyName)
	require.NoError(t, err)
	require.Equal(t, int64(88), attr.APIKeyID)
	require.Empty(t, repo.created)
}

func TestResolveInternalUsageAttributionCreatesWithCurrentName(t *testing.T) {
	const newName = "智力检测·系统"
	const legacyName = "智力检测记账专用Key（系统自动创建，请勿删除）"

	// 全新部署：库里两条都没有，此时应当新建，并且**用当前短名**建。
	repo, users := newAttributionStubs()

	attr, err := resolveInternalUsageAttribution(context.Background(), users, repo, newName, legacyName)
	require.NoError(t, err)
	require.Equal(t, int64(500), attr.APIKeyID)
	require.Len(t, repo.created, 1)
	require.Equal(t, newName, repo.created[0].Name, "新建必须用当前名，不能沿用旧的长名")
	require.Equal(t, StatusDisabled, repo.created[0].Status, "专用 Key 恒为 disabled，不参与认证")
}

func TestFindInternalUsageKeyIgnoresOtherUsersKeys(t *testing.T) {
	// 同名但属于另一个用户的 Key 不算命中，否则会把记账挂到别人名下。
	repo := &nameFilteringAPIKeyRepoStub{keys: []APIKey{
		{ID: 66, UserID: 2, Name: "渠道监控·系统", Status: StatusDisabled},
	}}

	_, ok := findInternalUsageKey(context.Background(), repo, 1, "渠道监控·系统")
	require.False(t, ok)
}

func TestFindInternalUsageKeySkipsEmptyLegacyNames(t *testing.T) {
	// 旧名登记为空串时必须跳过，不能把「空名字的 Key」当成命中。
	repo := &nameFilteringAPIKeyRepoStub{keys: []APIKey{
		{ID: 55, UserID: 1, Name: "", Status: StatusDisabled},
	}}

	_, ok := findInternalUsageKey(context.Background(), repo, 1, "渠道监控·系统", "")
	require.False(t, ok)
}
