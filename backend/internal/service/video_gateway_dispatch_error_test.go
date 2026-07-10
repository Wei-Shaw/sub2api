package service

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/http/httptrace"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

func TestSeedanceCreateTransportErrorPreSendIsExplicitFailure(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Fatal("TLS validation failure must happen before the request reaches the handler")
	}))
	defer server.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, server.URL, strings.NewReader(`{"prompt":"offline"}`))
	if err != nil {
		t.Fatalf("build request: %v", err)
	}
	_, err = executeVideoDispatchRequest(&http.Client{Timeout: time.Second}, request)
	transportErr := requireVideoDispatchTransportError(t, err)
	if transportErr.RequestMayHaveBeenSent {
		t.Fatalf("pre-send TLS failure marked ambiguous: %v", err)
	}
	if infraerrors.Reason(err) != "SEEDANCE_CREATE_HTTP_ERROR" {
		t.Fatalf("wrapped infra reason = %q", infraerrors.Reason(err))
	}
}

func TestSeedanceCreateTransportErrorAfterWriteIsAmbiguous(t *testing.T) {
	t.Run("timeout", func(t *testing.T) {
		received := make(chan struct{})
		server := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, request *http.Request) {
			_, _ = io.Copy(io.Discard, request.Body)
			close(received)
			<-request.Context().Done()
		}))
		defer server.Close()

		ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
		defer cancel()
		request, err := http.NewRequestWithContext(ctx, http.MethodPost, server.URL, strings.NewReader(`{"prompt":"offline"}`))
		if err != nil {
			t.Fatalf("build request: %v", err)
		}
		_, err = executeVideoDispatchRequest(&http.Client{Timeout: time.Second}, request)
		select {
		case <-received:
		default:
			t.Fatal("server did not receive the written request")
		}
		transportErr := requireVideoDispatchTransportError(t, err)
		if !transportErr.RequestMayHaveBeenSent {
			t.Fatalf("post-write timeout marked explicit: %v", err)
		}
	})

	t.Run("eof", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
			hijacker, ok := writer.(http.Hijacker)
			if !ok {
				t.Error("test server does not support hijacking")
				return
			}
			conn, _, err := hijacker.Hijack()
			if err != nil {
				t.Errorf("hijack: %v", err)
				return
			}
			_ = conn.Close()
		}))
		defer server.Close()

		request, err := http.NewRequestWithContext(context.Background(), http.MethodPost, server.URL, strings.NewReader(`{"prompt":"offline"}`))
		if err != nil {
			t.Fatalf("build request: %v", err)
		}
		_, err = executeVideoDispatchRequest(&http.Client{Timeout: time.Second}, request)
		transportErr := requireVideoDispatchTransportError(t, err)
		if !transportErr.RequestMayHaveBeenSent {
			t.Fatalf("post-write EOF marked explicit: %v", err)
		}
	})
}

func TestVideoDispatchWriteEvidenceRequiresActualRequestBytesAfterHeaders(t *testing.T) {
	for _, tc := range []struct {
		name    string
		write   func([]byte) (int, error)
		maySent bool
	}{
		{
			name: "headers callback then zero-byte flush failure",
			write: func([]byte) (int, error) {
				return 0, errors.New("flush failed before writing request bytes")
			},
			maySent: false,
		},
		{
			name: "headers callback then partial write",
			write: func(payload []byte) (int, error) {
				if len(payload) == 0 {
					return 0, errors.New("empty write")
				}
				return 1, io.ErrUnexpectedEOF
			},
			maySent: true,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var wroteHeaders atomic.Bool
			ctx := httptrace.WithClientTrace(context.Background(), &httptrace.ClientTrace{
				WroteHeaders: func() { wroteHeaders.Store(true) },
			})
			request, err := http.NewRequestWithContext(ctx, http.MethodPost, "http://dispatch.test/create", strings.NewReader(`{"prompt":"offline"}`))
			if err != nil {
				t.Fatalf("build request: %v", err)
			}
			transport := &http.Transport{
				DialContext: func(context.Context, string, string) (net.Conn, error) {
					return &scriptedDispatchConn{write: tc.write, writeStarted: make(chan struct{})}, nil
				},
			}
			_, err = executeVideoDispatchRequest(&http.Client{Transport: transport, Timeout: time.Second}, request)
			if !wroteHeaders.Load() {
				t.Fatal("test did not reach WroteHeaders callback")
			}
			transportErr := requireVideoDispatchTransportError(t, err)
			if transportErr.RequestMayHaveBeenSent != tc.maySent {
				t.Fatalf("RequestMayHaveBeenSent=%t, want %t", transportErr.RequestMayHaveBeenSent, tc.maySent)
			}
		})
	}
}

