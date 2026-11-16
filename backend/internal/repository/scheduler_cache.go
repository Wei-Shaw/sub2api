package repository

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"runtime/debug"
	"slices"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/redis/go-redis/v9"
	"github.com/tidwall/gjson"
	"golang.org/x/sync/singleflight"
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
	// 账号精简记录（sched:meta:<id>）的修订号：全局修订号与按平台划分的修订号（前缀后接平台名）。
	// 改写精简记录后写入所涉平台的修订号；删除记录、改写平台为空或平台发生变化的记录、写入出错时写入全局修订号。
	// 取值都是新的随机串，读侧只比较是否相等，键丢失后重建也不会与旧取值相同；
	// Redis 回滚恢复出缓存记录过的旧取值时，由进程内快照缓存的直接复用时长与宽限期兜底。
	schedulerAccountMetaRevisionKey            = "sched:meta:rev"
	schedulerAccountMetaPlatformRevisionPrefix = "sched:meta:rev:"

	defaultSchedulerSnapshotMGetChunkSize  = 128
	defaultSchedulerSnapshotWriteChunkSize = 256
	schedulerLastUsedUpdateChunkSize       = 256

	// snapshotGraceTTLSeconds 旧快照过期的宽限期（秒）。
	// 替代立即 DEL，让正在读取旧版本的 reader 有足够时间完成 ZRANGE。
	snapshotGraceTTLSeconds = 60

	// schedulerSnapshotCacheMaxAge 是进程内快照缓存条目的直接复用时长，与 outbox_lag_rebuild_seconds 默认 10 秒的陈旧容忍度相同。
	// 未推进修订号的精简记录改写在本进程的可见延迟不超过该时长与 schedulerSnapshotCacheStaleGrace 之和（另加一次加载耗时）。
	schedulerSnapshotCacheMaxAge = 10 * time.Second
	// schedulerSnapshotCacheStaleGrace：条目超过直接复用时长、但 (active, 修订号) 仍与 Redis 一致时，
	// 在该宽限期内继续返回该条目，同时触发一次后台加载；超过宽限期后调用方等待加载结果。
	schedulerSnapshotCacheStaleGrace = 20 * time.Second
	// schedulerSnapshotCacheEvictAge：写入条目时，加载时间早于该时长的其它条目一并清除，
	// 已退休分组的条目不会长期驻留。
	schedulerSnapshotCacheEvictAge = 10 * schedulerSnapshotCacheMaxAge
	// schedulerLastUsedCacheMaxAge 是条目内 last_used 读取结果的复用时长；本进程写入 last_used 后该结果立即失效。
	schedulerLastUsedCacheMaxAge = 200 * time.Millisecond
	// schedulerSharedLoadTimeout 是合并加载（快照条目、last_used）的上下文时限，约束取连接与重试；
	// 单次套接字读取另受 Redis 客户端读超时约束。合并加载不随单个调用方取消。
	schedulerSharedLoadTimeout = 10 * time.Second

	schedulerAccountMetaRevisionBytes        = 16
	schedulerAccountMetaRevisionWriteTimeout = 3 * time.Second
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
)

type schedulerCache struct {
	rdb            *redis.Client
	mgetChunkSize  int
	writeChunkSize int

	// now 是进程内快照缓存判定条目年龄的时钟。
	now func() time.Time

	snapshotMu      sync.RWMutex
	snapshotEntries map[string]*schedulerSnapshotEntry
	// snapshotLoads 合并同一 bucket、同一组 (active, 修订号) 的条目加载。
	snapshotLoads singleflight.Group
	// lastUsedGeneration 在本进程每次写入 last_used 后递增，条目内缓存的 last_used 读取结果随之失效。
	lastUsedGeneration atomic.Uint64

	// globalRevisionPending 与 globalRevisionWriting 合并写入出错后的后台全局修订号写入，见 scheduleGlobalAccountMetaRevision。
	globalRevisionPending atomic.Bool
	globalRevisionWriting atomic.Bool
}

// schedulerSnapshotEntry 是一个 bucket 的已解码快照。
// revisions 依次是全局修订号与该 bucket 成员可能涉及的各平台修订号（见 schedulerSnapshotTagKeys）。
// accounts 是按快照顺序排列的母本，发布后只读，指针不外泄；母本可以同时被多个条目引用。
// ids、payloads、lastUsedKeys 与 accounts 一一对齐，payloads 是解码出母本的精简记录原文。
// 每次读取都按值复制出本次请求独占的副本再套用 last_used。
type schedulerSnapshotEntry struct {
	active       string
	revisions    []string
	loadedAt     time.Time
	ids          []string
	accounts     []*service.Account
	payloads     []string
	lastUsedKeys []string

	lastUsed      atomic.Pointer[schedulerLastUsedValues]
	lastUsedLoads singleflight.Group
	// refreshing 在宽限期内的后台加载进行期间为 true，同一条目同时只发起一次后台加载。
	refreshing atomic.Bool
}

