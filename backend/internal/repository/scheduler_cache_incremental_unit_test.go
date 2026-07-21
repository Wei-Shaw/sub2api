//go:build unit

package repository

import (
	"context"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestSchedulerCacheRemoveAccountFromBucketFencesOlderWriter(t *testing.T) {
	ctx := context.Background()
	cache := newSchedulerCacheUnit(t)
	bucket := service.SchedulerBucket{GroupID: 81, Platform: service.PlatformOpenAI, Mode: service.SchedulerModeSingle}
	account := service.Account{ID: 8101, Platform: service.PlatformOpenAI, Status: service.StatusActive, Schedulable: true}

	oldToken, err := cache.CaptureBucketWriteToken(ctx, bucket)
	require.NoError(t, err)
	require.NoError(t, cache.SetSnapshot(ctx, bucket, oldToken, []service.Account{account}))

	removed, err := cache.RemoveAccountFromBucket(ctx, bucket, oldToken, account.ID)
	require.NoError(t, err)
	require.True(t, removed)

	err = cache.SetSnapshot(ctx, bucket, oldToken, []service.Account{account})
	require.ErrorIs(t, err, service.ErrSchedulerBucketWriteFenced)

	freshToken, err := cache.CaptureBucketWriteToken(ctx, bucket)
	require.NoError(t, err)
	require.Greater(t, freshToken.Epoch, oldToken.Epoch)
	require.NoError(t, cache.SetSnapshot(ctx, bucket, freshToken, []service.Account{account}))
}

func TestSchedulerCacheRemoveAccountFromUnreadyBucketStillFencesWriter(t *testing.T) {
	ctx := context.Background()
	cache := newSchedulerCacheUnit(t)
	bucket := service.SchedulerBucket{GroupID: 82, Platform: service.PlatformGrok, Mode: service.SchedulerModeForced}

	oldToken, err := cache.CaptureBucketWriteToken(ctx, bucket)
	require.NoError(t, err)
	removed, err := cache.RemoveAccountFromBucket(ctx, bucket, oldToken, 8201)
	require.NoError(t, err)
	require.False(t, removed)

	err = cache.SetSnapshot(ctx, bucket, oldToken, []service.Account{{ID: 8201, Platform: service.PlatformGrok}})
	require.ErrorIs(t, err, service.ErrSchedulerBucketWriteFenced)
}

func TestSchedulerCacheOutboxWatermarkNeverMovesBackward(t *testing.T) {
	ctx := context.Background()
	cache := newSchedulerCacheUnit(t)

	require.NoError(t, cache.SetOutboxWatermark(ctx, 20))
	require.NoError(t, cache.SetOutboxWatermark(ctx, 12))
	watermark, err := cache.GetOutboxWatermark(ctx)
	require.NoError(t, err)
	require.EqualValues(t, 20, watermark)

	require.NoError(t, cache.SetOutboxWatermark(ctx, 21))
	watermark, err = cache.GetOutboxWatermark(ctx)
	require.NoError(t, err)
	require.EqualValues(t, 21, watermark)
}

func TestSchedulerCacheUpdateLastUsedIsMonotonic(t *testing.T) {
	ctx := context.Background()
	cache := newSchedulerCacheUnit(t)
	initial := time.Unix(200, 0).UTC()
	account := service.Account{ID: 8301, Platform: service.PlatformOpenAI, LastUsedAt: &initial}
	require.NoError(t, cache.SetAccount(ctx, &account))
	revisionBefore, err := cache.rdb.Get(ctx, schedulerMetadataRevisionKey).Int64()
	require.NoError(t, err)

	require.NoError(t, cache.UpdateLastUsed(ctx, map[int64]time.Time{account.ID: time.Unix(100, 0).UTC()}))
	unchanged, err := cache.GetAccount(ctx, account.ID)
	require.NoError(t, err)
	require.NotNil(t, unchanged)
	require.Equal(t, initial, *unchanged.LastUsedAt)
	revisionAfterOldUpdate, err := cache.rdb.Get(ctx, schedulerMetadataRevisionKey).Int64()
	require.NoError(t, err)
	require.Equal(t, revisionBefore, revisionAfterOldUpdate)

	newer := time.Unix(300, 0).UTC()
	require.NoError(t, cache.UpdateLastUsed(ctx, map[int64]time.Time{account.ID: newer}))
	updated, err := cache.GetAccount(ctx, account.ID)
	require.NoError(t, err)
	require.NotNil(t, updated)
	require.Equal(t, newer, *updated.LastUsedAt)
	revisionAfterNewUpdate, err := cache.rdb.Get(ctx, schedulerMetadataRevisionKey).Int64()
	require.NoError(t, err)
	require.Greater(t, revisionAfterNewUpdate, revisionAfterOldUpdate)
}
