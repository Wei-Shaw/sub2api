package httpx

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestResolveRequestBaseURLFallsBackToForwardedHost(t *testing.T) {
	req := httptest.NewRequest("GET", "http://plugin-service/plugins/image-generation", nil)
	req.Header.Set("X-Forwarded-Proto", "https")
	req.Header.Set("X-Forwarded-Host", "edge.example.com")

	got := ResolveRequestBaseURL(req)
	if got != "https://edge.example.com" {
		t.Fatalf("ResolveRequestBaseURL() = %q, want %q", got, "https://edge.example.com")
	}
}

func TestResolveRequestBaseURLReturnsEmptyWithoutForwardedHost(t *testing.T) {
	req := httptest.NewRequest("GET", "http://plugin-service/plugins/image-generation", nil)
	if got := ResolveRequestBaseURL(req); got != "http://plugin-service" {
		t.Fatalf("ResolveRequestBaseURL() = %q, want %q", got, "http://plugin-service")
	}
}

func TestResolveRequestBaseURLUsesForwardedHeader(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "http://plugin-service/plugins/image-generation", nil)
	req.Header.Set("Forwarded", `for=192.0.2.60;proto=https;host=app.example.com`)

	got := ResolveRequestBaseURL(req)
	if got != "https://app.example.com" {
		t.Fatalf("ResolveRequestBaseURL() = %q, want %q", got, "https://app.example.com")
	}
}

func TestResolveRequestBaseURLFallsBackToRefererOrigin(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "http://plugin-service/plugins/image-generation", nil)
	req.Header.Set("Referer", "https://app.example.com/user/custom-pages/plugin")

	got := ResolveRequestBaseURL(req)
	if got != "https://app.example.com" {
		t.Fatalf("ResolveRequestBaseURL() = %q, want %q", got, "https://app.example.com")
	}
}

func TestResolveFrameAncestorOriginPrefersForwardedHeaders(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "http://plugin-service/plugins/image-generation", nil)
	req.Host = "plugin-service"
	req.Header.Set("X-Forwarded-Proto", "https")
	req.Header.Set("X-Forwarded-Host", "app.example.com")

	got := ResolveFrameAncestorOrigin(req)
	if got != "https://app.example.com" {
		t.Fatalf("ResolveFrameAncestorOrigin() = %q, want %q", got, "https://app.example.com")
	}
}

func TestResolveFrameAncestorOriginFallsBackToReferer(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "http://plugin-service/plugins/image-generation", nil)
	req.Host = "plugin-service"
	req.Header.Set("Referer", "https://app.example.com/user/custom-pages/plugin")

	got := ResolveFrameAncestorOrigin(req)
	if got != "https://app.example.com" {
		t.Fatalf("ResolveFrameAncestorOrigin() = %q, want %q", got, "https://app.example.com")
	}
}

func TestResolveFrameAncestorOriginIgnoresPlainRequestHost(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "http://plugin-service/plugins/image-generation", nil)
	req.Host = "example.com"

	got := ResolveFrameAncestorOrigin(req)
	if got != "" {
		t.Fatalf("ResolveFrameAncestorOrigin() = %q, want empty", got)
	}
}
