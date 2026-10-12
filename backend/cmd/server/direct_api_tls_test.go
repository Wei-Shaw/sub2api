//go:build unit

package main

import (
	"crypto/tls"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDirectAPIHandlerRestrictsRoutesAndHost(t *testing.T) {
	handler := directAPIHandler(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}), "direct.buaga.dpdns.org", func() []string { return nil })
	tests := []struct {
		name, method, target, host string
		withTLS                    bool
		want                       int
	}{
		{"health", "GET", "/health", "direct.buaga.dpdns.org:8443", true, 204},
		{"models", "GET", "/v1/models", "direct.buaga.dpdns.org:8443", true, 204},
		{"responses", "POST", "/v1/responses", "direct.buaga.dpdns.org:8443", true, 204},
		{"alias", "GET", "/models", "direct.buaga.dpdns.org:8443", true, 204},
		{"video task", "GET", "/contents/generations/tasks/123", "direct.buaga.dpdns.org:8443", true, 204},
		{"admin", "GET", "/api/v1/admin/accounts", "direct.buaga.dpdns.org:8443", true, 404},
		{"auth", "POST", "/api/v1/auth/login", "direct.buaga.dpdns.org:8443", true, 404},
		{"frontend", "GET", "/assets/index.js", "direct.buaga.dpdns.org:8443", true, 404},
		{"similar prefix", "GET", "/v1admin/accounts", "direct.buaga.dpdns.org:8443", true, 404},
		{"parent traversal", "GET", "/v1/../api/v1/admin", "direct.buaga.dpdns.org:8443", true, 404},
		{"encoded slash", "GET", "/v1/%2f../api/v1/admin", "direct.buaga.dpdns.org:8443", true, 404},
		{"other host", "GET", "/v1/models", "buaga.dpdns.org:8443", true, 404},
		{"plaintext", "GET", "/v1/models", "direct.buaga.dpdns.org:8443", false, 404},
		{"health post", "POST", "/health", "direct.buaga.dpdns.org:8443", true, 404},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(tc.method, tc.target, nil)
			req.Host = tc.host
			if tc.withTLS {
				req.TLS = &tls.ConnectionState{}
			}
			w := httptest.NewRecorder()
			handler.ServeHTTP(w, req)
			require.Equal(t, tc.want, w.Code)
		})
	}
}

func TestDirectAPIHandlerDropsSpoofedForwardingHeaders(t *testing.T) {
	handler := directAPIHandler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		for _, name := range []string{
			"Forwarded", "X-Forwarded-For", "X-Real-IP", "CF-Connecting-IP",
			"X-Forwarded-Proto", "X-Forwarded-Host", "X-Cdn-IP",
		} {
			require.Empty(t, r.Header.Get(name), name)
		}
		require.Equal(t, "203.0.113.11:12345", r.RemoteAddr)
		require.Equal(t, "Bearer test", r.Header.Get("Authorization"))
		w.WriteHeader(http.StatusNoContent)
	}), "direct.buaga.dpdns.org", func() []string { return []string{"X-Cdn-IP"} })
	req := httptest.NewRequest("GET", "/v1/models", nil)
	req.Host = "direct.buaga.dpdns.org:8443"
	req.TLS = &tls.ConnectionState{}
	req.RemoteAddr = "203.0.113.11:12345"
	for _, name := range []string{
		"Forwarded", "X-Forwarded-For", "X-Real-IP", "CF-Connecting-IP",
		"X-Forwarded-Proto", "X-Forwarded-Host", "X-Cdn-IP",
	} {
		req.Header.Set(name, "1.2.3.4")
	}
	req.Header.Set("Authorization", "Bearer test")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	require.Equal(t, http.StatusNoContent, w.Code)
	require.Equal(t, "max-age=15552000", w.Header().Get("Strict-Transport-Security"))
}
