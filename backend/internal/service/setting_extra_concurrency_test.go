//go:build unit

package service

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/stretchr/testify/require"
)

type extraConcurrencySettingRepoStub struct {
	values       map[string]string
	getAllErr    error
	getMultiErr  error
	setMultiErr  error
	getMultiCall int
	updateFence  int64
}

type blockingExtraConcurrencySettingRepo struct {
	*extraConcurrencySettingRepoStub
	mu      sync.Mutex
	calls   int
	started chan struct{}
	release chan struct{}
}

type reorderingExtraConcurrencySettingRepo struct {
	*extraConcurrencySettingRepoStub
	mu              sync.Mutex
	calls           int
	firstPersisted  chan struct{}
	secondPersisted chan struct{}
	releaseFirst    chan struct{}
	persisted       bool
}

func (r *reorderingExtraConcurrencySettingRepo) SetMultiple(_ context.Context, settings map[string]string) error {
	r.mu.Lock()
	r.calls++
	call := r.calls
	r.persisted = settings[SettingKeyExtraConcurrencyEnabled] == "true"
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

func (r *reorderingExtraConcurrencySettingRepo) SetMultipleFenced(ctx context.Context, settings map[string]string, fence int64) error {
	if fence != r.updateFence {
		return ErrStaleSettingUpdateFence
	}
	return r.SetMultiple(ctx, settings)
}

func (r *reorderingExtraConcurrencySettingRepo) persistedEnabled() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.persisted
}

type recordingExtraConcurrencySettingsNotifier struct {
	mu        sync.Mutex
	states    []bool
	published chan bool
}

func (n *recordingExtraConcurrencySettingsNotifier) PublishExtraConcurrencySettingsState(_ context.Context, enabled bool) error {
	n.mu.Lock()
	n.states = append(n.states, enabled)
	n.mu.Unlock()
	n.published <- enabled
	return nil
}

func (n *recordingExtraConcurrencySettingsNotifier) SerializeExtraConcurrencySettingsUpdate(
	ctx context.Context,
	enabled bool,
	reserveFence func(context.Context) (int64, error),
	update func(context.Context, int64) error,
) error {
	fence, err := reserveFence(ctx)
	if err != nil {
		return err
	}
	if err := update(ctx, fence); err != nil {
		return err
	}
	return n.PublishExtraConcurrencySettingsState(ctx, enabled)
}

func (n *recordingExtraConcurrencySettingsNotifier) SubscribeExtraConcurrencySettingsInvalidation(context.Context, func()) error {
	return nil
}

func (n *recordingExtraConcurrencySettingsNotifier) completed() []bool {
	n.mu.Lock()
	defer n.mu.Unlock()
	return append([]bool(nil), n.states...)
}

func (s *blockingExtraConcurrencySettingRepo) GetMultiple(_ context.Context, keys []string) (map[string]string, error) {
	s.mu.Lock()
	s.calls++
	first := s.calls == 1
	out := make(map[string]string, len(keys))
	for _, key := range keys {
		if value, ok := s.values[key]; ok {
			out[key] = value
		}
	}
	s.mu.Unlock()

	if first {
		close(s.started)
		<-s.release
	}
	return out, nil
}

func (s *extraConcurrencySettingRepoStub) Get(context.Context, string) (*Setting, error) {
	panic("unexpected Get call")
}

func (s *extraConcurrencySettingRepoStub) GetValue(_ context.Context, key string) (string, error) {
	if value, ok := s.values[key]; ok {
		return value, nil
	}
	return "", ErrSettingNotFound
}

func (s *extraConcurrencySettingRepoStub) Set(context.Context, string, string) error {
	panic("unexpected Set call")
}

func (s *extraConcurrencySettingRepoStub) GetMultiple(_ context.Context, keys []string) (map[string]string, error) {
	s.getMultiCall++
	if s.getMultiErr != nil {
		return nil, s.getMultiErr
	}
	out := make(map[string]string, len(keys))
	for _, key := range keys {
		if value, ok := s.values[key]; ok {
			out[key] = value
		}
	}
	return out, nil
}

