package routes

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/handler"
	servermiddleware "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type routeResponsesImageStatusStore struct {
	items      map[string]*service.ResponsesImageStatus
	gets       int
	batchGets  int
	batchSizes []int
}

func (s *routeResponsesImageStatusStore) GetResponsesImageStatus(_ context.Context, requestID string) (*service.ResponsesImageStatus, error) {
	s.gets++
	if status := s.items[requestID]; status != nil {
		return status, nil
	}
	return nil, service.ErrResponsesImageStatusNotFound
}

func (s *routeResponsesImageStatusStore) GetResponsesImageStatuses(_ context.Context, requestIDs []string) (map[string]*service.ResponsesImageStatus, error) {
	s.batchGets++
	s.batchSizes = append(s.batchSizes, len(requestIDs))
	out := make(map[string]*service.ResponsesImageStatus, len(requestIDs))
	for _, requestID := range requestIDs {
		if status := s.items[requestID]; status != nil {
			out[requestID] = status
		}
	}
	return out, nil
}

func (s *routeResponsesImageStatusStore) SetResponsesImageStatus(_ context.Context, status *service.ResponsesImageStatus, _ time.Duration) error {
	if s.items == nil {
		s.items = make(map[string]*service.ResponsesImageStatus)
	}
	s.items[status.RequestID] = status
	return nil
}

func newGatewayRoutesTestRouter(platform ...string) *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()

	groupPlatform := service.PlatformOpenAI
	if len(platform) > 0 && platform[0] != "" {
		groupPlatform = platform[0]
	}

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
				Group:   &service.Group{Platform: groupPlatform},
			})
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

func newImagesStatusRouteTestRouter(auth gin.HandlerFunc, store service.ResponsesImageStatusStore) *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	openAIService := service.NewOpenAIGatewayService(
		nil, nil, nil, nil, nil, nil, nil, store, &config.Config{},
		nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil,
	)
	openAIHandler := handler.NewOpenAIGatewayHandler(openAIService, nil, nil, nil, nil, nil, nil, nil, &config.Config{})
	RegisterGatewayRoutes(
		router,
		&handler.Handlers{
			Gateway:       &handler.GatewayHandler{},
			OpenAIGateway: openAIHandler,
			FalGateway:    &handler.FalGatewayHandler{},
		},
		servermiddleware.APIKeyAuthMiddleware(auth),
		nil,
		nil,
		nil,
		nil,
		&config.Config{},
	)
	return router
}

