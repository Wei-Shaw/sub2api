package service

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

type upstreamModelMetadataRepoStub struct {
	AccountRepository
	accountID int64
	updates   map[string]any
	err       error
}

func headerValuesEqualFold(header http.Header, name string) []string {
	for key, values := range header {
		if strings.EqualFold(key, name) {
			return values
		}
	}
	return nil
}

func (r *upstreamModelMetadataRepoStub) UpdateExtra(_ context.Context, id int64, updates map[string]any) error {
	r.accountID = id
	r.updates = updates
	return r.err
}

func upstreamModelSyncTestConfig() *config.Config {
	return &config.Config{
		Security: config.SecurityConfig{
			URLAllowlist: config.URLAllowlistConfig{Enabled: false},
		},
	}
}

func grokOAuthModelSyncTestAccount(baseURL string) *Account {
	credentials := map[string]any{
		"access_token":  "oauth-access-token",
		"refresh_token": "oauth-refresh-token",
		"expires_at":    time.Now().Add(time.Hour).UTC().Format(time.RFC3339),
		"sub":           "grok-user-id",
		"email":         "grok-user@example.com",
	}
	if strings.TrimSpace(baseURL) != "" {
		credentials["base_url"] = baseURL
	}
	return &Account{
		ID:          10,
		Platform:    PlatformGrok,
		Type:        AccountTypeOAuth,
		Credentials: credentials,
	}
}

func TestBuildV1ModelsURL(t *testing.T) {
	t.Parallel()

	require.Equal(t, "https://api.anthropic.com/v1/models", buildV1ModelsURL("https://api.anthropic.com"))
	require.Equal(t, "https://api.anthropic.com/v1/models", buildV1ModelsURL("https://api.anthropic.com/v1"))
	require.Equal(t, "https://api.anthropic.com/v1/models", buildV1ModelsURL("https://api.anthropic.com/v1/models"))
	require.Equal(t, "https://gateway.example.com/antigravity/v1/models", buildV1ModelsURL("https://gateway.example.com/antigravity/"))
}

func TestBuildOpenAIModelsURL(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		base string
		want string
	}{
		{
			name: "zhipu v4 coding base url",
			base: "https://open.bigmodel.cn/api/coding/paas/v4",
			want: "https://open.bigmodel.cn/api/coding/paas/v4/models",
		},
		{
			name: "openai v1 base url",
			base: "https://api.openai.com/v1",
			want: "https://api.openai.com/v1/models",
		},
		{
			name: "models url unchanged",
			base: "https://api.openai.com/v1/models",
			want: "https://api.openai.com/v1/models",
		},
		{
			name: "host fallback uses v1",
			base: "https://api.openai.com",
			want: "https://api.openai.com/v1/models",
		},
		{
			name: "trailing slash on v4",
			base: "https://open.bigmodel.cn/api/coding/paas/v4/",
			want: "https://open.bigmodel.cn/api/coding/paas/v4/models",
		},
		{
			name: "v2 base url",
			base: "https://gateway.example.com/openai/v2",
			want: "https://gateway.example.com/openai/v2/models",
		},
		{
			name: "v3 base url",
			base: "https://gateway.example.com/openai/v3",
			want: "https://gateway.example.com/openai/v3/models",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			require.Equal(t, tt.want, buildOpenAIModelsURL(tt.base))
		})
	}
}

func TestBuildGeminiModelsURL(t *testing.T) {
	t.Parallel()

	require.Equal(t, "https://generativelanguage.googleapis.com/v1beta/models", buildGeminiModelsURL("https://generativelanguage.googleapis.com"))
	require.Equal(t, "https://generativelanguage.googleapis.com/v1beta/models", buildGeminiModelsURL("https://generativelanguage.googleapis.com/v1beta"))
	require.Equal(t, "https://generativelanguage.googleapis.com/v1beta/models", buildGeminiModelsURL("https://generativelanguage.googleapis.com/v1beta/models"))
}

func TestExtractUpstreamModelIDs(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		body string
		want []string
	}{
		{
			name: "openai and anthropic data array",
			body: `{"data":[{"id":"claude-sonnet-4-5"},{"id":"gpt-5"},{"id":"gpt-5"},{"id":""}]}`,
			want: []string{"claude-sonnet-4-5", "gpt-5"},
		},
		{
			name: "gemini models array strips prefix",
			body: `{"models":[{"name":"models/gemini-2.5-pro"},{"name":"gemini-2.5-flash"}]}`,
			want: []string{"gemini-2.5-flash", "gemini-2.5-pro"},
		},
		{
			name: "top level array",
			body: `[{"id":"z-model"},{"name":"models/a-model"}]`,
			want: []string{"a-model", "z-model"},
		},
		{
			name: "standard id wins over provider-specific model field",
			body: `{"data":[{"id":"canonical-id","model":"display-model"}]}`,
			want: []string{"canonical-id"},
		},
		{
			name: "codex manifest uses slug",
			body: `{"models":[{"slug":"gpt-5.6-sol"},{"slug":"gpt-5.5-codex"}]}`,
			want: []string{"gpt-5.5-codex", "gpt-5.6-sol"},
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := extractUpstreamModelIDs([]byte(tt.body))
			require.NoError(t, err)
			require.Equal(t, tt.want, got)
		})
	}
}

