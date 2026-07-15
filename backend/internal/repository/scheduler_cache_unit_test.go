//go:build unit

package repository

import (
	"context"
	"encoding/hex"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
)

func newSchedulerCacheUnit(t *testing.T) *schedulerCache {
	cache, _ := newSchedulerCacheUnitWithRedis(t)
	return cache
}

func newSchedulerCacheUnitWithRedis(t *testing.T) (*schedulerCache, *miniredis.Miniredis) {
	t.Helper()
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })
	cache, ok := newSchedulerCacheWithChunkSizes(rdb, defaultSchedulerSnapshotMGetChunkSize, defaultSchedulerSnapshotWriteChunkSize).(*schedulerCache)
	require.True(t, ok)
	return cache, mr
}

func TestSchedulerCacheWriteAccountsSkipsUnencodableTimes(t *testing.T) {
	ctx := context.Background()
	cache := newSchedulerCacheUnit(t)
	invalidTime := time.Date(10000, time.January, 1, 0, 0, 0, 0, time.UTC)

	cacheable, err := cache.writeAccounts(ctx, []service.Account{
		{ID: 111, Platform: service.PlatformOpenAI, Type: service.AccountTypeAPIKey},
		{ID: 112, Platform: service.PlatformOpenAI, Type: service.AccountTypeAPIKey, ExpiresAt: &invalidTime},
	})
	require.NoError(t, err)
	require.Len(t, cacheable, 1)
	require.Equal(t, int64(111), cacheable[0].ID)

	cached, err := cache.GetAccount(ctx, 111)
	require.NoError(t, err)
	require.NotNil(t, cached)

	invalid, err := cache.GetAccount(ctx, 112)
	require.NoError(t, err)
	require.Nil(t, invalid)
}

func TestSchedulerCacheSetAccountClearsUnencodablePayload(t *testing.T) {
	ctx := context.Background()
	cache := newSchedulerCacheUnit(t)

	account := service.Account{ID: 113, Platform: service.PlatformOpenAI, Type: service.AccountTypeAPIKey}
	require.NoError(t, cache.SetAccount(ctx, &account))

	invalidTime := time.Date(10000, time.January, 1, 0, 0, 0, 0, time.UTC)
	account.ExpiresAt = &invalidTime
	require.NoError(t, cache.SetAccount(ctx, &account))

	cached, err := cache.GetAccount(ctx, account.ID)
	require.NoError(t, err)
	require.Nil(t, cached)
}

func TestSchedulerCacheUpdateLastUsedClearsUnencodablePayload(t *testing.T) {
	ctx := context.Background()
	cache := newSchedulerCacheUnit(t)
	account := service.Account{ID: 114, Platform: service.PlatformOpenAI, Type: service.AccountTypeAPIKey}
	require.NoError(t, cache.SetAccount(ctx, &account))

	invalidTime := time.Date(10000, time.January, 1, 0, 0, 0, 0, time.UTC)
	require.NoError(t, cache.UpdateLastUsed(ctx, map[int64]time.Time{account.ID: invalidTime}))

	cached, err := cache.GetAccount(ctx, account.ID)
	require.NoError(t, err)
	require.Nil(t, cached)
}

func TestBuildSchedulerMetadataAccount_KeepsOpenAIWSFlags(t *testing.T) {
	account := service.Account{
		ID:       42,
		Platform: service.PlatformOpenAI,
		Type:     service.AccountTypeOAuth,
		Extra: map[string]any{
			"openai_oauth_responses_websockets_v2_enabled": true,
			"openai_oauth_responses_websockets_v2_mode":    service.OpenAIWSIngressModePassthrough,
			"openai_ws_force_http":                         true,
			"openai_responses_mode":                        "force_chat_completions",
			"openai_responses_supported":                   false,
			"mixed_scheduling":                             true,
			"unused_large_field":                           "drop-me",
		},
	}

	got := buildSchedulerMetadataAccount(account)

	require.Equal(t, true, got.Extra["openai_oauth_responses_websockets_v2_enabled"])
	require.Equal(t, service.OpenAIWSIngressModePassthrough, got.Extra["openai_oauth_responses_websockets_v2_mode"])
	require.Equal(t, true, got.Extra["openai_ws_force_http"])
	require.Equal(t, "force_chat_completions", got.Extra["openai_responses_mode"])
	require.Equal(t, false, got.Extra["openai_responses_supported"])
	require.Equal(t, true, got.Extra["mixed_scheduling"])
	require.Nil(t, got.Extra["unused_large_field"])
}