// schedulerLastUsedValues 是条目的一次 last_used 读取结果，与条目的 accounts 一一对齐。
// nil 表示沿用母本的 LastUsedAt；非 nil 的时间晚于母本，由各次读取共享，不得原地修改。
type schedulerLastUsedValues struct {
	fetchedAt  time.Time
	generation uint64
	values     []*time.Time
}

func (v *schedulerLastUsedValues) freshAt(now time.Time, generation uint64) bool {
	if v == nil || v.generation != generation {
		return false
	}
	age := now.Sub(v.fetchedAt)
	return age >= 0 && age < schedulerLastUsedCacheMaxAge
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
		rdb:             rdb,
		mgetChunkSize:   mgetChunkSize,
		writeChunkSize:  writeChunkSize,
		now:             time.Now,
		snapshotEntries: make(map[string]*schedulerSnapshotEntry),
	}
}

// GetSnapshot 读取 bucket 的激活快照，语义与返回账号的使用约束同 GetSnapshotValues。
func (c *schedulerCache) GetSnapshot(ctx context.Context, bucket service.SchedulerBucket) ([]*service.Account, bool, error) {
	values, hit, err := c.GetSnapshotValues(ctx, bucket)
	if err != nil || !hit {
		return nil, false, err
	}
	accounts := make([]*service.Account, len(values))
	for i := range values {
		accounts[i] = &values[i]
	}
	return accounts, true, nil
}

// GetSnapshotValues 读取 bucket 的激活快照。解码后的精简记录按 bucket 常驻进程内存：
// ready、active 版本、全局修订号与该 bucket 所涉平台的修订号都与 Redis 一致且未超过直接复用时长时直接复用；
// (active, 修订号) 一致但超过直接复用时长的条目在宽限期内继续返回，同时触发一次后台加载；其余情况等待加载结果。
// 同一 bucket、同一组 (active, 修订号) 的并发加载合并为一次；加载时精简记录原文未变、
// 且预解析结果对应当前运行时模型映射版本的账号沿用已解码的母本。
// last_used 读取结果在条目内复用 schedulerLastUsedCacheMaxAge。
// 返回的切片与其中的账号由本次调用独占，可以修改账号的顶层字段；map、切片底层数组与指针指向的值与其它调用共享，不得原地修改。
func (c *schedulerCache) GetSnapshotValues(ctx context.Context, bucket service.SchedulerBucket) ([]service.Account, bool, error) {
	entry, err := c.snapshotEntry(ctx, bucket)
	if err != nil {
		return nil, false, err
	}
	if entry == nil {
		return nil, false, nil
	}
	accounts, err := c.snapshotAccountsFromEntry(ctx, entry)
	if err != nil {
		return nil, false, err
	}
	return accounts, true, nil
}

