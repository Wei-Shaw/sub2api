package repository

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"sort"
	"strconv"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/application/service"
	"github.com/redis/go-redis/v9"
)

const (
	schedulerBucketSetKey          = "sched:buckets"
	schedulerOutboxWatermarkKey    = "sched:outbox:watermark"
	schedulerAccountPrefix         = "sched:acc:"
	schedulerAccountMetaPrefix     = "sched:meta:"
	schedulerAccountLastUsedPrefix = "sched:acc:last_used:"
	schedulerActivePrefix          = "sched:active:"
	schedulerReadyPrefix           = "sched:ready:"
	schedulerVersionPrefix         = "sched:ver:"
	schedulerEpochPrefix           = "sched:epoch:"
	schedulerRetiredPrefix         = "sched:retired:"
	schedulerSnapshotPrefix        = "sched:"
	schedulerLockPrefix            = "sched:lock:"
	schedulerEngineKey             = "sched:engine"
	schedulerEngineStatusKey       = "sched:engine:status"
	schedulerEngineErrorKey        = "sched:engine:error"
	schedulerCandidateLimitKey     = "sched:v2:candidate-limit"
	schedulerScanLimitKey          = "sched:v2:scan-limit"
	schedulerCandidatePrefix       = "sched:v2:cand:"
	schedulerCandidateReady        = "sched:v2:ready:"
	schedulerCandidateCount        = "sched:v2:count:"
	schedulerCandidateBuckets      = "sched:v2:buckets"
	schedulerAccountBuckets        = "sched:v2:acc-buckets:"

	defaultSchedulerSnapshotMGetChunkSize  = 128
	defaultSchedulerSnapshotWriteChunkSize = 256
	schedulerLastUsedUpdateChunkSize       = 256

	// snapshotGraceTTLSeconds 旧快照过期的宽限期（秒）。
	// 替代立即 DEL，让正在读取旧版本的 reader 有足够时间完成 ZRANGE。
	snapshotGraceTTLSeconds = 60
)

const (
	schedulerGroupLifecycleLockPrefix      = "sched:group:lifecycle-lock:"
	schedulerGroupLifecycleOwnerTokenBytes = 16
)

var updateSchedulerLastUsedScript = redis.NewScript(`
local updated = 0
for index = 1, #ARGV do
    local key_index = (index - 1) * 2 + 1
    local candidate = tonumber(ARGV[index])
    if candidate == nil then
        return redis.error_reply('invalid last_used value')
    end
    if redis.call('EXISTS', KEYS[key_index]) == 1 then
        local current = tonumber(redis.call('GET', KEYS[key_index + 1]))
        if current == nil or candidate > current then
            redis.call('SET', KEYS[key_index + 1], ARGV[index])
            updated = updated + 1
        end
    end
end
return updated
`)

var (
	// epoch 标识 bucket writer 的代际，retired key 是持久退休标记。
	// Capture、allocate、activate 都在 Lua 内同时校验两者：-1 表示已退休，-2 表示 epoch 无效或与 token 代际不匹配；
	// allocate 与 activate 的双重校验可拦截快照写入期间发生的 Retire。
	// Retire 仅在首次退休时推进 epoch，Reopen 只清除标记并沿用该代际，因此重复调用保持幂等。
	captureBucketWriteTokenScript = redis.NewScript(`
if redis.call('EXISTS', KEYS[2]) == 1 then
    return -1
end

local currentEpoch = redis.call('GET', KEYS[1])
if currentEpoch == false then
    redis.call('SET', KEYS[1], '1')
    return 1
end

local parsedEpoch = tonumber(currentEpoch)
if parsedEpoch == nil or parsedEpoch < 1 then
    return -2
end
return parsedEpoch
`)

	allocateSnapshotVersionScript = redis.NewScript(`
if redis.call('EXISTS', KEYS[2]) == 1 then
    return -1
end

local currentEpoch = tonumber(redis.call('GET', KEYS[1]))
local expectedEpoch = tonumber(ARGV[1])
if currentEpoch == nil or expectedEpoch == nil or currentEpoch ~= expectedEpoch then
    return -2
end

return redis.call('INCR', KEYS[3])
`)

	retireBucketScript = redis.NewScript(`
local retired = redis.call('GET', KEYS[2])
local currentEpoch = tonumber(redis.call('GET', KEYS[1])) or 0

if retired == false then
    currentEpoch = currentEpoch + 1
    if currentEpoch < 1 then
        currentEpoch = 1
    end
    redis.call('SET', KEYS[1], tostring(currentEpoch))
    redis.call('SET', KEYS[2], tostring(currentEpoch))
elseif currentEpoch < 1 then
    currentEpoch = tonumber(retired) or 1
    redis.call('SET', KEYS[1], tostring(currentEpoch))
end

redis.call('SREM', KEYS[3], ARGV[1])
local currentActive = redis.call('GET', KEYS[5])
if currentActive ~= false then
    redis.call('EXPIRE', ARGV[2] .. currentActive, tonumber(ARGV[3]))
end
redis.call('DEL', KEYS[4], KEYS[5])

local candidateIDs = redis.call('ZRANGE', KEYS[8], 0, -1)
for _, id in ipairs(candidateIDs) do
    redis.call('SREM', ARGV[4] .. id, ARGV[1])
end
redis.call('DEL', KEYS[6], KEYS[7], KEYS[8])
redis.call('SREM', KEYS[9], ARGV[1])
return currentEpoch
`)

	reopenBucketScript = redis.NewScript(`
local currentEpochRaw = redis.call('GET', KEYS[1])
local currentEpoch = tonumber(currentEpochRaw)
local retiredEpochRaw = redis.call('GET', KEYS[2])

if retiredEpochRaw == false then
    if currentEpochRaw == false then
        redis.call('SET', KEYS[1], '1')
        return 1
    end
    if currentEpoch == nil or currentEpoch < 1 then
        return -2
    end
    return currentEpoch
end

local retiredEpoch = tonumber(retiredEpochRaw)
if retiredEpoch == nil or retiredEpoch < 1 then
    return -2
end
if currentEpoch == nil or currentEpoch < retiredEpoch then
    currentEpoch = retiredEpoch
end

redis.call('SET', KEYS[1], tostring(currentEpoch))
redis.call('DEL', KEYS[2])
redis.call('SREM', KEYS[3], ARGV[1])
local currentActive = redis.call('GET', KEYS[5])
if currentActive ~= false then
    redis.call('EXPIRE', ARGV[2] .. currentActive, tonumber(ARGV[3]))
end
redis.call('DEL', KEYS[4], KEYS[5])

local candidateIDs = redis.call('ZRANGE', KEYS[8], 0, -1)
for _, id in ipairs(candidateIDs) do
    redis.call('SREM', ARGV[4] .. id, ARGV[1])
end
redis.call('DEL', KEYS[6], KEYS[7], KEYS[8])
redis.call('SREM', KEYS[9], ARGV[1])
return currentEpoch
`)

	// 释放租约必须先比较所有者令牌再删除，过期持有者的延迟释放不能误删继任租约。
	releaseGroupLifecycleLeaseScript = redis.NewScript(`
if redis.call('GET', KEYS[1]) == ARGV[1] then
    return redis.call('DEL', KEYS[1])
end
return 0
`)

	// activateSnapshotScript 原子 CAS 切换快照版本。
	// 仅当新版本号 >= 当前激活版本时才切换，防止并发写入导致版本回滚。
	// 旧快照使用 EXPIRE 设置宽限期而非立即 DEL，避免与 reader 竞态。
	//
	// KEYS[1] = activeKey     (sched:active:{bucket})
	// KEYS[2] = readyKey      (sched:ready:{bucket})
	// KEYS[3] = bucketSetKey  (sched:buckets)
	// KEYS[4] = snapshotKey   (新写入的快照 key)
	// KEYS[5] = epochKey
	// KEYS[6] = retiredKey
	// ARGV[1] = 新版本号字符串
	// ARGV[2] = bucket 字符串 (用于 SADD)
	// ARGV[3] = 快照 key 前缀 (用于构造旧快照 key)
	// ARGV[4] = 宽限期 TTL 秒数
	// ARGV[5] = writer epoch
	//
	// 返回 1 = 已激活, 0 = 版本过旧未激活
	activateSnapshotScript = redis.NewScript(`
if redis.call('EXISTS', KEYS[6]) == 1 then
    redis.call('DEL', KEYS[4])
    return -1
end

local currentEpoch = tonumber(redis.call('GET', KEYS[5]))
local expectedEpoch = tonumber(ARGV[5])
if currentEpoch == nil or expectedEpoch == nil or currentEpoch ~= expectedEpoch then
    redis.call('DEL', KEYS[4])
    return -2
end

local currentActive = redis.call('GET', KEYS[1])
local newVersion = tonumber(ARGV[1])

if currentActive ~= false then
	local curVersion = tonumber(currentActive)
	if curVersion and newVersion < curVersion then
		redis.call('DEL', KEYS[4])
		return 0
	end
end

redis.call('SET', KEYS[1], ARGV[1])
redis.call('SET', KEYS[2], '1')
redis.call('SADD', KEYS[3], ARGV[2])

if currentActive ~= false and currentActive ~= ARGV[1] then
	redis.call('EXPIRE', ARGV[3] .. currentActive, tonumber(ARGV[4]))
end

return 1
`)

	upsertCandidateScript = redis.NewScript(`
if redis.call('GET', KEYS[3]) ~= ARGV[3] or redis.call('EXISTS', KEYS[2]) == 0 then
	return -1
end
local added = redis.call('ZADD', KEYS[1], ARGV[1], ARGV[2])
if added > 0 then
	local current = tonumber(redis.call('GET', KEYS[2]) or '0')
	redis.call('SET', KEYS[2], tostring(current + added))
end
return added
`)

	removeCandidateScript = redis.NewScript(`
if redis.call('GET', KEYS[3]) ~= ARGV[2] or redis.call('EXISTS', KEYS[2]) == 0 then
	return -1
end
local removed = redis.call('ZREM', KEYS[1], ARGV[1])
if removed > 0 then
	local current = tonumber(redis.call('GET', KEYS[2]) or '0')
	local next = current - removed
	if next < 0 then
		next = 0
	end
	redis.call('SET', KEYS[2], tostring(next))
end
return removed
`)

	readCandidatePageScript = redis.NewScript(`
if redis.call('GET', KEYS[1]) ~= ARGV[3] then
	return {0}
end
local count = tonumber(redis.call('GET', KEYS[2]))
if count == nil or count < 0 then
	return {0}
end
local offset = tonumber(ARGV[1])
local limit = tonumber(ARGV[2])
if count == 0 or offset >= count then
	return {1, count}
end
local ids = redis.call('ZRANGE', KEYS[3], offset, offset + limit - 1)
if #ids == 0 then
	return {0}
end
local result = {1, count}
for _, id in ipairs(ids) do
	table.insert(result, id)
end
return result
`)

	activateCandidateIndexScript = redis.NewScript(`
if redis.call('EXISTS', KEYS[2]) == 1 then
	redis.call('DEL', KEYS[3])
	return -1
end
local currentEpoch = tonumber(redis.call('GET', KEYS[1]))
local expectedEpoch = tonumber(ARGV[1])
if currentEpoch == nil or expectedEpoch == nil or currentEpoch ~= expectedEpoch then
	redis.call('DEL', KEYS[3])
	return -2
end

local oldIDs = redis.call('ZRANGE', KEYS[4], 0, -1)
for _, id in ipairs(oldIDs) do
	redis.call('SREM', ARGV[4] .. id, ARGV[2])
end
if redis.call('EXISTS', KEYS[3]) == 1 then
	redis.call('RENAME', KEYS[3], KEYS[4])
else
	redis.call('DEL', KEYS[4])
end
local newIDs = redis.call('ZRANGE', KEYS[4], 0, -1)
for _, id in ipairs(newIDs) do
	redis.call('SADD', ARGV[4] .. id, ARGV[2])
end
redis.call('SET', KEYS[5], tostring(#newIDs))
redis.call('SET', KEYS[6], ARGV[3])
redis.call('SADD', KEYS[7], ARGV[2])
redis.call('SADD', KEYS[8], ARGV[2])
return #newIDs
`)

	compareAndSetSchedulerEngineScript = redis.NewScript(`
local currentEngine = redis.call('GET', KEYS[1]) or ''
local currentStatus = redis.call('GET', KEYS[2]) or ''
if currentEngine ~= ARGV[1] or currentStatus ~= ARGV[2] then
	return 0
end
redis.call('SET', KEYS[1], ARGV[3])
redis.call('SET', KEYS[2], ARGV[4])
redis.call('SET', KEYS[3], ARGV[5])
return 1
`)
)

