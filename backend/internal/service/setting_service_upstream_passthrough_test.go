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

func TestSettingService_SetUpstreamPassthroughDefaults_PersistAndInvalidate(t *testing.T) {
	resetUpstreamPassthroughDefaultsCache()
	repo := &fakeSettingRepoForUpstream{values: map[string]string{}}
	svc := NewSettingService(repo, &config.Config{})

	// Warm cache with code defaults
	_ = svc.GetUpstreamPassthroughDefaults(context.Background())
	priorCalls := repo.getCalls

	// Write a custom value
	custom := UpstreamPassthroughDefaults{
		Relay:    UpstreamPassthroughCategoryDefault{Profile: ProfileStrict},
		Official: UpstreamPassthroughCategoryDefault{Profile: ProfileProtected},
		Reverse:  UpstreamPassthroughCategoryDefault{Profile: ProfileStrict},
	}
	require.NoError(t, svc.SetUpstreamPassthroughDefaults(context.Background(), custom))

	// Next Get must read fresh from repo (cache invalidated) and return the custom value
	got := svc.GetUpstreamPassthroughDefaults(context.Background())
	require.Equal(t, custom, got)
	require.Greater(t, repo.getCalls, priorCalls, "Set must invalidate cache so next Get refetches")

	// And the persisted JSON round-trips
	saved := repo.values[SettingKeyUpstreamPassthroughDefaults]
	require.NotEmpty(t, saved)
	var parsed UpstreamPassthroughDefaults
	require.NoError(t, json.Unmarshal([]byte(saved), &parsed))
	require.Equal(t, custom, parsed)
}

func TestSettingService_SetUpstreamPassthroughDefaults_RejectsInvalidProfile(t *testing.T) {
	resetUpstreamPassthroughDefaultsCache()
	repo := &fakeSettingRepoForUpstream{values: map[string]string{}}
	svc := NewSettingService(repo, &config.Config{})

	bad := UpstreamPassthroughDefaults{
		Relay:    UpstreamPassthroughCategoryDefault{Profile: PassthroughProfile("nonexistent")},
		Official: UpstreamPassthroughCategoryDefault{Profile: ProfileProtected},
		Reverse:  UpstreamPassthroughCategoryDefault{Profile: ProfileStrict},
	}
	err := svc.SetUpstreamPassthroughDefaults(context.Background(), bad)
	require.Error(t, err)
	require.Empty(t, repo.values[SettingKeyUpstreamPassthroughDefaults], "must not persist invalid value")
}

func TestSettingService_SetUpstreamPassthroughDefaults_StripsUnknownToggleKeys(t *testing.T) {
	resetUpstreamPassthroughDefaultsCache()
	repo := &fakeSettingRepoForUpstream{values: map[string]string{}}
	svc := NewSettingService(repo, &config.Config{})

	input := UpstreamPassthroughDefaults{
		Relay: UpstreamPassthroughCategoryDefault{
			Profile: ProfileTransparent,
			Overrides: map[string]bool{
				"forward_client_headers": true,
				"unknown_garbage_toggle": true,
			},
		},
		Official: UpstreamPassthroughCategoryDefault{Profile: ProfileProtected},
		Reverse:  UpstreamPassthroughCategoryDefault{Profile: ProfileStrict},
	}
	require.NoError(t, svc.SetUpstreamPassthroughDefaults(context.Background(), input))

	got := svc.GetUpstreamPassthroughDefaults(context.Background())
	require.Len(t, got.Relay.Overrides, 1)
	require.True(t, got.Relay.Overrides[ToggleForwardClientHeaders])
	_, exists := got.Relay.Overrides["unknown_garbage_toggle"]
	require.False(t, exists)
}

func TestSettingService_GetSetGlobalOverride(t *testing.T) {
	repo := &fakeSettingRepoForUpstream{values: map[string]string{}}
	svc := NewSettingService(repo, &config.Config{})

	t.Run("absent returns auto", func(t *testing.T) {
		require.Equal(t, GlobalOverrideAuto, svc.GetUpstreamPassthroughGlobalOverride(context.Background()))
	})

	t.Run("set then get round-trip", func(t *testing.T) {
		require.NoError(t, svc.SetUpstreamPassthroughGlobalOverride(context.Background(), GlobalOverrideForceStrict))
		require.Equal(t, GlobalOverrideForceStrict, svc.GetUpstreamPassthroughGlobalOverride(context.Background()))
	})

	t.Run("set invalid value rejected", func(t *testing.T) {
		err := svc.SetUpstreamPassthroughGlobalOverride(context.Background(), GlobalOverrideMode("garbage"))
		require.Error(t, err)
	})

	t.Run("set auto persists empty string equivalent", func(t *testing.T) {
		require.NoError(t, svc.SetUpstreamPassthroughGlobalOverride(context.Background(), GlobalOverrideAuto))
		require.Equal(t, GlobalOverrideAuto, svc.GetUpstreamPassthroughGlobalOverride(context.Background()))
	})

	t.Run("stored garbage in db returns auto (fail-open)", func(t *testing.T) {
		repo.mu.Lock()
		repo.values[SettingKeyUpstreamPassthroughGlobalOverride] = "definitely-not-valid"
		repo.mu.Unlock()
		require.Equal(t, GlobalOverrideAuto, svc.GetUpstreamPassthroughGlobalOverride(context.Background()))
	})
}

func TestSettingService_IsUpstreamPolicyV1Enabled(t *testing.T) {
	repo := &fakeSettingRepoForUpstream{values: map[string]string{}}
	svc := NewSettingService(repo, &config.Config{})

	t.Run("absent returns false (Phase A default OFF)", func(t *testing.T) {
		require.False(t, svc.IsUpstreamPolicyV1Enabled(context.Background()))
	})

	t.Run("set true", func(t *testing.T) {
		repo.mu.Lock()
		repo.values[SettingKeyUpstreamPolicyV1Enabled] = "true"
		repo.mu.Unlock()
		require.True(t, svc.IsUpstreamPolicyV1Enabled(context.Background()))
	})

	t.Run("non-true value returns false", func(t *testing.T) {
		repo.mu.Lock()
		repo.values[SettingKeyUpstreamPolicyV1Enabled] = "anything-else"
		repo.mu.Unlock()
		require.False(t, svc.IsUpstreamPolicyV1Enabled(context.Background()))
	})
}
