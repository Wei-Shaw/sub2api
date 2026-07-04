package kirocooldown

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"math/rand"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
)

const (
	MinRequestInterval = time.Second
	MaxRequestInterval = 2 * time.Second

	CooldownReason429       = "rate_limit_exceeded"
	CooldownReasonSuspended = "account_suspended"

	ShortCooldown = time.Minute
	MaxCooldown   = 5 * time.Minute
	LongCooldown  = 24 * time.Hour

	redisTimeout = 3 * time.Second
	activeTTL    = 10 * time.Second
	stateTTL     = 25 * time.Hour
	keyPrefix    = "kiro:cooldown:"
)

var (
	ErrStoreUnavailable = errors.New("kiro cooldown store unavailable")

	reserveRequestScript = redis.NewScript(`
local t = redis.call('TIME')
local now_ms = tonumber(t[1]) * 1000 + math.floor(tonumber(t[2]) / 1000)
local last_request_ms = tonumber(redis.call('HGET', KEYS[1], 'last_request_ms') or '0')
local fail_count = tonumber(redis.call('HGET', KEYS[1], 'fail_count') or '0')
local cooldown_until_ms = tonumber(redis.call('HGET', KEYS[1], 'cooldown_until_ms') or '0')
local cooldown_reason = redis.call('HGET', KEYS[1], 'cooldown_reason') or ''
local interval_ms = tonumber(ARGV[1])
local active_ttl_ms = tonumber(ARGV[2])
local state_ttl_ms = tonumber(ARGV[3])

if cooldown_until_ms > now_ms then
  return {1, cooldown_until_ms - now_ms, cooldown_reason}
end

if cooldown_until_ms > 0 then
  redis.call('HDEL', KEYS[1], 'cooldown_until_ms', 'cooldown_reason')
end

local next_slot_ms = now_ms
if last_request_ms > 0 then
  local candidate_ms = last_request_ms + interval_ms
  if candidate_ms > now_ms then
    next_slot_ms = candidate_ms
  end
end

redis.call('HSET', KEYS[1], 'last_request_ms', next_slot_ms)
if fail_count > 0 or cooldown_until_ms > now_ms then
  redis.call('PEXPIRE', KEYS[1], state_ttl_ms)
else
  redis.call('PEXPIRE', KEYS[1], active_ttl_ms)
end
return {0, next_slot_ms - now_ms, ''}
`)

	mark429Script = redis.NewScript(`
local t = redis.call('TIME')
local now_ms = tonumber(t[1]) * 1000 + math.floor(tonumber(t[2]) / 1000)
local fail_count = tonumber(redis.call('HGET', KEYS[1], 'fail_count') or '0') + 1
local short_cooldown_ms = tonumber(ARGV[1])
local max_cooldown_ms = tonumber(ARGV[2])
local state_ttl_ms = tonumber(ARGV[3])
local cooldown_ms = short_cooldown_ms * (2 ^ (fail_count - 1))
if cooldown_ms > max_cooldown_ms then
  cooldown_ms = max_cooldown_ms
end
redis.call('HSET', KEYS[1],
  'fail_count', fail_count,
  'cooldown_until_ms', now_ms + cooldown_ms,
  'cooldown_reason', ARGV[4]
)
redis.call('PEXPIRE', KEYS[1], state_ttl_ms)
return cooldown_ms
`)

	markSuccessScript = redis.NewScript(`
redis.call('HSET', KEYS[1],
  'fail_count', 0,
  'cooldown_until_ms', 0,
  'cooldown_reason', ''
)
redis.call('PEXPIRE', KEYS[1], tonumber(ARGV[1]))
return 1
`)

	markSuspendedScript = redis.NewScript(`
local t = redis.call('TIME')
local now_ms = tonumber(t[1]) * 1000 + math.floor(tonumber(t[2]) / 1000)
local cooldown_ms = tonumber(ARGV[1])
local state_ttl_ms = tonumber(ARGV[2])
redis.call('HSET', KEYS[1],
  'fail_count', 0,
  'cooldown_until_ms', now_ms + cooldown_ms,
  'cooldown_reason', ARGV[3]
)
redis.call('PEXPIRE', KEYS[1], state_ttl_ms)
return cooldown_ms
`)
)

