package service

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/claude"
	"github.com/Wei-Shaw/sub2api/internal/pkg/tlsfingerprint"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

type anthropicHTTPUpstreamRecorder struct {
	lastReq  *http.Request
	lastBody []byte
	resp     *http.Response
	err      error
}

func newAnthropicAPIKeyAccountForTest() *Account {
	return &Account{
		ID:          201,
		Name:        "anthropic-apikey-pass-test",
		Platform:    PlatformAnthropic,
		Type:        AccountTypeAPIKey,
		Concurrency: 1,
		Credentials: map[string]any{
			"api_key":  "upstream-anthropic-key",
			"base_url": "https://api.anthropic.com",
		},
		Extra: map[string]any{
			"anthropic_passthrough": true,
		},
		Status:      StatusActive,
		Schedulable: true,
	}
}

func (u *anthropicHTTPUpstreamRecorder) Do(req *http.Request, proxyURL string, accountID int64, accountConcurrency int) (*http.Response, error) {
	u.lastReq = req
	if req != nil && req.Body != nil {
		b, _ := io.ReadAll(req.Body)
		u.lastBody = b
		_ = req.Body.Close()
		req.Body = io.NopCloser(bytes.NewReader(b))
	}
	if u.err != nil {
		return nil, u.err
	}
	return u.resp, nil
}

func (u *anthropicHTTPUpstreamRecorder) DoWithTLS(req *http.Request, proxyURL string, accountID int64, accountConcurrency int, profile *tlsfingerprint.Profile) (*http.Response, error) {
	return u.Do(req, proxyURL, accountID, accountConcurrency)
}

type streamReadCloser struct {
	payload []byte
	sent    bool
	err     error
}

func (r *streamReadCloser) Read(p []byte) (int, error) {
	if !r.sent {
		r.sent = true
		n := copy(p, r.payload)
		return n, nil
	}
	if r.err != nil {
		return 0, r.err
	}
	return 0, io.EOF
}

func (r *streamReadCloser) Close() error { return nil }

type failWriteResponseWriter struct {
	gin.ResponseWriter
}

func (w *failWriteResponseWriter) Write(data []byte) (int, error) {
	return 0, errors.New("client disconnected")
}

func (w *failWriteResponseWriter) WriteString(_ string) (int, error) {
	return 0, errors.New("client disconnected")
}

func TestGatewayService_AnthropicAPIKeyPassthrough_ForwardStreamPreservesBodyAndAuthReplacement(t *testing.T) {
	gin.SetMode(gin.TestMode)

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)
	c.Request.Header.Set("User-Agent", "claude-cli/1.0.0")
	c.Request.Header.Set("Authorization", "Bearer inbound-token")
	c.Request.Header.Set("X-Api-Key", "inbound-api-key")
	c.Request.Header.Set("X-Goog-Api-Key", "inbound-goog-key")
	c.Request.Header.Set("Cookie", "secret=1")
	c.Request.Header.Set("Anthropic-Beta", "interleaved-thinking-2025-05-14")

	body := []byte(`{"model":"claude-3-7-sonnet-20250219","stream":true,"system":[{"type":"text","text":"x-anthropic-billing-header keep"}],"messages":[{"role":"user","content":[{"type":"text","text":"hello"}]}]}`)
	parsed := &ParsedRequest{
		Body:   body,
		Model:  "claude-3-7-sonnet-20250219",
		Stream: true,
	}

	upstreamSSE := strings.Join([]string{
		`data: {"type":"message_start","message":{"usage":{"input_tokens":9,"cached_tokens":7}}}`,
		"",
		`data: {"type":"message_delta","usage":{"output_tokens":3}}`,
		"",
		"data: [DONE]",
		"",
	}, "\n")
	upstream := &anthropicHTTPUpstreamRecorder{
		resp: &http.Response{
			StatusCode: http.StatusOK,
			Header: http.Header{
				"Content-Type": []string{"text/event-stream"},
				"x-request-id": []string{"rid-anthropic-pass"},
				"Set-Cookie":   []string{"secret=upstream"},
			},
			Body: io.NopCloser(strings.NewReader(upstreamSSE)),
		},
	}

	cfg := &config.Config{
		Gateway: config.GatewayConfig{
			MaxLineSize: defaultMaxLineSize,
		},
	}
	svc := &GatewayService{
		cfg:                  cfg,
		responseHeaderFilter: compileResponseHeaderFilter(cfg),
		httpUpstream:         upstream,
		rateLimitService:     &RateLimitService{},
		deferredService:      &DeferredService{},
		billingCacheService:  nil,
	}

	account := &Account{
		ID:          101,
		Name:        "anthropic-apikey-pass",
		Platform:    PlatformAnthropic,
		Type:        AccountTypeAPIKey,
		Concurrency: 1,
		Credentials: map[string]any{
			"api_key":       "upstream-anthropic-key",
			"base_url":      "https://api.anthropic.com",
			"model_mapping": map[string]any{"claude-3-7-sonnet-20250219": "claude-3-haiku-20240307"},
		},
		Extra: map[string]any{
			"anthropic_passthrough": true,
		},
		Status:      StatusActive,
		Schedulable: true,
	}

	result, err := svc.Forward(context.Background(), c, account, parsed)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.True(t, result.Stream)

	require.Equal(t, "claude-3-haiku-20240307", gjson.GetBytes(upstream.lastBody, "model").String(), "透传模式应应用账号级模型映射")

	require.Equal(t, "upstream-anthropic-key", getHeaderRaw(upstream.lastReq.Header, "x-api-key"))
	require.Empty(t, getHeaderRaw(upstream.lastReq.Header, "authorization"))
	require.Empty(t, getHeaderRaw(upstream.lastReq.Header, "x-goog-api-key"))
	require.Empty(t, getHeaderRaw(upstream.lastReq.Header, "cookie"))
	require.Equal(t, "2023-06-01", getHeaderRaw(upstream.lastReq.Header, "anthropic-version"))
	require.Equal(t, "interleaved-thinking-2025-05-14", getHeaderRaw(upstream.lastReq.Header, "anthropic-beta"))
	require.Empty(t, getHeaderRaw(upstream.lastReq.Header, "x-stainless-lang"), "API Key 透传不应注入 OAuth 指纹头")

	require.Contains(t, rec.Body.String(), `"cached_tokens":7`)
	require.NotContains(t, rec.Body.String(), `"cache_read_input_tokens":7`, "透传输出不应被网关改写")
	require.Equal(t, 7, result.Usage.CacheReadInputTokens, "计费 usage 解析应保留 cached_tokens 兼容")
	require.Empty(t, rec.Header().Get("Set-Cookie"), "响应头应经过安全过滤")
	rawBody, ok := c.Get(OpsUpstreamRequestBodyKey)
	require.True(t, ok)
	bodyBytes, ok := rawBody.([]byte)
	require.True(t, ok, "应以 []byte 形式缓存上游请求体，避免重复 string 拷贝")
	require.Equal(t, "claude-3-haiku-20240307", gjson.GetBytes(bodyBytes, "model").String(), "缓存的上游请求体应包含映射后的模型")
}

