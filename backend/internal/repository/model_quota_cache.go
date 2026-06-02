package repository

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strconv"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/redis/go-redis/v9"
)

// Per-model USD quota cache.
//
// Layout: one Redis hash per (apiKeyID, pattern) — keyed by a short hash of the
// pattern so it's safe regardless of special characters. The hash fields mirror
// the global rate-limit hash (usage_5h/1d/7d + window_5h/1d/7d) so the Lua
// rotation logic is identical.

const (
	modelRateLimitKeyPrefix = "apikey:model_rate:"
	modelRateLimitCacheTTL  = 8 * 24 * time.Hour // > longest window (7d) with margin
)

func modelRateLimitKey(keyID int64, patternHash string) string {
	return fmt.Sprintf("%s%d:%s", modelRateLimitKeyPrefix, keyID, patternHash)
}

func modelRateLimitKeyScanPattern(keyID int64) string {
	return fmt.Sprintf("%s%d:*", modelRateLimitKeyPrefix, keyID)
}

// updateModelRateLimitUsageScript mirrors updateRateLimitUsageScript but does
// NOT require the hash to pre-exist — the per-model bucket is created on first
// write. Otherwise the rolling-window semantics are identical.
//
// ARGV: [1]=cost, [2]=ttl, [3]=now_unix, [4]=win5h_sec, [5]=win1d_sec, [6]=win7d_sec
var updateModelRateLimitUsageScript = redis.NewScript(`
	local cost = tonumber(ARGV[1])
	local now = tonumber(ARGV[3])
	local win5h = tonumber(ARGV[4])
	local win1d = tonumber(ARGV[5])
	local win7d = tonumber(ARGV[6])

	local function update_window(usage_field, window_field, window_duration)
		local w = tonumber(redis.call('HGET', KEYS[1], window_field) or 0)
		if w == 0 or (now - w) >= window_duration then
			redis.call('HSET', KEYS[1], usage_field, tostring(cost))
			redis.call('HSET', KEYS[1], window_field, tostring(now))
		else
			redis.call('HINCRBYFLOAT', KEYS[1], usage_field, cost)
		end
	end

	update_window('usage_5h', 'window_5h', win5h)
	update_window('usage_1d', 'window_1d', win1d)
	update_window('usage_7d', 'window_7d', win7d)
	redis.call('EXPIRE', KEYS[1], ARGV[2])
	return 1
`)

func (c *billingCache) GetModelQuotaUsage(ctx context.Context, keyID int64, patternHash string) (*service.ModelQuotaUsage, error) {
	if patternHash == "" {
		return nil, errors.New("model quota: empty pattern hash")
	}
	key := modelRateLimitKey(keyID, patternHash)
	result, err := c.rdb.HGetAll(ctx, key).Result()
	if err != nil {
		return nil, err
	}
	if len(result) == 0 {
		return nil, redis.Nil
	}
	out := &service.ModelQuotaUsage{}
	if v, ok := result[rateLimitFieldUsage5h]; ok {
		out.Usage5h, _ = strconv.ParseFloat(v, 64)
	}
	if v, ok := result[rateLimitFieldUsage1d]; ok {
		out.Usage1d, _ = strconv.ParseFloat(v, 64)
	}
	if v, ok := result[rateLimitFieldUsage7d]; ok {
		out.Usage7d, _ = strconv.ParseFloat(v, 64)
	}
	if v, ok := result[rateLimitFieldWindow5h]; ok {
		out.Window5h, _ = strconv.ParseInt(v, 10, 64)
	}
	if v, ok := result[rateLimitFieldWindow1d]; ok {
		out.Window1d, _ = strconv.ParseInt(v, 10, 64)
	}
	if v, ok := result[rateLimitFieldWindow7d]; ok {
		out.Window7d, _ = strconv.ParseInt(v, 10, 64)
	}
	return out, nil
}

func (c *billingCache) UpdateModelQuotaUsage(ctx context.Context, keyID int64, patternHash string, cost float64) error {
	if patternHash == "" {
		return errors.New("model quota: empty pattern hash")
	}
	if cost == 0 {
		return nil
	}
	key := modelRateLimitKey(keyID, patternHash)
	now := time.Now().Unix()
	_, err := updateModelRateLimitUsageScript.Run(ctx, c.rdb, []string{key},
		cost,
		int(modelRateLimitCacheTTL.Seconds()),
		now,
		int(rateLimitWindow5h.Seconds()),
		int(rateLimitWindow1d.Seconds()),
		int(rateLimitWindow7d.Seconds()),
	).Result()
	if err != nil && !errors.Is(err, redis.Nil) {
		log.Printf("Warning: update model quota cache failed for api key %d pattern %s: %v", keyID, patternHash, err)
		return err
	}
	return nil
}

// InvalidateModelQuotaUsage deletes every per-model bucket for the given key.
// Used when admin rewrites the key's per-model config, so stale buckets for
// removed patterns don't linger.
func (c *billingCache) InvalidateModelQuotaUsage(ctx context.Context, keyID int64) error {
	pattern := modelRateLimitKeyScanPattern(keyID)
	var (
		cursor uint64
		keys   []string
	)
	for {
		batch, next, err := c.rdb.Scan(ctx, cursor, pattern, 100).Result()
		if err != nil {
			return err
		}
		keys = append(keys, batch...)
		cursor = next
		if cursor == 0 {
			break
		}
	}
	if len(keys) == 0 {
		return nil
	}
	return c.rdb.Del(ctx, keys...).Err()
}
