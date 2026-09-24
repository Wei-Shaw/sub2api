package handler

import (
	"bytes"
	"sync"

	"github.com/Wei-Shaw/sub2api/internal/dataset"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

// datasetCaptureWriter tees accepted response bytes to a bounded buffer while
// delegating every response operation to Gin's original writer. It is installed
// only when the dataset collector is initialized, so the disabled path has no
// additional response-body work.
type datasetCaptureWriter struct {
	gin.ResponseWriter

	mu       sync.Mutex
	buf      bytes.Buffer
	overflow bool
}

func newDatasetCaptureWriter(writer gin.ResponseWriter) *datasetCaptureWriter {
	return &datasetCaptureWriter{
		ResponseWriter: writer,
	}
}

func (w *datasetCaptureWriter) Write(data []byte) (int, error) {
	n, err := w.ResponseWriter.Write(data)
	w.capture(data, n)
	return n, err
}

func (w *datasetCaptureWriter) WriteString(data string) (int, error) {
	n, err := w.ResponseWriter.WriteString(data)
	w.capture([]byte(data), n)
	return n, err
}

func (w *datasetCaptureWriter) capture(data []byte, written int) {
	if written <= 0 {
		return
	}
	if written > len(data) {
		written = len(data)
	}

	w.mu.Lock()
	defer w.mu.Unlock()
	if w.overflow {
		return
	}
	remaining := dataset.MaxCaptureBytes - w.buf.Len()
	if written > remaining {
		w.overflow = true
		return
	}
	_, _ = w.buf.Write(data[:written])
}

func (w *datasetCaptureWriter) capturedBytes() []byte {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.overflow {
		return nil
	}
	return append([]byte(nil), w.buf.Bytes()...)
}

func (w *datasetCaptureWriter) canCapture() bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	return !w.overflow
}

func beginDatasetCapture(c *gin.Context) (*datasetCaptureWriter, gin.ResponseWriter) {
	if c == nil || c.Writer == nil || dataset.CollectorSingleton == nil {
		return nil, nil
	}
	original := c.Writer
	captureWriter := newDatasetCaptureWriter(original)
	c.Writer = captureWriter
	return captureWriter, original
}

func captureOpenAIChatCompletionResult(requestBody []byte, result *service.OpenAIForwardResult, forwardErr error, writer *datasetCaptureWriter) {
	if writer == nil || result == nil || forwardErr != nil || result.ClientDisconnect || !writer.canCapture() {
		return
	}
	responseBody := writer.capturedBytes()
	if len(responseBody) == 0 {
		return
	}
	if result.Stream {
		dataset.CaptureFromOpenAIStream(requestBody, responseBody, writer.Status())
		return
	}
	dataset.CaptureFromOpenAIRequest(requestBody, responseBody, writer.Status())
}