func TestGatewayService_AnthropicAPIKeyPassthrough_ForwardCountTokensPreservesBody(t *testing.T) {
	gin.SetMode(gin.TestMode)

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages/count_tokens", nil)
	c.Request.Header.Set("Authorization", "Bearer inbound-token")
	c.Request.Header.Set("X-Api-Key", "inbound-api-key")
	c.Request.Header.Set("Cookie", "secret=1")

	body := []byte(`{"model":"claude-3-5-sonnet-latest","messages":[{"role":"user","content":[{"type":"text","text":"hello"}]}],"thinking":{"type":"enabled"}}`)
	parsed := &ParsedRequest{
		Body:  body,
		Model: "claude-3-5-sonnet-latest",
	}

	upstreamRespBody := `{"input_tokens":42}`
	upstream := &anthropicHTTPUpstreamRecorder{
		resp: &http.Response{
			StatusCode: http.StatusOK,
			Header: http.Header{
				"Content-Type": []string{"application/json"},
				"x-request-id": []string{"rid-count"},
				"Set-Cookie":   []string{"secret=upstream"},
			},
			Body: io.NopCloser(strings.NewReader(upstreamRespBody)),
		},
	}

	cfg := &config.Config{
		Gateway: config.GatewayConfig{
			MaxLineSize: defaultMaxLineSize,
		},
	}
	svc := &GatewayService{
		cfg:                  cfg,
		responseHeaderFilter: compileResponseHeaderFilter(cfg),
		httpUpstream:         upstream,
		rateLimitService:     &RateLimitService{},
	}

	account := &Account{
		ID:          102,
		Name:        "anthropic-apikey-pass-count",
		Platform:    PlatformAnthropic,
		Type:        AccountTypeAPIKey,
		Concurrency: 1,
		Credentials: map[string]any{
			"api_key":       "upstream-anthropic-key",
			"base_url":      "https://api.anthropic.com",
			"model_mapping": map[string]any{"claude-3-5-sonnet-latest": "claude-3-opus-20240229"},
		},
		Extra: map[string]any{
			"anthropic_passthrough": true,
		},
		Status:      StatusActive,
		Schedulable: true,
	}

	err := svc.ForwardCountTokens(context.Background(), c, account, parsed)
	require.NoError(t, err)

	require.Equal(t, "claude-3-opus-20240229", gjson.GetBytes(upstream.lastBody, "model").String(), "count_tokens 透传模式应应用账号级模型映射")
	require.Equal(t, "upstream-anthropic-key", getHeaderRaw(upstream.lastReq.Header, "x-api-key"))
	require.Empty(t, getHeaderRaw(upstream.lastReq.Header, "authorization"))
	require.Empty(t, getHeaderRaw(upstream.lastReq.Header, "cookie"))
	require.Equal(t, http.StatusOK, rec.Code)
	require.JSONEq(t, upstreamRespBody, rec.Body.String())
	require.Empty(t, rec.Header().Get("Set-Cookie"))
}

