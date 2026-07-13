//go:build integration

package repository

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type blockingSchedulerOutboxRepository struct {
	service.SchedulerOutboxRepository
	listed     chan struct{}
	release    chan struct{}
	blockOnce  sync.Once
	listedOnce sync.Once
}

func (r *blockingSchedulerOutboxRepository) ListAfterAndReleaseDedup(
	ctx context.Context,
	afterID int64,
	limit int,
) ([]service.SchedulerOutboxEvent, error) {
	events, err := r.SchedulerOutboxRepository.ListAfterAndReleaseDedup(ctx, afterID, limit)
	if err != nil || len(events) == 0 {
		return events, err
	}
	r.blockOnce.Do(func() {
		r.listedOnce.Do(func() { close(r.listed) })
		<-r.release
	})
	return events, nil
}

func TestSchedulerSnapshotOutboxReplay(t *testing.T) {
	ctx := context.Background()
	rdb := testRedis(t)
	client := testEntClient(t)

	_, _ = integrationDB.ExecContext(ctx, "TRUNCATE scheduler_outbox")

	accountRepo := newAccountRepositoryWithSQL(client, integrationDB, nil)
	outboxRepo := NewSchedulerOutboxRepository(integrationDB)
	cache := NewSchedulerCache(rdb)

	cfg := &config.Config{
		RunMode: config.RunModeStandard,
		Gateway: config.GatewayConfig{
			Scheduling: config.GatewaySchedulingConfig{
				OutboxPollIntervalSeconds:  1,
				FullRebuildIntervalSeconds: 0,
				DbFallbackEnabled:          true,
			},
		},
	}

	account := &service.Account{
		Name:        "outbox-replay-" + time.Now().Format("150405.000000"),
		Platform:    service.PlatformOpenAI,
		Type:        service.AccountTypeAPIKey,
		Status:      service.StatusActive,
		Schedulable: true,
		Concurrency: 3,
		Priority:    1,
		Credentials: map[string]any{},
		Extra:       map[string]any{},
	}
	require.NoError(t, accountRepo.Create(ctx, account))
	require.NoError(t, cache.SetAccount(ctx, account))

	svc := service.NewSchedulerSnapshotService(cache, outboxRepo, accountRepo, nil, cfg)
	svc.Start()
	t.Cleanup(svc.Stop)

	require.NoError(t, accountRepo.UpdateLastUsed(ctx, account.ID))
	updated, err := accountRepo.GetByID(ctx, account.ID)
	require.NoError(t, err)
	require.NotNil(t, updated.LastUsedAt)
	expectedUnix := updated.LastUsedAt.Unix()

	require.Eventually(t, func() bool {
		cached, err := cache.GetAccount(ctx, account.ID)
		if err != nil || cached == nil || cached.LastUsedAt == nil {
			return false
		}
		return cached.LastUsedAt.Unix() == expectedUnix
	}, 5*time.Second, 100*time.Millisecond)
}