func TestBuildSchedulerMetadataAccount_KeepsSlimGroupMembership(t *testing.T) {
	account := service.Account{
		ID:       42,
		Platform: service.PlatformAnthropic,
		GroupIDs: []int64{7, 9, 7, 0},
		AccountGroups: []service.AccountGroup{
			{
				AccountID: 42,
				GroupID:   7,
				Priority:  2,
				Account:   &service.Account{ID: 42, Name: "drop-from-metadata"},
				Group:     &service.Group{ID: 7, Name: "drop-from-metadata"},
			},
			{
				AccountID: 42,
				GroupID:   11,
				Priority:  3,
				Group:     &service.Group{ID: 11, Name: "drop-from-metadata"},
			},
			{
				AccountID: 42,
				GroupID:   0,
				Priority:  4,
			},
		},
	}

	got := buildSchedulerMetadataAccount(account)

	require.Equal(t, []int64{7, 9, 11}, got.GroupIDs)
	require.Len(t, got.AccountGroups, 2)
	require.Equal(t, int64(42), got.AccountGroups[0].AccountID)
	require.Equal(t, int64(7), got.AccountGroups[0].GroupID)
	require.Equal(t, 2, got.AccountGroups[0].Priority)
	require.Nil(t, got.AccountGroups[0].Account)
	require.Nil(t, got.AccountGroups[0].Group)
	require.Equal(t, int64(11), got.AccountGroups[1].GroupID)
	require.Nil(t, got.Groups)
}

func TestBuildSchedulerMetadataAccount_KeepsQuotaAutoPauseFields(t *testing.T) {
	account := service.Account{
		ID: 88,
		Extra: map[string]any{
			"codex_5h_used_percent":        12.34,
			"codex_7d_used_percent":        56.78,
			"codex_5h_reset_at":            "2026-05-29T10:00:00Z",
			"codex_7d_reset_at":            "2026-06-01T10:00:00Z",
			"codex_5h_reset_after_seconds": 300,
			"codex_7d_reset_after_seconds": 600,
			"codex_usage_updated_at":       "2026-05-29T09:00:00Z",
			"auto_pause_5h_threshold":      0.95,
			"auto_pause_7d_threshold":      0.96,
			"auto_pause_5h_disabled":       true,
			"auto_pause_7d_disabled":       false,
		},
	}

	got := buildSchedulerMetadataAccount(account)

	require.Equal(t, 12.34, got.Extra["codex_5h_used_percent"])
	require.Equal(t, 56.78, got.Extra["codex_7d_used_percent"])
	require.Equal(t, "2026-05-29T10:00:00Z", got.Extra["codex_5h_reset_at"])
	require.Equal(t, "2026-06-01T10:00:00Z", got.Extra["codex_7d_reset_at"])
	require.Equal(t, 300, got.Extra["codex_5h_reset_after_seconds"])
	require.Equal(t, 600, got.Extra["codex_7d_reset_after_seconds"])
	require.Equal(t, "2026-05-29T09:00:00Z", got.Extra["codex_usage_updated_at"])
	require.Equal(t, 0.95, got.Extra["auto_pause_5h_threshold"])
	require.Equal(t, 0.96, got.Extra["auto_pause_7d_threshold"])
	require.Equal(t, true, got.Extra["auto_pause_5h_disabled"])
	require.Equal(t, false, got.Extra["auto_pause_7d_disabled"])
}

