package service

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/apicompat"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

type countTokensRuntimeStateRepo struct {
	AccountRepository
	tempUnschedCalls int
	setErrorCalls    int
}

func (r *countTokensRuntimeStateRepo) SetTempUnschedulable(_ context.Context, _ int64, _ time.Time, _ string) error {
	r.tempUnschedCalls++
	return nil
}

func (r *countTokensRuntimeStateRepo) SetError(_ context.Context, _ int64, _ string) error {
	r.setErrorCalls++
	return nil
}

func TestOpenAIGatewayService_ForwardCountTokensAsAnthropic_APIKeyUsesResponsesInputTokens(t *testing.T) {
	gin.SetMode(gin.TestMode)

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	body := []byte(`{"model":"claude-sonnet-4-5","system":"You are helpful.","messages":[{"role":"user","content":"hello"}],"tools":[{"name":"lookup","input_schema":{"type":"object"}}]}`)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages/count_tokens", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(strings.NewReader(`{"object":"response.input_tokens","input_tokens":42}`)),
	}}

	svc := &OpenAIGatewayService{
		cfg: &config.Config{Security: config.SecurityConfig{URLAllowlist: config.URLAllowlistConfig{
			Enabled:           false,
			AllowInsecureHTTP: true,
		}}},
		httpUpstream: upstream,
	}
	account := &Account{
		ID:          101,
		Name:        "openai-apikey",
		Platform:    PlatformOpenAI,
		Type:        AccountTypeAPIKey,
		Concurrency: 1,
		Credentials: map[string]any{
			"api_key":  "sk-test",
			"base_url": "http://upstream.example",
		},
		Status:      StatusActive,
		Schedulable: true,
	}

	err := svc.ForwardCountTokensAsAnthropic(context.Background(), c, account, body, "gpt-5.3-codex")
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, rec.Code)
	require.JSONEq(t, `{"input_tokens":42}`, rec.Body.String())
	require.NotNil(t, upstream.lastReq)
	require.Equal(t, "http://upstream.example/v1/responses/input_tokens", upstream.lastReq.URL.String())
	require.Equal(t, "Bearer sk-test", upstream.lastReq.Header.Get("authorization"))
	require.Equal(t, "gpt-5.3-codex", gjson.GetBytes(upstream.lastBody, "model").String())
	require.True(t, gjson.GetBytes(upstream.lastBody, "input").Exists())
	require.False(t, gjson.GetBytes(upstream.lastBody, "messages").Exists())
}

