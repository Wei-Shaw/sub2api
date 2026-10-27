package service

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

func TestOpenAIWSConnPool_CleanupRotatesConnBeforeUpstreamLimit(t *testing.T) {
	require.Equal(t, 50*time.Minute, openAIWSConnMaxAge, "上游 60 分钟硬上限需提前 10 分钟轮换")

	cfg := &config.Config{}
	cfg.Gateway.OpenAIWS.MaxIdlePerAccount = 4
	pool := newOpenAIWSConnPool(cfg)
	accountID := int64(301)
	ap := pool.getOrCreateAccountPool(accountID)

	expired := newOpenAIWSConn("expired", accountID, &openAIWSFakeConn{}, nil)
	expired.createdAtNano.Store(time.Now().Add(-51 * time.Minute).UnixNano())
	fresh := newOpenAIWSConn("fresh", accountID, &openAIWSFakeConn{}, nil)
	fresh.createdAtNano.Store(time.Now().Add(-49 * time.Minute).UnixNano())
	ap.conns[expired.id] = expired
	ap.conns[fresh.id] = fresh

	evicted := pool.cleanupAccountLocked(ap, time.Now(), pool.maxConnsHardCap())
	closeOpenAIWSConns(evicted)

	require.Nil(t, ap.conns["expired"], "满 50 分钟的空闲连接应被回收")
	require.NotNil(t, ap.conns["fresh"], "未满 50 分钟的空闲连接应保留")
}

func TestOpenAIWSConnPool_AcquireSkipsExpiredIdleConn(t *testing.T) {
	cfg := &config.Config{}
	cfg.Gateway.OpenAIWS.MaxConnsPerAccount = 2
	cfg.Gateway.OpenAIWS.MinIdlePerAccount = 0
	pool := newOpenAIWSConnPool(cfg)
	dialer := &openAIWSQueueDialer{conns: []openAIWSClientConn{&openAIWSFakeConn{}}}
	pool.setClientDialerForTest(dialer)

	accountID := int64(302)
	account := &Account{ID: accountID, Platform: PlatformOpenAI, Type: AccountTypeAPIKey}
	ap := pool.getOrCreateAccountPool(accountID)
	expired := newOpenAIWSConn("expired_idle", accountID, &openAIWSFakeConn{}, nil)
	expired.createdAtNano.Store(time.Now().Add(-51 * time.Minute).UnixNano())
	ap.mu.Lock()
	ap.conns[expired.id] = expired
	ap.lastCleanupAt = time.Now()
	ap.mu.Unlock()

	lease, err := pool.Acquire(context.Background(), openAIWSAcquireRequest{
		Account: account,
		WSURL:   "wss://example.com/v1/responses",
	})
	require.NoError(t, err)
	require.NotNil(t, lease)
	defer lease.Release()

	require.False(t, lease.Reused(), "到龄的空闲连接不应再借出")
	require.NotEqual(t, expired.id, lease.ConnID())
	require.Equal(t, 1, dialer.DialCount(), "应新拨一条连接")
	ap.mu.Lock()
	_, stillPooled := ap.conns[expired.id]
	ap.mu.Unlock()
	require.False(t, stillPooled, "到龄连接应在借出前被剔除，不受 cleanup 节流约束")
	require.True(t, expired.isClosed(), "被剔除的到龄连接应已关闭")
}

func TestOpenAIWSConnPool_PickLeastBusySkipsExpiredConn(t *testing.T) {
	pool := newOpenAIWSConnPool(&config.Config{})
	accountID := int64(303)
	ap := pool.getOrCreateAccountPool(accountID)

	expiredBusy := newOpenAIWSConn("expired_busy", accountID, &openAIWSFakeConn{}, nil)
	expiredBusy.createdAtNano.Store(time.Now().Add(-55 * time.Minute).UnixNano())
	require.True(t, expiredBusy.tryAcquire())
	freshBusy := newOpenAIWSConn("fresh_busy", accountID, &openAIWSFakeConn{}, nil)
	require.True(t, freshBusy.tryAcquire())
	freshBusy.waiters.Add(3)
	ap.conns[expiredBusy.id] = expiredBusy
	ap.conns[freshBusy.id] = freshBusy

	picked := pool.pickLeastBusyConnLocked(ap, "", openAIWSHandshakeCompatibilityKey{})
	require.NotNil(t, picked)
	require.Equal(t, freshBusy.id, picked.id, "排队时不应选到龄连接，即使它等待者更少")
	require.Equal(t, freshBusy.id, pool.pickLeastBusyConnLocked(ap, expiredBusy.id, openAIWSHandshakeCompatibilityKey{}).id, "首选连接到龄时同样跳过")
}

