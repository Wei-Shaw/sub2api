package routes

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/handler"
	servermiddleware "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestStrictAnthropicAuxiliaryRoutesRequireAuthentication(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	handlers := &handler.Handlers{Gateway: &handler.GatewayHandler{}, OpenAIGateway: &handler.OpenAIGatewayHandler{}, AsyncImage: handler.NewAsyncImageHandler(nil, nil)}
	authCalls := 0
	RegisterGatewayRoutes(router, handlers, servermiddleware.APIKeyAuthMiddleware(func(c *gin.Context) {
		authCalls++
		c.AbortWithStatus(http.StatusUnauthorized)
	}), nil, nil, nil, nil, nil, &config.Config{})
	cases := []struct{ method, path string }{
		{"GET", "/v1/models"}, {"GET", "/v1/models/claude-test"},
		{"GET", "/v1/files"}, {"POST", "/v1/files"}, {"GET", "/v1/files/file_test"},
		{"GET", "/v1/files/file_test/content"}, {"DELETE", "/v1/files/file_test"},
	}
	for _, tc := range cases {
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, httptest.NewRequest(tc.method, tc.path, nil))
		require.Equal(t, http.StatusUnauthorized, rec.Code, tc.method+" "+tc.path)
	}
	require.Equal(t, len(cases), authCalls)
}
