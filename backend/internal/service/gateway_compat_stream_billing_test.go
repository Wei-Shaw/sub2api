//go:build unit

package service

import (
	"bytes"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/iotest"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/apicompat"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

// compatDisconnectingWriter accepts a fixed number of writes, then fails every
// later write the way a closed client connection does.
type compatDisconnectingWriter struct {
	header     http.Header
	body       bytes.Buffer
	writesLeft int
}

func (w *compatDisconnectingWriter) Header() http.Header {
	if w.header == nil {
		w.header = http.Header{}
	}
	return w.header
}

func (w *compatDisconnectingWriter) WriteHeader(int) {}

func (w *compatDisconnectingWriter) Write(p []byte) (int, error) {
	if w.writesLeft <= 0 {
		return 0, errors.New("write: broken pipe")
	}
	w.writesLeft--
	return w.body.Write(p)
}

func (w *compatDisconnectingWriter) Flush() {}

func compatAnthropicStream(withStop bool) string {
	lines := []string{
		`event: message_start`,
		`data: {"type":"message_start","message":{"id":"msg_cb","type":"message","role":"assistant","content":[],"model":"claude-sonnet-4-5","stop_reason":"","usage":{"input_tokens":10,"cache_read_input_tokens":3,"output_tokens":1}}}`,
		``,
		`event: content_block_start`,
		`data: {"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}`,
		``,
		`event: content_block_delta`,
		`data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"hello"}}`,
		``,
		`event: content_block_delta`,
		`data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":" world"}}`,
		``,
		`event: content_block_stop`,
		`data: {"type":"content_block_stop","index":0}`,
		``,
		`event: message_delta`,
		`data: {"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"output_tokens":15}}`,
		``,
	}
	if withStop {
		lines = append(lines, `event: message_stop`, `data: {"type":"message_stop"}`, ``)
	}
	return strings.Join(lines, "\n") + "\n"
}

type compatStreamHandler func(svc *GatewayService, resp *http.Response, c *gin.Context) (*ForwardResult, error)

func compatStreamHandlers() map[string]compatStreamHandler {
	return map[string]compatStreamHandler{
		"chat_completions": func(svc *GatewayService, resp *http.Response, c *gin.Context) (*ForwardResult, error) {
			return svc.handleCCStreamingFromAnthropic(resp, c, "claude-sonnet-4-5", "claude-sonnet-4-5", nil, time.Now())
		},
		"responses": func(svc *GatewayService, resp *http.Response, c *gin.Context) (*ForwardResult, error) {
			return svc.handleResponsesStreamingResponse(resp, c, "claude-sonnet-4-5", "claude-sonnet-4-5", nil, time.Now(), apicompat.ResponsesClientToolMapping{})
		},
	}
}

// A client that disconnects mid-stream must not cut off upstream reading:
// the terminal message_delta carries the billed output tokens.
func TestCompatStreamClientDisconnectStillBillsTerminalUsage(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for name, handle := range compatStreamHandlers() {
		t.Run(name, func(t *testing.T) {
			w := &compatDisconnectingWriter{writesLeft: 1}
			c, _ := gin.CreateTestContext(w)
			resp := &http.Response{Header: http.Header{"x-request-id": {"rid_disconnect"}}, Body: io.NopCloser(strings.NewReader(compatAnthropicStream(true)))}

			result, err := handle(&GatewayService{}, resp, c)
			require.NoError(t, err, "a complete upstream stream stays successful after the client left")
			require.NotNil(t, result)
			require.Equal(t, 10, result.Usage.InputTokens)
			require.Equal(t, 3, result.Usage.CacheReadInputTokens)
			require.Equal(t, 15, result.Usage.OutputTokens, "output usage from the terminal message_delta must be kept")
			require.True(t, result.ClientDisconnect)
		})
	}
}

// An upstream that closes without message_stop is a truncated response: it is
// reported as an error, never finalized as a synthetic completion, and the
// usage the upstream already metered travels with the error for billing.
func TestCompatStreamMissingMessageStopReturnsPartialUsageError(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for name, handle := range compatStreamHandlers() {
		t.Run(name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(rec)
			resp := &http.Response{Header: http.Header{"x-request-id": {"rid_truncated"}}, Body: io.NopCloser(strings.NewReader(compatAnthropicStream(false)))}

			result, err := handle(&GatewayService{}, resp, c)
			require.Error(t, err)
			var failoverErr *UpstreamFailoverError
			require.False(t, errors.As(err, &failoverErr), "output already reached the client; no failover")
			require.NotNil(t, result, "metered usage must be returned with the error")
			require.Equal(t, 10, result.Usage.InputTokens)
			require.Equal(t, 15, result.Usage.OutputTokens)
			require.NotContains(t, rec.Body.String(), "[DONE]")
			require.NotContains(t, rec.Body.String(), "response.completed")
		})
	}
}

// A read failure before anything reached the client and before any usage was
// metered fails over like the native /v1/messages path, with a nil result so a
// successful retry is not billed twice.
func TestCompatStreamReadErrorBeforeOutputFailsOver(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for name, handle := range compatStreamHandlers() {
		t.Run(name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(rec)
			resp := &http.Response{Header: http.Header{}, Body: io.NopCloser(iotest.ErrReader(errors.New("connection reset by peer")))}

			result, err := handle(&GatewayService{}, resp, c)
			require.Nil(t, result)
			var failoverErr *UpstreamFailoverError
			require.ErrorAs(t, err, &failoverErr)
			require.Equal(t, http.StatusBadGateway, failoverErr.StatusCode)
			require.False(t, c.Writer.Written(), "nothing may be written before the handler fails over")
		})
	}
}

// After the client is gone, a stalled upstream is abandoned once the stream
// data interval elapses, so draining for usage cannot hang forever.
func TestCompatStreamDrainAfterDisconnectIsBounded(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for name, handle := range compatStreamHandlers() {
		t.Run(name, func(t *testing.T) {
			pr, pw := io.Pipe()
			defer func() { _ = pw.Close() }()
			go func() {
				head := strings.SplitAfter(compatAnthropicStream(false), "\n\n")
				for _, frame := range head[:4] {
					if _, err := pw.Write([]byte(frame)); err != nil {
						return
					}
				}
			}()
			w := &compatDisconnectingWriter{writesLeft: 1}
			c, _ := gin.CreateTestContext(w)
			resp := &http.Response{Header: http.Header{}, Body: pr}
			svc := &GatewayService{cfg: &config.Config{Gateway: config.GatewayConfig{StreamDataIntervalTimeout: 1}}}

			done := make(chan struct{})
			var result *ForwardResult
			var err error
			go func() {
				defer close(done)
				result, err = handle(svc, resp, c)
			}()
			select {
			case <-done:
			case <-time.After(10 * time.Second):
				t.Fatal("draining a stalled upstream after client disconnect never stopped")
			}
			require.Error(t, err)
			require.NotNil(t, result)
			require.Equal(t, 10, result.Usage.InputTokens)
			require.True(t, result.ClientDisconnect)
		})
	}
}
