//go:build unit

package routes

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
)

// countingReader 记录处理链实际从请求体读取的字节数。
type countingReader struct {
	r    io.Reader
	read int64
}

func (c *countingReader) Read(p []byte) (int, error) {
	n, err := c.r.Read(p)
	c.read += int64(n)
	return n, err
}

func newAuthRoutesMiniredisRouter(t *testing.T) http.Handler {
	t.Helper()
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })
	return newAuthRoutesTestRouter(rdb)
}

func TestAuthRoutesLimitRequestBodySize(t *testing.T) {
	router := newAuthRoutesMiniredisRouter(t)

	// 8MB 空白：没有上限时 JSON 解码会把整段读完；有上限时读到 1MB+1 即中止。
	body := &countingReader{r: bytes.NewReader(bytes.Repeat([]byte(" "), 8<<20))}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/logout", body)
	req.Header.Set("Content-Type", "application/json")
	req.RemoteAddr = "203.0.113.20:12345"

	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code, "logout tolerates an unreadable body")
	require.LessOrEqual(t, body.read, int64(1<<20)+1)
}

func TestAuthRoutesLogoutRateLimited(t *testing.T) {
	router := newAuthRoutesMiniredisRouter(t)

	send := func() *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/logout", strings.NewReader(`{}`))
		req.Header.Set("Content-Type", "application/json")
		req.RemoteAddr = "203.0.113.21:12345"
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		return w
	}

	for i := 0; i < 30; i++ {
		require.Equal(t, http.StatusOK, send().Code, "request %d", i+1)
	}
	w := send()
	require.Equal(t, http.StatusTooManyRequests, w.Code)
	require.Contains(t, w.Body.String(), "rate limit exceeded")
}
