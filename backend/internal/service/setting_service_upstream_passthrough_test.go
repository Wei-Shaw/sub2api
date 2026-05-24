package service

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

// fakeSettingRepoForUpstream is a minimal stub for SettingRepository.
type fakeSettingRepoForUpstream struct {
	mu       sync.Mutex
	values   map[string]string
	getCalls int
	failGet  bool
}

func (r *fakeSettingRepoForUpstream) Get(ctx context.Context, key string) (*Setting, error) {
	return nil, ErrSettingNotFound
}

func (r *fakeSettingRepoForUpstream) GetValue(ctx context.Context, key string) (string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.getCalls++
	if r.failGet {
		return "", errors.New("db unavailable")
	}
	if v, ok := r.values[key]; ok {
		return v, nil
	}
	return "", ErrSettingNotFound
}

func (r *fakeSettingRepoForUpstream) Set(ctx context.Context, key, value string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.values == nil {
		r.values = map[string]string{}
	}
	r.values[key] = value
	return nil
}

func (r *fakeSettingRepoForUpstream) GetMultiple(ctx context.Context, keys []string) (map[string]string, error) {
	return nil, errors.New("not implemented")
}

func (r *fakeSettingRepoForUpstream) SetMultiple(ctx context.Context, settings map[string]string) error {
	return errors.New("not implemented")
}

func (r *fakeSettingRepoForUpstream) GetAll(ctx context.Context) (map[string]string, error) {
	return nil, errors.New("not implemented")
}

func (r *fakeSettingRepoForUpstream) Delete(ctx context.Context, key string) error {
	return errors.New("not implemented")
}

func resetUpstreamPassthroughDefaultsCache() {
	upstreamPassthroughDefaultsCache.Store((*cachedUpstreamPassthroughDefaults)(nil))
}

func TestSettingService_GetUpstreamPassthroughDefaults_AbsentReturnsCodeDefaults(t *testing.T) {
	resetUpstreamPassthroughDefaultsCache()
	repo := &fakeSettingRepoForUpstream{values: map[string]string{}}
	svc := NewSettingService(repo, &config.Config{})

	got := svc.GetUpstreamPassthroughDefaults(context.Background())
	require.Equal(t, DefaultUpstreamPassthroughDefaults(), got)
}

func TestSettingService_GetUpstreamPassthroughDefaults_StoredValueWins(t *testing.T) {
	resetUpstreamPassthroughDefaultsCache()
	custom := UpstreamPassthroughDefaults{
		Relay:    UpstreamPassthroughCategoryDefault{Profile: ProfileStrict},
		Official: UpstreamPassthroughCategoryDefault{Profile: ProfileProtected},
		Reverse:  UpstreamPassthroughCategoryDefault{Profile: ProfileStrict},
	}
	raw, _ := json.Marshal(custom)
	repo := &fakeSettingRepoForUpstream{values: map[string]string{
		SettingKeyUpstreamPassthroughDefaults: string(raw),
	}}
	svc := NewSettingService(repo, &config.Config{})

	got := svc.GetUpstreamPassthroughDefaults(context.Background())
	require.Equal(t, custom, got)
}

func TestSettingService_GetUpstreamPassthroughDefaults_BrokenJSONReturnsCodeDefaults(t *testing.T) {
	resetUpstreamPassthroughDefaultsCache()
	repo := &fakeSettingRepoForUpstream{values: map[string]string{
		SettingKeyUpstreamPassthroughDefaults: "{not valid json",
	}}
	svc := NewSettingService(repo, &config.Config{})

	got := svc.GetUpstreamPassthroughDefaults(context.Background())
	require.Equal(t, DefaultUpstreamPassthroughDefaults(), got)
}

func TestSettingService_GetUpstreamPassthroughDefaults_DBErrorReturnsCodeDefaults(t *testing.T) {
	resetUpstreamPassthroughDefaultsCache()
	repo := &fakeSettingRepoForUpstream{failGet: true}
	svc := NewSettingService(repo, &config.Config{})

	got := svc.GetUpstreamPassthroughDefaults(context.Background())
	require.Equal(t, DefaultUpstreamPassthroughDefaults(), got)
}

func TestSettingService_GetUpstreamPassthroughDefaults_CachesAcrossCalls(t *testing.T) {
	resetUpstreamPassthroughDefaultsCache()
	repo := &fakeSettingRepoForUpstream{values: map[string]string{}}
	svc := NewSettingService(repo, &config.Config{})

	_ = svc.GetUpstreamPassthroughDefaults(context.Background())
	firstCalls := repo.getCalls
	_ = svc.GetUpstreamPassthroughDefaults(context.Background())
	_ = svc.GetUpstreamPassthroughDefaults(context.Background())
	require.Equal(t, firstCalls, repo.getCalls, "subsequent calls should hit cache")
}
