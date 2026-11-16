//go:build unit

package service

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/gemini"
	"github.com/stretchr/testify/require"
)

func newGeminiModelsEndpointsTestService(accounts []Account, upstream HTTPUpstream) *GeminiMessagesCompatService {
	repo := &mockAccountRepoForGemini{listByGroupFunc: func(context.Context, int64, []string) ([]Account, error) {
		return accounts, nil
	}}
	return &GeminiMessagesCompatService{
		accountRepo:   repo,
		httpUpstream:  upstream,
		cfg:           &config.Config{},
		tokenProvider: NewGeminiTokenProvider(nil, &vertexModelsTokenCacheStub{token: "ya29.vertex-token"}, nil),
	}
}

func decodeGeminiModelNames(t *testing.T, body []byte) []string {
	t.Helper()
	var got gemini.ModelsListResponse
	require.NoError(t, json.Unmarshal(body, &got))
	names := make([]string, 0, len(got.Models))
	for _, model := range got.Models {
		require.Contains(t, model.SupportedGenerationMethods, "generateContent")
		names = append(names, model.Name)
	}
	return names
}

func TestSelectAccountForModelsEndpoints_VertexServiceAccountOnlyGroup(t *testing.T) {
	groupID := int64(8)
	vertex := *vertexModelSyncTestAccount(nil)
	vertex.ID = 18
	svc := newGeminiModelsEndpointsTestService([]Account{vertex}, nil)

	account, err := svc.SelectAccountForModelsEndpoints(context.Background(), &groupID)
	require.NoError(t, err)
	require.Equal(t, int64(18), account.ID)
}

func TestModelsEndpointAccountRank_Order(t *testing.T) {
	ordered := []*Account{
		{Platform: PlatformGemini, Type: AccountTypeAPIKey, Credentials: map[string]any{"api_key": "k"}},
		{Platform: PlatformGemini, Type: AccountTypeOAuth, Credentials: map[string]any{}},
		{Platform: PlatformGemini, Type: AccountTypeOAuth, Credentials: map[string]any{"project_id": "p", "oauth_type": "ai_studio"}},
		{Platform: PlatformGemini, Type: AccountTypeOAuth, Credentials: map[string]any{"project_id": "p", "oauth_type": "code_assist"}},
		{Platform: PlatformGemini, Type: AccountTypeUpstream, Credentials: map[string]any{"api_key": "k"}},
		vertexModelSyncTestAccount(nil),
		{Platform: PlatformGemini, Type: AccountTypeServiceAccount, Credentials: map[string]any{"auth_mode": "apikey", "api_key": "AQ.k"}},
	}
	for i := 1; i < len(ordered); i++ {
		require.Less(t, modelsEndpointAccountRank(ordered[i-1]), modelsEndpointAccountRank(ordered[i]), "position %d", i)
	}
}

func TestSelectAccountForModelsEndpoints_PrefersAPIKeyOverVertex(t *testing.T) {
	groupID := int64(8)
	vertex := *vertexModelSyncTestAccount(nil)
	vertex.ID = 18
	apiKey := Account{ID: 19, Platform: PlatformGemini, Type: AccountTypeAPIKey, Priority: 9, Credentials: map[string]any{"api_key": "k"}}
	svc := newGeminiModelsEndpointsTestService([]Account{vertex, apiKey}, nil)

	account, err := svc.SelectAccountForModelsEndpoints(context.Background(), &groupID)
	require.NoError(t, err)
	require.Equal(t, int64(19), account.ID)
}

func TestForwardListModels_VertexServiceAccountReturnsAIStudioFormat(t *testing.T) {
	upstream := &httpUpstreamRecorder{responses: []*http.Response{
		vertexModelsJSONResponse(http.StatusOK, `{"publisherModels":[
			{"name":"publishers/google/models/gemini-3.8-flash"},
			{"name":"publishers/google/models/gemma4"}
		],"nextPageToken":"page-2"}`),
		vertexModelsJSONResponse(http.StatusOK, `{"publisherModels":[{"name":"publishers/google/models/gemini-2.5-flash"}]}`),
	}}
	svc := newGeminiModelsEndpointsTestService(nil, upstream)

	res, err := svc.ForwardListModels(context.Background(), vertexModelSyncTestAccount(map[string]any{"location": "europe-west4"}))
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, res.StatusCode)
	require.Equal(t, []string{"models/gemini-2.5-flash", "models/gemini-3.8-flash"}, decodeGeminiModelNames(t, res.Body))

	require.Len(t, upstream.requests, 2)
	require.Equal(t, "https://europe-west4-aiplatform.googleapis.com/v1beta1/publishers/google/models?pageSize=300", upstream.requests[0].URL.String())
	require.Equal(t, "https://europe-west4-aiplatform.googleapis.com/v1beta1/publishers/google/models?pageSize=300&pageToken=page-2", upstream.requests[1].URL.String())
	for _, req := range upstream.requests {
		require.Equal(t, "Bearer ya29.vertex-token", req.Header.Get("Authorization"))
	}
}

func TestForwardListModels_VertexUpstreamErrorPassesThrough(t *testing.T) {
	body := `{"error":{"code":403,"status":"PERMISSION_DENIED"}}`
	upstream := &httpUpstreamRecorder{resp: vertexModelsJSONResponse(http.StatusForbidden, body)}
	svc := newGeminiModelsEndpointsTestService(nil, upstream)

	res, err := svc.ForwardListModels(context.Background(), vertexModelSyncTestAccount(map[string]any{"location": "global"}))
	require.NoError(t, err)
	require.Equal(t, http.StatusForbidden, res.StatusCode)
	require.JSONEq(t, body, string(res.Body))
}

