package service

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/ctxkey"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

type antigravityCompactWaitUpstream struct {
	HTTPUpstream
	wrote    <-chan struct{}
	response *http.Response
}

func (u *antigravityCompactWaitUpstream) Do(req *http.Request, _ string, _ int64, _ int) (*http.Response, error) {
	select {
	case <-u.wrote:
		return u.response, nil
	case <-req.Context().Done():
		return nil, req.Context().Err()
	}
}

type antigravityCompactFuncUpstream struct {
	HTTPUpstream
	do func(*http.Request) (*http.Response, error)
}

func (u *antigravityCompactFuncUpstream) Do(req *http.Request, _ string, _ int64, _ int) (*http.Response, error) {
	return u.do(req)
}

type antigravityCompactGatedBody struct {
	ctx       context.Context
	wrote     <-chan struct{}
	initial   string
	remaining io.Reader
	waited    bool
	closed    chan struct{}
	closeOnce sync.Once
}

func (b *antigravityCompactGatedBody) Read(p []byte) (int, error) {
	if b.initial != "" {
		n := copy(p, b.initial)
		b.initial = b.initial[n:]
		return n, nil
	}
	if !b.waited {
		for i := 0; i < 2; i++ {
			select {
			case <-b.wrote:
			case <-b.ctx.Done():
				return 0, b.ctx.Err()
			case <-b.closed:
				return 0, io.ErrClosedPipe
			}
		}
		b.waited = true
	}
	return b.remaining.Read(p)
}

func (b *antigravityCompactGatedBody) Close() error {
	b.closeOnce.Do(func() {
		close(b.closed)
		if closer, ok := b.remaining.(io.Closer); ok {
			_ = closer.Close()
		}
	})
	return nil
}

func TestAntigravityCompactCollectionFailureAfterKeepalive(t *testing.T) {
	for _, scenario := range []string{"read failure", "upstream deadline", "empty stream", "empty summary", "incomplete summary", "truncated summary"} {
		t.Run(scenario, func(t *testing.T) {
			body := []byte(`{"model":"gemini-3.1-pro-high","stream":true,"input":[{"type":"message","role":"user","content":"Summarize"},{"type":"compaction_trigger"}]}`)
			c, recorder := newAntigravityCompatContext(http.MethodPost, "/v1/responses", body)
			ctx, cancel := context.WithTimeout(context.WithValue(c.Request.Context(), ctxkey.UserID, int64(42)), 3500*time.Millisecond)
			defer cancel()
			c.Request = c.Request.WithContext(ctx)
			wrote := make(chan struct{}, 10)
			c.Writer = &antigravityCompatNotifyingWriter{ResponseWriter: c.Writer, wrote: wrote}
			var remaining io.Reader = strings.NewReader("")
			switch scenario {
			case "read failure":
				remaining = &antigravityCompatErrorReader{err: io.ErrUnexpectedEOF}
			case "upstream deadline":
				remaining = &antigravityCompatErrorReader{err: context.DeadlineExceeded}
			case "empty summary":
				remaining = strings.NewReader(`data: {"response":{"candidates":[{"content":{"parts":[]},"finishReason":"STOP"}],"usageMetadata":{"promptTokenCount":8,"candidatesTokenCount":3}}}` + "\n\n")
			case "incomplete summary":
				remaining = strings.NewReader(`data: {"response":{"candidates":[{"content":{"parts":[{"text":"Partial summary"}]}}]}}` + "\n\n")
			case "truncated summary":
				remaining = strings.NewReader(`data: {"response":{"candidates":[{"content":{"parts":[{"text":"Truncated summary"}]},"finishReason":"MAX_TOKENS"}],"usageMetadata":{"promptTokenCount":8,"candidatesTokenCount":3}}}` + "\n\n")
			}
			var upstreamBody *antigravityCompactGatedBody
			upstream := &antigravityCompactFuncUpstream{do: func(req *http.Request) (*http.Response, error) {
				upstreamBody = &antigravityCompactGatedBody{ctx: req.Context(), wrote: wrote, remaining: remaining, closed: make(chan struct{})}
				return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": []string{"text/event-stream"}}, Body: upstreamBody}, nil
			}}
			svc := newAntigravityCompatService(config.GatewayConfig{StreamKeepaliveInterval: 1}, upstream)
			result, err := svc.ForwardAsResponses(ctx, c, newAntigravityCompatAccount(AccountTypeOAuth), body, nil)
			require.Error(t, err)
			require.Nil(t, result)
			var failover *UpstreamFailoverError
			require.False(t, errors.As(err, &failover), "a committed response must terminate in-stream")
			require.Equal(t, http.StatusOK, recorder.Code)
			require.Contains(t, recorder.Body.String(), "event: error\n")
			require.NotContains(t, recorder.Body.String(), "response.completed")
			require.NotContains(t, recorder.Body.String(), "encrypted_content")
			select {
			case <-upstreamBody.closed:
			default:
				t.Fatal("failed upstream body was not closed")
			}
		})
	}
}

