package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	middleware "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestCompositeModelOverridesApplyToAdminPreviewAndGateway(t *testing.T) {
	h := newGatewayModelsHandlerForTest(&gatewayModelsAccountRepoStub{byGroup: map[int64][]service.Account{7: {
		{ID: 1, Platform: service.PlatformMiniMax, Status: service.StatusActive, Schedulable: true, Credentials: map[string]any{"model_mapping": map[string]any{"MiniMax-M3": "MiniMax-M3"}}},
		{ID: 2, Platform: service.PlatformKimi, Status: service.StatusActive, Schedulable: true, Credentials: map[string]any{"model_mapping": map[string]any{"k3": "k3"}}},
	}}})
	group := &service.Group{ID: 7, Platform: service.PlatformComposite, CodexModelsManifestConfig: service.GroupCodexModelsManifestConfig{ModelOverrides: map[string]map[string]json.RawMessage{
		"MiniMax-M3": {"description": json.RawMessage(`"Composite override"`), "context_window": json.RawMessage(`1000000`), "max_context_window": json.RawMessage(`1000000`)},
	}}}
	preview, err := h.GroupCodexModelCatalog(context.Background(), group)
	require.NoError(t, err)
	require.Contains(t, string(preview), `"description":"Composite override"`)
	require.Contains(t, string(preview), `"slug":"k3"`)
	perform := func(etag string) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(rec)
		c.Request = httptest.NewRequest(http.MethodGet, "/v1/models?client_version=0.155.1", nil)
		c.Request.Header.Set("If-None-Match", etag)
		c.Set(string(middleware.ContextKeyAPIKey), &service.APIKey{Group: group})
		h.CodexModels(c)
		return rec
	}
	first := perform("")
	require.Equal(t, http.StatusOK, first.Code)
	require.JSONEq(t, string(preview), first.Body.String())
	require.Equal(t, http.StatusNotModified, perform(first.Header().Get("ETag")).Code)
	group.CodexModelsManifestConfig.ModelOverrides["MiniMax-M3"]["description"] = json.RawMessage(`"updated"`)
	require.Equal(t, http.StatusOK, perform(first.Header().Get("ETag")).Code)
}

func TestPinnedCodexModelOverridesChangeETagWithoutPollutingCache(t *testing.T) {
	upstream := &codexModelsPinnedHTTPUpstream{bodies: map[int64]string{1: `{"models":[{"slug":"MiniMax-M3","description":"upstream","future_field":{"keep":true}}],"data":[{"id":"MiniMax-M3","description":"upstream"}]}`}}
	h := newPinnedCodexTestHandler([]service.Account{newPinnedCodexAccount(1, service.StatusActive, true, false)}, upstream, 3)
	group := &service.Group{ID: 91, Platform: service.PlatformOpenAI, CodexModelsManifestConfig: service.GroupCodexModelsManifestConfig{Enabled: true, AccountIDs: []int64{1}}}
	first := performPinnedCodexModelsRequest(t, h, group, "")
	require.Equal(t, http.StatusOK, first.Code, first.Body.String())
	copy := *group
	copy.CodexModelsManifestConfig.ModelOverrides = map[string]map[string]json.RawMessage{
		"MiniMax-M3": {"description": json.RawMessage(`"manual"`), "context_window": json.RawMessage(`1000000`), "max_context_window": json.RawMessage(`1000000`)},
		"new-id":     {"description": json.RawMessage(`"not allowed"`)},
	}
	updated := performPinnedCodexModelsRequest(t, h, &copy, first.Header().Get("ETag"))
	require.Equal(t, http.StatusOK, updated.Code, updated.Body.String())
	require.Contains(t, updated.Body.String(), `"description":"manual"`)
	require.Contains(t, updated.Body.String(), `"future_field":{"keep":true}`)
	require.NotContains(t, updated.Body.String(), `new-id`)
	require.NotEqual(t, first.Header().Get("ETag"), updated.Header().Get("ETag"))
	cached := performPinnedCodexModelsRequest(t, h, &copy, updated.Header().Get("ETag"))
	require.Equal(t, http.StatusNotModified, cached.Code)
	otherGroup := performPinnedCodexModelsRequest(t, h, group, "")
	require.Equal(t, first.Body.String(), otherGroup.Body.String())
	ordinary := performOrdinaryPinnedModelsRequest(t, h, &copy, "/v1/models", "")
	require.Equal(t, http.StatusOK, ordinary.Code)
	require.NotContains(t, ordinary.Body.String(), `manual`)
	require.Equal(t, []int64{1, 1}, upstream.accountIDs())
}