func TestBuildUpstreamModelsRequestSupportsOpenAIOAuth(t *testing.T) {
	svc := &AccountTestService{cfg: upstreamModelSyncTestConfig()}
	account := &Account{
		ID:       11,
		Platform: PlatformOpenAI,
		Type:     AccountTypeOAuth,
		Credentials: map[string]any{
			"access_token":       "openai-oauth-token",
			"chatgpt_account_id": "chatgpt-account",
		},
	}

	req, err := svc.buildUpstreamModelsRequest(context.Background(), account)
	require.NoError(t, err)
	require.Equal(t, chatgptCodexModelsURL, req.URL.Scheme+"://"+req.URL.Host+req.URL.Path)
	require.NotEmpty(t, req.URL.Query().Get("client_version"))
	require.Equal(t, "Bearer openai-oauth-token", req.Header.Get("Authorization"))
	require.Equal(t, "chatgpt-account", req.Header.Get("chatgpt-account-id"))
	require.NotEmpty(t, req.Header.Get("Originator"))
	require.NotEmpty(t, req.Header.Get("User-Agent"))
	require.NotEmpty(t, req.Header.Get("Version"))
}

func TestFetchUpstreamSupportedModelsParsesOpenAIOAuthManifest(t *testing.T) {
	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(strings.NewReader(`{"models":[{"slug":"gpt-5.6-sol"},{"slug":"gpt-5.5-codex"}]}`)),
	}}
	svc := &AccountTestService{
		httpUpstream: upstream,
		cfg:          upstreamModelSyncTestConfig(),
	}

	models, err := svc.FetchUpstreamSupportedModels(context.Background(), &Account{
		ID:       12,
		Platform: PlatformOpenAI,
		Type:     AccountTypeOAuth,
		Credentials: map[string]any{
			"access_token": "openai-oauth-token",
		},
	})
	require.NoError(t, err)
	require.Equal(t, []string{"gpt-5.5-codex", "gpt-5.6-sol"}, models)
	require.Equal(t, "Bearer openai-oauth-token", upstream.lastReq.Header.Get("Authorization"))
}

func TestExtractGrokUpstreamModelIDs(t *testing.T) {
	t.Parallel()

	models, err := extractGrokUpstreamModelIDs([]byte(`{"data":[{"id":"display-id","model":"grok-4.5"},{"modelId":"grok-build-0.1"},{"model_id":"grok-composer-2.5-fast"},{"name":"Grok Meta Display Name","_meta":{"model":"grok-meta"}},{"name":"grok-name"},{"id":"grok-safe","_meta":"not-an-object"}]}`))
	require.NoError(t, err)
	require.Equal(t, []string{"grok-4.5", "grok-build-0.1", "grok-composer-2.5-fast", "grok-meta", "grok-name", "grok-safe"}, models)
}

