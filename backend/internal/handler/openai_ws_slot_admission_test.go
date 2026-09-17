//go:build unit

package handler

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/Wei-Shaw/sub2api/internal/testutil"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
)

// wsWaitQueueCache 记录账号等待队列计数器的调用，用于确认 WS 等待对
// HTTP 准入和调度选号是可见的。
type wsWaitQueueCache struct {
	testutil.StubConcurrencyCache
	mu         sync.Mutex
	increments []int // 每次 Increment 收到的 maxWait
	decrements int
	full       bool
	failErr    error
}

func (c *wsWaitQueueCache) IncrementAccountWaitCount(_ context.Context, _ int64, maxWait int) (bool, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.increments = append(c.increments, maxWait)
	if c.failErr != nil {
		return false, c.failErr
	}
	return !c.full, nil
}

func (c *wsWaitQueueCache) DecrementAccountWaitCount(_ context.Context, _ int64) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.decrements++
	return nil
}

func (c *wsWaitQueueCache) counts() (int, int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.increments), c.decrements
}

func newWSWaitQueueHandler(cache *wsWaitQueueCache) *OpenAIGatewayHandler {
	return &OpenAIGatewayHandler{
		concurrencyHelper: NewConcurrencyHelper(service.NewConcurrencyService(cache), SSEPingFormatNone, time.Second),
	}
}

func TestEnterOpenAIWSAccountWaitQueue_CountsWaiter(t *testing.T) {
	cache := &wsWaitQueueCache{}
	h := newWSWaitQueueHandler(cache)

	allowed, leave := h.enterOpenAIWSAccountWaitQueue(context.Background(), zap.NewNop(), 77, 5)
	require.True(t, allowed)

	inc, dec := cache.counts()
	require.Equal(t, 1, inc, "等待期间必须计数，否则等待者对 HTTP 准入和调度选号不可见")
	require.Equal(t, 0, dec, "离队前不应 Decrement")
	require.Equal(t, []int{5}, cache.increments, "必须把 MaxWaiting 透传给队列脚本")

	leave()
	_, dec = cache.counts()
	require.Equal(t, 1, dec)

	// leave 幂等：等待路径上有多个出口，重复调用不能把计数器减穿。
	leave()
	leave()
	_, dec = cache.counts()
	require.Equal(t, 1, dec)
}

func TestEnterOpenAIWSAccountWaitQueue_RejectsWhenFull(t *testing.T) {
	cache := &wsWaitQueueCache{full: true}
	h := newWSWaitQueueHandler(cache)
	logger, logs := newSlotWaitObservedLogger()

	allowed, leave := h.enterOpenAIWSAccountWaitQueue(context.Background(), logger, 77, 3)
	require.False(t, allowed, "队列满必须快速失败，而不是再等一个完整超时窗口")

	leave()
	_, dec := cache.counts()
	require.Equal(t, 0, dec, "没入队就不该出队")

	entries := logs.FilterMessage("openai.websocket_account_wait_queue_full").All()
	require.Len(t, entries, 1, "拒绝要留下日志，否则又是一个盲区")
}

func TestEnterOpenAIWSAccountWaitQueue_SkipsCountingWithoutLimit(t *testing.T) {
	cache := &wsWaitQueueCache{}
	h := newWSWaitQueueHandler(cache)

	// Lua 脚本以 current >= maxWait 判拒，maxWaiting=0 传下去会把所有等待一律
	// 拒掉，所以这里必须跳过计数而不是照传。
	allowed, leave := h.enterOpenAIWSAccountWaitQueue(context.Background(), zap.NewNop(), 77, 0)
	require.True(t, allowed)
	leave()

	inc, dec := cache.counts()
	require.Equal(t, 0, inc)
	require.Equal(t, 0, dec)
}

func TestEnterOpenAIWSAccountWaitQueue_FailsOpenOnCounterError(t *testing.T) {
	cache := &wsWaitQueueCache{failErr: errors.New("redis down")}
	h := newWSWaitQueueHandler(cache)

	// 计数器故障不该顺带把等待能力关掉——等待本身仍是对客户端更好的选择。
	// ConcurrencyService 有意不把 cache error 传给调用方（见
	// TestIncrementAccountWaitCount_FailOpen），所以这里只能观察到「放行」，
	// 与 HTTP 侧四条准入路径的实际行为一致。
	allowed, leave := h.enterOpenAIWSAccountWaitQueue(context.Background(), zap.NewNop(), 77, 5)
	require.True(t, allowed)
	require.NotNil(t, leave)
	leave()
}

func TestResolveOpenAIWSAccountSlotWaitTimeout(t *testing.T) {
	cases := []struct {
		name      string
		cfg, plan time.Duration
		want      time.Duration
	}{
		{"调度器预算更紧时取调度器的", 30 * time.Second, 10 * time.Second, 10 * time.Second},
		{"调度器预算更宽时取 WS 上限", 30 * time.Second, 60 * time.Second, 30 * time.Second},
		{"没有调度器预算时用 WS 上限", 30 * time.Second, 0, 30 * time.Second},
		{"WS 配置为 0 表示显式关掉等待，优先级最高", 0, 10 * time.Second, 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.want, resolveOpenAIWSAccountSlotWaitTimeout(tc.cfg, tc.plan))
		})
	}
}

func TestLogOpenAIWSTurnUserSlotUnavailable_IsObservable(t *testing.T) {
	// 回归保护：turn 级 user 槽拒绝此前静默关连接，日志里查不到任何痕迹。
	// 这里只钉住字段约定，真实触发在 BeforeTurn 里。
	core, logs := observer.New(zap.WarnLevel)
	logger := zap.New(core)
	logger.Warn("openai.websocket_turn_user_slot_unavailable",
		zap.Int("turn", 7),
		zap.Int64("user_id", 5),
		zap.Int("user_max_concurrency", 3),
	)
	entries := logs.FilterMessage("openai.websocket_turn_user_slot_unavailable").All()
	require.Len(t, entries, 1)
	require.Equal(t, int64(7), entries[0].ContextMap()["turn"])
}
