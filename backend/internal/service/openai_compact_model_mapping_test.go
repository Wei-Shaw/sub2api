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
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/tlsfingerprint"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

type compactTimeoutHTTPUpstream struct{}

func (u *compactTimeoutHTTPUpstream) Do(req *http.Request, _ string, _ int64, _ int) (*http.Response, error) {
	return http.DefaultClient.Do(req)
}

func (u *compactTimeoutHTTPUpstream) DoWithTLS(req *http.Request, proxyURL string, accountID int64, accountConcurrency int, _ *tlsfingerprint.Profile) (*http.Response, error) {
	return u.Do(req, proxyURL, accountID, accountConcurrency)
}

func testOpenAICompactGatewayConfig(timeoutSeconds int) *config.Config {
	return &config.Config{
		Gateway: config.GatewayConfig{
			OpenAICompactTimeoutSeconds: timeoutSeconds,
		},
		Security: config.SecurityConfig{
			URLAllowlist: config.URLAllowlistConfig{
				Enabled:           false,
				AllowInsecureHTTP: true,
			},
		},
	}
}

func TestOpenAIGatewayService_Forward_CompactOnlyModelMappingOverridesOAuthUpstreamModel(t *testing.T) {
	gin.SetMode(gin.TestMode)

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	body := []byte(`{"model":"gpt-5.4","stream":false,"instructions":"compact-test","input":"hello"}`)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses/compact", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/json"}, "x-request-id": []string{"rid-compact-map"}},
		Body:       io.NopCloser(strings.NewReader(`{"id":"resp_123","status":"completed","model":"gpt-5.4-openai-compact","output":[],"usage":{"input_tokens":1,"output_tokens":1}}`)),
	}}

	svc := &OpenAIGatewayService{httpUpstream: upstream}
	account := &Account{
		ID:          1,
		Name:        "openai-oauth",
		Platform:    PlatformOpenAI,
		Type:        AccountTypeOAuth,
		Concurrency: 1,
		Credentials: map[string]any{
			"access_token":          "oauth-token",
			"chatgpt_account_id":    "chatgpt-acc",
			"compact_model_mapping": map[string]any{"gpt-5.4": "gpt-5.4-openai-compact"},
		},
		Status:      StatusActive,
		Schedulable: true,
	}

	result, err := svc.Forward(context.Background(), c, account, body)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Equal(t, "gpt-5.4", result.Model)
	require.Equal(t, "gpt-5.4-openai-compact", result.UpstreamModel)
	require.Equal(t, "gpt-5.4-openai-compact", gjson.GetBytes(upstream.lastBody, "model").String())
}

func TestOpenAIGatewayService_Forward_NonCompactRequestIgnoresCompactOnlyModelMapping(t *testing.T) {
	gin.SetMode(gin.TestMode)

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	body := []byte(`{"model":"gpt-5.4","stream":false,"instructions":"normal-test","input":"hello"}`)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/json"}, "x-request-id": []string{"rid-normal-map"}},
		Body:       io.NopCloser(strings.NewReader(`{"id":"resp_124","status":"completed","model":"gpt-5.4","output":[],"usage":{"input_tokens":1,"output_tokens":1}}`)),
	}}

	svc := &OpenAIGatewayService{httpUpstream: upstream}
	account := &Account{
		ID:          2,
		Name:        "openai-oauth",
		Platform:    PlatformOpenAI,
		Type:        AccountTypeOAuth,
		Concurrency: 1,
		Credentials: map[string]any{
			"access_token":          "oauth-token",
			"chatgpt_account_id":    "chatgpt-acc",
			"compact_model_mapping": map[string]any{"gpt-5.4": "gpt-5.4-openai-compact"},
		},
		Status:      StatusActive,
		Schedulable: true,
	}

	result, err := svc.Forward(context.Background(), c, account, body)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Equal(t, "gpt-5.4", result.Model)
	require.Equal(t, "gpt-5.4", result.UpstreamModel)
	require.Equal(t, "gpt-5.4", gjson.GetBytes(upstream.lastBody, "model").String())
}