func TestBuildUpstreamModelsRequestsForAPIKeyAccounts(t *testing.T) {
	t.Parallel()

	svc := &AccountTestService{cfg: upstreamModelSyncTestConfig()}
	ctx := context.Background()

	anthropicReq, err := svc.buildAnthropicUpstreamModelsRequest(ctx, &Account{
		Platform: PlatformAnthropic,
		Type:     AccountTypeAPIKey,
		Credentials: map[string]any{
			"api_key":  "anthropic-key",
			"base_url": "https://anthropic.example.com/v1",
		},
	})
	require.NoError(t, err)
	require.Equal(t, "https://anthropic.example.com/v1/models", anthropicReq.URL.String())
	require.Equal(t, "anthropic-key", anthropicReq.Header.Get("x-api-key"))
	require.Equal(t, "2023-06-01", anthropicReq.Header.Get("anthropic-version"))

	anthropicBearerReq, err := svc.buildAnthropicUpstreamModelsRequest(ctx, &Account{
		Platform: PlatformAnthropic,
		Type:     AccountTypeAPIKey,
		Credentials: map[string]any{
			"api_key":  "ollama-key",
			"base_url": "https://ollama.com",
		},
		Extra: map[string]any{
			"anthropic_apikey_auth_scheme": AnthropicAPIKeyAuthSchemeAuthorizationBearer,
		},
	})
	require.NoError(t, err)
	require.Equal(t, "https://ollama.com/v1/models", anthropicBearerReq.URL.String())
	require.Equal(t, "Bearer ollama-key", anthropicBearerReq.Header.Get("Authorization"))
	require.Empty(t, anthropicBearerReq.Header.Get("x-api-key"))
	require.Equal(t, "2023-06-01", anthropicBearerReq.Header.Get("anthropic-version"))

	openAIReq, err := svc.buildOpenAIUpstreamModelsRequest(ctx, &Account{
		Platform: PlatformOpenAI,
		Type:     AccountTypeAPIKey,
		Credentials: map[string]any{
			"api_key":  "openai-key",
			"base_url": "https://openai.example.com",
		},
	})
	require.NoError(t, err)
	require.Equal(t, "https://openai.example.com/v1/models", openAIReq.URL.String())
	require.Equal(t, "Bearer openai-key", openAIReq.Header.Get("Authorization"))

	grokReq, err := svc.buildUpstreamModelsRequest(ctx, &Account{
		Platform: PlatformGrok,
		Type:     AccountTypeAPIKey,
		Credentials: map[string]any{
			"api_key":  "xai-key",
			"base_url": "https://xai.example.com/v1",
		},
	})
	require.NoError(t, err)
	require.Equal(t, "https://xai.example.com/v1/models", grokReq.URL.String())
	require.Equal(t, "Bearer xai-key", grokReq.Header.Get("Authorization"))

	geminiReq, err := svc.buildGeminiUpstreamModelsRequest(ctx, &Account{
		Platform: PlatformGemini,
		Type:     AccountTypeAPIKey,
		Credentials: map[string]any{
			"api_key":  "gemini-key",
			"base_url": "https://generativelanguage.googleapis.com/v1beta",
		},
	})
	require.NoError(t, err)
	require.Equal(t, "https://generativelanguage.googleapis.com/v1beta/models", geminiReq.URL.String())
	require.Equal(t, "gemini-key", geminiReq.Header.Get("x-goog-api-key"))

	antigravityReq, err := svc.buildAntigravityAPIKeyModelsRequest(ctx, &Account{
		Platform: PlatformAntigravity,
		Type:     AccountTypeAPIKey,
		Credentials: map[string]any{
			"api_key":  "antigravity-key",
			"base_url": "https://gateway.example.com/antigravity",
		},
	})
	require.NoError(t, err)
	require.Equal(t, "https://gateway.example.com/antigravity/v1/models", antigravityReq.URL.String())
	require.Equal(t, "antigravity-key", antigravityReq.Header.Get("x-api-key"))
}

func TestBuildUpstreamModelsRequestSupportsGrokOAuth(t *testing.T) {
	t.Parallel()

	svc := &AccountTestService{
		cfg:               upstreamModelSyncTestConfig(),
		grokTokenProvider: NewGrokTokenProvider(nil, nil),
	}
	req, err := svc.buildUpstreamModelsRequest(context.Background(), grokOAuthModelSyncTestAccount(""))
	require.NoError(t, err)
	require.Equal(t, "https://cli-chat-proxy.grok.com/v1/models", req.URL.String())
	require.Equal(t, "Bearer oauth-access-token", req.Header.Get("Authorization"))
	require.Equal(t, grokCLIVersion, req.Header.Get("X-Grok-Client-Version"))
	require.Equal(t, "interactive", req.Header.Get("X-Grok-Client-Mode"))
	require.Equal(t, defaultGrokUpstreamUserAgent(), req.Header.Get("User-Agent"))
	require.Equal(t, "grok-user-id", req.Header.Get("X-UserID"))
	require.Equal(t, "grok-user@example.com", req.Header.Get("X-Email"))
	require.NotContains(t, req.Header.Get("Authorization"), "oauth-refresh-token")
}

func TestBuildUpstreamModelsRequestGrokOAuthRequiresTokenProvider(t *testing.T) {
	t.Parallel()

	svc := &AccountTestService{cfg: upstreamModelSyncTestConfig()}
	_, err := svc.buildUpstreamModelsRequest(context.Background(), grokOAuthModelSyncTestAccount(""))
	require.Error(t, err)

	var syncErr *UpstreamModelSyncError
	require.True(t, errors.As(err, &syncErr))
	require.Equal(t, UpstreamModelSyncErrorConfiguration, syncErr.Kind)
	require.Contains(t, syncErr.SafeMessage(), "token provider")
}

func TestBuildAntigravityAPIKeyModelsRequestRejectsOfficialCloudCodeBase(t *testing.T) {
	t.Parallel()

	svc := &AccountTestService{cfg: upstreamModelSyncTestConfig()}
	_, err := svc.buildAntigravityAPIKeyModelsRequest(context.Background(), &Account{
		Platform: PlatformAntigravity,
		Type:     AccountTypeAPIKey,
		Credentials: map[string]any{
			"api_key":  "antigravity-key",
			"base_url": "https://cloudcode-pa.googleapis.com",
		},
	})
	require.Error(t, err)

	var syncErr *UpstreamModelSyncError
	require.True(t, errors.As(err, &syncErr))
	require.Equal(t, UpstreamModelSyncErrorUnsupported, syncErr.Kind)
	require.Contains(t, syncErr.SafeMessage(), "compatible gateway")
}