func TestBuildSchedulerMetadataAccount_KeepsModelRateLimits(t *testing.T) {
	account := service.Account{
		ID:       90,
		Platform: service.PlatformAntigravity,
		Extra: map[string]any{
			"model_rate_limits": map[string]any{
				"gemini-3-flash": map[string]any{
					"rate_limit_reset_at": "2026-05-30T10:10:00Z",
				},
				"antigravity:gemini": map[string]any{
					"rate_limit_reset_at": "2026-05-30T10:10:00Z",
				},
			},
			"unused_large_field": "drop-me",
		},
	}

	got := buildSchedulerMetadataAccount(account)

	limits, ok := got.Extra["model_rate_limits"].(map[string]any)
	require.True(t, ok)
	require.Contains(t, limits, "gemini-3-flash")
	require.Contains(t, limits, "antigravity:gemini")
	require.Nil(t, got.Extra["unused_large_field"])
}

func TestBuildSchedulerMetadataAccount_KeepsSparkShadowRoutingIdentity(t *testing.T) {
	parentID := int64(100)
	account := service.Account{
		ID:              200,
		Platform:        service.PlatformOpenAI,
		Type:            service.AccountTypeOAuth,
		ParentAccountID: &parentID,
		QuotaDimension:  service.QuotaDimensionSpark,
		Credentials: map[string]any{
			"model_mapping": map[string]any{
				"gpt-5.3-codex-spark": "gpt-5.3-codex-spark",
			},
			"compact_model_mapping": map[string]any{
				"gpt-5.4": "gpt-5.4-openai-compact",
			},
			"access_token": "drop-me",
		},
	}

	got := buildSchedulerMetadataAccount(account)

	require.NotNil(t, got.ParentAccountID)
	require.Equal(t, parentID, *got.ParentAccountID)
	require.Equal(t, service.QuotaDimensionSpark, got.QuotaDimension)
	require.Equal(t, map[string]any{"gpt-5.3-codex-spark": "gpt-5.3-codex-spark"}, got.Credentials["model_mapping"])
	require.Equal(t, map[string]any{"gpt-5.4": "gpt-5.4-openai-compact"}, got.Credentials["compact_model_mapping"])
	require.Nil(t, got.Credentials["access_token"])
}

