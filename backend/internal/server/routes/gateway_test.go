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

func newGatewayRoutesTestRouter() *gin.Engine {
	return newGatewayRoutesTestRouterForPlatform(service.PlatformOpenAI, false)
}

// newGatewayRoutesTestRouterForPlatform 构造一个测试路由器，认证桩按指定平台分组，
// setAuthSubject 控制是否注入 AuthSubject（不注入时门面会在早期返回，避免触达 nil 依赖）。
func newGatewayRoutesTestRouterForPlatform(platform string, setAuthSubject bool) *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()

	RegisterGatewayRoutes(
		router,
		&handler.Handlers{
			Gateway:       &handler.GatewayHandler{},
			OpenAIGateway: &handler.OpenAIGatewayHandler{},
			FalGateway:    &handler.FalGatewayHandler{},
		},
		servermiddleware.APIKeyAuthMiddleware(func(c *gin.Context) {
			groupID := int64(1)
			c.Set(string(servermiddleware.ContextKeyAPIKey), &service.APIKey{
				GroupID: &groupID,
				Group:   &service.Group{Platform: platform},
			})
			if setAuthSubject {
				c.Set(string(servermiddleware.ContextKeyUser), servermiddleware.AuthSubject{UserID: 1})
			}
			c.Next()
		}),
		nil,
		nil,
		nil,
		nil,
		&config.Config{},
	)

	return router
}

func TestGatewayRoutesOpenAIResponsesCompactPathIsRegistered(t *testing.T) {
	router := newGatewayRoutesTestRouter()

	for _, path := range []string{
		"/v1/responses/compact",
		"/responses/compact",
		"/backend-api/codex/responses",
		"/backend-api/codex/responses/compact",
	} {
		req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(`{"model":"gpt-5"}`))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)
		require.NotEqual(t, http.StatusNotFound, w.Code, "path=%s should hit OpenAI responses handler", path)
	}
}

func TestGatewayRoutesOpenAIImagesPathsAreRegistered(t *testing.T) {
	router := newGatewayRoutesTestRouter()

	for _, path := range []string{
		"/v1/images/generations",
		"/v1/images/edits",
		"/images/generations",
		"/images/edits",
	} {
		req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(`{"model":"gpt-image-2","prompt":"draw a cat"}`))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)
		require.NotEqual(t, http.StatusNotFound, w.Code, "path=%s should hit OpenAI images handler", path)
	}
}

// TestGatewayRoutesFalImagesDispatch 验证 fal 平台分组的 images 请求被分流到 fal 伪同步门面。
// 不注入 AuthSubject，使 Fal.Images 在 "User context not found" 处早返回 500，
// 既证明命中了 fal 门面（而非 OpenAI 或 404），又避免触达 nil 依赖。
func TestGatewayRoutesFalImagesDispatch(t *testing.T) {
	router := newGatewayRoutesTestRouterForPlatform(service.PlatformFal, false)

	for _, path := range []string{
		"/v1/images/generations",
		"/v1/images/edits",
		"/images/generations",
		"/images/edits",
	} {
		req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(`{"model":"gpt-image-2","prompt":"draw a cat"}`))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)
		require.Equal(t, http.StatusInternalServerError, w.Code, "path=%s should reach fal Images handler", path)
		require.Contains(t, w.Body.String(), "User context not found", "path=%s should be handled by fal facade", path)
	}
}

// TestGatewayRoutesImagesUnsupportedPlatformReturns404 验证非 OpenAI/fal 平台的 images 请求返回 404。
func TestGatewayRoutesImagesUnsupportedPlatformReturns404(t *testing.T) {
	router := newGatewayRoutesTestRouterForPlatform(service.PlatformAntigravity, false)

	req := httptest.NewRequest(http.MethodPost, "/v1/images/generations", strings.NewReader(`{"model":"x"}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusNotFound, w.Code)
	require.Contains(t, w.Body.String(), "not supported for this platform")
}

// TestGatewayRoutesFalNativeGroupIsRegistered 验证 /fal 原生路由组已注册并由 fal 门面分发。
// GET /fal/{model}（无 /requests、无 /status）命中 Native 的 default 分支，返回门面自定义 404，
// 据此与「路由未注册」的 gin 默认 404 区分。
func TestGatewayRoutesFalNativeGroupIsRegistered(t *testing.T) {
	router := newGatewayRoutesTestRouterForPlatform(service.PlatformFal, false)

	req := httptest.NewRequest(http.MethodGet, "/fal/openai/gpt-image-2", nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusNotFound, w.Code)
	require.Contains(t, w.Body.String(), "Unsupported fal endpoint")
}
