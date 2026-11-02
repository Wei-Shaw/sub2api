package routes

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/handler"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestDecisionBridgeProductionRouteDispatchRejectsUnrepresentable(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	cfg := &config.Config{Gateway: config.GatewayConfig{MaxBodySize: 1 << 20, TextMaxBodySize: 1 << 20}}
	auth := middleware.APIKeyAuthMiddleware(func(c *gin.Context) {
		if c.GetHeader("Authorization") != "Bearer fixture-token" {
			c.AbortWithStatus(401)
			return
		}
		id := int64(123)
		c.Set(string(middleware.ContextKeyAPIKey), &service.APIKey{GroupID: &id, Group: &service.Group{ID: id, Platform: service.PlatformTypeSafe}})
		c.Set(string(middleware.ContextKeyUser), middleware.AuthSubject{UserID: 9})
		c.Next()
	})
	RegisterGatewayRoutes(router, &handler.Handlers{Gateway: &handler.GatewayHandler{}, OpenAIGateway: &handler.OpenAIGatewayHandler{}, AsyncImage: handler.NewAsyncImageHandler(nil, nil)}, auth, nil, nil, nil, nil, nil, cfg)
	body := `{"model":"gpt-6-luna","input":"hello","questions":[{"type":"predicate","instructions":"x"}],"safety_identifier":"cannot-forward"}`
	request := httptest.NewRequest(http.MethodPost, "/v1/decisions", strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	denied := httptest.NewRecorder()
	router.ServeHTTP(denied, request)
	require.Equal(t, 401, denied.Code)
	request = httptest.NewRequest(http.MethodPost, "/v1/decisions", strings.NewReader(body))
	request.Header.Set("Authorization", "Bearer fixture-token")
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	require.Equal(t, 400, recorder.Code, recorder.Body.String())
	require.Contains(t, recorder.Body.String(), "nonrepresentable")
	require.False(t, gjson.GetBytes(recorder.Body.Bytes(), "type").Exists(), "Decisions errors must use the Decisions envelope")
}