type Error struct {
	remaining time.Duration
	reason    string
}

type State struct {
	Active        bool
	Reason        string
	CooldownUntil time.Time
	Remaining     time.Duration
	FailCount     int
}

func NewError(remaining time.Duration, reason string) error {
	return &Error{remaining: remaining, reason: reason}
}

func (e *Error) Error() string {
	if e == nil {
		return ""
	}
	if e.reason == "" {
		return fmt.Sprintf("kiro token is in cooldown for %v", e.remaining.Round(time.Second))
	}
	return fmt.Sprintf("kiro token is in cooldown for %v (reason: %s)", e.remaining.Round(time.Second), e.reason)
}

func Calculate429Cooldown(retryCount int) time.Duration {
	if retryCount < 0 {
		retryCount = 0
	}
	cooldown := ShortCooldown * time.Duration(1<<retryCount)
	if cooldown > MaxCooldown {
		return MaxCooldown
	}
	return cooldown
}

type Store struct {
	client *redis.Client
	rngMu  sync.Mutex
	rng    *rand.Rand
}

func NewStore(client *redis.Client) *Store {
	return &Store{
		client: client,
		rng:    rand.New(rand.NewSource(time.Now().UnixNano())),
	}
}

func (s *Store) ReserveRequest(ctx context.Context, tokenKey string) (time.Duration, error) {
	if err := s.validate(); err != nil {
		return 0, err
	}
	cacheCtx, cancel := withRedisTimeout(ctx)
	defer cancel()

	values, err := reserveRequestScript.Run(
		cacheCtx,
		s.client,
		[]string{RedisKey(tokenKey)},
		s.nextInterval().Milliseconds(),
		activeTTL.Milliseconds(),
		stateTTL.Milliseconds(),
	).Result()
	if err != nil {
		return 0, fmt.Errorf("kiro cooldown reserve request: %w", err)
	}
	parts, ok := values.([]any)
	if !ok || len(parts) != 3 {
		return 0, fmt.Errorf("kiro cooldown reserve request: unexpected response %T", values)
	}
	state, err := luaInt64(parts[0])
	if err != nil {
		return 0, fmt.Errorf("kiro cooldown reserve request state: %w", err)
	}
	waitMS, err := luaInt64(parts[1])
	if err != nil {
		return 0, fmt.Errorf("kiro cooldown reserve request wait: %w", err)
	}
	reason, err := luaString(parts[2])
	if err != nil {
		return 0, fmt.Errorf("kiro cooldown reserve request reason: %w", err)
	}
	if state == 1 {
		return 0, NewError(time.Duration(waitMS)*time.Millisecond, reason)
	}
	if waitMS <= 0 {
		return 0, nil
	}
	return time.Duration(waitMS) * time.Millisecond, nil
}

func (s *Store) MarkSuccess(ctx context.Context, tokenKey string) error {
	if err := s.validate(); err != nil {
		return err
	}
	cacheCtx, cancel := withRedisTimeout(ctx)
	defer cancel()
	if err := markSuccessScript.Run(
		cacheCtx,
		s.client,
		[]string{RedisKey(tokenKey)},
		activeTTL.Milliseconds(),
	).Err(); err != nil {
		return fmt.Errorf("kiro cooldown mark success: %w", err)
	}
	return nil
}

