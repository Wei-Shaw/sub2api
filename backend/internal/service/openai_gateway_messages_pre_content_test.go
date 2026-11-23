//go:build unit

package service

import (
	"bytes"
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

const (
	preContentCreatedFrame   = `data: {"type":"response.created","response":{"id":"resp_pre","object":"response","model":"gpt-5.4","status":"in_progress","output":[]}}` + "\n\n"
	preContentFailoverFailed = `data: {"type":"response.failed","response":{"status":"failed","error":{"code":"server_error","message":"An error occurred while processing your request."}}}` + "\n\n"
)

func newPreContentContext() (*httptest.ResponseRecorder, *gin.Context, []byte) {
	gin.SetMode(gin.TestMode)
	body := []byte(`{"model":"gpt-5.4","max_tokens":32,"messages":[{"role":"user","content":"hello"}],"stream":true}`)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	return rec, c, body
}

func forwardPreContent(t *testing.T, cfg *config.Config, header http.Header, upstreamBody io.ReadCloser) (*httptest.ResponseRecorder, *gin.Context, error) {
	t.Helper()
	rec, c, body := newPreContentContext()
	if header == nil {
		header = http.Header{}
	}
	header.Set("Content-Type", "text/event-stream")
	upstream := &httpUpstreamRecorder{resp: &http.Response{StatusCode: http.StatusOK, Header: header, Body: upstreamBody}}
	svc := &OpenAIGatewayService{cfg: cfg, httpUpstream: upstream, responseHeaderFilter: compileResponseHeaderFilter(cfg)}
	_, err := svc.ForwardAsAnthropic(context.Background(), c, rawChatCompletionsTestAccount(), body, "", "")
	return rec, c, err
}

func TestForwardAsAnthropic_PreContentFailedFailsOver(t *testing.T) {
	stream := preContentCreatedFrame +
		`data: {"type":"response.in_progress","response":{"id":"resp_pre","object":"response","model":"gpt-5.4","status":"in_progress","output":[]}}` + "\n\n" +
		`data: {"type":"response.output_item.added","output_index":0,"item":{"id":"rs_1","type":"reasoning","summary":[]}}` + "\n\n" +
		preContentFailoverFailed
	rec, c, err := forwardPreContent(t, rawChatCompletionsTestConfig(), nil, io.NopCloser(strings.NewReader(stream)))

	var failoverErr *UpstreamFailoverError
	require.True(t, errors.As(err, &failoverErr), "failure before content must fail over: %T: %v", err, err)
	require.Empty(t, rec.Body.String())
	require.False(t, c.Writer.Written())
}

func TestForwardAsAnthropic_PreContentMissingTerminalFailsOver(t *testing.T) {
	rec, c, err := forwardPreContent(t, rawChatCompletionsTestConfig(), nil, io.NopCloser(strings.NewReader(preContentCreatedFrame)))

	var failoverErr *UpstreamFailoverError
	require.True(t, errors.As(err, &failoverErr), "EOF without a terminal event before content must fail over: %T: %v", err, err)
	require.Empty(t, rec.Body.String())
	require.False(t, c.Writer.Written())
}

func TestForwardAsAnthropic_PreContentReadErrorDoesNotFailOver(t *testing.T) {
	reader := &errAfterReader{data: strings.NewReader(preContentCreatedFrame), err: io.ErrUnexpectedEOF}
	rec, _, err := forwardPreContent(t, rawChatCompletionsTestConfig(), nil, io.NopCloser(reader))

	require.ErrorIs(t, err, io.ErrUnexpectedEOF)
	var failoverErr *UpstreamFailoverError
	require.False(t, errors.As(err, &failoverErr), "a read error never fails over: %v", err)
	require.Contains(t, rec.Body.String(), "message_start")
}

func TestForwardAsAnthropic_PreContentPingKeepsFailover(t *testing.T) {
	cfg := rawChatCompletionsTestConfig()
	cfg.Gateway.StreamKeepaliveInterval = 1
	pr, pw := io.Pipe()
	go func() {
		_, _ = io.WriteString(pw, preContentCreatedFrame)
		time.Sleep(1300 * time.Millisecond)
		_, _ = io.WriteString(pw, preContentFailoverFailed)
		_ = pw.Close()
	}()
	rec, c, err := forwardPreContent(t, cfg, http.Header{"X-Request-Id": []string{"attempt-1"}}, pr)

	var failoverErr *UpstreamFailoverError
	require.True(t, errors.As(err, &failoverErr), "ping-only output must keep failover: %T: %v", err, err)
	body := rec.Body.String()
	require.GreaterOrEqual(t, strings.Count(body, "event: ping"), 1)
	require.NotContains(t, body, "message_start")
	require.Equal(t, "text/event-stream", rec.Header().Get("Content-Type"))
	require.Empty(t, rec.Header().Get("X-Request-Id"), "account headers must stay private while the prelude is held")
	require.Equal(t, -1, OpenAICompactKeepaliveAdjustedWrittenSize(c))
}

func TestForwardAsAnthropic_ContentReleasedNeverFailsOver(t *testing.T) {
	stream := preContentCreatedFrame +
		`data: {"type":"response.output_text.delta","output_index":0,"content_index":0,"delta":"partial"}` + "\n\n" +
		preContentFailoverFailed
	rec, _, err := forwardPreContent(t, rawChatCompletionsTestConfig(), nil, io.NopCloser(strings.NewReader(stream)))

	require.Error(t, err)
	var failoverErr *UpstreamFailoverError
	require.False(t, errors.As(err, &failoverErr), "released content must not fail over: %v", err)
	body := rec.Body.String()
	start := strings.Index(body, "event: message_start")
	text := strings.Index(body, `"text":"partial"`)
	errEvt := strings.Index(body, "event: error")
	require.True(t, start >= 0 && text > start && errEvt > text, "body order: %q", body)
}

func TestForwardAsAnthropic_TerminalBeforeContentReleasesPrelude(t *testing.T) {
	stream := preContentCreatedFrame +
		`data: {"type":"response.completed","response":{"id":"resp_pre","object":"response","model":"gpt-5.4","status":"completed","output":[],"usage":{"input_tokens":3,"output_tokens":0,"total_tokens":3}}}` + "\n\n"
	rec, _, err := forwardPreContent(t, rawChatCompletionsTestConfig(), nil, io.NopCloser(strings.NewReader(stream)))

	require.NoError(t, err)
	body := rec.Body.String()
	start := strings.Index(body, "event: message_start")
	delta := strings.Index(body, "event: message_delta")
	stop := strings.Index(body, "event: message_stop")
	require.True(t, start >= 0 && delta > start && stop > delta, "body order: %q", body)
}

func TestForwardAsAnthropic_IntervalTimeoutReleasesPrelude(t *testing.T) {
	cfg := rawChatCompletionsTestConfig()
	cfg.Gateway.StreamDataIntervalTimeout = 1
	pr, pw := io.Pipe()
	defer func() { _ = pw.Close() }()
	go func() {
		_, _ = io.WriteString(pw, preContentCreatedFrame)
	}()
	rec, _, err := forwardPreContent(t, cfg, nil, pr)

	require.Error(t, err)
	require.Contains(t, err.Error(), "stream data interval timeout")
	var failoverErr *UpstreamFailoverError
	require.False(t, errors.As(err, &failoverErr))
	require.Contains(t, rec.Body.String(), "message_start")
}

func TestWriteAnthropicErrorAfterCommittedSSE(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)
	setSSEHeaders(c)
	_, err := io.WriteString(c.Writer, "event: ping\ndata: {\"type\":\"ping\"}\n\n")
	require.NoError(t, err)

	writeAnthropicError(c, http.StatusBadRequest, "invalid_request_error", "prompt is too long")

	body := rec.Body.String()
	require.True(t, strings.HasSuffix(body, "event: error\ndata: {\"error\":{\"message\":\"prompt is too long\",\"type\":\"invalid_request_error\"},\"type\":\"error\"}\n\n"), "body: %q", body)
	for _, line := range strings.Split(body, "\n") {
		require.False(t, strings.HasPrefix(line, "{"), "bare JSON line in SSE body: %q", line)
	}
}