func TestOpenAIGatewayService_OAuthPassthrough_CompactOnlyModelMappingOverridesUpstreamModel(t *testing.T) {
	gin.SetMode(gin.TestMode)

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses/compact", bytes.NewReader(nil))
	c.Request.Header.Set("User-Agent", "codex_cli_rs/0.1.0")
	c.Request.Header.Set("Content-Type", "application/json")

	originalBody := []byte(`{"model":"gpt-5.4","stream":true,"store":true,"instructions":"compact-pass","input":[{"type":"text","text":"compact me"}]}`)
	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/json"}, "x-request-id": []string{"rid-compact-pass-map"}},
		Body:       io.NopCloser(strings.NewReader(`{"id":"cmp_124","model":"gpt-5.4-openai-compact","usage":{"input_tokens":2,"output_tokens":3}}`)),
	}}

	svc := &OpenAIGatewayService{httpUpstream: upstream}
	account := &Account{
		ID:          3,
		Name:        "openai-oauth-pass",
		Platform:    PlatformOpenAI,
		Type:        AccountTypeOAuth,
		Concurrency: 1,
		Credentials: map[string]any{
			"access_token":          "oauth-token",
			"chatgpt_account_id":    "chatgpt-acc",
			"compact_model_mapping": map[string]any{"gpt-5.4": "gpt-5.4-openai-compact"},
		},
		Extra:       map[string]any{"openai_passthrough": true},
		Status:      StatusActive,
		Schedulable: true,
	}

	result, err := svc.Forward(context.Background(), c, account, originalBody)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Equal(t, "gpt-5.4", result.Model)
	require.Equal(t, "gpt-5.4-openai-compact", result.UpstreamModel)
	require.Equal(t, "gpt-5.4-openai-compact", gjson.GetBytes(upstream.lastBody, "model").String())
	require.Equal(t, "gpt-5.4", gjson.GetBytes(rec.Body.Bytes(), "model").String())
}

func TestOpenAIGatewayService_Forward_CompactTimeoutReturnsStructuredGatewayTimeout(t *testing.T) {
	gin.SetMode(gin.TestMode)

	upstreamServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(2 * time.Second)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"resp_late","status":"completed","model":"gpt-5.5","output":[],"usage":{"input_tokens":1,"output_tokens":1}}`))
	}))
	defer upstreamServer.CloseClientConnections()
	defer upstreamServer.Close()

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	body := []byte(`{"model":"gpt-5.5","stream":false,"instructions":"compact-test","input":"hello"}`)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses/compact", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Request.Header.Set("CF-Ray", "9fb7c368a8031fc4-HKG")

	svc := &OpenAIGatewayService{
		httpUpstream: &compactTimeoutHTTPUpstream{},
		cfg:          testOpenAICompactGatewayConfig(1),
	}
	account := &Account{
		ID:          11,
		Name:        "openai-apikey",
		Platform:    PlatformOpenAI,
		Type:        AccountTypeAPIKey,
		Concurrency: 1,
		Credentials: map[string]any{
			"api_key":  "test-key",
			"base_url": upstreamServer.URL,
		},
		Status:      StatusActive,
		Schedulable: true,
	}

	startedAt := time.Now()
	result, err := svc.Forward(context.Background(), c, account, body)

	require.Nil(t, result)
	var timeoutErr *OpenAICompactTimeoutError
	require.True(t, errors.As(err, &timeoutErr), "err = %v", err)
	require.GreaterOrEqual(t, time.Since(startedAt), time.Second)
	require.Equal(t, http.StatusGatewayTimeout, rec.Code)
	require.Equal(t, "upstream_timeout", gjson.Get(rec.Body.String(), "error.type").String())
	require.Equal(t, "upstream_wait_timeout", gjson.Get(rec.Body.String(), "error.stage").String())
	require.True(t, gjson.Get(rec.Body.String(), "error.retryable").Bool())
	require.Equal(t, "9fb7c368a8031fc4-HKG", gjson.Get(rec.Body.String(), "error.cf_ray").String())

	errorClass, ok := c.Get(OpenAICompactUpstreamErrorClassKey)
	require.True(t, ok)
	require.Equal(t, "upstream_wait_timeout", errorClass)
	timeoutMs, ok := c.Get(OpenAICompactTimeoutMsKey)
	require.True(t, ok)
	require.Equal(t, int64(1000), timeoutMs)
}

func TestOpenAIGatewayService_Forward_NonCompactRequestDoesNotUseCompactTimeout(t *testing.T) {
	gin.SetMode(gin.TestMode)

	upstreamServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(1100 * time.Millisecond)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"resp_normal","status":"completed","model":"gpt-5.5","output":[],"usage":{"input_tokens":1,"output_tokens":1}}`))
	}))
	defer upstreamServer.Close()

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	body := []byte(`{"model":"gpt-5.5","stream":false,"instructions":"normal-test","input":"hello"}`)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	svc := &OpenAIGatewayService{
		httpUpstream: &compactTimeoutHTTPUpstream{},
		cfg:          testOpenAICompactGatewayConfig(1),
	}
	account := &Account{
		ID:          12,
		Name:        "openai-apikey",
		Platform:    PlatformOpenAI,
		Type:        AccountTypeAPIKey,
		Concurrency: 1,
		Credentials: map[string]any{
			"api_key":  "test-key",
			"base_url": upstreamServer.URL,
		},
		Status:      StatusActive,
		Schedulable: true,
	}

	result, err := svc.Forward(context.Background(), c, account, body)

	require.NoError(t, err)
	require.NotNil(t, result)
	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, "resp_normal", gjson.Get(rec.Body.String(), "id").String())
	_, hasCompactTimeout := c.Get(OpenAICompactTimeoutMsKey)
	require.False(t, hasCompactTimeout)
}