func TestBuildAnthropicUpstreamModelsRequestRejectsBedrock(t *testing.T) {
	t.Parallel()

	svc := &AccountTestService{cfg: upstreamModelSyncTestConfig()}
	_, err := svc.buildAnthropicUpstreamModelsRequest(context.Background(), &Account{
		Platform: PlatformAnthropic,
		Type:     AccountTypeBedrock,
	})
	require.Error(t, err)

	var syncErr *UpstreamModelSyncError
	require.True(t, errors.As(err, &syncErr))
	require.Equal(t, UpstreamModelSyncErrorUnsupported, syncErr.Kind)
}

func TestFetchUpstreamSupportedModelsParsesOpenAIResponse(t *testing.T) {
	t.Parallel()

	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(strings.NewReader(`{"data":[{"id":"gpt-5"},{"id":"gpt-5"},{"name":"o3"}]}`)),
	}}
	svc := &AccountTestService{
		httpUpstream: upstream,
		cfg:          upstreamModelSyncTestConfig(),
	}

	models, err := svc.FetchUpstreamSupportedModels(context.Background(), &Account{
		ID:       7,
		Platform: PlatformOpenAI,
		Type:     AccountTypeAPIKey,
		Credentials: map[string]any{
			"api_key":  "openai-key",
			"base_url": "https://openai.example.com/v1",
		},
	})
	require.NoError(t, err)
	require.Equal(t, []string{"gpt-5", "o3"}, models)
	require.Equal(t, "https://openai.example.com/v1/models", upstream.lastReq.URL.String())
	require.Equal(t, "Bearer openai-key", upstream.lastReq.Header.Get("Authorization"))
}

// Scenario: ID-only 模型列表从 Models.dev 补齐能力。
func TestSyncUpstreamModelCatalogEnrichesOpenCodeIDOnlyListAndPersistsSnapshot(t *testing.T) {
	upstream := &httpUpstreamRecorder{responses: []*http.Response{
		{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body:       io.NopCloser(strings.NewReader(`{"object":"list","data":[{"id":"x-preview-f-free","object":"model"}]}`)),
		},
		{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body: io.NopCloser(strings.NewReader(`{
				"opencode": {
					"id": "opencode",
					"name": "OpenCode Zen",
					"api": "https://opencode.ai/zen/v1",
					"models": {
						"x-preview-f-free": {
							"id": "x-preview-f-free",
							"name": "Ox Alpha Free (Unlimited)",
							"description": "Stealth reasoning model for coding, agentic tasks, and tool use",
							"reasoning": true,
							"reasoning_options": [{"type":"effort","values":["low","high","max"]}],
							"modalities": {"input":["text","image","video"],"output":["text"]},
							"limit": {"context":1000000,"output":131072}
						}
					}
				}
			}`)),
		},
	}}
	repo := &upstreamModelMetadataRepoStub{}
	svc := &AccountTestService{
		accountRepo:  repo,
		httpUpstream: upstream,
		cfg:          upstreamModelSyncTestConfig(),
	}
	account := &Account{
		ID:       91,
		Platform: PlatformOpenAI,
		Type:     AccountTypeAPIKey,
		Credentials: map[string]any{
			"api_key":                 "opencode-key",
			"base_url":                "https://opencode.ai/zen/v1",
			"header_override_enabled": true,
			"header_overrides": map[string]any{
				"X-Custom-Account-Header": "account-secret",
			},
		},
	}

	catalog, err := svc.SyncUpstreamModelCatalog(context.Background(), account)
	require.NoError(t, err)
	require.Equal(t, []string{"x-preview-f-free"}, catalog.Models)
	require.Len(t, upstream.requests, 2)
	require.Equal(t, "https://opencode.ai/zen/v1/models", upstream.requests[0].URL.String())
	require.Equal(t, []string{"account-secret"}, headerValuesEqualFold(upstream.requests[0].Header, "X-Custom-Account-Header"))
	require.Equal(t, modelsDevRegistryURL, upstream.requests[1].URL.String())
	require.Empty(t, upstream.requests[1].Header.Get("Authorization"))
	require.Empty(t, upstream.requests[1].Header.Get("x-api-key"))
	require.Empty(t, headerValuesEqualFold(upstream.requests[1].Header, "X-Custom-Account-Header"))

	metadata := catalog.Metadata["x-preview-f-free"]
	require.Equal(t, "Ox Alpha Free (Unlimited)", metadata.DisplayName)
	require.NotNil(t, metadata.Reasoning)
	require.True(t, *metadata.Reasoning)
	require.Equal(t, []string{"low", "high", "max"}, metadata.SupportedReasoningLevels)
	require.Equal(t, []string{"text", "image"}, metadata.InputModalities)
	require.Equal(t, int64(1_000_000), metadata.ContextWindow)
	require.Equal(t, int64(131_072), metadata.MaxOutputTokens)
	require.Equal(t, int64(91), repo.accountID)

	rawSnapshot, ok := repo.updates[UpstreamModelMetadataExtraKey]
	require.True(t, ok)
	encoded, err := json.Marshal(rawSnapshot)
	require.NoError(t, err)
	var snapshot UpstreamModelMetadataSnapshot
	require.NoError(t, json.Unmarshal(encoded, &snapshot))
	require.Equal(t, "models.dev", snapshot.Source)
	require.Equal(t, metadata, snapshot.Models["x-preview-f-free"])
}

