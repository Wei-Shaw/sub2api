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
	"github.com/tidwall/gjson"
)

type openAIHealthSettingsRepo struct{ settingHandlerRepoStub }

func (r *openAIHealthSettingsRepo) Set(_ context.Context, key, value string) error {
	r.values[key] = value
	return nil
}

func TestOpenAIHealthSettingsRoundTripAndValidation(t *testing.T) {
	gin.SetMode(gin.TestMode)
	repo := &openAIHealthSettingsRepo{settingHandlerRepoStub{values: map[string]string{}}}
	svc := service.NewSettingService(repo, &config.Config{})
	handler := NewSettingHandler(svc, nil, nil, nil, nil, nil, nil)
	router := gin.New()
	router.GET("/health", handler.GetOpenAIAPIKeyHealthBreakerSettings)
	router.PUT("/health", handler.UpdateOpenAIAPIKeyHealthBreakerSettings)
	request := func(method, body string) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(method, "/health", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		router.ServeHTTP(rec, req)
		return rec
	}
	before := request(http.MethodGet, "")
	require.Equal(t, http.StatusOK, before.Code)
	require.False(t, gjson.GetBytes(before.Body.Bytes(), "data.enabled").Bool())
	require.Empty(t, repo.values, "reading defaults must not enable the breaker")

	updated := request(http.MethodPut, `{"enabled":true,"window_minutes":10,"failure_threshold":3,"cooldown_minutes":2}`)
	require.Equal(t, http.StatusOK, updated.Code)
	read := request(http.MethodGet, "")
	require.True(t, gjson.GetBytes(read.Body.Bytes(), "data.enabled").Bool(), "saving must invalidate the cached disabled value")
	require.Equal(t, int64(3), gjson.GetBytes(read.Body.Bytes(), "data.failure_threshold").Int())
	stored := repo.values[service.SettingKeyOpenAIAPIKeyHealthBreakerSettings]
	for _, body := range []string{
		`{"enabled":true,"window_minutes":0,"failure_threshold":3,"cooldown_minutes":2}`,
		`{"enabled":true,"window_minutes":10,"failure_threshold":10001,"cooldown_minutes":2}`,
		`{"enabled":true,"window_minutes":10,"failure_threshold":3,"cooldown_minutes":61}`,
		`{"enabled":"yes"}`,
	} {
		require.Equal(t, http.StatusBadRequest, request(http.MethodPut, body).Code)
		require.Equal(t, stored, repo.values[service.SettingKeyOpenAIAPIKeyHealthBreakerSettings])
	}
	disabled := request(http.MethodPut, `{"enabled":false,"window_minutes":10,"failure_threshold":3,"cooldown_minutes":2}`)
	require.Equal(t, http.StatusOK, disabled.Code)
	require.False(t, gjson.GetBytes(disabled.Body.Bytes(), "data.enabled").Bool())
}
