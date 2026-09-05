package service

import (
	"context"
	"net/http"
	"net/http/httptest"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

const preHeaderKeepaliveTestInterval = 5 * time.Millisecond

func newPreHeaderKeepaliveTestContext(t *testing.T) (*gin.Context, *httptest.ResponseRecorder, context.CancelFunc) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	ctx, cancel := context.WithCancel(context.Background())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil).WithContext(ctx)
	return c, recorder, cancel
}

func TestPreHeaderSSEKeepalivePeriodicProtocolFrames(t *testing.T) {
	t.Run("anthropic", func(t *testing.T) {
		c, recorder, cancel := newPreHeaderKeepaliveTestContext(t)
		defer cancel()
		EnsureAnthropicPreHeaderSSEKeepalive(c, preHeaderKeepaliveTestInterval)
		time.Sleep(12 * preHeaderKeepaliveTestInterval)
		require.True(t, StopAnthropicPreHeaderSSEKeepaliveCommitted(c))
		require.GreaterOrEqual(t, strings.Count(recorder.Body.String(), `event: ping`), 3)
		require.Equal(t, http.StatusOK, recorder.Code)
		require.Equal(t, "text/event-stream", recorder.Header().Get("Content-Type"))
		require.Contains(t, recorder.Body.String(), `data: {"type": "ping"}`)
	})

	t.Run("openai", func(t *testing.T) {
		c, recorder, cancel := newPreHeaderKeepaliveTestContext(t)
		defer cancel()
		EnsureOpenAIPreHeaderSSEKeepalive(c, preHeaderKeepaliveTestInterval)
		time.Sleep(12 * preHeaderKeepaliveTestInterval)
		require.True(t, StopOpenAIPreHeaderSSEKeepaliveCommitted(c))
		require.GreaterOrEqual(t, strings.Count(recorder.Body.String(), ":\n\n"), 3)
		require.Equal(t, http.StatusOK, recorder.Code)
		require.NotContains(t, recorder.Body.String(), "data:")
	})
}

func TestPreHeaderSSEKeepaliveFast429PreservesHTTPStatus(t *testing.T) {
	t.Run("anthropic", func(t *testing.T) {
		c, recorder, cancel := newPreHeaderKeepaliveTestContext(t)
		defer cancel()
		EnsureAnthropicPreHeaderSSEKeepalive(c, time.Hour)
		writeAnthropicGatewayErrorResponse(c, http.StatusTooManyRequests, "rate_limit_error", "slow down")
		require.Equal(t, http.StatusTooManyRequests, recorder.Code)
		require.Equal(t, "application/json; charset=utf-8", recorder.Header().Get("Content-Type"))
		require.NotContains(t, recorder.Body.String(), "event:")
	})

	t.Run("openai", func(t *testing.T) {
		c, recorder, cancel := newPreHeaderKeepaliveTestContext(t)
		defer cancel()
		EnsureOpenAIPreHeaderSSEKeepalive(c, time.Hour)
		writeOpenAIResponsesGatewayError(c, http.StatusTooManyRequests, "rate_limit_error", "slow down")
		require.Equal(t, http.StatusTooManyRequests, recorder.Code)
		require.Equal(t, "application/json; charset=utf-8", recorder.Header().Get("Content-Type"))
		require.NotContains(t, recorder.Body.String(), "response.failed")
	})
}

func TestPreHeaderHeartbeatDoesNotCountAsBusinessOutputOrBlockFailover(t *testing.T) {
	t.Run("anthropic", func(t *testing.T) {
		c, _, cancel := newPreHeaderKeepaliveTestContext(t)
		defer cancel()
		before := AnthropicPreHeaderKeepaliveAdjustedWrittenSize(c)
		EnsureAnthropicPreHeaderSSEKeepalive(c, preHeaderKeepaliveTestInterval)
		time.Sleep(4 * preHeaderKeepaliveTestInterval)
		require.Equal(t, before, AnthropicPreHeaderKeepaliveAdjustedWrittenSize(c))
		// A second account attempt may still take over and write real output.
		_, err := c.Writer.Write([]byte("event: message_start\ndata: {}\n\n"))
		require.NoError(t, err)
		require.Greater(t, AnthropicPreHeaderKeepaliveAdjustedWrittenSize(c), before)
		StopAnthropicPreHeaderSSEKeepaliveCommitted(c)
	})

	t.Run("openai", func(t *testing.T) {
		c, _, cancel := newPreHeaderKeepaliveTestContext(t)
		defer cancel()
		before := OpenAICompactKeepaliveAdjustedWrittenSize(c)
		EnsureOpenAIPreHeaderSSEKeepalive(c, preHeaderKeepaliveTestInterval)
		time.Sleep(4 * preHeaderKeepaliveTestInterval)
		require.Equal(t, before, OpenAICompactKeepaliveAdjustedWrittenSize(c))
		_, err := c.Writer.Write([]byte("data: {\"type\":\"response.output_text.delta\",\"delta\":\"ok\"}\n\n"))
		require.NoError(t, err)
		require.Greater(t, OpenAICompactKeepaliveAdjustedWrittenSize(c), before)
		StopOpenAIPreHeaderSSEKeepaliveCommitted(c)
	})
}

func TestPreHeaderSSEKeepaliveCommittedFailureUsesProtocolSSE(t *testing.T) {
	t.Run("anthropic", func(t *testing.T) {
		c, recorder, cancel := newPreHeaderKeepaliveTestContext(t)
		defer cancel()
		EnsureAnthropicPreHeaderSSEKeepalive(c, preHeaderKeepaliveTestInterval)
		time.Sleep(4 * preHeaderKeepaliveTestInterval)
		writeAnthropicGatewayErrorResponse(c, http.StatusBadGateway, "upstream_error", "all accounts failed")
		require.Equal(t, http.StatusOK, recorder.Code)
		require.Contains(t, recorder.Body.String(), "event: error")
		require.Contains(t, recorder.Body.String(), "all accounts failed")
	})

	t.Run("openai", func(t *testing.T) {
		c, recorder, cancel := newPreHeaderKeepaliveTestContext(t)
		defer cancel()
		EnsureOpenAIPreHeaderSSEKeepalive(c, preHeaderKeepaliveTestInterval)
		time.Sleep(4 * preHeaderKeepaliveTestInterval)
		writeOpenAIResponsesGatewayError(c, http.StatusBadGateway, "upstream_error", "all accounts failed")
		require.Equal(t, http.StatusOK, recorder.Code)
		require.Contains(t, recorder.Body.String(), "event: response.failed")
		require.Contains(t, recorder.Body.String(), "all accounts failed")
	})
}

func TestPreHeaderSSEKeepaliveCancelStopsGoroutine(t *testing.T) {
	before := runtime.NumGoroutine()
	c, recorder, cancel := newPreHeaderKeepaliveTestContext(t)
	EnsureOpenAIPreHeaderSSEKeepalive(c, preHeaderKeepaliveTestInterval)
	time.Sleep(4 * preHeaderKeepaliveTestInterval)
	cancel()
	StopOpenAIPreHeaderSSEKeepaliveCommitted(c)
	lengthAfterStop := recorder.Body.Len()
	time.Sleep(4 * preHeaderKeepaliveTestInterval)
	require.Equal(t, lengthAfterStop, recorder.Body.Len())
	require.Eventually(t, func() bool { return runtime.NumGoroutine() <= before+1 }, time.Second, preHeaderKeepaliveTestInterval)
}
