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

func TestDevinDefaultModelListDispatch(t *testing.T) {
	want := []string{"swe-1-6", "swe-1-6-fast"}
	require.Equal(t, want, defaultModelIDsForPlatform(service.PlatformDevin))
	require.Equal(t, want, defaultCodexModelIDsForPlatform(service.PlatformDevin))
	for _, id := range want {
		require.Contains(t, defaultModelIDsForPlatform(service.PlatformComposite), id)
		require.Contains(t, defaultCodexModelIDsForPlatform(service.PlatformComposite), id)
	}
}

func TestDevinMessagesDispatchGate(t *testing.T) {
	group := &service.Group{Platform: service.PlatformDevin}
	key := &service.APIKey{Group: group}
	require.True(t, allowOpenAICompatibleMessagesDispatch(nil, key))
	require.Empty(t, resolveOpenAIMessagesDispatchMappedModel(nil, key, "claude-sonnet-4-5"))
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)
	c.Request = c.Request.WithContext(service.WithResolvedTargetPlatform(c.Request.Context(), service.PlatformDevin))
	group.Platform = service.PlatformComposite
	require.True(t, allowOpenAICompatibleMessagesDispatch(c, key))
	require.Empty(t, resolveOpenAIMessagesDispatchMappedModel(c, key, "claude-sonnet-4-5"))
}

func TestDevinPublicModelsFallback(t *testing.T) {
	h := newGatewayModelsHandlerForTest(&gatewayModelsAccountRepoStub{})
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	c.Set(string(middleware2.ContextKeyAPIKey), &service.APIKey{Group: &service.Group{ID: 1, Platform: service.PlatformDevin}})
	h.Models(c)
	require.Equal(t, http.StatusOK, rec.Code)
	var result gatewayModelsResponseForTest
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &result))
	require.Equal(t, service.DefaultDevinModelIDs(), modelIDsForTest(result.Data))
}
