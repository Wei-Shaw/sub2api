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
	// 格式: concurrency:api_key:{apiKeyIDREDACTED
	apiKeySlotKeyPrefix = "concurrency:api_key:"
	// 等待队列计数器格式: concurrency:wait:{userIDREDACTED
	waitQueueKeyPrefix = "concurrency:wait:"
	// 账号级等待队列计数器格式: wait:account:{accountIDREDACTED
	accountWaitKeyPrefix = "wait:account:"

	// 默认槽位过期时间（分钟），可通过配置覆盖
	defaultSlotTTLMinutes = 15

	// 活跃索引用来替代后台任务全量 SCAN 槽位键。
	// member 是账号/用户 ID，score 是“预计仍需关注到”的 Redis Unix 秒时间戳。
	accountActiveIndexKey = "concurrency:account:active_index" // ZSET member=accountID, score=expireAtUnixSeconds
	userActiveIndexKey    = "concurrency:user:active_index"    // ZSET member=userID, score=expireAtUnixSeconds

	// 后台清理只按批处理索引候选，避免单次任务占用 Redis 太久。
	activeIndexCleanupBatchSize  = 1000
	activeIndexPipelineChunkSize = 500
)

var (
	// acquireScript 使用有序集合计数并在未达上限时添加槽位
	// 使用 Redis TIME 命令获取服务器时间，避免多实例时钟不同步问题
	// KEYS[1] = 有序集合键 (concurrency:account:{idREDACTED / concurrency:user:{idREDACTED)
	// ARGV[1] = maxConcurrency
	// ARGV[2] = TTL（秒）
	// ARGV[3] = requestID
	acquireScript = redis.NewScript(`
		-- Redis 3.2-4.x compat: opt into effects replication so redis.call('TIME')
		-- replicates correctly. No-op on Redis 5.0+ (effects replication is default).
		redis.replicate_commands()
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
		-- Redis 3.2-4.x compat: opt into effects replication so redis.call('TIME')
		-- replicates correctly. No-op on Redis 5.0+ (effects replication is default).
		redis.replicate_commands()
		local key = KEYS[1]
		local ttl = tonumber(ARGV[1])

		-- 使用 Redis 服务器时间
		local timeResult = redis.call('TIME')
		local now = tonumber(timeResult[1])
		local expireBefore = now - ttl

		redis.call('ZREMRANGEBYSCORE', key, '-inf', expireBefore)
		return redis.call('ZCARD', key)
	`)

	// trackSlotScript 记录 stats-only 槽位，不做并发上限判断。
	// KEYS[1] = 有序集合键
	// ARGV[1] = TTL（秒）
	// ARGV[2] = requestID
	trackSlotScript = redis.NewScript(`
		-- Redis 3.2-4.x compat: opt into effects replication so redis.call('TIME')
		-- replicates correctly. No-op on Redis 5.0+ (effects replication is default).
		redis.replicate_commands()
		local key = KEYS[1]
		local ttl = tonumber(ARGV[1])
		local requestID = ARGV[2]

		local timeResult = redis.call('TIME')
		local now = tonumber(timeResult[1])
		local expireBefore = now - ttl

		redis.call('ZREMRANGEBYSCORE', key, '-inf', expireBefore)
		redis.call('ZADD', key, now, requestID)
		redis.call('EXPIRE', key, ttl)
		return 1
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

	// cleanupExpiredSlotsScript 清理单个账号/用户有序集合中过期槽位
	// KEYS[1] = 有序集合键
	// ARGV[1] = TTL（秒）
	cleanupExpiredSlotsScript = redis.NewScript(`
		-- Redis 3.2-4.x compat: opt into effects replication so redis.call('TIME')
		-- replicates correctly. No-op on Redis 5.0+ (effects replication is default).
		redis.replicate_commands()
		local key = KEYS[1]
		local ttl = tonumber(ARGV[1])
		local timeResult = redis.call('TIME')
		local now = tonumber(timeResult[1])
		local expireBefore = now - ttl
		redis.call('ZREMRANGEBYSCORE', key, '-inf', expireBefore)
		if redis.call('ZCARD', key) == 0 then
			redis.call('DEL', key)
		else
			redis.call('EXPIRE', key, ttl)
		end
		return 1
	`)

	// startupCleanupSlotScript 清理单个槽位 key 中非当前进程前缀的成员，避免 Redis Cluster CROSSSLOT。
	// KEYS[1] 是有序集合键，ARGV[1] 是当前进程前缀，ARGV[2] 是槽位 TTL。
	startupCleanupSlotScript = redis.NewScript(`
		local key = KEYS[1]
		local activePrefix = ARGV[1]
		local slotTTL = tonumber(ARGV[2])
		local removed = 0
		local members = redis.call('ZRANGE', key, 0, -1)
		for _, member in ipairs(members) do
			if string.sub(member, 1, string.len(activePrefix)) ~= activePrefix then
				removed = removed + redis.call('ZREM', key, member)
			end
		end
		if redis.call('ZCARD', key) == 0 then
			redis.call('DEL', key)
		else
			redis.call('EXPIRE', key, slotTTL)
		end
		return removed
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

func apiKeySlotKey(apiKeyID int64) string {
	return fmt.Sprintf("%s%d", apiKeySlotKeyPrefix, apiKeyID)
REDACTED

func waitQueueKey(userID int64) string {
	return fmt.Sprintf("%s%d", waitQueueKeyPrefix, userID)
REDACTED

func accountWaitKey(accountID int64) string {
	return fmt.Sprintf("%s%d", accountWaitKeyPrefix, accountID)
REDACTED

// redisUnixSeconds 统一使用 Redis 服务器时间，避免多实例本地时钟漂移导致索引提前/延后过期。
func (c *concurrencyCache) redisUnixSeconds(ctx context.Context) (int64, error) {
	now, err := c.rdb.Time(ctx).Result()
	if err != nil {
		return 0, fmt.Errorf("redis TIME: %w", err)
REDACTED
	return now.Unix(), nil
REDACTED

func (c *concurrencyCache) touchAccountActiveIndex(ctx context.Context, accountID int64, ttlSeconds int) {
	c.touchActiveIndex(ctx, accountActiveIndexKey, accountID, ttlSeconds)
REDACTED

func (c *concurrencyCache) touchUserActiveIndex(ctx context.Context, userID int64, ttlSeconds int) {
	c.touchActiveIndex(ctx, userActiveIndexKey, userID, ttlSeconds)
REDACTED

// touchActiveIndex 是写路径上的轻量标记：主操作已成功时，尽力把 ID 放入活跃索引。
// 索引失败不影响并发槽位/等待队列本身，后续释放或清理会再次校正。
func (c *concurrencyCache) touchActiveIndex(ctx context.Context, indexKey string, id int64, ttlSeconds int) {
	if c == nil || c.rdb == nil || id <= 0 || ttlSeconds <= 0 {
		return
REDACTED
	now, err := c.redisUnixSeconds(ctx)
	if err != nil {
		return
REDACTED
	_ = c.rdb.ZAdd(ctx, indexKey, redis.Z{
		Score:  float64(now + int64(ttlSeconds)),
		Member: strconv.FormatInt(id, 10),
REDACTED).Err()
REDACTED

func (c *concurrencyCache) refreshAccountActiveIndex(ctx context.Context, accountID int64) {
	c.refreshActiveIndex(ctx, accountActiveIndexKey, accountID, accountSlotKey(accountID), accountWaitKey(accountID))
REDACTED

func (c *concurrencyCache) refreshUserActiveIndex(ctx context.Context, userID int64) {
	c.refreshActiveIndex(ctx, userActiveIndexKey, userID, userSlotKey(userID), waitQueueKey(userID))
REDACTED

// refreshActiveIndex 以 Redis 中的真实槽位/等待数为准重建索引状态。
// 释放槽位、等待计数减少、清理过期成员后都会调用它，防止索引残留。
func (c *concurrencyCache) refreshActiveIndex(ctx context.Context, indexKey string, id int64, slotKey, waitKey string) {
	if c == nil || c.rdb == nil || id <= 0 {
		return
REDACTED
	now, err := c.redisUnixSeconds(ctx)
	if err != nil {
		return
REDACTED

	load, err := c.readActiveLoadForKey(ctx, id, slotKey, waitKey, now)
	if err != nil {
		return
REDACTED
	member := strconv.FormatInt(id, 10)
	if load.slotCount == 0 && load.waitCount <= 0 {
		_ = c.rdb.ZRem(ctx, indexKey, member).Err()
		return
REDACTED

	ttlSeconds := c.activeIndexTTL(load.slotCount, load.waitCount)
	if ttlSeconds <= 0 {
		return
REDACTED
	_ = c.rdb.ZAdd(ctx, indexKey, redis.Z{
		Score:  float64(now + int64(ttlSeconds)),
		Member: member,
REDACTED).Err()
REDACTED

type activeIndexLoad struct {
	id        int64
	member    string
	slotCount int
	waitCount int
REDACTED

// activeIndexTTL 取槽位 TTL 与等待队列 TTL 中仍然需要关注的较大值。
// 只要并发槽位或等待计数还有负载，就保留索引；两者都为 0 时调用方会删除索引。
func (c *concurrencyCache) activeIndexTTL(slotCount int, waitCount int) int {
	ttlSeconds := 0
	if slotCount > 0 {
		ttlSeconds = c.slotTTLSeconds
REDACTED
	if waitCount > 0 && c.waitQueueTTLSeconds > ttlSeconds {
		ttlSeconds = c.waitQueueTTLSeconds
REDACTED
	return ttlSeconds
REDACTED

// readActiveLoadForKey 读取单个 ID 的当前负载，并顺手清理该槽位集合中的过期成员。
func (c *concurrencyCache) readActiveLoadForKey(ctx context.Context, id int64, slotKey, waitKey string, now int64) (activeIndexLoad, error) {
	cutoffTime := now - int64(c.slotTTLSeconds)
	pipe := c.rdb.Pipeline()
	pipe.ZRemRangeByScore(ctx, slotKey, "-inf", strconv.FormatInt(cutoffTime, 10))
	zcardCmd := pipe.ZCard(ctx, slotKey)
	getCmd := pipe.Get(ctx, waitKey)
	if _, err := pipe.Exec(ctx); err != nil && !errors.Is(err, redis.Nil) {
		return activeIndexLoad{REDACTED, fmt.Errorf("pipeline exec: %w", err)
REDACTED

	waitCount := 0
	if v, err := getCmd.Int(); err == nil && v > 0 {
		waitCount = v
REDACTED
	return activeIndexLoad{
		id:        id,
		member:    strconv.FormatInt(id, 10),
		slotCount: int(zcardCmd.Val()),
		waitCount: waitCount,
REDACTED, nil
REDACTED

// readAccountIndexLoads 批量读取账号索引候选的真实负载。
// 分块 Pipeline 可以减少 Redis 往返，同时避免一次 Pipeline 塞入过多命令。
func (c *concurrencyCache) readAccountIndexLoads(ctx context.Context, members []string, now int64) ([]activeIndexLoad, []string, error) {
	loads := make([]activeIndexLoad, 0, len(members))
	staleMembers := make([]string, 0)
	candidates := make([]activeIndexLoad, 0, len(members))
	for _, member := range members {
		id, err := strconv.ParseInt(member, 10, 64)
		if err != nil || id <= 0 {
			staleMembers = append(staleMembers, member)
			continue
	REDACTED
		candidates = append(candidates, activeIndexLoad{id: id, member: memberREDACTED)
REDACTED

	cutoffTime := now - int64(c.slotTTLSeconds)
	for start := 0; start < len(candidates); start += activeIndexPipelineChunkSize {
		end := start + activeIndexPipelineChunkSize
		if end > len(candidates) {
			end = len(candidates)
	REDACTED
		chunk := candidates[start:end]

		pipe := c.rdb.Pipeline()
		type accountCmd struct {
			activeIndexLoad
			zcardCmd *redis.IntCmd
			getCmd   *redis.StringCmd
	REDACTED
		cmds := make([]accountCmd, 0, len(chunk))
		for _, candidate := range chunk {
			slotKey := accountSlotKey(candidate.id)
			waitKey := accountWaitKey(candidate.id)
			pipe.ZRemRangeByScore(ctx, slotKey, "-inf", strconv.FormatInt(cutoffTime, 10))
			cmds = append(cmds, accountCmd{
				activeIndexLoad: candidate,
				zcardCmd:        pipe.ZCard(ctx, slotKey),
				getCmd:          pipe.Get(ctx, waitKey),
		REDACTED)
	REDACTED
		if _, err := pipe.Exec(ctx); err != nil && !errors.Is(err, redis.Nil) {
			return nil, nil, fmt.Errorf("pipeline exec: %w", err)
	REDACTED
		for _, cmd := range cmds {
			waitCount := 0
			if v, err := cmd.getCmd.Int(); err == nil && v > 0 {
				waitCount = v
		REDACTED
			loads = append(loads, activeIndexLoad{
				id:        cmd.id,
				member:    cmd.member,
				slotCount: int(cmd.zcardCmd.Val()),
				waitCount: waitCount,
		REDACTED)
	REDACTED
REDACTED

	return loads, staleMembers, nil
REDACTED

// removeActiveIndexMembers 清理无效 member；这是辅助索引的维护动作，调用方无需因为失败中断主流程。
func (c *concurrencyCache) removeActiveIndexMembers(ctx context.Context, indexKey string, members []string) {
	if len(members) == 0 {
		return
REDACTED
	args := make([]any, 0, len(members))
	for _, member := range members {
		args = append(args, member)
REDACTED
	_ = c.rdb.ZRem(ctx, indexKey, args...).Err()
REDACTED

// touchActiveIndexForLoad 根据已读取的真实负载刷新索引过期时间。
func (c *concurrencyCache) touchActiveIndexForLoad(ctx context.Context, indexKey string, load activeIndexLoad) {
	c.touchActiveIndex(ctx, indexKey, load.id, c.activeIndexTTL(load.slotCount, load.waitCount))
REDACTED

// Account slot operations

func (c *concurrencyCache) AcquireAccountSlot(ctx context.Context, accountID int64, maxConcurrency int, requestID string) (bool, error) {
	key := accountSlotKey(accountID)
	// 时间戳在 Lua 脚本内使用 Redis TIME 命令获取，确保多实例时钟一致
	result, err := acquireScript.Run(ctx, c.rdb, []string{keyREDACTED, maxConcurrency, c.slotTTLSeconds, requestID).Int()
	if err != nil {
		return false, err
REDACTED
	if result == 1 {
		// 成功占槽后标记活跃账号，后台清理即可从索引定位候选账号。
		c.touchAccountActiveIndex(ctx, accountID, c.slotTTLSeconds)
REDACTED
	return result == 1, nil
REDACTED

func (c *concurrencyCache) ReleaseAccountSlot(ctx context.Context, accountID int64, requestID string) error {
	key := accountSlotKey(accountID)
	if err := c.rdb.ZRem(ctx, key, requestID).Err(); err != nil {
		return err
REDACTED
	// 释放后用真实负载刷新索引；若没有槽位和等待计数，会移除索引 member。
	c.refreshAccountActiveIndex(ctx, accountID)
	return nil
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

func (c *concurrencyCache) GetAccountConcurrencyBatch(ctx context.Context, accountIDs []int64) (map[int64]int, error) {
	if len(accountIDs) == 0 {
		return map[int64]int{REDACTED, nil
REDACTED

	now, err := c.rdb.Time(ctx).Result()
	if err != nil {
		return nil, fmt.Errorf("redis TIME: %w", err)
REDACTED
	cutoffTime := now.Unix() - int64(c.slotTTLSeconds)

	pipe := c.rdb.Pipeline()
	type accountCmd struct {
		accountID int64
		zcardCmd  *redis.IntCmd
REDACTED
	cmds := make([]accountCmd, 0, len(accountIDs))
	for _, accountID := range accountIDs {
		slotKey := accountSlotKeyPrefix + strconv.FormatInt(accountID, 10)
		pipe.ZRemRangeByScore(ctx, slotKey, "-inf", strconv.FormatInt(cutoffTime, 10))
		cmds = append(cmds, accountCmd{
			accountID: accountID,
			zcardCmd:  pipe.ZCard(ctx, slotKey),
	REDACTED)
REDACTED

	if _, err := pipe.Exec(ctx); err != nil && !errors.Is(err, redis.Nil) {
		return nil, fmt.Errorf("pipeline exec: %w", err)
REDACTED

	result := make(map[int64]int, len(accountIDs))
	for _, cmd := range cmds {
		result[cmd.accountID] = int(cmd.zcardCmd.Val())
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
	if result == 1 {
		// 成功占槽后标记活跃用户，避免启动清理依赖全量 SCAN。
		c.touchUserActiveIndex(ctx, userID, c.slotTTLSeconds)
REDACTED
	return result == 1, nil
REDACTED

func (c *concurrencyCache) ReleaseUserSlot(ctx context.Context, userID int64, requestID string) error {
	key := userSlotKey(userID)
	if err := c.rdb.ZRem(ctx, key, requestID).Err(); err != nil {
		return err
REDACTED
	// 释放后按 Redis 中剩余负载修正索引状态。
	c.refreshUserActiveIndex(ctx, userID)
	return nil
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

func (c *concurrencyCache) TrackAPIKeySlot(ctx context.Context, apiKeyID int64, requestID string) error {
	key := apiKeySlotKey(apiKeyID)
	_, err := trackSlotScript.Run(ctx, c.rdb, []string{keyREDACTED, c.slotTTLSeconds, requestID).Result()
	return err
REDACTED

func (c *concurrencyCache) ReleaseAPIKeySlot(ctx context.Context, apiKeyID int64, requestID string) error {
	key := apiKeySlotKey(apiKeyID)
	return c.rdb.ZRem(ctx, key, requestID).Err()
REDACTED

func (c *concurrencyCache) GetAPIKeyConcurrencyBatch(ctx context.Context, apiKeyIDs []int64) (map[int64]int, error) {
	if len(apiKeyIDs) == 0 {
		return map[int64]int{REDACTED, nil
REDACTED

	now, err := c.rdb.Time(ctx).Result()
	if err != nil {
		return nil, fmt.Errorf("redis TIME: %w", err)
REDACTED
	cutoffTime := now.Unix() - int64(c.slotTTLSeconds)

	pipe := c.rdb.Pipeline()
	type apiKeyCmd struct {
		apiKeyID int64
		zcardCmd *redis.IntCmd
REDACTED
	cmds := make([]apiKeyCmd, 0, len(apiKeyIDs))
	for _, apiKeyID := range apiKeyIDs {
		slotKey := apiKeySlotKeyPrefix + strconv.FormatInt(apiKeyID, 10)
		pipe.ZRemRangeByScore(ctx, slotKey, "-inf", strconv.FormatInt(cutoffTime, 10))
		cmds = append(cmds, apiKeyCmd{
			apiKeyID: apiKeyID,
			zcardCmd: pipe.ZCard(ctx, slotKey),
	REDACTED)
REDACTED

	if _, err := pipe.Exec(ctx); err != nil && !errors.Is(err, redis.Nil) {
		return nil, fmt.Errorf("pipeline exec: %w", err)
REDACTED

	result := make(map[int64]int, len(apiKeyIDs))
	for _, cmd := range cmds {
		result[cmd.apiKeyID] = int(cmd.zcardCmd.Val())
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
	if result == 1 {
		// 等待队列也会让用户保持“活跃”，否则槽位为 0 时后台任务可能漏看等待计数。
		c.touchUserActiveIndex(ctx, userID, c.waitQueueTTLSeconds)
REDACTED
	return result == 1, nil
REDACTED

func (c *concurrencyCache) DecrementWaitCount(ctx context.Context, userID int64) error {
	key := waitQueueKey(userID)
	_, err := decrementWaitScript.Run(ctx, c.rdb, []string{keyREDACTED).Result()
	if err == nil {
		// 等待数减少后重新判断是否还需要保留索引。
		c.refreshUserActiveIndex(ctx, userID)
REDACTED
	return err
REDACTED

// Account wait queue operations

func (c *concurrencyCache) IncrementAccountWaitCount(ctx context.Context, accountID int64, maxWait int) (bool, error) {
	key := accountWaitKey(accountID)
	result, err := incrementAccountWaitScript.Run(ctx, c.rdb, []string{keyREDACTED, maxWait, c.waitQueueTTLSeconds).Int()
	if err != nil {
		return false, err
REDACTED
	if result == 1 {
		// 账号级等待队列同样写入账号活跃索引，供负载查询和清理任务使用。
		c.touchAccountActiveIndex(ctx, accountID, c.waitQueueTTLSeconds)
REDACTED
	return result == 1, nil
REDACTED

func (c *concurrencyCache) DecrementAccountWaitCount(ctx context.Context, accountID int64) error {
	key := accountWaitKey(accountID)
	_, err := decrementWaitScript.Run(ctx, c.rdb, []string{keyREDACTED).Result()
	if err == nil {
		// 等待计数归零后索引需要同步删除，避免后台任务反复处理空账号。
		c.refreshAccountActiveIndex(ctx, accountID)
REDACTED
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

	// 使用 Pipeline 替代 Lua 脚本，兼容 Redis Cluster（Lua 内动态拼 key 会 CROSSSLOT）。
	// 每个账号执行 3 个命令：ZREMRANGEBYSCORE（清理过期）、ZCARD（并发数）、GET（等待数）。
	now, err := c.rdb.Time(ctx).Result()
	if err != nil {
		return nil, fmt.Errorf("redis TIME: %w", err)
REDACTED
	cutoffTime := now.Unix() - int64(c.slotTTLSeconds)

	pipe := c.rdb.Pipeline()

	type accountCmds struct {
		id             int64
		maxConcurrency int
		zcardCmd       *redis.IntCmd
		getCmd         *redis.StringCmd
REDACTED
	cmds := make([]accountCmds, 0, len(accounts))
	for _, acc := range accounts {
		slotKey := accountSlotKeyPrefix + strconv.FormatInt(acc.ID, 10)
		waitKey := accountWaitKeyPrefix + strconv.FormatInt(acc.ID, 10)
		pipe.ZRemRangeByScore(ctx, slotKey, "-inf", strconv.FormatInt(cutoffTime, 10))
		ac := accountCmds{
			id:             acc.ID,
			maxConcurrency: acc.MaxConcurrency,
			zcardCmd:       pipe.ZCard(ctx, slotKey),
			getCmd:         pipe.Get(ctx, waitKey),
	REDACTED
		cmds = append(cmds, ac)
REDACTED

	if _, err := pipe.Exec(ctx); err != nil && !errors.Is(err, redis.Nil) {
		return nil, fmt.Errorf("pipeline exec: %w", err)
REDACTED

	loadMap := make(map[int64]*service.AccountLoadInfo, len(accounts))
	for _, ac := range cmds {
		currentConcurrency := int(ac.zcardCmd.Val())
		waitingCount := 0
		if v, err := ac.getCmd.Int(); err == nil {
			waitingCount = v
	REDACTED
		loadRate := 0
		if ac.maxConcurrency > 0 {
			loadRate = (currentConcurrency + waitingCount) * 100 / ac.maxConcurrency
	REDACTED
		loadMap[ac.id] = &service.AccountLoadInfo{
			AccountID:          ac.id,
			CurrentConcurrency: currentConcurrency,
			WaitingCount:       waitingCount,
			LoadRate:           loadRate,
	REDACTED
REDACTED

	return loadMap, nil
REDACTED

func (c *concurrencyCache) GetUsersLoadBatch(ctx context.Context, users []service.UserWithConcurrency) (map[int64]*service.UserLoadInfo, error) {
	if len(users) == 0 {
		return map[int64]*service.UserLoadInfo{REDACTED, nil
REDACTED

	// 使用 Pipeline 替代 Lua 脚本，兼容 Redis Cluster。
	now, err := c.rdb.Time(ctx).Result()
	if err != nil {
		return nil, fmt.Errorf("redis TIME: %w", err)
REDACTED
	cutoffTime := now.Unix() - int64(c.slotTTLSeconds)

	pipe := c.rdb.Pipeline()

	type userCmds struct {
		id             int64
		maxConcurrency int
		zcardCmd       *redis.IntCmd
		getCmd         *redis.StringCmd
REDACTED
	cmds := make([]userCmds, 0, len(users))
	for _, u := range users {
		slotKey := userSlotKeyPrefix + strconv.FormatInt(u.ID, 10)
		waitKey := waitQueueKeyPrefix + strconv.FormatInt(u.ID, 10)
		pipe.ZRemRangeByScore(ctx, slotKey, "-inf", strconv.FormatInt(cutoffTime, 10))
		uc := userCmds{
			id:             u.ID,
			maxConcurrency: u.MaxConcurrency,
			zcardCmd:       pipe.ZCard(ctx, slotKey),
			getCmd:         pipe.Get(ctx, waitKey),
	REDACTED
		cmds = append(cmds, uc)
REDACTED

	if _, err := pipe.Exec(ctx); err != nil && !errors.Is(err, redis.Nil) {
		return nil, fmt.Errorf("pipeline exec: %w", err)
REDACTED

	loadMap := make(map[int64]*service.UserLoadInfo, len(users))
	for _, uc := range cmds {
		currentConcurrency := int(uc.zcardCmd.Val())
		waitingCount := 0
		if v, err := uc.getCmd.Int(); err == nil {
			waitingCount = v
	REDACTED
		loadRate := 0
		if uc.maxConcurrency > 0 {
			loadRate = (currentConcurrency + waitingCount) * 100 / uc.maxConcurrency
	REDACTED
		loadMap[uc.id] = &service.UserLoadInfo{
			UserID:             uc.id,
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
	if err == nil {
		// 单账号清理后同步索引，保持后台批量清理的候选集准确。
		c.refreshAccountActiveIndex(ctx, accountID)
REDACTED
	return err
REDACTED

// GetActiveAccountLoadMap 只读取活跃账号索引中的账号负载。
// 这是给热路径使用的轻量视图，避免为获取全局账号负载而扫描所有槽位键。
func (c *concurrencyCache) GetActiveAccountLoadMap(ctx context.Context) (map[int64]*service.AccountLoadInfo, error) {
	now, err := c.redisUnixSeconds(ctx)
	if err != nil {
		return nil, err
REDACTED
	if err := c.rdb.ZRemRangeByScore(ctx, accountActiveIndexKey, "-inf", strconv.FormatInt(now, 10)).Err(); err != nil {
		return nil, fmt.Errorf("cleanup account active index: %w", err)
REDACTED
	members, err := c.rdb.ZRangeByScore(ctx, accountActiveIndexKey, &redis.ZRangeBy{
		Min: strconv.FormatInt(now+1, 10),
		Max: "+inf",
REDACTED).Result()
	if err != nil {
		return nil, fmt.Errorf("read account active index: %w", err)
REDACTED

	loads, staleMembers, err := c.readAccountIndexLoads(ctx, members, now)
	if err != nil {
		return nil, err
REDACTED

	loadMap := make(map[int64]*service.AccountLoadInfo, len(loads))
	for _, load := range loads {
		if load.slotCount == 0 && load.waitCount <= 0 {
			// 索引候选已没有实际负载，删除 member 而不是返回空负载。
			staleMembers = append(staleMembers, load.member)
			continue
	REDACTED
		loadMap[load.id] = &service.AccountLoadInfo{
			AccountID:          load.id,
			CurrentConcurrency: load.slotCount,
			WaitingCount:       load.waitCount,
	REDACTED
		c.touchActiveIndexForLoad(ctx, accountActiveIndexKey, load)
REDACTED
	c.removeActiveIndexMembers(ctx, accountActiveIndexKey, staleMembers)
	return loadMap, nil
REDACTED

// CleanupExpiredAccountSlotKeys 只处理索引中过期的账号候选。
// 若候选仍有真实负载，则刷新索引；若没有负载，则移除索引 member。
func (c *concurrencyCache) CleanupExpiredAccountSlotKeys(ctx context.Context) error {
	now, err := c.redisUnixSeconds(ctx)
	if err != nil {
		return err
REDACTED
	members, err := c.rdb.ZRangeByScore(ctx, accountActiveIndexKey, &redis.ZRangeBy{
		Min:   "-inf",
		Max:   strconv.FormatInt(now, 10),
		Count: activeIndexCleanupBatchSize,
REDACTED).Result()
	if err != nil {
		return fmt.Errorf("read expired account active index: %w", err)
REDACTED

	loads, staleMembers, err := c.readAccountIndexLoads(ctx, members, now)
	if err != nil {
		return err
REDACTED
	for _, load := range loads {
		if load.slotCount == 0 && load.waitCount <= 0 {
			// 真实槽位和等待数都为空，说明这个索引 member 已经完成使命。
			staleMembers = append(staleMembers, load.member)
			continue
	REDACTED
		c.touchActiveIndexForLoad(ctx, accountActiveIndexKey, load)
REDACTED
	c.removeActiveIndexMembers(ctx, accountActiveIndexKey, staleMembers)
	return nil
REDACTED

// CleanupStaleProcessSlots 启动时清理非当前进程前缀的槽位。
// 清理范围来自活跃索引，避免在 Redis 上 SCAN 全部 concurrency:* 键。
// API Key 槽位（concurrency:api_key:*）是 stats-only 数据：每次 Track/读取都会按分数
// 裁剪过期成员，key 自带 TTL，可在一个 slot TTL 内自愈，因此不参与启动清理。
func (c *concurrencyCache) CleanupStaleProcessSlots(ctx context.Context, activeRequestPrefix string) error {
	if activeRequestPrefix == "" {
		return nil
REDACTED
	now, err := c.redisUnixSeconds(ctx)
	if err != nil {
		return err
REDACTED

	accountMembers, err := c.activeIndexMembers(ctx, accountActiveIndexKey, now)
	if err != nil {
		return err
REDACTED
	if err := c.cleanupStaleProcessSlotsForIndex(ctx, accountActiveIndexKey, accountMembers, activeRequestPrefix, accountSlotKey, accountWaitKey, c.refreshAccountActiveIndex); err != nil {
		return err
REDACTED

	userMembers, err := c.activeIndexMembers(ctx, userActiveIndexKey, now)
	if err != nil {
		return err
REDACTED
	return c.cleanupStaleProcessSlotsForIndex(ctx, userActiveIndexKey, userMembers, activeRequestPrefix, userSlotKey, waitQueueKey, c.refreshUserActiveIndex)
REDACTED

// activeIndexMembers 只返回当前仍未过期的索引 member；过期 member 由对应清理任务处理。
func (c *concurrencyCache) activeIndexMembers(ctx context.Context, indexKey string, now int64) ([]string, error) {
	members, err := c.rdb.ZRangeByScore(ctx, indexKey, &redis.ZRangeBy{
		Min: strconv.FormatInt(now+1, 10),
		Max: "+inf",
REDACTED).Result()
	if err != nil {
		return nil, fmt.Errorf("read active index %s: %w", indexKey, err)
REDACTED
	return members, nil
REDACTED

// cleanupStaleProcessSlotsForIndex 逐个处理索引中的账号/用户。
// Lua 脚本一次只碰一个槽位 key，兼容 Redis Cluster，随后删除重启后已失效的等待计数。
func (c *concurrencyCache) cleanupStaleProcessSlotsForIndex(
	ctx context.Context,
	indexKey string,
	members []string,
	activeRequestPrefix string,
	slotKeyForID func(int64) string,
	waitKeyForID func(int64) string,
	refreshIndex func(context.Context, int64),
) error {
	staleMembers := make([]string, 0)
	for _, member := range members {
		id, err := strconv.ParseInt(member, 10, 64)
		if err != nil || id <= 0 {
			staleMembers = append(staleMembers, member)
			continue
	REDACTED

		if _, err := startupCleanupSlotScript.Run(ctx, c.rdb, []string{slotKeyForID(id)REDACTED, activeRequestPrefix, c.slotTTLSeconds).Result(); err != nil {
			return fmt.Errorf("cleanup stale process slots %s: %w", slotKeyForID(id), err)
	REDACTED
		if err := c.rdb.Del(ctx, waitKeyForID(id)).Err(); err != nil {
			return fmt.Errorf("delete stale wait key %s: %w", waitKeyForID(id), err)
	REDACTED
		refreshIndex(ctx, id)
REDACTED
	c.removeActiveIndexMembers(ctx, indexKey, staleMembers)
	return nil
REDACTED
