//go:build unit

package service

import (
	"context"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

type settingWeChatRepoStub struct {
	values map[string]string
}

func (s *settingWeChatRepoStub) Get(context.Context, string) (*Setting, error) {
	panic("unexpected Get call")
}

func (s *settingWeChatRepoStub) GetValue(_ context.Context, key string) (string, error) {
	if value, ok := s.values[key]; ok {
		return value, nil
	}
	return "", ErrSettingNotFound
}

func (s *settingWeChatRepoStub) Set(context.Context, string, string) error {
	panic("unexpected Set call")
}

func (s *settingWeChatRepoStub) GetMultiple(_ context.Context, keys []string) (map[string]string, error) {
	out := make(map[string]string, len(keys))
	for _, key := range keys {
		if value, ok := s.values[key]; ok {
			out[key] = value
		}
	}
	return out, nil
}

func (s *settingWeChatRepoStub) SetMultiple(context.Context, map[string]string) error {
	panic("unexpected SetMultiple call")
}

func (s *settingWeChatRepoStub) GetAll(context.Context) (map[string]string, error) {
	panic("unexpected GetAll call")
}

func (s *settingWeChatRepoStub) Delete(context.Context, string) error {
	panic("unexpected Delete call")
}

func TestSettingService_GetWeChatConnectOAuthConfig_UsesDatabaseOverrides(t *testing.T) {
	repo := &settingWeChatRepoStub{
		values: map[string]string{
			SettingKeyWeChatConnectEnabled:             "true",
			SettingKeyWeChatConnectAppID:               "wx-db-app",
			SettingKeyWeChatConnectAppSecret:           "wx-db-secret",
			SettingKeyWeChatConnectMode:                "mp",
			SettingKeyWeChatConnectScopes:              "snsapi_base",
			SettingKeyWeChatConnectOpenEnabled:         "true",
			SettingKeyWeChatConnectMPEnabled:           "true",
			SettingKeyWeChatConnectRedirectURL:         "https://api.example.com/api/v1/auth/oauth/wechat/callback",
			SettingKeyWeChatConnectFrontendRedirectURL: "/auth/wechat/callback",
		},
	}
	svc := NewSettingService(repo, &config.Config{})

	got, err := svc.GetWeChatConnectOAuthConfig(context.Background())
	require.NoError(t, err)
	require.True(t, got.Enabled)
	require.Equal(t, "wx-db-app", got.AppIDForMode("mp"))
	require.Equal(t, "wx-db-secret", got.AppSecretForMode("mp"))
	require.True(t, got.OpenEnabled)
	require.True(t, got.MPEnabled)
	require.Equal(t, "mp", got.Mode)
	require.Equal(t, "snsapi_base", got.Scopes)
	require.Equal(t, "https://api.example.com/api/v1/auth/oauth/wechat/callback", got.RedirectURL)
	require.Equal(t, "/auth/wechat/callback", got.FrontendRedirectURL)
}

func TestSettingService_GetWeChatConnectOAuthConfig_FallsBackToConfigWhenDatabaseEmpty(t *testing.T) {
	repo := &settingWeChatRepoStub{values: map[string]string{}}
	svc := NewSettingService(repo, &config.Config{
		WeChat: config.WeChatConnectConfig{
			Enabled:             true,
			OpenEnabled:         true,
			MPEnabled:           true,
			Mode:                "open",
			OpenAppID:           "wx-open-config",
			OpenAppSecret:       "wx-open-secret",
			MPAppID:             "wx-mp-config",
			MPAppSecret:         "wx-mp-secret",
			FrontendRedirectURL: "/auth/wechat/config-callback",
		},
	})

	got, err := svc.GetWeChatConnectOAuthConfig(context.Background())
	require.NoError(t, err)
	require.True(t, got.Enabled)
	require.True(t, got.OpenEnabled)
	require.True(t, got.MPEnabled)
	require.Equal(t, "wx-open-config", got.AppIDForMode("open"))
	require.Equal(t, "wx-open-secret", got.AppSecretForMode("open"))
	require.Equal(t, "wx-mp-config", got.AppIDForMode("mp"))
	require.Equal(t, "wx-mp-secret", got.AppSecretForMode("mp"))
	require.Equal(t, "/auth/wechat/config-callback", got.FrontendRedirectURL)
	require.Empty(t, got.RedirectURL)
}

func TestSettingService_GetWeChatConnectOAuthConfig_IgnoresSyntheticDisabledCapabilitiesFromMigration118(t *testing.T) {
	repo := &settingWeChatRepoStub{
		values: map[string]string{
			SettingKeyWeChatConnectOpenEnabled: "false",
			SettingKeyWeChatConnectMPEnabled:   "false",
		},
	}
	svc := NewSettingService(repo, &config.Config{
		WeChat: config.WeChatConnectConfig{
			Enabled:             true,
			OpenEnabled:         true,
			MPEnabled:           true,
			Mode:                "open",
			OpenAppID:           "wx-open-config",
			OpenAppSecret:       "wx-open-secret",
			MPAppID:             "wx-mp-config",
			MPAppSecret:         "wx-mp-secret",
			FrontendRedirectURL: "/auth/wechat/config-callback",
		},
	})

	got, err := svc.GetWeChatConnectOAuthConfig(context.Background())
	require.NoError(t, err)
	require.True(t, got.Enabled)
	require.True(t, got.OpenEnabled)
	require.True(t, got.MPEnabled)
	require.Equal(t, "wx-open-config", got.AppIDForMode("open"))
	require.Equal(t, "wx-mp-config", got.AppIDForMode("mp"))
}

func TestSettingService_ParseSettings_FallsBackToConfigForWeChatAdminView(t *testing.T) {
	svc := NewSettingService(&settingWeChatRepoStub{values: map[string]string{}}, &config.Config{
		WeChat: config.WeChatConnectConfig{
			Enabled:             true,
			OpenEnabled:         true,
			Mode:                "open",
			OpenAppID:           "wx-open-config",
			OpenAppSecret:       "wx-open-secret",
			FrontendRedirectURL: "/auth/wechat/config-callback",
		},
	})

	got := svc.parseSettings(map[string]string{})
	require.True(t, got.WeChatConnectEnabled)
	require.True(t, got.WeChatConnectOpenEnabled)
	require.Equal(t, "wx-open-config", got.WeChatConnectOpenAppID)
	require.True(t, got.WeChatConnectOpenAppSecretConfigured)
	require.Equal(t, "/auth/wechat/config-callback", got.WeChatConnectFrontendRedirectURL)
	require.Equal(t, "open", got.WeChatConnectMode)
	require.Equal(t, "snsapi_login", got.WeChatConnectScopes)
}
