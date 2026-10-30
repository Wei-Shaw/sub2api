package service

import (
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/apicompat"
	"github.com/gin-gonic/gin"
)

func nativeAnthropicPreambleStream() string {
	return strings.Join([]string{
		"event: message_start",
		`data: {"type":"message_start","message":{"id":"msg_failover","type":"message","role":"assistant","content":[],"model":"glm-4.7","usage":{"input_tokens":10,"output_tokens":1}}}`,
		"",
	}, "\n")
}

func nativeAnthropicSemanticStream() string {
	return nativeAnthropicPreambleStream() + strings.Join([]string{
		"event: content_block_start",
		`data: {"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}`,
		"",
		"event: content_block_delta",
		`data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"Hello"}}`,
		"",
	}, "\n")
}

func newNativeAnthropicFailoverTestContext(t *testing.T) (*gin.Context, *httptest.ResponseRecorder) {
	t.Helper()
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/", nil)
	return c, rec
}

func TestNativeAnthropicConversion_ReadErrorBeforeOutputTriggersFailover(t *testing.T) {
	gin.SetMode(gin.TestMode)
	cause := errors.New("read tcp 10.0.0.1:1234: reset internal-secret")

	tests := []struct {
		name string
		run  func(*OpenAIGatewayService, *http.Response, *gin.Context) (*OpenAIForwardResult, error)
	}{
		{
			name: "chat_completions_buffered",
			run: func(s *OpenAIGatewayService, resp *http.Response, c *gin.Context) (*OpenAIForwardResult, error) {
				return s.handleCCBufferedFromNativeAnthropic(resp, c, "glm-4.7", "glm-4.7", "glm-4.7", nil, time.Now())
			},
		},
		{
			name: "chat_completions_streaming",
			run: func(s *OpenAIGatewayService, resp *http.Response, c *gin.Context) (*OpenAIForwardResult, error) {
				return s.handleCCStreamingFromNativeAnthropic(resp, c, "glm-4.7", "glm-4.7", "glm-4.7", nil, time.Now())
			},
		},
		{
			name: "responses_buffered",
			run: func(s *OpenAIGatewayService, resp *http.Response, c *gin.Context) (*OpenAIForwardResult, error) {
				return s.handleResponsesBufferedFromNativeAnthropic(resp, c, "glm-4.7", "glm-4.7", "glm-4.7", nil, time.Now(), apicompat.ResponsesClientToolMapping{})
			},
		},
		{
			name: "responses_streaming",
			run: func(s *OpenAIGatewayService, resp *http.Response, c *gin.Context) (*OpenAIForwardResult, error) {
				return s.handleResponsesStreamingFromNativeAnthropic(resp, c, "glm-4.7", "glm-4.7", "glm-4.7", nil, time.Now(), apicompat.ResponsesClientToolMapping{})
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			svc := newNativeAnthropicHangTestService(0)
			c, rec := newNativeAnthropicFailoverTestContext(t)
			before := c.Writer.Size()
			resp := &http.Response{
				StatusCode: http.StatusOK,
				Header: http.Header{
					"Content-Type": []string{"text/event-stream"},
					"X-Request-Id": []string{"req_native_failover"},
				},
				Body: &streamReadCloser{payload: []byte(nativeAnthropicPreambleStream()), err: cause},
			}

			result, err := tc.run(svc, resp, c)
			if err == nil {
				t.Fatal("expected upstream read error")
			}
			if result != nil {
				t.Fatalf("failover must not return a billable partial result: %+v", result)
			}
			var failoverErr *UpstreamFailoverError
			if !errors.As(err, &failoverErr) {
				t.Fatalf("pre-output read error must trigger failover, got %T: %v", err, err)
			}
			if failoverErr.StatusCode != http.StatusBadGateway {
				t.Fatalf("expected 502 failover status, got %d", failoverErr.StatusCode)
			}
			body := string(failoverErr.ResponseBody)
			if !strings.Contains(body, OpenAIUpstreamStreamReadErrorCode) || !strings.Contains(body, "Upstream response stream was interrupted") {
				t.Fatalf("expected stable sanitized read-error payload, got %q", body)
			}
			for _, leaked := range []string{"10.0.0.1", "1234", "internal-secret"} {
				if strings.Contains(body, leaked) {
					t.Fatalf("failover payload leaked %q: %s", leaked, body)
				}
			}
			if c.Writer.Size() != before || c.Writer.Written() || rec.Body.Len() != 0 {
				t.Fatalf("pre-output failover must leave route handler eligible: before=%d after=%d written=%v body=%q", before, c.Writer.Size(), c.Writer.Written(), rec.Body.String())
			}
			if got := rec.Header().Get("Content-Type"); got != "" {
				t.Fatalf("failed attempt headers leaked downstream: Content-Type=%q", got)
			}
		})
	}
}

