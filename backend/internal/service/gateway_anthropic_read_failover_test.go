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
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

type anthropicReadErrorBody struct {
	err error
}

func (r *anthropicReadErrorBody) Read([]byte) (int, error) { return 0, r.err }
func (r *anthropicReadErrorBody) Close() error             { return nil }

func newAnthropicReadFailureResponse(err error) *http.Response {
	return &http.Response{
		StatusCode: http.StatusOK,
		Header: http.Header{
			"Content-Type": []string{"application/json"},
			"X-Request-Id": []string{"anthropic-read-failure"},
		},
		Body: &anthropicReadErrorBody{err: err},
	}
}

func newAnthropicReadFailureContext() (*gin.Context, *httptest.ResponseRecorder) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)
	return c, rec
}

type anthropicBufferedReadCall func(context.Context, *GatewayService, *gin.Context, *http.Response, *Account) error

func anthropicBufferedReadCalls() []struct {
	name string
	call anthropicBufferedReadCall
} {
	return []struct {
		name string
		call anthropicBufferedReadCall
	}{
		{
			name: "normal messages",
			call: func(ctx context.Context, svc *GatewayService, c *gin.Context, resp *http.Response, account *Account) error {
				_, err := svc.handleNonStreamingResponse(ctx, resp, c, account, "claude-sonnet-4-6", "claude-sonnet-4-6")
				return err
			},
		},
		{
			name: "api key passthrough",
			call: func(ctx context.Context, svc *GatewayService, c *gin.Context, resp *http.Response, account *Account) error {
				_, err := svc.handleNonStreamingResponseAnthropicAPIKeyPassthrough(ctx, resp, c, account)
				return err
			},
		},
	}
}

func TestAnthropicMessagesBufferedReadErrorFailsOverBeforeOutput(t *testing.T) {
	for _, test := range anthropicBufferedReadCalls() {
		t.Run(test.name, func(t *testing.T) {
			c, rec := newAnthropicReadFailureContext()
			svc := &GatewayService{
				cfg:              &config.Config{},
				rateLimitService: &RateLimitService{},
			}
			resp := newAnthropicReadFailureResponse(errors.New("read tcp 10.0.0.1:1234->52.1.2.3:443: stream error: stream ID 7; INTERNAL_ERROR; received from peer"))

			err := test.call(context.Background(), svc, c, resp, &Account{ID: 41, Platform: PlatformAnthropic})

			require.Error(t, err)
			var failoverErr *UpstreamFailoverError
			require.ErrorAs(t, err, &failoverErr)
			require.Equal(t, http.StatusBadGateway, failoverErr.StatusCode)
			require.True(t, failoverErr.RetryableOnSameAccount)
			require.Equal(t, "anthropic-read-failure", failoverErr.ResponseHeaders.Get("x-request-id"))
			require.Equal(t, "upstream_disconnected", gjson.GetBytes(failoverErr.ResponseBody, "error.type").String())
			require.Equal(t, "upstream stream disconnected: upstream connection error", gjson.GetBytes(failoverErr.ResponseBody, "error.message").String())
			require.NotContains(t, string(failoverErr.ResponseBody), "10.0.0.1")
			require.NotContains(t, string(failoverErr.ResponseBody), "stream ID 7")
			require.False(t, c.Writer.Written())
			require.Empty(t, rec.Body.String())
		})
	}
}

func TestAnthropicMessagesBufferedReadErrorDoesNotFailOverForExcludedErrors(t *testing.T) {
	tests := []struct {
		name          string
		ctx           context.Context
		readErr       error
		cancelRequest bool
	}{
		{name: "canceled read", ctx: canceledContext(), readErr: io.ErrUnexpectedEOF},
		{name: "deadline read", ctx: deadlineContext(), readErr: io.ErrUnexpectedEOF},
		{name: "canceled error", ctx: context.Background(), readErr: context.Canceled},
		{name: "deadline error", ctx: context.Background(), readErr: context.DeadlineExceeded},
		{name: "canceled request context", ctx: context.Background(), readErr: io.ErrUnexpectedEOF, cancelRequest: true},
	}

	for _, route := range anthropicBufferedReadCalls() {
		t.Run(route.name, func(t *testing.T) {
			for _, test := range tests {
				t.Run(test.name, func(t *testing.T) {
					c, rec := newAnthropicReadFailureContext()
					if test.cancelRequest {
						c.Request = c.Request.WithContext(canceledContext())
					}
					svc := &GatewayService{
						cfg:              &config.Config{},
						rateLimitService: &RateLimitService{},
					}
					err := route.call(test.ctx, svc, c, newAnthropicReadFailureResponse(test.readErr), &Account{ID: 42})

					require.Error(t, err)
					var failoverErr *UpstreamFailoverError
					require.NotErrorAs(t, err, &failoverErr)
					require.Empty(t, rec.Body.String())
				})
			}
		})
	}
}

func TestAnthropicMessagesBufferedReadErrorOversizeDoesNotFailOver(t *testing.T) {
	for _, route := range anthropicBufferedReadCalls() {
		t.Run(route.name, func(t *testing.T) {
			c, rec := newAnthropicReadFailureContext()
			svc := &GatewayService{
				cfg: &config.Config{Gateway: config.GatewayConfig{
					UpstreamResponseReadMaxBytes: 1,
				}},
				rateLimitService: &RateLimitService{},
			}
			resp := newAnthropicReadFailureResponse(nil)
			resp.Body = io.NopCloser(strings.NewReader("too large"))

			err := route.call(context.Background(), svc, c, resp, &Account{ID: 43})

			require.ErrorIs(t, err, ErrUpstreamResponseBodyTooLarge)
			var failoverErr *UpstreamFailoverError
			require.NotErrorAs(t, err, &failoverErr)
			require.NotEmpty(t, rec.Body.String())
		})
	}
}

