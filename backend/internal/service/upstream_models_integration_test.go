//go:build integration

package service

import (
	"context"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/tlsfingerprint"
	"github.com/stretchr/testify/require"
)

type upstreamModelsLiveEOFUpstream struct{}

func (*upstreamModelsLiveEOFUpstream) Do(*http.Request, string, int64, int) (*http.Response, error) {
	return nil, io.EOF
}

func (*upstreamModelsLiveEOFUpstream) DoWithTLS(*http.Request, string, int64, int, *tlsfingerprint.Profile) (*http.Response, error) {
	return nil, io.EOF
}

func TestFetchUpstreamSupportedModels_ChromeFallbackWithLiveAccount(t *testing.T) {
	baseURL := strings.TrimSpace(os.Getenv("SUB2API_LIVE_OPENAI_BASE_URL"))
	apiKey := strings.TrimSpace(os.Getenv("SUB2API_LIVE_OPENAI_API_KEY"))
	if baseURL == "" || apiKey == "" {
		t.Skip("requires SUB2API_LIVE_OPENAI_BASE_URL and SUB2API_LIVE_OPENAI_API_KEY")
	}

	svc := NewAccountTestService(
		nil,
		nil,
		nil,
		nil,
		nil,
		&upstreamModelsLiveEOFUpstream{},
		&config.Config{Security: config.SecurityConfig{URLAllowlist: config.URLAllowlistConfig{Enabled: false}}},
		nil,
	)

	models, err := svc.FetchUpstreamSupportedModels(context.Background(), &Account{
		ID:          11,
		Platform:    PlatformOpenAI,
		Type:        AccountTypeAPIKey,
		Concurrency: 1,
		Credentials: map[string]any{
			"api_key":  apiKey,
			"base_url": baseURL,
		},
	})

	require.NoError(t, err)
	require.NotEmpty(t, models)
}
