package repository

import (
	"context"
	"fmt"

	"github.com/redis/go-redis/v9"
)

// Both keys share the account hash tag, so the reservation is cluster-safe.
// Redis TIME avoids clock skew between gateway instances. Only admissions are
// stored: an overload of rejected requests cannot keep the account blocked.
var reserveRPMScript = redis.NewScript(`
local t = redis.call('TIME')
local now = tonumber(t[1]) * 1000 + math.floor(tonumber(t[2]) / 1000)
redis.call('ZREMRANGEBYSCORE', KEYS[1], '-inf', now - 60000)
if redis.call('ZCARD', KEYS[1]) >= tonumber(ARGV[1]) then
  return 0
end
local seq = redis.call('INCR', KEYS[2])
redis.call('ZADD', KEYS[1], now, tostring(now) .. ':' .. tostring(seq))
redis.call('PEXPIRE', KEYS[1], 120000)
redis.call('PEXPIRE', KEYS[2], 120000)
return 1
`)

var reservedRPMReadScript = redis.NewScript(`
local exists = redis.call('EXISTS', KEYS[1])
local t = redis.call('TIME')
local now = tonumber(t[1]) * 1000 + math.floor(tonumber(t[2]) / 1000)
redis.call('ZREMRANGEBYSCORE', KEYS[1], '-inf', now - 60000)
return {exists, redis.call('ZCARD', KEYS[1])}
`)

func rpmReservationKey(accountID int64) string {
	return fmt.Sprintf("rpm:reservation:{%d}", accountID)
}

func (c *RPMCacheImpl) ReserveRPM(ctx context.Context, accountID int64, limit int) (bool, error) {
	if limit <= 0 {
		return true, nil
	}
	key := rpmReservationKey(accountID)
	result, err := reserveRPMScript.Run(ctx, c.rdb, []string{key, key + ":seq"}, limit).Int()
	if err != nil {
		return false, fmt.Errorf("rpm reservation: %w", err)
	}
	return result == 1, nil
}

func reservedRPMResult(raw any) (int, bool, error) {
	values, ok := raw.([]any)
	if !ok || len(values) != 2 {
		return 0, false, fmt.Errorf("invalid RPM reservation counter response")
	}
	exists, ok1 := values[0].(int64)
	count, ok2 := values[1].(int64)
	if !ok1 || !ok2 {
		return 0, false, fmt.Errorf("invalid RPM reservation counter values")
	}
	return int(count), exists != 0, nil
}

func (c *RPMCacheImpl) reservedRPM(ctx context.Context, accountID int64) (int, bool, error) {
	raw, err := reservedRPMReadScript.Run(ctx, c.rdb, []string{rpmReservationKey(accountID)}).Result()
	if err != nil {
		return 0, false, err
	}
	return reservedRPMResult(raw)
}
