//go:build integration

package repository

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	redisclient "github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
)

type blockSchedulerLastUsedEvalHook struct {
	blocked     chan struct{}
	release     chan struct{}
	blockOnce   sync.Once
	releaseOnce sync.Once
}

func newBlockSchedulerLastUsedEvalHook() *blockSchedulerLastUsedEvalHook {
	return &blockSchedulerLastUsedEvalHook{
		blocked: make(chan struct{}),
		release: make(chan struct{}),
	}
}

func (h *blockSchedulerLastUsedEvalHook) DialHook(next redisclient.DialHook) redisclient.DialHook {
	return next
}

func (h *blockSchedulerLastUsedEvalHook) ProcessHook(next redisclient.ProcessHook) redisclient.ProcessHook {
	return next
}

func (h *blockSchedulerLastUsedEvalHook) ProcessPipelineHook(next redisclient.ProcessPipelineHook) redisclient.ProcessPipelineHook {
	return func(ctx context.Context, cmds []redisclient.Cmder) error {
		for _, cmd := range cmds {
			if !strings.EqualFold(cmd.Name(), "eval") && !strings.EqualFold(cmd.Name(), "evalsha") {
				continue
			}
			h.blockOnce.Do(func() {
				close(h.blocked)
				<-h.release
			})
			break
		}
		return next(ctx, cmds)
	}
}

func (h *blockSchedulerLastUsedEvalHook) waitUntilBlocked(t *testing.T) {
	t.Helper()
	select {
	case <-h.blocked:
	case <-time.After(5 * time.Second):
		t.Fatal("UpdateLastUsed did not reach the blocked Redis eval")
	}
}

func (h *blockSchedulerLastUsedEvalHook) unblock() {
	h.releaseOnce.Do(func() { close(h.release) })
}

func TestSchedulerAdmissionCapacityExpiresThroughRedisWithoutOutbox(t *testing.T) {
	ctx := context.Background()
	cache := NewSchedulerCache(testRedis(t))
	capacityCache, ok := cache.(service.AdmissionCapacityCache)
	require.True(t, ok)

	validUntil := time.Now().Add(200 * time.Millisecond).UTC()
	require.NoError(t, capacityCache.SetAdmissionCapacity(ctx, service.PlatformOpenAI, service.AdmissionCapacitySnapshot{
		TotalConcurrency:   3,
		AccountConcurrency: map[int64]int{7: 3},
		BuiltAt:            time.Now().UTC(),
		ValidUntil:         &validUntil,
	}))
	reader := service.NewSchedulerSnapshotService(cache, nil, nil, nil, nil)

	beforeExpiry, err := reader.AdmissionCapacity(ctx, service.PlatformOpenAI)
	require.NoError(t, err)
	require.Equal(t, 3, beforeExpiry.TotalConcurrency)

	require.Eventually(t, func() bool {
		afterExpiry, admissionErr := reader.AdmissionCapacity(ctx, service.PlatformOpenAI)
		return errors.Is(admissionErr, service.ErrSchedulerCacheNotReady) && afterExpiry.TotalConcurrency == 0
	}, 2*time.Second, 10*time.Millisecond)
}

func TestSchedulerAdmissionCapacityRejectsStaleBuiltAtThroughRedis(t *testing.T) {
	ctx := context.Background()
	cache := NewSchedulerCache(testRedis(t))
	capacityCache, ok := cache.(service.AdmissionCapacityCache)
	require.True(t, ok)

	require.NoError(t, capacityCache.SetAdmissionCapacity(ctx, service.PlatformOpenAI, service.AdmissionCapacitySnapshot{
		TotalConcurrency:   3,
		AccountConcurrency: map[int64]int{7: 3},
		BuiltAt:            time.Now().Add(-11 * time.Minute).UTC(),
	}))
	reader := service.NewSchedulerSnapshotService(cache, nil, nil, nil, nil)

	snapshot, err := reader.AdmissionCapacity(ctx, service.PlatformOpenAI)

	require.ErrorIs(t, err, service.ErrSchedulerCacheNotReady)
	require.Zero(t, snapshot.TotalConcurrency)
}

