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

func TestUnifiedEndpointCapabilityMatrix(t *testing.T) {
	tests := []struct {
		name       string
		account    *Account
		embeddings bool
		rerank     bool
	}{
		{
			name: "openai api key",
			account: &Account{Platform: PlatformOpenAI, Type: AccountTypeAPIKey,
				Credentials: map[string]any{"api_key": "sk-openai"}},
			embeddings: true,
		},
		{
			name: "openrouter represented by openai api key",
			account: &Account{Platform: PlatformOpenAI, Type: AccountTypeAPIKey,
				Credentials: map[string]any{"api_key": "sk-or", "base_url": "https://openrouter.ai/api/v1"}},
			embeddings: true,
			rerank:     true,
		},
		{
			name: "zhipu api key",
			account: &Account{Platform: PlatformZhipu, Type: AccountTypeAPIKey,
				Credentials: map[string]any{"api_key": "zhipu-key"}},
			embeddings: true,
		},
		{
			name: "gemini api key",
			account: &Account{Platform: PlatformGemini, Type: AccountTypeAPIKey,
				Credentials: map[string]any{"api_key": "AIza-key"}},
			embeddings: true,
		},
		{
			name: "gemini oauth is not native embedding eligible",
			account: &Account{Platform: PlatformGemini, Type: AccountTypeOAuth,
				Credentials: map[string]any{"access_token": "oauth-token"}},
		},
		{
			name: "kimi has no verified embedding endpoint",
			account: &Account{Platform: PlatformKimi, Type: AccountTypeAPIKey,
				Credentials: map[string]any{"api_key": "kimi-key"}},
		},
		{
			name: "deepseek has no verified embedding endpoint",
			account: &Account{Platform: PlatformDeepseek, Type: AccountTypeAPIKey,
				Credentials: map[string]any{"api_key": "deepseek-key"}},
		},
		{
			name: "anthropic has no embedding model",
			account: &Account{Platform: PlatformAnthropic, Type: AccountTypeAPIKey,
				Credentials: map[string]any{"api_key": "anthropic-key"}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.embeddings, tt.account.SupportsOpenAIEndpointCapability(OpenAIEndpointCapabilityEmbeddings))
			require.Equal(t, tt.rerank, tt.account.SupportsOpenAIEndpointCapability(OpenAIEndpointCapabilityRerank))
		})
	}
}

func TestOpenRouterRerankRequiresOfficialHost(t *testing.T) {
	for _, baseURL := range []string{
		"https://openrouter.ai/api/v1",
		"https://openrouter.ai",
	} {
		account := &Account{Platform: PlatformOpenAI, Type: AccountTypeAPIKey,
			Credentials: map[string]any{"api_key": "sk-or", "base_url": baseURL}}
		require.True(t, account.SupportsOpenAIEndpointCapability(OpenAIEndpointCapabilityRerank), baseURL)
	}
	for _, baseURL := range []string{
		"https://openrouter.ai.evil.example/api/v1",
		"https://relay.openrouter.ai/api/v1",
		"https://api.openai.com/v1",
	} {
		account := &Account{Platform: PlatformOpenAI, Type: AccountTypeAPIKey,
			Credentials: map[string]any{"api_key": "sk", "base_url": baseURL}}
		require.False(t, account.SupportsOpenAIEndpointCapability(OpenAIEndpointCapabilityRerank), baseURL)
	}
}

func TestZhipuEndpointCapabilitiesUseGenericCredentialsAndDefaultToBoth(t *testing.T) {
	defaultAccount := &Account{Platform: PlatformZhipu, Type: AccountTypeAPIKey}
	require.True(t, defaultAccount.SupportsOpenAIEndpointCapability(OpenAIEndpointCapabilityChatCompletions))
	require.True(t, defaultAccount.SupportsOpenAIEndpointCapability(OpenAIEndpointCapabilityEmbeddings))

	embeddingsDisabled := &Account{Platform: PlatformZhipu, Type: AccountTypeAPIKey,
		Credentials: map[string]any{endpointCapabilitiesCredentialKey: []any{"chat_completions"}}}
	require.True(t, embeddingsDisabled.SupportsOpenAIEndpointCapability(OpenAIEndpointCapabilityChatCompletions))
	require.False(t, embeddingsDisabled.SupportsOpenAIEndpointCapability(OpenAIEndpointCapabilityEmbeddings))
}