// TestGatewayService_AnthropicAPIKeyPassthrough_ModelMappingEdgeCases 覆盖透传模式下模型映射的各种边界情况
func TestGatewayService_AnthropicAPIKeyPassthrough_ModelMappingEdgeCases(t *testing.T) {
	gin.SetMode(gin.TestMode)

	tests := []struct {
		name          string
		model         string
		modelMapping  map[string]any // nil = 不配置映射
		expectedModel string
		endpoint      string // "messages" or "count_tokens"
	}{
		{
			name:          "Forward: 无映射配置时不改写模型",
			model:         "claude-sonnet-4-20250514",
			modelMapping:  nil,
			expectedModel: "claude-sonnet-4-20250514",
			endpoint:      "messages",
		},
		{
			name:          "Forward: 空映射配置时不改写模型",
			model:         "claude-sonnet-4-20250514",
			modelMapping:  map[string]any{},
			expectedModel: "claude-sonnet-4-20250514",
			endpoint:      "messages",
		},
		{
			name:          "Forward: 模型不在映射表中时不改写",
			model:         "claude-sonnet-4-20250514",
			modelMapping:  map[string]any{"claude-3-haiku-20240307": "claude-3-opus-20240229"},
			expectedModel: "claude-sonnet-4-20250514",
			endpoint:      "messages",
		},
		{
			name:          "Forward: 精确匹配映射应改写模型",
			model:         "claude-sonnet-4-20250514",
			modelMapping:  map[string]any{"claude-sonnet-4-20250514": "claude-sonnet-4-5-20241022"},
			expectedModel: "claude-sonnet-4-5-20241022",
			endpoint:      "messages",
		},
		{
			name:          "Forward: 通配符映射应改写模型",
			model:         "claude-sonnet-4-20250514",
			modelMapping:  map[string]any{"claude-sonnet-4-*": "claude-sonnet-4-5-20241022"},
			expectedModel: "claude-sonnet-4-5-20241022",
			endpoint:      "messages",
		},
		{
			name:          "CountTokens: 无映射配置时不改写模型",
			model:         "claude-sonnet-4-20250514",
			modelMapping:  nil,
			expectedModel: "claude-sonnet-4-20250514",
			endpoint:      "count_tokens",
		},
		{
			name:          "CountTokens: 模型不在映射表中时不改写",
			model:         "claude-sonnet-4-20250514",
			modelMapping:  map[string]any{"claude-3-haiku-20240307": "claude-3-opus-20240229"},
			expectedModel: "claude-sonnet-4-20250514",
			endpoint:      "count_tokens",
		},
		{
			name:          "CountTokens: 精确匹配映射应改写模型",
			model:         "claude-sonnet-4-20250514",
			modelMapping:  map[string]any{"claude-sonnet-4-20250514": "claude-sonnet-4-5-20241022"},
			expectedModel: "claude-sonnet-4-5-20241022",
			endpoint:      "count_tokens",
		},
		{
			name:          "CountTokens: 通配符映射应改写模型",
			model:         "claude-sonnet-4-20250514",
			modelMapping:  map[string]any{"claude-sonnet-4-*": "claude-sonnet-4-5-20241022"},
			expectedModel: "claude-sonnet-4-5-20241022",
			endpoint:      "count_tokens",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(rec)

			body := []byte(`{"model":"` + tt.model + `","messages":[{"role":"user","content":[{"type":"text","text":"hello"}]}]}`)
			parsed := &ParsedRequest{
				Body:  body,
				Model: tt.model,
			}

			credentials := map[string]any{
				"api_key":  "upstream-key",
				"base_url": "https://api.anthropic.com",
			}
			if tt.modelMapping != nil {
				credentials["model_mapping"] = tt.modelMapping
			}

			account := &Account{
				ID:          300,
				Name:        "edge-case-test",
				Platform:    PlatformAnthropic,
				Type:        AccountTypeAPIKey,
				Concurrency: 1,
				Credentials: credentials,
				Extra:       map[string]any{"anthropic_passthrough": true},
				Status:      StatusActive,
				Schedulable: true,
			}

			if tt.endpoint == "messages" {
				c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)
				parsed.Stream = false

				upstreamJSON := `{"id":"msg_1","type":"message","usage":{"input_tokens":5,"output_tokens":3}}`
				upstream := &anthropicHTTPUpstreamRecorder{
					resp: &http.Response{
						StatusCode: http.StatusOK,
						Header:     http.Header{"Content-Type": []string{"application/json"}},
						Body:       io.NopCloser(strings.NewReader(upstreamJSON)),
					},
				}
				svc := &GatewayService{
					cfg:              &config.Config{},
					httpUpstream:     upstream,
					rateLimitService: &RateLimitService{},
				}

				result, err := svc.Forward(context.Background(), c, account, parsed)
				require.NoError(t, err)
				require.NotNil(t, result)
				require.Equal(t, tt.expectedModel, gjson.GetBytes(upstream.lastBody, "model").String(),
					"Forward 上游请求体中的模型应为: %s", tt.expectedModel)
			} else {
				c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages/count_tokens", nil)

				upstreamRespBody := `{"input_tokens":42}`
				upstream := &anthropicHTTPUpstreamRecorder{
					resp: &http.Response{
						StatusCode: http.StatusOK,
						Header:     http.Header{"Content-Type": []string{"application/json"}},
						Body:       io.NopCloser(strings.NewReader(upstreamRespBody)),
					},
				}
				svc := &GatewayService{
					cfg:              &config.Config{Gateway: config.GatewayConfig{MaxLineSize: defaultMaxLineSize}},
					httpUpstream:     upstream,
					rateLimitService: &RateLimitService{},
				}

				err := svc.ForwardCountTokens(context.Background(), c, account, parsed)
				require.NoError(t, err)
				require.Equal(t, tt.expectedModel, gjson.GetBytes(upstream.lastBody, "model").String(),
					"CountTokens 上游请求体中的模型应为: %s", tt.expectedModel)
			}
		})
	}
}

// TestGatewayService_AnthropicAPIKeyPassthrough_ModelMappingPreservesOtherFields
// 确保模型映射只替换 model 字段，不影响请求体中的其他字段
func TestGatewayService_AnthropicAPIKeyPassthrough_ModelMappingPreservesOtherFields(t *testing.T) {
	gin.SetMode(gin.TestMode)

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages/count_tokens", nil)

	// 包含复杂字段的请求体：system、thinking、messages
	body := []byte(`{"model":"claude-sonnet-4-20250514","system":[{"type":"text","text":"You are a helpful assistant."}],"messages":[{"role":"user","content":[{"type":"text","text":"hello world"}]}],"thinking":{"type":"enabled","budget_tokens":5000},"max_tokens":1024}`)
	parsed := &ParsedRequest{
		Body:  body,
		Model: "claude-sonnet-4-20250514",
	}

	upstreamRespBody := `{"input_tokens":42}`
	upstream := &anthropicHTTPUpstreamRecorder{
		resp: &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body:       io.NopCloser(strings.NewReader(upstreamRespBody)),
		},
	}

	svc := &GatewayService{
		cfg:              &config.Config{Gateway: config.GatewayConfig{MaxLineSize: defaultMaxLineSize}},
		httpUpstream:     upstream,
		rateLimitService: &RateLimitService{},
	}

	account := &Account{
		ID:          301,
		Name:        "preserve-fields-test",
		Platform:    PlatformAnthropic,
		Type:        AccountTypeAPIKey,
		Concurrency: 1,
		Credentials: map[string]any{
			"api_key":       "upstream-key",
			"base_url":      "https://api.anthropic.com",
			"model_mapping": map[string]any{"claude-sonnet-4-20250514": "claude-sonnet-4-5-20241022"},
		},
		Extra:       map[string]any{"anthropic_passthrough": true},
		Status:      StatusActive,
		Schedulable: true,
	}

	err := svc.ForwardCountTokens(context.Background(), c, account, parsed)
	require.NoError(t, err)

	sentBody := upstream.lastBody
	require.Equal(t, "claude-sonnet-4-5-20241022", gjson.GetBytes(sentBody, "model").String(), "model 应被映射")
	require.Equal(t, "You are a helpful assistant.", gjson.GetBytes(sentBody, "system.0.text").String(), "system 字段不应被修改")
	require.Equal(t, "hello world", gjson.GetBytes(sentBody, "messages.0.content.0.text").String(), "messages 字段不应被修改")
	require.Equal(t, "enabled", gjson.GetBytes(sentBody, "thinking.type").String(), "thinking 字段不应被修改")
	require.Equal(t, int64(5000), gjson.GetBytes(sentBody, "thinking.budget_tokens").Int(), "thinking.budget_tokens 不应被修改")
	require.Equal(t, int64(1024), gjson.GetBytes(sentBody, "max_tokens").Int(), "max_tokens 不应被修改")
}

