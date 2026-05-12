//go:build unit

package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type settingHandlerPublicRepoStub struct {
	values map[string]string
}

func (s *settingHandlerPublicRepoStub) Get(ctx context.Context, key string) (*service.Setting, error) {
	panic("unexpected Get call")
}

func (s *settingHandlerPublicRepoStub) GetValue(ctx context.Context, key string) (string, error) {
	if value, ok := s.values[key]; ok {
		return value, nil
	}
	return "", nil
}

func (s *settingHandlerPublicRepoStub) Set(ctx context.Context, key, value string) error {
	panic("unexpected Set call")
}

func (s *settingHandlerPublicRepoStub) GetMultiple(ctx context.Context, keys []string) (map[string]string, error) {
	out := make(map[string]string, len(keys))
	for _, key := range keys {
		if value, ok := s.values[key]; ok {
			out[key] = value
		}
	}
	return out, nil
}

func (s *settingHandlerPublicRepoStub) SetMultiple(ctx context.Context, settings map[string]string) error {
	panic("unexpected SetMultiple call")
}

func (s *settingHandlerPublicRepoStub) GetAll(ctx context.Context) (map[string]string, error) {
	panic("unexpected GetAll call")
}

func (s *settingHandlerPublicRepoStub) Delete(ctx context.Context, key string) error {
	panic("unexpected Delete call")
}

func TestSettingHandler_GetPublicSettings_ExposesForceEmailOnThirdPartySignup(t *testing.T) {
	gin.SetMode(gin.TestMode)

	repo := &settingHandlerPublicRepoStub{
		values: map[string]string{
			service.SettingKeyForceEmailOnThirdPartySignup: "true",
		},
	}
	h := NewSettingHandler(service.NewSettingService(repo, &config.Config{}), "test-version")

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/settings/public", nil)

	h.GetPublicSettings(c)

	require.Equal(t, http.StatusOK, recorder.Code)

	var resp struct {
		Code int `json:"code"`
		Data struct {
			ForceEmailOnThirdPartySignup bool `json:"force_email_on_third_party_signup"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &resp))
	require.Equal(t, 0, resp.Code)
	require.True(t, resp.Data.ForceEmailOnThirdPartySignup)
}

func TestSettingHandler_GetPublicSettings_ExposesWeChatOAuthModeCapabilities(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h := NewSettingHandler(service.NewSettingService(&settingHandlerPublicRepoStub{
		values: map[string]string{
			service.SettingKeyWeChatConnectEnabled:             "true",
			service.SettingKeyWeChatConnectAppID:               "wx-mp-app",
			service.SettingKeyWeChatConnectAppSecret:           "wx-mp-secret",
			service.SettingKeyWeChatConnectMode:                "mp",
			service.SettingKeyWeChatConnectScopes:              "snsapi_base",
			service.SettingKeyWeChatConnectOpenEnabled:         "true",
			service.SettingKeyWeChatConnectMPEnabled:           "true",
			service.SettingKeyWeChatConnectRedirectURL:         "https://api.example.com/api/v1/auth/oauth/wechat/callback",
			service.SettingKeyWeChatConnectFrontendRedirectURL: "/auth/wechat/callback",
		},
	}, &config.Config{}), "test-version")

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/settings/public", nil)

	h.GetPublicSettings(c)

	require.Equal(t, http.StatusOK, recorder.Code)

	var resp struct {
		Code int `json:"code"`
		Data struct {
			WeChatOAuthEnabled     bool `json:"wechat_oauth_enabled"`
			WeChatOAuthOpenEnabled bool `json:"wechat_oauth_open_enabled"`
			WeChatOAuthMPEnabled   bool `json:"wechat_oauth_mp_enabled"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &resp))
	require.Equal(t, 0, resp.Code)
	require.True(t, resp.Data.WeChatOAuthEnabled)
	require.True(t, resp.Data.WeChatOAuthOpenEnabled)
	require.True(t, resp.Data.WeChatOAuthMPEnabled)
}

func TestSettingHandler_GetPublicSettings_ExposesFrontendURL(t *testing.T) {
	gin.SetMode(gin.TestMode)

	cfg := &config.Config{}
	cfg.Server.FrontendURL = "https://example.com"
	h := NewSettingHandler(service.NewSettingService(&settingHandlerPublicRepoStub{
		values: map[string]string{},
	}, cfg), "test-version")

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/settings/public", nil)

	h.GetPublicSettings(c)

	require.Equal(t, http.StatusOK, recorder.Code)

	var resp struct {
		Code int `json:"code"`
		Data struct {
			FrontendURL string `json:"frontend_url"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &resp))
	require.Equal(t, 0, resp.Code)
	require.Equal(t, "https://example.com", resp.Data.FrontendURL)
}

func TestSettingHandler_GetPublicSettings_ExposesSEOFields(t *testing.T) {
	gin.SetMode(gin.TestMode)

	h := NewSettingHandler(service.NewSettingService(&settingHandlerPublicRepoStub{
		values: map[string]string{
			service.SettingKeySEODefaultTitle:       "DEFAULT_TITLE",
			service.SettingKeySEOHomeTitle:          "HOME_TITLE",
			service.SettingKeySEODefaultDescription: "DEFAULT_DESCRIPTION",
			service.SettingKeySEOHomeDescription:    "HOME_DESCRIPTION",
			service.SettingKeySEODefaultOGImage:     "/og/custom.png",
			service.SettingKeySEODefaultRobots:      "index, follow",
		},
	}, &config.Config{}), "test-version")

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/settings/public", nil)

	h.GetPublicSettings(c)

	require.Equal(t, http.StatusOK, recorder.Code)

	var resp struct {
		Code int `json:"code"`
		Data struct {
			SEODefaultTitle       string `json:"seo_default_title"`
			SEOHomeTitle          string `json:"seo_home_title"`
			SEODefaultDescription string `json:"seo_default_description"`
			SEOHomeDescription    string `json:"seo_home_description"`
			SEODefaultOGImage     string `json:"seo_default_og_image"`
			SEODefaultRobots      string `json:"seo_default_robots"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &resp))
	require.Equal(t, 0, resp.Code)
	require.Equal(t, "DEFAULT_TITLE", resp.Data.SEODefaultTitle)
	require.Equal(t, "HOME_TITLE", resp.Data.SEOHomeTitle)
	require.Equal(t, "DEFAULT_DESCRIPTION", resp.Data.SEODefaultDescription)
	require.Equal(t, "HOME_DESCRIPTION", resp.Data.SEOHomeDescription)
	require.Equal(t, "/og/custom.png", resp.Data.SEODefaultOGImage)
	require.Equal(t, "index, follow", resp.Data.SEODefaultRobots)
}
