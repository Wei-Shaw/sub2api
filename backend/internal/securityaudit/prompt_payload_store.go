package securityaudit

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/redis/go-redis/v9"
)

type PayloadStore interface {
	Set(ctx context.Context, jobID int64, scanText string, ttl time.Duration) error
	Get(ctx context.Context, jobID int64) (string, error)
	Delete(ctx context.Context, jobID int64) error
	Ping(ctx context.Context) error
}

type RedisPayloadStore struct {
	client *redis.Client
}

func NewRedisPayloadStore(client *redis.Client) *RedisPayloadStore {
	return &RedisPayloadStore{client: client}
}

func (s *RedisPayloadStore) Set(ctx context.Context, jobID int64, scanText string, ttl time.Duration) error {
	if s == nil || s.client == nil {
		return fmt.Errorf("prompt audit payload store unavailable")
	}
	if jobID <= 0 || scanText == "" {
		return fmt.Errorf("prompt audit payload input invalid")
	}
	if ttl <= 0 || ttl > DefaultPayloadTTL {
		ttl = DefaultPayloadTTL
	}
	return s.client.Set(ctx, payloadKey(jobID), scanText, ttl).Err()
}

func (s *RedisPayloadStore) Get(ctx context.Context, jobID int64) (string, error) {
	if s == nil || s.client == nil {
		return "", fmt.Errorf("prompt audit payload store unavailable")
	}
	return s.client.Get(ctx, payloadKey(jobID)).Result()
}

func (s *RedisPayloadStore) Delete(ctx context.Context, jobID int64) error {
	if s == nil || s.client == nil {
		return fmt.Errorf("prompt audit payload store unavailable")
	}
	return s.client.Del(ctx, payloadKey(jobID)).Err()
}

func (s *RedisPayloadStore) Ping(ctx context.Context) error {
	if s == nil || s.client == nil {
		return fmt.Errorf("prompt audit payload store unavailable")
	}
	return s.client.Ping(ctx).Err()
}

func payloadKey(jobID int64) string {
	return PayloadKeyPrefix + strconv.FormatInt(jobID, 10)
}
