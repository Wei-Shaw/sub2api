package httputil

import (
	"bufio"
	"errors"
	"io"
	"net"
	"net/http"
	"sync/atomic"
	"time"
)

// WithBodyReadTimeout bounds each socket read while receiving a body. It does
// not set a whole-request deadline or limit the duration of an SSE response.
func WithBodyReadTimeout(next http.Handler, timeout time.Duration) http.Handler {
	if timeout <= 0 {
		return next
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Body != nil && r.Body != http.NoBody {
			body := &bodyReadTimeout{ReadCloser: r.Body, controller: http.NewResponseController(w), timeout: timeout}
			r.Body = body
			if r.ProtoMajor == 1 {
				writer := &bodyDrainTimeoutWriter{ResponseWriter: w, body: body}
				defer func() {
					if !writer.hijacked.Load() {
						// net/http closes its original body after ServeHTTP returns,
						// bypassing our Read and Close methods.
						body.startDrain()
					}
				}()
				next.ServeHTTP(writer, r)
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

type bodyReadTimeout struct {
	io.ReadCloser
	controller *http.ResponseController
	timeout    time.Duration
	finished   atomic.Bool
	failed     atomic.Bool
}

func (r *bodyReadTimeout) Read(p []byte) (int, error) {
	err := r.controller.SetReadDeadline(time.Now().Add(r.timeout))
	if err != nil && !errors.Is(err, http.ErrNotSupported) {
		return 0, err
	}
	n, readErr := r.ReadCloser.Read(p)
	if errors.Is(readErr, io.EOF) {
		r.finished.Store(true)
	} else if readErr != nil {
		r.failed.Store(true)
	}
	if readErr == nil || errors.Is(readErr, io.EOF) {
		_ = r.controller.SetReadDeadline(time.Time{})
	}
	// Keep a failed read's deadline: net/http may drain an unread HTTP/1 body
	// after the handler returns. Clearing a timeout here could block that drain.
	return n, readErr
}

func (r *bodyReadTimeout) Close() error {
	armed := r.startDrain()
	err := r.ReadCloser.Close()
	if err == nil {
		r.finished.Store(true)
	} else {
		r.failed.Store(true)
	}
	r.finishDrain(armed)
	return err
}

func (r *bodyReadTimeout) startDrain() bool {
	if r.finished.Load() || r.failed.Load() {
		return false
	}
	return r.controller.SetReadDeadline(time.Now().Add(r.timeout)) == nil
}

func (r *bodyReadTimeout) finishDrain(armed bool) {
	if armed && !r.failed.Load() {
		_ = r.controller.SetReadDeadline(time.Time{})
	}
}

// HTTP/1 can drain an unread request body during its first response write or
// flush as well as after the handler returns. These reads use net/http's original
// body, so guard the response operations too. HTTP/2 has no such implicit drain.
type bodyDrainTimeoutWriter struct {
	http.ResponseWriter
	body       *bodyReadTimeout
	hijacked   atomic.Bool
	fullDuplex atomic.Bool
}

func (w *bodyDrainTimeoutWriter) Unwrap() http.ResponseWriter { return w.ResponseWriter }

func (w *bodyDrainTimeoutWriter) startDrain() bool {
	return !w.fullDuplex.Load() && w.body.startDrain()
}

func (w *bodyDrainTimeoutWriter) Write(p []byte) (int, error) {
	armed := w.startDrain()
	defer w.body.finishDrain(armed)
	return w.ResponseWriter.Write(p)
}

func (w *bodyDrainTimeoutWriter) WriteString(s string) (int, error) {
	armed := w.startDrain()
	defer w.body.finishDrain(armed)
	return io.WriteString(w.ResponseWriter, s)
}

func (w *bodyDrainTimeoutWriter) WriteHeader(status int) {
	armed := w.startDrain()
	defer w.body.finishDrain(armed)
	w.ResponseWriter.WriteHeader(status)
}

func (w *bodyDrainTimeoutWriter) FlushError() error {
	armed := w.startDrain()
	defer w.body.finishDrain(armed)
	return http.NewResponseController(w.ResponseWriter).Flush()
}

func (w *bodyDrainTimeoutWriter) Flush() { _ = w.FlushError() }

func (w *bodyDrainTimeoutWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	conn, rw, err := http.NewResponseController(w.ResponseWriter).Hijack()
	if err == nil {
		w.hijacked.Store(true)
		_ = conn.SetReadDeadline(time.Time{})
	}
	return conn, rw, err
}

func (w *bodyDrainTimeoutWriter) EnableFullDuplex() error {
	if err := http.NewResponseController(w.ResponseWriter).EnableFullDuplex(); err != nil {
		return err
	}
	w.fullDuplex.Store(true)
	return nil
}

func (w *bodyDrainTimeoutWriter) CloseNotify() <-chan bool {
	if notifier, ok := w.ResponseWriter.(http.CloseNotifier); ok { //nolint:staticcheck // Gin's ResponseWriter still requires legacy CloseNotify forwarding.
		return notifier.CloseNotify()
	}
	return make(chan bool)
}
