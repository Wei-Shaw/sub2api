package repository

import (
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/http/httptrace"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/tlsfingerprint"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

// Exercise both public entry points. The TLS case uses the test server's trusted
// transport so the test remains local and does not depend on system trust roots.
func lifecycleClient(t *testing.T, srv *httptest.Server, withTLS bool) (*httpUpstreamService, func(*http.Request) (*http.Response, error)) {
	t.Helper()
	s, ok := NewHTTPUpstream(nil).(*httpUpstreamService)
	require.True(t, ok)
	profile := &tlsfingerprint.Profile{Name: "body-lifecycle-test"}
	var entry *upstreamClientEntry
	var err error
	if withTLS {
		entry, err = s.getClientEntryWithTLS("", 1, 1, profile, service.HTTPUpstreamProfileDefault, false, true)
	} else {
		entry, err = s.acquireClientWithProfile("", 1, 1, service.HTTPUpstreamProfileDefault)
		if err == nil {
			atomic.AddInt64(&entry.inFlight, -1)
		}
	}
	require.NoError(t, err)
	baseTransport, ok := srv.Client().Transport.(*http.Transport)
	require.True(t, ok)
	tr := baseTransport.Clone()
	tr.MaxConnsPerHost = 1
	tr.MaxIdleConnsPerHost = 1
	tr.IdleConnTimeout = 10 * time.Second
	entry.client = &http.Client{Transport: tr}
	t.Cleanup(tr.CloseIdleConnections)
	return s, func(req *http.Request) (*http.Response, error) {
		if withTLS {
			return s.DoWithTLS(req, "", 1, 1, profile)
		}
		return s.Do(req, "", 1, 1)
	}
}

func requireNoUpstreamInFlight(t *testing.T, s *httpUpstreamService) {
	t.Helper()
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, entry := range s.clients {
		require.Zero(t, atomic.LoadInt64(&entry.inFlight))
	}
}

type notifyReadCloser struct {
	io.ReadCloser
	started chan<- struct{}
	once    sync.Once
}

func (r *notifyReadCloser) Read(p []byte) (int, error) {
	r.once.Do(func() { close(r.started) })
	return r.ReadCloser.Read(p)
}

func TestHTTPUpstreamConcurrentEarlyCloseDoesNotPoisonNextResponse(t *testing.T) {
	for _, withTLS := range []bool{false, true} {
		t.Run(fmt.Sprintf("tls=%v", withTLS), func(t *testing.T) {
			finishFirst := make(chan struct{})
			firstFinished := make(chan struct{})
			var requests atomic.Int32
			srv := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "text/event-stream")
				_, _ = io.WriteString(w, "data: begin\n\n")
				_ = http.NewResponseController(w).Flush()
				if requests.Add(1) == 1 {
					defer close(firstFinished)
					select {
					case <-finishFirst:
					case <-r.Context().Done():
						return
					}
				}
				_, _ = io.WriteString(w, "data: complete\n\n")
			}))
			if withTLS {
				srv.StartTLS()
			} else {
				srv.Start()
			}
			defer srv.Close()
			defer srv.CloseClientConnections()
			s, do := lifecycleClient(t, srv, withTLS)
			ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
			defer cancel()
			req, err := http.NewRequestWithContext(ctx, http.MethodGet, srv.URL, nil)
			require.NoError(t, err)
			resp, err := do(req)
			require.NoError(t, err)
			prefix := make([]byte, len("data: begin\n\n"))
			_, err = io.ReadFull(resp.Body, prefix)
			require.NoError(t, err)
			readerDone := make(chan struct{})
			readStarted := make(chan struct{})
			resp.Body = &notifyReadCloser{ReadCloser: resp.Body, started: readStarted}
			go func() { _, _ = io.Copy(io.Discard, resp.Body); close(readerDone) }()
			// Start Close only after the background reader has entered Read;
			// the server is still holding the response open at this point.
			<-readStarted
			closed := make(chan error, 1)
			go func() { closed <- resp.Body.Close() }()
			select {
			case err := <-closed:
				require.NoError(t, err)
			case <-time.After(5 * time.Second):
				t.Fatal("Close blocked behind the active reader")
			}
			// Let the early-close drain finish only after Close has returned.
			// Go 1.27 can reuse that connection while its old reader awaits EOF.
			close(finishFirst)
			<-firstFinished
			for i := 0; i < 8; i++ {
				resp, err := do(req)
				require.NoError(t, err)
				done := make(chan error, 1)
				go func() { _, err := io.Copy(io.Discard, resp.Body); _ = resp.Body.Close(); done <- err }()
				select {
				case err := <-done:
					require.NoError(t, err)
				case <-time.After(5 * time.Second):
					t.Fatal("response EOF waits for a subsequent request")
				}
			}
			select {
			case <-readerDone:
			case <-time.After(5 * time.Second):
				t.Fatal("early-closed reader leaked")
			}
			require.NoError(t, req.Context().Err(), "closing an attempt must not cancel its caller or retries")
			requireNoUpstreamInFlight(t, s)
		})
	}
}

