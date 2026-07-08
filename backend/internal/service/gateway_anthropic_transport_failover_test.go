package service

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

// newAnthropicTransportFailoverTestService builds the minimal GatewayService used
// by the transport-error failover tests (mirrors the passthrough test setup).
func newAnthropicTransportFailoverTestService(upstream *anthropicHTTPUpstreamRecorder) *GatewayService {
	cfg := &config.Config{
		Gateway: config.GatewayConfig{
			MaxLineSize: defaultMaxLineSize,
		},
	}
	return &GatewayService{
		cfg:                  cfg,
		responseHeaderFilter: compileResponseHeaderFilter(cfg),
		httpUpstream:         upstream,
		rateLimitService:     &RateLimitService{},
		deferredService:      &DeferredService{},
	}
}

func newAnthropicTransportFailoverTestContext() (*gin.Context, *httptest.ResponseRecorder) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)
	return c, rec
}

// A transport-level failure (proxy/TCP/TLS, no HTTP status) on the native
// Anthropic forward path must surface as *UpstreamFailoverError so the handler
// can switch to a healthy account, and must NOT write a hard 502 itself.
func TestGatewayServiceForward_AnthropicNative_TransportErrorFailsOver(t *testing.T) {
	upstream := &anthropicHTTPUpstreamRecorder{
		err: errors.New(`Post "https://api.anthropic.com/v1/messages": read tcp 10.0.0.2:39724->160.79.104.10:443: read: connection reset by peer`),
	}
	svc := newAnthropicTransportFailoverTestService(upstream)

	// API-key account WITHOUT the anthropic_passthrough flag → native forward path.
	account := &Account{
		ID:          301,
		Name:        "anthropic-native",
		Platform:    PlatformAnthropic,
		Type:        AccountTypeAPIKey,
		Concurrency: 1,
		Credentials: map[string]any{
			"api_key":  "upstream-anthropic-key",
			"base_url": "https://api.anthropic.com",
		},
		Status:      StatusActive,
		Schedulable: true,
	}

	c, rec := newAnthropicTransportFailoverTestContext()
	body := []byte(`{"model":"claude-3-7-sonnet-20250219","messages":[{"role":"user","content":[{"type":"text","text":"hello"}]}]}`)
	parsed := &ParsedRequest{
		Body:  NewRequestBodyRef(body),
		Model: "claude-3-7-sonnet-20250219",
	}

	result, err := svc.Forward(context.Background(), c, account, parsed)

	require.Nil(t, result)
	var fo *UpstreamFailoverError
	require.True(t, errors.As(err, &fo), "transport error must trigger account failover, got: %v", err)
	require.Equal(t, http.StatusBadGateway, fo.StatusCode)
	require.Equal(t, 0, rec.Body.Len(), "service must not write a hard 502 before handler can fail over")
}

// Same requirement for the Chat Completions → Anthropic conversion path.
func TestGatewayServiceForwardAsChatCompletions_TransportErrorFailsOver(t *testing.T) {
	upstream := &anthropicHTTPUpstreamRecorder{
		err: errors.New(`Post "https://api.anthropic.com/v1/messages": proxyconnect tcp: dial tcp 10.1.1.1:7890: i/o timeout`),
	}
	svc := newAnthropicTransportFailoverTestService(upstream)

	account := &Account{
		ID:          302,
		Name:        "anthropic-cc",
		Platform:    PlatformAnthropic,
		Type:        AccountTypeAPIKey,
		Concurrency: 1,
		Credentials: map[string]any{
			"api_key":  "upstream-anthropic-key",
			"base_url": "https://api.anthropic.com",
		},
		Status:      StatusActive,
		Schedulable: true,
	}

	c, rec := newAnthropicTransportFailoverTestContext()
	body := []byte(`{"model":"claude-3-7-sonnet-20250219","messages":[{"role":"user","content":"hello"}]}`)

	result, err := svc.ForwardAsChatCompletions(context.Background(), c, account, body, &ParsedRequest{Body: NewRequestBodyRef(body)})

	require.Nil(t, result)
	var fo *UpstreamFailoverError
	require.True(t, errors.As(err, &fo), "transport error must trigger account failover, got: %v", err)
	require.Equal(t, http.StatusBadGateway, fo.StatusCode)
	require.Equal(t, 0, rec.Body.Len(), "service must not write a hard 502 before handler can fail over")
}

