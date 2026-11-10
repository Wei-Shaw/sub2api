package httputil

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestWriteTimeoutReleasesSlowHTTP1Client(t *testing.T) {
	type result struct {
		err       error
		cancelled bool
	}
	finished := make(chan result, 1)
	srv := httptest.NewServer(WithWriteTimeout(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		payload := make([]byte, 64<<10)
		for i := 0; i < 1024; i++ {
			if _, err := w.Write(payload); err != nil {
				finished <- result{err, r.Context().Err() != nil}
				return
			}
		}
		finished <- result{}
	}), 40*time.Millisecond))
	defer srv.Close()
	conn, err := net.Dial("tcp", strings.TrimPrefix(srv.URL, "http://"))
	require.NoError(t, err)
	defer func() { _ = conn.Close() }()
	if tcp, ok := conn.(*net.TCPConn); ok {
		require.NoError(t, tcp.SetReadBuffer(1024))
	}
	_, err = io.WriteString(conn, "GET / HTTP/1.1\r\nHost: example\r\n\r\n")
	require.NoError(t, err)
	select {
	case got := <-finished:
		require.Error(t, got.err)
		require.True(t, got.cancelled)
	case <-time.After(3 * time.Second):
		t.Fatal("slow HTTP/1 client blocked the handler past its write timeout")
	}
}

func TestWriteTimeoutOnlyCancelsSlowHTTP2Stream(t *testing.T) {
	finished := make(chan error, 1)
	srv := httptest.NewUnstartedServer(WithWriteTimeout(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/fast" {
			_, _ = io.WriteString(w, "ok")
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_ = http.NewResponseController(w).Flush()
		payload := make([]byte, 64<<10)
		for i := 0; i < 1024; i++ {
			if _, err := w.Write(payload); err != nil {
				if r.Context().Err() == nil {
					finished <- errors.New("write failed without cancelling request")
				} else {
					finished <- err
				}
				return
			}
		}
		finished <- nil
	}), 40*time.Millisecond))
	srv.EnableHTTP2 = true
	srv.StartTLS()
	defer srv.Close()
	client := srv.Client()
	client.Timeout = 3 * time.Second
	resp, err := client.Get(srv.URL + "/slow")
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()
	require.Equal(t, 2, resp.ProtoMajor)
	select {
	case err := <-finished:
		require.Error(t, err)
	case <-time.After(3 * time.Second):
		t.Fatal("HTTP/2 flow-control stall did not time out")
	}
	fast, err := client.Get(srv.URL + "/fast")
	require.NoError(t, err)
	defer func() { _ = fast.Body.Close() }()
	body, err := io.ReadAll(fast.Body)
	require.NoError(t, err)
	require.Equal(t, "ok", string(body))
}

func TestWriteTimeoutClearsDeadlineDuringUpstreamThinkTime(t *testing.T) {
	srv := httptest.NewServer(WithWriteTimeout(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, "first")
		_ = http.NewResponseController(w).Flush()
		time.Sleep(80 * time.Millisecond)
		_, _ = io.WriteString(w, "second")
		_ = http.NewResponseController(w).Flush()
	}), 20*time.Millisecond))
	defer srv.Close()
	resp, err := srv.Client().Get(srv.URL)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.Equal(t, "firstsecond", string(body))
}

type deadlineFlushWriter struct {
	header   http.Header
	deadline time.Time
}

func (w *deadlineFlushWriter) Header() http.Header         { return w.header }
func (w *deadlineFlushWriter) Write(p []byte) (int, error) { return len(p), nil }
func (w *deadlineFlushWriter) WriteHeader(int)             {}
func (w *deadlineFlushWriter) SetWriteDeadline(deadline time.Time) error {
	w.deadline = deadline
	return nil
}
func (w *deadlineFlushWriter) FlushError() error {
	if w.deadline.IsZero() {
		return errors.New("flush has no deadline")
	}
	<-time.After(time.Until(w.deadline))
	return os.ErrDeadlineExceeded
}

func TestWriteTimeoutFlushCancelsContextAndRemembersFailure(t *testing.T) {
	underlying := &deadlineFlushWriter{header: make(http.Header)}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	w := &writeTimeoutWriter{ResponseWriter: underlying, timeout: 10 * time.Millisecond, cancel: cancel}
	err := w.FlushError()
	require.ErrorIs(t, err, os.ErrDeadlineExceeded)
	require.ErrorIs(t, ctx.Err(), context.Canceled)
	require.True(t, underlying.deadline.IsZero())
	_, err = w.Write([]byte("must not retry after failed flush"))
	require.ErrorIs(t, err, os.ErrDeadlineExceeded)
}
