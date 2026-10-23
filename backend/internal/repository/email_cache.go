package repository

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/redis/go-redis/v9"
)

const (
	verifyCodeKeyPrefix          = "verify_code:"
	notifyVerifyKeyPrefix        = "notify_verify:"
	passwordResetKeyPrefix       = "password_reset:"
	passwordResetSentAtKeyPrefix = "password_reset_sent:"
	notifyCodeUserRateKeyPrefix  = "notify_code_user_rate:"

	// attemptsKeySuffix stores the failed-attempt counter next to a verification code.
	// Kept in a separate key so it can be incremented atomically with INCR.
	attemptsKeySuffix = ":attempts"
)

// incrAttemptsScript atomically inherits legacy JSON attempts, increments the
// greater of the legacy and separate counters, and preserves the code's TTL.
// KEYS[1] = code key, KEYS[2] = attempts key. Returns -1 for a missing/invalid code.
var incrAttemptsScript = redis.NewScript(`
local raw = redis.call('GET', KEYS[1])
if not raw then
  return -1
end
local ok, data = pcall(cjson.decode, raw)
if not ok or type(data) ~= 'table' then
  return -1
end
local ttl = redis.call('PTTL', KEYS[1])
if ttl == -2 or ttl == 0 then
  return -1
end
local legacy = tonumber(data['Attempts']) or 0
local current = tonumber(redis.call('GET', KEYS[2])) or 0
if legacy > current then
  redis.call('SET', KEYS[2], legacy)
end
local n = redis.call('INCR', KEYS[2])
if ttl > 0 then
  redis.call('PEXPIRE', KEYS[2], ttl)
end
return n
`)

// consumeResetTokenScript atomically compares the stored token hash and deletes it.
// KEYS[1] = reset key, ARGV[1] = expected token hash. Returns 1 on success, 0 otherwise.
var consumeResetTokenScript = redis.NewScript(`
local v = redis.call('GET', KEYS[1])
if not v then
  return 0
end
local ok, d = pcall(cjson.decode, v)
if not ok or type(d) ~= 'table' or d['Token'] ~= ARGV[1] then
  return 0
end
redis.call('DEL', KEYS[1])
return 1
`)

// verifyCodeKey generates the Redis key for email verification code.
// Email is lowercased for case-insensitive consistency.
func verifyCodeKey(email string) string {
	return verifyCodeKeyPrefix + strings.ToLower(email)
}

// notifyVerifyKey generates the Redis key for notify email verification code.
// Email is lowercased to prevent case-sensitive key mismatch (the business layer
// uses strings.EqualFold for comparison).
func notifyVerifyKey(email string) string {
	return notifyVerifyKeyPrefix + strings.ToLower(email)
}

// passwordResetKey generates the Redis key for password reset token.
func passwordResetKey(email string) string {
	return passwordResetKeyPrefix + strings.ToLower(email)
}

// passwordResetSentAtKey generates the Redis key for password reset email sent timestamp.
func passwordResetSentAtKey(email string) string {
	return passwordResetSentAtKeyPrefix + strings.ToLower(email)
}

type emailCache struct {
	rdb *redis.Client
}

func NewEmailCache(rdb *redis.Client) service.EmailCache {
	return &emailCache{rdb: rdb}
}

func (c *emailCache) getCode(ctx context.Context, key string) (*service.VerificationCodeData, error) {
	val, err := c.rdb.Get(ctx, key).Result()
	if err != nil {
		return nil, err
	}
	var data service.VerificationCodeData
	if err := json.Unmarshal([]byte(val), &data); err != nil {
		return nil, err
	}
	if n, err := c.rdb.Get(ctx, key+attemptsKeySuffix).Int(); err == nil && n > data.Attempts {
		data.Attempts = n
	}
	return &data, nil
}

func (c *emailCache) setCode(ctx context.Context, key string, data *service.VerificationCodeData, ttl time.Duration) error {
	val, err := json.Marshal(data)
	if err != nil {
		return err
	}
	pipe := c.rdb.TxPipeline()
	pipe.Set(ctx, key, val, ttl)
	pipe.Del(ctx, key+attemptsKeySuffix)
	if data.Attempts > 0 {
		pipe.Set(ctx, key+attemptsKeySuffix, data.Attempts, ttl)
	}
	_, err = pipe.Exec(ctx)
	return err
}

func (c *emailCache) incrCodeAttempts(ctx context.Context, key string) (int, error) {
	n, err := incrAttemptsScript.Run(ctx, c.rdb, []string{key, key + attemptsKeySuffix}).Int()
	if err != nil {
		return 0, err
	}
	if n < 0 {
		return 0, redis.Nil
	}
	return n, nil
}

