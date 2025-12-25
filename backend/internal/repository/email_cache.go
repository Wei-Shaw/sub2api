package repository

import (
	"context"
	"encoding/json"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service/ports"

	"github.com/redis/go-redis/v9"
)

const verifyCodeKeyPrefix = "verify_code:"

type emailCache struct {
	rdb *redis.Client
}

func NewEmailCache(rdb *redis.Client) ports.EmailCache {
	return &emailCache{rdb: rdb}
}

func (c *emailCache) GetVerificationCode(ctx context.Context, email string) (*ports.VerificationCodeData, error) {
	key := verifyCodeKeyPrefix + email
	val, err := c.rdb.Get(ctx, key).Result()
	if err != nil {
		return nil, err
	}
	var data ports.VerificationCodeData
	if err := json.Unmarshal([]byte(val), &data); err != nil {
		return nil, err
	}
	return &data, nil
}

func (c *emailCache) SetVerificationCode(ctx context.Context, email string, data *ports.VerificationCodeData, ttl time.Duration) error {
	key := verifyCodeKeyPrefix + email
	val, err := json.Marshal(data)
	if err != nil {
		return err
	}
	return c.rdb.Set(ctx, key, val, ttl).Err()
}

func (c *emailCache) DeleteVerificationCode(ctx context.Context, email string) error {
	key := verifyCodeKeyPrefix + email
	return c.rdb.Del(ctx, key).Err()
}
