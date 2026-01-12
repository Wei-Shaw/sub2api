package repository

import (
	"context"
	"errors"
	"fmt"
	"strconv"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/redis/go-redis/v9"
)

// 并发控制缓存常量定义
//
// 性能优化说明：
// 原实现使用 SCAN 命令遍历独立的槽位键（concurrency:account:{idREDACTED:{requestIDREDACTED），
// 在高并发场景下 SCAN 需要多次往返，且遍历大量键时性能下降明显。
//
// 新实现改用 Redis 有序集合（Sorted Set）：
// 1. 每个账号/用户只有一个键，成员为 requestID，分数为时间戳
// 2. 使用 ZCARD 原子获取并发数，时间复杂度 O(1)
// 3. 使用 ZREMRANGEBYSCORE 清理过期槽位，避免手动管理 TTL
// 4. 单次 Redis 调用完成计数，减少网络往返
const (
	// 并发槽位键前缀（有序集合）
	// 格式: concurrency:account:{accountIDREDACTED
	accountSlotKeyPrefix = "concurrency:account:"
	// 格式: concurrency:user:{userIDREDACTED
	userSlotKeyPrefix = "concurrency:user:"
	// 等待队列计数器格式: concurrency:wait:{userIDREDACTED
	waitQueueKeyPrefix = "concurrency:wait:"
	// 账号级等待队列计数器格式: wait:account:{accountIDREDACTED
	accountWaitKeyPrefix = "wait:account:"

	// 默认槽位过期时间（分钟），可通过配置覆盖
	defaultSlotTTLMinutes = 15
)

var (
	// acquireScript 使用有序集合计数并在未达上限时添加槽位
	// 使用 Redis TIME 命令获取服务器时间，避免多实例时钟不同步问题
	// KEYS[1] = 有序集合键 (concurrency:account:{idREDACTED / concurrency:user:{idREDACTED)
	// ARGV[1] = maxConcurrency
	// ARGV[2] = TTL（秒）
	// ARGV[3] = requestID
	acquireScript = redis.NewScript(`
		local key = KEYS[1]
		local maxConcurrency = tonumber(ARGV[1])
		local ttl = tonumber(ARGV[2])
		local requestID = ARGV[3]

		-- 使用 Redis 服务器时间，确保多实例时钟一致
		local timeResult = redis.call('TIME')
		local now = tonumber(timeResult[1])
		local expireBefore = now - ttl

		-- 清理过期槽位
		redis.call('ZREMRANGEBYSCORE', key, '-inf', expireBefore)

		-- 检查是否已存在（支持重试场景刷新时间戳）
		local exists = redis.call('ZSCORE', key, requestID)
		if exists ~= false then
			redis.call('ZADD', key, now, requestID)
			redis.call('EXPIRE', key, ttl)
			return 1
		end

		-- 检查是否达到并发上限
		local count = redis.call('ZCARD', key)
		if count < maxConcurrency then
			redis.call('ZADD', key, now, requestID)
			redis.call('EXPIRE', key, ttl)
			return 1
		end

		return 0
	`)

	// getCountScript 统计有序集合中的槽位数量并清理过期条目
	// 使用 Redis TIME 命令获取服务器时间
	// KEYS[1] = 有序集合键
	// ARGV[1] = TTL（秒）
	getCountScript = redis.NewScript(`
		local key = KEYS[1]
		local ttl = tonumber(ARGV[1])

		-- 使用 Redis 服务器时间
		local timeResult = redis.call('TIME')
		local now = tonumber(timeResult[1])
		local expireBefore = now - ttl

		redis.call('ZREMRANGEBYSCORE', key, '-inf', expireBefore)
		return redis.call('ZCARD', key)
	`)

	// incrementWaitScript - refreshes TTL on each increment to keep queue depth accurate
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

		-- Refresh TTL so long-running traffic doesn't expire active queue counters.
		redis.call('EXPIRE', KEYS[1], ARGV[2])

			return 1
		`)

	// incrementAccountWaitScript - account-level wait queue count (refresh TTL on each increment)
	incrementAccountWaitScript = redis.NewScript(`
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

			-- Refresh TTL so long-running traffic doesn't expire active queue counters.
			redis.call('EXPIRE', KEYS[1], ARGV[2])

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

	// getAccountsLoadBatchScript - batch load query with expired slot cleanup
	// ARGV[1] = slot TTL (seconds)
	// ARGV[2..n] = accountID1, maxConcurrency1, accountID2, maxConcurrency2, ...
	getAccountsLoadBatchScript = redis.NewScript(`
			local result = {REDACTED
			local slotTTL = tonumber(ARGV[1])

			-- Get current server time
			local timeResult = redis.call('TIME')
			local nowSeconds = tonumber(timeResult[1])
			local cutoffTime = nowSeconds - slotTTL

			local i = 2
			while i <= #ARGV do
				local accountID = ARGV[i]
				local maxConcurrency = tonumber(ARGV[i + 1])

				local slotKey = 'concurrency:account:' .. accountID

				-- Clean up expired slots before counting
				redis.call('ZREMRANGEBYSCORE', slotKey, '-inf', cutoffTime)
				local currentConcurrency = redis.call('ZCARD', slotKey)

				local waitKey = 'wait:account:' .. accountID
				local waitingCount = redis.call('GET', waitKey)
				if waitingCount == false then
					waitingCount = 0
				else
					waitingCount = tonumber(waitingCount)
				end

				local loadRate = 0
				if maxConcurrency > 0 then
					loadRate = math.floor((currentConcurrency + waitingCount) * 100 / maxConcurrency)
				end

				table.insert(result, accountID)
				table.insert(result, currentConcurrency)
				table.insert(result, waitingCount)
				table.insert(result, loadRate)

				i = i + 2
			end

			return result
		`)

	// cleanupExpiredSlotsScript - remove expired slots
	// KEYS[1] = concurrency:account:{accountIDREDACTED
	// ARGV[1] = TTL (seconds)
	cleanupExpiredSlotsScript = redis.NewScript(`
			local key = KEYS[1]
			local ttl = tonumber(ARGV[1])
			local timeResult = redis.call('TIME')
			local now = tonumber(timeResult[1])
			local expireBefore = now - ttl
			return redis.call('ZREMRANGEBYSCORE', key, '-inf', expireBefore)
		`)
)

