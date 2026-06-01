//go:build unit

package service

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func responsesToCCTestAccount(chatCompletionsURL string) *Account {
	return &Account{
		ID:          202,
		Name:        "responses-to-cc",
		Platform:    PlatformOpenAI,
		Type:        AccountTypeAPIKeyChatCompletions,
		Concurrency: 1,
		Credentials: map[string]any{
			"api_key":              "sk-test-rcc",
			"chat_completions_url": chatCompletionsURL,
		},
	}
}

func responsesToCCTestConfig() *config.Config {
	return &config.Config{
		Security: config.SecurityConfig{
			URLAllowlist: config.URLAllowlistConfig{
				Enabled:           false,
				AllowInsecureHTTP: true,
			},
		},
	}
}

func TestForward_APIKeyChatCompletionsResponsesUsesAdapter(t *testing.T) {
	gin.SetMode(gin.TestMode)

	upstreamResp := `{
		"id":"chatcmpl_rcc_forward",
		"object":"chat.completion",
		"created":1,
		"model":"gpt-5.4",
		"choices":[{"index":0,"message":{"role":"assistant","content":"hello from cc"},"finish_reason":"stop"}],
		"usage":{"prompt_tokens":6,"completion_tokens":3,"total_tokens":9}
	}`

	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/json"}, "x-request-id": []string{"rid_rcc_forward"}},
		Body:       io.NopCloser(strings.NewReader(upstreamResp)),
	}}

	body := []byte(`{"model":"gpt-5.4","input":"hi","stream":false}`)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	svc := &OpenAIGatewayService{
		cfg:          responsesToCCTestConfig(),
		httpUpstream: upstream,
	}
	account := responsesToCCTestAccount("http://upstream.example/v1/chat/completions")

	result, err := svc.Forward(context.Background(), c, account, body)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.False(t, result.Stream)
	require.Equal(t, "gpt-5.4", result.Model)
	require.Equal(t, 6, result.Usage.InputTokens)
	require.Equal(t, 3, result.Usage.OutputTokens)

	require.NotNil(t, upstream.lastReq)
	require.Equal(t, "http://upstream.example/v1/chat/completions", upstream.lastReq.URL.String())
	require.Equal(t, "Bearer sk-test-rcc", upstream.lastReq.Header.Get("Authorization"))
	require.Contains(t, string(upstream.lastBody), `"messages"`)
	require.NotContains(t, string(upstream.lastBody), `"input":"hi"`)

	clientBody := rec.Body.String()
	require.Contains(t, clientBody, `"object":"response"`)
	require.Contains(t, clientBody, "hello from cc")
}

func TestForwardResponsesAsChatCompletions_NonStreamingTextResponse(t *testing.T) {
	gin.SetMode(gin.TestMode)

	upstreamResp := `{
		"id":"chatcmpl_rcc_1",
		"object":"chat.completion",
		"created":1,
		"model":"gpt-4o",
		"choices":[{"index":0,"message":{"role":"assistant","content":"hello world"},"finish_reason":"stop"}],
		"usage":{"prompt_tokens":7,"completion_tokens":2,"total_tokens":9,"prompt_tokens_details":{"cached_tokens":1}}
	}`

	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/json"}, "x-request-id": []string{"rid_rcc_buffer"}},
		Body:       io.NopCloser(strings.NewReader(upstreamResp)),
	}}

	body := []byte(`{"model":"gpt-4o","input":"hi","stream":false}`)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	svc := &OpenAIGatewayService{
		cfg:          responsesToCCTestConfig(),
		httpUpstream: upstream,
	}
	account := responsesToCCTestAccount("http://upstream.example/v1/chat/completions")

	result, err := svc.ForwardResponsesAsChatCompletions(context.Background(), c, account, body)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.False(t, result.Stream)
	require.Equal(t, "gpt-4o", result.Model)
	require.Equal(t, 7, result.Usage.InputTokens)
	require.Equal(t, 2, result.Usage.OutputTokens)
	require.Equal(t, 1, result.Usage.CacheReadInputTokens)

	// Upstream request was sent to the configured URL with Bearer + JSON headers.
	require.NotNil(t, upstream.lastReq)
	require.Equal(t, "http://upstream.example/v1/chat/completions", upstream.lastReq.URL.String())
	require.Equal(t, "application/json", upstream.lastReq.Header.Get("Content-Type"))
	require.Equal(t, "Bearer sk-test-rcc", upstream.lastReq.Header.Get("Authorization"))
	require.Equal(t, "application/json", upstream.lastReq.Header.Get("Accept"))

	// Upstream body must be a Chat Completions JSON (not a Responses payload).
	require.Contains(t, string(upstream.lastBody), `"messages"`)
	require.NotContains(t, string(upstream.lastBody), `"input"`)

	// Response body sent to client is Responses-shaped JSON containing the assistant text.
	clientBody := rec.Body.String()
	require.Contains(t, clientBody, `"object":"response"`)
	require.Contains(t, clientBody, "hello world")
	require.Contains(t, clientBody, `"output_text"`)
}

