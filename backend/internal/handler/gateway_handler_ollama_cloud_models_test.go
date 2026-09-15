package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	middleware2 "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

// ollama_cloud 的默认模型目录 = DefaultOllamaCloudModelIDs：defaultModelIDsForPlatform
// 不得回落 Claude 列表（default 分支），/models 在账号侧清单缺失时回落该目录。
func TestDefaultModelIDsForPlatform_OllamaCloudReturnsDefaultCatalog(t *testing.T) {
	require.Equal(t, service.DefaultOllamaCloudModelIDs(), defaultModelIDsForPlatform(service.PlatformOllamaCloud))
	require.NotEmpty(t, defaultModelIDsForPlatform(service.PlatformAnthropic))
	for _, id := range defaultModelIDsForPlatform(service.PlatformOllamaCloud) {
		require.NotContains(t, id, "claude")
	}
}

func TestGatewayModels_OllamaCloudGroupListsAllowedModels(t *testing.T) {
	gin.SetMode(gin.TestMode)

	groupID := int64(9201)
	h := newGatewayModelsHandlerForTest(
		&gatewayModelsAccountRepoStub{
			byGroup: map[int64][]service.Account{
				groupID: {
					{
						ID:          1,
						Platform:    service.PlatformOllamaCloud,
						Credentials: map[string]any{},
						Extra:       map[string]any{service.OllamaCloudAllowedModelsExtraKey: []any{"gpt-oss:120b"}},
					},
				},
			},
		},
	)

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	c.Set(string(middleware2.ContextKeyAPIKey), &service.APIKey{
		Group: &service.Group{ID: groupID, Platform: service.PlatformOllamaCloud},
	})

	h.Models(c)

	require.Equal(t, http.StatusOK, rec.Code)
	var got gatewayModelsResponseForTest
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &got))
	require.Equal(t, []string{"gpt-oss:120b"}, modelIDsForTest(got.Data))
}

// 账号侧清单缺失（deny-all）时 /models 回落 ollama 默认目录，而不是 Claude 列表。
func TestGatewayModels_OllamaCloudGroupWithoutListFallsBackToDefaultCatalog(t *testing.T) {
	gin.SetMode(gin.TestMode)

	groupID := int64(9202)
	h := newGatewayModelsHandlerForTest(
		&gatewayModelsAccountRepoStub{
			byGroup: map[int64][]service.Account{
				groupID: {
					{ID: 1, Platform: service.PlatformOllamaCloud, Credentials: map[string]any{}},
				},
			},
		},
	)

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	c.Set(string(middleware2.ContextKeyAPIKey), &service.APIKey{
		Group: &service.Group{ID: groupID, Platform: service.PlatformOllamaCloud},
	})

	h.Models(c)

	require.Equal(t, http.StatusOK, rec.Code)
	var got gatewayModelsResponseForTest
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &got))
	require.Equal(t, service.DefaultOllamaCloudModelIDs(), modelIDsForTest(got.Data),
		"ollama_cloud 不得回落 Claude 默认模型列表，应回落实测默认目录")
}

// ollama_cloud 分组的 composite 白名单来源不串入 Claude 目录之外的默认回落。
func TestGatewayModels_CompositeIncludesOllamaDefaultCatalog(t *testing.T) {
	gin.SetMode(gin.TestMode)

	groupID := int64(9203)
	h := newGatewayModelsHandlerForTest(
		&gatewayModelsAccountRepoStub{byGroup: map[int64][]service.Account{groupID: {}}},
	)

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	c.Set(string(middleware2.ContextKeyAPIKey), &service.APIKey{
		Group: &service.Group{ID: groupID, Platform: service.PlatformComposite},
	})

	h.Models(c)

	require.Equal(t, http.StatusOK, rec.Code)
	var got gatewayModelsResponseForTest
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &got))
	ids := modelIDsForTest(got.Data)
	idsSet := make(map[string]struct{}, len(ids))
	for _, id := range ids {
		idsSet[id] = struct{}{}
	}
	for _, id := range service.DefaultOllamaCloudModelIDs() {
		require.Contains(t, idsSet, id, "composite /v1/models 兜底须含 ollama 目录条目 %q", id)
	}
}
