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

func newSchedulerV2TestCache(t *testing.T) *schedulerCache {
	t.Helper()
	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { require.NoError(t, client.Close()) })
	return newSchedulerCacheWithChunkSizes(client, 8, 8).(*schedulerCache)
}

func schedulerV2TestAccount(id int64, platform, accountType string, priority int, lastUsedAt *time.Time) service.Account {
	return service.Account{
		ID:          id,
		Platform:    platform,
		Type:        accountType,
		Priority:    priority,
		LastUsedAt:  lastUsedAt,
		Status:      service.StatusActive,
		Schedulable: true,
		Concurrency: 1,
	}
}

func TestSchedulerCacheV2_EngineStateAndCandidatePaging(t *testing.T) {
	ctx := context.Background()
	cache := newSchedulerV2TestCache(t)
	state := service.SchedulerEngineState{Engine: service.SchedulerEngineV2, Status: service.SchedulerEngineStatusActive}
	require.NoError(t, cache.SetSchedulerEngineState(ctx, state))
	require.NoError(t, cache.SetSchedulerV2Limits(ctx, 32, 128))
	gotState, err := cache.GetSchedulerEngineState(ctx)
	require.NoError(t, err)
	state.CandidateLimit = 32
	state.ScanLimit = 128
	require.Equal(t, state, gotState)

	now := time.Now().UTC().Truncate(time.Second)
	older := now.Add(-time.Hour)
	bucket := service.SchedulerBucket{GroupID: 7, Platform: service.PlatformOpenAI, Mode: service.SchedulerModeSingle}
	accounts := []service.Account{
		schedulerV2TestAccount(1, service.PlatformOpenAI, service.AccountTypeOAuth, 2, nil),
		schedulerV2TestAccount(2, service.PlatformOpenAI, service.AccountTypeAPIKey, 1, &now),
		schedulerV2TestAccount(3, service.PlatformOpenAI, service.AccountTypeOAuth, 1, &older),
		schedulerV2TestAccount(4, service.PlatformOpenAI, service.AccountTypeOAuth, 1, nil),
	}
	require.NoError(t, cache.SetCandidateIndex(ctx, bucket, accounts))

	page, hit, err := cache.GetCandidatePage(ctx, bucket, 0, 8)
	require.NoError(t, err)
	require.True(t, hit)
	require.True(t, page.Done)
	require.Len(t, page.Accounts, 4)
	require.Equal(t, []int64{4, 3, 2, 1}, candidateAccountIDs(page.Accounts))
}

func TestSchedulerCacheV2_EngineStateCompareAndSetAndIndexVersion(t *testing.T) {
	ctx := context.Background()
	cache := newSchedulerV2TestCache(t)
	building := service.SchedulerEngineState{Engine: service.SchedulerEngineV2, Status: service.SchedulerEngineStatusBuilding}
	require.NoError(t, cache.SetSchedulerEngineState(ctx, building))

	active := service.SchedulerEngineState{Engine: service.SchedulerEngineV2, Status: service.SchedulerEngineStatusActive}
	changed, err := cache.CompareAndSetSchedulerEngineState(ctx, service.SchedulerEngineLegacy, service.SchedulerEngineStatusDisabled, active)
	require.NoError(t, err)
	require.False(t, changed)
	changed, err = cache.CompareAndSetSchedulerEngineState(ctx, service.SchedulerEngineV2, service.SchedulerEngineStatusBuilding, active)
	require.NoError(t, err)
	require.True(t, changed)

	bucket := service.SchedulerBucket{GroupID: 8, Platform: service.PlatformGemini, Mode: service.SchedulerModeMixed}
	require.NoError(t, cache.SetCandidateIndex(ctx, bucket, []service.Account{
		schedulerV2TestAccount(8, service.PlatformGemini, service.AccountTypeOAuth, 1, nil),
	}))
	require.NoError(t, cache.rdb.Set(ctx, schedulerBucketKey(schedulerCandidateReady, bucket), "obsolete", 0).Err())
	_, hit, err := cache.GetCandidatePage(ctx, bucket, 0, 8)
	require.NoError(t, err)
	require.False(t, hit, "a score-version mismatch must force a candidate rebuild")
}

func TestSchedulerCacheV2_ReplaceMovesOnlyOneAccountBetweenBuckets(t *testing.T) {
	ctx := context.Background()
	cache := newSchedulerV2TestCache(t)
	from := service.SchedulerBucket{GroupID: 1, Platform: service.PlatformAnthropic, Mode: service.SchedulerModeMixed}
	to := service.SchedulerBucket{GroupID: 2, Platform: service.PlatformAnthropic, Mode: service.SchedulerModeMixed}
	account := schedulerV2TestAccount(10, service.PlatformAnthropic, service.AccountTypeSetupToken, 1, nil)
	require.NoError(t, cache.SetCandidateIndex(ctx, from, []service.Account{account}))
	require.NoError(t, cache.SetCandidateIndex(ctx, to, nil))

	require.NoError(t, cache.ReplaceAccountCandidates(ctx, &account, []service.SchedulerBucket{to}))

	fromPage, hit, err := cache.GetCandidatePage(ctx, from, 0, 8)
	require.NoError(t, err)
	require.True(t, hit)
	require.Empty(t, fromPage.Accounts)
	toPage, hit, err := cache.GetCandidatePage(ctx, to, 0, 8)
	require.NoError(t, err)
	require.True(t, hit)
	require.Equal(t, []int64{10}, candidateAccountIDs(toPage.Accounts))
}