// TestGatewayService_AnthropicAPIKeyPassthrough_EmptyModelSkipsMapping
// 确保空模型名不会触发映射逻辑
func TestGatewayService_AnthropicAPIKeyPassthrough_EmptyModelSkipsMapping(t *testing.T) {
	gin.SetMode(gin.TestMode)

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages/count_tokens", nil)

	body := []byte(`{"messages":[{"role":"user","content":"hello"}]}`)
	parsed := &ParsedRequest{
		Body:  body,
		Model: "", // 空模型
	}

	upstreamRespBody := `{"input_tokens":10}`
	upstream := &anthropicHTTPUpstreamRecorder{
		resp: &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body:       io.NopCloser(strings.NewReader(upstreamRespBody)),
		},
	}

	svc := &GatewayService{
		cfg:              &config.Config{Gateway: config.GatewayConfig{MaxLineSize: defaultMaxLineSize}},
		httpUpstream:     upstream,
		rateLimitService: &RateLimitService{},
	}

	account := &Account{
		ID:          302,
		Name:        "empty-model-test",
		Platform:    PlatformAnthropic,
		Type:        AccountTypeAPIKey,
		Concurrency: 1,
		Credentials: map[string]any{
			"api_key":       "upstream-key",
			"base_url":      "https://api.anthropic.com",
			"model_mapping": map[string]any{"*": "claude-3-opus-20240229"},
		},
		Extra:       map[string]any{"anthropic_passthrough": true},
		Status:      StatusActive,
		Schedulable: true,
	}

	err := svc.ForwardCountTokens(context.Background(), c, account, parsed)
	require.NoError(t, err)
	// 空模型名时，body 应原样透传，不应触发映射
	require.Equal(t, body, upstream.lastBody, "空模型名时请求体不应被修改")
}

func TestGatewayService_AnthropicAPIKeyPassthrough_CountTokens404PassthroughNotError(t *testing.T) {
	gin.SetMode(gin.TestMode)

	tests := []struct {
		name            string
		statusCode      int
		respBody        string
		wantPassthrough bool
	}{
		{
			name:            "404 endpoint not found passes through as 404",
			statusCode:      http.StatusNotFound,
			respBody:        `{"error":{"message":"Not found: /v1/messages/count_tokens","type":"not_found_error"}}`,
			wantPassthrough: true,
		},
		{
			name:            "404 generic not found does not passthrough",
			statusCode:      http.StatusNotFound,
			respBody:        `{"error":{"message":"resource not found","type":"not_found_error"}}`,
			wantPassthrough: false,
		},
		{
			name:            "400 Invalid URL does not passthrough",
			statusCode:      http.StatusBadRequest,
			respBody:        `{"error":{"message":"Invalid URL (POST /v1/messages/count_tokens)","type":"invalid_request_error"}}`,
			wantPassthrough: false,
		},
		{
			name:            "400 model error does not passthrough",
			statusCode:      http.StatusBadRequest,
			respBody:        `{"error":{"message":"model not found: claude-unknown","type":"invalid_request_error"}}`,
			wantPassthrough: false,
		},
		{
			name:            "500 internal error does not passthrough",
			statusCode:      http.StatusInternalServerError,
			respBody:        `{"error":{"message":"internal error","type":"api_error"}}`,
			wantPassthrough: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(rec)
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages/count_tokens", nil)

			body := []byte(`{"model":"claude-sonnet-4-5-20250929","messages":[{"role":"user","content":"hi"}]}`)
			parsed := &ParsedRequest{Body: body, Model: "claude-sonnet-4-5-20250929"}

			upstream := &anthropicHTTPUpstreamRecorder{
				resp: &http.Response{
					StatusCode: tt.statusCode,
					Header:     http.Header{"Content-Type": []string{"application/json"}},
					Body:       io.NopCloser(strings.NewReader(tt.respBody)),
				},
			}

			svc := &GatewayService{
				cfg: &config.Config{
					Gateway: config.GatewayConfig{MaxLineSize: defaultMaxLineSize},
				},
				httpUpstream:     upstream,
				rateLimitService: nil,
			}

			account := &Account{
				ID:          200,
				Name:        "proxy-acc",
				Platform:    PlatformAnthropic,
				Type:        AccountTypeAPIKey,
				Concurrency: 1,
				Credentials: map[string]any{
					"api_key":  "sk-proxy",
					"base_url": "https://proxy.example.com",
				},
				Extra:       map[string]any{"anthropic_passthrough": true},
				Status:      StatusActive,
				Schedulable: true,
			}

			err := svc.ForwardCountTokens(context.Background(), c, account, parsed)

			if tt.wantPassthrough {
				// 返回 nil（不记录为错误），HTTP 状态码 404 + Anthropic 错误体
				require.NoError(t, err)
				require.Equal(t, http.StatusNotFound, rec.Code)
				var errResp map[string]any
				require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &errResp))
				require.Equal(t, "error", errResp["type"])
				errObj, ok := errResp["error"].(map[string]any)
				require.True(t, ok)
				require.Equal(t, "not_found_error", errObj["type"])
			} else {
				require.Error(t, err)
				require.Equal(t, tt.statusCode, rec.Code)
			}
		})
	}
}

