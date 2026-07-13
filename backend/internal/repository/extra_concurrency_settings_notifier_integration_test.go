//go:build integration

package repository

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

type reorderingCrossInstanceSettingRepo struct {
	mu              sync.Mutex
	calls           int
	firstPersisted  chan struct{}
	secondPersisted chan struct{}
	releaseFirst    chan struct{}
	persisted       bool
	persistedFence  int64
}

func (r *reorderingCrossInstanceSettingRepo) Get(context.Context, string) (*service.Setting, error) {
	return nil, service.ErrSettingNotFound
}

func (r *reorderingCrossInstanceSettingRepo) GetValue(context.Context, string) (string, error) {
	return "", service.ErrSettingNotFound
}

func (r *reorderingCrossInstanceSettingRepo) Set(context.Context, string, string) error {
	return nil
}

func (r *reorderingCrossInstanceSettingRepo) GetMultiple(context.Context, []string) (map[string]string, error) {
	return map[string]string{}, nil
}

func (r *reorderingCrossInstanceSettingRepo) SetMultiple(_ context.Context, settings map[string]string) error {
	r.mu.Lock()
	r.calls++
	call := r.calls
	r.persisted = settings[service.SettingKeyExtraConcurrencyEnabled] == "true"
	if call == 1 {
		close(r.firstPersisted)
	}
	if call == 2 {
		close(r.secondPersisted)
	}
	r.mu.Unlock()
	if call == 1 {
		<-r.releaseFirst
	}
	return nil
}

func (r *reorderingCrossInstanceSettingRepo) ReserveSettingUpdateFence(context.Context) (int64, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.persistedFence++
	return r.persistedFence, nil
}

func (r *reorderingCrossInstanceSettingRepo) SetMultipleFenced(
	_ context.Context,
	settings map[string]string,
	fence int64,
) error {
	r.mu.Lock()
	if fence != r.persistedFence {
		r.mu.Unlock()
		return service.ErrStaleSettingUpdateFence
	}
	r.calls++
	call := r.calls
	r.persisted = settings[service.SettingKeyExtraConcurrencyEnabled] == "true"
	if call == 1 {
		close(r.firstPersisted)
	}
	if call == 2 {
		close(r.secondPersisted)
	}
	r.mu.Unlock()
	if call == 1 {
		<-r.releaseFirst
	}
	return nil
}

func (r *reorderingCrossInstanceSettingRepo) GetAll(context.Context) (map[string]string, error) {
	return map[string]string{}, nil
}

func (r *reorderingCrossInstanceSettingRepo) Delete(context.Context, string) error {
	return nil
}

func (r *reorderingCrossInstanceSettingRepo) persistedEnabled() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.persisted
}

type leaseLossCrossInstanceSettingRepo struct {
	mu             sync.Mutex
	persisted      bool
	persistedFence int64
	oldStarted     chan struct{}
	releaseOld     chan struct{}
	oldOnce        sync.Once
}

type blockingRealFencedSettingRepo struct {
	service.SettingRepository
	fenced     service.FencedSettingRepository
	oldReady   chan int64
	newReady   chan int64
	releaseOld chan struct{}
	releaseNew chan struct{}
	oldOnce    sync.Once
	newOnce    sync.Once
}

type blockingFirstFenceReservationRepo struct {
	service.SettingRepository
	fenced              service.FencedSettingRepository
	mu                  sync.Mutex
	reserveCalls        int
	updates             []bool
	firstReserveStarted chan struct{}
	releaseFirstReserve chan struct{}
	firstWriteStarted   chan struct{}
	releaseFirstWrite   chan struct{}
	firstWriteOnce      sync.Once
}

func (r *blockingFirstFenceReservationRepo) ReserveSettingUpdateFence(context.Context) (int64, error) {
	r.mu.Lock()
	r.reserveCalls++
	first := r.reserveCalls == 1
	r.mu.Unlock()
	if first {
		close(r.firstReserveStarted)
		<-r.releaseFirstReserve
	}
	return r.fenced.ReserveSettingUpdateFence(context.Background())
}

func (r *blockingFirstFenceReservationRepo) SetMultipleFenced(
	ctx context.Context,
	settings map[string]string,
	fence int64,
) error {
	if r.firstWriteStarted != nil {
		r.firstWriteOnce.Do(func() {
			close(r.firstWriteStarted)
			<-r.releaseFirstWrite
		})
	}
	r.mu.Lock()
	r.updates = append(r.updates, settings[service.SettingKeyExtraConcurrencyEnabled] == "true")
	r.mu.Unlock()
	return r.fenced.SetMultipleFenced(ctx, settings, fence)
}

func (r *blockingFirstFenceReservationRepo) completedUpdates() []bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]bool(nil), r.updates...)
}

func (r *blockingRealFencedSettingRepo) ReserveSettingUpdateFence(ctx context.Context) (int64, error) {
	return r.fenced.ReserveSettingUpdateFence(ctx)
}

func (r *blockingRealFencedSettingRepo) SetMultipleFenced(
	ctx context.Context,
	settings map[string]string,
	fence int64,
) error {
	enabled := settings[service.SettingKeyExtraConcurrencyEnabled] == "true"
	if enabled {
		r.oldOnce.Do(func() { r.oldReady <- fence })
		<-r.releaseOld
	} else {
		r.newOnce.Do(func() { r.newReady <- fence })
		<-r.releaseNew
	}
	return r.fenced.SetMultipleFenced(ctx, settings, fence)
}

func (r *leaseLossCrossInstanceSettingRepo) Get(context.Context, string) (*service.Setting, error) {
	return nil, service.ErrSettingNotFound
}

func (r *leaseLossCrossInstanceSettingRepo) GetValue(context.Context, string) (string, error) {
	return "", service.ErrSettingNotFound
}

func (r *leaseLossCrossInstanceSettingRepo) Set(context.Context, string, string) error {
	return nil
}

func (r *leaseLossCrossInstanceSettingRepo) GetMultiple(context.Context, []string) (map[string]string, error) {
	return map[string]string{}, nil
}

func (r *leaseLossCrossInstanceSettingRepo) SetMultiple(_ context.Context, settings map[string]string) error {
	return r.persist(settings, 0, false)
}

func (r *leaseLossCrossInstanceSettingRepo) ReserveSettingUpdateFence(context.Context) (int64, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.persistedFence++
	return r.persistedFence, nil
}

func (r *leaseLossCrossInstanceSettingRepo) SetMultipleFenced(
	_ context.Context,
	settings map[string]string,
	fence int64,
) error {
	return r.persist(settings, fence, true)
}