func TestHTTPUpstreamCompletedBodyPreservesKeepAlive(t *testing.T) {
	for _, encoding := range []string{"", "gzip"} {
		t.Run("encoding="+encoding, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if encoding == "gzip" {
					w.Header().Set("Content-Encoding", encoding)
					zw := gzip.NewWriter(w)
					_, _ = io.WriteString(zw, "complete response")
					_ = zw.Close()
					return
				}
				_, _ = io.WriteString(w, "complete response")
			}))
			defer srv.Close()
			defer srv.CloseClientConnections()
			s, do := lifecycleClient(t, srv, false)
			for i := 0; i < 20; i++ {
				var reused bool
				ctx := httptrace.WithClientTrace(t.Context(), &httptrace.ClientTrace{GotConn: func(info httptrace.GotConnInfo) { reused = info.Reused }})
				req, err := http.NewRequestWithContext(ctx, http.MethodGet, srv.URL, nil)
				require.NoError(t, err)
				req.Header.Set("Accept-Encoding", encoding)
				resp, err := do(req)
				require.NoError(t, err)
				body, err := io.ReadAll(resp.Body)
				require.NoError(t, err)
				require.Equal(t, "complete response", string(body))
				var wg sync.WaitGroup
				for j := 0; j < 4; j++ {
					wg.Add(1)
					go func() { defer wg.Done(); _ = resp.Body.Close() }()
				}
				wg.Wait()
				if i > 0 {
					require.True(t, reused, "fully consumed responses must keep their connection")
				}
			}
			requireNoUpstreamInFlight(t, s)
		})
	}
}

func TestHTTPUpstreamCancellationReleasesAttempt(t *testing.T) {
	for _, beforeHeaders := range []bool{false, true} {
		t.Run(fmt.Sprintf("before_headers=%v", beforeHeaders), func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if !beforeHeaders {
					w.WriteHeader(http.StatusOK)
					_ = http.NewResponseController(w).Flush()
				}
				<-r.Context().Done()
			}))
			defer srv.Close()
			defer srv.CloseClientConnections()
			s, do := lifecycleClient(t, srv, false)
			ctx, cancel := context.WithTimeout(t.Context(), 50*time.Millisecond)
			defer cancel()
			req, err := http.NewRequestWithContext(ctx, http.MethodGet, srv.URL, nil)
			require.NoError(t, err)
			resp, err := do(req)
			if beforeHeaders {
				require.ErrorIs(t, err, context.DeadlineExceeded)
			} else {
				require.NoError(t, err)
				_, err = io.ReadAll(resp.Body)
				require.ErrorIs(t, err, context.DeadlineExceeded)
				require.NoError(t, resp.Body.Close())
			}
			requireNoUpstreamInFlight(t, s)
		})
	}
}

func TestHTTPUpstreamCompressedEarlyCloseStopsReader(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Encoding", "gzip")
		zw := gzip.NewWriter(w)
		_, _ = io.WriteString(zw, "data: begin\n\n")
		_ = zw.Flush()
		_ = http.NewResponseController(w).Flush()
		<-r.Context().Done()
		_ = zw.Close()
	}))
	defer srv.Close()
	defer srv.CloseClientConnections()
	s, do := lifecycleClient(t, srv, false)
	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, srv.URL, nil)
	require.NoError(t, err)
	req.Header.Set("Accept-Encoding", "gzip")
	resp, err := do(req)
	require.NoError(t, err)
	prefix := make([]byte, len("data: begin\n\n"))
	_, err = io.ReadFull(resp.Body, prefix)
	require.NoError(t, err)
	done := make(chan error, 1)
	readStarted := make(chan struct{})
	resp.Body = &notifyReadCloser{ReadCloser: resp.Body, started: readStarted}
	go func() { _, err := io.Copy(io.Discard, resp.Body); done <- err }()
	<-readStarted
	closed := make(chan error, 1)
	go func() { closed <- resp.Body.Close() }()
	select {
	case err := <-closed:
		require.NoError(t, err)
	case <-time.After(5 * time.Second):
		t.Fatal("compressed body Close blocked")
	}
	select {
	case err := <-done:
		require.Error(t, err)
	case <-time.After(5 * time.Second):
		t.Fatal("compressed body reader leaked")
	}
	requireNoUpstreamInFlight(t, s)
}