func TestForwardListModels_VertexAPIKeyReturnsBuiltinList(t *testing.T) {
	upstream := &httpUpstreamRecorder{}
	svc := newGeminiModelsEndpointsTestService(nil, upstream)
	account := &Account{ID: 174, Platform: PlatformGemini, Type: AccountTypeServiceAccount, Credentials: map[string]any{
		"auth_mode": "apikey",
		"api_key":   "AQ.test-key",
	}}

	res, err := svc.ForwardListModels(context.Background(), account)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, res.StatusCode)
	expected, err := json.Marshal(gemini.FallbackModelsList())
	require.NoError(t, err)
	require.JSONEq(t, string(expected), string(res.Body))
	require.Empty(t, upstream.requests)
}

func TestForwardListModels_APIKeyForwardsToBaseURL(t *testing.T) {
	upstream := &httpUpstreamRecorder{resp: vertexModelsJSONResponse(http.StatusOK, `{"models":[{"name":"models/gemini-native"}]}`)}
	svc := newGeminiModelsEndpointsTestService(nil, upstream)
	account := &Account{ID: 261, Platform: PlatformGemini, Type: AccountTypeAPIKey, Credentials: map[string]any{
		"api_key":  "sk-pool",
		"base_url": "https://relay.example.com",
	}}

	res, err := svc.ForwardListModels(context.Background(), account)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, res.StatusCode)
	require.JSONEq(t, `{"models":[{"name":"models/gemini-native"}]}`, string(res.Body))
	require.Equal(t, "https://relay.example.com/v1beta/models", upstream.lastReq.URL.String())
	require.Equal(t, "sk-pool", upstream.lastReq.Header.Get("x-goog-api-key"))
}

func TestForwardGetModel_VertexServiceAccountLooksUpModelList(t *testing.T) {
	listBody := `{"publisherModels":[{"name":"publishers/google/models/gemini-3.8-flash"}]}`

	t.Run("listed model", func(t *testing.T) {
		upstream := &httpUpstreamRecorder{resp: vertexModelsJSONResponse(http.StatusOK, listBody)}
		svc := newGeminiModelsEndpointsTestService(nil, upstream)

		res, err := svc.ForwardGetModel(context.Background(), vertexModelSyncTestAccount(nil), "gemini-3.8-flash")
		require.NoError(t, err)
		require.Equal(t, http.StatusOK, res.StatusCode)
		var model gemini.Model
		require.NoError(t, json.Unmarshal(res.Body, &model))
		require.Equal(t, "models/gemini-3.8-flash", model.Name)
	})

	t.Run("unlisted model", func(t *testing.T) {
		upstream := &httpUpstreamRecorder{resp: vertexModelsJSONResponse(http.StatusOK, listBody)}
		svc := newGeminiModelsEndpointsTestService(nil, upstream)

		res, err := svc.ForwardGetModel(context.Background(), vertexModelSyncTestAccount(nil), "gemini-unknown")
		require.NoError(t, err)
		require.Equal(t, http.StatusNotFound, res.StatusCode)
		require.JSONEq(t, `{"error":{"code":404,"message":"models/gemini-unknown is not found","status":"NOT_FOUND"}}`, string(res.Body))
	})
}

func TestForwardGetModel_VertexUsesModelLocation(t *testing.T) {
	upstream := &httpUpstreamRecorder{resp: vertexModelsJSONResponse(http.StatusOK, `{"publisherModels":[{"name":"publishers/google/models/gemini-3.8-flash"}]}`)}
	svc := newGeminiModelsEndpointsTestService(nil, upstream)
	account := vertexModelSyncTestAccount(map[string]any{
		"location":               "europe-west4",
		"vertex_model_locations": map[string]any{"gemini-3.8-flash": "global"},
	})

	res, err := svc.ForwardGetModel(context.Background(), account, "gemini-3.8-flash")
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, res.StatusCode)
	require.Equal(t, "https://aiplatform.googleapis.com/v1beta1/publishers/google/models?pageSize=300", upstream.lastReq.URL.String())
}

func TestForwardListModels_VertexMidPaginationErrorPassesThrough(t *testing.T) {
	body := `{"error":{"code":503,"status":"UNAVAILABLE"}}`
	upstream := &httpUpstreamRecorder{responses: []*http.Response{
		vertexModelsJSONResponse(http.StatusOK, `{"publisherModels":[{"name":"publishers/google/models/gemini-3.8-flash"}],"nextPageToken":"page-2"}`),
		vertexModelsJSONResponse(http.StatusServiceUnavailable, body),
	}}
	svc := newGeminiModelsEndpointsTestService(nil, upstream)

	res, err := svc.ForwardListModels(context.Background(), vertexModelSyncTestAccount(nil))
	require.NoError(t, err)
	require.Equal(t, http.StatusServiceUnavailable, res.StatusCode)
	require.JSONEq(t, body, string(res.Body))
	require.Len(t, upstream.requests, 2)
}

func TestForwardListModels_ResponseOverReadLimitFails(t *testing.T) {
	upstream := &httpUpstreamRecorder{resp: vertexModelsJSONResponse(http.StatusOK, `{"models":[{"name":"models/gemini-native"}]}`)}
	svc := newGeminiModelsEndpointsTestService(nil, upstream)
	svc.cfg.Gateway.ModelsListReadMaxBytes = 16
	account := &Account{ID: 261, Platform: PlatformGemini, Type: AccountTypeAPIKey, Credentials: map[string]any{"api_key": "sk-pool"}}

	_, err := svc.ForwardListModels(context.Background(), account)
	require.ErrorContains(t, err, "exceeds 16 bytes")
}
