package handler

import (
	"bufio"
	"bytes"
	"net"
	"net/http"
)

// responseBuffer captures all output written by gin.Context.Writer.
// It implements gin.ResponseWriter so it can be swapped in via c.Writer.
type responseBuffer struct {
	header http.Header
	body   bytes.Buffer
	status int
	size   int
}

func newResponseBuffer() *responseBuffer {
	return &responseBuffer{
		header: make(http.Header),
		status: http.StatusOK,
	}
}

// --- http.ResponseWriter ---
func (w *responseBuffer) Header() http.Header  { return w.header }
func (w *responseBuffer) Write(b []byte) (int, error) {
	n, err := w.body.Write(b)
	w.size += n
	return n, err
}
func (w *responseBuffer) WriteHeader(statusCode int) { w.status = statusCode }

// --- gin.ResponseWriter ---
func (w *responseBuffer) Status() int             { return w.status }
func (w *responseBuffer) Size() int               { return w.size }
func (w *responseBuffer) Written() bool           { return w.size > 0 }
func (w *responseBuffer) WriteHeaderNow()         {}
func (w *responseBuffer) WriteString(s string) (int, error) { return w.Write([]byte(s)) }
func (w *responseBuffer) Pusher() http.Pusher     { return nil }

// --- http.Hijacker ---
func (w *responseBuffer) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	return nil, nil, http.ErrNotSupported
}

// --- http.Flusher ---
func (w *responseBuffer) Flush() {}

// --- http.CloseNotifier ---
func (w *responseBuffer) CloseNotify() <-chan bool { return nil }
