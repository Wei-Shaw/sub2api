package repository

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"log/slog"
	"strconv"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/redis/go-redis/v9"
)

const (
	schedulerBucketSetKey       = "sched:buckets"
	schedulerOutboxWatermarkKey = "sched:outbox:watermark"
	schedulerAccountPrefix      = "sched:acc:"
	schedulerAccountMetaPrefix  = "sched:meta:"
	schedulerActivePrefix       = "sched:active:"
	schedulerReadyPrefix        = "sched:ready:"
	schedulerVersionPrefix      = "sched:ver:"
	schedulerSnapshotPrefix     = "sched:"
	schedulerLockPrefix         = "sched:lock:"
	schedulerEngineKey          = "sched:engine"
	schedulerEngineStatusKey    = "sched:engine:status"
	schedulerEngineErrorKey     = "sched:engine:error"
	schedulerCandidateLimitKey  = "sched:v2:candidate-limit"
	schedulerScanLimitKey       = "sched:v2:scan-limit"
	schedulerCandidatePrefix    = "sched:v2:cand:"
	schedulerCandidateReady     = "sched:v2:ready:"
	schedulerCandidateCount     = "sched:v2:count:"
	schedulerCandidateBuckets   = "sched:v2:buckets"
	schedulerAccountBuckets     = "sched:v2:acc-buckets:"

	defaultSchedulerSnapshotMGetChunkSize  = 128
	defaultSchedulerSnapshotWriteChunkSize = 256

	// snapshotGraceTTLSeconds 旧快照过期的宽限期（秒）。
	// 替代立即 DEL，让正在读取旧版本的 reader 有足够时间完成 ZRANGE。
	snapshotGraceTTLSeconds = 60
)

