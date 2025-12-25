package repository

import (
	"context"
	"fmt"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service/ports"

	"github.com/redis/go-redis/v9"
)

const (
	geminiTokenKeyPrefix       = "gemini:token:"
	geminiRefreshLockKeyPrefix = "gemini:refresh_lock:"
)

type geminiTokenCache struct {
	rdb *redis.Client
REDACTED

func NewGeminiTokenCache(rdb *redis.Client) ports.GeminiTokenCache {
	return &geminiTokenCache{rdb: rdbREDACTED
REDACTED

func (c *geminiTokenCache) GetAccessToken(ctx context.Context, cacheKey string) (string, error) {
	key := fmt.Sprintf("%s%s", geminiTokenKeyPrefix, cacheKey)
	return c.rdb.Get(ctx, key).Result()
REDACTED

func (c *geminiTokenCache) SetAccessToken(ctx context.Context, cacheKey string, token string, ttl time.Duration) error {
	key := fmt.Sprintf("%s%s", geminiTokenKeyPrefix, cacheKey)
	return c.rdb.Set(ctx, key, token, ttl).Err()
REDACTED

func (c *geminiTokenCache) AcquireRefreshLock(ctx context.Context, cacheKey string, ttl time.Duration) (bool, error) {
	key := fmt.Sprintf("%s%s", geminiRefreshLockKeyPrefix, cacheKey)
	return c.rdb.SetNX(ctx, key, 1, ttl).Result()
REDACTED

func (c *geminiTokenCache) ReleaseRefreshLock(ctx context.Context, cacheKey string) error {
	key := fmt.Sprintf("%s%s", geminiRefreshLockKeyPrefix, cacheKey)
	return c.rdb.Del(ctx, key).Err()
REDACTED
