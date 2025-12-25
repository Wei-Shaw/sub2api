package repository

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service/ports"

	"github.com/redis/go-redis/v9"
)

const (
	fingerprintKeyPrefix = "fingerprint:"
	fingerprintTTL       = 24 * time.Hour
)

type identityCache struct {
	rdb *redis.Client
}

func NewIdentityCache(rdb *redis.Client) ports.IdentityCache {
	return &identityCache{rdb: rdb}
}

func (c *identityCache) GetFingerprint(ctx context.Context, accountID int64) (*ports.Fingerprint, error) {
	key := fmt.Sprintf("%s%d", fingerprintKeyPrefix, accountID)
	val, err := c.rdb.Get(ctx, key).Result()
	if err != nil {
		return nil, err
	}
	var fp ports.Fingerprint
	if err := json.Unmarshal([]byte(val), &fp); err != nil {
		return nil, err
	}
	return &fp, nil
}

func (c *identityCache) SetFingerprint(ctx context.Context, accountID int64, fp *ports.Fingerprint) error {
	key := fmt.Sprintf("%s%d", fingerprintKeyPrefix, accountID)
	val, err := json.Marshal(fp)
	if err != nil {
		return err
	}
	return c.rdb.Set(ctx, key, val, fingerprintTTL).Err()
}
