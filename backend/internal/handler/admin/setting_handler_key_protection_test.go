package admin

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type keyProtectionAdminRepo struct{ settingHandlerRepoStub }

func (r *keyProtectionAdminRepo) Set(_ context.Context, key, value string) error {
	r.values[key] = value
	return nil
}

func newKeyProtectionTestHandler(t *testing.T, stored map[string]string) (*SettingHandler, *keyProtectionAdminRepo) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	repo := &keyProtectionAdminRepo{settingHandlerRepoStub{values: stored}}
	platformConfig := &config.Config{}
	platformConfig.Totp.EncryptionKeyConfigured = true
	return NewSettingHandler(service.NewSettingService(repo, platformConfig), nil, nil, nil, nil, nil, nil), repo
}

func TestUpdateKeyProtectionConfigRequiresConfiguredPlatformKey(t *testing.T) {
	for _, test := range []struct {
		body   string
		status int
	}{
		{`{"enabled":true,"mode":"reversible"}`, http.StatusBadRequest},
		{`{"enabled":false,"mode":"reversible"}`, http.StatusOK},
		{`{"enabled":true,"mode":"redact"}`, http.StatusOK},
	} {
		t.Run(test.body, func(t *testing.T) {
			repo := &keyProtectionAdminRepo{settingHandlerRepoStub{values: map[string]string{}}}
			h := NewSettingHandler(service.NewSettingService(repo, &config.Config{}), nil, nil, nil, nil, nil, nil)
			rec := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(rec)
			c.Request = httptest.NewRequest(http.MethodPut, "/api/v1/admin/settings/key-protection", strings.NewReader(test.body))
			h.UpdateKeyProtectionConfig(c)
			require.Equal(t, test.status, rec.Code)
			if test.status == http.StatusBadRequest {
				require.Contains(t, rec.Body.String(), "KEY_PROTECTION_PLATFORM_KEY_REQUIRED")
				require.Empty(t, repo.values)
			}
		})
	}
}

func TestUpdateKeyProtectionConfigRejectsInvalidDocumentWithoutReplacingPolicy(t *testing.T) {
	for _, body := range []string{
		"null", "[]", "{broken", `{"enabled":true} {}`, `{"enabeld":true}`,
		`{"enabled":true,"mode":"unknown"}`, `{"custom_rules":[{"name":"example","pattern":"["}]}`,
		`{"enabled":true,"user_ids":[-1]}`, `{"custom_rules":[{"name":"example","pattern":"` + strings.Repeat("a", 65<<10) + `"}]}`,
	} {
		t.Run(body[:min(len(body), 40)], func(t *testing.T) {
			h, repo := newKeyProtectionTestHandler(t, map[string]string{service.SettingKeyKeyProtection: `{"enabled":true}`})
			rec := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(rec)
			c.Request = httptest.NewRequest(http.MethodPut, "/api/v1/admin/settings/key-protection", strings.NewReader(body))
			h.UpdateKeyProtectionConfig(c)
			require.Equal(t, http.StatusBadRequest, rec.Code)
			require.Equal(t, `{"enabled":true}`, repo.values[service.SettingKeyKeyProtection])
		})
	}
}

func TestKeyProtectionConfigAdminRoundTrip(t *testing.T) {
	h, repo := newKeyProtectionTestHandler(t, map[string]string{})
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPut, "/api/v1/admin/settings/key-protection", strings.NewReader(`{"enabled":true,"group_ids":[7],"restore_scope":"tools_only"}`))
	h.UpdateKeyProtectionConfig(c)
	require.Equal(t, http.StatusOK, rec.Code)
	require.Contains(t, repo.values[service.SettingKeyKeyProtection], `"enabled":true`)
	require.Contains(t, rec.Body.String(), `"mode":"reversible"`)
	require.Contains(t, rec.Body.String(), `"restore_scope":"tools_only"`)

	read := httptest.NewRecorder()
	get, _ := gin.CreateTestContext(read)
	get.Request = httptest.NewRequest(http.MethodGet, "/api/v1/admin/settings/key-protection", nil)
	h.GetKeyProtectionConfig(get)
	require.Equal(t, http.StatusOK, read.Code)
	require.JSONEq(t, rec.Body.String(), read.Body.String())
}

func TestKeyProtectionConfigAdminReadCorruptPolicy(t *testing.T) {
	h, _ := newKeyProtectionTestHandler(t, map[string]string{service.SettingKeyKeyProtection: "sensitive-corrupt-document"})
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/admin/settings/key-protection", nil)
	h.GetKeyProtectionConfig(c)
	require.Equal(t, http.StatusServiceUnavailable, rec.Code)
	require.NotContains(t, rec.Body.String(), "sensitive-corrupt-document")
}