func TestGatewayRoutesImagesStatusQuery(t *testing.T) {
	now := time.Date(2026, 7, 4, 12, 0, 0, 0, time.UTC)
	store := &routeResponsesImageStatusStore{items: map[string]*service.ResponsesImageStatus{
		"img-1": {
			RequestID: "img-1",
			Status:    service.ResponsesImageStatusSucceeded,
			Progress:  100,
			COSURLs:   []string{"https://cos.example/img-1.png"},
			CreatedAt: now,
			UpdatedAt: now,
		},
	}}
	auth := func(c *gin.Context) {
		c.Set(string(servermiddleware.ContextKeyAPIKey), &service.APIKey{ID: 99})
		c.Next()
	}
	router := newImagesStatusRouteTestRouter(auth, store)

	req := httptest.NewRequest(http.MethodGet, "/v1/images/status/?request_id=img-1", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	require.Contains(t, w.Body.String(), `"data":[`)
	require.Contains(t, w.Body.String(), `"request_id":"img-1"`)
	require.Contains(t, w.Body.String(), `"cos_urls":["https://cos.example/img-1.png"]`)
}

func TestGatewayRoutesImagesStatusBatchQuery(t *testing.T) {
	store := &routeResponsesImageStatusStore{items: map[string]*service.ResponsesImageStatus{
		"img-1": {
			RequestID: "img-1",
			Status:    service.ResponsesImageStatusSucceeded,
			Progress:  100,
		},
		"img-2": {
			RequestID: "img-2",
			Status:    service.ResponsesImageStatusRunning,
			Progress:  25,
		},
	}}
	auth := func(c *gin.Context) {
		c.Set(string(servermiddleware.ContextKeyAPIKey), &service.APIKey{ID: 99})
		c.Next()
	}
	router := newImagesStatusRouteTestRouter(auth, store)

	req := httptest.NewRequest(http.MethodGet, "/v1/images/status/?request_ids=img-1,missing&request_id=img-2", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	require.Contains(t, w.Body.String(), `"request_id":"img-1"`)
	require.Contains(t, w.Body.String(), `"request_id":"img-2"`)
	require.Contains(t, w.Body.String(), `"not_found":["missing"]`)
	require.Equal(t, 0, store.gets)
	require.Equal(t, 1, store.batchGets)
	require.Equal(t, []int{3}, store.batchSizes)
}

func TestGatewayRoutesImagesStatusBatchLimit(t *testing.T) {
	auth := func(c *gin.Context) {
		c.Set(string(servermiddleware.ContextKeyAPIKey), &service.APIKey{ID: 99})
		c.Next()
	}
	store := &routeResponsesImageStatusStore{}
	router := newImagesStatusRouteTestRouter(auth, store)

	ids := make([]string, 0, 101)
	for i := 0; i < 101; i++ {
		ids = append(ids, "img-"+strconv.Itoa(i))
	}
	req := httptest.NewRequest(http.MethodGet, "/v1/images/status/?request_ids="+strings.Join(ids, ","), nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusBadRequest, w.Code)
	require.Contains(t, w.Body.String(), "at most 100")
	require.Equal(t, 0, store.gets)
	require.Equal(t, 0, store.batchGets)
}

func TestGatewayRoutesImagesStatusMissingExpiredAndInvalidAuth(t *testing.T) {
	authOK := func(c *gin.Context) {
		c.Set(string(servermiddleware.ContextKeyAPIKey), &service.APIKey{ID: 1})
		c.Next()
	}
	router := newImagesStatusRouteTestRouter(authOK, &routeResponsesImageStatusStore{})

	req := httptest.NewRequest(http.MethodGet, "/v1/images/status/?request_id=expired", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusNotFound, w.Code)

	authReject := func(c *gin.Context) {
		c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "invalid"})
	}
	router = newImagesStatusRouteTestRouter(authReject, &routeResponsesImageStatusStore{})
	req = httptest.NewRequest(http.MethodGet, "/v1/images/status/?request_id=img-1", nil)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestGatewayRoutesImagesStatusDoesNotRequireGroupOrOwnerMatch(t *testing.T) {
	store := &routeResponsesImageStatusStore{items: map[string]*service.ResponsesImageStatus{
		"shared": {
			RequestID: "shared",
			Status:    service.ResponsesImageStatusRunning,
			Progress:  25,
		},
	}}
	auth := func(c *gin.Context) {
		c.Set(string(servermiddleware.ContextKeyAPIKey), &service.APIKey{ID: 200})
		c.Next()
	}
	router := newImagesStatusRouteTestRouter(auth, store)

	req := httptest.NewRequest(http.MethodGet, "/v1/images/status/?request_id=shared", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	require.Contains(t, w.Body.String(), `"request_id":"shared"`)
}

// TestGatewayRoutesFalImagesDispatch 验证 fal 平台分组的 images 请求被分流到 fal 伪同步门面。
// 不注入 AuthSubject，使 Fal.Images 在 "User context not found" 处早返回 500，
// 既证明命中了 fal 门面（而非 OpenAI 或 404），又避免触达 nil 依赖。
func TestGatewayRoutesFalImagesDispatch(t *testing.T) {
	router := newGatewayRoutesTestRouter(service.PlatformFal)

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

func TestGatewayRoutesGrokImagesAndVideosPathsAreRegistered(t *testing.T) {
	router := newGatewayRoutesTestRouter(service.PlatformGrok)

	for _, path := range []string{
		"/v1/images/generations",
		"/v1/images/edits",
		"/images/generations",
		"/images/edits",
		"/v1/videos/generations",
		"/videos/generations",
	} {
		req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(`{"model":"grok-imagine","prompt":"draw a cat"}`))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)
		require.NotEqual(t, http.StatusNotFound, w.Code, "path=%s should hit Grok media handler", path)
		require.NotContains(t, w.Body.String(), "not supported for this platform")
	}

	for _, path := range []string{
		"/v1/videos/request-123",
		"/videos/request-123",
	} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)
		require.NotEqual(t, http.StatusNotFound, w.Code, "path=%s should hit Grok video handler", path)
		require.NotContains(t, w.Body.String(), "not supported for this platform")
	}
}

