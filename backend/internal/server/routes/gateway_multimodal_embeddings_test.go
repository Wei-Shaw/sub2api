package routes

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/handler"
	servermiddleware "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

var multimodalEmbeddingPaths = []string{
	"/v1/embeddings/multimodal",
	"/embeddings/multimodal",
	"/api/v3/embeddings/multimodal",
	"/v3/embeddings/multimodal",
}

func TestGatewayRoutesMultimodalEmbeddings(t *testing.T) {
	for _, path := range multimodalEmbeddingPaths {
		t.Run(path, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(`{"model":"doubao-embedding-vision-251215","input":[{"type":"image_url","image_url":{"url":"https://example.com/image.png"}}]}`))
			rec := httptest.NewRecorder()
			newGatewayRoutesTestRouter().ServeHTTP(rec, req)
			// The fixture supplies an API key but no user context, so reaching
			// the embeddings handler produces this error before scheduling.
			require.Equal(t, http.StatusInternalServerError, rec.Code)
			require.Contains(t, rec.Body.String(), "User context not found")
			require.Equal(t, service.CompositeRouteEndpointEmbeddings, compositeRouteEndpointForPath(path))
		})
	}
}

func TestGatewayRoutesMultimodalEmbeddingsRequireAuthentication(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	cfg := &config.Config{Gateway: config.GatewayConfig{MaxBodySize: 4096, TextMaxBodySize: 256}}
	RegisterGatewayRoutes(router, &handler.Handlers{
		Gateway: &handler.GatewayHandler{}, OpenAIGateway: &handler.OpenAIGatewayHandler{},
		AsyncImage: handler.NewAsyncImageHandler(nil, nil),
	}, servermiddleware.NewAPIKeyAuthMiddleware(nil, nil, cfg), nil, nil, nil, nil, nil, cfg)

	for _, path := range multimodalEmbeddingPaths {
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, path, strings.NewReader(`{"model":"doubao-embedding-vision-251215"}`)))
		require.Equal(t, http.StatusUnauthorized, rec.Code, "path=%s", path)
	}
}

func TestGatewayRoutesMultimodalEmbeddingsRejectUnsupportedPlatform(t *testing.T) {
	for _, platform := range []string{service.PlatformGemini, service.PlatformAnthropic, service.PlatformGrok} {
		router := newGatewayRoutesTestRouter(platform)
		for _, path := range multimodalEmbeddingPaths {
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, path, strings.NewReader(`{"model":"doubao-embedding-vision-251215"}`)))
			require.Equal(t, http.StatusNotFound, rec.Code, "platform=%s path=%s", platform, path)
			require.Contains(t, rec.Body.String(), "Embeddings API is not supported for this platform")
		}
	}
}

func TestGatewayRoutesMultimodalEmbeddingsRespectModelAllowlist(t *testing.T) {
	router := newGatewayRoutesTestRouterWithGroup(allowlistGroup(service.PlatformOpenAI, true, "text-embedding-3-small"))
	for _, path := range multimodalEmbeddingPaths {
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, path, strings.NewReader(`{"model":"doubao-embedding-vision-251215"}`)))
		require.Equal(t, http.StatusNotFound, rec.Code, "path=%s", path)
		require.Contains(t, rec.Body.String(), "not available for this group")
	}
}