func TestSchedulerSnapshotOutboxUpdatesAdmissionCapacity(t *testing.T) {
	ctx := context.Background()
	clients := testRedisClients(t, 2)
	client := testEntClient(t)

	_, _ = integrationDB.ExecContext(ctx, "TRUNCATE scheduler_outbox")

	accountRepo := newAccountRepositoryWithSQL(client, integrationDB, nil)
	outboxRepo := NewSchedulerOutboxRepository(integrationDB)
	cache := NewSchedulerCache(clients[0])
	capacityReader := service.NewSchedulerSnapshotService(NewSchedulerCache(clients[1]), nil, nil, nil, nil)
	baselineCapacity := make(map[string]int, 2)
	for _, platform := range []string{service.PlatformOpenAI, service.PlatformAnthropic} {
		baselineAccounts, err := accountRepo.ListSchedulableByPlatform(ctx, platform)
		require.NoError(t, err)
		for i := range baselineAccounts {
			if baselineAccounts[i].Concurrency > 0 {
				baselineCapacity[platform] += baselineAccounts[i].Concurrency
			}
		}
	}

	cfg := &config.Config{
		RunMode: config.RunModeStandard,
		Gateway: config.GatewayConfig{
			Scheduling: config.GatewaySchedulingConfig{
				OutboxPollIntervalSeconds:  1,
				FullRebuildIntervalSeconds: 0,
				DbFallbackEnabled:          true,
			},
		},
	}

	account := &service.Account{
		Name:        "outbox-capacity-" + time.Now().Format("150405.000000"),
		Platform:    service.PlatformOpenAI,
		Type:        service.AccountTypeAPIKey,
		Status:      service.StatusActive,
		Schedulable: true,
		Concurrency: 3,
		Priority:    1,
		Credentials: map[string]any{},
		Extra:       map[string]any{},
	}
	require.NoError(t, accountRepo.Create(ctx, account))

	svc := service.NewSchedulerSnapshotService(cache, outboxRepo, accountRepo, nil, cfg)
	svc.Start()
	t.Cleanup(svc.Stop)

	require.Eventually(t, func() bool {
		snapshot, err := capacityReader.AdmissionCapacity(ctx, service.PlatformOpenAI)
		return err == nil &&
			snapshot.TotalConcurrency == baselineCapacity[service.PlatformOpenAI]+3 &&
			snapshot.AccountConcurrency[account.ID] == 3
	}, 5*time.Second, 100*time.Millisecond)

	account.Concurrency = 5
	require.NoError(t, accountRepo.Update(ctx, account))

	require.Eventually(t, func() bool {
		snapshot, err := capacityReader.AdmissionCapacity(ctx, service.PlatformOpenAI)
		return err == nil &&
			snapshot.TotalConcurrency == baselineCapacity[service.PlatformOpenAI]+5 &&
			snapshot.AccountConcurrency[account.ID] == 5
	}, 5*time.Second, 100*time.Millisecond)

	account.Platform = service.PlatformAnthropic
	require.NoError(t, accountRepo.Update(ctx, account))

	require.Eventually(t, func() bool {
		openAI, openAIErr := capacityReader.AdmissionCapacity(ctx, service.PlatformOpenAI)
		anthropic, anthropicErr := capacityReader.AdmissionCapacity(ctx, service.PlatformAnthropic)
		return openAIErr == nil && anthropicErr == nil &&
			openAI.TotalConcurrency == baselineCapacity[service.PlatformOpenAI] &&
			openAI.AccountConcurrency[account.ID] == 0 &&
			anthropic.TotalConcurrency == baselineCapacity[service.PlatformAnthropic]+5 &&
			anthropic.AccountConcurrency[account.ID] == 5
	}, 5*time.Second, 100*time.Millisecond)

	account.Status = service.StatusDisabled
	require.NoError(t, accountRepo.Update(ctx, account))

	require.Eventually(t, func() bool {
		snapshot, err := capacityReader.AdmissionCapacity(ctx, service.PlatformAnthropic)
		return err == nil &&
			snapshot.TotalConcurrency == baselineCapacity[service.PlatformAnthropic] &&
			snapshot.AccountConcurrency[account.ID] == 0
	}, 5*time.Second, 100*time.Millisecond)

	account.Status = service.StatusActive
	require.NoError(t, accountRepo.Update(ctx, account))

	require.Eventually(t, func() bool {
		snapshot, err := capacityReader.AdmissionCapacity(ctx, service.PlatformAnthropic)
		return err == nil &&
			snapshot.TotalConcurrency == baselineCapacity[service.PlatformAnthropic]+5 &&
			snapshot.AccountConcurrency[account.ID] == 5
	}, 5*time.Second, 100*time.Millisecond)

	require.NoError(t, accountRepo.SetSchedulable(ctx, account.ID, false))

	require.Eventually(t, func() bool {
		snapshot, err := capacityReader.AdmissionCapacity(ctx, service.PlatformAnthropic)
		return err == nil &&
			snapshot.TotalConcurrency == baselineCapacity[service.PlatformAnthropic] &&
			snapshot.AccountConcurrency[account.ID] == 0
	}, 5*time.Second, 100*time.Millisecond)

	require.NoError(t, accountRepo.SetSchedulable(ctx, account.ID, true))

	require.Eventually(t, func() bool {
		snapshot, err := capacityReader.AdmissionCapacity(ctx, service.PlatformAnthropic)
		return err == nil &&
			snapshot.TotalConcurrency == baselineCapacity[service.PlatformAnthropic]+5 &&
			snapshot.AccountConcurrency[account.ID] == 5
	}, 5*time.Second, 100*time.Millisecond)

	require.NoError(t, accountRepo.Delete(ctx, account.ID))

	require.Eventually(t, func() bool {
		snapshot, err := capacityReader.AdmissionCapacity(ctx, service.PlatformAnthropic)
		return err == nil &&
			snapshot.TotalConcurrency == baselineCapacity[service.PlatformAnthropic] &&
			snapshot.AccountConcurrency[account.ID] == 0
	}, 5*time.Second, 100*time.Millisecond)
}

