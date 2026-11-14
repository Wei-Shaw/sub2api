//go:build unit

package service

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestValidateMonitorEndpoint_BasePath(t *testing.T) {
	for _, endpoint := range []string{
		"https://8.8.8.8",
		"http://8.8.8.8/anthropic",
		"http://localhost:3000",
		"https://127.0.0.1/anthropic",
		"http://[::1]:3000/anthropic",
		"https://8.8.8.8/anthropic",
		"https://8.8.8.8/anthropic/",
		"https://8.8.8.8/anthropic/v1",
		"https://8.8.8.8/tenant%2Fone/anthropic",
	} {
		t.Run(endpoint, func(t *testing.T) {
			require.NoError(t, validateEndpoint(endpoint))
		})
	}

	for _, tc := range []struct {
		endpoint string
		want     error
	}{
		{"ftp://8.8.8.8/anthropic", ErrChannelMonitorEndpointScheme},
		{"https://8.8.8.8/anthropic?key=secret", ErrChannelMonitorEndpointPath},
		{"https://8.8.8.8/anthropic#fragment", ErrChannelMonitorEndpointPath},
		{"https://10.0.0.1/anthropic", ErrChannelMonitorEndpointPrivate},
		{"https://169.254.169.254/anthropic", ErrChannelMonitorEndpointPrivate},
		{"http://metadata.google.internal", ErrChannelMonitorEndpointPrivate},
		{"https://[fd00::1]/anthropic", ErrChannelMonitorEndpointPrivate},
		{"https:///anthropic", ErrChannelMonitorInvalidEndpoint},
	} {
		t.Run(tc.endpoint, func(t *testing.T) {
			require.ErrorIs(t, validateEndpoint(tc.endpoint), tc.want)
		})
	}
}

func TestMonitorHTTPClient_LocalHTTP(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/relay/v1/models" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(server.Close)
	client := newSSRFSafeHTTPClient(5 * time.Second)
	t.Cleanup(client.CloseIdleConnections)
	for _, endpoint := range []string{server.URL, strings.Replace(server.URL, "127.0.0.1", "localhost", 1)} {
		t.Run(endpoint, func(t *testing.T) {
			require.NoError(t, validateEndpoint(endpoint))
			resp, err := client.Get(endpoint + "/relay/v1/models")
			require.NoError(t, err)
			defer resp.Body.Close()
			require.Equal(t, http.StatusNoContent, resp.StatusCode)
		})
	}
}

func TestMonitorDial_RejectsPrivateTargets(t *testing.T) {
	for _, address := range []string{"10.0.0.1:80", "169.254.169.254:80", "[fd00::1]:80", "metadata.google.internal:80"} {
		t.Run(address, func(t *testing.T) {
			conn, err := safeDialContext(context.Background(), "tcp", address)
			if conn != nil {
				conn.Close()
			}
			require.ErrorContains(t, err, "blocked by SSRF policy")
		})
	}
}

func TestJoinMonitorURL_DoesNotMatchHostOrEncodedSlash(t *testing.T) {
	require.Equal(t, "https://v1/v1/messages", joinURL("https://v1", "/v1/messages"))
	require.Equal(t, "https://relay.example/tenant%2Fv1/v1/messages",
		joinURL("https://relay.example/tenant%2Fv1", "/v1/messages"))
}

func TestNormalizeMonitorEndpoint_PreservesBasePath(t *testing.T) {
	require.Equal(t, "https://api.deepseek.com/anthropic",
		normalizeEndpoint("  https://api.deepseek.com/anthropic/  "))
	require.Equal(t, "https://relay.example/tenant%2Fone/anthropic/v1",
		normalizeEndpoint("https://relay.example/tenant%2Fone/anthropic/v1/"))
}

func TestCallProvider_BasePath(t *testing.T) {
	swapMonitorHTTPClient(t)
	requests := make(chan string, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests <- r.RequestURI
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte("{}"))
	}))
	t.Cleanup(server.Close)

	for _, tc := range []struct {
		name     string
		provider string
		apiMode  string
		basePath string
		wantPath string
	}{
		{"anthropic origin", MonitorProviderAnthropic, "", "", "/v1/messages"},
		{"anthropic prefix", MonitorProviderAnthropic, "", "/anthropic", "/anthropic/v1/messages"},
		{"anthropic trailing slash", MonitorProviderAnthropic, "", "/anthropic/", "/anthropic/v1/messages"},
		{"anthropic version", MonitorProviderAnthropic, "", "/anthropic/v1", "/anthropic/v1/messages"},
		{"version lookalike", MonitorProviderAnthropic, "", "/anthropic/v10", "/anthropic/v10/v1/messages"},
		{"encoded prefix", MonitorProviderAnthropic, "", "/tenant%2Fone/anthropic", "/tenant%2Fone/anthropic/v1/messages"},
		{"openai chat version", MonitorProviderOpenAI, "", "/relay/v1/", "/relay/v1/chat/completions"},
		{"openai responses version", MonitorProviderOpenAI, MonitorAPIModeResponses, "/relay/v1", "/relay/v1/responses"},
		{"gemini version", MonitorProviderGemini, "", "/relay/v1beta", "/relay/v1beta/models/test-model:generateContent"},
		{"zhipu version", MonitorProviderZhipu, "", "/api/paas/v4", "/api/paas/v4/chat/completions"},
		{"zhipu coding plan", MonitorProviderZhipu, "", "/api/coding/paas/v4", "/api/coding/paas/v4/chat/completions"},
		{"zhipu relay origin", MonitorProviderZhipu, "", "", "/v1/chat/completions"},
		{"zhipu relay version", MonitorProviderZhipu, "", "/relay/v1", "/relay/v1/chat/completions"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, _, status, err := callProvider(context.Background(), tc.provider,
				server.URL+tc.basePath, "test-key", "test-model", "hello", &CheckOptions{APIMode: tc.apiMode})
			require.NoError(t, err)
			require.Equal(t, http.StatusOK, status)
			require.Equal(t, tc.wantPath, <-requests)
		})
	}
}

func TestMonitorRequestPath_Zhipu(t *testing.T) {
	adapter := providerAdapters[MonitorProviderZhipu]
	for endpoint, want := range map[string]string{
		"https://open.bigmodel.cn":                    providerZhipuPath,
		"https://api.z.ai":                            providerZhipuPath,
		"https://open.bigmodel.cn/api/coding/paas/v4": "/chat/completions",
		"https://birdapi.up.railway.app":              providerOpenAIPath,
		"https://notbigmodel.cn":                      providerOpenAIPath,
		"https://relay.example.com/v1":                providerOpenAIPath,
	} {
		require.Equal(t, want, monitorRequestPath(MonitorProviderZhipu, endpoint, adapter, "glm-5.3-flash"), endpoint)
	}
	require.Equal(t, providerOpenAIPath,
		monitorRequestPath(MonitorProviderDeepseek, "https://birdapi.up.railway.app", providerAdapters[MonitorProviderDeepseek], "m"))
}
