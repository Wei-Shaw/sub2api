//go:build integration

package repository

import (
	"errors"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

type UpdateCacheSuite struct {
	IntegrationRedisSuite
	cache *updateCache
}

func (s *UpdateCacheSuite) SetupTest() {
	s.IntegrationRedisSuite.SetupTest()
	s.cache = NewUpdateCache(s.rdb).(*updateCache)
}

func (s *UpdateCacheSuite) TestGetUpdateInfo_Missing() {
	_, err := s.cache.GetUpdateInfo(s.ctx)
	require.True(s.T(), errors.Is(err, redis.Nil), "expected redis.Nil for missing update info")
}

func (s *UpdateCacheSuite) TestSetAndGetUpdateInfo() {
	updateTTL := 5 * time.Minute
	require.NoError(s.T(), s.cache.SetUpdateInfo(s.ctx, "v1.2.3", updateTTL), "SetUpdateInfo")

	info, err := s.cache.GetUpdateInfo(s.ctx)
	require.NoError(s.T(), err, "GetUpdateInfo")
	require.Equal(s.T(), "v1.2.3", info, "update info mismatch")
}

func (s *UpdateCacheSuite) TestSetUpdateInfo_TTL() {
	updateTTL := 5 * time.Minute
	require.NoError(s.T(), s.cache.SetUpdateInfo(s.ctx, "v1.2.3", updateTTL))

	ttl, err := s.rdb.TTL(s.ctx, updateCacheKey).Result()
	require.NoError(s.T(), err, "TTL updateCacheKey")
	s.AssertTTLWithin(ttl, 1*time.Second, updateTTL)
}

func (s *UpdateCacheSuite) TestSetUpdateInfo_Overwrite() {
	require.NoError(s.T(), s.cache.SetUpdateInfo(s.ctx, "v1.0.0", 5*time.Minute))
	require.NoError(s.T(), s.cache.SetUpdateInfo(s.ctx, "v2.0.0", 5*time.Minute))

	info, err := s.cache.GetUpdateInfo(s.ctx)
	require.NoError(s.T(), err)
	require.Equal(s.T(), "v2.0.0", info, "expected overwritten value")
}

func (s *UpdateCacheSuite) TestSetUpdateInfo_ZeroTTL() {
	// TTL=0 means persist forever (no expiry) in Redis SET command
	require.NoError(s.T(), s.cache.SetUpdateInfo(s.ctx, "v0.0.0", 0))

	info, err := s.cache.GetUpdateInfo(s.ctx)
	require.NoError(s.T(), err)
	require.Equal(s.T(), "v0.0.0", info)

	ttl, err := s.rdb.TTL(s.ctx, updateCacheKey).Result()
	require.NoError(s.T(), err)
	// TTL=-1 means no expiry, TTL=-2 means key doesn't exist
	require.Equal(s.T(), time.Duration(-1), ttl, "expected TTL=-1 for key with no expiry")
}

func TestUpdateCacheSuite(t *testing.T) {
	suite.Run(t, new(UpdateCacheSuite))
}
