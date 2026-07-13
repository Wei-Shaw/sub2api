package service

import (
	"bufio"
	"bytes"
	"io"
	"net"
	"net/http"

	"github.com/gin-gonic/gin"
)

// openAIImagesBufferedResponseWriter isolates a compatibility probe from the
// real client response until the probe is selected as the final result.
type openAIImagesBufferedResponseWriter struct {
	target      gin.ResponseWriter
	header      http.Header
	body        bytes.Buffer
	status      int
	size        int
	committed   bool
	passthrough bool
	commitErr   error
}

var _ gin.ResponseWriter = (*openAIImagesBufferedResponseWriter)(nil)

func newOpenAIImagesBufferedResponseWriter(target gin.ResponseWriter) *openAIImagesBufferedResponseWriter {
	header := make(http.Header)
	if target != nil {
		header = cloneHTTPHeader(target.Header())
	}
	return &openAIImagesBufferedResponseWriter{
		target: target,
		header: header,
		status: http.StatusOK,
		size:   -1,
	}
}

func (w *openAIImagesBufferedResponseWriter) Header() http.Header {
	if w.passthrough && w.target != nil {
		return w.target.Header()
	}
	return w.header
}

func (w *openAIImagesBufferedResponseWriter) WriteHeader(code int) {
	if w.passthrough && w.target != nil {
		w.target.WriteHeader(code)
		return
	}
	if code > 0 && !w.Written() {
		w.status = code
	}
}

func (w *openAIImagesBufferedResponseWriter) WriteHeaderNow() {
	if w.passthrough && w.target != nil {
		w.target.WriteHeaderNow()
		return
	}
	if !w.Written() {
		w.size = 0
	}
}

func (w *openAIImagesBufferedResponseWriter) Write(data []byte) (int, error) {
	if w.passthrough && w.target != nil {
		return w.target.Write(data)
	}
	w.WriteHeaderNow()
	n, err := w.body.Write(data)
	w.size += n
	return n, err
}

func (w *openAIImagesBufferedResponseWriter) WriteString(value string) (int, error) {
	if w.passthrough && w.target != nil {
		return w.target.WriteString(value)
	}
	w.WriteHeaderNow()
	n, err := w.body.WriteString(value)
	w.size += n
	return n, err
}

func (w *openAIImagesBufferedResponseWriter) Status() int {
	if w.passthrough && w.target != nil {
		return w.target.Status()
	}
	return w.status
}

func (w *openAIImagesBufferedResponseWriter) Size() int {
	if w.passthrough && w.target != nil {
		return w.target.Size()
	}
	return w.size
}

func (w *openAIImagesBufferedResponseWriter) Written() bool {
	if w.passthrough && w.target != nil {
		return w.target.Written()
	}
	return w.size >= 0
}

func (w *openAIImagesBufferedResponseWriter) Flush() {
	_ = w.Commit()
	if w.target != nil {
		w.target.Flush()
	}
}

func (w *openAIImagesBufferedResponseWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	if err := w.Commit(); err != nil {
		return nil, nil, err
	}
	return w.target.Hijack()
}

func (w *openAIImagesBufferedResponseWriter) CloseNotify() <-chan bool {
	return w.target.CloseNotify()
}

func (w *openAIImagesBufferedResponseWriter) Pusher() http.Pusher {
	if w.target == nil {
		return nil
	}
	return w.target.Pusher()
}

func (w *openAIImagesBufferedResponseWriter) Committed() bool {
	return w != nil && w.committed
}

func (w *openAIImagesBufferedResponseWriter) Commit() error {
	if w == nil || w.committed {
		if w == nil {
			return nil
		}
		return w.commitErr
	}
	if w.target == nil {
		return nil
	}

	targetHeader := w.target.Header()
	for key := range targetHeader {
		delete(targetHeader, key)
	}
	for key, values := range w.header {
		targetHeader[key] = append([]string(nil), values...)
	}

	w.committed = true
	w.passthrough = true
	w.target.WriteHeader(w.status)
	if w.body.Len() == 0 {
		w.target.WriteHeaderNow()
		return nil
	}

	n, err := w.target.Write(w.body.Bytes())
	if err == nil && n != w.body.Len() {
		err = io.ErrShortWrite
	}
	w.commitErr = err
	return err
}
