package admin

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestSettingsPUTPreservesOmittedExtraConcurrencySettings(t *testing.T) {
	gin.SetMode(gin.TestMode)
	repo := &settingHandlerRepoStub{values: map[string]string{
		service.SettingKeyDefaultConcurrency:                 "5",
		service.SettingKeyDefaultExtraConcurrency:            "4",
		service.SettingKeyExtraConcurrencyEnabled:            "true",
		service.SettingKeyExtraConcurrencyWaitTimeoutSeconds: "45",
		service.SettingKeyExtraConcurrencyReservePercent:     "25.5",
		service.SettingKeyExtraConcurrencyMinReservedSlots:   "3",
		service.SettingKeyExtraConcurrencyPlatformReserves:   `{"openai":{"reserve_percent":20,"min_reserved_slots":2}}`,
	}}
	svc := service.NewSettingService(repo, &config.Config{Default: config.DefaultConfig{UserConcurrency: 5}})
	handler := NewSettingHandler(svc, nil, nil, nil, nil, nil, nil)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPut, "/api/v1/admin/settings", bytes.NewBufferString(`{}`))
	c.Request.Header.Set("Content-Type", "application/json")

	handler.UpdateSettings(c)

	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	require.Equal(t, "4", repo.values[service.SettingKeyDefaultExtraConcurrency])
	require.Equal(t, "true", repo.values[service.SettingKeyExtraConcurrencyEnabled])
	require.Equal(t, "45", repo.values[service.SettingKeyExtraConcurrencyWaitTimeoutSeconds])
	require.Equal(t, "25.5", repo.values[service.SettingKeyExtraConcurrencyReservePercent])
	require.Equal(t, "3", repo.values[service.SettingKeyExtraConcurrencyMinReservedSlots])
	require.JSONEq(t, `{"openai":{"reserve_percent":20,"min_reserved_slots":2}}`, repo.values[service.SettingKeyExtraConcurrencyPlatformReserves])
}

func TestSettingsGETReturnsExtraConcurrencySettings(t *testing.T) {
	gin.SetMode(gin.TestMode)
	repo := &settingHandlerRepoStub{values: map[string]string{
		service.SettingKeyDefaultConcurrency:                 "5",
		service.SettingKeyDefaultExtraConcurrency:            "4",
		service.SettingKeyExtraConcurrencyEnabled:            "true",
		service.SettingKeyExtraConcurrencyWaitTimeoutSeconds: "45",
		service.SettingKeyExtraConcurrencyReservePercent:     "25.5",
		service.SettingKeyExtraConcurrencyMinReservedSlots:   "3",
		service.SettingKeyExtraConcurrencyPlatformReserves:   `{"openai":{"reserve_percent":20,"min_reserved_slots":2}}`,
	}}
	svc := service.NewSettingService(repo, &config.Config{Default: config.DefaultConfig{UserConcurrency: 5}})
	handler := NewSettingHandler(svc, nil, nil, nil, nil, nil, nil)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/admin/settings", nil)

	handler.GetSettings(c)

	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	var resp response.Response
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	data, ok := resp.Data.(map[string]any)
	require.True(t, ok)
	require.Equal(t, float64(4), data["default_extra_concurrency"])
	require.Equal(t, true, data["extra_concurrency_enabled"])
	require.Equal(t, float64(45), data["extra_concurrency_wait_timeout_seconds"])
	require.Equal(t, 25.5, data["extra_concurrency_reserve_percent"])
	require.Equal(t, float64(3), data["extra_concurrency_min_reserved_slots"])
	platformReserves, ok := data["extra_concurrency_platform_reserves"].(map[string]any)
	require.True(t, ok)
	openAIReserve, ok := platformReserves["openai"].(map[string]any)
	require.True(t, ok)
	require.Equal(t, float64(20), openAIReserve["reserve_percent"])
	require.Equal(t, float64(2), openAIReserve["min_reserved_slots"])
}

