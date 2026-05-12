//go:build unit

package service

import (
	"context"
	"errors"
	"testing"
	"time"

	gocache "github.com/patrickmn/go-cache"
	"github.com/stretchr/testify/require"
)

type userGroupRateResolverRepoStub struct {
	UserGroupRateRepository

	rate  *float64
	err   error
	calls int
}

func (s *userGroupRateResolverRepoStub) GetByUserAndGroup(ctx context.Context, userID, groupID int64) (*float64, error) {
	s.calls++
	if s.err != nil {
		return nil, s.err
	}
	return s.rate, nil
}

func TestNewUserGroupRateResolver_Defaults(t *testing.T) {
	resolver := newUserGroupRateResolver(nil, nil, 0, nil, "")

	require.NotNil(t, resolver)
	require.NotNil(t, resolver.cache)
	require.Equal(t, defaultUserGroupRateCacheTTL, resolver.cacheTTL)
	require.NotNil(t, resolver.sf)
	require.Equal(t, "service.gateway", resolver.logComponent)
}

func TestUserGroupRateResolverResolve_FallbackForNilResolverAndInvalidIDs(t *testing.T) {
	var nilResolver *userGroupRateResolver
	require.Equal(t, 1.4, nilResolver.Resolve(context.Background(), 101, 202, 1.4))

	resolver := newUserGroupRateResolver(nil, nil, time.Second, nil, "service.test")
	require.Equal(t, 1.4, resolver.Resolve(context.Background(), 0, 202, 1.4))
	require.Equal(t, 1.4, resolver.Resolve(context.Background(), 101, 0, 1.4))
}

func TestUserGroupRateResolverResolve_InvalidCacheEntryLoadsRepoAndCaches(t *testing.T) {
	resetGatewayHotpathStatsForTest()

	rate := 1.7
	repo := &userGroupRateResolverRepoStub{rate: &rate}
	cache := gocache.New(time.Minute, time.Minute)
	cache.Set("101:202", "bad-cache", time.Minute)
	resolver := newUserGroupRateResolver(repo, cache, time.Minute, nil, "service.test")

	got := resolver.Resolve(context.Background(), 101, 202, 1.2)
	require.Equal(t, rate, got)
	require.Equal(t, 1, repo.calls)

	cached, ok := cache.Get("101:202")
	require.True(t, ok)
	require.Equal(t, rate, cached)

	hit, miss, load, _, fallback := GatewayUserGroupRateCacheStats()
	require.Equal(t, int64(0), hit)
	require.Equal(t, int64(1), miss)
	require.Equal(t, int64(1), load)
	require.Equal(t, int64(0), fallback)
}

func TestGatewayServiceGetUserGroupRateMultiplier_FallbacksAndUsesExistingResolver(t *testing.T) {
	var nilSvc *GatewayService
	require.Equal(t, 1.3, nilSvc.getUserGroupRateMultiplier(context.Background(), 101, 202, 1.3))

	rate := 1.9
	repo := &userGroupRateResolverRepoStub{rate: &rate}
	resolver := newUserGroupRateResolver(repo, nil, time.Minute, nil, "service.gateway")
	svc := &GatewayService{userGroupRateResolver: resolver}

	got := svc.getUserGroupRateMultiplier(context.Background(), 101, 202, 1.2)
	require.Equal(t, rate, got)
	require.Equal(t, 1, repo.calls)
}

// poolAwareRateRepoStub implements UserGroupRateRepository with Pool fallback tracking.
type poolAwareRateRepoStub struct {
	UserGroupRateRepository

	directRate *float64
	directErr  error

	poolGrant     PoolGroupGrantSelection
	poolErr       error
	poolCallCount int
}

func (s *poolAwareRateRepoStub) GetByUserAndGroup(_ context.Context, _, _ int64) (*float64, error) {
	return s.directRate, s.directErr
}

func (s *poolAwareRateRepoStub) GetPoolGroupGrantByUserAndGroup(_ context.Context, _, _ int64) (PoolGroupGrantSelection, error) {
	s.poolCallCount++
	return s.poolGrant, s.poolErr
}

func ptrFloat64ForResolver(v float64) *float64 { return &v }
func ptrIntForResolver(v int) *int             { return &v }

// TestResolve_PoolFallback_GrantFound: Pool grant has rate, should use it.
func TestResolve_PoolFallback_GrantFound(t *testing.T) {
	resetGatewayHotpathStatsForTest()

	repo := &poolAwareRateRepoStub{
		directRate: nil,
		poolGrant: PoolGroupGrantSelection{
			Found:          true,
			RateMultiplier: ptrFloat64ForResolver(2.5),
		},
	}
	resolver := newUserGroupRateResolverWithConfig(repo, nil, time.Minute, nil, "service.test")

	got := resolver.Resolve(context.Background(), 101, 202, 1.0)
	require.Equal(t, 2.5, got, "should use Pool grant rate multiplier")
	require.Equal(t, 1, repo.poolCallCount, "should call GetPoolGroupGrantByUserAndGroup exactly once")
}

// TestResolve_PoolFallback_NullRateMultiplier: Pool grant found but RateMultiplier=nil → use group default, no bigger Pool.
func TestResolve_PoolFallback_NullRateMultiplier(t *testing.T) {
	resetGatewayHotpathStatsForTest()

	repo := &poolAwareRateRepoStub{
		directRate: nil,
		poolGrant: PoolGroupGrantSelection{
			Found:          true,
			RateMultiplier: nil, // NULL: inherit group default, don't query bigger pool
		},
	}
	resolver := newUserGroupRateResolverWithConfig(repo, nil, time.Minute, nil, "service.test")

	got := resolver.Resolve(context.Background(), 101, 202, 1.0)
	require.Equal(t, 1.0, got, "null RateMultiplier on smallest pool_id should inherit group default")
	require.Equal(t, 1, repo.poolCallCount, "should only call once – not query bigger pools")
}

// TestResolve_PoolFallback_QueryFails: Pool query error → fallback to group default, counter incremented.
func TestResolve_PoolFallback_QueryFails(t *testing.T) {
	resetGatewayHotpathStatsForTest()

	repo := &poolAwareRateRepoStub{
		directRate: nil,
		poolErr:    errors.New("pool query failed"),
	}
	resolver := newUserGroupRateResolverWithConfig(repo, nil, time.Minute, nil, "service.test")

	got := resolver.Resolve(context.Background(), 101, 202, 1.0)
	require.Equal(t, 1.0, got, "pool query failure should fall back to group default")
	require.Equal(t, int64(1), GatewayUserPoolRateFallbackQueryErrorTotal(),
		"error counter should be incremented on pool query failure")
}
