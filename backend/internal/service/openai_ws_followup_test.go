package service

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	coderws "github.com/coder/websocket"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type cancelOnBridgeBodyClose struct {
	io.ReadCloser
	cancel context.CancelFunc
}

func (b *cancelOnBridgeBodyClose) Close() error {
	err := b.ReadCloser.Close()
	b.cancel()
	return err
}

func TestOpenAIWSHTTPBridgeCompletedUsageSurvivesControlCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	cfg := passthroughLifecycleConfig()
	svc := newPassthroughLifecycleService(cfg, newStagedPassthroughConn())
	svc.httpUpstream = &httpUpstreamRecorder{resp: &http.Response{StatusCode: 200, Header: make(http.Header), Body: &cancelOnBridgeBodyClose{
		ReadCloser: io.NopCloser(strings.NewReader("data: {\"type\":\"response.completed\",\"response\":{\"id\":\"resp_complete_cancel\",\"usage\":{\"input_tokens\":13,\"output_tokens\":5}}}\n\n")), cancel: cancel,
	}}}
	account := passthroughLifecycleAccount()
	account.Extra["openai_apikey_responses_websockets_v2_mode"] = OpenAIWSIngressModeHTTPBridge
	results := make(chan *OpenAIForwardResult, 2)
	turnErrors := make(chan error, 2)
	server, done := startPassthroughLifecycleServer(t, ctx, svc, account, &OpenAIWSIngressHooks{AfterTurn: func(_ int, result *OpenAIForwardResult, err error) { results <- result; turnErrors <- err }})
	defer server.Close()
	client := dialPassthroughLifecycleClient(t, server)
	defer func() { _ = client.CloseNow() }()
	_, err := readPassthroughLifecycleFrame(t, client, 3*time.Second)
	require.NoError(t, err)
	_, err = readPassthroughLifecycleFrame(t, client, 3*time.Second)
	require.Equal(t, coderws.StatusGoingAway, coderws.CloseStatus(err))
	select {
	case err := <-done:
		require.ErrorIs(t, err, context.Canceled)
	case <-time.After(3 * time.Second):
		t.Fatal("control close did not finish")
	}
	require.NoError(t, <-turnErrors)
	result := <-results
	require.NotNil(t, result)
	require.Equal(t, "resp_complete_cancel", result.RequestID)
	require.Equal(t, 13, result.Usage.InputTokens)
	require.Equal(t, 5, result.Usage.OutputTokens)
	require.Empty(t, results)
}

func TestOpenAIWSProxyPeerCloseCodesDrainCleanly(t *testing.T) {
	for _, mode := range []string{OpenAIWSIngressModeCtxPool, OpenAIWSIngressModeHTTPBridge, OpenAIWSIngressModePassthrough} {
		for _, code := range []coderws.StatusCode{coderws.StatusNormalClosure, coderws.StatusPolicyViolation, 4001} {
			t.Run(mode+"/"+code.String(), func(t *testing.T) {
				cfg := passthroughLifecycleConfig()
				cfg.Gateway.OpenAIFirstOutputTimeoutSeconds = 60
				cfg.Gateway.OpenAIWS.ReadTimeoutSeconds = 60
				upstream := newStagedPassthroughConn()
				pool := newOpenAIWSConnPool(cfg)
				pool.setClientDialerForTest(&stagedPassthroughDialer{conn: &ingressDrainTestConn{upstream}})
				defer pool.Close()
				svc := newPassthroughLifecycleService(cfg, upstream)
				svc.openaiWSPool = pool
				body, writer := io.Pipe()
				defer func() { _ = writer.Close() }()
				svc.httpUpstream = &contextAwareBridgeUpstream{httpUpstreamRecorder: httpUpstreamRecorder{resp: &http.Response{StatusCode: 200, Header: make(http.Header), Body: body}}, writer: writer}
				account := passthroughLifecycleAccount()
				account.Extra["openai_apikey_responses_websockets_v2_mode"] = mode
				results := make(chan *OpenAIForwardResult, 2)
				turnErrors := make(chan error, 2)
				server, done := startPassthroughLifecycleServer(t, context.Background(), svc, account, &OpenAIWSIngressHooks{AfterTurn: func(_ int, result *OpenAIForwardResult, err error) { results <- result; turnErrors <- err }})
				defer server.Close()
				client := dialPassthroughLifecycleClient(t, server)
				defer func() { _ = client.CloseNow() }()
				if mode != OpenAIWSIngressModeHTTPBridge {
					requirePassthroughUpstreamWrite(t, upstream, time.Second)
				} else {
					_, err := io.WriteString(writer, ": reading\n\n")
					require.NoError(t, err)
				}
				require.NoError(t, client.Close(code, "client ended session"))
				select {
				case err := <-done:
					require.NoError(t, err)
				case <-time.After(4 * time.Second):
					t.Fatal("peer close drain did not finish")
				}
				require.NoError(t, <-turnErrors)
				require.Nil(t, <-results)
				require.Empty(t, results)
				require.Empty(t, turnErrors)
			})
		}
	}
}

type gatedBridgeFailoverUpstream struct {
	httpUpstreamRecorder
	started chan struct{}
	proceed <-chan struct{}
}

func (u *gatedBridgeFailoverUpstream) Do(req *http.Request, proxy string, account int64, concurrency int) (*http.Response, error) {
	close(u.started)
	select {
	case <-u.proceed:
	case <-req.Context().Done():
		return nil, req.Context().Err()
	}
	return u.httpUpstreamRecorder.Do(req, proxy, account, concurrency)
}

