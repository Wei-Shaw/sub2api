package service

import (
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestOpenAIClientTransport_SetAndGet(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	require.Equal(t, OpenAIClientTransportUnknown, GetOpenAIClientTransport(c))

	SetOpenAIClientTransport(c, OpenAIClientTransportHTTP)
	require.Equal(t, OpenAIClientTransportHTTP, GetOpenAIClientTransport(c))

	SetOpenAIClientTransport(c, OpenAIClientTransportWS)
	require.Equal(t, OpenAIClientTransportWS, GetOpenAIClientTransport(c))
}

func TestOpenAIClientTransport_GetNormalizesRawContextValue(t *testing.T) {
	gin.SetMode(gin.TestMode)

	tests := []struct {
		name     string
		rawValue any
		want     OpenAIClientTransport
	}{
		{
			name:     "type_value_ws",
			rawValue: OpenAIClientTransportWS,
			want:     OpenAIClientTransportWS,
		},
		{
			name:     "http_sse_alias",
			rawValue: "http_sse",
			want:     OpenAIClientTransportHTTP,
		},
		{
			name:     "sse_alias",
			rawValue: "sSe",
			want:     OpenAIClientTransportHTTP,
		},
		{
			name:     "websocket_alias",
			rawValue: "WebSocket",
			want:     OpenAIClientTransportWS,
		},
		{
			name:     "invalid_string",
			rawValue: "tcp",
			want:     OpenAIClientTransportUnknown,
		},
		{
			name:     "invalid_type",
			rawValue: 123,
			want:     OpenAIClientTransportUnknown,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			c.Set(openAIClientTransportContextKey, tt.rawValue)
			require.Equal(t, tt.want, GetOpenAIClientTransport(c))
		})
	}
}

func TestOpenAIClientTransport_NilAndUnknownInput(t *testing.T) {
	SetOpenAIClientTransport(nil, OpenAIClientTransportHTTP)
	require.Equal(t, OpenAIClientTransportUnknown, GetOpenAIClientTransport(nil))

	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	SetOpenAIClientTransport(c, OpenAIClientTransportUnknown)
	_, exists := c.Get(openAIClientTransportContextKey)
	require.False(t, exists)

	SetOpenAIClientTransport(c, OpenAIClientTransport("   "))
	_, exists = c.Get(openAIClientTransportContextKey)
	require.False(t, exists)
}

func TestResolveOpenAIWSDecisionByClientTransport_HTTP(t *testing.T) {
	base := OpenAIWSProtocolDecision{
		Transport: OpenAIUpstreamTransportResponsesWebsocketV2,
		Reason:    "ws_v2_enabled",
	}

	apiKeyDecision := resolveOpenAIWSDecisionByClientTransport(base, OpenAIClientTransportHTTP, &Account{
		Platform: PlatformOpenAI,
		Type:     AccountTypeAPIKey,
	})
	require.Equal(t, OpenAIUpstreamTransportHTTPSSE, apiKeyDecision.Transport)
	require.Equal(t, "client_protocol_http", apiKeyDecision.Reason)

	oauthDecision := resolveOpenAIWSDecisionByClientTransport(base, OpenAIClientTransportHTTP, &Account{
		Platform: PlatformOpenAI,
		Type:     AccountTypeOAuth,
	})
	require.Equal(t, OpenAIUpstreamTransportResponsesWebsocketV2, oauthDecision.Transport)
	require.Equal(t, "http_responses_facade_ws_v2_enabled", oauthDecision.Reason)

	wsDecision := resolveOpenAIWSDecisionByClientTransport(base, OpenAIClientTransportWS, nil)
	require.Equal(t, base, wsDecision)

	unknownDecision := resolveOpenAIWSDecisionByClientTransport(base, OpenAIClientTransportUnknown, nil)
	require.Equal(t, base, unknownDecision)
}