type concurrencyCache struct {
	rdb                 *redis.Client
	slotTTLSeconds      int // 槽位过期时间（秒）
	waitQueueTTLSeconds int // 等待队列过期时间（秒）
REDACTED

// NewConcurrencyCache 创建并发控制缓存
// slotTTLMinutes: 槽位过期时间（分钟），0 或负数使用默认值 15 分钟
// waitQueueTTLSeconds: 等待队列过期时间（秒），0 或负数使用 slot TTL
func NewConcurrencyCache(rdb *redis.Client, slotTTLMinutes int, waitQueueTTLSeconds int) service.ConcurrencyCache {
	if slotTTLMinutes <= 0 {
		slotTTLMinutes = defaultSlotTTLMinutes
REDACTED
	if waitQueueTTLSeconds <= 0 {
		waitQueueTTLSeconds = slotTTLMinutes * 60
REDACTED
	return &concurrencyCache{
		rdb:                 rdb,
		slotTTLSeconds:      slotTTLMinutes * 60,
		waitQueueTTLSeconds: waitQueueTTLSeconds,
REDACTED
REDACTED

// Helper functions for key generation
func accountSlotKey(accountID int64) string {
	return fmt.Sprintf("%s%d", accountSlotKeyPrefix, accountID)
REDACTED

func userSlotKey(userID int64) string {
	return fmt.Sprintf("%s%d", userSlotKeyPrefix, userID)
REDACTED

func waitQueueKey(userID int64) string {
	return fmt.Sprintf("%s%d", waitQueueKeyPrefix, userID)
REDACTED

func accountWaitKey(accountID int64) string {
	return fmt.Sprintf("%s%d", accountWaitKeyPrefix, accountID)
REDACTED

// Account slot operations

func (c *concurrencyCache) AcquireAccountSlot(ctx context.Context, accountID int64, maxConcurrency int, requestID string) (bool, error) {
	key := accountSlotKey(accountID)
	// 时间戳在 Lua 脚本内使用 Redis TIME 命令获取，确保多实例时钟一致
	result, err := acquireScript.Run(ctx, c.rdb, []string{keyREDACTED, maxConcurrency, c.slotTTLSeconds, requestID).Int()
	if err != nil {
		return false, err
REDACTED
	return result == 1, nil
REDACTED

func (c *concurrencyCache) ReleaseAccountSlot(ctx context.Context, accountID int64, requestID string) error {
	key := accountSlotKey(accountID)
	return c.rdb.ZRem(ctx, key, requestID).Err()
REDACTED

func (c *concurrencyCache) GetAccountConcurrency(ctx context.Context, accountID int64) (int, error) {
	key := accountSlotKey(accountID)
	// 时间戳在 Lua 脚本内使用 Redis TIME 命令获取
	result, err := getCountScript.Run(ctx, c.rdb, []string{keyREDACTED, c.slotTTLSeconds).Int()
	if err != nil {
		return 0, err
REDACTED
	return result, nil
REDACTED

// User slot operations

func (c *concurrencyCache) AcquireUserSlot(ctx context.Context, userID int64, maxConcurrency int, requestID string) (bool, error) {
	key := userSlotKey(userID)
	// 时间戳在 Lua 脚本内使用 Redis TIME 命令获取，确保多实例时钟一致
	result, err := acquireScript.Run(ctx, c.rdb, []string{keyREDACTED, maxConcurrency, c.slotTTLSeconds, requestID).Int()
	if err != nil {
		return false, err
REDACTED
	return result == 1, nil
REDACTED

func (c *concurrencyCache) ReleaseUserSlot(ctx context.Context, userID int64, requestID string) error {
	key := userSlotKey(userID)
	return c.rdb.ZRem(ctx, key, requestID).Err()
REDACTED

func (c *concurrencyCache) GetUserConcurrency(ctx context.Context, userID int64) (int, error) {
	key := userSlotKey(userID)
	// 时间戳在 Lua 脚本内使用 Redis TIME 命令获取
	result, err := getCountScript.Run(ctx, c.rdb, []string{keyREDACTED, c.slotTTLSeconds).Int()
	if err != nil {
		return 0, err
REDACTED
	return result, nil
REDACTED

// Wait queue operations

func (c *concurrencyCache) IncrementWaitCount(ctx context.Context, userID int64, maxWait int) (bool, error) {
	key := waitQueueKey(userID)
	result, err := incrementWaitScript.Run(ctx, c.rdb, []string{keyREDACTED, maxWait, c.waitQueueTTLSeconds).Int()
	if err != nil {
		return false, err
REDACTED
	return result == 1, nil
REDACTED

func (c *concurrencyCache) DecrementWaitCount(ctx context.Context, userID int64) error {
	key := waitQueueKey(userID)
	_, err := decrementWaitScript.Run(ctx, c.rdb, []string{keyREDACTED).Result()
	return err
REDACTED

// Account wait queue operations

func (c *concurrencyCache) IncrementAccountWaitCount(ctx context.Context, accountID int64, maxWait int) (bool, error) {
	key := accountWaitKey(accountID)
	result, err := incrementAccountWaitScript.Run(ctx, c.rdb, []string{keyREDACTED, maxWait, c.waitQueueTTLSeconds).Int()
	if err != nil {
		return false, err
REDACTED
	return result == 1, nil
REDACTED

func (c *concurrencyCache) DecrementAccountWaitCount(ctx context.Context, accountID int64) error {
	key := accountWaitKey(accountID)
	_, err := decrementWaitScript.Run(ctx, c.rdb, []string{keyREDACTED).Result()
	return err
REDACTED

func (c *concurrencyCache) GetAccountWaitingCount(ctx context.Context, accountID int64) (int, error) {
	key := accountWaitKey(accountID)
	val, err := c.rdb.Get(ctx, key).Int()
	if err != nil && !errors.Is(err, redis.Nil) {
		return 0, err
REDACTED
	if errors.Is(err, redis.Nil) {
		return 0, nil
REDACTED
	return val, nil
REDACTED

func (c *concurrencyCache) GetAccountsLoadBatch(ctx context.Context, accounts []service.AccountWithConcurrency) (map[int64]*service.AccountLoadInfo, error) {
	if len(accounts) == 0 {
		return map[int64]*service.AccountLoadInfo{REDACTED, nil
REDACTED

	args := []any{c.slotTTLSecondsREDACTED
	for _, acc := range accounts {
		args = append(args, acc.ID, acc.MaxConcurrency)
REDACTED

	result, err := getAccountsLoadBatchScript.Run(ctx, c.rdb, []string{REDACTED, args...).Slice()
	if err != nil {
		return nil, err
REDACTED

	loadMap := make(map[int64]*service.AccountLoadInfo)
	for i := 0; i < len(result); i += 4 {
		if i+3 >= len(result) {
			break
	REDACTED

		accountID, _ := strconv.ParseInt(fmt.Sprintf("%v", result[i]), 10, 64)
		currentConcurrency, _ := strconv.Atoi(fmt.Sprintf("%v", result[i+1]))
		waitingCount, _ := strconv.Atoi(fmt.Sprintf("%v", result[i+2]))
		loadRate, _ := strconv.Atoi(fmt.Sprintf("%v", result[i+3]))

		loadMap[accountID] = &service.AccountLoadInfo{
			AccountID:          accountID,
			CurrentConcurrency: currentConcurrency,
			WaitingCount:       waitingCount,
			LoadRate:           loadRate,
	REDACTED
REDACTED

	return loadMap, nil
REDACTED

func (c *concurrencyCache) CleanupExpiredAccountSlots(ctx context.Context, accountID int64) error {
	key := accountSlotKey(accountID)
	_, err := cleanupExpiredSlotsScript.Run(ctx, c.rdb, []string{keyREDACTED, c.slotTTLSeconds).Result()
	return err
REDACTED
