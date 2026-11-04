package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/pkg/minimax"
	middleware2 "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type miniMaxModelsGroupRepo struct {
	service.GroupRepository
	group *service.Group
}

func (r *miniMaxModelsGroupRepo) GetByID(context.Context, int64) (*service.Group, error) {
	return r.group, nil
}

func TestMiniMaxMediaCatalogKeepsOrdinaryAndCodexTextModels(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, codexFirst := range []bool{false, true} {
		name := "ordinary_first"
		if codexFirst {
			name = "codex_first"
		}
		t.Run(name, func(t *testing.T) {
			group := &service.Group{ID: 1, Platform: service.PlatformMiniMax}
			accounts := &gatewayModelsAccountRepoStub{byGroup: map[int64][]service.Account{1: {{
				Platform: service.PlatformMiniMax, Type: service.AccountTypeAPIKey,
				Credentials: map[string]any{"api_key": "test"},
				Extra:       map[string]any{"minimax_media_enabled": true, "minimax_media_base_url": "https://api.minimax.io"},
			}}}}
			h := &GatewayHandler{gatewayService: service.NewGatewayService(accounts, &miniMaxModelsGroupRepo{group: group},
				nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil,
				nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)}
			ordinary := func() []string {
				recorder := httptest.NewRecorder()
				c, _ := gin.CreateTestContext(recorder)
				c.Request = httptest.NewRequest(http.MethodGet, "/v1/models", nil)
				c.Set(string(middleware2.ContextKeyAPIKey), &service.APIKey{GroupID: &group.ID, Group: group})
				h.Models(c)
				require.Equal(t, http.StatusOK, recorder.Code)
				var body gatewayModelsResponseForTest
				require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &body))
				ids := make([]string, 0, len(body.Data))
				for _, model := range body.Data {
					ids = append(ids, model.ID)
				}
				return ids
			}
			var ordinaryModels, codexModels []string
			if codexFirst {
				codexModels = h.codexModelIDsForGroup(context.Background(), group, "")
				ordinaryModels = ordinary()
			} else {
				ordinaryModels = ordinary()
				codexModels = h.codexModelIDsForGroup(context.Background(), group, "")
			}
			for _, model := range defaultModelIDsForPlatform(service.PlatformMiniMax) {
				require.Contains(t, ordinaryModels, model)
				require.NotContains(t, codexModels, model)
			}
			for _, model := range defaultCodexModelIDsForPlatform(service.PlatformMiniMax) {
				require.Contains(t, codexModels, model)
			}
			require.Contains(t, ordinaryModels, minimax.SpeechHD)
			require.Contains(t, codexModels, minimax.SpeechHD)
		})
	}
}