func TestSchedulerSnapshotOutboxRetriesAdmissionCapacityWhenRebuildLockIsBusy(t *testing.T) {
	ctx := context.Background()
	rdb := testRedis(t)
	client := testEntClient(t)

	_, _ = integrationDB.ExecContext(ctx, "TRUNCATE scheduler_outbox")

	accountRepo := newAccountRepositoryWithSQL(client, integrationDB, nil)
	outboxRepo := NewSchedulerOutboxRepository(integrationDB)
	cache := NewSchedulerCache(rdb)
	baselineAccounts, err := accountRepo.ListSchedulableByPlatform(ctx, service.PlatformOpenAI)
	require.NoError(t, err)
	baselineCapacity := 0
	for i := range baselineAccounts {
		if baselineAccounts[i].Concurrency > 0 {
			baselineCapacity += baselineAccounts[i].Concurrency
		}
	}

	account := &service.Account{
		Name:        "outbox-capacity-lock-" + time.Now().Format("150405.000000"),
		Platform:    service.PlatformOpenAI,
		Type:        service.AccountTypeAPIKey,
		Status:      service.StatusActive,
		Schedulable: true,
		Concurrency: 3,
		Priority:    1,
		Credentials: map[string]any{},
		Extra:       map[string]any{},
	}
	require.NoError(t, accountRepo.Create(ctx, account))
	createEventID, err := outboxRepo.MaxID(ctx)
	require.NoError(t, err)

	cfg := &config.Config{
		RunMode: config.RunModeStandard,
		Gateway: config.GatewayConfig{
			Scheduling: config.GatewaySchedulingConfig{
				OutboxPollIntervalSeconds:  1,
				FullRebuildIntervalSeconds: 0,
				DbFallbackEnabled:          true,
			},
		},
	}
	svc := service.NewSchedulerSnapshotService(cache, outboxRepo, accountRepo, nil, cfg)
	svc.Start()
	t.Cleanup(svc.Stop)

	require.Eventually(t, func() bool {
		snapshot, snapshotErr := svc.AdmissionCapacity(ctx, service.PlatformOpenAI)
		return snapshotErr == nil &&
			snapshot.TotalConcurrency == baselineCapacity+3 &&
			snapshot.AccountConcurrency[account.ID] == 3
	}, 5*time.Second, 100*time.Millisecond)
	require.Eventually(t, func() bool {
		watermark, watermarkErr := cache.GetOutboxWatermark(ctx)
		return watermarkErr == nil && watermark >= createEventID
	}, 5*time.Second, 100*time.Millisecond)

	capacityBucket := service.SchedulerBucket{
		GroupID:  0,
		Platform: service.PlatformOpenAI,
		Mode:     "admission_capacity",
	}
	locked, err := cache.TryLockBucket(ctx, capacityBucket, 30*time.Second)
	require.NoError(t, err)
	require.True(t, locked)
	lockHeld := true
	t.Cleanup(func() {
		if lockHeld {
			_ = cache.UnlockBucket(ctx, capacityBucket)
		}
	})

	account.Concurrency = 5
	require.NoError(t, accountRepo.Update(ctx, account))
	updateEventID, err := outboxRepo.MaxID(ctx)
	require.NoError(t, err)
	require.Greater(t, updateEventID, createEventID)

	require.Eventually(t, func() bool {
		var released bool
		queryErr := integrationDB.QueryRowContext(ctx,
			"SELECT dedup_key IS NULL FROM scheduler_outbox WHERE id = $1",
			updateEventID,
		).Scan(&released)
		return queryErr == nil && released
	}, 5*time.Second, 100*time.Millisecond)
	require.Never(t, func() bool {
		watermark, watermarkErr := cache.GetOutboxWatermark(ctx)
		return watermarkErr == nil && watermark >= updateEventID
	}, 1500*time.Millisecond, 100*time.Millisecond)
	require.NoError(t, cache.UnlockBucket(ctx, capacityBucket))
	lockHeld = false

	require.Eventually(t, func() bool {
		snapshot, snapshotErr := svc.AdmissionCapacity(ctx, service.PlatformOpenAI)
		if snapshotErr != nil || snapshot.TotalConcurrency != baselineCapacity+5 || snapshot.AccountConcurrency[account.ID] != 5 {
			return false
		}
		watermark, watermarkErr := cache.GetOutboxWatermark(ctx)
		return watermarkErr == nil && watermark >= updateEventID
	}, 5*time.Second, 100*time.Millisecond)
}

