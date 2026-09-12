//go:build unit

package service

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestValidateMonitorEndpoint_BasePath(t *testing.T) {
	for _, endpoint := range []string{
		"https://8.8.8.8",
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
		{"http://8.8.8.8/anthropic", ErrChannelMonitorEndpointScheme},
		{"https://8.8.8.8/anthropic?key=secret", ErrChannelMonitorEndpointPath},
		{"https://8.8.8.8/anthropic#fragment", ErrChannelMonitorEndpointPath},
		{"https://127.0.0.1/anthropic", ErrChannelMonitorEndpointPrivate},
		{"https://10.0.0.1/anthropic", ErrChannelMonitorEndpointPrivate},
		{"https://169.254.169.254/anthropic", ErrChannelMonitorEndpointPrivate},
		{"https://[::1]/anthropic", ErrChannelMonitorEndpointPrivate},
		{"https://[fd00::1]/anthropic", ErrChannelMonitorEndpointPrivate},
		{"https:///anthropic", ErrChannelMonitorInvalidEndpoint},
	} {
		t.Run(tc.endpoint, func(t *testing.T) {
			require.ErrorIs(t, validateEndpoint(tc.endpoint), tc.want)
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

func TestIsZhipuOfficialEndpoint(t *testing.T) {
	for _, tc := range []struct {
		endpoint string
		want     bool
	}{
		{"https://open.bigmodel.cn", true},
		{"https://open.bigmodel.cn/api/paas/v4", true},
		{"https://OPEN.BIGMODEL.CN/api/coding/paas/v4", true},
		{"https://relay.example.com", false},
		{"https://not-open.bigmodel.cn", false},
		{"https://open.bigmodel.cn.evil.com", false},
		{"://bad", true}, // url.Parse 失败时保守当作官方域名，保留旧行为
	} {
		t.Run(tc.endpoint, func(t *testing.T) {
			require.Equal(t, tc.want, isZhipuOfficialEndpoint(tc.endpoint))
		})
	}
}

func TestProviderAdapterFor_ZhipuPathSelection(t *testing.T) {
	// 官方域名（含 Coding 计划的 base path）：走官方原生路径，行为不变。
	for _, endpoint := range []string{"https://open.bigmodel.cn", "https://open.bigmodel.cn/api/paas/v4", "https://open.bigmodel.cn/api/coding/paas/v4"} {
		t.Run("official/"+endpoint, func(t *testing.T) {
			adapter, apiMode, ok := providerAdapterFor(MonitorProviderZhipu, "", endpoint)
			require.True(t, ok)
			require.Equal(t, MonitorAPIModeChatCompletions, apiMode)
			require.Equal(t, providerZhipuPath, adapter.buildPath("glm-5.3-flash"))
		})
	}

	// 第三方中转站：回退到通用兼容路径，修复 404。
	for _, endpoint := range []string{"https://relay.example.com", "https://api.opkaa.com"} {
		t.Run("third-party/"+endpoint, func(t *testing.T) {
			adapter, apiMode, ok := providerAdapterFor(MonitorProviderZhipu, "", endpoint)
			require.True(t, ok)
			require.Equal(t, MonitorAPIModeChatCompletions, apiMode)
			require.Equal(t, providerOpenAIPath, adapter.buildPath("glm-5.3-flash"))
		})
	}
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
		// httptest server 的地址不是官方域名 open.bigmodel.cn，所以按当前行为
		// （见 isZhipuOfficialEndpoint）应该回退到通用兼容路径，而不是官方原生路径。
		{"zhipu third-party relay", MonitorProviderZhipu, "", "", "/v1/chat/completions"},
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
