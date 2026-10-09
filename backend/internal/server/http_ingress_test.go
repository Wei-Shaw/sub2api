//go:build unit

package server

import (
	"bufio"
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"golang.org/x/net/http2"
)

func ingressTestConfig() *config.Config {
	return &config.Config{
		Server: config.ServerConfig{
			Host:               "127.0.0.1",
			ReadHeaderTimeout:  1,
			IdleTimeout:        5,
			MaxHeaderBytes:     8 * 1024,
			MaxRequestBodySize: 1024,
		},
		Gateway: config.GatewayConfig{MaxBodySize: 1024},
	}
}

func TestProvideHTTPServerAppliesIngressLimits(t *testing.T) {
	srv := ProvideHTTPServer(ingressTestConfig(), gin.New())
	require.Equal(t, 8*1024, srv.MaxHeaderBytes)
	require.Equal(t, time.Second, srv.ReadHeaderTimeout)
	require.Equal(t, 5*time.Second, srv.IdleTimeout)
	require.Nil(t, srv.Protocols)
	require.Nil(t, srv.HTTP2)
}

func TestProvideHTTPServerEnablesBoundedH2C(t *testing.T) {
	cfg := ingressTestConfig()
	cfg.Server.H2C = config.H2CConfig{
		Enabled:                      true,
		MaxConcurrentStreams:         25,
		IdleTimeout:                  30,
		MaxReadFrameSize:             64 * 1024,
		MaxUploadBufferPerConnection: 1024 * 1024,
		MaxUploadBufferPerStream:     256 * 1024,
	}
	srv := ProvideHTTPServer(cfg, gin.New())
	require.NotNil(t, srv.Protocols)
	require.True(t, srv.Protocols.UnencryptedHTTP2())
	require.True(t, srv.Protocols.HTTP1())
	require.False(t, srv.Protocols.HTTP2())
	require.Equal(t, 5*time.Second, srv.IdleTimeout)
	require.Nil(t, srv.HTTP2)

	addr, stop := serveIngressTestServer(t, srv)
	defer stop()
	conn, err := net.DialTimeout("tcp", addr, time.Second)
	require.NoError(t, err)
	defer func() { _ = conn.Close() }()
	require.NoError(t, conn.SetDeadline(time.Now().Add(3*time.Second)))
	_, err = io.WriteString(conn, http2.ClientPreface)
	require.NoError(t, err)
	framer := http2.NewFramer(conn, conn)
	require.NoError(t, framer.WriteSettings())
	var gotSettings, gotWindow bool
	for !gotSettings || !gotWindow {
		frame, err := framer.ReadFrame()
		require.NoError(t, err)
		switch frame := frame.(type) {
		case *http2.SettingsFrame:
			if frame.IsAck() {
				continue
			}
			for setting, want := range map[http2.SettingID]uint32{
				http2.SettingMaxConcurrentStreams: 25,
				http2.SettingMaxFrameSize:         64 * 1024,
				http2.SettingInitialWindowSize:    256 * 1024,
			} {
				value, ok := frame.Value(setting)
				require.True(t, ok, "missing setting %v", setting)
				require.Equal(t, want, value, "setting %v", setting)
			}
			require.NoError(t, framer.WriteSettingsAck())
			gotSettings = true
		case *http2.WindowUpdateFrame:
			require.Zero(t, frame.StreamID)
			require.Equal(t, uint32(1024*1024-65535), frame.Increment)
			gotWindow = true
		}
	}
}

func TestProvideHTTPServerH2CIdleTimeoutFallsBackToServer(t *testing.T) {
	cfg := ingressTestConfig()
	cfg.Server.H2C.Enabled = true
	srv := ProvideHTTPServer(cfg, gin.New())
	require.Equal(t, 5*time.Second, srv.IdleTimeout)
}

