package routes

import (
	"context"
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/handler"
	servermiddleware "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestGeneratedImageRouteIsUnauthenticatedBeforeFallback(t *testing.T) {
	gin.SetMode(gin.TestMode)
	store := service.NewOpenAIGeneratedImageStore(t.TempDir())
	rec, err := store.SaveBase64(context.Background(), service.OpenAIGeneratedImageSaveInput{
		Base64:       base64.StdEncoding.EncodeToString([]byte("\x89PNG\r\n\x1a\nimage-bytes")),
		OutputFormat: "png",
	})
	require.NoError(t, err)

	router := gin.New()
	RegisterCommonRoutes(router, &handler.Handlers{GeneratedImage: handler.NewGeneratedImageHandler(store)})
	RegisterGatewayRoutes(
		router,
		&handler.Handlers{
			Gateway:       &handler.GatewayHandler{},
			OpenAIGateway: &handler.OpenAIGatewayHandler{},
		},
		servermiddleware.APIKeyAuthMiddleware(func(c *gin.Context) {
			c.Header("WWW-Authenticate", "Bearer")
			c.AbortWithStatus(http.StatusUnauthorized)
		}),
		nil,
		nil,
		nil,
		nil,
		&config.Config{},
	)
	router.NoRoute(func(c *gin.Context) {
		c.Header("WWW-Authenticate", "Bearer")
		c.AbortWithStatus(http.StatusUnauthorized)
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/sub2api/generated-images/"+rec.Filename, nil)
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	require.Empty(t, w.Header().Get("WWW-Authenticate"))
}
