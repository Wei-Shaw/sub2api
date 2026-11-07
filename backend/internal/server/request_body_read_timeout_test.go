//go:build unit

package server

import (
	"bufio"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/handler"
	pkghttputil "github.com/Wei-Shaw/sub2api/internal/pkg/httputil"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

const testBodyReadTimeout = 300 * time.Millisecond

// bodyKindHandler reads the body through the shared gateway reader and reports
// the classified failure kind and status.
func bodyKindHandler(kinds chan<- string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := pkghttputil.ReadRequestBodyWithPrealloc(r)
		if err != nil {
			failure := pkghttputil.DescribeRequestBodyReadFailure(r, err)
			if kinds != nil {
				kinds <- failure.Kind
			}
			w.Header().Set("X-Body-Error", failure.Kind)
			w.WriteHeader(failure.Status)
			return
		}
		_, _ = io.WriteString(w, strconv.Itoa(len(body)))
	})
}

func newBodyReadTimeoutTestServer(t *testing.T, h http.Handler, h2c bool) (*httptest.Server, *http.Client) {
	t.Helper()
	ts := httptest.NewUnstartedServer(pkghttputil.WithBodyReadTimeout(h, testBodyReadTimeout))
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

// A stalled upload is classified as a read timeout (408) over both protocols.
// On HTTP/1 the deadline also cancels the request context; the timeout must
// still win over the client-disconnect classification.
func TestBodyReadTimeoutClassifiedAsTimeout(t *testing.T) {
	for _, h2c := range []bool{false, true} {
		name := "http1"
		if h2c {
			name = "h2c"
		}
		t.Run(name, func(t *testing.T) {
			ts, client := newBodyReadTimeoutTestServer(t, bodyKindHandler(nil), h2c)
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
			require.GreaterOrEqual(t, elapsed, testBodyReadTimeout)
			require.Less(t, elapsed, 5*time.Second)
		})
	}
}

// A client that disconnects mid-upload is still classified as a disconnect,
// using the real net/http error and context behavior.
func TestBodyReadClassifiesHTTP1ClientDisconnect(t *testing.T) {
	kinds := make(chan string, 1)
	ts, _ := newBodyReadTimeoutTestServer(t, bodyKindHandler(kinds), false)
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

// End to end over a real HTTP/1 connection: a stalled Anthropic /v1/messages
// upload is answered with an Anthropic-format 408 after the configured body
// read timeout instead of an empty 499 or a generic 400.
func TestHTTPServerBodyReadTimeoutAnthropicMessages(t *testing.T) {
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
	cfg.Gateway.RequestBodyReadTimeoutSeconds = 1
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
}