func TestSchedulerCacheV2_LastUsedUpdatesCandidateOrder(t *testing.T) {
	ctx := context.Background()
	cache := newSchedulerV2TestCache(t)
	bucket := service.SchedulerBucket{GroupID: 5, Platform: service.PlatformGrok, Mode: service.SchedulerModeSingle}
	account1 := schedulerV2TestAccount(1, service.PlatformGrok, service.AccountTypeOAuth, 1, nil)
	account2 := schedulerV2TestAccount(2, service.PlatformGrok, service.AccountTypeAPIKey, 1, nil)
	require.NoError(t, cache.SetCandidateIndex(ctx, bucket, []service.Account{account1, account2}))

	require.NoError(t, cache.UpdateCandidateLastUsed(ctx, map[int64]time.Time{1: time.Now().UTC()}))
	page, hit, err := cache.GetCandidatePage(ctx, bucket, 0, 8)
	require.NoError(t, err)
	require.True(t, hit)
	require.Equal(t, []int64{2, 1}, candidateAccountIDs(page.Accounts))
}

func TestSchedulerCacheV2_InvalidatesStaleLegacySnapshotsOnRollback(t *testing.T) {
	ctx := context.Background()
	cache := newSchedulerV2TestCache(t)
	bucket := service.SchedulerBucket{GroupID: 9, Platform: service.PlatformGemini, Mode: service.SchedulerModeMixed}
	account := schedulerV2TestAccount(90, service.PlatformGemini, service.AccountTypeOAuth, 1, nil)
	require.NoError(t, cache.SetSnapshot(ctx, bucket, []service.Account{account}))
	_, hit, err := cache.GetSnapshot(ctx, bucket)
	require.NoError(t, err)
	require.True(t, hit)

	require.NoError(t, cache.InvalidateLegacySnapshots(ctx))
	_, hit, err = cache.GetSnapshot(ctx, bucket)
	require.NoError(t, err)
	require.False(t, hit)
}

func TestSchedulerCacheV2_DisableRestoreAndDeleteAccountAcrossBuckets(t *testing.T) {
	ctx := context.Background()
	cache := newSchedulerV2TestCache(t)
	first := service.SchedulerBucket{GroupID: 10, Platform: service.PlatformAnthropic, Mode: service.SchedulerModeSingle}
	second := service.SchedulerBucket{GroupID: 11, Platform: service.PlatformAnthropic, Mode: service.SchedulerModeForced}
	account := schedulerV2TestAccount(101, service.PlatformAnthropic, service.AccountTypeOAuth, 1, nil)
	require.NoError(t, cache.SetCandidateIndex(ctx, first, []service.Account{account}))
	require.NoError(t, cache.SetCandidateIndex(ctx, second, []service.Account{account}))

	disabled := account
	disabled.Schedulable = false
	require.NoError(t, cache.ReplaceAccountCandidates(ctx, &disabled, nil))
	for _, bucket := range []service.SchedulerBucket{first, second} {
		page, hit, err := cache.GetCandidatePage(ctx, bucket, 0, 8)
		require.NoError(t, err)
		require.True(t, hit)
		require.Empty(t, page.Accounts)
	}
	stored, err := cache.GetAccount(ctx, account.ID)
	require.NoError(t, err)
	require.NotNil(t, stored)
	require.False(t, stored.Schedulable)

	require.NoError(t, cache.ReplaceAccountCandidates(ctx, &account, []service.SchedulerBucket{first, second}))
	require.NoError(t, cache.DeleteCandidateAccount(ctx, account.ID))
	for _, bucket := range []service.SchedulerBucket{first, second} {
		page, hit, err := cache.GetCandidatePage(ctx, bucket, 0, 8)
		require.NoError(t, err)
		require.True(t, hit)
		require.Empty(t, page.Accounts)
	}
	stored, err = cache.GetAccount(ctx, account.ID)
	require.NoError(t, err)
	require.Nil(t, stored)
}

func TestSchedulerCacheV2_EmptyIndexIsReadyButMissingMetadataIsMiss(t *testing.T) {
	ctx := context.Background()
	cache := newSchedulerV2TestCache(t)
	bucket := service.SchedulerBucket{GroupID: 12, Platform: service.PlatformGrok, Mode: service.SchedulerModeSingle}
	require.NoError(t, cache.SetCandidateIndex(ctx, bucket, nil))
	page, hit, err := cache.GetCandidatePage(ctx, bucket, 0, 8)
	require.NoError(t, err)
	require.True(t, hit)
	require.True(t, page.Done)
	require.Empty(t, page.Accounts)

	account := schedulerV2TestAccount(102, service.PlatformGrok, service.AccountTypeAPIKey, 1, nil)
	require.NoError(t, cache.SetCandidateIndex(ctx, bucket, []service.Account{account}))
	require.NoError(t, cache.rdb.Del(ctx, schedulerAccountMetaKey("102")).Err())
	_, hit, err = cache.GetCandidatePage(ctx, bucket, 0, 8)
	require.NoError(t, err)
	require.False(t, hit, "an incomplete ready index must be rebuilt instead of silently skipping metadata")
}

func candidateAccountIDs(accounts []*service.Account) []int64 {
	ids := make([]int64, 0, len(accounts))
	for _, account := range accounts {
		if account != nil {
			ids = append(ids, account.ID)
		}
	}
	return ids
}