type antigravityCompactFinalWriteFailure struct {
	gin.ResponseWriter
	failAll bool
}

func (w *antigravityCompactFinalWriteFailure) Write(p []byte) (int, error) {
	if w.failAll || strings.Contains(string(p), "event: response.completed\n") {
		return 0, io.ErrClosedPipe
	}
	return w.ResponseWriter.Write(p)
}

func TestAntigravityCompactStopsOnClientDisconnect(t *testing.T) {
	for _, scenario := range []string{"cancel", "heartbeat write failure"} {
		t.Run(scenario, func(t *testing.T) {
			body := []byte(`{"model":"gemini-3.1-pro-high","stream":true,"input":[{"type":"message","role":"user","content":"Summarize"},{"type":"compaction_trigger"}]}`)
			c, recorder := newAntigravityCompatContext(http.MethodPost, "/v1/responses", body)
			ctx, cancel := context.WithTimeout(context.WithValue(c.Request.Context(), ctxkey.UserID, int64(42)), 3500*time.Millisecond)
			defer cancel()
			c.Request = c.Request.WithContext(ctx)
			wrote := make(chan struct{}, 10)
			if scenario == "cancel" {
				c.Writer = &antigravityCompatNotifyingWriter{ResponseWriter: c.Writer, wrote: wrote}
				go func() {
					select {
					case <-wrote:
						cancel()
					case <-ctx.Done():
					}
				}()
			} else {
				c.Writer = &antigravityCompactFinalWriteFailure{ResponseWriter: c.Writer, failAll: true}
			}
			var upstreamBody *antigravityCompactGatedBody
			upstream := &antigravityCompactFuncUpstream{do: func(req *http.Request) (*http.Response, error) {
				upstreamBody = &antigravityCompactGatedBody{
					ctx: req.Context(), wrote: make(chan struct{}), closed: make(chan struct{}), remaining: strings.NewReader(""),
					initial: `data: {"response":{"candidates":[{"content":{"parts":[{"text":"Partial summary"}]}}]}}` + "\n\n",
				}
				return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": []string{"text/event-stream"}}, Body: upstreamBody}, nil
			}}
			svc := newAntigravityCompatService(config.GatewayConfig{StreamKeepaliveInterval: 1}, upstream)
			result, err := svc.ForwardAsResponses(ctx, c, newAntigravityCompatAccount(AccountTypeOAuth), body, nil)
			require.Error(t, err)
			require.Nil(t, result)
			if scenario == "cancel" {
				require.ErrorIs(t, err, context.Canceled)
			} else {
				require.NoError(t, ctx.Err(), "failed heartbeat must stop promptly without waiting for request timeout")
			}
			require.NotContains(t, recorder.Body.String(), "response.completed")
			select {
			case <-upstreamBody.closed:
			default:
				t.Fatal("disconnected compact did not close upstream body")
			}
		})
	}
}

func TestAntigravityCompactReportsFinalWriteFailure(t *testing.T) {
	body := []byte(`{"model":"gemini-3.1-pro-high","stream":true,"input":[{"type":"message","role":"user","content":"Summarize"},{"type":"compaction_trigger"}]}`)
	c, _ := newAntigravityCompatContext(http.MethodPost, "/v1/responses", body)
	c.Writer = &antigravityCompactFinalWriteFailure{ResponseWriter: c.Writer}
	ctx := context.WithValue(c.Request.Context(), ctxkey.UserID, int64(42))
	svc := newAntigravityCompatService(config.GatewayConfig{}, &queuedHTTPUpstreamStub{responses: []*http.Response{antigravityCompatSuccessResponse()}})
	result, err := svc.ForwardAsResponses(ctx, c, newAntigravityCompatAccount(AccountTypeOAuth), body, nil)
	require.Error(t, err)
	require.Nil(t, result)
}