func TestOpenAIGatewayService_ForwardCountTokensAsAnthropic_OAuthFallsBackWhenPlatformEndpointUnsupported(t *testing.T) {
	gin.SetMode(gin.TestMode)

	body := []byte(`{"model":"claude-opus-4-1","messages":[{"role":"user","content":"hello"}]}`)
	account := &Account{
		ID:          202,
		Name:        "openai-oauth",
		Platform:    PlatformOpenAI,
		Type:        AccountTypeOAuth,
		Concurrency: 1,
		Credentials: map[string]any{
			"access_token":  "oauth-token",
			"refresh_token": "oauth-refresh-token",
		},
		Status:      StatusActive,
		Schedulable: true,
	}

	prepared, err := prepareOpenAIInputTokensCountRequest(body, account, "gpt-5.4")
	require.NoError(t, err)
	expectedEstimate, err := estimateOpenAIInputTokens(prepared.Request)
	require.NoError(t, err)

	cases := []struct {
		name       string
		statusCode int
		body       string
	}{
		{
			name:       "401_missing_responses_write_scope",
			statusCode: http.StatusUnauthorized,
			body:       `{"error":{"type":"invalid_request_error","code":"missing_scope","message":"You have insufficient permissions for this operation. Missing scopes: api.responses.write."}}`,
		},
		{
			name:       "403_missing_responses_write_scope",
			statusCode: http.StatusForbidden,
			body:       `{"error":{"type":"invalid_request_error","code":"missing_scope","message":"Missing scopes: api.responses.write"}}`,
		},
		{
			name:       "404_input_tokens_unsupported",
			statusCode: http.StatusNotFound,
			body:       `{"error":{"type":"invalid_request_error","message":"The /v1/responses/input_tokens endpoint was not found"}}`,
		},
	}

	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(rec)
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages/count_tokens", bytes.NewReader(body))
			c.Request.Header.Set("Content-Type", "application/json")
			c.Request.Header.Set("User-Agent", "Claude-Code/1.0")

			upstream := &httpUpstreamRecorder{resp: &http.Response{
				StatusCode: tt.statusCode,
				Header:     http.Header{"Content-Type": []string{"application/json"}},
				Body:       io.NopCloser(strings.NewReader(tt.body)),
			}}
			repo := &countTokensRuntimeStateRepo{}
			svc := &OpenAIGatewayService{
				cfg:              &config.Config{},
				httpUpstream:     upstream,
				rateLimitService: &RateLimitService{accountRepo: repo, cfg: &config.Config{}},
			}

			err := svc.ForwardCountTokensAsAnthropic(context.Background(), c, account, body, "gpt-5.4")
			require.NoError(t, err)
			require.Equal(t, http.StatusOK, rec.Code)
			require.JSONEq(t, `{"input_tokens":`+strconv.Itoa(expectedEstimate)+`}`, rec.Body.String())
			require.NotNil(t, upstream.lastReq)
			require.Equal(t, "https://api.openai.com/v1/responses/input_tokens", upstream.lastReq.URL.String())
			require.Equal(t, "Bearer oauth-token", upstream.lastReq.Header.Get("authorization"))
			require.Empty(t, upstream.lastReq.Header.Get("Chatgpt-Account-Id"))
			require.Zero(t, repo.tempUnschedCalls, "OAuth input_tokens unsupported errors must not temp-unschedule the account")
			require.Zero(t, repo.setErrorCalls, "OAuth input_tokens unsupported errors must not mark the account error")
		})
	}
}

func TestOpenAIGatewayService_OpenAIOAuthInputTokensFallbackUsesMinimumWhenEstimateFails(t *testing.T) {
	gin.SetMode(gin.TestMode)

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	prepared := &openAIInputTokensCountPrepared{
		Request: openAIInputTokensCountRequest{
			Model: "gpt-5",
			Input: json.RawMessage(`[`),
		},
		UpstreamModel: "gpt-5",
	}

	writeOpenAIOAuthInputTokensFallback(c, &Account{ID: 303}, prepared, http.StatusUnauthorized)

	require.Equal(t, http.StatusOK, rec.Code)
	require.JSONEq(t, `{"input_tokens":1}`, rec.Body.String())
}

func TestEstimateOpenAIInputTokens_RequestSamples(t *testing.T) {
	cases := []struct {
		name string
		req  openAIInputTokensCountRequest
		want int
	}{
		{
			name: "simple text input",
			req: openAIInputTokensCountRequest{
				Model: "gpt-5",
				Input: json.RawMessage(`[{"role":"user","content":"hello world"}]`),
			},
			want: 6,
		},
		{
			name: "instructions plus tool schema",
			req: openAIInputTokensCountRequest{
				Model:        "gpt-5",
				Instructions: "You are helpful.",
				Input:        json.RawMessage(`[{"role":"user","content":"lookup weather in shanghai"}]`),
				Tools: []apicompat.ResponsesTool{
					{
						Type:        "function",
						Name:        "lookup_weather",
						Description: "Look up current weather",
						Parameters:  json.RawMessage(`{"type":"object","properties":{"city":{"type":"string"}},"required":["city"]}`),
					},
				},
			},
			want: 50,
		},
		{
			name: "input parts and tool output",
			req: openAIInputTokensCountRequest{
				Model: "gpt-4.1",
				Input: json.RawMessage(`[
					{"role":"user","content":[{"type":"input_text","text":"first line"},{"type":"input_text","text":"second line"}]},
					{"type":"function_call_output","call_id":"call_123","output":"{\"ok\":true}"}
				]`),
			},
			want: 24,
		},
	}

	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			got, err := estimateOpenAIInputTokens(tt.req)
			require.NoError(t, err)
			require.Equal(t, tt.want, got)
		})
	}
}

