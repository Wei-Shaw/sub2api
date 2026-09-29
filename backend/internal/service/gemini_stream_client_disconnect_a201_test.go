//go:build unit

package service

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

// A2-01：Gemini 三条流式路径（原生 / messages 兼容 / chat completions 兼容）在客户端中途断开后，
// 必须继续读取上游并返回已观测到的 usage，而不是以 (nil, err) 丢弃、让 handler 跳过计费。

const (
	a201Chunk1 = `data: {"candidates":[{"content":{"parts":[{"text":"hello"}],"role":"model"}}],"usageMetadata":{"candidatesTokenCount":5,"promptTokenCount":10,"totalTokenCount":15}}` + "\n\n"
	a201Chunk2 = `data: {"candidates":[{"content":{"parts":[{"text":"hello world"}],"role":"model"},"finishReason":"STOP"}],"usageMetadata":{"candidatesTokenCount":20,"promptTokenCount":10,"totalTokenCount":30}}` + "\n\n"
)

// a201CtxBody 模拟 net/http 的响应体：请求 context 取消后读取返回 context 错误，
// 阻塞中的读取也会被唤醒。
type a201CtxBody struct {
	ctx context.Context
	pr  *io.PipeReader
}

func (b *a201CtxBody) Read(p []byte) (int, error) {
	if err := b.ctx.Err(); err != nil {
		return 0, err
	}
	return b.pr.Read(p)
}

func (b *a201CtxBody) Close() error {
	return b.pr.Close()
}

// a201Upstream 返回一条由测试逐块喂入的 SSE 流。
type a201Upstream struct {
	HTTPUpstream
	pw       *io.PipeWriter
	pr       *io.PipeReader
	reqCtx   chan context.Context
	canceled chan struct{}
}

func newA201Upstream() *a201Upstream {
	pr, pw := io.Pipe()
	return &a201Upstream{pw: pw, pr: pr, reqCtx: make(chan context.Context, 1), canceled: make(chan struct{})}
}

func (u *a201Upstream) Do(req *http.Request, _ string, _ int64, _ int) (*http.Response, error) {
	ctx := req.Context()
	context.AfterFunc(ctx, func() {
		_ = u.pw.CloseWithError(context.Cause(ctx))
		close(u.canceled)
	})
	u.reqCtx <- ctx
	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
		Body:       &a201CtxBody{ctx: ctx, pr: u.pr},
	}, nil
}

// a201ClientWriter 模拟下游客户端：disconnect 后所有写入失败，并取消请求 context
// （net/http 在客户端连接关闭时的行为）。写入内容含 "hello" 时通知 sawHello。
type a201ClientWriter struct {
	header    http.Header
	mu        sync.Mutex
	gone      bool
	cancel    context.CancelFunc
	sawHello  chan struct{}
	helloOnce sync.Once
}

func (w *a201ClientWriter) Header() http.Header { return w.header }
func (w *a201ClientWriter) WriteHeader(int)     {}
func (w *a201ClientWriter) Flush()              {}

func (w *a201ClientWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	gone := w.gone
	w.mu.Unlock()
	if gone {
		return 0, errors.New("write: broken pipe")
	}
	if strings.Contains(string(p), "hello") {
		w.helloOnce.Do(func() { close(w.sawHello) })
	}
	return len(p), nil
}

func (w *a201ClientWriter) disconnect() {
	w.mu.Lock()
	w.gone = true
	w.mu.Unlock()
	w.cancel()
}

type a201Path struct {
	name    string
	forward func(svc *GeminiMessagesCompatService, ctx context.Context, c *gin.Context) (*ForwardResult, error)
}

func a201Paths() []a201Path {
	return []a201Path{
		{
			name: "native",
			forward: func(svc *GeminiMessagesCompatService, ctx context.Context, c *gin.Context) (*ForwardResult, error) {
				return svc.ForwardNative(ctx, c, a202GeminiAccount(), "gemini-2.5-flash", "streamGenerateContent", true,
					[]byte(`{"contents":[{"role":"user","parts":[{"text":"hi"}]}]}`))
			},
		},
		{
			name: "messages_compat",
			forward: func(svc *GeminiMessagesCompatService, ctx context.Context, c *gin.Context) (*ForwardResult, error) {
				return svc.Forward(ctx, c, a202GeminiAccount(),
					[]byte(`{"model":"gemini-2.5-flash","max_tokens":64,"stream":true,"messages":[{"role":"user","content":"hi"}]}`))
			},
		},
		{
			name: "chat_completions_compat",
			forward: func(svc *GeminiMessagesCompatService, ctx context.Context, c *gin.Context) (*ForwardResult, error) {
				return svc.ForwardAsChatCompletions(ctx, c, a202GeminiAccount(),
					[]byte(`{"model":"gemini-2.5-flash","stream":true,"messages":[{"role":"user","content":"hi"}]}`))
			},
		},
	}
}

type a201Run struct {
	upstream *a201Upstream
	client   *a201ClientWriter
	done     chan struct{}
	result   *ForwardResult
	err      error
}

