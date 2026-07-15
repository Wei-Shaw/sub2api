package service

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type contentModerationWorkerConfigSettingRepo struct {
	SettingRepository
	mu       sync.RWMutex
	value    string
	getErr   error
	getDelay time.Duration
	getCalls atomic.Int64
	started  chan struct{}
	release  <-chan struct{}
}

func (r *contentModerationWorkerConfigSettingRepo) GetValue(ctx context.Context, _ string) (string, error) {
	call := r.getCalls.Add(1)
	r.mu.RLock()
	value, err, delay := r.value, r.getErr, r.getDelay
	r.mu.RUnlock()
	if call == 1 && r.started != nil {
		close(r.started)
		select {
		case <-r.release:
		case <-ctx.Done():
			return "", ctx.Err()
		}
	}
	if delay > 0 {
		time.Sleep(delay)
	}
	return value, err
}

func (r *contentModerationWorkerConfigSettingRepo) Set(_ context.Context, _ string, value string) error {
	r.setValue(value, nil)
	return nil
}

func (r *contentModerationWorkerConfigSettingRepo) setValue(value string, err error) {
	r.mu.Lock()
	r.value = value
	r.getErr = err
	r.mu.Unlock()
}

func TestContentModerationWorkerConfig_ConcurrentMissLoadsOnceAndReturnsClones(t *testing.T) {
	cfg := defaultContentModerationConfig()
	cfg.Enabled = true
	cfg.WorkerCount = 7
	cfg.APIKeys = []string{"sk-worker"}
	cfg.GroupIDs = []int64{11, 12}
	cfg.AllGroups = false
	cfg.BlockedKeywords = []string{"blocked"}
	cfg.ModelFilter = ContentModerationModelFilter{Type: ContentModerationModelFilterInclude, Models: []string{"gpt-5"}}
	repo := &contentModerationWorkerConfigSettingRepo{
		value:    marshalContentModerationWorkerConfig(t, cfg),
		getDelay: 25 * time.Millisecond,
	}
	svc := NewContentModerationService(repo, nil, nil, nil, nil, nil, nil)

	configs := loadContentModerationWorkerConfigsConcurrently(t, svc, 32)

	require.Equal(t, int64(1), repo.getCalls.Load())
	require.NotSame(t, configs[0], configs[1])
	configs[0].APIKeys[0] = "mutated"
	configs[0].GroupIDs[0] = 99
	configs[0].BlockedKeywords[0] = "mutated"
	configs[0].Thresholds["sexual"] = 0
	configs[0].ModelFilter.Models[0] = "mutated"

	got, err := svc.loadWorkerConfig(context.Background())
	require.NoError(t, err)
	require.Equal(t, int64(1), repo.getCalls.Load())
	require.Equal(t, []string{"sk-worker"}, got.APIKeys)
	require.Equal(t, []int64{11, 12}, got.GroupIDs)
	require.Equal(t, []string{"blocked"}, got.BlockedKeywords)
	require.NotZero(t, got.Thresholds["sexual"])
	require.Equal(t, []string{"gpt-5"}, got.ModelFilter.Models)
}

func TestContentModerationWorkerConfig_TTLRefreshLoadsOnce(t *testing.T) {
	cfg := defaultContentModerationConfig()
	repo := &contentModerationWorkerConfigSettingRepo{
		value:    marshalContentModerationWorkerConfig(t, cfg),
		getDelay: 25 * time.Millisecond,
	}
	svc := NewContentModerationService(repo, nil, nil, nil, nil, nil, nil)

	_, err := svc.loadWorkerConfig(context.Background())
	require.NoError(t, err)
	require.Equal(t, int64(1), repo.getCalls.Load())

	time.Sleep(contentModerationWorkerConfigTTL + 50*time.Millisecond)
	loadContentModerationWorkerConfigsConcurrently(t, svc, 32)
	require.Equal(t, int64(2), repo.getCalls.Load())
}

func TestContentModerationWorkerConfig_UpdatePublishesImmediately(t *testing.T) {
	cfg := defaultContentModerationConfig()
	cfg.WorkerCount = 4
	repo := &contentModerationWorkerConfigSettingRepo{value: marshalContentModerationWorkerConfig(t, cfg)}
	svc := NewContentModerationService(repo, nil, nil, nil, nil, nil, nil)

	warm, err := svc.loadWorkerConfig(context.Background())
	require.NoError(t, err)
	require.Equal(t, 4, warm.WorkerCount)
	require.Equal(t, int64(1), repo.getCalls.Load())

	workerCount := 9
	_, err = svc.UpdateConfig(context.Background(), UpdateContentModerationConfigInput{WorkerCount: &workerCount})
	require.NoError(t, err)
	require.Equal(t, int64(2), repo.getCalls.Load(), "UpdateConfig must keep its direct read")

	updated, err := svc.loadWorkerConfig(context.Background())
	require.NoError(t, err)
	require.Equal(t, 9, updated.WorkerCount)
	require.Equal(t, int64(2), repo.getCalls.Load(), "worker must observe the published snapshot without another read")

	_, err = svc.GetConfig(context.Background())
	require.NoError(t, err)
	require.Equal(t, int64(3), repo.getCalls.Load(), "synchronous GetConfig must keep its direct read")
}

