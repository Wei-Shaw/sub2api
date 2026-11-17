package admin

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type moderationEngineSettings struct {
	service.SettingRepository
	value string
}

func (r *moderationEngineSettings) GetValue(context.Context, string) (string, error) {
	return r.value, nil
}
func (r *moderationEngineSettings) Set(_ context.Context, _ string, value string) error {
	r.value = value
	return nil
}

func TestContentModerationEngineHandlerRoundTrip(t *testing.T) {
	gin.SetMode(gin.TestMode)
	settings := &moderationEngineSettings{value: `{"api_keys":["legacy-secret"],"model":"omni-moderation-latest"}`}
	svc := service.NewContentModerationService(settings, nil, nil, nil, nil, nil, nil, nil)
	h := NewContentModerationHandler(svc)
	router := gin.New()
	router.PUT("/config", h.UpdateConfig)
	router.GET("/config", h.GetConfig)
	body := `{"engine":"typesafe","engine_configs":{"typesafe":{"base_url":"https://api.typesafe.ai","model":"jev-latest","api_keys":["new-secret"],"thresholds":{"sexual":0.91}}}}`
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/config", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(w, req)
	require.Equal(t, 200, w.Code)
	require.Contains(t, w.Body.String(), `"engine":"typesafe"`)
	require.Contains(t, w.Body.String(), `"sexual":0.91`)
	require.NotContains(t, w.Body.String(), "new-secret")
	require.NotContains(t, w.Body.String(), "legacy-secret")
	w = httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/config", nil))
	require.Equal(t, 200, w.Code)
	require.Contains(t, w.Body.String(), `"engine_configs"`)
	require.Contains(t, w.Body.String(), `"model":"jev-latest"`)
	require.Contains(t, settings.value, "legacy-secret")
	require.Contains(t, settings.value, "new-secret")
}
