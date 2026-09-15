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

func TestBuildOpenAIRerankURL(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		base string
		want string
	}{
		{"bare domain", "https://api.openai.com", "https://api.openai.com/v1/rerank"},
		{"bare /v1", "https://api.openai.com/v1", "https://api.openai.com/v1/rerank"},
		{"third-party /v1 base", "https://api.siliconflow.cn/v1", "https://api.siliconflow.cn/v1/rerank"},
		{"third-party versioned path", "https://open.bigmodel.cn/api/paas/v4", "https://open.bigmodel.cn/api/paas/v4/rerank"},
		{"already rerank", "https://api.siliconflow.cn/v1/rerank", "https://api.siliconflow.cn/v1/rerank"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tt.want, buildOpenAIRerankURL(tt.base))
		})
	}
}

func TestExtractOpenAIRerankUsage(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		body       string
		wantInput  int
		wantOutput int
	}{
		{
			name:       "openai style top-level usage",
			body:       `{"id":"rerank-abc","usage":{"prompt_tokens":17,"total_tokens":17}}`,
			wantInput:  17,
			wantOutput: 0,
		},
		{
			// 生产实测（v0.9.331，硅基流动）：Jina/Cohere 风格响应没有顶层
			// usage，用量只在 meta.tokens 中；只读顶层会记账 input_tokens=0。
			name:       "jina cohere style meta tokens",
			body:       `{"id":"rerank-abc","results":[],"meta":{"tokens":{"input_tokens":255,"output_tokens":0,"total_tokens":255},"billed_units":{"input_tokens":255,"output_tokens":0}}}`,
			wantInput:  255,
			wantOutput: 0,
		},
		{
			name:       "jina cohere style meta tokens with output",
			body:       `{"meta":{"tokens":{"input_tokens":255,"output_tokens":7}}}`,
			wantInput:  255,
			wantOutput: 7,
		},
		{
			name:       "top-level usage wins over meta tokens",
			body:       `{"usage":{"prompt_tokens":17},"meta":{"tokens":{"input_tokens":255}}}`,
			wantInput:  17,
			wantOutput: 0,
		},
		{
			name:       "no usage anywhere",
			body:       `{"id":"rerank-abc","results":[]}`,
			wantInput:  0,
			wantOutput: 0,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			usage := extractOpenAIRerankUsage([]byte(tt.body))
			require.Equal(t, tt.wantInput, usage.InputTokens)
			require.Equal(t, tt.wantOutput, usage.OutputTokens)
		})
	}
}

func TestForwardRerank_APIKeyPassthroughRecordsUsage(t *testing.T) {
	gin.SetMode(gin.TestMode)

	reqBody := []byte(`{
		"model":"bge-reranker-v2-m3",
		"query":"what is sub2api",
		"documents":["doc-a","doc-b","doc-c"],
		"top_n":2,
		"return_documents":true
	}`)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/rerank", bytes.NewReader(reqBody))
	c.Request.Header.Set("Content-Type", "application/json")

	upstreamBody := `{"id":"rerank-abc","model":"BAAI/bge-reranker-v2-m3","results":[{"index":0,"relevance_score":0.97,"document":{"text":"doc-a"}},{"index":2,"relevance_score":0.42,"document":{"text":"doc-c"}}],"usage":{"prompt_tokens":17,"total_tokens":17}}`
	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusOK,
		Header: http.Header{
			"Content-Type": []string{"application/json"},
			"X-Request-Id": []string{"rerank-rid"},
		},
		Body: io.NopCloser(strings.NewReader(upstreamBody)),
	}}
	svc := &OpenAIGatewayService{
		cfg:          &config.Config{},
		httpUpstream: upstream,
	}
	account := &Account{
		ID:       88,
		Platform: PlatformOpenAI,
		Type:     AccountTypeAPIKey,
		Credentials: map[string]any{
			"api_key":  "sk-test",
			"base_url": "https://api.siliconflow.cn/v1",
			"model_mapping": map[string]any{
				"bge-reranker-v2-m3": "BAAI/bge-reranker-v2-m3",
			},
		},
	}

	result, err := svc.ForwardRerank(context.Background(), c, account, reqBody, "")

	require.NoError(t, err)
	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, upstreamBody, rec.Body.String())
	require.Equal(t, "application/json", rec.Header().Get("Content-Type"))
	require.NotNil(t, result)
	require.Equal(t, "rerank-rid", result.RequestID)
	require.Equal(t, "bge-reranker-v2-m3", result.Model)
	require.Equal(t, "BAAI/bge-reranker-v2-m3", result.BillingModel)
	require.Equal(t, "BAAI/bge-reranker-v2-m3", result.UpstreamModel)
	require.Equal(t, 17, result.Usage.InputTokens)
	require.Equal(t, 0, result.Usage.OutputTokens)
	require.Equal(t, "https://api.siliconflow.cn/v1/rerank", upstream.lastReq.URL.String())
	require.Equal(t, "Bearer sk-test", upstream.lastReq.Header.Get("Authorization"))
	require.Equal(t, "application/json", upstream.lastReq.Header.Get("Content-Type"))
	// 请求体除 model 映射外逐字节原样透传。
	require.JSONEq(t, `{
		"model":"BAAI/bge-reranker-v2-m3",
		"query":"what is sub2api",
		"documents":["doc-a","doc-b","doc-c"],
		"top_n":2,
		"return_documents":true
	}`, string(upstream.lastBody))
	require.Equal(t, int64(2), gjson.GetBytes(upstream.lastBody, "top_n").Int())
}

