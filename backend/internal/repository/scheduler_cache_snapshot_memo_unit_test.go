//go:build unit

package repository

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
)

// schedulerCacheCommandRecorder 记录发往 Redis 的命令（名称 + 首个参数）与每次 pipeline 执行的命令批次，
// 并可让第 N 次 pipeline 执行失败。新连接的握手命令不计入。
type schedulerCacheCommandRecorder struct {
	mu             sync.Mutex
	commands       []string
	batches        [][]string
	pipelines      int
	failPipelineAt int
}

func (r *schedulerCacheCommandRecorder) DialHook(next redis.DialHook) redis.DialHook { return next }

func (r *schedulerCacheCommandRecorder) ProcessHook(next redis.ProcessHook) redis.ProcessHook {
	return func(ctx context.Context, cmd redis.Cmder) error {
		if !isSchedulerCacheHandshakeCommand(cmd) {
			r.record(cmd)
		}
		return next(ctx, cmd)
	}
}

func (r *schedulerCacheCommandRecorder) ProcessPipelineHook(next redis.ProcessPipelineHook) redis.ProcessPipelineHook {
	return func(ctx context.Context, cmds []redis.Cmder) error {
		handshake := len(cmds) > 0
		for _, cmd := range cmds {
			handshake = handshake && isSchedulerCacheHandshakeCommand(cmd)
		}
		if handshake {
			return next(ctx, cmds)
		}
		r.mu.Lock()
		r.pipelines++
		fail := r.failPipelineAt > 0 && r.pipelines == r.failPipelineAt
		r.mu.Unlock()
		batch := make([]string, 0, len(cmds))
		for _, cmd := range cmds {
			batch = append(batch, r.record(cmd))
		}
		r.mu.Lock()
		r.batches = append(r.batches, batch)
		r.mu.Unlock()
		if fail {
			err := errors.New("injected pipeline failure")
			for _, cmd := range cmds {
				cmd.SetErr(err)
			}
			return err
		}
		return next(ctx, cmds)
	}
}

func isSchedulerCacheHandshakeCommand(cmd redis.Cmder) bool {
	switch strings.ToLower(cmd.Name()) {
	case "hello", "auth", "client", "select":
		return true
	default:
		return false
	}
}

func (r *schedulerCacheCommandRecorder) record(cmd redis.Cmder) string {
	entry := strings.ToLower(cmd.Name())
	if args := cmd.Args(); len(args) > 1 {
		entry += " " + fmt.Sprint(args[1])
	}
	r.mu.Lock()
	r.commands = append(r.commands, entry)
	r.mu.Unlock()
	return entry
}

func (r *schedulerCacheCommandRecorder) reset() {
	r.mu.Lock()
	r.commands = nil
	r.batches = nil
	r.pipelines = 0
	r.mu.Unlock()
}

func (r *schedulerCacheCommandRecorder) pipelineBatches() [][]string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([][]string(nil), r.batches...)
}

func (r *schedulerCacheCommandRecorder) setFailPipelineAt(n int) {
	r.mu.Lock()
	r.failPipelineAt = n
	r.mu.Unlock()
}

func (r *schedulerCacheCommandRecorder) recorded() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string(nil), r.commands...)
}

func (r *schedulerCacheCommandRecorder) countPrefix(prefix string) int {
	count := 0
	for _, command := range r.recorded() {
		if strings.HasPrefix(command, prefix) {
			count++
		}
	}
	return count
}

func newSchedulerCacheMemoUnit(t *testing.T, writeChunkSize int) (*schedulerCache, *miniredis.Miniredis, *schedulerCacheCommandRecorder) {
	t.Helper()
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })
	recorder := &schedulerCacheCommandRecorder{}
	rdb.AddHook(recorder)
	cache, ok := newSchedulerCacheWithChunkSizes(rdb, defaultSchedulerSnapshotMGetChunkSize, writeChunkSize).(*schedulerCache)
	require.True(t, ok)
	return cache, mr, recorder
}

func memoTestBucket(groupID int64) service.SchedulerBucket {
	return service.SchedulerBucket{GroupID: groupID, Platform: service.PlatformAnthropic, Mode: service.SchedulerModeSingle}
}

func memoTestAccount(id int64, name string) service.Account {
	return memoTestAccountOn(service.PlatformAnthropic, id, name)
}

func memoTestAccountOn(platform string, id int64, name string) service.Account {
	return service.Account{
		ID:          id,
		Name:        name,
		Platform:    platform,
		Type:        service.AccountTypeOAuth,
		Status:      service.StatusActive,
		Schedulable: true,
		Concurrency: 2,
		Credentials: map[string]any{
			"model_mapping": map[string]any{"claude-sonnet": "sonnet-" + name},
			"access_token":  "dropped-by-projection",
		},
		Extra: map[string]any{"session_window_utilization": 0.25},
	}
}

func memoPublish(t *testing.T, cache *schedulerCache, bucket service.SchedulerBucket, accounts ...service.Account) {
	t.Helper()
	ctx := context.Background()
	token, err := cache.CaptureBucketWriteToken(ctx, bucket)
	require.NoError(t, err)
	require.NoError(t, cache.SetSnapshot(ctx, bucket, token, accounts))
}

func memoRead(t *testing.T, cache *schedulerCache, bucket service.SchedulerBucket) []*service.Account {
	t.Helper()
	accounts, hit, err := cache.GetSnapshot(context.Background(), bucket)
	require.NoError(t, err)
	require.True(t, hit)
	return accounts
}

func memoRequireMiss(t *testing.T, cache *schedulerCache, bucket service.SchedulerBucket) {
	t.Helper()
	accounts, hit, err := cache.GetSnapshot(context.Background(), bucket)
	require.NoError(t, err)
	require.False(t, hit)
	require.Nil(t, accounts)
}

func memoIDs(accounts []*service.Account) []int64 {
	ids := make([]int64, 0, len(accounts))
	for _, account := range accounts {
		ids = append(ids, account.ID)
	}
	return ids
}

