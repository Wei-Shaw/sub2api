package middleware

import (
	"bytes"
	"compress/gzip"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/pkg/httputil"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestRequestDecompressionLimitsAreRequestScoped(t *testing.T) {
	gin.SetMode(gin.TestMode)
	payload := bytes.Repeat([]byte("x"), 200)
	var compressed bytes.Buffer
	writer := gzip.NewWriter(&compressed)
	_, err := writer.Write(payload)
	require.NoError(t, err)
	require.NoError(t, writer.Close())
	for _, tt := range []struct {
		name         string
		wireLimit    int64
		decodedLimit int64
		wantStatus   int
	}{
		{name: "configured_limit", wireLimit: 1024, decodedLimit: 128, wantStatus: 413},
		{name: "larger_limit", wireLimit: 1024, decodedLimit: 256, wantStatus: 200},
		{name: "route_cap", wireLimit: 128, decodedLimit: 256, wantStatus: 413},
		{name: "wire_cap", wireLimit: 16, decodedLimit: 256, wantStatus: 413},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			router := gin.New()
			router.Use(RequestBodyLimitWithDecompression(tt.wireLimit, tt.decodedLimit))
			router.POST("/", func(c *gin.Context) {
				body, err := httputil.ReadRequestBodyWithPrealloc(c.Request)
				if err != nil {
					var maxErr *http.MaxBytesError
					require.True(t, errors.As(err, &maxErr))
					require.Nil(t, body)
					c.Status(http.StatusRequestEntityTooLarge)
					return
				}
				require.Equal(t, payload, body)
				c.Status(http.StatusOK)
			})
			req := httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(compressed.Bytes()))
			req.Header.Set("Content-Encoding", "gzip")
			recorder := httptest.NewRecorder()
			router.ServeHTTP(recorder, req)
			require.Equal(t, tt.wantStatus, recorder.Code)
		})
	}
}
