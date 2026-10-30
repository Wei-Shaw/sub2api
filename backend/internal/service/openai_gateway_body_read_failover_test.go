//go:build unit

package service

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

const upstreamReadFailureDetail = "http2: stream error: stream ID 37; INTERNAL_ERROR; provider-secret"

func newOpenAIBodyReadTestContext(method, path string, body []byte) (*gin.Context, *httptest.ResponseRecorder) {
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(method, path, bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	return c, rec
}

func requireOpenAIBodyReadFailover(
	t *testing.T,
	c *gin.Context,
	err error,
	account *Account,
	requestID string,
) *UpstreamFailoverError {
	t.Helper()

	var failoverErr *UpstreamFailoverError
	require.ErrorAs(t, err, &failoverErr)
	require.True(t, failoverErr.ShouldRetryNextAccount())
	require.Equal(t, http.StatusBadGateway, failoverErr.StatusCode)
	require.Equal(t, requestID, failoverErr.ResponseHeaders.Get("x-request-id"))
	require.Contains(t, string(failoverErr.ResponseBody), "Upstream HTTP/2 stream failed")
	require.NotContains(t, string(failoverErr.ResponseBody), upstreamReadFailureDetail)
	require.False(t, c.Writer.Written(), "pre-output failover must not commit a client response")

	rawEvents, ok := c.Get(OpsUpstreamErrorsKey)
	require.True(t, ok)
	events, ok := rawEvents.([]*OpsUpstreamErrorEvent)
	require.True(t, ok)
	require.NotEmpty(t, events)
	event := events[len(events)-1]
	require.Equal(t, account.ID, event.AccountID)
	require.Equal(t, requestID, event.UpstreamRequestID)
	require.NotContains(t, event.Message, upstreamReadFailureDetail)
	return failoverErr
}

func TestOpenAIMessagesStream_PreOutputReadErrorFailsOverInSyncAndAsyncReaders(t *testing.T) {
	gin.SetMode(gin.TestMode)

	tests := []struct {
		name    string
		gateway config.GatewayConfig
	}{
		{name: "synchronous scanner"},
		{name: "asynchronous scanner", gateway: config.GatewayConfig{StreamKeepaliveInterval: 60}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, _ := newOpenAIBodyReadTestContext(http.MethodPost, "/v1/messages", nil)
			account := rawChatCompletionsTestAccount()
			requestID := "rid_messages_" + strings.ReplaceAll(tt.name, " ", "_")
			resp := &http.Response{
				StatusCode: http.StatusOK,
				Header:     http.Header{"Content-Type": []string{"text/event-stream"}, "X-Request-Id": []string{requestID}},
				Body:       &errTailReader{err: errors.New(upstreamReadFailureDetail)},
			}
			svc := &OpenAIGatewayService{cfg: &config.Config{Gateway: tt.gateway}}

			_, err := svc.handleAnthropicStreamingResponse(resp, c, account, "gpt-5.4", "gpt-5.4", "gpt-5.4", time.Now())

			requireOpenAIBodyReadFailover(t, c, err, account, requestID)
		})
	}
}

func TestOpenAIMessagesStream_CancellationDoesNotFailOver(t *testing.T) {
	gin.SetMode(gin.TestMode)

	c, _ := newOpenAIBodyReadTestContext(http.MethodPost, "/v1/messages", nil)
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}, "X-Request-Id": []string{"rid_messages_cancel"}},
		Body:       &errTailReader{err: context.Canceled},
	}
	svc := &OpenAIGatewayService{cfg: rawChatCompletionsTestConfig()}

	_, err := svc.handleAnthropicStreamingResponse(resp, c, rawChatCompletionsTestAccount(), "gpt-5.4", "gpt-5.4", "gpt-5.4", time.Now())

	require.ErrorIs(t, err, context.Canceled)
	var failoverErr *UpstreamFailoverError
	require.NotErrorAs(t, err, &failoverErr)
}