func TestTranslateOpenAIEmbeddingsToGemini(t *testing.T) {
	model, action, payload, err := translateOpenAIEmbeddingsToGemini([]byte(`{
		"model":"gemini-embedding-001",
		"input":["hello","world"],
		"dimensions":256,
		"encoding_format":"float"
	}`))
	require.NoError(t, err)
	require.Equal(t, "gemini-embedding-001", model)
	require.Equal(t, "batchEmbedContents", action)
	require.JSONEq(t, `{
		"requests":[
			{"model":"models/gemini-embedding-001","content":{"parts":[{"text":"hello"}]},"outputDimensionality":256},
			{"model":"models/gemini-embedding-001","content":{"parts":[{"text":"world"}]},"outputDimensionality":256}
		]
	}`, string(payload))
}

func TestTranslateGeminiEmbeddingResponseToOpenAI(t *testing.T) {
	payload, usage, err := translateGeminiEmbeddingResponse([]byte(`{
		"embeddings":[{"values":[0.1,0.2]},{"values":[0.3,0.4]}],
		"usageMetadata":{"promptTokenCount":7,"totalTokenCount":7}
	}`), "gemini-embedding-001", "float")
	require.NoError(t, err)
	require.JSONEq(t, `{
		"object":"list",
		"data":[
			{"object":"embedding","index":0,"embedding":[0.1,0.2]},
			{"object":"embedding","index":1,"embedding":[0.3,0.4]}
		],
		"model":"gemini-embedding-001",
		"usage":{"prompt_tokens":7,"total_tokens":7}
	}`, string(payload))
	require.Equal(t, 7, usage.InputTokens)
}

func TestTranslateSingleGeminiEmbeddingResponse(t *testing.T) {
	payload, _, err := translateGeminiEmbeddingResponse([]byte(`{"embedding":{"values":[1,2,3]}}`), "gemini-embedding-001", "float")
	require.NoError(t, err)
	require.JSONEq(t, `{"object":"list","data":[{"object":"embedding","index":0,"embedding":[1,2,3]}],"model":"gemini-embedding-001","usage":{}}`, string(payload))
}

func TestCompositeRouteSupportsRerankEndpoint(t *testing.T) {
	require.Equal(t, CompositeRouteEndpointRerank, normalizeCompositeRouteEndpoint("rerank"))
}

func TestCompositeRoutesSelectConcreteProvidersForUnifiedEndpoints(t *testing.T) {
	resolver := NewCompositeRouteResolver(compositeRouteRepoStub{routes: []CompositeModelRoute{
		{
			ID: 1, GroupID: 7, PublicModel: "gemini-embedding-alias", MatchType: CompositeRouteMatchExact,
			TargetPlatform: PlatformGemini, UpstreamModel: "gemini-embedding-001", Endpoint: CompositeRouteEndpointEmbeddings, Enabled: true,
		},
		{
			ID: 2, GroupID: 7, PublicModel: "openrouter-rerank", MatchType: CompositeRouteMatchExact,
			TargetPlatform: PlatformOpenAI, UpstreamModel: "openai/gpt-4o-mini", Endpoint: CompositeRouteEndpointRerank, Enabled: true,
		},
	}})

	embeddings, err := resolver.Resolve(context.Background(), 7, "gemini-embedding-alias", CompositeRouteEndpointEmbeddings)
	require.NoError(t, err)
	require.True(t, embeddings.Matched)
	require.Equal(t, PlatformGemini, embeddings.TargetPlatform)

	rerank, err := resolver.Resolve(context.Background(), 7, "openrouter-rerank", CompositeRouteEndpointRerank)
	require.NoError(t, err)
	require.True(t, rerank.Matched)
	require.Equal(t, PlatformOpenAI, rerank.TargetPlatform)
}

