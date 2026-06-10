package xai

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestResolveInferenceBaseURLUsesAccountBase(t *testing.T) {
	require.Equal(t, "https://api.x.ai/v1", ResolveInferenceBaseURL("https://api.x.ai/v1"))
	require.Equal(t, "https://api.x.ai/v1", ResolveInferenceBaseURL(""))
	require.Equal(t, CLIChatProxyBaseURL, ResolveInferenceBaseURL("https://cli-chat-proxy.grok.com/v1"))
}

func TestShouldUseGrokCLINormalizeDetectsGrokCLIRequests(t *testing.T) {
	headers := http.Header{}
	headers.Set("x-grok-client", "grok-cli/1.0")

	require.True(t, ShouldUseGrokCLINormalize("https://api.x.ai/v1", "xai-grok-cli/0.2.39", "grok-4.3", nil))
	require.True(t, ShouldUseGrokCLINormalize("https://api.x.ai/v1", "curl/8.0", "grok-composer-2.5-fast", nil))
	require.True(t, ShouldUseGrokCLINormalize("https://api.x.ai/v1", "curl/8.0", "grok-4.3", headers))
	require.True(t, ShouldUseGrokCLINormalize("https://cli-chat-proxy.grok.com/v1", "curl/8.0", "grok-4.3", nil))
	require.False(t, ShouldUseGrokCLINormalize("https://api.x.ai/v1", "curl/8.0", "grok-4.3", nil))
}

func TestForwardGrokCLIRequestHeaders(t *testing.T) {
	src := http.Header{}
	src.Set("User-Agent", "xai-grok-cli/0.2.39")
	src.Set("Accept-Language", "en-US")
	src.Set("X-Grok-Client-Version", "0.2.39")
	src.Set("X-Grok-Conv-Id", "client-conv")

	dst := make(http.Header)
	ForwardGrokCLIRequestHeaders(dst, src, "prompt-cache-conv")

	require.Equal(t, "xai-grok-cli/0.2.39", dst.Get("User-Agent"))
	require.Equal(t, "en-US", dst.Get("Accept-Language"))
	require.Equal(t, "0.2.39", dst.Get("X-Grok-Client-Version"))
	require.Equal(t, "prompt-cache-conv", dst.Get("X-Grok-Conv-Id"))
}

func TestIsGrokCLIHTTPRequest(t *testing.T) {
	headers := http.Header{}
	headers.Set("x-grok-client", "grok-cli/1.0")

	require.True(t, IsGrokCLIHTTPRequest("xai-grok-cli/0.2.39", "grok-4.3", nil))
	require.True(t, IsGrokCLIHTTPRequest("curl/8.0", "grok-composer-2.5-fast", nil))
	require.True(t, IsGrokCLIHTTPRequest("curl/8.0", "grok-composer-2", nil))
	require.True(t, IsGrokCLIHTTPRequest("curl/8.0", "grok-4.3", headers))
	require.False(t, IsGrokCLIHTTPRequest("curl/8.0", "gpt-5.1", nil))
}

func TestBuildModelsV2URL(t *testing.T) {
	require.Equal(t, CLIChatProxyBaseURL+"/models-v2", BuildModelsV2URL(""))
	require.Equal(t, CLIChatProxyBaseURL+"/models-v2", BuildModelsV2URL(CLIChatProxyBaseURL))
	require.Equal(t, CLIChatProxyBaseURL+"/models-v2", BuildModelsV2URL(CLIChatProxyBaseURL+"/responses"))
}