// Same requirement for the Responses → Anthropic conversion path.
func TestGatewayServiceForwardAsResponses_TransportErrorFailsOver(t *testing.T) {
	upstream := &anthropicHTTPUpstreamRecorder{
		err: errors.New(`Post "https://api.anthropic.com/v1/messages": tls: handshake failure`),
	}
	svc := newAnthropicTransportFailoverTestService(upstream)

	account := &Account{
		ID:          303,
		Name:        "anthropic-responses",
		Platform:    PlatformAnthropic,
		Type:        AccountTypeAPIKey,
		Concurrency: 1,
		Credentials: map[string]any{
			"api_key":  "upstream-anthropic-key",
			"base_url": "https://api.anthropic.com",
		},
		Status:      StatusActive,
		Schedulable: true,
	}

	c, rec := newAnthropicTransportFailoverTestContext()
	body := []byte(`{"model":"claude-3-7-sonnet-20250219","input":[{"type":"message","role":"user","content":[{"type":"input_text","text":"hello"}]}]}`)

	result, err := svc.ForwardAsResponses(context.Background(), c, account, body, &ParsedRequest{Body: NewRequestBodyRef(body)})

	require.Nil(t, result)
	var fo *UpstreamFailoverError
	require.True(t, errors.As(err, &fo), "transport error must trigger account failover, got: %v", err)
	require.Equal(t, http.StatusBadGateway, fo.StatusCode)
	require.Equal(t, 0, rec.Body.Len(), "service must not write a hard 502 before handler can fail over")
}

// Same requirement for the Bedrock upstream execution path.
func TestGatewayServiceExecuteBedrockUpstream_TransportErrorFailsOver(t *testing.T) {
	upstream := &anthropicHTTPUpstreamRecorder{
		err: errors.New(`Post "https://bedrock-runtime.us-east-1.amazonaws.com/model/x/invoke": read: connection reset by peer`),
	}
	svc := newAnthropicTransportFailoverTestService(upstream)

	account := &Account{
		ID:          304,
		Name:        "bedrock-apikey",
		Platform:    PlatformAnthropic,
		Type:        AccountTypeBedrock,
		Concurrency: 1,
		Credentials: map[string]any{
			"auth_mode": "apikey",
			"api_key":   "bedrock-api-key",
		},
		Status:      StatusActive,
		Schedulable: true,
	}

	c, rec := newAnthropicTransportFailoverTestContext()
	body := []byte(`{"messages":[{"role":"user","content":[{"type":"text","text":"hello"}]}]}`)

	resp, err := svc.executeBedrockUpstream(context.Background(), c, account, body,
		"anthropic.claude-3-7-sonnet-20250219-v1:0", "us-east-1", false, nil, "bedrock-api-key", "")

	require.Nil(t, resp)
	var fo *UpstreamFailoverError
	require.True(t, errors.As(err, &fo), "transport error must trigger account failover, got: %v", err)
	require.Equal(t, http.StatusBadGateway, fo.StatusCode)
	require.Equal(t, 0, rec.Body.Len(), "service must not write a hard 502 before handler can fail over")
}

// Same requirement for the Anthropic API-key passthrough forward path.
func TestGatewayServiceForward_AnthropicAPIKeyPassthrough_TransportErrorFailsOver(t *testing.T) {
	upstream := &anthropicHTTPUpstreamRecorder{
		err: errors.New(`Post "https://api.anthropic.com/v1/messages": EOF`),
	}
	svc := newAnthropicTransportFailoverTestService(upstream)

	account := newAnthropicAPIKeyAccountForTest() // anthropic_passthrough: true

	c, rec := newAnthropicTransportFailoverTestContext()
	body := []byte(`{"model":"claude-3-7-sonnet-20250219","messages":[{"role":"user","content":[{"type":"text","text":"hello"}]}]}`)
	parsed := &ParsedRequest{
		Body:  NewRequestBodyRef(body),
		Model: "claude-3-7-sonnet-20250219",
	}

	result, err := svc.Forward(context.Background(), c, account, parsed)

	require.Nil(t, result)
	var fo *UpstreamFailoverError
	require.True(t, errors.As(err, &fo), "transport error must trigger account failover, got: %v", err)
	require.Equal(t, http.StatusBadGateway, fo.StatusCode)
	require.Equal(t, 0, rec.Body.Len(), "service must not write a hard 502 before handler can fail over")
}
