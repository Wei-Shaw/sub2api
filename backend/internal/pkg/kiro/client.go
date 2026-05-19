package kiro

import (
	"net/http"
	"net/url"
	"time"
)

// HTTPClient returns an *http.Client configured for Kiro REST calls
// (OAuth endpoints, profile fetch). The streaming call in the gateway layer
// uses its own client with a longer timeout — keep this one short so a
// stuck auth/profile lookup can't tie up a request.
//
// If proxyURL is non-empty, requests are routed through it. http, https,
// and socks5 schemes are supported by Go's net/http stack out of the box.
// Empty proxyURL falls back to HTTP_PROXY / HTTPS_PROXY env vars.
func HTTPClient(proxyURL string) *http.Client {
	t := &http.Transport{
		MaxIdleConns:        100,
		MaxIdleConnsPerHost: 10,
		IdleConnTimeout:     90 * time.Second,
		ForceAttemptHTTP2:   true,
	}
	if proxyURL != "" {
		if u, err := url.Parse(proxyURL); err == nil {
			t.Proxy = http.ProxyURL(u)
			// HTTP/2 negotiation through an HTTP proxy is brittle.
			t.ForceAttemptHTTP2 = false
		}
	} else {
		t.Proxy = http.ProxyFromEnvironment
	}
	return &http.Client{
		Timeout:   30 * time.Second,
		Transport: t,
	}
}
