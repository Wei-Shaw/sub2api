package service

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	coderws "github.com/coder/websocket"
	"github.com/stretchr/testify/require"

	"github.com/Wei-Shaw/sub2api/internal/config"
)

// openAIWSReaderLoopFakeConn 模拟 coder/websocket 的契约：
// 控制帧只在有人阻塞读时被消费，因此 Ping 只有在存在未返回的 ReadMessage 时才成功。
type openAIWSReaderLoopFakeConn struct {
	messages  chan []byte
	failCh    chan struct{}
	failOnce  sync.Once
	failErr   error
	closed    chan struct{}
	closeOnce sync.Once
	readers   atomic.Int32
	pings     atomic.Int32
	pinging   atomic.Int32
	pingDelay time.Duration
	pingHold  chan struct{}
	pingErr   error
}

func newOpenAIWSReaderLoopFakeConn() *openAIWSReaderLoopFakeConn {
	return &openAIWSReaderLoopFakeConn{
		messages: make(chan []byte, 8),
		failCh:   make(chan struct{}),
		closed:   make(chan struct{}),
	}
}

func (c *openAIWSReaderLoopFakeConn) WriteJSON(context.Context, any) error {
	return nil
}

func (c *openAIWSReaderLoopFakeConn) ReadMessage(ctx context.Context) ([]byte, error) {
	c.readers.Add(1)
	defer c.readers.Add(-1)
	select {
	case msg := <-c.messages:
		return msg, nil
	case <-c.failCh:
		return nil, c.failErr
	case <-c.closed:
		return nil, errors.New("closed")
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func (c *openAIWSReaderLoopFakeConn) Ping(ctx context.Context) error {
	c.pings.Add(1)
	if c.readers.Load() == 0 {
		return errors.New("ping without reader")
	}
	c.pinging.Add(1)
	defer c.pinging.Add(-1)
	if c.pingHold != nil {
		select {
		case <-c.pingHold:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	if c.pingDelay > 0 {
		timer := time.NewTimer(c.pingDelay)
		defer timer.Stop()
		select {
		case <-timer.C:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	return c.pingErr
}

// openAIWSSlowPingConn 是没有读循环的旧式连接，用来确认请求路径上的健康检查语义不变。
type openAIWSSlowPingConn struct {
	openAIWSFakeConn
	pingDelay time.Duration
}

func (c *openAIWSSlowPingConn) Ping(ctx context.Context) error {
	timer := time.NewTimer(c.pingDelay)
	defer timer.Stop()
	select {
	case <-timer.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (c *openAIWSReaderLoopFakeConn) SupportsIdlePingWithoutReader() bool {
	return false
}

func (c *openAIWSReaderLoopFakeConn) RequiresReaderLoop() bool {
	return true
}

func (c *openAIWSReaderLoopFakeConn) Close() error {
	c.closeOnce.Do(func() { close(c.closed) })
	return nil
}

func (c *openAIWSReaderLoopFakeConn) failReads(err error) {
	c.failOnce.Do(func() {
		c.failErr = err
		close(c.failCh)
	})
}

func (c *openAIWSReaderLoopFakeConn) isClosed() bool {
	select {
	case <-c.closed:
		return true
	default:
		return false
	}
}

type openAIWSReaderLoopFakeDialer struct {
	mu    sync.Mutex
	conns []*openAIWSReaderLoopFakeConn
}

func (d *openAIWSReaderLoopFakeDialer) Dial(context.Context, string, http.Header, string) (openAIWSClientConn, int, http.Header, error) {
	conn := newOpenAIWSReaderLoopFakeConn()
	d.mu.Lock()
	d.conns = append(d.conns, conn)
	d.mu.Unlock()
	return conn, http.StatusSwitchingProtocols, nil, nil
}

func (d *openAIWSReaderLoopFakeDialer) dialed() []*openAIWSReaderLoopFakeConn {
	d.mu.Lock()
	defer d.mu.Unlock()
	return append([]*openAIWSReaderLoopFakeConn(nil), d.conns...)
}

func requireConnClosed(t *testing.T, conn *openAIWSConn) {
	t.Helper()
	select {
	case <-conn.closedCh:
	case <-time.After(time.Second):
		t.Fatal("连接应已关闭")
	}
}

func TestCoderOpenAIWSClientConn_RequiresReaderLoop(t *testing.T) {
	require.True(t, (&coderOpenAIWSClientConn{}).RequiresReaderLoop())
}

func TestOpenAIWSConnReaderLoop_NotStartedForPlainConn(t *testing.T) {
	conn := newOpenAIWSConn("rl_plain", 1, &openAIWSFakeConn{}, nil)
	defer conn.close()
	require.False(t, conn.hasReaderLoop())
}

func TestOpenAIWSConnReaderLoop_KeepsReaderWhileIdleSoPingSucceeds(t *testing.T) {
	fake := newOpenAIWSReaderLoopFakeConn()
	conn := newOpenAIWSConn("rl_idle_ping", 1, fake, nil)

	require.True(t, conn.hasReaderLoop())
	require.Eventually(t, func() bool { return fake.readers.Load() == 1 }, time.Second, 5*time.Millisecond)
	require.True(t, conn.supportsIdlePingWithoutReader())
	require.NoError(t, conn.pingWithTimeout(time.Second))

	conn.close()
	require.Eventually(t, func() bool { return fake.readers.Load() == 0 }, time.Second, 5*time.Millisecond, "关闭后读循环应退出")
}

func TestOpenAIWSConnReaderLoop_DeliversMessagesInOrder(t *testing.T) {
	fake := newOpenAIWSReaderLoopFakeConn()
	conn := newOpenAIWSConn("rl_order", 1, fake, nil)
	defer conn.close()
	require.True(t, conn.tryAcquire())

	fake.messages <- []byte("m1")
	fake.messages <- []byte("m2")
	fake.messages <- []byte("m3")
	for _, want := range []string{"m1", "m2", "m3"} {
		got, err := conn.readMessageWithTimeout(time.Second)
		require.NoError(t, err)
		require.Equal(t, want, string(got))
	}
}

func TestOpenAIWSConnReaderLoop_ReadTimeoutClosesConn(t *testing.T) {
	fake := newOpenAIWSReaderLoopFakeConn()
	conn := newOpenAIWSConn("rl_timeout", 1, fake, nil)
	require.True(t, conn.tryAcquire())

	_, err := conn.readMessageWithTimeout(30 * time.Millisecond)
	require.ErrorIs(t, err, context.DeadlineExceeded)
	requireConnClosed(t, conn)
	require.True(t, fake.isClosed())
}

func TestOpenAIWSConnReaderLoop_UpstreamCloseWhileIdleClosesConn(t *testing.T) {
	fake := newOpenAIWSReaderLoopFakeConn()
	conn := newOpenAIWSConn("rl_idle_close", 1, fake, nil)

	fake.failReads(errors.New("received close frame: keepalive ping timeout"))
	requireConnClosed(t, conn)
	require.False(t, conn.tryAcquire(), "上游已关闭的连接不能再被借出")
	require.True(t, conn.readerLoopClosedByPeer(), "上游主动关闭应被记为对端关闭")
}

func TestOpenAIWSConnReaderLoop_LocalCloseIsNotPeerClose(t *testing.T) {
	fake := newOpenAIWSReaderLoopFakeConn()
	conn := newOpenAIWSConn("rl_local_close", 1, fake, nil)
	require.Eventually(t, func() bool { return fake.readers.Load() == 1 }, time.Second, 5*time.Millisecond)

	conn.close()
	require.Eventually(t, func() bool { return fake.readers.Load() == 0 }, time.Second, 5*time.Millisecond)
	require.False(t, conn.readerLoopClosedByPeer(), "本地主动关闭引发的读错误不应记为对端关闭")
}

func TestOpenAIWSConnReaderLoop_UpstreamCloseWhileReadingReturnsCause(t *testing.T) {
	fake := newOpenAIWSReaderLoopFakeConn()
	conn := newOpenAIWSConn("rl_read_close", 1, fake, nil)
	require.True(t, conn.tryAcquire())

	upstreamErr := errors.New("received close frame: keepalive ping timeout")
	done := make(chan error, 1)
	go func() {
		_, err := conn.readMessageWithTimeout(time.Second)
		done <- err
	}()
	require.Eventually(t, func() bool {
		if conn.readMu.TryLock() {
			conn.readMu.Unlock()
			return false
		}
		return true
	}, time.Second, 5*time.Millisecond, "借用者应已进入读等待")
	fake.failReads(upstreamErr)

	select {
	case err := <-done:
		require.ErrorIs(t, err, upstreamErr)
	case <-time.After(time.Second):
		t.Fatal("借用者应收到上游关闭错误")
	}
	requireConnClosed(t, conn)
}

func TestOpenAIWSConnReaderLoop_ReadAfterUpstreamCloseReturnsCause(t *testing.T) {
	fake := newOpenAIWSReaderLoopFakeConn()
	conn := newOpenAIWSConn("rl_late_read", 1, fake, nil)
	require.True(t, conn.tryAcquire())

	upstreamErr := errors.New("received close frame: keepalive ping timeout")
	fake.failReads(upstreamErr)
	requireConnClosed(t, conn)

	_, err := conn.readMessageWithTimeout(time.Second)
	require.ErrorIs(t, err, upstreamErr, "持有租约期间上游关闭，之后的读应带回关闭原因")
}

// 上游发完最后一条事件随即关闭连接时，借用者必须先拿到已缓存的事件，之后才收到关闭错误。
func TestOpenAIWSConnReaderLoop_BufferedMessageDeliveredAfterPeerClose(t *testing.T) {
	fake := newOpenAIWSReaderLoopFakeConn()
	conn := newOpenAIWSConn("rl_buffered_close", 1, fake, nil)
	require.True(t, conn.tryAcquire())

	fake.messages <- []byte(`{"type":"response.completed"}`)
	require.Eventually(t, conn.readerLoopPending, time.Second, 5*time.Millisecond)
	upstreamErr := errors.New("received close frame: status = StatusNormalClosure")
	fake.failReads(upstreamErr)
	requireConnClosed(t, conn)

	payload, err := conn.readMessageWithTimeout(time.Second)
	require.NoError(t, err, "连接关闭前已缓存的事件不能丢")
	require.Equal(t, `{"type":"response.completed"}`, string(payload))

	_, err = conn.readMessageWithTimeout(time.Second)
	require.ErrorIs(t, err, upstreamErr)
}

func TestOpenAIWSConnPool_PeerClosedIdleConnEvictedImmediately(t *testing.T) {
	cfg := &config.Config{}
	cfg.Gateway.OpenAIWS.MaxConnsPerAccount = 1
	cfg.Gateway.OpenAIWS.MaxIdlePerAccount = 1
	cfg.Gateway.OpenAIWS.DialTimeoutSeconds = 1
	pool := newOpenAIWSConnPool(cfg)
	defer pool.Close()
	dialer := &openAIWSReaderLoopFakeDialer{}
	pool.setClientDialerForTest(dialer)

	account := &Account{ID: 307, Platform: PlatformOpenAI, Type: AccountTypeAPIKey}
	req := openAIWSAcquireRequest{Account: account, WSURL: "wss://example.com/v1/responses"}

	first, err := pool.Acquire(context.Background(), req)
	require.NoError(t, err)
	firstID := first.ConnID()
	first.Release()

	dialer.dialed()[0].failReads(errors.New("received close frame: keepalive ping timeout"))
	requireConnClosed(t, first.conn)

	ap, ok := pool.getAccountPool(account.ID)
	require.True(t, ok)
	require.Eventually(t, func() bool {
		ap.mu.Lock()
		defer ap.mu.Unlock()
		_, exists := ap.conns[firstID]
		return !exists
	}, time.Second, 5*time.Millisecond, "上游关闭的空闲连接应立即移出账号池")

	acquireCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	second, err := pool.Acquire(acquireCtx, req)
	require.NoError(t, err, "池满时死连接不应继续占用容量")
	defer second.Release()
	require.NotEqual(t, firstID, second.ConnID())
	require.Len(t, dialer.dialed(), 2)
}

func newReaderLoopTestPool(t *testing.T, maxConns int) (*openAIWSConnPool, *openAIWSReaderLoopFakeDialer, openAIWSAcquireRequest) {
	t.Helper()
	cfg := &config.Config{}
	cfg.Gateway.OpenAIWS.MaxConnsPerAccount = maxConns
	cfg.Gateway.OpenAIWS.MaxIdlePerAccount = maxConns
	cfg.Gateway.OpenAIWS.DialTimeoutSeconds = 1
	pool := newOpenAIWSConnPool(cfg)
	t.Cleanup(pool.Close)
	dialer := &openAIWSReaderLoopFakeDialer{}
	pool.setClientDialerForTest(dialer)
	account := &Account{ID: 308, Platform: PlatformOpenAI, Type: AccountTypeAPIKey}
	return pool, dialer, openAIWSAcquireRequest{Account: account, WSURL: "wss://example.com/v1/responses"}
}

func requireConnGone(t *testing.T, pool *openAIWSConnPool, accountID int64, connID string) {
	t.Helper()
	ap, ok := pool.getAccountPool(accountID)
	require.True(t, ok)
	ap.mu.Lock()
	defer ap.mu.Unlock()
	_, exists := ap.conns[connID]
	require.False(t, exists, "失效连接应已移出账号池")
}

// 池上限为 1：唯一的空闲连接空闲期收到残留消息，本次获取必须直接新建，不能等清理周期。
func TestOpenAIWSConnPool_DirtyIdleConnReplacedAtCapacity(t *testing.T) {
	pool, dialer, req := newReaderLoopTestPool(t, 1)

	first, err := pool.Acquire(context.Background(), req)
	require.NoError(t, err)
	firstID := first.ConnID()
	first.Release()
	dialer.dialed()[0].messages <- []byte(`{"type":"response.output_text.delta"}`)
	require.Eventually(t, first.conn.readerLoopPending, time.Second, 5*time.Millisecond)

	acquireCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	second, err := pool.Acquire(acquireCtx, req)
	require.NoError(t, err)
	defer second.Release()
	require.NotEqual(t, firstID, second.ConnID())
	require.Len(t, dialer.dialed(), 2)
	requireConnGone(t, pool, req.Account.ID, firstID)
	requireConnClosed(t, first.conn)
}

// 池已满且其余连接都在忙：脏的空闲连接要让位给新建连接，而不是把请求挂到忙连接上排队。
func TestOpenAIWSConnPool_DirtyIdleConnReplacedWhileOthersBusy(t *testing.T) {
	pool, dialer, req := newReaderLoopTestPool(t, 2)

	busy, err := pool.Acquire(context.Background(), req)
	require.NoError(t, err)
	defer busy.Release()
	idle, err := pool.Acquire(context.Background(), req)
	require.NoError(t, err)
	idleID := idle.ConnID()
	idle.Release()
	dialer.dialed()[1].messages <- []byte(`{"type":"response.output_text.delta"}`)
	require.Eventually(t, idle.conn.readerLoopPending, time.Second, 5*time.Millisecond)

	acquireCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	fresh, err := pool.Acquire(acquireCtx, req)
	require.NoError(t, err)
	defer fresh.Release()
	require.NotEqual(t, idleID, fresh.ConnID())
	require.NotEqual(t, busy.ConnID(), fresh.ConnID())
	require.Len(t, dialer.dialed(), 3)
	requireConnGone(t, pool, req.Account.ID, idleID)
}

// 排队等待的获取在拿到令牌时发现连接已脏：应把它出池并新建，而不是失败。
func TestOpenAIWSConnPool_QueuedAcquireReplacesDirtyConnOnHandoff(t *testing.T) {
	pool, dialer, req := newReaderLoopTestPool(t, 1)

	first, err := pool.Acquire(context.Background(), req)
	require.NoError(t, err)
	firstID := first.ConnID()

	type acquireResult struct {
		lease *openAIWSConnLease
		err   error
	}
	done := make(chan acquireResult, 1)
	go func() {
		acquireCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		lease, err := pool.Acquire(acquireCtx, req)
		done <- acquireResult{lease: lease, err: err}
	}()
	require.Eventually(t, func() bool { return first.conn.waiters.Load() == 1 }, time.Second, 5*time.Millisecond)

	dialer.dialed()[0].messages <- []byte(`{"type":"response.output_text.delta"}`)
	require.Eventually(t, first.conn.readerLoopPending, time.Second, 5*time.Millisecond)
	first.Release()

	select {
	case res := <-done:
		require.NoError(t, res.err)
		defer res.lease.Release()
		require.NotEqual(t, firstID, res.lease.ConnID())
	case <-time.After(4 * time.Second):
		t.Fatal("排队获取未返回")
	}
	require.Len(t, dialer.dialed(), 2)
	requireConnGone(t, pool, req.Account.ID, firstID)
}

func newSweepTestPool(conn *openAIWSConn, accountID int64) (*openAIWSConnPool, *openAIWSAccountPool) {
	cfg := &config.Config{}
	cfg.Gateway.OpenAIWS.MaxConnsPerAccount = 2
	cfg.Gateway.OpenAIWS.MaxIdlePerAccount = 2
	pool := &openAIWSConnPool{cfg: cfg}
	ap := &openAIWSAccountPool{conns: map[string]*openAIWSConn{conn.id: conn}}
	pool.accounts.Store(accountID, ap)
	return pool, ap
}

func requireInPool(t *testing.T, ap *openAIWSAccountPool, connID string, want bool) {
	t.Helper()
	ap.mu.Lock()
	defer ap.mu.Unlock()
	_, exists := ap.conns[connID]
	require.Equal(t, want, exists)
}

// 后台巡检经代理往返可能超过 2 秒：慢 pong 不应被判死。
func TestOpenAIWSConnPool_BackgroundPingSweepToleratesSlowPong(t *testing.T) {
	fake := newOpenAIWSReaderLoopFakeConn()
	fake.pingDelay = 2500 * time.Millisecond
	conn := newOpenAIWSConn("rl_slow_pong", 309, fake, nil)
	defer conn.close()
	pool, ap := newSweepTestPool(conn, 309)
	require.Eventually(t, func() bool { return fake.readers.Load() == 1 }, time.Second, 5*time.Millisecond)

	pool.runBackgroundPingSweep()

	requireInPool(t, ap, conn.id, true)
	require.False(t, conn.isClosed(), "慢 pong 的健康连接不应被巡检剔除")
	require.EqualValues(t, 1, fake.pings.Load())
}

// ping 期间连接被借出，随后 ping 失败：巡检不得剔除已借出的连接。
func TestOpenAIWSConnPool_BackgroundPingSweepSkipsLeasedConnOnFailure(t *testing.T) {
	fake := newOpenAIWSReaderLoopFakeConn()
	fake.pingHold = make(chan struct{})
	fake.pingErr = errors.New("pong lost")
	conn := newOpenAIWSConn("rl_leased_during_ping", 310, fake, nil)
	defer conn.close()
	pool, ap := newSweepTestPool(conn, 310)
	require.Eventually(t, func() bool { return fake.readers.Load() == 1 }, time.Second, 5*time.Millisecond)

	done := make(chan struct{})
	go func() {
		defer close(done)
		pool.runBackgroundPingSweep()
	}()
	require.Eventually(t, func() bool { return fake.pinging.Load() == 1 }, time.Second, 5*time.Millisecond)
	require.True(t, conn.tryAcquire(), "ping 进行中连接应仍可借出")
	close(fake.pingHold)
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("巡检未结束")
	}

	requireInPool(t, ap, conn.id, true)
	require.False(t, conn.isClosed(), "已借出的连接不应被巡检剔除")
}

// 巡检判定失败到真正剔除之间若被借出，绝不能关闭借用者手里的连接：
// 用 ap.mu 把 evictConn 卡在门口，在这个空隙里借出连接，复现"看一眼再动手"的竞态。
func TestOpenAIWSConnPool_BackgroundPingSweepNeverClosesConnLeasedInEvictWindow(t *testing.T) {
	fake := newOpenAIWSReaderLoopFakeConn()
	fake.pingHold = make(chan struct{})
	fake.pingErr = errors.New("pong lost")
	conn := newOpenAIWSConn("rl_evict_window", 312, fake, nil)
	defer conn.close()
	pool, ap := newSweepTestPool(conn, 312)
	require.Eventually(t, func() bool { return fake.readers.Load() == 1 }, time.Second, 5*time.Millisecond)

	done := make(chan struct{})
	go func() {
		defer close(done)
		pool.runBackgroundPingSweep()
	}()
	require.Eventually(t, func() bool { return fake.pinging.Load() == 1 }, time.Second, 5*time.Millisecond)

	ap.mu.Lock()
	close(fake.pingHold)
	time.Sleep(50 * time.Millisecond)
	leased := conn.tryAcquire()
	ap.mu.Unlock()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("巡检未结束")
	}

	if leased {
		require.False(t, conn.isClosed(), "借用者拿到租约后，巡检不得再关闭该连接")
		requireInPool(t, ap, conn.id, true)
	} else {
		require.True(t, conn.isClosed(), "巡检拿到租约令牌后才允许剔除")
	}
}

// 读超时后必须立即切断连接：上游已不响应时礼貌关闭握手要等 5 秒，会拖慢重试。
func TestOpenAIWSConnReaderLoop_ReadTimeoutAbortsWithoutCloseHandshake(t *testing.T) {
	release := make(chan struct{})
	defer close(release)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		srvConn, err := coderws.Accept(w, r, nil)
		if err != nil {
			return
		}
		<-release
		_ = srvConn.CloseNow()
	}))
	defer server.Close()

	ws, _, _, err := newDefaultOpenAIWSClientDialer().Dial(context.Background(), "ws"+strings.TrimPrefix(server.URL, "http"), nil, "")
	require.NoError(t, err)
	conn := newOpenAIWSConn("rl_read_timeout_abort", 1, ws, nil)
	defer conn.close()
	require.True(t, conn.tryAcquire())

	started := time.Now()
	_, err = conn.readMessageWithTimeout(20 * time.Millisecond)
	elapsed := time.Since(started)
	require.ErrorIs(t, err, context.DeadlineExceeded)
	require.Less(t, elapsed, 500*time.Millisecond, "读超时后关闭不应等待关闭握手")
	requireConnClosed(t, conn)
}

// ping 等待 pong 期间不能阻塞写：借用者写请求不应等到 pong 回来。
func TestOpenAIWSConnPingDoesNotBlockWrite(t *testing.T) {
	fake := newOpenAIWSReaderLoopFakeConn()
	fake.pingHold = make(chan struct{})
	conn := newOpenAIWSConn("rl_ping_write", 1, fake, nil)
	defer conn.close()
	defer close(fake.pingHold)
	require.Eventually(t, func() bool { return fake.readers.Load() == 1 }, time.Second, 5*time.Millisecond)

	pingDone := make(chan error, 1)
	go func() { pingDone <- conn.pingWithTimeout(5 * time.Second) }()
	require.Eventually(t, func() bool { return fake.pinging.Load() == 1 }, time.Second, 5*time.Millisecond)

	started := time.Now()
	require.NoError(t, conn.writeJSONWithTimeout(context.Background(), map[string]any{"type": "response.create"}, time.Second))
	require.Less(t, time.Since(started), 300*time.Millisecond, "写请求不应等待 ping 的 pong")
	require.EqualValues(t, 1, fake.pinging.Load(), "写完成时 ping 仍应在等待 pong")
}

// 有读循环的连接不再做借出前的空闲健康检查：上游关闭能被即时感知，巡检已覆盖半开探测。
func TestOpenAIWSConnPool_AcquireSkipsIdleHealthCheckForReaderLoopConn(t *testing.T) {
	pool, dialer, req := newReaderLoopTestPool(t, 1)

	first, err := pool.Acquire(context.Background(), req)
	require.NoError(t, err)
	firstID := first.ConnID()
	first.Release()
	first.conn.lastUsedNano.Store(time.Now().Add(-openAIWSConnHealthCheckIdle - time.Second).UnixNano())

	second, err := pool.Acquire(context.Background(), req)
	require.NoError(t, err)
	defer second.Release()
	require.Equal(t, firstID, second.ConnID())
	require.Zero(t, dialer.dialed()[0].pings.Load(), "有读循环的连接借出时不应再 ping")
}

// 没有读循环的旧式连接保持原语义：空闲超过阈值借出前 ping，2 秒无 pong 即换连接。
func TestOpenAIWSConnPool_AcquireIdleHealthCheckStillAppliesWithoutReaderLoop(t *testing.T) {
	cfg := &config.Config{}
	cfg.Gateway.OpenAIWS.MaxConnsPerAccount = 1
	cfg.Gateway.OpenAIWS.MaxIdlePerAccount = 1
	cfg.Gateway.OpenAIWS.DialTimeoutSeconds = 1
	pool := newOpenAIWSConnPool(cfg)
	defer pool.Close()
	dialer := &openAIWSQueueDialer{conns: []openAIWSClientConn{
		&openAIWSSlowPingConn{pingDelay: 2200 * time.Millisecond},
		&openAIWSFakeConn{},
	}}
	pool.setClientDialerForTest(dialer)
	account := &Account{ID: 311, Platform: PlatformOpenAI, Type: AccountTypeAPIKey}
	req := openAIWSAcquireRequest{Account: account, WSURL: "wss://example.com/v1/responses"}

	first, err := pool.Acquire(context.Background(), req)
	require.NoError(t, err)
	firstID := first.ConnID()
	first.Release()
	first.conn.lastUsedNano.Store(time.Now().Add(-openAIWSConnHealthCheckIdle - time.Second).UnixNano())

	started := time.Now()
	second, err := pool.Acquire(context.Background(), req)
	require.NoError(t, err)
	defer second.Release()
	require.NotEqual(t, firstID, second.ConnID(), "2 秒内无 pong 的旧式连接应被换掉")
	require.GreaterOrEqual(t, time.Since(started), openAIWSConnHealthCheckTO)
	require.Equal(t, 2, dialer.DialCount())
}

func TestOpenAIWSConnReaderLoop_DataWhileIdleRejectsLease(t *testing.T) {
	fake := newOpenAIWSReaderLoopFakeConn()
	conn := newOpenAIWSConn("rl_dirty_try", 1, fake, nil)

	fake.messages <- []byte(`{"type":"response.output_text.delta"}`)
	require.Eventually(t, conn.readerLoopPending, time.Second, 5*time.Millisecond)

	require.False(t, conn.tryAcquire(), "空闲期收到数据的连接不能借出")
	require.True(t, conn.isUnusable(), "脏连接应标记为不可用，由池在锁外关闭")
	require.False(t, conn.tryAcquire(), "脏连接的令牌不应归还，避免被其他获取者拿到")
}

func TestOpenAIWSConnReaderLoop_DataWhileIdleRejectsBlockingAcquire(t *testing.T) {
	fake := newOpenAIWSReaderLoopFakeConn()
	conn := newOpenAIWSConn("rl_dirty_acquire", 1, fake, nil)

	fake.messages <- []byte(`{"type":"response.output_text.delta"}`)
	require.Eventually(t, conn.readerLoopPending, time.Second, 5*time.Millisecond)

	require.ErrorIs(t, conn.acquire(context.Background()), errOpenAIWSConnClosed)
	require.True(t, conn.isUnusable(), "脏连接应标记为不可用，由池在锁外关闭")
	require.False(t, conn.tryAcquire(), "脏连接的令牌不应归还")
}

func TestOpenAIWSConnPool_ReaderLoopConnNotIdleRecycled(t *testing.T) {
	cfg := &config.Config{}
	cfg.Gateway.OpenAIWS.MaxConnsPerAccount = 2
	cfg.Gateway.OpenAIWS.MaxIdlePerAccount = 2
	pool := &openAIWSConnPool{cfg: cfg}

	accountID := int64(304)
	ap := &openAIWSAccountPool{conns: make(map[string]*openAIWSConn)}
	conn := newOpenAIWSConn("rl_no_recycle", accountID, newOpenAIWSReaderLoopFakeConn(), nil)
	defer conn.close()
	conn.lastUsedNano.Store(time.Now().Add(-openAIWSConnIdleRecycleAfter - time.Second).UnixNano())
	ap.conns[conn.id] = conn
	pool.accounts.Store(accountID, ap)

	pool.runBackgroundCleanupSweep(time.Now())

	ap.mu.Lock()
	_, exists := ap.conns[conn.id]
	ap.mu.Unlock()
	require.True(t, exists, "有常驻读循环的连接不应按空闲阈值回收")
}

// 用真实 coder/websocket 连接复现线上场景：空闲池化连接必须能应答服务端 ping，
// 服务端随后以 1011/keepalive ping timeout 关闭时，池要立即感知并保留关闭原因。
func TestOpenAIWSConnReaderLoop_RealConnAnswersServerPingWhileIdle(t *testing.T) {
	pingResult := make(chan error, 1)
	closeNow := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		srvConn, err := coderws.Accept(w, r, nil)
		if err != nil {
			pingResult <- err
			return
		}
		defer func() { _ = srvConn.CloseNow() }()
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()
		readDone := make(chan struct{})
		go func() {
			defer close(readDone)
			_, _, _ = srvConn.Reader(ctx)
		}()
		pingResult <- srvConn.Ping(ctx)
		<-closeNow
		_ = srvConn.Write(ctx, coderws.MessageText, []byte(`{"type":"response.completed"}`))
		_ = srvConn.Close(coderws.StatusInternalError, "keepalive ping timeout")
		<-readDone
	}))
	defer server.Close()

	ws, _, _, err := newDefaultOpenAIWSClientDialer().Dial(context.Background(), "ws"+strings.TrimPrefix(server.URL, "http"), nil, "")
	require.NoError(t, err)
	conn := newOpenAIWSConn("rl_real", 1, ws, nil)
	defer conn.close()
	require.True(t, conn.hasReaderLoop())

	select {
	case pingErr := <-pingResult:
		require.NoError(t, pingErr, "空闲池化连接应由读循环应答服务端 ping")
	case <-time.After(5 * time.Second):
		t.Fatal("服务端 ping 未在期限内收到 pong")
	}
	require.Eventually(t, func() bool { return conn.upstreamPingCount() == 1 }, time.Second, 5*time.Millisecond)

	require.True(t, conn.tryAcquire())
	close(closeNow)
	requireConnClosed(t, conn)
	require.True(t, conn.readerLoopClosedByPeer())

	payload, err := conn.readMessageWithTimeout(2 * time.Second)
	require.NoError(t, err, "close 帧之前的数据帧必须先交给借用者")
	require.Equal(t, `{"type":"response.completed"}`, string(payload))
	_, err = conn.readMessageWithTimeout(2 * time.Second)
	require.Error(t, err)
	require.Contains(t, err.Error(), "keepalive ping timeout")
}