// snapshotEntry 返回 bucket 当前可用的条目；nil 表示快照未命中。
func (c *schedulerCache) snapshotEntry(ctx context.Context, bucket service.SchedulerBucket) (*schedulerSnapshotEntry, error) {
	bucketKey := bucket.String()
	tagKeys := schedulerSnapshotTagKeys(bucket)
	tags, err := c.rdb.MGet(ctx, tagKeys...).Result()
	if err != nil {
		return nil, err
	}
	if len(tags) != len(tagKeys) {
		return nil, fmt.Errorf("unexpected scheduler snapshot tag count: got %d, want %d", len(tags), len(tagKeys))
	}
	readyVal, err := schedulerCacheOptionalString(tags[0])
	if err != nil {
		return nil, err
	}
	activeVal, err := schedulerCacheOptionalString(tags[1])
	if err != nil {
		return nil, err
	}
	// 修订号缺失（nil）按空字符串比较：它是一个独立取值，不会与任何写入过的随机修订号相等。
	revisions := make([]string, 0, len(tags)-2)
	for _, tag := range tags[2:] {
		revision, err := schedulerCacheOptionalString(tag)
		if err != nil {
			return nil, err
		}
		revisions = append(revisions, revision)
	}
	if readyVal != "1" || activeVal == "" {
		c.dropSnapshotEntry(bucketKey)
		return nil, nil
	}

	now := c.now()
	if entry := c.lookupSnapshotEntry(bucketKey); entry != nil && entry.sameVersion(activeVal, revisions) {
		age := now.Sub(entry.loadedAt)
		if age >= 0 && age < schedulerSnapshotCacheMaxAge {
			return entry, nil
		}
		if age >= 0 && age < schedulerSnapshotCacheMaxAge+schedulerSnapshotCacheStaleGrace {
			if entry.refreshing.CompareAndSwap(false, true) {
				results := c.loadSnapshotEntryShared(ctx, bucket, bucketKey, activeVal, revisions, now)
				go func() {
					if result := <-results; result.Err != nil {
						slog.Warn("scheduler snapshot background reload failed", "bucket", bucketKey, "error", result.Err)
					}
					entry.refreshing.Store(false)
				}()
			}
			return entry, nil
		}
	}

	select {
	case result := <-c.loadSnapshotEntryShared(ctx, bucket, bucketKey, activeVal, revisions, now):
		if result.Err != nil {
			return nil, result.Err
		}
		entry, _ := result.Val.(*schedulerSnapshotEntry)
		return entry, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// loadSnapshotEntryShared 发起或加入 bucket 在给定 (active, 修订号) 下的条目加载，结果的 Val 为 nil 表示快照未命中。
// 加载使用与调用方取消解耦、时限为 schedulerSharedLoadTimeout 的上下文。
func (c *schedulerCache) loadSnapshotEntryShared(ctx context.Context, bucket service.SchedulerBucket, bucketKey, active string, revisions []string, now time.Time) <-chan singleflight.Result {
	key := bucketKey + "\x00" + active + "\x00" + strings.Join(revisions, "\x00")
	loadCtx := context.WithoutCancel(ctx)
	return c.snapshotLoads.DoChan(key, recoverSchedulerSharedLoad("scheduler snapshot load", func() (any, error) {
		ctx, cancel := context.WithTimeout(loadCtx, schedulerSharedLoadTimeout)
		defer cancel()
		entry, err := c.loadSnapshotEntry(ctx, bucket, bucketKey, active, revisions, now)
		if err != nil || entry == nil {
			return nil, err
		}
		return entry, nil
	}))
}

// loadSnapshotEntry 从 Redis 加载条目并写入进程内缓存，返回 nil 表示快照未命中。
// 条目记录的是加载前读到的 (active, 修订号) 与当时的时钟 now：精简记录读取晚于这组取值，
// 之后若再有改写，修订号或 active 必然不同，或在直接复用时长与宽限期之和后不再返回。
// 进程内已有同一 (active, 修订号) 且未过期的条目时直接返回它。
func (c *schedulerCache) loadSnapshotEntry(ctx context.Context, bucket service.SchedulerBucket, bucketKey, active string, revisions []string, now time.Time) (*schedulerSnapshotEntry, error) {
	previous := c.lookupSnapshotEntry(bucketKey)
	if previous != nil && previous.matches(active, revisions, c.now()) {
		return previous, nil
	}

	ids, err := c.rdb.ZRange(ctx, schedulerSnapshotKey(bucket, active), 0, -1).Result()
	if err != nil {
		return nil, err
	}
	if len(ids) == 0 {
		// 空快照视为缓存未命中，触发数据库回退查询
		// 这解决了新分组创建后立即绑定账号时的竞态条件问题
		c.dropSnapshotEntry(bucketKey)
		return nil, nil
	}

	keys := make([]string, 0, len(ids))
	lastUsedKeys := make([]string, 0, len(ids))
	for _, id := range ids {
		keys = append(keys, schedulerAccountMetaKey(id))
		lastUsedKeys = append(lastUsedKeys, schedulerLastUsedKey(id))
	}
	values, err := c.mgetChunked(ctx, keys)
	if err != nil {
		return nil, err
	}
	if len(values) != len(ids) {
		return nil, fmt.Errorf("unexpected scheduler account metadata count: got %d, want %d", len(values), len(ids))
	}

	reusable := previous.masterIndex()
	masters := make([]*service.Account, len(values))
	payloads := make([]string, len(values))
	for i, val := range values {
		if val == nil {
			c.dropSnapshotEntry(bucketKey)
			return nil, nil
		}
		payload, err := schedulerCachePayload(val)
		if err != nil {
			return nil, err
		}
		if j, ok := reusable[ids[i]]; ok && previous.payloads[j] == payload && previous.accounts[j].SharedSnapshotCurrent() {
			masters[i] = previous.accounts[j]
			payloads[i] = previous.payloads[j]
			continue
		}
		account, err := decodeCachedAccountPayload(payload)
		if err != nil {
			return nil, err
		}
		// 母本发布进缓存前由本 goroutine 完成预处理，发布后只读。
		account.PrepareSharedSnapshot()
		masters[i] = account
		payloads[i] = payload
	}

	entry := &schedulerSnapshotEntry{
		active:       active,
		revisions:    revisions,
		loadedAt:     now,
		ids:          ids,
		accounts:     masters,
		payloads:     payloads,
		lastUsedKeys: lastUsedKeys,
	}
	c.storeSnapshotEntry(bucketKey, entry)
	return entry, nil
}

func (e *schedulerSnapshotEntry) sameVersion(active string, revisions []string) bool {
	return e.active == active && slices.Equal(e.revisions, revisions)
}

func (e *schedulerSnapshotEntry) matches(active string, revisions []string, now time.Time) bool {
	if !e.sameVersion(active, revisions) {
		return false
	}
	age := now.Sub(e.loadedAt)
	return age >= 0 && age < schedulerSnapshotCacheMaxAge
}

// masterIndex 返回账号 ID 到母本下标的映射；条目为 nil 或缺少原文时返回 nil。
func (e *schedulerSnapshotEntry) masterIndex() map[string]int {
	if e == nil || len(e.ids) == 0 || len(e.ids) != len(e.accounts) || len(e.ids) != len(e.payloads) {
		return nil
	}
	index := make(map[string]int, len(e.ids))
	for i, id := range e.ids {
		index[id] = i
	}
	return index
}

// snapshotAccountsFromEntry 为本次调用一次性分配一组母本副本，并套用 last_used。
func (c *schedulerCache) snapshotAccountsFromEntry(ctx context.Context, entry *schedulerSnapshotEntry) ([]service.Account, error) {
	lastUsed, err := c.entryLastUsed(ctx, entry)
	if err != nil {
		return nil, err
	}
	accounts := make([]service.Account, len(entry.accounts))
	for i, master := range entry.accounts {
		accounts[i] = *master
		if lastUsed[i] != nil {
			accounts[i].LastUsedAt = lastUsed[i]
		}
	}
	return accounts, nil
}

// entryLastUsed 返回条目的 last_used 读取结果：结果未超过 schedulerLastUsedCacheMaxAge 且本进程此后未写入 last_used 时直接复用，
// 否则等待一次合并后的读取。
func (c *schedulerCache) entryLastUsed(ctx context.Context, entry *schedulerSnapshotEntry) ([]*time.Time, error) {
	generation := c.lastUsedGeneration.Load()
	if current := entry.lastUsed.Load(); current.freshAt(c.now(), generation) {
		return current.values, nil
	}
	loadCtx := context.WithoutCancel(ctx)
	results := entry.lastUsedLoads.DoChan(strconv.FormatUint(generation, 10), recoverSchedulerSharedLoad("scheduler last_used load", func() (any, error) {
		fetchedAt := c.now()
		if current := entry.lastUsed.Load(); current.freshAt(fetchedAt, generation) {
			return current, nil
		}
		ctx, cancel := context.WithTimeout(loadCtx, schedulerSharedLoadTimeout)
		defer cancel()
		raw, err := c.mgetChunked(ctx, entry.lastUsedKeys)
		if err != nil {
			return nil, err
		}
		if len(raw) != len(entry.accounts) {
			return nil, fmt.Errorf("unexpected scheduler last_used value count: got %d, want %d", len(raw), len(entry.accounts))
		}
		values := make([]*time.Time, len(raw))
		for i, value := range raw {
			lastUsedAt, err := parseSchedulerLastUsed(value)
			if err != nil {
				return nil, err
			}
			if lastUsedAt != nil && (entry.accounts[i].LastUsedAt == nil || lastUsedAt.After(*entry.accounts[i].LastUsedAt)) {
				values[i] = lastUsedAt
			}
		}
		loaded := &schedulerLastUsedValues{fetchedAt: fetchedAt, generation: generation, values: values}
		entry.lastUsed.Store(loaded)
		return loaded, nil
	}))
	select {
	case result := <-results:
		if result.Err != nil {
			return nil, result.Err
		}
		loaded, _ := result.Val.(*schedulerLastUsedValues)
		if loaded == nil {
			return nil, errors.New("scheduler last_used load returned no values")
		}
		return loaded.values, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// recoverSchedulerSharedLoad 包装合并加载：加载 panic 时记录堆栈并转成错误交给全部等待方。
func recoverSchedulerSharedLoad(name string, load func() (any, error)) func() (any, error) {
	return func() (result any, err error) {
		defer func() {
			if recovered := recover(); recovered != nil {
				slog.Error("scheduler shared load panicked", "load", name, "panic", recovered, "stack", string(debug.Stack()))
				result, err = nil, fmt.Errorf("%s panicked: %v", name, recovered)
			}
		}()
		return load()
	}
}

func (c *schedulerCache) lookupSnapshotEntry(bucketKey string) *schedulerSnapshotEntry {
	c.snapshotMu.RLock()
	defer c.snapshotMu.RUnlock()
	return c.snapshotEntries[bucketKey]
}

// storeSnapshotEntry 写入条目，并清除加载时间早于 schedulerSnapshotCacheEvictAge 的其它条目。
// 已有条目的加载时间晚于新条目时保留已有条目：它基于更晚读到的 (active, 修订号)。
func (c *schedulerCache) storeSnapshotEntry(bucketKey string, entry *schedulerSnapshotEntry) {
	c.snapshotMu.Lock()
	defer c.snapshotMu.Unlock()
	if current := c.snapshotEntries[bucketKey]; current == nil || !current.loadedAt.After(entry.loadedAt) {
		c.snapshotEntries[bucketKey] = entry
	}
	for key, other := range c.snapshotEntries {
		if key != bucketKey && entry.loadedAt.Sub(other.loadedAt) > schedulerSnapshotCacheEvictAge {
			delete(c.snapshotEntries, key)
		}
	}
}

func (c *schedulerCache) dropSnapshotEntry(bucketKey string) {
	c.snapshotMu.RLock()
	_, ok := c.snapshotEntries[bucketKey]
	c.snapshotMu.RUnlock()
	if !ok {
		return
	}
	c.snapshotMu.Lock()
	delete(c.snapshotEntries, bucketKey)
	c.snapshotMu.Unlock()
}

// schedulerSnapshotTagKeys 返回 GetSnapshot 一次读取的键：ready、active、全局修订号、bucket 平台的修订号，
// mixed 模式再加 antigravity 平台的修订号。按平台判定失效依赖 bucket 成员只取自 bucket 平台
// （mixed 模式另含开启混合调度的 antigravity 账号）；账号平台发生变化时，写入方另行写入全局修订号。
func schedulerSnapshotTagKeys(bucket service.SchedulerBucket) []string {
	keys := make([]string, 0, 5)
	keys = append(keys,
		schedulerBucketKey(schedulerReadyPrefix, bucket),
		schedulerBucketKey(schedulerActivePrefix, bucket),
		schedulerAccountMetaRevisionKey,
		schedulerAccountMetaRevisionKeyForPlatform(bucket.Platform),
	)
	if bucket.Mode == service.SchedulerModeMixed && bucket.Platform != service.PlatformAntigravity {
		keys = append(keys, schedulerAccountMetaRevisionKeyForPlatform(service.PlatformAntigravity))
	}
	return keys
}

// schedulerAccountMetaRevisionKeyForPlatform 返回平台修订号的键；平台为空时返回全局修订号的键。
func schedulerAccountMetaRevisionKeyForPlatform(platform string) string {
	if platform == "" {
		return schedulerAccountMetaRevisionKey
	}
	return schedulerAccountMetaPlatformRevisionPrefix + platform
}

func schedulerCacheOptionalString(value any) (string, error) {
	switch typed := value.(type) {
	case nil:
		return "", nil
	case string:
		return typed, nil
	case []byte:
		return string(typed), nil
	default:
		return "", fmt.Errorf("unexpected scheduler cache value type: %T", value)
	}
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
	}, bucket.String(), snapshotKeyPrefix, snapshotGraceTTLSeconds).Int64()
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
	}, bucket.String(), snapshotKeyPrefix, snapshotGraceTTLSeconds).Int64()
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
	// 删除精简记录与推进全局修订号在同一个 MULTI 中原子执行：读到删除结果的读者也读到新修订号，
	// 进程内快照缓存不会给出已删除的账号。
	pipe := c.rdb.TxPipeline()
	pipe.Del(ctx, schedulerAccountKey(id), schedulerAccountMetaKey(id), schedulerLastUsedKey(id))
	pipe.Set(ctx, schedulerAccountMetaRevisionKey, newSchedulerAccountMetaRevision(), 0)
	_, err := pipe.Exec(ctx)
	return err
}

