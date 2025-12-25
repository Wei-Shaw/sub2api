package repository

import (
	"context"
	"fmt"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service/ports"

	"github.com/redis/go-redis/v9"
)

const (
	// Key prefixes for independent slot keys
	// Format: concurrency:account:{accountID}:{requestID}
	accountSlotKeyPrefix = "concurrency:account:"
	// Format: concurrency:user:{userID}:{requestID}
	userSlotKeyPrefix = "concurrency:user:"
	// Wait queue keeps counter format: concurrency:wait:{userID}
	waitQueueKeyPrefix = "concurrency:wait:"

	// Slot TTL - each slot expires independently
	slotTTL = 5 * time.Minute
)

var (
	// acquireScript uses SCAN to count existing slots and creates new slot if under limit
	// KEYS[1] = pattern for SCAN (e.g., "concurrency:account:2:*")
	// KEYS[2] = full slot key (e.g., "concurrency:account:2:req_xxx")
	// ARGV[1] = maxConcurrency
	// ARGV[2] = TTL in seconds
	acquireScript = redis.NewScript(`
		local pattern = KEYS[1]
		local slotKey = KEYS[2]
		local maxConcurrency = tonumber(ARGV[1])
		local ttl = tonumber(ARGV[2])

		-- Count existing slots using SCAN
		local cursor = "0"
		local count = 0
		repeat
			local result = redis.call('SCAN', cursor, 'MATCH', pattern, 'COUNT', 100)
			cursor = result[1]
			count = count + #result[2]
		until cursor == "0"

		-- Check if we can acquire a slot
		if count < maxConcurrency then
			redis.call('SET', slotKey, '1', 'EX', ttl)
			return 1
		end

		return 0
	`)

	// getCountScript counts slots using SCAN
	// KEYS[1] = pattern for SCAN
	getCountScript = redis.NewScript(`
		local pattern = KEYS[1]
		local cursor = "0"
		local count = 0
		repeat
			local result = redis.call('SCAN', cursor, 'MATCH', pattern, 'COUNT', 100)
			cursor = result[1]
			count = count + #result[2]
		until cursor == "0"
		return count
	`)

	// incrementWaitScript - only sets TTL on first creation to avoid refreshing
	// KEYS[1] = wait queue key
	// ARGV[1] = maxWait
	// ARGV[2] = TTL in seconds
	incrementWaitScript = redis.NewScript(`
		local current = redis.call('GET', KEYS[1])
		if current == false then
			current = 0
		else
			current = tonumber(current)
		end

		if current >= tonumber(ARGV[1]) then
			return 0
		end

		local newVal = redis.call('INCR', KEYS[1])

		-- Only set TTL on first creation to avoid refreshing zombie data
		if newVal == 1 then
			redis.call('EXPIRE', KEYS[1], ARGV[2])
		end

		return 1
	`)

	// decrementWaitScript - same as before
	decrementWaitScript = redis.NewScript(`
		local current = redis.call('GET', KEYS[1])
		if current ~= false and tonumber(current) > 0 then
			redis.call('DECR', KEYS[1])
		end
		return 1
	`)
)

type concurrencyCache struct {
	rdb *redis.Client
}

func NewConcurrencyCache(rdb *redis.Client) ports.ConcurrencyCache {
	return &concurrencyCache{rdb: rdb}
}

// Helper functions for key generation
func accountSlotKey(accountID int64, requestID string) string {
	return fmt.Sprintf("%s%d:%s", accountSlotKeyPrefix, accountID, requestID)
}

func accountSlotPattern(accountID int64) string {
	return fmt.Sprintf("%s%d:*", accountSlotKeyPrefix, accountID)
}

func userSlotKey(userID int64, requestID string) string {
	return fmt.Sprintf("%s%d:%s", userSlotKeyPrefix, userID, requestID)
}

func userSlotPattern(userID int64) string {
	return fmt.Sprintf("%s%d:*", userSlotKeyPrefix, userID)
}

func waitQueueKey(userID int64) string {
	return fmt.Sprintf("%s%d", waitQueueKeyPrefix, userID)
}

// Account slot operations

func (c *concurrencyCache) AcquireAccountSlot(ctx context.Context, accountID int64, maxConcurrency int, requestID string) (bool, error) {
	pattern := accountSlotPattern(accountID)
	slotKey := accountSlotKey(accountID, requestID)

	result, err := acquireScript.Run(ctx, c.rdb, []string{pattern, slotKey}, maxConcurrency, int(slotTTL.Seconds())).Int()
	if err != nil {
		return false, err
	}
	return result == 1, nil
}

func (c *concurrencyCache) ReleaseAccountSlot(ctx context.Context, accountID int64, requestID string) error {
	slotKey := accountSlotKey(accountID, requestID)
	return c.rdb.Del(ctx, slotKey).Err()
}

func (c *concurrencyCache) GetAccountConcurrency(ctx context.Context, accountID int64) (int, error) {
	pattern := accountSlotPattern(accountID)
	result, err := getCountScript.Run(ctx, c.rdb, []string{pattern}).Int()
	if err != nil {
		return 0, err
	}
	return result, nil
}

// User slot operations

func (c *concurrencyCache) AcquireUserSlot(ctx context.Context, userID int64, maxConcurrency int, requestID string) (bool, error) {
	pattern := userSlotPattern(userID)
	slotKey := userSlotKey(userID, requestID)

	result, err := acquireScript.Run(ctx, c.rdb, []string{pattern, slotKey}, maxConcurrency, int(slotTTL.Seconds())).Int()
	if err != nil {
		return false, err
	}
	return result == 1, nil
}

func (c *concurrencyCache) ReleaseUserSlot(ctx context.Context, userID int64, requestID string) error {
	slotKey := userSlotKey(userID, requestID)
	return c.rdb.Del(ctx, slotKey).Err()
}

func (c *concurrencyCache) GetUserConcurrency(ctx context.Context, userID int64) (int, error) {
	pattern := userSlotPattern(userID)
	result, err := getCountScript.Run(ctx, c.rdb, []string{pattern}).Int()
	if err != nil {
		return 0, err
	}
	return result, nil
}

// Wait queue operations

func (c *concurrencyCache) IncrementWaitCount(ctx context.Context, userID int64, maxWait int) (bool, error) {
	key := waitQueueKey(userID)
	result, err := incrementWaitScript.Run(ctx, c.rdb, []string{key}, maxWait, int(slotTTL.Seconds())).Int()
	if err != nil {
		return false, err
	}
	return result == 1, nil
}

func (c *concurrencyCache) DecrementWaitCount(ctx context.Context, userID int64) error {
	key := waitQueueKey(userID)
	_, err := decrementWaitScript.Run(ctx, c.rdb, []string{key}).Result()
	return err
}