func TestGatewayService_AnthropicAPIKeyPassthrough_BuildRequestRejectsInvalidBaseURL(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)

	svc := &GatewayService{
		cfg: &config.Config{
			Security: config.SecurityConfig{
				URLAllowlist: config.URLAllowlistConfig{
					Enabled: false,
				},
			},
		},
	}
	account := &Account{
		Platform: PlatformAnthropic,
		Type:     AccountTypeAPIKey,
		Credentials: map[string]any{
			"api_key":  "k",
			"base_url": "://invalid-url",
		},
	}

	_, err := svc.buildUpstreamRequestAnthropicAPIKeyPassthrough(context.Background(), c, account, []byte(`{}`), "k")
	require.Error(t, err)
}

func TestGatewayService_AnthropicOAuth_NotAffectedByAPIKeyPassthroughToggle(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)

	svc := &GatewayService{
		cfg: &config.Config{
			Gateway: config.GatewayConfig{MaxLineSize: defaultMaxLineSize},
		},
	}
	account := &Account{
		Platform: PlatformAnthropic,
		Type:     AccountTypeOAuth,
		Extra: map[string]any{
			"anthropic_passthrough": true,
		},
	}

	require.False(t, account.IsAnthropicAPIKeyPassthroughEnabled())

	req, err := svc.buildUpstreamRequest(context.Background(), c, account, []byte(`{"model":"claude-3-7-sonnet-20250219"}`), "oauth-token", "oauth", "claude-3-7-sonnet-20250219", true, false)
	require.NoError(t, err)
	require.Equal(t, "Bearer oauth-token", getHeaderRaw(req.Header, "authorization"))
	require.Contains(t, getHeaderRaw(req.Header, "anthropic-beta"), claude.BetaOAuth, "OAuth 链路仍应按原逻辑补齐 oauth beta")
}

