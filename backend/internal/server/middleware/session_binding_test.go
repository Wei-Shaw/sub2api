//go:build unit

package middleware

import (
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestSessionBindingContextDoesNotTrustHeadersWithoutTrustedProxy(t *testing.T) {
	gin.SetMode(gin.TestMode)

	for _, tc := range []struct {
		name           string
		trustForwarded bool
		wantIP         string
REDACTED{
		{name: "trust disabled records proxy address", trustForwarded: false, wantIP: "127.0.0.1"REDACTED,
		{name: "legacy trust toggle cannot bypass trusted proxies", trustForwarded: true, wantIP: "127.0.0.1"REDACTED,
REDACTED {
		t.Run(tc.name, func(t *testing.T) {
			cfg := &config.Config{REDACTED
			cfg.SetTrustForwardedIPForAPIKeyACL(tc.trustForwarded)

			r := gin.New()
			require.NoError(t, r.SetTrustedProxies(nil))
			r.Use(SessionBindingContext(cfg))
			r.GET("/t", func(c *gin.Context) {
				binding := service.SessionBindingFromContext(c.Request.Context())
				require.NotNil(t, binding)
				require.Equal(t, tc.wantIP, binding.IP)
				require.Equal(t, "test-agent", binding.UserAgent)
				require.Equal(t, tc.wantIP, SecurityClientIP(c))
				c.Status(200)
		REDACTED)

			w := httptest.NewRecorder()
			req := httptest.NewRequest("GET", "/t", nil)
			req.RemoteAddr = "127.0.0.1:54321"
			req.Header.Set("X-Real-IP", "1.2.3.4")
			req.Header.Set("User-Agent", "test-agent")
			r.ServeHTTP(w, req)

			require.Equal(t, 200, w.Code)
	REDACTED)
REDACTED
REDACTED

func TestSessionBindingContextBoundsPersistedUserAgent(t *testing.T) {
	cfg := &config.Config{REDACTED
	r := gin.New()
	r.Use(SessionBindingContext(cfg))
	r.GET("/t", func(c *gin.Context) {
		binding := service.SessionBindingFromContext(c.Request.Context())
		require.Len(t, binding.UserAgent, maxPersistentUserAgentBytes)
		require.Equal(t, binding.UserAgent, c.Request.UserAgent())
		c.Status(200)
REDACTED)
	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/t", nil)
	req.Header.Set("User-Agent", strings.Repeat("u", 2048))
	r.ServeHTTP(w, req)
	require.Equal(t, 200, w.Code)
REDACTED

// 未经过 SessionBindingContext 注入时（异常挂载顺序/单测直调），回退 trusted_proxies 链，
// 等价于开关关闭时的历史行为。
func TestSecurityClientIPFallsBackWithoutInjectedBinding(t *testing.T) {
	gin.SetMode(gin.TestMode)

	r := gin.New()
	require.NoError(t, r.SetTrustedProxies(nil))
	r.GET("/t", func(c *gin.Context) {
		c.String(200, SecurityClientIP(c))
REDACTED)

	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/t", nil)
	req.RemoteAddr = "9.9.9.9:12345"
	req.Header.Set("X-Real-IP", "1.2.3.4")
	r.ServeHTTP(w, req)

	require.Equal(t, 200, w.Code)
	require.Equal(t, "9.9.9.9", w.Body.String())
REDACTED

func TestRequestSessionBindingPrefersInjectedBinding(t *testing.T) {
	gin.SetMode(gin.TestMode)

	cfg := &config.Config{REDACTED
	cfg.SetTrustForwardedIPForAPIKeyACL(true)

	r := gin.New()
	require.NoError(t, r.SetTrustedProxies([]string{"127.0.0.1"REDACTED))
	r.Use(SessionBindingContext(cfg))
	r.GET("/t", func(c *gin.Context) {
		issued := &service.SessionBinding{IP: "1.2.3.4", UserAgent: "test-agent"REDACTED
		require.Equal(t, issued.Hash(), requestSessionBinding(c).Hash())
		c.Status(200)
REDACTED)

	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/t", nil)
	req.RemoteAddr = "127.0.0.1:54321"
	req.Header.Set("X-Real-IP", "1.2.3.4")
	req.Header.Set("User-Agent", "test-agent")
	r.ServeHTTP(w, req)

	require.Equal(t, 200, w.Code)
REDACTED
