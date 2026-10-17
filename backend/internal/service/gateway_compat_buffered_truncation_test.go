//go:build unit

package service

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/iotest"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/apicompat"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func compatBufferedHandlers() map[string]compatStreamHandler {
	return map[string]compatStreamHandler{
		"chat_completions": func(svc *GatewayService, resp *http.Response, c *gin.Context) (*ForwardResult, error) {
			return svc.handleCCBufferedFromAnthropic(resp, c, "claude-sonnet-4-5", "claude-sonnet-4-5", nil, time.Now())
		},
		"responses": func(svc *GatewayService, resp *http.Response, c *gin.Context) (*ForwardResult, error) {
			return svc.handleResponsesBufferedStreamingResponse(resp, c, "claude-sonnet-4-5", "claude-sonnet-4-5", nil, time.Now(), apicompat.ResponsesClientToolMapping{})
		},
	}
}

// compatBufferedErrorMessage reads the error message of a Chat Completions or
// Responses error body; both put it under error.message.
func compatBufferedErrorMessage(t *testing.T, body []byte) string {
	t.Helper()
	var payload struct {
		Error struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	require.NoError(t, json.Unmarshal(body, &payload), "error body must be JSON: %s", body)
	return payload.Error.Message
}

// Control: a complete upstream stream still answers 200 with the full content.
func TestCompatBufferedCompleteStreamSucceeds(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for name, handle := range compatBufferedHandlers() {
		t.Run(name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(rec)
			resp := &http.Response{Header: http.Header{"x-request-id": {"rid_buffered_ok"}}, Body: io.NopCloser(strings.NewReader(compatAnthropicStream(true)))}

			result, err := handle(&GatewayService{}, resp, c)
			require.NoError(t, err)
			require.NotNil(t, result)
			require.Equal(t, http.StatusOK, rec.Code)
			require.Contains(t, rec.Body.String(), "hello world")
			require.Equal(t, 15, result.Usage.OutputTokens)
		})
	}
}

// A non-streaming client whose upstream closed without message_stop must not
// receive the half-assembled answer as a success. It gets a 502 in its own
// endpoint format, and the usage the upstream already metered travels with the
// error so the handler still bills it.
func TestCompatBufferedMissingMessageStopIsNotSuccess(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for name, handle := range compatBufferedHandlers() {
		t.Run(name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(rec)
			resp := &http.Response{Header: http.Header{"x-request-id": {"rid_buffered_truncated"}}, Body: io.NopCloser(strings.NewReader(compatAnthropicStream(false)))}

			result, err := handle(&GatewayService{}, resp, c)
			require.Error(t, err)
			var failoverErr *UpstreamFailoverError
			require.False(t, errors.As(err, &failoverErr), "metered usage is billed, not retried")
			require.NotNil(t, result, "metered usage must be returned with the error")
			require.Equal(t, 10, result.Usage.InputTokens)
			require.Equal(t, 3, result.Usage.CacheReadInputTokens)
			require.Equal(t, 15, result.Usage.OutputTokens)
			require.False(t, result.Stream)

			require.Equal(t, http.StatusBadGateway, rec.Code)
			require.NotContains(t, rec.Body.String(), "hello", "partial content must not reach the client")
			require.NotEmpty(t, compatBufferedErrorMessage(t, rec.Body.Bytes()))
		})
	}
}

// An upstream that dies mid-stream after message_start is truncated as well:
// the read error does not fail over because input usage was already metered.
func TestCompatBufferedReadErrorAfterUsageIsBilledError(t *testing.T) {
	gin.SetMode(gin.TestMode)
	head := strings.Join(strings.SplitAfter(compatAnthropicStream(false), "\n\n")[:3], "")
	for name, handle := range compatBufferedHandlers() {
		t.Run(name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(rec)
			body := io.MultiReader(strings.NewReader(head), iotest.ErrReader(errors.New("unexpected EOF")))
			resp := &http.Response{Header: http.Header{}, Body: io.NopCloser(body)}

			result, err := handle(&GatewayService{}, resp, c)
			require.Error(t, err)
			var failoverErr *UpstreamFailoverError
			require.False(t, errors.As(err, &failoverErr))
			require.NotNil(t, result)
			require.Equal(t, 10, result.Usage.InputTokens)
			require.Equal(t, http.StatusBadGateway, rec.Code)
			require.NotContains(t, rec.Body.String(), "hello")
		})
	}
}

// A read failure before any usage was metered fails over like the native
// /v1/messages path: nil result, nothing written, retryable on the same account.
func TestCompatBufferedReadErrorBeforeUsageFailsOver(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for name, handle := range compatBufferedHandlers() {
		t.Run(name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(rec)
			resp := &http.Response{Header: http.Header{}, Body: io.NopCloser(iotest.ErrReader(errors.New("connection reset by peer")))}

			result, err := handle(&GatewayService{}, resp, c)
			require.Nil(t, result)
			var failoverErr *UpstreamFailoverError
			require.ErrorAs(t, err, &failoverErr)
			require.Equal(t, http.StatusBadGateway, failoverErr.StatusCode)
			require.True(t, failoverErr.RetryableOnSameAccount)
			require.False(t, c.Writer.Written(), "nothing may be written before the handler fails over")
		})
	}
}
