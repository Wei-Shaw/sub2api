//go:build unit

package plugin

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
)

func newAuthCtx(t *testing.T, fn func(c *gin.Context)) *gin.Context {
	t.Helper()
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/x", nil)
	c.Request.Header.Set("X-Forwarded-For", "203.0.113.42")
	if fn != nil {
		fn(c)
	}
	return c
}

func TestInjectRequestContext_FullAuthCtx(t *testing.T) {
	authCtx := newAuthCtx(t, func(c *gin.Context) {
		c.Set(string(middleware.ContextKeyUser), middleware.AuthSubject{UserID: 7})
		c.Set(string(middleware.ContextKeyUserRole), "admin")
		c.Set(string(middleware.ContextKeyAPIKey), &service.APIKey{ID: 1234})
	})

	req := httptest.NewRequest(http.MethodGet, "/plugin/x", nil)
	injectRequestContext(req, authCtx, "channel-management")

	if got := req.Header.Get(HeaderPluginUserID); got != "7" {
		t.Fatalf("user id = %q", got)
	}
	if got := req.Header.Get(HeaderPluginUserRole); got != "admin" {
		t.Fatalf("role = %q", got)
	}
	if got := req.Header.Get(HeaderPluginAPIKeyID); got != "1234" {
		t.Fatalf("api key id = %q", got)
	}
	if got := req.Header.Get(HeaderPluginName); got != "channel-management" {
		t.Fatalf("plugin name = %q", got)
	}
	if got := req.Header.Get(HeaderPluginRequestID); got == "" {
		t.Fatalf("request id should be auto-generated")
	}
	tp := req.Header.Get(HeaderTraceparent)
	if tp == "" || !isValidTraceparent(tp) {
		t.Fatalf("traceparent should be valid, got %q", tp)
	}
	if ip := req.Header.Get(HeaderPluginClientIP); ip == "" {
		t.Fatalf("client ip should be set, got empty")
	}
}

func TestInjectRequestContext_TraceparentPassThrough(t *testing.T) {
	const validTP = "00-aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa-bbbbbbbbbbbbbbbb-01"
	req := httptest.NewRequest(http.MethodGet, "/plugin/x", nil)
	req.Header.Set(HeaderTraceparent, validTP)
	injectRequestContext(req, nil, "p")

	if got := req.Header.Get(HeaderTraceparent); got != validTP {
		t.Fatalf("valid traceparent should pass through, got %q", got)
	}
}

func TestInjectRequestContext_InvalidTraceparentRewritten(t *testing.T) {
	const bogus = "garbage-trace"
	req := httptest.NewRequest(http.MethodGet, "/plugin/x", nil)
	req.Header.Set(HeaderTraceparent, bogus)
	injectRequestContext(req, nil, "p")

	got := req.Header.Get(HeaderTraceparent)
	if got == bogus {
		t.Fatalf("invalid traceparent leaked through")
	}
	if !isValidTraceparent(got) {
		t.Fatalf("rewritten traceparent invalid: %q", got)
	}
}

func TestInjectRequestContext_RequestIDPassThrough(t *testing.T) {
	const incoming = "11111111-2222-3333-4444-555555555555"
	req := httptest.NewRequest(http.MethodGet, "/plugin/x", nil)
	req.Header.Set(HeaderPluginRequestID, incoming)
	injectRequestContext(req, nil, "p")

	if got := req.Header.Get(HeaderPluginRequestID); got != incoming {
		t.Fatalf("request id should pass through, got %q", got)
	}
}

func TestInjectRequestContext_NoAuthCtxStillSetsName(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/plugin/public", nil)
	injectRequestContext(req, nil, "hello-world")

	if got := req.Header.Get(HeaderPluginName); got != "hello-world" {
		t.Fatalf("plugin name missing, got %q", got)
	}
	if req.Header.Get(HeaderPluginUserID) != "" {
		t.Fatalf("user id should be empty for unauth request")
	}
	if req.Header.Get(HeaderPluginAPIKeyID) != "" {
		t.Fatalf("api key id should be empty for unauth request")
	}
	if req.Header.Get(HeaderTraceparent) == "" {
		t.Fatalf("traceparent should still be generated")
	}
}