func (r *leaseLossCrossInstanceSettingRepo) persist(settings map[string]string, fence int64, fenced bool) error {
	enabled := settings[service.SettingKeyExtraConcurrencyEnabled] == "true"
	if enabled {
		r.oldOnce.Do(func() { close(r.oldStarted) })
		<-r.releaseOld
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	if fenced && fence != r.persistedFence {
		return fmt.Errorf("stale settings update fence %d != %d", fence, r.persistedFence)
	}
	r.persisted = enabled
	return nil
}

func (r *leaseLossCrossInstanceSettingRepo) GetAll(context.Context) (map[string]string, error) {
	return map[string]string{}, nil
}

func (r *leaseLossCrossInstanceSettingRepo) Delete(context.Context, string) error {
	return nil
}

func (r *leaseLossCrossInstanceSettingRepo) persistedEnabled() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.persisted
}

type blockedExtraConcurrencyInvalidationNotifier struct {
	service.ExtraConcurrencySettingsNotifier
	notificationArrived chan struct{}
	releaseNotification chan struct{}
	notificationOnce    sync.Once
}

func (n *blockedExtraConcurrencyInvalidationNotifier) SubscribeExtraConcurrencySettingsInvalidation(
	ctx context.Context,
	handler func(),
) error {
	return n.ExtraConcurrencySettingsNotifier.SubscribeExtraConcurrencySettingsInvalidation(ctx, func() {
		n.notificationOnce.Do(func() { close(n.notificationArrived) })
		select {
		case <-ctx.Done():
			return
		case <-n.releaseNotification:
		}
		if handler != nil {
			handler()
		}
	})
}

func TestExtraConcurrencySettingsInvalidationPropagatesAcrossInstances(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	clients := testRedisClients(t, 2)
	tx := testEntTx(t)
	repo := NewSettingRepository(tx.Client())
	require.NoError(t, repo.SetMultiple(ctx, map[string]string{
		service.SettingKeyExtraConcurrencyEnabled:            "true",
		service.SettingKeyExtraConcurrencyWaitTimeoutSeconds: "45",
		service.SettingKeyExtraConcurrencyReservePercent:     "20",
		service.SettingKeyExtraConcurrencyMinReservedSlots:   "2",
		service.SettingKeyExtraConcurrencyPlatformReserves:   `{}`,
	}))

	instanceA := service.NewSettingService(repo, &config.Config{})
	instanceB := service.NewSettingService(repo, &config.Config{})
	instanceA.SetExtraConcurrencySettingsNotifier(NewExtraConcurrencySettingsNotifier(clients[0]))
	instanceB.SetExtraConcurrencySettingsNotifier(NewExtraConcurrencySettingsNotifier(clients[1]))
	require.NoError(t, instanceB.StartExtraConcurrencySettingsSubscriber(ctx))

	before := instanceB.GetExtraConcurrencyRuntimeSettings(ctx)
	require.True(t, before.Enabled)
	require.Equal(t, 45, before.WaitTimeoutSeconds)
	require.Equal(t, 20.0, before.ReservePercent)

	settings, err := instanceA.GetAllSettings(ctx)
	require.NoError(t, err)
	settings.ExtraConcurrencyEnabled = false
	settings.ExtraConcurrencyWaitTimeoutSeconds = 60
	settings.ExtraConcurrencyReservePercent = 35
	require.NoError(t, instanceA.UpdateSettings(ctx, settings))

	require.Eventually(t, func() bool {
		got := instanceB.GetExtraConcurrencyRuntimeSettings(ctx)
		return !got.Enabled && got.WaitTimeoutSeconds == 60 && got.ReservePercent == 35
	}, time.Second, 20*time.Millisecond)
}

func TestExtraConcurrencySettingsUpdatesSerializePersistenceThroughDrainPublishAcrossInstances(t *testing.T) {
	clients := testRedisClients(t, 2)
	require.NoError(t, clients[0].Del(
		t.Context(),
		extraConcurrencyAdmissionDrainKey,
		extraConcurrencyAdmissionEpochKey,
	).Err())
	repo := &reorderingCrossInstanceSettingRepo{
		firstPersisted:  make(chan struct{}),
		secondPersisted: make(chan struct{}),
		releaseFirst:    make(chan struct{}),
	}
	instanceA := service.NewSettingService(repo, &config.Config{})
	instanceB := service.NewSettingService(repo, &config.Config{})
	instanceA.SetExtraConcurrencySettingsNotifier(NewExtraConcurrencySettingsNotifier(clients[0]))
	instanceB.SetExtraConcurrencySettingsNotifier(NewExtraConcurrencySettingsNotifier(clients[1]))

	firstDone := make(chan error, 1)
	go func() {
		firstDone <- instanceA.UpdateSettings(t.Context(), crossInstanceExtraConcurrencySettings(true))
	}()
	select {
	case <-repo.firstPersisted:
	case <-time.After(time.Second):
		t.Fatal("first settings update did not persist")
	}

	secondDone := make(chan error, 1)
	go func() {
		secondDone <- instanceB.UpdateSettings(t.Context(), crossInstanceExtraConcurrencySettings(false))
	}()
	secondCompleted := false
	select {
	case <-repo.secondPersisted:
		select {
		case err := <-secondDone:
			require.NoError(t, err)
			secondCompleted = true
		case <-time.After(time.Second):
			t.Fatal("newer disable update did not publish before the delayed enable refresh")
		}
		require.Equal(t, int64(1), clients[0].Exists(t.Context(), extraConcurrencyAdmissionDrainKey).Val())
	case <-time.After(100 * time.Millisecond):
	}
	close(repo.releaseFirst)

	select {
	case err := <-firstDone:
		require.NoError(t, err)
	case <-time.After(2 * time.Second):
		t.Fatal("first settings update did not finish")
	}
	if !secondCompleted {
		select {
		case err := <-secondDone:
			require.NoError(t, err)
		case <-time.After(2 * time.Second):
			t.Fatal("second settings update did not finish")
		}
	}
	require.False(t, repo.persistedEnabled())
	require.Equal(t, int64(1), clients[0].Exists(t.Context(), extraConcurrencyAdmissionDrainKey).Val(),
		"the final persisted disable must retain the global admission drain")
}

