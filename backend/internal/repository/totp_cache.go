package repository

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/Wei-Shaw/sub2api/internal/service"
)

var consumeTotpLoginSessionScript = redis.NewScript(`
local value = redis.call("GET", KEYS[1])
if value then
  redis.call("DEL", KEYS[1])
end
return value
`)

const (
	totpSetupKeyPrefix    = "totp:setup:"
	totpLoginKeyPrefix    = "totp:login:"
	totpAttemptsKeyPrefix = "totp:attempts:"
	totpStepUpKeyPrefix   = "totp:stepup:"
	totpAttemptsTTL       = 15 * time.Minute
)

// TotpCache implements service.TotpCache using Redis
type TotpCache struct {
	rdb *redis.Client
}

// NewTotpCache creates a new TOTP cache
func NewTotpCache(rdb *redis.Client) service.TotpCache {
	return &TotpCache{rdb: rdb}
}

// GetSetupSession retrieves a TOTP setup session
func (c *TotpCache) GetSetupSession(ctx context.Context, userID int64) (*service.TotpSetupSession, error) {
	key := fmt.Sprintf("%s%d", totpSetupKeyPrefix, userID)
	data, err := c.rdb.Get(ctx, key).Bytes()
	if err != nil {
		if err == redis.Nil {
			return nil, nil
		}
		return nil, fmt.Errorf("get setup session: %w", err)
	}

	var session service.TotpSetupSession
	if err := json.Unmarshal(data, &session); err != nil {
		return nil, fmt.Errorf("unmarshal setup session: %w", err)
	}

	return &session, nil
}

// SetSetupSession stores a TOTP setup session
func (c *TotpCache) SetSetupSession(ctx context.Context, userID int64, session *service.TotpSetupSession, ttl time.Duration) error {
	key := fmt.Sprintf("%s%d", totpSetupKeyPrefix, userID)
	data, err := json.Marshal(session)
	if err != nil {
		return fmt.Errorf("marshal setup session: %w", err)
	}

	if err := c.rdb.Set(ctx, key, data, ttl).Err(); err != nil {
		return fmt.Errorf("set setup session: %w", err)
	}

	return nil
}

// DeleteSetupSession deletes a TOTP setup session
func (c *TotpCache) DeleteSetupSession(ctx context.Context, userID int64) error {
	key := fmt.Sprintf("%s%d", totpSetupKeyPrefix, userID)
	return c.rdb.Del(ctx, key).Err()
}

// GetLoginSession retrieves a TOTP login session
func (c *TotpCache) GetLoginSession(ctx context.Context, tempToken string) (*service.TotpLoginSession, error) {
	key := totpLoginKeyPrefix + tempToken
	data, err := c.rdb.Get(ctx, key).Bytes()
	if err != nil {
		if err == redis.Nil {
			return nil, nil
		}
		return nil, fmt.Errorf("get login session: %w", err)
	}

	var session service.TotpLoginSession
	if err := json.Unmarshal(data, &session); err != nil {
		return nil, fmt.Errorf("unmarshal login session: %w", err)
	}

	return &session, nil
}

// SetLoginSession stores a TOTP login session
func (c *TotpCache) SetLoginSession(ctx context.Context, tempToken string, session *service.TotpLoginSession, ttl time.Duration) error {
	key := totpLoginKeyPrefix + tempToken
	data, err := json.Marshal(session)
	if err != nil {
		return fmt.Errorf("marshal login session: %w", err)
	}

	if err := c.rdb.Set(ctx, key, data, ttl).Err(); err != nil {
		return fmt.Errorf("set login session: %w", err)
	}

	return nil
}

// ConsumeLoginSession atomically returns and deletes a TOTP login session.
// A Lua GET+DEL is used instead of separate commands so concurrent valid-code
// requests cannot all proceed to token issuance.
func (c *TotpCache) ConsumeLoginSession(ctx context.Context, tempToken string) (*service.TotpLoginSession, error) {
	key := totpLoginKeyPrefix + tempToken
	result, err := consumeTotpLoginSessionScript.Run(ctx, c.rdb, []string{key}).Result()
	if err != nil {
		if err == redis.Nil {
			return nil, nil
		}
		return nil, fmt.Errorf("consume login session: %w", err)
	}
	data, ok := result.(string)
	if !ok {
		return nil, fmt.Errorf("consume login session: unexpected redis result type %T", result)
	}

	var session service.TotpLoginSession
	if err := json.Unmarshal([]byte(data), &session); err != nil {
		return nil, fmt.Errorf("unmarshal consumed login session: %w", err)
	}
	return &session, nil
}

// DeleteLoginSession deletes a TOTP login session
func (c *TotpCache) DeleteLoginSession(ctx context.Context, tempToken string) error {
	key := totpLoginKeyPrefix + tempToken
	return c.rdb.Del(ctx, key).Err()
}

// IncrementVerifyAttempts increments the verify attempt counter
func (c *TotpCache) IncrementVerifyAttempts(ctx context.Context, userID int64) (int, error) {
	key := fmt.Sprintf("%s%d", totpAttemptsKeyPrefix, userID)

	// Use pipeline for atomic increment and set TTL
	pipe := c.rdb.Pipeline()
	incrCmd := pipe.Incr(ctx, key)
	pipe.Expire(ctx, key, totpAttemptsTTL)

	if _, err := pipe.Exec(ctx); err != nil {
		return 0, fmt.Errorf("increment verify attempts: %w", err)
	}

	count, err := incrCmd.Result()
	if err != nil {
		return 0, fmt.Errorf("get increment result: %w", err)
	}

	return int(count), nil
}

// GetVerifyAttempts gets the current verify attempt count
func (c *TotpCache) GetVerifyAttempts(ctx context.Context, userID int64) (int, error) {
	key := fmt.Sprintf("%s%d", totpAttemptsKeyPrefix, userID)
	count, err := c.rdb.Get(ctx, key).Int()
	if err != nil {
		if err == redis.Nil {
			return 0, nil
		}
		return 0, fmt.Errorf("get verify attempts: %w", err)
	}
	return count, nil
}

// ClearVerifyAttempts clears the verify attempt counter
func (c *TotpCache) ClearVerifyAttempts(ctx context.Context, userID int64) error {
	key := fmt.Sprintf("%s%d", totpAttemptsKeyPrefix, userID)
	return c.rdb.Del(ctx, key).Err()
}

func totpStepUpKey(userID int64, sessionKey string) string {
	return fmt.Sprintf("%s%d:%s", totpStepUpKeyPrefix, userID, sessionKey)
}

// SetStepUpGrant 记录一次 step-up 验证通过（sudo 窗口），绑定用户+会话。
func (c *TotpCache) SetStepUpGrant(ctx context.Context, userID int64, sessionKey, credentialGeneration string, ttl time.Duration) error {
	if credentialGeneration == "" {
		return fmt.Errorf("set step-up grant: empty credential generation")
	}
	return c.rdb.Set(ctx, totpStepUpKey(userID, sessionKey), credentialGeneration, ttl).Err()
}

// HasStepUpGrant 检查 step-up 授权是否仍在有效期内。
func (c *TotpCache) HasStepUpGrant(ctx context.Context, userID int64, sessionKey, credentialGeneration string) (bool, error) {
	storedGeneration, err := c.rdb.Get(ctx, totpStepUpKey(userID, sessionKey)).Result()
	if err != nil {
		if err == redis.Nil {
			return false, nil
		}
		return false, fmt.Errorf("get step-up grant: %w", err)
	}
	return subtle.ConstantTimeCompare([]byte(storedGeneration), []byte(credentialGeneration)) == 1, nil
}