func TestAntigravityCompactEmptyStreamCanFailOverBeforeKeepalive(t *testing.T) {
	body := []byte(`{"model":"gemini-3.1-pro-high","stream":true,"input":[{"type":"message","role":"user","content":"Summarize"},{"type":"compaction_trigger"}]}`)
	c, recorder := newAntigravityCompatContext(http.MethodPost, "/v1/responses", body)
	ctx := context.WithValue(c.Request.Context(), ctxkey.UserID, int64(42))
	upstream := &queuedHTTPUpstreamStub{responses: []*http.Response{{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(""))}}}
	svc := newAntigravityCompatService(config.GatewayConfig{}, upstream)
	result, err := svc.ForwardAsResponses(ctx, c, newAntigravityCompatAccount(AccountTypeOAuth), body, nil)
	require.Error(t, err)
	require.Nil(t, result)
	var failover *UpstreamFailoverError
	require.ErrorAs(t, err, &failover)
	require.Empty(t, recorder.Body.String())
}

func TestAntigravityCompactHeaderErrorAfterKeepalive(t *testing.T) {
	body := []byte(`{"model":"gemini-3.1-pro-high","stream":true,"input":[{"type":"message","role":"user","content":"Summarize"},{"type":"compaction_trigger"}]}`)
	c, recorder := newAntigravityCompatContext(http.MethodPost, "/v1/responses", body)
	ctx, cancel := context.WithTimeout(context.WithValue(c.Request.Context(), ctxkey.UserID, int64(42)), 2500*time.Millisecond)
	defer cancel()
	c.Request = c.Request.WithContext(ctx)
	wrote := make(chan struct{}, 10)
	c.Writer = &antigravityCompatNotifyingWriter{ResponseWriter: c.Writer, wrote: wrote}
	upstream := &antigravityCompactWaitUpstream{wrote: wrote, response: &http.Response{
		StatusCode: http.StatusBadRequest, Header: make(http.Header),
		Body: io.NopCloser(strings.NewReader(`{"error":{"message":"invalid compact request"}}`)),
	}}
	svc := newAntigravityCompatService(config.GatewayConfig{StreamKeepaliveInterval: 1}, upstream)
	result, err := svc.ForwardAsResponses(ctx, c, newAntigravityCompatAccount(AccountTypeOAuth), body, nil)
	require.Error(t, err)
	require.Nil(t, result)
	require.Equal(t, http.StatusOK, recorder.Code)
	require.Contains(t, recorder.Body.String(), ": ping\n\n")
	require.Contains(t, recorder.Body.String(), "event: error\n")
	require.NotContains(t, recorder.Body.String(), "response.completed")
}