func TestVideoDispatchWriteEvidencePreservesLegacyCustomDial(t *testing.T) {
	for _, tc := range []struct {
		name    string
		write   func([]byte) (int, error)
		maySent bool
	}{
		{
			name: "zero byte failure",
			write: func([]byte) (int, error) {
				return 0, errors.New("legacy dial flush failed")
			},
			maySent: false,
		},
		{
			name: "partial write",
			write: func([]byte) (int, error) {
				return 1, io.ErrUnexpectedEOF
			},
			maySent: true,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var dialCalls atomic.Int64
			transport := &http.Transport{
				Dial: func(string, string) (net.Conn, error) { //nolint:staticcheck // exercise legacy custom Dial precedence
					dialCalls.Add(1)
					return &scriptedDispatchConn{write: tc.write, writeStarted: make(chan struct{})}, nil
				},
			}
			request, err := http.NewRequestWithContext(context.Background(), http.MethodPost, "http://127.0.0.1:1/create", strings.NewReader(`{"prompt":"offline"}`))
			if err != nil {
				t.Fatalf("build request: %v", err)
			}
			_, err = executeVideoDispatchRequest(&http.Client{Transport: transport, Timeout: time.Second}, request)
			transportErr := requireVideoDispatchTransportError(t, err)
			if dialCalls.Load() != 1 {
				t.Fatalf("legacy custom Dial calls=%d, want 1", dialCalls.Load())
			}
			if transportErr.RequestMayHaveBeenSent != tc.maySent {
				t.Fatalf("RequestMayHaveBeenSent=%t, want %t", transportErr.RequestMayHaveBeenSent, tc.maySent)
			}
		})
	}
}

func TestVideoDispatchCustomRoundTripperIsPreservedAndConservativelyAmbiguous(t *testing.T) {
	roundTripper := &recordingDispatchRoundTripper{err: errors.New("custom transport failed at https://secret.internal/?token=must-not-leak")}
	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatalf("new cookie jar: %v", err)
	}
	request, err := http.NewRequestWithContext(context.Background(), http.MethodPost, "http://dispatch.test/create", strings.NewReader(`{"prompt":"offline"}`))
	if err != nil {
		t.Fatalf("build request: %v", err)
	}
	jar.SetCookies(request.URL, []*http.Cookie{{Name: "dispatch_session", Value: "offline"}})
	checkRedirect := func(*http.Request, []*http.Request) error { return errors.New("redirect not expected") }
	client := &http.Client{
		Transport:     roundTripper,
		Timeout:       time.Second,
		Jar:           jar,
		CheckRedirect: checkRedirect,
	}

	_, err = executeVideoDispatchRequest(client, request)
	transportErr := requireVideoDispatchTransportError(t, err)
	if roundTripper.calls.Load() != 1 {
		t.Fatalf("custom RoundTripper calls=%d, want 1", roundTripper.calls.Load())
	}
	if !roundTripper.sawDeadline.Load() {
		t.Fatal("client Timeout was not preserved")
	}
	if !roundTripper.sawCookie.Load() {
		t.Fatal("client Jar was not preserved")
	}
	if !transportErr.RequestMayHaveBeenSent {
		t.Fatal("custom RoundTripper error must be conservatively ambiguous")
	}
	if strings.Contains(err.Error(), "secret.internal") || strings.Contains(err.Error(), "token=") {
		t.Fatalf("custom RoundTripper error was not safe: %v", err)
	}
	if client.Transport != roundTripper || client.Jar != jar || client.Timeout != time.Second || client.CheckRedirect == nil {
		t.Fatal("executeVideoDispatchRequest mutated client configuration")
	}
}