type schedulerCache struct {
	rdb            *redis.Client
	mgetChunkSize  int
	writeChunkSize int
}

func NewSchedulerCache(rdb *redis.Client) service.SchedulerCache {
	return newSchedulerCacheWithChunkSizes(rdb, defaultSchedulerSnapshotMGetChunkSize, defaultSchedulerSnapshotWriteChunkSize)
}

func newSchedulerCacheWithChunkSizes(rdb *redis.Client, mgetChunkSize, writeChunkSize int) service.SchedulerCache {
	if mgetChunkSize <= 0 {
		mgetChunkSize = defaultSchedulerSnapshotMGetChunkSize
	}
	if writeChunkSize <= 0 {
		writeChunkSize = defaultSchedulerSnapshotWriteChunkSize
	}
	return &schedulerCache{
		rdb:            rdb,
		mgetChunkSize:  mgetChunkSize,
		writeChunkSize: writeChunkSize,
	}
}

func (c *schedulerCache) GetSnapshot(ctx context.Context, bucket service.SchedulerBucket) ([]*service.Account, bool, error) {
	readyKey := schedulerBucketKey(schedulerReadyPrefix, bucket)
	readyVal, err := c.rdb.Get(ctx, readyKey).Result()
	if err == redis.Nil {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	if readyVal != "1" {
		return nil, false, nil
	}

	activeKey := schedulerBucketKey(schedulerActivePrefix, bucket)
	activeVal, err := c.rdb.Get(ctx, activeKey).Result()
	if err == redis.Nil {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}

	snapshotKey := schedulerSnapshotKey(bucket, activeVal)
	ids, err := c.rdb.ZRange(ctx, snapshotKey, 0, -1).Result()
	if err != nil {
		return nil, false, err
	}
	if len(ids) == 0 {
		// 空快照视为缓存未命中，触发数据库回退查询
		// 这解决了新分组创建后立即绑定账号时的竞态条件问题
		return nil, false, nil
	}

	keys := make([]string, 0, len(ids))
	lastUsedKeys := make([]string, 0, len(ids))
	for _, id := range ids {
		keys = append(keys, schedulerAccountMetaKey(id))
		lastUsedKeys = append(lastUsedKeys, schedulerLastUsedKey(id))
	}
	values, err := c.mgetChunked(ctx, keys)
	if err != nil {
		return nil, false, err
	}
	lastUsedValues, err := c.mgetChunked(ctx, lastUsedKeys)
	if err != nil {
		return nil, false, err
	}

	accounts := make([]*service.Account, 0, len(values))
	for i, val := range values {
		if val == nil {
			return nil, false, nil
		}
		account, err := decodeCachedAccount(val)
		if err != nil {
			return nil, false, err
		}
		if err := applySchedulerLastUsed(account, lastUsedValues[i]); err != nil {
			return nil, false, err
		}
		accounts = append(accounts, account)
	}

	return accounts, true, nil
}

func (c *schedulerCache) CaptureBucketWriteToken(ctx context.Context, bucket service.SchedulerBucket) (service.SchedulerBucketWriteToken, error) {
	result, err := captureBucketWriteTokenScript.Run(ctx, c.rdb, []string{
		schedulerBucketKey(schedulerEpochPrefix, bucket),
		schedulerBucketKey(schedulerRetiredPrefix, bucket),
	}).Int64()
	if err != nil {
		return service.SchedulerBucketWriteToken{}, err
	}
	if err := schedulerBucketWriteResultError(result, bucket); err != nil {
		return service.SchedulerBucketWriteToken{}, err
	}
	return service.SchedulerBucketWriteToken{Bucket: bucket, Epoch: result}, nil
}

func (c *schedulerCache) RetireBucket(ctx context.Context, bucket service.SchedulerBucket) error {
	snapshotKeyPrefix := fmt.Sprintf("%s%d:%s:%s:v", schedulerSnapshotPrefix, bucket.GroupID, bucket.Platform, bucket.Mode)
	result, err := retireBucketScript.Run(ctx, c.rdb, []string{
		schedulerBucketKey(schedulerEpochPrefix, bucket),
		schedulerBucketKey(schedulerRetiredPrefix, bucket),
		schedulerBucketSetKey,
		schedulerBucketKey(schedulerReadyPrefix, bucket),
		schedulerBucketKey(schedulerActivePrefix, bucket),
		schedulerBucketKey(schedulerCandidateReady, bucket),
		schedulerBucketKey(schedulerCandidateCount, bucket),
		schedulerCandidateKey(bucket),
		schedulerCandidateBuckets,
	}, bucket.String(), snapshotKeyPrefix, snapshotGraceTTLSeconds, schedulerAccountBuckets).Int64()
	if err != nil {
		return err
	}
	if result < 1 {
		return fmt.Errorf("retire scheduler bucket %s returned invalid epoch %d", bucket.String(), result)
	}
	return nil
}

func (c *schedulerCache) ReopenBucket(ctx context.Context, bucket service.SchedulerBucket) (service.SchedulerBucketWriteToken, error) {
	snapshotKeyPrefix := fmt.Sprintf("%s%d:%s:%s:v", schedulerSnapshotPrefix, bucket.GroupID, bucket.Platform, bucket.Mode)
	result, err := reopenBucketScript.Run(ctx, c.rdb, []string{
		schedulerBucketKey(schedulerEpochPrefix, bucket),
		schedulerBucketKey(schedulerRetiredPrefix, bucket),
		schedulerBucketSetKey,
		schedulerBucketKey(schedulerReadyPrefix, bucket),
		schedulerBucketKey(schedulerActivePrefix, bucket),
		schedulerBucketKey(schedulerCandidateReady, bucket),
		schedulerBucketKey(schedulerCandidateCount, bucket),
		schedulerCandidateKey(bucket),
		schedulerCandidateBuckets,
	}, bucket.String(), snapshotKeyPrefix, snapshotGraceTTLSeconds, schedulerAccountBuckets).Int64()
	if err != nil {
		return service.SchedulerBucketWriteToken{}, err
	}
	if err := schedulerBucketWriteResultError(result, bucket); err != nil {
		return service.SchedulerBucketWriteToken{}, err
	}
	return service.SchedulerBucketWriteToken{Bucket: bucket, Epoch: result}, nil
}

func (c *schedulerCache) TryAcquireGroupLifecycleLease(ctx context.Context, groupID int64, ttl time.Duration) (service.SchedulerGroupLifecycleLease, bool, error) {
	if groupID <= 0 {
		return service.SchedulerGroupLifecycleLease{}, false, fmt.Errorf("%w: group id must be positive", service.ErrSchedulerGroupLifecycleLeaseInvalid)
	}
	if ttl <= 0 {
		return service.SchedulerGroupLifecycleLease{}, false, fmt.Errorf("%w: ttl must be positive", service.ErrSchedulerGroupLifecycleLeaseInvalid)
	}
	ownerToken, err := newSchedulerGroupLifecycleOwnerToken()
	if err != nil {
		return service.SchedulerGroupLifecycleLease{}, false, err
	}
	acquired, err := c.rdb.SetNX(ctx, schedulerGroupLifecycleLockKey(groupID), ownerToken, ttl).Result()
	if err != nil {
		return service.SchedulerGroupLifecycleLease{}, false, err
	}
	if !acquired {
		return service.SchedulerGroupLifecycleLease{}, false, nil
	}
	return service.SchedulerGroupLifecycleLease{GroupID: groupID, OwnerToken: ownerToken}, true, nil
}

func (c *schedulerCache) ReleaseGroupLifecycleLease(ctx context.Context, lease service.SchedulerGroupLifecycleLease) error {
	if !lease.ValidFor(lease.GroupID) {
		return service.ErrSchedulerGroupLifecycleLeaseInvalid
	}
	result, err := releaseGroupLifecycleLeaseScript.Run(
		ctx,
		c.rdb,
		[]string{schedulerGroupLifecycleLockKey(lease.GroupID)},
		lease.OwnerToken,
	).Int64()
	if err != nil {
		return err
	}
	if result == 0 {
		return fmt.Errorf("%w: group=%d", service.ErrSchedulerGroupLifecycleLeaseLost, lease.GroupID)
	}
	if result != 1 {
		return fmt.Errorf("release scheduler group lifecycle lease returned %d", result)
	}
	return nil
}

func newSchedulerGroupLifecycleOwnerToken() (string, error) {
	raw := make([]byte, schedulerGroupLifecycleOwnerTokenBytes)
	if _, err := rand.Read(raw); err != nil {
		return "", fmt.Errorf("generate scheduler group lifecycle owner token: %w", err)
	}
	return hex.EncodeToString(raw), nil
}

func (c *schedulerCache) SetSnapshot(ctx context.Context, bucket service.SchedulerBucket, token service.SchedulerBucketWriteToken, accounts []service.Account) error {
	if !token.ValidFor(bucket) {
		return fmt.Errorf("%w: bucket=%s", service.ErrSchedulerBucketWriteFenced, bucket.String())
	}
	// 分配版本与激活指针是两个 fencing 边界；中间写入的数据只有通过第二次校验才能发布。
	version, err := c.allocateSnapshotVersion(ctx, bucket, token)
	if err != nil {
		return err
	}
	// 快照成员最终只依赖可编码账号的有序 ID；直接复用 ID 路径，避免为
	// 随后立即丢弃的完整 Account 再分配一份临时切片。
	if _, err := c.writeSnapshotVersionAndReturnAccountIDs(ctx, bucket, version, accounts); err != nil {
		return err
	}
	return c.activateSnapshotVersion(ctx, bucket, token, version)
}

// SetSnapshotAndReturnAccountIDs 完整发布快照，并返回实际成功编码并写入的有序账号 ID。
// 该可选能力只供同一重建批次复用，返回前仍会完成版本激活与 fencing 校验。
func (c *schedulerCache) SetSnapshotAndReturnAccountIDs(ctx context.Context, bucket service.SchedulerBucket, token service.SchedulerBucketWriteToken, accounts []service.Account) ([]int64, error) {
	if !token.ValidFor(bucket) {
		return nil, fmt.Errorf("%w: bucket=%s", service.ErrSchedulerBucketWriteFenced, bucket.String())
	}
	// 分配版本与激活指针是两个 fencing 边界；中间写入的数据只有通过第二次校验才能发布。
	version, err := c.allocateSnapshotVersion(ctx, bucket, token)
	if err != nil {
		return nil, err
	}
	accountIDs, err := c.writeSnapshotVersionAndReturnAccountIDs(ctx, bucket, version, accounts)
	if err != nil {
		return nil, err
	}
	if err := c.activateSnapshotVersion(ctx, bucket, token, version); err != nil {
		return nil, err
	}
	return accountIDs, nil
}

// SetSnapshotByAccountIDs 复用同批次首次完整写入后得到的账号成员。
// 每个桶仍独立分配版本、写入有序集合并执行激活 fencing，只省略重复的账号 JSON 与全局键写入。
func (c *schedulerCache) SetSnapshotByAccountIDs(ctx context.Context, bucket service.SchedulerBucket, token service.SchedulerBucketWriteToken, accountIDs []int64) error {
	if !token.ValidFor(bucket) {
		return fmt.Errorf("%w: bucket=%s", service.ErrSchedulerBucketWriteFenced, bucket.String())
	}
	version, err := c.allocateSnapshotVersion(ctx, bucket, token)
	if err != nil {
		return err
	}
	if err := c.writeSnapshotAccountIDs(ctx, bucket, version, accountIDs); err != nil {
		return err
	}
	return c.activateSnapshotVersion(ctx, bucket, token, version)
}

func (c *schedulerCache) allocateSnapshotVersion(ctx context.Context, bucket service.SchedulerBucket, token service.SchedulerBucketWriteToken) (string, error) {
	result, err := allocateSnapshotVersionScript.Run(ctx, c.rdb, []string{
		schedulerBucketKey(schedulerEpochPrefix, bucket),
		schedulerBucketKey(schedulerRetiredPrefix, bucket),
		schedulerBucketKey(schedulerVersionPrefix, bucket),
	}, token.Epoch).Int64()
	if err != nil {
		return "", err
	}
	if err := schedulerBucketWriteResultError(result, bucket); err != nil {
		return "", err
	}
	return strconv.FormatInt(result, 10), nil
}

func (c *schedulerCache) writeSnapshotVersionAndReturnAccountIDs(ctx context.Context, bucket service.SchedulerBucket, version string, accounts []service.Account) ([]int64, error) {
	accountIDs, err := c.writeAccountIDs(ctx, accounts)
	if err != nil {
		return nil, err
	}
	if err := c.writeSnapshotAccountIDs(ctx, bucket, version, accountIDs); err != nil {
		return nil, err
	}
	return accountIDs, nil
}

func (c *schedulerCache) writeSnapshotAccountIDs(ctx context.Context, bucket service.SchedulerBucket, version string, accountIDs []int64) error {
	members := schedulerSnapshotMembers(accountIDs)
	return c.writeSnapshotMembers(ctx, bucket, version, members)
}

func schedulerSnapshotMembers(accountIDs []int64) []redis.Z {
	if len(accountIDs) == 0 {
		return nil
	}
	// 使用序号作为 score，保持数据库返回的排序语义；重复 ID 继续交由 Redis ZADD
	// 按最后一个 score 覆盖，与直接从账号切片构造成员时的行为一致。
	members := make([]redis.Z, 0, len(accountIDs))
	for idx, accountID := range accountIDs {
		members = append(members, redis.Z{
			Score:  float64(idx),
			Member: strconv.FormatInt(accountID, 10),
		})
	}
	return members
}

func (c *schedulerCache) writeSnapshotMembers(ctx context.Context, bucket service.SchedulerBucket, version string, members []redis.Z) error {
	if len(members) == 0 {
		return nil
	}
	snapshotKey := schedulerSnapshotKey(bucket, version)
	pipe := c.rdb.Pipeline()
	for start := 0; start < len(members); start += c.writeChunkSize {
		end := start + c.writeChunkSize
		if end > len(members) {
			end = len(members)
		}
		pipe.ZAdd(ctx, snapshotKey, members[start:end]...)
	}
	_, err := pipe.Exec(ctx)
	return err
}

func (c *schedulerCache) activateSnapshotVersion(ctx context.Context, bucket service.SchedulerBucket, token service.SchedulerBucketWriteToken, version string) error {
	snapshotKey := schedulerSnapshotKey(bucket, version)
	// Phase 2: 原子 CAS 切换版本，同时再次校验退休状态与 writer epoch。
	// Lua 脚本保证：仅当新版本 >= 当前激活版本时才切换 active 指针，
	// 防止并发写入导致版本回滚。
	// 旧快照使用 EXPIRE 宽限期而非立即 DEL，避免 reader 竞态。
	activeKey := schedulerBucketKey(schedulerActivePrefix, bucket)
	readyKey := schedulerBucketKey(schedulerReadyPrefix, bucket)
	snapshotKeyPrefix := fmt.Sprintf("%s%d:%s:%s:v", schedulerSnapshotPrefix, bucket.GroupID, bucket.Platform, bucket.Mode)

	keys := []string{
		activeKey,
		readyKey,
		schedulerBucketSetKey,
		snapshotKey,
		schedulerBucketKey(schedulerEpochPrefix, bucket),
		schedulerBucketKey(schedulerRetiredPrefix, bucket),
	}
	args := []any{version, bucket.String(), snapshotKeyPrefix, snapshotGraceTTLSeconds, token.Epoch}

	result, err := activateSnapshotScript.Run(ctx, c.rdb, keys, args...).Int64()
	if err != nil {
		return err
	}
	return schedulerBucketWriteResultError(result, bucket)
}

func schedulerBucketWriteResultError(result int64, bucket service.SchedulerBucket) error {
	switch result {
	case -1:
		return fmt.Errorf("%w: bucket=%s", service.ErrSchedulerBucketRetired, bucket.String())
	case -2:
		return fmt.Errorf("%w: bucket=%s", service.ErrSchedulerBucketWriteFenced, bucket.String())
	default:
		return nil
	}
}

func (c *schedulerCache) GetAccount(ctx context.Context, accountID int64) (*service.Account, error) {
	id := strconv.FormatInt(accountID, 10)
	values, err := c.rdb.MGet(ctx, schedulerAccountKey(id), schedulerLastUsedKey(id)).Result()
	if err != nil {
		return nil, err
	}
	if len(values) != 2 || values[0] == nil {
		return nil, nil
	}
	account, err := decodeCachedAccount(values[0])
	if err != nil {
		return nil, err
	}
	if err := applySchedulerLastUsed(account, values[1]); err != nil {
		return nil, err
	}
	return account, nil
}

func (c *schedulerCache) SetAccount(ctx context.Context, account *service.Account) error {
	if account == nil || account.ID <= 0 {
		return nil
	}
	accountIDs, err := c.writeAccountIDs(ctx, []service.Account{*account})
	if err != nil {
		return err
	}
	if len(accountIDs) == 0 {
		return c.DeleteAccount(ctx, account.ID)
	}
	return nil
}

func (c *schedulerCache) DeleteAccount(ctx context.Context, accountID int64) error {
	if accountID <= 0 {
		return nil
	}
	id := strconv.FormatInt(accountID, 10)
	return c.rdb.Del(ctx, schedulerAccountKey(id), schedulerAccountMetaKey(id), schedulerLastUsedKey(id)).Err()
}

func (c *schedulerCache) UpdateLastUsed(ctx context.Context, updates map[int64]time.Time) error {
	if len(updates) == 0 {
		return nil
	}

	pipe := c.rdb.Pipeline()
	queued := 0
	keys := make([]string, 0, schedulerLastUsedUpdateChunkSize*2)
	args := make([]any, 0, schedulerLastUsedUpdateChunkSize)
	queueBatch := func() {
		if len(args) == 0 {
			return
		}
		updateSchedulerLastUsedScript.Eval(ctx, pipe, keys, args...)
		queued++
		keys = make([]string, 0, schedulerLastUsedUpdateChunkSize*2)
		args = make([]any, 0, schedulerLastUsedUpdateChunkSize)
	}
	for id, usedAt := range updates {
		if id <= 0 {
			continue
		}
		millis, err := schedulerLastUsedMillis(usedAt)
		if err != nil {
			slog.Warn("scheduler cache removes account with unencodable payload",
				"account_id", id,
				"error", err,
			)
			idText := strconv.FormatInt(id, 10)
			pipe.Del(ctx, schedulerAccountKey(idText), schedulerAccountMetaKey(idText), schedulerLastUsedKey(idText))
			queued++
			continue
		}
		idText := strconv.FormatInt(id, 10)
		keys = append(keys, schedulerAccountKey(idText), schedulerLastUsedKey(idText))
		args = append(args, millis)
		if len(args) >= schedulerLastUsedUpdateChunkSize {
			queueBatch()
		}
	}
	queueBatch()
	if queued == 0 {
		return nil
	}
	_, err := pipe.Exec(ctx)
	return err
}

func (c *schedulerCache) TryLockBucket(ctx context.Context, bucket service.SchedulerBucket, ttl time.Duration) (bool, error) {
	key := schedulerBucketKey(schedulerLockPrefix, bucket)
	return c.rdb.SetNX(ctx, key, time.Now().UnixNano(), ttl).Result()
}

func (c *schedulerCache) UnlockBucket(ctx context.Context, bucket service.SchedulerBucket) error {
	key := schedulerBucketKey(schedulerLockPrefix, bucket)
	return c.rdb.Del(ctx, key).Err()
}

func (c *schedulerCache) ListBuckets(ctx context.Context) ([]service.SchedulerBucket, error) {
	raw, err := c.rdb.SMembers(ctx, schedulerBucketSetKey).Result()
	if err != nil {
		return nil, err
	}
	out := make([]service.SchedulerBucket, 0, len(raw))
	for _, entry := range raw {
		bucket, ok := service.ParseSchedulerBucket(entry)
		if !ok {
			continue
		}
		out = append(out, bucket)
	}
	return out, nil
}

func (c *schedulerCache) GetOutboxWatermark(ctx context.Context) (int64, error) {
	val, err := c.rdb.Get(ctx, schedulerOutboxWatermarkKey).Result()
	if err == redis.Nil {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	id, err := strconv.ParseInt(val, 10, 64)
	if err != nil {
		return 0, err
	}
	return id, nil
}

func (c *schedulerCache) SetOutboxWatermark(ctx context.Context, id int64) error {
	return c.rdb.Set(ctx, schedulerOutboxWatermarkKey, strconv.FormatInt(id, 10), 0).Err()
}

func (c *schedulerCache) GetSchedulerEngineState(ctx context.Context) (service.SchedulerEngineState, error) {
	values, err := c.rdb.MGet(ctx,
		schedulerEngineKey,
		schedulerEngineStatusKey,
		schedulerEngineErrorKey,
		schedulerCandidateLimitKey,
		schedulerScanLimitKey,
	).Result()
	if err != nil {
		return service.SchedulerEngineState{}, err
	}
	state := service.SchedulerEngineState{}
	if len(values) > 0 && values[0] != nil {
		state.Engine = fmt.Sprint(values[0])
	}
	if len(values) > 1 && values[1] != nil {
		state.Status = fmt.Sprint(values[1])
	}
	if len(values) > 2 && values[2] != nil {
		state.LastError = fmt.Sprint(values[2])
	}
	if len(values) > 3 && values[3] != nil {
		state.CandidateLimit, _ = strconv.Atoi(fmt.Sprint(values[3]))
	}
	if len(values) > 4 && values[4] != nil {
		state.ScanLimit, _ = strconv.Atoi(fmt.Sprint(values[4]))
	}
	return state, nil
}

func (c *schedulerCache) SetSchedulerV2Limits(ctx context.Context, candidateLimit, scanLimit int) error {
	if err := service.ValidateSchedulerV2Limits(candidateLimit, scanLimit); err != nil {
		return err
	}
	return c.rdb.MSet(ctx,
		schedulerCandidateLimitKey, candidateLimit,
		schedulerScanLimitKey, scanLimit,
	).Err()
}

func (c *schedulerCache) SetSchedulerEngineState(ctx context.Context, state service.SchedulerEngineState) error {
	if state.Engine != service.SchedulerEngineV2 {
		state.Engine = service.SchedulerEngineLegacy
	}
	if state.Status == "" {
		if state.Engine == service.SchedulerEngineV2 {
			state.Status = service.SchedulerEngineStatusBuilding
		} else {
			state.Status = service.SchedulerEngineStatusDisabled
		}
	}
	values := []any{
		schedulerEngineKey, state.Engine,
		schedulerEngineStatusKey, state.Status,
		schedulerEngineErrorKey, state.LastError,
	}
	if service.ValidateSchedulerV2Limits(state.CandidateLimit, state.ScanLimit) == nil {
		values = append(values,
			schedulerCandidateLimitKey, state.CandidateLimit,
			schedulerScanLimitKey, state.ScanLimit,
		)
	}
	return c.rdb.MSet(ctx, values...).Err()
}

func (c *schedulerCache) CompareAndSetSchedulerEngineState(ctx context.Context, expectedEngine, expectedStatus string, state service.SchedulerEngineState) (bool, error) {
	if state.Engine != service.SchedulerEngineV2 {
		state.Engine = service.SchedulerEngineLegacy
	}
	if state.Status == "" {
		if state.Engine == service.SchedulerEngineV2 {
			state.Status = service.SchedulerEngineStatusBuilding
		} else {
			state.Status = service.SchedulerEngineStatusDisabled
		}
	}
	result, err := compareAndSetSchedulerEngineScript.Run(ctx, c.rdb, []string{
		schedulerEngineKey,
		schedulerEngineStatusKey,
		schedulerEngineErrorKey,
	}, expectedEngine, expectedStatus, state.Engine, state.Status, state.LastError).Int64()
	if err != nil {
		return false, err
	}
	return result == 1, nil
}

func (c *schedulerCache) GetCandidatePage(ctx context.Context, bucket service.SchedulerBucket, offset int64, limit int) (service.SchedulerCandidatePage, bool, error) {
	if offset < 0 {
		offset = 0
	}
	if limit < service.MinSchedulerCandidateFetchLimit {
		limit = service.MinSchedulerCandidateFetchLimit
	}

	result, err := readCandidatePageScript.Run(ctx, c.rdb, []string{
		schedulerBucketKey(schedulerCandidateReady, bucket),
		schedulerBucketKey(schedulerCandidateCount, bucket),
		schedulerCandidateKey(bucket),
	}, offset, limit, service.SchedulerCandidateIndexVersion).Slice()
	if err != nil {
		return service.SchedulerCandidatePage{}, false, err
	}
	if len(result) == 0 || toRedisInt64(result[0]) != 1 {
		return service.SchedulerCandidatePage{}, false, nil
	}
	if len(result) < 2 {
		return service.SchedulerCandidatePage{}, false, nil
	}
	count := toRedisInt64(result[1])
	if count == 0 || offset >= count {
		return service.SchedulerCandidatePage{Accounts: []*service.Account{}, NextOffset: offset, Done: true}, true, nil
	}
	ids := make([]string, 0, len(result)-2)
	for _, raw := range result[2:] {
		ids = append(ids, fmt.Sprint(raw))
	}
	if len(ids) == 0 {
		return service.SchedulerCandidatePage{}, false, nil
	}
	keys := make([]string, 0, len(ids)*2)
	for _, id := range ids {
		keys = append(keys, schedulerAccountMetaKey(id))
	}
	for _, id := range ids {
		keys = append(keys, schedulerLastUsedKey(id))
	}
	values, err := c.rdb.MGet(ctx, keys...).Result()
	if err != nil {
		return service.SchedulerCandidatePage{}, false, err
	}
	if len(values) != len(ids)*2 {
		return service.SchedulerCandidatePage{}, false, nil
	}
	accounts := make([]*service.Account, 0, len(ids))
	for i, value := range values[:len(ids)] {
		if value == nil {
			// A ready index with missing metadata is incomplete. The caller will
			// rebuild this bucket instead of silently skipping an account.
			return service.SchedulerCandidatePage{}, false, nil
		}
		account, err := decodeCachedAccount(value)
		if err != nil {
			return service.SchedulerCandidatePage{}, false, err
		}
		if err := applySchedulerLastUsed(account, values[len(ids)+i]); err != nil {
			return service.SchedulerCandidatePage{}, false, err
		}
		accounts = append(accounts, account)
	}
	next := offset + int64(len(ids))
	return service.SchedulerCandidatePage{
		Accounts:   accounts,
		NextOffset: next,
		Done:       next >= count,
	}, true, nil
}

func (c *schedulerCache) SetCandidateIndex(ctx context.Context, bucket service.SchedulerBucket, token service.SchedulerBucketWriteToken, accounts []service.Account) error {
	if !token.ValidFor(bucket) {
		return fmt.Errorf("%w: bucket=%s", service.ErrSchedulerBucketWriteFenced, bucket.String())
	}
	cacheableAccounts, err := c.writeAccounts(ctx, accounts)
	if err != nil {
		return err
	}

	candidateKey := schedulerCandidateKey(bucket)
	tmpKey := fmt.Sprintf("%s:tmp:%d", candidateKey, time.Now().UnixNano())
	newIDs := make(map[string]struct{}, len(cacheableAccounts))
	members := make([]redis.Z, 0, len(cacheableAccounts))
	for _, account := range cacheableAccounts {
		if account.ID <= 0 {
			continue
		}
		id := strconv.FormatInt(account.ID, 10)
		if _, exists := newIDs[id]; exists {
			continue
		}
		newIDs[id] = struct{}{}
		members = append(members, redis.Z{
			Score:  service.SchedulerCandidateScore(account, bucket),
			Member: id,
		})
	}
	if err := c.rdb.Del(ctx, tmpKey).Err(); err != nil {
		return err
	}
	if len(members) > 0 {
		pipe := c.rdb.Pipeline()
		for start := 0; start < len(members); start += c.writeChunkSize {
			end := start + c.writeChunkSize
			if end > len(members) {
				end = len(members)
			}
			pipe.ZAdd(ctx, tmpKey, members[start:end]...)
		}
		if _, err := pipe.Exec(ctx); err != nil {
			return err
		}
	}

	result, err := activateCandidateIndexScript.Run(ctx, c.rdb, []string{
		schedulerBucketKey(schedulerEpochPrefix, bucket),
		schedulerBucketKey(schedulerRetiredPrefix, bucket),
		tmpKey,
		candidateKey,
		schedulerBucketKey(schedulerCandidateCount, bucket),
		schedulerBucketKey(schedulerCandidateReady, bucket),
		schedulerCandidateBuckets,
		schedulerBucketSetKey,
	}, token.Epoch, bucket.String(), service.SchedulerCandidateIndexVersion, schedulerAccountBuckets).Int64()
	if err != nil {
		return err
	}
	return schedulerBucketWriteResultError(result, bucket)
}

func (c *schedulerCache) ReplaceAccountCandidates(ctx context.Context, account *service.Account, buckets []service.SchedulerBucket) error {
	if account == nil || account.ID <= 0 {
		return nil
	}
	if err := c.SetAccount(ctx, account); err != nil {
		return err
	}
	id := strconv.FormatInt(account.ID, 10)
	oldRaw, err := c.rdb.SMembers(ctx, schedulerAccountCandidateBucketsKey(id)).Result()
	if err != nil {
		return err
	}
	old := make(map[string]service.SchedulerBucket, len(oldRaw))
	for _, raw := range oldRaw {
		if bucket, ok := service.ParseSchedulerBucket(raw); ok {
			old[raw] = bucket
		}
	}
	requested := make(map[string]service.SchedulerBucket, len(buckets))
	for _, bucket := range buckets {
		requested[bucket.String()] = bucket
	}
	all := make(map[string]service.SchedulerBucket, len(old)+len(requested))
	for raw, bucket := range old {
		all[raw] = bucket
	}
	for raw, bucket := range requested {
		all[raw] = bucket
	}
	ordered := make([]string, 0, len(all))
	for raw := range all {
		ordered = append(ordered, raw)
	}
	sort.Strings(ordered)

	actual := make([]string, 0, len(requested))
	for _, raw := range ordered {
		bucket := all[raw]
		if err := c.withCandidateBucketLock(ctx, bucket, func() error {
			_, want := requested[raw]
			if want {
				result, err := c.upsertCandidate(ctx, bucket, id, service.SchedulerCandidateScore(*account, bucket))
				if err != nil {
					return err
				}
				if result < 0 {
					return nil
				}
				actual = append(actual, raw)
				return nil
			}
			_, err := c.removeCandidate(ctx, bucket, id)
			return err
		}); err != nil {
			return err
		}
	}

	pipe := c.rdb.Pipeline()
	pipe.Del(ctx, schedulerAccountCandidateBucketsKey(id))
	if len(actual) > 0 {
		members := make([]any, 0, len(actual))
		for _, raw := range actual {
			members = append(members, raw)
		}
		pipe.SAdd(ctx, schedulerAccountCandidateBucketsKey(id), members...)
	}
	if _, err = pipe.Exec(ctx); err != nil {
		return err
	}
	// A bucket rebuild that started before this account event may have written
	// older metadata while the event waited on its lock. Publish the event's DB
	// snapshot once more after all bucket mutations so the newer state wins.
	return c.SetAccount(ctx, account)
}

func (c *schedulerCache) DeleteCandidateAccount(ctx context.Context, accountID int64) error {
	if accountID <= 0 {
		return nil
	}
	id := strconv.FormatInt(accountID, 10)
	rawBuckets, err := c.rdb.SMembers(ctx, schedulerAccountCandidateBucketsKey(id)).Result()
	if err != nil {
		return err
	}
	sort.Strings(rawBuckets)
	for _, raw := range rawBuckets {
		bucket, ok := service.ParseSchedulerBucket(raw)
		if !ok {
			continue
		}
		if err := c.withCandidateBucketLock(ctx, bucket, func() error {
			_, err := c.removeCandidate(ctx, bucket, id)
			return err
		}); err != nil {
			return err
		}
	}
	return c.rdb.Del(ctx, schedulerAccountCandidateBucketsKey(id), schedulerAccountKey(id), schedulerAccountMetaKey(id)).Err()
}

func (c *schedulerCache) UpdateCandidateLastUsed(ctx context.Context, updates map[int64]time.Time) error {
	for accountID, lastUsedAt := range updates {
		if err := c.updateCandidateLastUsed(ctx, accountID, lastUsedAt); err != nil {
			return err
		}
	}
	return nil
}

func (c *schedulerCache) updateCandidateLastUsed(ctx context.Context, accountID int64, lastUsedAt time.Time) error {
	if accountID <= 0 {
		return nil
	}
	id := strconv.FormatInt(accountID, 10)
	value, err := c.rdb.Get(ctx, schedulerAccountKey(id)).Result()
	if err == redis.Nil {
		return nil
	}
	if err != nil {
		return err
	}
	account, err := decodeCachedAccount(value)
	if err != nil {
		return err
	}
	account.LastUsedAt = ptrTime(lastUsedAt)
	rawBuckets, err := c.rdb.SMembers(ctx, schedulerAccountCandidateBucketsKey(id)).Result()
	if err != nil {
		return err
	}
	for _, raw := range rawBuckets {
		bucket, ok := service.ParseSchedulerBucket(raw)
		if !ok {
			continue
		}
		if err := c.withCandidateBucketLock(ctx, bucket, func() error {
			return c.rdb.ZAddXX(ctx, schedulerCandidateKey(bucket), redis.Z{
				Score:  service.SchedulerCandidateScore(*account, bucket),
				Member: id,
			}).Err()
		}); err != nil {
			return err
		}
	}
	return c.SetAccount(ctx, account)
}

func (c *schedulerCache) InvalidateLegacySnapshots(ctx context.Context) error {
	buckets, err := c.ListBuckets(ctx)
	if err != nil {
		return err
	}
	for start := 0; start < len(buckets); start += c.writeChunkSize {
		end := start + c.writeChunkSize
		if end > len(buckets) {
			end = len(buckets)
		}
		activeKeys := make([]string, 0, end-start)
		for _, bucket := range buckets[start:end] {
			activeKeys = append(activeKeys, schedulerBucketKey(schedulerActivePrefix, bucket))
		}
		activeVersions, err := c.rdb.MGet(ctx, activeKeys...).Result()
		if err != nil {
			return err
		}
		pipe := c.rdb.Pipeline()
		for i, bucket := range buckets[start:end] {
			pipe.Del(ctx,
				schedulerBucketKey(schedulerReadyPrefix, bucket),
				schedulerBucketKey(schedulerActivePrefix, bucket),
			)
			if activeVersions[i] != nil {
				pipe.Del(ctx, schedulerSnapshotKey(bucket, fmt.Sprint(activeVersions[i])))
			}
		}
		if _, err := pipe.Exec(ctx); err != nil {
			return err
		}
	}
	return nil
}

func (c *schedulerCache) withCandidateBucketLock(ctx context.Context, bucket service.SchedulerBucket, fn func() error) error {
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	for {
		acquired, err := c.TryLockBucket(ctx, bucket, 30*time.Second)
		if err != nil {
			return err
		}
		if acquired {
			defer func() { _ = c.UnlockBucket(context.Background(), bucket) }()
			return fn()
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}

func (c *schedulerCache) upsertCandidate(ctx context.Context, bucket service.SchedulerBucket, id string, score float64) (int64, error) {
	return upsertCandidateScript.Run(ctx, c.rdb, []string{
		schedulerCandidateKey(bucket),
		schedulerBucketKey(schedulerCandidateCount, bucket),
		schedulerBucketKey(schedulerCandidateReady, bucket),
	}, score, id, service.SchedulerCandidateIndexVersion).Int64()
}

func (c *schedulerCache) removeCandidate(ctx context.Context, bucket service.SchedulerBucket, id string) (int64, error) {
	return removeCandidateScript.Run(ctx, c.rdb, []string{
		schedulerCandidateKey(bucket),
		schedulerBucketKey(schedulerCandidateCount, bucket),
		schedulerBucketKey(schedulerCandidateReady, bucket),
	}, id, service.SchedulerCandidateIndexVersion).Int64()
}

func schedulerBucketKey(prefix string, bucket service.SchedulerBucket) string {
	return fmt.Sprintf("%s%d:%s:%s", prefix, bucket.GroupID, bucket.Platform, bucket.Mode)
}

func schedulerGroupLifecycleLockKey(groupID int64) string {
	return schedulerGroupLifecycleLockPrefix + strconv.FormatInt(groupID, 10)
}

func schedulerSnapshotKey(bucket service.SchedulerBucket, version string) string {
	return fmt.Sprintf("%s%d:%s:%s:v%s", schedulerSnapshotPrefix, bucket.GroupID, bucket.Platform, bucket.Mode, version)
}

func schedulerAccountKey(id string) string {
	return schedulerAccountPrefix + id
}

func schedulerAccountMetaKey(id string) string {
	return schedulerAccountMetaPrefix + id
}

func schedulerCandidateKey(bucket service.SchedulerBucket) string {
	return schedulerBucketKey(schedulerCandidatePrefix, bucket)
}

func schedulerAccountCandidateBucketsKey(id string) string {
	return schedulerAccountBuckets + id
}

func schedulerLastUsedKey(id string) string {
	return schedulerAccountLastUsedPrefix + id
}

func ptrTime(t time.Time) *time.Time {
	return &t
}

func toRedisInt64(value any) int64 {
	switch typed := value.(type) {
	case int64:
		return typed
	case int:
		return int64(typed)
	case string:
		parsed, _ := strconv.ParseInt(typed, 10, 64)
		return parsed
	case []byte:
		parsed, _ := strconv.ParseInt(string(typed), 10, 64)
		return parsed
	default:
		parsed, _ := strconv.ParseInt(fmt.Sprint(value), 10, 64)
		return parsed
	}
}

func schedulerLastUsedMillis(value time.Time) (int64, error) {
	if _, err := value.MarshalJSON(); err != nil {
		return 0, err
	}
	return value.UTC().UnixMilli(), nil
}

func applySchedulerLastUsed(account *service.Account, value any) error {
	if account == nil || value == nil {
		return nil
	}
	var raw string
	switch typed := value.(type) {
	case string:
		raw = typed
	case []byte:
		raw = string(typed)
	default:
		return fmt.Errorf("unexpected last_used cache type: %T", value)
	}
	millis, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		return fmt.Errorf("invalid last_used cache value %q: %w", raw, err)
	}
	lastUsedAt := time.UnixMilli(millis).UTC()
	if account.LastUsedAt == nil || lastUsedAt.After(*account.LastUsedAt) {
		account.LastUsedAt = ptrTime(lastUsedAt)
	}
	return nil
}

func decodeCachedAccount(val any) (*service.Account, error) {
	var payload []byte
	switch raw := val.(type) {
	case string:
		payload = []byte(raw)
	case []byte:
		payload = raw
	default:
		return nil, fmt.Errorf("unexpected account cache type: %T", val)
	}
	var account service.Account
	if err := json.Unmarshal(payload, &account); err != nil {
		return nil, err
	}
	return &account, nil
}

// writeAccounts is retained for candidate-index rebuilds, which need the
// cacheable account metadata to calculate candidate scores. Snapshot-only
// callers use writeAccountIDs to avoid materializing this slice.
func (c *schedulerCache) writeAccounts(ctx context.Context, accounts []service.Account) ([]service.Account, error) {
	cacheableAccounts, _, err := c.writeAccountPayloads(ctx, accounts, false)
	return cacheableAccounts, err
}

func (c *schedulerCache) writeAccountIDs(ctx context.Context, accounts []service.Account) ([]int64, error) {
	_, accountIDs, err := c.writeAccountPayloads(ctx, accounts, true)
	return accountIDs, err
}

func (c *schedulerCache) writeAccountPayloads(ctx context.Context, accounts []service.Account, collectIDs bool) ([]service.Account, []int64, error) {
	if len(accounts) == 0 {
		return nil, nil, nil
	}

	pipe := c.rdb.Pipeline()
	var cacheableAccounts []service.Account
	var accountIDs []int64
	if collectIDs {
		accountIDs = make([]int64, 0, len(accounts))
	} else {
		cacheableAccounts = make([]service.Account, 0, len(accounts))
	}
	pending := 0
	flush := func() error {
		if pending == 0 {
			return nil
		}
		if _, err := pipe.Exec(ctx); err != nil {
			return err
		}
		pipe = c.rdb.Pipeline()
		pending = 0
		return nil
	}

	for _, account := range accounts {
		fullPayload, metaPayload, err := marshalSchedulerCacheAccount(account)
		if err != nil {
			slog.Warn("scheduler cache skips account with unencodable payload",
				"account_id", account.ID,
				"error", err,
			)
			continue
		}

		id := strconv.FormatInt(account.ID, 10)
		pipe.Set(ctx, schedulerAccountKey(id), fullPayload, 0)
		pipe.Set(ctx, schedulerAccountMetaKey(id), metaPayload, 0)
		// Do not touch the hot LastUsedAt side key here: a lagging snapshot
		// rebuild must not overwrite a newer scheduler update.
		if collectIDs {
			accountIDs = append(accountIDs, account.ID)
		} else {
			cacheableAccounts = append(cacheableAccounts, account)
		}
		pending++
		if pending >= c.writeChunkSize {
			if err := flush(); err != nil {
				return nil, nil, err
			}
		}
	}

	if err := flush(); err != nil {
		return nil, nil, err
	}
	return cacheableAccounts, accountIDs, nil
}

func marshalSchedulerCacheAccount(account service.Account) ([]byte, []byte, error) {
	fullPayload, err := json.Marshal(account)
	if err != nil {
		return nil, nil, fmt.Errorf("marshal account: %w", err)
	}
	metaPayload, err := json.Marshal(buildSchedulerMetadataAccount(account))
	if err != nil {
		return nil, nil, fmt.Errorf("marshal account metadata: %w", err)
	}
	return fullPayload, metaPayload, nil
}

func (c *schedulerCache) mgetChunked(ctx context.Context, keys []string) ([]any, error) {
	if len(keys) == 0 {
		return []any{}, nil
	}

	out := make([]any, 0, len(keys))
	chunkSize := c.mgetChunkSize
	if chunkSize <= 0 {
		chunkSize = defaultSchedulerSnapshotMGetChunkSize
	}
	for start := 0; start < len(keys); start += chunkSize {
		end := start + chunkSize
		if end > len(keys) {
			end = len(keys)
		}
		part, err := c.rdb.MGet(ctx, keys[start:end]...).Result()
		if err != nil {
			return nil, err
		}
		out = append(out, part...)
	}
	return out, nil
}

func buildSchedulerMetadataAccount(account service.Account) service.Account {
	extra := filterSchedulerExtra(account.Extra)
	if account.IsOpenAIOAuth() {
		if virtualClientKey := account.CodexVirtualClientKey(); virtualClientKey != "" {
			if extra == nil {
				extra = make(map[string]any, 1)
			}
			// This namespace is intentionally non-credential state. Keeping it in
			// metadata lets request predicates identify the owner principal before
			// Scheduler V2 hydrates the selected account's full credential payload.
			extra[service.CodexVirtualClientKeyExtraKey] = virtualClientKey
		}
	}
	return service.Account{
		ID:                      account.ID,
		Name:                    account.Name,
		Platform:                account.Platform,
		Type:                    account.Type,
		Concurrency:             account.Concurrency,
		LoadFactor:              account.LoadFactor,
		Priority:                account.Priority,
		RateMultiplier:          account.RateMultiplier,
		Status:                  account.Status,
		LastUsedAt:              account.LastUsedAt,
		ExpiresAt:               account.ExpiresAt,
		AutoPauseOnExpired:      account.AutoPauseOnExpired,
		Schedulable:             account.Schedulable,
		RateLimitedAt:           account.RateLimitedAt,
		RateLimitResetAt:        account.RateLimitResetAt,
		OverloadUntil:           account.OverloadUntil,
		TempUnschedulableUntil:  account.TempUnschedulableUntil,
		TempUnschedulableReason: account.TempUnschedulableReason,
		SessionWindowStart:      account.SessionWindowStart,
		SessionWindowEnd:        account.SessionWindowEnd,
		SessionWindowStatus:     account.SessionWindowStatus,
		ParentAccountID:         account.ParentAccountID,
		QuotaDimension:          account.QuotaDimension,
		EgressMode:              account.EgressMode,
		EgressBinding:           account.EgressBinding,
		AccountGroups:           filterSchedulerAccountGroups(account.AccountGroups),
		GroupIDs:                filterSchedulerGroupIDs(account.GroupIDs, account.AccountGroups),
		Credentials:             filterSchedulerCredentials(account.Credentials),
		Extra:                   extra,
	}
}

func filterSchedulerAccountGroups(accountGroups []service.AccountGroup) []service.AccountGroup {
	if len(accountGroups) == 0 {
		return nil
	}

	filtered := make([]service.AccountGroup, 0, len(accountGroups))
	for _, ag := range accountGroups {
		if ag.GroupID <= 0 {
			continue
		}
		filtered = append(filtered, service.AccountGroup{
			AccountID: ag.AccountID,
			GroupID:   ag.GroupID,
			Priority:  ag.Priority,
			CreatedAt: ag.CreatedAt,
		})
	}
	if len(filtered) == 0 {
		return nil
	}
	return filtered
}

func filterSchedulerGroupIDs(groupIDs []int64, accountGroups []service.AccountGroup) []int64 {
	if len(groupIDs) == 0 && len(accountGroups) == 0 {
		return nil
	}

	seen := make(map[int64]struct{}, len(groupIDs)+len(accountGroups))
	filtered := make([]int64, 0, len(groupIDs)+len(accountGroups))
	for _, id := range groupIDs {
		if id <= 0 {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		filtered = append(filtered, id)
	}
	for _, ag := range accountGroups {
		if ag.GroupID <= 0 {
			continue
		}
		if _, ok := seen[ag.GroupID]; ok {
			continue
		}
		seen[ag.GroupID] = struct{}{}
		filtered = append(filtered, ag.GroupID)
	}
	if len(filtered) == 0 {
		return nil
	}
	return filtered
}

func filterSchedulerCredentials(credentials map[string]any) map[string]any {
	if len(credentials) == 0 {
		return nil
	}
	keys := []string{
		"model_mapping",
		"compact_model_mapping",
		"compact_model_fallbacks",
		"openai_capabilities",
		"auth_mode",
		"openai_auth_mode",
		"api_key",
		"project_id",
		"oauth_type",
		"plan_type",
	}
	filtered := make(map[string]any)
	for _, key := range keys {
		if value, ok := credentials[key]; ok && value != nil {
			filtered[key] = value
		}
	}
	if len(filtered) == 0 {
		return nil
	}
	return filtered
}

func filterSchedulerExtra(extra map[string]any) map[string]any {
	if len(extra) == 0 {
		return nil
	}
	keys := []string{
		"quota_limit",
		"quota_used",
		"quota_daily_limit",
		"quota_daily_used",
		"quota_daily_start",
		"quota_daily_reset_mode",
		"quota_daily_reset_hour",
		"quota_weekly_limit",
		"quota_weekly_used",
		"quota_weekly_start",
		"quota_weekly_reset_mode",
		"quota_weekly_reset_day",
		"quota_weekly_reset_hour",
		"quota_reset_timezone",
		"mixed_scheduling",
		"window_cost_limit",
		"window_cost_sticky_reserve",
		"max_sessions",
		"session_idle_timeout_minutes",
		"base_rpm",
		"rpm_strategy",
		"rpm_sticky_buffer",
		"openai_compact_mode",
		"openai_compact_supported",
		"openai_passthrough",
		"openai_oauth_passthrough",
		service.AccountExtraOpenAIImageSizeTiers,
		"openai_oauth_responses_websockets_v2_enabled",
		"openai_oauth_responses_websockets_v2_mode",
		"openai_apikey_responses_websockets_v2_enabled",
		"openai_apikey_responses_websockets_v2_mode",
		"responses_websockets_v2_enabled",
		"openai_ws_enabled",
		"openai_ws_force_http",
		"openai_responses_mode",
		"openai_responses_supported",
		"codex_5h_used_percent",
		"codex_7d_used_percent",
		"codex_5h_reset_at",
		"codex_7d_reset_at",
		"codex_5h_reset_after_seconds",
		"codex_7d_reset_after_seconds",
		"codex_usage_updated_at",
		"auto_pause_5h_threshold",
		"auto_pause_7d_threshold",
		"auto_pause_5h_disabled",
		"auto_pause_7d_disabled",
		service.CodexPrewarmContinuationExtraKey,
		service.CodexThinkingTagNormalizationExtraKey,
		"codex_fingerprint_mode",
		service.AutoDisableOnUpstreamInsufficientBalanceExtraKey,
		"model_rate_limits",
		service.UpstreamBillingProbeExtraKey,
		service.GrokMediaEligibleExtraKey,
		"grok_billing_snapshot",
		service.OAuthSupportedModelsExtraKey,
		service.OAuthSupportedModelsSyncedAtExtraKey,
		service.OpenAIOAuthSupportedModelsExtraKey,
		service.OpenAIOAuthSupportedModelsSyncedAtExtraKey,
	}
	filtered := make(map[string]any)
	for _, key := range keys {
		if value, ok := extra[key]; ok && value != nil {
			if key == service.UpstreamBillingProbeExtraKey {
				filteredProbe := filterSchedulerUpstreamBillingProbe(value)
				if filteredProbe == nil {
					continue
				}
				value = filteredProbe
			}
			filtered[key] = value
		}
	}
	if len(filtered) == 0 {
		return nil
	}
	return filtered
}

func filterSchedulerUpstreamBillingProbe(value any) map[string]any {
	source, ok := value.(map[string]any)
	if !ok {
		return nil
	}

	status, ok := source["status"].(string)
	if !ok || status == "" {
		return nil
	}
	filtered := map[string]any{"status": status}
	for _, key := range []string{"received_at", "fresh_until", "next_probe_at"} {
		if field, exists := source[key]; exists && field != nil {
			filtered[key] = field
		}
	}
	data, ok := source["data"].(map[string]any)
	if !ok {
		return filtered
	}
	filteredData := make(map[string]any)
	for _, key := range []string{
		"billing_scope",
		"resolved_rate_multiplier",
		"peak_rate_enabled",
		"peak_start",
		"peak_end",
		"peak_rate_multiplier",
		"timezone",
	} {
		if field, exists := data[key]; exists && field != nil {
			filteredData[key] = field
		}
	}
	if len(filteredData) > 0 {
		filtered["data"] = filteredData
	}
	return filtered
}