func TestSchedulerCacheBucketRetirementFencesWritersAndReopen(t *testing.T) {
	ctx := context.Background()
	cache, mr := newSchedulerCacheUnitWithRedis(t)
	bucket := service.SchedulerBucket{GroupID: 41, Platform: service.PlatformOpenAI, Mode: service.SchedulerModeSingle}
	otherBucket := service.SchedulerBucket{GroupID: 42, Platform: service.PlatformOpenAI, Mode: service.SchedulerModeSingle}
	account := service.Account{ID: 4101, Platform: service.PlatformOpenAI, Type: service.AccountTypeAPIKey}

	token, err := cache.CaptureBucketWriteToken(ctx, bucket)
	require.NoError(t, err)
	require.True(t, token.ValidFor(bucket))
	require.NoError(t, cache.SetSnapshot(ctx, bucket, token, []service.Account{account}))

	// A token is bound to the full bucket identity, not just an epoch number.
	err = cache.SetSnapshot(ctx, otherBucket, token, []service.Account{account})
	require.ErrorIs(t, err, service.ErrSchedulerBucketWriteFenced)
	_, err = cache.rdb.Get(ctx, schedulerBucketKey(schedulerVersionPrefix, otherBucket)).Result()
	require.ErrorIs(t, err, redis.Nil)
	otherAccount := service.Account{ID: 4201, Platform: service.PlatformOpenAI, Type: service.AccountTypeAPIKey}
	otherToken, err := cache.CaptureBucketWriteToken(ctx, otherBucket)
	require.NoError(t, err)
	require.NoError(t, cache.SetSnapshot(ctx, otherBucket, otherToken, []service.Account{otherAccount}))
	otherEpoch := otherToken.Epoch

	activeVersion, err := cache.rdb.Get(ctx, schedulerBucketKey(schedulerActivePrefix, bucket)).Result()
	require.NoError(t, err)
	require.NoError(t, cache.RetireBucket(ctx, bucket))
	retiredEpoch, err := cache.rdb.Get(ctx, schedulerBucketKey(schedulerEpochPrefix, bucket)).Int64()
	require.NoError(t, err)
	require.Greater(t, retiredEpoch, token.Epoch)

	// Retirement is idempotent and does not advance the epoch again.
	require.NoError(t, cache.RetireBucket(ctx, bucket))
	retiredEpochAgain, err := cache.rdb.Get(ctx, schedulerBucketKey(schedulerEpochPrefix, bucket)).Int64()
	require.NoError(t, err)
	require.Equal(t, retiredEpoch, retiredEpochAgain)

	// New readers miss because ready/active were removed atomically. A reader that
	// captured activeVersion before retirement may still finish against that version.
	_, hit, err := cache.GetSnapshot(ctx, bucket)
	require.NoError(t, err)
	require.False(t, hit)
	ids, err := cache.rdb.ZRange(ctx, schedulerSnapshotKey(bucket, activeVersion), 0, -1).Result()
	require.NoError(t, err)
	require.Equal(t, []string{"4101"}, ids)
	ttl, err := cache.rdb.TTL(ctx, schedulerSnapshotKey(bucket, activeVersion)).Result()
	require.NoError(t, err)
	require.Positive(t, ttl)
	require.LessOrEqual(t, ttl, time.Duration(snapshotGraceTTLSeconds)*time.Second)

	buckets, err := cache.ListBuckets(ctx)
	require.NoError(t, err)
	require.NotContains(t, buckets, bucket)
	require.Contains(t, buckets, otherBucket)
	otherSnapshot, otherHit, err := cache.GetSnapshot(ctx, otherBucket)
	require.NoError(t, err)
	require.True(t, otherHit)
	require.Len(t, otherSnapshot, 1)
	require.Equal(t, otherAccount.ID, otherSnapshot[0].ID)
	otherEpochAfter, err := cache.rdb.Get(ctx, schedulerBucketKey(schedulerEpochPrefix, otherBucket)).Int64()
	require.NoError(t, err)
	require.Equal(t, otherEpoch, otherEpochAfter)

	_, err = cache.CaptureBucketWriteToken(ctx, bucket)
	require.ErrorIs(t, err, service.ErrSchedulerBucketRetired)
	versionBeforeRejectedWrite, err := cache.rdb.Get(ctx, schedulerBucketKey(schedulerVersionPrefix, bucket)).Int64()
	require.NoError(t, err)
	err = cache.SetSnapshot(ctx, bucket, token, []service.Account{account})
	require.ErrorIs(t, err, service.ErrSchedulerBucketRetired)
	versionAfterRejectedWrite, err := cache.rdb.Get(ctx, schedulerBucketKey(schedulerVersionPrefix, bucket)).Int64()
	require.NoError(t, err)
	require.Equal(t, versionBeforeRejectedWrite, versionAfterRejectedWrite, "fenced writers must not allocate a new version")
	retired, err := cache.rdb.Exists(ctx, schedulerBucketKey(schedulerRetiredPrefix, bucket)).Result()
	require.NoError(t, err)
	require.EqualValues(t, 1, retired, "ordinary writers must never clear the tombstone")
	mr.FastForward(time.Duration(snapshotGraceTTLSeconds+1) * time.Second)
	exists, err := cache.rdb.Exists(ctx, schedulerSnapshotKey(bucket, activeVersion)).Result()
	require.NoError(t, err)
	require.Zero(t, exists, "retired active snapshot must expire after the in-flight grace period")

	newToken, err := cache.ReopenBucket(ctx, bucket)
	require.NoError(t, err)
	require.True(t, newToken.ValidFor(bucket))
	require.Equal(t, retiredEpoch, newToken.Epoch)
	reopenedAgain, err := cache.ReopenBucket(ctx, bucket)
	require.NoError(t, err)
	require.Equal(t, newToken, reopenedAgain, "reopen must be idempotent within one retirement generation")
	err = cache.SetSnapshot(ctx, bucket, token, []service.Account{account})
	require.ErrorIs(t, err, service.ErrSchedulerBucketWriteFenced)
	require.NoError(t, cache.SetSnapshot(ctx, bucket, newToken, []service.Account{account}))
	reopenedWhileOpen, err := cache.ReopenBucket(ctx, bucket)
	require.NoError(t, err)
	require.Equal(t, newToken, reopenedWhileOpen)

	snapshot, hit, err := cache.GetSnapshot(ctx, bucket)
	require.NoError(t, err)
	require.True(t, hit)
	require.Len(t, snapshot, 1)
	require.Equal(t, account.ID, snapshot[0].ID)
}