func TestExtraConcurrencySettingsLeaseLossCannotLetOlderWriterOverwriteNewerPersistence(t *testing.T) {
	clients := testRedisClients(t, 2)
	repo := &leaseLossCrossInstanceSettingRepo{
		oldStarted: make(chan struct{}),
		releaseOld: make(chan struct{}),
	}
	notifierA := &extraConcurrencySettingsNotifier{
		rdb:                     clients[0],
		drainTTL:                extraConcurrencyAdmissionDrainTTL,
		updateLockTTL:           150 * time.Millisecond,
		updateLockRetryInterval: 5 * time.Millisecond,
	}
	notifierB := &extraConcurrencySettingsNotifier{
		rdb:                     clients[1],
		drainTTL:                extraConcurrencyAdmissionDrainTTL,
		updateLockTTL:           150 * time.Millisecond,
		updateLockRetryInterval: 5 * time.Millisecond,
	}
	instanceA := service.NewSettingService(repo, &config.Config{})
	instanceB := service.NewSettingService(repo, &config.Config{})
	instanceA.SetExtraConcurrencySettingsNotifier(notifierA)
	instanceB.SetExtraConcurrencySettingsNotifier(notifierB)

	oldDone := make(chan error, 1)
	go func() {
		oldDone <- instanceA.UpdateSettings(t.Context(), crossInstanceExtraConcurrencySettings(true))
	}()
	select {
	case <-repo.oldStarted:
	case <-time.After(time.Second):
		t.Fatal("older writer did not enter persistence")
	}
	require.NoError(t, clients[0].Set(
		t.Context(),
		extraConcurrencySettingsUpdateLockKey,
		"replacement-owner",
		100*time.Millisecond,
	).Err())

	require.NoError(t, instanceB.UpdateSettings(t.Context(), crossInstanceExtraConcurrencySettings(false)))
	close(repo.releaseOld)
	select {
	case err := <-oldDone:
		require.Error(t, err, "the older writer must be rejected after losing its lease")
	case <-time.After(2 * time.Second):
		t.Fatal("older writer did not finish")
	}

	require.False(t, repo.persistedEnabled(), "the newer disable must remain the persisted value")
	require.Equal(t, int64(1), clients[0].Exists(t.Context(), extraConcurrencyAdmissionDrainKey).Val())
}

func TestExtraConcurrencySettingsRedisResetCannotReuseInflightDatabaseFence(t *testing.T) {
	clients := testRedisClients(t, 2)
	base := NewSettingRepository(integrationEntClient)
	fenced := base.(service.FencedSettingRepository)
	settingKeys := []string{
		extraConcurrencySettingsUpdateFenceSettingKey,
		service.SettingKeyExtraConcurrencyEnabled,
		service.SettingKeyExtraConcurrencyWaitTimeoutSeconds,
		service.SettingKeyExtraConcurrencyReservePercent,
		service.SettingKeyExtraConcurrencyMinReservedSlots,
		service.SettingKeyExtraConcurrencyPlatformReserves,
	}
	for _, key := range settingKeys {
		require.NoError(t, base.Delete(t.Context(), key))
	}
	t.Cleanup(func() {
		for _, key := range settingKeys {
			_ = base.Delete(context.Background(), key)
		}
	})
	require.NoError(t, base.Set(t.Context(), extraConcurrencySettingsUpdateFenceSettingKey, "50"))
	require.NoError(t, fenced.SetMultipleFenced(t.Context(), map[string]string{
		service.SettingKeyExtraConcurrencyEnabled:            "false",
		service.SettingKeyExtraConcurrencyWaitTimeoutSeconds: "30",
		service.SettingKeyExtraConcurrencyReservePercent:     "10",
		service.SettingKeyExtraConcurrencyMinReservedSlots:   "1",
		service.SettingKeyExtraConcurrencyPlatformReserves:   `{}`,
	}, 50))

	repo := &blockingRealFencedSettingRepo{
		SettingRepository: base,
		fenced:            fenced,
		oldReady:          make(chan int64, 1),
		newReady:          make(chan int64, 1),
		releaseOld:        make(chan struct{}),
		releaseNew:        make(chan struct{}),
	}
	notifierA := &extraConcurrencySettingsNotifier{
		rdb:                     clients[0],
		drainTTL:                extraConcurrencyAdmissionDrainTTL,
		updateLockTTL:           150 * time.Millisecond,
		updateLockRetryInterval: 5 * time.Millisecond,
	}
	notifierB := &extraConcurrencySettingsNotifier{
		rdb:                     clients[1],
		drainTTL:                extraConcurrencyAdmissionDrainTTL,
		updateLockTTL:           150 * time.Millisecond,
		updateLockRetryInterval: 5 * time.Millisecond,
	}
	instanceA := service.NewSettingService(repo, &config.Config{})
	instanceB := service.NewSettingService(repo, &config.Config{})
	instanceA.SetExtraConcurrencySettingsNotifier(notifierA)
	instanceB.SetExtraConcurrencySettingsNotifier(notifierB)

	oldDone := make(chan error, 1)
	go func() {
		oldDone <- instanceA.UpdateSettings(t.Context(), crossInstanceExtraConcurrencySettings(true))
	}()
	var oldFence int64
	select {
	case oldFence = <-repo.oldReady:
	case <-time.After(time.Second):
		t.Fatal("older writer did not reserve a database fence")
	}
	require.NoError(t, clients[0].Del(
		t.Context(),
		extraConcurrencySettingsUpdateLockKey,
	).Err())

	newDone := make(chan error, 1)
	go func() {
		newDone <- instanceB.UpdateSettings(t.Context(), crossInstanceExtraConcurrencySettings(false))
	}()
	var newFence int64
	select {
	case newFence = <-repo.newReady:
	case <-time.After(time.Second):
		t.Fatal("newer writer did not reserve a database fence")
	}

	close(repo.releaseOld)
	var oldErr error
	select {
	case oldErr = <-oldDone:
	case <-time.After(2 * time.Second):
		t.Fatal("older writer did not finish")
	}
	close(repo.releaseNew)
	var newErr error
	select {
	case newErr = <-newDone:
	case <-time.After(2 * time.Second):
		t.Fatal("newer writer did not finish")
	}

	require.Error(t, oldErr, "the older writer must fail after losing Redis ownership")
	require.NoError(t, newErr, "the newer writer must retain a distinct database fence")
	require.Greater(t, newFence, oldFence)
	enabled, err := base.GetValue(t.Context(), service.SettingKeyExtraConcurrencyEnabled)
	require.NoError(t, err)
	require.Equal(t, "false", enabled)
}

