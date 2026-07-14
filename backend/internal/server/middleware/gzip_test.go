//go:build unit

package middleware

import (
	"bytes"
	"compress/gzip"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func init() {
	gin.SetMode(gin.TestMode)
}

func TestIsGzipExcludedPath(t *testing.T) {
	excluded := []string{
		"/v1",
		"/v1/messages",
		"/v1/chat/completions",
		"/v1beta/models",
		"/backend-api/codex/responses",
		"/antigravity/v1/messages",
		"/responses",
		"/responses/abc",
		"/alpha/search",
		"/images/generations",
		"/videos/request-123",
	}
	for _, path := range excluded {
		assert.True(t, isGzipExcludedPath(path), "path=%s", path)
	}

	allowed := []string{
		"/",
		"/index.html",
		"/assets/index.js",
		"/api/v1/admin/settings",
		"/api/v1/auth/me",
		"/health",
		"/setup/",
	}
	for _, path := range allowed {
		assert.False(t, isGzipExcludedPath(path), "path=%s", path)
	}
}

func TestIsCompressibleContentType(t *testing.T) {
	assert.True(t, isCompressibleContentType("text/html; charset=utf-8"))
	assert.True(t, isCompressibleContentType("application/json"))
	assert.True(t, isCompressibleContentType("application/javascript"))
	assert.True(t, isCompressibleContentType("image/svg+xml"))
	assert.False(t, isCompressibleContentType("text/event-stream"))
	assert.False(t, isCompressibleContentType("image/png"))
	assert.False(t, isCompressibleContentType("application/octet-stream"))
}

func TestGzip_CompressesLargeJSON(t *testing.T) {
	r := gin.New()
	r.Use(Gzip())
	body := strings.Repeat(`{"ok":true,"msg":"hello-world"}`, 20) // > 256 bytes
	r.GET("/api/v1/ping", func(c *gin.Context) {
		c.Data(http.StatusOK, "application/json", []byte(body))
	})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/ping", nil)
	req.Header.Set("Accept-Encoding", "gzip")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "gzip", w.Header().Get("Content-Encoding"))
	assert.Contains(t, w.Header().Get("Vary"), "Accept-Encoding")

	gr, err := gzip.NewReader(w.Body)
	require.NoError(t, err)
	defer gr.Close()
	decoded, err := io.ReadAll(gr)
	require.NoError(t, err)
	assert.Equal(t, body, string(decoded))
	assert.Less(t, w.Body.Len(), len(body))
}

func TestGzip_SkipsSmallResponse(t *testing.T) {
	r := gin.New()
	r.Use(Gzip())
	r.GET("/api/v1/small", func(c *gin.Context) {
		c.Data(http.StatusOK, "application/json", []byte(`{"ok":true}`))
	})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/small", nil)
	req.Header.Set("Accept-Encoding", "gzip")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	assert.Empty(t, w.Header().Get("Content-Encoding"))
	assert.Equal(t, `{"ok":true}`, w.Body.String())
}

func TestGzip_SkipsGatewayStreamingPath(t *testing.T) {
	r := gin.New()
	r.Use(Gzip())
	payload := strings.Repeat("data: hello\n\n", 40)
	r.GET("/v1/chat/completions", func(c *gin.Context) {
		c.Header("Content-Type", "text/event-stream")
		c.String(http.StatusOK, payload)
	})

	req := httptest.NewRequest(http.MethodGet, "/v1/chat/completions", nil)
	req.Header.Set("Accept-Encoding", "gzip")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	assert.Empty(t, w.Header().Get("Content-Encoding"))
	assert.Equal(t, payload, w.Body.String())
}

func TestGzip_SkipsWhenAcceptEncodingMissing(t *testing.T) {
	r := gin.New()
	r.Use(Gzip())
	body := strings.Repeat("x", 512)
	r.GET("/assets/app.js", func(c *gin.Context) {
		c.Data(http.StatusOK, "application/javascript", []byte(body))
	})

	req := httptest.NewRequest(http.MethodGet, "/assets/app.js", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	assert.Empty(t, w.Header().Get("Content-Encoding"))
	assert.Equal(t, body, w.Body.String())
}

func TestGzip_SkipsAlreadyEncoded(t *testing.T) {
	r := gin.New()
	r.Use(Gzip())
	raw := strings.Repeat("precompressed", 40)
	r.GET("/assets/app.js", func(c *gin.Context) {
		c.Header("Content-Encoding", "br")
		c.Data(http.StatusOK, "application/javascript", []byte(raw))
	})

	req := httptest.NewRequest(http.MethodGet, "/assets/app.js", nil)
	req.Header.Set("Accept-Encoding", "gzip")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "br", w.Header().Get("Content-Encoding"))
	assert.Equal(t, raw, w.Body.String())
}

func TestGzip_SkipsEventStreamContentTypeOnNonExcludedPath(t *testing.T) {
	r := gin.New()
	r.Use(Gzip())
	payload := strings.Repeat("data: x\n\n", 50)
	r.GET("/api/v1/events", func(c *gin.Context) {
		c.Header("Content-Type", "text/event-stream")
		_, _ = c.Writer.Write([]byte(payload))
	})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/events", nil)
	req.Header.Set("Accept-Encoding", "gzip")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	assert.Empty(t, w.Header().Get("Content-Encoding"))
	assert.Equal(t, payload, w.Body.String())
}

func TestGzip_PassthroughBinary(t *testing.T) {
	r := gin.New()
	r.Use(Gzip())
	raw := bytes.Repeat([]byte{0x1, 0x2, 0x3, 0x4}, 128)
	r.GET("/assets/logo.png", func(c *gin.Context) {
		c.Data(http.StatusOK, "image/png", raw)
	})

	req := httptest.NewRequest(http.MethodGet, "/assets/logo.png", nil)
	req.Header.Set("Accept-Encoding", "gzip")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	assert.Empty(t, w.Header().Get("Content-Encoding"))
	assert.Equal(t, raw, w.Body.Bytes())
}