// Scenario: 不提供 /models 的兼容上游使用管理员已配置模型继续同步能力。
func TestSyncUpstreamModelCatalogUsesConfiguredModelsWhenListEndpointUnsupported(t *testing.T) {
	upstream := &httpUpstreamRecorder{responses: []*http.Response{
		{
			StatusCode: http.StatusNotFound,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body:       io.NopCloser(strings.NewReader(`{"error":"not found"}`)),
		},
		{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body: io.NopCloser(strings.NewReader(`{
				"configured-provider": {
					"id": "configured-provider",
					"name": "Configured Provider",
					"api": "https://provider.example/v1",
					"models": {
						"glm-5.3": {
							"id": "glm-5.3",
							"name": "GLM-5.3",
							"reasoning": true,
							"reasoning_options": [{"type":"effort","values":["low","medium","high"]}],
							"modalities": {"input":["text"],"output":["text"]},
							"limit": {"context":1000000,"output":131072}
						}
					}
				}
			}`)),
		},
	}}
	repo := &upstreamModelMetadataRepoStub{}
	svc := &AccountTestService{accountRepo: repo, httpUpstream: upstream, cfg: upstreamModelSyncTestConfig()}
	account := &Account{
		ID: 97, Platform: PlatformOpenAI, Type: AccountTypeAPIKey,
		Credentials: map[string]any{
			"api_key":  "key",
			"base_url": "https://provider.example/v1",
			"model_mapping": map[string]any{
				"public-glm": "glm-5.3",
				"duplicate":  "glm-5.3",
				"wildcard":   "glm-*",
				"empty":      "",
			},
		},
	}

	catalog, err := svc.SyncUpstreamModelCatalog(context.Background(), account)
	require.NoError(t, err)
	require.Equal(t, []string{"glm-5.3"}, catalog.Models)
	require.Empty(t, catalog.Warnings)
	require.Len(t, upstream.requests, 2)
	require.Equal(t, "https://provider.example/v1/models", upstream.requests[0].URL.String())
	require.Equal(t, modelsDevRegistryURL, upstream.requests[1].URL.String())
	metadata := catalog.Metadata["glm-5.3"]
	require.Equal(t, []string{"low", "medium", "high"}, metadata.SupportedReasoningLevels)
	require.Equal(t, []string{"text"}, metadata.InputModalities)
	require.Equal(t, int64(1_000_000), metadata.ContextWindow)
	require.NotNil(t, repo.updates)
}

func TestSyncUpstreamModelCatalogDoesNotUseConfiguredModelsForRealUpstreamFailures(t *testing.T) {
	tests := []struct {
		name       string
		statusCode int
	}{
		{name: "unauthorized", statusCode: http.StatusUnauthorized},
		{name: "rate limited", statusCode: http.StatusTooManyRequests},
		{name: "server error", statusCode: http.StatusBadGateway},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			upstream := &httpUpstreamRecorder{resp: &http.Response{
				StatusCode: tt.statusCode,
				Body:       io.NopCloser(strings.NewReader(`{"error":"failed"}`)),
			}}
			svc := &AccountTestService{httpUpstream: upstream, cfg: upstreamModelSyncTestConfig()}

			_, err := svc.SyncUpstreamModelCatalog(context.Background(), &Account{
				ID: 98, Platform: PlatformOpenAI, Type: AccountTypeAPIKey,
				Credentials: map[string]any{
					"api_key":       "key",
					"base_url":      "https://provider.example/v1",
					"model_mapping": map[string]any{"public-glm": "glm-5.3"},
				},
			})
			require.Error(t, err)
			require.Len(t, upstream.requests, 1)
			require.Equal(t, tt.statusCode, upstreamModelSyncStatusCode(err))
		})
	}
}

func TestSyncUpstreamModelCatalogRequiresConfiguredModelsForUnsupportedListEndpoint(t *testing.T) {
	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusMethodNotAllowed,
		Body:       io.NopCloser(strings.NewReader(`{"error":"method not allowed"}`)),
	}}
	svc := &AccountTestService{httpUpstream: upstream, cfg: upstreamModelSyncTestConfig()}

	_, err := svc.SyncUpstreamModelCatalog(context.Background(), &Account{
		ID: 99, Platform: PlatformOpenAI, Type: AccountTypeAPIKey,
		Credentials: map[string]any{"api_key": "key", "base_url": "https://provider.example/v1"},
	})
	require.Error(t, err)
	require.Equal(t, http.StatusMethodNotAllowed, upstreamModelSyncStatusCode(err))
	require.Len(t, upstream.requests, 1)
}

