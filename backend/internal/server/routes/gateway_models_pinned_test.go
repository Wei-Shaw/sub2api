package routes

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/handler"
	servermiddleware "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type pinnedModelsRoutesRepository struct {
	service.AccountRepository
	account service.Account
}

func (r *pinnedModelsRoutesRepository) ListByGroup(context.Context, int64) ([]service.Account, error) {
	return []service.Account{r.account}, nil
}

func (r *pinnedModelsRoutesRepository) ListSchedulableByGroupID(context.Context, int64) ([]service.Account, error) {
	return []service.Account{r.account}, nil
}

func (r *pinnedModelsRoutesRepository) ListModelAvailabilityCandidates(context.Context, *int64, []string, bool) ([]service.Account, error) {
	return []service.Account{r.account}, nil
}

type pinnedModelsRoutesUpstream struct {
	service.HTTPUpstream
	ordinaryCalls atomic.Int32
	codexCalls    atomic.Int32
}

func (u *pinnedModelsRoutesUpstream) Do(req *http.Request, _ string, _ int64, _ int) (*http.Response, error) {
	body := `{"data":[{"id":"ordinary-upstream-model"}]}`
	if req.URL.Query().Has("client_version") {
		u.codexCalls.Add(1)
		body = `{"models":[{"slug":"gpt-5.5"}]}`
	} else {
		u.ordinaryCalls.Add(1)
	}
	return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body))}, nil
}

func TestGatewayRoutesPinnedModelsDispatchesOrdinaryAndCodexRequests(t *testing.T) {
	gin.SetMode(gin.TestMode)
	repo := &pinnedModelsRoutesRepository{account: service.Account{
		ID: 7, Platform: service.PlatformOpenAI, Type: service.AccountTypeAPIKey,
		Status: service.StatusActive, Schedulable: true,
		Credentials: map[string]any{"api_key": "test-models-key", "base_url": "https://models.example/v1"},
	}}
	upstream := &pinnedModelsRoutesUpstream{}
	cfg := &config.Config{RunMode: config.RunModeSimple}
	s := service.NewOpenAIGatewayService(repo, nil, nil, nil, nil, nil, nil, cfg,
		nil, nil, nil, nil, nil, upstream, nil, nil, nil, nil, nil, nil, nil, nil)
	h := &handler.Handlers{
		Gateway:       handler.NewGatewayHandler(nil, s, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, cfg, nil),
		OpenAIGateway: handler.NewOpenAIGatewayHandler(s, nil, nil, nil, nil, nil, nil, nil, cfg),
		AsyncImage:    handler.NewAsyncImageHandler(nil, nil),
	}
	group := &service.Group{ID: 1, Platform: service.PlatformOpenAI,
		CodexModelsManifestConfig: service.GroupCodexModelsManifestConfig{Enabled: true, AccountIDs: []int64{7}}}
	router := gin.New()
	RegisterGatewayRoutes(router, h, servermiddleware.APIKeyAuthMiddleware(func(c *gin.Context) {
		c.Set(string(servermiddleware.ContextKeyAPIKey), &service.APIKey{GroupID: &group.ID, Group: group})
		c.Next()
	}), nil, nil, nil, nil, nil, cfg)
	for _, path := range []string{"/v1/models", "/models", "/v1/models?client_version=", "/models?client_version=", "/v1/models?format=unknown"} {
		w := httptest.NewRecorder()
		router.ServeHTTP(w, httptest.NewRequest(http.MethodGet, path, nil))
		require.Equal(t, http.StatusOK, w.Code, "%s: %s", path, w.Body.String())
		var response struct {
			Object string `json:"object"`
			Data   []struct {
				ID string `json:"id"`
			} `json:"data"`
		}
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))
		require.Equal(t, "list", response.Object)
		require.Len(t, response.Data, 1)
		require.Equal(t, "ordinary-upstream-model", response.Data[0].ID)
	}
	for _, path := range []string{"/v1/models?format=codex", "/models?format=codex", "/v1/models?format=codex&client_version=", "/v1/models?client_version=" + service.CodexCanonicalClientVersion(), "/models?client_version=" + service.CodexCanonicalClientVersion(), "/backend-api/codex/models"} {
		w := httptest.NewRecorder()
		router.ServeHTTP(w, httptest.NewRequest(http.MethodGet, path, nil))
		require.Equal(t, http.StatusOK, w.Code, "%s: %s", path, w.Body.String())
		require.Contains(t, w.Body.String(), `"slug":"gpt-5.5"`)
		require.NotContains(t, w.Body.String(), `"data"`)
	}
	require.EqualValues(t, 1, upstream.ordinaryCalls.Load())
	require.EqualValues(t, 1, upstream.codexCalls.Load())
}