func TestAntigravityCompactPreservesTerminalUsage(t *testing.T) {
	for _, path := range []string{"/v1/responses", "/v1/responses/compact"} {
		t.Run(path, func(t *testing.T) {
			body := []byte(`{"model":"gemini-3.1-pro-high","stream":true,"input":[{"type":"message","role":"user","content":"Summarize"},{"type":"compaction_trigger"}]}`)
			c, recorder := newAntigravityCompatContext(http.MethodPost, path, body)
			ctx := context.WithValue(c.Request.Context(), ctxkey.UserID, int64(42))
			sse := `data: {"response":{"candidates":[{"content":{"parts":[{"text":"Complete summary"}]}}]}}` + "\n\n" +
				`data: {"response":{"candidates":[{"finishReason":"STOP"}],"usageMetadata":{"promptTokenCount":12,"candidatesTokenCount":5,"cachedContentTokenCount":4}}}` + "\n\n"
			upstream := &queuedHTTPUpstreamStub{responses: []*http.Response{{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": []string{"text/event-stream"}}, Body: io.NopCloser(strings.NewReader(sse))}}}
			svc := newAntigravityCompatService(config.GatewayConfig{}, upstream)
			result, err := svc.ForwardAsResponses(ctx, c, newAntigravityCompatAccount(AccountTypeOAuth), body, nil)
			require.NoError(t, err)
			require.NotNil(t, result)
			require.Equal(t, 8, result.Usage.InputTokens)
			require.Equal(t, 4, result.Usage.CacheReadInputTokens)
			require.Equal(t, 5, result.Usage.OutputTokens)
			if path == "/v1/responses/compact" {
				require.Equal(t, "application/json; charset=utf-8", recorder.Header().Get("Content-Type"))
				require.True(t, gjson.Valid(recorder.Body.String()))
				require.Equal(t, "compaction", gjson.Get(recorder.Body.String(), "output.0.type").String())
			} else {
				require.NotContains(t, recorder.Body.String(), ": ping", "fast compact must not need a heartbeat")
			}
		})
	}
}

func TestAntigravityCompactKeepsAliveWhileCollectingSummary(t *testing.T) {
	body := []byte(`{"model":"gemini-3.1-pro-high","stream":true,"input":[{"type":"message","role":"user","content":"Summarize the work"},{"type":"compaction_trigger"}]}`)
	c, recorder := newAntigravityCompatContext(http.MethodPost, "/v1/responses", body)
	ctx, cancel := context.WithTimeout(context.WithValue(c.Request.Context(), ctxkey.UserID, int64(42)), 3500*time.Millisecond)
	defer cancel()
	c.Request = c.Request.WithContext(ctx)
	wrote := make(chan struct{}, 10)
	c.Writer = &antigravityCompatNotifyingWriter{ResponseWriter: c.Writer, wrote: wrote}
	var upstreamBody *antigravityCompactGatedBody
	upstream := &antigravityCompactFuncUpstream{do: func(req *http.Request) (*http.Response, error) {
		resp := antigravityCompatSuccessResponse()
		upstreamBody = &antigravityCompactGatedBody{
			ctx: req.Context(), wrote: wrote, closed: make(chan struct{}),
			initial:   `data: {"response":{"candidates":[{"content":{"parts":[{"text":"Progress: "}]}}]}}` + "\n\n",
			remaining: resp.Body,
		}
		resp.Body = upstreamBody
		return resp, nil
	}}
	svc := newAntigravityCompatService(config.GatewayConfig{StreamKeepaliveInterval: 1}, upstream)
	result, err := svc.ForwardAsResponses(ctx, c, newAntigravityCompatAccount(AccountTypeOAuth), body, nil)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Equal(t, 8, result.Usage.InputTokens)
	require.Equal(t, 3, result.Usage.OutputTokens)
	require.GreaterOrEqual(t, strings.Count(recorder.Body.String(), ": ping\n\n"), 2)
	require.Equal(t, 1, strings.Count(recorder.Body.String(), "event: response.completed\n"))
	require.NotContains(t, recorder.Body.String(), "Progress: ", "partial summary must not leak as response text")
	for _, line := range strings.Split(recorder.Body.String(), "\n") {
		payload := strings.TrimPrefix(line, "data: ")
		if gjson.Get(payload, "type").String() == "response.completed" {
			checkpoint, err := openGatewayCompactCheckpoint(ctx, svc.settingService.cfg, gjson.Get(payload, "response.output.0.encrypted_content").String())
			require.NoError(t, err)
			require.Contains(t, checkpoint, "<compacted-summary>Progress: ok</compacted-summary>")
		}
	}
	select {
	case <-upstreamBody.closed:
	default:
		t.Fatal("upstream response body was not closed")
	}
}

func TestAntigravityCompactKeepsAliveBeforeHeaders(t *testing.T) {
	body := []byte(`{"model":"gemini-3.1-pro-high","stream":true,"input":[{"type":"message","role":"user","content":"Summarize the work"},{"type":"compaction_trigger"}]}`)
	c, recorder := newAntigravityCompatContext(http.MethodPost, "/v1/responses", body)
	ctx, cancel := context.WithTimeout(context.WithValue(c.Request.Context(), ctxkey.UserID, int64(42)), 2500*time.Millisecond)
	defer cancel()
	c.Request = c.Request.WithContext(ctx)
	wrote := make(chan struct{}, 10)
	c.Writer = &antigravityCompatNotifyingWriter{ResponseWriter: c.Writer, wrote: wrote}
	upstream := &antigravityCompactWaitUpstream{wrote: wrote, response: antigravityCompatSuccessResponse()}
	svc := newAntigravityCompatService(config.GatewayConfig{StreamKeepaliveInterval: 1}, upstream)
	result, err := svc.ForwardAsResponses(ctx, c, newAntigravityCompatAccount(AccountTypeOAuth), body, nil)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.True(t, result.NativeCompactionV2)
	require.Equal(t, 8, result.Usage.InputTokens)
	require.Equal(t, 3, result.Usage.OutputTokens)
	require.Equal(t, "text/event-stream", recorder.Header().Get("Content-Type"))
	require.Equal(t, "no", recorder.Header().Get("X-Accel-Buffering"))
	require.True(t, recorder.Flushed)
	require.True(t, strings.HasPrefix(recorder.Body.String(), ": ping\n\n"))
	var completed int
	for _, line := range strings.Split(recorder.Body.String(), "\n") {
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		payload := strings.TrimPrefix(line, "data: ")
		if gjson.Get(payload, "type").String() == "response.completed" {
			completed++
			outputs := gjson.Get(payload, "response.output").Array()
			require.Len(t, outputs, 1)
			require.Equal(t, "compaction", outputs[0].Get("type").String())
			checkpoint, err := openGatewayCompactCheckpoint(ctx, svc.settingService.cfg, outputs[0].Get("encrypted_content").String())
			require.NoError(t, err)
			require.Contains(t, checkpoint, "<compacted-summary>ok</compacted-summary>")
		}
	}
	require.Equal(t, 1, completed)
}
