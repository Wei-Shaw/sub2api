package service

import (
	"bufio"
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/apicompat"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestSSEQueueBudgetBackpressureAndCancellation(t *testing.T) {
	cfg := &config.Config{Gateway: config.GatewayConfig{SSEQueueMaxBytes: 64}}
	budget := newSSEQueueBudget(cfg)
	done := make(chan struct{})
	require.True(t, budget.acquire(48, done))
	acquired := make(chan bool, 1)
	go func() { acquired <- budget.acquire(32, done) }()
	select {
	case <-acquired:
		t.Fatal("queue accepted more bytes than its budget")
	case <-time.After(20 * time.Millisecond):
	}
	budget.release(48)
	require.True(t, <-acquired)
	budget.release(32)

	// Large image frames may occupy the budget alone, without reducing the
	// configured per-line limit to the (much smaller) queue budget.
	require.True(t, budget.acquire(1024, done))
	require.Equal(t, int64(64), budget.used.Load())
	go func() { acquired <- budget.acquire(1, done) }()
	select {
	case <-acquired:
		t.Fatal("queued another payload alongside an oversized frame")
	case <-time.After(20 * time.Millisecond):
	}
	close(done)
	select {
	case ok := <-acquired:
		require.False(t, ok)
	case <-time.After(time.Second):
		t.Fatal("cancellation did not release queue producer")
	}
	budget.release(1024)
	require.Zero(t, budget.used.Load())
	require.False(t, budget.acquire(1, done))
}

func TestBoundedSSESplitRejectsUndelimitedEvents(t *testing.T) {
	for _, tc := range []struct {
		name    string
		gateway config.GatewayConfig
		wire    string
	}{
		{"lines", config.GatewayConfig{SSEEventMaxLines: 4}, strings.Repeat(": keepalive\n", 5)},
		{"bytes", config.GatewayConfig{SSEEventMaxBytes: 20}, strings.Repeat("data: abcd\n", 3)},
		{"incomplete_line", config.GatewayConfig{SSEEventMaxBytes: 20}, "data: " + strings.Repeat("a", 32)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			scanner := bufio.NewScanner(strings.NewReader(tc.wire))
			scanner.Split(newBoundedSSESplit(&config.Config{Gateway: tc.gateway}, 1<<20, nil))
			for scanner.Scan() {
			}
			require.ErrorIs(t, scanner.Err(), errSSETransportLimit)
			require.ErrorIs(t, scanner.Err(), bufio.ErrTooLong)
		})
	}
}

func TestBoundedSSESplitResetsAtEventBoundaryAndPreservesEOF(t *testing.T) {
	cfg := &config.Config{Gateway: config.GatewayConfig{SSEEventMaxBytes: 11, SSEEventMaxLines: 1}}
	scanner := bufio.NewScanner(strings.NewReader("data: abcd\n\ndata: last"))
	scanner.Split(newBoundedSSESplit(cfg, 1<<20, nil))
	var lines []string
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}
	require.NoError(t, scanner.Err())
	require.Equal(t, []string{"data: abcd", "", "data: last"}, lines)
}

func TestBoundedSSELineReaderPreservesLargeImageFrame(t *testing.T) {
	payload := "data: {\"b64_json\":\"" + strings.Repeat("A", 128<<10) + "\"}\r\n"
	cfg := &config.Config{Gateway: config.GatewayConfig{MaxLineSize: 256 << 10, SSEQueueMaxBytes: 1024}}
	reader := newBoundedSSELineReader(strings.NewReader(payload+"\r\n"+"data: [DONE]"), cfg)
	line, err := reader.ReadLine()
	require.NoError(t, err)
	require.Equal(t, payload, string(line))
	line, err = reader.ReadLine()
	require.NoError(t, err)
	require.Equal(t, "\r\n", string(line))
	line, err = reader.ReadLine()
	require.ErrorIs(t, err, io.EOF)
	require.Equal(t, "data: [DONE]", string(line))
}

func TestBoundedSSELineReaderRejectsOversizeWithoutTruncatedPrefix(t *testing.T) {
	cfg := &config.Config{Gateway: config.GatewayConfig{MaxLineSize: 4096, SSEEventMaxBytes: 8192}}
	reader := newBoundedSSELineReader(strings.NewReader("data: "+strings.Repeat("A", 64<<10)), cfg)
	line, err := reader.ReadLine()
	require.Nil(t, line)
	require.ErrorIs(t, err, errSSETransportLimit)
}

