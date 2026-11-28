//go:build unit

package admin

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/handler/dto"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

const legacyRectifierSettingsJSON = `{"enabled":true,"thinking_signature_enabled":true,"thinking_budget_enabled":false,"apikey_signature_enabled":false,"apikey_signature_patterns":null}`

type rectifierSettingRepoStub struct {
	settingHandlerRepoStub
}

func (s *rectifierSettingRepoStub) Set(_ context.Context, key, value string) error {
	if s.values == nil {
		s.values = map[string]string{}
	}
	s.values[key] = value
	return nil
}

func newRectifierSettingHandlerForTest(stored string) (*SettingHandler, *rectifierSettingRepoStub) {
	gin.SetMode(gin.TestMode)
	repo := &rectifierSettingRepoStub{}
	if stored != "" {
		repo.values = map[string]string{service.SettingKeyRectifierSettings: stored}
	}
	svc := service.NewSettingService(repo, &config.Config{})
	return NewSettingHandler(svc, nil, nil, nil, nil, nil, nil), repo
}

func decodeRectifierSettingsResponse(t *testing.T, rec *httptest.ResponseRecorder) dto.RectifierSettings {
	t.Helper()
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	var resp struct {
		Data dto.RectifierSettings `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	return resp.Data
}

func putRectifierSettings(t *testing.T, handler *SettingHandler, body map[string]any) dto.RectifierSettings {
	t.Helper()
	raw, err := json.Marshal(body)
	require.NoError(t, err)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPut, "/api/v1/admin/settings/rectifier", bytes.NewReader(raw))
	c.Request.Header.Set("Content-Type", "application/json")
	handler.UpdateRectifierSettings(c)
	return decodeRectifierSettingsResponse(t, rec)
}

func TestGetRectifierSettings_LegacyJSONDefaultsMaxTokensEnabled(t *testing.T) {
	handler, _ := newRectifierSettingHandlerForTest(legacyRectifierSettingsJSON)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/admin/settings/rectifier", nil)
	handler.GetRectifierSettings(c)

	got := decodeRectifierSettingsResponse(t, rec)
	require.True(t, got.Enabled)
	require.True(t, got.ThinkingSignatureEnabled)
	require.False(t, got.ThinkingBudgetEnabled, "stored values must still win over defaults")
	require.True(t, got.MaxTokensEnabled, "field missing from legacy JSON must default to enabled")
	require.NotNil(t, got.APIKeySignaturePatterns)
}

func TestUpdateRectifierSettings_MaxTokensEnabledRoundTrip(t *testing.T) {
	handler, repo := newRectifierSettingHandlerForTest(legacyRectifierSettingsJSON)
	got := putRectifierSettings(t, handler, map[string]any{
		"enabled":                    true,
		"thinking_signature_enabled": true,
		"thinking_budget_enabled":    true,
		"max_tokens_enabled":         false,
	})
	require.False(t, got.MaxTokensEnabled)
	require.Contains(t, repo.values[service.SettingKeyRectifierSettings], `"max_tokens_enabled":false`)

	got = putRectifierSettings(t, handler, map[string]any{
		"enabled":            true,
		"max_tokens_enabled": true,
	})
	require.True(t, got.MaxTokensEnabled)
}

func TestUpdateRectifierSettings_OmittedMaxTokensEnabledKeepsSavedValue(t *testing.T) {
	handler, _ := newRectifierSettingHandlerForTest(`{"enabled":true,"max_tokens_enabled":false}`)
	got := putRectifierSettings(t, handler, map[string]any{
		"enabled":                    true,
		"thinking_signature_enabled": true,
		"thinking_budget_enabled":    true,
	})
	require.False(t, got.MaxTokensEnabled, "clients that do not know the field must not flip it")

	handler, _ = newRectifierSettingHandlerForTest(legacyRectifierSettingsJSON)
	got = putRectifierSettings(t, handler, map[string]any{
		"enabled":                    true,
		"thinking_signature_enabled": true,
		"thinking_budget_enabled":    true,
	})
	require.True(t, got.MaxTokensEnabled, "legacy settings default to enabled and stay enabled")
}