func startA201Run(t *testing.T, p a201Path, cfg *config.Config) *a201Run {
	t.Helper()
	gin.SetMode(gin.TestMode)
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	upstream := newA201Upstream()
	t.Cleanup(func() { _ = upstream.pw.Close() })
	client := &a201ClientWriter{header: http.Header{}, cancel: cancel, sawHello: make(chan struct{})}
	c, _ := gin.CreateTestContext(client)
	c.Request = httptest.NewRequest(http.MethodPost, "/", strings.NewReader("{}")).WithContext(ctx)

	svc := &GeminiMessagesCompatService{
		httpUpstream:     upstream,
		cfg:              cfg,
		rateLimitService: NewRateLimitService(&errorPolicyRepoStub{}, nil, cfg, nil, nil),
	}
	run := &a201Run{upstream: upstream, client: client, done: make(chan struct{})}
	go func() {
		defer close(run.done)
		run.result, run.err = p.forward(svc, ctx, c)
	}()
	return run
}

// sendFirstChunkThenDisconnect 喂入首个带 usage 的块，等下游写出其内容后模拟客户端断开。
func (r *a201Run) sendFirstChunkThenDisconnect(t *testing.T) {
	t.Helper()
	_, err := io.WriteString(r.upstream.pw, a201Chunk1)
	require.NoError(t, err)
	select {
	case <-r.client.sawHello:
	case <-time.After(5 * time.Second):
		t.Fatal("首块内容未写到下游")
	}
	r.client.disconnect()
}

func (r *a201Run) wait(t *testing.T, timeout time.Duration) {
	t.Helper()
	select {
	case <-r.done:
	case <-time.After(timeout):
		t.Fatal("流式转发未在预期时间内返回")
	}
}

// 客户端断开后继续排水上游：返回的 usage 为上游最终 usageMetadata（断开之后才到达）。
func TestA201GeminiStream_ClientDisconnectKeepsDrainingAndReturnsUsage(t *testing.T) {
	for _, p := range a201Paths() {
		t.Run(p.name, func(t *testing.T) {
			run := startA201Run(t, p, &config.Config{})
			run.sendFirstChunkThenDisconnect(t)

			// 上游请求 context 若随客户端取消，AfterFunc 会关闭上游 body；否则继续发送剩余数据。
			select {
			case <-run.upstream.canceled:
			case <-time.After(100 * time.Millisecond):
				_, _ = io.WriteString(run.upstream.pw, a201Chunk2)
				_ = run.upstream.pw.Close()
			}
			run.wait(t, 5*time.Second)

			require.NoError(t, run.err)
			require.NotNil(t, run.result, "客户端断开后必须返回已收集的 usage 供计费")
			require.Equal(t, 10, run.result.Usage.InputTokens)
			require.Equal(t, 20, run.result.Usage.OutputTokens, "断开后应继续读取上游直到最终 usage")
			require.True(t, run.result.Stream)
			require.True(t, run.result.ClientDisconnect)
		})
	}
}

// 客户端断开后上游挂起：排水受 stream_data_interval_timeout 约束，超时后返回已观测的 usage。
func TestA201GeminiStream_ClientDisconnectIdleUpstreamReturnsObservedUsage(t *testing.T) {
	cfg := &config.Config{Gateway: config.GatewayConfig{StreamDataIntervalTimeout: 1}}
	for _, p := range a201Paths() {
		t.Run(p.name, func(t *testing.T) {
			run := startA201Run(t, p, cfg)
			run.sendFirstChunkThenDisconnect(t)
			run.wait(t, 5*time.Second)

			require.NoError(t, run.err)
			require.NotNil(t, run.result, "客户端断开后必须返回已收集的 usage 供计费")
			require.Equal(t, 10, run.result.Usage.InputTokens)
			require.Equal(t, 5, run.result.Usage.OutputTokens)
			require.True(t, run.result.ClientDisconnect)
		})
	}
}

// 上游请求 context 与客户端请求 context 解耦（仅流式）。
func TestA201GeminiStream_UpstreamContextDetachedFromClient(t *testing.T) {
	for _, p := range a201Paths() {
		t.Run(p.name, func(t *testing.T) {
			run := startA201Run(t, p, &config.Config{})
			var upstreamCtx context.Context
			select {
			case upstreamCtx = <-run.upstream.reqCtx:
			case <-time.After(5 * time.Second):
				t.Fatal("上游请求未发出")
			}
			require.Nil(t, upstreamCtx.Done(), "流式上游请求不应随客户端 context 取消")
			_, _ = io.WriteString(run.upstream.pw, a201Chunk2)
			_ = run.upstream.pw.Close()
			run.wait(t, 5*time.Second)
			require.NoError(t, run.err)
			require.NotNil(t, run.result)
			require.False(t, run.result.ClientDisconnect)
			require.Equal(t, 20, run.result.Usage.OutputTokens)
		})
	}
}