func TestGatewayService_AnthropicOAuth_ForwardPreservesBillingHeaderSystemBlock(t *testing.T) {
	gin.SetMode(gin.TestMode)

	tests := []struct {
		name string
		body string
	}{
		{
			name: "system array",
			body: `{"model":"claude-3-5-sonnet-latest","system":[{"type":"text","text":"x-anthropic-billing-header keep"}],"messages":[{"role":"user","content":[{"type":"text","text":"hello"}]}]}`,
		},
		{
			name: "system string",
			body: `{"model":"claude-3-5-sonnet-latest","system":"x-anthropic-billing-header keep","messages":[{"role":"user","content":[{"type":"text","text":"hello"}]}]}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(rec)
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)

			parsed, err := ParseGatewayRequest([]byte(tt.body), PlatformAnthropic)
			require.NoError(t, err)

			upstream := &anthropicHTTPUpstreamRecorder{
				resp: &http.Response{
					StatusCode: http.StatusOK,
					Header: http.Header{
						"Content-Type": []string{"application/json"},
						"x-request-id": []string{"rid-oauth-preserve"},
					},
					Body: io.NopCloser(strings.NewReader(`{"id":"msg_1","type":"message","role":"assistant","model":"claude-3-5-sonnet-20241022","content":[{"type":"text","text":"ok"}],"usage":{"input_tokens":12,"output_tokens":7}}`)),
				},
			}

			cfg := &config.Config{
				Gateway: config.GatewayConfig{
					MaxLineSize: defaultMaxLineSize,
				},
			}
			svc := &GatewayService{
				cfg:                  cfg,
				responseHeaderFilter: compileResponseHeaderFilter(cfg),
				httpUpstream:         upstream,
				rateLimitService:     &RateLimitService{},
				deferredService:      &DeferredService{},
			}

			account := &Account{
				ID:          301,
				Name:        "anthropic-oauth-preserve",
				Platform:    PlatformAnthropic,
				Type:        AccountTypeOAuth,
				Concurrency: 1,
				Credentials: map[string]any{
					"access_token": "oauth-token",
				},
				Status:      StatusActive,
				Schedulable: true,
			}

			result, err := svc.Forward(context.Background(), c, account, parsed)
			require.NoError(t, err)
			require.NotNil(t, result)
			require.NotNil(t, upstream.lastReq)
			require.Equal(t, "Bearer oauth-token", getHeaderRaw(upstream.lastReq.Header, "authorization"))
			require.Contains(t, getHeaderRaw(upstream.lastReq.Header, "anthropic-beta"), claude.BetaOAuth)

			system := gjson.GetBytes(upstream.lastBody, "system")
			require.True(t, system.Exists())
			require.True(t, system.IsArray(), "system should be an array")
			require.Equal(t, claudeCodeSystemPrompt, system.Array()[0].Get("text").String())
			require.Equal(t, "ephemeral", system.Array()[0].Get("cache_control.type").String())

			// 原始 system prompt 应迁移至 messages 中
			messages := gjson.GetBytes(upstream.lastBody, "messages")
			require.True(t, messages.IsArray())
			firstMsg := messages.Array()[0]
			require.Equal(t, "user", firstMsg.Get("role").String())
			require.Contains(t, firstMsg.Get("content.0.text").String(), "x-anthropic-billing-header keep")
		})
	}
}

func TestGatewayService_AnthropicAPIKeyPassthrough_StreamingStillCollectsUsageAfterClientDisconnect(t *testing.T) {
	gin.SetMode(gin.TestMode)

	// Use a canceled context recorder to simulate client disconnect behavior.
	req := httptest.NewRequest(http.MethodPost, "/v1/messages", nil)
	ctx, cancel := context.WithCancel(req.Context())
	cancel()
	req = req.WithContext(ctx)

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = req

	svc := &GatewayService{
		cfg: &config.Config{
			Gateway: config.GatewayConfig{
				MaxLineSize: defaultMaxLineSize,
			},
		},
		rateLimitService: &RateLimitService{},
	}

	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
		Body: io.NopCloser(strings.NewReader(strings.Join([]string{
			`data: {"type":"message_start","message":{"usage":{"input_tokens":11}}}`,
			"",
			`data: {"type":"message_delta","usage":{"output_tokens":5}}`,
			"",
			"data: [DONE]",
			"",
		}, "\n"))),
	}

	result, err := svc.handleStreamingResponseAnthropicAPIKeyPassthrough(context.Background(), resp, c, &Account{ID: 1}, time.Now(), "claude-3-7-sonnet-20250219")
	require.NoError(t, err)
	require.NotNil(t, result)
	require.NotNil(t, result.usage)
	require.Equal(t, 11, result.usage.InputTokens)
	require.Equal(t, 5, result.usage.OutputTokens)
}

func TestGatewayService_AnthropicAPIKeyPassthrough_MissingTerminalEventReturnsError(t *testing.T) {
	gin.SetMode(gin.TestMode)

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)

	svc := &GatewayService{
		cfg: &config.Config{
			Gateway: config.GatewayConfig{
				MaxLineSize: defaultMaxLineSize,
			},
		},
		rateLimitService: &RateLimitService{},
	}

	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
		Body: io.NopCloser(strings.NewReader(strings.Join([]string{
			`data: {"type":"message_start","message":{"usage":{"input_tokens":11}}}`,
			"",
			`data: {"type":"message_delta","usage":{"output_tokens":5}}`,
			"",
		}, "\n"))),
	}

	result, err := svc.handleStreamingResponseAnthropicAPIKeyPassthrough(context.Background(), resp, c, &Account{ID: 1}, time.Now(), "claude-3-7-sonnet-20250219")
	require.Error(t, err)
	require.Contains(t, err.Error(), "missing terminal event")
	require.NotNil(t, result)
}

func TestGatewayService_AnthropicAPIKeyPassthrough_ForwardDirect_NonStreamingSuccess(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)

	body := []byte(`{"model":"claude-3-5-sonnet-latest","messages":[{"role":"user","content":[{"type":"text","text":"hello"}]}]}`)
	upstreamJSON := `{"id":"msg_1","type":"message","usage":{"input_tokens":12,"output_tokens":7,"cache_creation":{"ephemeral_5m_input_tokens":2,"ephemeral_1h_input_tokens":3},"cached_tokens":4}}`
	upstream := &anthropicHTTPUpstreamRecorder{
		resp: &http.Response{
			StatusCode: http.StatusOK,
			Header: http.Header{
				"Content-Type": []string{"application/json"},
				"x-request-id": []string{"rid-nonstream"},
			},
			Body: io.NopCloser(strings.NewReader(upstreamJSON)),
		},
	}
	svc := &GatewayService{
		cfg:              &config.Config{},
		httpUpstream:     upstream,
		rateLimitService: &RateLimitService{},
	}

	result, err := svc.forwardAnthropicAPIKeyPassthrough(context.Background(), c, newAnthropicAPIKeyAccountForTest(), body, "claude-3-5-sonnet-latest", "claude-3-5-sonnet-latest", false, time.Now())
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Equal(t, 12, result.Usage.InputTokens)
	require.Equal(t, 7, result.Usage.OutputTokens)
	require.Equal(t, 5, result.Usage.CacheCreationInputTokens)
	require.Equal(t, 4, result.Usage.CacheReadInputTokens)
	require.Equal(t, upstreamJSON, rec.Body.String())
}

func TestGatewayService_AnthropicAPIKeyPassthrough_ForwardDirect_InvalidTokenType(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)

	account := &Account{
		ID:       202,
		Name:     "anthropic-oauth",
		Platform: PlatformAnthropic,
		Type:     AccountTypeOAuth,
		Credentials: map[string]any{
			"access_token": "oauth-token",
		},
	}
	svc := &GatewayService{}

	result, err := svc.forwardAnthropicAPIKeyPassthrough(context.Background(), c, account, []byte(`{}`), "claude-3-5-sonnet-latest", "claude-3-5-sonnet-latest", false, time.Now())
	require.Nil(t, result)
	require.Error(t, err)
	require.Contains(t, err.Error(), "requires apikey token")
}

func TestGatewayService_AnthropicAPIKeyPassthrough_ForwardDirect_UpstreamRequestError(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)

	upstream := &anthropicHTTPUpstreamRecorder{
		err: errors.New("dial tcp timeout"),
	}
	svc := &GatewayService{
		cfg: &config.Config{
			Security: config.SecurityConfig{
				URLAllowlist: config.URLAllowlistConfig{Enabled: false},
			},
		},
		httpUpstream: upstream,
	}
	account := newAnthropicAPIKeyAccountForTest()

	result, err := svc.forwardAnthropicAPIKeyPassthrough(context.Background(), c, account, []byte(`{"model":"x"}`), "x", "x", false, time.Now())
	require.Nil(t, result)
	require.Error(t, err)
	require.Contains(t, err.Error(), "upstream request failed")
	require.Equal(t, http.StatusBadGateway, rec.Code)
	rawBody, ok := c.Get(OpsUpstreamRequestBodyKey)
	require.True(t, ok)
	_, ok = rawBody.([]byte)
	require.True(t, ok)
}

func TestGatewayService_AnthropicAPIKeyPassthrough_ForwardDirect_EmptyResponseBody(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)

	upstream := &anthropicHTTPUpstreamRecorder{
		resp: &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"x-request-id": []string{"rid-empty-body"}},
			Body:       nil,
		},
	}
	svc := &GatewayService{
		cfg: &config.Config{
			Security: config.SecurityConfig{
				URLAllowlist: config.URLAllowlistConfig{Enabled: false},
			},
		},
		httpUpstream: upstream,
	}

	result, err := svc.forwardAnthropicAPIKeyPassthrough(context.Background(), c, newAnthropicAPIKeyAccountForTest(), []byte(`{"model":"x"}`), "x", "x", false, time.Now())
	require.Nil(t, result)
	require.Error(t, err)
	require.Contains(t, err.Error(), "empty response")
}

func TestExtractAnthropicSSEDataLine(t *testing.T) {
	t.Run("valid data line with spaces", func(t *testing.T) {
		data, ok := extractAnthropicSSEDataLine("data:   {\"type\":\"message_start\"}")
		require.True(t, ok)
		require.Equal(t, `{"type":"message_start"}`, data)
	})

	t.Run("non data line", func(t *testing.T) {
		data, ok := extractAnthropicSSEDataLine("event: message_start")
		require.False(t, ok)
		require.Empty(t, data)
	})
}

func TestGatewayService_ParseSSEUsagePassthrough_MessageStartFallbacks(t *testing.T) {
	svc := &GatewayService{}
	usage := &ClaudeUsage{}
	data := `{"type":"message_start","message":{"usage":{"input_tokens":12,"cache_creation_input_tokens":0,"cache_read_input_tokens":0,"cached_tokens":9,"cache_creation":{"ephemeral_5m_input_tokens":3,"ephemeral_1h_input_tokens":4}}}}`

	svc.parseSSEUsagePassthrough(data, usage)

	require.Equal(t, 12, usage.InputTokens)
	require.Equal(t, 9, usage.CacheReadInputTokens, "应兼容 cached_tokens 字段")
	require.Equal(t, 7, usage.CacheCreationInputTokens, "聚合字段为空时应从 5m/1h 明细回填")
	require.Equal(t, 3, usage.CacheCreation5mTokens)
	require.Equal(t, 4, usage.CacheCreation1hTokens)
}

func TestGatewayService_ParseSSEUsagePassthrough_MessageDeltaSelectiveOverwrite(t *testing.T) {
	svc := &GatewayService{}
	usage := &ClaudeUsage{
		InputTokens:           10,
		CacheCreation5mTokens: 2,
		CacheCreation1hTokens: 6,
	}
	data := `{"type":"message_delta","usage":{"input_tokens":0,"output_tokens":5,"cache_creation_input_tokens":8,"cache_read_input_tokens":0,"cached_tokens":11,"cache_creation":{"ephemeral_5m_input_tokens":1,"ephemeral_1h_input_tokens":0}}}`

	svc.parseSSEUsagePassthrough(data, usage)

	require.Equal(t, 10, usage.InputTokens, "message_delta 中 0 值不应覆盖已有 input_tokens")
	require.Equal(t, 5, usage.OutputTokens)
	require.Equal(t, 8, usage.CacheCreationInputTokens)
	require.Equal(t, 11, usage.CacheReadInputTokens, "cache_read_input_tokens 为空时应回退到 cached_tokens")
	require.Equal(t, 1, usage.CacheCreation5mTokens)
	require.Equal(t, 6, usage.CacheCreation1hTokens, "message_delta 中 0 值不应覆盖已有 1h 明细")
}

func TestGatewayService_ParseSSEUsagePassthrough_NoopCases(t *testing.T) {
	svc := &GatewayService{}

	usage := &ClaudeUsage{InputTokens: 3}
	svc.parseSSEUsagePassthrough("", usage)
	require.Equal(t, 3, usage.InputTokens)

	svc.parseSSEUsagePassthrough("[DONE]", usage)
	require.Equal(t, 3, usage.InputTokens)

	svc.parseSSEUsagePassthrough("not-json", usage)
	require.Equal(t, 3, usage.InputTokens)

	// nil usage 不应 panic
	svc.parseSSEUsagePassthrough(`{"type":"message_start"}`, nil)
}

func TestGatewayService_ParseSSEUsagePassthrough_FallbackFromUsageNode(t *testing.T) {
	svc := &GatewayService{}
	usage := &ClaudeUsage{}
	data := `{"type":"content_block_delta","usage":{"cached_tokens":6,"cache_creation":{"ephemeral_5m_input_tokens":2,"ephemeral_1h_input_tokens":1}}}`

	svc.parseSSEUsagePassthrough(data, usage)

	require.Equal(t, 6, usage.CacheReadInputTokens)
	require.Equal(t, 3, usage.CacheCreationInputTokens)
}

func TestParseClaudeUsageFromResponseBody(t *testing.T) {
	t.Run("empty or missing usage", func(t *testing.T) {
		got := parseClaudeUsageFromResponseBody(nil)
		require.NotNil(t, got)
		require.Equal(t, 0, got.InputTokens)

		got = parseClaudeUsageFromResponseBody([]byte(`{"id":"x"}`))
		require.NotNil(t, got)
		require.Equal(t, 0, got.OutputTokens)
	})

	t.Run("parse all usage fields and fallback", func(t *testing.T) {
		body := []byte(`{"usage":{"input_tokens":21,"output_tokens":34,"cache_creation_input_tokens":0,"cache_read_input_tokens":0,"cached_tokens":13,"cache_creation":{"ephemeral_5m_input_tokens":5,"ephemeral_1h_input_tokens":8}}}`)
		got := parseClaudeUsageFromResponseBody(body)
		require.Equal(t, 21, got.InputTokens)
		require.Equal(t, 34, got.OutputTokens)
		require.Equal(t, 13, got.CacheReadInputTokens, "cache_read_input_tokens 为空时应回退 cached_tokens")
		require.Equal(t, 13, got.CacheCreationInputTokens, "聚合字段为空时应由 5m/1h 回填")
		require.Equal(t, 5, got.CacheCreation5mTokens)
		require.Equal(t, 8, got.CacheCreation1hTokens)
	})

	t.Run("keep explicit aggregate values", func(t *testing.T) {
		body := []byte(`{"usage":{"input_tokens":1,"output_tokens":2,"cache_creation_input_tokens":9,"cache_read_input_tokens":7,"cached_tokens":99,"cache_creation":{"ephemeral_5m_input_tokens":4,"ephemeral_1h_input_tokens":5}}}`)
		got := parseClaudeUsageFromResponseBody(body)
		require.Equal(t, 9, got.CacheCreationInputTokens, "已显式提供聚合字段时不应被明细覆盖")
		require.Equal(t, 7, got.CacheReadInputTokens, "已显式提供 cache_read_input_tokens 时不应回退 cached_tokens")
	})
}

func TestGatewayService_AnthropicAPIKeyPassthrough_StreamingErrTooLong(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)

	svc := &GatewayService{
		cfg: &config.Config{
			Gateway: config.GatewayConfig{
				MaxLineSize: 32,
			},
		},
	}

	// Scanner 初始缓冲为 64KB，构造更长单行触发 bufio.ErrTooLong。
	longLine := "data: " + strings.Repeat("x", 80*1024)
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
		Body:       io.NopCloser(strings.NewReader(longLine)),
	}

	result, err := svc.handleStreamingResponseAnthropicAPIKeyPassthrough(context.Background(), resp, c, &Account{ID: 2}, time.Now(), "claude-3-7-sonnet-20250219")
	require.Error(t, err)
	require.ErrorIs(t, err, bufio.ErrTooLong)
	require.NotNil(t, result)
}

func TestGatewayService_AnthropicAPIKeyPassthrough_StreamingDataIntervalTimeout(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)

	svc := &GatewayService{
		cfg: &config.Config{
			Gateway: config.GatewayConfig{
				StreamDataIntervalTimeout: 1,
				MaxLineSize:               defaultMaxLineSize,
			},
		},
		rateLimitService: &RateLimitService{},
	}

	pr, pw := io.Pipe()
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
		Body:       pr,
	}

	result, err := svc.handleStreamingResponseAnthropicAPIKeyPassthrough(context.Background(), resp, c, &Account{ID: 5}, time.Now(), "claude-3-7-sonnet-20250219")
	_ = pw.Close()
	_ = pr.Close()

	require.Error(t, err)
	require.Contains(t, err.Error(), "stream data interval timeout")
	require.NotNil(t, result)
	require.False(t, result.clientDisconnect)
}

func TestGatewayService_AnthropicAPIKeyPassthrough_StreamingReadError(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)

	svc := &GatewayService{
		cfg: &config.Config{
			Gateway: config.GatewayConfig{
				MaxLineSize: defaultMaxLineSize,
			},
		},
	}

	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
		Body: &streamReadCloser{
			err: io.ErrUnexpectedEOF,
		},
	}

	result, err := svc.handleStreamingResponseAnthropicAPIKeyPassthrough(context.Background(), resp, c, &Account{ID: 6}, time.Now(), "claude-3-7-sonnet-20250219")
	require.Error(t, err)
	require.Contains(t, err.Error(), "stream read error")
	require.NotNil(t, result)
	require.False(t, result.clientDisconnect)
}

func TestGatewayService_AnthropicAPIKeyPassthrough_StreamingTimeoutAfterClientDisconnect(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)
	c.Writer = &failWriteResponseWriter{ResponseWriter: c.Writer}

	svc := &GatewayService{
		cfg: &config.Config{
			Gateway: config.GatewayConfig{
				StreamDataIntervalTimeout: 1,
				MaxLineSize:               defaultMaxLineSize,
			},
		},
		rateLimitService: &RateLimitService{},
	}

	pr, pw := io.Pipe()
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
		Body:       pr,
	}

	done := make(chan struct{})
	go func() {
		defer close(done)
		_, _ = pw.Write([]byte(`data: {"type":"message_start","message":{"usage":{"input_tokens":9}}}` + "\n"))
		// 保持上游连接静默，触发数据间隔超时分支。
		time.Sleep(1500 * time.Millisecond)
		_ = pw.Close()
	}()

	result, err := svc.handleStreamingResponseAnthropicAPIKeyPassthrough(context.Background(), resp, c, &Account{ID: 7}, time.Now(), "claude-3-7-sonnet-20250219")
	_ = pr.Close()
	<-done

	require.Error(t, err)
	require.Contains(t, err.Error(), "stream usage incomplete after timeout")
	require.NotNil(t, result)
	require.True(t, result.clientDisconnect)
	require.Equal(t, 9, result.usage.InputTokens)
}

func TestGatewayService_AnthropicAPIKeyPassthrough_StreamingContextCanceled(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)

	svc := &GatewayService{
		cfg: &config.Config{
			Gateway: config.GatewayConfig{
				MaxLineSize: defaultMaxLineSize,
			},
		},
	}

	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
		Body: &streamReadCloser{
			err: context.Canceled,
		},
	}

	result, err := svc.handleStreamingResponseAnthropicAPIKeyPassthrough(context.Background(), resp, c, &Account{ID: 3}, time.Now(), "claude-3-7-sonnet-20250219")
	require.Error(t, err)
	require.Contains(t, err.Error(), "stream usage incomplete")
	require.NotNil(t, result)
	require.True(t, result.clientDisconnect)
}

func TestGatewayService_AnthropicAPIKeyPassthrough_StreamingUpstreamReadErrorAfterClientDisconnect(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)
	c.Writer = &failWriteResponseWriter{ResponseWriter: c.Writer}

	svc := &GatewayService{
		cfg: &config.Config{
			Gateway: config.GatewayConfig{
				MaxLineSize: defaultMaxLineSize,
			},
		},
	}

	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
		Body: &streamReadCloser{
			payload: []byte(`data: {"type":"message_start","message":{"usage":{"input_tokens":8}}}` + "\n\n"),
			err:     io.ErrUnexpectedEOF,
		},
	}

	result, err := svc.handleStreamingResponseAnthropicAPIKeyPassthrough(context.Background(), resp, c, &Account{ID: 4}, time.Now(), "claude-3-7-sonnet-20250219")
	require.Error(t, err)
	require.Contains(t, err.Error(), "stream usage incomplete after disconnect")
	require.NotNil(t, result)
	require.True(t, result.clientDisconnect)
	require.Equal(t, 8, result.usage.InputTokens)
}
