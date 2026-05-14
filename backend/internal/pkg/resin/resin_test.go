package resin

import (
	"context"
	"net/http"
	"net/url"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestParseAndForwardProxyURLForAccount(t *testing.T) {
	cfg, err := Parse("https://openai:token123@resin.example.com/proxy?foo=bar#resin")
	require.NoError(t, err)

	require.Equal(t, "https://resin.example.com", cfg.ForwardProxyBaseURL())
	require.Equal(t, "https://openai:token123@resin.example.com", cfg.BaseAuthProxyURL())
	require.Equal(t, "https://openai.acct-42:token123@resin.example.com", cfg.ForwardProxyURLForAccount(42))
	require.True(t, cfg.MatchesForwardProxyURL("https://resin.example.com"))
}

func TestPrepareReverseRequest(t *testing.T) {
	cfg, err := Parse("https://openai:token123@resin.example.com/reverse#resin")
	require.NoError(t, err)

	req, err := http.NewRequest(http.MethodPost, "https://api.openai.com/v1/responses?stream=true", nil)
	require.NoError(t, err)
	req.Header.Set("Authorization", "Bearer test")

	rewritten, err := PrepareReverseRequest(req, cfg, 7)
	require.NoError(t, err)

	require.Equal(t, "https://resin.example.com/reverse/openai/https/api.openai.com/v1/responses?stream=true", rewritten.URL.String())
	require.Equal(t, "api.openai.com", rewritten.Host)
	require.Equal(t, "acct-7", rewritten.Header.Get(HeaderAccount))
	require.Equal(t, "Bearer test", rewritten.Header.Get("Authorization"))
}

func TestPrepareReverseWS(t *testing.T) {
	cfg, err := Parse("https://openai:token123@resin.example.com/reverse#resin")
	require.NoError(t, err)

	headers := make(http.Header)
	headers.Set("Authorization", "Bearer test")

	rewrittenURL, host, rewrittenHeaders, err := PrepareReverseWS("wss://chatgpt.com/backend-api/realtime", headers, cfg, 9)
	require.NoError(t, err)

	require.Equal(t, "https://resin.example.com/reverse/openai/https/chatgpt.com/backend-api/realtime", rewrittenURL)
	require.Equal(t, "chatgpt.com", host)
	require.Equal(t, "acct-9", rewrittenHeaders.Get(HeaderAccount))
}

func TestWrapForwardProxyRoundTripperAddsProxyAuthorization(t *testing.T) {
	cfg, err := Parse("https://openai:token123@resin.example.com#resin")
	require.NoError(t, err)

	var gotAuth string
	base := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		gotAuth = req.Header.Get("Proxy-Authorization")
		return &http.Response{StatusCode: http.StatusOK, Body: http.NoBody}, nil
	})

	req, err := http.NewRequestWithContext(WithAccountID(context.Background(), 11), http.MethodGet, "https://chatgpt.com/backend-api", nil)
	require.NoError(t, err)

	_, err = WrapForwardProxyRoundTripper(base, cfg).RoundTrip(req)
	require.NoError(t, err)
	require.Equal(t, cfg.ProxyAuthorizationHeader(11), gotAuth)
}

func TestBuildReverseURLMergesBaseAndOriginalQuery(t *testing.T) {
	cfg := &Config{
		BaseURL: &url.URL{
			Scheme:   "https",
			Host:     "resin.example.com",
			Path:     "/reverse",
			RawQuery: "region=us",
		},
		Platform: "openai",
		Secret:   "token",
	}

	target, err := cfg.BuildReverseURL(&url.URL{
		Scheme:   "https",
		Host:     "api.openai.com",
		Path:     "/v1/responses",
		RawQuery: "stream=true",
	})
	require.NoError(t, err)
	require.Equal(t, "https://resin.example.com/reverse/openai/https/api.openai.com/v1/responses?region=us&stream=true", target.String())
}

func TestBuildReverseURLKeepsResinBasePath(t *testing.T) {
	cfg, err := Parse("http://openai:token123@127.0.0.1:2260/my-token#resin")
	require.NoError(t, err)

	target, err := cfg.BuildReverseURL(&url.URL{
		Scheme: "https",
		Host:   "api.openai.com",
		Path:   "/v1/responses",
	})
	require.NoError(t, err)
	require.Equal(t, "http://127.0.0.1:2260/my-token/openai/https/api.openai.com/v1/responses", target.String())
}
