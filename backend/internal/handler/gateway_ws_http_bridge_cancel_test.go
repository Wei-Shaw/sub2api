package handler

import (
	"compress/flate"
	"compress/gzip"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/Wei-Shaw/sub2api/internal/testutil"
	coderws "github.com/coder/websocket"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type bridgeCloseBarrierBody struct {
	io.ReadCloser
	reading         atomic.Bool
	reads           atomic.Int32
	closeCalls      atomic.Int32
	concurrentClose atomic.Bool
	readStarted     chan struct{}
	closeStarted    chan struct{}
	allowClose      chan struct{}
}

func (b *bridgeCloseBarrierBody) Read(p []byte) (int, error) {
	b.reading.Store(true)
	defer b.reading.Store(false)
	if b.reads.Add(1) == 2 {
		close(b.readStarted)
	}
	return b.ReadCloser.Read(p)
}

func (b *bridgeCloseBarrierBody) Close() error {
	b.concurrentClose.Store(b.reading.Load())
	if b.closeCalls.Add(1) == 1 {
		close(b.closeStarted)
	}
	<-b.allowClose
	return b.ReadCloser.Close()
}

type bridgeRealTransport struct {
	service.HTTPUpstream
	encoding string
	body     *bridgeCloseBarrierBody
}

func (u *bridgeRealTransport) Do(req *http.Request, proxy string, account int64, concurrency int) (*http.Response, error) {
	// Explicit encoding makes repository.decompressResponseBody own decoding,
	// rather than net/http's automatic gzip decoding.
	req.Header.Set("Accept-Encoding", u.encoding)
	resp, err := u.HTTPUpstream.Do(req, proxy, account, concurrency)
	if err != nil {
		return nil, err
	}
	u.body.ReadCloser = resp.Body
	resp.Body = u.body
	return resp, nil
}

func TestWSHTTPBridgeCompressedCancellationClosesBeforeSlotRelease(t *testing.T) {
	for _, encoding := range []string{"gzip", "deflate"} {
		for _, cancellation := range []string{"outer", "disconnect"} {
			t.Run(encoding+"/"+cancellation, func(t *testing.T) {
				upstreamCanceled := make(chan struct{})
				upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.Header().Set("Content-Type", "text/event-stream")
					w.Header().Set("Content-Encoding", encoding)
					var compressed interface {
						io.WriteCloser
						Flush() error
					}
					if encoding == "gzip" {
						compressed = gzip.NewWriter(w)
					} else {
						var err error
						compressed, err = flate.NewWriter(w, flate.DefaultCompression)
						if err != nil {
							return
						}
					}
					_, _ = io.WriteString(compressed, ": keepalive\n\n")
					_ = compressed.Flush()
					if err := http.NewResponseController(w).Flush(); err != nil {
						t.Errorf("flush compressed upstream response: %v", err)
						return
					}
					// No terminal event and no compressor trailer: the next SSE
					// read stalls until the actual HTTP request is canceled.
					<-r.Context().Done()
					close(upstreamCanceled)
				}))
				defer upstream.Close()
				cfg := &config.Config{}
				cfg.Security.URLAllowlist.AllowPrivateHosts = true
				cfg.Security.URLAllowlist.AllowInsecureHTTP = true
				cfg.Gateway.OpenAIWS.ModeRouterV2Enabled = true
				cfg.Gateway.OpenAIWS.WriteTimeoutSeconds = 1
				body := &bridgeCloseBarrierBody{readStarted: make(chan struct{}), closeStarted: make(chan struct{}), allowClose: make(chan struct{})}
				var allowOnce sync.Once
				allowClose := func() { allowOnce.Do(func() { close(body.allowClose) }) }
				defer allowClose()
				transport := &bridgeRealTransport{HTTPUpstream: testutil.NewRealHTTPUpstream(cfg), encoding: encoding, body: body}
				svc := service.NewOpenAIGatewayService(nil, nil, nil, nil, nil, nil, nil, cfg, nil, nil, nil, nil, nil, transport, nil, nil, nil, nil, nil, nil, nil, nil)
				account := &service.Account{ID: 1, Platform: service.PlatformOpenAI, Type: service.AccountTypeAPIKey, Concurrency: 1,
					Credentials: map[string]any{"base_url": upstream.URL, "api_key": "sk-test"},
					Extra:       map[string]any{"openai_apikey_responses_websockets_v2_mode": service.OpenAIWSIngressModeHTTPBridge}}
				cache := &helperConcurrencyCacheStub{userSeq: []bool{true}}
				helper := NewConcurrencyHelper(service.NewConcurrencyService(cache), SSEPingFormatNone, time.Second)
				ctx, cancel := context.WithCancel(context.Background())
				defer cancel()
				done := make(chan error, 1)
				afterCalls := atomic.Int32{}
				gateway := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					conn, err := coderws.Accept(w, r, nil)
					if err != nil {
						done <- err
						return
					}
					defer func() { _ = conn.CloseNow() }()
					release, acquired, err := helper.TryAcquireWSUserSlotForAPIKey(ctx, 202, 3, 77, 0)
					if err != nil || !acquired {
						done <- err
						return
					}
					defer release()
					c, _ := gin.CreateTestContext(httptest.NewRecorder())
					c.Request = r.WithContext(ctx)
					done <- svc.ProxyResponsesWebSocketFromClient(ctx, c, conn, account, "sk-test",
						[]byte(`{"type":"response.create","model":"gpt-5","input":"hi"}`),
						&service.OpenAIWSIngressHooks{AfterTurn: func(_ int, _ *service.OpenAIForwardResult, _ error) { afterCalls.Add(1); release() }})
				}))
				defer func() { cancel(); allowClose(); gateway.Close() }()
				client, _, err := coderws.Dial(context.Background(), "ws"+strings.TrimPrefix(gateway.URL, "http"), nil)
				require.NoError(t, err)
				defer func() { _ = client.CloseNow() }()
				select {
				case <-body.readStarted:
				case err := <-done:
					t.Fatalf("bridge exited before reading: %v", err)
				case <-time.After(4 * time.Second):
					t.Fatal("compressed SSE did not reach stalled read")
				}
				if cancellation == "outer" {
					cancel()
				} else {
					require.NoError(t, client.CloseNow())
				}
				select {
				case <-body.closeStarted:
				case <-time.After(4 * time.Second):
					t.Fatal("transport cancellation did not unblock scanner")
				}
				require.False(t, body.concurrentClose.Load(), "Close must not race decompressor Read")
				require.Zero(t, afterCalls.Load(), "AfterTurn must wait for Body.Close")
				cache.mu.Lock()
				releasedBeforeClose := cache.apiKeyReleaseCalls
				cache.mu.Unlock()
				require.Zero(t, releasedBeforeClose, "outer cancellation must not release API key while upstream Close is blocked")
				allowClose()
				if cancellation == "outer" {
					readCtx, stop := context.WithTimeout(context.Background(), 4*time.Second)
					_, _, readErr := client.Read(readCtx)
					stop()
					require.Equal(t, coderws.StatusGoingAway, coderws.CloseStatus(readErr))
				}
				select {
				case err := <-done:
					if cancellation == "outer" {
						require.Error(t, err)
						var closeErr *service.OpenAIWSClientCloseError
						require.ErrorAs(t, err, &closeErr)
						require.ErrorIs(t, err, context.Canceled)
					} else {
						require.NoError(t, err)
					}
				case <-time.After(4 * time.Second):
					t.Fatal("turn cleanup did not finish")
				}
				select {
				case <-upstreamCanceled:
				case <-time.After(time.Second):
					t.Fatal("real upstream request was not canceled")
				}
				require.Equal(t, int32(1), body.closeCalls.Load())
				require.Equal(t, int32(1), afterCalls.Load())
				cache.mu.Lock()
				released := cache.apiKeyReleaseCalls
				cache.mu.Unlock()
				require.Equal(t, 1, released)
			})
		}
	}
}
