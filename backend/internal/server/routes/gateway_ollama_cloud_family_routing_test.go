package routes

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	servermiddleware "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

// familyRouteHits 记录 stub terminal handler 各自被命中的次数。
type familyRouteHits struct {
	openAICompatible int
	grok             int
	generic          int
}

// newOllamaCloudFamilyRouter 按 RegisterGatewayRoutes 的文本端点装配方式搭最小
// 路由：APIKey 注入普通分组（非 composite、无池候选），composite 中间件原样接
// 入（对非 composite 分组直接放行），messages/responses/chat completions 走真实
// dispatchOpenAICompatibleGateway，count_tokens 走真实
// dispatchOpenAICompatibleCountTokens，terminal handler 用 stub 标记实际进入的
// 网关家族。
func newOllamaCloudFamilyRouter(t *testing.T, groupPlatform string) (*gin.Engine, *familyRouteHits) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(gin.HandlerFunc(servermiddleware.APIKeyAuthMiddleware(func(c *gin.Context) {
		groupID := int64(1)
		c.Set(string(servermiddleware.ContextKeyAPIKey), &service.APIKey{
			GroupID: &groupID,
			Group:   &service.Group{ID: groupID, Platform: groupPlatform},
		})
		c.Next()
	})))
	router.Use(compositeTargetPlatformMiddleware(service.NewCompositeRouteResolver(compositeRouteRepoStub{})))

	hits := &familyRouteHits{}
	openAICompatible := func(c *gin.Context) { hits.openAICompatible++; c.Status(http.StatusNoContent) }
	grokCompatible := func(c *gin.Context) { hits.grok++; c.Status(http.StatusNoContent) }
	generic := func(c *gin.Context) { hits.generic++; c.Status(http.StatusNoContent) }

	// 与 RegisterGatewayRoutes 相同的注册闭包：族分发交给生产 dispatcher。
	router.POST("/v1/messages", func(c *gin.Context) {
		dispatchOpenAICompatibleGateway(c, openAICompatible, generic)
	})
	router.POST("/v1/responses", func(c *gin.Context) {
		dispatchOpenAICompatibleGateway(c, openAICompatible, generic)
	})
	router.POST("/v1/chat/completions", func(c *gin.Context) {
		dispatchOpenAICompatibleGateway(c, openAICompatible, generic)
	})
	router.POST("/v1/messages/count_tokens", func(c *gin.Context) {
		dispatchOpenAICompatibleCountTokens(c, openAICompatible, grokCompatible, generic)
	})
	return router, hits
}

// ollama_cloud 普通分组（非 composite、无池候选）的三种文本入站端点必须全部
// 进入 OpenAI 兼容网关，而不是通用 Anthropic 网关。族判定的唯一源头是
// isOpenAICompatibleGatewayFamilyPlatform：清单缺 ollama_cloud 时，下列断言会以
// generic 命中的形式失败，而不是静默通过。
func TestOllamaCloudGroupRoutesTextEndpointsIntoOpenAICompatibleGateway(t *testing.T) {
	require.True(t, isOpenAICompatibleGatewayFamilyPlatform(service.PlatformOllamaCloud),
		"ollama_cloud must belong to the OpenAI-compatible gateway family")

	router, hits := newOllamaCloudFamilyRouter(t, service.PlatformOllamaCloud)

	for _, path := range []string{"/v1/messages", "/v1/responses", "/v1/chat/completions"} {
		before := *hits
		req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(`{"model":"llama3.1"}`))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		require.Equal(t, http.StatusNoContent, w.Code, "%s must reach a terminal handler", path)
		require.Equal(t, before.openAICompatible+1, hits.openAICompatible,
			"%s must route the ollama_cloud group into the OpenAI-compatible gateway", path)
		require.Equal(t, before.generic, hits.generic,
			"%s must not fall through to the generic Anthropic gateway", path)
	}
}

// count_tokens 对 ollama_cloud 普通分组沿用兼容分支语义：走 OpenAI
// CountTokens（compatible stub），不落 Grok 本地估算，也不落通用处理。
func TestOllamaCloudGroupCountTokensUsesCompatibleBranch(t *testing.T) {
	router, hits := newOllamaCloudFamilyRouter(t, service.PlatformOllamaCloud)

	req := httptest.NewRequest(http.MethodPost, "/v1/messages/count_tokens",
		strings.NewReader(`{"model":"llama3.1","messages":[]}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusNoContent, w.Code)
	require.Equal(t, 1, hits.openAICompatible, "count_tokens must take the compatible branch for ollama_cloud")
	require.Zero(t, hits.grok)
	require.Zero(t, hits.generic, "count_tokens must not fall through to the generic handler for ollama_cloud")
}