func TestHTTPServerH2CPreservesPriorKnowledgeAndHTTP1(t *testing.T) {
	cfg := ingressTestConfig()
	cfg.Server.H2C.Enabled = true
	cfg.Server.H2C.IdleTimeout = 30
	router := gin.New()
	router.GET("/", func(c *gin.Context) { c.Status(http.StatusOK) })
	addr, stop := serveIngressTestServer(t, ProvideHTTPServer(cfg, router))
	defer stop()

	for _, wantMajor := range []int{1, 2} {
		protocols := new(http.Protocols)
		protocols.SetHTTP1(wantMajor == 1)
		protocols.SetUnencryptedHTTP2(wantMajor == 2)
		transport := &http.Transport{Protocols: protocols}
		client := &http.Client{Transport: transport, Timeout: 3 * time.Second}
		req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, "http://"+addr+"/", nil)
		require.NoError(t, err)
		resp, err := client.Do(req)
		require.NoError(t, err)
		require.NoError(t, resp.Body.Close())
		transport.CloseIdleConnections()
		require.Equal(t, http.StatusOK, resp.StatusCode)
		require.Equal(t, wantMajor, resp.ProtoMajor)
	}

	conn, err := net.DialTimeout("tcp", addr, time.Second)
	require.NoError(t, err)
	defer func() { _ = conn.Close() }()
	require.NoError(t, conn.SetDeadline(time.Now().Add(3*time.Second)))
	_, err = io.WriteString(conn, "GET / HTTP/1.1\r\nHost: test\r\nConnection: Upgrade, HTTP2-Settings\r\nUpgrade: h2c\r\nHTTP2-Settings: AAMAAABk\r\n\r\n")
	require.NoError(t, err)
	resp, err := http.ReadResponse(bufio.NewReader(conn), nil)
	require.NoError(t, err)
	require.NoError(t, resp.Body.Close())
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.Equal(t, 1, resp.ProtoMajor)
}

func TestHTTPServerH2CIdleTimeoutPreservesHTTP1Connection(t *testing.T) {
	cfg := ingressTestConfig()
	cfg.Server.H2C.Enabled = true
	cfg.Server.H2C.IdleTimeout = 1
	router := gin.New()
	router.GET("/", func(c *gin.Context) { c.Status(http.StatusOK) })
	srv := ProvideHTTPServer(cfg, router)
	require.Equal(t, 5*time.Second, srv.IdleTimeout)
	addr, stop := serveIngressTestServer(t, srv)
	defer stop()

	http1Conn, err := net.DialTimeout("tcp", addr, time.Second)
	require.NoError(t, err)
	defer func() { _ = http1Conn.Close() }()
	require.NoError(t, http1Conn.SetDeadline(time.Now().Add(4*time.Second)))
	reader := bufio.NewReader(http1Conn)
	requestHTTP1 := func() {
		_, err := io.WriteString(http1Conn, "GET / HTTP/1.1\r\nHost: test\r\n\r\n")
		require.NoError(t, err)
		resp, err := http.ReadResponse(reader, nil)
		require.NoError(t, err)
		require.NoError(t, resp.Body.Close())
		require.Equal(t, http.StatusOK, resp.StatusCode)
		require.Equal(t, 1, resp.ProtoMajor)
	}
	requestHTTP1()

	http2Conn, err := net.DialTimeout("tcp", addr, time.Second)
	require.NoError(t, err)
	defer func() { _ = http2Conn.Close() }()
	require.NoError(t, http2Conn.SetDeadline(time.Now().Add(3*time.Second)))
	_, err = io.WriteString(http2Conn, http2.ClientPreface)
	require.NoError(t, err)
	framer := http2.NewFramer(http2Conn, http2Conn)
	require.NoError(t, framer.WriteSettings())
	for {
		frame, err := framer.ReadFrame()
		require.NoError(t, err)
		if settings, ok := frame.(*http2.SettingsFrame); ok && !settings.IsAck() {
			require.NoError(t, framer.WriteSettingsAck())
		}
		if goAway, ok := frame.(*http2.GoAwayFrame); ok {
			require.Equal(t, http2.ErrCodeNo, goAway.ErrCode)
			break
		}
	}
	requestHTTP1()
}

