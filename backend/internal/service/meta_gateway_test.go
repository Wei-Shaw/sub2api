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

	"github.com/Wei-Shaw/sub2api/internal/pkg/meta"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func metaGatewayTestAccount(model string) *Account {
	return &Account{
		ID:          5002,
		Name:        "meta-gateway",
		Platform:    PlatformMeta,
		Type:        AccountTypeAPIKey,
		Concurrency: 1,
		Credentials: map[string]any{
			"api_key":  "sk-muse",
			"base_url": "http://meta.example/v1",
		},
	}
}

func metaResponsesStreamFixture() string {
	return strings.Join([]string{
		`data: {"type":"response.output_text.delta","delta":"ok"}`,
		"",
		`data: {"type":"response.completed","response":{"id":"resp_meta","object":"response","model":"muse-spark-1.3","output":[],"usage":{"input_tokens":2,"output_tokens":1}}}`,
		"",
		"",
	}, string([]byte{10}))
}

func metaResponsesJSONFixture(model string) string {
	return `{"id":"resp_meta","object":"response","model":"` + model + `","output":[],"usage":{"input_tokens":2,"output_tokens":1}}`
}

func TestMetaForwardResponsesAppliesIdentityHeadersAndNamespace(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, stream := range []bool{false, true} {
		name := "non-stream"
		streamLiteral := "false"
		if stream {
			name = "stream"
			streamLiteral = "true"
		}
		t.Run(name, func(t *testing.T) {
			body := []byte(`{"model":"muse-spark-1.3","input":"hello","tools":[{"type":"function","name":"shell"}],"stream":` + streamLiteral + `}`)
			contentType := "application/json"
			responseBody := metaResponsesJSONFixture("muse-spark-1.3")
			if stream {
				contentType = "text/event-stream"
				responseBody = metaResponsesStreamFixture()
			}
			upstream := &httpUpstreamRecorder{resp: &http.Response{
				StatusCode: http.StatusOK,
				Header:     http.Header{"Content-Type": []string{contentType}},
				Body:       io.NopCloser(strings.NewReader(responseBody)),
			}}
			svc := &OpenAIGatewayService{cfg: rawChatCompletionsTestConfig(), httpUpstream: upstream}

			rec := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(rec)
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewReader(body))
			c.Request.Header.Set("Content-Type", "application/json")
			c.Request.Header.Set("User-Agent", "curl/8.5.0")

			result, err := svc.Forward(context.Background(), c, metaGatewayTestAccount("muse-spark-1.3"), body)
			require.NoError(t, err)
			require.NotNil(t, result)

			require.Equal(t, "http://meta.example/v1/responses", upstream.lastReq.URL.String())
			require.Equal(t, "Bearer sk-muse", upstream.lastReq.Header.Get("Authorization"))
			require.Equal(t, "tbh:exec", upstream.lastReq.Header.Get("X-Client-Id"))
			require.Equal(t, []string{meta.UserAgent()}, upstream.lastReq.Header.Values("User-Agent"))
			require.NotEmpty(t, upstream.lastReq.Header.Get("X-Tbh-Session-Id"))
			require.Equal(t, upstream.lastReq.Header.Get("X-Tbh-Session-Id"), upstream.lastReq.Header.Get("X-Tbh-Root-Session-Id"))
			require.Equal(t, "main", upstream.lastReq.Header.Get("X-Tbh-Agent-Role"))
			require.Empty(t, upstream.lastReq.Header.Get("X-Tbh-Contributor-Mode"))

			tools := gjson.GetBytes(upstream.lastBody, "tools").Array()
			require.Len(t, tools, 2)
			require.Equal(t, "shell", tools[0].Get("name").String())
			require.Equal(t, "muse", tools[1].Get("name").String())
		})
	}
}

func TestMetaForwardContributorModelSetsContributorHeader(t *testing.T) {
	gin.SetMode(gin.TestMode)
	body := []byte(`{"model":"muse-spark-1.3-contributor","input":"hello"}`)
	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(strings.NewReader(metaResponsesJSONFixture("muse-spark-1.3-contributor"))),
	}}
	svc := &OpenAIGatewayService{cfg: rawChatCompletionsTestConfig(), httpUpstream: upstream}

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewReader(body))

	_, err := svc.Forward(context.Background(), c, metaGatewayTestAccount("muse-spark-1.3-contributor"), body)
	require.NoError(t, err)
	require.Equal(t, "1", upstream.lastReq.Header.Get("X-Tbh-Contributor-Mode"))
}