func TestSchedulerCacheActivationIsFencedAfterRetire(t *testing.T) {
	ctx := context.Background()
	cache := newSchedulerCacheUnit(t)
	bucket := service.SchedulerBucket{GroupID: 51, Platform: service.PlatformAnthropic, Mode: service.SchedulerModeMixed}
	account := service.Account{ID: 5101, Platform: service.PlatformAnthropic, Type: service.AccountTypeAPIKey}

	token, err := cache.CaptureBucketWriteToken(ctx, bucket)
	require.NoError(t, err)
	version, err := cache.allocateSnapshotVersion(ctx, bucket, token)
	require.NoError(t, err)
	require.NoError(t, cache.writeSnapshotVersion(ctx, bucket, version, []service.Account{account}))

	// Deterministic race C: retirement and authoritative reopen both happen after
	// INCR/write but before the old writer activates.
	require.NoError(t, cache.RetireBucket(ctx, bucket))
	_, err = cache.ReopenBucket(ctx, bucket)
	require.NoError(t, err)
	err = cache.activateSnapshotVersion(ctx, bucket, token, version)
	require.ErrorIs(t, err, service.ErrSchedulerBucketWriteFenced)

	exists, err := cache.rdb.Exists(ctx, schedulerSnapshotKey(bucket, version)).Result()
	require.NoError(t, err)
	require.Zero(t, exists, "fenced activation must delete its unpublished snapshot")
	exists, err = cache.rdb.Exists(
		ctx,
		schedulerBucketKey(schedulerReadyPrefix, bucket),
		schedulerBucketKey(schedulerActivePrefix, bucket),
	).Result()
	require.NoError(t, err)
	require.Zero(t, exists)
	buckets, err := cache.ListBuckets(ctx)
	require.NoError(t, err)
	require.NotContains(t, buckets, bucket)
}

func TestSchedulerCacheConcurrentReopenReturnsSameToken(t *testing.T) {
	ctx := context.Background()
	cache := newSchedulerCacheUnit(t)
	bucket := service.SchedulerBucket{GroupID: 53, Platform: service.PlatformOpenAI, Mode: service.SchedulerModeForced}
	account := service.Account{ID: 5301, Platform: service.PlatformOpenAI, Type: service.AccountTypeAPIKey}

	oldToken, err := cache.CaptureBucketWriteToken(ctx, bucket)
	require.NoError(t, err)
	require.NoError(t, cache.RetireBucket(ctx, bucket))

	type reopenResult struct {
		token service.SchedulerBucketWriteToken
		err   error
	}
	start := make(chan struct{})
	results := make(chan reopenResult, 2)
	for range 2 {
		go func() {
			<-start
			token, err := cache.ReopenBucket(ctx, bucket)
			results <- reopenResult{token: token, err: err}
		}()
	}
	close(start)
	first := <-results
	second := <-results
	require.NoError(t, first.err)
	require.NoError(t, second.err)
	require.Equal(t, first.token, second.token)
	require.Greater(t, first.token.Epoch, oldToken.Epoch)

	require.ErrorIs(t, cache.SetSnapshot(ctx, bucket, oldToken, []service.Account{account}), service.ErrSchedulerBucketWriteFenced)
	require.NoError(t, cache.SetSnapshot(ctx, bucket, first.token, []service.Account{account}))
}

