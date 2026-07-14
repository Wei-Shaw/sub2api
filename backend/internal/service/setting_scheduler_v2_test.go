package service

import (
	"context"
	"errors"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

type schedulerV2SettingRepo struct {
	values map[string]string
}

func (r *schedulerV2SettingRepo) Get(_ context.Context, key string) (*Setting, error) {
	value, ok := r.values[key]
	if !ok {
		return nil, ErrSettingNotFound
	}
	return &Setting{Key: key, Value: value}, nil
}

func (r *schedulerV2SettingRepo) GetValue(_ context.Context, key string) (string, error) {
	value, ok := r.values[key]
	if !ok {
		return "", ErrSettingNotFound
	}
	return value, nil
}

func (r *schedulerV2SettingRepo) Set(_ context.Context, key, value string) error {
	r.values[key] = value
	return nil
}

func (r *schedulerV2SettingRepo) GetMultiple(_ context.Context, keys []string) (map[string]string, error) {
	result := make(map[string]string)
	for _, key := range keys {
		if value, ok := r.values[key]; ok {
			result[key] = value
		}
	}
	return result, nil
}

func (r *schedulerV2SettingRepo) SetMultiple(_ context.Context, values map[string]string) error {
	for key, value := range values {
		r.values[key] = value
	}
	return nil
}

func (r *schedulerV2SettingRepo) GetAll(context.Context) (map[string]string, error) {
	result := make(map[string]string, len(r.values))
	for key, value := range r.values {
		result[key] = value
	}
	return result, nil
}

func (r *schedulerV2SettingRepo) Delete(_ context.Context, key string) error {
	delete(r.values, key)
	return nil
}

type schedulerV2SwitcherStub struct {
	state          SchedulerEngineState
	calls          []bool
	err            error
	candidateLimit int
	scanLimit      int
	limitsErr      error
}

func (s *schedulerV2SwitcherStub) SetSchedulerV2Enabled(_ context.Context, enabled bool, candidateLimit, scanLimit int) error {
	s.calls = append(s.calls, enabled)
	if s.err != nil {
		return s.err
	}
	s.candidateLimit = candidateLimit
	s.scanLimit = scanLimit
	if enabled {
		s.state = SchedulerEngineState{Engine: SchedulerEngineV2, Status: SchedulerEngineStatusBuilding}
	} else {
		s.state = SchedulerEngineState{Engine: SchedulerEngineLegacy, Status: SchedulerEngineStatusDisabled}
	}
	return nil
}

func (s *schedulerV2SwitcherStub) SchedulerEngineState(context.Context) SchedulerEngineState {
	return s.state
}

func (s *schedulerV2SwitcherStub) ConfigureSchedulerV2Limits(_ context.Context, candidateLimit, scanLimit int) error {
	if s.limitsErr != nil {
		return s.limitsErr
	}
	s.candidateLimit = candidateLimit
	s.scanLimit = scanLimit
	return nil
}

func (s *schedulerV2SwitcherStub) SchedulerV2Limits() (int, int) {
	if ValidateSchedulerV2Limits(s.candidateLimit, s.scanLimit) != nil {
		return DefaultSchedulerCandidateFetchLimit, DefaultSchedulerCandidateScanLimit
	}
	return s.candidateLimit, s.scanLimit
}

func TestSettingServiceSchedulerV2_OverlayAndSwitchOnlyOnChange(t *testing.T) {
	repo := &schedulerV2SettingRepo{values: map[string]string{SettingKeySchedulerV2Enabled: "false"}}
	switcher := &schedulerV2SwitcherStub{state: SchedulerEngineState{Engine: SchedulerEngineLegacy, Status: SchedulerEngineStatusDisabled}}
	svc := NewSettingService(repo, &config.Config{})
	svc.SetSchedulerEngineSwitcher(switcher)

	settings, err := svc.GetAllSettings(context.Background())
	require.NoError(t, err)
	require.False(t, settings.SchedulerV2Enabled)
	require.Equal(t, SchedulerEngineStatusDisabled, settings.SchedulerV2Status)
	require.Equal(t, DefaultSchedulerCandidateFetchLimit, settings.SchedulerV2CandidateLimit)
	require.Equal(t, DefaultSchedulerCandidateScanLimit, settings.SchedulerV2ScanLimit)

	settings.SchedulerV2Enabled = true
	settings.SchedulerV2CandidateLimit = 32
	settings.SchedulerV2ScanLimit = 128
	require.NoError(t, svc.UpdateSettings(context.Background(), settings))
	require.Equal(t, []bool{true}, switcher.calls)
	require.Equal(t, "true", repo.values[SettingKeySchedulerV2Enabled])
	require.Equal(t, "32", repo.values[SettingKeySchedulerV2CandidateLimit])
	require.Equal(t, "128", repo.values[SettingKeySchedulerV2ScanLimit])
	require.Equal(t, 32, switcher.candidateLimit)
	require.Equal(t, 128, switcher.scanLimit)

	updated, err := svc.GetAllSettings(context.Background())
	require.NoError(t, err)
	require.True(t, updated.SchedulerV2Enabled)
	require.Equal(t, SchedulerEngineStatusBuilding, updated.SchedulerV2Status)
	require.NoError(t, svc.UpdateSettings(context.Background(), updated))
	require.Equal(t, []bool{true}, switcher.calls, "saving unrelated settings must not rebuild the scheduler")
}

func TestSettingServiceSchedulerV2_FailedStateCanBeRetried(t *testing.T) {
	repo := &schedulerV2SettingRepo{values: map[string]string{SettingKeySchedulerV2Enabled: "true"}}
	switcher := &schedulerV2SwitcherStub{state: SchedulerEngineState{
		Engine:    SchedulerEngineV2,
		Status:    SchedulerEngineStatusFailed,
		LastError: "redis unavailable",
	}}
	svc := NewSettingService(repo, &config.Config{})
	svc.SetSchedulerEngineSwitcher(switcher)
	settings, err := svc.GetAllSettings(context.Background())
	require.NoError(t, err)

	require.NoError(t, svc.UpdateSettings(context.Background(), settings))
	require.Equal(t, []bool{true}, switcher.calls)
}

func TestSettingServiceSchedulerV2_RuntimeFailureRollsBackPersistedTarget(t *testing.T) {
	repo := &schedulerV2SettingRepo{values: map[string]string{SettingKeySchedulerV2Enabled: "false"}}
	switcher := &schedulerV2SwitcherStub{
		state: SchedulerEngineState{Engine: SchedulerEngineLegacy, Status: SchedulerEngineStatusDisabled},
		err:   errors.New("redis unavailable"),
	}
	svc := NewSettingService(repo, &config.Config{})
	svc.SetSchedulerEngineSwitcher(switcher)
	settings, err := svc.GetAllSettings(context.Background())
	require.NoError(t, err)
	settings.SchedulerV2Enabled = true

	err = svc.UpdateSettings(context.Background(), settings)
	require.ErrorContains(t, err, "switch scheduler engine")
	require.Equal(t, "false", repo.values[SettingKeySchedulerV2Enabled])
}

func TestSettingServiceSchedulerV2_RejectsInvalidLimitsBeforePersisting(t *testing.T) {
	repo := &schedulerV2SettingRepo{values: map[string]string{SettingKeySchedulerV2Enabled: "false"}}
	switcher := &schedulerV2SwitcherStub{state: SchedulerEngineState{Engine: SchedulerEngineLegacy, Status: SchedulerEngineStatusDisabled}}
	svc := NewSettingService(repo, &config.Config{})
	svc.SetSchedulerEngineSwitcher(switcher)
	settings, err := svc.GetAllSettings(context.Background())
	require.NoError(t, err)
	settings.SchedulerV2CandidateLimit = 65
	settings.SchedulerV2ScanLimit = 64

	err = svc.UpdateSettings(context.Background(), settings)
	require.ErrorContains(t, err, "scan limit must be greater than or equal to candidate limit")
	require.NotContains(t, repo.values, SettingKeySchedulerV2CandidateLimit)
	require.Zero(t, switcher.candidateLimit)
}

func TestSettingServiceSchedulerV2_LimitPublishFailureRollsBackPersistedValues(t *testing.T) {
	repo := &schedulerV2SettingRepo{values: map[string]string{
		SettingKeySchedulerV2Enabled:        "true",
		SettingKeySchedulerV2CandidateLimit: "64",
		SettingKeySchedulerV2ScanLimit:      "256",
	}}
	switcher := &schedulerV2SwitcherStub{
		state:          SchedulerEngineState{Engine: SchedulerEngineV2, Status: SchedulerEngineStatusActive},
		candidateLimit: 64,
		scanLimit:      256,
		limitsErr:      errors.New("redis unavailable"),
	}
	svc := NewSettingService(repo, &config.Config{})
	svc.SetSchedulerEngineSwitcher(switcher)
	settings, err := svc.GetAllSettings(context.Background())
	require.NoError(t, err)
	settings.SchedulerV2CandidateLimit = 32
	settings.SchedulerV2ScanLimit = 128

	err = svc.UpdateSettings(context.Background(), settings)
	require.ErrorContains(t, err, "configure scheduler v2 limits")
	require.Equal(t, "64", repo.values[SettingKeySchedulerV2CandidateLimit])
	require.Equal(t, "256", repo.values[SettingKeySchedulerV2ScanLimit])
}