func TestOpenAIWSConnPool_AcquireSnapshotsIdleBefore(t *testing.T) {
	cfg := &config.Config{}
	cfg.Gateway.OpenAIWS.MaxConnsPerAccount = 2
	cfg.Gateway.OpenAIWS.MaxIdlePerAccount = 2
	cfg.Gateway.OpenAIWS.DialTimeoutSeconds = 1
	pool := newOpenAIWSConnPool(cfg)
	defer pool.Close()
	pool.setClientDialerForTest(&openAIWSReaderLoopFakeDialer{})

	account := &Account{ID: 306, Platform: PlatformOpenAI, Type: AccountTypeAPIKey}
	req := openAIWSAcquireRequest{Account: account, WSURL: "wss://example.com/v1/responses"}

	first, err := pool.Acquire(context.Background(), req)
	require.NoError(t, err)
	require.Less(t, first.IdleBefore(), 500*time.Millisecond)
	first.Release()
	first.conn.lastUsedNano.Store(time.Now().Add(-5 * time.Second).UnixNano())
	first.conn.createdAtNano.Store(time.Now().Add(-time.Minute).UnixNano())

	second, err := pool.Acquire(context.Background(), req)
	require.NoError(t, err)
	defer second.Release()
	require.True(t, second.Reused())
	require.GreaterOrEqual(t, second.IdleBefore(), 5*time.Second, "借出时应记录借出前的空闲时长")
	require.GreaterOrEqual(t, second.AgeBefore(), time.Minute, "借出时应记录连接年龄")
	require.Zero(t, second.UpstreamPingCount())
}