func memoRevision(t *testing.T, cache *schedulerCache, key string) string {
	t.Helper()
	revision, err := cache.rdb.Get(context.Background(), key).Result()
	if errors.Is(err, redis.Nil) {
		return ""
	}
	require.NoError(t, err)
	return revision
}

// memoRevisionValue 读取修订号，不做断言，供 require.Eventually 的条件函数使用。
func memoRevisionValue(cache *schedulerCache, key string) string {
	revision, _ := cache.rdb.Get(context.Background(), key).Result()
	return revision
}

var memoAnthropicRevisionKey = schedulerAccountMetaRevisionKeyForPlatform(service.PlatformAnthropic)

func memoEntryCount(cache *schedulerCache) int {
	cache.snapshotMu.RLock()
	defer cache.snapshotMu.RUnlock()
	return len(cache.snapshotEntries)
}

func TestSchedulerCacheSnapshotMemoSecondReadOnlyFetchesTagsAndLastUsed(t *testing.T) {
	cache, _, recorder := newSchedulerCacheMemoUnit(t, defaultSchedulerSnapshotWriteChunkSize)
	bucket := memoTestBucket(101)
	memoPublish(t, cache, bucket, memoTestAccount(1, "a"), memoTestAccount(2, "b"), memoTestAccount(3, "c"))

	recorder.reset()
	first := memoRead(t, cache, bucket)
	require.Equal(t, 1, recorder.countPrefix("zrange "))
	require.Equal(t, 1, recorder.countPrefix("mget "+schedulerAccountMetaPrefix))

	recorder.reset()
	second := memoRead(t, cache, bucket)
	require.Equal(t, []string{
		"mget " + schedulerBucketKey(schedulerReadyPrefix, bucket),
		"mget " + schedulerLastUsedKey("1"),
	}, recorder.recorded())

	require.Equal(t, []int64{1, 2, 3}, memoIDs(second))
	require.Equal(t, first, second)
	for i := range first {
		require.NotSame(t, first[i], second[i])
	}
	require.Equal(t, "sonnet-b", second[1].GetMappedModel("claude-sonnet"))
	require.NotContains(t, second[1].Credentials, "access_token")
}

func TestSchedulerCacheSnapshotMemoLastUsedIsFreshAndCopiesAreIndependent(t *testing.T) {
	ctx := context.Background()
	cache, _, recorder := newSchedulerCacheMemoUnit(t, defaultSchedulerSnapshotWriteChunkSize)
	bucket := memoTestBucket(102)
	embedded := time.Now().UTC().Truncate(time.Millisecond).Add(-time.Hour)
	account := memoTestAccount(11, "a")
	account.LastUsedAt = &embedded
	memoPublish(t, cache, bucket, account)

	first := memoRead(t, cache, bucket)
	require.Equal(t, embedded, *first[0].LastUsedAt)

	latest := embedded.Add(42 * time.Second)
	require.NoError(t, cache.UpdateLastUsed(ctx, map[int64]time.Time{account.ID: latest}))
	recorder.reset()
	second := memoRead(t, cache, bucket)
	require.Zero(t, recorder.countPrefix("zrange "), "last_used updates must not invalidate the decoded snapshot")
	require.Equal(t, latest, *second[0].LastUsedAt)
	require.Equal(t, embedded, *first[0].LastUsedAt, "an earlier result must not observe later reads")

	// 修改返回对象的顶层字段只影响本次结果。
	second[0].Schedulable = false
	second[0].Name = "mutated"
	second[0].LastUsedAt = nil
	second[0].Credentials = map[string]any{"model_mapping": map[string]any{"claude-sonnet": "replaced"}}
	require.Equal(t, "replaced", second[0].GetMappedModel("claude-sonnet"))

	// 删除 last_used 侧键（不改精简记录）：若母本曾被套用 last_used，这里会读到 latest。
	require.NoError(t, cache.rdb.Del(ctx, schedulerLastUsedKey("11")).Err())
	recorder.reset()
	third := memoRead(t, cache, bucket)
	require.Zero(t, recorder.countPrefix("zrange "))
	require.Equal(t, embedded, *third[0].LastUsedAt)
	require.True(t, third[0].Schedulable)
	require.Equal(t, "a", third[0].Name)
	require.Equal(t, "sonnet-a", third[0].GetMappedModel("claude-sonnet"))
}

func TestSchedulerCacheSnapshotMemoSetAccountIsVisibleImmediately(t *testing.T) {
	ctx := context.Background()
	cache, _, _ := newSchedulerCacheMemoUnit(t, defaultSchedulerSnapshotWriteChunkSize)
	bucket := memoTestBucket(103)
	account := memoTestAccount(21, "a")
	memoPublish(t, cache, bucket, account, memoTestAccount(22, "b"))
	require.True(t, memoRead(t, cache, bucket)[0].Schedulable)

	globalBefore := memoRevision(t, cache, schedulerAccountMetaRevisionKey)
	platformBefore := memoRevision(t, cache, memoAnthropicRevisionKey)
	account.Schedulable = false
	account.Extra = map[string]any{"session_window_utilization": 0.9}
	require.NoError(t, cache.SetAccount(ctx, &account))
	require.NotEqual(t, platformBefore, memoRevision(t, cache, memoAnthropicRevisionKey))
	require.Equal(t, globalBefore, memoRevision(t, cache, schedulerAccountMetaRevisionKey))

	got := memoRead(t, cache, bucket)
	require.Equal(t, []int64{21, 22}, memoIDs(got))
	require.False(t, got[0].Schedulable)
	require.Equal(t, 0.9, got[0].Extra["session_window_utilization"])
	require.True(t, got[1].Schedulable)
}

func TestSchedulerCacheSnapshotMemoDeleteAccountMisses(t *testing.T) {
	ctx := context.Background()
	cache, _, _ := newSchedulerCacheMemoUnit(t, defaultSchedulerSnapshotWriteChunkSize)
	bucket := memoTestBucket(104)
	memoPublish(t, cache, bucket, memoTestAccount(31, "a"), memoTestAccount(32, "b"))
	memoRead(t, cache, bucket)

	revisionBefore := memoRevision(t, cache, schedulerAccountMetaRevisionKey)
	require.NoError(t, cache.DeleteAccount(ctx, 32))
	require.NotEqual(t, revisionBefore, memoRevision(t, cache, schedulerAccountMetaRevisionKey))
	memoRequireMiss(t, cache, bucket)
	require.Zero(t, memoEntryCount(cache))
}

