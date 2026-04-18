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
	fingerprintKeyPrefix       = "fingerprint:"
	fingerprintTTL             = 7 * 24 * time.Hour // 7天，配合每24小时懒续期可保持活跃账号永不过期
	maskedSessionKeyPrefix     = "masked_session:"
	maskedSessionTTL           = 15 * time.Minute
	stickySessionUUIDKeyPrefix = "sticky_session_uuid:"
	// 30 分钟 TTL 贴近一次真实 Claude Code CLI 会话的典型生命周期；
	// 会话窗口内复用 UUID，之后自然换新。
	stickySessionUUIDTTL = 30 * time.Minute
)

// fingerprintKey generates the Redis key for account fingerprint cache.
func fingerprintKey(accountID int64) string {
	return fmt.Sprintf("%s%d", fingerprintKeyPrefix, accountID)
}

// maskedSessionKey generates the Redis key for masked session ID cache.
func maskedSessionKey(accountID int64) string {
	return fmt.Sprintf("%s%d", maskedSessionKeyPrefix, accountID)
}

// stickySessionUUIDKey generates the Redis key for the (accountID, sessionHash)-scoped
// session UUID used to keep metadata.session_id stable within a single CLI session.
func stickySessionUUIDKey(accountID int64, sessionHash string) string {
	return fmt.Sprintf("%s%d:%s", stickySessionUUIDKeyPrefix, accountID, sessionHash)
}

type identityCache struct {
	rdb *redis.Client
}

func NewIdentityCache(rdb *redis.Client) service.IdentityCache {
	return &identityCache{rdb: rdb}
}

// GetFingerprint returns the cached fingerprint for accountID.
// Returns (nil, nil) when the key doesn't exist; (nil, err) only on
// transient Redis failures so callers can preserve identity on outage
// instead of minting a fresh ClientID.
func (c *identityCache) GetFingerprint(ctx context.Context, accountID int64) (*service.Fingerprint, error) {
	key := fingerprintKey(accountID)
	val, err := c.rdb.Get(ctx, key).Result()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return nil, nil
		}
		return nil, err
	}
	var fp service.Fingerprint
	if err := json.Unmarshal([]byte(val), &fp); err != nil {
		return nil, err
	}
	return &fp, nil
}

func (c *identityCache) SetFingerprint(ctx context.Context, accountID int64, fp *service.Fingerprint) error {
	key := fingerprintKey(accountID)
	val, err := json.Marshal(fp)
	if err != nil {
		return err
	}
	return c.rdb.Set(ctx, key, val, fingerprintTTL).Err()
}

func (c *identityCache) GetMaskedSessionID(ctx context.Context, accountID int64) (string, error) {
	key := maskedSessionKey(accountID)
	val, err := c.rdb.Get(ctx, key).Result()
	if err != nil {
		if err == redis.Nil {
			return "", nil
		}
		return "", err
	}
	return val, nil
}

func (c *identityCache) SetMaskedSessionID(ctx context.Context, accountID int64, sessionID string) error {
	key := maskedSessionKey(accountID)
	return c.rdb.Set(ctx, key, sessionID, maskedSessionTTL).Err()
}

func (c *identityCache) GetStickySessionUUID(ctx context.Context, accountID int64, sessionHash string) (string, error) {
	if sessionHash == "" {
		return "", nil
	}
	val, err := c.rdb.Get(ctx, stickySessionUUIDKey(accountID, sessionHash)).Result()
	if err != nil {
		if err == redis.Nil {
			return "", nil
		}
		return "", err
	}
	return val, nil
}

func (c *identityCache) SetStickySessionUUID(ctx context.Context, accountID int64, sessionHash string, sessionUUID string) error {
	if sessionHash == "" || sessionUUID == "" {
		return nil
	}
	return c.rdb.Set(ctx, stickySessionUUIDKey(accountID, sessionHash), sessionUUID, stickySessionUUIDTTL).Err()
}
