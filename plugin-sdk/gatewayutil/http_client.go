// Package gatewayutil — HTTP client factories tuned for streaming gateway forward requests.
package gatewayutil

import (
	"crypto/tls"
	"net"
	"net/http"
	"time"
)

// NewStreamingHTTPClient returns an *http.Client tuned for streaming
// upstream requests used by gateway plugins.
//
// Streaming responses (SSE / chunked) can stay open for minutes while the
// model generates output, so the client deliberately omits an overall
// http.Client.Timeout. Connection-level deadlines (DialContext, TLS
// handshake, response headers) protect against hung initial connects.
//
// Per-request cancellation should be done via context, not Client.Timeout.
func NewStreamingHTTPClient() *http.Client {
	return &http.Client{
		Transport: &http.Transport{
			DialContext: (&net.Dialer{
				Timeout:   30 * time.Second,
				KeepAlive: 30 * time.Second,
			}).DialContext,
			TLSClientConfig:       &tls.Config{MinVersion: tls.VersionTLS12},
			TLSHandshakeTimeout:   15 * time.Second,
			MaxIdleConns:          100,
			MaxIdleConnsPerHost:   20,
			IdleConnTimeout:       90 * time.Second,
			ResponseHeaderTimeout: 60 * time.Second,
			ForceAttemptHTTP2:     true,
		},
	}
}
