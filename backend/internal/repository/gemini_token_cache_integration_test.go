//go:build integration

package repository

import (
	"errors"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

type GeminiTokenCacheSuite struct {
	IntegrationRedisSuite
	cache service.GeminiTokenCache
}

func (s *GeminiTokenCacheSuite) SetupTest() {
	s.IntegrationRedisSuite.SetupTest()
	s.cache = NewGeminiTokenCache(s.rdb)
}

func (s *GeminiTokenCacheSuite) TestDeleteAccessToken() {
	cacheKey := "project-123"
	token := "token-value"
	require.NoError(s.T(), s.cache.SetAccessToken(s.ctx, cacheKey, token, time.Minute))

	got, err := s.cache.GetAccessToken(s.ctx, cacheKey)
	require.NoError(s.T(), err)
	require.Equal(s.T(), token, got)

	require.NoError(s.T(), s.cache.DeleteAccessToken(s.ctx, cacheKey))

	_, err = s.cache.GetAccessToken(s.ctx, cacheKey)
	require.True(s.T(), errors.Is(err, redis.Nil), "expected redis.Nil after delete")
}

func (s *GeminiTokenCacheSuite) TestDeleteAccessToken_MissingKey() {
	require.NoError(s.T(), s.cache.DeleteAccessToken(s.ctx, "missing-key"))
}

func TestGeminiTokenCacheSuite(t *testing.T) {
	suite.Run(t, new(GeminiTokenCacheSuite))
}