func TestGatewayStreamingUndelimitedEventFailsWithoutSuccessfulTerminal(t *testing.T) {
	gin.SetMode(gin.TestMode)
	svc := newStreamingResponseTestGatewayService()
	svc.cfg.Gateway.SSEEventMaxLines = 4
	svc.cfg.Gateway.SSEQueueMaxBytes = 128
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)
	wire := "data: {\"type\":\"message_start\",\"message\":{\"usage\":{\"input_tokens\":3}}}\n\n" + strings.Repeat(": malformed event\n", 5) + "\ndata: {\"type\":\"message_stop\"}\n\n"
	resp := &http.Response{StatusCode: http.StatusOK, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(wire))}
	defer func() { _ = resp.Body.Close() }()
	result, err := svc.handleStreamingResponse(context.Background(), resp, c, &Account{ID: 1}, time.Now(), "model", "model", false)
	require.ErrorIs(t, err, errSSETransportLimit)
	require.NotNil(t, result)
	require.Equal(t, 3, result.usage.InputTokens)
	require.Contains(t, rec.Body.String(), "response_too_large")
	require.NotContains(t, rec.Body.String(), "message_stop")
}

func TestAnthropicNativePumpLargeLineBudgetAndStop(t *testing.T) {
	cfg := &config.Config{Gateway: config.GatewayConfig{SSEQueueMaxBytes: 64}}
	scanner := bufio.NewScanner(strings.NewReader(strings.Repeat(strings.Repeat("x", 1024)+"\n", 32)))
	pump := newAnthropicNativeLinePump(scanner, time.Second, cfg)
	line, err := pump.next()
	require.NoError(t, err)
	require.Len(t, line, 1024)
	pump.stop()
	// The producer must stop even if another oversized payload already occupies
	// the entire queue budget. Draining here observes channel closure only.
	deadline := time.After(time.Second)
	for {
		select {
		case _, ok := <-pump.events:
			if !ok {
				return
			}
		case <-deadline:
			t.Fatal("line pump remained blocked after stop")
		}
	}
}

func TestSSETransportDefaultsAndFirstOutputGuardCompose(t *testing.T) {
	require.Equal(t, int64(defaultSSEQueueMaxBytes), newSSEQueueBudget(&config.Config{}).limit)
	limits := newSSEEventLimits(&config.Config{}, 1234)
	require.Equal(t, int64(1234), limits.maxBytes)
	require.Equal(t, defaultSSEEventMaxLines, limits.maxLines)
	innerErr := errors.New("pre-output guard")
	scanner := bufio.NewScanner(strings.NewReader("data: test\n"))
	scanner.Split(newBoundedSSESplit(nil, 1024, func([]byte, bool) (int, []byte, error) { return 0, nil, innerErr }))
	require.False(t, scanner.Scan())
	require.ErrorIs(t, scanner.Err(), innerErr)
}

func TestSSEConfiguredLargeFrameLimitRemainsAvailable(t *testing.T) {
	defaults := newBoundedSSELineReader(strings.NewReader(""), nil)
	require.Equal(t, 500<<20, defaults.maxLineSize)
	cfg := &config.Config{Gateway: config.GatewayConfig{MaxLineSize: 64 << 20}}
	configured := newBoundedSSELineReader(strings.NewReader(""), cfg)
	require.Equal(t, 64<<20, configured.maxLineSize)
	require.Equal(t, int64(64<<20), configured.limits.maxBytes)
}

type sseTransportFailureReader struct{ err error }

func (r sseTransportFailureReader) Read([]byte) (int, error) { return 0, r.err }

