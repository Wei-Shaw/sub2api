//go:build unit

package service

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// TestGoAsync_RunsTaskWithoutBlocking 验证 goAsync 立刻返回，并最终把任务跑完。
//
// 闭包里 sleep 50ms 模拟 SCAN+DEL；require.Eventually 等到 done=1 即认为异步任务执行了。
func TestGoAsync_RunsTaskWithoutBlocking(t *testing.T) {
	t.Parallel()
	svc := &serviceQuotaService{}

	var done atomic.Int32
	startedAt := time.Now()
	svc.goAsync(context.Background(), "test_op", 42, func(_ context.Context) {
		time.Sleep(50 * time.Millisecond)
		done.Store(1)
	})
	// goAsync 应该立刻返回，远小于闭包里的 sleep 时间。
	require.Less(t, time.Since(startedAt), 10*time.Millisecond, "goAsync 应非阻塞")

	require.Eventually(t, func() bool { return done.Load() == 1 },
		500*time.Millisecond, 5*time.Millisecond, "异步任务最终应完成")
}

// TestGoAsync_DoesNotPropagateOuterCancel 验证 outer ctx 取消不会让异步任务的 ctx 被取消。
//
// 这是 P0 修复的核心：admin 请求 ctx 在响应返回后会被框架取消，但 SCAN+DEL 还在跑。
// context.WithoutCancel 解除 cancel 联动，让异步 ctx.Done() 永不触发。
func TestGoAsync_DoesNotPropagateOuterCancel(t *testing.T) {
	t.Parallel()
	svc := &serviceQuotaService{}

	outerCtx, cancel := context.WithCancel(context.Background())

	asyncCtxCanceled := make(chan bool, 1)
	svc.goAsync(outerCtx, "test_op", 42, func(asyncCtx context.Context) {
		// 立刻取消 outer，sleep 一下让取消传播完（如果会传播的话）。
		cancel()
		time.Sleep(20 * time.Millisecond)
		select {
		case <-asyncCtx.Done():
			asyncCtxCanceled <- true
		default:
			asyncCtxCanceled <- false
		}
	})

	select {
	case canceled := <-asyncCtxCanceled:
		require.False(t, canceled, "outer ctx 取消不应传播到异步 ctx")
	case <-time.After(500 * time.Millisecond):
		t.Fatal("异步任务未在 500ms 内完成")
	}
}

// TestGoAsync_PanicRecovered 验证 goroutine 顶层 panic 被 recover，不会让进程崩溃。
//
// 用 sync.WaitGroup 等待 goroutine 退出（recover 后 defer 链结束 → goroutine 自然退出）。
// 这里通过 done channel 等"panic 后 goroutine 还能干净退出"作为 recover 生效的间接证据。
func TestGoAsync_PanicRecovered(t *testing.T) {
	t.Parallel()
	svc := &serviceQuotaService{}

	finished := make(chan struct{})
	svc.goAsync(context.Background(), "panic_op", 99, func(_ context.Context) {
		defer close(finished) // recover 后 defer 仍会执行
		panic("intentional test panic")
	})

	select {
	case <-finished:
		// goroutine 正常 unwound，说明 recover 拦住了 panic。
	case <-time.After(500 * time.Millisecond):
		t.Fatal("goroutine 未能正常退出（recover 未生效或被吞）")
	}
}
