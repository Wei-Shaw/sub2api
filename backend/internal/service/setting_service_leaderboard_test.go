//go:build unit

package service

import (
	"context"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

type leaderboardSettingsRepoStub struct {
	values  map[string]string
	updates map[string]string
}

func (s *leaderboardSettingsRepoStub) Get(ctx context.Context, key string) (*Setting, error) {
	panic("unexpected Get call")
}

func (s *leaderboardSettingsRepoStub) GetValue(ctx context.Context, key string) (string, error) {
	if value, ok := s.values[key]; ok {
		return value, nil
	}
	return "", ErrSettingNotFound
}

func (s *leaderboardSettingsRepoStub) Set(ctx context.Context, key, value string) error {
	panic("unexpected Set call")
}

func (s *leaderboardSettingsRepoStub) GetMultiple(ctx context.Context, keys []string) (map[string]string, error) {
	out := make(map[string]string, len(keys))
	for _, key := range keys {
		if value, ok := s.values[key]; ok {
			out[key] = value
		}
	}
	return out, nil
}

func (s *leaderboardSettingsRepoStub) SetMultiple(ctx context.Context, settings map[string]string) error {
	s.updates = make(map[string]string, len(settings))
	for key, value := range settings {
		s.updates[key] = value
	}
	return nil
}

func (s *leaderboardSettingsRepoStub) GetAll(ctx context.Context) (map[string]string, error) {
	out := make(map[string]string, len(s.values))
	for key, value := range s.values {
		out[key] = value
	}
	return out, nil
}

func (s *leaderboardSettingsRepoStub) Delete(ctx context.Context, key string) error {
	panic("unexpected Delete call")
}

func TestDailyTokenLeaderboardSettingDefaultsDisabled(t *testing.T) {
	svc := NewSettingService(&leaderboardSettingsRepoStub{values: map[string]string{}}, &config.Config{})

	require.False(t, svc.IsDailyTokenLeaderboardEnabled(context.Background()))
}

func TestDailyTokenLeaderboardSettingReadsStrictTrue(t *testing.T) {
	svc := NewSettingService(&leaderboardSettingsRepoStub{
		values: map[string]string{
			SettingKeyDailyTokenLeaderboardEnabled: "true",
		},
	}, &config.Config{})

	require.True(t, svc.IsDailyTokenLeaderboardEnabled(context.Background()))
}

func TestPublicAndSystemSettingsIncludeDailyTokenLeaderboardEnabled(t *testing.T) {
	svc := NewSettingService(&leaderboardSettingsRepoStub{
		values: map[string]string{
			SettingKeyDailyTokenLeaderboardEnabled: "true",
		},
	}, &config.Config{})

	publicSettings, err := svc.GetPublicSettings(context.Background())
	require.NoError(t, err)
	require.True(t, publicSettings.DailyTokenLeaderboardEnabled)

	systemSettings, err := svc.GetAllSettings(context.Background())
	require.NoError(t, err)
	require.True(t, systemSettings.DailyTokenLeaderboardEnabled)
}

func TestUpdateSettingsStoresDailyTokenLeaderboardEnabled(t *testing.T) {
	repo := &leaderboardSettingsRepoStub{values: map[string]string{}}
	svc := NewSettingService(repo, &config.Config{})

	err := svc.UpdateSettings(context.Background(), &SystemSettings{
		DailyTokenLeaderboardEnabled: true,
	})
	require.NoError(t, err)

	require.Equal(t, "true", repo.updates[SettingKeyDailyTokenLeaderboardEnabled])
}