func TestSchedulerCacheSnapshotMemoCrossBucketMetadataRewrite(t *testing.T) {
	ctx := context.Background()
	cache, _, _ := newSchedulerCacheMemoUnit(t, defaultSchedulerSnapshotWriteChunkSize)
	bucketA := memoTestBucket(105)
	bucketB := memoTestBucket(106)
	shared := memoTestAccount(41, "shared")
	memoPublish(t, cache, bucketA, shared)
	memoPublish(t, cache, bucketB, shared, memoTestAccount(42, "b-only"))
	require.Equal(t, "shared", memoRead(t, cache, bucketB)[0].Name)
	activeB := cache.rdb.Get(ctx, schedulerBucketKey(schedulerActivePrefix, bucketB)).Val()

	shared.Name = "shared-renamed"
	shared.Priority = 9
	memoPublish(t, cache, bucketA, shared)

	require.Equal(t, activeB, cache.rdb.Get(ctx, schedulerBucketKey(schedulerActivePrefix, bucketB)).Val())
	got := memoRead(t, cache, bucketB)
	require.Equal(t, []int64{41, 42}, memoIDs(got))
	require.Equal(t, "shared-renamed", got[0].Name)
	require.Equal(t, 9, got[0].Priority)
}

func TestSchedulerCacheSnapshotMemoUnencodableLastUsedDeletionMisses(t *testing.T) {
	ctx := context.Background()
	cache, _, _ := newSchedulerCacheMemoUnit(t, defaultSchedulerSnapshotWriteChunkSize)
	bucket := memoTestBucket(107)
	memoPublish(t, cache, bucket, memoTestAccount(51, "a"), memoTestAccount(52, "b"))
	memoRead(t, cache, bucket)

	revisionBefore := memoRevision(t, cache, schedulerAccountMetaRevisionKey)
	invalid := time.Date(10000, time.January, 1, 0, 0, 0, 0, time.UTC)
	require.NoError(t, cache.UpdateLastUsed(ctx, map[int64]time.Time{52: invalid}))
	require.NotEqual(t, revisionBefore, memoRevision(t, cache, schedulerAccountMetaRevisionKey))
	memoRequireMiss(t, cache, bucket)

	// 正常的 last_used 写入不推进任何修订号。
	globalAfterDelete := memoRevision(t, cache, schedulerAccountMetaRevisionKey)
	platformAfterDelete := memoRevision(t, cache, memoAnthropicRevisionKey)
	require.NoError(t, cache.UpdateLastUsed(ctx, map[int64]time.Time{51: time.Now()}))
	require.Equal(t, globalAfterDelete, memoRevision(t, cache, schedulerAccountMetaRevisionKey))
	require.Equal(t, platformAfterDelete, memoRevision(t, cache, memoAnthropicRevisionKey))
}

func TestSchedulerCacheSnapshotMemoMembershipChangeByAccountIDs(t *testing.T) {
	ctx := context.Background()
	cache, _, _ := newSchedulerCacheMemoUnit(t, defaultSchedulerSnapshotWriteChunkSize)
	bucket := memoTestBucket(108)
	token, err := cache.CaptureBucketWriteToken(ctx, bucket)
	require.NoError(t, err)
	ids, err := cache.SetSnapshotAndReturnAccountIDs(ctx, bucket, token, []service.Account{memoTestAccount(61, "a"), memoTestAccount(62, "b")})
	require.NoError(t, err)
	require.Equal(t, []int64{61, 62}, ids)
	require.Equal(t, []int64{61, 62}, memoIDs(memoRead(t, cache, bucket)))

	globalBefore := memoRevision(t, cache, schedulerAccountMetaRevisionKey)
	platformBefore := memoRevision(t, cache, memoAnthropicRevisionKey)
	activeBefore := cache.rdb.Get(ctx, schedulerBucketKey(schedulerActivePrefix, bucket)).Val()
	token, err = cache.CaptureBucketWriteToken(ctx, bucket)
	require.NoError(t, err)
	require.NoError(t, cache.SetSnapshotByAccountIDs(ctx, bucket, token, []int64{62}))

	require.Equal(t, globalBefore, memoRevision(t, cache, schedulerAccountMetaRevisionKey), "membership-only publishing does not touch metadata")
	require.Equal(t, platformBefore, memoRevision(t, cache, memoAnthropicRevisionKey), "membership-only publishing does not touch metadata")
	require.NotEqual(t, activeBefore, cache.rdb.Get(ctx, schedulerBucketKey(schedulerActivePrefix, bucket)).Val())
	require.Equal(t, []int64{62}, memoIDs(memoRead(t, cache, bucket)))
}

func TestSchedulerCacheSnapshotMemoRetireReopenAndRepublish(t *testing.T) {
	ctx := context.Background()
	cache, _, _ := newSchedulerCacheMemoUnit(t, defaultSchedulerSnapshotWriteChunkSize)
	bucket := memoTestBucket(109)
	memoPublish(t, cache, bucket, memoTestAccount(71, "before"))
	memoRead(t, cache, bucket)

	require.NoError(t, cache.RetireBucket(ctx, bucket))
	memoRequireMiss(t, cache, bucket)
	require.Zero(t, memoEntryCount(cache))

	token, err := cache.ReopenBucket(ctx, bucket)
	require.NoError(t, err)
	memoRequireMiss(t, cache, bucket)

	require.NoError(t, cache.SetSnapshot(ctx, bucket, token, []service.Account{memoTestAccount(72, "after")}))
	got := memoRead(t, cache, bucket)
	require.Equal(t, []int64{72}, memoIDs(got))
	require.Equal(t, "after", got[0].Name)
}

