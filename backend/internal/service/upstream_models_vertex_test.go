package service

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type vertexModelsTokenCacheStub struct {
	token string
	calls int
}

func (c *vertexModelsTokenCacheStub) GetAccessToken(context.Context, string) (string, error) {
	c.calls++
	if c.token == "" {
		return "", errors.New("missing token")
	}
	return c.token, nil
}

func (c *vertexModelsTokenCacheStub) SetAccessToken(context.Context, string, string, time.Duration) error {
	return nil
}

func (c *vertexModelsTokenCacheStub) DeleteAccessToken(context.Context, string) error {
	return nil
}

func (c *vertexModelsTokenCacheStub) AcquireRefreshLock(context.Context, string, time.Duration) (bool, error) {
	return false, nil
}

func (c *vertexModelsTokenCacheStub) ReleaseRefreshLock(context.Context, string) error {
	return nil
}

func vertexModelSyncTestAccount(credentials map[string]any) *Account {
	merged := map[string]any{
		"service_account_json": map[string]any{
			"type":         "service_account",
			"project_id":   "proj",
			"private_key":  "-----BEGIN PRIVATE KEY-----\nabc\n-----END PRIVATE KEY-----\n",
			"client_email": "svc@proj.iam.gserviceaccount.com",
		},
	}
	for key, value := range credentials {
		merged[key] = value
	}
	return &Account{
		ID:          173,
		Platform:    PlatformGemini,
		Type:        AccountTypeServiceAccount,
		Credentials: merged,
	}
}

func vertexModelSyncTestService(upstream *httpUpstreamRecorder) *AccountTestService {
	return &AccountTestService{
		httpUpstream:        upstream,
		cfg:                 upstreamModelSyncTestConfig(),
		geminiTokenProvider: NewGeminiTokenProvider(nil, &vertexModelsTokenCacheStub{token: "ya29.vertex-token"}, nil),
	}
}

func vertexModelsJSONResponse(status int, body string) *http.Response {
	return &http.Response{
		StatusCode: status,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(strings.NewReader(body)),
	}
}

func TestBuildVertexPublisherModelsURL(t *testing.T) {
	t.Parallel()

	u, err := buildVertexPublisherModelsURL("global", "")
	require.NoError(t, err)
	require.Equal(t, "https://aiplatform.googleapis.com/v1beta1/publishers/google/models?pageSize=300", u)

	u, err = buildVertexPublisherModelsURL("us-central1", "next/page+token")
	require.NoError(t, err)
	require.Equal(t, "https://us-central1-aiplatform.googleapis.com/v1beta1/publishers/google/models?pageSize=300&pageToken=next%2Fpage%2Btoken", u)

	u, err = buildVertexPublisherModelsURL("", "")
	require.NoError(t, err)
	require.Equal(t, "https://us-central1-aiplatform.googleapis.com/v1beta1/publishers/google/models?pageSize=300", u)

	_, err = buildVertexPublisherModelsURL("evil.com/x", "")
	require.Error(t, err)
}

// Scenario: Vertex SA 账号按账号区域分页读取发布方模型列表，只保留 gemini- 前缀模型。
func TestFetchUpstreamSupportedModelsVertexServiceAccountPaginatesAndKeepsGemini(t *testing.T) {
	t.Parallel()

	upstream := &httpUpstreamRecorder{responses: []*http.Response{
		vertexModelsJSONResponse(http.StatusOK, `{"publisherModels":[
			{"name":"publishers/google/models/gemini-3.8-flash"},
			{"name":"publishers/google/models/gemma4"},
			{"name":"publishers/google/models/veo-3.1-generate-001"}
		],"nextPageToken":"page-2"}`),
		vertexModelsJSONResponse(http.StatusOK, `{"publisherModels":[
			{"name":" publishers/google/models/gemini-2.5-flash "},
			{"name":"publishers/google/models/gemini-3.8-flash"},
			{"name":"gemini-without-publisher-prefix"},
			{"name":"publishers/anthropic/models/gemini-other-publisher"},
			{"name":"publishers/google/models/gemini-"}
		]}`),
	}}
	svc := vertexModelSyncTestService(upstream)

	models, err := svc.FetchUpstreamSupportedModels(context.Background(), vertexModelSyncTestAccount(map[string]any{
		"location": "europe-west4",
	}))
	require.NoError(t, err)
	require.Equal(t, []string{"gemini-2.5-flash", "gemini-3.8-flash"}, models)

	require.Len(t, upstream.requests, 2)
	require.Equal(t, "https://europe-west4-aiplatform.googleapis.com/v1beta1/publishers/google/models?pageSize=300", upstream.requests[0].URL.String())
	require.Equal(t, "https://europe-west4-aiplatform.googleapis.com/v1beta1/publishers/google/models?pageSize=300&pageToken=page-2", upstream.requests[1].URL.String())
	for _, req := range upstream.requests {
		require.Equal(t, http.MethodGet, req.Method)
		require.Equal(t, "Bearer ya29.vertex-token", req.Header.Get("Authorization"))
	}
}

