package service

import (
	"context"
	"errors"
	"io"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
)

// errAntigravityFirstByteTimeout 表示 Antigravity 上游在首字节截止时间前没有任何响应体数据。
// 该错误只在尚未收到任何上游响应体字节时产生，因此可以安全地走账号 failover。
var errAntigravityFirstByteTimeout = errors.New("antigravity upstream first byte timeout")

// antigravityFirstByteGuard 从请求发出开始计时，直到响应体读出第一个字节。
// 它同时覆盖“等待响应头”和“响应头已到但响应体静默”两个阶段。
type antigravityFirstByteGuard struct {
	cancel context.CancelFunc
	timer  *time.Timer
	state  atomic.Int32
	mu     sync.Mutex
	body   io.ReadCloser
}

const (
	antigravityFirstByteArmed int32 = iota
	antigravityFirstByteReleased
	antigravityFirstByteFired
)

func newAntigravityFirstByteGuard(parent context.Context, timeout time.Duration) (context.Context, *antigravityFirstByteGuard) {
	if parent == nil || timeout <= 0 {
		return parent, nil
	}
	ctx, cancel := context.WithCancel(parent)
	guard := &antigravityFirstByteGuard{cancel: cancel}
	guard.timer = time.AfterFunc(timeout, guard.fire)
	return ctx, guard
}

func (g *antigravityFirstByteGuard) fire() {
	if g == nil || !g.state.CompareAndSwap(antigravityFirstByteArmed, antigravityFirstByteFired) {
		return
	}
	g.cancel()
	g.mu.Lock()
	body := g.body
	g.mu.Unlock()
	if body != nil {
		_ = body.Close()
	}
}

func (g *antigravityFirstByteGuard) wrap(body io.ReadCloser) io.ReadCloser {
	if g == nil || body == nil {
		return body
	}
	g.mu.Lock()
	g.body = body
	alreadyFired := g.state.Load() == antigravityFirstByteFired
	g.mu.Unlock()
	if alreadyFired {
		_ = body.Close()
	}
	return &antigravityFirstByteBody{ReadCloser: body, guard: g}
}

func (g *antigravityFirstByteGuard) release() {
	if g == nil {
		return
	}
	g.state.CompareAndSwap(antigravityFirstByteArmed, antigravityFirstByteReleased)
	if g.timer != nil {
		g.timer.Stop()
	}
}

func (g *antigravityFirstByteGuard) abort() {
	if g == nil {
		return
	}
	g.release()
	g.cancel()
}

func (g *antigravityFirstByteGuard) Fired() bool {
	return g != nil && g.state.Load() == antigravityFirstByteFired
}

type antigravityFirstByteBody struct {
	io.ReadCloser
	guard *antigravityFirstByteGuard
}

func (b *antigravityFirstByteBody) Read(p []byte) (int, error) {
	n, err := b.ReadCloser.Read(p)
	if n > 0 {
		b.guard.release()
		return n, err
	}
	if b.guard.Fired() {
		return 0, errAntigravityFirstByteTimeout
	}
	return n, err
}

func (b *antigravityFirstByteBody) Close() error {
	b.guard.release()
	err := b.ReadCloser.Close()
	b.guard.cancel()
	return err
}

func (s *AntigravityGatewayService) antigravityFirstByteTimeout() time.Duration {
	if s == nil || s.settingService == nil || s.settingService.cfg == nil {
		return 0
	}
	seconds := s.settingService.cfg.Gateway.AntigravityFirstByteTimeout
	if seconds <= 0 {
		return 0
	}
	return time.Duration(seconds) * time.Second
}

func newAntigravityFirstByteTimeoutFailoverError(resp *http.Response, prefix string) *UpstreamFailoverError {
	var headers http.Header
	if resp != nil {
		headers = resp.Header.Clone()
	}
	logger.LegacyPrintf("service.antigravity_gateway", "Antigravity first byte timeout (%s), triggering failover", prefix)
	return &UpstreamFailoverError{
		StatusCode:               http.StatusGatewayTimeout,
		ResponseBody:             []byte(`{"error":{"type":"first_byte_timeout","message":"Antigravity upstream produced no bytes before the first-byte deadline"}}`),
		ResponseHeaders:          headers,
		SafeToFailoverAfterWrite: true,
	}
}

// doAntigravityUpstreamRequest 在首字节超时保护下执行一次上游请求。
// 响应头等待超时直接返回可 failover 的错误；响应体首字节超时由包装后的 Body 返回
// errAntigravityFirstByteTimeout，供流式处理层转换为统一的 UpstreamFailoverError。
func (s *AntigravityGatewayService) doAntigravityUpstreamRequest(p antigravityRetryLoopParams, req *http.Request) (*http.Response, error) {
	timeout := s.antigravityFirstByteTimeout()
	if timeout <= 0 {
		return p.httpUpstream.Do(req, p.proxyURL, p.account.ID, p.account.Concurrency)
	}

	guardedCtx, guard := newAntigravityFirstByteGuard(p.ctx, timeout)
	resp, err := p.httpUpstream.Do(req.WithContext(guardedCtx), p.proxyURL, p.account.ID, p.account.Concurrency)
	if err != nil {
		guard.abort()
		if guard.Fired() && p.ctx.Err() == nil {
			return nil, newAntigravityFirstByteTimeoutFailoverError(nil, p.prefix)
		}
		return nil, err
	}
	if resp == nil {
		guard.abort()
		return nil, nil
	}
	resp.Body = guard.wrap(resp.Body)
	return resp, nil
}
