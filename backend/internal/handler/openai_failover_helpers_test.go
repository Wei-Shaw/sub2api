package handler

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

// TestRecordOpenAIFanoutAndCheckExhausted 覆盖瑕疵 2 修复的主要分支：
//   - limiter / provider / sessionHash 缺失 → noop
//   - fanoutLimit ≤ 0 → noop（向后兼容）
//   - 未超限 → 记录但返回 false
//   - 超限 → 返回 true（调用方应改走 exhausted 分支）
func TestRecordOpenAIFanoutAndCheckExhausted(t *testing.T) {
	ctx := context.Background()
	groupID := int64(7)

	t.Run("limiter 为 nil 时退化为 noop", func(t *testing.T) {
		exhausted := recordOpenAIFanoutAndCheckExhausted(
			ctx, nil,
			staticSessionFanoutConfigProvider{limit: 2},
			100, "sess1", &groupID, 500, zap.NewNop(),
		)
		require.False(t, exhausted, "limiter==nil 不应阻断切号")
	})

	t.Run("provider 为 nil 时退化为 noop", func(t *testing.T) {
		mock := newMockSessionFanoutLimiter(2)
		exhausted := recordOpenAIFanoutAndCheckExhausted(
			ctx, mock, nil,
			100, "sess1", &groupID, 500, zap.NewNop(),
		)
		require.False(t, exhausted, "provider==nil 不应阻断切号")
		require.Empty(t, mock.fanoutSets, "未走 RecordSessionFanout")
	})

	t.Run("sessionHash 为空时退化为 noop", func(t *testing.T) {
		mock := newMockSessionFanoutLimiter(2)
		exhausted := recordOpenAIFanoutAndCheckExhausted(
			ctx, mock,
			staticSessionFanoutConfigProvider{limit: 2},
			100, "", &groupID, 500, zap.NewNop(),
		)
		require.False(t, exhausted)
		require.Empty(t, mock.fanoutSets, "空 sessionHash 不记录")
	})

	t.Run("fanoutLimit ≤ 0 视为禁用", func(t *testing.T) {
		mock := newMockSessionFanoutLimiter(2)
		exhausted := recordOpenAIFanoutAndCheckExhausted(
			ctx, mock,
			staticSessionFanoutConfigProvider{limit: 0},
			100, "sess1", &groupID, 500, zap.NewNop(),
		)
		require.False(t, exhausted)
		require.Empty(t, mock.fanoutSets, "limit=0 不应记录")
	})

	t.Run("limit=2 第一次记录后未超限", func(t *testing.T) {
		mock := newMockSessionFanoutLimiter(2)
		exhausted := recordOpenAIFanoutAndCheckExhausted(
			ctx, mock,
			staticSessionFanoutConfigProvider{limit: 2},
			100, "sess1", &groupID, 500, zap.NewNop(),
		)
		require.False(t, exhausted)
		require.Len(t, mock.fanoutSets["sess1"], 1)
	})

	t.Run("limit=2 第二个不同账号即超限", func(t *testing.T) {
		mock := newMockSessionFanoutLimiter(2)
		// 第一次
		recordOpenAIFanoutAndCheckExhausted(
			ctx, mock,
			staticSessionFanoutConfigProvider{limit: 2},
			100, "sess1", &groupID, 500, zap.NewNop(),
		)
		// 第二次（达到 limit=2 即 exhausted）
		exhausted := recordOpenAIFanoutAndCheckExhausted(
			ctx, mock,
			staticSessionFanoutConfigProvider{limit: 2},
			200, "sess1", &groupID, 500, zap.NewNop(),
		)
		require.True(t, exhausted, "达到 limit=2 即 exhausted")
		require.Len(t, mock.fanoutSets["sess1"], 2)
	})

	t.Run("同一账号多次记录不消耗 fanout 配额", func(t *testing.T) {
		mock := newMockSessionFanoutLimiter(2)
		for i := 0; i < 5; i++ {
			exhausted := recordOpenAIFanoutAndCheckExhausted(
				ctx, mock,
				staticSessionFanoutConfigProvider{limit: 2},
				100, "sess1", &groupID, 500, zap.NewNop(),
			)
			require.False(t, exhausted, "第 %d 次同账号不应超限", i+1)
		}
		require.Len(t, mock.fanoutSets["sess1"], 1)
	})

	t.Run("不同 session 互不干扰", func(t *testing.T) {
		mock := newMockSessionFanoutLimiter(2)
		recordOpenAIFanoutAndCheckExhausted(
			ctx, mock,
			staticSessionFanoutConfigProvider{limit: 2},
			100, "sessA", &groupID, 500, zap.NewNop(),
		)
		recordOpenAIFanoutAndCheckExhausted(
			ctx, mock,
			staticSessionFanoutConfigProvider{limit: 2},
			200, "sessA", &groupID, 500, zap.NewNop(),
		)
		// sessB 仍未超限
		exhausted := recordOpenAIFanoutAndCheckExhausted(
			ctx, mock,
			staticSessionFanoutConfigProvider{limit: 2},
			300, "sessB", &groupID, 500, zap.NewNop(),
		)
		require.False(t, exhausted)
		require.Len(t, mock.fanoutSets["sessA"], 2)
		require.Len(t, mock.fanoutSets["sessB"], 1)
	})

	t.Run("nil reqLog 时不 panic", func(t *testing.T) {
		mock := newMockSessionFanoutLimiter(1)
		require.NotPanics(t, func() {
			recordOpenAIFanoutAndCheckExhausted(
				ctx, mock,
				staticSessionFanoutConfigProvider{limit: 1},
				100, "sess1", &groupID, 500, nil,
			)
		})
	})
}

