package service

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type contentModerationRuntimeSettingRepo struct {
	mu               sync.Mutex
	values           map[string]string
	getValueCalls    int
	getMultipleCalls int
	getMultipleErr   error
	getMultipleStart chan<- struct{}
	getMultipleWait  <-chan struct{}
}

func (r *contentModerationRuntimeSettingRepo) Get(_ context.Context, key string) (*Setting, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	value, ok := r.values[key]
	if !ok {
		return nil, ErrSettingNotFound
	}
	return &Setting{Key: key, Value: value}, nil
}

func (r *contentModerationRuntimeSettingRepo) GetValue(_ context.Context, key string) (string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.getValueCalls++
	value, ok := r.values[key]
	if !ok {
		return "", ErrSettingNotFound
	}
	return value, nil
}

func (r *contentModerationRuntimeSettingRepo) Set(_ context.Context, key, value string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.values == nil {
		r.values = make(map[string]string)
	}
	r.values[key] = value
	return nil
}

func (r *contentModerationRuntimeSettingRepo) GetMultiple(_ context.Context, keys []string) (map[string]string, error) {
	r.mu.Lock()
	r.getMultipleCalls++
	if err := r.getMultipleErr; err != nil {
		r.mu.Unlock()
		return nil, err
	}
	out := make(map[string]string, len(keys))
	for _, key := range keys {
		if value, ok := r.values[key]; ok {
			out[key] = value
		}
	}
	start := r.getMultipleStart
	wait := r.getMultipleWait
	r.getMultipleStart = nil
	r.getMultipleWait = nil
	r.mu.Unlock()
	if start != nil {
		start <- struct{}{}
	}
	if wait != nil {
		<-wait
	}
	return out, nil
}

func (r *contentModerationRuntimeSettingRepo) SetMultiple(_ context.Context, values map[string]string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.values == nil {
		r.values = make(map[string]string)
	}
	for key, value := range values {
		r.values[key] = value
	}
	return nil
}

func (r *contentModerationRuntimeSettingRepo) GetAll(_ context.Context) (map[string]string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make(map[string]string, len(r.values))
	for key, value := range r.values {
		out[key] = value
	}
	return out, nil
}

func (r *contentModerationRuntimeSettingRepo) Delete(_ context.Context, key string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.values, key)
	return nil
}

func (r *contentModerationRuntimeSettingRepo) calls() (getValue, getMultiple int) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.getValueCalls, r.getMultipleCalls
}

func (r *contentModerationRuntimeSettingRepo) failMultiple(err error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.getMultipleErr = err
}

func (r *contentModerationRuntimeSettingRepo) blockNextMultiple(start chan<- struct{}, wait <-chan struct{}) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.getMultipleStart = start
	r.getMultipleWait = wait
}

func runtimeCacheTestConfig(t *testing.T, keywords ...string) string {
	t.Helper()
	cfg := defaultContentModerationConfig()
	cfg.Enabled = true
	cfg.Mode = ContentModerationModePreBlock
	cfg.KeywordBlockingMode = ContentModerationKeywordModeKeywordOnly
	cfg.BlockedKeywords = keywords
	raw, err := json.Marshal(cfg)
	require.NoError(t, err)
	return string(raw)
}

func runtimeCacheTestService(repo *contentModerationRuntimeSettingRepo, ttl time.Duration) *ContentModerationService {
	return &ContentModerationService{
		settingRepo:     repo,
		repo:            &contentModerationTestRepo{},
		runtimeCacheTTL: ttl,
	}
}

func runtimeCacheTestInput(text string) ContentModerationCheckInput {
	return ContentModerationCheckInput{
		Protocol: ContentModerationProtocolOpenAIChat,
		Model:    "risk-cache-test",
		Body:     []byte(`{"messages":[{"role":"user","content":"` + text + `"}]}`),
	}
}

func TestContentModerationRuntimeSnapshotCachesSettings(t *testing.T) {
	repo := &contentModerationRuntimeSettingRepo{values: map[string]string{
		SettingKeyRiskControlEnabled:      "true",
		SettingKeyContentModerationConfig: runtimeCacheTestConfig(t, "blocked"),
	}}
	svc := runtimeCacheTestService(repo, time.Hour)

	for range 20 {
		decision, err := svc.Check(context.Background(), runtimeCacheTestInput("clean prompt"))
		require.NoError(t, err)
		require.True(t, decision.Allowed)
	}

	getValue, getMultiple := repo.calls()
	require.Zero(t, getValue)
	require.Equal(t, 1, getMultiple)
}

