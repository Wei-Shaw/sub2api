package repository

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/redis/go-redis/v9"
)

const (
	apiKeyRateLimitKeyPrefix = "apikey:ratelimit:"
	apiKeyRateLimitDuration  = 24 * time.Hour
	apiKeyAuthCachePrefix    = "apikey:auth:"
)

// apiKeyRateLimitKey generates the Redis key for API key creation rate limiting.
func apiKeyRateLimitKey(userID int64) string {
	return fmt.Sprintf("%s%d", apiKeyRateLimitKeyPrefix, userID)
REDACTED

func apiKeyAuthCacheKey(key string) string {
	return fmt.Sprintf("%s%s", apiKeyAuthCachePrefix, key)
REDACTED

type apiKeyCache struct {
	rdb *redis.Client
REDACTED

func NewAPIKeyCache(rdb *redis.Client) service.APIKeyCache {
	return &apiKeyCache{rdb: rdbREDACTED
REDACTED

func (c *apiKeyCache) GetCreateAttemptCount(ctx context.Context, userID int64) (int, error) {
	key := apiKeyRateLimitKey(userID)
	count, err := c.rdb.Get(ctx, key).Int()
	if errors.Is(err, redis.Nil) {
		return 0, nil
REDACTED
	return count, err
REDACTED

func (c *apiKeyCache) IncrementCreateAttemptCount(ctx context.Context, userID int64) error {
	key := apiKeyRateLimitKey(userID)
	pipe := c.rdb.Pipeline()
	pipe.Incr(ctx, key)
	pipe.Expire(ctx, key, apiKeyRateLimitDuration)
	_, err := pipe.Exec(ctx)
	return err
REDACTED

func (c *apiKeyCache) DeleteCreateAttemptCount(ctx context.Context, userID int64) error {
	key := apiKeyRateLimitKey(userID)
	return c.rdb.Del(ctx, key).Err()
REDACTED

func (c *apiKeyCache) IncrementDailyUsage(ctx context.Context, apiKey string) error {
	return c.rdb.Incr(ctx, apiKey).Err()
REDACTED

func (c *apiKeyCache) SetDailyUsageExpiry(ctx context.Context, apiKey string, ttl time.Duration) error {
	return c.rdb.Expire(ctx, apiKey, ttl).Err()
REDACTED

func (c *apiKeyCache) GetAuthCache(ctx context.Context, key string) (*service.APIKeyAuthCacheEntry, error) {
	val, err := c.rdb.Get(ctx, apiKeyAuthCacheKey(key)).Bytes()
	if err != nil {
		return nil, err
REDACTED
	var entry service.APIKeyAuthCacheEntry
	if err := json.Unmarshal(val, &entry); err != nil {
		return nil, err
REDACTED
	return &entry, nil
REDACTED

func (c *apiKeyCache) SetAuthCache(ctx context.Context, key string, entry *service.APIKeyAuthCacheEntry, ttl time.Duration) error {
	if entry == nil {
		return nil
REDACTED
	payload, err := json.Marshal(entry)
	if err != nil {
		return err
REDACTED
	return c.rdb.Set(ctx, apiKeyAuthCacheKey(key), payload, ttl).Err()
REDACTED

func (c *apiKeyCache) DeleteAuthCache(ctx context.Context, key string) error {
	return c.rdb.Del(ctx, apiKeyAuthCacheKey(key)).Err()
REDACTED