func TestForwardEmbeddingsGeminiUsesNativeBatchEndpoint(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/embeddings", bytes.NewReader([]byte(`{"model":"gemini-embedding-001","input":["hello","world"],"dimensions":128}`)))
	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(strings.NewReader(`{"embeddings":[{"values":[0.1,0.2]},{"values":[0.3,0.4]}]}`)),
	}}
	svc := &OpenAIGatewayService{cfg: &config.Config{}, httpUpstream: upstream}
	account := &Account{ID: 91, Platform: PlatformGemini, Type: AccountTypeAPIKey,
		Credentials: map[string]any{"api_key": "AIza-test"}}

	result, err := svc.ForwardEmbeddings(context.Background(), c, account, []byte(`{"model":"gemini-embedding-001","input":["hello","world"],"dimensions":128}`), "")
	require.NoError(t, err)
	require.Equal(t, "https://generativelanguage.googleapis.com/v1beta/models/gemini-embedding-001:batchEmbedContents", upstream.lastReq.URL.String())
	require.Equal(t, "AIza-test", upstream.lastReq.Header.Get("x-goog-api-key"))
	require.Equal(t, int64(2), gjson.GetBytes(upstream.lastBody, "requests.#").Int())
	require.Equal(t, int64(128), gjson.GetBytes(upstream.lastBody, "requests.0.outputDimensionality").Int())
	require.Equal(t, "list", gjson.GetBytes(rec.Body.Bytes(), "object").String())
	require.Len(t, gjson.GetBytes(rec.Body.Bytes(), "data").Array(), 2)
	require.NotNil(t, result)
}

func TestForwardEmbeddingsZhipuUsesOfficialOpenAIFormatEndpoint(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	body := []byte(`{"model":"embedding-3","input":"hello"}`)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/embeddings", bytes.NewReader(body))
	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(strings.NewReader(`{"object":"list","data":[{"object":"embedding","index":0,"embedding":[0.1]}],"model":"embedding-3","usage":{"prompt_tokens":2,"total_tokens":2}}`)),
	}}
	svc := &OpenAIGatewayService{cfg: &config.Config{}, httpUpstream: upstream}
	account := &Account{ID: 92, Platform: PlatformZhipu, Type: AccountTypeAPIKey,
		Credentials: map[string]any{"api_key": "zhipu-key"}}

	result, err := svc.ForwardEmbeddings(context.Background(), c, account, body, "")
	require.NoError(t, err)
	require.Equal(t, "https://open.bigmodel.cn/api/paas/v4/embeddings", upstream.lastReq.URL.String())
	require.Equal(t, "Bearer zhipu-key", upstream.lastReq.Header.Get("Authorization"))
	require.Equal(t, 2, result.Usage.InputTokens)
}

func TestForwardEmbeddingsOpenRouterUsesOfficialEndpointWhenBaseIsHostOnly(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	body := []byte(`{"model":"openai/text-embedding-3-small","input":"hello"}`)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/embeddings", bytes.NewReader(body))
	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(strings.NewReader(`{"object":"list","data":[{"object":"embedding","index":0,"embedding":[0.1]}],"model":"openai/text-embedding-3-small"}`)),
	}}
	svc := &OpenAIGatewayService{cfg: &config.Config{}, httpUpstream: upstream}
	account := &Account{ID: 94, Platform: PlatformOpenAI, Type: AccountTypeAPIKey,
		Credentials: map[string]any{"api_key": "sk-or", "base_url": "https://openrouter.ai"}}

	_, err := svc.ForwardEmbeddings(context.Background(), c, account, body, "")
	require.NoError(t, err)
	require.Equal(t, "https://openrouter.ai/api/v1/embeddings", upstream.lastReq.URL.String())
	require.Equal(t, "Bearer sk-or", upstream.lastReq.Header.Get("Authorization"))
}