func TestContentModerationRuntimeSnapshotUpdateConfigIsImmediate(t *testing.T) {
	repo := &contentModerationRuntimeSettingRepo{values: map[string]string{
		SettingKeyRiskControlEnabled:      "true",
		SettingKeyContentModerationConfig: runtimeCacheTestConfig(t, "old-keyword"),
	}}
	svc := runtimeCacheTestService(repo, time.Hour)

	decision, err := svc.Check(context.Background(), runtimeCacheTestInput("new-keyword"))
	require.NoError(t, err)
	require.True(t, decision.Allowed)

	keywords := []string{"new-keyword"}
	_, err = svc.UpdateConfig(context.Background(), UpdateContentModerationConfigInput{
		BlockedKeywords: &keywords,
	})
	require.NoError(t, err)

	decision, err = svc.Check(context.Background(), runtimeCacheTestInput("new-keyword"))
	require.NoError(t, err)
	require.True(t, decision.Blocked)

	_, getMultiple := repo.calls()
	require.Equal(t, 1, getMultiple)
}

func TestContentModerationRuntimeSnapshotUpdateWinsOverInitialLoad(t *testing.T) {
	repo := &contentModerationRuntimeSettingRepo{values: map[string]string{
		SettingKeyRiskControlEnabled:      "true",
		SettingKeyContentModerationConfig: runtimeCacheTestConfig(t, "old-keyword"),
	}}
	svc := runtimeCacheTestService(repo, time.Hour)

	refreshStarted := make(chan struct{}, 1)
	releaseRefresh := make(chan struct{})
	released := false
	defer func() {
		if !released {
			close(releaseRefresh)
		}
	}()
	repo.blockNextMultiple(refreshStarted, releaseRefresh)

	initialCheckDone := make(chan error, 1)
	go func() {
		decision, err := svc.Check(context.Background(), runtimeCacheTestInput("clean prompt"))
		if err == nil && (decision == nil || !decision.Allowed) {
			err = errors.New("unexpected initial moderation decision")
		}
		initialCheckDone <- err
	}()
	require.Eventually(t, func() bool {
		select {
		case <-refreshStarted:
			return true
		default:
			return false
		}
	}, time.Second, time.Millisecond)

	updateDone := make(chan error, 1)
	go func() {
		keywords := []string{"new-keyword"}
		_, updateErr := svc.UpdateConfig(context.Background(), UpdateContentModerationConfigInput{
			BlockedKeywords: &keywords,
		})
		updateDone <- updateErr
	}()
	select {
	case updateErr := <-updateDone:
		require.NoError(t, updateErr)
		t.Fatal("configuration update completed before the initial load released its lock")
	case <-time.After(10 * time.Millisecond):
	}

	close(releaseRefresh)
	released = true
	require.NoError(t, <-initialCheckDone)
	require.NoError(t, <-updateDone)

	decision, err := svc.Check(context.Background(), runtimeCacheTestInput("new-keyword"))
	require.NoError(t, err)
	require.True(t, decision.Blocked)
	decision, err = svc.Check(context.Background(), runtimeCacheTestInput("old-keyword"))
	require.NoError(t, err)
	require.True(t, decision.Allowed)
}

func TestContentModerationRuntimeSnapshotRefreshFailureKeepsStaleConfig(t *testing.T) {
	repo := &contentModerationRuntimeSettingRepo{values: map[string]string{
		SettingKeyRiskControlEnabled:      "true",
		SettingKeyContentModerationConfig: runtimeCacheTestConfig(t, "blocked"),
	}}
	svc := runtimeCacheTestService(repo, time.Nanosecond)
	input := runtimeCacheTestInput("blocked")

	decision, err := svc.Check(context.Background(), input)
	require.NoError(t, err)
	require.True(t, decision.Blocked)

	repo.failMultiple(errors.New("database unavailable"))
	decision, err = svc.Check(context.Background(), input)
	require.NoError(t, err)
	require.True(t, decision.Blocked)
	require.Eventually(t, func() bool {
		_, calls := repo.calls()
		return calls >= 2
	}, time.Second, time.Millisecond)
}

func TestContentModerationRuntimeSnapshotRefreshFailureBacksOff(t *testing.T) {
	repo := &contentModerationRuntimeSettingRepo{values: map[string]string{
		SettingKeyRiskControlEnabled:      "true",
		SettingKeyContentModerationConfig: runtimeCacheTestConfig(t, "blocked"),
	}}
	svc := runtimeCacheTestService(repo, time.Minute)
	input := runtimeCacheTestInput("blocked")

	decision, err := svc.Check(context.Background(), input)
	require.NoError(t, err)
	require.True(t, decision.Blocked)

	current := svc.runtimeSnapshot.Load()
	require.NotNil(t, current)
	expired := *current
	expired.loadedAt = time.Now().Add(-2 * time.Minute)
	svc.runtimeSnapshot.Store(&expired)
	repo.failMultiple(errors.New("database unavailable"))

	decision, err = svc.Check(context.Background(), input)
	require.NoError(t, err)
	require.True(t, decision.Blocked)
	require.Eventually(t, func() bool {
		_, calls := repo.calls()
		return calls == 2
	}, time.Second, time.Millisecond)

	for range 100 {
		decision, err = svc.Check(context.Background(), input)
		require.NoError(t, err)
		require.True(t, decision.Blocked)
	}
	_, calls := repo.calls()
	require.Equal(t, 2, calls)
}