func (s *Store) Mark429(ctx context.Context, tokenKey string) (time.Duration, error) {
	if err := s.validate(); err != nil {
		return 0, err
	}
	cacheCtx, cancel := withRedisTimeout(ctx)
	defer cancel()
	result, err := mark429Script.Run(
		cacheCtx,
		s.client,
		[]string{RedisKey(tokenKey)},
		ShortCooldown.Milliseconds(),
		MaxCooldown.Milliseconds(),
		stateTTL.Milliseconds(),
		CooldownReason429,
	).Result()
	if err != nil {
		return 0, fmt.Errorf("kiro cooldown mark 429: %w", err)
	}
	cooldownMS, err := luaInt64(result)
	if err != nil {
		return 0, fmt.Errorf("kiro cooldown mark 429: %w", err)
	}
	return time.Duration(cooldownMS) * time.Millisecond, nil
}

func (s *Store) MarkSuspended(ctx context.Context, tokenKey string) (time.Duration, error) {
	if err := s.validate(); err != nil {
		return 0, err
	}
	cacheCtx, cancel := withRedisTimeout(ctx)
	defer cancel()
	result, err := markSuspendedScript.Run(
		cacheCtx,
		s.client,
		[]string{RedisKey(tokenKey)},
		LongCooldown.Milliseconds(),
		stateTTL.Milliseconds(),
		CooldownReasonSuspended,
	).Result()
	if err != nil {
		return 0, fmt.Errorf("kiro cooldown mark suspended: %w", err)
	}
	cooldownMS, err := luaInt64(result)
	if err != nil {
		return 0, fmt.Errorf("kiro cooldown mark suspended: %w", err)
	}
	return time.Duration(cooldownMS) * time.Millisecond, nil
}

func (s *Store) GetState(ctx context.Context, tokenKey string) (*State, error) {
	if err := s.validate(); err != nil {
		return nil, err
	}
	cacheCtx, cancel := withRedisTimeout(ctx)
	defer cancel()

	values, err := s.client.HMGet(
		cacheCtx,
		RedisKey(tokenKey),
		"cooldown_until_ms",
		"cooldown_reason",
		"fail_count",
	).Result()
	if err != nil {
		return nil, fmt.Errorf("kiro cooldown get state: %w", err)
	}
	if len(values) != 3 {
		return nil, fmt.Errorf("kiro cooldown get state: unexpected response length %d", len(values))
	}

	cooldownUntilMS, err := luaInt64(values[0])
	if err != nil && values[0] != nil {
		return nil, fmt.Errorf("kiro cooldown get state cooldown_until_ms: %w", err)
	}
	reason, err := luaString(values[1])
	if err != nil {
		return nil, fmt.Errorf("kiro cooldown get state reason: %w", err)
	}
	failCount, err := luaInt64(values[2])
	if err != nil && values[2] != nil {
		return nil, fmt.Errorf("kiro cooldown get state fail_count: %w", err)
	}
	if cooldownUntilMS <= 0 {
		return nil, nil
	}

	cooldownUntil := time.UnixMilli(cooldownUntilMS)
	remaining := time.Until(cooldownUntil)
	if remaining <= 0 {
		return nil, nil
	}

	return &State{
		Active:        true,
		Reason:        reason,
		CooldownUntil: cooldownUntil,
		Remaining:     remaining,
		FailCount:     int(failCount),
	}, nil
}

