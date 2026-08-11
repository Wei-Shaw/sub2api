package admin

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func baseValidFeishuBody() map[string]any {
	return map[string]any{
		"feishu_connect_enabled":             true,
		"feishu_connect_app_id":              "test-client-id",
		"feishu_connect_app_secret":          "test-client-secret",
		"feishu_connect_redirect_url":        "https://example.com/auth/feishu/callback",
		"feishu_connect_restrict_tenant":     true,
		"feishu_connect_allowed_tenant_keys": "tenant-one",
	}
}

func putSettings(t *testing.T, handler *SettingHandler, body map[string]any) *httptest.ResponseRecorder {
	t.Helper()
	rawBody, err := json.Marshal(body)
	require.NoError(t, err)

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPut, "/api/v1/admin/settings", bytes.NewReader(rawBody))
	c.Request.Header.Set("Content-Type", "application/json")
	handler.UpdateSettings(c)
	return rec
}

func TestSettingsPUTFeishuBypassRegistrationRoundTrip(t *testing.T) {
	gin.SetMode(gin.TestMode)
	handler, repo := newDingTalkSettingsHandler()
	body := baseValidFeishuBody()
	body["feishu_connect_bypass_registration"] = true

	rec := putSettings(t, handler, body)
	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, "true", repo.values[service.SettingKeyFeishuConnectBypassRegistration])

	var resp response.Response
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	data, ok := resp.Data.(map[string]any)
	require.True(t, ok)
	require.Equal(t, true, data["feishu_connect_bypass_registration"])
}

func TestSettingsPUTFeishuBypassRegistrationWithoutTenantRestrictionIsDisabled(t *testing.T) {
	gin.SetMode(gin.TestMode)
	handler, repo := newDingTalkSettingsHandler()
	body := baseValidFeishuBody()
	body["feishu_connect_restrict_tenant"] = false
	body["feishu_connect_bypass_registration"] = true

	rec := putSettings(t, handler, body)
	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, "false", repo.values[service.SettingKeyFeishuConnectBypassRegistration])
}

func TestSettingsPUTFeishuRestrictedTenantRequiresAllowlist(t *testing.T) {
	gin.SetMode(gin.TestMode)
	handler, _ := newDingTalkSettingsHandler()
	body := baseValidFeishuBody()
	body["feishu_connect_allowed_tenant_keys"] = " , \n "
	body["feishu_connect_bypass_registration"] = true

	rec := putSettings(t, handler, body)
	require.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestSettingsPUTFeishuBypassRegistrationPartialUpdateUsesStoredTenantBoundary(t *testing.T) {
	gin.SetMode(gin.TestMode)
	handler, repo := newDingTalkSettingsHandler()

	rec := putSettings(t, handler, baseValidFeishuBody())
	require.Equal(t, http.StatusOK, rec.Code)

	rec = putSettings(t, handler, map[string]any{
		"feishu_connect_bypass_registration": true,
	})
	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, "true", repo.values[service.SettingKeyFeishuConnectBypassRegistration])
}