func TestGatewayRoutesNonGrokVideosAreRejectedAtPlatformGate(t *testing.T) {
	router := newGatewayRoutesTestRouter(service.PlatformOpenAI)

	for _, tc := range []struct {
		method string
		path   string
		body   string
	}{
		{http.MethodPost, "/v1/videos/generations", `{"model":"grok-imagine-video-1.5","prompt":"waves"}`},
		{http.MethodPost, "/videos/generations", `{"model":"grok-imagine-video-1.5","prompt":"waves"}`},
		{http.MethodGet, "/v1/videos/request-123", ""},
		{http.MethodGet, "/videos/request-123", ""},
	} {
		req := httptest.NewRequest(tc.method, tc.path, strings.NewReader(tc.body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)
		require.Equal(t, http.StatusNotFound, w.Code, "method=%s path=%s", tc.method, tc.path)
		require.Contains(t, w.Body.String(), "Videos API is not supported for this platform")
	}
}

func TestGatewayRoutesGrokAllowsCLICompatibilityEntrypoints(t *testing.T) {
	router := newGatewayRoutesTestRouter(service.PlatformGrok)

	for _, tc := range []struct {
		method string
		path   string
	}{
		{http.MethodPost, "/v1/messages"},
		{http.MethodPost, "/v1/chat/completions"},
		{http.MethodPost, "/chat/completions"},
		{http.MethodGet, "/v1/responses"},
		{http.MethodGet, "/responses"},
		{http.MethodGet, "/backend-api/codex/responses"},
	} {
		req := httptest.NewRequest(tc.method, tc.path, strings.NewReader(`{"model":"grok"}`))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)
		require.NotEqual(t, http.StatusNotFound, w.Code, "method=%s path=%s", tc.method, tc.path)
		require.NotContains(t, w.Body.String(), "not supported for Grok groups")
	}

	req := httptest.NewRequest(http.MethodPost, "/v1/messages/count_tokens", strings.NewReader(`{"model":"grok","messages":[{"role":"user","content":"hi"}]}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusNotFound, w.Code)
	require.Contains(t, w.Body.String(), "Token counting is not supported for this platform")

	for _, path := range []string{
		"/v1/responses",
		"/responses",
		"/backend-api/codex/responses",
	} {
		req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(`{"model":"grok","input":"hi"}`))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)
		require.NotEqual(t, http.StatusNotFound, w.Code, "path=%s should still reach Responses handler", path)
	}
}

// TestGatewayRoutesImagesUnsupportedPlatformReturns404 验证非 OpenAI/fal 平台的 images 请求返回 404。
func TestGatewayRoutesImagesUnsupportedPlatformReturns404(t *testing.T) {
	router := newGatewayRoutesTestRouter(service.PlatformAntigravity)

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
	router := newGatewayRoutesTestRouter(service.PlatformFal)

	req := httptest.NewRequest(http.MethodGet, "/fal/openai/gpt-image-2", nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusNotFound, w.Code)
	require.Contains(t, w.Body.String(), "Unsupported fal endpoint")
}

func TestGatewayRoutesOpenAICountTokensPathIsRegistered(t *testing.T) {
	router := newGatewayRoutesTestRouter(service.PlatformOpenAI)

	req := httptest.NewRequest(http.MethodPost, "/v1/messages/count_tokens", strings.NewReader(`{"model":"claude-sonnet-4-5","messages":[{"role":"user","content":"hi"}]}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)
	require.NotEqual(t, http.StatusNotFound, w.Code)
}