func TestSchedulerCacheSnapshotMemoEmptyAndNilMetadataAreNotCached(t *testing.T) {
	ctx := context.Background()
	cache, _, _ := newSchedulerCacheMemoUnit(t, defaultSchedulerSnapshotWriteChunkSize)

	t.Run("empty zset", func(t *testing.T) {
		bucket := memoTestBucket(110)
		_, err := cache.writeAccountIDs(ctx, []service.Account{memoTestAccount(81, "a")})
		require.NoError(t, err)
		memoPublish(t, cache, bucket)
		memoRequireMiss(t, cache, bucket)

		active := cache.rdb.Get(ctx, schedulerBucketKey(schedulerActivePrefix, bucket)).Val()
		require.NotEmpty(t, active)
		require.NoError(t, cache.rdb.ZAdd(ctx, schedulerSnapshotKey(bucket, active), redis.Z{Score: 0, Member: "81"}).Err())
		require.Equal(t, []int64{81}, memoIDs(memoRead(t, cache, bucket)))
	})

	t.Run("nil metadata", func(t *testing.T) {
		bucket := memoTestBucket(111)
		memoPublish(t, cache, bucket, memoTestAccount(91, "a"), memoTestAccount(92, "b"))
		metaKey := schedulerAccountMetaKey("92")
		payload, err := cache.rdb.Get(ctx, metaKey).Bytes()
		require.NoError(t, err)
		require.NoError(t, cache.rdb.Del(ctx, metaKey).Err())
		memoRequireMiss(t, cache, bucket)

		require.NoError(t, cache.rdb.Set(ctx, metaKey, payload, 0).Err())
		require.Equal(t, []int64{91, 92}, memoIDs(memoRead(t, cache, bucket)))
	})
}

func TestSchedulerCacheSnapshotMemoDecodeFailureIsNotCached(t *testing.T) {
	ctx := context.Background()
	cache, _, _ := newSchedulerCacheMemoUnit(t, defaultSchedulerSnapshotWriteChunkSize)
	bucket := memoTestBucket(112)
	memoPublish(t, cache, bucket, memoTestAccount(101, "a"))
	metaKey := schedulerAccountMetaKey("101")
	payload, err := cache.rdb.Get(ctx, metaKey).Bytes()
	require.NoError(t, err)
	require.NoError(t, cache.rdb.Set(ctx, metaKey, "{not-json", 0).Err())

	accounts, hit, err := cache.GetSnapshot(ctx, bucket)
	require.Error(t, err)
	require.False(t, hit)
	require.Nil(t, accounts)
	require.Zero(t, memoEntryCount(cache))

	require.NoError(t, cache.rdb.Set(ctx, metaKey, payload, 0).Err())
	require.Equal(t, "a", memoRead(t, cache, bucket)[0].Name)
}

func TestSchedulerCacheSnapshotMemoRedisErrorIsReturned(t *testing.T) {
	cache, mr, _ := newSchedulerCacheMemoUnit(t, defaultSchedulerSnapshotWriteChunkSize)
	bucket := memoTestBucket(113)
	memoPublish(t, cache, bucket, memoTestAccount(111, "a"))
	memoRead(t, cache, bucket)

	mr.SetError("injected failure")
	accounts, hit, err := cache.GetSnapshot(context.Background(), bucket)
	require.Error(t, err)
	require.False(t, hit)
	require.Nil(t, accounts)

	mr.SetError("")
	require.Equal(t, []int64{111}, memoIDs(memoRead(t, cache, bucket)))
}

func TestSchedulerCacheSnapshotMemoConcurrentReadersAndWriters(t *testing.T) {
	ctx := context.Background()
	cache, _, _ := newSchedulerCacheMemoUnit(t, defaultSchedulerSnapshotWriteChunkSize)
	bucket := memoTestBucket(114)
	accounts := []service.Account{memoTestAccount(121, "a"), memoTestAccount(122, "b"), memoTestAccount(123, "c")}
	memoPublish(t, cache, bucket, accounts...)

	var wg sync.WaitGroup
	for reader := 0; reader < 8; reader++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < 40; i++ {
				got, hit, err := cache.GetSnapshot(ctx, bucket)
				if err != nil || !hit {
					t.Errorf("GetSnapshot: hit=%v err=%v", hit, err)
					return
				}
				for _, account := range got {
					if account.GetModelMapping()["claude-sonnet"] == "" {
						t.Errorf("account %d lost its model mapping", account.ID)
						return
					}
					_ = account.IsSchedulableForModel("claude-sonnet")
					_ = account.GetHeaderOverrides()
					account.Schedulable = !account.Schedulable
					account.LastUsedAt = nil
				}
			}
		}()
	}
	for writer := 0; writer < 2; writer++ {
		wg.Add(1)
		go func(writer int) {
			defer wg.Done()
			for i := 0; i < 20; i++ {
				account := memoTestAccount(int64(121+writer), fmt.Sprintf("w%d-%d", writer, i))
				account.Extra = map[string]any{"session_window_utilization": float64(i) / 100}
				if err := cache.SetAccount(ctx, &account); err != nil {
					t.Errorf("SetAccount: %v", err)
					return
				}
				if err := cache.UpdateLastUsed(ctx, map[int64]time.Time{account.ID: time.Now()}); err != nil {
					t.Errorf("UpdateLastUsed: %v", err)
					return
				}
			}
		}(writer)
	}
	wg.Wait()

	got := memoRead(t, cache, bucket)
	require.Equal(t, "w0-19", got[0].Name)
	require.Equal(t, "w1-19", got[1].Name)
	require.Equal(t, "c", got[2].Name)
}

