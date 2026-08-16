package repository

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/redis/go-redis/v9"
)

const billingRiskRedisKeyPrefix = "billing:risk:"

type billingRiskKeys struct {
	leases  string
	costs   string
	meta    string
	balance string
}

func billingRiskRedisKeys(userID int64) billingRiskKeys {
	tag := "{" + strconv.FormatInt(userID, 10) + "}"
	base := billingRiskRedisKeyPrefix + tag
	return billingRiskKeys{
		leases:  base + ":leases",
		costs:   base + ":costs",
		meta:    base + ":meta",
		balance: base + ":balance",
	}
}

var acquireBillingRiskScript = redis.NewScript(`
local leases = KEYS[1]
local costs = KEYS[2]
local meta = KEYS[3]
local shared_balance = KEYS[4]
local lease_id = ARGV[1]
local risk = tonumber(ARGV[2])
local balance = tonumber(ARGV[3])
local minimum_reserve = tonumber(ARGV[4])
local overdraft = tonumber(ARGV[5])
local lease_ttl = tonumber(ARGV[6])
local idle_ttl = tonumber(ARGV[7])
local redis_time = redis.call('TIME')
local now = tonumber(redis_time[1]) * 1000 + math.floor(tonumber(redis_time[2]) / 1000)

local function extend_ttl(key, ttl)
  local current = redis.call('PTTL', key)
  if current < ttl then
    redis.call('PEXPIRE', key, ttl)
  end
end

local reserved = tonumber(redis.call('HGET', meta, 'reserved_total') or '0')
local expired = redis.call('ZRANGEBYSCORE', leases, '-inf', now, 'LIMIT', 0, 256)
for _, expired_id in ipairs(expired) do
  local expired_cost = tonumber(redis.call('HGET', costs, expired_id) or '0')
  redis.call('ZREM', leases, expired_id)
  redis.call('HDEL', costs, expired_id)
  reserved = math.max(0, reserved - expired_cost)
end
redis.call('HSET', meta, 'reserved_total', reserved)
if #expired == 256 and #redis.call('ZRANGEBYSCORE', leases, '-inf', now, 'LIMIT', 0, 1) > 0 then
  for _, key in ipairs(KEYS) do
    extend_ttl(key, idle_ttl)
  end
  return {2, 0, reserved, balance}
end

local known = balance
local shared_known_raw = redis.call('HGET', shared_balance, 'known_balance')
if shared_known_raw ~= false then
  known = math.min(known, tonumber(shared_known_raw))
end
local known_raw = redis.call('HGET', meta, 'known_balance')
if known_raw ~= false then
  known = math.min(known, tonumber(known_raw))
end
redis.call('HSET', meta, 'known_balance', known)

local existing = redis.call('HGET', costs, lease_id)
if existing ~= false then
  if tonumber(existing) ~= risk then
    return redis.error_reply('billing risk lease id reused with different risk')
  end
  redis.call('ZADD', leases, now + lease_ttl, lease_id)
  local key_ttl = lease_ttl + idle_ttl
  extend_ttl(leases, key_ttl)
  extend_ttl(costs, key_ttl)
  extend_ttl(meta, key_ttl)
  extend_ttl(shared_balance, key_ttl)
  return {1, 0, reserved, known}
end

local spendable = math.max(known - minimum_reserve + overdraft, 0)
local would_reject = reserved + risk > spendable
if would_reject then
  extend_ttl(meta, idle_ttl)
  extend_ttl(shared_balance, idle_ttl)
  return {0, 1, reserved, known}
end

redis.call('ZADD', leases, now + lease_ttl, lease_id)
redis.call('HSET', costs, lease_id, risk)
reserved = reserved + risk
redis.call('HSET', meta, 'reserved_total', reserved)
local key_ttl = lease_ttl + idle_ttl
extend_ttl(leases, key_ttl)
extend_ttl(costs, key_ttl)
extend_ttl(meta, key_ttl)
extend_ttl(shared_balance, key_ttl)
return {1, would_reject and 1 or 0, reserved, known}
`)