func TestSchedulerCacheSnapshotUsesSlimMetadataButKeepsFullAccount(t *testing.T) {
	ctx := context.Background()
	rdb := testRedis(t)
	cache := NewSchedulerCache(rdb)

	bucket := service.SchedulerBucket{GroupID: 2, Platform: service.PlatformGemini, Mode: service.SchedulerModeSingle}
	now := time.Now().UTC().Truncate(time.Second)
	limitReset := now.Add(10 * time.Minute)
	overloadUntil := now.Add(2 * time.Minute)
	tempUnschedUntil := now.Add(3 * time.Minute)
	windowEnd := now.Add(5 * time.Hour)

	account := service.Account{
		ID:          101,
		Name:        "gemini-heavy",
		Platform:    service.PlatformGemini,
		Type:        service.AccountTypeOAuth,
		Status:      service.StatusActive,
		Schedulable: true,
		Concurrency: 3,
		Priority:    7,
		LastUsedAt:  &now,
		Credentials: map[string]any{
			"api_key":       "gemini-api-key",
			"access_token":  "secret-access-token",
			"project_id":    "proj-1",
			"oauth_type":    "ai_studio",
			"model_mapping": map[string]any{"gemini-2.5-pro": "gemini-2.5-pro"},
			"huge_blob":     strings.Repeat("x", 4096),
		},
		Extra: map[string]any{
			"mixed_scheduling":             true,
			"window_cost_limit":            12.5,
			"window_cost_sticky_reserve":   8.0,
			"max_sessions":                 4,
			"session_idle_timeout_minutes": 11,
			"unused_large_field":           strings.Repeat("y", 4096),
		},
		RateLimitResetAt:       &limitReset,
		OverloadUntil:          &overloadUntil,
		TempUnschedulableUntil: &tempUnschedUntil,
		SessionWindowStart:     &now,
		SessionWindowEnd:       &windowEnd,
		SessionWindowStatus:    "active",
		GroupIDs:               []int64{bucket.GroupID},
		AccountGroups: []service.AccountGroup{
			{
				AccountID: 101,
				GroupID:   bucket.GroupID,
				Priority:  5,
				Group:     &service.Group{ID: bucket.GroupID, Name: "gemini-group"},
			},
		},
	}

	require.NoError(t, cache.SetSnapshot(ctx, bucket, []service.Account{account}))

	snapshot, hit, err := cache.GetSnapshot(ctx, bucket)
	require.NoError(t, err)
	require.True(t, hit)
	require.Len(t, snapshot, 1)

	got := snapshot[0]
	require.NotNil(t, got)
	require.Equal(t, "gemini-api-key", got.GetCredential("api_key"))
	require.Equal(t, "proj-1", got.GetCredential("project_id"))
	require.Equal(t, "ai_studio", got.GetCredential("oauth_type"))
	require.NotEmpty(t, got.GetModelMapping())
	require.Empty(t, got.GetCredential("access_token"))
	require.Empty(t, got.GetCredential("huge_blob"))
	require.Equal(t, true, got.Extra["mixed_scheduling"])
	require.Equal(t, 12.5, got.GetWindowCostLimit())
	require.Equal(t, 8.0, got.GetWindowCostStickyReserve())
	require.Equal(t, 4, got.GetMaxSessions())
	require.Equal(t, 11, got.GetSessionIdleTimeoutMinutes())
	require.Nil(t, got.Extra["unused_large_field"])
	require.Equal(t, []int64{bucket.GroupID}, got.GroupIDs)
	require.Len(t, got.AccountGroups, 1)
	require.Equal(t, account.ID, got.AccountGroups[0].AccountID)
	require.Equal(t, bucket.GroupID, got.AccountGroups[0].GroupID)
	require.Nil(t, got.AccountGroups[0].Group)

	full, err := cache.GetAccount(ctx, account.ID)
	require.NoError(t, err)
	require.NotNil(t, full)
	require.Equal(t, "secret-access-token", full.GetCredential("access_token"))
	require.Equal(t, strings.Repeat("x", 4096), full.GetCredential("huge_blob"))
	require.Len(t, full.AccountGroups, 1)
	require.NotNil(t, full.AccountGroups[0].Group)
}

func TestSchedulerCacheLastUsedOverlayKeepsConcurrentAccountFields(t *testing.T) {
	ctx := context.Background()
	clients := testRedisClients(t, 2)
	reader := NewSchedulerCache(clients[0])
	writer := NewSchedulerCache(clients[1])
	bucket := service.SchedulerBucket{GroupID: 17, Platform: service.PlatformOpenAI, Mode: service.SchedulerModeSingle}
	initialLastUsed := time.Now().Add(-2 * time.Minute).UTC().Truncate(time.Millisecond)
	newLastUsed := initialLastUsed.Add(time.Minute)
	account := service.Account{
		ID:          701,
		Name:        "last-used-overlay",
		Platform:    service.PlatformOpenAI,
		Type:        service.AccountTypeAPIKey,
		Status:      service.StatusActive,
		Schedulable: true,
		Concurrency: 1,
		LastUsedAt:  &initialLastUsed,
		Credentials: map[string]any{"api_key": "sk-old"},
	}
	require.NoError(t, writer.SetSnapshot(ctx, bucket, []service.Account{account}))

	blocker := newBlockSchedulerLastUsedEvalHook()
	clients[0].AddHook(blocker)
	t.Cleanup(blocker.unblock)
	updateDone := make(chan error, 1)
	go func() {
		updateDone <- reader.UpdateLastUsed(ctx, map[int64]time.Time{account.ID: newLastUsed})
	}()
	blocker.waitUntilBlocked(t)

	replacement := account
	replacement.Status = service.StatusDisabled
	replacement.Schedulable = false
	replacement.Concurrency = 9
	replacement.Credentials = map[string]any{"api_key": "sk-new"}
	require.NoError(t, writer.SetAccount(ctx, &replacement))

	blocker.unblock()
	require.NoError(t, <-updateDone)

	full, err := writer.GetAccount(ctx, account.ID)
	require.NoError(t, err)
	require.NotNil(t, full)
	require.Equal(t, service.StatusDisabled, full.Status)
	require.False(t, full.Schedulable)
	require.Equal(t, 9, full.Concurrency)
	require.Equal(t, "sk-new", full.GetCredential("api_key"))
	require.NotNil(t, full.LastUsedAt)
	require.True(t, full.LastUsedAt.Equal(newLastUsed))

	snapshot, hit, err := writer.GetSnapshot(ctx, bucket)
	require.NoError(t, err)
	require.True(t, hit)
	require.Len(t, snapshot, 1)
	require.Equal(t, service.StatusDisabled, snapshot[0].Status)
	require.False(t, snapshot[0].Schedulable)
	require.Equal(t, 9, snapshot[0].Concurrency)
	require.NotNil(t, snapshot[0].LastUsedAt)
	require.True(t, snapshot[0].LastUsedAt.Equal(newLastUsed))

	// A stale full account refresh must not lower the independent last-used overlay.
	require.NoError(t, writer.SetAccount(ctx, &replacement))
	full, err = writer.GetAccount(ctx, account.ID)
	require.NoError(t, err)
	require.NotNil(t, full.LastUsedAt)
	require.True(t, full.LastUsedAt.Equal(newLastUsed))
}

