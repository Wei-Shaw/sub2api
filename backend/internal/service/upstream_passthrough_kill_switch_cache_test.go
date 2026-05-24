package service

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

func resetUpstreamPassthroughGlobalOverrideCache() {
	upstreamPassthroughGlobalOverrideCache.Store((*cachedUpstreamPassthroughGlobalOverride)(nil))
}

func TestGetUpstreamPassthroughGlobalOverride_CachesAcrossCalls(t *testing.T) {
	resetUpstreamPassthroughGlobalOverrideCache()
	repo := &fakeSettingRepoForUpstream{values: map[string]string{}}
	svc := NewSettingService(repo, &config.Config{})

	_ = svc.GetUpstreamPassthroughGlobalOverride(context.Background())
	firstCalls := repo.getCalls
	_ = svc.GetUpstreamPassthroughGlobalOverride(context.Background())
	_ = svc.GetUpstreamPassthroughGlobalOverride(context.Background())
	require.Equal(t, firstCalls, repo.getCalls, "subsequent calls should hit cache")
}

func TestGetUpstreamPassthroughGlobalOverride_SetInvalidatesCache(t *testing.T) {
	resetUpstreamPassthroughGlobalOverrideCache()
	repo := &fakeSettingRepoForUpstream{values: map[string]string{}}
	svc := NewSettingService(repo, &config.Config{})

	// Warm cache
	got := svc.GetUpstreamPassthroughGlobalOverride(context.Background())
	require.Equal(t, GlobalOverrideAuto, got)
	priorCalls := repo.getCalls

	// Set a new value
	require.NoError(t, svc.SetUpstreamPassthroughGlobalOverride(context.Background(), GlobalOverrideForceStrict))

	// Next get must refetch
	got = svc.GetUpstreamPassthroughGlobalOverride(context.Background())
	require.Equal(t, GlobalOverrideForceStrict, got)
	require.Greater(t, repo.getCalls, priorCalls, "Set must invalidate cache so next Get refetches")
}

func TestGetUpstreamPassthroughGlobalOverride_TTLExpiry(t *testing.T) {
	resetUpstreamPassthroughGlobalOverrideCache()
	// Manually warm the cache with a past expiry
	upstreamPassthroughGlobalOverrideCache.Store(&cachedUpstreamPassthroughGlobalOverride{
		value:     GlobalOverrideForceProtected,
		expiresAt: time.Now().Add(-1 * time.Second).UnixNano(),
	})
	repo := &fakeSettingRepoForUpstream{values: map[string]string{}}
	svc := NewSettingService(repo, &config.Config{})

	// Expired cache → refetch → returns auto (no stored value)
	got := svc.GetUpstreamPassthroughGlobalOverride(context.Background())
	require.Equal(t, GlobalOverrideAuto, got)
}

func TestGetUpstreamPassthroughGlobalOverride_ConcurrentReadsSafe(t *testing.T) {
	resetUpstreamPassthroughGlobalOverrideCache()
	repo := &fakeSettingRepoForUpstream{values: map[string]string{
		SettingKeyUpstreamPassthroughGlobalOverride: string(GlobalOverrideForceStrict),
	}}
	svc := NewSettingService(repo, &config.Config{})

	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			got := svc.GetUpstreamPassthroughGlobalOverride(context.Background())
			require.Equal(t, GlobalOverrideForceStrict, got)
		}()
	}
	wg.Wait()
}