func TestExtraConcurrencySettingsLeaseLossDuringFenceReserveSkipsOlderWrite(t *testing.T) {
	clients := testRedisClients(t, 2)
	base := NewSettingRepository(integrationEntClient)
	fenced := base.(service.FencedSettingRepository)
	settingKeys := []string{
		extraConcurrencySettingsUpdateFenceSettingKey,
		service.SettingKeyExtraConcurrencyEnabled,
		service.SettingKeyExtraConcurrencyWaitTimeoutSeconds,
		service.SettingKeyExtraConcurrencyReservePercent,
		service.SettingKeyExtraConcurrencyMinReservedSlots,
		service.SettingKeyExtraConcurrencyPlatformReserves,
	}
	for _, key := range settingKeys {
		require.NoError(t, base.Delete(t.Context(), key))
	}
	t.Cleanup(func() {
		for _, key := range settingKeys {
			_ = base.Delete(context.Background(), key)
		}
	})
	require.NoError(t, base.Set(t.Context(), extraConcurrencySettingsUpdateFenceSettingKey, "50"))
	require.NoError(t, fenced.SetMultipleFenced(t.Context(), map[string]string{
		service.SettingKeyExtraConcurrencyEnabled:            "false",
		service.SettingKeyExtraConcurrencyWaitTimeoutSeconds: "30",
		service.SettingKeyExtraConcurrencyReservePercent:     "10",
		service.SettingKeyExtraConcurrencyMinReservedSlots:   "1",
		service.SettingKeyExtraConcurrencyPlatformReserves:   `{}`,
	}, 50))

	repo := &blockingFirstFenceReservationRepo{
		SettingRepository:   base,
		fenced:              fenced,
		firstReserveStarted: make(chan struct{}),
		releaseFirstReserve: make(chan struct{}),
	}
	notifierA := &extraConcurrencySettingsNotifier{
		rdb:                     clients[0],
		drainTTL:                extraConcurrencyAdmissionDrainTTL,
		updateLockTTL:           5 * time.Second,
		updateLockRetryInterval: 5 * time.Millisecond,
	}
	notifierB := &extraConcurrencySettingsNotifier{
		rdb:                     clients[1],
		drainTTL:                extraConcurrencyAdmissionDrainTTL,
		updateLockTTL:           5 * time.Second,
		updateLockRetryInterval: 5 * time.Millisecond,
	}
	instanceA := service.NewSettingService(repo, &config.Config{})
	instanceB := service.NewSettingService(repo, &config.Config{})
	instanceA.SetExtraConcurrencySettingsNotifier(notifierA)
	instanceB.SetExtraConcurrencySettingsNotifier(notifierB)

	oldDone := make(chan error, 1)
	go func() {
		oldDone <- instanceA.UpdateSettings(t.Context(), crossInstanceExtraConcurrencySettings(true))
	}()
	select {
	case <-repo.firstReserveStarted:
	case <-time.After(time.Second):
		t.Fatal("older writer did not enter fence reservation")
	}
	require.NoError(t, clients[0].Del(t.Context(), extraConcurrencySettingsUpdateLockKey).Err())
	require.NoError(t, instanceB.UpdateSettings(t.Context(), crossInstanceExtraConcurrencySettings(false)))
	close(repo.releaseFirstReserve)
	select {
	case err := <-oldDone:
		require.Error(t, err, "older writer must fail ownership verification after reserve")
	case <-time.After(2 * time.Second):
		t.Fatal("older writer did not finish")
	}

	require.Equal(t, []bool{false}, repo.completedUpdates(), "the stale owner must never enter the write callback")
	enabled, err := base.GetValue(t.Context(), service.SettingKeyExtraConcurrencyEnabled)
	require.NoError(t, err)
	require.Equal(t, "false", enabled)
	fence, err := base.GetValue(t.Context(), extraConcurrencySettingsUpdateFenceSettingKey)
	require.NoError(t, err)
	require.Equal(t, "52", fence)
}

func TestExtraConcurrencySettingsCurrentOwnerRetriesAfterStaleFenceAdvance(t *testing.T) {
	clients := testRedisClients(t, 2)
	base := NewSettingRepository(integrationEntClient)
	fenced := base.(service.FencedSettingRepository)
	settingKeys := []string{
		extraConcurrencySettingsUpdateFenceSettingKey,
		service.SettingKeyExtraConcurrencyEnabled,
		service.SettingKeyExtraConcurrencyWaitTimeoutSeconds,
		service.SettingKeyExtraConcurrencyReservePercent,
		service.SettingKeyExtraConcurrencyMinReservedSlots,
		service.SettingKeyExtraConcurrencyPlatformReserves,
	}
	for _, key := range settingKeys {
		require.NoError(t, base.Delete(t.Context(), key))
	}
	t.Cleanup(func() {
		for _, key := range settingKeys {
			_ = base.Delete(context.Background(), key)
		}
	})
	require.NoError(t, base.Set(t.Context(), extraConcurrencySettingsUpdateFenceSettingKey, "50"))
	require.NoError(t, fenced.SetMultipleFenced(t.Context(), map[string]string{
		service.SettingKeyExtraConcurrencyEnabled:            "false",
		service.SettingKeyExtraConcurrencyWaitTimeoutSeconds: "30",
		service.SettingKeyExtraConcurrencyReservePercent:     "10",
		service.SettingKeyExtraConcurrencyMinReservedSlots:   "1",
		service.SettingKeyExtraConcurrencyPlatformReserves:   `{}`,
	}, 50))

	repo := &blockingFirstFenceReservationRepo{
		SettingRepository:   base,
		fenced:              fenced,
		firstReserveStarted: make(chan struct{}),
		releaseFirstReserve: make(chan struct{}),
		firstWriteStarted:   make(chan struct{}),
		releaseFirstWrite:   make(chan struct{}),
	}
	notifierA := &extraConcurrencySettingsNotifier{
		rdb:                     clients[0],
		drainTTL:                extraConcurrencyAdmissionDrainTTL,
		updateLockTTL:           5 * time.Second,
		updateLockRetryInterval: 5 * time.Millisecond,
	}
	notifierB := &extraConcurrencySettingsNotifier{
		rdb:                     clients[1],
		drainTTL:                extraConcurrencyAdmissionDrainTTL,
		updateLockTTL:           5 * time.Second,
		updateLockRetryInterval: 5 * time.Millisecond,
	}
	instanceA := service.NewSettingService(repo, &config.Config{})
	instanceB := service.NewSettingService(repo, &config.Config{})
	instanceA.SetExtraConcurrencySettingsNotifier(notifierA)
	instanceB.SetExtraConcurrencySettingsNotifier(notifierB)

	oldDone := make(chan error, 1)
	go func() {
		oldDone <- instanceA.UpdateSettings(t.Context(), crossInstanceExtraConcurrencySettings(true))
	}()
	select {
	case <-repo.firstReserveStarted:
	case <-time.After(time.Second):
		t.Fatal("older writer did not enter fence reservation")
	}
	require.NoError(t, clients[0].Del(t.Context(), extraConcurrencySettingsUpdateLockKey).Err())

	newDone := make(chan error, 1)
	go func() {
		newDone <- instanceB.UpdateSettings(t.Context(), crossInstanceExtraConcurrencySettings(false))
	}()
	select {
	case <-repo.firstWriteStarted:
	case <-time.After(time.Second):
		t.Fatal("current owner did not reach its first fenced write")
	}
	close(repo.releaseFirstReserve)
	select {
	case err := <-oldDone:
		require.Error(t, err, "older writer must fail post-reserve ownership verification")
	case <-time.After(2 * time.Second):
		t.Fatal("older writer did not finish")
	}
	close(repo.releaseFirstWrite)
	select {
	case err := <-newDone:
		require.NoError(t, err, "current owner must retry after a stale fence advance")
	case <-time.After(2 * time.Second):
		t.Fatal("current owner did not finish")
	}

	require.Equal(t, []bool{false, false}, repo.completedUpdates())
	enabled, err := base.GetValue(t.Context(), service.SettingKeyExtraConcurrencyEnabled)
	require.NoError(t, err)
	require.Equal(t, "false", enabled)
	fence, err := base.GetValue(t.Context(), extraConcurrencySettingsUpdateFenceSettingKey)
	require.NoError(t, err)
	require.Equal(t, "53", fence)
}

