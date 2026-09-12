//go:build unit

package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestSettingHandler_GetPublicSettingsExposesCanvasOrigin(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/settings/public", nil)

	h := NewSettingHandler(
		service.NewSettingService(&settingHandlerPublicRepoStub{values: map[string]string{}}, &config.Config{
			Canvas: config.CanvasConfig{Origin: "https://canvas.example.test/"},
		}),
		"test-version",
	)
	h.GetPublicSettings(c)

	require.Equal(t, http.StatusOK, recorder.Code)
	var response struct {
		Data struct {
			CanvasOrigin string `json:"canvas_origin"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &response))
	require.Equal(t, "https://canvas.example.test", response.Data.CanvasOrigin)
}