func (s *Store) ClearEarliestTransientCooldown(ctx context.Context, tokenKeys []string) (bool, error) {
	if err := s.validate(); err != nil {
		return false, err
	}
	uniqueKeys := make([]string, 0, len(tokenKeys))
	seen := make(map[string]struct{}, len(tokenKeys))
	for _, tokenKey := range tokenKeys {
		tokenKey = strings.TrimSpace(tokenKey)
		if tokenKey == "" {
			continue
		}
		redisKey := RedisKey(tokenKey)
		if _, ok := seen[redisKey]; ok {
			continue
		}
		seen[redisKey] = struct{}{}
		uniqueKeys = append(uniqueKeys, redisKey)
	}
	if len(uniqueKeys) == 0 {
		return false, nil
	}

	cacheCtx, cancel := withRedisTimeout(ctx)
	defer cancel()

	type candidate struct {
		redisKey        string
		cooldownUntilMS int64
		failCount       int64
	}
	now := time.Now().UnixMilli()
	var best *candidate

	pipe := s.client.Pipeline()
	cmds := make([]*redis.SliceCmd, 0, len(uniqueKeys))
	for _, redisKey := range uniqueKeys {
		cmds = append(cmds, pipe.HMGet(cacheCtx, redisKey, "cooldown_until_ms", "cooldown_reason", "fail_count"))
	}
	if _, err := pipe.Exec(cacheCtx); err != nil {
		return false, fmt.Errorf("kiro cooldown clear transient scan: %w", err)
	}

	for i, cmd := range cmds {
		values, err := cmd.Result()
		if err != nil {
			return false, fmt.Errorf("kiro cooldown clear transient state: %w", err)
		}
		if len(values) != 3 {
			return false, fmt.Errorf("kiro cooldown clear transient state: unexpected response length %d", len(values))
		}
		cooldownUntilMS, err := luaInt64(values[0])
		if err != nil && values[0] != nil {
			return false, fmt.Errorf("kiro cooldown clear transient cooldown_until_ms: %w", err)
		}
		reason, err := luaString(values[1])
		if err != nil {
			return false, fmt.Errorf("kiro cooldown clear transient reason: %w", err)
		}
		failCount, err := luaInt64(values[2])
		if err != nil && values[2] != nil {
			return false, fmt.Errorf("kiro cooldown clear transient fail_count: %w", err)
		}
		if cooldownUntilMS <= now || reason != CooldownReason429 {
			continue
		}
		current := &candidate{redisKey: uniqueKeys[i], cooldownUntilMS: cooldownUntilMS, failCount: failCount}
		if best == nil ||
			current.cooldownUntilMS < best.cooldownUntilMS ||
			(current.cooldownUntilMS == best.cooldownUntilMS && current.failCount < best.failCount) {
			best = current
		}
	}
	if best == nil {
		return false, nil
	}

	if err := s.client.HDel(cacheCtx, best.redisKey, "cooldown_until_ms", "cooldown_reason").Err(); err != nil {
		return false, fmt.Errorf("kiro cooldown clear transient: %w", err)
	}
	if err := s.client.Expire(cacheCtx, best.redisKey, activeTTL).Err(); err != nil {
		return false, fmt.Errorf("kiro cooldown clear transient ttl: %w", err)
	}
	return true, nil
}

func RedisKey(tokenKey string) string {
	sum := sha256.Sum256([]byte(strings.TrimSpace(tokenKey)))
	digest := hex.EncodeToString(sum[:])
	return keyPrefix + "{" + digest + "}"
}

func ActiveTTL() time.Duration {
	return activeTTL
}

func StateTTL() time.Duration {
	return stateTTL
}

func (s *Store) validate() error {
	if s == nil || s.client == nil {
		return ErrStoreUnavailable
	}
	return nil
}

func (s *Store) nextInterval() time.Duration {
	s.rngMu.Lock()
	defer s.rngMu.Unlock()
	if MaxRequestInterval <= MinRequestInterval {
		return MinRequestInterval
	}
	return MinRequestInterval + time.Duration(s.rng.Int63n(int64(MaxRequestInterval-MinRequestInterval)))
}

func withRedisTimeout(ctx context.Context) (context.Context, context.CancelFunc) {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithTimeout(ctx, redisTimeout)
}

func luaInt64(v any) (int64, error) {
	switch n := v.(type) {
	case int64:
		return n, nil
	case int:
		return int64(n), nil
	case string:
		return strconv.ParseInt(strings.TrimSpace(n), 10, 64)
	case []byte:
		return strconv.ParseInt(strings.TrimSpace(string(n)), 10, 64)
	default:
		return 0, fmt.Errorf("unsupported lua numeric type %T", v)
	}
}

func luaString(v any) (string, error) {
	switch s := v.(type) {
	case string:
		return s, nil
	case []byte:
		return string(s), nil
	case nil:
		return "", nil
	default:
		return "", fmt.Errorf("unsupported lua string type %T", v)
	}
}