func TestSettingsPUTUpdatesExtraConcurrencySettingsAndClearsPlatformReserves(t *testing.T) {
	gin.SetMode(gin.TestMode)
	repo := &settingHandlerRepoStub{values: map[string]string{
		service.SettingKeyDefaultConcurrency:                 "5",
		service.SettingKeyDefaultExtraConcurrency:            "4",
		service.SettingKeyExtraConcurrencyEnabled:            "true",
		service.SettingKeyExtraConcurrencyWaitTimeoutSeconds: "45",
		service.SettingKeyExtraConcurrencyReservePercent:     "25.5",
		service.SettingKeyExtraConcurrencyMinReservedSlots:   "3",
		service.SettingKeyExtraConcurrencyPlatformReserves:   `{"openai":{"reserve_percent":20,"min_reserved_slots":2}}`,
	}}
	svc := service.NewSettingService(repo, &config.Config{Default: config.DefaultConfig{UserConcurrency: 5}})
	handler := NewSettingHandler(svc, nil, nil, nil, nil, nil, nil)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPut, "/api/v1/admin/settings", bytes.NewBufferString(`{
		"default_extra_concurrency": 0,
		"extra_concurrency_enabled": false,
		"extra_concurrency_wait_timeout_seconds": 1,
		"extra_concurrency_reserve_percent": 0,
		"extra_concurrency_min_reserved_slots": 0,
		"extra_concurrency_platform_reserves": {}
	}`))
	c.Request.Header.Set("Content-Type", "application/json")

	handler.UpdateSettings(c)

	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	require.Equal(t, "0", repo.values[service.SettingKeyDefaultExtraConcurrency])
	require.Equal(t, "false", repo.values[service.SettingKeyExtraConcurrencyEnabled])
	require.Equal(t, "1", repo.values[service.SettingKeyExtraConcurrencyWaitTimeoutSeconds])
	require.Equal(t, "0", repo.values[service.SettingKeyExtraConcurrencyReservePercent])
	require.Equal(t, "0", repo.values[service.SettingKeyExtraConcurrencyMinReservedSlots])
	require.JSONEq(t, `{}`, repo.values[service.SettingKeyExtraConcurrencyPlatformReserves])
	var resp response.Response
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	data, ok := resp.Data.(map[string]any)
	require.True(t, ok)
	require.Equal(t, float64(0), data["default_extra_concurrency"])
	require.Equal(t, false, data["extra_concurrency_enabled"])
	require.Equal(t, float64(1), data["extra_concurrency_wait_timeout_seconds"])
	require.Equal(t, float64(0), data["extra_concurrency_reserve_percent"])
	require.Equal(t, float64(0), data["extra_concurrency_min_reserved_slots"])
	require.Empty(t, data["extra_concurrency_platform_reserves"])
}

func TestDiffSettingsIncludesAllExtraConcurrencySettings(t *testing.T) {
	beforePercent := 20.0
	beforeSlots := 2
	afterPercent := 30.0
	afterSlots := 3
	before := &service.SystemSettings{
		ExtraConcurrencyWaitTimeoutSeconds: 30,
		ExtraConcurrencyReservePercent:     10,
		ExtraConcurrencyMinReservedSlots:   1,
		ExtraConcurrencyPlatformReserves: map[string]service.ExtraConcurrencyPlatformReserve{
			"openai": {ReservePercent: &beforePercent, MinReservedSlots: &beforeSlots},
		},
	}
	after := &service.SystemSettings{
		DefaultExtraConcurrency:            4,
		ExtraConcurrencyEnabled:            true,
		ExtraConcurrencyWaitTimeoutSeconds: 45,
		ExtraConcurrencyReservePercent:     25,
		ExtraConcurrencyMinReservedSlots:   3,
		ExtraConcurrencyPlatformReserves: map[string]service.ExtraConcurrencyPlatformReserve{
			"openai": {ReservePercent: &afterPercent, MinReservedSlots: &afterSlots},
		},
	}

	changed := diffSettings(before, after, nil, nil, UpdateSettingsRequest{})

	require.ElementsMatch(t, []string{
		service.SettingKeyDefaultExtraConcurrency,
		service.SettingKeyExtraConcurrencyEnabled,
		service.SettingKeyExtraConcurrencyWaitTimeoutSeconds,
		service.SettingKeyExtraConcurrencyReservePercent,
		service.SettingKeyExtraConcurrencyMinReservedSlots,
		service.SettingKeyExtraConcurrencyPlatformReserves,
	}, changed)
}

func TestAuditSettingsUpdateLogsExtraConcurrencyChanges(t *testing.T) {
	gin.SetMode(gin.TestMode)
	var output bytes.Buffer
	previousLogger := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&output, nil)))
	t.Cleanup(func() { slog.SetDefault(previousLogger) })

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Set(string(middleware.ContextKeyUser), middleware.AuthSubject{UserID: 73})
	c.Set(string(middleware.ContextKeyUserRole), service.RoleAdmin)
	before := &service.SystemSettings{}
	after := &service.SystemSettings{
		DefaultExtraConcurrency: 2,
		ExtraConcurrencyEnabled: true,
	}

	(&SettingHandler{}).auditSettingsUpdate(c, before, after, nil, nil, UpdateSettingsRequest{})

	var entry map[string]any
	require.NoError(t, json.Unmarshal(bytes.TrimSpace(output.Bytes()), &entry))
	require.Equal(t, "settings updated", entry["msg"])
	require.Equal(t, true, entry["audit"])
	require.Equal(t, float64(73), entry["user_id"])
	require.Equal(t, service.RoleAdmin, entry["role"])
	changed, ok := entry["changed"].([]any)
	require.True(t, ok)
	require.ElementsMatch(t, []any{
		service.SettingKeyDefaultExtraConcurrency,
		service.SettingKeyExtraConcurrencyEnabled,
	}, changed)
}