func (c *schedulerCache) UpdateLastUsed(ctx context.Context, updates map[int64]time.Time) error {
	if len(updates) == 0 {
		return nil
	}

	pipe := c.rdb.Pipeline()
	queued := 0
	metaDeleted := false
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
			metaDeleted = true
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
	if metaDeleted {
		// 被删账号的平台未知，全局修订号排在同一 pipeline 的 DEL 之后。
		pipe.Set(ctx, schedulerAccountMetaRevisionKey, newSchedulerAccountMetaRevision(), 0)
	}
	_, err := pipe.Exec(ctx)
	// 写入结束（含部分失败）后才推进本进程的代际：此后发起的 last_used 读取都排在写入之后。
	c.lastUsedGeneration.Add(1)
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

func schedulerLastUsedKey(id string) string {
	return schedulerAccountLastUsedPrefix + id
}

func ptrTime(t time.Time) *time.Time {
	return &t
}

func schedulerLastUsedMillis(value time.Time) (int64, error) {
	if _, err := value.MarshalJSON(); err != nil {
		return 0, err
	}
	return value.UTC().UnixMilli(), nil
}

func applySchedulerLastUsed(account *service.Account, value any) error {
	if account == nil {
		return nil
	}
	lastUsedAt, err := parseSchedulerLastUsed(value)
	if err != nil {
		return err
	}
	if lastUsedAt != nil && (account.LastUsedAt == nil || lastUsedAt.After(*account.LastUsedAt)) {
		account.LastUsedAt = lastUsedAt
	}
	return nil
}

// parseSchedulerLastUsed 解析 last_used 侧键的毫秒时间戳；键不存在（nil）时返回 nil。
func parseSchedulerLastUsed(value any) (*time.Time, error) {
	if value == nil {
		return nil, nil
	}
	var raw string
	switch typed := value.(type) {
	case string:
		raw = typed
	case []byte:
		raw = string(typed)
	default:
		return nil, fmt.Errorf("unexpected last_used cache type: %T", value)
	}
	millis, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		return nil, fmt.Errorf("invalid last_used cache value %q: %w", raw, err)
	}
	return ptrTime(time.UnixMilli(millis).UTC()), nil
}

