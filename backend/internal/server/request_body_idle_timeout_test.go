//go:build unit

package server

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/handler"
	pkghttputil "github.com/Wei-Shaw/sub2api/internal/pkg/httputil"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

const testBodyIdleTimeout = 300 * time.Millisecond

// bodyProbeHandler reads the body through the shared gateway reader. pauseAfter
// bytes into the body it stops reading for pause, and after a successful read it
// waits postReadWait before checking that the request context is still alive.
func bodyProbeHandler(pauseAfter int64, pause, postReadWait time.Duration) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if pause > 0 {
			if _, err := io.CopyN(io.Discard, r.Body, pauseAfter); err != nil {
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
			time.Sleep(pause)
		}
		body, err := pkghttputil.ReadRequestBodyWithPrealloc(r)
		if err != nil {
			failure := pkghttputil.DescribeRequestBodyReadFailure(r, err)
			w.Header().Set("X-Body-Error", failure.Kind)
			w.WriteHeader(failure.Status)
			return
		}
		if postReadWait > 0 {
			time.Sleep(postReadWait)
			if r.Context().Err() != nil {
				w.WriteHeader(http.StatusServiceUnavailable)
				return
			}
		}
		if pause > 0 {
			body = append(make([]byte, pauseAfter), body...)
		}
		_, _ = io.WriteString(w, strconv.Itoa(len(body)))
	})
}

func newIdleTimeoutTestServer(t *testing.T, h http.Handler, h2c bool) (*httptest.Server, *http.Client) {
	t.Helper()
	ts := httptest.NewUnstartedServer(requestBodyIdleTimeoutHandler(h, testBodyIdleTimeout))
	transport := &http.Transport{}
	if h2c {
		protocols := new(http.Protocols)
		protocols.SetHTTP1(true)
		protocols.SetUnencryptedHTTP2(true)
		ts.Config.Protocols = protocols
		clientProtocols := new(http.Protocols)
		clientProtocols.SetUnencryptedHTTP2(true)
		transport.Protocols = clientProtocols
	}
	ts.Start()
	t.Cleanup(ts.Close)
	t.Cleanup(transport.CloseIdleConnections)
	return ts, &http.Client{Transport: transport, Timeout: 10 * time.Second}
}

func forEachProtocol(t *testing.T, fn func(t *testing.T, h2c bool)) {
	for _, h2c := range []bool{false, true} {
		name := "http1"
		if h2c {
			name = "h2c"
		}
		t.Run(name, func(t *testing.T) { fn(t, h2c) })
	}
}

// writeSlowly sends total bytes in pieces separated by gap, then closes.
func writeSlowly(w *io.PipeWriter, total, pieces int, gap time.Duration) {
	piece := bytes.Repeat([]byte("x"), total/pieces)
	for i := 0; i < pieces; i++ {
		if i > 0 {
			time.Sleep(gap)
		}
		if _, err := w.Write(piece); err != nil {
			return
		}
	}
	_ = w.Close()
}

func TestRequestBodyIdleTimeoutAllowsSlowProgressingUpload(t *testing.T) {
	forEachProtocol(t, func(t *testing.T, h2c bool) {
		ts, client := newIdleTimeoutTestServer(t, bodyProbeHandler(0, 0, 0), h2c)
		const total, pieces = 64 << 10, 8
		pr, pw := io.Pipe()
		// 8 pieces 100ms apart: the upload takes ~2.3x the idle timeout in
		// total, but no single gap reaches it.
		go writeSlowly(pw, total, pieces, testBodyIdleTimeout/3)
		req, err := http.NewRequest(http.MethodPost, ts.URL, pr)
		require.NoError(t, err)
		req.ContentLength = total

		start := time.Now()
		resp, err := client.Do(req)
		require.NoError(t, err)
		defer func() { _ = resp.Body.Close() }()
		got, _ := io.ReadAll(resp.Body)

		require.Equal(t, http.StatusOK, resp.StatusCode, resp.Header.Get("X-Body-Error"))
		require.Equal(t, strconv.Itoa(total), string(got))
		require.Greater(t, time.Since(start), testBodyIdleTimeout)
		if h2c {
			require.Equal(t, 2, resp.ProtoMajor)
		}
	})
}

func TestRequestBodyIdleTimeoutFailsStalledUpload(t *testing.T) {
	forEachProtocol(t, func(t *testing.T, h2c bool) {
		ts, client := newIdleTimeoutTestServer(t, bodyProbeHandler(0, 0, 0), h2c)
		pr, pw := io.Pipe()
		defer func() { _ = pw.Close() }()
		go func() { _, _ = pw.Write([]byte(`{"model":"x","messages":[`)) }()
		req, err := http.NewRequest(http.MethodPost, ts.URL, pr)
		require.NoError(t, err)
		req.ContentLength = 1 << 20

		start := time.Now()
		resp, err := client.Do(req)
		require.NoError(t, err)
		defer func() { _ = resp.Body.Close() }()

		require.Equal(t, http.StatusRequestTimeout, resp.StatusCode)
		require.Equal(t, pkghttputil.RequestBodyReadErrorTimeout, resp.Header.Get("X-Body-Error"))
		elapsed := time.Since(start)
		require.GreaterOrEqual(t, elapsed, testBodyIdleTimeout)
		require.Less(t, elapsed, 5*time.Second)
	})
}

