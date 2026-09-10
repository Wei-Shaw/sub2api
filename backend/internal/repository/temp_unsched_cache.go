package repository

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/redis/go-redis/v9"
)

const (
	tempUnschedPrefix                = "temp_unsched:account:"
	openAIAPIKeyHealthFailurePrefix  = "openai_apikey_health:"
	openAIOAuthCapacityFailurePrefix = "openai_oauth_capacity:"
	openAIOAuthCapacityStateTTL      = 7 * 24 * time.Hour
)

var openAIOAuthCapacityOutcomeScript = redis.NewScript(`
	local outcomes_key = KEYS[1]
	local order_key = KEYS[2]
	local expiry_key = KEYS[3]
	local pending_key = KEYS[4]
	local acked_key = KEYS[5]
	local sequence = tonumber(ARGV[1])
	local outcome = ARGV[2]
	local window_ms = tonumber(ARGV[3])
	local threshold = tonumber(ARGV[4])
	local ttl = tonumber(ARGV[5])
	local now = redis.call('TIME')
	local now_ms = (tonumber(now[1]) * 1000) + math.floor(tonumber(now[2]) / 1000)

	local stale = redis.call('ZRANGEBYSCORE', expiry_key, '-inf', now_ms - window_ms)
	for _, stale_sequence in ipairs(stale) do
		redis.call('HDEL', outcomes_key, stale_sequence)
		redis.call('ZREM', order_key, stale_sequence)
	end
	redis.call('ZREMRANGEBYSCORE', expiry_key, '-inf', now_ms - window_ms)

	local acked = tonumber(redis.call('GET', acked_key) or '0')
	if sequence > acked and redis.call('HEXISTS', outcomes_key, tostring(sequence)) == 0 then
		redis.call('HSET', outcomes_key, tostring(sequence), outcome)
		redis.call('ZADD', order_key, sequence, tostring(sequence))
		redis.call('ZADD', expiry_key, now_ms, tostring(sequence))
	end

	redis.call('EXPIRE', outcomes_key, ttl)
	redis.call('EXPIRE', order_key, ttl)
	redis.call('EXPIRE', expiry_key, ttl)
	redis.call('EXPIRE', acked_key, ttl)

	local count = 0
	local recent = redis.call('ZREVRANGE', order_key, 0, threshold - 1)
	local expected = nil
	for _, current_sequence in ipairs(recent) do
		local current = tonumber(current_sequence)
		if expected and current ~= expected then break end
		if redis.call('HGET', outcomes_key, current_sequence) ~= 'f' then break end
		count = count + 1
		expected = current - 1
	end

	local pending = tonumber(redis.call('GET', pending_key) or '0')
	if pending > 0 then
		redis.call('EXPIRE', pending_key, ttl)
		return {count, pending, 1}
	end
	if count >= threshold then
		local trip_sequence = tonumber(recent[1])
		redis.call('SET', pending_key, trip_sequence, 'EX', ttl)
		return {count, trip_sequence, 1}
	end
	return {count, 0, 0}
`)

var openAIOAuthCapacityPrepareScript = redis.NewScript(`
	local marker_key = KEYS[1]
	local trip_sequence = tonumber(ARGV[1])
	local until_nanos = tonumber(ARGV[2])
	local expires_at = tonumber(ARGV[3])
	local existing_sequence = tonumber(redis.call('HGET', marker_key, 'trip_sequence') or '0')
	local existing_until = tonumber(redis.call('HGET', marker_key, 'until_unix_nano') or '0')
	if existing_sequence > trip_sequence or (existing_sequence == trip_sequence and existing_until > until_nanos) then
		return 0
	end
	redis.call('HSET', marker_key, 'trip_sequence', trip_sequence, 'until_unix_nano', ARGV[2], 'state', 'pending')
	redis.call('EXPIREAT', marker_key, expires_at)
	return 1
`)