func TestOpenAIWSConnPool_AcquireWaitsForExpiredBusyConn(t *testing.T) {
	for _, routingHint := range []string{"", "route-age"} {
		t.Run("routing_hint="+routingHint, func(t *testing.T) {
			cfg := &config.Config{}
			cfg.Gateway.OpenAIWS.MaxConnsPerAccount = 1
			cfg.Gateway.OpenAIWS.MinIdlePerAccount = 0
			cfg.Gateway.OpenAIWS.MaxIdlePerAccount = 1
			pool := newOpenAIWSConnPool(cfg)
			defer pool.Close()
			pool.setClientDialerForTest(&openAIWSQueueDialer{conns: []openAIWSClientConn{&openAIWSFakeConn{}, &openAIWSFakeConn{}}})
			req := openAIWSAcquireRequest{
				Account: &Account{ID: 304, Platform: PlatformOpenAI, Type: AccountTypeAPIKey},
				WSURL:   "wss://example.com/v1/responses",
				Headers: http.Header{"X-Codex-Routing-Hint": []string{routingHint}},
			}
			first, err := pool.Acquire(context.Background(), req)
			require.NoError(t, err)
			defer first.Release()
			first.conn.createdAtNano.Store(time.Now().Add(-51 * time.Minute).UnixNano())

			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			type result struct {
				lease *openAIWSConnLease
				err   error
			}
			resultCh := make(chan result, 1)
			go func() {
				lease, acquireErr := pool.Acquire(ctx, req)
				resultCh <- result{lease: lease, err: acquireErr}
			}()
			require.Eventually(t, func() bool {
				return pool.SnapshotMetrics().AcquireQueueWaitTotal == 1
			}, time.Second, 5*time.Millisecond, "到龄连接仍被租用时应等待容量")
			queueObservedAt := time.Now()
			require.Eventually(t, func() bool {
				return time.Since(queueObservedAt) > 0
			}, time.Second, time.Millisecond, "释放前应形成可测量的排队时间")
			first.Release()
			select {
			case got := <-resultCh:
				require.NoError(t, got.err)
				require.NotNil(t, got.lease)
				defer got.lease.Release()
				require.NotEqual(t, first.ConnID(), got.lease.ConnID())
				require.False(t, got.lease.Reused())
				require.True(t, first.conn.isClosed(), "释放后应回收到龄连接")
				require.Greater(t, got.lease.QueueWaitDuration(), time.Duration(0))
			case <-ctx.Done():
				t.Fatal("释放到龄连接后未唤醒容量等待者")
			}
		})
	}
}

func TestOpenAIWSConnPool_AcquireExpiredBusyConnWaitHonorsContext(t *testing.T) {
	cfg := &config.Config{}
	cfg.Gateway.OpenAIWS.MaxConnsPerAccount = 1
	cfg.Gateway.OpenAIWS.MinIdlePerAccount = 0
	pool := newOpenAIWSConnPool(cfg)
	defer pool.Close()
	pool.setClientDialerForTest(&openAIWSQueueDialer{conns: []openAIWSClientConn{&openAIWSFakeConn{}}})
	req := openAIWSAcquireRequest{
		Account: &Account{ID: 305, Platform: PlatformOpenAI, Type: AccountTypeAPIKey},
		WSURL:   "wss://example.com/v1/responses",
	}
	first, err := pool.Acquire(context.Background(), req)
	require.NoError(t, err)
	defer first.Release()
	first.conn.createdAtNano.Store(time.Now().Add(-51 * time.Minute).UnixNano())
	ctx, cancel := context.WithTimeout(context.Background(), 40*time.Millisecond)
	defer cancel()
	lease, err := pool.Acquire(ctx, req)
	if lease != nil {
		lease.Release()
	}
	require.ErrorIs(t, err, context.DeadlineExceeded)
	require.False(t, first.conn.isClosed(), "等待超时不能关闭其他会话仍在使用的连接")
}