func (s *extraConcurrencySettingRepoStub) SetMultiple(_ context.Context, settings map[string]string) error {
	if s.setMultiErr != nil {
		return s.setMultiErr
	}
	if s.values == nil {
		s.values = map[string]string{}
	}
	for key, value := range settings {
		s.values[key] = value
	}
	return nil
}

func (s *extraConcurrencySettingRepoStub) ReserveSettingUpdateFence(context.Context) (int64, error) {
	s.updateFence++
	return s.updateFence, nil
}

func (s *extraConcurrencySettingRepoStub) SetMultipleFenced(ctx context.Context, settings map[string]string, fence int64) error {
	if fence != s.updateFence {
		return ErrStaleSettingUpdateFence
	}
	return s.SetMultiple(ctx, settings)
}

func (s *extraConcurrencySettingRepoStub) GetAll(context.Context) (map[string]string, error) {
	if s.getAllErr != nil {
		return nil, s.getAllErr
	}
	out := make(map[string]string, len(s.values))
	for key, value := range s.values {
		out[key] = value
	}
	return out, nil
}

func (s *extraConcurrencySettingRepoStub) Delete(context.Context, string) error {
	panic("unexpected Delete call")
}

func TestSettingServiceGetAllSettingsUsesExtraConcurrencyDefaults(t *testing.T) {
	svc := NewSettingService(&extraConcurrencySettingRepoStub{}, &config.Config{})

	got, err := svc.GetAllSettings(context.Background())

	require.NoError(t, err)
	require.Zero(t, got.DefaultExtraConcurrency)
	require.False(t, got.ExtraConcurrencyEnabled)
	require.Equal(t, 30, got.ExtraConcurrencyWaitTimeoutSeconds)
	require.Equal(t, 10.0, got.ExtraConcurrencyReservePercent)
	require.Equal(t, 1, got.ExtraConcurrencyMinReservedSlots)
	require.Empty(t, got.ExtraConcurrencyPlatformReserves)
}

func TestSettingServiceGetAllSettingsParsesExtraConcurrencySettings(t *testing.T) {
	svc := NewSettingService(&extraConcurrencySettingRepoStub{values: map[string]string{
		SettingKeyDefaultExtraConcurrency:            "4",
		SettingKeyExtraConcurrencyEnabled:            "true",
		SettingKeyExtraConcurrencyWaitTimeoutSeconds: "45",
		SettingKeyExtraConcurrencyReservePercent:     "25.5",
		SettingKeyExtraConcurrencyMinReservedSlots:   "3",
		SettingKeyExtraConcurrencyPlatformReserves:   `{"openai":{"reserve_percent":20,"min_reserved_slots":2}}`,
	}}, &config.Config{})

	got, err := svc.GetAllSettings(context.Background())

	require.NoError(t, err)
	require.Equal(t, 4, got.DefaultExtraConcurrency)
	require.True(t, got.ExtraConcurrencyEnabled)
	require.Equal(t, 45, got.ExtraConcurrencyWaitTimeoutSeconds)
	require.Equal(t, 25.5, got.ExtraConcurrencyReservePercent)
	require.Equal(t, 3, got.ExtraConcurrencyMinReservedSlots)
	require.Contains(t, got.ExtraConcurrencyPlatformReserves, "openai")
	openAIReserve := got.ExtraConcurrencyPlatformReserves["openai"]
	require.NotNil(t, openAIReserve.ReservePercent)
	require.Equal(t, 20.0, *openAIReserve.ReservePercent)
	require.NotNil(t, openAIReserve.MinReservedSlots)
	require.Equal(t, 2, *openAIReserve.MinReservedSlots)
}