// Scenario: 完整上游模型清单优先保存能力。
func TestSyncUpstreamModelCatalogPrefersDirectUpstreamMetadata(t *testing.T) {
	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body: io.NopCloser(strings.NewReader(`{"models":[{
			"slug":"custom-thinking-model",
			"display_name":"Upstream Display",
			"description":"Upstream description",
			"default_reasoning_level":"high",
			"supported_reasoning_levels":[{"effort":"low"},{"effort":"high"},{"effort":"ultra"}],
			"input_modalities":["text","image"],
			"context_window":256000
		}]}`)),
	}}
	repo := &upstreamModelMetadataRepoStub{}
	svc := &AccountTestService{accountRepo: repo, httpUpstream: upstream, cfg: upstreamModelSyncTestConfig()}

	catalog, err := svc.SyncUpstreamModelCatalog(context.Background(), &Account{
		ID: 92, Platform: PlatformOpenAI, Type: AccountTypeAPIKey,
		Credentials: map[string]any{"api_key": "key", "base_url": "https://provider.example/v1"},
	})
	require.NoError(t, err)
	require.Len(t, upstream.requests, 1, "complete upstream metadata must not be replaced by a registry fetch")
	metadata := catalog.Metadata["custom-thinking-model"]
	require.Equal(t, "Upstream Display", metadata.DisplayName)
	require.Equal(t, "high", metadata.DefaultReasoningLevel)
	require.Equal(t, []string{"low", "high", "ultra"}, metadata.SupportedReasoningLevels)
	require.Equal(t, []string{"text", "image"}, metadata.InputModalities)
	require.Equal(t, int64(256_000), metadata.ContextWindow)
}

// Scenario: 上游 /models 增删型号后，正式同步用最新清单替换能力快照。
func TestSyncUpstreamModelCatalogReplacesSnapshotWhenUpstreamModelsChange(t *testing.T) {
	upstream := &httpUpstreamRecorder{responses: []*http.Response{
		{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body: io.NopCloser(strings.NewReader(`{"data":[
				{"id":"old-model","reasoning":false,"input_modalities":["text"],"context_window":128000},
				{"id":"kept-model","reasoning":false,"input_modalities":["text"],"context_window":128000}
			]}`)),
		},
		{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body: io.NopCloser(strings.NewReader(`{"data":[
				{"id":"kept-model","reasoning":false,"input_modalities":["text"],"context_window":128000},
				{"id":"new-model","reasoning":false,"input_modalities":["text"],"context_window":256000}
			]}`)),
		},
	}}
	repo := &upstreamModelMetadataRepoStub{}
	svc := &AccountTestService{accountRepo: repo, httpUpstream: upstream, cfg: upstreamModelSyncTestConfig()}
	account := &Account{
		ID: 101, Platform: PlatformOpenAI, Type: AccountTypeAPIKey,
		Credentials: map[string]any{
			"api_key":  "key",
			"base_url": "https://provider.example/v1",
		},
	}

	first, err := svc.SyncUpstreamModelCatalog(context.Background(), account)
	require.NoError(t, err)
	require.Equal(t, []string{"kept-model", "old-model"}, first.Models)

	second, err := svc.SyncUpstreamModelCatalog(context.Background(), account)
	require.NoError(t, err)
	require.Equal(t, []string{"kept-model", "new-model"}, second.Models)
	require.NotContains(t, second.Metadata, "old-model")
	require.Contains(t, second.Metadata, "new-model")

	encoded, err := json.Marshal(repo.updates[UpstreamModelMetadataExtraKey])
	require.NoError(t, err)
	var snapshot UpstreamModelMetadataSnapshot
	require.NoError(t, json.Unmarshal(encoded, &snapshot))
	require.NotContains(t, snapshot.Models, "old-model")
	require.Contains(t, snapshot.Models, "new-model")
}

// Scenario: 上游明确声明无推理能力时保存 false。
func TestSyncUpstreamModelCatalogPersistsExplicitNonReasoningCapability(t *testing.T) {
	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body: io.NopCloser(strings.NewReader(`{"models":[{
			"id":"company-coding-model",
			"display_name":"Company Coding Model",
			"reasoning":false,
			"input_modalities":["text"],
			"context_window":64000
		}]}`)),
	}}
	repo := &upstreamModelMetadataRepoStub{}
	svc := &AccountTestService{accountRepo: repo, httpUpstream: upstream, cfg: upstreamModelSyncTestConfig()}

	catalog, err := svc.SyncUpstreamModelCatalog(context.Background(), &Account{
		ID: 94, Platform: PlatformOpenAI, Type: AccountTypeAPIKey,
		Credentials: map[string]any{"api_key": "key", "base_url": "https://provider.example/v1"},
	})
	require.NoError(t, err)
	require.Len(t, upstream.requests, 1)
	metadata := catalog.Metadata["company-coding-model"]
	require.NotNil(t, metadata.Reasoning)
	require.False(t, *metadata.Reasoning)
	require.Empty(t, metadata.SupportedReasoningLevels)
	require.Equal(t, []string{"text"}, metadata.InputModalities)
	require.Equal(t, int64(64_000), metadata.ContextWindow)
	require.NotNil(t, repo.updates)
}

