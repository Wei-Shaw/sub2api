//go:build unit

package service

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestGatewayServiceBuildUpstreamRequestVersionedBaseURL(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)

	svc := &GatewayService{cfg: &config.Config{
		Security: config.SecurityConfig{
			URLAllowlist: config.URLAllowlistConfig{Enabled: false},
		},
	}}
	account := &Account{
		Platform: PlatformAnthropic,
		Type:     AccountTypeAPIKey,
		Credentials: map[string]any{
			"api_key":  "upstream-key",
			"base_url": "https://relay.example.com/v1",
		},
	}

	req, _, err := svc.buildUpstreamRequest(
		context.Background(), c, account,
		[]byte(`{"model":"claude-sonnet-4-5","messages":[]}`),
		"upstream-key", "apikey", "claude-sonnet-4-5", false, false,
	)
	require.NoError(t, err)
	require.Equal(t, "https://relay.example.com/v1/messages?beta=true", req.URL.String())
}

func TestBuildUpstreamRequestAnthropicAPIKeyPassthroughVersionedBaseURL(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)

	svc := &GatewayService{cfg: &config.Config{
		Security: config.SecurityConfig{
			URLAllowlist: config.URLAllowlistConfig{Enabled: false},
		},
	}}
	account := &Account{
		Platform: PlatformAnthropic,
		Type:     AccountTypeAPIKey,
		Credentials: map[string]any{
			"api_key":  "upstream-key",
			"base_url": "https://relay.example.com/v1/",
		},
		Extra: map[string]any{"anthropic_passthrough": true},
	}

	req, _, err := svc.buildUpstreamRequestAnthropicAPIKeyPassthrough(
		context.Background(), c, account,
		[]byte(`{"model":"claude-sonnet-4-5","messages":[]}`),
		"upstream-key",
	)
	require.NoError(t, err)
	require.Equal(t, "https://relay.example.com/v1/messages?beta=true", req.URL.String())
}