var (
	// activateSnapshotScript 原子 CAS 切换快照版本。
	// 仅当新版本号 >= 当前激活版本时才切换，防止并发写入导致版本回滚。
	// 旧快照使用 EXPIRE 设置宽限期而非立即 DEL，避免与 reader 竞态。
	//
	// KEYS[1] = activeKey     (sched:active:{bucket})
	// KEYS[2] = readyKey      (sched:ready:{bucket})
	// KEYS[3] = bucketSetKey  (sched:buckets)
	// KEYS[4] = snapshotKey   (新写入的快照 key)
	// ARGV[1] = 新版本号字符串
	// ARGV[2] = bucket 字符串 (用于 SADD)
	// ARGV[3] = 快照 key 前缀 (用于构造旧快照 key)
	// ARGV[4] = 宽限期 TTL 秒数
	//
	// 返回 1 = 已激活, 0 = 版本过旧未激活
	activateSnapshotScript = redis.NewScript(`
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
	for _, id := range ids {
		keys = append(keys, schedulerAccountMetaKey(id))
	}
	values, err := c.mgetChunked(ctx, keys)
	if err != nil {
		return nil, false, err
	}

	accounts := make([]*service.Account, 0, len(values))
	for _, val := range values {
		if val == nil {
			return nil, false, nil
		}
		account, err := decodeCachedAccount(val)
		if err != nil {
			return nil, false, err
		}
		accounts = append(accounts, account)
	}

	return accounts, true, nil
}

func (c *schedulerCache) SetSnapshot(ctx context.Context, bucket service.SchedulerBucket, accounts []service.Account) error {
	// Phase 1: 分配新版本号并写入快照数据。
	// INCR 保证每个调用方获得唯一递增版本号。
	// 写入的 snapshotKey 是新的版本化 key，reader 尚不知晓，因此无竞态。
	versionKey := schedulerBucketKey(schedulerVersionPrefix, bucket)
	version, err := c.rdb.Incr(ctx, versionKey).Result()
	if err != nil {
		return err
	}

	versionStr := strconv.FormatInt(version, 10)
	snapshotKey := schedulerSnapshotKey(bucket, versionStr)

	cacheableAccounts, err := c.writeAccounts(ctx, accounts)
	if err != nil {
		return err
	}

	if len(cacheableAccounts) > 0 {
		// 使用序号作为 score，保持数据库返回的排序语义。
		members := make([]redis.Z, 0, len(cacheableAccounts))
		for idx, account := range cacheableAccounts {
			members = append(members, redis.Z{
				Score:  float64(idx),
				Member: strconv.FormatInt(account.ID, 10),
			})
		}
		pipe := c.rdb.Pipeline()
		for start := 0; start < len(members); start += c.writeChunkSize {
			end := start + c.writeChunkSize
			if end > len(members) {
				end = len(members)
			}
			pipe.ZAdd(ctx, snapshotKey, members[start:end]...)
		}
		if _, err := pipe.Exec(ctx); err != nil {
			return err
		}
	}

	// Phase 2: 原子 CAS 激活版本。
	// Lua 脚本保证：仅当新版本 >= 当前激活版本时才切换 active 指针，
	// 防止并发写入导致版本回滚。
	// 旧快照使用 EXPIRE 宽限期而非立即 DEL，避免 reader 竞态。
	activeKey := schedulerBucketKey(schedulerActivePrefix, bucket)
	readyKey := schedulerBucketKey(schedulerReadyPrefix, bucket)
	snapshotKeyPrefix := fmt.Sprintf("%s%d:%s:%s:v", schedulerSnapshotPrefix, bucket.GroupID, bucket.Platform, bucket.Mode)

	keys := []string{activeKey, readyKey, schedulerBucketSetKey, snapshotKey}
	args := []any{versionStr, bucket.String(), snapshotKeyPrefix, snapshotGraceTTLSeconds}

	_, err = activateSnapshotScript.Run(ctx, c.rdb, keys, args...).Result()
	if err != nil {
		return err
	}

	return nil
}

func (c *schedulerCache) GetAccount(ctx context.Context, accountID int64) (*service.Account, error) {
	key := schedulerAccountKey(strconv.FormatInt(accountID, 10))
	val, err := c.rdb.Get(ctx, key).Result()
	if err == redis.Nil {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return decodeCachedAccount(val)
}

func (c *schedulerCache) SetAccount(ctx context.Context, account *service.Account) error {
	if account == nil || account.ID <= 0 {
		return nil
	}
	cacheableAccounts, err := c.writeAccounts(ctx, []service.Account{*account})
	if err != nil {
		return err
	}
	if len(cacheableAccounts) == 0 {
		return c.DeleteAccount(ctx, account.ID)
	}
	return nil
}

func (c *schedulerCache) DeleteAccount(ctx context.Context, accountID int64) error {
	if accountID <= 0 {
		return nil
	}
	id := strconv.FormatInt(accountID, 10)
	return c.rdb.Del(ctx, schedulerAccountKey(id), schedulerAccountMetaKey(id)).Err()
}

func (c *schedulerCache) UpdateLastUsed(ctx context.Context, updates map[int64]time.Time) error {
	if len(updates) == 0 {
		return nil
	}

	keys := make([]string, 0, len(updates))
	ids := make([]int64, 0, len(updates))
	for id := range updates {
		keys = append(keys, schedulerAccountKey(strconv.FormatInt(id, 10)))
		ids = append(ids, id)
	}

	values, err := c.mgetChunked(ctx, keys)
	if err != nil {
		return err
	}

	pipe := c.rdb.Pipeline()
	for i, val := range values {
		if val == nil {
			continue
		}
		account, err := decodeCachedAccount(val)
		if err != nil {
			return err
		}
		account.LastUsedAt = ptrTime(updates[ids[i]])
		updated, metaPayload, err := marshalSchedulerCacheAccount(*account)
		if err != nil {
			slog.Warn("scheduler cache removes account with unencodable payload",
				"account_id", ids[i],
				"error", err,
			)
			pipe.Del(ctx, keys[i], schedulerAccountMetaKey(strconv.FormatInt(ids[i], 10)))
			continue
		}
		pipe.Set(ctx, keys[i], updated, 0)
		pipe.Set(ctx, schedulerAccountMetaKey(strconv.FormatInt(ids[i], 10)), metaPayload, 0)
	}
	_, err = pipe.Exec(ctx)
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

	ready, err := c.rdb.Get(ctx, schedulerBucketKey(schedulerCandidateReady, bucket)).Result()
	if err == redis.Nil {
		return service.SchedulerCandidatePage{}, false, nil
	}
	if err != nil {
		return service.SchedulerCandidatePage{}, false, err
	}
	if ready != service.SchedulerCandidateIndexVersion {
		return service.SchedulerCandidatePage{}, false, nil
	}

	countRaw, err := c.rdb.Get(ctx, schedulerBucketKey(schedulerCandidateCount, bucket)).Result()
	if err == redis.Nil {
		return service.SchedulerCandidatePage{}, false, nil
	}
	if err != nil {
		return service.SchedulerCandidatePage{}, false, err
	}
	count, err := strconv.ParseInt(countRaw, 10, 64)
	if err != nil || count < 0 {
		return service.SchedulerCandidatePage{}, false, nil
	}
	if count == 0 || offset >= count {
		return service.SchedulerCandidatePage{Accounts: []*service.Account{}, NextOffset: offset, Done: true}, true, nil
	}

	ids, err := c.rdb.ZRange(ctx, schedulerCandidateKey(bucket), offset, offset+int64(limit)-1).Result()
	if err != nil {
		return service.SchedulerCandidatePage{}, false, err
	}
	if len(ids) == 0 {
		return service.SchedulerCandidatePage{}, false, nil
	}
	keys := make([]string, 0, len(ids))
	for _, id := range ids {
		keys = append(keys, schedulerAccountMetaKey(id))
	}
	values, err := c.mgetChunked(ctx, keys)
	if err != nil {
		return service.SchedulerCandidatePage{}, false, err
	}
	accounts := make([]*service.Account, 0, len(values))
	for _, value := range values {
		if value == nil {
			// A ready index with missing metadata is incomplete. The caller will
			// rebuild this bucket instead of silently skipping an account.
			return service.SchedulerCandidatePage{}, false, nil
		}
		account, err := decodeCachedAccount(value)
		if err != nil {
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

func (c *schedulerCache) SetCandidateIndex(ctx context.Context, bucket service.SchedulerBucket, accounts []service.Account) error {
	if err := c.writeAccounts(ctx, accounts); err != nil {
		return err
	}
	readyKey := schedulerBucketKey(schedulerCandidateReady, bucket)
	if err := c.rdb.Set(ctx, readyKey, "building", 0).Err(); err != nil {
		return err
	}

	candidateKey := schedulerCandidateKey(bucket)
	tmpKey := fmt.Sprintf("%s:tmp:%d", candidateKey, time.Now().UnixNano())
	newIDs := make(map[string]struct{}, len(accounts))
	members := make([]redis.Z, 0, len(accounts))
	for _, account := range accounts {
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

	oldIDs, err := c.rdb.ZRange(ctx, candidateKey, 0, -1).Result()
	if err != nil && err != redis.Nil {
		return err
	}
	if len(members) > 0 {
		if err := c.rdb.Rename(ctx, tmpKey, candidateKey).Err(); err != nil {
			return err
		}
	} else if err := c.rdb.Del(ctx, candidateKey, tmpKey).Err(); err != nil {
		return err
	}

	bucketName := bucket.String()
	pipe := c.rdb.Pipeline()
	for _, id := range oldIDs {
		if _, keep := newIDs[id]; !keep {
			pipe.SRem(ctx, schedulerAccountCandidateBucketsKey(id), bucketName)
		}
	}
	for id := range newIDs {
		pipe.SAdd(ctx, schedulerAccountCandidateBucketsKey(id), bucketName)
	}
	pipe.Set(ctx, schedulerBucketKey(schedulerCandidateCount, bucket), len(members), 0)
	pipe.Set(ctx, readyKey, service.SchedulerCandidateIndexVersion, 0)
	pipe.SAdd(ctx, schedulerCandidateBuckets, bucketName)
	pipe.SAdd(ctx, schedulerBucketSetKey, bucketName)
	_, err = pipe.Exec(ctx)
	return err
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

func ptrTime(t time.Time) *time.Time {
	return &t
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

func (c *schedulerCache) writeAccounts(ctx context.Context, accounts []service.Account) ([]service.Account, error) {
	if len(accounts) == 0 {
		return nil, nil
	}

	pipe := c.rdb.Pipeline()
	cacheableAccounts := make([]service.Account, 0, len(accounts))
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
		cacheableAccounts = append(cacheableAccounts, account)
		pending++
		if pending >= c.writeChunkSize {
			if err := flush(); err != nil {
				return nil, err
			}
		}
	}

	if err := flush(); err != nil {
		return nil, err
	}
	return cacheableAccounts, nil
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
		AccountGroups:           filterSchedulerAccountGroups(account.AccountGroups),
		GroupIDs:                filterSchedulerGroupIDs(account.GroupIDs, account.AccountGroups),
		Credentials:             filterSchedulerCredentials(account.Credentials),
		Extra:                   filterSchedulerExtra(account.Extra),
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
	keys := []string{"model_mapping", "compact_model_mapping", "api_key", "project_id", "oauth_type", "plan_type"}
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
		"mixed_scheduling",
		"window_cost_limit",
		"window_cost_sticky_reserve",
		"max_sessions",
		"session_idle_timeout_minutes",
		"base_rpm",
		"rpm_strategy",
		"rpm_sticky_buffer",
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
		"model_rate_limits",
	}
	filtered := make(map[string]any)
	for _, key := range keys {
		if value, ok := extra[key]; ok && value != nil {
			filtered[key] = value
		}
	}
	if len(filtered) == 0 {
		return nil
	}
	return filtered
}
