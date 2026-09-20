package admin

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

func TestSettingsCodexTicketStatusUsesRuntimeConfigWithoutSecrets(t *testing.T) {
	gin.SetMode(gin.TestMode)
	repo := &settingHandlerRepoStub{values: map[string]string{
		service.SettingKeyOpenAICodexTicketEnabled:         "true",
		service.SettingKeyOpenAICodexTicketHarvestProxyURL: "http://user:secret@proxy.example:8080",
	}}
	cfg := &config.Config{Default: config.DefaultConfig{UserConcurrency: 5}}
	cfg.Gateway.OpenAICodexTicket = config.OpenAICodexTicketConfig{
		RefreshBeforeSeconds: 1200, TTLSeconds: 3600, Models: []string{"gpt-6-astra", "gpt-5.6-sol"},
	}
	h := NewSettingHandler(service.NewSettingService(repo, cfg), nil, nil, nil, nil, nil, nil)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/admin/settings/ticket-harvest-status", nil)
	h.GetTicketHarvestStatus(c)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	var result struct {
		Data service.OpenAICodexTicketRuntimeStatus `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &result))
	require.True(t, result.Data.Enabled)
	require.Equal(t, 1200, result.Data.RefreshBeforeSeconds)
	require.Equal(t, cfg.Gateway.OpenAICodexTicket.Models, result.Data.Models)
	require.True(t, result.Data.HarvestProxyConfigured)
	require.NotContains(t, rec.Body.String(), "secret")
	require.NotContains(t, rec.Body.String(), "proxy.example")
}

func TestSettingsCodexTicketToggleDoesNotOverwriteUnrelatedSettings(t *testing.T) {
	h, repo := newStepUpSwitchTestHandler(t, map[string]string{
		"site_name": "Existing site", "registration_enabled": "true",
		service.SettingKeyOpenAICodexTicketHarvestProxyURL: "http://proxy.example:8080",
	})
	rec := doUpdateSettings(t, h, map[string]any{service.SettingKeyOpenAICodexTicketEnabled: true}, nil)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	require.Equal(t, "true", repo.values[service.SettingKeyOpenAICodexTicketEnabled])
	require.Equal(t, "Existing site", repo.values["site_name"])
	require.Equal(t, "true", repo.values["registration_enabled"])
	require.Equal(t, "http://proxy.example:8080", repo.values[service.SettingKeyOpenAICodexTicketHarvestProxyURL])
}

func TestSettingsCodexTicketProxyWriteReadAndHotReload(t *testing.T) {
	key := service.SettingKeyOpenAICodexTicketHarvestProxyURL
	oldProxy := "http://user:old-secret@old.example.com:8080"
	newProxy := "socks5h://user:new-secret@new.example.com:1080"
	h, repo := newStepUpSwitchTestHandler(t, map[string]string{key: oldProxy})
	require.Equal(t, oldProxy, h.settingService.GetOpenAICodexTicketHarvestProxyURL(context.Background()))
	rec := doUpdateSettings(t, h, map[string]any{key: newProxy}, nil)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	require.Equal(t, newProxy, repo.values[key])
	require.Equal(t, newProxy, h.settingService.GetOpenAICodexTicketHarvestProxyURL(context.Background()))
	require.NotContains(t, rec.Body.String(), "new-secret")
	require.Contains(t, rec.Body.String(), `"openai_codex_ticket_harvest_proxy_configured":true`)
	// Omission, empty input and the masked GET value all preserve the real secret.
	for _, body := range []map[string]any{{"site_name": "updated"}, {key: ""}, {key: service.MaskProxyURL(newProxy)}} {
		rec = doUpdateSettings(t, h, body, nil)
		require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
		require.Equal(t, newProxy, repo.values[key])
	}
	get := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(get)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/admin/settings", nil)
	h.GetSettings(c)
	require.Equal(t, http.StatusOK, get.Code)
	require.NotContains(t, get.Body.String(), "new-secret")
	require.Contains(t, get.Body.String(), "new.example.com")
}

func TestSettingsCodexTicketRejectInvalidProxyWithoutLeakingPassword(t *testing.T) {
	key := service.SettingKeyOpenAICodexTicketHarvestProxyURL
	h, repo := newStepUpSwitchTestHandler(t, map[string]string{key: "http://previous.example.com:8080"})
	rec := doUpdateSettings(t, h, map[string]any{key: "ftp://user:invalid-secret@proxy.example.com:21"}, nil)
	require.Equal(t, http.StatusBadRequest, rec.Code, rec.Body.String())
	require.NotContains(t, rec.Body.String(), "invalid-secret")
	require.Equal(t, "http://previous.example.com:8080", repo.values[key])
}
