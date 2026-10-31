package handler

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestOpenAIAsyncDisabled(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h := &OpenAIAsyncHandler{}
	r := gin.New()
	r.POST("/v1/files", h.Upload)
	r.GET("/v1/files", h.ListFiles)
	r.POST("/v1/batches", h.CreateBatch)
	for _, tc := range []struct{ method, path string }{{"POST", "/v1/files"}, {"GET", "/v1/files"}, {"POST", "/v1/batches"}} {
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest(tc.method, tc.path, nil))
		require.Equal(t, 404, w.Code)
	}
}

func TestBatchResponseCaptureBound(t *testing.T) {
	w := &batchResponseWriter{header: make(http.Header), limit: 4}
	n, err := w.Write([]byte("1234"))
	require.NoError(t, err)
	require.Equal(t, 4, n)
	n, err = w.Write([]byte("5"))
	require.NoError(t, err)
	require.Equal(t, 1, n)
	require.True(t, w.overflow)
	require.Equal(t, "1234", w.body.String())
}