func TestForwardResponsesAsChatCompletions_StreamingSSEConversion(t *testing.T) {
	gin.SetMode(gin.TestMode)

	// Mock upstream emits Chat Completions SSE chunks; service must convert each
	// to Responses SSE events and stream them back to the client.
	upstreamSSE := strings.Join([]string{
		`data: {"id":"chatcmpl_stream","object":"chat.completion.chunk","created":1,"model":"gpt-4o","choices":[{"index":0,"delta":{"role":"assistant"},"finish_reason":null}]}`,
		"",
		`data: {"id":"chatcmpl_stream","object":"chat.completion.chunk","created":1,"model":"gpt-4o","choices":[{"index":0,"delta":{"content":"Hi"},"finish_reason":null}]}`,
		"",
		`data: {"id":"chatcmpl_stream","object":"chat.completion.chunk","created":1,"model":"gpt-4o","choices":[{"index":0,"delta":{},"finish_reason":"stop"}],"usage":{"prompt_tokens":4,"completion_tokens":1,"total_tokens":5}}`,
		"",
		"data: [DONE]",
		"",
	}, "\n")

	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}, "x-request-id": []string{"rid_rcc_stream"}},
		Body:       io.NopCloser(strings.NewReader(upstreamSSE)),
	}}

	body := []byte(`{"model":"gpt-4o","input":"hi","stream":true}`)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	svc := &OpenAIGatewayService{
		cfg:          responsesToCCTestConfig(),
		httpUpstream: upstream,
	}
	account := responsesToCCTestAccount("http://upstream.example/v1/chat/completions")

	result, err := svc.ForwardResponsesAsChatCompletions(context.Background(), c, account, body)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.True(t, result.Stream)
	require.Equal(t, 4, result.Usage.InputTokens)
	require.Equal(t, 1, result.Usage.OutputTokens)

	// Upstream Accept must be SSE for stream requests.
	require.Equal(t, "text/event-stream", upstream.lastReq.Header.Get("Accept"))

	// Client received Responses SSE events (not raw CC chunks) — should at minimum
	// contain a response.created and response.completed event plus terminator.
	clientBody := rec.Body.String()
	require.Contains(t, clientBody, "event: response.created")
	require.Contains(t, clientBody, "event: response.output_text.delta")
	require.Contains(t, clientBody, "event: response.completed")
	require.Contains(t, clientBody, "data: [DONE]")
}

func TestForwardResponsesAsChatCompletions_MissingModelReturnsBadRequest(t *testing.T) {
	gin.SetMode(gin.TestMode)

	body := []byte(`{"input":"hi"}`)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	svc := &OpenAIGatewayService{
		cfg:          responsesToCCTestConfig(),
		httpUpstream: &httpUpstreamRecorder{},
	}
	account := responsesToCCTestAccount("http://upstream.example/v1/chat/completions")

	_, err := svc.ForwardResponsesAsChatCompletions(context.Background(), c, account, body)
	require.Error(t, err)
	require.Equal(t, http.StatusBadRequest, rec.Code)
}
