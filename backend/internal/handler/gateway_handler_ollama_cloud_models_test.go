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

// ollama_cloud 没有静态默认模型列表：defaultModelIDsForPlatform 不得回落
// Claude 列表（default 分支），/models 在账号侧清单缺失时返回空列表。
func TestDefaultModelIDsForPlatform_OllamaCloudReturnsEmpty(t *testing.T) {
	require.Empty(t, defaultModelIDsForPlatform(service.PlatformOllamaCloud))
	require.NotEmpty(t, defaultModelIDsForPlatform(service.PlatformAnthropic))
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

// 账号侧清单缺失（deny-all）时 /models 必须返回空，而不是回落 Claude 列表。
func TestGatewayModels_OllamaCloudGroupWithoutListReturnsEmpty(t *testing.T) {
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
	require.Empty(t, got.Data, "ollama_cloud 不得回落 Claude 默认模型列表")
	require.NotContains(t, modelIDsForTest(got.Data), "claude-sonnet-4-6")
}