func TestExtraConcurrencySettingsUpdatePanicStopsRenewalAndReleasesLease(t *testing.T) {
	client := testRedis(t)
	notifier := &extraConcurrencySettingsNotifier{
		rdb:                     client,
		drainTTL:                extraConcurrencyAdmissionDrainTTL,
		updateLockTTL:           150 * time.Millisecond,
		updateLockRetryInterval: 5 * time.Millisecond,
	}
	panicValue := make(chan any, 1)
	go func() {
		defer func() { panicValue <- recover() }()
		_ = notifier.SerializeExtraConcurrencySettingsUpdate(
			t.Context(),
			true,
			func(context.Context) (int64, error) { return 1, nil },
			func(context.Context, int64) error {
				panic("settings update panic")
			},
		)
	}()
	select {
	case got := <-panicValue:
		require.Equal(t, "settings update panic", got)
	case <-time.After(time.Second):
		t.Fatal("settings update panic did not propagate")
	}

	require.Eventually(t, func() bool {
		return client.Exists(t.Context(), extraConcurrencySettingsUpdateLockKey).Val() == 0
	}, 500*time.Millisecond, 10*time.Millisecond, "panic cleanup must stop renewal and release the lease")
}

func TestExtraConcurrencySettingsUpdateReturnsCallbackAndOwnershipLossErrors(t *testing.T) {
	clients := testRedisClients(t, 2)
	notifier := &extraConcurrencySettingsNotifier{
		rdb:                     clients[0],
		drainTTL:                extraConcurrencyAdmissionDrainTTL,
		updateLockTTL:           150 * time.Millisecond,
		updateLockRetryInterval: 5 * time.Millisecond,
	}
	callbackErr := errors.New("settings callback failed")
	err := notifier.SerializeExtraConcurrencySettingsUpdate(
		t.Context(),
		true,
		func(context.Context) (int64, error) { return 1, nil },
		func(ctx context.Context, _ int64) error {
			require.NoError(t, clients[1].Set(
				t.Context(),
				extraConcurrencySettingsUpdateLockKey,
				"replacement-owner",
				time.Second,
			).Err())
			select {
			case <-ctx.Done():
			case <-time.After(time.Second):
				t.Fatal("renewal did not detect ownership loss")
			}
			return callbackErr
		},
	)
	require.ErrorIs(t, err, callbackErr)
	require.ErrorIs(t, err, errExtraConcurrencySettingsUpdateLockLost)
}

func TestExtraConcurrencySettingsUpdatePreservesReleaseFailure(t *testing.T) {
	client := testRedis(t)
	notifier := &extraConcurrencySettingsNotifier{
		rdb:                     client,
		drainTTL:                extraConcurrencyAdmissionDrainTTL,
		updateLockTTL:           150 * time.Millisecond,
		updateLockRetryInterval: 5 * time.Millisecond,
	}
	reserveErr := errors.New("reserve failed")
	err := notifier.SerializeExtraConcurrencySettingsUpdate(
		t.Context(),
		true,
		func(context.Context) (int64, error) {
			require.NoError(t, client.Close())
			return 0, reserveErr
		},
		func(context.Context, int64) error {
			t.Fatal("write callback must not run when fence reservation fails")
			return nil
		},
	)
	require.ErrorIs(t, err, reserveErr)
	require.ErrorContains(t, err, "client is closed")
}

func TestExtraConcurrencySettingsFenceAllocationSurvivesRedisStateLoss(t *testing.T) {
	client := testRedis(t)
	tx := testEntTx(t)
	repo := NewSettingRepository(tx.Client()).(*settingRepository)
	require.NoError(t, repo.Set(t.Context(), extraConcurrencySettingsUpdateFenceSettingKey, "50"))
	require.NoError(t, repo.SetMultipleFenced(t.Context(), map[string]string{
		service.SettingKeyExtraConcurrencyEnabled:            "true",
		service.SettingKeyExtraConcurrencyWaitTimeoutSeconds: "30",
		service.SettingKeyExtraConcurrencyReservePercent:     "10",
		service.SettingKeyExtraConcurrencyMinReservedSlots:   "1",
		service.SettingKeyExtraConcurrencyPlatformReserves:   `{}`,
	}, 50))
	require.NoError(t, client.Del(
		t.Context(),
		extraConcurrencySettingsUpdateLockKey,
		extraConcurrencyAdmissionDrainKey,
		extraConcurrencyAdmissionEpochKey,
	).Err())

	instance := service.NewSettingService(repo, &config.Config{})
	instance.SetExtraConcurrencySettingsNotifier(NewExtraConcurrencySettingsNotifier(client))
	require.NoError(t, instance.UpdateSettings(t.Context(), crossInstanceExtraConcurrencySettings(false)))

	fence, err := repo.GetValue(t.Context(), extraConcurrencySettingsUpdateFenceSettingKey)
	require.NoError(t, err)
	require.Equal(t, "51", fence)
	enabled, err := repo.GetValue(t.Context(), service.SettingKeyExtraConcurrencyEnabled)
	require.NoError(t, err)
	require.Equal(t, "false", enabled)
}