func TestSyncUpstreamModelCatalogClassifiesSnapshotPersistenceFailureAsInternal(t *testing.T) {
	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body: io.NopCloser(strings.NewReader(`{"models":[{
			"id":"company-coding-model",
			"reasoning":false,
			"input_modalities":["text"],
			"context_window":64000
		}]}`)),
	}}
	repo := &upstreamModelMetadataRepoStub{err: errors.New("database unavailable")}
	svc := &AccountTestService{accountRepo: repo, httpUpstream: upstream, cfg: upstreamModelSyncTestConfig()}

	_, err := svc.SyncUpstreamModelCatalog(context.Background(), &Account{
		ID: 95, Platform: PlatformOpenAI, Type: AccountTypeAPIKey,
		Credentials: map[string]any{"api_key": "key", "base_url": "https://provider.example/v1"},
	})
	require.Error(t, err)
	var syncErr *UpstreamModelSyncError
	require.ErrorAs(t, err, &syncErr)
	require.Equal(t, UpstreamModelSyncErrorInternal, syncErr.Kind)
}

// Scenario: 元数据源失败时保留已有快照。
func TestSyncUpstreamModelCatalogDoesNotOverwriteSnapshotWhenRegistryFails(t *testing.T) {
	upstream := &httpUpstreamRecorder{responses: []*http.Response{
		{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(`{"data":[{"id":"x-preview-f-free"}]}`))},
		{StatusCode: http.StatusBadGateway, Body: io.NopCloser(strings.NewReader(`{"error":"unavailable"}`))},
	}}
	repo := &upstreamModelMetadataRepoStub{}
	svc := &AccountTestService{accountRepo: repo, httpUpstream: upstream, cfg: upstreamModelSyncTestConfig()}

	catalog, err := svc.SyncUpstreamModelCatalog(context.Background(), &Account{
		ID: 93, Platform: PlatformOpenAI, Type: AccountTypeAPIKey,
		Credentials: map[string]any{"api_key": "key", "base_url": "https://opencode.ai/zen/v1"},
		Extra: map[string]any{UpstreamModelMetadataExtraKey: map[string]any{
			"source": "models.dev", "models": map[string]any{"x-preview-f-free": map[string]any{"reasoning": true}},
		}},
	})
	require.NoError(t, err)
	require.Equal(t, []string{"x-preview-f-free"}, catalog.Models)
	require.Empty(t, catalog.Metadata)
	require.Equal(t, []UpstreamModelSyncWarning{{
		Code:    UpstreamModelMetadataIncompleteCode,
		Message: "Model IDs were synced, but capability metadata is incomplete.",
	}}, catalog.Warnings)
	require.Nil(t, repo.updates, "a failed metadata enrichment must not erase a previously saved snapshot")
}

func TestSyncUpstreamModelCatalogDoesNotPersistPartialMetadataWhenRegistryFails(t *testing.T) {
	upstream := &httpUpstreamRecorder{responses: []*http.Response{
		{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(`{"models":[{
			"id":"partially-described-model",
			"display_name":"Partial Model"
		}]}`))},
		{StatusCode: http.StatusBadGateway, Body: io.NopCloser(strings.NewReader(`{"error":"unavailable"}`))},
	}}
	repo := &upstreamModelMetadataRepoStub{}
	svc := &AccountTestService{accountRepo: repo, httpUpstream: upstream, cfg: upstreamModelSyncTestConfig()}

	catalog, err := svc.SyncUpstreamModelCatalog(context.Background(), &Account{
		ID: 96, Platform: PlatformOpenAI, Type: AccountTypeAPIKey,
		Credentials: map[string]any{"api_key": "key", "base_url": "https://provider.example/v1"},
		Extra: map[string]any{UpstreamModelMetadataExtraKey: map[string]any{
			"source": "upstream", "models": map[string]any{"partially-described-model": map[string]any{
				"reasoning": true, "supported_reasoning_levels": []any{"low", "high"},
			}},
		}},
	})
	require.NoError(t, err)
	require.Equal(t, []string{"partially-described-model"}, catalog.Models)
	require.Equal(t, "Partial Model", catalog.Metadata["partially-described-model"].DisplayName)
	require.Equal(t, UpstreamModelMetadataIncompleteCode, catalog.Warnings[0].Code)
	require.Nil(t, repo.updates, "partial metadata must not replace a more complete persisted snapshot")
}

