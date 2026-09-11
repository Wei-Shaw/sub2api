package routes

import (
	"bytes"
	"compress/gzip"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/handler"
	"github.com/Wei-Shaw/sub2api/internal/pkg/httputil"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func gatewayCompressedPayload(t *testing.T, size int) []byte {
	t.Helper()
	body := append([]byte(`{"model":"gpt-test","data":"`), bytes.Repeat([]byte("x"), size-30)...)
	body = append(body, []byte(`"}`)...)
	// 以下测试只依赖正文超过限制，保持 JSON 完整即可。
	var compressed bytes.Buffer
	writer := gzip.NewWriter(&compressed)
	_, err := writer.Write(body)
	require.NoError(t, err)
	require.NoError(t, writer.Close())
	return compressed.Bytes()
}

func TestGatewayRoutesBindDecompressionLimitBeforeBodyReaders(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, tt := range []struct {
		path      string
		wantLimit int64
	}{
		{path: "/v1/responses", wantLimit: 512},
		{path: "/responses", wantLimit: 512},
		{path: "/responses/compact", wantLimit: 512},
		{path: "/backend-api/codex/responses", wantLimit: 512},
		{path: "/v1/messages", wantLimit: 512},
		{path: "/v1/chat/completions", wantLimit: 512},
		{path: "/v1/embeddings", wantLimit: 128},
		{path: "/embeddings", wantLimit: 128},
		{path: "/v1/alpha/search", wantLimit: 128},
		{path: "/alpha/search", wantLimit: 128},
		{path: "/backend-api/codex/alpha/search", wantLimit: 128},
	} {
		t.Run(tt.path, func(t *testing.T) {
			router := gin.New()
			RegisterGatewayRoutes(router, &handler.Handlers{
				Gateway: &handler.GatewayHandler{}, OpenAIGateway: &handler.OpenAIGatewayHandler{},
				AsyncImage: handler.NewAsyncImageHandler(nil, nil),
			}, middleware.APIKeyAuthMiddleware(func(c *gin.Context) {
				// 在最早进入鉴权时检查请求策略，不依赖账号、数据库或模型上游。
				body, err := httputil.ReadRequestBodyWithPrealloc(c.Request)
				var maxErr *http.MaxBytesError
				require.True(t, errors.As(err, &maxErr))
				require.Equal(t, tt.wantLimit, maxErr.Limit)
				require.Nil(t, body)
				c.AbortWithStatus(http.StatusRequestEntityTooLarge)
			}), nil, nil, nil, nil, nil, &config.Config{Gateway: config.GatewayConfig{
				MaxBodySize: 1024, TextMaxBodySize: 128, MaxDecompressedBodySize: 512,
			}})
			req := httptest.NewRequest(http.MethodPost, tt.path, bytes.NewReader(gatewayCompressedPayload(t, 600)))
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Content-Encoding", "gzip")
			recorder := httptest.NewRecorder()
			router.ServeHTTP(recorder, req)
			require.Equal(t, http.StatusRequestEntityTooLarge, recorder.Code)
		})
	}
}

func TestGatewayDecompressionLimitAppliesToEarlyReaders(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, allowlist := range []bool{false, true} {
		t.Run(map[bool]string{false: "composite", true: "allowlist"}[allowlist], func(t *testing.T) {
			router := gin.New()
			router.Use(middleware.RequestBodyLimitWithDecompression(1024, 128))
			router.Use(func(c *gin.Context) {
				c.Set(string(middleware.ContextKeyAPIKey), &service.APIKey{Group: &service.Group{
					ID: 1, Platform: service.PlatformComposite,
					ModelAllowlist: service.GroupModelAllowlist{Enabled: allowlist, Models: []string{"gpt-test"}},
				}})
				c.Next()
			})
			router.Use(middleware.GroupModelAllowlist(), compositeTargetPlatformMiddleware(nil))
			called := false
			router.POST("/v1/responses", func(c *gin.Context) { called = true; c.Status(http.StatusOK) })
			req := httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewReader(gatewayCompressedPayload(t, 300)))
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Content-Encoding", "gzip")
			recorder := httptest.NewRecorder()
			router.ServeHTTP(recorder, req)
			require.Equal(t, http.StatusRequestEntityTooLarge, recorder.Code)
			require.Contains(t, recorder.Body.String(), "too large")
			require.False(t, called)
		})
	}
}
