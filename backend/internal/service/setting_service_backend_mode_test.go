//go:build unit

package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

type bmRepoStub struct {
	getValueFn func(ctx context.Context, key string) (string, error)
	calls      int
}

func (s *bmRepoStub) Get(ctx context.Context, key string) (*Setting, error) {
	panic("unexpected Get call")
}

func (s *bmRepoStub) GetValue(ctx context.Context, key string) (string, error) {
	s.calls++
	if s.getValueFn == nil {
		panic("unexpected GetValue call")
	}
	return s.getValueFn(ctx, key)
}

func (s *bmRepoStub) Set(ctx context.Context, key, value string) error {
	panic("unexpected Set call")
}

func (s *bmRepoStub) GetMultiple(ctx context.Context, keys []string) (map[string]string, error) {
	panic("unexpected GetMultiple call")
}

func (s *bmRepoStub) SetMultiple(ctx context.Context, settings map[string]string) error {
	panic("unexpected SetMultiple call")
}

func (s *bmRepoStub) GetAll(ctx context.Context) (map[string]string, error) {
	panic("unexpected GetAll call")
}

func (s *bmRepoStub) Delete(ctx context.Context, key string) error {
	panic("unexpected Delete call")
}

type bmUpdateRepoStub struct {
	updates    map[string]string
	getValueFn func(ctx context.Context, key string) (string, error)
}

func (s *bmUpdateRepoStub) Get(ctx context.Context, key string) (*Setting, error) {
	panic("unexpected Get call")
}

func (s *bmUpdateRepoStub) GetValue(ctx context.Context, key string) (string, error) {
	if s.getValueFn == nil {
		panic("unexpected GetValue call")
	}
	return s.getValueFn(ctx, key)
}

func (s *bmUpdateRepoStub) Set(ctx context.Context, key, value string) error {
	panic("unexpected Set call")
}

func (s *bmUpdateRepoStub) GetMultiple(ctx context.Context, keys []string) (map[string]string, error) {
	panic("unexpected GetMultiple call")
}

func (s *bmUpdateRepoStub) SetMultiple(ctx context.Context, settings map[string]string) error {
	s.updates = make(map[string]string, len(settings))
	for k, v := range settings {
		s.updates[k] = v
	}
	return nil
}

func (s *bmUpdateRepoStub) GetAll(ctx context.Context) (map[string]string, error) {
	panic("unexpected GetAll call")
}

func (s *bmUpdateRepoStub) Delete(ctx context.Context, key string) error {
	panic("unexpected Delete call")
}

func resetBackendModeTestCache(t *testing.T) {
	t.Helper()

	backendModeCache.Store((*cachedBackendMode)(nil))
	t.Cleanup(func() {
		backendModeCache.Store((*cachedBackendMode)(nil))
	})
}

func TestIsBackendModeEnabled_ReturnsTrue(t *testing.T) {
	resetBackendModeTestCache(t)

	repo := &bmRepoStub{
		getValueFn: func(ctx context.Context, key string) (string, error) {
			require.Equal(t, SettingKeyBackendModeEnabled, key)
			return "true", nil
		},
	}
	svc := NewSettingService(repo, &config.Config{})

	require.True(t, svc.IsBackendModeEnabled(context.Background()))
	require.Equal(t, 1, repo.calls)
}

func TestIsBackendModeEnabled_ReturnsFalse(t *testing.T) {
	resetBackendModeTestCache(t)

	repo := &bmRepoStub{
		getValueFn: func(ctx context.Context, key string) (string, error) {
			require.Equal(t, SettingKeyBackendModeEnabled, key)
			return "false", nil
		},
	}
	svc := NewSettingService(repo, &config.Config{})

	require.False(t, svc.IsBackendModeEnabled(context.Background()))
	require.Equal(t, 1, repo.calls)
}

func TestIsBackendModeEnabled_ReturnsFalseOnNotFound(t *testing.T) {
	resetBackendModeTestCache(t)

	repo := &bmRepoStub{
		getValueFn: func(ctx context.Context, key string) (string, error) {
			require.Equal(t, SettingKeyBackendModeEnabled, key)
			return "", ErrSettingNotFound
		},
	}
	svc := NewSettingService(repo, &config.Config{})

	require.False(t, svc.IsBackendModeEnabled(context.Background()))
	require.Equal(t, 1, repo.calls)
}

func TestIsBackendModeEnabled_ReturnsFalseOnDBError(t *testing.T) {
	resetBackendModeTestCache(t)

	repo := &bmRepoStub{
		getValueFn: func(ctx context.Context, key string) (string, error) {
			require.Equal(t, SettingKeyBackendModeEnabled, key)
			return "", errors.New("db down")
		},
	}
	svc := NewSettingService(repo, &config.Config{})

	require.False(t, svc.IsBackendModeEnabled(context.Background()))
	require.Equal(t, 1, repo.calls)
}

func TestIsBackendModeEnabled_CachesResult(t *testing.T) {
	resetBackendModeTestCache(t)

	repo := &bmRepoStub{
		getValueFn: func(ctx context.Context, key string) (string, error) {
			require.Equal(t, SettingKeyBackendModeEnabled, key)
			return "true", nil
		},
	}
	svc := NewSettingService(repo, &config.Config{})

	require.True(t, svc.IsBackendModeEnabled(context.Background()))
	require.True(t, svc.IsBackendModeEnabled(context.Background()))
	require.Equal(t, 1, repo.calls)
}

func TestUpdateSettings_InvalidatesBackendModeCache(t *testing.T) {
	resetBackendModeTestCache(t)

	backendModeCache.Store(&cachedBackendMode{
		value:     true,
		expiresAt: time.Now().Add(backendModeCacheTTL).UnixNano(),
	})

	repo := &bmUpdateRepoStub{
		getValueFn: func(ctx context.Context, key string) (string, error) {
			require.Equal(t, SettingKeyBackendModeEnabled, key)
			return "true", nil
		},
	}
	svc := NewSettingService(repo, &config.Config{})

	err := svc.UpdateSettings(context.Background(), &SystemSettings{
		BackendModeEnabled: false,
	})
	require.NoError(t, err)
	require.Equal(t, "false", repo.updates[SettingKeyBackendModeEnabled])
	require.False(t, svc.IsBackendModeEnabled(context.Background()))
}