var refreshBillingRiskScript = redis.NewScript(`
local leases = KEYS[1]
local costs = KEYS[2]
local meta = KEYS[3]
local lease_id = ARGV[1]
local lease_ttl = tonumber(ARGV[2])
local idle_ttl = tonumber(ARGV[3])
local redis_time = redis.call('TIME')
local now = tonumber(redis_time[1]) * 1000 + math.floor(tonumber(redis_time[2]) / 1000)
if redis.call('HGET', costs, lease_id) == false or redis.call('ZSCORE', leases, lease_id) == false then
  return 0
end
redis.call('ZADD', leases, now + lease_ttl, lease_id)
local key_ttl = lease_ttl + idle_ttl
for _, key in ipairs(KEYS) do
  local current = redis.call('PTTL', key)
  if current < key_ttl then redis.call('PEXPIRE', key, key_ttl) end
end
return 1
`)

var commitBillingRiskScript = redis.NewScript(`
local leases = KEYS[1]
local costs = KEYS[2]
local meta = KEYS[3]
local lease_id = ARGV[1]
local new_balance = tonumber(ARGV[2])
local idle_ttl = tonumber(ARGV[3])

local shared_known_raw = redis.call('HGET', KEYS[4], 'known_balance')
local shared_known = new_balance
if shared_known_raw ~= false then shared_known = math.min(tonumber(shared_known_raw), new_balance) end
redis.call('HSET', KEYS[4], 'known_balance', shared_known)
redis.call('HINCRBY', KEYS[4], 'version', 1)

local known_raw = redis.call('HGET', meta, 'known_balance')
local known = new_balance
if known_raw ~= false then known = math.min(tonumber(known_raw), new_balance) end
redis.call('HSET', meta, 'known_balance', known)

local cost_raw = redis.call('HGET', costs, lease_id)
local removed = 0
if cost_raw ~= false then
  local reserved = tonumber(redis.call('HGET', meta, 'reserved_total') or '0')
  reserved = math.max(0, reserved - tonumber(cost_raw))
  redis.call('HSET', meta, 'reserved_total', reserved)
  redis.call('HDEL', costs, lease_id)
  redis.call('ZREM', leases, lease_id)
  removed = 1
end
for _, key in ipairs(KEYS) do
  local current = redis.call('PTTL', key)
  if current < idle_ttl then redis.call('PEXPIRE', key, idle_ttl) end
end
return removed
`)

var releaseBillingRiskScript = redis.NewScript(`
local cost_raw = redis.call('HGET', KEYS[2], ARGV[1])
if cost_raw == false then return 0 end
local reserved = tonumber(redis.call('HGET', KEYS[3], 'reserved_total') or '0')
reserved = math.max(0, reserved - tonumber(cost_raw))
redis.call('HSET', KEYS[3], 'reserved_total', reserved)
redis.call('HDEL', KEYS[2], ARGV[1])
redis.call('ZREM', KEYS[1], ARGV[1])
local idle_ttl = tonumber(ARGV[2])
for _, key in ipairs(KEYS) do
  local current = redis.call('PTTL', key)
  if current < idle_ttl then redis.call('PEXPIRE', key, idle_ttl) end
end
return 1
`)

var markBillingRiskUncertainScript = redis.NewScript(`
local lease_id = ARGV[1]
local risk = tonumber(ARGV[2])
local cooldown = tonumber(ARGV[3])
local idle_ttl = tonumber(ARGV[4])
local cost_raw = redis.call('HGET', KEYS[2], lease_id)
local reserved_raw = redis.call('HGET', KEYS[3], 'reserved_total')
if cost_raw == false then
  redis.call('HSET', KEYS[2], lease_id, risk)
end
if cost_raw == false or reserved_raw == false then
  local reserved = 0
  for _, value in ipairs(redis.call('HVALS', KEYS[2])) do
    reserved = reserved + tonumber(value)
  end
  redis.call('HSET', KEYS[3], 'reserved_total', reserved)
end
local redis_time = redis.call('TIME')
local now = tonumber(redis_time[1]) * 1000 + math.floor(tonumber(redis_time[2]) / 1000)
local expires_at = now + cooldown
redis.call('ZADD', KEYS[1], expires_at, lease_id)
local uncertain_raw = redis.call('HGET', KEYS[3], 'uncertain_until')
local uncertain_until = expires_at
if uncertain_raw ~= false then uncertain_until = math.max(tonumber(uncertain_raw), expires_at) end
redis.call('HSET', KEYS[3], 'uncertain_until', uncertain_until)
local key_ttl = cooldown + idle_ttl
for _, key in ipairs(KEYS) do
  local current = redis.call('PTTL', key)
  if current < key_ttl then redis.call('PEXPIRE', key, key_ttl) end
end
return 1
`)

