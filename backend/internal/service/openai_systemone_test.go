package service

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestBuildOpenAISystemOneURL(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		base string
		want string
	}{
		{"bare domain", "https://api.typesafe.ai", "https://api.typesafe.ai/v1/systemone"},
		{"bare /v1", "https://api.typesafe.ai/v1", "https://api.typesafe.ai/v1/systemone"},
		// 与 embeddings/rerank 同语义：base_url 已带版本段时只追加端点相对路径。
		{"third-party versioned path", "https://gateway.example.com/api/paas/v4", "https://gateway.example.com/api/paas/v4/systemone"},
		{"already systemone", "https://api.typesafe.ai/v1/systemone", "https://api.typesafe.ai/v1/systemone"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tt.want, buildOpenAISystemOneURL(tt.base))
		})
	}
}

func TestExtractOpenAISystemOneUsage(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		body       string
		wantInput  int
		wantOutput int
	}{
		{
			// TypeSafe Jev 判断题的官方响应形态。
			name:       "typesafe input output tokens",
			body:       `{"model":"jev-judge-v1","usage":{"input_tokens":1234,"output_tokens":56},"answers":{"q1":true}}`,
			wantInput:  1234,
			wantOutput: 56,
		},
		{
			name:       "openai style prompt completion tokens",
			body:       `{"usage":{"prompt_tokens":17,"completion_tokens":9,"total_tokens":26}}`,
			wantInput:  17,
			wantOutput: 9,
		},
		{
			name:       "total tokens fallback",
			body:       `{"usage":{"total_tokens":26}}`,
			wantInput:  26,
			wantOutput: 0,
		},
		{
			name:       "no usage",
			body:       `{"model":"jev-judge-v1","answers":{"q1":false}}`,
			wantInput:  0,
			wantOutput: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			usage := extractOpenAISystemOneUsage([]byte(tt.body))
			require.Equal(t, tt.wantInput, usage.InputTokens)
			require.Equal(t, tt.wantOutput, usage.OutputTokens)
		})
	}
}

func newSystemOneTestAccount(credentials map[string]any) *Account {
	return &Account{
		ID:          4242,
		Name:        "typesafe-acc",
		Platform:    PlatformTypeSafe,
		Type:        AccountTypeAPIKey,
		Credentials: credentials,
	}
}

func TestForwardSystemOne_APIKeyPassthroughRecordsUsage(t *testing.T) {
	gin.SetMode(gin.TestMode)

	reqBody := []byte(`{
		"model":"jev-judge-v1",
		"state":"the customer asked for a refund twice",
		"questions":[{"id":"q1","text":"is the customer angry?"},{"id":"q2","text":"did they pay?"}]
	}`)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/systemone", bytes.NewReader(reqBody))
	c.Request.Header.Set("Content-Type", "application/json")

	upstreamBody := `{"model":"jev-judge-v1-2026","usage":{"input_tokens":1234,"output_tokens":56},"answers":{"q1":true,"q2":false}}`
	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusOK,
		Header: http.Header{
			"Content-Type": []string{"application/json"},
			"X-Request-Id": []string{"systemone-rid"},
		},
		Body: io.NopCloser(strings.NewReader(upstreamBody)),
	}}
	svc := &OpenAIGatewayService{
		cfg:          &config.Config{},
		httpUpstream: upstream,
	}
	account := newSystemOneTestAccount(map[string]any{
		"api_key":  "ts-test-key",
		"base_url": "https://upstream.example.com/v1",
		"model_mapping": map[string]any{
			"jev-judge-v1": "jev-judge-v1-2026",
		},
	})

	result, err := svc.ForwardSystemOne(context.Background(), c, account, reqBody, "")

	require.NoError(t, err)
	require.Equal(t, http.StatusOK, rec.Code)
	// 成功响应体原样回写。
	require.Equal(t, upstreamBody, rec.Body.String())
	require.Equal(t, "application/json", rec.Header().Get("Content-Type"))
	require.NotNil(t, result)
	require.Equal(t, "systemone-rid", result.RequestID)
	require.Equal(t, false, result.Stream)
	require.Equal(t, "jev-judge-v1", result.Model)
	// 账号级模型映射：计费用映射名，上游也收到映射名。
	require.Equal(t, "jev-judge-v1-2026", result.BillingModel)
	require.Equal(t, "jev-judge-v1-2026", result.UpstreamModel)
	// TypeSafe usage{input_tokens,output_tokens} 命中取值链第二档。
	require.Equal(t, 1234, result.Usage.InputTokens)
	require.Equal(t, 56, result.Usage.OutputTokens)

	require.Equal(t, "https://upstream.example.com/v1/systemone", upstream.lastReq.URL.String())
	require.Equal(t, "Bearer ts-test-key", upstream.lastReq.Header.Get("Authorization"))
	require.Equal(t, "application/json", upstream.lastReq.Header.Get("Content-Type"))
	// 除 model 映射外逐字节原样透传：state/questions 必须一字不改。
	require.JSONEq(t, `{
		"model":"jev-judge-v1-2026",
		"state":"the customer asked for a refund twice",
		"questions":[{"id":"q1","text":"is the customer angry?"},{"id":"q2","text":"did they pay?"}]
	}`, string(upstream.lastBody))
	require.Equal(t, "the customer asked for a refund twice", gjson.GetBytes(upstream.lastBody, "state").String())
	require.Len(t, gjson.GetBytes(upstream.lastBody, "questions").Array(), 2)
}