type automaticCatalogRoutesUpstream struct {
	service.HTTPUpstream
	versions sync.Map
}

func (u *automaticCatalogRoutesUpstream) Do(req *http.Request, _ string, _ int64, _ int) (*http.Response, error) {
	version := req.URL.Query().Get("client_version")
	u.versions.Store(version, req.Header.Get("Version"))
	body := `{"models":[{"slug":"existing-model","input_modalities":["text"]}]}`
	status := http.StatusOK
	switch version {
	case "0.201.0":
		body = `{"models":[{"slug":"existing-model","input_modalities":["text"]},{"slug":"future-model","input_modalities":["text","image"],"supported_reasoning_levels":[{"effort":"high"}],"model_messages":{"instructions_template":"upstream instructions"},"future_capability":{"enabled":true}}]}`
	case "0.202.0":
		status = http.StatusBadGateway
		body = `{"error":{"message":"catalog unavailable"}}`
	}
	return &http.Response{StatusCode: status, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body))}, nil
}

func TestGatewayRoutesAutomaticCodexCatalogTracksCanonicalIdentity(t *testing.T) {
	gin.SetMode(gin.TestMode)
	var version atomic.Value
	version.Store("0.200.0")
	service.SetCodexCanonicalUserAgentResolver(func() string {
		return "codex_cli_rs/" + version.Load().(string) + " (Linux; x86_64) test"
	})
	t.Cleanup(func() { service.SetCodexCanonicalUserAgentResolver(nil) })

	repo := &pinnedModelsRoutesRepository{account: service.Account{
		ID: 7, Platform: service.PlatformOpenAI, Type: service.AccountTypeAPIKey,
		Status: service.StatusActive, Schedulable: true,
		Credentials: map[string]any{"api_key": "test-models-key", "base_url": "https://models.example/v1"},
	}}
	upstream := &automaticCatalogRoutesUpstream{}
	cfg := &config.Config{RunMode: config.RunModeSimple}
	s := service.NewOpenAIGatewayService(repo, nil, nil, nil, nil, nil, nil, cfg,
		nil, nil, nil, nil, nil, upstream, nil, nil, nil, nil, nil, nil, nil, nil)
	h := &handler.Handlers{
		Gateway:       handler.NewGatewayHandler(nil, s, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, cfg, nil),
		OpenAIGateway: handler.NewOpenAIGatewayHandler(s, nil, nil, nil, nil, nil, nil, nil, cfg),
		AsyncImage:    handler.NewAsyncImageHandler(nil, nil),
	}
	group := &service.Group{ID: 1, Platform: service.PlatformOpenAI,
		CodexModelsManifestConfig: service.GroupCodexModelsManifestConfig{Enabled: true, AccountIDs: []int64{7}}}
	router := gin.New()
	RegisterGatewayRoutes(router, h, servermiddleware.APIKeyAuthMiddleware(func(c *gin.Context) {
		if c.GetHeader("Authorization") != "Bearer client-key" {
			c.AbortWithStatus(http.StatusUnauthorized)
			return
		}
		c.Set(string(servermiddleware.ContextKeyAPIKey), &service.APIKey{GroupID: &group.ID, Group: group})
		c.Next()
	}), nil, nil, nil, nil, nil, cfg)
	request := func(path, key string) *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, path, nil)
		req.Header.Set("Authorization", key)
		router.ServeHTTP(w, req)
		return w
	}
	const catalogPath = "/v1/models?format=codex"
	require.Equal(t, http.StatusUnauthorized, request(catalogPath, "").Code)
	initial := request(catalogPath, "Bearer client-key")
	require.Equal(t, http.StatusOK, initial.Code, initial.Body.String())
	var initialManifest struct {
		Models []map[string]json.RawMessage `json:"models"`
	}
	require.NoError(t, json.Unmarshal(initial.Body.Bytes(), &initialManifest))
	require.Len(t, initialManifest.Models, 1)
	require.Equal(t, `"existing-model"`, string(initialManifest.Models[0]["slug"]))

	version.Store("0.201.0")
	updated := request(catalogPath, "Bearer client-key")
	require.Equal(t, http.StatusOK, updated.Code, updated.Body.String())
	var manifest struct {
		Models []map[string]json.RawMessage `json:"models"`
	}
	require.NoError(t, json.Unmarshal(updated.Body.Bytes(), &manifest))
	require.Len(t, manifest.Models, 2)
	require.JSONEq(t, `{"enabled":true}`, string(manifest.Models[1]["future_capability"]))
	var messages map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(manifest.Models[1]["model_messages"], &messages))
	require.Equal(t, `"upstream instructions"`, string(messages["instructions_template"]))
	require.JSONEq(t, `["text","image"]`, string(manifest.Models[1]["input_modalities"]))
	require.JSONEq(t, `[{"effort":"high"}]`, string(manifest.Models[1]["supported_reasoning_levels"]))
	require.NotEqual(t, initial.Header().Get("ETag"), updated.Header().Get("ETag"))
	legacy := request("/v1/models?client_version=0.200.0", "Bearer client-key")
	require.Equal(t, http.StatusOK, legacy.Code)
	require.JSONEq(t, initial.Body.String(), legacy.Body.String())
	for _, v := range []string{"0.200.0", "0.201.0"} {
		header, ok := upstream.versions.Load(v)
		require.True(t, ok)
		require.Equal(t, v, header)
	}

	version.Store("0.202.0")
	failed := request(catalogPath, "Bearer client-key")
	require.Equal(t, http.StatusBadGateway, failed.Code, failed.Body.String())
	require.NotContains(t, failed.Body.String(), `"models"`)
}