func TestOpenAIWSFailoverGateRejectsDepartedClient(t *testing.T) {
	for _, mode := range []string{OpenAIWSIngressModeCtxPool, OpenAIWSIngressModeHTTPBridge, OpenAIWSIngressModePassthrough} {
		for _, departed := range []bool{true, false} {
			name := "live"
			if departed {
				name = "departed"
			}
			t.Run(mode+"/"+name, func(t *testing.T) {
				cfg := passthroughLifecycleConfig()
				cfg.Gateway.OpenAIFirstOutputTimeoutSeconds = 60
				cfg.Gateway.OpenAIWS.ReadTimeoutSeconds = 60
				upstream := newStagedPassthroughConn()
				pool := newOpenAIWSConnPool(cfg)
				pool.setClientDialerForTest(&stagedPassthroughDialer{conn: &ingressDrainTestConn{upstream}})
				defer pool.Close()
				svc := newPassthroughLifecycleService(cfg, upstream)
				svc.openaiWSPool = pool
				proceed := make(chan struct{})
				httpStarted := make(chan struct{})
				svc.httpUpstream = &gatedBridgeFailoverUpstream{httpUpstreamRecorder: httpUpstreamRecorder{resp: &http.Response{StatusCode: 429, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{"error":{"type":"rate_limit_error"}}`))}}, started: httpStarted, proceed: proceed}
				account := passthroughLifecycleAccount()
				account.Extra["openai_apikey_responses_websockets_v2_mode"] = mode
				readers := make(chan *openAIWSIngressReader, 1)
				done := make(chan error, 1)
				attempts := atomic.Int32{}
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
					conn, err := coderws.Accept(w, req, nil)
					if err != nil {
						done <- err
						return
					}
					defer func() { _ = conn.CloseNow() }()
					c, _ := gin.CreateTestContext(httptest.NewRecorder())
					c.Request = req
					readers <- openAIWSGetIngressReader(c, conn)
					first := []byte(`{"type":"response.create","model":"gpt-5.1"}`)
					attempts.Add(1)
					err = svc.ProxyResponsesWebSocketFromClient(req.Context(), c, conn, account, "sk-test", first, nil)
					var failover *UpstreamFailoverError
					// Same shared gate as handleWSFailover: preserve the actual
					// error, but never select/forward to another account for a dead reader.
					if errors.As(err, &failover) && OpenAIWSIngressCanFailover(req.Context(), c) {
						attempts.Add(1)
						next := *account
						next.ID++
						next.Extra = map[string]any{"openai_apikey_responses_websockets_v2_mode": OpenAIWSIngressModeHTTPBridge}
						nextSvc := newPassthroughLifecycleService(cfg, newStagedPassthroughConn())
						nextSvc.httpUpstream = &httpUpstreamRecorder{resp: &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader("data: {\"type\":\"response.completed\",\"response\":{\"id\":\"resp_second_account\"}}\n\n"))}}
						_ = nextSvc.ProxyResponsesWebSocketFromClient(req.Context(), c, conn, &next, "sk-test", first, nil)
					}
					done <- err
				}))
				defer server.Close()
				client, _, err := coderws.Dial(context.Background(), "ws"+strings.TrimPrefix(server.URL, "http"), nil)
				require.NoError(t, err)
				defer func() { _ = client.CloseNow() }()
				r := <-readers
				if mode == OpenAIWSIngressModeHTTPBridge {
					<-httpStarted
				} else {
					requirePassthroughUpstreamWrite(t, upstream, time.Second)
				}
				if departed {
					require.NoError(t, client.Close(coderws.StatusNormalClosure, "done"))
					<-r.done
				}
				if mode == OpenAIWSIngressModeHTTPBridge {
					close(proceed)
				} else {
					upstream.Send(`{"type":"error","error":{"code":"rate_limit_exceeded","type":"usage_limit_reached","message":"rate limited"}}`)
				}
				if !departed {
					payload, err := readPassthroughLifecycleFrame(t, client, 3*time.Second)
					require.NoError(t, err)
					require.Contains(t, string(payload), "resp_second_account")
					require.NoError(t, client.Close(coderws.StatusNormalClosure, "done"))
				}
				select {
				case err := <-done:
					var failover *UpstreamFailoverError
					require.ErrorAs(t, err, &failover, "keep substantive upstream failure evidence")
					require.Equal(t, 429, failover.StatusCode)
				case <-time.After(4 * time.Second):
					t.Fatal("account attempts did not stop")
				}
				want := int32(2)
				if departed {
					want = 1
				}
				require.Equal(t, want, attempts.Load())
			})
		}
	}
}

func TestOpenAIWSFailoverGateRejectsControlAndPolicyClose(t *testing.T) {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	require.True(t, OpenAIWSIngressCanFailover(context.Background(), c))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	require.False(t, OpenAIWSIngressCanFailover(ctx, c))
	done := make(chan struct{})
	close(done)
	c.Set(openAIWSIngressReaderKey, &openAIWSIngressReader{done: done, err: NewOpenAIWSClientCloseError(coderws.StatusPolicyViolation, "too many pending websocket requests", nil)})
	require.False(t, OpenAIWSIngressCanFailover(context.Background(), c))
}

func TestOpenAIWSFailoverGateRejectsInvalidReaderState(t *testing.T) {
	for _, value := range []any{"invalid", (*openAIWSIngressReader)(nil)} {
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		c.Set(openAIWSIngressReaderKey, value)
		require.False(t, OpenAIWSIngressCanFailover(context.Background(), c))
		reader := openAIWSGetIngressReader(c, nil)
		require.Error(t, reader.err)
		select {
		case <-reader.done:
		default:
			t.Fatal("invalid cached state must not start another reader")
		}
		require.Same(t, reader, openAIWSGetIngressReader(c, nil))
	}
}
