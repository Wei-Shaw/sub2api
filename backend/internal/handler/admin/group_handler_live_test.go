package admin

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"runtime"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestLiveCapabilitySeparatesLocalSupportFromUpstreamVerification(t *testing.T) {
	handler := &GroupHandler{cfg: &config.Config{}}
	handler.cfg.Gateway.Live.UpstreamAuthMode = config.LiveUpstreamAuthModeCodexOAuth
	router := gin.New()
	router.GET("/live-capability", handler.GetLiveCapability)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/live-capability", nil))
	require.Equal(t, http.StatusOK, response.Code)
	var body struct {
		Data struct {
			Supported        bool   `json:"supported"`
			Mode             string `json:"upstream_auth_mode"`
			Experimental     bool   `json:"experimental"`
			UpstreamVerified *bool  `json:"upstream_verified"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(response.Body.Bytes(), &body))
	require.True(t, body.Data.Supported)
	require.Equal(t, config.LiveUpstreamAuthModeCodexOAuth, body.Data.Mode)
	require.True(t, body.Data.Experimental)
	require.NotNil(t, body.Data.UpstreamVerified)
	require.False(t, *body.Data.UpstreamVerified)
}

func TestLiveCapabilityPreservesPlatformGuardByDefault(t *testing.T) {
	if runtime.GOOS == "darwin" {
		t.Skip("DeviceCheck availability on macOS depends on the installed application")
	}
	handler := &GroupHandler{}
	router := gin.New()
	router.GET("/live-capability", handler.GetLiveCapability)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/live-capability", nil))
	require.Equal(t, http.StatusOK, response.Code)
	var body struct {
		Data struct {
			Supported bool   `json:"supported"`
			Mode      string `json:"upstream_auth_mode"`
			Reason    string `json:"reason"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(response.Body.Bytes(), &body))
	require.False(t, body.Data.Supported)
	require.Equal(t, config.LiveUpstreamAuthModeDeviceCheck, body.Data.Mode)
	require.NotEmpty(t, body.Data.Reason)
}
