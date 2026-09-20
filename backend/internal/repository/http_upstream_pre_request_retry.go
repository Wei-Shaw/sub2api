package repository

import (
	"crypto/tls"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptrace"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/servertiming"
	"github.com/Wei-Shaw/sub2api/internal/service"
)

// Retry only an observed transient TLS handshake failure, before any connection
// was handed to HTTP. In particular, a missing trace is NOT proof of no write.
// Keep evidence across all redirects/connections in the first Client.Do call.
func doWithOpenAIPreRequestRetry(client *http.Client, req *http.Request, proxyURL string, profile service.HTTPUpstreamProfile) (*http.Response, error) {
	if req == nil || req.URL == nil || req.URL.Scheme != "https" ||
		strings.TrimSpace(proxyURL) == "" || profile != service.HTTPUpstreamProfileOpenAI ||
		(req.Body != nil && req.Body != http.NoBody && req.GetBody == nil) {
		return servertiming.Do(client, req)
	}
	var mu sync.Mutex
	var started, transientFailure, handedToHTTP bool
	markHTTP := func() {
		mu.Lock()
		handedToHTTP = true
		mu.Unlock()
	}
	trace := &httptrace.ClientTrace{
		TLSHandshakeStart: func() {
			mu.Lock()
			started = true
			mu.Unlock()
		},
		TLSHandshakeDone: func(_ tls.ConnectionState, err error) {
			mu.Lock()
			if started && transientTLSHandshakeError(err) {
				transientFailure = true
			}
			mu.Unlock()
		},
		GotConn:              func(httptrace.GotConnInfo) { markHTTP() },
		WroteHeaderField:     func(string, []string) { markHTTP() },
		WroteHeaders:         markHTTP,
		WroteRequest:         func(httptrace.WroteRequestInfo) { markHTTP() },
		GotFirstResponseByte: markHTTP,
	}
	first := req.Clone(httptrace.WithClientTrace(req.Context(), trace))
	resp, err := servertiming.Do(client, first)
	mu.Lock()
	retry := transientFailure && !handedToHTTP
	mu.Unlock()
	if err == nil || resp != nil || !retry || !transientTLSHandshakeError(err) || req.Context().Err() != nil {
		return resp, err
	}
	timer := time.NewTimer(150 * time.Millisecond)
	defer timer.Stop()
	select {
	case <-req.Context().Done():
		return nil, req.Context().Err()
	case <-timer.C:
	}
	second := req.Clone(req.Context())
	if req.Body != nil && req.Body != http.NoBody {
		body, bodyErr := req.GetBody()
		if bodyErr != nil {
			return nil, err
		}
		second.Body = body
	}
	// No URL, proxy credentials, account identity or request body in this marker.
	slog.Info("openai.upstream_pre_request_retry", "attempt", 2, "reason", "tls_handshake_before_http")
	// Same client/proxy and TLS verification; at most one additional attempt.
	return servertiming.Do(client, second)
}

func transientTLSHandshakeError(err error) bool {
	return errors.Is(err, syscall.ECONNRESET) || errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF)
}