func TestSchedulerCacheSnapshotMemoWriteAccountIDsAdvancesRevision(t *testing.T) {
	ctx := context.Background()
	geminiRevisionKey := schedulerAccountMetaRevisionKeyForPlatform(service.PlatformGemini)

	t.Run("revisions share the last chunk", func(t *testing.T) {
		cache, _, recorder := newSchedulerCacheMemoUnit(t, 2)
		accounts := []service.Account{
			memoTestAccount(131, "a"),
			memoTestAccountOn(service.PlatformGemini, 132, "b"),
			memoTestAccount(133, "c"),
		}
		recorder.reset()
		ids, err := cache.writeAccountIDs(ctx, accounts)
		require.NoError(t, err)
		require.Equal(t, []int64{131, 132, 133}, ids)

		batches := recorder.pipelineBatches()
		require.Len(t, batches, 2, "one execution per chunk")
		require.Equal(t, []string{
			"set " + schedulerAccountKey("133"),
			"set " + schedulerAccountMetaKey("133"),
			"set " + memoAnthropicRevisionKey,
			"set " + geminiRevisionKey,
		}, batches[1])
		require.NotContains(t, strings.Join(batches[0], "\n"), schedulerAccountMetaRevisionKey)
		require.Equal(t, 2, recorder.countPrefix("set "+schedulerAccountMetaRevisionKey), "no revision write outside the last chunk")
		require.Empty(t, memoRevision(t, cache, schedulerAccountMetaRevisionKey))
		first := memoRevision(t, cache, memoAnthropicRevisionKey)
		require.Len(t, first, schedulerAccountMetaRevisionBytes*2)
		require.Len(t, memoRevision(t, cache, geminiRevisionKey), schedulerAccountMetaRevisionBytes*2)

		_, err = cache.writeAccountIDs(ctx, accounts[:1])
		require.NoError(t, err)
		require.NotEqual(t, first, memoRevision(t, cache, memoAnthropicRevisionKey))
	})

	t.Run("account count is a multiple of the chunk size", func(t *testing.T) {
		cache, _, recorder := newSchedulerCacheMemoUnit(t, 2)
		accounts := []service.Account{
			memoTestAccount(141, "a"), memoTestAccount(142, "b"),
			memoTestAccount(143, "c"), memoTestAccount(144, "d"),
		}
		recorder.reset()
		_, err := cache.writeAccountIDs(ctx, accounts)
		require.NoError(t, err)

		batches := recorder.pipelineBatches()
		require.Len(t, batches, 2, "no extra execution for the revision")
		require.Equal(t, []string{
			"set " + schedulerAccountKey("143"),
			"set " + schedulerAccountMetaKey("143"),
			"set " + schedulerAccountKey("144"),
			"set " + schedulerAccountMetaKey("144"),
			"set " + memoAnthropicRevisionKey,
		}, batches[1])
		require.Len(t, recorder.recorded(), 9, "every command travels inside the two pipelines")
	})

	t.Run("failure in a later chunk advances the global revision", func(t *testing.T) {
		cache, _, recorder := newSchedulerCacheMemoUnit(t, 1)
		accounts := []service.Account{memoTestAccount(151, "a"), memoTestAccount(152, "b"), memoTestAccount(153, "c")}
		bucket := memoTestBucket(115)
		memoPublish(t, cache, bucket, accounts...)
		memoRead(t, cache, bucket)
		globalBefore := memoRevision(t, cache, schedulerAccountMetaRevisionKey)
		platformBefore := memoRevision(t, cache, memoAnthropicRevisionKey)

		renamed := []service.Account{memoTestAccount(151, "a2"), memoTestAccount(152, "b2"), memoTestAccount(153, "c2")}
		recorder.reset()
		recorder.setFailPipelineAt(2)
		_, err := cache.writeAccountIDs(ctx, renamed)
		require.Error(t, err)
		recorder.setFailPipelineAt(0)
		require.Eventually(t, func() bool {
			return memoRevisionValue(cache, schedulerAccountMetaRevisionKey) != globalBefore
		}, 2*time.Second, 5*time.Millisecond, "the global revision is advanced in the background")
		require.Equal(t, platformBefore, memoRevision(t, cache, memoAnthropicRevisionKey))

		recorder.reset()
		got := memoRead(t, cache, bucket)
		require.Equal(t, 1, recorder.countPrefix("zrange "), "the cached snapshot must be reloaded")
		require.Equal(t, []string{"a2", "b", "c"}, []string{got[0].Name, got[1].Name, got[2].Name})
	})

	t.Run("failure in the last chunk advances the global revision", func(t *testing.T) {
		cache, _, recorder := newSchedulerCacheMemoUnit(t, 2)
		recorder.reset()
		recorder.setFailPipelineAt(1)
		_, err := cache.writeAccountIDs(ctx, []service.Account{memoTestAccount(161, "a")})
		require.Error(t, err)
		recorder.setFailPipelineAt(0)
		require.Eventually(t, func() bool {
			return memoRevisionValue(cache, schedulerAccountMetaRevisionKey) != ""
		}, 2*time.Second, 5*time.Millisecond, "the global revision is advanced in the background")
		require.Empty(t, memoRevision(t, cache, memoAnthropicRevisionKey))
	})

	t.Run("nothing encodable", func(t *testing.T) {
		cache, _, recorder := newSchedulerCacheMemoUnit(t, 1)
		invalid := time.Date(10000, time.January, 1, 0, 0, 0, 0, time.UTC)
		recorder.reset()
		ids, err := cache.writeAccountIDs(ctx, []service.Account{{ID: 171, Platform: service.PlatformAnthropic, ExpiresAt: &invalid}})
		require.NoError(t, err)
		require.Empty(t, ids)
		require.Empty(t, recorder.recorded())
		require.Empty(t, memoRevision(t, cache, schedulerAccountMetaRevisionKey))
		require.Empty(t, memoRevision(t, cache, memoAnthropicRevisionKey))
	})

	t.Run("empty platform uses the global revision", func(t *testing.T) {
		cache, _, _ := newSchedulerCacheMemoUnit(t, 2)
		_, err := cache.writeAccountIDs(ctx, []service.Account{{ID: 181, Name: "no-platform"}})
		require.NoError(t, err)
		require.NotEmpty(t, memoRevision(t, cache, schedulerAccountMetaRevisionKey))
	})
}

// memoReadExpectHit 读取一次并报告是否重新加载（发出过 ZRANGE）。
func memoReadExpectHit(t *testing.T, cache *schedulerCache, recorder *schedulerCacheCommandRecorder, bucket service.SchedulerBucket) ([]*service.Account, bool) {
	t.Helper()
	recorder.reset()
	accounts := memoRead(t, cache, bucket)
	return accounts, recorder.countPrefix("zrange ") > 0
}