func TestContentModerationWorkerConfig_UpdateWinsAgainstInflightRefresh(t *testing.T) {
	initial := defaultContentModerationConfig()
	initial.WorkerCount = 4
	started := make(chan struct{})
	release := make(chan struct{})
	repo := &contentModerationWorkerConfigSettingRepo{
		value:   marshalContentModerationWorkerConfig(t, initial),
		started: started,
		release: release,
	}
	svc := NewContentModerationService(repo, nil, nil, nil, nil, nil, nil)
	result := make(chan *ContentModerationConfig, 1)
	errResult := make(chan error, 1)
	go func() {
		cfg, err := svc.loadWorkerConfig(context.Background())
		result <- cfg
		errResult <- err
	}()
	<-started

	workerCount := 9
	_, err := svc.UpdateConfig(context.Background(), UpdateContentModerationConfigInput{WorkerCount: &workerCount})
	require.NoError(t, err)
	close(release)

	require.NoError(t, <-errResult)
	require.Equal(t, 9, (<-result).WorkerCount, "inflight stale read must not overwrite the update")
	got, err := svc.loadWorkerConfig(context.Background())
	require.NoError(t, err)
	require.Equal(t, 9, got.WorkerCount)
	require.Equal(t, int64(2), repo.getCalls.Load())
}

func TestContentModerationWorkerConfig_UpdateWinsAgainstInflightRefreshError(t *testing.T) {
	initial := defaultContentModerationConfig()
	initial.WorkerCount = 4
	started := make(chan struct{})
	release := make(chan struct{})
	repo := &contentModerationWorkerConfigSettingRepo{
		value:   marshalContentModerationWorkerConfig(t, initial),
		getErr:  errors.New("stale read failed"),
		started: started,
		release: release,
	}
	svc := NewContentModerationService(repo, nil, nil, nil, nil, nil, nil)
	result := make(chan *ContentModerationConfig, 1)
	errResult := make(chan error, 1)
	go func() {
		cfg, err := svc.loadWorkerConfig(context.Background())
		result <- cfg
		errResult <- err
	}()
	<-started

	repo.setValue(marshalContentModerationWorkerConfig(t, initial), nil)
	workerCount := 9
	_, err := svc.UpdateConfig(context.Background(), UpdateContentModerationConfigInput{WorkerCount: &workerCount})
	require.NoError(t, err)
	close(release)

	require.NoError(t, <-errResult)
	require.Equal(t, 9, (<-result).WorkerCount)
	got, err := svc.loadWorkerConfig(context.Background())
	require.NoError(t, err)
	require.Equal(t, 9, got.WorkerCount)
	require.Equal(t, int64(2), repo.getCalls.Load())
}

func TestContentModerationWorkerConfig_UpdateClearsRefreshErrorThrottle(t *testing.T) {
	repo := &contentModerationWorkerConfigSettingRepo{getErr: errors.New("read failed")}
	svc := NewContentModerationService(repo, nil, nil, nil, nil, nil, nil)

	got, err := svc.loadWorkerConfig(context.Background())
	require.Error(t, err)
	require.Nil(t, got)
	require.Equal(t, int64(1), repo.getCalls.Load())

	initial := defaultContentModerationConfig()
	repo.setValue(marshalContentModerationWorkerConfig(t, initial), nil)
	workerCount := 9
	_, err = svc.UpdateConfig(context.Background(), UpdateContentModerationConfigInput{WorkerCount: &workerCount})
	require.NoError(t, err)

	got, err = svc.loadWorkerConfig(context.Background())
	require.NoError(t, err)
	require.Equal(t, 9, got.WorkerCount)
	require.Equal(t, int64(2), repo.getCalls.Load())
}

func TestContentModerationWorkerConfig_FirstCallerCancellationDoesNotCancelRefresh(t *testing.T) {
	cfg := defaultContentModerationConfig()
	cfg.WorkerCount = 7
	started := make(chan struct{})
	release := make(chan struct{})
	repo := &contentModerationWorkerConfigSettingRepo{
		value:   marshalContentModerationWorkerConfig(t, cfg),
		started: started,
		release: release,
	}
	svc := NewContentModerationService(repo, nil, nil, nil, nil, nil, nil)

	firstCtx, cancelFirst := context.WithCancel(context.Background())
	firstResult := make(chan error, 1)
	go func() {
		_, err := svc.loadWorkerConfig(firstCtx)
		firstResult <- err
	}()
	<-started
	cancelFirst()
	require.ErrorIs(t, <-firstResult, context.Canceled)

	close(release)
	got, err := svc.loadWorkerConfig(context.Background())
	require.NoError(t, err)
	require.Equal(t, 7, got.WorkerCount)
	require.Equal(t, int64(1), repo.getCalls.Load())
}

