package handler

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestAcquireAccountSlotWithWaitCtx_AcquiresImmediately(t *testing.T) {
	var attempts int32
	cache := &concurrencyCacheMock{
		acquireAccountSlotFn: func(ctx context.Context, accountID int64, maxConcurrency int, requestID string) (bool, error) {
			atomic.AddInt32(&attempts, 1)
			return true, nil
		},
	}
	helper := NewConcurrencyHelper(service.NewConcurrencyService(cache), SSEPingFormatNone, time.Second)

	start := time.Now()
	release, err := helper.AcquireAccountSlotWithWaitCtx(context.Background(), 201, 1, 5*time.Second)
	require.NoError(t, err)
	require.NotNil(t, release)
	// 首次尝试必须立即发生，不能先吃掉一个退避周期。
	require.Less(t, time.Since(start), 100*time.Millisecond)
	require.Equal(t, int32(1), atomic.LoadInt32(&attempts))

	release()
	require.Equal(t, int32(1), atomic.LoadInt32(&cache.releaseAccountCalled))
}

func TestAcquireAccountSlotWithWaitCtx_WaitsForSlotToFreeUp(t *testing.T) {
	var attempts int32
	cache := &concurrencyCacheMock{
		acquireAccountSlotFn: func(ctx context.Context, accountID int64, maxConcurrency int, requestID string) (bool, error) {
			// 前两次模拟槽位被同账号的其他连接占满，第三次才释放出来。
			return atomic.AddInt32(&attempts, 1) >= 3, nil
		},
	}
	helper := NewConcurrencyHelper(service.NewConcurrencyService(cache), SSEPingFormatNone, time.Second)

	release, err := helper.AcquireAccountSlotWithWaitCtx(context.Background(), 202, 1, 5*time.Second)
	require.NoError(t, err)
	require.NotNil(t, release)
	require.GreaterOrEqual(t, atomic.LoadInt32(&attempts), int32(3))
}

func TestAcquireAccountSlotWithWaitCtx_TimesOutAsConcurrencyError(t *testing.T) {
	var attempts int32
	cache := &concurrencyCacheMock{
		acquireAccountSlotFn: func(ctx context.Context, accountID int64, maxConcurrency int, requestID string) (bool, error) {
			atomic.AddInt32(&attempts, 1)
			return false, nil
		},
	}
	helper := NewConcurrencyHelper(service.NewConcurrencyService(cache), SSEPingFormatNone, time.Second)

	release, err := helper.AcquireAccountSlotWithWaitCtx(context.Background(), 203, 1, 300*time.Millisecond)
	require.Nil(t, release)

	var concurrencyErr *ConcurrencyError
	require.True(t, errors.As(err, &concurrencyErr))
	require.True(t, concurrencyErr.IsTimeout)
	require.Equal(t, "account", concurrencyErr.SlotType)
	// 超时前应该重试过若干次，而不是只试一次就放弃。
	require.Greater(t, atomic.LoadInt32(&attempts), int32(1))
	require.Equal(t, int32(0), atomic.LoadInt32(&cache.releaseAccountCalled))
}

func TestAcquireAccountSlotWithWaitCtx_ZeroTimeoutTriesOnce(t *testing.T) {
	var attempts int32
	cache := &concurrencyCacheMock{
		acquireAccountSlotFn: func(ctx context.Context, accountID int64, maxConcurrency int, requestID string) (bool, error) {
			atomic.AddInt32(&attempts, 1)
			return false, nil
		},
	}
	helper := NewConcurrencyHelper(service.NewConcurrencyService(cache), SSEPingFormatNone, time.Second)

	release, err := helper.AcquireAccountSlotWithWaitCtx(context.Background(), 204, 1, 0)
	require.Nil(t, release)

	var concurrencyErr *ConcurrencyError
	require.True(t, errors.As(err, &concurrencyErr))
	require.True(t, concurrencyErr.IsTimeout)
	// timeout<=0 保持 legacy 的「只试一次」语义，供紧急回滚使用。
	require.Equal(t, int32(1), atomic.LoadInt32(&attempts))
}

func TestAcquireAccountSlotWithWaitCtx_ParentContextCancelWins(t *testing.T) {
	cache := &concurrencyCacheMock{
		acquireAccountSlotFn: func(ctx context.Context, accountID int64, maxConcurrency int, requestID string) (bool, error) {
			return false, nil
		},
	}
	helper := NewConcurrencyHelper(service.NewConcurrencyService(cache), SSEPingFormatNone, time.Second)

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()

	release, err := helper.AcquireAccountSlotWithWaitCtx(ctx, 205, 1, 10*time.Second)
	require.Nil(t, release)
	// 客户端先走掉时返回原始取消原因，调用方据此区分「容量不足」与「对端已断」。
	require.ErrorIs(t, err, context.Canceled)

	var concurrencyErr *ConcurrencyError
	require.False(t, errors.As(err, &concurrencyErr))
}

func TestAcquireAccountSlotWithWaitCtx_UnlimitedConcurrency(t *testing.T) {
	cache := &concurrencyCacheMock{
		acquireAccountSlotFn: func(ctx context.Context, accountID int64, maxConcurrency int, requestID string) (bool, error) {
			t.Fatal("unlimited concurrency must not reach the slot cache")
			return false, nil
		},
	}
	helper := NewConcurrencyHelper(service.NewConcurrencyService(cache), SSEPingFormatNone, time.Second)

	release, err := helper.AcquireAccountSlotWithWaitCtx(context.Background(), 206, 0, 5*time.Second)
	require.NoError(t, err)
	require.NotNil(t, release)
	release()
}