func TestSchedulerCacheSnapshotMemoPlatformRevisionScopesInvalidation(t *testing.T) {
	ctx := context.Background()
	cache, _, recorder := newSchedulerCacheMemoUnit(t, defaultSchedulerSnapshotWriteChunkSize)
	geminiSingle := service.SchedulerBucket{GroupID: 201, Platform: service.PlatformGemini, Mode: service.SchedulerModeSingle}
	gemini := memoTestAccountOn(service.PlatformGemini, 211, "gemini")
	memoPublish(t, cache, geminiSingle, gemini)
	_, reloaded := memoReadExpectHit(t, cache, recorder, geminiSingle)
	require.True(t, reloaded)

	anthropic := memoTestAccount(212, "anthropic")
	require.NoError(t, cache.SetAccount(ctx, &anthropic))
	recorder.reset()
	memoRead(t, cache, geminiSingle)
	require.Equal(t, []string{
		"mget " + schedulerBucketKey(schedulerReadyPrefix, geminiSingle),
		"mget " + schedulerLastUsedKey("211"),
	}, recorder.recorded(), "an Anthropic account write must not invalidate a Gemini bucket")

	gemini.Name = "gemini-renamed"
	require.NoError(t, cache.SetAccount(ctx, &gemini))
	got, reloaded := memoReadExpectHit(t, cache, recorder, geminiSingle)
	require.True(t, reloaded)
	require.Equal(t, "gemini-renamed", got[0].Name)
}

func TestSchedulerCacheSnapshotMemoPlatformChangeInvalidatesOldPlatformBuckets(t *testing.T) {
	ctx := context.Background()
	cache, _, recorder := newSchedulerCacheMemoUnit(t, defaultSchedulerSnapshotWriteChunkSize)
	geminiSingle := service.SchedulerBucket{GroupID: 202, Platform: service.PlatformGemini, Mode: service.SchedulerModeSingle}
	moved := memoTestAccountOn(service.PlatformGemini, 221, "moved")
	memoPublish(t, cache, geminiSingle, moved)
	globalBefore := memoRevision(t, cache, schedulerAccountMetaRevisionKey)

	moved.Name = "same-platform"
	require.NoError(t, cache.SetAccount(ctx, &moved))
	require.Equal(t, globalBefore, memoRevision(t, cache, schedulerAccountMetaRevisionKey), "a same-platform rewrite only advances its platform revision")
	memoRead(t, cache, geminiSingle)

	moved.Platform = service.PlatformAnthropic
	moved.Name = "now-anthropic"
	require.NoError(t, cache.SetAccount(ctx, &moved))
	require.NotEqual(t, globalBefore, memoRevision(t, cache, schedulerAccountMetaRevisionKey), "a platform change advances the global revision")

	got, reloaded := memoReadExpectHit(t, cache, recorder, geminiSingle)
	require.True(t, reloaded, "the old platform bucket must drop the cached account")
	require.Equal(t, service.PlatformAnthropic, got[0].Platform)
	require.Equal(t, "now-anthropic", got[0].Name)
}

func TestSchedulerCacheSnapshotMemoNewAccountOnlyAdvancesPlatformRevision(t *testing.T) {
	cache, _, _ := newSchedulerCacheMemoUnit(t, defaultSchedulerSnapshotWriteChunkSize)
	fresh := memoTestAccountOn(service.PlatformGemini, 231, "fresh")
	require.NoError(t, cache.SetAccount(context.Background(), &fresh))
	require.Empty(t, memoRevision(t, cache, schedulerAccountMetaRevisionKey))
	require.NotEmpty(t, memoRevision(t, cache, schedulerAccountMetaRevisionKeyForPlatform(service.PlatformGemini)))
}

func TestSchedulerCacheSnapshotMemoAntigravityRevisionInvalidatesMixedBuckets(t *testing.T) {
	ctx := context.Background()
	cache, _, recorder := newSchedulerCacheMemoUnit(t, defaultSchedulerSnapshotWriteChunkSize)
	antigravity := memoTestAccountOn(service.PlatformAntigravity, 221, "ag")
	antigravity.Extra = map[string]any{"mixed_scheduling": true}
	gemini := memoTestAccountOn(service.PlatformGemini, 222, "gemini")
	anthropic := memoTestAccount(223, "anthropic")

	geminiMixed := service.SchedulerBucket{GroupID: 202, Platform: service.PlatformGemini, Mode: service.SchedulerModeMixed}
	anthropicMixed := service.SchedulerBucket{GroupID: 202, Platform: service.PlatformAnthropic, Mode: service.SchedulerModeMixed}
	geminiSingle := service.SchedulerBucket{GroupID: 202, Platform: service.PlatformGemini, Mode: service.SchedulerModeSingle}
	geminiForced := service.SchedulerBucket{GroupID: 202, Platform: service.PlatformGemini, Mode: service.SchedulerModeForced}
	memoPublish(t, cache, geminiMixed, gemini, antigravity)
	memoPublish(t, cache, anthropicMixed, anthropic, antigravity)
	memoPublish(t, cache, geminiSingle, gemini)
	memoPublish(t, cache, geminiForced, gemini)
	for _, bucket := range []service.SchedulerBucket{geminiMixed, anthropicMixed, geminiSingle, geminiForced} {
		memoRead(t, cache, bucket)
	}

	antigravity.Name = "ag-renamed"
	require.NoError(t, cache.SetAccount(ctx, &antigravity))

	got, reloaded := memoReadExpectHit(t, cache, recorder, geminiMixed)
	require.True(t, reloaded)
	require.Equal(t, "ag-renamed", got[1].Name)
	got, reloaded = memoReadExpectHit(t, cache, recorder, anthropicMixed)
	require.True(t, reloaded)
	require.Equal(t, "ag-renamed", got[1].Name)
	_, reloaded = memoReadExpectHit(t, cache, recorder, geminiSingle)
	require.False(t, reloaded, "single buckets never contain Antigravity accounts")
	_, reloaded = memoReadExpectHit(t, cache, recorder, geminiForced)
	require.False(t, reloaded, "forced buckets never contain Antigravity accounts")
}