func TestForwardSystemOne_UnmappedBodyPassesThroughUnchanged(t *testing.T) {
	gin.SetMode(gin.TestMode)

	reqBody := []byte(`{"model":"jev-judge-v1","state":"s","questions":[{"id":"q1","text":"t"}]}`)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/systemone", bytes.NewReader(reqBody))

	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(strings.NewReader(`{"model":"jev-judge-v1","usage":{"input_tokens":5,"output_tokens":1},"answers":{}}`)),
	}}
	svc := &OpenAIGatewayService{cfg: &config.Config{}, httpUpstream: upstream}
	account := newSystemOneTestAccount(map[string]any{
		"api_key":  "ts-test-key",
		"base_url": "https://upstream.example.com/v1",
	})

	result, err := svc.ForwardSystemOne(context.Background(), c, account, reqBody, "")

	require.NoError(t, err)
	// 账号没有 model_mapping：出站请求体与入站逐字节相同。
	require.Equal(t, reqBody, upstream.lastBody)
	require.Equal(t, "https://upstream.example.com/v1/systemone", upstream.lastReq.URL.String())
	require.NotNil(t, result)
	require.Equal(t, 5, result.Usage.InputTokens)
	require.Equal(t, 1, result.Usage.OutputTokens)
	require.Equal(t, "jev-judge-v1", result.UpstreamModel)
}

func TestForwardSystemOne_DefaultBaseURLDoesNotFallBackToOpenAI(t *testing.T) {
	gin.SetMode(gin.TestMode)

	reqBody := []byte(`{"model":"jev-judge-v1","state":"s","questions":[]}`)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/systemone", bytes.NewReader(reqBody))

	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(strings.NewReader(`{"usage":{"input_tokens":1,"output_tokens":0}}`)),
	}}
	svc := &OpenAIGatewayService{cfg: &config.Config{}, httpUpstream: upstream}
	// 未配置 base_url：必须回落 DefaultTypeSafeBaseURL，绝不能是 api.openai.com。
	account := newSystemOneTestAccount(map[string]any{"api_key": "ts-test-key"})

	result, err := svc.ForwardSystemOne(context.Background(), c, account, reqBody, "")

	require.NoError(t, err)
	require.NotNil(t, result)
	require.Equal(t, "https://api.typesafe.ai/v1/systemone", upstream.lastReq.URL.String())
	require.NotContains(t, upstream.lastReq.URL.String(), "api.openai.com")
}

