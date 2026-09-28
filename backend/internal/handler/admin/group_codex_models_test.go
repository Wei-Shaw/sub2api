package admin

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestGroupCodexPreviewUsesDraftAllowlistWithoutSaving(t *testing.T) {
	gin.SetMode(gin.TestMode)
	group := &service.Group{ID: 1, Platform: service.PlatformComposite, ModelAllowlist: service.GroupModelAllowlist{Enabled: true, Models: []string{"saved-model"}}}
	h := NewGroupHandler(&modelConfigAdminStub{group: group}, nil, nil)
	h.SetCodexModelCatalog(func(_ context.Context, g *service.Group) ([]byte, error) {
		return service.BuildCodexModelsManifest(g.ModelAllowlist.FilterForListing([]string{"saved-model", "new-model", "new-fast"}))
	})
	h.SetCodexModelUpstream(func(_ context.Context, g *service.Group, model string) (map[string]json.RawMessage, error) {
		require.Equal(t, []string{"new-model"}, g.ModelAllowlist.Models)
		require.Equal(t, "new-model", model)
		return map[string]json.RawMessage{"description": json.RawMessage(`"upstream metadata"`)}, nil
	})
	r := gin.New()
	r.POST("/groups/:id/preview", h.PreviewCodexModelConfig)
	for _, tc := range []struct {
		body     string
		slugs    []string
		upstream bool
	}{
		{`{"model_allowlist":{"enabled":true,"models":["new-model"]}}`, []string{"new-model"}, false},
		{`{"model_allowlist":{"enabled":true,"models":["new-fast","new-*"]}}`, []string{"new-fast", "new-model"}, false},
		{`{"model_allowlist":{"enabled":true,"models":[]}}`, []string{}, false},
		{`{"model_allowlist":{"enabled":false,"models":[]}}`, []string{"saved-model", "new-model", "new-fast"}, false},
		{`{"model_allowlist":{"enabled":true,"models":["new-model"]},"model":"new-model"}`, nil, true},
	} {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/groups/1/preview", strings.NewReader(tc.body))
		req.Header.Set("Content-Type", "application/json")
		r.ServeHTTP(w, req)
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())
		var response struct {
			Data struct {
				Models []struct {
					Slug string `json:"slug"`
				} `json:"models"`
				Fields map[string]json.RawMessage `json:"fields"`
			} `json:"data"`
		}
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))
		if tc.upstream {
			require.Contains(t, response.Data.Fields, "description")
		} else {
			slugs := make([]string, 0, len(response.Data.Models))
			for _, m := range response.Data.Models {
				slugs = append(slugs, m.Slug)
			}
			require.Equal(t, tc.slugs, slugs)
		}
		require.Equal(t, []string{"saved-model"}, group.ModelAllowlist.Models, "preview must not mutate persisted or cached group state")
	}
}

type modelConfigAdminStub struct {
	service.AdminService
	group *service.Group
}

func (s *modelConfigAdminStub) GetGroup(context.Context, int64) (*service.Group, error) {
	return s.group, nil
}

func TestGroupModelOverrideAdmission(t *testing.T) {
	group := &service.Group{ID: 1, Platform: service.PlatformMiniMax}
	h := NewGroupHandler(&modelConfigAdminStub{group: group}, nil, nil)
	calls := 0
	h.SetCodexModelCatalog(func(_ context.Context, g *service.Group) ([]byte, error) {
		calls++
		require.Empty(t, g.CodexModelsManifestConfig.ModelOverrides)
		return service.BuildCodexModelsManifest([]string{"MiniMax-M3"})
	})
	fields := map[string]json.RawMessage{"description": json.RawMessage(`"custom"`)}
	req := &UpdateGroupRequest{CodexModelsManifestConfig: &service.GroupCodexModelsManifestConfig{ModelOverrides: map[string]map[string]json.RawMessage{"unavailable": fields}}}
	require.ErrorContains(t, h.validateModelOverrideUpdate(context.Background(), 1, req), "not available")
	req.CodexModelsManifestConfig.ModelOverrides = map[string]map[string]json.RawMessage{"MiniMax-M3": fields}
	require.NoError(t, h.validateModelOverrideUpdate(context.Background(), 1, req))
	require.Equal(t, 2, calls)
	// 原有配置可以保留或移除，无须不可用的上游恢复。
	group.CodexModelsManifestConfig.ModelOverrides = req.CodexModelsManifestConfig.ModelOverrides
	require.NoError(t, h.validateModelOverrideUpdate(context.Background(), 1, req))
	req.CodexModelsManifestConfig.ModelOverrides = nil
	require.NoError(t, h.validateModelOverrideUpdate(context.Background(), 1, req))
	require.Equal(t, 2, calls)
}