func TestSchedulerCacheSnapshotMemoGlobalRevisionInvalidatesEveryPlatform(t *testing.T) {
	ctx := context.Background()
	buckets := []service.SchedulerBucket{
		{GroupID: 203, Platform: service.PlatformGemini, Mode: service.SchedulerModeSingle},
		{GroupID: 203, Platform: service.PlatformAnthropic, Mode: service.SchedulerModeSingle},
		{GroupID: 203, Platform: service.PlatformOpenAI, Mode: service.SchedulerModeSingle},
	}
	setup := func(t *testing.T) (*schedulerCache, *schedulerCacheCommandRecorder) {
		cache, _, recorder := newSchedulerCacheMemoUnit(t, defaultSchedulerSnapshotWriteChunkSize)
		for i, bucket := range buckets {
			memoPublish(t, cache, bucket, memoTestAccountOn(bucket.Platform, int64(231+i), bucket.Platform))
			memoRead(t, cache, bucket)
		}
		// 不属于任何上述 bucket 的账号。
		unrelated := memoTestAccountOn(service.PlatformGrok, 239, "grok")
		require.NoError(t, cache.SetAccount(ctx, &unrelated))
		for _, bucket := range buckets {
			_, reloaded := memoReadExpectHit(t, cache, recorder, bucket)
			require.False(t, reloaded)
		}
		return cache, recorder
	}

	t.Run("DeleteAccount", func(t *testing.T) {
		cache, recorder := setup(t)
		require.NoError(t, cache.DeleteAccount(ctx, 239))
		for _, bucket := range buckets {
			_, reloaded := memoReadExpectHit(t, cache, recorder, bucket)
			require.True(t, reloaded, bucket.String())
		}
	})

	t.Run("UpdateLastUsed unencodable deletion", func(t *testing.T) {
		cache, recorder := setup(t)
		invalid := time.Date(10000, time.January, 1, 0, 0, 0, 0, time.UTC)
		require.NoError(t, cache.UpdateLastUsed(ctx, map[int64]time.Time{239: invalid}))
		for _, bucket := range buckets {
			_, reloaded := memoReadExpectHit(t, cache, recorder, bucket)
			require.True(t, reloaded, bucket.String())
		}
	})
}

func TestSchedulerCacheSnapshotMemoFlushAllAndRepublishSameVersion(t *testing.T) {
	ctx := context.Background()
	cache, mr, _ := newSchedulerCacheMemoUnit(t, defaultSchedulerSnapshotWriteChunkSize)
	bucket := memoTestBucket(116)
	memoPublish(t, cache, bucket, memoTestAccount(141, "before"))
	require.Equal(t, "before", memoRead(t, cache, bucket)[0].Name)
	activeBefore := cache.rdb.Get(ctx, schedulerBucketKey(schedulerActivePrefix, bucket)).Val()

	mr.FlushAll()
	memoPublish(t, cache, bucket, memoTestAccount(141, "after"))
	require.Equal(t, activeBefore, cache.rdb.Get(ctx, schedulerBucketKey(schedulerActivePrefix, bucket)).Val())
	require.Equal(t, "after", memoRead(t, cache, bucket)[0].Name)
}

type memoTestClock struct {
	mu  sync.Mutex
	now time.Time
}

func (c *memoTestClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *memoTestClock) Advance(d time.Duration) {
	c.mu.Lock()
	c.now = c.now.Add(d)
	c.mu.Unlock()
}

func TestSchedulerCacheSnapshotMemoMaxAgeBoundsUnversionedWrites(t *testing.T) {
	ctx := context.Background()
	cache, _, _ := newSchedulerCacheMemoUnit(t, defaultSchedulerSnapshotWriteChunkSize)
	clock := &memoTestClock{now: time.Date(2026, 10, 20, 8, 0, 0, 0, time.UTC)}
	cache.now = clock.Now
	bucket := memoTestBucket(117)
	memoPublish(t, cache, bucket, memoTestAccount(151, "old"))
	require.Equal(t, "old", memoRead(t, cache, bucket)[0].Name)

	// 模拟不推进修订号的旧实例：直接改写精简记录。
	_, metaPayload, err := marshalSchedulerCacheAccount(memoTestAccount(151, "new"))
	require.NoError(t, err)
	globalBefore := memoRevision(t, cache, schedulerAccountMetaRevisionKey)
	platformBefore := memoRevision(t, cache, memoAnthropicRevisionKey)
	require.NoError(t, cache.rdb.Set(ctx, schedulerAccountMetaKey("151"), metaPayload, 0).Err())
	require.Equal(t, globalBefore, memoRevision(t, cache, schedulerAccountMetaRevisionKey))
	require.Equal(t, platformBefore, memoRevision(t, cache, memoAnthropicRevisionKey))

	clock.Advance(schedulerSnapshotCacheMaxAge - time.Millisecond)
	require.Equal(t, "old", memoRead(t, cache, bucket)[0].Name)

	clock.Advance(time.Millisecond)
	require.Equal(t, "new", memoRead(t, cache, bucket)[0].Name)
}

func TestSchedulerCacheSnapshotMemoEvictsLongIdleEntries(t *testing.T) {
	cache, _, _ := newSchedulerCacheMemoUnit(t, defaultSchedulerSnapshotWriteChunkSize)
	clock := &memoTestClock{now: time.Date(2026, 10, 20, 8, 0, 0, 0, time.UTC)}
	cache.now = clock.Now
	idle := memoTestBucket(118)
	recent := memoTestBucket(119)
	active := memoTestBucket(120)
	memoPublish(t, cache, idle, memoTestAccount(161, "idle"))
	memoPublish(t, cache, recent, memoTestAccount(162, "recent"))
	memoPublish(t, cache, active, memoTestAccount(163, "active"))

	memoRead(t, cache, idle)
	clock.Advance(schedulerSnapshotCacheEvictAge / 2)
	memoRead(t, cache, recent)
	clock.Advance(schedulerSnapshotCacheEvictAge/2 + time.Second)
	memoRead(t, cache, active)

	cache.snapshotMu.RLock()
	defer cache.snapshotMu.RUnlock()
	require.NotContains(t, cache.snapshotEntries, idle.String())
	require.Contains(t, cache.snapshotEntries, recent.String())
	require.Contains(t, cache.snapshotEntries, active.String())
}

