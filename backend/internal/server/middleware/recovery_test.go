//go:build unit

package middleware

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestRecovery_PanicLogContainsInfo(t *testing.T) {
	gin.SetMode(gin.TestMode)

	// 临时替换 DefaultErrorWriter 以捕获日志输出
	var buf bytes.Buffer
	originalWriter := gin.DefaultErrorWriter
	gin.DefaultErrorWriter = &buf
	t.Cleanup(func() {
		gin.DefaultErrorWriter = originalWriter
	})

	r := gin.New()
	r.Use(Recovery())
	r.GET("/panic", func(c *gin.Context) {
		panic("custom panic message for test")
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/panic", nil)
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusInternalServerError, w.Code)

	logOutput := buf.String()
	require.Contains(t, logOutput, "custom panic message for test", "日志应包含 panic 信息")
	require.Contains(t, logOutput, "recovery_test.go", "日志应包含堆栈跟踪文件名")
}

func TestRecovery(t *testing.T) {
	gin.SetMode(gin.TestMode)

	tests := []struct {
		name         string
		handler      gin.HandlerFunc
		wantHTTPCode int
		wantBody     response.Response
	}{
		{
			name: "panic_returns_standard_json_500",
			handler: func(c *gin.Context) {
				panic("boom")
			},
			wantHTTPCode: http.StatusInternalServerError,
			wantBody: response.Response{
				Code:    http.StatusInternalServerError,
				Message: infraerrors.UnknownMessage,
			},
		},
		{
			name: "no_panic_passthrough",
			handler: func(c *gin.Context) {
				response.Success(c, gin.H{"ok": true})
			},
			wantHTTPCode: http.StatusOK,
			wantBody: response.Response{
				Code:    0,
				Message: "success",
				Data:    map[string]any{"ok": true},
			},
		},
		{
			name: "panic_after_write_does_not_override_body",
			handler: func(c *gin.Context) {
				response.Success(c, gin.H{"ok": true})
				panic("boom")
			},
			wantHTTPCode: http.StatusOK,
			wantBody: response.Response{
				Code:    0,
				Message: "success",
				Data:    map[string]any{"ok": true},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := gin.New()
			r.Use(Recovery())
			r.GET("/t", tt.handler)

			w := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, "/t", nil)
			r.ServeHTTP(w, req)

			require.Equal(t, tt.wantHTTPCode, w.Code)

			var got response.Response
			require.NoError(t, json.Unmarshal(w.Body.Bytes(), &got))
			require.Equal(t, tt.wantBody, got)
		})
	}
}