type recordingDispatchRoundTripper struct {
	calls       atomic.Int64
	sawDeadline atomic.Bool
	sawCookie   atomic.Bool
	err         error
}

func (r *recordingDispatchRoundTripper) RoundTrip(request *http.Request) (*http.Response, error) {
	r.calls.Add(1)
	_, hasDeadline := request.Context().Deadline()
	r.sawDeadline.Store(hasDeadline)
	r.sawCookie.Store(strings.Contains(request.Header.Get("Cookie"), "dispatch_session=offline"))
	return nil, r.err
}

func TestSeedanceCreateResponseAfterHeadersPreservesAmbiguity(t *testing.T) {
	t.Run("truncated body", func(t *testing.T) {
		reader := io.MultiReader(
			strings.NewReader(`{"id":"partial`),
			dispatchErrorReader{err: io.ErrUnexpectedEOF},
		)
		_, err := readSeedanceCreateResponseBody(reader)
		transportErr := requireVideoDispatchTransportError(t, err)
		if !transportErr.RequestMayHaveBeenSent {
			t.Fatalf("truncated response marked explicit: %v", err)
		}
	})

	for _, tc := range []struct {
		name string
		body string
	}{
		{name: "malformed 2xx", body: `{"id":`},
		{name: "2xx without upstream id", body: `{"status":"submitted"}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := parseSeedanceCreateResponse(&VideoProviderAccount{PlainAPIKey: "offline-key"}, "offline-model", http.StatusOK, []byte(tc.body))
			transportErr := requireVideoDispatchTransportError(t, err)
			if !transportErr.RequestMayHaveBeenSent {
				t.Fatalf("invalid 2xx response marked explicit: %v", err)
			}
		})
	}
}

func TestSeedanceCreateCompleteNon2xxRemainsProviderRejection(t *testing.T) {
	_, err := parseSeedanceCreateResponse(
		&VideoProviderAccount{PlainAPIKey: "offline-key"},
		"offline-model",
		http.StatusBadRequest,
		[]byte(`{"error":{"message":"request rejected"}}`),
	)
	if err == nil {
		t.Fatal("expected provider rejection")
	}
	var transportErr *VideoDispatchTransportError
	if errors.As(err, &transportErr) {
		t.Fatalf("complete non-2xx response must not be ambiguous: %v", err)
	}
	if infraerrors.Reason(err) != "SEEDANCE_CREATE_UPSTREAM_ERROR" {
		t.Fatalf("provider rejection reason=%q", infraerrors.Reason(err))
	}
}

type dispatchErrorReader struct {
	err error
}

func (r dispatchErrorReader) Read([]byte) (int, error) { return 0, r.err }

type scriptedDispatchConn struct {
	write        func([]byte) (int, error)
	writeStarted chan struct{}
	writeOnce    sync.Once
}

func (c *scriptedDispatchConn) Read([]byte) (int, error) {
	<-c.writeStarted
	return 0, io.EOF
}
func (c *scriptedDispatchConn) Write(payload []byte) (int, error) {
	c.writeOnce.Do(func() { close(c.writeStarted) })
	return c.write(payload)
}
func (c *scriptedDispatchConn) Close() error                     { return nil }
func (c *scriptedDispatchConn) LocalAddr() net.Addr              { return dispatchTestAddr("local") }
func (c *scriptedDispatchConn) RemoteAddr() net.Addr             { return dispatchTestAddr("remote") }
func (c *scriptedDispatchConn) SetDeadline(time.Time) error      { return nil }
func (c *scriptedDispatchConn) SetReadDeadline(time.Time) error  { return nil }
func (c *scriptedDispatchConn) SetWriteDeadline(time.Time) error { return nil }

type dispatchTestAddr string

func (a dispatchTestAddr) Network() string { return "test" }
func (a dispatchTestAddr) String() string  { return string(a) }

func requireVideoDispatchTransportError(t *testing.T, err error) *VideoDispatchTransportError {
	t.Helper()
	if err == nil {
		t.Fatal("expected transport error")
	}
	var transportErr *VideoDispatchTransportError
	if !errors.As(err, &transportErr) {
		t.Fatalf("error type = %T, want *VideoDispatchTransportError: %v", err, err)
	}
	return transportErr
}