func TestAnthropicMessagesBufferedReadErrorAfterOutputDoesNotFailOver(t *testing.T) {
	for _, route := range anthropicBufferedReadCalls() {
		t.Run(route.name, func(t *testing.T) {
			c, rec := newAnthropicReadFailureContext()
			_, writeErr := c.Writer.WriteString("already committed")
			require.NoError(t, writeErr)
			svc := &GatewayService{
				cfg:              &config.Config{},
				rateLimitService: &RateLimitService{},
			}

			err := route.call(context.Background(), svc, c, newAnthropicReadFailureResponse(io.ErrUnexpectedEOF), &Account{ID: 47})

			require.Error(t, err)
			var failoverErr *UpstreamFailoverError
			require.NotErrorAs(t, err, &failoverErr)
			require.Equal(t, "already committed", rec.Body.String())
		})
	}
}

func TestAnthropicPassthroughStreamingReadErrorFailsOverBeforeOutput(t *testing.T) {
	c, rec := newAnthropicReadFailureContext()
	svc := &GatewayService{cfg: &config.Config{Gateway: config.GatewayConfig{
		MaxLineSize: defaultMaxLineSize,
	}}}
	resp := newAnthropicReadFailureResponse(errors.New("read tcp 10.0.0.1:1234->52.1.2.3:443: stream error: stream ID 9; INTERNAL_ERROR; received from peer"))
	resp.Header.Set("Content-Type", "text/event-stream")

	result, err := svc.handleStreamingResponseAnthropicAPIKeyPassthrough(context.Background(), resp, c, &Account{ID: 44}, time.Now(), "claude-sonnet-4-6")

	require.Error(t, err)
	require.Nil(t, result)
	var failoverErr *UpstreamFailoverError
	require.ErrorAs(t, err, &failoverErr)
	require.Equal(t, http.StatusBadGateway, failoverErr.StatusCode)
	require.True(t, failoverErr.RetryableOnSameAccount)
	require.Equal(t, "upstream_disconnected", gjson.GetBytes(failoverErr.ResponseBody, "error.type").String())
	require.Equal(t, "upstream stream disconnected: upstream connection error", gjson.GetBytes(failoverErr.ResponseBody, "error.message").String())
	require.NotContains(t, string(failoverErr.ResponseBody), "10.0.0.1")
	require.NotContains(t, string(failoverErr.ResponseBody), "stream ID 9")
	require.False(t, c.Writer.Written())
	require.Empty(t, rec.Body.String())
}

func TestAnthropicPassthroughStreamingReadErrorAfterOutputDoesNotFailOver(t *testing.T) {
	c, rec := newAnthropicReadFailureContext()
	svc := &GatewayService{cfg: &config.Config{Gateway: config.GatewayConfig{
		MaxLineSize: defaultMaxLineSize,
	}}}
	resp := newAnthropicReadFailureResponse(nil)
	resp.Header.Set("Content-Type", "text/event-stream")
	resp.Body = &streamReadCloser{
		payload: []byte("data: {\"type\":\"message_start\",\"message\":{\"usage\":{\"input_tokens\":2}}}\n\n"),
		err:     io.ErrUnexpectedEOF,
	}

	result, err := svc.handleStreamingResponseAnthropicAPIKeyPassthrough(context.Background(), resp, c, &Account{ID: 46}, time.Now(), "claude-sonnet-4-6")

	require.Error(t, err)
	require.NotNil(t, result)
	var failoverErr *UpstreamFailoverError
	require.NotErrorAs(t, err, &failoverErr)
	require.Contains(t, err.Error(), "stream read error")
	require.Contains(t, rec.Body.String(), "message_start")
	require.True(t, c.Writer.Written())
}

func TestAnthropicPassthroughStreamingReadErrorDoesNotFailOverForExcludedErrors(t *testing.T) {
	tests := []struct {
		name          string
		ctx           context.Context
		readErr       error
		cancelRequest bool
	}{
		{name: "canceled context", ctx: canceledContext(), readErr: io.ErrUnexpectedEOF},
		{name: "deadline context", ctx: deadlineContext(), readErr: io.ErrUnexpectedEOF},
		{name: "canceled read", ctx: context.Background(), readErr: context.Canceled},
		{name: "deadline read", ctx: context.Background(), readErr: context.DeadlineExceeded},
		{name: "oversized line", ctx: context.Background(), readErr: bufio.ErrTooLong},
		{name: "canceled request context", ctx: context.Background(), readErr: io.ErrUnexpectedEOF, cancelRequest: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			c, _ := newAnthropicReadFailureContext()
			if test.cancelRequest {
				c.Request = c.Request.WithContext(canceledContext())
			}
			svc := &GatewayService{cfg: &config.Config{Gateway: config.GatewayConfig{
				MaxLineSize: defaultMaxLineSize,
			}}}
			resp := newAnthropicReadFailureResponse(test.readErr)
			resp.Header.Set("Content-Type", "text/event-stream")

			result, err := svc.handleStreamingResponseAnthropicAPIKeyPassthrough(test.ctx, resp, c, &Account{ID: 45}, time.Now(), "claude-sonnet-4-6")

			require.Error(t, err)
			require.NotNil(t, result)
			var failoverErr *UpstreamFailoverError
			require.NotErrorAs(t, err, &failoverErr)
		})
	}
}

func canceledContext() context.Context {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	return ctx
}

func deadlineContext() context.Context {
	ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
	cancel()
	return ctx
}
