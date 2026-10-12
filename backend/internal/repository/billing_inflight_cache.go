package repository

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/redis/go-redis/v9"
)

// 余额在途预留：
//   - billing:inflight:{uid}      ZSET  member=requestID score=过期时间(ms)
//   - billing:inflight_amt:{uid}  HASH  field=requestID value=预留金额(USD)
//
// 两个 key 使用同一 hash tag，保证 Redis Cluster 下 Lua 可同时访问。
const (
	billingInflightKeyPrefix    = "billing:inflight:"
	billingInflightAmtKeyPrefix = "billing:inflight_amt:"
)

func billingInflightKeys(userID int64) (string, string) {
	return fmt.Sprintf("%s{%d}", billingInflightKeyPrefix, userID),
		fmt.Sprintf("%s{%d}", billingInflightAmtKeyPrefix, userID)
}

var (
	// KEYS: [1]=zset [2]=hash
	// ARGV: [1]=now_ms [2]=expire_at_ms [3]=member [4]=amount [5]=balance [6]=key_ttl_ms
	// 返回 {allowed, inflight_sum(string), inflight_count}
	// 规则：先清理已过期成员；若无在途预留则直接放行（与旧行为一致，外层已校验余额 > 阈值）；
	// 否则要求 balance - sum(在途) >= amount。放行时登记预留。
	reserveInflightBalanceScript = redis.NewScript(`
		local now = tonumber(ARGV[1])
		local expired = redis.call('ZRANGEBYSCORE', KEYS[1], '-inf', now)
		if #expired > 0 then
			redis.call('ZREMRANGEBYSCORE', KEYS[1], '-inf', now)
			redis.call('HDEL', KEYS[2], unpack(expired))
		end
		local vals = redis.call('HVALS', KEYS[2])
		local sum = 0
		for i = 1, #vals do
			sum = sum + (tonumber(vals[i]) or 0)
		end
		local count = redis.call('ZCARD', KEYS[1])
		local amount = tonumber(ARGV[4])
		local balance = tonumber(ARGV[5])
		if count > 0 and (balance - sum) < amount then
			return {0, tostring(sum), count}
		end
		redis.call('ZADD', KEYS[1], tonumber(ARGV[2]), ARGV[3])
		redis.call('HSET', KEYS[2], ARGV[3], ARGV[4])
		redis.call('PEXPIRE', KEYS[1], ARGV[6])
		redis.call('PEXPIRE', KEYS[2], ARGV[6])
		return {1, tostring(sum), count}
	`)

	// KEYS: [1]=zset [2]=hash  ARGV: [1]=member [2]=expire_at_ms [3]=key_ttl_ms [4]=now_ms
	// 仅当成员仍存在且尚未过期（score > now）时续期（XX），并把两个 key 的 TTL 至少延长到 key_ttl_ms。
	// 已过期但尚未被惰性清理的成员不得被复活：顺手清掉并返回 0。
	renewInflightBalanceScript = redis.NewScript(`
		local score = redis.call('ZSCORE', KEYS[1], ARGV[1])
		if not score then
			return 0
		end
		if tonumber(score) <= tonumber(ARGV[4]) then
			redis.call('ZREM', KEYS[1], ARGV[1])
			redis.call('HDEL', KEYS[2], ARGV[1])
			return 0
		end
		redis.call('ZADD', KEYS[1], 'XX', tonumber(ARGV[2]), ARGV[1])
		local ttl = tonumber(ARGV[3])
		if redis.call('PTTL', KEYS[1]) < ttl then
			redis.call('PEXPIRE', KEYS[1], ttl)
		end
		if redis.call('PTTL', KEYS[2]) < ttl then
			redis.call('PEXPIRE', KEYS[2], ttl)
		end
		return 1
	`)

	releaseInflightBalanceScript = redis.NewScript(`
		redis.call('ZREM', KEYS[1], ARGV[1])
		redis.call('HDEL', KEYS[2], ARGV[1])
		return 1
	`)
)

// ReserveInflightBalance 实现 service.InflightBalanceReservationCache。
func (c *billingCache) ReserveInflightBalance(ctx context.Context, userID int64, requestID string, amount, balance float64, ttl time.Duration) (bool, float64, error) {
	zkey, hkey := billingInflightKeys(userID)
	now := time.Now().UnixMilli()
	ttlMs := ttl.Milliseconds()
	if ttlMs <= 0 {
		ttlMs = 1
	}
	res, err := reserveInflightBalanceScript.Run(ctx, c.rdb, []string{zkey, hkey},
		now,
		now+ttlMs,
		requestID,
		strconv.FormatFloat(amount, 'f', -1, 64),
		strconv.FormatFloat(balance, 'f', -1, 64),
		ttlMs,
	).Slice()
	if err != nil {
		return false, 0, err
	}
	if len(res) < 2 {
		return false, 0, fmt.Errorf("unexpected inflight reservation reply: %v", res)
	}
	allowed, _ := res[0].(int64)
	var sum float64
	if s, ok := res[1].(string); ok {
		sum, _ = strconv.ParseFloat(s, 64)
	}
	return allowed == 1, sum, nil
}

// ReleaseInflightBalance 实现 service.InflightBalanceReservationCache。
func (c *billingCache) ReleaseInflightBalance(ctx context.Context, userID int64, requestID string) error {
	zkey, hkey := billingInflightKeys(userID)
	return releaseInflightBalanceScript.Run(ctx, c.rdb, []string{zkey, hkey}, requestID).Err()
}

// RenewInflightBalance 实现 service.InflightBalanceReservationRenewer。
func (c *billingCache) RenewInflightBalance(ctx context.Context, userID int64, requestID string, ttl time.Duration) (bool, error) {
	zkey, hkey := billingInflightKeys(userID)
	ttlMs := ttl.Milliseconds()
	if ttlMs <= 0 {
		ttlMs = 1
	}
	now := time.Now().UnixMilli()
	n, err := renewInflightBalanceScript.Run(ctx, c.rdb, []string{zkey, hkey}, requestID, now+ttlMs, ttlMs, now).Int64()
	if err != nil {
		return false, err
	}
	return n == 1, nil
}

var (
	_ service.InflightBalanceReservationCache   = (*billingCache)(nil)
	_ service.InflightBalanceReservationRenewer = (*billingCache)(nil)
)