func crossInstanceExtraConcurrencySettings(enabled bool) *service.SystemSettings {
	return &service.SystemSettings{
		ExtraConcurrencyEnabled:            enabled,
		ExtraConcurrencyWaitTimeoutSeconds: 30,
		ExtraConcurrencyReservePercent:     10,
		ExtraConcurrencyMinReservedSlots:   1,
		ExtraConcurrencyPlatformReserves:   map[string]service.ExtraConcurrencyPlatformReserve{},
	}
}

func TestExtraConcurrencyDisableDrainPreventsStaleInstanceFromOvertakingLegacyWaiter(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	clients := testRedisClients(t, 2)
	tx := testEntTx(t)
	repo := NewSettingRepository(tx.Client())
	require.NoError(t, repo.SetMultiple(ctx, map[string]string{
		service.SettingKeyExtraConcurrencyEnabled:            "true",
		service.SettingKeyExtraConcurrencyWaitTimeoutSeconds: "45",
		service.SettingKeyExtraConcurrencyReservePercent:     "20",
		service.SettingKeyExtraConcurrencyMinReservedSlots:   "2",
		service.SettingKeyExtraConcurrencyPlatformReserves:   `{}`,
	}))

	instanceA := service.NewSettingService(repo, &config.Config{})
	instanceB := service.NewSettingService(repo, &config.Config{})
	instanceA.SetExtraConcurrencySettingsNotifier(NewExtraConcurrencySettingsNotifier(clients[0]))
	blockedNotifier := &blockedExtraConcurrencyInvalidationNotifier{
		ExtraConcurrencySettingsNotifier: NewExtraConcurrencySettingsNotifier(clients[1]),
		notificationArrived:              make(chan struct{}),
		releaseNotification:              make(chan struct{}),
	}
	instanceB.SetExtraConcurrencySettingsNotifier(blockedNotifier)
	require.NoError(t, instanceB.StartExtraConcurrencySettingsSubscriber(ctx))
	t.Cleanup(func() { close(blockedNotifier.releaseNotification) })

	before := instanceB.GetExtraConcurrencyRuntimeSettings(ctx)
	require.True(t, before.Enabled)

	legacy := NewConcurrencyCache(clients[0], 1, 60)
	gateway := NewGatewayAdmissionStore(clients[1], time.Minute)
	const userID int64 = 1_107
	require.True(t, mustAcquireLegacyUserSlot(t, legacy, ctx, userID, 1, "legacy-active"))
	require.True(t, mustIncrementWaitCount(t, legacy, ctx, userID, 20))
	require.False(t, mustAcquireLegacyUserSlot(t, legacy, ctx, userID, 1, "legacy-earlier-waiter"))

	settings, err := instanceA.GetAllSettings(ctx)
	require.NoError(t, err)
	settings.ExtraConcurrencyEnabled = false
	require.NoError(t, instanceA.UpdateSettings(ctx, settings))

	select {
	case <-blockedNotifier.notificationArrived:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for the remote invalidation notification")
	}
	require.True(t, instanceB.GetExtraConcurrencyRuntimeSettings(ctx).Enabled, "barrier keeps instance B stale")

	later, err := gateway.TryAcquireUserLease(ctx, service.UserLeaseRequest{
		RequestID:     "gateway-later-request",
		UserID:        userID,
		StandardLimit: 1,
		ExtraLimit:    1,
		MaxWaiting:    20,
		WaitTimeout:   time.Minute,
	})
	require.NoError(t, err)
	require.False(t, later.Acquired, "a stale enabled instance must not admit new gateway work while draining")
	require.True(t, later.Draining, "the caller needs an explicit signal to fall back to legacy admission")

	require.NoError(t, legacy.ReleaseUserSlot(ctx, userID, "legacy-active"))
	require.True(t, mustAcquireLegacyUserSlot(t, legacy, ctx, userID, 1, "legacy-earlier-waiter"))
}

func TestExtraConcurrencyDisableDrainAllowsExistingGatewayWorkToFinish(t *testing.T) {
	clients := testRedisClients(t, 2)
	notifier := NewExtraConcurrencySettingsNotifier(clients[0])
	store := NewGatewayAdmissionStore(clients[1], time.Minute)
	request := service.UserLeaseRequest{
		UserID:        1_108,
		StandardLimit: 1,
		MaxWaiting:    20,
		WaitTimeout:   time.Minute,
	}

	request.RequestID = "gateway-active"
	active, err := store.TryAcquireUserLease(t.Context(), request)
	require.NoError(t, err)
	require.True(t, active.Acquired)

	request.RequestID = "gateway-existing-waiter"
	waiting, err := store.TryAcquireUserLease(t.Context(), request)
	require.NoError(t, err)
	require.False(t, waiting.Acquired)
	require.False(t, waiting.Draining)

	require.NoError(t, notifier.PublishExtraConcurrencySettingsState(t.Context(), false))

	request.RequestID = "gateway-active"
	renewed, err := store.TryAcquireUserLease(t.Context(), request)
	require.NoError(t, err)
	require.True(t, renewed.Acquired, "an existing lease must remain renewable while draining")
	require.False(t, renewed.Draining)

	request.RequestID = "gateway-new"
	blocked, err := store.TryAcquireUserLease(t.Context(), request)
	require.NoError(t, err)
	require.False(t, blocked.Acquired)
	require.True(t, blocked.Draining)

	require.NoError(t, store.ReleaseUserLease(t.Context(), request.UserID, "gateway-active"))
	request.RequestID = "gateway-existing-waiter"
	drained, err := store.TryAcquireUserLease(t.Context(), request)
	require.NoError(t, err)
	require.True(t, drained.Acquired, "a request already queued before shutdown must be allowed to drain")
	require.False(t, drained.Draining)
}