func TestMetaChatCompletionsInboundRoutesToResponsesUpstream(t *testing.T) {
	gin.SetMode(gin.TestMode)
	body := []byte(`{"model":"muse-spark-1.3","messages":[{"role":"user","content":"hello"}],"stream":false}`)
	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
		Body:       io.NopCloser(strings.NewReader(metaResponsesStreamFixture())),
	}}
	svc := &OpenAIGatewayService{cfg: rawChatCompletionsTestConfig(), httpUpstream: upstream}

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(body))

	result, err := svc.forwardAsChatCompletions(context.Background(), c, metaGatewayTestAccount("muse-spark-1.3"), body, "", "", false)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Equal(t, "http://meta.example/v1/responses", upstream.lastReq.URL.String())
	require.Equal(t, "muse-spark-1.3", gjson.GetBytes(upstream.lastBody, "model").String())
	require.True(t, gjson.GetBytes(upstream.lastBody, "input").Exists())
}

func metaGatewayOAuthTestAccount(baseURL string) *Account {
	credentials := map[string]any{
		"api_key":   "sk-muse-oauth",
		"dca_token": "dca:test-token",
	}
	if baseURL != "" {
		credentials["base_url"] = baseURL
	}
	return &Account{
		ID:          5003,
		Name:        "meta-oauth-gateway",
		Platform:    PlatformMeta,
		Type:        AccountTypeOAuth,
		Concurrency: 1,
		Credentials: credentials,
	}
}

func TestMetaForwardOAuthAccountKeepsMuseIdentity(t *testing.T) {
	gin.SetMode(gin.TestMode)
	body := []byte(`{"model":"muse-spark-1.3","input":"hello","tools":[{"type":"function","name":"shell"}]}`)
	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(strings.NewReader(metaResponsesJSONFixture("muse-spark-1.3"))),
	}}
	svc := &OpenAIGatewayService{cfg: rawChatCompletionsTestConfig(), httpUpstream: upstream}

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Request.Header.Set("User-Agent", "curl/8.5.0")
	c.Request.Header.Set("Originator", "codex_cli_rs")
	c.Request.Header.Set("Session_id", "client-session")
	c.Request.Header.Set("OpenAI-Beta", "responses=experimental")

	result, err := svc.Forward(context.Background(), c, metaGatewayOAuthTestAccount(""), body)
	require.NoError(t, err)
	require.NotNil(t, result)

	require.Equal(t, "https://api.meta.ai/v1/responses", upstream.lastReq.URL.String())
	require.Equal(t, "api.meta.ai", upstream.lastReq.Host)
	require.Equal(t, "Bearer sk-muse-oauth", upstream.lastReq.Header.Get("Authorization"))
	require.Equal(t, []string{meta.UserAgent()}, upstream.lastReq.Header.Values("User-Agent"))
	require.Equal(t, "tbh:exec", upstream.lastReq.Header.Get("X-Client-Id"))
	require.Empty(t, upstream.lastReq.Header.Get("Originator"))
	require.Empty(t, upstream.lastReq.Header.Get("OpenAI-Beta"))
	require.Empty(t, upstream.lastReq.Header.Get("Session_id"))
	require.NotEmpty(t, upstream.lastReq.Header.Get("X-Tbh-Session-Id"))
	require.Equal(t, upstream.lastReq.Header.Get("X-Tbh-Session-Id"), upstream.lastReq.Header.Get("X-Meta-Ai-Gateway-Session-Id"))
	require.Equal(t, "main", upstream.lastReq.Header.Get("X-Tbh-Agent-Role"))

	tools := gjson.GetBytes(upstream.lastBody, "tools").Array()
	require.Len(t, tools, 2)
	require.Equal(t, "shell", tools[0].Get("name").String())
	require.Equal(t, "muse", tools[1].Get("name").String())
}

func TestMetaForwardOAuthAccountRejectsDisallowedBaseURL(t *testing.T) {
	gin.SetMode(gin.TestMode)
	cfg := rawChatCompletionsTestConfig()
	cfg.Security.URLAllowlist.Enabled = true
	cfg.Security.URLAllowlist.UpstreamHosts = []string{"api.meta.ai"}
	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(strings.NewReader(metaResponsesJSONFixture("muse-spark-1.3"))),
	}}
	svc := &OpenAIGatewayService{cfg: cfg, httpUpstream: upstream}
	body := []byte(`{"model":"muse-spark-1.3","input":"hello"}`)

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	_, err := svc.Forward(context.Background(), c, metaGatewayOAuthTestAccount("https://metadata.internal.example/v1"), body)
	require.Error(t, err)
	require.Nil(t, upstream.lastReq)
}
