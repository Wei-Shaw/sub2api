package repository

import (
	"net/http"

	"github.com/Wei-Shaw/sub2api/internal/pkg/traffic"
)

// Instrument beneath Grok's fallback so rejected attempts, redirects, and
// fallback responses are measured too, not just the final successful body.
func httpClientWithTraffic(client *http.Client) *http.Client {
	if client == nil {
		client = http.DefaultClient
	}
	clone := *client
	base := clone.Transport
	if base == nil {
		base = http.DefaultTransport
	}
	if fallback, ok := base.(*grokAccessDeniedFallbackTransport); ok {
		clone.Transport = &grokAccessDeniedFallbackTransport{base: traffic.NewUpstreamTransport(fallback.base)}
	} else {
		clone.Transport = traffic.NewUpstreamTransport(base)
	}
	return &clone
}