func decodeCachedAccount(val any) (*service.Account, error) {
	payload, err := schedulerCachePayload(val)
	if err != nil {
		return nil, err
	}
	return decodeCachedAccountPayload(payload)
}

// schedulerCachePayload 取出 Redis 返回的账号记录原文。
func schedulerCachePayload(val any) (string, error) {
	switch raw := val.(type) {
	case string:
		return raw, nil
	case []byte:
		return string(raw), nil
	default:
		return "", fmt.Errorf("unexpected account cache type: %T", val)
	}
}

func decodeCachedAccountPayload(payload string) (*service.Account, error) {
	var account service.Account
	if err := json.Unmarshal([]byte(payload), &account); err != nil {
		return nil, err
	}
	return &account, nil
}

// writeAccountIDs 分块写入整账号与精简记录，返回成功编码并写入的有序账号 ID。
// 每块在追加下一个账号前才执行，最后一块总是非空；所涉平台的修订号追加在最后一块末尾，
// 与最后一批精简记录同一次往返发出，且排在全部精简记录写入之后。
// 精简记录用 SET ... GET 写入并取回旧值：旧值的平台与新平台不同时，旧平台 bucket 中可能仍缓存着该账号，
// 全部写入完成后另行写入全局修订号。该检测尽力而为（客户端在 pipeline 内部重试时取回的可能已是本次写入的值），
// 漏检由进程内快照缓存的直接复用时长与宽限期兜底。
// 任一块执行出错时，此前已执行的块与出错的块都可能已有精简记录生效，返回时启动后台写入全局修订号。
func (c *schedulerCache) writeAccountIDs(ctx context.Context, accounts []service.Account) ([]int64, error) {
	if len(accounts) == 0 {
		return nil, nil
	}

	type metaWrite struct {
		platform string
		previous *redis.StatusCmd
	}
	pipe := c.rdb.Pipeline()
	accountIDs := make([]int64, 0, len(accounts))
	platforms := make([]string, 0, 1)
	writes := make([]metaWrite, 0, min(len(accounts), c.writeChunkSize))
	pending := 0
	execFailed := false
	platformChanged := false
	defer func() {
		if execFailed {
			c.scheduleGlobalAccountMetaRevision(ctx)
		}
	}()
	exec := func() error {
		cmds, err := pipe.Exec(ctx)
		if err != nil && !errors.Is(err, redis.Nil) {
			execFailed = true
			return err
		}
		// SET ... GET 在键原本不存在时返回 redis.Nil，不算失败；其余命令的错误逐条检查。
		for _, cmd := range cmds {
			if cmdErr := cmd.Err(); cmdErr != nil && !errors.Is(cmdErr, redis.Nil) {
				execFailed = true
				return cmdErr
			}
		}
		for _, write := range writes {
			if schedulerMetaPlatformChanged(write.previous, write.platform) {
				platformChanged = true
			}
		}
		writes = writes[:0]
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
		if pending >= c.writeChunkSize {
			if err := exec(); err != nil {
				return nil, err
			}
		}

		id := strconv.FormatInt(account.ID, 10)
		pipe.Set(ctx, schedulerAccountKey(id), fullPayload, 0)
		writes = append(writes, metaWrite{
			platform: account.Platform,
			previous: pipe.SetArgs(ctx, schedulerAccountMetaKey(id), metaPayload, redis.SetArgs{Get: true}),
		})
		// Keep the hot LastUsedAt side key untouched: a lagging snapshot rebuild
		// must not overwrite a newer scheduler update.
		accountIDs = append(accountIDs, account.ID)
		if !slices.Contains(platforms, account.Platform) {
			platforms = append(platforms, account.Platform)
		}
		pending++
	}

	if pending == 0 {
		return accountIDs, nil
	}
	for _, platform := range platforms {
		pipe.Set(ctx, schedulerAccountMetaRevisionKeyForPlatform(platform), newSchedulerAccountMetaRevision(), 0)
	}
	if err := exec(); err != nil {
		return nil, err
	}
	if platformChanged {
		c.advanceGlobalAccountMetaRevision(ctx)
	}
	return accountIDs, nil
}

