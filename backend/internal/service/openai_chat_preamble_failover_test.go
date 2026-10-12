package service

import (
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
)

const chatPreambleFixture = "data: {\"type\":\"response.created\",\"response\":{\"id\":\"resp_test\",\"status\":\"in_progress\"}}\n\n"
const chatBusyFixture = "data: {\"type\":\"response.failed\",\"response\":{\"status\":\"failed\",\"error\":{\"code\":\"server_error\",\"message\":\"Our servers are currently overloaded. Please try again later.\"}}}\n\n"

func TestChatPreambleFailover(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, tc := range []struct {
		name, progress string
		bodySize       int
		wantFailover   bool
	}{
		{"small request preamble", "", 220, true},
		{"large request preamble", "", 95000, true},
		{"text already sent", "data: {\"type\":\"response.output_text.delta\",\"delta\":\"Hi\"}\n\n", 220, false},
		{"reasoning already sent", "data: {\"type\":\"response.reasoning_text.delta\",\"delta\":\"thinking\"}\n\n", 220, false},
		{"tool already sent", "data: {\"type\":\"response.output_item.added\",\"output_index\":0,\"item\":{\"type\":\"function_call\",\"id\":\"fc_1\",\"call_id\":\"call_1\",\"name\":\"run\",\"arguments\":\"\"}}\n\n", 220, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(rec)
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
			resp := &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(chatPreambleFixture + tc.progress + chatBusyFixture))}
			svc := &OpenAIGatewayService{cfg: &config.Config{}}
			result, err := svc.handleChatStreamingResponse(resp, c, &Account{ID: 1, Platform: PlatformOpenAI, Type: AccountTypeAPIKey}, "gpt-5.6-sol", "gpt-5.6-sol", "gpt-5.6-sol", time.Now(), tc.bodySize)
			require.Error(t, err)
			var failover *UpstreamFailoverError
			require.Equal(t, tc.wantFailover, errors.As(err, &failover))
			if tc.wantFailover {
				require.Nil(t, result, "failed attempt must not submit usage before retry")
				require.False(t, c.Writer.Written())
				require.Empty(t, rec.Body.String())
			} else {
				require.NotNil(t, result)
				require.Contains(t, rec.Body.String(), "Our servers are currently overloaded")
			}
		})
	}
}

func TestChatPreambleKeepsOneTokenCompletion(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	stream := chatPreambleFixture + "data: {\"type\":\"response.output_text.delta\",\"delta\":\"1\"}\n\n" +
		"data: {\"type\":\"response.completed\",\"response\":{\"status\":\"completed\",\"usage\":{\"input_tokens\":29,\"output_tokens\":1}}}\n\n"
	resp := &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(stream))}
	svc := &OpenAIGatewayService{cfg: &config.Config{}}
	result, err := svc.handleChatStreamingResponse(resp, c, &Account{ID: 1, Platform: PlatformOpenAI}, "gpt-6-astra", "gpt-6-astra", "gpt-6-astra", time.Now(), 220)
	require.NoError(t, err)
	require.Equal(t, 1, result.Usage.OutputTokens)
	require.Equal(t, 29, result.Usage.InputTokens)
	require.Contains(t, rec.Body.String(), `"content":"1"`)
	require.Equal(t, 1, strings.Count(rec.Body.String(), "[DONE]"))
}

func TestChatPreambleHeartbeatThenRetry(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	reader, writer := io.Pipe()
	defer reader.Close()
	finished := make(chan struct{})
	go func() {
		defer close(finished)
		defer writer.Close()
		_, _ = io.WriteString(writer, chatPreambleFixture)
		time.Sleep(1200 * time.Millisecond)
		_, _ = io.WriteString(writer, chatBusyFixture)
	}()
	cfg := &config.Config{}
	cfg.Gateway.StreamKeepaliveInterval = 1
	svc := &OpenAIGatewayService{cfg: cfg}
	account := &Account{ID: 1, Platform: PlatformOpenAI, Type: AccountTypeAPIKey}
	resp := &http.Response{StatusCode: 200, Header: http.Header{}, Body: reader}
	before := OpenAICompactKeepaliveAdjustedWrittenSize(c)
	result, err := svc.handleChatStreamingResponse(resp, c, account, "gpt-5.6-sol", "gpt-5.6-sol", "gpt-5.6-sol", time.Now(), 220)
	<-finished
	var failover *UpstreamFailoverError
	require.ErrorAs(t, err, &failover)
	require.Nil(t, result, "discarded attempt cannot create a usage record")
	require.Equal(t, ":\n\n", rec.Body.String(), "heartbeat must survive, failed response preamble must not")
	require.Equal(t, before, OpenAICompactKeepaliveAdjustedWrittenSize(c), "handler must still permit retry")

	// A second attempt can reuse the committed SSE connection and deliver the
	// only completion and billable result, without leaking the failed preamble.
	stream := strings.ReplaceAll(chatPreambleFixture, "resp_test", "resp_success") +
		"data: {\"type\":\"response.output_text.delta\",\"delta\":\"OK\"}\n\n" +
		"data: {\"type\":\"response.completed\",\"response\":{\"usage\":{\"input_tokens\":10,\"output_tokens\":1}}}\n\n"
	resp.Body = io.NopCloser(strings.NewReader(stream))
	result, err = svc.handleChatStreamingResponse(resp, c, account, "gpt-5.6-sol", "gpt-5.6-sol", "gpt-5.6-sol", time.Now(), 220)
	require.NoError(t, err)
	require.Equal(t, 10, result.Usage.InputTokens)
	require.Equal(t, 1, result.Usage.OutputTokens)
	require.NotContains(t, rec.Body.String(), "resp_test")
	require.Contains(t, rec.Body.String(), "resp_success")
	require.Equal(t, 1, strings.Count(rec.Body.String(), `"role":"assistant"`))
	require.Equal(t, 1, strings.Count(rec.Body.String(), "[DONE]"))
}

func TestChatPreambleDoesNotRetryCanceledClient(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil).WithContext(ctx)
	resp := &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(chatPreambleFixture + chatBusyFixture))}
	svc := &OpenAIGatewayService{cfg: &config.Config{}}
	_, err := svc.handleChatStreamingResponse(resp, c, &Account{ID: 1, Platform: PlatformOpenAI}, "gpt-5.6-sol", "gpt-5.6-sol", "gpt-5.6-sol", time.Now(), 220)
	require.Error(t, err)
	var failover *UpstreamFailoverError
	require.False(t, errors.As(err, &failover))
}