func TestOpenAIMessagesStream_PostOutputReadErrorDoesNotReplay(t *testing.T) {
	gin.SetMode(gin.TestMode)

	c, rec := newOpenAIBodyReadTestContext(http.MethodPost, "/v1/messages", nil)
	partial := strings.Join([]string{
		`data: {"type":"response.created","response":{"id":"resp_partial","object":"response","model":"gpt-5.4","status":"in_progress","output":[]}}`,
		"",
		`data: {"type":"response.output_text.delta","output_index":0,"content_index":0,"delta":"partial"}`,
		"",
		"",
	}, "\n")
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}, "X-Request-Id": []string{"rid_messages_partial"}},
		Body:       &errTailReader{data: []byte(partial), err: errors.New(upstreamReadFailureDetail)},
	}
	svc := &OpenAIGatewayService{cfg: rawChatCompletionsTestConfig()}

	_, err := svc.handleAnthropicStreamingResponse(resp, c, rawChatCompletionsTestAccount(), "gpt-5.4", "gpt-5.4", "gpt-5.4", time.Now())

	require.Error(t, err)
	var failoverErr *UpstreamFailoverError
	require.NotErrorAs(t, err, &failoverErr)
	require.True(t, c.Writer.Written())
	require.Contains(t, rec.Body.String(), `"text":"partial"`)
}

func TestOpenAIBufferedChatCompletions_PreOutputReadErrorFailsOver(t *testing.T) {
	gin.SetMode(gin.TestMode)

	tests := []struct {
		name string
		run  func(*testing.T, *OpenAIGatewayService, *gin.Context, *Account) error
	}{
		{
			name: "raw chat completions",
			run: func(t *testing.T, svc *OpenAIGatewayService, c *gin.Context, account *Account) error {
				t.Helper()
				resp := svc.httpUpstream.(*httpUpstreamRecorder).resp
				_, err := svc.bufferRawChatCompletions(c, resp, account, "gpt-5.4", "gpt-5.4", "gpt-5.4", nil, nil, time.Now())
				return err
			},
		},
		{
			name: "messages chat fallback",
			run: func(t *testing.T, svc *OpenAIGatewayService, c *gin.Context, account *Account) error {
				t.Helper()
				body := []byte(`{"model":"gpt-5.4","max_tokens":8,"messages":[{"role":"user","content":"hello"}],"stream":false}`)
				c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", bytes.NewReader(body))
				_, err := svc.ForwardAsAnthropic(context.Background(), c, account, body, "", "")
				return err
			},
		},
		{
			name: "responses chat fallback",
			run: func(t *testing.T, svc *OpenAIGatewayService, c *gin.Context, account *Account) error {
				t.Helper()
				body := []byte(`{"model":"gpt-5.4","input":"hello","stream":false}`)
				c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewReader(body))
				_, err := svc.Forward(context.Background(), c, account, body)
				return err
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, _ := newOpenAIBodyReadTestContext(http.MethodPost, "/", nil)
			requestID := "rid_buffered_" + strings.ReplaceAll(tt.name, " ", "_")
			upstream := &httpUpstreamRecorder{resp: &http.Response{
				StatusCode: http.StatusOK,
				Header:     http.Header{"Content-Type": []string{"application/json"}, "X-Request-Id": []string{requestID}},
				Body:       &errTailReader{err: errors.New(upstreamReadFailureDetail)},
			}}
			svc := &OpenAIGatewayService{cfg: rawChatCompletionsTestConfig(), httpUpstream: upstream}
			account := rawChatCompletionsTestAccount()
			switch tt.name {
			case "messages chat fallback":
				account = forceChatMessagesFallbackAccount()
			case "responses chat fallback":
				account = forceChatResponsesFallbackAccount()
			}

			err := tt.run(t, svc, c, account)

			requireOpenAIBodyReadFailover(t, c, err, account, requestID)
		})
	}
}

func TestOpenAIBufferedChatCompletions_ClientCancellationDoesNotFailOver(t *testing.T) {
	gin.SetMode(gin.TestMode)

	requestCtx, cancel := context.WithCancel(context.Background())
	cancel()
	c, _ := newOpenAIBodyReadTestContext(http.MethodPost, "/v1/chat/completions", nil)
	c.Request = c.Request.WithContext(requestCtx)
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/json"}, "X-Request-Id": []string{"rid_buffered_cancel"}},
		Body:       &errTailReader{err: errors.New(upstreamReadFailureDetail)},
	}
	svc := &OpenAIGatewayService{cfg: rawChatCompletionsTestConfig()}

	_, err := svc.bufferRawChatCompletions(c, resp, rawChatCompletionsTestAccount(), "gpt-5.4", "gpt-5.4", "gpt-5.4", nil, nil, time.Now())

	require.Error(t, err)
	var failoverErr *UpstreamFailoverError
	require.NotErrorAs(t, err, &failoverErr)
}