func TestSchedulerCacheReopenExpiresPreviousActiveSnapshot(t *testing.T) {
	ctx := context.Background()
	cache, mr := newSchedulerCacheUnitWithRedis(t)
	bucket := service.SchedulerBucket{GroupID: 52, Platform: service.PlatformGemini, Mode: service.SchedulerModeForced}
	account := service.Account{ID: 5201, Platform: service.PlatformGemini, Type: service.AccountTypeAPIKey}

	oldToken, err := cache.CaptureBucketWriteToken(ctx, bucket)
	require.NoError(t, err)
	require.NoError(t, cache.SetSnapshot(ctx, bucket, oldToken, []service.Account{account}))
	oldVersion, err := cache.rdb.Get(ctx, schedulerBucketKey(schedulerActivePrefix, bucket)).Result()
	require.NoError(t, err)
	retiredEpoch := oldToken.Epoch + 1
	require.NoError(t, cache.rdb.Set(ctx, schedulerBucketKey(schedulerEpochPrefix, bucket), retiredEpoch, 0).Err())
	require.NoError(t, cache.rdb.Set(ctx, schedulerBucketKey(schedulerRetiredPrefix, bucket), retiredEpoch, 0).Err())

	newToken, err := cache.ReopenBucket(ctx, bucket)
	require.NoError(t, err)
	require.Equal(t, retiredEpoch, newToken.Epoch)
	_, hit, err := cache.GetSnapshot(ctx, bucket)
	require.NoError(t, err)
	require.False(t, hit)
	ttl, err := cache.rdb.TTL(ctx, schedulerSnapshotKey(bucket, oldVersion)).Result()
	require.NoError(t, err)
	require.Positive(t, ttl)
	require.LessOrEqual(t, ttl, time.Duration(snapshotGraceTTLSeconds)*time.Second)

	require.ErrorIs(t, cache.SetSnapshot(ctx, bucket, oldToken, []service.Account{account}), service.ErrSchedulerBucketWriteFenced)
	mr.FastForward(time.Duration(snapshotGraceTTLSeconds+1) * time.Second)
	exists, err := cache.rdb.Exists(ctx, schedulerSnapshotKey(bucket, oldVersion)).Result()
	require.NoError(t, err)
	require.Zero(t, exists)
	require.NoError(t, cache.SetSnapshot(ctx, bucket, newToken, []service.Account{account}))
}

