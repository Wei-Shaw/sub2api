//go:build unit

package middleware

import (
	"bufio"
	"bytes"
	"compress/gzip"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	pkghttputil "github.com/Wei-Shaw/sub2api/internal/pkg/httputil"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestDeepseekToolsBodyLimits(t *testing.T) {
	const textLimit = 1024
	for _, tc := range []struct {
		path      string
		limit     int64
		anthropic bool
	}{
		{"/v1/tools/search", textLimit, false}, {"/v1/tools/fetch", textLimit, false},
		{"/v1/tools/tts", 64 << 10, false}, {"/v1/tools/asr", 8 << 20, false},
		{"/v1/tools/ocr", 100 << 20, false}, {"/v1/files", 100 << 20, false}, {"/v1/files", 100 << 20, true},
	} {
		t.Run(tc.path, func(t *testing.T) {
			r := gin.New()
			r.Use(DeepseekToolsBodyLimit(textLimit, 256<<20))
			r.POST(tc.path, func(c *gin.Context) { c.Status(204) })
			for _, extra := range []int64{0, 1} {
				w := httptest.NewRecorder()
				req := httptest.NewRequest("POST", tc.path, nil)
				req.ContentLength = tc.limit + extra
				req.Body = unreadDeepseekBody{t}
				if tc.anthropic {
					req.Header.Set("anthropic-beta", "files-api-2025-04-14")
				}
				r.ServeHTTP(w, req)
				if extra == 0 {
					require.Equal(t, 204, w.Code)
					continue
				}
				require.Equal(t, 413, w.Code)
				require.Equal(t, tc.anthropic, gjson.GetBytes(w.Body.Bytes(), "type").String() == "error")
				require.Equal(t, tc.anthropic, gjson.GetBytes(w.Body.Bytes(), "error.type").String() == "request_too_large")
			}
		})
	}
}

type unreadDeepseekBody struct{ t *testing.T }

func (b unreadDeepseekBody) Read([]byte) (int, error) {
	b.t.Fatal("declared length must be checked before reading")
	return 0, io.EOF
}
func (unreadDeepseekBody) Close() error { return nil }

func TestDeepseekToolsChunkedAndDecodedLimits(t *testing.T) {
	const limit = 64 << 10
	for _, tc := range []struct {
		name, path string
		gzip, beta bool
	}{
		{"chunked TTS", "/v1/tools/tts", false, false}, {"compressed TTS", "/v1/tools/tts", true, false},
		{"chunked Files under tighter gateway limit", "/v1/files", false, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := gin.New()
			r.Use(DeepseekToolsBodyLimit(1024, limit))
			r.POST(tc.path, func(c *gin.Context) {
				_, err := pkghttputil.ReadRequestBodyWithPrealloc(c.Request)
				if err != nil {
					AbortWithRequestBodyReadFailure(c, "test", err)
					return
				}
				c.Status(204)
			})
			for _, n := range []int{limit, limit + 1} {
				var data bytes.Buffer
				if tc.gzip {
					w := gzip.NewWriter(&data)
					_, err := io.WriteString(w, strings.Repeat("x", n))
					require.NoError(t, err)
					require.NoError(t, w.Close())
				} else {
					data.WriteString(strings.Repeat("x", n))
				}
				req := httptest.NewRequest("POST", tc.path, &data)
				req.ContentLength = -1
				if tc.gzip {
					req.Header.Set("Content-Encoding", "gzip")
				}
				if tc.beta {
					req.Header.Set("anthropic-beta", "files-api-2025-04-14")
				}
				w := httptest.NewRecorder()
				r.ServeHTTP(w, req)
				if n == limit {
					require.Equal(t, 204, w.Code)
					continue
				}
				require.Equal(t, 413, w.Code)
				require.Equal(t, tc.beta, gjson.GetBytes(w.Body.Bytes(), "type").String() == "error")
			}
		})
	}
}

// net/http owns the final chunk. Exercise it over an in-memory connection so a
// recovery middleware that swallows ErrAbortHandler would produce a clean EOF.
func TestRecoveryAbortsChunkedStream(t *testing.T) {
	server, client := net.Pipe()
	defer client.Close()
	require.NoError(t, client.SetDeadline(time.Now().Add(5*time.Second)))
	r := gin.New()
	r.Use(Recovery())
	r.GET("/", func(c *gin.Context) {
		c.Writer.WriteHeader(200)
		_, _ = c.Writer.Write([]byte("audio"))
		c.Writer.Flush()
		panic(http.ErrAbortHandler)
	})
	srv := &http.Server{Handler: r}
	defer srv.Close()
	go func() { _ = srv.Serve(&deepseekPipeListener{conn: server}) }()
	_, err := io.WriteString(client, "GET / HTTP/1.1\r\nHost: test\r\n\r\n")
	require.NoError(t, err)
	resp, err := http.ReadResponse(bufio.NewReader(client), nil)
	require.NoError(t, err)
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	require.ErrorIs(t, err, io.ErrUnexpectedEOF)
	require.Equal(t, "audio", string(body))
}

type deepseekPipeListener struct{ conn net.Conn }

func (l *deepseekPipeListener) Accept() (net.Conn, error) {
	if l.conn == nil {
		return nil, net.ErrClosed
	}
	c := l.conn
	l.conn = nil
	return c, nil
}
func (*deepseekPipeListener) Close() error   { return nil }
func (*deepseekPipeListener) Addr() net.Addr { return &net.TCPAddr{} }
