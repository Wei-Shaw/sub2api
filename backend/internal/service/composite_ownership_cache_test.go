package service

import (
	"context"
	"testing"
	"time"

	gocache "github.com/patrickmn/go-cache"
	"github.com/stretchr/testify/require"
)

type countingOwnershipRepo struct {
	compositeOwnershipScopedRepo
	calls int
}

func (r *countingOwnershipRepo) ListModelAvailabilityCandidates(ctx context.Context, groupID *int64, platforms []string, includeUnschedulable bool) ([]Account, error) {
	r.calls++
	return r.compositeOwnershipScopedRepo.ListModelAvailabilityCandidates(ctx, groupID, platforms, includeUnschedulable)
}

// 归属解析在每个 composite 请求上执行，结果只取决于持久化配置：同一分组+模型在
// TTL 内只查一次库，且缓存值与调用方不共享候选平台切片。
func TestResolveCompositeModelOwnershipCachesConfigResult(t *testing.T) {
	const group = int64(9)
	repo := &countingOwnershipRepo{compositeOwnershipScopedRepo: compositeOwnershipScopedRepo{records: []compositeOwnershipScopedRecord{
		{groupID: group, platform: PlatformKimi, schedulable: true, account: mappingAccount(1, PlatformKimi, "shared", "kimi-k3")},
		{groupID: group, platform: PlatformDeepseek, schedulable: true, account: mappingAccount(2, PlatformDeepseek, "shared", "deepseek-v4-pro")},
	}}}
	svc := &GatewayService{accountRepo: repo, modelsListCache: gocache.New(time.Minute, time.Minute), modelsListCacheTTL: time.Minute}

	first, err := svc.resolveCompositeModelOwnership(context.Background(), group, "shared")
	require.NoError(t, err)
	require.True(t, first.Matched)
	require.Equal(t, []string{PlatformDeepseek, PlatformKimi}, first.CandidatePlatforms)
	first.CandidatePlatforms[0] = "mutated"

	second, err := svc.resolveCompositeModelOwnership(context.Background(), group, "shared")
	require.NoError(t, err)
	require.Equal(t, 1, repo.calls, "TTL 内同一分组+模型不得重复查库")
	require.Equal(t, []string{PlatformDeepseek, PlatformKimi}, second.CandidatePlatforms, "缓存值不得被调用方修改")

	_, err = svc.resolveCompositeModelOwnership(context.Background(), group, "other-model")
	require.NoError(t, err)
	require.Equal(t, 2, repo.calls, "不同模型各自解析")
}
