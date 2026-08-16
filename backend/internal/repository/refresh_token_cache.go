package repository

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/redis/go-redis/v9"
)

const (
	refreshTokenKeyPrefix         = "refresh_token:"
	userRefreshTokensPrefix       = "user_refresh_tokens:"
	tokenFamilyPrefix             = "token_family:"
	consumedRefreshTokenKeyPrefix = "consumed_refresh_token:"
	revokedTokenFamilyKeyPrefix   = "revoked_token_family:"
	refreshTombstoneFallbackTTL   = 5 * time.Minute
)

var storeRefreshTokenScript = redis.NewScript(`
if redis.call("EXISTS", KEYS[4]) == 1 then
  return 0
end

local ttl = tonumber(ARGV[2])
if not ttl or ttl < 1 then
  return redis.error_reply("invalid refresh token ttl")
end

redis.call("PSETEX", KEYS[1], ttl, ARGV[1])
redis.call("SADD", KEYS[2], ARGV[3])
redis.call("SADD", KEYS[3], ARGV[3])

local user_ttl = redis.call("PTTL", KEYS[2])
if user_ttl < ttl then
  redis.call("PEXPIRE", KEYS[2], ttl)
end
local family_ttl = redis.call("PTTL", KEYS[3])
if family_ttl < ttl then
  redis.call("PEXPIRE", KEYS[3], ttl)
end
return 1
`)

var consumeRefreshTokenScript = redis.NewScript(`
local raw = redis.call("GET", KEYS[1])
if raw then
  local ttl = redis.call("PTTL", KEYS[1])
  local fallback_ttl = tonumber(ARGV[6])
  if ttl < fallback_ttl then
    ttl = fallback_ttl
  end
  redis.call("DEL", KEYS[1])
  redis.call("PSETEX", KEYS[2], ttl, raw)

  local ok, data = pcall(cjson.decode, raw)
  if ok then
    if data.user_id then
      redis.call("SREM", ARGV[3] .. tostring(data.user_id), ARGV[1])
    end
    if data.family_id then
      redis.call("SREM", ARGV[4] .. tostring(data.family_id), ARGV[1])
    end
  end
  return {1, raw}
end

raw = redis.call("GET", KEYS[2])
if not raw then
  return {0}
end

local ttl = redis.call("PTTL", KEYS[2])
local fallback_ttl = tonumber(ARGV[6])
if ttl < fallback_ttl then
  ttl = fallback_ttl
end
local ok, data = pcall(cjson.decode, raw)
if not ok then
  return {3, raw}
end

local family_id = data.family_id and tostring(data.family_id) or ""
local user_id = data.user_id and tostring(data.user_id) or ""
if family_id ~= "" then
  local family_key = ARGV[4] .. family_id
  redis.call("PSETEX", ARGV[5] .. family_id, ttl, "1")
  local hashes = redis.call("SMEMBERS", family_key)
  for _, hash in ipairs(hashes) do
    redis.call("DEL", ARGV[2] .. hash)
    if user_id ~= "" then
      redis.call("SREM", ARGV[3] .. user_id, hash)
    end
  end
  redis.call("DEL", family_key)
end
return {2, raw}
`)

var revokeTokenFamilyScript = redis.NewScript(`
local hashes = redis.call("SMEMBERS", KEYS[1])
local ttl = tonumber(ARGV[3])
for _, hash in ipairs(hashes) do
  local token_key = ARGV[1] .. hash
  local token_ttl = redis.call("PTTL", token_key)
  if token_ttl > ttl then
    ttl = token_ttl
  end
  local raw = redis.call("GET", token_key)
  if raw then
    local ok, data = pcall(cjson.decode, raw)
    if ok and data.user_id then
      redis.call("SREM", ARGV[2] .. tostring(data.user_id), hash)
    end
  end
  redis.call("DEL", token_key)
end
redis.call("DEL", KEYS[1])
redis.call("PSETEX", KEYS[2], ttl, "1")
return #hashes
`)

