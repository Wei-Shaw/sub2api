package service

import (
	"errors"
	"io"
	"net/http"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/apicompat"
	"github.com/gin-gonic/gin"
)

func TestGatewayAnthropicCompatPreOutputFailoverDoesNotCommitSSEHeaders(t *testing.T) {
	gin.SetMode(gin.TestMode)
	readErr := errors.New("read tcp 10.0.0.1:1234: connection reset by peer")

	tests := []struct {
		name      string
		wantFrame string
		run       func(*GatewayService, *http.Response, *gin.Context) error
	}{
		{
			name:      "chat_completions",
			wantFrame: "[DONE]",
			run: func(s *GatewayService, resp *http.Response, c *gin.Context) error {
				_, err := s.handleCCStreamingFromAnthropic(resp, c, "claude-3-7-sonnet", "claude-3-7-sonnet", nil, time.Now())
				return err
			},
		},
		{
			name:      "responses",
			wantFrame: "response.completed",
			run: func(s *GatewayService, resp *http.Response, c *gin.Context) error {
				_, err := s.handleResponsesStreamingResponse(resp, c, "claude-3-7-sonnet", "claude-3-7-sonnet", nil, time.Now(), apicompat.ResponsesClientToolMapping{})
				return err
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			c, rec := newNativeAnthropicFailoverTestContext(t)
			svc := &GatewayService{}
			failedResp := &http.Response{
				StatusCode: http.StatusOK,
				Header:     http.Header{"Content-Type": []string{"text/event-stream"}, "X-Request-Id": []string{"req_gateway_failover"}},
				Body:       &streamReadCloser{err: readErr},
			}

			if err := tc.run(svc, failedResp, c); err == nil {
				t.Fatal("expected an eligible upstream read failure")
			} else {
				var failoverErr *UpstreamFailoverError
				if !errors.As(err, &failoverErr) {
					t.Fatalf("expected retryable upstream failure, got %T: %v", err, err)
				}
			}
			if rec.Body.Len() != 0 || c.Writer.Written() {
				t.Fatalf("pre-output failover committed response: status=%d body=%q", rec.Code, rec.Body.String())
			}
			for _, header := range []string{"Content-Type", "Cache-Control", "Connection", "X-Accel-Buffering", "X-Request-Id"} {
				if got := c.Writer.Header().Get(header); got != "" {
					t.Fatalf("pre-output failover left stale %s %q", header, got)
				}
			}

			// A successful retry on the same Gin context must still be able to install
			// its SSE headers and complete the stream.
			retryResp := &http.Response{
				StatusCode: http.StatusOK,
				Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
				Body:       io.NopCloser(strings.NewReader(miniAnthropicSSEStream())),
			}
			if err := tc.run(svc, retryResp, c); err != nil {
				t.Fatalf("successful retry failed: %v", err)
			}
			if got := c.Writer.Header().Get("Content-Type"); got != "text/event-stream" {
				t.Fatalf("retry Content-Type = %q, want text/event-stream", got)
			}
			if !strings.Contains(rec.Body.String(), tc.wantFrame) {
				t.Fatalf("successful retry lacks %q: %q", tc.wantFrame, rec.Body.String())
			}
		})
	}
}

