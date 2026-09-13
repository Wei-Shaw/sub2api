package service

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/config"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

const SettingKeyDingTalkApps = "dingtalk_connect_apps"

func (s *SettingService) GetDingTalkApps(ctx context.Context) ([]config.DingTalkAppConfig, error) {
	apps := []config.DingTalkAppConfig{}
	if s.cfg != nil {
		apps = append(apps, s.cfg.DingTalk.Apps...)
	}
	values, err := s.settingRepo.GetMultiple(ctx, []string{SettingKeyDingTalkApps})
	if err != nil {
		return nil, err
	}
	if raw, ok := values[SettingKeyDingTalkApps]; ok {
		if err := json.Unmarshal([]byte(raw), &apps); err != nil {
			return nil, fmt.Errorf("invalid DingTalk applications configuration: %w", err)
		}
	}
	if apps == nil {
		apps = []config.DingTalkAppConfig{}
	}
	return apps, nil
}

// Applications have immutable identity and are disabled rather than deleted:
// otherwise reusing an ID could inherit another organization's permissions.
func (s *SettingService) SaveDingTalkApps(ctx context.Context, apps []config.DingTalkAppConfig) error {
	old, err := s.GetDingTalkApps(ctx)
	if err != nil {
		return err
	}
	previous := map[string]config.DingTalkAppConfig{}
	for _, app := range old {
		previous[app.ID] = app
	}
	for i := range apps {
		app := &apps[i]
		app.ID, app.Name, app.ClientID = strings.TrimSpace(app.ID), strings.TrimSpace(app.Name), strings.TrimSpace(app.ClientID)
		app.ClientSecret, app.RedirectURL, app.CorpID = strings.TrimSpace(app.ClientSecret), strings.TrimSpace(app.RedirectURL), strings.TrimSpace(app.CorpID)
		if prev, ok := previous[app.ID]; ok {
			if prev.ClientID != app.ClientID || prev.CorpID != app.CorpID {
				return infraerrors.BadRequest("IMMUTABLE_APP_IDENTITY", "Existing application Client ID and corporation cannot be changed; disable it and add another application")
			}
			if app.ClientSecret == "" {
				app.ClientSecret = prev.ClientSecret
			}
			delete(previous, app.ID)
		}
		app.ClientSecretConfigured = false
	}
	if len(previous) != 0 {
		return infraerrors.BadRequest("APP_DELETE_FORBIDDEN", "Disable applications instead of deleting them")
	}
	if err := config.ValidateDingTalkApps(apps); err != nil {
		return infraerrors.BadRequest("INVALID_DINGTALK_APPS", err.Error())
	}
	raw, err := json.Marshal(apps)
	if err != nil {
		return err
	}
	if err := s.settingRepo.Set(ctx, SettingKeyDingTalkApps, string(raw)); err != nil {
		return err
	}
	if s.onUpdate != nil {
		s.onUpdate()
	}
	return nil
}

func RedactDingTalkApps(apps []config.DingTalkAppConfig) []config.DingTalkAppConfig {
	result := append([]config.DingTalkAppConfig{}, apps...)
	for i := range result {
		result[i].ClientSecretConfigured = result[i].ClientSecret != ""
		result[i].ClientSecret = ""
	}
	return result
}

func (s *SettingService) GetDingTalkOAuthConfigForApp(ctx context.Context, id string) (config.DingTalkConnectConfig, error) {
	if id == "" || id == "default" {
		return s.GetDingTalkConnectOAuthConfig(ctx)
	}
	apps, err := s.GetDingTalkApps(ctx)
	if err != nil {
		return config.DingTalkConnectConfig{}, err
	}
	for _, app := range apps {
		if app.ID != id {
			continue
		}
		if !app.Enabled {
			break
		}
		if s.cfg == nil {
			break
		}
		cfg := s.cfg.DingTalk
		cfg.Apps = nil
		cfg.Enabled, cfg.ClientID, cfg.ClientSecret, cfg.RedirectURL = true, app.ClientID, app.ClientSecret, app.RedirectURL
		cfg.InternalCorpID = app.CorpID
		cfg.DingTalkAppKind, cfg.AppType, cfg.CorpRestrictionPolicy = "internal_app", "internal", "internal_only"
		// New applications follow the site's registration policy.
		cfg.BypassRegistration = false
		if cfg.AuthorizeURL == "" {
			cfg.AuthorizeURL = "https://login.dingtalk.com/oauth2/auth"
		}
		if cfg.TokenURL == "" {
			cfg.TokenURL = "https://api.dingtalk.com/v1.0/oauth2/userAccessToken"
		}
		if cfg.UserInfoURL == "" {
			cfg.UserInfoURL = "https://api.dingtalk.com/v1.0/contact/users/me"
		}
		if cfg.FrontendRedirectURL == "" {
			cfg.FrontendRedirectURL = "/auth/dingtalk/callback"
		}
		if err := config.ValidateDingTalkApps([]config.DingTalkAppConfig{app}); err != nil {
			return config.DingTalkConnectConfig{}, err
		}
		return cfg, nil
	}
	return config.DingTalkConnectConfig{}, infraerrors.NotFound("DINGTALK_APP_DISABLED", "DingTalk application is missing or disabled")
}

// Bind registration policy to the server-stored pending application's identity.
type dingTalkApplicationContextKey struct{}

func WithDingTalkApplication(ctx context.Context, app string) context.Context {
	return context.WithValue(ctx, dingTalkApplicationContextKey{}, app)
}
