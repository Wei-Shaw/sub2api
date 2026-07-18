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
	"github.com/tidwall/gjson"
)

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

func TestOpenAIGatewayService_Forward_CompactTransientFailureDoesNotBlockRegularCapability(t *testing.T) {
	gin.SetMode(gin.TestMode)

	responses := make([]*http.Response, 2)
	for i := range responses {
		responses[i] = &http.Response{
			StatusCode: http.StatusBadGateway,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body:       io.NopCloser(strings.NewReader(`{"error":{"message":"temporary compact failure","type":"upstream_error"}}`)),
		}
	}
	upstream := &httpUpstreamRecorder{responses: responses}
	cfg := &config.Config{}
	cfg.Security.URLAllowlist.Enabled = false
	svc := &OpenAIGatewayService{cfg: cfg, httpUpstream: upstream}
	svc.rateLimitService = NewRateLimitService(transientCooldownAccountRepo{}, nil, cfg, nil, nil)
	account := &Account{
		ID:          4,
		Name:        "openai-apikey-compact",
		Platform:    PlatformOpenAI,
		Type:        AccountTypeAPIKey,
		Concurrency: 1,
		Credentials: map[string]any{
			"api_key":  "sk-test",
			"base_url": "https://example.com",
			"model_mapping": map[string]any{
				"public-alias":  "upstream-base",
				"upstream-base": "must-not-double-map",
			},
			"compact_model_mapping": map[string]any{"upstream-base": "upstream-compact"},
		},
		Extra:       map[string]any{"use_responses_api": true},
		Status:      StatusActive,
		Schedulable: true,
	}
	body := []byte(`{"model":"public-alias","stream":false,"instructions":"compact-test","input":"hello"}`)

	for range 2 {
		rec := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(rec)
		c.Request = httptest.NewRequest(http.MethodPost, "/openai/v1/responses/compact", bytes.NewReader(body))
		SetOpenAIClientTransport(c, OpenAIClientTransportHTTP)

		result, err := svc.Forward(context.Background(), c, account, body)
		require.Nil(t, result)
		require.Error(t, err)
	}

	require.Len(t, upstream.bodies, 2)
	for _, requestBody := range upstream.bodies {
		require.Equal(t, "upstream-compact", gjson.GetBytes(requestBody, "model").String())
	}
	require.True(t, svc.isOpenAIAccountModelRuntimeBlockedForCapability(account, "public-alias", true))
	require.False(t, svc.isOpenAIAccountModelRuntimeBlockedForCapability(account, "public-alias", false))
}

func TestOpenAIGatewayService_Forward_NestedCompactPathUsesRegularTransientScope(t *testing.T) {
	gin.SetMode(gin.TestMode)

	responses := make([]*http.Response, 2)
	for i := range responses {
		responses[i] = &http.Response{
			StatusCode: http.StatusBadGateway,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body:       io.NopCloser(strings.NewReader(`{"error":{"message":"temporary upstream failure","type":"upstream_error"}}`)),
		}
	}
	upstream := &httpUpstreamRecorder{responses: responses}
	cfg := &config.Config{}
	cfg.Security.URLAllowlist.Enabled = false
	svc := &OpenAIGatewayService{cfg: cfg, httpUpstream: upstream}
	svc.rateLimitService = NewRateLimitService(transientCooldownAccountRepo{}, nil, cfg, nil, nil)
	account := &Account{
		ID:          5,
		Name:        "openai-apikey-nested-compact",
		Platform:    PlatformOpenAI,
		Type:        AccountTypeAPIKey,
		Concurrency: 1,
		Credentials: map[string]any{
			"api_key":  "sk-test",
			"base_url": "https://example.com",
			"model_mapping": map[string]any{
				"public-alias": "upstream-base",
			},
			"compact_model_mapping": map[string]any{
				"upstream-base": "upstream-compact",
			},
		},
		Extra:       map[string]any{"use_responses_api": true},
		Status:      StatusActive,
		Schedulable: true,
	}
	body := []byte(`{"model":"public-alias","stream":false,"input":"hello"}`)

	for range 2 {
		rec := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(rec)
		c.Request = httptest.NewRequest(http.MethodPost, "/openai/v1/responses/compact/detail", bytes.NewReader(body))
		SetOpenAIClientTransport(c, OpenAIClientTransportHTTP)

		result, err := svc.Forward(context.Background(), c, account, body)
		require.Nil(t, result)
		require.Error(t, err)
	}

	require.Len(t, upstream.bodies, 2)
	for _, requestBody := range upstream.bodies {
		require.Equal(t, "upstream-base", gjson.GetBytes(requestBody, "model").String())
	}
	require.True(t, svc.isOpenAIAccountModelRuntimeBlockedForCapability(account, "public-alias", false))
	require.False(t, svc.isOpenAIAccountModelRuntimeBlockedForCapability(account, "public-alias", true))
}