// refreshTokenKey generates the Redis key for a refresh token.
func refreshTokenKey(tokenHash string) string {
	return refreshTokenKeyPrefix + tokenHash
}

// userRefreshTokensKey generates the Redis key for user's token set.
func userRefreshTokensKey(userID int64) string {
	return fmt.Sprintf("%s%d", userRefreshTokensPrefix, userID)
}

// tokenFamilyKey generates the Redis key for token family set.
func tokenFamilyKey(familyID string) string {
	return tokenFamilyPrefix + familyID
}

func consumedRefreshTokenKey(tokenHash string) string {
	return consumedRefreshTokenKeyPrefix + tokenHash
}

func revokedTokenFamilyKey(familyID string) string {
	return revokedTokenFamilyKeyPrefix + familyID
}

type refreshTokenCache struct {
	rdb *redis.Client
}

var _ service.RefreshTokenReplayAcknowledger = (*refreshTokenCache)(nil)

// NewRefreshTokenCache creates a new RefreshTokenCache implementation.
func NewRefreshTokenCache(rdb *redis.Client) service.RefreshTokenCache {
	return &refreshTokenCache{rdb: rdb}
}

func (c *refreshTokenCache) StoreRefreshToken(ctx context.Context, tokenHash string, data *service.RefreshTokenData, ttl time.Duration) error {
	if data == nil || data.UserID <= 0 || data.FamilyID == "" || ttl <= 0 {
		return fmt.Errorf("invalid refresh token data")
	}
	key := refreshTokenKey(tokenHash)
	val, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("marshal refresh token data: %w", err)
	}
	stored, err := storeRefreshTokenScript.Run(ctx, c.rdb, []string{
		key,
		userRefreshTokensKey(data.UserID),
		tokenFamilyKey(data.FamilyID),
		revokedTokenFamilyKey(data.FamilyID),
	}, val, ttl.Milliseconds(), tokenHash).Int()
	if err != nil {
		return fmt.Errorf("store refresh token atomically: %w", err)
	}
	if stored != 1 {
		return service.ErrRefreshTokenReused
	}
	return nil
}

func (c *refreshTokenCache) GetRefreshToken(ctx context.Context, tokenHash string) (*service.RefreshTokenData, error) {
	key := refreshTokenKey(tokenHash)
	val, err := c.rdb.Get(ctx, key).Result()
	if err != nil {
		if err == redis.Nil {
			return nil, service.ErrRefreshTokenNotFound
		}
		return nil, err
	}
	var data service.RefreshTokenData
	if err := json.Unmarshal([]byte(val), &data); err != nil {
		return nil, fmt.Errorf("unmarshal refresh token data: %w", err)
	}
	return &data, nil
}

func (c *refreshTokenCache) ConsumeRefreshToken(ctx context.Context, tokenHash string) (*service.RefreshTokenData, error) {
	result, err := consumeRefreshTokenScript.Run(ctx, c.rdb, []string{
		refreshTokenKey(tokenHash),
		consumedRefreshTokenKey(tokenHash),
	},
		tokenHash,
		refreshTokenKeyPrefix,
		userRefreshTokensPrefix,
		tokenFamilyPrefix,
		revokedTokenFamilyKeyPrefix,
		refreshTombstoneFallbackTTL.Milliseconds(),
	).Slice()
	if err != nil {
		return nil, fmt.Errorf("consume refresh token atomically: %w", err)
	}
	if len(result) == 0 {
		return nil, fmt.Errorf("consume refresh token returned no status")
	}
	status, ok := result[0].(int64)
	if !ok {
		return nil, fmt.Errorf("consume refresh token returned invalid status %T", result[0])
	}
	if status == 0 {
		return nil, service.ErrRefreshTokenNotFound
	}
	if len(result) < 2 {
		return nil, fmt.Errorf("consume refresh token returned no payload")
	}
	raw, ok := result[1].(string)
	if !ok {
		return nil, fmt.Errorf("consume refresh token returned invalid payload %T", result[1])
	}
	var data service.RefreshTokenData
	if err := json.Unmarshal([]byte(raw), &data); err != nil {
		return nil, fmt.Errorf("unmarshal consumed refresh token data: %w", err)
	}
	switch status {
	case 1:
		return &data, nil
	case 2:
		return &data, service.ErrRefreshTokenReused
	case 3:
		return nil, fmt.Errorf("consumed refresh token tombstone is malformed")
	default:
		return nil, fmt.Errorf("consume refresh token returned unknown status %d", status)
	}
}

