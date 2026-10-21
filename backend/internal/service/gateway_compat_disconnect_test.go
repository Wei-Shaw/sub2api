//go:build unit

package service

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/apicompat"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

// Cancel at a specific pending line read so the event/data boundary is tested
// deterministically, without timing sleeps or changing production behavior.
type cancelOnStreamWaitContext struct {
	context.Context
	cancel context.CancelFunc
	waits  int
}

func (c *cancelOnStreamWaitContext) Done() <-chan struct{} {
	c.waits--
	if c.waits == 0 {
		c.cancel()
	}
	return c.Context.Done()
}

func TestGatewayCompatibilityStreams_ContextCancellationPreservesPendingUsage(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, endpoint := range []string{"chat_completions", "responses"} {
		t.Run(endpoint, func(t *testing.T) {
			rec := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(rec)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			// message_start event + data, then message_delta event + pending data.
			c.Request = httptest.NewRequest(http.MethodPost, "/", nil).WithContext(&cancelOnStreamWaitContext{Context: ctx, cancel: cancel, waits: 4})
			pr, pw := io.Pipe()
			defer pr.Close()
			defer pw.Close()
			resp := &http.Response{Body: pr, Header: http.Header{}}
			go func() {
				defer pw.Close()
				_, _ = io.WriteString(pw, "event: message_start\ndata: {\"type\":\"message_start\",\"message\":{\"id\":\"msg_cancel\",\"role\":\"assistant\",\"content\":[],\"usage\":{\"input_tokens\":12}}}\nevent: message_delta\n")
				<-ctx.Done()
				_, _ = io.WriteString(pw, "data: {\"type\":\"message_delta\",\"delta\":{\"stop_reason\":\"end_turn\"},\"usage\":{\"output_tokens\":7}}\n\nevent: message_stop\ndata: {\"type\":\"message_stop\"}\n\n")
			}()
			svc := &GatewayService{} // Normal idle timeout disabled.
			var result *ForwardResult
			var err error
			if endpoint == "chat_completions" {
				result, err = svc.handleCCStreamingFromAnthropic(resp, c, "claude-sonnet-4.5", "claude-sonnet-4.5", nil, time.Now())
			} else {
				result, err = svc.handleResponsesStreamingResponse(resp, c, "claude-sonnet-4.5", "claude-sonnet-4.5", nil, time.Now(), apicompat.ResponsesClientToolMapping{})
			}
			require.NoError(t, err)
			require.NotNil(t, result)
			require.True(t, result.ClientDisconnect)
			require.Equal(t, 12, result.Usage.InputTokens)
			require.Equal(t, 7, result.Usage.OutputTokens)
			require.NotContains(t, rec.Body.String(), "[DONE]")
			require.NotContains(t, rec.Body.String(), "response.completed")
		})
	}
}

func TestGatewayCompatibilityStreams_CanceledClientIdleTimeout(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, endpoint := range []string{"chat_completions", "responses"} {
		t.Run(endpoint, func(t *testing.T) {
			rec := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(rec)
			ctx, cancel := context.WithCancel(context.Background())
			cancel()
			c.Request = httptest.NewRequest(http.MethodPost, "/", nil).WithContext(ctx)
			pr, pw := io.Pipe()
			defer pr.Close()
			defer pw.Close()
			resp := &http.Response{Body: pr, Header: http.Header{}}
			svc := &GatewayService{cfg: &config.Config{Gateway: config.GatewayConfig{StreamDataIntervalTimeout: 1}}}
			var result *ForwardResult
			var err error
			if endpoint == "chat_completions" {
				result, err = svc.handleCCStreamingFromAnthropic(resp, c, "claude-sonnet-4.5", "claude-sonnet-4.5", nil, time.Now())
			} else {
				result, err = svc.handleResponsesStreamingResponse(resp, c, "claude-sonnet-4.5", "claude-sonnet-4.5", nil, time.Now(), apicompat.ResponsesClientToolMapping{})
			}
			require.ErrorIs(t, err, errAnthropicNativeStreamIdle)
			require.NotNil(t, result)
			require.True(t, result.ClientDisconnect)
			require.Empty(t, rec.Body.String())
			_, writeErr := io.WriteString(pw, "data: late\n")
			require.ErrorIs(t, writeErr, io.ErrClosedPipe)
		})
	}
}