func TestFetchUpstreamSupportedModelsVertexUpstreamErrorKeepsStatus(t *testing.T) {
	t.Parallel()

	upstream := &httpUpstreamRecorder{resp: vertexModelsJSONResponse(http.StatusForbidden, `{"error":{"code":403,"status":"PERMISSION_DENIED"}}`)}
	svc := vertexModelSyncTestService(upstream)

	_, err := svc.FetchUpstreamSupportedModels(context.Background(), vertexModelSyncTestAccount(map[string]any{"location": "global"}))
	var syncErr *UpstreamModelSyncError
	require.ErrorAs(t, err, &syncErr)
	require.Equal(t, UpstreamModelSyncErrorUpstream, syncErr.Kind)
	require.Equal(t, http.StatusForbidden, syncErr.StatusCode)
	require.Equal(t, "https://aiplatform.googleapis.com/v1beta1/publishers/google/models?pageSize=300", upstream.lastReq.URL.String())
}

func TestFetchUpstreamSupportedModelsVertexWithoutGeminiModelsFails(t *testing.T) {
	t.Parallel()

	upstream := &httpUpstreamRecorder{resp: vertexModelsJSONResponse(http.StatusOK, `{"publisherModels":[{"name":"publishers/google/models/gemma4"}]}`)}
	svc := vertexModelSyncTestService(upstream)

	_, err := svc.FetchUpstreamSupportedModels(context.Background(), vertexModelSyncTestAccount(nil))
	var syncErr *UpstreamModelSyncError
	require.ErrorAs(t, err, &syncErr)
	require.Equal(t, UpstreamModelSyncErrorUpstream, syncErr.Kind)
	require.Equal(t, "Upstream returned no supported models", syncErr.SafeMessage())
}

func TestFetchUpstreamSupportedModelsVertexStopsEndlessPagination(t *testing.T) {
	t.Parallel()

	responses := make([]*http.Response, 0, vertexPublisherModelsMaxPages)
	for i := 0; i < vertexPublisherModelsMaxPages; i++ {
		responses = append(responses, vertexModelsJSONResponse(http.StatusOK, `{"publisherModels":[{"name":"publishers/google/models/gemini-3.8-flash"}],"nextPageToken":"again"}`))
	}
	upstream := &httpUpstreamRecorder{responses: responses}
	svc := vertexModelSyncTestService(upstream)

	_, err := svc.FetchUpstreamSupportedModels(context.Background(), vertexModelSyncTestAccount(nil))
	var syncErr *UpstreamModelSyncError
	require.ErrorAs(t, err, &syncErr)
	require.Equal(t, "Upstream model list pagination did not finish", syncErr.SafeMessage())
	require.Len(t, upstream.requests, vertexPublisherModelsMaxPages)
}

func TestFetchUpstreamSupportedModelsVertexAPIKeyUnsupported(t *testing.T) {
	t.Parallel()

	upstream := &httpUpstreamRecorder{}
	svc := vertexModelSyncTestService(upstream)
	account := &Account{
		ID:       174,
		Platform: PlatformGemini,
		Type:     AccountTypeServiceAccount,
		Credentials: map[string]any{
			"auth_mode": "apikey",
			"api_key":   "AQ.test-key",
		},
	}

	_, err := svc.FetchUpstreamSupportedModels(context.Background(), account)
	var syncErr *UpstreamModelSyncError
	require.ErrorAs(t, err, &syncErr)
	require.Equal(t, UpstreamModelSyncErrorUnsupported, syncErr.Kind)
	require.Empty(t, upstream.requests)
}

