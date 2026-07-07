//go:build unit

package service

import (
	"context"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

type settingInitRepoStub struct {
	values  map[string]string
	updates map[string]string
}

func (s *settingInitRepoStub) Get(context.Context, string) (*Setting, error) {
	panic("unexpected Get call")
}

func (s *settingInitRepoStub) GetValue(context.Context, string) (string, error) {
	panic("unexpected GetValue call")
}

func (s *settingInitRepoStub) Set(context.Context, string, string) error {
	panic("unexpected Set call")
}

func (s *settingInitRepoStub) GetMultiple(context.Context, []string) (map[string]string, error) {
	panic("unexpected GetMultiple call")
}

func (s *settingInitRepoStub) SetMultiple(_ context.Context, settings map[string]string) error {
	s.updates = make(map[string]string, len(settings))
	for key, value := range settings {
		s.updates[key] = value
		s.values[key] = value
	}
	return nil
}

func (s *settingInitRepoStub) GetAll(context.Context) (map[string]string, error) {
	result := make(map[string]string, len(s.values))
	for key, value := range s.values {
		result[key] = value
	}
	return result, nil
}

func (s *settingInitRepoStub) Delete(context.Context, string) error {
	panic("unexpected Delete call")
}

func TestSettingService_InitializeDefaultSettings_BackfillsWithoutOverwriting(t *testing.T) {
	repo := &settingInitRepoStub{
		values: map[string]string{
			SettingKeyRegistrationEnabled: "false",
			SettingKeySiteName:            "Custom Site",
		},
	}
	svc := NewSettingService(repo, &config.Config{})

	require.NoError(t, svc.InitializeDefaultSettings(context.Background()))

	require.NotContains(t, repo.updates, SettingKeyRegistrationEnabled)
	require.NotContains(t, repo.updates, SettingKeySiteName)
	require.Equal(t, "false", repo.values[SettingKeyRegistrationEnabled])
	require.Equal(t, "Custom Site", repo.values[SettingKeySiteName])
	require.Contains(t, repo.updates, SettingKeyPromoCodeEnabled)
	require.Contains(t, repo.updates, SettingKeyTableDefaultPageSize)
}

func TestSettingService_InitializeDefaultSettings_PreservesExistingDefaultKeys(t *testing.T) {
	repo := &settingInitRepoStub{
		values: map[string]string{
			SettingKeyRegistrationEnabled:  "false",
			SettingKeyPromoCodeEnabled:     "false",
			SettingKeyTableDefaultPageSize: "50",
		},
	}
	svc := NewSettingService(repo, &config.Config{})

	require.NoError(t, svc.InitializeDefaultSettings(context.Background()))

	require.NotContains(t, repo.updates, SettingKeyRegistrationEnabled)
	require.NotContains(t, repo.updates, SettingKeyPromoCodeEnabled)
	require.NotContains(t, repo.updates, SettingKeyTableDefaultPageSize)
	require.Equal(t, "false", repo.values[SettingKeyPromoCodeEnabled])
	require.Equal(t, "50", repo.values[SettingKeyTableDefaultPageSize])
}