func TestGatewayAnthropicCompatExhaustedPreOutputFailoverCanWriteJSON(t *testing.T) {
	gin.SetMode(gin.TestMode)
	readErr := errors.New("read tcp 10.0.0.1:1234: connection reset by peer")

	tests := []struct {
		name string
		run  func(*GatewayService, *http.Response, *gin.Context) error
	}{
		{
			name: "chat_completions",
			run: func(s *GatewayService, resp *http.Response, c *gin.Context) error {
				_, err := s.handleCCStreamingFromAnthropic(resp, c, "claude-3-7-sonnet", "claude-3-7-sonnet", nil, time.Now())
				return err
			},
		},
		{
			name: "responses",
			run: func(s *GatewayService, resp *http.Response, c *gin.Context) error {
				_, err := s.handleResponsesStreamingResponse(resp, c, "claude-3-7-sonnet", "claude-3-7-sonnet", nil, time.Now(), apicompat.ResponsesClientToolMapping{})
				return err
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			c, rec := newNativeAnthropicFailoverTestContext(t)
			resp := &http.Response{
				StatusCode: http.StatusOK,
				Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
				Body:       &streamReadCloser{err: readErr},
			}
			err := tc.run(&GatewayService{}, resp, c)
			var failoverErr *UpstreamFailoverError
			if !errors.As(err, &failoverErr) {
				t.Fatalf("expected retryable upstream failure, got %T: %v", err, err)
			}
			if c.Writer.Written() || rec.Body.Len() != 0 {
				t.Fatalf("exhausted retry path already committed response: status=%d body=%q", rec.Code, rec.Body.String())
			}

			// This is the same response shape used after the handler exhausts account
			// retries; stale SSE headers must not override it.
			c.JSON(http.StatusBadGateway, gin.H{"error": gin.H{"message": "all upstreams failed"}})
			if rec.Code != http.StatusBadGateway {
				t.Fatalf("fallback status = %d, want 502", rec.Code)
			}
			if got := rec.Header().Get("Content-Type"); !strings.HasPrefix(got, "application/json") {
				t.Fatalf("fallback Content-Type = %q, want application/json", got)
			}
		})
	}
}

func TestGatewayAnthropicCompatInBandSSEErrorBeforeOutputRestoresHeaders(t *testing.T) {
	gin.SetMode(gin.TestMode)
	const stream = `event: error
data: {"type":"error","error":{"type":"overloaded_error","message":"busy"}}

`

	tests := []struct {
		name string
		run  func(*GatewayService, *http.Response, *gin.Context) error
	}{
		{
			name: "chat_completions",
			run: func(s *GatewayService, resp *http.Response, c *gin.Context) error {
				_, err := s.handleCCStreamingFromAnthropic(resp, c, "claude-3-7-sonnet", "claude-3-7-sonnet", nil, time.Now())
				return err
			},
		},
		{
			name: "responses",
			run: func(s *GatewayService, resp *http.Response, c *gin.Context) error {
				_, err := s.handleResponsesStreamingResponse(resp, c, "claude-3-7-sonnet", "claude-3-7-sonnet", nil, time.Now(), apicompat.ResponsesClientToolMapping{})
				return err
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			c, rec := newNativeAnthropicFailoverTestContext(t)
			c.Writer.Header().Set("X-Request-Id", "gateway-request")
			c.Writer.Header().Add("Vary", "Origin")
			c.Writer.Header().Add("Vary", "Accept-Encoding")
			original := c.Writer.Header().Clone()
			resp := &http.Response{
				StatusCode: http.StatusOK,
				Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
				Body:       io.NopCloser(strings.NewReader(stream)),
			}
			err := tc.run(&GatewayService{}, resp, c)
			var sseErr *sseStreamErrorEventError
			if !errors.As(err, &sseErr) {
				t.Fatalf("expected typed in-band SSE error, got %T: %v", err, err)
			}
			if c.Writer.Written() || rec.Body.Len() != 0 {
				t.Fatalf("in-band error committed response before failover: status=%d body=%q", rec.Code, rec.Body.String())
			}
			if !reflect.DeepEqual(c.Writer.Header(), original) {
				t.Fatalf("headers after in-band error = %v, want %v", c.Writer.Header(), original)
			}

			c.JSON(http.StatusBadGateway, gin.H{"error": gin.H{"message": "all upstreams failed"}})
			if got := rec.Header().Get("Content-Type"); !strings.HasPrefix(got, "application/json") {
				t.Fatalf("fallback Content-Type = %q, want application/json", got)
			}
		})
	}
}