func TestOpenAIGatewayService_APIKeyPassthrough_CompactCooldownUsesActualUpstreamModel(t *testing.T) {
	gin.SetMode(gin.TestMode)

	responses := make([]*http.Response, 3)
	for i := range responses[:2] {
		responses[i] = &http.Response{
			StatusCode: http.StatusBadGateway,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body:       io.NopCloser(strings.NewReader(`{"error":{"message":"temporary compact failure","type":"upstream_error"}}`)),
		}
	}
	responses[2] = &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(strings.NewReader(`{"id":"resp_passthrough_success","status":"completed","model":"public-alias","output":[],"usage":{"input_tokens":1,"output_tokens":1}}`)),
	}
	upstream := &httpUpstreamRecorder{responses: responses}
	cfg := &config.Config{}
	cfg.Security.URLAllowlist.Enabled = false
	svc := &OpenAIGatewayService{cfg: cfg, httpUpstream: upstream}
	svc.rateLimitService = NewRateLimitService(transientCooldownAccountRepo{}, nil, cfg, nil, nil)
	account := &Account{
		ID:          6,
		Name:        "openai-apikey-passthrough",
		Platform:    PlatformOpenAI,
		Type:        AccountTypeAPIKey,
		Concurrency: 1,
		Credentials: map[string]any{
			"api_key":  "sk-test",
			"base_url": "https://example.com",
			"model_mapping": map[string]any{
				"public-alias": "upstream-base",
			},
			"compact_model_mapping": map[string]any{
				"upstream-base": "upstream-compact",
			},
		},
		Extra: map[string]any{
			"openai_passthrough": true,
		},
		Status:      StatusActive,
		Schedulable: true,
	}
	body := []byte(`{"model":"public-alias","stream":false,"input":"hello"}`)

	for range 2 {
		rec := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(rec)
		c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses/compact", bytes.NewReader(body))
		SetOpenAIClientTransport(c, OpenAIClientTransportHTTP)

		result, err := svc.Forward(context.Background(), c, account, body)
		require.Nil(t, result)
		require.Error(t, err)
	}

	require.Len(t, upstream.bodies, 2)
	for _, requestBody := range upstream.bodies {
		require.Equal(t, "public-alias", gjson.GetBytes(requestBody, "model").String())
	}
	require.True(t, svc.isOpenAIAccountModelRuntimeBlockedForCapability(account, "public-alias", true))
	require.False(t, svc.isOpenAIAccountModelRuntimeBlockedForCapability(account, "public-alias", false))

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses/compact", bytes.NewReader(body))
	SetOpenAIClientTransport(c, OpenAIClientTransportHTTP)
	result, err := svc.Forward(context.Background(), c, account, body)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Equal(t, "public-alias", result.UpstreamModel)
	svc.ReportOpenAIAccountScheduleResultForCapability(account.ID, result.UpstreamModel, true, nil, true)
	require.False(t, svc.isOpenAIAccountModelRuntimeBlockedForCapability(account, "public-alias", true))
}

func TestResolveOpenAIAccountUpstreamModelForRequest_CompactMappingSkipsOAuthNormalization(t *testing.T) {
	account := &Account{
		Platform: PlatformOpenAI,
		Type:     AccountTypeOAuth,
		Credentials: map[string]any{
			"compact_model_mapping": map[string]any{
				"public-alias": "gpt-5.3",
			},
		},
	}

	require.Equal(t, "gpt-5.3", resolveOpenAIAccountUpstreamModelForRequest(account, "public-alias", true))
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
