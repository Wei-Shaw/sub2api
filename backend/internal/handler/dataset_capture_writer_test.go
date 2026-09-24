package handler

import (
	"bytes"
	"errors"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/dataset"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestBeginDatasetCaptureSkipsDisabledCollector(t *testing.T) {
	previous := dataset.CollectorSingleton
	dataset.CollectorSingleton = nil
	t.Cleanup(func() { dataset.CollectorSingleton = previous })

	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	original := c.Writer

	capture, restored := beginDatasetCapture(c)

	require.Nil(t, capture)
	require.Nil(t, restored)
	require.Same(t, original, c.Writer)
}

func TestDatasetCaptureWriterPreservesClientResponse(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	capture := newDatasetCaptureWriter(c.Writer)
	c.Writer = capture
	c.Writer.Header().Set("X-Test", "preserved")
	c.Writer.WriteHeader(202)

	_, err := c.Writer.WriteString("data: hello\n\n")
	require.NoError(t, err)
	require.Equal(t, 202, w.Code)
	require.Equal(t, "preserved", w.Header().Get("X-Test"))
	require.Equal(t, "data: hello\n\n", w.Body.String())
	require.Equal(t, []byte("data: hello\n\n"), capture.capturedBytes())
}

func TestDatasetCaptureWriterDropsOversizedCaptureWithoutTruncatingClient(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	capture := newDatasetCaptureWriter(c.Writer)
	c.Writer = capture
	payload := bytes.Repeat([]byte("x"), dataset.MaxCaptureBytes+1)

	_, err := c.Writer.Write(payload)
	require.NoError(t, err)
	require.Equal(t, payload, w.Body.Bytes())
	require.False(t, capture.canCapture())
	require.Nil(t, capture.capturedBytes())
}

func TestCaptureOpenAIChatCompletionResultCapturesOnlyCompletedResults(t *testing.T) {
	previous := dataset.CollectorSingleton
	dataset.CollectorSingleton = dataset.NewCollector(10)
	t.Cleanup(func() { dataset.CollectorSingleton = previous })

	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	capture := newDatasetCaptureWriter(c.Writer)
	c.Writer = capture
	_, err := c.Writer.Write(captureTestResponseForHandler())
	require.NoError(t, err)

	requestBody := []byte(`{"model":"gpt-test","messages":[{"role":"user","content":"hello"}]}`)
	captureOpenAIChatCompletionResult(requestBody, &service.OpenAIForwardResult{Stream: false}, nil, capture)
	require.Len(t, dataset.CollectorSingleton.Drain(1), 1)

	captureOpenAIChatCompletionResult(requestBody, &service.OpenAIForwardResult{Stream: false}, errors.New("partial forward"), capture)
	require.Empty(t, dataset.CollectorSingleton.Drain(1))

	captureOpenAIChatCompletionResult(requestBody, &service.OpenAIForwardResult{Stream: false, ClientDisconnect: true}, nil, capture)
	require.Empty(t, dataset.CollectorSingleton.Drain(1))
}

func captureTestResponseForHandler() []byte {
	return []byte(`{"id":"chatcmpl-test","object":"chat.completion","model":"gpt-test","choices":[{"index":0,"message":{"role":"assistant","content":"world"},"finish_reason":"stop"}]}`)
}