var openAIOAuthCapacityAcknowledgeScript = redis.NewScript(`
	local outcomes_key = KEYS[1]
	local order_key = KEYS[2]
	local expiry_key = KEYS[3]
	local pending_key = KEYS[4]
	local acked_key = KEYS[5]
	local marker_key = KEYS[6]
	local trip_sequence = tonumber(ARGV[1])
	local ttl = tonumber(ARGV[2])
	local completed = redis.call('ZRANGEBYSCORE', order_key, '-inf', trip_sequence)
	for _, sequence in ipairs(completed) do
		redis.call('HDEL', outcomes_key, sequence)
		redis.call('ZREM', expiry_key, sequence)
	end
	redis.call('ZREMRANGEBYSCORE', order_key, '-inf', trip_sequence)
	local acked = tonumber(redis.call('GET', acked_key) or '0')
	if trip_sequence > acked then redis.call('SET', acked_key, trip_sequence, 'EX', ttl) end
	local pending = tonumber(redis.call('GET', pending_key) or '0')
	if pending > 0 and pending <= trip_sequence then redis.call('DEL', pending_key) end
	if tonumber(redis.call('HGET', marker_key, 'trip_sequence') or '0') == trip_sequence then
		redis.call('HSET', marker_key, 'state', 'published')
	end
	return 1
`)

var rollingFailureWindowScript = redis.NewScript(`
	local key = KEYS[1]
	local sequence_key = key .. ':sequence'
	local now = redis.call('TIME')
	local now_ms = (tonumber(now[1]) * 1000) + math.floor(tonumber(now[2]) / 1000)
	local window_ms = tonumber(ARGV[1]) * 60 * 1000
	local threshold = tonumber(ARGV[2])
	local sequence = redis.call('INCR', sequence_key)

	redis.call('ZREMRANGEBYSCORE', key, '-inf', now_ms - window_ms)
	redis.call('ZADD', key, now_ms, tostring(now_ms) .. ':' .. tostring(sequence))
	local count = redis.call('ZCARD', key)
	local ttl = math.max(60, (tonumber(ARGV[1]) + 1) * 60)
	redis.call('EXPIRE', key, ttl)
	redis.call('EXPIRE', sequence_key, ttl)
	if count >= threshold then
		redis.call('DEL', key, sequence_key)
		return {count, 1}
	end
	return {count, 0}
`)

var tempUnschedSetScript = redis.NewScript(`
	local key = KEYS[1]
	local new_until = tonumber(ARGV[1])
	local new_value = ARGV[2]
	local new_ttl = tonumber(ARGV[3])

	local existing = redis.call('GET', key)
	if existing then
		local ok, existing_data = pcall(cjson.decode, existing)
		if ok and existing_data and existing_data.until_unix then
			local existing_until = tonumber(existing_data.until_unix)
			if existing_until and new_until <= existing_until then
				return 0
			end
		end
	end

	redis.call('SET', key, new_value, 'EX', new_ttl)
	return 1
`)

type tempUnschedCache struct {
	rdb *redis.Client
}

func NewTempUnschedCache(rdb *redis.Client) service.TempUnschedCache {
	return &tempUnschedCache{rdb: rdb}
}

// SetTempUnsched 设置临时不可调度状态（只延长不缩短）
func (c *tempUnschedCache) SetTempUnsched(ctx context.Context, accountID int64, state *service.TempUnschedState) error {
	key := fmt.Sprintf("%s%d", tempUnschedPrefix, accountID)

	stateJSON, err := json.Marshal(state)
	if err != nil {
		return fmt.Errorf("marshal state: %w", err)
	}

	ttl := time.Until(time.Unix(state.UntilUnix, 0))
	if ttl <= 0 {
		return nil // 已过期，不设置
	}

	ttlSeconds := int(ttl.Seconds())
	if ttlSeconds < 1 {
		ttlSeconds = 1
	}

	_, err = tempUnschedSetScript.Run(ctx, c.rdb, []string{key}, state.UntilUnix, string(stateJSON), ttlSeconds).Result()
	return err
}