// TestForwardSystemOne_UpstreamErrorDoesNotLeakUpstreamBody 硬约束回归：
// TypeSafe 的错误体可能回显请求内容甚至凭据，绝不能出现在客户端响应里。
func TestForwardSystemOne_UpstreamErrorDoesNotLeakUpstreamBody(t *testing.T) {
	gin.SetMode(gin.TestMode)

	const sentinel = "SECRET-ECHO-REQUEST-BODY-4711"
	reqBody := []byte(`{"model":"jev-judge-v1","state":"` + sentinel + `","questions":[]}`)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/systemone", bytes.NewReader(reqBody))

	// 上游 404 属不可 failover 的状态：走「自造错误体」分支。
	upstreamErrorBody := `{"error":{"message":"invalid payload: ` + sentinel + `","authorization":"Bearer ts-test-key"}}`
	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusNotFound,
		Header: http.Header{
			"Content-Type": []string{"application/json"},
			"X-Request-Id": []string{"upstream-rid-404"},
		},
		Body: io.NopCloser(strings.NewReader(upstreamErrorBody)),
	}}
	svc := &OpenAIGatewayService{cfg: &config.Config{}, httpUpstream: upstream}
	account := newSystemOneTestAccount(map[string]any{
		"api_key":  "ts-test-key",
		"base_url": "https://upstream.example.com/v1",
	})

	result, err := svc.ForwardSystemOne(context.Background(), c, account, reqBody, "")

	require.Error(t, err)
	require.Nil(t, result)
	require.Equal(t, http.StatusNotFound, rec.Code)
	body := rec.Body.String()
	require.NotContains(t, body, sentinel, "上游错误体不得回显请求内容")
	require.NotContains(t, body, "ts-test-key", "上游错误体不得回显凭据")
	require.NotContains(t, body, "invalid payload")
	require.Contains(t, body, "Upstream returned status 404")
	require.Contains(t, body, "upstream-rid-404")
	require.JSONEq(t, `{"error":{"type":"upstream_error","message":"Upstream returned status 404 (upstream request id: upstream-rid-404)"}}`, body)
}

// TestForwardSystemOne_FailoverErrorCarriesNoUpstreamBody 回归：
// failover 分支携带的 ResponseBody 同样必须是自造错误体——它会被
// handleFailoverExhausted 的透传规则写出到客户端。
func TestForwardSystemOne_FailoverErrorCarriesNoUpstreamBody(t *testing.T) {
	gin.SetMode(gin.TestMode)

	const sentinel = "SECRET-ECHO-REQUEST-BODY-0815"
	reqBody := []byte(`{"model":"jev-judge-v1","state":"` + sentinel + `","questions":[]}`)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/systemone", bytes.NewReader(reqBody))

	upstreamErrorBody := `{"error":{"message":"rate limited: ` + sentinel + `"}}`
	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusTooManyRequests,
		Header: http.Header{
			"Content-Type": []string{"application/json"},
			"X-Request-Id": []string{"upstream-rid-429"},
		},
		Body: io.NopCloser(strings.NewReader(upstreamErrorBody)),
	}}
	svc := &OpenAIGatewayService{cfg: &config.Config{}, httpUpstream: upstream}
	account := newSystemOneTestAccount(map[string]any{
		"api_key":  "ts-test-key",
		"base_url": "https://upstream.example.com/v1",
	})

	result, err := svc.ForwardSystemOne(context.Background(), c, account, reqBody, "")

	require.Error(t, err)
	require.Nil(t, result)
	// 尚未向客户端写任何内容：failover 由 handler 决定后续动作。
	require.Zero(t, rec.Body.Len())

	var failoverErr *UpstreamFailoverError
	require.True(t, errors.As(err, &failoverErr))
	require.Equal(t, http.StatusTooManyRequests, failoverErr.StatusCode)
	require.NotContains(t, string(failoverErr.ResponseBody), sentinel)
	require.NotContains(t, string(failoverErr.ResponseBody), "rate limited")
	require.Contains(t, string(failoverErr.ResponseBody), "Upstream returned status 429")
	require.Contains(t, string(failoverErr.ResponseBody), "upstream-rid-429")
}