func TestSchedulerCacheSnapshotMemoKeepsNewerEntryOnLateStore(t *testing.T) {
	cache, _, _ := newSchedulerCacheMemoUnit(t, defaultSchedulerSnapshotWriteChunkSize)
	base := time.Date(2026, 10, 20, 8, 0, 0, 0, time.UTC)
	newer := &schedulerSnapshotEntry{active: "2", revisions: []string{"r2"}, loadedAt: base.Add(time.Second)}
	older := &schedulerSnapshotEntry{active: "1", revisions: []string{"r1"}, loadedAt: base}
	cache.storeSnapshotEntry("b", newer)
	cache.storeSnapshotEntry("b", older)
	require.Same(t, newer, cache.lookupSnapshotEntry("b"))
}

// 母本在发布前预解析了模型映射：副本命中时不计算签名。此处故意原地改写共享的原始映射，
// 只用来观察副本走的是免签名路径（生产代码不会原地修改该 map）。
func TestSchedulerCacheSnapshotMemoPreparesSharedModelMapping(t *testing.T) {
	cache, _, _ := newSchedulerCacheMemoUnit(t, defaultSchedulerSnapshotWriteChunkSize)
	bucket := memoTestBucket(121)
	memoPublish(t, cache, bucket, memoTestAccount(171, "a"))
	memoRead(t, cache, bucket)

	hit := memoRead(t, cache, bucket)[0]
	require.Equal(t, "sonnet-a", hit.GetMappedModel("claude-sonnet"))

	replaced := memoRead(t, cache, bucket)[0]
	replaced.Credentials = map[string]any{"model_mapping": map[string]any{"claude-sonnet": "sonnet-replaced"}}
	require.Equal(t, "sonnet-replaced", replaced.GetMappedModel("claude-sonnet"))

	probe := memoRead(t, cache, bucket)[0]
	rawMapping, ok := probe.Credentials["model_mapping"].(map[string]any)
	require.True(t, ok)
	rawMapping["claude-sonnet"] = "sonnet-mutated-in-place"
	require.Equal(t, "sonnet-a", probe.GetMappedModel("claude-sonnet"))
}

func TestSchedulerCacheSnapshotMemoRevisionValuesAreUnique(t *testing.T) {
	seen := make(map[string]struct{}, 64)
	for i := 0; i < 64; i++ {
		revision := newSchedulerAccountMetaRevision()
		require.Len(t, revision, schedulerAccountMetaRevisionBytes*2)
		_, err := strconv.ParseUint(revision[:8], 16, 32)
		require.NoError(t, err)
		_, dup := seen[revision]
		require.False(t, dup)
		seen[revision] = struct{}{}
	}
}

func TestSchedulerCacheSnapshotMemoPlatformChangeInEarlierChunk(t *testing.T) {
	ctx := context.Background()
	cache, _, _ := newSchedulerCacheMemoUnit(t, 1)
	require.NoError(t, cache.SetAccount(ctx, ptrAccount(memoTestAccountOn(service.PlatformGemini, 241, "moved"))))
	require.NoError(t, cache.SetAccount(ctx, ptrAccount(memoTestAccount(242, "stays"))))
	globalBefore := memoRevision(t, cache, schedulerAccountMetaRevisionKey)

	_, err := cache.writeAccountIDs(ctx, []service.Account{
		memoTestAccount(241, "moved"),
		memoTestAccount(242, "stays"),
	})
	require.NoError(t, err)
	require.NotEqual(t, globalBefore, memoRevision(t, cache, schedulerAccountMetaRevisionKey))
}

func TestSchedulerCacheSnapshotMemoUnparsableOldMetadataAdvancesGlobalRevision(t *testing.T) {
	ctx := context.Background()
	cache, mr, _ := newSchedulerCacheMemoUnit(t, defaultSchedulerSnapshotWriteChunkSize)
	require.NoError(t, mr.Set(schedulerAccountMetaKey("251"), "not-json"))

	require.NoError(t, cache.SetAccount(ctx, ptrAccount(memoTestAccount(251, "replaced"))))
	require.NotEmpty(t, memoRevision(t, cache, schedulerAccountMetaRevisionKey))
}

func TestSchedulerCacheSnapshotMemoNilBeforeRealPipelineError(t *testing.T) {
	ctx := context.Background()
	cache, mr, _ := newSchedulerCacheMemoUnit(t, defaultSchedulerSnapshotWriteChunkSize)
	mr.HSet(schedulerAccountMetaKey("262"), "field", "value")

	_, err := cache.writeAccountIDs(ctx, []service.Account{
		memoTestAccount(261, "fresh"),
		memoTestAccount(262, "wrong-type"),
	})
	require.Error(t, err, "a WRONGTYPE reply after a nil SET ... GET reply must fail the write")
	require.Eventually(t, func() bool {
		return memoRevisionValue(cache, schedulerAccountMetaRevisionKey) != ""
	}, 2*time.Second, 5*time.Millisecond, "the global revision is advanced in the background")
}

func TestSchedulerCacheSnapshotMemoCoalescesBackgroundGlobalRevisionWrites(t *testing.T) {
	cache, _, recorder := newSchedulerCacheMemoUnit(t, defaultSchedulerSnapshotWriteChunkSize)
	recorder.reset()
	for range 50 {
		cache.scheduleGlobalAccountMetaRevision(context.Background())
	}
	require.Eventually(t, func() bool {
		return !cache.globalRevisionWriting.Load() && memoRevisionValue(cache, schedulerAccountMetaRevisionKey) != ""
	}, 2*time.Second, 5*time.Millisecond)
	writes := 0
	for _, command := range recorder.recorded() {
		if command == "set "+schedulerAccountMetaRevisionKey {
			writes++
		}
	}
	require.GreaterOrEqual(t, writes, 1)
	require.LessOrEqual(t, writes, 3, "concurrent schedules collapse into at most a couple of background writes")
}

func ptrAccount(account service.Account) *service.Account {
	return &account
}