func TestHTTPUpstreamHTTP2CloseDoesNotCancelOtherStream(t *testing.T) {
	srv := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/held" {
			w.WriteHeader(http.StatusOK)
			_ = http.NewResponseController(w).Flush()
			<-r.Context().Done()
			return
		}
		_, _ = io.WriteString(w, "complete response")
	}))
	srv.EnableHTTP2 = true
	srv.StartTLS()
	defer srv.Close()
	defer srv.CloseClientConnections()
	s, do := lifecycleClient(t, srv, false)
	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, srv.URL+"/held", nil)
	require.NoError(t, err)
	held, err := do(req)
	require.NoError(t, err)
	require.Equal(t, 2, held.ProtoMajor)
	for i := 0; i < 2; i++ {
		var reused bool
		ctx := httptrace.WithClientTrace(t.Context(), &httptrace.ClientTrace{GotConn: func(info httptrace.GotConnInfo) { reused = info.Reused }})
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, srv.URL, nil)
		require.NoError(t, err)
		resp, err := do(req)
		require.NoError(t, err)
		if i == 0 {
			require.NoError(t, held.Body.Close())
		}
		body, err := io.ReadAll(resp.Body)
		require.NoError(t, err)
		require.Equal(t, "complete response", string(body))
		require.NoError(t, resp.Body.Close())
		require.True(t, reused, "closing one HTTP/2 stream must retain the shared connection")
	}
	requireNoUpstreamInFlight(t, s)
}

func TestHTTPUpstreamDetachedRequestDrainsAfterClientDisconnect(t *testing.T) {
	disconnectCtx, disconnect := context.WithCancel(t.Context())
	defer disconnect()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, "begin ")
		_ = http.NewResponseController(w).Flush()
		<-disconnectCtx.Done()
		_, _ = io.WriteString(w, "usage and completion")
	}))
	defer srv.Close()
	defer srv.CloseClientConnections()
	s, do := lifecycleClient(t, srv, false)
	req, err := http.NewRequestWithContext(context.WithoutCancel(disconnectCtx), http.MethodGet, srv.URL, nil)
	require.NoError(t, err)
	resp, err := do(req)
	require.NoError(t, err)
	disconnect()
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.Equal(t, "begin usage and completion", string(body))
	require.NoError(t, resp.Body.Close())
	requireNoUpstreamInFlight(t, s)
}

// Keep the same benchmark runnable against the base revision: it measures a
// complete public Do/read/Close lifecycle without network latency hiding the
// per-attempt context and per-Read synchronization costs.
func BenchmarkHTTPUpstreamBodyLifecycle(b *testing.B) {
	for _, size := range []int{256, 64 * 1024} {
		b.Run(fmt.Sprintf("bytes=%d", size), func(b *testing.B) {
			s, ok := NewHTTPUpstream(nil).(*httpUpstreamService)
			require.True(b, ok)
			entry, err := s.getOrCreateClient("", 1, 1)
			require.NoError(b, err)
			payload := strings.Repeat("x", size)
			entry.client = &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
				return &http.Response{
					StatusCode: http.StatusOK, Header: make(http.Header),
					Body: io.NopCloser(strings.NewReader(payload)),
				}, nil
			})}
			req, err := http.NewRequestWithContext(b.Context(), http.MethodGet, "https://example.com", nil)
			require.NoError(b, err)
			buf := make([]byte, 256)
			b.ReportAllocs()
			b.SetBytes(int64(size))
			b.ResetTimer()
			for b.Loop() {
				resp, err := s.Do(req, "", 1, 1)
				if err != nil {
					b.Fatal(err)
				}
				for {
					_, err = resp.Body.Read(buf)
					if err != nil {
						break
					}
				}
				if err != io.EOF {
					b.Fatal(err)
				}
				if err := resp.Body.Close(); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