func TestExtraConcurrencyDisableDrainConvertsStaleQueuedExtraToStandard(t *testing.T) {
	clients := testRedisClients(t, 2)
	notifier := NewExtraConcurrencySettingsNotifier(clients[0])
	store := NewGatewayAdmissionStore(clients[1], time.Minute)
	request := service.UserLeaseRequest{
		UserID:        1_110,
		StandardLimit: 1,
		ExtraLimit:    1,
		MaxWaiting:    20,
		WaitTimeout:   time.Minute,
	}

	request.RequestID = "gateway-active-standard"
	standard, err := store.TryAcquireUserLease(t.Context(), request)
	require.NoError(t, err)
	require.True(t, standard.Acquired)
	require.Equal(t, service.AdmissionClassStandard, standard.Class)

	request.RequestID = "gateway-active-extra"
	extra, err := store.TryAcquireUserLease(t.Context(), request)
	require.NoError(t, err)
	require.True(t, extra.Acquired)
	require.Equal(t, service.AdmissionClassExtra, extra.Class)

	request.RequestID = "gateway-stale-queued-extra"
	queued, err := store.TryAcquireUserLease(t.Context(), request)
	require.NoError(t, err)
	require.False(t, queued.Acquired)
	require.False(t, queued.Draining)

	require.NoError(t, notifier.PublishExtraConcurrencySettingsState(t.Context(), false))
	require.NoError(t, store.ReleaseUserLease(t.Context(), request.UserID, "gateway-active-extra"))

	queued, err = store.TryAcquireUserLease(t.Context(), request)
	require.NoError(t, err)
	require.False(t, queued.Acquired, "a stale queued extra request must not consume released extra capacity while draining")
	require.False(t, queued.Draining, "an existing waiter must remain in the gateway queue for standard capacity")

	require.NoError(t, store.ReleaseUserLease(t.Context(), request.UserID, "gateway-active-standard"))
	queued, err = store.TryAcquireUserLease(t.Context(), request)
	require.NoError(t, err)
	require.True(t, queued.Acquired)
	require.Equal(t, service.AdmissionClassStandard, queued.Class)
}

func TestExtraConcurrencyDisableDrainRequeuesStaleExtraBeforeTargetAdmission(t *testing.T) {
	clients := testRedisClients(t, 2)
	notifier := NewExtraConcurrencySettingsNotifier(clients[0])
	store := NewGatewayAdmissionStore(clients[1], time.Minute)
	const (
		userID    int64 = 1_112
		accountID int64 = 1_302
	)

	blockerRequest := service.UserLeaseRequest{
		RequestID:     "gateway-target-standard-blocker",
		UserID:        userID,
		StandardLimit: 1,
		ExtraLimit:    1,
		MaxWaiting:    20,
		WaitTimeout:   3 * time.Second,
	}
	blocker, err := store.TryAcquireUserLease(t.Context(), blockerRequest)
	require.NoError(t, err)
	require.True(t, blocker.Acquired)
	require.Equal(t, service.AdmissionClassStandard, blocker.Class)
	t.Cleanup(func() {
		_ = store.ReleaseUserLease(context.Background(), userID, blockerRequest.RequestID)
	})

	admission := service.NewGatewayAdmission(
		store,
		nil,
		gatewayAdmissionIntegrationCapacity{accountID: accountID},
	)
	session, err := admission.Begin(t.Context(), service.GatewayAdmissionRequest{
		UserID:        userID,
		StandardLimit: 1,
		ExtraLimit:    1,
		Settings: service.ExtraConcurrencyRuntimeSettings{
			Enabled:            true,
			WaitTimeoutSeconds: 3,
		},
	})
	require.NoError(t, err)
	t.Cleanup(session.Close)
	require.Equal(t, service.AdmissionClassExtra, session.Class())

	require.NoError(t, notifier.PublishExtraConcurrencySettingsState(t.Context(), false))
	account := &service.Account{
		ID:          accountID,
		Platform:    service.PlatformAnthropic,
		Concurrency: 1,
	}
	targetResult := make(chan *service.AdmittedTarget, 1)
	targetError := make(chan error, 1)
	go func() {
		target, targetErr := session.NextTarget(t.Context(), service.GatewayTargetRequest{
			Selector: service.GatewayTargetSelectorFunc(func(ctx context.Context, claimer service.TargetClaimer) (*service.AccountSelectionResult, error) {
				release, acquired, claimErr := claimer.TryClaim(ctx, service.TargetClaimRequest{
					Platform:           account.Platform,
					AccountID:          account.ID,
					AccountConcurrency: account.Concurrency,
				})
				return &service.AccountSelectionResult{
					Account:     account,
					Acquired:    acquired,
					ReleaseFunc: release,
				}, claimErr
			}),
		})
		if targetErr != nil {
			targetError <- targetErr
			return
		}
		targetResult <- target
	}()

	select {
	case target := <-targetResult:
		t.Fatalf("stale request acquired %s target admission while the global drain was active", target.Class)
	case err := <-targetError:
		t.Fatalf("target admission failed before standard concurrency became available: %v", err)
	case <-time.After(150 * time.Millisecond):
	}

	require.NoError(t, store.ReleaseUserLease(t.Context(), userID, blockerRequest.RequestID))
	select {
	case target := <-targetResult:
		require.Equal(t, service.AdmissionClassStandard, target.Class)
		require.Equal(t, service.AdmissionClassStandard, session.Class())
	case err := <-targetError:
		t.Fatalf("target admission failed while requeued for standard concurrency: %v", err)
	case <-time.After(2 * time.Second):
		t.Fatal("requeued request did not acquire a standard target")
	}
}