func TestContentModerationRuntimeSnapshotRefreshReusesUnchangedMatcher(t *testing.T) {
	repo := &contentModerationRuntimeSettingRepo{values: map[string]string{
		SettingKeyRiskControlEnabled:      "true",
		SettingKeyContentModerationConfig: runtimeCacheTestConfig(t, "blocked"),
	}}
	svc := runtimeCacheTestService(repo, time.Minute)
	input := runtimeCacheTestInput("blocked")

	decision, err := svc.Check(context.Background(), input)
	require.NoError(t, err)
	require.True(t, decision.Blocked)

	current := svc.runtimeSnapshot.Load()
	require.NotNil(t, current)
	expired := *current
	expired.loadedAt = time.Now().Add(-2 * time.Minute)
	svc.runtimeSnapshot.Store(&expired)

	decision, err = svc.Check(context.Background(), input)
	require.NoError(t, err)
	require.True(t, decision.Blocked)
	require.Eventually(t, func() bool {
		refreshed := svc.runtimeSnapshot.Load()
		return refreshed != nil && refreshed.loadedAt.After(expired.loadedAt)
	}, time.Second, time.Millisecond)

	refreshed := svc.runtimeSnapshot.Load()
	require.Same(t, current.config, refreshed.config)
	require.Same(t, current.keywordMatcher, refreshed.keywordMatcher)
	_, calls := repo.calls()
	require.Equal(t, 2, calls)
}

func TestContentModerationRuntimeSnapshotUpdateWinsOverInFlightRefresh(t *testing.T) {
	repo := &contentModerationRuntimeSettingRepo{values: map[string]string{
		SettingKeyRiskControlEnabled:      "true",
		SettingKeyContentModerationConfig: runtimeCacheTestConfig(t, "old-keyword"),
	}}
	svc := runtimeCacheTestService(repo, time.Minute)

	decision, err := svc.Check(context.Background(), runtimeCacheTestInput("old-keyword"))
	require.NoError(t, err)
	require.True(t, decision.Blocked)

	current := svc.runtimeSnapshot.Load()
	require.NotNil(t, current)
	expired := *current
	expired.loadedAt = time.Now().Add(-2 * time.Minute)
	svc.runtimeSnapshot.Store(&expired)

	refreshStarted := make(chan struct{}, 1)
	releaseRefresh := make(chan struct{})
	repo.blockNextMultiple(refreshStarted, releaseRefresh)
	decision, err = svc.Check(context.Background(), runtimeCacheTestInput("clean prompt"))
	require.NoError(t, err)
	require.True(t, decision.Allowed)
	require.Eventually(t, func() bool {
		select {
		case <-refreshStarted:
			return true
		default:
			return false
		}
	}, time.Second, time.Millisecond)

	updateDone := make(chan error, 1)
	go func() {
		keywords := []string{"new-keyword"}
		_, updateErr := svc.UpdateConfig(context.Background(), UpdateContentModerationConfigInput{
			BlockedKeywords: &keywords,
		})
		updateDone <- updateErr
	}()
	select {
	case updateErr := <-updateDone:
		require.NoError(t, updateErr)
		t.Fatal("configuration update completed before the in-flight refresh released its lock")
	case <-time.After(10 * time.Millisecond):
	}

	close(releaseRefresh)
	require.NoError(t, <-updateDone)
	decision, err = svc.Check(context.Background(), runtimeCacheTestInput("new-keyword"))
	require.NoError(t, err)
	require.True(t, decision.Blocked)
	decision, err = svc.Check(context.Background(), runtimeCacheTestInput("old-keyword"))
	require.NoError(t, err)
	require.True(t, decision.Allowed)
}

func TestContentModerationRuntimeSnapshotConcurrentReadAndReplace(t *testing.T) {
	repo := &contentModerationRuntimeSettingRepo{values: map[string]string{
		SettingKeyRiskControlEnabled:      "true",
		SettingKeyContentModerationConfig: runtimeCacheTestConfig(t, "blocked-0"),
	}}
	svc := runtimeCacheTestService(repo, time.Hour)
	_, err := svc.Check(context.Background(), runtimeCacheTestInput("clean prompt"))
	require.NoError(t, err)

	var wg sync.WaitGroup
	errs := make(chan error, 8)
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range 100 {
				decision, checkErr := svc.Check(context.Background(), runtimeCacheTestInput("clean prompt"))
				if checkErr != nil {
					errs <- checkErr
					return
				}
				if decision == nil || !decision.Allowed {
					errs <- errors.New("unexpected moderation decision")
					return
				}
			}
		}()
	}
	for i := 1; i <= 20; i++ {
		keywords := []string{"blocked-" + time.Duration(i).String()}
		_, err := svc.UpdateConfig(context.Background(), UpdateContentModerationConfigInput{
			BlockedKeywords: &keywords,
		})
		require.NoError(t, err)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		require.NoError(t, err)
	}
}