func TestSettingServiceUpdateSettingsPersistsExtraConcurrencySettings(t *testing.T) {
	repo := &extraConcurrencySettingRepoStub{}
	svc := NewSettingService(repo, &config.Config{})
	settings, err := svc.GetAllSettings(context.Background())
	require.NoError(t, err)
	reservePercent := 20.0
	minReservedSlots := 2
	settings.DefaultExtraConcurrency = 4
	settings.ExtraConcurrencyEnabled = true
	settings.ExtraConcurrencyWaitTimeoutSeconds = 45
	settings.ExtraConcurrencyReservePercent = 25.5
	settings.ExtraConcurrencyMinReservedSlots = 3
	settings.ExtraConcurrencyPlatformReserves = map[string]ExtraConcurrencyPlatformReserve{
		"openai": {
			ReservePercent:   &reservePercent,
			MinReservedSlots: &minReservedSlots,
		},
	}

	require.NoError(t, svc.UpdateSettings(context.Background(), settings))
	got, err := svc.GetAllSettings(context.Background())

	require.NoError(t, err)
	require.Equal(t, 4, got.DefaultExtraConcurrency)
	require.True(t, got.ExtraConcurrencyEnabled)
	require.Equal(t, 45, got.ExtraConcurrencyWaitTimeoutSeconds)
	require.Equal(t, 25.5, got.ExtraConcurrencyReservePercent)
	require.Equal(t, 3, got.ExtraConcurrencyMinReservedSlots)
	require.Contains(t, got.ExtraConcurrencyPlatformReserves, "openai")
	openAIReserve := got.ExtraConcurrencyPlatformReserves["openai"]
	require.NotNil(t, openAIReserve.ReservePercent)
	require.Equal(t, 20.0, *openAIReserve.ReservePercent)
	require.NotNil(t, openAIReserve.MinReservedSlots)
	require.Equal(t, 2, *openAIReserve.MinReservedSlots)
}

func TestSettingServiceUpdateSettingsRejectsNegativeDefaultExtraConcurrency(t *testing.T) {
	repo := &extraConcurrencySettingRepoStub{}
	svc := NewSettingService(repo, &config.Config{})
	settings, err := svc.GetAllSettings(context.Background())
	require.NoError(t, err)
	settings.DefaultExtraConcurrency = -1

	err = svc.UpdateSettings(context.Background(), settings)

	require.Equal(t, "INVALID_EXTRA_CONCURRENCY_SETTINGS", infraerrors.Reason(err))
	require.Nil(t, repo.values)
}

func TestSettingServiceUpdateSettingsRejectsExtraConcurrencyTimeoutOutsideRange(t *testing.T) {
	for _, timeoutSeconds := range []int{0, 301} {
		t.Run(fmt.Sprintf("timeout_%d", timeoutSeconds), func(t *testing.T) {
			repo := &extraConcurrencySettingRepoStub{}
			svc := NewSettingService(repo, &config.Config{})
			settings, err := svc.GetAllSettings(context.Background())
			require.NoError(t, err)
			settings.ExtraConcurrencyWaitTimeoutSeconds = timeoutSeconds

			err = svc.UpdateSettings(context.Background(), settings)

			require.Equal(t, "INVALID_EXTRA_CONCURRENCY_SETTINGS", infraerrors.Reason(err))
			require.Nil(t, repo.values)
		})
	}
}

func TestSettingServiceUpdateSettingsRejectsExtraConcurrencyReservePercentOutsideRange(t *testing.T) {
	for _, reservePercent := range []float64{-0.1, 100.1} {
		t.Run(fmt.Sprintf("percent_%g", reservePercent), func(t *testing.T) {
			repo := &extraConcurrencySettingRepoStub{}
			svc := NewSettingService(repo, &config.Config{})
			settings, err := svc.GetAllSettings(context.Background())
			require.NoError(t, err)
			settings.ExtraConcurrencyReservePercent = reservePercent

			err = svc.UpdateSettings(context.Background(), settings)

			require.Equal(t, "INVALID_EXTRA_CONCURRENCY_SETTINGS", infraerrors.Reason(err))
			require.Nil(t, repo.values)
		})
	}
}

func TestSettingServiceUpdateSettingsRejectsNegativeExtraConcurrencyMinReservedSlots(t *testing.T) {
	repo := &extraConcurrencySettingRepoStub{}
	svc := NewSettingService(repo, &config.Config{})
	settings, err := svc.GetAllSettings(context.Background())
	require.NoError(t, err)
	settings.ExtraConcurrencyMinReservedSlots = -1

	err = svc.UpdateSettings(context.Background(), settings)

	require.Equal(t, "INVALID_EXTRA_CONCURRENCY_SETTINGS", infraerrors.Reason(err))
	require.Nil(t, repo.values)
}