func TestSchedulerCacheGroupLifecycleLeaseConcurrentAcquireSingleOwner(t *testing.T) {
	ctx := context.Background()
	cache := newSchedulerCacheUnit(t)
	const groupID int64 = 71

	type result struct {
		lease    service.SchedulerGroupLifecycleLease
		acquired bool
		err      error
	}
	start := make(chan struct{})
	results := make(chan result, 32)
	for range 32 {
		go func() {
			<-start
			lease, acquired, err := cache.TryAcquireGroupLifecycleLease(ctx, groupID, time.Minute)
			results <- result{lease: lease, acquired: acquired, err: err}
		}()
	}
	close(start)

	var owner service.SchedulerGroupLifecycleLease
	acquiredCount := 0
	for range 32 {
		got := <-results
		require.NoError(t, got.err)
		if got.acquired {
			acquiredCount++
			owner = got.lease
			require.True(t, got.lease.ValidFor(groupID))
		} else {
			require.Equal(t, service.SchedulerGroupLifecycleLease{}, got.lease)
		}
	}
	require.Equal(t, 1, acquiredCount)
	require.Len(t, owner.OwnerToken, schedulerGroupLifecycleOwnerTokenBytes*2)
	require.Equal(t, strings.ToLower(owner.OwnerToken), owner.OwnerToken)
	decodedOwner, err := hex.DecodeString(owner.OwnerToken)
	require.NoError(t, err)
	require.Len(t, decodedOwner, schedulerGroupLifecycleOwnerTokenBytes)

	require.NoError(t, cache.ReleaseGroupLifecycleLease(ctx, owner))
	next, acquired, err := cache.TryAcquireGroupLifecycleLease(ctx, groupID, time.Minute)
	require.NoError(t, err)
	require.True(t, acquired)
	require.True(t, next.ValidFor(groupID))
	require.NotEqual(t, owner.OwnerToken, next.OwnerToken)
	require.NoError(t, cache.ReleaseGroupLifecycleLease(ctx, next))
}

func TestSchedulerCacheGroupLifecycleLeaseStaleReleaseCannotDeleteSuccessor(t *testing.T) {
	ctx := context.Background()
	cache, mr := newSchedulerCacheUnitWithRedis(t)
	const groupID int64 = 72
	const ttl = time.Minute

	first, acquired, err := cache.TryAcquireGroupLifecycleLease(ctx, groupID, ttl)
	require.NoError(t, err)
	require.True(t, acquired)

	mr.FastForward(ttl + time.Second)
	second, acquired, err := cache.TryAcquireGroupLifecycleLease(ctx, groupID, ttl)
	require.NoError(t, err)
	require.True(t, acquired)
	require.NotEqual(t, first.OwnerToken, second.OwnerToken)

	require.ErrorIs(t, cache.ReleaseGroupLifecycleLease(ctx, first), service.ErrSchedulerGroupLifecycleLeaseLost)
	owner, err := cache.rdb.Get(ctx, schedulerGroupLifecycleLockKey(groupID)).Result()
	require.NoError(t, err)
	require.Equal(t, second.OwnerToken, owner)

	_, acquired, err = cache.TryAcquireGroupLifecycleLease(ctx, groupID, ttl)
	require.NoError(t, err)
	require.False(t, acquired)
	require.NoError(t, cache.ReleaseGroupLifecycleLease(ctx, second))
	require.ErrorIs(t, cache.ReleaseGroupLifecycleLease(ctx, second), service.ErrSchedulerGroupLifecycleLeaseLost)
}

func TestSchedulerCacheGroupLifecycleLeaseExpiredReleaseIsLost(t *testing.T) {
	ctx := context.Background()
	cache, mr := newSchedulerCacheUnitWithRedis(t)
	const groupID int64 = 73
	const ttl = time.Minute

	lease, acquired, err := cache.TryAcquireGroupLifecycleLease(ctx, groupID, ttl)
	require.NoError(t, err)
	require.True(t, acquired)
	mr.FastForward(ttl + time.Second)

	require.ErrorIs(t, cache.ReleaseGroupLifecycleLease(ctx, lease), service.ErrSchedulerGroupLifecycleLeaseLost)
}

