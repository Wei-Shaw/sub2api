package middleware

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type trackedUpload struct {
	io.Reader
	read int
}

type failedUpload struct{}

func (failedUpload) Read([]byte) (int, error) { return 0, io.ErrUnexpectedEOF }

func (r *trackedUpload) Read(p []byte) (int, error) {
	n, err := r.Reader.Read(p)
	r.read += n
	return n, err
}

func TestResponsesCompactionBodyLimit(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, tc := range []struct {
		name, path, method string
		limit, declared    int64
		wantStatus         int
		wantRead           int
	}{
		{"known oversized", "/v1/responses", "POST", 4, 6, 400, 0},
		{"root alias", "/responses", "POST", 4, 6, 400, 0},
		{"codex alias", "/backend-api/codex/responses", "POST", 4, 6, 400, 0},
		{"chunked oversized", "/v1/responses", "POST", 4, -1, 400, 5},
		{"understated length", "/v1/responses", "POST", 4, 2, 400, 5},
		{"exact boundary", "/v1/responses", "POST", 6, 6, 200, 6},
		{"chunked within limit", "/v1/responses", "POST", 6, -1, 200, 6},
		{"disabled", "/v1/responses", "POST", 0, 6, 200, 6},
		{"other endpoint", "/v1/messages", "POST", 4, 6, 200, 6},
		{"compact exempt", "/v1/responses/compact", "POST", 4, 6, 200, 6},
		{"websocket exempt", "/v1/responses", "GET", 4, 6, 200, 6},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := gin.New()
			r.Use(ResponsesCompactionBodyLimit(tc.limit))
			called := false
			r.Handle(tc.method, tc.path, func(c *gin.Context) {
				called = true
				body, err := io.ReadAll(c.Request.Body)
				require.NoError(t, err)
				require.Equal(t, "abcdef", string(body))
				c.Status(http.StatusOK)
			})
			upload := &trackedUpload{Reader: strings.NewReader("abcdef")}
			req := httptest.NewRequest(tc.method, tc.path, upload)
			req.ContentLength = tc.declared
			rec := httptest.NewRecorder()
			r.ServeHTTP(rec, req)
			require.Equal(t, tc.wantStatus, rec.Code)
			require.Equal(t, tc.wantRead, upload.read)
			require.Equal(t, tc.wantStatus == 200, called)
			if !called {
				require.Contains(t, rec.Body.String(), `"code":"context_length_exceeded"`)
			}
		})
	}
}

func TestResponsesCompactionBodyLimitReadErrors(t *testing.T) {
	for _, upstreamLimit := range []int64{0, 2} {
		t.Run(strconv.FormatInt(upstreamLimit, 10), func(t *testing.T) {
			r := gin.New()
			if upstreamLimit > 0 {
				r.Use(RequestBodyLimit(upstreamLimit))
			}
			r.Use(ResponsesCompactionBodyLimit(10))
			r.POST("/responses", func(c *gin.Context) { t.Fatal("failed upload must not reach the handler") })
			req := httptest.NewRequest("POST", "/responses", io.MultiReader(strings.NewReader("abc"), failedUpload{}))
			req.ContentLength = 8
			rec := httptest.NewRecorder()
			r.ServeHTTP(rec, req)
			if upstreamLimit == 0 {
				require.Equal(t, 400, rec.Code)
			} else {
				require.Equal(t, 413, rec.Code)
			}
			require.NotContains(t, rec.Body.String(), "context_length_exceeded")
		})
	}
}
