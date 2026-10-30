//go:build unit

package service

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/tlsfingerprint"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type contextBoundBlockingReadCloser struct {
	data       []byte
	offset     int
	ctx        context.Context
	forceClose chan struct{}
	closeOnce  sync.Once
}

func newContextBoundBlockingReadCloser(data []byte) *contextBoundBlockingReadCloser {
	return &contextBoundBlockingReadCloser{
		data:       data,
		forceClose: make(chan struct{}),
	}
}

func (r *contextBoundBlockingReadCloser) Read(p []byte) (int, error) {
	if r.offset < len(r.data) {
		n := copy(p, r.data[r.offset:])
		r.offset += n
		return n, nil
	}
	select {
	case <-r.ctx.Done():
		return 0, r.ctx.Err()
	case <-r.forceClose:
		return 0, io.EOF
	}
}

func (r *contextBoundBlockingReadCloser) Close() error {
	select {
	case <-r.ctx.Done():
	case <-r.forceClose:
	}
	return nil
}

func (r *contextBoundBlockingReadCloser) forceUnblock() {
	r.closeOnce.Do(func() { close(r.forceClose) })
}

type contextBoundHTTPUpstream struct {
	body *contextBoundBlockingReadCloser
}

func (u *contextBoundHTTPUpstream) Do(req *http.Request, _ string, _ int64, _ int) (*http.Response, error) {
	u.body.ctx = req.Context()
	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
		Body:       u.body,
	}, nil
}

func (u *contextBoundHTTPUpstream) DoWithTLS(req *http.Request, proxyURL string, accountID int64, accountConcurrency int, _ *tlsfingerprint.Profile) (*http.Response, error) {
	return u.Do(req, proxyURL, accountID, accountConcurrency)
}

func TestForwardAsChatCompletions_CancelsUpstreamBeforeClosingBody(t *testing.T) {
	gin.SetMode(gin.TestMode)

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	body := []byte(`{"model":"gpt-5.4","messages":[{"role":"user","content":"hello"}],"stream":true}`)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(string(body)))
	c.Request.Header.Set("Content-Type", "application/json")

	upstreamBody := []byte("data: {\"type\":\"response.completed\",\"response\":{\"id\":\"resp_1\",\"object\":\"response\",\"model\":\"gpt-5.4\",\"status\":\"completed\",\"output\":[{\"type\":\"message\",\"id\":\"msg_1\",\"role\":\"assistant\",\"status\":\"completed\",\"content\":[{\"type\":\"output_text\",\"text\":\"ok\"}]}],\"usage\":{\"input_tokens\":17,\"output_tokens\":8,\"total_tokens\":25}}}\n\n")
	stream := newContextBoundBlockingReadCloser(upstreamBody)
	t.Cleanup(stream.forceUnblock)

	cfg := rawChatCompletionsTestConfig()
	cfg.Gateway.StreamKeepaliveInterval = 1
	svc := &OpenAIGatewayService{
		cfg:          cfg,
		httpUpstream: &contextBoundHTTPUpstream{body: stream},
	}

	type forwardResult struct {
		result *OpenAIForwardResult
		err    error
	}
	resultCh := make(chan forwardResult, 1)
	go func() {
		result, err := svc.ForwardAsChatCompletions(context.Background(), c, rawChatCompletionsTestAccount(), body, "", "gpt-5.1")
		resultCh <- forwardResult{result: result, err: err}
	}()

	select {
	case got := <-resultCh:
		require.NoError(t, got.err)
		require.NotNil(t, got.result)
		require.Equal(t, 17, got.result.Usage.InputTokens)
	case <-time.After(time.Second):
		t.Fatal("ForwardAsChatCompletions did not cancel upstream before closing the body")
	}
}
