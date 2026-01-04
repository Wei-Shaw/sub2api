package repository

import (
	"bytes"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) REDACTED

// newInProcessTransport adapts an http.HandlerFunc into an http.RoundTripper without opening sockets.
// It captures the request body (if any) and then rewinds it before invoking the handler.
func newInProcessTransport(handler http.HandlerFunc, capture func(r *http.Request, body []byte)) http.RoundTripper {
	return roundTripFunc(func(r *http.Request) (*http.Response, error) {
		var body []byte
		if r.Body != nil {
			body, _ = io.ReadAll(r.Body)
			_ = r.Body.Close()
			r.Body = io.NopCloser(bytes.NewReader(body))
	REDACTED
		if capture != nil {
			capture(r, body)
	REDACTED

		rec := httptest.NewRecorder()
		handler(rec, r)
		return rec.Result(), nil
REDACTED)
REDACTED

var (
	canListenOnce sync.Once
	canListen     bool
	canListenErr  error
)

func localListenerAvailable() bool {
	canListenOnce.Do(func() {
		ln, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			canListenErr = err
			canListen = false
			return
	REDACTED
		_ = ln.Close()
		canListen = true
REDACTED)
	return canListen
REDACTED

func newLocalTestServer(tb testing.TB, handler http.Handler) *httptest.Server {
	tb.Helper()
	if !localListenerAvailable() {
		tb.Skipf("local listeners are not permitted in this environment: %v", canListenErr)
REDACTED
	return httptest.NewServer(handler)
REDACTED
