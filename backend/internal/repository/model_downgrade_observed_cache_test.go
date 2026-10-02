//go:build unit

package repository

import (
	"context"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
)

func newModelDowngradeObservedFixture(t *testing.T) (*miniredis.Miniredis, service.ModelDowngradeObservedCache, context.Context) {
	t.Helper()
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })
	return mr, NewModelDowngradeObservedCache(rdb), context.Background()
}

func observedEntry(accountID int64, sentModel string) service.ModelDowngradeObservedEntry {
	now := time.Date(2026, 9, 19, 10, 0, 0, 0, time.UTC)
	return service.ModelDowngradeObservedEntry{
		AccountID:            accountID,
		AccountName:          "openai-pool",
		Cause:                service.ModelDowngradeObservedCauseDryRun,
		SentModel:            sentModel,
		ResponseModel:        "gpt-5.6-luna",
		TriggerCount:         5,
		TriggerThreshold:     5,
		TriggerWindowMinutes: 30,
		ObservedAt:           now,
		ExpiresAt:            now.Add(24 * time.Hour),
	}
}

func TestModelDowngradeObservedCacheRecordAndList(t *testing.T) {
	_, cache, ctx := newModelDowngradeObservedFixture(t)

	require.NoError(t, cache.RecordObserved(ctx, observedEntry(101, "gpt-6-astra"), time.Hour))
	ratioCapped := observedEntry(102, "gpt-6-sol")
	ratioCapped.Cause = service.ModelDowngradeObservedCauseRatioCap
	ratioCapped.Blocked, ratioCapped.Total, ratioCapped.MaxBlockedRatio = 3, 10, 0.3
	require.NoError(t, cache.RecordObserved(ctx, ratioCapped, time.Hour))

	entries, err := cache.ListObserved(ctx)
	require.NoError(t, err)
	require.Len(t, entries, 2)

	byID := map[int64]service.ModelDowngradeObservedEntry{}
	for _, entry := range entries {
		byID[entry.AccountID] = entry
	}
	require.Equal(t, service.ModelDowngradeObservedCauseDryRun, byID[101].Cause)
	require.Equal(t, "gpt-6-astra", byID[101].SentModel)
	require.Equal(t, int64(5), byID[101].TriggerCount)
	// ExpiresAt 写入时就算好，读取时原样返回，列表接口不必再查 TTL。
	require.Equal(t, observedEntry(101, "gpt-6-astra").ExpiresAt.UTC(), byID[101].ExpiresAt.UTC())

	require.Equal(t, service.ModelDowngradeObservedCauseRatioCap, byID[102].Cause)
	require.Equal(t, int64(3), byID[102].Blocked)
	require.Equal(t, int64(10), byID[102].Total)
	require.InDelta(t, 0.3, byID[102].MaxBlockedRatio, 1e-9)
}

// 同一账号 + 模型再次命中时覆盖旧值，页面上永远只留最新的一条。
// 模型名的大小写和空格不应该拆出第二条记录。
func TestModelDowngradeObservedCacheOverwritesSameAccountAndModel(t *testing.T) {
	_, cache, ctx := newModelDowngradeObservedFixture(t)

	require.NoError(t, cache.RecordObserved(ctx, observedEntry(101, "gpt-6-astra"), time.Hour))
	second := observedEntry(101, " GPT-6-Astra ")
	second.TriggerCount = 9
	require.NoError(t, cache.RecordObserved(ctx, second, time.Hour))

	entries, err := cache.ListObserved(ctx)
	require.NoError(t, err)
	require.Len(t, entries, 1)
	require.Equal(t, int64(9), entries[0].TriggerCount)
}

func TestModelDowngradeObservedCacheDelete(t *testing.T) {
	_, cache, ctx := newModelDowngradeObservedFixture(t)
	require.NoError(t, cache.RecordObserved(ctx, observedEntry(101, "gpt-6-astra"), time.Hour))

	deleted, err := cache.DeleteObserved(ctx, 101, "GPT-6-ASTRA")
	require.NoError(t, err)
	require.True(t, deleted, "模型名大小写不同也必须命中同一条记录")

	// 已经删掉的记录再删一次返回 false，handler 据此回 404。
	deleted, err = cache.DeleteObserved(ctx, 101, "gpt-6-astra")
	require.NoError(t, err)
	require.False(t, deleted)

	entries, err := cache.ListObserved(ctx)
	require.NoError(t, err)
	require.Empty(t, entries)
}

// TTL 到期后记录自动消失，不需要任何清理任务。
func TestModelDowngradeObservedCacheExpires(t *testing.T) {
	mr, cache, ctx := newModelDowngradeObservedFixture(t)
	require.NoError(t, cache.RecordObserved(ctx, observedEntry(101, "gpt-6-astra"), time.Hour))

	mr.FastForward(59 * time.Minute)
	entries, err := cache.ListObserved(ctx)
	require.NoError(t, err)
	require.Len(t, entries, 1)

	mr.FastForward(2 * time.Minute)
	entries, err = cache.ListObserved(ctx)
	require.NoError(t, err)
	require.Empty(t, entries)
}

func TestModelDowngradeObservedCacheRejectsNonPositiveTTL(t *testing.T) {
	_, cache, ctx := newModelDowngradeObservedFixture(t)

	require.Error(t, cache.RecordObserved(ctx, observedEntry(101, "gpt-6-astra"), 0))
}

// 一条脏数据不应该让整张表打不开：解析失败的 key 跳过，其余照常返回。
func TestModelDowngradeObservedCacheSkipsUndecodableValues(t *testing.T) {
	mr, cache, ctx := newModelDowngradeObservedFixture(t)
	require.NoError(t, cache.RecordObserved(ctx, observedEntry(101, "gpt-6-astra"), time.Hour))
	require.NoError(t, mr.Set(modelDowngradeObservedKey(999, "gpt-6-sol"), "not json"))

	entries, err := cache.ListObserved(ctx)
	require.NoError(t, err)
	require.Len(t, entries, 1)
	require.Equal(t, int64(101), entries[0].AccountID)
}

func TestModelDowngradeObservedCacheEmptyListIsNotAnError(t *testing.T) {
	_, cache, ctx := newModelDowngradeObservedFixture(t)

	entries, err := cache.ListObserved(ctx)
	require.NoError(t, err)
	require.Empty(t, entries)
}
