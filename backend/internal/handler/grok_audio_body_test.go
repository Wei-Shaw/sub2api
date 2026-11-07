package handler

import (
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"testing/iotest"

	"github.com/Wei-Shaw/sub2api/internal/config"
	pkghttputil "github.com/Wei-Shaw/sub2api/internal/pkg/httputil"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestGrokVoiceRequestBodyMemoryOverload(t *testing.T) {
	cfg := &config.Config{RunMode: config.RunModeSimple}
	billing := service.NewBillingCacheService(nil, nil, nil, nil, nil, nil, cfg, nil)
	t.Cleanup(billing.Stop)
	h := &OpenAIGatewayHandler{
		gatewayService:      &service.OpenAIGatewayService{},
		billingCacheService: billing,
		apiKeyService:       &service.APIKeyService{},
		concurrencyHelper:   &ConcurrencyHelper{concurrencyService: service.NewConcurrencyService(nil)},
		cfg:                 cfg,
	}
	for _, endpoint := range []string{"tts", "stt", "custom-voices"} {
		t.Run(endpoint, func(t *testing.T) {
			router := gin.New()
			router.Use(middleware.RequestBodyLimit(1024, pkghttputil.NewBodyMemoryBudget(1)))
			router.Use(func(c *gin.Context) {
				c.Set(string(middleware.ContextKeyAPIKey), &service.APIKey{
					Group: &service.Group{Platform: service.PlatformGrok},
					User:  &service.User{ID: 1},
				})
				c.Next()
			})
			// A normal Grok group without an allowlist reaches the handler before
			// any middleware reads its body.
			router.Use(middleware.GroupModelAllowlist())
			router.POST("/v1/"+endpoint, func(c *gin.Context) { h.GrokVoice(c, endpoint) })
			req := httptest.NewRequest(http.MethodPost, "/v1/"+endpoint, strings.NewReader(`{"input":"hello"}`))
			req.Header.Set("Content-Type", "application/json")
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, req)

			require.Equal(t, http.StatusServiceUnavailable, rec.Code, rec.Body.String())
			require.Equal(t, "1", rec.Header().Get("Retry-After"))
			require.Contains(t, rec.Body.String(), "Request body buffering capacity is exhausted")
		})
	}
}

// Read failures other than the memory budget go through the shared classifier:
// a timeout is a 408 with a fixed message, never the raw socket error, and a
// missing body keeps its validation message.
func TestGrokVoiceRequestBodyReadFailures(t *testing.T) {
	cfg := &config.Config{RunMode: config.RunModeSimple}
	billing := service.NewBillingCacheService(nil, nil, nil, nil, nil, nil, cfg, nil)
	t.Cleanup(billing.Stop)
	h := &OpenAIGatewayHandler{
		gatewayService:      &service.OpenAIGatewayService{},
		billingCacheService: billing,
		apiKeyService:       &service.APIKeyService{},
		concurrencyHelper:   &ConcurrencyHelper{concurrencyService: service.NewConcurrencyService(nil)},
		cfg:                 cfg,
	}
	timeoutErr := &net.OpError{Op: "read", Net: "tcp", Source: &net.TCPAddr{IP: net.IPv4(10, 0, 0, 1), Port: 8080}, Err: os.ErrDeadlineExceeded}
	for _, tc := range []struct {
		name       string
		body       io.Reader
		wantStatus int
		wantText   string
	}{
		{"timeout", iotest.ErrReader(timeoutErr), http.StatusRequestTimeout, "Timed out reading request body"},
		{"truncated", iotest.ErrReader(io.ErrUnexpectedEOF), http.StatusBadRequest, "Failed to read request body"},
		{"missing", nil, http.StatusBadRequest, "request body is required"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			router := gin.New()
			router.Use(func(c *gin.Context) {
				c.Set(string(middleware.ContextKeyAPIKey), &service.APIKey{
					Group: &service.Group{Platform: service.PlatformGrok},
					User:  &service.User{ID: 1},
				})
				c.Next()
			})
			router.POST("/v1/tts", func(c *gin.Context) { h.GrokVoice(c, "tts") })
			req := httptest.NewRequest(http.MethodPost, "/v1/tts", nil)
			if tc.body != nil {
				req.Body = io.NopCloser(tc.body)
			} else {
				req.Body = nil
			}
			req.Header.Set("Content-Type", "application/json")
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, req)

			require.Equal(t, tc.wantStatus, rec.Code, rec.Body.String())
			require.Contains(t, rec.Body.String(), tc.wantText)
			require.NotContains(t, rec.Body.String(), "10.0.0.1")
		})
	}
}