// GetTempUnsched 获取临时不可调度状态
func (c *tempUnschedCache) GetTempUnsched(ctx context.Context, accountID int64) (*service.TempUnschedState, error) {
	key := fmt.Sprintf("%s%d", tempUnschedPrefix, accountID)

	val, err := c.rdb.Get(ctx, key).Result()
	if err == redis.Nil {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	var state service.TempUnschedState
	if err := json.Unmarshal([]byte(val), &state); err != nil {
		return nil, fmt.Errorf("unmarshal state: %w", err)
	}

	return &state, nil
}

// DeleteTempUnsched 删除临时不可调度状态
func (c *tempUnschedCache) DeleteTempUnsched(ctx context.Context, accountID int64) error {
	key := fmt.Sprintf("%s%d", tempUnschedPrefix, accountID)
	return c.rdb.Del(ctx, key).Err()
}

func rollingFailureWindowKey(prefix string, accountID int64) string {
	// The hash tag keeps the rolling window and sequence key in one Redis
	// Cluster slot even though the Lua script derives the latter dynamically.
	return fmt.Sprintf("%s{%d}:failures", prefix, accountID)
}

func (c *tempUnschedCache) recordRollingFailure(ctx context.Context, key string, windowMinutes, threshold int, operation string) (int64, bool, error) {
	if windowMinutes < 1 {
		windowMinutes = 1
	}
	if threshold < 1 {
		threshold = 1
	}
	result, err := rollingFailureWindowScript.Run(ctx, c.rdb, []string{key}, windowMinutes, threshold).Slice()
	if err != nil {
		return 0, false, fmt.Errorf("%s: %w", operation, err)
	}
	if len(result) != 2 {
		return 0, false, fmt.Errorf("%s: unexpected result length %d", operation, len(result))
	}
	count, countOK := result[0].(int64)
	tripped, trippedOK := result[1].(int64)
	if !countOK || !trippedOK {
		return 0, false, fmt.Errorf("%s: unexpected result types %T/%T", operation, result[0], result[1])
	}
	return count, tripped == 1, nil
}

func (c *tempUnschedCache) RecordOpenAIAPIKeyHealthFailure(ctx context.Context, accountID int64, windowMinutes, threshold int) (int64, bool, error) {
	return c.recordRollingFailure(
		ctx,
		rollingFailureWindowKey(openAIAPIKeyHealthFailurePrefix, accountID),
		windowMinutes,
		threshold,
		"record OpenAI API key health failure",
	)
}

func openAIOAuthCapacityKey(accountID int64) string {
	return fmt.Sprintf("%s{%d}", openAIOAuthCapacityFailurePrefix, accountID)
}

func (c *tempUnschedCache) BeginOpenAIOAuthCapacityAttempt(ctx context.Context, accountID int64) (int64, error) {
	key := openAIOAuthCapacityKey(accountID) + ":sequence"
	sequence, err := c.rdb.Incr(ctx, key).Result()
	if err != nil {
		return 0, fmt.Errorf("begin OpenAI OAuth capacity attempt: %w", err)
	}
	if err := c.rdb.Expire(ctx, key, openAIOAuthCapacityStateTTL).Err(); err != nil {
		return 0, fmt.Errorf("expire OpenAI OAuth capacity sequence: %w", err)
	}
	return sequence, nil
}

func (c *tempUnschedCache) RecordOpenAIOAuthCapacityOutcome(ctx context.Context, accountID, sequence int64, failed bool, windowMinutes, threshold, cooldownMinutes int) (int64, int64, bool, error) {
	if sequence <= 0 {
		return 0, 0, false, fmt.Errorf("record OpenAI OAuth capacity outcome: invalid sequence %d", sequence)
	}
	if windowMinutes < 1 {
		windowMinutes = 1
	}
	if threshold < 1 {
		threshold = 1
	}
	if cooldownMinutes < 1 {
		cooldownMinutes = 1
	}
	base := openAIOAuthCapacityKey(accountID)
	outcome := "s"
	if failed {
		outcome = "f"
	}
	ttl := time.Duration(windowMinutes+cooldownMinutes+1) * time.Minute
	if ttl > openAIOAuthCapacityStateTTL {
		ttl = openAIOAuthCapacityStateTTL
	}
	result, err := openAIOAuthCapacityOutcomeScript.Run(ctx, c.rdb, []string{base + ":outcomes", base + ":order", base + ":expiry", base + ":pending", base + ":acked"}, sequence, outcome, int64(time.Duration(windowMinutes)*time.Minute/time.Millisecond), threshold, int64(ttl/time.Second)).Slice()
	if err != nil {
		return 0, 0, false, fmt.Errorf("record OpenAI OAuth capacity outcome: %w", err)
	}
	if len(result) != 3 {
		return 0, 0, false, fmt.Errorf("record OpenAI OAuth capacity outcome: unexpected result length %d", len(result))
	}
	count, countOK := result[0].(int64)
	tripSequence, tripOK := result[1].(int64)
	tripped, trippedOK := result[2].(int64)
	if !countOK || !tripOK || !trippedOK {
		return 0, 0, false, fmt.Errorf("record OpenAI OAuth capacity outcome: unexpected result types %T/%T/%T", result[0], result[1], result[2])
	}
	return count, tripSequence, tripped == 1, nil
}

func (c *tempUnschedCache) PrepareOpenAIOAuthCapacityCooldown(ctx context.Context, accountID, tripSequence int64, until time.Time) error {
	if tripSequence <= 0 || until.IsZero() {
		return fmt.Errorf("prepare OpenAI OAuth capacity cooldown: invalid transition")
	}
	base := openAIOAuthCapacityKey(accountID)
	expiresAt := until.Add(time.Minute).Unix()
	_, err := openAIOAuthCapacityPrepareScript.Run(ctx, c.rdb, []string{base + ":cooldown"}, tripSequence, until.UnixNano(), expiresAt).Result()
	if err != nil {
		return fmt.Errorf("prepare OpenAI OAuth capacity cooldown: %w", err)
	}
	return nil
}

func (c *tempUnschedCache) AcknowledgeOpenAIOAuthCapacityCooldown(ctx context.Context, accountID, tripSequence int64) error {
	base := openAIOAuthCapacityKey(accountID)
	_, err := openAIOAuthCapacityAcknowledgeScript.Run(ctx, c.rdb, []string{base + ":outcomes", base + ":order", base + ":expiry", base + ":pending", base + ":acked", base + ":cooldown"}, tripSequence, int64(openAIOAuthCapacityStateTTL/time.Second)).Result()
	if err != nil {
		return fmt.Errorf("acknowledge OpenAI OAuth capacity cooldown: %w", err)
	}
	return nil
}

func (c *tempUnschedCache) GetOpenAIOAuthCapacityCooldown(ctx context.Context, accountID int64) (int64, time.Time, bool, error) {
	values, err := c.rdb.HMGet(ctx, openAIOAuthCapacityKey(accountID)+":cooldown", "trip_sequence", "until_unix_nano").Result()
	if err != nil {
		return 0, time.Time{}, false, fmt.Errorf("get OpenAI OAuth capacity cooldown: %w", err)
	}
	if len(values) != 2 || values[0] == nil || values[1] == nil {
		return 0, time.Time{}, false, nil
	}
	tripSequence, err := strconv.ParseInt(fmt.Sprint(values[0]), 10, 64)
	if err != nil {
		return 0, time.Time{}, false, fmt.Errorf("parse OpenAI OAuth capacity trip sequence: %w", err)
	}
	untilNanos, err := strconv.ParseInt(fmt.Sprint(values[1]), 10, 64)
	if err != nil {
		return 0, time.Time{}, false, fmt.Errorf("parse OpenAI OAuth capacity deadline: %w", err)
	}
	until := time.Unix(0, untilNanos)
	if !time.Now().Before(until) {
		return 0, time.Time{}, false, nil
	}
	return tripSequence, until, true, nil
}