// schedulerMetaPlatformChanged 判断 SET ... GET 取回的旧精简记录是否属于另一个平台；
// 旧值不是合法 JSON 或缺少字符串类型的顶层 Platform 时按已变化处理。
func schedulerMetaPlatformChanged(previous *redis.StatusCmd, platform string) bool {
	raw, err := previous.Result()
	if err != nil {
		return false
	}
	if !gjson.Valid(raw) {
		return true
	}
	old := gjson.Get(raw, "Platform")
	return old.Type != gjson.String || old.Str != platform
}

// scheduleGlobalAccountMetaRevision 在后台写入全局修订号，不阻塞调用方。
// 同一时刻至多一个后台写入：写入前先清除待写标记，写完后标记再次被置位就再写一轮，
// 因此每次调用之后都至少还有一次完整的写入发生在它之后。
func (c *schedulerCache) scheduleGlobalAccountMetaRevision(ctx context.Context) {
	c.globalRevisionPending.Store(true)
	if !c.globalRevisionWriting.CompareAndSwap(false, true) {
		return
	}
	go func() {
		for {
			c.globalRevisionPending.Store(false)
			c.advanceGlobalAccountMetaRevision(ctx)
			c.globalRevisionWriting.Store(false)
			if !c.globalRevisionPending.Load() || !c.globalRevisionWriting.CompareAndSwap(false, true) {
				return
			}
		}
	}()
}