func TestOpenAIWSConnPool_AcquireSkipsDirtyIdleConn(t *testing.T) {
	cfg := &config.Config{}
	cfg.Gateway.OpenAIWS.MaxConnsPerAccount = 2
	cfg.Gateway.OpenAIWS.MaxIdlePerAccount = 2
	cfg.Gateway.OpenAIWS.DialTimeoutSeconds = 1
	pool := newOpenAIWSConnPool(cfg)
	defer pool.Close()
	dialer := &openAIWSReaderLoopFakeDialer{}
	pool.setClientDialerForTest(dialer)

	account := &Account{ID: 305, Platform: PlatformOpenAI, Type: AccountTypeAPIKey}
	req := openAIWSAcquireRequest{Account: account, WSURL: "wss://example.com/v1/responses"}

	first, err := pool.Acquire(context.Background(), req)
	require.NoError(t, err)
	firstID := first.ConnID()
	first.Release()

	dialed := dialer.dialed()
	require.Len(t, dialed, 1)
	dialed[0].messages <- []byte(`{"type":"response.output_text.delta"}`)
	require.Eventually(t, first.conn.readerLoopPending, time.Second, 5*time.Millisecond)

	second, err := pool.Acquire(context.Background(), req)
	require.NoError(t, err)
	defer second.Release()
	require.NotEqual(t, firstID, second.ConnID(), "脏连接不应再被借出")
	require.Len(t, dialer.dialed(), 2)
	requireConnClosed(t, first.conn)
}