func (c *refreshTokenCache) AcknowledgeRefreshTokenReplay(ctx context.Context, tokenHash string) error {
	if err := c.rdb.Del(ctx, consumedRefreshTokenKey(tokenHash)).Err(); err != nil {
		return fmt.Errorf("acknowledge refresh token replay: %w", err)
	}
	return nil
}

func (c *refreshTokenCache) DeleteRefreshToken(ctx context.Context, tokenHash string) error {
	key := refreshTokenKey(tokenHash)
	return c.rdb.Del(ctx, key).Err()
}

func (c *refreshTokenCache) DeleteUserRefreshTokens(ctx context.Context, userID int64) error {
	// Get all token hashes for this user
	tokenHashes, err := c.GetUserTokenHashes(ctx, userID)
	if err != nil && err != redis.Nil {
		return fmt.Errorf("get user token hashes: %w", err)
	}

	if len(tokenHashes) == 0 {
		return nil
	}

	// Build keys to delete
	keys := make([]string, 0, len(tokenHashes)+1)
	for _, hash := range tokenHashes {
		keys = append(keys, refreshTokenKey(hash))
	}
	keys = append(keys, userRefreshTokensKey(userID))

	// Delete all keys in a pipeline
	pipe := c.rdb.Pipeline()
	for _, key := range keys {
		pipe.Del(ctx, key)
	}
	_, err = pipe.Exec(ctx)
	return err
}

func (c *refreshTokenCache) DeleteTokenFamily(ctx context.Context, familyID string) error {
	if familyID == "" {
		return nil
	}
	_, err := revokeTokenFamilyScript.Run(ctx, c.rdb, []string{
		tokenFamilyKey(familyID),
		revokedTokenFamilyKey(familyID),
	}, refreshTokenKeyPrefix, userRefreshTokensPrefix, refreshTombstoneFallbackTTL.Milliseconds()).Result()
	if err != nil {
		return fmt.Errorf("revoke refresh token family atomically: %w", err)
	}
	return nil
}

func (c *refreshTokenCache) AddToUserTokenSet(ctx context.Context, userID int64, tokenHash string, ttl time.Duration) error {
	key := userRefreshTokensKey(userID)
	pipe := c.rdb.Pipeline()
	pipe.SAdd(ctx, key, tokenHash)
	pipe.Expire(ctx, key, ttl)
	_, err := pipe.Exec(ctx)
	return err
}

func (c *refreshTokenCache) AddToFamilyTokenSet(ctx context.Context, familyID string, tokenHash string, ttl time.Duration) error {
	key := tokenFamilyKey(familyID)
	pipe := c.rdb.Pipeline()
	pipe.SAdd(ctx, key, tokenHash)
	pipe.Expire(ctx, key, ttl)
	_, err := pipe.Exec(ctx)
	return err
}

func (c *refreshTokenCache) GetUserTokenHashes(ctx context.Context, userID int64) ([]string, error) {
	key := userRefreshTokensKey(userID)
	return c.rdb.SMembers(ctx, key).Result()
}

func (c *refreshTokenCache) GetFamilyTokenHashes(ctx context.Context, familyID string) ([]string, error) {
	key := tokenFamilyKey(familyID)
	return c.rdb.SMembers(ctx, key).Result()
}

func (c *refreshTokenCache) IsTokenInFamily(ctx context.Context, familyID string, tokenHash string) (bool, error) {
	key := tokenFamilyKey(familyID)
	return c.rdb.SIsMember(ctx, key, tokenHash).Result()
}