var resetBillingRiskBalanceScript = redis.NewScript(`
local balance = tonumber(ARGV[1])
local expected_version = tonumber(ARGV[2])
local idle_ttl = tonumber(ARGV[3])
local current_version = tonumber(redis.call('HGET', KEYS[2], 'version') or '0')
if current_version ~= expected_version then
  local known_raw = redis.call('HGET', KEYS[2], 'known_balance')
  local known = balance
  if known_raw ~= false then known = tonumber(known_raw) end
  return {0, known}
end

if redis.call('EXISTS', KEYS[1]) == 1 then
  redis.call('HSET', KEYS[1], 'known_balance', balance)
  local current = redis.call('PTTL', KEYS[1])
  if current < idle_ttl then redis.call('PEXPIRE', KEYS[1], idle_ttl) end
end
if redis.call('EXISTS', KEYS[1]) == 1 or redis.call('EXISTS', KEYS[2]) == 1 then
  redis.call('HSET', KEYS[2], 'known_balance', balance)
  redis.call('HSET', KEYS[2], 'version', current_version + 1)
  local current = redis.call('PTTL', KEYS[2])
  if current < idle_ttl then redis.call('PEXPIRE', KEYS[2], idle_ttl) end
end
return {1, balance}
`)

type billingRiskStore struct {
	rdb *redis.Client
}

func NewBillingRiskStore(rdb *redis.Client) service.BillingRiskStore {
	return &billingRiskStore{rdb: rdb}
}

func (s *billingRiskStore) Acquire(ctx context.Context, request service.BillingRiskAcquireRequest) (*service.BillingRiskAcquireResult, error) {
	if s == nil || s.rdb == nil {
		return nil, fmt.Errorf("billing risk redis store is unavailable")
	}
	leaseID := strings.TrimSpace(request.LeaseID)
	if request.UserID <= 0 || leaseID == "" || request.RiskMicros <= 0 || request.LeaseTTL <= 0 || request.IdleTTL <= 0 {
		return nil, fmt.Errorf("invalid billing risk acquire request")
	}
	keys := billingRiskRedisKeys(request.UserID)
	for {
		result, err := acquireBillingRiskScript.Run(ctx, s.rdb, []string{keys.leases, keys.costs, keys.meta, keys.balance},
			leaseID,
			request.RiskMicros,
			request.BalanceMicros,
			request.MinimumReserveMicros,
			request.OverdraftAllowanceMicros,
			durationMillis(request.LeaseTTL),
			durationMillis(request.IdleTTL),
		).Result()
		if err != nil {
			return nil, err
		}
		values, ok := result.([]any)
		if !ok || len(values) != 4 {
			return nil, fmt.Errorf("unexpected billing risk acquire result: %T", result)
		}
		acquired, err := billingRiskRedisInt64(values[0])
		if err != nil {
			return nil, err
		}
		if acquired == 2 {
			continue
		}
		wouldReject, err := billingRiskRedisInt64(values[1])
		if err != nil {
			return nil, err
		}
		reserved, err := billingRiskRedisInt64(values[2])
		if err != nil {
			return nil, err
		}
		known, err := billingRiskRedisInt64(values[3])
		if err != nil {
			return nil, err
		}
		return &service.BillingRiskAcquireResult{
			Acquired:            acquired == 1,
			WouldReject:         wouldReject == 1,
			ReservedTotalMicros: reserved,
			KnownBalanceMicros:  known,
		}, nil
	}
}

