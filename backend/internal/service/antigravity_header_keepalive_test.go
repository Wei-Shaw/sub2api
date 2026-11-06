package service

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

// Hold response headers until a downstream write arrives. This reproduces the
// proxy's idle interval before the normal stream reader has even started.
type antigravityHeaderWaitUpstream struct {
	HTTPUpstream
	wrote    <-chan struct{}
	response *http.Response
}

func (u *antigravityHeaderWaitUpstream) Do(req *http.Request, _ string, _ int64, _ int) (*http.Response, error) {
	select {
	case <-u.wrote:
		return u.response, nil
	case <-req.Context().Done():
		return nil, req.Context().Err()
	}
}

func TestAntigravityCompatKeepsAliveBeforeResponseHeaders(t *testing.T) {
	for _, protocol := range []string{"responses", "chat/completions"} {
		t.Run(protocol, func(t *testing.T) {
			body := []byte(`{"model":"gemini-3.1-pro-high","stream":true,"input":"hello","messages":[{"role":"user","content":"hello"}]}`)
			c, recorder := newAntigravityCompatContext(http.MethodPost, "/v1/"+protocol, body)
			ctx, cancel := context.WithTimeout(c.Request.Context(), 1500*time.Millisecond)
			defer cancel()
			c.Request = c.Request.WithContext(ctx)
			wrote := make(chan struct{}, 10)
			c.Writer = &antigravityCompatNotifyingWriter{ResponseWriter: c.Writer, wrote: wrote}
			u := &antigravityHeaderWaitUpstream{wrote: wrote, response: antigravityCompatSuccessResponse()}
			svc := newAntigravityCompatService(config.GatewayConfig{StreamKeepaliveInterval: 1}, u)
			forward := svc.ForwardAsResponses
			if protocol == "chat/completions" {
				forward = svc.ForwardAsChatCompletions
			}
			result, err := forward(ctx, c, newAntigravityCompatAccount(AccountTypeOAuth), body, nil)
			require.NoError(t, err)
			require.NotNil(t, result)
			require.Equal(t, 8, result.Usage.InputTokens)
			require.Equal(t, 3, result.Usage.OutputTokens)
			require.Equal(t, "text/event-stream", recorder.Header().Get("Content-Type"))
			require.True(t, recorder.Flushed)
			require.True(t, strings.HasPrefix(recorder.Body.String(), ": ping\n\n"), recorder.Body.String())
			require.Contains(t, recorder.Body.String(), "ok")
		})
	}
}

func TestAntigravityCompatHTTPErrorAfterHeaderKeepalive(t *testing.T) {
	body := []byte(`{"model":"gemini-3.1-pro-high","stream":true,"input":"hello"}`)
	c, recorder := newAntigravityCompatContext(http.MethodPost, "/v1/responses", body)
	ctx, cancel := context.WithTimeout(c.Request.Context(), 1500*time.Millisecond)
	defer cancel()
	c.Request = c.Request.WithContext(ctx)
	wrote := make(chan struct{}, 10)
	c.Writer = &antigravityCompatNotifyingWriter{ResponseWriter: c.Writer, wrote: wrote}
	u := &antigravityHeaderWaitUpstream{wrote: wrote, response: &http.Response{
		StatusCode: http.StatusBadRequest, Header: make(http.Header),
		Body: io.NopCloser(strings.NewReader(`{"error":{"message":"bad request"}}`)),
	}}
	svc := newAntigravityCompatService(config.GatewayConfig{StreamKeepaliveInterval: 1}, u)
	_, err := svc.ForwardAsResponses(ctx, c, newAntigravityCompatAccount(AccountTypeOAuth), body, nil)
	require.Error(t, err)
	require.Equal(t, http.StatusOK, recorder.Code)
	require.Contains(t, recorder.Body.String(), ": ping\n\n")
	require.Contains(t, recorder.Body.String(), "event: error\n")
	require.Equal(t, "text/event-stream", recorder.Header().Get("Content-Type"))
}

type antigravityHeaderFuncUpstream struct {
	HTTPUpstream
	do func(*http.Request) (*http.Response, error)
}