func TestConfigureTrustedProxies(t *testing.T) {
	gin.SetMode(gin.TestMode)
	tests := []struct {
		name string
		cfg  config.ServerConfig
		want string
	}{
		{
			name: "configured proxy resolves forwarded client",
			cfg: config.ServerConfig{
				TrustedProxies:           []string{"9.9.9.9/32"},
				TrustedProxiesConfigured: true,
			},
			want: "1.2.3.4",
		},
		{
			name: "explicit empty list ignores forwarded client",
			cfg: config.ServerConfig{
				TrustedProxiesConfigured: true,
			},
			want: "9.9.9.9",
		},
		{
			name: "invalid proxy list fails closed",
			cfg: config.ServerConfig{
				TrustedProxies:           []string{"not-a-cidr"},
				TrustedProxiesConfigured: true,
			},
			want: "9.9.9.9",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			r := gin.New()
			configureTrustedProxies(r, tc.cfg)
			r.GET("/t", func(c *gin.Context) { c.String(http.StatusOK, c.ClientIP()) })

			w := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, "/t", nil)
			req.RemoteAddr = "9.9.9.9:12345"
			req.Header.Set("X-Forwarded-For", "1.2.3.4")
			r.ServeHTTP(w, req)

			require.Equal(t, http.StatusOK, w.Code)
			require.Equal(t, tc.want, w.Body.String())
		})
	}
}

func TestHTTPServerRejectsOversizedHTTP1Header(t *testing.T) {
	r := gin.New()
	r.GET("/", func(c *gin.Context) { c.Status(http.StatusOK) })
	srv := ProvideHTTPServer(ingressTestConfig(), r)
	addr, stop := serveIngressTestServer(t, srv)
	defer stop()

	conn, err := net.DialTimeout("tcp", addr, time.Second)
	require.NoError(t, err)
	defer func() { _ = conn.Close() }()
	_ = conn.SetDeadline(time.Now().Add(3 * time.Second))
	_, err = io.WriteString(conn, "GET / HTTP/1.1\r\nHost: test\r\nX-Fill: "+strings.Repeat("a", 32*1024)+"\r\n\r\n")
	require.NoError(t, err)
	resp, err := http.ReadResponse(bufio.NewReader(conn), nil)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()
	require.Equal(t, http.StatusRequestHeaderFieldsTooLarge, resp.StatusCode)
}

func TestHTTPServerClosesSlowIncompleteHeader(t *testing.T) {
	r := gin.New()
	r.GET("/", func(c *gin.Context) { c.Status(http.StatusOK) })
	srv := ProvideHTTPServer(ingressTestConfig(), r)
	addr, stop := serveIngressTestServer(t, srv)
	defer stop()

	conn, err := net.DialTimeout("tcp", addr, time.Second)
	require.NoError(t, err)
	defer func() { _ = conn.Close() }()
	_, err = io.WriteString(conn, "GET / HTTP/1.1\r\nHost: test\r\nX-Slow:")
	require.NoError(t, err)
	time.Sleep(1200 * time.Millisecond)
	_ = conn.SetReadDeadline(time.Now().Add(time.Second))
	_, err = bufio.NewReader(conn).ReadByte()
	require.Error(t, err)
}

func TestHTTPServerGlobalBodyLimit(t *testing.T) {
	r := gin.New()
	r.POST("/", func(c *gin.Context) {
		_, err := io.ReadAll(c.Request.Body)
		if err != nil {
			var maxErr *http.MaxBytesError
			if errors.As(err, &maxErr) {
				c.Status(http.StatusRequestEntityTooLarge)
				return
			}
		}
		c.Status(http.StatusOK)
	})
	srv := ProvideHTTPServer(ingressTestConfig(), r)
	req, err := http.NewRequest(http.MethodPost, "/", strings.NewReader(strings.Repeat("x", 1025)))
	require.NoError(t, err)
	rec := httptest.NewRecorder()
	srv.Handler.ServeHTTP(rec, req)
	require.Equal(t, http.StatusRequestEntityTooLarge, rec.Code)
}

func serveIngressTestServer(t *testing.T, srv *http.Server) (string, func()) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	go func() { _ = srv.Serve(ln) }()
	return ln.Addr().String(), func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = srv.Shutdown(ctx)
	}
}