func TestSettingServiceUpdateSettingsRejectsUnknownExtraConcurrencyPlatform(t *testing.T) {
	repo := &extraConcurrencySettingRepoStub{}
	svc := NewSettingService(repo, &config.Config{})
	settings, err := svc.GetAllSettings(context.Background())
	require.NoError(t, err)
	settings.ExtraConcurrencyPlatformReserves = map[string]ExtraConcurrencyPlatformReserve{
		"sora": {},
	}

	err = svc.UpdateSettings(context.Background(), settings)

	require.Equal(t, "INVALID_EXTRA_CONCURRENCY_SETTINGS", infraerrors.Reason(err))
	require.Nil(t, repo.values)
}

func TestSettingServiceUpdateSettingsRejectsInvalidExtraConcurrencyPlatformReserve(t *testing.T) {
	invalidPercent := 100.1
	negativeSlots := -1
	cases := map[string]ExtraConcurrencyPlatformReserve{
		"reserve_percent":    {ReservePercent: &invalidPercent},
		"min_reserved_slots": {MinReservedSlots: &negativeSlots},
	}
	for name, reserve := range cases {
		t.Run(name, func(t *testing.T) {
			repo := &extraConcurrencySettingRepoStub{}
			svc := NewSettingService(repo, &config.Config{})
			settings, err := svc.GetAllSettings(context.Background())
			require.NoError(t, err)
			settings.ExtraConcurrencyPlatformReserves = map[string]ExtraConcurrencyPlatformReserve{
				"openai": reserve,
			}

			err = svc.UpdateSettings(context.Background(), settings)

			require.Equal(t, "INVALID_EXTRA_CONCURRENCY_SETTINGS", infraerrors.Reason(err))
			require.Nil(t, repo.values)
		})
	}
}

func TestSettingServiceUpdateSettingsPreservesExplicitZeroExtraConcurrencyReserves(t *testing.T) {
	repo := &extraConcurrencySettingRepoStub{}
	svc := NewSettingService(repo, &config.Config{})
	settings, err := svc.GetAllSettings(context.Background())
	require.NoError(t, err)
	zeroPercent := 0.0
	zeroSlots := 0
	settings.ExtraConcurrencyReservePercent = 0
	settings.ExtraConcurrencyMinReservedSlots = 0
	settings.ExtraConcurrencyPlatformReserves = map[string]ExtraConcurrencyPlatformReserve{
		"openai": {
			ReservePercent:   &zeroPercent,
			MinReservedSlots: &zeroSlots,
		},
	}

	require.NoError(t, svc.UpdateSettings(context.Background(), settings))
	got, err := svc.GetAllSettings(context.Background())

	require.NoError(t, err)
	require.Zero(t, got.ExtraConcurrencyReservePercent)
	require.Zero(t, got.ExtraConcurrencyMinReservedSlots)
	openAIReserve := got.ExtraConcurrencyPlatformReserves["openai"]
	require.NotNil(t, openAIReserve.ReservePercent)
	require.Zero(t, *openAIReserve.ReservePercent)
	require.NotNil(t, openAIReserve.MinReservedSlots)
	require.Zero(t, *openAIReserve.MinReservedSlots)
}

func TestSettingServiceGetExtraConcurrencyRuntimeSettingsLoadsValidSnapshot(t *testing.T) {
	repo := &extraConcurrencySettingRepoStub{values: map[string]string{
		SettingKeyExtraConcurrencyEnabled:            "true",
		SettingKeyExtraConcurrencyWaitTimeoutSeconds: "45",
		SettingKeyExtraConcurrencyReservePercent:     "25.5",
		SettingKeyExtraConcurrencyMinReservedSlots:   "3",
		SettingKeyExtraConcurrencyPlatformReserves:   `{"openai":{"reserve_percent":20,"min_reserved_slots":2}}`,
	}}
	svc := NewSettingService(repo, &config.Config{})

	got := svc.GetExtraConcurrencyRuntimeSettings(context.Background())

	require.True(t, got.Enabled)
	require.Equal(t, 45, got.WaitTimeoutSeconds)
	require.Equal(t, 25.5, got.ReservePercent)
	require.Equal(t, 3, got.MinReservedSlots)
	openAIReserve, ok := got.PlatformReserves["openai"]
	require.True(t, ok)
	require.NotNil(t, openAIReserve.ReservePercent)
	require.Equal(t, 20.0, *openAIReserve.ReservePercent)
	require.NotNil(t, openAIReserve.MinReservedSlots)
	require.Equal(t, 2, *openAIReserve.MinReservedSlots)
}