// With less than 256 KiB of a declared body unread, net/http discards the
// remainder before writing the response. The expired deadline must stay in
// place so the 408 is sent at once instead of waiting on the stalled client.
func TestRequestBodyIdleTimeoutAnswersStalledSmallHTTP1Upload(t *testing.T) {
	ts, _ := newIdleTimeoutTestServer(t, bodyProbeHandler(0, 0, 0), false)
	conn, err := net.DialTimeout("tcp", ts.Listener.Addr().String(), time.Second)
	require.NoError(t, err)
	defer func() { _ = conn.Close() }()
	_ = conn.SetDeadline(time.Now().Add(5 * time.Second))
	_, err = io.WriteString(conn, "POST / HTTP/1.1\r\nHost: test\r\nContent-Length: 1000\r\n\r\nabc")
	require.NoError(t, err)

	start := time.Now()
	resp, err := http.ReadResponse(bufio.NewReader(conn), nil)
	require.NoError(t, err)
	_ = resp.Body.Close()

	require.Equal(t, http.StatusRequestTimeout, resp.StatusCode)
	require.Less(t, time.Since(start), 3*testBodyIdleTimeout)
	require.True(t, resp.Close)
}

// net/http drains up to 256 KiB of an unread HTTP/1 body before it writes the
// response. A handler that rejects a request without reading its stalled body,
// or after reading only part of it, must still answer within the idle timeout
// instead of never sending the response. HTTP/2 resets the stream instead.
func TestRequestBodyIdleTimeoutBoundsUnreadBodyDrain(t *testing.T) {
	for _, readFirst := range []int{0, 3} {
		reject := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if readFirst > 0 {
				if _, err := io.ReadFull(r.Body, make([]byte, readFirst)); err != nil {
					w.WriteHeader(http.StatusInternalServerError)
					return
				}
			}
			w.WriteHeader(http.StatusUnauthorized)
		})
		t.Run(fmt.Sprintf("read_%d", readFirst), func(t *testing.T) {
			forEachProtocol(t, func(t *testing.T, h2c bool) {
				ts, client := newIdleTimeoutTestServer(t, reject, h2c)
				start := time.Now()
				if h2c {
					pr, pw := io.Pipe()
					defer func() { _ = pw.Close() }()
					go func() { _, _ = pw.Write([]byte("abc")) }()
					req, err := http.NewRequest(http.MethodPost, ts.URL, pr)
					require.NoError(t, err)
					req.ContentLength = 1000
					resp, err := client.Do(req)
					require.NoError(t, err)
					_ = resp.Body.Close()
					require.Equal(t, http.StatusUnauthorized, resp.StatusCode)
					require.Less(t, time.Since(start), 3*testBodyIdleTimeout)
					return
				}
				conn, err := net.DialTimeout("tcp", ts.Listener.Addr().String(), time.Second)
				require.NoError(t, err)
				defer func() { _ = conn.Close() }()
				_ = conn.SetDeadline(time.Now().Add(5 * time.Second))
				_, err = io.WriteString(conn, "POST / HTTP/1.1\r\nHost: test\r\nContent-Length: 1000\r\n\r\nabc")
				require.NoError(t, err)
				resp, err := http.ReadResponse(bufio.NewReader(conn), nil)
				require.NoError(t, err)
				_ = resp.Body.Close()
				require.Equal(t, http.StatusUnauthorized, resp.StatusCode)
				require.Less(t, time.Since(start), 3*testBodyIdleTimeout)
				require.True(t, resp.Close)
			})
		})
	}
}

// A client that disconnects mid-upload is reported as 499, not as a malformed
// request, using the real net/http error and context behavior.
func TestRequestBodyReadClassifiesHTTP1ClientDisconnect(t *testing.T) {
	kinds := make(chan string, 1)
	ts, _ := newIdleTimeoutTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, err := pkghttputil.ReadRequestBodyWithPrealloc(r)
		kinds <- pkghttputil.DescribeRequestBodyReadFailure(r, err).Kind
	}), false)
	conn, err := net.DialTimeout("tcp", ts.Listener.Addr().String(), time.Second)
	require.NoError(t, err)
	_, err = io.WriteString(conn, "POST / HTTP/1.1\r\nHost: test\r\nContent-Length: 1000\r\n\r\nabc")
	require.NoError(t, err)
	time.Sleep(50 * time.Millisecond)
	require.NoError(t, conn.Close())

	select {
	case kind := <-kinds:
		require.Equal(t, pkghttputil.RequestBodyReadErrorClientDisconnect, kind)
	case <-time.After(5 * time.Second):
		t.Fatal("handler did not observe the disconnect")
	}
}