func TestOpenAIGatewayService_CompactTimeoutContextIgnoresParentCancel(t *testing.T) {
	svc := &OpenAIGatewayService{cfg: testOpenAICompactGatewayConfig(1)}
	parent, cancelParent := context.WithCancel(context.Background())

	upstreamCtx, release := svc.openAICompactAwareUpstreamContext(parent, true)
	defer release()
	cancelParent()

	select {
	case <-upstreamCtx.Done():
		require.Fail(t, "compact upstream context should not inherit parent cancellation")
	case <-time.After(50 * time.Millisecond):
	}
	require.NoError(t, upstreamCtx.Err())
}

func TestResolveOpenAICompactForwardModel_UsesCompactOnlyMapping(t *testing.T) {
	account := &Account{Credentials: map[string]any{
		"compact_model_mapping": map[string]any{"gpt-5.4": "gpt-5.4-openai-compact"},
	}}

	require.Equal(t, "gpt-5.4-openai-compact", resolveOpenAICompactForwardModel(account, "gpt-5.4"))
}

func TestResolveOpenAICompactForwardModel_IgnoresRegularModelMapping(t *testing.T) {
	account := &Account{Credentials: map[string]any{
		"model_mapping":         map[string]any{"gpt-5.4": "gpt-5.4-regular"},
		"compact_model_mapping": map[string]any{"gpt-5.4": "gpt-5.4-openai-compact"},
	}}

	require.Equal(t, "gpt-5.4-openai-compact", resolveOpenAICompactForwardModel(account, "gpt-5.4"))
}

func TestResolveOpenAICompactForwardModel_FallsBackToRequestedModel(t *testing.T) {
	account := &Account{Credentials: map[string]any{
		"compact_model_mapping": map[string]any{"gpt-5.4": "gpt-5.4-openai-compact"},
	}}

	require.Equal(t, "gpt-5.3-codex", resolveOpenAICompactForwardModel(account, "gpt-5.3-codex"))
}

func TestResolveOpenAICompactForwardModel_EmptyMappedModelFallsBackToRequestedModel(t *testing.T) {
	account := &Account{Credentials: map[string]any{
		"compact_model_mapping": map[string]any{"gpt-5.4": "   "},
	}}

	require.Equal(t, "gpt-5.4", resolveOpenAICompactForwardModel(account, "gpt-5.4"))
}
