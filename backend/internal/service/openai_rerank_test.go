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

func TestBuildOpenRouterRerankURL(t *testing.T) {
	require.Equal(t, "https://openrouter.ai/api/v1/rerank", buildOpenRouterRerankURL("https://openrouter.ai/api/v1"))
	require.Equal(t, "https://openrouter.ai/api/v1/rerank", buildOpenRouterRerankURL("https://openrouter.ai"))
}

func TestBuildOpenRouterEmbeddingsURL(t *testing.T) {
	require.Equal(t, "https://openrouter.ai/api/v1/embeddings", buildOpenRouterEmbeddingsURL("https://openrouter.ai"))
	require.Equal(t, "https://openrouter.ai/api/v1/embeddings", buildOpenRouterEmbeddingsURL("https://openrouter.ai/api"))
	require.Equal(t, "https://openrouter.ai/api/v1/embeddings", buildOpenRouterEmbeddingsURL("https://openrouter.ai/api/v1"))
}

func TestOpenRouterRerankCapabilityRequiresOfficialHostAndExplicitSelection(t *testing.T) {
	baseCredentials := map[string]any{
		"api_key":  "sk-or",
		"base_url": "https://openrouter.ai/api/v1",
	}

	defaultAccount := &Account{Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Credentials: baseCredentials}
	require.True(t, defaultAccount.SupportsOpenAIEndpointCapability(OpenAIEndpointCapabilityRerank))

	explicitlyDisabled := &Account{Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Credentials: map[string]any{
		"api_key":             "sk-or",
		"base_url":            "https://openrouter.ai/api/v1",
		"openai_capabilities": []any{"chat_completions", "embeddings"},
	}}
	require.False(t, explicitlyDisabled.SupportsOpenAIEndpointCapability(OpenAIEndpointCapabilityRerank))

	explicitlyEnabled := &Account{Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Credentials: map[string]any{
		"api_key":             "sk-or",
		"base_url":            "https://openrouter.ai/api/v1",
		"openai_capabilities": []any{"rerank"},
	}}
	require.True(t, explicitlyEnabled.SupportsOpenAIEndpointCapability(OpenAIEndpointCapabilityRerank))

	nonOpenRouter := &Account{Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Credentials: map[string]any{
		"api_key":             "sk-other",
		"base_url":            "https://example.test/v1",
		"openai_capabilities": []any{"rerank"},
	}}
	require.False(t, nonOpenRouter.SupportsOpenAIEndpointCapability(OpenAIEndpointCapabilityRerank))
}

func TestGeminiEmbeddingsCapabilityReadsGenericCredentials(t *testing.T) {
	defaultAccount := &Account{Platform: PlatformGemini, Type: AccountTypeAPIKey}
	require.True(t, defaultAccount.SupportsOpenAIEndpointCapability(OpenAIEndpointCapabilityEmbeddings))

	disabled := &Account{Platform: PlatformGemini, Type: AccountTypeAPIKey, Credentials: map[string]any{
		"endpoint_capabilities": []any{"gemini_native"},
	}}
	require.False(t, disabled.SupportsOpenAIEndpointCapability(OpenAIEndpointCapabilityEmbeddings))

	enabled := &Account{Platform: PlatformGemini, Type: AccountTypeAPIKey, Credentials: map[string]any{
		"endpoint_capabilities": []any{"gemini_native", "embeddings"},
	}}
	require.True(t, enabled.SupportsOpenAIEndpointCapability(OpenAIEndpointCapabilityEmbeddings))
}

func TestForwardRerankUsesOpenRouterContract(t *testing.T) {
	gin.SetMode(gin.TestMode)
	body := []byte(`{"model":"openai/gpt-4o-mini","query":"what is ai?","documents":["AI is useful",{"text":"A sandwich is food","image":"data:image/png;base64,AA=="}],"top_n":1,"return_documents":true}`)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/rerank", bytes.NewReader(body))

	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(strings.NewReader(`{"results":[{"index":0,"relevance_score":0.98}],"usage":{"prompt_tokens":7}}`)),
	}}
	svc := &OpenAIGatewayService{cfg: &config.Config{}, httpUpstream: upstream}
	account := &Account{ID: 77, Platform: PlatformOpenAI, Type: AccountTypeAPIKey,
		Credentials: map[string]any{"api_key": "sk-or", "base_url": "https://openrouter.ai/api/v1"}}

	result, err := svc.ForwardRerank(context.Background(), c, account, body, "")
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, "https://openrouter.ai/api/v1/rerank", upstream.lastReq.URL.String())
	require.Equal(t, "Bearer sk-or", upstream.lastReq.Header.Get("Authorization"))
	require.Equal(t, "openai/gpt-4o-mini", gjson.GetBytes(upstream.lastBody, "model").String())
	require.Equal(t, int64(2), gjson.GetBytes(upstream.lastBody, "documents.#").Int())
	require.True(t, gjson.GetBytes(upstream.lastBody, "return_documents").Bool())
	require.Equal(t, 7, result.Usage.InputTokens)
	require.JSONEq(t, `{"results":[{"index":0,"relevance_score":0.98}],"usage":{"prompt_tokens":7}}`, rec.Body.String())
}