func TestGatewayRoutesRetrievePinnedModel(t *testing.T) {
	gin.SetMode(gin.TestMode)
	repo := &pinnedModelsRoutesRepository{account: service.Account{
		ID: 7, Platform: service.PlatformOpenAI, Type: service.AccountTypeAPIKey,
		Status: service.StatusActive, Schedulable: true,
		Credentials: map[string]any{"api_key": "test-models-key", "base_url": "https://models.example/v1"},
	}}
	upstream := &pinnedModelsRoutesUpstream{}
	cfg := &config.Config{RunMode: config.RunModeSimple}
	s := service.NewOpenAIGatewayService(repo, nil, nil, nil, nil, nil, nil, cfg,
		nil, nil, nil, nil, nil, upstream, nil, nil, nil, nil, nil, nil, nil, nil)
	h := &handler.Handlers{
		Gateway:       handler.NewGatewayHandler(nil, s, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, cfg, nil),
		OpenAIGateway: handler.NewOpenAIGatewayHandler(s, nil, nil, nil, nil, nil, nil, nil, cfg),
		AsyncImage:    handler.NewAsyncImageHandler(nil, nil),
	}
	group := &service.Group{ID: 1, Platform: service.PlatformOpenAI,
		CodexModelsManifestConfig: service.GroupCodexModelsManifestConfig{Enabled: true, AccountIDs: []int64{7}}}
	router := gin.New()
	RegisterGatewayRoutes(router, h, servermiddleware.APIKeyAuthMiddleware(func(c *gin.Context) {
		if c.GetHeader("Authorization") != "Bearer client-key" {
			c.AbortWithStatus(http.StatusUnauthorized)
			return
		}
		c.Set(string(servermiddleware.ContextKeyAPIKey), &service.APIKey{GroupID: &group.ID, Group: group})
		c.Next()
	}), nil, nil, nil, nil, nil, cfg)
	request := func(path, key, etag string) *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, path, nil)
		req.Header.Set("Authorization", key)
		req.Header.Set("If-None-Match", etag)
		router.ServeHTTP(w, req)
		return w
	}
	list := request("/v1/models", "Bearer client-key", "")
	require.Equal(t, http.StatusOK, list.Code)
	var catalog struct {
		Data []json.RawMessage `json:"data"`
	}
	require.NoError(t, json.Unmarshal(list.Body.Bytes(), &catalog))
	require.Len(t, catalog.Data, 1)
	for _, base := range []string{"/v1/models/", "/models/"} {
		path := base + "ordinary-upstream-model?client_version=ignored"
		require.Equal(t, http.StatusUnauthorized, request(path, "", "").Code)
		got := request(path, "Bearer client-key", list.Header().Get("ETag"))
		require.Equal(t, http.StatusOK, got.Code, got.Body.String())
		require.JSONEq(t, string(catalog.Data[0]), got.Body.String())
		require.Empty(t, got.Header().Get("ETag"), "a collection validator cannot validate one model")
		missing := request(base+"unknown-model", "Bearer client-key", "")
		require.Equal(t, http.StatusNotFound, missing.Code)
		require.Contains(t, missing.Body.String(), `"error"`)
	}
	require.Zero(t, upstream.codexCalls.Load(), "retrieve never dispatches a Codex manifest")
}
