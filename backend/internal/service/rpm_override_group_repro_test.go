//go:build unit

package service

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"
)

// 复现：snapshot 里的 override 属于分组 10，但请求跑到分组 11 时，
// 当前实现仍然使用分组 10 的 override，导致分组 11 的 rpm_limit 失效。
func TestBillingCacheService_CheckRPM_OverrideFromAnotherGroupMustNotApply(t *testing.T) {
	zero := 0 // 分组 10 的 override：0 = 免检
	cache := &userRPMCacheStub{
		userGroupCounts: []int{1, 2, 3, 4, 5, 6}, // 分组 11 的 RPM 计数
	}
	repo := &rpmOverrideRepoStub{} // 分组 11 没有 override
	svc := newBillingServiceForRPM(t, cache, repo)

	user := &User{
		ID:                          1,
		UserGroupRPMOverride:        &zero,
		UserGroupRPMOverrideGroupID: 10,
	}
	group := &Group{ID: 11, RPMLimit: 5}

	for i := 0; i < 5; i++ {
		require.NoError(t, svc.checkRPM(context.Background(), user, group))
	}
	require.ErrorIs(t, svc.checkRPM(context.Background(), user, group), ErrGroupRPMExceeded,
		"分组 11 的 rpm_limit 应生效，不能使用分组 10 的 override")
	require.EqualValues(t, 6, atomic.LoadInt32(&repo.calls),
		"snapshot 只确认了分组 10，分组 11 应回源查询")
}

// override 属于当前分组时必须直接生效，且不应回源查询（保留 snapshot 缓存优化）。
func TestBillingCacheService_CheckRPM_SnapshotOverrideAppliesForItsOwnGroup(t *testing.T) {
	override := 2
	cache := &userRPMCacheStub{userGroupCounts: []int{1, 2, 3}}
	repo := &rpmOverrideRepoStub{err: errors.New("must not be queried")}
	svc := newBillingServiceForRPM(t, cache, repo)

	user := &User{
		ID:                          1,
		UserGroupRPMOverride:        &override,
		UserGroupRPMOverrideGroupID: 10,
	}
	group := &Group{ID: 10, RPMLimit: 100}

	require.NoError(t, svc.checkRPM(context.Background(), user, group))
	require.NoError(t, svc.checkRPM(context.Background(), user, group))
	require.ErrorIs(t, svc.checkRPM(context.Background(), user, group), ErrGroupRPMExceeded)
	require.Zero(t, atomic.LoadInt32(&repo.calls), "分组 10 的 override 已确认，不应回源")
}
