package handler

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestOpenAIImagesStreamPrimerWritesInitialComment(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/images/generations", strings.NewReader(`{"stream":true}`))

	streamStarted := false
	stop, started := startOpenAIImagesStreamPrimer(c, &streamStarted, 10*time.Second)
	require.True(t, started)
	require.True(t, streamStarted)
	require.NotNil(t, stop)
	stop()
	stop()

	require.Equal(t, http.StatusOK, w.Code)
	require.Equal(t, "text/event-stream", w.Header().Get("Content-Type"))
	require.Equal(t, "no-cache", w.Header().Get("Cache-Control"))
	require.Equal(t, "keep-alive", w.Header().Get("Connection"))
	require.Equal(t, "no", w.Header().Get("X-Accel-Buffering"))
	require.Contains(t, w.Body.String(), ":\n\n")
}

func TestOpenAIImagesStreamPrimerNoopsWhenAlreadyStarted(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/images/generations", nil)

	streamStarted := true
	stop, started := startOpenAIImagesStreamPrimer(c, &streamStarted, 10*time.Second)
	require.False(t, started)
	require.Nil(t, stop)
	require.Empty(t, w.Body.String())
}

func TestOpenAIImagesStreamPrimerWritesPeriodicComments(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/images/generations", nil)

	streamStarted := false
	stop, started := startOpenAIImagesStreamPrimer(c, &streamStarted, 10*time.Millisecond)
	require.True(t, started)
	require.NotNil(t, stop)
	require.Eventually(t, func() bool {
		return strings.Count(w.Body.String(), ":\n\n") >= 2
	}, 200*time.Millisecond, 10*time.Millisecond)
	stop()
	stop()
}

func TestHandleOpenAIImagesStreamPrimerForwardErrorWritesSSEErrorAndStops(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/images/generations", nil)
	c.Writer.WriteHeader(http.StatusOK)
	_, _ = c.Writer.Write([]byte(":\n\n"))

	stopCalls := 0
	wrote := handleOpenAIImagesStreamPrimerForwardError(c, func() { stopCalls++ })

	require.True(t, wrote)
	require.Equal(t, 1, stopCalls)
	require.Contains(t, w.Body.String(), "event: error")
	require.Contains(t, w.Body.String(), "Upstream request failed")
}

func TestShouldWriteOpenAIImagesStreamPrimerForwardErrorIgnoresPrimerComments(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/images/generations", nil)

	streamStarted := false
	stop, started := startOpenAIImagesStreamPrimer(c, &streamStarted, 10*time.Millisecond)
	require.True(t, started)
	t.Cleanup(stop)

	require.Eventually(t, func() bool {
		return strings.Count(w.Body.String(), ":\n\n") >= 2
	}, 200*time.Millisecond, 10*time.Millisecond)
	require.True(t, shouldWriteOpenAIImagesStreamPrimerForwardError(c))

	service.MarkOpenAIImagesStreamResponseWritten(c)
	require.False(t, shouldWriteOpenAIImagesStreamPrimerForwardError(c))
}
