package handler

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
)

func newWSSlotHandler(cache *concurrencyCacheMock) *OpenAIGatewayHandler {
	return &OpenAIGatewayHandler{
		concurrencyHelper: NewConcurrencyHelper(service.NewConcurrencyService(cache), SSEPingFormatNone, time.Second),
	}
}

func newWSSlotLogger() (*zap.Logger, *observer.ObservedLogs) {
	core, logs := observer.New(zap.InfoLevel)
	return zap.New(core), logs
}

func TestAcquireOpenAIWSAccountSlot_ImmediateHitSkipsWaitQueue(t *testing.T) {
	cache := &concurrencyCacheMock{
		acquireAccountSlotFn: func(context.Context, int64, int, string) (bool, error) { return true, nil },
	}
	h := newWSSlotHandler(cache)
	logger, logs := newWSSlotLogger()

	release, err := h.acquireOpenAIWSAccountSlot(context.Background(), logger, 3, 77, 5, 3, 30*time.Second)
	require.NoError(t, err)
	require.NotNil(t, release)

	// 绝大多数 turn 走这条路。入队/离队合计约 6 次 Redis 往返，为一次没发生的
	// 等待付出去是纯浪费，也会让账号反复进出活跃索引。
	require.Zero(t, atomic.LoadInt32(&cache.incrementAccountWait), "首次即命中时不该入队")
	require.Zero(t, atomic.LoadInt32(&cache.decrementAccountWait))
	require.Zero(t, logs.Len(), "没等过就不该记等待日志")
}

func TestAcquireOpenAIWSAccountSlot_WaitsAndLeavesQueue(t *testing.T) {
	var attempts int32
	cache := &concurrencyCacheMock{
		acquireAccountSlotFn: func(context.Context, int64, int, string) (bool, error) {
			// 第一次 miss 触发排队，之后放行。
			return atomic.AddInt32(&attempts, 1) > 2, nil
		},
	}
	h := newWSSlotHandler(cache)
	logger, logs := newWSSlotLogger()

	release, err := h.acquireOpenAIWSAccountSlot(context.Background(), logger, 7, 77, 5, 3, 5*time.Second)
	require.NoError(t, err)
	require.NotNil(t, release)

	require.Equal(t, int32(1), atomic.LoadInt32(&cache.incrementAccountWait), "真的要等时必须入队，否则等待者对 HTTP 准入和调度选号不可见")
	require.Equal(t, int32(1), atomic.LoadInt32(&cache.decrementAccountWait), "等待结束必须离队")

	entries := logs.FilterMessage("openai.websocket_account_slot_waited").All()
	require.Len(t, entries, 1, "等到了也要留痕，否则无从判断等待窗口是否起作用")
	require.Equal(t, int64(7), entries[0].ContextMap()["turn"])
}

func TestAcquireOpenAIWSAccountSlot_QueueFullFailsFast(t *testing.T) {
	cache := &concurrencyCacheMock{
		acquireAccountSlotFn:   func(context.Context, int64, int, string) (bool, error) { return false, nil },
		incrementAccountWaitFn: func(context.Context, int64, int) (bool, error) { return false, nil },
	}
	h := newWSSlotHandler(cache)
	logger, logs := newWSSlotLogger()

	start := time.Now()
	release, err := h.acquireOpenAIWSAccountSlot(context.Background(), logger, 2, 77, 5, 3, 30*time.Second)
	require.ErrorIs(t, err, errOpenAIWSAccountWaitQueueFull)
	require.Nil(t, release)
	require.Less(t, time.Since(start), time.Second, "队列满要立刻放弃，而不是再等满超时窗口")

	require.Zero(t, atomic.LoadInt32(&cache.decrementAccountWait), "没入队就不该出队，否则减掉的是别人的计数")
	require.Len(t, logs.FilterMessage("openai.websocket_account_wait_queue_full").All(), 1)
	require.True(t, openAIWSAccountSlotErrorIsBusy(context.Background(), err), "队列满对客户端是「稍后重试」而非内部错误")
}

func TestAcquireOpenAIWSAccountSlot_ZeroTimeoutKeepsTryOnce(t *testing.T) {
	cache := &concurrencyCacheMock{
		acquireAccountSlotFn: func(context.Context, int64, int, string) (bool, error) { return false, nil },
	}
	h := newWSSlotHandler(cache)

	// turn_slot_wait_timeout_seconds 设 0 表示显式关掉等待，回到旧行为。
	release, err := h.acquireOpenAIWSAccountSlot(context.Background(), zap.NewNop(), 4, 77, 5, 3, 0)
	require.Nil(t, release)
	var concurrencyErr *ConcurrencyError
	require.ErrorAs(t, err, &concurrencyErr)
	require.Zero(t, atomic.LoadInt32(&cache.incrementAccountWait), "不等就不该入队")
}

func TestAcquireOpenAIWSAccountSlot_SkipsQueueWithoutLimit(t *testing.T) {
	var attempts int32
	cache := &concurrencyCacheMock{
		acquireAccountSlotFn: func(context.Context, int64, int, string) (bool, error) {
			return atomic.AddInt32(&attempts, 1) > 1, nil
		},
	}
	h := newWSSlotHandler(cache)

	// Lua 脚本以 current >= maxWait 判拒，maxWaiting=0 照传会把所有等待一律拒掉。
	release, err := h.acquireOpenAIWSAccountSlot(context.Background(), zap.NewNop(), 5, 77, 5, 0, 5*time.Second)
	require.NoError(t, err)
	require.NotNil(t, release)
	require.Zero(t, atomic.LoadInt32(&cache.incrementAccountWait))
	require.Zero(t, atomic.LoadInt32(&cache.decrementAccountWait))
}

func TestAcquireOpenAIWSAccountSlot_TimeoutLeavesQueueAndClassifiesBusy(t *testing.T) {
	cache := &concurrencyCacheMock{
		acquireAccountSlotFn: func(context.Context, int64, int, string) (bool, error) { return false, nil },
	}
	h := newWSSlotHandler(cache)
	logger, logs := newWSSlotLogger()

	release, err := h.acquireOpenAIWSAccountSlot(context.Background(), logger, 9, 77, 5, 3, 150*time.Millisecond)
	require.Error(t, err)
	require.Nil(t, release)
	require.True(t, openAIWSAccountSlotErrorIsBusy(context.Background(), err))

	require.Equal(t, int32(1), atomic.LoadInt32(&cache.decrementAccountWait), "等满超时也必须离队，否则计数器泄漏")
	require.Len(t, logs.FilterMessage("openai.websocket_account_slot_wait_failed").All(), 1)
}

func TestOpenAIWSAccountSlotErrorIsBusy_SeparatesInternalFailures(t *testing.T) {
	require.False(t, openAIWSAccountSlotErrorIsBusy(context.Background(), errors.New("redis down")),
		"底层故障应当走内部错误，而不是让客户端以为账号忙")

	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	require.True(t, openAIWSAccountSlotErrorIsBusy(cancelled, errors.New("redis down")),
		"对端已经走了就不必再区分原因")
}