// TestApplyOpenAIFailoverJitter 覆盖瑕疵 2 抖动延迟分支：
//   - 第 1 次切换不引入抖动
//   - 第 2 次起绑定会话使用 boundJitter[min,max]
//   - 第 2 次起非绑定会话使用 0-600ms
//   - ctx 取消立即返回 false
func TestApplyOpenAIFailoverJitter(t *testing.T) {
	provider := staticSessionFanoutConfigProvider{
		limit: 5,
		min:   2 * time.Second,
		max:   5 * time.Second,
	}

	t.Run("第 1 次切换不引入抖动", func(t *testing.T) {
		start := time.Now()
		ok := applyOpenAIFailoverJitter(context.Background(), provider, true, 1, zap.NewNop())
		elapsed := time.Since(start)
		require.True(t, ok)
		require.Less(t, elapsed, 50*time.Millisecond, "第 1 次切换无抖动")
	})

	t.Run("绑定会话第 2 次切换使用 bound jitter", func(t *testing.T) {
		start := time.Now()
		ok := applyOpenAIFailoverJitter(context.Background(), provider, true, 2, zap.NewNop())
		elapsed := time.Since(start)
		require.True(t, ok)
		// 绑定会话应在 [2s, 5s] 之间
		require.GreaterOrEqual(t, elapsed, 1800*time.Millisecond, "应至少 ~2s")
		require.Less(t, elapsed, 6*time.Second, "应小于 5s + 调度噪声")
	})

	t.Run("非绑定会话第 2 次切换使用 0-600ms", func(t *testing.T) {
		start := time.Now()
		ok := applyOpenAIFailoverJitter(context.Background(), provider, false, 2, zap.NewNop())
		elapsed := time.Since(start)
		require.True(t, ok)
		require.Less(t, elapsed, 700*time.Millisecond, "非绑定应 < 600ms + 噪声")
	})

	t.Run("provider 为 nil 时仍提供 0-600ms 抖动", func(t *testing.T) {
		// 没有 provider 信息时，bound 分支因 boundJitterMax==0 退回到 cross-account jitter。
		start := time.Now()
		ok := applyOpenAIFailoverJitter(context.Background(), nil, true, 2, zap.NewNop())
		elapsed := time.Since(start)
		require.True(t, ok)
		require.Less(t, elapsed, 700*time.Millisecond, "无 provider 时应退化到 cross-account jitter (≤600ms)")
	})

	t.Run("绑定会话但 boundJitterMax=0 退化到 cross-account jitter", func(t *testing.T) {
		zero := staticSessionFanoutConfigProvider{limit: 5, min: 0, max: 0}
		start := time.Now()
		ok := applyOpenAIFailoverJitter(context.Background(), zero, true, 2, zap.NewNop())
		elapsed := time.Since(start)
		require.True(t, ok)
		require.Less(t, elapsed, 700*time.Millisecond)
	})

	t.Run("ctx 取消立即返回 false", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		start := time.Now()
		ok := applyOpenAIFailoverJitter(ctx, provider, true, 2, zap.NewNop())
		elapsed := time.Since(start)
		require.False(t, ok)
		require.Less(t, elapsed, 50*time.Millisecond)
	})

	t.Run("nil reqLog 时不 panic", func(t *testing.T) {
		require.NotPanics(t, func() {
			applyOpenAIFailoverJitter(context.Background(), provider, true, 2, nil)
		})
	})
}
