package repository

import (
	"context"
	"fmt"
	"time"

	"sub2api/internal/service/ports"

	"github.com/redis/go-redis/v9"
)

const (
	apiKeyRateLimitKeyPrefix = "apikey:ratelimit:"
	apiKeyRateLimitDuration  = 24 * time.Hour
)

type apiKeyCache struct {
	rdb *redis.Client
REDACTED

func NewApiKeyCache(rdb *redis.Client) ports.ApiKeyCache {
	return &apiKeyCache{rdb: rdbREDACTED
REDACTED

func (c *apiKeyCache) GetCreateAttemptCount(ctx context.Context, userID int64) (int, error) {
	key := fmt.Sprintf("%s%d", apiKeyRateLimitKeyPrefix, userID)
	return c.rdb.Get(ctx, key).Int()
REDACTED

func (c *apiKeyCache) IncrementCreateAttemptCount(ctx context.Context, userID int64) error {
	key := fmt.Sprintf("%s%d", apiKeyRateLimitKeyPrefix, userID)
	pipe := c.rdb.Pipeline()
	pipe.Incr(ctx, key)
	pipe.Expire(ctx, key, apiKeyRateLimitDuration)
	_, err := pipe.Exec(ctx)
	return err
REDACTED

func (c *apiKeyCache) DeleteCreateAttemptCount(ctx context.Context, userID int64) error {
	key := fmt.Sprintf("%s%d", apiKeyRateLimitKeyPrefix, userID)
	return c.rdb.Del(ctx, key).Err()
REDACTED

func (c *apiKeyCache) IncrementDailyUsage(ctx context.Context, apiKey string) error {
	return c.rdb.Incr(ctx, apiKey).Err()
REDACTED

func (c *apiKeyCache) SetDailyUsageExpiry(ctx context.Context, apiKey string, ttl time.Duration) error {
	return c.rdb.Expire(ctx, apiKey, ttl).Err()
REDACTED