func (u *antigravityHeaderFuncUpstream) Do(req *http.Request, _ string, _ int64, _ int) (*http.Response, error) {
	return u.do(req)
}

type antigravityTrackedHeaderBody struct {
	io.Reader
	closed atomic.Bool
}

func (b *antigravityTrackedHeaderBody) Close() error { b.closed.Store(true); return nil }

func TestAntigravityHeaderKeepaliveCancelClosesLateResponse(t *testing.T) {
	c, _ := newAntigravityCompatContext(http.MethodPost, "/v1/responses", nil)
	ctx, cancel := context.WithCancel(c.Request.Context())
	c.Request = c.Request.WithContext(ctx)
	entered := make(chan struct{})
	release := make(chan struct{})
	defer close(release)
	body := &antigravityTrackedHeaderBody{Reader: strings.NewReader("ok")}
	u := &antigravityHeaderKeepaliveUpstream{
		c: c, interval: time.Second,
		HTTPUpstream: &antigravityHeaderFuncUpstream{do: func(req *http.Request) (*http.Response, error) {
			close(entered)
			<-release // A transport may return a response even after cancellation.
			return &http.Response{StatusCode: 200, Body: body}, nil
		}},
	}
	go func() { <-entered; cancel() }()
	_, err := u.Do(c.Request, "", 1, 1)
	require.ErrorIs(t, err, context.Canceled)
	// Release the late response without closing the channel twice.
	release <- struct{}{}
	require.Eventually(t, body.closed.Load, time.Second, time.Millisecond)
}

func TestAntigravityHeaderKeepalivePreservesBodyContextUntilClose(t *testing.T) {
	c, recorder := newAntigravityCompatContext(http.MethodPost, "/v1/responses", nil)
	var bodyContext context.Context
	u := &antigravityHeaderKeepaliveUpstream{
		c: c, interval: time.Second,
		HTTPUpstream: &antigravityHeaderFuncUpstream{do: func(req *http.Request) (*http.Response, error) {
			bodyContext = req.Context()
			return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader("ok"))}, nil
		}},
	}
	resp, err := u.Do(c.Request, "", 1, 1)
	require.NoError(t, err)
	require.NoError(t, bodyContext.Err())
	data, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.Equal(t, "ok", string(data))
	require.NoError(t, resp.Body.Close())
	require.ErrorIs(t, bodyContext.Err(), context.Canceled)
	require.Empty(t, recorder.Body.String(), "fast upstream must not commit the downstream")
}

func TestAntigravityCompatFailoverAfterHeaderKeepaliveWritesError(t *testing.T) {
	for _, scenario := range []string{"unauthorized", "account switch"} {
		t.Run(scenario, func(t *testing.T) {
			c, recorder := newAntigravityCompatContext(http.MethodPost, "/v1/responses", nil)
			c.Header("Content-Type", "text/event-stream")
			MarkResponseCommitted(c)
			_, err := c.Writer.Write([]byte(": ping\n\n"))
			require.NoError(t, err)
			c.Writer.Flush()
			svc := newAntigravityCompatService(config.GatewayConfig{}, nil)
			if scenario == "unauthorized" {
				err = svc.handleAntigravityCompatHTTPError(c.Request.Context(), c,
					newAntigravityCompatAccount(AccountTypeOAuth),
					&antigravityCompatUpstreamCall{request: antigravityCompatRequest{originalModel: "gemini-3.1-pro-high"}},
					&http.Response{StatusCode: http.StatusUnauthorized, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{"error":{"message":"Invalid bearer token"}}`))})
			} else {
				err = svc.handleAntigravityCompatTransportError(c, &AntigravityAccountSwitchError{OriginalAccountID: 1})
			}
			require.Error(t, err)
			var failover *UpstreamFailoverError
			require.False(t, errors.As(err, &failover), "committed response must not request account replay")
			require.Contains(t, recorder.Body.String(), "event: error\n")
			require.Contains(t, recorder.Body.String(), `"error":`)
		})
	}
}