func TestSettingServiceGetExtraConcurrencyRuntimeSettingsUsesProcessCache(t *testing.T) {
	repo := &extraConcurrencySettingRepoStub{values: map[string]string{
		SettingKeyExtraConcurrencyEnabled:            "true",
		SettingKeyExtraConcurrencyWaitTimeoutSeconds: "45",
		SettingKeyExtraConcurrencyReservePercent:     "25.5",
		SettingKeyExtraConcurrencyMinReservedSlots:   "3",
		SettingKeyExtraConcurrencyPlatformReserves:   `{}`,
	}}
	svc := NewSettingService(repo, &config.Config{})

	first := svc.GetExtraConcurrencyRuntimeSettings(context.Background())
	repo.values[SettingKeyExtraConcurrencyEnabled] = "false"
	second := svc.GetExtraConcurrencyRuntimeSettings(context.Background())

	require.True(t, first.Enabled)
	require.True(t, second.Enabled)
	require.Equal(t, 1, repo.getMultiCall)
}

func TestSettingServiceConcurrentUpdatesPublishPersistedDrainStateLast(t *testing.T) {
	repo := &reorderingExtraConcurrencySettingRepo{
		extraConcurrencySettingRepoStub: &extraConcurrencySettingRepoStub{},
		firstPersisted:                  make(chan struct{}),
		secondPersisted:                 make(chan struct{}),
		releaseFirst:                    make(chan struct{}),
	}
	notifier := &recordingExtraConcurrencySettingsNotifier{published: make(chan bool, 2)}
	svc := NewSettingService(repo, &config.Config{})
	svc.SetExtraConcurrencySettingsNotifier(notifier)

	firstDone := make(chan error, 1)
	go func() {
		firstDone <- svc.UpdateSettings(context.Background(), concurrentExtraConcurrencySettings(false))
	}()
	<-repo.firstPersisted

	secondDone := make(chan error, 1)
	go func() {
		secondDone <- svc.UpdateSettings(context.Background(), concurrentExtraConcurrencySettings(true))
	}()

	select {
	case <-repo.secondPersisted:
		select {
		case state := <-notifier.published:
			require.True(t, state, "the newer persisted setting should publish first in the forced race")
		case <-time.After(time.Second):
			t.Fatal("newer settings update did not publish before the delayed older refresh")
		}
	case <-time.After(100 * time.Millisecond):
	}
	close(repo.releaseFirst)

	select {
	case err := <-firstDone:
		require.NoError(t, err)
	case <-time.After(time.Second):
		t.Fatal("first settings refresh did not finish")
	}
	select {
	case err := <-secondDone:
		require.NoError(t, err)
	case <-time.After(time.Second):
		t.Fatal("second settings refresh did not finish")
	}
	require.True(t, repo.persistedEnabled())
	require.Equal(t, []bool{false, true}, notifier.completed(),
		"drain publishes must follow the same order as the serialized settings writes")
}

func concurrentExtraConcurrencySettings(enabled bool) *SystemSettings {
	return &SystemSettings{
		ExtraConcurrencyEnabled:            enabled,
		ExtraConcurrencyWaitTimeoutSeconds: 30,
		ExtraConcurrencyReservePercent:     10,
		ExtraConcurrencyMinReservedSlots:   1,
		ExtraConcurrencyPlatformReserves:   map[string]ExtraConcurrencyPlatformReserve{},
	}
}