func TestExtraConcurrencyDisableDrainRequeuesStaleUndispatchedExtraAtDispatchBoundary(t *testing.T) {
	clients := testRedisClients(t, 2)
	notifier := NewExtraConcurrencySettingsNotifier(clients[0])
	store := NewGatewayAdmissionStore(clients[1], time.Minute)
	const (
		userID    int64 = 1_111
		accountID int64 = 1_301
	)

	blockerRequest := service.UserLeaseRequest{
		RequestID:     "gateway-dispatch-standard-blocker",
		UserID:        userID,
		StandardLimit: 1,
		ExtraLimit:    1,
		MaxWaiting:    20,
		WaitTimeout:   3 * time.Second,
	}
	blocker, err := store.TryAcquireUserLease(t.Context(), blockerRequest)
	require.NoError(t, err)
	require.True(t, blocker.Acquired)
	require.Equal(t, service.AdmissionClassStandard, blocker.Class)
	t.Cleanup(func() {
		_ = store.ReleaseUserLease(context.Background(), userID, blockerRequest.RequestID)
	})

	admission := service.NewGatewayAdmission(
		store,
		nil,
		gatewayAdmissionIntegrationCapacity{accountID: accountID},
	)
	session, err := admission.Begin(t.Context(), service.GatewayAdmissionRequest{
		UserID:        userID,
		StandardLimit: 1,
		ExtraLimit:    1,
		Settings: service.ExtraConcurrencyRuntimeSettings{
			Enabled:            true,
			WaitTimeoutSeconds: 3,
		},
	})
	require.NoError(t, err)
	t.Cleanup(session.Close)
	require.Equal(t, service.AdmissionClassExtra, session.Class())

	account := &service.Account{
		ID:          accountID,
		Platform:    service.PlatformAnthropic,
		Concurrency: 1,
	}
	target, err := session.NextTarget(t.Context(), service.GatewayTargetRequest{
		Selector: service.GatewayTargetSelectorFunc(func(ctx context.Context, claimer service.TargetClaimer) (*service.AccountSelectionResult, error) {
			release, acquired, claimErr := claimer.TryClaim(ctx, service.TargetClaimRequest{
				Platform:           account.Platform,
				AccountID:          account.ID,
				AccountConcurrency: account.Concurrency,
			})
			return &service.AccountSelectionResult{
				Account:     account,
				Acquired:    acquired,
				ReleaseFunc: release,
			}, claimErr
		}),
	})
	require.NoError(t, err)
	require.Equal(t, service.AdmissionClassExtra, target.Class)

	require.NoError(t, notifier.PublishExtraConcurrencySettingsState(t.Context(), false))
	upstreamStarted := make(chan struct{}, 1)
	dispatchDone := make(chan error, 1)
	dispatchCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	go func() {
		dispatchDone <- target.Dispatch(dispatchCtx, nil, func(context.Context, *service.Account) error {
			upstreamStarted <- struct{}{}
			return nil
		})
	}()

	select {
	case <-upstreamStarted:
		t.Fatal("stale undispatched extra request reached upstream after the global drain")
	case err := <-dispatchDone:
		t.Fatalf("dispatch returned before standard concurrency became available: %v", err)
	case <-time.After(150 * time.Millisecond):
	}

	require.NoError(t, store.ReleaseUserLease(t.Context(), userID, blockerRequest.RequestID))
	select {
	case <-upstreamStarted:
	case err := <-dispatchDone:
		t.Fatalf("dispatch failed while requeued for standard concurrency: %v", err)
	case <-time.After(2 * time.Second):
		t.Fatal("requeued request did not dispatch after standard concurrency became available")
	}
	require.NoError(t, <-dispatchDone)
	require.Equal(t, service.AdmissionClassStandard, session.Class())
	require.Equal(t, service.AdmissionClassStandard, target.Class)
}

func TestExtraConcurrencyDisableDrainAllowsStartedExtraTargetDispatchToFinish(t *testing.T) {
	clients := testRedisClients(t, 2)
	notifier := NewExtraConcurrencySettingsNotifier(clients[0])
	store := NewGatewayAdmissionStore(clients[1], time.Minute)
	request := service.TargetLeaseRequest{
		RequestID:        "gateway-started-extra-dispatch",
		Platform:         service.PlatformAnthropic,
		AccountID:        1_303,
		AccountLimit:     1,
		PlatformCapacity: 1,
		Class:            service.AdmissionClassExtra,
		WaitTimeout:      time.Minute,
	}

	lease, err := store.TryAcquireTargetLease(t.Context(), request)
	require.NoError(t, err)
	require.True(t, lease.Acquired)

	started, err := store.BeginTargetDispatch(t.Context(), service.TargetDispatchRequest{
		RequestID: request.RequestID,
		Platform:  request.Platform,
		AccountID: request.AccountID,
		Class:     request.Class,
	})
	require.NoError(t, err)
	require.True(t, started.Started)
	require.False(t, started.Draining)

	require.NoError(t, notifier.PublishExtraConcurrencySettingsState(t.Context(), false))
	renewed, err := store.RenewTargetLease(
		t.Context(),
		request.Platform,
		request.AccountID,
		request.RequestID,
	)
	require.NoError(t, err)
	require.True(t, renewed, "a target dispatch ordered before the drain must remain renewable while in flight")
}

func TestExtraConcurrencyUnlimitedTargetCanBeginDispatchWithoutLease(t *testing.T) {
	store := NewGatewayAdmissionStore(testRedis(t), time.Minute)
	request := service.TargetLeaseRequest{
		RequestID: "gateway-unlimited-extra-dispatch",
		Platform:  service.PlatformAnthropic,
		AccountID: 1_304,
		Class:     service.AdmissionClassExtra,
		Unlimited: true,
	}

	lease, err := store.TryAcquireTargetLease(t.Context(), request)
	require.NoError(t, err)
	require.True(t, lease.Acquired)

	started, err := store.BeginTargetDispatch(t.Context(), service.TargetDispatchRequest{
		RequestID: request.RequestID,
		Platform:  request.Platform,
		AccountID: request.AccountID,
		Class:     request.Class,
		Unlimited: true,
	})
	require.NoError(t, err)
	require.True(t, started.Started)
	require.False(t, started.Draining)
}

func TestExtraConcurrencyEnableClearsAdmissionDrain(t *testing.T) {
	clients := testRedisClients(t, 2)
	notifier := NewExtraConcurrencySettingsNotifier(clients[0])
	store := NewGatewayAdmissionStore(clients[1], time.Minute)
	request := service.UserLeaseRequest{
		RequestID:     "gateway-after-reenable",
		UserID:        1_109,
		StandardLimit: 1,
	}

	require.NoError(t, notifier.PublishExtraConcurrencySettingsState(t.Context(), false))
	blocked, err := store.TryAcquireUserLease(t.Context(), request)
	require.NoError(t, err)
	require.False(t, blocked.Acquired)
	require.True(t, blocked.Draining)

	require.NoError(t, notifier.PublishExtraConcurrencySettingsState(t.Context(), true))
	admitted, err := store.TryAcquireUserLease(t.Context(), request)
	require.NoError(t, err)
	require.True(t, admitted.Acquired)
	require.False(t, admitted.Draining)
}

func mustAcquireLegacyUserSlot(
	t *testing.T,
	cache service.ConcurrencyCache,
	ctx context.Context,
	userID int64,
	maxConcurrency int,
	requestID string,
) bool {
	t.Helper()
	acquired, err := cache.AcquireUserSlot(ctx, userID, maxConcurrency, requestID)
	require.NoError(t, err)
	return acquired
}
