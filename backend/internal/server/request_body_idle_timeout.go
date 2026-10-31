package server

import (
	"errors"
	"io"
	"net/http"
	"os"
	"time"
)

// requestBodyIdleTimeoutHandler fails a request body read that receives no
// bytes for timeout. Unlike http.Server.ReadTimeout it never bounds the total
// upload time, so large bodies that keep making progress are not cut off, and
// time the handler spends between reads is not counted as client idleness:
// every Read re-arms the deadline before it waits.
//
// HTTP/2 stream deadlines fire even without a pending Read, so they are armed
// only during a Read. HTTP/1 deadlines only affect reads, so they stay armed
// while the body is unfinished: net/http drains up to 256 KiB of an unread
// body itself before writing the response, and that drain must not wait on a
// stalled client forever when a handler rejects a request without reading it.
// After a timeout the expired deadline is kept, so that drain fails at once
// and the connection is closed instead of reused.
func requestBodyIdleTimeoutHandler(next http.Handler, timeout time.Duration) http.Handler {
	if timeout <= 0 {
		return next
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Body == nil || r.Body == http.NoBody {
			next.ServeHTTP(w, r)
			return
		}
		body := &idleTimeoutBody{
			body:       r.Body,
			controller: http.NewResponseController(w),
			timeout:    timeout,
			http1:      r.ProtoMajor == 1,
		}
		if body.http1 {
			body.arm()
		}
		// Shallow copy like http.MaxBytesHandler: net/http inspects the
		// original *body when writing the response to decide whether the
		// connection can be reused.
		r2 := *r
		r2.Body = body
		next.ServeHTTP(w, &r2)
	})
}

type idleTimeoutBody struct {
	body       io.ReadCloser
	controller *http.ResponseController
	timeout    time.Duration
	http1      bool
	// unsupported is set when the ResponseWriter cannot set read deadlines
	// (for example httptest.ResponseRecorder); reads are then unbounded.
	unsupported bool
	timeoutErr  error
}

// arm sets the deadline to now+timeout and reports whether it is supported.
func (b *idleTimeoutBody) arm() bool {
	if b.unsupported {
		return false
	}
	if err := b.controller.SetReadDeadline(time.Now().Add(b.timeout)); err != nil {
		b.unsupported = true
		return false
	}
	return true
}

func (b *idleTimeoutBody) Read(p []byte) (int, error) {
	if b.timeoutErr != nil {
		return 0, b.timeoutErr
	}
	if len(p) == 0 || !b.arm() {
		return b.body.Read(p)
	}
	n, err := b.body.Read(p)
	switch {
	case err == nil && b.http1:
		b.arm()
	case err != nil && errors.Is(err, os.ErrDeadlineExceeded):
		b.timeoutErr = err
	default:
		// The server sets no ReadTimeout, so "no deadline" is the state to
		// restore. At EOF net/http has already cleared the HTTP/1 deadline
		// for its background read, which must not inherit a new one.
		_ = b.controller.SetReadDeadline(time.Time{})
	}
	return n, err
}

func (b *idleTimeoutBody) Close() error {
	return b.body.Close()
}
