package service

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/tlsfingerprint"
	"github.com/stretchr/testify/require"
)

type upstreamModelsEOFUpstream struct {
	err error
}

func (u *upstreamModelsEOFUpstream) Do(*http.Request, string, int64, int) (*http.Response, error) {
	return nil, u.err
}

func (u *upstreamModelsEOFUpstream) DoWithTLS(*http.Request, string, int64, int, *tlsfingerprint.Profile) (*http.Response, error) {
	return nil, u.err
}

func TestFetchUpstreamSupportedModels_RetriesOpenAIAPIKeyWithChromeAfterEOF(t *testing.T) {
	upstream := &upstreamModelsEOFUpstream{err: io.EOF}
	svc := NewAccountTestService(
		nil,
		nil,
		nil,
		nil,
		nil,
		upstream,
		&config.Config{Security: config.SecurityConfig{URLAllowlist: config.URLAllowlistConfig{Enabled: false}}},
		nil,
	)

	fallbackCalled := false
	svc.openAIModelSyncChromeRequester = func(req *http.Request, proxyURL string) (*http.Response, error) {
		fallbackCalled = true
		require.Empty(t, proxyURL)
		require.Equal(t, http.MethodGet, req.Method)
		require.Equal(t, "https://upstream.example/v1/models", req.URL.String())
		require.Equal(t, "Bearer test-key", req.Header.Get("Authorization"))
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     make(http.Header),
			Body:       io.NopCloser(strings.NewReader(`{"data":[{"id":"gpt-5.6"}]}`)),
		}, nil
	}

	models, err := svc.FetchUpstreamSupportedModels(context.Background(), &Account{
		ID:          11,
		Platform:    PlatformOpenAI,
		Type:        AccountTypeAPIKey,
		Concurrency: 1,
		Credentials: map[string]any{
			"api_key":  "test-key",
			"base_url": "https://upstream.example/v1",
		},
	})

	require.NoError(t, err)
	require.True(t, fallbackCalled)
	require.Equal(t, []string{"gpt-5.6"}, models)
}

func TestFetchUpstreamSupportedModels_RetriesChromeFallbackAfterTransientEOF(t *testing.T) {
	upstream := &upstreamModelsEOFUpstream{err: io.EOF}
	svc := NewAccountTestService(
		nil,
		nil,
		nil,
		nil,
		nil,
		upstream,
		&config.Config{Security: config.SecurityConfig{URLAllowlist: config.URLAllowlistConfig{Enabled: false}}},
		nil,
	)

	fallbackCalls := 0
	svc.openAIModelSyncChromeRequester = func(*http.Request, string) (*http.Response, error) {
		fallbackCalls++
		if fallbackCalls == 1 {
			return nil, io.ErrUnexpectedEOF
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     make(http.Header),
			Body:       io.NopCloser(strings.NewReader(`{"data":[{"id":"gpt-5.6"}]}`)),
		}, nil
	}

	models, err := svc.FetchUpstreamSupportedModels(context.Background(), &Account{
		ID:          11,
		Platform:    PlatformOpenAI,
		Type:        AccountTypeAPIKey,
		Concurrency: 1,
		Credentials: map[string]any{
			"api_key":  "test-key",
			"base_url": "https://upstream.example/v1",
		},
	})

	require.NoError(t, err)
	require.Equal(t, 2, fallbackCalls)
	require.Equal(t, []string{"gpt-5.6"}, models)
}