func TestSchedulerCacheGroupLifecycleLeaseWrongOwnerAndCrossGroupAreLost(t *testing.T) {
	ctx := context.Background()
	cache := newSchedulerCacheUnit(t)
	const firstGroupID int64 = 74
	const secondGroupID int64 = 75

	first, acquired, err := cache.TryAcquireGroupLifecycleLease(ctx, firstGroupID, time.Minute)
	require.NoError(t, err)
	require.True(t, acquired)
	second, acquired, err := cache.TryAcquireGroupLifecycleLease(ctx, secondGroupID, time.Minute)
	require.NoError(t, err)
	require.True(t, acquired, "different groups must acquire independently")
	require.NotEqual(t, first.OwnerToken, second.OwnerToken)

	wrongOwner := first
	wrongOwner.OwnerToken = strings.Repeat("0", schedulerGroupLifecycleOwnerTokenBytes*2)
	if wrongOwner.OwnerToken == first.OwnerToken {
		wrongOwner.OwnerToken = strings.Repeat("1", schedulerGroupLifecycleOwnerTokenBytes*2)
	}
	require.ErrorIs(t, cache.ReleaseGroupLifecycleLease(ctx, wrongOwner), service.ErrSchedulerGroupLifecycleLeaseLost)

	crossGroup := first
	crossGroup.GroupID = secondGroupID
	require.ErrorIs(t, cache.ReleaseGroupLifecycleLease(ctx, crossGroup), service.ErrSchedulerGroupLifecycleLeaseLost)

	firstOwner, err := cache.rdb.Get(ctx, schedulerGroupLifecycleLockKey(firstGroupID)).Result()
	require.NoError(t, err)
	require.Equal(t, first.OwnerToken, firstOwner)
	secondOwner, err := cache.rdb.Get(ctx, schedulerGroupLifecycleLockKey(secondGroupID)).Result()
	require.NoError(t, err)
	require.Equal(t, second.OwnerToken, secondOwner)

	require.NoError(t, cache.ReleaseGroupLifecycleLease(ctx, first))
	require.NoError(t, cache.ReleaseGroupLifecycleLease(ctx, second))
}

func TestSchedulerCacheGroupLifecycleLeaseCanceledContextFailsClosed(t *testing.T) {
	cache := newSchedulerCacheUnit(t)
	canceledCtx, cancel := context.WithCancel(context.Background())
	cancel()

	lease, acquired, err := cache.TryAcquireGroupLifecycleLease(canceledCtx, 76, time.Minute)
	require.ErrorIs(t, err, context.Canceled)
	require.False(t, acquired)
	require.Equal(t, service.SchedulerGroupLifecycleLease{}, lease)

	ctx := context.Background()
	lease, acquired, err = cache.TryAcquireGroupLifecycleLease(ctx, 76, time.Minute)
	require.NoError(t, err)
	require.True(t, acquired)
	require.ErrorIs(t, cache.ReleaseGroupLifecycleLease(canceledCtx, lease), context.Canceled)
	owner, err := cache.rdb.Get(ctx, schedulerGroupLifecycleLockKey(lease.GroupID)).Result()
	require.NoError(t, err)
	require.Equal(t, lease.OwnerToken, owner)
	require.NoError(t, cache.ReleaseGroupLifecycleLease(ctx, lease))
}

func TestSchedulerCacheGroupLifecycleLeaseRejectsInvalidInput(t *testing.T) {
	ctx := context.Background()
	cache := newSchedulerCacheUnit(t)

	lease, acquired, err := cache.TryAcquireGroupLifecycleLease(ctx, 0, time.Minute)
	require.ErrorIs(t, err, service.ErrSchedulerGroupLifecycleLeaseInvalid)
	require.False(t, acquired)
	require.Equal(t, service.SchedulerGroupLifecycleLease{}, lease)

	lease, acquired, err = cache.TryAcquireGroupLifecycleLease(ctx, 73, 0)
	require.ErrorIs(t, err, service.ErrSchedulerGroupLifecycleLeaseInvalid)
	require.False(t, acquired)
	require.Equal(t, service.SchedulerGroupLifecycleLease{}, lease)

	canceledCtx, cancel := context.WithCancel(ctx)
	cancel()
	require.ErrorIs(t, cache.ReleaseGroupLifecycleLease(canceledCtx, service.SchedulerGroupLifecycleLease{}), service.ErrSchedulerGroupLifecycleLeaseInvalid)
	keys, err := cache.rdb.DBSize(ctx).Result()
	require.NoError(t, err)
	require.Zero(t, keys)
}