func TestSchedulerSnapshotOutboxSlowInstanceCannotRegressWatermarkOrLastUsed(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	clients := testRedisClients(t, 3)
	cache := NewSchedulerCache(clients[0])
	repo := NewSchedulerOutboxRepository(integrationDB)
	require.NoError(t, truncateSchedulerOutbox(ctx))

	accountID := int64(91_001)
	oldLastUsed := time.Now().Add(-2 * time.Minute).UTC().Truncate(time.Second)
	newLastUsed := oldLastUsed.Add(time.Minute)
	require.NoError(t, cache.SetAccount(ctx, &service.Account{
		ID:          accountID,
		Platform:    service.PlatformOpenAI,
		Status:      service.StatusActive,
		Schedulable: true,
		Concurrency: 1,
	}))
	oldEventID := insertSchedulerLastUsedOutboxEvent(t, ctx, accountID, oldLastUsed)

	releaseSlow := make(chan struct{})
	var releaseSlowOnce sync.Once
	releaseSlowWorker := func() {
		releaseSlowOnce.Do(func() { close(releaseSlow) })
	}
	slowRepo := &blockingSchedulerOutboxRepository{
		SchedulerOutboxRepository: repo,
		listed:                    make(chan struct{}),
		release:                   releaseSlow,
	}
	slow := service.NewSchedulerSnapshotService(
		NewSchedulerCache(clients[0]),
		slowRepo,
		nil,
		nil,
		oneShotSchedulerOutboxConfig(),
	)
	slow.Start()
	t.Cleanup(func() {
		releaseSlowWorker()
		slow.Stop()
	})
	select {
	case <-slowRepo.listed:
	case <-ctx.Done():
		t.Fatal("slow scheduler instance did not read the old outbox event")
	}

	fastOld := service.NewSchedulerSnapshotService(
		NewSchedulerCache(clients[1]),
		repo,
		nil,
		nil,
		oneShotSchedulerOutboxConfig(),
	)
	fastOld.Start()
	require.Eventually(t, func() bool {
		watermark, err := cache.GetOutboxWatermark(ctx)
		return err == nil && watermark == oldEventID
	}, 5*time.Second, 20*time.Millisecond)
	fastOld.Stop()

	newEventID := insertSchedulerLastUsedOutboxEvent(t, ctx, accountID, newLastUsed)
	fastNew := service.NewSchedulerSnapshotService(
		NewSchedulerCache(clients[2]),
		repo,
		nil,
		nil,
		oneShotSchedulerOutboxConfig(),
	)
	fastNew.Start()
	require.Eventually(t, func() bool {
		watermark, err := cache.GetOutboxWatermark(ctx)
		if err != nil || watermark != newEventID {
			return false
		}
		cached, err := cache.GetAccount(ctx, accountID)
		return err == nil && cached != nil && cached.LastUsedAt != nil && cached.LastUsedAt.Equal(newLastUsed)
	}, 5*time.Second, 20*time.Millisecond)
	fastNew.Stop()

	releaseSlowWorker()
	require.Eventually(t, func() bool {
		watermark, err := cache.GetOutboxWatermark(ctx)
		return err == nil && watermark <= newEventID
	}, 5*time.Second, 20*time.Millisecond)
	slow.Stop()

	watermark, err := cache.GetOutboxWatermark(ctx)
	require.NoError(t, err)
	cached, err := cache.GetAccount(ctx, accountID)
	require.NoError(t, err)
	require.NotNil(t, cached)
	require.NotNil(t, cached.LastUsedAt)
	assert.Equal(t, newEventID, watermark, "a slow instance must not lower a newer shared watermark")
	assert.True(t, cached.LastUsedAt.Equal(newLastUsed),
		"an old account_last_used event must not overwrite a newer cached timestamp: got %s want %s",
		cached.LastUsedAt, newLastUsed)
}

func oneShotSchedulerOutboxConfig() *config.Config {
	return &config.Config{
		RunMode: config.RunModeStandard,
		Gateway: config.GatewayConfig{
			Scheduling: config.GatewaySchedulingConfig{
				OutboxPollIntervalSeconds:  3600,
				FullRebuildIntervalSeconds: 0,
				DbFallbackEnabled:          true,
			},
		},
	}
}

func truncateSchedulerOutbox(ctx context.Context) error {
	_, err := integrationDB.ExecContext(ctx, "TRUNCATE scheduler_outbox RESTART IDENTITY")
	return err
}

func insertSchedulerLastUsedOutboxEvent(
	t *testing.T,
	ctx context.Context,
	accountID int64,
	lastUsed time.Time,
) int64 {
	t.Helper()

	payload := []byte(`{"last_used":{"` + fmt.Sprint(accountID) + `":` + fmt.Sprint(lastUsed.Unix()) + `}}`)
	var id int64
	err := integrationDB.QueryRowContext(ctx, `
		INSERT INTO scheduler_outbox (event_type, payload)
		VALUES ($1, $2)
		RETURNING id
	`, service.SchedulerOutboxEventAccountLastUsed, payload).Scan(&id)
	require.NoError(t, err)
	return id
}
