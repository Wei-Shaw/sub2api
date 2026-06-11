//go:build unit

package service

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

type settingValueCacheRepoStub struct {
	mu       sync.Mutex
	values   map[string]string
	getCalls map[string]*atomic.Int32
	onGet    chan string
	release  <-chan struct{}
}

func newSettingValueCacheRepoStub(values map[string]string) *settingValueCacheRepoStub {
	if values == nil {
		values = map[string]string{}
	}
	return &settingValueCacheRepoStub{
		values:   values,
		getCalls: map[string]*atomic.Int32{},
	}
}

func (r *settingValueCacheRepoStub) Get(ctx context.Context, key string) (*Setting, error) {
	value, err := r.GetValue(ctx, key)
	if err != nil {
		return nil, err
	}
	return &Setting{Key: key, Value: value}, nil
}

func (r *settingValueCacheRepoStub) GetValue(ctx context.Context, key string) (string, error) {
	r.mu.Lock()
	counter := r.getCalls[key]
	if counter == nil {
		counter = &atomic.Int32{}
		r.getCalls[key] = counter
	}
	value, ok := r.values[key]
	r.mu.Unlock()

	counter.Add(1)
	if r.onGet != nil {
		select {
		case r.onGet <- key:
		default:
		}
	}
	if r.release != nil {
		select {
		case <-r.release:
		case <-ctx.Done():
			return "", ctx.Err()
		}
	}
	if !ok {
		return "", ErrSettingNotFound
	}
	return value, nil
}

func (r *settingValueCacheRepoStub) Set(ctx context.Context, key, value string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.values[key] = value
	return nil
}

func (r *settingValueCacheRepoStub) GetMultiple(ctx context.Context, keys []string) (map[string]string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make(map[string]string, len(keys))
	for _, key := range keys {
		if value, ok := r.values[key]; ok {
			out[key] = value
		}
	}
	return out, nil
}

func (r *settingValueCacheRepoStub) SetMultiple(ctx context.Context, settings map[string]string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	for key, value := range settings {
		r.values[key] = value
	}
	return nil
}

func (r *settingValueCacheRepoStub) GetAll(ctx context.Context) (map[string]string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make(map[string]string, len(r.values))
	for key, value := range r.values {
		out[key] = value
	}
	return out, nil
}

func (r *settingValueCacheRepoStub) Delete(ctx context.Context, key string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.values, key)
	return nil
}

func (r *settingValueCacheRepoStub) callsFor(key string) int32 {
	r.mu.Lock()
	counter := r.getCalls[key]
	r.mu.Unlock()
	if counter == nil {
		return 0
	}
	return counter.Load()
}

func TestSettingService_ValueCacheCollapsesRepeatedGetSiteName(t *testing.T) {
	repo := newSettingValueCacheRepoStub(map[string]string{
		SettingKeySiteName: "Cached Site",
	})
	svc := NewSettingService(repo, &config.Config{})

	for i := 0; i < 100; i++ {
		require.Equal(t, "Cached Site", svc.GetSiteName(context.Background()))
	}

	require.Equal(t, int32(1), repo.callsFor(SettingKeySiteName))
}

func TestSettingService_ValueCacheInvalidatesAfterUpdateSettings(t *testing.T) {
	repo := newSettingValueCacheRepoStub(map[string]string{
		SettingKeySiteName: "Old Site",
	})
	svc := NewSettingService(repo, &config.Config{})

	require.Equal(t, "Old Site", svc.GetSiteName(context.Background()))

	err := svc.UpdateSettings(context.Background(), &SystemSettings{
		SiteName: "New Site",
	})
	require.NoError(t, err)

	require.Equal(t, "New Site", svc.GetSiteName(context.Background()))
	require.Equal(t, int32(1), repo.callsFor(SettingKeySiteName))
}

func TestSettingService_ValueCacheSingleflightCollapsesConcurrentLoads(t *testing.T) {
	started := make(chan string, 1)
	release := make(chan struct{})
	repo := newSettingValueCacheRepoStub(map[string]string{
		SettingKeySiteName: "Concurrent Site",
	})
	repo.onGet = started
	repo.release = release
	svc := NewSettingService(repo, &config.Config{})

	errs := make(chan error, 2)
	go func() {
		require.Equal(t, "Concurrent Site", svc.GetSiteName(context.Background()))
		errs <- nil
	}()

	select {
	case key := <-started:
		require.Equal(t, SettingKeySiteName, key)
	case <-time.After(time.Second):
		t.Fatal("等待首次设置回源超时")
	}

	go func() {
		require.Equal(t, "Concurrent Site", svc.GetSiteName(context.Background()))
		errs <- nil
	}()
	time.Sleep(10 * time.Millisecond)

	close(release)
	require.NoError(t, <-errs)
	require.NoError(t, <-errs)
	require.Equal(t, int32(1), repo.callsFor(SettingKeySiteName))
}
