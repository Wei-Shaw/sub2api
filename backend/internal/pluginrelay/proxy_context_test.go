package pluginrelay

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestPrepareUpstreamRequestBypassesProxyForAIRelay(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "http://plugin-server:8091/plugins/ai-relay/agnes/1/v1/images/generations", nil)
	req.Header.Set(ProxyIDHeader, "42")

	proxyURL := PrepareUpstreamRequest(req, "http://proxy.example:8080")

	require.Empty(t, proxyURL)
	require.Equal(t, "42", req.Header.Get(ProxyIDHeader))
}

func TestPrepareUpstreamRequestStripsReservedHeaderForOtherUpstreams(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "https://api.openai.com/v1/images/generations", nil)
	req.Header.Set(ProxyIDHeader, "42")

	proxyURL := PrepareUpstreamRequest(req, "http://proxy.example:8080")

	require.Equal(t, "http://proxy.example:8080", proxyURL)
	require.Empty(t, req.Header.Get(ProxyIDHeader))
}

func TestSetProxyIDOverwritesOrRemovesReservedHeader(t *testing.T) {
	header := http.Header{}
	header.Set(ProxyIDHeader, "999")
	proxyID := int64(42)

	SetProxyID(header, &proxyID)
	require.Equal(t, "42", header.Get(ProxyIDHeader))

	SetProxyID(header, nil)
	require.Empty(t, header.Get(ProxyIDHeader))
}