func (c *emailCache) deleteCode(ctx context.Context, key string) error {
	return c.rdb.Del(ctx, key, key+attemptsKeySuffix).Err()
}

func (c *emailCache) GetVerificationCode(ctx context.Context, email string) (*service.VerificationCodeData, error) {
	return c.getCode(ctx, verifyCodeKey(email))
}

func (c *emailCache) SetVerificationCode(ctx context.Context, email string, data *service.VerificationCodeData, ttl time.Duration) error {
	return c.setCode(ctx, verifyCodeKey(email), data, ttl)
}

func (c *emailCache) IncrVerificationCodeAttempts(ctx context.Context, email string) (int, error) {
	return c.incrCodeAttempts(ctx, verifyCodeKey(email))
}

func (c *emailCache) DeleteVerificationCode(ctx context.Context, email string) error {
	return c.deleteCode(ctx, verifyCodeKey(email))
}

// Password reset token methods

func (c *emailCache) GetPasswordResetToken(ctx context.Context, email string) (*service.PasswordResetTokenData, error) {
	key := passwordResetKey(email)
	val, err := c.rdb.Get(ctx, key).Result()
	if err != nil {
		return nil, err
	}
	var data service.PasswordResetTokenData
	if err := json.Unmarshal([]byte(val), &data); err != nil {
		return nil, err
	}
	return &data, nil
}

func (c *emailCache) SetPasswordResetToken(ctx context.Context, email string, data *service.PasswordResetTokenData, ttl time.Duration) error {
	key := passwordResetKey(email)
	val, err := json.Marshal(data)
	if err != nil {
		return err
	}
	return c.rdb.Set(ctx, key, val, ttl).Err()
}

// ConsumePasswordResetToken atomically deletes the stored reset token when its
// stored hash equals tokenHash. Returns true only for the single winning caller.
func (c *emailCache) ConsumePasswordResetToken(ctx context.Context, email, tokenHash string) (bool, error) {
	n, err := consumeResetTokenScript.Run(ctx, c.rdb, []string{passwordResetKey(email)}, tokenHash).Int()
	if err != nil {
		return false, err
	}
	return n == 1, nil
}

func (c *emailCache) DeletePasswordResetToken(ctx context.Context, email string) error {
	key := passwordResetKey(email)
	return c.rdb.Del(ctx, key).Err()
}

// Password reset email cooldown methods

func (c *emailCache) IsPasswordResetEmailInCooldown(ctx context.Context, email string) bool {
	key := passwordResetSentAtKey(email)
	exists, err := c.rdb.Exists(ctx, key).Result()
	return err == nil && exists > 0
}

func (c *emailCache) SetPasswordResetEmailCooldown(ctx context.Context, email string, ttl time.Duration) error {
	key := passwordResetSentAtKey(email)
	return c.rdb.Set(ctx, key, "1", ttl).Err()
}

// Notify email verification code methods

func (c *emailCache) GetNotifyVerifyCode(ctx context.Context, email string) (*service.VerificationCodeData, error) {
	return c.getCode(ctx, notifyVerifyKey(email))
}

func (c *emailCache) SetNotifyVerifyCode(ctx context.Context, email string, data *service.VerificationCodeData, ttl time.Duration) error {
	return c.setCode(ctx, notifyVerifyKey(email), data, ttl)
}

func (c *emailCache) IncrNotifyVerifyCodeAttempts(ctx context.Context, email string) (int, error) {
	return c.incrCodeAttempts(ctx, notifyVerifyKey(email))
}

func (c *emailCache) DeleteNotifyVerifyCode(ctx context.Context, email string) error {
	return c.deleteCode(ctx, notifyVerifyKey(email))
}

// User-level rate limiting for notify email verification codes

func notifyCodeUserRateKey(userID int64) string {
	return notifyCodeUserRateKeyPrefix + fmt.Sprintf("%d", userID)
}

func (c *emailCache) IncrNotifyCodeUserRate(ctx context.Context, userID int64, window time.Duration) (int64, error) {
	key := notifyCodeUserRateKey(userID)
	count, err := c.rdb.Incr(ctx, key).Result()
	if err != nil {
		return 0, err
	}
	// Always set TTL (idempotent) to avoid orphan keys if process crashes between INCR and EXPIRE.
	if err := c.rdb.Expire(ctx, key, window).Err(); err != nil {
		return count, fmt.Errorf("expire notify code rate key: %w", err)
	}
	return count, nil
}

func (c *emailCache) GetNotifyCodeUserRate(ctx context.Context, userID int64) (int64, error) {
	key := notifyCodeUserRateKey(userID)
	count, err := c.rdb.Get(ctx, key).Int64()
	if err != nil {
		return 0, err
	}
	return count, nil
}