// Time the handler spends between reads is not client idleness: the deadline
// is armed only during a Read. The second half arrives while the handler is
// paused but after one idle timeout, so a deadline left armed from the first
// read would fire (an HTTP/2 stream timer runs even without a pending Read).
// A completed body must also leave the request context alive.
func TestRequestBodyIdleTimeoutIgnoresHandlerPauses(t *testing.T) {
	forEachProtocol(t, func(t *testing.T, h2c bool) {
		const total = 32 << 10
		probe := bodyProbeHandler(total/2, 3*testBodyIdleTimeout, 2*testBodyIdleTimeout)
		ts, client := newIdleTimeoutTestServer(t, probe, h2c)
		pr, pw := io.Pipe()
		go writeSlowly(pw, total, 2, 3*testBodyIdleTimeout/2)
		req, err := http.NewRequest(http.MethodPost, ts.URL, pr)
		require.NoError(t, err)
		req.ContentLength = total

		resp, err := client.Do(req)
		require.NoError(t, err)
		defer func() { _ = resp.Body.Close() }()
		got, _ := io.ReadAll(resp.Body)

		require.Equal(t, http.StatusOK, resp.StatusCode, resp.Header.Get("X-Body-Error"))
		require.Equal(t, strconv.Itoa(total), string(got))
	})
}

func TestRequestBodyIdleTimeoutFallsBackWithoutDeadlineSupport(t *testing.T) {
	h := requestBodyIdleTimeoutHandler(bodyProbeHandler(0, 0, 0), time.Nanosecond)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/", strings.NewReader("hello")))
	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, "5", rec.Body.String())
}

func TestRequestBodyIdleTimeoutDisabled(t *testing.T) {
	next := http.NotFoundHandler()
	require.Equal(t, fmt.Sprintf("%p", next), fmt.Sprintf("%p", requestBodyIdleTimeoutHandler(next, 0)))
}

// End to end over a real HTTP/1 connection: a stalled Anthropic /v1/messages
// upload is answered with 408 after the configured idle timeout, and the
// connection is closed rather than reused.
func TestHTTPServerRequestBodyIdleTimeoutAnthropicMessages(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	groupID := int64(1)
	r.POST("/v1/messages", func(c *gin.Context) {
		c.Set(string(middleware.ContextKeyAPIKey), &service.APIKey{ID: 1, UserID: 1, GroupID: &groupID})
		c.Set(string(middleware.ContextKeyUser), middleware.AuthSubject{UserID: 1, Concurrency: 1})
		c.Next()
	}, (&handler.GatewayHandler{}).Messages)
	cfg := ingressTestConfig()
	cfg.Server.MaxRequestBodySize = 0
	cfg.Gateway.MaxBodySize = 0
	cfg.Server.RequestBodyIdleTimeout = 1
	addr, stop := serveIngressTestServer(t, ProvideHTTPServer(cfg, r))
	defer stop()

	conn, err := net.DialTimeout("tcp", addr, time.Second)
	require.NoError(t, err)
	defer func() { _ = conn.Close() }()
	_ = conn.SetDeadline(time.Now().Add(10 * time.Second))
	partial := `{"model":"claude-sonnet-4-5","max_tokens":1,"messages":[`
	_, err = io.WriteString(conn, "POST /v1/messages HTTP/1.1\r\nHost: test\r\nContent-Type: application/json\r\nContent-Length: 1048576\r\n\r\n"+partial)
	require.NoError(t, err)

	start := time.Now()
	reader := bufio.NewReader(conn)
	resp, err := http.ReadResponse(reader, nil)
	require.NoError(t, err)
	body, _ := io.ReadAll(resp.Body)
	_ = resp.Body.Close()

	require.Equal(t, http.StatusRequestTimeout, resp.StatusCode)
	elapsed := time.Since(start)
	require.GreaterOrEqual(t, elapsed, 900*time.Millisecond)
	require.Less(t, elapsed, 5*time.Second)
	var payload struct {
		Error struct {
			Type    string `json:"type"`
			Message string `json:"message"`
		} `json:"error"`
	}
	require.NoError(t, json.Unmarshal(body, &payload))
	require.Equal(t, "invalid_request_error", payload.Error.Type)
	require.Equal(t, "Timed out reading request body", payload.Error.Message)
	require.True(t, resp.Close, "connection with an unread body must not be reused")
	_, err = reader.ReadByte()
	require.ErrorIs(t, err, io.EOF)
}
