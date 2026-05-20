//go:build unit

package service

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestValidateEndpointAllowsLocalMonitorEndpoints(t *testing.T) {
	for _, endpoint := range []string{
		"http://localhost:8080",
		"http://127.0.0.1:8080",
		"http://[::1]:8080",
		"https://localhost:8443",
		"https://127.0.0.1:8443",
	} {
		t.Run(endpoint, func(t *testing.T) {
			require.NoError(t, validateEndpoint(endpoint))
		})
	}
}

func TestValidateEndpointRejectsUnsafeNonLocalEndpoints(t *testing.T) {
	tests := []struct {
		name     string
		endpoint string
		wantErr  error
	}{
		{
			name:     "public_http",
			endpoint: "http://example.com",
			wantErr:  ErrChannelMonitorEndpointScheme,
		},
		{
			name:     "private_http",
			endpoint: "http://192.168.1.10:8080",
			wantErr:  ErrChannelMonitorEndpointScheme,
		},
		{
			name:     "private_https",
			endpoint: "https://10.0.0.1:8443",
			wantErr:  ErrChannelMonitorEndpointPrivate,
		},
		{
			name:     "metadata_https",
			endpoint: "https://169.254.169.254",
			wantErr:  ErrChannelMonitorEndpointPrivate,
		},
		{
			name:     "path_not_origin",
			endpoint: "https://api.openai.com/v1",
			wantErr:  ErrChannelMonitorEndpointPath,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.ErrorIs(t, validateEndpoint(tt.endpoint), tt.wantErr)
		})
	}
}

func TestRunCheckForModelOpenAIImageUsesImagesEndpointAndLocalHTTP(t *testing.T) {
	var gotPath string
	var gotAuth string
	var gotBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotAuth = r.Header.Get("Authorization")
		defer func() { _ = r.Body.Close() }()
		require.NoError(t, json.NewDecoder(r.Body).Decode(&gotBody))

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": []map[string]any{
				{"b64_json": "aGVsbG8="},
			},
		})
	}))
	defer srv.Close()

	require.NoError(t, validateEndpoint(srv.URL))
	res := runCheckForModel(context.Background(), MonitorProviderOpenAI, srv.URL, "sk-local", "gpt-image-2", nil)

	require.Equal(t, MonitorStatusOperational, res.Status, res.Message)
	require.Equal(t, openAIImagesGenerationsEndpoint, gotPath)
	require.Equal(t, "Bearer sk-local", gotAuth)
	require.Equal(t, "gpt-image-2", gotBody["model"])
	require.Equal(t, defaultOpenAIImageTestPrompt, gotBody["prompt"])
	require.Equal(t, float64(1), gotBody["n"])
}

func TestRunCheckForModelOpenAIImageRequiresImageOutput(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[]}`))
	}))
	defer srv.Close()

	res := runCheckForModel(context.Background(), MonitorProviderOpenAI, srv.URL, "sk-local", "gpt-image-2", nil)

	require.Equal(t, MonitorStatusFailed, res.Status)
	require.Contains(t, res.Message, "without image output")
}

func TestRunCheckForModelOpenAIImageAcceptsTruncatedB64JSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[{"b64_json":"`))
		_, _ = w.Write([]byte(strings.Repeat("a", monitorImageResponseMaxBytes+1024)))
	}))
	defer srv.Close()

	res := runCheckForModel(context.Background(), MonitorProviderOpenAI, srv.URL, "sk-local", "gpt-image-2", nil)

	require.Equal(t, MonitorStatusOperational, res.Status, res.Message)
	require.Empty(t, res.Message)
}
