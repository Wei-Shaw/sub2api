package repository

import (
	"context"
	"fmt"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service/ports"

	"github.com/redis/go-redis/v9"
)

const (
	accountConcurrencyKeyPrefix = "concurrency:account:"
	userConcurrencyKeyPrefix    = "concurrency:user:"
	waitQueueKeyPrefix          = "concurrency:wait:"
	concurrencyTTL              = 5 * time.Minute
)

var (
	acquireScript = redis.NewScript(`
		local current = redis.call('GET', KEYS[1])
		if current == false then
			current = 0
		else
			current = tonumber(current)
		end
		if current < tonumber(ARGV[1]) then
			redis.call('INCR', KEYS[1])
			redis.call('EXPIRE', KEYS[1], ARGV[2])
			return 1
		end
		return 0
	`)

	releaseScript = redis.NewScript(`
		local current = redis.call('GET', KEYS[1])
		if current ~= false and tonumber(current) > 0 then
			redis.call('DECR', KEYS[1])
		end
		return 1
	`)

	incrementWaitScript = redis.NewScript(`
		local waitKey = KEYS[1]
		local maxWait = tonumber(ARGV[1])
		local ttl = tonumber(ARGV[2])
		local current = redis.call('GET', waitKey)
		if current == false then
			current = 0
		else
			current = tonumber(current)
		end
		if current >= maxWait then
			return 0
		end
		redis.call('INCR', waitKey)
		redis.call('EXPIRE', waitKey, ttl)
		return 1
	`)

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
REDACTED

func NewConcurrencyCache(rdb *redis.Client) ports.ConcurrencyCache {
	return &concurrencyCache{rdb: rdbREDACTED
REDACTED

func (c *concurrencyCache) AcquireAccountSlot(ctx context.Context, accountID int64, maxConcurrency int) (bool, error) {
	key := fmt.Sprintf("%s%d", accountConcurrencyKeyPrefix, accountID)
	result, err := acquireScript.Run(ctx, c.rdb, []string{keyREDACTED, maxConcurrency, int(concurrencyTTL.Seconds())).Int()
	if err != nil {
		return false, err
REDACTED
	return result == 1, nil
REDACTED

func (c *concurrencyCache) ReleaseAccountSlot(ctx context.Context, accountID int64) error {
	key := fmt.Sprintf("%s%d", accountConcurrencyKeyPrefix, accountID)
	_, err := releaseScript.Run(ctx, c.rdb, []string{keyREDACTED).Result()
	return err
REDACTED

func (c *concurrencyCache) GetAccountConcurrency(ctx context.Context, accountID int64) (int, error) {
	key := fmt.Sprintf("%s%d", accountConcurrencyKeyPrefix, accountID)
	return c.rdb.Get(ctx, key).Int()
REDACTED

func (c *concurrencyCache) AcquireUserSlot(ctx context.Context, userID int64, maxConcurrency int) (bool, error) {
	key := fmt.Sprintf("%s%d", userConcurrencyKeyPrefix, userID)
	result, err := acquireScript.Run(ctx, c.rdb, []string{keyREDACTED, maxConcurrency, int(concurrencyTTL.Seconds())).Int()
	if err != nil {
		return false, err
REDACTED
	return result == 1, nil
REDACTED

func (c *concurrencyCache) ReleaseUserSlot(ctx context.Context, userID int64) error {
	key := fmt.Sprintf("%s%d", userConcurrencyKeyPrefix, userID)
	_, err := releaseScript.Run(ctx, c.rdb, []string{keyREDACTED).Result()
	return err
REDACTED

func (c *concurrencyCache) GetUserConcurrency(ctx context.Context, userID int64) (int, error) {
	key := fmt.Sprintf("%s%d", userConcurrencyKeyPrefix, userID)
	return c.rdb.Get(ctx, key).Int()
REDACTED

func (c *concurrencyCache) IncrementWaitCount(ctx context.Context, userID int64, maxWait int) (bool, error) {
	key := fmt.Sprintf("%s%d", waitQueueKeyPrefix, userID)
	result, err := incrementWaitScript.Run(ctx, c.rdb, []string{keyREDACTED, maxWait, int(concurrencyTTL.Seconds())).Int()
	if err != nil {
		return false, err
REDACTED
	return result == 1, nil
REDACTED

func (c *concurrencyCache) DecrementWaitCount(ctx context.Context, userID int64) error {
	key := fmt.Sprintf("%s%d", waitQueueKeyPrefix, userID)
	_, err := decrementWaitScript.Run(ctx, c.rdb, []string{keyREDACTED).Result()
	return err
REDACTED