func TestSchedulerCacheLastUsedUpdateDoesNotResurrectDeletedAccount(t *testing.T) {
	ctx := context.Background()
	clients := testRedisClients(t, 2)
	reader := NewSchedulerCache(clients[0])
	writer := NewSchedulerCache(clients[1])
	initialLastUsed := time.Now().Add(-2 * time.Minute).UTC().Truncate(time.Millisecond)
	account := service.Account{
		ID:          702,
		Name:        "last-used-delete",
		Platform:    service.PlatformOpenAI,
		Type:        service.AccountTypeAPIKey,
		Status:      service.StatusActive,
		Schedulable: true,
		Concurrency: 1,
		LastUsedAt:  &initialLastUsed,
		Credentials: map[string]any{"api_key": "sk-delete"},
	}
	require.NoError(t, writer.SetAccount(ctx, &account))

	blocker := newBlockSchedulerLastUsedEvalHook()
	clients[0].AddHook(blocker)
	t.Cleanup(blocker.unblock)
	updateDone := make(chan error, 1)
	go func() {
		updateDone <- reader.UpdateLastUsed(ctx, map[int64]time.Time{
			account.ID: initialLastUsed.Add(time.Minute),
		})
	}()
	blocker.waitUntilBlocked(t)

	require.NoError(t, writer.DeleteAccount(ctx, account.ID))
	blocker.unblock()
	require.NoError(t, <-updateDone)

	full, err := writer.GetAccount(ctx, account.ID)
	require.NoError(t, err)
	require.Nil(t, full)
	id := strconv.FormatInt(account.ID, 10)
	exists, err := clients[1].Exists(
		ctx,
		schedulerAccountKey(id),
		schedulerAccountMetaKey(id),
		schedulerAccountLastUsedKey(id),
	).Result()
	require.NoError(t, err)
	require.Zero(t, exists)
}

func TestSchedulerCacheReadsLegacyLastUsedWithoutOverlayKey(t *testing.T) {
	ctx := context.Background()
	rdb := testRedis(t)
	cache := NewSchedulerCache(rdb)
	bucket := service.SchedulerBucket{GroupID: 18, Platform: service.PlatformOpenAI, Mode: service.SchedulerModeSingle}
	lastUsed := time.Now().Add(-time.Minute).UTC().Truncate(time.Millisecond)
	account := service.Account{
		ID:          703,
		Name:        "legacy-last-used",
		Platform:    service.PlatformOpenAI,
		Type:        service.AccountTypeAPIKey,
		Status:      service.StatusActive,
		Schedulable: true,
		Concurrency: 1,
		LastUsedAt:  &lastUsed,
	}
	require.NoError(t, cache.SetSnapshot(ctx, bucket, []service.Account{account}))
	require.NoError(t, rdb.Del(ctx, schedulerAccountLastUsedKey(strconv.FormatInt(account.ID, 10))).Err())

	full, err := cache.GetAccount(ctx, account.ID)
	require.NoError(t, err)
	require.NotNil(t, full)
	require.NotNil(t, full.LastUsedAt)
	require.True(t, full.LastUsedAt.Equal(lastUsed))

	snapshot, hit, err := cache.GetSnapshot(ctx, bucket)
	require.NoError(t, err)
	require.True(t, hit)
	require.Len(t, snapshot, 1)
	require.NotNil(t, snapshot[0].LastUsedAt)
	require.True(t, snapshot[0].LastUsedAt.Equal(lastUsed))
}