func TestForwardAsAnthropic_OpenCodeUnsupportedModelAfterCommittedSSEIsSSEError(t *testing.T) {
	rec, c, body := newPreContentContext()
	body = []byte(`{"model":"gemini-3-pro","max_tokens":32,"messages":[{"role":"user","content":"hello"}],"stream":true}`)
	setSSEHeaders(c)
	_, err := io.WriteString(c.Writer, "event: ping\ndata: {\"type\":\"ping\"}\n\n")
	require.NoError(t, err)
	account := &Account{
		ID: 1, Platform: PlatformOpenCodeGo, Type: AccountTypeAPIKey,
		Credentials: map[string]any{"api_key": "sk-test", "base_url": "http://upstream.example"},
	}
	svc := &OpenAIGatewayService{cfg: rawChatCompletionsTestConfig()}

	_, err = svc.ForwardAsAnthropic(context.Background(), c, account, body, "", "")

	require.Error(t, err)
	out := rec.Body.String()
	lastEvent := out[strings.LastIndex(strings.TrimRight(out, "\n"), "\n\n")+2:]
	require.True(t, strings.HasPrefix(lastEvent, "event: error\ndata: {"), "last event: %q", lastEvent)
	for _, line := range strings.Split(out, "\n") {
		require.False(t, strings.HasPrefix(line, "{"), "bare JSON line in SSE body: %q", line)
	}
}
