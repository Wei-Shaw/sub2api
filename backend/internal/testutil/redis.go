package testutil

import (
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/repository"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

func NewTestConcurrencyCache(t *testing.T) service.ConcurrencyCache {
	t.Helper()
	cache, _ := NewTestConcurrencyCacheWithRedis(t)
	return cache
}

func NewTestConcurrencyCacheWithRedis(t *testing.T) (service.ConcurrencyCache, *miniredis.Miniredis) {
	t.Helper()
	redisServer := miniredis.RunT(t)
	redisClient := redis.NewClient(&redis.Options{Addr: redisServer.Addr()})
	t.Cleanup(func() { _ = redisClient.Close() })
	return repository.NewConcurrencyCache(redisClient, 1, 60), redisServer
}

// NewRedisGatewayCache returns a real Redis-backed gateway cache for tests.
func NewRedisGatewayCache(t *testing.T) service.GatewayCache {
	t.Helper()

	redisServer := miniredis.RunT(t)
	redisClient := redis.NewClient(&redis.Options{Addr: redisServer.Addr()})
	t.Cleanup(func() { _ = redisClient.Close() })

	return repository.NewGatewayCache(redisClient)
}
