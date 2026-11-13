package service

import (
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestChatPreambleKeepaliveDoesNotCommitBeforeRetry(t *testing.T) {
	reader, writer := io.Pipe()
	defer reader.Close()
	done := make(chan struct{})
	go func() {
		defer close(done)
		defer writer.Close()
		_, _ = io.WriteString(writer, "data: {\"type\":\"response.created\",\"response\":{\"id\":\"resp_1\",\"status\":\"in_progress\"}}\n\n")
		time.Sleep(1200 * time.Millisecond)
		_, _ = io.WriteString(writer, "data: {\"type\":\"response.failed\",\"response\":{\"status\":\"failed\",\"error\":{\"code\":\"server_is_overloaded\",\"message\":\"Our servers are currently overloaded. Please try again later.\"}}}\n\n")
	}()
	t.Cleanup(func() { _ = reader.Close(); <-done })
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	cfg := &config.Config{}
	cfg.Gateway.StreamKeepaliveInterval = 1
	svc := &OpenAIGatewayService{cfg: cfg}
	result, err := svc.handleChatStreamingResponse(
		&http.Response{StatusCode: 200, Header: http.Header{"Content-Type": []string{"text/event-stream"}}, Body: reader},
		c, &Account{ID: 1, Platform: PlatformOpenAI, Type: AccountTypeOAuth},
		"gpt-6-astra", "gpt-6-astra", "gpt-6-astra", time.Now(), 100,
	)
	var failover *UpstreamFailoverError
	require.True(t, errors.As(err, &failover))
	require.True(t, failover.RetryableOnSameAccount)
	require.Nil(t, result)
	require.False(t, c.Writer.Written(), "a keepalive must not commit the pending role-only preamble")
	require.Empty(t, rec.Body.String())
}
