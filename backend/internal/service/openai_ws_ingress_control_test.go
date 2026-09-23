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

	coderws "github.com/coder/websocket"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestOpenAIWSIngressInterTurnControlClose(t *testing.T) {
	for _, mode := range []string{OpenAIWSIngressModeCtxPool, OpenAIWSIngressModeHTTPBridge} {
		for _, cause := range []error{ErrOpenAIWSIngressLeaseLost, context.Canceled, context.DeadlineExceeded, nil} {
			name := "client_close"
			if cause != nil {
				name = cause.Error()
			}
			t.Run(mode+"/"+name, func(t *testing.T) {
				cfg := passthroughLifecycleConfig()
				cfg.Gateway.OpenAIWS.IngressInterTurnIdleTimeoutSeconds = 60
				upstream := newStagedPassthroughConn()
				pool := newOpenAIWSConnPool(cfg)
				pool.setClientDialerForTest(&stagedPassthroughDialer{conn: &ingressDrainTestConn{upstream}})
				defer pool.Close()
				svc := newPassthroughLifecycleService(cfg, upstream)
				svc.openaiWSPool = pool
				terminal := `{"type":"response.completed","response":{"id":"resp_control","usage":{"input_tokens":3,"output_tokens":1}}}`
				svc.httpUpstream = &httpUpstreamRecorder{resp: &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader("data: " + terminal + "\n\n"))}}
				account := passthroughLifecycleAccount()
				account.Extra["openai_apikey_responses_websockets_v2_mode"] = mode
				ctx, cancel := context.WithCancelCause(context.Background())
				defer cancel(context.Canceled)
				if cause == context.DeadlineExceeded {
					var stop context.CancelFunc
					ctx, stop = context.WithTimeout(ctx, time.Second)
					defer stop()
				}
				after := make(chan error, 2)
				server, done := startPassthroughLifecycleServer(t, ctx, svc, account, &OpenAIWSIngressHooks{AfterTurn: func(_ int, _ *OpenAIForwardResult, err error) { after <- err }})
				defer server.Close()
				client := dialPassthroughLifecycleClient(t, server)
				defer func() { _ = client.CloseNow() }()
				if mode == OpenAIWSIngressModeCtxPool {
					requirePassthroughUpstreamWrite(t, upstream, time.Second)
					upstream.Send(terminal)
				}
				_, err := readPassthroughLifecycleFrame(t, client, time.Second)
				require.NoError(t, err)
				require.NoError(t, <-after)
				if cause == nil {
					require.NoError(t, client.Close(coderws.StatusNormalClosure, "done"))
				} else {
					if cause != context.DeadlineExceeded {
						cancel(cause)
					}
					_, readErr := readPassthroughLifecycleFrame(t, client, 3*time.Second)
					status, reason := coderws.StatusGoingAway, "websocket request canceled"
					if cause == ErrOpenAIWSIngressLeaseLost {
						status, reason = coderws.StatusTryAgainLater, "websocket ingress capacity lease lost; please reconnect"
					}
					var wireClose coderws.CloseError
					require.ErrorAs(t, readErr, &wireClose)
					require.Equal(t, status, wireClose.Code)
					require.Equal(t, reason, wireClose.Reason)
				}
				select {
				case err := <-done:
					if cause == nil {
						require.NoError(t, err)
					} else {
						var typed *OpenAIWSClientCloseError
						require.ErrorAs(t, err, &typed)
						require.ErrorIs(t, err, cause)
						require.False(t, isOpenAIWSClientDisconnectError(err))
					}
				case <-time.After(3 * time.Second):
					t.Fatal("inter-turn reader did not join")
				}
				require.Empty(t, after, "inter-turn close must not complete another turn")
			})
		}
	}
}

func TestOpenAIWSIngressReaderOverflowSendsPolicyCloseAndJoins(t *testing.T) {
	readers := make(chan *openAIWSIngressReader, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		conn, err := coderws.Accept(w, req, nil)
		if err != nil {
			return
		}
		defer func() { _ = conn.CloseNow() }()
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		r := openAIWSGetIngressReader(c, conn)
		readers <- r
		<-r.done
	}))
	defer server.Close()
	client, _, err := coderws.Dial(context.Background(), "ws"+strings.TrimPrefix(server.URL, "http"), nil)
	require.NoError(t, err)
	defer func() { _ = client.CloseNow() }()
	r := <-readers
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
	defer cancel()
	for i := 0; i < 9; i++ {
		require.NoError(t, client.Write(ctx, coderws.MessageText, []byte(`{"type":"conversation.item.create"}`)))
	}
	_, _, err = client.Read(ctx)
	var closeErr coderws.CloseError
	require.ErrorAs(t, err, &closeErr)
	require.Equal(t, coderws.StatusPolicyViolation, closeErr.Code)
	require.Equal(t, "too many pending websocket requests", closeErr.Reason)
	select {
	case <-r.done:
	case <-ctx.Done():
		t.Fatal("overflow reader leaked")
	}
	require.False(t, r.disconnected(), "local overflow is not a downstream disconnect")
	var typed *OpenAIWSClientCloseError
	require.True(t, errors.As(r.err, &typed))
}

func TestOpenAIWSIngressDisconnectKeepsUpstreamFailure(t *testing.T) {
	for _, mode := range []string{OpenAIWSIngressModeCtxPool, OpenAIWSIngressModeHTTPBridge} {
		t.Run(mode, func(t *testing.T) {
			cfg := passthroughLifecycleConfig()
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
			after := make(chan error, 2)
			server, done := startPassthroughLifecycleServer(t, context.Background(), svc, account, &OpenAIWSIngressHooks{AfterTurn: func(_ int, _ *OpenAIForwardResult, err error) { after <- err }})
			defer server.Close()
			client := dialPassthroughLifecycleClient(t, server)
			defer func() { _ = client.CloseNow() }()
			if mode == OpenAIWSIngressModeCtxPool {
				requirePassthroughUpstreamWrite(t, upstream, time.Second)
			} else {
				_, err := io.WriteString(writer, ": ready\n\n")
				require.NoError(t, err)
			}
			require.NoError(t, client.Close(coderws.StatusNormalClosure, "done"))
			if mode == OpenAIWSIngressModeCtxPool {
				_ = upstream.Close()
			} else {
				_ = writer.CloseWithError(errors.New("upstream stream failed"))
			}
			select {
			case err := <-done:
				require.Error(t, err)
			case <-time.After(3 * time.Second):
				t.Fatal("upstream failure did not return")
			}
			require.Error(t, <-after)
			require.Empty(t, after)
		})
	}
}