func TestOpenAIInputTokensEncodingForModel(t *testing.T) {
	cases := []struct {
		model string
		want  string
	}{
		{model: "gpt-5", want: "o200k_base"},
		{model: "gpt-5.3-codex", want: "o200k_base"},
		{model: "gpt-4o-mini", want: "o200k_base"},
		{model: "gpt-4.1", want: "o200k_base"},
		{model: "gpt-4-turbo", want: "cl100k_base"},
		{model: "gpt-3.5-turbo", want: "cl100k_base"},
	}

	for _, tt := range cases {
		t.Run(tt.model, func(t *testing.T) {
			require.Equal(t, tt.want, string(openAIInputTokensEncodingForModel(tt.model)))
		})
	}
}

func TestEstimateOpenAIInputTokens_CompareWithOpenAIAPI(t *testing.T) {
	apiKey := strings.TrimSpace(os.Getenv("OPENAI_API_KEY"))
	if apiKey == "" {
		t.Skip("OPENAI_API_KEY not set")
	}

	client := &http.Client{Timeout: 30 * time.Second}
	cases := []struct {
		name               string
		anthropicBody      []byte
		defaultOpenAIModel string
	}{
		{
			name:               "simple user text",
			defaultOpenAIModel: "gpt-5",
			anthropicBody:      []byte(`{"model":"claude-sonnet-4-5","messages":[{"role":"user","content":"hello world from sub2api"}]}`),
		},
		{
			name:               "system plus tool",
			defaultOpenAIModel: "gpt-5",
			anthropicBody:      []byte(`{"model":"claude-sonnet-4-5","system":"You are helpful.","messages":[{"role":"user","content":"find weather in shanghai"}],"tools":[{"name":"lookup_weather","description":"Look up current weather","input_schema":{"type":"object","properties":{"city":{"type":"string"}},"required":["city"]}}]}`),
		},
		{
			name:               "multi turn text",
			defaultOpenAIModel: "gpt-4.1",
			anthropicBody:      []byte(`{"model":"claude-opus-4-1","messages":[{"role":"user","content":"summarize this repo"},{"role":"assistant","content":"which repo?"},{"role":"user","content":"sub2api"}]}`),
		},
	}

	account := &Account{Platform: PlatformOpenAI, Type: AccountTypeAPIKey}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			prepared, err := prepareOpenAIInputTokensCountRequest(tc.anthropicBody, account, tc.defaultOpenAIModel)
			require.NoError(t, err)

			estimated, err := estimateOpenAIInputTokens(prepared.Request)
			require.NoError(t, err)

			actual, err := callOpenAIInputTokensAPIForTest(client, apiKey, prepared.Request)
			require.NoError(t, err)

			diff := estimated - actual
			if diff < 0 {
				diff = -diff
			}
			t.Logf("model=%s estimated=%d actual=%d diff=%d", prepared.Request.Model, estimated, actual, diff)
			require.LessOrEqual(t, diff, maxLocalInt(24, actual/4))
		})
	}
}

func callOpenAIInputTokensAPIForTest(client *http.Client, apiKey string, reqBody openAIInputTokensCountRequest) (int, error) {
	body, err := marshalOpenAIUpstreamJSON(reqBody)
	if err != nil {
		return 0, err
	}
	req, err := http.NewRequest(http.MethodPost, openaiPlatformAPIInputTokensURL, bytes.NewReader(body))
	if err != nil {
		return 0, err
	}
	req.Header.Set("authorization", "Bearer "+apiKey)
	req.Header.Set("content-type", "application/json")
	req.Header.Set("accept", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return 0, err
	}
	defer func() { _ = resp.Body.Close() }()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return 0, err
	}
	if resp.StatusCode >= 400 {
		return 0, fmt.Errorf("openai input_tokens api error: status=%d body=%s", resp.StatusCode, string(respBody))
	}

	value := gjson.GetBytes(respBody, "input_tokens")
	if !value.Exists() {
		return 0, fmt.Errorf("openai input_tokens api missing input_tokens: %s", string(respBody))
	}
	return int(value.Int()), nil
}

func maxLocalInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