func TestSettingServiceGetExtraConcurrencyRuntimeSettingsRefreshesAfterTTL(t *testing.T) {
	repo := &extraConcurrencySettingRepoStub{values: map[string]string{
		SettingKeyExtraConcurrencyEnabled:            "true",
		SettingKeyExtraConcurrencyWaitTimeoutSeconds: "45",
		SettingKeyExtraConcurrencyReservePercent:     "25.5",
		SettingKeyExtraConcurrencyMinReservedSlots:   "3",
		SettingKeyExtraConcurrencyPlatformReserves:   `{}`,
	}}
	now := time.Unix(1_700_000_000, 0)
	svc := NewSettingService(repo, &config.Config{})
	svc.extraConcurrencyRuntimeNow = func() time.Time { return now }

	first := svc.GetExtraConcurrencyRuntimeSettings(context.Background())
	repo.values[SettingKeyExtraConcurrencyEnabled] = "false"
	now = now.Add(9 * time.Second)
	withinTTL := svc.GetExtraConcurrencyRuntimeSettings(context.Background())
	now = now.Add(2 * time.Second)
	afterTTL := svc.GetExtraConcurrencyRuntimeSettings(context.Background())

	require.True(t, first.Enabled)
	require.True(t, withinTTL.Enabled)
	require.False(t, afterTTL.Enabled)
	require.Equal(t, 2, repo.getMultiCall)
}

func TestSettingServiceExtraConcurrencyInvalidationRejectsStaleInflightLoad(t *testing.T) {
	repo := &blockingExtraConcurrencySettingRepo{
		extraConcurrencySettingRepoStub: &extraConcurrencySettingRepoStub{values: map[string]string{
			SettingKeyExtraConcurrencyEnabled:            "true",
			SettingKeyExtraConcurrencyWaitTimeoutSeconds: "45",
			SettingKeyExtraConcurrencyReservePercent:     "25.5",
			SettingKeyExtraConcurrencyMinReservedSlots:   "3",
			SettingKeyExtraConcurrencyPlatformReserves:   `{}`,
		}},
		started: make(chan struct{}),
		release: make(chan struct{}),
	}
	svc := NewSettingService(repo, &config.Config{})
	firstResult := make(chan ExtraConcurrencyRuntimeSettings, 1)
	go func() {
		firstResult <- svc.GetExtraConcurrencyRuntimeSettings(context.Background())
	}()
	<-repo.started

	repo.values[SettingKeyExtraConcurrencyEnabled] = "false"
	svc.InvalidateExtraConcurrencyRuntimeSettings()
	second := svc.GetExtraConcurrencyRuntimeSettings(context.Background())
	close(repo.release)
	first := <-firstResult
	third := svc.GetExtraConcurrencyRuntimeSettings(context.Background())

	require.False(t, first.Enabled)
	require.False(t, second.Enabled)
	require.False(t, third.Enabled)
	require.Equal(t, 2, repo.calls)
}

func TestSettingServiceGetExtraConcurrencyRuntimeSettingsDisablesWhenKeyIsMissing(t *testing.T) {
	repo := &extraConcurrencySettingRepoStub{values: map[string]string{
		SettingKeyExtraConcurrencyEnabled:            "true",
		SettingKeyExtraConcurrencyWaitTimeoutSeconds: "45",
		SettingKeyExtraConcurrencyReservePercent:     "25.5",
		SettingKeyExtraConcurrencyMinReservedSlots:   "3",
	}}
	svc := NewSettingService(repo, &config.Config{})

	got := svc.GetExtraConcurrencyRuntimeSettings(context.Background())

	require.False(t, got.Enabled)
	require.Equal(t, 30, got.WaitTimeoutSeconds)
	require.Equal(t, 10.0, got.ReservePercent)
	require.Equal(t, 1, got.MinReservedSlots)
	require.Empty(t, got.PlatformReserves)
}

func TestSettingServiceGetExtraConcurrencyRuntimeSettingsDisablesWhenValueIsMalformed(t *testing.T) {
	validValues := map[string]string{
		SettingKeyExtraConcurrencyEnabled:            "true",
		SettingKeyExtraConcurrencyWaitTimeoutSeconds: "45",
		SettingKeyExtraConcurrencyReservePercent:     "25.5",
		SettingKeyExtraConcurrencyMinReservedSlots:   "3",
		SettingKeyExtraConcurrencyPlatformReserves:   `{}`,
	}
	malformed := map[string]string{
		SettingKeyExtraConcurrencyEnabled:            "not-a-bool",
		SettingKeyExtraConcurrencyWaitTimeoutSeconds: "not-an-int",
		SettingKeyExtraConcurrencyReservePercent:     "not-a-number",
		SettingKeyExtraConcurrencyMinReservedSlots:   "not-an-int",
		SettingKeyExtraConcurrencyPlatformReserves:   `{`,
	}
	for key, value := range malformed {
		t.Run(key, func(t *testing.T) {
			values := make(map[string]string, len(validValues))
			for validKey, validValue := range validValues {
				values[validKey] = validValue
			}
			values[key] = value
			svc := NewSettingService(&extraConcurrencySettingRepoStub{values: values}, &config.Config{})

			got := svc.GetExtraConcurrencyRuntimeSettings(context.Background())

			require.False(t, got.Enabled)
		})
	}
}

