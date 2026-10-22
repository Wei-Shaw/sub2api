package repository

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"

	"github.com/redis/go-redis/v9"
)

const modelRouteKeyPrefix = "model_route:"

// acquireModelRouteScript atomically enforces per-account and rule-total caps.
// KEYS[1]=accountKey KEYS[2]=totalKey
// ARGV: accountMax, totalMax, ttlSeconds, requestID (0 max = unlimited for that dimension)
var acquireModelRouteScript = redis.NewScript(`
	redis.replicate_commands()
	local accountKey = KEYS[1]
	local totalKey = KEYS[2]
	local accountMax = tonumber(ARGV[1])
	local totalMax = tonumber(ARGV[2])
	local ttl = tonumber(ARGV[3])
	local requestID = ARGV[4]
	local now = tonumber(redis.call('TIME')[1])
	local expireBefore = now - ttl

	redis.call('ZREMRANGEBYSCORE', accountKey, '-inf', expireBefore)
	redis.call('ZREMRANGEBYSCORE', totalKey, '-inf', expireBefore)

	if redis.call('ZSCORE', accountKey, requestID) ~= false or redis.call('ZSCORE', totalKey, requestID) ~= false then
		redis.call('ZADD', accountKey, now, requestID)
		redis.call('ZADD', totalKey, now, requestID)
		redis.call('EXPIRE', accountKey, ttl)
		redis.call('EXPIRE', totalKey, ttl)
		return 1
	end

	if accountMax > 0 and redis.call('ZCARD', accountKey) >= accountMax then
		return 0
	end
	if totalMax > 0 and redis.call('ZCARD', totalKey) >= totalMax then
		return 0
	end

	redis.call('ZADD', accountKey, now, requestID)
	redis.call('ZADD', totalKey, now, requestID)
	redis.call('EXPIRE', accountKey, ttl)
	redis.call('EXPIRE', totalKey, ttl)
	return 1
`)

var getModelRouteCountScript = redis.NewScript(`
	redis.replicate_commands()
	local key = KEYS[1]
	local ttl = tonumber(ARGV[1])
	local now = tonumber(redis.call('TIME')[1])
	redis.call('ZREMRANGEBYSCORE', key, '-inf', now - ttl)
	return redis.call('ZCARD', key)
`)

func modelRoutePatternToken(pattern string) string {
	sum := sha256.Sum256([]byte(pattern))
	return hex.EncodeToString(sum[:8])
}

func modelRouteAccountKey(groupID int64, pattern string, accountID int64) string {
	return fmt.Sprintf("%sg%d:p%s:a%d", modelRouteKeyPrefix, groupID, modelRoutePatternToken(pattern), accountID)
}

func modelRouteTotalKey(groupID int64, pattern string) string {
	return fmt.Sprintf("%sg%d:p%s:total", modelRouteKeyPrefix, groupID, modelRoutePatternToken(pattern))
}

// AcquireModelRouteSlot reserves a model-routing concurrency slot for an account under a rule.
func (c *concurrencyCache) AcquireModelRouteSlot(
	ctx context.Context,
	groupID int64,
	pattern string,
	accountID int64,
	accountMax int,
	totalMax int,
	requestID string,
) (bool, error) {
	if accountMax <= 0 && totalMax <= 0 {
		return true, nil
	}
	result, err := acquireModelRouteScript.Run(
		ctx,
		c.rdb,
		[]string{modelRouteAccountKey(groupID, pattern, accountID), modelRouteTotalKey(groupID, pattern)},
		accountMax,
		totalMax,
		c.slotTTLSeconds,
		requestID,
	).Int()
	if err != nil {
		return false, err
	}
	return result == 1, nil
}

// ReleaseModelRouteSlot releases a previously acquired model-routing slot.
func (c *concurrencyCache) ReleaseModelRouteSlot(
	ctx context.Context,
	groupID int64,
	pattern string,
	accountID int64,
	requestID string,
) error {
	pipe := c.rdb.Pipeline()
	pipe.ZRem(ctx, modelRouteAccountKey(groupID, pattern, accountID), requestID)
	pipe.ZRem(ctx, modelRouteTotalKey(groupID, pattern), requestID)
	_, err := pipe.Exec(ctx)
	return err
}

// GetModelRouteAccountConcurrency returns current in-flight count for one preferred account.
func (c *concurrencyCache) GetModelRouteAccountConcurrency(
	ctx context.Context,
	groupID int64,
	pattern string,
	accountID int64,
) (int, error) {
	return getModelRouteCountScript.Run(
		ctx, c.rdb,
		[]string{modelRouteAccountKey(groupID, pattern, accountID)},
		c.slotTTLSeconds,
	).Int()
}

// GetModelRouteTotalConcurrency returns current in-flight count for the whole rule pool.
func (c *concurrencyCache) GetModelRouteTotalConcurrency(
	ctx context.Context,
	groupID int64,
	pattern string,
) (int, error) {
	return getModelRouteCountScript.Run(
		ctx, c.rdb,
		[]string{modelRouteTotalKey(groupID, pattern)},
		c.slotTTLSeconds,
	).Int()
}