func TestFetchUpstreamSupportedModelsUsesConfiguredBodyLimit(t *testing.T) {
	t.Parallel()

	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(strings.NewReader(`{"data":[{"id":"gpt-5"}]}`)),
	}}
	cfg := upstreamModelSyncTestConfig()
	cfg.Gateway.ModelsListReadMaxBytes = 8
	svc := &AccountTestService{httpUpstream: upstream, cfg: cfg}

	_, err := svc.FetchUpstreamSupportedModels(context.Background(), &Account{
		ID:       7,
		Platform: PlatformOpenAI,
		Type:     AccountTypeAPIKey,
		Credentials: map[string]any{
			"api_key":  "openai-key",
			"base_url": "https://openai.example.com/v1",
		},
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "response exceeds 8 bytes")
}

func TestFetchUpstreamSupportedModelsParsesGrokAPIKeyResponse(t *testing.T) {
	t.Parallel()

	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(strings.NewReader(`{"data":[{"id":"grok-4.5"},{"id":"grok-4.5"},{"id":"grok-imagine"}]}`)),
	}}
	svc := &AccountTestService{
		httpUpstream: upstream,
		cfg:          upstreamModelSyncTestConfig(),
	}

	models, err := svc.FetchUpstreamSupportedModels(context.Background(), &Account{
		ID:       9,
		Platform: PlatformGrok,
		Type:     AccountTypeAPIKey,
		Credentials: map[string]any{
			"api_key":  "xai-key",
			"base_url": "https://xai.example.com/v1",
		},
	})
	require.NoError(t, err)
	require.Equal(t, []string{"grok-4.5", "grok-imagine"}, models)
	require.Equal(t, "https://xai.example.com/v1/models", upstream.lastReq.URL.String())
	require.Equal(t, "Bearer xai-key", upstream.lastReq.Header.Get("Authorization"))
}

func TestFetchUpstreamSupportedModelsParsesGrokOAuthResponse(t *testing.T) {
	t.Parallel()

	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(strings.NewReader(`{"data":[{"model":"grok-4.5"},{"model":"grok-4.5"},{"modelId":"grok-build-0.1"}]}`)),
	}}
	svc := &AccountTestService{
		httpUpstream:      upstream,
		cfg:               upstreamModelSyncTestConfig(),
		grokTokenProvider: NewGrokTokenProvider(nil, nil),
	}

	models, err := svc.FetchUpstreamSupportedModels(context.Background(), grokOAuthModelSyncTestAccount(""))
	require.NoError(t, err)
	require.Equal(t, []string{"grok-4.5", "grok-build-0.1"}, models)
	require.Equal(t, "https://cli-chat-proxy.grok.com/v1/models", upstream.lastReq.URL.String())
	require.Equal(t, "Bearer oauth-access-token", upstream.lastReq.Header.Get("Authorization"))
	require.Equal(t, grokCLIVersion, upstream.lastReq.Header.Get("X-Grok-Client-Version"))
	require.Equal(t, "interactive", upstream.lastReq.Header.Get("X-Grok-Client-Mode"))
	require.Equal(t, "grok-user-id", upstream.lastReq.Header.Get("X-UserID"))
	require.Equal(t, "grok-user@example.com", upstream.lastReq.Header.Get("X-Email"))
}

func TestBuildUpstreamModelsRequestGrokOAuthDoesNotSendIdentityToCustomBase(t *testing.T) {
	t.Parallel()

	svc := &AccountTestService{
		cfg:               upstreamModelSyncTestConfig(),
		grokTokenProvider: NewGrokTokenProvider(nil, nil),
	}
	req, err := svc.buildUpstreamModelsRequest(context.Background(), grokOAuthModelSyncTestAccount("https://relay.example/v1"))
	require.NoError(t, err)
	require.Equal(t, "https://relay.example/v1/models", req.URL.String())
	require.Empty(t, req.Header.Get("X-UserID"))
	require.Empty(t, req.Header.Get("X-Email"))
}

func TestFetchUpstreamSupportedModelsDoesNotExposeUpstreamBody(t *testing.T) {
	t.Parallel()

	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusBadGateway,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(strings.NewReader(`{"error":"SECRET_TOKEN should not be exposed"}`)),
	}}
	svc := &AccountTestService{
		httpUpstream: upstream,
		cfg:          upstreamModelSyncTestConfig(),
	}

	_, err := svc.FetchUpstreamSupportedModels(context.Background(), &Account{
		ID:       8,
		Platform: PlatformOpenAI,
		Type:     AccountTypeAPIKey,
		Credentials: map[string]any{
			"api_key":  "openai-key",
			"base_url": "https://openai.example.com/v1",
		},
	})
	require.Error(t, err)
	require.NotContains(t, err.Error(), "SECRET_TOKEN")

	var syncErr *UpstreamModelSyncError
	require.True(t, errors.As(err, &syncErr))
	require.Equal(t, UpstreamModelSyncErrorUpstream, syncErr.Kind)
	require.NotContains(t, syncErr.SafeMessage(), "SECRET_TOKEN")
	require.Contains(t, syncErr.SafeMessage(), "HTTP 502")
}