func TestSettingServiceGetExtraConcurrencyRuntimeSettingsDisablesWhenRepositoryFails(t *testing.T) {
	repo := &extraConcurrencySettingRepoStub{getMultiErr: fmt.Errorf("repository unavailable")}
	svc := NewSettingService(repo, &config.Config{})

	got := svc.GetExtraConcurrencyRuntimeSettings(context.Background())

	require.False(t, got.Enabled)
	require.Equal(t, 30, got.WaitTimeoutSeconds)
	require.Equal(t, 1, repo.getMultiCall)
}

func TestSettingServiceGetExtraConcurrencyRuntimeSettingsReturnsClonedPlatformReserves(t *testing.T) {
	repo := &extraConcurrencySettingRepoStub{values: map[string]string{
		SettingKeyExtraConcurrencyEnabled:            "true",
		SettingKeyExtraConcurrencyWaitTimeoutSeconds: "45",
		SettingKeyExtraConcurrencyReservePercent:     "25.5",
		SettingKeyExtraConcurrencyMinReservedSlots:   "3",
		SettingKeyExtraConcurrencyPlatformReserves:   `{"openai":{"reserve_percent":20,"min_reserved_slots":2}}`,
	}}
	svc := NewSettingService(repo, &config.Config{})

	first := svc.GetExtraConcurrencyRuntimeSettings(context.Background())
	*first.PlatformReserves["openai"].ReservePercent = 99
	first.PlatformReserves["grok"] = ExtraConcurrencyPlatformReserve{}
	second := svc.GetExtraConcurrencyRuntimeSettings(context.Background())

	require.Equal(t, 20.0, *second.PlatformReserves["openai"].ReservePercent)
	require.NotContains(t, second.PlatformReserves, "grok")
}

func TestSettingServiceUpdateSettingsImmediatelyRefreshesExtraConcurrencyRuntimeCache(t *testing.T) {
	repo := &extraConcurrencySettingRepoStub{values: map[string]string{
		SettingKeyExtraConcurrencyEnabled:            "true",
		SettingKeyExtraConcurrencyWaitTimeoutSeconds: "45",
		SettingKeyExtraConcurrencyReservePercent:     "25.5",
		SettingKeyExtraConcurrencyMinReservedSlots:   "3",
		SettingKeyExtraConcurrencyPlatformReserves:   `{}`,
	}}
	svc := NewSettingService(repo, &config.Config{})
	require.True(t, svc.GetExtraConcurrencyRuntimeSettings(context.Background()).Enabled)
	settings, err := svc.GetAllSettings(context.Background())
	require.NoError(t, err)
	reservePercent := 40.0
	settings.ExtraConcurrencyEnabled = false
	settings.ExtraConcurrencyWaitTimeoutSeconds = 60
	settings.ExtraConcurrencyReservePercent = 50
	settings.ExtraConcurrencyMinReservedSlots = 4
	settings.ExtraConcurrencyPlatformReserves = map[string]ExtraConcurrencyPlatformReserve{
		"grok": {ReservePercent: &reservePercent},
	}

	require.NoError(t, svc.UpdateSettings(context.Background(), settings))
	got := svc.GetExtraConcurrencyRuntimeSettings(context.Background())

	require.False(t, got.Enabled)
	require.Equal(t, 60, got.WaitTimeoutSeconds)
	require.Equal(t, 50.0, got.ReservePercent)
	require.Equal(t, 4, got.MinReservedSlots)
	require.Equal(t, 40.0, *got.PlatformReserves["grok"].ReservePercent)
	require.Equal(t, 1, repo.getMultiCall)
}
