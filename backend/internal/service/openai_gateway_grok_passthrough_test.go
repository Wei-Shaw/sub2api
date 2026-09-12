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
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/xai"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func grokPassthroughTestAccount(id int64, extra map[string]any) *Account {
	account := healthyGrokOAuthGatewayTestAccount(id, "access-token")
	account.Extra = extra
	account.Credentials["model_mapping"] = map[string]any{"grok-4.5": "grok-4.5"}
	return account
}

func TestForwardGrokPassthroughKeepsClientBodyAndReplacesAuth(t *testing.T) {
	gin.SetMode(gin.TestMode)

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	body := []byte(`{"model":"grok-4.6-new","input":[{"type":"message","role":"user","content":"hi"}],"reasoning":{"effort":"none"},"prompt_cache_key":"session-abc","stream":true}`)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Request.Header.Set("Authorization", "Bearer inbound-key")
	c.Request.Header.Set("User-Agent", "grok-shell/1.0.5")
	c.Request.Header.Set("x-grok-client-version", "1.0.5")
	c.Request.Header.Set("x-grok-client-identifier", "grok-shell")
	c.Request.Header.Set("x-grok-req-id", "req-passthrough")
	c.Request.Header.Set("x-grok-session-id", "sess-passthrough")
	c.Set("api_key", &APIKey{ID: 7701})

	account := grokPassthroughTestAccount(77, map[string]any{"grok_passthrough": true})
	upstreamBody := strings.Join([]string{
		"event: ping",
		`data: {"type":"ping"}`,
		"",
		`data: {"type":"response.output_text.delta","sequence_number":0,"delta":"ok"}`,
		"",
		`data: {"type":"response.completed","sequence_number":1,"response":{"id":"resp_pt","usage":{"input_tokens":2,"output_tokens":1}}}`,
		"",
	}, "\n")
	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
		Body:       io.NopCloser(strings.NewReader(upstreamBody)),
	}}
	svc := &OpenAIGatewayService{
		httpUpstream:      upstream,
		grokTokenProvider: NewGrokTokenProvider(nil, nil),
	}

	result, err := svc.forwardGrokResponses(context.Background(), c, account, body, "grok-4.6-new", true, time.Now())
	require.NoError(t, err)
	require.Equal(t, xai.DefaultCLIBaseURL+"/responses", upstream.lastReq.URL.String())
	require.Equal(t, "Bearer access-token", upstream.lastReq.Header.Get("Authorization"))
	require.Equal(t, "grok-shell/1.0.5", upstream.lastReq.Header.Get("User-Agent"))
	require.Equal(t, "1.0.5", upstream.lastReq.Header.Get("x-grok-client-version"))
	require.NotEqual(t, xai.CLIClientVersion, upstream.lastReq.Header.Get("x-grok-client-version"))
	require.Equal(t, xai.CLITokenAuth, upstream.lastReq.Header.Get("X-XAI-Token-Auth"))
	require.Equal(t, xai.CLIAuthenticateResponse, upstream.lastReq.Header.Get("x-authenticateresponse"))
	require.Equal(t, "req-passthrough", upstream.lastReq.Header.Get("x-grok-req-id"))
	require.Equal(t, "sess-passthrough", upstream.lastReq.Header.Get("x-grok-session-id"))
	require.Equal(t, "grok-4.6-new", gjson.GetBytes(upstream.lastBody, "model").String())
	require.Equal(t, "none", gjson.GetBytes(upstream.lastBody, "reasoning.effort").String())
	require.False(t, gjson.GetBytes(upstream.lastBody, "tools").Exists())
	require.NotEqual(t, "session-abc", gjson.GetBytes(upstream.lastBody, "prompt_cache_key").String())
	require.NotEmpty(t, gjson.GetBytes(upstream.lastBody, "prompt_cache_key").String())
	require.Equal(t, "resp_pt", result.ResponseID)
	require.NotContains(t, recorder.Body.String(), "event: ping")
	require.Contains(t, recorder.Body.String(), ": ping")
}

func TestForwardGrokPassthroughOffUsesCompatRewrite(t *testing.T) {
	gin.SetMode(gin.TestMode)

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	body := []byte(`{"model":"grok","input":"hi","stream":true}`)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set("api_key", &APIKey{ID: 7702})

	account := grokPassthroughTestAccount(78, nil)
	upstreamBody := strings.Join([]string{
		`data: {"type":"response.completed","response":{"id":"resp_compat","usage":{"input_tokens":1,"output_tokens":1}}}`,
		"",
	}, "\n")
	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
		Body:       io.NopCloser(strings.NewReader(upstreamBody)),
	}}
	svc := &OpenAIGatewayService{
		httpUpstream:      upstream,
		grokTokenProvider: NewGrokTokenProvider(nil, nil),
	}

	_, err := svc.forwardGrokResponses(context.Background(), c, account, body, "grok", true, time.Now())
	require.NoError(t, err)
	require.NotEqual(t, "grok", gjson.GetBytes(upstream.lastBody, "model").String())
	require.Equal(t, xai.CLIUserAgent(xai.ResolveCLIVersion()), upstream.lastReq.Header.Get("User-Agent"))
}

func TestFilterOpenAIResponsesNoneReasoningEffortPreservesGrokPassthrough(t *testing.T) {
	account := &Account{
		Platform: PlatformGrok,
		Type:     AccountTypeAPIKey,
		Extra:    map[string]any{"grok_passthrough": true},
	}
	got, err := filterOpenAIResponsesNoneReasoningEffortForAccount(account, []byte(`{"reasoning":{"effort":"none"}}`))
	require.NoError(t, err)
	require.Equal(t, "none", gjson.GetBytes(got, "reasoning.effort").String())
}

func TestApplyGrokPassthroughCacheIdentityKeepsClientKeyWhenUnresolved(t *testing.T) {
	body := []byte(`{"model":"grok-4.6","prompt_cache_key":"session-keep"}`)
	got, err := applyGrokPassthroughCacheIdentity(body, "")
	require.NoError(t, err)
	require.Equal(t, "session-keep", gjson.GetBytes(got, "prompt_cache_key").String())
}
