package service

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

func TestDingTalkAppsPersistenceSelectionAndRedaction(t *testing.T) {
	repo := &panelRateLimitSettingRepo{values: map[string]string{}}
	s := &SettingService{settingRepo: repo, cfg: &config.Config{}}
	app := config.DingTalkAppConfig{ID: "engineering", Name: "Engineering", ClientID: "client-a", ClientSecret: "secret-a", RedirectURL: "https://example.com/api/v1/auth/oauth/dingtalk/callback", Enabled: true}
	ctx := context.Background()
	require.NoError(t, s.SaveDingTalkApps(ctx, []config.DingTalkAppConfig{app}))
	apps, err := s.GetDingTalkApps(ctx)
	require.NoError(t, err)
	require.Equal(t, "secret-a", apps[0].ClientSecret)
	public := RedactDingTalkApps(apps)
	raw, err := json.Marshal(public)
	require.NoError(t, err)
	require.NotContains(t, string(raw), "secret-a")
	require.True(t, public[0].ClientSecretConfigured)
	require.Equal(t, "secret-a", apps[0].ClientSecret)
	public[0].Name = "Renamed"
	require.NoError(t, s.SaveDingTalkApps(ctx, public))
	cfg, err := s.GetDingTalkOAuthConfigForApp(ctx, "engineering")
	require.NoError(t, err)
	require.Equal(t, "client-a", cfg.ClientID)
	require.Equal(t, "secret-a", cfg.ClientSecret)
	require.Equal(t, "internal_only", cfg.CorpRestrictionPolicy)
	require.False(t, cfg.BypassRegistration)
	_, err = s.GetDingTalkOAuthConfigForApp(ctx, "unknown")
	require.Error(t, err)
	public[0].ClientID = "replacement"
	require.Error(t, s.SaveDingTalkApps(ctx, public))
	require.Error(t, s.SaveDingTalkApps(ctx, []config.DingTalkAppConfig{}))
	app.Enabled = false
	require.NoError(t, s.SaveDingTalkApps(ctx, []config.DingTalkAppConfig{app}))
	_, err = s.GetDingTalkOAuthConfigForApp(ctx, app.ID)
	require.Error(t, err)
	app.ID = "default"
	require.Error(t, s.SaveDingTalkApps(ctx, []config.DingTalkAppConfig{app}))
}