func (s *billingRiskStore) GetBalanceVersion(ctx context.Context, userID int64) (int64, error) {
	if s == nil || s.rdb == nil || userID <= 0 {
		return 0, fmt.Errorf("invalid billing risk balance version request")
	}
	keys := billingRiskRedisKeys(userID)
	version, err := s.rdb.HGet(ctx, keys.balance, "version").Int64()
	if err == redis.Nil {
		return 0, nil
	}
	return version, err
}

func (s *billingRiskStore) Refresh(ctx context.Context, userID int64, leaseID string, ttl, idleTTL time.Duration) (bool, error) {
	keys := billingRiskRedisKeys(userID)
	result, err := refreshBillingRiskScript.Run(ctx, s.rdb, []string{keys.leases, keys.costs, keys.meta},
		strings.TrimSpace(leaseID), durationMillis(ttl), durationMillis(idleTTL)).Int64()
	return result == 1, err
}

func (s *billingRiskStore) Commit(ctx context.Context, userID int64, leaseID string, newBalanceMicros int64, idleTTL time.Duration) (bool, error) {
	keys := billingRiskRedisKeys(userID)
	result, err := commitBillingRiskScript.Run(ctx, s.rdb, []string{keys.leases, keys.costs, keys.meta, keys.balance},
		strings.TrimSpace(leaseID), newBalanceMicros, durationMillis(idleTTL)).Int64()
	return result == 1, err
}

func (s *billingRiskStore) Release(ctx context.Context, userID int64, leaseID string, idleTTL time.Duration) (bool, error) {
	keys := billingRiskRedisKeys(userID)
	result, err := releaseBillingRiskScript.Run(ctx, s.rdb, []string{keys.leases, keys.costs, keys.meta},
		strings.TrimSpace(leaseID), durationMillis(idleTTL)).Int64()
	return result == 1, err
}

func (s *billingRiskStore) MarkUncertain(ctx context.Context, userID int64, leaseID string, riskMicros int64, cooldown, idleTTL time.Duration) (bool, error) {
	if riskMicros <= 0 {
		return false, fmt.Errorf("invalid billing risk uncertain request")
	}
	keys := billingRiskRedisKeys(userID)
	result, err := markBillingRiskUncertainScript.Run(ctx, s.rdb, []string{keys.leases, keys.costs, keys.meta},
		strings.TrimSpace(leaseID), riskMicros, durationMillis(cooldown), durationMillis(idleTTL)).Int64()
	return result == 1, err
}

func (s *billingRiskStore) ResetBalance(ctx context.Context, userID int64, balanceMicros, expectedVersion int64, idleTTL time.Duration) (*service.BillingRiskBalanceResetResult, error) {
	keys := billingRiskRedisKeys(userID)
	result, err := resetBillingRiskBalanceScript.Run(ctx, s.rdb, []string{keys.meta, keys.balance},
		balanceMicros, expectedVersion, durationMillis(idleTTL)).Result()
	if err != nil {
		return nil, err
	}
	values, ok := result.([]any)
	if !ok || len(values) != 2 {
		return nil, fmt.Errorf("unexpected billing risk balance reset result: %T", result)
	}
	accepted, err := billingRiskRedisInt64(values[0])
	if err != nil {
		return nil, err
	}
	known, err := billingRiskRedisInt64(values[1])
	if err != nil {
		return nil, err
	}
	return &service.BillingRiskBalanceResetResult{Accepted: accepted == 1, KnownBalanceMicros: known}, nil
}

func durationMillis(duration time.Duration) int64 {
	millis := duration.Milliseconds()
	if millis <= 0 {
		return 1
	}
	return millis
}

func billingRiskRedisInt64(value any) (int64, error) {
	switch typed := value.(type) {
	case int64:
		return typed, nil
	case string:
		return strconv.ParseInt(typed, 10, 64)
	case []byte:
		return strconv.ParseInt(string(typed), 10, 64)
	default:
		return 0, fmt.Errorf("unexpected Redis integer type: %T", value)
	}
}
