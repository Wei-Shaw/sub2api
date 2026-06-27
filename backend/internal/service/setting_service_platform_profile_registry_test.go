//go:build unit

package service

import (
	"context"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/stretchr/testify/require"
)

type platformProfileRegistrySettingRepoStub struct {
	values      map[string]string
	setCalls    map[string]string
	setMultiple map[string]string
}

func (s *platformProfileRegistrySettingRepoStub) Get(ctx context.Context, key string) (*Setting, error) {
	panic("unexpected Get call")
}

func (s *platformProfileRegistrySettingRepoStub) GetValue(ctx context.Context, key string) (string, error) {
	if value, ok := s.values[key]; ok {
		return value, nil
	}
	return "", ErrSettingNotFound
}

func (s *platformProfileRegistrySettingRepoStub) Set(ctx context.Context, key, value string) error {
	if s.setCalls == nil {
		s.setCalls = map[string]string{}
	}
	s.setCalls[key] = value
	s.values[key] = value
	return nil
}

func (s *platformProfileRegistrySettingRepoStub) GetMultiple(ctx context.Context, keys []string) (map[string]string, error) {
	out := make(map[string]string, len(keys))
	for _, key := range keys {
		if value, ok := s.values[key]; ok {
			out[key] = value
		}
	}
	return out, nil
}

func (s *platformProfileRegistrySettingRepoStub) SetMultiple(ctx context.Context, settings map[string]string) error {
	s.setMultiple = make(map[string]string, len(settings))
	for key, value := range settings {
		s.setMultiple[key] = value
		s.values[key] = value
	}
	return nil
}

func (s *platformProfileRegistrySettingRepoStub) GetAll(ctx context.Context) (map[string]string, error) {
	out := make(map[string]string, len(s.values))
	for key, value := range s.values {
		out[key] = value
	}
	return out, nil
}

func (s *platformProfileRegistrySettingRepoStub) Delete(ctx context.Context, key string) error {
	delete(s.values, key)
	return nil
}

func TestSettingService_InitializeDefaultSettings_InsertsPlatformProfileRegistryForExistingDB(t *testing.T) {
	repo := &platformProfileRegistrySettingRepoStub{
		values: map[string]string{
			SettingKeyRegistrationEnabled: "true",
		},
	}
	svc := NewSettingService(repo, &config.Config{})

	require.NoError(t, svc.InitializeDefaultSettings(context.Background()))

	inserted, ok := repo.setCalls[SettingKeyPlatformProfileRegistry]
	require.True(t, ok)
	require.Contains(t, inserted, `"platform": "openai"`)
	require.Contains(t, inserted, `"platform": "anthropic"`)
	require.Contains(t, inserted, `"platform": "gemini"`)
	require.Empty(t, repo.setMultiple, "existing DB path must only insert the missing registry setting")
}

func TestSettingService_UpdateSettings_PersistsPlatformProfileRegistryToSettingsTable(t *testing.T) {
	repo := &settingUpdateRepoStub{}
	svc := NewSettingService(repo, &config.Config{})

	err := svc.UpdateSettings(context.Background(), &SystemSettings{
		PlatformProfileRegistry: `{
			"version": 1,
			"profiles": [{
				"platform": "OpenAI",
				"guide": {
					"title": "DB guide",
					"description": "editable metadata",
					"copy_blocks": [{
						"id": "block-1",
						"client_id": "codex",
						"path": "~/.codex/config.toml",
						"content_template": "{{base_url}} {{api_key}}"
					}]
				}
			}]
		}`,
	})
	require.NoError(t, err)

	stored, ok := repo.updates[SettingKeyPlatformProfileRegistry]
	require.True(t, ok)
	require.Contains(t, stored, `"platform": "openai"`)
	require.Contains(t, stored, `"title": "DB guide"`)
	require.Contains(t, stored, `"content_template": "{{base_url}} {{api_key}}"`)
}

func TestSettingService_UpdateSettings_RejectsInvalidPlatformProfileRegistry(t *testing.T) {
	repo := &settingUpdateRepoStub{}
	svc := NewSettingService(repo, &config.Config{})

	err := svc.UpdateSettings(context.Background(), &SystemSettings{
		PlatformProfileRegistry: `{"version":1,"profiles":[]}`,
	})

	require.Error(t, err)
	require.Equal(t, "INVALID_PLATFORM_PROFILE_REGISTRY", infraerrors.Reason(err))
	require.Nil(t, repo.updates)
}
