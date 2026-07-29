package service

import (
	"context"
	"sync"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

type connectionRiskSettingRepo struct {
	mu            sync.Mutex
	values        map[string]string
	getValueErr   error
	getValueCalls int
}

func (r *connectionRiskSettingRepo) Get(_ context.Context, key string) (*Setting, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	value, ok := r.values[key]
	if !ok {
		return nil, ErrSettingNotFound
	}
	return &Setting{Key: key, Value: value}, nil
}

func (r *connectionRiskSettingRepo) GetValue(_ context.Context, key string) (string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.getValueCalls++
	if r.getValueErr != nil {
		return "", r.getValueErr
	}
	value, ok := r.values[key]
	if !ok {
		return "", ErrSettingNotFound
	}
	return value, nil
}

func (r *connectionRiskSettingRepo) Set(_ context.Context, key, value string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.values == nil {
		r.values = make(map[string]string)
	}
	r.values[key] = value
	return nil
}

func (r *connectionRiskSettingRepo) GetMultiple(_ context.Context, keys []string) (map[string]string, error) {
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

func (r *connectionRiskSettingRepo) SetMultiple(_ context.Context, settings map[string]string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.values == nil {
		r.values = make(map[string]string)
	}
	for key, value := range settings {
		r.values[key] = value
	}
	return nil
}

func (r *connectionRiskSettingRepo) GetAll(_ context.Context) (map[string]string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make(map[string]string, len(r.values))
	for key, value := range r.values {
		out[key] = value
	}
	return out, nil
}

func (r *connectionRiskSettingRepo) Delete(_ context.Context, key string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.values, key)
	return nil
}

func newConnectionRiskTestService(repo SettingRepository) *SettingService {
	return NewSettingService(repo, &config.Config{})
}

func TestGetConnectionRiskSettingsDefaults(t *testing.T) {
	svc := newConnectionRiskTestService(&connectionRiskSettingRepo{})

	settings, err := svc.GetConnectionRiskSettings(context.Background())
	require.NoError(t, err)
	require.Equal(t, DefaultConnectionRiskSettings(), settings)
	require.False(t, settings.Enabled)
	require.False(t, settings.EmitEnabled)
	require.True(t, settings.WorkerEnabled)
	require.True(t, settings.R7IncludeAdminActors)
	require.Equal(t, 0.1, settings.EmitSampleRateEvidence)
}

func TestGetConnectionRiskSettingsInvalidJSONFallsBack(t *testing.T) {
	repo := &connectionRiskSettingRepo{values: map[string]string{
		SettingKeyConnectionRiskSettings: "{not-json",
	}}
	svc := newConnectionRiskTestService(repo)

	settings, err := svc.GetConnectionRiskSettings(context.Background())
	require.NoError(t, err)
	require.Equal(t, DefaultConnectionRiskSettings(), settings)
}

func TestGetConnectionRiskSettingsNormalizesValues(t *testing.T) {
	repo := &connectionRiskSettingRepo{values: map[string]string{
		SettingKeyConnectionRiskSettings: `{
			"enabled":true,
			"emit_enabled":true,
			"emit_sample_rate_evidence": 2.5,
			"max_active_members": -1,
			"worker_interval_seconds": 10,
			"phase": "weird",
			"min_notify_severity": "nope",
			"r7_include_admin_actors": false
		}`,
	}}
	svc := newConnectionRiskTestService(repo)

	settings, err := svc.GetConnectionRiskSettings(context.Background())
	require.NoError(t, err)
	require.True(t, settings.Enabled)
	require.True(t, settings.EmitEnabled)
	require.Equal(t, 1.0, settings.EmitSampleRateEvidence)
	require.Equal(t, 50000, settings.MaxActiveMembers)
	require.Equal(t, 60, settings.WorkerIntervalSeconds)
	require.Equal(t, "observe", settings.Phase)
	require.Equal(t, "high", settings.MinNotifySeverity)
	require.False(t, settings.R7IncludeAdminActors)
}

func TestSetConnectionRiskSettingsRoundTripAndCacheRefresh(t *testing.T) {
	repo := &connectionRiskSettingRepo{}
	svc := newConnectionRiskTestService(repo)

	cached := svc.GetConnectionRiskSettingsCached(context.Background())
	require.Equal(t, *DefaultConnectionRiskSettings(), cached)

	want := DefaultConnectionRiskSettings()
	want.Enabled = true
	want.EmitEnabled = true
	want.NotifyEmail = true
	want.Rules.R2.UACount1h = 8
	require.NoError(t, svc.SetConnectionRiskSettings(context.Background(), want))

	cached = svc.GetConnectionRiskSettingsCached(context.Background())
	require.True(t, cached.Enabled)
	require.True(t, cached.EmitEnabled)
	require.True(t, cached.NotifyEmail)
	require.Equal(t, 8, cached.Rules.R2.UACount1h)

	stored, err := svc.GetConnectionRiskSettings(context.Background())
	require.NoError(t, err)
	require.True(t, stored.Enabled)
	require.Equal(t, 8, stored.Rules.R2.UACount1h)
}

func TestGetConnectionRiskSettingsCachedAvoidsRepeatedDBReads(t *testing.T) {
	repo := &connectionRiskSettingRepo{values: map[string]string{
		SettingKeyConnectionRiskSettings: `{"enabled":true,"emit_enabled":false,"worker_enabled":true}`,
	}}
	svc := newConnectionRiskTestService(repo)

	for i := 0; i < 5; i++ {
		settings := svc.GetConnectionRiskSettingsCached(context.Background())
		require.True(t, settings.Enabled)
		require.False(t, settings.EmitEnabled)
	}

	repo.mu.Lock()
	calls := repo.getValueCalls
	repo.mu.Unlock()
	require.Equal(t, 1, calls, "TTL 内应只读一次 DB")
}

func TestConnectionRiskFlagPrecedence(t *testing.T) {
	s := ConnectionRiskSettings{Enabled: true, EmitEnabled: true, WorkerEnabled: true}
	require.False(t, s.EffectiveEmitEnabled(false), "YAML master kill must win")
	require.False(t, s.EffectiveWorkerEnabled(false))

	s.Enabled = false
	require.False(t, s.EffectiveEmitEnabled(true), "settings.enabled=false disables emit")
	require.False(t, s.EffectiveWorkerEnabled(true))

	s.Enabled = true
	s.EmitEnabled = false
	require.False(t, s.EffectiveEmitEnabled(true), "emit_enabled gates hot path only")
	require.True(t, s.EffectiveWorkerEnabled(true), "worker can run while emit off")

	s.EmitEnabled = true
	s.WorkerEnabled = false
	require.True(t, s.EffectiveEmitEnabled(true))
	require.False(t, s.EffectiveWorkerEnabled(true))
}