// advanceGlobalAccountMetaRevision 单独写入新的全局修订号，使所有 bucket 的进程内快照失效。
// 不受调用方取消影响；失败只记录日志，由进程内快照缓存的直接复用时长与宽限期兜底。
func (c *schedulerCache) advanceGlobalAccountMetaRevision(ctx context.Context) {
	writeCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), schedulerAccountMetaRevisionWriteTimeout)
	defer cancel()
	if err := c.rdb.Set(writeCtx, schedulerAccountMetaRevisionKey, newSchedulerAccountMetaRevision(), 0).Err(); err != nil {
		slog.Warn("scheduler cache failed to advance account metadata revision", "error", err)
	}
}

func newSchedulerAccountMetaRevision() string {
	raw := make([]byte, schedulerAccountMetaRevisionBytes)
	// crypto/rand.Read 读取失败时直接终止进程，不会返回错误。
	_, _ = rand.Read(raw)
	return hex.EncodeToString(raw)
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

	chunkSize := c.mgetChunkSize
	if chunkSize <= 0 {
		chunkSize = defaultSchedulerSnapshotMGetChunkSize
	}
	if len(keys) <= chunkSize {
		return c.rdb.MGet(ctx, keys...).Result()
	}
	// 多个分块的 MGET 放在同一个 pipeline 里，一次往返发出。
	pipe := c.rdb.Pipeline()
	cmds := make([]*redis.SliceCmd, 0, (len(keys)+chunkSize-1)/chunkSize)
	for start := 0; start < len(keys); start += chunkSize {
		end := start + chunkSize
		if end > len(keys) {
			end = len(keys)
		}
		cmds = append(cmds, pipe.MGet(ctx, keys[start:end]...))
	}
	if _, err := pipe.Exec(ctx); err != nil {
		return nil, err
	}
	out := make([]any, 0, len(keys))
	for _, cmd := range cmds {
		out = append(out, cmd.Val()...)
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
	// Candidate-list admission evaluates the account override before hydrating
	// the full account. Dropping it silently falls back to the platform threshold.
	keys := []string{"model_mapping", "compact_model_mapping", "api_key", "project_id", "oauth_type", "plan_type", "account_scheduling_threshold",
		// OpenAI 端点能力集必须进投影：候选过滤按 SupportsOpenAIEndpointCapability 判
		// capability_mismatch，而 Seedance 分支直接读该键、不走「未配置即放行」。
		// 裁掉后 Seedance 账号在选号阶段必然被剔除，表现为 /v1/models 能列出模型、
		// 建任务恒定 503 seedance_no_eligible_account。
		"openai_capabilities",
		// Gemini 显式缓存的候选过滤（GeminiExplicitCacheExclusions）读本投影判定账号是否支持显式缓存：
		// 官方地址、Vertex 认证方式与上游网关的显式缓存声明。缺这几个键时只能按类型粗筛，
		// 不支持的账号要逐个选中后才被剔除。
		"base_url", "auth_mode", service.GeminiExplicitCacheUpstreamCredentialKey, service.GeminiExplicitCacheUpstreamMaxTTLCredentialKey}
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
		// Anthropic shared-window and Fable-only threshold checks run on this
		// projection. UpdateExtra refreshes both payloads without a bucket rebuild.
		"session_window_utilization",
		"passive_usage_7d_utilization",
		"passive_usage_7d_reset",
		"passive_usage_7d_oi_utilization",
		"passive_usage_7d_oi_reset",
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
		// RPM 门与窗口费用门一样跑在本投影上：isAccountSchedulableForRPM 读 base_rpm，
		// 缺失时 GetBaseRPM() 返回 0 并直接放行，已配置限流的账号会被超额调度。
		"base_rpm",
		"rpm_strategy",
		"rpm_sticky_buffer",
		"max_sessions",
		"session_idle_timeout_minutes",
		"openai_oauth_responses_websockets_v2_enabled",
		"openai_oauth_responses_websockets_v2_mode",
		"openai_apikey_responses_websockets_v2_enabled",
		"openai_apikey_responses_websockets_v2_mode",
		"responses_websockets_v2_enabled",
		"openai_ws_enabled",
		"openai_ws_force_http",
		"openai_responses_mode",
		"openai_responses_supported",
		// 透传开关必须进投影：候选过滤(ListSchedulableAccounts)读的是本投影，
		// 而 Account.IsModelSupported 靠 extra 上的这两个键短路 model_mapping 白名单。
		// 裁掉它们，透传账号在选号阶段会退回按(常为过期的)白名单判定并被误判为
		// model_not_supported —— 转发阶段却仍按透传工作，表现为"单独测账号能通、
		// 走网关报 no available accounts"。
		"openai_passthrough",
		"openai_oauth_passthrough",
		// 生图尺寸档位在候选过滤阶段按本投影判定，裁掉后受限账号会接到不允许的档位。
		service.AccountExtraOpenAIImageSizeTiers,
		"codex_fingerprint_mode",
		"codex_fingerprint_seed",
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
		// 自动用卡：卡可用的 OpenAI 号在暂停阈值与用卡阈值之间继续调度。
		// 候选过滤读的是本投影，缺这几个键时放行分支永远不会生效，
		// 账号会在暂停阈值处被一刀切停调，直到窗口自然重置。
		service.OpenAIAutoResetCreditEnabledExtraKey,
		service.OpenAIAutoResetCredit5hThresholdExtraKey,
		service.OpenAIAutoResetCredit7dThresholdExtraKey,
		service.OpenAIAutoResetCreditStateExtraKey,
		"model_rate_limits",
		service.UpstreamBillingProbeExtraKey,
		service.GrokMediaEligibleExtraKey,
		"grok_billing_snapshot",
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
