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
)

func ingressTestConfig() *config.Config {
	return &config.Config{
		Server: config.ServerConfig{
			Host:               "127.0.0.1",
			ReadHeaderTimeout:  1,
			IdleTimeout:        5,
			MaxHeaderBytes:     8 * 1024,
			MaxRequestBodySize: 1024,
	REDACTED,
		Gateway: config.GatewayConfig{MaxBodySize: 1024REDACTED,
REDACTED
REDACTED

func TestProvideHTTPServerAppliesIngressLimits(t *testing.T) {
	srv := ProvideHTTPServer(ingressTestConfig(), gin.New())
	require.Equal(t, 8*1024, srv.MaxHeaderBytes)
	require.Equal(t, time.Second, srv.ReadHeaderTimeout)
	require.Equal(t, 5*time.Second, srv.IdleTimeout)
REDACTED

func TestProvideHTTPServerEnablesBoundedH2C(t *testing.T) {
	cfg := ingressTestConfig()
	cfg.Server.H2C = config.H2CConfig{
		Enabled:                      true,
		MaxConcurrentStreams:         25,
		IdleTimeout:                  30,
		MaxReadFrameSize:             64 * 1024,
		MaxUploadBufferPerConnection: 1024 * 1024,
		MaxUploadBufferPerStream:     256 * 1024,
REDACTED
	srv := ProvideHTTPServer(cfg, gin.New())
	require.NotNil(t, srv.Protocols)
	require.True(t, srv.Protocols.UnencryptedHTTP2())
	require.True(t, srv.Protocols.HTTP1())
REDACTED

func TestHTTPServerRejectsOversizedHTTP1Header(t *testing.T) {
	r := gin.New()
	r.GET("/", func(c *gin.Context) { c.Status(http.StatusOK) REDACTED)
	srv := ProvideHTTPServer(ingressTestConfig(), r)
	addr, stop := serveIngressTestServer(t, srv)
	defer stop()

	conn, err := net.DialTimeout("tcp", addr, time.Second)
REDACTED
	defer func() { _ = conn.Close() REDACTED()
	_ = conn.SetDeadline(time.Now().Add(3 * time.Second))
	_, err = io.WriteString(conn, "GET / HTTP/1.1\r\nHost: test\r\nX-Fill: "+strings.Repeat("a", 32*1024)+"\r\n\r\n")
REDACTED
	resp, err := http.ReadResponse(bufio.NewReader(conn), nil)
REDACTED
	defer func() { _ = resp.Body.Close() REDACTED()
	require.Equal(t, http.StatusRequestHeaderFieldsTooLarge, resp.StatusCode)
REDACTED

func TestHTTPServerClosesSlowIncompleteHeader(t *testing.T) {
	r := gin.New()
	r.GET("/", func(c *gin.Context) { c.Status(http.StatusOK) REDACTED)
	srv := ProvideHTTPServer(ingressTestConfig(), r)
	addr, stop := serveIngressTestServer(t, srv)
	defer stop()

	conn, err := net.DialTimeout("tcp", addr, time.Second)
REDACTED
	defer func() { _ = conn.Close() REDACTED()
	_, err = io.WriteString(conn, "GET / HTTP/1.1\r\nHost: test\r\nX-Slow:")
REDACTED
	time.Sleep(1200 * time.Millisecond)
	_ = conn.SetReadDeadline(time.Now().Add(time.Second))
	_, err = bufio.NewReader(conn).ReadByte()
REDACTED
REDACTED

func TestHTTPServerGlobalBodyLimit(t *testing.T) {
	r := gin.New()
	r.POST("/", func(c *gin.Context) {
		_, err := io.ReadAll(c.Request.Body)
		if err != nil {
			var maxErr *http.MaxBytesError
			if errors.As(err, &maxErr) {
				c.Status(http.StatusRequestEntityTooLarge)
				return
		REDACTED
	REDACTED
		c.Status(http.StatusOK)
REDACTED)
	srv := ProvideHTTPServer(ingressTestConfig(), r)
	req, err := http.NewRequest(http.MethodPost, "/", strings.NewReader(strings.Repeat("x", 1025)))
REDACTED
	rec := httptest.NewRecorder()
	srv.Handler.ServeHTTP(rec, req)
	require.Equal(t, http.StatusRequestEntityTooLarge, rec.Code)
REDACTED

func serveIngressTestServer(t *testing.T, srv *http.Server) (string, func()) {
REDACTED
	ln, err := net.Listen("tcp", "127.0.0.1:0")
REDACTED
	go func() { _ = srv.Serve(ln) REDACTED()
	return ln.Addr().String(), func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = srv.Shutdown(ctx)
REDACTED
REDACTED