func TestContentModerationWorkerConfig_RefreshErrorDoesNotServeOrPoisonCache(t *testing.T) {
	tests := []struct {
		name     string
		badValue string
		getErr   error
	}{
		{name: "repository error", getErr: errors.New("read failed")},
		{name: "parse error", badValue: "{"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			initial := defaultContentModerationConfig()
			initial.WorkerCount = 4
			repo := &contentModerationWorkerConfigSettingRepo{value: marshalContentModerationWorkerConfig(t, initial)}
			svc := NewContentModerationService(repo, nil, nil, nil, nil, nil, nil)

			_, err := svc.loadWorkerConfig(context.Background())
			require.NoError(t, err)
			expireContentModerationWorkerConfig(svc)
			repo.setValue(tt.badValue, tt.getErr)

			got, err := svc.loadWorkerConfig(context.Background())
			require.Error(t, err)
			require.Nil(t, got, "expired config must not be served stale")
			firstErr := err

			recovered := defaultContentModerationConfig()
			recovered.WorkerCount = 8
			repo.setValue(marshalContentModerationWorkerConfig(t, recovered), nil)
			for range 32 {
				got, err = svc.loadWorkerConfig(context.Background())
				require.Equal(t, firstErr, err, "refresh errors must be throttled for one second")
				require.Nil(t, got)
			}
			require.Equal(t, int64(2), repo.getCalls.Load())

			expireContentModerationWorkerConfigError(svc)
			got, err = svc.loadWorkerConfig(context.Background())
			require.NoError(t, err)
			require.Equal(t, 8, got.WorkerCount)
			require.Equal(t, int64(3), repo.getCalls.Load(), "refresh must retry after the error throttle expires")
		})
	}
}

func BenchmarkContentModerationWorkerConfigLoad(b *testing.B) {
	cfg := defaultContentModerationConfig()
	cfg.APIKeys = []string{"sk-worker-a", "sk-worker-b"}
	cfg.GroupIDs = []int64{11, 12}
	cfg.BlockedKeywords = []string{"blocked-a", "blocked-b"}
	cfg.ModelFilter = ContentModerationModelFilter{Type: ContentModerationModelFilterInclude, Models: []string{"gpt-5", "gpt-5-mini"}}
	raw, err := json.Marshal(cfg)
	if err != nil {
		b.Fatal(err)
	}
	repo := &contentModerationWorkerConfigSettingRepo{value: string(raw)}
	svc := NewContentModerationService(repo, nil, nil, nil, nil, nil, nil)
	ctx := context.Background()
	if _, err := svc.loadWorkerConfig(ctx); err != nil {
		b.Fatal(err)
	}
	startCalls := repo.getCalls.Load()

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		cfg, err := svc.loadWorkerConfig(ctx)
		if err != nil {
			b.Fatal(err)
		}
		if cfg.WorkerCount == 0 {
			b.Fatal("unexpected zero worker count")
		}
	}
	b.StopTimer()
	loads := repo.getCalls.Load() - startCalls
	b.ReportMetric(float64(loads)/b.Elapsed().Seconds(), "repo-loads/s")
}

func marshalContentModerationWorkerConfig(t testing.TB, cfg *ContentModerationConfig) string {
	t.Helper()
	raw, err := json.Marshal(cfg)
	require.NoError(t, err)
	return string(raw)
}

func loadContentModerationWorkerConfigsConcurrently(t *testing.T, svc *ContentModerationService, count int) []*ContentModerationConfig {
	t.Helper()
	start := make(chan struct{})
	configs := make([]*ContentModerationConfig, count)
	errs := make([]error, count)
	var wg sync.WaitGroup
	wg.Add(count)
	for i := 0; i < count; i++ {
		go func(index int) {
			defer wg.Done()
			<-start
			configs[index], errs[index] = svc.loadWorkerConfig(context.Background())
		}(i)
	}
	close(start)
	wg.Wait()
	for _, err := range errs {
		require.NoError(t, err)
	}
	return configs
}

func expireContentModerationWorkerConfig(svc *ContentModerationService) {
	snapshot := svc.workerConfigCache.Load()
	svc.workerConfigCache.Store(&contentModerationWorkerConfigSnapshot{
		config:    cloneContentModerationConfig(snapshot.config),
		expiresAt: time.Now().Add(-time.Second).UnixNano(),
	})
}

func expireContentModerationWorkerConfigError(svc *ContentModerationService) {
	snapshot := svc.workerConfigError.Load()
	svc.workerConfigError.Store(&contentModerationWorkerConfigErrorSnapshot{
		err:       snapshot.err,
		expiresAt: time.Now().Add(-time.Second).UnixNano(),
	})
}
