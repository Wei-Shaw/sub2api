//go:build unit

package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

const canvasTestOrigin = "https://canvas.example.test"
const canvasTestSecret = "canvas-test-bff-shared-secret-0123456789"

func newCanvasSSOHandler(t *testing.T) *AuthHandler {
	t.Helper()
	h := NewAuthHandler(&config.Config{Canvas: config.CanvasConfig{
		Origin:            canvasTestOrigin,
		BFFSharedSecret:   canvasTestSecret,
		LaunchCodeTTLSecs: 60,
	}}, nil, nil, nil, nil, nil, nil, nil)
	h.SetCanvasLaunchStore(newCanvasLaunchTestStore())
	return h
}

type canvasLaunchTestStore struct{ values map[string]string }

func newCanvasLaunchTestStore() *canvasLaunchTestStore {
	return &canvasLaunchTestStore{values: map[string]string{}}
}

func (s *canvasLaunchTestStore) SetNX(_ context.Context, key, value string, _ time.Duration) (bool, error) {
	if _, exists := s.values[key]; exists {
		return false, nil
	}
	s.values[key] = value
	return true, nil
}

func (s *canvasLaunchTestStore) GetDel(_ context.Context, key string) (string, error) {
	value, ok := s.values[key]
	if !ok {
		return "", service.ErrCanvasLaunchNotFound
	}
	delete(s.values, key)
	return value, nil
}

func canvasContext(method, target string, body any, headers map[string]string) (*gin.Context, *httptest.ResponseRecorder) {
	raw, _ := json.Marshal(body)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(method, target, bytes.NewReader(raw))
	c.Request.Header.Set("Content-Type", "application/json")
	for key, value := range headers {
		c.Request.Header.Set(key, value)
	}
	return c, w
}

func TestCanvasLaunchExchangeIsOneUseAndDoesNotExposeTokenAtIssue(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h := newCanvasSSOHandler(t)
	issue, issueResponse := canvasContext(http.MethodPost, "/api/v1/canvas/launch", map[string]string{"canvas_origin": canvasTestOrigin}, map[string]string{"Authorization": "Bearer dashboard-token"})
	h.CreateCanvasLaunch(issue)
	require.Equal(t, http.StatusOK, issueResponse.Code)
	require.NotContains(t, issueResponse.Body.String(), "dashboard-token")
	var issued struct {
		Data canvasLaunchResponse `json:"data"`
	}
	require.NoError(t, json.Unmarshal(issueResponse.Body.Bytes(), &issued))
	require.Len(t, issued.Data.LaunchCode, 43)
	require.Equal(t, 60, issued.Data.ExpiresIn)

	exchange, exchangeResponse := canvasContext(http.MethodPost, "/api/v1/canvas/launch/exchange", canvasLaunchExchangeRequest{LaunchCode: issued.Data.LaunchCode, CanvasOrigin: canvasTestOrigin}, map[string]string{"X-Canvas-BFF-Secret": canvasTestSecret})
	h.ExchangeCanvasLaunch(exchange)
	require.Equal(t, http.StatusOK, exchangeResponse.Code)
	var exchanged struct {
		Data canvasLaunchExchangeResponse `json:"data"`
	}
	require.NoError(t, json.Unmarshal(exchangeResponse.Body.Bytes(), &exchanged))
	require.Equal(t, "dashboard-token", exchanged.Data.AccessToken)

	reused, reusedResponse := canvasContext(http.MethodPost, "/api/v1/canvas/launch/exchange", canvasLaunchExchangeRequest{LaunchCode: issued.Data.LaunchCode, CanvasOrigin: canvasTestOrigin}, map[string]string{"X-Canvas-BFF-Secret": canvasTestSecret})
	h.ExchangeCanvasLaunch(reused)
	require.Equal(t, http.StatusUnauthorized, reusedResponse.Code)
}

func TestCanvasLaunchExchangeRejectsWrongSecretWithoutConsumingCode(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h := newCanvasSSOHandler(t)
	issue, issueResponse := canvasContext(http.MethodPost, "/api/v1/canvas/launch", canvasLaunchRequest{CanvasOrigin: canvasTestOrigin}, map[string]string{"Authorization": "Bearer dashboard-token"})
	h.CreateCanvasLaunch(issue)
	var issued struct {
		Data canvasLaunchResponse `json:"data"`
	}
	require.NoError(t, json.Unmarshal(issueResponse.Body.Bytes(), &issued))

	wrong, wrongResponse := canvasContext(http.MethodPost, "/api/v1/canvas/launch/exchange", canvasLaunchExchangeRequest{LaunchCode: issued.Data.LaunchCode, CanvasOrigin: canvasTestOrigin}, map[string]string{"X-Canvas-BFF-Secret": "wrong-canvas-bff-secret-0123456789"})
	h.ExchangeCanvasLaunch(wrong)
	require.Equal(t, http.StatusUnauthorized, wrongResponse.Code)

	correct, correctResponse := canvasContext(http.MethodPost, "/api/v1/canvas/launch/exchange", canvasLaunchExchangeRequest{LaunchCode: issued.Data.LaunchCode, CanvasOrigin: canvasTestOrigin}, map[string]string{"X-Canvas-BFF-Secret": canvasTestSecret})
	h.ExchangeCanvasLaunch(correct)
	require.Equal(t, http.StatusOK, correctResponse.Code)
}
