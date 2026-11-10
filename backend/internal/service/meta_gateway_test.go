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

			result, err := svc.Forward(context.Background(), c, metaGatewayTestAccount("muse-spark-1.3"), body)
			require.NoError(t, err)
			require.NotNil(t, result)

			require.Equal(t, "http://meta.example/v1/responses", upstream.lastReq.URL.String())
			require.Equal(t, "Bearer sk-muse", upstream.lastReq.Header.Get("Authorization"))
			require.Equal(t, "tbh:exec", upstream.lastReq.Header.Get("X-Client-Id"))
			require.Contains(t, upstream.lastReq.Header.Get("User-Agent"), "muse-build/1.4.14")
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