func TestNativeAnthropicStreaming_ReadErrorAfterSemanticOutputDoesNotFailover(t *testing.T) {
	gin.SetMode(gin.TestMode)
	cause := errors.New("upstream reset after output")

	tests := []struct {
		name string
		run  func(*OpenAIGatewayService, *http.Response, *gin.Context) (*OpenAIForwardResult, error)
	}{
		{
			name: "chat_completions",
			run: func(s *OpenAIGatewayService, resp *http.Response, c *gin.Context) (*OpenAIForwardResult, error) {
				return s.handleCCStreamingFromNativeAnthropic(resp, c, "glm-4.7", "glm-4.7", "glm-4.7", nil, time.Now())
			},
		},
		{
			name: "responses",
			run: func(s *OpenAIGatewayService, resp *http.Response, c *gin.Context) (*OpenAIForwardResult, error) {
				return s.handleResponsesStreamingFromNativeAnthropic(resp, c, "glm-4.7", "glm-4.7", "glm-4.7", nil, time.Now(), apicompat.ResponsesClientToolMapping{})
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			svc := newNativeAnthropicHangTestService(0)
			c, rec := newNativeAnthropicFailoverTestContext(t)
			resp := &http.Response{
				StatusCode: http.StatusOK,
				Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
				Body:       &streamReadCloser{payload: []byte(nativeAnthropicSemanticStream()), err: cause},
			}

			result, err := tc.run(svc, resp, c)
			if err == nil {
				t.Fatal("expected upstream read error")
			}
			if result == nil {
				t.Fatal("post-output failure must retain the partial result for usage accounting")
			}
			var failoverErr *UpstreamFailoverError
			if errors.As(err, &failoverErr) {
				t.Fatalf("semantic output must prevent replay, got failover error: %+v", failoverErr)
			}
			code, message, ok := OpenAIUpstreamStreamReadErrorDetails(err)
			if !ok || code != OpenAIUpstreamStreamReadErrorCode || message != "Upstream response stream was interrupted" {
				t.Fatalf("expected sanitized post-output read error, got code=%q message=%q ok=%v err=%v", code, message, ok, err)
			}
			if !strings.Contains(rec.Body.String(), "Hello") {
				t.Fatalf("expected semantic output before failure, got %q", rec.Body.String())
			}
			if strings.Contains(rec.Body.String(), "[DONE]") || strings.Contains(rec.Body.String(), "response.completed") {
				t.Fatalf("truncated stream must not be finalized as success: %q", rec.Body.String())
			}
		})
	}
}

func TestNativeAnthropicConversion_CleanEOFWithoutTerminalIsNotSuccessOrFailover(t *testing.T) {
	gin.SetMode(gin.TestMode)

	tests := []struct {
		name string
		run  func(*OpenAIGatewayService, *http.Response, *gin.Context) (*OpenAIForwardResult, error)
	}{
		{
			name: "chat_completions_buffered",
			run: func(s *OpenAIGatewayService, resp *http.Response, c *gin.Context) (*OpenAIForwardResult, error) {
				return s.handleCCBufferedFromNativeAnthropic(resp, c, "glm-4.7", "glm-4.7", "glm-4.7", nil, time.Now())
			},
		},
		{
			name: "chat_completions_streaming",
			run: func(s *OpenAIGatewayService, resp *http.Response, c *gin.Context) (*OpenAIForwardResult, error) {
				return s.handleCCStreamingFromNativeAnthropic(resp, c, "glm-4.7", "glm-4.7", "glm-4.7", nil, time.Now())
			},
		},
		{
			name: "responses_buffered",
			run: func(s *OpenAIGatewayService, resp *http.Response, c *gin.Context) (*OpenAIForwardResult, error) {
				return s.handleResponsesBufferedFromNativeAnthropic(resp, c, "glm-4.7", "glm-4.7", "glm-4.7", nil, time.Now(), apicompat.ResponsesClientToolMapping{})
			},
		},
		{
			name: "responses_streaming",
			run: func(s *OpenAIGatewayService, resp *http.Response, c *gin.Context) (*OpenAIForwardResult, error) {
				return s.handleResponsesStreamingFromNativeAnthropic(resp, c, "glm-4.7", "glm-4.7", "glm-4.7", nil, time.Now(), apicompat.ResponsesClientToolMapping{})
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			svc := newNativeAnthropicHangTestService(0)
			c, rec := newNativeAnthropicFailoverTestContext(t)
			resp := &http.Response{
				StatusCode: http.StatusOK,
				Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
				Body:       &streamReadCloser{payload: []byte(nativeAnthropicPreambleStream()), err: io.EOF},
			}

			_, err := tc.run(svc, resp, c)
			if err == nil {
				t.Fatal("clean EOF before message_stop must not be reported as success")
			}
			var failoverErr *UpstreamFailoverError
			if errors.As(err, &failoverErr) {
				t.Fatalf("clean EOF is not a transport read failure eligible for replay: %+v", failoverErr)
			}
			code, _, ok := OpenAIUpstreamStreamReadErrorDetails(err)
			if !ok || code != OpenAIUpstreamStreamTruncatedCode {
				t.Fatalf("expected typed truncated-stream error, got code=%q ok=%v err=%v", code, ok, err)
			}
			if strings.Contains(rec.Body.String(), "[DONE]") || strings.Contains(rec.Body.String(), "response.completed") {
				t.Fatalf("clean truncation must not synthesize success: %q", rec.Body.String())
			}
		})
	}
}
