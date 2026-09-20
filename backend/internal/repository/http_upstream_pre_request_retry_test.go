package repository

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/binary"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/http/httptrace"
	"strings"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

type preRequestRoundTripper func(*http.Request) (*http.Response, error)

func (f preRequestRoundTripper) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestOpenAIPreRequestRetrySafety(t *testing.T) {
	for _, name := range []string{"transient", "persistent", "no_trace", "got_conn", "header_field", "wrote_headers", "wrote_request", "response_byte", "certificate", "cancelled", "cancel_during_backoff", "direct", "grok", "default", "no_getbody", "getbody_error", "429", "502", "503", "redirect_already_sent"} {
		t.Run(name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			var calls, originalTrace int
			ctx = httptrace.WithClientTrace(ctx, &httptrace.ClientTrace{TLSHandshakeStart: func() { originalTrace++ }})
			req, err := http.NewRequestWithContext(ctx, "POST", "https://example.test/first", strings.NewReader("payload"))
			require.NoError(t, err)
			profile, proxy := service.HTTPUpstreamProfileOpenAI, "http://proxy.test"
			if name == "direct" {
				proxy = ""
			}
			if name == "grok" {
				profile = service.HTTPUpstreamProfileGrok
			}
			if name == "default" {
				profile = service.HTTPUpstreamProfileDefault
			}
			if name == "no_getbody" {
				req.GetBody = nil
			}
			if name == "getbody_error" {
				req.GetBody = func() (io.ReadCloser, error) { return nil, errors.New("unavailable") }
			}
			client := &http.Client{Transport: preRequestRoundTripper(func(r *http.Request) (*http.Response, error) {
				calls++
				defer func() { _ = r.Body.Close() }()
				tr := httptrace.ContextClientTrace(r.Context())
				if name == "redirect_already_sent" && calls == 1 {
					if tr.GotConn != nil {
						tr.GotConn(httptrace.GotConnInfo{})
					}
					return &http.Response{StatusCode: 307, Header: http.Header{"Location": []string{"https://example.test/second"}}, Body: http.NoBody, Request: r}, nil
				}
				if name == "429" || name == "502" || name == "503" || (name == "transient" && calls == 2) {
					status := 200
					switch name {
					case "429":
						status = 429
					case "502":
						status = 502
					case "503":
						status = 503
					}
					b, readErr := io.ReadAll(r.Body)
					require.NoError(t, readErr)
					require.Equal(t, "payload", string(b))
					return &http.Response{StatusCode: status, Header: http.Header{}, Body: http.NoBody, Request: r}, nil
				}
				failure := error(syscall.ECONNRESET)
				if name == "certificate" {
					failure = x509.UnknownAuthorityError{}
				}
				if name != "no_trace" {
					if tr.TLSHandshakeStart != nil {
						tr.TLSHandshakeStart()
					}
					if tr.TLSHandshakeDone != nil {
						tr.TLSHandshakeDone(tls.ConnectionState{}, failure)
					}
				}
				switch name {
				case "got_conn":
					if tr.GotConn != nil {
						tr.GotConn(httptrace.GotConnInfo{})
					}
				case "header_field":
					if tr.WroteHeaderField != nil {
						tr.WroteHeaderField("X-Test", []string{"value"})
					}
				case "wrote_headers":
					if tr.WroteHeaders != nil {
						tr.WroteHeaders()
					}
				case "wrote_request":
					if tr.WroteRequest != nil {
						tr.WroteRequest(httptrace.WroteRequestInfo{})
					}
				case "response_byte":
					if tr.GotFirstResponseByte != nil {
						tr.GotFirstResponseByte()
					}
				case "cancelled":
					cancel()
				case "cancel_during_backoff":
					time.AfterFunc(20*time.Millisecond, cancel)
				}
				return nil, failure
			})}
			resp, err := doWithOpenAIPreRequestRetry(client, req, proxy, profile)
			if resp != nil {
				require.NoError(t, resp.Body.Close())
			}
			if name == "transient" || name == "429" || name == "502" || name == "503" {
				require.NoError(t, err)
			} else {
				require.Error(t, err)
			}
			want := 1
			if name == "transient" || name == "persistent" || name == "redirect_already_sent" {
				want = 2
			}
			require.Equal(t, want, calls)
			if name == "transient" {
				require.Equal(t, 1, originalTrace, "preserve caller trace")
			}
		})
	}
}

// Close the first accepted TCP connection before TLS. Real net/http tracing
// must recover without delivering the POST twice (no synthetic trace evidence).
type failFirstTLSListener struct {
	net.Listener
	accepts atomic.Int64
}

func (l *failFirstTLSListener) Accept() (net.Conn, error) {
	for {
		c, err := l.Listener.Accept()
		if err != nil {
			return nil, err
		}
		if l.accepts.Add(1) == 1 {
			// Consume the ClientHello record before an orderly close so this
			// exercises EOF consistently on both Windows and Linux.
			_ = c.SetReadDeadline(time.Now().Add(time.Second))
			var header [5]byte
			if _, err := io.ReadFull(c, header[:]); err == nil {
				_, _ = io.CopyN(io.Discard, c, int64(binary.BigEndian.Uint16(header[3:])))
			}
			_ = c.Close()
			continue
		}
		return c, nil
	}
}

func TestOpenAIPreRequestRetryRealTLS(t *testing.T) {
	var posts atomic.Int64
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		posts.Add(1)
		body, err := io.ReadAll(r.Body)
		if err != nil || string(body) != "one-generation" {
			w.WriteHeader(400)
			return
		}
		w.WriteHeader(200)
	}))
	listener := &failFirstTLSListener{Listener: server.Listener}
	server.Listener = listener
	server.StartTLS()
	defer server.Close()
	req, err := http.NewRequest("POST", server.URL, strings.NewReader("one-generation"))
	require.NoError(t, err)
	resp, err := doWithOpenAIPreRequestRetry(server.Client(), req, "http://test-proxy", service.HTTPUpstreamProfileOpenAI)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()
	require.Equal(t, 200, resp.StatusCode)
	require.Equal(t, int64(2), listener.accepts.Load())
	require.Equal(t, int64(1), posts.Load())
}

func TestOpenAIPreRequestRetryDoesNotReplayRealWrittenPOST(t *testing.T) {
	var posts atomic.Int64
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		posts.Add(1)
		_, _ = io.Copy(io.Discard, r.Body)
		hijacker, ok := w.(http.Hijacker)
		if !ok {
			t.Error("test server does not support hijacking")
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		conn, _, err := hijacker.Hijack()
		if err == nil {
			_ = conn.Close()
		}
	}))
	defer server.Close()
	req, err := http.NewRequest("POST", server.URL, strings.NewReader("one-generation"))
	require.NoError(t, err)
	resp, err := doWithOpenAIPreRequestRetry(server.Client(), req, "http://test-proxy", service.HTTPUpstreamProfileOpenAI)
	require.Error(t, err)
	require.Nil(t, resp)
	require.Equal(t, int64(1), posts.Load())
}