func TestFetchUpstreamSupportedModelsVertexInvalidCredentialsIsConfigError(t *testing.T) {
	t.Parallel()

	cache := &vertexModelsTokenCacheStub{token: "ya29.vertex-token"}
	upstream := &httpUpstreamRecorder{}
	svc := &AccountTestService{
		httpUpstream:        upstream,
		cfg:                 upstreamModelSyncTestConfig(),
		geminiTokenProvider: NewGeminiTokenProvider(nil, cache, nil),
	}
	account := &Account{
		ID:          175,
		Platform:    PlatformGemini,
		Type:        AccountTypeServiceAccount,
		Credentials: map[string]any{"api_key": "not-a-service-account"},
	}

	_, err := svc.FetchUpstreamSupportedModels(context.Background(), account)
	var syncErr *UpstreamModelSyncError
	require.ErrorAs(t, err, &syncErr)
	require.Equal(t, UpstreamModelSyncErrorConfiguration, syncErr.Kind)
	require.Equal(t, "Invalid Vertex service account credentials", syncErr.SafeMessage())
	require.Zero(t, cache.calls)
	require.Empty(t, upstream.requests)
}

func TestFetchUpstreamSupportedModelsVertexInvalidLocationSkipsTokenExchange(t *testing.T) {
	t.Parallel()

	cache := &vertexModelsTokenCacheStub{token: "ya29.vertex-token"}
	upstream := &httpUpstreamRecorder{}
	svc := &AccountTestService{
		httpUpstream:        upstream,
		cfg:                 upstreamModelSyncTestConfig(),
		geminiTokenProvider: NewGeminiTokenProvider(nil, cache, nil),
	}

	_, err := svc.FetchUpstreamSupportedModels(context.Background(), vertexModelSyncTestAccount(map[string]any{
		"location": "us-central1.evil.com",
	}))
	var syncErr *UpstreamModelSyncError
	require.ErrorAs(t, err, &syncErr)
	require.Equal(t, UpstreamModelSyncErrorConfiguration, syncErr.Kind)
	require.Equal(t, "Invalid Vertex location", syncErr.SafeMessage())
	require.Zero(t, cache.calls)
	require.Empty(t, upstream.requests)
}

// Scenario: 翻页途中的 404 不能被当成列表端点不可用而回退到配置模型。
func TestSyncUpstreamModelCatalogVertexMidPagination404DoesNotFallback(t *testing.T) {
	t.Parallel()

	upstream := &httpUpstreamRecorder{responses: []*http.Response{
		vertexModelsJSONResponse(http.StatusOK, `{"publisherModels":[{"name":"publishers/google/models/gemini-a"}],"nextPageToken":"page-2"}`),
		vertexModelsJSONResponse(http.StatusNotFound, `{"error":{"code":404}}`),
	}}
	svc := vertexModelSyncTestService(upstream)

	_, err := svc.SyncUpstreamModelCatalog(context.Background(), vertexModelSyncTestAccount(map[string]any{
		"model_mapping": map[string]any{"x": "gemini-mapped"},
	}))
	var syncErr *UpstreamModelSyncError
	require.ErrorAs(t, err, &syncErr)
	require.Equal(t, UpstreamModelSyncErrorUpstream, syncErr.Kind)
	require.Zero(t, syncErr.StatusCode)
	require.Len(t, upstream.requests, 2)
}

func TestFetchUpstreamSupportedModelsVertexFirstPage404KeepsStatus(t *testing.T) {
	t.Parallel()

	upstream := &httpUpstreamRecorder{resp: vertexModelsJSONResponse(http.StatusNotFound, `{"error":{"code":404}}`)}
	svc := vertexModelSyncTestService(upstream)

	_, err := svc.FetchUpstreamSupportedModels(context.Background(), vertexModelSyncTestAccount(nil))
	require.True(t, upstreamModelListEndpointUnsupported(err))
}