func TestUnifiedEmbeddingSchedulerSelectsGeminiAPIKeyAccount(t *testing.T) {
	groupID := int64(95)
	cfg := &config.Config{}
	cfg.Gateway.Scheduling.LoadBatchEnabled = false
	svc := &OpenAIGatewayService{
		accountRepo: schedulerTestOpenAIAccountRepo{accounts: []Account{{
			ID: 9501, Platform: PlatformGemini, Type: AccountTypeAPIKey,
			Status: StatusActive, Schedulable: true, Concurrency: 1,
			Credentials: map[string]any{"api_key": "AIza-test"},
		}}},
		cache:              &schedulerTestGatewayCache{},
		cfg:                cfg,
		concurrencyService: NewConcurrencyService(schedulerTestConcurrencyCache{}),
	}

	selection, _, err := svc.SelectAccountWithSchedulerForCapability(
		context.Background(), &groupID, "", "", "gemini-embedding-001", nil,
		OpenAIUpstreamTransportHTTPSSE, OpenAIEndpointCapabilityEmbeddings,
		false, false, true, PlatformGemini,
	)
	require.NoError(t, err)
	require.NotNil(t, selection)
	require.Equal(t, int64(9501), selection.Account.ID)
}

func TestUnifiedEmbeddingSchedulerRejectsGeminiNonAPIKeyAccount(t *testing.T) {
	groupID := int64(96)
	cfg := &config.Config{}
	cfg.Gateway.Scheduling.LoadBatchEnabled = false
	svc := &OpenAIGatewayService{
		accountRepo: schedulerTestOpenAIAccountRepo{accounts: []Account{{
			ID: 9601, Platform: PlatformGemini, Type: AccountTypeOAuth,
			Status: StatusActive, Schedulable: true, Concurrency: 1,
			Credentials: map[string]any{"endpoint_capabilities": []any{"gemini_native", "embeddings"}},
		}}},
		cache:              &schedulerTestGatewayCache{},
		cfg:                cfg,
		concurrencyService: NewConcurrencyService(schedulerTestConcurrencyCache{}),
	}

	selection, _, err := svc.SelectAccountWithSchedulerForCapability(
		context.Background(), &groupID, "", "", "gemini-embedding-001", nil,
		OpenAIUpstreamTransportHTTPSSE, OpenAIEndpointCapabilityEmbeddings,
		false, false, true, PlatformGemini,
	)
	require.Error(t, err)
	require.Nil(t, selection)
}

func TestUnifiedEmbeddingSchedulerHonorsZhipuGenericCapability(t *testing.T) {
	groupID := int64(97)
	cfg := &config.Config{}
	cfg.Gateway.Scheduling.LoadBatchEnabled = false
	svc := &OpenAIGatewayService{
		accountRepo: schedulerTestOpenAIAccountRepo{accounts: []Account{
			{
				ID: 9701, Platform: PlatformZhipu, Type: AccountTypeAPIKey,
				Status: StatusActive, Schedulable: true, Concurrency: 1, Priority: 0,
				Credentials: map[string]any{endpointCapabilitiesCredentialKey: []any{"chat_completions"}},
			},
			{
				ID: 9702, Platform: PlatformZhipu, Type: AccountTypeAPIKey,
				Status: StatusActive, Schedulable: true, Concurrency: 1, Priority: 10,
				Credentials: map[string]any{endpointCapabilitiesCredentialKey: []any{"chat_completions", "embeddings"}},
			},
		}},
		cache:              &schedulerTestGatewayCache{},
		cfg:                cfg,
		concurrencyService: NewConcurrencyService(schedulerTestConcurrencyCache{}),
	}

	selection, _, err := svc.SelectAccountWithSchedulerForCapability(
		context.Background(), &groupID, "", "", "embedding-3", nil,
		OpenAIUpstreamTransportHTTPSSE, OpenAIEndpointCapabilityEmbeddings,
		false, false, true, PlatformZhipu,
	)
	require.NoError(t, err)
	require.NotNil(t, selection)
	require.Equal(t, int64(9702), selection.Account.ID)
}

func TestForwardEmbeddingsDoesNotForwardUnsupportedProvider(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	upstream := &httpUpstreamRecorder{resp: &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(`{}`))}}
	svc := &OpenAIGatewayService{cfg: &config.Config{}, httpUpstream: upstream}
	account := &Account{ID: 93, Platform: PlatformDeepseek, Type: AccountTypeAPIKey,
		Credentials: map[string]any{"api_key": "deepseek-key"}}

	result, err := svc.ForwardEmbeddings(context.Background(), c, account, []byte(`{"model":"deepseek-embedding","input":"hello"}`), "")
	require.Error(t, err)
	require.Nil(t, result)
	require.Nil(t, upstream.lastReq)
}