func TestAnthropicCompatTransportFailureNeverFinalizesSuccess(t *testing.T) {
	gin.SetMode(gin.TestMode)
	cfg := &config.Config{Gateway: config.GatewayConfig{MaxLineSize: 1 << 20, SSEEventMaxLines: 4, SSEQueueMaxBytes: 128}}
	gateway := &GatewayService{cfg: cfg}
	openai := &OpenAIGatewayService{cfg: cfg}
	mapping := apicompat.ResponsesClientToolMapping{}
	calls := []struct {
		name   string
		stream bool
		call   func(*http.Response, *gin.Context) (int, error)
	}{
		{"gateway_cc_buffered", false, func(resp *http.Response, c *gin.Context) (int, error) {
			r, e := gateway.handleCCBufferedFromAnthropic(resp, c, "m", "m", nil, time.Now())
			if r == nil {
				return 0, e
			}
			return r.Usage.InputTokens, e
		}},
		{"gateway_responses_buffered", false, func(resp *http.Response, c *gin.Context) (int, error) {
			r, e := gateway.handleResponsesBufferedStreamingResponse(resp, c, "m", "m", nil, time.Now(), mapping)
			if r == nil {
				return 0, e
			}
			return r.Usage.InputTokens, e
		}},
		{"native_cc_buffered", false, func(resp *http.Response, c *gin.Context) (int, error) {
			r, e := openai.handleCCBufferedFromNativeAnthropic(resp, c, "m", "m", "m", nil, time.Now())
			if r == nil {
				return 0, e
			}
			return r.Usage.InputTokens, e
		}},
		{"native_responses_buffered", false, func(resp *http.Response, c *gin.Context) (int, error) {
			r, e := openai.handleResponsesBufferedFromNativeAnthropic(resp, c, "m", "m", "m", nil, time.Now(), mapping)
			if r == nil {
				return 0, e
			}
			return r.Usage.InputTokens, e
		}},
		{"native_cc_stream", true, func(resp *http.Response, c *gin.Context) (int, error) {
			r, e := openai.handleCCStreamingFromNativeAnthropic(resp, c, "m", "m", "m", nil, time.Now())
			if r == nil {
				return 0, e
			}
			return r.Usage.InputTokens, e
		}},
		{"native_responses_stream", true, func(resp *http.Response, c *gin.Context) (int, error) {
			r, e := openai.handleResponsesStreamingFromNativeAnthropic(resp, c, "m", "m", "m", nil, time.Now(), mapping)
			if r == nil {
				return 0, e
			}
			return r.Usage.InputTokens, e
		}},
	}
	prefix := "event: message_start\ndata: {\"type\":\"message_start\",\"message\":{\"id\":\"msg_test\",\"model\":\"m\",\"usage\":{\"input_tokens\":3}}}\n\n"
	for _, call := range calls {
		for _, failure := range []string{"event_limit", "cancel", "truncated"} {
			t.Run(call.name+"/"+failure, func(t *testing.T) {
				var tail io.Reader
				switch failure {
				case "event_limit":
					tail = strings.NewReader(strings.Repeat(": missing delimiter\n", 5))
				case "cancel":
					tail = sseTransportFailureReader{context.Canceled}
				default:
					tail = strings.NewReader("")
				}
				rec := httptest.NewRecorder()
				c, _ := gin.CreateTestContext(rec)
				c.Request = httptest.NewRequest(http.MethodPost, "/", nil)
				resp := &http.Response{StatusCode: http.StatusOK, Header: http.Header{}, Body: io.NopCloser(io.MultiReader(strings.NewReader(prefix), tail))}
				defer func() { _ = resp.Body.Close() }()
				usage, err := call.call(resp, c)
				require.Error(t, err)
				if failure == "event_limit" {
					require.ErrorIs(t, err, errSSETransportLimit)
				}
				if failure == "cancel" {
					require.ErrorIs(t, err, context.Canceled)
				}
				require.Equal(t, 3, usage)
				if !call.stream {
					require.Equal(t, http.StatusBadGateway, rec.Code)
				}
				require.NotContains(t, rec.Body.String(), "response.completed")
				require.NotContains(t, rec.Body.String(), "data: [DONE]")
			})
		}
	}
}

func TestImageStreamLimitDoesNotFinalizePendingTerminal(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, async := range []bool{false, true} {
		for _, oauth := range []bool{false, true} {
			name := "direct"
			if oauth {
				name = "responses"
			}
			if async {
				name += "_async"
			} else {
				name += "_sync"
			}
			t.Run(name, func(t *testing.T) {
				cfg := &config.Config{Gateway: config.GatewayConfig{MaxLineSize: 1024, SSEQueueMaxBytes: 128}}
				if async {
					cfg.Gateway.ImageStreamDataIntervalTimeout = 1
				}
				svc := &OpenAIGatewayService{cfg: cfg}
				wire := "data: {\"type\":\"image_generation.completed\",\"b64_json\":\"AA==\"}\n"
				if oauth {
					wire = "data: {\"type\":\"response.completed\",\"response\":{\"output\":[{\"type\":\"image_generation_call\",\"result\":\"AA==\"}]}}\n"
				}
				wire += ":" + strings.Repeat("x", 2048)
				rec := httptest.NewRecorder()
				c, _ := gin.CreateTestContext(rec)
				c.Request = httptest.NewRequest(http.MethodPost, "/v1/images/generations", nil)
				resp := &http.Response{StatusCode: http.StatusOK, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(wire))}
				defer func() { _ = resp.Body.Close() }()
				var err error
				if oauth {
					_, _, _, _, err = svc.handleOpenAIImagesOAuthStreamingResponse(resp, c, time.Now(), "b64_json", "image_generation", "gpt-image-2")
				} else {
					_, _, _, _, err = svc.handleOpenAIImagesStreamingResponse(resp, c, time.Now(), &OpenAIImagesRequest{Model: "gpt-image-2", ResponseFormat: "b64_json"})
				}
				require.ErrorIs(t, err, errSSETransportLimit)
				require.NotContains(t, rec.Body.String(), "image_generation.completed")
				require.NotContains(t, rec.Body.String(), "response.completed")
			})
		}
	}
}
