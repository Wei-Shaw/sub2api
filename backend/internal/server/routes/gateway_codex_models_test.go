package routes

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestGatewayRoutesCodexModelsManifestPathIsRegistered(t *testing.T) {
	router := newGatewayRoutesTestRouter()

	registered := make(map[string]string)
	for _, route := range router.Routes() {
		if route.Method == http.MethodGet {
			registered[route.Path] = route.Handler
		}
	}

	require.NotEmpty(t, registered["/backend-api/codex/models"], "GET /backend-api/codex/models should be registered")
	require.NotEmpty(t, registered["/v1/models"], "GET /v1/models should be registered")
	require.NotEmpty(t, registered["/models"], "GET /models should be registered")
	require.Equal(t, registered["/v1/models"], registered["/models"], "root alias should use the same platform-aware handler")
}

func TestDispatchCodexModelsGatewayKeepsOnlyOpenAIOnLiveManifestHandler(t *testing.T) {
	gin.SetMode(gin.TestMode)
	tests := []struct {
		platform   string
		wantOpenAI bool
	}{
		{platform: service.PlatformOpenAI, wantOpenAI: true},
		{platform: service.PlatformComposite},
		{platform: service.PlatformGrok},
		{platform: service.PlatformDeepseek},
	}

	for _, tt := range tests {
		t.Run(tt.platform, func(t *testing.T) {
			rec := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(rec)
			c.Request = httptest.NewRequest(http.MethodGet, "/models?client_version=0.147.0", nil)
			c.Set(string(middleware.ContextKeyAPIKey), &service.APIKey{
				Group: &service.Group{Platform: tt.platform},
			})
			called := ""

			dispatchCodexModelsGateway(c,
				func(c *gin.Context) { called = "openai" },
				func(c *gin.Context) { called = "generated" },
			)

			if tt.wantOpenAI {
				require.Equal(t, "openai", called)
			} else {
				require.Equal(t, "generated", called)
			}
		})
	}
}