func TestForwardRerank_UnmappedBodyPassesThroughUnchanged(t *testing.T) {
	gin.SetMode(gin.TestMode)

	reqBody := []byte(`{"model":"BAAI/bge-reranker-v2-m3","query":"q","documents":["a","b"]}`)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/rerank", bytes.NewReader(reqBody))

	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(strings.NewReader(`{"results":[],"usage":{"total_tokens":5}}`)),
	}}
	svc := &OpenAIGatewayService{cfg: &config.Config{}, httpUpstream: upstream}
	account := &Account{
		ID:       89,
		Platform: PlatformOpenAI,
		Type:     AccountTypeAPIKey,
		Credentials: map[string]any{
			"api_key":  "sk-test",
			"base_url": "https://api.siliconflow.cn",
		},
	}

	result, err := svc.ForwardRerank(context.Background(), c, account, reqBody, "")

	require.NoError(t, err)
	require.Equal(t, reqBody, upstream.lastBody)
	require.Equal(t, "https://api.siliconflow.cn/v1/rerank", upstream.lastReq.URL.String())
	require.NotNil(t, result)
	require.Equal(t, 5, result.Usage.InputTokens)
	require.Equal(t, "BAAI/bge-reranker-v2-m3", result.UpstreamModel)
}

func TestForwardRerank_UpstreamErrorIsPassedThrough(t *testing.T) {
	gin.SetMode(gin.TestMode)

	reqBody := []byte(`{"model":"bge-reranker-v2-m3","query":"q","documents":["a"]}`)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/rerank", bytes.NewReader(reqBody))

	upstreamBody := `{"code":20012,"message":"Model does not exist"}`
	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusNotFound,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(strings.NewReader(upstreamBody)),
	}}
	svc := &OpenAIGatewayService{cfg: &config.Config{}, httpUpstream: upstream}
	account := &Account{
		ID:       90,
		Platform: PlatformOpenAI,
		Type:     AccountTypeAPIKey,
		Credentials: map[string]any{
			"api_key":  "sk-test",
			"base_url": "https://api.siliconflow.cn/v1",
		},
	}

	result, err := svc.ForwardRerank(context.Background(), c, account, reqBody, "")

	require.Error(t, err)
	require.Nil(t, result)
	// 上游状态码与响应体原样回写，不做错误格式转换。
	require.Equal(t, http.StatusNotFound, rec.Code)
	require.Equal(t, upstreamBody, rec.Body.String())
}

func TestAccountSupportsOpenAIEndpointCapability_Rerank(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		credentials map[string]any
		want        bool
	}{
		{
			name:        "unconfigured capabilities allows rerank",
			credentials: map[string]any{"api_key": "sk-test"},
			want:        true,
		},
		{
			name:        "embeddings whitelist excludes rerank",
			credentials: map[string]any{"api_key": "sk-test", "openai_capabilities": []any{"embeddings"}},
			want:        false,
		},
		{
			name:        "chat completions whitelist excludes rerank",
			credentials: map[string]any{"api_key": "sk-test", "openai_capabilities": []any{"chat_completions", "embeddings"}},
			want:        false,
		},
		{
			name:        "rerank whitelist allows rerank",
			credentials: map[string]any{"api_key": "sk-test", "openai_capabilities": []any{"rerank"}},
			want:        true,
		},
		{
			name:        "embeddings and rerank whitelist allows rerank",
			credentials: map[string]any{"api_key": "sk-test", "openai_capabilities": []any{"embeddings", "rerank"}},
			want:        true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			account := &Account{
				ID:          91,
				Platform:    PlatformOpenAI,
				Type:        AccountTypeAPIKey,
				Credentials: tt.credentials,
			}
			require.Equal(t, tt.want, account.SupportsOpenAIEndpointCapability(OpenAIEndpointCapabilityRerank))
		})
	}
}

func TestAccountSupportsOpenAIEndpointCapability_RerankRequiresAPIKeyAccount(t *testing.T) {
	t.Parallel()

	oauth := &Account{
		ID:          92,
		Platform:    PlatformOpenAI,
		Type:        AccountTypeOAuth,
		Credentials: map[string]any{"openai_capabilities": []any{"rerank"}},
	}
	require.False(t, oauth.SupportsOpenAIEndpointCapability(OpenAIEndpointCapabilityRerank))
}
