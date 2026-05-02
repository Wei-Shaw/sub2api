//go:build unit

package service

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"
)

// rectifierSettingsCacheStub 用 in-memory map 模拟跨实例缓存。
// 同时记录 Get/Set/Invalidate 调用次数以断言行为。
type rectifierSettingsCacheStub struct {
	value      *RectifierSettings
	getCalls   atomic.Int32
	setCalls   atomic.Int32
	invCalls   atomic.Int32
	getErr     error
	forceEmpty bool
}

func (c *rectifierSettingsCacheStub) Get(ctx context.Context) (*RectifierSettings, bool, error) {
	c.getCalls.Add(1)
	if c.getErr != nil {
		return nil, false, c.getErr
	}
	if c.forceEmpty || c.value == nil {
		return nil, false, nil
	}
	// 返回拷贝避免调用方修改后影响后续 Get
	copyVal := *c.value
	return &copyVal, true, nil
}

func (c *rectifierSettingsCacheStub) Set(ctx context.Context, settings *RectifierSettings) error {
	c.setCalls.Add(1)
	if settings == nil {
		c.value = nil
		return nil
	}
	copyVal := *settings
	c.value = &copyVal
	return nil
}

func (c *rectifierSettingsCacheStub) Invalidate(ctx context.Context) error {
	c.invCalls.Add(1)
	c.value = nil
	return nil
}

// writableSettingRepoStub 支持 Set 的 settingRepo 版本，专供 rectifier cache 测试。
// 与 settingRepoStub 分开定义避免污染其他已依赖"Set 会 panic"语义的测试。
type writableSettingRepoStub struct {
	values   map[string]string
	getCalls atomic.Int32
}

func (s *writableSettingRepoStub) Get(ctx context.Context, key string) (*Setting, error) {
	panic("unexpected Get call")
}

func (s *writableSettingRepoStub) GetValue(ctx context.Context, key string) (string, error) {
	s.getCalls.Add(1)
	if v, ok := s.values[key]; ok {
		return v, nil
	}
	return "", ErrSettingNotFound
}

func (s *writableSettingRepoStub) Set(ctx context.Context, key, value string) error {
	if s.values == nil {
		s.values = map[string]string{}
	}
	s.values[key] = value
	return nil
}

func (s *writableSettingRepoStub) GetMultiple(ctx context.Context, keys []string) (map[string]string, error) {
	result := map[string]string{}
	for _, k := range keys {
		if v, ok := s.values[k]; ok {
			result[k] = v
		}
	}
	return result, nil
}

func (s *writableSettingRepoStub) SetMultiple(ctx context.Context, settings map[string]string) error {
	panic("unexpected SetMultiple call")
}

func (s *writableSettingRepoStub) GetAll(ctx context.Context) (map[string]string, error) {
	panic("unexpected GetAll call")
}

func (s *writableSettingRepoStub) Delete(ctx context.Context, key string) error {
	panic("unexpected Delete call")
}

func TestGetRectifierSettings_CacheHit_SkipsDB(t *testing.T) {
	repo := &writableSettingRepoStub{values: map[string]string{
		SettingKeyRectifierSettings: `{"enabled":true,"advisor_tool_enabled":true,"advisor_tool_patterns":["db-value"]}`,
	}}
	cache := &rectifierSettingsCacheStub{value: &RectifierSettings{
		Enabled:             true,
		AdvisorToolEnabled:  true,
		AdvisorToolPatterns: []string{"cache-value"},
	}}
	svc := NewSettingService(repo, nil)
	svc.SetRectifierSettingsCache(cache)

	got, err := svc.GetRectifierSettings(context.Background())
	require.NoError(t, err)
	require.Equal(t, []string{"cache-value"}, got.AdvisorToolPatterns,
		"cache hit must return cached value, not DB")
	require.Equal(t, int32(0), repo.getCalls.Load(), "cache hit must NOT hit DB")
	require.Equal(t, int32(0), cache.setCalls.Load(), "cache hit must NOT refill (no TTL refresh)")
}

func TestGetRectifierSettings_CacheMiss_RefillsFromDB(t *testing.T) {
	repo := &writableSettingRepoStub{values: map[string]string{
		SettingKeyRectifierSettings: `{"enabled":true,"advisor_tool_enabled":true,"advisor_tool_patterns":["db-value"]}`,
	}}
	cache := &rectifierSettingsCacheStub{forceEmpty: true}
	svc := NewSettingService(repo, nil)
	svc.SetRectifierSettingsCache(cache)

	got, err := svc.GetRectifierSettings(context.Background())
	require.NoError(t, err)
	require.Equal(t, []string{"db-value"}, got.AdvisorToolPatterns)
	require.Equal(t, int32(1), repo.getCalls.Load(), "cache miss must hit DB once")
	require.Equal(t, int32(1), cache.setCalls.Load(), "cache miss must refill cache")
}

func TestGetRectifierSettings_CacheError_FallsThroughToDB(t *testing.T) {
	repo := &writableSettingRepoStub{values: map[string]string{
		SettingKeyRectifierSettings: `{"enabled":true,"advisor_tool_enabled":true}`,
	}}
	cache := &rectifierSettingsCacheStub{getErr: errors.New("redis down")}
	svc := NewSettingService(repo, nil)
	svc.SetRectifierSettingsCache(cache)

	got, err := svc.GetRectifierSettings(context.Background())
	require.NoError(t, err, "Redis 故障时不应传播错误到上层")
	require.True(t, got.Enabled)
	require.Equal(t, int32(1), repo.getCalls.Load(), "Redis 错误时必须 fall through 到 DB")
}

func TestSetRectifierSettings_InvalidatesCache(t *testing.T) {
	repo := &writableSettingRepoStub{values: map[string]string{}}
	cache := &rectifierSettingsCacheStub{value: &RectifierSettings{AdvisorToolEnabled: false}}
	svc := NewSettingService(repo, nil)
	svc.SetRectifierSettingsCache(cache)

	newSettings := &RectifierSettings{
		Enabled:             true,
		AdvisorToolEnabled:  true,
		AdvisorToolPatterns: []string{"new-value"},
	}
	require.NoError(t, svc.SetRectifierSettings(context.Background(), newSettings))

	require.Equal(t, int32(1), cache.invCalls.Load(), "SetRectifierSettings 必须 Invalidate 缓存")
	require.Nil(t, cache.value, "Invalidate 后 cache value 应为 nil")
	require.Contains(t, repo.values, SettingKeyRectifierSettings)
}

func TestGetRectifierSettings_NoCache_AlwaysHitsDB(t *testing.T) {
	// 不注入 cache 时，GetRectifierSettings 退化为每次直查 DB（向后兼容）。
	repo := &writableSettingRepoStub{values: map[string]string{
		SettingKeyRectifierSettings: `{"enabled":true,"advisor_tool_enabled":true}`,
	}}
	svc := NewSettingService(repo, nil)

	_, err := svc.GetRectifierSettings(context.Background())
	require.NoError(t, err)
	_, err = svc.GetRectifierSettings(context.Background())
	require.NoError(t, err)
	require.Equal(t, int32(2), repo.getCalls.Load(), "无缓存时每次调用都应查 DB")
}
